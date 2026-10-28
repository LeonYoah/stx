# CDC 延迟告警（B4）执行计划

## 0. 准备

- [ ] 在独立的 worktree 里开发：`/data/workspace/stx-wt-cdc-delay-alert`，新分支 `feat/cdc-delay-alert`。
  - 主目录 `/data/workspace/stx-src` 正被另一个会话用于 `feat/job-detail-dag`（PR #79），有未提交的改动，不能在里面开发。
- [ ] 基线：**开工时的最新 `origin/main`**（2026-10-28 用户确认）。规划文档和归档先单独提一个 PR。
  - B4 需要 #79 里的 `jobRunMode`（判断批 / 实时以提交内容为准）。开工时如果 #79 已合并，直接用；如果还没合并，先问用户，不要在本分支重复实现，以免和 #79 冲突。
- [ ] `task.py start`（在 worktree 里执行）。
- [ ] 按 `trellis-before-dev` 读：
  - backend：`index.md`、`sync-alerts.md`、`engine-events-and-realtime.md`、`job-status-reconciler.md`，以及错误处理、日志规范；
  - frontend：`index.md`、`ui-conventions.md`；
  - `guides/cross-layer-thinking-guide.md`。

## 1. 契约（`internal/pkg/syncalert`）
- [ ] `RuleCDCDelayHigh`，加入 `TaskRules`。
- [ ] `KindRefresh`、`ResolveCDCDelay`、`Signal.ResolveReason` 及其原因常量。
- [ ] `TaskConfig` 增加 `CDCDelayThresholdSeconds`、`CDCDelayDurationMinutes`，以及默认值和范围常量；`Normalize` 补默认值，`Validate` 校验范围。
- [ ] 单测：默认值、上下界、旧请求不带这两个字段时能通过。

## 2. monitoring
- [ ] 规则目录、名称（含模板标签）加入 B4。
- [ ] `UpsertAlertPolicyRequest.Validate` 的 sync_job 分支校验 B4 条件。
- [ ] `taskConfigToPolicyRequest` 和 `syncJobPolicyToTaskConfig` 双向转换阈值和持续时间。
- [ ] `handle` 增加 `KindRefresh` 和 `refreshTask`；`resolve` 增加 `ResolveCDCDelay` 和按原因的静默关闭。
- [ ] `buildSyncAlertInstanceSummary` 显示自动关闭的原因。
- [ ] 单测：
  - 配置存取的往返；
  - 非法阈值返回 400；
  - Refresh 不计次、不通知，没有阶段时忽略；
  - Resolve 为 recovered 时发恢复通知，其余原因不发；
  - JobID 不匹配时不关闭；
  - 提醒内容使用最新的消息。

## 3. sync 判定器
- [ ] 新文件 `cdc_delay_alert.go`：
  - 状态结构；
  - 样本累计（在 `recordCDCDelay` 里接入，超过 2 分钟的旧样本不计入）；
  - `evaluateCDCDelayAlerts(ctx)`：候选作业、本轮分类、状态转移、追赶判定、消息。
- [ ] `StartEngineEventRuntime` 增加 30 秒定时器。
- [ ] `reportJobTransition`：非批作业进入终态时，发 `job_ended`。
- [ ] 配置变化（去掉 B4、停用）时，发 `rule_removed`。
- [ ] 单测（假时钟和假 Reporter、Store）：
  - 持续高于阈值达到 M 分钟时 Fire 一次；
  - 短暂尖峰不触发；
  - 追赶期（下降一成以上）不触发；追赶太慢、持平、上涨时触发；
  - 低于阈值连续两轮时 Resolve（recovered）；
  - 没有样本、读取条数增长时 Resolve；
  - 没有样本、读取条数不增长时只 Refresh，不 Resolve；
  - 通道降级时为 unknown；
  - 通道关闭、作业结束、去掉规则时静默关闭；
  - 重启后的 maybe 阶段只补发一次 Resolve；
  - 没配 B4 时不发信号；
  - 批作业和数据预览不参与；
  - 超过 2 分钟的旧样本不计入；
  - 并发安全（`-race`）。

## 4. CLI、registry、文档
- [ ] `stx sync task alert set` 增加 `--cdc-delay-threshold`、`--cdc-delay-duration`。没给这两个参数时保留当前值。
- [ ] `--rules` 的帮助文字；`registry_sync.go` 的参数说明。
- [ ] CLI 单测。
- [ ] `swag init` 后重新生成 `docs/`。核对 `route_baseline.json` 没有变化（或按需更新）。

## 5. 前端
- [ ] `lib/services/sync/types.ts`：规则联合类型，以及配置和请求的新字段。
- [ ] `task-alert-utils.ts`：
  - 规则列表；
  - 新配置默认不勾选 B4；
  - 表单字段、解析、校验（`errorCdcDelayThreshold`、`errorCdcDelayDuration`）、请求组装；
  - 单测。
- [ ] `TaskAlertForm.tsx`：
  - 勾选 B4 后展开参数和说明；
  - 显示事件通道状态提示（需要集群 ID：任务侧栏和策略中心两个入口都传）。
- [ ] i18n（zh、en）：规则名、说明、参数、错误、通道提示，以及告警中心的规则名。
- [ ] 自检 `ui-conventions.md`：渐进披露、原子信息不换行、零塌陷；侧栏宽度不变（第一期踩过的坑）。

## 6. Spec
- [ ] `sync-alerts.md`：加 B4 章节，包括信号、判定表、原因、限制、需要的测试，以及至少一组错误与正确写法的对照。
- [ ] `engine-events-and-realtime.md`：说明引擎不发空批、B4 怎样消费样本。
- [ ] `index.md` 按需更新。

## 7. 验证

```bash
go test ./internal/...                       # 已知 installer 2 个本机失败与本任务无关
go test -race ./internal/apps/sync/ ./internal/apps/monitoring/ ./internal/pkg/syncalert/
(cd frontend && pnpm exec tsc --noEmit && pnpm test && pnpm lint)
python3 scripts/check_license.py             # 以及契约报告，命令与第一期相同
```

**t1 实测**：
- 前置：t1 上现在跑的是 #79 的构建，换成本分支前**先征得用户同意**；构建单独跑，并且 `nice -n 19`。
- 准备：
  - 集群 1 是 3.0.0。如果事件通道没开，需要开启并滚动重启，这一步也要先确认。
  - 用任务 16 `mysql-cdc-schema-demo`（MySQL CDC 到 PG）。
  - 设置阈值 10 秒、持续 1 分钟；渠道选 webhook，指向本机的临时接收端。
- 场景：
  1. 持续往 MySQL 写入，同时在 PG 端间歇性锁住目标表，让延迟持续在 10 秒以上。预期：告警一次，之后有提醒。
  2. 解锁，等追上。预期：收到恢复通知。
  3. 暂停作业，期间持续写入，然后恢复。预期：追赶期间不告警。
  4. 长时间锁住目标表。预期：显示"无法确认"，不发恢复通知。
  5. 停止作业。预期：告警静默关闭。
- 收尾：恢复到测试前的 t1 状态（任务配置、锁、测试数据）。

## 8. 提交
- [ ] 中文提交，PR 指向 main。
- [ ] 先过一遍 `trellis-check`，再提交。

## 回滚点
- 契约、monitoring、sync、前端分开提交，便于单独 revert。
- 不改表；回滚后旧版本忽略 B4 条件（见 design §7）。
