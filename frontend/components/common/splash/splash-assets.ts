/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

/**
 * 开屏静态资源：光斑贴图、字体与品牌图形加载、品牌排版与粒子目标采样。
 * Splash assets: glow sprites, font and brand-mark loading, brand layout and
 * particle-target sampling.
 */

import {createRandom} from './splash-timeline';

/** 品牌字体（Michroma）/ Brand display font (Michroma). */
export const BRAND_FONT_FAMILY = '"Michroma"';
/** 中文欢迎语字体 / Chinese greeting font. */
export const GREETING_FONT_FAMILY = '"Noto Serif SC Variable"';
/** 品牌图形资源 / Brand mark asset. */
export const BRAND_MARK_SRC = '/brand/stx-mark.png';

/** 品牌图层：清晰位图 + 粒子目标点 / Brand layer: crisp bitmap + particle targets. */
export interface BrandArt {
  crisp: HTMLCanvasElement;
  /** [x, y, r, g, b] × count */
  targets: Float32Array;
  count: number;
  bounds: {x: number; y: number; w: number; h: number};
}

function makeSprite(
  size: number,
  paint: (ctx: CanvasRenderingContext2D, size: number) => void,
): HTMLCanvasElement {
  const canvas = document.createElement('canvas');
  canvas.width = size;
  canvas.height = size;
  const ctx = canvas.getContext('2d');
  if (ctx) {
    paint(ctx, size);
  }
  return canvas;
}

/** 暖白核心 + 冷色外晕的光斑贴图 / Glow sprite: warm-white core with a cool halo. */
export function createGlowSprite(): HTMLCanvasElement {
  return makeSprite(128, (ctx, s) => {
    const g = ctx.createRadialGradient(s / 2, s / 2, 0, s / 2, s / 2, s / 2);
    g.addColorStop(0, 'rgba(255,250,242,0.9)');
    g.addColorStop(0.06, 'rgba(255,226,196,0.5)');
    g.addColorStop(0.2, 'rgba(255,170,120,0.16)');
    g.addColorStop(0.5, 'rgba(120,150,255,0.05)');
    g.addColorStop(1, 'rgba(0,0,0,0)');
    ctx.fillStyle = g;
    ctx.fillRect(0, 0, s, s);
  });
}

/** 细长光芒贴图（水平）/ Thin horizontal flare sprite. */
export function createFlareSprite(): HTMLCanvasElement {
  const canvas = document.createElement('canvas');
  canvas.width = 256;
  canvas.height = 6;
  const ctx = canvas.getContext('2d');
  if (ctx) {
    const g = ctx.createLinearGradient(0, 0, 256, 0);
    g.addColorStop(0, 'rgba(160,190,255,0)');
    g.addColorStop(0.42, 'rgba(255,236,220,0.55)');
    g.addColorStop(0.5, 'rgba(255,255,255,1)');
    g.addColorStop(0.58, 'rgba(255,236,220,0.55)');
    g.addColorStop(1, 'rgba(160,190,255,0)');
    ctx.fillStyle = g;
    ctx.fillRect(0, 2, 256, 2);
    ctx.globalAlpha = 0.35;
    ctx.fillRect(0, 1, 256, 4);
  }
  return canvas;
}

/** 等待字体，超时也继续（字体只影响观感，不阻塞动画）/ Await a font; continue on timeout. */
export async function loadFont(spec: string, text: string, timeoutMs: number) {
  if (typeof document === 'undefined' || !document.fonts?.load) {
    return;
  }
  await Promise.race([
    document.fonts.load(spec, text).catch(() => undefined),
    new Promise((resolve) => window.setTimeout(resolve, timeoutMs)),
  ]);
}

/** 加载品牌图形，失败返回 null（只聚合文字）/ Load the brand mark; null means text only. */
export async function loadMark(): Promise<HTMLImageElement | null> {
  const img = new Image();
  img.decoding = 'async';
  img.src = BRAND_MARK_SRC;
  try {
    await img.decode();
    return img;
  } catch {
    return null;
  }
}

/**
 * 排版并绘制“LOGO + STX”，同时采样粒子目标点。
 * 清晰位图与采样共用一套坐标，保证粒子落点与最终品牌严丝合缝。
 * Lay out and render "LOGO + STX" and sample particle targets. The crisp
 * bitmap and the sampling share one coordinate system, so particles land
 * exactly on the final brand.
 */
export function buildBrandArt(
  width: number,
  height: number,
  dpr: number,
  mark: HTMLImageElement | null,
  maxParticles: number,
): BrandArt {
  const probe = document.createElement('canvas').getContext('2d');
  if (!probe) {
    throw new Error('2d context unavailable');
  }

  // 以 100 为单位排版，再整体缩放到视口 / Lay out in 100-unit space, then scale.
  const unit = 100;
  const unitFont = unit * 0.8;
  probe.font = `400 ${unitFont}px ${BRAND_FONT_FAMILY}, sans-serif`;
  const letters = ['S', 'T', 'X'];
  const letterWidths = letters.map((l) => probe.measureText(l).width);
  const unitSpacing = unitFont * 0.16;
  const unitTextW =
    letterWidths.reduce((a, b) => a + b, 0) +
    unitSpacing * (letters.length - 1);
  const metrics = probe.measureText('STX');
  const unitAscent = metrics.actualBoundingBoxAscent || unitFont * 0.72;
  const unitDescent = metrics.actualBoundingBoxDescent || 0;
  const markAspect = mark ? mark.naturalWidth / mark.naturalHeight : 0;
  const unitMarkH = mark ? unit * 1.08 : 0;
  const unitMarkW = unitMarkH * markAspect;
  const unitGap = mark ? unit * 0.36 : 0;
  const unitW = unitMarkW + unitGap + unitTextW;
  const unitH = Math.max(unitMarkH, unitAscent + unitDescent);

  const scale = Math.min(
    Math.min(width * 0.76, 640) / unitW,
    (height * 0.19) / unitH,
  );
  const brandW = unitW * scale;
  const brandH = unitH * scale;
  const x0 = (width - brandW) / 2;
  const cy = height * 0.45;
  const fontPx = unitFont * scale;
  const spacing = unitSpacing * scale;
  const baseline = cy + ((unitAscent - unitDescent) * scale) / 2;
  const textX = x0 + (unitMarkW + unitGap) * scale;

  const paint = (ctx: CanvasRenderingContext2D, withGlow: boolean) => {
    if (mark) {
      const mh = unitMarkH * scale;
      const mw = unitMarkW * scale;
      if (withGlow) {
        ctx.shadowColor = 'rgba(70,170,255,0.65)';
        ctx.shadowBlur = 26 * scale;
      }
      ctx.drawImage(mark, x0, cy - mh / 2, mw, mh);
    }
    ctx.font = `400 ${fontPx}px ${BRAND_FONT_FAMILY}, sans-serif`;
    ctx.textBaseline = 'alphabetic';
    // 单色银白金属质感：上亮下灰，中线一道“地平线”折光，不引入额外色相。
    // Single-tone brushed silver: bright top, cooler lower half with a crisp
    // horizon break in the middle; no extra hues.
    const capTop = baseline - unitAscent * scale;
    const capBottom = baseline + unitDescent * scale;
    const grad = ctx.createLinearGradient(0, capTop, 0, capBottom);
    grad.addColorStop(0, '#ffffff');
    grad.addColorStop(0.46, '#eef0f3');
    grad.addColorStop(0.54, '#bfc4cc');
    grad.addColorStop(1, '#e4e7eb');
    ctx.fillStyle = grad;
    if (withGlow) {
      ctx.shadowColor = 'rgba(255,255,255,0.22)';
      ctx.shadowBlur = 20 * scale;
    }
    let x = textX;
    letters.forEach((letter, i) => {
      ctx.fillText(letter, x, baseline);
      x += letterWidths[i] * scale + spacing;
    });
    ctx.shadowBlur = 0;
    ctx.shadowColor = 'transparent';
  };

  // 清晰品牌层（含柔光）/ Crisp brand layer with soft glow.
  const crisp = document.createElement('canvas');
  crisp.width = Math.max(1, Math.round(width * dpr));
  crisp.height = Math.max(1, Math.round(height * dpr));
  const crispCtx = crisp.getContext('2d');
  if (crispCtx) {
    crispCtx.scale(dpr, dpr);
    paint(crispCtx, true);
    paint(crispCtx, false);
  }

  // 采样层：只覆盖品牌包围盒，1x 无光晕 / Sampling layer: brand bbox only, 1x, no glow.
  const pad = 4;
  const sx = Math.floor(x0) - pad;
  const sy = Math.floor(cy - brandH / 2) - pad;
  const sw = Math.ceil(brandW) + pad * 2;
  const sh = Math.ceil(brandH) + pad * 2;
  const sample = document.createElement('canvas');
  sample.width = Math.max(1, sw);
  sample.height = Math.max(1, sh);
  const sampleCtx = sample.getContext('2d', {willReadFrequently: true});
  const targets: number[] = [];
  if (sampleCtx) {
    sampleCtx.translate(-sx, -sy);
    paint(sampleCtx, false);
    const data = sampleCtx.getImageData(0, 0, sample.width, sample.height).data;
    let filled = 0;
    for (let i = 3; i < data.length; i += 4) {
      if (data[i] > 120) {
        filled += 1;
      }
    }
    // 按填充面积反推采样步长，控制粒子总数 / Derive the step from filled area to cap the count.
    const step = Math.max(2, Math.ceil(Math.sqrt(filled / maxParticles)));
    const rand = createRandom(7);
    for (let y = 0; y < sample.height; y += step) {
      for (let x = 0; x < sample.width; x += step) {
        const jx = Math.min(
          sample.width - 1,
          Math.max(0, Math.round(x + (rand() - 0.5) * step * 0.7)),
        );
        const jy = Math.min(
          sample.height - 1,
          Math.max(0, Math.round(y + (rand() - 0.5) * step * 0.7)),
        );
        const idx = (jy * sample.width + jx) * 4;
        if (data[idx + 3] > 120) {
          targets.push(
            jx + sx,
            jy + sy,
            data[idx],
            data[idx + 1],
            data[idx + 2],
          );
        }
      }
    }
  }

  return {
    crisp,
    targets: new Float32Array(targets),
    count: targets.length / 5,
    bounds: {x: x0, y: cy - brandH / 2, w: brandW, h: brandH},
  };
}
