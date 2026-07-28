package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"ruoyi-proxy/internal/hub"
)

func (c *CLI) handleHubAction(args []string) {
	if c.selectedSpoke != "" && len(args) > 0 && isHubActionType(args[0]) {
		args = append([]string{c.selectedSpoke}, args...)
	}
	if len(args) < 2 {
		c.printError("用法: /hub-action <spoke-id[,spoke-id]|@分组> <status|logs|restart|deploy|database_query> [参数]")
		return
	}
	group := ""
	if strings.HasPrefix(strings.TrimSpace(args[0]), "@") {
		group = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(args[0]), "@"))
	}
	targets := splitHubTargets(args[0])
	if group != "" {
		targets = nil
	}
	if len(targets) == 0 && group == "" {
		c.printError("至少需要一个 Spoke ID，或使用 @分组名")
		return
	}
	action := hub.ControlAction{Type: strings.ToLower(strings.TrimSpace(args[1])), Params: map[string]string{}}
	switch action.Type {
	case hub.ControlActionStatus, hub.ControlActionRestart, hub.ControlActionDeploy:
		if len(args) > 2 {
			action.Params["service_id"] = args[2]
		}
	case hub.ControlActionLogs:
		if len(args) > 2 {
			action.Params["service_id"] = args[2]
		}
		if len(args) > 3 {
			action.Params["lines"] = args[3]
		}
	case hub.ControlActionDatabaseQuery:
		if len(args) < 4 {
			c.printError("用法: /hub-action <目标|@分组> database_query <数据库档案> <只读SQL>")
			return
		}
		action.Params["profile"] = args[2]
		action.Params["sql"] = strings.Join(args[3:], " ")
	default:
		c.printError("不支持的动作，可选: status、logs、restart、deploy、database_query")
		return
	}
	if err := action.Validate(); err != nil {
		c.printError(err.Error())
		return
	}

	confirmedAt := time.Time{}
	confirmedBy := ""
	if action.Type == hub.ControlActionRestart ||
		action.Type == hub.ControlActionDeploy ||
		action.Type == hub.ControlActionDatabaseQuery {
		if !c.confirmDangerAction("执行远程结构化动作", []string{
			"目标: " + formatHubActionTargets(targets, group),
			"动作: " + action.Summary(),
		}) {
			return
		}
		confirmedAt = time.Now()
		confirmedBy = "local-user"
	}
	payload := map[string]interface{}{
		"action":       action,
		"source":       "cli",
		"actor":        "local-user",
		"confirmed_by": confirmedBy,
		"confirmed_at": confirmedAt,
	}
	if group != "" {
		payload["group"] = group
	} else if len(targets) == 1 {
		payload["spoke_id"] = targets[0]
	} else {
		payload["spoke_ids"] = targets
	}
	c.submitHubAction(payload, group != "" || len(targets) > 1)
}

func (c *CLI) submitHubAction(payload map[string]interface{}, batch bool) {
	body, err := json.Marshal(payload)
	if err != nil {
		c.printError(fmt.Sprintf("编码结构化任务失败: %v", err))
		return
	}
	resp, err := http.Post(mgmtBaseURL()+"/hub/control", "application/json", bytes.NewReader(body))
	if err != nil {
		c.printError(fmt.Sprintf("提交结构化任务失败: %v", err))
		return
	}
	defer resp.Body.Close()
	responseBody, _ := io.ReadAll(io.LimitReader(resp.Body, 256<<10))
	if resp.StatusCode != http.StatusAccepted {
		c.printError(fmt.Sprintf("提交结构化任务失败 HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(responseBody))))
		return
	}
	if !batch {
		var job hub.ControlJob
		if err := json.Unmarshal(responseBody, &job); err != nil {
			c.printError(fmt.Sprintf("解析任务响应失败: %v", err))
			return
		}
		c.printSuccess(fmt.Sprintf("结构化任务已提交: %s", job.ID))
		return
	}
	var created hub.ControlJobBatch
	if err := json.Unmarshal(responseBody, &created); err != nil {
		c.printError(fmt.Sprintf("解析批量任务响应失败: %v", err))
		return
	}
	c.printSuccess(fmt.Sprintf("批次 %s 已创建，共 %d 个独立任务", created.ID, len(created.Jobs)))
	for _, job := range created.Jobs {
		c.printInfo(fmt.Sprintf("%s → %s", job.SpokeID, job.ID))
	}
}

func isHubActionType(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case hub.ControlActionStatus,
		hub.ControlActionLogs,
		hub.ControlActionRestart,
		hub.ControlActionDeploy,
		hub.ControlActionDatabaseQuery:
		return true
	default:
		return false
	}
}
func formatHubActionTargets(targets []string, group string) string {
	if group != "" {
		return "节点组 @" + group
	}
	return strings.Join(targets, ", ")
}
func splitHubTargets(value string) []string {
	seen := map[string]bool{}
	var targets []string
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(item)
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		targets = append(targets, item)
	}
	return targets
}
