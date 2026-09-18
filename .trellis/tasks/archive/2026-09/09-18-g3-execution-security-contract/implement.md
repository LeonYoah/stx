# 统一安全执行、任务取消与审计实施计划

## 1. 实施顺序

### 阶段 A：公共执行基础

- [x] 新增 `internal/apps/execution` 模型、Repository、Service 和状态迁移校验。
- [x] 增加公共执行表和确认表迁移。
- [x] 实现幂等键哈希、请求摘要和冲突检查。
- [x] 实现 R0 至 R3 服务端确认检查。
- [x] 实现用户所有者和管理员访问判断。
- [x] 增加 `GET /executions/:id`、`wait` 和 `cancel`。
- [x] 增加状态竞争、重复取消、终态保护和幂等冲突测试。

验收：

```bash
go test ./internal/apps/execution/...
```

### 阶段 B：CLI 公共执行命令

- [x] 增加 `stx execution get`。
- [x] 增加 `stx execution wait`，轮询时进度事件写 stderr，最终结果写 stdout。
- [x] 增加 `stx execution cancel`，支持 `--confirm` 和幂等键。
- [x] 增加认证、无权限、等待超时和取消失败退出码测试。

验收：

```bash
go test ./internal/cmd/... ./internal/cli/...
```

### 阶段 C：同步任务接入

- [x] `JobInstance` 增加公共执行编号和取消中间状态。
- [x] 提交、预览和恢复创建公共执行记录。
- [x] 列表、详情、日志、Checkpoint 和取消按用户过滤。
- [x] 取消先写 `cancel_requested`，调用真实停止后写 `cancelling`。
- [x] 引擎或本地 Agent 确认停止后写 `cancelled`。
- [x] 补充重复取消、停止失败、迟到成功状态和越权测试。

验收：

```bash
go test ./internal/apps/sync/...
```

### 阶段 D：诊断任务接入

- [x] 诊断任务增加公共执行编号和取消中间状态。
- [x] 创建、启动、列表、详情、步骤、日志、预览和下载按用户过滤。
- [x] 取消请求只在安全步骤边界生效。
- [x] 正在运行 Agent 命令时返回 `cancelling`，命令返回后再进入 `cancelled`。
- [x] 自动策略任务使用系统所有者，默认只允许管理员查看。
- [x] 补充可选步骤、Agent 超时和取消竞争测试。

验收：

```bash
go test ./internal/apps/diagnostics/...
```

### 阶段 E：升级任务接入

- [x] 升级任务增加公共执行编号和取消中间状态。
- [x] 计划执行、列表、详情、步骤、日志和事件流按用户过滤。
- [x] `pending`、`ready` 允许取消。
- [x] `running` 和回滚阶段返回稳定 `not_cancellable` 原因。
- [x] 防止取消后后台协程再次写成成功。
- [x] 补充取消窗口、越权和终态竞争测试。

验收：

```bash
go test ./internal/apps/stupgrade/...
```

### 阶段 F：审计和秘密处理

- [x] 扩展审计模型和 Repository 查询字段。
- [x] 增加递归参数脱敏器，并覆盖大小写、嵌套 map、slice 和 JSON。
- [x] 诊断、升级、同步的创建、开始、取消和结果写入统一审计。
- [x] 普通用户只能查询自己的审计，管理员可以查询全部。
- [x] 审计不保存任务正文、提交正文、令牌、密码和密钥原文。
- [x] 为同步任务正文设计并实现可靠的 JSON/HOCON 脱敏和带掩码更新；无法安全处理时拒绝返回原文。

验收：

```bash
go test ./internal/apps/audit/... ./internal/apps/sync/...
```

## 2. 可以同时进行的工作

- 公共执行模型确定后，CLI 命令和审计字段可以分别开发。
- 同步、诊断和升级的适配可以分别修改各自目录，但都依赖阶段 A 的公共接口。
- 秘密脱敏器可以与模块取消流程同时开发，但同步任务接口接入必须等脱敏器测试通过。

当前主会话按阶段顺序实施，不同时修改相同公共文件。

## 3. 高风险文件

- `internal/router/router.go`：服务装配和路由较集中，改动后运行全量路由测试。
- `internal/db/migrator/migrator.go`：新增表后检查 SQLite、MySQL、PostgreSQL 类型兼容。
- `internal/apps/diagnostics/task_execute.go`：不能让取消状态被末尾成功更新覆盖。
- `internal/apps/stupgrade/execute.go`：取消判断不能绕过必要回滚。
- `internal/apps/sync/service.go`：提交正文包含秘密，不能进入公共执行记录或审计。
- `internal/apps/audit/model.go`：新增字段不能破坏现有查询和前端响应。

如需回撤这些改动，必须先征得用户确认，且禁止使用 `git restore`。

## 4. 总体验证

```bash
gofmt -w <本任务修改的 Go 文件>
go test ./internal/apps/execution/... ./internal/apps/audit/... ./internal/apps/diagnostics/... ./internal/apps/stupgrade/... ./internal/apps/sync/... ./internal/cmd/... ./internal/cli/...
go test ./...
go vet ./...
git diff --check
python3 scripts/check_license.py
python3 ./.trellis/scripts/task.py validate 09-18-g3-execution-security-contract
```

## 5. 完成条件

- 三个模块都返回公共执行编号并执行用户归属检查。
- 公共查询、等待和取消命令可以通过真实 `stx` 二进制运行。
- 任何不能真实停止的阶段都不会返回 `cancelled`。
- 相同幂等键不会产生第二次实际执行。
- 审计能通过请求编号和执行编号查询调用链，且不包含秘密原文。
- 全量测试、静态检查、License 检查和 Trellis 校验通过。
