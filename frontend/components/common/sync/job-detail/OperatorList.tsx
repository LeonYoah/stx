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

import {useMemo, useState} from 'react';
import {ArrowDown, ArrowUp} from 'lucide-react';
import {useTranslations} from 'next-intl';
import {Table, TableBody, TableCell, TableHead, TableHeader, TableRow} from '@/components/ui/table';
import {cn} from '@/lib/utils';
import {
  backpressureTone,
  formatBytes,
  formatCount,
  formatPercent,
  type DagVertex,
  type MetricIndex,
  type RealtimeOverlay,
  type TableStats,
} from './dag-model';
import {OperatorIcon} from './operator-style';
import type {DagMode} from './JobDagCanvas';

type SortKey = 'vertexId' | 'tables' | 'readCount' | 'readBytes' | 'writeCount' | 'writeBytes' | 'qps' | 'busy' | 'backpressured';

interface OperatorRow {
  vertex: DagVertex;
  stats?: TableStats;
  qps?: number;
  busy?: number;
  backpressured?: number;
}

function sortValue(row: OperatorRow, key: SortKey): number {
  switch (key) {
    case 'vertexId':
      return row.vertex.vertexId;
    case 'tables':
      return row.vertex.tablePaths.length;
    case 'qps':
      return row.qps ?? -1;
    case 'busy':
      return row.busy ?? -1;
    case 'backpressured':
      return row.backpressured ?? -1;
    default:
      return row.stats?.[key] ?? -1;
  }
}

/**
 * 算子列表（对照 Flink job-overview-list）：各列可排序，点行与点节点等价；SeaTunnel 没有的子任务 / 并行度 / 每算子时间不展示。
 * Operator list (modeled on Flink's job-overview-list): sortable columns and row clicks equal node clicks; subtasks,
 * per-operator parallelism and timings that SeaTunnel lacks are omitted.
 */
export function OperatorList({
  vertices,
  mode,
  metricIndex,
  overlay,
  selectedVertexId,
  onSelectVertex,
}: {
  vertices: DagVertex[];
  mode: DagMode;
  metricIndex?: MetricIndex;
  overlay?: RealtimeOverlay;
  selectedVertexId: number | null;
  onSelectVertex: (vertexId: number | null) => void;
}) {
  const t = useTranslations('workbenchStudio.jobDetail');
  const [sort, setSort] = useState<{key: SortKey; desc: boolean}>({key: 'vertexId', desc: false});

  const rows = useMemo(() => {
    const list: OperatorRow[] = vertices.map((vertex) => {
      const stats = vertex.metricPrefix ? metricIndex?.vertices.get(vertex.metricPrefix) : undefined;
      return {
        vertex,
        stats,
        qps: vertex.type === 'sink' ? stats?.writeQps : stats?.readQps,
        busy: overlay?.busy.get(vertex.vertexId),
        backpressured: overlay?.backpressured.get(vertex.vertexId),
      };
    });
    return list.sort((left, right) => {
      const diff = sortValue(left, sort.key) - sortValue(right, sort.key);
      return sort.desc ? -diff : diff;
    });
  }, [vertices, metricIndex, overlay, sort]);

  const runtime = mode === 'runtime';
  const header = (key: SortKey, label: string, className?: string) => (
    <TableHead className={cn('h-8 whitespace-nowrap px-2 text-xs', className)}>
      <button
        type='button'
        className='inline-flex items-center gap-0.5 hover:text-foreground'
        onClick={() => setSort((prev) => ({key, desc: prev.key === key ? !prev.desc : key !== 'vertexId'}))}
      >
        {label}
        {sort.key === key ? sort.desc ? <ArrowDown className='size-3' /> : <ArrowUp className='size-3' /> : null}
      </button>
    </TableHead>
  );

  return (
    <Table className='text-xs' data-testid='dag-operator-list'>
      <TableHeader className='sticky top-0 z-10 bg-background'>
        <TableRow className='hover:bg-transparent'>
          {header('vertexId', t('col.name'), 'sticky left-0 z-10 bg-background min-w-[240px]')}
          <TableHead className='h-8 whitespace-nowrap px-2 text-xs'>{t('col.type')}</TableHead>
          {header('tables', t('col.tables'))}
          {runtime ? (
            <>
              {header('readCount', t('col.readCount'))}
              {header('readBytes', t('col.readBytes'))}
              {header('writeCount', t('col.writeCount'))}
              {header('writeBytes', t('col.writeBytes'))}
              {header('qps', t('col.qps'))}
              {header('busy', t('col.busy'))}
              {header('backpressured', t('col.backpressured'))}
            </>
          ) : null}
        </TableRow>
      </TableHeader>
      <TableBody>
        {rows.map((row) => {
          const selected = row.vertex.vertexId === selectedVertexId;
          return (
            <TableRow
              key={row.vertex.vertexId}
              className={cn('cursor-pointer', selected && 'bg-primary/5')}
              onClick={() => onSelectVertex(selected ? null : row.vertex.vertexId)}
              data-state={selected ? 'selected' : undefined}
            >
              <TableCell className={cn('sticky left-0 max-w-[320px] bg-background px-2 py-1.5', selected && 'bg-primary/5')}>
                <div className='flex min-w-0 items-center gap-1.5'>
                  <span className='font-mono text-[10px] text-muted-foreground'>#{row.vertex.vertexId}</span>
                  <span className='truncate font-medium' title={row.vertex.rawName}>
                    {row.vertex.displayName}
                  </span>
                </div>
              </TableCell>
              <TableCell className='whitespace-nowrap px-2 py-1.5'>
                <span className='inline-flex items-center gap-1 uppercase text-muted-foreground'>
                  <OperatorIcon type={row.vertex.type} className='size-3' />
                  {row.vertex.type}
                </span>
              </TableCell>
              <TableCell className='px-2 py-1.5 font-mono'>{row.vertex.tablePaths.length}</TableCell>
              {runtime ? (
                <>
                  <TableCell className='px-2 py-1.5 font-mono'>{formatCount(row.stats?.readCount)}</TableCell>
                  <TableCell className='px-2 py-1.5 font-mono'>{formatBytes(row.stats?.readBytes)}</TableCell>
                  <TableCell className='px-2 py-1.5 font-mono'>{formatCount(row.stats?.writeCount)}</TableCell>
                  <TableCell className='px-2 py-1.5 font-mono'>{formatBytes(row.stats?.writeBytes)}</TableCell>
                  <TableCell className='px-2 py-1.5 font-mono'>
                    {typeof row.qps === 'number' ? `${formatCount(row.qps)}/s` : '-'}
                  </TableCell>
                  <TableCell className='px-2 py-1.5 font-mono'>{formatPercent(row.busy)}</TableCell>
                  <TableCell className={cn('px-2 py-1.5 font-mono', backpressureTone(row.backpressured ?? 0))}>
                    {formatPercent(row.backpressured)}
                  </TableCell>
                </>
              ) : null}
            </TableRow>
          );
        })}
      </TableBody>
    </Table>
  );
}
