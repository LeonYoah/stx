# E2 设计：每日 / 每月 / 指定时间读写量

分支 `feat/dashboard-e2-volume`，从 main 拉出（P1–P4 已全部合并）。依赖 `promquery`（内嵌 tsdb 已具备 Range/Instant）与 `embedts`（本任务补 `OldestSampleTime`）。

## 1. embedts：最早样本时间

`OldestSampleTime() (time.Time, bool)`：取 `db.Blocks()` 全部块 `Meta().MinTime` 与 `db.Head().MinTime()` 的最小值；未就绪或无样本返回 `false`。PRD 已声明「内嵌任务未提供则本任务补上」。

## 2. 接口

`GET /api/v1/dashboard/home/volume`（CLI `stx dashboard home volume`），登录必填。

参数：`cluster_id`、`range`、`start`、`end`（与 jobs/clusters 大盘同套 `promquery.ParseWindow` + `ResolveWindow` + `alignPresetNow`）、
`granularity=day|month`（缺省则不出 buckets，只回 totals——指标卡场景省掉桶查询）、`split=none|cluster|task`（缺省 none）。

```jsonc
{
  "source": "prometheus" | "none", "reason": "", "range": "7d", "start_ms": 0, "end_ms": 0,
  "granularity": "day", "split": "none",
  "oldest_sample_ms": 0,                      // embedts.OldestSampleTime；无样本 / 不可用时 0
  "totals": {                                 // 始终返回（指标卡用）；source=none 时为 null
    "window": {"read_rows":0,"write_rows":0,"read_bytes":0,"write_bytes":0},
    "today": {…}, "month": {…},
    "today_start_ms": 0, "month_start_ms": 0   // 显示时区当日 / 当月起点，前端据此出「自 M/D 起」
  },
  "buckets": [{"start_ms":0,"end_ms":0,"label":"2026-11-16","partial":false}],  // granularity 缺省则不下发
  "series": [                                  // 与 buckets 按下标对齐；split=none 时每指标一条 group_id="0"
    {"metric":"write_rows","group_id":"3","totals":[b0,b1,…]}
  ],
  "groups": {"3":"prod-cluster","10":"订单同步"},  // split≠none 时 id→名称；"0" 固定为全部（不发）
  "partial_errors": []
}
```

## 3. 桶与查询（对 PRD 的一点实现修正）

- **统一「每桶一次即时查询」**：桶值 = `sum by (splitKey) (increase(<metric>{sel}[<span>s])) @ bucketEnd`，
  span = bucketEnd − bucketStart。日桶、月桶、totals（window / today / month 视为三个临时桶）走同一条代码路径。
  PRD 原文写「按日用 step=1d 区间查询」；改为逐桶即时查询的理由：显示时区跨 DST 时日长 ≠ 86400s，固定 step 会漂出零点边界，
  验收项「跨日 / 跨月边界按显示时区对齐」要求严格对齐；且与月粒度共用执行路径、缓存键按天界复用（多用户/多窗口共享同一缓存）。
- 日桶：显示时区零点切分窗口，`timex.DisplayLocation()`；首末被窗口裁掉的日记 `partial=true`。
- 月桶：窗口内每个自然月一桶（含被裁的首末月，partial 同理）；窗口 ≤31 天 ⇒ 最多 3 桶。
- 指标：read_rows=`stx_job_source_received_rows_total`、write_rows=`stx_job_sink_write_rows_total`、
  read_bytes=`stx_job_source_received_bytes_total`、write_bytes=`stx_job_sink_write_bytes_total`。
- split：`sum by (cluster_id|task_id)`；名称解析 task 走 `sync.ResolveTaskNames`，cluster 走 `ClusterNodeSource.List` 建映射；
  解析不到的组显示 `#id`。桶内 4 指标 × N 桶并发即时查询（promTask 模式），走 `dashboardPromCache`。
- `increase` 天然处理计数器重置；终值补写已由内嵌 P2 的 deleteJob 快照保证（验收时核对）。
- `source=none`（未开启 / 未就绪）：仍回 `oldest_sample_ms:0`、totals=null、buckets 空，前端走 `TrendEmptyState`。

## 4. 前端

- `HomeService.getVolume` + 类型；数组 / 对象兜底。
- **指标卡**：`DashboardKpiRow` 加第 7 张「数据量」卡（主数字 = 当前时间窗写入行数，副标题「今日 X · 本月 Y」，本月起点早于
  最早样本时注明「自 M/D 起」），点击跳作业页签。卡片数据由 `HomeDashboard` 轮询 `getVolume`（无 granularity）喂入；
  网格改 `min-[480px]:2 lg:4 xl:7`，390px 下不溢出。
- **作业页签「数据量」面板**（吞吐之后）：SegmentedControl ×3——指标（写行/读行/写字节/读字节）、粒度（日/月）、
  拆分（全部/按集群/按任务）；beUI `BarChart` 竖向，split≠none 时 `layout='stacked'` 多 series，颜色循环 `--series-N`；
  桶 label 日=`M-D`、月=`YYYY-M`；窗口起点早于最早样本时面板提示「数据自 YYYY-M-D 起」；`source=none` 走 `TrendEmptyState`。
- 自定义时间窗固定不轮询（与现有一致）；预设窗 30s 轮询。

## 5. 其余

- 路由挂 `/dashboard/home` 组（LoginRequired）；operation 注册 `dashboard.home.volume`（cluster_id/range/start/end/granularity/split）；
  swagger 与路由基线随检查更新。
- i18n：`dashboard.home.kpi.volume*`、`dashboard.home.jobsTab.volume.*`、`dashboard.home.blocks.volume`，zh/en 双份。
- 权限：聚合全局口径（同大盘锁定决策 #6），split 组名（集群 / 任务名）对所有登录用户可见，逐作业数据不在本接口范围。

## 6. 测试

- Go：桶切分（零点 / 月末 / DST 时区如 America/New_York、首末 partial）、PromQL 拼装（splitKey / span）、
  组名解析与 fallback、source=none、oldest_sample 透传、partial_errors。
- embedts：写样本后 OldestSampleTime 返回最早时间。
- 前端：JobsTab 面板三个开关改请求参数、KpiRow 新卡渲染与「自 M/D 起」、无 Prometheus 空态。
