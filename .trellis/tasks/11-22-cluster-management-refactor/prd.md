# 集群管理页重构与前端规范全面刷新 PRD

## 1. 背景与目标

近期告警中心（PR #166）、诊断中心（PR #170）、列表与常驻详情栏联动动效（PR #178）、审计/用户/令牌/主机列表统一化（PR #163）以及单层画布压平（PR #179）已沉淀出 STX 控制台的新一代设计语言，并与 `kubai-design-tips` 设计质感规则高度吻合：

- **统一外壳与密度**：`.arc-compact` 半胶囊密度（`--radius-control: 14px`、`--radius-panel: 16px`、`34px/30px` 行高）+ `WorkspaceHeader` + `ModuleNavTabs` + Arc `TabsList`。
- **单行工具栏 + 唯一主卡片**：Arc `SegmentedControl` 状态/短枚举筛选 + Arc `SearchField` + `ViewToggle`（`useViewMorph` 共享形变）+ `TableCard`（300ms 延迟 / 500ms 驻留骨架、静默刷新 `TableLoadingBar`、卡片内错误重试、定制 `EmptyState`、底部分页栏）。
- **列表 + 常驻详情分栏与克制联动**：`DetailHost`（≥1080px 常驻右侧栏 + #178 零间距压暗/凹圆角/名称飞入 + J/K 导航；<1080px 贴边 Arc Drawer）+ `DirtyCloseDialog` 未保存脏态保护。
- **单层画布 + 0.5px 细线压平**：去除页面、详情栏和弹窗内「卡片套卡片」的多层边框与重阴影，以留白 + `0.5px` 细线自然分层。

相比之下，当前集群管理域（`/clusters`、`/clusters/[id]`）及 `.trellis/spec/frontend/` 存在两类核心问题：
1. **前端规范文档滞后且存在新旧冲突**：`.trellis/spec/frontend/` 仍保留大量迁移前的旧规范（如推荐 `StatPillsBar`、`opacity-60` 刷新遮灰、`shadow-2xl` 重弹窗边框、GSAP 整页入场、以 `components/ui/` 为默认基座、独立搜索框写法等），极易误导后续开发写出倒退代码。
2. **集群管理页停留在旧版或半迁移混血形态**：
   - `/clusters`（`ClusterMain` + `ClusterCard`）仅有固定 `400px` 列宽卡片网格，未接入 `.arc-compact`、`TableCard`、`ViewToggle`、`DetailHost`，部署模式 3 选项藏在 `Select` 中，加载失败直接清空列表，空态仅为虚线纯文本，分页游离于卡片外。
   - `/clusters/[id]`（`ClusterDetail.tsx`，4031 行）头部塞入 4 枚 Badge + 5 个操作按钮 + 语义混杂的 `StatPillsBar`，下方平铺 **10 个同级 Tab**；节点批量条挤在卡片头；`MonitorConfigPanel` 的 `auto_restart` 用了 `Switch` 却依赖底部「保存」按钮；各编辑弹窗仍为旧版 shadcn `Dialog` 与多层嵌套卡片，且缺失未保存脏态拦截。
   - `/clusters/[id]/upgrade/{prepare,config,execute}`（3167 行）处于半迁移状态：三页 `.upgrade-page` 均未挂 `.arc-compact`；大量混用 shadcn `Card` / `Select` / `Input` 与 Arc 控件，存在最多 **4 层卡片套盒子**、虚线纯文本空态、`max-w-[1800px]` 自带居中容器、绿色成功条误用 `AlertTriangle` 图标；执行页日志区把 4 个短选项日志级别塞进 `Select`、每条日志包独立圆角卡片、长日志套用 Radix `ScrollArea` 并与 `Pagination` +「加载更早日志」两套分页打架。

**本任务目标**：先全面刷新并清理过期前端规范（确立单一事实源），再按新规范重构集群列表页、集群详情页、集群升级三阶段页及附属弹窗/配置面板。

---

## 2. 已确认决策（2026-11-22）

1. **规范先行、清除旧律**：全面更新 `.trellis/spec/frontend/` 并移除已废止的旧规范条款，后续所有实现严格以新规范为准。
2. **单层扁平画布硬约束（贯穿所有详情、子面板与弹窗，对齐 PR #179）**：所有详情页、右侧速览栏、Tab 主面板与管理类弹窗一律**压平成单层卡片，去掉里面多余的嵌套边框与阴影，用留白和一条 `0.5px` 细线（`border-t-[0.5px]` / `border-b-[0.5px]` + `border-border/80`）自然分开各组信息**；内嵌表格直接用 `[&_tr]:border-b-[0.5px] [&_tr]:border-border/70` 承接，不再多包一层圆角边框或虚线盒；单行路径/坐标/命令条用轻底色（如 `bg-muted/50`）承托而非再画描边。
3. **列表页形态**：`/clusters` 采用 **表/卡双视图（默认表格 `list`）+ `DetailHost` 右侧常驻速览栏**，右侧栏提供快捷运维与直达 `/clusters/[id]` 完整控制台入口。
4. **详情页信息架构**：`/clusters/[id]` 顶层收敛为 **严格 6 个一级 Tab**（`概览 / 节点 / 配置 / 插件 / 运行时 / 运维`），内部用 `SegmentedControl` 承载子视角，完整兼容历史 URL 深链。
5. **`auto_restart` 语义修正**：`MonitorConfigPanel` 的 `auto_restart` 开关改为 **拨动立即保存**（后端 `PUT /api/v1/clusters/:id/monitor-config` 已验证采用指针字段支持部分更新，不污染未保存的数值表单），带轻量内联保存态与失败回滚。
6. **覆盖升级三阶段页**：`/clusters/[id]/upgrade/{prepare,config,execute}` 纳入本次重构范围，同步落实 `.arc-compact` 与单层画布 `0.5px` 细线压平设计。

---

## 3. 范围与需求

### R0：全面更新与移除过期前端规范（先于业务代码落地）

涉及文档：`.trellis/spec/frontend/index.md`、`.trellis/spec/frontend/ui-conventions.md`、`.trellis/spec/frontend/directory-structure.md`。

#### R0.1 必须移除/废止的旧规范条款
- **移除「优先采用 `StatPillsBar` 做状态筛选 / 多用途切换」**：改为统一使用 Arc `SegmentedControl`（单行工具栏）承载状态筛选；`StatPillsBar` 降级为历史兼容组件，禁止新页面或重构页面用于兼任 Tab 切换与搜索词写入。
- **移除「静默刷新配合数据行 `opacity-60 pointer-events-none`」**：统一遵循 `TableCard` 与 Kubai Tip 017「一次等待只留一种指示」原则——有旧数据时刷新仅亮顶部 `TableLoadingBar`，旧数据保持清晰可读、可交互，不压暗或禁点整表。
- **移除「弹窗必须配 `border border-border/80 shadow-2xl` 边框 + 内部子 Card 分块」**：改为 Arc `Dialog`（挂 `.arc-compact`）+ 单层画布 + `0.5px` 细线分隔（对齐 PR #179），严禁在弹窗、详情栏或主面板内再套多层 `<Card>` / 虚线框。
- **移除「列表筛选栏默认 `flex-1 min-w-[200px] max-w-sm` + 独立搜索按钮」与旧 `ClusterMain` 示例**：改为单行工具栏（`layout.toolbar`）+ Arc `SearchField`（`/` 聚焦、`Esc` 清空、Enter/即时查询），不再额外配无意义的「搜索」按钮。
- **移除「整页/列表默认套 GSAP `animateTableRows` / `animateGridCards` 与 500–600ms Motion 级联入场」**：避免慢拖沓感与重复入场；统一使用 150–300ms 语义缓动曲线（入场 `ease-out` / 快速落位 `cubic-bezier(0.22, 1, 0.36, 1)` / 退场 `ease-in` / 位移 `ease-in-out`），动效优先服务于状态连续性（`useViewMorph`、#178 联动与名称飞入），不堆砌无意义进场动画。
- **移除「优先组合 `components/ui/` 中的小组件」与双横杠错字 `-- **i8n 国际化规范**`**：明确新开发与重构页面优先使用 `components/arc/` 与 `components/common/layout/` 公共原语；`components/ui/` 仅保留给未迁移页面及 Arc 暂无替代的 `AlertDialog` / `Textarea` / `Pagination`。

#### R0.2 必须确立并写入的新规范契约（融合近期 PR 与 `kubai-design-tips`）
- **组件与画布层级（Design System Pass + PR #179）**：
  - 所有控制台工作区根容器统一挂 `.arc-compact`，Portal 浮层（`Dialog` / `Drawer`）内容节点必挂 `.arc-compact`。
  - 统一使用语义色与 `SoftChip` / `NeutralChip` 表达状态，禁止在业务页面散落硬编码色值（如 `bg-green-500` / `bg-yellow-500`）。
  - **详情与弹窗单层扁平画布硬规则**：外层只保留一层卡片/栏/弹窗容器，内部**严禁**再套 `<Card>`、`rounded-lg border bg-card shadow-xs`、`rounded-md border bg-muted/30` 或虚线外框；各信息组通过统一留白（如 `pb-5` / `py-5`）+ 一条 **`0.5px` 细线**（`border-t-[0.5px] border-border/80` / `border-b-[0.5px] border-border/80`）自然分隔；内嵌表格直接透出在画布上，行分隔用 `[&_tr]:border-b-[0.5px] [&_tr]:border-border/70`；代码/坐标/路径条用轻底色无描边条承托。
- **圆角与排版（Tip 004 & Tip 007）**：
  - 面板与控件统一消费 `--radius-panel` / `--radius-control` token，嵌套容器遵循同心圆角（`inner = outer - padding`，不低于 4px）。
  - 计数、版本号、端口、PID、内存、时间、百分比等数字强制 `tabular-nums`；技术标识符/路径禁止 `break-all` 硬折，一律单行 `truncate` + Tooltip + 复制。
- **选择控件语义（Tip 020）**：
  - `Switch` 仅用于**拨动立即生效**的设置项（禁止放进必须点「保存」才提交的表单）；随表单提交的布尔项用 `Checkbox`。
  - 2–5 个短选项用 `SegmentedControl`（禁止藏进 `Select`）；选项可动态增长或 ≥6 项用 `Select` / 可搜索 `Combobox`；动作集合用 `DropdownMenu`（禁止用 `Select` 放动词）。
- **浮层分工（Tip 009）**：
  - 纯文本补充用 `Tooltip`；高危/不可逆/跨资源影响确认用居中 `AlertDialog`（写明影响范围与具名动作按钮，禁用泛化「确定」）；短表单用 Arc `Dialog`；长详情/需对照列表浏览用 `DetailHost`（≥1080px 常驻栏 / <1080px 贴边 `Drawer`）；重型多域工作台用独立整页。
  - 所有含编辑态的 `Dialog` / `Drawer` / 详情栏 / Tab 切换统一走 `DirtyCloseDialog`（或 `UnsavedChangesDialog`）未保存拦截。
- **加载、错误与空态（Tip 017 & Empty States Note）**：
  - 列表页一律由 `TableCard` 承载：首屏加载超过 **300ms** 才显骨架、显现后至少保留 **500ms**；刷新保留旧数据与可交互性，只亮顶部 `TableLoadingBar`；请求失败不清空已有旧数据，在卡片内提供报错与重试按钮；单页独立卡片/表单加载使用同构骨架屏，禁止居中单 Spinner 替换整卡。
  - 禁止仅按当前页数据捏造与总数矛盾的状态计数（遵循 PR #170）。
  - 空态一律使用 Arc `EmptyState`，必须区分「首次无数据（配功能微缩示意与创建主动作）」与「筛选无结果（配一键清空筛选）」，禁止裸虚线框纯文本。
- **列表与详情连续性（Tip 015 + PR #178 + PR #163）**：
  - 列表状态、筛选与页码支持 URL 双向同步（筛选变更重置 `page=1`）+ 请求竞态保护（忽略过期响应）。
  - 列表 + 常驻详情分栏统一接入 `DetailHost` + `list-detail.module.css`（#178 联动压暗、凹圆角、点击/回车名称飞入、J/K 切换、窄屏与 `prefers-reduced-motion` 自动降级）。
  - 表/卡双视图切换统一使用 `ViewToggle` + `useViewMorph`（200ms FLIP 共享元素形变）。

---

### R1：集群列表页重构（`/clusters` — `ClusterMain`）

1. **页面外壳与页头**
   - 根容器挂 `.arc-compact` + `list-detail.module.css` 的 `.page` / `.pageViewport`（≥1080px 锁视口，列表与常驻栏等高独立滚动）。
   - `WorkspaceHeader` 保留 `icon={<Layers />}` 与 `ModuleNavTabs`（集群 | 主机）。
   - 页头主操作收敛为 2 个核心动作：**部署集群**（主 CTA，打开 `ClusterDeployWizard`）与 **注册集群**（次操作，打开 `CreateClusterDialog`）；刷新按钮下沉为工具栏右侧 `iconOnly` 图标按钮（点击时图标平滑旋转完整圈）。

2. **单行工具栏**
   - **状态分段（Arc `SegmentedControl`）**：`全部` / `运行中` / `部署中` / `已停止` / `异常`（在后端未提供全局状态分桶计数前，**不展示**按当前页拼凑的假数字）。
   - **搜索框（Arc `SearchField`）**：支持 `/` 快捷键聚焦、`Esc` 清空、Enter 或防抖触发查询；移除旧版独立「搜索」按钮。
   - **部署模式分段（Arc `SegmentedControl`）**：`全部` / `混合` / `分离`（替换原 `Select`，一眼可见、一点即切）。
   - **右侧工具组**：`ViewToggle`（`表格 List` 默认 / `卡片 Grid`，接入 `useViewMorph('clusters')` 共享形变，记住在 URL `?view=grid|list`）+ 刷新图标按钮 + 有活跃筛选时显示紧凑「重置筛选」入口。
   - **URL 同步与请求安全**：`status`、`mode`、`q`、`page`、`view`、`cluster`（当前打开速览栏的集群 ID）同步 URL；任意筛选变更重置 `page=1`；请求带递增序号竞态保护。

3. **唯一主卡片（`TableCard`）与双视图**
   - **表格视图（默认 `list`）**：
     - 严格满足 #178 联动 DOM 契约（行是 `.tableScroll` 直接子元素、选中行挂 `.rowActive`、名称节点挂 `data-detail-name`、列宽由 `data-mcols="clusters"` + `@media` 驱动，禁止内联 `--cols`）。
     - 展示列：**集群名称**（名称 + `cluster_code` 副标 + 唯一集群「默认」标记）、**状态**（统一 `ClusterStatusChip`；运行中但 `online_nodes === 0` 或 `health_status === 'unhealthy'` 明确显示异常态）、**部署模式**（`NeutralChip`）、**版本**（`tabular-nums font-mono`）、**节点在线**（`online / total`，`tabular-nums`，存在离线节点时视觉标警）、**操作**（紧凑「控制台」或启/停快捷图标 + `⋯` Arc `DropdownMenu` 收纳进入控制台 / 编辑 / 启停重启 / 升级 / 删除）。
   - **卡片视图（`grid`）**：
     - 修复死宽列为响应式网格 `grid-cols-[repeat(auto-fill,minmax(320px,1fr))]`，消除宽屏右侧大白边与窄屏溢出。
     - 单层扁平画布 + `0.5px` 细线底栏（顶行名称与状态芯片、中段紧凑 KV 横条、底栏操作与控制台直达），点击卡片同样联动打开速览栏，支持选中态高亮。
   - **加载、错误、空态与分页**：
     - 全权交由 `TableCard` 承载（300ms 延迟 / 500ms 驻留骨架；刷新保留旧数据可点只亮 `TableLoadingBar`；错误留卡片内附重试；分页固定卡片底部，移除外部孤立分页条）。
     - 区分两种 Arc `EmptyState`：首次无集群（展示微缩集群示意 +「部署集群 / 注册集群」动作按钮）与筛选无结果（展示清空筛选动作按钮）。

4. **右侧常驻速览栏（`DetailHost`）**
   - ≥1080px 常驻右侧栏（接入 #178 联动压暗、零间距、10px 凹圆角、点击/回车名称飞入、J/K 切换与 `↑↓ i/n`）；<1080px 自动退化为贴边 Arc Drawer。
   - **头部**：集群名称（飞入落点 `nameSlot='title'`）+ `ClusterStatusChip` + 顶部操作条（主动作「进入控制台 →」跳 `/clusters/[id]`，次动作启/停/重启、编辑）。
   - **内容分区（单层画布 + `0.5px` 细线分隔，按需请求单集群详情与状态，带竞态保护与轻量骨架）**：
     - **核心规格**：集群编码（可复制）、SeaTunnel 版本、部署模式、安装目录（单行 `truncate` + Tooltip + 复制）、更新时间（走 `lib/timezone`）。
     - **节点健康速览**：`online / total` 比例与最多前 5 个节点摘要（主机名/IP、角色、状态点），底部「查看全部节点 →」直达 `/clusters/[id]?tab=nodes`。
     - **运维直达**：存在活跃告警时展示紧凑单行警示条直达 `/monitoring?tab=alerts&cluster_id=<id>`；底部提供直达配置、插件、诊断、升级的快捷入口。
   - **共享生命周期去重**：抽离 `useClusterLifecycle`（或统一操作组件），消除原 `ClusterCard` 与 `ClusterActions` 中完全重复的启/停/重启接口调用、自动重启提示识别与二次确认 `AlertDialog`。

---

### R2：集群详情页重构（`/clusters/[id]` — `ClusterDetail`）

1. **外壳、页头与 URL 契约**
   - 根容器挂 `.arc-compact`；移除原 500–600ms 整页 `motion.div` 级联入场。
   - `WorkspaceHeader` 保留面包屑（集群管理 / 集群名）+ `icon={<Layers />}` + 标题；头部徽章精简为最多 2 枚（`ClusterStatusChip` + 版本 `vX.Y.Z`），集群编码与描述归位到概览首段。
   - 移除原详情页顶部混杂统计、Tab 切换与硬塞搜索词的 `StatPillsBar`。
   - 头部操作区收敛为：整集群生命周期操作（主动作 + 紧凑分组）+ 刷新图标按钮 + `SeaTunnelObserveMenu`（有可用引擎观测入口时）+ `⋯` 更多菜单（编辑 / 升级准备 / 打开诊断中心 / 删除）。
   - 保留 PR #159 的可关闭单行活跃告警条，样式统一迁入 Arc `Alert`。

2. **严格 6 个一级 Tab（Arc `TabsList` + URL `?tab=` & `?view=` 双向同步）**

   | 一级 Tab (`?tab=`) | 稳定 `data-testid` | 承载内容与内部子视角 (`?view=`) |
   | --- | --- | --- |
   | **概览 (`overview`)** | `cluster-detail-tab-overview` | 集群画像、安装目录/编码/版本/端口/JVM 规格、节点健康与近期事件速览；压平为单层画布 + `0.5px` 细线分区，去掉多层嵌套小卡片 |
   | **节点 (`nodes`)** | `cluster-detail-tab-nodes` | 迁入 `TableCard` + Arc 状态/角色筛选与 `SearchField`；多选批量启停改为避让 Dock 的底部浮动 `BatchBar`（移除卡片头内联批量条）；保留扩缩容横幅与节点增删改/日志能力 |
   | **配置 (`configs`)** | `cluster-detail-tab-configs` | 保留 `ClusterConfigs` 业务能力（与 `config-real.spec.ts` 锚点兼容），压平外层嵌套卡片，编辑未保存接入 `DirtyCloseDialog` |
   | **插件 (`plugins`)** | `cluster-detail-tab-plugins` | 保留 `ClusterPlugins` 业务能力（与 `plugin-real.spec.ts` 锚点兼容），沿用 PR #179 单层画布压平风格 |
   | **运行时 (`runtime`)** | `cluster-detail-tab-runtime` | 内部 Arc `SegmentedControl` 切换：`Checkpoint 存储 (checkpoint)` \| `IMAP 存储 (imap)` \| `Java Proxy (proxy)` \| 按需 `Web UI (webui)`（仅 `webUINode` 存在时出现） |
   | **运维 (`ops`)** | `cluster-detail-tab-ops` | 内部 Arc `SegmentedControl` 切换：`守护与自愈 (guard)`（`MonitorConfigPanel`）\| `诊断异常 (diagnostics)` \| `升级记录 (upgrades)`（迁入 `TableCard`） |

   - **旧 URL 深链无损兼容映射**（读取旧参数并归一化到新 URL，不破坏外部链接与历史书签）：
     - `?tab=storage` → `tab=runtime&view=checkpoint`
     - `?tab=proxy` → `tab=runtime&view=proxy`
     - `?tab=webui` → `tab=runtime&view=webui`
     - `?tab=monitoring` → `tab=ops&view=guard`
     - `?tab=diagnostics` → `tab=ops&view=diagnostics`
     - `?tab=upgrades` → `tab=ops&view=upgrades`

3. **`ClusterDetail.tsx`（4031 行）模块化拆解与全详情面压平清单**
   - 将巨石组件按职责拆分为清晰的子目录模块（如 `components/common/cluster/detail/` 下的概览、节点、运行时、运维子模块及独立 hooks），单文件保持高内聚、可单测。
   - **明确须按「单层卡片 + 去嵌套边框阴影 + 留白与 `0.5px` 细线分隔」逐一压平的详情界面**：
     1. 列表页右侧 `DetailHost` 集群速览栏（规格 / 节点速览 / 运维直达三组仅以 `0.5px` 细线分隔）；
     2. 详情页「概览」Tab（原多张 `<Card>` 与内部 8 个 `rounded-lg border bg-muted/10` 字段小盒、端口/事件通道卡片合并压平为单层主卡片 + 留白 + `0.5px` 细线分组）；
     3. 详情页「运行时」Tab（Checkpoint / IMAP 配置编辑器、存储校验与文件浏览、STX Java Proxy 规格与状态说明均压平在单层主卡片内，去内部嵌套 `<Card>` 与虚线/灰底描边框）；
     4. 详情页「运维」Tab（`MonitorConfigPanel` 开关行与参数组、诊断异常摘要列表、升级记录表均压平在单层主卡片内，列表行以 `0.5px` 细线分隔，去单个错误组独立圆角框与虚线空态框）；
     5. 详情页「配置」与「插件」Tab（`ClusterConfigs` 模板列表/编辑器外框与 `ClusterPlugins` 均保持单层画布风格，去掉卡片套卡片）；
     6. 所有详情/编辑/配置/预览类弹窗（`EditClusterDialog`、`AddNodeDialog`、`EditNodeDialog`、`RemoveNodeDialog`、`ClusterNodeLogDialog`、Proxy 内存配置与日志弹窗、Checkpoint/IMAP 检视弹窗）一律以弹窗本体作唯一画布，内部用留白和 `0.5px` 细线分组。

---

### R3：`MonitorConfigPanel` 与集群弹窗体系重构

1. **`MonitorConfigPanel` 交互语义修正（`auto_restart` 拨动即保存）**
   - 外层与内部压平为单层画布 + `0.5px` 细线分区；首次加载改用同构骨架屏（移除居中单 `Loader2`）。
   - **`auto_restart` 开关（Arc `Switch`，保留 `data-testid="monitor-auto-restart-switch"`）**：
     - 拨动后**乐观更新 UI** 并立即调用 `PUT /api/v1/clusters/:id/monitor-config` 仅提交 `{ auto_restart: next }`（不发送未保存的数值表单字段，不触发「未保存」脏态）。
     - 在开关旁固定占位区展示轻量状态（`保存中…` → `已保存` 短暂淡出），不遮灰整页、不跳动布局。
     - 请求失败时立即回滚开关状态，在同位置展示简短错误原因与「重试」入口（同时避免并发连点竞态）。
   - **数值策略参数（5 项）**：保持显式「保存 / 重置」操作（仅追踪这 5 项的脏态）；未保存时切换 `ops` 子视图或顶层 Tab 触发 `DirtyCloseDialog` 拦截。

2. **弹窗体系全面迁移 Arc 与单层画布压平**
   - `EditClusterDialog`、`AddNodeDialog`、`EditNodeDialog`、`RemoveNodeDialog`、`ClusterNodeLogDialog` 及详情内附带的 Proxy 配置/日志/存储预览弹窗：
     - 统一迁至 Arc `Dialog`（内容节点挂 `.arc-compact`），日志/代码等长文本弹窗使用 `ExpandableTextPanel` 或规范大宽度（`w-[min(92vw,960px)]!`）+ 原生 `overflow-auto`（禁止长日志再包 `ScrollArea`）。
     - 内部去掉多余 `<Card>` 嵌套，改用留白 + `0.5px` 细线。
     - 编辑类弹窗关闭时如有未保存修改，统一接入 `DirtyCloseDialog`。
   - 启/停/重启/删除集群及批量节点启停等高危确认保留 `AlertDialog`，文案明确受影响集群/节点名称与不可逆风险，确认按钮使用具名动作（如「停止集群」「删除集群」，禁用泛化「确定」）。
   - `CreateClusterDialog` 与 `ClusterDeployWizard` 接入 `.arc-compact` 密度与关闭脏态保护（保持既有向导步骤与版本能力矩阵契约不变）。

---

### R4：集群升级三阶段页重构（`/clusters/[id]/upgrade/{prepare,config,execute}`）

统一三个升级页面的外壳密度、画布层级、控件选型与日志浏览体验，同时**完整保留全部既有 `data-testid`、会话草稿逻辑、预检阻断/目标 JDK 校验规则与计划确认主链路**。

1. **三页公共外壳与排版规范**
   - `.upgrade-page` 根容器统一挂 `.arc-compact`，使 `UpgradeSteps`、Arc `Button` / `Badge` / `ButtonGroup` / `Tabs` / `SegmentedControl` / `Select` / `Input` 全部落在同一半胶囊紧凑尺度。
   - 所有状态提示条（草稿过期提示、阻断问题提示、配置全部解决/待解决提示、升级完成/可重启提示）统一采用 Arc `Alert` 与匹配语义的图标（修正配置合并页绿色成功态误用 `AlertTriangle` 的问题）。
   - 三页内所有计数、版本号、步序、进度百分比、行号、时间戳统一使用 `tabular-nums`；所有安装路径、校验和、命令摘要禁用 `break-all` 硬折，改用单行 `truncate` + Tooltip / 独立横向滚动。
   - 缺失草稿 / 缺失计划 / 无日志 / 未运行预检 / 无阻塞问题等空态统一采用 Arc `EmptyState`（并保留 `WorkspaceHeader` 与 `UpgradeSteps`，不再渲染孤立无头 `<Card>`）。
   - 刷新与异步加载（预检、建计划、轮询任务状态）增加请求序号竞态保护与错误态卡片内重试，保留已有快照不清空页面。

2. **升级准备页（`ClusterUpgradePrepare.tsx` — `/upgrade/prepare`）**
   - 左侧输入表单迁移至 Arc `Select` / `Input`；保留「高级选项」默认折叠渐进式披露，压平折叠区外框为 `0.5px` 细线分隔。
   - 右侧将当前 3 张嵌套 4 层边框的 `<Card>`（预检摘要、阻断问题、节点目标）压平为**单层主画布 + `0.5px` 细线分区**：
     - 预检摘要（`data-testid="upgrade-prepare-summary"`）：去嵌套 `SummaryCard` 边框与虚线提示盒，用紧凑双列 KV + 单行微提示呈现安装包/连接器清单与状态徽章。
     - 阻断问题（`data-testid="upgrade-prepare-issues"`）：按分类以 `0.5px` 细线分组列出问题项与直达处置按钮（去嵌套双层圆角盒）。
     - 节点目标与目标 JDK（`data-testid="upgrade-prepare-node-targets"`）：采用统一单层表格表面（保留 `upgrade-prepare-node-jdk-<id>` 等全部锚点与 `JavaHomePicker` 交互）。

3. **配置合并页（`ClusterUpgradeConfig.tsx` — `/upgrade/config`）**
   - 移除仅包了一层文件 `TabsList` 的独立外层 `<Card>`，将文件 Arc `Tabs`（保留 `role="tab"` 兼容 E2E）与下方三栏差异合并编辑器整合为**一张统一主画布**，并移除违背 `ui-conventions.md §1` 的 `mx-auto max-w-[1800px]` 外层限宽容器。
   - 三栏差异表头（本地旧配置 / 合并结果 / 目标新配置）压平为单层 sticky 工具头 + `0.5px` 细线列分隔；代码行移除 `break-all` 硬折，保留横向滚动容器与行号 `tabular-nums` 对齐；冲突行高亮与行级采纳动效统一使用语义色与 150–300ms 过渡。
   - 保留 `upgrade-config-create-plan` 的 `DisabledReasonTooltip` 禁用原因与 `blockedIssues` 回显能力。

4. **执行与日志页（`ClusterUpgradeExecute.tsx` — `/upgrade/execute`）**
   - 顶部 4 个散装 `StatusCard` 收敛为一条单层状态摘要条（`0.5px` 竖线分隔，对齐监控/诊断指标条）；执行中状态条附带轻量静默刷新指示（`TableLoadingBar`），不遮灰页面。
   - **启动前计划确认态（`upgrade-plan-review`）**：压平嵌套 `PlanSummaryCard` 与步骤卡片框，计划步骤与目标节点清单以单层画布 + `0.5px` 细线展示；`upgrade-execute-start` 保持计划确认页的直接主执行动作（与 `upgrade-real.spec.ts` 一致）。
   - **执行中 / 终态**：
     - 左侧进度条 + `步骤树 / 节点执行` Arc `SegmentedControl`（保留 `upgrade-view-steps` / `upgrade-view-nodes`），步骤列表与节点表压平去嵌套边框。
     - 右侧日志面板（关键优化）：
       - 日志级别（`全部 / INFO / WARN / ERROR`，4 个短选项）由 `Select` 改为 Arc `SegmentedControl`（Tip 020）；步骤与节点筛选迁至 Arc `Select`；
       - 移除长日志外层的 Radix `ScrollArea`（违背长 mono 日志规范），改用原生 `overflow-auto` + 紧凑单行日志条目 + `0.5px` 细线分隔（移除每条日志的 `rounded-lg border bg-muted/20 p-3` 盒子，显著提升同屏日志密度）；命令摘要采用可横滚 mono 块（禁用 `break-all`）；
       - 统一日志翻页交互：底部卡片底栏保留唯一分页栏（`border-t bg-muted/10`），仅当存在更早页时保留顶部快捷按钮（`data-testid="upgrade-load-earlier-logs"`）作为跳上一页快捷方式且语义说明清晰，消除两套并列分页感。

---

## 4. 兼容性、CLI 与质量要求

- **后端与 CLI 影响**：本任务为纯前端规范与 UI 重构，不修改后端 API 路由、请求/响应结构与执行协议；核查 `stx cluster` / `stx monitor` / `stx upgrade` CLI 命令不受影响。
- **国际化**：所有新增/调整文案完整覆盖 `zh.json` 与 `en.json`，单语展示，后端动态状态码先 `t.has()` 校验；`translation-coverage.test.ts` 必须通过。
- **E2E 锚点保护**：保留既有 `cluster-detail-tab-overview` / `nodes` / `configs` / `plugins`、`monitor-auto-restart-switch`、`cluster-webui-master-address`、以及升级三页全部 `upgrade-prepare-*` / `upgrade-config-*` / `upgrade-execute-*` / `upgrade-view-*` / `upgrade-plan-*` / `upgrade-load-earlier-logs` 锚点与文件 `role="tab"` 语义，确保 `config-real.spec.ts`、`plugin-real.spec.ts`、`upgrade-prepare-template.spec.ts`、`upgrade-prepare-negative.spec.ts`、`upgrade-real.spec.ts`、`upgrade-real-profiles-missing.spec.ts`、`diagnostics-errors-template.spec.ts` 零破坏。
- **新增自动化覆盖**：补充集群列表（双视图切换、筛选 URL 同步、`DetailHost` 打开/关闭/J-K 导航、错误与空态）、`MonitorConfigPanel`（`auto_restart` 立即保存与失败回滚）、详情页 6 Tab 路由与旧 URL 映射、以及升级三页关键渲染与日志筛选的 Vitest 单测及 Playwright mock E2E。

---

## 5. 验收标准

- [ ] **R0 前端规范**：`.trellis/spec/frontend/index.md`、`ui-conventions.md`、`directory-structure.md` 已完成全面更新，过期的旧规范条款已移除，新规范与告警中心/诊断中心/插件压平/Kubai 规则完全一致且无自相矛盾。
- [ ] **R1 列表页**：`/clusters` 默认表格视图，支持 `ViewToggle` 切换卡片并具备 `useViewMorph` 形变；单行工具栏含状态与部署模式 `SegmentedControl` + `SearchField`（无独立搜索按钮）；`TableCard` 统一承载骨架（300/500ms）、静默刷新条（旧数据不压暗）、错误重试、区分场景的 `EmptyState` 与底部分页；≥1080px 点行打开 `DetailHost` 右侧常驻速览栏并具备 #178 联动压暗/凹圆角/名称飞入与 J/K 切换，栏内可直达 `/clusters/[id]`。
- [ ] **R2 详情页**：`/clusters/[id]` 顶层严格为 6 个 Arc Tab（概览/节点/配置/插件/运行时/运维），URL `?tab=` / `?view=` 双向同步且所有旧 tab 深链自动映射；顶部移除 `StatPillsBar`；概览与子面板完成单层画布 `0.5px` 细线压平；节点 Tab 迁入 `TableCard` 并使用底部浮动 `BatchBar`；`ClusterDetail.tsx` 完成模块化拆解。
- [ ] **R3 配置与弹窗**：`MonitorConfigPanel` 的 `auto_restart` 拨动即保存（仅提交 `auto_restart`，失败回滚可重试），数值表单保留保存/重置与脏态拦截；集群管理类弹窗迁入 Arc `Dialog` + `.arc-compact` + 单层画布压平 + `DirtyCloseDialog`；高危确认 `AlertDialog` 具名动作且消除 `ClusterCard`/`ClusterActions` 重复实现。
- [ ] **R4 升级三阶段页**：`/clusters/[id]/upgrade/{prepare,config,execute}` 统一接入 `.arc-compact` 与单层画布 `0.5px` 细线压平；修正配置页 `AlertTriangle` 语义错配、独立 Tabs 卡片与 `max-w-[1800px]` 限宽；执行页日志级别改 `SegmentedControl`、移除长日志 Radix `ScrollArea` 与每条日志独立卡片框、去 `break-all`；现有升级模板与真实 E2E 锚点 100% 兼容。
- [ ] **单层画布与 `0.5px` 细线一致性**：列表速览栏、详情页 6 个 Tab、升级三阶段页及全部集群管理类弹窗均已压平为单层卡片/画布，无多余嵌套 `<Card>`、无冗余边框与阴影、无虚线占位盒，各组信息统一以留白 + `0.5px` 细线自然分隔。
- [ ] **响应式与主题**：亮/暗色主题、<1080px 抽屉降级、`prefers-reduced-motion` 降级均表现正常；无数字抖动（`tabular-nums`）或技术标识被迫折断（无 `break-all`）。
- [ ] **测试与门禁**：`cd frontend && pnpm exec tsc --noEmit`、`cd frontend && pnpm test`、`translation-coverage.test.ts`、`dialog-width-classes.test.ts` 与升级/集群 mock E2E 全部通过。

---

## 6. 不在范围

- 不修改后端集群/节点/监控/升级 API 契约（如列表全局状态分桶统计接口暂不在本次范围，若后续需要另提后端需求）。
- 不改变 SeaTunnel 升级状态机、预检规则判定、目标 JDK 门禁、配置三路合并算法与扩缩容核心业务逻辑（仅重构前端呈现与交互体验）。
- 不新增独立于现有后端数据的实时监控图表（大盘趋势仍归 Dashboard 与监控中心）。

---

## 7. 建议分阶段交付顺序

1. **Phase 0（规范先行）**：全面更新 `.trellis/spec/frontend/{index,ui-conventions,directory-structure}.md`，移除过期规则并固化新设计体系。
2. **Phase 1（列表页重构）**：重构 `ClusterMain` + `TableCard` + `DetailHost` 速览栏 + `ViewToggle` 双视图 + `useClusterLifecycle` 去重 + 单测。
3. **Phase 2（详情页拆解与 6 Tab 重构）**：拆解 `ClusterDetail.tsx`（4031 行），落地 6 个一级 Tab + 内部 `SegmentedControl` + 旧 URL 映射 + 概览压平 + 节点 `TableCard` & `BatchBar`。
4. **Phase 3（配置与弹窗精修）**：`MonitorConfigPanel` `auto_restart` 立即保存、集群弹窗全量迁移 Arc & 压平 & 脏态保护。
5. **Phase 4（升级三阶段页重构 + 全面 E2E 回归）**：重构 `ClusterUpgradePrepare` / `ClusterUpgradeConfig` / `ClusterUpgradeExecute`、运行升级模板 E2E（`upgrade-prepare-template` / `upgrade-prepare-negative`）与新增集群 mock E2E 全面验证。
