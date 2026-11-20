# 设计：三页列表迁移（审计 / 用户 / 主机）

> 关联：告警中心 list+`DetailHost` 模式、PR #168（大盘，出范围）、PR #170（诊断原语升到 `common/layout`）
> 规范：`.trellis/spec/frontend/ui-conventions.md`
> 拍板：2026-11-21 LeonYoah 选 A —— 组件复用、页面一致性；只迁三页

## 1. 结论

- 本期只做 **审计、用户中心（含令牌）、主机** 三页的完整列表迁移。
- 三页走**同一套壳**：既有 `TableCard` + `DetailHost` + `list-detail.module.css` + `DirtyCloseDialog`（按需）+ `SoftChip`（按需）+ `ui/badge` + Arc 交互件；对齐告警中心模式，**不改告警/错误/巡检代码**。
- **不重建** `TableCard`；新建 `useServerListQuery`。
- 集群（卡片网格 + 4031 行多 Tab 详情）、告警/诊断、命令记录、大盘布局全部出范围。

## 2. 现有资产（main `918bfdc1`）

| 资产 | 位置 | 本任务用法 |
|---|---|---|
| `TableCard`（208） | `common/layout/TableCard.tsx` | 三页列表主外框；禁止重建 |
| `DetailHost`（222）+ `list-detail.module.css`（623） | `common/layout/` | 用户/主机常驻详情；审计行详情若需要亦用；追踪除外 |
| `DirtyCloseDialog` / `UnsavedChangesDialog` | `DirtyCloseGuard.tsx` 等 | 脏编辑拦截 |
| `SoftChip` | layout | 行内轻量标签（需要时） |
| `StatPillsBar` | layout | 本期在令牌、主机退役；组件本身下期再删 |
| 告警 `AlertsPanel` | monitoring/alerts | **模式参考 only** |
| `HostDetail` Sheet（559） | host/ | 迁移后内容迁入 DetailHost 插槽；去掉 Sheet 宿主职责 |
| `AuditTraceSheet` | audit/ | 保留 `ui/sheet` 临时覆盖 |

## 3. 交互与壳选型（本期适用）

| 场景 | 组件 | 说明 |
|---|---|---|
| 列表主外框 | 既有 `TableCard` | 加载/骨架/空/错/分页 |
| 常驻详情 | `DetailHost` | J/K；窄屏 Drawer |
| 临时追踪 | `ui/sheet` | 仅 `AuditTraceSheet` |
| 状态筛选（主机） | `SegmentedControl` **无计数** | 现状 StatPills 无 count；禁止编造页内桶计数 |
| 状态筛选（令牌） | `SegmentedControl` **保留计数** | 计数 = `response.active/expired/revoked` |
| 状态标签 | `ui/badge` | 不批量迁 Arc Badge |
| 下拉 / 可搜索 / 更多 | Arc `Select` / `Combobox` / `DropdownMenu` | 三页筛选栏与行内；Combobox 必须有可见 label 或 `aria-label` |
| 破坏性确认 | `ui/alert-dialog` | 删用户、吊销令牌等 |
| 表单对话框 | 维持现状外框 | 主机创建/编辑/安装/发现**不做整页 Arc 化** |

视觉 Token、防框框堆叠、同一区域不混用等约定仍见 `ui-conventions.md` §8；本期在三页落地范围内执行。

### 3.1 筛选计数政策

- **禁止**当前页分桶计数。
- 令牌：服务端全量字段 → 显示计数。
- 主机：无可靠全量分桶 → 分段不显示数字。
- 审计：按现有筛选项迁移，不新造页内计数。

### 3.2 主机信息架构

```
/hosts                    HostMain：TableCard 列表 + DetailHost（选中行）
/hosts/[id]               同一 HostMain（或薄包装）：深链打开 id 对应常驻详情
创建 / 编辑 / 安装 / 发现    既有 Dialog 保留，不做整页 Arc 化
```

- 关闭常驻详情：清除选中并（若在 `/hosts/[id]`）回到 `/hosts` 或等价列表 URL，**不再**依赖「Sheet-only 详情页 + 关 Sheet 才 router.push」。
- `HostDetail` 面板内容复用为 DetailHost children，避免重写业务块。

### 3.3 为何不做集群

- 列表是 **卡片网格**（`ClusterMain`），不是表+常驻详情心智。
- 详情是 **独立多 Tab 整页**（`ClusterDetail` ≈ 4031 行），硬套 DetailHost 会改掉现有交互且分栏放不下。
- 拍板：集群列表与详情本期完全不动。

## 4. 公共件

| 件 | 状态 | 职责 |
|---|---|---|
| `TableCard` | 已有 | § ui-conventions 零塌陷加载与分页 |
| `useServerListQuery` | **待建** | page / pageSize / filters；`patch` 回第 1 页；可选 URL 同步（主机深链依赖） |
| `DetailHost` 等 | 已有（#170） | 直接复用 |

不抽万能 `ServerListShell`。

## 5. 各页迁移要点

| 页 | 要点 | 风险 / AC 焦点 |
|---|---|---|
| 审计（试点） | `TableCard` + hook；`AuditLogTable` 只渲染；追踪 Sheet 保留 | 时区、追踪链路 |
| 用户 | `TableCard` + hook；确认与「不能操作自己」 | 权限约束 |
| 令牌 | 分段 + 接口计数；列表壳一致 | 计数与列表过滤一致 |
| 主机 | 去 Sheet 主详情；深链接；无计数分段；对话框不 Arc 化 | `/hosts/[id]` 行为、安装向导闭环 |

## 6. 回归矩阵（Egolite）

| 页 | 必测 |
|---|---|
| 审计 | 筛选、翻页、页码回 1、追踪 Sheet、时区 |
| 用户 | 搜索翻页、删/停确认、不能操作自己 |
| 令牌 | 分段计数与接口一致、筛选后列表正确 |
| 主机 | 列表选中 DetailHost J/K；`/hosts/[id]` 深链打开对应详情；关详情回列表；状态分段无假计数；创建/编辑/安装不回退 |

## 7. 风险

| 风险 | 缓解 |
|---|---|
| 误改告警/诊断/集群 | 评审 diff 白名单；prd Out of Scope |
| 主机深链回退成 Sheet 页 | AC 明确；Egolite 覆盖 `/hosts/[id]` |
| 主机分段加回页内计数 | 规范 + design §3.1；无 count 验收 |
| 另造卡片/详情壳 | 强制复用 layout 原语；禁止新 TableCard |
