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

## 破坏性变更 / Breaking Changes

- 告警类别统一为 `platform_health` / `metric` / `job`；创建通知路由或策略时传旧值返回 400。
  Alert categories are now `platform_health` / `metric` / `job`; requests with legacy values get 400.
- 旧告警类别的通知路由、旧条件的自动巡检策略不再生效但不会删除：列表 / 详情 API、CLI 与
  界面通过 `invalid_reason` 标出，需编辑重新选择。Legacy-category notification routes and
  legacy-condition auto policies stop working but are not deleted: `invalid_reason` flags them
  in API, CLI and UI for re-editing.
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
