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
  cluster: Pick<ClusterInfo, 'status' | 'infra_type'>,
): ClusterActionFlags {
  // K8s 集群的"删除"即资源生命周期本身（解绑/卸载工作负载），后端允许运行态删除，前端必须对齐。
  // For K8s clusters deletion IS the resource lifecycle (unbind/uninstall workloads);
  // the backend permits deleting running clusters and the frontend must match.
  const isK8s = Boolean(cluster.infra_type && cluster.infra_type !== 'bare_metal');
  return {
    canStart:
      cluster.status === ClusterStatus.CREATED ||
      cluster.status === ClusterStatus.STOPPED,
    canStop: cluster.status === ClusterStatus.RUNNING,
    canRestart: cluster.status === ClusterStatus.RUNNING,
    canDelete: isK8s
      ? cluster.status !== ClusterStatus.DEPLOYING
      : cluster.status !== ClusterStatus.RUNNING &&
        cluster.status !== ClusterStatus.DEPLOYING,
  };
}

interface PendingLifecycle {
  cluster: ClusterInfo;
  operation: OperationType;
}

export interface UseClusterLifecycleOptions {
  onComplete?: (cluster: ClusterInfo, action: OperationType | 'delete') => void;
  /**
   * 重启确认后、POST 返回前触发：详情页用它立刻开轮询，勿与 onComplete 混用以免重采 baseline。
   * Fires after restart confirm, before POST returns: detail pages start poll here;
   * do not also refresh baseline from onComplete.
   */
  onRestartConfirmed?: (cluster: ClusterInfo) => void;
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
  const {onComplete, onRestartConfirmed} = options;
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

      // 重启：POST 可能阻塞数十秒后 500/超时，但事件与新 PID 已异步到达。
      // 确认后立刻 onRestartConfirmed 开轮询；结束后仍调 onComplete（含失败）刷新列表。
      // Restart: POST may block then 500/timeout while events/new PID land async.
      // Fire onRestartConfirmed immediately for poll; always call onComplete after finish (incl. failure).
      if (operation === OperationType.RESTART) {
        onRestartConfirmed?.(cluster);
        toast.info(t('cluster.restartAccepted'));
      } else {
        toast.info(
          operation === OperationType.START
            ? t('cluster.startAccepted')
            : t('cluster.stopAccepted'),
        );
      }

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
          if (operation !== OperationType.RESTART) {
            const successMessage = {
              [OperationType.START]: isAutoRestart
                ? t('cluster.startSuccessAutoRestart')
                : t('cluster.startSuccess'),
              [OperationType.STOP]: t('cluster.stopSuccess'),
              [OperationType.RESTART]: t('cluster.restartSuccess'),
            }[operation];
            toast.success(successMessage);
          } else if (isAutoRestart) {
            // 受理已 toast；自动重启托管仅补充说明，成功仍等轮询 onSatisfied。
            // Accepted already toasted; auto-restart only adds a note — success waits for poll onSatisfied.
            toast.info(t('cluster.restartSuccessAutoRestart'));
          }
          onComplete?.(cluster, operation);
        } else {
          const fallbackError = {
            [OperationType.START]: t('cluster.startError'),
            [OperationType.STOP]: t('cluster.stopError'),
            [OperationType.RESTART]: t('cluster.restartError'),
          }[operation];
          toast.error(result.error || fallbackError);
          // 失败也通知调用方：列表页需刷新；详情页勿在 onComplete 里对 RESTART 重采 baseline。
          // Notify on failure too: list refreshes; detail must not recapture RESTART baseline in onComplete.
          onComplete?.(cluster, operation);
        }
      } finally {
        setOperatingClusterId(null);
        setOperatingAction(null);
      }
    },
    [onComplete, onRestartConfirmed, t],
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

  // adopted=解绑并仅清理 STX Java Proxy；stx_deployed=卸载控制面部署的 K8s 资源；非 K8s 走原文案。
  // adopted=unbind plus STX Java Proxy cleanup only; stx_deployed=uninstall the control-plane
  // deployed K8s resources; non-K8s keeps the original copy.
  const deleteDialogK8sKind = (() => {
    if (!pendingDelete?.infra_type || pendingDelete.infra_type === 'bare_metal') {
      return null;
    }
    const cfg = (pendingDelete.config as Record<string, unknown> | undefined)?.['kubernetes'] as
      | Record<string, unknown>
      | undefined;
    const source = cfg?.['management_source'];
    return source === 'stx_deployed' ? 'stx_deployed' : 'adopted';
  })();

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
          {deleteDialogK8sKind === 'adopted'
            ? t('cluster.k8s.deleteAdoptedDesc')
            : deleteDialogK8sKind === 'stx_deployed'
              ? t('cluster.k8s.deleteDeployedDesc')
              : t('cluster.deleteConfirmWarning')}
        </p>
        {deleteDialogK8sKind === null ? (
          <div className='pt-1'>
            <Checkbox
              checked={forceDelete}
              onCheckedChange={(v) => setForceDelete(v === true)}
              label={t('cluster.forceDeleteOption')}
            />
          </div>
        ) : null}
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
