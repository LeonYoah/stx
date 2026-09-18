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
 * Host Detail Component
 * 主机详情组件
 *
 * Displays detailed information about a host including basic info,
 * Agent status, resource usage, and terminal install commands.
 * 显示主机详细信息，包括基本信息、Agent 状态、资源使用率和终端安装命令。
 */

import {useState, useEffect, useRef} from 'react';
import {useTranslations} from 'next-intl';
import {Button} from '@/components/ui/button';
import {Badge} from '@/components/ui/badge';
import {Progress} from '@/components/ui/progress';
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
  SheetDescription,
} from '@/components/ui/sheet';
import {toast} from 'sonner';
import {
  Copy,
  Check,
  Pencil,
  Server,
  Container,
  Cloud,
  Cpu,
  HardDrive,
  MemoryStick,
  Clock,
  Terminal,
  Activity,
  Layers,
  Network,
} from 'lucide-react';
import {useGSAP} from '@gsap/react';
import {animateSheetSections} from '@/lib/animations/gsap-motion';
import services from '@/lib/services';
import {HostInfo, HostType, HostStatus} from '@/lib/services/host/types';

interface HostDetailProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  host: HostInfo;
  onEdit: () => void;
}

/**
 * Format bytes to human readable string
 * 格式化字节为人类可读字符串
 */
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

/**
 * Format date time
 * 格式化日期时间
 */
function formatDateTime(dateStr: string | null | undefined): string {
  if (!dateStr) {
    return '-';
  }
  return new Date(dateStr).toLocaleString();
}

/**
 * Get host type icon
 * 获取主机类型图标
 */
function getHostTypeIcon(hostType: HostType) {
  switch (hostType) {
    case HostType.BARE_METAL:
      return <Server className='h-5 w-5' />;
    case HostType.DOCKER:
      return <Container className='h-5 w-5' />;
    case HostType.KUBERNETES:
      return <Cloud className='h-5 w-5' />;
    default:
      return <Server className='h-5 w-5' />;
  }
}

/**
 * Get status badge variant
 * 获取状态徽章变体
 */
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
 * Host Detail Component
 * 主机详情组件
 */
export function HostDetail({open, onOpenChange, host, onEdit}: HostDetailProps) {
  const t = useTranslations();
  const [installCommand, setInstallCommand] = useState<string>('');
  const [loadingCommand, setLoadingCommand] = useState(false);
  const [copiedInstall, setCopiedInstall] = useState(false);
  const [copiedUninstall, setCopiedUninstall] = useState(false);
  const contentContainerRef = useRef<HTMLDivElement>(null);

  /**
   * Load install command for bare_metal hosts
   * 加载物理机的安装命令
   */
  const loadInstallCommand = async () => {
    setLoadingCommand(true);
    try {
      const result = await services.host.getInstallCommandSafe(host.id);
      if (result.success && result.data) {
        setInstallCommand(result.data.command);
      }
    } catch (err) {
      console.error('Failed to load install command:', err);
    } finally {
      setLoadingCommand(false);
    }
  };

  useEffect(() => {
    if (open && host.host_type === HostType.BARE_METAL) {
      loadInstallCommand();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, host.id, host.host_type]);

  // 当侧滑抽屉展开时执行各区块平滑滑动交错动画
  // Trigger smooth staggered slide-in animation for sections when sheet opens
  useGSAP(
    () => {
      if (open && contentContainerRef.current) {
        animateSheetSections('.host-detail-section');
      }
    },
    {dependencies: [open, host.id], scope: contentContainerRef},
  );

  /**
   * Copy install command to clipboard
   * 复制安装命令到剪贴板
   */
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
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className='w-full sm:max-w-2xl p-0 flex flex-col h-full bg-background border-l border-border/80 shadow-2xl overflow-hidden'>
        {/* Header / 抽屉顶栏 */}
        <SheetHeader className='p-6 pb-5 border-b border-border/70 bg-muted/20 backdrop-blur-xs'>
          <div className='flex items-start gap-4'>
            <div className='h-12 w-12 rounded-xl bg-primary/10 text-primary border border-primary/20 flex items-center justify-center shrink-0 shadow-xs'>
              {getHostTypeIcon(host.host_type)}
            </div>
            <div className='flex-1 min-w-0 space-y-1.5'>
              <div className='flex items-center gap-2.5 flex-wrap'>
                <SheetTitle className='text-lg font-semibold tracking-tight truncate'>
                  {host.name}
                </SheetTitle>
                <Badge variant={getStatusBadgeVariant(host.status)} className='gap-1.5 py-0.5 px-2.5 font-normal'>
                  {host.status === HostStatus.CONNECTED && (
                    <span className='relative flex h-2 w-2'>
                      <span className='animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75'></span>
                      <span className='relative inline-flex rounded-full h-2 w-2 bg-emerald-500'></span>
                    </span>
                  )}
                  {t(`host.statuses.${host.status}`)}
                </Badge>
                <Badge variant='outline' className='text-xs font-mono text-muted-foreground'>
                  {t(`host.types.${host.host_type === HostType.BARE_METAL ? 'bareMetal' : host.host_type}`)}
                </Badge>
              </div>
              <SheetDescription className='text-xs text-muted-foreground line-clamp-2'>
                {host.description || t('host.noDescription')}
              </SheetDescription>
            </div>
          </div>
        </SheetHeader>

        {/* Scrollable Body / 可滚动内容区 */}
        <div
          ref={contentContainerRef}
          className='flex-1 overflow-y-auto p-6 space-y-5'
        >
          {/* Section 1: Resource Usage / 资源使用率监控 */}
          <div className='host-detail-section border border-border/70 rounded-xl bg-card/60 p-4 space-y-4 shadow-xs hover:border-primary/20 transition-all'>
            <div className='flex items-center justify-between'>
              <div className='flex items-center gap-2 text-sm font-medium'>
                <Activity className='h-4 w-4 text-primary' />
                <span>{t('host.resources')}</span>
              </div>
              {host.last_check && (
                <div className='flex items-center gap-1.5 text-xs text-muted-foreground'>
                  <Clock className='h-3 w-3' />
                  <span>{t('host.lastCheck')}: {formatDateTime(host.last_check)}</span>
                </div>
              )}
            </div>

            <div className='grid grid-cols-1 md:grid-cols-3 gap-3.5 pt-1'>
              {/* CPU Usage / CPU使用率 */}
              <div className='bg-background/80 border border-border/60 rounded-lg p-3 space-y-2'>
                <div className='flex items-center justify-between text-xs'>
                  <span className='flex items-center gap-1.5 text-muted-foreground'>
                    <Cpu className='h-3.5 w-3.5 text-blue-500' />
                    CPU
                  </span>
                  <span className='font-mono font-medium'>{host.cpu_usage?.toFixed(1) || 0}%</span>
                </div>
                <Progress value={host.cpu_usage || 0} className='h-1.5' />
                <div className='text-[11px] text-muted-foreground truncate'>
                  {host.cpu_cores ? `${host.cpu_cores} ${t('host.cores')}` : '-'}
                </div>
              </div>

              {/* Memory Usage / 内存使用率 */}
              <div className='bg-background/80 border border-border/60 rounded-lg p-3 space-y-2'>
                <div className='flex items-center justify-between text-xs'>
                  <span className='flex items-center gap-1.5 text-muted-foreground'>
                    <MemoryStick className='h-3.5 w-3.5 text-emerald-500' />
                    {t('host.memory')}
                  </span>
                  <span className='font-mono font-medium'>{host.memory_usage?.toFixed(1) || 0}%</span>
                </div>
                <Progress value={host.memory_usage || 0} className='h-1.5' />
                <div className='text-[11px] text-muted-foreground truncate'>
                  {host.total_memory ? `${formatBytes(host.total_memory)} ${t('host.total')}` : '-'}
                </div>
              </div>

              {/* Disk Usage / 磁盘使用率 */}
              <div className='bg-background/80 border border-border/60 rounded-lg p-3 space-y-2'>
                <div className='flex items-center justify-between text-xs'>
                  <span className='flex items-center gap-1.5 text-muted-foreground'>
                    <HardDrive className='h-3.5 w-3.5 text-purple-500' />
                    {t('host.disk')}
                  </span>
                  <span className='font-mono font-medium'>{host.disk_usage?.toFixed(1) || 0}%</span>
                </div>
                <Progress value={host.disk_usage || 0} className='h-1.5' />
                <div className='text-[11px] text-muted-foreground truncate'>
                  {host.total_disk ? `${formatBytes(host.total_disk)} ${t('host.total')}` : '-'}
                </div>
              </div>
            </div>
          </div>

          {/* Section 2: Host Overview & Agent Info / 主机概览与Agent信息 */}
          <div className='host-detail-section border border-border/70 rounded-xl bg-card/60 p-4 space-y-3.5 shadow-xs hover:border-primary/20 transition-all'>
            <div className='flex items-center gap-2 text-sm font-medium'>
              <Layers className='h-4 w-4 text-primary' />
              <span>{t('host.basicInfo')}</span>
            </div>

            <div className='grid grid-cols-2 gap-x-6 gap-y-3 text-xs'>
              <div className='flex justify-between py-1 border-b border-border/40'>
                <span className='text-muted-foreground'>{t('host.hostType')}</span>
                <span className='font-medium'>
                  {t(`host.types.${host.host_type === HostType.BARE_METAL ? 'bareMetal' : host.host_type}`)}
                </span>
              </div>
              <div className='flex justify-between py-1 border-b border-border/40'>
                <span className='text-muted-foreground'>{t('host.status')}</span>
                <span className='font-medium'>{t(`host.statuses.${host.status}`)}</span>
              </div>
              <div className='flex justify-between py-1 border-b border-border/40'>
                <span className='text-muted-foreground'>{t('host.createdAt')}</span>
                <span className='font-mono text-muted-foreground'>{formatDateTime(host.created_at)}</span>
              </div>
              <div className='flex justify-between py-1 border-b border-border/40'>
                <span className='text-muted-foreground'>{t('host.updatedAt')}</span>
                <span className='font-mono text-muted-foreground'>{formatDateTime(host.updated_at)}</span>
              </div>
            </div>
          </div>

          {/* Section 3: Connection & Environment Info / 连接与环境详情 */}
          <div className='host-detail-section border border-border/70 rounded-xl bg-card/60 p-4 space-y-3.5 shadow-xs hover:border-primary/20 transition-all'>
            <div className='flex items-center gap-2 text-sm font-medium'>
              <Network className='h-4 w-4 text-primary' />
              <span>{t('host.connectionInfo')}</span>
            </div>

            {host.host_type === HostType.BARE_METAL && (
              <div className='grid grid-cols-2 gap-x-6 gap-y-3 text-xs'>
                <div className='flex justify-between py-1 border-b border-border/40'>
                  <span className='text-muted-foreground'>{t('host.ipAddress')}</span>
                  <span className='font-mono font-medium'>{host.ip_address || '-'}</span>
                </div>
                <div className='flex justify-between py-1 border-b border-border/40'>
                  <span className='text-muted-foreground'>{t('host.agentVersion')}</span>
                  <span className='font-mono'>{host.agent_version || '-'}</span>
                </div>
                <div className='flex justify-between py-1 border-b border-border/40'>
                  <span className='text-muted-foreground'>{t('host.osType')}</span>
                  <span>{host.os_type ? `${host.os_type} (${host.arch || 'x86_64'})` : '-'}</span>
                </div>
                <div className='flex justify-between py-1 border-b border-border/40'>
                  <span className='text-muted-foreground'>{t('host.lastHeartbeat')}</span>
                  <span className='font-mono text-muted-foreground'>{formatDateTime(host.last_heartbeat)}</span>
                </div>
              </div>
            )}

            {host.host_type === HostType.DOCKER && (
              <div className='grid grid-cols-2 gap-x-6 gap-y-3 text-xs'>
                <div className='flex justify-between py-1 border-b border-border/40 col-span-2'>
                  <span className='text-muted-foreground'>{t('host.dockerApiUrl')}</span>
                  <span className='font-mono font-medium truncate max-w-[320px]'>{host.docker_api_url || '-'}</span>
                </div>
                <div className='flex justify-between py-1 border-b border-border/40'>
                  <span className='text-muted-foreground'>{t('host.tlsEnabled')}</span>
                  <span>{host.docker_tls_enabled ? t('common.yes') : t('common.no')}</span>
                </div>
                <div className='flex justify-between py-1 border-b border-border/40'>
                  <span className='text-muted-foreground'>{t('host.dockerVersion')}</span>
                  <span className='font-mono'>{host.docker_version || '-'}</span>
                </div>
              </div>
            )}

            {host.host_type === HostType.KUBERNETES && (
              <div className='grid grid-cols-2 gap-x-6 gap-y-3 text-xs'>
                <div className='flex justify-between py-1 border-b border-border/40 col-span-2'>
                  <span className='text-muted-foreground'>{t('host.k8sApiUrl')}</span>
                  <span className='font-mono font-medium truncate max-w-[320px]'>{host.k8s_api_url || '-'}</span>
                </div>
                <div className='flex justify-between py-1 border-b border-border/40'>
                  <span className='text-muted-foreground'>{t('host.namespace')}</span>
                  <span className='font-mono'>{host.k8s_namespace || 'default'}</span>
                </div>
                <div className='flex justify-between py-1 border-b border-border/40'>
                  <span className='text-muted-foreground'>{t('host.k8sVersion')}</span>
                  <span className='font-mono'>{host.k8s_version || '-'}</span>
                </div>
              </div>
            )}
          </div>

          {/* Section 4: Terminal Commands (for Bare Metal) / 拟物化终端命令（物理机） */}
          {host.host_type === HostType.BARE_METAL && (
            <div className='host-detail-section border border-border/70 rounded-xl bg-card/60 p-4 space-y-4 shadow-xs'>
              <div className='flex items-center gap-2 text-sm font-medium'>
                <Terminal className='h-4 w-4 text-primary' />
                <span>{t('host.installCommand')} / {t('host.uninstallCommand')}</span>
              </div>

              {/* 待连接主机的醒目指引提示 / Prominent guidance alert for pending host */}
              {host.status === HostStatus.PENDING && (
                <div className='p-3 rounded-lg border border-amber-500/30 bg-amber-500/10 text-xs text-amber-700 dark:text-amber-300 flex items-start gap-2.5'>
                  <span className='h-2 w-2 rounded-full bg-amber-500 animate-pulse mt-1 shrink-0' />
                  <div className='space-y-0.5'>
                    <span className='font-semibold block'>
                      {t('host.installGuide.title')}
                    </span>
                    <span className='text-[11px] text-muted-foreground block'>
                      {t('host.installGuide.pendingAlert')}
                    </span>
                  </div>
                </div>
              )}

              {/* Install Command Terminal / 安装脚本终端框 */}
              <div className='space-y-1.5'>
                <div className='text-xs font-medium text-muted-foreground flex items-center justify-between'>
                  <span>{t('host.installCommand')}</span>
                  <span className='text-[11px] text-muted-foreground/80 font-mono'>bash</span>
                </div>

                <div className='rounded-lg border border-zinc-800 bg-zinc-950 overflow-hidden shadow-sm'>
                  {/* Terminal Header Bar / 终端顶栏 */}
                  <div className='flex items-center justify-between px-3 py-1.5 bg-zinc-900/90 border-b border-zinc-800'>
                    <div className='flex items-center gap-1.5'>
                      <span className='h-2.5 w-2.5 rounded-full bg-red-500/80 inline-block'></span>
                      <span className='h-2.5 w-2.5 rounded-full bg-yellow-500/80 inline-block'></span>
                      <span className='h-2.5 w-2.5 rounded-full bg-emerald-500/80 inline-block'></span>
                      <span className='ml-2 text-[11px] font-mono text-zinc-400'>seatunnel-agent-install</span>
                    </div>
                    {installCommand && (
                      <Button
                        variant='ghost'
                        size='sm'
                        className='h-6 px-2 text-[11px] text-zinc-300 hover:text-white hover:bg-zinc-800 gap-1'
                        onClick={() => handleCopyCommand(installCommand, false)}
                      >
                        {copiedInstall ? (
                          <>
                            <Check className='h-3 w-3 text-emerald-400' />
                            <span className='text-emerald-400'>{t('common.copied')}</span>
                          </>
                        ) : (
                          <>
                            <Copy className='h-3 w-3' />
                            <span>{t('common.copy')}</span>
                          </>
                        )}
                      </Button>
                    )}
                  </div>

                  {/* Terminal Content / 终端代码行 */}
                  <div className='p-3 font-mono text-xs text-emerald-400 overflow-x-auto leading-relaxed select-all whitespace-pre-wrap break-all'>
                    {loadingCommand ? (
                      <span className='text-zinc-500'>{t('common.loading')}...</span>
                    ) : installCommand ? (
                      installCommand
                    ) : (
                      <span className='text-zinc-500'>{t('host.noInstallCommand')}</span>
                    )}
                  </div>
                </div>
              </div>

              {/* Uninstall Command Terminal / 卸载脚本终端框 */}
              {installCommand && (
                <div className='space-y-1.5 pt-1'>
                  <div className='text-xs font-medium text-muted-foreground flex items-center justify-between'>
                    <span>{t('host.uninstallCommand')}</span>
                    <span className='text-[11px] text-muted-foreground/80 font-mono'>bash</span>
                  </div>

                  <div className='rounded-lg border border-zinc-800 bg-zinc-950 overflow-hidden shadow-sm'>
                    <div className='flex items-center justify-between px-3 py-1.5 bg-zinc-900/90 border-b border-zinc-800'>
                      <div className='flex items-center gap-1.5'>
                        <span className='h-2.5 w-2.5 rounded-full bg-red-500/80 inline-block'></span>
                        <span className='h-2.5 w-2.5 rounded-full bg-yellow-500/80 inline-block'></span>
                        <span className='h-2.5 w-2.5 rounded-full bg-emerald-500/80 inline-block'></span>
                        <span className='ml-2 text-[11px] font-mono text-zinc-400'>seatunnel-agent-uninstall</span>
                      </div>
                      <Button
                        variant='ghost'
                        size='sm'
                        className='h-6 px-2 text-[11px] text-zinc-300 hover:text-white hover:bg-zinc-800 gap-1'
                        onClick={() =>
                          handleCopyCommand(
                            installCommand.replace('/install.sh', '/uninstall.sh'),
                            true,
                          )
                        }
                      >
                        {copiedUninstall ? (
                          <>
                            <Check className='h-3 w-3 text-emerald-400' />
                            <span className='text-emerald-400'>{t('common.copied')}</span>
                          </>
                        ) : (
                          <>
                            <Copy className='h-3 w-3' />
                            <span>{t('common.copy')}</span>
                          </>
                        )}
                      </Button>
                    </div>

                    <div className='p-3 font-mono text-xs text-amber-300/90 overflow-x-auto leading-relaxed select-all whitespace-pre-wrap break-all'>
                      {installCommand.replace('/install.sh', '/uninstall.sh')}
                    </div>
                  </div>

                  <p className='text-[11px] text-muted-foreground'>
                    {t('host.uninstallCommandTip')}
                  </p>
                </div>
              )}
            </div>
          )}
        </div>

        {/* Sticky Footer / 底部操作栏 */}
        <div className='p-4 border-t border-border/70 bg-muted/20 backdrop-blur-xs flex items-center justify-between'>
          <Button variant='outline' size='sm' onClick={() => onOpenChange(false)}>
            {t('common.close')}
          </Button>
          <Button size='sm' onClick={onEdit} className='gap-1.5 active:scale-[0.98]'>
            <Pencil className='h-3.5 w-3.5' />
            {t('common.edit')}
          </Button>
        </div>
      </SheetContent>
    </Sheet>
  );
}

