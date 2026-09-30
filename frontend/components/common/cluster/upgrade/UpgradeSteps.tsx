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

import Link from 'next/link';
import {useTranslations} from 'next-intl';
import {Check} from 'lucide-react';
import {cn} from '@/lib/utils';

export type UpgradeStage = 'prepare' | 'config' | 'execute';

interface UpgradeStepsProps {
  /** 当前所处升级阶段 / Current upgrade stage */
  current: UpgradeStage;
  clusterId: number;
}

/**
 * 升级三阶段流程导航（准备 → 配置 → 执行）
 * Three-stage upgrade flow navigator (prepare → config → execute)
 */
export function UpgradeSteps({current, clusterId}: UpgradeStepsProps) {
  const t = useTranslations('stUpgrade');

  const steps: Array<{
    key: UpgradeStage;
    label: string;
    href: string;
  }> = [
    {
      key: 'prepare',
      label: t('prepareStage'),
      href: `/clusters/${clusterId}/upgrade/prepare`,
    },
    {
      key: 'config',
      label: t('configStage'),
      href: `/clusters/${clusterId}/upgrade/config`,
    },
    {
      key: 'execute',
      label: t('executeStage'),
      href: `/clusters/${clusterId}/upgrade/execute`,
    },
  ];

  const currentIndex = steps.findIndex((step) => step.key === current);

  return (
    <nav
      aria-label={t('upgradeStagesAria')}
      className='flex flex-wrap items-center gap-2 rounded-lg border bg-muted/20 px-3 py-2.5'
      data-testid='upgrade-steps'
    >
      {steps.map((step, index) => {
        const isCurrent = step.key === current;
        const isCompleted = index < currentIndex;

        return (
          <div key={step.key} className='flex items-center gap-2'>
            {index > 0 ? (
              <div
                aria-hidden
                className={cn(
                  'hidden h-px w-6 sm:block',
                  isCompleted || isCurrent ? 'bg-primary/50' : 'bg-border',
                )}
              />
            ) : null}
            <Link
              href={step.href}
              aria-current={isCurrent ? 'step' : undefined}
              className={cn(
                'inline-flex items-center gap-2 rounded-md px-2.5 py-1.5 text-sm transition-colors',
                isCurrent &&
                  'bg-background font-medium text-foreground shadow-sm ring-1 ring-border',
                isCompleted &&
                  !isCurrent &&
                  'text-muted-foreground hover:text-foreground',
                !isCompleted &&
                  !isCurrent &&
                  'text-muted-foreground/70 hover:text-muted-foreground',
              )}
            >
              <span
                className={cn(
                  'flex h-5 w-5 shrink-0 items-center justify-center rounded-full text-[11px] font-medium',
                  isCurrent && 'bg-primary text-primary-foreground',
                  isCompleted && !isCurrent && 'bg-primary/15 text-primary',
                  !isCompleted &&
                    !isCurrent &&
                    'bg-muted text-muted-foreground',
                )}
              >
                {isCompleted && !isCurrent ? (
                  <Check className='h-3 w-3' />
                ) : (
                  index + 1
                )}
              </span>
              <span className='whitespace-nowrap'>{step.label}</span>
            </Link>
          </div>
        );
      })}
    </nav>
  );
}
