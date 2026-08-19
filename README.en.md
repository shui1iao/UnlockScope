# UnlockScope

UnlockScope is an independent Go CLI that uses credential-free HTTP probes to heuristically check access to streaming, AI, forum/social, knowledge, sports, and game-store services from the current egress. It was designed from scratch and does not copy the source code of other region-checking projects.

> **A probe is not a platform guarantee.** Providers change pages, CDNs, login flows, and regional APIs. `available` means that the public probe passed at this moment; it does not promise account-level playback, purchasing, or every feature. Dynamic pages, login walls, rate limits, and ambiguous responses become `unknown` or `failed` rather than a fabricated verdict.

## Quick start

```bash
go run ./cmd/unlockscope --scope auto --no-color
go run ./cmd/unlockscope --scope ai --timeout 5s --concurrency 4
go run ./cmd/unlockscope --provider netflix,chatgpt --json
```

The planned public one-line entry point (available once Releases are published) is:

```bash
bash <(curl -Ls https://unlock.shuijiao.de)
```

The installer downloads a platform archive and its `.sha256` file from GitHub Releases, verifies the archive before extraction, and then installs it. Pin a release explicitly:

```bash
UNLOCKSCOPE_VERSION=v0.1.0 bash install.sh
UNLOCKSCOPE_VERSION=v0.1.0 PREFIX="$HOME/.local/bin" bash install.sh
```

## Options

```text
--scope auto|all|global|ai|social|tw|hk|jp|kr|na|sa|eu|af|oc|sports|games
--region CODE       override egress detection; accepts country codes or groups
--provider ID,...   select providers; repeatable; --list-providers lists all
--ip auto|4|6       ipv4/ipv6 are also accepted
--proxy URL         http://, https://, socks5://, or socks5h://
--interface NAME    bind an address from this interface
--source IP         bind an explicit source address
--timeout DURATION  per-provider timeout (default 8s)
--total-timeout DURATION global timeout (default 60s)
--concurrency N     maximum concurrent checks (default 8)
--json              machine-readable JSON output
--no-color/--plain  plain text output
--version
```

`auto` first queries `ipapi.co` for the egress country (a failure does not stop checks), then selects global providers plus the matching regional group. Use `--region hk` or another explicit group to avoid the geo lookup. The configured proxy is used for both geo detection and provider requests.

## Result model

- `available`: a stable JSON/status signal was returned, or the response matched the service marker without a regional block signal.
- `unavailable`: an explicit denial or service-unavailable signal was found.
- `region_only`: a regional restriction signal was found in the page or HTTP 403/451 response.
- `failed`: network error, per-item/global timeout, or server-side 5xx.
- `unknown`: login wall, rate limit, dynamic response, unstable JSON, or another ambiguous outcome.

The default terminal output is grouped under English category headings. Human-readable states use Chinese labels, uppercase region codes, and full-width parentheses, for example `可用（JP）`; `unavailable` is shown without a region. The stable `--json` fields and values are unchanged.

JSON is an array. Each item has the stable fields `id`, `service`, `category`, `regions`, `state`, `region`, `note`, `duration_ms`, and `checked_at`. Consumers should depend on `state`, not on the human-readable `note`:

```json
[
  {
    "id": "netflix",
    "service": "Netflix",
    "category": "streaming",
    "regions": [],
    "state": "available",
    "region": "jp",
    "note": "Public page reachable; heuristic result",
    "duration_ms": 184,
    "checked_at": "2026-08-16T00:00:00Z"
  }
]
```

## Coverage

The initial catalog contains **161 providers**, including:

- Global video/music: TikTok, Netflix, Disney+, YouTube Premium, YouTube CDN, Prime Video, Spotify, DAZN, Max, Hulu, Paramount+, Peacock, Crunchyroll, Apple TV+, Pluto TV, Tubi, iQIYI, Viki, Bilibili, SoundCloud, Deezer, TIDAL, and others.
- AI: ChatGPT, Claude, Gemini, Microsoft Copilot, Grok, Perplexity, Meta AI, Poe, DeepSeek, Mistral, Kimi, Qwen, Coze, HuggingChat, Google AI Studio, NotebookLM, and others.
- Forum/social/knowledge: TikTok, Reddit, Instagram, Facebook, X, Threads, Discord, Twitch, Kick, and Wikipedia editability (the edit page is read only; no write is performed).
- Games/stores: Steam currency (public featured-categories JSON reachability; it does not guess account currency), Epic Games Store, Xbox, PlayStation Store, Nintendo eShop, and Roblox.
- Regional representatives: Viu/Now TV/myTV SUPER/TVB/HOY TV/RTHK in Hong Kong; KKBOX/KKTV/LiTV/Hami Video/MyVideo/4GTV/Bahamut in Taiwan; ABEMA/U-NEXT/DMM TV/TVer/WOWOW/NHK Plus/radiko in Japan; Wavve/Watcha/TVING/Coupang Play/KBS/Naver TV in Korea; plus representative services for North America, Europe, South America, Africa, and Oceania.

URLs, categories, regional groups, and signal words live in `internal/provider/provider.go`. New checks can implement the `Provider` interface or add a declarative definition there.

## Privacy, proxies, and IP families

- The default behavior is credential-free public `GET` requests with an UnlockScope User-Agent. It does not log in, submit forms, write to Wikipedia, or carry private tokens/API keys.
- `--proxy` supports HTTP(S) and SOCKS5. Never put a password-bearing proxy URL in public logs or CI output.
- `--ip ipv4` and `--ip ipv6` force an address family; `auto` leaves dual-stack selection to the system.
- Response bodies are capped at 2 MiB. Per-provider and global timeouts plus a concurrency limit are enforced.
- Public endpoints may log your IP, User-Agent, and access time. Follow local law, network policy, and provider terms; use only proxies you are authorized to use.

## GUKO integration

UnlockScope has no built-in GUKO client and changes no GUKO configuration. A controlled GUKO probe or scheduled job can invoke the local binary:

```bash
unlockscope --scope auto --json --no-color > /var/lib/guko/unlockscope.json
```

GUKO can inspect `state` and apply its own alert policy to `failed` and persistent `unavailable`/`region_only` outcomes. Keep the latest result and egress context, but do not put proxy credentials, environment variables, or full response bodies into notifications. Start with a small `--provider` set in a candidate environment and configure task-specific resource, interval, and retry limits.

## Development

Go 1.22+ is required:

```bash
make fmt
make test
make race
make vet
make lint
make build
bash -n install.sh
```

CI runs tests, race tests, gofmt, go vet, golangci-lint, and cross-platform builds. A `v*` tag produces tar.gz and SHA-256 files for Linux, macOS, and FreeBSD on amd64/arm64, plus Linux 386.

## License and security

This project is MIT licensed. Read [SECURITY.md](SECURITY.md); never disclose proxy passwords, cookies, tokens, full network responses, or personal IP logs in issues.
