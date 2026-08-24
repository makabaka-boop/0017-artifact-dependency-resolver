# 制品依赖解析与版本选择服务

面向发布工程团队与 CI 构建流水线的纯后端 API 服务：登记软件制品及其语义化版本、声明依赖约束，并依据约束解析出一组无冲突的版本集合。数据持久化到 SQLite，所有变更写入追加式留存记录。

## 快速开始

```bash
go build -o /tmp/artifact-server ./cmd/server
LISTEN_ADDR=:8080 DB_PATH=./data.db /tmp/artifact-server
```

服务默认监听 `:8080`。可通过环境变量覆盖：

- `LISTEN_ADDR`：监听地址，默认 `:8080`
- `DB_PATH`：SQLite 数据文件，默认 `data.db`
- `LOG_LEVEL`：日志级别，默认 `info`

## 健康检查

```bash
curl http://127.0.0.1:8080/healthz
# {"db":"up","status":"ok"}
```

## 核心 API

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/healthz` | 健康检查 |
| POST | `/api/v1/artifacts` | 创建制品 |
| GET | `/api/v1/artifacts` | 分页列表 |
| GET | `/api/v1/artifacts/{name}` | 制品详情 |
| POST | `/api/v1/artifacts/{name}/versions` | 登记版本 |
| GET | `/api/v1/artifacts/{name}/versions` | 版本列表 |
| GET | `/api/v1/artifacts/{name}/versions/{version}` | 版本详情 |
| PUT | `/api/v1/artifacts/{name}/versions/{version}/dependencies` | 全量替换依赖 |
| GET | `/api/v1/artifacts/{name}/versions/{version}/dependencies` | 直接依赖 |
| GET | `/api/v1/artifacts/{name}/dependencies` | 依赖图 |
| POST | `/api/v1/resolve` | 解析版本集合 |
| GET | `/api/v1/resolutions` | 解析历史 |
| GET | `/api/v1/resolutions/{id}` | 解析详情 |
| POST | `/api/v1/resolutions/{id}/rollback` | 回滚为新快照 |
| POST | `/api/v1/resolutions/{id}/rerun` | 按原清单重新解析 |
| POST | `/api/v1/lockfiles` | 基于解析结果生成锁定快照 |
| GET | `/api/v1/lockfiles` | 锁定快照列表 |
| GET | `/api/v1/lockfiles/{name}` | 锁定快照详情 |
| POST | `/api/v1/resolve/lockfile` | 以锁定快照约束再次解析 |
| GET | `/api/v1/artifacts/{name}/versions/{v1}/diff/{v2}` | 两版本依赖差异 |
| GET | `/api/v1/artifacts/{name}/versions/{version}/readiness` | 发布就绪检查 |
| GET | `/api/v1/artifacts/{name}/readiness` | 就绪检查（最高版本） |
| GET | `/api/v1/changes` | 变更记录列表 |
| GET | `/api/v1/changes/{id}` | 变更记录详情 |

统一错误信封：`{"error":{"code":"...","message":"...","details":[...]}}`。

## 约束语法

依赖约束支持 `=`、`>`、`>=`、`<`、`<=`、`^`、`~` 与区间（逗号或空格表 AND，可含连字符区间），例如 `^1.2.3`、`~1.2.3`、`>=1.0.0 <2.0.0`、`1.2.3 - 2.0.0`。

## 测试与冒烟

```bash
go test -count=1 ./...
./scripts/smoke_test.sh
```

## Docker

```bash
docker build -t artifact-resolver .
docker run --rm -p 8080:8080 artifact-resolver
```

Dockerfile 使用 `BUILDPLATFORM`/`TARGETOS`/`TARGETARCH` 交叉编译，同时支持 `linux/arm64` 与 `linux/amd64`，CGO 关闭。
