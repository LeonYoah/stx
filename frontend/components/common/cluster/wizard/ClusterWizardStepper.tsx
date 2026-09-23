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
 * Cluster Wizard Stepper Component
 * 集群部署向导步进器组件
 *
 * High-density, elegant horizontal step indicator for cluster wizard.
 * 集群向导高密度、现代感横向步骤指示器。
 */

'use client';

import React from 'react';
import {useTranslations} from 'next-intl';
import {Check} from 'lucide-react';
import {cn} from '@/lib/utils';
import {StepConfig} from './types';

interface ClusterWizardStepperProps {
  /** Step configurations / 步骤配置列表 */
  steps: StepConfig[];
  /** Current step index / 当前步骤索引 */
  currentStepIndex: number;
  /** Callback when completed step is clicked / 点击已完成步骤时的回调 */
  onStepClick?: (index: number) => void;
  /** Whether deployment is currently in progress / 是否正在部署中 */
  deploying?: boolean;
}

export function ClusterWizardStepper({
  steps,
  currentStepIndex,
  onStepClick,
  deploying = false,
}: ClusterWizardStepperProps) {
  const t = useTranslations();

  return (
    <div className='w-full px-6 py-3 border-b bg-muted/10'>
      <div className='flex items-center justify-between'>
        {steps.map((step, index) => {
          const isCompleted = index < currentStepIndex;
          const isCurrent = index === currentStepIndex;
          const isPending = index > currentStepIndex;
          const canClick = isCompleted && !deploying && onStepClick;

          return (
            <React.Fragment key={step.id}>
              {/* Step Item / 单个步骤 */}
              <button
                type='button'
                disabled={!canClick}
                onClick={() => canClick && onStepClick(index)}
                className={cn(
                  'group flex items-center gap-2.5 transition-all text-left outline-none rounded-md px-2 py-1',
                  canClick && 'cursor-pointer hover:bg-background/80',
                  !canClick && 'cursor-default',
                )}
              >
                {/* Step Circle / 步骤圆圈 */}
                <div
                  className={cn(
                    'w-6 h-6 rounded-full flex items-center justify-center text-xs font-semibold transition-all shrink-0',
                    isCompleted &&
                      'bg-primary/15 text-primary border border-primary/30 group-hover:bg-primary/25',
                    isCurrent &&
                      'bg-primary text-primary-foreground shadow-xs ring-2 ring-primary/20',
                    isPending &&
                      'bg-muted/60 text-muted-foreground/60 border border-muted-foreground/20',
                  )}
                >
                  {isCompleted ? (
                    <Check className='w-3.5 h-3.5 stroke-[2.5]' />
                  ) : (
                    <span>{index + 1}</span>
                  )}
                </div>

                {/* Step Label / 步骤文本 */}
                <div className='hidden sm:block min-w-0'>
                  <p
                    className={cn(
                      'text-xs font-medium truncate transition-colors',
                      isCurrent && 'text-foreground font-semibold',
                      isCompleted &&
                        'text-foreground/80 group-hover:text-primary',
                      isPending && 'text-muted-foreground/50',
                    )}
                  >
                    {t(step.titleKey)}
                  </p>
                </div>
              </button>

              {/* Connecting line / 连接线 */}
              {index < steps.length - 1 && (
                <div
                  className={cn(
                    'flex-1 h-px mx-2 transition-colors',
                    index < currentStepIndex ? 'bg-primary/40' : 'bg-border/60',
                  )}
                />
              )}
            </React.Fragment>
          );
        })}
      </div>
    </div>
  );
}

export default ClusterWizardStepper;
