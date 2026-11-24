# 技术设计：同步任务三态共享权限模型（Route B）

## 1. 跨层数据流总览

```text
[Web UI StudioSharePopover / STX CLI]
        │  PUT /api/v1/sync/tasks/:id/permissions
        │  { access_level?: "private"|"public_readonly"|"public_editable",
        │    is_public?: boolean, collaborator_ids?: number[] }
        ▼
[HTTP Handler (internal/apps/sync/handler.go)]
        │  鉴权 + 绑定请求 + 安全头/幂等/审计 (sync.task.permissions.update)
        ▼
[Service (internal/apps/sync/service.go)]
        │  1) canManageTaskPermissions (admin || owner || created_by==0)
        │  2) 校验 access_level 合法性 + access_level/is_public 一致性
        │  3) 双写 definition["access_level"] & definition["is_public"]
        │  4) 去重并双写 definition["collaborators"] & definition["collaborator_ids"]
        ▼
[DB sync_tasks.definition (JSON)]
        │  读取时统一经 Task.AccessLevel() / Task.DecoratePermissions() 归一化
        ▼
[API Response: TaskPermissions / Task / TaskTreeNode]
        │  同时返回 access_level + is_public + can_edit + can_run + is_owner + is_collaborator (+ can_manage)
        ▼
[Frontend Service & Utils (resolveTaskAccessLevel / patchTreeNode)]
        │  单点解析 + 同步回填 EditorState 与左侧树节点
        ▼
[UI: StudioSharePopover / TreeView / StudioTreeContextMenu / DataSyncStudio]
```

---

## 2. 后端设计（`internal/apps/sync/`）

### 2.1 枚举类型与错误定义

- 在 `internal/apps/sync/model.go` 定义：
  ```go
  // TaskAccessLevel 表示工作台同步任务的团队共享访问级别。
  // TaskAccessLevel represents the workspace sharing access level of a sync task.
  type TaskAccessLevel string

  const (
      TaskAccessLevelPrivate        TaskAccessLevel = "private"
      TaskAccessLevelPublicReadOnly TaskAccessLevel = "public_readonly"
      TaskAccessLevelPublicEditable TaskAccessLevel = "public_editable"
  )
  ```
- 提供归一化校验 helper `normalizeTaskAccessLevel(raw string) (TaskAccessLevel, error)`：
  - `strings.ToLower(strings.TrimSpace(raw))` 仅接受 `"private"`、`"public_readonly"`、`"public_editable"`。
  - 其余值返回新错误 `ErrInvalidTaskAccessLevel`（定义在 `internal/apps/sync/err.go`），并在 `handler.go` 的 `getStatusCodeForError` 中映射为 `http.StatusBadRequest`（`400`）。

### 2.2 读取解析与单点真相（`model.go`）

- `Task.AccessLevel() TaskAccessLevel`：
  1. 若 `t == nil || t.Definition == nil` → 返回 `TaskAccessLevelPublicReadOnly`。
  2. 若 `t.Definition["access_level"]` 存在且为非空字符串：
     - 若合法，直接返回对应 `TaskAccessLevel`。
     - 若为脏字符串，回落到 `is_public` 推导逻辑（防历史异常数据崩溃）。
  3. 否则读取 `t.Definition["is_public"]`：
     - `bool(false)` 或不区分大小写的 `"false"` → `TaskAccessLevelPrivate`；
     - 缺失或其余值 → `TaskAccessLevelPublicReadOnly`。
- `Task.IsPublic() bool`：
  - 统一委派为 `return t.AccessLevel() != TaskAccessLevelPrivate`（消除两套判断漂移）。
- `Task.CanUserEdit(userID uint, isAdmin bool) bool`：
  - 更新注释（中英双语）：明确此方法裁决编辑、保存、发布与运行等协作权限；**删除任务与修改共享设置不走此方法，而是由 `DeleteTaskForActor` / `canManageTaskPermissions` 单独校验所有者或管理员权限**。
  - 判断逻辑：
    ```go
    if isAdmin {
        return true
    }
    if t == nil {
        return false
    }
    if t.CreatedBy == 0 || t.CreatedBy == userID {
        return true
    }
    if t.HasCollaborator(userID) {
        return true
    }
    return t.AccessLevel() == TaskAccessLevelPublicEditable
    ```
- `Task.CanUserRun(userID uint, isAdmin bool) bool`：
  - 继续复用 `t.CanUserEdit(userID, isAdmin)`（自然包含 `public_editable`）。
- `Task.CanUserView(userID uint, isAdmin bool) bool`：
  - 保持现有逻辑：`isAdmin || CreatedBy == 0 || CreatedBy == userID || HasCollaborator(userID) || t.IsPublic()`（因为 `IsPublic()` 对 `public_readonly` 和 `public_editable` 均返回 `true`）。
- `Task.DecoratePermissions(userID uint, isAdmin bool)`：
  - 额外填充虚拟字段 `t.AccessLevelValue = t.AccessLevel()`（struct 字段命名注意避免与方法名 `AccessLevel()` 冲突！例如 struct 字段叫 `AccessLevelValue TaskAccessLevel` 且 json tag 为 `json:"access_level" gorm:"-"`，或方法叫 `EffectiveAccessLevel()` / `ResolvedAccessLevel()`。**推荐使用字段 `AccessLevel TaskAccessLevel` + 方法 `EffectiveAccessLevel()`，或保留方法 `AccessLevel()` 并将字段命名为 `TaskAccessLevel`（tag `json:"access_level"`）——与同结构体里已有 `IsPublicTask bool` + `IsPublic()` 完全一致！**
    - 核对 `model.go:207`：现有命名正是 `IsPublicTask bool json:"is_public" gorm:"-"` 配方法 `IsPublic() bool`，且 `TaskTreeNode` 也叫 `IsPublicTask bool json:"is_public"`！因此 `Task` 和 `TaskTreeNode` 字段命名为 `TaskAccessLevel TaskAccessLevel json:"access_level" gorm:"-"`（或 `AccessLevel` 在 `TaskTreeNode` / `TaskPermissions` 上，因这两者无 `AccessLevel()` 方法），与代码库惯例 100% 契合）。

### 2.3 写入契约、兼容优先级与安全保护（`types.go` + `service.go`）

- `UpdateTaskPermissionsRequest`（`internal/apps/sync/types.go`）：
  ```go
  type UpdateTaskPermissionsRequest struct {
      AccessLevel     *string `json:"access_level"`
      IsPublic        *bool   `json:"is_public"`
      CollaboratorIDs []uint  `json:"collaborator_ids"`
  }
  ```
- `UpdateTaskPermissionsForActor` 解析规则：
  1. 若 `req.AccessLevel != nil`：
     - 调用 `normalizeTaskAccessLevel(*req.AccessLevel)`，非法返回 `ErrInvalidTaskAccessLevel`（`400`）。
     - 若同时提供了 `req.IsPublic != nil`：
       - 计算 `expectedPublic := level != TaskAccessLevelPrivate`；若 `*req.IsPublic != expectedPublic`，返回 `ErrInvalidTaskAccessLevel`（`400`，拒绝矛盾请求）。
     - 写入 `definition["access_level"] = string(level)` 与 `definition["is_public"] = (level != TaskAccessLevelPrivate)`。
  2. 否则若 `req.IsPublic != nil`（仅旧字段）：
     - 若 `!*req.IsPublic` → `level = TaskAccessLevelPrivate`。
     - 若 `*req.IsPublic`：若原任务当前 `task.AccessLevel() == TaskAccessLevelPublicEditable`，保留 `TaskAccessLevelPublicEditable`（防止旧客户端编辑协作者时附带 `is_public: true` 误降级）；否则设为 `TaskAccessLevelPublicReadOnly`。
     - 写入 `definition["access_level"] = string(level)` 与 `definition["is_public"] = (level != TaskAccessLevelPrivate)`。
  3. `req.CollaboratorIDs != nil` 时按原规则校验（禁止 `0`、去重、双写 `collaborators` 和 `collaborator_ids`）；若 `req.CollaboratorIDs == nil`，原有协作者名单**原样保留**（即使切换到了 `public_editable`）。
- `preserveTaskPermissionFields(original, updated JSONMap)`（`service.go:523`）：
  - 保护键清单更新为：`[]string{"access_level", "is_public", "collaborators", "collaborator_ids"}`。
  - 确保 `PUT /sync/tasks/:id` 永远无法篡改或清除已保存的 `access_level`。
- `validateTaskDefinition(definition JSONMap)`：
  - 若 `definition["access_level"]` 存在且非空，必须校验为合法三态字符串，否则返回 `ErrInvalidTaskAccessLevel`（`400`），防止 `POST /sync/tasks` 写入非法 `access_level`。
- `TaskPermissions`（`model.go:358`）：
  - 新增 `AccessLevel TaskAccessLevel json:"access_level"`；`taskPermissionsForActor` 填充 `AccessLevel: task.AccessLevel()`。
- `buildTaskTree`（`service.go:2197`）：
  - `TaskTreeNode` 同步填充 `AccessLevel: task.AccessLevel()`（`json:"access_level"`）。
- `DeleteTaskForActor` 与 `canManageTaskPermissions` 与 `EvaluateJobPermissions`：
  - `DeleteTaskForActor` 维持仅 `admin / created_by / created_by==0`；
  - `canManageTaskPermissions` 维持仅 `admin / created_by / created_by==0`；
  - `EvaluateJobPermissions` 维持 `CanStop` 仅 `admin / submitter / task owner`，`CanRecover` 走 `task.CanUserRun`（对 `public_editable` 生效）。

---

## 3. CLI 设计（`internal/cmd/sync.go` & `internal/operation/registry_sync.go`）

- 命令：`stx sync task permissions update <id>`
- 新增参数：
  - `--access-level <value>`：可选值 `private`、`public_readonly`、`public_editable`（自动 trim + lower）。
- 保留兼容参数：
  - `--public` / `--private`（继续发送 `is_public: true/false`，确保现有脚本与单测不破坏）。
  - `--collaborator-id` / `--clear-collaborators`。
- 本地校验（均返回 `clioutput.CodeUsage` + `clioutput.ExitUsage`，不发请求）：
  1. `--access-level` 与 `--public` 或 `--private` 同时指定 → 拒绝。
  2. `--access-level` 非法枚举值 → 拒绝（明确列出合法三值）。
  3. `--public` 与 `--private` 同时指定 → 拒绝（保持原逻辑）。
  4. `--clear-collaborators` 与 `--collaborator-id` 同时指定 → 拒绝（保持原逻辑）。
  5. `--access-level`、`--public`、`--private`、`--collaborator-id`、`--clear-collaborators` 全部未传 → 拒绝。
- `internal/operation/registry_sync.go`：更新 `sync.task.permissions.update` 的中文说明，明确包含访问级别（私有 / 公开只读 / 公开可协作）与协作者列表。

---

## 4. 前端设计（`frontend/`）

### 4.1 类型与纯函数归一化

- `frontend/lib/services/sync/types.ts`：
  - 新增 `export type SyncTaskAccessLevel = 'private' | 'public_readonly' | 'public_editable';`
  - 在 `SyncTask`、`SyncTaskTreeNode`、`SyncTaskPermissions`、`UpdateSyncTaskPermissionsRequest` 中补齐 `access_level?: SyncTaskAccessLevel`（`SyncTaskPermissions` 中为必填 `access_level: SyncTaskAccessLevel`）。
- `frontend/components/common/sync/sync-studio-utils.ts`：
  - 新增唯一解析函数 `resolveTaskAccessLevel(source)`：
    - 优先级：`source.access_level` → `source.definition?.access_level` → `source.is_public` / `source.definition?.is_public`（显式 `false` → `'private'`，其余 → `'public_readonly'`）。
  - `EditorState` 与 `EMPTY_EDITOR` 增加 `accessLevel?: SyncTaskAccessLevel`（默认 `'public_readonly'`）。
  - `extractEditorStateFromTask` 与 `extractEditorStateFromTreeNode` 统一通过 `resolveTaskAccessLevel` 填充 `accessLevel` 与 `isPublic`（`accessLevel !== 'private'`）。
  - `buildTaskSavePayload` 同步在 `definition` 中保留 `access_level` 与 `is_public`（与后端双写一致）。
  - **修复 `patchTreeNode`**：除原有字段外，当 `task` 上携带 `access_level`、`is_public`、`can_edit`、`can_run`、`is_owner`、`is_collaborator` 时，一并更新到目标树节点。

### 4.2 图标与视觉语义（严守 `ui-conventions.md`）

- 图标映射（均来自已安装的 `lucide-react`，12px–14px 清晰可辨）：
  | 语义 | 图标 | 色调 | 适用位置 |
  | --- | --- | --- | --- |
  | 私有（`private`） | `Lock` | `text-amber-600 dark:text-amber-400` / `bg-amber-500/15` | 共享触发按钮小标、共享弹窗「私有」选项 |
  | 公开只读（`public_readonly`） / 只读态 | `Eye` | `text-sky-600 dark:text-sky-400`（共享态） / `text-muted-foreground`（树与横幅只读态） | 共享触发按钮小标、共享弹窗「公开只读」选项、左侧树只读标识、编辑区只读徽章与横幅 |
  | 公开可协作（`public_editable`） | `Globe` | `text-emerald-600 dark:text-emerald-400` / `bg-emerald-500/15` | 共享触发按钮小标、共享弹窗「公开可协作」选项、团队协作者角色徽章 |
- **彻底移除误导**：
  - `TreeView.tsx:341-348`：把 `<Lock className='size-3.5 text-amber-500 shrink-0' />` + `t('readOnlyLocked')` 改为 `<Eye className='size-3.5 text-muted-foreground shrink-0' />` + `t('readOnlyViewOnly')`。
  - `DataSyncStudio.tsx:4681-4691`（顶栏徽章）与 `5256-5269`（编辑区横幅）：改用 `<Eye>` + 柔和中性样式 + `t('readOnly')` 与新版 `t('readOnlyBannerText')`。
  - `StudioSharePopover.tsx`：
    - `PopoverContent` 根挂 `arc-compact`，内部分区采用 `border-t-[0.5px] border-border/80` 细线压平，禁止 `cursor-not-allowed`。
    - 顶部角色徽章四态分支：
      1. `isOwner` → `<Shield>` + `t('owner')`
      2. `isCollaborator` → `<UserCheck>` + `t('collaborator')`
      3. `canEdit` → `<Globe>` + `t('teamCollaborator')`（「团队可协作」/ `"Team Editable"`）
      4. 否则 → `<Eye>` + `t('readOnly')`（「只读」/ `"Read-Only"`）
    - 访问权限选择区：按 `ui-conventions.md` §3“带说明行的 2–5 个选项用竖排 Radio”规范，改为 3 行竖排 `role="radiogroup"` / `role="radio"` 选项：
      1. `private`（`Lock`）：私有 — 仅所有者与指定共建者可见、可编辑运行
      2. `public_readonly`（`Eye`）：公开只读 — 团队全员可查看，仅所有者与共建者可编辑运行
      3. `public_editable`（`Globe`）：公开可协作 — 团队全员可查看、编辑与运行（删除与权限管理仍归所有者）
    - 协作者区：当 `accessLevel === 'public_editable'` 时，不清空已有名单，下方提示文案切换为 `t('publicEditableCollaboratorHint')`（说明全员可协作且名单在切回私有/公开只读时保留生效）。
    - `onUpdateSharing` 签名升级为 `(accessLevel: SyncTaskAccessLevel, collaboratorIds: number[]) => void | Promise<void>`；组件 props 支持 `accessLevel?: SyncTaskAccessLevel` 并兜底 `isPublic` 兼容旧调用方。
- **右键菜单删除门禁修正（`StudioTreeContextMenu.tsx`）**：
  - 区分 `canEdit`（重命名 / 移动）与 `canDelete`（删除）：
    - `const canDelete = Boolean(menuState.node && (menuState.node.is_owner === true || menuState.node.created_by === 0 || (menuState.node.is_owner === undefined && canEdit)));`
  - 仅在 `!isRoot && canDelete && menuState.node` 时渲染「删除」菜单项，与后端 `DeleteTaskForActor` 严格一致。

### 4.3 `DataSyncStudio.tsx` 保存与同步流

- `handleUpdateSharing(accessLevel, collaboratorIds)`：同步写入 `accessLevel`、`isPublic: accessLevel !== 'private'` 与 `definition.access_level` / `definition.is_public` / `definition.collaborator_ids` / `definition.collaborators`。
- `handleSavePermissions()`：
  - 请求体发送 `{ access_level, collaborator_ids }`（不再依赖旧 `is_public` 字段）。
  - 保存成功后同步刷新 `editor`（`accessLevel`、`isPublic`、`canEdit`、`canRun`、`isOwner`、`isCollaborator`、`definition`）与 `setTree((current) => patchTreeNode(current, updatedTask))`。

---

## 5. 国际化设计（`zh.json` / `en.json`）

在 `workbenchStudio` 命名空间补齐/更新对称键（禁止同屏双语拼接）：
- 更新语义文案：`readOnly`（「只读」/ `"Read-Only"`）、`readOnlyViewOnly`（「仅查看（无编辑权限）」/ `"View only (no edit permission)"`）、`readOnlyBannerText`（明确说明这是公开只读任务，如需编辑可联系所有者加共建者或设为公开可协作）。
- 新增访问权限三态文案：`accessLevelPrivate` / `accessLevelPublicReadOnly` / `accessLevelPublicEditable` 及其说明 `accessLevelPrivateDesc` / `accessLevelPublicReadOnlyDesc` / `accessLevelPublicEditableDesc`、`teamCollaborator`、`publicEditableCollaboratorHint`。
- 若既有测试或页面仍引用旧键 `readOnlyLocked` / `public` / `private` 等，保留或同步更新所有引用处并通过 `translation-coverage.test.ts` 验证。

---

## 6. 兼容性与回滚策略

- **零迁移向前兼容**：数据库不改表结构；新写入同时落盘 `definition.access_level` 与 `definition.is_public`，即使回滚到旧版服务端，旧版仍能正确读取 `definition.is_public`（将 `public_readonly` 和 `public_editable` 都安全回落为旧版「公开只读」，将 `private` 识别为「私有」，**绝不会在回滚时发生权限放大或数据丢失**）。
- **Spec 同步**：在实现验证通过后，更新 `.trellis/spec/backend/execution-security-guidelines.md` §14，补充 `access_level` 字段契约、兼容规则、错误矩阵与 CLI `--access-level` 说明。
