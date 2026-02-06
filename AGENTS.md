# AGENTS.md - AI Coding Agent Guidelines

## Project Overview

This is **ruoyi-proxy** - a blue-green deployment proxy server for managing Java applications with zero-downtime deployments, HTTPS auto-configuration, and multi-service management.

- **Language**: Go (version 1.24+)
- **Module**: `ruoyi-proxy`
- **Main Entry**: `cmd/proxy/main.go`

## Build Commands

```bash
# Build for current platform (auto-syncs scripts/configs)
make build

# Build Linux version (for deployment)
make linux

# Run in development mode
make run

# Run interactive CLI mode
make cli

# Install dependencies
go mod tidy
go mod download

# Clean build artifacts
make clean

# Format code
go fmt ./...
```

## Testing

**No test files exist yet.** To run tests if they are added:

```bash
# Run all tests
go test -v ./...

# Run single test
go test -v -run TestFunctionName ./path/to/package
```

## Code Style Guidelines

### General Go Conventions

- Follow [Effective Go](https://golang.org/doc/effective_go.html)
- Use `gofmt` to format all code
- Line length: keep under 120 characters when possible
- Use tabs for indentation (Go standard)

### Naming Conventions

- **Exported (public)**: PascalCase (e.g., `New`, `LoadConfig`, `ServiceConfig`)
- **Unexported (private)**: camelCase (e.g., `createProxy`, `loadProxyConfig`)
- **Constants**: PascalCase for exported, camelCase for unexported
- **Interfaces**: `-er` suffix when appropriate (e.g., `Reader`, `Writer`)
- **Acronyms**: Keep uppercase (e.g., `HTTPS`, `URL`, `ID`)

### Imports

```go
import (
    // Standard library packages first
    "encoding/json"
    "fmt"
    "os"
    
    // Third-party packages second
    "github.com/chzyer/readline"
    "golang.org/x/term"
    
    // Internal/project packages last
    "ruoyi-proxy/internal/config"
    "ruoyi-proxy/internal/proxy"
)
```

- Group imports: stdlib, external, internal
- Use blank identifier imports only when necessary
- Use dot imports sparingly and only in test files

### Types and Structs

```go
// ServiceConfig 单个服务配置
type ServiceConfig struct {
    Name        string `json:"name"`         // 服务名称（显示用）
    BlueTarget  string `json:"blue_target"`  // 蓝色环境地址
    GreenTarget string `json:"green_target"` // 绿色环境地址
    ActiveEnv   string `json:"active_env"`   // 当前活跃环境 blue/green
}
```

- Add Chinese comments for exported types and fields
- Use JSON tags for all struct fields that are serialized
- Keep struct definitions in dedicated files (e.g., `config.go`)

### Error Handling

```go
// Always check errors explicitly
if err != nil {
    return fmt.Errorf("operation failed: %v", err)
}

// Wrap errors with context
return nil, fmt.Errorf("创建服务[%s]蓝色代理失败: %v", serviceID, err)

// Log errors with context
log.Printf("代理错误: %v, URL: %s", err, r.URL.String())
```

- Never ignore errors (avoid `_ = someFunc()`)
- Wrap errors with meaningful context using `fmt.Errorf`
- Use `log.Printf` for logging (not `println`)
- Fatal only in `main()` or initialization: `log.Fatalf("初始化失败: %v", err)`

### Concurrency

```go
type Proxy struct {
    mu       sync.RWMutex
    config   *config.Config
    services map[string]*ServiceProxy
}

// Readers use RLock
func (p *Proxy) GetConfig() *config.Config {
    p.mu.RLock()
    defer p.mu.RUnlock()
    return p.config
}

// Writers use Lock
func (p *Proxy) UpdateConfig(cfg *config.Config) error {
    p.mu.Lock()
    defer p.mu.Unlock()
    // ... modify state
}
```

- Use `sync.RWMutex` for protecting shared state
- Always defer unlocks: `defer p.mu.Unlock()`
- Minimize critical sections

### File Organization

```
ruoyi-proxy/
├── cmd/
│   └── proxy/
│       ├── main.go              # Entry point with embed directives
│       ├── scripts/             # Embedded shell scripts (auto-synced)
│       └── configs/             # Embedded config templates (auto-synced)
├── internal/
│   ├── cli/                     # Interactive CLI commands
│   │   ├── cli.go              # Main CLI logic
│   │   ├── commands.go         # Command implementations
│   │   ├── config.go           # Config management
│   │   └── embed.go            # Embedded file handling
│   ├── config/                  # Configuration types and loading
│   │   └── config.go
│   └── proxy/                   # Reverse proxy logic
│       └── proxy.go
├── scripts/                     # Source shell scripts (development)
├── configs/                     # Source configs (development)
└── bin/                         # Build output
```

### Comments

- Use Chinese comments for business logic
- Use Go doc comments for exported functions/types
- Format: `// FunctionName 功能描述`

```go
// LoadConfig 加载代理配置文件
func LoadConfig() (*Config, error) {
    // ...
}

// ServiceConfig 单个服务配置
type ServiceConfig struct {
    Name string `json:"name"` // 服务名称（显示用）
}
```

### Constants

```go
const (
    ConfigFile = "configs/proxy_config.json"
    ProxyPort  = ":8000" // 代理监听端口
    MgmtPort   = ":8001" // 管理接口端口
)
```

### HTTP Handlers

```go
func (p *Proxy) HandleProxy(w http.ResponseWriter, r *http.Request) {
    // Set headers
    r.Header.Set("X-Proxy-Service", serviceID)
    r.Header.Set("X-Proxy-Env", svcCfg.ActiveEnv)
    r.Header.Set("X-Proxy-Time", time.Now().Format("2006-01-02 15:04:05"))
    
    // Delegate to reverse proxy
    proxy.ServeHTTP(w, r)
}
```

### Shell Script Integration

The project embeds shell scripts using `//go:embed`:

```go
//go:embed scripts/*
var scriptsFS embed.FS

//go:embed configs/*
var configsFS embed.FS
```

When modifying scripts in `scripts/` or `configs/`, run `make sync` before building to copy them to `cmd/proxy/`.

## Commit Message Format

Follow conventional commits:

- `feat:` 新功能
- `fix:` Bug 修复
- `docs:` 文档更新
- `style:` 代码格式调整
- `refactor:` 代码重构
- `test:` 测试相关
- `chore:` 构建/工具相关

## Important Notes

1. **Always run `make sync` before building** if you modified scripts or configs
2. **Scripts are embedded** - changes to `scripts/` or `configs/` require rebuild
3. **No tests exist yet** - add tests to `*_test.go` files when implementing
4. **Use Chinese comments** for business logic to match project style
5. **Platform support**: Windows (dev), Linux (deployment target)
