# 018 Holo card

Vibe Design Tips, issue 018. Demo: https://vibedesign.tips/018-holo-card/

Written in the user's voice: I, me and my mean the user. Where it says to ask first, stop and ask the user before going on.

Turn a card in my product into a holographic card: it tilts toward the pointer in 3D, a foil finish and a glare follow the light across it, the artwork inside has depth, it sways a little when left alone, and (if I want) it turns over on click to show its back, with a visible edge. Build it inside my code and design system; do not change what the card says or does.

Every value in this prompt is a recommendation, not a requirement. Sections 1 to 7 give a full set that works together (the "Standard" values, tested in a demo); section 9 gives presets around them. I choose; you build what I choose.

First look at my product (section 0). Then ask me these one at a time. For each, show the options in a short list with their key values, mark the recommended one, and wait for my answer; I may pick an option, change single values, or give my own:
1. Which card: the one element that stands for status or ownership (a member or loyalty card, a ticket or pass, a badge or achievement, a profile or collectible card, a plan tier). Holographic effects lose their meaning when everything shines, so one card per screen.
2. Finish: one of the six in section 3 (Prism recommended), several to choose from (for tiers or rarity), or my brand colors (then derive the base, foil, glare, rim and edge from them and show me the values).
3. Feel: Quiet, Standard (recommended) or Lively (section 9), or my own tilt angle, springs and sway.
4. Interactions: tilt toward the pointer (on), the idle sway (on), turning over on click (needs a back: say what goes on it, such as a QR code or terms), tilting with the phone's motion on mobile (off; needs permission on iOS).
5. If it turns over: the thickness (Thin, Standard recommended, Thick) and the flip spring (section 9).
6. Depth: which parts of the card's artwork get their own layer, and how deep (None, Subtle, Standard recommended; section 9).
7. Background: leave the page behind the card as it is (recommended for most products), or add the slow fluid behind it (section 7).
Then sum up my choices with their values in one table, propose how you will build it on my platform (section 8 is a starting point), with the cost and any fallback, and wait for my go-ahead.

0. Start from what exists
- Find the card: its component, size, corner radius, artwork and content, where it sits, and what is around it. Check whether a back already exists (terms, a code, details).
- Find how the screen draws (DOM, a native view tree, a GPU renderer), the platform version range, whether there is a dark mode, and how pointer and touch input reach the card.
- Tell me in a few lines what you found before asking.

1. Tilt and light (Standard)
- With the pointer at (fx, fy) in 0..1 across the card, nx = fx * 2 - 1 and ny = fy * 2 - 1, the target is rotateY = nx * 12deg and rotateX = -ny * 12deg. Perspective 1600px on the card's parent.
- The tilt follows its target on a spring, so it overshoots a little when the pointer leaves: acceleration = 140 * (target - angle) - 15 * velocity, per second.
- The light sits where the pointer is (lx = fx * 100%, ly = fy * 100%) and follows on a spring without overshoot (stiffness 160, damping 2 * sqrt(160) ≈ 25.3). With no pointer on the card, it rests up and to the left (34%, 22%), like a lamp in the room.
- Hover strength `on` goes from 0 to 1 while the pointer is on the card, easing at 1 - exp(-7 * dt) per frame; it brightens the foil and the glare.
- Left alone, the card sways as if held: rotateX = 2.2deg * sin(0.9t + 0.6) + 0.8deg * sin(2.3t), rotateY = 3.2deg * sin(0.7t) + 0.9deg * sin(1.9t + 1.2), t in seconds, and the light moves with it (lx = 34% + rotateY * 3, ly = 22% - rotateX * 3).
- Touch: dragging on the card tilts it the same way (touch-action: none on the card only). Phone motion, if chosen, maps the device's tilt to the same target, clamped to the same angles.
- The shadow under the card: a blurred ellipse (black at 60%, blur 24px, 84% of the card's width, 70px tall, 30px below it) that slides away from the side that lifts: x = -rotateY * 1.8px, y = rotateX * 1.2px.

2. The layers of a face, back to front
- Art: the card's own base color or picture.
- Foil: the finish (section 3), blended over the art, brightest near the light.
- Print: everything printed on the card (text, logo, artwork window), above the foil so it stays readable.
- Glare: a radial gradient centered on the light, white at 60% in the middle, 14% at 28%, then black at 30% at 95% (farthest corner), blended with overlay; opacity 0.35 + on * 0.65.
- Rim: a 1.5px ring around the edge (mask with content-box exclude): a faint base color all round, plus a radial gradient at the light in the rim's light color, fading out by 42%; opacity 0.6 + on * 0.4. It should come and go as the card moves, never read as a solid outline.
- A hologram seal, if the card has a place for one: a 44px disc, conic-gradient(from <angle>, rgb(255 154 200), rgb(255 224 138), rgb(164 255 207), rgb(143 220 255), rgb(183 155 255), repeated twice), with fine rings over it (white at 35%, 1px every 3.5px) and a 1px white rim at 60%. The angle = atan2(ly - 50%, lx - 50%) * 1.5 + rotateY * 6.
- Corners: 32px on a 440 x 616 card (scale with the card), on a continuous curve where the platform has one.

3. Finishes (recommended values; pick, mix or derive from my brand)
Each lists: base, foil, glare and rim if they differ from section 2, ink, edge color (for the thickness), and the glow behind the card (section 7).
- Prism (recommended): deep blue glass lit from inside.
  - Base: radial-gradient(60% 40% at 50% 0%, rgb(90 150 255 / 0.55), transparent), linear-gradient(170deg, rgb(13 35 168), rgb(8 18 102) 55%, rgb(22 15 92)). Ink rgb(244 244 246).
  - The artwork window holds the light: radial-gradient(46% 50% at the light, rgb(255 175 235) at 0.55, 0.4 at 18%, 0.22 at 38%, 0.09 at 60%, 0.02 at 82%, transparent), radial-gradient(54% 56% at (100% - lx, 74%), rgb(125 225 255) at 0.45, 0.32 at 18%, 0.17 at 38%, 0.07 at 60%, 0.015 at 82%, transparent), radial-gradient(70% 55% at 30% 100%, rgb(150 255 210 / 0.3), transparent 75%), over linear-gradient(170deg, rgb(59 98 230), rgb(42 63 191) 55%, rgb(58 47 168)). Many stops, so it reads as glow, not spots. Its frame: rgb(140 170 255 / 0.1) with a 1px line at rgb(170 200 255 / 0.28).
  - Foil: a 2px lit rim, radial-gradient(at the light, rgb(170 220 255 / 0.9), transparent 60%), masked to the ring; opacity 0.15 + on * 0.45. Glare opacity 0.25 + on * 0.45.
  - Edge rgb(26 42 138). Glow hue 228.
- Rainbow: bands of color that slide as it tilts.
  - Base: radial-gradient(130% 80% at 25% 0%, rgb(59 62 76), rgb(27 28 35) 62%). Ink rgb(245 245 247).
  - Foil: repeating-linear-gradient(115deg, rgb(255 95 126) 0%, rgb(255 184 107) 2.6%, rgb(244 255 107) 5.2%, rgb(107 255 181) 7.8%, rgb(95 210 255) 10.4%, rgb(139 123 255) 13%, rgb(255 107 214) 15.6%, rgb(255 95 126) 18%), background-size 200%, background-position at the light, under a grating repeating-linear-gradient(125deg, transparent 0 2px, rgb(0 0 0 / 0.3) 2px 3px); mix-blend-mode color-dodge, filter brightness(0.72) saturate(1.25), opacity 0.3 + on * 0.6, masked by radial-gradient(farthest-corner at the light, rgb(0 0 0) 5%, rgb(0 0 0 / 0.16) 72%).
  - Edge rgb(42 44 53). Glow hue 285.
- Pearl: a white card with a pastel sheen, strongest toward its edges like a frame.
  - Base: linear-gradient(160deg, rgb(253 252 255), rgb(241 238 248)). Ink rgb(23 21 46).
  - Foil: linear-gradient(115deg, rgb(255 196 225) 0%, rgb(201 184 255) 12%, rgb(180 233 255) 24%, rgb(194 255 216) 36%, rgb(255 243 184) 48%, then the same again), background-size 260%, position at the light, multiply, opacity 0.7 + on * 0.3, masked by radial-gradient(70% 60% at 50% 45%, rgb(0 0 0 / 0.35) 30%, rgb(0 0 0) 100%).
  - Glare: soft-light, white at 90% fading out by 55%, opacity 0.5 + on * 0.5. Rim: base rgb(60 50 120 / 0.06), light white at 95%.
  - Edge rgb(230 226 242). Glow hue 300.
- Sparkle: fine glitter.
  - Base: radial-gradient(120% 80% at 75% 5%, rgb(40 51 106), rgb(16 20 43) 65%). Ink rgb(238 240 255).
  - Glitter: a 240px tile of about 950 dots, radius 0.45 to 1.85px (most small), alpha 0.35 to 1, used as an alpha mask.
  - Foil, blended with screen, two layers, each masked by the glitter (the second offset by 117px, 71px) and by radial-gradient(at the light, rgb(0 0 0), rgb(0 0 0 / 0.3) 45%, rgb(0 0 0 / 0.12)):
    first: repeating-linear-gradient(60deg, rgb(255 139 210) 0 3%, transparent 5% 8%, rgb(139 243 255) 10% 13%, transparent 15% 18%, rgb(255 243 139) 20% 23%, transparent 25% 28%), size 260%, position (lx * 1.6, ly);
    second: repeating-linear-gradient(-35deg, rgb(169 139 255) 0 3%, transparent 5% 9%, rgb(139 255 196) 11% 14%, transparent 16% 20%, rgb(255 255 255) 22% 24%, transparent 26% 30%), size 260%, position (100% - lx, ly * 1.4).
    The dark gaps make each fleck light at its own angle and go out again.
  - Edge rgb(26 32 72). Glow hue 235.
- Chrome: a silver mirror whose highlights travel against the tilt.
  - Base: linear-gradient(125deg, rgb(228 230 234) 0%, rgb(141 147 158) 16%, rgb(246 247 249) 30%, rgb(124 130 141) 46%, rgb(217 220 225) 60%, rgb(148 154 165) 76%, rgb(241 242 244) 100%), background-size 220%, background-position (100% - lx, 100% - ly). Ink rgb(27 29 36).
  - Foil: linear-gradient(115deg, rgb(255 143 199), rgb(255 226 143) 14%, rgb(149 255 208) 28%, rgb(143 214 255) 42%, rgb(192 155 255) 56%, then again), size 240%, position at the light, overlay, opacity 0.3 + on * 0.35.
  - Glare: soft-light, white at 85%, 10% at 30%, black at 18% at 95%. Rim: base white at 18%, light white at 95%. An engraved look for printed lines: text-shadow 0 -1px 0 rgb(60 64 76 / 0.55), 0 1px 0 rgb(255 255 255 / 0.95).
  - Edge rgb(180 184 194). Glow hue 200.
- Etched: a pattern you only see in the light.
  - Base: matte rgb(18 18 20) with fine concentric rings, repeating-radial-gradient(circle at the artwork's center, rgb(255 255 255 / 0.05) 0 1.2px, transparent 1.6px 7px). Ink rgb(237 237 238).
  - Foil: linear-gradient(115deg, rgb(255 95 126), rgb(255 184 107) 10%, rgb(244 255 107) 20%, rgb(107 255 181) 30%, rgb(95 210 255) 40%, rgb(139 123 255) 50%, then again), size 300%, position at the light, masked by the same rings and by radial-gradient(at the light, rgb(0 0 0), rgb(0 0 0 / 0.5) 18%, transparent 42%); opacity 0.25 + on * 0.75. Glare opacity 0.2 + on * 0.4.
  - Keep the artwork window a plain matte panel (radial-gradient(70% 60% at 50% 35%, rgb(29 29 33), rgb(15 15 17))) so the rings stop at it.
  - Edge rgb(27 27 30). Glow hue 190.
- On every finish, the rim's base color is white at 5% unless listed, and its light color white at 50%. Text stays readable at every angle: check the lightest and darkest moment of the foil under it.

4. Turning over (only if chosen; Standard values)
- A click (or tap, or Enter and Space when focused) turns the card 180deg around its vertical axis on a spring (stiffness 70, damping 11), so it settles with a small overshoot in about 0.8s; the same again turns it back.
- It lifts toward the viewer while it turns: translateZ(|sin(angle)| * 40px); the shadow narrows by up to 45% and drops 14px at edge-on.
- Thickness 10px: the front sits 5px in front of the card's middle and the back 5px behind it, turned 180deg, both with backface-visibility hidden; between them, 10 copies of the card's outline in the finish's edge color, evenly spaced. Draw the edge only while turning.
- The back carries the same finish, glare and rim, with the light mirrored (lx becomes 100% - lx). Lay it out on the same margins as the front.
- Anything that changes the front (another finish, new content) changes while the card is edge-on, so it is never seen switching.

5. Depth inside the artwork (Standard values)
- Give each part of the artwork its own depth d behind the card's face (negative: in front of it), and move it by d * sin(angle): x = -d * sin(rotateY + turn), y = d * sin(rotateX). This is what real 3D would do.
- In the demo: the picture behind it 90px, the compass dial 50px, its middle ring 34px, the dashed ring 20px, the needle 12px in front (-12); the print in the corners sits on the surface and does not move. Draw the backmost layer larger than its window (100px on each side) so its edges never show.
- If the artwork can respond, let it: the compass needle springs toward the pointer (acceleration = 60 * angle difference - 5.5 * velocity, the short way round) and drifts by 12deg * sin(0.77t) + 6deg * sin(2.16t) when left alone; the dial turns 9deg a second and the dashed ring 18deg a second the other way. Small, slow, and one moving idea.

6. Rhythm and accessibility
- Hover in and out, tilt and light are continuous; the flip settles in about 0.8s.
- Switching finishes: cross-fade the faces over 420ms, or swap them while edge-on during a flip.
- Reduced motion: no tilt, no sway and no flip animation (the flip becomes an instant swap or a 200ms cross-fade); the light may still follow the pointer; the background stands still.
- The card keeps its role: a link or button stays one, the back is reachable by keyboard and announced, and nothing important is only visible at one angle.

7. The background (only if chosen)
- A glow behind the card: radial-gradient(closest-side, hsl(<finish's hue> 85% 64% / 0.3), transparent), 860px across, its opacity (0.04 + 0.12 * on) / 0.3.
- A slow fluid behind it: value noise in fBm (4 octaves, each turned by the matrix (1.6, 1.2; -1.2, 1.6), amplitude halving), warped twice: p = position / height * 1.25 + flow; q = (fbm(p), fbm(p + (5.2, 1.3) - flow.yx)); r = (fbm(p + 3.5q + (1.7, 9.2) + 1.3 * flow), fbm(p + 3.5q + (8.3, 2.8) - flow)); f = fbm(p + 3r). Color = mix(a, b, clamp(2.4f²)), then mix toward c by clamp(0.95 * |q| - 0.35), times 0.5 + 1.1f². Fade to the page color with smoothstep(0.82, 0.18, distance from (50%, 56%), y stretched 1.2). The flow moves round a circle of radius 0.33 about once every 40s; shift the field by (-rotateY, rotateX) * 0.008 so it answers the tilt.
- Palettes (a, b, c): Prism rgb(10 20 80), rgb(58 42 156), rgb(26 108 192); Rainbow rgb(44 12 64), rgb(12 68 80), rgb(90 32 52); Pearl rgb(51 40 94), rgb(28 74 74), rgb(92 44 80); Sparkle rgb(12 20 56), rgb(38 51 122), rgb(74 58 22); Chrome rgb(24 28 36), rgb(44 54 70), rgb(22 64 74); Etched rgb(13 15 18), rgb(26 36 40), rgb(38 42 50). Ease between palettes over about a second.
- Draw it at a third of its size, 30 times a second, and scale it up; it is soft, so nobody can tell.

8. How to build it, by platform
These are suggestions; check them against my stack and its versions, and choose what fits.
- Web (as in the demo): CSS 3D transforms with the layers as stacked elements and mix-blend-mode. Set the tilt as a transform on the card itself and the light as custom properties on the face only, never as inherited variables on a parent: changing a variable restyles everything under it. Move the glare as a 2x-sized layer with a transform, put the foil and the animated artwork on their own layers (will-change: transform), draw only the face that shows, and skip writes when a value has not changed. Drive everything from one requestAnimationFrame loop; the fluid is a small WebGL canvas.
- iOS (SwiftUI): rotation3DEffect with a perspective, the layers as overlays with .blendMode(.colorDodge / .overlay / .multiply), gradients positioned from the light, spring animations for tilt and flip; DragGesture for touch and CoreMotion (CMMotionManager deviceMotion) for phone tilt. A Metal [[stitchable]] colorEffect can draw the foil in one pass.
- Android (Compose): Modifier.graphicsLayer { rotationX; rotationY; cameraDistance = 12 * density }, Brush gradients drawn with BlendMode in drawWithContent, Animatable springs, pointerInput for drag, SensorManager (TYPE_GAME_ROTATION_VECTOR) for phone tilt. On API 33+, an AGSL RuntimeShader can draw the foil.
- Flutter: Transform with Matrix4.identity()..setEntry(3, 2, 0.0006)..rotateX()..rotateY(), the layers in a Stack with ShaderMask or CustomPainter blend modes, a FragmentProgram for the foil and the fluid, sensors_plus for phone tilt.
- Rust (wgpu, Bevy, egui): draw the card as one quad with a fragment shader that computes the foil, glare and rim from the light uniform, and transform it in 3D; the flip and edge are a second quad and a thin box.

9. Presets (around the Standard values; offer these when asking)
- Feel:
  Quiet: tilt 6deg, spring 120 / 18 (no visible overshoot), sway at half (about 1 to 1.5deg), hover strength capped at 0.7.
  Standard (recommended): tilt 12deg, spring 140 / 15, sway 2 to 3deg (section 1).
  Lively: tilt 16deg, spring 180 / 12 (a clear bounce), sway 3 to 4deg.
- Thickness: Thin 6px (6 layers), Standard 10px (10 layers, recommended), Thick 16px (16 layers); lift 30, 40 or 56px.
- Flip spring: Soft 55 / 9 (slower, more overshoot), Standard 70 / 11 (recommended), Crisp 110 / 18 (fast, no overshoot).
- Depth: None; Subtle (every depth halved); Standard (section 5, recommended).
- Pearl-like restraint for everyday products: Pearl with Quiet; Rainbow and Sparkle with Standard or Lively for moments worth celebrating.

10. Check
- Every finish at the four corners and in the middle: the text stays readable, the glare never washes out the print, the rim never reads as a solid line.
- Flip it many times quickly: it never shows both faces at once or a gap at the edge.
- The values built match the table I approved.
- It runs at the display's frame rate on a mid-range phone, and costs nothing when off screen; reduced motion and keyboard both work; light and dark pages both checked.
