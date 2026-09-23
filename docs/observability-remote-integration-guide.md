# STX 可观测性远程接入指南（MVP）

## 1. 目标

本指南用于把已有的 Prometheus / Alertmanager / Grafana 与 STX 远程对接，形成最小闭环：

- Prometheus 从 STX 拉取 HTTP SD 目标；
- Alertmanager 将告警推送回 STX；
- STX 提供告警查询和健康聚合 API；
- Grafana 导入 STX 提供的默认 Dashboard JSON。

补充：
- 如果你要走“本地 deps 一键启动（含预置配置文件）”模式，见：`docs/可观测性三件套一键接入说明.md`

---

## 2. STX 配置

在 `config.yaml` 中启用：

```yaml
app:
  external_url: "https://your-stx.example.com"

observability:
  enabled: true
  prometheus:
    url: "http://prometheus.example.com:9090"
    http_sd_path: "/api/v1/monitoring/prometheus/discovery"
  alertmanager:
    url: "http://alertmanager.example.com:9093"
    webhook_path: "/api/v1/monitoring/alertmanager/webhook"
  grafana:
    url: "http://grafana.example.com:3000"
```

说明：

- `observability.enabled=true` 时，STX 启用远程集成 API；
- `http_sd_path` / `webhook_path` 可按需修改，但建议保持默认；
- `app.external_url` 必须是可外部访问的 HTTP(S) 地址。

---

## 3. Prometheus 配置（一次性）

在 `prometheus.yml` 增加：

```yaml
scrape_configs:
  - job_name: 'seatunnel_engine_http'
    metrics_path: /metrics
    scheme: http
    http_sd_configs:
      - url: https://your-stx.example.com/api/v1/monitoring/prometheus/discovery
        refresh_interval: 30s
```

---

## 4. Alertmanager 配置（一次性）

在 `alertmanager.yml` 增加 receiver：

```yaml
route:
  receiver: 'stx'

receivers:
  - name: 'stx'
    webhook_configs:
      - url: 'https://your-stx.example.com/api/v1/monitoring/alertmanager/webhook'
        send_resolved: true
```

---

## 5. Grafana 默认面板

默认 Dashboard JSON 已固定放在：

- `deps/grafana_config/dashboards/seatunnel-overview-en.json`
- `deps/grafana_config/dashboards/seatunnel-overview-zh.json`

建议：

1. Grafana 中先创建 Prometheus 数据源，UID 使用 `prometheus`；
2. 导入上述 JSON；
3. 通过语言切换使用中英文面板 UID：
   - `seatunnel-overview-en`
   - `seatunnel-overview-zh`

---

## 6. 联调与 Smoke 测试

仓库内置脚本：`scripts/observability-remote-smoke.sh`

### 6.1 仅验证公开接口

```bash
./scripts/observability-remote-smoke.sh https://your-stx.example.com
```

会检查：

- Prometheus HTTP SD 接口；
- Alertmanager Webhook 入库接口。

### 6.2 额外验证登录后接口

```bash
STX_USERNAME=admin \
STX_PASSWORD=admin \
./scripts/observability-remote-smoke.sh https://your-stx.example.com
```

会额外检查：

- `GET /api/v1/monitoring/remote-alerts`
- `GET /api/v1/clusters/health`
- `GET /api/v1/monitoring/platform-health`

### 6.3 开关回归（enabled true/false）

仓库内置脚本：`scripts/observability-enabled-switch-regression.sh`

```bash
./scripts/observability-enabled-switch-regression.sh
```

默认行为：

- 使用 `go run . api` 启动临时实例（保证测试当前源码而非历史二进制）；
- 自动验证两组用例：
  - `observability.enabled=true`：`/prometheus/discovery`、`/alertmanager/webhook` 期望 `200`
  - `observability.enabled=false`：上述接口期望 `404`

---

## 7. 常见问题（MVP）

### Q1：`/api/v1/monitoring/prometheus/discovery` 返回 404

优先检查：

1. `observability.enabled` 是否为 `true`；
2. 是否修改了 `observability.prometheus.http_sd_path`；
3. 反向代理是否把 `/api/v1/...` 路径改写掉。

### Q2：`/api/v1/monitoring/alertmanager/webhook` 返回 404

优先检查：

1. `observability.enabled` 是否为 `true`；
2. 是否修改了 `observability.alertmanager.webhook_path`；
3. Alertmanager 回调 URL 是否与 STX 暴露路径一致。

---

## 8. 本地三件套联调参考

本地栈由控制面 `bin/start.sh` 管理（默认 `--observability auto`），资产在 `$INSTALL_DIR/observability/`：

```bash
/opt/stx/bin/start.sh
/opt/stx/bin/status.sh
/opt/stx/bin/start.sh --observability off
```

约定：

- 启停脚本不负责「远程模式自动改写」；
- 远程可观测性联动依赖 `config.yaml` 中 `observability.*.url`；
- 默认本地地址是 `127.0.0.1:9090/9093/3000`。

详见 [可观测性三件套一键接入说明](./可观测性三件套一键接入说明.md)。

如需手工检查，关键点如下：

1. **Prometheus**
   - 本地部署可先使用默认 `static_configs`
   - 若切远程 HTTP SD，由运维手工配置 `http_sd_configs`（不是本地启停脚本职责）

2. **Alertmanager**
   - 若需要告警回流 STX，由运维手工增加 webhook receiver
   - 然后热重载（SIGHUP）或重启 Alertmanager

3. **验证**
   - 执行：
     ```bash
     STX_USERNAME=admin \
     STX_PASSWORD=<password> \
     ./scripts/observability-remote-smoke.sh https://cpa.120501.xyz
     ```
   - 结果应覆盖并通过：
     - `prometheus/discovery`
     - `alertmanager/webhook`
     - `remote-alerts`
     - `clusters/health`
     - `platform-health`

---

## 9. 监控中心 UI（MVP）能力说明

当前监控中心（以 Grafana 为主）对应 MVP 已支持：

1. **平台健康摘要**
   - 数据来源：`GET /api/v1/monitoring/platform-health`
   - 展示总集群、健康/降级/异常集群、活动/严重告警

2. **集群健康表**
   - 数据来源：`GET /api/v1/clusters/health`
   - 支持一键跳转到集群详情和告警页（带 `cluster_id` 过滤）

3. **远程告警列表**
   - 数据来源：`GET /api/v1/monitoring/remote-alerts`
   - 支持筛选：`cluster_id`、`status`、`start_time`、`end_time`
   - 支持分页：`page`、`page_size`
