# 设计

## 文件

- `components/common/diagnostics/DiagnosticsWorkspace.tsx`：外壳改造（R1/R2）；`DiagnosticsTabKey` 增加 `policies`（类型定义处同步，`resolveTab` 接受 `policies`）。持有 `policiesDirty` 状态，`handleTabChange` 在脏态时挂起目标标签并弹 `UnsavedChangesDialog`。
- 新目录 `components/common/diagnostics/policies/`：
  - `PoliciesPanel.tsx`：数据加载（沿用 `listAutoPoliciesSafe({page_size:100})` + 模板 + 规则目录）、筛选、`TableCard` 列表、`DetailHost` / Arc `Dialog`、脏态拦截（照 `monitoring/notify/NotifyPanel.tsx` 的 `editKey: number | '__new__' | null` + `discardTarget` 模式）、启用切换、删除。Props：`clusterId?: number`、`clusterOptions`、`onDirtyChange?: (dirty: boolean) => void`。
  - `PolicyFormBody.tsx`：从 `AutoPolicyConfigPanel` 抽出的表单体（名称 / 集群 / 启用 / 冷却 / 条件模板与覆盖 / 规则选择器 / 自动诊断包与任务选项），`policy: InspectionAutoPolicy | null`、`templates`、`ruleCatalog`、`clusterOptions`、`defaultClusterId`、`onSaved`、`onCancel`、`onDelete?`、`onDirtyChange?`。基线 = 打开时由 policy 推导的表单值，`dirty = JSON.stringify(form) !== JSON.stringify(base)`（同 `ChannelDrawerBody`）。保存校验与提交逻辑原样迁移。
  - `policy-form-utils.ts`：迁入 `shouldRenderCronExprInput` / `applyConditionTextOverride` / `normalizeConditionItemsForSave` / `normalizeAutoPolicyTaskOptions` / `INVALID_REASON_CODES`，既有单测改导入路径。
- 删除 `AutoPolicyConfigPanel.tsx`；`diagnostics/index.ts` 改导出 `PoliciesPanel`。
- 复用告警中心原语：`../monitoring/detail-host`（`DetailHost`、`useDetailNarrow`）、`../monitoring/dirty-close`（`UnsavedChangesDialog`、`useBeforeUnload`）、`../monitoring/monitoring.module.css`（`page` / `pageViewport` / `split` / `listCol` / `row` / `rowHead` 等）、`useDrawerItem`（告警中心现用的同一 hook）、`TableCard`、Arc `ConfirmMorph` / `SegmentedControl` / `Switch`。在 `monitoring.module.css` 增加 `.listCol[data-mcols='policies']` 列模板（含 640px 窄屏只留名称 + 开关）。跨模块直接引用加 `shortcut:` 双语注释：第 2 步错误面板也接入时提升到 `common/layout`。

## 交互细节

- 状态分段计数基于集群筛选后的集合；“失效” = `invalid_reason` 非空。
- 行点击打开详情；行内启用开关 `stopPropagation`，乐观更新、失败回滚并 toast。
- 新建保存成功：关闭弹窗、刷新列表、打开新策略详情不需要（保持简单：只刷新）。
- 编辑保存成功：刷新列表，详情保持打开并以新数据为基线（`key` 带 `updated_at` 重挂载）。
- J/K：`DetailHost` 的 `nav` 基于当前页过滤后列表；切行走脏态拦截。
- 作用范围标签：`cluster_id===0` → `autoPolicies.scopeGlobal`；否则在 `clusterOptions` 找名称，找不到回落原 `scopeCluster` 文案。

## 兼容

- URL 新增 `tab=policies`；其他参数不变。后端、CLI 无变化。
