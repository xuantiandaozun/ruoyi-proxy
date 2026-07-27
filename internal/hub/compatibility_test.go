package hub

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestLegacySpokeRegistrationFlow 固化旧版 Spoke 使用的 v1 注册与档案协议。
func TestLegacySpokeRegistrationFlow(t *testing.T) {
	useTemporaryHubStore(t)

	tokenRecorder := httptest.NewRecorder()
	tokenRequest := httptest.NewRequest(http.MethodGet, "/__hub__/v1/token", nil)
	RegisterTokenHandler(tokenRecorder, tokenRequest)
	if tokenRecorder.Code != http.StatusOK {
		t.Fatalf("旧版领取 Token 失败: HTTP %d body=%s", tokenRecorder.Code, tokenRecorder.Body.String())
	}
	var tokenResponse struct {
		Token     string `json:"token"`
		ExpiresIn int    `json:"expires_in"`
	}
	if err := json.Unmarshal(tokenRecorder.Body.Bytes(), &tokenResponse); err != nil {
		t.Fatal(err)
	}
	if tokenResponse.Token == "" || tokenResponse.ExpiresIn != 900 {
		t.Fatalf("旧版 Token 响应字段异常: %#v", tokenResponse)
	}

	registerBody, _ := json.Marshal(map[string]string{"token": tokenResponse.Token})
	registerRecorder := httptest.NewRecorder()
	registerRequest := httptest.NewRequest(http.MethodPost, "/__hub__/v1/register", bytes.NewReader(registerBody))
	RegisterHandler(registerRecorder, registerRequest)
	if registerRecorder.Code != http.StatusOK {
		t.Fatalf("旧版注册失败: HTTP %d body=%s", registerRecorder.Code, registerRecorder.Body.String())
	}
	var registered registerResponse
	if err := json.Unmarshal(registerRecorder.Body.Bytes(), &registered); err != nil {
		t.Fatal(err)
	}
	if registered.SpokeID == "" || registered.Token == "" {
		t.Fatalf("旧版注册响应字段异常: %#v", registered)
	}

	// 只提交旧版本已有字段，不包含公网 IP、系统架构等新增字段。
	legacyProfile := []byte(`{"hostname":"legacy-host","label":"旧生产节点","project_name":"ruoyi-admin","project_type":"java","description":"legacy client","domain":"legacy.example.com","app_home":"/opt/app"}`)
	profileRecorder := httptest.NewRecorder()
	profileRequest := httptest.NewRequest(http.MethodPost, "/__hub__/v1/profile", bytes.NewReader(legacyProfile))
	profileRequest.RemoteAddr = "198.51.100.20:32100"
	profileRequest.Header.Set("Authorization", "Bearer "+registered.Token)
	ProfileHandler(profileRecorder, profileRequest)
	if profileRecorder.Code != http.StatusOK {
		t.Fatalf("旧版档案上报失败: HTTP %d body=%s", profileRecorder.Code, profileRecorder.Body.String())
	}
	record, ok := GetSpoke(registered.SpokeID)
	if !ok || record.Profile == nil || record.Profile.Hostname != "legacy-host" {
		t.Fatalf("旧版档案未正确保存: %#v", record)
	}
}

// TestLegacyPendingTokenFileMigration 验证新 Hub 可读取旧版单 Token 文件。
func TestLegacyPendingTokenFileMigration(t *testing.T) {
	useTemporaryHubStore(t)
	token := "legacy-one-time-token"
	record := pendingTokenRecord{Token: token, ExpiresAt: time.Now().Add(time.Minute)}
	data, _ := json.Marshal(record)
	if err := os.MkdirAll("configs", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pendingTokenFile, data, 0600); err != nil {
		t.Fatal(err)
	}
	if !consumeRegisterToken(token) {
		t.Fatal("新 Hub 未能消费旧版单 Token 文件")
	}
}

// TestLegacySpokeStoreMigration 验证新增档案字段不会破坏旧注册表加载。
func TestLegacySpokeStoreMigration(t *testing.T) {
	useTemporaryHubStore(t)
	legacy := `[{"id":"spoke-legacy","token_hash":"hash","created_at":"2025-01-01T00:00:00Z","last_seen":"2025-01-01T00:00:00Z","revoked":false,"profile":{"hostname":"old-host","label":"旧节点","project_name":"old-app","project_type":"java"}}]`
	if err := os.MkdirAll(filepath.Dir(spokesFile), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(spokesFile, []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	if err := LoadSpokes(); err != nil {
		t.Fatal(err)
	}
	record, ok := GetSpoke("spoke-legacy")
	if !ok || record.Profile == nil || record.Profile.ProjectName != "old-app" {
		t.Fatalf("旧版注册表加载异常: %#v", record)
	}
}

func useTemporaryHubStore(t *testing.T) {
	t.Helper()
	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	defaultStore = &spokeStore{
		spokes:        make(map[string]*SpokeRecord),
		pendingTokens: make(map[string]time.Time),
	}
	defaultControlStore = &controlJobStore{jobs: make(map[string]*ControlJob)}
	t.Cleanup(func() {
		if err := os.Chdir(oldDir); err != nil {
			t.Errorf("恢复测试目录失败: %v", err)
		}
	})
}
