# 进程剖析（pprof）

> 适用于 STX 控制面进程的 Go pprof：本机 loopback 监听、管理员 API 与 CLI。

---

## 范围

- 默认关闭。配置 `app.pprof.enabled: true` 后才提供剖析。
- 本机监听默认 `127.0.0.1:6060`，路径为标准 `/debug/pprof/`，供 `go tool pprof` 使用。
- 管理员 API：`GET /api/v1/debug/pprof/:name`（需登录且管理员）。未开启时返回 404。
- 操作登记 `debug.pprof.get` 必须 `AdminOnly: true`，让 `/capabilities` 对非管理员返回 `admin_required`，CLI 在下载前就拒绝。
- CLI：`stx debug pprof <name> --file <path>`。`cpu` 映射到 `profile`。CPU / trace 用 `--seconds`（默认 30）。

## 契约

- 剖析文件可能含堆上的密钥与业务数据，只给管理员，且默认不监听非 loopback。
- CLI 成功时 stdout 仍是单个 JSON（含 `file` / `size` / `sha256`），二进制只写 `--file`。
- 本机监听失败应阻止进程启动（显式开启却绑不上端口）。
- 作业趋势 `/sync/jobs/:id/timeseries` 的多条 PromQL 必须并发执行；引擎内部仍受 `maxConcurrentQueries` 排队。

## 测接口性能

内嵌 tsdb 之后，首页大盘和作业趋势会在 STX 进程内跑 PromQL。建议：

1. 配置 `app.pprof.enabled: true`，重启 `stx server`。
2. 压首页或作业趋势：`stx dashboard home clusters --range 1h`、`stx dashboard home jobs --range 1h`、`stx sync job timeseries <id>`。
3. 另开终端：`go tool pprof -http :0 http://127.0.0.1:6060/debug/pprof/profile?seconds=30` 看 CPU；heap 用 `/debug/pprof/heap`。
4. 或 `stx debug pprof cpu --seconds 30 --file cpu.pprof` 后本地 `go tool pprof cpu.pprof`。

仓库内基准：`go test -bench=TSDB -benchtime=5s ./internal/apps/dashboard ./internal/apps/sync`。

夹具是 3 集群 × 2 节点、8 个作业、1h / 15s 步长。在 4 核 Xeon 上测过一次：

| 路径 | 约 ns/op | 约 B/op | allocs/op |
|------|----------|---------|-----------|
| 集群首页大盘冷查询 | 1.0ms | 952KiB | 6637 |
| 同上，命中 15s PromQL 缓存 | 61µs | 40KiB | 299 |
| 作业首页大盘冷查询 | 607µs | 481KiB | 3142 |
| 单条 heap 占比区间查询 | 524µs | 81KiB | 519 |
| 作业趋势（throughput/rows/cdc_delay，并发 PromQL） | 133µs | 104KiB | 1160 |

CPU 热点在 `promql.(*evaluator).eval` / `extrapolatedRate`（内嵌引擎本身），不是 HTTP 路由。作业趋势原先串行打 PromQL，已改为并发；首页大盘本身已并发且有 15s 缓存。
