# 设计说明

## 命令与接口

| CLI | HTTP |
| --- | --- |
| `stx host precheck <host-id>` | `POST /api/v1/hosts/:id/precheck` |
| `stx host install start <host-id>` | `POST /api/v1/hosts/:id/install` |
| `stx host install retry <host-id>` | `POST /api/v1/hosts/:id/install/retry` |
| `stx host install cancel <host-id>` | `POST /api/v1/hosts/:id/install/cancel` |
| `stx upgrade precheck <cluster-id>` | `POST /api/v1/st-upgrade/precheck` |
| `stx upgrade plan create <cluster-id>` | `POST /api/v1/st-upgrade/plan` |
| `stx upgrade plan execute <plan-id>` | `POST /api/v1/st-upgrade/execute` |
| `stx upgrade plan get <plan-id>` | `GET /api/v1/st-upgrade/plans/:id` |
| `stx upgrade task list/get/steps/logs` | 对应升级任务查询接口 |
| `stx upgrade task wait <task-id>` | 轮询 `GET /api/v1/st-upgrade/tasks/:id` |

安装请求字段较多，不扩展普通命令生成器。新增 `internal/cmd/host_install.go` 负责 JSON 正文构造，并复用现有安全写请求、确认提示和结果渲染函数。简单查询 `host.install.status.get` 继续由登记表生成。

## 数据流

```text
CLI 参数或 request-file
  -> CLI 生成请求正文和安全请求头
  -> STX installer handler/service
  -> Agent 安装命令
  -> /tmp/seatunnel-2.3.12
  -> 安装状态接口和本地目录验证
```

`--request-file` 提供完整正文；显式命令参数只覆盖用户确实传入的字段，避免零值意外改变安装配置。路径中的 host ID 同时写入安装请求的 `host_id`，服务端仍以路径参数为准覆盖该字段。

安装命令带 `cluster_id` 时，Agent 完成文件安装后，Control Plane 先根据“集群、主机、角色”创建或刷新节点元数据，再复用集群服务启动节点。节点登记或启动失败时，安装状态进入 `failed`，不能继续保留 `success`。安装和启动全部完成后，`current_step` 固定为 `complete`。

升级预检查和计划创建同样支持完整 JSON 文件与显式参数。计划执行使用 R2 一次性确认；CLI 兼容当前升级接口缺少 `error_code` 的 `428 + confirmation_id` 响应。任务等待每两秒读取任务状态，状态变化写入 stderr，终态结果写入 stdout。

升级后的模板任务由 STX 编排侧执行有限重试，不改变 Agent 的单次命令语义。只匹配 `Unable to connect to any cluster`、`connection refused`、`cluster is not ready` 三类暂时错误。第一次失败等待 3 秒，第二次失败等待 5 秒，第三次仍失败则按原有规则记录告警但不阻塞升级。每次重试更新 `UpgradeTaskStep.RetryCount`，日志记录尝试次数、最大次数和等待秒数。

## 安全与兼容

- 预检是只读操作，风险等级为 R0。
- 安装、重试和取消为 R1，要求 `--confirm` 并发送幂等请求头。
- 真实验证只操作主机 10 和独立安装目录，不停止现有 2.3.13 集群。
- 不修改 installer 服务端的旧任务模型和取消语义。

## 同主机多集群进程隔离

Agent 管理的进程标识由安装目录和角色共同生成，格式保留可读前缀，并附加安装目录的短 SHA-256 摘要。例如：

```text
seatunnel@<摘要>
seatunnel-master@<摘要>
seatunnel-worker@<摘要>
```

Control Plane 下发监控配置、集群启停命令和处理心跳时使用同一个标识算法。Agent 的进程管理器查找 PID 或发送停止信号前，必须同时核对 SeaTunnel 主类、安装目录和角色。`check_process` 也接收 `install_dir`，防止启动后的短时检查把同角色的其他集群 PID 写入当前节点。

监控配置中的每个进程同时携带 `cluster_id`。Agent 收到单个集群的配置更新时，只替换该集群的监控项；服务启动后发送的主机级完整列表则可以替换全部监控项。诊断日志扫描目标以 Agent 当前完整监控列表为准，不能因为更新一个集群而移除同机其他集群。

手动停止时先把对应进程标记为人工停止，避免自动拉起立即重新启动它。若不能确认 PID 属于目标安装目录，停止命令必须拒绝处理，不能按角色扫描并停止其他 SeaTunnel 进程。
