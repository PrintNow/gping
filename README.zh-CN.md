<p align="center"><a href="README.md">English</a> · <strong>简体中文</strong></p>

# gping

> **g**eo + **ping** — 一个增强版的 ping 工具，在执行 ping 前显示目标 IP 的地理位置信息。支持 DoT / DoH 自定义 DNS。

```
$ gping 1.1.1.1
Los Angeles, United States

PING 1.1.1.1 (1.1.1.1): 56 data bytes
64 bytes from 1.1.1.1: icmp_seq=0 ttl=57 time=5.123 ms
64 bytes from 1.1.1.1: icmp_seq=1 ttl=57 time=5.234 ms
...
```

## 快速上手

### 下载安装

从 [GitHub Releases](../../releases) 下载对应平台的压缩包，解压后放入 PATH：

```bash
# macOS (Apple Silicon)
curl -LO "https://github.com/PrintNow/gping/releases/download/v1.1.0/gping-darwin-arm64-v1.1.0.tar.gz"
tar xzf "gping-darwin-arm64-v1.1.0.tar.gz"
rm "gping-darwin-arm64-v1.1.0.tar.gz"
mv gping ~/.local/bin/

# Linux (x86_64)
curl -LO "https://github.com/PrintNow/gping/releases/download/v1.1.0/gping-linux-amd64-v1.1.0.tar.gz"
tar xzf "gping-linux-amd64-v1.1.0.tar.gz"
rm "gping-linux-amd64-v1.1.0.tar.gz"
mv gping ~/.local/bin/
```

#### 精简版（约 8MB，不内嵌数据库）

更小的二进制文件，不包含内嵌的 GeoLite2 数据库。需自行提供 MMDB 文件（放置路径见[注意事项](#注意事项)）。

```bash
# macOS (Apple Silicon)
curl -LO "https://github.com/PrintNow/gping/releases/download/v1.1.0/gping-tiny-darwin-arm64-v1.1.0.tar.gz"
tar xzf "gping-tiny-darwin-arm64-v1.1.0.tar.gz"
rm "gping-tiny-darwin-arm64-v1.1.0.tar.gz"
mv gping ~/.local/bin/
```

### 从源码构建

需要 Go 1.25+。

```bash
# 完整构建（嵌入 GeoLite2-City.mmdb，二进制约 70MB）
# 下载数据库（无需注册账号）：
make download-geolite
make build

# 或从 MaxMind 手动下载（需注册账号）：
# https://www.maxmind.com/en/geolite2/signup
# 将 GeoLite2-City.mmdb 放到 data/ 目录

# 精简构建（二进制约 8MB，运行时需外部 MMDB）
make build-tiny
```

## 使用方法

```bash
# 基本用法
gping 1.1.1.1            # ping IP
gping ipxy.cc             # ping 域名（多 IP 时随机选一个）

# 指定 DNS
gping 127.0.0.1:5353 ipxy.cc                # 自定义端口
gping cf ipxy.cc                            # 内置别名 → DoH (Cloudflare)
gping ali www.youtube.com                   # 内置别名 → DoH (阿里)
gping doh://dns.google/dns-query baidu.com  # 完整 DoH URL
gping dot://cf gping.dev                    # DoT (Cloudflare)
gping dot://192.168.1.1 internal-svc        # DoT 内网

# 透传 ping 参数
gping -c 5 1.1.1.1
```

### 内置 DNS 别名

| 别名 | 服务 |
|------|------|
| `cf` / `cloudflare` | Cloudflare DoH |
| `google` / `g` | Google DoH |
| `quad9` | Quad9 DoH |
| `adguard` | AdGuard DoH |
| `ali` / `aliyun` | 阿里 DoH |
| `dnspod` / `tx` | DNSPod DoH |
| `360` | 360 DoH |

短别名默认走 DoH；DoT 需显式 `dot://` 前缀。

### 自定义别名

创建 `~/.config/gping/dns.toml`（或 `$XDG_CONFIG_HOME/gping/dns.toml`）：

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

之后 `gping corp internal-svc` 即可。同名条目会覆盖内置别名。

## 开发指南

### 项目结构

```
.
├── main.go          # 入口：参数解析、DNS 解析、地理位置查询、调用 ping
├── color.go         # 终端着色（TTY 检测、NO_COLOR 支持）
├── json.go          # -json 模式一次性输出
├── dnsproto.go      # DoT / DoH 协议实现
├── dnsalias.go      # 内置别名与用户配置加载
├── dnsalias_test.go # 别名测试
├── dnsproto_test.go # DNS 协议测试
├── json_test.go     # JSON 输出测试
├── main_test.go     # 参数解析与端到端测试
├── geoip/           # MaxMind 数据库查询封装
│   └── lookup.go
├── data/            # 数据库目录（.gitignore 排除 mmdb 文件）
│   ├── README.md
│   └── embed.go
├── build.sh         # 构建脚本
└── Makefile         # 常用命令快捷入口
```

### 常用命令

```bash
make build          # 构建到 ./bin/gping（完整版，约 70MB）
make build-tiny     # 构建到 ./bin/gping（精简版，约 8MB，不含内嵌数据库）
make test           # 运行测试
make clean          # 清理构建产物
```

### 发布流程

打 tag 后 GitHub Actions 自动构建并发布 Release：

```bash
git tag v1.1.0
git push origin v1.1.0
```

CI 会交叉编译 `linux/amd64`、`linux/arm64`、`darwin/amd64`、`darwin/arm64` 四个平台的完整版和精简版（共 8 个产物），打包为 `.tar.gz` 并创建 Release。

### 依赖

- [maxminddb-golang/v2](https://github.com/oschwald/maxminddb-golang) — MaxMind 数据库读取
- [miekg/dns](https://github.com/miekg/dns) — DoT / DoH 协议
- [golang.org/x/term](https://pkg.go.dev/golang.org/x/term) — TTY 检测

## 注意事项

- **完整构建**：数据库文件约 70MB，嵌入到二进制中（不提交到 git）
- **精简构建**：不嵌入数据库，二进制约 8MB。需自行提供 MMDB 文件，放在以下任一位置（按优先级）：
  - `GEOIP_CITY_DB` 环境变量（完整路径）
  - 工作目录下的 `data/GeoLite2-City.mmdb`
  - 可执行文件所在目录下的 `data/GeoLite2-City.mmdb`
  - macOS：`~/Library/Application Support/gping/GeoLite2-City.mmdb`
  - Linux：`$XDG_DATA_HOME/gping/GeoLite2-City.mmdb`（默认 `~/.local/share/gping/GeoLite2-City.mmdb`）
- 完整构建同样支持上述文件系统路径（环境变量和文件路径优先于嵌入副本）
- 仅支持 macOS 和 Linux
- 数据库加载失败时会显示警告，仍可正常 ping

## 许可证

MIT License。MaxMind GeoLite2 数据库遵循 [CC BY-SA 4.0](https://creativecommons.org/licenses/by-sa/4.0/) 许可证。
