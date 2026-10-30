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

import {useEffect, useMemo, useState} from 'react';
import {ChevronsLeft, ChevronsRight, ChevronsUpDown, Copy, RefreshCw, X} from 'lucide-react';
import {useTranslations} from 'next-intl';
import {Badge} from '@/components/ui/badge';
import {Button} from '@/components/ui/button';
import {Command, CommandEmpty, CommandInput, CommandItem, CommandList} from '@/components/ui/command';
import {Input} from '@/components/ui/input';
import {Popover, PopoverContent, PopoverTrigger} from '@/components/ui/popover';
import {Table, TableBody, TableCell, TableHead, TableHeader, TableRow} from '@/components/ui/table';
import {Tabs, TabsContent, TabsList, TabsTrigger} from '@/components/ui/tabs';
import {HelpHint} from '@/components/common/HelpHint';
import {cn} from '@/lib/utils';
import type {SyncSinkSaveModePreviewTable} from '@/lib/services/sync';
import {
  backpressureTone,
  filterTables,
  formatBytes,
  formatCount,
  formatPercent,
  shortTablePath,
  type DagVertex,
  type MetricIndex,
  type RealtimeOverlay,
} from './dag-model';
import {copyText} from './copy-text';
import type {DagMode} from './JobDagCanvas';
import {OperatorIcon, operatorBadgeTone} from './operator-style';
import {resolveSaveModePlan, summarizeTableSchema} from './table-schema';

// 多表场景单次最多渲染的行数，超过时按需展开，避免几百张表一次性挂到 DOM。
// Maximum rows rendered at once for multi-table vertices; more are revealed on demand so hundreds of tables never
// mount at once.
const TABLE_RENDER_LIMIT = 200;

/** 计划 DAG 的加载状态，运行模式下由详情传入；plan 模式数据已在节点上。/ Plan DAG load state passed in runtime mode. */
export interface PlanDagStatus {
  loading: boolean;
  available: boolean;
  reason?: string;
  message?: string;
  error?: string | null;
  onRefresh?: () => void;
}

type DrawerTab = 'overview' | 'schema' | 'plan' | 'metrics';

function SaveModePlanView({preview}: {preview: SyncSinkSaveModePreviewTable}) {
  const t = useTranslations('workbenchStudio.jobDetail');
  const actions = preview.actions || [];
  const warnings = preview.warnings || [];
  return (
    <div className='space-y-3'>
      <div className='flex flex-wrap gap-2'>
        {preview.schemaSaveMode ? <Badge variant='outline'>Schema: {preview.schemaSaveMode}</Badge> : null}
        {preview.dataSaveMode ? <Badge variant='outline'>Data: {preview.dataSaveMode}</Badge> : null}
        {preview.completeness ? <Badge variant='secondary'>{preview.completeness}</Badge> : null}
      </div>
      {warnings.length > 0 ? (
        <ul className='space-y-1 text-xs text-amber-700 dark:text-amber-300'>
          {warnings.map((warning, index) => (
            <li key={`${warning}-${index}`}>{warning}</li>
          ))}
        </ul>
      ) : null}
      {actions.length > 0 ? (
        actions.map((action, index) => (
          <div key={`${action.actionType || 'action'}-${index}`} className='rounded-md border border-border/60 bg-muted/20 p-2'>
            <div className='flex flex-wrap items-center gap-2 text-[11px] text-muted-foreground'>
              {action.phase ? <span>{action.phase}</span> : null}
              {action.actionType ? <span>{action.actionType}</span> : null}
              {action.resultType ? <span>{action.resultType}</span> : null}
            </div>
            {action.content ? (
              <pre className='mt-1 overflow-x-auto whitespace-pre font-mono text-xs text-foreground'>{action.content}</pre>
            ) : null}
          </div>
        ))
      ) : warnings.length === 0 ? (
        <p className='text-xs text-muted-foreground'>{t('noPlanActions')}</p>
      ) : null}
    </div>
  );
}

function TablePicker({tables, value, onChange}: {tables: string[]; value: string; onChange: (table: string) => void}) {
  const t = useTranslations('workbenchStudio.jobDetail');
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState('');
  const matched = useMemo(() => filterTables(tables, query), [tables, query]);
  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button variant='outline' size='sm' className='h-8 w-full justify-between gap-2 font-mono text-xs'>
          <span className='min-w-0 truncate' title={value}>
            {value || t('pickTable')}
          </span>
          <ChevronsUpDown className='size-3.5 shrink-0 opacity-60' />
        </Button>
      </PopoverTrigger>
      <PopoverContent className='w-[var(--radix-popover-trigger-width)] p-0' align='start'>
        {/* 自己过滤并限量渲染，cmdk 自带过滤会把几百项都挂上。/ Filter and cap ourselves; cmdk filtering mounts every item. */}
        <Command shouldFilter={false}>
          <CommandInput value={query} onValueChange={setQuery} placeholder={t('searchTables')} />
          <CommandList>
            <CommandEmpty>{t('noMatchedTables')}</CommandEmpty>
            {matched.slice(0, TABLE_RENDER_LIMIT).map((table) => (
              <CommandItem
                key={table}
                value={table}
                onSelect={() => {
                  onChange(table);
                  setOpen(false);
                }}
                className='font-mono text-xs'
              >
                <span className='break-all'>{table}</span>
              </CommandItem>
            ))}
            {matched.length > TABLE_RENDER_LIMIT ? (
              <p className='px-2 py-1.5 text-[11px] text-muted-foreground'>
                {t('moreTablesHint', {shown: TABLE_RENDER_LIMIT, total: matched.length})}
              </p>
            ) : null}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}

/**
 * 节点抽屉（对照 Flink job-overview-drawer）：概览、表结构、建表计划（仅 Sink）、指标（运行模式），有关闭与铺满两个把手。
 * Node drawer (modeled on Flink's job-overview-drawer): overview, schema, save-mode plan (sinks only) and metrics
 * (runtime mode), with close and full-width handles.
 */
export function DagNodeDrawer({
  vertex,
  mode,
  metricIndex,
  overlay,
  plan,
  full,
  onToggleFull,
  onClose,
}: {
  vertex: DagVertex;
  mode: DagMode;
  metricIndex?: MetricIndex;
  overlay?: RealtimeOverlay;
  plan?: PlanDagStatus;
  full: boolean;
  onToggleFull: () => void;
  onClose: () => void;
}) {
  const t = useTranslations('workbenchStudio.jobDetail');
  const [tab, setTab] = useState<DrawerTab>('overview');
  const [query, setQuery] = useState('');
  const [limit, setLimit] = useState(TABLE_RENDER_LIMIT);
  const [table, setTable] = useState(vertex.tablePaths[0] ?? '');
  const [metricSort, setMetricSort] = useState<'count' | 'qps'>('count');

  // 只在换节点或表集合真正变化时重置；作业列表轮询会换掉 tablePaths 数组引用，按引用比较会把用户选的表和搜索词清掉。
  // Reset only when the vertex or its table set actually changes; job list polling replaces the tablePaths array, so a
  // reference comparison would wipe the user's selected table and search text.
  const tablesKey = vertex.tablePaths.join('\n');
  useEffect(() => {
    setTable(tablesKey.split('\n')[0] ?? '');
    setQuery('');
    setLimit(TABLE_RENDER_LIMIT);
  }, [vertex.vertexId, tablesKey]);

  const isSink = vertex.type === 'sink';
  const runtime = mode === 'runtime';
  // 运行态且计划 DAG 可用时，表结构来自数据库当前状态，需要提示；其余情况没有说明 / Only runtime with an available plan DAG needs the live-schema hint
  const schemaHint = runtime && plan?.available ? t('schemaLiveHint') : null;
  useEffect(() => {
    if ((tab === 'plan' && !isSink) || (tab === 'metrics' && !runtime)) {
      setTab('overview');
    }
  }, [tab, isSink, runtime]);

  const matchedTables = useMemo(() => filterTables(vertex.tablePaths, query), [vertex.tablePaths, query]);
  const schema = useMemo(() => summarizeTableSchema(vertex.tableSchemas?.[table]), [vertex.tableSchemas, table]);
  const columnNames = vertex.tableColumns?.[table] ?? [];
  const savePlan = table ? resolveSaveModePlan(vertex, table) : undefined;
  const busy = overlay?.busy.get(vertex.vertexId);
  const backpressured = overlay?.backpressured.get(vertex.vertexId);

  const tableMetrics = useMemo(() => {
    const byTable = vertex.metricPrefix ? metricIndex?.tables.get(vertex.metricPrefix) : undefined;
    const list = filterTables(vertex.tablePaths.length > 0 ? vertex.tablePaths : [...(byTable?.keys() ?? [])], query).map(
      (path) => {
        const stats = byTable?.get(path);
        return {
          path,
          count: isSink ? stats?.writeCount : stats?.readCount,
          bytes: isSink ? stats?.writeBytes : stats?.readBytes,
          qps: isSink ? stats?.writeQps : stats?.readQps,
          committed: stats?.committedCount,
        };
      },
    );
    return list.sort((left, right) =>
      metricSort === 'count' ? (right.count ?? -1) - (left.count ?? -1) : (right.qps ?? -1) - (left.qps ?? -1),
    );
  }, [vertex.metricPrefix, vertex.tablePaths, metricIndex, query, isSink, metricSort]);

  // 运行模式下表结构来自计划 DAG：加载中、不可用、没对应上都要说清楚，不报错。
  // In runtime mode schemas come from the plan DAG: loading, unavailable and unmatched states are explained, not errors.
  const planNotice = (() => {
    if (!runtime || !plan) {
      return null;
    }
    if (plan.loading) {
      return t('planLoading');
    }
    if (plan.error) {
      return t('planLoadFailed', {error: plan.error});
    }
    if (!plan.available) {
      // 原因码来自后端，先确认有文案再翻译，未知原因显示原值。
      // Reason codes come from the backend; translate only known ones and show unknown codes as-is.
      const key = `planReason.${plan.reason || 'inspect_failed'}`;
      const text = t.has(key) ? t(key) : plan.reason || '';
      return plan.message ? `${text}: ${plan.message}` : text;
    }
    if (vertex.planMatched === false) {
      return t('planUnmatched');
    }
    return null;
  })();

  const pickTable = (path: string) => {
    setTable(path);
    setTab('schema');
  };

  return (
    <div className='flex h-full min-h-0 flex-col border-l border-border/60 bg-background' data-testid='dag-node-drawer'>
      <div className='flex items-start gap-2 border-b border-border/60 px-3 py-2.5'>
        <OperatorIcon type={vertex.type} className='mt-0.5' />
        <div className='min-w-0 flex-1'>
          <div className='flex items-center gap-1.5'>
            <Badge variant='outline' className={cn('rounded-sm px-1 py-0 text-[10px] uppercase', operatorBadgeTone(vertex.type))}>
              {vertex.type}
            </Badge>
            <span className='font-mono text-[11px] text-muted-foreground'>#{vertex.vertexId}</span>
            {vertex.pipeline ? <span className='font-mono text-[11px] text-muted-foreground'>P{vertex.pipeline}</span> : null}
          </div>
          <div className='mt-1 break-all text-sm font-semibold'>{vertex.displayName}</div>
        </div>
        <Button variant='ghost' size='icon' className='size-7' onClick={onToggleFull} aria-label={full ? t('drawerRestore') : t('drawerFull')} title={full ? t('drawerRestore') : t('drawerFull')}>
          {full ? <ChevronsRight className='size-4' /> : <ChevronsLeft className='size-4' />}
        </Button>
        <Button variant='ghost' size='icon' className='size-7' onClick={onClose} aria-label={t('drawerClose')} title={t('drawerClose')}>
          <X className='size-4' />
        </Button>
      </div>

      <Tabs value={tab} onValueChange={(value) => setTab(value as DrawerTab)} className='flex min-h-0 flex-1 flex-col gap-0'>
        <TabsList className='mx-3 mt-2 h-8 w-fit'>
          <TabsTrigger value='overview' className='text-xs'>{t('tab.overview')}</TabsTrigger>
          <TabsTrigger value='schema' className='text-xs' data-testid='dag-node-tab-schema'>{t('tab.schema')}</TabsTrigger>
          {isSink ? <TabsTrigger value='plan' className='text-xs' data-testid='dag-node-tab-plan'>{t('tab.plan')}</TabsTrigger> : null}
          {runtime ? <TabsTrigger value='metrics' className='text-xs'>{t('tab.metrics')}</TabsTrigger> : null}
        </TabsList>

        <TabsContent value='overview' className='min-h-0 flex-1 overflow-auto px-3 py-3'>
          <dl className='grid grid-cols-[auto_1fr] gap-x-3 gap-y-1.5 text-xs'>
            <dt className='text-muted-foreground'>{t('rawName')}</dt>
            <dd className='flex min-w-0 items-start gap-1'>
              <span className='break-all font-mono'>{vertex.rawName}</span>
              <button type='button' className='shrink-0 text-muted-foreground hover:text-foreground' onClick={() => copyText(vertex.rawName, t('copied'))} aria-label={t('copy')}>
                <Copy className='size-3' />
              </button>
            </dd>
            {vertex.metricPrefix ? (
              <>
                <dt className='text-muted-foreground'>{t('metricPrefix')}</dt>
                <dd className='font-mono'>{vertex.metricPrefix}</dd>
              </>
            ) : null}
            <dt className='text-muted-foreground'>{t('tableCount')}</dt>
            <dd className='font-mono'>{vertex.tablePaths.length}</dd>
            {runtime ? (
              <>
                <dt className='text-muted-foreground'>{t('col.busy')}</dt>
                <dd className='font-mono'>{formatPercent(busy)}</dd>
                <dt className='text-muted-foreground'>{t('col.backpressured')}</dt>
                <dd className={cn('font-mono', backpressureTone(backpressured ?? 0))}>{formatPercent(backpressured)}</dd>
              </>
            ) : null}
          </dl>
          {vertex.saveModeWarnings?.length ? (
            <ul className='mt-3 space-y-1 text-xs text-amber-700 dark:text-amber-300'>
              {vertex.saveModeWarnings.map((warning, index) => (
                <li key={`${warning}-${index}`}>{warning}</li>
              ))}
            </ul>
          ) : null}

          <div className='mt-4 flex items-center gap-2'>
            <span className='text-xs font-medium'>{t('tables')}</span>
            <Input value={query} onChange={(event) => setQuery(event.target.value)} placeholder={t('searchTables')} className='h-7 flex-1 text-xs' />
          </div>
          <ul className='mt-2 divide-y divide-border/50 rounded-md border border-border/60'>
            {matchedTables.slice(0, limit).map((path) => (
              <li key={path} className='group flex items-start gap-2 px-2 py-1.5 hover:bg-muted/40'>
                <button type='button' className='min-w-0 flex-1 break-all text-left font-mono text-xs hover:text-primary' onClick={() => pickTable(path)} title={t('viewSchema')}>
                  {path}
                </button>
                <button type='button' className='shrink-0 text-muted-foreground opacity-0 hover:text-foreground group-hover:opacity-100' onClick={() => copyText(path, t('copied'))} aria-label={t('copyTableName')}>
                  <Copy className='size-3' />
                </button>
              </li>
            ))}
            {matchedTables.length === 0 ? (
              <li className='px-2 py-2 text-xs text-muted-foreground'>{vertex.tablePaths.length === 0 ? t('noTables') : t('noMatchedTables')}</li>
            ) : null}
          </ul>
          {matchedTables.length > limit ? (
            <Button variant='ghost' size='sm' className='mt-1 h-7 w-full text-xs' onClick={() => setLimit((value) => value + TABLE_RENDER_LIMIT)}>
              {t('showMoreTables', {shown: limit, total: matchedTables.length})}
            </Button>
          ) : null}
        </TabsContent>

        <TabsContent value='schema' className='min-h-0 flex-1 overflow-auto px-3 py-3'>
          {/* 说明放在页签内容的选表行末尾，不包在 TabsTrigger 外（Tooltip 的 data-state 会覆盖 Tabs 的 active 样式）；没有说明时不渲染 */}
          {/* The hint sits at the end of the table picker row instead of wrapping the TabsTrigger (Tooltip's data-state would override the tab's active styling); nothing is rendered without a hint */}
          {vertex.tablePaths.length > 0 || schemaHint ? (
            <div className={cn('flex items-center gap-2', vertex.tablePaths.length === 0 && 'justify-end')}>
              {vertex.tablePaths.length > 0 ? (
                <div className='min-w-0 flex-1'>
                  <TablePicker tables={vertex.tablePaths} value={table} onChange={setTable} />
                </div>
              ) : null}
              {table ? (
                <Button variant='ghost' size='icon' className='size-8' onClick={() => copyText(table, t('copied'))} aria-label={t('copyTableName')}>
                  <Copy className='size-3.5' />
                </Button>
              ) : null}
              {schemaHint ? (
                <HelpHint side='left' label={t('tab.schema')} content={schemaHint} data-testid='dag-node-schema-hint' />
              ) : null}
            </div>
          ) : null}
          {/* Refresh is decoupled from planNotice: on success planNotice is null but refresh must stay visible. */}
          {/* 刷新与 planNotice 解耦：加载成功后 planNotice 为 null，仍需保留刷新入口。 */}
          {planNotice ? (
            <div className='mt-3 flex items-start gap-2 rounded-md border border-border/60 bg-muted/20 px-2.5 py-2 text-xs text-muted-foreground'>
              <span className='flex-1'>{planNotice}</span>
              {runtime && plan?.onRefresh && !plan.loading ? (
                <Button
                  variant='ghost'
                  size='icon'
                  className='size-6'
                  onClick={plan.onRefresh}
                  aria-label={t('refresh')}
                  data-testid='plan-dag-refresh'
                >
                  <RefreshCw className='size-3' />
                </Button>
              ) : null}
            </div>
          ) : runtime && plan?.onRefresh && !plan.loading ? (
            <div className='mt-3 flex justify-end'>
              <Button
                variant='ghost'
                size='icon'
                className='size-6'
                onClick={plan.onRefresh}
                aria-label={t('refresh')}
                data-testid='plan-dag-refresh'
              >
                <RefreshCw className='size-3' />
              </Button>
            </div>
          ) : null}
          {schema ? (
            <div className='mt-3 space-y-3'>
              <dl className='grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-xs'>
                <dt className='text-muted-foreground'>{t('primaryKey')}</dt>
                <dd className='font-mono'>{schema.primaryKey.join(', ') || '-'}</dd>
                {schema.partitionKeys.length > 0 ? (
                  <>
                    <dt className='text-muted-foreground'>{t('partitionKeys')}</dt>
                    <dd className='font-mono'>{schema.partitionKeys.join(', ')}</dd>
                  </>
                ) : null}
                {schema.comment ? (
                  <>
                    <dt className='text-muted-foreground'>{t('tableComment')}</dt>
                    <dd>{schema.comment}</dd>
                  </>
                ) : null}
              </dl>
              <div className='overflow-auto rounded-md border border-border/60'>
                <Table className='text-xs'>
                  <TableHeader className='sticky top-0 bg-background'>
                    <TableRow className='hover:bg-transparent'>
                      <TableHead className='h-8 px-2 text-xs'>{t('column.name')}</TableHead>
                      <TableHead className='h-8 px-2 text-xs'>{t('column.type')}</TableHead>
                      <TableHead className='h-8 px-2 text-xs'>{t('column.key')}</TableHead>
                      <TableHead className='h-8 px-2 text-xs'>{t('column.nullable')}</TableHead>
                      <TableHead className='h-8 px-2 text-xs'>{t('column.default')}</TableHead>
                      <TableHead className='h-8 px-2 text-xs'>{t('column.comment')}</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {schema.columns.map((column) => (
                      <TableRow key={column.name}>
                        <TableCell className='whitespace-nowrap px-2 py-1.5 font-mono font-medium'>{column.name}</TableCell>
                        <TableCell className='whitespace-nowrap px-2 py-1.5 font-mono'>{column.dataType}</TableCell>
                        <TableCell className='px-2 py-1.5'>
                          {column.primaryKey ? <Badge className='px-1 py-0 text-[10px]'>PK</Badge> : null}
                          {column.uniqueKey ? <Badge variant='outline' className='px-1 py-0 text-[10px]'>UK</Badge> : null}
                        </TableCell>
                        <TableCell className='px-2 py-1.5'>
                          {column.nullable === null ? '-' : column.nullable ? t('yes') : t('no')}
                        </TableCell>
                        <TableCell className='px-2 py-1.5 font-mono'>{column.defaultValue}</TableCell>
                        <TableCell className='min-w-[120px] px-2 py-1.5 text-muted-foreground'>{column.comment || '-'}</TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            </div>
          ) : columnNames.length > 0 ? (
            <div className='mt-3 flex flex-wrap gap-1.5'>
              {columnNames.map((name) => (
                <Badge key={name} variant='secondary' className='font-mono text-[11px]'>
                  {name}
                </Badge>
              ))}
            </div>
          ) : !planNotice ? (
            <p className='mt-3 text-xs text-muted-foreground'>{t('noSchema')}</p>
          ) : null}
        </TabsContent>

        {isSink ? (
          <TabsContent value='plan' className='min-h-0 flex-1 overflow-auto px-3 py-3'>
            <div className={cn('flex items-center gap-2', vertex.tablePaths.length <= 1 && 'justify-end')}>
              {vertex.tablePaths.length > 1 ? (
                <div className='min-w-0 flex-1'>
                  <TablePicker tables={vertex.tablePaths} value={table} onChange={setTable} />
                </div>
              ) : null}
              <HelpHint
                side='left'
                label={t('tab.plan')}
                content={runtime ? t('planReadonlyHint') : t('planPreviewHint')}
                data-testid='dag-node-plan-hint'
              />
            </div>
            {planNotice ? <p className='mt-2 text-xs text-muted-foreground'>{planNotice}</p> : null}
            <div className='mt-3'>
              {savePlan ? <SaveModePlanView preview={savePlan} /> : !planNotice ? <p className='text-xs text-muted-foreground'>{t('noPlanActions')}</p> : null}
            </div>
          </TabsContent>
        ) : null}

        {runtime ? (
          <TabsContent value='metrics' className='min-h-0 flex-1 overflow-auto px-3 py-3'>
            <div className='flex items-center gap-2'>
              <Input value={query} onChange={(event) => setQuery(event.target.value)} placeholder={t('searchTables')} className='h-7 flex-1 text-xs' />
              <Button variant={metricSort === 'count' ? 'secondary' : 'ghost'} size='sm' className='h-7 text-xs' onClick={() => setMetricSort('count')}>
                {t('sortByCount')}
              </Button>
              <Button variant={metricSort === 'qps' ? 'secondary' : 'ghost'} size='sm' className='h-7 text-xs' onClick={() => setMetricSort('qps')}>
                {t('sortByQps')}
              </Button>
            </div>
            {vertex.type === 'transform' ? (
              <p className='mt-3 text-xs text-muted-foreground'>{t('transformNoMetrics')}</p>
            ) : (
              <div className='mt-2 overflow-auto rounded-md border border-border/60'>
                <Table className='text-xs'>
                  <TableHeader className='sticky top-0 bg-background'>
                    <TableRow className='hover:bg-transparent'>
                      <TableHead className='h-8 px-2 text-xs'>{t('column.table')}</TableHead>
                      <TableHead className='h-8 px-2 text-xs'>{isSink ? t('col.writeCount') : t('col.readCount')}</TableHead>
                      <TableHead className='h-8 px-2 text-xs'>{isSink ? t('col.writeBytes') : t('col.readBytes')}</TableHead>
                      <TableHead className='h-8 px-2 text-xs'>{t('col.qps')}</TableHead>
                      {isSink ? <TableHead className='h-8 px-2 text-xs'>{t('col.committed')}</TableHead> : null}
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {tableMetrics.slice(0, limit).map((row) => (
                      <TableRow key={row.path}>
                        <TableCell className='max-w-[260px] px-2 py-1.5 font-mono'>
                          <span className='block truncate' title={row.path}>{shortTablePath(row.path, 36)}</span>
                        </TableCell>
                        <TableCell className='px-2 py-1.5 font-mono'>{formatCount(row.count)}</TableCell>
                        <TableCell className='px-2 py-1.5 font-mono'>{formatBytes(row.bytes)}</TableCell>
                        <TableCell className='px-2 py-1.5 font-mono'>{typeof row.qps === 'number' ? `${formatCount(row.qps)}/s` : '-'}</TableCell>
                        {isSink ? <TableCell className='px-2 py-1.5 font-mono'>{formatCount(row.committed)}</TableCell> : null}
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            )}
            {tableMetrics.length > limit ? (
              <Button variant='ghost' size='sm' className='mt-1 h-7 w-full text-xs' onClick={() => setLimit((value) => value + TABLE_RENDER_LIMIT)}>
                {t('showMoreTables', {shown: limit, total: tableMetrics.length})}
              </Button>
            ) : null}
          </TabsContent>
        ) : null}
      </Tabs>
    </div>
  );
}
