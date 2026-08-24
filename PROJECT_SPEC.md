# 制品依赖解析与版本选择服务 项目文档

一、业务目标与真实使用者
本服务面向软件发布工程团队与 CI 构建流水线，是一个纯后端 API 服务：登记软件制品及其版本、声明版本间依赖与语义化版本约束，并依据约束解析出一组无冲突的版本集合。真实使用者分两类：发布工程师通过 API 登记制品、录入 semver 版本、声明依赖约束、查询依赖图、查看解析历史并执行历史回滚；CI 构建流水线作为机器调用方，在构建前调用解析接口获取可安装版本集合，并通过 /healthz 探测服务可用性。服务保持纯 API 形态，不引入前端页面，前端 HTML/JS/CSS 等资源不计入规模。数据持久化到 SQLite。

二、核心业务闭环
1. 发布工程师创建制品，制品名满足「小写字母、数字、短横线」规则且全局唯一。
2. 为该制品登记符合语义化版本的版本号，同一制品不允许重复版本。
3. 为某版本声明依赖：依赖指向已存在的目标制品，并给出约束（^、~ 或区间写法），每个目标制品至多一条约束。
4. 发布工程师或 CI 流水线发起解析：给定根制品（可选版本，缺省选最高登记版本），服务以回溯策略递归展开传递依赖，为每个制品选择满足全部约束的版本。
5. 当某候选版本导致下游约束冲突时，解析器回溯尝试其他候选版本；若全部候选都失败，返回结构化诊断（冲突链、环路径或缺失依赖），而不是静默失败。
6. 解析结果持久化为快照，可与历史记录对照；版本登记、依赖变更、解析结果与回滚均写入变更留存。
7. 依赖图查询用于查看某制品（或某版本）的直接与传递依赖；历史回滚把某个解析快照重新物化为新的当前快照。

三、实体字段和关系
Artifact：id、name（全局唯一）、created_at、updated_at。
ArtifactVersion：id、artifact_id（FK→Artifact）、version（规范化 semver 字符串）、created_at；唯一(artifact_id, version)。
Dependency：id、from_version_id（FK→ArtifactVersion）、to_artifact_id（FK→Artifact）、constraint（约束原文）、created_at；唯一(from_version_id, to_artifact_id)。
Resolution：id、request_ref（业务唯一）、root_artifact_id、root_version_id（可空，表示选最高版本）、input_json（原始请求）、status（succeeded|failed）、result_json（解析出的版本集合）、error_code、error_message、source（direct|rollback）、source_resolution_id（可空）、created_at。
ResolutionItem：id、resolution_id（FK→Resolution）、artifact_id、version_id、selected_version、depth（依赖深度）、reason（选择说明）；反规范化便于查询明细。
ChangeRecord：id、entity_type、entity_id、action、before_json、after_json、created_at；append-only。
关系：Artifact 1:N ArtifactVersion；ArtifactVersion 1:N Dependency（作为来源端）；Artifact 1:N Dependency（作为目标端）；Resolution 1:N ResolutionItem；ChangeRecord 以 entity_type+entity_id 多态指向任意实体。

四、状态流转与约束
ArtifactVersion 为「已登记」即进入终态，版本号不可修改、不可删除，变更只能通过新增版本或重写依赖表达，保证历史可追溯。
Dependency 采用全量替换语义：PUT 依赖接口会生成新的依赖集合并写入 before/after 变更记录。
Resolution 为同步计算产物，状态为 succeeded 或 failed，二者均不可变；rollback 不修改原快照，而是创建 source=rollback 的新快照并指向原快照。
约束：制品名满足 ^[a-z0-9]+(-[a-z0-9]+)*$（小写字母、数字、短横线，不以短横线开头或结尾），长度≤128；版本必须是合法 semver（major.minor.patch，可含 prerelease/build，忽略前导 v 并规范化）；依赖约束语法必须合法并支持 =、>、>=、<、<=、^、~ 与区间（逗号或空格表 AND、可含连字符区间）；依赖目标制品必须已存在；同一版本对同一目标制品至多一条约束；根制品必须存在，解析失败不产生 5xx，而是以 failed 结果返回诊断。

五、API 输入输出和错误语义
统一错误信封：{"error":{"code":"...","message":"...","details":[...]}}。状态码约定：200 成功、201 创建、204 删除（如适用）、400 请求体/名称/版本/约束非法、404 资源不存在、409 重名或重复版本、422 依赖目标缺失等业务校验失败、500 内部错误、503 未就绪。
GET /healthz → 200 {"status":"ok","db":"up"}；DB 不可用 → 503。
POST /api/v1/artifacts {"name"} → 201 Artifact；非法名称 400；重名 409。
GET /api/v1/artifacts?limit=&offset= → 分页列表。
GET /api/v1/artifacts/{name} → 200；404。
POST /api/v1/artifacts/{name}/versions {"version"} → 201；非法 semver 400；重复 409；制品不存在 404。
GET /api/v1/artifacts/{name}/versions → 列表。
GET /api/v1/artifacts/{name}/versions/{version} → 版本详情及直接依赖；404。
PUT /api/v1/artifacts/{name}/versions/{version}/dependencies {"dependencies":[{"artifact","constraint"}]} → 200 全量替换；约束非法 400；目标制品不存在 422；404。
GET /api/v1/artifacts/{name}/versions/{version}/dependencies → 直接依赖列表。
GET /api/v1/artifacts/{name}/dependencies?depth= → 依赖图（直接+传递）；404；存在环时在响应中标注环路径。
POST /api/v1/resolve {"artifact","version"(可选)} → 200 {"resolution_id","status":"succeeded|failed","resolved":[{"artifact","version"}],"diagnostics":[{"type","message","details"}]}；参数非法 400；根制品不存在 404；failed 用于 CYCLE/CONFLICT/MISSING。
GET /api/v1/resolutions?limit=&offset=&status= → 历史列表。
GET /api/v1/resolutions/{id} → 详情含明细与诊断；404。
POST /api/v1/resolutions/{id}/rollback → 200 新快照（source=rollback）；404。
GET /api/v1/changes?entity_type=&entity_id=&limit=&offset= → 变更记录列表。
GET /api/v1/changes/{id} → 200；404。

六、持久化与变更留存
使用 SQLite 与 modernc.org/sqlite 纯 Go 驱动（CGO 关闭，便于交叉编译 linux/arm64 与 linux/amd64），开启 WAL、foreign_keys=ON、busy_timeout，由 migration 维护 schema_version。表：artifacts、artifact_versions、dependencies、resolutions、resolution_items、change_records；唯一索引覆盖 artifacts.name、artifact_versions(artifact_id,version)、dependencies(from_version_id,to_artifact_id)、resolutions.request_ref。业务写与对应 change_records 在同一事务提交；change_records 只增不改不删，作为审计与回溯依据；resolutions.result_json 保存解析快照，保证历史可复现。

七、模块边界
cmd/server/main.go：装配 config→db→store→service→httpapi，监听 :8080，处理优雅退出。
internal/config：读取环境变量（LISTEN_ADDR 默认 :8080、DB_PATH、LOG_LEVEL）。
internal/model：实体、状态枚举、制品名称与版本基础校验。
internal/semver：semver 解析、校验、比较、规范化（prerelease 优先级、build 忽略）。
internal/constraint：约束语法解析与匹配（操作符、^、~、区间、AND 组合）。
internal/resolver：依赖图构建、回溯式版本选择、传递依赖展开、环检测、冲突与缺失诊断、确定性排序。
internal/store：SQLite 仓储接口与实现（制品/版本/依赖/解析/明细/变更记录）。
internal/service：业务编排、输入校验、状态约束、变更记录写入、解析与回滚。
internal/httpapi：路由、handler、DTO、错误映射、healthz。
internal/errcode：错误码与 HTTP 状态码映射。
各层仅依赖下层接口，禁止跨层直连数据库。

八、关键测试
semver：合法/非法解析、比较、prerelease 与 build 元数据、规范化与稳定排序。
constraint：^、~、各比较符、区间边界、连字符区间、AND 组合、非法语法、匹配与淘汰。
resolver：单层最高版本选择、传递依赖展开、冲突时回溯选择替代版本、循环检测路径、缺失依赖、确定性输出、失败诊断。
service：制品重名、重复版本、依赖目标缺失、变更记录随事务写入、回滚生成新快照。
store：唯一约束、事务写入 change_records、分页查询、result_json 可读回放。
httpapi：各端点状态码与错误信封、healthz、非法 JSON。
集成：临时 SQLite 端到端（创建→登记版本→声明依赖→解析→依赖图→历史→回滚）。

九、启动冒烟和验收场景
scripts/smoke_test.sh：真实启动服务（临时 DB），轮询 GET /healthz 确认 200；随后创建制品与版本、声明依赖、调用解析并校验返回的版本集合；用 trap 在退出时清理进程与临时数据，并以退出码反映结果。
Dockerfile：基于官方 golang 镜像在容器内编译，EXPOSE 8080，服务仅监听 8080，支持 linux/arm64 与 linux/amd64（不写死 CPU 架构、不 COPY 本机二进制）。
验收场景：1 制品重名返回 409；2 非法 semver 返回 400；3 非法约束返回 400；4 依赖目标不存在返回 422；5 冲突场景解析通过回溯选中替代版本；6 无解场景返回 failed 与冲突/环/缺失诊断；7 依赖图查询返回直接+传递依赖；8 回滚生成新快照；9 变更记录可查询历史快照；10 /healthz 返回 200；11 Docker 双架构构建成功且可访问 API。

十、两阶段计划
本项目计划产出 1 条 Bug 数据。最终验收规模：非测试 Go 源码严格大于 2000 且小于 5000 行，非测试 .go 文件严格大于 20 且小于 50 个；测试代码、前端 HTML/JS/CSS 等资源、vendor、空行与纯注释均不计入。
初始核心版本（约 1900 行非测试 Go 有效代码、约 22 个生产 .go 文件，落在 1600~2200 行、18~24 个文件区间）：实现上述全部模块与核心 API（制品登记、版本登记、依赖声明、依赖图查询、解析、历史查询、回滚、变更留存、healthz）、Dockerfile、smoke_test.sh，完整可运行、可 Docker 部署、可冒烟；初始版本不为最终规模凑代码。
第一次业务扩写（约 3000 行、约 34 个生产 .go 文件，落在 2600~3800 行、28~40 个文件区间）：在初始版本上新增真实业务能力——锁定文件快照（解析后将版本 pin 为可复现快照，支持以锁定快照约束再次解析）、版本依赖差异对比（同一制品两个版本的依赖新增/删除/约束变化）、解析历史分页与重跑（按 resolution_id 复用原清单重解析）、发布就绪检查（校验某版本全部传递依赖可解析并列出阻断项）、冲突诊断明细增强（列出候选版本与逐条淘汰原因）。即使初始版本已超过最终最低门槛，也必须通过上述真实业务能力完成第一次扩写；禁止复制粘贴、空实现、无调用模块、无意义包装和其他死代码，所有生产代码必须被 API、业务服务、后台任务或启动路径真实调用。

十一、代码质量与规模约束
不得用重复代码、死代码、空实现、无调用模块或无意义包装凑行数；每个生产文件、类型和函数都必须服务于文档中的真实业务路径，并由 API、业务服务、后台任务或启动流程实际使用。测试代码、前端 HTML/JS/CSS 等资源、vendor、空行与纯注释均不计入验收规模；所有非测试 .go 源码均需纳入有效生产路径，禁止保留未挂接的可达死代码。

十二、实现状态
当前处于「第一次业务扩写」已完成阶段。相较初始核心版本，新增以下真实业务能力，均经 API、业务服务与启动路径真实调用：

1. 锁定文件快照（lockfiles / lockfile_entries 表）：
   - `POST /api/v1/lockfiles`：基于一次 succeeded 解析生成 pin 快照；
   - `GET /api/v1/lockfiles`（分页）与 `GET /api/v1/lockfiles/{name}`；
   - `POST /api/v1/resolve/lockfile`：以锁定快照为基准再次解析，返回 in_sync / drifted 及逐条 drift 明细；
   - 失败解析不可生成锁定（422）。
2. 版本依赖差异对比：
   - `GET /api/v1/artifacts/{name}/versions/{v1}/diff/{v2}`：返回新增 / 删除 / 约束变化三类，按制品名稳定排序。
3. 解析历史分页与重跑：
   - `POST /api/v1/resolutions/{id}/rerun`：按原请求复用根制品与版本重新解析，生成 source=rerun 的新快照。
4. 发布就绪检查：
   - `GET /api/v1/artifacts/{name}/versions/{version}/readiness` 与 `GET /api/v1/artifacts/{name}/readiness`（缺省最高版本）：校验全量传递依赖是否可解析并列出阻断项。
5. 冲突诊断明细增强：
   - resolver 在回溯失败时记录每个候选版本与（不满足约束 / 下游冲突等）淘汰或接受原因，透出到解析输出的 diagnostics[].candidates。

对应测试覆盖：service（锁定、diff、rerun、readiness、禁锁失败解析、重名锁）、resolver（候选明细诊断）、store（锁定持久化与唯一名）、httpapi（新端点状态码）。
