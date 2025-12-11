#!/bin/bash
# 交互式完整初始化脚本
# 安装环境 + 配置代理程序 + 启动服务

set -e

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

# 获取当前目录
CURRENT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

echo -e "${CYAN}"
echo "╔════════════════════════════════════════════════════════╗"
echo "║                                                        ║"
echo "║      若依蓝绿部署 - 完整初始化向导                    ║"
echo "║                                                        ║"
echo "╚════════════════════════════════════════════════════════╝"
echo -e "${NC}"

# 询问函数
ask_install() {
    local component=$1
    echo -e "${YELLOW}是否安装 $component? (y/n):${NC} "
    read -r response
    if [[ "$response" =~ ^[Yy]$ ]]; then
        return 0
    else
        return 1
    fi
}

# 检查是否已安装
check_installed() {
    local cmd=$1
    if command -v $cmd &> /dev/null; then
        echo -e "${GREEN}✓ $cmd 已安装${NC}"
        return 0
    else
        echo -e "${YELLOW}✗ $cmd 未安装${NC}"
        return 1
    fi
}

# ==================== 检查端口占用 ====================
check_port_80() {
    if ! command -v ss >/dev/null 2>&1; then
        return 0
    fi
    
    # 检查端口80是否被占用
    if ! ss -tlnp | grep -q ':80 '; then
        return 0
    fi
    
    # 获取占用端口的进程信息
    local port_info=$(sudo ss -tlnp | grep ':80 ' | head -n 1)
    local process_info=$(echo "$port_info" | grep -oP 'users:\(\(".*?",pid=\d+' | head -n 1)
    local process_name=$(echo "$process_info" | grep -oP '(?<=\(\(").*?(?=")' || echo "unknown")
    local process_pid=$(echo "$process_info" | grep -oP '(?<=pid=)\d+' || echo "")
    
    echo -e "${YELLOW}端口80已被占用${NC}"
    echo -e "${CYAN}占用进程: $process_name (PID: $process_pid)${NC}"
    
    # 如果是nginx进程，跳过
    if [[ "$process_name" == "nginx" ]]; then
        echo -e "${GREEN}占用进程是Nginx，跳过处理${NC}"
        return 0
    fi
    
    # 询问是否kill
    echo -e "${YELLOW}是否终止该进程以释放端口80? (y/n):${NC} "
    read -r kill_process
    
    if [[ "$kill_process" =~ ^[Yy]$ ]] && [ -n "$process_pid" ]; then
        echo -e "${CYAN}终止进程 $process_pid...${NC}"
        if sudo kill -9 "$process_pid" 2>/dev/null; then
            echo -e "${GREEN}✓ 进程已终止${NC}"
            sleep 2
            return 0
        else
            echo -e "${RED}✗ 终止进程失败${NC}"
            return 1
        fi
    else
        echo -e "${YELLOW}跳过终止进程，Nginx可能无法启动${NC}"
        return 1
    fi
}

# ==================== Nginx 安装 ====================
install_nginx() {
    echo -e "\n${BLUE}========================================${NC}"
    echo -e "${BLUE}[1/6] 安装 Nginx${NC}"
    echo -e "${BLUE}========================================${NC}"
    
    # 检查Nginx是否已安装
    if check_installed nginx; then
        echo -e "${GREEN}✓ Nginx 已安装${NC}"
        
        # 检查是否运行
        if sudo systemctl is-active --quiet nginx; then
            echo -e "${GREEN}✓ Nginx 正在运行${NC}"
            return 0
        else
            echo -e "${YELLOW}Nginx未运行，尝试启动...${NC}"
            
            # 检查端口占用
            check_port_80
            local port_check_result=$?
            
            # 如果端口检查返回1（用户选择不终止进程），询问是否仍要尝试启动
            if [ $port_check_result -ne 0 ]; then
                echo -e "${YELLOW}是否仍要尝试启动Nginx? (y/n):${NC} "
                read -r try_start
                if [[ ! "$try_start" =~ ^[Yy]$ ]]; then
                    echo -e "${YELLOW}跳过Nginx启动${NC}"
                    return 0
                fi
            fi
            
            # 启动Nginx
            if sudo systemctl start nginx; then
                sudo systemctl enable nginx
                echo -e "${GREEN}✓ Nginx 已启动${NC}"
            else
                echo -e "${RED}✗ Nginx 启动失败${NC}"
                sudo journalctl -u nginx -n 10 --no-pager
            fi
            return 0
        fi
    fi
    
    # 检查端口占用
    echo -e "${CYAN}检查端口80...${NC}"
    if ! check_port_80; then
        echo -e "${YELLOW}端口80被占用，但继续安装Nginx${NC}"
    fi
    
    # 安装Nginx
    echo -e "${CYAN}开始安装 Nginx...${NC}"
    
    # 检查是否已有epel源
    if ! rpm -q epel-release &>/dev/null && ! rpm -q epel-aliyuncs-release &>/dev/null; then
        sudo yum install -y epel-release
    else
        echo -e "${CYAN}EPEL源已存在，跳过安装${NC}"
    fi
    
    sudo yum install -y nginx
    
    # 创建默认目录
    sudo mkdir -p /etc/nginx/html
    sudo mkdir -p /etc/nginx/cert
    sudo mkdir -p /etc/nginx/conf.d
    
    # 检查配置文件
    echo -e "${CYAN}检查Nginx配置...${NC}"
    if sudo nginx -t; then
        echo -e "${GREEN}✓ Nginx配置正确${NC}"
    else
        echo -e "${RED}✗ Nginx配置有误${NC}"
        return 1
    fi
    
    # 启动Nginx
    echo -e "${CYAN}启动Nginx...${NC}"
    if sudo systemctl start nginx; then
        sudo systemctl enable nginx
        echo -e "${GREEN}✓ Nginx 安装并启动成功${NC}"
    else
        echo -e "${YELLOW}⚠ Nginx安装成功但启动失败${NC}"
        echo -e "${CYAN}查看错误日志:${NC}"
        sudo journalctl -u nginx -n 20 --no-pager
        echo -e "${YELLOW}可能是端口被占用，稍后可手动启动: sudo systemctl start nginx${NC}"
    fi
}

# ==================== Docker 安装 ====================
install_docker() {
    echo -e "\n${BLUE}========================================${NC}"
    echo -e "${BLUE}[2/6] 安装 Docker${NC}"
    echo -e "${BLUE}========================================${NC}"
    
    if check_installed docker; then
        echo -e "${YELLOW}Docker 已存在，跳过安装${NC}"
        return
    fi
    
    if ! ask_install "Docker"; then
        echo -e "${YELLOW}跳过 Docker 安装${NC}"
        return
    fi
    
    echo -e "${CYAN}开始安装 Docker...${NC}"
    sudo yum install -y yum-utils device-mapper-persistent-data lvm2
    
    # 添加Docker仓库（如果不存在）
    if [ ! -f /etc/yum.repos.d/docker-ce.repo ]; then
        sudo yum-config-manager --add-repo https://mirrors.aliyun.com/docker-ce/linux/centos/docker-ce.repo
    fi
    
    sudo yum install -y docker-ce docker-ce-cli containerd.io
    sudo systemctl start docker
    sudo systemctl enable docker
    
    sudo mkdir -p /etc/docker
    sudo tee /etc/docker/daemon.json <<-'EOF'
{
  "registry-mirrors": ["https://0532d87147000f650f78c018b8fb6d40.mirror.swr.myhuaweicloud.com"]
}
EOF
    
    sudo systemctl daemon-reload
    sudo systemctl restart docker
    echo -e "${GREEN}✓ Docker 安装完成${NC}"
}

# ==================== Redis 安装 ====================
install_redis() {
    echo -e "\n${BLUE}========================================${NC}"
    echo -e "${BLUE}[3/6] 安装 Redis${NC}"
    echo -e "${BLUE}========================================${NC}"
    
    if sudo docker ps -a 2>/dev/null | grep -q redis; then
        echo -e "${YELLOW}Redis 容器已存在，跳过安装${NC}"
        return
    fi
    
    if ! command -v docker &> /dev/null; then
        echo -e "${YELLOW}Docker 未安装，跳过 Redis${NC}"
        return
    fi
    
    if ! ask_install "Redis"; then
        echo -e "${YELLOW}跳过 Redis 安装${NC}"
        return
    fi
    
    echo -e "${YELLOW}请输入Redis密码（默认: Redis@200722）:${NC} "
    read -r redis_password
    if [ -z "$redis_password" ]; then
        redis_password="Redis@200722"
    fi
    
    sudo docker run -d --name redis -p 6379:6379 \
        --restart=always \
        redis:latest redis-server --requirepass "$redis_password"
    
    echo -e "${GREEN}✓ Redis 安装完成${NC}"
    echo -e "${CYAN}Redis密码: $redis_password${NC}"
}

# ==================== Java 安装 ====================
install_java() {
    echo -e "\n${BLUE}========================================${NC}"
    echo -e "${BLUE}[4/6] 安装 Java 17${NC}"
    echo -e "${BLUE}========================================${NC}"
    
    if check_installed java; then
        echo -e "${YELLOW}Java 已存在，跳过安装${NC}"
        return
    fi
    
    if ! ask_install "Java 17"; then
        echo -e "${YELLOW}跳过 Java 安装${NC}"
        return
    fi
    
    echo -e "${CYAN}开始安装 OpenJDK 17...${NC}"
    sudo yum install -y java-17-openjdk java-17-openjdk-devel
    echo -e "${GREEN}✓ Java 17 安装完成${NC}"
}

# ==================== 配置代理程序 ====================
setup_proxy() {
    echo -e "\n${BLUE}========================================${NC}"
    echo -e "${BLUE}[5/6] 配置代理程序${NC}"
    echo -e "${BLUE}========================================${NC}"
    
    # 创建目录
    mkdir -p "$CURRENT_DIR/logs"
    mkdir -p "$CURRENT_DIR/configs"
    mkdir -p "$CURRENT_DIR/bin"
    echo -e "${GREEN}✓ 目录创建完成${NC}"
    
    # 查找可执行文件
    PROXY_BIN=""
    if [ -f "$CURRENT_DIR/ruoyi-proxy-linux" ]; then
        PROXY_BIN="$CURRENT_DIR/ruoyi-proxy-linux"
    elif [ -f "$CURRENT_DIR/bin/ruoyi-proxy-linux" ]; then
        PROXY_BIN="$CURRENT_DIR/bin/ruoyi-proxy-linux"
    elif [ -f "$CURRENT_DIR/ruoyi-proxy" ]; then
        PROXY_BIN="$CURRENT_DIR/ruoyi-proxy"
    else
        echo -e "${RED}✗ 未找到可执行文件${NC}"
        return 1
    fi
    
    chmod +x "$PROXY_BIN"
    echo -e "${GREEN}✓ 找到可执行文件: $(basename $PROXY_BIN)${NC}"
    
    # 创建systemd服务
    SERVICE_FILE="/etc/systemd/system/ruoyi-proxy.service"
    sudo tee "$SERVICE_FILE" > /dev/null <<EOF
[Unit]
Description=Ruoyi Blue-Green Deployment Proxy
After=network.target

[Service]
Type=simple
User=$USER
WorkingDirectory=$CURRENT_DIR
ExecStart=$PROXY_BIN
Restart=on-failure
RestartSec=5s
StandardOutput=append:$CURRENT_DIR/logs/proxy.log
StandardError=append:$CURRENT_DIR/logs/proxy-error.log
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
EOF
    
    sudo systemctl daemon-reload
    echo -e "${GREEN}✓ systemd服务已创建${NC}"
    
    # 设置脚本权限
    chmod +x "$CURRENT_DIR/scripts"/*.sh 2>/dev/null || true
    echo -e "${GREEN}✓ 脚本权限已设置${NC}"
}

# ==================== 配置应用 ====================
configure_app() {
    echo -e "\n${BLUE}========================================${NC}"
    echo -e "${BLUE}[6/6] 配置应用${NC}"
    echo -e "${BLUE}========================================${NC}"
    
    APP_CONFIG="$CURRENT_DIR/configs/app_config.json"
    
    # 域名配置
    echo -e "\n${CYAN}域名配置${NC}"
    echo -e "${YELLOW}请输入域名（例: api.example.com）:${NC} "
    read -r domain_input
    if [ -z "$domain_input" ]; then
        domain_input="example.com"
    fi
    
    # HTTPS配置
    echo -e "\n${CYAN}HTTPS配置${NC}"
    echo -e "${YELLOW}是否启用HTTPS? (y/n):${NC} "
    read -r enable_https_input
    
    ENABLE_HTTPS="false"
    SSL_EMAIL="your-email@example.com"
    
    if [[ "$enable_https_input" =~ ^[Yy]$ ]]; then
        ENABLE_HTTPS="true"
        echo -e "${YELLOW}邮箱地址（用于Let's Encrypt通知）:${NC} "
        read -r ssl_email
        if [ -n "$ssl_email" ]; then
            SSL_EMAIL="$ssl_email"
        fi
    fi
    
    # 文件同步配置
    echo -e "\n${CYAN}文件同步配置${NC}"
    echo -e "${YELLOW}是否启用文件同步? (y/n):${NC} "
    read -r enable_sync
    
    SYNC_ENABLED="false"
    SERVER_ROLE="master"
    SLAVE_HOST="http://192.168.1.10:8002"
    SYNC_PASSWORD="change-me-please"
    VUE_SOURCE="./vue/dist"
    
    if [[ "$enable_sync" =~ ^[Yy]$ ]]; then
        SYNC_ENABLED="true"
        
        echo -e "\n${CYAN}选择服务器角色:${NC}"
        echo -e "  1. master - 主服务器（推送文件）"
        echo -e "  2. slave  - 从服务器（接收文件）"
        echo -e "${YELLOW}请选择 (1/2):${NC} "
        read -r role_choice
        
        if [[ "$role_choice" == "1" ]]; then
            SERVER_ROLE="master"
            
            echo -e "${YELLOW}从服务器地址 (例: http://192.168.1.20:8002):${NC} "
            read -r slave_host_input
            if [ -n "$slave_host_input" ]; then
                SLAVE_HOST="$slave_host_input"
            fi
            
            echo -e "${YELLOW}同步密码:${NC} "
            read -r sync_pass_input
            if [ -n "$sync_pass_input" ]; then
                SYNC_PASSWORD="$sync_pass_input"
            fi
            
            echo -e "${YELLOW}是否同步Vue文件? (y/n):${NC} "
            read -r sync_vue
            if [[ ! "$sync_vue" =~ ^[Yy]$ ]]; then
                VUE_SOURCE=""
            fi
            
        elif [[ "$role_choice" == "2" ]]; then
            SERVER_ROLE="slave"
            
            echo -e "${YELLOW}同步密码（需与主服务器一致）:${NC} "
            read -r sync_pass_input
            if [ -n "$sync_pass_input" ]; then
                SYNC_PASSWORD="$sync_pass_input"
            fi
        fi
    fi
    
    # 生成统一配置文件
    cat > "$APP_CONFIG" <<EOF
{
  "domain": "$domain_input",
  "enable_https": $ENABLE_HTTPS,
  "proxy": {
    "blue_target": "http://127.0.0.1:8080",
    "green_target": "http://127.0.0.1:8081",
    "active_env": "blue",
    "proxy_port": "8000"
  },
  "sync": {
    "enabled": $SYNC_ENABLED,
    "server_role": "$SERVER_ROLE",
    "slave_host": "$SLAVE_HOST",
    "slave_password": "$SYNC_PASSWORD",
    "jar_source_path": "./ruoyi-*.jar",
    "vue_source_path": "$VUE_SOURCE",
    "remote_jar_path": "./ruoyi-*.jar",
    "remote_vue_path": "./vue/dist",
    "remote_restart_script": "./scripts/service.sh restart",
    "vue_sync_interval": 300
  },
  "ssl": {
    "email": "$SSL_EMAIL",
    "cert_path": "/etc/nginx/cert",
    "webroot_path": "/etc/nginx/html",
    "log_file": "/var/log/cert-renewal.log"
  },
  "nginx": {
    "config_path": "/etc/nginx/conf.d/ruoyi.conf",
    "html_path": "/etc/nginx/html",
    "vue_path": "/etc/nginx/html/admin"
  }
}
EOF
    
    # 同时生成兼容的旧配置文件
    cat > "$CURRENT_DIR/configs/sync_config.json" <<EOF
{
  "enabled": $SYNC_ENABLED,
  "server_role": "$SERVER_ROLE",
  "slave_host": "$SLAVE_HOST",
  "slave_password": "$SYNC_PASSWORD",
  "jar_source_path": "./ruoyi-*.jar",
  "vue_source_path": "$VUE_SOURCE",
  "remote_jar_path": "./ruoyi-*.jar",
  "remote_vue_path": "./vue/dist",
  "remote_restart_script": "./scripts/service.sh restart",
  "vue_sync_interval": 300
}
EOF
    
    echo -e "${GREEN}✓ 应用配置已保存${NC}"
    echo -e "${CYAN}配置文件: $APP_CONFIG${NC}"
    
    # 配置Nginx
    echo -e "\n${CYAN}配置Nginx...${NC}"
    
    if [[ "$ENABLE_HTTPS" == "true" ]]; then
        # 先用HTTP配置启动Nginx
        echo -e "${CYAN}生成临时HTTP配置...${NC}"
        bash "$CURRENT_DIR/scripts/configure-nginx.sh" false
        
        # 申请SSL证书
        echo -e "\n${CYAN}申请SSL证书...${NC}"
        bash "$CURRENT_DIR/scripts/https.sh" "$domain_input"
        
        # 检查证书是否申请成功
        if [ -f "/etc/letsencrypt/live/$domain_input/fullchain.pem" ]; then
            echo -e "${GREEN}✓ SSL证书申请成功${NC}"
            
            # 生成HTTPS配置
            echo -e "${CYAN}生成HTTPS配置...${NC}"
            bash "$CURRENT_DIR/scripts/configure-nginx.sh" true
        else
            echo -e "${RED}✗ SSL证书申请失败，使用HTTP配置${NC}"
        fi
    else
        # 直接生成HTTP配置
        bash "$CURRENT_DIR/scripts/configure-nginx.sh" false
    fi
    
    echo -e "${GREEN}✓ Nginx配置完成${NC}"
}

# ==================== 主流程 ====================

echo -e "${CYAN}开始初始化...${NC}\n"

install_nginx
install_docker
install_redis
install_java
setup_proxy
configure_app

# 完成
echo -e "\n${GREEN}╔════════════════════════════════════════════════════════╗${NC}"
echo -e "${GREEN}║                                                        ║${NC}"
echo -e "${GREEN}║      ✓ 初始化完成！                                   ║${NC}"
echo -e "${GREEN}║                                                        ║${NC}"
echo -e "${GREEN}╚════════════════════════════════════════════════════════╝${NC}"

echo -e "\n${CYAN}已安装组件:${NC}"
check_installed nginx && echo -e "  ${GREEN}✓${NC} Nginx"
check_installed docker && echo -e "  ${GREEN}✓${NC} Docker"
sudo docker ps -a 2>/dev/null | grep -q redis && echo -e "  ${GREEN}✓${NC} Redis"
check_installed java && echo -e "  ${GREEN}✓${NC} Java"

# ==================== 启动服务 ====================
echo -e "\n${BLUE}========================================${NC}"
echo -e "${BLUE}启动服务${NC}"
echo -e "${BLUE}========================================${NC}"

echo -e "${YELLOW}是否后台启动代理程序? (y/n):${NC} "
read -r start_proxy
if [[ "$start_proxy" =~ ^[Yy]$ ]]; then
    echo -e "${CYAN}启动代理程序...${NC}"
    sudo systemctl start ruoyi-proxy
    sudo systemctl enable ruoyi-proxy
    sleep 2
    
    if sudo systemctl is-active --quiet ruoyi-proxy; then
        echo -e "${GREEN}✓ 代理程序已启动${NC}"
        echo -e "${CYAN}查看状态: sudo systemctl status ruoyi-proxy${NC}"
        echo -e "${CYAN}查看日志: tail -f $CURRENT_DIR/logs/proxy.log${NC}"
    else
        echo -e "${RED}✗ 代理程序启动失败${NC}"
        echo -e "${CYAN}查看日志: sudo journalctl -u ruoyi-proxy -n 50${NC}"
    fi
else
    echo -e "${YELLOW}跳过启动，稍后可手动启动:${NC}"
    echo -e "  ${CYAN}sudo systemctl start ruoyi-proxy${NC}"
fi

echo -e "\n${CYAN}提示:${NC}"
echo -e "  - JAR文件请放在: ${BLUE}$CURRENT_DIR${NC}"
echo -e "  - 使用CLI管理: ${BLUE}$PROXY_BIN cli${NC}"
echo -e "  - 修改同步配置: ${BLUE}$PROXY_BIN cli${NC} 然后执行 ${BLUE}sync-config${NC}"
echo ""