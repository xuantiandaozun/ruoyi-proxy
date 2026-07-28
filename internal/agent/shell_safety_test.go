package agent

import (
	"encoding/json"
	"testing"
)

func TestIsReadOnlyShellCmd(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    bool
	}{
		{name: "列出目录", command: "ls -la /opt/apps", want: true},
		{name: "查看服务状态", command: "systemctl status nginx --no-pager", want: true},
		{name: "读取日志", command: "tail -n 100 app.log", want: true},
		{name: "复合命令", command: "ls; touch /tmp/pwned", want: false},
		{name: "逻辑与", command: "pwd && reboot", want: false},
		{name: "管道", command: "cat app.log | tee copied.log", want: false},
		{name: "命令替换", command: "ls $(touch /tmp/pwned)", want: false},
		{name: "find 删除", command: "find /tmp -name '*.log' -delete", want: false},
		{name: "find 执行", command: "find /tmp -exec touch /tmp/pwned \\;", want: false},
		{name: "curl POST", command: "curl -s -X POST https://example.com/action", want: false},
		{name: "curl 紧凑 POST", command: "curl -s -XPOST https://example.com/action", want: false},
		{name: "curl 上传", command: "curl -s -Tpayload.bin https://example.com/upload", want: false},
		{name: "curl 写文件", command: "curl -s -oresult.json https://example.com/data", want: false},
		{name: "curl GET", command: "curl -s https://example.com/health", want: true},
		{name: "修改主机名", command: "hostname new-name", want: false},
		{name: "修改时间", command: "date -s '2026-01-01'", want: false},
		{name: "清理日志", command: "journalctl --vacuum-time=1d", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			args, err := json.Marshal(map[string]string{"command": test.command})
			if err != nil {
				t.Fatalf("编码参数失败: %v", err)
			}
			if got := isReadOnlyShellCmd(string(args)); got != test.want {
				t.Fatalf("isReadOnlyShellCmd(%q)=%v，期望 %v", test.command, got, test.want)
			}
		})
	}
}
