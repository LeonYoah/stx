# SeaTunnel 无偏移时间解析

> 适用于根 Go 模块解析引擎 REST / 作业 `finishTime`、诊断日志时间窗，以及 Agent 心跳上报的主机时区。

---

## 概述

SeaTunnel 引擎 REST 与日志里的时间大多**没有时区偏移**。这些时间必须按**节点主机时区**解释后再存 UTC。控制面进程所在主机的时区（容器常为 UTC）不一定等于 SeaTunnel 节点时区。

---

## 优先级

1. 环境变量 `STX_SEATUNNEL_TIMEZONE`（IANA，控制面与 Agent 各自读取；**最高优先级**）。
2. Agent 心跳 `HeartbeatRequest.timezone`：节点主机 IANA 时区，持久化到 `hosts.timezone`。
3. 解析某个**集群**的引擎 REST / `finishTime` 时，取该集群**在线 Master / master_worker**（与 `pickSTXJavaProxyNode` 相同选择）上报的时区；都离线时回退任一 Master 已上报值。
4. 解析某个**具体节点**的诊断日志时，用该节点主机的 `hosts.timezone`（`timex.SeatunnelLocationForHost`）；主机时区为空再回退第 3 条。
5. 仍未知时用控制面 `time.Local`。不要在升级时把缺省值悄悄改成展示时区或上海时区。

旧 Agent 心跳不带 `timezone` 时字段为空。容器部署的控制面 `time.Local` 常为 UTC，相对节点 `Asia/Shanghai` 会把 `finishTime` 偏 8 小时，直到 Agent 升级上报时区，或显式设置 `STX_SEATUNNEL_TIMEZONE`。

---

## 契约

- Agent 探测顺序：`STX_SEATUNNEL_TIMEZONE` → `TZ` → `/etc/localtime` 指向 `zoneinfo/` 的软链接 → `/etc/timezone`。非法名称不上报。
- 控制面 `timex.SeatunnelLocationForCluster(ctx, clusterID)` / `SeatunnelLocationForHost(iana)` 供作业对账与诊断日志使用。作业对账传入调用方 ctx；`cluster.EngineLocation` 在 ctx 没有截止时间时加 2 秒超时（`GetNodes` 会查库）。
- `STX_SEATUNNEL_TIMEZONE` 只覆盖解析，不改展示时区（`app.display_timezone`）和用户偏好时区。
- 旧 Agent 不带 `timezone` 时字段为空，解析回退到上一条规则，不能因此让心跳失败。

---

## 校验

- 单元测试：环境变量压过心跳时区；Master 心跳时区用于该集群作业 `finishTime`；Worker 时区不得覆盖 Master。
- 单元测试：心跳把 IANA 名称写入 `hosts.timezone`，并出现在 `cluster.HostInfo`。
- 单元测试：诊断日志窗口使用该节点主机时区；主机时区为空时回退集群 Master。

---

## 错误与正确写法

错误：用控制面 `time.Local` 或 `Asia/Shanghai` 硬编码解析引擎时间；诊断日志用集群 Master 时区代替该节点主机时区。

正确：作业 `timex.SeatunnelLocationForCluster(ctx, instance.ClusterID)`；诊断日志 `timex.SeatunnelLocationForHost(host.Timezone)`。
