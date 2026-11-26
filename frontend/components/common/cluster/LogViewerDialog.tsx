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

import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type KeyboardEvent,
  type ReactNode,
} from 'react';
import {useTranslations} from 'next-intl';
import {RefreshCw} from 'lucide-react';
import {CopyButton} from '@/components/arc/copy-button/copy-button';
import {Button} from '@/components/arc/button/button';
import {Dialog, DialogContent} from '@/components/arc/dialog/dialog';
import {Input} from '@/components/arc/input/input';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {TableLoadingBar} from '@/components/common/layout';
import {toast} from '@/lib/toast';

export interface LogFetchParams {
  lines: number;
  mode: string;
  filter?: string;
  date?: string;
}

export interface LogFetchResult {
  logs?: string;
  error?: string;
}

export interface LogViewerCapabilities {
  /** 文件日志才有 tail/head/all 分块语义；kubectl logs 只有 tail。File logs have tail/head/all chunk semantics; kubectl logs only has tail. */
  modes?: boolean;
  /** 服务端或客户端 grep 过滤。Server- or client-side grep filtering. */
  filter?: boolean;
  /** 按日志文件日期读取（仅文件日志）。Read by log-file date (file logs only). */
  date?: boolean;
}

interface LogViewerDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** 标题区内容（调用方拼名称 chip）。Title area content (caller composes name chips). */
  title: ReactNode;
  description?: ReactNode;
  /** 数据获取器：日志呈现与基础设施解耦的唯一接缝。The single seam decoupling log presentation from infrastructure. */
  fetcher: (params: LogFetchParams) => Promise<LogFetchResult>;
  capabilities?: LogViewerCapabilities;
}

const ALL_CAPABILITIES: Required<LogViewerCapabilities> = {
  modes: true,
  filter: true,
  date: true,
};

/**
 * 基础设施无关的日志查看弹窗：呈现层（模式/行数/过滤/复制/刷新/加载条）共享，
 * 数据获取由调用方注入——物理机走 Agent 文件日志，K8s 走 kubectl logs。
 * Infrastructure-agnostic log viewer dialog: the presentation layer (mode/lines/filter/
 * copy/refresh/loading bar) is shared while data acquisition is injected by the caller —
 * bare metal reads file logs through the Agent, K8s streams kubectl logs.
 */
export function LogViewerDialog({
  open,
  onOpenChange,
  title,
  description,
  fetcher,
  capabilities,
}: LogViewerDialogProps) {
  const t = useTranslations();
  const caps = {...ALL_CAPABILITIES, ...capabilities};

  const [logLines, setLogLines] = useState<number>(200);
  const [logMode, setLogMode] = useState<string>('tail');
  const [logFilter, setLogFilter] = useState<string>('');
  const [logDate, setLogDate] = useState<string>('');

  const [logContent, setLogContent] = useState<string>('');
  const [logLoading, setLogLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const reqSeqRef = useRef(0);

  const fetchLogs = useCallback(async () => {
    const seq = ++reqSeqRef.current;
    setLogLoading(true);
    setError(null);
    try {
      const result = await fetcher({
        lines: logLines,
        mode: logMode,
        filter: logFilter || undefined,
        date: logDate || undefined,
      });
      if (seq !== reqSeqRef.current) {
        return;
      }
      if (result.error) {
        setError(result.error);
      } else {
        setLogContent(result.logs || '');
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
  }, [fetcher, logDate, logFilter, logLines, logMode, t]);

  useEffect(() => {
    if (open) {
      void fetchLogs();
    } else {
      setLogContent('');
      setError(null);
    }
  }, [open, fetchLogs]);

  const handleKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter') {
      e.preventDefault();
      void fetchLogs();
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        title={title}
        description={description}
        className='arc-compact flex! h-[min(86vh,760px)]! w-[min(94vw,1040px)]! sm:max-w-none! flex-col! overflow-hidden!'
        bodyClassName='flex min-h-0 flex-1 flex-col gap-0 overflow-hidden p-0!'
      >
        {/* 查询控制条（单行 0.5px 细线分隔）/ Filter toolbar separated by 0.5px hairline */}
        <div className='flex flex-wrap items-end justify-between gap-2 border-b-[0.5px] border-border/80 px-5 py-3'>
          <div className='flex flex-wrap items-end gap-2'>
            {caps.modes ? (
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
            ) : null}
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
            {caps.filter ? (
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
            ) : null}
            {caps.date ? (
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
            ) : null}
          </div>
          <div className='flex shrink-0 items-center gap-1.5'>
            <CopyButton
              value={logContent}
              label={t('cluster.copyLog')}
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
