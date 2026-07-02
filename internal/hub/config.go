package hub

import (
	"encoding/json"
	"os"
)

const appConfigFile = "configs/app_config.json"

// HubSettings Hub 开关配置（存于 app_config.json 的 hub 字段）
type HubSettings struct {
	Enabled bool `json:"enabled"`
}

// LoadHubSettings 读取 Hub 开关
func LoadHubSettings() (HubSettings, error) {
	var settings HubSettings
	data, err := os.ReadFile(appConfigFile)
	if err != nil {
		return settings, nil
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil {
		return settings, err
	}
	raw, ok := root["hub"]
	if !ok {
		return settings, nil
	}
	if err := json.Unmarshal(raw, &settings); err != nil {
		return settings, err
	}
	return settings, nil
}

// SaveHubEnabled 更新 hub.enabled 并写回 app_config.json
func SaveHubEnabled(enabled bool) error {
	var root map[string]json.RawMessage
	if data, err := os.ReadFile(appConfigFile); err == nil {
		_ = json.Unmarshal(data, &root)
	}
	if root == nil {
		root = make(map[string]json.RawMessage)
	}
	settings := HubSettings{Enabled: enabled}
	raw, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	root["hub"] = raw
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll("configs", 0755); err != nil {
		return err
	}
	return os.WriteFile(appConfigFile, out, 0644)
}
