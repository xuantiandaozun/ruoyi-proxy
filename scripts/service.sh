#!/bin/bash

# 若依项目蓝绿部署控制脚本 - Java 17 + AppCDS
# 
# 重要说明：
# 1. jar 文件必须放在项目根目录（和 scripts/ 目录同级）
# 2. jar 文件命名格式：ruoyi-*.jar
# 3. 脚本会自动查找最新的 jar 文件进行部署
# 
# 多服务部署示例：
#   BLUE_PORT=9080 GREEN_PORT=9081 APP_NAME=collector ./scripts/service.sh start

# ==================== 配置区域 ====================
# 基础配置（可通过环境变量覆盖）
SERVICE_ID="${SERVICE_ID:-}"
APP_NAME="${APP_NAME:-ruoyi}"
APP_JAR_PATTERN="${APP_JAR_PATTERN:-ruoyi-*.jar}"
SPRING_PROFILE="${SPRING_PROFILE:-prod}"

# 蓝绿部署配置（可通过环境变量覆盖）
BLUE_PORT="${BLUE_PORT:-8080}"
GREEN_PORT="${GREEN_PORT:-8081}"
PROXY_PORT="${PROXY_PORT:-8000}"
PROXY_MGMT_PORT="${PROXY_MGMT_PORT:-8001}"

# 部署配置
KEEP_HISTORY_JARS=2
STARTUP_TIMEOUT=30

# JVM参数配置
BASE_JVM_OPTS="-server \
-Xms1g \
-Xmx3g \
-XX:+UseG1GC \
-XX:MaxGCPauseMillis=200 \
-XX:+UseStringDeduplication \
-XX:+HeapDumpOnOutOfMemoryError \
-XX:HeapDumpPath=./logs/ \
-Xlog:gc*:./logs/gc.log:time,tags \
-XX:+ShowCodeDetailsInExceptionMessages \
-Dfile.encoding=UTF-8 \
-Djava.security.egd=file:/dev/./urandom"

# ==================== 权限和环境检查 ====================
check_permissions() {
    # 检查脚本执行权限
    if [ ! -x "${BASH_SOURCE[0]}" ]; then
        echo -e "${RED}错误: 脚本没有执行权限${NC}"
        echo "请运行: chmod +x ${BASH_SOURCE[0]}"
        exit 1
    fi
    
    # 检查写权限
    if [ ! -w "$APP_HOME" ]; then
        echo -e "${RED}错误: 没有目录写权限 $APP_HOME${NC}"
        exit 1
    fi
    
    # 检查Java版本
    if ! command -v java >/dev/null 2>&1; then
        echo -e "${RED}错误: 未找到Java环境${NC}"
        exit 1
    fi
    
    local java_version=$(java -version 2>&1 | head -n 1 | grep -oE '[0-9]+' | head -n 1)
    if [ "$java_version" -lt 17 ]; then
        echo -e "${RED}错误: 需要Java 17或更高版本，当前版本: $java_version${NC}"
        exit 1
    fi
}

# ==================== 脚本逻辑 ====================
# 获取脚本所在目录的绝对路径
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# APP_HOME 是 scripts 的父目录
APP_HOME="$(cd "$SCRIPT_DIR/.." && pwd)"
PROXY_BINARY="$APP_HOME/bin/ruoyi-proxy"
PROXY_CONFIG="$APP_HOME/configs/app_config.json"
PROXY_PID_FILE="$APP_HOME/ruoyi-proxy.pid"
PROXY_LOG_FILE="$APP_HOME/logs/ruoyi-proxy.log"

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

# 查找最新jar文件
find_jar_file() {
    local jar_files=($(find "$APP_HOME" -maxdepth 1 -name "$APP_JAR_PATTERN" -type f 2>/dev/null | sort -V))
    
    if [ ${#jar_files[@]} -eq 0 ]; then
        echo ""
        return 1
    else
        echo "$(basename "${jar_files[-1]}")"
        return 0
    fi
}

# 获取当前jar文件
get_current_jar() {
    local jar_file=$(find_jar_file)
    if [ -z "$jar_file" ]; then
        echo -e "${RED}错误: 未找到匹配的jar文件 ($APP_JAR_PATTERN)${NC}" >&2
        echo -e "${YELLOW}查找目录: $APP_HOME${NC}" >&2
        echo -e "${YELLOW}提示: 请将 jar 文件放在 $APP_HOME 目录${NC}" >&2
        return 1
    fi
    echo "$jar_file"
}

# 获取环境端口
get_env_port() {
    case "$1" in
        blue) echo $BLUE_PORT ;;
        green) echo $GREEN_PORT ;;
        *) echo $BLUE_PORT ;;
    esac
}

# 获取环境PID文件
get_env_pid_file() {
    echo "$APP_HOME/$APP_NAME-$1.pid"
}

# 获取环境日志文件
get_env_log_file() {
    echo "$APP_HOME/logs/$APP_NAME-$1.log"
}

# 获取当前活跃环境
get_active_env() {
    if [ -f "$APP_HOME/.active_env" ]; then
        cat "$APP_HOME/.active_env"
    else
        echo "blue"
    fi
}

# 设置活跃环境
set_active_env() {
    echo "$1" > "$APP_HOME/.active_env"
}

# 获取另一个环境
get_other_env() {
    local env="$1"
    if [ "$env" = "blue" ]; then
        echo "green"
    else
        echo "blue"
    fi
}

# 改进的代理状态检查 - 优先使用端口检查
check_proxy_status() {
    # 方案1：只检查端口（最简单可靠）
    if command -v nc >/dev/null 2>&1; then
        if nc -z localhost $PROXY_PORT 2>/dev/null; then
            # 再检查管理端口确保是我们的代理程序
            if nc -z localhost $PROXY_MGMT_PORT 2>/dev/null; then
                echo "running"
                return 0
            fi
        fi
    fi
    
    # 方案2：如果没有nc，通过curl检查管理接口
    if command -v curl >/dev/null 2>&1; then
        if curl -s --connect-timeout 2 "localhost:$PROXY_MGMT_PORT/health" >/dev/null 2>&1; then
            echo "running"
            return 0
        fi
    fi
    
    # 方案3：最后才检查PID文件（作为备选）
    if [ -f "$PROXY_PID_FILE" ] && [ -s "$PROXY_PID_FILE" ]; then
        local pid=$(cat "$PROXY_PID_FILE")
        if kill -0 $pid 2>/dev/null; then
            echo "running"
            return 0
        else
            # PID文件无效，清理它
            rm -f "$PROXY_PID_FILE"
        fi
    fi
    
    echo "stopped"
}

# 启动代理程序
start_proxy() {
    echo -e "${BLUE}启动蓝绿代理程序...${NC}"
    
    if [ "$(check_proxy_status)" = "running" ]; then
        echo -e "${GREEN}代理程序已在运行${NC}"
        return 0
    fi
    
    if [ ! -f "$PROXY_BINARY" ]; then
        echo -e "${RED}错误: 未找到代理程序 $PROXY_BINARY${NC}"
        return 1
    fi
    
    mkdir -p "$APP_HOME/logs"
    
    cd "$APP_HOME"
    nohup "$PROXY_BINARY" > "$PROXY_LOG_FILE" 2>&1 &
    echo $! > "$PROXY_PID_FILE"
    
    sleep 3
    if [ "$(check_proxy_status)" = "running" ]; then
        echo -e "${GREEN}代理程序启动成功 (端口: $PROXY_PORT)${NC}"
        return 0
    else
        echo -e "${RED}代理程序启动失败${NC}"
        rm -f "$PROXY_PID_FILE"
        return 1
    fi
}

# 停止代理程序
stop_proxy() {
    echo -e "${BLUE}停止蓝绿代理程序...${NC}"
    
    if [ -f "$PROXY_PID_FILE" ] && [ -s "$PROXY_PID_FILE" ]; then
        local pid=$(cat "$PROXY_PID_FILE")
        if kill -0 $pid 2>/dev/null; then
            kill $pid
            
            for i in {1..10}; do
                if ! kill -0 $pid 2>/dev/null; then
                    rm -f "$PROXY_PID_FILE"
                    echo -e "${GREEN}代理程序已停止${NC}"
                    return 0
                fi
                sleep 1
            done
            
            kill -9 $pid 2>/dev/null
            rm -f "$PROXY_PID_FILE"
            echo -e "${GREEN}代理程序已强制停止${NC}"
        else
            rm -f "$PROXY_PID_FILE"
            echo -e "${YELLOW}代理程序未运行${NC}"
        fi
    else
        echo -e "${YELLOW}代理程序未运行${NC}"
    fi
}

# 检查环境状态
check_env_status() {
    local env="$1"
    local pid_file=$(get_env_pid_file "$env")
    local port=$(get_env_port "$env")
    
    if [ -f "$pid_file" ] && [ -s "$pid_file" ]; then
        local pid=$(cat "$pid_file")
        if kill -0 $pid 2>/dev/null; then
            if command -v nc >/dev/null 2>&1; then
                if nc -z localhost $port 2>/dev/null; then
                    echo "running"
                else
                    echo "unhealthy"
                fi
            else
                echo "running"
            fi
        else
            rm -f "$pid_file"
            echo "stopped"
        fi
    else
        echo "stopped"
    fi
}

# 启动指定环境
start_env() {
    local env="$1"
    local jar_file="$2"
    local port=$(get_env_port "$env")
    local pid_file=$(get_env_pid_file "$env")
    local log_file=$(get_env_log_file "$env")
    
    echo -e "${BLUE}启动 $env 环境 (端口: $port)${NC}"
    
    if [ "$(check_env_status "$env")" = "running" ]; then
        echo -e "${YELLOW}$env 环境已在运行${NC}"
        return 1
    fi
    
    if [ -z "$jar_file" ]; then
        echo -e "${RED}错误: jar_file参数为空${NC}"
        echo -e "${YELLOW}提示: 请确保 jar 文件在项目根目录，命名格式为 ruoyi-*.jar${NC}"
        return 1
    fi
    
    if [ ! -f "$APP_HOME/$jar_file" ]; then
        echo -e "${RED}错误: 找不到jar文件 $APP_HOME/$jar_file${NC}"
        echo -e "${YELLOW}提示: 请将 jar 文件放在项目根目录（和 scripts/ 同级）${NC}"
        echo -e "${YELLOW}当前查找目录: $APP_HOME${NC}"
        return 1
    fi
    
    mkdir -p "$APP_HOME/logs"
    
    # 构建启动命令
    local JAVA_OPTS="$BASE_JVM_OPTS"
    local JAR_FILE="$APP_HOME/$jar_file"
    
    echo -e "${CYAN}启动命令: java $JAVA_OPTS -Dspring.profiles.active=$SPRING_PROFILE -Dserver.port=$port -jar $JAR_FILE${NC}"
    
    cd "$APP_HOME"
    nohup java $JAVA_OPTS \
        -Dspring.profiles.active=$SPRING_PROFILE \
        -Dserver.port=$port \
        -jar "$JAR_FILE" \
        > "$log_file" 2>&1 &
    
    local java_pid=$!
    echo $java_pid > "$pid_file"
    echo -e "${CYAN}Java进程ID: $java_pid${NC}"
    
    # 等待应用启动 - 增加等待时间
    echo -e "${CYAN}等待应用启动...${NC}"
    local startup_wait=20  # 增加到20秒
    for i in $(seq 1 $startup_wait); do
        # 检查进程是否还在运行
        if ! kill -0 $java_pid 2>/dev/null; then
            echo -e "${RED}Java进程 $java_pid 在第 $i 秒时意外退出${NC}"
            rm -f "$pid_file"
            
            # 显示启动日志
            if [ -f "$log_file" ]; then
                echo -e "${YELLOW}=== 启动日志 ===${NC}"
                tail -n 30 "$log_file"
                echo -e "${YELLOW}=== 日志结束 ===${NC}"
            fi
            return 1
        fi
        
        # 检查端口是否可用
        if command -v nc >/dev/null 2>&1; then
            if nc -z localhost $port 2>/dev/null; then
                echo -e "${GREEN}第 $i 秒：端口 $port 可连接，应用启动成功${NC}"
                echo -e "${GREEN}$env 环境启动成功 (端口: $port)${NC}"
                return 0
            fi
        fi
        
        # 检查日志中是否有启动成功的标志
        if [ -f "$log_file" ]; then
            if grep -q "Started.*Server.*Started" "$log_file" 2>/dev/null; then
                echo -e "${GREEN}第 $i 秒：从日志检测到服务器启动完成${NC}"
                echo -e "${GREEN}$env 环境启动成功 (端口: $port)${NC}"
                return 0
            fi
        fi
        
        if [ $((i % 5)) -eq 0 ]; then
            echo -e "${CYAN}第 $i 秒：应用仍在启动中...${NC}"
        fi
        sleep 1
    done
    
    # 超时后最终检查
    if kill -0 $java_pid 2>/dev/null; then
        echo -e "${YELLOW}启动超时，但进程仍在运行，尝试最终检查...${NC}"
        
        # 最后再等5秒并检查
        sleep 5
        if command -v nc >/dev/null 2>&1; then
            if nc -z localhost $port 2>/dev/null; then
                echo -e "${GREEN}最终检查通过：端口可连接${NC}"
                echo -e "${GREEN}$env 环境启动成功 (端口: $port)${NC}"
                return 0
            fi
        fi
        
        # 检查日志
        if [ -f "$log_file" ]; then
            if grep -q "Started.*Server" "$log_file" 2>/dev/null; then
                echo -e "${GREEN}最终检查通过：日志显示启动成功${NC}"
                echo -e "${GREEN}$env 环境启动成功 (端口: $port)${NC}"
                return 0
            fi
        fi
        
        # 进程在运行但检查不通过，可能还在初始化
        echo -e "${YELLOW}进程运行中但检查未通过，请手动验证${NC}"
        echo -e "${YELLOW}$env 环境可能已启动 (端口: $port)${NC}"
        return 0
    else
        echo -e "${RED}$env 环境启动失败：进程已退出${NC}"
        rm -f "$pid_file"
        
        # 显示启动日志
        if [ -f "$log_file" ]; then
            echo -e "${YELLOW}=== 启动日志 ===${NC}"
            tail -n 30 "$log_file"
            echo -e "${YELLOW}=== 日志结束 ===${NC}"
        fi
        return 1
    fi
}

# 停止指定环境
stop_env() {
    local env="$1"
    local pid_file=$(get_env_pid_file "$env")
    local port=$(get_env_port "$env")
    
    echo -e "${BLUE}停止 $env 环境 (端口: $port)${NC}"
    
    local stopped=false
    
    # 1. 先尝试通过PID文件停止
    if [ -f "$pid_file" ] && [ -s "$pid_file" ]; then
        local pid=$(cat "$pid_file")
        echo -e "${CYAN}PID文件中的进程ID: $pid${NC}"
        
        if kill -0 $pid 2>/dev/null; then
            echo -e "${CYAN}发送TERM信号给进程 $pid${NC}"
            kill $pid
            
            # 等待进程正常退出
            for i in {1..15}; do
                if ! kill -0 $pid 2>/dev/null; then
                    rm -f "$pid_file"
                    echo -e "${GREEN}进程 $pid 已正常停止${NC}"
                    stopped=true
                    break
                fi
                echo -e "${CYAN}等待进程 $pid 退出... ($i/15)${NC}"
                sleep 1
            done
            
            # 强制杀死进程
            if [ "$stopped" = "false" ]; then
                echo -e "${YELLOW}强制停止进程 $pid${NC}"
                kill -9 $pid 2>/dev/null
                sleep 2
                if ! kill -0 $pid 2>/dev/null; then
                    echo -e "${GREEN}进程 $pid 已强制停止${NC}"
                    stopped=true
                fi
            fi
        else
            echo -e "${YELLOW}PID文件中的进程 $pid 不存在，清理PID文件${NC}"
        fi
        rm -f "$pid_file"
    else
        echo -e "${YELLOW}未找到PID文件 $pid_file${NC}"
    fi
    
    # 2. 通过端口查找并杀死可能遗留的进程
    if command -v lsof >/dev/null 2>&1; then
        local port_pids=$(lsof -ti tcp:$port 2>/dev/null | tr '\n' ' ')
        if [ -n "$port_pids" ]; then
            echo -e "${YELLOW}发现端口 $port 上的进程: $port_pids${NC}"
            for port_pid in $port_pids; do
                if [ -n "$port_pid" ]; then
                    echo -e "${CYAN}强制停止端口进程 $port_pid${NC}"
                    kill -9 $port_pid 2>/dev/null
                    stopped=true
                fi
            done
        fi
    elif command -v netstat >/dev/null 2>&1; then
        # 备用方案：使用netstat
        local port_info=$(netstat -tlnp 2>/dev/null | grep ":$port ")
        if [ -n "$port_info" ]; then
            local port_pid=$(echo "$port_info" | awk '{print $7}' | cut -d/ -f1)
            if [ -n "$port_pid" ] && [ "$port_pid" != "-" ] && [ "$port_pid" -gt 0 ] 2>/dev/null; then
                echo -e "${YELLOW}通过netstat发现端口 $port 进程: $port_pid${NC}"
                echo -e "${CYAN}强制停止端口进程 $port_pid${NC}"
                kill -9 $port_pid 2>/dev/null
                stopped=true
            fi
        fi
    fi
    
    # 3. 通过jps查找相关进程
    if command -v jps >/dev/null 2>&1; then
        local jar_pids=$(jps -l 2>/dev/null | grep "ruoyi.*\.jar" | awk '{print $1}')
        if [ -n "$jar_pids" ]; then
            echo -e "${YELLOW}发现ruoyi相关进程，检查端口占用...${NC}"
            for jar_pid in $jar_pids; do
                # 检查这个进程是否占用了目标端口
                if command -v lsof >/dev/null 2>&1; then
                    if lsof -p $jar_pid 2>/dev/null | grep -q ":$port "; then
                        echo -e "${CYAN}进程 $jar_pid 占用端口 $port，强制停止${NC}"
                        kill -9 $jar_pid 2>/dev/null
                        stopped=true
                    fi
                fi
            done
        fi
    fi
    
    # 4. 最终检查
    sleep 2
    if command -v lsof >/dev/null 2>&1; then
        if lsof -ti tcp:$port 2>/dev/null | grep -q .; then
            echo -e "${RED}警告: 端口 $port 仍被占用${NC}"
        else
            echo -e "${GREEN}$env 环境确认已停止，端口 $port 已释放${NC}"
        fi
    elif [ "$(check_env_status "$env")" = "stopped" ]; then
        echo -e "${GREEN}$env 环境确认已停止${NC}"
    else
        echo -e "${YELLOW}$env 环境状态未确认${NC}"
    fi
}

# 确保只有一个环境运行
ensure_single_env() {
    local target_env="$1"
    local other_env=$(get_other_env "$target_env")
    
    echo -e "${YELLOW}确保只有 $target_env 环境运行...${NC}"
    
    # 如果另一个环境在运行，先停止它
    if [ "$(check_env_status "$other_env")" = "running" ]; then
        echo -e "${YELLOW}停止 $other_env 环境...${NC}"
        stop_env "$other_env"
    fi
}

# 切换环境
switch_env() {
    local target_env="$1"
    
    if [ "$target_env" != "blue" ] && [ "$target_env" != "green" ]; then
        echo -e "${RED}错误: 环境必须是 blue 或 green${NC}"
        return 1
    fi
    
    if [ "$(check_proxy_status)" != "running" ]; then
        echo -e "${RED}错误: 代理程序未运行${NC}"
        return 1
    fi
    
    echo -e "${BLUE}切换到 $target_env 环境${NC}"
    
    if command -v curl > /dev/null 2>&1; then
        # 构建API URL，如果设置了SERVICE_ID则只切换该服务，否则切换所有服务
        local api_url="localhost:$PROXY_MGMT_PORT/switch?env=$target_env"
        if [ -n "$SERVICE_ID" ]; then
            api_url="${api_url}&service=${SERVICE_ID}"
            echo -e "${CYAN}切换服务: ${SERVICE_ID}${NC}"
        else
            echo -e "${CYAN}切换所有服务${NC}"
        fi
        
        local response=$(curl -s -X POST "$api_url" 2>/dev/null)
        local curl_exit_code=$?
        
        echo -e "${CYAN}代理响应: $response${NC}"
        
        if [ $curl_exit_code -eq 0 ]; then
            echo -e "${GREEN}代理切换请求发送成功${NC}"
            set_active_env "$target_env"
            
            # 验证切换结果
            sleep 1
            local status_response=$(curl -s "localhost:$PROXY_MGMT_PORT/status" 2>/dev/null)
            if echo "$status_response" | grep -q "\"active_env\":\"$target_env\""; then
                echo -e "${GREEN}环境切换验证成功${NC}"
                return 0
            else
                echo -e "${YELLOW}环境切换验证失败，但继续执行${NC}"
                return 0
            fi
        else
            echo -e "${RED}切换失败 (curl退出码: $curl_exit_code)${NC}"
            return 1
        fi
    else
        echo -e "${RED}错误: 需要curl命令${NC}"
        return 1
    fi
}

# 清理旧文件
cleanup() {
    local jar_files=($(find "$APP_HOME" -maxdepth 1 -name "$APP_JAR_PATTERN" -type f 2>/dev/null | sort -V))
    local jar_count=${#jar_files[@]}
    
    if [ $jar_count -le $KEEP_HISTORY_JARS ]; then
        return 0
    fi
    
    local remove_count=$((jar_count - KEEP_HISTORY_JARS))
    echo -e "${YELLOW}清理旧文件，保留最新的 $KEEP_HISTORY_JARS 个${NC}"
    
    for ((i=0; i<remove_count; i++)); do
        local old_jar=$(basename "${jar_files[i]}")
        echo -e "${CYAN}删除: $old_jar${NC}"
        rm -f "${jar_files[i]}"
        
        local old_workdir=$(get_jar_workdir "$old_jar")
        if [ -d "$old_workdir" ]; then
            rm -rf "$old_workdir"
        fi
    done
}

# ==================== 主要功能函数 ====================

# 启动服务
start() {
    echo -e "${BLUE}=== 启动服务 (蓝绿模式) ===${NC}"
    
    # 确保代理运行
    if [ "$(check_proxy_status)" != "running" ]; then
        if ! start_proxy; then
            echo -e "${RED}代理程序启动失败${NC}"
            return 1
        fi
    fi
    
    local jar_file=$(get_current_jar) || return 1
    local current_env=$(get_active_env)
    
    echo -e "${CYAN}使用JAR: $jar_file${NC}"
    echo -e "${CYAN}启动环境: $current_env${NC}"
    
    # 确保只有目标环境运行
    ensure_single_env "$current_env"
    
    # 启动环境
    if start_env "$current_env" "$jar_file"; then
        # 确保代理指向正确的环境
        switch_env "$current_env"
        echo -e "${GREEN}服务启动成功${NC}"
        echo -e "${GREEN}访问地址: http://localhost:$PROXY_PORT${NC}"
    else
        echo -e "${RED}服务启动失败${NC}"
        return 1
    fi
}

# 停止服务
stop() {
    echo -e "${BLUE}=== 停止服务 ===${NC}"
    
    stop_env "blue"
    stop_env "green"
    stop_proxy
    
    echo -e "${GREEN}所有服务已停止${NC}"
}

# 重启服务
restart() {
    echo -e "${BLUE}=== 重启服务 ===${NC}"
    
    local current_env=$(get_active_env)
    local other_env=$(get_other_env "$current_env")
    local jar_file=$(get_current_jar) || return 1
    
    # 停止两个环境
    stop_env "$current_env"
    stop_env "$other_env"
    
    # 确保代理运行
    if [ "$(check_proxy_status)" != "running" ]; then
        start_proxy
    fi
    
    # 启动当前环境
    if start_env "$current_env" "$jar_file"; then
        # 确保代理指向正确的环境
        switch_env "$current_env"
        echo -e "${GREEN}服务重启完成${NC}"
    else
        echo -e "${RED}服务重启失败${NC}"
        return 1
    fi
}

# 简单健康检查 - 只检查端口和进程稳定性
health_check() {
    local env="$1"
    local port=$(get_env_port "$env")
    local check_duration=${2:-5}  # 检查持续时间，默认5秒
    
    echo -e "${YELLOW}健康检查 $env 环境 (端口: $port)，检查 $check_duration 秒...${NC}"
    
    # 先检查进程是否存在
    if [ "$(check_env_status "$env")" != "running" ]; then
        echo -e "${RED}$env 环境进程未运行${NC}"
        return 1
    fi
    
    # 等待应用启动
    echo -e "${CYAN}等待应用启动...${NC}"
    sleep 3
    
    # 检查端口是否可连接
    if ! command -v nc >/dev/null 2>&1; then
        echo -e "${YELLOW}未找到nc命令，跳过端口检查${NC}"
        sleep $check_duration
        if [ "$(check_env_status "$env")" = "running" ]; then
            echo -e "${GREEN}进程稳定运行${NC}"
            return 0
        else
            echo -e "${RED}进程意外退出${NC}"
            return 1
        fi
    fi
    
    # 检查端口是否可连接
    if ! nc -z localhost $port 2>/dev/null; then
        echo -e "${RED}端口 $port 不可连接${NC}"
        return 1
    fi
    
    echo -e "${GREEN}端口 $port 可连接${NC}"
    
    # 持续检查指定时间，确保进程稳定
    echo -e "${CYAN}检查进程稳定性 $check_duration 秒...${NC}"
    for i in $(seq 1 $check_duration); do
        if [ "$(check_env_status "$env")" != "running" ]; then
            echo -e "${RED}第 $i 秒时进程意外退出${NC}"
            return 1
        fi
        
        if ! nc -z localhost $port 2>/dev/null; then
            echo -e "${RED}第 $i 秒时端口不可连接${NC}"
            return 1
        fi
        
        sleep 1
    done
    
    echo -e "${GREEN}健康检查通过：进程稳定运行 $check_duration 秒${NC}"
    return 0
}

# 等待端口释放
wait_port_release() {
    local port="$1"
    local max_wait=${2:-10}
    
    echo -e "${YELLOW}等待端口 $port 释放...${NC}"
    
    if ! command -v nc >/dev/null 2>&1; then
        echo -e "${YELLOW}未找到nc命令，等待 $max_wait 秒${NC}"
        sleep $max_wait
        return 0
    fi
    
    for i in $(seq 1 $max_wait); do
        if ! nc -z localhost $port 2>/dev/null; then
            echo -e "${GREEN}端口 $port 已释放${NC}"
            return 0
        fi
        echo -e "${CYAN}第 $i 秒，端口 $port 仍在使用中...${NC}"
        sleep 1
    done
    
    echo -e "${RED}端口 $port 在 $max_wait 秒后仍未释放${NC}"
    return 1
}

# 部署新版本
deploy() {
    echo -e "${BLUE}=== 蓝绿部署开始 ===${NC}"
    
    # 确保代理运行
    if [ "$(check_proxy_status)" != "running" ]; then
        if ! start_proxy; then
            echo -e "${RED}代理程序启动失败${NC}"
            return 1
        fi
    fi
    
    local new_jar=$(get_current_jar) || return 1
    local current_env=$(get_active_env)
    local standby_env=$(get_other_env "$current_env")
    
    echo -e "${CYAN}当前环境: $current_env${NC}"
    echo -e "${CYAN}部署到: $standby_env${NC}"
    echo -e "${CYAN}JAR文件: $new_jar${NC}"
    
    # 启动待机环境
    echo -e "${YELLOW}启动待机环境...${NC}"
    if ! start_env "$standby_env" "$new_jar"; then
        echo -e "${RED}待机环境启动失败${NC}"
        return 1
    fi
    
    # 简单健康检查 - 5秒稳定性检查
    if ! health_check "$standby_env" 5; then
        echo -e "${RED}健康检查失败，回滚部署${NC}"
        # 显示部分日志帮助调试
        local log_file=$(get_env_log_file "$standby_env")
        if [ -f "$log_file" ]; then
            echo -e "${CYAN}--- 最近30行日志 ---${NC}"
            tail -n 30 "$log_file"
            echo -e "${CYAN}--- 日志结束 ---${NC}"
        fi
        stop_env "$standby_env"
        return 1
    fi
    
    # 切换流量
    echo -e "${YELLOW}切换流量...${NC}"
    if ! switch_env "$standby_env"; then
        echo -e "${RED}流量切换失败${NC}"
        stop_env "$standby_env"
        return 1
    fi
    
    # 停止旧环境并等待端口释放
    echo -e "${YELLOW}停止旧环境...${NC}"
    local old_port=$(get_env_port "$current_env")
    stop_env "$current_env"
    
    # 等待旧环境端口完全释放
    wait_port_release "$old_port" 10
    
    # 清理
    cleanup
    
    echo -e "${GREEN}部署完成!${NC}"
    echo -e "${GREEN}当前环境: $standby_env${NC}"
    echo -e "${GREEN}访问地址: http://localhost:$PROXY_PORT${NC}"
}

# 查看状态
status() {
    echo -e "${BLUE}=== 服务状态 ===${NC}"
    
    # 代理状态
    local proxy_status=$(check_proxy_status)
    if [ "$proxy_status" = "running" ]; then
        echo -e "${GREEN}代理程序: 运行中 (端口: $PROXY_PORT)${NC}"
    else
        echo -e "${RED}代理程序: 已停止${NC}"
    fi
    
    # 环境状态
    local blue_status=$(check_env_status "blue")
    local green_status=$(check_env_status "green")
    local active_env=$(get_active_env)
    
    if [ "$blue_status" = "running" ]; then
        local mark=""
        [ "$active_env" = "blue" ] && mark=" <- 活跃"
        echo -e "${GREEN}蓝色环境: 运行中 (端口: $BLUE_PORT)$mark${NC}"
    else
        echo -e "${RED}蓝色环境: 已停止${NC}"
    fi
    
    if [ "$green_status" = "running" ]; then
        local mark=""
        [ "$active_env" = "green" ] && mark=" <- 活跃"
        echo -e "${GREEN}绿色环境: 运行中 (端口: $GREEN_PORT)$mark${NC}"
    else
        echo -e "${RED}绿色环境: 已停止${NC}"
    fi
    
    # JAR信息
    local current_jar=$(get_current_jar 2>/dev/null)
    if [ -n "$current_jar" ]; then
        echo "当前JAR: $current_jar"
    fi
}

# 查看日志
logs() {
    local lines=${1:-300}
    local active_env=$(get_active_env)
    local logfile="$APP_HOME/logs/$APP_NAME-${active_env}.log"
    
    if [ -f "$logfile" ]; then
        echo -e "${BLUE}查看 $active_env 环境日志 (最近 $lines 行):${NC}"
        tail -n "$lines" "$logfile"
    else
        echo -e "${RED}日志文件不存在: $logfile${NC}"
    fi
}

# 实时日志
logs_follow() {
    local active_env=$(get_active_env)
    local logfile="$APP_HOME/logs/$APP_NAME-${active_env}.log"
    
    if [ -f "$logfile" ]; then
        echo -e "${BLUE}实时查看 $active_env 环境日志:${NC}"
        tail -f "$logfile"
    else
        echo -e "${RED}日志文件不存在: $logfile${NC}"
    fi
}

# 按日期查询历史日志并可搜索关键字
resolve_logfile_by_date() {
    local base="$1"
    local date="$2"
    local log_dir="$APP_HOME/logs"

    if [ -z "$base" ]; then
        base="$APP_NAME"
    fi
    base="${base%.log}"

    local logfile=""
    local candidates=(
        "$log_dir/$base.$date.log"
        "$log_dir/$base-$date.log"
        "$log_dir/$base_$date.log"
    )

    for f in "${candidates[@]}"; do
        if [ -f "$f" ]; then
            logfile="$f"
            break
        fi
    done

    # 兼容不规则命名
    if [ -z "$logfile" ]; then
        local glob_files=("$log_dir/$base"*"$date"*.log)
        if [ -f "${glob_files[0]}" ]; then
            logfile="${glob_files[0]}"
        fi
    fi

    # 若日期为今天，尝试使用当前日志
    if [ -z "$logfile" ]; then
        if [ "$(date +%F 2>/dev/null)" = "$date" ] && [ -f "$log_dir/$base.log" ]; then
            logfile="$log_dir/$base.log"
        fi
    fi

    # 兼容不带日期的日志文件
    if [ -z "$logfile" ] && [ -f "$log_dir/$base.log" ]; then
        logfile="$log_dir/$base.log"
    fi

    if [ -z "$logfile" ]; then
        return 1
    fi

    echo "$logfile"
}

logs_search() {
    local arg1="$1"
    local arg2="$2"
    local arg3="$3"
    local arg4="$4"

    local base=""
    local date=""
    local keyword=""
    local limit=""
    local today=""
    if command -v date >/dev/null 2>&1; then
        today="$(date +%F 2>/dev/null)"
    fi
    local example_date="${today}"
    if [ -z "$example_date" ]; then
        example_date="YYYY-MM-DD"
    fi

    if [[ "$arg1" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]]; then
        date="$arg1"
        keyword="$arg2"
        limit="${arg3:-200}"
        base="$APP_NAME"
    else
        base="$arg1"
        date="$arg2"
        keyword="$arg3"
        limit="${arg4:-200}"
    fi

    if [ -z "$base" ]; then
        base="$APP_NAME"
    fi

    if [ -z "$date" ]; then
        if [ -n "$today" ]; then
            date="$today"
            echo -e "${YELLOW}未输入日期，默认使用今天: $date${NC}"
        else
            echo -e "${RED}用法: logs-search [日志名] [日期] [关键字] [行数]${NC}"
            echo -e "${YELLOW}示例: logs-search $example_date ERROR 200${NC}"
            echo -e "${YELLOW}示例: logs-search sys-info $example_date ERROR 200${NC}"
            return 1
        fi
    fi

    local logfile=$(resolve_logfile_by_date "$base" "$date")
    if [ -z "$logfile" ]; then
        echo -e "${RED}未找到日期 $date 的日志文件 (日志名=${base:-$APP_NAME}, 目录=$APP_HOME/logs)${NC}"
        return 1
    fi

    if [ -z "$keyword" ]; then
        echo -e "${BLUE}查看 $date 日志 (文件: $logfile, 最后 $limit 行):${NC}"
        tail -n "$limit" "$logfile"
        return 0
    fi

    echo -e "${BLUE}搜索 $date 日志 (文件: $logfile, 关键字: $keyword, 最多 $limit 行匹配):${NC}"
    local result=""
    if command -v rg >/dev/null 2>&1; then
        result=$(rg --no-heading -n "$keyword" "$logfile" | tail -n "$limit")
    else
        result=$(grep -n "$keyword" "$logfile" | tail -n "$limit")
    fi

    if [ -z "$result" ]; then
        echo -e "${YELLOW}未找到匹配关键字的日志: $keyword${NC}"
        return 1
    fi

    echo "$result"
}

# 导出日志文件（优先压缩，无法压缩则直接复制）
logs_export() {
    local arg1="$1"
    local arg2="$2"
    local arg3="$3"

    local base=""
    local date=""
    local output=""
    local today=""
    if command -v date >/dev/null 2>&1; then
        today="$(date +%F 2>/dev/null)"
    fi
    local example_date="${today}"
    if [ -z "$example_date" ]; then
        example_date="YYYY-MM-DD"
    fi

    if [[ "$arg1" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}$ ]]; then
        date="$arg1"
        output="$arg2"
        base="$APP_NAME"
    else
        base="$arg1"
        date="$arg2"
        output="$arg3"
    fi

    if [ -z "$base" ]; then
        base="$APP_NAME"
    fi

    if [ -z "$date" ]; then
        if [ -n "$today" ]; then
            date="$today"
            echo -e "${YELLOW}未输入日期，默认使用今天: $date${NC}"
        else
            echo -e "${RED}用法: logs-export [日志名] [日期] [输出名]${NC}"
            echo -e "${YELLOW}示例: logs-export $example_date${NC}"
            echo -e "${YELLOW}示例: logs-export sys-info $example_date mylog.tar.gz${NC}"
            return 1
        fi
    fi

    local logfile=$(resolve_logfile_by_date "$base" "$date")
    if [ -z "$logfile" ]; then
        echo -e "${RED}未找到日期 $date 的日志文件 (日志名=${base:-$APP_NAME}, 目录=$APP_HOME/logs)${NC}"
        return 1
    fi

    local export_dir="$APP_HOME/logs/exports"
    mkdir -p "$export_dir"

    local base_name="${base:-$APP_NAME}"
    base_name="${base_name%.log}"
    local out_name=""
    if [ -n "$output" ]; then
        out_name="$(basename "$output")"
    else
        out_name="${base_name}.${date}"
    fi

    local out_path=""
    if command -v tar >/dev/null 2>&1; then
        if [[ "$out_name" != *.tar.gz && "$out_name" != *.tgz ]]; then
            out_name="${out_name}.tar.gz"
        fi
        out_path="$export_dir/$out_name"
        tar -czf "$out_path" -C "$(dirname "$logfile")" "$(basename "$logfile")"
        if [ $? -ne 0 ]; then
            echo -e "${RED}压缩失败，尝试直接复制...${NC}"
            out_name="${base_name}.${date}.log"
            out_path="$export_dir/$out_name"
            cp "$logfile" "$out_path"
        fi
    elif command -v gzip >/dev/null 2>&1; then
        if [[ "$out_name" != *.gz ]]; then
            out_name="${out_name}.gz"
        fi
        out_path="$export_dir/$out_name"
        gzip -c "$logfile" > "$out_path"
        if [ $? -ne 0 ]; then
            echo -e "${RED}压缩失败，尝试直接复制...${NC}"
            out_name="${base_name}.${date}.log"
            out_path="$export_dir/$out_name"
            cp "$logfile" "$out_path"
        fi
    else
        out_name="${out_name%.tar.gz}"
        out_name="${out_name%.tgz}"
        out_name="${out_name%.gz}"
        if [[ "$out_name" != *.log ]]; then
            out_name="${out_name}.log"
        fi
        out_path="$export_dir/$out_name"
        cp "$logfile" "$out_path"
    fi

    if [ -f "$out_path" ]; then
        echo -e "${GREEN}导出成功: $out_path${NC}"
        return 0
    fi

    echo -e "${RED}导出失败${NC}"
    return 1
}

# 清理所有相关的java进程（紧急情况使用）
cleanup() {
    local jar_files=($(find "$APP_HOME" -maxdepth 1 -name "$APP_JAR_PATTERN" -type f 2>/dev/null | sort -V))
    local jar_count=${#jar_files[@]}
    
    if [ $jar_count -le $KEEP_HISTORY_JARS ]; then
        return 0
    fi
    
    local remove_count=$((jar_count - KEEP_HISTORY_JARS))
    echo -e "${YELLOW}清理旧文件，保留最新的 $KEEP_HISTORY_JARS 个${NC}"
    
    for ((i=0; i<remove_count; i++)); do
        local old_jar=$(basename "${jar_files[i]}")
        echo -e "${CYAN}删除: $old_jar${NC}"
        rm -f "${jar_files[i]}"
    done
}

# 显示帮助
help() {
    echo -e "${BLUE}若依蓝绿部署控制脚本${NC}"
    echo ""
    echo "用法: $0 {命令} [参数]"
    echo ""
    echo -e "${CYAN}基本命令:${NC}"
    echo "  start          - 启动服务"
    echo "  stop           - 停止服务"  
    echo "  restart        - 重启服务"
    echo "  deploy         - 蓝绿部署新版本"
    echo "  status         - 查看状态"
    echo "  logs [行数]    - 查看日志"
    echo "  logs-follow    - 实时日志"
    echo "  logs-search [日志名] [日期] [关键字] [行数] - 查询历史日志（日期可省略，默认今天）"
    echo "  logs-export [日志名] [日期] [输出名] - 导出日志（日期可省略，默认今天）"
    echo "  force-cleanup  - 强制清理所有进程"
    echo "  help           - 显示帮助"
    echo ""
    echo -e "${CYAN}配置信息:${NC}"
    echo "  蓝色端口: $BLUE_PORT"
    echo "  绿色端口: $GREEN_PORT" 
    echo "  代理端口: $PROXY_PORT"
}

# ==================== 主程序 ====================

# 权限检查
check_permissions

case "$1" in
    start)
        start
        ;;
    stop)
        stop
        ;;
    restart)
        restart
        ;;
    deploy)
        deploy
        ;;
    status)
        status
        ;;
    logs)
        logs "$2"
        ;;
    logs-follow)
        logs_follow
        ;;
    logs-search)
        logs_search "$2" "$3" "$4"
        ;;
    logs-export)
        logs_export "$2" "$3" "$4"
        ;;
    force-cleanup)
        force_cleanup
        ;;
    help|--help|-h)
        help
        ;;
    *)
        echo -e "${RED}错误: 无效命令 '$1'${NC}"
        echo ""
        help
        exit 1
        ;;
esac

exit $?
