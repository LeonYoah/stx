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

### 4. 基准怎么跑

```bash
go test ./internal/pkg/embedts -count=1 -timeout 15m -run 'TestCapacity_TenThousandSeries'
STX_TSDB_BENCH=1 go test ./internal/pkg/embedts -count=1 -timeout 30m -run 'TestCapacity_LargerProfiles'
STX_TSDB_BENCH=full go test ./internal/pkg/embedts -count=1 -timeout 60m -run 'TestCapacity_LargerProfiles'
```

10k 档默认进 `go test`；50k / 20 万由环境变量打开，避免 CI 内存打满。

### 5. 本机测量（2026-11-17，Linux cloud agent）

每条序列 8 个点、时间戳从 now 起每 15s 一个，查询表达式 `sum(stx_capacity_value{job="bench"})`。这不是 31 天 head：真实 15s 抓取的 2h head 内存会高一个数量级以上，所以推荐上限按 8G 机器留余量，而不是按 20 万档的查询延迟。

| 档位 | 写入 | Compact | 1h 查询 | 7d 查询 | HeapInuse | RSS | Head 序列 | Head 字节 | Blocks 字节 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| 1 万 | 36ms | 44ms | 40ms | 48ms | 18MB | 53MB | 10000 | 1.6MB | 1.2MB |
| 5 万 | 180ms | 169ms | 228ms | 250ms | 51MB | 125MB | 50000 | 8.1MB | 6.8MB |
| 20 万 | 725ms | 768ms | 917ms | 1081ms | 177MB | 439MB | 200000 | 32MB | 27MB |

20 万档在短窗口下查询仍约 1s，但 RSS 已 439MB；再乘上真实 head 密度后 8G API 进程不安全。推荐活跃序列 **2 万**，`max_samples=10000000`（约 160MB），`sample_limit=50000`。

### 6. Good / Base / Bad

- Good：一个目标 exposition 过大时只该目标 `last_error=sample limit exceeded`，邻居目标 `up=1`。
- Base：8G 机器默认 `max_samples=10000000`，7d 首页聚合查询在 2 万序列内不 `ErrTooManySamples`。
- Bad：把独立 Prometheus 的 5000 万原样搬进 API 进程；或超限时连累整轮 appender。

### 7. 必须有的测试

- `TestScraper_SampleLimitDropsOneTargetKeepsOther`
- `TestFlattenMetricFamiliesLimited_OverLimitSkipsMaterializing`
- `TestCapacity_TenThousandSeries`（默认跑）
- `TestValidateConfig_EmbeddedObservabilityInvalid` 覆盖负 `sample_limit`
