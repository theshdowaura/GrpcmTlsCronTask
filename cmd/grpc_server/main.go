// cmd/grpc_server/main.go

package main

import (
	"context"
	"log"
	"net"

	"GrpcmTlsCronTask/internal/db"
	"GrpcmTlsCronTask/internal/server"
	pb "GrpcmTlsCronTask/proto/scheduler/proto" // 修正导入路径

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func main() {
	// 配置
	dbType := "sqlite" // 或 "mongodb"
	sqliteDBPath := "../../tasks.db"
	mongoURI := "mongodb://localhost:27017"

	var dbInstance interface{}
	var err error

	// 初始化数据库连接
	if dbType == "sqlite" {
		sqliteDB, err := db.NewSQLiteDB(sqliteDBPath)
		if err != nil {
			log.Fatalf("Failed to initialize SQLite: %v", err)
		}
		defer sqliteDB.Close()
		dbInstance = sqliteDB
	} else if dbType == "mongodb" {
		mongoDB, err := db.NewMongoDB(mongoURI)
		if err != nil {
			log.Fatalf("Failed to initialize MongoDB: %v", err)
		}
		defer mongoDB.Close(context.Background())
		dbInstance = mongoDB
	} else {
		log.Fatalf("Unsupported dbType: %s", dbType)
	}

	// 创建服务器实例
	srv := server.NewServer(dbInstance, dbType)

	// 设置 gRPC 服务器的 TLS 配置
	certFile := "certs/server.crt" // 替换为实际证书路径
	keyFile := "certs/server.key"  // 替换为实际密钥路径

	creds, err := credentials.NewServerTLSFromFile(certFile, keyFile)
	if err != nil {
		log.Fatalf("Failed to load TLS credentials: %v", err)
	}

	// 创建 gRPC 服务器
	grpcServer := grpc.NewServer(grpc.Creds(creds))

	// 注册 TaskServiceServer
	pb.RegisterTaskServiceServer(grpcServer, srv)

	// 监听端口
	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("Failed to listen on port 50051: %v", err)
	}
	log.Println("gRPC server is running on port 50051...")

	// 启动 gRPC 服务器
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("Failed to serve gRPC server: %v", err)
	}
}
