package hub

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
