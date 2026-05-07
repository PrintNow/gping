package main

import (
	"bytes"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/miekg/dns"
)

type dnsProto int

const (
	protoUDP dnsProto = iota
	protoDoT
	protoDoH
)

// Endpoint is a fully-resolved DNS server target. Construct via resolveDNSEndpoint.
type Endpoint struct {
	Proto    dnsProto
	Addr     string // host:port for UDP/DoT
	URL      string // for DoH
	SNI      string // for DoT (defaults to Addr's host)
	AliasKey string // non-empty when source was an alias name
	Display  string // shown after "DNS Server: "
}

func (e Endpoint) IsZero() bool {
	return e.Proto == protoUDP && e.Addr == "" && e.URL == ""
}

// resolveDNSEndpoint turns a CLI string into an Endpoint, consulting the alias
// table when the input is a bare short name.
func resolveDNSEndpoint(raw string, aliases map[string]Endpoint) (Endpoint, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Endpoint{}, errors.New("empty DNS endpoint")
	}

	lower := strings.ToLower(raw)
	switch {
	case strings.HasPrefix(lower, "doh://"), strings.HasPrefix(lower, "https://"):
		body := raw
		if strings.HasPrefix(lower, "doh://") {
			body = raw[len("doh://"):]
		}
		return buildDoH(body, aliases)
	case strings.HasPrefix(lower, "dot://"), strings.HasPrefix(lower, "tls://"):
		body := raw[6:]
		return buildDoT(body, aliases)
	}

	// Bare alias?
	if !strings.Contains(raw, "://") && !strings.Contains(raw, "/") {
		// Reject IPs and host:port from alias lookup so they fall through to UDP path.
		if net.ParseIP(raw) == nil {
			if _, _, err := net.SplitHostPort(raw); err != nil {
				if ep, ok := aliases[strings.ToLower(raw)]; ok {
					ep.AliasKey = strings.ToLower(raw)
					ep.Display = displayFor(ep)
					return ep, nil
				}
			}
		}
	}

	// Fall back to UDP/53 (existing behavior). normalizeDNSAddr handles port default.
	addr, err := normalizeDNSAddr(raw)
	if err != nil {
		return Endpoint{}, err
	}
	return Endpoint{Proto: protoUDP, Addr: addr, Display: addr}, nil
}

func buildDoH(body string, aliases map[string]Endpoint) (Endpoint, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return Endpoint{}, errors.New("empty DoH endpoint")
	}
	// Alias inside doh://?
	if !strings.Contains(body, "/") && !strings.Contains(body, ":") {
		if ep, ok := aliases[strings.ToLower(body)]; ok && ep.URL != "" {
			ep.AliasKey = strings.ToLower(body)
			ep.Proto = protoDoH
			ep.Display = displayFor(ep)
			return ep, nil
		}
	}
	full := body
	if !strings.HasPrefix(strings.ToLower(full), "https://") {
		full = "https://" + full
	}
	if !strings.Contains(strings.TrimPrefix(full, "https://"), "/") {
		full += "/dns-query"
	}
	if _, err := url.Parse(full); err != nil {
		return Endpoint{}, fmt.Errorf("invalid DoH URL %q: %w", body, err)
	}
	ep := Endpoint{Proto: protoDoH, URL: full}
	ep.Display = displayFor(ep)
	return ep, nil
}

func buildDoT(body string, aliases map[string]Endpoint) (Endpoint, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return Endpoint{}, errors.New("empty DoT endpoint")
	}
	if !strings.Contains(body, ":") && net.ParseIP(body) == nil {
		if ep, ok := aliases[strings.ToLower(body)]; ok && ep.Addr != "" {
			ep.AliasKey = strings.ToLower(body)
			ep.Proto = protoDoT
			ep.Display = displayFor(ep)
			return ep, nil
		}
		// Treat as plain host, default :853, SNI = host.
		return doTFromHost(body, body), nil
	}
	host, port, err := net.SplitHostPort(body)
	if err != nil {
		// No port given.
		return doTFromHost(body, body), nil
	}
	ep := Endpoint{Proto: protoDoT, Addr: net.JoinHostPort(host, port), SNI: host}
	if net.ParseIP(host) != nil {
		ep.SNI = "" // caller may not want SNI for raw IP; let TLS pick servername blank
	}
	ep.Display = displayFor(ep)
	return ep, nil
}

func doTFromHost(host, sni string) Endpoint {
	ep := Endpoint{Proto: protoDoT, Addr: net.JoinHostPort(host, "853"), SNI: sni}
	if net.ParseIP(host) != nil {
		ep.SNI = ""
	}
	ep.Display = displayFor(ep)
	return ep
}

func displayFor(ep Endpoint) string {
	var s string
	switch ep.Proto {
	case protoDoH:
		s = "doh " + ep.URL
	case protoDoT:
		s = "dot " + ep.Addr
		if ep.SNI != "" {
			s += " sni=" + ep.SNI
		}
	default:
		s = ep.Addr
	}
	if ep.AliasKey != "" {
		s += "  (" + ep.AliasKey + ")"
	}
	return s
}

// queryDNS dispatches a DNS query to the right transport.
func queryDNS(ep Endpoint, name string, qtype uint16) (*dns.Msg, error) {
	q := &dns.Msg{}
	q.SetQuestion(dns.Fqdn(name), qtype)
	q.RecursionDesired = true

	switch ep.Proto {
	case protoDoH:
		return queryDoH(ep, q)
	case protoDoT:
		return queryDoT(ep, q)
	default:
		return queryUDP(ep, q)
	}
}

func queryUDP(ep Endpoint, q *dns.Msg) (*dns.Msg, error) {
	c := &dns.Client{Net: "udp", Timeout: 5 * time.Second}
	resp, _, err := c.Exchange(q, ep.Addr)
	if err != nil {
		return nil, err
	}
	if resp != nil && resp.Truncated {
		c.Net = "tcp"
		resp, _, err = c.Exchange(q, ep.Addr)
		if err != nil {
			return nil, err
		}
	}
	return resp, nil
}

func queryDoT(ep Endpoint, q *dns.Msg) (*dns.Msg, error) {
	tlsCfg := &tls.Config{ServerName: ep.SNI}
	c := &dns.Client{Net: "tcp-tls", TLSConfig: tlsCfg, Timeout: 5 * time.Second}
	resp, _, err := c.Exchange(q, ep.Addr)
	return resp, err
}

var dohClient = &http.Client{Timeout: 8 * time.Second}

func queryDoH(ep Endpoint, q *dns.Msg) (*dns.Msg, error) {
	q.Id = 0 // RFC 8484 recommends 0 for caching
	wire, err := q.Pack()
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest("POST", ep.URL, bytes.NewReader(wire))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/dns-message")
	req.Header.Set("Accept", "application/dns-message")
	resp, err := dohClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("DoH HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	out := &dns.Msg{}
	if err := out.Unpack(body); err != nil {
		return nil, fmt.Errorf("DoH unpack: %w", err)
	}
	return out, nil
}

// resolveAddrsViaEndpoint queries A and/or AAAA records via the given endpoint.
// Returns the deduplicated IP list filtered by family, plus the canonical name
// observed in any of the responses (empty if none / equals target).
func resolveAddrsViaEndpoint(ep Endpoint, name string, family ipFamily) ([]net.IP, string, error) {
	var qtypes []uint16
	switch family {
	case ipFamily4:
		qtypes = []uint16{dns.TypeA}
	case ipFamily6:
		qtypes = []uint16{dns.TypeAAAA}
	default:
		qtypes = []uint16{dns.TypeA, dns.TypeAAAA}
	}

	var (
		ips     []net.IP
		cname   string
		lastErr error
		anyOK   bool
	)
	for _, qt := range qtypes {
		resp, err := queryDNS(ep, name, qt)
		if err != nil {
			lastErr = err
			continue
		}
		anyOK = true
		for _, rr := range resp.Answer {
			switch v := rr.(type) {
			case *dns.A:
				ips = append(ips, v.A)
			case *dns.AAAA:
				ips = append(ips, v.AAAA)
			case *dns.CNAME:
				if cname == "" {
					cname = strings.TrimSuffix(v.Target, ".")
				}
			}
		}
	}
	if !anyOK {
		return nil, "", lastErr
	}
	return ips, cname, nil
}
