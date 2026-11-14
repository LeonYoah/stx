# UI 约定

> 现代 STX 控制台布局、组件、加载动效、高密度数据展示与交互规范。

---

## 概述

STX 控制台采用现代化控制台体验架构：
- **主框架布局**：底部常驻 macOS 风格悬浮 Dock 导航栏（`ManagementBar`）+ 顶部轻量路由进度条（`RouteProgressBar`）+ 统一页面级自适应容器（`@container/main`）。
- **统一页面头部**：全站主页面标准化采用 **`WorkspaceHeader`**（面包屑导航、大标题、副标题描述、与 Dock 严格对齐的语义图标、右侧操作区）。
- **数据量化与状态筛选**：列表及概览页统一使用 **`StatPillsBar`**（状态胶囊汇总栏 + 快捷点击联动筛选）。
- **信息密度哲学**：坚决杜绝在列表与卡片顶部堆砌冗长的“无效说明文案”，首屏垂直空间 100% 让渡给真实业务数据。
- **非阻塞平滑加载**：淘汰会导致容器高度塌陷闪烁的大 Spinner，统一采用初次骨架屏（`TableSkeletonRows`）与刷新非阻塞进度条（`TableLoadingBar`）。
- **表格卡片收口**：表格与底部分页栏封装于统一卡片内，两极对齐，使用规范页码与箭头，杜绝“上一步/下一步”。
- **弹窗美学与分级**：消除无边框感，重内容弹窗（详情、巡检、配置、DAG）显式放宽并采用内部滚动。
- **向导式闭环引导**：新增关键异步/物理资源后，必须提供带有操作命令与实时心跳侦听的引导向导。
- **保密凭据安全**：敏感密码/Token 字段不回显、保存后脱敏，编辑时支持“保持原密码”与“重写覆盖”。
- **微动效赋能**：借助 GSAP 微动效库为表格行与卡片网格赋予克制丝滑的入场动效。

新增或重构前端 UI 时必须严格遵循本约定。

---

## 1. 页面主布局与常驻 Dock 栏

- **主布局架构**：由 `app/(main)/layout.tsx` 控制，采用“无全局侧栏 + 底部常驻 Dock 栏”设计。
  - 底部常驻悬浮 Dock 栏（`ManagementBar`）保持持续挂载，在路由切换时**零卸载、零重绘**；
  - 搭配客户端预取（Prefetching）与顶部路由加载指示（`RouteProgressBar`），保证页面跳转极致丝滑。
- **页面容器与内边距**：
  - 内容区使用全宽自适应容器：`w-full max-w-none px-3 sm:px-4 lg:px-6 xl:px-8 2xl:px-10 py-6 mb-8`；
  - `(main)` 下的所有单页**严禁**再额外嵌套自带 `max-w-*`、居中或重复 padding 的外层 `container`，以免与全局主容器冲突。
- **页面结构**：各子路由 `page.tsx` 保持轻量，负责组装对应的 `WorkspaceHeader` 与功能主组件（如 `ClusterMain`、`HostMain`、`DiagnosticsInspectionCenter`）。

---

## 2. 页面标题与头部规范（`WorkspaceHeader`）

- **统一标准组件**：主页面头部**必须统一使用** `components/common/layout/WorkspaceHeader`，**严禁**各页面随意手写 `flex items-center gap-3` 等零散标题块。
- **语义图标一致性**：`WorkspaceHeader` 的 `icon` 属性必须与底部 Dock 栏保持严格一致的语义图标（来自 `lucide-react`）：
  - 控制台仪表盘：`LayoutDashboard`
  - 集群管理：`Layers`
  - 主机管理：`Server`
  - 数据同步：`Workflow`
  - 安装包管理：`Package`
  - 插件市场：`Boxes`
  - 诊断中心：`Activity`
  - 监控告警：`Bell`
  - 审计日志：`FileText`
  - 命令历史：`Terminal`
  - 用户管理：`Users`
- **图标选型先确认，再改代码**：
  - 换图标或加一枚新图标时，不要按名字从 Lucide 里随手挑一个就提交。先去项目已经依赖的 `lucide-react`、`@tabler/icons-react` 里找，再看当前界面用的组件库：Arc / `@uiarc` 没有图标集；HeroUI 文档用的是 Gravity UI（`@gravity-ui/icons`），不是 HeroUI 自带。需要时再看 Heroicons、Phosphor。
  - 按图标真正出现的尺寸看，侧栏是 16px。放大图好看不够。和旁边已经定下来的图标放在一起比，避免时钟撞上定时、地球当成全局、分支当成版本。
  - 至少列出 3 个候选，写清来源、图标名，以及不建议的原因。等开发者点名确认后再改代码。开发者没点头之前，保持现有图标。
  - 确认后优先用已经装上的库。只有一枚、且来自还没安装的库时，内联这一个 SVG，不要为了一枚图标加整包。
- **结构化配置**：
  - 支持清晰的面包屑导航（`breadcrumbs`）；
  - 副标题简要阐述页面职责（如“STX 集群运行状态与节点拓扑”）；
  - 右侧统一集成 `actions` 操作槽（刷新按钮、主创建按钮、辅助配置等），按钮增加 `active:scale-[0.98]` 触感微反馈。

---

## 3. 状态统计与胶囊筛选（`StatPillsBar`）

适用于集群、主机、告警、诊断、审计等**具备状态分布的数据驱动型列表页**。

- **组件标准**：优先使用 `components/common/layout/StatPillsBar` 呈现核心量化数据（如总数、在线/离线、成功/失败/运行中、关键告警数等）。
- **交互联动**：
  - 状态胶囊不仅是数据呈现，更应当作为**一键快速筛选器**；
  - 点击对应状态胶囊时，联动触发筛选状态切换并重置分页为第 1 页，让“查看概览”与“检索数据”无缝衔接。
- **防膨胀原则**：
  - 坚决避免为展示几个状态数字而铺设多张占满大半屏的巨型卡片；
  - 状态胶囊横向排列于头部下方，高度紧凑收敛，为下方数据列表腾出最大化的视口空间。

---

## 4. 高信噪比哲学与渐进式披露（Anti-Noise & Progressive Disclosure）

这是 STX 控制台在所有页面、卡片、弹窗与详情组件中确立的**最高视觉与信息架构原则**。高密度（High Density）不等于无序堆砌（Clutter）；高密度是通过消除视觉杂质、紧凑对齐来提高信息获取效率，而堆砌则是将不同认知负荷、不同优先级的复杂组件全量平铺在同一视口内。

### 4.1 首屏数据与聚焦优先（Primary Focus First）
- **单一视觉目标**：每个容器（单页 / 弹窗 / 卡片）在任意时刻只解决一个主要的视觉焦点；
- **首屏数据优先**：
  - **严禁**在卡片顶部、列表顶部堆砌大段不可交互、显而易见的静态说明文本（例如“本页面用于管理集群安装包，您可以点击上传或者查看……”等废话横幅）；此类无效文字会挤占 100~200px 宝贵的垂直高度，导致真正重要的数据列表需要用户滚动才能看到。

### 4.2 渐进式披露与反平铺堆叠（Progressive Disclosure vs. Flat Dumping）
- **严禁全量平铺展开（Anti-Flat-Dumping）**：
  - **严禁**在同一个长滚动容器中，将“基础属性 + 多个多行数据表格 + 复杂新增/编辑表单”从上到下全量平铺堆叠；
  - 这种反模式会导致用户首屏抓不住重点，视觉充斥大量同等权重的边框与输入框，产生极强的拥挤感和压迫感。
- **工作台右侧是高级属性，上下两段都是**：设置页上段只放任务属性（执行模式、集群、变量），下段只放运行属性（可观测性、超时、告警）。任务变量只保留一个标题，不要再套「自定义变量」卡片。精选模板和插件默认参数都在「模板管理」页签里用分段视图切换，不要把模板浏览器再塞进设置页。两段各自滚动，不要再合成一条长列表。
- **多维信息物理隔离（Dimensional Isolation）**：
  - 复杂实体必须按职责维度进行物理分流：
    - **概览（Overview）**：仅展示决策核心（核心指标、关键状态胶囊、基础属性、主操作入口），保持清爽克制与视觉呼吸感；
    - **重型明细（Details / Sub-tables / Logs）**：从属数据表格、日志流、事件历史必须隔离在独立的二级 Tab、侧边抽屉（Sheet）或独立弹窗中；
    - **高阶配置 / 表单（Configuration / Edit）**：复杂表单或上传组件独立收纳，高级可选参数默认收入折叠面板（Accordion）按需展开。
- **轻量速览与下钻穿透（Snapshot & Drill-Down）**：
  - 主视图若需体现次级从属实体的存在，**只允许采用轻量级的“状态速览条（Snapshot Bar）/ 微型状态胶囊（Stat Pill）+ 显式一键下钻动作”**（例如：`包含 3 项关联实体 → [查看与配置明细]`）；
  - 严禁直接把次级模块的完整复杂表格平铺在主视图中。

### 4.3 提示信息轻量化与紧凑布局
- **提示轻量化**：
  - 若确需提供技术说明、依赖约束或注意事项，优先采用：
    1. 输入框内的占位提示（Placeholder）；
    2. 字段标签旁带 `Tooltip` 气泡；
    3. 极紧凑单行警告条（内边距微小的 Warning Banner）；
    4. 或收纳在“帮助向导”的抽屉/弹窗内，按需展开。
- **两极对齐与紧凑布局**：
  - 工具栏、筛选栏、表格底栏均采用极简紧凑的排版，最大化提升信息密度与交互效率。

---

## 5. 非阻塞平滑加载体系（Zero-Collapse Visual Loading）

彻底淘汰传统“数据一刷新就出现整屏/整卡片大 Spinner，导致容器瞬间塌陷闪烁”的粗暴加载方式。

- **初次加载（Initial Loading）**：
  - 当页面或列表尚无任何缓存数据时，使用 **`TableSkeletonRows`** 或骨架卡片进行占位，预估行高与列数，保持容器骨架稳定。
- **异步刷新 / 翻页 / 搜索（Background Refreshing）**：
  - 表格或列表容器顶部挂载 **`TableLoadingBar`**（2px 渐变非阻塞动态进度条）；
  - 数据行保持渲染，仅附加透明度淡化与点击拦截：`opacity-60 pointer-events-none transition-opacity duration-200`；
  - 容器尺寸始终保持**零塌陷、零跳动**，用户能清晰感知旧数据正在被后台无缝替换。

---

## 6. 筛选栏（列表/搜索页）

- **对齐**：筛选栏在项目内必须**左对齐**。不要用 `w-full` 把整栏拉满、把操作推到最右，或加弹性空白把操作推到末尾。
- **搜索框**：使用限定宽度（如 `flex-1 min-w-[200px] max-w-sm`），避免搜索框过宽；其他筛选（下拉、按钮）自然排在右侧。
- **提交交互**：凡是带明确“搜索 / 刷新 / 应用过滤”主按钮的筛选区，除鼠标点击按钮外，所有可编辑输入项（如文本、数字、日期）都应支持 **Enter** 直接触发该主操作。
- **主次动作隔离**：Enter 只触发主动作（如搜索、刷新、应用过滤），不应误触发“清空条件”“关闭弹窗”等次要按钮。

---

## 7. 弹窗设计美学与尺寸分级标准（Dialog Hierarchy & Width）

### 7.1 视觉层级与边框质感
- 解决“弹框看着跟没边框似的”苍白问题：
  - `DialogContent` 必须保持明确的外边框与立体感：`border bg-background shadow-lg`；
  - Header、Body 与 Footer 之间通过清晰的分隔线 `border-b` / `border-t` 区隔；
  - Footer 底栏采用微弱的层次底色（如 `bg-muted/15` 或 `bg-muted/10 px-6 py-3`），形成扎实稳重的收口。

### 7.2 弹窗尺寸严格分级
严禁所有场景混用默认狭窄弹窗。按内容体量显式划分为三级：
1. **轻量确认 / 简单操作（Small）**：
   - 适用：删除确认、快速重启、轻量重命名等；
   - 尺寸：`sm:max-w-md` ~ `sm:max-w-lg`。
2. **标准表单 / 中型配置（Medium）**：
   - 适用：新建用户、编辑集群标签、普通规则表单；
   - 尺寸：`sm:max-w-xl` ~ `sm:max-w-2xl`。
3. **重内容 / 详情 / 巡检报告 / 日志流 / DAG / 代码预览（Large & Heavy）**：
   - 适用：主机详情弹窗、自动巡检发现报告、安装插件详情、DAG 预览、配置 Diff 比较；
   - 必须显式在 `DialogContent` 上放宽尺寸，严禁挤在窄框内：
     - `sm:max-w-4xl` 或 `sm:max-w-5xl`，甚至特定全屏级工具使用 `w-[92vw] max-w-[1400px]`；
     - 高度限制：`max-h-[85vh]` 或 `h-[88vh]`；
     - 结构收口：`flex flex-col overflow-hidden`，内部内容区通过 `overflow-auto min-h-0 flex-1` 自行滚动；**长 mono 日志/堆栈/证据禁止用 `ScrollArea` 包一层**（默认只有纵向条，易裁切长行）。

> **断点前缀是硬要求**：`DialogContent` / `AlertDialogContent` 底座自带 `sm:max-w-lg`，调用处写不带前缀的 `max-w-4xl` 会在宽屏被底座覆盖，大弹窗被锁成 512px（曾导致步骤名被挤成「预检/查」）。宽度一律写 `sm:max-w-*`；`components/ui/__tests__/dialog-width-classes.test.ts` 会扫描全部弹窗并拦截违规写法。

### 7.3 只读长文本面板（ExpandableTextPanel）
适用于错误堆栈、运行日志、配置/JSON 预览、证据样本等**只读长文本**：

- 统一使用 `frontend/components/common/layout/ExpandableTextPanel`：
  - 紧凑区用原生 `overflow-auto`（双向滚动），**禁止**再包 `ScrollArea`；
  - 「放大查看」必须走 `WorkbenchDialogContent`（约 `94vw` / 最高 `1400px` / `88vh`），禁止手写窄 `max-w-5xl` 放大框；
  - `wrap=false`（默认）保留行并横向可滚；`wrap=true` 用于 JSON/配置折行预览；
  - `tone='dark'` 用于堆栈/日志；`tone='plain'` 用于浅色边框场景；
- 短摘要、卡片内一两行预览可继续本地写 `<pre>`；**可编辑**大文本放大（如排障方案表单）不并入本组件，沿用既有字段内放大编辑模式。

---

## 8. 关键运维链路的向导式引导交互（Actionable Guidance）

适用于主机接入、集群创建、初始化部署等**涉及异步外部操作或多步骤链路的场景**。

- **拒绝冷冰冰的“提交即关闭”**：
  - 用户新增资源（如新增主机）后，仅弹出一个“添加成功”并直接关闭弹窗会导致用户陷入迷茫（不知道还要复制命令到目标机器运行）；
- **向导式闭环引导规范**：
  - 关键资源创建成功后，必须自动或引导弹出“操作指引与接入向导”（如 `HostInstallGuideDialog`）；
  - 提供环境检测提示、一键复制单行命令按钮（带成功反馈）；
  - 内置实时心跳侦听机制（通过定时轮询或 SSE 监听 Agent 上线）；
  - 当监听到真实心跳后，自动将界面流转为“已接入成功”的高亮庆祝状态，形成有始有终的完整交互闭环。

---

## 9. 敏感保密凭据（Secret / Password）与变量体系规范

适用于全局变量、任务自定义变量、数据源连接密码、访问密钥（AccessKey/SecretKey）、认证 Token 等场景。

- **存储与传输原则**：
  - 敏感凭据保存后，后端接口**严禁**再明文返回密码原文；
  - 前端展示统一进行保密脱敏（如固定显示 `••••••••`、锁定为不可反查状态并标注“保密”胶囊）；
- **任务定义与运行时兼顾契约（DataSync Variables Contract）**：
  - 在数据同步工作台中，任务级自定义变量同时支持普通文本（`string`）与保密凭据（`secret`）；
  - 真实值写入 `task.definition.custom_variables`（以供 SeaTunnel 任务引擎无感知安全注入）；
  - 变量类型元数据写入 `task.definition.custom_variable_types`（例如 `{"DB_PASS": "secret"}`），确保前端重新加载或编辑时准确呈现脱敏锁与安全状态；
- **编辑交互契约**：
  - 打开编辑态时，密码框默认保持空状态或置灰占位，并明确提示“留空保持原密码不变”；
  - 仅当用户在编辑框内重新键入新密码时，才将新明文提交并覆盖更新；若保持留空，前端自动回填并保留原始敏感值；
  - 杜绝在列表内直接暴露行内明文输入框，统一通过专属弹窗（如 `CustomVariableDialog`、`GlobalVariableDialog`）进行严谨的类型选型与键名校验（避免冲突平台内置时间保留字）；
  - 变量项支持快捷复制变量引用语法 `{{KEY}}`，单击直接复制并向用户弹出明确成功的 Toast 反馈。

---

## 10. 表格卡片一体化、视口保底高度与底部分页规范（Table Card & Pagination）

适用于用户管理、主机列表、审计日志、命令历史、告警列表等**所有带分页的数据表格页面**。

- **一体化收口（Table Card Integration）**：
  - 表格与分页栏必须作为统一的实体封装在同一个卡片或容器内（如 `border rounded-lg relative overflow-hidden bg-card/40 shadow-xs`）；
  - 分页栏作为表格底栏（Table Footer），紧接在表格下方，使用 `border-t bg-muted/10`（或 `bg-muted/15`）区隔并添加适当内边距（如 `px-4 py-2.5`）；
  - **严禁**让分页控制栏脱离表格外框、孤立悬空在下方的整片大空白区域中。
- **视口自适应保底高度与底部锚定（Viewport-Fitting Min-Height & Bottom-Anchored Pagination）**：
  - **痛点预防**：当表格仅检索到 1~2 行数据时，若不设保底高度，表格卡片会瞬间塌陷为一条极其细窄的缝隙，分页栏浮在屏幕上方，卡片下方留下 70% 的黑色视觉空洞，严重破坏企业级桌面端控制台质感；
  - **结构规范**：
    - 外层 Table Card 容器必须采用 Flex 纵向列流并声明视口响应式最小高度：`flex flex-col flex-1 min-h-[480px] sm:min-h-[calc(100vh-270px)]`；
    - 中间表格内容区声明伸缩扩展：`flex-1`（或包装在 `flex-1 min-h-0` 中）；
    - 底部分页栏声明底部外边距自适应吸底：`mt-auto border-t bg-muted/10`；
    - **效果保障**：在数据量极少时，卡片自然撑开填满首屏黄金可视区，分页栏始终稳稳扎根在卡片底部；在数据多页或展开时，高度自然向外延伸滚动，兼顾美学稳定性与内容延展性。
- **两极对齐版式（Polar Layout）**：
  - **左侧**：展示数据体量指标（`共 {total} 条`）及可选的每页条数选择器（`每页 [10/20/50/100] 条`）；
  - **右侧**：展示紧凑精美的页码操作区（上一页箭头、具体页码按键、当前激活页高亮、多页省略号 `...`、下一页箭头）。按钮尺寸使用轻量图标键（`h-8 w-8`）或紧凑型按钮。
- **语义与文案严谨性（Semantic Precision）**：
  - **严禁**在表格分页中使用“上一步 / 下一步”（`previous / next` 文案）；此类文案专属于 Wizard 向导或分步表单；
  - 分页必须使用“上一页 / 下一页”（`prevPage / nextPage`）或直接使用语义明确的左右箭头图标与数字页码。
- **组件复用**：
  - 统一使用 `@/components/ui/pagination` 组件，不得各页面随意手写独立且样式割裂的分页标签；
  - 保持完整的双语国际化支持（`useTranslations('common')`）。

---

## 11. GSAP 声明式微动效体系（Micro-Interactions）

为了提供丝滑的桌面级工业质感，项目在 `lib/animations/gsap-motion.ts` 中封装了统一的 GSAP 微动效。

- **表格数据入场**：在表格外层挂载 `ref={tableContainerRef}`，利用 `useGSAP` 监听数据与 loading 变化，触发 `animateTableRows('.xxx-data-row')` 产生轻微的错峰平滑进场。
- **卡片网格入场**：对集群卡片、工作台节点卡片等网格，触发 `animateGridCards` 实现自然排列动画。
- **动效克制原则**：动效时间控制在 150ms~300ms 之间，缓动采用 `easeOut`，重点在于消除生硬闪烁，绝不可阻碍用户的正常快速操作。

---

## 12. 高度收敛、对齐与内部滚动

适用于监控中心、诊断中心、任务中心、日志中心这类**信息密度高、左右分栏、卡片内列表很多**的页面。

### 12.1 外层布局：同一行对齐靠父级 Grid 控制
- 若存在“左侧创建/列表 + 右侧详情”的结构，不要左右两列各自 `space-y-*` 堆叠；
- 上下对齐靠父级统一网格（Grid）定位，不靠子列自己凑高度。

### 12.2 固定高度 + 内部滚动
- 任务列表、执行步骤、节点明细、日志流等内容区优先使用 **固定高度 / 最大高度 + 内部滚动**；
- 模式：外层卡片 `flex flex-col overflow-hidden`，内容区 `flex-1 min-h-0`，内嵌 `ScrollArea`；
- 原则：**外层负责收口，内层负责滚动**。

### 12.3 文本截断与 Hover 查看全量
- 凡是做了 `truncate` / 单行省略的字段，必须提供 hover 查看完整值的入口（`title` 或 `Tooltip`）；
- 严禁为了显示长文本而破坏既有表格宽度与网格对齐。

### 12.4 原子信息完整性与防被迫换行（Atomic Integrity & Anti-Forced-Wrapping）
- **核心定义**：
  - **原子数据单元在视觉上具有不可分割性**；
  - 凡是属于原子语义的信息——包括技术标识符（ID / Code / UUID / Key）、单行键值对（Key-Value Item）、复合状态胶囊、度量数值带单位、文件路径与 API URL、单行代码片段，**严禁因容器宽度不足而在单词/字符中间被机械切断折行**。
- **常见反模式（严禁出现）**：
  - **机械等宽栅格切分**：盲目使用 `grid-cols-2 / grid-cols-3 / grid-cols-4` 将容器切成若干细窄列（如单列仅 ~200px），导致较长标识符或键值被迫折成两截（如 `key: \n value`，或单词中间折断）；
  - **行内徽标折叠**：状态徽标或标签内部的文字被上下挤压折叠。
- **三种通用排版策略（严禁无策略的自然断行）**：
  1. **策略 A：弹性自适应流（整块换行，内部不拆）**：
     - 适用：并列的键值对、标签组、属性卡片；
     - 样式：父级 `flex flex-wrap items-center gap-x-4 gap-y-2`，子单元声明 `inline-flex items-center shrink-0 whitespace-nowrap`。允许整个键值单元整体换到下一行，绝不在单元内部断开。
  2. **策略 B：单行溢出滚动保护（保持绝对单行工整）**：
     - 适用：代码条、命令行、长路径、复合坐标（如 Maven GAV）、单行横向流水步骤；
     - 样式：`flex items-center gap-2 overflow-x-auto whitespace-nowrap scrollbar-none`，子项保持在一条物理水平线，超宽时横向滚动，保证绝对不折行，并配有一键复制。
  3. **策略 C：单行截断 + 全量悬浮穿透**：
     - 适用：表格数据列、列表中的长名称、单行描述；
     - 样式：`truncate max-w-[...]` 配合 `title` 或 `Tooltip`，以优雅单行省略替代破相折行。
     - **在 `ScrollArea` 或按内容定宽的容器里，只写 `truncate` 不够**：Radix `ScrollArea` 的内层是 `display: table; min-width: 100%`，会按子内容的最小宽度（min-content）撑开；`whitespace-nowrap` 的长文本即使 `truncate`，它的最小宽度仍是整行文字，会把整张卡片撑宽、挤出可视区（`min-w-0` 不能阻止这一点）。截断元素所在的弹性子项要用 `w-0 min-w-0 flex-1`（或固定的 `max-w-[...]`），让它不参与撑宽，只占剩余空间。
     - 实例：工作台右侧设置栏的告警摘要（`TaskAlertSection`）最初只写了 `min-w-0 flex-1` + `truncate`，长摘要把"运行与集群"卡片撑出侧栏；改为 `w-0 min-w-0 flex-1` 后与改动前宽度一致。改动侧栏这类容器后，要在 1024～1920 几种窗口宽度下对比改动前后卡片的实际宽度。

---

## 13. 国际化（i18n）

- **语言包**：在 `lib/i18n/locales/` 下（如 `zh.json`、`en.json`）。所有用户可见文案统一走 i18n key。
- **单语言可切换展示**：界面在切换语言后仅展示当前目标语言；**严禁**同屏采用“中文 / English”斜杠拼接或并排展示。
- **跨层语言传递**：对于诊断报告、日志分析、模板说明等由后端生成的文本，前端请求时必须显式传参 `lang`，保证双语切换一致性。
- **静态文案键**：代码中通过 `useTranslations` 直接引用的字符串键，必须同时存在于 `zh.json` 与 `en.json`。`frontend/lib/i18n/__tests__/translation-coverage.test.ts` 会扫描生产代码并检查缺键。
- **语言包结构**：测试必须递归比较全部叶子键，不能只比较语言包第一层；ICU 参数（如 `{count}`）也必须在中英文中保持一致。
- **动态后端值**：审计 action、资源类型、状态等由后端返回的动态值，不得直接调用可能缺键的 `t(dynamicKey)`。必须先调用 `t.has(dynamicKey)`；不存在时显示服务端原值或明确的兜底文案，避免 `next-intl` 在页面运行时报告缺键错误。

```tsx
// 错误：try/catch 无法阻止 next-intl 先报告缺键错误。
// Wrong: try/catch cannot stop next-intl from reporting a missing key first.
try {
  return t(dynamicKey);
} catch {
  return fallback;
}

// 正确：先检查文案是否存在，再调用翻译函数。
// Correct: check message existence before calling the translator.
return t.has(dynamicKey) ? t(dynamicKey) : fallback;
```

国际化改动至少运行：

```bash
cd frontend
pnpm exec vitest run lib/i18n/__tests__/translation-coverage.test.ts
```

---

## 14. Arc 组件（工作台）

- 免费组件放在 `frontend/components/arc/`。从 `frontend/` 执行 `pnpm dlx shadcn@latest add @uiarc/<name> --yes`。注册表是 `frontend/components.json` 的 `@uiarc`。不要覆盖 `components/ui/`，也不要安装 `@uiarc-pro`。
- `app/layout.tsx` 引入一次 `@/components/arc/foundation.css`。Arc 只认 `html[data-theme]`。`ThemeProvider` 里的 `ArcThemeAttribute` 把 `resolvedTheme` 写成 `light` 或 `dark`。Tailwind 继续用 `class`。
- 界面文案仍走 next-intl，一次只显示一种语言。`SearchField.clearLabel`、`CopyButton.copiedLabel` 和 `failedLabel` 是给中文用的可选文案，默认保持英文。不要改组件的动画实现。
- 顶栏不要给每个动作单独描边再留缝，那会像一串胶囊。按 Cursor / Figma / HeroUI Toolbar：状态和侧栏开关无边框；探查用一组 `ExpandingButtonGroup`；保存、发布、运行、停止用一组 `button-group` outline，Savepoint 变体进末尾菜单。不要用 `solid`（深色主题会反成白）。不要把「版本管理」塞进菜单。发布弹窗取消用 Arc `button` secondary，确认用 primary（`loading` 跟发布中）。保存段跟随当前草稿：保存中显示「保存中」；已有文件且草稿干净显示「已保存」；没有文件，或正文、右侧栏、全局变量把草稿弄脏，立刻回到「保存」。不要用定时器把已保存打回保存。`reserve` 锁住「保存 / 保存中 / 已保存」宽度。保存失败且草稿仍脏时必须停在「保存」。发布段只打开对话框。测试、DAG、预览不写 `doneLabel`。定时侧栏不要套卡片：标题 + `switch` + 状态 `badge` 一行，长说明进 HelpHint；Cron 预设用 `chip-group`，解除绑定用 `confirm-morph`，展开后单独占一行。顶栏三列是「面包屑 0.7fr / 搜索 280px / 动作 max-content」。搜索槽固定 280px 并靠左，`ExpandingSearch` 的壳是绝对定位，必须在槽内从左往右展开，不要盖住保存/运行。不在本地列表里的数字 Job ID 仍走 Cmd/Ctrl+P 对话框。顶栏把 `--control-height-md` 收到 `2.25rem`。右侧栏和控制台用 `.studio-sidebar-chrome` 同一套行高。发布按钮用短文案「发布」，弹窗标题仍用「发布新版本」。文件树范围只保留「我的 | 全部」：我的 = 自己的任务 + 公开任务。模板管理用分段控件切换「精选」和「插件默认」；插件默认的长说明收进 HelpHint，不要在列表上堆一段静态提示。精选分区用 `chip-group` 多选，来源用分段控件，插件列表用 `combobox`（选中即插入）。全局变量类型用分段控件，时间类别用 `chip-group` 单选。底部控制台「任务 / 日志 / 预览 / Checkpoint」也用分段控件。打开文件后默认停在「任务」，不要切到「日志」。任务表和预览表用 `StudioDataTable`：HeroUI Table 的解剖（粘性表头、列排序、表内空状态、底栏分页），颜色走 Arc token，不要装 `@heroui/react`，也不要 uppercase 表头。任务 / 预览贴满控制台，不要再套一层卡片边框；预览分页放 `StudioTableFooter`，刷新用 `TableLoadingBar` 而不是居中 Spinner。行内操作保持 ghost。预览左右栏用 Arc `ResizablePanels`：拖动分隔条改宽度，拖过最小宽度或按 Enter 收起数据集栏，不要再写死 12.5rem。作业详情页签用 Arc `Tabs`。指标「快照 | 趋势」用 `SegmentedControl`，放在页签同一行右侧，说明进 HelpHint，不要在页签下面再叠一条分段。状态用 Arc `Badge`，恢复 / 停止用 Arc `Button`。侧栏文案用短标签，长说明放 tooltip 或 hint，不要靠 `truncate` 把功能入口吞掉。侧栏标题不要 uppercase。
- 左侧文件树用 beUI File Tree（`frontend/components/motion/file-tree.tsx`）做展开、选中高亮和键盘导航，不要换成 Arc Tree view 或 File dropzone。右键、行内重命名/新建、拖拽移动仍由 `TreeView` 适配层保留。安装 `@beui/*` 时不要写入 `lib/i18n.tsx`，那会覆盖 next-intl 的 `lib/i18n/`；文案走 `ariaLabel` / `labels` 或 next-intl。目录栏标题行只放「资源」计数和一个「资源操作」菜单，不要并排堆新建目录/文件/展开/刷新四枚图标。树行也不要并排悬停操作图标，以免挤掉文件名；这些动作走资源操作菜单和右键，双击重命名。文件名随资源栏宽度截断（`min-w-0` + `truncate`），`vN` / 「草稿」徽章 `shrink-0` 留在行尾，不要被栏宽裁掉。截断后悬停用 Tooltip 展示全称。空目录用一句空状态加「新建目录」。Radix ScrollArea 默认 `display:table` 会把树行撑出栏宽，资源树视口必须覆盖成 `display:block; min-width:0`。
- 工作台外壳（资源栏 / 编辑区 / 右侧栏 / 控制台）用网格拖动手柄改宽高，尺寸和开合一起记在 `localStorage`。分隔条叠在卡片接缝上：平时是 1px 线，握把只在悬停、聚焦或拖动时出现，不要常驻白条或 6px 轨道。分隔条光标用 Figma 小三角（`public/cursors/{ew,ns}-resize.png` 与 2x，热点 16,16）：两个实心三角、中间留缝、贴合细白边、带抗锯齿。Chromium 不绘制 SVG 光标，所以必须用 PNG；系统 `col-resize` 仅作兜底。不要 `not-allowed`。拖动期间给 `html[data-studio-resizing=x|y]`，用 `!important` 盖住 Monaco 的文本光标。整页 `html` 锁必须写在非 CSS Module 文件（`studio-split-handle.css`）：Next 生产构建要求 module 选择器含本地 class，`:global(html[...])` 会让 `next build` 失败。预览左右栏继续用 Arc `ResizablePanels`（只有横向，且开合是组件内部状态，不能驱动顶栏开关）。HeroUI Pro `Resizable` 和 beUI 都没有可装的免费分隔条，不要为外壳再装一套。默认左 240px、右 360px、控制台 260px；顶栏开关仍负责开合。
- 工作台左右侧栏开合记在 `localStorage`，但首帧必须用默认值（左展开、右收起），挂载后再套用上次记录。不要在 `useState` 初始值里读存储：服务端没有这份记录，客户端若记住了「右侧展开」，水合会改掉顶栏开关的 `aria-label` 和图标。Next 开发态会因此弹出红色报错层，开关点不了。那不是 Arc 的禁用样式。
- 点不了的按钮用默认箭头，不要 `cursor: not-allowed`。这套系统光标会把禁止光标画成一块红色大图标，盖在「停止」这类禁用按钮上。禁用只靠变淡，原因放 tooltip。
- 工作台右侧图标栏：设置 `Settings`（齿轮），模板 `FileStack`（叠放的模板文件），定时 `Clock3`，版本 Lucide `Tags`（两枚标签），全局变量用 Gravity UI 的 `curly-brackets`（16px 实心花括号，内联 SVG，不引入整包）。Arc 和 HeroUI 都没有自己的图标集。不要用数据库、扳手、地球、Git 分支、叠放菱形，或和定时相同的时钟，冒充设置、模板、全局变量和版本。
- 集群升级三页（准备 / 配置合并 / 执行）共用 `.upgrade-steps` 轨道，当前步是抬起的选中块。返回、清空、刷新这类次要动作进 `ButtonGroup` 或 Arc `Button` secondary；继续、生成计划、启动执行用 primary，进行中走 `loading`，阻断原因用 `aria-disabled` + tooltip。状态用 Arc `Badge`。配置文件切换用 Arc `Tabs`。执行页「步骤 | 节点」用 `SegmentedControl`。长说明进 HelpHint，不要在标题下再铺一整段。状态摘要是一行四格，不要四张大卡片。
- 单测一旦引入 Arc 的 CSS Module，`vitest.config.ts` 的 `css.postcss.plugins` 必须是空数组。否则 Vite 会拒绝 `postcss.config.mjs` 里的字符串插件 `@tailwindcss/postcss`。

## 15. 工作台草稿手动保存与离开拦截（Workbench Draft Save）

适用于 `/workbench` → `DataSyncStudio` 的未落盘草稿防丢：

- **只手动保存草稿**：编辑后必须点顶栏「保存」才调用 `persistCurrentFile(false)` / `updateTask`。不要防抖自动保存，更不要自动 `publishTask`。
- **共享状态**：顶栏 Arc 保存段展示「保存 / 保存中 / 已保存」，失败时另用 `DraftSaveStatusBadge` 提供重试。成功以状态指示为准，避免双重成功 toast。
- **离开风险**：任意可编辑 `editorDrafts.dirty`，或保存中 / 失败未恢复时，挂 `beforeunload`，并捕获站内离开 `/workbench` 的链接点击；对话框提供留下 / 保存并离开 / 丢弃并离开，默认焦点不在「丢弃」。
- **只读**：`!editor.canEdit` 时保存按钮不可用；无其它可编辑脏草稿时不拦截离开。

---

## 15. 登录开屏动画（Welcome Splash）

- **触发**：只在登录成功后播放一次（密码登录 `AuthService.login`、OAuth `CallbackHandler`）。两者都是整页跳转，跳转前统一调用 `beginWelcomeSplashTransition()`：写入带时间戳的 `sessionStorage['stx:welcome-splash']`，并把当前页淡入纯黑。
- **防首页先露出**：根布局 `<head>` 里的内联脚本 `welcomeSplashBootScript` 在首帧前读取并消费标记，命中后给 `<html>` 加 `data-stx-splash`，`globals.css` 用 `body::before` 先铺纯黑遮罩；`WelcomeSplash` 水合后接手，退场开始时才撤遮罩。遮罩有 15 秒兜底超时，不会永久黑屏。
- **不播放的情况**：标记超过 60 秒、落地页是 `/login`、自动化浏览器（`navigator.webdriver`）。E2E 要验证动画时，先设置 `localStorage['stx:welcome-splash-force'] = '1'`。
- **可跳过**：右下角「跳过」按钮和 `Esc` 都会进入快速退场；`prefers-reduced-motion` 下只渐显品牌和欢迎语，约 2 秒结束。
- **视觉约束**：整段开屏为单色银白。奇点着色器（移植自 Inspira UI Singularity Background，MIT）在片元里去色，LOGO 图形保留品牌蓝，这是全片唯一的色彩；粒子、尾迹、冲击波一律中性白。不要加彩色背景、彩色渐变字，也不要用书法类艺术字。品牌字和英文欢迎语用 Michroma（`@fontsource/michroma`）。中文欢迎语用 `public/fonts/stx-splash-greeting.woff2`：这是只含欢迎语字形的 Noto Serif SC Light 子集，约 2KB。不要为几个字引入整套中文字体包，那样会多出约 6MB 分片，还会给每个页面的全局 CSS 加约 100KB 的 `@font-face`。
- **欢迎语字体再生成**：修改 `zh.json` 的 `splash.welcomeBack` 后，运行 `cd frontend && bash scripts/fonts/build-splash-greeting-font.sh`（依赖 curl、node、uv），并提交 `public/fonts/stx-splash-greeting.*`。`greeting-font.test.ts` 会校验清单中的文案与语言包一致。
- **碎裂节奏**：撞击前内爆收缩（`uZoom`）→ 撞击瞬间短暂慢放（顿帧）+ 径向过曝 → 镜头震动、放射光芒、双冲击波、碎片带边缘高光，部分碎片朝镜头放大飞出 → 粒子带速度拖影炸开。
- **实现分层**：`splash-timeline.ts` 放时间轴和纯数学，有单测；`singularity-renderer.ts` 是原生 WebGL2，不可用时降级为 CSS 漩涡；`splash-assets.ts` 负责品牌排版和粒子采样；`splash-director.ts` 用单条 rAF 时间轴驱动画面，逐帧不触发 React 渲染。

## 一致性检查项

在提交代码前，必须核对以下检查清单：
- [ ] 页面头部是否使用了统一的 `WorkspaceHeader`？语义图标是否与 Dock 栏对齐？
- [ ] 新增或替换图标前，是否查过 Lucide、Tabler、Arc、HeroUI（Gravity UI），并列了至少 3 个候选给开发者确认？没确认就不要改。
- [ ] 页面是否没有再手写冲突的外层 `container` 或冗余的大 Padding？
- [ ] 顶部是否有状态量化？是否优先使用了 `StatPillsBar` 并支持快捷筛选？
- [ ] 是否消除了卡片顶部的“大篇幅无效说明文字”，保证高密度垂直视口？
- [ ] **[降噪与防堆砌]** 页面与弹窗是否遵循渐进式披露（Progressive Disclosure），严禁在首屏将概览、重型表格与复杂表单全量无脑平铺？
- [ ] **[下钻与速览]** 次级重型内容是否通过 Tab / 抽屉 / 折叠面板进行物理分流？主视图是否仅保留轻量速览与穿透动作？
- [ ] **[排版防碎裂]** 关键标识、键值单元、代码、状态组是否声明了防被迫换行保护（`whitespace-nowrap` / `truncate` / `overflow-x-auto`），杜绝机械等宽 Grid 导致的字符内部被迫断行？
- [ ] 加载状态是否采用 `TableSkeletonRows` + `TableLoadingBar` 的零塌陷架构，而非大 Spinner？
- [ ] 筛选栏是否统一左对齐？搜索框等输入项是否已支持 **Enter** 快捷提交？
- [ ] 表格底部分页是否通过 `border-t bg-muted/10` 封装在 Table Card 内？
- [ ] 分页栏是否统一使用 `@/components/ui/pagination`？是否杜绝了“上一步/下一步”？
- [ ] 弹窗是否具有明显的边框与层次感？重内容弹窗是否已显式放宽至 `sm:max-w-4xl` 以上？
- [ ] 新建物理/异步资源是否有引导式交互（一键命令、实时状态侦听）？
- [ ] 密码/Token 字段是否满足“保存后脱敏、编辑时不回显”的安全规范？
- [ ] 语言包是否已在 `zh.json` 与 `en.json` 补充对应的独立单语言文案？
- [ ] 动态后端值是否在调用翻译函数前使用 `t.has()` 检查，并准备原值或兜底文案？
- [ ] 国际化覆盖测试是否通过，确认深层键、静态代码引用和 ICU 参数均无缺失？
- [ ] 自增/修改的代码注释是否严格遵循“中文在前，英文在后”的双语规范？
- [ ] 工作台若改用 Arc 控件，是否仍保留 `components/ui`、`data-theme` 同步？保存段是否跟随草稿：干净显示「已保存」，一改回到「保存」，失败且仍脏时不显示「已保存」？
- [ ] 工作台任务 / 预览表是否用 `StudioDataTable`（粘头、排序、底栏、空状态），颜色是否仍走 Arc token 而不是另一套主题？预览左右栏是否用 Arc `ResizablePanels`，而不是写死宽度？工作台外壳是否可拖动改宽高，拖动光标是否是 Figma 小三角 PNG（中间留缝、带抗锯齿），而不是过大的系统光标，树行是否不再堆悬停图标，文件名是否随栏宽截断且 `vN` / 草稿徽章不被裁掉，截断文件名 hover 是否能看到全称？
- [ ] 工作台草稿是否只靠手动保存（非发布、无自动保存），并在未落盘时提供离开拦截与共享保存状态？
- [ ] 登录后的整页跳转是否先走 `beginWelcomeSplashTransition()`？新增登录入口时不要绕过开屏契约。
- [ ] 修改开屏中文欢迎语后，是否已重跑 `build-splash-greeting-font.sh` 并提交子集字体？
