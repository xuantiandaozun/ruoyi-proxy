package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"ruoyi-proxy/internal/config"
)

var hubMgmtBaseURL = localHubMgmtBaseURL()
var hubMgmtHTTPClient = &http.Client{Timeout: 8 * time.Second}

type hubRemoteAction struct {
	Type   string            `json:"type"`
	Params map[string]string `json:"params,omitempty"`
}

type hubRemoteJob struct {
	ID           string           `json:"id"`
	BatchID      string           `json:"batch_id,omitempty"`
	SpokeID      string           `json:"spoke_id"`
	Command      string           `json:"command"`
	Action       *hubRemoteAction `json:"action,omitempty"`
	WorkDir      string           `json:"workdir,omitempty"`
	TimeoutSecs  int              `json:"timeout_seconds"`
	Status       string           `json:"status"`
	Output       string           `json:"output,omitempty"`
	Error        string           `json:"error,omitempty"`
	Attempt      int              `json:"attempt,omitempty"`
	MaxAttempts  int              `json:"max_attempts,omitempty"`
	CreatedAt    time.Time        `json:"created_at"`
	ClaimedUntil time.Time        `json:"claimed_until,omitempty"`
	StartedAt    time.Time        `json:"started_at,omitempty"`
	FinishedAt   time.Time        `json:"finished_at,omitempty"`
}

func init() {
	AllTools = append(AllTools,
		ToolDef{
			Name:        "hub_spokes",
			Description: "在 Hub 本机查询已注册的 Spoke 节点或指定节点详情。用于按项目名、备注、主机名找到 spoke_id；这是档案查询，不代表远程业务服务实时健康",
			ReadOnly:    true,
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"action":   map[string]interface{}{"type": "string", "enum": []string{"list", "detail"}, "description": "list 列出节点，detail 查看单节点"},
					"spoke_id": map[string]interface{}{"type": "string", "description": "detail 时必填"},
				},
				"required": []string{"action"},
			},
		},
		ToolDef{
			Name:        "hub_remote_command",
			Description: "通过 Hub 任务队列让指定 Spoke 执行远程 Shell 命令，并等待返回结果。适合远程查进程、端口、systemd、Docker、日志和服务健康；远程执行始终需要用户确认",
			ReadOnly:    false,
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"spoke_id":        map[string]interface{}{"type": "string", "description": "目标 Spoke ID"},
					"command":         map[string]interface{}{"type": "string", "description": "在目标 Spoke 执行的 Shell 命令"},
					"workdir":         map[string]interface{}{"type": "string", "description": "目标 Spoke 上的工作目录，可留空"},
					"timeout_seconds": map[string]interface{}{"type": "integer", "description": "远程命令超时，默认 60，最大 300"},
					"wait_seconds":    map[string]interface{}{"type": "integer", "description": "等待回传结果的秒数，默认 30，最大 120"},
				},
				"required": []string{"spoke_id", "command"},
			},
		},
		ToolDef{
			Name:        "hub_remote_jobs",
			Description: "查询 Hub 远程任务状态、输出和错误。可按 spoke_id 或 job_id 筛选",
			ReadOnly:    true,
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"spoke_id": map[string]interface{}{"type": "string"},
					"job_id":   map[string]interface{}{"type": "string"},
				},
				"required": []string{},
			},
		},
	)
}

func (e *ToolExecutor) hubSpokes(action, spokeID string) (string, error) {
	endpoint := hubMgmtBaseURL + "/hub/status"
	if action == "detail" {
		if strings.TrimSpace(spokeID) == "" {
			return "", fmt.Errorf("detail 操作必须提供 spoke_id")
		}
		endpoint = hubMgmtBaseURL + "/hub/spoke?spoke=" + url.QueryEscape(strings.TrimSpace(spokeID))
	}
	body, err := hubMgmtRequest(http.MethodGet, endpoint, nil, http.StatusOK)
	if err != nil {
		return "", err
	}
	return prettyJSON(body), nil
}

func (e *ToolExecutor) hubRemoteCommand(spokeID, command, workDir string, timeoutSecs, waitSecs int) (string, error) {
	spokeID = strings.TrimSpace(spokeID)
	command = strings.TrimSpace(command)
	if spokeID == "" || command == "" {
		return "", fmt.Errorf("spoke_id 和 command 不能为空")
	}
	if timeoutSecs <= 0 {
		timeoutSecs = 60
	}
	if timeoutSecs > 300 {
		timeoutSecs = 300
	}
	if waitSecs <= 0 {
		waitSecs = 30
	}
	if waitSecs > 120 {
		waitSecs = 120
	}
	payload, err := json.Marshal(map[string]interface{}{
		"spoke_id": spokeID, "command": command,
		"workdir": strings.TrimSpace(workDir), "timeout_seconds": timeoutSecs,
		"actor": "local-user", "source": "agent",
		"confirmed_by": "local-user", "confirmed_at": time.Now(),
	})
	if err != nil {
		return "", fmt.Errorf("编码远程任务失败: %v", err)
	}
	body, err := hubMgmtRequest(
		http.MethodPost, hubMgmtBaseURL+"/hub/control", payload, http.StatusAccepted,
	)
	if err != nil {
		return "", err
	}
	var job hubRemoteJob
	if err := json.Unmarshal(body, &job); err != nil {
		return "", fmt.Errorf("解析远程任务失败: %v", err)
	}
	deadline := time.Now().Add(time.Duration(waitSecs) * time.Second)
	for time.Now().Before(deadline) {
		current, found, err := fetchHubRemoteJob(spokeID, job.ID)
		if err != nil {
			return "", err
		}
		if found && isHubRemoteJobFinished(current.Status) {
			return formatHubRemoteJob(current), nil
		}
		time.Sleep(time.Second)
	}
	return fmt.Sprintf("远程任务已提交但仍在等待执行: %s\n目标: %s\n可调用 hub_remote_jobs 查询后续结果", job.ID, spokeID), nil
}

func (e *ToolExecutor) hubRemoteJobs(spokeID, jobID string) (string, error) {
	jobs, err := fetchHubRemoteJobs(spokeID)
	if err != nil {
		return "", err
	}
	var results []string
	for _, job := range jobs {
		if strings.TrimSpace(jobID) == "" || job.ID == strings.TrimSpace(jobID) {
			results = append(results, formatHubRemoteJob(job))
		}
	}
	if len(results) == 0 {
		return "未找到匹配的远程任务", nil
	}
	return strings.Join(results, "\n\n"), nil
}

func fetchHubRemoteJob(spokeID, jobID string) (hubRemoteJob, bool, error) {
	jobs, err := fetchHubRemoteJobs(spokeID)
	if err != nil {
		return hubRemoteJob{}, false, err
	}
	for _, job := range jobs {
		if job.ID == jobID {
			return job, true, nil
		}
	}
	return hubRemoteJob{}, false, nil
}

func fetchHubRemoteJobs(spokeID string) ([]hubRemoteJob, error) {
	endpoint := hubMgmtBaseURL + "/hub/jobs"
	if strings.TrimSpace(spokeID) != "" {
		endpoint += "?spoke=" + url.QueryEscape(strings.TrimSpace(spokeID))
	}
	body, err := hubMgmtRequest(http.MethodGet, endpoint, nil, http.StatusOK)
	if err != nil {
		return nil, err
	}
	var response struct {
		Jobs []hubRemoteJob `json:"jobs"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("解析远程任务列表失败: %v", err)
	}
	return response.Jobs, nil
}

func hubMgmtRequest(method, endpoint string, payload []byte, expectedStatus int) ([]byte, error) {
	var body io.Reader
	if payload != nil {
		body = bytes.NewReader(payload)
	}
	req, err := http.NewRequest(method, endpoint, body)
	if err != nil {
		return nil, fmt.Errorf("创建 Hub 管理请求失败: %v", err)
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := hubMgmtHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("连接 Hub 本机管理接口失败: %v（请确认 Hub 代理进程已启动）", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("读取 Hub 管理响应失败: %v", err)
	}
	if resp.StatusCode != expectedStatus {
		return nil, fmt.Errorf("Hub 管理接口 HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return data, nil
}

func prettyJSON(data []byte) string {
	var value interface{}
	if json.Unmarshal(data, &value) != nil {
		return strings.TrimSpace(string(data))
	}
	formatted, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return strings.TrimSpace(string(data))
	}
	return string(formatted)
}

func formatHubRemoteJob(job hubRemoteJob) string {
	var lines []string
	lines = append(lines,
		fmt.Sprintf("任务: %s", job.ID),
		fmt.Sprintf("Spoke: %s", job.SpokeID),
		fmt.Sprintf("状态: %s", job.Status),
	)
	if job.BatchID != "" {
		lines = append(lines, "批次: "+job.BatchID)
	}
	if job.Action != nil {
		actionJSON, _ := json.Marshal(job.Action)
		lines = append(lines, "动作: "+string(actionJSON))
	} else {
		lines = append(lines, fmt.Sprintf("命令: %s", job.Command))
	}
	if job.MaxAttempts > 0 {
		lines = append(lines, fmt.Sprintf("尝试: %d/%d", job.Attempt, job.MaxAttempts))
	}
	if strings.TrimSpace(job.Output) != "" {
		lines = append(lines, "输出:\n"+strings.TrimSpace(job.Output))
	}
	if strings.TrimSpace(job.Error) != "" {
		lines = append(lines, "错误: "+strings.TrimSpace(job.Error))
	}
	return strings.Join(lines, "\n")
}

func isHubRemoteJobFinished(status string) bool {
	return status == "succeeded" || status == "failed" || status == "timed_out" || status == "canceled"
}

func localHubMgmtBaseURL() string {
	port := config.MgmtPort
	if strings.HasPrefix(port, ":") {
		return "http://127.0.0.1" + port
	}
	return "http://127.0.0.1:" + port
}
