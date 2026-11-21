# Components and surfaces

## 002 Cascade menu (rule)

- Also called: 级联菜单的安全区域与延时, 级联菜单, 安全三角区
- Applies: Menus with submenus in admin panels, editors and settings.
- Signals: SubMenu, DropdownMenu.Sub or other submenu components; children opened on mouseenter with no delay; submenus that close on a diagonal move toward them.
- Skip: No nested menus; touch-only products.
- Demo: https://vibedesign.tips/002-cascade-menu/
- Text: tips/002-cascade-menu.md (5.1k characters)

## 004 Continuous corners (rule)

- Also called: 递进式圆角与连续曲率圆角, 连续曲率圆角, 嵌套圆角
- Applies: Any UI with cards, panels, inputs and buttons nested inside each other.
- Signals: Hard-coded border-radius values; the same radius on a container and the box inside it; no radius tokens; rounded-* repeated on parent and child.
- Skip: None: every product with nested boxes.
- Demo: https://vibedesign.tips/004-continuous-corners/
- Text: tips/004-continuous-corners.md (1.4k characters)

## 020 Choice controls (rule)

- Also called: 选择控件的诊断与改造, 选择控件, 开关, 下拉选择
- Applies: Any screen where people pick something, turn something on or off, or run an action from a list: settings, forms and dialogs, surveys and onboarding, filters, toolbars and row menus.
- Signals: <select>, Select, Combobox, DropdownMenu, Menu, Switch, Toggle, Checkbox, RadioGroup, SegmentedControl, Tabs; a switch inside a form with a Save button; On / Off as two radios; a select with 2 or 3 options; a dropdown menu used to pick a value; a long list of options without search; radio circles inside a select's list.
- Skip: None: nearly every product has choice controls.
- Demo: https://vibedesign.tips/020-choice-controls/
- Text: tips/020-choice-controls.md (8.7k characters)

## Note: Design system pass (rule)

- Also called: 去 AI 味的设计系统检查, 设计系统, 去 AI 味, AI Slop, Design Token, 间距体系, 精简文案
- Applies: Any product with more than a few screens, above all one built quickly with AI.
- Signals: Hex colours and px values written inline across many files; no colour or spacing tokens, or tokens that screens bypass; the same role with several values (radii 6, 8 and 10 on cards; gaps of 5, 7 or 13px); several components doing one job; dark mode patched per screen; helper text under every field, or marketing tone in interface copy.
- Skip: Throwaway prototypes. In a product with a maintained design system, check only for values and components that bypass it.
- Source: https://x.com/Kubai087/status/2123266417671958600, https://x.com/Kubai087/status/2121615225816494303, https://x.com/Kubai087/status/2123279382664077343
- Text: notes/design-system-pass.md (4.0k characters)

## 001 Skeuomorphic buttons (effect)

- Also called: 有按压反馈的拟物按钮, 拟物按钮
- Applies: Admin and tool products that want a calm, tactile button set.
- Signals: A Button component with variants; flat buttons with no pressed state.
- Skip: Products with an established brand button style, unless the user asks.
- Demo: https://vibedesign.tips/001-skeuomorphic-buttons/
- Text: tips/001-skeuomorphic-buttons.md (3.5k characters)

## 010 Liquid glass (effect)

- Also called: 液态玻璃材质, 液态玻璃, 毛玻璃
- Applies: Cards, players, toolbars and menus over images, wallpapers, video or maps.
- Signals: backdrop-filter or blur( surfaces; overlays on media; players; map controls.
- Skip: Surfaces over a plain background: glass over a flat color looks gray.
- Demo: https://vibedesign.tips/010-liquid-glass/
- Text: tips/010-liquid-glass.md (7.2k characters)

## 012 Message bubbles (effect)

- Also called: 消息气泡的材质与动效, 消息气泡
- Applies: Chat, support and AI conversation screens.
- Signals: A message list with user and assistant roles; chat bubble components; a composer.
- Skip: No chat in the product.
- Demo: https://vibedesign.tips/012-message-bubbles/
- Text: tips/012-message-bubbles.md (16.7k characters)
