package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUserEntryToEndpoint(t *testing.T) {
	cases := []struct {
		name   string
		entry  userAliasEntry
		want   Endpoint
		errSub string
	}{
		{"doh ok", userAliasEntry{Type: "doh", URL: "https://x/dns-query"}, Endpoint{Proto: protoDoH, URL: "https://x/dns-query"}, ""},
		{"doh missing url", userAliasEntry{Type: "doh"}, Endpoint{}, "doh entry requires url"},
		{"dot host", userAliasEntry{Type: "dot", Addr: "ns.example.com"}, Endpoint{Proto: protoDoT, Addr: "ns.example.com:853", SNI: "ns.example.com"}, ""},
		{"dot host:port + sni", userAliasEntry{Type: "dot", Addr: "ns.example.com:8530", SNI: "real.tld"}, Endpoint{Proto: protoDoT, Addr: "ns.example.com:8530", SNI: "real.tld"}, ""},
		{"dot raw IP no sni", userAliasEntry{Type: "dot", Addr: "1.1.1.1"}, Endpoint{Proto: protoDoT, Addr: "1.1.1.1:853", SNI: ""}, ""},
		{"dot missing addr", userAliasEntry{Type: "dot"}, Endpoint{}, "dot entry requires addr"},
		{"udp", userAliasEntry{Type: "udp", Addr: "10.0.0.1"}, Endpoint{Proto: protoUDP, Addr: "10.0.0.1:53"}, ""},
		{"empty type defaults udp", userAliasEntry{Addr: "10.0.0.1:5353"}, Endpoint{Proto: protoUDP, Addr: "10.0.0.1:5353"}, ""},
		{"udp missing addr", userAliasEntry{Type: "udp"}, Endpoint{}, "udp entry requires addr"},
		{"unknown type", userAliasEntry{Type: "https"}, Endpoint{}, "unknown type"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := userEntryToEndpoint(c.entry)
			if c.errSub != "" {
				if err == nil || !strings.Contains(err.Error(), c.errSub) {
					t.Fatalf("err=%v want substring %q", err, c.errSub)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got.Proto != c.want.Proto || got.Addr != c.want.Addr || got.URL != c.want.URL || got.SNI != c.want.SNI {
				t.Errorf("got %+v want %+v", got, c.want)
			}
		})
	}
}

func TestLoadUserAliasesMissing(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp) // no file inside
	got := loadUserAliases()
	if len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}

func TestLoadUserAliasesValid(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	dir := filepath.Join(tmp, "gping")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := `
[corp]
type = "doh"
url  = "https://dns.corp.local/dns-query"

[home]
type = "dot"
addr = "192.168.1.1:853"
sni  = "router.local"

[fast53]
type = "udp"
addr = "10.0.0.1"
`
	if err := os.WriteFile(filepath.Join(dir, "dns.toml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	got := loadUserAliases()
	if len(got) != 3 {
		t.Fatalf("expected 3 aliases, got %d: %v", len(got), got)
	}
	if got["corp"].Proto != protoDoH || got["corp"].URL != "https://dns.corp.local/dns-query" {
		t.Errorf("corp: %+v", got["corp"])
	}
	if got["home"].Proto != protoDoT || got["home"].Addr != "192.168.1.1:853" || got["home"].SNI != "router.local" {
		t.Errorf("home: %+v", got["home"])
	}
	if got["fast53"].Proto != protoUDP || got["fast53"].Addr != "10.0.0.1:53" {
		t.Errorf("fast53: %+v", got["fast53"])
	}
}

func TestLoadUserAliasesBadTOML(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	dir := filepath.Join(tmp, "gping")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "dns.toml"), []byte("garbage\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Redirect stderr to suppress warning noise from test output.
	oldErr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	got := loadUserAliases()
	w.Close()
	os.Stderr = oldErr
	_ = r // discard

	if len(got) != 0 {
		t.Errorf("expected empty map on bad toml, got %v", got)
	}
}

func TestLoadUserAliasesBadEntry(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	dir := filepath.Join(tmp, "gping")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Mix one valid + one invalid entry; expect only the valid one.
	content := `
[good]
type = "doh"
url  = "https://x/dns-query"

[bad]
type = "doh"
`
	if err := os.WriteFile(filepath.Join(dir, "dns.toml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	oldErr := os.Stderr
	_, w, _ := os.Pipe()
	os.Stderr = w
	got := loadUserAliases()
	w.Close()
	os.Stderr = oldErr

	if _, ok := got["good"]; !ok {
		t.Errorf("good missing: %v", got)
	}
	if _, ok := got["bad"]; ok {
		t.Errorf("bad should be skipped: %v", got)
	}
}

func TestBuiltinAliasesShape(t *testing.T) {
	// Sanity: every DoH alias has URL; entries with Addr also have SNI when host is non-IP.
	for name, ep := range builtinDNS {
		if ep.Proto != protoDoH {
			t.Errorf("builtin %q has non-DoH proto", name)
		}
		if ep.URL == "" {
			t.Errorf("builtin %q missing URL", name)
		}
	}
}
