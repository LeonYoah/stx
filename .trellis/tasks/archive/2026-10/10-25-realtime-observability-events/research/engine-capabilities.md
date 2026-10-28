# SeaTunnel 引擎侧能力：事件上报与实时可观测性

> 调研时间：2026-10-25。证据来源：
> - t1 上的 `/opt/seatunnel` 3.0.0 发布包（解出 jar，用 `javap`/`strings` 核对）
> - Maven Central 上 2.3.4–2.3.12、3.0.0 的 `seatunnel-api` / `seatunnel-engine-common` / `seatunnel-engine-server` jar
> - 上游 dev 源码 `/data/workspace/spike/seatunnel`（`7604a83`，3.1.0-SNAPSHOT）
> - t1 上 3.0.0 实测：`curl localhost:8080/metrics/realtime/jobs`
> - 前序 spike：`.trellis/tasks/10-24-zeta-application-mode/research/k3s-spike.md`、`/data/workspace/spike/events/standalone.jsonl`

## 1. 版本矩阵

| 版本 | `event-report-http` | `JOB_STATUS` 事件 | 非终态（`report-non-terminal-job-state`） | `/metrics/realtime` |
| --- | --- | --- | --- | --- |
| 2.3.0 – 2.3.4 | ✗ | ✗ | ✗ | ✗ |
| 2.3.5 – 2.3.11 | ✓（只有生命周期和 schema change） | ✗ | ✗ | ✗ |
| 2.3.12（STX 推荐版本） | ✓ | ✓，**只推终态** | ✗ | ✗ |
| 3.0.0 及以上 | ✓ | ✓ | ✓ | ✓ |

核对方法：
- `EventType.class` 里有没有 `JOB_STATUS` 常量；
- `JobStateEvent.class` 是否存在；
- `ServerConfigOptions` 里有没有 `report-non-terminal-job-state` 字符串；
- 2.3.12 的 `PhysicalPlan.reportJobStateEvent` 只调用了 `isEndState`；
- 3.0.0 的 starter jar 里有 `observability/RealtimeMetricsService.class`，2.3.12 没有。

## 2. 实时可观测性（3.0.0）

### 开关：作业级，提交时注入即可

```hocon
env {
  engine {
    observability {
      enabled = true          # 默认 false
      bucket_ms = 5000        # 最小 1000
      retention_minutes = 10  # 范围 1–10，默认 3
      async_boundaries = []   # 会改拓扑：在指定 transform 前插入有界队列
      edge_buffer_capacity = 0
      edge_overrides = []
    }
  }
}
```

- `ObservabilityConfig.fromEnvOptions` 从 `JobDAGInfo.getEnvOptions()` 读取。显式 `enabled=false` 优先级最高；未显式设置时，配了 `async_boundaries` 或 `split_sink_io` 会自动开启。
- worker 端用 `ObservabilityConfig.resolveEnabled` 判断，**关闭时不计数**，所以开销只落在开启的作业上。

### 采集与接口

- 只有 active master 采集：`RealtimeMetricsService`，`POLL_INTERVAL_MS = 5000`（3.0.0 jar 用 `javap` 确认），`METRICS_FETCH_TIMEOUT_MS = 3000`。
  - **网页文档里写的 1 秒是错的。**
  - t1 实测返回 `pollIntervalMs: 5000`。
- 数据只在内存里，换主后丢失，作业结束后清除。
- 走 v2 REST（Jetty，`http.enable-http` 端口）：
  - `GET /metrics/realtime/jobs` → `{jobs:[{jobId, enabled, bucketMs, retentionMinutes, latestBucketStartMs}], collector:{pollIntervalMs, fetchTimeoutMs, lastCollectStartMs, lastCollectEndMs, lastRawMetricsFetchCostMs, lastRawMetricsBlobs, collectFailureCount, lastCollectError?}}`
  - `GET /metrics/realtime/jobs/{jobId}/vertices?windowMs=`（默认 180000，最大 600000）→ `{enabled?, bucketMs, fromMs, toMs, vertices:[{vertexId, points:[...]}]}`
  - `GET /metrics/realtime/jobs/{jobId}/edges?windowMs=` → `{enabled?, bucketMs, fromMs, toMs, edges:[{queueId, targetVertexId, points:[...]}]}`
  - 作业未知时返回 `{}`；未开启时返回空数组加 `enabled:false`。
- **只能找 active master**：`BaseServlet.getSeaTunnelServer(true)` 判断 `isMasterNode()`，否则返回 **404 `Not a master node`**；服务未启动返回 503 `Realtime metrics disabled`。

### 字段

- 节点 point（vertex）：`ts, subtaskCount`，再按节点类型分别有：
  - source：`sourceReadNs, sourceIdleNs, sourceReadRatio, sourceIdleRatio`
  - transform：`transformProcessNs, transformRecordsIn, transformRecordsOut, transformBusyRatio`（以及每条记录的耗时）
  - sink：`sinkWriteNs, sinkRecordsIn, sinkPrepareCommitNs, sinkCommitNs, sinkAbortNs, sinkBusyRatio, sinkWriteNsPerRecord`
- 边 point（edge）：`ts, emitBlockedNs, subtaskCount, bpRatio, queueCapacity, queueSize, queueFillRatio`。
- `vertexId` 和 job-info 返回的 `jobDag.vertexInfoMap` 编号一致。
- 边按 `queueId` 区分：async boundary 的 `queueId = -2×actionId`，sink 拆分的 `queueId = -(2×actionId+1)`；用 `targetVertexId` 对应到 DAG 上的边。多个输入指向同一节点时，单靠 `targetVertexId` 区分不开，要用 `queueId` 区分。
- 只有带 IntermediateQueue 的边才有背压数据，也就是 sink 的输入边和 async boundary 插入的边。
- 上游判定表（busyness-and-backpressure.md）：
  - 输入边 `bpRatio` 和 `queueFillRatio` 持续偏高，同时下游忙碌度也高 → 下游跟不上。
  - source `sourceIdleRatio` 高，同时下游队列是空的 → 上游没有数据。
  - source `sourceReadRatio` 高，但第一条可观测的边没有背压 → 读取或同步执行的链路慢。
  - transform `transformBusyRatio` 高，同时它的输入边又满又有背压 → 这条 transform 链是瓶颈候选。
  - sink `sinkWriteNsPerRecord` 高，同时输入边又满又有背压 → 目标库慢。
- 上游 UI 的颜色阈值：`bpRatio` 为 0 灰，<5% 绿，<20% 黄，<50% 橙，≥50% 红；边宽对应 `queueFillRatio`。上游 UI 每 2 秒刷新一次。

## 3. 事件上报（3.0.0）

### 配置（集群 `seatunnel.yaml`，改了要重启）

```yaml
seatunnel:
  engine:
    event-report-http:
      url: "http://stx/api/v1/sync/engine-events/1"
      headers:
        Authorization: "Bearer <token>"
      report-non-terminal-job-state: true   # 必须写在 event-report-http 下面（YamlSeaTunnelDomConfigProcessor）
```

### 投递语义（`JobEventHttpReportHandler`）

- 只在 master 上创建，worker 上的事件经 `JobEventReportOperation` 转发到 master。
- Hazelcast ringbuffer `zeta-job-event`：容量 2000，`OverflowPolicy.OVERWRITE`，异步备份 1 份。
- 固定每 10 秒（`REPORT_INTERVAL`，不能配置）读最多 1000 条，作为一个 JSON 数组 POST 出去，带上配置的 headers。
- `committedEventIndex` 只在收到 2xx 后前进 → 至少送达一次，STX 停机期间的事件会补发。**返回非 2xx 时会一直重试同一批，后面的事件全被堵住。**
- 换主后新 handler 从 `headSequence` 开始读，会产生重复。事件没有 ID。
- 3.0.0 关闭时会把本地缓冲区尽力冲刷一次；2.3.12 没有这个缓冲区。
- `JobEventProcessor` 把每个事件发给**所有** handler：开了 stain trace 后，`STAIN_TRACE` 事件也会进同一个 ringbuffer（每个 worker 每秒最多 50 条），挤占 `JOB_STATUS`。
- standalone 实测延迟 6.5–9.9 秒，并观察到整批重放。

### payload

| eventType | 字段 |
| --- | --- |
| `JOB_STATUS` | `jobId, jobName, jobStatus, createdTime` |
| `LIFECYCLE_ENUMERATOR_OPEN/CLOSE`、`LIFECYCLE_READER_OPEN/CLOSE`、`LIFECYCLE_WRITER_CLOSE` | `jobId, createdTime`（没有 task/vertex 编号，每个 reader/writer 一条） |
| `SCHEMA_CHANGE_ADD_COLUMN/DROP_COLUMN/MODIFY_COLUMN/CHANGE_COLUMN/UPDATE_COLUMNS/RENAME_TABLE/RESTORE` | 表标识和列变更（AlterTable* 事件序列化） |
| `READER_MESSAGE_DELAYED` | `jobId, createdTime, delayTime(ms), record`。CDC 发射延迟 ≥1 秒时触发，每个 reader 每秒最多 0.5 条（`MessageDelayedEventLimiter`）。**`record` 是 `SourceRecord.toString()`，包含整行数据** |
| `STAIN_TRACE` | `jobId, traceId, payload(byte[]→base64), sinkTaskId, tableId` |

- job-info 的 metrics 只有 Source/Sink 的计数、QPS 和字节数，**不含** CDC 的 `CDCRecordEmitDelay` / `CDCRecordFetchDelay`。
- 所以 `READER_MESSAGE_DELAYED` 是 STX 拿到 CDC 延迟的唯一途径，而且只有延迟 ≥1 秒时才会出现。

## 4. 本轮不采用

- Stain trace：要在引擎和作业两级都打开，而且会挤占事件通道，放到 P3。
- 自定义 SPI `EventHandler`：2.3.12 和 3.0.0 都支持用 `ServiceLoader` 加载，可以绕开 10 秒间隔和堵塞问题，但要在每个 master 上放 jar，还要跟着引擎版本做兼容。
- Prometheus telemetry：只有 `job_count`、线程池这类集群级指标，没有单个作业的吞吐。
