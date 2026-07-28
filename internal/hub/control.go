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
const defaultControlMaxAttempts = 3
const controlLeaseGrace = 30 * time.Second

const (
	// ControlJobPending 表示任务等待 Spoke 领取。
	ControlJobPending = "pending"
	// ControlJobClaimed 表示任务已被 Spoke 领取但尚未确认开始。
	ControlJobClaimed = "claimed"
	// ControlJobRunning 表示任务正在 Spoke 执行。
	ControlJobRunning = "running"
	// ControlJobSucceeded 表示任务执行成功。
	ControlJobSucceeded = "succeeded"
	// ControlJobFailed 表示任务执行失败。
	ControlJobFailed = "failed"
	// ControlJobTimedOut 表示任务执行超时。
	ControlJobTimedOut = "timed_out"
	// ControlJobCanceled 表示任务在执行前被取消。
	ControlJobCanceled = "canceled"
)

// ControlJob Hub 下发给 Spoke 的远程任务。
type ControlJob struct {
	ID             string            `json:"id"`
	BatchID        string            `json:"batch_id,omitempty"`
	SpokeID        string            `json:"spoke_id"`
	Command        string            `json:"command"`
	Action         *ControlAction    `json:"action,omitempty"`
	WorkDir        string            `json:"workdir,omitempty"`
	TimeoutSecs    int               `json:"timeout_seconds"`
	Status         string            `json:"status"`
	Output         string            `json:"output,omitempty"`
	Error          string            `json:"error,omitempty"`
	IdempotencyKey string            `json:"idempotency_key,omitempty"`
	Attempt        int               `json:"attempt,omitempty"`
	MaxAttempts    int               `json:"max_attempts,omitempty"`
	CreatedAt      time.Time         `json:"created_at"`
	ClaimedUntil   time.Time         `json:"claimed_until,omitempty"`
	StartedAt      time.Time         `json:"started_at,omitempty"`
	FinishedAt     time.Time         `json:"finished_at,omitempty"`
	ConfirmedBy    string            `json:"confirmed_by,omitempty"`
	ConfirmedAt    time.Time         `json:"confirmed_at,omitempty"`
	Source         string            `json:"source,omitempty"`
	Events         []ControlJobEvent `json:"events,omitempty"`
}

// ControlJobOptions 创建远程任务时的可选参数。
type ControlJobOptions struct {
	IdempotencyKey string
	MaxAttempts    int
	Action         *ControlAction
	BatchID        string
	Actor          string
	Source         string
	ConfirmedBy    string
	ConfirmedAt    time.Time
}

type controlJobStore struct {
	mu     sync.RWMutex
	loaded bool
	jobs   map[string]*ControlJob
}

var defaultControlStore = &controlJobStore{jobs: make(map[string]*ControlJob)}

// EnqueueControlJob 创建等待 Spoke 执行的远程任务。
func EnqueueControlJob(spokeID, command, workDir string, timeoutSecs int) (ControlJob, error) {
	return EnqueueControlJobWithOptions(spokeID, command, workDir, timeoutSecs, ControlJobOptions{})
}

// EnqueueControlJobWithOptions 创建支持幂等键和最大尝试次数的远程任务。
func EnqueueControlJobWithOptions(spokeID, command, workDir string, timeoutSecs int, options ControlJobOptions) (ControlJob, error) {
	spokeID = strings.TrimSpace(spokeID)
	command = strings.TrimSpace(command)
	if spokeID == "" {
		return ControlJob{}, fmt.Errorf("spoke ID 不能为空")
	}
	if options.Action == nil && command == "" {
		return ControlJob{}, fmt.Errorf("命令或结构化动作不能为空")
	}
	if options.Action != nil {
		actionCopy := cloneControlAction(options.Action)
		options.Action = actionCopy
		if err := options.Action.Validate(); err != nil {
			return ControlJob{}, err
		}
		if options.Action.Type == ControlActionShell {
			command = strings.TrimSpace(options.Action.Params["command"])
		}
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
	if spoke.Maintenance {
		return ControlJob{}, fmt.Errorf("spoke 处于维护状态: %s", spokeID)
	}
	if options.Action != nil {
		required := options.Action.RequiredCapability()
		if required != "" && !SpokeAllowsCapability(spoke, required) {
			return ControlJob{}, fmt.Errorf("spoke 缺少或未获准能力 %s: %s", required, spokeID)
		}
	}
	if timeoutSecs <= 0 {
		timeoutSecs = 60
	}
	if timeoutSecs > 300 {
		timeoutSecs = 300
	}
	idempotencyKey := strings.TrimSpace(options.IdempotencyKey)
	if len(idempotencyKey) > 128 {
		return ControlJob{}, fmt.Errorf("幂等键不能超过 128 字符")
	}
	maxAttempts := options.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = defaultControlMaxAttempts
	}
	if maxAttempts > 10 {
		return ControlJob{}, fmt.Errorf("最大尝试次数不能超过 10")
	}

	defaultControlStore.mu.Lock()
	defer defaultControlStore.mu.Unlock()
	if err := defaultControlStore.loadLocked(); err != nil {
		return ControlJob{}, err
	}
	if idempotencyKey != "" {
		for _, existing := range defaultControlStore.jobs {
			if existing.SpokeID == spokeID && existing.IdempotencyKey == idempotencyKey {
				if existing.Command != command ||
					!equalControlActions(existing.Action, options.Action) ||
					existing.WorkDir != strings.TrimSpace(workDir) ||
					existing.TimeoutSecs != timeoutSecs {
					return ControlJob{}, fmt.Errorf("幂等键已被不同任务使用: %s", idempotencyKey)
				}
				return cloneControlJob(existing), nil
			}
		}
	}
	id, err := newControlJobID()
	if err != nil {
		return ControlJob{}, err
	}
	now := time.Now()
	confirmedAt := options.ConfirmedAt
	if strings.TrimSpace(options.ConfirmedBy) != "" && confirmedAt.IsZero() {
		confirmedAt = now
	}
	job := ControlJob{
		ID:             id,
		BatchID:        strings.TrimSpace(options.BatchID),
		SpokeID:        spokeID,
		Command:        command,
		Action:         cloneControlAction(options.Action),
		WorkDir:        strings.TrimSpace(workDir),
		TimeoutSecs:    timeoutSecs,
		Status:         ControlJobPending,
		IdempotencyKey: idempotencyKey,
		MaxAttempts:    maxAttempts,
		CreatedAt:      now,
		ConfirmedBy:    strings.TrimSpace(options.ConfirmedBy),
		ConfirmedAt:    confirmedAt,
		Source:         strings.TrimSpace(options.Source),
	}
	summary := command
	if job.Action != nil {
		summary = job.Action.Summary()
	}
	if job.ConfirmedBy != "" {
		appendControlEvent(&job, "confirmed", "", "", job.ConfirmedBy, options.Source, summary, job.ConfirmedAt)
	}
	appendControlEvent(&job, "created", "", ControlJobPending, options.Actor, options.Source, summary, now)
	defaultControlStore.jobs[job.ID] = &job
	defaultControlStore.pruneLocked()
	if err := defaultControlStore.saveLocked(); err != nil {
		delete(defaultControlStore.jobs, job.ID)
		return ControlJob{}, err
	}
	return job, nil
}

// ClaimControlJob 以旧版协议领取指定 Spoke 最早的待执行 Shell 任务。
func ClaimControlJob(spokeID string) (*ControlJob, error) {
	return ClaimControlJobWithVersion(spokeID, 1)
}

// ClaimControlJobWithVersion 按 Worker 支持的控制协议版本领取任务。
func ClaimControlJobWithVersion(spokeID string, protocolVersion int) (*ControlJob, error) {
	return ClaimControlJobWithCapabilities(spokeID, protocolVersion, nil)
}

// ClaimControlJobWithCapabilities 按节点实时能力和 Hub 治理规则领取任务。
func ClaimControlJobWithCapabilities(spokeID string, protocolVersion int, capabilities []string) (*ControlJob, error) {
	record, ok := GetSpoke(spokeID)
	if ok && record.Maintenance {
		return nil, nil
	}
	if len(capabilities) == 0 && record.Profile != nil {
		capabilities = record.Profile.Capabilities
	}
	controlBatchMu.Lock()
	defer controlBatchMu.Unlock()
	defaultControlStore.mu.Lock()
	defer defaultControlStore.mu.Unlock()
	if err := defaultControlStore.loadLocked(); err != nil {
		return nil, err
	}
	now := time.Now()
	changed := defaultControlStore.recoverExpiredLocked(now)
	var selected *ControlJob
	for _, job := range defaultControlStore.jobs {
		if job.SpokeID != spokeID || job.Status != ControlJobPending {
			continue
		}
		if job.Action != nil {
			if protocolVersion < 2 {
				continue
			}
			required := job.Action.RequiredCapability()
			if required != "" && !capabilityAllowed(record, capabilities, required) {
				continue
			}
		} else if len(capabilities) > 0 && !capabilityAllowed(record, capabilities, "shell") {
			continue
		}
		if selected == nil || job.CreatedAt.Before(selected.CreatedAt) {
			selected = job
		}
	}
	if selected == nil {
		if changed {
			if err := defaultControlStore.saveLocked(); err != nil {
				return nil, err
			}
		}
		return nil, nil
	}
	selected.Status = ControlJobClaimed
	selected.Attempt++
	selected.ClaimedUntil = now.Add(time.Duration(selected.TimeoutSecs)*time.Second + controlLeaseGrace)
	appendControlEvent(selected, "claimed", ControlJobPending, ControlJobClaimed, "spoke:"+spokeID, "spoke_poll", fmt.Sprintf("第 %d 次领取", selected.Attempt), now)
	if err := defaultControlStore.saveLocked(); err != nil {
		return nil, err
	}
	copyJob := cloneControlJob(selected)
	return &copyJob, nil
}

// StartControlJob 确认 Spoke 已开始执行任务。
func StartControlJob(spokeID, jobID string) error {
	defaultControlStore.mu.Lock()
	defer defaultControlStore.mu.Unlock()
	if err := defaultControlStore.loadLocked(); err != nil {
		return err
	}
	job, err := defaultControlStore.ownedJobLocked(spokeID, jobID)
	if err != nil {
		return err
	}
	if job.Status == ControlJobRunning {
		return nil
	}
	if job.Status != ControlJobClaimed {
		return fmt.Errorf("任务当前不可开始: %s", job.Status)
	}
	now := time.Now()
	job.Status = ControlJobRunning
	job.StartedAt = now
	appendControlEvent(job, "started", ControlJobClaimed, ControlJobRunning, "spoke:"+spokeID, "spoke_result", "开始执行", now)
	return defaultControlStore.saveLocked()
}

// CompleteControlJob 保存 Spoke 返回的执行结果。
func CompleteControlJob(spokeID, jobID, status, output, errorText string) error {
	if !isControlTerminalResult(status) {
		return fmt.Errorf("无效任务状态: %s", status)
	}
	defaultControlStore.mu.Lock()
	defer defaultControlStore.mu.Unlock()
	if err := defaultControlStore.loadLocked(); err != nil {
		return err
	}
	job, err := defaultControlStore.ownedJobLocked(spokeID, jobID)
	if err != nil {
		return err
	}
	if isControlTerminal(job.Status) {
		if job.Status == status {
			return nil
		}
		return fmt.Errorf("任务已结束: %s", job.Status)
	}
	// 兼容旧 Spoke：旧版本领取后不会单独上报 running，可从 claimed 直接完成。
	if job.Status != ControlJobClaimed && job.Status != ControlJobRunning {
		return fmt.Errorf("任务当前不可完成: %s", job.Status)
	}
	from := job.Status
	now := time.Now()
	job.Status = status
	job.Output = truncateControlText(output, 64*1024)
	job.Error = truncateControlText(errorText, 4096)
	job.ClaimedUntil = time.Time{}
	job.FinishedAt = now
	appendControlEvent(job, "completed", from, status, "spoke:"+spokeID, "spoke_result", truncateControlText(errorText, 200), now)
	return defaultControlStore.saveLocked()
}

// CancelControlJob 取消尚未被 Spoke 领取的任务。
func CancelControlJob(jobID string) (ControlJob, error) {
	defaultControlStore.mu.Lock()
	defer defaultControlStore.mu.Unlock()
	if err := defaultControlStore.loadLocked(); err != nil {
		return ControlJob{}, err
	}
	job, ok := defaultControlStore.jobs[strings.TrimSpace(jobID)]
	if !ok {
		return ControlJob{}, fmt.Errorf("任务不存在: %s", jobID)
	}
	if job.Status != ControlJobPending {
		return ControlJob{}, fmt.Errorf("仅等待中的任务可取消，当前状态: %s", job.Status)
	}
	now := time.Now()
	job.Status = ControlJobCanceled
	job.Error = "任务已由 Hub 管理员取消"
	job.FinishedAt = now
	appendControlEvent(job, "canceled", ControlJobPending, ControlJobCanceled, "hub-admin", "management_api", job.Error, now)
	if err := defaultControlStore.saveLocked(); err != nil {
		return ControlJob{}, err
	}
	return cloneControlJob(job), nil
}

// RetryControlJob 将失败、超时或取消的任务重新放回队列。
func RetryControlJob(jobID string) (ControlJob, error) {
	defaultControlStore.mu.Lock()
	defer defaultControlStore.mu.Unlock()
	if err := defaultControlStore.loadLocked(); err != nil {
		return ControlJob{}, err
	}
	job, ok := defaultControlStore.jobs[strings.TrimSpace(jobID)]
	if !ok {
		return ControlJob{}, fmt.Errorf("任务不存在: %s", jobID)
	}
	if job.Status != ControlJobFailed && job.Status != ControlJobTimedOut && job.Status != ControlJobCanceled {
		return ControlJob{}, fmt.Errorf("仅失败、超时或已取消任务可重试，当前状态: %s", job.Status)
	}
	if job.Attempt >= job.MaxAttempts {
		job.MaxAttempts = job.Attempt + 1
	}
	from := job.Status
	job.Status = ControlJobPending
	job.Output = ""
	job.Error = ""
	job.ClaimedUntil = time.Time{}
	job.StartedAt = time.Time{}
	job.FinishedAt = time.Time{}
	appendControlEvent(job, "retried", from, ControlJobPending, "hub-admin", "management_api", "管理员请求重试", time.Now())
	if err := defaultControlStore.saveLocked(); err != nil {
		return ControlJob{}, err
	}
	return cloneControlJob(job), nil
}

// ListControlJobs 返回远程任务，spokeID 为空时返回全部。
func ListControlJobs(spokeID string, limit int) ([]ControlJob, error) {
	defaultControlStore.mu.Lock()
	defer defaultControlStore.mu.Unlock()
	if err := defaultControlStore.loadLocked(); err != nil {
		return nil, err
	}
	if defaultControlStore.recoverExpiredLocked(time.Now()) {
		if err := defaultControlStore.saveLocked(); err != nil {
			return nil, err
		}
	}
	items := make([]ControlJob, 0, len(defaultControlStore.jobs))
	for _, job := range defaultControlStore.jobs {
		if spokeID == "" || job.SpokeID == spokeID {
			items = append(items, cloneControlJob(job))
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

func capabilityAllowed(record SpokeRecord, capabilities []string, required string) bool {
	if !containsGovernanceValue(capabilities, required) {
		return false
	}
	return len(record.AllowedCapabilities) == 0 || containsGovernanceValue(record.AllowedCapabilities, required)
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
		if job.MaxAttempts <= 0 {
			job.MaxAttempts = defaultControlMaxAttempts
		}
		// 兼容旧任务文件：旧 running 没有租约，重启后回到等待队列。
		if job.Status == ControlJobRunning && job.ClaimedUntil.IsZero() {
			job.Status = ControlJobPending
			job.StartedAt = time.Time{}
		}
		if len(job.Events) == 0 {
			summary := job.Command
			if job.Action != nil {
				summary = job.Action.Summary()
			}
			appendControlEvent(&job, "imported", job.Status, job.Status, "hub", "migration", summary, job.CreatedAt)
		}
		s.jobs[job.ID] = &job
	}
	return nil
}

func (s *controlJobStore) saveLocked() error {
	items := make([]ControlJob, 0, len(s.jobs))
	for _, job := range s.jobs {
		items = append(items, cloneControlJob(job))
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

func (s *controlJobStore) ownedJobLocked(spokeID, jobID string) (*ControlJob, error) {
	job, ok := s.jobs[strings.TrimSpace(jobID)]
	if !ok || job.SpokeID != strings.TrimSpace(spokeID) {
		return nil, fmt.Errorf("任务不存在或不属于当前 spoke: %s", jobID)
	}
	return job, nil
}

func (s *controlJobStore) recoverExpiredLocked(now time.Time) bool {
	changed := false
	for _, job := range s.jobs {
		if (job.Status != ControlJobClaimed && job.Status != ControlJobRunning) ||
			job.ClaimedUntil.IsZero() || now.Before(job.ClaimedUntil) {
			continue
		}
		changed = true
		job.ClaimedUntil = time.Time{}
		job.StartedAt = time.Time{}
		from := job.Status
		if job.Attempt < job.MaxAttempts {
			job.Status = ControlJobPending
			job.Error = "上次执行租约已过期，等待重新领取"
			appendControlEvent(job, "lease_expired", from, ControlJobPending, "hub", "lease_recovery", job.Error, now)
			continue
		}
		job.Status = ControlJobFailed
		job.Error = "执行租约已过期且达到最大尝试次数"
		job.FinishedAt = now
		appendControlEvent(job, "lease_exhausted", from, ControlJobFailed, "hub", "lease_recovery", job.Error, now)
	}
	return changed
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
		if !isControlTerminal(job.Status) {
			continue
		}
		delete(s.jobs, job.ID)
		removeCount--
	}
}

func isControlTerminalResult(status string) bool {
	return status == ControlJobSucceeded || status == ControlJobFailed || status == ControlJobTimedOut
}

func isControlTerminal(status string) bool {
	return isControlTerminalResult(status) || status == ControlJobCanceled
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
