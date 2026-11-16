# P4 设计：内嵌可观测补完 + 三件套退役 + 首页大盘对齐

需求见 `prd.md`；内嵌架构见 `11-16-embedded-observability/design.md`。

## 1. 作业终态数据（A1 / A2）

### 1.1 现状

```
reconcile ──► observeJobMetrics(instance, metrics)
                 ├─ 非终态：setCounterAbs / gauge 更新 Registry
                 └─ 终态：deleteJob → DeletePartialMatch（终态这次的 metrics 被丢弃）
exporter（每 scrape_interval）──► Registry.Gather → 写 tsdb → tracker.Diff → 消失序列写 stale
```

终态对账带来的最终累计值没写进 Registry；上一轮导出之后的增量也随序列删除丢失。

### 1.2 改法（`internal/apps/sync/job_prometheus.go`、`job_metrics_exporter.go`）

- `observeJobMetrics` 拆成 `applyJobMetrics`（写计数器 / gauge）+ 终态分支：终态时先 `applyJobMetrics`，`stx_job_up` 写当前终态，再 `scheduleDelete`。
- `jobPrometheusExporter` 新增：
  - `deferDeletes bool`：导出协程启动时置 true（`StartJobMetricsExporter`）。
  - `pendingDeletes map[jobKey]prometheus.Labels`。
  - `deleteJob`：`deferDeletes` 为 false 时维持立即删除；为 true 时只登记。
  - `takePendingDeletes()`：取走并清空当前登记。
- 导出协程 `exportOnce`：**先** `takePendingDeletes()` 取快照，**再** `Gather` 写入，提交后对快照执行真正的 `DeletePartialMatch`。
  快照之后才登记的作业留到下一轮，保证每个终态作业的最终值至少被写入一次；下一轮 `tracker.Diff` 给它们写 stale marker。
- `job_engine_sync.go:173` 的直接 `deleteJob` 调用走同一延迟逻辑（不改调用方）。

### 1.3 新作业零值起点（A2）

问题：短作业只被看到一两次，序列第一个样本就是最终累计值 N，`rate` / `increase` 需要两个样本之间的差值，结果为空。

- `jobPrometheusExporter` 记录 `zeroAt map[job_id]int64`：作业第一次被观察时，若 `StartedAt` 非空且距今 ≤ 10 分钟，记 `zeroAt = StartedAt(ms)`。
  `deleteJob` 真正执行时清掉。
- 导出时，对**本来源首次出现**（`SeriesTracker` 新增 `Seen(lset)`）且名称以 `_total` 结尾、`job_id` 在 `zeroAt` 中的序列，
  先在 `min(zeroAt, tsMs-1)` 追加一个 0 样本。追加失败（例如超出 head 可写下界）只忽略该 0 样本，不影响本轮。
- 不满足 10 分钟条件（STX 重启后接手的长作业）不补：否则重启前的全部累计量会被算进重启后的第一个窗口。
- 10 分钟远小于 head 的可写下界（约 1 小时），0 样本不会被 tsdb 判为越界。

## 2. 查询并发上限（A3，`internal/pkg/embedts/embedts.go`）

- `promql.EngineOpts.ActiveQueryTracker = promql.NewActiveQueryTracker(opts.Dir, maxConcurrent, logger)`，
  `maxConcurrent = max(4, runtime.GOMAXPROCS(0))`。跟踪文件 `queries.active` 放在 tsdb 目录（与 Prometheus 同做法），关闭时随 `Close` 释放。
- 覆盖 `promquery`（大盘 / 趋势 / 诊断）与 `rules.Manager`（告警评估）全部查询；超过上限的查询在引擎内排队，受 `query_timeout` 约束。
- 不新增配置项（规模小，常量够用；需要时再开放）。

## 3. 首页大盘缓存（C3，`internal/apps/dashboard/prom_cache.go`）

- 包住集群 / 作业两个服务对 `promquery.Range/Instant` 的调用。
- 键：`kind | expr | start | end | step`。预设窗口的 `end` 先按 15 秒向下取整再算 `start`；自定义窗口本来就固定。
- 值：结果与错误；成功 TTL 15 秒，错误 5 秒。`golang.org/x/sync/singleflight` 合并同键并发。最多 256 键，超限整体清空。
- 只缓存 PromQL 结果；DB 部分（卡片、节点表、作业名、`viewable`、日历、标注）每次照常计算，逐用户字段不串号。
- 与 §2 的并发上限配合：缓存去掉重复计算，上限兜住不同参数的并发。

## 4. 三件套退役（B1–B5）

### 4.1 Go / 配置

| 位置 | 处理 |
|---|---|
| `monitoring/handler.go` `ProxyGrafana`、`isGrafanaLiveWSRequest`、只为它服务的 `parseProxyTarget` | 删除 |
| `router/router.go` `/monitoring/proxy/grafana` 路由组；`router/middlewares.go` `isGrafana*Path` | 删除 |
| `config/model.go` `ObservabilityGrafanaConfig`；`config.go` 环境变量覆盖、默认值、校验 | 删除；测试同步 |
| `config.example.yaml`、`config.e2e*.yaml`、`deploy/docker/compose.base.yml` 与 `.env*` 中的 grafana / prometheus / alertmanager 项 | 删除 |
| 路由基线、swagger | 重新生成 |

兼容：配置加载 `viper.Unmarshal`（非 Exact），旧段被忽略；加载测试覆盖含三段旧配置的 yaml。

### 4.2 安装 / 打包 / docker / CI / 脚本

原则：**删除下载、渲染、启停、端口与清单；不删用户磁盘上的旧目录**。

- `install/observability/`（`render.sh`、三个 `*_config/`）整目录删除；`install/bin/lib/observability.sh` 删除，`start.sh` / `stop.sh` / `status.sh` 去掉对它的 source 与三件套启停。
- `install/install-core.sh` / `install.sh` / `install-online.sh` / `build-offline-bundle.sh` / `package-release.sh`：
  - 去掉三件套二进制下载 / 校验 / 解包、`STX_PROMETHEUS_PORT` 等端口变量、`--without-observability` 中与三件套相关的分支
    （开关保留语义改为只写 `observability.enabled=false`）。
  - 安装完成摘要的「可观测性」一行改为内嵌说明：数据目录 `data/tsdb`、保留 31 天、上限 30GiB。
  - 升级检测到旧 `observability/` 或 `deps/prometheus-*` 时输出一行：「内嵌可观测已替代外部 Prometheus / Alertmanager / Grafana，旧目录保留在 …，确认不需要后可手动删除」。
    旧 systemd / 进程：升级脚本若曾为它们注册服务，停止并禁用（不删数据）。
- `deploy/docker/observability/` 整目录删除；compose 去掉 prometheus / alertmanager / grafana 服务、卷、依赖与 `STX_OBSERVABILITY_*_URL` 环境变量；tsdb 数据随 STX 数据卷持久化。
- `.github/workflows/release-deps.yml`、`ci-main.yml`：去掉三件套依赖产物的构建 / 下载 / 测试步骤。
- `scripts/`：`manage-deps-obs.sh`、`observability-remote-smoke.sh` 删除；`merge-deps-manifest.py`、`test-release-deps.sh`、
  `test-release-download-lib.sh`、`test-offline-bundle.sh`、`smoke-offline-bundle.sh`、`observability-enabled-switch-regression.sh` 去掉三件套断言，
  必要时改为断言「产物中不含三件套」。
- 逐文件改之前先读清楚依赖关系（`deps` 清单格式、下载库函数被谁调用），每改一组跑对应自测脚本。

### 4.3 文档与 CLI

- `docs/observability-remote-design.md`、`observability-remote-integration-guide.md`、`observability-remote-mvp-plan.md`、`可观测性三件套一键接入说明.md`：
  合并改写为一篇 `docs/内嵌可观测说明.md`（架构、配置项、保留期 / 上限、抓取 / 评估、remote ingest 鉴权、升级说明），旧四篇删除，引用处改链接。
- `一键安装与部署设计.md`、`打包发布说明.md`、`告警中心触发-恢复-处理-通知设计说明.md`：删去三件套段落，改为内嵌描述。
- AGENTS.md / README：端口与三件套描述同步。
- CLI 由 operation 注册表生成：路由删除后重新跑 contract-report；检查 `stx monitoring integration status` 输出组件名。

## 5. 前端（C1 / C2）

- 删除 `components/common/monitoring/MonitoringOverview.tsx`（无引用；`ClusterDetail` 只用同名类型）及只被它用的 i18n 键。
- `TrendEmptyState` 增加 `reason`：`observability_disabled` →「可观测性未开启」（提示开启 `observability.enabled`）；
  其余 →「监控数据暂不可用」（内嵌存储未就绪或查询失败，稍后自动重试）。集群 / 作业页签、预览抽屉透传接口 `reason`。
  i18n 键名 `noProm*` 保留，只改文案并新增 `unavailable*`。
- 作业详情 `MetricsTrendView`、`AutoPolicyConfigPanel` 等处若有「Prometheus」用户可见文案，一并改为内嵌口径。

### 5.1 用户可见文案去实现名（C5）

盘点（2026-11-16）：

| 位置 | 现状 | 改为 |
|---|---|---|
| 前端 i18n（zh / en 各约 21 条） | 「接入 Prometheus」「远程 Alertmanager」「Prometheus 指标」「PromQL 预览」「来自 Prometheus」、Grafana 文案 | 「开启可观测性」「外部告警」「监控指标」、删 PromQL 预览、删 Grafana 键 |
| `policy_center_service.go` 能力卡 / 构建器说明 | 「even without Prometheus」「Ingest Alertmanager webhook」「Prometheus-backed signals」 | 「内置监控指标」「接入外部告警（兼容 Alertmanager Webhook 格式）」 |
| 能力卡 `reason` / `depends_on` 展示 | 组件名 `embedded_tsdb` 等直接显示 | 前端映射为「内置监控存储」「引擎指标抓取」「外部告警接入」 |
| `monitoring/handler.go` `error_msg` | 「Failed to process alertmanager webhook」 | 「处理外部告警回调失败」（中英） |
| `operation/registry*.go` 命令描述 | 「in one Prometheus query」「Query Prometheus timeseries」「Alertmanager webhook receiver」 | 「监控指标」措辞 |
| 诊断报告（`task_execute.go` 模板与条目） | 「· Prometheus」「未采集到 Prometheus 指标快照」 | 「· 监控指标」「未采集到监控指标快照」 |
| 安装 / 升级输出 | 三件套端口与数据目录说明 | 内嵌监控一行说明（见 §4.2） |

规则：
- 兼容性枚举值保持不变，前端按值映射显示文案（如 `remote_alertmanager` →「外部告警」）。
- 策略中心「PromQL 预览」字段与相关 i18n 删除；后端若仍返回该字段，前端不再渲染（接口字段保留，CLI 可见）。
- 外部告警接入的对接说明（文档、能力卡详情）保留「兼容 Alertmanager Webhook 格式」。
- SeaTunnel 引擎配置键 `metrics.prometheus.enabled` 原样显示。
- 不改代码标识符、注释、服务端日志。

## 6. spec

- `frontend/ui-conventions.md` §16：空态按 `reason` 分文案。
- `backend/job-status-reconciler.md` 首页大盘一节：「与 HTTP SD 一致」→「与内嵌抓取目标标签一致」；「外部 Prometheus 可能没加载录制规则」→
  「大盘一律写原始表达式，不依赖告警评估器物化的 `seatunnel:*` 录制序列」；补大盘缓存、终态数据与零值起点约定。
- 其他 backend spec 中描述外部 Prometheus / HTTP SD / Alertmanager 的段落同步。

## 7. 验证

- 单测：延迟删除（终态值被导出一次后才删、未启动导出协程时立即删）、零值起点（10 分钟内补 / 超出不补 / 只补计数器）、
  并发上限（超限查询排队）、大盘缓存（singleflight、TTL、错误缓存、`-race`）、配置旧段兼容。
- 本地：`~/stx-local` 用本分支二进制起服务（内嵌，observability 开启），跑 5 秒 FakeSource 批作业核对 A1/A2；
  与 t1 共享环境的外部 Prometheus（9090）抽样对比吞吐 / 堆 / Top 8。
- 打包：`scripts/test-offline-bundle.sh`、`test-release-deps.sh` 等在新产物上通过。

## 8. 提交与回退

单 PR（`feat/embedded-observability-p4`，base `feat/embedded-observability-p3b`），按块分提交，便于单独 revert：

1. A1–A3 内嵌不足修复；2. 大盘缓存 + 空态文案；3. Grafana 应用层 + 配置；4. 用户可见文案去实现名；5. 安装 / docker / CI / 脚本；6. 文档与 spec。

合并顺序：#137 → … → #143 → 本 PR。内嵌 P1 起查询切到 tsdb，栈内各层需连续合并、中间不发版。
