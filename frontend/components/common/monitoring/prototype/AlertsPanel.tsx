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
 * Alerts panel prototype: metric cards, filters, server-driven style list, detail drawer.
 * 告警面板原型：指标行（点击即筛选）、筛选工具栏、告警列表、详情抽屉（命中规则 / 时间线 / 投递）。
 */
import {useMemo, useState} from 'react';
import {useLocale, useTranslations} from 'next-intl';
import {
  Bell,
  BellOff,
  CheckCheck,
  ChevronRight,
  History,
  MoreHorizontal,
  Send,
  TrendingDown,
  TrendingUp,
  X,
} from 'lucide-react';
import {Badge} from '@/components/arc/badge/badge';
import {ChipGroup} from '@/components/arc/chip-group/chip-group';
import {SearchField} from '@/components/arc/search-field/search-field';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {Button} from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import {Pagination} from '@/components/ui/pagination';
import {cn} from '@/lib/utils';
import {PROTO_ALERTS, type ProtoAlert, type ProtoAlertStatus} from './alerts-mock';

const STATUS_TONE: Record<ProtoAlertStatus, 'danger' | 'info' | 'warning' | 'neutral'> = {
  firing: 'danger',
  acknowledged: 'info',
  silenced: 'warning',
  resolved: 'neutral',
};

function useTimeAgo() {
  const t = useTranslations('monitoringProto');
  return (min: number) =>
    min < 60
      ? t('agoMinutes', {value: min})
      : min < 1440
        ? t('agoHours', {value: Math.round(min / 60)})
        : t('agoDays', {value: Math.round(min / 1440)});
}

export function AlertsPanel({onOpenRule}: {onOpenRule?: (ruleKey: string) => void}) {
  const t = useTranslations('monitoringProto');
  const locale = useLocale();
  const ago = useTimeAgo();
  const [scope, setScope] = useState('all');
  const [statusChips, setStatusChips] = useState<string[]>([]);
  const [severity, setSeverity] = useState<'all' | 'warning' | 'critical'>('all');
  const [query, setQuery] = useState('');
  const [page, setPage] = useState(1);
  const [activeId, setActiveId] = useState<string | null>(null);
  const [alerts, setAlerts] = useState(PROTO_ALERTS);
  const pageSize = 8;

  const firingCount = alerts.filter((a) => a.status === 'firing').length;
  const criticalCount = alerts.filter((a) => a.status === 'firing' && a.severity === 'critical').length;
  const ackedCount = alerts.filter((a) => a.status === 'acknowledged').length;
  const newCount = alerts.filter((a) => a.startedMinAgo <= 1440).length;

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    return alerts.filter((a) => {
      if (scope !== 'all' && a.scope !== scope) {return false;}
      if (statusChips.length > 0 && !statusChips.includes(a.status)) {return false;}
      if (severity !== 'all' && a.severity !== severity) {return false;}
      const name = locale === 'en' ? a.titleEn : a.title;
      if (q && !name.toLowerCase().includes(q) && !a.ruleKey.toLowerCase().includes(q) && !a.cluster.toLowerCase().includes(q)) {
        return false;
      }
      return true;
    });
  }, [alerts, scope, statusChips, severity, query, locale]);

  const totalPages = Math.max(1, Math.ceil(filtered.length / pageSize));
  const pageItems = filtered.slice((page - 1) * pageSize, page * pageSize);
  const active = alerts.find((a) => a.id === activeId) ?? null;

  const patchStatus = (id: string, status: ProtoAlertStatus) => {
    setAlerts((prev) => prev.map((a) => (a.id === id ? {...a, status} : a)));
  };

  return (
    <div className='relative flex h-full flex-col gap-3'>
      <div className='grid grid-cols-2 divide-x divide-border/60 rounded-lg border border-border/60 bg-card sm:grid-cols-4'>
        {(
          [
            {
              label: t('metricFiring'),
              value: firingCount,
              delta: 2,
              bad: true,
              active: statusChips.length === 1 && statusChips[0] === 'firing' && severity === 'all',
              onClick: () => { setStatusChips(['firing']); setSeverity('all'); setPage(1); },
            },
            {
              label: t('metricCritical'),
              value: criticalCount,
              delta: 1,
              bad: true,
              active: severity === 'critical',
              onClick: () => { setStatusChips(['firing']); setSeverity('critical'); setPage(1); },
            },
            {
              label: t('metricAcked'),
              value: ackedCount,
              delta: -1,
              bad: false,
              active: statusChips.includes('acknowledged'),
              onClick: () => { setStatusChips(['acknowledged']); setSeverity('all'); setPage(1); },
            },
            {
              label: t('metricNew24h'),
              value: newCount,
              delta: 4,
              bad: true,
              active: false,
              onClick: () => { setStatusChips([]); setSeverity('all'); setPage(1); },
            },
          ]
        ).map((m) => (
          <button
            key={m.label}
            type='button'
            onClick={m.onClick}
            className={cn(
              'px-4 py-3 text-left transition-colors hover:bg-muted/40',
              m.active && 'bg-muted/50',
            )}
          >
            <div className='text-xs text-muted-foreground'>{m.label}</div>
            <div className='mt-1 text-2xl font-medium tabular-nums'>{m.value}</div>
            <div className='mt-0.5 flex items-center gap-1 text-xs'>
              {m.delta >= 0 ? (
                <TrendingUp className={cn('size-3', m.bad ? 'text-destructive' : 'text-(--success)')} aria-hidden />
              ) : (
                <TrendingDown className={cn('size-3', m.bad ? 'text-(--success)' : 'text-destructive')} aria-hidden />
              )}
              <span className={m.bad === (m.delta >= 0) ? 'text-destructive' : 'text-(--success)'}>
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
        <ChipGroup
          label={t('alertStatusLabel')}
          value={statusChips}
          onValueChange={(v) => { setStatusChips(v); setPage(1); }}
          options={[
            {value: 'firing', label: t('status.firing')},
            {value: 'acknowledged', label: t('status.acknowledged')},
            {value: 'silenced', label: t('status.silenced')},
            {value: 'resolved', label: t('status.resolved')},
          ]}
        />
        <select
          value={severity}
          onChange={(e) => { setSeverity(e.target.value as typeof severity); setPage(1); }}
          className='h-8 rounded-md border border-border/60 bg-background px-2 text-sm'
          aria-label={t('fieldSeverity')}
        >
          <option value='all'>{t('severityAll')}</option>
          <option value='warning'>{t('severityWarning')}</option>
          <option value='critical'>{t('severityCritical')}</option>
        </select>
        <div className='ml-auto w-56'>
          <SearchField label={t('alertSearchLabel')} value={query} onValueChange={(v) => { setQuery(v); setPage(1); }} placeholder={t('alertSearchPlaceholder')} />
        </div>
      </div>

      <div className='min-h-0 flex-1 overflow-y-auto'>
        <div
          className='grid grid-cols-[auto_minmax(0,1.4fr)_auto_auto_minmax(0,1fr)_auto_auto] items-center gap-3 border-b border-border/60 px-4 pb-2 pt-1 text-xs text-muted-foreground'
          aria-hidden
        >
          <span className='size-2' />
          <span>{t('colAlert')}</span>
          <span>{t('fieldSeverity')}</span>
          <span>{t('colScope')}</span>
          <span>{t('colCluster')}</span>
          <span>{t('colTime')}</span>
          <span>{t('colStatus')}</span>
        </div>
        {pageItems.map((alert) => (
          <AlertRow
            key={alert.id}
            alert={alert}
            ago={ago}
            onOpen={() => setActiveId(alert.id)}
            onAck={() => patchStatus(alert.id, 'acknowledged')}
            onSilence={() => patchStatus(alert.id, 'silenced')}
            onClose={() => patchStatus(alert.id, 'resolved')}
          />
        ))}
        {pageItems.length === 0 ? (
          <div className='flex min-h-[200px] flex-col items-center justify-center gap-2 text-sm text-muted-foreground'>
            <CheckCheck className='size-6 opacity-50' aria-hidden />
            {t('alertsEmpty')}
          </div>
        ) : null}
      </div>

      <Pagination
        currentPage={page}
        totalPages={totalPages}
        pageSize={pageSize}
        totalItems={filtered.length}
        onPageChange={setPage}
        showPageSizeSelector={false}
      />

      {active ? (
        <>
          <button type='button' aria-label={t('closeDetail')} className='fixed inset-0 z-40 cursor-default bg-foreground/20' onClick={() => setActiveId(null)} />
          <AlertDetail alert={active} ago={ago} onClose={() => setActiveId(null)} onOpenRule={onOpenRule} onAck={() => patchStatus(active.id, 'acknowledged')} onSilence={() => patchStatus(active.id, 'silenced')} />
        </>
      ) : null}
    </div>
  );
}

function AlertRow({
  alert,
  ago,
  onOpen,
  onAck,
  onSilence,
  onClose,
}: {
  alert: ProtoAlert;
  ago: (m: number) => string;
  onOpen: () => void;
  onAck: () => void;
  onSilence: () => void;
  onClose: () => void;
}) {
  const t = useTranslations('monitoringProto');
  const locale = useLocale();
  const name = locale === 'en' ? alert.titleEn : alert.title;

  return (
    <div
      role='button'
      tabIndex={0}
      onClick={onOpen}
      onKeyDown={(e) => {
        if (e.key === 'Enter') {onOpen();}
      }}
      className={cn(
        'grid cursor-pointer grid-cols-[auto_minmax(0,1.4fr)_auto_auto_minmax(0,1fr)_auto_auto] items-center gap-3 border-b border-border/40 px-4 py-3.5 last:border-b-0 hover:bg-muted/30',
        alert.status === 'firing' && 'bg-destructive/[0.03]',
      )}
      data-testid={`proto-alert-row-${alert.id}`}
    >
      <span
        className={cn('size-2 rounded-full', {
          'animate-pulse bg-destructive': alert.status === 'firing',
          'bg-sky-500': alert.status === 'acknowledged',
          'bg-amber-500': alert.status === 'silenced',
          'bg-muted-foreground/40': alert.status === 'resolved',
        })}
        aria-hidden
      />
      <div className='min-w-0'>
        <div className='truncate text-sm font-medium'>{name}</div>
        <div className='truncate font-mono text-xs text-muted-foreground'>{alert.ruleKey}</div>
      </div>
      <Badge tone={alert.severity === 'critical' ? 'danger' : 'warning'} size='sm'>
        {alert.severity === 'critical' ? t('severityCritical') : t('severityWarning')}
      </Badge>
      <Badge size='sm' outline>
        {alert.scope === 'cluster' ? t('scopeCluster') : t('scopeJob')}
      </Badge>
      <div className='min-w-0 truncate text-xs text-muted-foreground'>{alert.cluster}</div>
      <span className='whitespace-nowrap text-xs tabular-nums text-muted-foreground'>{ago(alert.startedMinAgo)}</span>
      <div className='flex items-center gap-1'>
        <Badge tone={STATUS_TONE[alert.status]} size='sm' outline>
          {t(`status.${alert.status}`)}
        </Badge>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button type='button' variant='ghost' size='icon' className='size-7' onClick={(e) => e.stopPropagation()} aria-label={t('moreActions')}>
              <MoreHorizontal className='size-4' aria-hidden />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align='end' onClick={(e) => e.stopPropagation()}>
            <DropdownMenuItem disabled={alert.status !== 'firing'} onClick={onAck}>
              <CheckCheck className='mr-2 size-4' aria-hidden />
              {t('actionAck')}
            </DropdownMenuItem>
            <DropdownMenuItem disabled={alert.status !== 'firing'} onClick={onSilence}>
              <BellOff className='mr-2 size-4' aria-hidden />
              {t('actionSilence')}
            </DropdownMenuItem>
            <DropdownMenuItem disabled={alert.status === 'resolved'} onClick={onClose}>
              <X className='mr-2 size-4' aria-hidden />
              {t('actionClose')}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
        <ChevronRight className='size-4 text-muted-foreground' aria-hidden />
      </div>
    </div>
  );
}

function AlertDetail({
  alert,
  ago,
  onClose,
  onOpenRule,
  onAck,
  onSilence,
}: {
  alert: ProtoAlert;
  ago: (m: number) => string;
  onClose: () => void;
  onOpenRule?: (ruleKey: string) => void;
  onAck: () => void;
  onSilence: () => void;
}) {
  const t = useTranslations('monitoringProto');
  const locale = useLocale();
  const name = locale === 'en' ? alert.titleEn : alert.title;
  const timelineIcon = {triggered: Bell, notified: Send, acknowledged: CheckCheck, resolved: History} as const;

  return (
    <aside
      data-testid='proto-alert-detail'
      className='fixed inset-y-4 right-4 z-50 flex w-[min(92vw,420px)] flex-col overflow-hidden rounded-lg border border-border/70 bg-card shadow-xl'
    >
      <div className='flex items-start justify-between gap-3 border-b border-border/60 px-4 py-3'>
        <div className='min-w-0'>
          <div className='flex flex-wrap items-center gap-2'>
            <h3 className='truncate text-base font-medium'>{name}</h3>
            <Badge tone={alert.severity === 'critical' ? 'danger' : 'warning'} size='sm'>
              {alert.severity === 'critical' ? t('severityCritical') : t('severityWarning')}
            </Badge>
            <Badge tone={STATUS_TONE[alert.status]} size='sm' outline>
              {t(`status.${alert.status}`)}
            </Badge>
          </div>
          <p className='mt-0.5 truncate text-xs text-muted-foreground'>{alert.cluster}</p>
        </div>
        <Button type='button' variant='ghost' size='icon' className='size-7 shrink-0' onClick={onClose} aria-label={t('closeDetail')}>
          <X className='size-4' aria-hidden />
        </Button>
      </div>

      <div className='min-h-0 flex-1 space-y-4 overflow-y-auto px-4 py-4'>
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
            {' · '}
            {alert.thresholdText}
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
            {alert.timeline.map((ev, i) => {
              const Icon = timelineIcon[ev.kind];
              return (
                <li key={i} className='relative text-sm'>
                  <span className='absolute -left-[21px] top-0.5 flex size-3.5 items-center justify-center rounded-full bg-card'>
                    <Icon className='size-3 text-muted-foreground' aria-hidden />
                  </span>
                  <div className='flex items-baseline justify-between gap-2'>
                    <span>{t(`timelineKind.${ev.kind}`)}</span>
                    <span className='shrink-0 text-xs tabular-nums text-muted-foreground'>{ago(ev.minAgo)}</span>
                  </div>
                  {ev.note ? <div className='mt-0.5 text-xs text-muted-foreground'>{ev.note}</div> : null}
                </li>
              );
            })}
          </ol>
        </div>

        <div className='space-y-2'>
          <span className='text-sm font-medium'>{t('deliveries')}</span>
          {alert.deliveries.length === 0 ? (
            <p className='rounded-lg border border-dashed border-border/60 px-3 py-2.5 text-xs text-muted-foreground'>{t('deliveriesEmpty')}</p>
          ) : (
            <ul className='space-y-1.5'>
              {alert.deliveries.map((d, i) => (
                <li key={i} className='flex items-center justify-between gap-2 rounded-lg border border-border/60 px-3 py-2 text-sm'>
                  <span className='truncate font-mono text-xs'>{d.channel}</span>
                  <span className='flex shrink-0 items-center gap-2'>
                    <Badge size='sm' tone={d.status === 'success' ? 'success' : d.status === 'failed' ? 'danger' : 'neutral'} outline>
                      {t(`delivery.${d.status}`)}
                    </Badge>
                    <span className='text-xs tabular-nums text-muted-foreground'>{ago(d.minAgo)}</span>
                  </span>
                </li>
              ))}
            </ul>
          )}
        </div>
      </div>
    </aside>
  );
}
