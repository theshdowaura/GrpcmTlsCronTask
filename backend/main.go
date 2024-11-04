// client/main.go

package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	pb "GrpcmTlsCronTask/client/scheduler/proto" // 根据实际情况调整导入路径
)

func main() {
	// Create Fyne app
	a := app.New()
	w := a.NewWindow("Scheduler Client GUI")
	w.Resize(fyne.NewSize(600, 400))

	caCertEntry := widget.NewEntry()
	caCertEntry.SetText("certs/ca.crt")
	clientCertEntry := widget.NewEntry()
	clientCertEntry.SetText("certs/client.crt")
	clientKeyEntry := widget.NewEntry()
	clientKeyEntry.SetText("certs/client.key")
	serverAddrEntry := widget.NewEntry()
	serverAddrEntry.SetText("localhost:50051")
	taskIDEntry := widget.NewEntry()
	cronExprEntry := widget.NewEntry()
	commandEntry := widget.NewEntry()
	taskOutput := widget.NewMultiLineEntry()
	timeout := 10
	executeButton := widget.NewButton("Execute Command", func() {
		caCert := caCertEntry.Text
		clientCert := clientCertEntry.Text
		clientKey := clientKeyEntry.Text
		serverAddr := serverAddrEntry.Text
		conn, err := initGRPCClient(caCert, clientCert, clientKey, serverAddr)
		if err != nil {
			taskOutput.SetText(fmt.Sprintf("Failed to initialize gRPC client: %v", err))
			return
		}
		defer conn.Close()

		client := pb.NewTaskServiceClient(conn)
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
		defer cancel()

		if cronExprEntry.Text != "" && commandEntry.Text != "" {
			// Add Task
			taskRequest := &pb.TaskRequest{
				CronExpression: cronExprEntry.Text,
				Command:        commandEntry.Text,
			}
			response, err := client.AddTask(ctx, taskRequest)
			if err != nil {
				taskOutput.SetText(fmt.Sprintf("Failed to add task: %v", err))
				return
			}
			taskOutput.SetText(fmt.Sprintf("Task added successfully, Task ID: %d", response.TaskId))
		} else if taskIDEntry.Text != "" {
			// Delete Task
			taskID, err := strconv.ParseInt(taskIDEntry.Text, 10, 64)
			if err != nil {
				taskOutput.SetText(fmt.Sprintf("Invalid Task ID: %v", err))
				return
			}
			deleteRequest := &pb.DeleteTaskRequest{
				Id: taskID,
			}
			response, err := client.DeleteTask(ctx, deleteRequest)
			if err != nil {
				taskOutput.SetText(fmt.Sprintf("Failed to delete task: %v", err))
				return
			}
			taskOutput.SetText(fmt.Sprintf("Task deleted successfully, Task ID: %d", response.TaskId))
		} else {
			response, err := client.ListTasks(ctx, &pb.EmptyRequest{})
			if err != nil {
				taskOutput.SetText(fmt.Sprintf("Failed to list tasks: %v", err))
				return
			}
			tasks := "Current Task List:\n"
			for _, task := range response.Tasks {
				tasks += fmt.Sprintf("ID: %d, Cron: %s, Command: %s\n", task.Id, task.CronExpression, task.Command)
			}
			taskOutput.SetText(tasks)
		}
	})

	w.SetContent(container.NewVBox(
		widget.NewLabel("CA Certificate Path:"),
		caCertEntry,
		widget.NewLabel("Client Certificate Path:"),
		clientCertEntry,
		widget.NewLabel("Client Key Path:"),
		clientKeyEntry,
		widget.NewLabel("Server Address:"),
		serverAddrEntry,
		widget.NewLabel("Task ID (for delete/update):"),
		taskIDEntry,
		widget.NewLabel("Cron Expression (for add/update):"),
		cronExprEntry,
		widget.NewLabel("Command (for add/update):"),
		commandEntry,
		executeButton,
		taskOutput,
	))

	w.ShowAndRun()
}

// Initialize gRPC client connection with TLS
func initGRPCClient(caCertPath, clientCertPath, clientKeyPath, serverAddress string) (*grpc.ClientConn, error) {
	// Load CA certificate
	ca, err := ioutil.ReadFile(caCertPath)
	if err != nil {
		return nil, fmt.Errorf("\u65e0\u6cd5\u8bfb\u53d6 CA \u8bc1\u4e66: %v", err)
	}
	certPool := x509.NewCertPool()
	if ok := certPool.AppendCertsFromPEM(ca); !ok {
		return nil, fmt.Errorf("\u65e0\u6cd5\u6dfb\u52a0 CA \u8bc1\u4e66")
	}

	// Load client certificate and key
	clientCertPair, err := tls.LoadX509KeyPair(clientCertPath, clientKeyPath)
	if err != nil {
		return nil, fmt.Errorf("\u65e0\u6cd5\u52a0\u8f7d\u5ba2\u6237\u7aef\u8bc1\u4e66\u548c\u5bc6\u94a5: %v", err)
	}

	// Create TLS credentials
	creds := credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{clientCertPair},
		RootCAs:      certPool,
	})

	// Dial the server with TLS
	conn, err := grpc.Dial(serverAddress, grpc.WithTransportCredentials(creds))
	if err != nil {
		return nil, fmt.Errorf("\u65e0\u6cd5\u8fde\u63a5\u5230\u670d\u52a1\u5668: %v", err)
	}

	return conn, nil
}
