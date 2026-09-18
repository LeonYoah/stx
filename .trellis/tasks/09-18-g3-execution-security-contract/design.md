# 统一安全执行、任务取消与审计设计

## 1. 设计目标

诊断、升级和同步继续维护各自的业务表、步骤和详细状态。新增公共执行记录，为 API、CLI 和 AI Agent 提供一致的执行编号、用户归属、状态、取消语义、风险提示和审计关联。

公共执行记录不是新的任务调度器，也不替代业务模块。它负责公共约定，具体执行和实际停止仍由业务模块完成。

## 2. 当前问题

- 诊断和升级使用 `context.Background()` 启动后台任务，没有任务级取消控制。
- 同步取消会调用停止接口，但随后直接把数据库状态写成已取消，没有区分“已经请求停止”和“已经确认停止”。
- 三个模块的列表和详情接口没有统一执行编号，也没有统一的用户归属检查。
- 审计记录缺少请求编号、执行编号、客户端来源、风险等级和结果状态。
- 操作登记表已经声明风险等级和影响文案，但服务端还没有执行统一确认和幂等检查。
- Agent 协议没有取消单条已下发命令的消息，控制端取消等待不能证明 Agent 进程已经停止实际命令。

## 3. 模块结构

新增 `internal/apps/execution/`：

```text
internal/apps/execution/
├── model.go          # 公共执行记录、状态和确认记录
├── repository.go     # GORM 持久化与并发更新
├── service.go        # 状态迁移、用户权限、幂等和确认
├── provider.go       # 业务模块适配接口
├── handler.go        # get、wait、cancel API
├── middleware.go     # 风险确认、幂等键和请求编号读取
└── *_test.go
```

CLI 新增：

```text
stx execution get <execution-id>
stx execution wait <execution-id>
stx execution cancel <execution-id>
```

## 4. 公共执行记录

`Execution` 使用 UUID 字符串作为对外编号，核心字段如下：

| 字段 | 说明 |
| --- | --- |
| `execution_id` | 对外稳定编号 |
| `operation_id` | 操作登记表编号 |
| `owner_user_id` | 执行所有者，系统任务为 0 |
| `actor_type` | `user` 或 `system` |
| `module` | `diagnostics`、`stupgrade`、`sync` |
| `module_ref` | 模块任务 ID |
| `request_id` | HTTP 调用链编号 |
| `idempotency_key_hash` | 幂等键 SHA-256，不保存原文 |
| `request_hash` | 规范化请求摘要，用于判断幂等冲突 |
| `risk_level` | R0 至 R3 |
| `status` | 公共状态 |
| `cancellable` | 当前是否允许请求取消 |
| `cancellable_reason` | 不允许取消的稳定原因 |
| `progress` | 0 至 100 |
| `result_ref` | 结果引用，不保存大型正文 |
| `error_code`、`error_message` | 脱敏后的失败信息 |
| `started_at`、`finished_at` | 运行时间 |

数据库唯一约束：

```text
(owner_user_id, operation_id, idempotency_key_hash)
```

相同键且请求摘要相同，返回原执行；请求摘要不同，返回 `idempotency_conflict`。

## 5. 状态迁移

公共状态：

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

允许的主要迁移：

```text
pending -> running | cancel_requested | cancelled | failed
running -> cancel_requested | succeeded | failed | timed_out
cancel_requested -> cancelling | cancelled | failed
cancelling -> cancelled | failed | timed_out
```

终态 `cancelled`、`succeeded`、`failed`、`timed_out` 不允许回到非终态。Repository 使用状态条件更新，避免完成回调和取消请求互相覆盖。

## 6. 风险确认和幂等

沿用已经确认的 Header：

```text
Idempotency-Key
X-STX-Confirm
X-STX-Confirmation-ID
X-Request-ID
```

- R0：直接执行。
- R1：要求 `X-STX-Confirm: true` 和幂等键。
- R2：第一次请求只返回影响说明和一次性确认编号；第二次请求必须携带相同幂等键和 `X-STX-Confirmation-ID`。
- R3：按 R2 处理，并要求管理员权限。

确认记录绑定用户、操作编号、请求摘要和幂等键，默认五分钟失效，使用一次后作废。确认编号不写入日志和可复制命令样例。

## 7. 用户归属

- 业务任务继续保存 `CreatedBy`，公共执行记录保存 `OwnerUserID`。
- 普通用户只能查询、等待和取消自己的执行。
- 管理员可以查询全部执行，但取消他人执行仍写入实际操作者和所有者。
- `OwnerUserID=0` 的系统任务只允许管理员查看。
- 列表接口在 Repository 查询阶段增加用户过滤，不能先查全部再在 Handler 丢弃。

## 8. 三个模块的取消语义

### 8.1 诊断

诊断任务增加 `cancel_requested`、`cancelling`。服务保存运行中的任务控制项，但不取消正在等待的 Agent 命令，避免把控制端停止等待误当成 Agent 命令已经停止。

取消请求到达后：

1. 任务进入 `cancel_requested`。
2. 当前步骤仍然等待真实返回。
3. 当前步骤结束后，不再开始下一步，任务进入 `cancelled`。
4. 如果任务正在生成本地 Manifest 或报告，可以在写入前检查取消标记并安全停止。

### 8.2 升级

升级涉及停止集群、切换目录和回滚。首版只允许 `pending`、`ready` 状态取消。进入 `running` 或回滚后返回 `not_cancellable`，并说明当前步骤和影响。

这比设置一个无法保证真实停止的取消标记更安全。后续如果 Agent 支持命令取消，再按步骤增加安全取消点和必要回滚。

### 8.3 同步

同步任务先写 `cancel_requested`，再调用：

- SeaTunnel 引擎停止 API；或
- 本地模式的 `sync_local_stop` Agent 命令。

停止调用成功后进入 `cancelling`。只有查询到引擎状态为取消或本地停止命令明确成功后进入 `cancelled`。请求失败则恢复可观察的运行状态并保存失败原因，不能伪造取消成功。

## 9. 审计

`AuditLog` 增加：

```text
request_id
execution_id
client_type
risk_level
result_status
duration_ms
```

审计详情写入前通过公共脱敏器处理。脱敏器递归处理 map、slice 和结构化 JSON，敏感键至少包括：

```text
password, passwd, pwd, secret, secret_key, access_key,
token, authorization, credential, private_key, storage_secret_key
```

审计不保存请求和响应正文，只保存脱敏后的参数摘要、状态、耗时和返回大小。

普通用户查询审计时强制使用自己的用户 ID；只有管理员能指定其他用户或查询系统记录。详情接口同样执行所有者检查。

任务正文的完整 HOCON/JSON 脱敏和带掩码更新属于单独实现项。本任务先保证新执行记录、审计和提交摘要不保存正文或秘密原文；现有同步任务正文接口需要在秘密处理子阶段完成后才算通过最终验收。

## 10. API

```text
GET  /api/v1/executions/:id
GET  /api/v1/executions/:id/wait?timeout_seconds=30
POST /api/v1/executions/:id/cancel
```

取消响应始终返回当前真实状态、`cancellable` 和原因。重复取消已经取消的任务返回相同结果；已成功或已失败任务返回 `too_late_to_cancel`。

## 11. 并发和失败处理

- 状态更新带旧状态条件，避免迟到的成功回调覆盖取消终态。
- 幂等记录和模块任务创建尽量放在同一数据库事务；无法共用事务时，公共记录失败要标记为失败并允许同键重试恢复。
- `wait` 使用数据库轮询和短等待，不持有全局锁。
- 服务重启后没有内存控制项的 `running` 任务仍可查询，但诊断和升级取消返回 `runtime_not_attached`，不能直接宣称已经取消。

## 12. 兼容性

- 服务尚未正式部署，不做旧数据迁移脚本。
- 现有模块接口继续保留，新增字段只增加响应内容。
- 公共执行 API 是 CLI 的主要入口；网页可以继续使用模块接口，但状态语义必须与公共记录一致。
- 本任务不修改 `.proto`。如果后续增加 Agent 单命令取消，再按项目约定重新生成两个 Go 文件。
