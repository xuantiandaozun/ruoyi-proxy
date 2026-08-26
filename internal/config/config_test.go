package config

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigDoesNotLogSuccessfulBackgroundRead(t *testing.T) {
	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tempDir := t.TempDir()
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(oldDir); err != nil {
			t.Errorf("恢复工作目录失败: %v", err)
		}
	}()

	if err := os.MkdirAll(filepath.Dir(ConfigFile), 0755); err != nil {
		t.Fatal(err)
	}
	data := []byte(`{"services":{"default":{"name":"test","blue_target":"http://127.0.0.1:8080","green_target":"http://127.0.0.1:8081","active_env":"blue","jar_file":"*.jar","app_name":"test"}}}`)
	if err := os.WriteFile(ConfigFile, data, 0644); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	oldWriter := log.Writer()
	log.SetOutput(&output)
	defer log.SetOutput(oldWriter)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Services) != 1 {
		t.Fatalf("服务数量 = %d，期望 1", len(cfg.Services))
	}
	if strings.Contains(output.String(), "配置文件加载成功") {
		t.Fatalf("后台成功读取不应输出日志: %q", output.String())
	}
}
