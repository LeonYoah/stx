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
 *   2. WebGL 奇点漩涡（碎裂瞬间截图后隐藏）/ WebGL vortex (snapshotted then hidden at impact)
 *   3. 2D 特效画布：碎片、冲击波、流星、粒子、品牌 / 2D FX canvas: shards, shockwave, meteor, particles, brand
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

/** 漩涡吸积盘半径 ≈ 0.245 × 视口高（由着色器 length(p) = .7 推得）/ Accretion rim radius from the shader. */
const RIM_RADIUS_RATIO = 0.245;
/** 视差在着色器 p 空间的最大偏移 / Max parallax offset in shader p-space. */
const PARALLAX_STRENGTH = 0.11;
/** p 空间单位 → 像素（× 视口高）/ p-space unit → pixels (× viewport height). */
const P_TO_PX = 0.35;

const GATHER_DURATION = 920;
const STAR_TAIL_MS = 300;
const SPARK_CAPACITY = 360;

/**
 * 穿心顿帧：撞击后这段时间轴时长以慢速播放，制造“定格一瞬再爆开”的力量感。
 * Impact hit-stop: this slice of timeline after impact plays in slow motion,
 * giving a beat of stillness before the blast.
 */
const HITSTOP_SPAN = 70;
const HITSTOP_SCALE = 0.2;
/** 撞击前内爆收缩时长 / Implosion lead-in before impact. */
const IMPLODE_MS = 420;
/** 镜头震动 / Camera shake. */
const SHAKE_PX = 16;
const SHAKE_MS = 650;
/** 爆心放射光芒数量 / Number of light rays at the blast. */
const RAY_COUNT = 22;

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

/** 漩涡碎片 / Vortex shard. */
interface Shard {
  /** 所在环序号（0 为中心）/ Ring index (0 = center). */
  ring: number;
  cx: number;
  cy: number;
  /** 相对质心的多边形 / Polygon relative to the centroid. */
  poly: Float32Array;
  bx: number;
  by: number;
  bw: number;
  bh: number;
  dirX: number;
  dirY: number;
  dist: number;
  tangent: number;
  spin: number;
  /** 终点缩放：>1 表示朝镜头飞来 / Final scale: >1 means flying toward the camera. */
  depth: number;
  delay: number;
  dur: number;
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

  private shattered = false;
  private impactCenter: Point = {x: 0, y: 0};
  private snapshot: HTMLCanvasElement | null = null;
  private shards: Shard[] = [];
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
  private rays: Array<{angle: number; len: number; width: number}> = [];

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
    if (!this.shattered) {
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
        this.shatter();
      }
    }

    if (!ctx) {
      return;
    }
    ctx.setTransform(this.dpr, 0, 0, this.dpr, 0, 0);
    ctx.clearRect(0, 0, this.width, this.height);

    // —— 2. 碎片与冲击波（带镜头震动）/ Shards and shockwave, with camera shake ——
    if (this.shattered) {
      const since = this.t - T.impact;
      const shake =
        SHAKE_PX * Math.exp(-since / 170) * (since < SHAKE_MS ? 1 : 0);
      ctx.save();
      if (shake > 0.2) {
        ctx.translate(
          Math.sin(since * 0.21) * shake + Math.sin(since * 0.53) * shake * 0.4,
          Math.cos(since * 0.27) * shake * 0.8 +
            Math.sin(since * 0.61) * shake * 0.35,
        );
      }
      this.drawShards(ctx);
      this.drawShockwave(ctx);
      ctx.restore();
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

    // —— 5. 撞击白屏过曝 / Impact white-out ——
    if (this.shattered) {
      // 径向过曝：中心炽白、边缘迅速衰减，时长极短，避免整屏发灰。
      // Radial overexposure: blazing core, fast falloff, very short so the
      // screen never turns flat grey.
      const whiteout = Math.pow(1 - progress(this.t - T.impact, 0, 150), 2);
      if (whiteout > 0) {
        const {x, y} = this.impactCenter;
        const radius = Math.hypot(this.width, this.height) * 0.55;
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
   * 穿心瞬间：同帧截取漩涡画面，切成螺旋裂纹碎片，并生成粒子。
   * Impact: snapshot the vortex in the same frame, cut it into spiral-cracked
   * shards, and spawn the particles.
   */
  private shatter(): void {
    this.shattered = true;
    this.impactCenter = this.vortexCenter();
    this.opts.root.dataset.shattered = 'on';

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
        this.snapshot = snap;
        this.shards = this.buildShards();
      }
      this.renderer.dispose();
      this.renderer = null;
    }

    // 爆心放射光芒 / Light rays from the blast core.
    const rr = createRandom(77);
    this.rays = Array.from({length: RAY_COUNT}, () => ({
      angle: rr() * Math.PI,
      len: this.height * (0.45 + rr() * 0.75),
      width: 1.5 + rr() * 4,
    }));

    if (!this.brand) {
      this.layoutBrand();
    }
    this.particles = this.buildParticles();
  }

  private buildShards(): Shard[] {
    const {x: cx, y: cy} = this.impactCenter;
    const h = this.height;
    const maxR =
      Math.max(
        Math.hypot(cx, cy),
        Math.hypot(this.width - cx, cy),
        Math.hypot(cx, h - cy),
        Math.hypot(this.width - cx, h - cy),
      ) * 1.04;
    const ringRatios = [
      0,
      0.05,
      0.1,
      0.165,
      RIM_RADIUS_RATIO * 1.06,
      0.33,
      0.44,
      0.6,
      0.8,
      1.06,
      1.42,
      2,
    ];
    const radii: number[] = [];
    for (const r of ringRatios) {
      if (r * h >= maxR) {
        break;
      }
      radii.push(r * h);
    }
    radii.push(maxR);

    const rand = createRandom(4242);
    const wobble = radii.map(() => rand() * Math.PI * 2);
    // 螺旋扭转：裂纹顺着漩涡方向 / Spiral twist so cracks follow the vortex.
    const twist = radii.map((_, k) => k * 0.24);
    const boundary = (k: number, theta: number) =>
      k === 0
        ? 0
        : radii[k] *
          (k === radii.length - 1
            ? 1
            : 1 +
              0.05 * Math.sin(3 * theta + wobble[k]) +
              0.025 * Math.sin(7 * theta + wobble[k] * 1.7));

    const shards: Shard[] = [];
    for (let k = 0; k < radii.length - 1; k += 1) {
      const n = k === 0 ? 9 : 10 + k * 5;
      const angles: number[] = [];
      const ringRot = rand() * Math.PI * 2;
      for (let j = 0; j < n; j += 1) {
        angles.push(((j + (rand() - 0.5) * 0.6) / n) * Math.PI * 2 + ringRot);
      }
      for (let j = 0; j < n; j += 1) {
        const a0 = angles[j];
        const a1 = j === n - 1 ? angles[0] + Math.PI * 2 : angles[j + 1];
        const pts: number[] = [];
        // 内弧 / Inner arc.
        if (k === 0) {
          pts.push(cx, cy);
        } else {
          const steps = Math.max(2, Math.ceil((a1 - a0) / 0.05));
          for (let s = 0; s <= steps; s += 1) {
            const th = lerp(a0, a1, s / steps) + twist[k];
            const r = boundary(k, th);
            pts.push(cx + Math.cos(th) * r, cy + Math.sin(th) * r);
          }
        }
        // 外弧（反向）/ Outer arc, reversed.
        const steps = Math.max(2, Math.ceil((a1 - a0) / 0.05));
        for (let s = steps; s >= 0; s -= 1) {
          const th = lerp(a0, a1, s / steps) + twist[k + 1];
          const r = boundary(k + 1, th);
          pts.push(cx + Math.cos(th) * r, cy + Math.sin(th) * r);
        }

        let sx = 0;
        let sy = 0;
        const count = pts.length / 2;
        for (let i = 0; i < pts.length; i += 2) {
          sx += pts[i];
          sy += pts[i + 1];
        }
        const mx = sx / count;
        const my = sy / count;
        let minX = Infinity;
        let minY = Infinity;
        let maxX = -Infinity;
        let maxY = -Infinity;
        const poly = new Float32Array(pts.length);
        for (let i = 0; i < pts.length; i += 2) {
          const px = pts[i] - mx;
          const py = pts[i + 1] - my;
          poly[i] = px;
          poly[i + 1] = py;
          minX = Math.min(minX, px);
          minY = Math.min(minY, py);
          maxX = Math.max(maxX, px);
          maxY = Math.max(maxY, py);
        }
        const dx = mx - cx;
        const dy = my - cy;
        const len = Math.hypot(dx, dy) || 1;
        shards.push({
          ring: k,
          cx: mx,
          cy: my,
          poly,
          bx: minX - 1,
          by: minY - 1,
          bw: maxX - minX + 2,
          bh: maxY - minY + 2,
          dirX: dx / len,
          dirY: dy / len,
          dist: h * (0.28 + 0.62 * rand() + 0.07 * k),
          tangent: h * (0.05 + 0.16 * rand()),
          spin: (rand() - 0.5) * 3.2,
          // 内圈约四成碎片朝镜头飞来（放大），其余向远处缩小。
          // About 40% of inner shards fly toward the camera (grow); the rest recede.
          depth:
            k <= 5 && rand() < 0.4 ? 1.5 + rand() * 0.9 : 0.45 + rand() * 0.3,
          delay: k * 22 + rand() * 40,
          dur: 1000 + rand() * 450,
        });
      }
    }
    return shards;
  }

  private drawShards(ctx: CanvasRenderingContext2D): void {
    const snap = this.snapshot;
    if (!snap || this.shards.length === 0) {
      return;
    }
    const since = this.t - SPLASH_TIMELINE.impact;
    if (since > 1700) {
      return;
    }
    const sx = snap.width / this.width;
    const sy = snap.height / this.height;
    ctx.globalCompositeOperation = 'source-over';

    for (const shard of this.shards) {
      const k = clamp01((since - shard.delay) / shard.dur);
      const alpha =
        1 - easeInOutCubic(progress(k, shard.depth > 1 ? 0.05 : 0.15, 1));
      if (alpha <= 0.01) {
        continue;
      }
      // 指数缓出：起速极快，随后减速飘散 / Exponential ease-out: violent start, then drift.
      const e = k >= 1 ? 1 : 1 - Math.pow(2, -9 * k);
      const x =
        shard.cx + shard.dirX * shard.dist * e - shard.dirY * shard.tangent * e;
      const y =
        shard.cy + shard.dirY * shard.dist * e + shard.dirX * shard.tangent * e;
      const sc = lerp(1, shard.depth, e);
      ctx.save();
      ctx.globalAlpha = alpha;
      ctx.translate(x, y);
      ctx.rotate(shard.spin * e);
      ctx.scale(sc, sc);
      ctx.beginPath();
      const p = shard.poly;
      ctx.moveTo(p[0], p[1]);
      for (let i = 2; i < p.length; i += 2) {
        ctx.lineTo(p[i], p[i + 1]);
      }
      ctx.closePath();
      ctx.save();
      ctx.clip();
      ctx.drawImage(
        snap,
        (shard.cx + shard.bx) * sx,
        (shard.cy + shard.by) * sy,
        shard.bw * sx,
        shard.bh * sy,
        shard.bx,
        shard.by,
        shard.bw,
        shard.bh,
      );
      ctx.restore();
      // 碎片边缘高光：像玻璃断面反光 / Edge highlight like light catching a glass fracture.
      if (shard.ring <= 5 && k > 0) {
        ctx.globalCompositeOperation = 'lighter';
        ctx.globalAlpha = alpha * Math.max(0, 1 - e * 1.4) * 0.5;
        ctx.strokeStyle = '#ffffff';
        ctx.lineWidth = 1.2 / sc;
        ctx.stroke();
        ctx.globalCompositeOperation = 'source-over';
      }
      ctx.restore();
    }

    // 裂纹高光：碎片分离前只在漩涡本体附近一闪，外圈暗区不画，避免满屏蛛网。
    // Crack glints flash only around the vortex body before separation; the
    // dark outskirts stay clean so the screen doesn't turn into a spider web.
    const crack = 1 - progress(since, 0, 160);
    if (crack > 0) {
      ctx.globalCompositeOperation = 'lighter';
      ctx.strokeStyle = '#ffffff';
      ctx.lineWidth = 1.4;
      ctx.globalAlpha = crack * 0.9;
      ctx.beginPath();
      for (const shard of this.shards) {
        if (shard.ring > 4) {
          continue;
        }
        const p = shard.poly;
        ctx.moveTo(shard.cx + p[0], shard.cy + p[1]);
        for (let i = 2; i < p.length; i += 2) {
          ctx.lineTo(shard.cx + p[i], shard.cy + p[i + 1]);
        }
        ctx.closePath();
      }
      ctx.stroke();
      ctx.globalAlpha = 1;
      ctx.globalCompositeOperation = 'source-over';
    }
  }

  /**
   * 爆心：核心闪光、放射光芒、两道冲击波（主波带柔光，次波稍后跟进）。
   * Blast core: core flash, radiating light rays, and two shockwaves (a
   * glowing primary and a delayed secondary).
   */
  private drawShockwave(ctx: CanvasRenderingContext2D): void {
    const since = this.t - SPLASH_TIMELINE.impact;
    const {x, y} = this.impactCenter;
    const diag = Math.hypot(this.width, this.height);
    ctx.globalCompositeOperation = 'lighter';

    const flash = 1 - progress(since, 0, 520);
    if (flash > 0) {
      const r = this.height * (0.35 + 0.5 * (1 - flash));
      ctx.globalAlpha = flash * flash;
      ctx.drawImage(this.glowSprite, x - r, y - r, r * 2, r * 2);
    }

    // 放射光芒：瞬间拉长后渐隐 / Rays: snap outward, then fade.
    const rayK = progress(since, 0, 700);
    if (rayK > 0 && rayK < 1) {
      const grow = easeOutCubic(progress(since, 0, 260));
      ctx.save();
      ctx.translate(x, y);
      for (const ray of this.rays) {
        ctx.save();
        ctx.rotate(ray.angle);
        ctx.globalAlpha = Math.pow(1 - rayK, 1.6) * 0.7;
        const len = ray.len * grow;
        ctx.drawImage(
          this.flareSprite,
          -len,
          -ray.width / 2,
          len * 2,
          ray.width,
        );
        ctx.restore();
      }
      ctx.restore();
    }

    const drawRing = (
      start: number,
      dur: number,
      reach: number,
      strength: number,
    ) => {
      const k = progress(since, start, start + dur);
      if (k <= 0 || k >= 1) {
        return;
      }
      const r = easeOutCubic(k) * diag * reach;
      const fade = Math.pow(1 - k, 1.3) * strength;
      ctx.strokeStyle = '#ffffff';
      // 三层叠描：外层宽而淡做柔光，内层细而亮做锋线。
      // Three passes: wide faint glow outside, thin bright edge inside.
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
    };
    drawRing(0, 950, 0.7, 1);
    drawRing(130, 1150, 0.55, 0.55);

    ctx.globalAlpha = 1;
    ctx.globalCompositeOperation = 'source-over';
  }

  /**
   * 生成粒子：起点散布在吸积盘附近并取漩涡截图颜色，终点为品牌采样点。
   * 起点按极角、终点按横坐标排序后一一对应，让粒子流成片地从左到右汇聚。
   * Spawn particles: origins scatter around the accretion rim, colored from
   * the vortex snapshot; targets are brand samples. Origins sorted by angle
   * map onto targets sorted by x, so the flow converges in sheets left→right.
   */
  private buildParticles(): ParticleField | null {
    const brand = this.brand;
    if (!brand || brand.count === 0) {
      return null;
    }
    const n = brand.count;
    const rand = createRandom(99);
    const {x: cx, y: cy} = this.impactCenter;
    const rim = RIM_RADIUS_RATIO * this.height;

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
      const nearRim = rand() < 0.65;
      const r = nearRim
        ? rim * (1 + (rand() - 0.5) * 0.34)
        : rim * (0.18 + 1.5 * Math.sqrt(rand()));
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
      const speed = this.height * (0.45 + 1.5 * Math.pow(rand(), 1.6));
      // 径向炸开 + 顺漩涡切向 / Radial burst plus tangential vortex drift.
      field.vx[i] = (dx / len) * speed - (dy / len) * speed * 0.55;
      field.vy[i] = (dy / len) * speed + (dx / len) * speed * 0.55;
      field.swirl[i] = 0.5 + rand() * 0.7;
      field.tx[i] = brand.targets[ti];
      field.ty[i] = brand.targets[ti + 1];
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
    const since = (this.t - T.impact) / 1000;
    const drag = 3.4;
    const travel = (1 - Math.exp(-drag * since)) / drag;
    const {x: cx, y: cy} = this.impactCenter;
    const gatherStart = T.gather[0];
    const pointerOn = this.pointer.active && brandAlpha > 0;
    const repelR = Math.max(70, this.height * 0.09);
    const settle = Math.min(1, dt / 16.7);

    for (let i = 0; i < f.n; i += 1) {
      // 炸开：带阻尼的径向运动 + 绕心旋转 / Burst: damped radial motion plus swirl.
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
        g > 0 ? 1 : 0.55 + 0.45 * (1 - progress(since, 0.2, 0.8));
      const alpha = burstFade * lerp(1, 0.42, brandAlpha) * twinkle;
      if (alpha <= 0.01) {
        continue;
      }

      const size = f.size[i] * (g >= 1 ? 1 : 1.25);
      const fill = this.color(r, gg, b);
      ctx.globalAlpha = alpha;
      // 炸开初期按瞬时速度画拖影，体现爆发速度 / Early burst draws velocity streaks to sell the speed.
      if (g <= 0 && since < 0.5) {
        const decay = Math.exp(-drag * since) * 0.022;
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
