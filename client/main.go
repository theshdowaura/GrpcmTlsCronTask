// client/main.go

package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"time"

	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	pb "GrpcmTlsCronTask/client/scheduler/proto" // 根据实际情况调整导入路径
)

func main() {
	var (
		caCert     string
		clientCert string
		clientKey  string
		serverAddr string
		timeout    int
	)

	// Root command with global flags
	var rootCmd = &cobra.Command{
		Use:   "scheduler-client",
		Short: "Scheduler Client 用于与调度服务器交互",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			// 可以在这里进行全局初始化，例如日志设置
		},
	}

	// Global flags
	rootCmd.PersistentFlags().StringVar(&caCert, "ca", "certs/ca.crt", "CA 证书路径")
	rootCmd.PersistentFlags().StringVar(&clientCert, "cert", "certs/client.crt", "客户端证书路径")
	rootCmd.PersistentFlags().StringVar(&clientKey, "key", "certs/client.key", "客户端密钥路径")
	rootCmd.PersistentFlags().StringVar(&serverAddr, "server", "localhost:50051", "服务器地址")
	rootCmd.PersistentFlags().IntVarP(&timeout, "timeout", "t", 10, "超时时间（秒）")

	// Add subcommands
	rootCmd.AddCommand(addCmd(&caCert, &clientCert, &clientKey, &serverAddr, &timeout))
	rootCmd.AddCommand(updateCmd(&caCert, &clientCert, &clientKey, &serverAddr, &timeout))
	rootCmd.AddCommand(deleteCmd(&caCert, &clientCert, &clientKey, &serverAddr, &timeout))
	rootCmd.AddCommand(listCmd(&caCert, &clientCert, &clientKey, &serverAddr, &timeout)) // 新增的 list 命令

	// Execute the root command
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

// Initialize gRPC client connection with TLS
func initGRPCClient(caCertPath, clientCertPath, clientKeyPath, serverAddress string) (*grpc.ClientConn, error) {
	// Load CA certificate
	ca, err := ioutil.ReadFile(caCertPath)
	if err != nil {
		return nil, fmt.Errorf("无法读取 CA 证书: %v", err)
	}
	certPool := x509.NewCertPool()
	if ok := certPool.AppendCertsFromPEM(ca); !ok {
		return nil, fmt.Errorf("无法添加 CA 证书")
	}

	// Load client certificate and key
	clientCertPair, err := tls.LoadX509KeyPair(clientCertPath, clientKeyPath)
	if err != nil {
		return nil, fmt.Errorf("无法加载客户端证书和密钥: %v", err)
	}

	// Create TLS credentials
	creds := credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{clientCertPair},
		RootCAs:      certPool,
	})

	// Dial the server with TLS
	conn, err := grpc.Dial(serverAddress, grpc.WithTransportCredentials(creds))
	if err != nil {
		return nil, fmt.Errorf("无法连接到服务器: %v", err)
	}

	return conn, nil
}

// AddTask command
func addCmd(caCert, clientCert, clientKey, serverAddr *string, timeout *int) *cobra.Command {
	var (
		cronExpr string
		command  string
	)

	cmd := &cobra.Command{
		Use:   "add",
		Short: "添加一个新任务到调度服务器",
		Run: func(cmd *cobra.Command, args []string) {
			// Validate required flags
			if cronExpr == "" || command == "" {
				fmt.Println("错误: cron 表达式和命令不能为空")
				cmd.Usage()
				os.Exit(1)
			}

			// Initialize gRPC client
			conn, err := initGRPCClient(*caCert, *clientCert, *clientKey, *serverAddr)
			if err != nil {
				log.Fatalf("初始化gRPC客户端失败: %v", err)
			}
			defer conn.Close()

			client := pb.NewTaskServiceClient(conn)

			// Create TaskRequest
			taskRequest := &pb.TaskRequest{
				CronExpression: cronExpr,
				Command:        command,
			}

			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*timeout)*time.Second)
			defer cancel()

			// Call AddTask
			response, err := client.AddTask(ctx, taskRequest)
			if err != nil {
				log.Fatalf("添加任务失败: %v", err)
			}

			if response.Status == "Success" {
				log.Printf("任务添加成功，任务ID: %d", response.TaskId)
			} else {
				log.Printf("任务添加失败: %s", response.ErrorMessage)
			}
		},
	}

	// Add command flags
	cmd.Flags().StringVarP(&cronExpr, "cron", "c", "@every 5s", "Cron 表达式")
	cmd.Flags().StringVarP(&command, "command", "m", "echo Hello, World!", "要执行的命令")

	return cmd
}

// UpdateTask command
func updateCmd(caCert, clientCert, clientKey, serverAddr *string, timeout *int) *cobra.Command {
	var (
		taskID   int64
		cronExpr string
		command  string
	)

	cmd := &cobra.Command{
		Use:   "update",
		Short: "更新一个已存在的任务",
		Run: func(cmd *cobra.Command, args []string) {
			// Validate required flags
			if taskID == 0 || cronExpr == "" || command == "" {
				fmt.Println("错误: 任务ID、cron表达式和命令不能为空")
				cmd.Usage()
				os.Exit(1)
			}

			// Initialize gRPC client
			conn, err := initGRPCClient(*caCert, *clientCert, *clientKey, *serverAddr)
			if err != nil {
				log.Fatalf("初始化gRPC客户端失败: %v", err)
			}
			defer conn.Close()

			client := pb.NewTaskServiceClient(conn)

			// Create UpdateTaskRequest
			updateRequest := &pb.UpdateTaskRequest{
				Id:             taskID,
				CronExpression: cronExpr,
				Command:        command,
			}

			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*timeout)*time.Second)
			defer cancel()

			// Call UpdateTask
			response, err := client.UpdateTask(ctx, updateRequest)
			if err != nil {
				log.Fatalf("更新任务失败: %v", err)
			}

			if response.Status == "Success" {
				log.Printf("任务更新成功，任务ID: %d", response.TaskId)
			} else {
				log.Printf("任务更新失败: %s", response.ErrorMessage)
			}
		},
	}

	// Add command flags
	cmd.Flags().Int64VarP(&taskID, "id", "i", 0, "任务ID")
	cmd.Flags().StringVarP(&cronExpr, "cron", "c", "", "新的Cron表达式")
	cmd.Flags().StringVarP(&command, "command", "m", "", "新的要执行的命令")

	// Mark flags as required
	cmd.MarkFlagRequired("id")
	cmd.MarkFlagRequired("cron")
	cmd.MarkFlagRequired("command")

	return cmd
}

// DeleteTask command
func deleteCmd(caCert, clientCert, clientKey, serverAddr *string, timeout *int) *cobra.Command {
	var (
		taskID int64
	)

	cmd := &cobra.Command{
		Use:   "delete",
		Short: "删除一个已存在的任务",
		Run: func(cmd *cobra.Command, args []string) {
			// Validate required flag
			if taskID == 0 {
				fmt.Println("错误: 任务ID不能为空")
				cmd.Usage()
				os.Exit(1)
			}

			// Initialize gRPC client
			conn, err := initGRPCClient(*caCert, *clientCert, *clientKey, *serverAddr)
			if err != nil {
				log.Fatalf("初始化gRPC客户端失败: %v", err)
			}
			defer conn.Close()

			client := pb.NewTaskServiceClient(conn)

			// Create DeleteTaskRequest
			deleteRequest := &pb.DeleteTaskRequest{
				Id: taskID,
			}

			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*timeout)*time.Second)
			defer cancel()

			// Call DeleteTask
			response, err := client.DeleteTask(ctx, deleteRequest)
			if err != nil {
				log.Fatalf("删除任务失败: %v", err)
			}

			if response.Status == "Success" {
				log.Printf("任务删除成功，任务ID: %d", response.TaskId)
			} else {
				log.Printf("任务删除失败: %s", response.ErrorMessage)
			}
		},
	}

	// Add command flags
	cmd.Flags().Int64VarP(&taskID, "id", "i", 0, "任务ID")

	// Mark flag as required
	cmd.MarkFlagRequired("id")

	return cmd
}

// ListTasks command
func listCmd(caCert, clientCert, clientKey, serverAddr *string, timeout *int) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "查看当前任务队列",
		Run: func(cmd *cobra.Command, args []string) {
			// Initialize gRPC client
			conn, err := initGRPCClient(*caCert, *clientCert, *clientKey, *serverAddr)
			if err != nil {
				log.Fatalf("初始化gRPC客户端失败: %v", err)
			}
			defer conn.Close()

			client := pb.NewTaskServiceClient(conn)

			// Create EmptyRequest
			emptyRequest := &pb.EmptyRequest{}

			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*timeout)*time.Second)
			defer cancel()

			// Call ListTasks
			response, err := client.ListTasks(ctx, emptyRequest)
			if err != nil {
				log.Fatalf("获取任务列表失败: %v", err)
			}

			// Check if there are tasks
			if len(response.Tasks) == 0 {
				fmt.Println("当前没有任务队列。")
				return
			}

			// Print the task list
			fmt.Println("当前任务队列:")
			fmt.Printf("%-5s %-20s %-25s %-25s %-15s %-20s %-20s\n", "ID", "Cron 表达式", "命令", "Next Run", "状态", "Created At", "Updated At")
			for _, task := range response.Tasks {
				fmt.Printf("%-5d %-20s %-25s %-25s %-15s %-20s %-20s\n",
					task.Id, task.CronExpression, task.Command, task.NextRun, task.Status, task.CreatedAt, task.UpdatedAt)
			}
		},
	}

	return cmd
}
