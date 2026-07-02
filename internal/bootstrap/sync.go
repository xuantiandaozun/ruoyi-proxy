package bootstrap

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"ruoyi-proxy/internal/agent"
	"ruoyi-proxy/internal/hub"
)

// SyncProfileToHub 将 Spoke 档案同步到 Hub
func SyncProfileToHub(profile hub.SpokeProfile) error {
	aiCfg, err := agent.LoadAIConfig()
	if err != nil || aiCfg.Provider != "hub" || !aiCfg.IsConfigured() {
		return fmt.Errorf("未配置 Hub 连接")
	}
	profile.UpdatedAt = time.Now()
	body, err := json.Marshal(profile)
	if err != nil {
		return err
	}
	url := strings.TrimRight(aiCfg.BaseURL, "/") + "/__hub__/v1/profile"
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+aiCfg.APIKey)
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("同步请求失败: %v", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		return fmt.Errorf("同步失败 HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return nil
}
