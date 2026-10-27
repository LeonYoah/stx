# 实施计划：提交与停止结果未知 + 超时配置

分支：从 `origin/main` 切出 `fix/submit-timeout-recovery`。开 PR 前确认 #71 的状态，需要时 rebase；改动的函数和 #71 不重叠。

## 0. 开工前
- [ ] 加载 `trellis-before-dev`，阅读以下 spec：
  - backend：`index.md`、`job-status-reconciler.md`、`error-handling.md`、`logging-guidelines.md`、`database-guidelines.md`、`execution-security-guidelines.md`、`dolphin-scheduler-shell.md`、`cli-guidelines.md`、`quality-guidelines.md`
  - frontend：`index.md`、`ui-conventions.md`
  - `guides/cross-layer-thinking-guide.md`
- [ ] `git fetch && git switch -c fix/submit-timeout-recovery origin/main`

## 1. 错误分类（检查点 A）
- [ ] `engine_client.go`：
  - 新增 `ErrEngineCallOutcomeUnknown`、`EngineCallUnknownError` 和判定函数。
  - `Submit`、`submitV1`、`StopJob`、`stopJobV1` 都接入分类，按 design §3 处理。
  - `shouldFallbackToLegacy` 去掉 504。
- [ ] 单测 `engine_client_outcome_test.go`。
- [ ] `go test ./internal/apps/sync/ -run 'Submit|Stop|Legacy|Outcome'`

## 2. 超时配置（检查点 B）
- [ ] `internal/config/model.go`：新增 `SubmitTimeoutSeconds`、`StopTimeoutSeconds`。
- [ ] `config.example.yaml`：补上示例和说明。
- [ ] `submit_outcome.go`：新增 `effectiveSubmitTimeout`、`effectiveStopTimeout` 和相关常量。
- [ ] `validateTaskDefinition`：校验两个覆盖值的范围。
- [ ] 任务响应增加 `timeout_defaults` 字段。
- [ ] 单测：生效值的计算、范围校验。

## 3. 提交登记、ctx 解绑、拦截（检查点 C）
- [ ] `submit_outcome.go`：
  - `recordUnknownSubmitOutcome`
  - 辅助函数和 `guardSubmitOutcomePending`
  - 提交超时的专用文案
  - 文件带 License 头，注释中英双语
- [ ] `service.go`：
  - 接入登记。
  - 成功分支写入 `stop_timeout_seconds`。
  - `submitNewTask` 和 `RecoverJob` 接入拦截。
  - 用 `WithoutCancel` 解绑 ctx。
- [ ] `task_schedule.go`：接入拦截。
- [ ] `execution.go`：submit、recover、cancel 三处用 `WithoutCancel`。
- [ ] `repository.go`：新增 `ListPendingJobInstancesByTaskID`；有接口或 fake 的一起补上。
- [ ] 错误定义：新增 `ErrSubmitOutcomePending`；handler 映射为 409；审计 details 带上 `submit_outcome`。
- [ ] 单测。

## 4. 对账确认（检查点 D）
- [ ] `job_engine_sync.go`：确认规则，以及恢复提交在截止时间前不采信终态。
- [ ] `job_reconciler.go`：
  - 孤儿分支加截止判断和过期处理。
  - cancelling 超过截止时间时写入 `cancel_overdue_at`。
  - 留出接入点函数，先不做实现。
- [ ] 单测。

## 5. 停止（检查点 E）
- [ ] `CancelJob`：
  - 写入 `cancel_requested_at`、`cancel_timeout_seconds`、`cancel_deadline`。
  - 结果未知时不 restore。
  - 写入 `cancel_outcome`。
- [ ] 作业响应增加计算字段 `cancel_overdue`。
- [ ] 单测。

## 6. CLI（检查点 F）
- [ ] `sync_job_wait.go` / `sync.go`：
  - submit、recover、cancel 遇到结果未知时输出 warning。
  - 补一个 `--definition-file` 写入超时字段的例子和单测。
- [ ] `go test ./internal/cmd/`

## 7. 前端（检查点 G）
- [ ] `SettingsSidebarPanel.tsx`：新增"超时"分组和校验。
- [ ] `sync-studio-utils.ts`：新增 4 个工具函数，配单测。
- [ ] `DataSyncStudio.tsx`：结果未知时显示警告提示。
- [ ] 作业状态徽标：显示"提交确认中"和"停止超时"。
- [ ] `zh.json` 和 `en.json`。
- [ ] `pnpm exec tsc --noEmit && pnpm test`；改动的文件跑 lint。

## 8. Swagger、spec、自检（检查点 H）
- [x] Swagger：提交、恢复、停止三个路由原本就没有 Swagger 注解，本任务也没有新增路由，所以不需要重新生成 Swagger，`route_baseline.json` 也不用改（2026-10-27 核实）。
- [ ] 加载 `trellis-check` 做全量自检，跑 `go test ./internal/...`、`check_license.py`、`check_db_compatibility.py`。
- [ ] 加载 `trellis-update-spec`，更新以下 spec：
  - `job-status-reconciler.md`：新增"写操作结果未知"一节，按 7 段式写，覆盖提交和停止。
  - `dolphin-scheduler-shell.md`：409 和接管规则。
  - `engine-events-and-realtime.md`：结果未知时不登记事件预期。

## 9. t1 实测（检查点 I）
- [ ] 部署分支构建。
- [ ] 在 STX 和引擎之间放一个延迟代理：`/submit-job`、`/stop-job` 转发后延迟 20 秒再回复，其余请求直接透传。把测试任务的 `engine_base_url` 临时指向代理。**不重启 SeaTunnel 集群。**
- [ ] 验证以下几项：
  - 作业显示"提交确认中"，之后转为运行中。
  - 重复提交返回 409。
  - 停止时显示"停止请求超时，正在确认"，之后变为已取消。
  - 任务覆盖值设为 1 分钟，并让代理丢弃提交请求：1 分钟后作业变为失败，错误信息是提交超时的专用文案。
- [ ] 清理测试环境。

## 10. 提交与 PR
- [ ] 中文提交信息，末尾加 Co-Authored-By。PR 描述末尾加署名。
- [ ] 跟踪 CI。

## 回滚点
- 检查点 A 到 E 是后端，可以单独回滚；前端和 CLI 的提示不影响后端行为。
