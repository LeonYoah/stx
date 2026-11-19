# 设计：首页大盘视觉改版

> 前端改动；不改后端接口、不改 CLI、不改 `components/arc/` 源码。
> 规范：`ui-conventions.md` §5 零塌陷、§14 Arc 组件、§16 控制台首页；`cross-layer-thinking-guide.md`（本任务不跨层，仅消费现有字段）。

## 1. 改动总览

| 区域 | 现在 | 改成 | 文件 |
|---|---|---|---|
| 顶部指标 | 7 个 Arc `MetricCard`，各自带边框 / 34px 圆角 / 阴影，高 158px | 新组件 `KpiStrip`：一个底板 7 个单元格，1px 分隔线，高 ≤ 96px | `DashboardKpiRow.tsx`（改）、`KpiStrip.tsx`（新） |
| 吞吐面板 | 标题行 + `SegmentedControl`（4 项）+ `BrushChart` | 新组件 `ThroughputTabs`（4 个指标格）+ `BrushChart`；分段控件删除 | `JobsTab.tsx`（改）、`ThroughputTabs.tsx`（新） |
| 面板外观 | 5 个文件各写一份 `rounded-xl border border-border/70 bg-card/40 p-4` 与标题 helper | 新组件 `DashboardPanel`（主 / 次两档 + 空态矮条） | `DashboardPanel.tsx`（新）；`JobsTab` / `ClusterTab` / `HostsTab` / `PlatformTab` / `ClusterDetailGroups` 替换 |
| 区块页签 | `TabsList` 被父级 `flex-col` 拉满整行 | 外包一层 `flex`，按内容宽度显示 | `HomeDashboard.tsx` |
| 需要处理 | 无事项时也是带框整行 | 无事项时只留一行浅色文字 | `AttentionStrip.tsx` |

新组件都放在 `components/common/dashboard/`，只在首页使用，不放进 `common/layout/`（复用到作业详情时再上移）。

## 2. 数据来源（全部是现有字段）

| 用途 | 字段 | 接口 |
|---|---|---|
| 指标栏 7 格 | `HomeSummary.jobs.{running, running_streaming, running_batch, write_qps, write_trend, max_cdc_delay_seconds, max_cdc_job_name, cdc_threshold_seconds, failed_24h, failed_prev_24h}`、`clusters.{healthy, total, degraded, unhealthy, nodes_online, nodes_total}`、`alerts.{active, critical}` | `GET /dashboard/home/summary` |
| 数据量格 | `HomeVolume.totals.{window, today, month}`、`oldest_sample_ms` | `GET /dashboard/home/volume` |
| 吞吐 4 格 + 图 | `HomeJobDashboard.throughput[{key, points}]`、`step_seconds`、`annotations` | `GET /dashboard/home/jobs` |

吞吐格的数值在前端从 `points` 计算（§4.2），不新增接口字段。因此 `stx dashboard home summary|jobs|volume` 的输出不变，CLI 无需改动。

## 3. 顶部指标栏 `KpiStrip`

### 3.1 组件接口

```ts
// components/common/dashboard/KpiStrip.tsx
export type KpiTone = 'danger' | 'warning';

export interface KpiStripItem {
  key: string;            // 同时用于 data-testid：home-kpi-${key}
  href: string;           // 整格跳转，沿用现在的目标
  label: string;
  value: number;          // 已按量级缩放后的数值（scaledCompact().value）
  decimals?: number;
  scale?: string;         // 量级：K / M / B…，和数字同字号
  unit?: string;          // 计量单位：行/秒、秒、行、/ 3，小号次要色（传入前 trim）
  context: string;        // 说明，只显示一行
  change?: {percent: number; good: boolean}; // 只有写入速率用
  tone?: KpiTone;         // 数值染色
}

export function KpiStrip(props: {items: KpiStripItem[]; label: string}): JSX.Element;
export function KpiStripSkeleton(props: {count: number}): JSX.Element;
```

`DashboardKpiRow` 只负责把接口数据映射成 `KpiStripItem[]`（§3.3），渲染交给 `KpiStrip`。

### 3.2 结构与样式

```html
<nav aria-label="核心指标" data-testid="home-kpi-row">
  <div class="overflow-hidden rounded-xl border border-border/70 bg-card">       <!-- 底板 -->
    <ul class="-mr-px -mb-px grid grid-cols-1 min-[480px]:grid-cols-2 lg:grid-cols-4 xl:grid-cols-7">
      <li class="shadow-[inset_-1px_-1px_0_var(--border)]" data-tone="danger?">
        <a href="…" data-testid="home-kpi-failed" class="block h-full px-4 py-3 hover:bg-muted/40 focus-visible:… ">
          <span class="block truncate text-xs text-muted-foreground">失败作业</span>
          <span class="mt-1.5 flex items-baseline gap-0.5" style="--text-3xl:1.5rem">
            <AnimatedCounter value={2} />          <!-- 数字 + 量级 -->
            <span class="text-xs text-muted-foreground">行/秒</span>   <!-- 计量单位 -->
          </span>
          <span class="mt-1 block truncate text-xs text-muted-foreground" title="…">近 24 小时 · 昨日 5</span>
        </a>
      </li>
    </ul>
  </div>
</nav>
```

要点：

- **分隔线**：每格用 inset box-shadow 画右边和下边的 1px 线（`DASHBOARD_CELL_DIVIDER` =
  `shadow-[inset_-1px_-1px_0_var(--border)]`，在 `KpiStrip.tsx` 导出，`ThroughputTabs` 的格子复用同一常量）；
  `ul` 用 `-mr-px -mb-px` 往外扩，最右列的右线与末行的底线被底板的 `overflow-hidden` 裁掉。这样无论
  7 / 4 / 2 / 1 列怎么换行，线都连续，末行空位露出的是底板颜色，不会出现色块。
  （不用 `gap-px` + 底色的做法，那样末行空位会露出一整块边框色；也不用「左 + 上」线，那样末行短缺处
  上方那格没有底线，分隔会断。）
- **数字字号**：`AnimatedCounter` 用 `font-size: var(--text-3xl)`（30px）。在包裹层把 `--text-3xl` 局部设为 `1.5rem`，
  数字变成 24px，不改 Arc 源码。量级（K / M）作为 `AnimatedCounter` 的 `suffix` 传入，和数字同字号；计量单位单独一个
  `<span>`，`text-xs` + 次要色，基线对齐（`items-baseline`）。
- **染色**：`tone` 在包裹层覆盖 `--foreground`（`danger` → `var(--danger)`，`warning` → `var(--warning)`），
  `AnimatedCounter` 的数字随之变色。与 `ClusterDetailGroups` 现有做法一致（spec §16「>0 时包层 `--foreground: var(--danger)`」）。
  `li` 上写 `data-tone` 方便测试。
- **涨跌**：写入速率的说明行前面加 `ArrowUpRight` / `ArrowDownRight`（lucide，`size-3`）+ 百分比，`good` 为真用
  `text-success`，否则 `text-destructive`；其余文字仍是次要色。不再用 Arc 的右上角胶囊，所以标题行高度在 7 格里一致。
- **整格可点**：`<Link>` 撑满格子；hover `bg-muted/40`；键盘焦点用 `focus-visible:outline-2 outline-offset-[-2px]`（向内，
  避免被底板裁掉）；去掉现在的 `active:scale-[0.99]`（缩放在连续底板上会露缝）。
- **无障碍**：`<nav aria-label>` + `<ul>`；链接的可读名称就是格子文字（标签、数值、单位、说明），不另写 `aria-label`。
  `AnimatedCounter` 已自带 `srOnly` 文本。
- **高度**：`py-3` + 12px 标签 + 24px 数字 + 12px 说明 ≈ 84–88px。

### 3.3 字段映射

| key | 标签 | value / scale / unit | 说明（一行） | change | tone |
|---|---|---|---|---|---|
| running | 运行中作业 | `jobs.running` | 流 {streaming} · 批 {batch} | — | — |
| write | 全局写入速率 | `scaledCompact(write_qps)` / `行/秒` | 有趋势：较 1 小时前；无趋势：当前合计（10 秒对账） | `trendChangePercent(write_trend)`，`good = percent >= 0` | — |
| cdc | 最大 CDC 延迟 | `scaledCompact(max_cdc_delay_seconds)` / `秒` | 超阈值时：超阈值 · {作业名}；否则：{作业名} 或「暂无 CDC 延迟数据」 | — | `cdc >= cdc_threshold_seconds` → danger |
| failed | 失败作业 | `jobs.failed_24h` | 近 24 小时 · 昨日 {failed_prev_24h} | — | `failed_24h > 0` → danger |
| volume | 数据量 | `scaledCompact(totals.window.write_rows)` / `行` | 现有 `volumeContext`（今日 · 本月 / 自 {日期} 起），超长截断 + `title` | — | — |
| clusters | 集群健康 | `clusters.healthy` / unit `/ {total}` | 有异常：{n} 个异常 · 节点 {online}/{total} 在线；只有降级：{n} 个降级 · 节点 {online}/{total} 在线；否则：节点 {online}/{total} 在线 | — | `unhealthy > 0` → danger；否则 `degraded > 0` → warning |
| alerts | 活跃告警 | `alerts.active` | 严重 {critical} | — | `critical > 0` → danger |

- 失败数、昨日数仍是中性文字，不带正负号（spec §16 既有规则）。
- 「超阈值」「N 个降级」从原来的右上角胶囊移到说明行开头，信息不丢。
- 7 格跳转目标与现在完全一致（`buildHref({tab: 'list', status: 'running'})` 等）。

### 3.4 加载态

`KpiStripSkeleton`：同一个底板与网格，7 个格子里各放三条 `Skeleton`（`h-3 w-16` / `h-6 w-20` / `h-3 w-24`），
总高度与真实指标栏一致。保留 `data-testid="home-kpi-loading"`，且 `ul` 的直接子元素仍是 7 个（现有单测断言 7 个子元素）。

## 4. 「作业」页签吞吐面板 `ThroughputTabs`

### 4.1 组件接口

```ts
// components/common/dashboard/ThroughputTabs.tsx
export interface ThroughputTabItem {
  key: ThroughputKey;              // 'write' | 'read' | 'write_bytes' | 'read_bytes'
  label: string;                   // 现有 i18n：jobsTab.throughput.{key}
  summary: RateSummary | null;     // §4.2
  color: string;                   // THROUGHPUT_COLOR[key]
}

export function ThroughputTabs(props: {
  items: ThroughputTabItem[];
  value: ThroughputKey;
  onValueChange: (key: ThroughputKey) => void;
  label: string;                   // tablist 的 aria-label，沿用 jobsTab.throughput.metricLabel
  panelId: string;                 // 图表区域 id，tab 的 aria-controls 指向它
  formatValue: (key: ThroughputKey, value: number) => {text: string; unit: string};
  peakLabel: (value: string) => string; // t('throughput.peak', {value})
}): JSX.Element;
```

### 4.2 数值计算

在 `dashboard-format.ts` 新增纯函数：

```ts
export interface RateSummary { avg: number; peak: number }
/** 窗口内的均值与峰值；忽略 NaN / Infinity / null；没有有效点时返回 null。 */
export function summarizeRate(points: [number, number][]): RateSummary | null;
```

- **主数值 = 窗口均值**，说明行 =「峰值 {peak}」。
- 为什么不用「当前值」：顶部指标栏的写入速率是 10 秒对账的实时合计（`write_qps`），Prometheus 序列最后一个点和它口径不同，
  两处数字对不上会让人困惑。均值跟随头部选的时间窗，与「写入速率 Top 8 · 窗口内平均」口径一致。
- 不显示涨跌：「较上一时间窗」需要后端对比查询，见 §8 后续。
- 格式化沿用现有：行类 `formatCompactNumber(v)` + 单位 `/s`；字节类 `formatBytes(v)`（返回 "21.3 MB" / "512 B" / "—"）
  按最后一个空格拆成数字和单位，单位后加 `/s`（如 `21.3` + `MB/s`）。`summary === null` 时数值显示「—」，说明行为空。

### 4.3 结构与交互

```html
<DashboardPanel variant="primary" flush testId="home-throughput">
  <div role="tablist" aria-label="吞吐指标" class="-mr-px grid grid-cols-2 lg:grid-cols-4">
    <button role="tab" aria-selected="true" aria-controls="{panelId}" tabindex="0"
            class="relative px-4 py-3 text-left … aria-selected:bg-muted/50">
      <span class="absolute inset-x-0 top-0 h-0.5" style="background: {color}" />   <!-- 仅选中格显示 -->
      <span class="text-xs text-muted-foreground">写入 行/秒</span>
      <span class="mt-1 flex items-baseline gap-0.5 text-xl font-medium tabular-nums">48.2K <small>/s</small></span>
      <span class="mt-0.5 text-xs text-muted-foreground">峰值 61.0K</span>
    </button>
    …
  </div>
  <div id="{panelId}" role="tabpanel" class="p-4">
    <p class="mb-2 text-xs text-muted-foreground">全局吞吐 · 所有作业合计</p>
    <BrushChart …（参数与现在相同）/>
  </div>
</DashboardPanel>
```

- 格子之间用与指标栏相同的 inset 分隔线（`DASHBOARD_CELL_DIVIDER`，右 + 下）；`tablist` 只外扩右边缘 `-mr-px`
  （末行底线保留，兼作与图表的分隔，不再另加 `border-b`）。选中格 `bg-muted/50`，顶部 2px 色条用该序列颜色
  （读 `--series-1`、写 `--series-2`），把格子和下面曲线的颜色对应起来。
- 键盘：roving tabindex（只有选中格 `tabIndex=0`）；←/→ 循环移动并立即选中（选择跟随焦点），Home / End 到首尾。
- 选中状态仍存在 `JobsTab` 的 `metric` 本地 state，不写 URL（与现在一致）。
- 原「全局吞吐」标题行与 `SegmentedControl` 删除；标题和说明作为图表上方的一行小字保留。
- 窄屏 2×2，`lg` 起 4 列。

## 5. 面板 `DashboardPanel`

```ts
// components/common/dashboard/DashboardPanel.tsx
export function DashboardPanel(props: {
  title?: string;
  hint?: string;
  actions?: React.ReactNode;       // 右侧控件，窄屏换行并在容器内横向滚动（沿用现在 header helper 的写法）
  variant?: 'primary' | 'secondary'; // 默认 secondary
  flush?: boolean;                 // 去掉内边距（吞吐面板、带表格的面板用）
  empty?: boolean;                 // true 时渲染为空态矮条
  emptyLabel?: string;
  testId?: string;
  className?: string;
  children?: React.ReactNode;
}): JSX.Element;
```

| 档位 | 样式 | 用在 |
|---|---|---|
| primary | `rounded-xl border border-border/70 bg-card` | 每个页签的主图：作业页签吞吐面板 |
| secondary | `rounded-xl border border-border/50 bg-card/30` | 其余面板（默认） |
| empty | `flex h-11 items-center gap-3 rounded-xl border border-dashed border-border/60 px-4 text-xs`：左侧面板标题（`text-foreground/80`），后面接 `emptyLabel`（次要色） | 作业页签里窗口内无数据的次要面板 |

- 标题 `text-sm font-medium`，说明 `text-xs text-muted-foreground`，与现在 header helper 相同；各文件里的 header helper 删除。
- `empty` 时不渲染 `actions` 和 `children`；`data-testid` 仍挂在同一个根元素上，现有按 testid 找面板的单测不受影响。
- 替换范围（只换外层，不动内容）：`JobsTab`、`ClusterTab`、`HostsTab`、`PlatformTab`、`ClusterDetailGroups` 里
  `const panel = 'rounded-xl border border-border/70 bg-card/40 p-4'` 及同样 class 的面板；
  `overflow-hidden … bg-card/40` 的表格面板用 `flush`。集群 / 主机 / 平台页签全部用 secondary，不设 primary
  （它们没有唯一的主图，改布局不在本任务范围）。
- `AttentionStrip`、`TrendEmptyState` 不用 `DashboardPanel`（它们不是图表面板）。

### 5.1 作业页签哪些面板会缩成矮条

只在 `resource.data` 已加载、且该面板自己的数据为空时缩：

| 面板 | 判空条件（沿用现有分支） |
|---|---|
| 数据量 `home-volume` | `volumeData` 已加载且 `volumeSplit === 'none'` 且 `(volumeData.series ?? []).length === 0`（四个指标都没有行） |
| 写入速率 Top 8 `home-top-write` | `data.top_write.length === 0` |
| CDC 延迟 Top 8 `home-top-cdc` | `data.top_cdc.length === 0` |
| 数据流向 `home-flow` | `flow.links.length === 0` |
| Checkpoint 耗时 `home-checkpoints` | `checkpointData.length === 0` |
| 算子热点 `home-hotspots` | `data.hotspots.length === 0` |

- 吞吐主面板和运行日历不缩：吞吐为空时 `BrushChart` 自己显示 `emptyLabel`（主图保留高度，页面不会整体塌下去）；
  日历本来就有格子。
- `home-volume` 带「指标 / 拆分」两个分段控件：按拆分后的序列判空会把控件一并藏掉、用户切到空指标后切不回来，
  所以只在「不拆分且四个指标都没有行」时缩矮条；其余空态（拆分后无序列、单指标无行）保持面板内 `h-[240px]` 空文案。
- 首次加载仍是骨架；轮询刷新时 `usePolledResource` 保留旧数据，不会在「有数据 ↔ 矮条」之间闪。
- 两个相邻面板都为空时，各自一条矮条并排（仍在原来的两列网格里），不做跨面板合并，保持 DOM 与 testid 稳定。
- 监控不可用（`source !== 'prometheus'`）时维持现状：`TrendEmptyState` + 运行日历。

## 6. 区块页签与「需要处理」

- `HomeDashboard`：把 `<TabsList>` 包在 `<div className='flex'>` 里。Arc 的 `listShell` 是 `inline-flex`，在父级
  `flex-col` 里会被 `align-items: stretch` 拉满；包一层横向 flex 后按内容宽度显示。`TabsList` 的 `className`
  落在内层列表上，改不到外壳，所以不能用 `self-start`。不改 Arc 样式。
- `AttentionStrip`：`visible.length === 0` 时改为
  `<p data-testid="home-attention" class="px-1 text-xs text-muted-foreground">需要处理 · 暂无需要处理的事项</p>`；
  有事项时保持现在的带框横条。E2E 只断言 `home-attention` 可见，不受影响。

## 7. i18n

zh / en 同步，键都在 `dashboard.home` 下：

| 键 | zh | en | 说明 |
|---|---|---|---|
| `kpi.label` | 核心指标 | Key metrics | 指标栏 `aria-label` |
| `kpi.failedContextWithPrev` | 近 24 小时 · 昨日 {count} | Last 24h · {count} yesterday | 合并原 `failedContext` + `failedYesterday` |
| `kpi.cdcOverContext` | 超阈值 · {name} | Over threshold · {name} | |
| `kpi.clustersDegradedContext` | {count} 个降级 · 节点 {online}/{total} 在线 | {count} degraded · {online}/{total} nodes online | |
| `kpi.clustersUnhealthyContext` | {count} 个异常 · 节点 {online}/{total} 在线 | {count} unhealthy · {online}/{total} nodes online | 集群页「异常」徽标对齐，异常优先于降级 |
| `jobsTab.throughput.peak` | 峰值 {value} | Peak {value} | |
| `jobsTab.throughput.avgHint` | 窗口均值 | Window average | 格子 `title`，说明主数值口径 |

删除不再使用的 `kpi.failedContext`、`kpi.failedYesterday`、`kpi.cdcOver`、`kpi.clustersDegraded`（先 grep 确认无其他引用）。
单位字符串 `unit.*` 带前导空格，`KpiStrip` 渲染前 `trim()`，不改词条。

## 8. 后续（不在本任务）

- **吞吐「较上一时间窗」涨跌**：`GET /dashboard/home/jobs` 增加 `compare=previous`，后端对 4 条序列各查一次
  `avg_over_time(...[window] offset window)`，返回 `throughput_summary: [{key, avg, peak, prev_avg}]`；
  CLI `stx dashboard home jobs --compare previous`；前端格子说明行加涨跌。
- **作业详情「指标」页签**复用 `ThroughputTabs`（把组件上移到 `common/layout/` 时再泛化 key 类型）。
- 集群 / 主机页签是否也要「指标页签 + 一张图」，等本任务上线后看效果再定。

## 9. 风险与取舍

- **与 Arc Pro 的边界**：「KPI 页签 + 一张图」是通用的分析看板布局（Plausible、Vercel Analytics 都这样）。本任务用 Arc 免费的
  `AnimatedCounter`、`BrushChart` 和设计变量自己实现，不引入 Pro 源码，视觉细节（字号、间距、色条、分隔线）按 STX 现有 token 定。
- **`MetricCard` 不再用于首页**：spec §16 写的是「6 张 Arc MetricCard」，要同步改掉；`MetricCard` 仍在主机页签小卡、集群细节恒零指标中使用，不删除。
- **数字动画**：`AnimatedCounter` 的 `animateOnView` 保留；11 个计数器（7 + 4）同屏，开销可忽略。
- **视觉验证需要有数据的环境**：本机实例当前无运行作业。验证用有作业的环境，或用 Playwright 拦截
  `/api/v1/dashboard/home/*` 返回夹具数据截图（见 implement.md）。

## 10. 回滚

纯前端、无数据与接口变化：回退本任务提交即可。新组件只在首页引用，不影响其他页面。

## 11. 第二轮追加：范围选择与集群选择条

> 第一轮上线后用户仍反馈「没质感」：头部 Select 与大卡片 `RadioCards` 和新的条带视觉不统一。
> 仍不改接口与 CLI、不改 `components/arc/` 源码。对应 prd「追加范围」。

### 11.1 头部集群范围：shadcn `Select` → Arc `Combobox`

- 依据 `11-18-unified-admin-console-lists/design.md` §3：集群名长度不固定、需要搜索，选型 Arc `Combobox`，
  不新增 `select` / `dropdown-menu` 依赖。
- `HomeDashboard`：保留 `label={t('scope.label')}`（外层 `[&_label]:sr-only` 视觉隐藏但留在 DOM 供无障碍）、
  `placeholder` / `emptyMessage`、`className='min-w-52'`、`data-testid='home-scope'` 经 Arc props 透传到输入框。
- 选项：`all`（全部集群）+ 已加载集群 + 未知 `cluster` 时的 `clusterFallback` 兜底项；
  选中 `all` 仍映射 `clusterId: undefined`，URL 行为不变。

### 11.2 集群页签：`RadioCards` → `ClusterScopeStrip`

- 新组件 `components/common/dashboard/ClusterScopeStrip.tsx`：
  - 复用 `KpiStrip` 的 `DASHBOARD_CELL_DIVIDER` 与底板（`overflow-hidden rounded-xl border bg-card`），
    格子 `-mr-px -mb-px`、`grid-cols-1 sm:grid-cols-2 lg:grid-cols-3`；
  - 语义：外层 `role='radiogroup'` + `aria-label`，格子 `button role='radio'`、`aria-checked`、
    roving tabindex（选中格 `tabIndex=0`）；
  - 选中格 `aria-checked:bg-muted/50` + 顶部 2px `--accent` 色条；焦点用 `--focus-outline-offset: -2px` 内缩；
  - 首格「全部集群」+ 每集群一格，保留原卡片全部信息（名称 / 版本 / 节点在线 / 运行数 / 堆 / 告警 + `HEALTH_TONE` 徽标）；
  - `ClusterScopeStripSkeleton`：同底板 3 格各 3 条骨架，等高不塌陷。
- `ClusterTab`：`update({clusterId: value === 'all' ? undefined : value, page: 1})` 不变。
- **焦点保持**：`usePolledResource` 换 key 时 `data` 归零会把选择条换成骨架、焦点掉回 body。
  `ClusterTab` 用 `useRef` 记住上一组 `ClusterScopeItem[]`，refetch 期间继续渲染旧条，数据回来后替换。
- 键盘导航抽到 `use-roving-keys.ts`（←/↑ 上一格、→/↓ 下一格循环、Home/End 首尾），
  `ClusterScopeStrip` 与 `ThroughputTabs` 共用。

### 11.3 控制台告警清理（React 19 / Radix / GSAP）

| 告警 | 根因 | 修法 |
|---|---|---|
| `Each child in a list should have a unique "key"`（ClusterDetailGroups） | React 19 对拼进 children 数组的动态兄弟表达式逐个校验 key；且 `cellShell()` 返回的对象带 `key` / `title` / `unit` 被 `{...shell}` 展开（key 展开告警 + 非 DOM 属性泄漏） | 动态兄弟（actions、事件列表、加载 / 空 / 网格三分支、格内 head / body、MetricCard / Sparkline）全部显式 `key`；`cellShell` 只留 `className` / `data-testid` |
| `DialogContent requires a DialogTitle` / `Missing Description`（×2，StrictMode 双调用） | `OnboardingPathChooser` 给 `DialogTitle` / `DialogDescription` 传了自定义 `id`，覆盖 Radix context 生成的 `titleId` / `descriptionId`，`aria-labelledby` 指向不存在的元素 | 删掉自定义 `id`，让 Radix 生成；不改 Arc 源码 |
| `GSAP target .radar-pulse-ring not found` | `StatPillsBar` 无 `pulse` 项时选择器为空仍启动动画 | 动画前 `querySelector` 探测，无目标不启动 |
| `<path d="undefined">` | 富夹具全页签扫描未复现；`Sparkline` 已把非有限值转 `null`、无有效基线时返回空 path | 不改共享图表内部，保持观察 |
| `401 user-info` / `proxy.kapa.ai 404` / `requestStorageAccess` / RSC 预取 `ERR_ABORTED` | 登录前预检、第三方 Ask AI 代理、第三方存储权限、Next 软导航取消预取 | 非本仓库缺陷，分类记录 |

验证：Playwright 夹具全页签扫描无 React 警告；ArrowRight 选集群 → `?cluster=11` 且焦点落在 `home-cluster-scope-11`，ArrowLeft 返回 `home-cluster-scope-all`；dashboard 54 单测 + `tsc` 通过。
