# 实现计划：连接器与作业配置模板

## 阶段 0：准备

- [x] 确认 PRD/Design 已评审；`task.py start 09-23-connector-job-templates`
- [x] 开发前读：`.trellis/spec/backend/{directory-structure,database-guidelines,quality-guidelines}.md`、`.trellis/spec/frontend/{ui-conventions,api-and-services}.md`、`guides/cross-layer-thinking-guide.md`

## 阶段 1：后端精选模板

- [x] 新增 model `CuratedTemplate`（含 `section`: env|source|transform|sink|combo）+ AutoMigrate
- [x] 种子目录：落地已审定 `curated-draft/`（env/source/transform/sink）；loader 单测
- [x] Repository：CRUD、按 owner 过滤、按 builtin_id 查副本
- [x] Service：List 合并视图、Fork、CreateFromContent（另存）、**ParseFourSections**（一键组合）
- [x] Handler + router + `operation` 注册（226 ops / route_baseline 已对齐）
- [x] CLI：`stx sync curated list|create|update|delete|fork|render|parse-combo`
- [x] 单测：合并规则、fork、四节解析、禁止删内置、升级种子不影响 DB、凭证变量覆盖

**验证**：`go test ./internal/apps/sync/ ./internal/operation/ ./internal/cmd/` 已通过

## 阶段 2：前端「模板」Tab 改造

- [x] i18n 中英：模板 / 精选 / 原始默认参数；废弃「配置模板」用户文案
- [x] `SettingsSidebarPanel`：Tabs；精选按 section 分组列表+插入；原始默认参数迁入现有三下拉
- [x] sync.service + types：curated-templates API（含 update）
- [x] 插入：env/source/transform/sink 分别落入对应节（transform 同 source/sink 插入工具扩展）
- [x] 另存 Dialog（选区优先，否则全文）；**一键组合须带上存在的四节**
- [x] 树右键另存入口 → 已改为编辑器选区右键「另存为精选」
- [x] 右侧「模板管理」Tab：我的精选改/删；内置创建副本
- [x] 精选插入列表纯插入，不再混入编辑/副本操作

**验证**：`tsc --noEmit` + vitest curated helpers 已通过；手动 UI 冒烟待用户确认

## 阶段 3：种子与组合验收

- [x] `curated-draft/` 与正式种子一致（20 条，含 transform/Hive 三变体/四类文件 Source）
- [x] 一键组合：含 transform 的作业另存后四节齐全（单测）
- [x] 冒烟：凭证类 `{{}}` 可被 `detectVariables` 扫出（单测）

## 阶段 4：质量门禁

- [x] `trellis-check`（curated 范围 PASS）
- [x] 前端 lint/typecheck；必要单测
- [ ] 更新 spec（可选，`trellis-update-spec` 记 sync 模板约定）

## 回滚点

1. 仅后端合并前：不注册路由即可无影响。
2. 前端 Tab 合入后：临时只渲染原始默认参数 Tab。
3. DB 表可保留；无需 destructive down migration。

## 明确不做（本迭代）

- 块内对照补全
- 原始默认参数结果自动入库
- 改 java-proxy 模板渲染语义

## Review Gate（start 前）

- [x] PRD 决策收口（叫法/Tab/存储/不做对照补全/分块四节/变量策略）
- [x] design.md 边界与契约
- [x] 精选草稿 v2 已审定
- [x] 用户确认可 `task.py start` 进入实现
