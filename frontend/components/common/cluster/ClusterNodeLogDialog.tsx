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
 * 集群节点日志查看弹窗：Arc Dialog（.arc-compact）+ 单层扁平 0.5px 细线 +
 * SegmentedControl 切 tail/head/all + 原生双向 overflow-auto（禁用 ScrollArea 与 break-all）+
 * TableLoadingBar 静默刷新与请求竞态保护。
 * Cluster node log viewer dialog: Arc Dialog (.arc-compact) + flat 0.5px hairlines +
 * SegmentedControl mode switch + native overflow-auto (no ScrollArea/break-all) + race guard.
 */
import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type KeyboardEvent,
} from 'react';
import {useTranslations} from 'next-intl';
import {FileText, RefreshCw} from 'lucide-react';
import {Button} from '@/components/arc/button/button';
import {Dialog, DialogContent} from '@/components/arc/dialog/dialog';
import {Input} from '@/components/arc/input/input';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {NeutralChip, TableLoadingBar} from '@/components/common/layout';
import services from '@/lib/services';
import type {NodeInfo} from '@/lib/services/cluster/types';
import {CopyButton} from '@/components/arc/copy-button/copy-button';

interface ClusterNodeLogDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  node: NodeInfo | null;
  clusterId: number;
}

export function ClusterNodeLogDialog({
  open,
  onOpenChange,
  node,
  clusterId,
}: ClusterNodeLogDialogProps) {
  const t = useTranslations();

  const [logLines, setLogLines] = useState<number>(200);
  const [logMode, setLogMode] = useState<string>('tail');
  const [logFilter, setLogFilter] = useState<string>('');
  const [logDate, setLogDate] = useState<string>('');

  const [logContent, setLogContent] = useState<string>('');
  const [logLoading, setLogLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const reqSeqRef = useRef(0);

  const fetchLogs = useCallback(async () => {
    if (!node) {
      return;
    }
    const seq = ++reqSeqRef.current;
    setLogLoading(true);
    setError(null);
    try {
      const result = await services.cluster.getNodeLogsSafe(
        clusterId,
        node.id,
        {
          lines: logLines,
          mode: logMode,
          filter: logFilter || undefined,
          date: logDate || undefined,
        },
      );
      if (seq !== reqSeqRef.current) {
        return;
      }
      if (result.success && result.data) {
        setLogContent(result.data.logs || '');
      } else {
        setError(result.error || t('cluster.getLogsError'));
      }
    } catch (err) {
      if (seq !== reqSeqRef.current) {
        return;
      }
      setError(err instanceof Error ? err.message : t('cluster.getLogsError'));
    } finally {
      if (seq === reqSeqRef.current) {
        setLogLoading(false);
      }
    }
  }, [clusterId, logDate, logFilter, logLines, logMode, node, t]);

  useEffect(() => {
    if (open && node) {
      void fetchLogs();
    } else {
      setLogContent('');
      setError(null);
    }
  }, [open, node, fetchLogs]);

  const handleKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter') {
      e.preventDefault();
      void fetchLogs();
    }
  };

  const titleNode = (
    <span className='flex flex-wrap items-center gap-2'>
      <FileText className='size-4 shrink-0 text-primary' aria-hidden />
      <span>{t('cluster.viewLogs')}</span>
      {node ? (
        <NeutralChip className='font-mono tabular-nums'>
          {node.host_name || node.host_ip} (#{node.id})
        </NeutralChip>
      ) : null}
      {node?.role ? <NeutralChip>{node.role.toUpperCase()}</NeutralChip> : null}
    </span>
  );

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        title={titleNode}
        description={
          node?.install_dir ? (
            <span
              className='block truncate font-mono text-xs'
              title={`${node.install_dir}/logs`}
            >
              {node.install_dir}/logs
            </span>
          ) : undefined
        }
        className='arc-compact flex! h-[min(86vh,760px)]! w-[min(94vw,1040px)]! sm:max-w-none! flex-col! overflow-hidden!'
        bodyClassName='flex min-h-0 flex-1 flex-col gap-0 overflow-hidden p-0!'
      >
        {/* 查询控制条（单行 0.5px 细线分隔）/ Filter toolbar separated by 0.5px hairline */}
        <div className='flex flex-wrap items-end justify-between gap-2 border-b-[0.5px] border-border/80 px-5 py-3'>
          <div className='flex flex-wrap items-end gap-2'>
            <SegmentedControl
              label={t('cluster.logMode')}
              value={logMode}
              onValueChange={setLogMode}
              options={[
                {value: 'tail', label: t('cluster.logModeTail')},
                {value: 'head', label: t('cluster.logModeHead')},
                {value: 'all', label: t('cluster.logModeAll')},
              ]}
            />
            <div className='w-24'>
              <Input
                type='number'
                label={t('cluster.logLines')}
                value={String(logLines)}
                onChange={(e) => setLogLines(Number(e.target.value) || 100)}
                onKeyDown={handleKeyDown}
                min={10}
                max={10000}
                className='tabular-nums'
              />
            </div>
            <div className='w-36'>
              <Input
                type='text'
                label={t('cluster.logFilter')}
                value={logFilter}
                onChange={(e) => setLogFilter(e.target.value)}
                onKeyDown={handleKeyDown}
                placeholder='grep pattern'
              />
            </div>
            <div className='w-32'>
              <Input
                type='text'
                label={t('cluster.logDate')}
                value={logDate}
                onChange={(e) => setLogDate(e.target.value)}
                onKeyDown={handleKeyDown}
                placeholder='2026-11-22-1'
                className='font-mono tabular-nums'
              />
            </div>
          </div>

          <div className='flex shrink-0 items-center gap-1.5'>
            <CopyButton
              value={logContent}
              label={t('cluster.copyLog')}
              copiedLabel={t('common.copied')}
              failedLabel={t('common.copyFailed')}
              className='h-[var(--control-height-sm)]'
              disabled={!logContent}
            />
            <Button
              type='button'
              variant='secondary'
              size='sm'
              iconOnly
              aria-label={t('common.refresh')}
              title={t('common.refresh')}
              onClick={() => void fetchLogs()}
            >
              <RefreshCw
                className={`size-3.5 ${logLoading ? 'animate-spin' : ''}`}
                aria-hidden
              />
            </Button>
          </div>
        </div>

        {/* 顶部静默加载条，保留旧日志不遮灰 / Hairline loading bar preserving old logs */}
        <TableLoadingBar loading={logLoading} />

        {error ? (
          <div className='flex items-center justify-between gap-2 border-b-[0.5px] border-border/80 px-5 py-2 text-xs text-destructive'>
            <span className='min-w-0 truncate' title={error}>
              {error}
            </span>
            <Button
              type='button'
              variant='ghost'
              size='sm'
              onClick={() => void fetchLogs()}
            >
              {t('monitoring.retry')}
            </Button>
          </div>
        ) : null}

        {/* 原生双向滚动日志区，禁用 ScrollArea 与 break-all / Native bidirectional scroll, no ScrollArea or break-all */}
        <div className='min-h-0 flex-1 overflow-auto bg-muted/20 px-5 py-4'>
          <pre className='font-mono text-xs leading-relaxed whitespace-pre text-foreground/90 select-text'>
            {logContent || t('cluster.noLogs')}
          </pre>
        </div>

        <div className='flex shrink-0 items-center justify-end border-t-[0.5px] border-border/80 px-5 py-3'>
          <Button
            type='button'
            variant='secondary'
            size='sm'
            onClick={() => onOpenChange(false)}
          >
            {t('common.close')}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
