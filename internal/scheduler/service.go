package scheduler

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// Executor 执行一条已领取的 AI 定时任务。
type Executor func(ctx context.Context, task Task) (string, error)

// LogFunc 接收调度器后台事件。
type LogFunc func(format string, args ...interface{})

// Service 轮询 SQLite 并执行到期任务。
type Service struct {
	store      *Store
	executor   Executor
	interval   time.Duration
	maxWorkers int
	workers    chan struct{}
	wg         sync.WaitGroup
	logf       LogFunc
}

// NewService 创建调度服务。
func NewService(store *Store, executor Executor) *Service {
	return &Service{
		store: store, executor: executor,
		interval: time.Second, maxWorkers: 2,
		workers: make(chan struct{}, 2), logf: log.Printf,
	}
}

// SetLogger 设置后台事件输出；传 nil 可完全静默。
func (s *Service) SetLogger(logf LogFunc) {
	s.logf = logf
}

func (s *Service) writeLog(format string, args ...interface{}) {
	if s.logf != nil {
		s.logf(format, args...)
	}
}

// Run 启动调度循环，直到上下文取消。
func (s *Service) Run(ctx context.Context) error {
	if s.store == nil || s.executor == nil {
		return fmt.Errorf("调度存储和执行器不能为空")
	}
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	s.dispatch(ctx)
	for {
		select {
		case <-ctx.Done():
			s.wg.Wait()
			return nil
		case <-ticker.C:
			s.dispatch(ctx)
		}
	}
}

func (s *Service) dispatch(ctx context.Context) {
	for len(s.workers) < s.maxWorkers {
		task, err := s.store.ClaimDue(ctx, time.Now())
		if err != nil {
			if ctx.Err() == nil {
				s.writeLog("领取 AI 定时任务失败: %v", err)
			}
			return
		}
		if task == nil {
			return
		}
		s.workers <- struct{}{}
		s.wg.Add(1)
		go s.execute(ctx, *task)
	}
}

func (s *Service) execute(parent context.Context, task Task) {
	defer func() {
		<-s.workers
		s.wg.Done()
	}()
	run, err := s.store.StartRun(task, "schedule")
	if err != nil {
		s.writeLog("创建 AI 定时任务执行记录失败: %v", err)
		return
	}
	timeout := time.Duration(task.TimeoutSecs) * time.Second
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	output, runErr := s.executor(ctx, task)
	if ctx.Err() == context.DeadlineExceeded {
		runErr = fmt.Errorf("任务执行超时（%s）", timeout)
	}
	if err := s.store.FinishRun(task, run.ID, output, runErr); err != nil {
		s.writeLog("保存 AI 定时任务结果失败: %v", err)
		return
	}
	if runErr != nil {
		s.writeLog("AI 定时任务[%d:%s]执行失败: %v", task.ID, task.Name, runErr)
		return
	}
	s.writeLog("AI 定时任务[%d:%s]执行成功", task.ID, task.Name)
}
