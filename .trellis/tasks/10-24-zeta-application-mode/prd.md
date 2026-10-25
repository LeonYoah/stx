# Zeta Application Mode 集成：执行目标抽象与按作业制品

## Goal

让 STX 在**不维护大规模常驻 Zeta 集群**的前提下，把单个 SeaTunnel 作业以 Zeta Application Mode 运行在用户现有的 YARN / Kubernetes 上，并且每个作业只带上自己需要的连接器。用户只需要一个**小型常驻 Zeta 集群做网关**，负责提交、预检查和调试预览。这种模式与现有的常驻集群模式并存，由用户为每个作业选择执行目标。

用户价值：
- 省掉大集群的扩缩容、容量规划这一整套运维成本，常驻部分只剩一个小网关；
- 作业之间资源隔离；
- 制品更轻（只带所需连接器）。

## Confirmed Facts

详见 `research/`：

- `upstream-stip41.md`：上游目前只合入了 K8s provider，YARN 还没有 PR；Application Mode 强制关闭 REST；状态由平台映射，存在 `UNKNOWN` 的特殊语义；尚未包含在任何已发布版本中（dev = 3.0.1-SNAPSHOT）；K8s provider 会覆盖容器 command，并且没有 init 容器或 pod template。
- `dinky-gateway.md`：Dinky 的 gateway 模式、YARN 的 provided lib 与 ship files、K8s 运行期从控制面拉依赖、JM 内回调。
- `stx-current-assets.md`：STX 现有链路全部依赖常驻集群和 REST；可复用 stx-java-proxy、插件依赖管理、K8s 凭据、runtime storage、版本能力矩阵。

## Decisions

- 2026-10-24：**不包含 SeaTunnel on Flink** 执行目标（用户确认）。
- 2026-10-24：本任务作为**父任务**；首个子任务 = **执行目标抽象 + stx-java-proxy gateway + K8s application**（镜像由用户提供、包含完整发行版；基于上游 3.0.1-SNAPSHOT 在 k3s 上验证）。制品层和 YARN 作为后续子任务。
- 2026-10-25：**运行期观测统一按“master 开启 REST v2”设计**（用户确认）。
  - STX 向上游贡献 `application.master.rest.enabled/port`。补丁已在 k3s 上验证，见 `research/k3s-spike.md` 第三轮和 `research/patches/`。
  - STX 复用现有的 v2 `EngineClient`，经 kube-apiserver 的 `services/<appId>:<restPort>/proxy` 访问 master。
  - 日志走 K8s `pods/log`。
  - 事件推送（`event-report-http`）只作为触发器，不作为状态的唯一来源。
  - 停止顺序：先通过引擎 v2 `stop-job` 停止，再收集日志，最后由平台清理资源。
- 2026-10-25：**交互模型分两层**（用户确认）：
  - 常驻的 Application 集群**复用 `clusters` 表**（`deployment_mode=application`，`platform=kubernetes|yarn`），承担执行目标和网关的角色；
  - 每次运行的 Application 实例存放在**独立的 `application_instances` 表**中，不登记为集群。
- 2026-10-25：**K8s 先行，YARN 留口子**：模型、表、Go 侧 `ApplicationPlatform` 接口都按平台无关设计，第一期只实现 kubernetes（见 `design.md` 3.4）。
- 2026-10-25：**使用 Application 模式必须先有一个小型常驻 Zeta 集群做网关**（用户确认方案 A）：
  - Application 集群本身没有节点，通过 `config.application.gateway_cluster_id` 引用一个 standalone 集群；
  - 预检查（DAG、Catalog、plugin template、runtime storage 浏览）和调试预览由网关集群执行，正式运行时才启动 application；
  - 提交命令在网关集群的节点上执行，使用该集群的 `SEATUNNEL_HOME`；
  - 一个网关集群可以同时服务多个 Application 集群；
  - 版本必须一致：网关集群和 application 镜像都要 ≥ 3.1.0，发行包要带 `resource-managers/<platform>`。
  - 理由：`PreviewTask` 依赖常驻集群的 REST（`service.go:1153`），如果每次预览都起一个 application，要等 10–20 秒并创建 3 个 Pod；另外现有的安装、插件同步、proxy、agent 链路都可以原样复用，不需要新增一种“只装发行包”的网关节点角色。
- 2026-10-25：**提交通道一步到位走 proxy，动态加载平台 provider**（用户确认，理由是后期可扩展性强）：
  - proxy 按平台创建 classloader，内容为 bridge jar 加 `$SEATUNNEL_HOME/resource-managers/<platform>/*`，调用期间切换 TCCL，这与上游 `ServiceLoader` 使用 TCCL 的契约一致；
  - bridge 是一个新模块，按 3.1.0 编译，只在平台 classloader 中加载，现有的 epoch-v2/v3 proxy 不受影响；
  - 新平台只需要放入 provider jar，STX 侧零代码发现；
  - 上游的 kubeconfig 只接受文件路径，过渡期由 proxy 临时落盘（0600、用完即删），同时推动上游支持直接传内容。详见 `design.md` 3.6。
- 2026-10-25：**控制面引入 client-go**，封装在共享包 `internal/pkg/kubex`（用户确认）。理由：后续要和 K8s 深度集成（informer、patch、port-forward、资源浏览等），不自己造轮子。
- 2026-10-25：**平台资源的保留与回收依赖 K8s TTL**（用户确认）：
  - 上游 `finished-job-retention-seconds` 写入 Job 的 `ttlSecondsAfterFinished`。STX 的默认值为 **1 小时**（上游默认 24 小时）；
  - 实例上提供“保留现场”（patch TTL，每次延长到 +24 小时，上限 7 天）和“立即回收”；
  - 作业失败不自动延长；STX 不维护自己的回收计时器。
- 2026-10-25：**日志在运行期间流式采集，归档到控制面本地**（用户确认）：
  - 已核实：application 结束时，master 会以宽限期 0 删除所有 worker Pod（`ApplicationResourceManager.java:246`），所以结束后再归档做不到；
  - 方案：client-go informer 发现 Pod，用 `GetLogs(Follow, Timestamps)` 写入 `storage.base_dir/application-logs/<instance_id>/<pod>.log`；
  - STX 重启后按时间戳续采；STX 停机期间丢失的部分标记为“日志不完整”；
  - 单个 Pod 的日志上限 200MB，保留尾部；
  - 读取时复用现有的日志接口；
  - 同时推动上游把“作业结束时删除 worker”改为可配置。
- 2026-10-25：**上游补丁由用户提交 PR，并把 master REST v2 改为默认开启**（`application.master.rest.enabled` 默认 true，端口 8080；worker 始终不开）：
  - 第一期**只支持带 master REST 的版本，不做 v1 兜底**；
  - 网关 proxy 的 bridge 检测这个选项是否存在，并作为一项能力上报，没有这项能力就不允许创建 Application 集群；
  - STX 提交时仍然**显式**设置 `enabled=true` 和端口，不依赖上游默认值；
  - 上游 3.1.0 正式发布并包含这个补丁之前，该功能标为“实验性”。
- 上游基线更新：dev 已升到 **3.1.0-SNAPSHOT**（`7604a83`），Application Mode 最早随 3.1.0 发布。

## Requirements（草案，待逐项确认）

- R1 执行目标抽象：常驻 standalone 集群（现状）与 Application 集群（`deployment_mode=application`，`platform=kubernetes`，YARN 留口子）同级，作业仍然通过 `cluster_id` 绑定其中一个；每次运行产生一条 `application_instances` 记录。
- R1a 网关集群：Application 集群必须引用一个运行中的 standalone 集群作为网关。
  - 创建和修改时校验：网关集群在线、版本一致、发行包中带有对应平台的 provider。
  - 被引用的网关集群不允许删除；升级网关集群时，提示受影响的 Application 集群。
  - 绑定 Application 集群的作业，其预检查和调试预览透明地路由到网关集群执行。
- R2 校验、提交、平台状态和平台清理都经网关集群的 stx-java-proxy 执行（进程内调用上游 descriptor，动态加载 provider）；REST 访问、日志和孤儿发现由控制面直连平台 API。差异全部收敛在 `ApplicationPlatform` 驱动中。Go 侧不重写上游的 descriptor 和状态映射。
- R2a proxy 新增 `providers`、`validate`、`deploy`、`status`、`cancel` 五个 application 接口：
  - 使用独立的线程池并设置超时，不影响预检查接口；
  - 返回结构化错误；
  - 不在日志中记录凭据；
  - SeaTunnel < 3.1.0，或缺少 provider 时，返回 `unsupported`。
- R3 观察：运行期的状态、DAG、指标、checkpoint 复用 v2 `EngineClient`（经平台代理访问 master REST）；REST 不可达时用平台状态兜底；日志在运行期间从平台流式采集（K8s 为 `pods/log` follow），归档到控制面本地；事件推送只作为触发器；正确处理 `UNKNOWN`，并且 STX 自己发起的停止要记为对应的终态。
- R4 按作业生成制品：根据作业 DAG 算出所需连接器和依赖，生成精简制品并缓存。
- R5 CLI 能力和 Web 能力对等（遵循项目的 CLI 约定）。
- R6 按 SeaTunnel 版本控制功能开关。

## Acceptance Criteria（父任务级，K8s 第一期；环境为 k3s t2/t3，控制面在 t1，访问不到 Pod 网段）

- [ ] 网关与集群：
  - 有一个单节点的 standalone 网关集群（3.1.0-SNAPSHOT，带 master REST 补丁），在 UI 和 CLI 上都能基于它创建 K8s Application 集群；
  - 校验会分别指出 Go 侧（RBAC）和 proxy 侧（`validate`）各自的失败原因；
  - 版本不一致、SeaTunnel 低于 3.1.0、缺少 provider、缺少 master REST 能力这几种情况，都会被拒绝并说明原因；
  - 网关集群被引用时不能删除。
- [ ] 预检查与调试：绑定到 Application 集群的作业，plugin template、DAG、Catalog 探测、调试预览都会透明地路由到网关集群，并且执行成功。
- [ ] 运行与观测：
  - 每次运行都会生成一条 `application_instances` 记录，状态依次为 SUBMITTING → DEPLOYING → RUNNING；
  - 工作台能显示 DAG、指标、checkpoint 概览和历史，数据经 apiserver 的 service proxy 获取；
  - 控制面不需要 Pod 网络，也不需要 port-forward。
- [ ] 日志：
  - 运行中和结束后，都能看到 master 和**所有 worker** 的日志，包括 worker Pod 已被删除的情况；
  - 作业运行中重启 STX，日志会续采；漏采的部分在实例上标记为“日志不完整”。
- [ ] 停止与终态：
  - 带 savepoint 停止、不带 savepoint 停止、批作业自然结束、作业失败，这四种情况都能记录为正确的终态；
  - 最终指标标注采样时间。
- [ ] 回收：
  - 默认在作业结束 1 小时后由 K8s TTL 回收，保留时长可以配置；
  - “保留现场”能延长保留时间，最长 7 天；“立即回收”立即生效；
  - Job 被回收后，实例状态变为 CLEANED，实例记录保留。
- [ ] 恢复：配置共享 checkpoint 存储后，流作业可以通过 `--restore-job-id` 从 checkpoint 恢复，恢复时生成新的实例和新的 JobInstance（`RecoveredFromInstanceID`）。
- [ ] 孤儿：
  - 带 STX label、但数据库中没有对应记录的 Job，能被发现并给出提示；
  - 数据库中有记录、但对应 Job 已不存在的实例，标记为 CLEANED 或 UNKNOWN。
- [ ] 新增的接口都有对应的 CLI 能力；现有 standalone 集群的流程回归测试全部通过。

## Out of Scope

- SeaTunnel on Flink / Spark 执行目标。
- Kerberos（上游 v1 不支持）、master HA、worker 自动替换、autoscaling、Session Cluster。
- 用 Go 重写上游的 YARN/K8s descriptor。

## Open Questions

1. YARN provider 是由 STX 团队在上游贡献，还是等社区？
2. K8s 上按作业携带连接器：推动上游补 pod template / init 能力，还是由 STX 构建镜像？
3. ~~作业内部数据在没有 REST 时怎么获取~~ → 已决策（2026-10-25）：上游开启 master REST v2。如果上游合入之前需要先交付，过渡方案是 5801 上的 v1，或 port-forward 加原生客户端，见 `research/k3s-spike.md`。
4. 交互模型的剩余决策（见 `design.md` 第 4 节）：
   - ~~平台资源保留时长、保留现场~~ → ✅ 见 Decisions；
   - ~~提交通道~~ → ✅ proxy；
   - ~~控制面访问平台 API 的方式~~ → ✅ client-go。
5. ~~作业结束后的终态快照~~ → 默认接受“最终指标 = 最后一次采样”：
   - 收到 FINISHED 事件或 informer 通知后立即拉取一次，尽量缩小差距；
   - UI 标注“最终指标采样于 <时间>”；
   - 同时推动上游支持 master 在作业结束后延迟 N 秒退出。
   - 依据：spike 测得事件到达 STX 时，master 只剩约 30ms 就关闭，在没有上游支持的情况下没有更好的办法。
6. ~~第一期对上游补丁的依赖方式~~ → ✅ 见 Decisions（只支持带 master REST 的版本，不做 v1 兜底）。
7. YARN provider、按作业制品（原第 1、2 项）放到各自子任务规划时再定，不阻塞第一个子任务。
