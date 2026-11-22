# 全局 UI 统一：三页列表迁移（审计 / 用户 / 主机）

## 背景

作业列表与告警 / 诊断的 list+`DetailHost` 模式已分别落地。本任务目标是：**尽可能保证组件复用与页面一致性**，把仍缺统一壳的管理列表迁到与告警中心同一套原语上。

2026-11-21 产品拍板（LeonYoah 选 A + stx UI 检查）：本期**只做三个页面**的完整列表迁移——审计日志、用户中心（含令牌）、主机列表（含详情）。告警 / 错误 / 巡检 / 策略、集群列表与详情、命令记录、大盘布局均出范围。

## 已确认事实（2026-11-21 复核，main tip 仍为 `918bfdc1`）

### 本期要迁的三页

| 页面 | 文件（行数） | 现状 | 本期目标 |
|---|---|---|---|
| 审计日志 | `AuditLogMain.tsx`（368）+ `AuditLogTable.tsx`（464）+ `AuditTraceSheet.tsx`（263） | 自建卡片/分页；追踪用 `ui/sheet` | `TableCard` + `useServerListQuery` + 列表壳对齐；**追踪仍用 Sheet** |
| 用户中心 | `UserManagement.tsx`（728） | 自建卡片/分页 | 同上 + DetailHost（若有行详情） |
| 令牌 | `TokenManagement.tsx`（621） | `StatPillsBar`，计数来自接口 `response.active/expired/revoked`（可靠全量） | `SegmentedControl` **保留计数**；列表壳对齐 |
| 主机 | `HostMain.tsx`（438）+ `HostTable.tsx`（441）+ `HostDetail.tsx`（559，`ui/sheet`）+ `HostDetailPage.tsx`（147） | 自建表；点查看开 Sheet；`/hosts/[id]` 仍套同一张 Sheet，关 Sheet → `/hosts`；`StatPillsBar` **无 count** | `TableCard` + `DetailHost`；`/hosts/[id]` 改为列表页深链接打开常驻详情；`SegmentedControl` **不编造桶计数** |

### 明确不动（本期零改动）

| 区域 | 依据（main） | 理由 |
|---|---|---|
| 告警 / 错误 / 巡检 / 策略 | 已是 `TableCard` + `DetailHost` | 连交互组件对齐也不做 |
| 集群列表 | `ClusterMain.tsx`（480），卡片网格 + `StatPillsBar` | 卡片网格，不硬套 DetailHost |
| 集群详情 | `ClusterDetail.tsx`（**4031**），独立多 Tab 整页 | 体量不适配分栏常驻详情 |
| 命令记录 `/commands` | — | 不进本期 |
| 大盘布局 | #168 已合入 | 出范围；`HomeDashboard` 的 Combobox `sr-only` 清理仅为可选小遗留，不阻塞三页 |

### 可复用既有资产（禁止另造）

| 资产 | 位置 | 说明 |
|---|---|---|
| `TableCard` | `common/layout/TableCard.tsx`（208） | **已存在，禁止重建** |
| `DetailHost` | `DetailHost.tsx`（222） | 列表 + 常驻 rail / 窄屏 Drawer / J/K |
| 脏关闭 | `DirtyCloseGuard.tsx`（`DirtyCloseDialog`）、`UnsavedChangesDialog.tsx` | 编辑脏态 |
| `SoftChip` / `list-detail.module.css` | layout | 与告警中心同壳 |
| `ui/badge` | `components/ui/badge.tsx` | 不做 Arc Badge 批量迁移 |
| Arc 交互件 | Select / Combobox / DropdownMenu / SegmentedControl / SearchField / Button 等 | 三页筛选与行内操作对齐 |
| `useServerListQuery` | **尚不存在** | Phase 1 新建 |

模式参考：告警中心 `AlertsPanel` 的 list+`DetailHost`（只对齐模式，**不改告警代码**）。

## Locked Decisions（2026-11-21 选 A，不再重开）

1. **本期只迁三页**：审计、用户中心（含令牌）、主机（含详情）。核心原则：组件复用、页面一致性。
2. **告警 / 错误 / 巡检 / 策略完全不动**（含交互对齐）。
3. **集群列表与集群详情完全不动**（不改成表+DetailHost，不改 `StatPillsBar`）。
4. **详情标准 = `DetailHost`**（J/K、窄屏 drawer）。`ui/sheet` 仅临时覆盖；`AuditTraceSheet` 保留 Sheet。
5. **主机**：列表 → `TableCard` + `DetailHost`；`/hosts/[id]` → 列表深链接打开常驻详情（不再 Sheet-only）；创建/编辑/安装/发现对话框不做整页 Arc 化；状态 `StatPillsBar`（无 count）→ `SegmentedControl` 且**不编造当前页分桶计数**。
6. **令牌**：`StatPillsBar` → `SegmentedControl`，计数继续用接口 `active/expired/revoked`。
7. **禁止**加回当前页桶计数；**不重建** `TableCard`；Phase 1 建 `useServerListQuery`。
8. **Combobox a11y**：去掉 `sr-only` 后必须有可见 label 或 `aria-label`（三页若用到）。大盘 `HomeDashboard` sr-only 清理可选、不阻塞。
9. **命令记录、大盘布局**出范围。
10. **回归用 Egolite**；现有 CI 通过；保留 `ui/badge`；`StatPillsBar` 在本期三页内退役（令牌、主机），其余调用下期。
11. **不做**：审计批量、「万能表格」、移动端专用方案（窄屏用 DetailHost 已有 Drawer）。

## Requirements

### R1 共享壳与选型
- 三页同一套：`TableCard` + `DetailHost` + `list-detail.module.css` + Arc 交互件 + `ui/badge`。
- 选型已写在 `ui-conventions.md` §3/§5/§8/§9；本期范围变更同步到规范「迁移进度」表述。

### R2 三页完整迁移
- Phase 1：`useServerListQuery`；复用 #170 原语；不新建 `TableCard`。
- 审计（试点）→ 用户+令牌 → 主机（含 `/hosts/[id]` 深链接 AC）。
- 业务逻辑、接口、权限不变。

### R3 回归
- 每阶段 PR 附 Egolite；覆盖三页 AC（见 Acceptance Criteria）。

## Acceptance Criteria

### 共享
- [ ] 三页列表外框均为既有 `TableCard`；详情（除审计追踪）均为 `DetailHost`；未新建第二套宿主 / 卡片。
- [ ] 筛选变化回第 1 页；无当前页桶计数；令牌分段计数与接口全量字段一致。
- [ ] 本期改动文件不含告警 / 错误 / 巡检 / 策略 / 集群列表 / 集群详情 / 命令记录 / 大盘布局。
- [ ] Egolite + 现有 CI 通过；文案走 next-intl；可点击控件有可读可访问名称。

### 审计
- [ ] 列表壳与告警中心模式一致（`TableCard` + 查询 hook）。
- [ ] 追踪仍走 `AuditTraceSheet`（`ui/sheet`）；时区与追踪链路不回退。

### 用户 + 令牌
- [ ] 用户列表接入 `TableCard` + hook；删除/停用仍为 `AlertDialog`；「不能操作自己」不回退。
- [ ] 令牌 `StatPillsBar` 已换 `SegmentedControl` 且保留 `active/expired/revoked` 计数。

### 主机
- [ ] 列表为 `TableCard` + `DetailHost`（J/K；窄屏 drawer）；不再用常驻 `HostDetail` Sheet 作为主详情。
- [ ] `/hosts/[id]` 打开对应主机的常驻详情（深链接）；关闭详情回到列表态且 URL 行为符合约定（不再「关 Sheet 才从详情路由弹回」的 Sheet-only 页）。
- [ ] 状态筛选为无分桶计数的 `SegmentedControl`；创建/编辑/安装/发现对话框行为不回退且不做整页 Arc 化。

## Out of Scope

- 告警、错误、巡检、巡检策略的任何改动。
- 集群列表 / 集群详情；命令记录；大盘布局与作业工具栏 `StatPillsBar`。
- 主机创建/编辑/安装/发现对话框的整页 Arc 化。
- 其余页面的 `StatPillsBar` 与表单 Arc 化（下期）。
