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
 * 独立的 SeaTunnel 引擎运行账号（seatunnel-runner）RBAC 模板引导区：
 * 与 STX 控制面账号彻底拆离，支持两种复用场景：
 * 1) 集群部署向导（onRefreshAccounts）：应用后刷新当前命名空间 SA 列表；
 * 2) 主机接入完成页 / 主机详情页（hostId + onBoundDefaultSA）：检测目标命名空间已存在该 SA 后一键绑定为本主机默认，后续部署自动复用。
 * Standalone SeaTunnel engine runner ServiceAccount RBAC template section:
 * strictly decoupled from the STX control-plane SA, supporting both cluster-wizard account refresh
 * and host-level verify-and-bind default workflows for automatic reuse across future deployments.
 */

import {useEffect, useState} from 'react';
import {useTranslations} from 'next-intl';
import {AlertTriangle, Check, RefreshCw} from 'lucide-react';
import {Button} from '@/components/arc/button/button';
import {CopyButton} from '@/components/arc/copy-button/copy-button';
import {Input} from '@/components/arc/input/input';
import services from '@/lib/services';
import type {HostInfo} from '@/lib/services/host/types';
import {toast} from '@/lib/toast';
import {
  DEFAULT_SEATUNNEL_RUNNER_SA,
  renderSeaTunnelRunnerK8sRbacYaml,
} from '@/lib/k8s-rbac-templates';

interface K8sEngineSATemplateSectionProps {
  namespace: string;
  initialRunnerSA?: string;
  defaultOpen?: boolean;
  collapsible?: boolean;
  onRefreshAccounts?: (preferredSA: string) => void | Promise<void>;
  refreshing?: boolean;
  hostId?: number;
  onBoundDefaultSA?: (updatedHost: HostInfo) => void;
}

interface BindFeedback {
  tone: 'success' | 'warning' | 'error';
  text: string;
}

export function K8sEngineSATemplateSection({
  namespace,
  initialRunnerSA,
  defaultOpen = false,
  collapsible = true,
  onRefreshAccounts,
  refreshing = false,
  hostId,
  onBoundDefaultSA,
}: K8sEngineSATemplateSectionProps) {
  const t = useTranslations();
  const [open, setOpen] = useState(defaultOpen);
  const [runnerSA, setRunnerSA] = useState(
    initialRunnerSA?.trim() || DEFAULT_SEATUNNEL_RUNNER_SA,
  );
  const [binding, setBinding] = useState(false);
  const [feedback, setFeedback] = useState<BindFeedback | null>(null);

  useEffect(() => {
    if (initialRunnerSA?.trim()) {
      setRunnerSA(initialRunnerSA.trim());
    }
  }, [initialRunnerSA]);

  const effectiveNs = namespace.trim() || 'seatunnel';
  const effectiveRunnerSA = runnerSA.trim() || DEFAULT_SEATUNNEL_RUNNER_SA;
  const yaml = renderSeaTunnelRunnerK8sRbacYaml(effectiveNs, effectiveRunnerSA);
  const isExpanded = !collapsible || open;

  const handleVerifyAndBindDefault = async () => {
    if (!hostId) {
      return;
    }
    setBinding(true);
    setFeedback(null);
    try {
      const accounts = await services.host.listK8sNamespaceObjects(
        hostId,
        effectiveNs,
        'serviceaccounts',
      );
      if (!accounts.includes(effectiveRunnerSA)) {
        setFeedback({
          tone: 'warning',
          text: t('host.k8sEngineSaBindNotFound', {
            ns: effectiveNs,
            sa: effectiveRunnerSA,
          }),
        });
        return;
      }
      const updated = await services.host.updateHostSafe(hostId, {
        k8s_default_service_account: effectiveRunnerSA,
      });
      if (!updated.success || !updated.data) {
        setFeedback({
          tone: 'error',
          text: t('host.k8sEngineSaBindFailed', {
            error: updated.error || '',
          }),
        });
        return;
      }
      const msg = t('host.k8sEngineSaBindSuccess', {sa: effectiveRunnerSA});
      setFeedback({tone: 'success', text: msg});
      toast.success(msg);
      onBoundDefaultSA?.(updated.data);
    } catch (err) {
      setFeedback({
        tone: 'error',
        text: t('host.k8sEngineSaBindFailed', {
          error: err instanceof Error ? err.message : String(err),
        }),
      });
    } finally {
      setBinding(false);
    }
  };

  return (
    <div className={collapsible ? 'space-y-2 pt-1' : 'space-y-2.5'}>
      {collapsible ? (
        <button
          type='button'
          className='text-xs font-medium text-muted-foreground hover:text-foreground'
          onClick={() => setOpen((prev) => !prev)}
          aria-expanded={open}
          data-testid='k8s-engine-sa-template-toggle'
        >
          {open ? '▾' : '▸'} {t('host.k8sEngineSaTemplateTitle')}
        </button>
      ) : null}

      {isExpanded ? (
        <div
          className={
            collapsible
              ? 'space-y-2.5 rounded-[var(--radius-control)] bg-muted/35 p-3 dark:bg-muted/25'
              : 'space-y-2.5'
          }
          data-testid='k8s-engine-sa-template-panel'
        >
          {collapsible ? (
            <p className='text-xs text-muted-foreground'>
              {t('host.k8sEngineSaTemplateDesc')}
            </p>
          ) : null}

          <div className='flex flex-wrap items-end gap-2'>
            <div className='min-w-[200px] flex-1'>
              <Input
                label={t('host.k8sEngineSaNameLabel')}
                value={runnerSA}
                onChange={(e) => {
                  setRunnerSA(e.target.value);
                  setFeedback(null);
                }}
                placeholder={DEFAULT_SEATUNNEL_RUNNER_SA}
                className='font-mono text-xs'
                data-testid='k8s-engine-sa-name-input'
              />
            </div>
            {onRefreshAccounts ? (
              <Button
                type='button'
                size='sm'
                variant='secondary'
                loading={refreshing}
                disabled={refreshing}
                onClick={() => void onRefreshAccounts(effectiveRunnerSA)}
                data-testid='k8s-engine-sa-refresh-btn'
              >
                <RefreshCw className='size-3.5' aria-hidden />
                {t('host.k8sEngineSaRefreshList')}
              </Button>
            ) : null}
            {hostId ? (
              <Button
                type='button'
                size='sm'
                variant={onRefreshAccounts ? 'secondary' : 'primary'}
                loading={binding}
                disabled={binding}
                onClick={() => void handleVerifyAndBindDefault()}
                data-testid='k8s-engine-sa-bind-btn'
              >
                <Check className='size-3.5' aria-hidden />
                {t('host.k8sEngineSaBindDefault')}
              </Button>
            ) : null}
          </div>

          {feedback ? (
            <div
              className={`flex items-start gap-1.5 rounded-md px-2.5 py-1.5 text-xs ${
                feedback.tone === 'success'
                  ? 'bg-emerald-500/10 text-emerald-700 dark:text-emerald-300'
                  : feedback.tone === 'warning'
                    ? 'bg-amber-500/10 text-amber-800 dark:text-amber-300'
                    : 'bg-rose-500/10 text-rose-700 dark:text-rose-300'
              }`}
              data-testid='k8s-engine-sa-bind-feedback'
            >
              {feedback.tone === 'success' ? (
                <Check className='mt-0.5 size-3.5 shrink-0' aria-hidden />
              ) : (
                <AlertTriangle className='mt-0.5 size-3.5 shrink-0' aria-hidden />
              )}
              <span>{feedback.text}</span>
            </div>
          ) : null}

          <div className='space-y-1.5'>
            <div className='flex items-center justify-between gap-2 text-xs text-muted-foreground'>
              <span>{t('host.k8sEngineSaYamlLabel')}</span>
              <CopyButton
                value={yaml}
                label={t('common.copy')}
                variant='plain'
                className='h-7 px-2 text-xs'
              />
            </div>
            <pre
              data-testid='k8s-engine-sa-template-yaml'
              className='m-0 max-h-44 overflow-auto rounded-[var(--radius-control)] bg-muted/50 px-3 py-2 font-mono text-[11px] leading-relaxed text-foreground dark:bg-muted/30'
            >
              {yaml}
            </pre>
          </div>
        </div>
      ) : null}
    </div>
  );
}
