# AGENTS.md - AI Coding Agent Guidelines

## Project Overview

**ruoyi-proxy** — blue-green deployment proxy for managing Java apps with zero-downtime deployments,
HTTPS auto-configuration, multi-service management, and an AI agent mode (ReAct engine).

- **Language**: Go 1.24.1+
- **Module**: `ruoyi-proxy`
- **Entry point**: `cmd/proxy/main.go` (two modes: CLI `go run . cli` or proxy server `go run .`)

## Build Commands

```bash
make build          # Auto-syncs scripts/configs, then builds for current platform → bin/ruoyi-proxy
make linux          # Cross-compile for Linux → bin/ruoyi-proxy-linux
make linux-hub      # Hub 节点：嵌入完整 AI 配置 + hub.enabled=true → bin/ruoyi-proxy-linux-hub
make linux-spoke HUB_URL=https://your-hub.example.com  # Spoke：嵌入 Hub 地址，不含密钥 → bin/ruoyi-proxy-linux-spoke
make run            # Dev mode: go run cmd/proxy/main.go (proxy server, no CLI)
make cli            # Dev CLI:  go run cmd/proxy/main.go cli
```

**打包前**：在本地 `configs/app_config.json` 写好 AI 配置（可参考 `configs/app_config.example.json`）。

- **Hub 包** (`make linux-hub`)：将 `ai` 段（含 api_key）和 `hub.enabled=true` 打入二进制，部署后可直接对话
- **Spoke 包** (`make linux-spoke HUB_URL=...`)：只嵌入 Hub 地址（`ai.provider=hub`, `ai.base_url`），不含 api_key；服务器上只需 `/agent-config` 填一次性注册 Token

**Windows**: `build.bat linux-hub` / `build.bat linux-spoke`（Spoke 需先 `set HUB_URL=https://...`）

## Testing

No test files exist yet. Use `go test -v ./...` or `go test -v -run TestX ./path/to/package`.

## File Organization

```
ruoyi-proxy/
├── cmd/proxy/
│   ├── main.go              # Entry point (proxy server + /switch /status API)
│   ├── scripts/             # Build artifact: copied from scripts/ (gitignored)
│   └── configs/             # Build artifact: copied from configs/ (gitignored)
├── internal/
│   ├── agent/               # AI Agent (ReAct engine, LLM adapters, tools)
│   │   ├── agent.go         #   ReAct loop, confirm flow, auto-resume (30×5 rounds)
│   │   ├── types.go         #   Message, ToolDef, StreamEvent, ExecContext
│   │   ├── provider.go      #   Provider interface + factory (openai/anthropic/ollama)
│   │   ├── openai.go        #   OpenAI-compatible streaming adapter
│   │   ├── anthropic.go     #   Anthropic streaming adapter
│   │   ├── tools.go         #   Tool definitions + executor (14 tools, backup logic)
│   │   ├── config.go        #   AIConfig load/save from app_config.json
│   │   ├── context.go       #   Message history with token budget
│   │   └── mdstream.go      #   Stream markdown → terminal (glamour)
│   ├── cli/                 # Interactive CLI (readline-based, tab completion)
│   │   ├── cli.go           #   Main CLI loop, all command handlers
│   │   ├── commands.go      #   Agent start, status, deploy, etc.
│   │   ├── config.go        #   Config display/edit, HTTPS enable/disable
│   │   └── embed.go         #   Embedded FS injection from main
│   ├── config/              # Proxy config types + load/save
│   │   └── config.go        #   ServiceConfig, Config, LoadConfig, SaveConfig
│   ├── proxy/               # Reverse proxy (blue-green routing)
│   │   └── proxy.go         #   Path-based service routing, env switching
│   ├── handler/             # (planned, currently empty)
│   └── sync/                # (planned, currently empty)
├── scripts/                 # Source shell scripts (service.sh, init.sh, https.sh, etc.)
├── configs/                 # Source configs + nginx templates
├── bin/                     # Build output (gitignored)
├── build.bat                # Windows build script
└── Makefile
```

**Key fact**: `internal/handler/` and `internal/sync/` are empty — they are planned but have no code.
Do not add import references to them.

## Build Flow & Embed Mechanics

The `main.go` uses `//go:embed scripts/*` and `//go:embed configs/*` on `cmd/proxy/` subdirectories.
Before compiling, `make sync` (or `build.bat`) copies `scripts/` and `configs/` into `cmd/proxy/`.
Both `cmd/proxy/scripts/` and `cmd/proxy/configs/` are **gitignored** — they only exist after sync.

**If you modify anything in `scripts/` or `configs/`, you must rebuild** (not just `go run`).
`make build` handles this automatically.

## Runtime Config Files (gitignored)

These are generated at runtime, **never commit them**:

```
configs/proxy_config.json    # Multi-service proxy targets + active env
configs/app_config.json      # Domain, HTTPS, JVM presets, AI provider config
configs/sync_config.json     # File sync settings
```

## Code Conventions

- **Chinese comments** for business logic (exported types/fields/functions)
- **Go doc comments** use `// FunctionName 功能描述` format
- **JSON tags** required on all serializable struct fields
- **Error wrapping** with `fmt.Errorf("上下文: %v", err)` — never discard errors
- **Logging**: `log.Printf` / `log.Fatalf` (not `println`, not `fmt.Println` for errors)
- **Concurrency**: `sync.RWMutex` for shared state; always `defer mu.Unlock()`
- **Imports**: stdlib → third-party → `ruoyi-proxy/internal/...` (three groups, blank lines between)

## Agent Module Key Design

- ReAct loop: think (LLM stream) → act (execute tools) → observe (feed results back), up to 30 rounds
- Auto-resume: up to 5 continuation injections (150 effective rounds max)
- Write tools require confirmation UI unless user already approved this turn
- `run_shell` with read-only commands (ls, cat, grep, systemctl status, etc.) skips confirmation
- Important files auto-backed up to `~/.ruoyi-backup/<timestamp>/` before modification
- Tool output truncated to 3000 chars; Anthropic requires non-empty tool result strings
- AI config stored in `configs/app_config.json` under `"ai"` key, not a separate file
- System prompt adapts by build profile (Hub/Spoke/default): probes deployment mode before assuming blue-green proxy; injects `spoke_profile.json` or registered Spoke list from `hub_spokes.json`
- `get_status` uses adaptive health checks (script status → project-type HTTP/TCP → Java actuator only when applicable)
- Spoke `/self-check` (`RunSpokeChecks`): base env + Hub connectivity; :8000 proxy is optional (skipped if not listening)

## CLI Architecture

- **`make cli` / `ruoyi-proxy cli` 默认直接进入 AI Agent 对话模式**（不再是菜单式 REPL）
- 运维命令以 **`/xxx` 斜杠命令** 在 Agent 提示符下调用（如 `/start`、`/deploy`、`/status`）；也可用自然语言描述需求
- `/help` 或 `/commands` 查看命令列表；`/agent-config` 配置 AI；`/exit` 退出
- Uses `github.com/chzyer/readline` for tab completion and prompt management
- Service commands call per-service control script via `bash` with env vars:
  `SERVICE_ID`, `APP_NAME`, `APP_JAR_PATTERN`, `BLUE_PORT`, `GREEN_PORT`, `APP_HOME`
- Default script is `scripts/service.sh`; non-Java projects can register custom script via `ServiceConfig.script_path` (AI tool `configure_service`)
- `APP_HOME` is derived from `scripts/service.sh` location (two levels up)
- Proxy process management: start via `exec.Command`, stop via PID kill or port-lookup

## Hub AI Gateway (多服务器)

- Hub 模式：`configs/app_config.json` 中 `"hub": {"enabled": true}` 启用
- Hub 在代理端口暴露 `/__hub__/v1/register` 和 `/__hub__/v1/chat`；管理端口暴露 `/hub/token`、`/hub/status`、`/hub/revoke`
- Spoke 在 `/agent-config` 选择 `hub` 提供商，用 Hub 上 `/hub-token` 生成的一次性 Token 注册，之后 AI 调用经 Hub 转发（本地仍执行工具）
- Spoke 注册表持久化在 `configs/hub_spokes.json`（gitignored 运行时文件）
- v1 转发为非流式（整段返回）；Hub 进程重启会清空未使用的一次性注册 Token

## Important Notes

1. `make build` includes sync — no separate `make sync` needed before building
2. Scripts/configs changes require recompile (embedded at build time)
3. No test files exist; add `*_test.go` alongside source files
4. `internal/handler/` and `internal/sync/` are empty and should stay that way until explicitly developed
5. Platform: Windows (dev), Linux (deployment); `build.bat` defaults to Linux cross-compile
6. Config output goes to `bin/ruoyi-proxy` (current platform) or `bin/ruoyi-proxy-linux` (Linux)
