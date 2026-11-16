# 内嵌可观测：进程内 tsdb + promql，自研抓取与告警评估

独立任务（不挂在 home-monitoring-dashboard 下）。目标：把 STX 与外部 Prometheus / Alertmanager 的进程间集成，
换成**内嵌单模式**——STX 进程内跑存储与查询，抓取与告警评估自研，退役两个外部二进制。

## Locked Decisions（2026-11-16 用户确认）

1. **单模式**：内嵌是唯一形态。删掉「配置外部 `prometheus.url` 走远程」的双模式分支；HTTP SD 端点、AM webhook
   回流、`/-/reload`、promtool 校验这套对外集成面整体退役。
2. **不考虑 `AlertPolicyBuilderKindCustomPromQL`**：监控面板与告警全部由产品内置定义，不开放用户自定义。
   用户侧不再出现自定义 PromQL 输入。**存量策略行：功能与 UI 代码全部移除（含后端 builder / 校验 / 规则生成分支 /
   前端构建器与能力卡），存量 `policy_type='custom_promql'` 数据行由迁移删除。**
3. **内嵌 tsdb 保留期**：时间保留默认 **31 天可配**（覆盖 E2 月度视图），另支持**按存储大小**的可配上限
   （tsdb `MaxBytes`）。
4. **`jobs/metrics` 暴露端点：删除**。该端点当前是**无鉴权公开只读**（任何可达 STX API 端口的人都能拉到
   `stx_job_*` 作业/表名/读写量），不值得为假设性外部需求保留。`stx_job_*` / `stx_host_*` 只进内嵌 tsdb。
5. **`alertmanager/webhook`（remote ingest）保留 + 加鉴权**：新增 `observability.remote_ingest.token`，
   配置后校验 `Authorization: Bearer <token>`（常量时间比较）；未配置维持现状但启动时记一条 WARN
   （兼容已接入的外部 AM）。
6. **内置默认开（2026-11-16 用户确认）**：`observability.enabled` 维持现状默认 true（未配置即开），
   开关保留为资源受限场景的逃生口——空态降级链路已建成，保留零成本。三件套（prometheus/alertmanager/grafana）
   的进程、端口、安装、文档随本任务全量移除。
7. **升级 = 破坏性变更（2026-11-16 用户确认）**：不做兼容迁移——时序历史重新采集即可；
   旧 prometheus/alertmanager 数据目录**保留在磁盘上不删**（只是不再拉起进程）；
   DB 侧唯一迁移是删除 `policy_type='custom_promql'` 行；release note 记「监控历史不保留」。
8. **`prometheus:` / `alertmanager:` / `grafana:` 配置段终态全删（2026-11-16 用户确认）**：终态配置只剩
   `enabled` / `storage` / `scrape_interval` / `eval_interval` / `query_timeout` / `max_samples` /
   `seatunnel_metrics` / `remote_ingest{path,token}`（P3 时 `alertmanager.webhook_path` 迁入
   `remote_ingest.path`）。`storage.max_size` 缺省 **30GiB**，显式写 0 表示不限。

## 已确认事实（代码复核）

### 对外依赖面（嵌入前要消化的全部触点）

- 查询：`internal/pkg/promquery`（`BaseURL()` + `Range`/`Instant`，仅 5 个调用文件 ~57 处）；
  `internal/apps/diagnostics/task_execute.go` 另有自己的 HTTP 查询实现（diagnostic snapshot）。
- 健康/探测：`service.go` 的 `GetIntegrationStatus`（prometheus `/-/healthy` + `/targets`、alertmanager `/-/healthy`、
  grafana `/api/health`、seatunnel_metrics 探活）与 `listPrometheusActiveTargets`（`api/v1/targets?state=active`）。
- 告警链路：`managed_alert_policy_sync.go` 把 `metrics_template` / `custom_promql` 策略落成
  `deps/prometheus-*/rules/stx-managed-alert-policies.yml` → promtool check → POST `/-/reload` → 写
  `alertmanager.yml`（单路由 webhook 回 `GetExternalURL()+webhook_path`）→ 对 AM pid 发 SIGHUP。
  AM 分组后发 webhook → `HandleAlertmanagerWebhook` 写 remote alert → `deliverManagedRemoteAlertPolicyNotifications` 投递。
- Grafana 跳转：`handler.go:519` 的 grafana 代理（P4 已在移除前端依赖）。
- 安装侧：裸装 `deps/prometheus-*` + `deps/alertmanager-*` 二进制 + `prometheus.yml.tpl` + 默认 rules；
  docker 用 `prom/prometheus` / `prom/alertmanager` 镜像。
- Agent 无监听端口（纯 gRPC 出向）。主机指标任务（E3）已定 **B 链路**：Agent 采集 → gRPC 上报 → 控制面统一导出。

### 我们自己的 PromQL 体量（闭合集合）

- Go 代码内固定表达式 **~41 条**（dashboard/cluster+jobs、sync/timeseries+summary+sparklines）：
  `rate`/`increase`/`sum by`/`max by`/`min by`/`topk`/`max_over_time`/`clamp_min`/`and on` 向量匹配。
- 随产品发布的规则文件：10 条录制 + 11 条默认告警（`install/observability/prometheus_config/rules/`，
  deploy/docker 同一份）。其中 5 条默认告警引用 `seatunnel:*` 录制序列。
- 指标模板告警 8 条固定表达式（`SuggestedPromQL`，用户只挑算子/阈值/窗口）。
- 结论：**没有一条 PromQL 来自用户自由输入**（custom_promql 移除后）。所有查询都是产品自有的、有限的。

### 已有的进程内能力（为什么边界可以更小）

- `alert_policy_execution_service.go` 已实现 dedupe / cooldown 重发 / silence / close / 恢复前置 /
  reminder——**AM 的分组去重语义在 STX 已有等效实现**，AM 实际只当「规则评估触发器 + webhook 发射器」。
- `job_prometheus.go` 已演示「进程内数据 → client_golang Registry → 暴露 exposition」的完整模式；
  内嵌后把「Prometheus 来刮」改为「直接 append 进 tsdb」即可（registry `Gather()` → appender，或跳过 exposition 直写）。
- 抓取目标本来就是控制面算的：`collectManagedMetricsTargets` 产出 `cluster → 在线节点 → host:hazelcastPort` +
  metrics path + 标签，可直接喂自研抓取循环；HTTP SD 端点整个可以删。
- `up` 系列被 2 条查询 + 1 条默认告警使用——自研抓取循环需自行合成 `up{job,instance,...}`。
- `seatunnel:` 录制序列只被 5 条默认告警和（即将移除的）Grafana 面板引用，大盘代码全部用原始表达式——
  可以把录制表达式内联进告警评估，不必实现通用录制规则。

### 内嵌边界（本任务的方案骨架）

| 组件 | 处理 | 备注 |
|---|---|---|
| `prometheus/prometheus/tsdb` | **嵌入**：进程内 Head/DB，`data/` 下存块 | 保留期变 STX 配置项（顺带解决 E2 的 15 天限制） |
| `prometheus/prometheus/promql` | **嵌入**：进程内 `Engine`，`promquery.Range/Instant` 改走本地 | 签名不变，5 个调用文件零改动（BaseURL 改返回内嵌哨兵） |
| `prometheus/common/expfmt` | **嵌入**：解析 text exposition | 自研抓取循环用 |
| `scrape`/`discovery`/`rules`/`notifier`/`web/api`/`config` | **不嵌入** | 唯一远程抓取面 = SeaTunnel `/hazelcast/rest/instance/metrics`，自研 ~小循环 + 合成 `up` + 打 cluster 标签 |
| | | **⚠ 实测（prometheus v0.316.0，scratch module 验证）：仅 `tsdb`+`promql`+`expfmt` 的探针二进制 = 24.7MB / 727 包，但 `tsdb`→`config`→全部 `discovery` providers，client-go（14 包）、aws-sdk-v2（55）、azure-sdk（30）、msal（22）、smithy（27）**都编进了二进制**；模块图 ~294 个依赖。`remove_all_sd` build tag 在 0.316 尚未落地。接受死代码换取零外挂进程是划算的（对比如今外挂 prom 170MB + AM ~30MB），但 design 必须写明依赖图膨胀与升级验证面。** |
| Alertmanager | **退役**：策略评估改定时进程内 instant 查询 → 现有投递链 | rule 文件 / promtool / reload / AM 配置 / SIGHUP 全删 |
| HTTP SD 端点 | **删除**（不再有人刮） | `prometheus/discovery` + `__STX_SD_URL__` 模板 |
| `/api/v1/monitoring/prometheus/jobs/metrics` | **删除**（无鉴权信息泄露面） | `stx_job_*` 只进内嵌 tsdb |
| AM webhook 端点（remote ingest） | **保留 + 鉴权**：`remote_ingest.token` Bearer 校验 | 未配置 token 维持现状 + 启动 WARN |
| `custom_promql` 能力 + 构建器 | **移除**（用户决定不做自定义） | 存量数据处置见 Open Q1 |

## Requirements（骨架，design 阶段细化）

- `observability.enabled=true` 时：进程内 tsdb + promql 可用；`promquery.Range/Instant`、诊断快照、大盘全部走本地引擎。
- 自研抓取循环按配置间隔抓 `collectManagedMetricsTargets` 的所有目标：HTTP GET + expfmt 解析 + tsdb append +
  合成 `up` + 附加 `job/cluster/cluster_id/env` 标签；单目标失败记为该目标 `up=0` 与 last_error。
- 进程内生产者：`stx_job_*`（Registry gather → append）、未来 `stx_host_*`（E3 B 链路上报 → append）。
- 告警评估：定时器对启用的 metrics_template 策略跑固定形状 instant 查询 + 算子/阈值/窗口判定 →
  走 `dispatchAlertDelivery*` 投递链（episode/fingerprint 兼容现状）；随产品发布的 11 条默认告警同样进程内评估
  （`seatunnel:` 录制表达式内联）。
- 平台健康 / 作业类策略维持现状（本就进程内评估）。
- 配置：`observability.prometheus.url`、`alertmanager.url`、`grafana.url` 移除；新增内嵌存储配置
  （数据目录、retention、scrape interval）；`observability.enabled` 仍是总开关。
- 安装/部署：去掉 prometheus、alertmanager 两个二进制/镜像、prometheus.yml.tpl、rules 目录、deps glob；
  升级路径处理旧数据目录与旧端口。
- 前端：能力卡「custom_promql」与对应构建器移除；integration status 组件集改为内嵌语义；
  策略中心列表对存量 `custom_promql` 行的展示（见 Open Q1）。
- CLI：受影响命令同步（integration status / dashboard / 告警相关）。

## Open Questions

（无——全部已定。）

## Acceptance Criteria（草案）

- [ ] 实测已证实 `tsdb`→`config`→discovery providers 会把 client-go / aws / azure 死代码带进二进制（+~10-15MB 估）；
  验收项改为：**不引入 `scrape`/`discovery`/`rules`/`web`/`notifier` 的包级使用**，二进制净增控制在 ~30MB 内，
  go.mod 依赖增量与 licenses 清单在 design 中记录。
- [ ] `observability.enabled=true`：大盘/作业趋势/诊断快照在无外部 Prometheus 进程的情况下工作，数据与外部部署抽样一致。
- [ ] SeaTunnel 目标不可达时 `up=0` 正确合成，integration status 中 seatunnel_metrics 仍能给「目标不可达」语义。
- [ ] metrics_template 策略触发 → 通知投递路径与改造前等价（dedupe/cooldown/silence/恢复/recovery 全保）。
- [ ] `observability.enabled=false`：相关页面统一空态，无新增错误。
- [ ] 构建产物中不再包含 prometheus/alertmanager 二进制与配置模板；config.example.yaml 更新。
