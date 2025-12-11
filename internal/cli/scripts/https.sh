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
mkdir -p $WEBROOT_PATH/.well-known/acme-challenge

# 设置正确的权限，确保nginx和certbot都能访问
chmod -R 755 $WEBROOT_PATH
chown -R nginx:nginx $WEBROOT_PATH

# 安装必要的软件包
log "检查并安装必要的软件包..."
yum install -y epel-release
yum install -y certbot python3-certbot-nginx

# 处理每个域名
for DOMAIN in "$@"; do
    log "开始处理域名: $DOMAIN"
    
    # 创建临时nginx配置用于证书验证
    log "创建临时HTTP配置用于证书验证..."
    cat > /etc/nginx/conf.d/temp-$DOMAIN.conf << TEMPCONF
server {
    listen 80;
    server_name $DOMAIN;
    
    # Let's Encrypt验证
    location /.well-known/acme-challenge/ {
        root $WEBROOT_PATH;
        allow all;
    }
    
    # 其他请求返回404
    location / {
        return 404;
    }
}
TEMPCONF
    
    # 重载nginx使临时配置生效
    nginx -t && nginx -s reload
    log "临时配置已生效"
    
    # 申请证书
    log "开始申请 Let's Encrypt 证书..."
    # 添加non-interactive参数避免交互式提示
    certbot certonly --webroot \
            --webroot-path $WEBROOT_PATH \
            --agree-tos \
            --email $EMAIL \
            --non-interactive \
            -d $DOMAIN
    
    # 检查证书文件是否存在，而不是仅检查命令返回值
    if [ -f "/etc/letsencrypt/live/$DOMAIN/fullchain.pem" ] && [ -f "/etc/letsencrypt/live/$DOMAIN/privkey.pem" ]; then
        log "证书申请成功: $DOMAIN"
        
        # 复制证书到nginx证书目录
        cp /etc/letsencrypt/live/$DOMAIN/fullchain.pem $CERT_PATH/$DOMAIN.pem
        cp /etc/letsencrypt/live/$DOMAIN/privkey.pem $CERT_PATH/$DOMAIN.key
        
        # 设置正确的权限
        chown -R nginx:nginx $CERT_PATH
        chmod 600 $CERT_PATH/$DOMAIN.key
        chmod 644 $CERT_PATH/$DOMAIN.pem
        
        log "证书已复制到Nginx目录：$CERT_PATH/$DOMAIN.pem"
        
        # 删除临时配置
        rm -f /etc/nginx/conf.d/temp-$DOMAIN.conf
        log "临时配置已删除"
    else
        log "证书申请失败: $DOMAIN"
        # 删除临时配置
        rm -f /etc/nginx/conf.d/temp-$DOMAIN.conf
        continue
    fi
done

# 创建统一的证书更新脚本
cat > /usr/local/bin/renew-cert.sh << EOF
#!/bin/bash
CERT_PATH="$CERT_PATH"
WEBROOT_PATH="$WEBROOT_PATH"
# 更新所有证书
certbot renew --webroot --webroot-path \$WEBROOT_PATH --quiet
# 更新成功后，复制所有证书到nginx目录
for CERT_DIR in /etc/letsencrypt/live/*; do
    if [ -d "\$CERT_DIR" ]; then
        DOMAIN=\$(basename "\$CERT_DIR")
        cp "\$CERT_DIR/fullchain.pem" "\$CERT_PATH/\$DOMAIN.pem"
        cp "\$CERT_DIR/privkey.pem" "\$CERT_PATH/\$DOMAIN.key"
        chown nginx:nginx "\$CERT_PATH/\$DOMAIN.pem" "\$CERT_PATH/\$DOMAIN.key"
        chmod 600 "\$CERT_PATH/\$DOMAIN.key"
        chmod 644 "\$CERT_PATH/\$DOMAIN.pem"
    fi
done
# 重新加载nginx配置
nginx -s reload
EOF

# 设置更新脚本权限
chmod +x /usr/local/bin/renew-cert.sh

# 添加定时任务
(crontab -l 2>/dev/null | grep -v renew-cert.sh; echo "0 2 1 * * /usr/local/bin/renew-cert.sh") | crontab -

log "证书自动更新任务已设置"
log "所有证书位于: $CERT_PATH/"
log "下次更新时间: 下月1号凌晨2点"

# 重新加载nginx配置
nginx -s reload