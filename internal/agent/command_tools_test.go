package agent

import (
	"strings"
	"testing"
)

func TestQueryCLICommands(t *testing.T) {
	result, err := queryCLICommands("数据库", "")
	if err != nil {
		t.Fatalf("查询命令失败: %v", err)
	}
	for _, expected := range []string{"/db-add", "/db-discover", "/db-query"} {
		if !strings.Contains(result, expected) {
			t.Fatalf("查询结果缺少 %s:\n%s", expected, result)
		}
	}
	if strings.Contains(result, "/hub-exec") {
		t.Fatalf("关键词查询返回了无关命令:\n%s", result)
	}
}

func TestQueryCLICommandsToolIsReadOnly(t *testing.T) {
	for _, tool := range AllTools {
		if tool.Name == "query_cli_commands" {
			if !tool.ReadOnly {
				t.Fatal("命令查询工具必须是只读工具")
			}
			return
		}
	}
	t.Fatal("未注册 query_cli_commands 工具")
}
