.PHONY: build linux run clean install cli sync linux-hub linux-spoke build-hub build-spoke

LDFLAGS = -s -w
BUILD_PKG = ruoyi-proxy/internal/buildinfo

# 同步脚本和配置到 cmd/proxy/（编译前必须执行）
sync:
	@echo "同步最新脚本和配置..."
	@cp -r scripts cmd/proxy/ 2>/dev/null || xcopy /E /I /Y scripts cmd\proxy\scripts >nul
	@cp -r configs cmd/proxy/ 2>/dev/null || xcopy /E /I /Y configs cmd\proxy\configs >nul
	@echo "✓ 同步完成"

# 编译程序（自动同步）
build: sync
	go build -ldflags "$(LDFLAGS)" -o bin/ruoyi-proxy cmd/proxy/main.go

# 编译 Linux 版本（脚本已内嵌，自动同步）
linux: sync
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o bin/ruoyi-proxy-linux cmd/proxy/main.go
	@echo ""
	@echo "✓ 编译完成: bin/ruoyi-proxy-linux"

# Hub 节点：嵌入完整 AI 密钥 + 启用 Hub 网关
linux-hub: sync
	go run ./cmd/prepare-embed -profile=hub
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS) -X $(BUILD_PKG).Profile=hub" -o bin/ruoyi-proxy-linux-hub cmd/proxy/main.go
	@echo ""
	@echo "✓ Hub 包: bin/ruoyi-proxy-linux-hub（已嵌入 AI 配置，hub.enabled=true）"

# Spoke 节点：嵌入 Hub 地址，不含 AI 密钥
linux-spoke: sync
	go run ./cmd/prepare-embed -profile=spoke -hub-url="$(HUB_URL)"
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS) -X $(BUILD_PKG).Profile=spoke" -o bin/ruoyi-proxy-linux-spoke cmd/proxy/main.go
	@echo ""
	@echo "✓ Spoke 包: bin/ruoyi-proxy-linux-spoke（已嵌入 Hub 地址，首次运行 /agent-config 注册 Token）"

build-hub: linux-hub
build-spoke: linux-spoke

# 运行程序
run:
	go run cmd/proxy/main.go

# 运行CLI模式
cli:
	go run cmd/proxy/main.go cli

# 编译并运行CLI
build-cli:
	go build -o bin/ruoyi-proxy cmd/proxy/main.go
	./bin/ruoyi-proxy cli

# 清理编译文件
clean:
	rm -rf bin/
	rm -f configs/proxy_config.json
	rm -f configs/sync_config.json

# 安装依赖
install:
	go mod tidy
	go mod download

# 测试
test:
	go test -v ./...

# 格式化代码
fmt:
	go fmt ./...
