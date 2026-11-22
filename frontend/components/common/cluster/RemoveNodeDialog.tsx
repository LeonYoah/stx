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
 * 移除节点（缩容）弹窗：迁移至 Arc Dialog（.arc-compact）+ 单层画布 0.5px 细线 + Arc Alert/Button。
 * Remove-node (scale-in) dialog: Arc Dialog (.arc-compact) + flat 0.5px hairlines + Arc Alert/Button.
 */
import {useEffect, useState} from 'react';
import {useTranslations} from 'next-intl';
import {RefreshCw} from 'lucide-react';
import {Alert} from '@/components/arc/alert/alert';
import {Button} from '@/components/arc/button/button';
import {Dialog, DialogContent} from '@/components/arc/dialog/dialog';
import {TableLoadingBar} from '@/components/common/layout';
import {Skeleton} from '@/components/ui/skeleton';
import services from '@/lib/services';
import {
  getScaleInProgressDetail,
  type NodeInfo,
  type ScaleOperationInfo,
  type ScalePlan,
} from '@/lib/services/cluster';
import {toast} from '@/lib/toast';
import {ScaleOperationProgress} from './scale/ScaleOperationProgress';
import {ScalePlanView} from './scale/ScalePlanView';

export interface RemoveNodeDialogProps {
  clusterId: number;
  node: NodeInfo | null;
  onOpenChange: (open: boolean) => void;
  onRemoved: () => void;
}

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
  const targetOffline = failedCheck('target_agent_online');
  const sharedProcess = failedCheck('shared_process');
  const forceAvailable = targetOffline || sharedProcess;

  const canSubmit =
    Boolean(plan) &&
    Boolean(plan?.can_submit) &&
    !loading &&
    !submitting &&
    (!plan?.requires_jobs_ack || jobsAck);

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
      <DialogContent
        title={ts('removeTitle', {name: nodeName})}
        description={ts('removeDescription')}
        className='arc-compact w-[min(92vw,760px)]! sm:max-w-none!'
      >
        <div className='space-y-4'>
          <TableLoadingBar loading={loading && Boolean(plan)} />

          <div className='max-h-[60vh] overflow-y-auto'>
            {operationId !== null ? (
              <ScaleOperationProgress
                clusterId={clusterId}
                operationId={operationId}
                initial={initialOperation}
                onFinished={handleFinished}
              />
            ) : loading && !plan ? (
              <div className='space-y-3 py-1'>
                <Skeleton className='h-5 w-44' />
                <Skeleton className='h-20 w-full' />
                <Skeleton className='h-14 w-full' />
              </div>
            ) : (
              <div className='space-y-4'>
                {error ? (
                  <Alert tone='danger' title={`${ts('planFailed')}: ${error}`} />
                ) : null}
                {plan ? (
                  <ScalePlanView
                    plan={plan}
                    jobsAck={jobsAck}
                    onJobsAckChange={setJobsAck}
                  />
                ) : null}
                {forceAvailable ? (
                  <Alert
                    tone='warning'
                    title={
                      sharedProcess
                        ? ts('forceRemoveSharedHint')
                        : ts('forceRemoveHint')
                    }
                  />
                ) : null}
              </div>
            )}
          </div>

          <div className='flex flex-wrap items-center justify-end gap-2 border-t-[0.5px] border-border/80 pt-4'>
            {operationId !== null ? (
              <Button
                type='button'
                variant='secondary'
                size='sm'
                onClick={() => onOpenChange(false)}
              >
                {ts('close')}
              </Button>
            ) : (
              <>
                <Button
                  type='button'
                  variant='secondary'
                  size='sm'
                  onClick={() => onOpenChange(false)}
                >
                  {t('common.cancel')}
                </Button>
                {forceAvailable ? (
                  <Button
                    type='button'
                    variant='danger'
                    size='sm'
                    onClick={() => void handleForceRemove()}
                    loading={forcing}
                  >
                    {forceArmed ? ts('forceRemoveConfirm') : ts('forceRemove')}
                  </Button>
                ) : null}
                <Button
                  type='button'
                  variant='secondary'
                  size='sm'
                  onClick={() => node && void loadPlan(node)}
                  disabled={submitting}
                  loading={loading}
                >
                  <RefreshCw className='size-3.5' aria-hidden />
                  {ts('recheck')}
                </Button>
                <Button
                  type='button'
                  variant='danger'
                  size='sm'
                  onClick={() => void handleSubmit()}
                  disabled={!canSubmit}
                  loading={submitting}
                  title={
                    plan && !plan.can_submit ? ts('cannotSubmit') : undefined
                  }
                >
                  {ts('submitScaleIn')}
                </Button>
              </>
            )}
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
