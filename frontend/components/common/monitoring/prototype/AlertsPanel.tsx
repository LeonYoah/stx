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
 * Alerts panel prototype: metric strip, filter toolbar, sortable table, detail drawer.
 * 告警面板原型：分隔条指标行（点击即筛选）、filter-toolbar、sortable-data-table、
 * Arc drawer 详情（命中规则 / 时间线 / 投递），窄屏退化为双行卡片。
 */
import {useMemo, useState} from 'react';
import {useLocale, useTranslations} from 'next-intl';
import {
  BellOff,
  CheckCheck,
  ChevronRight,
  TrendingDown,
  TrendingUp,
  X,
} from 'lucide-react';
import {Badge} from '@/components/arc/badge/badge';
import {Drawer, DrawerContent} from '@/components/arc/drawer/drawer';
import {DropdownMenu} from '@/components/arc/dropdown-menu/dropdown-menu';
import {FilterToolbar, type FilterChip, type FilterField} from '@/components/arc/filter-toolbar/filter-toolbar';
import {SearchField} from '@/components/arc/search-field/search-field';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {SortableDataTable, type DataColumn} from '@/components/arc/sortable-data-table/sortable-data-table';
import {Button} from '@/components/ui/button';
import {Pagination} from '@/components/ui/pagination';
import {cn} from '@/lib/utils';
import {PROTO_ALERTS, type ProtoAlert, type ProtoAlertStatus} from './alerts-mock';

function useTimeAgo() {
  const t = useTranslations('monitoringProto');
  return (min: number) =>
    min < 60
      ? t('agoMinutes', {value: min})
      : min < 1440
        ? t('agoHours', {value: Math.round(min / 60)})
        : t('agoDays', {value: Math.round(min / 1440)});
}

function StatusCell({status, t}: {status: ProtoAlertStatus; t: (k: string) => string}) {
  return (
    <span className='flex items-center gap-1.5 whitespace-nowrap text-xs'>
      <span
        className={cn('size-1.5 shrink-0 rounded-full', {
          'animate-pulse bg-destructive': status === 'firing',
          'bg-muted-foreground/50': status === 'acknowledged',
          'bg-(--warning)': status === 'silenced',
          'bg-muted-foreground/30': status === 'resolved',
        })}
        aria-hidden
      />
      <span className={status === 'firing' ? 'text-destructive' : 'text-muted-foreground'}>
        {t(`status.${status}`)}
      </span>
    </span>
  );
}

export function AlertsPanel({onOpenRule}: {onOpenRule?: (ruleKey: string) => void}) {
  const t = useTranslations('monitoringProto');
  const locale = useLocale();
  const ago = useTimeAgo();
  const [scope, setScope] = useState('all');
  const [fieldSel, setFieldSel] = useState<Record<string, string>>({});
  const [query, setQuery] = useState('');
  const [page, setPage] = useState(1);
  const [activeId, setActiveId] = useState<string | null>(null);
  const [alerts, setAlerts] = useState(PROTO_ALERTS);
  const pageSize = 8;

  const firingCount = alerts.filter((a) => a.status === 'firing').length;
  const criticalCount = alerts.filter((a) => a.status === 'firing' && a.severity === 'critical').length;
  const newCount = alerts.filter((a) => a.startedMinAgo <= 1440).length;

  const clusters = useMemo(() => [...new Set(alerts.map((a) => a.cluster))], [alerts]);

  const filterFields: FilterField[] = [
    {
      id: 'status',
      label: t('alertStatusLabel'),
      options: (['firing', 'acknowledged', 'silenced', 'resolved'] as const).map((v) => ({
        value: v,
        label: t(`status.${v}`),
      })),
    },
    {
      id: 'severity',
      label: t('fieldSeverity'),
      options: [
        {value: 'warning', label: t('severityWarning')},
        {value: 'critical', label: t('severityCritical')},
      ],
    },
    {id: 'cluster', label: t('colCluster'), options: clusters},
  ];

  const fieldLabels: Record<string, Record<string, string>> = {
    status: Object.fromEntries(
      (['firing', 'acknowledged', 'silenced', 'resolved'] as const).map((v) => [v, t(`status.${v}`)]),
    ),
    severity: {warning: t('severityWarning'), critical: t('severityCritical')},
    cluster: {},
  };
  const activeChips: FilterChip[] = Object.entries(fieldSel).map(([fieldId, value]) => ({
    id: fieldId,
    label: filterFields.find((f) => f.id === fieldId)?.label ?? fieldId,
    value: fieldLabels[fieldId]?.[value] ?? value,
  }));

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    const list = alerts.filter((a) => {
      if (scope !== 'all' && a.scope !== scope) {return false;}
      if (fieldSel.status && a.status !== fieldSel.status) {return false;}
      if (fieldSel.severity && a.severity !== fieldSel.severity) {return false;}
      if (fieldSel.cluster && a.cluster !== fieldSel.cluster) {return false;}
      const name = locale === 'en' ? a.titleEn : a.title;
      if (q && !name.toLowerCase().includes(q) && !a.ruleKey.toLowerCase().includes(q) && !a.cluster.toLowerCase().includes(q)) {
        return false;
      }
      return true;
    });
    // 与表格默认排序一致：最近的在前。Match the table's default sort: most recent first.
    return list.sort((a, b) => a.startedMinAgo - b.startedMinAgo);
  }, [alerts, scope, fieldSel, query, locale]);

  const totalPages = Math.max(1, Math.ceil(filtered.length / pageSize));
  const pageItems = filtered.slice((page - 1) * pageSize, page * pageSize);
  const active = alerts.find((a) => a.id === activeId) ?? null;

  const patchStatus = (id: string, status: ProtoAlertStatus) => {
    setAlerts((prev) => prev.map((a) => (a.id === id ? {...a, status} : a)));
  };

  const setField = (fieldId: string, value: string | null) => {
    setFieldSel((prev) => {
      const next = {...prev};
      if (value == null) {
        delete next[fieldId];
      } else {
        next[fieldId] = value;
      }
      return next;
    });
    setPage(1);
  };

  const columns: DataColumn<Record<string, unknown>>[] = [
    {
      key: 'status',
      label: t('colStatus'),
      sortable: true,
      width: 96,
      render: (_, row) => <StatusCell status={row.status as ProtoAlertStatus} t={t} />,
    },
    {
      key: 'name',
      label: t('colAlert'),
      sortable: true,
      render: (_, row) => {
        const alert = row.alert as ProtoAlert;
        return (
          <button
            type='button'
            className='min-w-0 text-left'
            onClick={() => setActiveId(alert.id)}
            data-testid={`proto-alert-name-${alert.id}`}
          >
            <div className='truncate text-sm font-medium hover:underline'>{row.name as string}</div>
            <div className='truncate font-mono text-xs text-muted-foreground'>{alert.ruleKey}</div>
          </button>
        );
      },
    },
    {
      key: 'severity',
      label: t('fieldSeverity'),
      sortable: true,
      width: 76,
      render: (_, row) => (
        <Badge tone={row.severity === 'critical' ? 'danger' : 'warning'} size='sm'>
          {row.severity === 'critical' ? t('severityCritical') : t('severityWarning')}
        </Badge>
      ),
    },
    {
      key: 'scope',
      label: t('colScope'),
      width: 76,
      render: (_, row) => (
        <Badge size='sm' outline>
          {row.scope === 'cluster' ? t('scopeCluster') : t('scopeJob')}
        </Badge>
      ),
    },
    {
      key: 'cluster',
      label: t('colCluster'),
      sortable: true,
      render: (_, row) => (
        <span className='block min-w-0 truncate text-xs text-muted-foreground'>{row.cluster as string}</span>
      ),
    },
    {
      key: 'startedMinAgo',
      label: t('colTime'),
      sortable: true,
      numeric: true,
      width: 92,
      render: (_, row) => (
        <span className='whitespace-nowrap text-xs tabular-nums text-muted-foreground'>
          {ago(row.startedMinAgo as number)}
        </span>
      ),
    },
    {
      key: 'actions',
      label: '',
      width: 140,
      render: (_, row) => {
        const alert = row.alert as ProtoAlert;
        return (
          <div className='flex justify-end'>
          <DropdownMenu
            label={t('moreActions')}
            items={[
              {
                label: t('actionAck'),
                icon: <CheckCheck className='size-4' aria-hidden />,
                disabled: alert.status !== 'firing',
                onSelect: () => patchStatus(alert.id, 'acknowledged'),
              },
              {
                label: t('actionSilence'),
                icon: <BellOff className='size-4' aria-hidden />,
                disabled: alert.status !== 'firing',
                onSelect: () => patchStatus(alert.id, 'silenced'),
              },
              {
                label: t('actionClose'),
                icon: <X className='size-4' aria-hidden />,
                destructive: true,
                separatorBefore: true,
                disabled: alert.status === 'resolved',
                onSelect: () => patchStatus(alert.id, 'resolved'),
              },
            ]}
          />
          </div>
        );
      },
    },
  ];

  const rows = pageItems.map((a) => ({
    ...a,
    name: (locale === 'en' ? a.titleEn : a.title) as string,
    alert: a,
    actions: '',
  }));

  return (
    <div className='relative flex h-full flex-col gap-3'>
      <div className='grid grid-cols-3 divide-x divide-border/60 rounded-lg border border-border/60 bg-card'>
        {(
          [
            {
              label: t('metricFiring'),
              value: firingCount,
              delta: 2,
              active: fieldSel.status === 'firing' && !fieldSel.severity,
              onClick: () => setField('status', 'firing'),
            },
            {
              label: t('metricCritical'),
              value: criticalCount,
              delta: 1,
              active: fieldSel.severity === 'critical',
              onClick: () => {
                setFieldSel({status: 'firing', severity: 'critical'});
                setPage(1);
              },
            },
            {
              label: t('metricNew24h'),
              value: newCount,
              delta: 4,
              active: false,
              onClick: () => {
                setFieldSel({});
                setPage(1);
              },
            },
          ]
        ).map((m) => (
          <button
            key={m.label}
            type='button'
            onClick={m.onClick}
            className={cn('px-4 py-3 text-left transition-colors hover:bg-muted/40', m.active && 'bg-muted/50')}
          >
            <div className='text-xs text-muted-foreground'>{m.label}</div>
            <div className='mt-1 text-2xl font-medium tabular-nums'>{m.value}</div>
            <div className='mt-0.5 flex items-center gap-1 text-xs'>
              {m.delta >= 0 ? (
                <TrendingUp className='size-3 text-destructive' aria-hidden />
              ) : (
                <TrendingDown className='size-3 text-(--success)' aria-hidden />
              )}
              <span className={m.delta >= 0 ? 'text-destructive' : 'text-(--success)'}>
                {m.delta >= 0 ? '+' : ''}
                {m.delta}
              </span>
              <span className='text-muted-foreground'>{t('vsYesterday')}</span>
            </div>
          </button>
        ))}
      </div>

      <div className='flex flex-wrap items-center gap-3'>
        <SegmentedControl
          label={t('alertScopeLabel')}
          value={scope}
          onValueChange={setScope}
          options={[
            {value: 'all', label: t('scopeAll')},
            {value: 'cluster', label: t('scopeCluster')},
            {value: 'job', label: t('scopeJob')},
          ]}
        />
        {/* 组件内空态文案只有英文；无激活筛选时 chips 容器里唯一的 span 就是它，藏掉由「添加筛选」承担入口。
            The built-in empty note is English-only; with no active filters it is the only span in the chips track. */}
        <div className={cn('w-full sm:w-auto', activeChips.length === 0 && '[&>div>div>div>span:last-child]:hidden!')}>
          <FilterToolbar
            filters={activeChips}
            onRemove={(id) => setField(id, null)}
            onClearAll={() => setFieldSel({})}
          addFilter={{
            fields: filterFields,
            label: t('addFilter'),
            align: 'start',
            onAdd: (chip, field) => {
              // chip.value 是选项的展示文案，需反查回原始 value
              // chip.value is the option's display label; map back to the raw value
              const def = filterFields.find((f) => f.id === field.id);
              const raw = def?.options
                .map((o) => (typeof o === 'string' ? {v: o, l: o} : {v: o.value, l: o.label ?? o.value}))
                .find((o) => o.l === chip.value);
              setField(field.id, raw?.v ?? chip.value ?? '');
            },
          }}
          />
        </div>
        <div className='w-56 min-w-[200px] max-w-sm flex-1 [&_label]:sr-only'>
          <SearchField
            label={t('alertSearchLabel')}
            value={query}
            onValueChange={(v) => {
              setQuery(v);
              setPage(1);
            }}
            placeholder={t('alertSearchPlaceholder')}
            clearLabel={t('searchClear')}
          />
        </div>
      </div>

      <div className='min-h-0 flex-1 overflow-y-auto'>
        <div className='hidden sm:block'>
          <SortableDataTable
            rows={rows}
            columns={columns}
            rowKey='id'
            caption={t('tabAlerts')}
            emptyMessage={t('alertsEmpty')}
            defaultSort={{key: 'startedMinAgo', direction: 'asc'}}
            itemName={{one: t('itemAlert'), other: t('itemAlerts')}}
          />
        </div>
        <div className='sm:hidden'>
          {pageItems.length === 0 ? (
            <div className='flex min-h-[160px] flex-col items-center justify-center gap-2 text-sm text-muted-foreground'>
              <CheckCheck className='size-6 opacity-50' aria-hidden />
              {t('alertsEmpty')}
            </div>
          ) : (
            pageItems.map((alert) => (
              <MobileAlertCard key={alert.id} alert={alert} ago={ago} onOpen={() => setActiveId(alert.id)} />
            ))
          )}
        </div>
      </div>

      <Pagination
        currentPage={page}
        totalPages={totalPages}
        pageSize={pageSize}
        totalItems={filtered.length}
        onPageChange={setPage}
        showPageSizeSelector={false}
      />

      <Drawer open={active != null} onOpenChange={(open) => !open && setActiveId(null)}>
        {active ? (
          <DrawerContent
            side='right'
            title={locale === 'en' ? active.titleEn : active.title}
            description={active.cluster}
            className='w-[min(92vw,420px)]'
          >
            <AlertDetailBody
              alert={active}
              ago={ago}
              onOpenRule={onOpenRule}
              onAck={() => patchStatus(active.id, 'acknowledged')}
              onSilence={() => patchStatus(active.id, 'silenced')}
            />
          </DrawerContent>
        ) : null}
      </Drawer>
    </div>
  );
}

function MobileAlertCard({alert, ago, onOpen}: {alert: ProtoAlert; ago: (m: number) => string; onOpen: () => void}) {
  const t = useTranslations('monitoringProto');
  const locale = useLocale();
  const name = locale === 'en' ? alert.titleEn : alert.title;
  return (
    <div className='border-b border-border/40 px-2 py-3'>
      <button type='button' className='flex w-full items-center gap-2 text-left' onClick={onOpen}>
        <StatusCell status={alert.status} t={t} />
        <span className='min-w-0 flex-1 truncate text-sm font-medium'>{name}</span>
        <ChevronRight className='size-4 shrink-0 text-muted-foreground' aria-hidden />
      </button>
      <div className='mt-1.5 flex flex-wrap items-center gap-2 pl-4 text-xs text-muted-foreground'>
        <Badge tone={alert.severity === 'critical' ? 'danger' : 'warning'} size='sm'>
          {alert.severity === 'critical' ? t('severityCritical') : t('severityWarning')}
        </Badge>
        <Badge size='sm' outline>
          {alert.scope === 'cluster' ? t('scopeCluster') : t('scopeJob')}
        </Badge>
        <span className='min-w-0 truncate'>{alert.cluster}</span>
        <span className='whitespace-nowrap tabular-nums'>{ago(alert.startedMinAgo)}</span>
      </div>
    </div>
  );
}

function AlertDetailBody({
  alert,
  ago,
  onOpenRule,
  onAck,
  onSilence,
}: {
  alert: ProtoAlert;
  ago: (m: number) => string;
  onOpenRule?: (ruleKey: string) => void;
  onAck: () => void;
  onSilence: () => void;
}) {
  const t = useTranslations('monitoringProto');

  return (
    <div className='space-y-4 px-1 pb-4'>
      <div className='flex items-center gap-2'>
        <Badge tone={alert.severity === 'critical' ? 'danger' : 'warning'} size='sm'>
          {alert.severity === 'critical' ? t('severityCritical') : t('severityWarning')}
        </Badge>
        <StatusCell status={alert.status} t={t} />
      </div>

      <div className='rounded-lg border border-border/60 px-3 py-2.5 text-sm'>
        <span className='text-muted-foreground'>{t('hitRule')}</span>{' '}
        <button
          type='button'
          className='font-medium text-primary underline-offset-2 hover:underline'
          onClick={() => onOpenRule?.(alert.ruleKey)}
        >
          {alert.ruleKey}
        </button>
        <span className='text-muted-foreground'>
          {' · '}
          {alert.effectiveFrom === 'exception' ? t('fromException') : t('fromGlobal')}
          {alert.threshold ? (
            <>
              {' · '}
              <span className='whitespace-nowrap'>{alert.threshold}</span>
            </>
          ) : null}
          {alert.forMinutes != null ? (
            <>
              {' · '}
              <span className='whitespace-nowrap'>
                {alert.forMinutes === 0 ? t('realtime') : t('forMinutes', {minutes: alert.forMinutes})}
              </span>
            </>
          ) : null}
        </span>
      </div>

      {alert.status === 'firing' ? (
        <div className='flex gap-2'>
          <Button type='button' size='sm' className='h-8 rounded-md text-xs' onClick={onAck}>
            <CheckCheck className='mr-1.5 size-3.5' aria-hidden />
            {t('actionAck')}
          </Button>
          <Button type='button' size='sm' variant='outline' className='h-8 rounded-md text-xs' onClick={onSilence}>
            <BellOff className='mr-1.5 size-3.5' aria-hidden />
            {t('actionSilence')}
          </Button>
        </div>
      ) : null}

      <div className='space-y-2'>
        <span className='text-sm font-medium'>{t('timeline')}</span>
        <ol className='relative space-y-3 border-l border-border/60 pl-4'>
          {alert.timeline.map((ev, i) => (
            <li key={i} className='relative text-sm'>
              <span className='absolute -left-[21px] top-0.5 flex size-3.5 items-center justify-center rounded-full bg-card'>
                <TimelineDot kind={ev.kind} />
              </span>
              <div className='flex items-baseline justify-between gap-2'>
                <span>{t(`timelineKind.${ev.kind}`)}</span>
                <span className='shrink-0 whitespace-nowrap text-xs tabular-nums text-muted-foreground'>
                  {ago(ev.minAgo)}
                </span>
              </div>
              {ev.note ? <div className='mt-0.5 text-xs text-muted-foreground'>{ev.note}</div> : null}
            </li>
          ))}
        </ol>
      </div>

      <div className='space-y-2'>
        <span className='text-sm font-medium'>{t('deliveries')}</span>
        {alert.deliveries.length === 0 ? (
          <p className='rounded-lg border border-dashed border-border/60 px-3 py-2.5 text-xs text-muted-foreground'>
            {t('deliveriesEmpty')}
          </p>
        ) : (
          <ul className='space-y-1.5'>
            {alert.deliveries.map((d, i) => (
              <li
                key={i}
                className='flex items-center justify-between gap-2 rounded-lg border border-border/60 px-3 py-2 text-sm'
              >
                <span className='truncate font-mono text-xs'>{d.channel}</span>
                <span className='flex shrink-0 items-center gap-2'>
                  <Badge
                    size='sm'
                    tone={d.status === 'success' ? 'success' : d.status === 'failed' ? 'danger' : 'neutral'}
                    outline
                  >
                    {t(`delivery.${d.status}`)}
                  </Badge>
                  <span className='whitespace-nowrap text-xs tabular-nums text-muted-foreground'>
                    {ago(d.minAgo)}
                  </span>
                </span>
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}

function TimelineDot({kind}: {kind: 'triggered' | 'notified' | 'acknowledged' | 'resolved'}) {
  const cls = {
    triggered: 'bg-destructive',
    notified: 'bg-(--info, var(--accent))',
    acknowledged: 'bg-(--warning)',
    resolved: 'bg-(--success)',
  }[kind];
  return <span className={cn('size-2 rounded-full', cls)} aria-hidden />;
}
