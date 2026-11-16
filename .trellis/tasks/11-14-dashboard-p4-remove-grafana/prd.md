# P4 首页大盘收尾：Grafana 应用层摘除 + 内嵌可观测口径对齐

父任务：`11-14-home-monitoring-dashboard`。2026-11-16 因「内嵌可观测」任务（`11-16-embedded-observability`）重新定义。

## 为什么重定义

原 P4 = 「删 Grafana 代理 + 安装组件 + 面板 JSON + 配置废弃告警，保留 Prometheus 录制规则与 Alertmanager」。
内嵌可观测任务落定后，这些前提变了：

- Prometheus / Alertmanager / Grafana 三件套的进程、端口、安装、docker、文档由内嵌任务**一次性全量移除**
  （其 Locked Decision 6）。安装脚本里三件套交织在同一批文件（`install/bin/lib/observability.sh`、`render.sh`、
  `package-release.sh`、`deploy/docker/observability/`），分两次改只会制造冲突。
- `prometheus:` / `alertmanager:` / `grafana:` 配置段终态全删，升级按破坏性变更处理（其 Locked Decision 7 / 8），
  不再需要「废弃但可解析 + warn」。配置加载用 `viper.Unmarshal`（非 Exact），旧配置文件里残留的 `grafana:` 段会被忽略，不影响启动。
- 「保留录制规则与 Alertmanager」与内嵌方案冲突（录制规则内联进告警评估，AM 退役）。
- 大盘查询从外部 Prometheus 改走进程内引擎：文案、规范、查询负载、数据核对都要按新口径重做一遍，这是大盘任务自己的收尾。

## 分工（与内嵌任务的边界）

| 事项 | 归属 |
|---|---|
| Grafana 反向代理（handler / 路由 / 日志降噪中间件）、`observability.grafana` 配置结构与默认值 / 校验 / 环境变量 | **本任务** |
| 前端 Grafana 残留：`MonitoringOverview.tsx`（P2 后已无引用）及其专用 i18n / 测试、`HomeDashboard` 过时注释 | **本任务** |
| Grafana 面板 → STX 大盘的覆盖核对表（删面板 JSON 前的依据） | **本任务** |
| 大盘空态文案、spec、查询负载保护、内嵌口径下的复验 | **本任务** |
| integration status 的 grafana / prometheus / alertmanager 组件 | 内嵌 P2 / P3（组件集改版） |
| 安装器、离线包、docker 中的 Grafana（`grafana_config/`、面板 JSON、compose 服务）及三件套文档 | 内嵌 P4（与 prom / AM 一起删） |

## Requirements

- R1 删除 `/api/v1/monitoring/proxy/grafana` 代理与相关中间件分支、路由基线、swagger；`observability.grafana` 配置结构、默认值、
  校验、环境变量覆盖与 `config.example.yaml` / e2e 配置中的该段一并删除。
- R2 删除前端 `MonitoringOverview.tsx` 及只被它使用的 i18n 键与测试；清理 `HomeDashboard` 注释中的 Grafana 过渡说明。
- R3 产出覆盖核对表：两份 Grafana 面板（P0–P4）逐面板对应到「集群页签已覆盖 / E1 覆盖 / 告警覆盖 / 不再需要（附理由）」，
  写入本任务 `research/grafana-parity.md`；有缺口的关键信号补进 E1 PRD。内嵌 P4 删面板 JSON 以此为依据。
- R4 大盘文案按内嵌语义改写：不再出现「接入 Prometheus」；`reason=observability_disabled` 提示开启可观测性，
  `reason=prometheus_unavailable` 提示「内嵌监控存储未就绪 / 稍后重试」。接口的 `source` / `reason` 取值不变（CLI 与前端兼容）。
- R5 查询负载保护：首页集群 / 作业大盘接口加进程内短时响应缓存（≤15 秒，按参数为键；`viewable` 等逐用户字段在缓存外装饰），
  多人同时打开首页时不放大进程内 PromQL 计算。
- R6 spec 更新：`frontend/ui-conventions.md` §16 与 `backend/job-status-reconciler.md` 中「外部 Prometheus / HTTP SD /
  录制规则可能未加载」等表述改为内嵌口径。
- R7 内嵌口径下复验 P1–P3 的跨子任务验收（时间窗切换、集群范围、非管理员、关闭可观测性、390px / 英文），
  并在内嵌 P2 与 P3 之间（外部 Prometheus 与内嵌抓取并存）做一次抽样一致性核对。

## Dependencies

- 依赖内嵌 **P2** 已合并：大盘要有数据才能复验；integration status 的 grafana 组件随 P2 组件集改版移除。
- 与内嵌 **P3** 无先后要求；R7 的抽样核对必须在 P3（外部 Prometheus 退役）之前做。
- 内嵌 **P4** 删 Grafana 安装物时需要本任务的 R3 核对表。

## Acceptance Criteria

- [ ] `grep -ri grafana internal/ frontend/{app,components,lib}` 不再命中（变更记录与内嵌 P4 待删的安装物除外）。
- [ ] 旧配置文件保留 `observability.grafana:` 段时 STX 正常启动。
- [ ] `research/grafana-parity.md` 覆盖两份面板的全部面板，缺口已进 E1 或注明不需要。
- [ ] 首页三个页签在 `observability.enabled=false`、内嵌存储未就绪、正常三种状态下文案正确（zh / en）。
- [ ] 同参数并发请求集群 / 作业大盘，15 秒内只触发一轮 PromQL（单测 + 日志验证）；`viewable` 仍按当前用户。
- [ ] 抽样一致性：同一窗口内嵌与外部 Prometheus 的全局吞吐、节点堆 / CPU、Top N 差异在抓取对齐误差内（记录数值）。
- [ ] 路由基线、swagger、CLI 帮助中不再出现 Grafana 代理。
