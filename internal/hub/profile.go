package hub

import "time"

// SpokeProfile Spoke 节点上报的服务器、能力与项目信息。
type SpokeProfile struct {
	SchemaVersion   int                   `json:"schema_version,omitempty"`
	AgentVersion    string                `json:"agent_version,omitempty"`
	ControlProtocol int                   `json:"control_protocol,omitempty"`
	Capabilities    []string              `json:"capabilities,omitempty"`
	Hostname        string                `json:"hostname,omitempty"`
	Label           string                `json:"label,omitempty"`       // 服务器用途/别名
	PublicIP        string                `json:"public_ip,omitempty"`   // Spoke 自动探测或用户补录
	ObservedIP      string                `json:"observed_ip,omitempty"` // Hub 根据连接来源写入
	PrivateIPs      []string              `json:"private_ips,omitempty"`
	OS              string                `json:"os,omitempty"`
	Arch            string                `json:"arch,omitempty"`
	ProjectName     string                `json:"project_name,omitempty"`
	ProjectType     string                `json:"project_type,omitempty"` // java/node/python/docker/go 等
	Description     string                `json:"description,omitempty"`
	Domain          string                `json:"domain,omitempty"`
	AppHome         string                `json:"app_home,omitempty"`
	Services        []SpokeServiceRef     `json:"services,omitempty"`
	Resources       SpokeResourceSnapshot `json:"resources,omitempty"`
	Health          SpokeHealthSummary    `json:"health,omitempty"`
	UpdatedAt       time.Time             `json:"updated_at,omitempty"`
}

// SpokeResourceSnapshot 节点资源快照，零值表示当前平台无法可靠采集。
type SpokeResourceSnapshot struct {
	CPUCount             int       `json:"cpu_count,omitempty"`
	MemoryTotalBytes     uint64    `json:"memory_total_bytes,omitempty"`
	MemoryAvailableBytes uint64    `json:"memory_available_bytes,omitempty"`
	DiskTotalBytes       uint64    `json:"disk_total_bytes,omitempty"`
	DiskFreeBytes        uint64    `json:"disk_free_bytes,omitempty"`
	CollectedAt          time.Time `json:"collected_at,omitempty"`
}

// SpokeHealthSummary 节点业务健康摘要。
type SpokeHealthSummary struct {
	Status    string    `json:"status,omitempty"`
	Summary   string    `json:"summary,omitempty"`
	CheckedAt time.Time `json:"checked_at,omitempty"`
}

// SpokeServiceRef 服务摘要（供 Hub 集中展示）。
type SpokeServiceRef struct {
	ID          string `json:"id"`
	Name        string `json:"name,omitempty"`
	ProjectType string `json:"project_type,omitempty"`
	ActiveEnv   string `json:"active_env,omitempty"`
	Endpoint    string `json:"endpoint,omitempty"`
	Health      string `json:"health,omitempty"`
}
