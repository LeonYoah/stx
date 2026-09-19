# 验证记录

## 真实环境

- STX：`127.0.0.1:17800`，gRPC：`127.0.0.1:17890`。
- Agent：`agent-7f0fb54dfdcd7edf`，主机 ID `10`。
- SeaTunnel 2.3.13：`/tmp/seatunnel-2.3.13`，PID `92322`，端口 `15801`、`18080`。
- SeaTunnel 2.3.12：`/tmp/seatunnel-2.3.12`，PID `60355`，端口 `15812`、`18092`。

## 已验证行为

- 停止集群 8 后只有 PID `55203` 退出，PID `92322` 和 2.3.13 端口保持正常。
- 再次启动集群 8 后得到新 PID `60355`，命令行包含 `/tmp/seatunnel-2.3.12`。
- 将节点 9 的数据库 PID 临时改成错误的 `92322` 后推送集群 8 监控配置，Agent 仍保留正确 PID `60355`。
- 更新集群 8 的监控配置后，集群 6 仍在 Agent 监控列表中，诊断扫描目标数量保持为 2。
- 下一次心跳把节点 9 的数据库 PID 自动纠正为 `60355`。
- 两个 SeaTunnel 进程和四个监听端口在验证期间保持正常。

## 检查结果

- Agent 全量测试通过。
- 根模块除其他任务新增的 `POST /api/v1/clusters/:id/runtime-storage/:kind/apply` 尚未登记外，其余测试通过。
- 根模块 `go vet ./...` 通过。
- Agent `go vet ./...` 仍命中已有测试 `internal/diagnostics/error_collector_test.go` 复制 protobuf 锁对象的问题，本任务未修改该测试。
- 数据库兼容检查、License 检查和 `git diff --check` 通过。
- 仓库不存在 `src/templates/markdown/spec/`，因此没有可同步的规范模板目录。

## SeaTunnel 2.3.13 真实升级

- 使用新打包的 `dist/stx` 和软连接 `/Users/mac/.local/bin/stx` 执行全部升级操作。
- 升级预检查请求编号：`164efdab-04e2-4113-9241-a00cc20feb2a`，结果 `ready=true`。
- 4 个配置冲突全部采用 2.3.12 当前配置：`hazelcast-client.yaml`、`hazelcast.yaml`、`jvm_options`、`seatunnel.yaml`。
- 升级计划 ID：`1`；升级任务 ID：`1`；公共执行 ID：`b211efa6-f3d6-4ddd-b880-e642b6531d33`。
- `stx upgrade task wait 1` 返回 `succeeded`，当前步骤为 `COMPLETE`；步骤和日志接口均可由 CLI 查询。
- 集群 8 元数据已更新为版本 `2.3.13`、目录 `/tmp/seatunnel-2.3.13-new`。
- 新进程 PID 为 `36516`，命令行、日志和配置均来自 `/tmp/seatunnel-2.3.13-new`；端口 `15812`、`18092` 正常监听，HTTP 返回 200。
- 旧目录 `/tmp/seatunnel-2.3.12` 保留，旧 PID `60355` 已停止。
- 集群 6 PID `92322` 未变化，端口 `15801`、`18080` 正常监听，集群状态健康。
- 升级内置模板任务第一次因新进程尚未完成 Hazelcast 初始化而告警；集群稳定后使用同一模板再次执行成功，读取 32 条、写入 32 条、失败 0 条。
- CLI 真实请求发现升级接口的 `428` 响应没有 `error_code`；已让公共写命令兼容 `HTTP 428 + confirmation_id`，并补充测试。

## 安装节点登记修复

- 新建集群 9：`CLI升级重试验证`，源目录 `/tmp/seatunnel-2.3.12-retry`，端口 `15822`、`15823`、`18099`、`18100`。
- 首次真实安装显示所有安装步骤成功，但启动阶段报告找不到集群节点；检查确认集群 9 的节点列表为空。
- 修复后再次通过 `stx host install start 10` 安装，STX 自动创建节点 10，并启动 PID `77783`。
- 节点记录中的安装目录和三个 SeaTunnel 端口与 CLI 参数一致，集群状态为 `healthy`。
- 登记或启动失败现在会把安装状态写为 `failed`；成功完成后 `current_step` 写为 `complete`。
- 使用 `/tmp/seatunnel-2.3.12-current-step` 完成一次不绑定集群的真实安装，状态为 `success`、`current_step=complete`，并明确提示未提供集群 ID 因而跳过启动。

## 升级模板任务真实重试

- 集群 9 的升级计划 ID 为 `2`，升级任务 ID 为 `2`，公共执行 ID 为 `43ffc66a-0b80-4148-bdf4-64087e011ec9`。
- 集群从 2.3.12 升级到 2.3.13，目标目录为 `/tmp/seatunnel-2.3.13-retry`，源目录 `/tmp/seatunnel-2.3.12-retry` 保留。
- 模板任务第 1 次执行失败后等待 3 秒，第 2 次失败后等待 5 秒，第 3 次执行成功。
- `SMOKE_TEST` 步骤状态为 `succeeded`，`retry_count=2`，步骤消息明确说明第 3 次尝试通过。
- 两条重试日志均带 `attempt`、`max_attempts=3`、`retryable=true`、`retry_delay_seconds` 元数据。
- 升级后 PID 为 `81618`，端口 `15822`、`18099` 正常，集群状态为 `healthy`。
- 验证后集群 6、8、9 均为 `healthy`，PID 分别为 `92322`、`36516`、`81618`。

## 最终检查补充

- `go test ./internal/apps/cluster ./internal/apps/installer ./internal/apps/stupgrade ./internal/cmd ./internal/operation ./internal/router` 通过。
- 根模块 `go test ./...` 仅失败于其他任务新增但尚未登记的 `POST /api/v1/clusters/:id/runtime-storage/:kind/apply`。
- 根模块 `go vet ./...`、Agent `go test ./...`、数据库兼容检查、License 工作区检查和 `git diff --check` 通过。
- Agent `go vet ./...` 仍失败于已有的 `internal/diagnostics/error_collector_test.go:51`，原因是测试复制了包含 protobuf 锁的值，本任务未修改该文件。
