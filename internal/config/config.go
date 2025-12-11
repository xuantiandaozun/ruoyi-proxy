package config

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

// ServiceConfig 单个服务配置
type ServiceConfig struct {
	Name        string `json:"name"`         // 服务名称（显示用）
	BlueTarget  string `json:"blue_target"`  // 蓝色环境地址
	GreenTarget string `json:"green_target"` // 绿色环境地址
	ActiveEnv   string `json:"active_env"`   // 当前活跃环境 blue/green
	JarFile     string `json:"jar_file"`     // JAR文件名（用于启动服务）
	AppName     string `json:"app_name"`     // 应用名称（用于区分PID文件等）
}

// Config 代理配置结构（支持多服务）
type Config struct {
	Services map[string]*ServiceConfig `json:"services"` // 服务配置，key为服务ID
}

// 常量配置
const (
	ConfigFile = "configs/proxy_config.json"
	ProxyPort  = ":8000" // 代理监听端口
	MgmtPort   = ":8001" // 管理接口端口
)

// LoadConfig 加载代理配置文件
func LoadConfig() (*Config, error) {
	// 默认配置（单服务）
	config := &Config{
		Services: map[string]*ServiceConfig{
			"default": {
				Name:        "默认服务",
				BlueTarget:  "http://127.0.0.1:8080",
				GreenTarget: "http://127.0.0.1:8081",
				ActiveEnv:   "blue",
				JarFile:     "ruoyi-[0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9]-*.jar", // 精确匹配 ruoyi-YYYYMMDD-HHMMSS.jar (8位日期),不匹配 ruoyi-服务名-时间戳.jar
				AppName:     "ruoyi",
			},
		},
	}

	// 确保配置目录存在
	if err := os.MkdirAll(filepath.Dir(ConfigFile), 0755); err != nil {
		return nil, fmt.Errorf("创建配置目录失败: %v", err)
	}

	// 尝试从文件加载
	if data, err := os.ReadFile(ConfigFile); err == nil {
		if err := json.Unmarshal(data, config); err != nil {
			log.Printf("解析配置文件失败: %v, 使用默认配置", err)
		} else {
			log.Printf("配置文件加载成功: %s, 服务数量: %d", ConfigFile, len(config.Services))
		}
	} else {
		log.Printf("配置文件不存在，创建默认配置: %s", ConfigFile)
		if err := SaveConfig(config); err != nil {
			return nil, err
		}
	}

	return config, nil
}

// SaveConfig 保存代理配置文件
func SaveConfig(config *Config) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化配置失败: %v", err)
	}

	if err := os.WriteFile(ConfigFile, data, 0644); err != nil {
		return fmt.Errorf("写入配置文件失败: %v", err)
	}

	log.Printf("配置文件已保存: %s", ConfigFile)
	return nil
}

// GetService 获取指定服务配置
func (c *Config) GetService(serviceID string) *ServiceConfig {
	if svc, ok := c.Services[serviceID]; ok {
		return svc
	}
	return nil
}

// GetServiceIDs 获取所有服务ID
func (c *Config) GetServiceIDs() []string {
	ids := make([]string, 0, len(c.Services))
	for id := range c.Services {
		ids = append(ids, id)
	}
	return ids
}
