// cmd/scheduler_worker/main.go

package main

import (
	"context"
	"log"

	"GrpcmTlsCronTask/internal/db"
	"GrpcmTlsCronTask/internal/scheduler"
)

func main() {
	// 配置
	dbType := "sqlite" // 或 "mongodb"
	sqliteDBPath := "tasks.db"
	mongoURI := "mongodb://localhost:27017"

	var _ interface{}
	var _ error

	// 初始化数据库连接
	if dbType == "sqlite" {
		sqliteDB, err := db.NewSQLiteDB(sqliteDBPath)
		if err != nil {
			log.Fatalf("Failed to initialize SQLite: %v", err)
		}
		defer sqliteDB.Close()
		_ = sqliteDB
	} else if dbType == "mongodb" {
		mongoDB, err := db.NewMongoDB(mongoURI)
		if err != nil {
			log.Fatalf("Failed to initialize MongoDB: %v", err)
		}
		defer mongoDB.Close(context.Background())
		_ = mongoDB
	} else {
		log.Fatalf("Unsupported dbType: %s", dbType)
	}

	// 创建调度器实例
	srv := scheduler.NewScheduler()

	// 启动调度器
	srv.Start()
	defer srv.Stop()

	// 根据数据库中的任务加载调度
	// 实现逻辑类似于 server.go 中的 loadTasksFromDB

	// 这里省略具体实现，根据你的需求进行补充
}
