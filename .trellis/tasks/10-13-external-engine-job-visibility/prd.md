# 外部 SeaTunnel 作业可见性与治理 PRD

## 1. 目标与用户价值

**问题**：运维与开发同学确认——经 STX 工作台 / CLI / API 提交的作业可以完整管理；但有人直接调用 SeaTunnel Engine REST（`submit-job`、官方脚本、Airflow 等外部调度）提交到同一集群的作业，**不会进入 STX 的 `JobInstance` 台账**，导致：

- 工作台「运行作业」列表不完整，排障文档里「运行作业列表排查悬挂任务」对外部作业无效；
- 无法通过 `stx sync job cancel` / 统一 Execution 审计治理外部作业；
- Slot 被占、Grafana `job_count` 升高时，难以在 STX 内对应到「是谁提交的、能否安全取消」。

**目标**：在 **不强制所有提交必须走 STX** 的前提下，让 STX 对「已纳管集群上 Engine 真实在跑/近期结束的作业」具备 **可观测 + 有限治理** 能力，并与现有 STX 登记作业 **可区分、不混淆**。

**用户价值**：

- 集群管理员：一眼看到 **STX 管理 vs 外部提交** 的作业占用，减少 Slot 挤兑误判；
- 值班运维：对外部悬挂作业有 **受控取消**（若产品允许），不必 SSH 或手工调 Engine API；
- 平台一致性：监控指标（Engine 级）与控制面列表语义对齐，降低「指标有作业、列表没有」的认知 gap。

---

## 2. 已确认事实（来自代码库）

| 事实 | 依据 |
|------|------|
| STX 作业生命周期绑定 DB 中的 `JobInstance`，列表 API `GET /api/v1/sync/jobs` 查的是平台实例，不是 Engine 全量 | `internal/apps/sync/repository.go`、`handler.go` |
| 提交集群作业时经 `EngineClient.Submit` 写 `EngineJobID`，后续 `GetJobInfo` / `StopJob` / 日志与 Checkpoint 均 **按已知 jobId** | `internal/apps/sync/engine_client.go` |
| `EngineClient` **无**「列出集群全部作业」接口封装 | `EngineClient` interface |
| 集群删除前的「运行中任务」检查看的是 **集群部署状态**（deploying/running），**不是** Engine 作业数 | `internal/apps/cluster/service.go` `Delete` |
| 可观测性侧 Grafana 使用 Prometheus `job_count{...}`，反映 **Engine HTTP 暴露的作业统计**，含外部作业 | `install/observability/grafana_config/dashboards/seatunnel-overview-*.json` |
| CLI 作业能力（`stx sync job *`）均基于 **平台 job id**（`JobInstance.ID`） | `internal/operation/registry_sync.go`、`internal/cmd/sync.go` |
| 产品 PRD 已明确「绕过 STX 直接访问 SeaTunnel 节点」不在 Agent/CLI 安全模型内；本需求是 **控制面只读/写 Engine REST（经 STX 代理）**，不是 SSH | `09-13-stx-cli-agent-entry` 暂不纳入 |

---

## 3. 范围边界（草案）

### 3.1 建议纳入（待 MVP 决策细化）

- **Engine 作业镜像视图**（按集群）：展示 Engine 侧作业 id、名称、状态、创建/结束时间等（字段以 Engine API 为准）。
- **来源标注**：与 STX `JobInstance` 按 `cluster_id + engine_job_id`（及必要时 job name / platform job id）关联；无法关联的标为 **外部 / 未登记**。
- **只读详情**：对外部作业可查看 Engine 返回的 status、metrics 摘要、错误信息（复用现有 `GetJobInfo` 能力，**不**伪造 Task 版本链路）。
- **可选治理**：对外部作业提供 **Stop/Cancel**（R2，Execution + 审计），调用现有 `StopJob`；**不**默认提供 Savepoint/Recover/与工作台 Task 绑定的 Checkpoint UI，除非二期明确。
- **CLI**：补充只读查询（及若确认的写操作）并登记 `operation` registry。
- **版本适配**：2.x legacy URL 与 3.x REST 路径差异沿用 `EngineEndpoint.APIMode` 现有逻辑。

### 3.2 建议暂不纳入（首版）

- 将外部作业 **导入** 为 STX Task / 自动生成 `JobInstance` 全链路（审计 retroactive、配置回溯）。
- 在 Engine 或网络层 **拦截** 非 STX 提交（强管控多租户场景另立项目）。
- 周期性 **持久化同步** 全量 Engine 作业历史到 STX DB（首版可用实时查询 + 短 TTL 缓存）。
- 诊断包自动 attach 外部作业配置（无 Task 则无 `submitted_content`）。

---

## 4. 非功能与规范约束

- 改动前遵循 `.trellis/spec/backend/index.md`、`.trellis/spec/frontend/index.md`、`cross-layer-thinking-guide.md`。
- 新增 HTTP 路由必须登记 `internal/operation/registry*.go`，并补 CLI（AGENTS.md CLI 约定）。
- 写操作（取消外部作业）走 Execution 安全契约（R2、`--confirm`、幂等键）。
- 注释：新增/修改代码 **中英双语**，中文在前。
- Engine 列表/详情 API 需在 `design.md` 阶段 **实测** 各 SeaTunnel 版本（2.3.x / 3.x）路径与响应结构。

---

## 5. 开放决策（规划阶段逐项收敛）

| # | 决策 | 状态 |
|---|------|------|
| D1 | **MVP 能力边界**：仅只读列表+详情 vs 含 R2 取消 | **待确认** |
| D2 | **入口位置**：集群详情页「Engine 作业」vs 同步工作台并列 Tab vs 仅 CLI | 待确认 |
| D3 | **权限模型**：是否仅管理员 / 集群维护者可取消外部作业 | 待确认 |
| D4 | **刷新策略**：纯按需拉取 vs 后台轮询（间隔、对 Master 压力） | 待确认 |
| D5 | **与 STX 作业 UI 关系**：合并一张表（带来源列）vs 分开展示 | 待确认（[方案 C 概念 Canvas](https://cursor.com/canvas/store-54b05534-483d-47b3-9a10-0a4f746873dd/3afba018-87bb-49b9-acfa-bf4fbb8a007a)） |

---

## 6. 验收标准（随 D1–D5 调整后定稿）

### 6.1 只读 MVP（若 D1 选只读）

- [ ] 对已纳管且 Running 的集群，API/CLI 可返回 Engine 侧 **当前运行中** 作业列表（含 STX 已登记与未登记条目，来源字段可区分）。
- [ ] 未登记作业在 UI/CLI 中 **不会** 出现在现有 `sync/jobs` 平台实例语义中（避免 id 混淆）；使用独立资源或明确 `source=external`。
- [ ] STX 已登记作业与 Engine 列表 **去重展示** 或 **双向链接**（平台 job id ↔ engine job id）至少一种行为在 PRD/设计中文档化且实现一致。
- [ ] Engine 不可用 / 版本不支持列表 API 时，返回稳定错误码与文案，不拖垮集群列表页。
- [ ] 操作登记 + CLI help 覆盖新增只读命令；无未登记路由缺口（contract-report）。

### 6.2 治理扩展（若 D1 含取消）

- [ ] 对外部作业 cancel 走 Execution（R2），审计记录含 cluster_id、engine_job_id、操作者。
- [ ] 取消成功后 Engine 状态可在合理超时内反映为终态；失败原因可观测。
- [ ] 文档说明：外部作业 **无** Task 级 Savepoint/Recover，除非二期。

---

## 7. 风险与依赖

- **Engine API 差异**：列表接口在 2.3 legacy 与 3.x 可能不同，需 adapter 层（与现有 `engine_client` 一致）。
- **身份关联歧义**：仅 job name 匹配可能碰撞；优先 `engine_job_id` + `platform_job_id`（STX submit 时写入 Engine 的 job id）。
- **运维预期**：只读 MVP 仍无法回答「谁在外部提交的」——除非 Engine 暴露 submitter 或后续接审计旁路。

---

## 8. 相关任务与参考

- `09-20-cli-workbench-sync`：STX 登记作业 CLI 体系。
- `09-13-stx-cli-agent-entry`：Execution、操作登记、风险分级。
- 对话上下文：用户确认「非 STX 提交的作业目前基本无法在同一套平台能力里管理」。

---

## 9. 规划记录

- 2026-10-13：创建任务 `10-13-external-engine-job-visibility`，种子 PRD；待 brainstorm 收敛 D1–D5。
