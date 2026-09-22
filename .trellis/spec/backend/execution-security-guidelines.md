# 公共执行、安全输出与审计约定

> 适用于需要由 CLI 或 AI Agent 调用的异步任务，以及返回配置、任务正文、命令结果和诊断资源的接口。

## 1. 适用范围

当接口会启动、等待、取消异步任务，或者会返回可能包含密码、令牌、密钥的内容时，必须遵守本文。业务模块可以保留自己的任务表和详细状态，但要用 `internal/apps/execution` 提供统一的执行编号、用户归属、状态、确认、幂等和审计信息。

首批公共状态为：

```text
pending
running
cancel_requested
cancelling
cancelled
succeeded
failed
timed_out
```

`cancelled`、`succeeded`、`failed`、`timed_out` 是终态，不能重新进入运行状态。收到取消请求时不能直接写成 `cancelled`，只有业务模块确认实际执行已经停止后才能进入该状态。

## 2. 接口、命令与数据库签名

公共 API：

```text
GET  /api/v1/executions/:id
GET  /api/v1/executions/:id/wait?timeout_seconds=<1..30>
POST /api/v1/executions/:id/cancel
```

CLI：

```text
stx execution get <execution-id>
stx execution wait <execution-id> --timeout <duration>
stx execution cancel <execution-id> [--confirm] [--idempotency-key <key>]
```

公共执行表至少保存：

```text
execution_id, operation_id, owner_user_id, actor_type,
module, module_ref, request_id, idempotency_key_hash,
request_hash, risk_level, status, cancellable,
cancellable_reason, progress, result_ref,
error_code, error_message, started_at, finished_at
```

审计记录至少可以关联：

```text
request_id, execution_id, command_id, client_type,
risk_level, result_status, user_id
```

业务任务和审计以用户为归属单位。令牌只负责识别当前用户，不能作为任务所有者；机器编号也不能代替用户归属。

## 3. 请求、响应和环境约定

风险写请求使用以下 Header：

```text
Idempotency-Key
X-STX-Confirm
X-STX-Confirmation-ID
X-Request-ID
```

- R0：不要求确认。
- R1：要求 `X-STX-Confirm: true` 和幂等键。
- R2：先返回一次性确认编号，再使用相同幂等键和 `X-STX-Confirmation-ID` 重试。
- R3：在 R2 的基础上要求管理员权限。

CLI 成功结果只向 stdout 写一个 JSON 值。`execution wait` 的进度事件只向 stderr 写 NDJSON，最终结果仍写 stdout。非终态结果应给出：

```json
{"result_meta":{"next_command":"stx execution wait <execution-id>"}}
```

服务端在返回任务正文、提交参数、Agent 输出、错误、日志、DAG、预览结果和诊断资源前必须先屏蔽敏感内容。`--output raw` 只能改变 CLI 展示方式，不能绕过服务端处理。

客户端提交 `******` 更新 JSON 或 HOCON 时，服务端必须从已有记录恢复原值。无法确认旧值时返回错误，不能把掩码写入数据库。

新增公开 CLI/API 路由时，还必须同时完成：

1. 在 `internal/operation/registry.go` 登记操作。
2. 更新 `internal/operation/testdata/route_baseline.json`。
3. 更新 `docs/docs.go`、`docs/swagger.json`、`docs/swagger.yaml`。
4. 运行路由契约测试，确认路由、操作登记和 Swagger 一致。

## 4. 校验与错误对应表

| 情况 | 服务端行为 | CLI 行为 |
| --- | --- | --- |
| 普通用户读取他人任务 | `403` | 权限错误退出码 |
| 任务不存在 | `404` | 未找到退出码 |
| 相同幂等键、相同请求 | 返回原执行记录 | 输出原执行编号 |
| 相同幂等键、不同请求 | `409 idempotency_conflict` | 冲突退出码 |
| 风险操作缺少确认 | `428`，返回影响说明或确认编号 | 冲突类退出码，并保留请求编号 |
| 任务已经结束 | 返回 `too_late_to_cancel` | 不宣称取消成功 |
| 当前阶段无法安全停止 | 返回 `not_cancellable` 和原因 | 输出真实状态与原因 |
| 已请求停止但尚未确认 | 返回 `cancel_requested` 或 `cancelling` | `next_command` 指向 wait |
| 等待到失败终态 | 返回最终执行记录 | stdout 写最终结果，stderr 写错误事件，退出码 10 |
| 带掩码更新且无法恢复旧秘密 | `400` | 用法或服务端校验错误，不写入数据库 |

## 5. Good / Base / Bad

- Good：取消同步任务时先写 `cancel_requested`，调用真实停止接口，确认停止后再写 `cancelled`，并记录创建、开始、取消和结果审计。
- Base：升级已经进入不可中断步骤时返回 `not_cancellable`，保留运行状态并说明原因。
- Bad：只修改数据库状态为 `cancelled`，后台命令仍继续运行。
- Bad：先查询全部任务，再由 Handler 丢弃不属于当前用户的数据；归属条件必须在 Repository 查询时加入。
- Bad：把任务正文或 Agent 命令正文写入审计，或者允许 `raw` 输出原始秘密。

## 6. 必须有的测试

- 公共执行 Service：状态迁移、终态保护、重复取消、幂等冲突、R0 至 R3 确认、用户归属。
- 业务模块：创建时绑定执行编号；取消后不会被迟到的成功回调覆盖；不可取消阶段返回稳定原因。
- 审计：普通用户只能查看自己的记录；管理员可以查看全部；系统记录只允许管理员查看。
- 敏感内容：大小写字段、嵌套 map、slice、JSON 字符串、HOCON、YAML 风格和 `key=value` 都不会返回秘密原文。
- 带掩码更新：可以恢复旧值；缺少旧值时拒绝更新；数据库中不会保存 `******`。
- CLI：真实二进制连接独立 HTTP 服务，断言 stdout 是单个 JSON、stderr 是 NDJSON、退出码稳定、`next_command` 正确，取消请求包含确认和幂等 Header。
- 路由契约：新增路由已登记，操作基线和 Swagger 操作数同步更新。

## 7. 错误做法与正确做法

错误：

```go
task.Status = "cancelled"
repo.Save(ctx, task)
```

这段代码只改变展示状态，不能证明实际执行已经停止。

正确：

```go
executionService.Transition(ctx, executionID, "cancel_requested")
result, err := provider.RequestCancel(ctx, execution)
// 只有业务模块确认停止后，才允许进入 cancelled。
// Move to cancelled only after the business module confirms that execution stopped.
```

错误：直接返回保存的任务正文，或把客户端提交的 `******` 当作新秘密保存。

正确：返回前由服务端统一处理敏感内容；更新时恢复已有秘密，无法恢复就拒绝请求。

## 8. 场景：multipart 写请求的幂等摘要

### 8.1 适用范围

当上传安装包、插件包或其他文件的接口使用 `Idempotency-Key` 时，请求摘要必须能够区分文件内容。文件名和文件大小相同不代表请求相同。

### 8.2 签名

请求继续使用：

```text
Idempotency-Key: <stable-key>
X-STX-Confirm: true
```

服务端计算请求摘要时，至少包含业务字段和文件内容摘要：

```json
{
  "version": "2.3.13",
  "file_name": "apache-seatunnel-2.3.13-bin.tar.gz",
  "file_size": 450628193,
  "file_sha256": "<sha256>"
}
```

分片上传还要包含 `upload_id`、`chunk_index`、`total_chunks`、`total_size` 和当前分片的 `chunk_sha256`。

### 8.3 处理规则

- 文件摘要使用流式读取计算，不能为了幂等校验把整个文件读入内存。
- 计算摘要后，后续保存步骤必须能够重新打开并读取上传文件。
- 请求摘要只保存 SHA-256 等不可逆摘要，不保存文件正文。
- 相同幂等键只有在业务字段和文件内容摘要都相同时才允许复用原执行结果。

### 8.4 校验与错误对应表

| 情况 | 服务端行为 | CLI 行为 |
| --- | --- | --- |
| 同一幂等键、文件名和大小相同、内容相同 | 返回原执行结果 | 正常输出原结果 |
| 同一幂等键、文件名和大小相同、内容不同 | `409 idempotency_conflict` | 冲突退出码 6 |
| 文件无法打开或摘要计算失败 | `400 invalid_package_request` 或等价文件错误 | 文件传输或服务端校验错误 |
| 大文件上传 | 流式计算摘要，不整体载入内存 | 行为与小文件一致 |

### 8.5 Good / Base / Bad

- Good：请求摘要包含版本、文件名、大小和内容 SHA-256，同名同大小但内容不同的文件会被拒绝复用幂等键。
- Base：没有文件正文的普通 JSON 请求继续使用规范 JSON 的 SHA-256。
- Bad：只使用文件名和大小计算摘要；攻击者或误操作可能替换成同大小的不同文件，却得到旧请求结果。

### 8.6 必须有的测试

- 两个同名同大小但内容不同的文件，摘要必须不同。
- 计算摘要后再次打开文件，内容必须仍可完整读取。
- 真实 HTTP 上传中，同一幂等键首次成功，第二次换成同名同大小的不同内容时返回 `409`，CLI 退出码为 6。
- 大文件路径继续使用流式 multipart 和流式摘要，不出现 `io.ReadAll` 整体读取。

### 8.7 错误与正确示例

错误：

```go
requestHash, _ := execution.HashRequest(struct {
    FileName string
    FileSize int64
}{file.Filename, file.Size})
```

正确：

```go
fileSHA256, err := hashMultipartFile(file)
if err != nil {
    return err
}
requestHash, err := execution.HashRequest(struct {
    FileName   string
    FileSize   int64
    FileSHA256 string
}{file.Filename, file.Size, fileSHA256})
```

## 9. 场景：同步写操作的公共执行记录

### 9.1 范围 / 触发条件

- 触发条件：CLI 或直接 API 调用会同步修改资源，但仍需要确认、幂等和审计。
- 目标：同步接口也必须留下可查询的执行记录，不能因为响应较快而绕过公共安全约定。

### 9.2 签名

服务端使用 `BeginSynchronous`、`FinishSynchronous` 一类公共方法，输入至少包含：

```text
operation_id, module, module_ref, request_id,
idempotency_key, request_hash, risk_level,
confirmed, confirmation_id, client_type
```

执行记录至少写入：

```text
owner_user_id, execution_id, status, result_ref,
request_id, risk_level, client_type
```

### 9.3 契约

- 先按当前用户、操作编号和幂等键查找旧记录；请求摘要不同必须返回幂等冲突。
- 通过确认和权限校验后才创建运行记录，失败的前置确认请求不能伪造成功执行记录。
- 业务成功写 `succeeded` 和安全的 `result_ref`；失败写 `failed` 和可公开错误信息。
- `result_ref` 只保存重新读取结果所需的安全引用，不保存密码、令牌或原始请求正文。
- `owner_user_id` 来自当前令牌对应的用户，不能来自机器、令牌字符串或客户端传入字段。
- 接入公共执行记录的写路由必须经过登录中间件；不能只依赖 Handler 内读取用户，否则未认证上下文会把 `owner_user_id` 写成 `0`，后续幂等重试也无法读取首次结果。

### 9.4 校验与错误对应表

| 情况 | 服务端行为 |
| --- | --- |
| 相同用户、相同操作和相同请求摘要 | 返回原执行结果 |
| 相同幂等键但请求摘要不同 | 返回 `409 idempotency_conflict` |
| R1 缺少确认 | 返回 `428`，不创建成功执行记录 |
| R2 缺少确认编号 | 返回一次性确认编号，不执行删除 |
| 业务执行成功 | 执行记录为 `succeeded`，审计结果为成功 |
| 业务执行失败 | 执行记录为 `failed`，审计不包含敏感正文 |

### 9.5 Good / Base / Bad

- Good：用户创建、更新和删除都通过公共执行服务记录执行编号，并让审计关联请求编号和执行编号。
- Base：网页暂时保留旧调用方式时，CLI 和直接 API 仍必须显式传入来源标记。
- Bad：Handler 直接修改数据库后返回成功，不写执行记录或按令牌而不是用户归属记录任务。

### 9.6 必须有的测试

- 单元测试覆盖相同请求复用、幂等冲突、R1、R2、失败状态和用户归属。
- 路由测试或真实请求必须确认写接口能读取当前登录用户，首次执行与幂等重试都由同一用户访问。
- 真实 CLI 测试覆盖创建、更新、删除、普通用户权限拒绝和删除后的资源查询。
- 数据库检查确认执行结果、审计详情、资源名称和错误信息不含密码原文或密码摘要。
- 审计检查确认 `client_type`、`request_id`、`execution_id`、`risk_level` 和结果状态存在。

### 9.7 错误与正确示例

错误：

```go
userRepo.Update(ctx, user)
return c.JSON(http.StatusOK, user)
```

正确：

```go
execution, reused, err := executionService.BeginSynchronous(ctx, actor, input)
if err != nil {
    execution.WriteError(c, err)
    return
}
if !reused {
    // 业务成功后只保存安全引用，并把状态改为 succeeded。
    // On success, store only a safe reference and mark the execution succeeded.
}
```

## 10. 场景：安装后的节点登记与升级就绪重试

### 10.1 范围 / 触发条件

- 安装请求携带 `cluster_id`，并要求安装完成后自动加入集群和启动节点。
- 安装或升级的 Agent 命令已经成功，但后续控制面登记、节点启动或模板任务可能因短暂未就绪而失败。
- 目标是让任务状态反映完整业务结果，同时只对明确的暂时错误有限重试。

### 10.2 签名

安装请求至少把以下字段传到安装 Service：

```text
cluster_id, host_id, node_role, install_dir,
cluster_port, worker_port, http_port
```

集群 Service 提供幂等节点登记入口：

```go
EnsureNodeForInstallation(
    ctx context.Context,
    clusterID uint,
    hostID uint,
    role string,
    installDir string,
    hazelcastPort int,
    apiPort int,
    workerPort int,
) error
```

升级模板任务步骤使用已有字段记录重试次数：

```text
UpgradeTaskStep.retry_count
```

### 10.3 契约

- Agent 完成文件安装后，Control Plane 必须先按“集群、主机、角色”创建或刷新节点，再调用集群 Service 的启动方法。
- 节点已经存在时只刷新安装目录和端口，不能重复创建相同角色节点。
- 节点登记或启动失败时，安装任务必须进入 `failed`，并在 `error`、`message` 中返回真实原因；不能保留 `success`。
- 安装和启动全部完成后，`current_step` 必须为 `complete`。
- 升级模板任务默认最多执行 3 次：立即执行、等待 3 秒后执行、再等待 5 秒后执行。
- 只有错误包含 `Unable to connect to any cluster`、`connection refused` 或 `cluster is not ready` 时允许重试。
- 每次重试前更新 `retry_count`，并记录 `attempt`、`max_attempts`、`retryable`、`retry_delay_seconds`。
- 脚本不存在、模板不存在、配置解析失败、插件缺失和权限错误不能重试。
- 模板任务达到最大次数仍失败时，遵守升级模块已有的非阻塞告警规则，但必须保留最后一次错误和总尝试次数。

### 10.4 校验与错误对应表

| 情况 | 行为 |
| --- | --- |
| 携带 `cluster_id`，节点不存在 | 创建节点元数据后启动 |
| 携带 `cluster_id`，节点已存在 | 刷新安装目录和端口后启动，不新增重复节点 |
| 节点登记失败 | 安装任务进入 `failed`，不调用启动 |
| 节点启动失败 | 安装任务进入 `failed`，保留启动错误 |
| 安装和启动成功 | 状态为 `success`，`current_step=complete` |
| 模板任务出现暂时连接错误 | 按 3 秒、5 秒间隔有限重试 |
| 模板任务出现永久错误 | 只执行一次，直接记录告警 |
| 三次均为暂时连接错误 | `retry_count=2`，记录最终告警，不进行第 4 次 |

### 10.5 Good / Base / Bad

- Good：新集群不预建节点，安装完成后自动创建节点并启动；升级模板任务第 3 次成功，日志可看到前两次重试原因。
- Base：用户已经手工创建节点，安装完成后刷新节点目录和端口，再启动同一节点。
- Bad：Agent 返回安装成功后直接调用启动，找不到节点时仍让安装任务显示 `success`。
- Bad：对所有模板任务错误无条件重试，掩盖配置或插件永久错误。

### 10.6 必须有的测试

- 集群 Service：节点不存在时创建；节点存在时保持原 ID 并刷新目录和端口。
- 安装 Service：断言登记发生在启动之前；登记失败时不调用启动且状态为 `failed`；成功时 `current_step=complete`。
- 升级 Service：暂时错误一次后成功时执行两次、`retry_count=1`；永久错误只执行一次；连续暂时错误只执行三次、`retry_count=2`。
- 日志测试：断言重试日志包含尝试次数、最大次数、等待秒数和 `retryable=true`。
- 真实验证：使用独立安装目录和端口完成安装与升级，并确认其他集群 PID、端口和状态不变。

### 10.7 错误与正确示例

错误：

```go
status.Status = StepStatusSuccess
nodeStarter.StartNodeByClusterAndHostAndRole(ctx, clusterID, hostID, role)
```

这会在节点尚未写入数据库时启动，并可能用成功状态掩盖后置失败。

正确：

```go
if err := nodeStarter.EnsureNodeForInstallation(ctx, clusterID, hostID, role, installDir, clusterPort, apiPort, workerPort); err != nil {
    markInstallationFailed(status, err)
    return
}
if _, _, err := nodeStarter.StartNodeByClusterAndHostAndRole(ctx, clusterID, hostID, role); err != nil {
    markInstallationFailed(status, err)
    return
}
status.CurrentStep = InstallStepComplete
```

错误：

```go
for err != nil {
    runSmokeTest()
}
```

正确：

```go
for attempt := 1; attempt <= 3; attempt++ {
    err := runSmokeTest()
    if err == nil || !isRetryableClusterReadinessError(err) {
        break
    }
    recordRetry(attempt)
}
```

## 11. 场景：安装包关联源码的保存、补充和下载

### 11.1 范围 / 触发条件

- 自动下载安装包时，默认同时获取同版本 SeaTunnel 源码。
- 手工导入运行包时，可以附带源码；已有运行包也可以单独补充或替换源码。
- AI Agent 只能下载完整源码压缩包，STX 不解压、不搜索源码内容。

### 11.2 签名

```text
POST /api/v1/packages/download
body: {version, mirror?, with_source?}  # with_source 缺省为 true

POST /api/v1/packages/upload
multipart: version, file, source_file?

POST /api/v1/packages/:version/source/upload
multipart: source_file

POST /api/v1/packages/:version/source/fetch
body: {mirror?}

GET /api/v1/packages/:version/source/download
```

`PackageInfo` 至少返回：

```text
has_source, source_status, source_file_name, source_file_size,
source_checksum, source_uploaded_at, source_download_urls, source_error
```

### 11.3 契约

- 运行包文件名为 `apache-seatunnel-<version>-bin.tar.gz`，源码文件名为 `apache-seatunnel-<version>-src.tar.gz`。
- 本地安装包列表仍以运行包为主记录；只有源码而没有运行包时，不产生独立列表项。
- 自动下载中，运行包成功而源码失败时，运行包仍可安装和升级；任务返回 `completed`，同时把源码标记为 `failed` 并返回原因。
- 源码上传或在线补充使用临时文件，完成大小、gzip、完整 tar 流和版本根目录校验后再原子替换。
- 上传时不依赖用户本地文件名判断版本；文件即使被重命名，只要压缩包内容属于路径参数指定版本也允许导入。落盘名称统一为 `apache-seatunnel-<version>-src.tar.gz`。
- 源码整包下载需要登录，只能按版本读取固定路径，客户端不能提交服务器路径。
- `stx package source fetch` 默认使用 Apache 镜像，并使用独立的长耗时请求超时；默认值为 10 分钟，可通过 `--timeout` 调整，不能复用普通查询的 30 秒超时。
- CLI 不提供安装包或源码删除命令；服务端网页删除运行包时可以同时清理关联源码。

### 11.4 校验与错误对应表

| 情况 | 行为 |
| --- | --- |
| 源码文件被用户重命名 | 继续校验压缩包内容，成功后按固定文件名保存 |
| gzip 或 tar 不可读 | `400 invalid_source_package`，临时文件被清理 |
| tar 前半段有效、后半段损坏 | `400 invalid_source_package`，不能在发现版本目录后提前返回成功 |
| 压缩包根目录版本不一致 | `400 invalid_source_package` |
| 运行包不存在时补源码 | `404 package_not_found` |
| 自动下载源码返回 404 | 运行包任务成功，`source_status=failed` |
| 下载未保存的源码 | `404 package_not_found` |
| 源码在线补充超过普通 CLI 超时 | 命令使用自己的 `--timeout`，默认 10 分钟 |

### 11.5 Good / Base / Bad

- Good：自动下载先保存运行包，再下载和校验源码；源码失败只记录独立状态。
- Base：手工导入只上传运行包，列表显示 `has_source=false`，之后再补传源码。
- Bad：源码失败后删除已经可用的运行包，把未经校验的压缩包直接覆盖旧源码，或把本地文件名当成版本真实性依据。

### 11.6 必须有的测试

- 服务测试：有效源码保存成功，字段和 SHA-256 正确。
- 服务测试：官方 `apache-seatunnel-<version>-src/` 根目录和重命名的本地文件均可导入。
- 服务测试：匹配版本目录后的 tar 数据损坏仍会被拒绝。
- 服务测试：无效源码不会删除运行包，也不会覆盖旧源码。
- 下载测试：省略 `with_source` 时会请求运行包和源码；源码 404 时运行包仍为成功。
- Handler 测试：上传请求摘要包含源码文件内容摘要。
- CLI 测试：`--source-file` 发送两个 multipart 文件，源码整包下载使用临时文件和原子改名，在线补充使用独立长超时。
- 前端检查：自动下载默认带源码，本地列表显示源码状态，并能补传、联网补充和下载。

### 11.7 错误与正确示例

错误：

```go
if sourceErr != nil {
    _ = os.Remove(runtimePath)
    return sourceErr
}
```

正确：

```go
info, err := saveRuntimePackage(...)
if err != nil {
    return nil, err
}
if sourceErr := saveOptionalSource(...); sourceErr != nil {
    info.SourceStatus = DownloadStatusFailed
    info.SourceError = sourceErr.Error()
}
return info, nil
```

## 12. 场景：诊断资源单项执行、组合选择与 Agent 命令审计

### 12.1 范围 / 触发条件

- 诊断资源需要被 CLI 或 AI Agent 单独查看、执行或下载。
- 组合诊断任务需要只采集用户明确选择的资源。
- Control Plane 向 Agent 发送命令时，需要记录发送前、成功、失败和超时状态。

### 12.2 签名

```text
GET  /api/v1/diagnostics/resources
GET  /api/v1/diagnostics/resources/:code
POST /api/v1/diagnostics/resources/:code/run
GET  /api/v1/diagnostics/tasks/:id/artifacts

stx diagnostics resource list
stx diagnostics resource get <code>
stx diagnostics resource run <code> --cluster-id <id> [--node-id <id>] --confirm
stx diagnostics task create --cluster-id <id> --resource <code> [--resource <code>] --confirm
stx diagnostics task artifacts <task-id>
```

任务选项增加：

```text
selected_resources: []string
resource_only: bool
```

命令日志至少保存：

```text
request_id, execution_id, client_type, command_id, command_type,
display_command, parameters, status, error, started_at, finished_at, created_by
```

### 12.3 契约

- 公开资源编码与内部流程步骤分开。`ASSEMBLE_MANIFEST`、`RENDER_HTML_SUMMARY`、`COMPLETE` 不出现在资源列表。
- `selected_resources` 为空时保持原有诊断包行为；非空时只运行所选公开资源及组合任务需要的内部步骤。
- `resource_only=true` 时必须且只能选择一个公开资源，并跳过 Manifest 和 HTML 报告，`COMPLETE` 仍执行。
- 单项资源运行立即创建并启动诊断任务，返回稳定的 `execution_id`；CLI 后续使用 `stx execution wait`。
- JVM Dump 仍为 R3 且只允许管理员；线程快照为 R1；其他当前公开资源为 R0。
- 任务产物接口只返回任务目录内的普通文件和安全相对路径，不跟随符号链接。大于 64 MiB 的文件列表响应不现场计算 SHA-256，避免一次列表查询读取完整大文件。
- Agent 命令在发送前先写 `pending` 命令日志，得到响应后更新为终态；Agent 不存在、未连接、发送失败、超时或上下文取消都必须更新为 `failed`。
- 后台诊断任务从公共执行记录恢复 `request_id` 和 `client_type`，再写入命令上下文。
- `display_command` 必须经过服务端脱敏。线程快照和 JVM Dump 成功时记录 Agent 实际采用的 `jcmd`、`jstack` 或 `jmap` 命令；不经过系统命令的 Agent 操作记录为 `agent:<command_type>` 加安全参数说明。

### 12.4 校验与错误对应表

| 情况 | 行为 |
| --- | --- |
| 未知资源编码 | `404 diagnostic_resource_not_found`，不创建任务 |
| `resource_only=true` 但选择数量不是 1 | `400 invalid_diagnostic_task_request` |
| 普通用户运行 JVM Dump | `403 admin_required`，不发送 Agent 命令 |
| 写命令缺少确认或幂等键 | 按公共执行规则返回 `400`、`428` 或冲突错误 |
| 任务尚未产生文件 | `artifacts` 返回 `items: []`，不能返回 `null` |
| 产物目录中存在符号链接 | 列表忽略该条目，不能读取链接指向的外部文件 |
| Agent 发送前失败 | `command_logs` 保留一条 `failed` 记录和错误原因 |
| Agent 返回线程快照工具信息 | `display_command` 保存实际 Java 工具命令 |

### 12.5 Good / Base / Bad

- Good：单项线程快照只运行线程快照和完成步骤，命令日志保存请求来源、执行编号和实际 `jcmd` 命令。
- Base：组合任务选择配置和日志资源，仍生成 Manifest 与 HTML，但其他采集步骤全部标记为跳过。
- Bad：把全部内部步骤作为资源公开，资源单项运行仍强制生成完整报告，或 Agent 连接失败时不留下命令记录。

### 12.6 必须有的测试

- 资源登记测试：公开资源不含 Manifest、HTML 和完成步骤。
- 选择测试：单项任务只运行所选资源，组合任务只运行所选资源和内部报告步骤。
- Handler/Service 测试：未知资源、JVM Dump 管理员限制、用户归属和空产物列表。
- CLI 测试：路径、正文、确认、幂等请求头和 `next_command`。
- 审计测试：发送前失败仍有终态记录，`request_id`、`execution_id`、`client_type` 和 `created_by` 正确。
- Java 诊断测试：`jcmd`、`jstack` 和 `jmap` 的实际展示命令正确。
- 真实测试：连接本地 STX 与 Agent，执行一个单项资源和一个多资源组合任务，再检查步骤、产物、命令日志和下载校验和。

### 12.7 错误与正确示例

错误：

```go
resp, err := manager.SendCommand(ctx, agentID, commandType, params, timeout)
if err != nil {
    return err // 发送失败没有命令记录
}
createCommandLog(resp)
```

正确：

```go
commandID := uuid.NewString()
createPendingCommandLog(commandID, metadata)
resp, err := manager.SendCommandWithID(ctx, commandID, agentID, commandType, params, timeout)
if err != nil {
    updateCommandLogFailed(commandID, err)
    return err
}
updateCommandLogFromResponse(commandID, resp)
```

## 13. 场景：Agent 文件分片命令审计

### 13.1 范围 / 触发条件

- Agent 命令通过 Base64 参数传输插件、依赖、安装包或其他文件分片。
- Control Plane 需要记录实际传输动作，并允许按 `request_id` 查询该次操作。
- 文件正文可能达到数百 KB 或数 MB，不能直接写入命令日志。

### 13.2 签名

发送入口：

```go
SendCommand(ctx context.Context, agentID string, commandType string, params map[string]string) (bool, string, error)
```

`transfer_plugin` 的原始参数可包含：

```text
chunk, file_name, file_type, install_path, is_last,
offset, plugin_name, target_dir, total_size, version
```

审计参数只保存：

```text
chunk_bytes, file_name, file_type, install_path, is_last,
offset, plugin_name, target_dir, total_size, version
```

### 13.3 数据规则

- `chunk` 只用于发送，禁止写入 `command_logs.parameters`、`display_command`、`output` 或普通运行日志。
- `chunk_bytes` 保存当前 Base64 正文解码后的实际字节数，便于核对偏移量和总大小。
- `display_command` 使用安全参数生成，必须能看到文件名、目标目录、偏移量、分片大小和是否为最后一片。
- Agent 对普通分片返回 `RUNNING` 表示该分片已经接收完成；对应命令日志应写成 `success` 并设置 `finished_at`，不能长期停在 `running`。
- 分片记录和最后的 `install_plugin` 记录使用同一个 `request_id`、`client_type` 和 `created_by`，便于查询一次完整安装。

### 13.4 校验与错误对应表

| 情况 | 处理 |
| --- | --- |
| 参数包含 `chunk` | 发送给 Agent，但审计前移除正文并计算 `chunk_bytes` |
| 分片响应为 `RUNNING` | 当前分片审计记录写成 `success` |
| 分片发送失败或超时 | 预先创建的记录写成 `failed`，保留安全参数和错误 |
| 缺少 `request_id` | 仍记录技术命令，但无法关联到上层请求；公开 CLI 路径不得出现此情况 |
| 审计查询返回 Base64 正文 | 视为安全和性能错误，必须修正后再发布 |

### 13.5 Good / Base / Bad

- Good：审计显示 `chunk_bytes=385761`、`offset=0`、`file_name=connector-fake-2.3.13.jar`，并能通过请求编号查到后续安装记录。
- Base：一个文件有多条分片记录，但每条都很小、状态正确，并可按请求编号过滤。
- Bad：把完整 Base64 正文保存到参数或展示命令，导致一次查询返回几十 MB；或者已接收的分片一直显示为运行中。

### 13.6 必须有的测试

- 单元测试确认 pending 和完成记录均不含 `chunk`。
- 单元测试确认 `chunk_bytes` 等于 Base64 正文解码后的长度。
- 单元测试确认 `RUNNING` 分片响应保存为 `success`，且 `finished_at` 不为空。
- 真实测试安装一个尚未安装的小插件，再按安装请求编号查询命令日志。
- 真实查询结果必须同时包含安全的 `transfer_plugin` 和 `install_plugin` 记录，且输出大小不随 Jar 正文增长。

### 13.7 错误与正确示例

错误：

```go
commandLog.Parameters = audit.CommandParameters(params)
commandLog.DisplayCommand = buildAgentDisplayCommand(commandType, params, output)
```

正确：

```go
safeParams := sanitizeAgentCommandParameters(commandType, params)
commandLog.Parameters = toAuditParameters(safeParams)
commandLog.DisplayCommand = buildAgentDisplayCommand(commandType, safeParams, output)
if commandType == "transfer_plugin" && resp.Status == pb.CommandStatus_RUNNING {
    commandLog.Status = audit.CommandStatusSuccess
}
```

## 14. 场景：工作台正文与共享权限分开修改

### 14.1 范围 / 触发条件

- 工作台任务正文和共享设置由不同操作修改。
- 共享设置只包含公开状态与协作者，不应要求客户端回传任务正文。
- 修改正文时，即使请求体包含共享字段，也不能改变已经保存的共享设置。

### 14.2 接口与命令签名

```text
GET /api/v1/sync/tasks/:id/permissions
PUT /api/v1/sync/tasks/:id/permissions

stx sync task permissions get <id>
stx sync task permissions update <id> [--public|--private]
    [--collaborator-id <user-id>...] [--clear-collaborators]
    --confirm [--idempotency-key <key>]
```

权限修改请求：

```json
{
  "is_public": false,
  "collaborator_ids": [2, 3]
}
```

响应至少包含：

```text
task_id, is_public, collaborator_ids,
can_edit, can_manage, is_owner, is_collaborator
```

### 14.3 接口与数据规则

- `PUT /sync/tasks/:id` 只修改任务正文和基础信息；服务端必须从原记录保留 `is_public`、`collaborators`、`collaborator_ids`。
- `PUT /sync/tasks/:id/permissions` 只修改共享设置，不保存正文，也不创建任务正文版本。
- 只有任务所有者或管理员可以修改共享设置；拥有查看或编辑权限不等于可以管理共享设置。
- `collaborator_ids` 使用正整数用户编号，重复值由服务端去重；空数组表示清空协作者。
- 返回的 `collaborator_ids` 必须是数组，未配置时返回 `[]`，不能返回 `null`。
- 网页和 CLI 的权限写请求都要发送确认、幂等与客户端来源 Header。
- 审计使用 `operation_id=sync.task.permissions.update`，只记录修改前后的共享设置，不记录任务正文。

### 14.4 校验与错误对应表

| 情况 | 服务端行为 | CLI 行为 |
| --- | --- | --- |
| 无权查看任务 | `403` | 权限错误退出码 |
| 非所有者、非管理员修改共享设置 | `403` | 权限错误退出码 |
| 任务不存在 | `404` | 未找到退出码 |
| 任务已归档 | 拒绝修改 | 输出服务端错误，不宣称成功 |
| `collaborator_ids` 包含 `0` | `400` | 校验错误退出码 |
| 同一协作者重复出现 | 保存去重后的数组 | 输出去重后的结果 |
| 同时使用 `--public` 和 `--private` | 不发送请求 | 用法错误退出码 |
| 同时使用 `--clear-collaborators` 和 `--collaborator-id` | 不发送请求 | 用法错误退出码 |

### 14.5 Good / Base / Bad

- Good：网页保存共享设置时只调用权限接口，正文请求不携带共享设置；审计只保存共享设置修改前后的值。
- Base：旧客户端仍读取 `collaborators`，服务端在权限修改时同步保存 `collaborators` 与 `collaborator_ids`。
- Bad：用完整正文更新接口修改公开状态，或为了改一个协作者而把密码、连接配置和任务正文全部回传。

### 14.6 必须有的测试

- Service：所有者和管理员可以修改；普通查看者、协作者不能修改；编号 `0` 被拒绝；重复编号被去重。
- Service：正文更新请求携带伪造共享字段时，数据库中的共享设置保持不变。
- Handler：读取与修改权限的状态码、响应数组、审计 `before/after` 正确。
- CLI：读取命令路径正确；修改命令只发送共享字段；确认和幂等 Header 存在；冲突参数在本地拒绝。
- 前端：共享设置保存调用专用接口，不触发正文版本更新。
- 命令登记：所有登记了 `CommandPath` 的操作都能在 Cobra 命令树中找到。

### 14.7 错误与正确示例

错误：

```go
task.Definition = req.Definition // 请求可以顺便覆盖共享设置
repo.UpdateTask(ctx, task)
```

正确：

```go
definition := preserveTaskPermissionFields(task.Definition, req.Definition)
task.Definition = definition
repo.UpdateTask(ctx, task)
```

修改共享设置时使用独立入口：

```go
before, after, err := service.UpdateTaskPermissionsForActor(ctx, actor, taskID, req)
recordPermissionAudit(before, after)
```
