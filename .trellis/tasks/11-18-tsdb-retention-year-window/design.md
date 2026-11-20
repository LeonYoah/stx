# 内嵌 tsdb 默认按容量保留 + 一年时间窗 · 技术设计

## §1 存储默认值（R1 / R2）

### 1.1 配置层（`internal/config`）

- `observability.storage.retention` 默认改为空串。`config.example.yaml` 写成注释示例：
  `# retention: "90d"  # 可选：按时间删除的上限，留空表示只按容量`。
- `max_size` 的"未配置"状态要一直传到运行时，因为默认值依赖数据目录所在的磁盘，只能在运行时计算：
  - `StorageConfig` 增加 `MaxSizeSet bool`（`mapstructure:"-"`），由 `setDefaults` 根据 `loader.IsSet(...)` 写入；
  - 不再在配置层把 `MaxSize` 填成 30GiB；
  - 校验保持 `max_size >= 0`。

### 1.2 运行时（`internal/pkg/embedts`）

- `DefaultRetention` 改为 `0`（不按时间删除）。`withDefaults` 只把负数归零，**不再把 0 改成 31 天**。
- `OptionsFromObservability` 计算 `MaxBytes`：
  - `MaxSizeSet=true` 时直接用配置值（0 表示不限）；
  - 否则取 `autoMaxBytes(dir) = min(30GiB, 磁盘总容量 × 20%)`：
    - 先 `MkdirAll(dir)`，再用 `unix.Statfs` 计算 `Blocks × Bsize`；
    - 非 Linux 平台或 statfs 失败时退回 30GiB，并记 WARN；
    - 计算结果不小于 1GiB，防止小盘上算出过小的值导致数据刚落盘就被删。
  - 新增 `Options.MaxBytesSource`（`"config"` / `"auto:20%"` / `"auto:cap"` / `"fallback"`），只用于日志和状态展示。
- `MaxBlockDuration`（新增 `Options.MaxBlockDuration`，零值表示自动）：
  ```
  maxBlock = 7d
  if retention > 0 && retention/10 < maxBlock { maxBlock = retention/10 }
  if maxBlock < 2h { maxBlock = 2h }
  ```
  - tsdb 的合并档位是 `ExponentialBlockRanges(2h, 10, 3)` = 2h、6h、18h、54h、162h、486h…，
    超过 `maxBlock` 的档位会被截掉。所以实际最大块是 **162h（6.75 天）**，一年约 55 个块。
  - 基准测试仍用 `MinBlockDuration` 缩短块时长；新增的 `MaxBlockDuration` 覆盖项也只给测试用，
    让短块测试能观察到合并。
- `retention == 0 && MaxBytes == 0` 时允许启动，但记 WARN："按时间和按容量都不限，数据会无限增长"。
- 启动日志（`embedts.go:255`）改为输出：`retention=不限|<d>`、`max_bytes=<n>(<source>)`、`max_block=<d>`。

### 1.3 状态展示

- `monitoring/service.go:445` 的组件信息里，`retention` 为空时写 `"unlimited"`，并补充 `max_size_source`。
- 前端监控状态处把 `unlimited` 显示为"不限"（走 i18n）。

## §2 时间窗（R3）

### 2.1 后端（`internal/pkg/promquery/window.go`）

- `MaxCustomSpan = 366 * 24h`。错误信息改为"最长 366 天"。
- 预设时间窗：

  | range | 时长 | 默认 step | 点数 |
  |---|---|---|---|
  | 30d | 30d | 14400（4h） | 180 |
  | 90d | 90d | 43200（12h） | 180 |
  | 1y | 365d | 172800（2d） | ~183 |

  错误提示里的取值列表同步更新（`15m|1h|6h|24h|7d|30d|90d|1y`）。
- `stepCandidates` 追加 21600、43200、86400、172800。`AutoStep` 逻辑不变，一年窗口会选到 2d。

### 2.2 前端

- `MAX_CUSTOM_WINDOW_MS` / `MAX_TREND_WINDOW_MS` 改为 366 天，**两处共用一个常量**（放在 `lib/` 下）。
- 截断仍保留靠近结束的一段。截断发生时，在控件旁边显示一行提示"已截取为最近 1 年"（i18n），
  下次选择时清除。
- 分段控件加上 `30d / 90d / 1y`：首页 `HOME_RANGES`，以及作业详情趋势的预设。
- 窗口早于 `oldest_sample_ms` 的部分：
  - 柱状图对应的桶画成"无数据"占位（浅灰斜纹或空柱，按 `ui-conventions.md` 选），tooltip 写"该时间段暂无数据"；
  - 趋势图不补 0。
  - 后端在 volume 的 bucket 上加 `no_data: true`；趋势图由前端用 `oldest_sample_ms` 判断。
    如果趋势接口还没有这个字段，就一并加上。

## §3 长窗口趋势图取样（R4）

### 3.1 问题

步长为 2d 时，`rate(x[5m])` 只计算每个点之前 5 分钟的速率。一年 183 个点只覆盖了约 15 小时的数据，
峰值和低谷都会漏掉。仪表类指标同理，取到的只是某一刻的值。

### 3.2 规则

新增 `promquery.RangeWindow(step int, base time.Duration) string`，返回 PromQL 时长字面量 `max(base, step)`。

| 面板类型 | 步长 ≤ base | 步长 > base |
|---|---|---|
| 计数器速率 | `rate(x[base])`（不变） | `rate(x[<step>])`：整个区间的平均速率 |
| 仪表·均值语义（CPU / 内存 / 堆使用率、忙碌度、背压） | 原表达式 | `avg_over_time(x[<step>])` |
| 仪表·峰值语义（CDC 延迟、队列积压） | 原表达式 | `max_over_time(x[<step>])` |

- 涉及文件：`dashboard/cluster.go`、`cluster_detail.go`、`hosts.go`、`jobs.go`，`sync/job_timeseries.go`。
  所有固定写死 `[5m]` / `[1m]` 的范围查询都改为按 step 生成。
- **即时查询**（`instant: true`）不变。
- 每个面板属于哪一类，在 PR 描述里列一张表，审查时逐项确认。

### 3.3 成本

- 样本上限：PromQL 引擎对"函数作用于范围选择器"是逐条序列计算的，处理完一条就释放
  （`promql/engine.go`，per-series 循环里 `currentSamples -= len(floats)`）。
  峰值样本数约等于"单条序列一个窗口的样本数 + 输出点数"。一年 2d 步长时，单条约 1.15 万 + 输出若干，
  **不会**触发 `max_samples=1000 万`。
- 真正的成本是 CPU：一年窗口要解码所有匹配序列的全部原始样本，每条序列约 210 万个点。
  对 `sum(rate(stx_job_*_total[2d]))` 这类跨全部作业的面板来说，200 条作业序列约 4.2 亿个样本。
- 处理：
  1. 窗口跨度超过 31d 的查询，超时时间放宽为 `query_timeout × 4`（默认 32s）。只作用于这类长窗口，
     短窗口的 8s 不变。
  2. 新增手动基准测试 `TestCapacity_YearWindow`（`STX_TSDB_YEAR_BENCH=1` 时才运行）：
     - 合成 200 和 1000 条作业计数器序列的一年数据（15s 间隔）；
     - 测量一年窗口下 jobs 面板和读写量接口的耗时与峰值内存；
     - 结果写入 `embedded-tsdb-capacity.md`。
  3. **门槛**：200 条序列时一年窗口必须在 32s 内返回。超出时这次不放开到一年，
     先把降采样 / 预聚合单独立项（见 PRD Out of Scope）。

## §4 读写量按周 / 按月分桶（R5）

### 4.1 粒度

- `volumeGranularities` 增加 `week`。
- `volumeFloor` / `volumeNext` / `volumeLabel` 支持 `week`：
  - 按显示时区，取当周周一 00:00；
  - 向后推进 7 个自然日（用日历运算，跨夏令时也正确）；
  - 标签用周一的日期 `YYYY-MM-DD`，由前端渲染为 `11/10–11/16`。
- `volumeGranularityFor` 改为：

  | 跨度 | 粒度 | 柱数 |
  |---|---|---|
  | ≤ 6h | quarter_hour | ≤ 24 |
  | ≤ 72h | hour | ≤ 72 |
  | ≤ 31d | day | ≤ 31 |
  | ≤ 182d | week | ≤ 27 |
  | > 182d | month | ≤ 13 |

- 前端 `JobsTab.tsx:162` 的标签格式化补上 `week` 分支。`home.service.ts` 的类型和 CLI 的说明同步加上 `week`。

### 4.2 查询计划：逐日增量 + 后端聚合

现状是每个桶、每个指标，以及窗口合计，各自按天切开再逐日发即时查询，同一天会被重复查询。新的计划：

1. 把窗口分为三段：开头不完整的那一天 `[start, D0)`、完整日 `[D0, Dn)`、结尾不完整的那一天 `[Dn, end)`。
   其中 `D0` / `Dn` 是显示时区的零点。
2. **完整日只发一条范围查询**：
   - 表达式：`sum by (<split>) (increase(<counter>{<sel>}[1d]))`；
   - `start = D0 + 1d`、`end = Dn`、`step = 86400`；
   - 每个点 `t` 就是 `(t-1d, t]` 这一天的增量。
   - 只在 `[D0, Dn)` 内没有夏令时切换（每天都正好 86400s）时可用。有切换时，退回现有的逐日即时查询
     （`volumeIncreaseChunks`）。Asia/Shanghai 不受影响。
3. 首尾两段各发一条即时查询（不足 1 天，单条即可）。
4. 后端把逐日值按桶边界累加成 day / week / month 桶。窗口合计等于逐日值之和加上首尾两段，**不再单独查询**。
   今日 / 本月两个 KPI 沿用现有逻辑；如果它们落在已经查询过的范围内，就直接复用结果。
5. 粒度为 quarter_hour / hour 时，窗口不超过 72h，保持现状。
6. **缓存**：过去的完整日不会再变化，用进程内 LRU 缓存逐日结果：
   - 键：`counter + sel + split + 当天零点`；
   - 上限 4096 条；
   - 已缓存的日子不再查询，缺失的日子按连续区间发范围查询；
   - "今天"永远不缓存。
7. 查询数断言：
   - 单测给 `instantQuery` / `rangeQuery` 计数；
   - 一年窗口、按月分桶、不拆分时，查询数不超过 `len(volumeMetrics) × 3`；
   - 缓存命中后只剩首尾两段的查询。

### 4.3 无数据桶

- 桶的 `EndMs <= oldest_sample_ms` 时标记 `no_data: true`，数值保持为 0。
- 跨越最早样本时间的那个桶照常计算，同时标记 `partial: true`。

### 4.4 `increase` 外推

逐日 `increase` 在每天的边界都会按 Prometheus 的规则外推，各天相加后与单条长窗口的 `increase` 存在微小差异。
现有实现本来就是逐日相加的，这次不改变语义。单测里的数值比较按现有容差处理。

## §5 文档与规范（R6）

| 文件 | 改动 |
|---|---|
| `config.example.yaml` | `retention` 改为注释示例；`max_size` 说明改为"缺省取 min(30GiB, 磁盘 20%)" |
| `docs/内置监控说明.md` | 保留策略、容量清理机制（整块删除、只看目录大小）、一年窗口、按周 / 月分桶 |
| `install/install-core.sh` | 安装摘要里的"默认保留 31 天、最多 30GiB"改为新口径 |
| `.trellis/spec/backend/embedded-tsdb-capacity.md` | 补充保留策略、块合并、一年窗口成本与基准测试结果 |
| `.trellis/spec/backend/engine-events-and-realtime.md` | 窗口 ≤ 366 天，补充新的步长示例 |
| `.trellis/spec/frontend/ui-conventions.md` | "前端截取 31 天"改为"截取 1 年并给出提示"；新增预设；无数据桶的样式 |
| swagger（`docs/docs.go` / `swagger.*`） | 用 `swag` 重新生成，不要手改 |
| `docs/release-notes/unreleased.md` | #157 合入后再补；没合入时在 PR 描述里提醒 |

## 已知风险

| 风险 | 处理 |
|---|---|
| 一年窗口 CPU 开销大，超时 | 长窗口超时放宽到 32s；门槛不达标时不放开到一年，降采样单独立项 |
| 磁盘 20% 在大盘上远超 30GiB | 取较小值，所以封顶 30GiB |
| 小盘算出的上限过小 | 下限 1GiB |
| 块合并后单次删除的数据变多 | 最大块 162h，按 2 万序列估算约 1–1.5GB |
| DST 时区下范围查询的步长不等于一天 | 检测到切换时退回逐日即时查询 |
| 缓存占用内存 | 上限 4096 条；每条是按拆分维度聚合后的向量，拆分方式为作业时最多是作业数 |
