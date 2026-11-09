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
 * 开屏动画导演：用一条 requestAnimationFrame 时间轴驱动全部画面。
 * Splash director: one requestAnimationFrame timeline drives every visual.
 *
 * 图层（自下而上）/ Layers (bottom → top):
 *   1. CSS 深空底色 / CSS deep-space backdrop
 *   2. WebGL 奇点漩涡（坍缩瞬间截图后隐藏）/ WebGL vortex (snapshotted then hidden at collapse)
 *   3. 2D 特效画布：坍缩螺旋、冲击波、流星、粒子、品牌 / 2D FX canvas: collapse spiral, shockwave, meteor, particles, brand
 *   4. DOM：欢迎语与跳过按钮（由 React 渲染，这里只切换 data 属性）
 *      DOM: greeting and skip button (rendered by React; we only toggle data attributes)
 *
 * 逐帧不触发 React 渲染；时间按帧累加并限制单帧步长，标签页切到后台时动画暂停而不是跳帧。
 * No React renders per frame. Time accumulates per frame with a clamped step,
 * so a backgrounded tab pauses the show instead of skipping it.
 */

import {
  REDUCED_MOTION_TIMELINE,
  SPLASH_TIMELINE,
  buildDivePath,
  clamp01,
  createRandom,
  diveParam,
  easeInOutCubic,
  easeInOutQuart,
  easeOutBack,
  easeOutCubic,
  lerp,
  progress,
  quadBezier,
  type Point,
} from './splash-timeline';
import {SingularityRenderer} from './singularity-renderer';
import {
  BRAND_FONT_FAMILY,
  GREETING_FONT_FAMILY,
  buildBrandArt,
  createFlareSprite,
  createGlowSprite,
  loadFont,
  loadMark,
  type BrandArt,
} from './splash-assets';

/** 视差在着色器 p 空间的最大偏移 / Max parallax offset in shader p-space. */
const PARALLAX_STRENGTH = 0.11;
/** p 空间单位 → 像素（× 视口高）/ p-space unit → pixels (× viewport height). */
const P_TO_PX = 0.35;

const GATHER_DURATION = 920;
const STAR_TAIL_MS = 300;
const SPARK_CAPACITY = 360;

/**
 * 穿心顿帧：撞击后这段时间轴时长以慢速播放，制造“定格一瞬再坍缩”的力量感。
 * Impact hit-stop: this slice of timeline after impact plays in slow motion,
 * giving a beat of stillness before the collapse.
 */
const HITSTOP_SPAN = 70;
const HITSTOP_SCALE = 0.2;
/** 撞击前内爆收缩时长 / Implosion lead-in before impact. */
const IMPLODE_MS = 420;

/** 流星尾迹色带：白热 → 银白 → 冷灰 / Meteor tail ramp: white-hot → silver → cool grey. */
const TAIL_RAMP: Array<[number, number, number]> = [
  [255, 255, 255],
  [236, 240, 246],
  [196, 204, 216],
  [140, 150, 168],
];

/** 单色粒子色阶（无 WebGL 截图时取用）/ Monochrome particle tones (used without a snapshot). */
const VORTEX_PALETTE: Array<[number, number, number]> = [
  [255, 255, 255],
  [236, 240, 246],
  [214, 220, 230],
  [188, 196, 210],
];

export interface SplashDirectorOptions {
  root: HTMLElement;
  glCanvas: HTMLCanvasElement;
  fxCanvas: HTMLCanvasElement;
  /** WebGL 不可用时的 CSS 漩涡 / CSS vortex used when WebGL is unavailable. */
  fallbackVortex: HTMLElement;
  welcome: HTMLElement;
  reducedMotion: boolean;
  /** 开始退场（此时可撤掉首帧黑色遮罩）/ Exit begins (pre-paint cover may be released). */
  onExitStart: () => void;
  /** 动画完全结束 / Splash fully finished. */
  onDone: () => void;
}

/** 粒子场（结构化数组，避免逐帧分配）/ Particle field (struct-of-arrays, no per-frame allocation). */
interface ParticleField {
  n: number;
  ox: Float32Array;
  oy: Float32Array;
  vx: Float32Array;
  vy: Float32Array;
  swirl: Float32Array;
  tx: Float32Array;
  ty: Float32Array;
  /** 涌出延迟（毫秒）：粒子错峰从奇点喷出 / Emission delay in ms; particles pour out staggered. */
  emit: Float32Array;
  delay: Float32Array;
  curl: Float32Array;
  size: Float32Array;
  phase: Float32Array;
  /** 起始色与目标色 / Start and target colors. */
  c0: Uint8ClampedArray;
  c1: Uint8ClampedArray;
  /** 鼠标斥力形成的偏移与速度 / Pointer repulsion offset and velocity. */
  offX: Float32Array;
  offY: Float32Array;
  velX: Float32Array;
  velY: Float32Array;
  glow: Uint8Array;
}

function sampleRamp(t: number): [number, number, number] {
  const x = clamp01(t) * (TAIL_RAMP.length - 1);
  const i = Math.min(TAIL_RAMP.length - 2, Math.floor(x));
  const f = x - i;
  const a = TAIL_RAMP[i];
  const b = TAIL_RAMP[i + 1];
  return [lerp(a[0], b[0], f), lerp(a[1], b[1], f), lerp(a[2], b[2], f)];
}

export class SplashDirector {
  private readonly opts: SplashDirectorOptions;
  private readonly ctx: CanvasRenderingContext2D | null;
  private renderer: SingularityRenderer | null = null;
  private readonly glowSprite: HTMLCanvasElement;
  private readonly flareSprite: HTMLCanvasElement;
  private readonly colorCache = new Map<number, string>();

  private width = 1;
  private height = 1;
  private dpr = 1;

  private raf = 0;
  private lastNow = 0;
  /** 时间轴时间（毫秒）/ Timeline time in ms. */
  private t = 0;
  private shaderTime = 0;
  private disposed = false;

  private offset: [number, number] = [0, 0];
  private offsetTarget: [number, number] = [0, 0];
  private pointer = {x: -9999, y: -9999, active: false};

  private divePath: {a: Point; c: Point; b: Point} | null = null;
  private glideStart: Point | null = null;

  private mark: HTMLImageElement | null = null;
  private brand: BrandArt | null = null;
  private assetsReady = false;

  private collapsed = false;
  private impactCenter: Point = {x: 0, y: 0};
  private snapshot: HTMLCanvasElement | null = null;
  private particles: ParticleField | null = null;

  private readonly sparks = {
    x: new Float32Array(SPARK_CAPACITY),
    y: new Float32Array(SPARK_CAPACITY),
    vx: new Float32Array(SPARK_CAPACITY),
    vy: new Float32Array(SPARK_CAPACITY),
    born: new Float32Array(SPARK_CAPACITY).fill(-1e9),
    life: new Float32Array(SPARK_CAPACITY),
    head: 0,
  };
  private readonly rand = createRandom(20251107);

  private welcomeShown = false;
  private exitAt: number;
  private exitDuration: number;
  private exiting = false;
  private finished = false;

  constructor(opts: SplashDirectorOptions) {
    this.opts = opts;
    this.ctx = opts.fxCanvas.getContext('2d');
    this.glowSprite = createGlowSprite();
    this.flareSprite = createFlareSprite();
    const timeline = opts.reducedMotion
      ? REDUCED_MOTION_TIMELINE
      : SPLASH_TIMELINE;
    this.exitAt = timeline.exit;
    this.exitDuration = timeline.exitDuration;

    if (!opts.reducedMotion) {
      try {
        this.renderer = new SingularityRenderer(opts.glCanvas);
      } catch {
        // 无 WebGL2：改用 CSS 漩涡，其余分镜照常 / No WebGL2: CSS vortex, rest unchanged.
        this.renderer = null;
        opts.root.dataset.vortex = 'css';
      }
    }
  }

  /** 启动：加载资源、绑定事件、开始逐帧 / Start: load assets, bind events, begin frames. */
  start(): void {
    this.resize();
    window.addEventListener('resize', this.resize);
    this.opts.root.addEventListener('pointermove', this.onPointerMove);
    this.opts.root.addEventListener('pointerleave', this.onPointerLeave);
    void this.prepareAssets();
    this.lastNow = performance.now();
    this.raf = requestAnimationFrame(this.frame);
  }

  /** 跳过：直接进入快速退场 / Skip: jump straight into a fast exit. */
  skip(): void {
    if (this.exiting) {
      return;
    }
    this.exitDuration = SPLASH_TIMELINE.skipExitDuration;
    this.beginExit();
  }

  dispose(): void {
    if (this.disposed) {
      return;
    }
    this.disposed = true;
    cancelAnimationFrame(this.raf);
    window.removeEventListener('resize', this.resize);
    this.opts.root.removeEventListener('pointermove', this.onPointerMove);
    this.opts.root.removeEventListener('pointerleave', this.onPointerLeave);
    this.renderer?.dispose();
    this.renderer = null;
  }

  private async prepareAssets(): Promise<void> {
    const [mark] = await Promise.all([
      loadMark(),
      loadFont(`400 80px ${BRAND_FONT_FAMILY}`, 'STX', 2200),
      // 欢迎语字体预热，避免出场时回退字体闪烁 / Warm the greeting font to avoid a fallback flash.
      loadFont(`300 40px ${GREETING_FONT_FAMILY}`, '欢迎回来', 2200),
    ]);
    if (this.disposed) {
      return;
    }
    this.mark = mark;
    this.assetsReady = true;
    if (!this.particles) {
      this.layoutBrand();
    }
  }

  private layoutBrand(): void {
    const maxParticles = this.width * this.height < 700_000 ? 1100 : 2100;
    try {
      this.brand = buildBrandArt(
        this.width,
        this.height,
        this.dpr,
        this.mark,
        maxParticles,
      );
    } catch {
      this.brand = null;
      return;
    }
    const {y, h} = this.brand.bounds;
    this.opts.welcome.style.top = `${Math.round(y + h + Math.max(26, this.height * 0.045))}px`;
  }

  private readonly resize = (): void => {
    const rect = this.opts.root.getBoundingClientRect();
    this.width = Math.max(1, rect.width);
    this.height = Math.max(1, rect.height);
    this.dpr = Math.min(window.devicePixelRatio || 1, 2);
    const fx = this.opts.fxCanvas;
    fx.width = Math.round(this.width * this.dpr);
    fx.height = Math.round(this.height * this.dpr);
    this.renderer?.resize(this.width, this.height);
    // 粒子尚未生成前，按新尺寸重排品牌 / Re-layout the brand until particles exist.
    if (this.assetsReady && !this.particles) {
      this.layoutBrand();
    }
  };

  private readonly onPointerMove = (event: PointerEvent): void => {
    this.pointer.x = event.clientX;
    this.pointer.y = event.clientY;
    this.pointer.active = true;
    this.offsetTarget = [
      ((event.clientX / this.width - 0.5) *
        2 *
        PARALLAX_STRENGTH *
        this.width) /
        this.height,
      -(event.clientY / this.height - 0.5) * 2 * PARALLAX_STRENGTH,
    ];
  };

  private readonly onPointerLeave = (): void => {
    this.pointer.active = false;
    this.offsetTarget = [0, 0];
  };

  /** 漩涡中心（含鼠标视差）/ Vortex center including pointer parallax. */
  private vortexCenter(): Point {
    return {
      x: this.width / 2 + this.offset[0] * P_TO_PX * this.height,
      y: this.height / 2 - this.offset[1] * P_TO_PX * this.height,
    };
  }

  /** 左上角驻点 / Top-left perch for the star. */
  private perch(): Point {
    const portrait = this.height > this.width;
    return {
      x: this.width * (portrait ? 0.16 : 0.13),
      y: this.height * (portrait ? 0.13 : 0.16),
    };
  }

  private beginExit(): void {
    if (this.exiting) {
      return;
    }
    this.exiting = true;
    this.exitAt = this.t;
    const {root} = this.opts;
    root.style.setProperty('--stx-splash-exit', `${this.exitDuration}ms`);
    root.dataset.exit = 'on';
    this.opts.onExitStart();
  }

  private readonly frame = (now: number): void => {
    if (this.disposed) {
      return;
    }
    // 单帧步长上限 50ms：后台标签页回来时不跳过分镜 / Cap step at 50ms so hidden tabs don't skip scenes.
    const raw = Math.min(50, Math.max(0, now - this.lastNow));
    this.lastNow = now;
    // 撞击后短暂慢放（顿帧）/ Brief slow motion right after impact (hit-stop).
    const impact = SPLASH_TIMELINE.impact;
    const inHitStop =
      !this.opts.reducedMotion &&
      this.t >= impact &&
      this.t < impact + HITSTOP_SPAN;
    const dt = inHitStop ? raw * HITSTOP_SCALE : raw;
    this.t += dt;

    if (this.opts.reducedMotion) {
      this.drawReduced();
    } else {
      this.update(dt);
      this.draw(dt);
    }

    const welcomeAt = this.opts.reducedMotion
      ? REDUCED_MOTION_TIMELINE.welcome
      : SPLASH_TIMELINE.welcome;
    if (!this.welcomeShown && this.t >= welcomeAt) {
      this.welcomeShown = true;
      this.opts.root.dataset.welcome = 'on';
    }
    if (!this.exiting && this.t >= this.exitAt) {
      this.beginExit();
    }
    if (this.exiting && this.t >= this.exitAt + this.exitDuration) {
      this.finish();
      return;
    }
    this.raf = requestAnimationFrame(this.frame);
  };

  private finish(): void {
    if (this.finished) {
      return;
    }
    this.finished = true;
    this.dispose();
    this.opts.onDone();
  }

  private update(dt: number): void {
    const T = SPLASH_TIMELINE;
    // 下坠开始后冻结视差，保证流星精确穿心 / Freeze parallax once the dive starts so the meteor hits dead center.
    if (this.t < T.dive[0]) {
      const k = 1 - Math.exp(-dt / 260);
      this.offset[0] += (this.offsetTarget[0] - this.offset[0]) * k;
      this.offset[1] += (this.offsetTarget[1] - this.offset[1]) * k;
    }
    if (!this.glideStart && this.t >= T.glide[0]) {
      this.glideStart = this.vortexCenter();
    }
    if (!this.divePath && this.t >= T.dive[0]) {
      this.divePath = buildDivePath(
        this.perch(),
        this.vortexCenter(),
        this.height * 0.06,
      );
    }
    // 漩涡随流星逼近而加速 / Vortex spins up as the meteor approaches.
    const spin = 1 + 2.4 * Math.pow(progress(this.t, T.charge[0], T.impact), 2);
    this.shaderTime += (dt / 1000) * spin;
  }

  /** 星光位置：纯函数式地由时间推出，尾迹可直接回溯 / Star position as a pure function of time (tail can rewind). */
  private starPosition(t: number): Point {
    const T = SPLASH_TIMELINE;
    if (t < T.glide[0] || !this.glideStart) {
      return this.vortexCenter();
    }
    const perch = this.perch();
    if (t < T.glide[1]) {
      const s = easeInOutCubic(progress(t, T.glide[0], T.glide[1]));
      const from = this.glideStart;
      // 先上扬再左掠的弧线 / Arc that rises first, then sweeps left.
      const ctrl = {
        x: from.x - (from.x - perch.x) * 0.22,
        y: perch.y - this.height * 0.05,
      };
      return quadBezier(from, ctrl, perch, s);
    }
    if (t < T.dive[0] || !this.divePath) {
      // 蓄力：沿反方向轻微后撤再回弹 / Charge: slight recoil away from the dive, then back.
      const p = progress(t, T.charge[0], T.charge[1]);
      const center = this.vortexCenter();
      const dx = perch.x - center.x;
      const dy = perch.y - center.y;
      const len = Math.hypot(dx, dy) || 1;
      const recoil = Math.sin(Math.PI * p) * 10;
      return {
        x: perch.x + (dx / len) * recoil,
        y: perch.y + (dy / len) * recoil,
      };
    }
    const {a, c, b} = this.divePath;
    return quadBezier(a, c, b, diveParam(t));
  }

  private draw(dt: number): void {
    const T = SPLASH_TIMELINE;
    const {ctx} = this;

    // —— 1. 漩涡 / Vortex ——
    const flash =
      this.t < T.impact
        ? Math.pow(progress(this.t, T.impact - 200, T.impact), 2)
        : 0;
    // 内爆：撞击前漩涡被吸向中心并增亮 / Implosion: vortex pulled inward and brightened before impact.
    const implode = Math.pow(
      progress(this.t, T.impact - IMPLODE_MS, T.impact),
      2.2,
    );
    const intensity =
      progress(this.t, 0, 520) *
      lerp(0.2, 1, easeInOutCubic(progress(this.t, 150, T.glide[1]))) *
      (1 + implode * 0.5);
    if (!this.collapsed) {
      if (this.renderer) {
        this.renderer.render({
          time: this.shaderTime,
          offset: this.offset,
          intensity,
          flash,
          zoom: 1 + implode * 0.55,
        });
      } else {
        const center = this.vortexCenter();
        const style = this.opts.fallbackVortex.style;
        style.opacity = String(intensity);
        style.transform = `translate(${center.x - this.width / 2}px, ${center.y - this.height / 2}px)`;
      }
      if (this.t >= T.impact) {
        this.beginCollapse();
      }
    }

    if (!ctx) {
      return;
    }
    ctx.setTransform(this.dpr, 0, 0, this.dpr, 0, 0);
    ctx.clearRect(0, 0, this.width, this.height);

    // —— 2. 坍缩吸入与奇点释放 / Collapse spiral and singularity release ——
    if (this.collapsed) {
      this.drawCollapse(ctx);
      this.drawRelease(ctx);
    }

    // —— 3. 品牌清晰层 / Crisp brand ——
    const brandAlpha = easeOutCubic(progress(this.t, T.brand[0], T.brand[1]));
    if (this.brand && brandAlpha > 0) {
      ctx.globalCompositeOperation = 'source-over';
      ctx.globalAlpha = brandAlpha * (0.94 + 0.06 * Math.sin(this.t / 520));
      ctx.drawImage(this.brand.crisp, 0, 0, this.width, this.height);
      ctx.globalAlpha = 1;
    }

    // —— 4. 粒子、火花、流星（叠加混合）/ Particles, sparks, meteor (additive) ——
    ctx.globalCompositeOperation = 'lighter';
    if (this.particles) {
      this.drawParticles(ctx, dt, brandAlpha);
    }
    this.emitSparks(dt);
    this.drawSparks(ctx);
    this.drawStar(ctx);

    // —— 5. 释放白屏过曝 / Release white-out ——
    if (this.collapsed) {
      // 径向过曝：只在释放后 150ms 内，中心炽白、边缘迅速衰减，避免整屏发灰。
      // Radial overexposure: only in the 150ms after release — blazing core,
      // fast falloff, so the screen never turns flat grey.
      const since = this.t - T.release;
      const whiteout = since >= 0 ? Math.pow(1 - progress(since, 0, 150), 2) : 0;
      if (whiteout > 0) {
        const {x, y} = this.impactCenter;
        const radius = Math.hypot(this.width, this.height) * 0.45;
        const g = ctx.createRadialGradient(x, y, 0, x, y, radius);
        g.addColorStop(0, 'rgba(255,255,255,1)');
        g.addColorStop(0.18, 'rgba(255,255,255,0.85)');
        g.addColorStop(0.5, 'rgba(255,255,255,0.25)');
        g.addColorStop(1, 'rgba(255,255,255,0)');
        ctx.globalCompositeOperation = 'lighter';
        ctx.globalAlpha = whiteout;
        ctx.fillStyle = g;
        ctx.fillRect(0, 0, this.width, this.height);
      }
    }
    ctx.globalCompositeOperation = 'source-over';
    ctx.globalAlpha = 1;
  }

  /** 减弱动态：只渐显品牌 / Reduced motion: brand fade only. */
  private drawReduced(): void {
    const {ctx} = this;
    if (!ctx) {
      return;
    }
    ctx.setTransform(this.dpr, 0, 0, this.dpr, 0, 0);
    ctx.clearRect(0, 0, this.width, this.height);
    const [s, e] = REDUCED_MOTION_TIMELINE.brand;
    const alpha = easeOutCubic(progress(this.t, s, e));
    if (this.brand && alpha > 0) {
      ctx.globalAlpha = alpha;
      ctx.drawImage(this.brand.crisp, 0, 0, this.width, this.height);
      ctx.globalAlpha = 1;
    }
  }

  /**
   * 四芒星：按真实光学星芒分层绘制——大范围柔光、紧致辉光、四道由亮到透明渐隐的主芒、
   * 贯穿的衍射细线、45° 次级芒、白热核心。每道主芒长度独立闪烁，像大气中的星光。
   * Four-point star drawn in optical layers: wide bloom, tight glow, four main
   * spikes fading from white to transparent, through-going diffraction
   * hairlines, 45° secondary spikes, and a white-hot core. Each main spike
   * flickers independently, like starlight through atmosphere.
   */
  private drawSparkle(
    ctx: CanvasRenderingContext2D,
    x: number,
    y: number,
    r: number,
    rot: number,
    alpha: number,
    stretch: number,
  ): void {
    if (alpha <= 0.002 || r <= 0.1) {
      return;
    }
    const t = this.t;
    ctx.save();
    ctx.translate(x, y);
    ctx.rotate(rot);
    ctx.scale(stretch, 1);

    // 1. 大范围柔光 + 紧致辉光 / Wide bloom plus tight glow.
    ctx.globalAlpha = alpha * 0.32;
    ctx.drawImage(this.glowSprite, -r * 4, -r * 4, r * 8, r * 8);
    ctx.globalAlpha = alpha * 0.85;
    ctx.drawImage(this.glowSprite, -r * 1.5, -r * 1.5, r * 3, r * 3);

    // 2. 衍射细线：贯穿整星的发丝光 / Diffraction hairlines through the whole star.
    ctx.globalAlpha = alpha * 0.6;
    ctx.drawImage(this.flareSprite, -r * 3.4, -1.2, r * 6.8, 2.4);
    ctx.save();
    ctx.rotate(Math.PI / 2);
    ctx.drawImage(this.flareSprite, -r * 2.8, -1.1, r * 5.6, 2.2);
    ctx.restore();

    // 3. 四道主芒：根部宽、尖端细，亮度沿长度渐隐 / Main spikes: wide root, needle tip, fading along length.
    const drawSpike = (len: number, width: number, a: number) => {
      const g = ctx.createLinearGradient(0, 0, len, 0);
      g.addColorStop(0, 'rgba(255,255,255,1)');
      g.addColorStop(0.35, 'rgba(250,252,255,0.85)');
      g.addColorStop(1, 'rgba(235,242,255,0)');
      ctx.globalAlpha = a;
      ctx.fillStyle = g;
      ctx.beginPath();
      ctx.moveTo(0, -width);
      ctx.quadraticCurveTo(len * 0.18, -width * 0.18, len, 0);
      ctx.quadraticCurveTo(len * 0.18, width * 0.18, 0, width);
      ctx.closePath();
      ctx.fill();
    };
    for (let i = 0; i < 4; i += 1) {
      const flicker =
        1 +
        0.09 * Math.sin(t / 53 + i * 1.7) +
        0.05 * Math.sin(t / 19 + i * 2.9);
      ctx.save();
      ctx.rotate((Math.PI / 2) * i);
      drawSpike(r * 1.25 * flicker, r * 0.13, alpha);
      ctx.restore();
    }

    // 4. 45° 次级芒 / Secondary 45° spikes.
    for (let i = 0; i < 4; i += 1) {
      ctx.save();
      ctx.rotate(Math.PI / 4 + (Math.PI / 2) * i);
      drawSpike(
        r * 0.5 * (1 + 0.12 * Math.sin(t / 41 + i)),
        r * 0.06,
        alpha * 0.55,
      );
      ctx.restore();
    }

    // 5. 白热核心 / White-hot core.
    const core = ctx.createRadialGradient(0, 0, 0, 0, 0, r * 0.32);
    core.addColorStop(0, 'rgba(255,255,255,1)');
    core.addColorStop(0.45, 'rgba(255,255,255,0.9)');
    core.addColorStop(1, 'rgba(240,245,255,0)');
    ctx.globalAlpha = alpha;
    ctx.fillStyle = core;
    ctx.beginPath();
    ctx.arc(0, 0, r * 0.32, 0, Math.PI * 2);
    ctx.fill();

    // 6. 极淡的镜头光环，增加光学质感；高速拉伸时隐藏，避免变成椭圆。
    // A faint lens ring for optical texture; hidden when stretched at speed so
    // it never turns into an ellipse.
    const ringAlpha = alpha * 0.07 * clamp01(1 - (stretch - 1) * 6);
    if (ringAlpha > 0.003) {
      ctx.globalAlpha = ringAlpha;
      ctx.strokeStyle = '#ffffff';
      ctx.lineWidth = 1;
      ctx.beginPath();
      ctx.arc(0, 0, r * 1.9, 0, Math.PI * 2);
      ctx.stroke();
    }
    ctx.restore();
  }

  private drawStar(ctx: CanvasRenderingContext2D): void {
    const T = SPLASH_TIMELINE;
    const t = this.t;
    if (t > T.dive[1]) {
      return;
    }
    const base = Math.min(34, Math.max(18, this.height * 0.034));
    const pos = this.starPosition(t);
    const twinkle = 1 + 0.1 * Math.sin(t / 70) + 0.05 * Math.sin(t / 23);

    let radius = base;
    let alpha = 1;
    let rot = t * 0.0009;
    let stretch = 1;

    if (t < T.glide[0]) {
      const p = progress(t, 0, T.starIn[1]);
      radius = base * easeOutBack(p) * twinkle;
      alpha = easeOutCubic(progress(t, 0, 420));
      rot += (1 - easeOutCubic(p)) * Math.PI * 0.75;
    } else if (t < T.glide[1]) {
      radius =
        base *
        (1 - 0.14 * easeInOutCubic(progress(t, T.glide[0], T.glide[1]))) *
        twinkle;
    } else if (t < T.dive[0]) {
      const p = progress(t, T.charge[0], T.charge[1]);
      radius = base * (0.86 + 0.42 * Math.sin(Math.PI * p)) * twinkle;
    } else {
      // 下坠：沿速度方向拉长，穿心后逐渐熄灭 / Dive: stretch along velocity, fade after impact.
      const ahead = this.starPosition(t + 16);
      const vx = ahead.x - pos.x;
      const vy = ahead.y - pos.y;
      const speed = Math.hypot(vx, vy);
      rot = Math.atan2(vy, vx);
      stretch = 1 + Math.min(1.6, speed / 22);
      radius = base * (0.9 + 0.35 * progress(t, T.dive[0], T.impact)) * twinkle;
      alpha = 1 - easeInOutCubic(progress(t, T.impact + 60, T.dive[1]));
    }

    this.drawTail(ctx, t, base, alpha);
    this.drawSparkle(ctx, pos.x, pos.y, radius, rot, alpha, stretch);
  }

  /** 流星尾迹：沿解析路径回溯，帧率无关 / Meteor tail rewinds the analytic path, frame-rate independent. */
  private drawTail(
    ctx: CanvasRenderingContext2D,
    t: number,
    base: number,
    alpha: number,
  ): void {
    const T = SPLASH_TIMELINE;
    if (t < T.glide[0] || alpha <= 0.01) {
      return;
    }
    const tailMs = t < T.dive[0] ? STAR_TAIL_MS * 0.5 : STAR_TAIL_MS;
    const segments = 40;
    let prev = this.starPosition(t);
    // 平头端点：圆头在叠加混合下会在接缝处叠亮成“珠串” / Butt caps: round caps double up into beads under additive blending.
    ctx.lineCap = 'butt';
    for (let i = 1; i <= segments; i += 1) {
      const age = (i / segments) * tailMs;
      const tt = Math.max(T.glide[0], t - age);
      const p = this.starPosition(tt);
      const k = i / segments;
      const step = Math.hypot(p.x - prev.x, p.y - prev.y);
      // 按段长（即速度）衰减：星光静止时尾迹为零，不会叠成一团白斑。
      // Fade by segment length (speed): a resting star has no tail, so
      // segments never pile up into a white blob.
      const speedFade = clamp01((step - 0.6) / 4);
      const fade = Math.pow(1 - k, 1.6) * alpha * speedFade;
      if (fade > 0.01) {
        const [r, g, b] = sampleRamp(k);
        const width = base * 0.36 * Math.pow(1 - k, 1.2) + 0.5;
        ctx.globalAlpha = fade * 0.14;
        ctx.strokeStyle = `rgb(${r | 0},${g | 0},${b | 0})`;
        ctx.lineWidth = width * 3;
        ctx.beginPath();
        ctx.moveTo(prev.x, prev.y);
        ctx.lineTo(p.x, p.y);
        ctx.stroke();
        ctx.globalAlpha = fade * 0.9;
        ctx.lineWidth = width;
        ctx.stroke();
      }
      prev = p;
      if (tt === T.glide[0]) {
        break;
      }
    }
    ctx.globalAlpha = 1;
  }

  /** 下坠中沿途抛洒火花 / Shed sparks along the dive. */
  private emitSparks(dt: number): void {
    const T = SPLASH_TIMELINE;
    const t = this.t;
    if (t < T.dive[0] || t > T.dive[1] || dt <= 0) {
      return;
    }
    const pos = this.starPosition(t);
    const ahead = this.starPosition(t + 16);
    const vx = (ahead.x - pos.x) / 16;
    const vy = (ahead.y - pos.y) / 16;
    const count = Math.min(6, 1 + Math.round(Math.hypot(vx, vy) * 2));
    const s = this.sparks;
    for (let i = 0; i < count; i += 1) {
      const j = s.head;
      s.head = (s.head + 1) % SPARK_CAPACITY;
      s.x[j] = pos.x;
      s.y[j] = pos.y;
      const spread = 0.05 + this.rand() * 0.12;
      const angle = this.rand() * Math.PI * 2;
      s.vx[j] = -vx * 0.18 + Math.cos(angle) * spread;
      s.vy[j] = -vy * 0.18 + Math.sin(angle) * spread + 0.02;
      s.born[j] = t;
      s.life[j] = 380 + this.rand() * 420;
    }
  }

  private drawSparks(ctx: CanvasRenderingContext2D): void {
    const s = this.sparks;
    ctx.fillStyle = 'rgb(236,240,246)';
    for (let i = 0; i < SPARK_CAPACITY; i += 1) {
      const age = this.t - s.born[i];
      if (age < 0 || age > s.life[i]) {
        continue;
      }
      const k = age / s.life[i];
      const x = s.x[i] + s.vx[i] * age;
      const y = s.y[i] + s.vy[i] * age + 0.00006 * age * age;
      ctx.globalAlpha = (1 - k) * (1 - k) * 0.9;
      const size = 1.6 * (1 - k) + 0.4;
      ctx.fillRect(x - size / 2, y - size / 2, size, size);
    }
    ctx.globalAlpha = 1;
  }

  /**
   * 穿心瞬间：同帧截取漩涡画面（供坍缩螺旋与粒子取色），随后进入内爆。
   * Impact: snapshot the vortex in the same frame (for the collapse spiral
   * and particle tinting), then begin the implosion.
   */
  private beginCollapse(): void {
    this.collapsed = true;
    this.impactCenter = this.vortexCenter();
    this.opts.root.dataset.collapsed = 'on';

    if (this.renderer) {
      const src = this.opts.glCanvas;
      const snap = document.createElement('canvas');
      snap.width = src.width;
      snap.height = src.height;
      const snapCtx = snap.getContext('2d', {willReadFrequently: true});
      if (snapCtx) {
        // 必须与 render() 在同一回调中执行，绘制缓冲尚未被清空。
        // Must run in the same callback as render(), before the buffer is cleared.
        snapCtx.drawImage(src, 0, 0);
        // 坍缩素材加径向羽化：中心清晰、边缘渐隐，螺旋吸入时不露矩形边。
        // Radial-feather the collapse sprite: crisp center, fading edge, so
        // the spinning-in frame never shows a rectangular border.
        const sx = snap.width / this.width;
        const sy = snap.height / this.height;
        const {x: cx, y: cy} = this.impactCenter;
        const feather = Math.min(this.width, this.height);
        const g = snapCtx.createRadialGradient(
          cx * sx,
          cy * sy,
          feather * 0.28 * sx,
          cx * sx,
          cy * sy,
          feather * 0.5 * sx,
        );
        g.addColorStop(0, 'rgba(255,255,255,1)');
        g.addColorStop(1, 'rgba(255,255,255,0)');
        snapCtx.globalCompositeOperation = 'destination-in';
        snapCtx.fillStyle = g;
        snapCtx.fillRect(0, 0, snap.width, snap.height);
        this.snapshot = snap;
      }
      this.renderer.dispose();
      this.renderer = null;
    }

    if (!this.brand) {
      this.layoutBrand();
    }
    this.particles = this.buildParticles();
  }

  /**
   * 内爆：整幅漩涡截图被加速吸入奇点——收缩、顺漩涡方向旋转、末尾隐没；
   * 同时奇点急速增亮，衔接到释放闪光。
   * Implosion: the vortex snapshot is sucked into the point — shrinking and
   * spinning faster along the vortex direction, vanishing at the end —
   * while the core flares into the release flash.
   */
  private drawCollapse(ctx: CanvasRenderingContext2D): void {
    const T = SPLASH_TIMELINE;
    const k = progress(this.t, T.collapse[0], T.collapse[1]);
    if (k >= 1) {
      return;
    }
    const {x: cx, y: cy} = this.impactCenter;
    const snap = this.snapshot;

    if (snap) {
      // 加速吸入：越靠近奇点缩得越快、转得越急。
      // Accelerating pull: shrinks and spins faster the closer it gets.
      const s = Math.pow(1 - k, 1.6);
      const rot = Math.pow(k, 1.5) * Math.PI * -2.6;
      ctx.globalCompositeOperation = 'source-over';
      ctx.save();
      ctx.translate(cx, cy);
      ctx.rotate(rot);
      ctx.scale(s, s);
      ctx.globalAlpha = 1 - progress(k, 0.72, 1);
      ctx.drawImage(snap, -cx, -cy, this.width, this.height);
      ctx.restore();
      ctx.globalAlpha = 1;
    }

    // 奇点增亮：坍缩后段中心点急速变亮。
    // Core flare: the point brightens rapidly over the back half of collapse.
    const core = progress(
      this.t,
      T.collapse[0] + (T.collapse[1] - T.collapse[0]) * 0.45,
      T.collapse[1],
    );
    if (core > 0) {
      ctx.globalCompositeOperation = 'lighter';
      const r = this.height * (0.02 + 0.16 * easeOutCubic(core));
      ctx.globalAlpha = Math.min(1, core * 1.3);
      ctx.drawImage(this.glowSprite, cx - r, cy - r, r * 2, r * 2);
      const hot = ctx.createRadialGradient(cx, cy, 0, cx, cy, r * 0.35);
      hot.addColorStop(0, 'rgba(255,255,255,1)');
      hot.addColorStop(1, 'rgba(255,255,255,0)');
      ctx.globalAlpha = Math.min(1, core * 1.8);
      ctx.fillStyle = hot;
      ctx.beginPath();
      ctx.arc(cx, cy, r * 0.35, 0, Math.PI * 2);
      ctx.fill();
      ctx.globalAlpha = 1;
      ctx.globalCompositeOperation = 'source-over';
    }
  }

  /**
   * 释放：核心闪光加一道冲击波环。爆点只押一个放射元素——克制即是力量。
   * Release: a core flash plus one shockwave ring. A single radial element —
   * restraint is the power.
   */
  private drawRelease(ctx: CanvasRenderingContext2D): void {
    const since = this.t - SPLASH_TIMELINE.release;
    if (since < 0) {
      return;
    }
    const {x, y} = this.impactCenter;
    const diag = Math.hypot(this.width, this.height);
    ctx.globalCompositeOperation = 'lighter';

    // 核心闪光：炽白余晖随半径扩张衰减 / Core flash: decays as it expands.
    const flash = 1 - progress(since, 0, 460);
    if (flash > 0) {
      const r = this.height * (0.32 + 0.5 * (1 - flash));
      ctx.globalAlpha = flash * flash;
      ctx.drawImage(this.glowSprite, x - r, y - r, r * 2, r * 2);
    }

    // 单道冲击波：三层叠描，外层宽而淡做柔光，内层细而亮做锋线。
    // One ring in three passes: wide faint glow outside, thin bright edge inside.
    const k = progress(since, 0, 950);
    if (k > 0 && k < 1) {
      const r = easeOutCubic(k) * diag * 0.68;
      const fade = Math.pow(1 - k, 1.3);
      ctx.strokeStyle = '#ffffff';
      const passes: Array<[number, number]> = [
        [14 + 18 * (1 - k), 0.08],
        [5 + 6 * (1 - k), 0.22],
        [1.6, 0.9],
      ];
      for (const [w, a] of passes) {
        ctx.globalAlpha = fade * a;
        ctx.lineWidth = w;
        ctx.beginPath();
        ctx.arc(x, y, r, 0, Math.PI * 2);
        ctx.stroke();
      }
    }
    ctx.globalAlpha = 1;
    ctx.globalCompositeOperation = 'source-over';
  }

  /**
   * 生成粒子：起点聚集在奇点的小圆盘内并取漩涡截图颜色，终点为品牌采样点。
   * 起点按极角、终点按横坐标排序后一一对应，让粒子流成片地从左到右汇聚。
   * Spawn particles: origins cluster in a small disc at the singularity,
   * colored from the vortex snapshot; targets are brand samples. Origins
   * sorted by angle map onto targets sorted by x, so the flow converges in
   * sheets left→right.
   */
  private buildParticles(): ParticleField | null {
    const brand = this.brand;
    if (!brand || brand.count === 0) {
      return null;
    }
    const n = brand.count;
    const rand = createRandom(99);
    const {x: cx, y: cy} = this.impactCenter;

    let pixels: Uint8ClampedArray | null = null;
    let pw = 0;
    let ph = 0;
    if (this.snapshot) {
      pw = 192;
      ph = Math.max(1, Math.round((192 * this.height) / this.width));
      const small = document.createElement('canvas');
      small.width = pw;
      small.height = ph;
      const sctx = small.getContext('2d', {willReadFrequently: true});
      if (sctx) {
        sctx.drawImage(this.snapshot, 0, 0, pw, ph);
        pixels = sctx.getImageData(0, 0, pw, ph).data;
      }
    }

    const origins: Array<{x: number; y: number; a: number}> = [];
    for (let i = 0; i < n; i += 1) {
      const a = rand() * Math.PI * 2;
      // 从奇点涌出：起点集中在爆心的小圆盘内 / Emitted from the point: origins cluster in a small disc at the core.
      const r = this.height * (0.012 + 0.055 * Math.sqrt(rand()));
      origins.push({x: cx + Math.cos(a) * r, y: cy + Math.sin(a) * r, a});
    }
    // 从左侧（极角 π）开始顺时针排序 / Sort by angle starting from the left (π).
    origins.sort(
      (p, q) =>
        ((p.a + Math.PI) % (Math.PI * 2)) - ((q.a + Math.PI) % (Math.PI * 2)),
    );

    const order = Array.from({length: n}, (_, i) => i);
    order.sort((i, j) => brand.targets[i * 5] - brand.targets[j * 5]);

    const field: ParticleField = {
      n,
      ox: new Float32Array(n),
      oy: new Float32Array(n),
      vx: new Float32Array(n),
      vy: new Float32Array(n),
      swirl: new Float32Array(n),
      tx: new Float32Array(n),
      ty: new Float32Array(n),
      emit: new Float32Array(n),
      delay: new Float32Array(n),
      curl: new Float32Array(n),
      size: new Float32Array(n),
      phase: new Float32Array(n),
      c0: new Uint8ClampedArray(n * 3),
      c1: new Uint8ClampedArray(n * 3),
      offX: new Float32Array(n),
      offY: new Float32Array(n),
      velX: new Float32Array(n),
      velY: new Float32Array(n),
      glow: new Uint8Array(n),
    };
    const {x: bx, w: bw} = brand.bounds;

    for (let i = 0; i < n; i += 1) {
      const o = origins[i];
      const ti = order[i] * 5;
      field.ox[i] = o.x;
      field.oy[i] = o.y;
      const dx = o.x - cx;
      const dy = o.y - cy;
      const len = Math.hypot(dx, dy) || 1;
      const speed = this.height * (0.55 + 1.6 * Math.pow(rand(), 1.5));
      // 径向喷出 + 顺漩涡切向：像被奇点甩出的星尘 / Radial spray plus tangential drift, like stardust flung off the singularity.
      field.vx[i] = (dx / len) * speed - (dy / len) * speed * 0.55;
      field.vy[i] = (dy / len) * speed + (dx / len) * speed * 0.55;
      field.swirl[i] = 0.5 + rand() * 0.7;
      field.tx[i] = brand.targets[ti];
      field.ty[i] = brand.targets[ti + 1];
      field.emit[i] = rand() * 210;
      field.delay[i] =
        ((brand.targets[ti] - bx) / Math.max(1, bw)) *
          SPLASH_TIMELINE.gatherSweep +
        rand() * 160;
      field.curl[i] = (rand() - 0.5) * 0.55;
      field.size[i] = 1 + rand() * 1.3;
      field.phase[i] = rand() * Math.PI * 2;
      field.glow[i] = rand() < 0.07 ? 1 : 0;

      let r = 0;
      let g = 0;
      let b = 0;
      if (pixels) {
        const px = Math.min(
          pw - 1,
          Math.max(0, Math.round((o.x / this.width) * pw)),
        );
        const py = Math.min(
          ph - 1,
          Math.max(0, Math.round((o.y / this.height) * ph)),
        );
        const idx = (py * pw + px) * 4;
        r = pixels[idx];
        g = pixels[idx + 1];
        b = pixels[idx + 2];
      }
      if (r + g + b < 150) {
        const c = VORTEX_PALETTE[Math.floor(rand() * VORTEX_PALETTE.length)];
        r = c[0];
        g = c[1];
        b = c[2];
      }
      // 提亮并统一为中性银白，保证叠加混合下够亮且无色偏。
      // Brighten and neutralize to silver so additive blending stays bright and hue-free.
      const lum = Math.min(
        255,
        (r * 0.2126 + g * 0.7152 + b * 0.0722) * 1.3 + 40,
      );
      field.c0[i * 3] = lum * 0.97;
      field.c0[i * 3 + 1] = lum * 0.985;
      field.c0[i * 3 + 2] = lum;
      field.c1[i * 3] = Math.min(255, brand.targets[ti + 2] * 1.05 + 20);
      field.c1[i * 3 + 1] = Math.min(255, brand.targets[ti + 3] * 1.05 + 20);
      field.c1[i * 3 + 2] = Math.min(255, brand.targets[ti + 4] * 1.05 + 20);
    }
    return field;
  }

  private color(r: number, g: number, b: number): string {
    const key = ((r >> 3) << 10) | ((g >> 3) << 5) | (b >> 3);
    let value = this.colorCache.get(key);
    if (!value) {
      value = `rgb(${(r >> 3) << 3},${(g >> 3) << 3},${(b >> 3) << 3})`;
      this.colorCache.set(key, value);
    }
    return value;
  }

  private drawParticles(
    ctx: CanvasRenderingContext2D,
    dt: number,
    brandAlpha: number,
  ): void {
    const f = this.particles;
    if (!f) {
      return;
    }
    const T = SPLASH_TIMELINE;
    // 粒子时钟从释放时刻起算 / Particle clock starts at release.
    const sinceMs = this.t - T.release;
    const drag = 3.4;
    const {x: cx, y: cy} = this.impactCenter;
    const gatherStart = T.gather[0];
    const pointerOn = this.pointer.active && brandAlpha > 0;
    const repelR = Math.max(70, this.height * 0.09);
    const settle = Math.min(1, dt / 16.7);

    for (let i = 0; i < f.n; i += 1) {
      // 错峰涌出：未到自身喷发时刻的粒子跳过 / Staggered emission: skip until this particle's turn.
      const emitted = sinceMs - f.emit[i];
      if (emitted <= 0) {
        continue;
      }
      const eSec = emitted / 1000;
      const travel = (1 - Math.exp(-drag * eSec)) / drag;
      // 喷出：带阻尼的径向运动 + 绕心旋转 / Spray: damped radial motion plus swirl.
      let x = f.ox[i] + f.vx[i] * travel;
      let y = f.oy[i] + f.vy[i] * travel;
      const ang = f.swirl[i] * travel;
      const ca = Math.cos(ang);
      const sa = Math.sin(ang);
      const rx = x - cx;
      const ry = y - cy;
      x = cx + rx * ca - ry * sa;
      y = cy + rx * sa + ry * ca;

      const g = progress(
        this.t,
        gatherStart + f.delay[i],
        gatherStart + f.delay[i] + GATHER_DURATION,
      );
      const e = easeInOutQuart(g);
      if (g > 0) {
        const tx = f.tx[i];
        const ty = f.ty[i];
        const dx = tx - x;
        const dy = ty - y;
        // 汇聚途中沿法向弯出弧线 / Bow sideways along the normal while converging.
        const bow = Math.sin(Math.PI * g) * f.curl[i];
        x = lerp(x, tx, e) - dy * bow;
        y = lerp(y, ty, e) + dx * bow;
      }

      // 成形后：鼠标斥力 + 弹簧回位 / After forming: pointer repulsion with spring return.
      if (g >= 1) {
        let vx = f.velX[i];
        let vy = f.velY[i];
        if (pointerOn) {
          const px = x + f.offX[i] - this.pointer.x;
          const py = y + f.offY[i] - this.pointer.y;
          const d2 = px * px + py * py;
          if (d2 < repelR * repelR && d2 > 0.01) {
            const d = Math.sqrt(d2);
            const force = (1 - d / repelR) * 2.4 * settle;
            vx += (px / d) * force;
            vy += (py / d) * force;
          }
        }
        vx += -f.offX[i] * 0.06 * settle;
        vy += -f.offY[i] * 0.06 * settle;
        vx *= Math.pow(0.84, settle);
        vy *= Math.pow(0.84, settle);
        f.velX[i] = vx;
        f.velY[i] = vy;
        f.offX[i] += vx * settle;
        f.offY[i] += vy * settle;
        x += f.offX[i] + Math.sin(this.t / 600 + f.phase[i]) * 0.35;
        y += f.offY[i] + Math.cos(this.t / 700 + f.phase[i]) * 0.35;
      }

      const ci = i * 3;
      const r = lerp(f.c0[ci], f.c1[ci], e);
      const gg = lerp(f.c0[ci + 1], f.c1[ci + 1], e);
      const b = lerp(f.c0[ci + 2], f.c1[ci + 2], e);
      const twinkle = 0.75 + 0.25 * Math.sin(this.t / 140 + f.phase[i] * 3);
      const burstFade =
        g > 0 ? 1 : 0.55 + 0.45 * (1 - progress(eSec, 0.2, 0.8));
      const alpha = burstFade * lerp(1, 0.42, brandAlpha) * twinkle;
      if (alpha <= 0.01) {
        continue;
      }

      const size = f.size[i] * (g >= 1 ? 1 : 1.25);
      const fill = this.color(r, gg, b);
      ctx.globalAlpha = alpha;
      // 喷出初期按瞬时速度画拖影，体现喷发速度 / Early spray draws velocity streaks to sell the speed.
      if (g <= 0 && eSec < 0.5) {
        const decay = Math.exp(-drag * eSec) * 0.022;
        const sx = f.vx[i] * decay;
        const sy = f.vy[i] * decay;
        ctx.strokeStyle = fill;
        ctx.lineWidth = size * 0.9;
        ctx.beginPath();
        ctx.moveTo(x - sx, y - sy);
        ctx.lineTo(x, y);
        ctx.stroke();
      } else {
        ctx.fillStyle = fill;
        ctx.fillRect(x - size / 2, y - size / 2, size, size);
      }
      if (f.glow[i]) {
        const gs = size * 7;
        ctx.globalAlpha = alpha * 0.35;
        ctx.drawImage(this.glowSprite, x - gs / 2, y - gs / 2, gs, gs);
      }
    }
    ctx.globalAlpha = 1;
  }
}
