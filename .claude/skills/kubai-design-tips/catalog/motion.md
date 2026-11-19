# Motion and transitions

## 005 Drag to reorder (rule)

- Applies: Kanban boards, to-do lists, layer panels, tab or tag reordering.
- Signals: dnd-kit, @hello-pangea/dnd, react-beautiful-dnd, SortableJS, draggable=, onDragStart, SwiftUI .onMove, Compose reorderable; no drop indicator; items jumping into place.
- Skip: No reordering in the product.
- Demo: https://vibedesign.tips/005-drag-ripple/
- Text: tips/005-drag-ripple.md (1.7k characters)

## 011 Easing curves (rule)

- Applies: Any product with transitions or animations.
- Signals: transition: with no curve, or ease / linear on movement; ease-in on things that appear; @keyframes; Framer Motion, GSAP or Web Animations durations; no motion tokens.
- Skip: Products with no motion at all.
- Demo: https://vibedesign.tips/011-easing-curves/
- Text: tips/011-easing-curves.md (4.9k characters)

## 015 UI continuity (rule)

- Applies: Lists that open details, cards that expand, mini players, search, tabs, buttons that open menus, on phone or desktop.
- Signals: Route changes with hard cuts; modals or sheets opening from an unrelated edge; no View Transitions, layoutId, matchedGeometryEffect or Hero; whole-screen cross-fades.
- Skip: Single-screen tools with no navigation.
- Demo: https://vibedesign.tips/015-ui-continuity/
- Text: tips/015-ui-continuity.md (10.2k characters)

## 006 Book cards (effect)

- Applies: Card grids of documents, templates, books, notes or portfolio items, used with a pointer.
- Signals: Card grid components for docs or templates; an existing hover lift on cards.
- Skip: Touch-only products; dense data lists.
- Demo: https://vibedesign.tips/006-book-cards/
- Text: tips/006-book-cards.md (9.7k characters)

## 014 Card delete (effect)

- Applies: Cards or list items with their own delete action.
- Signals: onDelete or remove handlers; trash icons on items; an exit that only fades, or none.
- Skip: Tables with bulk delete only; serious contexts such as finance or medical records, unless the user asks.
- Demo: https://vibedesign.tips/014-card-delete/
- Text: tips/014-card-delete.md (22.1k characters)

## 019 Scroll cards (effect)

- Applies: Long pages read from top to bottom: landing and product pages, portfolios and case studies, launches, editorial stories, label, event and exhibition pages.
- Signals: A long marketing or product page; a features or steps section with three to five items; a horizontal carousel of work, products or albums; testimonials or picks shown one at a time; a large hero image or video; existing reveal-on-scroll, parallax, Swiper or GSAP ScrollTrigger code.
- Skip: Apps and dashboards people work in, documentation, forms, checkout, and any page people scan or search rather than read through.
- Demo: https://vibedesign.tips/019-scroll-cards/
- Text: tips/019-scroll-cards.md (13.1k characters)
