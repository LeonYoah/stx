# 3.x 实时作业：事件 + 实时可观测性适配

## Goal

让 STX 在 SeaTunnel 3.x 集群上，为实时（STREAMING）作业提供以下能力：

- **看得见瓶颈**：DAG 上叠加忙碌度和背压，显示最近 10 分钟的曲线，并给出瓶颈提示。
- **状态更及时**：用引擎推送的作业状态事件触发即时刷新。
- **看得见 DDL 和 CDC 延迟**：作业事件时间线，加 CDC 延迟徽标。
- **压低引擎调用量**：事件通道健康时降低轮询频率。

任何一路不可用时，自动退回现有的 API 轮询和快照指标，行为与现在一致。

依据见 `research/engine-capabilities.md`、`research/stx-current-state.md`。

## Decisions（2026-10-25，用户确认）

- 可观测性默认开关：**流式作业默认开启，任务可以关闭**；批作业默认不开，可以手动打开。
- 事件通道网络方向：**master 直连 STX**（`app.external_url`）。每个集群一个 token，用 HMAC 派生，不落库。
- 范围：**P1 + P2**。P3（降采样持久化、背压和延迟告警、stain trace）不做。
- 单个 Trellis 任务。

## Requirements

### R1 能力门控
- 按集群版本判断：实时可观测性要求 ≥3.0.0；作业状态事件要求 ≥2.3.12（只有终态）；非终态事件要求 ≥3.0.0。
- 不满足条件时，相关功能不出现、不注入、不改配置，并通过 API 返回明确的原因。

### R2 提交时注入可观测性
- 集群模式、集群版本 ≥3.0.0，并且任务设置判定为开启时，在提交内容里注入 `env.engine.observability`（`enabled=true, bucket_ms=5000, retention_minutes=10`）。
- 任务设置 `observability`：`auto`（默认，流式开、批量关）| `on` | `off`。
- 用户在作业内容里自己写的 observability 配置优先，包括显式的 `enabled=false`。
- 本地执行和预览不注入。
- 实际提交的内容（`submitted_content`）里能看到注入的部分；`submit_spec.observability` 记录注入结果。

### R3 实时指标代理
- `GET /api/v1/sync/jobs/:id/realtime?window_ms=` 返回节点和边的时间序列加瓶颈提示。
- 不可用时返回 `available=false` 和稳定的 `reason`：
  - `version_unsupported`、`not_cluster_mode`、`not_running`、`not_enabled`、`master_unreachable`、`no_engine_job`、`collecting`（已开启但还没采到第一个桶）。
- 自动找到 active master，非 master 节点返回 404 时换下一个节点。
- 同一作业的请求短时间内合并，避免多人查看时放大对引擎的请求。
- CLI：`stx sync job realtime <id>`。

### R4 事件通道
- 集群级开关：
  - `GET /api/v1/sync/event-channels/:cluster_id` 查看状态。
  - `PUT /api/v1/sync/event-channels/:cluster_id` 开启、关闭、轮换 token。
- 开启时把 `event-report-http`（url、Authorization header；≥3.0.0 时加 `report-non-terminal-job-state: true`）写进该集群的 seatunnel.yaml 模板和每个节点配置，并提示需要滚动重启。
- 关闭时移除这些配置。
- 接收：`POST /api/v1/sync/engine-events/:cluster_id`。
  - token 错误返回 401。
  - **内容合法与否都返回 2xx**，避免引擎堵塞。
  - 限制请求体大小。
- 清洗：
  - 去重（引擎没有事件 ID，换主后会重放）。
  - 丢弃 `STAIN_TRACE`。
  - **删除 `READER_MESSAGE_DELAYED.record`（行数据）**。
  - 生命周期事件数量少，入库，但时间线里默认折叠。
  - 保留 7 天。
- 分发：
  - `JOB_STATUS`：防抖后触发该作业的一次刷新，复用现有刷新和终态指标逻辑。
  - schema change 事件：写入时间线。
  - `READER_MESSAGE_DELAYED`：更新作业的 CDC 延迟摘要，时间线降采样记录。
- 通道健康：
  - 记录最近一次收到事件的时间。
  - 提交到 3.x 集群的作业在规定时间内没收到预期的状态事件时，判定通道降级。
- 时间线：`GET /api/v1/sync/jobs/:id/events`。
- CLI：通道状态、开关、时间线都要有对应命令。

### R5 自适应轮询
- 通道健康、版本 ≥3.0.0 的集群：
  - 列表对账降到 60 秒一次（可配置）。
  - 运行中作业的 job-info：被查看时（近 30 秒内有人读过）保持 10 秒，否则 30 秒。
- 通道降级、未开启或 2.3.x 集群：保持现有的 10 秒。
- 现有的对账语义（作业从引擎消失时置为 failed、终态补拉终态指标、`sync_state` 判定）保持不变。

### R6 前端
- **不自建背压 DAG 和曲线**（2026-10-25 用户确认）：SeaTunnel 3.0.0 自带的 Web UI 作业详情页（`#/jobs/:jobId`）已经实现了忙碌度、背压着色和 echarts 曲线，STX 已通过 `/api/v1/clusters/:id/webui/` 反向代理嵌入了这套 UI。
- **通用入口"跳转到 SeaTunnel 集群观测"**（2026-10-25 用户确认）：
  - 作业行（集群模式、有引擎作业 ID 的**实时和离线**作业都显示）、实时诊断面板、集群详情 Web UI 标签页共用一个组件。
  - 可以选择"通过 STX 代理打开"或"直连原生地址打开"。地址来自 `GET /api/v1/clusters/:id/webui-endpoints`，原生地址优先指向 active master。
  - 带作业时打开 `#/jobs/{engine_job_id}`。
- 作业 DAG 弹窗（运行中、3.x 集群）：显示 STX 计算的瓶颈提示和不可用原因，作为轻量面板，每 5 秒轮询一次，页面不可见时暂停。
- Web UI 反向代理改为优先指向 active master（复用 R3 的 master 定位），否则多 master 集群里实时部分会返回 404。
- 任务设置侧栏：集群模式下提供"实时可观测性：自动/开启/关闭"。
- 作业详情：事件时间线（状态、DDL、CDC 延迟点），以及 CDC 延迟徽标。
- 集群事件通道的状态和开关入口（含"需要滚动重启"提示）。
- 中英文切换，遵守 `ui-conventions.md`。

## Acceptance Criteria

- [ ] 2.3.x 集群提交的内容与现在逐字一致（没有注入）；3.0.0 集群上的流式任务提交后，`submitted_content` 带有 observability 配置。
- [ ] 用户在作业内容里写了 `engine.observability.enabled = false` 时，引擎侧最终仍为关闭。
- [ ] t1 3.0.0 集群跑 FakeSource 流式作业时，STX `/realtime` 返回的节点和边与直接请求引擎 `/metrics/realtime` 的数据一致；多节点时能找到 master（有单测覆盖）。
- [ ] 实时数据不可用的每一种情况都返回对应的 `reason`，前端显示原因，并退回快照指标。
- [ ] 从 STX 作业入口的深链能打开引擎 Web UI 对应作业的详情页，并能看到背压着色（注入生效）；多 master 集群里代理会找到 active master（有单测覆盖）。
- [ ] 开启事件通道后，seatunnel.yaml 模板和节点配置里有正确的 `event-report-http` 配置块（`report-non-terminal-job-state` 在它下面）；关闭后被移除；token 不出现在任何 API 响应和日志里。
- [ ] 接收端：token 错误返回 401；JSON 非法、未知事件类型、未知作业都返回 2xx；重复的整批只入库一次；`record` 字段不落库。
- [ ] 滚动重启后，提交、取消、失败一个作业，STX 状态在收到事件后 2 秒内刷新（不等下一轮对账），时间线里有对应的状态事件。
- [ ] STX 停机 1 分钟再恢复，补发的事件被正确去重。
- [ ] 通道健康时，对账器对该集群的列表调用降到 60 秒一次；人为让通道失效后（例如改错 token）能判定为降级，并回到 10 秒。
- [ ] CDC 作业出现延迟时，作业显示 CDC 延迟；发生 DDL 时，时间线里有 schema change 记录。
- [ ] 新接口都有 CLI；路由登记表测试通过。
- [ ] `go test ./...`、`(cd agent && go test ./...)`、前端 `tsc --noEmit`、`pnpm test`、lint 全部通过；`./scripts/test_db_compat.sh` 三库迁移通过。

## Out of Scope

- 实时指标的持久化和历史回看、告警模板、stain trace。
- 经 agent 转发事件。
- Application Mode 的观测（另见 `10-24-zeta-application-mode`）。
- 自定义 SPI EventHandler。
