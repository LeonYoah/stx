# Research: SeaTunnel K8s 双模式源码事实与 STX 架构缺口审计

> 调研日期：2026-11-25
> 调研对象：
> - 本地 SeaTunnel 源码（已更新 `dev` 至 `upstream/dev @ f08c722acb`，并核对最新 Master HTTP 发布设计 `LeonYoah/seatunnel#6`）
> - STX 本地代码库（`internal/apps/{host,cluster,sync,monitoring,config,plugin,diagnostics}`、`internal/seatunnel`、`internal/router`）
> - 云端 Devin Minikube 实测环境（`devin-01566d0d871141b9841e24249d7b6701`，Kubernetes v1.37.0，SeaTunnel 3.1.0）

---

## 1. SeaTunnel 两种 K8s 模式的源码级事实

### 1.1 常驻容器模式（Helm / StatefulSet）

来源：`seatunnel/deploy/kubernetes/seatunnel/`、`docs/zh/getting-started/kubernetes/{helm,separated-cluster-mode,operations}.md`

- **拓扑建议**：默认 Helm Chart 渲染为 `Deployment`，但官方文档明确标注**生产推荐 Master 与 Worker 均使用 `StatefulSet` + Headless Service**（云端 Minikube 实际部署的就是 `statefulset.apps/seatunnel-master` 1/1 + `statefulset.apps/seatunnel-worker` 2/2）。
- **Hazelcast 集群发现**：通过 `hazelcast.network.join.kubernetes.enabled: true` + Headless Service DNS（如 `seatunnel.seatunnel.svc.cluster.local:5801`）。
- **REST API 与指标端口（重要变更！）**：
  - `deploy/kubernetes/seatunnel/conf/hazelcast-master.yaml` 与 `hazelcast-worker.yaml` **默认关闭了 5801 端口的 REST API v1**（`rest-api.enabled: false`），因为 v1 无鉴权且已废弃。
  - Master 的 Jetty HTTP 服务监听 **8080**（`seatunnel.engine.http.enable-http: true, port: 8080`），同时承载 REST API v2（`/submit-job`, `/job-info/:id`, `/stop-job`, `/overview`, `/metrics/realtime/jobs`, Web UI）和 Prometheus 指标（`/metrics` 与 `/openmetrics`）。
  - Helm `values.yaml` 的 Pod 注解也已切到 `prometheus.io/port: "8080"`, `prometheus.io/path: "/metrics"`。
- **优雅退出**：容器 `command` 直接执行 `/opt/seatunnel/bin/seatunnel-cluster.sh`（不套 `/bin/sh -c`）让 JVM 作为 PID 1 接收 SIGTERM；推荐 `terminationGracePeriodSeconds: 120`（需大于 `hazelcast.graceful.shutdown.max.wait`）。

### 1.2 App 模式（STIP-41 Native Zeta Application Mode on Kubernetes）

来源：`seatunnel/seatunnel-resource-managers/kubernetes/`、`seatunnel/seatunnel-engine/seatunnel-engine-common/.../ApplicationSpecification.java`、`docs/zh/getting-started/application/kubernetes/`

- **资源创建原子顺序**（`KubernetesApplicationClusterDescriptor.java` & `ApplicationJobFactory.java`）：
  1. 校验用户引用的运行时 ConfigMap/Secret 存在且包含声明的 key；
  2. 创建 `suspend: true, backoffLimit: 0, completions: 1, parallelism: 1, ttlSecondsAfterFinished` 的 owner `batch/v1 Job`；
  3. 取到 Job UID 后，创建带 `OwnerReference -> Job` 的 **Headless Service**、可选的 **Application Secret**（`0400` 挂载到 `/etc/seatunnel-secrets/application`）及可选的自建 **ConfigMap**；
  4. 依赖资源全部被 API Server 确认后，Patch Job `suspend: false` 启动 Master Pod，提交立刻返回；
  5. Master Pod 通过 Kubernetes API 创建固定数量 `workerCount` 个 Worker Pods（带 `OwnerReference -> Job` 及标签 `seatunnel.apache.org/application-id`、`seatunnel.apache.org/role=worker`，Worker Pod 不挂载 ServiceAccount token）；
  6. Worker 通过 Master Pod IP 以 Hazelcast TCP 加入并注册固定 Slot（总容量 = `workerCount × worker.slots`，Master 不提供 Slot）；
  7. 作业进入终态后，Master 清理 Worker Pods 并以对应退出码退出，形成 Job 的 `Complete` 或 `Failed` condition，保留 `finished-job-retention-seconds` 供排障。
- **配置序列化格式（关键发现）**：
  - `SpecificationEnvDecorator.java` 将 `KubernetesApplicationConfig.write(writer)` 输出做 `Base64(UTF-8)` 写入环境变量 `SEATUNNEL_APPLICATION_SPECIFICATION`（上限 96 KiB）。
  - `KubernetesApplicationConfig.write(writer)` 写入的是**标准 `java.util.Properties` 键值对**（`application.*` 和 `kubernetes.*`），不涉及 Java 二进制序列化！
  - 当设置了 `kubernetes.job-config-file`（例如指向挂载的 Secret 文件 `/etc/seatunnel-secrets/application/job.conf`）时，`write()` 会主动剔除内联 `jobConfig`，彻底避免凭据暴露在 Job Spec 中以及突破 96 KiB 限制。
- **状态判定逻辑**（`KubernetesApplicationClient.java`）：
  - `Job.isFailed()` → `FAILED`
  - `Job.isComplete()` → `SUCCEEDED`
  - `Job.isActive()` 且存在 Running Worker Pod → `RUNNING`，否则 → `DEPLOYING`
  - `404 NotFound`（已取消或 TTL 过期）→ `UNKNOWN`
- **取消与恢复契约**：
  - 官方 `cancel` 删除 owner Job，触发 OwnerReference 级联清理，**不做 Savepoint**；引用的外部 ConfigMap/Secret/Checkpoint PVC 不受影响。
  - **恢复强约束**：`ApplicationSpecification` 构造器强制校验 `restoreJobId != jobId`（`IllegalArgumentException`）。因此 App 模式恢复必须生成全新正整数 `application.job-id`，并将历史作业 ID 填入 `application.restore-job-id`。
- **Master HTTP 发布机制（最新方案：`LeonYoah/seatunnel#6`，替代旧 `apache/seatunnel#13277`）**：
  - **职责彻底解耦**：Master 是否开启 HTTP 及实际绑定端口，**只由 Master Pod 内加载的 `seatunnel.yaml` 决定**（发行包默认 `enable-http: true, port: 8080`；Worker 不提供 HTTP）。**已移除**旧的 `kubernetes.master.enable-http` 与 `kubernetes.master.http-port` 引擎覆盖项，提交端也无需解析 `seatunnel.yaml`（即使来自外部 referenced ConfigMap 也完全成立）。
  - **新配置项 `application.master.http-port`**：位于 `ApplicationSpecification`（整数 `1..65535`，**强制校验不能与 `application.master.port` 默认 `5801` 相同**）。它**只控制 Kubernetes Service 对外发布的 `port`**，绝不修改引擎监听配置；不设置则不发布 HTTP Service 端口。
  - **两阶段 Service 发布（Master 自 Patch）**：
    1. 提交端创建的初始 Headless Service **只包含 Hazelcast 端口（`application.master.port`，默认 `5801`）**，Job Master Pod 也不预先声明 HTTP `containerPort`；
    2. Master 启动且 Jetty 完成绑定后，读取真实绑定监听端口并直接以 `application/json-patch+json` Patch Headless Service（`PATCH /api/v1/namespaces/{ns}/services/{appId}`），追加 `{"name":"http","port":<application.master.http-port>,"targetPort":<boundListenerPort>,"protocol":"TCP"}`（Service `port` 与真实监听 `targetPort` 允许不同，例如 `28080 -> 18080`）；
    3. 若配置了 `application.master.http-port` 但 `seatunnel.yaml` 未开启明文 HTTP，Application 启动直接快速失败。
  - **RBAC 与安全边界**：
    - 仅当配置了 `application.master.http-port` 时，**Master ServiceAccount 需要对 `services` 具备 `patch` 权限**（除常规 `pods: create/list/watch/delete/deletecollection`、`jobs: get` 之外）；
    - 注意：即使未设置 `application.master.http-port`，使用发行包默认 `seatunnel.yaml` 时 Master Pod 内 Jetty 仍会监听明文 HTTP，生产环境需结合 `seatunnel.yaml` 鉴权或 NetworkPolicy 管控 Pod IP 直连访问。

---

## 2. STX 现有代码库缺口审计

| 文件位置 | 现状实现 | K8s 接入缺口 / 必须修改点 |
| --- | --- | --- |
| `internal/apps/host/model.go` / `service.go` | 已定义 `HostTypeKubernetes` 及 `K8sAPIURL`/`K8sNamespace`/`K8sKubeconfig`/`K8sToken`/`K8sVersion` | ① 缺少 `in_cluster` 认证模式；② `K8sKubeconfig`/`K8sToken` 明文存储且更新空串会覆盖旧值（违反跨层指南错误九）；③ 缺少真连接测试与 RBAC 预检 |
| `internal/apps/cluster/model.go` | 只有 `DeploymentMode`（`hybrid`/`separated`），隐含全为物理机 | 需正交增加 `InfraType`（`bare_metal` / `kubernetes_session` / `kubernetes_application`）与 `K8sHostID` |
| `internal/apps/cluster/health.go` | `checkMemberAgreement` 与 `checkEngineReadiness` 全部通过 `s.agentSender.SendCommand` | K8s 集群无 Agent，需改为 Pod Ready 判定 + 经 `K8sTransport` 直接探测引擎 `/overview` |
| `internal/apps/cluster/webui_master.go` | `webUIProxyCandidates` 按 `NodeInfo.HostIP:APIPort` 拼接 HTTP 地址直连 | K8s 集群需支持经 `K8sTransport` 代理到各 Master Pod，探测 `/metrics/realtime/jobs` 选出 Active Master |
| `internal/apps/cluster/stx_java_proxy.go` | 由 Agent 在宿主机拉起 `stx-java-proxy` 进程 | K8s 无 Agent，需支持同 Namespace 伴生 Deployment 托管或显式 `java_proxy_url` 配置（及缺失时优雅降级） |
| `internal/apps/sync/engine_client.go` | `DefaultClusterRuntimeResolver.ListEngineEndpoints` 硬检查 `hostObj.AgentID != ""` 和主机心跳 | K8s 集群需分发到 `K8sRuntimeResolver` + 自定义 `http.RoundTripper`（`K8sTransport`） |
| `internal/apps/sync/runtime_support.go` | `GetJobLogs` 走 Agent `sync_job_logs` 读本地文件 | K8s 常驻与 App 模式均改为 K8s `Pods().GetLogs()`（支持按 Pod 切换与增量 cursor；常驻集群支持优先引擎 REST `/logs/:jobId`） |
| `internal/apps/sync/job_reconciler.go` | 仅按 `local` vs `cluster` 分支 | 需增加 `k8s_application` 批量对账分支（按 `cluster_id + namespace` 一次 `ListJobs + ListPods`，再按需拉开启 HTTP 的 Master `/job-info` 并采集 K8s Events） |
| `internal/apps/monitoring/service.go` | `collectManagedMetricsTargets` 硬编码抓取 `HostIP:HazelcastPort (5801)` | SeaTunnel 3.x K8s 默认关闭 5801 v1，需支持按集群配置的 `metrics_port`（默认 8080）与 `metrics_path`（`/metrics`）经 `K8sTransport` 抓取 |
| `internal/apps/monitoring/availability_evaluator.go` | 评估 `agent_offline` / `node_offline` / `master_unavailable` / `worker_insufficient` | K8s 集群必须跳过 `agent_offline`，改为基于 K8s Pod 状态与 API Server 连通性评估 |
| `internal/seatunnel/capabilities.go` | 记录版本能力（动态 slot、JDK 要求等） | 补充 `SupportsK8sApplicationMode`（≥3.1.0）与 Master HTTP 能力判定 |
| `go.mod` | 尚未引入 `k8s.io/client-go` | 需在底座阶段引入 `k8s.io/client-go` 与 `k8s.io/api` / `k8s.io/apimachinery` |

---

## 3. 云端 Minikube 环境基线实测记录

- **集群概况**：Minikube v1.37.0（Docker driver，节点 IP `192.168.49.2`，API Server `https://192.168.49.2:8443`）。
- **已部署常驻集群**（Namespace `seatunnel`，Chart `seatunnel-helm-3.1.0`，镜像 `apache/seatunnel:3.1.0`）：
  - `statefulset.apps/seatunnel-master`（1/1，`seatunnel-master-0`，挂载 `seatunnel-configs` ConfigMap + `seatunnel-data` PVC）
  - `statefulset.apps/seatunnel-worker`（2/2，`seatunnel-worker-0/1`）
  - Services：`seatunnel`（Headless 5801）、`seatunnel-master`（Headless 8080）、`seatunnel-worker`（Headless 5801）
- **配置确认**：`seatunnel-configs` 中 `hazelcast-master.yaml` 与 `hazelcast-worker.yaml` 均为 `rest-api.enabled: false`；`seatunnel.yaml` 为 `http.enable-http: true, port: 8080`，`telemetry.metric.enabled: true`。
- **实测发现与 `apiserver_proxy` 方案验证结论**：
  - **✅ `apiserver_proxy` 实测打通**：重建 `seatunnel-master-0` 后，通过 K8s API Server 原生代理**零 Ingress / 零 NodePort / 零 port-forward** 直接拿到 REST v2 响应：
    - `GET /api/v1/namespaces/seatunnel/pods/http:seatunnel-master-0:8080/proxy/overview` → `200 OK`（`{"projectVersion":"3.1.0","gitCommitAbbrev":"2d3983a",...}`）
    - `GET /api/v1/namespaces/seatunnel/services/http:seatunnel-master:8080/proxy/overview` → `200 OK`
    - `GET /api/v1/namespaces/seatunnel/pods/http:seatunnel-master-0:8080/proxy/system-monitoring-information` → `200 OK`（含堆内存、GC、CPU、isMaster 等监控字段）
    - `GET /api/v1/namespaces/seatunnel/pods/http:seatunnel-master-0:8080/proxy/metrics` & `/openmetrics` → `200 OK`（注意：单节点未完成选主 / 集群未就绪时 `/openmetrics` 返回 `# EOF`，`/metrics/realtime/jobs` 返回 `404`；与 STX 现有 `ErrEngineNotMaster` 和 `node_state` 缺样本检测完全吻合！）
  - **✅ 印证双层健康探活必要性**：云端容器长时间休眠唤醒后，旧 `seatunnel-master-0` Pod 保持 `Pod.Ready == True` 但 JVM 假死（Worker 心跳失败且 8080 无响应，删除时等待 `terminationGracePeriodSeconds` 长时间 `Terminating`）。说明 STX 对 K8s 集群**不能只看 Pod Ready，必须结合 K8s Pod 状态 + 经 `K8sTransport` 探测引擎 `/overview` 双重判定**！
  - **⚠️ Worker 指标与端口需在开发期进一步核实**：`seatunnel-worker-0` 在 `hazelcast-worker.yaml` 关闭了 5801 REST v1，默认也未启用 8080 Jetty HTTP；K8s 模式下需明确 Master 8080 `/metrics` 与 `/system-monitoring-information` 各自的指标覆盖面，以及是否需要为 Worker 启用指标端口或通过 Master 聚合视图展示。

---

## 4. `stx-java-proxy` 全量能力与 K8s 伴生加载机制调研

### 4.1 `stx-java-proxy` 承载的全部 21 个接口（无它时缺失清单）

来源：`tools/stx-java-proxy/.../StxJavaProxyServer.java`、`agent/internal/installer/stx_java_proxy_launch.go`、`PluginClassLoaderUtils.java`

- **数据同步工作台（11 个接口）**：
  - `POST /api/v1/config/validate`：HOCON/JSON 静态校验、OptionRule 校验、数据源连通性测试（`testConnection`）
  - `POST /api/v1/config/dag`、`/api/v1/config/webui-dag`：编辑器实时 DAG、作业详情「表结构 / 建表计划 DAG（`/jobs/:id/plan-dag`）」
  - `POST /api/v1/config/preview/source`、`/transform`：数据预览配置派生（注入 `HttpSink` 回调）
  - `POST /api/v1/config/preview/sink-savemode`：Sink SaveMode 建表/删表 DDL 预演
  - `POST /api/v1/catalog/probe`：Catalog 探测（库、表、字段 Schema）
  - `POST /api/v1/plugin/list`、`/options`、`/template`、`/enum-values`：扫描集群已装插件、读取 OptionRule 表单结构、渲染模板、拉取枚举值
- **运行时存储与 Checkpoint / IMap 检查（10 个接口）**：
  - `POST /api/v1/storage/{checkpoint,imap}/{probe,stat,list,preview}`：存储连通性、容量统计、文件列表与预览
  - `POST /api/v1/storage/checkpoint/inspect`、`inspect-source-state`：反序列化 Checkpoint 快照与流式 Source 位点（CDC binlog/LSN、Kafka offset）
  - `POST /api/v1/storage/imap/inspect-wal`：检查 IMap WAL 状态
- **无 Proxy 时的降级边界**：文本编辑 HOCON/JSON、保存发布、直接提交运行、对账、日志、引擎指标/背压、停止/Savepoint/恢复、扩缩容、ConfigMap 管理仍可用；上述 21 项智能辅助与存储深度检查不可用。

### 4.2 容器内连接器 JAR 加载原理与「272 KB ConfigMap `binaryData`」伴生方案

- **实测资产大小与容器环境**：
  - 本地 `lib/stx-java-proxy-v2.jar` = **270 KB**，`lib/stx-java-proxy-v3.jar` = **272 KB**（远小于 K8s `ConfigMap` 1 MiB 上限）。
  - 云端 `apache/seatunnel:3.1.0` 容器实测：内置 Temurin OpenJDK 11（含 `jcmd`/`jstack`、`curl`/`wget`），`/opt/seatunnel/connectors`（~3.1 GB 连接器）与 `/opt/seatunnel/lib`（~425 MB JDBC 驱动及云存储 SDK）均在镜像内。
- **Classpath 加载机制**（`stx_java_proxy_launch.go` + `PluginClassLoaderUtils.java`）：
  - 启动 classpath：`<SEATUNNEL_HOME>/lib/*:<SEATUNNEL_HOME>/starter/seatunnel-starter.jar:<proxy.jar>`。
  - 运行期每次请求动态扫描 `$SEATUNNEL_HOME/{connectors,plugins,lib,starter,seatunnel-dist}` 及 `STX_CONNECTOR_PATHS`，按「路径 + 文件大小 + mtime」指纹缓存 `CachedPluginClassLoader`，共享卷上插件变更自动热加载无需重启。
- **推荐 K8s 伴生部署方案（零定制镜像、零外网依赖、零侵入用户 Master/Worker Pod）**：
  1. STX 用 `client-go` 将对应代际的 `stx-java-proxy-v{2,3}.jar`（~272 KB）写入目标 Namespace 的 `ConfigMap`（`binaryData` + SHA256 annotation，STX 升级时自动滚动更新）；
  2. 创建独立 1 副本轻量 `Deployment`（`stx-java-proxy-<cluster_code>`）+ ClusterIP `Service`：
     - **复用完全相同的 SeaTunnel 镜像、ImagePullSecrets、环境变量**；
     - **智能克隆只读/共享类卷挂载**：自动复制目标 StatefulSet/Deployment 中挂载到 `/opt/seatunnel/{connectors,plugins,lib,config}` 及 Checkpoint 的 `ConfigMap` / `Secret` / `RWX|ROX PVC` / 插件 `initContainers + emptyDir`，**自动跳过 StatefulSet 的 `RWO` 单 Pod 数据 PVC**（避免跨节点挂卷冲突）；
     - 挂载 `ConfigMap` 中的 `stx-java-proxy.jar` 到 `/opt/stx-proxy/stx-java-proxy.jar`，直接用容器内 `java` 覆盖 `command` 启动 `StxJavaProxyApplication`；
  3. STX 经 `K8sTransport`（`apiserver_proxy` / `in_cluster`）直连该 Service，**网络视角与运行中作业完全一致**（可直连 K8s 内网 DB/Kafka/HDFS）。

---

## 5. K8s 无 `stx agent` 全量能力缺口与替代矩阵

来源：`internal/proto/agent/agent.proto`、`agent/internal/{executor,diagnostics,collector,monitor}`

| Agent 职责 | K8s 替代方案 | 结论与真实边界 |
| --- | --- | --- |
| 进程拉活/心跳/自动重启 (`Heartbeat`, `START/STOP/RESTART`, `auto_restart`) | Kubelet + StatefulSet/Deployment Probe + Scale / Rollout Restart | ✅ K8s 原生更强 |
| 集群发现与配置文件读写 (`DISCOVER_CLUSTERS`, `PULL/UPDATE/ROLLBACK_CONFIG`) | `client-go` Label/Workload 发现 + ConfigMap 读写（保留 STX 版本/Diff） | ✅ 完全替代 |
| 成员探测与引擎就绪检查 (`check_cluster_members`, `check_engine_health`) | `K8sTransport` 直连 Master Pod `:8080/overview`、`/system-monitoring-information` | ✅ 完全替代（已实测） |
| 作业日志 & 节点日志 (`sync_job_logs`, `COLLECT_LOGS`) | K8s `Pods().GetLogs()` + 引擎 REST `/logs/{jobId}` | ✅ 基本替代（Pod 彻底销毁且无外部日志系统时历史不可查；App 模式在 TTL 内可读） |
| 诊断中心错误日志巡检聚合 (`diagnostics/error_collector.go` → `LogStream`) | **STX 控制面补轻量 `K8sErrorLogCollector`**：按 `SinceTime` 游标增量拉取受管 Pod Logs，复用同套正则与错误归组入库 | ⚠️ 易遗漏缺口，控制面补轻量拉取即可完全保留诊断中心错误聚合 |
| JVM Thread Dump / Heap Dump (`THREAD_DUMP`, `JVM_DUMP`) | **Thread Dump**：`pods/exec` 调容器内 `jcmd 1 Thread.print` / `jstack 1`（实测 `apache/seatunnel:3.1.0` 自带）；**Heap Dump**：容器内存/磁盘受限极易 OOMKill，一期明确标受限或仅在挂载诊断卷时开放 | ⚠️ Thread Dump 可补齐（需 `pods/exec` 权限与 JDK 镜像）；Heap Dump 受限 |
| 主机细粒度资源指标 (`ReportHostMetrics` → `stx_host_*` CPU/Mem/Disk/Net) | 分级获取：① SeaTunnel `/system-monitoring-information` + `/metrics`（JVM/堆/GC）；② 若 K8s 安装了 `metrics-server`（实测云端 Minikube 默认未开），查 `PodMetrics`/`NodeMetrics` 写 `embedts`；③ 无宿主机磁盘 IOPS / 网卡吞吐 | ⚠️ Pod CPU/Mem 视 `metrics-server` 可选增强；宿主机 IOPS/网卡真实缺失 |
| 安装包/插件分发、JDK 探测、版本升级 (`INSTALL`, `UPGRADE`, `INSTALL_PLUGIN`) | 范式切换：镜像 Tag 滚动升级 + 自定义镜像（二期可选 InitContainer/PVC Overlay） | 🔄 容器不可变范式转换 |
| 工作台 `local` 本地执行模式 (`sync_local_*`) | 由 K8s App 模式（STIP-41）自然承接，K8s 集群隐藏 `local` 模式 | 🔄 由 App 模式替代 |
| `stx-java-proxy` 托管与 Runtime Storage 本地探测 | 伴生 `stx-java-proxy` Deployment 统一承接 HTTP 调用 | ✅ 完全替代 |

