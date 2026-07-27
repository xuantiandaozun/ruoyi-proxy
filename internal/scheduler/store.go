package scheduler

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
	_ "modernc.org/sqlite"
)

const (
	defaultDatabasePath = "configs/scheduler.db"
	defaultTimeout      = 600
	maxTimeout          = 3600
)

var cronParser = cron.NewParser(
	cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
)

// Task AI 定时任务。
type Task struct {
	ID          int64      `json:"id"`
	Name        string     `json:"name"`
	ServiceID   string     `json:"service_id"`
	Prompt      string     `json:"prompt"`
	Schedule    string     `json:"schedule"`
	Timezone    string     `json:"timezone"`
	AllowWrite  bool       `json:"allow_write"`
	Enabled     bool       `json:"enabled"`
	TimeoutSecs int        `json:"timeout_seconds"`
	NextRunAt   *time.Time `json:"next_run_at,omitempty"`
	LastRunAt   *time.Time `json:"last_run_at,omitempty"`
	LastStatus  string     `json:"last_status,omitempty"`
	LastError   string     `json:"last_error,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// Run 定时任务的一次执行记录。
type Run struct {
	ID         int64      `json:"id"`
	TaskID     int64      `json:"task_id"`
	TaskName   string     `json:"task_name"`
	Status     string     `json:"status"`
	Trigger    string     `json:"trigger"`
	Output     string     `json:"output,omitempty"`
	Error      string     `json:"error,omitempty"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

// TaskInput 新建或更新任务时由用户提供的字段。
type TaskInput struct {
	ID          int64  `json:"id,omitempty"`
	Name        string `json:"name"`
	ServiceID   string `json:"service_id,omitempty"`
	Prompt      string `json:"prompt"`
	Schedule    string `json:"schedule"`
	Timezone    string `json:"timezone,omitempty"`
	AllowWrite  bool   `json:"allow_write"`
	Enabled     bool   `json:"enabled"`
	TimeoutSecs int    `json:"timeout_seconds,omitempty"`
}

// Store 使用独立本地 SQLite 保存任务与执行审计。
type Store struct {
	db   *sql.DB
	path string
}

// Open 打开默认的本地调度数据库。
func Open() (*Store, error) {
	return OpenPath(defaultDatabasePath)
}

// OpenPath 打开指定路径的调度数据库，主要用于测试和迁移。
func OpenPath(path string) (*Store, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		path = defaultDatabasePath
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("创建调度数据库目录失败: %v", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("打开调度数据库失败: %v", err)
	}
	db.SetMaxOpenConns(1)
	store := &Store{db: db, path: path}
	if err := store.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	// 指令和执行结果可能包含敏感运维信息，仅允许当前系统用户读取。
	if err := os.Chmod(path, 0600); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("设置调度数据库权限失败: %v", err)
	}
	return store, nil
}

// Close 关闭调度数据库。
func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	statements := []string{
		`PRAGMA journal_mode=WAL`,
		`PRAGMA busy_timeout=5000`,
		`CREATE TABLE IF NOT EXISTS scheduled_tasks (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			service_id TEXT NOT NULL DEFAULT 'default',
			prompt TEXT NOT NULL,
			schedule TEXT NOT NULL,
			timezone TEXT NOT NULL DEFAULT 'Local',
			allow_write INTEGER NOT NULL DEFAULT 0,
			enabled INTEGER NOT NULL DEFAULT 1,
			timeout_seconds INTEGER NOT NULL DEFAULT 600,
			next_run_at TEXT,
			last_run_at TEXT,
			last_status TEXT NOT NULL DEFAULT '',
			last_error TEXT NOT NULL DEFAULT '',
			locked_until TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS scheduled_task_runs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			task_id INTEGER NOT NULL,
			task_name TEXT NOT NULL,
			status TEXT NOT NULL,
			trigger_type TEXT NOT NULL,
			output TEXT NOT NULL DEFAULT '',
			error TEXT NOT NULL DEFAULT '',
			started_at TEXT NOT NULL,
			finished_at TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_scheduled_tasks_due
			ON scheduled_tasks(enabled, next_run_at, locked_until)`,
		`CREATE INDEX IF NOT EXISTS idx_scheduled_task_runs_task
			ON scheduled_task_runs(task_id, started_at DESC)`,
	}
	for _, statement := range statements {
		if _, err := s.db.Exec(statement); err != nil {
			return fmt.Errorf("初始化调度数据库失败: %v", err)
		}
	}
	if err := s.ensureColumn("scheduled_tasks", "service_id", "TEXT NOT NULL DEFAULT 'default'"); err != nil {
		return err
	}
	// 只恢复明显超过最大执行时长的孤儿记录，避免另一个进程正在执行时被误判。
	staleBefore := formatTime(time.Now().Add(-time.Duration(maxTimeout+300) * time.Second))
	if _, err := s.db.Exec(
		`UPDATE scheduled_task_runs
		 SET status='failed', error='执行进程异常退出', finished_at=?
		 WHERE status='running' AND started_at<?`,
		formatTime(time.Now()), staleBefore,
	); err != nil {
		return fmt.Errorf("恢复中断任务记录失败: %v", err)
	}
	return nil
}

// SaveTask 新建或更新任务。
func (s *Store) SaveTask(input TaskInput) (Task, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.ServiceID = strings.TrimSpace(input.ServiceID)
	input.Prompt = strings.TrimSpace(input.Prompt)
	input.Schedule = strings.TrimSpace(input.Schedule)
	input.Timezone = strings.TrimSpace(input.Timezone)
	if input.Name == "" {
		return Task{}, fmt.Errorf("任务名称不能为空")
	}
	if input.Prompt == "" {
		return Task{}, fmt.Errorf("AI 任务指令不能为空")
	}
	if input.ServiceID == "" {
		input.ServiceID = "default"
	}
	if input.Timezone == "" {
		input.Timezone = "Local"
	}
	if input.TimeoutSecs <= 0 {
		input.TimeoutSecs = defaultTimeout
	}
	if input.TimeoutSecs > maxTimeout {
		return Task{}, fmt.Errorf("任务超时不能超过 %d 秒", maxTimeout)
	}
	next, err := NextTime(input.Schedule, input.Timezone, time.Now())
	if err != nil {
		return Task{}, err
	}
	now := time.Now()
	if input.ID == 0 {
		result, err := s.db.Exec(
			`INSERT INTO scheduled_tasks
			 (name, service_id, prompt, schedule, timezone, allow_write, enabled, timeout_seconds,
			  next_run_at, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			input.Name, input.ServiceID, input.Prompt, input.Schedule, input.Timezone, boolInt(input.AllowWrite),
			boolInt(input.Enabled), input.TimeoutSecs, nullableTime(next), formatTime(now), formatTime(now),
		)
		if err != nil {
			return Task{}, fmt.Errorf("创建定时任务失败: %v", err)
		}
		input.ID, _ = result.LastInsertId()
	} else {
		result, err := s.db.Exec(
			`UPDATE scheduled_tasks
			 SET name=?, service_id=?, prompt=?, schedule=?, timezone=?, allow_write=?, enabled=?,
			     timeout_seconds=?, next_run_at=?, locked_until=NULL, updated_at=?
			 WHERE id=?`,
			input.Name, input.ServiceID, input.Prompt, input.Schedule, input.Timezone, boolInt(input.AllowWrite),
			boolInt(input.Enabled), input.TimeoutSecs, nullableTime(next), formatTime(now), input.ID,
		)
		if err != nil {
			return Task{}, fmt.Errorf("更新定时任务失败: %v", err)
		}
		affected, _ := result.RowsAffected()
		if affected == 0 {
			return Task{}, fmt.Errorf("定时任务不存在: %d", input.ID)
		}
	}
	return s.GetTask(input.ID)
}

// ListTasks 列出所有任务。
func (s *Store) ListTasks() ([]Task, error) {
	rows, err := s.db.Query(
		`SELECT id, name, service_id, prompt, schedule, timezone, allow_write, enabled,
		        timeout_seconds, next_run_at, last_run_at, last_status, last_error,
		        created_at, updated_at
		 FROM scheduled_tasks ORDER BY id`,
	)
	if err != nil {
		return nil, fmt.Errorf("读取定时任务失败: %v", err)
	}
	defer rows.Close()
	var tasks []Task
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

// GetTask 按 ID 获取任务。
func (s *Store) GetTask(id int64) (Task, error) {
	row := s.db.QueryRow(
		`SELECT id, name, service_id, prompt, schedule, timezone, allow_write, enabled,
		        timeout_seconds, next_run_at, last_run_at, last_status, last_error,
		        created_at, updated_at
		 FROM scheduled_tasks WHERE id=?`, id,
	)
	task, err := scanTask(row)
	if err == sql.ErrNoRows {
		return Task{}, fmt.Errorf("定时任务不存在: %d", id)
	}
	return task, err
}

// SetEnabled 启用或暂停任务。
func (s *Store) SetEnabled(id int64, enabled bool) (Task, error) {
	task, err := s.GetTask(id)
	if err != nil {
		return Task{}, err
	}
	var next *time.Time
	if enabled {
		value, err := NextTime(task.Schedule, task.Timezone, time.Now())
		if err != nil {
			return Task{}, err
		}
		next = &value
	}
	result, err := s.db.Exec(
		`UPDATE scheduled_tasks SET enabled=?, next_run_at=?, locked_until=NULL, updated_at=? WHERE id=?`,
		boolInt(enabled), nullableTimePtr(next), formatTime(time.Now()), id,
	)
	if err != nil {
		return Task{}, fmt.Errorf("更新任务状态失败: %v", err)
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return Task{}, fmt.Errorf("定时任务不存在: %d", id)
	}
	return s.GetTask(id)
}

// DeleteTask 删除任务，执行历史保留用于审计。
func (s *Store) DeleteTask(id int64) error {
	result, err := s.db.Exec(`DELETE FROM scheduled_tasks WHERE id=?`, id)
	if err != nil {
		return fmt.Errorf("删除定时任务失败: %v", err)
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return fmt.Errorf("定时任务不存在: %d", id)
	}
	return nil
}

// QueueNow 把任务安排为立即执行，原有周期不会被改变。
func (s *Store) QueueNow(id int64) error {
	now := formatTime(time.Now())
	result, err := s.db.Exec(
		`UPDATE scheduled_tasks SET enabled=1, next_run_at=?, locked_until=NULL, updated_at=? WHERE id=?`,
		now, now, id,
	)
	if err != nil {
		return fmt.Errorf("安排立即执行失败: %v", err)
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return fmt.Errorf("定时任务不存在: %d", id)
	}
	return nil
}

// ClaimDue 原子领取一个到期任务，支持 CLI 与代理进程共享同一数据库。
func (s *Store) ClaimDue(ctx context.Context, now time.Time) (*Task, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("开始领取任务事务失败: %v", err)
	}
	defer tx.Rollback()
	var id int64
	err = tx.QueryRowContext(ctx,
		`SELECT id FROM scheduled_tasks
		 WHERE enabled=1 AND next_run_at IS NOT NULL AND next_run_at<=?
		   AND (locked_until IS NULL OR locked_until<?)
		 ORDER BY next_run_at, id LIMIT 1`,
		formatTime(now), formatTime(now),
	).Scan(&id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("查询到期任务失败: %v", err)
	}
	var timeout int
	if err := tx.QueryRowContext(ctx, `SELECT timeout_seconds FROM scheduled_tasks WHERE id=?`, id).Scan(&timeout); err != nil {
		return nil, fmt.Errorf("读取任务超时配置失败: %v", err)
	}
	lockUntil := now.Add(time.Duration(timeout+60) * time.Second)
	result, err := tx.ExecContext(ctx,
		`UPDATE scheduled_tasks SET locked_until=?
		 WHERE id=? AND (locked_until IS NULL OR locked_until<?)`,
		formatTime(lockUntil), id, formatTime(now),
	)
	if err != nil {
		return nil, fmt.Errorf("锁定到期任务失败: %v", err)
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return nil, nil
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("提交任务领取失败: %v", err)
	}
	task, err := s.GetTask(id)
	if err != nil {
		return nil, err
	}
	return &task, nil
}

// StartRun 创建运行中的审计记录。
func (s *Store) StartRun(task Task, trigger string) (Run, error) {
	now := time.Now()
	result, err := s.db.Exec(
		`INSERT INTO scheduled_task_runs
		 (task_id, task_name, status, trigger_type, started_at)
		 VALUES (?, ?, 'running', ?, ?)`,
		task.ID, task.Name, trigger, formatTime(now),
	)
	if err != nil {
		return Run{}, fmt.Errorf("创建任务执行记录失败: %v", err)
	}
	id, _ := result.LastInsertId()
	return Run{ID: id, TaskID: task.ID, TaskName: task.Name, Status: "running", Trigger: trigger, StartedAt: now}, nil
}

// FinishRun 完成执行记录，并计算任务的下一次执行时间。
func (s *Store) FinishRun(task Task, runID int64, output string, runErr error) error {
	now := time.Now()
	status := "success"
	errorText := ""
	if runErr != nil {
		status = "failed"
		errorText = runErr.Error()
	}
	if len(output) > 12000 {
		output = output[:12000] + "\n[输出已截断]"
	}
	if len(errorText) > 3000 {
		errorText = errorText[:3000]
	}
	enabled := 1
	var nextValue interface{}
	var nextErr error
	if strings.HasPrefix(strings.TrimSpace(task.Schedule), "@at ") {
		enabled = 0
	} else {
		var next time.Time
		next, nextErr = NextTime(task.Schedule, task.Timezone, now)
		if nextErr == nil {
			nextValue = formatTime(next)
		}
		if nextErr != nil {
			enabled = 0
		}
	}
	if nextErr != nil && errorText == "" {
		errorText = nextErr.Error()
		status = "failed"
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("开始完成任务事务失败: %v", err)
	}
	defer tx.Rollback()
	if _, err := tx.Exec(
		`UPDATE scheduled_task_runs SET status=?, output=?, error=?, finished_at=? WHERE id=?`,
		status, output, errorText, formatTime(now), runID,
	); err != nil {
		return fmt.Errorf("更新任务执行记录失败: %v", err)
	}
	if _, err := tx.Exec(
		`UPDATE scheduled_tasks
		 SET enabled=?, next_run_at=?, last_run_at=?, last_status=?, last_error=?,
		     locked_until=NULL, updated_at=? WHERE id=?`,
		enabled, nextValue, formatTime(now), status, errorText, formatTime(now), task.ID,
	); err != nil {
		return fmt.Errorf("更新任务执行状态失败: %v", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交任务完成状态失败: %v", err)
	}
	return nil
}

// ListRuns 查询最近执行记录，taskID 为 0 时查询全部任务。
func (s *Store) ListRuns(taskID int64, limit int) ([]Run, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	query := `SELECT id, task_id, task_name, status, trigger_type, output, error,
	                 started_at, finished_at FROM scheduled_task_runs`
	args := []interface{}{}
	if taskID > 0 {
		query += ` WHERE task_id=?`
		args = append(args, taskID)
	}
	query += ` ORDER BY id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("读取任务执行历史失败: %v", err)
	}
	defer rows.Close()
	var runs []Run
	for rows.Next() {
		var run Run
		var started string
		var finished sql.NullString
		if err := rows.Scan(&run.ID, &run.TaskID, &run.TaskName, &run.Status, &run.Trigger,
			&run.Output, &run.Error, &started, &finished); err != nil {
			return nil, fmt.Errorf("解析任务执行历史失败: %v", err)
		}
		run.StartedAt, _ = parseTime(started)
		run.FinishedAt = parseNullableTime(finished)
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

// NextTime 校验调度表达式并返回下一次执行时间。
// 支持标准五段 Cron、@hourly、@daily、@every 10m 和 @at RFC3339。
func NextTime(spec, timezone string, after time.Time) (time.Time, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return time.Time{}, fmt.Errorf("调度表达式不能为空")
	}
	location, err := loadLocation(timezone)
	if err != nil {
		return time.Time{}, err
	}
	if strings.HasPrefix(spec, "@at ") {
		value := strings.TrimSpace(strings.TrimPrefix(spec, "@at "))
		at, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return time.Time{}, fmt.Errorf("@at 时间必须使用 RFC3339 格式，例如 @at 2026-08-01T02:00:00+08:00")
		}
		if !at.After(after) {
			return time.Time{}, fmt.Errorf("@at 时间必须晚于当前时间")
		}
		return at, nil
	}
	schedule, err := cronParser.Parse(spec)
	if err != nil {
		return time.Time{}, fmt.Errorf("无效调度表达式: %v", err)
	}
	return schedule.Next(after.In(location)), nil
}

func loadLocation(value string) (*time.Location, error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "local") {
		return time.Local, nil
	}
	location, err := time.LoadLocation(value)
	if err != nil {
		return nil, fmt.Errorf("无效时区 %q: %v", value, err)
	}
	return location, nil
}

type rowScanner interface {
	Scan(dest ...interface{}) error
}

func scanTask(row rowScanner) (Task, error) {
	var task Task
	var allowWrite, enabled int
	var nextRun, lastRun sql.NullString
	var created, updated string
	if err := row.Scan(&task.ID, &task.Name, &task.ServiceID, &task.Prompt, &task.Schedule, &task.Timezone,
		&allowWrite, &enabled, &task.TimeoutSecs, &nextRun, &lastRun,
		&task.LastStatus, &task.LastError, &created, &updated); err != nil {
		return Task{}, err
	}
	task.AllowWrite = allowWrite == 1
	task.Enabled = enabled == 1
	task.NextRunAt = parseNullableTime(nextRun)
	task.LastRunAt = parseNullableTime(lastRun)
	task.CreatedAt, _ = parseTime(created)
	task.UpdatedAt, _ = parseTime(updated)
	return task, nil
}

func (s *Store) ensureColumn(table, column, definition string) error {
	rows, err := s.db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return fmt.Errorf("读取调度数据库结构失败: %v", err)
	}
	found := false
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull, primaryKey int
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			rows.Close()
			return fmt.Errorf("解析调度数据库结构失败: %v", err)
		}
		if name == column {
			found = true
		}
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("关闭调度数据库结构查询失败: %v", err)
	}
	if found {
		return nil
	}
	if _, err := s.db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + column + ` ` + definition); err != nil {
		return fmt.Errorf("升级调度数据库结构失败: %v", err)
	}
	return nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func formatTime(value time.Time) string         { return value.UTC().Format(time.RFC3339Nano) }
func parseTime(value string) (time.Time, error) { return time.Parse(time.RFC3339Nano, value) }

func parseNullableTime(value sql.NullString) *time.Time {
	if !value.Valid || value.String == "" {
		return nil
	}
	parsed, err := parseTime(value.String)
	if err != nil {
		return nil
	}
	return &parsed
}

func nullableTime(value time.Time) interface{} {
	if value.IsZero() {
		return nil
	}
	return formatTime(value)
}

func nullableTimePtr(value *time.Time) interface{} {
	if value == nil || value.IsZero() {
		return nil
	}
	return formatTime(*value)
}
