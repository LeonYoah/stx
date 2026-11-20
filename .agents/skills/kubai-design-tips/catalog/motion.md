# Motion and transitions

## 005 Drag to reorder (rule)

- Also called: 带避让动效的拖拽排序, 拖拽排序
- Applies: Kanban boards, to-do lists, layer panels, tab or tag reordering.
- Signals: dnd-kit, @hello-pangea/dnd, react-beautiful-dnd, SortableJS, draggable=, onDragStart, SwiftUI .onMove, Compose reorderable; no drop indicator; items jumping into place.
- Skip: No reordering in the product.
- Demo: https://vibedesign.tips/005-drag-ripple/
- Text: tips/005-drag-ripple.md (1.7k characters)

## 011 Easing curves (rule)

- Also called: 按场景选择缓动曲线, 缓动曲线
- Applies: Any product with transitions or animations.
- Signals: transition: with no curve, or ease / linear on movement; ease-in on things that appear; @keyframes; Framer Motion, GSAP or Web Animations durations; no motion tokens.
- Skip: Products with no motion at all.
- Demo: https://vibedesign.tips/011-easing-curves/
- Text: tips/011-easing-curves.md (4.9k characters)

## 015 UI continuity (rule)

- Also called: 界面连续性诊断与改造, 界面连续性, 共享元素转场
- Applies: Lists that open details, cards that expand, mini players, search, tabs, buttons that open menus, on phone or desktop.
- Signals: Route changes with hard cuts; modals or sheets opening from an unrelated edge; no View Transitions, layoutId, matchedGeometryEffect or Hero; whole-screen cross-fades.
- Skip: Single-screen tools with no navigation.
- Demo: https://vibedesign.tips/015-ui-continuity/
- Text: tips/015-ui-continuity.md (10.2k characters)

## Note: Popover origin (rule)

- Also called: 弹出层从触发处展开, 弹出层锚点, 下拉菜单锚点, transform-origin
- Applies: Dropdown and select menus, context menus, popovers, combobox lists, tooltips and popconfirms that open with a scale animation.
- Signals: Menus or popovers that animate scale (scale(0.95), zoom-in, scaleEffect) with no transform-origin or with transform-origin: center; component library popovers that ignore the origin variable they provide (--radix-*-content-transform-origin, --transform-origin); menus that flip above their button while still growing from the top.
- Skip: Overlays that only fade or do not animate; centred dialogs and full-screen sheets, which have no trigger to grow from.
- Source: https://x.com/Kubai087/status/2123076115464098210
- Text: notes/popover-origin.md (2.4k characters)

## 006 Book cards (effect)

- Also called: 悬停时像书一样翻开的卡片, 书本卡片
- Applies: Card grids of documents, templates, books, notes or portfolio items, used with a pointer.
- Signals: Card grid components for docs or templates; an existing hover lift on cards.
- Skip: Touch-only products; dense data lists.
- Demo: https://vibedesign.tips/006-book-cards/
- Text: tips/006-book-cards.md (9.7k characters)

## 014 Card delete (effect)

- Also called: 卡片删除的四种消失效果, 卡片删除, 燃烧, 撕贴纸
- Applies: Cards or list items with their own delete action.
- Signals: onDelete or remove handlers; trash icons on items; an exit that only fades, or none.
- Skip: Tables with bulk delete only; serious contexts such as finance or medical records, unless the user asks.
- Demo: https://vibedesign.tips/014-card-delete/
- Text: tips/014-card-delete.md (22.1k characters)

## 019 Scroll cards (effect)

- Also called: 滚动驱动的五种卡片交互, 滚动卡片, 层叠卡片, 卡组飞出, 横向展廊, 封面轮播, 图片展开
- Applies: Long pages read from top to bottom: landing and product pages, portfolios and case studies, launches, editorial stories, label, event and exhibition pages.
- Signals: A long marketing or product page; a features or steps section with three to five items; a horizontal carousel of work, products or albums; testimonials or picks shown one at a time; a large hero image or video; existing reveal-on-scroll, parallax, Swiper or GSAP ScrollTrigger code.
- Skip: Apps and dashboards people work in, documentation, forms, checkout, and any page people scan or search rather than read through.
- Demo: https://vibedesign.tips/019-scroll-cards/
- Text: tips/019-scroll-cards.md (13.1k characters)
