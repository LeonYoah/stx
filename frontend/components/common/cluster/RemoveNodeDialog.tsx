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

import {useEffect, useState} from 'react';
import {useTranslations} from 'next-intl';
import {toast} from 'sonner';
import {Loader2, RefreshCw} from 'lucide-react';

import services from '@/lib/services';
import {Button} from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {Skeleton} from '@/components/ui/skeleton';
import {
  NodeInfo,
  ScaleOperationInfo,
  ScalePlan,
  getScaleInProgressDetail,
} from '@/lib/services/cluster';
import {ScalePlanView} from './scale/ScalePlanView';
import {ScaleOperationProgress} from './scale/ScaleOperationProgress';

export interface RemoveNodeDialogProps {
  clusterId: number;
  /** 待移除节点；为空时关闭 / Node to remove; the dialog is closed when null */
  node: NodeInfo | null;
  onOpenChange: (open: boolean) => void;
  /** 节点被移除（缩容成功或强制移除记录）后回调 / Called after the node is removed (scale-in or forced record removal) */
  onRemoved: () => void;
}

/**
 * 移除节点（缩容）：展示缩容计划与护栏，运行中作业需强确认；主机离线时提供「仅移除记录」降级入口。
 * Remove-node (scale-in) dialog: shows the scale-in plan and guardrails, requires a strong acknowledgement for
 * running jobs, and offers a "remove record only" fallback when the host is offline.
 */
export function RemoveNodeDialog({
  clusterId,
  node,
  onOpenChange,
  onRemoved,
}: RemoveNodeDialogProps) {
  const t = useTranslations();
  const ts = useTranslations('cluster.scale');
  const open = node !== null;

  const [plan, setPlan] = useState<ScalePlan | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [jobsAck, setJobsAck] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [forceArmed, setForceArmed] = useState(false);
  const [forcing, setForcing] = useState(false);
  const [operationId, setOperationId] = useState<number | null>(null);
  const [initialOperation, setInitialOperation] =
    useState<ScaleOperationInfo | null>(null);

  const loadPlan = async (target: NodeInfo) => {
    setLoading(true);
    setError('');
    try {
      const next = await services.clusterScale.planScaleIn(
        clusterId,
        target.id,
      );
      setPlan(next);
      setJobsAck(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    setPlan(null);
    setError('');
    setJobsAck(false);
    setForceArmed(false);
    setOperationId(null);
    setInitialOperation(null);
    if (node) {
      void loadPlan(node);
    }
    // 仅在目标节点变化时重新加载 / Reload only when the target node changes
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [node?.id]);

  const failedCheck = (name: string) =>
    plan?.checks.some(
      (check) => check.name === name && check.status === 'failed',
    ) ?? false;
  // 主机离线（无法停进程）或进程被其他集群共用（不应停进程）时，提供「仅移除记录」。
  // Offer "remove record only" when the host is offline (cannot stop the process) or the process is shared with
  // another cluster (must not be stopped).
  const targetOffline = failedCheck('target_agent_online');
  const sharedProcess = failedCheck('shared_process');
  const forceAvailable = targetOffline || sharedProcess;

  const canSubmit =
    !!plan &&
    plan.can_submit &&
    !loading &&
    !submitting &&
    (!plan.requires_jobs_ack || jobsAck);

  const handleSubmit = async () => {
    if (!plan || !node) {
      return;
    }
    setSubmitting(true);
    try {
      const result = await services.clusterScale.submitScaleIn(
        clusterId,
        node.id,
        {
          plan_fingerprint: plan.fingerprint,
          jobs_risk_ack: jobsAck,
        },
      );
      setOperationId(result.operation_id);
      setInitialOperation(result.operation);
    } catch (err) {
      const conflict = getScaleInProgressDetail(err);
      if (conflict) {
        toast.error(
          ts('inProgressConflict', {
            id: conflict.operation_id,
            kind: ts(`kind.${conflict.kind}`),
            user: conflict.created_by_name || `#${conflict.created_by}`,
          }),
        );
      } else {
        toast.error(err instanceof Error ? err.message : String(err));
        if ((err as {status?: number}).status === 409) {
          void loadPlan(node);
        }
      }
    } finally {
      setSubmitting(false);
    }
  };

  /**
   * 强制仅删除节点记录（沿用旧接口），需二次点击确认。
   * Force-delete only the node record (legacy endpoint); requires a second click to confirm.
   */
  const handleForceRemove = async () => {
    if (!node) {
      return;
    }
    if (!forceArmed) {
      setForceArmed(true);
      return;
    }
    setForcing(true);
    const result = await services.cluster.removeNodeSafe(clusterId, node.id);
    setForcing(false);
    if (result.success) {
      toast.success(t('cluster.removeNodeSuccess'));
      onRemoved();
      onOpenChange(false);
    } else {
      toast.error(result.error || t('cluster.removeNodeError'));
    }
  };

  const handleFinished = (operation: ScaleOperationInfo) => {
    if (operation.status === 'succeeded') {
      onRemoved();
    }
  };

  const nodeName = node?.host_name || String(node?.id ?? '');

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='sm:max-w-3xl'>
        <DialogHeader>
          <DialogTitle>{ts('removeTitle', {name: nodeName})}</DialogTitle>
          <DialogDescription>{ts('removeDescription')}</DialogDescription>
        </DialogHeader>

        <div className='max-h-[64vh] overflow-y-auto pr-1'>
          {operationId !== null ? (
            <ScaleOperationProgress
              clusterId={clusterId}
              operationId={operationId}
              initial={initialOperation}
              onFinished={handleFinished}
            />
          ) : loading && !plan ? (
            <div className='space-y-3'>
              <p className='flex items-center gap-2 text-sm text-muted-foreground'>
                <Loader2 className='h-4 w-4 animate-spin' />
                {ts('planLoading')}
              </p>
              <Skeleton className='h-20 w-full' />
              <Skeleton className='h-14 w-full' />
            </div>
          ) : (
            <div className='space-y-3'>
              {error && (
                <p className='break-words text-sm text-red-600'>
                  {ts('planFailed')}: {error}
                </p>
              )}
              {plan && (
                <div
                  className={loading ? 'pointer-events-none opacity-60' : ''}
                >
                  <ScalePlanView
                    plan={plan}
                    jobsAck={jobsAck}
                    onJobsAckChange={setJobsAck}
                  />
                </div>
              )}
              {forceAvailable && (
                <p className='rounded-md border border-amber-300 bg-amber-50/60 px-3 py-2 text-xs dark:border-amber-800/60 dark:bg-amber-950/30'>
                  {sharedProcess
                    ? ts('forceRemoveSharedHint')
                    : ts('forceRemoveHint')}
                </p>
              )}
            </div>
          )}
        </div>

        <DialogFooter className='gap-2 sm:gap-0'>
          {operationId !== null ? (
            <Button variant='outline' onClick={() => onOpenChange(false)}>
              {ts('close')}
            </Button>
          ) : (
            <>
              <Button variant='outline' onClick={() => onOpenChange(false)}>
                {t('common.cancel')}
              </Button>
              {forceAvailable && (
                <Button
                  variant='destructive'
                  onClick={handleForceRemove}
                  disabled={forcing}
                >
                  {forcing && <Loader2 className='mr-2 h-4 w-4 animate-spin' />}
                  {forceArmed ? ts('forceRemoveConfirm') : ts('forceRemove')}
                </Button>
              )}
              <Button
                variant='secondary'
                onClick={() => node && void loadPlan(node)}
                disabled={loading || submitting}
              >
                {loading ? (
                  <Loader2 className='mr-2 h-4 w-4 animate-spin' />
                ) : (
                  <RefreshCw className='mr-2 h-4 w-4' />
                )}
                {ts('recheck')}
              </Button>
              <Button
                variant='destructive'
                onClick={handleSubmit}
                disabled={!canSubmit}
                title={
                  plan && !plan.can_submit ? ts('cannotSubmit') : undefined
                }
              >
                {submitting && (
                  <Loader2 className='mr-2 h-4 w-4 animate-spin' />
                )}
                {ts('submitScaleIn')}
              </Button>
            </>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
