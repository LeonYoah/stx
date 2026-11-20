# 设计

- 新目录 `components/common/diagnostics/inspections/`：
  - `InspectionsPanel.tsx`：列表 / 筛选 / 分页 / 自动刷新 / `DetailHost` / URL 同步 / 发起巡检入口。
  - `InspectionReportDetail.tsx`：详情体 + 诊断包区（含轮询、确认弹窗、执行日志弹窗）。
  - `StartInspectionDialog.tsx`：发起巡检 Arc 弹窗（回溯、错误阈值、资源选择 `DiagnosticResourceSelector`，校验与提交逻辑从旧组件迁移）。
  - `inspection-utils.ts`：状态 → `SoftChip` tone、严重度排序、`formatNodeOrigin` 复用 `../errors/error-utils`（不要复制）、回溯预设常量。
- 复用：`../monitoring/detail-host`、`../monitoring/monitoring.module.css`（列表与详情样式，同 errors/policies）、`../monitoring/status-chips` `SoftChip`、`TableCard`、Arc `SegmentedControl` / `Select` / `Button` / `Dialog` / `NumberField` / `Tooltip`、`InspectionFindingCard` / `InspectionZeroState`（来自 `InspectionEvidenceView`）。跨模块引用带与 errors 面板一致的 `shortcut:` 注释。`monitoring.module.css` 只在 640px 媒体查询内 / 列模板区**追加** `data-mcols='inspections'` 块（注意放在 `@media (max-width: 640px)` 块内部，第 1 步曾放错过）。
- 自动刷新：`useEffect` 依赖「当前页是否有 pending/running」布尔值；`setInterval` 5000ms 调用 `loadReports({silent: true})`（silent 不置 loading），卸载 / 布尔变 false 清理。详情运行中同理调 `loadDetail(id, {silent:true})`。
- toast 用 `@/lib/toast`（本分支已合入 Arc Toast 迁移，`sonner` 已移除）。
- `DiagnosticsWorkspace.tsx` 渲染 `InspectionsPanel` 并传 `clusterOptions`；删除 `DiagnosticsInspectionCenter.tsx`；`index.ts` 导出更新；`.trellis/spec/frontend/directory-structure.md` 诊断行同步。
