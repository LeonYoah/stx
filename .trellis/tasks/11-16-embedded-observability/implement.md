# 内嵌可观测 —— 实施清单

依赖：`design.md` §1–§11。四阶段顺序执行，每阶段一个 PR、独立可验收；后一阶段依赖前一阶段已合并。

## P1 存储 + 查询（地基，可空转验收）✅ 已完成（feat/embedded-observability）

- [x] 新包 `internal/pkg/embedts`：`tsdb.Open(data/tsdb)` + `promql.Engine` 生命周期管理（挂在
      `router.Serve` 的 `observability.enabled` 分支上，打开失败降级 Ready()=false 不 panic）。
- [x] 配置（本阶段**只增不删**）：`observability.storage.{dir,retention(默认31d),max_size}` +
      `scrape_interval`/`eval_interval`/`query_timeout`/`max_samples` 新增（config/model.go、
      config.go 校验、config.example.yaml、4 个 e2e yaml）。
      注意：`prometheus.url`/`alertmanager.url`/`grafana.url`/`http_sd_path` 仍有活消费者
      （service.go 集成状态、managed_alert_policy_sync、grafana 代理、P4 才摘 Grafana），
      它们的删除分别排在 P2（http_sd_path）/ P3（prometheus.url、alertmanager.url）/ P4（grafana.url）。
- [x] `promquery.Range/Instant` 改走 `promql.Engine`；`BaseURL()` 改为 embedts 可用性探测
      （reason 枚举不变，就绪时返回哨兵串 `"embedded"`）。
- [x] `diagnostics/task_execute.go` 自有 HTTP 查询改调 `promquery.Range`，私有 response 结构体已删。
- [x] `go.mod` + prometheus/prometheus v0.316.0（模块 810→953，+143；二进制 +8.9MB）；
      `licenses/` 无 Go 依赖清单可同步（仅前端 MIT 文件）。
- 验证已过：`go build ./...`；`go test ./internal/pkg/embedts ./internal/pkg/promquery
      ./internal/apps/diagnostics ./internal/config ./internal/apps/sync ./internal/apps/dashboard` 全绿；
      `go vet` 干净。起服务后空 tsdb 大盘走降级空态（BaseURL→prometheus_unavailable）。
      注意：`internal/apps/installer` 有预存在的跨设备 rename 环境性失败，与本改动无关（干净 HEAD 可复现）。
- 风险文件：`promquery.go`（所有大盘查询都过它）；`main.go` 停机顺序（先停 API 再 `db.Close()`）。

## P2 抓取 + 生产者（开始有数据）✅ 已完成（feat/embedded-observability-p2）

- [x] `embedts/scraper.go` + `flatten.go`：`MetricsTargetProvider` 接口注入（monitoring 适配
      `collectManagedMetricsTargets`，embedts 不反向依赖 apps）；15s errgroup 并发 GET + expfmt 解析 +
      Summary/Histogram 展平 + `exported_*` 改名 + `WriteTsWatermark` 防同毫秒 duplicate sample。
- [x] `up{job,instance,cluster,...}` 合成 + 目标/序列双级 stale marker + `TargetStates()` 内存状态。
- [x] `job_prometheus.go`：公开端点/handler/路由/promhttp/operation 条目全删；Registry 保留；
      `job_metrics_exporter.go` gather→tsdb 直写 + stale + `up{job="stx_sync_jobs"}=1`。
- [x] `remote_integration.go`：SD 目标构建/handler/路由删；`prometheus.http_sd_path` 配置键删除
      （model/default/validate/example/e2e 全清）。
- [x] `service.go`：`listPrometheusActiveTargets`/decorate 删；集成状态组件 =
      `embedded_tsdb` + `seatunnel_metrics`（含 total/up/down/last_error meta）+ `remote_ingest`；
      capability depends_on 同步改名；前端泛型消费 `components[]` 零改动。
- 验证：起一个真实 SeaTunnel 集群（或 mock exposition server）→ `promquery` 能查到 `up`、
      `jvm_memory_bytes_used` 等序列；断掉节点 → 下个周期 `up=0`；大盘集群页签出图。
- 风险文件：`service.go`（integration status 被策略中心能力卡消费，`resolveMetricsPolicyCapability` 的
  depends_on 要从 prometheus 组件改成 embedded_tsdb）。

## P3 告警评估 + 退役（拆最多东西）

- [ ] `metric_evaluator.go`：eval_interval 轮询启用的 `metrics_template` 策略 → `buildMetricTemplatePromQL`
      → `Instant` → 算子/阈值判定；内存 pending map 实现 `for`；命中走 `dispatchAlertPolicyChannels`。
- [ ] 11 条默认告警表达式移植为内置检查（5 条 `seatunnel:` 录制引用展开内联），命中走 RemoteAlertRecord
      + 通知路径。
- [ ] `managed_alert_policy_sync.go` 整文件删除；`SyncManagedAlertingArtifacts` 的调用点改 evaluator 重载。
- [ ] `custom_promql` 全删：常量 + `case` 分支（model.go 校验、policy_center、managed sync、前端 builder/
      能力卡/types/i18n）；迁移删除存量 `policy_type='custom_promql'` 行（查实体 FK 先摘约束）。
- [ ] `alertmanager/webhook` 加 `remote_ingest.token` Bearer 校验（`crypto/subtle`）+ 未配置 WARN。
- [ ] 安装器：去掉 prom/AM 二进制下载、systemd/docker 单元、`prometheus.yml.tpl`、`rules/`、deps glob；
      `reloadManaged*` / `discoverManagedObservabilityRuntimePaths` 删；`writeManagedAlertmanagerConfig` 删。
- 验证：配一条 metrics_template 策略（cpu>0.8）→ 跑高负载 → 通知到渠道；关窗口期 → 恢复通知；
      `policy_type='custom_promql'` 行在迁移后不存在；webhook 无 token 时 401（配置后）/ 200（未配置）。
- 风险文件：`alert_policy_execution_service.go`（投递语义必须等价：dedupe/cooldown/silence/恢复前置）；
      `MonitoringPolicyCenter.tsx`（custom_promql 前端删除面大）。

## P4 安装 / 文档 / 收尾

- [ ] `deploy/docker/observability/` 删 prometheus + alertmanager 服务与卷；compose 文件更新。
- [ ] `docs/observability-remote-*` 三篇更新为内嵌架构；`docs/全量功能文档与目录规划.md` 相关段同步。
- [ ] AGENTS.md / README 提及 observability 的地方更新（端口 9090/9093 消失）。
- [ ] CLI：`stx` 命令中 integration status / monitoring 相关输出对齐新组件集。
- [ ] E2E：`config.e2e*.yaml` 更新 + `scripts/observability-enabled-switch-regression.sh` 适配（它引用 http_sd）。
- 验证：`go test ./...`、`cd agent && go test ./...`、`cd frontend && pnpm test && pnpm exec tsc --noEmit`；
      全新裸装一次（无 prom/AM 二进制）→ 大盘 + 一条模板告警端到端。

## 全局验证命令

```
go test ./...
cd agent && go test ./...
cd frontend && pnpm test && pnpm exec tsc --noEmit
# 依赖审计（对照 design §10 记录值）
go list -m all | wc -l          # 预期 root 模块 +~290
go list -deps ./... | grep -c k8s.io  # 应等于实测基线（死代码，~14 包）
```

## 回滚点

- P1 完成后如依赖膨胀不可接受：可直接 revert（此时未动告警链路与外部 prom 集成，外部部署仍工作）。
- P3 前所有阶段外部 Prometheus 都可继续共存（外部部署不停服），P3 合并才是真正退役点。