// Package nodeinfo 采集 Spoke 节点可安全上报的版本、能力和资源摘要。
package nodeinfo

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"time"

	"ruoyi-proxy/internal/buildinfo"
	"ruoyi-proxy/internal/config"
)

const (
	// ControlProtocolVersion 当前 Spoke Worker 支持的远程控制协议版本。
	ControlProtocolVersion = 2

	CapabilityControlV2        = "control.v2"
	CapabilityShell            = "shell"
	CapabilityServiceStatus    = "service.status"
	CapabilityServiceLogs      = "service.logs"
	CapabilityServiceRestart   = "service.restart"
	CapabilityServiceDeploy    = "service.deploy"
	CapabilityDatabaseRead     = "database.query.read"
	CapabilityProxyBlueGreen   = "proxy.blue_green"
	CapabilityDockerInspection = "docker.inspect"
)

// Resources 节点资源快照，无法可靠采集的字段保持为零。
type Resources struct {
	CPUCount             int       `json:"cpu_count,omitempty"`
	MemoryTotalBytes     uint64    `json:"memory_total_bytes,omitempty"`
	MemoryAvailableBytes uint64    `json:"memory_available_bytes,omitempty"`
	DiskTotalBytes       uint64    `json:"disk_total_bytes,omitempty"`
	DiskFreeBytes        uint64    `json:"disk_free_bytes,omitempty"`
	CollectedAt          time.Time `json:"collected_at,omitempty"`
}

// Snapshot 返回当前节点的静态能力与资源快照。
func Snapshot() (string, int, []string, Resources) {
	resources := collectPlatformResources()
	resources.CPUCount = runtime.NumCPU()
	resources.CollectedAt = time.Now()
	return buildinfo.VersionLabel(), ControlProtocolVersion, Capabilities(), resources
}

// Capabilities 返回当前二进制和本机环境支持的能力集合。
func Capabilities() []string {
	items := []string{
		CapabilityControlV2,
		CapabilityShell,
		CapabilityDatabaseRead,
	}
	if hasServiceControlScript() {
		items = append(items,
			CapabilityServiceStatus,
			CapabilityServiceLogs,
			CapabilityServiceRestart,
			CapabilityServiceDeploy,
		)
	}
	if _, err := os.Stat(config.ConfigFile); err == nil {
		items = append(items, CapabilityProxyBlueGreen)
	}
	if _, err := exec.LookPath("docker"); err == nil {
		items = append(items, CapabilityDockerInspection)
	}
	sort.Strings(items)
	return items
}

func hasServiceControlScript() bool {
	candidates := []string{filepath.Join("scripts", "service.sh")}
	if executable, err := os.Executable(); err == nil {
		directory := filepath.Dir(executable)
		candidates = append(candidates,
			filepath.Join(directory, "scripts", "service.sh"),
			filepath.Join(filepath.Dir(directory), "scripts", "service.sh"),
		)
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return true
		}
	}
	return false
}

// HasCapability 判断能力集合中是否包含指定能力。
func HasCapability(capabilities []string, required string) bool {
	for _, capability := range capabilities {
		if capability == required {
			return true
		}
	}
	return false
}
