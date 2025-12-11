# Ruoyi Proxy - 蓝绿部署代理服务器

<div align="center">

[![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat&logo=go)](https://golang.org)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Platform](https://img.shields.io/badge/Platform-Linux%20%7C%20Windows-lightgrey)](https://github.com)

一个功能完整的蓝绿部署代理服务器，支持零停机部署、HTTPS自动配置、多服务管理和文件同步。

[功能特性](#-功能特性) • [快速开始](#-快速开始) • [使用指南](#-使用指南) • [架构设计](#-架构设计) • [贡献指南](#-贡献)

</div>

---

## 📋 目录

- [功能特性](#-功能特性)
- [快速开始](#-快速开始)
- [项目结构](#-项目结构)
- [配置说明](#-配置说明)
- [使用指南](#-使用指南)
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

# 4. 启动交互式 CLI
./ruoyi-proxy-linux cli

# 5. 执行初始化向导
ruoyi> init
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

#### 使用交互式 CLI（推荐）

```bash
# 启动 CLI
./bin/ruoyi-proxy cli

# 常用命令
ruoyi> status      # 查看服务状态
ruoyi> deploy      # 执行蓝绿部署
ruoyi> switch      # 切换环境
ruoyi> logs        # 查看日志
ruoyi> help        # 查看所有命令
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
│   └── proxy/          # 程序入口
├── internal/
│   ├── cli/            # 交互式 CLI
│   ├── config/         # 配置管理
│   ├── handler/        # HTTP 处理器
│   ├── proxy/          # 反向代理核心
│   └── sync/           # 文件同步
├── configs/            # 配置文件目录
│   ├── app_config.json           # 应用配置
│   ├── proxy_config.json         # 代理配置
│   ├── nginx.conf.template       # Nginx HTTP 模板
│   └── nginx-https.conf.template # Nginx HTTPS 模板
├── scripts/            # Shell 脚本（会被嵌入）
│   ├── init.sh         # 初始化脚本
│   ├── service.sh      # 服务管理
│   ├── https.sh        # HTTPS 管理
│   ├── deploy.sh       # 部署脚本
│   └── sync.sh         # 文件同步
├── bin/                # 编译输出目录
├── Makefile            # Make 构建脚本
├── build.bat           # Windows 构建脚本
├── go.mod              # Go 模块定义
└── README.md           # 项目文档
```

---

## ⚙️ 配置说明

### 应用配置 (configs/app_config.json)

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
build.bat               # 编译 Linux 版本（推荐）
```

**Linux/Mac 用户：**
```bash
make build              # 编译当前平台
make linux              # 编译 Linux 版本
make run                # 开发模式运行
make cli                # 启动交互式 CLI
make install            # 安装依赖
make clean              # 清理编译文件
```

> 💡 **提示**：修改 `scripts/` 目录下的脚本后，必须重新编译才能生效。

### 交互式 CLI 命令

```bash
# 服务管理
ruoyi> start           # 启动 Java 应用
ruoyi> stop            # 停止 Java 应用
ruoyi> restart         # 重启 Java 应用
ruoyi> status          # 查看服务状态
ruoyi> detail          # 详细状态（含健康检查）

# 蓝绿部署
ruoyi> deploy          # 执行蓝绿部署
ruoyi> switch          # 交互式切换环境
ruoyi> rollback        # 回滚到上一个环境

# 代理服务
ruoyi> proxy-start     # 启动代理服务
ruoyi> proxy-stop      # 停止代理服务
ruoyi> proxy-status    # 查看代理状态

# 多服务管理
ruoyi> service-add     # 添加新服务
ruoyi> service-list    # 查看服务列表
ruoyi> service-switch  # 切换当前服务
ruoyi> service-remove  # 删除服务

# HTTPS 管理
ruoyi> cert <域名>     # 申请 SSL 证书
ruoyi> enable-https    # 开启 HTTPS
ruoyi> disable-https   # 关闭 HTTPS

# 配置管理
ruoyi> config          # 查看完整配置
ruoyi> config-edit     # 编辑配置

# 文件同步
ruoyi> sync-config     # 配置文件同步
ruoyi> sync-status     # 查看同步状态

# 系统管理
ruoyi> init            # 完整初始化
ruoyi> logs            # 查看日志
ruoyi> monitor         # 实时监控
ruoyi> help            # 查看所有命令
ruoyi> exit            # 退出 CLI
```

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
# 在 CLI 中申请证书（域名需已解析到服务器）
ruoyi> cert example.com
```

证书会自动保存到 `/etc/nginx/cert/` 目录。

#### 开启 HTTPS

```bash
ruoyi> enable-https
```

**自动完成的操作：**
1. ✅ 检查 SSL 证书是否存在
2. ✅ 切换 Nginx 配置到 HTTPS 版本
3. ✅ 配置 HTTP 到 HTTPS 重定向
4. ✅ 更新 `app_config.json` 配置
5. ✅ 重载 Nginx 服务

#### 关闭 HTTPS

```bash
ruoyi> disable-https
```

#### 证书续期

Let's Encrypt 证书有效期为 90 天，可以手动续期：

```bash
ruoyi> cert example.com
```

或配置自动续期（crontab）：

```bash
# 每月 1 号凌晨 2 点自动续期
0 2 1 * * /opt/ruoyi-proxy/scripts/https.sh example.com >> /var/log/cert-renewal.log 2>&1
```

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
# 启动 CLI
./ruoyi-proxy-linux cli

# 执行部署（自动化整个流程）
ruoyi> deploy

# 或手动控制每一步
ruoyi> status          # 查看当前状态
ruoyi> switch green    # 切换到绿色环境
ruoyi> status          # 验证切换结果
ruoyi> switch blue     # 如需回滚
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
ruoyi> config-edit
```

### 配置文件不生效

**问题**：修改配置后没有生效

**解决方案**：
```bash
# 删除旧配置，重新初始化
rm configs/*.json
ruoyi> init
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

**问题**：执行 `proxy-start` 失败

**解决方案**：
```bash
# 检查是否已编译
make build

# 检查端口是否被占用
netstat -tlnp | grep 8001

# 查看详细日志
ruoyi> logs
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
| **internal/handler** | HTTP 处理 | 管理 API 接口处理 |
| **internal/cli** | CLI 管理 | 交互式命令行界面 |
| **internal/sync** | 文件同步 | 主从服务器文件同步 |

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
