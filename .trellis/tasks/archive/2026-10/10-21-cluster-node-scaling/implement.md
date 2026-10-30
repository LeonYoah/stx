# 执行计划：集群节点扩缩容

> 依据 `prd.md` + `design.md`。按阶段推进，每阶段结束跑对应校验，Review Gate 通过再进入下一阶段。
> 注释中英双语（中文在前）；新增源文件带 Apache License 头；不新增/修改 CLI。

## 阶段 0：准备

- [x] 阅读 `implement.jsonl` 中的 spec 与 `research/t2-scale-out-evidence.md`
- [x] 基线：`go test ./internal/apps/cluster/... ./internal/apps/config/... ./internal/apps/installer/... ./internal/apps/plugin/... ./internal/grpc/...`、`(cd agent && go test ./...)`、`(cd frontend && pnpm test)` 记录当前是否全绿（2026-10-22 基线：cluster/config/plugin/grpc 全绿；installer 2 例预先失败 `TestService_UploadPackageValidation`、`TestService_UploadPackageChunk_Success`（`./data/storage/temp` 与 `/tmp` 跨设备 rename，环境问题）；agent 1 例预先失败 `executor/TestChunkTransferRepliesAreTerminal`（`/tmp/seatunnel-plugins` 属 root 无写权限）；frontend 35 文件 / 229 用例全绿）

## 阶段 1：后端基础能力（无行为可见变化）

1. [x] `internal/clustermembers`（新）：`Compute(mode, nodes, add, remove) []string`、`IsLoopback(addr) bool`、`Validate(nodes)`（空/回环/重复 IP）；单测覆盖 hybrid / separated / 去重排序 / 回环 / 重复 IP。
2. [x] Agent 安装器 member-list 生成改为调用 `clustermembers`（行为不变，补单测对比旧输出）。
3. [x] `config.Service`：
   - [x] 新增 `EnsureClusterConfigsForHost`：模板不存在才创建，已存在只建/刷 host 配置；安装器 `configInitializer` 接口与 `installer/service.go:2947` 改调它。`InitClusterConfigs`（配置页显式「从节点初始化」）语义不变。单测：首节点建模板、第二节点不改模板版本；显式 Init 仍覆盖。
   - [x] `PatchMemberList(ctx, clusterID, members, opRef, userID)`：yaml.v3 Node 精确替换，模板 + host 配置分别更新并推送；单测：其他字段/注释不变、内容不变不出新版本、推送失败返回 partial。（实现签名增加 `deploymentMode` 参数：`PatchMemberList(ctx, clusterID, deploymentMode, members, opRef, userID)`；按行精确替换优先，必要时退回 yaml.v3 Node 重写，两者都重新解析校验）
   - [x] `RegisterHostConfigsFromNode`、`ApplyTemplatesToHost`；单测。（`ApplyTemplatesToHost(ctx, clusterID, hostID, deploymentMode, members, opRef, userID)`）
4. [x] `plugin.Service.ReplayClusterPluginsToHost`：只补缺失、profile 来自本地元数据、缺元数据降级标注；单测（mock AgentCommandSender）。
5. [x] `grpc` 监控下发：支持按 `cluster_id` 下发且允许空列表；单测 + agent 侧单测（空列表只清理该 cluster 项，其他集群项保留——对应进程身份规范必需测试）。
6. [x] 安装器内部字段 `DeferNodeStart`（`json:"-"`）：为真时 `EnsureNodeForInstallation` 后返回，不启动、不 `InitClusterConfigs`；单测：登记在前、不调用启动、状态 `complete`。

校验：`go test ./internal/... && (cd agent && go test ./...)`；`go vet ./...`
**Review Gate 1**：模板不再被覆盖、member-list 修补不改其他字段（附单测输出）。

## 阶段 2：伸缩操作编排（后端）

7. [x] `scale_model.go`：`ClusterScaleOperation` + Repository；注册到 `internal/db/migrator/migrator.go`；`./scripts/test_db_compat.sh`。（模型增加 `loopback_restart_ack`、`jobs_risk_ack`、`lang`；`PlanJSON`/`StepsJSON` 用 `db.ScriptText`；本机无 PG/MySQL，兼容脚本只跑了静态扫描与 SQLite）
8. [x] `scale_plan.go`：扩容 / 缩容 Plan（预检矩阵 design §5.2、回环检测 §3.3、护栏 §4.2、运行中作业与 checkpoint 存储 §3.6、fingerprint）；运行中作业通过接口注入（不直接依赖 sync 包）。（router 适配 `sync.ListClusterRunningJobs`、`discovery.DiscoverProcesses`；纳管版本取发现结果；磁盘空间取心跳 `disk_usage`）
9. [x] `scale_executor.go`：步骤状态机（design §4.1 / §4.2）、断点重试、集群级互斥（409）、控制面重启标记 `interrupted_by_restart`、公共执行接入（R2 提交、R1 重试、审计）。（提交/重试/查询在 `scale_service.go`；每次重试新建一条 R1 执行记录并更新操作的 `execution_id`）
10. [x] Handler + 路由（design §5），写路由经登录中间件；`internal/operation/registry.go`、`internal/operation/testdata/route_baseline.json`、`docs/docs.go`、`docs/swagger.json`、`docs/swagger.yaml` 同步。（`scale_handler.go`；docs 用 `scripts/swagger.sh` 同款 swag v1.16.5 重新生成，顺带补齐了 #52 遗留的 ClusterStatusInfo/plugin 定义漂移）
11. [x] 单测：Plan 各阻断/警告分支；执行器每步幂等重跑；失败→重试从失败步继续；互斥；缩容 stop 失败不删记录；java-proxy 跟随；`requires_restart_ack` / `jobs_risk_ack` 缺失拒绝。（`internal/apps/cluster/scale_test.go`，另含 R2 确认+幂等、重启打断恢复、归属过滤；`-race` 通过）
    - 审阅修正（`scale_guardrail_test.go`）：409 携带进行中操作编号/类型/发起人（新增 `created_by_name` 列）；指纹只记作业「有无 / 可否确认」不记 ID；回环修正 + 作业未知时追加 `loopback_restart_jobs_unknown` 警告；纳管模式新增 `target_joined_cluster` 组网警告。

校验：`go test ./...`；`go test ./internal/operation/contract/...`（路由契约）；`./scripts/test_db_compat.sh`
**Review Gate 2**：API 契约（Plan JSON 空数组为 `[]`）、R2 流程、步骤幂等。

## 阶段 3：前端

12. [x] 独立 `ClusterScaleService`（`lib/services/cluster/scale.service.ts` + `scale.types.ts`，挂到 `services.clusterScale`）：plan / R2 两步提交 / R1 重试 / 操作查询；边界统一 null→[]；带 `lang`；409 冲突详情解析 `getScaleInProgressDetail`。
13. [x] `AddNodeDialog` 重写为三步向导（选主机 → 识别+配置+预检 → 执行进度）；识别直接用后端计划的 `detected_mode`（不再单独调进程发现）；提交受 `can_submit`、配置未变更（dirty）与回环确认勾选约束；409 计划变化自动重新预检。
14. [x] `RemoveNodeDialog`：缩容计划、作业 / 本地 checkpoint 风险强确认、目标主机离线时「仅移除记录（强制）」二次确认降级（沿用旧 DELETE）。
15. [x] `ClusterDetail` 进行中操作横幅 + 进度弹窗；弹窗执行中关闭后由横幅接管。共享组件 `scale/ScalePlanView.tsx`、`scale/ScaleOperationProgress.tsx`。
16. [x] i18n `cluster.scale.*`（中英对齐，翻译覆盖率测试通过）；vitest：`scale.service.test.ts`（9）、`ScalePlanView.test.tsx`（3）。`ClusterDetail.tsx` 基线即未 prettier 格式化，未整体格式化以免无关 diff。

校验：`(cd frontend && pnpm lint && pnpm exec tsc --noEmit && pnpm test && pnpm format:check)`
**Review Gate 3**：对照 `ui-conventions.md` Checklist（渐进式披露、弹窗分级、零塌陷、原子信息不换行）。

## 阶段 4：真实环境验证（t1/t2）

> 2026-10-22 实测（经 API + CLI 令牌，非 UI 点击；UI 由组件测试覆盖）。验证前按用户确认删除了误建的重复集群 XCEC（先用旧接口注销其节点记录再删集群，未停进程）。

17. [x] 构建部署到 t1（控制面 + 前端 + agent）与 t2（agent）；旧版本备份在 `/opt/stx/backup-4f4424e/`、t2 `/opt/stx-agent/bin/stx-agent.bak-4f4424e`。
18. [x] 缩容（op #1）：6 步全部成功；t2 进程停止、t1 磁盘与模板 member-list 仅剩 `10.66.0.2:5801`、`workers=1`、集群 healthy、t2 主机配置清理。「不被拉起」实际观察约 3 分钟（随后为扩容验证移走了安装目录）。
19. [x] 扩容安装模式（op #2）：9 步全部成功；`workers=2`、两节点 member-list 完整；模板 10 份中 9 份内容不变、`hazelcast.yaml` 仅 member-list（排序）与手工复现时带入的末尾空行变化；t2 使用模板配置（`-Xmx1g`、per_job `routingAppender`）。**发现**：jdbc 只补了连接器、缺 PostgreSQL/PostGIS 驱动（缓存无画像元数据）→ 已修复，见下。
20. [x] 回环场景（与 19 同一次）：t1 复现为通告 `[127.0.0.1]:5801`，计划出现回环节点与必勾重启确认，`fix_loopback` 修正并重启 t1 后入群。**发现**：`verify_membership` 在新节点仅看到自己时误判成功（期望值按校验时 DB running 计数，刚重启的 t1 被漏算）→ 已修复，见下。
21. [x] 纳管模式（op #3 缩容 + op #4 纳管）：t2 手工启动并自行入群后，计划自动识别为 register、`target_joined_cluster` 通过；6 步全部成功；t2 配置只登记不覆盖（10 份 `match_template=true`）；驱动补齐与 t1 一致。
22. [x] 有运行中作业时缩容（仅计划 + 无确认提交）：`requires_jobs_ack=true`、列出作业、本地 checkpoint（`file:///tmp/`）风险提示；不勾确认提交返回 400 `scale_jobs_risk_ack_required`。测试作业已停止。

### 实测发现并已修复（均有回归测试）

- **入群误判**：`validate_plan` 记录操作开始时（任何重启前）运行中的现有节点为基线，`verify_membership` 要求新节点成员包含全部基线地址；旧操作无基线时回退当前运行节点。回归：`TestScaleOutVerifyRejectsIsolatedNewNode`（旧逻辑下失败、新逻辑通过）。
- **插件画像缺失**：元数据缺失时按控制面缓存中已存在的依赖反推画像（`profile_inferred`），只会传输缓存中已有的文件。回归：`TestReplayInfersProfileFromCacheWhenMetadataMissing`。
- **连接器已存在不补依赖**：连接器已在节点时只补传非连接器依赖（驱动 jar，幂等覆盖），不重传连接器。回归：`TestReplayClusterPluginsToHostOnlyFillsMissing`（按新语义更新）。
- **跨集群共享进程**（部署前发现）：缩容计划新增 `shared_process` 阻断，UI 提供「仅移除记录」。回归：`TestPlanScaleInBlocksSharedProcess`。

## 回滚点

- 阶段 1 每项独立可回退；`InitClusterConfigs` 变更若影响新建集群向导，回退该函数即可。
- 阶段 2 新表与新路由与旧路径解耦；回退前端即可恢复旧对话框。
- 部署回滚：`/opt/stx/backup-*` 二进制与前端备份。

## start 前检查

- [ ] 用户确认 `prd.md` / `design.md` / `implement.md`
- [ ] `implement.jsonl` / `check.jsonl` 已填写
- [ ] 确认是否拆分父子任务（见对话中的建议）
