# 设计：工作台草稿自动保存 + 离开拦截

## 边界

- 仅前端 `DataSyncStudio` 链路；后端 `updateTask` 契约不变。
- 自动保存 `publishAfterSave=false`，禁止调用 `publishTask`。
- 不改 Dock / Console / 侧栏收起相关代码结构（离开拦截用 document 捕获点击，避免改 FloatingDock API）。

## 数据流

1. 用户编辑 → `updateEditor` / 相关路径 → `markEditorDraft(..., true)` → `editorDrafts[id].dirty=true`
2. 防抖 1500ms → `persistCurrentFile(false)`（仅当前可编辑且 dirty）→ 成功 `markEditorDraft(..., false)` → 状态 `saved`
3. 失败 → dirty 保留，状态 `error`，toast 一次；可点状态重试或再编辑触发防抖
4. 离开风险 = 任意可编辑 dirty 草稿 **或** 保存中 **或** 失败未恢复
5. 站内离开：捕获 Dock/`<a>` 点击离开 `/workbench` → 对话框
6. Save&Leave：冲刷当前 + 其余 dirty 草稿（`updateTask`）→ 导航；Discard：清空 dirty drafts → 导航

## 关键契约

| 符号 | 说明 |
| --- | --- |
| `DraftSaveStatus` | `idle \| saving \| saved \| error` |
| `AUTOSAVE_DEBOUNCE_MS` | `1500` |
| `hasUnsavedRisk` | dirty(可编辑) \|\| saving \|\| error |
| Leave dialog | Stay / Save&Leave / Discard&Leave；Discard 非默认焦点 |

## 兼容性

- 手动「保存」与自动保存共用 `persistCurrentFile` 与同一 `draftSaveStatus`；成功态以状态条为准，避免双重 toast。
- 失败仍 toast（与现网一致），状态条同步 `error`。
- 多页签：非当前 dirty 保留在 `editorDrafts`；离开 Save&Leave 一并落盘。

## 回滚

删除 hooks/组件 + 还原 `DataSyncStudio` 保存/离开相关改动即可；无 DB/API 迁移。
