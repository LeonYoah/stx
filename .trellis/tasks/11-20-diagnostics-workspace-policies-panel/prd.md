# 诊断中心外壳对齐与巡检策略面板化（第 1 步 / 共 3 步）

## 背景

告警中心（`/monitoring`）重构后采用 Arc 标签 + `.arc-compact` 外壳、列表 + 常驻详情栏（`DetailHost`，J/K 切换）、编辑脏态走 `UnsavedChangesDialog`。诊断中心与它共用模块导航，但仍是旧形态。第 2、3 步（错误面板、巡检面板改分栏）另起 PR。

## 已确认问题（代码证据）

1. 外壳：`DiagnosticsWorkspace.tsx` 用 `components/ui/tabs` 手工 `grid-cols-3`，根节点无 `.arc-compact`；告警中心用 `@/components/arc/tabs/tabs` 并挂 `.arc-compact`（`MonitoringWorkspace.tsx`）。
2. 头部挤：三标签 + 自动策略按钮 + 刷新按钮。刷新只重拉 bootstrap（集群选项），三个面板各自已有刷新。
3. 上下文条里的“全局 / 集群：X”徽章与集群下拉重复。
4. 自动巡检策略：列表弹窗里再叠表单弹窗（`AutoPolicyConfigPanel.tsx`，1272 行）；**无未保存拦截**，关闭即丢；**删除一键直删无确认**；集群显示为“集群 #id”而非名称；与当前所选集群无关联。

## 需求

- R1 外壳：根节点挂 `.arc-compact`；头部 actions 换成 Arc `TabsList`，四个标签：错误 / 巡检 / 巡检策略 / 经验库（`tab=errors|inspections|policies|memories`，触发器带 `data-testid='diagnostics-tab-<key>'`）；移除头部“自动策略”与“刷新”按钮；标签列表由前端固定，`bootstrap.default_tab` 仍生效。
- R2 上下文条保留（三个面板共用的集群筛选 + 来源 / 告警上下文），移除与下拉重复的“全局 / 集群：X”徽章。
- R3 巡检策略面板（`tab=policies`），对齐通知渠道面板模式：
  - 工具栏：名称搜索 + 状态分段（全部 / 已开启 / 失效，带计数）+ 新建按钮；列表用 `TableCard`（骨架 / 错误重试 / 空态 / 分页）。
  - 行：名称（失效时警示徽标 + 原因一行）、作用范围（集群名，全局显示“全局”）、条件摘要、自动诊断包、启用开关。
  - 选中集群时只显示该集群与全局策略；新建默认作用范围取所选集群。
  - 编辑：行点击打开 `DetailHost`（宽栏 / 窄屏抽屉，J/K 切换）；新建：居中 Arc `Dialog`；两者共用同一表单体。
  - 脏态（表单 vs 基线 JSON 比较）下关闭、切行、切顶层标签都走 `UnsavedChangesDialog`；脏态时注册 `beforeunload`。
  - 删除在编辑表单底部用 `ConfirmMorph` 二次确认。
- R4 表单控件改用 Arc（Button / Input / NumberField / Select / Switch / Checkbox 等，有 Arc 等价物的都换），布局改单列以适配 520px 详情栏；`DiagnosticResourceSelector` 不改。
- R5 i18n 中英同步；删除不再使用的键（如 `autoPolicies.buttonLabel`）。
- R6 E2E：新增 mock 模式模板用例覆盖巡检策略面板的加载、新建、脏态拦截、删除确认，以及一个失败样例（新建接口 500 时弹窗保留并提示）。

## 验收标准

- [ ] `/diagnostics` 头部只有模块导航 + 四个 Arc 标签；根节点含 `arc-compact`。
- [ ] `?tab=policies` 打开策略面板；旧深链 `?tab=errors|inspections|memories` 不变。
- [ ] 编辑后点关闭 / 切行 / 切标签出现未保存确认；“继续编辑”保留修改，“放弃修改”执行原动作。
- [ ] 删除需二次确认；启用开关即时生效并回滚失败状态。
- [ ] 第一版 PR #170 的规则选择器行为（集群级 / 作业级分组、失效键可移除）保持并有测试。
- [ ] vitest（新面板单测 + 既有 diagnostics 测试）、`tsc`、eslint、translation-coverage、新 E2E 通过。

## 不在范围

- 错误面板、巡检面板改分栏（第 2、3 步）。
- 把 `DetailHost` / `dirty-close` / 列表样式提升到 `common/layout`（第 2 步错误面板也用到时再提升）。
- 后端接口与 CLI（本步纯前端；接口不变）。
