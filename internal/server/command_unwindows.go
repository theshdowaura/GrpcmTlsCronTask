//go:build !windows
// +build !windows

package server

import (
	"log"
	"os/exec"
)

func (s *Server) RunCommand(taskID int64, command string) {
	// Unix 系统使用 sh
	cmd := exec.Command("sh", "-c", command)
	startTime := time.Now().Format(time.RFC3339)

	// 捕获命令输出
	outputBytes, err := cmd.Output()
	status := "Success"
	errorMsg := ""
	if err != nil {
		status = "Failed"
		errorMsg = err.Error()
	}
	output := string(outputBytes)

	endTime := time.Now().Format(time.RFC3339)

	// 创建日志记录
	logEntry := TaskExecutionLog{
		TaskID:    taskID,
		Command:   command,
		Output:    output,
		StartTime: startTime,
		EndTime:   endTime,
		Status:    status,
		Error:     errorMsg,
	}

	// 保存日志到数据库
	err = s.saveTaskExecutionLog(logEntry)
	if err != nil {
		log.Printf("Failed to save task execution log for Task ID %d: %v", taskID, err)
	}

	// 更新任务状态和下次运行时间
	err = s.updateTaskRunStatus(taskID, status)
	if err != nil {
		log.Printf("Failed to update task status for Task ID %d: %v", taskID, err)
	}
}
