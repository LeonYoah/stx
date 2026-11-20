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
  Activity,
  Check,
  Clock,
  Cloud,
  Container,
  Copy,
  Cpu,
  HardDrive,
  Layers,
  MemoryStick,
  Network,
  Pencil,
  Server,
  Terminal,
} from 'lucide-react';
import {Button} from '@/components/arc/button/button';
import services from '@/lib/services';
import {HostInfo, HostStatus, HostType} from '@/lib/services/host/types';
import {formatDateTimeTz} from '@/lib/timezone';
import layout from '@/components/common/layout/list-detail.module.css';

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

function getHostTypeIcon(hostType: HostType) {
  switch (hostType) {
    case HostType.BARE_METAL:
      return <Server className='h-4 w-4' />;
    case HostType.DOCKER:
      return <Container className='h-4 w-4' />;
    case HostType.KUBERNETES:
      return <Cloud className='h-4 w-4' />;
    default:
      return <Server className='h-4 w-4' />;
  }
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
      await navigator.clipboard.writeText(commandText);
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
          <div className='space-y-4 p-1'>
            <div className='flex flex-wrap items-center gap-2'>
              <span className='inline-flex size-8 items-center justify-center rounded-lg border border-primary/20 bg-primary/10 text-primary'>
                {getHostTypeIcon(host.host_type)}
              </span>
              <Badge variant={getStatusBadgeVariant(host.status)} className='gap-1.5'>
                {host.status === HostStatus.CONNECTED ? (
                  <span className='relative flex size-2'>
                    <span className='absolute inline-flex size-full animate-ping rounded-full bg-emerald-400 opacity-75' />
                    <span className='relative inline-flex size-2 rounded-full bg-emerald-500' />
                  </span>
                ) : null}
                {t(`host.statuses.${host.status}`)}
              </Badge>
              <Badge variant='outline' className='font-mono text-xs text-muted-foreground'>
                {t(
                  `host.types.${host.host_type === HostType.BARE_METAL ? 'bareMetal' : host.host_type}`,
                )}
              </Badge>
            </div>
            <p className='text-xs text-muted-foreground'>
              {host.description || t('host.noDescription')}
            </p>

            <div className='space-y-3 rounded-lg border border-border/70 bg-card/60 p-3.5'>
              <div className='flex items-center justify-between'>
                <div className='flex items-center gap-2 text-sm font-medium'>
                  <Activity className='size-4 text-primary' aria-hidden />
                  <span>{t('host.resources')}</span>
                </div>
                {host.last_check ? (
                  <div className='flex items-center gap-1.5 text-xs text-muted-foreground'>
                    <Clock className='size-3' aria-hidden />
                    <span>
                      {t('host.lastCheck')}: {formatDateTime(host.last_check)}
                    </span>
                  </div>
                ) : null}
              </div>
              <div className='grid grid-cols-1 gap-3 sm:grid-cols-3'>
                <div className='space-y-2 rounded-lg border border-border/60 bg-background/80 p-3'>
                  <div className='flex items-center justify-between text-xs'>
                    <span className='flex items-center gap-1.5 text-muted-foreground'>
                      <Cpu className='size-3.5 text-blue-500' aria-hidden />
                      CPU
                    </span>
                    <span className='font-mono font-medium'>
                      {host.cpu_usage?.toFixed(1) || 0}%
                    </span>
                  </div>
                  <Progress value={host.cpu_usage || 0} className='h-1.5' />
                  <div className='truncate text-[11px] text-muted-foreground'>
                    {host.cpu_cores ? `${host.cpu_cores} ${t('host.cores')}` : '-'}
                  </div>
                </div>
                <div className='space-y-2 rounded-lg border border-border/60 bg-background/80 p-3'>
                  <div className='flex items-center justify-between text-xs'>
                    <span className='flex items-center gap-1.5 text-muted-foreground'>
                      <MemoryStick className='size-3.5 text-emerald-500' aria-hidden />
                      {t('host.memory')}
                    </span>
                    <span className='font-mono font-medium'>
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
                <div className='space-y-2 rounded-lg border border-border/60 bg-background/80 p-3'>
                  <div className='flex items-center justify-between text-xs'>
                    <span className='flex items-center gap-1.5 text-muted-foreground'>
                      <HardDrive className='size-3.5 text-purple-500' aria-hidden />
                      {t('host.disk')}
                    </span>
                    <span className='font-mono font-medium'>
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
            </div>

            <div className='space-y-3 rounded-lg border border-border/70 bg-card/60 p-3.5'>
              <div className='flex items-center gap-2 text-sm font-medium'>
                <Layers className='size-4 text-primary' aria-hidden />
                <span>{t('host.basicInfo')}</span>
              </div>
              <div className='grid grid-cols-2 gap-x-6 gap-y-3 text-xs'>
                <div className='flex justify-between border-b border-border/40 py-1'>
                  <span className='text-muted-foreground'>{t('host.hostType')}</span>
                  <span className='font-medium'>
                    {t(
                      `host.types.${host.host_type === HostType.BARE_METAL ? 'bareMetal' : host.host_type}`,
                    )}
                  </span>
                </div>
                <div className='flex justify-between border-b border-border/40 py-1'>
                  <span className='text-muted-foreground'>{t('host.status')}</span>
                  <span className='font-medium'>
                    {t(`host.statuses.${host.status}`)}
                  </span>
                </div>
                <div className='flex justify-between border-b border-border/40 py-1'>
                  <span className='text-muted-foreground'>{t('host.createdAt')}</span>
                  <span className='font-mono text-muted-foreground'>
                    {formatDateTime(host.created_at)}
                  </span>
                </div>
                <div className='flex justify-between border-b border-border/40 py-1'>
                  <span className='text-muted-foreground'>{t('host.updatedAt')}</span>
                  <span className='font-mono text-muted-foreground'>
                    {formatDateTime(host.updated_at)}
                  </span>
                </div>
              </div>
            </div>

            <div className='space-y-3 rounded-lg border border-border/70 bg-card/60 p-3.5'>
              <div className='flex items-center gap-2 text-sm font-medium'>
                <Network className='size-4 text-primary' aria-hidden />
                <span>{t('host.connectionInfo')}</span>
              </div>
              {host.host_type === HostType.BARE_METAL ? (
                <div className='grid grid-cols-2 gap-x-6 gap-y-3 text-xs'>
                  <div className='flex justify-between border-b border-border/40 py-1'>
                    <span className='text-muted-foreground'>{t('host.ipAddress')}</span>
                    <span className='font-mono font-medium'>
                      {host.ip_address || '-'}
                    </span>
                  </div>
                  <div className='flex justify-between border-b border-border/40 py-1'>
                    <span className='text-muted-foreground'>
                      {t('host.agentVersion')}
                    </span>
                    <span className='font-mono'>{host.agent_version || '-'}</span>
                  </div>
                  <div className='flex justify-between border-b border-border/40 py-1'>
                    <span className='text-muted-foreground'>{t('host.osType')}</span>
                    <span>
                      {host.os_type
                        ? `${host.os_type} (${host.arch || 'x86_64'})`
                        : '-'}
                    </span>
                  </div>
                  <div className='flex justify-between border-b border-border/40 py-1'>
                    <span className='text-muted-foreground'>
                      {t('host.lastHeartbeat')}
                    </span>
                    <span className='font-mono text-muted-foreground'>
                      {formatDateTime(host.last_heartbeat)}
                    </span>
                  </div>
                </div>
              ) : null}
              {host.host_type === HostType.DOCKER ? (
                <div className='grid grid-cols-2 gap-x-6 gap-y-3 text-xs'>
                  <div className='col-span-2 flex justify-between border-b border-border/40 py-1'>
                    <span className='text-muted-foreground'>
                      {t('host.dockerApiUrl')}
                    </span>
                    <span className='max-w-[320px] truncate font-mono font-medium'>
                      {host.docker_api_url || '-'}
                    </span>
                  </div>
                  <div className='flex justify-between border-b border-border/40 py-1'>
                    <span className='text-muted-foreground'>
                      {t('host.tlsEnabled')}
                    </span>
                    <span>
                      {host.docker_tls_enabled ? t('common.yes') : t('common.no')}
                    </span>
                  </div>
                  <div className='flex justify-between border-b border-border/40 py-1'>
                    <span className='text-muted-foreground'>
                      {t('host.dockerVersion')}
                    </span>
                    <span className='font-mono'>{host.docker_version || '-'}</span>
                  </div>
                </div>
              ) : null}
              {host.host_type === HostType.KUBERNETES ? (
                <div className='grid grid-cols-2 gap-x-6 gap-y-3 text-xs'>
                  <div className='col-span-2 flex justify-between border-b border-border/40 py-1'>
                    <span className='text-muted-foreground'>{t('host.k8sApiUrl')}</span>
                    <span className='max-w-[320px] truncate font-mono font-medium'>
                      {host.k8s_api_url || '-'}
                    </span>
                  </div>
                  <div className='flex justify-between border-b border-border/40 py-1'>
                    <span className='text-muted-foreground'>{t('host.namespace')}</span>
                    <span className='font-mono'>
                      {host.k8s_namespace || 'default'}
                    </span>
                  </div>
                  <div className='flex justify-between border-b border-border/40 py-1'>
                    <span className='text-muted-foreground'>
                      {t('host.k8sVersion')}
                    </span>
                    <span className='font-mono'>{host.k8s_version || '-'}</span>
                  </div>
                </div>
              ) : null}
            </div>

            {host.host_type === HostType.BARE_METAL ? (
              <div className='space-y-4 rounded-lg border border-border/70 bg-card/60 p-3.5'>
                <div className='flex items-center gap-2 text-sm font-medium'>
                  <Terminal className='size-4 text-primary' aria-hidden />
                  <span>
                    {t('host.installCommand')} / {t('host.uninstallCommand')}
                  </span>
                </div>
                {host.status === HostStatus.PENDING ? (
                  <div className='flex items-start gap-2.5 rounded-lg border border-amber-500/30 bg-amber-500/10 p-3 text-xs text-amber-700 dark:text-amber-300'>
                    <span className='mt-1 size-2 shrink-0 animate-pulse rounded-full bg-amber-500' />
                    <div className='space-y-0.5'>
                      <span className='block font-semibold'>
                        {t('host.installGuide.title')}
                      </span>
                      <span className='block text-[11px] text-muted-foreground'>
                        {t('host.installGuide.pendingAlert')}
                      </span>
                    </div>
                  </div>
                ) : null}
                <div className='space-y-1.5'>
                  <div className='flex items-center justify-between text-xs font-medium text-muted-foreground'>
                    <span>{t('host.installCommand')}</span>
                    <span className='font-mono text-[11px] text-muted-foreground/80'>
                      bash
                    </span>
                  </div>
                  <div className='overflow-hidden rounded-lg border border-zinc-800 bg-zinc-950 shadow-sm'>
                    <div className='flex items-center justify-between border-b border-zinc-800 bg-zinc-900/90 px-3 py-1.5'>
                      <span className='ml-2 font-mono text-[11px] text-zinc-400'>
                        seatunnel-agent-install
                      </span>
                      {installCommand ? (
                        <Button
                          type='button'
                          variant='ghost'
                          size='sm'
                          className='h-6 gap-1 px-2 text-[11px] text-zinc-300'
                          onClick={() => void handleCopyCommand(installCommand, false)}
                        >
                          {copiedInstall ? (
                            <>
                              <Check className='size-3 text-emerald-400' aria-hidden />
                              <span className='text-emerald-400'>{t('common.copied')}</span>
                            </>
                          ) : (
                            <>
                              <Copy className='size-3' aria-hidden />
                              <span>{t('common.copy')}</span>
                            </>
                          )}
                        </Button>
                      ) : null}
                    </div>
                    <div className='select-all overflow-x-auto p-3 font-mono text-xs leading-relaxed break-all whitespace-pre-wrap text-emerald-400'>
                      {loadingCommand
                        ? `${t('common.loading')}...`
                        : installCommand || t('host.noInstallCommand')}
                    </div>
                  </div>
                </div>
                {installCommand ? (
                  <div className='space-y-1.5 pt-1'>
                    <div className='flex items-center justify-between text-xs font-medium text-muted-foreground'>
                      <span>{t('host.uninstallCommand')}</span>
                      <span className='font-mono text-[11px] text-muted-foreground/80'>
                        bash
                      </span>
                    </div>
                    <div className='overflow-hidden rounded-lg border border-zinc-800 bg-zinc-950 shadow-sm'>
                      <div className='flex items-center justify-between border-b border-zinc-800 bg-zinc-900/90 px-3 py-1.5'>
                        <span className='ml-2 font-mono text-[11px] text-zinc-400'>
                          seatunnel-agent-uninstall
                        </span>
                        <Button
                          type='button'
                          variant='ghost'
                          size='sm'
                          className='h-6 gap-1 px-2 text-[11px] text-zinc-300'
                          onClick={() =>
                            void handleCopyCommand(
                              getUninstallCommand(installCommand),
                              true,
                            )
                          }
                        >
                          {copiedUninstall ? (
                            <>
                              <Check className='size-3 text-emerald-400' aria-hidden />
                              <span className='text-emerald-400'>{t('common.copied')}</span>
                            </>
                          ) : (
                            <>
                              <Copy className='size-3' aria-hidden />
                              <span>{t('common.copy')}</span>
                            </>
                          )}
                        </Button>
                      </div>
                      <div className='select-all overflow-x-auto p-3 font-mono text-xs leading-relaxed break-all whitespace-pre-wrap text-amber-300/90'>
                        {getUninstallCommand(installCommand)}
                      </div>
                    </div>
                    <p className='text-[11px] text-muted-foreground'>
                      {t('host.uninstallCommandTip')}
                    </p>
                  </div>
                ) : null}
              </div>
            ) : null}
          </div>
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
