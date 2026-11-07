# STX 开屏动画素材来源 / Splash asset provenance

本目录中与登录开屏（第四版）相关的静态素材说明如下。  
Provenance notes for post-login splash (edition v4) static assets in this folder.

## `stx-mark.png`

- **用途 / Use**：开屏动画中的真实品牌 mark（鸟标）。  
  Real brand mark used by the splash animation.
- **来源 / Source**：仓库既有品牌资产 `frontend/public/brand/stx-mark.png`（产品官方 mark，非生成图）。  
  Existing product brand asset already checked into this path (official mark; not AI-generated).

## `stx-splash-nebula.jpg`

- **用途 / Use**：开屏青蓝星云背景（object-fit: cover）。  
  Cyan-blue nebula background for the splash canvas (`object-fit: cover`).
- **来源 / Source**：从用户逐版审定的第四版单文件原型 `stx-meteor-intro-v4.html` 内嵌 JPEG 原样提取，未重新生成或替换。  
  Extracted verbatim from the user-approved fourth-edition single-file prototype `stx-meteor-intro-v4.html` (embedded JPEG). Not regenerated or substituted.
- **集成约束 / Integration constraint**：禁止依赖其它 bot / 会话的绝对路径资产；本文件随前端仓库一起发布。  
  Do not depend on absolute paths from other bot sessions; ship this file with the frontend.

## 原型对照 / Prototype reference

- 节奏与终态以第四版原型为准：流星 → 碎裂 → 聚字 → ~6.8–7.08s 干净实心 STX → mark 并排 +「欢迎回来」→ 9s 定格。  
  Timing and final lockup follow the v4 prototype: meteor → shatter → reform → clean solid STX around 6.8–7.08s → mark + welcome → 9s hold.
