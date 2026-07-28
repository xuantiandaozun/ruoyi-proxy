package spokecontrol

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"ruoyi-proxy/internal/config"
	"ruoyi-proxy/internal/database"
	"ruoyi-proxy/internal/hub"
)

func executeStructuredAction(ctx context.Context, job hub.ControlJob) Result {
	action := job.Action
	if action == nil {
		return Result{JobID: job.ID, Status: hub.ControlJobFailed, Error: "结构化动作不能为空"}
	}
	if err := action.Validate(); err != nil {
		return Result{JobID: job.ID, Status: hub.ControlJobFailed, Error: err.Error()}
	}
	if action.Type == hub.ControlActionDatabaseQuery {
		return executeDatabaseQueryAction(ctx, job)
	}
	cmd, err := buildServiceActionCommand(ctx, job)
	if err != nil {
		return Result{JobID: job.ID, Status: hub.ControlJobFailed, Error: err.Error()}
	}
	output, runErr := cmd.CombinedOutput()
	return commandResult(ctx, job.ID, output, runErr)
}

func buildServiceActionCommand(ctx context.Context, job hub.ControlJob) (*exec.Cmd, error) {
	cfg, err := config.LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("读取服务配置失败: %v", err)
	}
	serviceID := strings.TrimSpace(job.Action.Params["service_id"])
	if serviceID == "" {
		serviceID = "default"
	}
	service := cfg.Services[serviceID]
	if service == nil {
		return nil, fmt.Errorf("服务不存在: %s", serviceID)
	}
	scriptPath, err := resolveServiceScript(service.ScriptPath, job.WorkDir)
	if err != nil {
		return nil, err
	}
	command := job.Action.Type
	args := []string{scriptPath, command}
	if command == hub.ControlActionLogs {
		lines := strings.TrimSpace(job.Action.Params["lines"])
		if lines == "" {
			lines = "100"
		}
		args = append(args, lines)
	}
	cmd := exec.CommandContext(ctx, "bash", args...)
	cmd.Env = append(os.Environ(),
		"SERVICE_ID="+serviceID,
		"APP_NAME="+defaultString(service.AppName, "ruoyi"),
		"APP_JAR_PATTERN="+defaultString(service.JarFile, "ruoyi-*.jar"),
		"BLUE_PORT="+targetPort(service.BlueTarget, "8080"),
		"GREEN_PORT="+targetPort(service.GreenTarget, "8081"),
		"APP_HOME="+filepath.Dir(filepath.Dir(scriptPath)),
	)
	if job.WorkDir != "" {
		cmd.Dir = job.WorkDir
	}
	return cmd, nil
}

func executeDatabaseQueryAction(ctx context.Context, job hub.ControlJob) Result {
	statement := strings.TrimSpace(job.Action.Params["sql"])
	if !database.IsReadOnlySQL(statement) {
		return Result{
			JobID:  job.ID,
			Status: hub.ControlJobFailed,
			Error:  "结构化 database_query 当前仅允许只读 SQL",
		}
	}
	profile, err := database.GetProfile(strings.TrimSpace(job.Action.Params["profile"]))
	if err != nil {
		return Result{JobID: job.ID, Status: hub.ControlJobFailed, Error: err.Error()}
	}
	queryResult, err := database.Execute(ctx, profile, statement)
	if err != nil {
		return commandResult(ctx, job.ID, nil, err)
	}
	output, err := json.MarshalIndent(queryResult, "", "  ")
	if err != nil {
		return Result{JobID: job.ID, Status: hub.ControlJobFailed, Error: fmt.Sprintf("编码查询结果失败: %v", err)}
	}
	return Result{JobID: job.ID, Status: hub.ControlJobSucceeded, Output: truncateOutput(string(output), 64*1024)}
}

func commandResult(ctx context.Context, jobID string, output []byte, err error) Result {
	result := Result{JobID: jobID, Status: hub.ControlJobSucceeded, Output: truncateOutput(string(output), 64*1024)}
	if ctx.Err() == context.DeadlineExceeded {
		result.Status = hub.ControlJobTimedOut
		result.Error = "动作执行超时"
	} else if err != nil {
		result.Status = hub.ControlJobFailed
		result.Error = err.Error()
	}
	return result
}

func resolveServiceScript(configured, workDir string) (string, error) {
	var candidates []string
	if strings.TrimSpace(configured) != "" {
		candidates = append(candidates, strings.TrimSpace(configured))
	}
	if strings.TrimSpace(workDir) != "" {
		candidates = append(candidates, filepath.Join(workDir, "scripts", "service.sh"))
	}
	candidates = append(candidates, filepath.Join("scripts", "service.sh"))
	if executable, err := os.Executable(); err == nil {
		executableDir := filepath.Dir(executable)
		candidates = append(candidates,
			filepath.Join(executableDir, "scripts", "service.sh"),
			filepath.Join(filepath.Dir(executableDir), "scripts", "service.sh"),
		)
	}
	for _, candidate := range candidates {
		absolute, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		if info, err := os.Stat(absolute); err == nil && !info.IsDir() {
			return absolute, nil
		}
	}
	return "", fmt.Errorf("未找到服务控制脚本 service.sh")
}

func targetPort(target, fallback string) string {
	parsed, err := url.Parse(strings.TrimSpace(target))
	if err == nil {
		if port := parsed.Port(); port != "" {
			return port
		}
		if host := parsed.Host; host != "" {
			if _, port, splitErr := net.SplitHostPort(host); splitErr == nil && port != "" {
				return port
			}
		}
	}
	if value, err := strconv.Atoi(strings.TrimSpace(target)); err == nil && value > 0 && value <= 65535 {
		return strconv.Itoa(value)
	}
	return fallback
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}
