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

import {useMemo} from 'react';
import {useRouter} from 'next/navigation';
import {useTranslations} from 'next-intl';
import {OnboardingChecklist} from '@/components/arc/onboarding-checklist/onboarding-checklist';
import type {OnboardingStep} from '@/components/arc/onboarding-checklist/onboarding-checklist';
import {Button} from '@/components/ui/button';
import {
  formatOnboardingLastError,
  formatStepStatusDetail,
} from '@/lib/onboarding/messages';
import {
  ONBOARDING_STEP_IDS,
  type OnboardingStepId,
} from '@/lib/onboarding/types';
import {cn} from '@/lib/utils';
import {useOnboarding} from './OnboardingContext';

/**
 * Floating Arc checklist with STX three-state shell (no plan/bindings card).
 * Arc 浮窗清单 + STX 三态外壳（无目标与计划资源卡片）。
 */
export function OnboardingChecklistFloat() {
  const t = useTranslations('onboarding');
  const router = useRouter();
  const {
    ready,
    state,
    completedIds,
    stepResults,
    checking,
    lastError,
    setDisplayMode,
    refresh,
  } = useOnboarding();

  const title = state.path === 'new' ? t('titleNew') : t('titleExisting');

  const steps: OnboardingStep[] = useMemo(() => {
    if (!state.path) {
      return [];
    }
    const resultMap = new Map(stepResults.map((r) => [r.id, r]));
    const describe = (id: OnboardingStepId, baseKey: string, doneKey: string) => {
      const result = resultMap.get(id);
      if (result?.status === 'complete') {
        return t(doneKey);
      }
      const parts = [t(`${baseKey}Desc`)];
      if (
        result &&
        (result.status === 'error' ||
          result.errorMessage ||
          result.missing?.length)
      ) {
        parts.push(
          formatStepStatusDetail(
            t,
            result.status,
            result.errorMessage,
            result.missing,
          ),
        );
      } else if (checking && !result) {
        parts.push(t('checking'));
      }
      return parts.filter(Boolean).join('\n');
    };

    const list: OnboardingStep[] = [];
    if (state.path === 'new') {
      list.push({
        id: ONBOARDING_STEP_IDS.resources,
        title: t('stepResources'),
        description: describe(
          ONBOARDING_STEP_IDS.resources,
          'stepResources',
          'stepResourcesDone',
        ),
        actionLabel: t('goPrepare'),
        externalCompletion: true,
        onAction: async () => {
          router.push('/packages');
        },
      });
    }
    list.push(
      {
        id: ONBOARDING_STEP_IDS.hostsAgents,
        title: t('stepHosts'),
        description: describe(
          ONBOARDING_STEP_IDS.hostsAgents,
          'stepHosts',
          'stepHostsDone',
        ),
        actionLabel: t('goAddServers'),
        externalCompletion: true,
        onAction: async () => {
          router.push('/hosts');
        },
      },
      {
        id: ONBOARDING_STEP_IDS.cluster,
        title:
          state.path === 'new' ? t('stepClusterNew') : t('stepClusterExisting'),
        description: describe(
          ONBOARDING_STEP_IDS.cluster,
          state.path === 'new' ? 'stepClusterNew' : 'stepClusterExisting',
          'stepClusterDone',
        ),
        actionLabel:
          state.path === 'new' ? t('goInstallCluster') : t('goAddCluster'),
        externalCompletion: true,
        onAction: async () => {
          router.push('/clusters');
        },
      },
      {
        id: ONBOARDING_STEP_IDS.firstScript,
        title: t('stepFirstScript'),
        description: describe(
          ONBOARDING_STEP_IDS.firstScript,
          'stepFirstScript',
          'stepFirstScriptDone',
        ),
        actionLabel: t('goWorkbench'),
        externalCompletion: true,
        onAction: async () => {
          router.push('/workbench');
        },
      },
    );
    return list;
  }, [state.path, stepResults, checking, t, router]);

  if (!ready || !state.path || state.displayMode === 'hidden') {
    return null;
  }

  const effectiveCompleted =
    state.finished && state.completedStepIds.length > 0
      ? state.completedStepIds
      : completedIds;
  const doneCount = effectiveCompleted.length;
  const total = steps.length;
  const lastErrorText = formatOnboardingLastError(t, lastError);

  if (state.displayMode === 'collapsed') {
    return (
      <button
        type='button'
        data-testid='stx-onboarding-pill'
        className={cn(
          // Capsule exception: rounded-full for collapsed guide pill.
          // 胶囊例外：收起引导使用 rounded-full。
          'fixed z-[41] flex items-center gap-2 rounded-full border border-border/60 bg-card/95 px-3 py-2 shadow-lg backdrop-blur',
          'right-4 bottom-[calc(max(5.5rem,env(safe-area-inset-bottom))+3.25rem)] md:right-6 md:bottom-[5.5rem]',
        )}
        onClick={() => setDisplayMode('expanded')}
        aria-label={t('pillAria', {done: doneCount, total})}
      >
        <span className='text-xs font-semibold'>{t('pillTitle')}</span>
        <span className='text-[11px] text-muted-foreground'>
          {t('pillMeta', {done: doneCount, total})}
        </span>
      </button>
    );
  }

  return (
    <div
      data-testid='stx-onboarding-float'
      className={cn(
        'fixed z-[41] flex w-[min(360px,calc(100vw-1.5rem))] flex-col gap-2 overflow-y-auto overscroll-contain',
        'right-4 bottom-[calc(max(5.5rem,env(safe-area-inset-bottom))+3.25rem)] md:right-6 md:bottom-[5.5rem]',
        'max-h-[min(70vh,560px)]',
      )}
    >
      <div className='rounded-xl'>
        <OnboardingChecklist
          title={title}
          doneTitle={t('doneTitle')}
          steps={steps}
          completedIds={effectiveCompleted}
          hideLabel={t('collapse')}
          showLabel={t('expand')}
          fullyHideLabel={t('fullyHide')}
          countText={t('countText', {done: doneCount, total})}
          completedLabel={t('completedLabel')}
          completedGroupText={t('completedGroupText', {
            count: effectiveCompleted.length,
          })}
          progressAriaLabel={t('progressAriaLabel')}
          progressValueText={t('progressValueText', {done: doneCount, total})}
          stepsAriaLabel={t('stepsAriaLabel')}
          completedStepsAriaLabel={t('completedStepsAriaLabel')}
          externalCheckAriaLabel={t('externalCheckAriaLabel')}
          statusAllDone={t('statusAllDone', {total})}
          formatStatusStepDone={({title: stepTitle, done, total: tot}) =>
            t('statusStepDone', {title: stepTitle, done, total: tot})
          }
          actionFailedMessage={t('actionFailed')}
          onCollapseRequest={() => setDisplayMode('collapsed')}
          onFullyHideRequest={() => setDisplayMode('hidden')}
          className='shadow-xl border-border/60'
        />
      </div>

      <div
        className='flex items-center justify-between gap-2 rounded-xl border border-border/60 bg-card/95 px-3 py-2 text-xs shadow-lg backdrop-blur'
        data-testid='stx-onboarding-status-bar'
      >
        <span className='min-w-0 text-muted-foreground'>
          {checking ? (
            t('checking')
          ) : lastErrorText ? (
            <span className='text-destructive' role='alert'>
              {lastErrorText}
            </span>
          ) : (
            t('statusIdle')
          )}
        </span>
        <Button
          type='button'
          size='sm'
          variant='outline'
          className='h-7 shrink-0 text-[11px]'
          onClick={() => void refresh()}
          data-testid='stx-onboarding-retry'
        >
          {t('retry')}
        </Button>
      </div>
    </div>
  );
}
