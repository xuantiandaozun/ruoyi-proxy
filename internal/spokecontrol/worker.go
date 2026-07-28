package spokecontrol

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"ruoyi-proxy/internal/agent"
	"ruoyi-proxy/internal/bootstrap"
	"ruoyi-proxy/internal/hub"
	"ruoyi-proxy/internal/nodeinfo"
)

const defaultPollInterval = 3 * time.Second

// Result Spoke 远程任务执行结果。
type Result struct {
	JobID  string `json:"job_id"`
	Status string `json:"status"`
	Output string `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
}

// Worker Spoke 出站远程控制执行器，可由 CLI 或常驻进程复用。
type Worker struct {
	config       agent.AIConfig
	client       *http.Client
	pollInterval time.Duration
	profileSync  func() error
	logf         func(format string, args ...interface{})
}

// NewFromLocalConfig 从本地 AI 配置创建 Spoke 执行器。
func NewFromLocalConfig(logf func(format string, args ...interface{})) (*Worker, error) {
	cfg, err := agent.LoadAIConfig()
	if err != nil {
		return nil, fmt.Errorf("读取 Spoke 配置: %v", err)
	}
	if cfg.Provider != "hub" || !cfg.IsConfigured() {
		return nil, fmt.Errorf("尚未完成 Hub 注册")
	}
	if logf == nil {
		logf = func(string, ...interface{}) {}
	}
	return &Worker{
		config:       cfg,
		client:       &http.Client{Timeout: 15 * time.Second},
		pollInterval: defaultPollInterval,
		profileSync:  bootstrap.RefreshAndSyncSpokeProfile,
		logf:         logf,
	}, nil
}

// Run 持续领取并执行 Hub 任务，直到上下文取消。
func (w *Worker) Run(ctx context.Context) error {
	if w == nil {
		return fmt.Errorf("Spoke 执行器不能为空")
	}
	w.logf("Spoke 远程控制执行器已启动")
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()
	nextProfileSync := time.Time{}
	for {
		if w.profileSync != nil && !time.Now().Before(nextProfileSync) {
			nextProfileSync = time.Now().Add(time.Minute)
			if err := w.profileSync(); err != nil && ctx.Err() == nil {
				w.logf("刷新 Spoke 能力与健康档案失败: %v", err)
			}
		}
		job, err := w.poll(ctx)
		if err != nil && ctx.Err() == nil {
			w.logf("领取 Hub 任务失败: %v", err)
		} else if job != nil {
			running := Result{JobID: job.ID, Status: hub.ControlJobRunning}
			if err := w.postResultWithRetry(ctx, running); err != nil {
				if ctx.Err() == nil {
					w.logf("确认任务[%s]开始失败，本次不执行: %v", job.ID, err)
				}
				continue
			}
			result := executeJob(ctx, *job)
			if err := w.postResultWithRetry(ctx, result); err != nil && ctx.Err() == nil {
				w.logf("回传任务[%s]结果失败: %v", job.ID, err)
			}
		}
		select {
		case <-ctx.Done():
			w.logf("Spoke 远程控制执行器已停止")
			return nil
		case <-ticker.C:
		}
	}
}

func (w *Worker) poll(ctx context.Context) (*hub.ControlJob, error) {
	url := strings.TrimRight(w.config.BaseURL, "/") + "/__hub__/v1/control/poll"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+w.config.APIKey)
	req.Header.Set("X-Ruoyi-Control-Version", fmt.Sprintf("%d", nodeinfo.ControlProtocolVersion))
	req.Header.Set("X-Ruoyi-Capabilities", strings.Join(nodeinfo.Capabilities(), ","))
	resp, err := w.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var job hub.ControlJob
	if err := json.NewDecoder(io.LimitReader(resp.Body, 128<<10)).Decode(&job); err != nil {
		return nil, err
	}
	return &job, nil
}

func executeJob(parent context.Context, job hub.ControlJob) Result {
	timeout := time.Duration(job.TimeoutSecs) * time.Second
	if timeout <= 0 || timeout > 5*time.Minute {
		timeout = time.Minute
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	if job.Action != nil && job.Action.Type != hub.ControlActionShell {
		return executeStructuredAction(ctx, job)
	}

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", job.Command)
	} else {
		cmd = exec.CommandContext(ctx, "bash", "-lc", job.Command)
	}
	if job.WorkDir != "" {
		if info, err := os.Stat(job.WorkDir); err != nil || !info.IsDir() {
			return Result{JobID: job.ID, Status: "failed", Error: "工作目录不存在或不可访问: " + job.WorkDir}
		}
		cmd.Dir = job.WorkDir
	}
	output, err := cmd.CombinedOutput()
	result := Result{JobID: job.ID, Status: "succeeded", Output: truncateOutput(string(output), 64*1024)}
	if ctx.Err() == context.DeadlineExceeded {
		result.Status = "timed_out"
		result.Error = "命令执行超时"
	} else if err != nil {
		result.Status = "failed"
		result.Error = err.Error()
	}
	return result
}

func (w *Worker) postResultWithRetry(ctx context.Context, result Result) error {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if err := w.postResult(ctx, result); err == nil {
			return nil
		} else {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt+1) * time.Second):
		}
	}
	return lastErr
}

func (w *Worker) postResult(ctx context.Context, result Result) error {
	body, err := json.Marshal(result)
	if err != nil {
		return err
	}
	url := strings.TrimRight(w.config.BaseURL, "/") + "/__hub__/v1/control/result"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+w.config.APIKey)
	req.Header.Set("X-Ruoyi-Control-Version", fmt.Sprintf("%d", nodeinfo.ControlProtocolVersion))
	req.Header.Set("X-Ruoyi-Capabilities", strings.Join(nodeinfo.Capabilities(), ","))
	req.Header.Set("Content-Type", "application/json")
	resp, err := w.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return nil
}

func truncateOutput(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max] + "\n...（输出已截断）"
}
