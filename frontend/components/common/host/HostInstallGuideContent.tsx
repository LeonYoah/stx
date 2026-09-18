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
 * Host Install Guide Content Component
 * 主机 Agent 部署引导内容组件
 *
 * Provides a guided, step-by-step interactive onboarding experience
 * for newly created or pending bare-metal hosts to deploy and connect their Agent.
 * 为新建或待连接的物理机主机提供清晰的步骤化部署交互与实时心跳感应。
 */

import {useState, useEffect, useCallback, useRef} from 'react';
import {useTranslations} from 'next-intl';
import {Button} from '@/components/ui/button';
import {Badge} from '@/components/ui/badge';
import {toast} from 'sonner';
import {
  Copy,
  Check,
  Terminal,
  RefreshCw,
  Server,
  ShieldCheck,
  Sparkles,
  ExternalLink,
  Cpu,
  MemoryStick,
} from 'lucide-react';
import services from '@/lib/services';
import {HostInfo, HostStatus} from '@/lib/services/host/types';

interface HostInstallGuideContentProps {
  // 目标主机信息
  // Target host information
  host: HostInfo;
  // 当 Agent 成功接入并握手时的回调
  // Callback when Agent successfully connects and handshakes
  onConnected?: (updatedHost: HostInfo) => void;
  // 查看主机详情回调
  // Callback to navigate to host detail
  onViewDetail?: (host: HostInfo) => void;
  // 关闭引导对话框回调
  // Callback to close guide dialog
  onClose?: () => void;
}

/**
 * 格式化内存字节为人类易读单位
 * Format bytes to human readable string
 */
function formatBytes(bytes?: number): string {
  if (!bytes) {return '-';}
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let val = bytes;
  let idx = 0;
  while (val >= 1024 && idx < units.length - 1) {
    val /= 1024;
    idx++;
  }
  return `${val.toFixed(1)} ${units[idx]}`;
}

export function HostInstallGuideContent({
  host,
  onConnected,
  onViewDetail,
  onClose,
}: HostInstallGuideContentProps) {
  const t = useTranslations();
  const [installCommand, setInstallCommand] = useState<string>('');
  const [loadingCommand, setLoadingCommand] = useState<boolean>(true);
  const [copied, setCopied] = useState<boolean>(false);
  const [currentHost, setCurrentHost] = useState<HostInfo>(host);
  const [checkingHeartbeat, setCheckingHeartbeat] = useState<boolean>(false);
  const isMountedRef = useRef<boolean>(true);
  const pollingTimerRef = useRef<NodeJS.Timeout | null>(null);

  // 1. 加载主机的 Agent 一键安装命令
  // 1. Fetch Agent install command for the host
  const loadInstallCommand = useCallback(async () => {
    setLoadingCommand(true);
    try {
      const result = await services.host.getInstallCommandSafe(host.id);
      if (result.success && result.data && isMountedRef.current) {
        setInstallCommand(result.data.command);
      }
    } catch (err) {
      console.error('Failed to load install command:', err);
    } finally {
      if (isMountedRef.current) {
        setLoadingCommand(false);
      }
    }
  }, [host.id]);

  // 2. 检查主机最新心跳状态
  // 2. Check host heartbeat connection status
  const checkHeartbeat = useCallback(
    async (silent = false) => {
      if (!silent) {setCheckingHeartbeat(true);}
      try {
        const result = await services.host.getHostSafe(host.id);
        if (result.success && result.data && isMountedRef.current) {
          const updated = result.data;
          setCurrentHost(updated);
          if (
            updated.status === HostStatus.CONNECTED &&
            currentHost.status !== HostStatus.CONNECTED
          ) {
            toast.success(t('host.installGuide.connectedSuccess'));
            onConnected?.(updated);
          }
        }
      } catch (err) {
        console.error('Failed to check heartbeat:', err);
      } finally {
        if (!silent && isMountedRef.current) {
          setCheckingHeartbeat(false);
        }
      }
    },
    [currentHost.status, host.id, onConnected, t],
  );

  // 挂载时加载安装命令
  // Load command on mount
  useEffect(() => {
    isMountedRef.current = true;
    loadInstallCommand();
    return () => {
      isMountedRef.current = false;
    };
  }, [loadInstallCommand]);

  // 3. 实时轮询：当状态仍为未安装/待连接时，每 3 秒检测一次
  // 3. Polling: Auto-check every 3 seconds while pending
  useEffect(() => {
    if (currentHost.status === HostStatus.CONNECTED) {
      if (pollingTimerRef.current) {
        clearInterval(pollingTimerRef.current);
        pollingTimerRef.current = null;
      }
      return;
    }

    pollingTimerRef.current = setInterval(() => {
      void checkHeartbeat(true);
    }, 3000);

    return () => {
      if (pollingTimerRef.current) {
        clearInterval(pollingTimerRef.current);
        pollingTimerRef.current = null;
      }
    };
  }, [checkHeartbeat, currentHost.status]);

  // 复制命令到剪贴板
  // Copy command to clipboard
  const handleCopy = async () => {
    if (!installCommand) {return;}
    try {
      await navigator.clipboard.writeText(installCommand);
      setCopied(true);
      toast.success(t('host.commandCopied'));
      setTimeout(() => {
        if (isMountedRef.current) {
          setCopied(false);
        }
      }, 2000);
    } catch {
      toast.error(t('host.copyFailed'));
    }
  };

  const isConnected = currentHost.status === HostStatus.CONNECTED;

  return (
    <div className='space-y-4'>
      {/* 头部主机基本就绪条目 / Host Meta Summary Banner */}
      <div className='flex items-center justify-between p-3 rounded-lg border bg-muted/20 text-xs'>
        <div className='flex items-center gap-2'>
          <Server className='h-4 w-4 text-primary shrink-0' />
          <span className='font-medium text-foreground'>{currentHost.name}</span>
          <span className='text-muted-foreground font-mono'>
            ({currentHost.ip_address || '-'})
          </span>
        </div>
        <div className='flex items-center gap-1.5'>
          {isConnected ? (
            <Badge
              variant='default'
              className='bg-emerald-500/15 text-emerald-600 dark:text-emerald-400 border-emerald-500/30 gap-1 font-medium'
            >
              <span className='relative flex h-2 w-2'>
                <span className='animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75' />
                <span className='relative inline-flex rounded-full h-2 w-2 bg-emerald-500' />
              </span>
              {t('host.statuses.connected')}
            </Badge>
          ) : (
            <Badge
              variant='outline'
              className='border-amber-500/30 bg-amber-500/10 text-amber-600 dark:text-amber-400 gap-1'
            >
              <span className='h-1.5 w-1.5 rounded-full bg-amber-500 animate-pulse' />
              {t('host.statuses.pending')}
            </Badge>
          )}
        </div>
      </div>

      {/* 引导说明与三步指引 / Guided Instructions & 3 Steps */}
      <div className='rounded-lg border border-primary/20 bg-primary/5 p-3.5 space-y-2.5'>
        <div className='flex items-center gap-1.5 text-xs font-semibold text-primary'>
          <Sparkles className='h-3.5 w-3.5' />
          <span>{t('host.installGuide.stepsTitle')}</span>
        </div>
        <div className='grid grid-cols-1 md:grid-cols-3 gap-2 text-[11px] text-muted-foreground'>
          <div className='flex items-start gap-1.5 bg-background/60 p-2 rounded-md border border-border/60'>
            <span className='font-bold text-primary font-mono shrink-0'>1.</span>
            <span>
              {t('host.installGuide.step1Text', {
                ip: currentHost.ip_address || 'IP',
              })}
            </span>
          </div>
          <div className='flex items-start gap-1.5 bg-background/60 p-2 rounded-md border border-border/60'>
            <span className='font-bold text-primary font-mono shrink-0'>2.</span>
            <span>{t('host.installGuide.step2Text')}</span>
          </div>
          <div className='flex items-start gap-1.5 bg-background/60 p-2 rounded-md border border-border/60'>
            <span className='font-bold text-primary font-mono shrink-0'>3.</span>
            <span>{t('host.installGuide.step3Text')}</span>
          </div>
        </div>
      </div>

      {/* 终端安装脚本执行区 / Terminal Box with Quick Copy */}
      <div className='space-y-1.5'>
        <div className='flex items-center justify-between text-xs text-muted-foreground'>
          <span className='flex items-center gap-1 font-medium'>
            <Terminal className='h-3.5 w-3.5 text-primary' />
            {t('host.installGuide.terminalHint')}
          </span>
          <span className='text-[10px] font-mono text-muted-foreground/70'>
            bash (root / sudo)
          </span>
        </div>

        <div className='rounded-lg border border-zinc-800 bg-zinc-950 overflow-hidden shadow-xs'>
          {/* Terminal Titlebar */}
          <div className='flex items-center justify-between px-3 py-1.5 bg-zinc-900/90 border-b border-zinc-800'>
            <div className='flex items-center gap-1.5'>
              <span className='h-2.5 w-2.5 rounded-full bg-rose-500/80 inline-block' />
              <span className='h-2.5 w-2.5 rounded-full bg-amber-500/80 inline-block' />
              <span className='h-2.5 w-2.5 rounded-full bg-emerald-500/80 inline-block' />
              <span className='ml-2 text-[11px] font-mono text-zinc-400'>
                stx-agent-install.sh
              </span>
            </div>
            <Button
              variant='ghost'
              size='sm'
              disabled={loadingCommand || !installCommand}
              onClick={handleCopy}
              className='h-6 px-2 text-[11px] text-zinc-300 hover:text-white hover:bg-zinc-800 gap-1'
            >
              {copied ? (
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

          {/* Terminal Script Line */}
          <div className='p-3 font-mono text-xs text-emerald-400 overflow-x-auto leading-relaxed select-all whitespace-pre-wrap break-all'>
            {loadingCommand ? (
              <span className='text-zinc-500'>{t('common.loading')}...</span>
            ) : (
              installCommand || t('host.noInstallCommand')
            )}
          </div>
        </div>
      </div>

      {/* 实时心跳握手监听卡片 / Realtime Heartbeat Listener Card */}
      <div className='rounded-lg border p-3.5 transition-all bg-card/60 shadow-2xs'>
        {isConnected ? (
          /* 已成功连接状态展示 / Connected Success State */
          <div className='space-y-2.5'>
            <div className='flex items-center justify-between'>
              <div className='flex items-center gap-2 text-xs font-semibold text-emerald-600 dark:text-emerald-400'>
                <ShieldCheck className='h-4 w-4 shrink-0' />
                <span>{t('host.installGuide.connectedSuccess')}</span>
              </div>
              <Badge variant='outline' className='text-[10px] font-mono text-muted-foreground'>
                v{currentHost.agent_version || '1.0.0'}
              </Badge>
            </div>

            <div className='grid grid-cols-2 sm:grid-cols-4 gap-2 pt-1'>
              <div className='p-2 rounded bg-background/80 border text-xs'>
                <span className='text-muted-foreground block text-[10px]'>
                  {t('host.osType')}
                </span>
                <span className='font-medium truncate block'>
                  {currentHost.os_type || 'Linux'} ({currentHost.arch || 'x86_64'})
                </span>
              </div>
              <div className='p-2 rounded bg-background/80 border text-xs'>
                <span className='text-muted-foreground block text-[10px] flex items-center gap-1'>
                  <Cpu className='h-3 w-3 text-blue-500' />
                  CPU
                </span>
                <span className='font-medium font-mono block'>
                  {currentHost.cpu_cores ? `${currentHost.cpu_cores} ${t('host.cores')}` : '-'}
                </span>
              </div>
              <div className='p-2 rounded bg-background/80 border text-xs'>
                <span className='text-muted-foreground block text-[10px] flex items-center gap-1'>
                  <MemoryStick className='h-3 w-3 text-emerald-500' />
                  {t('host.memory')}
                </span>
                <span className='font-medium font-mono block'>
                  {formatBytes(currentHost.total_memory)}
                </span>
              </div>
              <div className='p-2 rounded bg-background/80 border text-xs'>
                <span className='text-muted-foreground block text-[10px]'>
                  {t('host.lastHeartbeat')}
                </span>
                <span className='font-medium font-mono text-[11px] truncate block'>
                  {currentHost.last_heartbeat
                    ? new Date(currentHost.last_heartbeat).toLocaleTimeString()
                    : '-'}
                </span>
              </div>
            </div>
          </div>
        ) : (
          /* 等待接入状态 / Waiting Heartbeat State */
          <div className='flex flex-col sm:flex-row items-center justify-between gap-3'>
            <div className='flex items-center gap-3 w-full sm:w-auto'>
              <div className='relative flex h-3 w-3 shrink-0'>
                <span className='animate-ping absolute inline-flex h-full w-full rounded-full bg-amber-400 opacity-75' />
                <span className='relative inline-flex rounded-full h-3 w-3 bg-amber-500' />
              </div>
              <div className='space-y-0.5'>
                <div className='text-xs font-medium text-foreground'>
                  {t('host.installGuide.waitingHeartbeat')}
                </div>
                <div className='text-[11px] text-muted-foreground'>
                  {t('host.installGuide.pollingHint')}
                </div>
              </div>
            </div>

            <Button
              type='button'
              variant='outline'
              size='sm'
              disabled={checkingHeartbeat}
              onClick={() => void checkHeartbeat(false)}
              className='h-7.5 px-2.5 text-xs shrink-0 w-full sm:w-auto'
            >
              <RefreshCw
                className={`h-3.5 w-3.5 mr-1.5 ${checkingHeartbeat ? 'animate-spin' : ''}`}
              />
              {t('host.installGuide.detectNow')}
            </Button>
          </div>
        )}
      </div>

      {/* 底部交互操作栏 / Bottom Actions */}
      <div className='flex items-center justify-between pt-2 border-t'>
        <Button
          type='button'
          variant='ghost'
          size='sm'
          onClick={onClose}
          className='h-8 text-xs text-muted-foreground hover:text-foreground'
        >
          {isConnected ? t('common.close') : t('host.installGuide.installLater')}
        </Button>

        <div className='flex items-center gap-2'>
          {onViewDetail && (
            <Button
              type='button'
              variant='outline'
              size='sm'
              onClick={() => onViewDetail(currentHost)}
              className='h-8 text-xs'
            >
              <ExternalLink className='h-3.5 w-3.5 mr-1.5' />
              {t('host.installGuide.viewHostDetail')}
            </Button>
          )}

          {isConnected ? (
            <Button
              type='button'
              size='sm'
              onClick={onClose}
              className='h-8 text-xs bg-emerald-600 hover:bg-emerald-500 text-white font-medium'
            >
              <Check className='h-3.5 w-3.5 mr-1.5' />
              {t('host.installGuide.done')}
            </Button>
          ) : (
            <Button
              type='button'
              size='sm'
              onClick={handleCopy}
              className='h-8 text-xs'
            >
              <Copy className='h-3.5 w-3.5 mr-1.5' />
              {t('host.installGuide.installAgent')}
            </Button>
          )}
        </div>
      </div>
    </div>
  );
}
