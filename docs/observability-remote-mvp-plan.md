# STX 可观测性（远程集成）MVP 规划

基于 `docs/observability-remote-design.md`。

## 1. MVP 目标

在不托管三件套进程（Prometheus/Alertmanager/Grafana）的前提下，完成最小可用远程集成闭环：

1. Prometheus 通过 STX HTTP SD 动态发现所有集群 metrics 目标；
2. Alertmanager 通过固定 webhook 推送告警到 STX；
3. STX 完成告警入库并可查询；
4. STX 提供集群级与平台级健康摘要 API；
5. Grafana 看板 JSON 以固定目录交付，支持导入使用。

---

## 2. 范围

## In Scope（MVP）

### M1：接入基础能力（本轮优先）

- 配置模型调整（远程集成）：
  - 保留 `observability.enabled` 作为总开关；
  - 新增：
    - `observability.prometheus.http_sd_path`
    - `observability.alertmanager.webhook_path`
- 启动校验（fail-fast）：
  - 当 `observability.enabled=true` 时，校验 `app.external_url` 为合法 HTTP(S) URL；
  - 校验三件套 URL 格式合法；
- 新增公开接口：
  - `GET /api/v1/monitoring/prometheus/discovery`
  - `POST /api/v1/monitoring/alertmanager/webhook`
- Alertmanager webhook 告警落库（最小字段集 + 去重 upsert）。

### M2：可视化闭环

- 新增告警查询 API（按集群、状态、时间过滤）；
- 新增集群健康与平台健康聚合 API：
  - `GET /api/v1/clusters/health`
  - `GET /api/v1/monitoring/platform-health`
- Grafana Dashboard JSON 固定目录交付与下载说明。

### M3：联调与发布

- Prometheus/Alertmanager/Grafana 对外接入手册；
- 联调脚本与 smoke 测试；
- 兼容性与回归验证。

## Out of Scope（MVP 外）

- STX 拉起与管理三件套进程；
- Alertmanager/Silence 高级策略编排 UI；
- 告警通知渠道高级路由器；
- 多租户隔离与复杂 RBAC。

---

## 3. M1 详细拆解

### 3.1 配置模型

- [x] `internal/config/model.go` 增加：
  - `ObservabilityPrometheusConfig.HTTPSDPath`
  - `ObservabilityAlertmanagerConfig.WebhookPath`
- [x] `internal/config/config.go` 增加默认值：
  - `http_sd_path: /api/v1/monitoring/prometheus/discovery`
  - `webhook_path: /api/v1/monitoring/alertmanager/webhook`
- [x] `config.example.yaml` 同步更新。

### 3.2 启动校验

- [x] `observability.enabled=true` 时：
  - [x] `app.external_url` 非空且为 `http/https`；
  - [x] `observability.prometheus.url` / `alertmanager.url` / `grafana.url` 为合法 URL。
- [x] 校验失败 fail-fast（启动失败并打印明确错误）。

### 3.3 HTTP SD 接口

- [x] 新增 handler：`GET /api/v1/monitoring/prometheus/discovery`
- [x] 返回 Prometheus HTTP SD 标准数组：
  - `targets`: `host:port`
  - `labels`: `cluster_id` / `cluster_name` / `env`
- [x] 仅返回可探测通过的 metrics 目标（MVP 可按现有探测逻辑）。

### 3.4 Alertmanager Webhook

- [x] 新增 handler：`POST /api/v1/monitoring/alertmanager/webhook`
- [x] 接收 Alertmanager webhook 标准 payload；
- [x] 落库字段（MVP）：
  - `fingerprint`, `status`, `alertname`, `severity`, `cluster_id`, `cluster_name`, `env`, `starts_at`, `ends_at`, `summary`, `description`, `labels_json`, `annotations_json`, `last_received_at`
- [x] 去重策略：`fingerprint + starts_at` 唯一 upsert。

---

## 4. 验收标准（M1）

- [x] `observability.enabled=true` 且 `app.external_url` 非法时，STX 启动失败并提示具体错误；
- [x] Prometheus 请求 HTTP SD 接口可拿到标准 TargetGroup JSON；
- [x] Alertmanager webhook 推送后，告警记录可在数据库中查询到，重复告警可正确 upsert；
- [x] `observability.enabled=false` 时，不注册以上公开接口。

---

## 5. M2/M3 进度追踪（持续更新）

### M2：可视化闭环

- [x] 告警查询 API：`GET /api/v1/monitoring/remote-alerts`
- [x] 集群健康聚合 API：`GET /api/v1/clusters/health`
- [x] 平台健康聚合 API：`GET /api/v1/monitoring/platform-health`
- [x] 告警状态/时间/分页过滤能力（MVP）

### M3：联调与发布

- [x] 对外接入手册（`docs/observability-remote-integration-guide.md`）
- [x] 联调脚本与 smoke 测试（`scripts/observability-remote-smoke.sh`）
- [x] Grafana Dashboard 固定目录交付（`deps/grafana_config/dashboards`）

---

## 6. 提交建议

1. `docs(observability): add remote integration mvp plan`
2. `feat(config): add remote observability paths and startup validation`
3. `feat(monitoring): add prometheus http-sd endpoint`
4. `feat(monitoring): add alertmanager webhook ingest and persistence`

---

## 7. 实施日志（按步骤）

> 说明：以下日志用于约束“按 MVP 设计推进”，避免偏离范围。

- 2026-02-27 / Step-1（M1）  
  完成远程集成基础能力：配置校验、HTTP SD、Alertmanager Webhook、告警落库去重。

- 2026-02-27 / Step-2（M2）  
  完成查询聚合 API：`/monitoring/remote-alerts`、`/clusters/health`、`/monitoring/platform-health`。

- 2026-02-27 / Step-3（M3）  
  完成接入文档与联调脚本：`observability-remote-integration-guide.md`、`observability-remote-smoke.sh`；  
  固定交付中英文 Dashboard JSON 到 `deps/grafana_config/dashboards`。

- 2026-02-27 / Step-4（M2 UI）  
  监控中心前端完成平台健康摘要、集群健康表、远程告警列表对接。

- 2026-02-27 / Step-5（M2 UI 增强）  
  远程告警页增加时间过滤、分页、按 `cluster_id` URL 过滤；  
  平台健康表增加跳转详情与跳转告警入口。

- 2026-02-27 / Step-6（M3 联调）  
  在本地临时环境完成端到端 smoke：  
  `scripts/observability-remote-smoke.sh`（含登录态）全部通过，覆盖：  
  `prometheus/discovery`、`alertmanager/webhook`、`remote-alerts`、`clusters/health`、`platform-health`。

- 2026-02-27 / Step-7（M3 回归）  
  新增并执行开关回归脚本 `scripts/observability-enabled-switch-regression.sh`：  
  - `observability.enabled=true`：discovery/webhook 均 `HTTP 200`；  
  - `observability.enabled=false`：discovery/webhook 均 `HTTP 404`；  
  两条路径回归通过。

- 2026-02-27 / Step-8（外部环境预检查）  
  对线上域名 `https://cpa.120501.xyz` 做只读探测：  
  - `GET /api/v1/health` -> `HTTP 200`  
  - `GET /api/v1/monitoring/prometheus/discovery` -> `HTTP 404`  
  - `POST /api/v1/monitoring/alertmanager/webhook` -> `HTTP 404`  
  结论：当前外部环境未暴露默认远程集成路径（常见原因：`observability.enabled=false` 或 path 改写），  
  因此“真实外部三件套端到端联调”仍待环境侧开关与路径确认后继续。

- 2026-02-27 / Step-9（外部三件套端到端联调完成）  
  在 `deps` 已部署 Prometheus/Alertmanager/Grafana 的前提下，完成真实联调闭环：  
  1) STX 控制面升级为当前分支最新二进制并重启（端口 `17800`）；<br>
  2) Prometheus 配置切换为 `http_sd_configs` 指向  
     `http://127.0.0.1:17800/api/v1/monitoring/prometheus/discovery` 并 reload；  
  3) Alertmanager receiver 增加 webhook  
     `http://127.0.0.1:17800/api/v1/monitoring/alertmanager/webhook` 并热重载；  
  4) 通过 `https://cpa.120501.xyz` 执行  
     `scripts/observability-remote-smoke.sh`（含登录态）全部通过；  
  5) 通过向 Alertmanager `POST /api/v2/alerts` 注入测试告警，验证告警成功转发并落库到  
     `GET /api/v1/monitoring/remote-alerts`（总数从 2 增至 3，命中 `AMForwardTest`）。

- 2026-02-27 / Step-10（deps 脚本简化）  
  按 MVP 最小原则收敛本地脚本职责：  
  - 删除 `deps/set-observability-remote-mode.sh`，不再引入额外“模式切换脚本”；  
  - `deps/start-observability.sh` 仅负责本地三件套一键重启（含默认配置初始化）；  
  - `deps/init-observability-defaults.sh` 固定生成本地 `static_configs`（默认 `127.0.0.1:*`）；  
  - 远程联动统一通过 `config.yaml` 的 `observability.*.url` 与运维手工配置实现。

- 2026-02-27 / Step-11（遗留字段与脚本瘦身）  
  继续收敛远程集成模型，完成“1-2”阶段：  
  - `internal/config/model.go` 移除历史字段（`bundled_stack_enabled`、`auto_onboard_clusters`、`prometheus.manage_config` 等）；  
  - `monitoring.Service` 删除本地 Prometheus 配置改写/热重载逻辑，仅保留远程健康探测与聚合；  
  - `router` 移除集群拓扑变更触发的本地 Prometheus 同步钩子；  
  - `scripts/package-release.sh` 移除对历史字段的 `sed` 改写；  
  - 回归脚本与设计文档同步更新，配置模板保持最小远程集成形态。

### 当前严格 MVP 剩余项（下一步）

- [x] 在真实外部三件套环境完成一轮端到端联调记录（不是仅本地接口可用）；
- [x] 补齐兼容性/回归验证记录（开启与关闭 `observability.enabled` 两条路径）；
- [x] 将联调结果与排障结论沉淀回文档（本文件 + integration guide）。
