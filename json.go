package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"gping/geoip"
)

type jsonOutput struct {
	Target     string       `json:"target"`
	ResolvedIP string       `json:"resolved_ip"`
	IPs        []string     `json:"ips"`
	CNAME      string       `json:"cname,omitempty"`
	DNSServer  string       `json:"dns_server,omitempty"`
	Location   *jsonLoc     `json:"location,omitempty"`
	Ping       jsonPingData `json:"ping"`
}

type jsonLoc struct {
	Country  string `json:"country,omitempty"`
	Province string `json:"province,omitempty"`
	City     string `json:"city,omitempty"`
}

type jsonPingData struct {
	Transmitted int         `json:"transmitted"`
	Received    int         `json:"received"`
	LossPercent float64     `json:"loss_percent"`
	RTT         *jsonRTT    `json:"rtt_ms,omitempty"`
	Replies     []jsonReply `json:"replies"`
}

type jsonRTT struct {
	Min    float64 `json:"min"`
	Avg    float64 `json:"avg"`
	Max    float64 `json:"max"`
	Stddev float64 `json:"stddev"`
}

type jsonReply struct {
	Seq    int     `json:"seq"`
	TTL    int     `json:"ttl,omitempty"`
	Hlim   int     `json:"hlim,omitempty"`
	TimeMS float64 `json:"time_ms"`
}

func runJSON(target, targetIP, targetHost, dnsUsed, cname string, allIPs []string, city *geoip.CityInfo, count int) error {
	out := jsonOutput{
		Target:     target,
		ResolvedIP: targetIP,
		IPs:        allIPs,
		CNAME:      cname,
		DNSServer:  dnsUsed,
		Location:   locationToJSON(city),
	}

	var buf bytes.Buffer
	pingErr := executePing(targetHost, count, &buf)
	// Treat normal non-zero exits (e.g. 100% loss) as data, not error.
	var exitErr *exec.ExitError
	if pingErr != nil && !errors.As(pingErr, &exitErr) {
		return pingErr
	}

	out.Ping = parsePingOutput(buf.String())

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func locationToJSON(c *geoip.CityInfo) *jsonLoc {
	if c == nil {
		return nil
	}
	if c.Country == "" && c.Province == "" && c.City == "" {
		return nil
	}
	return &jsonLoc{Country: c.Country, Province: c.Province, City: c.City}
}

var (
	// macOS:        64 bytes from 1.1.1.1: icmp_seq=0 ttl=57 time=5.123 ms
	// Linux:        64 bytes from 1.1.1.1: icmp_seq=1 ttl=57 time=5.12 ms
	// IPv6 macOS:   16 bytes from ...: icmp_seq=0 hlim=58 time=12.345 ms
	reReply = regexp.MustCompile(`icmp_seq=(\d+).*?(ttl|hlim)=(\d+).*?time=([\d.]+)\s*ms`)

	// macOS: "4 packets transmitted, 4 packets received, 0.0% packet loss"
	// Linux: "4 packets transmitted, 4 received, 0% packet loss, time 3004ms"
	reStats = regexp.MustCompile(`(\d+)\s+packets transmitted,\s*(\d+)(?:\s+packets)?\s+received[^,]*,\s*([\d.]+)%\s*packet loss`)

	// macOS: "round-trip min/avg/max/stddev = 45.0/45.5/46.1/0.4 ms"
	// Linux: "rtt min/avg/max/mdev = 45.0/45.5/46.1/0.4 ms"
	reRTT = regexp.MustCompile(`min/avg/max/(?:stddev|mdev)\s*=\s*([\d.]+)/([\d.]+)/([\d.]+)/([\d.]+)\s*ms`)
)

func parsePingOutput(s string) jsonPingData {
	data := jsonPingData{Replies: []jsonReply{}}
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if m := reReply.FindStringSubmatch(line); m != nil {
			seq, _ := strconv.Atoi(m[1])
			hop, _ := strconv.Atoi(m[3])
			t, _ := strconv.ParseFloat(m[4], 64)
			r := jsonReply{Seq: seq, TimeMS: t}
			if m[2] == "ttl" {
				r.TTL = hop
			} else {
				r.Hlim = hop
			}
			data.Replies = append(data.Replies, r)
			continue
		}
		if m := reStats.FindStringSubmatch(line); m != nil {
			data.Transmitted, _ = strconv.Atoi(m[1])
			data.Received, _ = strconv.Atoi(m[2])
			data.LossPercent, _ = strconv.ParseFloat(m[3], 64)
			continue
		}
		if m := reRTT.FindStringSubmatch(line); m != nil {
			min, _ := strconv.ParseFloat(m[1], 64)
			avg, _ := strconv.ParseFloat(m[2], 64)
			max, _ := strconv.ParseFloat(m[3], 64)
			std, _ := strconv.ParseFloat(m[4], 64)
			data.RTT = &jsonRTT{Min: min, Avg: avg, Max: max, Stddev: std}
		}
	}
	return data
}
