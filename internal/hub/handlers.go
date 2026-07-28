package hub

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"ruoyi-proxy/internal/agent"
)

type registerRequest struct {
	Token string `json:"token"`
}

type registerResponse struct {
	SpokeID string `json:"spoke_id"`
	Token   string `json:"token"`
}

type chatRequest struct {
	Messages []agent.Message `json:"messages"`
	Tools    []agent.ToolDef `json:"tools,omitempty"`
}

type chatResponse struct {
	Content          string           `json:"content"`
	ReasoningContent string           `json:"reasoning_content,omitempty"`
	ToolCalls        []agent.ToolCall `json:"tool_calls,omitempty"`
	Error            string           `json:"error,omitempty"`
}

type controlResultRequest struct {
	JobID  string `json:"job_id"`
	Status string `json:"status"`
	Output string `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
}

type controlEnqueueRequest struct {
	SpokeID        string         `json:"spoke_id"`
	SpokeIDs       []string       `json:"spoke_ids,omitempty"`
	Group          string         `json:"group,omitempty"`
	Command        string         `json:"command"`
	Action         *ControlAction `json:"action,omitempty"`
	WorkDir        string         `json:"workdir,omitempty"`
	TimeoutSecs    int            `json:"timeout_seconds,omitempty"`
	IdempotencyKey string         `json:"idempotency_key,omitempty"`
	MaxAttempts    int            `json:"max_attempts,omitempty"`
	Actor          string         `json:"actor,omitempty"`
	Source         string         `json:"source,omitempty"`
	ConfirmedBy    string         `json:"confirmed_by,omitempty"`
	ConfirmedAt    time.Time      `json:"confirmed_at,omitempty"`
}

type controlMutationRequest struct {
	JobID string `json:"job_id"`
}

// RegisterHandler POST /__hub__/v1/register
func RegisterHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "只允许 POST", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "读取请求失败", http.StatusBadRequest)
		return
	}
	var req registerRequest
	if err := json.Unmarshal(body, &req); err != nil || strings.TrimSpace(req.Token) == "" {
		http.Error(w, "无效请求体", http.StatusBadRequest)
		return
	}
	spokeID, secret, err := RegisterSpoke(strings.TrimSpace(req.Token))
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(registerResponse{SpokeID: spokeID, Token: secret})
}

// RegisterTokenHandler GET/POST /__hub__/v1/token
func RegisterTokenHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		http.Error(w, "只允许 GET/POST", http.StatusMethodNotAllowed)
		return
	}
	token, err := GenerateRegisterToken()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"token":      token,
		"expires_in": 900,
		"hint":       "Spoke 将自动使用此 Token 完成注册",
	})
}

// ProfileHandler POST /__hub__/v1/profile — Spoke 上报本机项目信息
func ProfileHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "只允许 POST", http.StatusMethodNotAllowed)
		return
	}
	secret := bearerToken(r)
	if secret == "" {
		http.Error(w, "缺少 Authorization", http.StatusUnauthorized)
		return
	}
	spokeID, ok := ValidateSpokeToken(secret)
	if !ok {
		http.Error(w, "无效或已吊销的凭证", http.StatusUnauthorized)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "读取请求失败", http.StatusBadRequest)
		return
	}
	var profile SpokeProfile
	if err := json.Unmarshal(body, &profile); err != nil {
		http.Error(w, "无效请求体", http.StatusBadRequest)
		return
	}
	profile.ObservedIP = clientIPFromRequest(r)
	if profile.PublicIP == "" && isPublicIP(profile.ObservedIP) {
		profile.PublicIP = profile.ObservedIP
	}
	if err := UpdateSpokeProfile(spokeID, profile); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"spoke":   spokeID,
		"message": "profile updated",
	})
}

func clientIPFromRequest(r *http.Request) string {
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err != nil {
		host = strings.TrimSpace(r.RemoteAddr)
	}
	remote := net.ParseIP(host)
	if remote != nil && remote.IsLoopback() {
		if value := strings.TrimSpace(r.Header.Get("X-Real-IP")); net.ParseIP(value) != nil {
			return value
		}
		if value := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0]); net.ParseIP(value) != nil {
			return value
		}
	}
	return host
}

func isPublicIP(value string) bool {
	ip := net.ParseIP(strings.TrimSpace(value))
	return ip != nil && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsUnspecified()
}

// ChatHandler POST /__hub__/v1/chat
func ChatHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "只允许 POST", http.StatusMethodNotAllowed)
		return
	}
	secret := bearerToken(r)
	if secret == "" {
		http.Error(w, "缺少 Authorization", http.StatusUnauthorized)
		return
	}
	if _, ok := ValidateSpokeToken(secret); !ok {
		http.Error(w, "无效或已吊销的凭证", http.StatusUnauthorized)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err != nil {
		http.Error(w, "读取请求失败", http.StatusBadRequest)
		return
	}
	var req chatRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "无效请求体", http.StatusBadRequest)
		return
	}

	aiCfg, err := agent.LoadAIConfig()
	if err != nil || !aiCfg.IsConfigured() || aiCfg.Provider == "hub" {
		writeChatError(w, "Hub 未配置有效的 AI 提供商，请在本机运行 /agent-config")
		return
	}
	provider, err := agent.NewProvider(aiCfg)
	if err != nil {
		writeChatError(w, fmt.Sprintf("创建 Provider 失败: %v", err))
		return
	}

	ctx := r.Context()
	resp, err := provider.Chat(ctx, req.Messages, req.Tools)
	if err != nil {
		writeChatError(w, fmt.Sprintf("AI 调用失败: %v", err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(chatResponse{
		Content:          resp.Content,
		ReasoningContent: resp.ReasoningContent,
		ToolCalls:        resp.ToolCalls,
	})
}

// ControlPollHandler GET /__hub__/v1/control/poll — Spoke 领取远程任务。
func ControlPollHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "只允许 GET", http.StatusMethodNotAllowed)
		return
	}
	spokeID, ok := authenticateSpoke(w, r)
	if !ok {
		return
	}
	protocolVersion, _ := strconv.Atoi(strings.TrimSpace(r.Header.Get("X-Ruoyi-Control-Version")))
	capabilities := splitControlCapabilities(r.Header.Get("X-Ruoyi-Capabilities"))
	job, err := ClaimControlJobWithCapabilities(spokeID, protocolVersion, capabilities)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if job == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(job)
}

// ControlResultHandler POST /__hub__/v1/control/result — Spoke 回传任务结果。
func ControlResultHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "只允许 POST", http.StatusMethodNotAllowed)
		return
	}
	spokeID, ok := authenticateSpoke(w, r)
	if !ok {
		return
	}
	var req controlResultRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 128<<10)).Decode(&req); err != nil {
		http.Error(w, "无效请求体", http.StatusBadRequest)
		return
	}
	var err error
	if req.Status == ControlJobRunning {
		err = StartControlJob(spokeID, req.JobID)
	} else {
		err = CompleteControlJob(spokeID, req.JobID, req.Status, req.Output, req.Error)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok", "job_id": req.JobID})
}

// ControlEnqueueAdminHandler POST /hub/control — Hub 本机创建远程任务。
func ControlEnqueueAdminHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "只允许 POST", http.StatusMethodNotAllowed)
		return
	}
	var req controlEnqueueRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 16<<10)).Decode(&req); err != nil {
		http.Error(w, "无效请求体", http.StatusBadRequest)
		return
	}
	options := ControlJobOptions{
		IdempotencyKey: req.IdempotencyKey,
		MaxAttempts:    req.MaxAttempts,
		Action:         req.Action,
		Actor:          req.Actor,
		Source:         req.Source,
		ConfirmedBy:    req.ConfirmedBy,
		ConfirmedAt:    req.ConfirmedAt,
	}
	w.Header().Set("Content-Type", "application/json")
	if strings.TrimSpace(req.Group) != "" {
		records := ListSpokesByGroup(req.Group)
		if len(records) == 0 {
			http.Error(w, "节点分组不存在或没有可用节点: "+strings.TrimSpace(req.Group), http.StatusBadRequest)
			return
		}
		targets := make([]string, 0, len(records))
		for _, record := range records {
			if !record.Maintenance {
				targets = append(targets, record.ID)
			}
		}
		if len(targets) == 0 {
			http.Error(w, "节点分组中的节点均处于维护状态: "+strings.TrimSpace(req.Group), http.StatusBadRequest)
			return
		}
		batch, err := EnqueueControlJobBatch(targets, req.Command, req.WorkDir, req.TimeoutSecs, options)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(batch)
		return
	}
	if len(req.SpokeIDs) > 0 {
		batch, err := EnqueueControlJobBatch(req.SpokeIDs, req.Command, req.WorkDir, req.TimeoutSecs, options)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(batch)
		return
	}
	job, err := EnqueueControlJobWithOptions(req.SpokeID, req.Command, req.WorkDir, req.TimeoutSecs, options)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(job)
}

// ControlCancelAdminHandler POST /hub/control/cancel — Hub 本机取消等待中的任务。
func ControlCancelAdminHandler(w http.ResponseWriter, r *http.Request) {
	handleControlMutation(w, r, CancelControlJob)
}

// ControlRetryAdminHandler POST /hub/control/retry — Hub 本机重试已结束的异常任务。
func ControlRetryAdminHandler(w http.ResponseWriter, r *http.Request) {
	handleControlMutation(w, r, RetryControlJob)
}

func handleControlMutation(w http.ResponseWriter, r *http.Request, mutate func(string) (ControlJob, error)) {
	if r.Method != http.MethodPost {
		http.Error(w, "只允许 POST", http.StatusMethodNotAllowed)
		return
	}
	var req controlMutationRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&req); err != nil {
		http.Error(w, "无效请求体", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.JobID) == "" {
		http.Error(w, "job_id 不能为空", http.StatusBadRequest)
		return
	}
	job, err := mutate(req.JobID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(job)
}

// ControlJobsAdminHandler GET /hub/jobs — Hub 本机查询远程任务。
func ControlJobsAdminHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "只允许 GET", http.StatusMethodNotAllowed)
		return
	}
	items, err := ListControlJobs(strings.TrimSpace(r.URL.Query().Get("spoke")), 20)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"count": len(items), "jobs": items})
}

func splitControlCapabilities(value string) []string {
	var capabilities []string
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			capabilities = append(capabilities, item)
		}
	}
	return capabilities
}
func authenticateSpoke(w http.ResponseWriter, r *http.Request) (string, bool) {
	secret := bearerToken(r)
	if secret == "" {
		http.Error(w, "缺少 Authorization", http.StatusUnauthorized)
		return "", false
	}
	spokeID, ok := ValidateSpokeToken(secret)
	if !ok {
		http.Error(w, "无效或已吊销的凭证", http.StatusUnauthorized)
		return "", false
	}
	return spokeID, true
}

func writeChatError(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadGateway)
	json.NewEncoder(w).Encode(chatResponse{Error: msg})
}

func bearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	}
	return ""
}

// TokenAdminHandler POST /hub/token — 生成本机注册 Token
func TokenAdminHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		http.Error(w, "只允许 GET/POST", http.StatusMethodNotAllowed)
		return
	}
	token, err := GenerateRegisterToken()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"token":      token,
		"expires_in": 900,
		"hint":       "在 spoke 服务器运行 /agent-config 选择 hub 并填入此 Token",
	})
}

// StatusAdminHandler GET /hub/status
func StatusAdminHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "只允许 GET", http.StatusMethodNotAllowed)
		return
	}
	items := ListSpokes()
	if group := strings.TrimSpace(r.URL.Query().Get("group")); group != "" {
		items = ListSpokesByGroup(group)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"count":  len(items),
		"spokes": items,
		"time":   time.Now().Format(time.RFC3339),
	})
}

// SpokeAdminHandler GET /hub/spoke?spoke=<id>
func SpokeAdminHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "只允许 GET", http.StatusMethodNotAllowed)
		return
	}
	spokeID := strings.TrimSpace(r.URL.Query().Get("spoke"))
	if spokeID == "" {
		http.Error(w, "缺少 spoke 参数", http.StatusBadRequest)
		return
	}
	item, ok := GetSpoke(spokeID)
	if !ok {
		http.Error(w, "spoke 不存在: "+spokeID, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(item)
}

// SpokeGovernanceAdminHandler POST /hub/spoke/governance?spoke=<id>
func SpokeGovernanceAdminHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "只允许 POST", http.StatusMethodNotAllowed)
		return
	}
	spokeID := strings.TrimSpace(r.URL.Query().Get("spoke"))
	if spokeID == "" {
		http.Error(w, "缺少 spoke 参数", http.StatusBadRequest)
		return
	}
	var patch SpokeGovernancePatch
	if err := json.NewDecoder(io.LimitReader(r.Body, 16<<10)).Decode(&patch); err != nil {
		http.Error(w, "无效请求体", http.StatusBadRequest)
		return
	}
	record, err := PatchSpokeGovernance(spokeID, patch)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(record)
}

// RevokeAdminHandler POST /hub/revoke?spoke=<id>
func RevokeAdminHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "只允许 POST", http.StatusMethodNotAllowed)
		return
	}
	spokeID := strings.TrimSpace(r.URL.Query().Get("spoke"))
	if spokeID == "" {
		http.Error(w, "缺少 spoke 参数", http.StatusBadRequest)
		return
	}
	if err := RevokeSpoke(spokeID); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "revoked", "spoke": spokeID})
}
