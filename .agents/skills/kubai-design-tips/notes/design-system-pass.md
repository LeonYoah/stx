# Note: Design system pass

A note from Kubai (vibedesign.tips), outside the numbered tips, with no demo. Source: https://x.com/Kubai087/status/2123266417671958600, https://x.com/Kubai087/status/2121615225816494303, https://x.com/Kubai087/status/2123279382664077343

Written in the user's voice: I, me and my mean the user. Where it says to ask first, stop and ask the user before going on.

Check my product as one design system and make it consistent. Much of what makes a vibe-coded product feel generated is drift: the same colour written five ways, radii and spacing picked one screen at a time, three components doing one job, and text that explains what the screen already shows. A design system has layers, each built on the one below: colours, then styles, then components, then patterns, then the decisions about when to use what. I don't need a full design system; I need one stable design language. Keep my look: this pass makes values consistent, it does not restyle.

First look at my product (section 0) and report, layer by layer, what you found and what you would change, the largest gain for the least work first. Then ask me which layers to take on, and do one layer at a time.

0. Start from what exists
- Where colours, radii, shadows, borders, spacing and font sizes are set: tokens, CSS variables, a theme file, Tailwind config, or values written inline. Count the hard-coded values and the near-duplicates (#111 and #121212; 7px, 8px and 9px for one role).
- The component library, if any, and the components the product adds: which ones do the same job, which screens bypass them.
- Light and dark mode: whether both exist and come from the same tokens.

1. Colours
- One palette for light and one for dark: greys, functional colours (success, warning, danger, info) and theme colours, kept as variables. Components use roles (text, muted text, surface, border, accent), never raw values.
- Merge near-duplicates into the nearest token. Add a token only for a role that has none.

2. Styles
- Shadows, radii and borders as tokens, each with a few strengths (subtle, default, strong) and named combinations for surfaces such as cards, popovers, dialogs and inputs. A toast, a dialog and a form field then each map to one surface style.
- Spacing on a 4px grid. Spacing follows nesting: the gap between items inside a group is smaller than the gap between groups, and a box that holds content gets equal padding on all sides where its role calls for it.
- Radii from variables, nested by inner = outer - padding, with continuous corners where they are supported and cheap to render (see tip 004, https://vibedesign.tips/004-continuous-corners/).

3. Components
- One base component per job, in the sizes and strengths the product needs (buttons, inputs, switches, lists, toasts, dialogs), each mapped to the tokens above. Compare against a mature component library to find gaps; if the product follows none, the open-source variants of shadcn/ui are a good reference.
- Choose each component by what it means, not by how it looks: write a short Do / Don't for the ones that are easy to confuse, and fix the places that use the wrong one. Tips 009 (overlays), 016 (toasts and notifications), 017 (loading states) and 020 (choice controls) cover the common cases.

4. Patterns
- A pattern is the smallest unit that completes a user task: a sign-in form, a profile form, a filtered table, a settings section. Where the same task is built twice in different ways, make one pattern and reuse it.

5. Copy
- Decide what each screen is for. Keep its main title and the labels and values people need; remove text that repeats what the interface already shows.
- Write explanations in plain written language, not marketing or chat. Move help people need rarely into a tooltip or an inline alert instead of showing it all the time.

Don't change the brand colours, the fonts or the layout, and don't add new visual effects. When a layer is done, list the tokens added and the values merged as "old -> token".
