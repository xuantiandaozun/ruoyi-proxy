# Ruoyi Proxy - 蓝绿部署代理服务器

<div align="center">

[![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat&logo=go)](https://golang.org)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Platform](https://img.shields.io/badge/Platform-Linux%20%7C%20Windows-lightgrey)](https://github.com)

一个功能完整的蓝绿部署代理服务器，支持零停机部署、HTTPS自动配置、多服务管理、文件同步和 AI 智能运维。

[功能特性](#-功能特性) • [快速开始](#-快速开始) • [使用指南](#-使用指南) • [AI Agent 模式](#-ai-agent-模式) • [Hub AI 网关](#-hub-ai-网关) • [架构设计](#-架构设计) • [贡献指南](#-贡献)

**[🇺🇸 English Documentation](README.md)**

</div>

---

## 📋 目录

- [功能特性](#-功能特性)
- [快速开始](#-快速开始)
- [项目结构](#-项目结构)
- [配置说明](#-配置说明)
- [使用指南](#-使用指南)
- [AI Agent 模式](#-ai-agent-模式)
- [Hub AI 网关](#-hub-ai-网关)
- [部署流程](#-部署流程)
- [常见问题](#-常见问题)
- [架构设计](#-架构设计)
- [贡献指南](#-贡献)
- [许可证](#-许可证)

---

## ✨ 功能特性

### 🔄 蓝绿部署
- **零停机部署** - 蓝绿环境无缝切换，用户无感知
- **一键回滚** - 出现问题立即切换回旧版本
- **健康检查** - 自动检测服务健康状态
- **长连接支持** - 支持 SSE、WebSocket 等长时间请求

### 🎯 多服务管理
- **多服务支持** - 管理多个 Java 应用服务
- **服务切换** - 快速切换当前操作的服务
- **独立配置** - 每个服务独立的端口和 JAR 文件配置
- **批量操作** - 支持批量启动、停止、部署

### 🔐 HTTPS 自动化
- **自动申请证书** - 集成 Let's Encrypt，一键申请免费 SSL 证书
- **自动续期** - 支持证书自动续期配置
- **一键开关** - HTTPS 模式一键开启/关闭
- **HTTP 重定向** - 自动配置 HTTP 到 HTTPS 重定向

### 🔧 交互式 CLI
- **友好界面** - 彩色输出，命令提示，操作简单
- **实时监控** - 实时查看服务状态和日志
- **交互确认** - 危险操作需要确认，避免误操作
- **命令补全** - 支持 Tab 键命令补全

### 🌐 Nginx 集成
- **自动配置** - 自动生成 Nginx 配置文件
- **静态文件服务** - 支持 Vue 等前端应用托管
- **反向代理** - 自动配置反向代理规则
- **配置模板** - 提供 HTTP 和 HTTPS 配置模板

### 📦 单文件部署
- **脚本内嵌** - 所有脚本和配置模板内嵌到可执行文件
- **一键部署** - 只需上传一个文件即可完成部署
- **跨平台编译** - 支持 Windows 编译 Linux 版本

### 🤖 AI Agent 智能运维
- **默认入口** - `cli` 启动后直接进入 AI Agent 对话，运维命令用 `/xxx` 斜杠命令调用
- **自然语言运维** - 用中文描述任务，AI 自动规划并执行
- **多 LLM 支持** - 兼容 Anthropic Claude、OpenAI 及任意兼容 API（DeepSeek、Qwen、Ollama 等）
- **会话管理** - 多会话持久化，支持 `/sessions`、`/load`、`/new` 切换历史对话
- **文件管理** - 读取、修改、删除服务器文件，重要文件自动备份
- **服务管理** - 安装软件包、管理 systemd 服务、执行 Shell 命令
- **安全确认** - 写操作前展示确认框，同一轮操作只需确认一次

### 🌐 Hub AI 网关（多服务器）
- **集中密钥** - Hub 节点持有 AI API Key，多台 Spoke 服务器无需各自配置密钥
- **Token 注册** - Hub 生成一次性注册 Token，Spoke 通过 `/agent-config` 完成注册
- **本地执行** - AI 推理经 Hub 转发，工具调用仍在 Spoke 本机执行
- **预编译包** - `make linux-hub` / `make linux-spoke` 分别打包 Hub 与 Spoke 节点

### 💾 低内存部署
- **内存友好** - `/deploy-lowmem` 先停止旧服务再启动新版本，无需同时运行蓝绿双环境
- **适用场景** - 小内存 VPS（如 1G/2G）无法做标准蓝绿部署时使用

---

## 🚀 快速开始

### 环境要求

- **编译环境**：Go 1.24+（仅编译时需要）
- **运行环境**：Linux/Unix 系统
- **可选依赖**：Nginx、Docker、Redis、Java 17

### 方式一：单文件部署（推荐）⭐

这是最简单的部署方式，只需上传一个可执行文件！

```bash
# 1. 在本地编译 Linux 版本（Windows 用户）
build.bat

# 或者使用 Make（Linux/Mac 用户）
make linux

# 2. 上传到服务器（只需一个文件！）
scp bin/ruoyi-proxy-linux user@server:/opt/ruoyi-proxy/

# 3. SSH 到服务器并添加执行权限
ssh user@server
cd /opt/ruoyi-proxy
chmod +x ruoyi-proxy-linux

# 4. 启动 CLI（默认进入 AI Agent 模式）
./ruoyi-proxy-linux cli

# 5. 执行初始化向导
/init
```

初始化向导会引导你完成以下配置：
- ✅ 安装 Nginx（必需）
- 🐳 安装 Docker（可选）
- 📦 安装 Redis（可选）
- ☕ 安装 Java 17（可选）
- ⚙️ 配置代理程序（端口、目标地址）
- 🌐 配置域名和 HTTPS
- 🔄 配置文件同步（可选）

### 方式二：本地开发

```bash
# 克隆项目
git clone https://github.com/xuantiandaozun/ruoyi-proxy.git
cd ruoyi-proxy

# 安装依赖
make install

# 开发模式运行
make run

# 或编译后运行
make build
./bin/ruoyi-proxy

# 启动交互式 CLI
make cli
```

### 快速测试

#### 使用 AI Agent CLI（推荐）

```bash
# 启动 CLI（默认进入 Agent 模式）
./bin/ruoyi-proxy cli

# 斜杠命令（运维）
/status            # 查看服务状态
/deploy            # 蓝绿部署
/deploy-lowmem     # 低内存部署
/switch            # 切换环境
/logs              # 查看日志
/help              # 查看说明
/commands          # 命令列表
/exit              # 退出

# 自然语言（需先 /agent-config 配置 AI）
查看 nginx 是否正常运行
帮我把流量切到绿色环境
```

#### 使用 HTTP API

```bash
# 查看状态
curl http://localhost:8001/status

# 切换到绿色环境
curl -X POST "http://localhost:8001/switch?env=green"

# 健康检查
curl http://localhost:8001/health
```

---

## 📁 项目结构

```
ruoyi-proxy/
├── cmd/
│   ├── proxy/          # 程序入口
│   └── prepare-embed/  # Hub/Spoke 打包前配置嵌入
├── internal/
│   ├── agent/          # AI Agent 运维模块（ReAct 引擎、工具、LLM 适配）
│   ├── cli/            # 交互式 CLI（Agent 为主入口）
│   ├── config/         # 配置管理
│   ├── hub/            # Hub AI 网关（注册、转发、Spoke 管理）
│   ├── proxy/          # 反向代理核心
│   ├── handler/        # （规划中，暂无代码）
│   └── sync/           # （规划中，暂无代码）
├── configs/            # 配置模板与示例
│   ├── app_config.example.json   # 应用配置示例（含 ai、hub、jvm）
│   ├── nginx.conf.template       # Nginx HTTP 模板
│   └── nginx-https.conf.template # Nginx HTTPS 模板
├── scripts/            # Shell 脚本（编译时嵌入二进制）
│   ├── init.sh         # 初始化脚本
│   ├── service.sh      # 服务管理（含 deploy-lowmem）
│   ├── https.sh        # HTTPS 管理
│   └── deploy.sh       # 部署脚本
├── bin/                # 编译输出目录
├── Makefile            # Make 构建脚本
├── build.bat           # Windows 构建脚本
├── AGENTS.md           # AI 编码助手开发指南
└── README.md           # 项目文档
```

---

## ⚙️ 配置说明

### 应用配置 (configs/app_config.json)

运行时生成，可参考 `configs/app_config.example.json`：

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

> AI 配置保存在 `configs/app_config.json` 的 `ai` 字段，通过 `/agent-config` 交互式修改。

### 代理配置 (configs/proxy_config.json)

```json
{
  "blue_target": "http://127.0.0.1:8080",
  "green_target": "http://127.0.0.1:8081",
  "active_env": "blue"
}
```

---

## 📖 使用指南

### 编译命令

**Windows 用户：**
```cmd
build.bat                    # 编译 Linux 标准包
build.bat linux-hub          # 编译 Hub 包（需先在 configs/app_config.json 写好 AI 配置）
set HUB_URL=https://hub.example.com && build.bat linux-spoke   # 编译 Spoke 包
```

**Linux/Mac 用户：**
```bash
make build              # 编译当前平台
make linux              # 编译 Linux 标准包
make linux-hub          # Hub 包：嵌入 AI 配置 + hub.enabled=true
make linux-spoke HUB_URL=https://hub.example.com  # Spoke 包：嵌入 Hub 地址，不含密钥
make run                # 开发模式运行代理
make cli                # 启动 CLI（Agent 模式）
make install            # 安装依赖
make clean              # 清理编译文件
```

> 💡 **Hub/Spoke 打包**：打包前在 `configs/app_config.json` 写好 AI 配置（Hub）或设置 `HUB_URL`（Spoke）。Hub 包部署后可直接对话；Spoke 包首次运行用 `/agent-config` 填一次性注册 Token。

> 💡 **提示**：修改 `scripts/` 或 `configs/` 目录后，必须重新编译才能生效（脚本与配置在编译时嵌入二进制）。

### Agent 斜杠命令

`make cli` 或 `./ruoyi-proxy-linux cli` 启动后**默认进入 AI Agent 模式**。运维命令以 `/xxx` 形式调用；输入 `/` 可打开命令菜单（↑/↓ 选择）。

```bash
# 会话管理
/sessions          # 查看历史会话
/load <id>         # 加载历史会话
/new               # 新建会话
/current           # 当前会话信息

# 服务管理
/start             # 启动 Java 应用
/stop              # 停止 Java 应用
/restart           # 重启 Java 应用
/status            # 查看服务状态
/detail            # 详细状态（含健康检查）
/logs              # 查看日志
/logs-follow       # 实时日志

# 蓝绿部署
/deploy            # 标准蓝绿部署（双环境并行）
/deploy-lowmem     # 低内存部署（先停旧再启新）
/switch            # 切换蓝绿环境

# 代理服务
/proxy-start       # 启动代理服务
/proxy-stop        # 停止代理服务
/proxy-status      # 查看代理状态

# 多服务管理
/service-list      # 查看服务列表
/service-switch    # 切换当前服务

# AI 与 Hub
/agent-config      # 配置 AI 提供商 / Spoke 注册
/hub-token         # （Hub）生成 Spoke 注册 Token
/hub-status        # （Hub）查看 Spoke 列表
/hub-spoke <id>    # （Hub）查看单个 Spoke
/hub-enable        # 启用 Hub 网关（需重启代理）
/hub-disable       # 禁用 Hub 网关
/hub-revoke <id>   # （Hub）吊销 Spoke
/self-check        # 运行环境自检
/fix-nginx-hub     # 让 AI 修复 Nginx Hub 路由

# 配置与系统
/config            # 查看完整配置
/init              # 完整初始化向导
/help              # 查看说明
/commands          # 命令列表
/cls               # 清屏
/exit              # 退出
```

> 未配置 AI 时，斜杠运维命令仍可直接使用；配置 AI 后还可自然语言描述需求。

### 管理 API

#### 查看状态
```bash
curl http://localhost:8001/status
```

**响应示例：**
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

#### 切换环境
```bash
# 切换到绿色环境
curl -X POST "http://localhost:8001/switch?env=green"

# 切换到蓝色环境
curl -X POST "http://localhost:8001/switch?env=blue"
```

#### 更新配置
```bash
curl -X POST http://localhost:8001/config \
  -H "Content-Type: application/json" \
  -d '{
    "blue_target": "http://127.0.0.1:8080",
    "green_target": "http://127.0.0.1:8081",
    "active_env": "blue"
  }'
```

### HTTPS 配置

#### 申请 SSL 证书

```bash
# 在 Agent 中申请证书（域名需已解析到服务器）
/cert example.com
```

证书会自动保存到 `/etc/nginx/cert/` 目录。

#### 开启 HTTPS

```bash
/enable-https
```

**自动完成的操作：**
1. ✅ 检查 SSL 证书是否存在
2. ✅ 切换 Nginx 配置到 HTTPS 版本
3. ✅ 配置 HTTP 到 HTTPS 重定向
4. ✅ 更新 `app_config.json` 配置
5. ✅ 重载 Nginx 服务

#### 关闭 HTTPS

```bash
/disable-https
```

#### 证书续期

Let's Encrypt 证书有效期为 90 天，可以手动续期：

```bash
/cert example.com
```

```bash
# 每月 1 号凌晨 2 点自动续期
0 2 1 * * /opt/ruoyi-proxy/scripts/https.sh example.com >> /var/log/cert-renewal.log 2>&1
```

---

## 🤖 AI Agent 模式

`cli` 启动后**默认进入 AI Agent 模式**，无需再输入 `agent` 命令。Agent 内置 ReAct（Reason + Act）推理引擎，会自动规划步骤、调用工具、观察结果，直到完成任务。

### 架构总览

```
用户（自然语言 / 斜杠命令） → AI Agent（推理引擎）→ 工具调用 → 服务器操作
                                    ↑
                          LLM API（Claude / OpenAI / Hub 转发 / 本地模型）
```

Agent 了解当前服务器的真实架构：

```
外部请求 → Nginx(:80/:443) → Ruoyi Proxy(:8000) → Java 应用(:8080/:8081)
```

### 快速开始

```bash
# 启动 CLI（直接进入 Agent）
./ruoyi-proxy-linux cli

# 配置 AI 提供商（首次使用）
/agent-config

# 自然语言对话
You: 查看一下 nginx 配置文件
You: 帮我把 /etc/nginx/conf.d/default.conf 的超时时间改成 60s
You: 安装 htop 并检查当前系统负载

# 或用斜杠命令
/status
/deploy
```

### 配置 AI 提供商

执行 `/agent-config` 后，按提示填写以下信息：

| 参数 | 说明 | 示例 |
|------|------|------|
| **provider** | LLM 提供商类型 | `anthropic` / `openai` / `hub` |
| **api_key** | API 密钥（Hub 模式无需填写） | `sk-...` |
| **base_url** | API 地址（Hub 模式填 Hub 地址） | `https://api.deepseek.com` |
| **model** | 使用的模型 | `claude-opus-4-5` |
| **max_tokens** | 最大输出 Token | `8096` |
| **timeout** | 请求超时（秒） | `120` |

**支持的 LLM 提供商：**

```bash
# Anthropic Claude（推荐）
provider:  anthropic
base_url:  https://api.anthropic.com
model:     claude-opus-4-5

# OpenAI
provider:  openai
base_url:  https://api.openai.com/v1
model:     gpt-4o

# DeepSeek（OpenAI 兼容）
provider:  openai
base_url:  https://api.deepseek.com/v1
model:     deepseek-chat

# Ollama 本地模型
provider:  openai
base_url:  http://localhost:11434/v1
model:     qwen2.5:14b
api_key:   ollama

# Hub 模式（Spoke 节点）
provider:  hub
base_url:  https://your-hub.example.com
# 首次运行填入 Hub 上 /hub-token 生成的一次性注册 Token
```

配置保存在 `configs/app_config.json` 的 `ai` 字段，下次启动自动加载。

### 内置工具列表

Agent 可以调用以下工具（无需手动操作）：

#### 📂 文件操作

| 工具 | 说明 | 是否需要确认 |
|------|------|:----------:|
| `read_file` | 读取文件内容 | ❌ |
| `list_directory` | 列出目录内容 | ❌ |
| `write_file` | 写入/修改文件 | ✅ |
| `delete_file` | 删除文件（支持批量） | ✅ |

**自动备份**：修改或删除以下类型的重要文件时，会自动备份到 `~/.ruoyi-backup/<时间戳>/`：

`.conf` `.cfg` `.json` `.yaml` `.yml` `.xml` `.properties` `.env` `.ini` `.toml` `.sh` `.pem` `.crt` `.key`

#### ⚙️ 系统管理

| 工具 | 说明 | 是否需要确认 |
|------|------|:----------:|
| `run_shell` | 执行 Shell 命令 | ✅（查询命令自动跳过）|
| `install_package` | 安装软件包（自动识别发行版） | ✅ |
| `manage_systemd` | 管理 systemd 服务 | ✅ |
| `systemd_info` | 查询服务状态 | ❌ |

**自动识别包管理器**：apt-get → dnf → yum → pacman → apk → zypper，无需关心 Linux 发行版差异。

**只读命令自动放行**（不弹确认框）：
`ls`、`cat`、`pwd`、`echo`、`df`、`du`、`ps`、`top`、`free`、`uname`、`whoami`、`id`、`date`、`uptime`、`netstat`、`ss`、`ip`、`nginx -t`、`systemctl status`、`journalctl` 等。

### 确认机制

为防止误操作，写操作会弹出确认框：

```
╭──────────────────────────────────────╮
│ ⚠  即将执行写操作                     │
│ 工具: write_file                      │
│ 参数: {"path":"/etc/nginx/nginx.conf"}│
│                                      │
│ 输入 y 确认，其他取消                  │
╰──────────────────────────────────────╯
```

**一轮确认机制**：同一次对话中，一旦用户确认，后续工具调用自动获得授权，无需重复确认。

**提前授权**：如果用户消息本身就是明确的确认意图（如「确认」「好的」「执行」「ok」等），将自动跳过确认框。

### 使用示例

```bash
# 查看服务状态
🤖 你: 帮我看看 nginx 服务是否正常，并显示最近的错误日志

# 修改配置文件
🤖 你: 把 /etc/nginx/conf.d/ruoyi.conf 的 proxy_read_timeout 改成 120

# 批量删除
🤖 你: 删掉这些临时文件：/tmp/a.log /tmp/b.log /tmp/c.log
     # ← 只需确认一次，Agent 批量处理

# 安装软件
🤖 你: 安装 vim 和 htop

# 部署操作
🤖 你: 帮我检查蓝绿两个环境的健康状态，然后把流量切到健康的那个
```

### 自动续跑

对于复杂任务，Agent 最多执行 **30 轮推理**，若仍未完成会自动注入续跑消息继续工作（最多续跑 5 次），合计最高 **150 轮**，确保长任务不中断。

---

## 🌐 Hub AI 网关

多台服务器运维时，可用 Hub/Spoke 架构集中管理 AI 密钥：Hub 持有 API Key 并转发 AI 请求，Spoke 在本机执行工具，无需每台服务器各自配置密钥。

### 架构

```
Spoke 服务器 A ──┐
Spoke 服务器 B ──┼──► Hub（集中 AI 密钥）──► LLM API
Spoke 服务器 C ──┘         ▲
                           │
                    工具仍在 Spoke 本地执行
```

### 部署步骤

**1. 打包 Hub 节点**

```bash
# 本地写好 configs/app_config.json 中的 ai 段（含 api_key）
make linux-hub
# 输出: bin/ruoyi-proxy-linux-hub
```

Hub 包已嵌入 AI 配置且 `hub.enabled=true`，部署后可直接 `/agent-config` 对话。

**2. 打包 Spoke 节点**

```bash
make linux-spoke HUB_URL=https://your-hub.example.com
# 输出: bin/ruoyi-proxy-linux-spoke
```

Spoke 包只嵌入 Hub 地址，不含 API Key。

**3. Hub 上生成注册 Token**

```bash
/hub-token
# 或通过管理 API: curl http://localhost:8001/hub/token
```

**4. Spoke 上注册**

```bash
/agent-config
# 选择 provider=hub，填入 Hub 地址与一次性 Token
```

### API 端点

| 端点 | 端口 | 说明 |
|------|------|------|
| `/__hub__/v1/token` | 8000（代理） | 生成一次性注册 Token |
| `/__hub__/v1/register` | 8000（代理） | Spoke 注册 |
| `/__hub__/v1/chat` | 8000（代理） | AI 聊天转发（v1 非流式） |
| `/hub/token` | 8001（管理） | 管理端生成 Token |
| `/hub/status` | 8001（管理） | Spoke 列表 |
| `/hub/revoke` | 8001（管理） | 吊销 Spoke |

> Hub 需在 Nginx 配置 `location ^~ /__hub__/` 转发到代理端口；可用 `/self-check` 自检，或用 `/fix-nginx-hub` 让 AI 辅助修复。

---

## 🔄 部署流程

### 蓝绿部署流程

```
┌─────────────┐
│ 1. 准备新版本 │  当前蓝色环境（8080）继续服务
└──────┬──────┘  在绿色环境（8081）部署新版本
       │
       ▼
┌─────────────┐
│ 2. 测试新版本 │  curl http://localhost:8081/health
└──────┬──────┘
       │
       ▼
┌─────────────┐
│ 3. 切换流量  │  curl -X POST "http://localhost:8001/switch?env=green"
└──────┬──────┘  所有流量切换到绿色环境
       │
       ▼
┌─────────────┐
│ 4. 验证服务  │  curl http://localhost:8001/status
└──────┬──────┘  确认切换成功
       │
       ▼
┌─────────────┐
│ 5. 回滚（可选）│ curl -X POST "http://localhost:8001/switch?env=blue"
└─────────────┘  如有问题，立即回滚
```

### 使用 CLI 执行部署

```bash
# 启动 CLI（Agent 模式）
./ruoyi-proxy-linux cli

# 标准蓝绿部署（双环境并行，零停机）
/deploy

# 低内存部署（先停旧再启新，适合小内存机器）
/deploy-lowmem

# 或手动控制
/status          # 查看当前状态
/switch          # 切换环境
/status          # 验证切换结果
```

### 生产部署建议

#### 1. 使用 systemd 管理服务

```bash
# 初始化时会自动创建 systemd 服务
sudo systemctl start ruoyi-proxy
sudo systemctl enable ruoyi-proxy    # 开机自启
sudo systemctl status ruoyi-proxy
```

#### 2. 配置监控和告警

```bash
# 定期健康检查
*/5 * * * * curl -sf http://localhost:8001/health || echo "Service down" | mail -s "Alert" admin@example.com

# 日志监控
tail -f /var/log/ruoyi-proxy.log
```

#### 3. 备份配置文件

```bash
# 定期备份配置
0 2 * * * tar -czf /backup/ruoyi-proxy-config-$(date +\%Y\%m\%d).tar.gz /opt/ruoyi-proxy/configs/
```

---

## ❓ 常见问题

### 端口被占用

**问题**：启动时提示端口已被占用

**解决方案**：
```bash
# 查看端口占用
netstat -tlnp | grep 8000

# 修改配置文件中的端口
/config
```

### 配置文件不生效

**问题**：修改配置后没有生效

**解决方案**：
```bash
# 删除旧配置，重新初始化
rm configs/*.json
/init
```

### 编译失败

**问题**：编译时出现依赖错误

**解决方案**：
```bash
# 清理并重新安装依赖
make clean
go mod tidy
make build
```

### 代理服务无法启动

**问题**：执行 `/proxy-start` 失败

**解决方案**：
```bash
# 检查是否已编译
make build

# 检查端口是否被占用
netstat -tlnp | grep 8001

# 查看详细日志
/logs
```

### HTTPS 证书申请失败

**问题**：申请 Let's Encrypt 证书失败

**解决方案**：
1. 确保域名已正确解析到服务器
2. 确保 80 端口可以被外部访问
3. 检查 Nginx 是否正常运行
4. 查看详细错误日志

---

## 🏗️ 架构设计

### 系统架构

```
┌─────────────┐
│   客户端     │
└──────┬──────┘
       │
       ▼
┌─────────────┐
│   Nginx     │  :80/:443
│  (反向代理)  │
└──────┬──────┘
       │
       ▼
┌─────────────┐
│ Ruoyi Proxy │  :8000 (代理端口)
│             │  :8001 (管理端口)
└──────┬──────┘
       │
       ├─────────────┬─────────────┐
       ▼             ▼             ▼
┌──────────┐  ┌──────────┐  ┌──────────┐
│ 蓝色环境  │  │ 绿色环境  │  │ 其他服务  │
│  :8080   │  │  :8081   │  │  :808x   │
└──────────┘  └──────────┘  └──────────┘
```

### 模块职责

| 模块 | 职责 | 说明 |
|------|------|------|
| **cmd/proxy** | 程序入口 | 启动各个服务，初始化配置 |
| **internal/config** | 配置管理 | 加载、保存、验证配置文件 |
| **internal/proxy** | 反向代理 | 核心代理逻辑，蓝绿切换 |
| **internal/hub** | Hub 网关 | Spoke 注册、AI 请求转发、Token 管理 |
| **internal/cli** | CLI 管理 | Agent 为主入口，斜杠命令调度 |
| **internal/agent** | AI 运维 | ReAct 引擎、工具集、LLM 适配器 |

### 数据流

#### 代理请求流程
```
客户端 → Nginx(:80) → Proxy(:8000) → 蓝色/绿色环境 → 返回响应
```

#### 环境切换流程
```
管理员 → CLI/API(:8001) → 验证环境 → 更新配置 → 保存文件 → 返回结果
```

### 设计原则

1. **单一职责** - 每个包只负责一个明确的功能
2. **依赖注入** - 通过参数传递依赖，便于测试
3. **配置驱动** - 所有配置通过文件管理，易于维护
4. **并发安全** - 使用 `atomic.Value` 保证线程安全
5. **错误处理** - 完善的日志记录和错误返回

### 性能优化

- **连接池**：配置 `http.Transport` 连接池，复用连接
- **超时控制**：合理设置读写超时，避免资源泄漏
- **并发处理**：使用 goroutine 处理独立任务
- **内存优化**：禁用不必要的压缩，减少 CPU 使用

### 安全建议

1. ✅ **使用 HTTPS** - 生产环境必须配置 SSL 证书
2. ✅ **限制端口访问** - 使用防火墙限制管理端口（8001）
3. ✅ **日志审计** - 记录所有管理操作
4. ✅ **权限控制** - 文件和目录权限最小化
5. ✅ **定期备份** - 定期备份配置文件和证书

---

## 🤝 贡献

欢迎贡献代码、报告问题或提出建议！

### 如何贡献

1. Fork 本仓库
2. 创建特性分支 (`git checkout -b feature/AmazingFeature`)
3. 提交更改 (`git commit -m 'Add some AmazingFeature'`)
4. 推送到分支 (`git push origin feature/AmazingFeature`)
5. 开启 Pull Request

### 报告问题

如果你发现了 bug 或有功能建议，请[创建 Issue](https://github.com/xuantiandaozun/ruoyi-proxy/issues)。

---

## 📄 许可证

本项目采用 MIT 许可证。详见 [LICENSE](LICENSE) 文件。

---

## 🙏 致谢

感谢所有为本项目做出贡献的开发者！

---

<div align="center">

**如果这个项目对你有帮助，请给个 ⭐️ Star 支持一下！**

Made with ❤️ by [xuantiandaozun](https://github.com/xuantiandaozun)

</div>
