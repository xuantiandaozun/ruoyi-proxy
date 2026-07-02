package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type hubRegisterRequest struct {
	Token string `json:"token"`
}

type hubRegisterResponse struct {
	SpokeID string `json:"spoke_id"`
	Token   string `json:"token"`
}

type hubTokenResponse struct {
	Token string `json:"token"`
}

// RequestHubRegisterToken 向 Hub 申请一次性注册 Token
func RequestHubRegisterToken(hubURL string) (string, error) {
	hubURL = strings.TrimSpace(hubURL)
	if hubURL == "" {
		return "", fmt.Errorf("Hub 地址不能为空")
	}
	url := strings.TrimRight(hubURL, "/") + "/__hub__/v1/token"
	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return "", err
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("申请注册 Token 失败: %v", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("申请注册 Token 失败 HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	var out hubTokenResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return "", fmt.Errorf("解析注册 Token 响应失败: %v, body=%s", err, strings.TrimSpace(string(data)))
	}
	if strings.TrimSpace(out.Token) == "" {
		return "", fmt.Errorf("Hub 未返回注册 Token，body=%s", strings.TrimSpace(string(data)))
	}
	return strings.TrimSpace(out.Token), nil
}

// RegisterWithHub 使用一次性 Token 向 Hub 注册并获取长期凭证
func RegisterWithHub(hubURL, oneTimeToken string) (token, spokeID string, err error) {
	hubURL = strings.TrimSpace(hubURL)
	oneTimeToken = strings.TrimSpace(oneTimeToken)
	if hubURL == "" || oneTimeToken == "" {
		return "", "", fmt.Errorf("Hub 地址和注册 Token 不能为空")
	}
	body, err := json.Marshal(hubRegisterRequest{Token: oneTimeToken})
	if err != nil {
		return "", "", err
	}
	url := strings.TrimRight(hubURL, "/") + "/__hub__/v1/register"
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("注册请求失败: %v", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", "", err
	}
	if resp.StatusCode >= 400 {
		return "", "", fmt.Errorf("注册失败 HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	var out hubRegisterResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return "", "", fmt.Errorf("解析注册响应失败（可能 Nginx 未转发到 Hub）: %v, body=%s", err, strings.TrimSpace(string(data)))
	}
	if out.Token == "" {
		var proxyErr struct {
			Msg  string `json:"msg"`
			Code int    `json:"code"`
		}
		if err := json.Unmarshal(data, &proxyErr); err == nil && proxyErr.Msg != "" && proxyErr.Code != 0 {
			return "", "", fmt.Errorf("Hub 未返回有效凭证，疑似请求被 Java 服务拦截（请检查 Nginx HTTPS server 的 /__hub__/ 路由），body=%s", strings.TrimSpace(string(data)))
		}
		return "", "", fmt.Errorf("Hub 未返回有效凭证，body=%s", strings.TrimSpace(string(data)))
	}
	return out.Token, out.SpokeID, nil
}
