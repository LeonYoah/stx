# 巡检记录清理（全局保留策略 + 批量删除级联）

## 背景

巡检记录（`diagnostics_inspection_reports`）和其触发的诊断包任务（`diagnostics_tasks`）
目前只增不删：自动巡检跑久了报告与磁盘诊断包（`storage/diagnostics/tasks/<id>/`）会无限
堆积，用户也无法手工删掉不想要的记录。

用户在对话中确认的方向（已锁定，不再讨论）：

1. **全局设置，覆盖自动和手动**：不再按每条自动巡检单独配保留策略，而是一套全局保留
   规则，对所有 `trigger_source`（auto / manual / cluster_detail /
   diagnostics_workspace）一视同仁。
2. **靠绑定保一致性**：诊断任务已通过 `source_ref.inspection_report_id` 绑定巡检记录，
   发现项通过 `report_id` 绑定。删除巡检记录时级联删除绑定物，就不会出现「报告没了
   诊断包还在 / 诊断包还在报告没了」的不一致。
3. **命名**：功能叫「巡检记录清理」，不叫「残留物清理」。

## 范围

### 做

1. **全局保留策略**：单行设置（保留 N 天 / 最多保留 N 份），默认 0/0 = 不清理；
   读取与更新走 API，前端在巡检面板提供设置入口。
2. **后台自动清理**：到期（超过保留天数）或超量（超过最大份数，删最旧）的巡检记录
   被级联删除；只清理终态（completed / failed）记录。
3. **手动批量删除**：巡检记录列表加勾选（页内全选 + 逐条勾选），浮动批量条触发删除，
   同样走级联；被阻止的记录在结果里说明原因。
4. **级联内容**（删除一条巡检记录时连带处理）：
   - `diagnostics_inspection_findings`（report_id 匹配）
   - `source_ref.inspection_report_id` 匹配的 `diagnostics_tasks` 及其子表
      （`diagnostics_task_steps` / `diagnostics_task_nodes` / `diagnostics_task_logs`）
   - 这些任务在磁盘上的 bundle 目录（`storage/diagnostics/tasks/<task_id>/`）
5. **删除守护**：
   - 状态为 pending / running 的巡检记录不可删除（接口拒绝并返回原因）；
   - 绑定的诊断任务处于非终态时，该记录本次不删并返回原因（不强行杀任务）。
6. **僵尸状态收尾（顺带修）**：进程重启后遗留的非终态对象永久卡死——
   巡检报告 pending / running（巡检评估是进程内同步执行，进程死了没人收尾）；
   诊断任务 pending / ready / running / cancel_requested / cancelling
   （`go executeDiagnosticTask` 异步执行同样随进程死亡）。启动时一次性 reconcile
   成终态（失败/取消），报告与任务不再永久卡 running，清理也能覆盖到它们。
   判定带宽限期（updated_at 早于进程启动/早于阈值才算僵尸），不杀启动后新创建的在途对象。
7. **CLI 对齐**：巡检记录批量删除命令 + 清理设置的读/写命令。
8. i18n（中/英）、审计、E2E 与单测同步。

### 不做

- 与巡检记录无绑定的诊断任务（error_group / alert / manual 来源且
  `inspection_report_id=0`）不纳入清理——「巡检记录清理」只管巡检域。
- `executions` 共享操作记录不删：它是操作历史，`result_ref` 悬挂与监控侧
  `policy_id` 悬挂同约定。
- 不做单条删除按钮：删除只走勾选 + 批量条（用户明确「列表做选中设计」）。
- 不恢复/软删：硬删，弹窗确认即生效。

## 验收标准

- [ ] AC1 保留策略 API：GET 返回 `{retention_days, max_reports}`（默认 0/0）；
      PUT 校验非负整数并持久化；非法值返回 4xx。
- [ ] AC2 自动清理：设置保留 1 天后，创建时间早于 1 天的终态巡检记录在下一轮 sweep
      被删除，其 findings / 绑定诊断任务（steps / nodes / logs）/ bundle 目录一并消失；
      pending / running 记录不受影响。
- [ ] AC3 超量清理：设置最多保留 N 份后，超出部分按创建时间最旧优先删除；同一记录同时
      命中到期与超量只删一次。
- [ ] AC4 批量删除接口：提交一组 id，终态且无运行中绑定任务的记录被级联删除；
      返回逐条结果（deleted / skipped_running / skipped_task_running / not_found）；
      磁盘 bundle 目录同步移除。
- [ ] AC5 前端列表：出现勾选列与页内全选；勾选后浮动批量条显示已选数量与删除按钮；
      删除完成后列表刷新、清空选择、结果用 toast 汇总（含被跳过原因）。
- [ ] AC6 前端设置入口：巡检面板可打开「巡检记录清理」设置，读写两个值，保存后立即生效
      （下一轮 sweep 按新值执行）。
- [ ] AC7 CLI：`stx diagnostics inspection cleanup get|set`（set 走
      `--request-file` + `--confirm`）可用，行为与 API 一致；按 cli-guidelines §11，
      CLI 不提供删除入口，批量删除只有 API + 网页。
- [ ] AC8 审计：批量删除写一条审计记录（操作者、id 列表、结果计数）。
- [ ] AC9 僵尸收尾：重启前插入 running 报告 / running 任务（含非终态 steps、node
      executions）的库，进程启动后被一次性置为终态并写明中断原因；启动后新创建的
      在途对象不受影响。
- [ ] AC10 验证：`go test ./internal/apps/diagnostics/...`、前端相关单测、
      `pnpm exec tsc --noEmit`、E2E 巡检用例通过。
