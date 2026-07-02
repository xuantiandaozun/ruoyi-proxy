package hub

import "time"

// SpokeProfile Spoke 节点上报的服务器与项目信息
type SpokeProfile struct {
	Hostname    string            `json:"hostname,omitempty"`
	Label       string            `json:"label,omitempty"`        // 服务器用途/别名
	ProjectName string            `json:"project_name,omitempty"`
	ProjectType string            `json:"project_type,omitempty"` // java/node/python/docker/go 等
	Description string            `json:"description,omitempty"`
	Domain      string            `json:"domain,omitempty"`
	AppHome     string            `json:"app_home,omitempty"`
	Services    []SpokeServiceRef `json:"services,omitempty"`
	UpdatedAt   time.Time         `json:"updated_at,omitempty"`
}

// SpokeServiceRef 服务摘要（供 Hub 集中展示）
type SpokeServiceRef struct {
	ID          string `json:"id"`
	Name        string `json:"name,omitempty"`
	ProjectType string `json:"project_type,omitempty"`
	ActiveEnv   string `json:"active_env,omitempty"`
}
