package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"gping/geoip"
)

var errInvalidArgs = errors.New("invalid arguments")

type ipFamily int

const (
	ipFamilyAny ipFamily = iota
	ipFamily4
	ipFamily6
)

func main() {
	dnsServer, target, family, err := parseArgs(os.Args[1:])
	if err != nil {
		if errors.Is(err, errInvalidArgs) {
			printUsage()
			os.Exit(1)
		}
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	targetIP, targetHost, dnsUsed, allIPs, err := resolveTarget(target, dnsServer, family)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	location := "Unknown"
	lookup, err := geoip.NewGeoIPLookup()
	if err != nil {
		fmt.Printf("Warning: Failed to load GeoIP database: %v\n", err)
		fmt.Println("Continuing with ping...")
	} else {
		defer lookup.Close()

		cityInfo, err := lookup.LookupCity(targetIP)
		if err != nil {
			cityInfo = &geoip.CityInfo{}
		}
		location = formatLocation(cityInfo)
	}

	if dnsUsed != "" {
		fmt.Printf("DNS Server: %s\n", dnsUsed)
	}

	if len(allIPs) > 1 {
		fmt.Println("IPs: " + formatPingIPList(allIPs, targetIP))
		fmt.Println()
	}

	printGPINGLine(target, targetIP, location)

	if err := executePing(targetHost); err != nil {
		fmt.Printf("Ping failed: %v\n", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Usage:")
	fmt.Println("  gping <host> [-4|-6]")
	fmt.Println("  gping <dns> <host> [-4|-6]")
	fmt.Println("  Options may also appear before the host.")
	fmt.Println("Examples:")
	fmt.Println("  gping 1.1.1.1")
	fmt.Println("  gping ipxy.cc")
	fmt.Println("  gping ipxy.cc -4")
	fmt.Println("  gping ipxy.cc -6")
	fmt.Println("  gping 22.22.22.22 translate.googleapis.com -4")
	fmt.Println("  gping -4 ipxy.cc")
	fmt.Println("  gping 127.0.0.1:253 ipxy.cc -4")
}

func flagLabel(f ipFamily) string {
	if f == ipFamily4 {
		return "-4"
	}
	return "-6"
}

func bumpFamily(family *ipFamily, v ipFamily) error {
	switch *family {
	case ipFamilyAny:
		*family = v
		return nil
	case v:
		return fmt.Errorf("duplicate %s flag", flagLabel(v))
	default:
		return errors.New("-4 and -6 are mutually exclusive")
	}
}

// parseArgs accepts [-4|-6] at the end and/or before positional args, then:
// one arg (<host>) or two (<dns> <host>).
func parseArgs(argv []string) (dnsServer, target string, family ipFamily, err error) {
	args := append([]string(nil), argv...)

	for len(args) > 0 {
		switch args[len(args)-1] {
		case "-4":
			if e := bumpFamily(&family, ipFamily4); e != nil {
				return "", "", 0, e
			}
			args = args[:len(args)-1]
		case "-6":
			if e := bumpFamily(&family, ipFamily6); e != nil {
				return "", "", 0, e
			}
			args = args[:len(args)-1]
		default:
			goto stripLeading
		}
	}
stripLeading:
	for len(args) > 0 {
		switch args[0] {
		case "-4":
			if e := bumpFamily(&family, ipFamily4); e != nil {
				return "", "", 0, e
			}
			args = args[1:]
		case "-6":
			if e := bumpFamily(&family, ipFamily6); e != nil {
				return "", "", 0, e
			}
			args = args[1:]
		default:
			goto positional
		}
	}
positional:
	if len(args) < 1 || len(args) > 2 {
		return "", "", 0, errInvalidArgs
	}
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			return "", "", 0, fmt.Errorf("unknown option %q (only -4 and -6 are supported)", a)
		}
	}
	target = args[len(args)-1]
	if len(args) == 2 {
		dnsServer = strings.TrimSpace(args[0])
		if dnsServer == "" {
			return "", "", 0, fmt.Errorf("DNS server address cannot be empty")
		}
	}
	return dnsServer, target, family, nil
}

// normalizeDNSAddr returns a host:port suitable for net.Dial. Port defaults to 53.
func normalizeDNSAddr(addr string) (string, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return "", errors.New("empty address")
	}
	host, port, err := net.SplitHostPort(addr)
	if err == nil {
		return net.JoinHostPort(host, port), nil
	}
	if ip := net.ParseIP(addr); ip != nil {
		return net.JoinHostPort(ip.String(), "53"), nil
	}
	return net.JoinHostPort(addr, "53"), nil
}

func filterByFamily(ips []net.IP, family ipFamily) []net.IP {
	if family == ipFamilyAny {
		return ips
	}
	var out []net.IP
	for _, ip := range ips {
		is4 := ip.To4() != nil
		switch family {
		case ipFamily4:
			if is4 {
				out = append(out, ip)
			}
		case ipFamily6:
			if !is4 {
				out = append(out, ip)
			}
		}
	}
	return out
}

func errNoAddrFamily(family ipFamily) error {
	switch family {
	case ipFamily4:
		return fmt.Errorf("no IPv4 addresses found")
	case ipFamily6:
		return fmt.Errorf("no IPv6 addresses found")
	default:
		return fmt.Errorf("no IP addresses found")
	}
}

// uniqueIPStrings returns distinct IP text representations, preserving first-seen order.
func uniqueIPStrings(ips []net.IP) []string {
	seen := make(map[string]struct{}, len(ips))
	out := make([]string, 0, len(ips))
	for _, ip := range ips {
		s := ip.String()
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// formatPingIPList is one line of all resolved addresses. The address used for ping
// is listed first with a * prefix (e.g. *1.1.1.1, 1.0.0.1). Used only when there are multiple IPs.
func formatPingIPList(all []string, selected string) string {
	var rest []string
	for _, ip := range all {
		if ip != selected {
			rest = append(rest, ip)
		}
	}
	out := "*" + selected
	if len(rest) > 0 {
		out += ", " + strings.Join(rest, ", ")
	}
	return out
}

func resolveTarget(target, dnsServer string, family ipFamily) (ip string, host string, dnsUsed string, allIPs []string, err error) {
	if parsed := net.ParseIP(target); parsed != nil {
		switch family {
		case ipFamily4:
			if parsed.To4() == nil {
				return "", "", "", nil, fmt.Errorf("not an IPv4 address (try without -4 or use -6)")
			}
		case ipFamily6:
			if parsed.To4() != nil {
				return "", "", "", nil, fmt.Errorf("not an IPv6 address (try without -6 or use -4)")
			}
		}
		return target, target, "", []string{target}, nil
	}

	if dnsServer == "" {
		ips, err := net.LookupIP(target)
		if err != nil {
			return "", "", "", nil, fmt.Errorf("DNS lookup failed: %w", err)
		}
		ips = filterByFamily(ips, family)
		if len(ips) == 0 {
			return "", "", "", nil, errNoAddrFamily(family)
		}
		allIPs = uniqueIPStrings(ips)
		selected := allIPs[rand.Intn(len(allIPs))]
		return selected, selected, "", allIPs, nil
	}

	dialAddr, err := normalizeDNSAddr(dnsServer)
	if err != nil {
		return "", "", "", nil, fmt.Errorf("invalid DNS server: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	r := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			d := net.Dialer{Timeout: 5 * time.Second}
			return d.DialContext(ctx, network, dialAddr)
		},
	}

	addrs, err := r.LookupIPAddr(ctx, target)
	if err != nil {
		return "", "", "", nil, fmt.Errorf("DNS lookup failed: %w", err)
	}
	var ips []net.IP
	for _, a := range addrs {
		ips = append(ips, a.IP)
	}
	ips = filterByFamily(ips, family)
	if len(ips) == 0 {
		return "", "", "", nil, errNoAddrFamily(family)
	}

	allIPs = uniqueIPStrings(ips)
	selected := allIPs[rand.Intn(len(allIPs))]
	return selected, selected, dialAddr, allIPs, nil
}

// formatLocation returns "country, province, city" omitting empty segments and
// consecutive duplicates (e.g. Singapore as both country and province → "Singapore").
func formatLocation(c *geoip.CityInfo) string {
	if c == nil {
		return "Unknown"
	}
	var parts []string
	for _, s := range []string{c.Country, c.Province, c.City} {
		if s == "" {
			continue
		}
		if len(parts) > 0 && parts[len(parts)-1] == s {
			continue
		}
		parts = append(parts, s)
	}
	if len(parts) == 0 {
		return "Unknown"
	}
	return strings.Join(parts, ", ")
}

func pingCommand(host string) (name string, args []string) {
	ip := net.ParseIP(host)
	if ip != nil && ip.To4() == nil {
		if runtime.GOOS == "darwin" {
			return "ping6", []string{host}
		}
		return "ping", []string{"-6", host}
	}
	return "ping", []string{host}
}

// skipPingBannerLine drops the first line of ping output when it is the standard
// "PING host (ip): ..." banner, since gping already prints host, IP, and geo above.
func skipPingBannerLine(r io.Reader, w io.Writer) error {
	scanner := bufio.NewScanner(r)
	first := true
	for scanner.Scan() {
		line := scanner.Text()
		if first {
			first = false
			if strings.HasPrefix(line, "PING ") {
				continue
			}
		}
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func executePing(host string) error {
	// Terminal SIGINT is delivered to the whole foreground process group. If the parent
	// uses the default handler, the Go runtime prints "signal: interrupt" after the
	// child (ping) has already handled Ctrl+C gracefully. Ignore SIGINT here so ping
	// alone receives meaningful interrupt handling and normal stats output.
	signal.Ignore(os.Interrupt)
	defer signal.Reset(os.Interrupt)

	name, args := pingCommand(host)
	cmd := exec.Command(name, args...)
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}

	streamErr := skipPingBannerLine(stdout, os.Stdout)
	waitErr := cmd.Wait()
	if streamErr != nil {
		return streamErr
	}
	if waitErr == nil {
		return nil
	}
	// macOS/Linux: Ctrl+C terminates ping via SIGINT; treat as successful stop.
	var exitErr *exec.ExitError
	if errors.As(waitErr, &exitErr) && wasSignaledSIGINT(exitErr) {
		return nil
	}
	return waitErr
}

func wasSignaledSIGINT(exitErr *exec.ExitError) bool {
	if exitErr == nil {
		return false
	}
	ws, ok := exitErr.Sys().(syscall.WaitStatus)
	return ok && ws.Signaled() && ws.Signal() == syscall.SIGINT
}
