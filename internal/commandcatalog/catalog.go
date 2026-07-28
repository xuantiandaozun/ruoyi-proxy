package commandcatalog

import (
	"sort"
	"strings"
)

// Command CLI 斜杠命令元数据。
type Command struct {
	Name        string `json:"name"`
	Usage       string `json:"usage"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Operational bool   `json:"operational"`
}

// commands 是 CLI 与 AI 共用的唯一命令目录。
// 新增斜杠命令时只需在这里登记，菜单、命令分发识别和 AI 查询会自动同步。
var commands = []Command{
	{Name: "/sessions", Usage: "/sessions", Description: "查看历史会话", Category: "会话"},
	{Name: "/load", Usage: "/load", Description: "加载历史会话", Category: "会话"},
	{Name: "/new", Usage: "/new", Description: "新建会话", Category: "会话"},
	{Name: "/current", Usage: "/current", Description: "查看当前会话信息", Category: "会话"},
	{Name: "/help", Usage: "/help", Description: "查看命令说明", Category: "帮助"},
	{Name: "/commands", Usage: "/commands", Description: "查看运维命令列表", Category: "帮助", Operational: true},
	{Name: "/cls", Usage: "/cls", Description: "清屏", Category: "帮助", Operational: true},
	{Name: "/exit", Usage: "/exit", Description: "退出 Agent 模式", Category: "帮助"},

	{Name: "/start", Usage: "/start", Description: "启动当前服务", Category: "服务", Operational: true},
	{Name: "/stop", Usage: "/stop", Description: "停止当前服务", Category: "服务", Operational: true},
	{Name: "/restart", Usage: "/restart", Description: "重启当前服务", Category: "服务", Operational: true},
	{Name: "/deploy", Usage: "/deploy", Description: "执行蓝绿部署", Category: "服务", Operational: true},
	{Name: "/deploy-lowmem", Usage: "/deploy-lowmem", Description: "执行低内存部署", Category: "服务", Operational: true},
	{Name: "/quick-deploy", Usage: "/quick-deploy", Description: "执行快速部署", Category: "服务", Operational: true},
	{Name: "/status", Usage: "/status", Description: "查看服务状态", Category: "服务", Operational: true},
	{Name: "/detail", Usage: "/detail", Description: "查看详细状态", Category: "服务", Operational: true},
	{Name: "/logs", Usage: "/logs [行数]", Description: "查看服务日志", Category: "日志", Operational: true},
	{Name: "/logs-follow", Usage: "/logs-follow", Description: "实时跟踪服务日志", Category: "日志", Operational: true},
	{Name: "/logs-search", Usage: "/logs-search [日志文件] [日期] [关键字]", Description: "搜索服务日志", Category: "日志", Operational: true},
	{Name: "/logs-export", Usage: "/logs-export [日志文件] [日期]", Description: "导出服务日志", Category: "日志", Operational: true},
	{Name: "/switch", Usage: "/switch [blue|green]", Description: "切换蓝绿环境", Category: "服务", Operational: true},
	{Name: "/service-add", Usage: "/service-add", Description: "添加服务", Category: "多服务", Operational: true},
	{Name: "/service-list", Usage: "/service-list", Description: "查看服务列表", Category: "多服务", Operational: true},
	{Name: "/service-remove", Usage: "/service-remove", Description: "删除服务", Category: "多服务", Operational: true},
	{Name: "/service-switch", Usage: "/service-switch", Description: "切换当前服务", Category: "多服务", Operational: true},

	{Name: "/proxy-start", Usage: "/proxy-start", Description: "启动代理进程", Category: "代理", Operational: true},
	{Name: "/proxy-stop", Usage: "/proxy-stop", Description: "停止代理进程", Category: "代理", Operational: true},
	{Name: "/proxy-restart", Usage: "/proxy-restart", Description: "重启代理进程", Category: "代理", Operational: true},
	{Name: "/proxy-status", Usage: "/proxy-status", Description: "查看代理状态", Category: "代理", Operational: true},

	{Name: "/config", Usage: "/config", Description: "查看配置", Category: "配置", Operational: true},
	{Name: "/config-edit", Usage: "/config-edit", Description: "编辑配置", Category: "配置", Operational: true},
	{Name: "/jvm-config", Usage: "/jvm-config", Description: "配置 JVM 预设", Category: "配置", Operational: true},
	{Name: "/agent-config", Usage: "/agent-config", Description: "配置 AI 或注册 Hub", Category: "配置", Operational: true},
	{Name: "/init", Usage: "/init", Description: "运行环境初始化向导", Category: "系统", Operational: true},
	{Name: "/cert", Usage: "/cert [域名]", Description: "申请或管理 HTTPS 证书", Category: "系统", Operational: true},
	{Name: "/enable-https", Usage: "/enable-https", Description: "启用 HTTPS", Category: "系统", Operational: true},
	{Name: "/disable-https", Usage: "/disable-https", Description: "禁用 HTTPS", Category: "系统", Operational: true},
	{Name: "/info", Usage: "/info", Description: "查看系统信息", Category: "系统", Operational: true},
	{Name: "/monitor", Usage: "/monitor", Description: "进入监控模式", Category: "系统", Operational: true},
	{Name: "/quick", Usage: "/quick", Description: "查看常用命令", Category: "帮助", Operational: true},
	{Name: "/self-check", Usage: "/self-check", Description: "运行环境自检", Category: "系统", Operational: true},
	{Name: "/fix-nginx-hub", Usage: "/fix-nginx-hub", Description: "让 AI 修复 Nginx Hub 路由", Category: "系统", Operational: true},

	{Name: "/hub-enable", Usage: "/hub-enable", Description: "启用 Hub 网关", Category: "Hub", Operational: true},
	{Name: "/hub-disable", Usage: "/hub-disable", Description: "禁用 Hub 网关", Category: "Hub", Operational: true},
	{Name: "/hub-token", Usage: "/hub-token", Description: "生成 Hub 注册 Token", Category: "Hub", Operational: true},
	{Name: "/hub-status", Usage: "/hub-status [spoke-id]", Description: "查看 Hub Spoke 列表或节点详情", Category: "Hub", Operational: true},
	{Name: "/hub-select", Usage: "/hub-select [spoke-id|clear]", Description: "选择后续远程命令的默认 Spoke", Category: "Hub", Operational: true},
	{Name: "/hub-spoke", Usage: "/hub-spoke <spoke-id>", Description: "查看单个 Spoke 详情", Category: "Hub", Operational: true},
	{Name: "/hub-node-set", Usage: "/hub-node-set <spoke-id> <key=value...>", Description: "设置节点标签、分组、维护状态和能力白名单", Category: "Hub", Operational: true},
	{Name: "/hub-revoke", Usage: "/hub-revoke <spoke-id>", Description: "吊销 Spoke", Category: "Hub", Operational: true},
	{Name: "/hub-exec", Usage: "/hub-exec <spoke-id> <命令>", Description: "在 Spoke 执行远程命令", Category: "Hub", Operational: true},
	{Name: "/hub-action", Usage: "/hub-action <spoke-id[,spoke-id]|@分组> <动作> [参数]", Description: "在一个或多个 Spoke 执行结构化动作", Category: "Hub", Operational: true},
	{Name: "/hub-jobs", Usage: "/hub-jobs [spoke-id]", Description: "查看远程任务和结果", Category: "Hub", Operational: true},
	{Name: "/hub-cancel", Usage: "/hub-cancel <job-id>", Description: "取消等待中的远程任务", Category: "Hub", Operational: true},
	{Name: "/hub-retry", Usage: "/hub-retry <job-id>", Description: "重试失败、超时或已取消任务", Category: "Hub", Operational: true},
	{Name: "/spoke-agent-install", Usage: "/spoke-agent-install", Description: "安装 Spoke 常驻服务", Category: "Hub", Operational: true},
	{Name: "/spoke-agent-status", Usage: "/spoke-agent-status", Description: "查看 Spoke 常驻服务", Category: "Hub", Operational: true},

	{Name: "/db-discover", Usage: "/db-discover [项目目录]", Description: "发现并保存项目数据库", Category: "数据库", Operational: true},
	{Name: "/db-add", Usage: "/db-add", Description: "添加远程 MySQL 项目连接", Category: "数据库", Operational: true},
	{Name: "/db-list", Usage: "/db-list", Description: "查看数据库连接档案", Category: "数据库", Operational: true},
	{Name: "/db-test", Usage: "/db-test <档案ID或名称>", Description: "测试数据库连接", Category: "数据库", Operational: true},
	{Name: "/db-schema", Usage: "/db-schema <档案ID或名称>", Description: "查看数据库表结构", Category: "数据库", Operational: true},
	{Name: "/db-query", Usage: "/db-query <档案ID或名称> <SQL>", Description: "执行受控 SQL", Category: "数据库", Operational: true},

	{Name: "/tasks", Usage: "/tasks", Description: "查看 AI 定时任务", Category: "定时任务", Operational: true},
	{Name: "/task-add", Usage: "/task-add", Description: "创建 AI 定时任务", Category: "定时任务", Operational: true},
	{Name: "/task-enable", Usage: "/task-enable <id>", Description: "启用定时任务", Category: "定时任务", Operational: true},
	{Name: "/task-disable", Usage: "/task-disable <id>", Description: "暂停定时任务", Category: "定时任务", Operational: true},
	{Name: "/task-run", Usage: "/task-run <id>", Description: "立即执行定时任务", Category: "定时任务", Operational: true},
	{Name: "/task-history", Usage: "/task-history [id]", Description: "查看定时任务历史", Category: "定时任务", Operational: true},
	{Name: "/task-delete", Usage: "/task-delete <id>", Description: "删除定时任务", Category: "定时任务", Operational: true},
}

// All 返回全部命令的副本。
func All() []Command {
	result := make([]Command, len(commands))
	copy(result, commands)
	return result
}

// Search 按命令名、用法、描述和分类查询命令。
func Search(query, category string) []Command {
	query = strings.ToLower(strings.TrimSpace(query))
	category = strings.ToLower(strings.TrimSpace(category))
	result := make([]Command, 0, len(commands))
	for _, command := range commands {
		if category != "" && strings.ToLower(command.Category) != category {
			continue
		}
		haystack := strings.ToLower(strings.Join([]string{
			command.Name, command.Usage, command.Description, command.Category,
		}, " "))
		if query != "" && !strings.Contains(haystack, query) {
			continue
		}
		result = append(result, command)
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Category == result[j].Category {
			return result[i].Name < result[j].Name
		}
		return result[i].Category < result[j].Category
	})
	return result
}

// IsOperational 判断命令是否由 CLI 运维分发器处理。
func IsOperational(name string) bool {
	name = "/" + strings.TrimPrefix(strings.ToLower(strings.TrimSpace(name)), "/")
	for _, command := range commands {
		if command.Name == name {
			return command.Operational
		}
	}
	return false
}
