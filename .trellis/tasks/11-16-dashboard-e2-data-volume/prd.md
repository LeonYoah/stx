# 增强：每日 / 每月 / 指定时间读写量

父任务：`11-14-home-monitoring-dashboard`。排期：P1–P4 验收完成之后。

## Locked Decisions（2026-11-16 用户确认）

- **只用 Prometheus**，不在 STX 数据库建汇总表（维持「不保存时序」的既有决定）。
- 回看范围受 Prometheus 保留期限制：STX 自带 Prometheus 当前未设 `--storage.tsdb.retention.time`，默认 15 天，按月看不全；
  需在设计中给出保留期建议 / 配置项（例如安装器渲染 retention，或在界面提示当前保留期）。未接入 Prometheus 时无数据（统一空态）。

## Requirements

- 指标卡新增「读写量」：主数字为当前时间窗（含自定义区间）的写入行数，副标题「今日 X · 本月 Y」（本月超出保留期时注明「仅含近 N 天」）。
- 作业页签新增「数据量」柱状图：按日 / 按月切换；读 / 写、行 / 字节切换；可按集群或任务拆分。
- 后端用 `increase(stx_job_*_total[...])`：按日用显示时区零点对齐的 `step=1d` 区间查询；按月对每个自然月做一次即时查询（月长不等）。
  计数器重置（作业重启）由 `increase` 处理；作业终态后序列删除前最后一次增量是否丢失需验证。
- 接口与 CLI（如 `stx dashboard home volume --granularity day|month`）。

## Acceptance Criteria

- [ ] 指定区间写入量与该区间内各作业「累计写入」差值之和一致（抽样核对，允许 Prometheus 外推误差）。
- [ ] 跨日 / 跨月边界按显示时区对齐。
- [ ] 超出保留期时明确提示，不给出误导性的 0。
