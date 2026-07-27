package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"ruoyi-proxy/internal/database"
)

// discoverDatabases 扫描项目配置并保存发现的数据库档案。
func (c *CLI) discoverDatabases(projectPath string) {
	if strings.TrimSpace(projectPath) == "" {
		projectPath = "."
	}
	profiles, err := database.Discover(projectPath)
	if err != nil {
		c.printError(err.Error())
		return
	}
	if len(profiles) == 0 {
		fmt.Println("未发现 MySQL 配置；支持 application*.yml/properties、.env、JSON 和 TOML")
		return
	}
	for _, profile := range profiles {
		if err := database.SaveProfile(profile); err != nil {
			c.printError(err.Error())
			return
		}
		fmt.Printf("  %-12s %-28s %s:%d/%s  来源: %s\n", profile.ID, profile.Name, profile.Host, profile.Port, profile.Database, profile.SourcePath)
	}
	c.printSuccess(fmt.Sprintf("已发现并保存 %d 个数据库连接（密码未写入档案）", len(profiles)))
}

// addDatabaseInteractive 通过向导保存任意项目的远程 MySQL 连接。
func (c *CLI) addDatabaseInteractive() {
	fmt.Println("\033[1;34m═══ 添加远程 MySQL 项目连接 ═══\033[0m")
	projectName, err := c.readLineWithPrompt("项目名称（必填，可与当前部署项目无关）: ")
	if err != nil || projectName == "" {
		c.printError("项目名称不能为空")
		return
	}
	name, _ := c.readLineWithPrompt("连接名称（如 订单系统生产库，可留空）: ")
	environment, _ := c.readLineWithPrompt("环境（prod/test/dev，可留空）: ")
	host, err := c.readLineWithPrompt("远程 MySQL 地址或域名: ")
	if err != nil || host == "" {
		c.printError("MySQL 地址不能为空")
		return
	}
	portText, _ := c.readLineWithPrompt("端口（默认 3306）: ")
	port := 3306
	if portText != "" {
		parsed, parseErr := strconv.Atoi(portText)
		if parseErr != nil {
			c.printError("端口格式错误")
			return
		}
		port = parsed
	}
	dbName, err := c.readLineWithPrompt("数据库名: ")
	if err != nil || dbName == "" {
		c.printError("数据库名不能为空")
		return
	}
	username, err := c.readLineWithPrompt("用户名: ")
	if err != nil || username == "" {
		c.printError("用户名不能为空")
		return
	}
	passwordBytes, err := c.rl.ReadPassword("密码（输入内容不会显示）: ")
	if err != nil {
		c.printError("读取密码失败")
		return
	}
	remark, _ := c.readLineWithPrompt("备注（服务器、用途等，可留空）: ")
	profile, err := database.SaveConnection(database.ConnectionInput{Name: name, ProjectName: projectName, Environment: environment, Host: host, Port: port, Database: dbName, Username: username, Password: string(passwordBytes), Remark: remark})
	for i := range passwordBytes {
		passwordBytes[i] = 0
	}
	if err != nil {
		c.printError(err.Error())
		return
	}
	c.printSuccess(fmt.Sprintf("已保存连接 %s，ID: %s", profile.Name, profile.ID))
	c.printInfo("密码已写入独立本机密钥文件，不会在列表或 AI 结果中显示")
}

// listDatabases 显示已保存的数据库档案。
func (c *CLI) listDatabases() {
	profiles, err := database.LoadProfiles()
	if err != nil {
		c.printError(err.Error())
		return
	}
	if len(profiles) == 0 {
		fmt.Println("暂无数据库档案，请执行 /db-add，或直接告诉 AI 项目名和远程 MySQL 连接信息")
		return
	}
	fmt.Println("ID           项目             环境      连接名称                     地址 / 数据库                 凭据")
	for _, profile := range profiles {
		credential := "外部"
		if profile.HasPassword {
			credential = "已保存"
		}
		fmt.Printf("%-12s %-16s %-9s %-28s %s:%d/%s (%s)  %s\n", profile.ID, profile.ProjectName, profile.Environment, profile.Name, profile.Host, profile.Port, profile.Database, profile.Username, credential)
	}
}

func (c *CLI) testDatabase(ref string) {
	profile, err := database.GetProfile(ref)
	if err != nil {
		c.printError(err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := database.Test(ctx, profile); err != nil {
		c.printError(err.Error())
		return
	}
	c.printSuccess("数据库连接成功: " + profile.Name)
}

func (c *CLI) showDatabaseSchema(ref string) {
	profile, err := database.GetProfile(ref)
	if err != nil {
		c.printError(err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := database.Schema(ctx, profile)
	if err != nil {
		c.printError(err.Error())
		return
	}
	printDatabaseResult(result)
}

func (c *CLI) queryDatabase(ref, statement string) {
	profile, err := database.GetProfile(ref)
	if err != nil {
		c.printError(err.Error())
		return
	}
	if !database.IsReadOnlySQL(statement) && !c.confirmDangerAction("执行数据库写操作", []string{"数据库: " + profile.Name, "SQL: " + statement}) {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	result, err := database.Execute(ctx, profile, statement)
	if err != nil {
		c.printError(err.Error())
		return
	}
	printDatabaseResult(result)
}

func printDatabaseResult(result database.QueryResult) {
	if len(result.Columns) == 0 {
		fmt.Printf("执行成功，影响行数: %d", result.RowsAffected)
		if result.LastInsertID > 0 {
			fmt.Printf("，新增 ID: %d", result.LastInsertID)
		}
		fmt.Println()
		return
	}
	raw, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fmt.Printf("结果格式化失败: %v\n", err)
		return
	}
	fmt.Println(string(raw))
}
