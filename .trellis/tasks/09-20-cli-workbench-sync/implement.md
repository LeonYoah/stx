# 数据同步工作台 CLI 全面覆盖与端到端真实作业验收 实施计划

## 实施阶段与步骤

### 阶段 1：操作契约登记与校验 (Registry & Operations Spec)
- [x] 1. 在 `internal/operation/registry.go` 中完整登记全部 25+ 个 `sync` 路由与操作定义。
  - `sync.task.*`（tree, list, get, create, update, delete, publish, versions, rollback, validate, test-connections, dag, preview, preview.sink-savemode, submit）
  - `sync.job.*`（list, get, logs, checkpoint, preview, cancel, recover）
  - `sync.variable.*`（list, create, update, delete）
  - `sync.plugin.*`（list, options, template, enum-values, enum-catalog）
- [x] 2. 运行 `go test ./internal/operation/...` 确保登记无重复、命令路径合法、Revision 合规。
- [x] 3. 运行路由覆盖检查，确保所有 sync 接口均进入覆盖列表，历史缺口减少至 0。

### 阶段 2：CLI 命令实现 (`internal/cmd/sync.go` 及相关文件)
- [x] 1. 编写 `internal/cmd/sync.go`：
  - 注册 `stx sync` 顶层命令及 `task`, `job`, `variable`, `plugin` 子命令组。
  - 只读命令复用/集成标准 Client 查询与输出。
  - 写命令接入 `secureWriteOptions`，支持 `--confirm`, `--idempotency-key`, `--confirmation-id`。
  - 实现关键参数：
    - `submit` / `preview`：`--wait`, `--detach`
    - `cancel`：`--savepoint`（带保存点安全停止）
    - `recover`：`--draft-file`
    - `logs`：`--lines`, `--all`
- [x] 2. 在 `internal/cmd/root.go` 中注册 `newSyncCommands()`。
- [x] 3. 编写 `internal/cmd/sync_test.go` 单元测试，覆盖各命令参数解析、离线 help、确认拦截与模拟调用。

### 阶段 3：本地服务构建与热更新
- [x] 1. 重新构建 `stx` 二进制 (`go build -o stx main.go`)。
- [x] 2. 使用 PM2 重载/重启 `stx-api`。
- [x] 3. 使用 `stx capability list` 验证远端服务已能发现所有的 `sync.*` 能力。

### 阶段 4：真实 MySQL CDC 与 JDBC 端到端闭环验收
- [x] 1. 本地启动 MySQL 8.0 容器并创建源表与目标表（配置 ROW 格式 binlog）。
- [x] 2. 通过 `stx sync task create` 编写并创建测试任务：
  - 场景 A: MySQL CDC -> MySQL / Console
  - 场景 B: JDBC -> MySQL / Console
- [x] 3. 运行 `stx sync task dag` 验证 DAG 生成。
- [x] 4. 运行 `stx sync task validate` 与 `stx sync task test-connections`。
- [x] 5. 运行 `stx sync task preview --wait` 验证数据采样。
- [x] 6. 运行 `stx sync task submit --confirm --wait` 提交运行并跟踪公共 Execution。
- [x] 7. 运行 `stx sync job logs` 与 `stx sync job checkpoint` 查看作业运行日志与 Checkpoint 指标。
- [x] 8. 运行 `stx sync job cancel --savepoint --confirm` 触发保存点并停止作业。
- [x] 9. 运行 `stx sync job recover --confirm --wait` 验证从保存点恢复作业并继续正常消费。
