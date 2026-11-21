# 设计：列表与常驻详情栏联动动效（克制版）

## 1. 结论

- 联动外观**全部由公共层驱动**：`list-detail.module.css` 用 `:has()` 判断"详情栏打开且本页有选中行"，自动切换为联动外观。
  7 个列表不改行结构、不加 class，选中行继续用现有的 `.rowActive`。
- `DetailHost` 改两处：常驻栏在关闭动画播完后才卸载；新增可选的名称飞入。
- `TableCard` 只加两个 `data-slot` 属性，方便公共样式定位外框和底栏。
- 各列表只在名称飞入上做少量接入：给行内名称加一个属性，把点击的行传给 `DetailHost`（§4）。
- 窄屏抽屉和"减少动态效果"一律降级，不做位移和飞入。

原型：`research/prototype.html`。下文的数值都以原型为准。

## 2. 现状（代码事实）

- `DetailHost.tsx`：
  - 窄屏（`useDetailNarrow`，<1080px）走 Arc `Drawer`；
  - 宽屏渲染 `<aside className={styles.rail} data-testid='detail-rail'>`，`if (!open) return null`，关闭即卸载，没有退出动画；
  - J/K 和头部 ↑↓ 通过 `nav.onPrev / onNext` 切换。
- `list-detail.module.css`：
  - `.split { gap: 16px }`；
  - `.rail` 有 1px 边框、`--radius-panel`、sticky，宽 440px（`.railWide` 520px），`max-width: 46%`；
  - `.row:hover` 即时填充，`.rowActive` 是 `--accent-subtle` 底色。
- `TableCard.tsx` 根节点带 `overflow-hidden border bg-card`，圆角 `--radius-panel`；表头 `.rowHead`、行容器 `.tableScroll`（`overflow: auto`）、分页底栏都在卡片内。
- 7 个列表的行都是 `.tableScroll` 的直接子元素（`div role='button'`），选中时加 `layout.rowActive`；
  点击行内按钮、复选框等交互元素不会触发选中。
- 每页 10 行（错误中心 20 行），轮询或刷新会重新渲染行。

## 3. 外观与动效规格

### 3.1 何时进入联动外观

条件：`.split` 内的常驻栏处于打开状态（`[data-state='open']`），**并且**本页存在 `.rowActive` 行。
两个条件都满足时，下表全部生效；只满足第一个（例如翻页后选中行不在本页）时，列表保持原样，只有详情栏打开。

### 3.2 外观

| 部分 | 规格 |
|---|---|
| 分栏间距 | `.split` 的 `gap` 由 16px 过渡到 0 |
| 列表外框 | `TableCard` 边框和底色变透明；表头、各行、底栏改用底层色，去掉行分隔线 |
| 底层色 | 在 `list-detail.module.css` 内定义局部变量，取 `--surface` 与页面背景的中间值：浅色约 `oklch(97.4% 0 0)`，深色约 `oklch(19.5% 0 0)`。不新增全局 token |
| 选中行 | `--surface` 白底、`--shadow-floating` 阴影、`translateY(-1px)`；左侧圆角 `--radius-control`，右侧直角；右端贴到详情栏 |
| 列表分段 | 选中行的上一个元素（行或表头）加下圆角，下一个元素（行或底栏）加上圆角，半径 `--radius-panel` |
| 接口凹圆角 | 选中行右端上下各一块 10px 凹圆角，用行的 `::before` 加两层径向渐变实现，颜色 `--surface` |
| 详情栏 | 去掉边框，保留 `--shadow-floating`；`margin-left: -1px`，盖住卡片透明边框留下的 1px 缝 |
| 其余行 | 行内内容（`.row > *`）的不透明度为 `1 − min(距离, 5) × 0.1`，不加模糊；只作用于内容，段的底色保持实心 |
| 工具栏 | 同一 `.page` 内的 `.toolbar` 不透明度 0.7；`:hover`、`:focus-within` 时恢复为 1 |
| 悬停变淡的行 | 立即恢复为 1 |

**距离用 CSS 兄弟选择器表达，不需要 JS**：
- 下方的行：`.rowActive + .row`（距离 1）、`.rowActive + .row + .row`（距离 2），依此类推到 4；`.rowActive ~ .row` 作为兜底，即 50%。
- 上方的行：`.row:has(+ .rowActive)`、`.row:has(+ .row + .rowActive)`，依此类推到 4；`.row:has(~ .rowActive)` 作为兜底。

**列表分段也只靠选择器**：
- 上一个元素：`.row:has(+ .rowActive)`；
- 选中行是第一行时：`.listCol:has(.tableScroll > .rowActive:first-child) .rowHead`；
- 下一个元素：`.rowActive + .row`；
- 选中行是最后一行时：`.listCol:has(.rowActive:last-child) [data-slot='table-card-footer']`。

**滚动条**：联动外观下 `.tableScroll` 设 `scrollbar-width: none`，仍可滚动，避免滚动条夹在选中行和详情栏之间。

### 3.3 时序

缓动统一用 `cubic-bezier(.22, 1, .36, 1)`。

| 动作 | 规格 |
|---|---|
| 详情栏打开 | 宽度 0 → 栏宽，以及 `translateX(20px)` → 0，均 520ms；不透明度 320ms；`blur(6px)` → 0 用 380ms；整体延迟 60ms。栏内容固定为栏宽，宽度动画期间不重新排版 |
| 分栏间距、卡片边框、底层色、段圆角 | 280–320ms |
| 其余行变淡 | 380ms，按距离错开，每级 24ms |
| 接口凹圆角 | 淡入 240ms，延迟 280ms，等选中行就位后才出现 |
| 栏内分区依次进入 | 打开时，`.detailScroll` 的前 6 个直接子元素依次淡入并上移 6px，各 440ms，间隔 45ms，起始延迟 150ms；切换行时改为 260ms，间隔 26ms |
| 关闭 | 以上过程反向，不错开；常驻栏在 `transitionend` 后卸载，600ms 兜底 |
| 名称飞入 | 520ms，见 §4 |

### 3.4 降级

- **窄屏**：抽屉形态不受影响。联动外观的选择器都挂在常驻栏上，窄屏下不存在常驻栏，自然不生效。
- **减少动态效果**（`prefers-reduced-motion: reduce`）：过渡只保留 `opacity` 和颜色，去掉位移、模糊和宽度动画；不飞入；分区不错开。

## 4. 名称飞入

### 4.1 接口

- 行内名称元素加 `data-detail-name`。
- 列表持有 `const flyOrigin = useRef<HTMLElement | null>(null)`。行的 `onClick` 和回车打开时，先执行 `flyOrigin.current = e.currentTarget`，再 `setActive`；J/K、↑↓、深链恢复都不写入。
- `DetailHost` 新增两个可选属性：
  - `flyOrigin?: RefObject<HTMLElement | null>`；
  - `nameSlot?: 'title' | 'meta'`，表示名称显示在头部的哪个位置。头部对应元素加 `data-detail-name-target`。

### 4.2 流程

`DetailHost` 在 `useLayoutEffect` 中处理，依赖 `open`、`title`、`meta`：

1. 读取并清空 `flyOrigin.current`；为空、窄屏、减少动态效果、或没有 `nameSlot` 时直接返回。
2. 在来源行里取 `[data-detail-name]`，和头部落点比较 `textContent`（去掉首尾空白），不一致就不飞。
3. 起点用 `getBoundingClientRect()`。终点用落点相对常驻栏的 `offsetLeft / offsetTop` 累加，加上常驻栏**最终**位置（分栏右边缘减去栏宽，顶部对齐分栏），不受栏正在滑入的位移影响。
4. 复制来源元素，放到 `document.body`，`position: fixed`，层级高于常驻栏，宽度锁定为起点宽度。
5. 用 Web Animations API 做位移加缩放，缩放比为两者计算后字号之比，520ms，`fill: 'forwards'`。动画期间落点 `opacity: 0`；结束后恢复落点、移除复制品。
6. 再次选中、关闭或卸载时，取消动画、移除复制品并恢复落点，不留残影。

### 4.3 各列表配置

| 列表 | 来源（行内名称） | `nameSlot` |
|---|---|---|
| 告警 | 告警名 `displayName(alert)` | `title` |
| 告警规则 | 规则名（按语言取 `name_en` / `name`） | `title` |
| 作业规则 | 作业名 | `meta`（`meta={taskName}`） |
| 通知渠道 | 渠道名 | `meta` |
| 诊断策略 | 策略名 | `meta` |
| 错误中心 | 错误组标题 | `meta` |
| 巡检 | 不接入：行里显示编号，头部是"报告 #编号"，文字不一致 | — |

运行时还有第 2 步的文字比较兜底：配置写错时只会不飞，不会飞错。

## 5. 改动清单

| 文件 | 改动 |
|---|---|
| `common/layout/list-detail.module.css` | §3 的全部样式与降级媒体查询，加双语注释说明 `:has()` 的判断条件 |
| `common/layout/DetailHost.tsx` | 常驻栏输出 `data-state`；关闭后保持挂载到动画结束再卸载；栏宽改为 CSS 变量供宽度动画使用；`flyOrigin`、`nameSlot` 与飞入逻辑 |
| `common/layout/TableCard.tsx` | 根节点加 `data-slot='table-card'`，分页底栏加 `data-slot='table-card-footer'` |
| 6 个接入飞入的列表 | 行内名称加 `data-detail-name`；`flyOrigin` 引用；传 `nameSlot` |
| `InspectionsPanel.tsx` | 不改（自动获得联动外观） |
| `.trellis/spec/frontend/ui-conventions.md` | "列表 + 常驻详情分栏"条目补充联动外观、降级与飞入接入方式 |

不改 `DeliveriesPanel` 和窄屏抽屉。

## 6. 兼容与依赖

- **PR #163**：主机列表接入 `DetailHost` 后，只要行是 `.tableScroll` 的直接子元素、选中时用 `.rowActive`，就自动获得联动外观；飞入另按 §4 接入。两个 PR 谁先合并都不冲突；后合并的一方顺手为主机接入飞入。
- **`:has()` 的浏览器支持**：Chrome 105+、Safari 15.4+、Firefox 121+。不支持的浏览器保持现有外观，功能不受影响。
- **现有测试**：
  - 依赖 `detail-rail` 在关闭后立即消失的单测和 E2E 要改成等待卸载；
  - 依赖 `.rowActive` 底色的截图断言需要更新。

## 7. 风险

| 风险 | 应对 |
|---|---|
| 行不是 `.tableScroll` 的直接子元素，兄弟选择器失效 | Phase 0 逐个确认 7 个列表的结构，不符合的只调整包裹层 |
| `TableCard` 的 `overflow-hidden` 裁掉选中行的阴影和接口凹圆角 | 凹圆角在行的上下方、卡片内部，不会被裁；阴影只在滚动区上下边缘被裁，可接受 |
| 宽度动画让列表每帧重排 | 每页 10–20 行，代价可接受；栏内容固定宽度，不跟着重排 |
| 关闭动画期间又打开另一行 | 详情栏状态直接回到打开并更新内容，取消卸载计时 |
| 轮询刷新时选中行被移出本页 | 按 §3.1 回到未联动外观，不报错 |
