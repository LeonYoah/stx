# 设计：集群节点扩缩容

> 需求见 `prd.md`（R1–R10、Q1–Q4 已决）。本文只描述技术方案。
> 相关规范：`spec/backend/execution-security-guidelines.md`（§1–3、§9、§10）、`spec/backend/managed-process-identity.md`、`spec/backend/database-guidelines.md`、`spec/guides/cross-layer-thinking-guide.md`、`spec/frontend/ui-conventions.md`（§4、§7、§8）、`spec/frontend/api-and-services.md`。

## 1. 总体思路

把扩容、缩容都建模成**集群级「伸缩操作」（Scale Operation）**：先 **Plan（只读预检 + 执行计划）**，用户确认后 **Execute（异步编排、分步持久化）**。
编排器只组合现有能力（安装器、配置服务、插件服务、节点启停、Agent 预检子命令），新增能力只有三类：

1. 成员地址计算与 **member-list 精确修补**（模板 + 每个 host 配置分别修补并推送）。
2. **非覆盖式**配置登记（修复 `InitClusterConfigs` 反向覆盖模板）。
3. 伸缩操作的持久化、互斥、进度与公共执行（execution）接入。

```
前端 AddNodeDialog（向导）            前端 RemoveNodeDialog（强确认）
   │ discoverProcesses                    │
   │ POST scale-out/plan  (R0)            │ POST nodes/:nodeId/scale-in/plan (R0)
   │ POST scale-out       (R2, async)     │ POST nodes/:nodeId/scale-in      (R2, async)
   │ GET  scale-operations/:opId  ◄───────┴──── 轮询进度
   ▼
cluster.ScaleService（新，internal/apps/cluster/scale_*.go）
   ├─ planner：成员计算 / 回环检测 / 预检 / 护栏 / 插件清单 / 运行中作业
   ├─ executor：步骤编排、断点重试、持久化 ScaleOperation
   ├─ config.Service：PatchMemberList / RegisterHostConfigsFromNode / ApplyTemplatesToHost
   ├─ installer.Service：StartInstallation(cluster_id, defer_node_start)
   ├─ plugin.Service：ReplayClusterPluginsToHost
   ├─ 节点启停：StopNode / StartNode / RestartNode（含 ManuallyStopped 语义）
   └─ Agent 预检子命令：check_tcp / check_port / check_java / check_directory / check_cluster_members
```

## 2. 模块边界与改动点

| 层 | 模块 / 文件 | 改动 |
|---|---|---|
| 后端 | `internal/apps/cluster/scale_plan.go`（新） | 扩/缩容 Plan 计算：成员列表、回环检测、护栏、预检汇总 |
| 后端 | `internal/apps/cluster/scale_executor.go`（新） | 异步编排、步骤状态机、断点重试、互斥 |
| 后端 | `internal/apps/cluster/scale_model.go`（新） | `ClusterScaleOperation` GORM model + Repository |
| 后端 | `internal/apps/cluster/handler.go` / `router.go` | 新路由（§5），旧路由保持 |
| 后端 | `internal/apps/config/service.go` | 新增 `PatchMemberList`、`RegisterHostConfigsFromNode`、`ApplyTemplatesToHost`、`EnsureClusterConfigsForHost`（不覆盖模板）；`InitClusterConfigs` 显式接口语义不变 |
| 后端 | `internal/apps/installer/service.go` | 安装请求新增内部字段 `DeferNodeStart`（不出现在公开 JSON，`json:"-"`）；为真时安装 + `EnsureNodeForInstallation` 后不启动、不调 `InitClusterConfigs`，交给编排器 |
| 后端 | `internal/apps/plugin/service.go` | 新增 `ReplayClusterPluginsToHost(clusterID, hostID, installDir)`：按集群已装插件 + 本地缓存元数据中的 profile 调 `TransferPluginToAgent`，只补缺失 |
| 后端 | `internal/grpc/handlers.go` | `pushMonitorConfigToAgent` 增加按 `cluster_id` 限定的下发；主机上该集群已无节点时**下发空列表**以清理跟踪项（当前直接跳过） |
| 后端 | `internal/db/migrator/migrator.go` | 注册新 model |
| 后端 | `internal/operation/registry.go`、`route_baseline.json`、`docs/swagger.*` | 新路由登记（API 契约要求，与 CLI 无关） |
| Agent | 无 | 复用既有子命令；不改 proto |
| 前端 | `components/common/cluster/AddNodeDialog.tsx` | 改为向导：识别 → 配置 → 预检/计划 → 执行进度 |
| 前端 | `components/common/cluster/RemoveNodeDialog.tsx`（新，自 `ClusterDetail.tsx` 抽出） | 缩容计划展示 + 强确认 + 进度 |
| 前端 | `lib/services/cluster/cluster.service.ts`、`types.ts` | 新 API 与类型；R2 两步确认沿用 `createWebExecutionHeaders`（参考 `st-upgrade.service.ts`） |
| 前端 | `lib/i18n/locales/{zh,en}.json` | 文案 |
| CLI | 无 | 本任务不做（PRD Out of Scope） |

## 3. 关键算法与契约

### 3.1 成员地址计算 `computeMemberList(cluster, nodes, extra?, exclude?)`

- 输入：集群部署模式、节点（host IP、role、hazelcast_port、worker_port）、待加入节点、待移除节点。
- hybrid：每个节点 `<hostIP>:<hazelcast_port>`。
- separated：master 节点 `<ip>:<master_port>` + worker 节点 `<ip>:<worker_port>`（与 agent 安装器 `manager.go:2781` 规则一致）。规则抽成共享纯函数包 `internal/clustermembers`，控制面与 Agent（`agent/go.mod` 已 `replace github.com/LeonYoah/stx => ..`，同 `internal/processidentity` 的共享方式）共用，避免两处漂移。
- host IP 为空、回环或与其他节点重复（如多台 `172.16.0.2`）时 Plan 直接给出阻断项「节点地址不可路由/重复，请在主机管理修正 IP（必要时在 Agent 配置 `agent.ip`）」。
- 输出稳定排序，便于对比与幂等。

### 3.2 member-list 精确修补 `config.PatchMemberList(clusterID, deploymentMode, members, opRef, userID)`

- `deploymentMode` 由调用方传入（配置服务不感知集群模式）；为空时修补全部三个 hazelcast 文件。

- 目标配置类型按模式：hybrid → `hazelcast.yaml`；separated → `hazelcast-master.yaml`、`hazelcast-worker.yaml`（若存在 `hazelcast.yaml` 也一并修补）。
- 对**模板**和**每个 host 配置**分别：解析 YAML（`yaml.v3` Node，保留注释与顺序）→ 仅替换 `hazelcast.network.join.tcp-ip.member-list` → 内容有变化才 `Update` 生成新版本（comment 标注伸缩操作 ID）→ 对 host 配置调用推送写盘。
- 不触碰其他字段，因此纳管节点与模板的既有差异保持（Q4）。
- 推送失败的节点进入步骤结果 `push_errors`，步骤状态为 `partial`，可重试。
- 不重启任何节点（回环修正步骤除外）。

### 3.3 回环检测（R5）

- 静态：节点 host 配置中 member-list 含 `localhost` / `127.*` / `::1`。
- 运行时：对在线节点调 `check_cluster_members`，成员地址为回环即判定。
- 任一命中 → Plan 输出 `loopback_nodes[]` 与 `requires_restart_ack=true`；集群有运行中作业时该项为阻断（`blocking`）。

### 3.4 配置登记（R3）

- `InitClusterConfigs` **保持原语义不变**：它也是配置管理页「从节点初始化配置」的显式接口（`config/handler.go:444`），用户主动要求以节点为准时覆盖模板是预期行为。
- 新增 `EnsureClusterConfigsForHost`（安装器路径专用）：模板**不存在**时由节点文件创建模板；模板已存在时**只**创建/刷新该 host 配置，不改模板。安装器 `installer/service.go:2947` 改调此函数（新建集群向导首节点行为不变；后续节点不再覆盖模板）。
- `RegisterHostConfigsFromNode`（纳管）：拉取节点各配置类型 → 建立 host 配置记录（`match_template` 自然反映差异）；不写盘、不改模板。
- `ApplyTemplatesToHost`（安装）：以模板内容（member-list 已替换为完整列表）为该 host 生成配置记录并推送写盘，在节点**启动前**执行。

### 3.5 插件重放（R6）

- 清单：`ListInstalledPlugins(clusterID)` 中除「已卸载 / 安装失败」外的全部记录（含 `disabled`、`partial`：启用只改库，新节点缺 jar 会导致日后重新启用失效）+ 本地缓存元数据 `SelectedProfileKeys`。
- 先 `LIST_PLUGINS` 取节点现有插件，只补缺失；逐个 `TransferPluginToAgent`。
- 元数据缺失时先按控制面缓存反推画像：非默认画像相对默认画像额外引入的依赖若全部已缓存即视为已选（`profile_inferred`，只会传输缓存中已有的文件）；仍无法推断才以默认 profile 下发并标注 `profile_unknown`，不阻断。
- 连接器已在节点上时不再直接跳过：若有非连接器依赖（驱动 jar），只补传这些依赖（节点无法列出依赖目录，同名覆盖幂等），不重传连接器。（实测发现：t2 只拿到 jdbc 连接器、缺驱动）
- 结果写入步骤 `plugin_results[]`；任一失败 → 步骤 `partial`，可单独重试。

### 3.6 运行中作业与 checkpoint 存储（R9 / Q3）

- 运行中作业：复用 `sync` 的 `ListRunningJobs`（`/running-jobs`）；通过接口注入集群服务，避免 cluster→sync 包直接依赖。
- checkpoint 存储类型：解析模板 `seatunnel.yaml` 的 `checkpoint.storage`（`type` + `fs.defaultFS`）；`file://` 或本地路径判定为 `local`。

## 4. 执行编排

### 4.1 扩容步骤（`kind=scale_out`）

| # | 步骤 | 安装模式 | 纳管模式 | 可重试 |
|---|---|---|---|---|
| 1 | `validate_plan`：重算 Plan，与提交时指纹比对，变化则失败并要求重新预检 | ✓ | ✓ | ✓ |
| 2 | `fix_loopback`（仅 Plan 含回环节点）：修补模板与回环节点 member-list 为**扩容后的完整列表 `members_after`（含尚未启动的新节点）** → 逐个 `RestartNode` → `check_cluster_members` 等待健康且非回环（超时 180s/节点） | ✓ | ✓ | ✓ |
| 3 | `install_files`：`StartInstallation(cluster_id, defer_node_start=true, master_addresses=完整成员 IP)`，轮询安装状态 | ✓ | — | ✓ |
| 4 | `register_node`：`EnsureNodeForInstallation`（安装模式由安装器完成，此处校验） / 纳管模式创建节点记录（等价 `AddNodes`） | ✓ | ✓ | ✓ |
| 5 | `apply_configs`：安装 → `ApplyTemplatesToHost`；纳管 → `RegisterHostConfigsFromNode` | ✓ | ✓ | ✓ |
| 6 | `replay_plugins` | ✓ | ✓ | ✓（partial 不阻断） |
| 7 | `start_node`：`StartNode` | ✓ | — | ✓ |
| 8 | `patch_members`：对全部节点（含新节点）`PatchMemberList` | ✓ | ✓ | ✓ |
| 9 | `verify_membership`：新节点成员须包含自身与 `validate_plan` 记录的基线（操作开始时、任何重启前运行中的现有节点），成员数 ≥ 基线 + 1；不以校验时 DB 状态计数（实测：刚重启的节点被漏算，新节点自成集群却判成功）；刷新集群状态 | ✓ | ✓ | ✓ |
| 10 | `rolling_restart`（**可选**，提交时 `rolling_restart=true`）：入群后逐个重启现有节点（不含回环节点与新节点），每个节点重启后须看到全部运行中成员再继续；每次重启前复核运行中作业，出现即停止。原因：Hazelcast 只在启动时读取 tcp-ip member-list，分裂后的合并只探测启动时的列表；扩容只改文件不重启，现有节点在下次重启前无法在分裂后找回新节点（2026-10-22 实测分裂不自愈）。计划返回 `rolling_restart{nodes, available, recommended, blocked_reason, note}`：有运行中作业不可选，作业未知可选但提示；不进指纹，提交与 `validate_plan` 复核 | ✓ | ✓ | ✓ |

### 4.2 缩容步骤（`kind=scale_in`）

| # | 步骤 | 说明 |
|---|---|---|
| 1 | `validate_plan` | 重算护栏：最后节点 / 最后 master / 进程被其他集群共用（`shared_process`，按 `ManagedName(cleanInstallDir, role)` 判定）阻断；运行中作业需 `jobs_risk_ack=true` |
| 2 | `stop_node` | 复用节点停止（下发前 `MARK_MANUAL_STOP`，失败时清除，遵循进程身份规范）；确认进程已停（`check_process`） |
| 3 | `patch_members` | 以「剩余节点」计算并修补模板 + 剩余节点 member-list |
| 4 | `cleanup_records` | 删除节点记录；该 host 在本集群已无节点时删除其 host 配置记录 |
| 5 | `refresh_monitor` | 向该 host agent 下发 `cluster_id` 限定的监控配置（可为空列表）以移除跟踪项 |
| 6 | `follow_java_proxy` | 若被移除节点是当前 proxy 承载节点：对新选出的 master 执行 `StartSTXJavaProxy` 并校验健康 |
| 7 | `verify` | 在线节点 `check_cluster_members` 成员数 = 期望值；刷新集群状态 |

- 主机离线导致无法 `stop_node`：步骤失败并提示；UI 提供「仅移除记录（强制）」降级入口，即现有 `DELETE /clusters/:id/nodes/:nodeId`（仍不停进程，确认文案注明进程可能仍在运行）。
- 「仅移除记录」补齐记录层清理（2026-10-23，PR #65；t2 被删后残留 member-list 与配置行导致 `sync-all` 永远报待推送）：`Service.RemoveNode` 删除记录后经 `onNodeRecordRemoved` 钩子调用 `ScaleService.CleanupAfterRecordRemoval`，尽力而为、失败只记日志不影响移除结果：
  - 修补模板 + 剩余节点 member-list（只写文件不重启，下次重启生效）；被移除节点或任一剩余节点进程登记在其他集群（`sharedProcessClusters`）时跳过——节点上的 hazelcast 文件为多个集群共用，修补会改到其他集群；
  - 该 host 在本集群已无节点时 `DeleteHostConfigs`；
  - host 在线时下发本集群监控配置；刷新集群状态。
  - 缩容流程的 `cleanup_records` 直接调用 `repo.RemoveNode`，不会重复触发该钩子。
- 删除主机（要求已不属于任何集群）后经 `host.Service` 的 `onHostDeleted` 钩子调用 `config.DeleteConfigsByHost`，清理该主机在所有集群残留的配置记录与版本（模板不动）。

### 4.3 状态与失败处理

- 步骤状态：`pending / running / succeeded / partial / failed / skipped`；操作状态映射公共执行：`running → succeeded / failed`，`partial` 完成计为 `succeeded` + `warnings`。
- 失败后提供「从失败步骤重试」（`POST .../scale-operations/:opId/retry`，R1）；各步骤需幂等（重复执行结果一致）。
- 不自动回滚：扩容中途失败时，节点可能已部分加入；UI 提示可「重试」或对该节点执行缩容清理。
- 控制面重启：启动时把 `running` 的伸缩操作标记为 `failed`（原因 `interrupted_by_restart`），可重试。
- 互斥：同一集群同时只允许一个 `running` 伸缩操作（DB 条件查询 + 进程内锁），冲突返回 `409 cluster_scale_in_progress`。

## 5. API 契约

| 方法 / 路径 | 风险 | 说明 |
|---|---|---|
| `POST /api/v1/clusters/:id/scale-out/plan` | R0 | body：`host_id, mode(install/register), install_dir, ports{...}`；返回 Plan（§5.1） |
| `POST /api/v1/clusters/:id/scale-out` | R2 | body：同上 + `plan_fingerprint` + `loopback_restart_ack`；返回 `execution_id, operation_id` |
| `POST /api/v1/clusters/:id/nodes/:nodeId/scale-in/plan` | R0 | 返回缩容 Plan |
| `POST /api/v1/clusters/:id/nodes/:nodeId/scale-in` | R2 | body：`plan_fingerprint` + `jobs_risk_ack`；返回 `execution_id, operation_id` |
| `GET  /api/v1/clusters/:id/scale-operations/:opId` | R0 | 操作详情与步骤 |
| `GET  /api/v1/clusters/:id/scale-operations?status=running` | R0 | 刷新页面后恢复进行中的操作 |
| `POST /api/v1/clusters/:id/scale-operations/:opId/retry` | R1 | 从失败步骤重试 |

- 写路由经过登录中间件，接入 `internal/apps/execution`（module=`cluster`，module_ref=`scale:<opId>`），审计记录 request_id / execution_id / risk_level。
- 旧路由 `POST nodes` / `nodes/batch` / `DELETE nodes/:nodeId` / `nodes/precheck` 保持原语义（新建集群向导与 CLI 仍在用）。

### 5.1 Plan 响应结构（空数组一律返回 `[]`，不返回 `null`）

```json
{
  "kind": "scale_out",
  "mode": "install",
  "detected_mode": "install",
  "fingerprint": "sha256:...",
  "members_before": ["10.66.0.2:5801"],
  "members_after":  ["10.66.0.2:5801", "10.66.0.1:5801"],
  "checks": [{"name": "java", "status": "passed|failed|warning", "blocking": true, "message": "..."}],
  "loopback_nodes": [{"node_id": 1, "host_name": "t1", "address": "127.0.0.1:5801"}],
  "requires_restart_ack": true,
  "plugins_to_replay": [{"name": "jdbc", "version": "3.0.0", "profile_keys": ["postgresql"]}],
  "running_jobs": [{"job_id": "...", "job_name": "..."}],
  "checkpoint_storage": {"type": "hdfs", "location": "file:///tmp/", "local": true},
  "java_proxy_follow": null,
  "steps": ["validate_plan", "fix_loopback", "install_files", "..."],
  "can_submit": false,
  "blocking_reasons": ["..."]
}
```

- `fingerprint` = 对 members_before/after、loopback_nodes、mode、host、端口、插件清单、运行中作业 ID 集合求摘要；执行时重算不一致即拒绝（防止「看到的计划」与「执行的计划」不同）。

### 5.2 预检项（随模式切换）

| 检查 | 安装 | 纳管 | 来源 |
|---|---|---|---|
| agent 在线 | 阻断 | 阻断 | host 状态 |
| Java 可用 | 阻断 | — | `check_java` |
| 端口空闲 | 阻断 | — | `check_port`（期望未监听） |
| 端口监听 | — | 阻断 | `check_port`（期望监听） |
| 目录可写 / 可创建 | 阻断 | 阻断 | `check_directory` |
| 磁盘空间 | 警告 | — | 安装器既有 precheck |
| 到现有节点 Hazelcast 端口可达 | 阻断 | 阻断 | `check_tcp`（新主机 agent 发起） |
| SeaTunnel 版本一致 | — | 阻断 | REST `/overview` projectVersion |
| 节点地址可路由且不重复 | 阻断 | 阻断 | §3.1 |
| 回环成员 | 需确认 / 有作业阻断 | 同左 | §3.3 |

## 6. 持久化模型

```go
// ClusterScaleOperation 伸缩操作（扩容/缩容）及其分步状态。
// ClusterScaleOperation is a scale-out/scale-in operation with per-step status.
type ClusterScaleOperation struct {
    ID          uint      `gorm:"primaryKey"`
    ExecutionID string    `gorm:"size:64;index"`
    ClusterID   uint      `gorm:"index;not null"`
    HostID      uint      `gorm:"index"`
    NodeIDs     string    `gorm:"type:text"`          // JSON []uint
    Kind        string    `gorm:"size:20;not null"`   // scale_out / scale_in
    Mode        string    `gorm:"size:20"`            // install / register
    Status      string    `gorm:"size:20;index"`
    CurrentStep string    `gorm:"size:40"`
    PlanJSON    string    `gorm:"type:text"`          // 提交时的 Plan 快照（已脱敏）
    StepsJSON   string    `gorm:"type:text"`          // []StepState
    Error       string    `gorm:"type:text"`
    CreatedBy   uint
    CreatedAt, UpdatedAt time.Time
    FinishedAt  *time.Time
}
```

- 三库兼容：仅用 `text`/`size` 基本类型；提交前运行 `./scripts/test_db_compat.sh`。

## 7. 前端交互

- **AddNodeDialog 向导**（`sm:max-w-4xl`，内部滚动；步骤指示器，实现为三步）：
  1. 选主机（分离模式另选 master / worker）→「下一步」只带主机与角色请求 plan，由后端识别模式并给出默认目录与端口（不再单独调用 `discoverProcesses`，识别逻辑只在后端一处）。
  2. 识别 + 配置 + 计划：展示识别结果与模式切换（ToggleGroup）、安装目录与端口；任一配置变更即标记 dirty（旧计划半透明保留，零塌陷），需「重新预检」后才能提交。计划区为共享组件 `ScalePlanView`：阻断 / 提醒展开、通过项折叠，成员 before→after、插件补齐、回环修正卡片（必勾确认）。`can_submit=false` 时提交按钮禁用。
  3. 执行：R2 两步确认提交 → 共享组件 `ScaleOperationProgress`（2s 轮询、失败步骤错误与「从失败步骤重试」）。409 冲突提示发起人；计划变化自动重新预检。
- **RemoveNodeDialog**：打开即请求缩容 Plan；阻断项直接展示原因；有运行中作业时列出作业 + checkpoint 存储类型，本地存储显示红色风险提示，必须勾选「我已了解风险」才可提交；主机离线时提供「仅移除记录（强制）」次级动作。
- **恢复**：`ClusterDetail` 加载时查询 `scale-operations?status=running`，存在则在节点区顶部显示进度条，点击打开进度视图。
- 零塌陷加载、i18n 中英、原子信息不换行（地址列表用 `font-mono` + `flex-wrap`）。

## 8. 兼容性与发布

- 旧接口语义不变；`InitClusterConfigs` 的变更只影响「模板已存在」场景（不再覆盖），新建集群首节点行为不变。
- `DeferNodeStart` 为内部字段，不改变公开安装请求 JSON。
- 监控下发改为 `cluster_id` 限定且允许空列表：agent 端已支持按 `cluster_id` 替换（`agent/cmd/main.go:2339`），需单测确认空列表时只清理该集群项。
- 回退：前端可回退到旧对话框（旧接口保留）；数据库新增表不影响旧版本运行。

## 9. 风险与取舍

- **不自动回滚**：自动回滚涉及停进程、改回 member-list、删安装目录，失败面更大；选择「可重试 + 缩容清理」。
- **插件 profile 来源**：依赖控制面本地缓存元数据；缓存被清理时降级为默认 profile 并标注。后续可考虑把 profile 持久化入库（不在本任务）。
- **运行中作业按集群维度**：无法精确到节点，护栏偏保守（Q3 已接受强确认）。
- **hybrid 新节点即 master/worker**：沿用 `normalizeNodeRoleForDeployment`。
