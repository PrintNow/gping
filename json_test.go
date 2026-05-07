package main

import (
	"reflect"
	"testing"

	"gping/geoip"
)

func TestParsePingOutputMacOS(t *testing.T) {
	in := `PING 1.1.1.1 (1.1.1.1): 56 data bytes
64 bytes from 1.1.1.1: icmp_seq=0 ttl=57 time=5.123 ms
64 bytes from 1.1.1.1: icmp_seq=1 ttl=57 time=5.456 ms

--- 1.1.1.1 ping statistics ---
2 packets transmitted, 2 packets received, 0.0% packet loss
round-trip min/avg/max/stddev = 5.123/5.290/5.456/0.166 ms
`
	got := parsePingOutput(in)
	if got.Transmitted != 2 || got.Received != 2 || got.LossPercent != 0.0 {
		t.Errorf("stats: %+v", got)
	}
	if got.RTT == nil || got.RTT.Min != 5.123 || got.RTT.Avg != 5.290 || got.RTT.Max != 5.456 || got.RTT.Stddev != 0.166 {
		t.Errorf("rtt: %+v", got.RTT)
	}
	if len(got.Replies) != 2 {
		t.Fatalf("replies: %d", len(got.Replies))
	}
	if got.Replies[0].Seq != 0 || got.Replies[0].TTL != 57 || got.Replies[0].TimeMS != 5.123 {
		t.Errorf("reply0: %+v", got.Replies[0])
	}
	if got.Replies[0].Hlim != 0 {
		t.Errorf("v4 hlim should be zero, got %d", got.Replies[0].Hlim)
	}
}

func TestParsePingOutputLinux(t *testing.T) {
	in := `PING 1.1.1.1 (1.1.1.1) 56(84) bytes of data.
64 bytes from 1.1.1.1: icmp_seq=1 ttl=57 time=5.12 ms
64 bytes from 1.1.1.1: icmp_seq=2 ttl=57 time=5.34 ms

--- 1.1.1.1 ping statistics ---
2 packets transmitted, 2 received, 0% packet loss, time 1004ms
rtt min/avg/max/mdev = 5.12/5.23/5.34/0.11 ms
`
	got := parsePingOutput(in)
	if got.Transmitted != 2 || got.Received != 2 || got.LossPercent != 0.0 {
		t.Errorf("stats: %+v", got)
	}
	if got.RTT == nil || got.RTT.Min != 5.12 || got.RTT.Stddev != 0.11 {
		t.Errorf("rtt: %+v", got.RTT)
	}
	if len(got.Replies) != 2 {
		t.Fatalf("replies: %d", len(got.Replies))
	}
}

func TestParsePingOutputIPv6(t *testing.T) {
	in := `16 bytes from 2606:4700::1: icmp_seq=0 hlim=58 time=12.345 ms
1 packets transmitted, 1 packets received, 0.0% packet loss
`
	got := parsePingOutput(in)
	if len(got.Replies) != 1 {
		t.Fatalf("replies: %d", len(got.Replies))
	}
	r := got.Replies[0]
	if r.Hlim != 58 || r.TTL != 0 || r.TimeMS != 12.345 {
		t.Errorf("v6 reply: %+v", r)
	}
}

func TestParsePingOutputLossy(t *testing.T) {
	in := `--- 1.1.1.1 ping statistics ---
4 packets transmitted, 0 packets received, 100.0% packet loss
`
	got := parsePingOutput(in)
	if got.Transmitted != 4 || got.Received != 0 || got.LossPercent != 100.0 {
		t.Errorf("stats: %+v", got)
	}
	if got.RTT != nil {
		t.Errorf("rtt should be nil, got %+v", got.RTT)
	}
	if len(got.Replies) != 0 {
		t.Errorf("replies should be empty: %v", got.Replies)
	}
}

func TestLocationToJSON(t *testing.T) {
	if got := locationToJSON(nil); got != nil {
		t.Errorf("nil: got %+v", got)
	}
	if got := locationToJSON(&geoip.CityInfo{}); got != nil {
		t.Errorf("all empty: got %+v", got)
	}
	in := &geoip.CityInfo{Country: "China", Province: "Guangdong", City: "Shenzhen"}
	got := locationToJSON(in)
	want := &jsonLoc{Country: "China", Province: "Guangdong", City: "Shenzhen"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v want %+v", got, want)
	}
}
