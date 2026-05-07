package main

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
)

// builtinDNS is the curated set of well-known public DNS providers reachable
// via DoH (default for short alias) and, when an Addr is set, via DoT through
// `dot://<alias>`.
var builtinDNS = map[string]Endpoint{
	"cf":         {Proto: protoDoH, URL: "https://cloudflare-dns.com/dns-query", Addr: "1.1.1.1:853", SNI: "cloudflare-dns.com"},
	"cloudflare": {Proto: protoDoH, URL: "https://cloudflare-dns.com/dns-query", Addr: "1.1.1.1:853", SNI: "cloudflare-dns.com"},
	"google":     {Proto: protoDoH, URL: "https://dns.google/dns-query", Addr: "8.8.8.8:853", SNI: "dns.google"},
	"g":          {Proto: protoDoH, URL: "https://dns.google/dns-query", Addr: "8.8.8.8:853", SNI: "dns.google"},
	"quad9":      {Proto: protoDoH, URL: "https://dns.quad9.net/dns-query", Addr: "9.9.9.9:853", SNI: "dns.quad9.net"},
	"adguard":    {Proto: protoDoH, URL: "https://dns.adguard-dns.com/dns-query", Addr: "94.140.14.14:853", SNI: "dns.adguard-dns.com"},
	"ali":        {Proto: protoDoH, URL: "https://dns.alidns.com/dns-query", Addr: "223.5.5.5:853", SNI: "dns.alidns.com"},
	"aliyun":     {Proto: protoDoH, URL: "https://dns.alidns.com/dns-query", Addr: "223.5.5.5:853", SNI: "dns.alidns.com"},
	"dnspod":     {Proto: protoDoH, URL: "https://doh.pub/dns-query", Addr: "1.12.12.12:853", SNI: "dot.pub"},
	"tx":         {Proto: protoDoH, URL: "https://doh.pub/dns-query", Addr: "1.12.12.12:853", SNI: "dot.pub"},
	"360":        {Proto: protoDoH, URL: "https://doh.360.cn/dns-query"},
}

type userAliasFile struct {
	// keys = alias names, value = entry
	Entries map[string]userAliasEntry `toml:"-"`
}

type userAliasEntry struct {
	Type string `toml:"type"` // udp / dot / doh
	Addr string `toml:"addr"` // host or host:port (udp/dot)
	URL  string `toml:"url"`  // doh URL
	SNI  string `toml:"sni"`  // optional, dot
}

// loadUserAliases reads ~/.config/gping/dns.toml. Missing file → empty map, no error.
// Parse errors → empty map + warning to stderr (still permits builtin lookups).
func loadUserAliases() map[string]Endpoint {
	out := map[string]Endpoint{}

	path := userAliasPath()
	if path == "" {
		return out
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "Warning: cannot read %s: %v\n", path, err)
		}
		return out
	}

	var raw map[string]userAliasEntry
	if _, err := toml.Decode(string(data), &raw); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: parse %s: %v\n", path, err)
		return out
	}

	for name, e := range raw {
		ep, err := userEntryToEndpoint(e)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: alias %q in %s: %v\n", name, path, err)
			continue
		}
		out[strings.ToLower(name)] = ep
	}
	return out
}

func userEntryToEndpoint(e userAliasEntry) (Endpoint, error) {
	switch strings.ToLower(strings.TrimSpace(e.Type)) {
	case "doh":
		if e.URL == "" {
			return Endpoint{}, fmt.Errorf("doh entry requires url")
		}
		return Endpoint{Proto: protoDoH, URL: e.URL}, nil
	case "dot":
		if e.Addr == "" {
			return Endpoint{}, fmt.Errorf("dot entry requires addr")
		}
		addr := e.Addr
		if _, _, err := net.SplitHostPort(addr); err != nil {
			addr = net.JoinHostPort(addr, "853")
		}
		host, _, _ := net.SplitHostPort(addr)
		sni := e.SNI
		if sni == "" && net.ParseIP(host) == nil {
			sni = host
		}
		return Endpoint{Proto: protoDoT, Addr: addr, SNI: sni}, nil
	case "udp", "":
		if e.Addr == "" {
			return Endpoint{}, fmt.Errorf("udp entry requires addr")
		}
		addr, err := normalizeDNSAddr(e.Addr)
		if err != nil {
			return Endpoint{}, err
		}
		return Endpoint{Proto: protoUDP, Addr: addr}, nil
	default:
		return Endpoint{}, fmt.Errorf("unknown type %q (want udp / dot / doh)", e.Type)
	}
}

func userAliasPath() string {
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "gping", "dns.toml")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".config", "gping", "dns.toml")
}

var (
	aliasOnce  sync.Once
	aliasCache map[string]Endpoint
)

func mergedAliases() map[string]Endpoint {
	aliasOnce.Do(func() {
		aliasCache = make(map[string]Endpoint, len(builtinDNS))
		for k, v := range builtinDNS {
			aliasCache[k] = v
		}
		for k, v := range loadUserAliases() {
			aliasCache[k] = v
		}
	})
	return aliasCache
}
