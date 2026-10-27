# 实施计划：作业告警第一期

**前置条件**：子任务 `10-26-submit-timeout-recovery` 已合并到 main，或者基于它的分支开工。分支名 `feat/job-alerts`。

## 0. 开工前
- [ ] 加载 `trellis-before-dev`，读以下规范：
  - 后端：`index.md`、`database-guidelines.md`、`error-handling.md`、`logging-guidelines.md`、`execution-security-guidelines.md`、`cli-guidelines.md`、`quality-guidelines.md`、`job-status-reconciler.md`、`engine-events-and-realtime.md`
  - 前端：`index.md`、`ui-conventions.md`、`api-and-services.md`
  - `guides/cross-layer-thinking-guide.md`
- [ ] 读 monitoring 现有测试（`alert_policy_execution_service_test.go`、`node_offline_evaluator_test.go`），搞清楚 fake 仓储和时钟是怎么注入的。

## 1. monitoring：数据模型与投递重构（检查点 A）
- [ ] 新增 `SyncAlertEpisode` 实体并注册迁移；给 `AlertPolicy` 加 `TaskID` 和 `NotifyTaskOwner` 两个字段。
- [ ] 新增 `AlertSourceTypeSync`、`AlertPolicyBuilderKindSyncJob` 和 6 个规则键常量。
- [ ] 抽出 `dispatchAlertDelivery` 和 `alertMessage`，本地事件改走这条路径。**先跑通现有测试再继续**。
- [ ] 跑 `go test ./internal/apps/monitoring/`。

## 2. monitoring：hub、sweeper、默认规则、模板（检查点 B）
- [ ] 新建 `sync_alerts.go`，包含 hub、阶段的增改查、四种 Resolve 范围、D1 窗口判定和 sweeper。
- [ ] `defaultClusterRules` 增加 D1、D2 两条规则；`allAlertPolicyTemplateSummaries` 增加两个模板。
- [ ] 告警中心增加 `buildSyncAlertInstances`，`parseAlertSourceKey` 支持 `sync:` 前缀。
- [ ] 策略列表支持按 `policy_type`、`task_id` 过滤；`sync_job` 策略按任务唯一，重复时返回 409。
- [ ] 实现 `TaskAlertPolicyStore`。
- [ ] 补单测。

## 3. sync：信号与 API（检查点 C）
- [ ] 新建 `alert_signals.go`：定义接口、信号结构和工具函数；在 service 上加 setter。
- [ ] 在设计文档 §3 列出的各个位置发出信号，并补单测，用 fake reporter 断言发出的信号。
- [ ] 任务告警 API：handler、service、权限、校验、审计和确认头；在 `registry.go`、`route_baseline.json` 登记；写 Swagger 注解。
- [ ] `router.go` 注入 reporter、store、task name resolver。
- [ ] 跑 `go test ./internal/apps/sync/ ./internal/apps/monitoring/ ./internal/operation/...`。

## 4. CLI（检查点 D）
- [ ] `stx sync task alert get|set`（停用用 `set --disable`，不提供 delete）；`alert-policy list` 加上过滤参数。
- [ ] 跑 `go test ./internal/cmd/`。

## 5. 前端（检查点 E）
- [ ] 在 service 层新增任务告警的 get、put、delete，以及对应的类型定义。
- [ ] 新建 `TaskAlertForm` 组件，加入任务设置侧栏的"告警"分组，包含摘要和展开逻辑。
- [ ] 策略中心：新增"作业告警"筛选，模板分类加入"作业观测"。
- [ ] 告警中心：显示 sync 来源的告警，并能跳转到对应作业。
- [ ] `/sync?task=&job=` 深链。
- [ ] 中英文 i18n。
- [ ] 跑 `pnpm exec tsc --noEmit && pnpm test`，并对改动的文件跑 lint。

## 6. 自检与 spec（检查点 F）
- [ ] 加载 `trellis-check`，全量自检：
  - `go test ./internal/...`
  - `check_license.py`
  - `check_db_compatibility.py`
  - 契约报告
- [ ] 加载 `trellis-update-spec`：
  - 新增 `backend/sync-alerts.md`，按 7 段式写：规则目录、信号契约、阶段生命周期、投递、API。
  - 更新 `backend/index.md`。
  - 前端如果有新约定，更新 `frontend/` 相应文档。

## 7. t1 实测（检查点 G）
- [ ] 在 t1 上用 Python 起一个临时 webhook 接收端，记录收到的请求体；在 STX 里配成 webhook 渠道。
- [ ] 给一个测试任务配置告警，依次验证：
  - B1：写一个错误配置的批作业，触发失败，再修好重跑，确认先收到告警、再收到恢复。
  - A2、C1：借用子任务 1 的延迟代理。
  - D1：让测试集群的端点暂时指向一个不可达的地址。**不要停 SeaTunnel**：在 STX 侧临时改集群的 API 地址，或者用代理返回错误。
- [ ] 验证邮件渠道（如果 t1 有可用的 SMTP；没有就只验证收件人合并的单测）。
- [ ] 清理：删除临时渠道和策略，停掉接收端。

## 8. 提交与 PR
- [ ] 提交信息用中文，按检查点分几次提交；PR 描述末尾带署名。跟踪 CI。

## 回滚点
- A：投递重构单独提交，便于单独回滚。
- B、C：新增功能，回滚后旧版本会忽略新加的表和列。
