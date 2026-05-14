<p align="center"><strong>English</strong> · <a href="README.zh-CN.md">简体中文</a></p>

# gping

> **g**eo + **ping** — An enhanced ping tool that displays the geolocation of the target IP before pinging. Supports DoT / DoH custom DNS.

```
$ gping 1.1.1.1
Los Angeles, United States

PING 1.1.1.1 (1.1.1.1): 56 data bytes
64 bytes from 1.1.1.1: icmp_seq=0 ttl=57 time=5.123 ms
64 bytes from 1.1.1.1: icmp_seq=1 ttl=57 time=5.234 ms
...
```

## Quick Start

### Install

Download from [GitHub Releases](../../releases), extract and place in your PATH:

```bash
# macOS (Apple Silicon)
curl -LO "https://github.com/PrintNow/gping/releases/download/v1.2.0/gping-darwin-arm64-v1.2.0.tar.gz"
tar xzf "gping-darwin-arm64-v1.2.0.tar.gz"
rm "gping-darwin-arm64-v1.2.0.tar.gz"
mkdir -p ~/.local/bin
mv gping ~/.local/bin/

# Linux (x86_64)
curl -LO "https://github.com/PrintNow/gping/releases/download/v1.2.0/gping-linux-amd64-v1.2.0.tar.gz"
tar xzf "gping-linux-amd64-v1.2.0.tar.gz"
rm "gping-linux-amd64-v1.2.0.tar.gz"
mkdir -p ~/.local/bin
mv gping ~/.local/bin/
```

> **macOS**: `~/.local/bin` may not be in your PATH. Add it to `~/.zshrc`:
> ```bash
> echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.zshrc
> source ~/.zshrc
> ```
> **Linux**: `~/.local/bin` is in PATH by default on most desktop distros (Ubuntu, Fedora, etc.).

#### Tiny Variant (~8MB, no embedded database)

A smaller binary without the embedded GeoLite2 database. You must provide the MMDB file manually (see [Notes](#notes) for where to place it).

```bash
# macOS (Apple Silicon)
curl -LO "https://github.com/PrintNow/gping/releases/download/v1.2.0/gping-tiny-darwin-arm64-v1.2.0.tar.gz"
tar xzf "gping-tiny-darwin-arm64-v1.2.0.tar.gz"
rm "gping-tiny-darwin-arm64-v1.2.0.tar.gz"
mkdir -p ~/.local/bin
mv gping ~/.local/bin/
```

### Build from Source

Requires Go 1.25+.

```bash
# Full build (embeds GeoLite2-City.mmdb, ~70MB binary)
# Download the database (no registration required):
make download-geolite
make build

# Or manually download from MaxMind (requires account):
# https://www.maxmind.com/en/geolite2/signup
# Place GeoLite2-City.mmdb in the data/ directory

# Tiny build (~8MB binary, requires external MMDB at runtime)
make build-tiny
```

## Usage

```bash
# Basic
gping 1.1.1.1            # ping an IP
gping ipxy.cc             # ping a domain (picks a random IP if multiple)

# Custom DNS
gping 127.0.0.1:5353 ipxy.cc                # custom port
gping cf ipxy.cc                            # built-in alias → DoH (Cloudflare)
gping ali www.youtube.com                   # built-in alias → DoH (Alibaba)
gping doh://dns.google/dns-query baidu.com  # full DoH URL
gping dot://cf gping.dev                    # DoT (Cloudflare)
gping dot://192.168.1.1 internal-svc        # DoT for internal network

# Pass-through ping flags
gping -c 5 1.1.1.1
```

### Built-in DNS Aliases

| Alias | Service |
|-------|---------|
| `cf` / `cloudflare` | Cloudflare DoH |
| `google` / `g` | Google DoH |
| `quad9` | Quad9 DoH |
| `adguard` | AdGuard DoH |
| `ali` / `aliyun` | Alibaba DoH |
| `dnspod` / `tx` | DNSPod DoH |
| `360` | 360 DoH |

Short aliases default to DoH. Use the `dot://` prefix explicitly for DoT.

### Custom Aliases

Create `~/.config/gping/dns.toml` (or `$XDG_CONFIG_HOME/gping/dns.toml`):

```toml
[corp]
type = "doh"
url  = "https://dns.corp.local/dns-query"

[home]
type = "dot"
addr = "192.168.1.1:853"
sni  = "router.local"

[fast53]
type = "udp"
addr = "10.0.0.1:53"
```

Then use `gping corp internal-svc`. Entries with the same name override built-in aliases.

## Development

### Project Structure

```
.
├── main.go          # Entry: arg parsing, DNS resolution, geo lookup, ping execution
├── color.go         # Terminal coloring (TTY detection, NO_COLOR support)
├── json.go          # -json mode: one-shot JSON output
├── dnsproto.go      # DoT / DoH protocol implementation
├── dnsalias.go      # Built-in aliases and user config loading
├── dnsalias_test.go # Alias tests
├── dnsproto_test.go # DNS protocol tests
├── json_test.go     # JSON output tests
├── main_test.go     # Arg parsing and end-to-end tests
├── geoip/           # MaxMind database query wrapper
│   └── lookup.go
├── data/            # Database directory (mmdb files excluded via .gitignore)
│   ├── README.md
│   └── embed.go
├── build.sh         # Build script
└── Makefile         # Common command shortcuts
```

### Common Commands

```bash
make build          # Build to ./bin/gping (full, ~70MB)
make build-tiny     # Build to ./bin/gping (tiny, ~8MB, no embedded DB)
make test           # Run tests
make clean          # Clean build artifacts
```

### Release Process

Pushing a tag triggers GitHub Actions to build and publish a release:

```bash
git tag v1.2.0
git push origin v1.2.0
```

CI cross-compiles for `linux/amd64`, `linux/arm64`, `darwin/amd64`, and `darwin/arm64` in both full and tiny variants (8 artifacts total), packages each as `.tar.gz`, and creates a GitHub Release.

### Dependencies

- [maxminddb-golang/v2](https://github.com/oschwald/maxminddb-golang) — MaxMind database reader
- [miekg/dns](https://github.com/miekg/dns) — DoT / DoH protocol
- [golang.org/x/term](https://pkg.go.dev/golang.org/x/term) — TTY detection

## Notes

- **Full build**: The database file (~70MB) is embedded into the binary (not committed to git)
- **Tiny build**: No embedded database; binary is ~8MB. You must provide the MMDB file at one of these locations (checked in order):
  - `GEOIP_CITY_DB` environment variable (full path to the file)
  - `data/GeoLite2-City.mmdb` relative to the working directory
  - `data/GeoLite2-City.mmdb` relative to the binary's directory
  - macOS: `~/Library/Application Support/gping/GeoLite2-City.mmdb`
  - Linux: `$XDG_DATA_HOME/gping/GeoLite2-City.mmdb` (defaults to `~/.local/share/gping/GeoLite2-City.mmdb`)
- The full build also checks these filesystem paths (env var and filesystem paths take priority over the embedded copy)
- Only supports macOS and Linux
- If the database fails to load, a warning is shown but ping still works

## License

MIT License. MaxMind GeoLite2 database is licensed under [CC BY-SA 4.0](https://creativecommons.org/licenses/by-sa/4.0/).

