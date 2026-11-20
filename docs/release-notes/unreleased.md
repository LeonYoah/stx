<!--
Licensed to the Apache Software Foundation (ASF) under one or more
contributor license agreements.  See the NOTICE file distributed with
this work for additional information regarding copyright ownership.
The ASF licenses this file to You under the Apache License, Version 2.0
(the "License"); you may not use this file except in compliance with
the License.  You may obtain a copy of the License at

   http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
-->

<!-- 下一版发布说明草稿：发正式版时改名为 docs/release-notes/<tag>.md 再发布。
     Draft for the next release: rename to docs/release-notes/<tag>.md when cutting a stable tag. -->

# <tag> 发布说明 / <tag> Release Notes

## 升级须知 / Upgrade Notes

- 建议升级前备份数据目录（SQLite 复制 `data/stx.db*`，MySQL 用 mysqldump）。
  Back up the data dir before upgrading (copy `data/stx.db*` for SQLite, mysqldump for MySQL).
- 监控已从外部 Prometheus / Alertmanager / Grafana 三件套切换为内置实现。一键升级由旧版本
  升级器执行，新版本首次启动时会自动用内嵌脚本刷新 `bin/`（旧脚本备份为 `.bak-<时间戳>`），
  并在启动后约 3 分钟内按 pid 文件停掉仍在运行的旧三件套进程；`observability/` 数据目录保留。
  Monitoring moved from the external Prometheus / Alertmanager / Grafana trio to the embedded
  implementation. In-app upgrades run under the OLD upgrader; on first start the new binary
  refreshes `bin/` from embedded scripts (old files become `.bak-<ts>`) and stops any still-running
  legacy trio processes by pid file within ~3 minutes; the `observability/` data dir is kept.
- Alertmanager 时代遗留的未恢复告警（没有恢复事件通道）会在首次启动时被一次性标记为已恢复，
  不发送恢复通知。Legacy unresolved alerts from the Alertmanager era are marked resolved once
  at first boot; no recovery notifications are sent.
- 时区回退从「展示时区」改为控制面机器时区（`time.Local`）。升级前建议把各 Agent 时区与控制面
  对齐；容器部署建议显式设置 `TZ` 或 `STX_SEATUNNEL_TIMEZONE`。The engine time fallback moved
  from the display timezone to the control-plane machine timezone (`time.Local`). Align agent
  timezones before upgrading; for containers set `TZ` or `STX_SEATUNNEL_TIMEZONE` explicitly.
- 老版本 Agent 继续兼容，但主机指标与节点时区采集需要升级 Agent 才能完整工作。
  Older agents keep working, but host metrics and node-timezone collection need the new agent.
- `observability.enabled` 配置项名字未变、含义已变：升级前它表示对接外部三件套，现在表示开启内置监控。
  v1.4.0 配置模板默认 `true`，因此升级后内置监控会自动开始采集，指标写入 `data/tsdb`，内存与磁盘占用
  会相应上涨；容量与采集上限的调优方法见《内置监控说明》。The `observability.enabled` key kept its name
  but changed meaning: it used to wire the external trio and now enables embedded monitoring. Since the v1.4.0
  config template defaults it to `true`, embedded monitoring starts collecting automatically after upgrade,
  writing metrics to `data/tsdb` and raising memory/disk usage; see the embedded monitoring doc for tuning.
- 告警配置一次性迁移（SchemaFlag `alert_rule_catalog_v1`，事务内执行，重复启动不再跑）：
  旧的指标模板 / 平台健康策略迁移为集群规则目录的全局配置与集群例外；旧通知路由按过滤条件并入
  默认通知或对应规则的自定义通知；有启用渠道的策略记为严重级别、无渠道的记为警告。旧策略行保留
  但置为停用，便于排查；`sync_job` 任务告警策略不受影响。One-time alert-config migration
  (SchemaFlag `alert_rule_catalog_v1`, transactional, skipped on later boots): legacy metric-template /
  platform-health policies become the cluster-rule catalog's global config and per-cluster overrides;
  legacy notification routes are merged into the notification defaults or a rule's custom notification by
  match condition; policies with enabled channels become critical, otherwise warning. Legacy policy rows
  are kept but disabled for audit; `sync_job` task policies are untouched.

## 破坏性变更 / Breaking Changes

- 告警类别统一为 `platform_health` / `metric` / `job`；创建或更新告警策略时传旧值返回 400。
  Alert categories are now `platform_health` / `metric` / `job`; requests with legacy values get 400.
- 通知路由接口已删除：`/api/v1/monitoring/notification-routes` 的增删改查不再可用（调用方收到 404）。
  通知改由「通知渠道 + 默认通知 + 规则自定义通知」承载；旧路由数据行保留在表中不再生效，
  升级迁移时按过滤条件并入默认通知或对应规则的自定义通知。Notification-route APIs are removed:
  CRUD under `/api/v1/monitoring/notification-routes` now 404s. Notification is carried by channels +
  defaults + per-rule custom config; legacy route rows stay in the table but no longer apply, and the
  upgrade migration merges them into defaults or per-rule custom notification by match condition.
- 旧条件的自动巡检策略不再生效但不会删除：列表 / 详情 API、CLI 与
  界面通过 `invalid_reason` 标出，需编辑重新选择。Legacy-condition auto policies stop working
  but are not deleted: `invalid_reason` flags them in API, CLI and UI for re-editing.
- 非 `sync_job` 的告警策略（`platform_health` / `metrics_template` / `custom_promql`）创建与更新
  一律被拒绝并提示改用 cluster-rules；只保留 `sync_job` 任务告警策略的读写（集群级规则改为
  内置目录，见新功能）。Non-`sync_job` alert policies (`platform_health` / `metrics_template` /
  `custom_promql`) are rejected on create/update with guidance to use cluster-rules; only `sync_job`
  task policies remain writable (cluster-level rules are now a builtin catalog, see Features).
- 删除接口：`GET /monitoring/prometheus/discovery`、`GET /monitoring/prometheus/jobs/metrics`、
  `POST /monitoring/alertmanager/webhook`、`ANY /monitoring/proxy/grafana`、
  `ANY /monitoring/proxy/grafana/*proxyPath`。Removed APIs: the two Prometheus discovery
  endpoints, the Alertmanager webhook, and both Grafana proxy routes.
- 安装脚本删除参数 `--with-observability`、`--prometheus-port`、`--alertmanager-port`、
  `--grafana-port`：`install.sh` 收到会直接报 `unknown option` 退出，请从安装命令里移除；
  仅 `bin/start.sh` / `bin/stop.sh` 还兼容忽略旧 systemd 单元传入的 `--observability`。
  Installer flags removed: `--with-observability`, `--prometheus-port`,
  `--alertmanager-port`, `--grafana-port` — `install.sh` exits with `unknown option` when
  given them, so remove them from install commands; only `bin/start.sh` / `bin/stop.sh`
  still accept-and-ignore `--observability` passed by old systemd units.
- Webhook 通知负载中的 `source_type` 取值随告警类别变化（`platform_health` / `metric` / `job`）。
  The `source_type` value in webhook notification payloads follows the new alert categories.
- 引擎时间回退时区改为控制面机器时区（见升级须知）。Engine time fallback now uses the
  control-plane machine timezone (see Upgrade Notes).

## 新功能 / Features

- 首页新增集群 / 作业 / 主机大盘。New home dashboards for clusters, jobs and hosts.
- 内置 tsdb 自观测与容量护栏：自采集、自监控指标与磁盘容量保护。
  Embedded tsdb self-observability and capacity guardrails.
- 管理员可用的进程 pprof 剖析，默认关闭。Admin-only process pprof profiling, off by default.
- 告警规则目录：内置规则可视化查阅。Alert rule catalog for browsing builtin rules.
- 集群级告警规则目录化：21 条内置规则统一替代旧的内置 YAML 规则与策略模板，支持全局配置、
  单集群例外、开关 / 阈值 / 持续时间 / 级别 / 通知方式编辑，重叠来源的重复告警去重；
  「警告」只进告警中心，「严重」必须外发。Cluster-level rules are now a 21-rule catalog replacing
  the builtin YAML rules and policy templates: global config + per-cluster overrides for enable /
  threshold / duration / severity / notification, with dedup across overlapping sources; warning
  stays in the alert center, critical must deliver externally.
- 告警中心重构：`/monitoring` 分为告警 / 规则 / 通知三个面板；告警实例按范围过滤
  （`scope=cluster|job`）。Reworked alert center: `/monitoring` has alerts / rules / notification
  panels; alert instances filter by scope (`scope=cluster|job`).
- 可用性告警：节点离线 / Agent 离线 / Master 不可用 / 可用 Worker 不足由可用性评估器统一判定，
  支持持续时间、启动抑制与恢复通知。Availability alerts (node offline / agent offline / master
  unavailable / insufficient workers) are evaluated by one availability evaluator with duration,
  startup suppression and recovery notification.
- 新增监控接口与 CLI：`GET|PUT|DELETE /api/v1/monitoring/cluster-rules[/:key[/clusters/:clusterId]]`、
  `GET|PUT /api/v1/monitoring/alert-notification-defaults`；`stx monitoring cluster-rule list|get|update
  |override set|delete`、`stx monitoring notification-defaults get|update`。New monitoring APIs and CLI:
  the cluster-rules and alert-notification-defaults endpoints, plus `stx monitoring cluster-rule …`
  and `stx monitoring notification-defaults …` commands.
- CLI 支持导入本地升级包并跟踪自升级任务：`stx self-upgrade import <file> --confirm`（流式上传）、
  `stx self-upgrade task get <id> --wait`、`stx self-upgrade task cancel <id> --confirm`。
  The CLI can now import local upgrade packages and track self-upgrade tasks.

## 修复 / Fixes

- 一键升级后旧版本 `bin/` 脚本不再残留：新二进制启动时自修复安装布局并停止旧监控三件套。
  After in-app upgrades the legacy `bin/` scripts no longer linger: the new binary self-heals
  the layout and stops the legacy monitoring trio.
- Alertmanager 时代遗留告警不再永久计入活跃告警。Legacy Alertmanager-era alerts no longer
  count as active forever.
- 回归缺陷修复与 JDK 固定路径探测（#147）。Regression fixes and pinned-JDK path probing (#147).

## 已知问题 / Known Issues

- 旧 Grafana 面板与外部 Prometheus 历史数据不迁移；`observability/` 目录保留可自行清理。
  Legacy Grafana dashboards and external Prometheus history are not migrated; the kept
  `observability/` directory can be removed manually.
