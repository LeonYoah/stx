# 执行计划：作业详情页与 Flink 风格执行拓扑

开工前：`trellis-before-dev` 读 `.trellis/spec/frontend/index.md`（重点 ui-conventions §4/§7/§12/§13）、
`.trellis/spec/backend/index.md`（cli-guidelines §8、error-handling、quality-guidelines、engine-events-and-realtime）、
`.trellis/spec/guides/cross-layer-thinking-guide.md`。

## 阶段 1：后端计划 DAG 接口（可独立验证）
- [x] `internal/apps/sync/plan_dag.go`：`JobPlanDAGResult`、`GetJobPlanDAGForActor`、原因常量（中英双语注释）。
- [x] `handler.go` 新增 `GetJobPlanDAG`（swag 注释），`router.go` 注册 `GET /api/v1/sync/jobs/:id/plan-dag`。
- [x] `operation/registry_sync.go` 登记 `sync.job.plan-dag`（R0），CLI 示例 `stx sync job plan-dag 26`。
- [x] `plan_dag_test.go`：不可见 403；无 `submitted_content`；本地模式；resolver 报错 → `config_tool_unavailable`；
      inspect 报错 → `inspect_failed` 且 message 脱敏截断；成功时顶点字段与 `extractWebUIDAGNodes` 一致。
- [x] 验证：`go test ./internal/apps/sync/ -run 'PlanDAG' ./internal/operation/... ./internal/cli/...`；`go vet ./internal/apps/sync/`。

## 阶段 2：前端数据模型与依赖
- [x] `pnpm add @xyflow/react @dagrejs/dagre`（锁文件随提交）。
- [x] `services/sync`：`getJobPlanDag(jobId)` 与类型 `SyncJobPlanDag`。
- [x] `job-detail/dag-model.ts` + `__tests__/dag-model.test.ts`：归一化（对象 / 数组、字符串 ID）、按 ID 合并、
      按 type+tablePaths 兜底、metricPrefix 解析（`pipeline-1 [Source[0]-Postgres-CDC]` → `Source[0]`）、按表指标、
      实时叠加、`middleEllipsis`、`layoutDag` 无重叠且左→右。用作业 #26 的真实结构做夹具。
- [x] 验证：`pnpm exec vitest run components/common/sync/job-detail`。

## 阶段 3：画布与节点抽屉
- [x] `JobDagCanvas.tsx`（plan / runtime 模式、MiniMap、Controls、fitView、表名复制、实时着色与边标签、顶部提示条）。
- [x] `DagNodeDrawer.tsx`（概览 / 表结构 / 建表计划 / 指标），迁移 `WebUiDagPreview` 的表结构与 SaveMode 渲染。
- [x] 所有新文案走 i18n（zh / en 同步），不保留硬编码中文。

## 阶段 4：作业详情弹层与入口
- [x] `JobEventTimelineDialog.tsx` 拆出 `JobEventTimelineContent`，弹窗改为包一层（行为不变）。
- [x] `getJobActionState` 抽出，行内与详情共用。
- [x] `JobDetailDialog.tsx`：头部操作 + 6 个页签；日志 / Checkpoint 为插槽。
- [x] `ConsolePanel.tsx`：行点击打开详情、行内只留 4 个按钮、多表徽标打开指标页签；移除 `viewExecutionDag`。
- [x] `DataSyncStudio.tsx`：`detailJobId/detailTab` 状态、深链打开详情、日志与 Checkpoint 加载条件扩展、
      编辑器 DAG 弹窗改用 `JobDagCanvas mode="plan"`；删除 `WebUiDagPreview.tsx` 与已无用的弹窗状态。
- [x] `observabilityHint` zh / en 改为指向作业详情。
- [x] 验证：`pnpm exec vitest run components/common/sync lib/i18n`；`pnpm exec tsc --noEmit`；`pnpm lint`；`pnpm build`
      （确认 React Flow 只出现在按需 chunk 中）。

## 阶段 5：真实环境验证
- [ ] 本地前端连 t1 后端（或用户同意后部署到 t1），用作业 #26 / 新提交的 CDC 作业检查：行点击打开详情、表结构与建表计划、
      长表名省略与复制、忙碌度 / 背压着色、6 个页签、停止 / 恢复按钮、告警深链直达详情、编辑器 DAG 不回退。
- [ ] Playwright 截图留证（亮 / 暗主题、窄屏 1280）。

## 阶段 6：收尾
- [ ] `trellis-check` 全量自检；`trellis-update-spec`：前端 DAG 画布约定写入 frontend spec，后端 `plan-dag` 契约写入
      `engine-events-and-realtime.md` 或新文档。
- [ ] 中文提交，包含上一轮的 `observabilityHint` 文案修复；不提交与本任务无关的脏文件
      （`.claude/settings.local.json`、sync-alerts spec、`10-24-zeta-application-mode`）。

## 风险与回滚点
- `DataSyncStudio.tsx` 改动面：只做状态接线，阶段 4 前后各跑一次全量 sync 组件测试。
- 复杂作业（多 pipeline、transform、多表 Sink）顶点 ID 可能与运行时不一致：靠 type+tablePaths 兜底，并在阶段 5 用多表作业实测。
- 依赖体积：若 `pnpm build` 显示进了主 chunk，改为动态导入整个 `JobDetailDialog`。

## 实施记录（2026-10-27）
- 阶段 1–4 已完成：后端 `plan-dag` 接口 + CLI + Swagger + 路由基线；前端 `job-detail/`（dag-model / dag-layout / JobDagCanvas /
  OperatorList / DagNodeDrawer / JobTopology / JobDetailDialog）；运行记录行精简；深链直达详情；编辑器 DAG 换新画布；
  删除 `WebUiDagPreview.tsx`、`RealtimeInsightPanel.tsx`；`JobEventTimelineDialog` 改为 `JobEventTimelineContent`。
- 顺带修复：`audit.RedactText` 对 "failed: password=xxx" 这类嵌套赋值不脱敏（plan-dag 失败信息与作业告警消息都会用到）。
- 偏离设计：连线只标背压 %，不标每秒条数——引擎没有按边的吞吐，只有按算子的 QPS（在节点与算子列表里）。
- 验证：`go test ./internal/apps/sync/... ./internal/apps/audit/... ./internal/operation/... ./internal/cli/... ./internal/router/...`；
  `pnpm exec vitest run components/common/sync lib/i18n`（含新增 24 个用例）；`tsc --noEmit`；`pnpm build`
  （React Flow / dagre 只在 3 个按需 chunk 中，不在 /workbench 首屏 30 个 chunk 内）。

## 第二轮（2026-10-28，用户反馈后）
- 提示文案改为说明“表结构 / 建表计划按当前库表状态推导”；卡片去掉类型色条，健康度 ≥20% 才染色，数值改为迷你进度条，Sink 不显示 BP；
  卡片表名去掉库名再中间省略；修复抽屉在作业列表轮询时重置选中表与搜索词（回归测试 `DagNodeDrawer.test.tsx`）。
- 多表演示：任务 15 `mt-cdc-schema-demo`（PG CDC 3 表 → JDBC `mt_sink`），作业 #30 删列失败（SE-07），作业 #31 新增列未被引擎识别、
  新列数据被静默丢弃；历史增列事件见作业 #11「事件」页签。
- 发现未修（范围外）：①终态经事件通道到达时，引擎 errorMsg 未写入 `error_message`（作业 #30）；②`submit_spec.mode` 取自任务元数据，
  可能与 `env.job.mode` 不一致，`jobIsBatch` 会把实时作业当批作业，影响作业告警恢复。

## 第三轮（2026-10-28）
- 指标页全 0 指标折叠；日志首屏骨架 + 刷新保留内容；事件可展开原始 JSON；删列摘要修复并带列类型。
- MySQL 演示：通过 STX 给集群 1 安装 `cdc-mysql` 3.0.0 插件；docker `stx-mysql`（mysql:8.0，127.0.0.1:3306，ROW binlog + GTID，
  密码在 `~/.config/stx-mysql-demo.env`）；任务 16 `mysql-cdc-schema-demo`（3 表 → PG `my_sink`），作业 #32 增列 / 删列 / 改列 4 个
  schema change 事件全部收到，PG 下游表结构同步变更。之后虚拟机重启，作业 #32 被标记失败（引擎已无此作业），事件与拓扑仍可查看。
- PG 演示作业 #31 已取消并删除复制槽 `stx_cdc_multi`。
