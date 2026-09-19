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
