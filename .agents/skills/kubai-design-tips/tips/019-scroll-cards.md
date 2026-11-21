# 019 Scroll cards

Vibe Design Tips, issue 019. Demo: https://vibedesign.tips/019-scroll-cards/

Written in the user's voice: I, me and my mean the user. Where it says to ask first, stop and ask the user before going on.

Rework a long page that already exists in my product so its cards move with scrolling. This changes my product, not a new page or a demo: keep my content, components, tokens and copy, find the sections that deserve it, and give each one of five scroll-driven card interactions: stacked cards (each new card covers the last, which shrinks and dims), a card deck (the top card flies off to one side and the next rises), a pinned gallery (the section holds still while scrolling moves a row sideways), a cover flow (the middle cover is largest; the others shrink, turn away and dim), or an image expand (a picture grows from a frame to the whole screen, with words behind and in front of it rising at different speeds for depth). The scroll position drives every effect, so the reader sets the pace and scrolling back plays it in reverse. It must hold up on every screen, phones above all, and at every viewport height (section 8).

Every value here is a recommendation. Sections 2 to 8 are a tested set (Standard); section 10 gives presets. I choose; you build what I choose.

First look at my product (section 0). Then ask me these one at a time, each as a short list of options with key values and the recommended one marked, and wait for my answer:
1. Which sections get which interaction (section 1): at most one of each per page, with ordinary sections between them.
2. Length: Short, Standard (recommended) or Long (section 10).
3. Phones and short screens: Standard (recommended: on phones the stack, deck and image expand stay, sized to the viewport; the gallery and cover flow become native horizontal rows with snap points; below 560px of viewport height nothing pins), the same as desktop, or plain sections on all phones. Tell me which screen sizes my analytics or audience suggest, if you can find out.
4. Reduced motion: plain sections (recommended: lists, horizontal rows, a full-width picture), or keep the pinning and drop the motion.
5. Engine: CSS scroll-driven animations with a small script as fallback (recommended), or the scroll library my project already uses. Never smooth-scrolling or scroll-hijacking.
Then sum up my choices in one table, propose how you will build it on my platform (section 9), with cost and fallback, and wait for my go-ahead.

0. Start from what exists
- The page: its sections in order, their content, cards, images and videos, and anything fixed or sticky (header, banner, cookie bar) with its height per breakpoint.
- For each section, which interaction fits and which sections must stay as they are (anything people scan, compare or search).
- How the layout responds today: breakpoints, how it sizes to the viewport (vh, svh, dvh, JS reading innerHeight), what already breaks on short screens.
- How the page scrolls: window or inner container, smooth-scroll libraries, anchor links, existing scroll effects.
- Platform and versions, tokens for radius and motion, dark mode. Tell me in a few lines before asking.

1. Which interaction for which content
- Stacked cards: 3 to 5 parallel items worth a screen each (steps, features, plans, chapters).
- Card deck: 3 to 6 items seen one at a time (picks, testimonials, highlights).
- Pinned gallery: 5 to 12 items wider than the screen, to browse (work, products, releases).
- Cover flow: 5 to 9 items of the same shape, one in focus (albums, books, plans, people).
- Image expand: one strong picture or video that opens the next chapter.

2. The rule behind all five
- Each section has one progress p = clamp((scrollY - start) / distance, 0, 1), and every property is a function of p, never a timed animation that scroll merely starts. Stopping halfway leaves it halfway; scrolling back reverses it exactly. Never change scroll speed, snap or catch the wheel.
- Pinning is position: sticky inside a section taller than the screen; the extra height is the distance the effect plays over.
- Animate only transform, opacity, clip-path and filter. Text stays in the page and readable at every p.
- When a section is pinned, show where the reader is as quietly as possible: a thin bar or dots, never a label.

3. Stacked cards (demo: 1000 x 896 viewport)
- Cards 936 x 720 (all 12 columns, bled 24px so their 24px padding puts content back on the columns), corners 28px, 48px apart.
- Each sticks 16px lower than the last: top = 40px + i * 16px (add a fixed header's height). Newer cards cover older ones, leaving a 16px strip of each.
- A card recedes as later ones arrive: k = clamp((scrollY - when it stuck) / 752px, 0, cards after it); scale = 1 - k * 0.05 from its top center (0.85 under three), dark layer at k * 0.12.
- Inside a card, the name and one line sit on columns 0 to 5 and wrap there, never into the picture; the name starts 48px below the card's top, the line 24px under it.
- Give each card its own muted tint and picture (demo: stone, sage, clay, then one ink card). No shadows.
- After the last card sticks, hold 320px, then the stack leaves together.
- Optional opening: the section's title sized to span the grid exactly (not cropped), the first card covering its foot; it moves at half the scroll and fades out over the 280px before the first card sticks.

4. Card deck
- Five cards in a pile, 432 x 456 (six columns, centred) under the title; each holds a picture 8px in, then a name and one line 24px in, nothing else.
- Section 100vh + 1920px, pinned: 420px of scrolling throws each of the first four cards, then a 240px hold. The last card stays.
- Card i: t = clamp((scrollY - start) / 420 - i, 0, 1). It flies to one side, alternating: x = side * t * t * 1000px, y = t * (160px * t - 40px), rotate = side * t * 16deg about its bottom center. Hidden once thrown; scrolling back returns it on the same path.
- Cards below sit 18px lower and 0.05 smaller per place, dark layer 0.04 per place, at most three deep, rising a place as the top card goes.

5. Pinned gallery
- 5 to 12 cards in a row: demo eight, 280 wide (four columns) and 24px apart, each a white card with its picture 8px in, then name and artist. Title above, 48px clear of the row; a 2px bar below (track 6% black, fill 30%).
- Section 100vh + overflow (row width minus viewport width, 1520px in the demo); content sticky at top 0, 100vh tall.
- p = (scrollY - start) / overflow; the row moves translateX(-p * overflow), no easing: scrolling turned sideways. It lets go at the last item.
- Keep the pinned distance under about three screens; more items belong in a grid.

6. Cover flow
- 5 to 9 square covers, 432 (six columns, centred), under the title, dots below, no text on the covers.
- Section 100vh + 320px per step, pinned. r = clamp((scrollY - start) / 320, 0, count - 1); c = floor(r) + smoothstep(fract(r)), so each cover rests in the middle a moment.
- For cover i, d = i - c: translateX = d * 260px, rotateY = clamp(d, -1, 1) * -30deg, scale = max(0.6, 1 - |d| * 0.2), dark layer min(|d|, 3) * 0.15, nearest on top. Perspective 1400px. Hide beyond 3.5 places. Dots light by 1 - |i - c|.

7. Image expand
- Section 100vh + 1200px, pinned: 900px to grow, 300px to hold.
- At the start the title and a 584 x 352 frame (columns 2 to 9, 48px below it) are centred together, corners 8px.
- The picture always covers the viewport; clip-path: inset() shows the frame and opens to 0 with no corners, while the picture settles from scale 1.2 to 1, so it reads as growing. Both on e = ease(p), cubic-bezier(0.45, 0, 0.25, 1). The title fades at twice the speed and rises 60px.
- Depth: cut the main shape out of the picture as its own layer (demo: the hills), set a title behind it with its foot hidden by the shape, and one small line in front. As it opens, each settles from its own scale (picture and shape 1.2, title 1.25, line 1.4). Once full, the picture and shape stay still while the title rises at 0.25 of the scroll and the line at 0.5. Something behind something, moving at another speed, reads as perspective.
- Then the next section slides over it with 28px top corners that flatten over its last 120px, while the picture darkens to 35% black. A video works the same, muted and looping, only while on screen.

8. Screens and viewport heights
The demo's sizes are for a 1000 x 896 viewport. Scale them to the screen; never a fixed height that assumes a tall window.
- Size pinned sections with 100svh, not 100vh: on phones 100vh is the height with the toolbar hidden, so content runs under the toolbar. Never dvh for pinned content; it jumps as the toolbar moves.
- Measure on load, on orientation change and when the width changes. Ignore height-only resizes under about 120px (the toolbar showing and hiding); recomputing then makes cards jump mid-scroll.
- Fit each pinned section into one screen:
  - Stacked card height = min(720px, 100svh - sticky top - (cards - 1) * 16px - 32px); the distance between cards sticking follows it.
  - Deck: title + 48px + card fit in 100svh - 64px; the picture gives up height first.
  - Gallery card height = min(520px, 100svh - title - 48px - 96px).
  - Cover size = min(432px, 48svh, 70vw); the spread scales with it.
  - Image frame = min(584px, 100vw - 2 * margin) wide at 5:3; the insets follow from it.
- Scale every scroll distance by clamp(0.7, viewport height / 896, 1.2).
- Below 560px of height (a phone in landscape, a short window), nothing pins: lists, horizontal rows, a full-width picture.
- Type follows the width: section titles clamp(36px, 7vw, 64px), card names clamp(28px, 5vw, 36px).
- Full-bleed moments respect env(safe-area-inset-*). Subtract a fixed header from every fit and sticky top.

Layout and type (all sections)
- Lay everything on the column grid first. Every text box is as wide as its columns, so long names wrap instead of running into a picture.
- What a title introduces starts 48px below it; in a pinned section the title and content are centred in the viewport together; centred titles over centred content, left-aligned over left content.
- Keep the cards in focus: outside them one short title per section, no tags, side copy, counters or captions; inside them one structure: picture, name, one line.
- Two typefaces in four sizes (demo: Newsreader 64 and 36, Geist 18 and 16). Pull large serif lines left by their first letter's side bearing; above about 100px use a text optical size so serifs stay blunt.
- No dividers except between list rows, no drop shadows, the accent colour once at most. One picture style per group, and never the same picture twice on a page.
- Anchor links land at a section's start. Keyboard scrolling works like the wheel; items in pinned sections stay reachable with Tab.

Check
- Each section slow, fast and backwards: no jumps, flicker or gaps.
- Viewports 375x667, 390x844, 430x932 (toolbar shown and hidden), 844x390, 768x1024, 1280x720, 1440x900, 1920x1080, and a 600px-tall desktop window: nothing cut off, no card taller than the screen, nothing under a toolbar or fixed header.
- 200% text zoom, reduced motion, keyboard, light and dark; full frame rate on a mid-range phone; the values built match my table.

9. How to build it
Check these against my stack and versions.
- Web: position: sticky for pinning; scroll-driven animations (animation-timeline: view() or a named timeline, animation-range). Where unsupported, or where keyframes are not enough (the deck's per-card ranges, the cover flow's resting steps), one passive scroll listener reads scrollY once per frame, computes each section's p from offsets measured on resize, and writes it as a custom property on that section only.
- If the project already uses GSAP ScrollTrigger (scrub, pin) or Motion (useScroll, useTransform), use it with these values; never add one only for this.
- iOS (SwiftUI): .scrollTransition or .visualEffect on the view's frame in the scroll view (onScrollGeometryChange on iOS 18), scaleEffect, rotation3DEffect, offset, clipShape; gallery and cover flow as horizontal ScrollViews with .scrollTargetBehavior(.viewAligned).
- Android (Compose): LazyListState for p, Modifier.graphicsLayer for scale, rotationY, alpha, translation and clip; LazyRow with snapping or HorizontalPager.
- Flutter: CustomScrollView with slivers, a ScrollController for p, Transform and ClipRRect; PageView for gallery and cover flow.

10. Presets
- Length: Short (stack 560px, deck 300px, gallery 1.25px sideways per 1px, cover step 240px, picture 600 + 200px), Standard (752, 420, 1:1, 320, 900 + 300, recommended), Long (900, 560, 1:1 with 200px held at each end, 420, 1200 + 400).
- Stack depth: Flat (scale 0.03, shade 0.08 per card), Standard (0.05, 0.12), Deep (0.07, 0.16, 24px strips).
- Deck throw: Gentle (8deg, 700px, always right), Standard (16deg, 1000px, alternating), Lively (24deg, 1200px, 60px lift).
- Cover flow: Flat (no turn, sides at 0.85), Standard (30deg, 0.8), Deep (45deg, 0.75, 180px apart).
- Picture start: Small (420 x 280), Standard (584 x 352), Wide band (full width, 340 tall).
