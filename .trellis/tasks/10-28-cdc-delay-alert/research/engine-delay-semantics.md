# 引擎 CDC 延迟事件的语义（2026-10-28 核实）

来源：`/data/workspace/spike/seatunnel`（upstream dev，7604a83e），以及 STX 现有实现。

## 1. 引擎何时上报 `READER_MESSAGE_DELAYED`

`connector-cdc-base/.../IncrementalSourceRecordEmitter.java` 的 `reportMetrics`：

```java
Long messageTimestamp = getMessageTimestamp(element);   // source.ts_ms：源库提交时间
if (messageTimestamp != null && messageTimestamp > 0L) {
    long emitDelay = now - messageTimestamp;            // worker 当前时间 - 源库提交时间
    recordEmitDelay.set(...);
    if (delayedEventLimiter.acquire(messageTimestamp)) {   // 延迟 ≥1 秒且限速 0.5 条/秒
        eventListener.onEvent(new MessageDelayedEvent(emitDelay, element.toString()));
    }
}
```

- `delayTime` = **SeaTunnel worker 当前时间 − 源库里这条变更的提交时间**。两台机器的时钟差会直接加进延迟。
- 只有**真正发出的记录**才会上报：
  - 源库没有写入时没有记录，也就没有事件；
  - 读取卡住时同样没有事件。
  - 所以"收不到事件"既可能是延迟 <1 秒，也可能是没有数据或卡住。B4 只能发现"数据在走但是晚了"，发现不了"完全卡住"，后者属于 B3（不在本任务内）。
- 心跳记录的 value 里没有 `source` 字段，`getMessageTimestamp` 返回 null，不会上报。
- 全量快照阶段的记录 `source.ts_ms` 为 0，不会上报。
- **追赶期**：全量阶段结束后，增量读取从最早的快照高水位开始，会把全量期间积压的 binlog 补读一遍，这段时间延迟可能接近全量耗时，然后逐步下降。从 checkpoint 恢复、暂停后恢复也一样。这是"误报"的主要来源。
- **大事务**：一个长事务里的变更带的是事务内的时间戳，提交后一起发出，会出现短暂的大延迟尖峰。
- 限速器：每个 reader 一个，`RateLimiter(0.5/s)`。增量阶段通常只有一个 reader 读 stream split。引擎每 10 秒批量投递一次，所以 STX 每 10 秒最多收到约 5 个样本。

## 2. STX 现状

- 接收端：`internal/apps/sync/engine_events.go`
  - `sanitizeEngineEvent`：只保留 `delay_ms`，删除 `record`（整行数据），时间线按分钟去重。
  - `recordCDCDelay`：内存汇总每个作业的最近值和 1 分钟最大值，按样本原始时间单调前进，换主后重放的旧样本会被忽略。
  - `jobCDCDelay`：60 秒内有新样本算 `fresh`。
- **不落库**：STX 重启后汇总清零。时间线表 `sync_engine_events` 里有每分钟一个点（保留 7 天）。
- 前端只在控制台作业行显示徽标（`ConsolePanel.tsx`），≥60 秒标红。
- 事件通道：
  - 默认关闭，需要用户在集群上开启并滚动重启；只有 3.x 集群支持。
  - 健康状态只在提交作业后的 45 秒预期窗口里判定（收到状态事件算健康，没收到算降级）。
  - 降级时上报 D2。

## 3. 告警框架现状（第一期）

- 契约：`internal/pkg/syncalert`。
  - 信号类型 Fire、Resolve、Observe。Observe 目前只给 D1 用，带 `Since`，由 monitoring 按规则窗口判断是否成立。
  - `TaskConfig` 没有按规则区分的参数。
- 策略存储：
  - 条件 `AlertPolicyConditionDTO{metric_key, operator, threshold, window_minutes}` 已经有阈值和窗口字段，现在固定写 `>= 1`。
  - B4 的阈值和持续时间可以放进同一个条件，不用改表。
- hub：`internal/apps/monitoring/sync_alerts.go`。
  - `resolveEpisode` 的 reason 不是 `recovered` 时只关闭、不发通知，可以用来"静默关闭"。
  - sweeper 每分钟跑一次。

## 4. 补充核实（2026-10-28）

- `JobEventHttpReportHandler`：
  - 定时任务里 ringbuffer 读不到事件时直接返回（`resultSet.size() <= 0`），本地缓冲为空时也直接返回。
  - **不会发空批**。通道安静时 STX 收不到任何请求，无法区分"一切正常"和"通道断了"。
- `rest/service/BaseService.getJobMetrics`：只输出白名单里的指标（`SourceReceivedCount`、`SinkWriteCount`、QPS、表级计数等），没有 `CDCRecordEmitDelay` 和 `CDCRecordFetchDelay`。
- STX 每次拉 job-info 时，把运行中作业的指标写进 `result_preview["metrics"]`。
  - 事件模式下没人查看的作业，30 秒拉一次；否则 10 秒一次。
  - B4 用里面的 `SourceReceivedCount` 判断"数据还在走"。
