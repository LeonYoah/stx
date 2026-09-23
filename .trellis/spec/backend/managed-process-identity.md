# 受管 SeaTunnel 进程身份约定

## 场景：同一 Agent 管理多个 SeaTunnel 安装目录

### 1. 适用范围

- 同一台主机上存在两个或更多 SeaTunnel 安装目录。
- Control Plane 向 Agent 下发启动、停止、监控、心跳或进程检查命令。
- 目标是避免把其他集群的 PID 写入当前节点，或停止其他集群的进程。

### 2. 签名

- 公共进程名：`processidentity.ManagedName(installDir, role) string`。
- Agent 查找：`process.FindSeaTunnelProcess(ctx, installDir, role)`。
- Agent 校验：`process.MatchesSeaTunnelProcess(ctx, pid, installDir, role)`。
- 监控配置中的每个进程必须包含：`pid`、`cluster_id`、`install_dir`、`role`。
- 集群启动、停止和 `check_process` 必须下发 `install_dir`；启停还要下发 `process_name`。

### 3. 契约

- 受管进程身份由“安装目录 + 角色”共同确定，不能只按角色或固定名称识别。
- Control Plane 和 Agent 必须调用同一个 `ManagedName` 实现，名称格式为 `seatunnel[-role]@<目录摘要>`。
- PID 查找、停止、监控、心跳和 `check_process` 必须使用相同的安装目录与角色条件。
- Agent 接收监控配置时，先保留已经通过身份校验的本地 PID，再校验服务端 PID；都不匹配时才重新扫描。
- `cluster_id` 用于限定一次监控配置更新的范围。更新一个集群时，不得移除同一 Agent 上其他集群的受监控进程。
- 心跳发现 PID 与目标身份不匹配时，必须上报 `stopped` 和 PID `0`，不能上报错误 PID。
- 手动停止前设置 `ManuallyStopped`；实际停止失败时清除该标志。

### 4. 校验与错误表

| 情况 | 行为 |
| --- | --- |
| PID 属于目标目录和角色 | 接受 PID，并上报 `running` |
| PID 存在，但属于其他安装目录 | 拒绝该 PID，继续查找目标进程 |
| 找不到目标进程 | 使用 PID `0`；由现有自动拉起设置决定后续行为 |
| 无法读取系统进程列表 | 保留已知候选 PID并记录警告，避免重复启动 |
| 停止时未找到目标身份 | 返回 `process not found`，不得按角色停止其他进程 |
| 收到某一集群的监控更新 | 只替换该 `cluster_id` 下的监控项 |

### 5. 正常、基础和错误案例

- 正常：`/tmp/seatunnel-2.3.12` 与 `/tmp/seatunnel-2.3.13` 都是混合角色，生成不同名称并保持不同 PID。
- 基础：主机只有一个安装目录，仍使用相同的进程身份算法。
- 错误：2.3.12 节点收到 2.3.13 的 PID。Agent 必须保留或查找到 2.3.12 的 PID，不能产生错误的崩溃或重启事件。

### 6. 必需测试

- 单元测试：不同目录生成不同名称；角色归一化结果稳定。
- 单元测试：进程扫描同时核对 SeaTunnel 主类、安装目录和角色。
- 单元测试：监控配置中的旧 PID 被拒绝，本地正确 PID被保留。
- 单元测试：更新一个 `cluster_id` 后，其他集群的监控项仍存在。
- 集成测试：心跳把两个安装目录的 PID 写回各自节点。
- 真实验证：停止并重新启动其中一个集群，另一个集群的 PID 和端口保持不变。

### 7. 错误与正确写法

错误：

```go
pid := findByRole("master/worker")
stop(pid)
```

正确：

```go
name := processidentity.ManagedName(installDir, role)
pid, _, err := process.FindSeaTunnelProcess(ctx, installDir, role)
```

仅按角色查找或停止会在同机多集群时命中错误进程。安装目录、角色和公共进程名必须一起贯穿命令下发、Agent 执行与状态回写。
