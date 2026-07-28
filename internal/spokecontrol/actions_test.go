package spokecontrol

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"ruoyi-proxy/internal/hub"
)

func TestDatabaseQueryActionRejectsWriteSQL(t *testing.T) {
	job := hub.ControlJob{
		ID: "job-db-write",
		Action: &hub.ControlAction{
			Type: hub.ControlActionDatabaseQuery,
			Params: map[string]string{
				"profile": "production",
				"sql":     "DELETE FROM users",
			},
		},
	}
	result := executeStructuredAction(context.Background(), job)
	if result.Status != hub.ControlJobFailed || result.Error != "结构化 database_query 当前仅允许只读 SQL" {
		t.Fatalf("数据库写操作未被拒绝: %#v", result)
	}
}

func TestResolveServiceScriptFromWorkDir(t *testing.T) {
	root := t.TempDir()
	script := filepath.Join(root, "scripts", "service.sh")
	if err := os.MkdirAll(filepath.Dir(script), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(script, []byte("#!/usr/bin/env bash\n"), 0755); err != nil {
		t.Fatal(err)
	}
	resolved, err := resolveServiceScript("", root)
	if err != nil {
		t.Fatal(err)
	}
	expected, _ := filepath.Abs(script)
	if resolved != expected {
		t.Fatalf("脚本路径 = %q, 期望 %q", resolved, expected)
	}
}

func TestTargetPort(t *testing.T) {
	if got := targetPort("http://127.0.0.1:9080", "8080"); got != "9080" {
		t.Fatalf("URL 端口 = %s", got)
	}
	if got := targetPort("", "8080"); got != "8080" {
		t.Fatalf("回退端口 = %s", got)
	}
}
