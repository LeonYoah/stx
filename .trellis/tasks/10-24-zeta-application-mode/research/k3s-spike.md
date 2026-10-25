# k3s 实测：Application Mode 的观测通道与事件推送

> 实测时间：2026-10-25 07:10–07:30 UTC。上游 apache/seatunnel dev `7604a83`（**版本号已变为 3.1.0-SNAPSHOT**）。
> 环境：t2（k3s server）和 t3（agent），k3s v1.36.5；t1 作为“集群外提交机 / proxy 主机”，可以访问 apiserver `10.66.0.1:6443`，**访问不到 Pod 网段**。
> 资产目录：`/data/workspace/spike`（kubectl、kubeconfig、镜像构建目录、探针、事件接收端、作业与部署配置）。

## 结论速览

| # | 问题 | 结论 |
| --- | --- | --- |
| S1 | 能否从集群外提交 | ✅ `seatunnel-application.sh submit` 用时 1.6 秒，`--job-id` 预指定的 ID 生效；状态依次为 DEPLOYING → RUNNING；资源为 owner Job、master Pod、2 个 worker Pod 和 headless Service（`<appId>:5801`） |
| P1 | t1 直连 master PodIP:5801 | ❌ 超时（路由走 eth0 默认网关） |
| P2 | port-forward + unisocket 原生客户端 | ✅ 能拿到状态、指标（约 51KB JSON）和 DAG（`vertexInfoMap`/`pipelineEdges`） |
| P2' | 断线重连 | ✅ 断开 40 秒后自动恢复；k3s 重启约 3 分钟后，重建隧道 4 秒内恢复。⚠️ kubectl port-forward 遇到 apiserver 重启会直接断开（`lost connection to pod`），**需要守护进程重建** |
| R1 | **5801 上的 v1 REST** | ✅ `/hazelcast/rest/maps/{overview,running-jobs,job-info/<id>}` 可用，job-info 中包含 `jobDag`、`metrics`、`diagnostics` |
| R2 | 不用 Java、不用 port-forward，经 apiserver 访问 | ✅ `pods/<master>:5801/proxy/...` 和 `services/<appId>:5801/proxy/...` 都可用。headless Service 的 endpoint 只有 master |
| E1 | 事件 payload | ✅ 定时批量 POST JSON 数组，带配置的 headers（Bearer token 已验证） |
| E2 | 接收端停机期间产生的事件 | ✅ 恢复后完整补发（基于游标，至少一次投递） |
| E3a | cancel 时的终态事件 | ❌ 只收到 `CANCELING`，**收不到 `CANCELED`**；Pod 2 秒内被删除，日志随之消失；之后状态变为 `UNKNOWN` |
| E3b | 自然结束、失败时的终态事件 | ✅ `FINISHED`（延迟 78ms）和 `FAILED`（延迟 180ms）都在 master 退出前送达 |
| E4 | standalone 集群 master 切换 | ⚠️ 新 master 会**从 ringbuffer 头部重推全部保留的事件（产生重复）**；事件没有唯一 ID；恢复过程表现为作业重新经历 PENDING → SCHEDULED → RUNNING |

## 第二轮：v1 是否“什么都齐了”（07:40–07:50）

| 工作台能力 | v1 实测 | 结论 |
| --- | --- | --- |
| 运行中的状态、DAG、指标、诊断 | `job-info` 和 `running-jobs` 可用 | ✅ |
| checkpoint 概览、历史 | `/jobs/checkpoints/<id>` 返回 **204 空响应**（只有 v2 servlet 实现了） | ❌ STX 现有代码在 v1 模式下本来就直接报不支持 |
| 日志 | `/logs/<id>` 返回 **400** `Log file path is empty`（application 的 master 用 `log4j2_client.properties` 输出到 console，不写文件） | ❌ 只能用 K8s `pods/log` |
| 作业结束后的数据 | master 退出后，service proxy 返回 **502**。FINISHED 事件发出到 Hazelcast 开始关闭约 110ms，事件到达 STX 时只剩约 30ms | ❌ 拿不到终态快照，**最终指标最多落后一个采样周期** |
| 带 savepoint 停止 | `stop-job {isStopWithSavePoint:true}` → 收到 `DOING_SAVEPOINT`、`SAVEPOINT_DONE` 事件，平台状态 **SUCCEEDED**，Job 为 Complete，Pod 保留 | ✅ 可以停，但平台状态分不清 FINISHED 和 SAVEPOINT_DONE |
| 引擎级 cancel | `stop-job {isStopWithSavePoint:false}` → 收到 `CANCELING`、`CANCELED` 事件，**但平台状态是 FAILED**，Job 为 Failed，Pod 保留，日志可查 | ⚠️ 终态必须以事件或 STX 自己的记录为准 |
| 平台级 cancel（`seatunnel-application.sh cancel`） | 只收到 CANCELING；Pod 2 秒内被删，日志随之丢失；状态变为 **UNKNOWN** | ❌ 只作为兜底 |
| 从 savepoint/checkpoint 恢复 | checkpoint 落在 master Pod 本地的 `/tmp/seatunnel/checkpoint_snapshot` | ❌ Pod 销毁后就没了，**恢复必须配置共享存储**（PVC/S3/HDFS，对应 STX 的 runtime storage），本次未测 |
| 提交、预检查 | v1 有 `submit-job`，但 application 是通过 `ClusterDescriptor` 部署的；配置级 DAG、Catalog 探测仍依赖 proxy | ❌ 与 v1 无关，仍需 Java |

**推荐的停止顺序**：先调 v1 `stop-job`（引擎级，可以选择是否带 savepoint）→ 等到 CANCELED/SAVEPOINT_DONE 事件，或者轮询到终态 → 收集日志 → 平台级 cancel 清理资源。只有引擎无响应时，才直接走平台级 cancel。

## 第三轮：给上游打补丁，在 master 上开启 REST v2（08:00–08:15）

补丁：`patches/seatunnel-application-master-rest.patch`（基于上游 dev `7604a83`，8 个文件，+159/-18；本地分支 `spike/application-master-rest`）。

- 新增选项 `application.master.rest.enabled`（默认 false）和 `application.master.rest.port`（默认 8080）。这两个选项写入 `ApplicationSpecification`，并随 V1 localized properties 一起传给 master；当端口非法或与 5801 冲突时，提交阶段就会被拒绝。
- `SeatunnelApplicationConfig.configure(..., Integer masterRestPort)`：
  - 只有 master 会保留引擎的 `HttpConfig`，包括 context-path、basic-auth 等配置；
  - 强制使用 HTTP，并固定端口；
  - worker 始终不开 REST；
  - 原有的 4 参数重载保持原来的行为（关闭 REST）。
- master CLI 会把这个开关透传下去。`HeadlessServiceDecorator` 在开关打开时额外暴露 `rest` 端口。
- 单元测试：新增 4 个，加上相关的原有用例，`SeatunnelApplicationConfigTest`、`ApplicationSpecificationTest`、`KubernetesApplication{,Resources}Test` 共 94 个用例全部通过。spotless 格式检查通过。

实测（镜像 `stx-spike/seatunnel:3.1.0-SNAPSHOT-rest`，部署配置加了 `application.master.rest.enabled = true`）：

- master 日志中出现 `Started ServerConnector ... 0.0.0.0:8080`，Service 端口为 `hazelcast=5801 rest=8080`。
- 经 apiserver 的 `services/<appId>:8080/proxy` 访问，**STX 现有的 Go `EngineClient`（v2）一行没改**就全部可用：
  - job-info：RUNNING，`jobDag` 有 4 个键，`metrics` 有 39 项；
  - running-jobs：1 条；
  - **checkpoint overview**：completed=6；
  - **checkpoint history**：3 条。
- `/logs/<id>` 仍返回 400，原因是 master 只把日志输出到 console，而且 worker 上没有 REST。日志继续用 K8s `pods/log`。
- 额外收获：`/system-monitoring-information` 也能用，可以拿到 master 的 JVM 和 GC 信息。

### 第三轮补充：改为默认开启（11:25）

用户决定由自己向上游提交 PR，并且**默认开启** master REST v2。补丁已更新：`patches/seatunnel-application-master-rest.patch`，11 个文件，+208/-18。

- `application.master.rest.enabled` 的默认值改为 **true**（端口 8080）；worker 仍然始终关闭 REST，走 4 参数的 `configure` 重载。
- 端口冲突的报错信息里提示了两种解决办法：改端口，或者设置 `enabled=false`。REST 关闭时不检查端口冲突。
- 测试：`carriesMasterRestSwitchAndEnablesItByDefault`（替换了原来的 DisabledByDefault 用例）；Service 默认暴露 `hazelcast` 和 `rest` 两个端口；新增 `publishesOnlyTheHazelcastPortWhenMasterRestIsDisabled`。4 个测试类共 95 个用例全部通过，spotless:check 通过。
- 文档：`docs/{en,zh}/getting-started/application/kubernetes/configuration.md` 的选项表中增加了这两个选项。
- 给 PR 的提示：REST v2 自带 submit/stop 这类写接口，默认开启后，能访问 master Service 的人就能调用这些接口。描述里应该说明：basic-auth 沿用引擎的 `http` 配置、不支持 HTTPS，并建议用 NetworkPolicy 限制访问。评审时很可能会被问到这一点。

## 关键证据

### 5801 REST 为什么是开着的

- `NodeExtension.createTextCommandService()` 无条件注册了 `RestHttpGet/PostCommandProcessor`（v1，包括 submit-job、stop-job 等写接口）。application 的 `configure()` 只关闭了 v2 使用的 Jetty（`HttpConfig`）。
- master 的启动命令里没有 `-Dhazelcast.config`。`kubernetes.master.hazelcast-config` **没有默认值**（`KubernetesOptions.java:108`），所以加载的是 starter jar 内置的 `hazelcast.yaml`，其中 `rest-api.enabled: true`。发行包 `config/hazelcast*.yaml` 中这一项是 `false`。
- 上游文档 `rest-api-v1.md` 写明 v1 已 deprecated、将来会移除，并且默认已关闭。
- **结论：5801 REST 是默认配置带来的副作用，不是上游的契约。** 但 STX 可以通过 `kubernetes.master.hazelcast-config` **显式**下发一份启用 REST 的 master 配置，把它变成由 STX 控制的行为。长期仍需要推动上游提供正式的只读 REST。

### STX 现有代码可以直接复用

`internal/apps/sync/engine_client.go` 已支持 v1（`APIMode=v1` + `LegacyURL`，L243-L685）：job-info、running/finished 列表、stop/cancel、logs。checkpoint overview/history 只有 v2 才有（L352/L384）。
需要补的只是：把 `LegacyURL` 设为 `https://<apiserver>/api/v1/namespaces/<ns>/services/<appId>:5801/proxy`，并让 HTTP transport 带上 kubeconfig 的鉴权。

### 事件推送的实际语义（源码 + 实测）

- 上报周期**写死为 10 秒**（`JobEventHttpReportHandler.REPORT_INTERVAL`，配置文件里改不了），只有 master 关闭时才会立即清空缓冲。
- `committedEventIndex` 只在 POST 成功后才前进。ringbuffer 容量 2000，满了会覆盖旧事件。新的 handler 从 `headSequence` 开始读，这就是 master 切换后会重推的原因。
- 默认 `report-non-terminal-job-state: false`，只推终态；本次实测设为了 true。
- payload 字段：JOB_STATUS 为 `{jobId, jobName, jobStatus, createdTime, eventType}`；lifecycle 事件为 `{jobId, createdTime, eventType}`，**没有 task/vertex 编号**。
- 事件只有 JOB_STATUS、生命周期、schema change 等几类，**不含指标**。

## 对设计的含义

1. **Application 的运行期监控可以全部由 Go 完成。** 现有的 `EngineClient` v1 模式经 apiserver service proxy 访问即可，不需要 Java、port-forward、Pod 网络或改上游；前提是 STX 显式下发 master 的 hazelcast 配置。原生客户端加 port-forward 的方案降为备选。
2. **事件推送适合做“触发器”，不能作为“真相源”。** 原因：
   - 不带指标；
   - 延迟与 10 秒轮询相当；
   - cancel 时收不到终态；
   - master 切换后会重复，而且没有事件 ID。

   建议：收到事件后立即对该作业做一次拉取；周期性对账保留，但频率可以降低。对 application，STX 自己发起的 cancel 由 STX 直接记为 canceled，不依赖事件。
3. **日志要在 cancel 之前收集**，否则会随 Pod 一起消失；或者转而依赖平台日志采集。
4. **k3s 环境问题**：t2 上的 k3s 在 07:25 因 kine/sqlite 报 fatal（`Transaction commit failed`）退出，systemd 约 15 秒后拉起，Pod 不受影响，但 port-forward 会断开。
