## STX 可观测性（远程集成版）设计文档

### 1. 设计目标

- **一次性配置**：Prometheus / Alertmanager / Grafana 侧仅需在首次接入时改一次配置文件，后续新增 / 删除 SeaTunnel 集群无需再次改配置。
- **全程远程交互**：STX 不再内嵌或管理 Prometheus / Alertmanager / Grafana 进程，而是通过 HTTP SD、Webhook、Dashboard JSON 等方式与外部监控栈集成。
- **多集群可见性**：所有监控与告警以 `cluster_id` / `cluster_name` / `env` 等 label 区分，既能查看单集群健康，也能聚合成平台级视图。
- **可插拔**：不开启 `observability` 时，STX 正常工作；开启后，按需接入外部 Prometheus / Alertmanager / Grafana。

---

### 2. 配置模型（`config.yaml` 中的 `observability` 段）

在当前示例配置文件 `config.example.yaml` 基础上，`observability` 段调整为：

```yaml
observability:
  # 总开关：关闭后监控中心集成状态只展示 disabled，且不暴露下述监控相关接口
  enabled: true

  prometheus:
    # Prometheus Web / 查询地址，仅用于健康检查和控制台跳转
    url: "http://127.0.0.1:9090"
    # STX 对外约定的 HTTP SD 路径（固定约定，不再需要是否启用的额外开关）
    # 实际访问地址 = app.external_url + http_sd_path
    http_sd_path: "/api/v1/monitoring/prometheus/discovery"

  alertmanager:
    # Alertmanager Web UI 地址，用于健康检查和跳转
    url: "http://127.0.0.1:9093"
    # STX 对外约定的告警 Webhook 路径（固定约定）
    webhook_path: "/api/v1/monitoring/alertmanager/webhook"

  grafana:
    # Grafana UI 地址，用于控制台嵌入跳转或文档引导
    url: "http://127.0.0.1:3000"

  seatunnel_metrics:
    # SeaTunnel metrics path，用于 HTTP SD 输出与连通性探测
    path: "/metrics"
    probe_timeout_seconds: 2
```

**已移除的历史字段（由本地一体化栈演进而来）：**

- `bundled_stack_enabled`
- `auto_onboard_clusters`
- `prometheus.manage_config`
- `prometheus.config_file`
- `prometheus.rules_glob`

这些字段与“由 STX 直接拉起和管理 Prometheus/Grafana/Alertmanager 进程及配置文件”强耦合，已从当前最小远程集成配置模型中移除。

**启用条件校验建议：**

- 当 `observability.enabled = true` 时，启动时需要至少校验：
  - `app.external_url` 非空且为合法 HTTP(S) URL；
  - `observability.prometheus.url` / `observability.alertmanager.url` / `observability.grafana.url` 若配置则校验格式。
- 校验失败时应 fail-fast，以避免监控相关接口“半开半关”的状态。

---

### 3. Prometheus 集成：HTTP 服务发现（HTTP SD）

#### 3.1 Prometheus 侧一次性配置

运维在 Prometheus 的 `prometheus.yml` 中，仅需配置一次 STX 相关 job，以 HTTP SD 方式动态发现所有 Seatunnel 集群节点：

```yaml
scrape_configs:
  - job_name: 'seatunnel_engine_http'
    metrics_path: /metrics
    scheme: http
    http_sd_configs:
      - url: http://<stx-external-url>/api/v1/monitoring/prometheus/discovery
        refresh_interval: 30s
```

- `<stx-external-url>` 由 `app.external_url` 决定。
- 之后新增 / 删除 Seatunnel 集群，仅需 STX 更改 HTTP SD 接口返回内容；Prometheus **无需再修改配置文件或 reload**。

#### 3.2 STX HTTP SD 接口设计

- **路径（固定约定）**：`GET /api/v1/monitoring/prometheus/discovery`
- **启用条件**：
  - 当 `observability.enabled = true` 时，该接口必须可用；
  - 当 `observability.enabled = false` 时，接口可直接返回 404 或 503。
- **返回结构**：Prometheus HTTP SD 标准格式，一个 `TargetGroup` 数组：

```json
[
  {
    "targets": ["10.0.0.1:8081", "10.0.0.2:8081"],
    "labels": {
      "job": "seatunnel_engine_http",
      "cluster_id": "c1",
      "cluster_name": "prod-cluster-1",
      "env": "prod"
    }
  },
  {
    "targets": ["10.0.1.1:8081"],
    "labels": {
      "job": "seatunnel_engine_http",
      "cluster_id": "c2",
      "cluster_name": "staging-cluster",
      "env": "staging"
    }
  }
]
```

- **字段说明**：
  - `targets`: Seatunnel metrics endpoint 列表，通常为 `host:port`；
  - `labels`: 该组 target 共享的 label，用于 PromQL 聚合与 Grafana 变量选择。
    - 约定至少包含：
      - `cluster_id`: STX 内部集群唯一 ID；
      - `cluster_name`: 集群展示名称；
      - `env`: 环境标识（如 `dev` / `staging` / `prod`）。
    - `job` 可以显式指定为 `seatunnel_engine_http`，也可以依赖 Prometheus 自身的 job 名。

#### 3.3 数据来源与更新策略

- HTTP SD 接口基于 STX 的集群元数据：
  - 每个集群记录：`id`、`name`、`env`、`metrics_port`、节点 IP 列表等。
  - 仅返回已启用监控且 metrics 连通性正常的集群，必要时可在元数据中增加“监控启用/禁用”标志。
- 集群的创建 / 扩缩容 / 删除：
  - 只需更新 STX 内部元数据；
  - HTTP SD 接口随之返回最新的 target 集合。

---

### 4. Alertmanager 集成：固定 Webhook + 内部分发

#### 4.1 Alertmanager 侧一次性配置

运维在 Alertmanager 的 `alertmanager.yml` 中，仅需增加一个指向 STX 的 Webhook receiver：

```yaml
route:
  receiver: 'stx'

receivers:
  - name: 'stx'
    webhook_configs:
      - url: 'http://<stx-external-url>/api/v1/monitoring/alertmanager/webhook'
        send_resolved: true
```

- `<stx-external-url>` 同样由 `app.external_url` 决定。
- 之后如需新增企业微信 / 钉钉 / 邮箱等通知渠道，不再建议通过修改 Alertmanager 配置文件来完成，而是交给 STX 内部的通知分发模块处理。

#### 4.2 STX Alertmanager Webhook 接口设计

- **路径（固定约定）**：`POST /api/v1/monitoring/alertmanager/webhook`
- **启用条件**：
  - 当 `observability.enabled = true` 时，该接口必须可用；
  - 当 `observability.enabled = false` 时，可拒绝请求（返回 404/503）。
- **请求体**：Alertmanager Webhook 标准结构，包含：
  - `status`、`receiver`、`groupLabels`、`commonLabels`、`alerts` 等字段。
- **内部处理流程（高层）**：
  1. 解析每条 `alert`，读取关键 label（如 `cluster_id`、`cluster_name`、`severity`、`alertname` 等）。
  2. 写入 STX 内部的告警存储（例如数据库表 `alerts`），支持按集群 / 时间线查看历史告警。
  3. 根据 STX 内部“通知配置”模块（独立设计，不在本文展开）决定：发送到哪些外部渠道（邮件、IM、自定义 Webhook 等）。
  4. 可选：支持在 STX 中配置静默规则（silences），对部分告警直接丢弃或降级。

通过这种设计，Alertmanager 的配置只在首次接入时改一次，后续所有通知策略变更都在 STX 内部完成。

---

### 5. Grafana 集成：固定数据源 + 固定路径 Dashboard JSON

#### 5.1 Grafana 侧一次性配置

Grafana 只需要在首次接入时配置一个 Prometheus 数据源，例如：

- Name / UID：建议固定为 `Prometheus` / `prometheus`；
- URL：`http://<prometheus-url>`;
- 其它认证参数按运维规范配置。

STX 的 Dashboard JSON 将默认引用 UID 为 `prometheus` 的数据源。

#### 5.2 Dashboard JSON 固定路径与打包方式

- Dashboard JSON 不再由运行时脚本临时生成，而是作为静态资源放在固定目录：

```text
deps/grafana_config/dashboards/
  seatunnel-overview-en.json
  seatunnel-overview-zh.json
  ...（未来可扩展更多专题面板）
```

- 部署脚本负责在打包时将上述 JSON 带入最终发布物，无需在配置文件中额外暴露 `dashboard_paths_root` 之类路径参数。
- 控制台中可以提供：
  - Dashboard JSON 的下载链接；
  - 在 Grafana 中导入 JSON 的简单步骤说明。

这种方式下，Grafana 侧只需要：

1. 首次创建 Prometheus 数据源；
2. 从 `deps/grafana_config/dashboards` 导入所需面板 JSON。

之后 STX 对 Dashboard 的改动只需要在新版本中更新 JSON 文件，无需用户侧再次修改配置。

---

### 6. 多集群健康与平台视图

在上述集成基础上，STX 可以围绕 `cluster_id` / `cluster_name` 这两个核心 label 构建两层健康视图：

- **集群级健康（Cluster Health）**：
  - 基于 Prometheus 指标与 Alertmanager 告警，计算每个 Seatunnel 集群的健康状态，例如：
    - `HEALTHY` / `DEGRADED` / `UNHEALTHY`；
  - 对应 API 示例（后续实现时参考）：
    - `GET /api/v1/clusters/:id/health`
    - `GET /api/v1/clusters/health`（返回所有集群健康摘要）。

- **平台级整体健康（Platform Health）**：
  - 从所有集群的健康状态聚合出平台级别的状态与统计，例如：
    - 当前集群总数、健康/降级/不健康数量；
    - 平台健康状态：例如“全部健康 / 部分降级 / 严重不健康”。
  - 对应 API 示例：
    - `GET /api/v1/monitoring/platform-health`

实现细节可以根据具体告警规则与指标定义逐步完善，但整体原则是：**监控与告警都是“按集群”组织，平台视图只是聚合展示。**

---

### 7. 与现有本地一体化脚本的关系

当前仓库中的以下脚本主要用于本地 demo / 一体化运行体验：

- `deps/init-observability-defaults.sh`
- `deps/start-observability.sh`

它们负责：

- 在本地 `deps/runtime` 下生成 Prometheus / Alertmanager / Grafana 配置与数据目录；
- 直接拉起三件套进程，并用于快速体验默认监控面板。

在本设计文档所描述的“远程集成模式”下：

- 推荐生产环境直接使用已有的 Prometheus / Alertmanager / Grafana 集群；
- 上述脚本可保留作为 **demo / 本地开发** 使用，但应在文档中标记为“非生产推荐路径”；
- 配置文件中的本地路径字段不再暴露给用户，而是由脚本内部自行管理。

---

### 8. 对外接入指南（摘要）

#### 8.1 STX 侧

1. 在 `config.yaml` 中设置：
   - `app.external_url` 为外部可访问的控制面地址；
   - `observability.enabled = true`；
   - 配置 `observability.prometheus.url` / `observability.alertmanager.url` / `observability.grafana.url`。
2. 重启 STX 控制面。

#### 8.2 Prometheus 侧

1. 在 `prometheus.yml` 中增加 `seatunnel_engine_http` job，并配置 HTTP SD：
   - URL 指向 `http://<stx-external-url>/api/v1/monitoring/prometheus/discovery`。
2. Reload Prometheus（或重启进程）。

#### 8.3 Alertmanager 侧

1. 在 `alertmanager.yml` 中增加指向 STX 的 Webhook receiver：
   - URL 指向 `http://<stx-external-url>/api/v1/monitoring/alertmanager/webhook`。
2. Reload Alertmanager（或重启进程）。

#### 8.4 Grafana 侧

1. 创建 Prometheus 数据源（UID 建议为 `prometheus`）。
2. 从 `deps/grafana_config/dashboards` 导入所需 Dashboard JSON。

完成以上步骤后，即可在：

- STX 内部看到按集群维度的健康状态与告警收敛信息；
- Grafana 中通过 Dashboard 面板查看每个 Seatunnel 集群的深度监控数据。
