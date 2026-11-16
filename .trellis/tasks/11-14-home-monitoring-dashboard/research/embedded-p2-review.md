# 内嵌可观测 P2 评审（大盘视角，2026-11-16）

对象：`11-16-embedded-observability` design §4–§5、implement P2，以及工作区中进行中的 `embedts/scraper.go`、`flatten.go`、`service.go` 改动。

## 需要改

1. **P2 删 HTTP SD / 外部组件，会让 P3 合并前的告警静默失效**（高）
   - 现状：metrics_template 策略与 11 条默认告警仍由外部 Prometheus 评估、经 AM 回流，直到 P3 才换成进程内评估器。
     外部 Prometheus 的抓取目标全部来自 HTTP SD（`prometheus.yml.tpl` 两个 job 都是 `http_sd_configs`）。
   - P2 若删 HTTP SD 与 `jobs/metrics`，外部 Prometheus 立刻没有目标，告警全部不再触发，且没有任何报错。
   - `GetIntegrationStatus`（工作区已改）去掉了 prometheus / alertmanager 组件；策略中心能力卡的 `depends_on` 若随之改到 `embedded_tsdb`，
     会在告警实际仍依赖外部 Prometheus 时显示「可用」。
   - 与 implement.md 回滚点「P3 前外部 Prometheus 都可继续共存」自相矛盾。
   - 建议：HTTP SD、`jobs/metrics`、prometheus / alertmanager 组件的删除**移到 P3**（真正的退役点）；P2 只新增 `embedded_tsdb` 与改造 `seatunnel_metrics`。
     或者 P2 与 P3 连续合并、中间不发版，并在 implement.md 写明。

2. **作业终态时最后一段增量丢失**（中，影响 E2 读写量与短批作业）
   - `job_prometheus.go` `deleteJob` 在终态时立即 `DeletePartialMatch`；下一轮 gather 发现序列消失只写 stale marker，
     上一轮 gather 到终态之间的增量（最多一个 `scrape_interval`）永远不进 tsdb。运行不足 15 秒的批作业可能一个样本都没有，
     全局吞吐、Top N、E2 日 / 月量都会漏。
   - 外部 Prometheus 时代同样有这个问题，但内嵌后可以修：终态时先同步把该作业当前值追加一次（时间戳 = 终态时刻），再写 stale marker、再删序列。
   - 建议写进 design §5 与 P2 验收（跑一个 5 秒的 FakeSource 批作业，`increase` 能查到写入量）。

3. **抓取目标标签要与 HTTP SD 完全一致**（中，确认项）
   - design §4 只列了 `cluster` / `cluster_id` / `env`；HTTP SD 实际下发 `job`、`cluster_id`（为 0 时是 `"static"`）、`cluster_name`、`cluster`（= 名称）、`env`（空为 `"unknown"`）。
   - `cluster_name` 被 metrics_template 的 `sum by (cluster_id, cluster_name, cluster, instance)` 与 `remote_integration.go` 告警解析使用。
     scraper.go 注释已写含 `cluster_name`，但目标提供者尚未落地。
   - 建议：抽一个 `managedTargetLabels(item)` 同时给 HTTP SD（P3 删除前）和 scraper 用，加单测锁住标签集。design §4 同步补全。

## 建议加

4. **引擎查询并发上限**（中）：`embedts.go` 的 `promql.NewEngine` 没有 `ActiveQueryTracker`，并发不设限。首页集群 / 作业大盘单次约 19 / 15 条并发查询，
   多人打开即成倍占用 STX CPU。建议在 `promquery` 加信号量（如 `GOMAXPROCS`）或启用 `ActiveQueryTracker`。大盘这边在 P4 加 15 秒查询缓存配合。
5. **P1 / P2 合并节奏**（中）：P1 已把查询切到 tsdb，P2 前 tsdb 是空的，单独合并 P1 会让所有大盘与作业趋势变成空态。implement.md 写明 P1、P2 连续合并。
6. **并存期抽样核对**（低）：P2 合并后到 P3 之前外部 Prometheus 与内嵌抓取并存（SeaTunnel 被抓两次，可接受）。这正好是核对「数据与外部部署抽样一致」的窗口，
   P3 之后就没有参照了。大盘 P4 会做吞吐 / 堆 / Top N 的抽样，建议内嵌 P2 验收也在这个窗口完成。

## 已确认没问题

- 并发抓取后在单个 appender 上串行追加（`group.Wait()` 之后），没有 appender 跨 goroutine 共用问题。
- 抓取失败时本轮 `written` 只有 `up`，`SeriesTracker.Diff` 会把该目标上一轮的全部序列标 stale，节点宕机后趋势不会续存 5 分钟。
- `expfmt.NewTextParser(model.LegacyValidation)` 用法与 prometheus/common 当前版本一致。
