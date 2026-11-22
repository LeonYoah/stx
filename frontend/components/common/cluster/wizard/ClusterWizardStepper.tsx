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
 * Thin wrapper around Arc Stepper that supplies localized labels,
 * error states, and completed-step navigation for the cluster wizard.
 * Arc Stepper 的薄封装：注入本地化文案、错误态与已完成步骤回跳。
 */

'use client';

import {useTranslations} from 'next-intl';
import {Stepper} from '@/components/arc/stepper/stepper';
import type {StepperStep} from '@/components/arc/stepper/stepper';
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
  /** Per-step error text shown under the label / 按步骤 id 展示在标签下的错误文案 */
  stepErrors?: Partial<Record<string, string>>;
  /** All steps done (complete page) / 到达完成步后点亮全部步骤 */
  allComplete?: boolean;
  /**
   * Read-only phase (deploy/complete step): completed steps are not reachable.
   * 只读阶段（部署中、部署步、完成步）：不允许回跳已完成步骤。
   */
  readOnly?: boolean;
}

export function ClusterWizardStepper({
  steps,
  currentStepIndex,
  onStepClick,
  deploying = false,
  stepErrors,
  allComplete = false,
  readOnly = false,
}: ClusterWizardStepperProps) {
  const t = useTranslations();

  const arcSteps: StepperStep[] = steps.map((step) => ({
    id: step.id,
    label: t(step.titleKey),
    error: stepErrors?.[step.id],
  }));

  const interactive = !deploying && !readOnly && Boolean(onStepClick);

  return (
    <div className='w-full px-6 py-2.5 border-b-[0.5px] border-border/80'>
      <Stepper
        steps={arcSteps}
        current={allComplete ? steps.length : currentStepIndex}
        onStepSelect={interactive ? onStepClick : undefined}
        label={t('cluster.wizard.stepperLabel')}
        completeLabel={t('cluster.wizard.allStepsComplete')}
        statusLabels={{
          complete: t('cluster.wizard.stepStatusComplete'),
          upcoming: t('cluster.wizard.stepStatusUpcoming'),
          error: t('cluster.wizard.stepStatusError'),
        }}
        formatStepAnnouncement={(active, count, label) =>
          t('cluster.wizard.stepAnnouncement', {active, count, label})
        }
      />
    </div>
  );
}

export default ClusterWizardStepper;
