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
 * 集群生命周期与删除操作共享 Hook：收敛原 ClusterCard / ClusterActions 重复的
 * 启停重启请求、自动重启托管检测与具名二次确认弹窗。
 * Shared cluster lifecycle and delete hook: consolidates duplicated start/stop/restart,
 * auto-restart detection, and named AlertDialog confirmations across views.
 */
import {useCallback, useState, type ReactNode} from 'react';
import {useTranslations} from 'next-intl';
import {Checkbox} from '@/components/arc/checkbox/checkbox';
import {ConfirmDialog} from '@/components/common/layout';
import {toast} from '@/lib/toast';
import services from '@/lib/services';
import {
  ClusterStatus,
  OperationType,
  type ClusterInfo,
} from '@/lib/services/cluster/types';

export interface ClusterActionFlags {
  canStart: boolean;
  canStop: boolean;
  canRestart: boolean;
  canDelete: boolean;
}

export function getClusterActionFlags(
  cluster: Pick<ClusterInfo, 'status'>,
): ClusterActionFlags {
  return {
    canStart:
      cluster.status === ClusterStatus.CREATED ||
      cluster.status === ClusterStatus.STOPPED,
    canStop: cluster.status === ClusterStatus.RUNNING,
    canRestart: cluster.status === ClusterStatus.RUNNING,
    canDelete:
      cluster.status !== ClusterStatus.RUNNING &&
      cluster.status !== ClusterStatus.DEPLOYING,
  };
}

interface PendingLifecycle {
  cluster: ClusterInfo;
  operation: OperationType;
}

export interface UseClusterLifecycleOptions {
  onComplete?: (cluster: ClusterInfo, action: OperationType | 'delete') => void;
}

export interface UseClusterLifecycleResult {
  operatingClusterId: number | null;
  operatingAction: OperationType | 'delete' | null;
  requestLifecycle: (cluster: ClusterInfo, operation: OperationType) => void;
  requestDelete: (cluster: ClusterInfo) => void;
  dialogs: ReactNode;
}

export function useClusterLifecycle(
  options: UseClusterLifecycleOptions = {},
): UseClusterLifecycleResult {
  const {onComplete} = options;
  const t = useTranslations();

  const [pendingLifecycle, setPendingLifecycle] =
    useState<PendingLifecycle | null>(null);
  const [pendingDelete, setPendingDelete] = useState<ClusterInfo | null>(null);
  const [forceDelete, setForceDelete] = useState(false);
  const [operatingClusterId, setOperatingClusterId] = useState<number | null>(
    null,
  );
  const [operatingAction, setOperatingAction] = useState<
    OperationType | 'delete' | null
  >(null);

  const executeLifecycle = useCallback(
    async (cluster: ClusterInfo, operation: OperationType) => {
      setOperatingClusterId(cluster.id);
      setOperatingAction(operation);
      setPendingLifecycle(null);

      try {
        const result =
          operation === OperationType.START
            ? await services.cluster.startClusterSafe(cluster.id)
            : operation === OperationType.STOP
              ? await services.cluster.stopClusterSafe(cluster.id)
              : await services.cluster.restartClusterSafe(cluster.id);

        if (result.success) {
          const isAutoRestart =
            result.data?.message?.includes('auto-restart') ||
            result.data?.message?.includes('auto-start') ||
            result.data?.node_results?.some(
              (nr) =>
                nr.message?.includes('auto-restart') ||
                nr.message?.includes('auto-start'),
            );
          const successMessage = {
            [OperationType.START]: isAutoRestart
              ? t('cluster.startSuccessAutoRestart')
              : t('cluster.startSuccess'),
            [OperationType.STOP]: t('cluster.stopSuccess'),
            [OperationType.RESTART]: isAutoRestart
              ? t('cluster.restartSuccessAutoRestart')
              : t('cluster.restartSuccess'),
          }[operation];
          toast.success(successMessage);
          onComplete?.(cluster, operation);
        } else {
          const fallbackError = {
            [OperationType.START]: t('cluster.startError'),
            [OperationType.STOP]: t('cluster.stopError'),
            [OperationType.RESTART]: t('cluster.restartError'),
          }[operation];
          toast.error(result.error || fallbackError);
          // 重启 HTTP 200 但 data.success=false（如 Operation completed with errors）时，
          // 进程事件/新 PID 仍可能异步落库；仍要启动停留页轮询，否则 UI 冻到重新进入（#186 AC2）。
          // Restart may return HTTP 200 with data.success=false while events/new PID still land async;
          // still start stay-on-page poll or the UI freezes until remount (#186 AC2).
          if (operation === OperationType.RESTART) {
            onComplete?.(cluster, operation);
          }
        }
      } finally {
        setOperatingClusterId(null);
        setOperatingAction(null);
      }
    },
    [onComplete, t],
  );

  const executeDelete = useCallback(
    async (cluster: ClusterInfo, force: boolean) => {
      setOperatingClusterId(cluster.id);
      setOperatingAction('delete');
      setPendingDelete(null);
      setForceDelete(false);

      try {
        const result = await services.cluster.deleteClusterSafe(cluster.id, {
          forceDelete: force,
        });
        if (result.success) {
          toast.success(t('cluster.deleteSuccess'));
          onComplete?.(cluster, 'delete');
        } else {
          toast.error(result.error || t('cluster.deleteError'));
        }
      } finally {
        setOperatingClusterId(null);
        setOperatingAction(null);
      }
    },
    [onComplete, t],
  );

  const requestLifecycle = useCallback(
    (cluster: ClusterInfo, operation: OperationType) => {
      setPendingLifecycle({cluster, operation});
    },
    [],
  );

  const requestDelete = useCallback((cluster: ClusterInfo) => {
    setForceDelete(false);
    setPendingDelete(cluster);
  }, []);

  const lifecycleTitle = pendingLifecycle
    ? {
        [OperationType.START]: t('cluster.startConfirmTitle'),
        [OperationType.STOP]: t('cluster.stopConfirmTitle'),
        [OperationType.RESTART]: t('cluster.restartConfirmTitle'),
      }[pendingLifecycle.operation]
    : '';

  const lifecycleMessage = pendingLifecycle
    ? {
        [OperationType.START]: t('cluster.startConfirmMessage', {
          name: pendingLifecycle.cluster.name,
        }),
        [OperationType.STOP]: t('cluster.stopConfirmMessage', {
          name: pendingLifecycle.cluster.name,
        }),
        [OperationType.RESTART]: t('cluster.restartConfirmMessage', {
          name: pendingLifecycle.cluster.name,
        }),
      }[pendingLifecycle.operation]
    : '';

  const lifecycleActionLabel = pendingLifecycle
    ? {
        [OperationType.START]: t('cluster.list.confirmStartAction'),
        [OperationType.STOP]: t('cluster.list.confirmStopAction'),
        [OperationType.RESTART]: t('cluster.list.confirmRestartAction'),
      }[pendingLifecycle.operation]
    : '';

  const isDestructiveLifecycle =
    pendingLifecycle?.operation === OperationType.STOP;

  const dialogs = (
    <>
      <ConfirmDialog
        open={pendingLifecycle !== null}
        onOpenChange={(open) => {
          if (!open) {
            setPendingLifecycle(null);
          }
        }}
        title={lifecycleTitle}
        description={lifecycleMessage}
        cancelLabel={t('common.cancel')}
        confirmLabel={lifecycleActionLabel}
        confirmVariant={isDestructiveLifecycle ? 'danger' : 'primary'}
        onConfirm={() => {
          if (pendingLifecycle) {
            void executeLifecycle(
              pendingLifecycle.cluster,
              pendingLifecycle.operation,
            );
          }
        }}
      />

      <ConfirmDialog
        open={pendingDelete !== null}
        onOpenChange={(open) => {
          if (!open) {
            setPendingDelete(null);
            setForceDelete(false);
          }
        }}
        title={t('cluster.deleteCluster')}
        description={
          pendingDelete
            ? t('cluster.deleteConfirm', {name: pendingDelete.name})
            : ''
        }
        cancelLabel={t('common.cancel')}
        confirmLabel={t('cluster.list.confirmDeleteAction')}
        confirmVariant='danger'
        onConfirm={() => {
          if (pendingDelete) {
            void executeDelete(pendingDelete, forceDelete);
          }
        }}
      >
        <p className='text-sm text-muted-foreground'>
          {t('cluster.deleteConfirmWarning')}
        </p>
        <div className='pt-1'>
          <Checkbox
            checked={forceDelete}
            onCheckedChange={(v) => setForceDelete(v === true)}
            label={t('cluster.forceDeleteOption')}
          />
        </div>
      </ConfirmDialog>
    </>
  );

  return {
    operatingClusterId,
    operatingAction,
    requestLifecycle,
    requestDelete,
    dialogs,
  };
}
