# 内嵌可观测 —— 技术设计

对应 `prd.md`。原则：**只借 Prometheus 的「存储 + 查询语言」，不借它的「服务器」**——`tsdb` 管数据落盘与压缩，
`promql` 管执行，其余（抓取、规则评估、告警分组投递）由 STX 进程内自研的最小机制替代。

## 1. 总体架构

```
                        ┌────────────────────────── STX 进程 ──────────────────────────┐
                        │                                                             │
   SeaTunnel 节点 ──────┼──► Scraper (自研循环) ──┐                                    │
   /hazelcast/.../metrics│   HTTP GET+expfmt      │                                    │
                        │                        ▼                                    │
   stx_job_* 生产者 ────┼──► Registry.Gather() ──►│  TSDB (embed)                        │
   stx_host_* (E3) ─────┼──► appender 直写 ──────►│  data/tsdb/  WAL+blocks+head         │
                        │                        │      │                             │
                        │                        │      ▼                             │
                        │  promquery.Range/Instant ──► promql.Engine (embed)           │
                        │      ▲                            ▲                         │
                        │      │ 5 处调用方(签名不变)         │ 告警评估器                  │
                        │      │                            │ MetricPolicyEvaluator   │
                        │  大盘/趋势/诊断/CLI                │ 定时 instant 查询          │
                        │                                   │   │ for 语义             │
                        │                                   ▼   ▼                     │
                        │                          dispatchAlertDelivery* (已有投递链)  │
                        │                                   │                         │
   外部系统 ────────────┼──► AM webhook 端点(保留) ─► RemoteAlertRecord → 投递链         │
                        └─────────────────────────────────────────────────────────────┘
```

被退役的外部面：Prometheus 进程、Alertmanager 进程、HTTP SD 端点、`/-/reload`、promtool、rule 文件生成、
AM 配置生成 + SIGHUP、Grafana 代理、安装侧两个二进制与镜像。

## 2. 存储层（`internal/pkg/embedts` 新包）

- `tsdb.Open(dir, logger, reg, opts)`：`dir = <data>/tsdb`（`observability.storage.dir` 可配，默认 `data/tsdb`）。
- `Options.RetentionDuration` ← `observability.storage.retention`（默认 `31d`）。
- `Options.MaxBytes` ← `observability.storage.max_size`（**缺省 30GiB**，显式 0=不限，走 IsSet 判定同
  `observability.enabled`）。tsdb 原生支持两种保留策略，直接映射。
- `Options.NoLockedfile = true` 视平台（Windows 兼容留口）。
- Head 内存：STX 规模（几十节点 × ~90 引擎指标 + 作业/主机指标 ≈ 万级活跃序列）对应 head 几十 MB，
  不需要 `HeadChunksWriteBufferSize` 等调优参数先暴露，留 config 扩展位。
- 生命周期：`observability.enabled=false` 或 STX 优雅停机时 `db.Close()`（`Close` 会 head compact + wal flush，
  注意 main.go 停机序列里放在 api server 停止**之前**，避免停机时还在写入）。
- **升级 = 破坏性变更（用户确认不兼容）**：不迁移时序历史，重采集即可；旧 prometheus/AM 数据目录保留
  在磁盘不删、只是不再拉起。可选逃生口：`storage.dir` 指到旧 prometheus 数据目录可直读既有块（同为
  tsdb 格式），docs 留手动指引，不做自动搬移。

## 3. 查询层（改 `internal/pkg/promquery`）

- `promql.NewEngine(EngineOpts{MaxSamples: cfg.max_samples(默认 50M 同 prom), Timeout: cfg.query_timeout(默认 8s,对应当前 httpClient 8s)})`。
- `promquery.BaseURL()` 语义改写为可用性探测：tsdb 就绪返回 `"embedded"` 哨兵串 + `""`，否则 `""` + reason
  （`observability_disabled` / `prometheus_unavailable` 两个 reason 保留，前端语义不动）。
- `Range(ctx, base, expr, start, end, step)`：忽略 base 参数（只作非空检查），`engine.NewRangeQuery`，
  `promql.Matrix` → `[]Series`（`Values` → `[][2]float64{ts*1000, v}`，同现有 JSON 解析输出形状）。
- `Instant(ctx, base, expr, at)`：`engine.NewInstantQuery`，`promql.Vector` → `[]Sample`；`at` 零值 → `time.Now()`。
- 调用方 `dashboard/cluster.go`、`dashboard/jobs.go`、`sync/job_timeseries.go`、`home_summary.go`、`job_sparklines.go`
  **零改动**（签名与返回形状不变）。
- `diagnostics/task_execute.go` 的自有 HTTP 查询实现改为调 `promquery`（顺带去重）。
- 引擎健康也兼作 `integration status` 的 `embedded_tsdb` 组件状态。

## 4. 抓取层（`internal/pkg/embedts/scraper.go`）

- 目标源：复用现有 `collectManagedMetricsTargets` 逻辑（cluster → 在线节点 → `host:hazelcastPort` + metrics path），
  抽成 `MetricsTargetProvider` 接口供 scraper 与 integration status 共用；30s 刷新一次目标集（原 HTTP SD refresh_interval）。
- 循环：每 `observability.scrape_interval`（默认 15s）对全部目标并发 GET（超时 `min(10s, interval)`），
  `expfmt.TextParser` 解析 exposition：
  - Gauge/Counter/Untyped → 按值 append；
  - Summary/Histogram → 展平成 `*_bucket{le}` / `*_sum` / `*_count`（同 Prometheus 约定）；
  - 样本时间戳统一用抓取时刻（不 honor_timestamps，与默认行为一致）。
- 标签注入：`job="seatunnel_engine_http"` + `instance=host:port` + `cluster` / `cluster_id` / `env`（来自目标提供者）。
- `up` 合成：每轮每目标写 `up{job,instance,cluster,...}` = 1/0，失败时同时记 `last_error` 到内存状态
  （integration status 的 `seatunnel_metrics` 组件读它，替代现在对 `/api/v1/targets` 的查询）。
- 消失目标处理：目标集变化时，对消失目标的历史序列**写 stale marker**（`math.Float64frombits(value.StaleNaN)`），
  防 PromQL 查询里幽灵续存 5 分钟。
- 并发：目标数有限（节点级），`errgroup` + 每目标超时即可；单轮全部 commit 在一个 appender 上按目标分段提交。
- 指标保留 `job=...` 覆盖：目标 exposition 里若自带 `job`/`instance` 标签，按 Prometheus honor_labels=false
  约定**重命名**为 `exported_job`/`exported_instance`——SeaTunnel 指标基本不带，但逻辑照标准写。

## 5. 进程内生产者

- `stx_job_*`：`job_prometheus.go` 的 Registry 保留为进程内累加器（`setCounterAbs` 逻辑照用），但
  **`/api/v1/monitoring/prometheus/jobs/metrics` HTTP 端点删除**（无鉴权公开 = 信息泄露面，用户已确认）；
  promhttp 引用随之移除。新增 gather→appender 协程，按 scrape_interval `reg.Gather()` 得到 MetricFamily →
  同上规则展平 + 打 `job="stx_sync_jobs"` append。终态作业序列消失问题：对比上轮 gather 结果，消失的序列写
  stale marker。
- `stx_host_*`（E3，后续任务）：gRPC 上报的结构化主机指标 → 同一 appender 路径（复用 gather 同款展平函数）。
- `up` 对进程内生产者恒为 1（enabled 时）。

## 6. 告警评估器（`internal/apps/monitoring/metric_evaluator.go` 新增）

### 6.0 我们需要的功能 vs AM/他方能力（先盘清楚再选型）

| 需要的能力 | 谁提供 | 现状 |
|---|---|---|
| 周期评估 PromQL（定时跑 expr） | **AM 不提供**——这是 Prometheus `rules` 的活 | 需自建 |
| `for` 持续期 pending 状态机 | `rules` ✓ / AM ✓ | 需自建或借 `rules` |
| fingerprint 去重 | AM nflog | **已有**（投递记录 dedupe） |
| 静默 silence | AM | **已有**（自有语义，UI 驱动） |
| 冷却重发 / reminder / 恢复 | AM repeat/send_resolved | **已有**（cooldown 投递链） |
| 分组打包 / 路由树 / 抑制 / memberlist HA | AM | **不需要**（按策略逐条投递，单一路径，单实例） |
| 通知到渠道 | AM notify | **已有**（email/IM/webhook 渠道系统） |

结论：**嵌 AM 没有收益**——它不做评估，独特功能全是我们已有或不需要的，还白搭 memberlist+自有 silence 两套语义。
n9e（Nightingale）验证了同形态架构（进程内引擎周期跑 PromQL + 自有 dedup/silence/notify），但它是应用不是库，无法 import。

### 6.1 选型：优先 B（嵌 `prometheus/rules`），A（自研评估器）兜底

**B（首选）**：`rules.NewManager` + 从策略 DB 合成 `rulefmt.RuleGroup`（`alert/expr/for/labels/annotations`），
策略 CRUD 时重建 group 并 `manager.Update`；Manager 自带 eval 调度、`for`/`keep_firing_for` pending、
resolved 判定、**重启后 firing 状态恢复**（Prom 语义对等）；`NotifyFunc` 收到 `*Alert` 后映射进
`alertMessage` → `dispatchAlertPolicyChannels`。
依赖账：`rules` 拉 `config`/`notifier`/`discovery`（17 包）——但 `tsdb` 已付过同样链路，**边际 ≈ 0**。
11 条默认告警同样转成一个内置 `rulefmt` 组（`seatunnel:` 录制引用展开内联）。
**退回条件**：策略→rulefmt 适配器超过 ~300 行或与 episode 语义打架，退回 A。

**A（兜底）**：`eval_interval` 定时器 + `buildMetricTemplatePromQL` → `Instant` → 算子/阈值判定 +
`fingerprint→firstMatchedAt` pending map 实现 `for`；命中走同一投递链。重启清 pending（等同 Prom 重启行为）。
~150-200 行，完全可控。

- `platform_health` / `sync_job`：不动（本就进程内）。
- `custom_promql`：全删（见 §8）。

## 7. 配置变更

```yaml
observability:
  enabled: true
  storage:
    dir: "data/tsdb"          # 新增
    retention: "31d"          # 新增（原 prom 15d 默认）
    max_size: 32212254720     # 新增，缺省 30GiB；显式 0=不限
  scrape_interval: "15s"      # 新增
  eval_interval: "15s"        # 新增
  query_timeout: "8s"         # 新增
  max_samples: 50000000       # 新增
  seatunnel_metrics:          # 保留
    path: "/hazelcast/rest/instance/metrics"
    probe_timeout_seconds: 2
  remote_ingest:
    path: "/api/v1/monitoring/alertmanager/webhook"  # P3 由 alertmanager.webhook_path 迁入（破坏性变更，无需双读）
    token: ""                 # 新增：非空则 webhook 校验 Authorization: Bearer（常量时间比较）
  # 移除：prometheus.url / prometheus.http_sd_path / alertmanager.{url,webhook_path} / grafana.url
  # enabled 维持现状默认 true（内置功能默认开），开关保留为资源受限逃生口
```

remote ingest 鉴权语义：token 非空 → `AlertmanagerWebhook` 先校验 `Authorization: Bearer <token>`（`crypto/subtle`
常量时间比较），失败 401；token 为空 → 维持现状兼容已接入的外部 AM，但启动时 WARN 一条
「remote ingest webhook 未配置鉴权」。

`config.example.yaml` + `config.e2e*.yaml` 同步；`config.Config` 校验更新。

## 8. custom_promql 移除面

- 后端：`AlertPolicyBuilderKindCustomPromQL` 常量删除→连带 `case` 分支（校验、rule 生成、窗口解析）、
  `resolveCustomPromQLCapability`、`CapabilityKeyCustomPromQL` 能力卡、`policy_center` builder 条目。
- 前端：`MonitoringPolicyCenter.tsx` 构建器入口与表单分支、types.ts、i18n。
- 迁移：`policy_type='custom_promql'` 行删除（gorm migration/启动时一次性 SQL）；关联 episode/投递记录
  按既有外键约定处理（policy_id 悬空即视为历史——检查实体是否有 FK 约束，若 cascade 存在则先摘约束）。

## 9. 删除清单（按文件）

- `internal/apps/monitoring/managed_alert_policy_sync.go` 整文件退役（rule 写盘/promtool/reload/AM 配置/SIGHUP）。
- `remote_integration.go`：`BuildPrometheusSDTargets` / `buildSTXSyncJobsSDTarget` / HTTP SD handler 删；
  `HandleAlertmanagerWebhook`（remote ingest）保留 + 入口加 `remote_ingest.token` 校验。
- `jobs/metrics`：`internal/apps/sync/job_prometheus.go` 里的 `ServeJobPrometheusMetrics` / `jobPrometheusMetricsPath`
  与路由、handler、operation registry 条目全删；Registry 与 `setCounterAbs` 保留转进程内 gather 用。
- `service.go`：`listPrometheusActiveTargets`、`decorateManagedMetricsTargetsFromPrometheus`、三个组件健康检查
  → 改为 `embedded_tsdb` + `seatunnel_metrics`（读 scraper 状态）+ `remote_ingest`（API 在线即可）。
- `handler.go`：grafana 代理、`GetPrometheusDiscovery` 删；`AlertmanagerWebhook` 留。
- 路由：`prometheus/discovery`、`jobs/metrics` 删；`alertmanager/webhook` 留（+token）；operation registry 基线更新。
- 剩余公开无鉴权端点审计：`alertmanager/webhook` 是唯一对外端点（配 token 后才真正安全，文档写明）。
- `install/`：`package-release.sh` 的 prom/AM 下载、`prometheus.yml.tpl`、`rules/`、systemd/docker 编排；
  `deploy/docker/observability/` 两个服务与卷。
- `docs/observability-remote-*` 三篇 → 更新为内嵌架构说明。

## 10. 风险与权衡（记录在案）

1. **go.mod 依赖膨胀**（scratch 探针 +294 模块；P1 落地实测 810→953，**+143**）：`tsdb`→`config`→discovery
   providers 的 client-go/aws/azure 死代码。应对：锁版本 + 升级清单（重新跑依赖 diff）+ licenses 合规清单同步。
   P1 实测二进制 75.07MB→83.96MB（**+8.9MB**，低于探针估算的 ~+25MB）。
   上游 `remove_all_sd` 落地后可再减。
2. **Go API 无稳定承诺**（上游官方声明）：tsdb/promql 是生态最被依赖的包（Mimir/Thanos/Alloy），破坏性变更频率
   低于措辞暗示；应对：升级 prometheus 依赖时跑「probe 清单」：tsdb.Open 签名、EngineOpts、Matrix/Vector 结构。
3. **单进程资源耦合**：head compaction/WAL fsync 与 API 同进程。规模小（万级序列）风险低；`max_samples`/
   `query_timeout` 保护查询面；文档注明大集群观察项。
4. **双写/迁移期**：切换当刻旧数据在 `deps/prometheus-*/data`（见 §2 迁移指引），不自动搬移。
5. **Windows**：tsdb 支持 Windows（`NoLockedfile`），但 STX server 主要 Linux——CI 构建验证即可。

## 11. 实施阶段（= implement.md 骨架，每阶段独立可验收）

- **P1 存储+查询**：embedts 包 + promquery 适配 + 配置迁移 + diagnostics 切换。此时可用「空 tsdb」起服务
  （无抓取也能过编译与查询路径验收）。
- **P2 抓取+生产者**：scraper + up 合成 + stx_job_* gather→append + integration status 改版 + HTTP SD 删除。
  此阶段大盘应该就有数据。
- **P3 告警评估+退役**：MetricPolicyEvaluator + 默认告警移植 + managed_* 删除 + AM/二进制移除 + custom_promql 删除。
- **P4 安装/文档/收尾**：安装器、docker、config 模板、docs、licenses、E2E。

建议 P1–P4 各一个 PR，依赖顺序写死在 implement.md。
