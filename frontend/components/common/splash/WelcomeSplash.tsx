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

'use client';

import {useCallback, useEffect, useRef, useState} from 'react';
import {useTranslations} from 'next-intl';
import {
  isWelcomeSplashPending,
  releaseWelcomeSplashCover,
} from '@/lib/welcome-splash';
import {SplashDirector} from './splash-director';
/**
 * 开屏专用展示字体（OFL，本地托管）：Michroma 用于品牌字与英文欢迎语，思源宋体（Noto Serif SC）细字重用于中文欢迎语。
 * 只注册 @font-face，未进入开屏时浏览器不会下载字体文件。
 * Splash-only display fonts (OFL, self-hosted): Michroma for the wordmark,
 * a light Noto Serif SC for the Chinese greeting. Only @font-face rules are
 * registered; files download only when the splash actually renders.
 */
import '@fontsource/michroma/400.css';
import '@fontsource-variable/noto-serif-sc/index.css';
import './welcome-splash.css';

/**
 * 开屏舞台：画布 + 欢迎语 + 跳过按钮，具体画面交给 SplashDirector。
 * Splash stage: canvases, greeting and skip button; visuals live in SplashDirector.
 */
function WelcomeSplashStage({onDone}: {onDone: () => void}) {
  const t = useTranslations('splash');
  const rootRef = useRef<HTMLDivElement>(null);
  const glRef = useRef<HTMLCanvasElement>(null);
  const fxRef = useRef<HTMLCanvasElement>(null);
  const fallbackRef = useRef<HTMLDivElement>(null);
  const welcomeRef = useRef<HTMLDivElement>(null);
  const skipRef = useRef<HTMLButtonElement>(null);
  const directorRef = useRef<SplashDirector | null>(null);
  const onDoneRef = useRef(onDone);
  onDoneRef.current = onDone;

  const greeting = t('welcomeBack');
  const greetingChars = Array.from(greeting);
  // 中文用细字重宋体，英文用宽体字；均为单一银白色 / CJK uses a light serif, Latin the wide display face; both single-tone silver.
  const script = /[\u3400-\u9fff]/.test(greeting) ? 'cjk' : 'latin';

  useEffect(() => {
    const root = rootRef.current;
    const glCanvas = glRef.current;
    const fxCanvas = fxRef.current;
    const fallbackVortex = fallbackRef.current;
    const welcome = welcomeRef.current;
    if (!root || !glCanvas || !fxCanvas || !fallbackVortex || !welcome) {
      onDoneRef.current();
      return;
    }

    const reducedMotion = window.matchMedia(
      '(prefers-reduced-motion: reduce)',
    ).matches;
    let director: SplashDirector;
    try {
      director = new SplashDirector({
        root,
        glCanvas,
        fxCanvas,
        fallbackVortex,
        welcome,
        reducedMotion,
        // 动画层自身不透明直到退场，此时再撤首帧遮罩，全程无首页露出。
        // The overlay is opaque until exit, so the pre-paint cover is released
        // only then and the home page never peeks through.
        onExitStart: releaseWelcomeSplashCover,
        onDone: () => onDoneRef.current(),
      });
    } catch {
      onDoneRef.current();
      return;
    }
    directorRef.current = director;
    director.start();
    skipRef.current?.focus({preventScroll: true});

    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.preventDefault();
        director.skip();
      }
    };
    window.addEventListener('keydown', onKeyDown);

    return () => {
      window.removeEventListener('keydown', onKeyDown);
      director.dispose();
      directorRef.current = null;
    };
  }, []);

  const handleSkip = useCallback(() => {
    directorRef.current?.skip();
  }, []);

  return (
    <div
      ref={rootRef}
      className='stx-splash'
      role='dialog'
      aria-modal='true'
      aria-label={t('ariaLabel')}
      data-testid='welcome-splash'
    >
      <div className='stx-splash__stage'>
        <div className='stx-splash__backdrop' aria-hidden='true' />
        <canvas ref={glRef} className='stx-splash__gl' aria-hidden='true' />
        <div
          ref={fallbackRef}
          className='stx-splash__vortex-fallback'
          aria-hidden='true'
        />
        <canvas ref={fxRef} className='stx-splash__fx' aria-hidden='true' />
        <div ref={welcomeRef} className='stx-splash__welcome'>
          <p
            className='stx-splash__greeting'
            data-script={script}
            aria-live='polite'
          >
            {greetingChars.map((char, index) => (
              <span
                key={`${char}-${index}`}
                className='stx-splash__char'
                style={{'--i': index} as React.CSSProperties}
                aria-hidden='true'
              >
                {char === ' ' ? ' ' : char}
              </span>
            ))}
            <span className='sr-only'>{greeting}</span>
          </p>
          <span className='stx-splash__rule' aria-hidden='true' />
        </div>
      </div>
      <button
        ref={skipRef}
        type='button'
        className='stx-splash__skip'
        onClick={handleSkip}
      >
        <span>{t('skip')}</span>
        <kbd>Esc</kbd>
      </button>
    </div>
  );
}

/**
 * 登录后开屏动画入口（挂在根布局，跨路由常驻）。
 * 是否播放由 <head> 内联脚本在首帧前决定；这里只在水合后接手。
 * Post-login splash entry (mounted in the root layout, persists across routes).
 * The <head> boot script decides before first paint; this only takes over
 * after hydration.
 */
export function WelcomeSplash() {
  const [active, setActive] = useState(false);

  useEffect(() => {
    if (isWelcomeSplashPending()) {
      setActive(true);
    }
  }, []);

  const handleDone = useCallback(() => {
    releaseWelcomeSplashCover();
    setActive(false);
  }, []);

  if (!active) {
    return null;
  }
  return <WelcomeSplashStage onDone={handleDone} />;
}
