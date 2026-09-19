# 补齐集群模块 CLI 与真实写操作验证

## Goal

批量覆盖 cluster 模块的查询、创建、更新、删除及异步操作，完善确认、幂等、执行记录和真实测试。

## Requirements

- 一次批量补齐 cluster 模块的常用 CLI，不再按少量命令分批。
- 覆盖集群 CRUD、节点 CRUD、节点预检查、集群启停重启、节点启停重启、节点日志、STX Java Proxy 状态与操作。
- GET 使用登记表生成命令；带 JSON 正文、确认或幂等要求的接口使用专用命令。
- 所有写命令支持 `--namespace`、`--confirm`、`--idempotency-key` 和 `--confirmation-id`。
- 命令帮助必须写明可能影响集群进程、主机资源或服务可用性的操作。
- 命令成功结果写 stdout，提示、影响说明和确认信息写 stderr。
- 操作必须登记到 operation registry，并保持路由、登记表、Swagger 契约检查可解释。
- 使用本地 STX 服务和测试资源完成真实调用；临时资源验证后清理。
- 不修改工作区中已有的 5 个前端文件，不执行 `git restore`。

## Acceptance Criteria

- [x] cluster 常用读写命令可从 `stx cluster --help` 发现，帮助阶段不访问服务端。
- [x] 集群创建、更新、删除在真实服务中成功，并验证确认和幂等请求头。
- [x] 节点新增、更新、删除和预检查至少在临时测试资源上完成真实调用。
- [x] 集群和节点进程操作具有明确影响提示，并在安全的测试资源上验证成功或记录真实失败原因。
- [x] 新增命令支持 JSON、YAML、table、raw 和 `--pick` 的现有输出约定。
- [x] 操作登记、路由基线、单元测试、`go vet`、完整 Go 测试、构建和 License 检查通过。
- [x] 重新生成 `dist/stx`，软连接调用的是本批次最新二进制。
- [x] 真实验证结果记录到任务文档，提交信息使用中文。

## 真实验证结论

- 验证日期：2026-09-19。
- 新增 20 条 cluster 操作登记，登记总数从 51 增至 71，历史缺口从 174 减至 154。
- 临时集群 ID 7 完成创建、更新、节点预检查、单节点新增/更新/删除、批量新增/删除、空集群启停重启和最终删除。
- 删除后按名称查询为 0 条，`cluster get 7` 返回退出码 5。
- 集群 6 完成节点日志、Java Proxy 状态和日志查询，并真实执行 Java Proxy stop、start、restart；最终状态 healthy，PID 为 48772。
- JSON、YAML、table、raw 和 `--pick` 均完成真实调用。

## 验证发现

- cluster handler 尚未接入公共执行服务。相同创建请求复用同一个幂等键时返回集群名重复，说明服务端还没有实现幂等重放和执行记录；CLI 已正确发送 Header，但不能代替服务端实现。
- 临时节点可能错误关联同一主机上的现有 SeaTunnel PID。本次节点 7/8 被识别为 PID 92322，因此没有对这些节点执行 stop/restart，避免影响集群 6。
- `force_delete` 只表示删除节点安装目录，不能跳过“集群必须先停止”的检查；命令帮助已按真实语义修正。

## Notes

- Keep `prd.md` focused on requirements, constraints, and acceptance criteria.
- Lightweight tasks can remain PRD-only.
- For complex tasks, add `design.md` for technical design and `implement.md` for execution planning before `task.py start`.
