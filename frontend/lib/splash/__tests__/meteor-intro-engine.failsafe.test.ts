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

import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest';
import {
  ASSET_LOAD_TIMEOUT_MS,
  MeteorIntroEngine,
} from '../meteor-intro-engine';

/** 永不触发 load/error 的 Image 替身 / Image stub that never fires load or error */
class HangImage {
  onload: ((ev?: Event) => void) | null = null;
  onerror: ((ev?: Event) => void) | null = null;
  decoding = '';
  naturalWidth = 0;
  naturalHeight = 0;
  width = 0;
  height = 0;
  set src(_value: string) {
    // intentionally never call onload/onerror
  }
}

describe('MeteorIntroEngine fail-safe (hang timeout / destroy / frame errors)', () => {
  const originalImage = globalThis.Image;
  let canvas: HTMLCanvasElement;

  beforeEach(() => {
    vi.useFakeTimers();
    // @ts-expect-error test stub
    globalThis.Image = HangImage;
    canvas = document.createElement('canvas');
    // jsdom 无真实 2d；统一注入最小可用 context，避免控制台噪音
    // jsdom has no real 2d context; always inject a minimal usable stub to avoid console noise
    const ctx = {
      canvas,
      fillStyle: '',
      globalAlpha: 1,
      font: '',
      textAlign: 'center',
      textBaseline: 'middle',
      shadowBlur: 0,
      shadowColor: '',
      setTransform: vi.fn(),
      fillRect: vi.fn(),
      clearRect: vi.fn(),
      beginPath: vi.fn(),
      closePath: vi.fn(),
      moveTo: vi.fn(),
      lineTo: vi.fn(),
      arc: vi.fn(),
      fill: vi.fn(),
      save: vi.fn(),
      restore: vi.fn(),
      translate: vi.fn(),
      rotate: vi.fn(),
      drawImage: vi.fn(),
      fillText: vi.fn(),
      measureText: vi.fn(() => ({width: 120})),
      createLinearGradient: vi.fn(() => ({
        addColorStop: vi.fn(),
      })),
      createRadialGradient: vi.fn(() => ({
        addColorStop: vi.fn(),
      })),
      getImageData: vi.fn(() => ({
        data: new Uint8ClampedArray(4),
        width: 1,
        height: 1,
      })),
    };
    vi.spyOn(canvas, 'getContext').mockReturnValue(
      ctx as unknown as CanvasRenderingContext2D,
    );
  });

  afterEach(() => {
    vi.useRealTimers();
    globalThis.Image = originalImage;
    vi.restoreAllMocks();
  });

  it('starts timeline after hang timeout when images never load or error', () => {
    const onComplete = vi.fn();
    const engine = new MeteorIntroEngine({
      canvas,
      welcomeText: '欢迎回来',
      reducedMotion: true,
      assetLoadTimeoutMs: 100,
      onComplete,
    });

    engine.start();
    expect(onComplete).not.toHaveBeenCalled();

    // 超时后应按失败降级并进入 reduced-motion 终态 hold，而不是永久挂起
    // After timeout, degrade and enter reduced-motion final hold instead of hanging forever
    vi.advanceTimersByTime(100);
    expect(onComplete).not.toHaveBeenCalled();

    vi.advanceTimersByTime(1200);
    expect(onComplete).toHaveBeenCalledTimes(1);
    engine.destroy();
  });

  it('destroy clears asset-load timer so late timeout cannot finish after teardown', () => {
    const onComplete = vi.fn();
    const engine = new MeteorIntroEngine({
      canvas,
      welcomeText: 'Welcome back',
      reducedMotion: true,
      assetLoadTimeoutMs: ASSET_LOAD_TIMEOUT_MS,
      onComplete,
    });

    engine.start();
    engine.destroy();
    vi.advanceTimersByTime(ASSET_LOAD_TIMEOUT_MS + 5000);
    expect(onComplete).not.toHaveBeenCalled();
  });

  it('frame drawing exceptions call fail-safe onComplete instead of freezing', async () => {
    const onComplete = vi.fn();
    const onFatalError = vi.fn();

    const boomCtx = {
      canvas,
      fillStyle: '',
      globalAlpha: 1,
      font: '',
      textAlign: 'center',
      textBaseline: 'middle',
      shadowBlur: 0,
      shadowColor: '',
      setTransform: vi.fn(),
      fillRect: vi.fn(),
      clearRect: vi.fn(),
      beginPath: vi.fn(),
      closePath: vi.fn(),
      moveTo: vi.fn(),
      lineTo: vi.fn(),
      arc: vi.fn(),
      fill: vi.fn(),
      save: vi.fn(),
      restore: vi.fn(),
      translate: vi.fn(),
      rotate: vi.fn(),
      drawImage: vi.fn(),
      fillText: vi.fn(),
      measureText: vi.fn(() => ({width: 120})),
      createLinearGradient: vi.fn(() => ({addColorStop: vi.fn()})),
      createRadialGradient: vi.fn(() => ({addColorStop: vi.fn()})),
      getImageData: vi.fn(() => ({
        data: new Uint8ClampedArray(4),
        width: 1,
        height: 1,
      })),
    };

    let explodeOnFill = false;
    boomCtx.fillRect.mockImplementation(() => {
      if (explodeOnFill) {
        throw new Error('draw boom');
      }
    });

    vi.spyOn(canvas, 'getContext').mockReturnValue(
      boomCtx as unknown as CanvasRenderingContext2D,
    );

    // Instant-resolving images so timeline starts without hang timeout
    class InstantImage {
      onload: ((ev?: Event) => void) | null = null;
      onerror: ((ev?: Event) => void) | null = null;
      decoding = '';
      naturalWidth = 48;
      naturalHeight = 36;
      set src(_value: string) {
        queueMicrotask(() => this.onload?.(new Event('load')));
      }
    }
    // @ts-expect-error test stub
    globalThis.Image = InstantImage;

    const rafCallbacks: FrameRequestCallback[] = [];
    vi.stubGlobal(
      'requestAnimationFrame',
      (cb: FrameRequestCallback) => {
        rafCallbacks.push(cb);
        return rafCallbacks.length;
      },
    );
    vi.stubGlobal('cancelAnimationFrame', vi.fn());

    const engine = new MeteorIntroEngine({
      canvas,
      welcomeText: '欢迎回来',
      reducedMotion: false,
      onComplete,
      onFatalError,
    });
    engine.start();

    await Promise.resolve();
    await Promise.resolve();

    // flush image microtasks → tryStart → schedule first rAF
    expect(rafCallbacks.length).toBeGreaterThan(0);
    explodeOnFill = true;
    const first = rafCallbacks.shift()!;
    expect(() => first(16)).not.toThrow();
    expect(onFatalError).toHaveBeenCalled();
    expect(onComplete).toHaveBeenCalledTimes(1);
    engine.destroy();
  });
});
