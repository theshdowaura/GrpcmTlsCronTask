# 使用官方 Golang 镜像作为基础镜像
FROM golang:1.22 AS builder
LABEL authors="haojiejack"

# 设置工作目录
WORKDIR /app

# 将 go.mod 和 go.sum 文件复制到工作目录
COPY go.mod ./
COPY go.sum ./

# 下载依赖
RUN go mod download

# 将项目中的所有代码复制到工作目录
COPY . .

# 编译项目
RUN go build -v -ldflags="-s -w" -o /app/grpc_task ./cmd/grpc_server

# 使用一个轻量级的镜像来运行二进制文件
FROM busybox:latest
WORKDIR /app
# 复制证书文件夹和编译好的二进制文件
COPY --from=builder /app/certs /app/certs
COPY --from=builder /app/grpc_task /app/grpc_task

# 设置运行时环境变量（如果有需要的环境变量，添加在这里）
# ENV ENV_VAR_NAME value

# 暴露应用监听的端口（根据你的应用端口）
EXPOSE 50051

# 运行应用
CMD ["/app/grpc_task"]
