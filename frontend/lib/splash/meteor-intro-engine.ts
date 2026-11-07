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
 * STX 第四版开屏 Canvas 引擎（由用户审定的单文件原型移植，去掉调试 HUD）。
 * STX splash edition v4 canvas engine (ported from the approved single-file prototype; debug HUD removed).
 *
 * 视觉与节奏保持原型：流星 → 碎裂 → 聚字 → 6.8–7.08s 干净实心字 → mark 并排 + 欢迎文案 → 9s 定格。
 * Keeps prototype visuals/timing: meteor → shatter → reform → clean solid text → mark lockup + welcome → 9s hold.
 */

export const SPLASH_MARK_SRC = '/brand/stx-mark.png';
export const SPLASH_BG_SRC = '/brand/stx-splash-nebula.jpg';

/** 时间轴（毫秒），与 v4 原型一致 / Timeline in ms, matching the v4 prototype */
export const SPLASH_TIMELINE = {
  logoInEnd: 1300,
  meteorEnd: 2700,
  shatterEnd: 4300,
  formEnd: 6800,
  cleanTextEnd: 7080,
  welcomeStart: 7200,
  welcomeEnd: 7800,
  lockupEnd: 8600,
  total: 9000,
} as const;

const T = SPLASH_TIMELINE;
const IMPACT_MS = 2100;

/** reduced-motion 下终态短暂展示后自动结束，避免卡死 / Brief final-frame hold under reduced-motion before auto-finish */
export const REDUCED_MOTION_HOLD_MS = 1200;

/**
 * 资源 onload/onerror 均未触发时的挂起超时；超时后按失败降级并启动时间轴，避免永久黑屏遮罩。
 * Hang timeout when neither onload nor onerror fires; degrade as failure and start timeline so the overlay cannot block forever.
 */
export const ASSET_LOAD_TIMEOUT_MS = 2500;

export type MeteorIntroEngineOptions = {
  canvas: HTMLCanvasElement;
  markSrc?: string;
  bgSrc?: string;
  welcomeText: string;
  reducedMotion?: boolean;
  /** 测试或定制资源加载超时（毫秒） / Override asset hang timeout (ms) for tests or customization */
  assetLoadTimeoutMs?: number;
  onComplete?: () => void;
  onFatalError?: (error: unknown) => void;
};

type Particle = {
  hx: number;
  hy: number;
  scatterX: number;
  scatterY: number;
  tx: number;
  ty: number;
  cpx: number;
  cpy: number;
  size: number;
  hue: number;
  bright: number;
  delay: number;
  phase: number;
};

type TrailSpark = {x: number; y: number; born: number; life: number; size: number};
type ImpactSpark = {ang: number; sp: number; life: number; size: number};
type TargetPt = {x: number; y: number};

type Layout = {
  markH: number;
  markW: number;
  fontSize: number;
  gap: number;
  cx: number;
  cy: number;
  markCx: number;
  textCx: number;
  textW: number;
  textBaselineY: number;
  welcomeSize: number;
  welcomeY: number;
  short: number;
  totalW: number;
};

function clamp(v: number, a: number, b: number): number {
  return Math.max(a, Math.min(b, v));
}
function lerp(a: number, b: number, t: number): number {
  return a + (b - a) * t;
}
function easeInOutCubic(t: number): number {
  return t < 0.5 ? 4 * t * t * t : 1 - Math.pow(-2 * t + 2, 3) / 2;
}
function easeOutCubic(t: number): number {
  return 1 - Math.pow(1 - t, 3);
}
function easeInCubic(t: number): number {
  return t * t * t;
}
function easeOutQuart(t: number): number {
  return 1 - Math.pow(1 - t, 4);
}
function easeOutBack(t: number): number {
  const c1 = 1.70158;
  const c3 = c1 + 1;
  return 1 + c3 * Math.pow(t - 1, 3) + c1 * Math.pow(t - 1, 2);
}
function smoothstep(e0: number, e1: number, x: number): number {
  const t = clamp((x - e0) / (e1 - e0), 0, 1);
  return t * t * (3 - 2 * t);
}
function rand(a: number, b: number): number {
  return a + Math.random() * (b - a);
}
function mulberry32(a: number): () => number {
  return function () {
    let t = (a += 0x6d2b79f5);
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

function brandFont(size: number): string {
  return `700 ${size}px Inter, "Segoe UI", "Helvetica Neue", Arial, sans-serif`;
}
function welcomeFont(size: number): string {
  return `400 ${size}px "Noto Sans SC", "PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", system-ui, sans-serif`;
}

/**
 * 可销毁的开屏引擎：负责 rAF / resize / 图片加载，卸载时必须 destroy。
 * Disposable splash engine: owns rAF / resize / image loads; callers must destroy on unmount.
 */
export class MeteorIntroEngine {
  private readonly canvas: HTMLCanvasElement;
  private readonly ctx: CanvasRenderingContext2D;
  private readonly markSrc: string;
  private readonly bgSrc: string;
  private welcomeText: string;
  private readonly reducedMotion: boolean;
  private readonly assetLoadTimeoutMs: number;
  private readonly onComplete?: () => void;
  private readonly onFatalError?: (error: unknown) => void;

  private W = 0;
  private H = 0;
  private dpr = 1;
  private markImg: HTMLImageElement | null = null;
  private bgImg: HTMLImageElement | null = null;
  private bgReady = false;
  private targets: TargetPt[] = [];
  private particles: Particle[] = [];
  private trail: TrailSpark[] = [];
  private sparks: ImpactSpark[] = [];
  private ready = false;
  private tMs = 0;
  private playing = false;
  private lastFrame = 0;
  private lastTrailMs = -1;
  private timelineStarted = false;
  private rafId: number | null = null;
  private reducedHoldTimer: ReturnType<typeof setTimeout> | null = null;
  private assetLoadTimer: ReturnType<typeof setTimeout> | null = null;
  private completed = false;
  private destroyed = false;
  private markLoaded = false;
  private bgLoaded = false;
  private readonly onResize = () => this.resize();

  constructor(options: MeteorIntroEngineOptions) {
    this.canvas = options.canvas;
    const ctx = options.canvas.getContext('2d', {alpha: false});
    if (!ctx) {
      throw new Error('Canvas 2D context unavailable');
    }
    this.ctx = ctx;
    this.markSrc = options.markSrc ?? SPLASH_MARK_SRC;
    this.bgSrc = options.bgSrc ?? SPLASH_BG_SRC;
    this.welcomeText = options.welcomeText;
    this.reducedMotion =
      options.reducedMotion ??
      (typeof window !== 'undefined' &&
        window.matchMedia('(prefers-reduced-motion: reduce)').matches);
    this.assetLoadTimeoutMs = options.assetLoadTimeoutMs ?? ASSET_LOAD_TIMEOUT_MS;
    this.onComplete = options.onComplete;
    this.onFatalError = options.onFatalError;
  }

  /** 更新欢迎文案（语言切换时） / Update welcome caption when locale changes */
  setWelcomeText(text: string): void {
    this.welcomeText = text;
    if (this.ready && !this.destroyed) {
      this.render(this.tMs);
    }
  }

  /** 启动资源加载与时间轴 / Start asset loading and the timeline */
  start(): void {
    if (this.destroyed || this.timelineStarted) {
      return;
    }
    try {
      this.paintLoading();
      window.addEventListener('resize', this.onResize);

      // 挂起超时：永不 load/error 时仍降级开播，避免遮罩永久阻塞
      // Hang timeout: if neither load nor error fires, degrade and start so the overlay cannot block forever
      this.clearAssetLoadTimer();
      this.assetLoadTimer = setTimeout(() => {
        this.assetLoadTimer = null;
        if (this.destroyed || this.timelineStarted) {
          return;
        }
        if (!this.markLoaded) {
          this.markLoaded = true;
        }
        if (!this.bgLoaded) {
          this.bgReady = false;
          this.bgLoaded = true;
        }
        this.tryStart();
      }, this.assetLoadTimeoutMs);

      this.markImg = new Image();
      this.markImg.decoding = 'async';
      this.markImg.onload = () => {
        this.markLoaded = true;
        this.tryStart();
      };
      this.markImg.onerror = () => {
        this.markLoaded = true;
        this.tryStart();
      };
      this.markImg.src = this.markSrc;

      this.bgImg = new Image();
      this.bgImg.decoding = 'async';
      this.bgImg.onload = () => {
        this.bgReady = true;
        this.bgLoaded = true;
        this.tryStart();
      };
      this.bgImg.onerror = () => {
        this.bgReady = false;
        this.bgLoaded = true;
        this.tryStart();
      };
      this.bgImg.src = this.bgSrc;
    } catch (error) {
      this.fail(error);
    }
  }

  /** 跳过并结束（清理由调用方 destroy） / Skip and finish (caller still destroys) */
  skip(): void {
    this.finish();
  }

  /** 销毁：取消 rAF、定时器、resize 监听与进行中绘制 / Destroy: cancel rAF, timers, resize listener */
  destroy(): void {
    if (this.destroyed) {
      return;
    }
    this.destroyed = true;
    this.playing = false;
    if (this.rafId != null) {
      cancelAnimationFrame(this.rafId);
      this.rafId = null;
    }
    if (this.reducedHoldTimer != null) {
      clearTimeout(this.reducedHoldTimer);
      this.reducedHoldTimer = null;
    }
    this.clearAssetLoadTimer();
    window.removeEventListener('resize', this.onResize);
    if (this.markImg) {
      this.markImg.onload = null;
      this.markImg.onerror = null;
      this.markImg.src = '';
      this.markImg = null;
    }
    if (this.bgImg) {
      this.bgImg.onload = null;
      this.bgImg.onerror = null;
      this.bgImg.src = '';
      this.bgImg = null;
    }
    this.particles = [];
    this.trail = [];
    this.sparks = [];
    this.targets = [];
  }

  private clearAssetLoadTimer(): void {
    if (this.assetLoadTimer != null) {
      clearTimeout(this.assetLoadTimer);
      this.assetLoadTimer = null;
    }
  }

  private fail(error: unknown): void {
    this.onFatalError?.(error);
    this.finish();
  }

  private finish(): void {
    if (this.completed || this.destroyed) {
      return;
    }
    this.completed = true;
    this.playing = false;
    if (this.rafId != null) {
      cancelAnimationFrame(this.rafId);
      this.rafId = null;
    }
    if (this.reducedHoldTimer != null) {
      clearTimeout(this.reducedHoldTimer);
      this.reducedHoldTimer = null;
    }
    this.clearAssetLoadTimer();
    this.onComplete?.();
  }

  private tryStart(): void {
    if (this.destroyed || this.timelineStarted) {
      return;
    }
    if (!(this.markLoaded && this.bgLoaded)) {
      return;
    }
    this.clearAssetLoadTimer();
    this.startTimeline();
  }

  private paintLoading(): void {
    this.dpr = Math.min(window.devicePixelRatio || 1, 2);
    this.W = window.innerWidth;
    this.H = window.innerHeight;
    this.canvas.width = Math.floor(this.W * this.dpr);
    this.canvas.height = Math.floor(this.H * this.dpr);
    this.canvas.style.width = `${this.W}px`;
    this.canvas.style.height = `${this.H}px`;
    this.ctx.setTransform(this.dpr, 0, 0, this.dpr, 0, 0);
    this.ctx.fillStyle = '#05070c';
    this.ctx.fillRect(0, 0, this.W, this.H);
  }

  private startTimeline(): void {
    if (this.timelineStarted || this.destroyed) {
      return;
    }
    this.timelineStarted = true;
    this.ready = true;
    try {
      this.resize();
      if (this.reducedMotion) {
        this.tMs = T.lockupEnd;
        this.playing = false;
        this.render(this.tMs);
        this.reducedHoldTimer = setTimeout(() => this.finish(), REDUCED_MOTION_HOLD_MS);
      } else {
        this.tMs = 0;
        this.playing = true;
        this.lastFrame = 0;
        this.render(0);
        this.rafId = requestAnimationFrame((now) => this.frame(now));
      }
    } catch (error) {
      this.fail(error);
    }
  }

  private frame(now: number): void {
    if (this.destroyed || this.completed) {
      return;
    }
    try {
      if (!this.lastFrame) {
        this.lastFrame = now;
      }
      const dt = Math.min(0.05, (now - this.lastFrame) / 1000);
      this.lastFrame = now;
      if (this.playing) {
        this.tMs += dt * 1000;
        if (this.tMs >= T.total) {
          this.tMs = T.total;
          this.playing = false;
          this.render(this.tMs);
          this.finish();
          return;
        }
      }
      if (this.tMs < IMPACT_MS) {
        this.sparks = [];
      }
      this.render(this.tMs);
      this.rafId = requestAnimationFrame((t) => this.frame(t));
    } catch (error) {
      // rAF 绘制异常不得停表卡死遮罩；fail-safe 结束开屏
      // Drawing errors in rAF must not freeze the overlay; fail-safe dismisses the splash
      this.fail(error);
    }
  }

  private resize(): void {
    if (this.destroyed) {
      return;
    }
    this.dpr = Math.min(window.devicePixelRatio || 1, 2);
    this.W = window.innerWidth;
    this.H = window.innerHeight;
    this.canvas.width = Math.floor(this.W * this.dpr);
    this.canvas.height = Math.floor(this.H * this.dpr);
    this.canvas.style.width = `${this.W}px`;
    this.canvas.style.height = `${this.H}px`;
    this.ctx.setTransform(this.dpr, 0, 0, this.dpr, 0, 0);
    if (this.ready) {
      this.buildTargets();
      this.seedParticles();
      this.sparks = [];
      this.render(this.tMs);
    }
  }

  private layout(): Layout {
    const short = Math.min(this.W, this.H);
    const markH = clamp(short * 0.2, 84, 156);
    const markAspect = this.markImg
      ? this.markImg.naturalWidth / this.markImg.naturalHeight
      : 480 / 362;
    const markW = markH * markAspect;
    const fontSize = clamp(short * 0.17, 60, 124);
    const gap = clamp(short * 0.04, 16, 32);
    this.ctx.save();
    this.ctx.font = brandFont(fontSize);
    const textW = this.ctx.measureText('STX').width;
    this.ctx.restore();
    const totalW = markW + gap + textW;
    const cx = this.W / 2;
    const cy = this.H * 0.47;
    const markCx = cx - totalW / 2 + markW / 2;
    const textCx = cx - totalW / 2 + markW + gap + textW / 2;
    const textBaselineY = cy + fontSize * 0.03;
    const welcomeSize = clamp(fontSize * 0.28, 14, 22);
    const welcomeY =
      cy + Math.max(markH, fontSize) * 0.55 + clamp(short * 0.04, 18, 36);
    return {
      markH,
      markW,
      fontSize,
      gap,
      cx,
      cy,
      markCx,
      textCx,
      textW,
      textBaselineY,
      welcomeSize,
      welcomeY,
      short,
      totalW,
    };
  }

  private buildTargets(): void {
    const L = this.layout();
    const off = document.createElement('canvas');
    const pad = 10;
    const tw = Math.ceil(L.textW + pad * 2);
    const th = Math.ceil(L.fontSize * 1.3 + pad * 2);
    off.width = Math.max(2, tw);
    off.height = Math.max(2, th);
    const o = off.getContext('2d');
    if (!o) {
      this.targets = [{x: L.textCx, y: L.cy}];
      return;
    }
    o.clearRect(0, 0, tw, th);
    o.fillStyle = '#fff';
    o.font = brandFont(L.fontSize);
    o.textAlign = 'center';
    o.textBaseline = 'middle';
    o.fillText('STX', tw / 2, th / 2 + L.fontSize * 0.03);

    const data = o.getImageData(0, 0, tw, th).data;
    const pts: TargetPt[] = [];
    const step = Math.max(2, Math.round(L.fontSize / 36));
    for (let y = 0; y < th; y += step) {
      for (let x = 0; x < tw; x += step) {
        if (data[(y * tw + x) * 4 + 3] > 130) {
          pts.push({
            x: L.textCx - tw / 2 + x,
            y: L.cy - th / 2 + y,
          });
        }
      }
    }
    this.targets = pts.length ? pts : [{x: L.textCx, y: L.cy}];
  }

  private seedParticles(): void {
    const L = this.layout();
    const rng = mulberry32(0x53545821);
    const n = Math.min(Math.max(this.targets.length, 520), 980);
    this.particles = [];
    for (let i = 0; i < n; i++) {
      const tgt = this.targets[i % this.targets.length];
      const a = rng() * Math.PI * 2;
      const r0 = rng() * Math.min(L.markW, L.markH) * 0.38;
      const hx = L.cx + Math.cos(a) * r0;
      const hy = L.cy + Math.sin(a) * r0 * 0.85;
      const blast = a * 0.55 + Math.PI * 0.19 + (rng() - 0.5) * 0.9;
      const dist = (0.22 + rng() * 0.55) * Math.min(this.W, this.H);
      const scatterX = L.cx + Math.cos(blast) * dist;
      const scatterY = L.cy + Math.sin(blast) * dist * (0.75 + rng() * 0.35);
      const midBias = (rng() - 0.5) * 90;
      this.particles.push({
        hx,
        hy,
        scatterX,
        scatterY,
        tx: tgt.x + (rng() - 0.5) * 0.9,
        ty: tgt.y + (rng() - 0.5) * 0.9,
        cpx: (scatterX + tgt.x) * 0.5 + midBias,
        cpy: (scatterY + tgt.y) * 0.5 - (40 + rng() * 70),
        size: 1.0 + rng() * 2.1,
        hue: 192 + rng() * 14,
        bright: 0.55 + rng() * 0.45,
        delay: rng() * 0.28,
        phase: rng(),
      });
    }
    this.trail = [];
    this.sparks = [];
  }

  private meteorAt(ms: number) {
    const startX = -this.W * 0.18;
    const startY = this.H * 0.05;
    const endX = this.W * 1.18;
    const endY = this.H * 0.95;
    const t0 = T.logoInEnd;
    const t1 = T.meteorEnd;
    const raw = clamp((ms - t0) / (t1 - t0), 0, 1);
    const e = easeInCubic(raw) * 0.25 + raw * 0.75;
    const x = lerp(startX, endX, e);
    const y = lerp(startY, endY, e);
    const ang = Math.atan2(endY - startY, endX - startX);
    const visible = ms >= t0 - 60 && ms <= t1 + 100;
    return {x, y, ang, e, visible, pastImpact: ms >= IMPACT_MS};
  }

  private buildSparks(): void {
    const rng = mulberry32(0xa11ce);
    this.sparks = [];
    for (let i = 0; i < 72; i++) {
      const a = rng() * Math.PI * 2;
      const sp = 140 + rng() * 460;
      this.sparks.push({
        ang: a,
        sp,
        life: 0.3 + rng() * 0.55,
        size: 1.4 + rng() * 3.2,
      });
    }
  }

  private poseParticle(p: Particle, ms: number) {
    if (ms < IMPACT_MS) {
      const wake = smoothstep(500, T.logoInEnd, ms);
      const shimmer = Math.sin(ms * 0.012 + p.phase * 12) * 1.2;
      return {
        x: p.hx + shimmer,
        y: p.hy + Math.cos(ms * 0.01 + p.phase) * 1.0,
        a: wake * 0.18,
        size: p.size * 0.55,
      };
    }

    if (ms < T.shatterEnd) {
      const u = clamp((ms - IMPACT_MS) / (T.shatterEnd - IMPACT_MS), 0, 1);
      const local = clamp((u - p.delay * 0.45) / (1 - p.delay * 0.45), 0, 1);
      const e = easeOutQuart(local);
      const boom = e < 0.7 ? easeOutCubic(e / 0.7) : 1;
      return {
        x: lerp(p.hx, p.scatterX, boom),
        y: lerp(p.hy, p.scatterY, boom) + e * e * 12,
        a: lerp(1, 0.78, e),
        size: p.size * lerp(1.55, 1.05, e),
      };
    }

    if (ms < T.formEnd) {
      const u = clamp((ms - T.shatterEnd) / (T.formEnd - T.shatterEnd), 0, 1);
      const local = clamp((u - p.delay * 0.4) / (1 - p.delay * 0.4), 0, 1);
      const e = easeInOutCubic(local);
      const omt = 1 - e;
      return {
        x: omt * omt * p.scatterX + 2 * omt * e * p.cpx + e * e * p.tx,
        y: omt * omt * p.scatterY + 2 * omt * e * p.cpy + e * e * p.ty,
        a: lerp(0.75, 1, e),
        size: p.size * lerp(1.05, 1.15, e),
      };
    }

    return {x: p.tx, y: p.ty, a: 1, size: p.size * 1.05};
  }

  private cleanTextBlend(ms: number): number {
    return smoothstep(T.formEnd, T.cleanTextEnd, ms);
  }

  private drawCoverBackground(img: HTMLImageElement): void {
    const iw = img.naturalWidth || img.width;
    const ih = img.naturalHeight || img.height;
    const scale = Math.max(this.W / iw, this.H / ih);
    const dw = iw * scale;
    const dh = ih * scale;
    const dx = (this.W - dw) / 2;
    const dy = (this.H - dh) / 2;
    this.ctx.drawImage(img, dx, dy, dw, dh);
  }

  private drawBackground(ms: number): void {
    const ctx = this.ctx;
    ctx.fillStyle = '#05070c';
    ctx.fillRect(0, 0, this.W, this.H);

    if (this.bgReady && this.bgImg) {
      this.drawCoverBackground(this.bgImg);
      const L = this.layout();
      const veil = ctx.createRadialGradient(
        L.cx,
        L.cy,
        Math.min(this.W, this.H) * 0.08,
        L.cx,
        L.cy,
        Math.min(this.W, this.H) * 0.42,
      );
      veil.addColorStop(0, 'rgba(5,7,12,0.22)');
      veil.addColorStop(0.55, 'rgba(5,7,12,0.08)');
      veil.addColorStop(1, 'rgba(5,7,12,0)');
      ctx.fillStyle = veil;
      ctx.fillRect(0, 0, this.W, this.H);
    }

    if (ms >= IMPACT_MS && ms < IMPACT_MS + 800) {
      const u = (ms - IMPACT_MS) / 800;
      const shock = (1 - u) * (1 - u) * 0.12;
      if (shock > 0.01) {
        const L = this.layout();
        const g = ctx.createRadialGradient(
          L.cx,
          L.cy,
          0,
          L.cx,
          L.cy,
          Math.min(this.W, this.H) * 0.65,
        );
        g.addColorStop(0, `rgba(34,181,244,${shock * 0.45})`);
        g.addColorStop(0.45, `rgba(20,60,100,${shock * 0.18})`);
        g.addColorStop(1, 'rgba(5,7,12,0)');
        ctx.fillStyle = g;
        ctx.fillRect(0, 0, this.W, this.H);
      }
    }

    if (ms > T.formEnd) {
      const L = this.layout();
      const a = smoothstep(T.formEnd, T.lockupEnd, ms) * 0.05;
      const g2 = ctx.createRadialGradient(
        L.cx,
        L.cy + L.fontSize * 0.2,
        0,
        L.cx,
        L.cy + L.fontSize * 0.2,
        Math.min(this.W, this.H) * 0.32,
      );
      g2.addColorStop(0, `rgba(34,181,244,${a})`);
      g2.addColorStop(1, 'rgba(34,181,244,0)');
      ctx.fillStyle = g2;
      ctx.fillRect(0, 0, this.W, this.H);
    }
  }

  private drawEarlyMark(ms: number): void {
    if (!this.markImg || ms > T.shatterEnd) {
      return;
    }
    const ctx = this.ctx;
    const L = this.layout();
    let alpha = smoothstep(0, 1000, ms);
    let scale = 1;
    let glow = smoothstep(180, 1300, ms);

    if (ms >= IMPACT_MS) {
      const dissolve = smoothstep(IMPACT_MS, IMPACT_MS + 520, ms);
      alpha *= 1 - dissolve;
      scale = lerp(1.05, 1.18, dissolve);
      glow = lerp(1.2, 0.2, dissolve);
    } else if (ms > T.logoInEnd) {
      const approach = smoothstep(T.logoInEnd, IMPACT_MS, ms);
      glow = lerp(glow, 1.25, approach);
      scale = lerp(1, 1.05, approach);
    }

    if (alpha < 0.02) {
      return;
    }
    const w = L.markW * scale;
    const h = L.markH * scale;
    const x = L.cx;
    const y = L.cy;

    const rg = ctx.createRadialGradient(x, y, w * 0.08, x, y, w * 0.95);
    rg.addColorStop(0, `rgba(34,181,244,${0.32 * glow * alpha})`);
    rg.addColorStop(0.5, `rgba(34,181,244,${0.1 * glow * alpha})`);
    rg.addColorStop(1, 'rgba(34,181,244,0)');
    ctx.fillStyle = rg;
    ctx.beginPath();
    ctx.arc(x, y, w, 0, Math.PI * 2);
    ctx.fill();

    ctx.save();
    ctx.globalAlpha = alpha;
    ctx.drawImage(this.markImg, x - w / 2, y - h / 2, w, h);
    ctx.restore();
  }

  private drawLockupMark(ms: number): void {
    if (!this.markImg || ms < T.formEnd) {
      return;
    }
    const ctx = this.ctx;
    const L = this.layout();
    const u = clamp((ms - T.formEnd) / (T.lockupEnd - T.formEnd), 0, 1);
    const e = easeOutCubic(u);
    const alpha = smoothstep(0.08, 0.48, u);
    const scale = lerp(0.9, 1, easeOutBack(clamp(u * 1.05, 0, 1)));
    const x = lerp(L.markCx - 36, L.markCx, e);
    const y = L.cy;
    const w = L.markW * scale;
    const h = L.markH * scale;

    ctx.save();
    ctx.globalAlpha = alpha;
    ctx.drawImage(this.markImg, x - w / 2, y - h / 2, w, h);
    ctx.restore();
  }

  private drawMeteor(ms: number): void {
    const m = this.meteorAt(ms);
    if (!m.visible) {
      return;
    }
    const ctx = this.ctx;
    const len = clamp(Math.min(this.W, this.H) * 0.28, 150, 340);
    ctx.save();
    ctx.translate(m.x, m.y);
    ctx.rotate(m.ang);

    const lg = ctx.createLinearGradient(-len, 0, 0, 0);
    lg.addColorStop(0, 'rgba(34,181,244,0)');
    lg.addColorStop(0.35, 'rgba(34,181,244,0.2)');
    lg.addColorStop(0.85, 'rgb(34,181,244)');
    lg.addColorStop(1, 'rgb(243,247,251)');
    ctx.fillStyle = lg;
    ctx.beginPath();
    ctx.moveTo(-len, -1.1);
    ctx.lineTo(-8, -2.6);
    ctx.lineTo(0, -2.2);
    ctx.lineTo(0, 2.2);
    ctx.lineTo(-8, 2.6);
    ctx.lineTo(-len, 1.1);
    ctx.closePath();
    ctx.fill();

    const halo = ctx.createRadialGradient(0, 0, 0, 0, 0, 18);
    halo.addColorStop(0, 'rgb(243,247,251)');
    halo.addColorStop(0.15, 'rgba(34,181,244,0.65)');
    halo.addColorStop(0.7, 'rgba(34,181,244,0)');
    ctx.fillStyle = halo;
    ctx.beginPath();
    ctx.arc(0, 0, 18, 0, Math.PI * 2);
    ctx.fill();
    ctx.restore();

    if (this.playing && ms > this.lastTrailMs + 16 && m.e > 0.02 && m.e < 0.98) {
      this.lastTrailMs = ms;
      const nx = Math.cos(m.ang);
      const ny = Math.sin(m.ang);
      this.trail.push({
        x: m.x - nx * rand(0, 36),
        y: m.y - ny * rand(0, 36),
        born: ms,
        life: 220 + rand(0, 220),
        size: rand(0.8, 2.3),
      });
      if (this.trail.length > 90) {
        this.trail.shift();
      }
    }
  }

  private drawImpact(ms: number): void {
    if (ms < IMPACT_MS || ms > IMPACT_MS + 750) {
      return;
    }
    const ctx = this.ctx;
    const L = this.layout();
    const u = (ms - IMPACT_MS) / 750;
    const rad = lerp(24, Math.min(this.W, this.H) * 0.58, easeOutCubic(u));
    const alpha = (1 - u) * (1 - u) * 0.9;
    const g = ctx.createRadialGradient(L.cx, L.cy, 0, L.cx, L.cy, rad);
    g.addColorStop(0, `rgba(243,247,251,${alpha})`);
    g.addColorStop(0.18, `rgba(34,181,244,${alpha * 0.75})`);
    g.addColorStop(1, 'rgba(34,181,244,0)');
    ctx.fillStyle = g;
    ctx.beginPath();
    ctx.arc(L.cx, L.cy, rad, 0, Math.PI * 2);
    ctx.fill();

    if (!this.sparks.length) {
      this.buildSparks();
    }
    for (const s of this.sparks) {
      const age = (ms - IMPACT_MS) / 1000;
      if (age > s.life) {
        continue;
      }
      const uu = age / s.life;
      const dist = s.sp * easeOutCubic(Math.min(1, age / 0.35)) * (1 - uu * 0.25);
      const x = L.cx + Math.cos(s.ang) * dist;
      const y = L.cy + Math.sin(s.ang) * dist;
      ctx.globalAlpha = 1 - uu;
      ctx.fillStyle = uu < 0.25 ? '#f3f7fb' : '#22b5f4';
      ctx.beginPath();
      ctx.arc(x, y, s.size * (1 - uu * 0.55), 0, Math.PI * 2);
      ctx.fill();
    }
    ctx.globalAlpha = 1;
  }

  private drawTrail(ms: number): void {
    const ctx = this.ctx;
    for (let i = this.trail.length - 1; i >= 0; i--) {
      const s = this.trail[i];
      const age = ms - s.born;
      if (age < 0 || age > s.life) {
        this.trail.splice(i, 1);
        continue;
      }
      const u = age / s.life;
      ctx.globalAlpha = (1 - u) * 0.65;
      ctx.fillStyle = '#8adefb';
      ctx.beginPath();
      ctx.arc(s.x, s.y, s.size * (1 - u * 0.4), 0, Math.PI * 2);
      ctx.fill();
    }
    ctx.globalAlpha = 1;
  }

  private drawParticles(ms: number): void {
    const blend = this.cleanTextBlend(ms);
    if (blend >= 0.999) {
      return;
    }
    const particleAlpha = 1 - blend;
    const ctx = this.ctx;

    for (const p of this.particles) {
      const pose = this.poseParticle(p, ms);
      const a = pose.a * p.bright * particleAlpha;
      if (a < 0.02) {
        continue;
      }
      ctx.globalAlpha = a;
      ctx.fillStyle = `hsla(${p.hue}, 95%, 70%, 1)`;
      ctx.beginPath();
      ctx.arc(pose.x, pose.y, pose.size, 0, Math.PI * 2);
      ctx.fill();
    }
    ctx.globalAlpha = 1;
  }

  private drawCleanSTX(ms: number): void {
    const blend = this.cleanTextBlend(ms);
    if (blend < 0.01) {
      return;
    }
    const ctx = this.ctx;
    const L = this.layout();
    ctx.save();
    ctx.globalAlpha = blend;
    ctx.shadowBlur = 0;
    ctx.shadowColor = 'transparent';
    ctx.fillStyle = '#eaf7ff';
    ctx.font = brandFont(L.fontSize);
    ctx.textAlign = 'center';
    ctx.textBaseline = 'middle';
    ctx.fillText('STX', L.textCx, L.textBaselineY);
    ctx.restore();
  }

  private drawWelcome(ms: number): void {
    let alpha: number;
    if (ms >= T.lockupEnd) {
      alpha = 1;
    } else {
      alpha = smoothstep(T.welcomeStart, T.welcomeEnd, ms);
    }
    if (alpha < 0.01) {
      return;
    }
    const ctx = this.ctx;
    const L = this.layout();
    ctx.save();
    ctx.globalAlpha = alpha * 0.88;
    ctx.shadowBlur = 0;
    ctx.fillStyle = '#9aa8b5';
    ctx.font = welcomeFont(L.welcomeSize);
    ctx.textAlign = 'center';
    ctx.textBaseline = 'middle';
    ctx.fillText(this.welcomeText, L.cx, L.welcomeY);
    ctx.restore();
  }

  private render(ms: number): void {
    if (this.destroyed) {
      return;
    }
    this.drawBackground(ms);
    this.drawEarlyMark(ms);
    this.drawMeteor(ms);
    this.drawImpact(ms);
    this.drawTrail(ms);
    this.drawParticles(ms);
    this.drawCleanSTX(ms);
    this.drawLockupMark(ms);
    this.drawWelcome(ms);
  }
}
