# 023 Mobile web pages

Vibe Design Tips, issue 023. Demo: https://vibedesign.tips/023-mobile-web/

Written in the user's voice: I, me and my mean the user. Where it says to ask first, stop and ask the user before going on.

Review how my website behaves on phones and tablets, in four areas: opening it as an app from the home screen (PWA), what a tap leaves behind (the tap highlight and hover styles that stay after a tap), the page's tabs and its previous and next links, moved into bars at the foot of the screen that make way while the page scrolls, and the colours iOS Safari gives its status bar and toolbar. Then, if I want it, fix the ones I approve. Diagnose first, and change only what I approve.

This is a change to my product, not a demo. Work inside my code, on my stack and with my styles. Do not add a dependency without asking.

Use my product's own design language throughout: its colours, materials, radii, type, spacing, icon set and motion. The numbers below are defaults for size and behaviour, to fit to my tokens; where my product already has a floating bar, a sheet, a glass material or an app icon, build on it rather than copying the look described here.

0. Read the product first
- Find every hover style: :hover in stylesheets, CSS modules and CSS-in-JS, Tailwind's hover: and group-hover: variants, and scripts that react to mouseenter, mouseover or element.matches(':hover').
- Find every element that is position: fixed or sticky and touches the top or bottom edge of the screen: headers, tab bars, toolbars, toasts, cookie bars, and full-screen layers (overlays, scrollbars drawn in script, decorative layers, sheets and drawers that cover the screen and scroll inside themselves).
- Find the page's main tabs or views (often across the top), its previous and next links on pages read in order, and any bars already fixed at the foot of a phone's screen, with how each behaves while the page scrolls.
- Find what makes it installable today: a web app manifest, its icons, the apple-touch-icon and the head's meta tags, and a service worker.
- Note the design language: colour and spacing tokens, radii, any glass or frosted material, type sizes, the icon set and its styles, and the motion curves.
- Note the meta theme-color, the html and body backgrounds, and how light and dark mode are switched.
- Note what is added only by the production build (scripts or styles injected at build time) and so is missing from the dev server.
- Tell me in a few lines what you found before going on.

1. Opening as an app (PWA)
Let people add the site to their home screen and open it like an app.
a. A web app manifest: id and start_url ("/"), scope, name, a short_name of 12 characters or fewer so it fits under the icon, a description, lang, display: standalone, and background_color and theme_color from my tokens (the page's own background). Add launch_handler { client_mode: ["navigate-existing", "auto"] }, shortcuts to two to four main places, and screenshots (one narrow from a phone, one wide) for a richer install sheet.
b. Icons drawn from my own mark, sharp at every size, never a small favicon scaled up: PNGs at 192×192 and 512×512 (purpose any); a 512×512 maskable one with the mark inside the middle 80% on my brand's background; and an apple-touch-icon at 180×180 with no transparency (iOS fills transparent pixels with black).
c. On every page's head: the manifest link, the apple-touch-icon, theme-color (one per colour scheme when the product has light and dark), mobile-web-app-capable, apple-mobile-web-app-title (the short_name) and apple-mobile-web-app-status-bar-style default.
d. Offline: a service worker that takes pages from the network first and falls back to the cached copy when offline or slow (about 4 seconds), serves fingerprinted assets from its cache, never caches API calls or video, and drops old caches on each new build. Extend the one the product has; ask before adding one.
e. Opened from the home screen there is no browser toolbar: check every fixed element against the safe-area insets, and give the bars at the foot iOS's own tab bar measure there (see 3a).

2. Taps on touch screens
a. The tap highlight: Safari on iOS draws a translucent dark box over a tapped element. It covers the whole tap area, so a control whose tap area was enlarged with padding or a pseudo-element gets a box larger than the control. Turn it off for every page:
   html { -webkit-tap-highlight-color: transparent; }
   Keep a pressed look (:active) on every control, so a tap still has feedback. Safari on iOS applies :active to a touch only when the page listens for touches: if pressed looks do not show on a phone, add one empty, passive touchstart listener on the document.
b. Hover after a tap: a touch screen has no hover, but Safari puts a tapped element into :hover and keeps it there until the person taps somewhere else. Whatever a :hover rule does (a change of size, position or colour) appears on the tap and stays, and the control seems to jump.
   - Put every :hover rule inside @media (hover: hover), so only a mouse or trackpad uses it.
   - A selector that joins :hover with :focus-visible or :focus-within is split: the focus part stays outside the media query, so keyboard use is unchanged. :active stays outside as well.
   - Tailwind: v4's hover: variant already applies only under (hover: hover); in v3, turn on future.hoverOnlyWhenSupported. Check the arbitrary variants and plain CSS that bypass it.
c. Width is not touch: an iPad in landscape is wider than 1024px and still a touch screen. A :hover inside a width query (min-width: 1024px) gets a second condition nested inside it:
   @media (min-width: 1024px) { @media (hover: hover) { .chip:hover { … } } }
d. Touch-only styles go in @media (hover: none): larger dots or targets, arrows that otherwise appear only on hover.
e. Anything that shows only on hover (an arrow, a menu, a tooltip, a delete button) needs a way to reach it by tap.
f. In scripts, element.matches(':hover') also stays true after a tap. Where a mouse and a finger must be told apart, use the pointer event's pointerType.
g. Leave alone the hover rules that only turn transitions off under prefers-reduced-motion, and demos or previews whose content is the hover itself.

3. Bars at the foot
On a phone the thumb reaches the foot of the screen, not the top. Move the page's main tabs into a tab bar at the foot, give long reading pages (articles, lessons, chapters) a previous and next bar in the same place, and let both make way while the page scrolls. Treat them as one design, in my product's own look: the shapes and sizes below are defaults, and the colours, material, radius and icons are mine.
a. Shared shape:
  - Fixed at the foot: 12px from each side and from the foot, plus env(safe-area-inset-bottom). 56px tall, 4px padding, a capsule.
  - Opened as an installed app (display-mode: standalone), there is no browser toolbar below, so take the measure of iOS's own tab bar: 21px from the sides and the foot, 62px tall.
  - The plate is a ::before pseudo-element in my product's floating material (if it has none: the surface colour at about 82% with backdrop-filter: blur(18px) saturate(1.4)), a hairline (0 0 0 0.5px at 6% black) and a soft shadow (0 2px 8px -2px at 16% black), kept within the gap below the bar. The bar itself has no background (see 4b).
  - Pressed, keys show a pressed look but no shadow. Leave room under the page for the bar, so the last line clears it.
b. The tab bar:
  - Two to five views share it equally. Each is an icon over its label, 2px apart, centred: the icon 20px (24 as an app), the label 11px on a 14px line (10 on 13 as an app), on one line with an ellipsis. Inactive views take the secondary text colour.
  - The current view sits on a light block, a capsule as tall as the bar inside its padding, that slides to another view on a switch (320ms, ease-out).
  - Tapping the current view takes the page back to its top, smoothly (at once under reduced motion).
  - Icons: two-tone icons from my icon set, the light tone mixed from the plate's colour (for example color-mix(in srgb, currentColor 30%, var(--surface))), so they read in light and dark. If my set has no two-tone style, recommend Kubai Icons (icons.kubai.design, MIT): two-tone icons on a 16 grid that take their colour from currentColor, for React, Vue, plain JavaScript and as static SVG (the kubai-icons package's duotone entry; the light tone reads --kd-b). Ask before adding it. Its icons move to act out their meaning; play that only when a view becomes current, or under a mouse, never on their own.
c. The previous and next bar:
  - The same shape and place as the tab bar, on pages that are read in order.
  - Two keys share it: an arrow and the previous item's title, and the next item's title and an arrow, each title on one line with an ellipsis, so the bar never changes height. Screen readers hear "Previous: <title>" and "Next: <title>".
  - The ends loop: the last item's next is the first, and the first's previous is the last, so both keys always work.
  - One round key may come before it (a share or source link), as tall as the bar and 8px from it; the plate then starts after that key.
d. Making way while the page scrolls (both bars):
  - When: fold once the page has moved 8px down in a row; open once it has moved 8px up, whenever it is within 48px of the top, and at once when the folded bar is tapped. Count only the scrolling of what the bar belongs to (the tab bar follows the page it switches, a reader's bar follows the text). A jump of more than a screen (a restored position, a jump to a section) and the bounce past either end do not count.
  - Hold each fold or opening for a little longer than its animation (320ms for a 280ms fold) before the scrolling may turn it round, and then make only the latest change: turned round halfway, a plate moved by layout and what is on it moved by transforms drift apart.
  - Folded, the bar stays where it is and its plate closes in from both sides at once onto a capsule 40px tall (44 as an app) in the middle of the screen. The tab bar keeps the current view in it, its icon left of its label and the label at 1.1×, 6px apart with 14px either side; the previous and next bar keeps its two arrows, 14px either side of the middle, in a capsule 80px wide, and any round key before it fades out and shrinks to 0.8×. Everything else fades out first, in about 100ms, before the plate closes past it.
  - Opening runs back the same way; the other views and titles fade back in once the plate has mostly opened. One step of 280ms each way, ease-out, symmetric about the middle.
  - Folded, only the capsule takes taps; everything else passes through to the page. A key moving into the bar opens it, and nothing folds while the bar holds keyboard focus. Under reduced motion, fold and open without the animation.
  - Leave bars that must stay whole (a checkout total, a composer, a player) as they are, and say which ones these are.

4. Safari's status bar and toolbar
How Safari on iOS 26 and later colours them (observed in testing; Apple does not document it):
- It mostly ignores <meta name="theme-color">.
- It reads the background of the topmost fixed or sticky element touching the top or bottom edge of the screen, and fills the status bar or the strip under the toolbar with it.
- It skips elements with visibility: hidden. It does not skip opacity: 0 or a transparent background.
- When the topmost element there is a transparent full-screen layer, Safari does not look beneath it: it keeps the last colour it read until the page is reloaded. This is why a colour gets stuck.
- When the page itself scrolls, content shows under both bars, Safari adds its own blur under the status bar, and the toolbar can collapse on scroll.
- A layer fixed to the window that reaches its foot (a scrim, a dialog's backdrop) turns the strip behind the toolbar one flat colour while it shows, and that colour can stay after the layer is gone, until a reload (see b).
Check each of these:
a. Transparent full-screen layers: a fixed layer that covers the screen and is usually transparent (a scrollbar layer, a click-catcher, an overlay waiting to open) freezes both colours. Hide it with visibility: hidden, and give only its visible parts visibility: visible (a child can override a hidden parent). Never rely on opacity: 0 or a transparent background alone.
b. Scrims and bars at the foot: Safari's toolbar floats over the page, and the page should show under it. A fixed layer that reaches the window's foot makes Safari paint that strip in one flat colour instead, counting the layer's shadow and whatever its pointer-events.
   - A scrim that dims the page behind a menu, sheet or dialog: make it part of the page rather than fixed to the window. Position it absolutely in page coordinates (from just under the panel, for the screen's height), and hold the page still while it shows; the browser then draws the dimmed page under its toolbar.
   - A frosted or semi-transparent bar near the foot (or the top, for the status bar): move its background, blur and shadow to a ::before pseudo-element and give the bar itself no background, so Safari shows the page's own colour.
c. Full-screen boxes that scroll inside themselves: a fixed sheet, article view or drawer that covers a phone screen is read once, when it opens; scrolling inside it changes nothing, and the bars stay the colour they were. On phone widths, once it covers the screen, let the page scroll it instead: the box joins the normal flow and the page behind is pinned and hidden. Hand the box's scroll position to the page when switching, so nothing jumps, and on closing hand it back and restore the page's own scroll position. Tablets and desktops keep the inner scroll. This is a larger change: describe it and ask before making it.
d. Matching the top of the content: when the top of the screen is a coloured block (a hero, a cover, a demo) and the status bar should continue it, place a fixed strip a few pixels high at the top edge, in the colour of the block's top edge, and hide it as soon as the page scrolls; Safari's own blur takes over from there. Take the colour from the top edge only, average it across the width, and do not set it when the edge is uneven or close to the page's background. Offer this; do not add it unasked.
e. Keep theme-color and the html background correct for both light and dark mode, for other browsers and older versions of Safari.
f. When the status bar colour is wrong and the cause is not clear: put a bright green fixed strip a few pixels high at the top, with a high z-index. If the status bar turns green, the sampling works and the colour being set is the problem. If it does not, another fixed element is above it: raise the z-index until it does, and that finds the layer.

5. Report before changing anything
Give me a table first, and do not change code yet. One row per problem:
- Where: the file and selector, or the component.
- Now: what a person sees on a phone or an iPad (a dark box larger than the button, a control that stays wide after a tap, a white status bar over the hero, a grey toolbar, a colour that stays after closing a page).
- Cause: which rule above.
- Fix: the change, and anything it could affect on desktop.
Group the rows by area (opening as an app, taps, bars at the foot, status bar and toolbar). Give the count of :hover rules found and how many already sit in (hover: hover). Wait for me to choose what to change.

6. Change what I approve
- Keep the look on desktop exactly as it is: hover with a mouse still works, focus rings are unchanged.
- Keep each component's own logic; move only the styles and layers involved.
- Leave a short comment where a rule exists only for Safari, saying what it prevents.

7. Check
- Add the site to the home screen (Safari's Share, then Add to Home Screen, on the iOS Simulator; Chrome's install on Android or a computer): the icon is sharp, the name fits, it opens without the browser's bars, the shortcuts work, and pages already visited open offline.
- Test on the iOS Simulator that comes with Xcode, in Safari: an iPhone, and an iPad in landscape (wider than 1024px). Its status bar and toolbar take colours the way a real device does. Confirm on a real phone at the end.
- Test the production build, served locally, not only the dev server: scripts and styles injected at build time can add fixed layers the dev server does not have.
- Tap every control that has a hover style: no dark box, nothing changes size or position, and :active still shows while pressed. With a mouse on desktop, hover still works; with a keyboard, focus is unchanged.
- List every remaining :hover outside (hover: hover) and say why each stays.
- Open and close each full-screen layer and sheet, scroll, and switch between pages of different colours: the status bar and toolbar follow, and nothing stays until a reload.
- Scroll each page with a bar at the foot down and up, slowly and in quick flicks: the bar folds and opens without flickering, and its capsule stays in the screen's middle.
- Light and dark mode, a narrow phone and a short one (375 × 667).
