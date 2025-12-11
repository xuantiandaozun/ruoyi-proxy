#!/bin/bash

# 修复默认服务 JAR 匹配模式的部署脚本

echo "=== 修复默认服务 JAR 匹配模式 ==="
echo ""

# 1. 停止所有服务
echo "1. 停止所有服务..."
./bin/ruoyi-cli stop

echo ""

# 2. 删除旧的配置文件
echo "2. 删除旧的配置文件..."
if [ -f "configs/proxy_config.json" ]; then
    echo "   备份旧配置到 configs/proxy_config.json.bak"
    cp configs/proxy_config.json configs/proxy_config.json.bak
    rm -f configs/proxy_config.json
    echo "   ✓ 已删除旧配置文件"
else
    echo "   ℹ 配置文件不存在，跳过"
fi

echo ""

# 3. 启动代理程序（会自动生成新配置）
echo "3. 启动代理程序（自动生成新配置）..."
./bin/ruoyi-cli proxy-start

echo ""

# 4. 查看新配置
echo "4. 查看新生成的配置..."
if [ -f "configs/proxy_config.json" ]; then
    echo "   ──────────────────────────────────────"
    cat configs/proxy_config.json
    echo ""
    echo "   ──────────────────────────────────────"
else
    echo "   ✗ 配置文件未生成"
fi

echo ""

# 5. 验证 JAR 匹配模式
echo "5. 验证 JAR 匹配模式..."
if [ -f "configs/proxy_config.json" ]; then
    jar_pattern=$(grep -o '"jar_file":[^,]*' configs/proxy_config.json | head -1)
    echo "   当前默认服务 JAR 模式: $jar_pattern"
    
    if echo "$jar_pattern" | grep -q "ruoyi-\[0-9\]\[0-9\]\[0-9\]\[0-9\]\[0-9\]\[0-9\]\[0-9\]\[0-9\]"; then
        echo "   ✓ JAR 匹配模式正确！"
    else
        echo "   ✗ JAR 匹配模式不正确，可能需要重新编译"
    fi
fi

echo ""
echo "=== 修复完成 ==="
echo ""
echo "下一步："
echo "  1. 如果有 collect 服务，请重新添加："
echo "     ./bin/ruoyi-cli service-add"
echo "  2. 重新部署默认服务："
echo "     ./bin/ruoyi-cli deploy"
