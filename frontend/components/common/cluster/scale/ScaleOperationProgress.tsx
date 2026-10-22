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
import {toast} from 'sonner';
import {
  AlertTriangle,
  CheckCircle2,
  Circle,
  Loader2,
  MinusCircle,
  RotateCcw,
  XCircle,
} from 'lucide-react';

import services from '@/lib/services';
import {Button} from '@/components/ui/button';
import type {
  ScaleOperationInfo,
  ScaleStepState,
  ScaleStepStatus,
} from '@/lib/services/cluster';

/** 轮询间隔（毫秒）/ Poll interval in milliseconds */
const POLL_INTERVAL_MS = 2000;

export interface ScaleOperationProgressProps {
  clusterId: number;
  operationId: number;
  /** 提交接口返回的首个快照，避免首帧空白 / First snapshot from submit, avoids an empty first frame */
  initial?: ScaleOperationInfo | null;
  /** 操作进入终态时回调（含重试后的终态）/ Called whenever the operation reaches a terminal state */
  onFinished?: (operation: ScaleOperationInfo) => void;
}

/**
 * 步骤状态图标 / Step status icon
 */
function StepIcon({status}: {status: ScaleStepStatus}) {
  switch (status) {
    case 'succeeded':
      return <CheckCircle2 className='h-4 w-4 shrink-0 text-green-600' />;
    case 'running':
      return <Loader2 className='h-4 w-4 shrink-0 animate-spin text-primary' />;
    case 'partial':
      return <AlertTriangle className='h-4 w-4 shrink-0 text-amber-500' />;
    case 'failed':
      return <XCircle className='h-4 w-4 shrink-0 text-red-600' />;
    case 'skipped':
      return <MinusCircle className='h-4 w-4 shrink-0 text-muted-foreground' />;
    default:
      return <Circle className='h-4 w-4 shrink-0 text-muted-foreground/60' />;
  }
}

/**
 * 伸缩操作进度：轮询操作详情直至终态，失败时提供「从失败步骤重试」。
 * 步骤列表在首次轮询前用计划步骤占位，保证高度稳定（零塌陷）。
 * Scaling operation progress: polls the operation until it is terminal and offers "retry from failed step"
 * on failure. Before the first poll the planned steps are used as placeholders to keep the height stable
 * (zero collapse).
 */
export function ScaleOperationProgress({
  clusterId,
  operationId,
  initial,
  onFinished,
}: ScaleOperationProgressProps) {
  const t = useTranslations('cluster.scale');
  const [operation, setOperation] = useState<ScaleOperationInfo | null>(
    initial ?? null,
  );
  const [retrying, setRetrying] = useState(false);
  const finishedRef = useRef<string>('');
  const onFinishedRef = useRef(onFinished);
  useEffect(() => {
    onFinishedRef.current = onFinished;
  }, [onFinished]);

  const load = useCallback(async () => {
    try {
      const next = await services.clusterScale.getOperation(
        clusterId,
        operationId,
      );
      setOperation(next);
      return next;
    } catch {
      // 轮询失败保持上一帧，下次重试。/ Keep the last frame on a failed poll and try again next tick.
      return null;
    }
  }, [clusterId, operationId]);

  useEffect(() => {
    let cancelled = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const tick = async () => {
      const next = await load();
      if (cancelled) {
        return;
      }
      if (!next || next.status === 'running') {
        timer = setTimeout(tick, POLL_INTERVAL_MS);
      }
    };
    void tick();
    return () => {
      cancelled = true;
      if (timer) {
        clearTimeout(timer);
      }
    };
    // retrying 变化后重新开始轮询 / Restart polling after a retry
  }, [load, retrying]);

  // 终态只回调一次（按 状态+更新时间 去重）。/ Report each terminal state once (deduplicated by status + update time).
  useEffect(() => {
    if (!operation || operation.status === 'running') {
      return;
    }
    const key = `${operation.status}-${operation.updated_at}`;
    if (finishedRef.current === key) {
      return;
    }
    finishedRef.current = key;
    onFinishedRef.current?.(operation);
  }, [operation]);

  const handleRetry = async () => {
    setRetrying(true);
    try {
      const result = await services.clusterScale.retryOperation(
        clusterId,
        operationId,
      );
      if (result.operation) {
        setOperation(result.operation);
      }
      toast.success(t('retrySubmitted'));
    } catch (error) {
      toast.error(error instanceof Error ? error.message : String(error));
    } finally {
      setRetrying(false);
    }
  };

  const plannedSteps = operation?.plan?.steps ?? [];
  const steps: ScaleStepState[] =
    operation && operation.steps.length > 0
      ? operation.steps
      : plannedSteps.map((name) => ({name, status: 'pending', attempts: 0}));

  const status = operation?.status ?? 'running';

  return (
    <div className='space-y-3'>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <h4 className='text-sm font-semibold'>{t('progressTitle')}</h4>
        {status === 'failed' && operation?.retryable && (
          <Button
            size='sm'
            variant='outline'
            onClick={handleRetry}
            disabled={retrying}
          >
            {retrying ? (
              <Loader2 className='mr-1.5 h-3.5 w-3.5 animate-spin' />
            ) : (
              <RotateCcw className='mr-1.5 h-3.5 w-3.5' />
            )}
            {t('retry')}
          </Button>
        )}
      </div>

      <ol className='space-y-1.5'>
        {steps.map((step) => (
          <li
            key={step.name}
            className='flex items-start gap-2 rounded-md px-2 py-1.5 text-sm odd:bg-muted/30'
          >
            <StepIcon status={step.status} />
            <div className='min-w-0 flex-1'>
              <div className='flex flex-wrap items-center gap-x-2'>
                <span className='font-medium'>{t(`steps.${step.name}`)}</span>
                <span className='text-xs text-muted-foreground'>
                  {t(`stepStatus.${step.status}`)}
                  {step.attempts > 1 &&
                    ` · ${t('attempts', {count: step.attempts})}`}
                </span>
              </div>
              {step.error ? (
                <p className='break-words text-xs text-red-600'>{step.error}</p>
              ) : (
                step.message && (
                  <p className='break-words text-xs text-muted-foreground'>
                    {step.message}
                  </p>
                )
              )}
            </div>
          </li>
        ))}
      </ol>

      {status === 'running' && (
        <p className='text-xs text-muted-foreground'>{t('operationRunning')}</p>
      )}
      {status === 'succeeded' && (
        <p className='text-sm font-medium text-green-700 dark:text-green-400'>
          {operation && operation.warnings.length > 0
            ? t('operationSucceededWithWarnings', {
                count: operation.warnings.length,
              })
            : t('operationSucceeded')}
        </p>
      )}
      {status === 'succeeded' && operation && operation.warnings.length > 0 && (
        <ul className='list-disc space-y-0.5 pl-5 text-xs text-amber-700 dark:text-amber-400'>
          {operation.warnings.map((warning, index) => (
            <li key={index} className='break-words'>
              {warning}
            </li>
          ))}
        </ul>
      )}
      {status === 'failed' && (
        <p className='break-words text-sm font-medium text-red-600'>
          {t('operationFailed', {error: operation?.error || '-'})}
        </p>
      )}
    </div>
  );
}
