# SeaTunnel 无偏移时间解析

> 适用于根 Go 模块解析引擎 REST / 作业 `finishTime`、诊断日志时间窗，以及 Agent 心跳上报的主机时区。

---

## 概述

SeaTunnel 引擎 REST 与日志里的时间大多**没有时区偏移**。这些时间必须按**节点主机时区**解释后再存 UTC。控制面进程所在主机的时区（容器常为 UTC）不一定等于 SeaTunnel 节点时区。

---

## 优先级

1. 环境变量 `STX_SEATUNNEL_TIMEZONE`（IANA，控制面与 Agent 各自读取；**最高优先级**）。
2. Agent 心跳 `HeartbeatRequest.timezone`：节点主机 IANA 时区，持久化到 `hosts.timezone`。
3. 解析某个集群时，取该集群**在线 Master / master_worker**（与 `pickSTXJavaProxyNode` 相同选择）上报的时区；都离线时回退任一 Master 已上报值。
4. 仍未知时用控制面 `time.Local`。不要在升级时把缺省值悄悄改成展示时区或上海时区。

---

## 契约

- Agent 探测顺序：`STX_SEATUNNEL_TIMEZONE` → `TZ` → `/etc/localtime` 指向 `zoneinfo/` 的软链接 → `/etc/timezone`。非法名称不上报。
- 控制面 `timex.SeatunnelLocationForCluster(clusterID)` / `SeatunnelLocationForHost(iana)` 供作业对账与诊断日志使用。
- `STX_SEATUNNEL_TIMEZONE` 只覆盖解析，不改展示时区（`app.display_timezone`）和用户偏好时区。
- 旧 Agent 不带 `timezone` 时字段为空，解析回退到上一条规则，不能因此让心跳失败。

---

## 校验

- 单元测试：环境变量压过心跳时区；Master 心跳时区用于该集群作业 `finishTime`；Worker 时区不得覆盖 Master。
- 单元测试：心跳把 IANA 名称写入 `hosts.timezone`，并出现在 `cluster.HostInfo`。

---

## 错误与正确写法

错误：用控制面 `time.Local` 或 `Asia/Shanghai` 硬编码解析引擎时间。

正确：`timex.SeatunnelLocationForCluster(instance.ClusterID)`。
