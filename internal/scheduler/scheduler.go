package scheduler

import (
	pb "GrpcmTlsCronTask/proto/scheduler/proto"
	"github.com/robfig/cron/v3"
)

// Scheduler 结构体包含一个 cron 实例
type Scheduler struct {
	Cron *cron.Cron
}

// NewScheduler 创建并返回一个新的 Scheduler 实例
func NewScheduler() *Scheduler {
	return &Scheduler{
		Cron: cron.New(),
	}
}

// Start 启动 cron 调度器
func (s *Scheduler) Start() {
	s.Cron.Start()
}

// AddTask 添加一个任务到调度器，并关联一个回调函数
func (s *Scheduler) AddTask(cronExpr string, taskID int64, command string, executeFunc func(pb.TaskDetail)) (cron.EntryID, error) {
	// 创建任务详情
	task := pb.TaskDetail{
		Id:             taskID,
		CronExpression: cronExpr,
		Command:        command,
	}

	// 定义任务执行时的回调函数
	jobFunc := func() {
		executeFunc(task)
	}

	// 添加任务到 cron
	return s.Cron.AddFunc(cronExpr, jobFunc)
}

// Stop 停止 cron 调度器
func (s *Scheduler) Stop() {
	s.Cron.Stop()
}
