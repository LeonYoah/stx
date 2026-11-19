---
name: kubai-design-tips
description: Review a product's UI against the Vibe Design Tips and suggest or apply fixes, such as loading states, toasts, overlays, motion and easing, continuity, corner radii, typography, and effects like liquid glass or delete animations. Use when the user asks to review or polish UI quality, or names one of these effects. 诊断界面质感，给出改造建议和任务。
---

# Vibe Design Tips

Design rules and effects from Kubai's series "999 tips to make your vibe-coded product feel crafted" (vibedesign.tips). Each tip has a live demo and a full text written in the user's voice: in a tip, I, me and my mean the user.

## Where the tips are

Paths below (catalog/index.md, tips/017-loading-states.md) are relative to the folder that holds this file.

Read only what the task needs: catalog/index.md, the categories that apply, then the chosen tips.

## Three ways in

**A. The user names an effect or a tip** ("add liquid glass", "fix our toasts").
Find it in the catalog, read that tip, and follow it, including the questions it says to ask first.

**B. The user asks for a review** ("what could feel better in this app?").
1. Read the product: platform and stack, the main screens, shared components, tokens and motion. Tell the user in a few lines what you found.
2. Read catalog/index.md and the categories that apply. Check every rule against its signals in the code. Consider an effect only where the product has the place it fits.
3. Report a table of at most 8 rows, the most value for the least work first: where, what happens now, what to change, the tip (with its demo link), and the size of the change (S, M or L). Then say in one line what you checked and found fine, and name anything no tip covers.
4. Change no code. Ask which rows to take on.

**C. Turn the chosen rows into tasks.**
For each row, read the tip's full text and write `kubai-tasks/<NNN-name>.md` in the project: the goal; the files and components involved; the tip's open questions, each with a default picked for this product; the steps; and how to check the result. Then ask whether to start one now.

## Rules

- The values in a tip are defaults to start from; fit them to the product.
- Do not paste a tip's full text to the user; quote only what a decision needs.
- Give the demo link whenever you suggest a tip, so the user can see it first.
- Do not suggest an effect the product has no place for, and do not invent tips.

## Version and updates

The version is in VERSION next to this file; a copy without it is an older download from vibedesign.tips. When the user asks to update this Skill, or which version they have, follow https://raw.githubusercontent.com/kubai087/vibedesign-skills/main/INSTALL.md (if GitHub cannot be reached, https://vibedesign.tips/downloads/INSTALL.md). Do not check for updates unasked.
