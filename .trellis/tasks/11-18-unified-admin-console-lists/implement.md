# 执行计划：管理列表迁移 + 全站交互组件对齐

每个阶段一个 PR，提交信息用中文。开工前按 AGENTS.md 读 `.trellis/spec/frontend/index.md` 和 `ui-conventions.md`。
新增或修改的注释写中英双语，中文在前。

**每个 PR 的门槛**：
- 现有 CI 通过：vitest、`pnpm exec tsc --noEmit`、已有的 Playwright E2E。lint 要区分存量问题。
- PR 描述附 Egolite 回归记录，格式见文末。

## Phase 0 · 差异清单与选型拍板（只改文档）

- [ ] 按 design §6.1 产出两层页面的差异清单，放在本任务目录的 `inventory.md`。
- [ ] 核对审计、用户在筛选变化时是否漏了"回到第 1 页"。
- [ ] 核对 `ui/popover` 的 3 处用法是否被当作菜单使用。
- 选型已全部拍板（design §3、§3.1、规范 §17、§18），Phase 0 不再有待定项。
- [ ] 差异清单逐区域列出要一起换成 Arc 的控件（搜索框、按钮、输入框、日期），确认是否需要安装 `textarea`、`checkbox`。
- [ ] 差异清单列出 91 处手写颜色 class 的 `Badge`，以及每处对应的 tone。
- [ ] 把选型表写回 `ui-conventions.md`。
- 评审门槛：用户确认差异清单和选型表之后，才进入 Phase 1。

## Phase 1 · 公共件与 Arc 改造

- [ ] 安装 Arc `select`、`dropdown-menu`（`pnpm dlx shadcn@latest add @uiarc/<name> --yes`），同步 `licenses/third_party_mit_files.txt`。
- [ ] 按 design §3.2 补改 `Select`、`Combobox`、`DropdownMenu`、`Button`、`chip-group`（`count`），在文件头注明本地改动；为每项新增能力补单测
      （无标签时有 `aria-label`、自定义触发器能打开菜单、`iconOnly` 为正方形）。
- [ ] 在控制台外层容器上覆盖 Arc 高度变量（36px / 行内 32px），控件不改圆角。
- [ ] 按 design §3.2 收紧三种弹层：`foundation.css` 的 `:root` 加 `--radius-popover`，`DropdownMenu`、`Select`、`Combobox` 改用它，选项行高 34px，高亮时图标变深。
      工作台已有的 `Combobox` 弹层会一起变，Egolite 补一条工作台用例，确认没有被裁切或错位。

- [ ] `TableCard`：容器、保底高度、加载条、首屏骨架 / 空状态 / 错误状态插槽、分页底栏。视觉与 §10 一致。
- [ ] `useServerListQuery`：page / pageSize / filters；`patch` 时回到第 1 页；可选同步到 URL。
- [ ] 单测：
  - hook 的页码重置；
  - 每页条数变化时回到第 1 页；
  - URL 同步的往返；
  - `TableCard` 在加载、空、错误、正常四种状态下的渲染。
- 本阶段不迁移任何页面。

## Phase 2 · 试点：审计日志

- [ ] 接入 `TableCard` 与 `useServerListQuery`。
- [ ] `AuditLogMain` 持有查询状态，`AuditLogTable` 只负责渲染。
- [ ] 状态列换成 Arc `badge`（`size='sm'` 加 `dot`）。用每页 100 行加自动刷新验证动画不卡顿，并把结论写进 PR。
- [ ] 迁移前后截图对比；Egolite 跑审计用例。
- 评审门槛：试点通过后，再批量迁移其余页面。

## Phase 3 · 用户中心

- [ ] 令牌管理的 `StatPillsBar` 换成 `SegmentedControl` + 计数（design §3.3）。

- [ ] 接入两个公共件，确认与表单弹窗按选型表处理。
- [ ] Egolite：用户用例，"不能操作自己"必须覆盖。

## Phase 4 · 错误中心 + 巡检

- [ ] 两页的 `StatPillsBar` 换成 `SegmentedControl` + 计数（design §3.3）。

- [ ] 两页接入公共件；巡检的启动对话框保留 `Dialog`。
- [ ] Egolite：两页用例，以及跳转链路。

## Phase 5 · 告警中心

- [ ] 告警状态的 `StatPillsBar` 换成 `SegmentedControl` + 计数（design §3.3）。

- [ ] 先换卡片与查询状态，再处理行操作与详情面板（可拆成两个 PR）。
- [ ] Egolite：告警用例，含深链参数。

## Phase 6 · 交互组件对齐层（单独 PR，Phase 0 之后可与 Phase 1–5 并行）

- [ ] 3 处 `window.confirm` 换成 `AlertDialog`（文案沿用现有 i18n key）。
- [ ] 作业列表工具栏的 `StatPillsBar` 换成 `chip-group` 多选 + 计数；URL 支持多值并兼容旧单值链接；只选"运行中"时才能按速率 / CDC 延迟排序。
- [ ] 下拉、可搜索下拉、行内"更多"换成 Arc；同一块区域里相邻的 shadcn 控件一起换，做到不混用。
- [ ] 用普通 `Dialog` 承载的纯确认，改成 `AlertDialog`。
- [ ] 手写颜色 class 的 `Badge` 换成 Arc `badge` 的 `tone`，每个域只用一个映射函数。完成后用 `grep` 确认没有遗漏。
- [ ] 下拉框、更多菜单按 `inventory.md` 的清单逐处替换。
- [ ] diff 只能是组件替换，不能有布局、数据流或轮询的改动。
- [ ] Egolite：每一处替换各一条用例。

## Phase 7 · 收尾

- [ ] 删除各页面里被公共件替代的重复代码（确认没有引用后再删）。
- [ ] `ui-conventions.md` §10 补充 `TableCard` 与 `useServerListQuery` 的用法；更新一致性检查项。
- [ ] 如果 vendored 文件有增减，同步 `licenses/third_party_mit_files.txt`。

## Egolite 回归记录格式（附在 PR 描述里）

```
### Egolite 回归 · Phase N
- 环境：macOS 版本 / 浏览器 / 后端版本（commit）
- 结果：通过 x / 共 y

| 页面 | 用例 | 步骤摘要 | 结果 | 截图 |
|---|---|---|---|---|
| /audit-logs | 筛选后页码回到 1 | 翻到第 3 页 → 改动作筛选 → 检查页码 | 通过 | … |
```
