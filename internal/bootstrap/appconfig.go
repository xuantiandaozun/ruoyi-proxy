package bootstrap

import (
	"encoding/json"
	"os"
)

const appConfigFile = "configs/app_config.json"
const spokeProfileFile = "configs/spoke_profile.json"

// AppPaths 从 app_config.json 读取的路径信息
type AppPaths struct {
	Domain     string
	NginxConf  string
	HTMLPath   string
	EnableHTTPS bool
}

// LoadAppPaths 读取域名与 Nginx 配置路径
func LoadAppPaths() AppPaths {
	var paths AppPaths
	paths.NginxConf = "/etc/nginx/conf.d/ruoyi.conf"

	data, err := os.ReadFile(appConfigFile)
	if err != nil {
		return paths
	}
	var root map[string]interface{}
	if err := json.Unmarshal(data, &root); err != nil {
		return paths
	}
	if v, ok := root["domain"].(string); ok {
		paths.Domain = v
	}
	if v, ok := root["enable_https"].(bool); ok {
		paths.EnableHTTPS = v
	}
	if nginx, ok := root["nginx"].(map[string]interface{}); ok {
		if v, ok := nginx["config_path"].(string); ok && v != "" {
			paths.NginxConf = v
		}
		if v, ok := nginx["html_path"].(string); ok {
			paths.HTMLPath = v
		}
	}
	return paths
}

// SaveSpokeProfile 保存 Spoke 本地档案
func SaveSpokeProfile(profile []byte) error {
	if err := os.MkdirAll("configs", 0755); err != nil {
		return err
	}
	return os.WriteFile(spokeProfileFile, profile, 0644)
}

// LoadSpokeProfileRaw 读取本地 Spoke 档案原始 JSON
func LoadSpokeProfileRaw() ([]byte, error) {
	return os.ReadFile(spokeProfileFile)
}
