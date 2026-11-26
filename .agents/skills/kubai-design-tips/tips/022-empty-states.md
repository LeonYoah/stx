# 022 Empty states

Vibe Design Tips, issue 022. Demo: https://vibedesign.tips/022-empty-states/

Written in the user's voice: I, me and my mean the user. Where it says to ask first, stop and ask the user before going on.

Review every place in my product that can be empty, and decide for each one what kind of empty it is, how big it should be, and what it should say and show. Then, if I want it, change the ones I approve. This is about judgment first: diagnose and recommend, and change only what I approve.

This is a change to my product, not a demo. Work inside my code, on my stack, with my components, tokens, icons and copy style. Do not add a dependency or a new visual pattern without asking. Every value below is a default to start from.

0. Read the product first
- Find every place that can show nothing: lists and tables, boards and calendars, inboxes and notifications, search and filter results, dashboards and chart cards, folders and uploads, comments and activity, members and invites, trash and archives, menus and pickers with no options, pages that can fail to load, and pages a person may not be allowed to open.
- For each one, note: what normally fills it; why it can be empty (nothing made yet, a search or filter, everything handled, a failed load, no permission, offline); how big the empty area is (a page, a card, one row of a list, a line in a menu); and what it shows today (nothing at all, a bare "No data", a stock illustration or emoji, a spinner that never ends).
- Find the shared pieces: an EmptyState component if there is one, the icon set and whether it has a two-tone or filled style, the button styles, the skeleton and error components, and how light and dark mode are switched.
- Tell me in a few lines what you found before going on.

1. The kinds of empty
a. First use: nothing has been made yet (a new project, a new account, a first folder).
  - Show what will be here: a wireframe of this very screen, filled in, in miniature (see 4), or a two-tone icon of what lives here.
  - Title: the state in a few words (No tasks yet). Description: how to start, in one sentence.
  - Buttons: one main action that makes the first thing (New task), and at most one other way in (Use a template, Import).
  - When the first thing is made, it takes the empty state's place at once; nothing else on the page moves.
b. No results: a search or filter found nothing. The product is not empty, so never say it is.
  - Keep the search text and every active filter in view, exactly as the person left them.
  - The title names what was searched (No tasks match "invoice q3"). The description offers one fix (check the spelling, search every project).
  - Buttons: Clear search or Clear filters, as a secondary button; and, when it exists, a wider search as a ghost button.
  - The picture is small (80 × 60 at most), or there is none.
  - In a search menu or a picker, it is one grey line with no picture (No tasks match "invo"), updated as the person types.
c. All done: everything was handled (an inbox cleared, a queue emptied, no notifications, an empty trash).
  - Say so plainly (You're all caught up), and in the description what will show up here next.
  - No button, and no accent colour in the picture: there is nothing to do.
d. Failed: the content could not be loaded.
  - Show it only where the failure is: the one card, list or panel that did not load, never the whole page when the rest of it loaded.
  - While loading, that area shows a skeleton in its own shape (the real rows, the real chart's columns); when the load fails, the skeleton gives way to the empty state in the same place, at the same size.
  - The picture is a two-tone alert icon, not a wireframe. Title: what did not load (Couldn't load this chart). Description: what happened and what is safe (The connection dropped. Nothing was lost.).
  - Button: Try again, as a secondary button whose own reload icon turns while it retries; on success the content comes in where the empty state was.
  - Offline is this kind too: say the work will sync when the connection is back, and offer Try again.
e. No access: the person opened something they may not see.
  - The picture: a wireframe of what is behind the door, with a lock, and no accent colour: getting in is the owner's call.
  - Title: what they cannot open, by name. Description: who owns it and how to get in.
  - Buttons: Request access as the main action, and a ghost button back to where they were.
  - After a request, the button turns in place into Request sent, with a check, and the description says what happens next. Nothing else moves.
f. Places that need no empty state: an area that simply closes or hides when empty (a notification tray that closes, a section that is left out). Say which ones these are, and leave them.

2. Sizes
a. Page: the empty state fills a page or a large panel. Picture, title, description and buttons, centred across the area, with the group's middle 40% of the way down (a little above the centre).
b. Card: a card on a dashboard or a sidebar panel. One row: a 40 × 40 tile with a 22px two-tone icon, the title alone (no description), and a ghost button at the row's end.
c. Row: one part of a list is empty (no pending invites under the members). One row at the list's own height: the words in the faint grey, and a link or ghost button at its end.
d. Menu: a menu, a picker or a search's suggestions. One line in the faint grey, no picture, no button.
Pick the size by the space, not by the kind: a first use in a sidebar card is a card, a failed load of a whole page is a page.

3. Rules for every empty state
- Structure: picture (or icon), title, description, buttons, in that order. Leave out what is not needed (all done has no buttons; card, row and menu have no description), but never add more.
- Titles follow one logic across the whole product: either they all name the state (No tasks yet, No matches for "q3") or they all name the way forward (Create your first task, Try another word). Pick the one the product already leans to and use it everywhere. Never an apology (no "Oops", no "Uh-oh"), and never blame the person.
- Description, when there is room: one of three things. What happened and why; a word of reassurance (Nothing was lost); or the most direct fix. A fix that takes steps may be a short numbered list of up to three steps. Card, row and menu sizes have no description.
- Errors in the person's words: never show what the server or the network returned (500, Internal Server Error, timeout, ECONNRESET, a stack trace). Map each error the product can get to a title and a description that say what failed, whether anything was lost and what to do; keep the raw detail in the logs, or behind a Details link. Translate these strings with the rest of the product.
- Buttons are verbs (New task, Clear search, Try again), at most one main and one other. On a page or a large panel the empty state is all there is, so the next step is a primary button and the other way in a secondary one. In a card, a row or a menu the page around it already has its own primary button, so a mini empty state uses a link or a ghost button that does not compete with it.
- Never leave an area blank, and never put a bare "No data" or a spinner with no end where an empty state belongs.
- Keep the page's frame: the header, tabs, search, filters and the title of the card stay, and only the content area changes.
- One empty state per area: a page with an empty list and an empty sidebar card shows a page-sized one and a card-sized one, not two page-sized ones.
- Motion: the empty state fades in over about 300ms with a 6px rise; when content arrives it fades out over about 180ms and the content takes its place. Under reduced motion, no rise, only the fade.
- Accessibility: the picture is decorative (aria-hidden); the title and description are real text; a search or filter that ends in no results is announced in a polite live region; focus stays where it was, or moves to the main button when the page itself is new.

4. The picture
a. Two styles, both drawn from my product. Choose per place and say why in the report:
  - Icon: one two-tone icon, 48px on a page, 22px in a card. Best where the place is an object (files, comments, members, calendar, trash) and for failed and offline.
  - Wireframe: a small, plain copy of this very screen, filled in. Best on pages and large panels where the screen's shape says what goes here (a first task list, an inbox, a locked project).
  - Never a stock illustration, a person, a mascot or an emoji.
b. Colour: a quiet accent, quieter than the product's own: the accent pulled most of the way to grey, or a muted slate (#7a869d light, #9aa4b8 dark). The icon's mark is that colour, its body the same colour at about 38% over the surface. The product's real accent stays for the real buttons.
c. Icons: use my icon set's two-tone or filled style when it has one. If it has none, recommend Kubai Icons (icons.kubai.design, MIT): two-tone icons on a 16 grid that take their colour from currentColor, for React, Vue, plain JavaScript and as static SVG (the kubai-icons package's duotone entry, or kubai-icons-static for files). Ask before adding it. Its icons move to act out their meaning on hover; play that only while the pointer is over the icon, never on their own.
d. The wireframe (defaults; use my tokens for the same roles):
  - viewBox 0 0 120 90 on a page; 80 × 60 for no results. No base, no background, no hatching: it sits on the page.
  - One scene, 2 or 3 objects, centred, with at least 14 clear on every side.
  - Objects are cards in the surface colour, corner 10 to 11, a 1px border in the line colour, and a soft shadow (0 2px 3px at 6%).
  - Words become placeholder bars with round ends: body 4 high, titles 5 high. Empty places are slots. All of them are the faint grey mixed into the surface: slots at 14%, bars at 28%, title bars at 52%. Line icons in the faint grey, 1.5 wide.
  - At most one accent element, and only for the next step the person can take here (add, upload, retry): a small round button in the quiet accent with a white mark. No next step, or not the person's call: no accent.
  - Draw it as one self-contained inline SVG: colours as CSS variables of my tokens so it follows light and dark mode, no external resources, no <image> or <use>, every id with its own prefix, shadows built from feGaussianBlur and feOffset (or a CSS drop-shadow) rather than feDropShadow.
e. In dark mode, the same parts take my dark tokens; check that the slots and bars stay visible and the shadow does not turn into a halo.

5. Report before changing anything
Give me a table first, and do not change code yet. One row per place:
- Where: the screen or component, and what normally fills it.
- Now: what it shows when empty.
- Kind and size: first use, no results, all done, failed, no access, or none needed; and page, card, row or menu.
- Picture: icon (which one) or wireframe (of what), or none.
- Copy: the title, the description and the button labels you propose.
Group the rows by kind. Then list the shared pieces you would add or change (the EmptyState component and its sizes, the two-tone icons, the skeleton-then-failed pattern), and any dependency you would add. Wait for me to choose what to change.

6. Change what I approve
- Build one EmptyState component with the sizes in 2 (page, card, row, menu) and props for the picture, title, description and up to two actions; use it everywhere instead of one-off markup.
- Keep each place's existing loading, error and data logic; only what is shown when empty changes.
- Write the copy in the product's own voice and languages; translate the new strings where the product is translated.

7. How to build it, by platform
These are suggestions; check them against my stack and its versions, and prefer what the product already uses.
- Web: a React (or Vue, Svelte) EmptyState component with a size prop; the picture as an inline SVG component that reads CSS variables; with React Query or SWR, decide the kind from the query's state (isError, an empty array, an active search); announce no results with aria-live="polite". Kubai Icons: import { createIcon, Folder } from 'kubai-icons/duotone', or the <kubai-icon> element with icon-style="duotone"; the light body reads --kd-b.
- iOS: ContentUnavailableView for page-sized states (its search variant for no results), with a custom label for the picture; for card and row sizes, a small HStack view. SF Symbols in hierarchical rendering give a two-tone icon.
- Android: a composable EmptyState(size, picture, title, description, actions) in Compose, driven by the screen's UI state; the picture as an ImageVector or a drawn Canvas in theme colours.
- Flutter: one EmptyState widget with a size enum; the picture with CustomPaint or flutter_svg in theme colours.
- Rust and other native desktops (egui, iced, GPUI, Tauri): the same component in the UI model, its picture drawn from the theme's colours.

8. Check
- Open every place in each kind: new account, a search for nonsense, everything cleared, the network off (and a slow network, to see the skeleton before the failure), and a link to something without access.
- Nothing is blank, nothing says only "No data", no spinner runs forever, and the search text and filters survive no results.
- Making the first item, clearing a search, a successful retry and a sent request each change the area in place, with nothing else on the page moving.
- Light and dark, a narrow phone width, long translated strings, a screen reader on the no-results announcement, reduced motion; existing tests still pass.
