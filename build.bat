@echo off
setlocal

echo 同步最新脚本...
xcopy /E /I /Y scripts cmd\proxy\scripts >nul
xcopy /E /I /Y configs cmd\proxy\configs >nul

set LDFLAGS=-s -w
set BUILD_PKG=ruoyi-proxy/internal/buildinfo

if /i "%1"=="linux-hub" (
    echo 准备 Hub 嵌入配置...
    go run ./cmd/prepare-embed -profile=hub -src=configs/app_config.json -dst=cmd/proxy/configs/app_config.json
    if %errorlevel% neq 0 exit /b 1
    echo 编译 Linux Hub 版本...
    set GOOS=linux
    set GOARCH=amd64
    go build -ldflags "%LDFLAGS% -X %BUILD_PKG%.Profile=hub" -o bin\ruoyi-proxy-linux-hub cmd\proxy\main.go
    goto :done
)

if /i "%1"=="linux-spoke" (
    echo 准备 Spoke 嵌入配置...
    go run ./cmd/prepare-embed -profile=spoke -hub-url=%HUB_URL% -src=configs/app_config.json -dst=cmd/proxy/configs/app_config.json
    if %errorlevel% neq 0 exit /b 1
    echo 编译 Linux Spoke 版本...
    set GOOS=linux
    set GOARCH=amd64
    go build -ldflags "%LDFLAGS% -X %BUILD_PKG%.Profile=spoke" -o bin\ruoyi-proxy-linux-spoke cmd\proxy\main.go
    goto :done
)

if /i "%1"=="linux" (
    echo 编译 Linux 版本...
    set GOOS=linux
    set GOARCH=amd64
    go build -ldflags "%LDFLAGS%" -o bin\ruoyi-proxy-linux cmd\proxy\main.go
    goto :done
)

echo 编译 Windows 版本...
set GOOS=windows
set GOARCH=amd64
go build -ldflags "%LDFLAGS%" -o bin\ruoyi-proxy.exe cmd\proxy\main.go

:done
if %errorlevel% neq 0 (
    echo 编译失败
    exit /b 1
)
echo.
echo 编译成功
