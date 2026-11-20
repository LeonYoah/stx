# 实施清单

> 开工前：`trellis-before-dev` 读 `frontend/index.md`、`ui-conventions.md`（§5、§13、§14、§16 与一致性检查项）、`e2e-testing.md`。
> 每步可单独验证；按顺序做，前一步的单测通过再进下一步。

## 0. 准备
- [ ] 先在有运行作业的环境（或第 6 步的夹具）截「改前」图：深 / 浅主题 × 集群页签、作业页签（有数据 / 无数据），存到任务目录
      `screenshots/before-*.png`，PR 里做前后对比。

## 1. 公共函数与面板
- [ ] `dashboard-format.ts` 新增 `summarizeRate(points)`（design §4.2）。
- [ ] 单测 `dashboard-format.test.ts`：空数组 → null；全是 NaN → null；混有 NaN / Infinity 时忽略；均值与峰值正确。
- [ ] 新建 `DashboardPanel.tsx`（design §5）：primary / secondary / flush / empty 四种渲染。
- [ ] 单测 `DashboardPanel.test.tsx`：`empty` 时只渲染标题 + `emptyLabel`，不渲染 actions 和 children，`data-testid` 仍在根元素上。
- 验证：`cd frontend && pnpm exec vitest run components/common/dashboard`

## 2. 顶部指标栏
- [ ] 新建 `KpiStrip.tsx`（`KpiStrip` + `KpiStripSkeleton`，design §3.1–3.4）。
- [ ] `DashboardKpiRow.tsx` 改为生成 `KpiStripItem[]`（design §3.3 映射表），删除 `MetricCard` 引用与 `scaledKpi` 里拼接单位的逻辑
      （量级进 `scale`，单位进 `unit`）。
- [ ] i18n：新增 `kpi.label`、`kpi.failedContextWithPrev`、`kpi.cdcOverContext`、`kpi.clustersDegradedContext`；
      grep 确认后删除 `kpi.failedContext`、`kpi.failedYesterday`、`kpi.cdcOver`、`kpi.clustersDegraded`（zh / en 同步）。
- [ ] 改 `DashboardKpiRow.test.tsx`：mock 从 `MetricCard` 换成 `AnimatedCounter`；断言
  - 7 个 `home-kpi-*` 链接与 href 不变；
  - 写入速率涨跌：`write_trend` 上涨显示 ↗ 与 `text-success`，下跌显示 ↘ 与 `text-destructive`，无趋势时显示「当前合计」；
  - `failed_24h > 0` → `data-tone="danger"`，为 0 时无 `data-tone`；CDC 超阈值 → danger 且说明以「超阈值 ·」开头；
    `critical > 0` → danger；集群有降级 → warning 且说明以「N 个降级 ·」开头；
  - 单位与数字分开（单位节点存在且不在计数器里）；
  - 加载态 `home-kpi-loading` 下仍是 7 个格子。
- 验证：同上 vitest；`pnpm exec tsc --noEmit`

## 3. 作业页签吞吐面板
- [ ] 新建 `ThroughputTabs.tsx`（design §4.1、§4.3），含 roving tabindex 与 ←/→/Home/End。
- [ ] `JobsTab.tsx`：吞吐面板改为 `DashboardPanel variant="primary" flush` + `ThroughputTabs` + `BrushChart`；删除吞吐的
      `SegmentedControl`；`summarizeRate` 用 `useMemo` 按 `data` 计算 4 条。
- [ ] i18n：新增 `jobsTab.throughput.peak`、`jobsTab.throughput.avgHint`。
- [ ] 改 `JobsTab.test.tsx` 里「switches throughput metrics」用例：点击格子切换；在选中格上按 ArrowRight 切到下一个并获得焦点；
      页面上不再有「吞吐指标」分段控件；均值 / 峰值文本按夹具计算正确；某条序列为空时显示「—」。

## 4. 面板替换与空态矮条
- [ ] `JobsTab`：其余面板换成 `DashboardPanel`（secondary），按 design §5.1 表格传 `empty`；删除本文件的 `header` helper 与 `panel` 常量。
- [ ] `ClusterTab`、`HostsTab`、`PlatformTab`、`ClusterDetailGroups`：只把面板外层换成 `DashboardPanel`（secondary；表格面板 `flush`），
      不改内容与布局；删除各自的 `panel` 常量 / header helper。
- [ ] `JobsTab.test.tsx` 新增：`top_write` / `top_cdc` / `checkpoints` 为空时对应 testid 元素是矮条（含标题与「该时间窗内没有数据」、
      不含图表）；有数据时正常渲染。
- [ ] 跑 `ClusterTab` / `HostsTab` / `PlatformTab` 现有单测，确认替换外层不影响断言。
- [ ] 骨架：`JobsTab` 加载态第一块高度改成新吞吐面板的高度（格子行约 76px + 图表区约 320px），避免加载完成后跳动。

## 5. 区块页签与「需要处理」
- [ ] `HomeDashboard.tsx`：`<TabsList>` 外包 `<div className='flex'>`。
- [ ] `AttentionStrip.tsx`：无事项时渲染无框单行文字（design §6）；补单测（空 / 有事项两种）。

## 6. 视觉验证
- [ ] 准备有数据的页面：
  - 首选有运行作业的环境（`go run main.go api` + `pnpm dev`，或已部署实例）；
  - 本机无作业时，用 Playwright `page.route('**/api/v1/dashboard/home/**')` 返回夹具 JSON（字段按 `home.service.ts` 类型，
    覆盖：写入上涨、失败 > 0、CDC 超阈值、集群降级、一条吞吐序列为空、Top/Checkpoint 为空），夹具写法参考 `e2e-testing.md`。
- [ ] 截「改后」图：深 / 浅主题 × 宽度 1440 / 1024 / 768 × 作业页签有数据 / 无数据；检查
  - 指标栏换行处分隔线连续、末行无色块；三条基线对齐；单位小号；染色正确；
  - 吞吐格选中色条与曲线同色；键盘切换可见焦点；
  - 矮条高度 ≤ 48px；区块页签按内容宽度；无事项时「需要处理」无框。
- [ ] 截图放任务目录 `screenshots/after-*.png`，PR 描述里前后对比。

## 7. 收尾
- [ ] spec `ui-conventions.md` §16：
  - 「结构」「指标卡」两条改为：顶部 `KpiStrip`（一个底板 7 格、inset 分隔线、数字 24px + 小号单位、`tone` 染色规则、
    涨跌只给写入速率、加载骨架同高）；不再写「6 张 Arc MetricCard」「158px 骨架」；
  - 「作业页签」改为：吞吐用 `ThroughputTabs`（窗口均值 + 峰值，选中格色条与序列同色）驱动 `BrushChart`，不再有分段控件；
  - 新增「面板层级」：`DashboardPanel` 主 / 次 / 空态矮条，每页签至多一个主面板；空态矮条只在数据已加载且为空时出现；
  - 「区块页签按内容宽度」「需要处理无事项时无框」各一句；
  - 一致性检查项把「首页指标卡的 change…」改成针对 `KpiStrip`，并加「新面板是否用 `DashboardPanel`」。
- [ ] 全量：`cd frontend && pnpm exec tsc --noEmit && pnpm test`；`pnpm exec playwright test e2e/dashboard.spec.ts`（testid 未变，应直接通过）。
- [ ] 确认 CLI 不受影响：本任务未改 `/dashboard/home/*` 的请求参数与响应字段（`git diff --stat` 中无 `internal/`）。
- [ ] 中文提交；PR 附前后截图；在 PR #163 下留言说明大盘指标栏 / 作业页签 / 面板层级由本任务负责。

## 回滚点
- 第 2 步（指标栏）、第 3 步（吞吐面板）、第 4 步（面板替换）、第 5 步各自独立提交，出问题可单独回退。
