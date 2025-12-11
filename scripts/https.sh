#!/bin/bash

# 获取脚本所在目录
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONFIG_FILE="$SCRIPT_DIR/../configs/app_config.json"

# 从配置文件读取SSL配置
if [ -f "$CONFIG_FILE" ]; then
    EMAIL=$(grep -o '"email"[[:space:]]*:[[:space:]]*"[^"]*"' "$CONFIG_FILE" | sed 's/.*"email"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')
    CERT_PATH=$(grep -o '"cert_path"[[:space:]]*:[[:space:]]*"[^"]*"' "$CONFIG_FILE" | sed 's/.*"cert_path"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')
    WEBROOT_PATH=$(grep -o '"webroot_path"[[:space:]]*:[[:space:]]*"[^"]*"' "$CONFIG_FILE" | sed 's/.*"webroot_path"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')
    LOG_FILE=$(grep -o '"log_file"[[:space:]]*:[[:space:]]*"[^"]*"' "$CONFIG_FILE" | sed 's/.*"log_file"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/')
else
    # 默认配置
    EMAIL="your-email@example.com"
    CERT_PATH="/etc/nginx/cert"
    WEBROOT_PATH="/etc/nginx/html"
    LOG_FILE="/var/log/cert-renewal.log"
fi

# 日志函数
log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1" | tee -a $LOG_FILE
}

# 检查参数
if [ $# -eq 0 ]; then
    echo "使用方法: $0 domain1.com [domain2.com ...]"
    echo "示例: $0 api1.example.com api.example.com"
    exit 1
fi

# 检查是否以 root 权限运行
if [ "$EUID" -ne 0 ]; then 
    log "请使用 root 权限运行此脚本"
    exit 1
fi

# 确保目录存在
mkdir -p $CERT_PATH
mkdir -p $WEBROOT_PATH

# 检查系统发行版
detect_distro() {
    if [ -f /etc/os-release ]; then
        . /etc/os-release
        echo "$ID"
    elif [ -f /etc/redhat-release ]; then
        echo "rhel"
    elif [ -f /etc/debian_version ]; then
        echo "debian"
    else
        echo "unknown"
    fi
}

# 尝试使用 certbot（主方案）
try_certbot() {
    log "尝试使用 certbot 方案..."
    
    DISTRO=$(detect_distro)
    
    # 尝试安装 certbot
    if [ "$DISTRO" = "rhel" ] || [ "$DISTRO" = "centos" ] || [ "$DISTRO" = "fedora" ]; then
        yum install -y epel-release 2>/dev/null
        yum install -y certbot python3-certbot-nginx 2>/dev/null
    elif [ "$DISTRO" = "debian" ] || [ "$DISTRO" = "ubuntu" ]; then
        apt-get update 2>/dev/null
        apt-get install -y certbot python3-certbot-nginx 2>/dev/null
    fi
    
    # 检查 certbot 是否可用
    if command -v certbot &> /dev/null; then
        log "certbot 已安装，开始申请证书..."
        return 0
    else
        log "certbot 安装失败或不可用"
        return 1
    fi
}

# 使用 acme.sh（备用方案）
use_acme_sh() {
    log "使用 acme.sh 备用方案..."
    
    # 安装 acme.sh
    if [ ! -d "/root/.acme.sh" ]; then
        log "安装 acme.sh..."
        curl -s https://get.acme.sh | sh -s email=$EMAIL
        if [ $? -ne 0 ]; then
            log "acme.sh 安装失败"
            return 1
        fi
        source /root/.acme.sh/acme.sh.env
    fi
    
    return 0
}

# 验证 webroot 路径是否可访问
verify_webroot() {
    local DOMAIN=$1
    local TEST_FILE="$WEBROOT_PATH/.well-known/acme-challenge/test-$(date +%s)"
    
    # 创建测试文件
    mkdir -p "$WEBROOT_PATH/.well-known/acme-challenge"
    echo "test" > "$TEST_FILE"
    
    # 验证文件是否可通过 HTTP 访问
    local TEST_URL="http://$DOMAIN/.well-known/acme-challenge/$(basename $TEST_FILE)"
    local RESPONSE=$(curl -s -o /dev/null -w "%{http_code}" "$TEST_URL" 2>/dev/null)
    
    # 清理测试文件
    rm -f "$TEST_FILE"
    
    if [ "$RESPONSE" = "200" ]; then
        log "webroot 验证成功: $DOMAIN"
        return 0
    else
        log "警告：webroot 验证失败 ($RESPONSE)，请确保 nginx 已启动且 $WEBROOT_PATH 可访问"
        return 1
    fi
}

# 使用 certbot 申请证书（文件验证）
certbot_issue() {
    local DOMAIN=$1
    
    log "使用 certbot 申请证书（webroot 文件验证）: $DOMAIN"
    
    certbot certonly --webroot \
            --webroot-path "$WEBROOT_PATH" \
            --agree-tos \
            --email "$EMAIL" \
            --non-interactive \
            --preferred-challenges http-01 \
            -d "$DOMAIN" 2>/dev/null
    
    if [ -f "/etc/letsencrypt/live/$DOMAIN/fullchain.pem" ] && [ -f "/etc/letsencrypt/live/$DOMAIN/privkey.pem" ]; then
        cp /etc/letsencrypt/live/$DOMAIN/fullchain.pem $CERT_PATH/$DOMAIN.pem
        cp /etc/letsencrypt/live/$DOMAIN/privkey.pem $CERT_PATH/$DOMAIN.key
        chown -R nginx:nginx $CERT_PATH 2>/dev/null
        chmod 600 $CERT_PATH/$DOMAIN.key
        chmod 644 $CERT_PATH/$DOMAIN.pem
        log "证书申请成功（certbot webroot）: $DOMAIN"
        return 0
    else
        log "证书申请失败（certbot）: $DOMAIN"
        return 1
    fi
}

# 使用 acme.sh 申请证书（文件验证）
acme_sh_issue() {
    local DOMAIN=$1
    
    log "使用 acme.sh 申请证书（webroot 文件验证）: $DOMAIN"
    
    /root/.acme.sh/acme.sh --issue -d "$DOMAIN" \
        -w "$WEBROOT_PATH" \
        --preferred-chain "ISRG Root X1" \
        --force 2>/dev/null
    
    if [ $? -eq 0 ]; then
        /root/.acme.sh/acme.sh --install-cert -d "$DOMAIN" \
            --key-file "$CERT_PATH/$DOMAIN.key" \
            --fullchain-file "$CERT_PATH/$DOMAIN.pem" \
            --reloadcmd "nginx -s reload" 2>/dev/null
        
        chmod 600 $CERT_PATH/$DOMAIN.key
        chmod 644 $CERT_PATH/$DOMAIN.pem
        log "证书申请成功（acme.sh webroot）: $DOMAIN"
        return 0
    else
        log "证书申请失败（acme.sh）: $DOMAIN"
        return 1
    fi
}

# 尝试 certbot，失败则使用 acme.sh
if try_certbot; then
    USE_CERTBOT=1
else
    log "certbot 不可用，切换到 acme.sh 备用方案"
    if use_acme_sh; then
        USE_CERTBOT=0
    else
        log "错误：certbot 和 acme.sh 都不可用"
        exit 1
    fi
fi

# 处理每个域名
for DOMAIN in "$@"; do
    log "开始处理域名: $DOMAIN"
    
    # 验证 webroot 路径
    if ! verify_webroot "$DOMAIN"; then
        log "跳过域名 $DOMAIN（webroot 验证失败）"
        continue
    fi
    
    if [ $USE_CERTBOT -eq 1 ]; then
        certbot_issue "$DOMAIN"
        if [ $? -ne 0 ]; then
            log "certbot 申请失败，尝试 acme.sh..."
            if use_acme_sh; then
                acme_sh_issue "$DOMAIN"
            fi
        fi
    else
        acme_sh_issue "$DOMAIN"
        if [ $? -ne 0 ]; then
            log "acme.sh 申请失败: $DOMAIN"
            continue
        fi
    fi
done

# 创建统一的证书更新脚本
cat > /usr/local/bin/renew-cert.sh << 'EOF'
#!/bin/bash
CERT_PATH="$CERT_PATH"
WEBROOT_PATH="$WEBROOT_PATH"
LOG_FILE="$LOG_FILE"

log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1" | tee -a $LOG_FILE
}

# 检查是否使用 certbot
if command -v certbot &> /dev/null; then
    log "使用 certbot 更新证书..."
    certbot renew --webroot --webroot-path $WEBROOT_PATH --quiet
    
    # 复制证书到nginx目录
    for CERT_DIR in /etc/letsencrypt/live/*; do
        if [ -d "$CERT_DIR" ]; then
            DOMAIN=$(basename "$CERT_DIR")
            cp "$CERT_DIR/fullchain.pem" "$CERT_PATH/$DOMAIN.pem"
            cp "$CERT_DIR/privkey.pem" "$CERT_PATH/$DOMAIN.key"
            chown nginx:nginx "$CERT_PATH/$DOMAIN.pem" "$CERT_PATH/$DOMAIN.key" 2>/dev/null
            chmod 600 "$CERT_PATH/$DOMAIN.key"
            chmod 644 "$CERT_PATH/$DOMAIN.pem"
        fi
    done
elif [ -d "/root/.acme.sh" ]; then
    log "使用 acme.sh 更新证书..."
    /root/.acme.sh/acme.sh --cron --home /root/.acme.sh
else
    log "错误：certbot 和 acme.sh 都不可用"
    exit 1
fi

# 重新加载nginx配置
nginx -s reload
log "证书更新完成"
EOF

# 设置更新脚本权限
chmod +x /usr/local/bin/renew-cert.sh

# 添加定时任务
(crontab -l 2>/dev/null | grep -v renew-cert.sh; echo "0 2 1 * * /usr/local/bin/renew-cert.sh") | crontab -

log "证书自动更新任务已设置"
log "所有证书位于: $CERT_PATH/"
log "下次更新时间: 下月1号凌晨2点"
log "使用方案: $([ $USE_CERTBOT -eq 1 ] && echo 'certbot' || echo 'acme.sh')"

# 重新加载nginx配置
nginx -s reload