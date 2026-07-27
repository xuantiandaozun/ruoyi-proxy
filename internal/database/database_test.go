package database

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverSpringDatasource(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "application-dev.yml")
	content := "spring:\n  datasource:\n    url: jdbc:mysql://127.0.0.1:3307/demo?useUnicode=true\n    username: demo_user\n    password: ${DB_PASSWORD:demo_pass}\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	profiles, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(profiles) != 1 {
		t.Fatalf("期望发现 1 个连接，实际 %d", len(profiles))
	}
	p := profiles[0]
	if p.Host != "127.0.0.1" || p.Port != 3307 || p.Database != "demo" || p.Username != "demo_user" {
		t.Fatalf("连接解析错误: %+v", p)
	}
	if p.Password != "demo_pass" || p.PasswordEnv != "DB_PASSWORD" {
		t.Fatalf("密码引用解析错误: %+v", p)
	}
}

func TestSaveRemoteConnectionsForMultipleProjects(t *testing.T) {
	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldDir)
	first, err := SaveConnection(ConnectionInput{ProjectName: "订单系统", Environment: "prod", Host: "mysql.example.com", Port: 3306, Database: "orders", Username: "ops", Password: "first-secret", Remark: "远程生产库"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := SaveConnection(ConnectionInput{ProjectName: "会员系统", Environment: "prod", Host: "mysql.example.com", Port: 3306, Database: "members", Username: "ops", Password: "second-secret"})
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == second.ID {
		t.Fatal("同一服务器的不同项目不应共享连接 ID")
	}
	profilesRaw, err := os.ReadFile(profilesFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(profilesRaw) == "" || containsSecret(profilesRaw, "first-secret") || containsSecret(profilesRaw, "second-secret") {
		t.Fatal("连接档案不应包含明文密码")
	}
	secretsRaw, err := os.ReadFile(secretsFile)
	if err != nil {
		t.Fatal(err)
	}
	var secrets map[string]string
	if err := json.Unmarshal(secretsRaw, &secrets); err != nil {
		t.Fatal(err)
	}
	if secrets[first.ID] != "first-secret" || secrets[second.ID] != "second-secret" {
		t.Fatal("独立密钥文件内容不正确")
	}
	loaded, err := LoadProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 2 || !loaded[0].HasPassword || !loaded[1].HasPassword {
		t.Fatalf("加载档案错误: %+v", loaded)
	}
}

func containsSecret(raw []byte, secret string) bool { return strings.Contains(string(raw), secret) }

func TestIsReadOnlySQL(t *testing.T) {
	cases := []struct {
		sql  string
		want bool
	}{
		{"SELECT * FROM users", true},
		{"/* inspect */ SHOW TABLES", true},
		{"EXPLAIN SELECT 1", true},
		{"WITH x AS (SELECT 1) SELECT * FROM x", false},
		{"SELECT 1; DELETE FROM users", false},
		{"SELECT * FROM users FOR UPDATE", false},
		{"SELECT data INTO OUTFILE '/tmp/x' FROM users", false},
		{"UPDATE users SET name='x'", false},
	}
	for _, tc := range cases {
		if got := IsReadOnlySQL(tc.sql); got != tc.want {
			t.Errorf("IsReadOnlySQL(%q)=%v, want %v", tc.sql, got, tc.want)
		}
	}
}
