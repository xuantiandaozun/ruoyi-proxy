package bootstrap

import (
	"os"
	"strings"
	"testing"

	"ruoyi-proxy/internal/hub"
)

func TestNormalizeIPAddress(t *testing.T) {
	if got := normalizeIPAddress(" 203.0.113.8\n"); got != "203.0.113.8" {
		t.Fatalf("规范化 IPv4 = %q", got)
	}
	if got := normalizeIPAddress("not-an-ip"); got != "" {
		t.Fatalf("无效 IP 应返回空字符串，实际 = %q", got)
	}
}

func TestMigrateLegacySpokeProfileOnlyAsksMissingUserFields(t *testing.T) {
	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldDir); err != nil {
			t.Errorf("恢复测试目录失败: %v", err)
		}
	})

	oldDetector := publicIPDetector
	publicIPDetector = func() string { return "203.0.113.10" }
	t.Cleanup(func() { publicIPDetector = oldDetector })

	profile := hub.SpokeProfile{
		SchemaVersion: 0,
		Hostname:      "old-host",
		ProjectType:   "java",
		Domain:        "existing.example.com",
	}
	var prompts []string
	io := &CLIO{
		Print: func(string) {},
		Ask: func(prompt string) (string, error) {
			prompts = append(prompts, prompt)
			switch {
			case strings.Contains(prompt, "别名"):
				return "生产-订单服务", nil
			case strings.Contains(prompt, "项目名称"):
				return "ruoyi-admin", nil
			case strings.Contains(prompt, "备注"):
				return "张三负责", nil
			default:
				t.Fatalf("不应询问已有或可自动检测的字段: %s", prompt)
				return "", nil
			}
		},
	}
	if err := migrateSpokeProfile(io, &profile); err != nil {
		t.Fatal(err)
	}
	if profile.SchemaVersion != currentSpokeProfileVersion {
		t.Fatalf("档案版本 = %d", profile.SchemaVersion)
	}
	if profile.Label != "生产-订单服务" || profile.ProjectName != "ruoyi-admin" || profile.Description != "张三负责" {
		t.Fatalf("用户字段未正确补齐: %#v", profile)
	}
	if profile.ProjectType != "java" || profile.Domain != "existing.example.com" {
		t.Fatalf("旧档案已有字段被覆盖: %#v", profile)
	}
	if profile.PublicIP != "203.0.113.10" || profile.OS == "" || profile.Arch == "" {
		t.Fatalf("系统字段未自动补齐: %#v", profile)
	}
	if len(prompts) != 3 {
		t.Fatalf("询问次数 = %d，期望 3", len(prompts))
	}
}
