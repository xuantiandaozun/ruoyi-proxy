package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// AppConfig 应用配置结构
type AppConfig struct {
	Proxy ProxyConfig `json:"proxy"`
	SSL   SSLConfig   `json:"ssl"`
}

// ProxyConfig 代理配置
type ProxyConfig struct {
	BlueTarget  string `json:"blue_target"`
	GreenTarget string `json:"green_target"`
	ActiveEnv   string `json:"active_env"`
}

// SSLConfig SSL配置
type SSLConfig struct {
	Email       string `json:"email"`
	CertPath    string `json:"cert_path"`
	WebrootPath string `json:"webroot_path"`
	LogFile     string `json:"log_file"`
}

// ShowConfig 显示完整配置
func (c *CLI) ShowConfig() {
	configFile := "configs/app_config.json"

	data, err := os.ReadFile(configFile)
	if err != nil {
		c.printError("配置文件不存在，请先运行 init 命令初始化")
		return
	}

	var config AppConfig
	if err := json.Unmarshal(data, &config); err != nil {
		c.printError("配置文件格式错误")
		return
	}

	fmt.Println("\n\033[1;34m═══ 应用配置 ═══\033[0m\n")

	// 代理配置
	fmt.Println("\033[1;33m代理配置:\033[0m")
	fmt.Printf("  蓝色环境: %s\n", config.Proxy.BlueTarget)
	fmt.Printf("  绿色环境: %s\n", config.Proxy.GreenTarget)
	fmt.Printf("  活跃环境: \033[1;36m%s\033[0m\n", config.Proxy.ActiveEnv)

	// SSL配置
	fmt.Println("\n\033[1;33mSSL证书:\033[0m")
	fmt.Printf("  邮箱: %s\n", config.SSL.Email)
	fmt.Printf("  证书目录: %s\n", config.SSL.CertPath)
	fmt.Printf("  网站根目录: %s\n", config.SSL.WebrootPath)

	fmt.Println("\n\033[1;36m配置文件: configs/app_config.json\033[0m")
	fmt.Println()
}

// EditConfig 编辑配置
func (c *CLI) EditConfig() {
	fmt.Println("\n\033[1;34m═══ 编辑配置 ═══\033[0m\n")

	fmt.Println("选择要编辑的配置:")
	fmt.Println("  1. SSL邮箱")
	fmt.Println("  2. 查看完整配置")
	fmt.Print("\n\033[1;33m请选择 (1-2): \033[0m")

	choice, err := c.readLine()
	if err != nil {
		return
	}

	choice = strings.TrimSpace(choice)

	switch choice {
	case "1":
		c.editSSLEmail()
	case "2":
		c.ShowConfig()
	default:
		c.printError("无效选择")
	}
}

// editSSLEmail 编辑SSL邮箱
func (c *CLI) editSSLEmail() {
	configFile := "configs/app_config.json"

	data, err := os.ReadFile(configFile)
	if err != nil {
		c.printError("配置文件不存在")
		return
	}

	var config AppConfig
	if err := json.Unmarshal(data, &config); err != nil {
		c.printError("配置文件格式错误")
		return
	}

	fmt.Printf("\n当前邮箱: \033[1;36m%s\033[0m\n", config.SSL.Email)
	fmt.Print("\033[1;33m新邮箱地址: \033[0m")

	newEmail, err := c.readLine()
	if err != nil {
		return
	}

	newEmail = strings.TrimSpace(newEmail)
	if newEmail == "" {
		c.printWarning("已取消")
		return
	}

	config.SSL.Email = newEmail

	// 保存配置
	data, err = json.MarshalIndent(config, "", "  ")
	if err != nil {
		c.printError("保存失败")
		return
	}

	if err := os.WriteFile(configFile, data, 0644); err != nil {
		c.printError("写入文件失败")
		return
	}

	c.printSuccess("SSL邮箱已更新")
}
