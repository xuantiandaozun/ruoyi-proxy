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
make linux          # Cross-compile for Linux (GOOS=linux GOARCH=amd64) → bin/ruoyi-proxy-linux
make run            # Dev mode: go run cmd/proxy/main.go (proxy server, no CLI)
make cli            # Dev CLI:  go run cmd/proxy/main.go cli
make install        # go mod tidy && go mod download
make clean          # rm -rf bin/ + runtime config files
make fmt            # go fmt ./...
make test           # go test -v ./...
```

**Windows**: `build.bat` (no args = Windows build, `build.bat linux` = Linux cross-compile).
It runs xcopy to sync scripts/configs before building, equivalent to `make sync && make build`.

`make build` / `make linux` already call `make sync` first — no separate sync needed.

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

## CLI Architecture

- Uses `github.com/chzyer/readline` for tab completion and prompt management
- Service commands (start/stop/deploy/logs) call `service.sh` via `bash` with env vars:
  `SERVICE_ID`, `APP_NAME`, `APP_JAR_PATTERN`, `BLUE_PORT`, `GREEN_PORT`, `APP_HOME`
- `APP_HOME` is derived from `scripts/service.sh` location (two levels up)
- Proxy process management: start via `exec.Command`, stop via PID kill or port-lookup

## Important Notes

1. `make build` includes sync — no separate `make sync` needed before building
2. Scripts/configs changes require recompile (embedded at build time)
3. No test files exist; add `*_test.go` alongside source files
4. `internal/handler/` and `internal/sync/` are empty and should stay that way until explicitly developed
5. Platform: Windows (dev), Linux (deployment); `build.bat` defaults to Linux cross-compile
6. Config output goes to `bin/ruoyi-proxy` (current platform) or `bin/ruoyi-proxy-linux` (Linux)
