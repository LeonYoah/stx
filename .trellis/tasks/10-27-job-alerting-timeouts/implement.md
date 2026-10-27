# 执行计划：父任务

父任务本身不写代码，负责三件事：确定子任务的执行顺序、做跨子任务验收、做最终的集成检查。

## 顺序
1. 启动 `10-26-submit-timeout-recovery`：先 `task.py start`，然后按它自己的 implement.md 执行，开 PR，合并。
2. 启动 `10-27-job-alerts`：从合并了子任务 1 的 main 切分支，开 PR，合并。
3. 启动 `10-27-engine-query-resilience`：开 PR，合并。如果出现急需的情况，可以和第 2 步对调，对调时按它 design §6 的说明迁移 D1 信号。

每个子任务都要单独完成以下几步：`trellis-check`、`trellis-update-spec`、t1 实测，开 PR，跟踪 CI。

## 集成检查（3 个子任务都合并之后）
- [ ] 用 main 构建，部署到 t1，按 memory 中 stx-deploy-upgrade 的步骤执行。
- [ ] 按父任务 prd 的"跨子任务验收"做一遍联调。
- [ ] 核对 spec 之间的交叉引用：`job-status-reconciler.md` ↔ `sync-alerts.md` ↔ `dolphin-scheduler-shell.md`。
- [ ] 清理 t1 上的测试数据：延迟代理、临时渠道和策略、测试任务。
- [ ] 用 `/trellis:finish-work` 归档父任务和三个子任务。
