# 实施计划（草案）：全局 UI 换新与统一管理控制台

> 本文档为设计启动后的实施拆解草案；具体子任务拆分、排期与 owner 在设计评审后细化。

## Phase 0：设计评审与决策冻结

- [ ] 评审 `prd.md` / `design.md`，确认范围（P0 全局页面）与非目标
- [ ] 拍板开放问题：
  - [ ] 通用列表壳落点（`common/list-shell` vs `common/data-table` vs 扩展 `StudioDataTable`）
  - [ ] 页面级 Tab 主实现（`ModuleNavTabs` / `ui/tabs` / Arc `tabs`）与收敛策略
  - [ ] 详情容器策略（`Sheet` 为主，`Drawer` 移动端补充的边界）
  - [ ] 批量操作首期是否纳入（多选列 + bulk bar 协议）
  - [ ] Egolite 回归覆盖粒度与失败判定
- [ ] 确认 Arc/beUI 组件引入清单与版本/安装方式（`pnpm dlx shadcn@latest add @uiarc/<name>` / `@beui/<name>`），核验 registry API 与本地现状差异

## Phase 1：通用骨架与原语（无业务语义变更优先）

- [ ] 抽取/实现服务端表格卡片壳（loading/empty/error/pagination/refreshing 插槽），对齐 `StudioDataTable` 行为
- [ ] 抽取工具栏布局 primitives（搜索、筛选槽位、刷新、可选 bulk 条），统一筛选 patch `page=1` 规则
- [ ] 提供/复用受控 query hook 模式（page/pageSize/filters/sort + 请求参数构建 + 状态机）
- [ ] 统一行操作渲染协议（主操作 + DropdownMenu + 危险确认接入点）
- [ ] 补充最小单测（query patch、分页边界、状态映射纯函数等，按实际抽取范围）

## Phase 2：试点迁移（建议：审计 或 用户中心）

- [ ] 选择试点模块并冻结行为基线（截图/关键用例）
- [ ] 将试点模块接入统一壳与 toolbar/table/pagination 模式
- [ ] 对齐 Dialog/AlertDialog/Sheet 交互与 i18n/a11y
- [ ] 回归：筛选、分页、刷新、详情抽屉/表单提交流程
- [ ] Egolite 点击回归（按矩阵）

## Phase 3：诊断模块收敛（错误中心 + 巡检中心）

- [ ] 统一 `DiagnosticsErrorCenter` 列表骨架与筛选/详情 Sheet 模式
- [ ] 统一 `DiagnosticsInspectionCenter` 列表骨架，保留启动巡检 `Dialog` 并规范刷新
- [ ] 校验与告警/任务/集群跳转联动参数不回归
- [ ] Egolite 点击回归（按矩阵）

## Phase 4：告警中心迁移（复杂度最高）

- [ ] 拆分迁移步骤：先壳与 toolbar，再行操作/批量（如有），最后详情与排障面板联动
- [ ] 保持告警状态转换、确认/静音/处理等业务规则与权限校验
- [ ] 全量回归告警筛选、深链、详情 Sheet、相关跳转
- [ ] Egolite 点击回归（按矩阵）

## Phase 5：工作台与控制台换新

- [ ] 工作台任务/作业列表与详情链路对齐 `JobTable` 模式
- [ ] 控制台作业列表与详情链路对齐
- [ ] 深链参数与轮询/overlay 语义不回归
- [ ] Egolite 点击回归（按矩阵）

## Phase 6：P1 页面换新

- [ ] 监控其它页（rules/history/integrations）
- [ ] 集群管理与升级向导
- [ ] 主机管理
- [ ] 安装包/插件
- [ ] 命令历史
- [ ] 系统升级
- [ ] 仪表盘
- [ ] Egolite 点击回归（按矩阵）

## Phase 7：收尾与规范沉淀

- [ ] 清理各模块重复样板与死代码（在确认无引用后）
- [ ] 更新前端 spec（如 `ui-conventions.md` 增补“管理列表统一壳/交互原语”约定）与必要 README/注释（仅按需）
- [ ] 记录 vendored 组件差异与升级注意事项（Arc/beUI），同步 `licenses/third_party_mit_files.txt`（若文件集变化）
- [ ] 质量验证：相关模块测试、`tsc --noEmit`、lint（区分存量基线）

## 验收检查（跨阶段）

- [ ] P0 页面骨架一致：头部/统计/工具栏/表格卡片/分页/加载与空错态
- [ ] Tabs/标签/下拉/弹窗/抽屉选型与样式层级符合设计决策
- [ ] server-driven 语义正确：筛选重置页码、分页与排序以后端为准、无误导性交互
- [ ] i18n 与 a11y 覆盖新增/改动 UI；无布局塌陷与明显交互回归
- [ ] `JobTable`（作业域）与新通用壳职责清晰，无领域逻辑反向依赖
- [ ] Egolite 回归矩阵通过（P0 全部，P1 至少主路径）
