# Note: Empty states

A note from Kubai (vibedesign.tips), outside the numbered tips, with no demo. Source: https://x.com/Kubai087/status/2113849121299894727

Written in the user's voice: I, me and my mean the user. Where it says to ask first, stop and ask the user before going on.

Give the empty states in my product pictures that show what will be there. A generic illustration (a cloud, a rocket, a sad face) says nothing about the feature; a small, simplified view of the real interface tells people at a glance what this place is for and what to do next. Draw each one as a miniature of my own product's UI, not as an illustration.

0. Start from what exists
- Every empty state: no data yet, a first run, no search or filter results, an emptied list or folder, a cleared inbox. Note what each shows now (a picture, an emoji, a line of text, nothing) and the title, text and action around it.
- My icon set, accent colour, surface and border tokens, and how light and dark mode are switched.
Ask me which empty states to change and whether to replace pictures I already have. Keep the title, text and action around each picture unless I ask.

1. What to draw
- The view as it looks in the real product at this moment, simplified to scale: an empty folder is a folder card with empty slots; an empty search is the search field over an empty list. No abstract symbols.
- One scene, 2 or 3 objects. Their bounding box is centred at (80, 60), with at least 14 of space on every side.
- No text, people, illustrated gradient backgrounds, or translucent objects laid over each other.

2. The defaults
These values come from a tested set. Where my product has a token for the same role (surface, border, accent, icon colour), use mine.
- Canvas: viewBox 0 0 160 120, a base with radius 18 filling it. Base #fafafa, with 45° hatching on top (line width 1, spacing 7, rgba(15,23,42,0.05)) and a 1px inner border rgba(15,23,42,0.07).
- Objects: white opaque cards or windows, radius 10 to 13, 1px border rgba(47,42,34,0.08), a shadow offset 5 down with blur 4.5 in rgb(47,42,34) at opacity 0.13. A browser window gets an 18-high tab bar #eef1f5 with the active tab white, radius 5.
- Text becomes placeholder bars with fully round ends: body 4 high #e5e9ef, titles 5 high #cdd4de. Empty slots #f1f3f6; cards showing behind others #f3f3f4.
- Icons from my icon set, in its line style (Remix Icon line if I have none): #a3abb7, secondary actions #6b7280.
- At most one accent element, and only for the next step (add, retry, create); with no next step, no accent. Accent #3462ad. A key or primary button uses a slate gradient #404859 to #273042 with a white icon.
- Dark mode: base #26262b, hatching rgba(255,255,255,0.035), surfaces #323238, tab bar #2a2a2f, placeholder bars #3f3f47 and #52525b, icons #7d828c, accent #7ea6ea, black shadows at opacity 0.36.

3. How to build it
- One self-contained inline SVG per empty state. No external resources, no <image> and no <use>.
- Colours as CSS variables in a <style> inside the SVG, scoped by the SVG's own id. The root takes data-theme="light", "dark" or "auto"; auto follows prefers-color-scheme. Wire it to the way my product switches themes.
- Build shadows from feGaussianBlur, feOffset, feFlood, feComposite and feMerge, not feDropShadow (some HTML parsers lower-case its name and break it).
- Give every id in an SVG the same random prefix, so several on one page never clash.
- Optional: the accent element pops in once with a small scale, with transform-box: fill-box, only under prefers-reduced-motion: no-preference.

When done, list each empty state with what it showed before and what it shows now.
