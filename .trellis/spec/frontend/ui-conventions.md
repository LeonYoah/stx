# UI 约定

> STX 控制台（`frontend/`）的全局界面、通用组件、交互契约与技术防坑规范。

---

## 1. 页面布局、头部与图标

- **主框架与容器**：`app/(main)/layout.tsx` 提供底部常驻 Dock（`ManagementBar`）与顶部路由进度条（`RouteProgressBar`）；内容区为全宽自适应容器，`app/(main)/*` 单页**严禁**再套自带 `max-w-*`、居中或大 padding 的外层 `container`。
- **统一页头**：主页面头部统一使用 `components/common/layout/WorkspaceHeader`，`icon` 必须与 Dock 栏保持一致（`lucide-react`）：
  - 仪表盘 `LayoutDashboard` / 集群 `Layers` / 主机 `Server` / 数据同步 `Workflow` / 安装包 `Package` / 插件 `Boxes` / 诊断 `Activity` / 监控告警 `Bell` / 审计 `FileText` / 命令历史 `Terminal` / 用户 `Users`。
- **图标选型先确认，再改代码**：换图标或加新图标前，先在已安装的 `lucide-react`、`@tabler/icons-react` 中检索（Arc 无图标集，HeroUI 文档用 Gravity UI），按实际显示尺寸（如侧栏 16px）列出至少 3 个候选及取舍原因，等开发者确认后再改；仅用单枚未装库图标时内联 SVG，不引整包。
- **禁用状态光标**：点不了的按钮保持默认箭头，**禁止** `cursor: not-allowed`（避免系统红色禁止图标遮挡按钮）；禁用靠视觉变淡，原因放 Tooltip。

---

## 2. 信息密度、渐进式披露与防碎裂

- **拒绝无效横幅**：严禁在列表或卡片顶部堆砌静态说明文字；技术说明优先放在 Placeholder、标签旁 `Tooltip`、`HelpHint` 或极紧凑单行提示条。
- **渐进式披露**：严禁在单屏内把基础属性、多张重型明细表与复杂表单从上到下全量平铺；重型明细、日志与编辑交互通过二级 Tab、Drawer 或 Dialog 物理分流。
- **原子信息防被迫换行**：ID / Key / 键值对 / 状态胶囊 / URL / 代码单元不得因窄列挤压在字符中间折断。按场景选用：`flex-wrap` + `inline-flex items-center shrink-0 whitespace-nowrap`、`overflow-x-auto whitespace-nowrap`、或 `truncate` + `title` / `Tooltip`。
- **Radix `ScrollArea` 内截断坑**：`ScrollArea` 内部默认 `display: table; min-width: 100%`，弹性子项只写 `min-w-0 flex-1` + `truncate` 仍会被撑宽；截断子项须用 `w-0 min-w-0 flex-1`（或固定 `max-w-*`），树视口需覆写为 `display: block; min-width: 0`。

---

## 3. 列表、筛选、加载与表格卡片（`TableCard`）

- **状态概览与筛选**：有状态分布的列表页可使用 `StatPillsBar` 或 `SegmentedControl` + `Badge` 作一键筛选；筛选变化必须重置 `page = 1`。
- **筛选栏**：保持左对齐，搜索框限宽（如 `flex-1 min-w-[200px] max-w-sm`），输入项支持 **Enter** 直接触发主查询且不误触发清空/关闭等次要动作。
- **零塌陷加载与 `TableCard`**：带分页的列表优先复用共享原语 **`components/common/layout/TableCard.tsx`**（或单层主卡片 + `border-t bg-muted/10` 底栏）：
  - 首屏无数据时使用 `TableSkeletonRows`（加载超过 **300ms** 才显现，显现后至少保留 **500ms** 防闪烁）；
  - 刷新/翻页保留旧数据与容器高度，顶部显示 `TableLoadingBar`（数据行 `opacity-60 pointer-events-none`）；
  - 错误留在卡片内提供重试（已有旧数据时降为顶部提示条）。
- **分页规范**：统一使用 `@/components/ui/pagination`（或 Arc `pagination`），左总数/每页条数、右页码，**严禁**使用「上一步 / 下一步」文案。
- **响应式列隐藏**：CSS Grid 表格列使用 `data-*` 属性选择器配合 `@media` 隐藏窄屏列，**禁止**在行节点内联 `--cols` 样式（会覆盖 `@media` 回退导致列错位）。

---

## 4. 动效、表卡切换与浮动批量条

- **GSAP 入场**：表格行（`animateTableRows`）与卡片网格（`animateGridCards`）动效收敛在 `lib/animations/gsap-motion.ts`，时长 150–300ms、`easeOut`。
- **表卡切换与共享形变（`ViewToggle` + `useViewMorph`）**：
  - 同一列表在表格与网格卡片间切换时，切换按钮使用 `components/common/ViewToggle.tsx`（16px 图标 + `ariaLabel`），直接并入卡片头或筛选栏右侧，不另起占用整行空间的独立标题工具条。
  - 共享元素形变调用 `lib/animations/view-morph.ts` 的 `useViewMorph(scope)`：同一 item 在两种视图使用相同 `part` 锚点生成 `layoutId`，Motion 以 200ms tween 做 FLIP 形变，`useReducedMotion` 自动退化（外层需包 `LayoutGroup`）。
- **底部浮动批量操作条**：多选批量操作采用避让底部 Dock（`bottom-24`）的居中浮动条（仅 `selectedCount > 0` 时出现，清除收敛为紧凑 X 图标按钮），**禁止**在表格上方插入整行占位横条顶高表格；分栏页面与底部分页共存时定位在分页栏上方，不遮挡页码。

---

## 5. 弹窗、抽屉与长文本

- **shadcn 弹窗宽度硬要求**：`ui/dialog` / `ui/alert-dialog` 底座自带 `sm:max-w-lg`，调用处写不带断点前缀的 `max-w-*` 会在宽屏被覆盖导致大弹窗被锁成 512px。宽度一律写 `sm:max-w-*`（`components/ui/__tests__/dialog-width-classes.test.ts` 自动扫描拦截）；重内容弹窗显式放宽至 `sm:max-w-4xl` 以上并采用 `flex flex-col overflow-hidden` + 内部 `overflow-auto min-h-0 flex-1`（长 mono 日志禁止再包 `ScrollArea`）。
- **Arc 弹窗与共享未保存确认**：
  - Arc 弹窗走 `@/components/arc/dialog/dialog`（内容节点附带 `.arc-compact`，尺寸显式写 `w-[min(...)]!` / `max-h-[85vh]!`）。
  - 跨模块未保存拦截统一走 `components/common/layout/UnsavedChangesDialog`（支持两/三动作、禁用遮罩误点、默认焦点落在安全留守按钮），仅编辑脏态触发、新建取消直接退出，**禁止**在底栏做内联替换条或绝对定位硬盖。
- **只读长文本**：错误堆栈、运行日志、JSON/配置样本统一用 `components/common/layout/ExpandableTextPanel`（原生双向 `overflow-auto`，放大走 `WorkbenchDialogContent`）。

---

## 6. 凭据安全与异步引导

- **敏感凭据（Secret / Password / Token）**：查询与列表严禁回显明文（脱敏展示）；编辑框默认留空表示保留原值，仅显式输入新值时覆盖；工作台任务变量密文存 `custom_variables`、类型元数据存 `custom_variable_types`。
- **异步资源引导**：主机接入等涉及异步外部步骤的创建动作，提交后必须接续带一键复制命令与实时心跳侦听的状态闭环向导（如 `HostInstallGuideDialog`）。

---

## 7. 国际化、时间与产品文案

- **单语言与防缺键**：所有可见文案走 `lib/i18n/locales/{zh,en}.json`，**严禁**同屏出现「中文 / English」双语拼接；请求后端本地化文本时显式传 `lang`；后端动态枚举在调用翻译前必须先用 `t.has(key)` 检查，缺失时回落原值或兜底文案（不能靠 `try/catch`）。改动语言包后运行：
  ```bash
  cd frontend && pnpm exec vitest run lib/i18n/__tests__/translation-coverage.test.ts
  ```
- **时间显示**：时间格式化一律使用 `lib/timezone`（`formatDateTimeMinuteTz` / `formatTimeTz`），不直接调用 `next-intl` 的 `format.dateTime`。
- **产品语言（不暴露底层实现名）**：界面、提示与错误文案禁止出现 `Prometheus` / `Alertmanager` / `Grafana` / `PromQL`（SeaTunnel 配置键如 `metrics.prometheus.enabled` 除外）；接口实现枚举（`source:"prometheus"`、`reason:"prometheus_unavailable"`）只在代码内映射文案。监控中心面向用户只分 **集群级 / 作业级**（`scope=cluster|job`），底层及诊断自动策略仍保留 `source_type`（`platform_health` / `metric` / `job`）；判断 sync 告警阶段一律看 `alert_id` 的 `sync:` 前缀。

---

## 8. Arc 与 beUI 组件约定

- **安装与许可**：免费 Arc 组件装入 `frontend/components/arc/`（`cd frontend && pnpm dlx shadcn@latest add @uiarc/<name> --yes`），不覆盖 `components/ui/`，不安装 `@uiarc-pro`；beUI 放在 `components/motion/` / `components/charts/`（不覆盖仓库 `lib/i18n/`、`lib/utils.ts`、`lib/ease.ts`）。两类 vendored 文件均须保留 MIT 头、登记 `licenses/third_party_mit_files.txt`；对组件做局部 STX 扩展（如 `SegmentedControl.layoutGroup`、`Select`/`Combobox.displayValue`、`MultiSelect.emptyMessage`、`Dialog` ReactNode 标题与 `showCloseButton`、`NumberField` 空草稿隐藏等）必须加双语 `Local modification` 注释以便升级 diff 合并。
- **主题与全局紧凑尺度**：`app/layout.tsx` 引入一次 `@/components/arc/foundation.css`，`ThemeProvider` 的 `ArcThemeAttribute` 同步 `html[data-theme]`。`foundation.css` 与 `app/globals.css` `.arc-compact` 统一半胶囊控制台尺度（`--radius-control: 0.875rem` / 14px、`--radius-panel: 1rem` / 16px、`--radius-popover: 0.75rem` / 12px、`--control-height-sm: 1.875rem` / 30px、`--control-height-md: 2.125rem` / 34px）；挂到 `<body>` 下的 Portal 表面（`DrawerContent`、`DialogContent`）必须在内容节点显式附带 `.arc-compact`。
- **`SegmentedControl` 跨挂载点共享 `layoutGroup`**：同一逻辑分段开关渲染在两个互斥挂载点（切换视图时旧实例卸载、新实例挂载，如「通知渠道 | 投递记录」）时，两处必须传相同的 `layoutGroup` 字符串，确保选中胶囊跨实例平滑形变而非瞬移。
- **日期范围选择**：全局起止日期走 `components/common/layout/ArcDateRangePicker`（默认 `months={1}`、`maxDate` 用 `useToday()` 避免首帧 `new Date()` 水合差异、Apply 闭区间写到 `:59.999`）；需分钟精度时开 `withTime`（内置 24h `ClockField`，不用原生 `<input type="time">`）。
- **技术防坑**：
  - `localStorage` 状态首帧必须使用服务端一致的默认值，挂载后再同步，避免水合报错。
  - 整页 `html` 状态锁（如拖拽光标 `html[data-studio-resizing]`）必须写在非 CSS Module 文件中，否则 Next 生产构建报错。
  - 引入 Arc CSS Module 的单测依赖 `vitest.config.ts` 中 `css.postcss.plugins: []`。
  - beUI 图表进入视口才播放生长动画；多序列图表显式按语义色板（`TREND_COLORS` / `colorForLabels` / `SERIES_PALETTE`）着色，不依赖 Arc 默认灰阶。
  - `/monitoring` 旧子路由（`alerts` / `rules` / `history` / `integrations`）与旧参数（`policies` / `history` / `integrations` / `notifications` / `platform`）重定向并收敛至 `/monitoring?tab=alerts|rules|notify`。

---

## 9. 提交前检查

- [ ] 页面未使用冲突的外层 `container`，头部使用 `WorkspaceHeader` 且图标与 Dock 对齐；新/改图标已列出候选并经确认。
- [ ] 无顶部废话横幅与无脑平铺堆叠；原子字段有防被迫换行保护（`ScrollArea` 内截断用 `w-0 min-w-0 flex-1`）。
- [ ] 筛选栏左对齐且支持 Enter 提交；列表使用 `TableCard` 零塌陷加载与 `@/components/ui/pagination`（无「上一步/下一步」）。
- [ ] 表格/卡片切换用 `ViewToggle` + `useViewMorph`；多选批量条浮于 Dock 上方，不顶高表格；互斥挂载的 `SegmentedControl` 传共享 `layoutGroup`。
- [ ] shadcn 弹窗宽度使用 `sm:max-w-*`；Arc 弹窗/抽屉 Portal 节点挂 `.arc-compact`；编辑脏态离开走 `UnsavedChangesDialog`。
- [ ] 敏感凭据不回显明文、编辑留空保留原值；异步资源创建后有心跳闭环向导。
- [ ] 中英文文案完整且单语展示，动态后端值先 `t.has()` 检查，`translation-coverage.test.ts` 通过；时间走 `lib/timezone`；无底层实现名泄露。
- [ ] 新增/修改注释遵循「中文在前，英文在后」双语规范；vendored 组件保留 MIT 头与本地改动标注。
