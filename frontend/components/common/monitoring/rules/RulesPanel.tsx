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
 * Rules panel: cluster-level rules list + inline detail panel, and the job-level view.
 * 规则面板：集群级规则列表 + 内联详情侧栏，以及作业级视图。
 * 行内开关/批量操作直接整层 PUT 全局层；默认通知缺失时顶部 Arc alert 提示并跳转通知页。
 */
import {useCallback, useEffect, useMemo, useState} from 'react';
import {usePathname, useRouter, useSearchParams} from 'next/navigation';
import {useLocale, useTranslations} from 'next-intl';
import {ChevronRight, Undo2, X} from 'lucide-react';
import {Alert} from '@/components/arc/alert/alert';
import {Badge} from '@/components/arc/badge/badge';
import {ChipGroup} from '@/components/arc/chip-group/chip-group';
import {Drawer, DrawerContent, useDrawerItem} from '@/components/arc/drawer/drawer';
import {SearchField} from '@/components/arc/search-field/search-field';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {Skeleton} from '@/components/arc/skeleton/skeleton';
import {Switch} from '@/components/arc/switch/switch';
import {Button} from '@/components/ui/button';
import {Checkbox} from '@/components/ui/checkbox';
import services from '@/lib/services';
import type {
  AlertNotificationDefaults,
  ClusterRule,
  ClusterRuleLayerRequest,
} from '@/lib/services/monitoring';
import {cn} from '@/lib/utils';
import {formatRuleThreshold, severityTone} from '../monitoring-utils';
import {RuleDetailPanel} from './RuleDetailPanel';
import {JobRulesPanel} from './JobRulesPanel';

export interface RulesPanelProps {
  /** 深链初始选中规则。Deep-linked rule key to open on mount. */
  initialActiveKey?: string;
  onConfigureNotify?: () => void;
}

/** 整层替换 PUT 的体：raw 层字段原样回传，调用方只覆盖要改的字段。 / Whole-layer PUT body builder. */
export function layerBody(
  raw: {
    enabled: boolean | null;
    threshold: number | null;
    for_seconds: number | null;
    severity: string | null;
    notify_mode: string | null;
    channel_ids: number[];
    receiver_user_ids: number[];
  },
  patch: Partial<ClusterRuleLayerRequest>,
): ClusterRuleLayerRequest {
  return {
    enabled: raw.enabled,
    threshold: raw.threshold,
    for_seconds: raw.for_seconds,
    severity: (raw.severity as ClusterRuleLayerRequest['severity']) ?? null,
    notify_mode:
      (raw.notify_mode as ClusterRuleLayerRequest['notify_mode']) ?? null,
    channel_ids: raw.channel_ids?.length ? raw.channel_ids : null,
    receiver_user_ids: raw.receiver_user_ids?.length ? raw.receiver_user_ids : null,
    ...patch,
  };
}

export function RulesPanel({initialActiveKey, onConfigureNotify}: RulesPanelProps) {
  const t = useTranslations('monitoring');
  const locale = useLocale();
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const [scope, setScope] = useState('cluster');
  const [chips, setChips] = useState<string[]>(['all']);
  const [query, setQuery] = useState('');
  const [activeKey, setActiveKey] = useState<string | null>(initialActiveKey ?? null);
  // 关闭拦截三态：dirty 由抽屉内容上报；armed 时 footer 显示确认行；
  // pendingKey 记住放弃后要跳转的目标行（null = 关闭）。
  // Dirty-close interception: dirty is reported by the drawer body; armed swaps
  // the footer for a confirm row; pendingKey remembers where to land on discard.
  const [ruleDirty, setRuleDirty] = useState(false);
  const [discardArmed, setDiscardArmed] = useState(false);
  const [pendingKey, setPendingKey] = useState<string | null | 'close'>(null);
  const view = searchParams.get('view') === 'grid' ? 'grid' : 'list';
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [rules, setRules] = useState<ClusterRule[]>([]);
  const [defaults, setDefaults] = useState<AlertNotificationDefaults | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [toggling, setToggling] = useState<Set<string>>(new Set());
  const [undo, setUndo] = useState<{key: string; raw: ClusterRuleLayerRequest}[] | null>(null);
  const [batchBusy, setBatchBusy] = useState(false);

  const load = useCallback(async () => {
    const [ruleData, defData] = await Promise.all([
      services.monitoring.listClusterRules(),
      services.monitoring.getAlertNotificationDefaults().catch(() => null),
    ]);
    setRules(Array.isArray(ruleData) ? ruleData : []);
    setDefaults(defData);
    setLoading(false);
  }, []);

  useEffect(() => {
    void load().catch((err) => {
      setError(err instanceof Error ? err.message : String(err));
      setLoading(false);
    });
  }, [load]);

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    return rules.filter((r) => {
      if (chips.includes('enabled') && !r.enabled) {return false;}
      if (chips.includes('disabled') && r.enabled) {return false;}
      if (chips.includes('exceptions') && r.override_count === 0) {return false;}
      const name = locale === 'en' ? r.name_en || r.name : r.name;
      if (q && !name.toLowerCase().includes(q) && !r.key.toLowerCase().includes(q)) {return false;}
      return true;
    });
  }, [rules, chips, query, locale]);

  const filteredCount = useMemo(
    () => ({
      all: rules.length,
      enabled: rules.filter((r) => r.enabled).length,
      disabled: rules.filter((r) => !r.enabled).length,
      exceptions: rules.filter((r) => r.override_count > 0).length,
    }),
    [rules],
  );

  const active = rules.find((r) => r.key === activeKey) ?? null;
  // 关闭动画期间保留最后打开的 rules，抽屉滑出时内容不先消失。
  // Keep the last opened rule mounted while the drawer slides out.
  const drawerRule = useDrawerItem(active);

  // 有未保存草稿时先拦截：抽屉保持打开并在 footer 露出确认行；
  // 放弃修改才真正落地 pendingKey（换行或关闭）。
  // Dirty drafts intercept close/row-switch first: the drawer stays open with a
  // footer confirm row; only discard commits the pending target.
  const requestKey = useCallback(
    (key: string | null) => {
      if (key === activeKey) {return;}
      if (ruleDirty && activeKey != null) {
        setPendingKey(key);
        setDiscardArmed(true);
        return;
      }
      setActiveKey(key);
      setRuleDirty(false);
      setDiscardArmed(false);
      setPendingKey(null);
    },
    [activeKey, ruleDirty],
  );

  const commitPending = useCallback(() => {
    setActiveKey(pendingKey === 'close' ? null : pendingKey);
    setRuleDirty(false);
    setDiscardArmed(false);
    setPendingKey(null);
  }, [pendingKey]);

  useEffect(() => {
    if (!ruleDirty) {
      setDiscardArmed(false);
      setPendingKey(null);
    }
  }, [ruleDirty]);

  const setView = useCallback(
    (v: string) => {
      const params = new URLSearchParams(searchParams.toString());
      if (v === 'list') {params.delete('view');} else {params.set('view', v);}
      router.replace(`${pathname}?${params.toString()}`, {scroll: false});
    },
    [pathname, router, searchParams],
  );

  const toggleSelect = (key: string, on: boolean) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (on) {next.add(key);}
      else {next.delete(key);}
      return next;
    });
  };

  /** 行内即时开关：整层 PUT，失败回滚并在行内提示。 Inline enable toggle with rollback. */
  const toggleEnabled = useCallback(
    async (rule: ClusterRule, on: boolean) => {
      if (toggling.has(rule.key)) {return;}
      setToggling((s) => new Set(s).add(rule.key));
      setError(null);
      const prev = rule.enabled;
      setRules((rs) => rs.map((r) => (r.key === rule.key ? {...r, enabled: on} : r)));
      try {
        const updated = await services.monitoring.updateClusterRule(
          rule.key,
          layerBody(rule.global, {enabled: on}),
        );
        setRules((rs) => rs.map((r) => (r.key === rule.key ? updated : r)));
      } catch (err) {
        setRules((rs) => rs.map((r) => (r.key === rule.key ? {...r, enabled: prev} : r)));
        setError(err instanceof Error ? err.message : String(err));
      } finally {
        setToggling((s) => {
          const next = new Set(s);
          next.delete(rule.key);
          return next;
        });
      }
    },
    [toggling],
  );

  /** 批量开关：逐条 PUT 全局层；undo 保存旧层供一键还原。 Batch toggle; undo keeps prior layers. */
  const batchSet = useCallback(
    async (enabled: boolean) => {
      const targets = rules.filter((r) => selected.has(r.key) && r.enabled !== enabled);
      if (targets.length === 0) {
        setSelected(new Set());
        return;
      }
      setBatchBusy(true);
      setError(null);
      const prevLayers = targets.map((r) => ({key: r.key, raw: layerBody(r.global, {})}));
      try {
        for (const r of targets) {
          const updated = await services.monitoring.updateClusterRule(
            r.key,
            layerBody(r.global, {enabled}),
          );
          setRules((rs) => rs.map((x) => (x.key === r.key ? updated : x)));
        }
        setUndo(prevLayers);
      } catch (err) {
        setError(err instanceof Error ? err.message : String(err));
        await load().catch(() => undefined);
      } finally {
        setBatchBusy(false);
        setSelected(new Set());
      }
    },
    [rules, selected, load],
  );

  const undoBatch = useCallback(async () => {
    if (!undo) {return;}
    const layers = undo;
    setUndo(null);
    setBatchBusy(true);
    try {
      for (const {key, raw} of layers) {
        const updated = await services.monitoring.updateClusterRule(key, raw);
        setRules((rs) => rs.map((x) => (x.key === key ? updated : x)));
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBatchBusy(false);
    }
  }, [undo]);

  const applySaved = useCallback((updated: ClusterRule) => {
    setRules((rs) => rs.map((r) => (r.key === updated.key ? updated : r)));
  }, []);

  const defaultsMissing = defaults != null && !defaults.configured;

  return (
    <div className='relative flex h-full min-h-[560px] flex-col gap-3 bg-background p-4 text-foreground'>
      {defaultsMissing ? (
        <Alert tone='warning' title={t('bannerMissingDefaultNotify')}>
          <Button
            type='button'
            size='sm'
            variant='outline'
            className='mt-1 h-7 rounded-md text-xs'
            onClick={onConfigureNotify}
          >
            {t('bannerAction')}
          </Button>
        </Alert>
      ) : null}

      <div className='flex flex-wrap items-center gap-3'>
        <SegmentedControl
          label={t('panelTitle')}
          value={scope}
          onValueChange={setScope}
          options={[
            {value: 'cluster', label: t('scopeCluster')},
            {value: 'job', label: t('scopeJob')},
          ]}
        />
        {scope === 'cluster' ? (
          <>
            <ChipGroup
              label={t('panelTitle')}
              value={chips}
              onValueChange={(next) => setChips(next.length === 0 ? ['all'] : next.filter((v) => v !== 'all'))}
              options={[
                {value: 'all', label: `${t('chipAll')} ${filteredCount.all}`},
                {value: 'enabled', label: `${t('chipEnabled')} ${filteredCount.enabled}`},
                {value: 'disabled', label: `${t('chipDisabled')} ${filteredCount.disabled}`},
                {value: 'exceptions', label: `${t('chipExceptions')} ${filteredCount.exceptions}`},
              ]}
            />
            <div className='w-56 min-w-[200px] max-w-sm flex-1 [&_label]:sr-only'>
              <SearchField
                label={t('searchLabel')}
                value={query}
                onValueChange={setQuery}
                placeholder={t('searchPlaceholder')}
                clearLabel={t('searchClear')}
              />
            </div>
            <SegmentedControl
              label={t('viewModeLabel')}
              value={view}
              onValueChange={setView}
              options={[
                {value: 'list', label: t('viewList')},
                {value: 'grid', label: t('viewGrid')},
              ]}
            />
          </>
        ) : null}
      </div>

      {error ? (
        <div className='rounded-lg border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive'>
          {error}
        </div>
      ) : null}

      {scope === 'cluster' ? (
        <div className='min-h-0 flex-1 overflow-y-auto pr-1'>
          {loading ? (
            <div className='space-y-2 py-2' aria-busy>
              {Array.from({length: 8}).map((_, i) => (
                <Skeleton key={i} className='h-12 w-full' lines={1} />
              ))}
            </div>
          ) : view === 'grid' ? (
            <div className='grid grid-cols-[repeat(auto-fill,minmax(280px,1fr))] gap-3'>
              {filtered.map((rule) => (
                <RuleCard
                  key={rule.key}
                  rule={rule}
                  active={activeKey === rule.key}
                  onOpen={() => requestKey(rule.key)}
                  onToggleEnabled={(on) => void toggleEnabled(rule, on)}
                />
              ))}
            </div>
          ) : (
            <RuleListView
              rules={filtered}
              activeKey={activeKey}
              selected={selected}
              toggling={toggling}
              onOpen={requestKey}
              onSelect={toggleSelect}
              onToggleEnabled={toggleEnabled}
            />
          )}
          {!loading && filtered.length === 0 ? (
            <div className='flex min-h-[200px] items-center justify-center rounded-lg border border-dashed border-border/60 text-sm text-muted-foreground'>
              {t('emptyFiltered')}
            </div>
          ) : null}
        </div>
      ) : (
        <JobRulesPanel />
      )}

      {/* 规则详情走 Arc Drawer：表格始终占满宽度，关闭动画期间沿用最后一条规则。
          Detail lives in an Arc Drawer; the table keeps full width and the last
          rule stays mounted while the panel slides out. */}
      <Drawer open={active != null} onOpenChange={(open) => !open && requestKey(null)}>
        {drawerRule ? (
          <DrawerContent
            side='right'
            title={locale === 'en' ? drawerRule.name_en || drawerRule.name : drawerRule.name}
            // body 默认不拉伸，内容短时 footer 悬在半空；flex-1 让子面板拿到整高
            // the vendored body does not grow, so a short panel leaves the footer
            // floating mid-drawer; flex-1 hands it the full column height
            className='w-[min(92vw,460px)] rounded-none! [&>div[class*=body]]:flex-1'
          >
            <RuleDetailPanel
              key={drawerRule.key}
              rule={drawerRule}
              onClose={() => requestKey(null)}
              onSaved={applySaved}
              onToggleEnabled={toggleEnabled}
              onConfigureNotify={onConfigureNotify}
              onDirtyChange={setRuleDirty}
              discardArmed={discardArmed}
              onDiscard={commitPending}
              onKeepEditing={() => {
                setDiscardArmed(false);
                setPendingKey(null);
              }}
            />
          </DrawerContent>
        ) : null}
      </Drawer>

      {selected.size > 0 ? (
        <div className='sticky bottom-0 z-30 flex items-center gap-3 rounded-lg border border-border/70 bg-card px-4 py-2.5 shadow-lg'>
          <span className='text-sm font-medium'>{t('selected', {count: selected.size})}</span>
          <div className='ml-auto flex items-center gap-2'>
            <Button
              type='button'
              size='sm'
              className='h-7 rounded-md text-xs'
              disabled={batchBusy}
              onClick={() => void batchSet(true)}
            >
              {t('batchEnable')}
            </Button>
            <Button
              type='button'
              size='sm'
              variant='outline'
              className='h-7 rounded-md text-xs'
              disabled={batchBusy}
              onClick={() => void batchSet(false)}
            >
              {t('batchDisable')}
            </Button>
            <Button
              type='button'
              size='sm'
              variant='ghost'
              className='h-7 rounded-md text-xs'
              onClick={() => setSelected(new Set())}
            >
              <X className='mr-1 size-3.5' aria-hidden />
              {t('batchClear')}
            </Button>
          </div>
        </div>
      ) : null}

      {undo ? (
        <div className='fixed bottom-4 left-1/2 z-50 flex -translate-x-1/2 items-center gap-3 rounded-lg border border-border/70 bg-card px-4 py-2 text-sm shadow-xl'>
          <span>{t('undoApplied', {count: undo.length})}</span>
          <Button
            type='button'
            size='sm'
            variant='ghost'
            className='h-6 rounded-md px-2 text-xs'
            disabled={batchBusy}
            onClick={() => void undoBatch()}
          >
            <Undo2 className='mr-1 size-3.5' aria-hidden />
            {t('undo')}
          </Button>
        </div>
      ) : null}
    </div>
  );
}

function RuleListView({
  rules,
  activeKey,
  selected,
  toggling,
  onOpen,
  onSelect,
  onToggleEnabled,
}: {
  rules: ClusterRule[];
  activeKey: string | null;
  selected: Set<string>;
  toggling: Set<string>;
  onOpen: (key: string) => void;
  onSelect: (key: string, on: boolean) => void;
  onToggleEnabled: (rule: ClusterRule, on: boolean) => void;
}) {
  const t = useTranslations('monitoring');
  const locale = useLocale();
  const cols =
    'grid-cols-[1.25rem_minmax(0,1.4fr)_4.5rem_6rem_minmax(0,1fr)_3.5rem_4.75rem]';
  return (
    <div>
      <div
        className={cn(
          'grid items-center gap-3 border-b border-border/60 px-4 pb-2 pt-1 text-xs text-muted-foreground',
          cols,
        )}
        aria-hidden
      >
        <span className='size-4' />
        <span>{t('colRule')}</span>
        <span>{t('fieldSeverity')}</span>
        <span>{t('colStatus')}</span>
        <span className='hidden md:block'>{t('colThreshold')}</span>
        <span className='hidden text-right lg:block'>{t('colExceptions')}</span>
        <span className='text-right'>{t('fieldEnabled')}</span>
      </div>
      {rules.map((rule) => {
        const name = locale === 'en' ? rule.name_en || rule.name : rule.name;
        const description =
          locale === 'en' ? rule.description_en || rule.description : rule.description;
        return (
          <div
            key={rule.key}
            data-testid={`rule-row-${rule.key}`}
            role='button'
            tabIndex={0}
            onClick={(e) => {
              if ((e.target as HTMLElement).closest('button, input, [role="checkbox"], [role="switch"], [role="combobox"]')) {return;}
              onOpen(rule.key);
            }}
            onKeyDown={(e) => {
              if (e.target !== e.currentTarget) {return;}
              if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault();
                onOpen(rule.key);
              }
            }}
            className={cn(
              'grid cursor-pointer items-center gap-3 border-b border-border/40 px-4 py-3.5 last:border-b-0',
              'hover:bg-muted/30 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/40 focus-visible:ring-inset',
              cols,
              activeKey === rule.key && 'bg-muted/40',
              selected.has(rule.key) && 'bg-primary/[0.04]',
              !rule.enabled && 'opacity-60',
            )}
          >
            <Checkbox
              checked={selected.has(rule.key)}
              onCheckedChange={(v) => onSelect(rule.key, v === true)}
              aria-label={t('selectRule', {name})}
            />
            <div className='min-w-0'>
              <button
                type='button'
                onClick={() => onOpen(rule.key)}
                className='block w-full truncate text-left text-sm font-medium hover:underline'
              >
                {name}
              </button>
              <div className='line-clamp-1 text-xs text-muted-foreground' title={description}>
                {description}
              </div>
              <div className='truncate font-mono text-xs text-muted-foreground/70'>{rule.key}</div>
            </div>
            <Badge tone={severityTone(rule.severity)} size='sm'>
              {rule.severity === 'critical' ? t('severityCritical') : t('severityWarning')}
            </Badge>
            <span className='whitespace-nowrap text-xs text-muted-foreground'>
              {rule.enabled ? (
                rule.external_channel_missing ? (
                  <span className='flex items-center gap-1.5 text-(--warning)'>
                    <span className='size-1.5 animate-pulse rounded-full bg-(--warning)' aria-hidden />
                    {t('channelMissing')}
                  </span>
                ) : (
                  t('statusHealthy')
                )
              ) : (
                t('firingNone')
              )}
            </span>
            <span className='hidden min-w-0 truncate text-xs tabular-nums text-muted-foreground md:block'>
              {formatRuleThreshold(rule, t)}
            </span>
            <span className='hidden truncate text-right text-xs tabular-nums text-muted-foreground lg:block'>
              {rule.override_count > 0 ? rule.override_count : '—'}
            </span>
            <div className='flex items-center justify-end gap-1.5'>
              <Switch
                checked={rule.enabled}
                disabled={toggling.has(rule.key)}
                onCheckedChange={(on) => void onToggleEnabled(rule, on)}
                aria-label={t('fieldEnabled')}
                onClick={(e) => e.stopPropagation()}
              />
              <button
                type='button'
                onClick={() => onOpen(rule.key)}
                aria-label={t('openDetail')}
                className='rounded p-0.5 text-muted-foreground hover:text-foreground'
              >
                <ChevronRight className='size-4' aria-hidden />
              </button>
            </div>
          </div>
        );
      })}
    </div>
  );
}

function RuleCard({
  rule,
  active,
  onOpen,
  onToggleEnabled,
}: {
  rule: ClusterRule;
  active: boolean;
  onOpen: () => void;
  onToggleEnabled: (on: boolean) => void;
}) {
  const t = useTranslations('monitoring');
  const locale = useLocale();
  const name = locale === 'en' ? rule.name_en || rule.name : rule.name;
  const description = locale === 'en' ? rule.description_en || rule.description : rule.description;
  return (
    <div
      role='button'
      tabIndex={0}
      data-testid={`rule-card-${rule.key}`}
      onClick={(e) => {
        if ((e.target as HTMLElement).closest('button, input, [role="switch"]')) {return;}
        onOpen();
      }}
      onKeyDown={(e) => {
        if (e.target !== e.currentTarget) {return;}
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault();
          onOpen();
        }
      }}
      className={cn(
        'group flex cursor-pointer flex-col gap-2 rounded-lg border border-border/70 bg-card px-3.5 py-3 transition-shadow hover:shadow-md',
        'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/40',
        active && 'border-primary/60 ring-1 ring-primary/30',
        !rule.enabled && 'opacity-70',
      )}
    >
      <div className='flex items-start justify-between gap-2'>
        <span className='min-w-0 flex-1 truncate text-sm font-medium'>{name}</span>
        <Badge tone={severityTone(rule.severity)} size='sm' className='shrink-0'>
          {rule.severity === 'critical' ? t('severityCritical') : t('severityWarning')}
        </Badge>
      </div>
      <p className='line-clamp-2 min-h-8 text-xs text-muted-foreground' title={description}>
        {description}
      </p>
      <div className='flex items-center justify-between gap-2 text-xs text-muted-foreground'>
        <span className='min-w-0 truncate tabular-nums'>
          {formatRuleThreshold(rule, t)}
          {rule.override_count > 0 ? ` · ${t('exceptions', {count: rule.override_count})}` : ''}
        </span>
        <Switch
          checked={rule.enabled}
          onCheckedChange={onToggleEnabled}
          onClick={(e) => e.stopPropagation()}
          aria-label={t('fieldEnabled')}
          className='shrink-0'
        />
      </div>
    </div>
  );
}
