# 实施计划：引擎查询韧性

**分支**：`fix/engine-query-resilience`，从 main 切出。
**顺序**：建议在子任务 1 之后做，这样可以复用它的错误分类。和"作业告警"谁先谁后都可以，具体看 design §6。

## 0. 开工前
- [ ] 加载 `trellis-before-dev`，阅读：`backend/index.md`、`job-status-reconciler.md`、`logging-guidelines.md`、`quality-guidelines.md`、`engine-events-and-realtime.md`。
- [ ] 确认 #71 已经合并，事件模式的间隔默认关闭。

## 1. 配置（检查点 A）
- [ ] 在 `SyncConfig` 中新增 3 项，同步到 `config.example.yaml`。
- [ ] `NewSeaTunnelEngineClient` 读取请求超时配置。
- [ ] 写单测。

## 2. 止损（检查点 B）
- [ ] 实现 `isEngineTransportError`，如果子任务 1 已有可复用的分类函数，就复用。
- [ ] `reconcileClusterJobs` 和 `reconcileClusterJobsLight` 改为返回 outcome，并加入止损逻辑。
- [ ] 写单测：止损后不再发起后续调用，用调用计数来断言。

## 3. 调度器与退避（检查点 C）
- [ ] 新建 `job_reconciler_scheduler.go`，包含 state、tryDispatch、finish、clusterInBackoff。
- [ ] `reconcileActiveJobs` 改用调度器分派任务。
- [ ] `refreshJobInstance` 接入退避判断。
- [ ] 添加日志：只在进入和退出退避时各打一条。
- [ ] 写单测，并带 `-race` 运行。

## 4. 与作业告警对齐（检查点 D）
- [ ] 如果"作业告警"已经合并：把 D1 的 Observe/Resolve 迁到 `finish`，原有测试改为断言 `finish` 的行为。
- [ ] 如果还没合并：在 `finish` 留好接入点，并在"作业告警"的 implement.md 里补一条说明。

## 5. 自检、spec、实测（检查点 E）
- [ ] `trellis-check`；`go test ./internal/...`；`go test -race ./internal/apps/sync/`。
- [ ] `trellis-update-spec`：在 `job-status-reconciler.md` 中补充并行、止损、退避、配置项的说明，包含 7 段式里的错误矩阵和测试点。
- [ ] t1 实测：把一个测试集群的引擎地址指向延迟代理，让 running 列表请求卡 20 秒。检查另一个集群的作业状态是否照常更新，日志里是否出现退避开始和恢复。**不要重启 SeaTunnel**。
- [ ] 提交信息用中文；开 PR；跟踪 CI。
