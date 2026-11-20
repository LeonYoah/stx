# 列表 + 详情分栏公共组件提升到 common/layout

## 背景

诊断中心三个面板（巡检策略 / 错误 / 巡检）直接引用告警中心的 `monitoring/detail-host`、`monitoring/dirty-close`、`monitoring/status-chips` 与 `monitoring/monitoring.module.css`，代码里留了 `shortcut:` 注释。告警中心的并行改动已经落地，现在把这些通用原语提升到 `components/common/layout/`，两边都从公共位置引用。

## 需求

- R1 组件迁移（行为不变）：
  - `monitoring/detail-host.tsx` → `common/layout/DetailHost.tsx`（`DetailHost`、`useDetailNarrow`、`DetailNav`）。
  - `monitoring/dirty-close.tsx` 中的 `UnsavedChangesDialog` 绑定文案包装 + `useBeforeUnload` → `common/layout/DirtyCloseGuard.tsx`（导出 `DirtyCloseDialog`、`useBeforeUnload`）；文案键从 `monitoring.unsavedChangesTitle / unsavedChangesDescription / keepEditing / discardChanges` 迁到 `common.dirtyClose.{title,description,keepEditing,discard}`（中英），删除旧键（grep 确认无其他引用）。
  - `monitoring/status-chips.tsx` 的 `SoftChip` + `ChipTone` → `common/layout/SoftChip.tsx`；`status-chips.tsx` 保留告警专属徽章（`SeverityChip` 等），改为从公共位置导入 `SoftChip`。
  - `common/layout/index.ts` 导出新模块；删除 `monitoring/detail-host.tsx`、`monitoring/dirty-close.tsx`（不留转发壳）。
- R2 样式拆分：新建 `common/layout/list-detail.module.css`，从 `monitoring.module.css` **原样搬移**两侧共用的布局类（页面 / 分栏 / 列表列 / 行与表头 / 工具栏 / 详情列 / 常驻栏与抽屉 / 字段与分区 / 通用文字类等）及其媒体查询；`data-mcols` 列模板（rules / job / channels / alerts / deliveries / policies / errors / inspections）随 `.listCol` 一起搬到公共文件的“列模板”分区，并在分区头用双语注释说明原因（CSS Modules 类名按文件哈希，列模板选择器必须与 `.listCol` 同文件）。告警中心专属类留在 `monitoring.module.css`。
  - 搬移规则：某类被诊断模块或 `DetailHost` 使用 → 搬；与已搬类出现在同一复合选择器 / 同一规则中的类 → 一起搬（传递闭包）；其余留在原文件。
- R3 所有 `../monitoring/*` 跨模块引用改为 `@/components/common/layout`；删除全部相关 `shortcut:` 注释。告警中心文件改为同时引入公共样式（如 `layout`）与自身样式（`styles`），按类所在文件取用。
- R4 规范：`.trellis/spec/frontend/ui-conventions.md` 第 3 节（列表）或第 5 节补一条：列表 + 常驻详情分栏用 `DetailHost` + `list-detail.module.css` + `DirtyCloseDialog`（一行即可）；`directory-structure.md` 的 layout 行同步。

## 验收标准

- [ ] **样式零丢失**：搬移前 `monitoring.module.css` 的规则集合（按“选择器 + 声明”规范化后）== 搬移后 `list-detail.module.css` ∪ 新 `monitoring.module.css` 的规则集合（允许媒体查询分组变化，但每条规则的媒体条件不变）。用临时脚本比对并在报告里贴结果。
- [ ] **类名引用全部可解析**：monitoring 与 diagnostics 目录下每个 `xxx.className` 引用（`styles.*` / `layout.*` 等 CSS Module 对象）都能在其导入的 CSS 文件里找到对应类。用临时脚本检查并贴结果。
- [ ] 不再有任何文件从 `monitoring/` 引入 `detail-host` / `dirty-close` / `SoftChip`；仓库内无 `shortcut:` 指向本次提升。
- [ ] tsc / vitest（monitoring + diagnostics + layout + i18n coverage）/ eslint / 三个诊断 E2E 通过。

## 不在范围

- 视觉或交互调整；告警中心组件逻辑改动。
- `monitoringCenter.policyCenterV2.*` 等疑似遗留文案键的清理。
