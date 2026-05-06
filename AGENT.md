# AGENT.md

给 Code agent 的项目速览。读完就能上手改动。

## 项目是什么

`gping` —— 在调用系统 `ping` 之前，先解析目标、查 GeoIP、可选打印 CNAME 与多 IP 列表的命令行小工具。仅支持 macOS / Linux。

执行流程（`main.go:main`）：
1. `parseArgs` 解析 `[<dns>] <host> [-4|-6] [-c N]`
2. `resolveTarget` 解析目标，得到 `targetIP` / `allIPs`（多 IP 时随机选一个 ping）
3. `geoip.NewGeoIPLookup` + `LookupCity` 取地理信息
4. 可选打印 `DNS Server` / `CNAME` / `IPs` 头部块
5. `printGPINGLine` 打印彩色 `GPING ...` 行
6. `executePing` `exec` 系统 `ping`/`ping6`，逐行透传 stdout（吞掉首行 `PING ...` banner）

## 仓库结构

```
main.go            参数解析、DNS/CNAME 解析、ping 子进程编排
color.go           TTY 检测 + ANSI 着色（GPING 行、CNAME 行）
geoip/lookup.go    MMDB 加载策略 + City 查询解码
data/embed.go      go:embed GeoLite2-City.mmdb（70MB，未提交）
data/README.md     数据库放置说明
Makefile           build / download-geolite
build.sh           编译并安装到 $HOME/bin（默认）
```

## 构建与运行

```bash
make build                       # 产物 bin/gping
make download-geolite            # 拉取 GeoLite2-City.mmdb 到 data/
./build.sh [INSTALL_DIR]         # 编译并安装到 $HOME/bin（或自定义）
go build -o bin/gping .          # 直接 go build
```

冒烟测试（不需要 sudo，ping 走 setuid）：

```bash
./bin/gping -c 2 1.1.1.1
./bin/gping www.youtube.com -c 3
./bin/gping 8.8.8.8 ipxy.cc -4 -c 2     # 自定义 DNS 服务器
```

## 编辑时要注意的点

- **改 main.go 后必须重新构建到 `bin/gping`**：`go build ./...` 只做检查不写文件，会跑到旧二进制上误判。统一用 `go build -o bin/gping .` 或 `make build`。
- **参数解析**：`main.go:parseArgs` 是单次扫描，`-4`/`-6`/`-c N` 可与 positional 任意顺序穿插。新增选项就在那个 switch 里加 case，并同步更新 `printUsage` 与未知选项错误提示文本。
- **彩色输出走 TTY 检测**：`color.go:stdoutANSI` 同时尊重 `NO_COLOR` / `CLICOLOR=0` / `TERM=dumb`。新增彩色行要遵循"非 TTY 降级为纯文本"的形态（参考 `printCNAMELine` 的 `→` ↔ `->`）。
- **CNAME 查询走 `net.Resolver`**：`lookupCNAME` 在自定义 DNS 时复用 `PreferGo` 的 resolver；macOS 上 cgo resolver 有时也能拿到 CNAME，但不要依赖。返回 `""` 表示无需展示（IP 字面量 / 查询失败 / canonical == target）。
- **MMDB 加载顺序**：`GEOIP_CITY_DB` → `./data/GeoLite2-City.mmdb` → 可执行文件旁的 `data/GeoLite2-City.mmdb` → 嵌入字节（`data.CityDB`）。改这里要保持"文件系统优先于 embed"的语义（embed 容易因构建缓存而过期）。
- **ping 透传**：`executePing` 故意 `signal.Ignore(os.Interrupt)`，让 Ctrl+C 直接交给 `ping` 子进程，避免 Go runtime 多打一行 `signal: interrupt`。`skipPingBannerLine` 仅吞首行 `PING ...`，保留其余原样输出。
- **IPv6 ping**：macOS 用 `ping6`，Linux 用 `ping -6`，见 `pingCommand`。新选项要考虑两条路径都 append。
- **多 IP 随机选择**：`resolveTarget` 用 `math/rand.Intn` 从 `allIPs` 里挑一个，未设种子（Go 1.20+ 默认随机）。`formatPingIPList` 把被选中的那个加 `*` 前缀。

## 不要做的事

- 不要把 `data/GeoLite2-City.mmdb` 提交进 git（已在 `.gitignore`，70MB）。
- 不要为了"未来扩展"加抽象层；本项目刻意保持平铺、单二进制、零内部包间接层（除 `geoip` / `data` 这两个必要边界）。
- 不要给 ping 输出再加解析/重排——透传是设计目标，用户期望和系统 `ping` 行为一致。
- 不要在 `pingCommand` 里硬编码额外标志；新需求走 CLI 选项 → 透传，保持"gping 是 ping 的薄壳"这个心智模型。

## 提交风格

近期 commit 都是中文 `type: 描述` 形式（`feat:` / `fix:` / `build:`），主题行简短，正文可省。沿用即可。
