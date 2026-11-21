---
name: kubai-design-tips
description: Review a product's UI against the Vibe Design Tips and suggest or apply fixes, or apply one tip the user names by its number, English name or Chinese name. Tips are 001 Skeuomorphic buttons, 002 Cascade menu, 003 ASCII and dither, 004 Continuous corners, 005 Drag to reorder, 006 Book cards, 007 Type switches, 008 Wiki layout, 009 Overlays, 010 Liquid glass, 011 Easing curves, 012 Message bubbles, 013 Shockwave glow, 014 Card delete, 015 UI continuity, 016 Toast or notification, 017 Loading states, 018 Holo card, 019 Scroll cards, 020 Choice controls, 021 Ring cards. Use when the user asks to review or polish UI quality, or names one of these tips or effects. 诊断界面质感，给出改造建议和任务；也可以按期号或名称指定某一期，如“019”“滚动卡片”“液态玻璃”。
---

# Vibe Design Tips

Design rules and effects from Kubai's series "999 tips to make your vibe-coded product feel crafted" (vibedesign.tips). Each tip has a live demo and a full text written in the user's voice: in a tip, I, me and my mean the user.

The catalog also holds a few notes: rules from Kubai's other posts, outside the numbered series. A note has no demo; it links to the posts it comes from. Treat a note like a tip.

## Where the tips are

Paths below (catalog/index.md, tips/017-loading-states.md, notes/popover-origin.md) are relative to the folder that holds this file.

Read only what the task needs: catalog/index.md, the categories that apply, then the chosen tips.

## Three ways in

**A. The user names an effect or a tip** ("add liquid glass", "fix our toasts", "019", "给首页加滚动卡片").
The user may give its number, its English name, its Chinese title or another name for it or for one of its effects; the Names list in catalog/index.md has them all. If the Skill was started with nothing but a tip's number or name, this is the way in. Find the tip, read it, and follow it, including the questions it says to ask first. If the name fits more than one tip, ask which one the user means.

**B. The user asks for a review** ("what could feel better in this app?").
1. Read the product: platform and stack, the main screens, shared components, tokens and motion. Tell the user in a few lines what you found.
2. Read catalog/index.md and the categories that apply. Check every rule against its signals in the code. Consider an effect only where the product has the place it fits.
3. Report a table of at most 8 rows, the most value for the least work first: where, what happens now, what to change, the tip (with its demo link, or a note's source link), and the size of the change (S, M or L). Then say in one line what you checked and found fine, and name anything no tip covers.
4. Change no code. Ask which rows to take on.

**C. Turn the chosen rows into tasks.**
For each row, read the tip's full text and write `kubai-tasks/<NNN-name>.md` in the project: the goal; the files and components involved; the tip's open questions, each with a default picked for this product; the steps; and how to check the result. Then ask whether to start one now.

## Rules

- The values in a tip are defaults to start from; fit them to the product.
- Do not paste a tip's full text to the user; quote only what a decision needs.
- Give the demo link whenever you suggest a tip, so the user can see it first; for a note, give its source link.
- Do not suggest an effect the product has no place for, and do not invent tips.

## While making changes

- Build on what the product has: its tokens, components and the names its component library uses. Keep the styles and behaviour the code already sets, unless the tip says to change them.
- Change shared components and tokens rather than adding one-off styles. If a fix would repeat in several places, make it a reusable component first.
- After each change, tidy the code you touched instead of stacking patches on it.
- Check the result in the running product or a browser (devtools, screenshots, the DOM) before calling a task done. To check one component, render it alone on a temporary page and look at its layout, spacing and motion there.

## Version and updates

The version is in VERSION next to this file; a copy without it is an older download from vibedesign.tips. When the user asks to update this Skill, or which version they have, follow https://raw.githubusercontent.com/kubai087/vibedesign-skills/main/INSTALL.md (if GitHub cannot be reached, https://vibedesign.tips/downloads/INSTALL.md). Do not check for updates unasked.
