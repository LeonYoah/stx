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

import {Badge} from '@/components/ui/badge';
import {Checkbox} from '@/components/arc/checkbox/checkbox';
import {mergeIdsIntoSelection} from '@/components/common/layout';
import {Tooltip, TooltipContent, TooltipTrigger} from '@/components/ui/tooltip';
import {Sparkline} from '@/components/charts/sparkline';
import type {SyncJobInstance, SyncJobListSort} from '@/lib/services/sync';
import {
  formatDateTimeMinuteTz,
  formatDateTimeTz,
  formatTimeTz,
} from '@/lib/timezone';
import {cn} from '@/lib/utils';
import {JobOutcomeHints} from '../JobOutcomeHints';
import {
  formatJobDateTime,
  getDisplayJobLifecycleStatus,
  getEngineAPIMode,
  getJobStatusBadgeClass,
  getJobStatusLabel,
  getRunModeLabel,
  submitSpecExecutionMode,
} from '../sync-studio-utils';
import {JobRowActions} from './JobRowActions';
import {
  formatJobDurationSeconds,
  formatRatePair,
  formatTotalPair,
  isRunningStatus,
  jobDisplayName,
  jobDurationSeconds,
  jobListRates,
  jobTimeBounds,
  resolveJobInitiatorName,
} from './job-table-format';
import type {JobColumnDef, JobTableContext} from './types';

type CellProps = {job: SyncJobInstance; ctx: JobTableContext};

/** 时间列承载的排序维度；工具栏「排序」菜单复用同一份清单。/ Sort dimensions hosted by the time column; the toolbar sort menu reuses this list. */
export const TIME_SORT_KEYS = [
  'started_at',
  'finished_at',
  'duration',
] as const satisfies readonly SyncJobListSort[];

/** 全选复选框（仅当前页）：merge 语义，不丢其他页的勾选。/ Select-all checkbox scoped to the current page; merge semantics keep picks made on other pages. */
function SelectAllHeader({ctx}: {ctx: JobTableContext}) {
  const selection = ctx.selection;
  if (!selection) {
    return null;
  }
  const allSelected =
    ctx.items.length > 0 &&
    ctx.items.every((job) => selection.ids.has(job.id));
  const someSelected =
    !allSelected && ctx.items.some((job) => selection.ids.has(job.id));
  return (
    <Checkbox
      checked={allSelected ? true : someSelected ? 'indeterminate' : false}
      aria-label={ctx.t('selectAll')}
      onCheckedChange={(checked) =>
        selection.onChange(
          mergeIdsIntoSelection(
            selection.ids,
            ctx.items.map((job) => job.id),
            checked === true,
          ),
        )
      }
    />
  );
}

/** 行复选框：点击不冒泡到行激活。/ Row checkbox; clicks do not bubble into row activation. */
function RowCheckbox({job, ctx}: CellProps) {
  const selection = ctx.selection;
  if (!selection) {
    return null;
  }
  return (
    <Checkbox
      checked={selection.ids.has(job.id)}
      aria-label={jobDisplayName(job)}
      onClick={(event) => event.stopPropagation()}
      onCheckedChange={(checked) => {
        const next = new Set(selection.ids);
        if (checked) {
          next.add(job.id);
        } else {
          next.delete(job.id);
        }
        selection.onChange(next);
      }}
    />
  );
}

/**
 * 作业标识列：global 显示作业名 + 「#id · engine_job_id」（hover title 带任务引用）；
 * task 显示「#id」+ 引擎 / 平台作业 ID。统一 truncate + title。
 * Job identity cell: the global preset shows the display name plus "#id · engine_job_id" (task ref on hover);
 * the task preset shows "#id" plus the engine / platform job ID. Both truncate with a title.
 */
function JobNameCell({job, ctx}: CellProps) {
  if (ctx.preset === 'task') {
    const secondary = job.engine_job_id || job.platform_job_id || '';
    return (
      <div className='flex min-w-0 max-w-[220px] flex-col'>
        <span
          className='truncate font-medium text-foreground'
          title={`#${job.id}`}
        >
          #{job.id}
        </span>
        {secondary ? (
          <span
            className='truncate font-mono text-[11px] text-muted-foreground'
            title={secondary}
          >
            {secondary}
          </span>
        ) : null}
      </div>
    );
  }
  const name = jobDisplayName(job);
  const engineRef = job.engine_job_id ? ` · ${job.engine_job_id}` : '';
  // 外部作业 task_id 恒为 0，「任务 #0」没有引用价值，悬浮提示只留作业与引擎 ID。
  // External jobs always have task_id 0, so "Task #0" is noise — keep only job and engine IDs in the title.
  const taskRef =
    job.task_id > 0 ? ctx.t('taskRef', {id: job.task_id}) : `#${job.id}`;
  return (
    <div className='flex min-w-0 max-w-[280px] flex-col'>
      <span className='truncate font-medium text-foreground' title={name}>
        {name}
      </span>
      <span
        className='truncate font-mono text-[11px] text-muted-foreground'
        title={`${taskRef}${engineRef}`}
      >
        {`#${job.id}${engineRef}`}
      </span>
    </div>
  );
}

/**
 * 运行方式 + 模式 + 版本 + 通道（Local / V1 / REST V2），整组不换行。
 * Run mode + task mode + version + channel (Local / V1 / REST V2) as one non-breaking group.
 */
function RunModeCell({job, ctx}: CellProps) {
  const t = ctx.t;
  const external = job.source === 'external';
  return (
    <div className='flex items-center gap-1 whitespace-nowrap'>
      <Badge
        variant='outline'
        className='rounded-sm px-1 py-0 text-[10px]'
      >
        {t(external ? 'source.external' : 'source.stx')}
      </Badge>
      {/* 外部作业的运行方式恒为「外部提交」，与来源徽标同义，不再重复渲染。
          External jobs always run "submitted externally", which the source badge already says — skip the duplicate. */}
      {!external && (
        <Badge
          variant='outline'
          className='rounded-sm px-1 py-0 text-[10px]'
          title={job.external_ref || undefined}
        >
          {getRunModeLabel(job, t)}
        </Badge>
      )}
      {job.mode ? (
        <Badge
          variant='outline'
          className='rounded-sm px-1.5 py-0 text-[11px]'
        >
          {t(`mode.${job.mode}`)}
        </Badge>
      ) : (
        // 缺 jobDag 的外部作业拿不到 mode，按设计稿显示占位符而不是留白。
        // External jobs without jobDag have no mode; show a placeholder per the design instead of blank.
        <span className='text-muted-foreground'>-</span>
      )}
      {job.task_version > 0 ? (
        <span className='font-mono text-[10px] text-muted-foreground'>
          v{job.task_version}
        </span>
      ) : null}
      <span className='font-mono text-[10px] text-muted-foreground'>
        {submitSpecExecutionMode(job.submit_spec) === 'local'
          ? 'Local'
          : getEngineAPIMode(job) === 'v1'
            ? 'V1'
            : 'REST V2'}
      </span>
    </div>
  );
}

/** 状态徽章 + 结果提示（提交确认中 / 超时等）。/ Status badge plus outcome hints (confirming submit, timeouts). */
function StatusCell({job}: {job: SyncJobInstance}) {
  const displayStatus = getDisplayJobLifecycleStatus(job);
  return (
    <div className='flex items-center whitespace-nowrap'>
      <Badge
        variant='outline'
        className={cn(
          'rounded-sm px-2 py-0 text-[11px] leading-tight',
          getJobStatusBadgeClass(displayStatus),
        )}
      >
        {getJobStatusLabel(displayStatus)}
      </Badge>
      <JobOutcomeHints job={job} className='ml-1 align-middle' />
    </div>
  );
}

/** 发起人：本人加「你」徽章。/ Initiator with a "you" badge for the current user. */
function InitiatorCell({job, ctx}: CellProps) {
  const name = resolveJobInitiatorName(
    job,
    ctx.currentUserId,
    ctx.currentUsername,
    ctx.initiatorUsers,
  );
  const isSelf =
    ctx.currentUserId !== undefined && job.created_by === ctx.currentUserId;
  return (
    <div className='flex items-center gap-1 whitespace-nowrap'>
      <span className='text-[11px] font-medium text-foreground'>{name}</span>
      {isSelf ? (
        <Badge
          variant='secondary'
          className='h-3.5 px-1 text-[9px] font-normal text-muted-foreground'
        >
          {ctx.t('you')}
        </Badge>
      ) : null}
    </div>
  );
}

/**
 * 速率列：Prometheus 可用时带迷你曲线；已结束作业显示平均速率（浅色 + 「均」前缀 + title 提示）。
 * Rates cell: sparkline when Prometheus answers; finished jobs show the run average (muted, "avg" prefix, hint).
 */
function RatesCell({job, ctx}: CellProps) {
  const points = ctx.sparklines?.items[String(job.id)];
  const running = isRunningStatus(job.status);
  const {endMs} = jobTimeBounds(job);
  const duration = jobDurationSeconds(job, ctx.updatedAt);
  const rates = jobListRates(
    running,
    job.live,
    endMs !== null ? duration : 0,
  );
  return (
    <div className='flex items-center gap-2'>
      {ctx.sparklines?.source === 'prometheus' ? (
        <div className='w-24 shrink-0'>
          {points && points.length > 1 ? (
            <Sparkline
              data={points.map((point) => point[1])}
              label={ctx.t('sparklineLabel', {name: jobDisplayName(job)})}
              height={22}
              variant='area'
              interactive={false}
              color='var(--series-2)'
            />
          ) : null}
        </div>
      ) : null}
      {rates.read === undefined && rates.write === undefined ? (
        <span>-</span>
      ) : (
        <span
          className={cn(
            'whitespace-nowrap tabular-nums',
            rates.average && 'text-muted-foreground',
          )}
          title={rates.average ? ctx.t('avgRateHint') : undefined}
        >
          {rates.average ? ctx.t('avgPrefix') : ''}
          {formatRatePair(rates.read, rates.write)}
        </span>
      )}
    </div>
  );
}

/**
 * 条数列：「读 / 写」compact 数字；live.table_count > 1 时加多表徽章，点击打开详情的指标页。
 * Totals cell: compact read / write pair; a multi-table badge (live.table_count > 1) opens the detail's
 * metrics tab.
 */
function TotalsCell({job, ctx}: CellProps) {
  const live = job.live;
  const tableCount = live?.table_count ?? 0;
  return (
    <div className='flex items-center justify-end gap-1.5 whitespace-nowrap tabular-nums'>
      <span>{formatTotalPair(live?.read_total, live?.write_total)}</span>
      {tableCount > 1 ? (
        <Tooltip>
          <TooltipTrigger asChild>
            <Badge
              variant='outline'
              className='cursor-pointer rounded-sm border-border/60 bg-muted/50 px-1 py-0 font-sans text-[10px] text-muted-foreground transition-colors hover:bg-muted'
              data-testid={`${ctx.testIdPrefix}-multi-table-${job.id}`}
              onClick={(event) => {
                event.stopPropagation();
                ctx.onOpenDetail(job, 'metrics');
              }}
            >
              {ctx.t('multiTableBadge', {count: tableCount})}
            </Badge>
          </TooltipTrigger>
          <TooltipContent side='top' className='text-xs'>
            {ctx.t('viewMultiTableMetricsHint')}
          </TooltipContent>
        </Tooltip>
      ) : null}
    </div>
  );
}

/**
 * CDC 延迟列：≥60s 红色、>0 琥珀色；job.cdc_delay 存在时 tooltip 显示 1 分钟内最大值与采样时间。
 * CDC delay cell: red at ≥60s, amber above 0; when job.cdc_delay exists the tooltip shows the 1-minute max
 * and sample instant.
 */
function CdcCell({job, ctx}: CellProps) {
  const delay = job.live?.cdc_delay_seconds;
  if (delay === undefined) {
    return <span>-</span>;
  }
  const body = (
    <span
      className={cn(
        'whitespace-nowrap tabular-nums',
        delay >= 60
          ? 'text-red-600 dark:text-red-400'
          : delay > 0
            ? 'text-amber-600 dark:text-amber-400'
            : '',
      )}
      data-testid={`${ctx.testIdPrefix}-cdc-${job.id}`}
    >
      {`${ctx.format.number(delay, {maximumFractionDigits: 1})}s`}
    </span>
  );
  if (job.cdc_delay?.fresh) {
    return (
      <Tooltip>
        <TooltipTrigger asChild>{body}</TooltipTrigger>
        <TooltipContent side='top' className='text-xs'>
          {ctx.t('cdcDelayTooltip', {
            max: `${(job.cdc_delay.max_1m_ms / 1000).toFixed(1)}s`,
            time: formatTimeTz(job.cdc_delay.at),
          })}
        </TooltipContent>
      </Tooltip>
    );
  }
  return body;
}

/**
 * 时间线列：主行开始时间，副行浅色时长（运行中「已运行」/ 已结束「耗时」）；tooltip 给开始、结束、时长全量。
 * Timeline cell: start instant on the primary line and a muted duration below ("running for" / "took"); the
 * tooltip carries full start / end / duration.
 */
function TimelineCell({job, ctx}: CellProps) {
  const {startMs, endMs} = jobTimeBounds(job);
  const running = isRunningStatus(job.status);
  const duration = jobDurationSeconds(job, ctx.updatedAt);
  const units = {
    d: ctx.t('units.d'),
    h: ctx.t('units.h'),
    m: ctx.t('units.m'),
    s: ctx.t('units.s'),
  };
  const durationText =
    duration > 0 ? formatJobDurationSeconds(duration, units) : null;
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <div className='flex cursor-default flex-col whitespace-nowrap'>
          <span className='tabular-nums text-foreground'>
            {startMs !== null ? formatDateTimeMinuteTz(startMs) : '-'}
          </span>
          {durationText ? (
            <span className='tabular-nums text-[11px] text-muted-foreground'>
              {running
                ? ctx.t('ranFor', {duration: durationText})
                : ctx.t('tookFor', {duration: durationText})}
            </span>
          ) : null}
        </div>
      </TooltipTrigger>
      <TooltipContent side='top' className='space-y-1 font-mono text-xs'>
        <div>
          {ctx.t('startedAt')}: {formatJobDateTime(job.started_at || job.created_at)}
        </div>
        <div>
          {ctx.t('finishedAt')}: {endMs !== null ? formatDateTimeTz(endMs) : '-'}
        </div>
        <div>
          {ctx.t('duration')}: {durationText ?? '-'}
        </div>
      </TooltipContent>
    </Tooltip>
  );
}

/**
 * 唯一的作业列注册表：大盘与工作台共用（父 design §4）；JobTable 按 presets + visibleWhen 过滤后渲染。
 * The single job column registry shared by dashboard and workbench (parent design §4); JobTable filters it by
 * presets + visibleWhen before rendering.
 */
export const JOB_COLUMNS: JobColumnDef[] = [
  {
    id: 'select',
    presets: ['global', 'task'],
    // 宿主不传 selection 时本列不出现（task 预设默认不开批量）。/ Only when the host supplies selection
    // (the task preset keeps bulk off by default).
    visibleWhen: (_items, ctx) => Boolean(ctx.selection),
    width: 36,
    minWidth: 0,
    header: (ctx) => <SelectAllHeader ctx={ctx} />,
    cell: (job, ctx) => <RowCheckbox job={job} ctx={ctx} />,
  },
  {
    id: 'job',
    presets: ['global', 'task'],
    minWidth: 220,
    header: (ctx) => ctx.t('columns.job'),
    cell: (job, ctx) => <JobNameCell job={job} ctx={ctx} />,
  },
  {
    id: 'cluster',
    presets: ['global'],
    header: (ctx) => ctx.t('columns.cluster'),
    cell: (job, ctx) => (
      <span className='text-muted-foreground'>
        {job.cluster_id
          ? (ctx.clusterNames?.get(job.cluster_id) ?? `#${job.cluster_id}`)
          : '-'}
      </span>
    ),
  },
  {
    id: 'run',
    presets: ['global', 'task'],
    minWidth: 0,
    header: (ctx) => ctx.t('columns.run'),
    cell: (job, ctx) => <RunModeCell job={job} ctx={ctx} />,
  },
  {
    id: 'status',
    presets: ['global', 'task'],
    sortKey: 'attention',
    header: (ctx) => ctx.t('columns.status'),
    cell: (job, _ctx) => <StatusCell job={job} />,
  },
  {
    id: 'initiator',
    presets: ['global', 'task'],
    header: (ctx) => ctx.t('columns.initiator'),
    cell: (job, ctx) => <InitiatorCell job={job} ctx={ctx} />,
  },
  {
    id: 'rates',
    presets: ['global', 'task'],
    sortKey: 'rate',
    sortableWhen: (query) => query.statusGroup === 'running',
    minWidth: 180,
    header: (ctx) => ctx.t('columns.rates'),
    cell: (job, ctx) => <RatesCell job={job} ctx={ctx} />,
  },
  {
    id: 'totals',
    presets: ['global', 'task'],
    align: 'end',
    header: (ctx) => ctx.t('columns.totals'),
    cell: (job, ctx) => <TotalsCell job={job} ctx={ctx} />,
  },
  {
    id: 'cdc',
    presets: ['global', 'task'],
    align: 'end',
    sortKey: 'cdc_delay',
    sortableWhen: (query) => query.statusGroup === 'running',
    // task 预设动态显示：本页任一行有延迟值时列才出现。/ Dynamic in the task preset: the column appears
    // only when any row on the page reports a delay.
    visibleWhen: (items, ctx) =>
      ctx.preset === 'global' ||
      items.some(
        (job) =>
          job.live?.cdc_delay_seconds !== undefined || job.cdc_delay?.fresh,
      ),
    header: (ctx) => ctx.t('columns.cdc'),
    cell: (job, ctx) => <CdcCell job={job} ctx={ctx} />,
  },
  {
    id: 'timeline',
    presets: ['global', 'task'],
    sortKey: 'started_at',
    // 维度切换走工具栏「排序」菜单；列头负责方向：命中任一维度即点亮，点按作用于当前维度。
    // Dimension switching lives in the toolbar sort menu; the header owns direction — it
    // lights on any matching dimension and clicking toggles the active one.
    sortKeys: [...TIME_SORT_KEYS],
    minWidth: 120,
    header: (ctx) => ctx.t('columns.timeline'),
    cell: (job, ctx) => <TimelineCell job={job} ctx={ctx} />,
  },
  {
    id: 'actions',
    presets: ['global', 'task'],
    align: 'end',
    minWidth: 0,
    header: (ctx) => ctx.t('columns.actions'),
    cell: (job, ctx) => (
      <JobRowActions
        job={job}
        actor={ctx.actorOf(job)}
        showWorkbenchLink={
          ctx.preset === 'global' && job.source !== 'external'
        }
        t={ctx.t}
        onOpenDetail={ctx.onOpenDetail}
        onAction={ctx.onAction}
      />
    ),
  },
];
