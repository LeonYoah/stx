# 020 Choice controls

Vibe Design Tips, issue 020. Demo: https://vibedesign.tips/020-choice-controls/

Written in the user's voice: I, me and my mean the user. Where it says to ask first, stop and ask the user before going on.

Review the choice controls in my product, and decide for each one whether it is the right control: switches, checkboxes, radio buttons, segmented controls, tabs, selects and dropdown menus. Then, if I want it, change the ones I approve. This is about judgment first: diagnose and recommend, and change only what I approve.

This is a change to my product, not a demo. Work inside my code, on my stack, with my components, tokens and copy style. Do not add a dependency or a new visual pattern without asking. When a control I need is missing (a segmented control, a select with search), say so and propose one from my component library or the platform before building it.

0. Read the product first
- Find every place a person picks something, turns something on or off, or runs an action from a list: settings and preferences, forms and dialogs, sign-up and onboarding questions, filters and toolbars, page headers, row menus.
- For each control, note:
  - what it sets, or what it does;
  - the options: how many, how long their labels are, and whether each needs a line of explanation;
  - how many can be picked, and whether picking none is a valid answer;
  - when a change takes effect: at once, or on Save or Submit;
  - where it sits: a settings list, a form, a survey, a toolbar or page header;
  - what it shows after a pick.
- Find the shared components behind them (switch, checkbox, radio group, segmented control, tabs, select, combobox, menu).
- Tell me in a few lines what you found before going on.

1. How to choose
Settle each control by these pairs. Each pair differs in one thing.

a. Switch or checkbox: when the change takes effect.
- Switch: it takes effect the moment it is flipped, like a light switch. Use it in settings and preferences that save on their own, with no Save button nearby. The effect should show at once, or be easy to see (a notification turned on, a panel shown).
- Checkbox: it is one answer in a form, and takes effect when the form is submitted (Make it private, in a New project dialog; Email me a copy, before Send).
- One thing on or off is never two radio buttons (On / Off): the reader has to read both to know the state, and it looks like a bigger choice than it is.
- The test: if the opposite of the option is just "not", it is one thing on or off. If both sides have names of their own (Light / Dark, Monthly / Yearly), it is a choice of one: go to (c).
- Wrong: a switch in a form with a Save button (people flip it, leave, and nothing was saved); a checkbox in a list that saves on its own (people look for a Save button).

b. Select or dropdown menu: a value or an action.
- Select: it holds a value. After a pick the field shows the new value, and a setting takes effect at once (pick a language and the page is in that language). In its list the picked option has a tick.
- Dropdown menu: it holds actions (Export data, Duplicate, Delete). A pick runs the action, the button keeps its own name (More, a "…" icon, Export), and the result is shown elsewhere (a toast, a new page, a download). Menus belong in page headers, toolbars and at the end of rows, not among a form's fields.
- Split button: a main action with an icon beside it that opens a dropdown menu of the actions that go with it (Send, and beside it Send tomorrow at 9:00, Send a test to me, Save as draft). Each item runs as it is picked, and the main button keeps its name. Use it when one action is used far more often than the others that go with it.
- Wrong: a menu whose button turns into the picked item; a select whose options are verbs, or that sits on "Actions" and stays on the action just run; a value whose current setting can't be seen without opening the list.
- Tabs and segmented controls differ the same way: tabs switch to a different page of content and set nothing; a segmented control sets a value, or switches the view of the same content (List / Board / Calendar).

c. One of several: by how many options there are.
- 2 to 5 short options: a segmented control. One press picks, and every option stays in view.
- 2 to 5 options that each need a line: radio buttons, one under another, each with its line under its label. Put the group under the setting's label, on the same left edge, rather than squeezed into the control column.
- 6 or more: a select.
- Options whose labels change or whose number can grow (names, counts, dates, items people add): a select, even when there are only a few today, so a longer label or a new option never pushes the layout around.
- Hundreds (time zones, countries, people): a select with a search field at the top of its list; typing narrows the list, and when nothing matches, the list says so (No time zones match "xyz") rather than going blank.
- Wrong: three short options hidden in a select (two presses, and only one option in view); a long list laid out in a row that runs off its end or wraps.

d. Radio button or checkbox: how many can be picked.
- Radio buttons: exactly one, with every option shown. Use them where all the options must be read before answering: a survey or onboarding question, a plan in a form, options that need explaining (see c). A radio can't be cleared once picked, so when "none" is a fair answer, add it as an option (Not sure yet, None of these), or make it a checkbox list.
- Checkboxes: any number, none included. Stack them, one under another.
- Inside a select it is the same: one value gets a tick on the picked option, never radio circles; several values get a checkbox before each option, and the field shows what is picked as chips, or as a count when there are many.

2. Rules for every control
- Each control says what it sets, in its label, not only in its options.
- The current value is always visible without opening anything.
- A control that takes effect at once shows the effect; a form shows nothing has happened until it is submitted.
- Defaults are the safe and common choice; nothing is pre-ticked that signs the person up for something.
- When a control is unavailable, say why, rather than only greying it out.
- Keyboard and screen readers: a switch is role="switch" with aria-checked; radio buttons are one radiogroup moved with the arrow keys; a select opens with Enter or Space and moves with the arrows; a menu is role="menu" and closes with Escape. Touch targets are at least 44 by 44.

3. Report before changing anything
Give me a table first, and do not change code yet. One row per control:
- Where: the screen, and the setting or question.
- What it sets or does, in a phrase: its options, how many can be picked, when it takes effect.
- Now: the control in use.
- Should be: the control it should be, or "right as it is".
- Why: one line that points to the rule in section 1.
- Change: what changes besides the control, if anything: a Save button added or removed, a "none" option added, an action moved out of the form into a menu.
Group the rows by the pairs in section 1. Mark the ones that are already right. When a case could go either way, say what decides it and ask me. Call out every change that alters behaviour (a switch that becomes a checkbox now waits for Save; an action that leaves a form moves to the page header). Then ask me which rows to change, and wait.

4. Change what I approve
- List the files you will touch and wait for my go-ahead.
- Change one control at a time and keep what it sets and does, unless I agreed to change that too. Keep the value people already saved.
- Use my own components where they exist; follow the platform's own controls where they don't:
  - Web: a native select or a listbox with a search field (combobox); role="switch" for switches; a radiogroup for radios; a menu button for menus.
  - iOS: Toggle, Picker with the .segmented or .menu style, Menu, and a searchable list for long ones.
  - Android: Switch, Checkbox, RadioButton, SegmentedButton, ExposedDropdownMenu for values and DropdownMenu for actions.
  - Flutter: Switch, Checkbox, Radio, SegmentedButton, DropdownMenu with search for values, and MenuAnchor for actions.

5. Check
- Every switch takes effect at once, and every checkbox only on submit.
- Every select shows its value after a pick; every menu button keeps its name after an action.
- No radio pair stands for on and off, and every radio question that allows none has a way to say so.
- Long lists can be searched, and say so when nothing matches; short ones are laid out.
- Keyboard, screen reader, phone and desktop layouts, light and dark; existing tests still pass.
