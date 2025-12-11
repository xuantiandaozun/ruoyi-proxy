@echo off
setlocal

echo 同步最新脚本...
xcopy /E /I /Y scripts cmd\proxy\scripts >nul
xcopy /E /I /Y configs cmd\proxy\configs >nul

if /i "%1"=="linux" (
    echo 编译 Linux 版本...
    set GOOS=linux
    set GOARCH=amd64
    go build -o bin\ruoyi-proxy-linux cmd\proxy\main.go
    if %errorlevel% equ 0 (
        echo.
        echo 编译成功: bin\ruoyi-proxy-linux
        echo 提示: 最新脚本已嵌入，上传到服务器即可使用
    )
) else (
    echo 编译 Windows 版本...
    set GOOS=windows
    set GOARCH=amd64
    go build -o bin\ruoyi-proxy.exe cmd\proxy\main.go
    if %errorlevel% equ 0 (
        echo.
        echo 编译成功: bin\ruoyi-proxy.exe
        echo 提示: 运行 bin\ruoyi-proxy.exe 启动服务
    )
)

if %errorlevel% neq 0 (
    echo 编译失败
    exit /b 1
)
