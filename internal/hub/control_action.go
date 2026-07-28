package hub

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

const (
	// ControlActionShell 执行兼容的原始 Shell 命令。
	ControlActionShell = "shell"
	// ControlActionStatus 查询服务状态。
	ControlActionStatus = "status"
	// ControlActionLogs 查询服务日志。
	ControlActionLogs = "logs"
	// ControlActionRestart 重启服务。
	ControlActionRestart = "restart"
	// ControlActionDeploy 部署服务。
	ControlActionDeploy = "deploy"
	// ControlActionDatabaseQuery 执行本地数据库档案的只读查询。
	ControlActionDatabaseQuery = "database_query"
)

// ControlAction 描述由 Spoke 本地执行器解释的结构化动作。
type ControlAction struct {
	Type   string            `json:"type"`
	Params map[string]string `json:"params,omitempty"`
}

// Validate 校验结构化动作及其参数边界。
func (a ControlAction) Validate() error {
	actionType := strings.ToLower(strings.TrimSpace(a.Type))
	switch actionType {
	case ControlActionShell:
		if strings.TrimSpace(a.Params["command"]) == "" {
			return fmt.Errorf("shell 动作缺少 command")
		}
	case ControlActionStatus, ControlActionRestart, ControlActionDeploy:
		if err := validateControlServiceID(a.Params["service_id"]); err != nil {
			return err
		}
	case ControlActionLogs:
		if err := validateControlServiceID(a.Params["service_id"]); err != nil {
			return err
		}
		if lines := strings.TrimSpace(a.Params["lines"]); lines != "" {
			value, err := strconv.Atoi(lines)
			if err != nil || value < 1 || value > 1000 {
				return fmt.Errorf("日志行数必须在 1-1000 之间")
			}
		}
	case ControlActionDatabaseQuery:
		if strings.TrimSpace(a.Params["profile"]) == "" {
			return fmt.Errorf("database_query 动作缺少 profile")
		}
		if strings.TrimSpace(a.Params["sql"]) == "" {
			return fmt.Errorf("database_query 动作缺少 sql")
		}
	default:
		return fmt.Errorf("不支持的结构化动作: %s", a.Type)
	}
	return nil
}

// RequiredCapability 返回执行该动作所需的节点能力。
func (a ControlAction) RequiredCapability() string {
	switch strings.ToLower(strings.TrimSpace(a.Type)) {
	case ControlActionShell:
		return "shell"
	case ControlActionStatus:
		return "service.status"
	case ControlActionLogs:
		return "service.logs"
	case ControlActionRestart:
		return "service.restart"
	case ControlActionDeploy:
		return "service.deploy"
	case ControlActionDatabaseQuery:
		return "database.query.read"
	default:
		return ""
	}
}

// Summary 返回不包含数据库密码等凭证的动作摘要。
func (a ControlAction) Summary() string {
	actionType := strings.ToLower(strings.TrimSpace(a.Type))
	switch actionType {
	case ControlActionShell:
		return "shell: " + truncateControlText(strings.TrimSpace(a.Params["command"]), 200)
	case ControlActionDatabaseQuery:
		return fmt.Sprintf(
			"database_query profile=%s sql=%s",
			strings.TrimSpace(a.Params["profile"]),
			truncateControlText(strings.TrimSpace(a.Params["sql"]), 160),
		)
	default:
		keys := make([]string, 0, len(a.Params))
		for key := range a.Params {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		parts := []string{actionType}
		for _, key := range keys {
			parts = append(parts, key+"="+strings.TrimSpace(a.Params[key]))
		}
		return strings.Join(parts, " ")
	}
}

func validateControlServiceID(serviceID string) error {
	serviceID = strings.TrimSpace(serviceID)
	if serviceID == "" {
		return nil
	}
	if len(serviceID) > 64 {
		return fmt.Errorf("service_id 不能超过 64 字符")
	}
	for _, char := range serviceID {
		if (char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == '-' || char == '_' || char == '.' {
			continue
		}
		return fmt.Errorf("service_id 包含非法字符")
	}
	return nil
}

func cloneControlAction(action *ControlAction) *ControlAction {
	if action == nil {
		return nil
	}
	cloned := &ControlAction{Type: strings.ToLower(strings.TrimSpace(action.Type))}
	if action.Params != nil {
		cloned.Params = make(map[string]string, len(action.Params))
		for key, value := range action.Params {
			cloned.Params[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}
	return cloned
}

func equalControlActions(left, right *ControlAction) bool {
	if left == nil || right == nil {
		return left == right
	}
	if strings.ToLower(strings.TrimSpace(left.Type)) != strings.ToLower(strings.TrimSpace(right.Type)) ||
		len(left.Params) != len(right.Params) {
		return false
	}
	for key, value := range left.Params {
		if right.Params[key] != value {
			return false
		}
	}
	return true
}
