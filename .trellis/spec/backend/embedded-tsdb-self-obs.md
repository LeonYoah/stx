# 内嵌 tsdb 自观测

> 适用于 `internal/pkg/embedts` 自抓取、Go/进程采集器，以及 `GET /api/v1/monitoring/integration/status` 的 `embedded_tsdb.meta`。

## 场景：把 tsdb / engine / 进程指标写回内嵌存储

### 1. 范围 / 触发条件

- `embedts.Start` 创建的 `prometheus.NewRegistry()` 会挂上 tsdb、PromQL 引擎、Go runtime、process collector。
- 抓取循环每一轮（即使 SeaTunnel 目标为空）调用 `SelfScrapeOnce`：`Gather` → `FlattenMetricFamilies` → 写入 tsdb。
- 查询、状态接口、CLI 都读同一份数据，不恢复外部 Prometheus。

### 2. 签名

```go
const JobEmbedded = "stx_embedded"
func SelfScrapeOnce(ctx context.Context) error
func GetStorageStatus() (StorageStatus, bool)
```

CLI：已有 `stx monitoring integration status`（`monitoring.integration.status`），`Meta` 多几个键即可，不新增 operation。

### 3. 契约

- 注入标签：`job="stx_embedded"`、`instance="stx"`。exposition 自带 `job`/`instance` 仍按 `honor_labels=false` 改名为 `exported_*`。
- `up{job="stx_embedded"}` 只表示控制面自抓取在线。集群大盘的 `up` 查询都带 `cluster_id`，内置规则只看 `job="seatunnel_engine_http"`，因此不会把这条序列算进节点在线数。以后若加「全局目标在线数」一类查询，必须排除 `job="stx_embedded"`（以及 `stx_sync_jobs` 等非引擎 job）。
- 空目标集不能跳过自抓取：`scrapeRound` 在 `len(targets)==0` 时仍要 `SelfScrapeOnce`。
- `GetStorageStatus` 未就绪返回 `ok=false`；就绪时提供 head 序列数、stale 序列数、head/blocks 字节、最早样本、`compactions_failed`、`size_retentions`。
- 集成状态 `embedded_tsdb.meta` 在健康时增加：
  - `head_series` / `head_stale_series` / `head_bytes` / `blocks_bytes`
  - `compactions_failed` / `size_retentions`
  - `oldest_sample_ms`（没有样本时省略）
  - `max_samples` / `sample_limit`
- 前端忽略未知 meta 键。

### 4. 校验与错误

| 情况 | 行为 |
| --- | --- |
| tsdb 未启动 | `SelfScrapeOnce` 返回 `errSelfNotReady`；抓取轮记 WARN，不失败整轮 |
| Gather / Append 失败 | Rollback，返回 error；目标抓取已提交的样本不受影响 |
| Close 后重开 | 自抓取 SeriesTracker 必须 Reset，避免对上一份目录的序列补 stale |

### 5. Good / Base / Bad

- Good：空集群上 `prometheus_tsdb_head_series{job="stx_embedded"}` 与 `go_memstats_heap_inuse_bytes{job="stx_embedded"}` 可即时查询；`stx monitoring integration status` 的 `embedded_tsdb.meta.head_series` 非空。
- Base：SeaTunnel 目标不可达时自抓取仍成功，`up{job="stx_embedded"}=1`。
- Bad：`len(targets)==0` 时直接 return，自观测永远写不进去。

### 6. 必须有的测试

- `TestSelfScrapeOnce_WritesAC1Metrics`：AC1 指标可按 `job="stx_embedded"` 查询。
- `TestScraper_EmptyTargetsStillSelfScrapes`：空目标轮次仍写入自观测。
- `TestService_GetIntegrationStatus_storageMetaWhenReady`：状态接口带存储 meta。
