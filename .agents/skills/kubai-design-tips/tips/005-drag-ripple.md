# 005 Drag to reorder

Vibe Design Tips, issue 005. Demo: https://vibedesign.tips/005-drag-ripple/

Written in the user's voice: I, me and my mean the user. Where it says to ask first, stop and ask the user before going on.

Make drag-to-reorder in my product feel like this. Apply it to the lists,
rows and docks I name, and reuse any drag helper the project already has.

1. Cursor: a pointing hand over anything draggable, a grabbing hand while
   dragging.
2. Lift: the picked item follows the pointer, raised off the list so it
   never hides where it will land: above a horizontal row, or pulled aside
   and shrunk to about 90% in a vertical list. Its old slot closes up.
3. Drop hint: a faint, thin line in the gap where it will land (vertical in
   a row, horizontal in a list), about 15% of the text color. It grows from
   its middle when it appears and glides to each new gap. No line over the
   item's own slot.
4. Ripple: items on both sides of the line lean away from it. Counting n
   from the line (0 = the item touching it), item n moves
   A x 0.7^n (A is about 10px for icons, 8px for cards)
   and starts 24ms x n later, on a quick spring that barely overshoots and
   settles in about 0.25s. Nearer items move sooner and further, so the
   push spreads out like a ripple.
5. Drop: the item takes the gap and every item springs into its new place.
   Escape or dropping outside the list cancels, and everything springs back.
6. Give each motion its own curve (hand-like for the pointer, a small pop
   for the lift, springs for settling) instead of one ease-out everywhere.

Move things with transforms, not layout. Let the keyboard reorder too, and
with reduced motion keep only the line.
