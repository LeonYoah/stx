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

import {useRef} from 'react';
import {useTranslations} from 'next-intl';
import {Button} from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import type {OnboardingPath} from '@/lib/onboarding/types';
import {useOnboarding} from './OnboardingContext';

/**
 * First-screen path chooser as accessible Dialog (focus trap / Esc = skip).
 * 首屏路径选择：标准 Dialog（焦点陷阱；Esc = 暂时跳过，不导航）。
 */
export function OnboardingPathChooser() {
  const t = useTranslations('onboarding');
  const {ready, state, selectPath, setDisplayMode} = useOnboarding();
  // Distinguish path pick vs Esc/skip so controlled close does not overwrite expanded.
  // 区分选路径与 Esc/跳过，避免受控关闭把 expanded 盖成 hidden。
  const closeReasonRef = useRef<'path' | 'skip' | null>(null);

  const open =
    ready && !state.finished && !state.path && state.displayMode !== 'hidden';

  const choosePath = (path: OnboardingPath) => {
    closeReasonRef.current = 'path';
    selectPath(path);
  };

  const skipForNow = () => {
    closeReasonRef.current = 'skip';
    setDisplayMode('hidden');
  };

  return (
    <Dialog
      open={open}
      onOpenChange={(nextOpen) => {
        if (nextOpen) {
          return;
        }
        const reason = closeReasonRef.current;
        closeReasonRef.current = null;
        // Path selection already set displayMode=expanded via selectPath.
        // 选路径时 selectPath 已写入 expanded，勿再 hidden。
        if (reason === 'path') {
          return;
        }
        // Esc / overlay / X / explicit skip = 暂时跳过（仅隐藏，不导航）
        // Esc / overlay / X / skip = hide guide only, no navigation
        if (reason !== 'skip') {
          setDisplayMode('hidden');
        }
      }}
    >
      <DialogContent
        className='max-w-xl border-border/60 sm:max-w-xl'
        data-testid='stx-onboarding-path-chooser'
        showCloseButton
      >
        <DialogHeader>
          <DialogTitle id='stx-onboarding-path-title'>
            {t('pathTitle')}
          </DialogTitle>
          <DialogDescription id='stx-onboarding-path-desc'>
            {t('pathSubtitle')}
          </DialogDescription>
        </DialogHeader>
        <div className='grid gap-3 sm:grid-cols-2'>
          <button
            type='button'
            className='rounded-xl border border-border/60 bg-background p-4 text-left transition-colors hover:border-foreground/40'
            data-testid='stx-onboarding-path-existing'
            onClick={() => choosePath('existing')}
          >
            <div className='text-sm font-semibold'>{t('pathExistingTitle')}</div>
            <p className='mt-2 min-h-10 text-xs text-muted-foreground'>
              {t('pathExistingDesc')}
            </p>
            <span className='mt-3 inline-flex h-8 items-center rounded-md bg-primary px-3 text-xs text-primary-foreground'>
              {t('pathExistingCta')}
            </span>
          </button>
          <button
            type='button'
            className='rounded-xl border border-border/60 bg-background p-4 text-left transition-colors hover:border-foreground/40'
            data-testid='stx-onboarding-path-new'
            onClick={() => choosePath('new')}
          >
            <div className='text-sm font-semibold'>{t('pathNewTitle')}</div>
            <p className='mt-2 min-h-10 text-xs text-muted-foreground'>
              {t('pathNewDesc')}
            </p>
            <span className='mt-3 inline-flex h-8 items-center rounded-md bg-primary px-3 text-xs text-primary-foreground'>
              {t('pathNewCta')}
            </span>
          </button>
        </div>
        <Button
          type='button'
          variant='link'
          className='mt-1 h-auto justify-start px-0 text-xs text-muted-foreground'
          data-testid='stx-onboarding-path-skip'
          onClick={skipForNow}
        >
          {t('pathSkip')}
        </Button>
      </DialogContent>
    </Dialog>
  );
}
