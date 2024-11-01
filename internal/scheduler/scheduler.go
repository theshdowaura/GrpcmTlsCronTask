// internal/scheduler/scheduler.go

package scheduler

import (
	"context"
	"log"
	"os/exec"

	"github.com/robfig/cron/v3"
)

// Scheduler 包装了 robfig/cron 的 Cron 实例
type Scheduler struct {
	Cron *cron.Cron
}

// NewScheduler 创建一个新的 Scheduler 实例
func NewScheduler() *Scheduler {
	return &Scheduler{
		Cron: cron.New(cron.WithSeconds()),
	}
}

// Start 启动调度器
func (s *Scheduler) Start() {
	s.Cron.Start()
}

// Stop 停止调度器
func (s *Scheduler) Stop() context.Context {
	return s.Cron.Stop()
}

// AddTask 向调度器添加一个新的任务
func (s *Scheduler) AddTask(cronExpr string, taskID int64, command string) (cron.EntryID, error) {
	id, err := s.Cron.AddFunc(cronExpr, func() {
		RunCommand(taskID, command)
	})
	if err != nil {
		return 0, err
	}
	return id, nil
}

// RunCommand 执行命令并记录结果
func RunCommand(taskID int64, command string) {
	// 执行命令
	cmd := exec.Command("sh", "-c", command)
	err := cmd.Run()
	if err != nil {
		log.Printf("Task ID %d failed to execute command: %v", taskID, err)
	} else {
		log.Printf("Task ID %d executed command successfully.", taskID)
	}
}
