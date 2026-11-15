# P1 作业列表与顶部指标卡（仅 DB）

父任务：`11-14-home-monitoring-dashboard`（需求总表、设计见父任务 `prd.md` / `design.md`，原型见 `prototype/`）。

## Goal

首页换成新骨架（顶部范围 / 时间窗 / 指标卡 / 需要处理条 + 页签框架），先交付「作业列表」页签。
本期不依赖 Prometheus 也能完整使用；接入时额外显示迷你曲线。

## Requirements

- 首页骨架：`WorkspaceHeader` + 范围选择（全部集群 / 单集群）+ 时间窗分段控件 + 刷新状态；状态写入 URL（`?tab=&cluster=&range=`）。
- 指标卡 6 张（父 PRD R1）。无 Prometheus 时写入速率卡显示当前合计值（对账数据），不画曲线。
- 「需要处理」条（父 PRD R2），合并原「告警事件」「待办提醒」。
- 页签：集群 / 作业 / 作业列表。本期「集群」页签暂放原 Grafana 折叠面板（P2 替换），「作业」页签显示「即将提供」占位（P3 替换）。
- 移除原首页「最近活动」「资源统计」「告警事件」「待办提醒」四块（后两块由「需要处理」条承接）。
- 作业列表页签（父 PRD R5）：
  - 筛选：状态（`StatPillsBar`，计数随范围变化）、模式（流 / 批）、集群、关键字（作业名 / 任务名 / 引擎作业 ID / 平台作业 ID）。
  - 列：作业（名称 + 任务 # + 引擎 ID）、集群、模式、状态、写入速率（+ 近 1 小时 sparkline）、累计写入、CDC 延迟、开始时间 / 时长、操作。
    （实现调整：Checkpoint 列不放进列表，最新值只在可观测性开启时由后台采集写入 Prometheus，列表不为它逐行请求引擎；预览抽屉与作业详情里可看。集群名由前端用集群列表映射。）
  - 排序：默认「需要关注优先」（失败 > 运行中 > 等待 > 其余，同级按开始时间倒序）；速率 / 累计 / CDC 延迟 / 开始时间可点表头排序（服务端排序）。
  - 多选批量停止 / 恢复：逐个调用现有 cancel / recover，使用现有确认交互，结果逐条反馈。
  - 行预览抽屉：状态、源 → 目标、当前值、近 1 小时吞吐（复用作业趋势接口）、失败原因；「打开作业详情」跳工作台深链。
  - 服务端分页（每页 50），列表用现有 `StudioDataTable`。
- 后端：`GET /sync/jobs` 扩展、`GET /sync/jobs/sparklines`、`GET /dashboard/home/summary`，各补 CLI、swagger、路由基线。
- 权限：非管理员列表可见范围与 `getJobInstanceForActor` 一致（自己创建的 + 有权查看的任务下的）。

## Acceptance Criteria

- [ ] `observability.enabled=false` 时首页与作业列表完整可用，sparkline 列不显示，无报错。
- [ ] 开启后 sparkline 由一次批量请求取得（不按行请求）。
- [ ] 状态 / 模式 / 集群 / 关键字筛选与排序在服务端完成，翻页后条件保持；URL 可分享。
- [ ] 旧 `GET /sync/jobs` 调用（工作台、CLI、引导）行为不变（有回归测试）。
- [ ] 非管理员看不到他人作业；sparklines 越权 id 被丢弃（有测试）。
- [ ] `stx sync job list --status failed --cluster_id 1`、`stx sync job sparklines --ids 1,2`、`stx dashboard home summary` 可用。
- [ ] zh / en 文案齐全；390px 下表格横向滚动，不撑破页面。
- [ ] `go test ./...`、`pnpm test`、`pnpm exec tsc --noEmit` 通过。
