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

import {useState} from 'react';
import {useTranslations} from 'next-intl';
import {Input} from '@/components/arc/input/input';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {CopyButton} from '@/components/arc/copy-button/copy-button';
import {type K8sAuthMode} from '@/lib/services/host/types';
import {
  DEFAULT_STX_MANAGER_SA,
  renderFullK8sRbacYaml,
  renderK8sExportKubeconfigCommand,
  renderK8sGenerateKubeconfigCommand,
  renderK8sTokenCommand,
  renderMinimalK8sRbacYaml,
} from '@/lib/k8s-rbac-templates';

type TemplateTier = 'minimal' | 'full';

interface K8sRBACTemplateSectionProps {
  namespace: string;
  runnerServiceAccount?: string;
  authMode?: K8sAuthMode;
  defaultOpen?: boolean;
}

// K8sRBACTemplateSection 渐进式披露 Kubernetes RBAC 授权模板（单命名空间最小权限 / 跨命名空间全面权限），
// 支持自定义控制面 ServiceAccount 并按当前认证模式（Kubeconfig / Token）自适应展示凭据获取命令。
// K8sRBACTemplateSection progressively discloses parameterized Kubernetes RBAC YAML templates
// (minimal single-namespace vs. full multi-namespace) with editable control-plane SA name and
// auth-mode-aware Kubeconfig/Token acquisition commands.
export function K8sRBACTemplateSection({
  namespace,
  runnerServiceAccount,
  authMode = 'token',
  defaultOpen = false,
}: K8sRBACTemplateSectionProps) {
  const t = useTranslations();
  const [open, setOpen] = useState(defaultOpen);
  const [tier, setTier] = useState<TemplateTier>('minimal');
  const [managerSA, setManagerSA] = useState(DEFAULT_STX_MANAGER_SA);

  const effectiveManagerSA = managerSA.trim() || DEFAULT_STX_MANAGER_SA;
  const yaml =
    tier === 'minimal'
      ? renderMinimalK8sRbacYaml(namespace, runnerServiceAccount, effectiveManagerSA)
      : renderFullK8sRbacYaml(namespace, effectiveManagerSA);
  const tokenCommand = renderK8sTokenCommand(namespace, effectiveManagerSA);
  const kubeconfigCommand = renderK8sGenerateKubeconfigCommand(
    namespace,
    effectiveManagerSA,
  );
  const exportKubeconfigCommand = renderK8sExportKubeconfigCommand();

  return (
    <div className='border-t-[0.5px] border-border/80 pt-3'>
      <button
        type='button'
        className='text-xs font-medium text-muted-foreground hover:text-foreground'
        onClick={() => setOpen((prev) => !prev)}
        aria-expanded={open}
        data-testid='k8s-rbac-template-toggle'
      >
        {open ? '▾' : '▸'} {t('host.k8sRbacTemplateTitle')}
      </button>

      {open ? (
        <div className='mt-3 space-y-3' data-testid='k8s-rbac-template-panel'>
          <div className='flex flex-wrap items-end justify-between gap-3'>
            <SegmentedControl
              label={t('host.k8sRbacTemplateTitle')}
              value={tier}
              onValueChange={(value) => setTier(value as TemplateTier)}
              options={[
                {value: 'minimal', label: t('host.k8sRbacTemplateMinimal')},
                {value: 'full', label: t('host.k8sRbacTemplateFull')},
              ]}
            />
            <div className='min-w-[180px] flex-1'>
              <Input
                label={t('host.k8sRbacTemplateManagerSa')}
                value={managerSA}
                onChange={(e) => setManagerSA(e.target.value)}
                placeholder={DEFAULT_STX_MANAGER_SA}
                className='font-mono text-xs'
                data-testid='k8s-rbac-template-manager-sa'
              />
            </div>
          </div>

          <p className='text-xs text-muted-foreground'>
            {tier === 'minimal'
              ? t('host.k8sRbacTemplateMinimalDesc')
              : t('host.k8sRbacTemplateFullDesc')}
          </p>

          <div className='space-y-1.5'>
            <div className='flex items-center justify-between text-xs text-muted-foreground'>
              <span>{t('host.k8sRbacTemplateYamlLabel')}</span>
              <CopyButton
                value={yaml}
                label={t('common.copy')}
                variant='plain'
                className='h-7 px-2 text-xs'
              />
            </div>
            <pre
              data-testid='k8s-rbac-template-yaml'
              className='m-0 max-h-56 overflow-auto rounded-[var(--radius-control)] bg-muted/50 px-3 py-2.5 font-mono text-[11px] leading-relaxed text-foreground dark:bg-muted/30'
            >
              {yaml}
            </pre>
          </div>

          {authMode === 'kubeconfig' ? (
            <div className='space-y-3'>
              <div className='space-y-1.5'>
                <div className='flex items-center justify-between text-xs text-muted-foreground'>
                  <span>
                    {t('host.k8sRbacTemplateKubeconfigLabel', {
                      sa: effectiveManagerSA,
                    })}
                  </span>
                  <CopyButton
                    value={kubeconfigCommand}
                    label={t('common.copy')}
                    variant='plain'
                    className='h-7 px-2 text-xs'
                  />
                </div>
                <pre
                  data-testid='k8s-rbac-template-kubeconfig-cmd'
                  className='m-0 max-h-44 overflow-auto rounded-[var(--radius-control)] bg-muted/50 px-3 py-2 font-mono text-[11px] leading-relaxed text-foreground dark:bg-muted/30'
                >
                  {kubeconfigCommand}
                </pre>
                <p className='text-[11px] text-muted-foreground'>
                  {t('host.k8sRbacTemplateKubeconfigHint')}
                </p>
              </div>

              <div className='space-y-1.5 border-t-[0.5px] border-border/70 pt-2.5'>
                <div className='flex items-center justify-between text-xs text-muted-foreground'>
                  <span>{t('host.k8sRbacTemplateExportLabel')}</span>
                  <CopyButton
                    value={exportKubeconfigCommand}
                    label={t('common.copy')}
                    variant='plain'
                    className='h-7 px-2 text-xs'
                  />
                </div>
                <pre
                  data-testid='k8s-rbac-template-export-cmd'
                  className='m-0 overflow-x-auto rounded-[var(--radius-control)] bg-muted/50 px-3 py-2 font-mono text-[11px] text-foreground dark:bg-muted/30'
                >
                  {exportKubeconfigCommand}
                </pre>
                <p className='text-[11px] text-muted-foreground'>
                  {t('host.k8sRbacTemplateExportHint')}
                </p>
              </div>
            </div>
          ) : (
            <div className='space-y-1.5'>
              <div className='flex items-center justify-between text-xs text-muted-foreground'>
                <span>
                  {t('host.k8sRbacTemplateTokenLabel', {
                    sa: effectiveManagerSA,
                  })}
                </span>
                <CopyButton
                  value={tokenCommand}
                  label={t('common.copy')}
                  variant='plain'
                  className='h-7 px-2 text-xs'
                />
              </div>
              <pre
                data-testid='k8s-rbac-template-token'
                className='m-0 overflow-x-auto rounded-[var(--radius-control)] bg-muted/50 px-3 py-2 font-mono text-[11px] text-foreground dark:bg-muted/30'
              >
                {tokenCommand}
              </pre>
              <p className='text-[11px] text-muted-foreground'>
                {t('host.k8sRbacTemplateTokenHint')}
              </p>
            </div>
          )}
        </div>
      ) : null}
    </div>
  );
}
