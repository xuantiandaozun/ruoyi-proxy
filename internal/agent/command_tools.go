package agent

import (
	"fmt"
	"strings"

	"ruoyi-proxy/internal/commandcatalog"
)

var cliCommandQueryTool = ToolDef{
	Name:        "query_cli_commands",
	Description: "查询当前版本 ruoyi-proxy 实际支持的 CLI 斜杠命令、用法和说明。用户询问有哪些命令、某类命令或自然语言需求对应哪个 CLI 命令时使用；不要凭记忆猜测命令",
	ReadOnly:    true,
	Parameters: map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "可选关键词，例如 Hub、数据库、日志、部署或命令名；留空返回全部命令",
			},
			"category": map[string]interface{}{
				"type":        "string",
				"description": "可选精确分类；通常优先使用 query 模糊查询",
			},
		},
		"required": []string{},
	},
}

func init() {
	AllTools = append(AllTools, cliCommandQueryTool)
}

func queryCLICommands(query, category string) (string, error) {
	commands := commandcatalog.Search(query, category)
	if len(commands) == 0 {
		return "未找到匹配的 CLI 命令；可清空 query 和 category 查询全部命令", nil
	}
	var lines []string
	currentCategory := ""
	for _, command := range commands {
		if command.Category != currentCategory {
			if len(lines) > 0 {
				lines = append(lines, "")
			}
			currentCategory = command.Category
			lines = append(lines, "【"+currentCategory+"】")
		}
		lines = append(lines, fmt.Sprintf("%s — %s", command.Usage, command.Description))
	}
	return strings.Join(lines, "\n"), nil
}
