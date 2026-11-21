# 实施计划：列表与常驻详情栏联动动效（克制版）

开工前先读 `.trellis/spec/frontend/index.md` 和 `ui-conventions.md`，并在浏览器打开 `research/prototype.html` 对照效果。
本机内存只有 8G，不跑 `next dev` / `next build`，验证以 CI 和 Egolite 录屏为准。

## Phase 0 · 结构核对（不改代码）

- [ ] 逐个确认 7 个列表：
  - 行是 `.tableScroll` 的直接子元素，选中时加 `layout.rowActive`；
  - 表头 `.rowHead` 和 `.tableScroll` 在同一个 `.listCol` 内；
  - 工具栏用的是 `layout.toolbar`；
  - 行内名称元素是哪个、在头部对应 `title` 还是 `meta`（核对 design §4.3）。
- [ ] 找出所有依赖 `detail-rail` 关闭后立即消失、或依赖 `.rowActive` 底色的单测和 E2E，列在 PR 描述里。
- 结构不符合的列表，只调整包裹层让行成为直接子元素，不改行为。

## Phase 1 · 公共层

- [ ] `TableCard.tsx`：根节点加 `data-slot='table-card'`，分页底栏加 `data-slot='table-card-footer'`。
- [ ] `list-detail.module.css`：按 design §3.2、§3.3 写联动外观；按 §3.4 写减少动态效果的降级。
  - 条件选择器统一写成"`.split` 内常驻栏 `[data-state='open']` 且存在 `.rowActive`"，集中放在一个注释分区里，注释双语；
  - 底层色用局部变量，浅色、深色各一份。
- [ ] `DetailHost.tsx`：
  - 常驻栏输出 `data-state='open' | 'closed'`；
  - 关闭后保持挂载，`transitionend` 后卸载，600ms 兜底；关闭途中重新打开时取消卸载；
  - 栏宽改为 CSS 变量（440 / 520px），宽度动画期间栏内容固定为该宽度。
- [ ] 单测：
  - 关闭后常驻栏先保持挂载，动画结束后卸载；
  - 关闭途中重新打开不会卸载；
  - 窄屏仍走抽屉，不输出常驻栏。

## Phase 2 · 名称飞入

- [ ] `DetailHost` 加 `flyOrigin`、`nameSlot` 两个可选属性和飞入逻辑（design §4.2），头部落点加 `data-detail-name-target`。
- [ ] 把"该不该飞"抽成纯函数，单测覆盖以下情况都返回不飞：
  - 没有来源；
  - 窄屏；
  - 系统开启减少动态效果；
  - 没有配置 `nameSlot`；
  - 来源文字和落点文字不一致。
- [ ] 6 个列表接入（巡检不接入）：行内名称加 `data-detail-name`，行点击和回车打开时写入 `flyOrigin`，向 `DetailHost` 传 `nameSlot`。
- [ ] 确认 J/K、↑↓、深链恢复都不写入 `flyOrigin`。

## Phase 3 · 规范与回归

- [ ] `ui-conventions.md` 的"列表 + 常驻详情分栏"条目补充：联动外观由公共样式自动生效；行必须是 `.tableScroll` 的直接子元素并用 `.rowActive`；飞入的接入方式；降级规则。
- [ ] 更新 Phase 0 列出的测试。
- [ ] tsc、vitest（layout、monitoring、diagnostics）、eslint 通过；CI 里现有的诊断与告警 E2E 通过。
- [ ] Egolite 录屏，7 个列表各录一遍，浅色、深色各一次：
  - 点击打开（含飞入）→ J/K 切换两次 → 点另一行 → Esc 关闭；
  - 选中第一行、选中最后一行、打开后翻页；
  - 告警列表打开详情后等一次轮询刷新；
  - 窗口缩到 1080px 以下，确认是抽屉且没有联动效果。
- [ ] PR 描述附上录屏，以及和 `research/prototype.html` 的对照截图。

## 回滚

改动集中在 `list-detail.module.css` 的联动分区、`DetailHost` 和 `TableCard` 的 `data-slot`。
出问题时删掉联动样式分区，外观即回到现状；飞入逻辑在不传 `flyOrigin` 时不执行。
