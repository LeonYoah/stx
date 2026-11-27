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
 * STX 控制面单命名空间 RBAC 授权模板生成区（v2 结构化版）：
 * 以 3 格结构化规格条替代三段说明文字，仅生成单命名空间控制面 Role + RoleBinding（不含 ClusterRole 与引擎运行账号），
 * 支持按需勾选删除/Secret 扩展权限并随认证方式与命名空间实时联动更新 YAML 与签发命令。
 * STX control-plane single-namespace RBAC template section (v2 structured layout):
 * replaces multi-paragraph prose with a 3-cell spec strip and renders control-plane Role + RoleBinding only
 * (no ClusterRole or engine runner SA), updating YAML and credential commands live with namespace & auth mode.
 */

import {useEffect, useState} from 'react';
import {useTranslations} from 'next-intl';
import {Checkbox} from '@/components/arc/checkbox/checkbox';
import {Input} from '@/components/arc/input/input';
import {CopyButton} from '@/components/arc/copy-button/copy-button';
import {type K8sAuthMode} from '@/lib/services/host/types';
import {
  DEFAULT_STX_MANAGER_SA,
  renderControlPlaneK8sRbacYaml,
  renderK8sGenerateKubeconfigCommand,
  renderK8sTokenCommand,
} from '@/lib/k8s-rbac-templates';

interface K8sRBACTemplateSectionProps {
  namespace: string;
  authMode?: K8sAuthMode;
  defaultOpen?: boolean;
  collapsible?: boolean;
}

const CONTEXT_CHECK_CMD = 'kubectl config current-context';

export function K8sRBACTemplateSection({
  namespace,
  authMode = 'kubeconfig',
  defaultOpen = false,
  collapsible = true,
}: K8sRBACTemplateSectionProps) {
  const t = useTranslations();
  const [open, setOpen] = useState(defaultOpen);
  const [managerSA, setManagerSA] = useState(DEFAULT_STX_MANAGER_SA);
  const [includeDeleteRules, setIncludeDeleteRules] = useState(false);
  const [includeSecretRules, setIncludeSecretRules] = useState(false);

  useEffect(() => {
    setOpen(defaultOpen);
  }, [defaultOpen]);

  const effectiveNs = namespace.trim() || 'seatunnel';
  const effectiveManagerSA = managerSA.trim() || DEFAULT_STX_MANAGER_SA;
  const yaml = renderControlPlaneK8sRbacYaml(effectiveNs, effectiveManagerSA, {
    includeDeleteRules,
    includeSecretRules,
  });
  const tokenCommand = renderK8sTokenCommand(effectiveNs, effectiveManagerSA);
  const kubeconfigCommand = renderK8sGenerateKubeconfigCommand(
    effectiveNs,
    effectiveManagerSA,
  );
  const isExpanded = !collapsible || open;

  return (
    <div className={collapsible ? 'border-t-[0.5px] border-border/80 pt-3.5' : ''}>
      {collapsible ? (
        <button
          type='button'
          className='text-xs font-medium text-muted-foreground hover:text-foreground'
          onClick={() => setOpen((prev) => !prev)}
          aria-expanded={open}
          data-testid='k8s-rbac-template-toggle'
        >
          {open ? '▾' : '▸'} {t('host.k8sRbacTemplateTitle')}
        </button>
      ) : null}

      {isExpanded ? (
        <div
          className={collapsible ? 'mt-3 space-y-3' : 'space-y-3'}
          data-testid='k8s-rbac-template-panel'
        >
          {/* 3 项结构化规格条：去长段解释，一眼看清范围、身份边界与执行前提 / 3-cell structured spec strip */}
          <div className='grid grid-cols-1 gap-2.5 rounded-[var(--radius-control)] bg-muted/40 px-3 py-2 text-xs sm:grid-cols-3 dark:bg-muted/25'>
            <div className='min-w-0'>
              <div className='text-[11px] text-muted-foreground'>
                {t('host.k8sRbacSpecScope')}
              </div>
              <div className='truncate font-medium text-foreground'>
                {t('host.k8sRbacSpecScopeValue')} (
                <span className='font-mono'>{effectiveNs}</span>)
              </div>
            </div>
            <div className='min-w-0'>
              <div className='text-[11px] text-muted-foreground'>
                {t('host.k8sRbacSpecIdentity')}
              </div>
              <div className='truncate font-medium text-foreground'>
                {t('host.k8sRbacSpecIdentityValue')}
              </div>
            </div>
            <div className='min-w-0'>
              <div className='text-[11px] text-muted-foreground'>
                {t('host.k8sRbacSpecPrereq')}
              </div>
              <div className='truncate font-medium text-foreground'>
                {t('host.k8sRbacSpecPrereqValue')}
              </div>
            </div>
          </div>

          <div className='grid grid-cols-1 gap-3 sm:grid-cols-2 sm:items-end'>
            <Input
              label={t('host.k8sRbacTemplateManagerSa')}
              value={managerSA}
              onChange={(e) => setManagerSA(e.target.value)}
              placeholder={DEFAULT_STX_MANAGER_SA}
              className='font-mono text-xs'
              data-testid='k8s-rbac-template-manager-sa'
            />
            <div className='flex items-center justify-between gap-2 rounded-[var(--radius-control)] bg-muted/40 px-3 py-1.5 text-[11px] dark:bg-muted/25'>
              <div className='min-w-0'>
                <div className='text-muted-foreground'>
                  {t('host.k8sRbacContextConfirmLabel')}
                </div>
                <code className='block truncate font-mono text-foreground'>
                  {CONTEXT_CHECK_CMD}
                </code>
              </div>
              <CopyButton
                value={CONTEXT_CHECK_CMD}
                label={t('common.copy')}
                variant='plain'
                className='h-7 shrink-0 px-2 text-xs'
              />
            </div>
          </div>

          <div className='grid grid-cols-1 gap-2.5 rounded-[var(--radius-control)] bg-muted/40 p-2.5 sm:grid-cols-2 dark:bg-muted/25'>
            <Checkbox
              checked={includeDeleteRules}
              onCheckedChange={(checked) => setIncludeDeleteRules(checked === true)}
              label={t('host.k8sRbacOptDelete')}
              description={t('host.k8sRbacOptDeleteDesc')}
              data-testid='k8s-rbac-opt-delete'
            />
            <Checkbox
              checked={includeSecretRules}
              onCheckedChange={(checked) => setIncludeSecretRules(checked === true)}
              label={t('host.k8sRbacOptSecret')}
              description={t('host.k8sRbacOptSecretDesc')}
              data-testid='k8s-rbac-opt-secret'
            />
          </div>

          <div className='space-y-1.5'>
            <div className='flex flex-wrap items-center justify-between gap-2 text-xs text-muted-foreground'>
              <span className='flex flex-wrap items-center gap-1.5'>
                <span>{t('host.k8sRbacTemplateYamlLabel')}</span>
                <code className='rounded bg-muted/70 px-1.5 py-0.5 font-mono text-[11px] text-foreground dark:bg-muted/40'>
                  kubectl apply -f stx-rbac.yaml
                </code>
              </span>
              <CopyButton
                value={yaml}
                label={t('common.copy')}
                variant='plain'
                className='h-7 px-2 text-xs'
              />
            </div>
            <pre
              data-testid='k8s-rbac-template-yaml'
              className='m-0 max-h-48 overflow-auto rounded-[var(--radius-control)] bg-muted/50 px-3 py-2.5 font-mono text-[11px] leading-relaxed text-foreground dark:bg-muted/30'
            >
              {yaml}
            </pre>
          </div>

          {authMode === 'kubeconfig' ? (
            <div className='space-y-1.5'>
              <div className='flex items-center justify-between gap-2 text-xs text-muted-foreground'>
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
                className='m-0 max-h-40 overflow-auto rounded-[var(--radius-control)] bg-muted/50 px-3 py-2 font-mono text-[11px] leading-relaxed text-foreground dark:bg-muted/30'
              >
                {kubeconfigCommand}
              </pre>
              <p className='text-[11px] text-muted-foreground'>
                {t('host.k8sRbacTemplateKubeconfigHint')}
              </p>
            </div>
          ) : (
            <div className='space-y-1.5'>
              <div className='flex items-center justify-between gap-2 text-xs text-muted-foreground'>
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
