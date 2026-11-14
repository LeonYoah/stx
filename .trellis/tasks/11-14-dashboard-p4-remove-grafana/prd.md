# P4 移除 Grafana 安装与代理

父任务：`11-14-home-monitoring-dashboard`。必须在 P2 上线后进行（首页已不依赖 Grafana）。

## Goal

从产品中彻底去掉 Grafana：代理、安装组件、面板 JSON、配置与文档，减少安装体积与维护面。

## Requirements

- 删除 `/api/v1/monitoring/proxy/grafana` 代理及前端 `MonitoringOverview` 中的 Grafana 相关代码。
- 安装器 / 离线包 / docker 部署去掉 Grafana（`install/observability/grafana_config`、`deploy/docker/observability/grafana`）。
- 配置 `observability.grafana.*` 标记废弃：仍可解析，启动时打一条 warn 日志，不报错。
- 保留 Prometheus 录制规则与 Alertmanager。
- 更新文档（`docs/observability-remote-design.md` 等）与相关 CLI / 路由基线 / swagger。
- 升级路径：已装 Grafana 的环境升级后不再管理它，给出清理说明（不自动删除用户数据）。

## Acceptance Criteria

- [ ] 全仓库 grep `grafana`（忽略大小写）只剩废弃配置兼容代码、升级说明与变更记录。
- [ ] 在线 / 离线安装、升级、监控中心、告警中心冒烟通过。
- [ ] 旧配置文件含 `observability.grafana` 时 STX 正常启动并给出废弃提示。
