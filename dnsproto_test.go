package main

import (
	"strings"
	"testing"
)

func TestResolveDNSEndpoint(t *testing.T) {
	aliases := map[string]Endpoint{
		"cf": {Proto: protoDoH, URL: "https://cloudflare-dns.com/dns-query", Addr: "1.1.1.1:853", SNI: "cloudflare-dns.com"},
	}

	type want struct {
		proto    dnsProto
		addr     string
		url      string
		sni      string
		alias    string
		errSub   string
		dispHas  string
	}
	cases := []struct {
		name string
		in   string
		want want
	}{
		{"builtin alias cf → DoH", "cf", want{proto: protoDoH, url: "https://cloudflare-dns.com/dns-query", sni: "cloudflare-dns.com", alias: "cf", dispHas: "(cf)"}},
		{"alias case-insensitive", "CF", want{proto: protoDoH, url: "https://cloudflare-dns.com/dns-query", sni: "cloudflare-dns.com", alias: "cf"}},
		{"plain IP → UDP", "8.8.8.8", want{proto: protoUDP, addr: "8.8.8.8:53"}},
		{"IP:port → UDP", "127.0.0.1:5353", want{proto: protoUDP, addr: "127.0.0.1:5353"}},
		{"unknown short → UDP fallback", "nope", want{proto: protoUDP, addr: "nope:53"}},
		{"hostname → UDP", "dns.example.com", want{proto: protoUDP, addr: "dns.example.com:53"}},
		{"hostname:port → UDP", "dns.example.com:5353", want{proto: protoUDP, addr: "dns.example.com:5353"}},

		{"doh:// alias", "doh://cf", want{proto: protoDoH, url: "https://cloudflare-dns.com/dns-query", sni: "cloudflare-dns.com", alias: "cf"}},
		{"doh:// host adds /dns-query", "doh://dns.google", want{proto: protoDoH, url: "https://dns.google/dns-query"}},
		{"doh:// full URL", "doh://dns.google/dns-query", want{proto: protoDoH, url: "https://dns.google/dns-query"}},
		{"https://", "https://dns.quad9.net/dns-query", want{proto: protoDoH, url: "https://dns.quad9.net/dns-query"}},

		{"dot:// alias", "dot://cf", want{proto: protoDoT, addr: "1.1.1.1:853", sni: "cloudflare-dns.com", alias: "cf"}},
		{"tls:// alias", "tls://cf", want{proto: protoDoT, addr: "1.1.1.1:853", sni: "cloudflare-dns.com", alias: "cf"}},
		{"dot:// IP", "dot://1.1.1.1", want{proto: protoDoT, addr: "1.1.1.1:853", sni: ""}},
		{"dot:// host", "dot://dns.google", want{proto: protoDoT, addr: "dns.google:853", sni: "dns.google"}},
		{"dot:// host:port", "dot://dns.google:8530", want{proto: protoDoT, addr: "dns.google:8530", sni: "dns.google"}},

		{"empty", "", want{errSub: "empty"}},
		{"doh:// empty body", "doh://", want{errSub: "empty DoH"}},
		{"dot:// empty body", "dot://", want{errSub: "empty DoT"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ep, err := resolveDNSEndpoint(c.in, aliases)
			if c.want.errSub != "" {
				if err == nil || !strings.Contains(err.Error(), c.want.errSub) {
					t.Fatalf("err=%v want substring %q", err, c.want.errSub)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if ep.Proto != c.want.proto {
				t.Errorf("proto: got %v want %v", ep.Proto, c.want.proto)
			}
			if c.want.addr != "" && ep.Addr != c.want.addr {
				t.Errorf("addr: got %q want %q", ep.Addr, c.want.addr)
			}
			if c.want.url != "" && ep.URL != c.want.url {
				t.Errorf("url: got %q want %q", ep.URL, c.want.url)
			}
			if ep.SNI != c.want.sni {
				t.Errorf("sni: got %q want %q", ep.SNI, c.want.sni)
			}
			if c.want.alias != "" && ep.AliasKey != c.want.alias {
				t.Errorf("alias: got %q want %q", ep.AliasKey, c.want.alias)
			}
			if c.want.dispHas != "" && !strings.Contains(ep.Display, c.want.dispHas) {
				t.Errorf("display %q missing %q", ep.Display, c.want.dispHas)
			}
			if ep.Display == "" {
				t.Errorf("display empty for %q", c.in)
			}
		})
	}
}

func TestDisplayFor(t *testing.T) {
	cases := []struct {
		name string
		ep   Endpoint
		want string
	}{
		{"udp", Endpoint{Proto: protoUDP, Addr: "1.1.1.1:53"}, "1.1.1.1:53"},
		{"doh + alias", Endpoint{Proto: protoDoH, URL: "https://x/dns-query", AliasKey: "cf"}, "doh https://x/dns-query  (cf)"},
		{"dot + sni", Endpoint{Proto: protoDoT, Addr: "1.1.1.1:853", SNI: "cf"}, "dot 1.1.1.1:853 sni=cf"},
		{"dot no sni", Endpoint{Proto: protoDoT, Addr: "1.1.1.1:853"}, "dot 1.1.1.1:853"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := displayFor(c.ep); got != c.want {
				t.Errorf("got %q want %q", got, c.want)
			}
		})
	}
}
