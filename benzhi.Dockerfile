# 官方 Go 镜像，自带完整工具链（单阶段，保留完整 go 工具链供评测改码/编译/测试）
FROM golang:1.22

WORKDIR /app

# 先复制依赖文件并下载依赖（利用 Docker 缓存，也保证容器内离线可用）
COPY go.mod go.sum ./
RUN go mod download

# 复制所有项目文件（.dockerignore 已排除构建脚本、BENZHI_README.md、Dockerfile 等交付/构建文件，避免污染镜像）
COPY . .

# 预编译一次，把编译缓存留在镜像里（不影响源码，模型仍可自由修改）
RUN go build ./...

# 容器启动后进入 shell，方便操作
CMD ["bash"]
