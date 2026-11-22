# UI 约定

> STX 控制台（`frontend/`）的全局界面、通用组件、交互契约与技术防坑规范。本文件为当前唯一有效的前端 UI 规范，已移除迁移前的旧版 shadcn 样式与过渡期写法。

---

## 1. 页面布局、头部与图标

- **主框架与容器**：`app/(main)/layout.tsx` 提供底部常驻 Dock（`ManagementBar`）、顶部路由进度条（`RouteProgressBar`）与全宽自适应内容区（已含 `pb-28` 避让 Dock）。`app/(main)/*` 单页**严禁**再套自带 `max-w-*` 居中或大 padding 的外层 `container`；控制台业务页根容器统一挂 `.arc-compact`。
- **统一页头**：主页面头部统一使用 `components/common/layout/WorkspaceHeader`，同域切换用 `ModuleNavTabs`，二级页面域用 Arc `TabsList` 放入 `actions`（参考 `MonitoringWorkspace`、`DiagnosticsWorkspace`）。`icon` 与 Dock 保持一致（`lucide-react`）：
  - 仪表盘 `LayoutDashboard` / 集群 `Layers` / 主机 `Server` / 数据同步 `Workflow` / 安装包 `Package` / 插件 `Boxes` / 诊断 `ScanSearch` / 监控告警 `BellRing` / 审计 `FileText` / 命令历史 `Terminal` / 用户 `Users`。
- **图标选型先确认，再改代码**：换图标或加新图标前，先在已安装的 `lucide-react`、`@tabler/icons-react` 中检索（Arc 无图标集，HeroUI 文档用 Gravity UI），按实际显示尺寸（如侧栏 16px）列出至少 3 个候选及取舍原因，等开发者确认后再改；仅用单枚未装库图标时内联 SVG，不引整包。
- **禁用状态光标**：点不了的按钮保持默认箭头，**禁止** `cursor: not-allowed`（避免系统红色禁止图标遮挡按钮）；禁用靠视觉变淡，原因放 `Tooltip` 或 `DisabledReasonTooltip`。

---

## 2. 单层画布、圆角、排版与信息密度

- **单层扁平画布 + `0.5px` 细线（核心视觉约束）**：
  - 详情页、右侧详情栏、Tab 主面板与管理类弹窗一律**压平为单层卡片/画布**，去掉内部多余的嵌套 `<Card>`、`rounded-lg border bg-card shadow-xs`、多余阴影及虚线占位盒。
  - 各信息分组通过统一留白（如首组 `pb-5`、后续各组 `py-5`、末组 `pt-5`）配合一条 **`0.5px` 细线**（`border-t-[0.5px] border-border/80` / `border-b-[0.5px] border-border/80`）自然分隔。
  - 画布内嵌表格不额外包一层 `rounded-lg border` 或虚线外框，直接落在画布上并使用 `[&_tr]:border-b-[0.5px] [&_tr]:border-border/70` 分行。
  - 单行路径、Maven 坐标、命令摘要等只读条用轻底色承托（如 `rounded-md bg-muted/50 px-3 py-2`），不额外套描边。
- **递进圆角与 Token 一致性**：
  - 容器与控件消费 `--radius-panel`（16px）/ `--radius-control`（14px）/ `--radius-popover`（12px），内层嵌套容器遵循 `inner = outer - padding`（不低于 4px）；状态表达统一用语义 Token 与 `SoftChip` / `NeutralChip`，禁止业务组件硬编码 `bg-green-500` / `bg-yellow-500` / 散落色值。
- **排版与原子信息防碎裂**：
  - 计数、版本号、端口、PID、内存、进度、时间等数字字段一律加 `tabular-nums`，防止更新时列宽跳动。
  - ID / Key / 路径 / URL / 命令 / 状态胶囊等原子信息**禁止** `break-all` 硬折断；表格列与紧凑详情中的截断文本/路径统一复用 `components/common/layout/TruncatedText`（仅在溢出时于悬停 180ms 内弹出主题化 Radix `Tooltip` 显示完整原文，禁止仅靠原生 HTML `title` 属性兜底；长路径使用 `mode='path'` 中间省略保留尾部关键段，并按需开启 `copyable` 一键复制）。单行长文本也可按场景选用 `overflow-x-auto whitespace-nowrap`，或弹性流整体换行（`flex-wrap` + `shrink-0 whitespace-nowrap`）。
  - **Radix `ScrollArea` 内截断坑**：`ScrollArea` 内部默认 `display: table; min-width: 100%`，弹性子项只写 `min-w-0 flex-1` + `truncate` 仍会被撑宽；截断子项须用 `w-0 min-w-0 flex-1`（或固定 `max-w-*`），树视口需覆写为 `display: block; min-width: 0`。
- **反噪音与渐进式披露**：
  - 严禁在列表或卡片顶部堆砌静态长说明；解释性文案收进 `Tooltip`、`HelpHint` 或 `EmptyState`。
  - 严禁在单屏内平铺过多同级 Tab（一般 ≤6 个）或把基础属性、重型明细表与表单全量堆叠；关联子视角通过内部 Arc `SegmentedControl` 折叠分流。

---

## 3. 控件语义与浮层分工

- **选择控件选型（按语义而非外观）**：
  - **即时开关 vs 表单勾选**：`Switch` 仅用于**拨动后立即持久化生效**的设置项（旁置轻量保存态，失败回滚）；需要点底部「保存 / 提交」才生效的布尔选项一律用 `Checkbox`。禁止用两个 Radio 表示开/关。
  - **单选数量规则**：2–5 个固定短选项一律用 Arc `SegmentedControl`（一目了然、一次点击，禁止藏进 `Select`）；≥6 项或选项会动态增长（如集群列表、版本号、主机）用 Arc `Select` / 可搜索 `Combobox`；带说明行的 2–5 个选项用竖排 Radio。
  - **值选择 vs 动作菜单**：选值用 `Select` / `Combobox`（选中后回显当前值）；执行动作（编辑、重启、升级、删除）用 Arc `DropdownMenu`（触发按钮保持原名，不放进表单字段区）。
  - **历史组件限制**：`StatPillsBar` 仅为历史兼容组件；新页面与重构页面筛选统一走 Arc `SegmentedControl`，严禁将 `StatPillsBar` 用作跨 Tab 跳转或往搜索框写字符串的混杂控件。
- **浮层组件选型**：
  - `Tooltip`：纯文本补充（图标按钮说明、截断全文、禁用原因），严禁在 `Tooltip` 内放可点击操作。
  - `AlertDialog`：高危、不可逆或跨节点/跨人影响的二次确认（如启停/重启/删除集群、批量节点操作）；必须写清受影响对象与不可逆后果，确认按钮使用**具名动作**（如「停止集群」「删除集群」），禁止泛化的「确定」。
  - Arc `Dialog`（`@/components/arc/dialog/dialog`）：短表单、独立配置或聚焦查看；内容节点必挂 `.arc-compact`，尺寸显式写 `w-[min(...)]!` / `max-h-[85vh]!`。
  - `DetailHost`（≥1080px 常驻栏 / <1080px 贴边 Arc `Drawer`）：列表项速览、长表单或需对照列表切换浏览的详情；严禁用抽屉做确认弹窗。
  - 独立整页路由：多域重型工作台或需独立分享 URL 的主流程（如单集群深度控制台、三阶段升级流）。
- **未保存脏态拦截**：所有含编辑态的 `Dialog`、`Drawer`、常驻详情栏及 Tab / 子视角切换，统一走 `components/common/layout/DirtyCloseDialog`（或 `UnsavedChangesDialog`）；仅编辑脏态触发，新建取消直接退出，禁止底栏内联替换条或遮罩硬盖。
- **shadcn 弹窗兜底要求**：存量未迁至 Arc 的 `ui/dialog` / `ui/alert-dialog` 底座自带 `sm:max-w-lg`，宽弹窗宽度一律写 `sm:max-w-*`（受 `components/ui/__tests__/dialog-width-classes.test.ts` 门禁保护）。
- **只读长文本与日志**：错误堆栈、节点/升级日志、JSON 样本使用 `ExpandableTextPanel` 或原生双向 `overflow-auto` 容器，**严禁**对长 mono 日志再包 Radix `ScrollArea`。

---

## 4. 列表、筛选、加载、空态与详情分栏

- **单行工具栏（`layout.toolbar`）**：
  - 状态/短枚举筛选用 Arc `SegmentedControl`，搜索用 Arc `SearchField`（支持 `/` 快捷键聚焦、`Esc` 清空、Enter 或防抖触发查询，**不再配冗余的独立「搜索」按钮**），排序与视图切换放右侧 `layout.toolbarRight`。
  - 筛选与页码支持 URL 查询参数双向同步；任意筛选变化重置 `page = 1`；异步拉取必须带请求序号竞态保护（忽略过期响应）。
  - 禁止仅按当前页条目统计并展示与总数矛盾的状态数字（无后端全量分桶计数时直接展示纯文字分段）。
- **唯一主卡片与零塌陷加载（`TableCard`）**：
  - 带分页或独立加载态的列表统一复用 **`components/common/layout/TableCard.tsx`**：
    - 首屏无数据时走 `TableSkeletonRows`（加载超过 **300ms** 才显现，显现后至少驻留 **500ms** 防闪烁）；
    - 刷新/翻页保留旧数据与可交互性，只亮顶部 `TableLoadingBar`（一次等待只留一种指示，**禁止**整表加 `opacity-60 pointer-events-none` 压暗禁点）；
    - 请求失败保留已有旧数据，在卡片顶部或卡片内展示错误与「重试」按钮（仅无旧数据时占满卡片）；
    - 独立表单/详情卡首次加载使用同构骨架屏，严禁居中单个 Spinner 替换整卡。
- **空态（Arc `EmptyState`）**：
  - 统一使用 `@/components/arc/empty-state/empty-state`，必须区分「首次无数据（配功能微缩示意与创建主动作）」与「筛选无结果（配一键清空筛选）」；严禁使用虚线边框 + 一行居中灰字占位。
- **分页规范**：分页器整合在 `TableCard` 底部 footer（`border-t bg-muted/10`），统一使用 `@/components/ui/pagination`，左总数/每页条数、右页码，**严禁**在卡片外悬浮孤立分页条，**严禁**使用「上一步 / 下一步」文案。
- **响应式列隐藏**：CSS Grid 表格列使用 `data-*` 属性选择器配合 `@media` 隐藏窄屏列，**禁止**在行节点内联 `--cols` 样式（会覆盖 `@media` 回退导致列错位）。
- **列表 + 常驻详情分栏（`DetailHost` + #178 克制联动）**：
  - 统一复用 `components/common/layout/DetailHost` + `common/layout/list-detail.module.css`（`.page` / `.pageViewport` / `.split` / `.listCol[data-mcols]` / `.row`）。
  - **联动外观（克制版）**：≥1080px 下 `.split` 存在 `> .rail[data-state='open']` 且本页有 `.rowActive` 时，列表自动退为底层色面板（卡片无框、行线消失、间距收 0）、选中行浮起并与详情栏 10px 凹圆角相接、其余行按距离逐级变淡（不模糊）、工具栏降透明；关闭时播完退出动画再卸载（600ms 兜底）。翻页后本页无选中行时不进入联动。
  - **接入约束**：行必须是 `.tableScroll` 的**直接子元素**，选中行挂 `.rowActive`；`rowHead`、`[data-slot='table-card-footer']` 与 `.tableScroll` 须同卡片。
  - **名称飞入**：行内名称节点挂 `data-detail-name`，仅在**行点击 / 行内 Enter 打开或切换**时写入 `flyOriginRef`（J/K、头部 `↑↓`、深链恢复不写）；`DetailHost` 配 `nameSlot='title' | 'meta'`；窄屏、`prefers-reduced-motion`、来源与落点文字不一致时自动降级不飞。

---

## 5. 动效、表卡切换与浮动批量条

- **语义缓动与克制时长**：
  - 禁止给整页或列表套用 500–600ms 的慢级联入场动画（不再新增整页 GSAP / Motion stagger 入场）；动效优先服务于状态连续性（`useViewMorph`、#178 联动与名称飞入、按钮图标就地反馈）。
  - 统一时长与曲线：小组件/悬浮/按压 100–200ms、面板/分栏/弹窗 150–300ms；入场与快速落位用 `cubic-bezier(0.22, 1, 0.36, 1)`（`--detail-ease`），退场用 `ease-in` 且短于入场，屏幕内位移/分段滑块用 `ease-in-out`，`linear` 仅用于旋转加载图标与进度条。必须尊重 `prefers-reduced-motion`。
- **表卡切换与共享形变（`ViewToggle` + `useViewMorph`）**：
  - 列表在表格与卡片网格间切换统一用 `components/common/ViewToggle.tsx`（16px 图标 + `ariaLabel`），并入工具栏右侧。
  - 共享元素形变调用 `lib/animations/view-morph.ts` 的 `useViewMorph(scope)`：同一 item 在双视图使用相同 `part` 锚点生成 `layoutId`，Motion 以 200ms tween 做 FLIP 形变，外层包 `LayoutGroup`，`useReducedMotion` 自动退化。
- **底部浮动批量操作条（`BatchBar`）**：
  - 多选批量操作统一复用 `components/common/layout/batch-bar.tsx`（搭配 `usePagedSelection`），浮于底部 Dock 上方（`bottom-24`）且仅在 `selectedCount > 0` 时出现；**禁止**在表格头部或上方插入占位横条顶高表格；分栏页面与底部分页共存时避让分页栏。

---

## 6. 凭据安全与异步引导

- **敏感凭据（Secret / Password / Token）**：查询与列表严禁回显明文（脱敏展示）；编辑框默认留空表示保留原值，仅显式输入新值时覆盖；工作台任务变量密文存 `custom_variables`、类型元数据存 `custom_variable_types`。
- **异步资源引导**：主机接入等涉及异步外部步骤的创建动作，提交后必须接续带一键复制命令与实时心跳侦听的状态闭环向导（如 `HostInstallGuideDialog`）。

---

## 7. 国际化、时间与产品文案

- **单语言与防缺键**：所有可见文案走 `lib/i18n/locales/{zh,en}.json`，**严禁**同屏出现「中文 / English」双语拼接；请求后端本地化文本时显式传 `lang`；后端动态枚举在调用翻译前必须先用 `t.has(key)` 检查，缺失时回落原值或兜底文案（不能靠 `try/catch`）。改动语言包后必须运行：
  ```bash
  cd frontend && pnpm exec vitest run lib/i18n/__tests__/translation-coverage.test.ts
  ```
- **时间显示**：时间格式化一律使用 `lib/timezone`（`formatDateTimeMinuteTz` / `formatDateTimeTz` / `formatTimeTz`），不直接调用 `next-intl` 的 `format.dateTime`。
- **产品语言（不暴露底层实现名）**：界面、提示与错误文案禁止出现 `Prometheus` / `Alertmanager` / `Grafana` / `PromQL`（SeaTunnel 配置键如 `metrics.prometheus.enabled` 除外）；接口实现枚举（`source:"prometheus"`、`reason:"prometheus_unavailable"`）只在代码内映射文案。监控中心面向用户只分 **集群级 / 作业级**（`scope=cluster|job`），底层及诊断自动策略仍保留 `source_type`（`platform_health` / `metric` / `job`）；判断 sync 告警阶段一律看 `alert_id` 的 `sync:` 前缀。

---

## 8. Arc 与 beUI 组件约定

- **安装与许可**：免费 Arc 组件装入 `frontend/components/arc/`（`cd frontend && pnpm dlx shadcn@latest add @uiarc/<name> --yes`），不覆盖 `components/ui/`，不安装 `@uiarc-pro`；beUI 放在 `components/motion/` / `components/charts/`（不覆盖仓库 `lib/i18n/`、`lib/utils.ts`、`lib/ease.ts`）。两类 vendored 文件均须保留 MIT 头、登记 `licenses/third_party_mit_files.txt`；对组件做局部 STX 扩展必须加双语 `Local modification` 注释以便升级 diff 合并。
- **主题与全局紧凑尺度**：`app/layout.tsx` 引入一次 `@/components/arc/foundation.css`，`ThemeProvider` 的 `ArcThemeAttribute` 同步 `html[data-theme]`。`foundation.css` 与 `app/globals.css` `.arc-compact` 统一半胶囊控制台尺度（`--radius-control: 0.875rem` / 14px、`--radius-panel: 1rem` / 16px、`--radius-popover: 0.75rem` / 12px、`--control-height-sm: 1.875rem` / 30px、`--control-height-md: 2.125rem` / 34px）；挂到 `<body>` 下的 Portal 表面（`DrawerContent`、`DialogContent`）必须在内容节点显式附带 `.arc-compact`。
- **`SegmentedControl` 跨挂载点共享 `layoutGroup`**：同一逻辑分段开关渲染在两个互斥挂载点时，两处必须传相同的 `layoutGroup` 字符串，确保选中胶囊跨实例平滑形变而非瞬移。
- **日期范围选择**：全局起止日期走 `components/common/layout/ArcDateRangePicker`（默认 `months={1}`、`maxDate` 用 `useToday()` 避免首帧水合差异、Apply 闭区间写到 `:59.999`）；需分钟精度时开 `withTime`（内置 24h `ClockField`，不用原生 `<input type="time">`）。
- **技术防坑**：
  - `localStorage` 状态首帧必须使用服务端一致的默认值，挂载后再同步，避免水合报错。
  - 整页 `html` 状态锁（如拖拽光标 `html[data-studio-resizing]`）必须写在非 CSS Module 文件中，否则 Next 生产构建报错。
  - 引入 Arc CSS Module 的单测依赖 `vitest.config.ts` 中 `css.postcss.plugins: []`。
  - beUI 图表进入视口才播放生长动画；多序列图表显式按语义色板（`TREND_COLORS` / `colorForLabels` / `SERIES_PALETTE`）着色，不依赖 Arc 默认灰阶。
  - `/monitoring` 旧子路由与旧参数重定向并收敛至 `/monitoring?tab=alerts|rules|notify`。

---

## 9. 提交前检查

- [ ] 业务页根容器挂 `.arc-compact`，未使用冲突的外层 `max-w-*` 容器；头部用 `WorkspaceHeader` 且图标与 Dock 对齐；新/改图标已列出候选并经确认。
- [ ] 详情页、右侧详情栏与管理类弹窗均压平为单层画布 + `0.5px` 细线分隔，无嵌套 `<Card>`、无冗余阴影与虚线占位盒。
- [ ] 控件与浮层语义准确：`Switch` 即时生效、表单布尔用 `Checkbox`、2–5 短选项用 `SegmentedControl`、高危确认用具名动作 `AlertDialog`；编辑态离开走 `DirtyCloseDialog`。
- [ ] 列表页采用单行工具栏（Arc `SearchField`、无冗余「搜索」按钮）+ `TableCard`（300/500ms 骨架、刷新保留旧数据可交互只亮 `TableLoadingBar`、卡片内错条重试、区分场景的 Arc `EmptyState`、底部分页）；无按当前页拼凑的假状态计数；筛选/页码同步 URL 并具竞态保护。
- [ ] 列表 + 详情分栏复用 `DetailHost` 与 #178 联动约束；表/卡切换用 `ViewToggle` + `useViewMorph`；多选批量条用底部浮动 `BatchBar`；互斥挂载的 `SegmentedControl` 传共享 `layoutGroup`。
- [ ] 数值统一 `tabular-nums`，路径/标识符/日志无 `break-all` 硬折；长 mono 日志不包 Radix `ScrollArea`；Arc 弹窗/抽屉 Portal 节点挂 `.arc-compact`（存量 shadcn 弹窗宽度写 `sm:max-w-*`）。
- [ ] 敏感凭据不回显明文、编辑留空保留原值；异步资源创建后有心跳闭环向导。
- [ ] 中英文文案完整且单语展示，动态后端值先 `t.has()` 检查，`translation-coverage.test.ts` 通过；时间走 `lib/timezone`；无底层实现名泄露。
- [ ] 新增/修改注释遵循「中文在前，英文在后」双语规范；vendored 组件保留 MIT 头与本地改动标注。
