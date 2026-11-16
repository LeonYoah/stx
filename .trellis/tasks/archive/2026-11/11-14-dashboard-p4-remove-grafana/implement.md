# P4 执行计划

分支 `feat/embedded-observability-p4`，base `feat/embedded-observability-p3b`（PR #143）。按块提交，每块独立可 revert。

## 0. 前置

- [ ] `task.py start`；加载 `trellis-before-dev`：backend（error-handling / logging / quality / cli-guidelines / job-status-reconciler）、frontend（ui-conventions / api-and-services）、infra。
- [ ] 基线：`go build ./... && go test ./internal/...` 在 p3b 上的结果记录（区分预存在失败，如 installer 跨设备 rename）。

## 1. 内嵌不足修复（A1–A3）

- [x] `job_prometheus.go`：`applyJobMetrics` 抽出；终态先写指标再 `scheduleDelete`；`deferDeletes` / `pendingDeletes` / `takePendingDeletes`；`zeroAt` 记录。
- [x] `job_metrics_exporter.go`：先取待删快照 → gather 写入（新计数器序列补 0 样本）→ 提交后执行删除；启动时开启延迟删除。
- [x] `embedts/flatten.go`：`SeriesTracker.Seen`。
- [x] `embedts/embedts.go`：`ActiveQueryTracker`（`max(4, GOMAXPROCS)`）。
- [x] 单测：延迟删除两种模式、零值起点三种情形、并发上限排队。`go test ./internal/apps/sync/... ./internal/pkg/embedts/...`。
- [x] 提交：`fix(observability): 作业终态终值写入、新作业零值起点与引擎查询并发上限`

## 2. 大盘缓存 + 空态文案（C2 / C3）

- [x] `dashboard/prom_cache.go` + 集群 / 作业服务接入；单测（singleflight、TTL、错误 5 秒、`-race`）。
- [x] `TrendEmptyState` 按 `reason`；集群 / 作业页签、预览抽屉、作业详情趋势透传；zh / en。
- [x] 前端单测；提交：`feat(dashboard): 大盘查询短时缓存与按原因区分的空态`

## 3. Grafana 应用层 + 配置（B3 / C1）

- [x] 代理 handler / 路由 / 中间件；`observability.grafana` 配置全链路；example / e2e / compose / `.env*`。
- [x] 配置加载测试：含 `grafana` / `prometheus` / `alertmanager` 旧段的 yaml 正常加载。
- [x] 删除 `MonitoringOverview.tsx` 与专用 i18n；`HomeDashboard` 注释。
- [x] contract-report 基线、swagger 重新生成。
- [x] 提交：`refactor(observability): 移除 Grafana 代理、配置与前端残留`

## 4. 用户可见文案去实现名（C5）

- [x] 前端 i18n 逐条改（见 design §5.1）；删除策略中心 PromQL 预览渲染；`remote_alertmanager` 等枚举显示映射。
- [x] 后端：能力卡 / 构建器说明、handler `error_msg`、operation 命令描述、诊断报告模板与条目。
- [x] `grep -rniE "prometheus|alertmanager|grafana|promql"` 核对 i18n、`error_msg`、operation 描述、诊断模板，只剩例外项。
- [x] 前后端测试；提交：`refactor(ui): 用户可见文案不再暴露监控底层实现名`

## 5. 安装 / docker / CI / 脚本（B1 / B2）

- [x] 读清楚依赖：`install/bin/lib/observability.sh` 的调用方、deps 清单格式、`test-release-download-lib.sh` 被谁 source。
- [x] 删除 `install/observability/`、`install/bin/lib/observability.sh`、`deploy/docker/observability/`、`scripts/manage-deps-obs.sh`、`scripts/observability-remote-smoke.sh`。
- [x] 改 `install-core.sh` / `install.sh` / `install-online.sh` / `build-offline-bundle.sh` / `package-release.sh` / `bin/{start,stop,status}.sh`；升级提示旧目录保留。
- [x] 改 compose、`.env*`、workflows、其余 scripts 的断言。
- [x] `bash -n` 全部改动脚本；`shellcheck`（如可用）；跑 `scripts/test-offline-bundle.sh`、`test-release-deps.sh`、`test-release-download-lib.sh`、`observability-enabled-switch-regression.sh`（能本地跑的）。
- [x] 提交：`refactor(install): 安装包、离线包与 docker 部署移除外部监控三件套`

## 6. 文档与 spec（B4 / C4）

- [x] 新 `docs/内置监控说明.md`；删旧四篇，修引用；安装部署 / 打包发布 / 告警中心说明改写；AGENTS.md / README。
- [x] spec：ui-conventions §16、job-status-reconciler 首页大盘一节、其他观测相关 spec。
- [x] 内嵌任务 implement.md P4 段标注「并入 dashboard P4」。
- [x] 提交：`docs: 内嵌可观测说明与规范同步`

## 7. 验证（D1–D3）

- [x] `go test ./...`、`cd agent && go test ./...`、`cd frontend && pnpm test && pnpm exec tsc --noEmit`、改动文件 eslint、`gofmt -l`、`go vet ./...`。
- [x] 本地 `~/stx-local`：本分支二进制（nice 19 构建）起服务；首页三页签出图；5 秒 FakeSource 批作业核对吞吐 / Top N / `increase`；
      作业终态后 stale；关闭可观测性与存储不可用两种空态；zh / en。
- [x] 抽样一致性：同窗口内嵌 vs t1 外部 9090（吞吐 / 节点堆 / Top 8），数值记入下方实施记录。
- [x] 推送、开 PR（base `feat/embedded-observability-p3b`）、链接 T3 线程；父任务跨子任务验收勾选。
      PR #144：https://github.com/LeonYoah/stx/pull/144（栈 #137–#144 全部链接至 T3 线程）。

## 实施记录

**2026-11-16 验证结果**（dev 实例 `:27800`，t1 共享环境同一台 SeaTunnel 引擎 `10.66.0.2:5801`）：

- 套件：`go vet` 干净；`gofmt` 补了本分支引入的两处；`go test ./...` 仅 `internal/apps/installer` 两个预存在环境性失败
  （跨设备 rename、`/tmp` root 属主目录，包未被本分支触碰）；`cd agent && go mod tidy` 修复根模块 P1–P3b 升版本造成的
  go.mod 失同步（本层顺手修），agent 测试仅 `TestEnsureSTXJavaProxySupportAssetsDownloadsFromAssetBase` 环境性失败
  （本机 `/opt/stx` 存在，测试隔离缺陷，预存在）；前端 tsc + 618 测试全过，改动文件 eslint 只有 MetricsTrendView 5 个
  预存在 curly 报错（base 上相同行）。
- B3 兼容：dev `config.yaml` 仍带旧 `prometheus` / `alertmanager` / `grafana` 段与 `http_sd_path`，新二进制正常启动。
- 内嵌启动：tsdb（`./data/tsdb` 31d / 30GiB）、抓取循环 15s/30s、告警评估 15s、`stx_job_*` 直写协程、`queries.active`
  并发上限文件全部就位；`remote_ingest.token` 空时启动 WARN 提示公开端点。
- A1/A2 实跑：经 dev stx 提交 `alert-smoke-batch`（FakeSource→Console，row.num=20，运行约 3 秒）→ 终态 `success`；
  `timeseries` 返回零值起点样本 + 终值 20 行，`rate` 算出 ~0.5/s，作业进入大盘写入 Top N 与数据流向；
  终态后序列在下一轮导出后不再延续（最后点 11:42:07 后无新样本）。
- 空态：`enabled=false` 重启后集群 / 作业大盘都返回 `source:"none" reason:"observability_disabled"`；恢复后正常。
- 抽样一致性（同 15s 窗口错位内）：外部 Prom `jvm_memory_bytes_used{heap}` ≈ 143.2MB（13.3% of 1GiB max），
  内嵌大盘同节点 heap ≈ 12.9%；`availability=1`、CPU ≈ 0.3%、FD 0.93%、`clusters_safe 1/1` 两侧一致。
- 备注：`prometheus_unavailable` 空态未实跑（需 tsdb 故障），由 C2 前端单测与接口 reason 契约覆盖；
  作业级一致性不可比（外部 Prom 抓的是旧 stx 自己的 jobs/metrics，两个实例作业集不同）。

## 回退

- 每块一个提交，可单独 revert；块 5（安装）回退会恢复三件套安装物，但后端已不再使用它们，只作代码回退。
