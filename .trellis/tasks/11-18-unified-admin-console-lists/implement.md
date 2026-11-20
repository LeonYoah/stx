# 执行计划：三页列表迁移（审计 / 用户 / 主机）

每个阶段一个 PR，提交信息用中文。开工前读 `.trellis/spec/frontend/index.md` 与 `ui-conventions.md`。
注释中英双语，中文在前。

**范围（2026-11-21 选 A）**：只迁审计、用户+令牌、主机。告警/错误/巡检/策略、集群、命令记录、大盘布局 **零改动**。已作废旧 Phase「告警/诊断交互对齐」与「集群相关改动」。

**每个 PR 门槛**：现有 CI 通过；PR 附 Egolite 记录（格式见文末）。

## Phase 0 · 对当前 main 盘点（只改文档）

- [ ] 按 main tip（复核仍为 `918bfdc1` 或更新 SHA）刷新 `inventory.md`：三页文件/行号、主机 Sheet 与 `/hosts/[id]` 行为、令牌计数字段。
- [ ] 确认三页筛选是否漏「回第 1 页」。
- [ ] 列出三页要对齐的 Arc 控件；Combobox 若使用则登记 a11y。
- [ ] 确认 diff 白名单不含 alerts/diagnostics/cluster/commands/dashboard 布局。
- 评审通过后进 Phase 1。

## Phase 1 · 共享能力（不迁页面）

- [ ] **禁止**新建 `TableCard`；复用 `common/layout/TableCard.tsx`。
- [ ] 复用 #170 原语：`DetailHost`、`DirtyCloseDialog` / `UnsavedChangesDialog`、`SoftChip`、`list-detail.module.css`。
- [ ] 新建 `useServerListQuery`（page / pageSize / filters；`patch` 回第 1 页；可选 URL 同步）+ 单测。
- [ ] 三页落地所需的 Arc 缺口（若有）：可选 label + **强制**可见 label 或 `aria-label`；弹层 Token 仅在三页容器需要时覆盖。
- [ ] （可选、不阻塞）`HomeDashboard` 去掉 `sr-only` 并补 `aria-label`——**不改大盘布局**。

## Phase 2 · 试点：审计日志

- [ ] 接入 `TableCard` + `useServerListQuery`。
- [ ] `AuditLogMain` 持查询状态，`AuditLogTable` 只渲染。
- [ ] `AuditTraceSheet` 保持 `ui/sheet`。
- [ ] 状态标签保留 `ui/badge`。
- [ ] Egolite：审计用例；截图对比。
- 试点通过后再迁用户 / 主机。

### AC · 审计
- [ ] 列表壳与告警中心同模式（TableCard + hook），未另造卡片。
- [ ] 追踪 Sheet 与时区行为不回退。

## Phase 3 · 用户中心 + 令牌

- [ ] 用户列表接入 `TableCard` + hook；删除/停用 `AlertDialog`；「不能操作自己」保留。
- [ ] 令牌：`StatPillsBar` → `SegmentedControl`，计数继续绑定 `response.active/expired/revoked`。
- [ ] 行内控件按选型；`ui/badge` 不动。
- [ ] Egolite：用户 + 令牌用例。

### AC · 用户 / 令牌
- [ ] 列表壳一致；权限与确认不回退。
- [ ] 令牌分段有可靠全量计数；无当前页假计数。

## Phase 4 · 主机列表 + 详情深链接（新建阶段）

- [ ] `HostMain`：`TableCard` + `useServerListQuery`；`HostTable` 列渲染接入卡片。
- [ ] 详情：`HostDetail` 内容进 `DetailHost`；去掉列表态常驻 `ui/sheet` 宿主。
- [ ] `/hosts/[id]`：改为打开列表页并选中该主机常驻详情（深链接）；关闭详情回到 `/hosts`（或清除 id 的列表 URL）；删除 Sheet-only 详情页心智。
- [ ] 状态：`StatPillsBar` → `SegmentedControl`，**不**显示分桶计数。
- [ ] 创建 / 编辑 / 安装向导 / 发现：对话框保留，**不做整页 Arc 化**；安装心跳闭环不回退。
- [ ] Egolite：列表 J/K、深链接、关详情、分段无假计数、对话框主路径。

### AC · 主机
- [ ] 布局壳与另两页一致（TableCard + DetailHost）。
- [ ] `/hosts/[id]` 深链接打开对应常驻详情，非 Sheet-only 页。
- [ ] 筛选无当前页桶计数；既有创建/编辑/安装/发现行为不回退。

## Phase 5 · 收尾

- [ ] 删除三页内被替代的重复卡片/分页/Sheet 宿主代码（确认无引用）。
- [ ] 核对 `ui-conventions.md`：三页迁移进度、主机详情走 DetailHost、计数政策。
- [ ] vendored 增减同步 license 清单。

## 已作废的旧阶段（勿再开 PR）

- ~~告警中心交互对齐 / ConfirmMorph~~
- ~~错误中心 + 巡检（及策略）组件对齐~~
- ~~工作台 / 大盘交互对齐层（含作业工具栏 StatPillsBar → chip-group）~~
- ~~集群列表 / 详情改 DetailHost 或改 StatPillsBar~~

## Egolite 记录格式

```
### Egolite 回归 · Phase N
- 环境：macOS / 浏览器 / 后端 commit
- 结果：通过 x / 共 y

| 页面 | 用例 | 步骤摘要 | 结果 | 截图 |
|---|---|---|---|---|
| /hosts/[id] | 深链接打开常驻详情 | 打开 /hosts/3 → 见 DetailHost 选中 3 → 关闭 → 回列表 | 通过 | … |
```
