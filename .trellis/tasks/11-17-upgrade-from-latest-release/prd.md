# 从最新正式版平滑升级到当前版本

v1.4.0（2026-11-09）之后 main 合了 74 个提交：监控从外部 Prometheus / Alertmanager / Grafana 整套换成内置
tsdb 与告警评估器，告警语义也做了不兼容的统一（#146）。打下一个 tag 之前，要让从最新正式版升级的用户不踩坑，
并让 CI 能证明这一点。

## 已确认事实（2026-11-17，基于 main `66bd15ef` 与 tag `v1.4.0` 复核）

### 一键升级（应用内自升级）

- 一键升级包 `stx-upgrade-<ver>-linux-<arch>.tar.gz` 只含 `stx`、`lib/agent/*`、java-proxy jar、
  `scripts/stx-java-proxy.sh`、`frontend/`（`install/package-release.sh:689-716`），**不含 `bin/`**。
- 执行升级的是**旧版本**的二进制。v1.4.0 的 `applyStep`（`internal/apps/selfupgrade/service.go:1529-1611`
  @v1.4.0）只替换上面这些路径，包里即使带了 `bin/` 也不会被复制。
- v1.4.0 的 systemd 单元是 `Type=oneshot`，`ExecStart=bin/start.sh`、`ExecStop=bin/stop.sh`
  （`install/install-core.sh:250-262`@v1.4.0）。自升级重启走 `systemctl restart`。
- v1.4.0 的 `bin/start.sh` 依次启动 backend → frontend → 外部监控三件套（`start.sh:177-179`@v1.4.0），
  三件套逻辑在 `bin/lib/observability.sh`。三件套的 pid 文件是
  `observability/{prometheus/prometheus.pid, alertmanager/alertmanager.pid, grafana/grafana.pid}`。
- **结论**：一键升级后，旧 `start.sh` 每次重启都会继续拉起三件套，与内置监控并存，造成双份抓取、
  多占内存与端口。旧 Alertmanager 还会持续向已删除的 webhook 推送，收到 404。
  `docs/内置监控说明.md` 里"重启时一并停止"的说法只对 `install.sh` 重装成立。

### 遗留告警与失效配置

- v1.4.0 的指标告警全部由 Alertmanager 经 `POST /api/v1/monitoring/alertmanager/webhook`（无鉴权）
  写入 `monitoring_remote_alerts`。升级后不会再有人发"已恢复"。
  - `ListUnresolvedRemoteAlerts` 只看 `status <> 'resolved'`（`internal/apps/monitoring/repository.go:362`），
    所以这些记录会**永远**计入告警中心、首页和大盘的活跃告警。
  - 区分方法：内置评估器写入的 `generator_url` 为空（`metric_evaluator.go:455`）；Prometheus 产生的告警
    带 `generatorURL`。两者的 `receiver` 都是 `stx-webhook`，不能用来区分。
- #146 按"破坏性变更、不做兼容"落地，以下旧数据会**静默失效**：
  - 通知路由 `source_type` 为旧值（`local_process_event` / `remote_alertmanager` / `sync`）：
    `matchRemoteAlertRoute`（`notification_delivery_service.go:530`）不再匹配，**通知停发且无任何提示**。
  - 巡检自动策略：
    - `PROM_*` / `NODE_UNHEALTHY` 条件被检查器跳过；
    - `ALERT_FIRING` 未选 `rule_keys` 时永不触发（`auto_policy_checker.go:136-190`）。

### 其他用户可感知的变化（文档需补齐）

- 删除的接口（对比 `route_baseline.json`）：
  - `GET /monitoring/prometheus/discovery`、`GET /monitoring/prometheus/jobs/metrics`（HTTP SD）；
  - `POST /monitoring/alertmanager/webhook`；
  - `ANY /monitoring/proxy/grafana`、`ANY /monitoring/proxy/grafana/*proxyPath`。
  - 现有文档只提到了 webhook。
- v1.4.0 默认配置就是 `observability.enabled: true`，没装三件套的用户升级后会自动开启内置监控：
  STX 进程开始抓取所有节点、写 `data/tsdb`（31 天 / 30GiB），内存上涨。
- 时区回退值从"展示时区（默认 Asia/Shanghai）"变为"控制面 `time.Local`"（#147）。
  Agent 未升级、心跳不带时区时，UTC 容器里解析出的作业结束时间偏 8 小时。
- 主机指标（`ReportHostMetrics`）和节点时区都需要新版 Agent，老 Agent 兼容但没有数据。
- Docker Compose：旧的 prometheus / alertmanager / grafana 容器会变成孤儿，需要 `--remove-orphans`。

### CI 与发布流程现状

- 现有 `E2E Real Upgrade` 测的是 **SeaTunnel** 2.3.13 → 3.0.0（`frontend/scripts/e2e/run-real-upgrade.sh`），
  不是 STX 自身的跨版本升级。离线包冒烟只覆盖全新安装。
- `release-on-tag.yml` 只用 `generate_release_notes: true`（PR 标题列表），Release 页面没有破坏性变更 /
  升级须知。
- v1.4.0 自升级可以导入本地构建的包。接口前缀为 `/api/v1/self-upgrade`：
  1. `POST /check`，`{"force_latest":"<ver>"}`：把目标版本设为任意合法版本；
  2. `POST /stage/import`：multipart 字段 `file`，上限 500MB；
  3. `POST /execute`；
  4. `GET /tasks/:id`：查询任务状态。

## Locked Decisions（用户确认）

1. **CI 跨版本升级测试的形态**：
   - 先装**当前最新正式版**（GitHub `releases/latest`，动态取，不写死 v1.4.0）；
   - 再用**当前代码**构建一个一键升级包；
   - 通过应用内一键升级完成升级，然后做断言。
2. #146 的"不做旧数据兼容"不变：失效的旧配置**只显式提示，不自动改写、不加别名**。
3. 遗留的三件套数据目录**保留不删**，只停进程。

## Requirements

### R1 一键升级后安装布局自修复

- 新版本启动时，如果检测到自己运行在安装目录布局中，用内置的最新版本覆盖 `bin/start.sh`、`stop.sh`、`status.sh`，
  并移走 `bin/lib/observability.sh`。旧文件备份后再替换，整个过程幂等。
- 以后的一键升级包也带上 `bin/`，并由新版升级器替换。
- 新版本启动后的一段时间内，按 pid 文件停掉旧三件套进程（核对进程名后才发信号）。
  不删除 `observability/` 目录。

### R2 遗留告警一次性收尾

- 升级后首次启动时，把 Prometheus 时代遗留、仍未恢复的远程告警标记为已恢复。
  - 只执行一次，有标记记录，并发启动也安全；
  - **不发送**恢复通知。

### R3 失效配置显式化

- 通知路由与巡检自动策略在列表 / 详情 API 中返回失效原因，界面醒目提示，CLI 输出同样带出。
- 启动时为每类失效配置各记一条 WARN，列出 ID。

### R4 CI：最新正式版 → 当前代码 的一键升级测试

- 新增工作流：按 Locked Decision 1 执行，覆盖：
  - 数据保留、版本切换、二次重启；
  - 旧三件套已停；
  - 遗留告警已收尾；
  - 失效配置已标记。
- 与"最新正式版具备什么能力"相关的断言按能力探测执行：最新版本身已没有三件套时自动跳过这些断言，测试仍然有效。
- 在 PR（相关路径）、main、手动触发时运行，并作为 `release-on-tag` 发布前的门禁。
- **CLI 优先**：
  - 升级前的登录、植入和一键升级，用安装目录里 FROM 自带的 `stx` CLI；
  - 升级后的接口断言，用同一路径下已替换成 TARGET 的 CLI；
  - 只有 FROM 的 CLI 缺少某个命令时，才在那一步退回接口调用。
  - 全程只登录一次，用来验证 CLI 凭据跨升级有效。

### R7 CLI 补齐：自升级导入本地包与任务查询

- 新增 `stx self-upgrade import <file>`：导入本地升级包，让内网、离线用户也能用 CLI 完成一键升级。
- 新增 `stx self-upgrade task get <id> [--wait]`，可等待升级任务结束，失败时返回非零退出码；
  并新增 `stx self-upgrade task cancel <id>`。
- 现状：v1.4.0 与当前 main 的这两个操作都只有接口、没有 CLI 命令，违反"新增接口要补 CLI 能力"的约定。

### R5 发布说明

- 正式版 tag 必须带人工撰写的发布说明，包含"升级须知 / 破坏性变更"；缺失时发布失败。
- 自动生成的 PR 列表附在人工说明之后。

### R6 文档

- 补齐 `docs/内置监控说明.md` 的「从旧版本升级」章节：一键升级与 `install.sh` 两条路径的差异、删除的接口、
  内存与磁盘、时区、Agent 升级、Docker 孤儿容器、失效配置提示。

## Acceptance Criteria

- [ ] 在装有 v1.4.0（含三件套）的机器上一键升级到当前版本：
  - [ ] 3 分钟内三件套进程全部退出，9090 / 9093 / 3000 不再监听；
  - [ ] 再执行 `systemctl restart stx` 后三件套也不会再起来；
  - [ ] `observability/` 目录仍在。
- [ ] 升级后 `bin/start.sh` 与仓库 `install/bin/start.sh` 字节一致，`bin/lib/observability.sh` 不存在，旧文件有备份。
- [ ] 升级前通过 v1.4.0 webhook 写入的 firing 告警，升级后不再出现在 `status=firing` 的告警实例列表中，
      且没有产生恢复通知投递记录。
- [ ] 升级前创建的 `source_type=remote_alertmanager` 通知路由、含 `PROM_CPU_HIGH` 条件的巡检策略：
  - [ ] 升级后 API 返回非空的 `invalid_reason`；
  - [ ] 界面有提示；
  - [ ] 启动日志各有一条 WARN。
- [ ] 新工作流在 PR 上可运行并通过；当最新正式版已不含三件套时，相关断言被跳过而不是失败。
- [ ] 工作流里的登录、植入、check、execute 走 FROM 的 CLI，升级后的接口断言走 TARGET 的 CLI。
      退回接口调用的地方只有 FROM 缺命令的那几步（v1.4.0 为导入和任务查询），并且都有 `::notice::` 说明。
- [ ] 升级后不重新登录，CLI 调用仍然成功。
- [ ] `stx self-upgrade import` / `task get --wait` / `task cancel` 可用；
      `task get --wait` 在任务失败或超时时返回非零退出码；operation 契约测试通过。
- [ ] `release-on-tag` 在正式版 tag 缺少发布说明文件时失败；有文件时 Release 正文 = 人工说明 + 自动 PR 列表。
- [ ] 文档补齐 R6 所列各项。

## Out of Scope

- `install.sh` 重装升级路径的 CI 覆盖（后续可作为同一工作流的第二个矩阵项）。
- 历史 Prometheus 数据迁移到内置 tsdb。
- 失效配置的自动改写 / 旧值别名（见 Locked Decision 2）。
- Docker / Kubernetes 部署的升级自动化，这次只更新文档。
- 版本号定为 v1.5.0 还是 v2.0.0，打 tag 时由用户决定。
