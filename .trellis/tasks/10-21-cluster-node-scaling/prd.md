# 集群节点扩缩容：添加节点支持安装入群与缩容闭环

## Goal

让「集群详情 → 添加节点」成为 SeaTunnel 集群**扩容**的唯一入口，「移除节点」成为可靠的**缩容**入口：
在一台只装了 STX Agent 的主机上，运维人员只通过 UI 即可完成「安装 SeaTunnel → 加入现有集群 → 配置/插件与集群一致 → 其他节点感知新成员」，
移除节点时进程被停止、其他节点不再寻找它、STX 状态与实际一致，全程无需手工改模板、拷插件或登录机器。

背景：2026-10-21 扩容 t2（10.66.0.1）加入 `seatunnel-3-single` 时，现有入口无法完成扩容，靠 CLI 调安装器 + 手工改模板 / sync-all / 重启 / 拷插件才完成，且安装过程反向覆盖了集群模板（per_job 日志配置丢失，已修复）。

## Confirmed Facts（代码与现场核实）

### 扩容现状
- `AddNodeDialog.tsx` 流程：选主机（仅 agent 已安装）→ 安装目录 → 可选「自动发现进程」→ 预检 → `POST /clusters/:id/nodes/batch`。**没有安装分支**，定位是「纳管已运行的 SeaTunnel」。
- `PrecheckNode`（`internal/apps/cluster/service.go:1991`）检查：agent 在线、Hazelcast 端口**正在监听**（=SeaTunnel 已运行才算通过）、目录存在可写、REST API v1/v2 可达；**不检查 Java**。全新主机必然全部失败。
- `handleSubmit`（`AddNodeDialog.tsx:461`）**不校验预检结果**，预检失败仍可提交，产生无法启动的节点记录。
- `AddNodes`（`service.go:1229`）只写库 + 探测一次进程 + 刷新集群状态 + `notifyClusterTopologyChanged`；不安装、不下发配置、不通知其他节点。
- `SetOnClusterTopologyChanged`（`service.go:235`）**全仓库无人注册**，拓扑变更钩子是空操作。
- 「安装 + 入群」能力已存在于安装器：`InstallationRequest.cluster_id` → 安装后 `EnsureNodeForInstallation` → `StartNodeByClusterAndHostAndRole` → `InitClusterConfigs`（`internal/apps/installer/service.go` ~2840–2955）。目前只有「新建集群」向导 `ClusterDeployWizard.tsx:654` 使用，成员地址由前端用 `host.ip_address` 拼。
- **`InitClusterConfigs` 会把新节点本地配置拉回并覆盖集群模板**（`internal/apps/config/service.go:393–446`）。实证：t2 安装后 `hazelcast.yaml`/`jvm_options`/`log4j2.properties`/`hazelcast-client.yaml` 模板版本被抬高，`log4j2.properties` 的 `routingAppender`（per_job）被默认 `fileAppender` 覆盖。
- 安装器只给**新节点**写 member-list（`agent/internal/installer/manager.go:2781`）；现有节点的 hazelcast 配置不变。Hazelcast 只在启动/入群时读取 member-list：
  - 现有节点通告的是可路由地址时，新节点 member-list 含任一现有节点即可入群；但现有节点不更新 member-list，其**重启后可能找不到其他成员而脑裂**。
  - 现有节点 member-list 为 `127.0.0.1` 时（单节点安装常见），其通告地址是回环，新节点**无法入群**，必须让现有节点用可路由地址重启（t1 实例）。
- 插件：新节点不会获得集群已装插件；`InstallPluginToCluster` 对已安装插件拒绝重装（`internal/apps/plugin/service.go:1295`）。已装插件的 profile（如 jdbc→postgresql 驱动）**不入库**（`types.go:199` `gorm:"-"`），仅能从控制面本地插件缓存元数据恢复。存在可复用的 `TransferPluginToAgent`（`service.go:1751`）。
- 成员/IP：节点地址取自 host 的 `ip_address`；多网卡或多机同 IP（instabox eth0 均为 172.16.0.2）时需手工 pin `agent.ip`。

### 缩容现状
- `RemoveNode`（`service.go:1291`）**只删数据库记录**：不停进程、不更新其他节点 member-list、不刷新 agent 监控列表。
- Agent 自动拉起受 `ManuallyStopped` 标记控制（`agent/internal/monitor/monitor.go:445`）；只有经 stop 命令停掉的进程才不会被拉起。直接删除记录 → 进程继续运行并留在 Hazelcast 集群中。
- 监控配置下发（`internal/grpc/handlers.go:1137`）在主机上**没有任何节点时直接跳过**，删光节点后 agent 侧旧的跟踪项不会被清理。
- 前端已有「移除节点」按钮（`ClusterDetail.tsx:1000` → `removeNodeSafe`）。

## Requirements

### 扩容
- **R1 两种添加模式（自动识别，可手动切换）**：添加节点对话框支持「纳管已运行节点」与「安装并加入集群」。选定主机后系统自动探测该主机上运行中的 SeaTunnel（复用进程发现）：探测到则默认「纳管」并回填安装目录/角色，否则默认「安装并加入」；识别结果在对话框中明确展示，用户可手动切换。对话框按向导分步：选主机/识别 → 配置 → 预检（规则随模式切换）→ 执行与进度。（Q1 已决）
- **R2 预检强约束**：两种模式都必须预检通过（或明确的可忽略警告）才能提交。安装模式预检至少包含：agent 在线、Java 可用（只检查不安装）、所需端口空闲、安装目录可创建/可写、磁盘空间。
- **R3 配置真相源**：任何扩容路径都**不得用节点配置覆盖集群模板**（修复 `InitClusterConfigs` 反向覆盖）。
  - 安装模式：新节点配置由集群模板下发生成（member-list 按 R4 生成）。
  - 纳管模式：**以节点为准，只登记不覆盖**——拉取节点现有配置建立该 host 的配置记录，集群模板与节点磁盘文件均不改动；与模板的差异通过现有配置管理的 `match_template=false` 呈现，由用户自行「从模板同步」或「提升为模板」。（Q4 已决）
- **R4 成员地址由后端计算、精确修补**：根据集群节点（host IP + 端口 + 角色）计算完整 member-list；扩容/缩容时对**集群模板与每个节点**的 hazelcast 相关配置（`hazelcast.yaml` / `hazelcast-master.yaml` / `hazelcast-worker.yaml`，按部署模式）**仅修补 `hazelcast.network.join.tcp-ip.member-list` 字段**并推送到节点磁盘，其余内容保持原样；仅写文件，不要求重启（回环修正除外，见 R5）。
- **R5 回环成员修正（扩容流程内显式步骤，必须确认）**：预检识别出现有节点 member-list 含回环地址（`localhost` / `127.0.0.1`，SeaTunnel 发行包默认值即 `localhost`）时，执行计划中单列一步「修正现有节点成员地址，需重启 N 个节点」并说明影响；用户显式勾选确认后方可提交。执行顺序：更新模板并同步 → 逐个重启回环节点（每个等待恢复健康再继续）→ 再安装/纳管新节点入群。集群存在运行中作业时该步骤阻断提交并提示。（Q2 已决）
- **R6 插件一致**：集群已安装插件（含 profile 依赖）在新节点入群过程中自动下发（两种模式均适用，只补缺失、不删除节点上多出的插件）；单个插件失败不阻断入群，结果按插件可见、可重试。
- **R6.1 安装版本与来源**：安装模式的 SeaTunnel 版本固定为集群版本，安装目录/端口默认取集群与现有节点配置；安装包沿用安装器既有来源（控制面缓存，缺失时下载）。
- **R6.2 网络可达预检**：安装/纳管前通过新主机 agent 检查其到现有节点 Hazelcast 端口的 TCP 可达性；不可达时预检失败并提示（典型原因：节点 IP 为不可路由地址、多网卡、防火墙）。入群后以集群成员数（`/overview` workers）校验反向连通。
- **R7 过程可见**：安装入群为异步任务，对话框/节点列表展示分步进度、失败步骤与可重试入口；页面刷新后可恢复查看。

### 缩容
- **R8 移除即停止**：移除节点 = 停止该节点进程（标记手动停止，不被自动拉起）→ 从集群 member-list 模板移除并同步剩余节点 → 删除节点记录 → 刷新该主机 agent 的监控跟踪列表。
- **R9 缩容护栏**：
  - 移除集群最后一个节点：阻断（应走删除集群）。
  - 移除后无在线 master（分离模式最后一个 master / hybrid 最后一个节点）：阻断。
  - 节点进程同时登记在其他集群（同主机 + 同安装目录 + 同角色 = 同一受管进程）：阻断缩容（停进程会影响其他集群），UI 提供「仅移除记录」只从本集群注销。（实施中发现：现场 `XCEC` 与 `seatunnel-3-single` 登记了同一套进程）
  - 「仅移除记录」（主机离线或进程共用时的降级入口）仍不停进程，但需同步剩余节点 member-list（进程共用时不改，避免影响其他集群）并清理该主机在本集群的配置记录；删除主机时清理其在所有集群残留的配置记录，避免 `sync-all` 永远报待推送。（2026-10-23 t2 被删后发现）
  - java-proxy 跟随：java-proxy 承载节点由 `pickSTXJavaProxyNode` 动态选取「首个在线 master」（`stx_java_proxy.go:241`）。若被移除节点是当前承载节点，缩容流程在移除后自动于新选出的 master 上启动 java-proxy 并校验健康；无其他 master 时按上一条阻断。
  - 集群有运行中作业：**允许移除，但必须强确认**。确认框列出运行中作业（数量 + 名称），并展示集群 checkpoint 存储类型；为本地文件存储（如 `fs.defaultFS: file:///`）时以醒目风险提示「作业 failover 到其他节点后无法从 checkpoint 恢复，可能从头运行或失败」；用户勾选「我已了解风险」后方可执行。无运行中作业时走普通确认。运行中作业按集群维度获取（`sync/engine_client.go` `ListRunningJobs` → `/running-jobs`，无法精确到节点）。（Q3 已决）
- **R10 状态一致**：移除完成后 STX 节点列表、集群健康、`/overview` workers 数一致。

## Acceptance Criteria

- [ ] 在只装了 agent、无 SeaTunnel 的主机上，仅通过「添加节点 → 安装并加入集群」完成扩容，无需任何手工命令；完成后 SeaTunnel `/overview` 的 `workers` = 原值 + 1，STX 集群 `healthy`、在线节点数 = 节点数。
- [ ] 扩容后**所有**节点磁盘上的 `hazelcast*.yaml` member-list 包含全部节点地址；集群模板中除 member-list 外的内容与扩容前**完全一致**（模板未被新节点默认配置覆盖，可用版本历史验证）。
- [ ] 新节点 `/opt/seatunnel/connectors` 与插件依赖目录包含集群已装插件及其 profile 依赖（与已有节点一致）。
- [ ] 预检失败（如缺 Java、端口占用、纳管模式端口未监听）时，「添加节点」按钮不可提交，并展示失败项。
- [ ] 现有节点为回环 member-list 时，扩容前给出明确提示与处理路径（按 Q2 决策），不会出现「新节点装好但无法入群 / 脑裂」的静默失败。
- [ ] 移除节点后：该进程被停止，且 5 分钟内未被 agent 自动拉起；剩余节点 member-list 不含该节点；`workers` = 原值 − 1；STX 集群状态正确。
- [ ] 集群有运行中作业 / 移除后无 master / 移除 java-proxy 所在节点时，移除操作被阻断或需显式确认（按 Q 决策）。
- [ ] 前后端新增/修改代码注释中英双语（中文在前）；通过 spec Checklist、lint、type-check、相关单测。

## Out of Scope

- **CLI**：本任务不新增/修改 CLI 命令（用户决定，暂不遵循 AGENTS.md「新增接口补 CLI」约定，后续单独补）。
- JVM 堆等节点级运行参数的变更与下发（本次 1g 是额外运维要求，已手工完成）。
- 自动安装 JDK（STX 不负责，预检只检查）。
- java-proxy 多实例：一个集群一个 java-proxy，新节点不部署。
- Checkpoint / IMAP 存储从本地文件迁移到共享存储（仅可在预检中提示风险）。
- 缩容时卸载/删除节点上的 SeaTunnel 安装目录。

## Open Questions

- ~~Q1 添加节点对话框的模式交互~~ → 已决：自动识别 + 可手动切换（见 R1）。
- ~~Q2 回环 member-list 处理~~ → 已决：扩容流程内显式步骤 + 必须确认 + 有运行中作业时阻断（见 R5）。
- ~~Q3 缩容护栏~~ → 已决：运行中作业允许移除但强确认（含 checkpoint 存储风险提示）；java-proxy 自动跟随新承载 master；最后节点/最后 master 阻断（见 R9）。
- ~~Q4 拓扑变更钩子~~ → 证据已答：Prometheus 目标经 HTTP SD 由控制面按节点表实时计算（`internal/apps/monitoring/service.go:700`），节点增删后自动生效，无需钩子；`SetOnClusterTopologyChanged` 维持现状不注册，扩缩容编排显式调用所需同步。
