# Ruoyi Proxy - Blue-Green Deployment Proxy Server

<div align="center">

[![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat&logo=go)](https://golang.org)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Platform](https://img.shields.io/badge/Platform-Linux%20%7C%20Windows-lightgrey)](https://github.com)

A full-featured blue-green deployment proxy server with zero-downtime deployments, automatic HTTPS, multi-service management, file synchronization, and AI-powered operations.

[Features](#-features) • [Quick Start](#-quick-start) • [Usage Guide](#-usage-guide) • [AI Agent Mode](#-ai-agent-mode) • [Hub AI Gateway](#-hub-ai-gateway) • [Architecture](#-architecture) • [Contributing](#-contributing)

**[🇨🇳 中文文档](README_CN.md)**

</div>

---

## 📋 Table of Contents

- [Features](#-features)
- [Quick Start](#-quick-start)
- [Project Structure](#-project-structure)
- [Configuration](#-configuration)
- [Usage Guide](#-usage-guide)
- [AI Agent Mode](#-ai-agent-mode)
- [Hub AI Gateway](#-hub-ai-gateway)
- [Deployment Workflow](#-deployment-workflow)
- [Troubleshooting](#-troubleshooting)
- [Architecture](#-architecture)
- [Contributing](#-contributing)
- [License](#-license)

---

## ✨ Features

### 🔄 Blue-Green Deployment
- **Zero-downtime** - Seamless environment switching, invisible to end users
- **One-click rollback** - Instantly revert to the previous version when issues arise
- **Health checks** - Automatic service health detection before switching
- **Long-connection support** - Handles SSE, WebSocket, and other long-lived requests

### 🎯 Multi-Service Management
- **Multiple services** - Manage multiple Java application services simultaneously
- **Quick switching** - Switch the active service with a single command
- **Independent config** - Per-service port and JAR file configuration
- **Batch operations** - Start, stop, and deploy multiple services at once

### 🔐 Automated HTTPS
- **Auto certificate** - Integrated Let's Encrypt for one-click free SSL certificates
- **Auto renewal** - Built-in support for certificate auto-renewal
- **One-click toggle** - Enable/disable HTTPS mode instantly
- **HTTP redirect** - Automatic HTTP → HTTPS redirect configuration

### 🔧 Interactive CLI
- **Friendly interface** - Color output, command hints, simple operations
- **Real-time monitoring** - Live service status and log streaming
- **Confirmation prompts** - Dangerous operations require explicit confirmation
- **Tab completion** - Shell-style command auto-completion

### 🌐 Nginx Integration
- **Auto configuration** - Automatically generates Nginx config files
- **Static file serving** - Host Vue and other frontend applications
- **Reverse proxy** - Automatic reverse proxy rule configuration
- **Config templates** - Ready-made HTTP and HTTPS templates

### 📦 Single-File Deployment
- **Embedded scripts** - All scripts and templates bundled into the binary
- **One-file deploy** - Upload a single executable to get started
- **Cross-platform build** - Compile Linux binaries from Windows

### 🤖 AI Agent Operations
- **Default entry** - `cli` launches directly into AI Agent mode; ops commands use `/xxx` slash syntax
- **Natural language** - Describe tasks in plain English; AI plans and executes them
- **Multi-LLM support** - Works with Anthropic Claude, OpenAI, and any compatible API (DeepSeek, Qwen, Ollama, etc.)
- **Session management** - Persistent multi-session history with `/sessions`, `/load`, `/new`
- **File management** - Read, modify, and delete server files with automatic backup for important files
- **Service management** - Install packages, manage systemd services, run shell commands
- **Safe confirmation** - Write operations require confirmation; one approval covers an entire turn

### 🌐 Hub AI Gateway (Multi-Server)
- **Centralized keys** - Hub holds the AI API key; Spoke servers never need their own secrets
- **Token registration** - Hub generates one-time registration tokens; Spokes register via `/agent-config`
- **Local execution** - AI inference goes through Hub; tool calls still run on the Spoke machine
- **Pre-built packages** - `make linux-hub` / `make linux-spoke` for Hub and Spoke nodes

### 💾 Low-Memory Deployment
- **Memory-friendly** - `/deploy-lowmem` stops the old service before starting the new one
- **Use case** - Small VPS instances (1G/2G RAM) that cannot run blue and green simultaneously

---

## 🚀 Quick Start

### Requirements

- **Build environment**: Go 1.24+ (only needed when compiling)
- **Runtime**: Linux/Unix system
- **Optional dependencies**: Nginx, Docker, Redis, Java 17

### Option 1: Single-File Deploy (Recommended) ⭐

The simplest way to get started — just upload one binary!

```bash
# 1. Build the Linux binary locally (Windows users)
build.bat

# Or with Make (Linux/Mac users)
make linux

# 2. Upload to your server (just one file!)
scp bin/ruoyi-proxy-linux user@server:/opt/ruoyi-proxy/

# 3. SSH into the server and make it executable
ssh user@server
cd /opt/ruoyi-proxy
chmod +x ruoyi-proxy-linux

# 4. Start CLI (defaults to AI Agent mode)
./ruoyi-proxy-linux cli

# 5. Run the initialization wizard
/init
```

The init wizard will guide you through:
- ✅ Installing Nginx (required)
- 🐳 Installing Docker (optional)
- 📦 Installing Redis (optional)
- ☕ Installing Java 17 (optional)
- ⚙️ Configuring the proxy (ports, target addresses)
- 🌐 Setting up domain and HTTPS
- 🔄 Configuring file sync (optional)

### Option 2: Local Development

```bash
# Clone the repo
git clone https://github.com/xuantiandaozun/ruoyi-proxy.git
cd ruoyi-proxy

# Install dependencies
make install

# Run in dev mode
make run

# Or build then run
make build
./bin/ruoyi-proxy

# Start the interactive CLI
make cli
```

### Quick Test

#### Using the AI Agent CLI (recommended)

```bash
# Start CLI (Agent mode by default)
./bin/ruoyi-proxy cli

# Slash commands (operations)
/status            # View service status
/deploy            # Blue-green deployment
/deploy-lowmem     # Low-memory deployment
/switch            # Switch environments
/logs              # View logs
/help              # Show help
/commands          # Command list
/exit              # Exit

# Natural language (requires /agent-config first)
Check if nginx is running properly
Switch traffic to the green environment
```

#### Using the HTTP API

```bash
# Check status
curl http://localhost:8001/status

# Switch to green environment
curl -X POST "http://localhost:8001/switch?env=green"

# Health check
curl http://localhost:8001/health
```

---

## 📁 Project Structure

```
ruoyi-proxy/
├── cmd/
│   ├── proxy/          # Program entry point
│   └── prepare-embed/  # Hub/Spoke embed config before build
├── internal/
│   ├── agent/          # AI Agent module (ReAct engine, tools, LLM adapters)
│   ├── cli/            # Interactive CLI (Agent-first entry)
│   ├── config/         # Configuration management
│   ├── hub/            # Hub AI gateway (registration, forwarding, Spoke mgmt)
│   ├── proxy/          # Reverse proxy core
│   ├── handler/        # (planned, not yet implemented)
│   └── sync/           # (planned, not yet implemented)
├── configs/            # Config templates and examples
│   ├── app_config.example.json   # App config example (ai, hub, jvm)
│   ├── nginx.conf.template       # Nginx HTTP template
│   └── nginx-https.conf.template # Nginx HTTPS template
├── scripts/            # Shell scripts (embedded at build time)
│   ├── init.sh         # Initialization script
│   ├── service.sh      # Service management (includes deploy-lowmem)
│   ├── https.sh        # HTTPS management
│   └── deploy.sh       # Deployment script
├── bin/                # Build output directory
├── Makefile            # Make build script
├── build.bat           # Windows build script
├── AGENTS.md           # AI coding agent guidelines
└── README.md           # Project documentation
```

---

## ⚙️ Configuration

### Application Config (configs/app_config.json)

Generated at runtime; see `configs/app_config.example.json` for reference:

```json
{
  "domain": "api.example.com",
  "enable_https": false,
  "hub": {
    "enabled": false
  },
  "ai": {
    "provider": "openai",
    "api_key": "sk-your-key-here",
    "base_url": "https://api.openai.com/v1",
    "model": "gpt-4o-mini",
    "max_tokens": 4096,
    "context_limit": 24000,
    "timeout_seconds": 120
  },
  "proxy": {
    "blue_target": "http://127.0.0.1:8080",
    "green_target": "http://127.0.0.1:8081",
    "active_env": "blue",
    "proxy_port": "8000"
  },
  "ssl": {
    "email": "admin@example.com",
    "cert_path": "/etc/nginx/cert"
  },
  "nginx": {
    "config_path": "/etc/nginx/conf.d/ruoyi.conf",
    "html_path": "/etc/nginx/html"
  },
  "jvm": {
    "preset": 2
  }
}
```

> AI config is stored under the `ai` key in `configs/app_config.json` and edited via `/agent-config`.

### Proxy Config (configs/proxy_config.json)

```json
{
  "blue_target": "http://127.0.0.1:8080",
  "green_target": "http://127.0.0.1:8081",
  "active_env": "blue"
}
```

---

## 📖 Usage Guide

### Build Commands

**Windows:**
```cmd
build.bat                    # Build standard Linux binary
build.bat linux-hub          # Build Hub package (set AI config in configs/app_config.json first)
set HUB_URL=https://hub.example.com && build.bat linux-spoke   # Build Spoke package
```

**Linux/Mac:**
```bash
make build              # Build for current platform
make linux              # Build standard Linux binary
make linux-hub          # Hub package: embeds AI config + hub.enabled=true
make linux-spoke HUB_URL=https://hub.example.com  # Spoke package: embeds Hub URL, no API key
make run                # Run proxy in dev mode
make cli                # Start CLI (Agent mode)
make install            # Install dependencies
make clean              # Clean build artifacts
```

> 💡 **Hub/Spoke builds**: Write AI config in `configs/app_config.json` before `linux-hub`, or set `HUB_URL` for `linux-spoke`. Hub deploys ready to chat; Spoke registers via `/agent-config` with a one-time token.

> 💡 **Tip**: After modifying `scripts/` or `configs/`, you must rebuild (files are embedded at compile time).

### Agent Slash Commands

`make cli` or `./ruoyi-proxy-linux cli` **defaults to AI Agent mode**. Ops commands use `/xxx` syntax; type `/` to open the command menu (↑/↓ to select).

```bash
# Session management
/sessions          # List saved sessions
/load <id>         # Load a session
/new               # New session
/current           # Current session info

# Service management
/start             # Start Java application
/stop              # Stop Java application
/restart           # Restart Java application
/status            # View service status
/detail            # Detailed status (with health check)
/logs              # View logs
/logs-follow       # Follow logs in real time

# Blue-green deployment
/deploy            # Standard blue-green (both envs running)
/deploy-lowmem     # Low-memory deploy (stop old, start new)
/switch            # Switch blue/green environment

# Proxy service
/proxy-start       # Start proxy service
/proxy-stop        # Stop proxy service
/proxy-status      # View proxy status

# Multi-service management
/service-list      # List all services
/service-switch    # Switch active service

# AI & Hub
/agent-config      # Configure AI provider / Spoke registration
/hub-token         # (Hub) Generate Spoke registration token
/hub-status        # (Hub) List Spokes
/hub-spoke <id>    # (Hub) View a Spoke
/hub-enable        # Enable Hub gateway (requires proxy restart)
/hub-disable       # Disable Hub gateway
/hub-revoke <id>   # (Hub) Revoke a Spoke
/self-check        # Run environment self-check
/fix-nginx-hub     # Ask AI to fix Nginx Hub routing

# Config & system
/config            # View full configuration
/init              # Full initialization wizard
/help              # Show help
/commands          # Command list
/cls               # Clear screen
/exit              # Exit
```

> Slash ops commands work even without AI configured; add AI via `/agent-config` for natural language tasks.

### Management API

#### Check Status
```bash
curl http://localhost:8001/status
```

**Response example:**
```json
{
  "status": "running",
  "active_env": "blue",
  "blue_target": "http://127.0.0.1:8080",
  "green_target": "http://127.0.0.1:8081",
  "proxy_port": ":8000",
  "mgmt_port": ":8001",
  "time": "2024-12-11 17:30:00",
  "version": "1.0.0"
}
```

#### Switch Environment
```bash
curl -X POST "http://localhost:8001/switch?env=green"
curl -X POST "http://localhost:8001/switch?env=blue"
```

#### Update Config
```bash
curl -X POST http://localhost:8001/config \
  -H "Content-Type: application/json" \
  -d '{
    "blue_target": "http://127.0.0.1:8080",
    "green_target": "http://127.0.0.1:8081",
    "active_env": "blue"
  }'
```

### HTTPS Setup

#### Request an SSL Certificate

```bash
# Domain must already point to this server
/cert example.com
```

Certificates are saved to `/etc/nginx/cert/`.

#### Enable HTTPS

```bash
/enable-https
```

This automatically:
1. ✅ Checks for an existing SSL certificate
2. ✅ Switches Nginx config to the HTTPS version
3. ✅ Configures HTTP → HTTPS redirect
4. ✅ Updates `app_config.json`
5. ✅ Reloads Nginx

#### Disable HTTPS

```bash
/disable-https
```

#### Certificate Renewal

Let's Encrypt certificates are valid for 90 days. Renew manually:

```bash
/cert example.com
```

Or set up auto-renewal via crontab:

```bash
# Auto-renew on the 1st of every month at 2 AM
0 2 1 * * /opt/ruoyi-proxy/scripts/https.sh example.com >> /var/log/cert-renewal.log 2>&1
```

---

## 🤖 AI Agent Mode

`cli` **launches directly into AI Agent mode** — no need to run a separate `agent` command. The Agent uses a built-in ReAct (Reason + Act) engine that automatically plans steps, calls tools, observes results, and iterates until the task is complete.

### Architecture Overview

```
User (natural language / slash commands) → AI Agent (reasoning engine) → Tool calls → Server
                                    ↑
                          LLM API (Claude / OpenAI / Hub relay / local model)
```

The Agent adapts to each node's role and actual environment — **it does not assume** every server runs blue-green proxy:

| Scenario | Agent behavior |
|----------|----------------|
| **Blue-green mode** | Proxy on :8000 or user confirms → use deploy/switch/proxy-status |
| **General ops mode** | Node/Python/Docker/single-instance → probe processes/ports/containers/systemd first |
| **Hub node** | Gateway duties (tokens, Spoke list, AI relay) separate from local service ops |
| **Spoke node** | Often AI ops CLI only; proxy not running ≠ failure; asks user when unsure |

Typical blue-green stack (**only when this server uses it**):

```
External request → Nginx(:80/:443) → Ruoyi Proxy(:8000) → Java apps(:8080/:8081)
```

For non-standard projects, the Agent detects project type and can adapt custom `service-*.sh` control scripts.

### Quick Start

```bash
# Start CLI (Agent mode)
./ruoyi-proxy-linux cli

# Configure AI provider (first time)
/agent-config

# Natural language
You: Show me the nginx config file
You: Change the timeout in /etc/nginx/conf.d/default.conf to 60s
You: Install htop and check current system load

# Or slash commands
/status
/deploy
```

### Configuring the AI Provider

Run `/agent-config` and fill in the following:

| Field | Description | Example |
|-------|-------------|---------|
| **provider** | LLM provider type | `anthropic` / `openai` / `hub` |
| **api_key** | API key (not needed for Hub mode) | `sk-...` |
| **base_url** | API endpoint (Hub URL for Spoke mode) | `https://api.deepseek.com` |
| **model** | Model to use | `claude-opus-4-5` |
| **max_tokens** | Max output tokens | `8096` |
| **timeout** | Request timeout (seconds) | `120` |

**Supported providers:**

```bash
# Anthropic Claude (recommended)
provider:  anthropic
base_url:  https://api.anthropic.com
model:     claude-opus-4-5

# OpenAI
provider:  openai
base_url:  https://api.openai.com/v1
model:     gpt-4o

# DeepSeek (OpenAI-compatible)
provider:  openai
base_url:  https://api.deepseek.com/v1
model:     deepseek-chat

# Ollama (local model)
provider:  openai
base_url:  http://localhost:11434/v1
model:     qwen2.5:14b
api_key:   ollama

# Hub mode (Spoke node)
provider:  hub
base_url:  https://your-hub.example.com
# Enter one-time registration token from Hub /hub-token on first run
```

Config is saved under the `ai` key in `configs/app_config.json` and loaded automatically on next start.

### Available Tools

The Agent can call these tools automatically:

#### 📂 File Operations

| Tool | Description | Confirmation required |
|------|-------------|:--------------------:|
| `read_file` | Read file contents | ❌ |
| `list_directory` | List directory contents | ❌ |
| `write_file` | Write / modify a file | ✅ |
| `delete_file` | Delete files (supports batch) | ✅ |

**Auto-backup**: When modifying or deleting important files, they are automatically backed up to `~/.ruoyi-backup/<timestamp>/`. Backed-up extensions:

`.conf` `.cfg` `.json` `.yaml` `.yml` `.xml` `.properties` `.env` `.ini` `.toml` `.sh` `.pem` `.crt` `.key`

#### ⚙️ System & Services

| Tool | Description | Confirmation required |
|------|-------------|:--------------------:|
| `get_status` | Service status (adaptive health checks by project type; proxy down is not auto-failure) | ❌ |
| `get_config` | View proxy and service config | ❌ |
| `get_logs` | Read service logs | ❌ |
| `get_system_info` | CPU / memory / disk info | ❌ |
| `run_shell` | Execute a shell command | ✅ (read-only cmds skipped) |
| `install_package` | Install packages (auto-detects distro) | ✅ |
| `manage_systemd` | Manage systemd services | ✅ |
| `systemd_info` | Query service status | ❌ |

**Auto-detected package managers**: apt-get → dnf → yum → pacman → apk → zypper

**Read-only commands skip confirmation automatically**:  
`ls`, `cat`, `pwd`, `echo`, `df`, `du`, `ps`, `top`, `free`, `uname`, `whoami`, `id`, `date`, `uptime`, `netstat`, `ss`, `ip`, `nginx -t`, `systemctl status`, `journalctl`, etc.

### Confirmation Flow

Write operations display a confirmation box:

```
╭──────────────────────────────────────────╮
│ ⚠  Write operation pending               │
│ Tool: write_file                         │
│ Args: {"path":"/etc/nginx/nginx.conf"}   │
│                                          │
│ Enter y to confirm, anything else cancels│
╰──────────────────────────────────────────╯
```

**Single-turn approval**: Once you confirm in a turn, all subsequent tool calls in the same turn are automatically approved — no repeated prompts.

**Pre-authorization**: If your message itself is a clear affirmative ("yes", "ok", "confirm", "go ahead", etc.), the confirmation box is skipped entirely.

### Environment Self-Check (`/self-check`)

| Node | Scope |
|------|-------|
| **Hub** | Base env, :8000/:8001 gateway, AI config, `/__hub__/` Nginx route |
| **Spoke** | Base env, Hub connectivity; :8000 proxy skipped if not listening (optional) |
| **Default** | Base env; full service health checks done by Agent based on deployment mode |

On Spoke or non-blue-green servers, ask naturally e.g. "self-check this server" — the Agent judges whether blue-green proxy applies, then uses systemd/docker/port probes as appropriate.

### Usage Examples

```bash
# Check service health
🤖 You: Is nginx running properly? Show me the recent error logs

# Modify a config file
🤖 You: Set proxy_read_timeout to 120 in /etc/nginx/conf.d/ruoyi.conf

# Batch delete (one confirmation covers all)
🤖 You: Delete these temp files: /tmp/a.log /tmp/b.log /tmp/c.log

# Install software
🤖 You: Install vim and htop

# Deployment (blue-green mode)
🤖 You: Check the health of both blue and green environments, then switch traffic to the healthy one

# General ops (non-blue-green project)
🤖 You: Self-check this server and tell me what services are actually running
```

### Auto-Resume

For complex tasks, the Agent runs up to **30 reasoning rounds**. If still incomplete, it automatically injects a continuation message and keeps going — up to **5 auto-resumes**, giving a total of **150 effective rounds** to handle long-running tasks without interruption.

---

## 🌐 Hub AI Gateway

For multi-server operations, use the Hub/Spoke architecture to centralize AI credentials: Hub holds the API key and forwards AI requests; Spokes execute tools locally without storing secrets.

### Architecture

```
Spoke server A ──┐
Spoke server B ──┼──► Hub (central AI key) ──► LLM API
Spoke server C ──┘         ▲
                           │
                    Tools still run on each Spoke
```

### Deployment Steps

**1. Build Hub package**

```bash
# Write ai section (with api_key) in configs/app_config.json first
make linux-hub
# Output: bin/ruoyi-proxy-linux-hub
```

Hub package embeds AI config with `hub.enabled=true`; deploy and chat via `/agent-config`.

**2. Build Spoke package**

```bash
make linux-spoke HUB_URL=https://your-hub.example.com
# Output: bin/ruoyi-proxy-linux-spoke
```

Spoke package embeds Hub URL only — no API key.

**3. Generate registration token on Hub**

```bash
/hub-token
# Or via management API: curl http://localhost:8001/hub/token
```

**4. Register on Spoke**

```bash
/agent-config
# Choose provider=hub, enter Hub URL and one-time token
```

### API Endpoints

| Endpoint | Port | Description |
|----------|------|-------------|
| `/__hub__/v1/token` | 8000 (proxy) | Generate one-time registration token |
| `/__hub__/v1/register` | 8000 (proxy) | Spoke registration |
| `/__hub__/v1/chat` | 8000 (proxy) | AI chat relay (v1 non-streaming) |
| `/hub/token` | 8001 (mgmt) | Admin token generation |
| `/hub/status` | 8001 (mgmt) | Spoke list |
| `/hub/revoke` | 8001 (mgmt) | Revoke Spoke |

> Hub requires Nginx `location ^~ /__hub__/` routing to the proxy port. Use `/self-check` or `/fix-nginx-hub` for diagnostics and AI-assisted fixes.
>
> **Note**: Hub and Spoke business servers may not use blue-green proxy. Spokes often run ruoyi-proxy as an AI ops CLI only. The Agent reads `spoke_profile.json` (Spoke) or `/hub-status` profiles (Hub) for project type, and self-checks adapt to the real environment — it does not default to proxy-status or `/actuator/health`.

---

## 🔄 Deployment Workflow

### Blue-Green Deployment Steps

```
┌──────────────────┐
│ 1. Prepare release│  Blue env (8080) keeps serving
└────────┬─────────┘  Deploy new version to green (8081)
         │
         ▼
┌──────────────────┐
│ 2. Test new build │  curl http://localhost:8081/health
└────────┬─────────┘
         │
         ▼
┌──────────────────┐
│ 3. Switch traffic │  curl -X POST "http://localhost:8001/switch?env=green"
└────────┬─────────┘  All traffic now routes to green
         │
         ▼
┌──────────────────┐
│ 4. Verify         │  curl http://localhost:8001/status
└────────┬─────────┘  Confirm switch succeeded
         │
         ▼
┌──────────────────┐
│ 5. Rollback (opt) │  curl -X POST "http://localhost:8001/switch?env=blue"
└──────────────────┘  Revert instantly if issues arise
```

### CLI-Based Deployment

```bash
./ruoyi-proxy-linux cli

# Standard blue-green (zero downtime, both envs running)
/deploy

# Low-memory deploy (stop old, start new — for small VPS)
/deploy-lowmem

# Or step by step
/status          # Check current state
/switch          # Route traffic
/status          # Confirm the switch
```

### Production Recommendations

#### 1. Manage with systemd

```bash
sudo systemctl start ruoyi-proxy
sudo systemctl enable ruoyi-proxy    # Auto-start on boot
sudo systemctl status ruoyi-proxy
```

#### 2. Monitoring and Alerts

```bash
# Periodic health check
*/5 * * * * curl -sf http://localhost:8001/health || echo "Service down" | mail -s "Alert" admin@example.com

# Log monitoring
tail -f /var/log/ruoyi-proxy.log
```

#### 3. Config Backup

```bash
0 2 * * * tar -czf /backup/ruoyi-proxy-config-$(date +\%Y\%m\%d).tar.gz /opt/ruoyi-proxy/configs/
```

---

## ❓ Troubleshooting

### Port Already in Use

**Problem**: Startup fails with "port already in use"

**Solution**:
```bash
netstat -tlnp | grep 8000
/config          # View/edit configuration
```

### Config Changes Not Taking Effect

**Problem**: Edits to config files are ignored

**Solution**:
```bash
rm configs/*.json
/init
```

### Build Failures

**Problem**: Dependency errors during compilation

**Solution**:
```bash
make clean
go mod tidy
make build
```

### Proxy Fails to Start

**Problem**: `/proxy-start` returns an error

**Solution**:
```bash
make build                    # Ensure binary is compiled
netstat -tlnp | grep 8001     # Check port availability
/logs                         # View detailed logs
```

### HTTPS Certificate Request Fails

**Problem**: Let's Encrypt certificate request fails

**Solution**:
1. Make sure the domain resolves to this server's IP
2. Ensure port 80 is accessible from the internet
3. Check that Nginx is running
4. Review the error output for details

---

## 🏗️ Architecture

### System Architecture

```
┌─────────────┐
│   Client    │
└──────┬──────┘
       │
       ▼
┌─────────────┐
│    Nginx    │  :80 / :443
│   (proxy)   │
└──────┬──────┘
       │
       ▼
┌─────────────┐
│ Ruoyi Proxy │  :8000 (proxy port)
│             │  :8001 (management port)
└──────┬──────┘
       │
       ├─────────────┬─────────────┐
       ▼             ▼             ▼
┌──────────┐  ┌──────────┐  ┌──────────┐
│   Blue   │  │  Green   │  │  Other   │
│  :8080   │  │  :8081   │  │  :808x   │
└──────────┘  └──────────┘  └──────────┘
```

### Module Responsibilities

| Module | Role | Description |
|--------|------|-------------|
| **cmd/proxy** | Entry point | Starts all services, initializes config |
| **internal/config** | Config management | Load, save, validate config files |
| **internal/proxy** | Reverse proxy | Core proxy logic and blue-green switching |
| **internal/hub** | Hub gateway | Spoke registration, AI relay, token management |
| **internal/cli** | CLI interface | Agent-first entry, slash command dispatch |
| **internal/agent** | AI operations | ReAct engine, tools, LLM adapters |

### Data Flow

#### Proxy Request
```
Client → Nginx(:80) → Proxy(:8000) → Blue/Green environment → Response
```

#### Environment Switch
```
Admin → CLI/API(:8001) → Validate env → Update config → Save file → Return result
```

### Design Principles

1. **Single responsibility** - Each package has one clearly defined purpose
2. **Dependency injection** - Dependencies passed as parameters for testability
3. **Config-driven** - All settings managed through files
4. **Concurrency safety** - `atomic.Value` ensures thread safety
5. **Error handling** - Comprehensive logging and error propagation

### Performance Optimizations

- **Connection pooling**: Configured `http.Transport` with connection reuse
- **Timeout control**: Appropriate read/write timeouts to prevent resource leaks
- **Concurrent processing**: Goroutines for independent tasks
- **Memory efficiency**: Disabled unnecessary compression to reduce CPU usage

### Security Recommendations

1. ✅ **Use HTTPS** - SSL certificates are mandatory in production
2. ✅ **Restrict port access** - Firewall the management port (8001)
3. ✅ **Audit logs** - Log all management operations
4. ✅ **Least privilege** - Minimize file and directory permissions
5. ✅ **Regular backups** - Back up config files and certificates periodically

---

## 🤝 Contributing

Contributions, bug reports, and feature suggestions are welcome!

### How to Contribute

1. Fork this repository
2. Create a feature branch (`git checkout -b feature/AmazingFeature`)
3. Commit your changes (`git commit -m 'Add some AmazingFeature'`)
4. Push to the branch (`git push origin feature/AmazingFeature`)
5. Open a Pull Request

### Reporting Issues

Found a bug or have a feature request? [Open an issue](https://github.com/xuantiandaozun/ruoyi-proxy/issues).

---

## 📄 License

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.

---

## 🙏 Acknowledgements

Thanks to everyone who has contributed to this project!

---

<div align="center">

**If this project helps you, please give it a ⭐️ Star!**

Made with ❤️ by [xuantiandaozun](https://github.com/xuantiandaozun)

</div>
