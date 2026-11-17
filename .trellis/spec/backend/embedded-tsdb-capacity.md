# 内嵌 tsdb 容量与默认值

> 适用于 `observability.max_samples`、`observability.sample_limit` 与序列规模基准（`internal/pkg/embedts/capacity_bench_test.go`）。

## 场景：按实测收紧查询/抓取上限

### 1. 范围 / 触发条件

- 内嵌 tsdb 与 API 同进程、同 GC，没有资源隔离。
- 首页 7d 面板 step=1800s，约 336 点/序列；一次 `sum(...)` 仍会先加载匹配序列。
- `stx_job_table_rows_total` 带 `table × direction`，整库 CDC 可能把活跃序列打到万级以上。本轮不改标签，用抓取 `sample_limit` 和查询 `max_samples` 做护栏。

### 2. 签名

```go
const DefaultMaxSamples = 10000000 // ~160MB at 16B/sample
const DefaultSampleLimit = 50000   // per-target scrape cap
```

配置：

```yaml
observability:
  max_samples: 10000000
  sample_limit: 50000
```

### 3. 契约

- `max_samples=0` 视为未配置，启动时填 `DefaultMaxSamples`；负值校验失败。
- `sample_limit=0` 视为未配置，启动时填 `DefaultSampleLimit`；负值校验失败。需要更大上限时显式写更大的数。
- 单目标单轮展平后样本数超过 `sample_limit`：丢弃该目标本轮样本，`up=0`，`TargetState.LastError` 以 `sample limit exceeded` 开头；其他目标照常提交。
- 超限判定在构建 `FlatSample` / 标签集之前按展平后样本数计数，数到 `limit+1` 即停；不得先把整份 exposition 物化再丢弃。响应体仍受 `maxExpositionBytes`（64MiB）限制。
- 推荐活跃序列上限 **2 万**（7d 面板约 670 万样本，低于 1000 万查询上限）。超过后优先截断 `table` 标签或拆作业，而不是再把 `max_samples` 调回 5000 万。
- **长窗口 `increase` 必须分桶**：`max_samples` 按「一次查询同时加载的原始样本数」计。15s 抓取、持续 31 天的作业每条序列约 17.9 万点；1000 万 ÷ 17.9 万 ≈ **56** 条长跑作业就会让 `increase(...[31d])` 报 `query processing would load too many samples`。首页「本月」总量和按月柱必须改成按显示时区日桶 `increase(...[1d])` 再求和（完整日桶求值时刻对齐当地零点，可命中 15s PromQL 缓存）。面板取点（7d × 1800s step）和长窗口 `increase` 是两类负载，不能只用前者推算容量。

### 4. 基准怎么跑

#150 第 2 步要两档，不能互相代替：

1. **突发写入**：三档序列一次性灌进 head，用来比较规模和收紧默认值。
2. **实时长时间运行**：N 个 HTTP 目标按墙钟 `15s` 持续抓取，至少跑 **48h**，跨过真实 2h head compaction，记 RSS / heap / GC / 单轮抓取耗时 / compaction 期间查询 p99。

#### 4.1 突发写入（三档规模，秒级）

```bash
go test ./internal/pkg/embedts -count=1 -timeout 15m -run 'TestCapacity_TenThousandSeries'
STX_TSDB_BENCH=1 go test ./internal/pkg/embedts -count=1 -timeout 30m -run 'TestCapacity_LargerProfiles'
STX_TSDB_BENCH=full go test ./internal/pkg/embedts -count=1 -timeout 60m -run 'TestCapacity_LargerProfiles'
```

10k 档默认进 `go test`；50k / 20 万由环境变量打开，避免 CI 内存打满。

#### 4.2 实时长时间运行（至少 48h）

```bash
./scripts/tsdb-soak.sh
# 等价于：
# STX_TSDB_SOAK=48h STX_TSDB_SOAK_SERIES=20000 STX_TSDB_SOAK_TARGETS=4 \
# STX_TSDB_SOAK_INTERVAL=15s STX_TSDB_SOAK_SAMPLE=1m \
# STX_TSDB_SOAK_OUT=tmp/tsdb-soak-48h.jsonl \
# go test ./internal/pkg/embedts -count=1 -timeout 50h -run '^TestCapacity_Soak$'
```

#### 4.3 历史块回灌（约 25GiB，看查询损耗 / 点查）

CDC soak 的真实抓取只有几百序列，首页接口会一直是几毫秒。要把 `data/tsdb` 做到接近生产体积再看点查和聚合差：

```bash
STX_TSDB_BACKFILL=25g ./scripts/tsdb-backfill.sh write
# 写入 data/tsdb-staging；导入前短暂停 API（SeaTunnel 作业不停）
./scripts/tsdb-backfill.sh import
# 中断后可直接再跑 write：已有块窗口会跳过，从缺口续写，不会把最老窗口再写一遍就提前达到 TargetBytes。
```

回灌指标是 `stx_loadtest_gauge{job="loadtest",series="N"}`，不混进作业读写量。点查 / 区间查询走 `GET /api/v1/monitoring/query` 与 `/query_range`（CLI：`stx monitoring query` / `stx monitoring query-range`）。

块按 2h 对齐，最新一块可比 `now` 早最多 2h，所以 `time=now` 的点查会空；评估时刻应落在 `newest_max_time` 内。周期观察只打点查 / 单序列 range，不要对 25GiB 数据循环发 `sum(stx_loadtest_gauge)` 的 7d/31d。

短跑（证明 harness，不是 48h 结论）：

```bash
STX_TSDB_SOAK=3m STX_TSDB_SOAK_SERIES=10000 STX_TSDB_SOAK_INTERVAL=15s \
  STX_TSDB_SOAK_BLOCK=1m STX_TSDB_SOAK_OUT=tmp/tsdb-soak-3m.jsonl ./scripts/tsdb-soak.sh
```

`TestCapacity_SoakSmoke` 默认进 `go test`（约 2.5s）。48h 不要设 `STX_TSDB_SOAK_BLOCK`，沿用生产 2h 块。JSONL 每分钟一行：RSS、heap、GC pause、scrape_ms、compaction 计数、首页 `sum(stx_capacity_value)` 的 1h/7d/30d、作业趋势 `sum(rate(stx_job_source_received_rows_total[1m]))` 的 1h。跑完把 summary 贴回本节 5.2。

机器要能连续开 48h；8G 部署机建议 2 万序列。磁盘粗算：2 万序列 × 48h × 15s ≈ 2.3 亿样本，压缩后大约 1–3GiB。

### 5. 本机测量

#### 5.1 突发写入（2026-11-17，Linux cloud agent）

每条序列 8 个点、时间戳从 now 起每 15s 一个，查询表达式 `sum(stx_capacity_value{job="bench"})`。这不是 31 天 head：真实 15s 抓取的 2h head 内存会高一个数量级以上，所以推荐上限按 8G 机器留余量，而不是按 20 万档的查询延迟。

| 档位 | 写入 | Compact | 1h 查询 | 7d 查询 | HeapInuse | RSS | Head 序列 | Head 字节 | Blocks 字节 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 1 万 | 36ms | 44ms | 40ms | 48ms | 18MB | 53MB | 10000 | 1.6MB | 1.2MB |
| 5 万 | 180ms | 169ms | 228ms | 250ms | 51MB | 125MB | 50000 | 8.1MB | 6.8MB |
| 20 万 | 725ms | 768ms | 917ms | 1081ms | 177MB | 439MB | 200000 | 32MB | 27MB |

20 万档在短窗口下查询仍约 1s，但 RSS 已 439MB；再乘上真实 head 密度后 8G API 进程不安全。推荐活跃序列 **2 万**，`max_samples=10000000`（约 160MB），`sample_limit=50000`。

#### 5.2 实时长时间运行（至少 48h）

按 15s 墙钟抓取、生产 2h 块、2 万序列。数字必须来自 `./scripts/tsdb-soak.sh` 的 summary / JSONL，不能用 5.1 的 8 点突发档外推。

| 时长 | 序列 | 目标 | 块时长 | RSS 末 | Heap 末 | Head 序列 | 抓取 p99 | 首页 1h p99 | 首页 7d p99 | 首页 30d p99 | 作业趋势 1h p99 | compaction 期间 1h p99 | GC pause 累计 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 3m（短实时，1m 块） | 1 万 gauge + 1 万 counter | 4 | 1m | 111MB | 41MB | 20281 | 34ms | 91ms | 106ms | 107ms | 141ms | 91ms | 2.1ms |
| 48h | 2 万 | 4 | 2h（生产） | （跑完填写） | （跑完填写） | （跑完填写） | （跑完填写） | （跑完填写） | （跑完填写） | （跑完填写） | （跑完填写） | （跑完填写） | （跑完填写） |

3m 行是 2026-11-17 cloud agent 上 `STX_TSDB_SOAK=3m` 的真实 15s 抓取（跨过 3 次 compaction）。它证明 harness，**不能**代替 48h：head 只有几分钟密度，块也不是 2h。48h 行必须在目标机器上跑 `./scripts/tsdb-soak.sh` 后再填。

短实时冒烟（`TestCapacity_SoakSmoke`，秒级，CI）只证明循环能跑，不填上表。

#### 5.3 25GiB 历史块回灌（2026-11-17，Linux cloud agent）

`STX_TSDB_BACKFILL=25g` 写入 staging 后 import 进 `data/tsdb`。372 个 2h 块，约 20625 条 `stx_loadtest_gauge` 序列，块字节 25.02GiB（`max_size=30GiB`）。API 冷启动 RSS ≈ 322MB（块走 mmap）。CDC soak 作业不受 import 影响。

| 查询 | 窗口 | 服务端 elapsed | HTTP | 结果 |
| --- | --- | --- | --- | --- |
| `stx_loadtest_gauge{series="0"}` 点查 | 历史 +7d 时刻 | 2.1ms | 200 | 1 sample |
| `sum(stx_loadtest_gauge)` 点查 | 历史 +7d 时刻 | 155ms | 200 | 1 sample |
| `stx_loadtest_gauge{series="0"}` range | 7d / step=1800s | 8ms | 200 | 1 series |
| `sum(stx_loadtest_gauge)` range | 7d / step=1800s | 35.1s | 200 | 1 series |
| `sum(stx_loadtest_gauge)` range | 31d / step=1800s | 60s（本机把 `query_timeout` 临时调到 60s） | 500 `query timed out in expression evaluation` | 随后 RSS 9.4GB、VM 37GB，API 被 OOM kill |

首页 / 作业趋势带紧匹配器，不扫 `stx_loadtest_gauge`，25GiB 后仍是几毫秒到几百毫秒。点查能力要看带 `series="N"` 的 instant 查询，不是看首页。默认 `query_timeout=8s` 会挡住 7d/31d 全量 `sum()`，但超时返回前仍可能把堆打高；不要在 soak 观察循环里发这类查询。

### 6. Good / Base / Bad

- Good：一个目标 exposition 过大时只该目标 `last_error=sample limit exceeded`，邻居目标 `up=1`。48h soak 的 JSONL 里 `compaction_window=true` 的样本上首页 1h p99 仍低于 `query_timeout`。
- Base：8G 机器默认 `max_samples=10000000`，7d 首页聚合查询在 2 万序列内不 `ErrTooManySamples`。本月读写量走日桶 `increase[1d]`，56+ 条 CDC 作业也不会因单条 `[31d]` 触顶。
- Bad：把独立 Prometheus 的 5000 万原样搬进 API 进程；或超限时连累整轮 appender；或用 8 点突发档外推 48h RSS；或对本月总量发一条 `increase(...[31d])`；或对 25GiB `stx_loadtest_gauge` 循环发 31d `sum()`（`max_samples` / `query_timeout` 拦不住超时前的 RSS，16G 机器会被 OOM kill）。

### 7. 必须有的测试

- `TestScraper_SampleLimitDropsOneTargetKeepsOther`
- `TestFlattenMetricFamiliesLimited_OverLimitSkipsMaterializing`
- `TestCapacity_TenThousandSeries`（默认跑）
- `TestCapacity_SoakSmoke`（默认跑，秒级真实抓取循环）
- `TestCapacity_Soak`（`STX_TSDB_SOAK=48h`，文档要求至少跑 2 天）
- `TestWriteLoadtestBlocks_Smoke`（默认跑）；`TestCapacity_Backfill`（`STX_TSDB_BACKFILL=25g`）
- `TestValidateConfig_EmbeddedObservabilityInvalid` 覆盖负 `sample_limit`
- `TestVolumeIncreaseChunks_LongSpanSplitsDays` / `TestGetVolume_NoGranularityOnlyTotals` / `TestGetVolume_MonthBucketsUseDayIncreases`：长窗口 `increase` 按日分桶，单条查询不超过约 1 天
