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
 * Kubernetes Kubeconfig 上传与即时结构化解析组件（Structured Output）：
 * 支持文件上传或粘贴 YAML，250ms 防抖调用后端 inspectK8sKubeconfig，
 * 解析结果以单层扁平结构化卡片呈现（状态头 + 上下文/命名空间/到期字段层级 + 风险行内修复操作）。
 * Kubernetes Kubeconfig upload & structured-output inspection field:
 * supports upload or paste with 250ms debounced inspectK8sKubeconfig, rendering parsed metadata
 * as a compact structured card with field hierarchy and inline remediation actions.
 */

import {useEffect, useMemo, useRef, useState} from 'react';
import {useTranslations} from 'next-intl';
import {AlertTriangle, Upload} from 'lucide-react';
import {Badge} from '@/components/ui/badge';
import {Label} from '@/components/ui/label';
import {Textarea} from '@/components/ui/textarea';
import {Button} from '@/components/arc/button/button';
import {CopyButton} from '@/components/arc/copy-button/copy-button';
import {Input} from '@/components/arc/input/input';
import {Select} from '@/components/arc/select/select';
import services from '@/lib/services';
import type {
  K8sAccessMode,
  K8sKubeconfigInspectResult,
} from '@/lib/services/host/types';
import {renderK8sExportKubeconfigCommand} from '@/lib/k8s-rbac-templates';
import {formatDateTimeTz} from '@/lib/timezone';
import {getK8sCredentialExpiryState} from './host-status';

interface K8sKubeconfigFieldProps {
  id?: string;
  label: string;
  value: string;
  onChange: (nextValue: string) => void;
  contextName: string;
  onContextChange: (nextContext: string) => void;
  serverOverride: string;
  onServerOverrideChange: (nextServer: string) => void;
  onNamespaceDetected?: (ns: string) => void;
  onInspectResult?: (result: K8sKubeconfigInspectResult | null) => void;
  accessMode?: K8sAccessMode;
  error?: string;
  hint?: string;
  testIdPrefix?: string;
}

const MAX_KUBECONFIG_FILE_BYTES = 1024 * 1024;

function isLoopbackUrl(urlStr: string): boolean {
  try {
    const u = new URL(urlStr.trim());
    const h = u.hostname.toLowerCase();
    return h === 'localhost' || h === '127.0.0.1' || h === '::1' || h === '0.0.0.0';
  } catch {
    return false;
  }
}

interface ParsedWarningItem {
  code: string;
  message: string;
  withExportAction?: boolean;
}

export function K8sKubeconfigField({
  id = 'k8s_kubeconfig',
  label,
  value,
  onChange,
  contextName,
  onContextChange,
  serverOverride,
  onServerOverrideChange,
  onNamespaceDetected,
  onInspectResult,
  accessMode = 'in_cluster',
  error,
  hint,
  testIdPrefix = 'k8s-kubeconfig',
}: K8sKubeconfigFieldProps) {
  const t = useTranslations();
  const fileInputRef = useRef<HTMLInputElement | null>(null);
  const [inspecting, setInspecting] = useState(false);
  const [inspectResult, setInspectResult] =
    useState<K8sKubeconfigInspectResult | null>(null);
  const serverTouchedRef = useRef(false);
  const lastSeededContextRef = useRef('');
  const exportKubeconfigCmd = renderK8sExportKubeconfigCommand();

  const onContextChangeRef = useRef(onContextChange);
  onContextChangeRef.current = onContextChange;
  const onServerOverrideChangeRef = useRef(onServerOverrideChange);
  onServerOverrideChangeRef.current = onServerOverrideChange;
  const onNamespaceDetectedRef = useRef(onNamespaceDetected);
  onNamespaceDetectedRef.current = onNamespaceDetected;
  const onInspectResultRef = useRef(onInspectResult);
  onInspectResultRef.current = onInspectResult;

  useEffect(() => {
    const trimmed = value.trim();
    if (!trimmed) {
      serverTouchedRef.current = false;
      lastSeededContextRef.current = '';
      setInspecting(false);
      setInspectResult(null);
      onInspectResultRef.current?.(null);
      return;
    }
    if (typeof services.host.inspectK8sKubeconfig !== 'function') {
      return;
    }

    const controller = new AbortController();
    const timer = setTimeout(() => {
      setInspecting(true);
      void services.host
        .inspectK8sKubeconfig(trimmed, contextName || undefined, controller.signal)
        .then((res) => {
          if (controller.signal.aborted) {
            return;
          }
          setInspectResult(res);
          onInspectResultRef.current?.(res);
          if (!res.valid) {
            return;
          }
          const activeName =
            res.selected_context || contextName || res.contexts?.[0]?.name || '';
          if (activeName && !contextName) {
            onContextChangeRef.current(activeName);
          }
          const activeSummary = res.contexts?.find((ctx) => ctx.name === activeName);
          if (activeSummary) {
            const seedKey = `${activeSummary.name}|${activeSummary.server}`;
            if (!serverTouchedRef.current || lastSeededContextRef.current !== seedKey) {
              lastSeededContextRef.current = seedKey;
              serverTouchedRef.current = false;
              onServerOverrideChangeRef.current(activeSummary.server || '');
            }
            if (activeSummary.namespace) {
              onNamespaceDetectedRef.current?.(activeSummary.namespace);
            }
          }
        })
        .catch(() => {
          // 防抖请求被取消或接口不可用时保留现有表单输入，不阻塞手动提交。
          // Ignore aborted/transient inspect failures so users can still proceed cleanly.
        })
        .finally(() => {
          if (!controller.signal.aborted) {
            setInspecting(false);
          }
        });
    }, 250);

    return () => {
      clearTimeout(timer);
      controller.abort();
    };
  }, [value, contextName]);

  const activeSummary = useMemo(() => {
    if (!inspectResult?.valid || !inspectResult.contexts?.length) {
      return null;
    }
    const targetName = inspectResult.selected_context || contextName;
    return (
      inspectResult.contexts.find((c) => c.name === targetName) ||
      inspectResult.contexts[0] ||
      null
    );
  }, [inspectResult, contextName]);

  const warnings = useMemo(() => {
    if (!inspectResult?.valid) {
      return [] as ParsedWarningItem[];
    }
    const list: ParsedWarningItem[] = [];
    const rawWarnings = inspectResult.warnings || [];
    for (const code of rawWarnings) {
      if (code === 'loopback_server') {
        const effectiveServer = serverOverride.trim() || activeSummary?.server || '';
        if (accessMode !== 'in_cluster' && effectiveServer && isLoopbackUrl(effectiveServer)) {
          list.push({code, message: t('host.k8sKubeconfigWarnLoopback')});
        }
        continue;
      }
      if (code === 'external_file_refs') {
        list.push({
          code,
          message: t('host.k8sKubeconfigWarnExternalFiles'),
          withExportAction: true,
        });
      } else if (code === 'exec_plugin') {
        list.push({code, message: t('host.k8sKubeconfigWarnExecPlugin')});
      } else if (code === 'missing_current_context') {
        list.push({code, message: t('host.k8sKubeconfigWarnMissingContext')});
      }
    }
    return list;
  }, [inspectResult, serverOverride, activeSummary, accessMode, t]);

  const expiryState = getK8sCredentialExpiryState(activeSummary?.credential_expires_at);
  const hasExternalFilesWarn = warnings.some((w) => w.withExportAction);

  const handleFileChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    event.target.value = '';
    if (!file || file.size > MAX_KUBECONFIG_FILE_BYTES) {
      return;
    }
    const reader = new FileReader();
    reader.onload = () => {
      if (typeof reader.result === 'string') {
        serverTouchedRef.current = false;
        onContextChange('');
        onChange(reader.result);
      }
    };
    reader.readAsText(file);
  };

  return (
    <div className='space-y-2.5'>
      <div className='flex items-center justify-between gap-2'>
        <Label htmlFor={id}>{label}</Label>
        <div>
          <input
            ref={fileInputRef}
            type='file'
            accept='.yaml,.yml,.conf,.kubeconfig,text/yaml,text/plain'
            className='hidden'
            onChange={handleFileChange}
            data-testid={`${testIdPrefix}-file-input`}
          />
          <Button
            type='button'
            size='sm'
            variant='secondary'
            onClick={() => fileInputRef.current?.click()}
            data-testid={`${testIdPrefix}-upload-btn`}
          >
            <Upload className='size-3.5' aria-hidden />
            {t('host.k8sKubeconfigUpload')}
          </Button>
        </div>
      </div>

      <Textarea
        id={id}
        value={value}
        aria-invalid={Boolean(error)}
        onChange={(e) => {
          serverTouchedRef.current = false;
          onContextChange('');
          onChange(e.target.value);
        }}
        placeholder={t('host.kubeconfigPlaceholder')}
        className='font-mono text-xs'
        rows={5}
      />

      {error ? (
        <p className='text-xs text-destructive'>{error}</p>
      ) : hint && !inspectResult?.valid ? (
        <p className='text-xs text-muted-foreground'>{hint}</p>
      ) : null}

      {inspecting ? (
        <p className='text-xs text-muted-foreground'>{t('host.k8sKubeconfigInspecting')}</p>
      ) : inspectResult && value.trim() ? (
        !inspectResult.valid ? (
          <p
            className='text-xs text-destructive'
            data-testid={`${testIdPrefix}-inspect-error`}
          >
            {t('host.k8sKubeconfigParseFailed', {error: inspectResult.error || ''})}
          </p>
        ) : (
          /* Structured Output 解析结果卡片：状态头 + 字段层级 + 行内修复动作 / Structured parsed card */
          <div
            className='overflow-hidden rounded-[var(--radius-control)] bg-muted/35 dark:bg-muted/25'
            data-testid={`${testIdPrefix}-inspect-panel`}
          >
            <div className='flex flex-wrap items-center justify-between gap-2 px-3 py-2 text-xs'>
              <div className='flex min-w-0 flex-wrap items-center gap-1.5'>
                <span className='size-1.5 shrink-0 rounded-full bg-emerald-500' aria-hidden />
                <span className='font-medium text-foreground'>
                  {t('host.k8sKubeconfigParsed')}
                </span>
                {inspectResult.contexts.length <= 1 ? (
                  <Badge
                    variant='outline'
                    className='font-mono text-[11px]'
                    data-testid={`${testIdPrefix}-context-badge`}
                  >
                    {activeSummary?.name || '-'}
                  </Badge>
                ) : null}
                {activeSummary?.namespace ? (
                  <Badge variant='outline' className='font-mono text-[11px]'>
                    ns: {activeSummary.namespace}
                  </Badge>
                ) : null}
              </div>

              {activeSummary?.credential_expires_at ? (
                <div className='flex items-center gap-1.5 text-[11px]'>
                  <span className='font-mono text-muted-foreground tabular-nums'>
                    {formatDateTimeTz(activeSummary.credential_expires_at)}
                  </span>
                  {expiryState === 'expired' ? (
                    <Badge variant='destructive' className='text-[11px]'>
                      {t('host.k8sCredentialExpired')}
                    </Badge>
                  ) : expiryState === 'expiring_soon' ? (
                    <Badge variant='secondary' className='text-[11px]'>
                      {t('host.k8sCredentialExpiringSoon')}
                    </Badge>
                  ) : null}
                </div>
              ) : null}
            </div>

            <div className='space-y-2.5 border-t-[0.5px] border-border/70 px-3 py-2.5'>
              <div
                className={
                  inspectResult.contexts.length > 1
                    ? 'grid grid-cols-1 gap-3 sm:grid-cols-2'
                    : ''
                }
              >
                {inspectResult.contexts.length > 1 ? (
                  <Select
                    label={t('host.k8sKubeconfigContext')}
                    value={contextName || inspectResult.selected_context || ''}
                    onValueChange={(nextCtx) => {
                      serverTouchedRef.current = false;
                      onContextChange(nextCtx);
                    }}
                    options={inspectResult.contexts.map((ctx) => ({
                      value: ctx.name,
                      label: ctx.namespace ? `${ctx.name} (${ctx.namespace})` : ctx.name,
                    }))}
                    data-testid={`${testIdPrefix}-context-select`}
                  />
                ) : null}

                <Input
                  label={t('host.k8sKubeconfigServerOverride')}
                  description={t('host.k8sKubeconfigServerOverrideHint')}
                  value={serverOverride}
                  onChange={(e) => {
                    serverTouchedRef.current = true;
                    onServerOverrideChange(e.target.value);
                  }}
                  placeholder='https://10.0.0.10:6443'
                  className='font-mono text-xs'
                  data-testid={`${testIdPrefix}-server-override`}
                />
              </div>

              {warnings.length > 0 ? (
                <div
                  className='space-y-1.5'
                  data-testid={`${testIdPrefix}-warnings`}
                >
                  {warnings.map((item, idx) => (
                    <div
                      key={`${item.code}-${idx}`}
                      className='flex flex-wrap items-center justify-between gap-2 rounded-md bg-amber-500/10 px-2.5 py-1.5 text-xs text-amber-800 dark:text-amber-300'
                    >
                      <span className='flex min-w-0 items-start gap-1.5'>
                        <AlertTriangle className='mt-0.5 size-3.5 shrink-0' aria-hidden />
                        <span>{item.message}</span>
                      </span>
                      {item.withExportAction ? (
                        <CopyButton
                          value={exportKubeconfigCmd}
                          label={t('host.k8sKubeconfigCopyExport')}
                          variant='plain'
                          className='h-6 shrink-0 px-2 text-[11px]'
                        />
                      ) : null}
                    </div>
                  ))}
                </div>
              ) : null}
            </div>
          </div>
        )
      ) : null}

      {!hasExternalFilesWarn ? (
        <div className='flex items-center justify-between gap-2 rounded-[var(--radius-control)] bg-muted/40 px-2.5 py-1.5 text-[11px] dark:bg-muted/25'>
          <div className='flex min-w-0 items-center gap-2'>
            <span className='shrink-0 text-muted-foreground'>
              {t('host.k8sKubeconfigQuickCmdLabel')}
            </span>
            <code className='truncate font-mono text-foreground'>
              {exportKubeconfigCmd}
            </code>
          </div>
          <CopyButton
            value={exportKubeconfigCmd}
            label={t('common.copy')}
            variant='plain'
            className='h-6 shrink-0 px-2 text-[11px]'
          />
        </div>
      ) : null}
    </div>
  );
}
