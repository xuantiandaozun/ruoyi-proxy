package cli

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"ruoyi-proxy/internal/scheduler"
	"ruoyi-proxy/internal/taskruntime"
)

func (c *CLI) startTaskScheduler() {
	ctx, cancel := context.WithCancel(context.Background())
	c.schedulerCancel = cancel
	go func() {
		logf := func(format string, args ...interface{}) {
			c.printAsyncTaskNotice(fmt.Sprintf(format, args...))
		}
		if err := taskruntime.Run(ctx, logf); err != nil && ctx.Err() == nil {
			c.printAsyncTaskNotice(fmt.Sprintf("AI 定时任务调度器退出: %v", err))
		}
	}()
}

func (c *CLI) printAsyncTaskNotice(message string) {
	if c.rl == nil {
		return
	}
	// 后台消息先清理当前绘制，再输出并恢复提示符，避免覆盖 You: 输入行。
	c.rl.Clean()
	_, _ = fmt.Fprintf(c.rl.Stdout(), "\n\033[1;36m[定时任务]\033[0m %s\n", message)
	c.rl.Refresh()
}

func (c *CLI) listScheduledTasks() {
	store, err := scheduler.Open()
	if err != nil {
		c.printError(err.Error())
		return
	}
	defer store.Close()
	tasks, err := store.ListTasks()
	if err != nil {
		c.printError(err.Error())
		return
	}
	if len(tasks) == 0 {
		c.printInfo("尚未创建 AI 定时任务，可使用 /task-add 或直接告诉 AI")
		return
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\t状态\t权限\t名称\t调度\t下次执行\t上次结果")
	for _, task := range tasks {
		state := "暂停"
		if task.Enabled {
			state = "启用"
		}
		permission := "只读"
		if task.AllowWrite {
			permission = "可写"
		}
		next := "-"
		if task.NextRunAt != nil {
			next = task.NextRunAt.Local().Format("01-02 15:04")
		}
		last := task.LastStatus
		if last == "" {
			last = "-"
		}
		fmt.Fprintf(w, "%d\t%s\t%s\t%s[%s]\t%s\t%s\t%s\n",
			task.ID, state, permission, task.Name, task.ServiceID, task.Schedule, next, last)
	}
	_ = w.Flush()
	c.printInfo("任务由当前运行的 ruoyi-proxy CLI 或代理进程调度；数据位于独立的 configs/scheduler.db")
}

func (c *CLI) addScheduledTaskInteractive() {
	name, err := c.readLineWithPrompt("任务名称: ")
	if err != nil || strings.TrimSpace(name) == "" {
		return
	}
	prompt, err := c.readLineWithPrompt("每次交给 AI 的完整指令: ")
	if err != nil || strings.TrimSpace(prompt) == "" {
		return
	}
	scheduleSpec, err := c.readLineWithPrompt("调度（如 0 2 * * *、@every 30m、@at RFC3339）: ")
	if err != nil || strings.TrimSpace(scheduleSpec) == "" {
		return
	}
	timezone, err := c.readLineWithPrompt("时区 [Local，或 Asia/Shanghai]: ")
	if err != nil {
		return
	}
	if strings.TrimSpace(timezone) == "" {
		timezone = "Local"
	}
	permission, ok := c.selectSimpleMenu("后台任务权限", []string{
		"只读（推荐；写工具会被拦截）",
		"允许写操作（无人值守执行）",
	}, 0)
	if !ok {
		return
	}
	allowWrite := permission == 1
	if allowWrite && !c.confirmDangerAction("预授权定时任务执行写操作", []string{
		"任务触发时不会再次弹窗确认。",
		"AI 可在任务指令范围内修改文件、服务、系统或远程数据库。",
	}) {
		return
	}
	store, err := scheduler.Open()
	if err != nil {
		c.printError(err.Error())
		return
	}
	defer store.Close()
	task, err := store.SaveTask(scheduler.TaskInput{
		Name: name, ServiceID: c.currentService, Prompt: prompt, Schedule: scheduleSpec, Timezone: timezone,
		AllowWrite: allowWrite, Enabled: true, TimeoutSecs: 600,
	})
	if err != nil {
		c.printError(err.Error())
		return
	}
	c.printSuccess(fmt.Sprintf("已创建任务 [%d] %s，下次执行 %s",
		task.ID, task.Name, task.NextRunAt.Local().Format(time.DateTime)))
}

func (c *CLI) scheduledTaskAction(action string, args []string) {
	if len(args) == 0 {
		c.printError(fmt.Sprintf("用法: /task-%s <任务ID>", action))
		return
	}
	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil || id <= 0 {
		c.printError("任务 ID 必须是正整数")
		return
	}
	store, err := scheduler.Open()
	if err != nil {
		c.printError(err.Error())
		return
	}
	defer store.Close()
	switch action {
	case "enable", "disable":
		task, err := store.SetEnabled(id, action == "enable")
		if err != nil {
			c.printError(err.Error())
			return
		}
		c.printSuccess(fmt.Sprintf("任务 [%d] %s 已%s", id, task.Name, map[bool]string{true: "启用", false: "暂停"}[task.Enabled]))
	case "run":
		if err := store.QueueNow(id); err != nil {
			c.printError(err.Error())
			return
		}
		c.printSuccess(fmt.Sprintf("任务 %d 已进入立即执行队列", id))
	case "delete":
		task, err := store.GetTask(id)
		if err != nil {
			c.printError(err.Error())
			return
		}
		if !c.confirmDangerAction("删除定时任务", []string{fmt.Sprintf("将删除 [%d] %s；执行历史会保留。", id, task.Name)}) {
			return
		}
		if err := store.DeleteTask(id); err != nil {
			c.printError(err.Error())
			return
		}
		c.printSuccess("定时任务已删除，执行历史已保留")
	}
}

func (c *CLI) scheduledTaskHistory(args []string) {
	var taskID int64
	if len(args) > 0 {
		value, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			c.printError("任务 ID 必须是整数")
			return
		}
		taskID = value
	}
	store, err := scheduler.Open()
	if err != nil {
		c.printError(err.Error())
		return
	}
	defer store.Close()
	runs, err := store.ListRuns(taskID, 20)
	if err != nil {
		c.printError(err.Error())
		return
	}
	if len(runs) == 0 {
		c.printInfo("暂无执行历史")
		return
	}
	for _, run := range runs {
		fmt.Printf("\n[%d] %s  %s  %s\n", run.ID, run.TaskName, run.Status, run.StartedAt.Local().Format(time.DateTime))
		if run.Error != "" {
			fmt.Printf("  错误: %s\n", run.Error)
		} else if strings.TrimSpace(run.Output) != "" {
			fmt.Printf("  %s\n", strings.ReplaceAll(strings.TrimSpace(run.Output), "\n", "\n  "))
		}
	}
}
