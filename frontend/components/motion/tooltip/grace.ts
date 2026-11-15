/*
 * MIT License
 *
 * Copyright (c) 2026 Saurabh Chauhan
 *
 * Vendored from beUI Status Bar (https://beui.dev/charts/status-bar).
 * Registry: https://beui.dev/r/status-bar.json
 * Upstream: https://github.com/starc007/ui-components
 * License: https://github.com/starc007/ui-components/blob/main/LICENSE
 *
 * 本地改动：beUI 的 lib/i18n 指向 components/charts/beui-i18n.tsx，不占用 STX next-intl 的 frontend/lib/i18n/。
 * Local change: beUI's lib/i18n points at components/charts/beui-i18n.tsx so it does not shadow STX's next-intl
 * frontend/lib/i18n/. Local STX adaptations remain under the same MIT terms and are not relicensed as Apache-2.0.
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in all
 * copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 */

type Point = {x: number; y: number};
type Box = {left: number; top: number; right: number; bottom: number};

/**
 * The corridor a pointer may cross between the trigger and the open tooltip
 * without closing it: the convex hull of where it left and the box it is
 * heading for. A fixed timeout either cuts a slow diagonal move short or keeps
 * the label alive after the pointer has plainly gone elsewhere; this follows
 * the path itself.
 */
export function graceArea(exit: Point, target: Box): Point[] {
  // A few pixels either side of the exit point give the corridor some width at
  // its narrow end, so a hand that drifts sideways while it moves stays inside.
  const pad = 5;
  return hull([
    {x: exit.x - pad, y: exit.y - pad},
    {x: exit.x + pad, y: exit.y - pad},
    {x: exit.x - pad, y: exit.y + pad},
    {x: exit.x + pad, y: exit.y + pad},
    {x: target.left, y: target.top},
    {x: target.right, y: target.top},
    {x: target.right, y: target.bottom},
    {x: target.left, y: target.bottom},
  ]);
}

export function inBox(point: Point, box: Box) {
  return (
    point.x >= box.left &&
    point.x <= box.right &&
    point.y >= box.top &&
    point.y <= box.bottom
  );
}

export function inPolygon(point: Point, polygon: Point[]) {
  let inside = false;
  for (let i = 0, j = polygon.length - 1; i < polygon.length; j = i++) {
    const a = polygon[i];
    const b = polygon[j];
    if (
      a.y > point.y !== b.y > point.y &&
      point.x < ((b.x - a.x) * (point.y - a.y)) / (b.y - a.y) + a.x
    ) {
      inside = !inside;
    }
  }
  return inside;
}

// Monotone chain; the corridor is always convex, so a hull is all it needs.
function hull(points: Point[]) {
  const sorted = [...points].sort((a, b) => a.x - b.x || a.y - b.y);
  const cross = (o: Point, a: Point, b: Point) =>
    (a.x - o.x) * (b.y - o.y) - (a.y - o.y) * (b.x - o.x);
  const lower: Point[] = [];
  for (const p of sorted) {
    while (
      lower.length >= 2 &&
      cross(lower[lower.length - 2], lower[lower.length - 1], p) <= 0
    ) {
      lower.pop();
    }
    lower.push(p);
  }
  const upper: Point[] = [];
  for (let i = sorted.length - 1; i >= 0; i--) {
    const p = sorted[i];
    while (
      upper.length >= 2 &&
      cross(upper[upper.length - 2], upper[upper.length - 1], p) <= 0
    ) {
      upper.pop();
    }
    upper.push(p);
  }
  lower.pop();
  upper.pop();
  return lower.concat(upper);
}

/**
 * How the user last reached for the page. A focus that a mouse press caused is
 * not a request to read the label: the pointer is already on the control and
 * hovering showed it. A focus that followed a key — Tab, or a roving arrow key
 * moving focus in script — is the only way a keyboard user sees it at all.
 */
let modality: 'keyboard' | 'pointer' = 'keyboard';
let listening = false;

export function watchModality() {
  if (listening || typeof window === 'undefined') {return;}
  listening = true;
  window.addEventListener('keydown', () => (modality = 'keyboard'), true);
  window.addEventListener('pointerdown', () => (modality = 'pointer'), true);
}

export function focusFromKeyboard() {
  return modality === 'keyboard';
}
