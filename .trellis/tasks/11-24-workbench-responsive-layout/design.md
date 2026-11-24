# 技术设计：工作台响应式布局重构

## 1. 视口高度与外层外壳（`app/(main)/layout.tsx`）

### 现状冲突
- `ProjectLayout` 为列表/滚动页设置了 `min-h-screen` + `mb-8 pb-28 py-6`（纵向外边距合计 `168px`）。
- `DataSyncStudio` 内部又设置了 `h-[calc(100vh-96px)] min-h-[780px]`，叠加恒大于 `100vh`，必出整页滚动条，且 720p/800p 笔记本把控制台顶到屏外。

### 设计方案
在 `frontend/app/(main)/layout.tsx` 按路径区分全视口工作台模式：
```tsx
const isWorkbench =
  pathname === '/workbench' || pathname.startsWith('/workbench/');
```
- 当 `isWorkbench === true`：
  - 最外层容器：`h-dvh min-h-[520px] flex flex-col overflow-hidden`（窗口高度 ≥520px 时锁死一屏零整页滚动；极端小高度允许自然滚动不把编辑器压成 0）。
  - 中间包裹：`flex min-h-0 flex-1 flex-col` + `@container/main flex min-h-0 flex-1 flex-col`。
  - `key={pageKey}` 业务内容容器：`flex min-h-0 flex-1 flex-col w-full max-w-none px-2.5 sm:px-3 pt-2.5 pb-[84px] animate-in fade-in-50 duration-200`（`84px` 精确避让 `bottom-4` + ~56px 高度的 `FloatingDock` 并保留 12px 呼吸缝）。
- 当 `isWorkbench === false`：保持原类名 100% 不变，零回归风险。

在 `DataSyncStudio.tsx`：
- 根节点改为：`arc-compact flex h-full min-h-0 w-full flex-1 flex-col gap-2 overflow-hidden bg-background/10`（彻底移除 `-mx-2 lg:-mx-3 h-[calc(100vh-96px)] min-h-[780px]`）。
- 空编辑器引导容器（原 `min-h-[380px]`）改为：`flex h-full min-h-0 select-none flex-col items-center justify-center gap-3 overflow-auto p-6 text-center`。

---

## 2. 响应式布局引擎（纯函数，`sync-studio-utils.ts`）

将所有断点判定与尺寸夹紧收敛到 `frontend/components/common/sync/sync-studio-utils.ts`，便于单测全覆盖且避免逻辑散落组件内部。

### 2.1 常量与类型

```ts
/** 工作台主区宽度模式：宽屏三栏 / 中屏互斥单栏 / 窄屏浮层抽屉。
 *  Workbench width tier: wide three-column / medium single-docked / compact overlay drawers. */
export type StudioViewportMode = 'wide' | 'medium' | 'compact';

/** 顶栏紧凑档位：完整 / 紧凑图标 / 极窄折叠。
 *  Topbar density tier: full / compact icon actions / narrow folded inspect menu. */
export type StudioTopbarTier = 'wide' | 'medium' | 'compact';

export const STUDIO_WIDE_BREAKPOINT = 1280;
export const STUDIO_MEDIUM_BREAKPOINT = 960;
export const STUDIO_TOPBAR_WIDE_BREAKPOINT = 1200;
export const STUDIO_TOPBAR_MEDIUM_BREAKPOINT = 900;
export const STUDIO_CENTER_MIN_WIDTH = 420;
export const STUDIO_EDITOR_MIN_HEIGHT = 180;
```

### 2.2 纯函数契约

1. `resolveStudioViewportMode(containerWidth: number): StudioViewportMode`
   - `containerWidth <= 0`（SSR / 尚未测量首帧）→ `'wide'`（与服务端默认一致，零水合差异）。
   - `>= 1280` → `'wide'`
   - `>= 960` → `'medium'`
   - `< 960` → `'compact'`

2. `resolveStudioTopbarTier(containerWidth: number): StudioTopbarTier`
   - `containerWidth <= 0` 或 `>= 1200` → `'wide'`
   - `>= 900` → `'medium'`
   - `< 900` → `'compact'`

3. `resolveEffectiveStudioLayout(input)`：
   - 输入：
     ```ts
     export interface StudioEffectiveLayoutInput {
       containerWidth: number;
       containerHeight: number;
       leftCollapsed: boolean;
       rightCollapsed: boolean;
       leftWidth: number;
       rightWidth: number;
       consoleHeight: number;
       consoleMaximized: boolean;
     }
     ```
   - 输出：
     ```ts
     export interface StudioEffectiveLayout {
       viewportMode: StudioViewportMode;
       topbarTier: StudioTopbarTier;
       /** 是否以浮层抽屉承载左右侧栏 / Whether sidebars render as overlay drawers */
       overlaySidebars: boolean;
       /** 实际停靠在网格列中的开合态（窄屏均为收起）/ Docked-in-grid collapse flags (both true in compact) */
       dockedLeftCollapsed: boolean;
       dockedRightCollapsed: boolean;
       effectiveLeftWidth: number;
       effectiveRightWidth: number;
       maxLeftWidth: number;
       maxRightWidth: number;
       effectiveConsoleHeight: number;
       maxConsoleHeight: number;
       gridTemplateColumns: string;
       gridTemplateRows: string;
     }
     ```
   - 核心计算规则（**绝不修改传入的偏好值，仅计算本次渲染的安全生效值**）：
     - **宽度保护**：
       - `overlaySidebars = viewportMode === 'compact'`。
       - 若 `overlaySidebars`：`dockedLeftCollapsed = true`、`dockedRightCollapsed = true`、`gridTemplateColumns = '0px minmax(0,1fr) 0px'`。
       - 若 `viewportMode === 'medium'` 且 `!leftCollapsed && !rightCollapsed`（瞬态兜底）：`dockedRightCollapsed = true`，保证单侧停靠。
       - 当 `containerWidth > 0` 且存在停靠侧栏时，可用侧栏预算 `budget = Math.max(0, containerWidth - STUDIO_CENTER_MIN_WIDTH)`：
         - 仅左栏停靠：`maxLeftWidth = clamp(budget, STUDIO_LEFT_WIDTH_MIN, STUDIO_LEFT_WIDTH_MAX)`（若连 `STUDIO_LEFT_WIDTH_MIN` 都放不下，则以 `Math.max(160, budget)` 兜底），`effectiveLeftWidth = Math.min(clampStudioLeftWidth(leftWidth), maxLeftWidth)`。
         - 仅右栏停靠：同理计算 `maxRightWidth` 与 `effectiveRightWidth`。
         - 两侧均停靠（仅 `wide` 可能）：
           - 若 `wantLeft + wantRight > budget`，按比例或优先压缩较宽侧使其总和 `<= budget`（且各自尽量不低于 `STUDIO_LEFT_WIDTH_MIN` / `STUDIO_RIGHT_WIDTH_MIN`）；
           - 动态拖拽上限：`maxLeftWidth = clamp(budget - effectiveRightWidth, STUDIO_LEFT_WIDTH_MIN, STUDIO_LEFT_WIDTH_MAX)`，`maxRightWidth = clamp(budget - effectiveLeftWidth, STUDIO_RIGHT_WIDTH_MIN, STUDIO_RIGHT_WIDTH_MAX)`。
     - **高度保护**：
       - 若 `consoleMaximized`：`gridTemplateRows = '0px minmax(0,1fr)'`。
       - 若 `containerHeight <= 0`：`maxConsoleHeight = STUDIO_CONSOLE_HEIGHT_MAX`，`effectiveConsoleHeight = clampStudioConsoleHeight(consoleHeight)`。
       - 若 `containerHeight > 0`：
         - 若 `containerHeight - STUDIO_EDITOR_MIN_HEIGHT >= STUDIO_CONSOLE_HEIGHT_MIN`：
           `maxConsoleHeight = Math.min(STUDIO_CONSOLE_HEIGHT_MAX, containerHeight - STUDIO_EDITOR_MIN_HEIGHT)`；
           `effectiveConsoleHeight = Math.min(clampStudioConsoleHeight(consoleHeight), maxConsoleHeight)`。
         - 若容器极矮（`containerHeight < STUDIO_EDITOR_MIN_HEIGHT + STUDIO_CONSOLE_HEIGHT_MIN`）：
           `maxConsoleHeight = Math.max(110, Math.floor(containerHeight * 0.45))`；
           `effectiveConsoleHeight = Math.min(clampStudioConsoleHeight(consoleHeight), maxConsoleHeight)`。
     - 保留原有 `buildStudioGridTemplateColumns` / `buildStudioGridTemplateRows` 导出签名与默认行为不变，确保已有单测完全兼容。

---

## 3. 顶栏渐进收拢设计（`DataSyncStudio.tsx`）

### 3.1 容器结构
- 顶栏 `<CardContent>` 从硬编码 3 列网格改为：
  `studio-topbar relative flex min-h-12 min-w-0 items-center justify-between gap-2 overflow-visible px-2.5 sm:px-3`
- 左侧（`flex min-w-0 flex-1 items-center gap-1.5 sm:gap-2`）：
  1. 左栏开关按钮（`shrink-0`）
  2. 文件名区（`flex min-w-0 items-center gap-1.5`）：
     - 仅渲染当前打开文件名 `activeTaskBreadcrumbs[activeTaskBreadcrumbs.length - 1]`（`min-w-0 truncate font-semibold text-foreground`），`title` 放完整路径 `activeTaskBreadcrumbs.join(' / ')`（不再在顶栏重复渲染全部父目录段，编辑器标签下方已有完整面包屑）。
     - 未保存圆点（`shrink-0`）。
  3. 仅在 `editor.id` 存在时展示元操作（`flex shrink-0 items-center gap-1.5`）：
     - 版本+发布状态微胶囊：`topbarTier !== 'compact'` 时渲染。
     - 只读 `Eye` 徽章：`topbarTier === 'wide'` 时渲染（中/窄屏下编辑区只读横幅与共享浮层已清楚表达只读态）。
     - `StudioSharePopover`：全档位始终渲染。
- 右侧（`flex shrink-0 items-center gap-1.5 sm:gap-2`）：
  1. **快速搜索（零空闲占位）**：
     - 外层锚点：`relative z-30 h-9 w-9 shrink-0`（在 flex 流中仅占 `36px`！）
     - 内层定位壳：`pointer-events-none absolute right-0 top-0 w-[min(280px,calc(100vw-6rem))]`
     - `<ExpandingSearch anchor='end' expandedWidth={280} ... />`
     - 原理解析：`expanding-search.module.css` 中 `.root` 自带 `pointer-events: none`、`.shell` 为 `position: absolute; right: 0; pointer-events: auto`，因此收起时左侧点击完全穿透，展开时向左平滑浮出 `280px` 盖在留白/文件名上方，绝不推挤右侧主按钮与右栏开关！
  2. 集群状态指示：仅 `topbarTier === 'wide'` 渲染（`max-w-[10rem] truncate`），中/窄屏可在右侧设置面板查看和切换。
  3. 探查按钮组 `ExpandingButtonGroup`（测试连通 / DAG / 预览）：
     - `topbarTier !== 'compact'`（`>= 900px`）时渲染；
     - `defaultExpanded={topbarTier === 'wide' ? 'test' : null}`（宽屏默认展开「测试连通」，中屏默认全收图标节省 ~60px；若 `defaultExpanded={null}` 在 Arc 中会回落第一项，先核对 `expanding-button-group.tsx` 行为，如是则在 `wide` 用 `'test'`，中屏同样安全适配）。
     - `topbarTier === 'compact'`（`< 900px`）时，将「测试连通 / DAG / 预览」并入 `ButtonGroup.menu.items`（带图标、禁用态与原回调，放在 Savepoint 项之前），顶栏不渲染 `ExpandingButtonGroup`。
  4. 保存失败重试徽章 `DraftSaveStatusBadge`（仅失败态出现）。
  5. 主操作 `ButtonGroup`（保存 / 发布 / 运行 / 停止 + 末尾菜单）：
     - `topbarTier === 'wide'`：`iconOnly: false`（保留全部文字标签，1280px E2E 零变化）。
     - `topbarTier !== 'wide'`（`< 1200px`）：4 项均设 `iconOnly: true`（Arc `ButtonGroup` 原生支持，自动保留 `aria-label` 与 Tooltip，立省 ~180px）。
  6. 右栏开关按钮（`shrink-0`，全档位始终可见）。

### 3.2 各宽度实测预算核验
- **1280px**（container ≈ 1256px，`wide`）：左 ~260px + 搜索 36px + 集群 ~120px + 探查 ~150px + 带文主动作 ~310px + 右钮 36px ≈ **950px**（留白 >300px）。
- **1024px**（container ≈ 1000px，`medium`）：左 ~220px + 搜索 36px + 探查 ~140px + 图标主动作 ~190px + 右钮 36px ≈ **650px**（留白 >350px）。
- **768px**（container ≈ 748px，`compact`）：左 ~180px + 搜索 36px + 图标主动作(含探查进菜单) ~190px + 右钮 36px ≈ **470px**（留白 >270px）。
- **480px**（container ≈ 460px，`compact`）：左(含截断文件名+共享) ~140px + 搜索 36px + 图标主动作 ~190px + 右钮 36px ≈ **420px**（零溢出！）。

---

## 4. 三栏联动与窄屏浮层抽屉（`DataSyncStudio.tsx`）

### 4.1 容器测量（`ResizeObserver`）
- 在 `DataSyncStudio` 根节点挂 `studioRootRef`（测宽度驱动 `viewportMode` / `topbarTier`），在三栏主网格容器挂 `studioWorkspaceRef`（测宽与高驱动有效侧栏宽与控制台高）。
- 初始 state 为 `{ width: 0, height: 0 }`（SSR 走 `'wide'` 安全默认，挂载后 `ResizeObserver` 同步更新，零 hydration warning）。

### 4.2 断点跨越与侧栏互斥规则
- 记录上一次 `viewportMode`（`prevViewportModeRef`）：
  - `wide → medium`：若当前左右侧栏同时展开，自动暂收右栏（`setIsRightSidebarCollapsed(true)`），保留左资源树与编辑器。
  - `* → compact`：若任一侧栏展开，自动收起两侧栏，避免缩屏后抽屉突然遮满编辑区。
  - 注意：**因窗口缩窄触发的自动收起不应覆写 `localStorage`**（维护独立开关或仅在用户显式点击/拖拽时持久化 `writeStudioLayoutPrefs`，避免用户拉窄一次窗口后永久弄丢桌面端布局偏好！）。
- 点击顶栏左/右开关按钮：
  - 在 `'wide'` 模式：左右完全独立开关，并持久化偏好。
  - 在 `'medium'` 与 `'compact'` 模式：打开左栏时自动收起右栏，打开右栏时自动收起左栏（互斥）。
- 窄屏（`'compact'`）下选中文件（`handleSelectNode` 打开 `node_type === 'file'` 时）：自动关闭左抽屉，让用户立刻看到代码。

### 4.3 窄屏浮层抽屉形态（不卸载子树，零状态丢失）
- 在 `overlaySidebars === true` 时：
  - 当 `!isLeftSidebarCollapsed || !isRightSidebarCollapsed`，在主网格容器内渲染半透明遮罩层：
    ```tsx
    <div
      aria-hidden='true'
      data-testid='studio-drawer-backdrop'
      className='absolute inset-0 z-20 bg-background/50 backdrop-blur-[1px] transition-opacity'
      onClick={() => {
        setIsLeftSidebarCollapsed(true);
        setIsRightSidebarCollapsed(true);
      }}
    />
    ```
  - 监听 `Escape` 键关闭当前打开的抽屉（不干扰 Monaco / Dialog 内的 Esc：仅当抽屉打开且无更高层 Dialog 打开时关闭）。
  - 左栏 `<Card data-studio-left-sidebar=''>` 在 `overlaySidebars` 时切为：
    `absolute inset-y-0 left-0 z-30 w-[min(320px,85vw)] shadow-xl transition-transform duration-200`（收起态 `-translate-x-full pointer-events-none opacity-0`，展开态 `translate-x-0 opacity-100`）。
  - 右栏 `<StudioSidebarShell data-studio-right-sidebar=''>` 在 `overlaySidebars` 时切为：
    `absolute inset-y-0 right-0 z-30 w-[min(380px,90vw)] shadow-xl transition-transform duration-200`（收起态 `translate-x-full pointer-events-none opacity-0`，展开态 `translate-x-0 opacity-100`）。
  - 两个垂直 `StudioSplitHandle` 仅在 `!overlaySidebars && !docked*Collapsed` 时渲染；水平控制台 `StudioSplitHandle` 的 `left` / `right` / `bottom` 均绑定到 `effectiveLayout` 计算出的停靠值（`overlaySidebars` 时 `left: 0, right: 0`）。

---

## 5. 控制台与内部细条微调

1. **只读横幅**（`DataSyncStudio.tsx:5278`）：改为 `flex flex-wrap items-center justify-between gap-2 border-b border-border/60 bg-muted/40 px-3 py-1.5 text-xs text-muted-foreground shrink-0`，内层文案 `div` 加 `min-w-0 flex-1`、徽章加 `shrink-0`。
2. **`actionPending` 提示条**（`DataSyncStudio.tsx:4996`）：加 `flex-wrap min-w-0`。
3. **控制台顶栏**（`DataSyncStudio.tsx:5595`）：外层 `SegmentedControl` 包一层 `min-w-0 flex-1 overflow-x-auto scrollbar-none`，右侧操作按钮组加 `shrink-0`。
4. **`ConsolePanel.tsx`**：
   - `MixedLogModeBanner` / `JobLogsEmptyBanner`：移除 `min-h-[180px]`，改用 `min-h-0 overflow-auto p-4`。
   - 日志头两行（lines 218 / 256）：改为 `flex flex-wrap items-center justify-between gap-2` 或在窄宽下允许平滑折行，避免 `SegmentedControl` + 展开按钮被裁切。

---

## 6. 回滚策略

- 所有改动仅限前端布局层（`layout.tsx` 仅分支处理 `/workbench`、纯函数集中于 `sync-studio-utils.ts`、`DataSyncStudio.tsx` 与 `ConsolePanel.tsx` 视图层），无后端/数据库/本地存储 schema 破坏；出现问题时单个 revert 即可无损恢复。
