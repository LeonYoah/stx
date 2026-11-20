# 执行计划：管理列表迁移 + 全站交互组件对齐

每个阶段一个 PR，提交信息用中文。开工前按 AGENTS.md 读 `.trellis/spec/frontend/index.md` 和 `ui-conventions.md`。
新增或修改的注释写中英双语，中文在前。

**范围收缩（2026-11-20）**：#168 大盘布局已合入；#170 诊断 list+detail 已合入；告警中心 list+detail 已在 main。Phase 4/5 改为组件对齐；Phase 1 **不**新建 `TableCard`。

**每个 PR 的门槛**：
- 现有 CI 通过：vitest、`pnpm exec tsc --noEmit`、已有 Playwright E2E。lint 区分存量问题。
- PR 描述附 Egolite 回归记录，格式见文末。

## Phase 0 · 差异清单与选型拍板（只改文档）

- [ ] 按 design §6.1 产出差异清单 → 本任务目录 `inventory.md`（按 **当前 main** 盘点，勿用 2026-11-18 旧行号）。
- [ ] 核对审计、用户筛选是否漏「回第 1 页」。
- [ ] 核对 `ui/popover` 是否被当菜单。
- [ ] grep `window.confirm` 与巡检策略确认路径（#170 后可能已无三处旧调用）。
- 选型已拍板（design §3、§3.1、§3.3 计数政策、§3.4、DetailHost 标准、规范 §3/§5/§8/§9/§10）。
- [ ] 列出要对齐的 Arc 控件包；确认 `textarea` / `checkbox` 是否还需新装。
- 评审门槛：用户确认清单后进入 Phase 1。

## Phase 1 · 公共件与 Arc 改造

- [ ] **不要**新建 `TableCard`（`common/layout/TableCard.tsx` 已存在）。
- [ ] 按需补齐 Arc：`Select` / `DropdownMenu` / `Button` `iconOnly` / `chip-group` `count` / 弹层 Token；`Combobox` label 在 main 已可选——验收：**无可见标签时必须有 `aria-label`**（单测覆盖）。
- [ ] 控制台外层覆盖 8–10px / 32–28px Token；`:root` `--radius-popover`；三种弹层收紧。
- [ ] 新建 `useServerListQuery`：page / pageSize / filters；`patch` 回第 1 页；可选 URL 同步 + 单测。
- [ ] 单测：`TableCard` 四态（若缺补）；hook 页码重置 / pageSize / URL 往返。
- 本阶段不迁移页面。

## Phase 2 · 试点：审计日志

- [ ] 接入既有 `TableCard` + `useServerListQuery`。
- [ ] `AuditLogMain` 持查询状态，`AuditLogTable` 只渲染；追踪保持 `AuditTraceSheet`（临时 Sheet）。
- [ ] 状态标签保留 `ui/badge`。
- [ ] 截图对比 + Egolite 审计用例。
- 评审门槛：试点通过后再迁用户。

## Phase 3 · 用户中心

- [ ] 令牌 `StatPillsBar` → `SegmentedControl`（有可靠计数才显示数字）。
- [ ] 接入 `TableCard` + hook；确认 / 表单按选型。
- [ ] Egolite：用户用例，必盖「不能操作自己」。

## Phase 4 · 错误中心 + 巡检（仅组件对齐）

> list + `DetailHost` + `TableCard` 已由 #170 落地。**禁止**重做布局或加回当前页桶计数。

- [ ] Select / Combobox / DropdownMenu / 确认路径对齐选型；同一区域不混用。
- [ ] 可选：接线 `useServerListQuery`（不改变筛选语义）。
- [ ] 复核策略页确认（`DirtyCloseDialog` / 删除确认），勿按旧 `window.confirm` 行号改。
- [ ] Egolite：两页 + 跳转链路；断言筛选无页内桶计数。

## Phase 5 · 告警中心（仅组件对齐）

> list + `DetailHost` 已在 main。**禁止**重做骨架。

- [ ] 「关闭告警」接 `ConfirmMorph`（防叠框）。
- [ ] 其余下拉 / 菜单 / 确认对齐；状态分段计数保持服务端全集统计。
- [ ] 可选接线 `useServerListQuery`。
- [ ] Egolite：深链、关闭确认展开/取消/确认、DetailHost J/K。

## Phase 6 · 交互组件对齐层（单独 PR；可与 Phase 1–5 并行）

- [ ] 工作台 + **大盘**：确认、下拉、溢出菜单按清单替换；**不改** `KpiStrip` / `ThroughputTabs` / `DashboardPanel` / `ClusterScopeStrip` / 作业列表结构。
- [ ] 大盘头部：去掉 `[&_label]:sr-only`；Combobox 使用可选 label + **`aria-label`（或可见 label）**——验收控件有可访问名称。
- [ ] **不要**恢复「全部 / 正式 / 预览」运行类型分段。
- [ ] 作业工具栏 `StatPillsBar` → `chip-group` 多选 + 计数；URL 多值兼容；只选「运行中」可速率排序。
- [ ] 全库复核 `window.confirm` 与 Dialog→AlertDialog 纯确认。
- [ ] diff 仅组件替换；Egolite 每处替换一条。

## Phase 7 · 收尾

- [ ] 删除迁移页重复的卡片/分页代码。
- [ ] `ui-conventions.md` 核对 `TableCard` / `useServerListQuery` / DetailHost / 计数政策 / Combobox a11y 检查项。
- [ ] vendored 增减同步 `licenses/third_party_mit_files.txt`。

## Egolite 回归记录格式（附在 PR 描述里）

```
### Egolite 回归 · Phase N
- 环境：macOS 版本 / 浏览器 / 后端版本（commit）
- 结果：通过 x / 共 y

| 页面 | 用例 | 步骤摘要 | 结果 | 截图 |
|---|---|---|---|---|
| /audit-logs | 筛选后页码回到 1 | 翻到第 3 页 → 改动作筛选 → 检查页码 | 通过 | … |
```
