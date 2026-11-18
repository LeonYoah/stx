# 全局 UI 换新：统一管理控制台与工作台（列表 / 工作台 / 控制台 / 其他页面）

## 背景 / Background

当前系统已在「工作台任务列表 / 控制台作业列表」完成了作业实例列表的统一（`frontend/components/common/sync/job-table/`：`JobTable` + `JobTableToolbar` + `StatPillsBar` + 服务端分页 + 统一行操作/详情宿主），形成了较成熟的 **服务端驱动列表（server-driven list）** 骨架与交互范式。

但除作业列表外，其他管理控制台与工作台页面仍各自实现布局、筛选、加载与覆盖层，存在重复与不一致。本轮明确扩大范围为 **全局大换新**，统一组件与交互体系，但遵守“扩展已有 UI 组件/壳，而非新造通用 UI”的最小变更原则。

## 范围 / Scope（全局）

### P0（必须换新与回归）
- 工作台：`/workbench`（任务列表、作业列表、运行详情、节点/拓扑、告警与恢复链路）
- 控制台作业：`/sync`（作业列表、作业详情、拓扑、恢复/保存点/停止、Dolphin 触发链路）
- 告警中心：`/monitoring?tab=alerts`
- 错误中心与巡检中心：`/diagnostics`（`DiagnosticsErrorCenter` / `DiagnosticsInspectionCenter`）
- 审计日志：`/audit-logs`
- 用户中心：`/admin/users`

### P1（按同一框架换新）
- 监控中心其它页面：`/monitoring`、`/monitoring/rules`、`/monitoring/history`、`/monitoring/integrations`
- 集群管理：`/clusters`、`/clusters/[id]`、升级向导页面
- 主机管理：`/hosts`、`/hosts/[id]`
- 安装包/插件：`/packages`、`/plugins`
- 命令历史：`/commands`
- 系统升级：`/system-upgrade`
- 仪表盘：`/dashboard`

### P2（保留但需组件对齐）
- E2E 实验室：`/e2e-lab/install-wizard`
- 鉴权页面：`/(auth)/login`、`/(auth)/callback`

## 组件策略 / Component Strategy

- **首选复用/扩展**：Arc UI（`frontend/components/arc/*`）、shadcn/ui（`frontend/components/ui/*`）、beUI Motion Table（`frontend/components/motion/table`，仅 client-side 大样本场景）。
- **统一壳优先**：`WorkspaceHeader`、`StatPillsBar`、`StudioDataTable`、`ModuleNavTabs`、`JobTable`/`JobTableToolbar` 模式。
- **不新造通用 UI**：新增组件仅在明确复用≥3处且无现成原语时抽象；优先扩展现有 wrapper/adapter，避免“万能超级组件”。
- **server-driven 约束**：管理列表分页/筛选/排序以后端为准；本地表格仅用于展示优化，不作为权威事实源。

## 组件与链接 / Components & Links

### Arc UI（已安装 registry：`@uiarc` → `https://uiarc.dev/r/{name}.json`）
- Tabs：<https://uiarc.dev/components/tabs>
- Badge：<https://uiarc.dev/components/badge>
- Segmented Control：<https://uiarc.dev/components/segmented-control>
- Combobox：<https://uiarc.dev/components/combobox>
- Date Range Picker：<https://uiarc.dev/components/date-range-picker>
- Search Field：<https://uiarc.dev/components/search-field>
- Button Group：<https://uiarc.dev/components/button-group>
- Empty State：<https://uiarc.dev/components/empty-state>
- Confirm Morph：<https://uiarc.dev/components/confirm-morph>
- Dialog：<https://uiarc.dev/components/dialog>
- Dropdown Menu：<https://uiarc.dev/components/dropdown-menu>
- Drawer：<https://uiarc.dev/components/drawer>
- Select：<https://uiarc.dev/components/select>
- Pagination：<https://uiarc.dev/components/pagination>
- Docs/AI：<https://uiarc.dev/docs/ai>

### shadcn/ui 基线（`frontend/components/ui/*`）
- Table：<https://ui.shadcn.com/docs/components/table>
- Tabs：<https://ui.shadcn.com/docs/components/tabs>
- Badge：<https://ui.shadcn.com/docs/components/badge>
- Dropdown Menu：<https://ui.shadcn.com/docs/components/dropdown-menu>
- Dialog：<https://ui.shadcn.com/docs/components/dialog>
- Alert Dialog：<https://ui.shadcn.com/docs/components/alert-dialog>
- Sheet：<https://ui.shadcn.com/docs/components/sheet>
- Drawer：<https://ui.shadcn.com/docs/components/drawer>
- Pagination：<https://ui.shadcn.com/docs/components/pagination>
- Select：<https://ui.shadcn.com/docs/components/select>

### beUI（registry：`@beui` → `https://beui.dev/r/{name}.json`）
- Home：<https://beui.dev>
- Motion Table：仅 client-side 大样本网格（如预览），不用于 server-driven 管理列表的本地分页/排序语义

## 问题 / Problem

1. **骨架重复**：卡片容器、工具栏布局、状态统计胶囊、加载/空/错态、分页条、刷新语义在各模块重复搭建，细节易漂移。
2. **交互原语不统一**：Tabs、标签/徽章、下拉菜单、弹窗、抽屉分属不同实现与样式约定，用户心智负担高。
3. **扩展成本高**：新增筛选项、批量操作或详情抽屉时，需要在每个模块复制模式，难以全局一致演进。
4. **回归盲区**：页面改造后缺少统一的点击回归清单，容易破坏既有交互链路。

## 目标 / Goals

1. **统一列表骨架（List Shell）**：为告警中心、错误巡检/错误中心、审计、用户中心提供一致的 server-driven 列表结构。
2. **统一交互原语选型与约定**：Tabs、Tag/Badge、Dropdown/Action Menu、Dialog/AlertDialog、Sheet/Drawer 的使用边界与样式层级在所有页面一致。
3. **全局页面换新**：工作台、控制台、监控、集群、主机、包/插件、命令、系统升级、仪表盘等页面对齐同一组件体系与布局契约。
4. **领域可插拔**：统一的是 **框架与原语**，不是强行共用一套列定义；各模块通过列配置、筛选器插件、行操作渲染器接入。
5. **功能回归保障**：每个改造页面必须提供 Egolite 点击回归用例，覆盖关键交互链路。

## 非目标 / Non-goals

- 不将 `JobTable`（sync 作业领域组件）直接套用到告警/审计/用户等非作业实体；`JobTable` 继续作为作业域参考实现与作业场景唯一实现。
- 不在本任务内重做各模块的业务逻辑、后端 API、权限模型或数据契约（除非为统一交互必须的小幅适配）。
- 不引入客户端本地排序/分页替代现有服务端分页（遵循 server-driven 约束）。
- 不一次性大改所有页面；采用分阶段迁移，允许并存期但需有明确收敛计划。

## 范围 / Scope

- **P0 覆盖（本设计主目标）**：工作台、控制台作业、告警中心、错误中心与巡检中心、审计日志、用户中心。
- **P1 参考对齐（可选后续）**：监控策略/集成、集群、主机、包/插件、命令、系统升级、仪表盘。
- **P2 保留对齐**：E2E 实验室、鉴权页面。

## 约束 / Constraints

- 遵循 `.trellis/spec/frontend/ui-conventions.md`：beUI/Arc 组件导入约束、零塌陷加载、卡片内嵌表格、表格右侧操作列吸附/实体化、分页规范、i18n（next-intl）与 a11y。
- 服务端驱动列表：分页/排序/筛选以服务端为准；`StudioDataTable`/`JobTable` 模式优先，不在通用 server list 中依赖第三方本地表格状态作为事实源。
- 受控 vendoring（PR #160 / 既有约定）：`frontend/components/motion/*`、`frontend/components/arc/*` 为受控依赖；升级走 re-pull/diff merge，项目适配集中在 wrapper/adapter；MIT 头与 `licenses/third_party_mit_files.txt` 合规。
- 最小变更：先抽共性骨架与原语约定，再逐模块迁移；不为未出现的第三类场景过度抽象。

## 验收标准（设计阶段）/ Acceptance (Design)

- [ ] 输出统一列表架构与分层（shell / toolbar / table / row-actions / overlays），明确与 `JobTable`、`StudioDataTable` 的关系。
- [ ] 给出 Tabs / Tag-Badge / Dropdown / Dialog / Sheet 的统一选型、适用场景与落点组件（含 Arc/beUI/shadcn 对照与链接）。
- [ ] 给出全局页面映射表（现状 → 目标组件/改造点/风险）。
- [ ] 给出分阶段实施计划与回归检查项（含 Egolite 点击回归、i18n、a11y、零塌陷、分页、权限操作确认）。

## 开放问题（待评审拍板）/ Open Questions

1. **共享组件落点**：新增 `frontend/components/common/data-table/`（通用 server-list 框架）还是扩展 `common/layout` + `common/sync` 既有模式？（建议：通用骨架进 `common/data-table` 或 `common/list-shell`，领域表格留在各域目录。）
2. **Tab 统一粒度**：页面级主导航 Tab（如 monitoring/diagnostics workspace）统一用 `ModuleNavTabs`/Arc Tabs 哪一种为主实现？内容区内筛选 Tab 是否统一为 `SegmentedControl`/pill？
3. **Drawer 统一**：详情/追踪统一优先 `Sheet`（右侧）还是允许 `Drawer`（底部/移动端）按场景？（需与现有 `AuditTraceSheet`、告警详情 Sheet 行为对齐。）
4. **批量操作**：告警/审计是否需要统一批量操作条（参考 `JobTableToolbar` bulk bar）与多选列协议？首期可不启用但需预留扩展点。
5. **beUI Table 引入边界**：除预览等本地大样本网格外，server-driven 管理列表是否继续以 `StudioDataTable`/原生 `Table` 为主，仅在明确的 client-side 场景引入 beUI Motion Table？（默认建议：保持 server-driven 列表不用 beUI Table 的本地排序/分页语义。）
6. **Egolite 回归范围**：需要先定义 Egolite 点击脚本的覆盖粒度与失败判定（建议：P0 页面至少覆盖“列表加载、筛选、分页、详情打开、创建/编辑/删除确认、跳转链路”）。
