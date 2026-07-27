package hub

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

const controlJobsFile = "configs/hub_jobs.json"
const maxStoredControlJobs = 200

// ControlJob Hub 下发给 Spoke 的远程任务。
type ControlJob struct {
	ID          string    `json:"id"`
	SpokeID     string    `json:"spoke_id"`
	Command     string    `json:"command"`
	WorkDir     string    `json:"workdir,omitempty"`
	TimeoutSecs int       `json:"timeout_seconds"`
	Status      string    `json:"status"`
	Output      string    `json:"output,omitempty"`
	Error       string    `json:"error,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	StartedAt   time.Time `json:"started_at,omitempty"`
	FinishedAt  time.Time `json:"finished_at,omitempty"`
}

type controlJobStore struct {
	mu     sync.RWMutex
	loaded bool
	jobs   map[string]*ControlJob
}

var defaultControlStore = &controlJobStore{jobs: make(map[string]*ControlJob)}

// EnqueueControlJob 创建等待 Spoke 执行的远程任务。
func EnqueueControlJob(spokeID, command, workDir string, timeoutSecs int) (ControlJob, error) {
	spokeID = strings.TrimSpace(spokeID)
	command = strings.TrimSpace(command)
	if spokeID == "" || command == "" {
		return ControlJob{}, fmt.Errorf("spoke ID 和命令不能为空")
	}
	if len(command) > 4096 {
		return ControlJob{}, fmt.Errorf("命令不能超过 4096 字符")
	}
	spoke, ok := GetSpoke(spokeID)
	if !ok {
		return ControlJob{}, fmt.Errorf("spoke 不存在: %s", spokeID)
	}
	if spoke.Revoked {
		return ControlJob{}, fmt.Errorf("spoke 已吊销: %s", spokeID)
	}
	if timeoutSecs <= 0 {
		timeoutSecs = 60
	}
	if timeoutSecs > 300 {
		timeoutSecs = 300
	}
	id, err := newControlJobID()
	if err != nil {
		return ControlJob{}, err
	}
	job := ControlJob{
		ID:          id,
		SpokeID:     spokeID,
		Command:     command,
		WorkDir:     strings.TrimSpace(workDir),
		TimeoutSecs: timeoutSecs,
		Status:      "pending",
		CreatedAt:   time.Now(),
	}

	defaultControlStore.mu.Lock()
	defer defaultControlStore.mu.Unlock()
	if err := defaultControlStore.loadLocked(); err != nil {
		return ControlJob{}, err
	}
	defaultControlStore.jobs[job.ID] = &job
	defaultControlStore.pruneLocked()
	if err := defaultControlStore.saveLocked(); err != nil {
		return ControlJob{}, err
	}
	return job, nil
}

// ClaimControlJob 领取指定 Spoke 最早的待执行任务。
func ClaimControlJob(spokeID string) (*ControlJob, error) {
	defaultControlStore.mu.Lock()
	defer defaultControlStore.mu.Unlock()
	if err := defaultControlStore.loadLocked(); err != nil {
		return nil, err
	}
	var selected *ControlJob
	for _, job := range defaultControlStore.jobs {
		if job.SpokeID != spokeID || job.Status != "pending" {
			continue
		}
		if selected == nil || job.CreatedAt.Before(selected.CreatedAt) {
			selected = job
		}
	}
	if selected == nil {
		return nil, nil
	}
	selected.Status = "running"
	selected.StartedAt = time.Now()
	if err := defaultControlStore.saveLocked(); err != nil {
		return nil, err
	}
	copyJob := *selected
	return &copyJob, nil
}

// CompleteControlJob 保存 Spoke 返回的执行结果。
func CompleteControlJob(spokeID, jobID, status, output, errorText string) error {
	if status != "succeeded" && status != "failed" && status != "timed_out" {
		return fmt.Errorf("无效任务状态: %s", status)
	}
	defaultControlStore.mu.Lock()
	defer defaultControlStore.mu.Unlock()
	if err := defaultControlStore.loadLocked(); err != nil {
		return err
	}
	job, ok := defaultControlStore.jobs[jobID]
	if !ok || job.SpokeID != spokeID {
		return fmt.Errorf("任务不存在或不属于当前 spoke: %s", jobID)
	}
	if job.Status != "running" {
		return fmt.Errorf("任务当前不可完成: %s", job.Status)
	}
	job.Status = status
	job.Output = truncateControlText(output, 64*1024)
	job.Error = truncateControlText(errorText, 4096)
	job.FinishedAt = time.Now()
	return defaultControlStore.saveLocked()
}

// ListControlJobs 返回远程任务，spokeID 为空时返回全部。
func ListControlJobs(spokeID string, limit int) ([]ControlJob, error) {
	defaultControlStore.mu.Lock()
	defer defaultControlStore.mu.Unlock()
	if err := defaultControlStore.loadLocked(); err != nil {
		return nil, err
	}
	items := make([]ControlJob, 0, len(defaultControlStore.jobs))
	for _, job := range defaultControlStore.jobs {
		if spokeID == "" || job.SpokeID == spokeID {
			items = append(items, *job)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (s *controlJobStore) loadLocked() error {
	if s.loaded {
		return nil
	}
	s.loaded = true
	data, err := os.ReadFile(controlJobsFile)
	if err != nil {
		if os.IsNotExist(err) {
			s.jobs = make(map[string]*ControlJob)
			return nil
		}
		return fmt.Errorf("读取 Hub 任务失败: %v", err)
	}
	var items []ControlJob
	if err := json.Unmarshal(data, &items); err != nil {
		return fmt.Errorf("解析 Hub 任务失败: %v", err)
	}
	s.jobs = make(map[string]*ControlJob, len(items))
	for i := range items {
		job := items[i]
		if job.Status == "running" {
			job.Status = "pending"
			job.StartedAt = time.Time{}
		}
		s.jobs[job.ID] = &job
	}
	return nil
}

func (s *controlJobStore) saveLocked() error {
	items := make([]ControlJob, 0, len(s.jobs))
	for _, job := range s.jobs {
		items = append(items, *job)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.Before(items[j].CreatedAt) })
	data, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return fmt.Errorf("编码 Hub 任务失败: %v", err)
	}
	if err := os.MkdirAll("configs", 0755); err != nil {
		return fmt.Errorf("创建配置目录失败: %v", err)
	}
	if err := os.WriteFile(controlJobsFile, data, 0600); err != nil {
		return fmt.Errorf("保存 Hub 任务失败: %v", err)
	}
	return nil
}

func (s *controlJobStore) pruneLocked() {
	if len(s.jobs) <= maxStoredControlJobs {
		return
	}
	items := make([]*ControlJob, 0, len(s.jobs))
	for _, job := range s.jobs {
		items = append(items, job)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.Before(items[j].CreatedAt) })
	removeCount := len(items) - maxStoredControlJobs
	for _, job := range items {
		if removeCount == 0 {
			break
		}
		if job.Status == "pending" || job.Status == "running" {
			continue
		}
		delete(s.jobs, job.ID)
		removeCount--
	}
}

func newControlJobID() (string, error) {
	raw := make([]byte, 6)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("生成任务 ID 失败: %v", err)
	}
	return "job-" + hex.EncodeToString(raw), nil
}

func truncateControlText(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max] + "\n...（输出已截断）"
}
