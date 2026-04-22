# Ruoyi Proxy - Blue-Green Deployment Proxy Server

<div align="center">

[![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat&logo=go)](https://golang.org)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Platform](https://img.shields.io/badge/Platform-Linux%20%7C%20Windows-lightgrey)](https://github.com)

A full-featured blue-green deployment proxy server with zero-downtime deployments, automatic HTTPS, multi-service management, file synchronization, and AI-powered operations.

[Features](#-features) • [Quick Start](#-quick-start) • [Usage Guide](#-usage-guide) • [AI Agent Mode](#-ai-agent-mode) • [Architecture](#-architecture) • [Contributing](#-contributing)

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
- **Natural language** - Describe tasks in plain English; AI plans and executes them
- **Multi-LLM support** - Works with Anthropic Claude, OpenAI, and any compatible API (DeepSeek, Qwen, Ollama, etc.)
- **File management** - Read, modify, and delete server files with automatic backup for important files
- **Service management** - Install packages, manage systemd services, run shell commands
- **Safe confirmation** - Write operations require confirmation; one approval covers an entire turn

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

# 4. Start the interactive CLI
./ruoyi-proxy-linux cli

# 5. Run the initialization wizard
ruoyi> init
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

#### Using the interactive CLI (recommended)

```bash
./bin/ruoyi-proxy cli

ruoyi> status      # View service status
ruoyi> deploy      # Run blue-green deployment
ruoyi> switch      # Switch environments
ruoyi> logs        # View logs
ruoyi> help        # Show all commands
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
│   └── proxy/          # Program entry point
├── internal/
│   ├── agent/          # AI Agent operations module
│   │   ├── agent.go    # ReAct reasoning engine
│   │   ├── tools.go    # Tool set (file / service / shell)
│   │   ├── openai.go   # OpenAI-compatible API adapter
│   │   └── anthropic.go# Anthropic API adapter
│   ├── cli/            # Interactive CLI
│   ├── config/         # Configuration management
│   ├── handler/        # HTTP handlers
│   ├── proxy/          # Reverse proxy core
│   └── sync/           # File synchronization
├── configs/            # Configuration files
│   ├── app_config.json           # Application config
│   ├── proxy_config.json         # Proxy config
│   ├── nginx.conf.template       # Nginx HTTP template
│   └── nginx-https.conf.template # Nginx HTTPS template
├── scripts/            # Shell scripts (embedded in binary)
│   ├── init.sh         # Initialization script
│   ├── service.sh      # Service management
│   ├── https.sh        # HTTPS management
│   ├── deploy.sh       # Deployment script
│   └── sync.sh         # File synchronization
├── bin/                # Build output directory
├── Makefile            # Make build script
├── build.bat           # Windows build script
├── go.mod              # Go module definition
└── README.md           # Project documentation
```

---

## ⚙️ Configuration

### Application Config (configs/app_config.json)

```json
{
  "domain": "api.example.com",
  "enable_https": false,
  "email": "admin@example.com",
  "proxy": {
    "proxy_port": ":8000",
    "mgmt_port": ":8001",
    "blue_port": ":8080",
    "green_port": ":8081"
  },
  "nginx": {
    "config_path": "/etc/nginx/conf.d/ruoyi.conf",
    "cert_path": "/etc/nginx/cert",
    "html_path": "/etc/nginx/html"
  },
  "sync": {
    "enabled": false,
    "role": "master",
    "remote_host": "",
    "remote_user": "",
    "remote_path": ""
  }
}
```

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
build.bat               # Build Linux binary (recommended)
```

**Linux/Mac:**
```bash
make build              # Build for current platform
make linux              # Build Linux binary
make run                # Run in dev mode
make cli                # Start interactive CLI
make install            # Install dependencies
make clean              # Clean build artifacts
```

> 💡 **Tip**: After modifying scripts in `scripts/`, you must rebuild for changes to take effect.

### CLI Commands

```bash
# Service management
ruoyi> start           # Start Java application
ruoyi> stop            # Stop Java application
ruoyi> restart         # Restart Java application
ruoyi> status          # View service status
ruoyi> detail          # Detailed status (includes health check)

# Blue-green deployment
ruoyi> deploy          # Run blue-green deployment
ruoyi> switch          # Interactive environment switch
ruoyi> rollback        # Roll back to previous environment

# Proxy service
ruoyi> proxy-start     # Start proxy service
ruoyi> proxy-stop      # Stop proxy service
ruoyi> proxy-status    # View proxy status

# Multi-service management
ruoyi> service-add     # Add a new service
ruoyi> service-list    # List all services
ruoyi> service-switch  # Switch active service
ruoyi> service-remove  # Remove a service

# HTTPS management
ruoyi> cert <domain>   # Request SSL certificate
ruoyi> enable-https    # Enable HTTPS
ruoyi> disable-https   # Disable HTTPS

# Configuration
ruoyi> config          # View full configuration
ruoyi> config-edit     # Edit configuration

# File synchronization
ruoyi> sync-config     # Configure file sync
ruoyi> sync-status     # View sync status

# AI Agent
ruoyi> agent           # Enter AI conversation mode
ruoyi> agent-config    # Configure AI provider and model

# System
ruoyi> init            # Full initialization wizard
ruoyi> logs            # View logs
ruoyi> monitor         # Real-time monitoring
ruoyi> help            # Show all commands
ruoyi> exit            # Exit CLI
```

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
ruoyi> cert example.com
```

Certificates are saved to `/etc/nginx/cert/`.

#### Enable HTTPS

```bash
ruoyi> enable-https
```

This automatically:
1. ✅ Checks for an existing SSL certificate
2. ✅ Switches Nginx config to the HTTPS version
3. ✅ Configures HTTP → HTTPS redirect
4. ✅ Updates `app_config.json`
5. ✅ Reloads Nginx

#### Disable HTTPS

```bash
ruoyi> disable-https
```

#### Certificate Renewal

Let's Encrypt certificates are valid for 90 days. Renew manually:

```bash
ruoyi> cert example.com
```

Or set up auto-renewal via crontab:

```bash
# Auto-renew on the 1st of every month at 2 AM
0 2 1 * * /opt/ruoyi-proxy/scripts/https.sh example.com >> /var/log/cert-renewal.log 2>&1
```

---

## 🤖 AI Agent Mode

AI Agent mode lets you interact with your server in plain English — no need to memorize commands. The Agent uses a built-in ReAct (Reason + Act) engine that automatically plans steps, calls tools, observes results, and iterates until the task is complete.

### Architecture Overview

```
User (natural language) → AI Agent (reasoning engine) → Tool calls → Server
                                    ↑
                          LLM API (Claude / OpenAI / local model)
```

The Agent understands the real server architecture:

```
External request → Nginx(:80/:443) → Ruoyi Proxy(:8000) → Java apps(:8080/:8081)
```

### Quick Start

```bash
# Step 1: Configure your AI provider
ruoyi> agent-config

# Step 2: Enter Agent mode
ruoyi> agent

# Start chatting
🤖 You: Show me the nginx config file
🤖 You: Change the timeout in /etc/nginx/conf.d/default.conf to 60s
🤖 You: Install htop and check current system load
```

### Configuring the AI Provider

Run `agent-config` and fill in the following:

| Field | Description | Example |
|-------|-------------|---------|
| **provider** | LLM provider type | `anthropic` / `openai` |
| **api_key** | API key | `sk-...` |
| **base_url** | API endpoint (optional) | `https://api.deepseek.com` |
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
base_url:  https://api.openai.com
model:     gpt-4o

# DeepSeek (OpenAI-compatible)
provider:  openai
base_url:  https://api.deepseek.com
model:     deepseek-chat

# Ollama (local model)
provider:  openai
base_url:  http://localhost:11434/v1
model:     qwen2.5:14b
api_key:   ollama
```

Config is saved to `~/.ruoyi-agent.json` and loaded automatically on next start.

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

#### ⚙️ System Management

| Tool | Description | Confirmation required |
|------|-------------|:--------------------:|
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

# Deployment
🤖 You: Check the health of both blue and green environments, then switch traffic to the healthy one
```

### Auto-Resume

For complex tasks, the Agent runs up to **30 reasoning rounds**. If still incomplete, it automatically injects a continuation message and keeps going — up to **5 auto-resumes**, giving a total of **150 effective rounds** to handle long-running tasks without interruption.

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

ruoyi> deploy          # Run the full automated deployment

# Or control each step manually
ruoyi> status          # Check current state
ruoyi> switch green    # Route traffic to green
ruoyi> status          # Confirm the switch
ruoyi> switch blue     # Rollback if needed
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
ruoyi> config-edit   # Change the port in config
```

### Config Changes Not Taking Effect

**Problem**: Edits to config files are ignored

**Solution**:
```bash
rm configs/*.json
ruoyi> init
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

**Problem**: `proxy-start` returns an error

**Solution**:
```bash
make build                    # Ensure binary is compiled
netstat -tlnp | grep 8001     # Check port availability
ruoyi> logs                   # View detailed logs
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
| **internal/handler** | HTTP handlers | Management API endpoints |
| **internal/cli** | CLI interface | Interactive command-line interface |
| **internal/sync** | File sync | Primary/replica server file synchronization |
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
