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
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/miekg/dns"

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
	dnsServer, target, family, count, jsonOut, err := parseArgs(os.Args[1:])
	if err != nil {
		if errors.Is(err, errInvalidArgs) {
			printUsage()
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if jsonOut && count == 0 {
		count = 4
	}

	targetIP, targetHost, dnsUsed, allIPs, err := resolveTarget(target, dnsServer, family)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	var cityInfo *geoip.CityInfo
	lookup, err := geoip.NewGeoIPLookup()
	if err != nil {
		if !jsonOut {
			fmt.Printf("Warning: Failed to load GeoIP database: %v\n", err)
			fmt.Println("Continuing with ping...")
		} else {
			fmt.Fprintf(os.Stderr, "Warning: Failed to load GeoIP database: %v\n", err)
		}
	} else {
		defer lookup.Close()
		cityInfo, err = lookup.LookupCity(targetIP)
		if err != nil {
			cityInfo = &geoip.CityInfo{}
		}
	}

	cname := lookupCNAME(target, dnsServer)

	if jsonOut {
		if err := runJSON(target, targetIP, targetHost, dnsUsed, cname, allIPs, cityInfo, count); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if dnsUsed != "" {
		fmt.Printf("DNS Server: %s\n", dnsUsed)
	}
	if cname != "" {
		printCNAMELine(target, cname)
	}
	if len(allIPs) > 1 {
		fmt.Println("IPs: " + formatPingIPList(allIPs, targetIP))
	}
	if cname != "" || len(allIPs) > 1 {
		fmt.Println()
	}

	printGPINGLine(target, targetIP, formatLocation(cityInfo))

	if err := executePing(targetHost, count, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "Ping failed: %v\n", err)
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Usage:")
	fmt.Println("  gping <host> [-4|-6] [-c N] [-json]")
	fmt.Println("  gping <dns> <host> [-4|-6] [-c N] [-json]")
	fmt.Println("  Options may appear before, between, or after positional args.")
	fmt.Println("  -json prints a single JSON object after ping completes (defaults -c to 4).")
	fmt.Println("DNS forms: <ip|host[:port]> | <alias> | doh://<alias|url> | dot://<alias|host[:port]>")
	fmt.Println("Built-in aliases: cf, cloudflare, google, g, quad9, adguard, ali, aliyun, dnspod, tx, 360")
	fmt.Println("User aliases: ~/.config/gping/dns.toml")
	fmt.Println("Examples:")
	fmt.Println("  gping 1.1.1.1")
	fmt.Println("  gping ipxy.cc")
	fmt.Println("  gping cf gping.dev                          # DoH cloudflare via alias")
	fmt.Println("  gping ali www.youtube.com -c 3              # 阿里 DoH")
	fmt.Println("  gping dot://cf gping.dev                    # DoT cloudflare")
	fmt.Println("  gping doh://dns.google/dns-query baidu.com  # full DoH URL")
	fmt.Println("  gping 22.22.22.22 translate.googleapis.com -4")
	fmt.Println("  gping 127.0.0.1:253 ipxy.cc -4 -c 3")
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

// parseArgs accepts options ([-4|-6], [-c N]) interleaved with one or two
// positional args: <host> or <dns> <host>.
func parseArgs(argv []string) (dnsServer, target string, family ipFamily, count int, jsonOut bool, err error) {
	var positional []string
	for i := 0; i < len(argv); i++ {
		a := argv[i]
		switch {
		case a == "-4":
			if e := bumpFamily(&family, ipFamily4); e != nil {
				return "", "", 0, 0, false, e
			}
		case a == "-6":
			if e := bumpFamily(&family, ipFamily6); e != nil {
				return "", "", 0, 0, false, e
			}
		case a == "-c":
			if i+1 >= len(argv) {
				return "", "", 0, 0, false, fmt.Errorf("-c requires a positive integer")
			}
			i++
			if count != 0 {
				return "", "", 0, 0, false, errors.New("duplicate -c flag")
			}
			n, e := strconv.Atoi(argv[i])
			if e != nil || n <= 0 {
				return "", "", 0, 0, false, fmt.Errorf("-c requires a positive integer, got %q", argv[i])
			}
			count = n
		case a == "-json" || a == "--json":
			if jsonOut {
				return "", "", 0, 0, false, errors.New("duplicate -json flag")
			}
			jsonOut = true
		case strings.HasPrefix(a, "-"):
			return "", "", 0, 0, false, fmt.Errorf("unknown option %q (only -4, -6, -c, -json are supported)", a)
		default:
			positional = append(positional, a)
		}
	}
	if len(positional) < 1 || len(positional) > 2 {
		return "", "", 0, 0, false, errInvalidArgs
	}
	target = positional[len(positional)-1]
	if len(positional) == 2 {
		dnsServer = strings.TrimSpace(positional[0])
		if dnsServer == "" {
			return "", "", 0, 0, false, fmt.Errorf("DNS server address cannot be empty")
		}
	}
	return dnsServer, target, family, count, jsonOut, nil
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

	ep, err := resolveDNSEndpoint(dnsServer, mergedAliases())
	if err != nil {
		return "", "", "", nil, fmt.Errorf("invalid DNS server: %w", err)
	}

	addrs, _, err := resolveAddrsViaEndpoint(ep, target, family)
	if err != nil {
		return "", "", "", nil, fmt.Errorf("DNS lookup failed: %w", err)
	}
	addrs = filterByFamily(addrs, family)
	if len(addrs) == 0 {
		return "", "", "", nil, errNoAddrFamily(family)
	}

	allIPs = uniqueIPStrings(addrs)
	selected := allIPs[rand.Intn(len(allIPs))]
	return selected, selected, ep.Display, allIPs, nil
}

// lookupCNAME returns the canonical name for target, or "" when target is an IP
// literal, lookup fails, or the canonical name equals target (no CNAME present).
// When dnsServer is non-empty, the same custom resolver as resolveTarget is used.
func lookupCNAME(target, dnsServer string) string {
	if net.ParseIP(target) != nil {
		return ""
	}

	if dnsServer == "" {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cname, err := net.DefaultResolver.LookupCNAME(ctx, target)
		if err != nil {
			return ""
		}
		return finalizeCNAME(cname, target)
	}

	ep, err := resolveDNSEndpoint(dnsServer, mergedAliases())
	if err != nil {
		return ""
	}
	resp, err := queryDNS(ep, target, dns.TypeCNAME)
	if err != nil || resp == nil {
		return ""
	}
	for _, rr := range resp.Answer {
		if c, ok := rr.(*dns.CNAME); ok {
			if out := finalizeCNAME(c.Target, target); out != "" {
				return out
			}
		}
	}
	return ""
}

func finalizeCNAME(cname, target string) string {
	cname = strings.TrimSuffix(cname, ".")
	if cname == "" || strings.EqualFold(cname, strings.TrimSuffix(target, ".")) {
		return ""
	}
	return cname
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

func pingCommand(host string, count int) (name string, args []string) {
	var opts []string
	if count > 0 {
		opts = []string{"-c", strconv.Itoa(count)}
	}
	ip := net.ParseIP(host)
	if ip != nil && ip.To4() == nil {
		if runtime.GOOS == "darwin" {
			return "ping6", append(opts, host)
		}
		return "ping", append([]string{"-6"}, append(opts, host)...)
	}
	return "ping", append(opts, host)
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

func executePing(host string, count int, out io.Writer) error {
	// Terminal SIGINT is delivered to the whole foreground process group. If the parent
	// uses the default handler, the Go runtime prints "signal: interrupt" after the
	// child (ping) has already handled Ctrl+C gracefully. Ignore SIGINT here so ping
	// alone receives meaningful interrupt handling and normal stats output.
	signal.Ignore(os.Interrupt)
	defer signal.Reset(os.Interrupt)

	name, args := pingCommand(host, count)
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

	streamErr := skipPingBannerLine(stdout, out)
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
