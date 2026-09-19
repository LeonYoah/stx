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
 * Cluster Node Log Viewer Dialog
 * 集群节点日志查看弹窗组件
 *
 * Modern dialog for inspecting real-time and historical node logs.
 * 提供现代化工作台级别的日志查看能力，支持过滤、行数选择与一键复制。
 */

import {useState, useEffect, useCallback, type KeyboardEvent} from 'react';
import {useTranslations} from 'next-intl';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {Button} from '@/components/ui/button';
import {Input} from '@/components/ui/input';
import {Badge} from '@/components/ui/badge';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import {ScrollArea} from '@/components/ui/scroll-area';
import {Loader2, RefreshCw, Copy, Check, FileText} from 'lucide-react';
import {toast} from 'sonner';
import services from '@/lib/services';
import type {NodeInfo} from '@/lib/services/cluster/types';

interface ClusterNodeLogDialogProps {
  // 弹窗可见性
  // Dialog visibility state
  open: boolean;
  // 弹窗开关控制
  // Dialog open state setter
  onOpenChange: (open: boolean) => void;
  // 当前查看日志的节点对象
  // Target node info for log inspection
  node: NodeInfo | null;
  // 集群标识
  // Cluster ID
  clusterId: number;
}

/**
 * Cluster Node Log Dialog
 * 集群节点日志弹窗
 */
export function ClusterNodeLogDialog({
  open,
  onOpenChange,
  node,
  clusterId,
}: ClusterNodeLogDialogProps) {
  const t = useTranslations();

  // 查询参数状态 / Query parameters state
  const [logLines, setLogLines] = useState<number>(200);
  const [logMode, setLogMode] = useState<string>('tail');
  const [logFilter, setLogFilter] = useState<string>('');
  const [logDate, setLogDate] = useState<string>('');

  // 数据与加载状态 / Data and loading state
  const [logContent, setLogContent] = useState<string>('');
  const [logLoading, setLogLoading] = useState(false);
  const [copied, setCopied] = useState(false);

  // 抓取节点日志 / Fetch node logs
  const fetchLogs = useCallback(async () => {
    if (!node) {
      return;
    }
    setLogLoading(true);
    try {
      const result = await services.cluster.getNodeLogsSafe(clusterId, node.id, {
        lines: logLines,
        mode: logMode,
        filter: logFilter || undefined,
        date: logDate || undefined,
      });

      if (result.success && result.data) {
        setLogContent(result.data.logs || t('cluster.noLogs'));
      } else {
        toast.error(result.error || t('cluster.getLogsError'));
      }
    } finally {
      setLogLoading(false);
    }
  }, [clusterId, node, logLines, logMode, logFilter, logDate, t]);

  // 打开弹窗时自动拉取日志 / Automatically load logs when dialog opens
  useEffect(() => {
    if (open && node) {
      void fetchLogs();
    } else {
      setLogContent('');
      setCopied(false);
    }
  }, [open, node, fetchLogs]);

  // 回车键快捷刷新 / Support Enter key to quickly trigger search
  const handleKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter') {
      e.preventDefault();
      void fetchLogs();
    }
  };

  // 一键复制日志 / Copy log contents to clipboard
  const handleCopy = async () => {
    if (!logContent) {
      return;
    }
    try {
      await navigator.clipboard.writeText(logContent);
      setCopied(true);
      toast.success(t('cluster.logCopied'));
      setTimeout(() => setCopied(false), 2000);
    } catch {
      toast.error('Copy failed');
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='sm:max-w-5xl h-[88vh] flex flex-col p-0 overflow-hidden border border-border/80 shadow-2xl'>
        {/* 弹窗头部 / Dialog Header */}
        <DialogHeader className='px-6 py-4 border-b bg-muted/15'>
          <div className='flex items-center justify-between gap-4'>
            <div className='space-y-1'>
              <DialogTitle className='flex items-center gap-2 text-base font-semibold'>
                <FileText className='size-4 text-primary' />
                <span>{t('cluster.viewLogs')}</span>
                {node && (
                  <Badge variant='outline' className='font-mono text-xs'>
                    {node.host_name || node.host_ip} (#{node.id})
                  </Badge>
                )}
                {node?.role && (
                  <Badge variant='secondary' className='text-xs'>
                    {node.role.toUpperCase()}
                  </Badge>
                )}
              </DialogTitle>
              <DialogDescription className='text-xs text-muted-foreground break-all'>
                {node?.install_dir ? `${node.install_dir}/logs` : '-'}
              </DialogDescription>
            </div>
            <div className='flex items-center gap-2'>
              <Button
                variant='outline'
                size='sm'
                onClick={handleCopy}
                disabled={!logContent || logLoading}
                className='h-8'
              >
                {copied ? (
                  <Check className='size-3.5 mr-1.5 text-emerald-500' />
                ) : (
                  <Copy className='size-3.5 mr-1.5' />
                )}
                {t('cluster.copyLog')}
              </Button>
              <Button
                variant='outline'
                size='sm'
                onClick={() => void fetchLogs()}
                disabled={logLoading}
                className='h-8'
              >
                <RefreshCw
                  className={`size-3.5 mr-1.5 ${logLoading ? 'animate-spin' : ''}`}
                />
                {t('common.refresh')}
              </Button>
            </div>
          </div>

          {/* 筛选参数控制条 / Filter query controls */}
          <div className='pt-3 flex flex-wrap items-center gap-2 text-xs'>
            {/* 行数 / Lines */}
            <div className='flex items-center gap-1.5'>
              <span className='text-muted-foreground whitespace-nowrap'>
                {t('cluster.logLines')}:
              </span>
              <Input
                type='number'
                value={logLines}
                onChange={(e) => setLogLines(Number(e.target.value) || 100)}
                onKeyDown={handleKeyDown}
                min={10}
                max={10000}
                className='h-8 w-20 text-xs'
              />
            </div>

            {/* 模式 / Mode */}
            <div className='flex items-center gap-1.5'>
              <span className='text-muted-foreground whitespace-nowrap'>
                {t('cluster.logMode')}:
              </span>
              <Select value={logMode} onValueChange={setLogMode}>
                <SelectTrigger className='h-8 w-24 text-xs'>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value='tail'>{t('cluster.logModeTail')}</SelectItem>
                  <SelectItem value='head'>{t('cluster.logModeHead')}</SelectItem>
                  <SelectItem value='all'>{t('cluster.logModeAll')}</SelectItem>
                </SelectContent>
              </Select>
            </div>

            {/* 关键词过滤 / Grep pattern */}
            <div className='flex items-center gap-1.5'>
              <span className='text-muted-foreground whitespace-nowrap'>
                {t('cluster.logFilter')}:
              </span>
              <Input
                type='text'
                value={logFilter}
                onChange={(e) => setLogFilter(e.target.value)}
                onKeyDown={handleKeyDown}
                placeholder='grep pattern'
                className='h-8 w-36 text-xs'
              />
            </div>

            {/* 日期后缀 / Date suffix */}
            <div className='flex items-center gap-1.5'>
              <span className='text-muted-foreground whitespace-nowrap'>
                {t('cluster.logDate')}:
              </span>
              <Input
                type='text'
                value={logDate}
                onChange={(e) => setLogDate(e.target.value)}
                onKeyDown={handleKeyDown}
                placeholder='2025-11-12-1'
                className='h-8 w-32 text-xs'
              />
            </div>
          </div>
        </DialogHeader>

        {/* 日志内容区域 / Log scroll content area */}
        <div className='flex-1 min-h-0 bg-background/50 relative'>
          {logLoading && (
            <div className='absolute inset-0 bg-background/60 z-10 flex items-center justify-center backdrop-blur-xs'>
              <Loader2 className='size-6 animate-spin text-primary' />
            </div>
          )}
          <ScrollArea className='h-full p-4'>
            <pre className='text-xs font-mono whitespace-pre-wrap break-all leading-relaxed select-text text-foreground/90'>
              {logContent || t('cluster.noLogs')}
            </pre>
          </ScrollArea>
        </div>

        {/* 弹窗底栏 / Dialog Footer */}
        <DialogFooter className='px-6 py-3 border-t bg-muted/10'>
          <Button variant='outline' size='sm' onClick={() => onOpenChange(false)}>
            {t('common.close')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
