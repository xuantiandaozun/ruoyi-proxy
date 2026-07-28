package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"ruoyi-proxy/internal/hub"
)

func (c *CLI) handleHubSelect(args []string) {
	if len(args) == 0 {
		if c.selectedSpoke == "" {
			c.printInfo("当前未选择 Spoke")
		} else {
			c.printInfo("当前 Spoke: " + c.selectedSpoke)
		}
		return
	}
	spokeID := strings.TrimSpace(args[0])
	if spokeID == "clear" || spokeID == "none" || spokeID == "-" {
		c.selectedSpoke = ""
		c.setMainPrompt()
		c.printSuccess("已清除当前 Spoke")
		return
	}
	endpoint := mgmtBaseURL() + "/hub/spoke?spoke=" + url.QueryEscape(spokeID)
	resp, err := http.Get(endpoint)
	if err != nil {
		c.printError(fmt.Sprintf("查询 Spoke 失败: %v", err))
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 128<<10))
	if resp.StatusCode != http.StatusOK {
		c.printError(fmt.Sprintf("选择 Spoke 失败 HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body))))
		return
	}
	var record hub.SpokeRecord
	if err := json.Unmarshal(body, &record); err != nil {
		c.printError(fmt.Sprintf("解析 Spoke 失败: %v", err))
		return
	}
	if record.Revoked {
		c.printError("不能选择已吊销节点: " + record.ID)
		return
	}
	c.selectedSpoke = record.ID
	c.setMainPrompt()
	label := record.Alias
	if label == "" && record.Profile != nil {
		label = record.Profile.Label
	}
	c.printSuccess(fmt.Sprintf("当前 Spoke: %s (%s)", record.ID, label))
}
func (c *CLI) handleHubNodeSet(args []string) {
	if len(args) < 2 {
		c.printError("用法: /hub-node-set <spoke-id> alias=... group=... env=... owner=... tags=a,b maintenance=true capabilities=a,b")
		return
	}
	spokeID := strings.TrimSpace(args[0])
	patch := make(map[string]interface{})
	for _, argument := range args[1:] {
		key, value, ok := strings.Cut(argument, "=")
		if !ok {
			c.printError("参数必须使用 key=value 格式: " + argument)
			return
		}
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "alias":
			patch["alias"] = strings.TrimSpace(value)
		case "group":
			patch["group"] = strings.TrimSpace(value)
		case "env", "environment":
			patch["environment"] = strings.TrimSpace(value)
		case "owner":
			patch["owner"] = strings.TrimSpace(value)
		case "tags":
			patch["tags"] = splitCommaValues(value)
		case "capabilities", "allowed_capabilities":
			patch["allowed_capabilities"] = splitCommaValues(value)
		case "maintenance":
			enabled, err := strconv.ParseBool(strings.TrimSpace(value))
			if err != nil {
				c.printError("maintenance 必须是 true 或 false")
				return
			}
			patch["maintenance"] = enabled
		default:
			c.printError("不支持的治理字段: " + key)
			return
		}
	}
	if !c.confirmDangerAction("更新 Hub 节点治理配置", []string{
		"节点: " + spokeID,
		"字段: " + strings.Join(args[1:], " "),
	}) {
		return
	}
	body, err := json.Marshal(patch)
	if err != nil {
		c.printError(fmt.Sprintf("编码节点配置失败: %v", err))
		return
	}
	endpoint := mgmtBaseURL() + "/hub/spoke/governance?spoke=" + url.QueryEscape(spokeID)
	resp, err := http.Post(endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		c.printError(fmt.Sprintf("更新节点配置失败: %v", err))
		return
	}
	defer resp.Body.Close()
	responseBody, _ := io.ReadAll(io.LimitReader(resp.Body, 128<<10))
	if resp.StatusCode != http.StatusOK {
		c.printError(fmt.Sprintf("更新节点配置失败 HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(responseBody))))
		return
	}
	var record hub.SpokeRecord
	if err := json.Unmarshal(responseBody, &record); err != nil {
		c.printError(fmt.Sprintf("解析节点配置失败: %v", err))
		return
	}
	c.printSuccess("节点治理配置已更新: " + record.ID)
}

func splitCommaValues(value string) []string {
	var result []string
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			result = append(result, item)
		}
	}
	return result
}

func formatByteSize(value uint64) string {
	if value == 0 {
		return "未知"
	}
	const unit = uint64(1024)
	if value < unit {
		return fmt.Sprintf("%d B", value)
	}
	divisor := unit
	exponent := 0
	for amount := value / unit; amount >= unit && exponent < 4; amount /= unit {
		divisor *= unit
		exponent++
	}
	return fmt.Sprintf("%.1f %ciB", float64(value)/float64(divisor), "KMGTPE"[exponent])
}
