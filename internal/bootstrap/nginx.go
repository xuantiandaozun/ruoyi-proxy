package bootstrap

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const hubLocationMarker = "location ^~ /__hub__/"
const hubLocationLegacyMarker = "location /__hub__/"

// hubLocationBlock 插入到 server 块内的 Hub 转发规则
func hubLocationBlock(useUpstream bool) string {
	backend := "http://127.0.0.1:8000/__hub__/"
	if useUpstream {
		backend = "http://ruoyi_backend/__hub__/"
	}
	return fmt.Sprintf(`
    # Hub AI 网关（Spoke 注册与 Chat 转发）
    # 必须放在通用 / 规则之前，且精确匹配 /__hub__/ 前缀
    location ^~ /__hub__/ {
        proxy_pass %s;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_http_version 1.1;
        proxy_connect_timeout 120s;
        proxy_send_timeout 900s;
        proxy_read_timeout 900s;
    }
`, backend)
}

// NginxHubResult Nginx Hub 路由检测结果（只读）
type NginxHubResult struct {
	Checked    bool
	AlreadyOK  bool
	ConfigPath string
	Message    string
}

// CheckHubNginxRoute 只读检测 Nginx 是否包含正确的 /__hub__/ 路由
func CheckHubNginxRoute() NginxHubResult {
	res := NginxHubResult{}
	if runtime.GOOS != "linux" {
		res.Message = "非 Linux 环境，跳过 Nginx 检测"
		return res
	}
	if _, err := exec.LookPath("nginx"); err != nil {
		res.Message = "未安装 nginx，跳过"
		return res
	}

	paths := LoadAppPaths()
	candidates := uniquePaths([]string{
		paths.NginxConf,
		"/etc/nginx/conf.d/ruoyi.conf",
	})

	for _, confPath := range candidates {
		if confPath == "" {
			continue
		}
		content, err := readNginxConf(confPath)
		if err != nil {
			continue
		}
		res.Checked = true
		res.ConfigPath = confPath

		if strings.Contains(content, hubLocationMarker) {
			res.AlreadyOK = true
			res.Message = fmt.Sprintf("Nginx 已包含 Hub 路由: %s", confPath)
			return res
		}
		if strings.Contains(content, hubLocationLegacyMarker) {
			res.Message = fmt.Sprintf("%s 包含旧版 /__hub__/ 路由，缺少 ^~ 前缀，建议让 AI 修复", confPath)
			return res
		}
		res.Message = fmt.Sprintf("%s 缺少 /__hub__/ 路由，建议让 AI 修复", confPath)
		return res
	}

	if !res.Checked {
		res.Message = "未找到可用的 Nginx 配置文件"
	}
	return res
}

// removeHubLocationBlocks 删除所有 /__hub__/ 相关 location 块（包括旧的）
func removeHubLocationBlocks(content string) string {
	for {
		idx := strings.Index(content, hubLocationLegacyMarker)
		if idx < 0 {
			break
		}
		// 找到块起始（包括前面的注释）
		start := idx
		if lineStart := strings.LastIndex(content[:idx], "\n"); lineStart >= 0 {
			start = lineStart + 1
		}
		// 找到块结束：匹配第一个 '}' 且缩进相同
		depth := 1
		pos := idx + len(hubLocationLegacyMarker)
		for pos < len(content) && depth > 0 {
			if content[pos] == '{' {
				depth++
			} else if content[pos] == '}' {
				depth--
			}
			pos++
		}
		// 包含 '}' 后面的换行
		end := pos
		if end < len(content) && content[end] == '\n' {
			end++
		}
		// 删除前面可能的 "# Hub AI" 注释行
		if start > 0 {
			prev := content[:start]
			if commentIdx := strings.LastIndex(prev, "# Hub AI"); commentIdx >= 0 {
				if lastNL := strings.LastIndex(prev[:commentIdx], "\n"); lastNL >= 0 {
					start = lastNL + 1
				}
			}
		}
		content = content[:start] + content[end:]
	}
	return content
}

func uniquePaths(in []string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, p := range in {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

func readNginxConf(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		return string(data), nil
	}
	out, err2 := exec.Command("sudo", "cat", path).CombinedOutput()
	if err2 != nil {
		return "", fmt.Errorf("%v / %v", err, err2)
	}
	return string(out), nil
}

func writeNginxConf(path, content string) error {
	backup := path + ".bak." + time.Now().Format("20060102-150405")
	_ = exec.Command("sudo", "cp", path, backup).Run()

	tmp, err := os.CreateTemp("", "ruoyi-nginx-*.conf")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		return err
	}
	tmp.Close()

	if err := exec.Command("sudo", "cp", tmpPath, path).Run(); err != nil {
		return fmt.Errorf("sudo cp 失败: %v", err)
	}
	if out, err := exec.Command("sudo", "nginx", "-t").CombinedOutput(); err != nil {
		_ = exec.Command("sudo", "cp", backup, path).Run()
		return fmt.Errorf("nginx -t 失败: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func reloadNginx() error {
	if err := exec.Command("sudo", "systemctl", "reload", "nginx").Run(); err != nil {
		return exec.Command("sudo", "nginx", "-s", "reload").Run()
	}
	return nil
}

func insertHubLocation(content, block string) string {
	markers := []string{
		"# API代理到Go程序",
		"location /api/",
		"location /admin",
	}
	for _, m := range markers {
		if idx := strings.Index(content, m); idx >= 0 {
			return content[:idx] + block + "\n" + content[idx:]
		}
	}
	// HTTPS server 块：在 access_log 之后插入
	if idx := strings.Index(content, "access_log"); idx >= 0 {
		lineEnd := strings.Index(content[idx:], "\n")
		if lineEnd >= 0 {
			pos := idx + lineEnd + 1
			return content[:pos] + block + content[pos:]
		}
	}
	return content
}

// LogNginxHubResult 记录 Nginx 检测结果
func LogNginxHubResult(res NginxHubResult) {
	if res.Message == "" {
		return
	}
	if res.AlreadyOK {
		log.Printf("[bootstrap] %s", res.Message)
	} else {
		log.Printf("[bootstrap] Nginx: %s", res.Message)
	}
}
