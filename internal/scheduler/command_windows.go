//go:build windows
// +build windows

package scheduler

import (
	"log"
	"os/exec"
)

// RunCommand 执行命令并记录结果（适用于 Windows）
func RunCommand(taskID int64, command string) {
	cmd := exec.Command("cmd", "/C", command)
	err := cmd.Run()
	if err != nil {
		log.Printf("Task ID %d failed to execute command: %v", taskID, err)
	} else {
		log.Printf("Task ID %d executed command successfully.", taskID)
	}
}
