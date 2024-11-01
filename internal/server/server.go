// internal/server/server.go

package server

import (
	"context"
	"fmt"
	"go.mongodb.org/mongo-driver/mongo"
	"log"
	"os/exec"
	"sync"
	"time"

	"GrpcmTlsCronTask/internal/db"
	"GrpcmTlsCronTask/internal/scheduler"
	pb "GrpcmTlsCronTask/proto/scheduler/proto"

	"github.com/robfig/cron/v3"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

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
		// 添加任务到调度器
		entryID, err := s.Scheduler.AddTask(task.CronExpression, task.Id, task.Command)
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
			// 获取当前调度器中的任务信息
			// 由于 cron.Entry 不直接存储任务的 Cron 表达式和命令，
			// 这里简化处理，假设数据库中的任务信息是最新的，直接重新调度
			s.Scheduler.Cron.Remove(entryID)
			delete(s.Tasks, id)

			newEntryID, err := s.Scheduler.AddTask(task.CronExpression, task.Id, task.Command)
			if err != nil {
				log.Printf("Failed to update task ID %d in scheduler: %v", id, err)
				continue
			}
			s.Tasks[id] = newEntryID
			log.Printf("Updated scheduled task ID %d: %s -> %s", id, task.CronExpression, task.Command)
		} else {
			// 新任务，添加到调度器
			newEntryID, err := s.Scheduler.AddTask(task.CronExpression, task.Id, task.Command)
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
		rows, err := sqliteDB.DB.Query("SELECT id, cron_expression, command, next_run, status, created_at, updated_at FROM tasks")
		if err != nil {
			return nil, fmt.Errorf("failed to query tasks: %v", err)
		}
		defer rows.Close()

		var tasks []*pb.TaskDetail
		for rows.Next() {
			var task pb.TaskDetail
			err := rows.Scan(&task.Id, &task.CronExpression, &task.Command, &task.NextRun, &task.Status, &task.CreatedAt, &task.UpdatedAt)
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

	// 添加任务到调度器
	entryID, err := s.Scheduler.AddTask(req.CronExpression, taskID, req.Command)
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

	// 添加更新后的任务到调度器
	newEntryID, err := s.Scheduler.AddTask(req.CronExpression, taskID, req.Command)
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

// RunCommand 被调度器调用，执行命令并更新状态
func (s *Server) RunCommand(taskID int64, command string) {
	// 执行命令
	cmd := exec.Command("sh", "-c", command)
	err := cmd.Run()
	status := "Success"
	if err != nil {
		status = fmt.Sprintf("Failed: %v", err)
	}

	// 更新任务状态和下次运行时间
	err = s.updateTaskRunStatus(taskID, status)
	if err != nil {
		log.Printf("Failed to update task status for Task ID %d: %v", taskID, err)
	}
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
	if s.DBType == "sqlite" {
		sqliteDB := s.DB.(*db.SQLiteDB)
		_, err := sqliteDB.DB.Exec(
			"INSERT INTO tasks (id, cron_expression, command, next_run, status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)",
			id, cronExpr, command, "", status,
		)
		return err
	} else if s.DBType == "mongodb" {
		mongoDB := s.DB.(*db.MongoDB)
		_, err := mongoDB.Collection.InsertOne(context.Background(), bson.M{
			"_id":             id,
			"cron_expression": cronExpr,
			"command":         command,
			"next_run":        "",
			"status":          status,
			"created_at":      time.Now().Format(time.RFC3339),
			"updated_at":      time.Now().Format(time.RFC3339),
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
			"UPDATE tasks SET cron_expression = ?, command = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?",
			cronExpr, command, id,
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
			"UPDATE tasks SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?",
			status, id,
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
