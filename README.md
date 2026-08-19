# UnlockScope

UnlockScope 是一个独立的 Go 命令行工具，用公开、无凭据的 HTTP 探针启发式判断流媒体、AI、论坛/社交和游戏商店在当前出口的位置是否可访问。项目名称、代码和测试均独立设计，不复制其他区域检测项目的源码。

> **重要：检测不是平台承诺。** 平台会改变页面、CDN、登录策略和地区接口；`available` 只表示公开探针在本次请求中通过，不能代表账号能播放、购买或使用全部功能。遇到动态页面、限流、登录墙或无法解释的响应，工具会返回 `unknown`/`failed`，不会伪造确定结论。

## 快速开始

```bash
go run ./cmd/unlockscope --scope auto --no-color
go run ./cmd/unlockscope --scope ai --timeout 5s --concurrency 4
go run ./cmd/unlockscope --provider netflix,chatgpt --json
```

发布版本的一键入口（上线 Release 后启用）：

```bash
bash <(curl -Ls https://unlock.shuijiao.de)
```

入口脚本应下载 GitHub Release 的对应平台压缩包，并在解压前校验 `.sha256`。也可以固定版本：

```bash
UNLOCKSCOPE_VERSION=v0.1.0 bash install.sh
# 或：UNLOCKSCOPE_VERSION=v0.1.0 PREFIX="$HOME/.local/bin" bash install.sh
```

## 选项

```text
--scope auto|all|global|ai|social|tw|hk|jp|kr|na|sa|eu|af|oc|sports|games
--region CODE       覆盖出口地区自动探测；支持国家码和上面的区域组
--provider ID,...   指定 provider，可重复；--list-providers 查看完整清单
--ip auto|4|6       也接受 ipv4/ipv6
--proxy URL         http://、https://、socks5:// 或 socks5h://
--interface NAME    绑定指定网卡上的匹配地址
--source IP         绑定指定源地址
--timeout DURATION  单项超时，默认 8s
--total-timeout DURATION 全局超时，默认 60s
--concurrency N     并发上限，默认 8
--json              输出机器可读 JSON
--no-color/--plain 纯文本输出
--version
```

默认 `auto` 会先通过 `ipapi.co` 查询出口国家（失败时不阻断检查），然后运行 `global` provider 与对应的地区组。`--region hk` 等显式参数可以避免地理服务请求；代理会同时用于地区探测和 provider 请求。

## 状态模型

- `available`：公开探针返回稳定 JSON/状态信号，或正文命中服务标识且未发现地区阻断信号。
- `unavailable`：明确的拒绝/服务不可用信号。
- `region_only`：页面、HTTP 403/451 或正文包含地区限制信号。
- `failed`：网络错误、单项/全局超时或服务端 5xx。
- `unknown`：登录墙、限流、动态响应、非稳定 JSON 等无法可靠判定的情况。

默认终端输出按英文分类标题分组；状态使用中文，地区代码统一大写并放在中文括号中，例如 `可用（JP）`。`unavailable` 只显示“不可用”，不附带地区。`--json` 的稳定字段和值保持不变。

JSON 是数组，每项的稳定字段为 `id`、`service`、`category`、`regions`、`state`、`region`、`note`、`duration_ms`、`checked_at`。消费者应依赖 `state`，不要依赖可变化的 `note`：

```json
[
  {
    "id": "netflix",
    "service": "Netflix",
    "category": "streaming",
    "regions": [],
    "state": "available",
    "region": "jp",
    "note": "公开页面可访问；结果为启发式判断",
    "duration_ms": 184,
    "checked_at": "2026-08-16T00:00:00Z"
  }
]
```

## 覆盖范围

初版包含 **161 个 provider**，覆盖：

- 全球流媒体/音乐/视频：TikTok、Netflix、Disney+、YouTube Premium、YouTube CDN、Prime Video、Spotify、DAZN、Max、Hulu、Paramount+、Peacock、Crunchyroll、Apple TV+、Pluto TV、Tubi、iQIYI、Viki、Bilibili、SoundCloud、Deezer、TIDAL 等。
- AI：ChatGPT、Claude、Gemini、Microsoft Copilot、Grok、Perplexity、Meta AI、Poe、DeepSeek、Mistral、Kimi、Qwen、Coze、HuggingChat、Google AI Studio、NotebookLM 等。
- 论坛/社交/知识：TikTok、Reddit、Instagram、Facebook、X、Threads、Discord、Twitch、Kick、Wikipedia editability（只读编辑页，不执行写入）。
- 游戏/商店：Steam currency（公开 featured-categories JSON 可达性，不猜测账号货币）、Epic Games Store、Xbox、PlayStation Store、Nintendo eShop、Roblox。
- 地区代表：香港（Viu、Now TV、myTV SUPER、TVB、HOY TV、RTHK）、台湾（KKBOX、KKTV、LiTV、Hami Video、MyVideo、4GTV、巴哈姆特）、日本（ABEMA、U-NEXT、DMM TV、TVer、WOWOW、NHK Plus、radiko）、韩国（Wavve、Watcha、TVING、Coupang Play、KBS、Naver TV）、北美、欧洲、南美、非洲、大洋洲的代表性站点。

服务 URL、分类、地区组与判定词集中在 `internal/provider/provider.go`，新增服务可通过实现 `Provider` 接口或增加声明式定义完成。

## 隐私、代理与网络

- 默认只发送带有 UnlockScope User-Agent 的公开 `GET`，不登录、不提交表单、不写入 Wikipedia、不携带私密 token/API key。
- `--proxy` 支持 HTTP(S) 和 SOCKS5；不要把含密码的代理 URL 放进公共日志或 CI 输出。
- `--ip ipv4`/`--ip ipv6` 强制出口地址族，`auto` 使用系统默认双栈选择。
- 请求体限制为 2 MiB；单项和总超时均有上限控制，且并发数可调。
- 第三方公开端点可能记录 IP、User-Agent 和访问时间；请按所在地法律、网络政策和服务条款使用。需要更强隐私时请使用你有权使用的代理，并审查代理日志。

## 与 GUKO 集成

UnlockScope 不内置 GUKO 客户端，也不会改动 GUKO 配置。可以在 GUKO 的受控探针/定时任务中调用本地二进制：

```bash
unlockscope --scope auto --json --no-color > /var/lib/guko/unlockscope.json
```

GUKO 侧读取 JSON 的 `state`，将 `failed`、持续的 `unavailable`/`region_only` 按自身告警策略处理；建议保留最近一次结果和探针出口说明，不要把代理凭据、环境变量或完整响应正文写入通知。部署前请先在候选环境运行 `--provider` 小范围检查，并给任务设置自己的资源、频率和失败重试上限。

## 开发

需要 Go 1.22+：

```bash
make fmt
make test
make race
make vet
make lint
make build
bash -n install.sh
```

CI 会运行 test、race、gofmt、go vet、golangci-lint 和跨平台构建。推送 `v*` tag 会为 Linux、macOS、FreeBSD 的 amd64/arm64，并额外为 Linux 386 生成 tar.gz 与 SHA-256 文件。

## 许可证与安全

本项目使用 MIT License。请阅读 [SECURITY.md](SECURITY.md)；不要在 issue 中公开代理密码、cookie、token、完整网络响应或个人 IP 记录。
