# Dinky 的 Flink Application Mode gateway 实现分析

> 源码基线：DataLinkDC/dinky `0b426b3`（2026-10-22）。重点模块：`dinky-gateway`、`dinky-app`、K8s 文档。

## 结论先行

Dinky 能“原生”支持 Flink on YARN/K8s application，主要依靠的是 **Flink 自身成熟的客户端部署 SDK 和运行期 REST**。Dinky 做的是 gateway 封装，外加一个运行在 JM 内的入口 jar（dinky-app）。
它的“无需安装集群”，本质上是把安装工作挪到了**制品准备**：YARN 需要把 Flink lib 和 dinky-app 放到 HDFS；K8s 需要自己构建镜像，文档里“页面构建镜像”一节仍写着“正在赶来中”。

## Gateway 抽象

- `Gateway` 接口通过 `ServiceLoader` 按 `GatewayType` 选择实现，覆盖：`submitJobGraph`/`submitJar`、savepoint、`getJobStatusById`、`killCluster`、`test()`（连通性测试）。
- 集群配置（`GatewayConfig`）= `ClusterConfig`（flinkConfigPath、**flinkLibPath**、hadoopConfigPath、自定义 hadoop 配置、appId）+ `FlinkConfig` + `AppConfig`（userJarPath、主类、参数）+ `K8sConfig`。
  → 对应到 STX：可以抽象成“执行目标”配置，单独保存在一张表里，与作业解耦，由作业引用。

## YARN Application（`yarn/YarnGateway.java`、`YarnApplicationGateway.java`）

| 关注点 | Dinky 做法 |
| --- | --- |
| 发行包 | `YarnConfigOptions.PROVIDED_LIB_DIRS = flinkLibPath`（HDFS），**每次提交不上传 Flink 本体** |
| 入口 | `PipelineOptions.JARS = [dinky-app jar]`（可以是 `rs://` 资源），`ApplicationConfiguration(args, mainClass)` |
| 按作业依赖 | `addShipFiles(...)`：把 UDF 和第三方 jar（`config.getJarPaths()`、resources）随这次提交一起带上 |
| 作业内容 | SQL 写入临时文件，通过 ship files 带过去，JM 内的 dinky-app 再读这个文件 |
| 标识 | `APPLICATION_TYPE = "Dinky Flink"`，`APPLICATION_NAME = 作业名` |
| 状态 | `YarnClient.getApplicationReport` → 由 `YarnApplicationState` 和 `FinalApplicationStatus` 映射出 CREATED/RUNNING/FINISHED/FAILED/CANCELED/UNKNOWN |
| 作业 ID | 提交后先等到 ACCEPTED→RUNNING，再访问 `trackingUrl + /jobs/overview`（Flink REST 经 YARN proxy）取 Flink job id；返回 401 时退回到 Kerberos SPNEGO |
| 失败诊断 | 从第一个 container 的 logUrl 拉取 `jobmanager.log` 尾部 |
| 取消 | `yarnClient.killApplication` |
| 安全 | keytab 方式用 Flink `SecurityUtils`；可以通过 UGI remote user 指定提交用户 |
| checkpoint | 应用模式下把 checkpoint/savepoint 目录改写为 `<dir>/<jobName>/<uuid>`，避免作业之间冲突 |

## K8s Application（`kubernetes/KubernetesApplicationGateway.java`）

- 调用 `KubernetesClusterDescriptor.deployApplicationCluster`。镜像中必须包含 Flink 和 dinky-app（`local:///...`）。
- SQL 通过 ConfigMap decorator 下发（`DinkySqlConfigMapDecorate`），带 ownerReference。
- 判断启动成功：轮询 JM Pod 的就绪状态，看到有重启或已终止就判失败；然后通过 ClusterClient 或 Ingress 的 `/jobs/overview` 取 job id。
- **按作业依赖（重点）**：dinky-app 在 JM 启动时，从 `dinkyAddr/download/downloadDepJar/{taskId}` 下载依赖 zip，解压到 `FLINK_HOME/usrlib`，再动态加入 classloader。也就是“**运行期从控制面拉取**”，这样镜像不必包含连接器。
- 这个做法之所以可行，是因为 dinky-app 本身就是用户代码，能在 JM 内执行任意逻辑。

## JM 内回调（`dinky-app`）

- `MainApp`：参数为 base64 编码的 JSON → `DBUtil.init` **直连 Dinky 数据库**读取任务和系统配置 → `Submitter.submit`。
- `FlinkAppUtil.monitorFlinkTask`：每 5 秒轮询一次 job 状态，作业结束时回调 `GET {dinkyAddr}/api/jobInstance/hookJobDone`（重试 10 次）。
- 代价：JM 需要能访问 Dinky 的数据库和 HTTP 服务，网络和凭据耦合较重。

## Application 模式下的运行监控（补充，2026-10-25）

结论：Dinky **没有为 application 模式单独设计一套监控**。Flink 在 application 模式下照常提供 REST，Dinky 把 application 集群当作“一个有地址的临时 standalone 集群”，监控、DAG、指标、checkpoint 走的都是和 standalone 相同的 Flink REST 代码。和模式相关的只有下面三件事：

1. **拿到 REST 地址**（`KubernetesApplicationGateway.submitJar`）
   - 默认：`client.getWebInterfaceURL()`，地址由 Flink 的 `kubernetes.rest-service.exposed.type` 决定（ClusterIP 只能在集群内访问，NodePort/LoadBalancer 可以从集群外访问）。
   - 可选：Dinky 为每个作业创建一个 nginx Ingress（`DinkyKubernetesIngress`），路径 `http://{domain}/{ns}/{clusterId}` → `{clusterId}-rest:8081`，带指向 JM Deployment 的 ownerReference，随集群一起删除。
   - 启动判定：先轮询 JM Pod 的就绪状态，再用 `listJobs` 或 `/jobs/overview` 取 jid；`webURL`、`jid`、`clusterId+jid` 写入结果。
2. **作业结束时抓取终态**（`dinky-app` 的 `Submitter.submit` → `FlinkAppUtil.monitorFlinkTask`）
   - application main 在 `executeAsync` 之后不返回，而是在 `finally` 中每 5 秒轮询一次 `jobClient.getJobStatus()`；作业结束后**同步**调用 `GET {dinkyAddr}/api/jobInstance/hookJobDone`（5 秒超时，最多重试 10 次），之后 main 才返回，集群随之关闭。
   - 效果：控制面在 JM 和 REST 仍然存活时收到通知，能做最后一次刷新并写入历史。admin 侧 `JobRefreshHandler` 的具体实现本次没有取到（环境的 shell/网络工具不可用），**这一点是根据 hook 设计推断的**。
3. **状态兜底**
   - YARN：`getJobStatusById` 读取 ApplicationReport；JM 漂移时用 `getLatestJobManageHost` 获取新地址。
   - K8s：**没有实现**，沿用 `AbstractGateway.getJobStatusById` 的默认行为，直接抛 `NotSupportGetStatusException`，状态完全依赖 Flink REST。

### 对照 Zeta Application Mode

| 环节 | Dinky / Flink | Zeta Application Mode |
| --- | --- | --- |
| 提交 | 进程内调用 Flink descriptor | ✅ 同构（`ClusterDescriptor`） |
| 运行期监控数据 | Flink REST 照常开启 | ❌ REST 被强制关闭，只能通过原生 Hazelcast 客户端 |
| 从集群外访问 | HTTP，可通过 Service 暴露类型或 Ingress | ❌ 二进制协议；`retrieve()` 直接使用 master 的 Pod IP |
| 终态快照 | dinky-app 在 JM 内阻塞 main 并回调 | ❌ 没有用户代码入口，master 跑完作业就退出 |
| 平台状态兜底 | 仅 YARN 有 | ✅ `getApplicationStatus`（K8s/YARN 统一） |

启示：如果上游在 Application Mode 下提供**可选的 REST**（默认关闭），STX 就能走 Dinky 的路线。现有的 `EngineClient`、对账和监控页面可以原样复用。HTTP 还可以通过 kube-apiserver 的 `pods/{name}:{port}/proxy` 访问，比 Dinky 的 Ingress 方案更省事：不需要 ingress controller，也不需要直通 Pod 网络。

## 版本隔离

- `dinky-app-1.14 … 1.20` 每个 Flink 版本各打一个入口 jar；gateway 依赖当前加载的 Flink 版本。

## 映射到 STX 的启示

| Dinky 的做法 | STX 是否可借鉴 |
| --- | --- |
| gateway SPI（Java，在进程内调用 SDK） | ✅ 放到 stx-java-proxy 中，Go 只做编排 |
| 执行目标配置与作业解耦，并带 `test()` 连通性检查 | ✅ |
| `provided.lib.dirs` 预置发行包 | ❌ Zeta v1 不支持（non-goal），需要推动上游 |
| ship files 按作业附带依赖 | ⚠️ Zeta YARN v1 只接受单个归档 → STX 自己生成精简归档 |
| K8s 运行期从控制面拉依赖 | ❌ Zeta 的 K8s provider 会覆盖 command，也没有 init 容器 → 需要上游提供 pod template/init 能力，或由 STX 构建镜像 |
| 通过 YARN tracking URL / Ingress 访问运行期 REST | ❌ Zeta 在 Application Mode 下关闭了 REST |
| JM 内回调控制面（hook） | ⚠️ Zeta 没有用户代码入口；可以看看引擎自带的 job event HTTP 上报能否作为替代（待验证） |
| `APPLICATION_TYPE`/name 打标 | ✅ K8s 用 labels；YARN 需要推动上游支持 |
