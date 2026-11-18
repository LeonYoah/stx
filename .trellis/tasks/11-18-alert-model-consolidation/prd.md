# 告警模型收拢：集群级/作业级与易用性

## Goal

用户面对的告警只分两层：**集群级**、**作业级**。每条告警能在界面上看到、开关、调阈值、配通知；不会因为同一件事收到重复告警，也不会有“推荐了但永远不触发”的模板。判定机制（进程事件 / 内嵌 TSDB 规则 / sync 阶段）只是后端实现，不出现在用户分类里。

## 背景与已确认事实（代码证据）

### 分类
- 告警类别 `source_type` 现为 `platform_health` / `metric` / `job`（`internal/apps/monitoring/model.go:188`）。前两者按判定机制区分，用户凭直觉分不清：如 `split_brain_risk`、`deadlocked_threads_detected` 属“指标”，`master_unavailable` 属“平台健康”；`failed_jobs_high` 名带“作业”实为集群汇总。
- 外部 Prometheus / Alertmanager 配置段已删除（`internal/config/model.go:267`），指标全部来自内嵌 TSDB（`internal/pkg/embedts`，`docs/内置监控说明.md`）。

### 自定义 PromQL 已去掉，但字段残留
- 指标策略的表达式只由模板拼出（`buildMetricTemplatePromQL`，`metric_evaluator.go:701`），评估不读 `AlertPolicy.PromQL`。
- 残留：`alert_policy_service.go:163/277/626` 仍收、存、回；前端 `types.ts` `promql?`；`MonitoringPolicyCenter.tsx:1351` 切换启用时回传。

### 三套集群级规则来源并存
| 来源 | 用户可改 | 通知 |
|---|---|---|
| 内置 YAML 规则 `rules/seatunnel-default-rules.yml`（11 条，`go:embed`，`metric_evaluator.go:102`） | 否 | 无托管策略时回落 `NotificationRoute`（`remote_integration.go` `deliverRemoteAlertRecord`）；路由**无前端页面**（service 方法无调用方） |
| 指标模板策略（9 个模板） | 是 | 策略渠道 |
| 平台健康策略 + `AlertRule`（每集群默认规则，`service.go:1297`） | 策略可改；`AlertRule` 无前端页面（`getClusterRules`/`updateClusterRule` 无调用方），仅通过 legacy bridge 被策略同步 `Enabled`（`alert_policy_service.go` `syncAlertPolicyLegacyBridge`），窗口/阈值改不了 | 策略渠道；无策略时只进告警中心 |

### 内置规则与模板重叠对照
| 内置规则 | 条件 | 对应模板 | 模板默认 | 结论 |
|---|---|---|---|---|
| SeaTunnelProcessCpuHigh | rate(cpu[5m]) > 0.8，for 10m | cpu_usage_high | > 0.8，5m | 重复，阈值相同窗口不同 |
| SeaTunnelJVMHeapUsageHigh | heap% > 85，for 10m | memory_usage_high | > 0.8，5m | 重复，阈值不同（85% vs 80%） |
| SeaTunnelFDUsageHigh | fd% > 80，for 10m | fd_usage_high | > 0.8，5m | 重复，窗口不同 |
| SeaTunnelJobThreadPoolQueueHigh | queue > 100，for 5m | job_thread_pool_queue_backlog_high | > 100，5m | 完全相同 |
| SeaTunnelCoordinatorRejectingTasks | reject rate5m > 0，for 5m | job_thread_pool_rejection_high | increase > 0，5m | 等价 |
| SeaTunnelThreadDeadlock | deadlocked > 0，for 1m | deadlocked_threads_detected | > 0，1m | 完全相同 |
| SeaTunnelPartitionUnsafe | clusterSafe==0 或 localMemberSafe==0，for 2m | split_brain_risk | clusterSafe < 1，1m | 重叠（内置更全） |
| SeaTunnelMetricsEndpointDown | up == 0，for 2m | — | — | 仅内置 |
| SeaTunnelNodeStateDown | node_state == 0，for 2m | （语义接近 node_offline） | — | 仅内置，需与 node_offline 对齐 |
| SeaTunnelCoordinatorBacklogGrowing | 队列 > 50 且提交>完成，for 10m | — | — | 仅内置 |
| SeaTunnelGCTimeHigh | gc% > 20，for 10m | — | — | 仅内置 |
| — | — | failed_jobs_high | > 0，1m | 仅模板 |

用户开启模板策略后，同一状况会收到内置 + 策略两条告警；内置那条在界面上关不掉、改不了投递。

### 推荐模板实际不触发
- `master_unavailable`、`worker_insufficient`、`agent_offline` 为 `platform_health` 模板且 `Recommended: true`，前端会列出供选择（`MonitoringPolicyCenter.tsx` `availableStaticTemplates`）。
- 它们没有 `LegacyRuleKey`，不走进程事件派发；`buildManagedMetricPolicyRule` 只处理 `metrics_template`。仓库中未找到其判定逻辑（仅有中文名/描述映射，`metric_evaluator.go:786/827`）。用户创建后不会触发。

### 作业级（现状较好，保留）
- 作业告警配置即一条 `sync_job` 策略，每任务一条；任务设置页与策略中心共用 `TaskAlertForm`。规则 B1–B4（失败 / 长时间运行 / 自动重启耗尽 / CDC 延迟）。

## Requirements

- R1 用户可见分类只有**集群级 / 作业级**两种范围；判定机制不出现在分类里。
- R2 集群级规则是一份内置目录（D1、D4）：常规规则默认开启，复杂规则默认关闭；用户无需新建策略即可收到告警。
- R3 每条集群级规则可编辑：启用、阈值 / 持续时间、通知（沿用默认通知 / 自定义渠道与接收人 / 仅告警中心）（D3）。
- R4 每条规则一份全局配置，可加集群例外（开关、阈值、通知）（D5）。
- R5 一份默认通知（渠道、接收人、冷却、恢复通知）；未配置时告警只进告警中心并在规则页提示（D3）。
- R6 同一状况只产生一条告警：内置 YAML 与模板合并；节点离线三合一（D2）。
- R7 不存在不会触发的规则：Master 不可用、可用 Worker 不足、Agent 离线补齐判定（D2）。
- R8 升级不丢配置：已有非作业策略、`AlertRule` 改动、通知路由迁入新模型；升级后行为变化写入迁移说明（D4）。
- R9 移除残留：`promql` 字段、`NotificationRoute`、legacy bridge、旧的“从模板新建策略”入口。
- R10 作业级保持现有任务告警配置与 B1–B4 语义不变，仅在告警中心归入“作业级”。
- R11 仓库约定：接口变更同步 CLI；i18n 中英文同步；三库迁移兼容（`./scripts/test_db_compat.sh`）。

## Acceptance Criteria

- [ ] 新装环境：不做任何配置，节点进程被杀后 2 分钟内告警中心出现一条“节点离线”（集群级）。
- [ ] 默认开启 / 默认关闭的规则清单与 D4 一致（有测试断言目录默认值）。
- [ ] 同一节点 `up==0`、`node_state==0`、心跳丢失同时成立时，只有一条“节点离线”告警，内容写明原因。
- [ ] Agent 失联但引擎仍在服务（`up==1`）时，只触发“Agent 离线”，不触发“节点离线”。
- [ ] Master 不可用、可用 Worker 不足、Agent 离线均有判定实现与单测（触发、持续时间、恢复）。
- [ ] 开启 CPU 高后，同一状况只产生一条告警（原内置 + 模板重复不再出现）。
- [ ] 修改规则全局阈值后，下一个评估周期内生效；集群例外只影响该集群。
- [ ] 规则通知为“沿用默认”时发往默认通知的渠道与接收人；“自定义”时只发往规则配置；“仅告警中心”时不投递。
- [ ] 默认通知未配置时，规则页顶部显示提示与配置入口，告警照常进入告警中心。
- [ ] 升级迁移：已有指标 / 平台健康策略转为对应规则的全局配置或集群例外；`AlertRule` 中被改动的开关 / 窗口转为集群例外；路由渠道并入默认通知或规则自定义；迁移只执行一次（`SchemaFlag`），并输出迁移摘要日志。
- [ ] 告警中心、规则目录、CLI 支持按 `scope=cluster|job` 筛选；旧 `source_type` 参数仍可用。
- [ ] 请求带 `promql` 不再被接受或存储；前端不再发送；`notification-routes` 接口与 CLI 移除。
- [ ] 作业告警配置（任务设置页 / `stx sync task alert`）行为不变，相关测试通过。
- [ ] `/monitoring` 为“告警 / 规则 / 通知”三个面板，监控模块内不再引用 shadcn `Tabs` / `Sheet` / `Select` / `Badge` 与 `StatPillsBar`；所用 Arc 组件按 Arc `checklist.md` 自检通过（亮 / 暗主题、390px、键盘、减少动效）。
- [ ] 旧链接 `/monitoring/alerts|rules|history|integrations` 与旧 `tab` / `section` 参数落到对应新面板。
- [ ] Egolite 回归矩阵（design §9.6）逐项通过并记录。
- [ ] `go test ./...`、`./scripts/test_db_compat.sh`、`cd frontend && pnpm test`、`cd frontend && pnpm exec tsc --noEmit` 通过。

## Out of Scope（初定）

- 作业级规则 B1–B4 的判定语义不变。
- 不恢复自定义 PromQL。
- 通知渠道本身（webhook/邮件/企微/钉钉/飞书）的发送实现不变。

## Decisions

- D1（Q1）集群级规则采用**内置目录、默认开启**：每个集群自动拥有一份规则清单与默认阈值，用户按集群关闭或调参；内置 YAML 规则与指标模板合并为这一份目录，不再需要“从模板新建策略”。已有模板策略需迁移为对应规则的集群覆盖。

- D2（Q2）可用性规则合并为 4 条，默认全部开启：
  - 节点离线：`up==0` / `node_state==0` / 心跳丢失 任一成立，合一条告警并在内容中写明原因；持续 2 分钟，critical（取代 `SeaTunnelMetricsEndpointDown`、`SeaTunnelNodeStateDown`、`node_offline` 三条）。
  - Master 不可用：集群内无健康 master；持续 1 分钟，critical（补齐判定）。
  - 可用 Worker 不足：健康 worker 数低于阈值，默认阈值 = 部署 worker 数（少一台即告警），可调低；warning（补齐判定）。
  - Agent 离线：管理面与主机 Agent 失联；持续 2 分钟，warning（补齐判定）。
  - 规划时补充（待用户确认）：代码中“心跳丢失”即 `NodeInfo.IsOnline=false`，来源是主机 Agent 心跳（`cluster/service.go:1430`），与 Agent 离线是同一信号。为避免一台主机 Agent 掉线同时出两条告警，节点离线的原因改为 `up==0` / `node_state==0` / 进程停止或缺失；Agent 心跳超时只算 Agent 离线。

- D3（Q3，经用户补充修订）规则分两档：**常规规则内置默认开启**，**复杂规则默认关闭、由用户自行开启**。每条规则可单独编辑：启用、阈值 / 持续时间、通知渠道、告警接收人。为避免默认开启的规则无处可发，保留一份**默认通知**（渠道、接收人、冷却、恢复通知）：规则未自定义通知时沿用默认通知，自定义后以规则为准。默认通知未配置时告警照常进告警中心，规则目录顶部提示并给出配置入口。按级别分渠道暂不做。
- D4（Q4）默认开关划分：
  - 默认开启（13 条集群级 + 作业级 B1–B4）：节点离线（合一）、Master 不可用、可用 Worker 不足、Agent 离线、进程崩溃、进程重启失败、达到重启上限、作业状态获取失败、分区不安全、线程死锁、任务被拒绝、JVM 堆内存高（> 85%，10 分钟）、FD 使用率高（> 80%，10 分钟）。堆内存与 FD 沿用现内置阈值，保证升级后行为不变。
  - 默认关闭（8 条）：CPU 高、GC 耗时高、作业线程池队列积压、积压持续增长、失败作业数过高、作业事件通道降级、集群重启请求通知、节点停止请求通知。
  - 升级行为变化（需写入迁移说明）：现内置规则中 CPU 高、GC 耗时高、线程池队列积压、积压持续增长 4 条当前默认生效，改为默认关闭后升级用户将不再收到，除非手动开启。

- D5（Q5）规则配置为**全局一份 + 集群例外**：全局配置（启用、阈值 / 持续时间、通知渠道、接收人）对所有集群生效；可为个别集群单独设置开关、阈值、通知。规则列表显示例外数量并可展开查看。
- D6 持续时间语义：沿用 Prometheus 规则引擎 `for`（每 15s 判定，连续满足满持续时间才触发，中途一次不满足即清零重计）。重复规则合并时阈值与持续时间取现内置规则值（CPU / 堆 / FD 为 10 分钟），避免升级后变得更敏感。

- D8（用户补充）本次允许**破坏原有布局与交互**，页面可省略与合并；界面**尽可能使用全新的 Arc UI 组件**（`frontend/components/arc/`，按 `.claude/skills/arc/SKILL.md` 选型与安装），设计思路参照 PR #163（`.trellis/tasks/11-18-unified-admin-console-lists/`：List Shell 分层、server-driven 列表、Egolite 回归矩阵）。注意：PR #163 的“扩展已有组件、最小变更、shadcn 为基线”在本任务的监控模块内被 D8 覆盖，改为 Arc 优先；Arc 没有的原语再复用现有组件。“先行版”形态：**完整功能、直接替换**，`/monitoring` 换成新界面，旧页面与旧接口一并删除，不保留并存期。界面合并：一个页面三个面板“告警 / 规则 / 通知”；策略中心省略；全局投递记录合并进告警详情与渠道详情（见 design §9.1）。
- D7（Q6）不拆子任务，在本任务内一次完成；按 `implement.md` 的阶段顺序推进，每阶段跑通测试后再进入下一阶段。

## Open Questions

- 无阻塞规划的问题。实现中如遇无法从代码判断的取舍，再单独确认。
