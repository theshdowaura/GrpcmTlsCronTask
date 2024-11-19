// internal/server/server.go

package server

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"sync"
	"time"

	"GrpcmTlsCronTask/internal/db"
	"GrpcmTlsCronTask/internal/scheduler"
	pb "GrpcmTlsCronTask/proto/scheduler/proto"

	"github.com/robfig/cron/v3"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// TaskExecutionLog 表示任务执行的日志记录
type TaskExecutionLog struct {
	TaskID    int64  `bson:"task_id" json:"task_id"`
	Command   string `bson:"command" json:"command"`
	Output    string `bson:"output" json:"output"`
	StartTime string `bson:"start_time" json:"start_time"`
	EndTime   string `bson:"end_time" json:"end_time"`
	Status    string `bson:"status" json:"status"`
	Error     string `bson:"error,omitempty" json:"error,omitempty"`
}

// Server 实现 TaskServiceServer 接口
type Server struct {
	pb.UnimplementedTaskServiceServer // 嵌入未实现的服务器以实现接口的全部方法

	DB        interface{} // *db.SQLiteDB 或 *db.MongoDB
	DBType    string
	Scheduler *scheduler.Scheduler
	Tasks     map[int64]cron.EntryID // 任务ID到Cron EntryID的映射
	mu        sync.Mutex             // 保护Tasks映射
}

// NewServer 创建一个新的服务器实例，并启动任务同步
func NewServer(dbInstance interface{}, dbType string) *Server {
	s := &Server{
		DBType:    dbType,
		Scheduler: scheduler.NewScheduler(),
		Tasks:     make(map[int64]cron.EntryID),
		DB:        dbInstance,
	}

	// 启动调度器
	s.Scheduler.Start()

	// 从数据库注册任务到调度器
	err := s.RegisterTasks()
	if err != nil {
		log.Fatalf("Failed to register tasks: %v", err)
	}
	log.Println("Registered tasks from the database.")

	// 启动任务同步
	go s.syncTasksPeriodically(1 * time.Minute) // 每分钟同步一次

	return s
}

// RegisterTasks 从数据库加载任务并添加到调度器
func (s *Server) RegisterTasks() error {
	tasks, err := s.fetchAllTasksFromDB()
	if err != nil {
		return fmt.Errorf("failed to fetch tasks from DB: %v", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, task := range tasks {
		// 添加任务到调度器，传递 executeTask 作为回调函数
		entryID, err := s.Scheduler.AddTask(task.CronExpression, task.Id, task.Command, s.executeTask)
		if err != nil {
			log.Printf("Failed to add task ID %d to scheduler: %v", task.Id, err)
			continue
		}
		s.Tasks[task.Id] = entryID
		log.Printf("Scheduled task ID %d: %s -> %s", task.Id, task.CronExpression, task.Command)
	}

	return nil
}

// syncTasksPeriodically 定期同步数据库中的任务到调度器
func (s *Server) syncTasksPeriodically(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		<-ticker.C
		err := s.syncTasks()
		if err != nil {
			log.Printf("Error syncing tasks: %v", err)
		}
	}
}

// syncTasks 同步数据库中的任务到调度器
func (s *Server) syncTasks() error {
	tasks, err := s.fetchAllTasksFromDB()
	if err != nil {
		return fmt.Errorf("failed to fetch tasks from DB: %v", err)
	}

	// 创建一个map以便快速查找
	taskMap := make(map[int64]pb.TaskDetail)
	for _, task := range tasks {
		taskMap[task.Id] = *task
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// 检查并添加或更新任务
	for id, task := range taskMap {
		entryID, exists := s.Tasks[id]
		if exists {
			// 移除旧的任务
			s.Scheduler.Cron.Remove(entryID)
			delete(s.Tasks, id)

			// 重新添加更新后的任务
			newEntryID, err := s.Scheduler.AddTask(task.CronExpression, task.Id, task.Command, s.executeTask)
			if err != nil {
				log.Printf("Failed to update task ID %d in scheduler: %v", id, err)
				continue
			}
			s.Tasks[id] = newEntryID
			log.Printf("Updated scheduled task ID %d: %s -> %s", id, task.CronExpression, task.Command)
		} else {
			// 新任务，添加到调度器
			newEntryID, err := s.Scheduler.AddTask(task.CronExpression, task.Id, task.Command, s.executeTask)
			if err != nil {
				log.Printf("Failed to add new task ID %d to scheduler: %v", id, err)
				continue
			}
			s.Tasks[id] = newEntryID
			log.Printf("Scheduled new task ID %d: %s -> %s", id, task.CronExpression, task.Command)
		}
	}

	// 检查并删除调度器中已删除的任务
	for id, entryID := range s.Tasks {
		if _, exists := taskMap[id]; !exists {
			// 任务已从数据库中删除，移除调度器中的任务
			s.Scheduler.Cron.Remove(entryID)
			delete(s.Tasks, id)
			log.Printf("Removed task ID %d from scheduler as it no longer exists in DB", id)
		}
	}

	return nil
}

// fetchAllTasksFromDB 从数据库中获取所有任务
func (s *Server) fetchAllTasksFromDB() ([]*pb.TaskDetail, error) {
	if s.DBType == "sqlite" {
		sqliteDB := s.DB.(*db.SQLiteDB)
		rows, err := sqliteDB.DB.Query("SELECT id, cron_expression, command, output, status, created_at, updated_at FROM tasks")
		if err != nil {
			return nil, fmt.Errorf("failed to query tasks: %v", err)
		}
		defer rows.Close()

		var tasks []*pb.TaskDetail
		for rows.Next() {
			var task pb.TaskDetail
			err := rows.Scan(&task.Id, &task.CronExpression, &task.Command, &task.Output, &task.Status, &task.CreatedAt, &task.UpdatedAt)
			if err != nil {
				return nil, fmt.Errorf("failed to scan task: %v", err)
			}
			tasks = append(tasks, &task)
		}
		return tasks, nil
	} else if s.DBType == "mongodb" {
		mongoDB := s.DB.(*db.MongoDB)
		cursor, err := mongoDB.Collection.Find(context.Background(), bson.M{})
		if err != nil {
			return nil, fmt.Errorf("failed to find tasks: %v", err)
		}
		defer cursor.Close(context.Background())

		var tasks []*pb.TaskDetail
		for cursor.Next(context.Background()) {
			var task pb.TaskDetail
			err := cursor.Decode(&task)
			if err != nil {
				return nil, fmt.Errorf("failed to decode task: %v", err)
			}
			tasks = append(tasks, &task)
		}
		return tasks, nil
	}
	return nil, fmt.Errorf("unsupported db type: %s", s.DBType)
}

// AddTask 实现 AddTask RPC 方法
func (s *Server) AddTask(ctx context.Context, req *pb.TaskRequest) (*pb.TaskResponse, error) {
	// 生成任务ID
	taskID, err := s.generateTaskID()
	if err != nil {
		return &pb.TaskResponse{
			Status:       "Failed",
			ErrorMessage: fmt.Sprintf("Failed to generate task ID: %v", err),
		}, err
	}

	// 添加任务到调度器，传递 executeTask 作为回调函数
	entryID, err := s.Scheduler.AddTask(req.CronExpression, taskID, req.Command, s.executeTask)
	if err != nil {
		return &pb.TaskResponse{
			Status:       "Failed",
			ErrorMessage: fmt.Sprintf("Failed to add task to scheduler: %v", err),
		}, err
	}

	// 保存任务到数据库
	err = s.saveTaskToDB(taskID, req.CronExpression, req.Command, "scheduled")
	if err != nil {
		// 如果保存失败，移除调度器中的任务
		s.Scheduler.Cron.Remove(entryID)
		return &pb.TaskResponse{
			Status:       "Failed",
			ErrorMessage: fmt.Sprintf("Failed to save task to database: %v", err),
		}, err
	}

	// 更新内存中的任务映射
	s.mu.Lock()
	s.Tasks[taskID] = entryID
	s.mu.Unlock()

	return &pb.TaskResponse{
		Status:       "Success",
		TaskId:       taskID,
		ErrorMessage: "",
	}, nil
}

// DeleteTask 实现 DeleteTask RPC 方法
func (s *Server) DeleteTask(ctx context.Context, req *pb.DeleteTaskRequest) (*pb.TaskResponse, error) {
	taskID := req.Id

	s.mu.Lock()
	entryID, exists := s.Tasks[taskID]
	if exists {
		// 从调度器中移除任务
		s.Scheduler.Cron.Remove(entryID)
		delete(s.Tasks, taskID)
	}
	s.mu.Unlock()

	if !exists {
		return &pb.TaskResponse{
			Status:       "Failed",
			ErrorMessage: fmt.Sprintf("Task ID %d not found", taskID),
		}, fmt.Errorf("task ID %d not found", taskID)
	}

	// 从数据库中删除任务
	err := s.deleteTaskFromDB(taskID)
	if err != nil {
		return &pb.TaskResponse{
			Status:       "Failed",
			ErrorMessage: fmt.Sprintf("Failed to delete task from database: %v", err),
		}, err
	}

	return &pb.TaskResponse{
		Status:       "Success",
		TaskId:       taskID,
		ErrorMessage: "",
	}, nil
}

// UpdateTask 实现 UpdateTask RPC 方法
func (s *Server) UpdateTask(ctx context.Context, req *pb.UpdateTaskRequest) (*pb.TaskResponse, error) {
	taskID := req.Id

	s.mu.Lock()
	entryID, exists := s.Tasks[taskID]
	if exists {
		// 从调度器中移除旧的任务
		s.Scheduler.Cron.Remove(entryID)
		delete(s.Tasks, taskID)
	}
	s.mu.Unlock()

	if !exists {
		return &pb.TaskResponse{
			Status:       "Failed",
			ErrorMessage: fmt.Sprintf("Task ID %d not found", taskID),
		}, fmt.Errorf("task ID %d not found", taskID)
	}

	// 添加更新后的任务到调度器，传递 executeTask 作为回调函数
	newEntryID, err := s.Scheduler.AddTask(req.CronExpression, taskID, req.Command, s.executeTask)
	if err != nil {
		return &pb.TaskResponse{
			Status:       "Failed",
			ErrorMessage: fmt.Sprintf("Failed to update task in scheduler: %v", err),
		}, err
	}

	// 更新任务在数据库中的信息
	err = s.updateTaskInDB(taskID, req.CronExpression, req.Command)
	if err != nil {
		// 如果更新失败，移除调度器中的任务
		s.Scheduler.Cron.Remove(newEntryID)
		return &pb.TaskResponse{
			Status:       "Failed",
			ErrorMessage: fmt.Sprintf("Failed to update task in database: %v", err),
		}, err
	}

	// 更新内存中的任务映射
	s.mu.Lock()
	s.Tasks[taskID] = newEntryID
	s.mu.Unlock()

	return &pb.TaskResponse{
		Status:       "Success",
		TaskId:       taskID,
		ErrorMessage: "",
	}, nil
}

// ListTasks 实现 ListTasks RPC 方法
func (s *Server) ListTasks(ctx context.Context, req *pb.EmptyRequest) (*pb.TasksResponse, error) {
	tasks, err := s.fetchAllTasksFromDB()
	if err != nil {
		return nil, fmt.Errorf("failed to list tasks: %v", err)
	}

	return &pb.TasksResponse{
		Tasks: tasks,
	}, nil
}

// generateTaskID 生成新的任务ID
func (s *Server) generateTaskID() (int64, error) {
	if s.DBType == "sqlite" {
		sqliteDB := s.DB.(*db.SQLiteDB)
		var id int64
		err := sqliteDB.DB.QueryRow("SELECT IFNULL(MAX(id), 0) + 1 FROM tasks").Scan(&id)
		return id, err
	} else if s.DBType == "mongodb" {
		mongoDB := s.DB.(*db.MongoDB)
		var result struct {
			ID int64 `bson:"_id"`
		}
		opts := options.FindOne().SetSort(bson.D{{Key: "_id", Value: -1}})
		err := mongoDB.Collection.FindOne(context.Background(), bson.M{}, opts).Decode(&result)
		if err == nil {
			return result.ID + 1, nil
		} else if err == mongo.ErrNoDocuments {
			return 1, nil
		} else {
			return 0, err
		}
	}
	return 0, fmt.Errorf("unsupported db type: %s", s.DBType)
}

// saveTaskToDB 保存任务到数据库
func (s *Server) saveTaskToDB(id int64, cronExpr, command, status string) error {
	// 创建 UTC+8 时区
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return fmt.Errorf("failed to load location: %v", err)
	}

	if s.DBType == "sqlite" {
		sqliteDB := s.DB.(*db.SQLiteDB)
		_, err := sqliteDB.DB.Exec(
			"INSERT INTO tasks (id, cron_expression, command, output, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
			id, cronExpr, command, "", status,
			time.Now().In(location).Format(time.RFC3339),
			time.Now().In(location).Format(time.RFC3339),
		)
		return err
	} else if s.DBType == "mongodb" {
		mongoDB := s.DB.(*db.MongoDB)
		// 使用单独的集合来保存任务
		_, err := mongoDB.Collection.InsertOne(context.Background(), bson.M{
			"_id":             id,
			"cron_expression": cronExpr,
			"command":         command,
			"output":          "",
			"status":          status,
			"created_at":      time.Now().In(location).Format(time.RFC3339),
			"updated_at":      time.Now().In(location).Format(time.RFC3339),
		})
		return err
	}
	return fmt.Errorf("unsupported db type: %s", s.DBType)
}

// updateTaskInDB 更新任务在数据库中的信息
func (s *Server) updateTaskInDB(id int64, cronExpr, command string) error {
	if s.DBType == "sqlite" {
		sqliteDB := s.DB.(*db.SQLiteDB)
		_, err := sqliteDB.DB.Exec(
			"UPDATE tasks SET cron_expression = ?, command = ?, updated_at = ? WHERE id = ?",
			cronExpr, command, time.Now().Format(time.RFC3339), id,
		)
		return err
	} else if s.DBType == "mongodb" {
		mongoDB := s.DB.(*db.MongoDB)
		_, err := mongoDB.Collection.UpdateOne(context.Background(), bson.M{"_id": id}, bson.M{
			"$set": bson.M{
				"cron_expression": cronExpr,
				"command":         command,
				"updated_at":      time.Now().Format(time.RFC3339),
			},
		})
		return err
	}
	return fmt.Errorf("unsupported db type: %s", s.DBType)
}

// deleteTaskFromDB 从数据库中删除任务
func (s *Server) deleteTaskFromDB(id int64) error {
	if s.DBType == "sqlite" {
		sqliteDB := s.DB.(*db.SQLiteDB)
		_, err := sqliteDB.DB.Exec("DELETE FROM tasks WHERE id = ?", id)
		return err
	} else if s.DBType == "mongodb" {
		mongoDB := s.DB.(*db.MongoDB)
		_, err := mongoDB.Collection.DeleteOne(context.Background(), bson.M{"_id": id})
		return err
	}
	return fmt.Errorf("unsupported db type: %s", s.DBType)
}

// updateTaskRunStatus 更新任务的运行状态
func (s *Server) updateTaskRunStatus(id int64, status string) error {
	if s.DBType == "sqlite" {
		sqliteDB := s.DB.(*db.SQLiteDB)
		_, err := sqliteDB.DB.Exec(
			"UPDATE tasks SET status = ?, updated_at = ? WHERE id = ?",
			status, time.Now().Format(time.RFC3339), id,
		)
		return err
	} else if s.DBType == "mongodb" {
		mongoDB := s.DB.(*db.MongoDB)
		_, err := mongoDB.Collection.UpdateOne(context.Background(), bson.M{"_id": id}, bson.M{
			"$set": bson.M{
				"status":     status,
				"updated_at": time.Now().Format(time.RFC3339),
			},
		})
		return err
	}
	return fmt.Errorf("unsupported db type: %s", s.DBType)
}

// saveTaskExecutionLog 保存任务执行日志到数据库
func (s *Server) saveTaskExecutionLog(logEntry TaskExecutionLog) error {
	if s.DBType == "sqlite" {
		sqliteDB := s.DB.(*db.SQLiteDB)
		_, err := sqliteDB.DB.Exec(
			"INSERT INTO task_execution_logs (task_id, command, output, start_time, end_time, status, error) VALUES (?, ?, ?, ?, ?, ?, ?)",
			logEntry.TaskID, logEntry.Command, logEntry.Output, logEntry.StartTime, logEntry.EndTime, logEntry.Status, logEntry.Error,
		)
		if err != nil {
			log.Printf("SQLite Insert Error: %v", err)
		}
		return err
	} else if s.DBType == "mongodb" {
		mongoDB := s.DB.(*db.MongoDB)
		// 使用单独的集合来保存日志
		logsCollection := mongoDB.Client.Database(mongoDB.DatabaseName).Collection("task_execution_logs")
		_, err := logsCollection.InsertOne(context.Background(), logEntry)
		if err != nil {
			log.Printf("MongoDB Insert Error: %v", err)
		}
		return err
	}
	return fmt.Errorf("unsupported db type: %s", s.DBType)
}

// executeTask 执行任务命令并保存执行日志
func (s *Server) executeTask(task pb.TaskDetail) {
	startTime := time.Now().In(time.FixedZone("CST", 8*3600)).Format(time.RFC3339)

	// 执行任务命令，并捕获输出
	cmd := exec.Command("sh", "-c", task.Command)
	outputBytes, err := cmd.CombinedOutput()
	output := string(outputBytes)
	endTime := time.Now().In(time.FixedZone("CST", 8*3600)).Format(time.RFC3339)

	status := "Success"
	errorMsg := ""
	if err != nil {
		status = "Failed"
		errorMsg = err.Error()
	}

	// 创建日志条目
	logEntry := TaskExecutionLog{
		TaskID:    task.Id,
		Command:   task.Command,
		Output:    output,
		StartTime: startTime,
		EndTime:   endTime,
		Status:    status,
		Error:     errorMsg,
	}

	// 保存日志到数据库
	err = s.saveTaskExecutionLog(logEntry)
	if err != nil {
		log.Printf("Failed to save task execution log for Task ID %d: %v", task.Id, err)
	}
}
