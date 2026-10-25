# Design（草案）：Zeta Application Mode 交互模型

> 状态：**草案，部分已确认**（3.1 两层模型、3.5 网关集群已于 2026-10-25 确认）。前提：master 开启 REST v2（见 `prd.md` 中 2026-10-25 的决策）。
> 平台：**K8s 先行，YARN 留口子**。所有模型、表和接口都按平台无关设计，平台差异收敛到 3.4 的平台驱动里。
> 证据：`research/k3s-spike.md`、`research/dinky-gateway.md`、Dinky `Job2MysqlHandler` / `ClusterInstanceServiceImpl`。

## 1. Dinky 的做法（对照）

| 概念 | Dinky | 证据 |
| --- | --- | --- |
| 常驻配置 | `ClusterConfiguration`：flink 配置、K8s/YARN 参数，由管理员维护 | `GatewayConfig` |
| 每次运行 | 提交成功后**自动登记一条 `ClusterInstance`**：`autoRegisters=true`，`name=jobId`，`alias=作业名_时间`，`hosts=JM 地址`，并记录 `clusterConfigurationId` 和 `taskId` | `Job2MysqlHandler.java:153-162` |
| 作业与实例的关系 | `JobInstance.clusterId` 指向这条自动登记的实例，监控就是按它的 `hosts` 调用 Flink REST | 同上 L186-206 |
| 列表 | 集群实例列表用 `isAutoCreate` 开关区分手动建的和自动登记的，默认只看手动建的 | `ClusterInstanceController.java:161`、`selectListByKeyWord` |
| 删除 | 自动登记的实例**健康时不允许删除**，要先 `killCluster` | `ClusterInstanceServiceImpl.java:182` |
| 回收 | 清理作业历史时，连带删除对应的自动登记实例 | `ClearJobHistoryHandler.java:97-102` |

Dinky 这样做的效果：监控链路完全复用 Session 集群那一套（都是“某个实例的 hosts 加上 Flink REST”），代价是集群列表里混进了大量临时记录。

## 2. STX 的约束（来自代码）

- 作业与集群的绑定：`Task.ClusterID`（`internal/apps/sync/model.go:181`）、`TaskVersion.ClusterIDSnapshot`，以及 `JobInstance.SubmitSpec["cluster_id"]`（`runtime_support.go:269`）。对账按 cluster 分组（`job_reconciler.go:85/108/279`）。
- 端点解析：`ResolveEngineEndpoint(clusterID, taskDefinition)` 遍历 `Cluster.Nodes`，取 `APIPort` 和在线 agent 主机的 IP（`engine_client.go:496-525`）；同时**已经支持**从 task definition 里读取 `engine_base_url` 作为覆盖值（`endpointFromTaskDefinition`）。
- `Cluster` 是一个**重运维对象**：有 nodes、安装目录、配置模板、启停、扩容；删除集群时会给每个节点发 STOP。plugin template、Catalog 探测等预检查接口都要求传 `cluster_id`（`plugin_template.go:135-172`），并经该集群节点上的 proxy 执行。
- K8s 主机已有字段：`K8sAPIURL`、`K8sNamespace`、`K8sKubeconfig`、`K8sToken`（`internal/apps/host/model.go:152-157`）；控制面目前**没有引入 client-go**，这些字段只用于增删改查（本任务引入 client-go，见 3.4）。

## 3. 模型

### 3.1 两层：常驻的“Application 集群”加上一次性的“Application 实例”（✅ 已确认）

```
集群管理
├─ seatunnel-3-single           (standalone，现状)
├─ gw-small  (standalone，1 节点)  ← 网关集群：一个普通的小 Zeta，现有安装/插件/proxy 全部照旧
└─ k3s-apps  [Application · K8s] ← 常驻：执行目标；没有节点，网关 = gw-small
     ├─ 网关：gateway_cluster_id → gw-small（预检查、调试预览、提交都在这里执行）
     ├─ 平台：K8s 主机(凭据)、namespace、ServiceAccount、镜像、拉取策略
     ├─ 默认资源：master/worker 内存与 CPU、worker 数、slots
     ├─ 运行期：REST 端口、basic-auth、事件回调开关、checkpoint 存储(runtime storage)
     ├─ 生命周期：结束后保留时长、日志归档
     └─ [运行中/历史 Application 实例]  ← 一次性，每次运行一条
          spike-v2-399a37a1-512  RUNNING  作业#123  2 workers  启动于 08:10
```

- **常驻层复用 `clusters` 表**（✅ 已确认），新增 `deployment_mode = application`，以及 `platform = kubernetes | yarn`（第一期只实现 kubernetes）。平台参数放在 `Config.application.<platform>` 中，公共参数放在 `Config.application` 中，结构见 3.4。好处：
  - `Task.ClusterID`、权限、版本能力矩阵、预检查接口（要求传 `cluster_id`）**全部不用改**；
  - 选执行目标就是选集群，用户的心智模型和现在一致。
  - Application 集群**没有节点**（✅ 3.5 已确认，取代早先设想的 `gateway` 节点角色）。安装、启停、扩容、配置同步这类节点操作对它不适用，按 deployment_mode 隐藏即可，不需要在每个节点操作里加判断。
- **一次性层不写入 `clusters` 表**（✅ 已确认，与 Dinky 不同）：新增 `application_instances` 表，与 JobInstance 一一对应，字段平台无关，完整字段见 3.4。

  理由：
  - 临时记录不会污染集群列表；
  - 不会误触集群的“重操作”，比如删除时给节点发 STOP；
  - 生命周期字段可以独立维护。

  展示方式：在父集群详情页下的“Application 实例”标签页里，实现与 Dinky 的 `isAutoCreate` 相同的效果。

### 3.2 生命周期

```
SUBMITTING ─deploy ok→ DEPLOYING ─REST 可达→ RUNNING ─作业终态(事件/轮询)→ FINISHED
     │                    │(启动超时/Pod失败)          │                        │
     └─失败→ FAILED       └────────→ FAILED            └─stop/cancel→ STOPPING ─┘
FINISHED ─收集日志+终态快照→ RETAINED ─K8s TTL 到期(默认1h，可"保留现场"延至≤7d)或"立即回收"→ CLEANED(平台资源删除，记录保留)
```

- 作业结束后，K8s 资源先**保留一段时间**，便于排查问题。
  - ⚠️ 已核实，**保留的只有 master Pod**。application 结束时，master 先释放并以宽限期 0 删除所有 worker Pod（`ApplicationResourceManager.java:246` → `KubernetesResourceManagerDriver.stopWorkers` → `deleteWorkers`），之后才发布最终结果。因此 worker 日志**不可能在 FINISHED 之后再归档**，必须在运行期间持续采集，见 3.7。
  - 已核实：上游的 `kubernetes.finished-job-retention-seconds`（**默认 86400**）会写入 Job 的 `ttlSecondsAfterFinished`（`ApplicationJobFactory.java:74`），由 K8s TTL controller 级联回收 Job 和 Pod。也就是说，**正常回收不需要 STX 自己计时**：STX 通过 informer 或 `ListOwned` 发现 Job 已经消失，就把实例标记为 CLEANED。
  - ✅ 已确认：
    - **默认保留 1 小时**，在 Application 集群的 `retention.finished_seconds` 配置，STX 提交时把它写入 `kubernetes.finished-job-retention-seconds`。不沿用上游的 24 小时，避免频繁运行的定时作业在 namespace 里留下大量残留。
    - 实例上提供**“保留现场”**：用 client-go patch Job 的 `ttlSecondsAfterFinished`，每次延长到距结束时间 +24 小时，可以重复延长，**上限 7 天**。
    - 实例上提供**“立即回收”**：调用 `Cleanup`。
    - 作业失败时不自动延长。
  - 限制：K8s 不保证 TTL 到期后再延长仍然有效，所以“保留现场”只在 RETAINED 状态、并且 `expire_at` 之前可以操作。`expire_at` 根据 `finished_at` 加 TTL 计算，每次 patch 后同步更新。
- 实例记录跟随作业历史一起清理（与 Dinky 的 ClearJobHistory 相同）。
- 孤儿处理：
  - 已核实：上游的 `kubernetes.master.labels`、`kubernetes.worker.labels` **只会打在 Pod 上**，owner Job 只带上游自己的身份 label（`ApplicationJobFactory.java:76-79`）。因此 `ListOwned` 改为按 `stx.io/instance-id` 列出 **master Pod**，再通过 `seatunnel.apache.org/application-id` 关联到 Job。master Pod 和 Job 的生命周期相同（backoffLimit=0，并随 TTL 一起回收）。
  - 数据库里没有对应记录的 → 告警，或者由用户确认后清理；
  - 数据库里有但 Job 已经没了的 → 标记为 UNKNOWN 或 CLEANED；
  - 同一 namespace 里用户自己用 CLI 提交的 application 不带 STX label，STX 不会去碰它们。
- 流作业重启：新建一个 application，并带上 `--restore-job-id`，产生新的实例和新的 JobInstance（`RecoveredFromInstanceID`）。前提是父集群配置了共享 checkpoint 存储。

### 3.3 提交链路

1. 运行绑定到 Application 集群的 Task：创建 JobInstance 和 application_instance（SUBMITTING），**预先分配 engine job id**（`--job-id`）。
2. Go 组装 deployment options，来源依次为：集群默认值 → task 级覆盖（worker 数、资源）→ 强制项。强制项包括：
   - `application.master.rest.enabled=true` 和 `application.master.rest.port`；
   - `application.job-id`（预先分配的 ID），以及恢复时的 `application.restore-job-id`；
   - STX label：通过 `kubernetes.master.labels` 和 `kubernetes.worker.labels` 打上 `stx.io/instance-id`，以及 `stx.io/cluster-id`；
   - `kubernetes.finished-job-retention-seconds`（来自 `retention.finished_seconds`）；
   - checkpoint 存储（`kubernetes.checkpoint-pvc`，或者 runtime storage 对应的引擎配置）；
   - 事件回调 URL 和 token（通过 ConfigMap 下发 seatunnel.yaml）。
3. Go → 网关集群的 stx-java-proxy（通过 `ResolveConfigToolEndpoint(gateway)` 解析，和现有预检查同一条路径）→ `POST /api/v1/application/deploy`。proxy 在进程内**动态加载平台 provider**，调用上游的 `ApplicationClusterDeployer`，返回 `appId`（✅ 已确认走 proxy，设计见 3.6）。
4. 写入 `rest_endpoint = <apiserver>/api/v1/namespaces/<ns>/services/<appId>:<restPort>/proxy`。对账器对 application 实例**按实例单独分组**，复用 `EngineClient` v2，只是 HTTP transport 改为 `kubex` 根据 K8s 主机凭据生成的 transport（需要 RBAC `services/proxy`）。在 REST 可达之前，用平台状态显示 DEPLOYING 或失败原因。
5. 停止：v2 `stop-job`（可选 savepoint）→ 等待终态（事件或轮询）→ 等待日志流写完（3.7）→ 平台清理。如果引擎无响应，直接调用 `Cleanup`。

### 3.4 平台抽象：K8s 先行，YARN 留口子

**原则**：sync、对账、工作台、CLI 只依赖平台无关的接口和字段；K8s 与 YARN 的差异全部放进 Go 侧的 `ApplicationPlatform` 驱动，以及 proxy 侧的 `-t <target>` provider 中。

**`clusters.config.application`（公共部分）**

| 字段 | 说明 |
| --- | --- |
| `platform` | `kubernetes` \| `yarn` |
| `seatunnel_version` | 用于能力矩阵判断（Application Mode 需要 3.1.0 及以上） |
| `defaults` | master/worker 的内存、CPU，worker 数，slots，startup timeout |
| `rest` | 端口、basic-auth（引用 secret） |
| `events` | 开关、回调地址；token 按实例生成 |
| `checkpoint_storage` | 引用 runtime storage，恢复依赖它 |
| `retention` | 结束后保留时长；是否在结束时自动归档日志 |
| `gateway_cluster_id` | 网关集群（standalone），见 3.5。YARN 场景下，网关集群的节点还需要 `HADOOP_CONF_DIR` |

**平台部分**

| 维度 | `kubernetes`（第一期） | `yarn`（留口子） |
| --- | --- | --- |
| 连接凭据 | K8s 主机记录（`K8sAPIURL`/`K8sKubeconfig`/`K8sToken`） | 网关集群节点上的 `HADOOP_CONF_DIR`（core/yarn/hdfs-site），或上传的配置文件；Kerberos 不在上游 v1 范围内 |
| 资源隔离 | namespace、ServiceAccount、node-selector/tolerations | queue、node-label |
| 制品 | 镜像（用户提供完整发行版；后续由制品层构建） | 上游 v1 设计：**每次提交一个自包含的 `yarn.distribution` 归档** → **非常适合按作业生成精简归档**，这正是“只带所需连接器”最早的动机 |
| 实例 ID | K8s Job 名（`<name>-<hash>`） | `application_<ts>_<seq>` |
| STX 打标 | 通过 `kubernetes.{master,worker}.labels` 在 Pod 上打 `stx.io/instance-id`（Job 上打不了） | application tags（**需要上游 YARN provider 支持**） |
| REST 访问 | apiserver `services/<appId>:<port>/proxy`（RBAC `services/proxy`） | RM web proxy `/proxy/<appId>/`（**需要 master 把 REST v2 地址注册为 trackingUrl**，Dinky 也是这样做的）；网络允许时也可以直连 AM 的 host:port |
| 平台状态 | Job/Pod 状态映射（上游 `getApplicationStatus`） | ApplicationReport 的 state 与 finalStatus（上游 provider 负责） |
| 日志 | `pods/log`（master 和所有 worker） | NM 容器日志 / 聚合日志（`yarn logs -applicationId`） |
| 清理 | 删除 owner Job（随之级联删除） | 运行中的作业 kill；已结束的作业由 RM 自行回收 |
| 孤儿发现 | 按 label 列出 master Pod，再关联到 Job | 按 application tag 和 type 列出 application |

**`application_instances`（平台无关）**：
- 标识与归属：`id`、`job_instance_id`（唯一）、`cluster_id`、`platform`、`platform_app_id`、`platform_scope`（K8s 为 namespace，YARN 为 queue）、`engine_job_id`；
- REST 访问：`rest_endpoint`、`rest_access`（`k8s_service_proxy` \| `yarn_rm_proxy` \| `direct`）；
- 状态：`platform_status`、`status`（3.2 中的生命周期状态）；
- 时间：`deployed_at`、`finished_at`、`expire_at`、`cleaned_at`；
- `logs_archived_at`；
- `detail`（JSON，存放平台特有的信息）。

**Go 侧 `ApplicationPlatform` 接口**（第一期只实现 kubernetes）：

```go
// ApplicationPlatform 屏蔽 K8s/YARN 差异；需要上游语义的操作经网关 proxy 执行，只读的观测类操作由控制面直连平台 API。
// ApplicationPlatform hides K8s/YARN differences; operations needing upstream semantics go through the gateway proxy, read-only observation hits platform APIs directly.
type ApplicationPlatform interface {
    // 经网关 proxy（上游 descriptor 语义）/ via gateway proxy (upstream descriptor semantics)
    Validate(ctx, cluster, spec) error                              // 解析 options 与作业，不部署
    Deploy(ctx, cluster, spec) (platformAppID string, err error)
    Status(ctx, cluster, inst) (PlatformStatus, error)             // 上游 getApplicationStatus 的映射，Go 不重复实现
    Cleanup(ctx, cluster, inst) error                               // 上游 cancelApplication = 前台级联删除 owner Job
    // 控制面直连 / direct from the control plane
    RESTTransport(ctx, cluster, inst) (baseURL string, rt http.RoundTripper, err error) // 交给现有 EngineClient v2
    CollectLogs(ctx, cluster, inst) ([]LogStream, error)
    ListOwned(ctx, cluster) ([]OwnedApplication, error)             // 孤儿发现
}
```

- 上游的 `cancelApplication` 会以 Foreground 方式删除 owner Job（`DefaultKubernetesClient.java:249-252`），它就是平台清理。所以原来的 `Cancel` 和 `Cleanup` 合并：引擎级停止走 v2 `stop-job`，平台级操作只剩 `Cleanup`。
- 网关不可用时：运行中作业的观测不受影响（REST 和日志由控制面直连）；平台状态显示为“暂不可用”；清理进入重试。

**控制面的 K8s 访问：引入 client-go（✅ 已确认，理由是后续要和 K8s 深度集成，不自己造轮子）**

- 新增共享包 `internal/pkg/kubex`，与 `schedulex`、`timex` 并列，符合 `directory-structure.md` 的共享包约定。它负责根据 K8s 主机记录构造 `rest.Config` 和 clientset，后续所有 K8s 集成都复用这个包：
  - 有 kubeconfig：`clientcmd.RESTConfigFromKubeConfig`，exec、auth-provider、证书等方式全部支持；
  - 只有 token：`K8sAPIURL` 加 `BearerToken`；
  - 按 host ID 和凭据指纹缓存 clientset，凭据变更后自动重建。
- 第一期用到的能力：
  - `RESTTransport`：`rest.TransportFor(cfg)` 得到 RoundTripper，baseURL 为 `cfg.Host + /api/v1/namespaces/<ns>/services/<appId>:<restPort>/proxy`，两者都直接交给现有的 `EngineClient`；
  - `CollectLogs`：`CoreV1().Pods(ns).GetLogs(pod, &PodLogOptions{Container, Previous})`，按 master 和 worker 的 label 枚举 Pod；
  - `ListOwned`：按 label selector `stx.io/instance-id` 列出 Job。
- 后续深度集成可以直接用上的能力（本期不做，只确保结构允许）：
  - 用 SharedInformer watch `seatunnel.apache.org/application-id` 下的 Pod 和 Job，状态变化时立刻触发对账，补上事件推送缺失的 cancel 终态；
  - 通过 Job patch 修改 `ttlSecondsAfterFinished`，实现保留现场；
  - port-forward（SPDY）；
  - 主机侧的 K8s 资源浏览；
  - 以后 STX 自身的 K8s 部署（operator/CRD）。
- 凭据一致性：Go 侧用 client-go，proxy 侧（上游 provider）用 client-java，两边都支持 exec 插件，但 exec 用到的二进制（如 `aws`、`gcloud`）必须同时存在于控制面主机和网关节点上。创建 Application 集群时，校验会分别从 Go 和网关 proxy 各发起一次 apiserver 请求（Go 用 `SelfSubjectAccessReview` 检查所需的 RBAC，proxy 用 `validate`），哪一侧失败就明确指出是哪一侧。

**需要推动的上游事项（按平台）**

- 通用：`application.master.rest.enabled/port`（补丁已验证，**默认开启**，由用户提交 PR）；作业结束后 master 延迟 N 秒退出（解决终态快照问题）。
- K8s provider：自定义 label 同时打到 owner Job 和 Service 上，方便按 label 管理资源（锦上添花，不阻塞 STX）。
- K8s provider：支持直接传入 kubeconfig 内容。当前 `KubernetesClientFactory.getApiClient` 只调用 `Config.fromConfig(path)`，而 client-java 本身有 `Config.fromConfig(Reader)`，改动很小。
- YARN provider（上游还没有 PR）：master 把 REST 地址注册为 trackingUrl；支持 application tags 和 type；保持每次提交使用 `yarn.distribution`。

### 3.5 网关集群（✅ 已确认：Application 集群引用一个小型 standalone 集群）

**为什么需要一个常驻的 Zeta**

- 调试预览：`PreviewTask` 在推导出预览配置后，通过 `ResolveEngineEndpoint` 和 `engineClient.Submit` 把预览作业提交到常驻集群的 REST（`internal/apps/sync/service.go:1153`）。如果改成每次起一个 application，要等 10–20 秒、创建 3 个 Pod。
- 预检查：DAG、Catalog、plugin template、runtime storage 浏览都依赖集群节点上的 stx-java-proxy，而这套安装、插件同步、proxy 启停、agent 链路已经存在。

**路由：只改三个解析入口，调用方不动**

| 入口 | 调用方 | Application 集群下的行为 |
| --- | --- | --- |
| `DefaultConfigToolResolver.ResolveConfigToolEndpoint`（`config_tool_client.go:560`） | 共 8 处：plugin template、DAG、校验、Catalog 等 | `clusterID` 换成 `gateway_cluster_id`，再走原来的逻辑 |
| `ResolveEngineEndpoint`，仅限预览调用点（`service.go:1153`） | 调试预览 | 换成网关集群的 REST。**正式运行和对账不走这里**，改为按实例使用 `rest_endpoint` |
| `pickSTXJavaProxyNode`（`stx_java_proxy.go:241`） | runtime storage 浏览/预览、proxy 管理 | 换成网关集群 |

建议做法：在 cluster service 中新增一个 `ResolveGatewayClusterID(clusterID)`（standalone 集群原样返回）。三个入口统一先调用它，测试只需要覆盖这一处。`cluster_running_jobs`（`cluster_running_jobs.go:42`）对 Application 集群改为列出它的运行中实例。

**约束与校验**

- 网关集群必须是 standalone，不能是另一个 Application 集群，不允许嵌套。
- 创建或修改 Application 集群时校验：
  - 网关集群在线；
  - 版本与 `seatunnel_version` 一致，且 ≥ 3.1.0；
  - 网关 proxy 的 `GET /api/v1/application/providers` 报告支持该平台（见 3.6）。
- 被 Application 集群引用的网关集群不允许删除；修改网关集群版本时，列出受影响的 Application 集群，并要求确认。
- 一个网关集群可以服务多个 Application 集群，比如不同的 namespace、不同的 K8s 集群，或者以后的 YARN。
- 网关集群本身仍是普通集群，可以直接跑小作业。资源按 1g 堆规格，1 个节点即可。
- 插件：网关集群的插件用于预检查和预览。application 镜像里的连接器由用户提供（第一期），之后由制品层（R4）按作业生成。两边的连接器集合可能不一致，所以预检查通过不代表镜像里一定有该连接器。第一期只提示，不做校验。

**权限**：STX 目前没有按集群划分的权限，只有角色级权限（`internal/apps` 下没有 cluster 级的 ACL）。路由到网关集群属于系统内部行为。以后如果加入集群级权限，以作业绑定的 Application 集群为准，不要求用户同时拥有网关集群的权限。

### 3.6 proxy 内提交：动态加载平台 provider（✅ 已确认走 proxy）

**现状与上游契约（已核实）**

- proxy 的 classpath 为 `$SEATUNNEL_HOME/lib/*:starter/seatunnel-starter.jar:stx-java-proxy-<epoch>.jar`（`scripts/stx-java-proxy.sh:205`）。在 3.1.0 中，starter jar 已经包含 `ApplicationClusterDeployer`、`ClusterClientServiceLoader` 和 `SeatunnelApplicationConfig`。上游 CLI 的 classpath 只是多了 `resource-managers/<target>/*`（`seatunnel-application.sh:57-60`）。
- 上游的 provider 发现机制是 `ServiceLoader.load(ApplicationClusterDescriptorFactory.class)`，**使用线程上下文 classloader**（`ClusterClientServiceLoader`）。也就是说，上游本来就预期 provider 可以由调用方提供的 classloader 加载。
- K8s provider jar（31M）已经**自行 shade** 了依赖：io.kubernetes client-java、commons-lang3 等都位于 `org/apache/seatunnel/shade/resource/kubernetes/...` 下，并通过 `META-INF/services/...ApplicationClusterDescriptorFactory` 注册。所以它和 proxy 的 classpath 不会发生冲突。
- 提交只需要三步调用，全部是上游的公开 API：
  1. `SeatunnelApplicationConfig.load(null, overrides)`，得到 options；
  2. `SeatunnelApplicationConfig.parse(String jobConfig, options)`，得到 spec。作业配置**可以直接传字符串**，不需要落盘；
  3. `new ApplicationClusterDeployer(type, spec, options).run()`。

  `status` 和 `cancel` 走 `factory.create(options)` 得到 descriptor，再调用对应方法（`ApplicationExecuteCommand.java:80-176`）。

**结构**

```
stx-java-proxy core（epoch-v2 / epoch-v3，现有）
 └─ ApplicationGatewayService：只依赖 JDK 类型（Map<String,String>、String）
      └─ PlatformClassLoaderRegistry
           平台 classloader = [bridge jar] + $SEATUNNEL_HOME/resource-managers/<platform>/*.jar
           parent = proxy 应用 classloader；按文件指纹缓存（沿用 PluginClassLoaderUtils 的做法）
           调用期间把 TCCL 切换为平台 classloader，finally 中还原
 └─ stx-java-proxy-application-bridge（新模块，按 3.1.0 编译）
      实现 ApplicationGateway：调用上游的 load/parse/deploy/status/cancel，把上游异常转换为结构化错误
```

- **为什么要单独一个 bridge 模块**：proxy 目前按代际构建（`pom.xml` 中的 epoch-v2=2.3.13、epoch-v3=3.0.0），而 3.0.0 里没有这些 API。bridge 按 3.1.0 编译，并且**只在平台 classloader 中加载**，core 永远不直接引用上游的 application 类。这样旧版本的 proxy 不受影响，以后 3.2 有 API 变化时，只需要新增一个 bridge。
- **扩展方式**：新平台（比如 YARN）只需要在 `resource-managers/yarn/` 下放 provider jar，STX 侧零代码就能发现。`GET /api/v1/application/providers` 用于扫描该目录并报告可用平台，创建 Application 集群时的“网关支持该平台”校验就用这个接口，取代 3.5 中提到的 `stat_path`。
- **版本门禁**：SeaTunnel < 3.1.0、bridge 缺失、provider 缺失，或者上游没有 `application.master.rest.enabled` 选项（即没有合入 master REST 补丁）时，接口返回 `unsupported`，并附带原因。第一期不做 v1 兜底。STX 提交时显式设置 `application.master.rest.enabled=true` 和端口，不依赖上游默认值。Go 侧再结合能力矩阵在 UI 上隐藏入口。

**接口**（与现有 proxy 接口风格一致）

| 接口 | 入参 | 出参 |
| --- | --- | --- |
| `GET /api/v1/application/providers` | — | `[{platform, jars, version}]` |
| `POST /api/v1/application/validate` | `platform`、`options`、`job_config`、`credentials` | `ok` 或结构化错误：`{type: option_invalid \| job_config_invalid \| provider_missing \| platform_unreachable, message}` |
| `POST /api/v1/application/deploy` | 同上，外加 `job_id`、`restore_job_id` | `{application_id, job_id}` |
| `POST /api/v1/application/status` | `platform`、`options`、`application_id`、`credentials` | `{status, terminal, failure}` |
| `POST /api/v1/application/cancel` | 同上 | `ok` |

**凭据**（⚠️ 修正上一轮说法）：上游 `KubernetesClientFactory.getApiClient` 只调用 `Config.fromConfig(path)`，所以**即使在 proxy 进程内调用，kubeconfig 也必须是一个文件**。做法如下：
- proxy 收到凭据后，写入它自己 state 目录下的 `application/creds/<uuid>`，权限 0600，并在 finally 中删除；进程启动时清空该目录，避免崩溃后残留。
- Go 侧根据 K8s 主机记录生成凭据：有 kubeconfig 时直接使用；只有 token 时，用 `K8sAPIURL` + token 拼一个最小 kubeconfig。
- 同时推动上游支持直接传入 kubeconfig 内容（见 3.4 的上游事项），上游合入后就不再落盘。

**线程与超时**：proxy 使用固定大小的线程池（`StxJavaProxyServer.java:88`），和预检查共用。一次 deploy 实测约 1.6 秒，但遇到 apiserver 慢或者不可达时，会阻塞到 provider 的 API 超时。因此 application 类接口**使用独立的小线程池**，并设置单次请求超时，避免拖慢 DAG 解析、plugin template 等交互式接口。

**日志脱敏**：上游约定 provider “must not modify or log” options（`ApplicationClusterDescriptorFactory.create` 的注释）。proxy 侧同样不记录 `credentials`，也不记录 options 中 `*.secret*`、`*password*` 这类键。

### 3.7 日志（✅ 已确认：运行期间流式采集，归档到控制面本地）

**现状**：standalone 集群的作业日志由 agent 从节点日志文件按需读取（`sync_job_logs`，`runtime_support.go:588`），支持 offset、keyword、level，没有归档，因为日志文件一直留在节点上。

**application 的约束**：
- worker Pod 在作业结束时立即被删除（3.2）；
- master Pod 在 TTL 到期（默认 1 小时）后被删除；
- master 只把日志输出到 console，`/logs` 接口返回 400（spike 第三轮）。

**方案：运行期间流式采集，归档到控制面本地**
- 控制面用 client-go 的 informer 发现实例的 Pod（label `seatunnel.apache.org/application-id`）。每发现一个 Pod，就开启一条 `GetLogs(Follow: true, Timestamps: true)` 流，写入 `storage.base_dir/application-logs/<instance_id>/<pod>.log`。
- STX 重启后，用每个文件最后一行的时间戳作为 `SinceTime` 续采，并按时间戳去重。STX 停机期间被删除的 worker，它的日志会丢失；这种情况在实例上标记为“日志不完整”。
- 读取：无论运行中还是已结束，都从本地归档读。复用现有的 `JobLogsResult` 接口（offset、keyword、level），只是由 Go 在本地过滤，不再经过 agent。
- 容量：单个 Pod 文件设上限（例如 200MB，超出后丢弃头部、保留尾部），随作业历史清理一起删除。
- 上游诉求：worker 是否在作业结束时删除改为可配置（例如保留到 master 退出，或交给 TTL 一起回收）。上游支持后，可以把流式采集降为兜底。

## 4. 待确认的决策（逐个确认）

0. ~~是否需要常驻 Zeta~~ → ✅ 必须引用一个小型 standalone 网关集群（3.5）。
1. ~~执行目标的位置~~ → ✅ 复用 `clusters` 表（`deployment_mode=application`，`platform` 留出 yarn）。
2. ~~每次运行的实例~~ → ✅ 独立的 `application_instances` 表，字段平台无关。
3. ~~保留时长与保留现场~~ → ✅ 依赖 K8s TTL，默认 1 小时；“保留现场”每次延长到 +24 小时，上限 7 天；提供“立即回收”（3.2）。
4. ~~提交通道~~ → ✅ 一步到位走 proxy，动态加载 provider（3.6）。
5. ~~控制面访问 apiserver~~ → ✅ 引入 client-go，封装在共享包 `internal/pkg/kubex`（见 3.4 末尾）。
6. ~~日志~~ → ✅ 运行期间流式采集，归档到控制面本地 `storage.base_dir/application-logs/`（3.7）。
