package main

import (
	"bytes"
	"errors"
	"net"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"gping/geoip"
)

func TestParseArgs(t *testing.T) {
	type want struct {
		dns    string
		target string
		fam    ipFamily
		count  int
		json   bool
		errIs  error  // sentinel match
		errSub string // substring match
	}
	cases := []struct {
		name string
		argv []string
		want want
	}{
		{"single host", []string{"ipxy.cc"}, want{target: "ipxy.cc"}},
		{"dns + host", []string{"1.1.1.1", "ipxy.cc"}, want{dns: "1.1.1.1", target: "ipxy.cc"}},
		{"flags interleaved", []string{"-4", "1.1.1.1", "-c", "5", "ipxy.cc"}, want{dns: "1.1.1.1", target: "ipxy.cc", fam: ipFamily4, count: 5}},
		{"-6", []string{"-6", "ipxy.cc"}, want{target: "ipxy.cc", fam: ipFamily6}},
		{"json", []string{"-json", "ipxy.cc"}, want{target: "ipxy.cc", json: true}},
		{"--json", []string{"--json", "ipxy.cc"}, want{target: "ipxy.cc", json: true}},
		{"alias dns", []string{"cf", "ipxy.cc"}, want{dns: "cf", target: "ipxy.cc"}},
		{"doh url", []string{"doh://dns.google/dns-query", "ipxy.cc"}, want{dns: "doh://dns.google/dns-query", target: "ipxy.cc"}},

		// errors
		{"no positional", []string{"-4"}, want{errIs: errInvalidArgs}},
		{"too many positional", []string{"a", "b", "c"}, want{errIs: errInvalidArgs}},
		{"-4 and -6", []string{"-4", "-6", "ipxy.cc"}, want{errSub: "mutually exclusive"}},
		{"duplicate -4", []string{"-4", "-4", "ipxy.cc"}, want{errSub: "duplicate -4"}},
		{"-c missing", []string{"-c"}, want{errSub: "-c requires"}},
		{"-c bad", []string{"-c", "0", "x"}, want{errSub: "positive integer"}},
		{"-c twice", []string{"-c", "3", "-c", "4", "x"}, want{errSub: "duplicate -c"}},
		{"unknown opt", []string{"--nope", "x"}, want{errSub: "unknown option"}},
		{"empty dns", []string{"", "x"}, want{errSub: "DNS server address cannot be empty"}},
		{"duplicate -json", []string{"-json", "--json", "x"}, want{errSub: "duplicate -json"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dns, target, fam, count, jsonOut, err := parseArgs(c.argv)
			if c.want.errIs != nil {
				if !errors.Is(err, c.want.errIs) {
					t.Fatalf("err=%v, want errors.Is(%v)", err, c.want.errIs)
				}
				return
			}
			if c.want.errSub != "" {
				if err == nil || !strings.Contains(err.Error(), c.want.errSub) {
					t.Fatalf("err=%v, want substring %q", err, c.want.errSub)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if dns != c.want.dns || target != c.want.target || fam != c.want.fam || count != c.want.count || jsonOut != c.want.json {
				t.Fatalf("got (%q,%q,%v,%d,%v) want (%q,%q,%v,%d,%v)",
					dns, target, fam, count, jsonOut,
					c.want.dns, c.want.target, c.want.fam, c.want.count, c.want.json)
			}
		})
	}
}

func TestNormalizeDNSAddr(t *testing.T) {
	cases := []struct {
		in, out string
		err     bool
	}{
		{"1.1.1.1", "1.1.1.1:53", false},
		{"1.1.1.1:5353", "1.1.1.1:5353", false},
		{"dns.example.com", "dns.example.com:53", false},
		{"dns.example.com:853", "dns.example.com:853", false},
		{"::1", "[::1]:53", false},
		{"[::1]:53", "[::1]:53", false},
		{"", "", true},
		{"   ", "", true},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got, err := normalizeDNSAddr(c.in)
			if c.err {
				if err == nil {
					t.Fatalf("expected error, got %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got != c.out {
				t.Fatalf("got %q, want %q", got, c.out)
			}
		})
	}
}

func TestFilterByFamily(t *testing.T) {
	v4 := net.ParseIP("1.2.3.4")
	v6 := net.ParseIP("2606:4700::1")
	all := []net.IP{v4, v6}

	if got := filterByFamily(all, ipFamilyAny); !reflect.DeepEqual(got, all) {
		t.Errorf("any: got %v want %v", got, all)
	}
	if got := filterByFamily(all, ipFamily4); len(got) != 1 || !got[0].Equal(v4) {
		t.Errorf("v4: got %v", got)
	}
	if got := filterByFamily(all, ipFamily6); len(got) != 1 || !got[0].Equal(v6) {
		t.Errorf("v6: got %v", got)
	}
	if got := filterByFamily([]net.IP{v6}, ipFamily4); len(got) != 0 {
		t.Errorf("v4 empty: got %v", got)
	}
}

func TestUniqueIPStrings(t *testing.T) {
	ips := []net.IP{
		net.ParseIP("1.1.1.1"),
		net.ParseIP("2.2.2.2"),
		net.ParseIP("1.1.1.1"),
		net.ParseIP("3.3.3.3"),
	}
	want := []string{"1.1.1.1", "2.2.2.2", "3.3.3.3"}
	got := uniqueIPStrings(ips)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestFormatPingIPList(t *testing.T) {
	got := formatPingIPList([]string{"1.1.1.1", "1.0.0.1", "2.2.2.2"}, "1.0.0.1")
	want := "*1.0.0.1, 1.1.1.1, 2.2.2.2"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
	// single IP
	if got := formatPingIPList([]string{"1.1.1.1"}, "1.1.1.1"); got != "*1.1.1.1" {
		t.Errorf("single: got %q", got)
	}
}

func TestFormatLocation(t *testing.T) {
	cases := []struct {
		name string
		in   *geoip.CityInfo
		want string
	}{
		{"nil", nil, "Unknown"},
		{"all empty", &geoip.CityInfo{}, "Unknown"},
		{"full", &geoip.CityInfo{Country: "China", Province: "Guangdong", City: "Shenzhen"}, "China, Guangdong, Shenzhen"},
		{"country=province dedup", &geoip.CityInfo{Country: "Singapore", Province: "Singapore", City: "Singapore"}, "Singapore"},
		{"missing province", &geoip.CityInfo{Country: "United States", City: "San Jose"}, "United States, San Jose"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := formatLocation(c.in); got != c.want {
				t.Errorf("got %q want %q", got, c.want)
			}
		})
	}
}

func TestPingCommand(t *testing.T) {
	// IPv4
	name, args := pingCommand("1.1.1.1", 3)
	if name != "ping" {
		t.Errorf("v4 name: got %q", name)
	}
	if !reflect.DeepEqual(args, []string{"-c", "3", "1.1.1.1"}) {
		t.Errorf("v4 args: %v", args)
	}

	// IPv4 count=0 (no -c)
	name, args = pingCommand("1.1.1.1", 0)
	if !reflect.DeepEqual(args, []string{"1.1.1.1"}) {
		t.Errorf("v4 no-count: %v", args)
	}

	// IPv6 platform-specific
	name, args = pingCommand("::1", 2)
	if runtime.GOOS == "darwin" {
		if name != "ping6" {
			t.Errorf("darwin v6 name: got %q", name)
		}
		if !reflect.DeepEqual(args, []string{"-c", "2", "::1"}) {
			t.Errorf("darwin v6 args: %v", args)
		}
	} else {
		if name != "ping" {
			t.Errorf("linux v6 name: got %q", name)
		}
		if !reflect.DeepEqual(args, []string{"-6", "-c", "2", "::1"}) {
			t.Errorf("linux v6 args: %v", args)
		}
	}

	// hostname (no IP parse) → ping default
	name, _ = pingCommand("example.com", 1)
	if name != "ping" {
		t.Errorf("host name: got %q", name)
	}
}

func TestSkipPingBannerLine(t *testing.T) {
	in := "PING 1.1.1.1 (1.1.1.1): 56 data bytes\n64 bytes from 1.1.1.1: time=1 ms\n--- stats ---\n"
	var buf bytes.Buffer
	if err := skipPingBannerLine(strings.NewReader(in), &buf); err != nil {
		t.Fatal(err)
	}
	want := "64 bytes from 1.1.1.1: time=1 ms\n--- stats ---\n"
	if buf.String() != want {
		t.Errorf("got %q want %q", buf.String(), want)
	}

	// non-banner first line → keep all
	in2 := "ping: cannot resolve foo: Unknown host\n"
	buf.Reset()
	if err := skipPingBannerLine(strings.NewReader(in2), &buf); err != nil {
		t.Fatal(err)
	}
	if buf.String() != in2 {
		t.Errorf("non-banner: got %q want %q", buf.String(), in2)
	}
}

func TestFinalizeCNAME(t *testing.T) {
	if got := finalizeCNAME("youtube-ui.l.google.com.", "www.youtube.com"); got != "youtube-ui.l.google.com" {
		t.Errorf("strip dot: got %q", got)
	}
	if got := finalizeCNAME("www.youtube.com.", "www.youtube.com"); got != "" {
		t.Errorf("equal: got %q want \"\"", got)
	}
	if got := finalizeCNAME("WWW.YouTube.com", "www.youtube.com"); got != "" {
		t.Errorf("case-insensitive equal: got %q", got)
	}
	if got := finalizeCNAME("", "x"); got != "" {
		t.Errorf("empty: got %q", got)
	}
}
