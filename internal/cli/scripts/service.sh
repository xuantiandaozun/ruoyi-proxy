#!/bin/bash

# 若依项目蓝绿部署控制脚本 - Java 17 + AppCDS
# 支持多服务部署，每个服务放在独立目录

# ==================== 配置区域 ====================
# 基础配置（可通过环境变量覆盖）
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

# AppCDS配置
ENABLE_CDS=true
WORKDIR_PREFIX="work"

# JVM参数配置
BASE_JVM_OPTS="-server \
-Xms256m \
-Xmx800m \
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
# APP_HOME 优先使用环境变量，如果没有设置则使用脚本父目录
if [ -z "$APP_HOME" ]; then
    APP_HOME="$(cd "$SCRIPT_DIR/.." && pwd)"
fi
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

# 获取jar对应的工作目录
get_jar_workdir() {
    local jar_file="$1"
    if [ -z "$jar_file" ]; then
        echo ""
        return 1
    fi
    local jar_name=$(basename "$jar_file" .jar)
    echo "$APP_HOME/${WORKDIR_PREFIX}_$jar_name"
}

# 获取jar对应的CDS文件
get_jar_cds() {
    local jar_file="$1"
    if [ -z "$jar_file" ]; then
        echo ""
        return 1
    fi
    local workdir=$(get_jar_workdir "$jar_file")
    echo "$workdir/app.jsa"
}

# 获取jar对应的解压文件
get_jar_extracted() {
    local jar_file="$1"
    if [ -z "$jar_file" ]; then
        echo ""
        return 1
    fi
    local workdir=$(get_jar_workdir "$jar_file")
    echo "$workdir/$jar_file"
}

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
    # ??1?????????????
    if command -v nc >/dev/null 2>&1; then
        if nc -z localhost $PROXY_PORT 2>/dev/null; then
            echo "running"
            return 0
        fi
    fi
    
    # ??2?????nc???curl??????
    if command -v curl >/dev/null 2>&1; then
        if curl -s --connect-timeout 2 "localhost:$PROXY_PORT" >/dev/null 2>&1; then
            echo "running"
            return 0
        fi
    fi
    
    # ??3??????PID????????
    if [ -f "$PROXY_PID_FILE" ] && [ -s "$PROXY_PID_FILE" ]; then
        local pid=$(cat "$PROXY_PID_FILE")
        if kill -0 $pid 2>/dev/null; then
            echo "running"
            return 0
        else
            # PID????????
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

# 强制清理旧的CDS缓存（修复CDS不刷新问题）
force_clean_cds_cache() {
    local jar_file="$1"
    if [ -z "$jar_file" ]; then
        return 1
    fi
    
    local workdir=$(get_jar_workdir "$jar_file")
    local cds_file=$(get_jar_cds "$jar_file")
    
    # 如果jar文件比CDS文件新，或者CDS文件损坏，强制重新生成
    if [ -f "$cds_file" ]; then
        if [ "$APP_HOME/$jar_file" -nt "$cds_file" ]; then
            echo -e "${YELLOW}检测到jar文件更新，清理旧的CDS缓存...${NC}"
            rm -rf "$workdir"
            return 0
        fi
        
        # 检查CDS文件是否有效（大小检查）
        if [ ! -s "$cds_file" ]; then
            echo -e "${YELLOW}检测到CDS文件损坏，清理缓存...${NC}"
            rm -rf "$workdir"
            return 0
        fi
    fi
    
    return 1
}

# 解压JAR文件
extract_jar() {
    local jar_file="$1"
    if [ -z "$jar_file" ]; then
        echo -e "${RED}错误: 未指定jar文件${NC}"
        return 1
    fi
    
    echo -e "${CYAN}解压JAR文件: $jar_file${NC}"
    
    local workdir=$(get_jar_workdir "$jar_file")
    local extracted_jar=$(get_jar_extracted "$jar_file")
    
    # 强制清理旧缓存
    force_clean_cds_cache "$jar_file"
    
    if [ -f "$extracted_jar" ]; then
        echo -e "${GREEN}JAR已解压，跳过${NC}"
        return 0
    fi
    
    # 清理并创建工作目录
    rm -rf "$workdir"
    mkdir -p "$workdir"
    
    cd "$APP_HOME"
    java -Djarmode=tools -jar "$jar_file" extract --destination "$workdir"
    
    if [ $? -eq 0 ] && [ -f "$extracted_jar" ]; then
        echo -e "${GREEN}JAR文件解压成功${NC}"
        return 0
    else
        echo -e "${RED}JAR文件解压失败${NC}"
        return 1
    fi
}

# 生成CDS归档文件
generate_cds() {
    local jar_file="$1"
    if [ -z "$jar_file" ]; then
        echo -e "${RED}错误: 未指定jar文件${NC}"
        return 1
    fi
    
    echo -e "${CYAN}生成AppCDS归档: $jar_file${NC}"
    
    local workdir=$(get_jar_workdir "$jar_file")
    local extracted_jar=$(get_jar_extracted "$jar_file")
    local cds_file=$(get_jar_cds "$jar_file")
    
    if [ ! -f "$extracted_jar" ]; then
        if ! extract_jar "$jar_file"; then
            return 1
        fi
    fi
    
    # 删除旧CDS文件
    rm -f "$cds_file"
    
    mkdir -p "$APP_HOME/logs"
    
    # 根据内存调整参数
    local available_mem=512
    if command -v free >/dev/null 2>&1; then
        available_mem=$(free -m | awk 'NR==2{print $7}')
    fi
    
    local CDS_BUILD_OPTS
    if [ "$available_mem" -lt 800 ]; then
        CDS_BUILD_OPTS="-server -Xms128m -Xmx384m -XX:+UseSerialGC -Dfile.encoding=UTF-8"
    else
        CDS_BUILD_OPTS="-server -Xms256m -Xmx512m -XX:+UseG1GC -Dfile.encoding=UTF-8"
    fi
    
    cd "$APP_HOME"
    timeout 300s java $CDS_BUILD_OPTS \
        -Xlog:cds:"$APP_HOME/logs/cds.log":time,tags \
        -XX:ArchiveClassesAtExit="$cds_file" \
        -Dspring.context.exit=onRefresh \
        -Dspring.profiles.active=$SPRING_PROFILE \
        -jar "$extracted_jar" \
        > "$APP_HOME/logs/cds-generation.log" 2>&1
    
    local exit_code=$?
    
    if [ $exit_code -eq 0 ] && [ -f "$cds_file" ] && [ -s "$cds_file" ]; then
        echo -e "${GREEN}CDS归档生成成功${NC}"
        return 0
    else
        echo -e "${RED}CDS归档生成失败 (退出码: $exit_code)${NC}"
        return 1
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
    
    if [ ! -f "$APP_HOME/$jar_file" ]; then
        echo -e "${RED}错误: 找不到jar文件 $APP_HOME/$jar_file${NC}"
        return 1
    fi
    
    mkdir -p "$APP_HOME/logs"
    
    # 构建启动命令
    local JAVA_OPTS="$BASE_JVM_OPTS"
    local JAR_FILE="$APP_HOME/$jar_file"
    
    # AppCDS支持
    if [ "$ENABLE_CDS" = "true" ]; then
        local cds_file=$(get_jar_cds "$jar_file")
        local extracted_jar=$(get_jar_extracted "$jar_file")
        
        if [ -f "$cds_file" ] && [ -f "$extracted_jar" ]; then
            JAVA_OPTS="$BASE_JVM_OPTS -XX:SharedArchiveFile=$cds_file"
            JAR_FILE="$extracted_jar"
            echo -e "${GREEN}使用AppCDS加速启动${NC}"
        fi
    fi
    
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
        echo -e "${RED}??: ????? blue ? green${NC}"
        return 1
    fi
    
    echo -e "${BLUE}??? $target_env ??${NC}"
    set_active_env "$target_env"
    
    echo -e "${YELLOW}??: ?????????????????${NC}"
    return 0
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
    
    # 准备CDS
    if [ "$ENABLE_CDS" = "true" ]; then
        echo -e "${YELLOW}准备AppCDS归档...${NC}"
        if extract_jar "$jar_file" && generate_cds "$jar_file"; then
            echo -e "${GREEN}AppCDS准备完成${NC}"
        fi
    fi
    
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
    
    # 准备CDS
    if [ "$ENABLE_CDS" = "true" ]; then
        echo -e "${YELLOW}准备AppCDS归档...${NC}"
        if extract_jar "$new_jar" && generate_cds "$new_jar"; then
            echo -e "${GREEN}AppCDS准备完成${NC}"
        fi
    fi
    
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
        
        # CDS状态
        if [ "$ENABLE_CDS" = "true" ]; then
            local cds_file=$(get_jar_cds "$current_jar")
            if [ -f "$cds_file" ]; then
                echo "AppCDS: 已启用"
            else
                echo "AppCDS: 未生成"
            fi
        fi
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
        
        local old_workdir=$(get_jar_workdir "$old_jar")
        if [ -d "$old_workdir" ]; then
            rm -rf "$old_workdir"
        fi
    done
    
    # 清理孤儿work目录
    echo -e "${YELLOW}清理孤儿work目录...${NC}"
    for workdir in "$APP_HOME"/${WORKDIR_PREFIX}_*; do
        if [ -d "$workdir" ]; then
            local jar_name=$(basename "$workdir" | sed "s/^${WORKDIR_PREFIX}_//")
            if [ ! -f "$APP_HOME/$jar_name.jar" ]; then
                echo -e "${CYAN}删除孤儿目录: $(basename "$workdir")${NC}"
                rm -rf "$workdir"
            fi
        fi
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
    echo "  force-cleanup  - 强制清理所有进程"
    echo "  help           - 显示帮助"
    echo ""
    echo -e "${CYAN}配置信息:${NC}"
    echo "  蓝色端口: $BLUE_PORT"
    echo "  绿色端口: $GREEN_PORT" 
    echo "  代理端口: $PROXY_PORT"
    echo "  AppCDS: $ENABLE_CDS"
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