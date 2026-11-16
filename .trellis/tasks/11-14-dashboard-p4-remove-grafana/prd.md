# P4 收尾：内嵌可观测补完 + 三件套退役 + 首页大盘对齐

父任务：`11-14-home-monitoring-dashboard`。2026-11-16 两次重定义：先因「内嵌可观测」任务（`11-16-embedded-observability`）缩成
「Grafana 应用层摘除 + 内嵌口径对齐」；随后用户决定（2026-11-16）**由本 P4 给当前所有 PR 收尾**，并入内嵌任务 P4 未完成的逻辑与内嵌 P1–P3 的不足。

## 背景

当前叠加的 PR：#137（大盘 P1）→ #138（P2）→ #139（P3）→ #140（内嵌 P1）→ #141（内嵌 P2）→ #142（内嵌 P3a）→ #143（内嵌 P3b）。
本任务是叠加栈的最后一层（分支 `feat/embedded-observability-p4`，base = `feat/embedded-observability-p3b`）。

内嵌 P1–P3b 已完成：进程内 tsdb + promql、自研抓取、`stx_job_*` 直写、HTTP SD / `jobs/metrics` 删除、integration status 改版、
custom_promql 删除、webhook Bearer 鉴权、`rules.Manager` 告警评估、`prometheus.url` / `alertmanager.url` 配置删除。

未完成或有缺陷的部分（2026-11-16 复核）：

- 安装器、离线包、docker、CI、发布脚本仍下载 / 渲染 / 启停 Prometheus、Alertmanager、Grafana（内嵌 implement.md P3 移到 P4 的项与原 P4 全部未做）。
- `observability.grafana` 配置、Grafana 反向代理、日志降噪中间件、前端 `MonitoringOverview.tsx`（已无引用）仍在。
- 文档（`docs/observability-remote-*`、`可观测性三件套一键接入说明`、安装部署 / 打包发布 / 告警中心说明）仍描述外部三件套。
- **作业终态的数据丢失**：`observeJobMetrics` 遇到终态直接 `deleteJob`，终态那次对账带来的最终累计值从未写入；
  且下一轮导出前序列已删，最后一个导出周期内的增量也丢。只被看到一两次的短批作业在吞吐 / Top N / 读写量里完全缺席
  （只有一个样本时 `rate` / `increase` 算不出值）。
- **引擎查询无并发上限**：`promql.NewEngine` 未配 `ActiveQueryTracker`；大盘、作业趋势、告警评估共用引擎，多人打开首页时成倍占用 STX CPU。
- 大盘文案仍写「接入 Prometheus」；空态不区分「未开启」与「存储不可用」。

## Requirements

### A. 内嵌可观测不足修复

- A1 作业终态：先把终态对账的指标写入（与运行中同一套 `setCounterAbs` / gauge 逻辑），序列延迟到**下一轮导出写入之后**再删除；
  没有导出协程运行时（可观测关闭 / tsdb 未就绪）维持立即删除，避免累积。
- A2 新作业零值起点：STX 首次看到的作业若开始时间在近 10 分钟内，其累计计数器序列首次写入前在开始时刻补一个 0 样本，
  让 `rate` / `increase` 能算出短作业的量；开始时间更早（STX 重启后接手的长作业）不补，避免把重启前的累计量算进当前窗口。
- A3 查询并发上限：内嵌引擎配置 `ActiveQueryTracker`，并发上限取 `max(4, GOMAXPROCS)`，覆盖大盘 / 趋势 / 告警评估全部查询。

### B. 三件套退役（内嵌 P4 未完成部分）

- B1 安装器（在线 / 离线 / 打包 / 状态脚本）、`install/observability/`、`deploy/docker/observability/` 与 compose、`.github/workflows`、
  `scripts/` 中 Prometheus / Alertmanager / Grafana 的下载、渲染、启停、端口、清单全部删除；安装后的 `observability.enabled` 只表示内嵌功能开关。
- B2 升级路径：已安装三件套的环境升级后不再拉起这些进程；旧 `deps/prometheus-*` 等目录与数据保留不删，安装 / 升级输出给出一行说明。
- B3 `observability.grafana` 配置结构、默认值、校验、环境变量、example / e2e 配置删除；Grafana 代理、路由、中间件分支删除。
  旧配置文件残留 `grafana:` / `prometheus:` / `alertmanager:` 段时仍能启动（`viper.Unmarshal` 非 Exact，补测试锁住）。
- B4 文档：`docs/` 中描述外部三件套的文档改写为内嵌架构（数据目录、保留期、存储上限、抓取 / 评估间隔、remote ingest 鉴权、升级说明）；
  AGENTS.md / README 中 9090 / 9093 / 3000 端口与三件套描述同步。
- B5 CLI：受影响命令（integration status、monitoring 相关）输出与新组件集一致；`stx` 帮助中不再出现 Grafana 代理。

### C. 首页大盘对齐

- C1 删除前端 `MonitoringOverview.tsx` 及只被它使用的 i18n 键与测试；`HomeDashboard` 注释去掉 Grafana 过渡说明。
- C2 空态文案按 `reason` 分两套：`observability_disabled` →「可观测性未开启」；`prometheus_unavailable` / `prometheus_error` →「监控数据暂不可用」。
  不再出现「接入 Prometheus」。接口 `source` / `reason` 取值不变。
- C3 首页集群 / 作业大盘的 PromQL 结果加进程内缓存（≤15 秒，singleflight 合并），逐用户字段在缓存外装饰。
- C5 **用户可见文案去实现名**（2026-11-16 用户要求）：监控是产品内置能力，用户不需要知道底层是 Prometheus / Alertmanager / Grafana。
  界面文案（i18n）、后端生成并展示给用户的文字（能力卡、策略模板说明、接口 `error_msg`、诊断报告）、CLI 命令描述、swagger 摘要、
  安装 / 升级输出一律用产品语言（「内置监控」「监控指标」「告警」等）。策略中心的 PromQL 预览不再展示。例外：
  - 兼容性枚举值不改（`source:"prometheus"`、`reason:"prometheus_unavailable"`、`source_type:"remote_alertmanager"`、诊断条件类别 `prometheus` 等），只改它们的显示文案；
  - 外部告警接入（remote ingest）对接说明保留「兼容 Alertmanager Webhook 格式」，对接方需要知道格式；
  - SeaTunnel 引擎自己的配置键（如 `metrics.prometheus.enabled`）照原样显示；
  - 代码标识符、注释、服务端日志不做改名。
- C4 spec：`frontend/ui-conventions.md` §16、`backend/job-status-reconciler.md` 首页大盘一节、观测相关 backend spec 改为内嵌口径。

### D. 验证

- D1 全量 `go test ./...`、`cd agent && go test ./...`、`cd frontend && pnpm test && pnpm exec tsc --noEmit`、改动文件 eslint、`gofmt -l`。
- D2 本地内嵌环境复验父任务跨子任务验收；与外部 Prometheus（t1 共享环境 9090，仍在抓同一批 SeaTunnel 节点）做一次抽样一致性核对。
- D3 打包 / 离线包脚本的自测（`scripts/test-offline-bundle.sh` 等）在不含三件套的产物上通过。

## Acceptance Criteria

- [ ] `grep -ri "grafana\|alertmanager_config\|prometheus_config\|9093"` 在 `install/ deploy/ scripts/ .github/ internal/ frontend/{app,components,lib}` 中只剩 remote ingest（外部 AM 回流）相关命名与变更说明。
- [ ] 旧配置文件含 `observability.grafana` / `prometheus` / `alertmanager` 段时 STX 正常启动。
- [ ] 跑一个 < 15 秒的批作业：全局吞吐、写入 Top N、`increase(stx_job_sink_write_rows_total[1h])` 都能看到它的写入量。
- [ ] 作业终态后序列在下一轮导出后写 stale marker，5 分钟内趋势不再延续。
- [ ] 并发超过上限的查询排队而不是同时执行（单测）；同参数并发请求大盘 15 秒内只算一轮 PromQL。
- [ ] 用户可见文案（zh / en i18n、后端展示文字、CLI 描述、诊断报告、安装输出）中不再出现 Prometheus / Alertmanager / Grafana / PromQL（C5 例外项除外）。
- [ ] 首页三个页签在关闭可观测性、存储不可用、正常三种状态下文案正确（zh / en）。
- [ ] 抽样一致性：同窗口内嵌与外部 Prometheus 的全局写入吞吐、节点堆使用率、写入 Top 8 差异在一个抓取周期的错位范围内（记录数值）。
- [ ] 打包产物与离线包中不含 prometheus / alertmanager / grafana 二进制与配置模板；全部测试通过。

## Out of Scope

- E1–E3 增强（各自子任务）。
- 自动迁移旧 Prometheus 历史数据（内嵌任务已定：不迁移，保留旧目录）。
