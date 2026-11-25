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
 * 主机详情体：供 DetailHost 插槽使用（不再自带 ui/sheet 宿主）。
 * Host detail body for the DetailHost slot (no longer owns a ui/sheet host).
 */

import {useEffect, useState} from 'react';
import {useTranslations} from 'next-intl';
import {Badge} from '@/components/ui/badge';
import {Progress} from '@/components/ui/progress';
import {Skeleton} from '@/components/ui/skeleton';
import {toast} from '@/lib/toast';
import {
  Check,
  Clock,
  Copy,
  Cpu,
  HardDrive,
  MemoryStick,
  Pencil,
} from 'lucide-react';
import {Button} from '@/components/arc/button/button';
import services from '@/lib/services';
import {HostInfo, HostStatus, HostType} from '@/lib/services/host/types';
import {formatDateTimeTz} from '@/lib/timezone';
import {cn} from '@/lib/utils';
import layout from '@/components/common/layout/list-detail.module.css';
import {writeClipboard} from '@/components/arc/lib/use-copy-feedback';

interface HostDetailProps {
  host: HostInfo | null;
  loading?: boolean;
  onEdit: () => void;
}

function formatBytes(bytes: number | undefined): string {
  if (!bytes) {
    return '-';
  }
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let unitIndex = 0;
  let value = bytes;
  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024;
    unitIndex++;
  }
  return `${value.toFixed(1)} ${units[unitIndex]}`;
}

function formatDateTime(dateStr: string | null | undefined): string {
  if (!dateStr) {
    return '-';
  }
  return formatDateTimeTz(dateStr);
}

function getStatusBadgeVariant(
  status: HostStatus,
): 'default' | 'secondary' | 'destructive' | 'outline' {
  switch (status) {
    case HostStatus.CONNECTED:
      return 'default';
    case HostStatus.PENDING:
      return 'secondary';
    case HostStatus.OFFLINE:
      return 'outline';
    case HostStatus.ERROR:
      return 'destructive';
    default:
      return 'secondary';
  }
}

/**
 * 从安装命令生成卸载命令，移除安装专属参数（如 --host-id、--auth-token）
 * Generate uninstall command from install command, stripping install-only flags
 */
function getUninstallCommand(installCmd: string): string {
  if (!installCmd) {
    return '';
  }
  return installCmd
    .replace('/install.sh', '/uninstall.sh')
    .replace(/\s+--host-id(?:=\S+|\s+\S+)/g, '')
    .replace(/\s+--auth-token(?:=\S+|\s+\S+)/g, '')
    .replace(/\s+--control-plane-addr(?:=\S+|\s+\S+)/g, '')
    .trim();
}

export function HostDetail({host, loading = false, onEdit}: HostDetailProps) {
  const t = useTranslations();
  const [installCommand, setInstallCommand] = useState<string>('');
  const [loadingCommand, setLoadingCommand] = useState(false);
  const [copiedInstall, setCopiedInstall] = useState(false);
  const [copiedUninstall, setCopiedUninstall] = useState(false);

  useEffect(() => {
    if (!host || host.host_type !== HostType.BARE_METAL) {
      setInstallCommand('');
      return;
    }
    let cancelled = false;
    setLoadingCommand(true);
    void services.host
      .getInstallCommandSafe(host.id)
      .then((result) => {
        if (cancelled) {
          return;
        }
        if (result.success && result.data) {
          setInstallCommand(result.data.command);
        } else {
          setInstallCommand('');
        }
      })
      .catch(() => {
        if (!cancelled) {
          setInstallCommand('');
        }
      })
      .finally(() => {
        if (!cancelled) {
          setLoadingCommand(false);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [host]);

  const handleCopyCommand = async (commandText: string, isUninstall = false) => {
    try {
      await writeClipboard(commandText);
      if (isUninstall) {
        setCopiedUninstall(true);
        setTimeout(() => setCopiedUninstall(false), 2000);
      } else {
        setCopiedInstall(true);
        setTimeout(() => setCopiedInstall(false), 2000);
      }
      toast.success(t('host.commandCopied'));
    } catch {
      toast.error(t('host.copyFailed'));
    }
  };

  return (
    <div className={layout.detailCol} data-testid='host-detail'>
      <div className={layout.detailScroll}>
        {loading && !host ? (
          <div className='space-y-3 p-1'>
            <Skeleton className='h-16 w-full' />
            <Skeleton className='h-28 w-full' />
            <Skeleton className='h-40 w-full' />
          </div>
        ) : host ? (
          <>
            {/* 单层扁平布局：DetailHost 已是唯一外框，各组信息仅用留白 + 0.5px 细线分隔。
                Single-layer flat layout: DetailHost is the only frame; groups use whitespace + 0.5px hairlines. */}
            <section className={cn(layout.detailSection, layout.detailSectionFirst)}>
              <div className='flex flex-wrap items-center gap-2'>
                <Badge
                  variant={getStatusBadgeVariant(host.status)}
                  className='gap-1.5'
                >
                  {host.status === HostStatus.CONNECTED ? (
                    <span className='relative flex size-1.5'>
                      <span className='absolute inline-flex size-full animate-ping rounded-full bg-emerald-400 opacity-75' />
                      <span className='relative inline-flex size-1.5 rounded-full bg-emerald-500' />
                    </span>
                  ) : null}
                  {t(`host.statuses.${host.status}`)}
                </Badge>
                <Badge
                  variant='outline'
                  className='font-mono text-xs text-muted-foreground'
                >
                  {t(
                    `host.types.${host.host_type === HostType.BARE_METAL ? 'bareMetal' : host.host_type}`,
                  )}
                </Badge>
              </div>
              <p className='text-xs text-muted-foreground'>
                {host.description || t('host.noDescription')}
              </p>
            </section>

            <section className={layout.detailSection}>
              <div className={layout.detailSectionHead}>
                <h3 className={layout.sectionTitle}>{t('host.resources')}</h3>
                {host.last_check ? (
                  <span className='inline-flex items-center gap-1 text-xs text-muted-foreground'>
                    <Clock className='size-3' aria-hidden />
                    {formatDateTime(host.last_check)}
                  </span>
                ) : null}
              </div>
              <div className='grid grid-cols-1 gap-4 sm:grid-cols-3'>
                <div className='space-y-1.5'>
                  <div className='flex items-center justify-between text-xs'>
                    <span className='inline-flex items-center gap-1.5 text-muted-foreground'>
                      <Cpu className='size-3.5 text-blue-500' aria-hidden />
                      CPU
                    </span>
                    <span className='font-mono font-medium tabular-nums'>
                      {host.cpu_usage?.toFixed(1) || 0}%
                    </span>
                  </div>
                  <Progress value={host.cpu_usage || 0} className='h-1.5' />
                  <div className='truncate text-[11px] text-muted-foreground'>
                    {host.cpu_cores ? `${host.cpu_cores} ${t('host.cores')}` : '-'}
                  </div>
                </div>
                <div className='space-y-1.5'>
                  <div className='flex items-center justify-between text-xs'>
                    <span className='inline-flex items-center gap-1.5 text-muted-foreground'>
                      <MemoryStick
                        className='size-3.5 text-emerald-500'
                        aria-hidden
                      />
                      {t('host.memory')}
                    </span>
                    <span className='font-mono font-medium tabular-nums'>
                      {host.memory_usage?.toFixed(1) || 0}%
                    </span>
                  </div>
                  <Progress value={host.memory_usage || 0} className='h-1.5' />
                  <div className='truncate text-[11px] text-muted-foreground'>
                    {host.total_memory
                      ? `${formatBytes(host.total_memory)} ${t('host.total')}`
                      : '-'}
                  </div>
                </div>
                <div className='space-y-1.5'>
                  <div className='flex items-center justify-between text-xs'>
                    <span className='inline-flex items-center gap-1.5 text-muted-foreground'>
                      <HardDrive
                        className='size-3.5 text-purple-500'
                        aria-hidden
                      />
                      {t('host.disk')}
                    </span>
                    <span className='font-mono font-medium tabular-nums'>
                      {host.disk_usage?.toFixed(1) || 0}%
                    </span>
                  </div>
                  <Progress value={host.disk_usage || 0} className='h-1.5' />
                  <div className='truncate text-[11px] text-muted-foreground'>
                    {host.total_disk
                      ? `${formatBytes(host.total_disk)} ${t('host.total')}`
                      : '-'}
                  </div>
                </div>
              </div>
            </section>

            <section className={layout.detailSection}>
              <h3 className={layout.sectionTitle}>{t('host.basicInfo')}</h3>
              <dl className={layout.detailKvGrid}>
                <div className={layout.detailKvItem}>
                  <dt className={layout.detailKvKey}>{t('host.hostType')}</dt>
                  <dd className={layout.detailKvVal}>
                    {t(
                      `host.types.${host.host_type === HostType.BARE_METAL ? 'bareMetal' : host.host_type}`,
                    )}
                  </dd>
                </div>
                <div className={layout.detailKvItem}>
                  <dt className={layout.detailKvKey}>{t('host.status')}</dt>
                  <dd className={layout.detailKvVal}>
                    {t(`host.statuses.${host.status}`)}
                  </dd>
                </div>
                <div className={layout.detailKvItem}>
                  <dt className={layout.detailKvKey}>{t('host.createdAt')}</dt>
                  <dd className={cn(layout.detailKvVal, 'font-mono text-xs')}>
                    {formatDateTime(host.created_at)}
                  </dd>
                </div>
                <div className={layout.detailKvItem}>
                  <dt className={layout.detailKvKey}>{t('host.updatedAt')}</dt>
                  <dd className={cn(layout.detailKvVal, 'font-mono text-xs')}>
                    {formatDateTime(host.updated_at)}
                  </dd>
                </div>
              </dl>
            </section>

            <section className={layout.detailSection}>
              <h3 className={layout.sectionTitle}>{t('host.connectionInfo')}</h3>
              {host.host_type === HostType.BARE_METAL ? (
                <dl className={layout.detailKvGrid}>
                  <div className={layout.detailKvItem}>
                    <dt className={layout.detailKvKey}>{t('host.ipAddress')}</dt>
                    <dd className={cn(layout.detailKvVal, 'font-mono')}>
                      {host.ip_address || '-'}
                    </dd>
                  </div>
                  <div className={layout.detailKvItem}>
                    <dt className={layout.detailKvKey}>
                      {t('host.agentVersion')}
                    </dt>
                    <dd className={cn(layout.detailKvVal, 'font-mono')}>
                      {host.agent_version || '-'}
                    </dd>
                  </div>
                  <div className={layout.detailKvItem}>
                    <dt className={layout.detailKvKey}>{t('host.osType')}</dt>
                    <dd className={layout.detailKvVal}>
                      {host.os_type
                        ? `${host.os_type} (${host.arch || 'x86_64'})`
                        : '-'}
                    </dd>
                  </div>
                  <div className={layout.detailKvItem}>
                    <dt className={layout.detailKvKey}>
                      {t('host.lastHeartbeat')}
                    </dt>
                    <dd className={cn(layout.detailKvVal, 'font-mono text-xs')}>
                      {formatDateTime(host.last_heartbeat)}
                    </dd>
                  </div>
                </dl>
              ) : null}
              {host.host_type === HostType.DOCKER ? (
                <dl className={layout.detailKvGrid}>
                  <div className={cn(layout.detailKvItem, layout.detailKvSpan)}>
                    <dt className={layout.detailKvKey}>
                      {t('host.dockerApiUrl')}
                    </dt>
                    <dd className={cn(layout.detailKvVal, 'font-mono text-xs')}>
                      {host.docker_api_url || '-'}
                    </dd>
                  </div>
                  <div className={layout.detailKvItem}>
                    <dt className={layout.detailKvKey}>
                      {t('host.tlsEnabled')}
                    </dt>
                    <dd className={layout.detailKvVal}>
                      {host.docker_tls_enabled ? t('common.yes') : t('common.no')}
                    </dd>
                  </div>
                  <div className={layout.detailKvItem}>
                    <dt className={layout.detailKvKey}>
                      {t('host.dockerVersion')}
                    </dt>
                    <dd className={cn(layout.detailKvVal, 'font-mono')}>
                      {host.docker_version || '-'}
                    </dd>
                  </div>
                </dl>
              ) : null}
              {host.host_type === HostType.KUBERNETES ? (
                <dl className={layout.detailKvGrid}>
                  <div className={cn(layout.detailKvItem, layout.detailKvSpan)}>
                    <dt className={layout.detailKvKey}>{t('host.k8sApiUrl')}</dt>
                    <dd className={cn(layout.detailKvVal, 'font-mono text-xs')}>
                      {host.k8s_api_url || '-'}
                    </dd>
                  </div>
                  <div className={layout.detailKvItem}>
                    <dt className={layout.detailKvKey}>{t('host.namespace')}</dt>
                    <dd className={cn(layout.detailKvVal, 'font-mono')}>
                      {host.k8s_namespace || 'default'}
                    </dd>
                  </div>
                  <div className={layout.detailKvItem}>
                    <dt className={layout.detailKvKey}>
                      {t('host.k8sVersion')}
                    </dt>
                    <dd className={cn(layout.detailKvVal, 'font-mono')}>
                      {host.k8s_version || '-'}
                    </dd>
                  </div>
                </dl>
              ) : null}
            </section>

            {host.host_type === HostType.BARE_METAL ? (
              <section className={layout.detailSection}>
                <h3 className={layout.sectionTitle}>
                  {t('host.installCommand')} / {t('host.uninstallCommand')}
                </h3>
                {host.status === HostStatus.PENDING ? (
                  <p className='flex items-start gap-2 text-xs text-amber-700 dark:text-amber-300'>
                    <span className='mt-1.5 size-1.5 shrink-0 animate-pulse rounded-full bg-amber-500' />
                    <span>{t('host.installGuide.pendingAlert')}</span>
                  </p>
                ) : null}
                <div className='space-y-1.5'>
                  <div className='flex items-center justify-between text-xs text-muted-foreground'>
                    <span>{t('host.installCommand')}</span>
                    {installCommand ? (
                      <Button
                        type='button'
                        variant='ghost'
                        size='sm'
                        onClick={() =>
                          void handleCopyCommand(installCommand, false)
                        }
                      >
                        {copiedInstall ? (
                          <Check
                            className='size-3.5 text-emerald-500'
                            aria-hidden
                          />
                        ) : (
                          <Copy className='size-3.5' aria-hidden />
                        )}
                        {copiedInstall ? t('common.copied') : t('common.copy')}
                      </Button>
                    ) : null}
                  </div>
                  <pre className='m-0 select-all overflow-x-auto rounded-[var(--radius-control)] bg-muted/50 px-3 py-2.5 font-mono text-xs leading-relaxed break-all whitespace-pre-wrap text-foreground dark:bg-muted/30'>
                    {loadingCommand
                      ? `${t('common.loading')}...`
                      : installCommand || t('host.noInstallCommand')}
                  </pre>
                </div>
                {installCommand ? (
                  <div className='space-y-1.5'>
                    <div className='flex items-center justify-between text-xs text-muted-foreground'>
                      <span>{t('host.uninstallCommand')}</span>
                      <Button
                        type='button'
                        variant='ghost'
                        size='sm'
                        onClick={() =>
                          void handleCopyCommand(
                            getUninstallCommand(installCommand),
                            true,
                          )
                        }
                      >
                        {copiedUninstall ? (
                          <Check
                            className='size-3.5 text-emerald-500'
                            aria-hidden
                          />
                        ) : (
                          <Copy className='size-3.5' aria-hidden />
                        )}
                        {copiedUninstall ? t('common.copied') : t('common.copy')}
                      </Button>
                    </div>
                    <pre className='m-0 select-all overflow-x-auto rounded-[var(--radius-control)] bg-muted/50 px-3 py-2.5 font-mono text-xs leading-relaxed break-all whitespace-pre-wrap text-muted-foreground dark:bg-muted/30'>
                      {getUninstallCommand(installCommand)}
                    </pre>
                    <p className='text-[11px] text-muted-foreground'>
                      {t('host.uninstallCommandTip')}
                    </p>
                  </div>
                ) : null}
              </section>
            ) : null}
          </>
        ) : (
          <p className='p-4 text-sm text-muted-foreground'>{t('host.loadError')}</p>
        )}
      </div>
      {host ? (
        <div className={layout.detailFoot}>
          <span className={layout.detailFootGrow} />
          <Button type='button' size='sm' onClick={onEdit}>
            <Pencil className='size-3.5' aria-hidden />
            {t('common.edit')}
          </Button>
        </div>
      ) : null}
    </div>
  );
}
