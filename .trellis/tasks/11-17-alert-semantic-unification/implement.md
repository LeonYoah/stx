# 告警语义统一 · 执行计划

在 `fix/cleanup-observability-compat-ui`（PR #146）上按顺序提交，每步一个 commit，便于审查和回滚。
每步开始前按 AGENTS.md 查对应 spec；新增 / 修改的注释中英双语，中文在前。

## Step 1 · 告警类别枚举与存储种类拆分（后端）

- [ ] `model.go` 换新枚举（design §1.1）；新增 `alertStoreKind` 并改 `parseAlertSourceKey` 与确认 / 静默 / 关闭分支（§1.2）。
- [ ] 派发消息加 `Kind`，`alert_policy_execution_service.go` 580 / 601 改按 `Kind` 判断（§1.2）。
- [ ] 各构造点按 §1.3 表赋类别；`ListAlertInstances` 过滤按 §1.4；handler / 路由校验与 `matchRemoteAlertRoute` 按 §1.5。
- [ ] CLI 描述与样例值（design §5 第二条）。
- 验证：`go vet ./internal/apps/monitoring/...`；`go test ./internal/apps/monitoring/... ./internal/operation/...`
  （机器内存紧张时只跑这两个包，全量交给 CI）。
- 回滚点：本 commit 独立可回退。

## Step 2 · 规则键与告警规则目录（后端 + CLI）

- [ ] 指标告警 `RuleKey` 取 `template_key`（§2.1）。
- [ ] `AlertRuleCatalog` + `GET /api/v1/monitoring/alert-rules/catalog` + `monitoring.alert-rule.catalog` 操作登记（§2.2）。
- [ ] `validate_test.go` 操作计数 +1。
- 验证：同上，外加 `go run main.go` 起服务后 `stx monitoring alert-rule catalog` 输出三类（可选，本地内存允许时）。

## Step 3 · 巡检触发条件收敛（后端 + CLI）

- [ ] 模型删码删类、加 `RuleKeys`、改 `ALERT_FIRING` 默认值（§3.1）。
- [ ] 创建 / 更新校验（§3.2），依赖 Step 2 的目录函数。
- [ ] 检查器改名与重写匹配，删除别名相关函数（§3.3）；本地化（§3.4）。
- [ ] CLI `--alert-rule-key`（§3.5）。
- 验证：`go test ./internal/apps/diagnostics/... ./internal/cmd/...`。

## Step 4 · 前端

- [ ] 类型、告警中心筛选与标签、`isSyncEpisode`（§4.1）。
- [ ] `AutoPolicyConfigPanel` 规则多选、阈值 / 窗口文案、序列化与旧码丢弃（§4.2）。
- [ ] i18n 新旧键与术语统一（§4.3）。
- 验证：`cd frontend && pnpm exec tsc --noEmit && pnpm test`（不要在 8G 机器上跑 `next dev` / `next build`）。

## Step 5 · 文档、spec、PR 描述

- [ ] 按 design §5 更新 spec 与 `docs/内置监控说明.md`。
- [ ] PR #146 描述追加「破坏性变更」小节，内容取 design §6 表格。
- [ ] 全局再搜一遍残留：
  `rg -n "local_process_event|remote_alertmanager|PROM_(GC|HEAP|CPU)|NODE_UNHEALTHY|CheckPrometheusPolicies|本地进程事件|指标信号" -g '!*.pb.go'`
  期望结果：除本任务目录与 CHANGELOG / release note 外为空。

## 审查门槛

- Step 1 合入前需确认：集群级 sync 阶段在告警中心显示为「平台健康」，且仍保留发生次数 / 暂停提醒等展示，仍走退避重试。
- Step 3 合入前需确认：没有任何基于文本子串的告警匹配残留。
