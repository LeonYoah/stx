# 实施计划：工作台响应式布局重构

> 开工前必读：
> - 任务规划：`prd.md`、`design.md`
> - 项目规范：`AGENTS.md`、`.trellis/spec/frontend/index.md`、`.trellis/spec/frontend/ui-conventions.md`
>
> 编码纪律：
> - 新增或修改的注释必须**中英双语，中文在前，英文在后**，仅解释 why/约束。
> - **严禁修改 `frontend/components/arc/*` vendored 源码**，直接复用公开 props。
> - 保持最小改动，不改动非 `/workbench` 路由的外层布局，不破坏既有 E2E 选择器与 `localStorage` 偏好。
> - **完成后不要 `git commit` 或 `git push`**，交由导师逐项审查后再提交。

---

## Phase 1 · 纯函数响应式布局引擎（`sync-studio-utils.ts` + 单测）

- [ ] 在 `frontend/components/common/sync/sync-studio-utils.ts` 新增：
  - `StudioViewportMode`（`'wide' | 'medium' | 'compact'`）、`StudioTopbarTier`（`'wide' | 'medium' | 'compact'`）；
  - 断点与保护常量：`STUDIO_WIDE_BREAKPOINT = 1280`、`STUDIO_MEDIUM_BREAKPOINT = 960`、`STUDIO_TOPBAR_WIDE_BREAKPOINT = 1200`、`STUDIO_TOPBAR_MEDIUM_BREAKPOINT = 900`、`STUDIO_CENTER_MIN_WIDTH = 420`、`STUDIO_EDITOR_MIN_HEIGHT = 180`；
  - `resolveStudioViewportMode(containerWidth)`、`resolveStudioTopbarTier(containerWidth)`、`resolveEffectiveStudioLayout(input)`；
  - 保持既有 `buildStudioGridTemplateColumns`、`buildStudioGridTemplateRows` 导出签名与行为不变。
- [ ] 在 `frontend/components/common/sync/__tests__/sync-studio-utils.test.ts` 补全单元测试：
  - SSR/0 宽回退 `'wide'`；
  - `1440 / 1280 / 1100 / 960 / 800 / 480` 各宽度下的 `viewportMode`、`topbarTier`、`overlaySidebars`、中栏 ≥420px 宽度保护；
  - 宽屏双栏超出预算时的夹紧与 `maxLeftWidth` / `maxRightWidth` 动态上限；
  - 中屏两侧均开时的单侧停靠兜底；
  - 矮视口（600px / 500px / 300px）下控制台高度夹紧（保护编辑器 ≥180px 或极端高度按比例）与最大化模式。

---

## Phase 2 · 外层视口外壳（`app/(main)/layout.tsx`）

- [ ] 修改 `frontend/app/(main)/layout.tsx`：
  - 判断 `isWorkbench = pathname === '/workbench' || pathname.startsWith('/workbench/')`；
  - 仅在 `isWorkbench` 时切换为全视口贴合外壳（`h-dvh min-h-[520px] overflow-hidden` + `pt-2.5 pb-[84px] px-2.5 sm:px-3`），非工作台路由保持原样不动。

---

## Phase 3 · 工作台顶栏、三栏自适应与窄屏抽屉（`DataSyncStudio.tsx`、`ConsolePanel.tsx`、`studio-chrome.css`）

- [ ] 核对 `ExpandingButtonGroup` 的 `defaultExpanded` 行为（若传入 `null` 会回退首项，则按需传参或保持中屏紧凑，不改 Arc 源码）。
- [ ] 修改 `frontend/components/common/sync/DataSyncStudio.tsx`：
  - 根节点改为 `h-full min-h-0 w-full flex-1 flex-col gap-2 overflow-hidden`（去掉 `-mx-2 lg:-mx-3 h-[calc(100vh-96px)] min-h-[780px]`）；
  - 挂载 `ResizeObserver` 监听根节点宽度与主工作区宽高，计算 `effectiveLayout = resolveEffectiveStudioLayout(...)`；
  - 区分“用户显式操作（持久化 `localStorage`）”与“窗口缩放自动收起（不污染 `localStorage`）”；
  - 重构顶栏：
    - 去掉硬编码 `280px` 静态列，改用 `w-9 shrink-0` 锚点 + `pointer-events-none` 浮层定位盒承托 `<ExpandingSearch anchor='end' expandedWidth={280} />`；
    - 去掉顶栏冗余父目录段（`title` 保留完整路径）；
    - 按 `topbarTier` 分三档渐进收拢（宽屏全显 / 中屏藏集群名+主按钮 `iconOnly` / 窄屏探查进菜单+藏版本与只读微胶囊）；
  - 重构主网格与侧栏：
    - `gridTemplateColumns`、`gridTemplateRows` 与三个 `StudioSplitHandle` 的 `value` / `max` / `style` 全部消费 `effectiveLayout`；
    - `medium` 与 `compact` 模式下左右开关互斥；`wide → medium` 双开自动暂收右栏，`* → compact` 自动收起侧栏；`compact` 下选文件自动收左抽屉；
    - `compact` 下将原左右侧栏 DOM 切为 `absolute` 贴边抽屉 + 遮罩层 + `Esc` 关闭，保留 `data-studio-left-sidebar` / `data-studio-right-sidebar` / `inert` / `aria-hidden`；
  - 内部细条：空编辑器画布去掉 `min-h-[380px]`；只读横幅与 `actionPending` 条补 `flex-wrap min-w-0`；控制台顶栏 `SegmentedControl` 允许横向滚动、右侧按钮 `shrink-0`。
- [ ] 修改 `frontend/components/common/sync/ConsolePanel.tsx`：
  - `MixedLogModeBanner` 与 `JobLogsEmptyBanner` 去掉 `min-h-[180px]` 改为 `min-h-0 overflow-auto`；
  - 日志头两行改为窄宽不裁切的弹性折行布局。
- [ ] 按需微调 `frontend/components/common/sync/studio-chrome.css`（保留既有 `studio-explorer-width.test.ts` 全部断言）。

---

## Phase 4 · 全量验证与汇报

- [ ] 运行全部前端检查并贴出真实输出：
  ```bash
  cd frontend && pnpm exec tsc --noEmit
  cd frontend && pnpm exec vitest run lib/i18n/__tests__/translation-coverage.test.ts components/common/sync/__tests__/
  ```
- [ ] 对照 `prd.md` AC1–AC6 逐项自检，确认无 `cursor-not-allowed`、无水合风险、无改动 `components/arc/*`、工作区未提交，汇报改动清单与测试输出等待导师审查。
