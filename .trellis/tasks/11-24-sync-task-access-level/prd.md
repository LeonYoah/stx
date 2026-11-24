# 同步任务三态共享权限模型（Route B）

## 背景与痛点

当前数据同步工作台（DataSync Studio）的共享权限仅提供二元布尔开关 `is_public`（「公开」/「私有」）外加 `collaborator_ids`（共建者名单），在实际团队协作中存在四处明显歧义与不一致：

1. **「公开」语义二义性**：团队用户开启「公开」后，预期常是「大家都能一起维护或按需只读」，但当前系统把「公开」硬绑定为「仅可查看、不可编辑运行」，没有提供「公开可协作」档位。
2. **「锁图标」语义冲突**：在共享设置弹窗里，锁图标（`Lock`）代表「私有」；但在左侧任务树和编辑区顶栏，同一枚琥珀色锁图标又挂在「公开但只读」的任务上，且文案称作「只读锁定」，让用户误以为任务被私密保护或被他人占锁。
3. **权限矩阵与右键菜单不一致**：后端已区分「编辑/运行」（所有者 + 管理员 + 共建者）与「删除任务 / 修改共享设置」（仅所有者 + 管理员），但工作台任务树右键菜单此前按 `can_edit` 显示「删除」入口，导致无删除权限的协作者点击删除后才收到 403 报错；同时模型代码注释也将删除与编辑混为一谈。
4. **权限保存后树节点状态不同步**：在工作台保存共享权限后，本地树节点未同步更新权限标记字段（`access_level` / `is_public` / `can_edit` / `can_run` / `is_owner` / `is_collaborator`），导致侧栏显示滞后。

2026-11-24 用户明确选定 **路线 B**：将任务共享权限升级为三态模型（`private` / `public_readonly` / `public_editable`），同时彻底解耦「只读」与「私有」的视觉图标和文案，并保持对历史数据、旧客户端及 CLI 的向后兼容。

---

## 目标与范围

### 范围内（In Scope）

1. **后端权限模型与 API**：`internal/apps/sync/`（模型、权限裁决、独立权限接口、正文更新保护、任务树/任务详情装饰、审计与校验错误）。
2. **STX CLI 对齐**：`internal/cmd/sync.go` 与 `internal/operation/registry_sync.go`（`stx sync task permissions get/update` 新增三态参数并兼容旧参数）。
3. **前端工作台 UI 与状态流**：`frontend/lib/services/sync/`、`frontend/components/common/sync/`（共享设置浮层、左侧树、右键菜单删除门禁、编辑区只读提示、树节点权限同步）。
4. **中英文国际化**：`frontend/lib/i18n/locales/{zh,en}.json` 完整单语文案与覆盖校验。
5. **自动化测试与规范同步**：后端单测/接口测试、CLI 测试、前端单测，以及 `.trellis/spec/backend/execution-security-guidelines.md` §14 契约同步。

### 范围外（Out of Scope）

- 不做数据库表结构迁移（复用现有 `sync_tasks.definition` JSON 字段承载 `access_level`，并双写 `is_public`）。
- 不新增「另存为我的任务」快捷按钮（已有树右键「复制文件」入口）。
- 不修改作业停止权（`CanStop` 仍严格归属管理员、作业发起人、任务所有者）与全局变量权限模型。
- 不修改任务目录树筛选器「全部 / 我的 / 仅我创建」的归属定义（「我的」仍仅包含我创建的或我被显式列为共建者的任务；他人的公开任务无论只读还是可协作，统一在「全部」视图查看）。

---

## 权限矩阵（权威需求真相）

| 身份 / 场景 | 查看任务与 DAG | 编辑 / 保存 / 发布 / 运行 / 预览 / 告警 / Dolphin 推送 / 作业恢复 | 删除任务 / 修改共享设置 |
| --- | :---: | :---: | :---: |
| 管理员（`admin`） / 所有者（`created_by`） | ✓ | ✓ | ✓ |
| 共建者（`collaborator`） | ✓ | ✓ | ✗ |
| 其他团队成员 + `public_editable`（公开可协作） | ✓ | ✓ | ✗ |
| 其他团队成员 + `public_readonly`（公开只读） | ✓ | ✗（只读） | ✗ |
| 其他团队成员 + `private`（私有） | ✗ 不可见 | — | — |

> 说明：
> - 历史遗留无归属任务（`created_by == 0`）沿用现有兼容规则。
> - 作业控制权限中，**停止作业（`CanStop`）**仍仅允许管理员、该作业发起人或任务所有者，不因 `public_editable` 对全员放开；**恢复作业（`CanRecover`）**跟随任务运行权（`CanUserRun`）。

---

## 核心需求

### R1 · 三态共享模型与向后兼容

- 任务共享级别固定为三个枚举值：
  - `private`：私有（仅所有者、管理员与指定共建者可见、可编辑/运行）。
  - `public_readonly`：公开只读（全员可查看，仅所有者、管理员与指定共建者可编辑/运行）。
  - `public_editable`：公开可协作（全员可查看、编辑与运行；删除任务与修改共享设置仍仅限所有者与管理员）。
- 历史未写入 `access_level` 的任务必须无缝兼容：
  - `is_public == false` → 等价于 `private`；
  - `is_public == true` 或未显式设置 → 等价于 `public_readonly`（保持安全默认）。
- 新建任务的默认共享级别为 `public_readonly`（与历史默认行为一致，不破坏既有团队可见性预期）。
- 切换至 `public_editable` 时，既有的 `collaborator_ids` 名单必须原样保留、不得静默清空；当所有者后续切回 `private` 或 `public_readonly` 时，原共建者名单立即重新生效。

### R2 · 独立权限接口与安全边界

- 权限读取/修改继续由独立接口 `GET/PUT /api/v1/sync/tasks/:id/permissions` 承担，不得要求回传任务正文，也不创建任务正文版本。
- `PUT /api/v1/sync/tasks/:id`（正文更新）必须保护 `access_level` 与 `is_public`、`collaborators`、`collaborator_ids` 一样不被伪造篡改，始终从数据库原记录保留。
- 仅任务所有者与管理员可修改共享设置；普通查看者、共建者、以及通过 `public_editable` 获得编辑权的成员均无权修改共享设置（返回 `403`）或删除任务（返回 `403`）。
- 权限修改接口同时支持新字段 `access_level` 与旧字段 `is_public`；对非法 `access_level` 或互相矛盾的 `access_level` + `is_public` 组合，必须明确拒绝（`400`），不得静默吞掉错误。
- 审计记录 `sync.task.permissions.update` 的 `before`/`after` 必须包含 `access_level` 变更前后状态，且不记录任务正文。

### R3 · CLI 能力完整对齐

- `stx sync task permissions get <id>` 输出包含 `access_level`、`is_public`、`collaborator_ids` 及能力标记。
- `stx sync task permissions update <id>` 支持 `--access-level <private|public_readonly|public_editable>`，同时保留 `--public` / `--private` 兼容旧脚本。
- CLI 在本地完成参数冲突与枚举校验（如 `--access-level` 与 `--public`/`--private` 混用、非法枚举值），直接以用法错误退出码失败，不发网络请求。

### R4 · 消除 UI 图标与文案歧义

- **图标语义解耦**：
  - `Lock`（琥珀色）仅表达 **私有（`private`）**，禁止再用于「公开只读」任务。
  - `Eye`（中性柔和色）表达 **公开只读 / 仅查看**（含左侧任务树只读标识、编辑区顶栏只读徽章、只读提示横幅及共享触发按钮的 `public_readonly` 状态）。
  - `Globe`（绿色）表达 **公开可协作（`public_editable`）**。
- **文案去歧义**：
  - 移除「只读锁定 / Read-Only Locked」中的「锁定」误导说法，改为准确的「只读 / Read-Only」或「仅查看 / View-Only」。
  - 共享设置弹窗将三种访问权限以竖排单选列表呈现（标题 + 清晰一行说明），让用户在点击前即明确「全员可查看」与「全员可编辑运行」的差异。
  - 共享设置弹窗右上角角色徽章区分四种状态：**所有者**、**共建者**、**团队可协作**（非所有者且非共建者但因 `public_editable` 拥有编辑权）、**只读**。
  - 当任务处于 `public_editable` 时，共建者区块明确提示“全员当前均可编辑运行，以下名单将在切回私有或公开只读时生效”。
- **右键菜单与权限同步修正**：
  - 任务树右键菜单的「删除」操作仅对有删除权的用户（所有者 / 管理员 / 遗留无归属任务）展示，不再对普通共建者或 `public_editable` 成员误展示。
  - 保存共享设置后，左侧树节点与编辑区状态立即同步更新 `access_level`、`is_public`、`can_edit`、`can_run` 等权限字段。
- **遵循 UI 规范**：不使用 `cursor-not-allowed`；支持中英文单语完整切换，无双语混排。

---

## 验收标准（Acceptance Criteria）

- [ ] **AC1（模型与兼容）**：`Task.AccessLevel()` 正确解析 `private` / `public_readonly` / `public_editable`，历史无 `access_level` 任务按 `is_public=false → private`、其余 `→ public_readonly` 兼容；`Task.IsPublic()` 与 `AccessLevel() != private` 始终一致。
- [ ] **AC2（权限裁决）**：在 `public_editable` 下，非所有者、非共建者的普通团队成员 `CanUserView=true`、`CanUserEdit=true`、`CanUserRun=true`，可以保存正文、发布、预览、提交运行、配置任务告警、推送 Dolphin、恢复作业；但调用 `DeleteTaskForActor` 或 `UpdateTaskPermissionsForActor` 仍返回 `403`。
- [ ] **AC3（安全与校验）**：正文更新 `PUT /sync/tasks/:id` 携带伪造 `access_level` 时数据库原值保持不变；`PUT /sync/tasks/:id/permissions` 收到非法 `access_level` 或相互矛盾的 `access_level` + `is_public` 返回 `400`；只传旧字段 `is_public` 时正确兼容（`false → private`，`true` 在原为 `private` 时升为 `public_readonly`，原为 `public_editable` 时不意外降级）。
- [ ] **AC4（CLI 对齐）**：`stx sync task permissions update <id> --access-level public_editable --confirm` 正常提交并带安全头；非法值或与 `--public`/`--private` 混用本地拦截返回用法错误退出码；旧 `--public`/`--private` 测试继续通过。
- [ ] **AC5（UI 消歧与防误操作）**：左侧任务树只读文件显示 `Eye` 图标而非 `Lock`；编辑区顶栏与横幅不再出现「只读锁定」与 `Lock`；共享弹窗可清晰切换三态并显示正确的四态角色徽章；右键菜单对非所有者隐藏「删除」；保存权限后树节点立即同步最新权限字段；无 `cursor-not-allowed`。
- [ ] **AC6（国际化与质量门禁）**：`zh.json` 与 `en.json` 键位对称并通过 `translation-coverage.test.ts`；后端 `go test`、CLI `go test`、前端 `tsc --noEmit` 与对应 `vitest` 全部通过；`.trellis/spec/backend/execution-security-guidelines.md` §14 已同步更新。
