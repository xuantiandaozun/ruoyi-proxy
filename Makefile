.PHONY: build linux run clean install cli sync

# 同步脚本和配置到 cmd/proxy/（编译前必须执行）
sync:
	@echo "同步最新脚本和配置..."
	@cp -r scripts cmd/proxy/ 2>/dev/null || xcopy /E /I /Y scripts cmd\proxy\scripts >nul
	@cp -r configs cmd/proxy/ 2>/dev/null || xcopy /E /I /Y configs cmd\proxy\configs >nul
	@echo "✓ 同步完成"

# 编译程序（自动同步）
build: sync
	go build -o bin/ruoyi-proxy cmd/proxy/main.go

# 编译 Linux 版本（脚本已内嵌，自动同步）
linux: sync
	GOOS=linux GOARCH=amd64 go build -o bin/ruoyi-proxy-linux cmd/proxy/main.go
	@echo ""
	@echo "✓ 编译完成: bin/ruoyi-proxy-linux"
	@echo "提示: 脚本和配置已内嵌，只需上传这一个文件"
	@echo ""
	@echo "部署命令:"
	@echo "  scp bin/ruoyi-proxy-linux user@server:/opt/ruoyi-proxy/"
	@echo "  ssh user@server"
	@echo "  cd /opt/ruoyi-proxy"
	@echo "  chmod +x ruoyi-proxy-linux"
	@echo "  ./ruoyi-proxy-linux cli"

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
