# 004 Continuous corners

Vibe Design Tips, issue 004. Demo: https://vibedesign.tips/004-continuous-corners/

Written in the user's voice: I, me and my mean the user. Where it says to ask first, stop and ask the user before going on.

Make the corner radii in this UI consistent, then switch them to continuous
corners. Work on the screen or components I name; ask me if I have not
named them. If the project has radius tokens, change those instead of
hard-coding values.

1. Inner radius = outer radius - padding (r = R - p)
- Go from the outside in: dialog or card -> panels, inputs, buttons ->
  chips and tags. A box p inside a container with radius R gets R - p,
  so both corners share one center. Example: dialog 32, padding 16 ->
  panels and buttons 16; a chip 8 inside a 16 panel -> 8.
- Boxes on the same level share one radius. Never go below 4.
- Pills and circles stay fully round.
- Keep every padding, gap and radius on a 4px grid.

2. Continuous corners
- Keep border-radius and add the curve as progressive enhancement:
    @supports (corner-shape: superellipse(1.25)) {
      :where(*) { corner-shape: superellipse(1.25); }
    }
  Set corner-shape: round on pills and circles.
- Chrome and Edge 139+ render it; Safari and Firefox skip the block and
  show the plain border-radius.

When done, list every radius you changed as "element: old -> new".
