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
