package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// prepare-embed 按 hub/spoke 角色生成嵌入用 app_config.json
func main() {
	profile := flag.String("profile", "default", "构建角色: default | hub | spoke")
	hubURL := flag.String("hub-url", "", "Spoke 构建时的 Hub 地址（留空则读取源配置 ai.base_url）")
	src := flag.String("src", "configs/app_config.json", "源配置文件")
	dst := flag.String("dst", "cmd/proxy/configs/app_config.json", "嵌入目标路径")
	flag.Parse()

	data, err := os.ReadFile(*src)
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取配置失败 %s: %v\n", *src, err)
		fmt.Fprintf(os.Stderr, "提示: 请先在 configs/app_config.json 配置 AI 密钥与 Hub 地址\n")
		os.Exit(1)
	}

	var root map[string]interface{}
	if err := json.Unmarshal(data, &root); err != nil {
		fmt.Fprintf(os.Stderr, "解析配置失败: %v\n", err)
		os.Exit(1)
	}

	switch strings.ToLower(*profile) {
	case "hub":
		if err := prepareHub(root); err != nil {
			fmt.Fprintf(os.Stderr, "Hub 配置准备失败: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✓ Hub 构建：已嵌入完整 AI 配置并启用 hub.enabled")
	case "spoke":
		if err := prepareSpoke(root, *hubURL); err != nil {
			fmt.Fprintf(os.Stderr, "Spoke 配置准备失败: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✓ Spoke 构建：已嵌入 Hub 地址，已移除 AI 密钥")
	default:
		fmt.Printf("✓ Default 构建：原样嵌入 %s\n", *src)
	}

	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "序列化失败: %v\n", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(filepath.Dir(*dst), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "创建目录失败: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*dst, out, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "写入失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("  → %s\n", *dst)
}

func prepareHub(root map[string]interface{}) error {
	ai, ok := root["ai"].(map[string]interface{})
	if !ok || ai == nil {
		return fmt.Errorf("Hub 构建需要在 %s 中配置 ai 段（含 provider/api_key/model）", "configs/app_config.json")
	}
	provider, _ := ai["provider"].(string)
	if provider == "hub" {
		return fmt.Errorf("Hub 节点请配置真实 AI 提供商（openai/anthropic/ollama），不要设为 hub")
	}
	if strings.TrimSpace(fmt.Sprint(ai["api_key"])) == "" && provider != "ollama" {
		return fmt.Errorf("Hub 构建需要 ai.api_key")
	}

	hubCfg, _ := root["hub"].(map[string]interface{})
	if hubCfg == nil {
		hubCfg = map[string]interface{}{}
	}
	hubCfg["enabled"] = true
	root["hub"] = hubCfg
	return nil
}

func prepareSpoke(root map[string]interface{}, hubURLFlag string) error {
	hubURL := strings.TrimSpace(hubURLFlag)
	if hubURL == "" {
		if ai, ok := root["ai"].(map[string]interface{}); ok {
			if u, ok := ai["base_url"].(string); ok {
				hubURL = strings.TrimSpace(u)
			}
		}
	}
	if hubURL == "" {
		return fmt.Errorf("Spoke 构建需要 Hub 地址：make linux-spoke HUB_URL=https://your-hub 或在 configs/app_config.json 的 ai.base_url 填写")
	}

	root["ai"] = map[string]interface{}{
		"provider":        "hub",
		"base_url":        strings.TrimRight(hubURL, "/"),
		"model":           "hub-relay",
		"api_key":         "",
		"max_tokens":      4096,
		"context_limit":   24000,
		"timeout_seconds": 120,
	}
	root["hub"] = map[string]interface{}{"enabled": false}
	return nil
}
