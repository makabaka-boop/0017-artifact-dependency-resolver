# 制品依赖解析与版本选择服务

面向发布工程团队与 CI 构建流水线的纯后端 Go API 服务：登记软件制品及其语义化版本、声明依赖约束，并依据约束解析出一组无冲突的版本集合。数据持久化到 SQLite（modernc.org/sqlite 纯 Go 驱动，CGO 关闭），变更写入追加式留存记录。

## 标准命令

```bash
# 编译全部代码
go build ./...

# 编译并启动服务（默认监听 :8080）
go run ./cmd/server

# 运行全部测试
go test ./...

# 端到端冒烟（真实启动服务并调用 API）
./scripts/smoke_test.sh
```

## 运行环境变量

- `LISTEN_ADDR`：监听地址，默认 `:8080`
- `DB_PATH`：SQLite 数据文件，默认 `data.db`
- `LOG_LEVEL`：日志级别，默认 `info`

## 快速验证

```bash
go run ./cmd/server &
sleep 1
curl http://127.0.0.1:8080/healthz
# {"status":"ok","db":"up"}
```

## 构建与启动镜像

```bash
# 构建默认 amd64 镜像
./build_benzhi_docker.sh artifact-resolver

# 也可显式指定目标平台（arm64 / amd64），脚本内部使用
# docker buildx build --load --platform ... -f benzhi.Dockerfile
./build_benzhi_docker.sh artifact-resolver linux/arm64
./build_benzhi_docker.sh artifact-resolver linux/amd64

# 进入容器（保留完整 Go 工具链，可离线编译/测试）
docker run -it artifact-resolver:latest
# 容器内验证：
go build ./... && go test ./...
```
