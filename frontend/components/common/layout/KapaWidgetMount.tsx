'use client';

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

import Image from 'next/image';
import {useEffect, useState} from 'react';
import {useTranslations} from 'next-intl';
import {usePathname} from 'next/navigation';
import {toast} from 'sonner';
import {cn} from '@/lib/utils';
import {
  ensureKapaWidget,
  isKapaAskAiAllowed,
  openSeaTunnelAskAi,
  startKapaThemeSync,
  KAPA_PROJECT_MARK_PATH,
} from '@/lib/services/kapa-ai';

/**
 * Theme-aware Ask AI launcher: mounts Kapa (hidden default button) and shows an STX-styled FAB.
 * 跟随主题的 Ask AI 启动器：挂载 Kapa（隐藏默认球）并展示 STX 风格悬浮按钮。
 */
export function KapaWidgetMount() {
  const pathname = usePathname();
  const t = useTranslations('plugin');
  const [allowed, setAllowed] = useState(false);
  const [opening, setOpening] = useState(false);

  useEffect(() => {
    if (!isKapaAskAiAllowed()) {
      setAllowed(false);
      return;
    }
    setAllowed(true);
    void ensureKapaWidget().then(() => {
      startKapaThemeSync();
    });
  }, []);

  // 在工作台页面隐去右下角全局悬浮球，避免遮挡右侧属性栏与控制台
  // Hide global floating FAB on workbench page to prevent blocking sidebar and console
  if (!allowed || pathname === '/workbench') {
    return null;
  }

  const handleOpen = async () => {
    if (opening) {
      return;
    }
    setOpening(true);
    try {
      const success = await openSeaTunnelAskAi();
      if (!success) {
        toast.error(t('askAiUnavailable'), {
          description: t('askAiUnavailableDesc'),
        });
      }
    } finally {
      setOpening(false);
    }
  };

  return (
    <button
      type='button'
      data-testid='stx-ask-ai-fab'
      aria-label={t('askAi')}
      title={t('askAiTooltip')}
      onClick={() => {
        void handleOpen();
      }}
      disabled={opening}
      className={cn(
        // Sit above mobile dock / clear of centered desktop dock
        // 高于移动端 Dock，并避开桌面居中 Dock
        'fixed z-40 bottom-[max(5.5rem,env(safe-area-inset-bottom))] right-4 md:bottom-6 md:right-6',
        'group flex items-center gap-2 rounded-2xl border border-border/70',
        'bg-background/85 text-foreground shadow-lg backdrop-blur-xl',
        'px-3 py-2.5 transition-all duration-200',
        'hover:bg-accent hover:border-border hover:shadow-xl',
        'active:scale-[0.98] disabled:opacity-60',
        'dark:bg-card/90 dark:border-border/50 dark:hover:bg-accent/80',
      )}
    >
      <span className='relative flex h-7 w-7 items-center justify-center overflow-hidden rounded-lg bg-muted/60 ring-1 ring-border/50'>
        <Image
          src={KAPA_PROJECT_MARK_PATH}
          alt=''
          width={28}
          height={28}
          className='h-6 w-6 object-contain'
          draggable={false}
          priority={false}
        />
      </span>
      <span className='pr-0.5 text-xs font-medium tracking-wide whitespace-nowrap'>
        {t('askAi')}
      </span>
    </button>
  );
}
