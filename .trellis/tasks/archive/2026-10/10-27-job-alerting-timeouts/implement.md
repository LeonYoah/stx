# 执行计划：父任务

父任务本身不写代码，负责三件事：确定子任务的执行顺序、做跨子任务验收、做最终的集成检查。

## 顺序
1. 启动 `10-26-submit-timeout-recovery`：先 `task.py start`，然后按它自己的 implement.md 执行，开 PR，合并。
2. 启动 `10-27-job-alerts`：从合并了子任务 1 的 main 切分支，开 PR，合并。
3. 启动 `10-27-engine-query-resilience`：开 PR，合并。如果出现急需的情况，可以和第 2 步对调，对调时按它 design §6 的说明迁移 D1 信号。

每个子任务都要单独完成以下几步：`trellis-check`、`trellis-update-spec`、t1 实测，开 PR，跟踪 CI。

## 集成检查（3 个子任务都合并之后）
- [x] 用 main 构建，部署到 t1，按 memory 中 stx-deploy-upgrade 的步骤执行。（三个子任务合成一个 PR #75，squash 为 c079da0；t1 运行的 `1.2.0-alerts.269a37c-fix2` 与 main 代码一致，main 比它只多了 spec 文档）
- [x] 按父任务 prd 的"跨子任务验收"做一遍联调。见下方完成记录；D1、D2、并行对账 / 退避没有做 t1 端到端。
- [x] 核对 spec 之间的交叉引用：`job-status-reconciler.md` ↔ `sync-alerts.md` ↔ `dolphin-scheduler-shell.md`（2026-10-27 补上 sync-alerts → dolphin / engine-events 的链接）。
- [x] 清理 t1 上的测试数据：延迟代理已停，临时渠道和策略已删，任务 13 已恢复为 v4（去掉代理和 1 分钟超时）；测试任务 14 `alert-smoke-batch` 保留。
- [x] 用 `/trellis:finish-work` 归档父任务和三个子任务。

## 完成记录（2026-10-27）
- 交付：PR #75（合并提交 c079da0），包含三个子任务。用户要求"做成一个 PR"，所以没有按原计划分三个 PR。
- t1 实测通过：提交超时确认中 / 409 / 过期判失败；停止结果未知保持停止中、停止超时标记；未配置告警不发；B1 只通知一次、累计、冷却提醒、批作业成功后恢复；A1 引擎拒绝与恢复；A2 过期且不重复报 B1、下一次提交成功后恢复；C1 停止超时与恢复；深链、任务侧栏、告警中心、策略中心界面。
- 实测发现并修复：批作业提交被误当成实时作业而解除告警；通知链接改用新增的 `app.console_url`（`external_url` 是后端地址，链接 404）；侧栏告警摘要撑宽卡片（同时写进 `frontend/ui-conventions.md`）。
- **未做 t1 端到端**：D1（作业状态获取失败）、D2（事件通道降级）、并行对账与退避。要让集群查询持续失败，只能临时改 STX 库里集群节点的 API 端口（通过接口改会改写 SeaTunnel 运行配置），属于修改线上共享资源，本次被拦下没有做；这部分由单测覆盖。以后需要时先征得用户同意。
- 第二期（B4 CDC 延迟告警）按父任务 prd 另建任务。
