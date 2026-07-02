package bootstrap

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"ruoyi-proxy/internal/agent"
)

// CheckItem 单项自检结果
type CheckItem struct {
	Name    string
	OK      bool
	Detail  string
	Skipped bool
}

// RunEnvChecks 运行基础环境自检
func RunEnvChecks() []CheckItem {
	return runEnvChecks(false)
}

func runEnvChecks(includeHubRoute bool) []CheckItem {
	items := []CheckItem{
		checkCommand("bash", "bash", "--version"),
		checkCommand("java", "java", "-version"),
	}
	if runtime.GOOS == "linux" {
		items = append(items,
			checkCommand("nginx", "nginx", "-v"),
			checkSystemd("nginx"),
		)
		if includeHubRoute {
			items = append(items, checkHubNginxRoute())
		}
	} else {
		items = append(items, CheckItem{Name: "nginx", Skipped: true, Detail: "非 Linux，跳过"})
	}
	items = append(items, checkWritable("configs"))
	return items
}

// RunHubChecks Hub 节点完整自检（含网关端口）
func RunHubChecks() []CheckItem {
	items := runEnvChecks(true)
	items = append(items,
		checkTCPPort("proxy:8000", "127.0.0.1", "8000"),
		checkTCPPort("mgmt:8001", "127.0.0.1", "8001"),
		checkHubAIConfig(),
	)
	return items
}

func checkHubAIConfig() CheckItem {
	item := CheckItem{Name: "hub:ai"}
	aiCfg, err := agent.LoadAIConfig()
	if err != nil || !aiCfg.IsConfigured() || aiCfg.Provider == "hub" {
		item.Detail = "未配置有效 AI（请 /agent-config）"
		return item
	}
	item.OK = true
	item.Detail = fmt.Sprintf("%s / %s", aiCfg.Provider, aiCfg.Model)
	return item
}

func checkTCPPort(name, host, port string) CheckItem {
	item := CheckItem{Name: name}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 2*time.Second)
	if err != nil {
		item.Detail = "未监听（请先无参数启动代理进程）"
		return item
	}
	conn.Close()
	item.OK = true
	item.Detail = "正常"
	return item
}

func checkHubNginxRoute() CheckItem {
	item := CheckItem{Name: "nginx:hub路由"}
	if runtime.GOOS != "linux" {
		item.Skipped = true
		item.Detail = "非 Linux，跳过"
		return item
	}
	paths := LoadAppPaths()
	for _, confPath := range uniquePaths([]string{paths.NginxConf, "/etc/nginx/conf.d/ruoyi.conf"}) {
		content, err := readNginxConf(confPath)
		if err != nil {
			continue
		}
		if strings.Contains(content, hubLocationMarker) {
			localOK, localDetail := probeHubRegisterEndpoint("http://127.0.0.1:8000/__hub__/v1/register")
			if !localOK {
				item.Detail = confPath + " 已含 /__hub__/，但本机代理未命中 Hub: " + localDetail
				return item
			}
			if isPlaceholderDomain(paths.Domain) {
				item.OK = true
				item.Detail = confPath + " 已含 /__hub__/，本机端点正常（未配置有效域名，跳过公网探测）"
				return item
			}
			scheme := "http"
			if paths.EnableHTTPS {
				scheme = "https"
			}
			publicURL := scheme + "://" + paths.Domain + "/__hub__/v1/register"
			publicOK, publicDetail := probeHubRegisterEndpoint(publicURL)
			if !publicOK {
				item.Detail = confPath + " 已含 /__hub__/，但域名端点未命中 Hub: " + publicDetail
				return item
			}
			item.OK = true
			item.Detail = confPath + " 已含 /__hub__/，本机/域名端点正常"
			return item
		}
		item.Detail = confPath + " 缺少 /__hub__/"
		return item
	}
	item.Detail = "未找到 Nginx 配置文件"
	return item
}

func isPlaceholderDomain(domain string) bool {
	domain = strings.TrimSpace(strings.ToLower(domain))
	return domain == "" || domain == "example.com"
}

func probeHubRegisterEndpoint(url string) (bool, string) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return false, err.Error()
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	text := strings.TrimSpace(string(body))
	if resp.StatusCode == http.StatusMethodNotAllowed && strings.Contains(text, "只允许 POST") {
		return true, "正常"
	}
	if text == "" {
		text = resp.Status
	}
	return false, fmt.Sprintf("HTTP %d %s", resp.StatusCode, text)
}

func checkCommand(name, cmd string, args ...string) CheckItem {
	item := CheckItem{Name: name}
	if _, err := exec.LookPath(cmd); err != nil {
		item.Detail = "未安装"
		return item
	}
	out, err := exec.Command(cmd, args...).CombinedOutput()
	if err != nil {
		item.Detail = strings.TrimSpace(string(out))
		if item.Detail == "" {
			item.Detail = err.Error()
		}
		return item
	}
	item.OK = true
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) > 0 {
		item.Detail = lines[0]
		if len(lines) > 1 && name == "java" {
			item.Detail = lines[0] + " " + lines[1]
		}
	}
	return item
}

func checkSystemd(service string) CheckItem {
	item := CheckItem{Name: "systemd:" + service}
	if _, err := exec.LookPath("systemctl"); err != nil {
		item.Skipped = true
		item.Detail = "无 systemctl"
		return item
	}
	out, err := exec.Command("systemctl", "is-active", service).CombinedOutput()
	status := strings.TrimSpace(string(out))
	if err != nil {
		item.Detail = "状态: " + status
		return item
	}
	item.OK = status == "active"
	item.Detail = "状态: " + status
	return item
}

func checkWritable(dir string) CheckItem {
	item := CheckItem{Name: "配置目录"}
	if err := os.MkdirAll(dir, 0755); err != nil {
		item.Detail = err.Error()
		return item
	}
	testFile := filepath.Join(dir, ".write_test")
	if err := os.WriteFile(testFile, []byte("ok"), 0644); err != nil {
		item.Detail = "不可写: " + err.Error()
		return item
	}
	_ = os.Remove(testFile)
	item.OK = true
	item.Detail = dir + " 可写"
	return item
}

// FormatCheckReport 格式化自检报告
func FormatCheckReport(title string, items []CheckItem) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\n\033[1;34m═══ %s ═══\033[0m\n", title)
	for _, it := range items {
		icon := "\033[1;31m✗\033[0m"
		if it.Skipped {
			icon = "\033[1;33m-\033[0m"
		} else if it.OK {
			icon = "\033[1;32m✓\033[0m"
		}
		fmt.Fprintf(&b, "  %s %-14s %s\n", icon, it.Name+":", it.Detail)
	}
	return b.String()
}

// DetectProjectType 根据当前目录文件猜测项目类型
func DetectProjectType() string {
	checks := []struct {
		typ  string
		path string
	}{
		{"java", "pom.xml"},
		{"java", "build.gradle"},
		{"node", "package.json"},
		{"python", "requirements.txt"},
		{"python", "pyproject.toml"},
		{"go", "go.mod"},
		{"docker", "docker-compose.yml"},
		{"docker", "Dockerfile"},
	}
	for _, c := range checks {
		if _, err := os.Stat(c.path); err == nil {
			return c.typ
		}
	}
	// 常见 Java 部署目录
	for _, dir := range []string{"apps", "jar", "lib"} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".jar") {
				return "java"
			}
		}
	}
	return ""
}

// Hostname 获取主机名
func Hostname() string {
	h, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return h
}
