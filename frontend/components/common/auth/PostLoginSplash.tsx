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

import {useCallback, useEffect, useRef} from 'react';
import {useTranslations} from 'next-intl';
import {MeteorIntroEngine} from '@/lib/splash/meteor-intro-engine';
import {shouldReduceMotion} from '@/lib/animations/gsap-motion';

export type PostLoginSplashProps = {
  /** 动画正常结束或跳过时回调 / Called when the splash finishes or is skipped */
  onDismiss: () => void;
};

/**
 * 登录成功后全屏开屏（第四版视觉）。无调试时间轴；支持跳过与 reduced-motion。
 * Full-screen post-login splash (v4 visuals). No debug timeline; supports skip and reduced-motion.
 */
export function PostLoginSplash({onDismiss}: PostLoginSplashProps) {
  const t = useTranslations('auth.splash');
  const canvasRef = useRef<HTMLCanvasElement | null>(null);
  const engineRef = useRef<MeteorIntroEngine | null>(null);
  const skipBtnRef = useRef<HTMLButtonElement | null>(null);
  const previousFocusRef = useRef<HTMLElement | null>(null);
  const dismissedRef = useRef(false);

  const dismiss = useCallback(() => {
    if (dismissedRef.current) {
      return;
    }
    dismissedRef.current = true;
    engineRef.current?.destroy();
    engineRef.current = null;
    onDismiss();
  }, [onDismiss]);

  const handleSkip = useCallback(() => {
    engineRef.current?.skip();
  }, []);

  useEffect(() => {
    previousFocusRef.current =
      document.activeElement instanceof HTMLElement
        ? document.activeElement
        : null;

    const canvas = canvasRef.current;
    if (!canvas) {
      dismiss();
      return;
    }

    let engine: MeteorIntroEngine;
    try {
      engine = new MeteorIntroEngine({
        canvas,
        welcomeText: t('welcomeBack'),
        reducedMotion: shouldReduceMotion(),
        onComplete: dismiss,
        onFatalError: (error) => {
          console.error('Post-login splash failed:', error);
          dismiss();
        },
      });
    } catch (error) {
      console.error('Post-login splash init failed:', error);
      dismiss();
      return;
    }

    engineRef.current = engine;
    engine.start();

    // 首焦落到可跳过控件，保证键盘可达 / Move initial focus to the skip control for keyboard access
    skipBtnRef.current?.focus();

    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        event.preventDefault();
        handleSkip();
        return;
      }
      // 简易焦点陷阱：Tab 循环停留在跳过按钮（唯一可聚焦控件）
      // Lightweight focus trap: Tab cycles on the sole focusable skip control
      if (event.key === 'Tab') {
        event.preventDefault();
        skipBtnRef.current?.focus();
      }
    };
    window.addEventListener('keydown', onKeyDown);

    return () => {
      window.removeEventListener('keydown', onKeyDown);
      engine.destroy();
      engineRef.current = null;
      const prev = previousFocusRef.current;
      if (prev && document.contains(prev)) {
        prev.focus();
      }
      previousFocusRef.current = null;
    };
    // welcome 文案随语言变化由下方 effect 同步；此处仅挂载引擎一次
    // Locale caption is synced in a separate effect; mount the engine once here
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [dismiss, handleSkip]);

  useEffect(() => {
    engineRef.current?.setWelcomeText(t('welcomeBack'));
  }, [t]);

  return (
    <div
      className='fixed inset-0 z-[100] bg-[#05070c]'
      role='dialog'
      aria-modal='true'
      aria-label={t('ariaLabel')}
    >
      <canvas
        ref={canvasRef}
        className='absolute inset-0 h-full w-full'
        aria-hidden='true'
      />
      {/* 装饰性 Canvas：对读屏隐藏，文案由 dialog 标签与欢迎字提供 */}
      {/* Decorative canvas: hidden from AT; dialog label + welcome text carry meaning */}
      <span className='sr-only'>{t('statusAnnouncement')}</span>
      <button
        ref={skipBtnRef}
        type='button'
        onClick={handleSkip}
        className='absolute right-4 top-4 z-[101] rounded-md border border-white/20 bg-black/40 px-3 py-1.5 text-sm text-white/90 backdrop-blur-sm transition-colors hover:bg-black/55 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[#22b5f4]'
      >
        {t('skip')}
      </button>
    </div>
  );
}

export default PostLoginSplash;
