# 内嵌 tsdb 默认按容量保留 + 一年时间窗 · 执行计划

分支 `feat/tsdb-size-retention-year-window`（基于 main `7ee6624b`）。每步一个 commit，提交信息用中文。
每步开始前按 AGENTS.md 查阅对应的 spec（后端：`embedded-tsdb-capacity.md`、`engine-events-and-realtime.md`；
前端：`ui-conventions.md`）。新增或修改的注释写中英双语，中文在前。

**注意内存**：8G 机器上不要跑 `next dev` / `next build`，也不要跑一年数据的基准测试。基准测试在 CI 或大内存机器上手动运行。

## Step 1 · 存储默认值与块合并（design §1）

- [ ] `config`：`retention` 默认留空；增加 `MaxSizeSet`；不再填充 30GiB。
- [ ] `embedts`：
  - `DefaultRetention = 0`，修正 `withDefaults`；
  - `autoMaxBytes`（statfs、20%、30GiB 上限、1GiB 下限、失败时退回）；
  - `MaxBlockDuration` 计算，以及只给测试用的覆盖项；
  - 两项都不限时记 WARN；更新启动日志。
- [ ] 状态展示：`unlimited` 和 `max_size_source`；前端显示"不限"。
- [ ] 单测：
  - 配置组合：留空、`0`、`90d`、显式 `max_size`、显式 `0`；
  - `autoMaxBytes`：注入 statfs 函数，覆盖 40GB / 1TB / 2GB / 失败四种情况；
  - `MaxBlockDuration`：无保留期、`retention=30d`、`retention=1h`；
  - 短块测试：设置 `MinBlockDuration=1m`、`MaxBlockDuration=1h`，写入数据后验证块被合并，且块时长不超过 1h。
- 验证：`go test ./internal/config/... ./internal/pkg/embedts/... ./internal/apps/monitoring/...`

## Step 2 · 时间窗上限、预设与步长（design §2.1）

- [ ] `promquery`：`MaxCustomSpan = 366d`；预设 `30d/90d/1y`；追加步长候选。
- [ ] 同步更新 `promquery` 和 `job_timeseries` 的单测（`autoTimeseriesStep` 的期望值）。
- [ ] CLI：检查 `--range` 的取值说明、`Example`、`OutputExample`，与 `registry.go` 一致。
- 验证：`go test ./internal/pkg/promquery/... ./internal/apps/sync/... ./internal/apps/dashboard/... ./internal/operation/...`

## Step 3 · 长窗口趋势图取样（design §3）

- [ ] `promquery.RangeWindow`。
- [ ] 逐个文件把范围查询改为按 step 生成速率窗口：`cluster.go`、`cluster_detail.go`、`hosts.go`、`jobs.go`、
      `job_timeseries.go`。仪表类面板按均值 / 峰值语义分类，在 PR 描述里列出分类表。
- [ ] 窗口跨度超过 31d 时，超时放宽为 `query_timeout × 4`。
- [ ] 单测：
  - step=30s 时表达式不变；
  - step=2d 时计数器为 `rate(x[2d])`，仪表为 `avg_over_time` / `max_over_time`。
- [ ] `TestCapacity_YearWindow`（`STX_TSDB_YEAR_BENCH=1` 时运行）：在大内存机器上跑 200 / 1000 条序列两档，
      把结果写进 `embedded-tsdb-capacity.md`。
- **门槛**：200 条序列时一年窗口要在 32s 内返回。达不到就停下来，在 PR 里报告结果，等用户决定。

## Step 4 · 读写量按周 / 按月分桶（design §4）

- [ ] 增加 `week` 粒度：floor / next / label 三个函数，以及新的自动分档。
- [ ] 查询计划：完整日用一条范围查询，首尾两段各用一条即时查询，后端聚合成桶和窗口合计；DST 时退回逐日查询。
- [ ] 逐日结果的 LRU 缓存（"今天"不缓存）。
- [ ] `no_data` / `partial` 标记。
- [ ] 单测：
  - 周的边界（跨月、跨年、周一零点）；
  - 自动分档的五个区间；
  - 查询计数断言（一年按月不拆分时 ≤ 指标数 × 3；缓存命中后只剩首尾两段）；
  - DST 退回路径（用 `America/New_York` 构造）；
  - 聚合结果与旧实现在 31 天窗口下一致。
- [ ] CLI 与 swagger 的 `granularity` 说明加上 `week`。
- 验证：`go test ./internal/apps/dashboard/... ./internal/operation/...`

## Step 5 · 前端（design §2.2、§4.1）

- [ ] 合并两处 31 天常量为一个 366 天的公共常量；截断时显示可见提示（i18n 中英文）。
- [ ] 首页和作业详情的分段控件加上 `30d / 90d / 1y`。
- [ ] 柱状图支持 `week` 标签和 `no_data` 占位；趋势图在 `oldest_sample_ms` 之前不画 0。
- [ ] 前端单测：
  - 截断与提示；
  - 周标签格式化；
  - `no_data` 渲染。
- 验证：`cd frontend && pnpm exec tsc --noEmit && pnpm test`

## Step 6 · 文档与规范（design §5）

- [ ] 按 design §5 的表逐项更新。swagger 用 `swag` 重新生成。
- [ ] 全仓库搜索"31 天 / 31 days / 31d / 30GiB"，确认没有遗漏的过时表述（容量规范里作为历史背景引用的除外）。

## 审查门槛

- Step 1：默认配置启动日志里能看到 `retention=不限`，`max_bytes` 来源为 auto；开发目录下的 tsdb 正常打开。
- Step 3：拿到基准测试数据后再进入 Step 4。数据不达标时由用户决定是否改做降采样。
- Step 4：一年窗口按月、90 天按周，在 PR 里贴出接口返回的截取片段（桶数、`no_data`、查询数日志）。
- 合入前：PR 描述贴出首页一年窗口与 90 天窗口的截图（在 CI 或其他机器上截取，不在 8G 机器上跑前端）。
