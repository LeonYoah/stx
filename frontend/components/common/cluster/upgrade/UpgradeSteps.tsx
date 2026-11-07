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
import './upgrade-chrome.css';

export type UpgradeStage = 'prepare' | 'config' | 'execute';

interface UpgradeStepsProps {
  /** 当前所处升级阶段 / Current upgrade stage */
  current: UpgradeStage;
  clusterId: number;
}

/**
 * 升级三阶段导航。外形对齐 Arc 分段轨道：当前步是抬起的选中块，已完成步只留勾。
 * 不用 SegmentedControl：那是互斥模式切换，这里每一步都是独立路由。
 * Three-stage upgrade navigator, shaped like an Arc segmented track.
 * SegmentedControl is a mode switch; each step here is its own route.
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
      className='upgrade-steps'
      data-testid='upgrade-steps'
    >
      {steps.map((step, index) => {
        const isCurrent = step.key === current;
        const isCompleted = index < currentIndex;

        return (
          <Link
            key={step.key}
            href={step.href}
            className='upgrade-steps__link'
            aria-current={isCurrent ? 'step' : undefined}
            data-complete={isCompleted && !isCurrent ? 'true' : undefined}
          >
            <span className='upgrade-steps__index' aria-hidden>
              {isCompleted && !isCurrent ? (
                <Check className='size-3' />
              ) : (
                index + 1
              )}
            </span>
            <span>{step.label}</span>
          </Link>
        );
      })}
    </nav>
  );
}
