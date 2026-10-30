# CDC 延迟告警（B4）执行计划

## 0. 准备

- [x] 直接在 PR #80（分支 `docs/cdc-delay-alert-plan`）上开发，先 merge 最新 `origin/main`（不 rebase、不 force-push）。
- [x] #79 已合并：批 / 实时直接用 `jobRunMode` / `jobIsBatch`。
- [x] 按 `trellis-before-dev` 读：
  - backend：`index.md`、`sync-alerts.md`、`engine-events-and-realtime.md`、`job-status-reconciler.md`，以及错误处理、日志规范；
  - frontend：`index.md`、`ui-conventions.md`；
  - `guides/cross-layer-thinking-guide.md`。

## 1. 契约（`internal/pkg/syncalert`）
- [x] `RuleCDCDelayHigh`，加入 `TaskRules`。
- [x] `KindRefresh`、`ResolveCDCDelay`、`Signal.ResolveReason` 及其原因常量。
- [x] `TaskConfig` 增加 `CDCDelayThresholdSeconds`、`CDCDelayDurationMinutes`，以及默认值和范围常量；`Normalize` 补默认值，`Validate` 校验范围。
- [x] `ResolveReasonRuleChanged`、`CDCDelayDetail`、`Signal.CDCDelay`、`FormatDelayZH`；`MaxMessageLength` 统一为 500。
- [x] 单测：默认值、上下界、旧请求不带这两个字段时能通过。

## 2. monitoring
- [x] 规则目录、名称（含模板标签）加入 B4。
- [x] `UpsertAlertPolicyRequest.Validate` 的 sync_job 分支校验 B4 条件。
- [x] `taskConfigToPolicyRequest` 和 `syncJobPolicyToTaskConfig` 双向转换阈值和持续时间。
- [x] `handle` 增加 `KindRefresh` 和 `refreshTask`；`resolve` 增加 `ResolveCDCDelay` 和按原因的静默关闭。
- [x] `buildSyncAlertInstanceSummary` 显示自动关闭的原因和"已停止提醒"；`source_ref` 带 `resolve_reason`、`reminders_paused`。
- [x] 阶段表加 `last_detail`；模板标题 / 字段 / Webhook 字段（见 design §5）；sweep 跳过停止提醒的阶段。
- [x] 保存策略去掉 B4 或停用时关闭告警中的 B4 阶段（`rule_removed`）。
- [x] 单测：
  - 配置存取的往返；
  - 非法阈值返回 400；
  - Refresh 不计次、不通知，没有阶段时忽略；
  - Resolve 为 recovered 时发恢复通知，其余原因不发；
  - JobID 不匹配时不关闭；
  - 提醒内容使用最新的消息。

## 3. sync 判定器
- [x] 新文件 `cdc_delay_alert.go`：
  - 状态结构；
  - 样本累计（在 `recordCDCDelay` 里接入，超过 2 分钟的旧样本不计入）；
  - `evaluateCDCDelayAlerts(ctx)`：候选作业、本轮分类、状态转移、追赶判定、消息。
- [x] `StartEngineEventRuntime` 增加 30 秒定时器。
- [x] `reportJobTransition`：非批作业进入终态时，发 `job_ended`。
- [x] 配置变化（去掉 B4、停用）时，发 `rule_removed`。
- [x] 单测（假时钟和假 Reporter、Store）：
  - 持续高于阈值达到 M 分钟时 Fire 一次；
  - 短暂尖峰不触发；
  - 追赶期（下降一成以上）不触发；追赶太慢、持平、上涨时触发；300→260 不触发、300→280 触发；
  - 基线用中位数，单次尖峰不抬高基线；追赶满 30 分钟触发；
  - 改阈值 / 持续时间重新计时，回落不误发恢复（`rule_changed`）；
  - 无法确认每 5 分钟 Refresh，超过 24 小时停止提醒；
  - 低于阈值连续两轮时 Resolve（recovered）；
  - 没有样本、读取条数增长时 Resolve；
  - 没有样本、读取条数不增长时只 Refresh，不 Resolve；
  - 通道降级时为 unknown；
  - 通道关闭、作业结束、去掉规则时静默关闭；
  - 重启后的 maybe 阶段计时期间 Refresh 当前值，回落只补发一次 Resolve；
  - 没配 B4 时不发信号；
  - 批作业和数据预览不参与；
  - 超过 2 分钟的旧样本不计入；
  - 并发安全（`-race`）。

## 4. CLI、registry、文档
- [x] `stx sync task alert set` 增加 `--cdc-delay-threshold`、`--cdc-delay-duration`。没给这两个参数时保留当前值。
- [x] `--rules` 的帮助文字（`registry_sync.go` 的参数说明在 TaskConfig 字段注释里，路由不变）。
- [x] CLI 单测。
- [x] `swag init` 后重新生成 `docs/`。核对 `route_baseline.json` 没有变化（或按需更新）。

## 5. 前端
- [x] `lib/services/sync/types.ts`：规则联合类型，以及配置和请求的新字段。
- [x] `task-alert-utils.ts`：
  - 规则列表；
  - 新配置默认不勾选 B4；
  - 表单字段、解析、校验（`errorCdcDelayThreshold`、`errorCdcDelayDuration`）、请求组装；
  - 单测。
- [x] `TaskAlertForm.tsx`：
  - 勾选 B4 后展开参数和说明；
  - 显示事件通道状态提示（需要集群 ID：任务侧栏和策略中心两个入口都传）。
- [x] 告警中心：自动关闭原因、"已停止提醒"。
- [x] i18n（zh、en）：规则名、说明、参数、错误、通道提示、关闭原因。
- [x] 自检 `ui-conventions.md`：渐进披露、原子信息不换行、零塌陷；侧栏宽度不变（第一期踩过的坑）。

## 6. Spec
- [x] `sync-alerts.md`：加 B4 章节，包括信号、判定表、原因、限制、需要的测试，以及至少一组错误与正确写法的对照。
- [x] `engine-events-and-realtime.md`：说明引擎不发空批、B4 怎样消费样本。
- [x] `index.md` 按需更新。

## 7. 验证

```bash
go test ./internal/...                       # 已知 installer 2 个本机失败与本任务无关
go test -race ./internal/apps/sync/ ./internal/apps/monitoring/ ./internal/pkg/syncalert/
(cd frontend && pnpm exec tsc --noEmit && pnpm test && pnpm lint)
python3 scripts/check_license.py             # 以及契约报告，命令与第一期相同
```

**集群实测**（可选，具备 SeaTunnel 2.3.12+ 测试集群和事件通道时执行；开通道、换构建等会影响共享环境的操作先征得用户同意）：
- 准备：一个 MySQL CDC 实时任务；阈值 10 秒、持续 1 分钟；渠道选 webhook，指向临时接收端。
- 场景：
  1. 持续写入源库，同时间歇性锁住目标表，让延迟持续在 10 秒以上。预期：告警一次，标题带延迟值，之后有 [提醒]。
  2. 解锁，等追上。预期：收到恢复通知。
  3. 暂停作业，期间持续写入，然后恢复。预期：追赶期间不告警（最多 30 分钟）。
  4. 长时间锁住目标表。预期：显示"无法确认"，不发恢复通知。
  5. 停止作业。预期：告警静默关闭，告警中心显示"作业已结束，自动关闭"。
- 收尾：恢复测试前的任务配置、锁和测试数据。

## 8. 提交
- [x] 中文提交，推到 PR #80（指向 main）。
- [x] 先过一遍 `trellis-check`，再提交。

## 回滚点
- 契约、monitoring、sync、前端分开提交，便于单独 revert。
- 策略不改表；阶段表只加 `last_detail` 一列。回滚后旧版本忽略 B4 条件和这一列（见 design §7）。
