# 主机安装 CLI 与 SeaTunnel 2.3.12 真实验证

## Goal

补齐主机预检、安装、重试、取消 CLI，并通过本机 STX/Agent 将 SeaTunnel 2.3.12 安装到 /tmp/seatunnel-2.3.12 后完成真实验证。

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
- 不修改当前工作区中用户已有的 cluster 模块文件。

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
- [ ] 完整 Go 测试、静态检查、License 检查和 `dist/stx` 构建通过。

## Notes

- 当前安装取消接口仍是旧实现，本任务只暴露现有能力并记录真实行为；若验证发现它会错误宣称已经停止实际执行，则单独报告，不扩大本任务范围。
