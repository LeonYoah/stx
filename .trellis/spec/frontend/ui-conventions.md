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

- **管理列表详情标准**：常驻详情统一用 `components/common/layout/DetailHost`（≥1080px 常驻 rail、窄屏 Drawer、J/K 导航）+ `common/layout/list-detail.module.css`（`.page` / `.split` / `.listCol[data-mcols]` / `.row` / detail 类）；编辑脏态拦截用 `DirtyCloseDialog` / `UnsavedChangesDialog`（均在 `common/layout/`）。**不要**再为管理列表详情新造一套宿主；`ui/sheet` 只留给审计追踪等临时覆盖层。任务 `11-18-unified-admin-console-lists`（2026-11-21 选 A）本期迁移审计 / 用户（含令牌）/ 主机到此壳；主机 `/hosts/[id]` 应为列表深链接打开常驻详情，不再用 Sheet-only 详情页。告警 / 诊断已是该模式且本期不改；集群列表（卡片网格）与集群详情（多 Tab 整页）不硬套 DetailHost。
- **`StatPillsBar` 已弃用**：新代码不得使用。状态筛选改用 Arc `SegmentedControl`（单选，2–5 项或后端只接受单值）或 Arc `chip-group`（多选，如作业 7 态）；立即生效的单个开关条件用 Arc `Switch`。一排控件只表示一个维度。本期在令牌 / 主机退役；其余存量（含集群）随下期页面，最后一处迁走后删除组件。
- **筛选计数**：只有后端（或客户端已持有的**全集**）能给出与列表总数一致的分桶统计时，才在 `SegmentedControl` / `chip-group` 上显示计数（等宽数字、固定最小宽度；危险类 >0 用 `tone='danger'`）。**禁止**只按当前页行数做分桶计数。例：令牌用接口 `active/expired/revoked` 全量计数；主机分段无可靠全量分桶则**不显示数字**；告警分段可显示服务端轻量全集统计；错误 / 巡检保持无当前页桶计数。
- **筛选栏**：保持左对齐，搜索框限宽（如 `flex-1 min-w-[200px] max-w-sm`），输入项支持 **Enter** 直接触发主查询且不误触发清空/关闭等次要动作；筛选变化必须重置 `page = 1`。
- **零塌陷加载与 `TableCard`**：带分页的列表优先复用已有共享原语 **`components/common/layout/TableCard.tsx`**（勿重建）：
  - 首屏无数据时使用 `TableSkeletonRows`（加载超过 **300ms** 才显现，显现后至少保留 **500ms** 防闪烁）；
  - 刷新/翻页保留旧数据与容器高度，顶部显示 `TableLoadingBar`；
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
- **详情 vs 临时覆盖**：管理列表的行详情走 `DetailHost`（§3）；`ui/sheet` 仅用于临时覆盖（如审计 `AuditTraceSheet`）。主机详情从 Sheet 迁到 DetailHost 后，不要把常驻详情塞回右侧 Sheet。
- **Arc 弹窗与共享未保存确认**：
  - Arc 弹窗走 `@/components/arc/dialog/dialog`（内容节点附带 `.arc-compact`，尺寸显式写 `w-[min(...)]!` / `max-h-[85vh]!`）。
  - 跨模块未保存拦截统一走 `components/common/layout/UnsavedChangesDialog`（支持两/三动作、禁用遮罩误点、默认焦点落在安全留守按钮）；常规两动作场景直接用文案已绑定的 `DirtyCloseDialog`。仅编辑脏态触发、新建取消直接退出，**禁止**在底栏做内联替换条或绝对定位硬盖。
- **只读长文本**：错误堆栈、运行日志、JSON/配置样本统一用 `components/common/layout/ExpandableTextPanel`（原生双向 `overflow-auto`，放大走 `WorkbenchDialogContent`）。
- **确认分层**：破坏性 / 影响他人或批量 → `ui/alert-dialog`；单条、不可撤回、只涉及自己且有按钮可锚定 → Arc `ConfirmMorph`（每行最多锚定 1 个）；能撤回的操作可不确认。全站禁止 `window.confirm`。用普通 `Dialog` 承载的纯确认应改为 `AlertDialog`。

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

- **安装与许可**：免费 Arc 组件装入 `frontend/components/arc/`（`cd frontend && pnpm dlx shadcn@latest add @uiarc/<name> --yes`），不覆盖 `components/ui/`，不安装 `@uiarc-pro`；beUI 放在 `components/motion/` / `components/charts/`（不覆盖仓库 `lib/i18n/`、`lib/utils.ts`、`lib/ease.ts`）。两类 vendored 文件均须保留 MIT 头、登记 `licenses/third_party_mit_files.txt`；对组件做局部 STX 扩展必须加双语 `Local modification` 注释以便升级 diff 合并。
- **主题与桌面后台收紧尺度（任务 `11-18-unified-admin-console-lists` 目标）**：`app/layout.tsx` 引入一次 `@/components/arc/foundation.css`，`ThemeProvider` 的 `ArcThemeAttribute` 同步 `html[data-theme]`。控制台外层容器覆盖（不魔改各组件圆角源码；工作台 `studio-chrome.css` 独立）：
  - `--radius-control: 0.5rem`（**8px**）、`--radius-panel: 0.625rem`（**10px**）、`--control-height-md: 2rem`（**32px**）、`--control-height-sm: 1.75rem`（**28px**）；
  - 弹层 `--radius-popover: 0.625rem`（**10px**，定义在 `:root`，因 Portal 挂到 `body`），内边距 4px，选项圆角 6px、行高 32px；
  - `ConfirmMorph` / `Switch` 保持胶囊；不叠 `corner-shape: superellipse`。
  - Portal 表面（`DrawerContent`、`DialogContent`）内容节点须挂 `.arc-compact`。
- **同一块区域只用一套组件**：同一行筛选栏、同一个对话框、同一张表里，交互控件要么全是 Arc，要么全是 shadcn（§9 的 `ui/badge` 豁免）。换下拉时，同行搜索框 / 按钮 / 日期一起换。
- **控件选型**：

  | 场景 | 组件 |
  |---|---|
  | 少量选项下拉 | Arc `Select`；2–5 个短选项且互斥的筛选用 `SegmentedControl` |
  | 可搜索 / 长列表 / 数量不固定的数据 | Arc `Combobox` |
  | 行内「更多」 | Arc `Button` ghost + `iconOnly` + Arc `DropdownMenu`；≤2 个动作可直接摆 ghost |
  | 行内轻确认 | Arc `ConfirmMorph`（§5） |
  | 破坏性确认 | `ui/alert-dialog` |
  | 命令面板 | `ui/command`（不要拿它拼可搜索下拉） |
  | 状态标签 | 见 §9 |

- **可访问名称（强制）**：`Select` / `Combobox` 的可见 `label` 可为可选，但**去掉可见标签后必须仍有可见标签或 `aria-label` 之一**，控件不得缺少可访问名称。纯图标按钮同理必须传 `aria-label`。大盘头部集群范围若去掉 `[&_label]:sr-only` 兜底，须改为不传 `label` + 明确 `aria-label`（或保留可见标签），禁止「既无可见标签又无 aria-label」。
- **`SegmentedControl` 跨挂载点共享 `layoutGroup`**：互斥挂载点（如「通知渠道 | 投递记录」「巡检记录 | 自动」）必须传相同 `layoutGroup` 字符串。
- **日期范围选择**：全局起止日期走 `components/common/layout/ArcDateRangePicker`（默认 `months={1}`、`maxDate` 用 `useToday()`；需分钟精度时开 `withTime`）。
- **技术防坑**：
  - `localStorage` 状态首帧必须使用服务端一致的默认值，挂载后再同步，避免水合报错。
  - 整页 `html` 状态锁（如拖拽光标 `html[data-studio-resizing]`）必须写在非 CSS Module 文件中，否则 Next 生产构建报错。
  - 引入 Arc CSS Module 的单测依赖 `vitest.config.ts` 中 `css.postcss.plugins: []`。
  - beUI 图表进入视口才播放生长动画；多序列图表显式按语义色板（`TREND_COLORS` / `colorForLabels` / `SERIES_PALETTE`）着色，不依赖 Arc 默认灰阶。
  - `/monitoring` 旧子路由与旧参数重定向并收敛至 `/monitoring?tab=alerts|rules|notify`。

---

## 9. 状态标签（Badge）

- **保留原本 shadcn `ui/badge` 不动**：列表、详情与卡片里的状态标签及中性标签继续沿用 `@/components/ui/badge`，不做向 Arc `Badge` 的批量替换，也不修改 `components/ui/badge.tsx` 本身。
- **已有 Arc `Badge` 维持现状**：工作台定时侧栏、作业详情、升级向导以及 `SegmentedControl` / `chip-group` 计数处已使用的 Arc `Badge` 保持不动。
- **一致性与深色可读性**：同一域内配色与尺寸保持统一；新增或顺手修改时必须同时带好浅色与 `dark:` 对比度变体。
- **文案走 next-intl**：未知后端状态值用 `t.has()` 兜底。

---

## 10. 控制台首页（监控大盘 + 全局作业列表）

> 布局与视觉由 PR #168 落地；任务 `11-18-unified-admin-console-lists` **不改大盘布局**。`HomeDashboard` 头部 Combobox 的 `[&_label]:sr-only` 清理仅为可选小遗留（须保留可见 label 或 `aria-label`），不阻塞审计 / 用户 / 主机三页迁移。

- **结构**：`app/(main)/dashboard/page.tsx` 只装配 `HomeDashboard`（`components/common/dashboard/`）。顶部 `WorkspaceHeader`（操作区放范围选择与刷新）→ `KpiStrip` 指标条 → 「需要处理」横条 → Arc `Tabs`（集群 / 作业 / 主机 / 平台 / 作业列表）。页签、范围（`cluster`）与列表筛选写在 URL（`useDashboardQuery`）。区块页签按内容宽度：Arc `listShell` 是 `inline-flex`，在 `flex-col` 父级里外包 `<div className='flex'>`。
- **头部集群范围**：Arc `Combobox`（集群名不固定，需搜索）。`label` 可选；去掉历史 `[&_label]:sr-only` 后必须保留可见标签或 `aria-label`（§8）。`data-testid='home-scope'` 透传到输入框；`all` → `clusterId: undefined`；当前 `cluster` 不在已加载列表时保留 `clusterFallback`。
- **指标条（`KpiStrip`）**：一底板 7 格，格间「右 + 下」1px inset 分隔（`DASHBOARD_CELL_DIVIDER`）；数字 24px 量级；`tone` 只染数字：失败 / CDC 超阈 / 严重告警 / 集群异常 → danger，仅降级 → warning。涨跌符号只给「越大越好」的写入速率。首屏用 `KpiStripSkeleton`。
- **面板层级（`DashboardPanel`）**：`primary` / `secondary` 两档；`empty` 收成 `h-11` 矮条；`flush` 去内边距用于指标页签行与表格。
- **吞吐页签（`ThroughputTabs`）**：读 / 写 × 行 / 字节 4 格驱动 `BrushChart`，**不再**使用分段控件；与 `ClusterScopeStrip` 共用 `use-roving-keys.ts`。
- **集群范围条（`ClusterScopeStrip`）**：替代大圆角 `RadioCards`；底板 + 分隔线格子，`radiogroup` + roving tabindex，选中格顶部 accent 色条；保留健康徽标与节点 / 运行 / 堆 / 告警说明。
- **作业列表**：**不要**恢复已删除的「全部 / 正式 / 预览」运行类型分段控件；状态筛选仍为工具栏现状（`StatPillsBar`，计数来自 `with_counts`）；`chip-group` 多选替换不在本期三页迁移范围内。详情就地走 `JobDetailHost`，不复刻工作台整页。
- **需要处理**：无事项时只渲染无框单行文字，不留空边框。
- **图表选型 / 配色 / 时间窗 / 轮询 / 空态 / 产品文案**：延续既有大盘约定（一个指标一种图；`colorForLabels` / `SERIES_PALETTE`；自定义窗口写 URL `start`/`end`，前端截取 366 天；`usePolledResource` 30s；`TrendEmptyState` 按 `reason` 区分；界面不出现底层实现名）。
- **平台页签**：`tab=platform`，主体 `PlatformTab`；快照 `getPlatformSnapshotSafe()`；降级用 `TrendEmptyState`；Agent 表用 `StudioTable`。

---

## 11. 提交前检查

- [ ] 页面未使用冲突的外层 `container`，头部使用 `WorkspaceHeader` 且图标与 Dock 对齐；新/改图标已列出候选并经确认。
- [ ] 无顶部废话横幅与无脑平铺堆叠；原子字段有防被迫换行保护（`ScrollArea` 内截断用 `w-0 min-w-0 flex-1`）。
- [ ] 管理列表详情是否用 `DetailHost`（而非常驻 `ui/sheet`）？脏态离开是否走 `DirtyCloseDialog` / `UnsavedChangesDialog`？
- [ ] 筛选栏左对齐且支持 Enter 提交；列表使用既有 `TableCard` 零塌陷加载与 `@/components/ui/pagination`；新代码是否未再引入 `StatPillsBar`？筛选计数是否来自全集/服务端而非当前页？
- [ ] `Select` / `Combobox` / 纯图标按钮是否具备可见标签或 `aria-label`（控件不得无名）？本期迁移页若用 Combobox 是否满足？大盘头部 `sr-only` 清理可选、不阻塞三页。
- [ ] 同一行筛选栏 / 对话框 / 表内是否只用一套控件（`ui/badge` 豁免）？下拉 / 可搜索下拉 / 行内更多 / 行内确认是否按 §8 选型？弹层是否走 `--radius-popover`？
- [ ] 状态标签是否保留原本 `ui/badge`（或既有 Arc `Badge`），并兼顾 `dark:` 对比度？
- [ ] 表格/卡片切换用 `ViewToggle` + `useViewMorph`；多选批量条浮于 Dock 上方；互斥挂载的 `SegmentedControl` 传共享 `layoutGroup`。
- [ ] shadcn 弹窗宽度使用 `sm:max-w-*`；Arc 弹窗/抽屉 Portal 节点挂 `.arc-compact`；无 `window.confirm`。
- [ ] 敏感凭据不回显明文；异步资源创建后有心跳闭环向导。
- [ ] 中英文文案完整且单语展示，动态后端值先 `t.has()`；时间走 `lib/timezone`；无底层实现名泄露。
- [ ] 新增/修改注释遵循「中文在前，英文在后」；vendored 组件保留 MIT 头与本地改动标注。
- [ ] 大盘是否仍用 `KpiStrip` / `ThroughputTabs` / `DashboardPanel` / `ClusterScopeStrip`，且未恢复「全部/正式/预览」分段？指标条涨跌符号是否只给「越大越好」的写入速率？
