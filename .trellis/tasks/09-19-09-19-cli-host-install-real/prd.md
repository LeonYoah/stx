# 主机安装 CLI 与 SeaTunnel 2.3.12 真实验证

## Goal

补齐主机安装与 SeaTunnel 升级 CLI，并通过本机 STX/Agent 完成 2.3.12 安装和 2.3.12 到 2.3.13 的真实升级验证。

## Requirements

- 新增 `stx host precheck <host-id>`，支持通过参数或完整 JSON 文件传入预检条件。
- 新增 `stx host install start <host-id>`，至少支持版本、安装目录、安装模式、部署模式、节点角色、端口和完整 JSON 文件。
- 新增 `stx host install retry <host-id>` 与 `stx host install cancel <host-id>`。
- 写操作沿用 CLI 的 `--confirm`、幂等键、确认编号、stdout/stderr 和能力检查约定。
- 操作登记必须覆盖对应服务端路由，不能新增未登记路由缺口。
- 使用本机 STX、Agent 和主机 `10` 完成真实安装，不创建临时 STX 服务。
- SeaTunnel 2.3.12 安装目录固定为 `/tmp/seatunnel-2.3.12`，不得覆盖 `/tmp/seatunnel-2.3.13`。
- 同一主机存在多个 SeaTunnel 安装目录时，Agent 必须按“安装目录 + 角色”识别、监控、启动和停止进程，不能把其他集群的 PID 写入当前节点。
- 停止 `/tmp/seatunnel-2.3.12` 时只能处理该目录对应的进程，不能停止 `/tmp/seatunnel-2.3.13`。
- 新增 `stx upgrade precheck/plan create/plan execute/task list/task get/task steps/task logs/task wait`。
- 升级执行沿用 R2 二次确认、幂等键、影响提示、stdout/stderr 和能力检查约定。
- 使用本机集群 `8` 从 SeaTunnel 2.3.12 升级到 2.3.13，目标目录固定为 `/tmp/seatunnel-2.3.13-new`。
- 配置冲突采用源集群当前配置，保留集群 8 的端口和 JVM 参数。
- 升级后保留 `/tmp/seatunnel-2.3.12`，且不得影响集群 `6` 的 `/tmp/seatunnel-2.3.13` 进程。
- 安装请求带 `cluster_id` 时，安装完成后必须创建或刷新对应集群节点，再启动该节点；登记或启动失败不能返回成功状态。
- 升级后的模板任务只对“集群暂未就绪”类连接错误有限重试，默认等待 3 秒、5 秒，最多执行 3 次；每次重试写日志并更新步骤重试次数。
- 不修改或提交当前工作区中其他任务已有的 `internal/router/router.go` 和 `internal/router/agent_command_audit_test.go` 改动。

## Acceptance Criteria

- [x] 四类安装命令能在帮助中显示影响说明、输出样例和正确参数。
- [x] CLI 单元测试覆盖请求路径、正文、确认参数、幂等请求头和结果输出。
- [x] 操作登记校验与路由报告通过，对四条路由不再标记为历史缺口。
- [x] 2.3.12 安装包进入 STX 本地包列表。
- [x] 主机预检通过，安装任务在真实 Agent 上完成。
- [x] `/tmp/seatunnel-2.3.12` 中存在可识别的 SeaTunnel 2.3.12 文件和主要目录。
- [x] 现有 `/tmp/seatunnel-2.3.13` 集群继续运行，进程和目录未被覆盖。
- [x] 同一 Agent 同时管理两个混合部署节点时使用不同的进程标识，心跳和进程事件能写回正确节点。
- [x] 2.3.12 启动后得到独立 PID，命令行包含 `/tmp/seatunnel-2.3.12`，并监听配置的端口。
- [x] 2.3.12 停止后，2.3.13 的 PID 和监听端口保持不变；再次启动 2.3.12 能恢复运行。
- [x] 升级命令帮助包含参数、影响说明和输出样例，单元测试覆盖正文、安全请求头、确认兼容和任务等待。
- [x] 升级预检查、计划创建、R2 确认、执行、等待、步骤和日志全部通过新 `stx upgrade` 命令完成。
- [x] 集群 8 已升级为 2.3.13，安装目录为 `/tmp/seatunnel-2.3.13-new`，进程 PID 为 `36516`，端口 `15812`、`18092` 正常。
- [x] `/tmp/seatunnel-2.3.12` 保留，旧 PID `60355` 已停止；集群 6 的 PID `92322` 和端口 `15801`、`18080` 未变化。
- [x] 升级后模板任务再次执行成功，读取 32 条、写入 32 条、失败 0 条。
- [x] 集群 9 的真实安装会自动创建节点 10，并启动 `/tmp/seatunnel-2.3.12-retry`，无需先手工添加节点。
- [x] 集群 9 升级到 `/tmp/seatunnel-2.3.13-retry` 时，模板任务前两次因集群未就绪失败，第 3 次成功，步骤 `retry_count=2`。
- [x] 永久错误不重试，连续暂时错误最多执行 3 次，测试覆盖日志元数据和最终告警。
- [ ] 完整 Go 测试、静态检查、License 检查和 `dist/stx` 构建通过。

## Notes

- 当前安装取消接口仍是旧实现，本任务只暴露现有能力并记录真实行为；若验证发现它会错误宣称已经停止实际执行，则单独报告，不扩大本任务范围。
