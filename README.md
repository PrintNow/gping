# gping

一个增强版的 ping 工具，在执行 ping 之前会显示目标 IP 的地理位置信息。

## 功能特性

- 支持直接 ping IP 地址
- 支持域名解析后 ping（多个 IP 时随机选择一个）
- 显示 IP 的地理位置信息（国家、城市）
- MaxMind GeoLite2 数据库嵌入到二进制文件中

## 准备工作

在构建之前，需要下载 MaxMind GeoLite2 数据库文件。

### 下载数据库

1. 访问 MaxMind 注册页面：https://www.maxmind.com/en/geolite2/signup
2. 注册并登录账号
3. 下载 GeoLite2 City 数据库（MMDB 格式）
4. 解压下载的文件，将 `GeoLite2-City.mmdb` 文件复制到本项目的 `data/` 目录下

## 构建

```bash
go build -o gping
```

## 使用方法

### 基本用法

```bash
# Ping IP 地址
./gping 1.1.1.1

# Ping 域名（会先解析域名）
./gping ipxy.cc
```

### 输出示例

```
Los Angeles, United States

PING 1.1.1.1 (1.1.1.1): 56 data bytes
64 bytes from 1.1.1.1: icmp_seq=0 ttl=57 time=5.123 ms
64 bytes from 1.1.1.1: icmp_seq=1 ttl=57 time=5.234 ms
...
```

输出格式说明：
- 第一行显示地理位置：`城市, 国家`
- 如果没有城市信息，则只显示国家
- 紧凑的格式，信息清晰

## 工作原理

1. **参数解析**：获取命令行参数（IP 或域名）
2. **DNS 解析**：如果输入是域名，则解析为 IP 地址（多个 IP 时随机选择）
3. **查询地理信息**：使用 MaxMind GeoLite2 数据库查询 IP 的地理位置
4. **显示信息**：格式化输出地理位置信息
5. **执行 ping**：调用系统原生 ping 命令

## 技术栈

- Go 1.25+
- [maxminddb-golang/v2](https://github.com/oschwald/maxminddb-golang) - MaxMind 数据库读取
- Go embed - 嵌入数据库文件到二进制

## 注意事项

- 数据库文件约 70MB，会被嵌入到二进制文件中
- 数据库文件不会被提交到 git 仓库（已加入 .gitignore）
- 仅支持 Unix-like 系统（macOS、Linux），不支持 Windows
- 如果数据库加载失败，会显示警告但仍会继续执行 ping

## 许可证

本项目仅供学习和个人使用。MaxMind GeoLite2 数据库遵循 [CC BY-SA 4.0](https://creativecommons.org/licenses/by-sa/4.0/) 许可证。
