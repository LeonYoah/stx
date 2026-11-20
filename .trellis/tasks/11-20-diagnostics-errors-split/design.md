# 设计

## 后端
- `error_models.go` `SeatunnelErrorGroupFilter` 加 `MinOccurrences *int`、`MaxOccurrences *int`（json `min_occurrences` / `max_occurrences`）。
- `handler.go` 组过滤解析处（~L800-830）解析两参：空 → nil；非整数或负数 → 返回解析错误（与 `parseOptionalTime` 等同路径，最终 400）。仅用于 group list（group detail 的 filter 构造不受影响）。
- `repository.go` `applyGroupFilters`：`occurrence_count >= ?` / `<= ?`。
- `internal/operation/registry.go` L1945：group list 的 inputs 在 `errorCommon` 基础上追加两个 `InputQuery`，示例改为含 `--min_occurrences 10`。确认 registry 覆盖/文档测试通过（`internal/operation`、`internal/cmd`）。

## 前端
- 新目录 `components/common/diagnostics/errors/`：
  - `ErrorsPanel.tsx`：列表、筛选、分页、`DetailHost`、URL 同步（props 与旧组件一致 + `clusterOptions: DiagnosticsClusterOption[]`）。
  - `ErrorGroupDetail.tsx`：详情体（含 `SaveMemoryDialog`）。
  - `error-utils.ts`：`formatNodeOrigin`、热度分桶 → `{min,max}` 映射、热度 → `SoftChip` tone（≥10 danger / ≥3 warning / 其他 neutral，按 `status-chips.tsx` 现有 tone 名）。
- 复用：`../monitoring/detail-host`、`../monitoring/monitoring.module.css`（`page`/`pageViewport`/`split`/`listCol`/`row`/`rowHead`/`toolbar` 等，同 `policies/PoliciesPanel.tsx`），`../monitoring/status-chips` 的 `SoftChip`，Arc `SegmentedControl` / `Input` / `Button` / `CopyButton`，`TableCard`。`monitoring.module.css` 只**追加** `data-mcols='errors'` 列模板与 ≤640px 回退（放在 policies 块旁边），不改其他规则。跨模块引用沿用第 1 步的 `shortcut:` 注释格式。
- `DiagnosticsWorkspace.tsx`：errors 标签渲染 `ErrorsPanel`，传 `clusterOptions`。
- 删除 `DiagnosticsErrorCenter.tsx`，`index.ts` 导出 `ErrorsPanel`。
- `useDrawerItem`、keyboard 行导航、`/` 聚焦：照 `policies/PoliciesPanel.tsx`。
- GSAP 入场动效（`animateTableRows` / `animateSheetSections`）：Sheet 动效删掉；列表行动效可去掉（告警中心列表不用），保持与告警中心一致。
- i18n：`diagnosticsCenter.errors.*` 下补齐键，复用已有键（`columns.*`、`keywordPlaceholder` 等）；删除不再使用的键（grep 确认）。
