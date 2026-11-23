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

/**
 * 集群启停重启操作组：复用 useClusterLifecycle 与 Arc Button，消除重复弹窗代码。
 * Cluster lifecycle actions group reusing useClusterLifecycle and Arc Button.
 */
import {useTranslations} from 'next-intl';
import {Play, RotateCcw, Square} from 'lucide-react';
import {Button} from '@/components/arc/button/button';
import {OperationType, type ClusterInfo} from '@/lib/services/cluster/types';
import {
  getClusterActionFlags,
  useClusterLifecycle,
} from './useClusterLifecycle';

interface ClusterActionsProps {
  cluster: ClusterInfo;
  onOperationComplete: (operation: OperationType) => void;
  variant?: 'default' | 'compact';
}

export function ClusterActions({
  cluster,
  onOperationComplete,
  variant = 'default',
}: ClusterActionsProps) {
  const t = useTranslations();
  const {canStart, canStop, canRestart} = getClusterActionFlags(cluster);
  const {operatingClusterId, operatingAction, requestLifecycle, dialogs} =
    useClusterLifecycle({
      onComplete: (_cluster, action) => {
        if (
          action === OperationType.START ||
          action === OperationType.STOP ||
          action === OperationType.RESTART
        ) {
          onOperationComplete(action);
        }
      },
    });

  const busy = operatingClusterId === cluster.id;

  if (variant === 'compact') {
    return (
      <>
        <div className='flex items-center gap-1'>
          <Button
            type='button'
            variant='ghost'
            size='sm'
            iconOnly
            onClick={() => requestLifecycle(cluster, OperationType.START)}
            disabled={!canStart || busy}
            loading={busy && operatingAction === OperationType.START}
            aria-label={t('cluster.start')}
            title={t('cluster.start')}
          >
            <Play className='size-3.5' aria-hidden />
          </Button>
          <Button
            type='button'
            variant='ghost'
            size='sm'
            iconOnly
            onClick={() => requestLifecycle(cluster, OperationType.STOP)}
            disabled={!canStop || busy}
            loading={busy && operatingAction === OperationType.STOP}
            aria-label={t('cluster.stop')}
            title={t('cluster.stop')}
          >
            <Square className='size-3.5' aria-hidden />
          </Button>
          <Button
            type='button'
            variant='ghost'
            size='sm'
            iconOnly
            onClick={() => requestLifecycle(cluster, OperationType.RESTART)}
            disabled={!canRestart || busy}
            loading={busy && operatingAction === OperationType.RESTART}
            aria-label={t('cluster.restart')}
            title={t('cluster.restart')}
          >
            <RotateCcw className='size-3.5' aria-hidden />
          </Button>
        </div>
        {dialogs}
      </>
    );
  }

  return (
    <>
      <div className='flex items-center gap-1.5'>
        <Button
          type='button'
          variant='secondary'
          size='sm'
          onClick={() => requestLifecycle(cluster, OperationType.START)}
          disabled={!canStart || busy}
          loading={busy && operatingAction === OperationType.START}
        >
          <Play className='size-3.5' aria-hidden />
          {t('cluster.start')}
        </Button>
        <Button
          type='button'
          variant='secondary'
          size='sm'
          onClick={() => requestLifecycle(cluster, OperationType.STOP)}
          disabled={!canStop || busy}
          loading={busy && operatingAction === OperationType.STOP}
        >
          <Square className='size-3.5' aria-hidden />
          {t('cluster.stop')}
        </Button>
        <Button
          type='button'
          variant='secondary'
          size='sm'
          onClick={() => requestLifecycle(cluster, OperationType.RESTART)}
          disabled={!canRestart || busy}
          loading={busy && operatingAction === OperationType.RESTART}
        >
          <RotateCcw className='size-3.5' aria-hidden />
          {t('cluster.restart')}
        </Button>
      </div>
      {dialogs}
    </>
  );
}
