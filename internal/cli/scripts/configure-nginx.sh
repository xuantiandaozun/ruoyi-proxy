#!/bin/bash
# Nginx配置脚本
# 根据配置文件生成Nginx配置

set -e

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONFIG_FILE="$SCRIPT_DIR/../configs/app_config.json"
ENABLE_HTTPS=${1:-false}

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}配置Nginx${NC}"
echo -e "${BLUE}========================================${NC}"

# 检查配置文件
if [ ! -f "$CONFIG_FILE" ]; then
    echo -e "${RED}错误: 配置文件不存在 $CONFIG_FILE${NC}"
    exit 1
fi

# 读取配置
DOMAIN=$(grep -o '"domain"[[:space:]]*:[[:space:]]*"[^"]*"' "$CONFIG_FILE" | sed 's/.*"domain"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')
PROXY_PORT=$(grep -o '"proxy_port"[[:space:]]*:[[:space:]]*"[^"]*"' "$CONFIG_FILE" | sed 's/.*"proxy_port"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')
HTML_PATH=$(grep -o '"html_path"[[:space:]]*:[[:space:]]*"[^"]*"' "$CONFIG_FILE" | sed 's/.*"html_path"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')
VUE_PATH=$(grep -o '"vue_path"[[:space:]]*:[[:space:]]*"[^"]*"' "$CONFIG_FILE" | sed 's/.*"vue_path"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')
CERT_PATH=$(grep -o '"cert_path"[[:space:]]*:[[:space:]]*"[^"]*"' "$CONFIG_FILE" | sed 's/.*"cert_path"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')

echo -e "${CYAN}域名: $DOMAIN${NC}"
echo -e "${CYAN}代理端口: $PROXY_PORT${NC}"
echo -e "${CYAN}HTTPS: $ENABLE_HTTPS${NC}"

# 创建目录
sudo mkdir -p "$HTML_PATH"
sudo mkdir -p "$VUE_PATH"
sudo mkdir -p "$CERT_PATH"

# 选择模板
if [ "$ENABLE_HTTPS" = "true" ]; then
    TEMPLATE="$SCRIPT_DIR/../configs/nginx-https.conf.template"
else
    TEMPLATE="$SCRIPT_DIR/../configs/nginx.conf.template"
fi

if [ ! -f "$TEMPLATE" ]; then
    echo -e "${RED}错误: 模板文件不存在 $TEMPLATE${NC}"
    exit 1
fi

# 生成配置文件
NGINX_CONF="/etc/nginx/conf.d/ruoyi.conf"
echo -e "${CYAN}生成配置文件: $NGINX_CONF${NC}"

sudo cat "$TEMPLATE" | \
    sed "s|{{DOMAIN}}|$DOMAIN|g" | \
    sed "s|{{PROXY_PORT}}|$PROXY_PORT|g" | \
    sed "s|{{HTML_PATH}}|$HTML_PATH|g" | \
    sed "s|{{VUE_PATH}}|$VUE_PATH|g" | \
    sed "s|{{CERT_PATH}}|$CERT_PATH|g" | \
    sudo tee "$NGINX_CONF" > /dev/null

echo -e "${GREEN}✓ Nginx配置已生成${NC}"

# 测试配置
echo -e "${CYAN}测试Nginx配置...${NC}"
if sudo nginx -t; then
    echo -e "${GREEN}✓ 配置测试通过${NC}"
else
    echo -e "${RED}✗ 配置测试失败${NC}"
    exit 1
fi

# 重载Nginx
echo -e "${CYAN}重载Nginx...${NC}"
if sudo systemctl reload nginx; then
    echo -e "${GREEN}✓ Nginx已重载${NC}"
else
    echo -e "${YELLOW}尝试重启Nginx...${NC}"
    sudo systemctl restart nginx
    echo -e "${GREEN}✓ Nginx已重启${NC}"
fi

echo -e "\n${GREEN}Nginx配置完成！${NC}"
echo -e "${CYAN}访问地址: http://$DOMAIN${NC}"
if [ "$ENABLE_HTTPS" = "true" ]; then
    echo -e "${CYAN}HTTPS地址: https://$DOMAIN${NC}"
fi
echo -e "${CYAN}Vue管理后台: http://$DOMAIN/admin${NC}"
