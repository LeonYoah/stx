# Note: Popover origin

A note from Kubai (vibedesign.tips), outside the numbered tips, with no demo. Source: https://x.com/Kubai087/status/2123076115464098210

Written in the user's voice: I, me and my mean the user. Where it says to ask first, stop and ask the user before going on.

Make the menus and popovers in my product grow out of the place that opened them. Many dropdowns look wrong for one reason: they scale from their own centre, so they seem to appear from nowhere instead of coming out of the button. Scale them from the point where they meet their trigger. Keep my components, durations and curves; change the origin only. If an overlay has no scale animation, leave it as it is unless I ask.

0. Start from what exists
- Every overlay that opens from a trigger: dropdown and select menus, context menus, popovers, combobox lists, tooltips, popconfirms, menus behind icon buttons, submenus.
- For each, how it animates in and out (CSS, Framer Motion, the component library) and what places it (Floating UI, Popper, Radix, Base UI, CSS anchor positioning, hand-written code).

1. Where it grows from
- The origin is the point of the overlay nearest its trigger. A menu below its button grows from its top edge, on the side it is aligned to: top left when aligned to the button's start, top centre when centred under it. A menu above its button grows from its bottom edge. A submenu grows from the edge facing its parent item, at that item's height.
- The origin follows the side the overlay really opens on. When it flips above the button or shifts to stay on screen, the origin moves with it. Use what the library gives: Radix sets --radix-<component>-content-transform-origin, Base UI sets --transform-origin, Floating UI returns the final placement; with CSS anchor positioning, set the origin in each position-try fallback.
- A context menu grows from the pointer.
- Use the centre only when the overlay is centred on its trigger and covers it.
- Native apps follow the same rule: SwiftUI .scaleEffect(anchor:), Android pivotX and pivotY, Compose transformOrigin.

2. The motion
- Closing shrinks back towards the same origin.
- Keep my durations and curves. Where an overlay scales but has none set, use scale 0.96 to 1 with opacity 0 to 1, about 150ms ease-out to open and 100ms to close.
- With reduced motion ("reduce"), fade without scaling.

When done, list each overlay as "overlay: old origin -> new origin".
