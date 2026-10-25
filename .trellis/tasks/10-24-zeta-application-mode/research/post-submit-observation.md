# 提交后跟进：Application Mode 下能拿到什么

> 调研时间：2026-10-25。上游基线同 `upstream-stip41.md`（dev `9ab97e1`）。

## 前提：Dinky 能监控，靠的是 REST，不是入口

| 能力 | Dinky 依靠什么 | 没有 dinky-app 入口时是否可行 |
| --- | --- | --- |
| 运行期监控、DAG、指标、checkpoint、savepoint | Flink 在 application 模式下照常开放 REST，Dinky 从集群外通过 webURL 或 Ingress 读取 | ✅ 可以 |
| 终态快照 | dinky-app 阻塞 main，在作业结束时同步调用 `hookJobDone` | ❌ 不行 |
| K8s 按作业加载依赖 | dinky-app 把依赖下载到 usrlib | ❌ 不行 |
| 读取 SQL 和任务配置 | dinky-app 直连 Dinky DB | ❌ 不行 |

Zeta 的情况：
- master 的 main class 固定为 `SeatunnelKubernetesApplicationCli`，容器 command 会被覆盖，所以**没有入口**。
- REST 被强制关闭，所以**也没有 REST**。

## “常驻小集群”能解决什么，不能解决什么

- **能解决提交前预检查。** proxy 有了宿主，配置级 DAG、Catalog 探测、预览、存储探测都能用。
  - 这些能力运行在 proxy 自己的 JVM 里，只需要 `SEATUNNEL_HOME` 发行包（`scripts/stx-java-proxy.sh` 只检查 `starter/seatunnel-starter.jar` 和 `bin/seatunnel.sh`），**不需要 Zeta 进程在运行**。
  - STX 现有的数据模型按 `cluster_id` 寻址 proxy（`executeSTXJavaProxyCommand(ctx, clusterID, cmd)`），所以需要有一条“集群记录”，但这台机器可以只是网关主机。
- **不能解决提交后的运行期数据。**
  - application 是一个独立的 Hazelcast 集群（`clusterName = "seatunnel-application-" + applicationId`，`SeatunnelApplicationConfig.java:252`），常驻小集群的 REST 并不知道它的存在。
  - STX 只在 agent 主机上部署集群（`HostTypeKubernetes` 只在 `internal/apps/host` 中使用，`internal/apps/cluster` 没有用到），所以 proxy 在 Pod 网络之外，`retrieve()` 用的 Pod IP 连不上。

## 不改上游时，提交后的可选手段

| 跟进内容 | 手段 | 前提 | 结论 |
| --- | --- | --- | --- |
| 状态、终态、取消 | `getApplicationStatus` / `cancelApplication`，或由 Go 直接调用 K8s API | 能访问 apiserver | ✅ 确定可行 |
| 日志 | K8s `pods/log`（master 和全部 worker） | 同上 | ✅ |
| 事件推送（相当于 Dinky 的 hook） | 在 seatunnel.yaml 中配置 `event-report-http.url` 指向 STX，通过 `kubernetes.seatunnel-config-file` 和 ConfigMap 挂载进 Pod。`SeatunnelApplicationConfig.configure()`（L271-317）只覆盖 join、角色、HTTP、jar storage 和 slot，**没有覆盖事件上报**；`CoordinatorService.java:651` 照常根据 `eventReportHttpApi` 创建 processor | Pod 能反向访问 STX 控制面 | ⚠️ 代码上可行；事件是 reader/writer 生命周期和 schema change 一类，**不是指标**；能否覆盖作业状态变化待验证 |
| 结束后的 DAG、指标、终态 | master 结束作业时会写入 `IMAP_FINISHED_JOB_STATE` / `METRICS` / `VERTEX_INFO`（`CoordinatorService.java:635-649`、`:2014`）。如果 master 的 hazelcast 配置（`kubernetes.master.hazelcast-config`）开启了 IMap 持久化到 S3/HDFS，proxy 可以在事后用 `IMapStorageFactory` 读回（proxy 已有 IMap 探测代码） | 写入发生在 master 退出之前；持久化路径按 clusterName 区分 | ⚠️ 推测，需要在 k3s 上验证写入时机和反序列化 |
| 运行中的实时指标和 DAG | 原生客户端连接 master 的 Pod IP | proxy 必须在 Pod 网络内 | ❌ proxy 在集群外时做不到 |

## 要拿到实时监控，只有两条路

1. 上游在 Application Mode 下提供可选的 REST，Go 通过 apiserver 的 `pods/{name}:{port}/proxy` 访问。这条路与 proxy 部署在哪里无关。
2. proxy 部署进目标 K8s 的 Pod 网络（集群内 proxy）。
