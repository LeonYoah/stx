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

import {useEffect, useMemo, useRef, useState, type RefObject} from 'react';
import {useTranslations} from 'next-intl';
import {
  Package,
  ArrowUpCircle,
  PencilLine,
  Bot,
  History,
  BugPlay,
} from 'lucide-react';
import {STXLogo, STXLogoDark} from '@/components/icons/logo';

/**
 * 左侧品牌面板鼠标跟随交互。
 * Pointer-follow interaction for the left brand panel.
 */
function useInteractivePanel(panelRef: RefObject<HTMLElement | null>) {
  useEffect(() => {
    const panel = panelRef.current;
    if (!panel) return;

    let raf = 0;
    const current = {x: 50, y: 50, nx: 0, ny: 0};
    const target = {x: 50, y: 50, nx: 0, ny: 0};

    const clamp = (value: number, min: number, max: number) =>
      Math.min(max, Math.max(min, value));

    const render = () => {
      current.x += (target.x - current.x) * 0.08;
      current.y += (target.y - current.y) * 0.08;
      current.nx += (target.nx - current.nx) * 0.08;
      current.ny += (target.ny - current.ny) * 0.08;

      panel.style.setProperty('--mouse-x', `${current.x}%`);
      panel.style.setProperty('--mouse-y', `${current.y}%`);
      panel.style.setProperty('--mouse-x-norm', String(current.nx));
      panel.style.setProperty('--mouse-y-norm', String(current.ny));

      raf = window.requestAnimationFrame(render);
    };

    const onMove = (event: PointerEvent) => {
      const rect = panel.getBoundingClientRect();
      const xPct = ((event.clientX - rect.left) / rect.width) * 100;
      const yPct = ((event.clientY - rect.top) / rect.height) * 100;
      const xNorm = xPct / 100 - 0.5;
      const yNorm = yPct / 100 - 0.5;

      target.x = clamp(xPct, 0, 100);
      target.y = clamp(yPct, 0, 100);
      target.nx = clamp(xNorm, -0.18, 0.18);
      target.ny = clamp(yNorm, -0.18, 0.18);
    };

    const onLeave = () => {
      target.x = 50;
      target.y = 50;
      target.nx = 0;
      target.ny = 0;
    };

    raf = window.requestAnimationFrame(render);
    panel.addEventListener('pointermove', onMove, {passive: true});
    panel.addEventListener('pointerleave', onLeave, {passive: true});

    return () => {
      if (raf) window.cancelAnimationFrame(raf);
      panel.removeEventListener('pointermove', onMove);
      panel.removeEventListener('pointerleave', onLeave);
    };
  }, [panelRef]);
}

/**
 * 能力关键词打字机效果。
 * Typewriter cycle for capability keywords.
 */
function useTypewriter(
  words: string[],
  options: {
    typingSpeed?: number;
    deleteSpeed?: number;
    hold?: number;
    nextHold?: number;
  } = {},
) {
  const {
    typingSpeed = 110,
    deleteSpeed = 56,
    hold = 1000,
    nextHold = 180,
  } = options;
  const [wordIndex, setWordIndex] = useState(0);
  const [display, setDisplay] = useState('');
  const [deleting, setDeleting] = useState(false);

  useEffect(() => {
    if (!words.length) return;
    const currentWord = words[wordIndex % words.length];

    let delay = deleting ? deleteSpeed : typingSpeed;
    if (!deleting && display === currentWord) delay = hold;
    if (deleting && display === '') delay = nextHold;

    const timer = window.setTimeout(() => {
      if (!deleting) {
        if (display.length < currentWord.length) {
          setDisplay(currentWord.slice(0, display.length + 1));
        } else {
          setDeleting(true);
        }
      } else if (display.length > 0) {
        setDisplay(currentWord.slice(0, display.length - 1));
      } else {
        setDeleting(false);
        setWordIndex((prev) => (prev + 1) % words.length);
      }
    }, delay);

    return () => window.clearTimeout(timer);
  }, [
    deleteSpeed,
    deleting,
    display,
    hold,
    nextHold,
    typingSpeed,
    wordIndex,
    words,
  ]);

  return display;
}

/**
 * 背景幻灯片隧道装饰。
 * Decorative slide-tunnel backdrop.
 */
function SlideTunnel() {
  return (
    <div className='stx-bw-slide-tunnel' aria-hidden='true'>
      <div className='stx-bw-slide stx-bw-slide-1'>
        <div className='stx-bw-element' />
        <div className='stx-bw-element-dim' />
        <div className='stx-bw-element-border' />
      </div>

      <div className='stx-bw-slide stx-bw-slide-2'>
        <div className='stx-bw-element' />
        <div className='stx-bw-chart'>
          <div className='stx-bw-bar' />
          <div className='stx-bw-bar' />
          <div className='stx-bw-bar' />
          <div className='stx-bw-bar' />
        </div>
      </div>

      <div className='stx-bw-slide stx-bw-slide-3'>
        <div className='stx-bw-grid-box'>
          <div />
          <div />
          <div />
          <div />
          <div />
          <div />
        </div>
      </div>
    </div>
  );
}

/**
 * 登录页左侧品牌视觉区：品牌、一句定位、三项能力、单行打字机。
 * Left brand panel for login: mark, positioning line, three chips, one typewriter.
 */
export function LoginBrandPanel() {
  const t = useTranslations();
  const panelRef = useRef<HTMLElement>(null);
  useInteractivePanel(panelRef);

  // 词组需保持引用稳定，避免打字机 effect 每帧重置。
  // Keep a stable word list so the typewriter effect does not reset every render.
  const capabilityWords = useMemo(
    () => [
      t('auth.brand.capabilityAgent'),
      t('auth.brand.capabilityCluster'),
      t('auth.brand.capabilityObserve'),
      t('auth.brand.capabilityRecover'),
      t('auth.brand.capabilityConnector'),
      t('auth.brand.capabilityCheckpoint'),
      t('auth.brand.capabilityDag'),
    ],
    [t],
  );
  const typed = useTypewriter(capabilityWords);

  // 能力标签：安装/升级相邻，AI Agent 与 CLI+Skill 合并为一枚。
  // Capability chips: install next to upgrade; AI Agent + CLI/Skill as one chip.
  const chips = [
    {icon: Package, label: t('auth.brand.chipInstall')},
    {icon: ArrowUpCircle, label: t('auth.brand.chipUpgrade')},
    {icon: PencilLine, label: t('auth.brand.chipMarketplace')},
    {icon: Bot, label: t('auth.brand.chipAgent')},
    {icon: History, label: t('auth.brand.chipCheckpoint')},
    {icon: BugPlay, label: t('auth.brand.chipDebugJob')},
  ] as const;

  const footerItems = [
    t('auth.brand.footerDeploy'),
    t('auth.brand.footerSubmit'),
    t('auth.brand.footerAlert'),
    t('auth.brand.footerMarketplace'),
    t('auth.brand.footerAgent'),
  ];

  return (
    <aside ref={panelRef} className='stx-brand-panel'>
      <div className='stx-bw-spotlight' />

      <div className='stx-bw-visual-bg' aria-hidden='true'>
        <div className='stx-bw-grid' />
        <div className='stx-bw-scanner' />
        <SlideTunnel />
      </div>

      <div className='stx-hero-scan-zone' aria-hidden='true'>
        <div className='stx-hero-scan-line' />
      </div>

      <div className='stx-brand-content'>
        <div className='stx-brand-top'>
          <STXLogo
            className='stx-brand-logo stx-brand-logo-light'
            priority
          />
          <STXLogoDark
            className='stx-brand-logo stx-brand-logo-dark'
            priority
          />
          <span className='stx-brand-badge'>{t('auth.brand.badge')}</span>
        </div>

        <div className='stx-brand-main'>
          <h1 className='stx-brand-title'>{t('auth.brand.headline')}</h1>
          <p className='stx-brand-subtitle'>{t('auth.brand.subtitle')}</p>

          <div className='stx-pill-row' aria-label={t('auth.brand.chipsLabel')}>
            {chips.map(({icon: Icon, label}) => (
              <span key={label} className='stx-pill'>
                <Icon className='h-4 w-4' aria-hidden='true' />
                {label}
              </span>
            ))}
          </div>

          <div
            className='stx-type-row'
            aria-label={t('auth.brand.capabilityLabel')}
          >
            <span className='stx-type-label'>
              {t('auth.brand.capabilityPrefix')}
            </span>
            <span className='stx-type-text'>
              {typed || '\u00A0'}
            </span>
            <span className='stx-type-caret' aria-hidden='true' />
          </div>
        </div>

        <div className='stx-brand-footer'>
          {footerItems.map((item) => (
            <span key={item}>{item}</span>
          ))}
        </div>
      </div>
    </aside>
  );
}
