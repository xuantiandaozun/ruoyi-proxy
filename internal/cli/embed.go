package cli

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
)

var scriptsFS embed.FS
var configsFS embed.FS

// SetEmbedFS 设置嵌入的文件系统（从外部注入）
func SetEmbedFS(scripts, configs embed.FS) {
	scriptsFS = scripts
	configsFS = configs
}

// InitializeFiles 初始化脚本和配置文件
func (c *CLI) InitializeFiles() error {
	// 检查并创建scripts目录
	if err := c.ensureScripts(); err != nil {
		return err
	}

	// 检查并创建configs目录
	if err := c.ensureConfigs(); err != nil {
		return err
	}

	return nil
}

// ensureScripts 确保脚本文件存在（每次都覆盖以保持最新）
func (c *CLI) ensureScripts() error {
	scriptsDir := "scripts"

	// 创建scripts目录
	if err := os.MkdirAll(scriptsDir, 0755); err != nil {
		return fmt.Errorf("创建scripts目录失败: %v", err)
	}

	// 需要的脚本文件
	scripts := []string{
		"init.sh",
		"service.sh",
		"https.sh",
		"configure-nginx.sh",
	}

	updated := false
	for _, script := range scripts {
		scriptPath := filepath.Join(scriptsDir, script)

		// 从嵌入的文件系统中提取（每次都覆盖）
		data, err := scriptsFS.ReadFile(filepath.Join("scripts", script))
		if err != nil {
			c.printWarning(fmt.Sprintf("跳过 %s: %v", script, err))
			continue
		}

		// 检查文件是否存在
		_, statErr := os.Stat(scriptPath)
		isNew := os.IsNotExist(statErr)

		// 写入文件（覆盖）
		if err := os.WriteFile(scriptPath, data, 0755); err != nil {
			return fmt.Errorf("写入 %s 失败: %v", script, err)
		}

		if isNew {
			c.printSuccess(fmt.Sprintf("创建脚本: %s", script))
			updated = true
		}
	}

	if updated {
		fmt.Println()
	}

	return nil
}

// ensureConfigs 确保配置文件存在（不覆盖已有配置）
func (c *CLI) ensureConfigs() error {
	configsDir := "configs"

	// 创建configs目录
	if err := os.MkdirAll(configsDir, 0755); err != nil {
		return fmt.Errorf("创建configs目录失败: %v", err)
	}

	// 配置文件和模板
	configFiles := map[string]bool{
		"app_config.json":           false, // false = 不覆盖
		"nginx.conf.template":       true,  // true = 每次覆盖
		"nginx-https.conf.template": true,  // true = 每次覆盖
	}

	created := false
	for config, shouldOverwrite := range configFiles {
		configPath := filepath.Join(configsDir, config)

		// 检查文件是否存在
		_, statErr := os.Stat(configPath)
		exists := !os.IsNotExist(statErr)

		// 如果文件存在且不应该覆盖，跳过
		if exists && !shouldOverwrite {
			continue
		}

		// 从嵌入的文件系统中提取
		data, err := configsFS.ReadFile(filepath.Join("configs", config))
		if err != nil {
			c.printWarning(fmt.Sprintf("跳过 %s: %v", config, err))
			continue
		}

		if err := os.WriteFile(configPath, data, 0644); err != nil {
			return fmt.Errorf("写入 %s 失败: %v", config, err)
		}

		if !exists {
			c.printSuccess(fmt.Sprintf("创建配置: %s", config))
			created = true
		}
	}

	if created {
		fmt.Println()
	}

	return nil
}
