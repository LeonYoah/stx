# 实施计划：同步任务三态共享权限模型（Route B）

> 开工前必读：
> - 本任务规划：`prd.md`、`design.md`
> - 项目规范：`.trellis/spec/backend/index.md`、`.trellis/spec/backend/execution-security-guidelines.md`（§14）、`.trellis/spec/backend/cli-guidelines.md`、`.trellis/spec/backend/quality-guidelines.md`、`.trellis/spec/frontend/index.md`、`.trellis/spec/frontend/ui-conventions.md`、`.trellis/spec/guides/cross-layer-thinking-guide.md`
>
> 编码纪律：
> - 新增或修改的注释必须**中英双语（中文在前，英文在后）**，只解释 why/契约。
> - 保持最小改动，不做范围外重构或投机抽象。
> - **完成开发与自检后不要 `git commit` 或 `git push`**，将干净的改动与测试结果交由导师审查通过后再统一提交。

---

## Phase 1 · 后端核心模型与权限接口（Go）

- [ ] `internal/apps/sync/err.go`：新增 `ErrInvalidTaskAccessLevel`。
- [ ] `internal/apps/sync/model.go`：
  - 定义 `TaskAccessLevel` 三态常量与 `normalizeTaskAccessLevel`；
  - 新增 `Task.AccessLevel() TaskAccessLevel`（兼容无 `access_level` 的历史 `is_public`）；
  - `Task.IsPublic()` 委派至 `t.AccessLevel() != TaskAccessLevelPrivate`；
  - `Task.CanUserEdit` 支持 `t.AccessLevel() == TaskAccessLevelPublicEditable`，并修正误导性注释（明确删除/权限管理单独走 owner/admin 校验）；
  - `Task` 新增虚拟字段 `TaskAccessLevel TaskAccessLevel` (`json:"access_level" gorm:"-"`) 并在 `DecoratePermissions` 填充；
  - `TaskPermissions` 新增 `AccessLevel TaskAccessLevel` (`json:"access_level"`)。
- [ ] `internal/apps/sync/types.go`：
  - `UpdateTaskPermissionsRequest` 新增 `AccessLevel *string` (`json:"access_level"`)；
  - `TaskTreeNode` 新增 `AccessLevel TaskAccessLevel` (`json:"access_level"`)。
- [ ] `internal/apps/sync/service.go`：
  - `UpdateTaskPermissionsForActor` 实现三态校验、与 `is_public` 的一致性校验、双写 `access_level` + `is_public`、以及仅传 `is_public` 的兼容逻辑（`true` 在原为 `public_editable` 时不意外降级）；
  - `preserveTaskPermissionFields` 补充保护 `"access_level"`；
  - `validateTaskDefinition` 若含 `"access_level"` 则校验合法枚举；
  - `taskPermissionsForActor` 与 `buildTaskTree` 填充 `AccessLevel`。
- [ ] `internal/apps/sync/handler.go`：
  - `getStatusCodeForError` 将 `ErrInvalidTaskAccessLevel` 映射为 `http.StatusBadRequest`（`400`）。
- [ ] 后端测试（`internal/apps/sync/service_test.go`、`internal/apps/sync/handler_test.go`、`internal/apps/sync/job_permissions_test.go` 如有）：
  - 覆盖三态读写、历史 `is_public` 回退、`public_editable` 非共建者可编辑/发布/运行/告警/恢复但**不可删除且不可改权限且不可停止他人作业**；
  - 覆盖非法 `access_level` 返回 `400`、矛盾的 `access_level` + `is_public` 返回 `400`；
  - 覆盖正文更新 `PUT /sync/tasks/:id` 伪造 `access_level` 不生效；
  - 覆盖切换至 `public_editable` 再切回 `private` 时 `collaborator_ids` 完整保留；
  - 覆盖只传 `is_public: true` 时对 `public_editable` 不误降级、对 `private` 升为 `public_readonly`。

---

## Phase 2 · CLI 对齐（Go）

- [ ] `internal/cmd/sync.go`：
  - `newSyncTaskPermissionsUpdateCommand` 新增 `--access-level` 参数；
  - 补充本地校验：`--access-level` 合法枚举检查、与 `--public`/`--private` 互斥检查、必填项检查；
  - 更新命令 `Long` / `Example`。
- [ ] `internal/operation/registry_sync.go`：更新 `sync.task.permissions.update` 说明与示例。
- [ ] `internal/cmd/sync_test.go`：
  - 验证 `permissions get` 解析 `access_level`；
  - 验证 `permissions update --access-level public_editable --confirm` 发送精确请求体与安全头；
  - 验证非法 `--access-level` 与 `--access-level` + `--public`/`--private` 冲突在本地以 `ExitUsage` 失败且不发请求；
  - 保留并确保既有 `--private`/`--public` 测试全部通过。

---

## Phase 3 · 前端服务、工具与 UI 消歧（TypeScript / React）

- [ ] `frontend/lib/services/sync/types.ts`：新增 `SyncTaskAccessLevel` 并在 `SyncTask` / `SyncTaskTreeNode` / `SyncTaskPermissions` / `UpdateSyncTaskPermissionsRequest` 补全 `access_level`。
- [ ] `frontend/components/common/sync/sync-studio-utils.ts`：
  - 新增 `resolveTaskAccessLevel` 纯函数；
  - `EditorState` / `EMPTY_EDITOR` / `extractEditorStateFromTask` / `extractEditorStateFromTreeNode` / `buildTaskSavePayload` 补全 `accessLevel` / `access_level`；
  - 修复 `patchTreeNode` 同步更新权限字段（`access_level` / `is_public` / `can_edit` / `can_run` / `is_owner` / `is_collaborator`）。
- [ ] `frontend/components/common/sync/StudioSharePopover.tsx`：
  - 支持 `accessLevel` prop（并兜底 `isPublic` 兼容）；
  - 触发按钮三态图标：`Lock`（私有）/ `Eye`（公开只读）/ `Globe`（公开可协作）；
  - 头部角色徽章四态：所有者 / 共建者 / 团队可协作 / 只读；
  - 访问级别改为 3 行竖排 `radiogroup`/`radio`，`PopoverContent` 挂 `arc-compact`、`0.5px` 细线分隔，移除 `cursor-not-allowed`；
  - `public_editable` 下展示共建者名单保留提示。
- [ ] `frontend/components/common/sync/TreeView.tsx`：只读文件图标从琥珀色 `Lock` 改为中性 `Eye` + `t('readOnlyViewOnly')`。
- [ ] `frontend/components/common/sync/StudioTreeContextMenu.tsx`：删除按钮门禁由 `canEdit` 修正为 `canDelete`（仅所有者/管理员/遗留无归属可见）。
- [ ] `frontend/components/common/sync/DataSyncStudio.tsx`：
  - 顶栏只读徽章与编辑区只读横幅由 `Lock` + 「只读锁定」改为 `Eye` + 柔和样式 + 「只读」；
  - `handleUpdateSharing`、`handleSavePermissions` 与 `StudioSharePopover` 调用传参全量接入 `accessLevel` 并补齐 `can_run` 等权限回写。
- [ ] `frontend/lib/i18n/locales/zh.json` & `frontend/lib/i18n/locales/en.json`：对称补齐并更新全部文案键（纯单语，无双语拼接）。
- [ ] 前端测试：
  - 更新/补充 `StudioSharePopover.test.tsx`（三态切换、四态角色徽章、非所有者只读态、`public_editable` 提示、回调参数）；
  - 更新/补充 `sync-studio-utils.test.ts`（`resolveTaskAccessLevel` 优先级、`patchTreeNode` 权限同步）；
  - 新增或补充 `StudioTreeContextMenu` / `TreeView` 测试，覆盖只读 `Eye` 图标与非所有者隐藏「删除」。

---

## Phase 4 · 规范同步与全量自检

- [ ] 更新 `.trellis/spec/backend/execution-security-guidelines.md` §14，记录 `access_level` 接口/CLI/兼容契约与错误矩阵。
- [ ] 运行验证命令并记录真实输出：
  ```bash
  go test ./internal/apps/sync/... ./internal/cmd/... ./internal/operation/...
  cd frontend && pnpm exec tsc --noEmit
  cd frontend && pnpm exec vitest run lib/i18n/__tests__/translation-coverage.test.ts components/common/sync/__tests__/
  ```
- [ ] 对照 `prd.md` AC1–AC6 逐项自检，输出改动摘要、文件清单与验证结果，等待导师审查。

---

## 回滚点

- 后端仅在 `definition` JSON 双写 `access_level` 与 `is_public`，不涉及 DB schema 变更；回滚代码后旧版本直接消费 `is_public`，无数据迁移或权限泄漏风险。
