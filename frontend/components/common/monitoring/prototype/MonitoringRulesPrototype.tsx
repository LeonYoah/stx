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
 * Monitoring rules page prototype: card grid + persistent detail panel.
 * 监控规则页原型：「图片网格」式规则卡 + 宽屏常驻详情面板 / 窄屏抽屉（主从布局方案 A）。
 */
import {useMemo, useState} from 'react';
import {useLocale, useTranslations} from 'next-intl';
import {AlertTriangle, Check, ChevronRight, Undo2, X} from 'lucide-react';
import {Badge} from '@/components/arc/badge/badge';
import {ChipGroup} from '@/components/arc/chip-group/chip-group';
import {NumberField} from '@/components/arc/number-field/number-field';
import {SearchField} from '@/components/arc/search-field/search-field';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {Select} from '@/components/arc/select/select';
import {Switch} from '@/components/arc/switch/switch';
import {RadioCards} from '@/components/arc/radio-cards/radio-cards';
import {Button} from '@/components/ui/button';
import {Checkbox} from '@/components/ui/checkbox';
import {cn} from '@/lib/utils';
import {RuleVisual, useRuleVisualLabels} from './RuleVisual';
import {formatThreshold} from './threshold-format';
import {
  PROTO_CLUSTERS,
  PROTO_JOB_RULES,
  RULE_NAME_EN,
  type ProtoRule,
} from './rules-mock';

interface MonitoringRulesPrototypeProps {
  rules: ProtoRule[];
  /** 初始选中，用于展示详情面板。Start with a selected rule to demo the detail panel. */
  initialActiveKey?: string;
  initialSelectedKeys?: string[];
  /** 模拟默认通知未配置。Pretend default notification is missing to show the banner. */
  defaultNotifyMissing?: boolean;
}

export function MonitoringRulesPrototype({
  rules,
  initialActiveKey,
  initialSelectedKeys = [],
  defaultNotifyMissing = false,
}: MonitoringRulesPrototypeProps) {
  const t = useTranslations('monitoringProto');
  const [scope, setScope] = useState('cluster');
  const [chips, setChips] = useState<string[]>(['all']);
  const [query, setQuery] = useState('');
  const [viewMode, setViewMode] = useState<'grid' | 'list'>('list');
  const [activeKey, setActiveKey] = useState<string | null>(initialActiveKey ?? null);
  const [selected, setSelected] = useState<Set<string>>(new Set(initialSelectedKeys));
  const [enabledMap, setEnabledMap] = useState<Record<string, boolean>>(() =>
    Object.fromEntries(rules.map((r) => [r.key, r.enabled])),
  );
  const [undo, setUndo] = useState<{keys: string[]; prev: boolean} | null>(null);

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    return rules.filter((r) => {
      const enabled = enabledMap[r.key] ?? r.enabled;
      if (chips.includes('enabled') && !enabled) {return false;}
      if (chips.includes('disabled') && enabled) {return false;}
      if (chips.includes('exceptions') && r.exceptionCount === 0) {return false;}
      if (q && !r.name.toLowerCase().includes(q) && !r.key.toLowerCase().includes(q)) {return false;}
      return true;
    });
  }, [rules, chips, query, enabledMap]);

  const filteredCount = useMemo(
    () => ({
      all: rules.length,
      enabled: rules.filter((r) => enabledMap[r.key] ?? r.enabled).length,
      disabled: rules.filter((r) => !(enabledMap[r.key] ?? r.enabled)).length,
      exceptions: rules.filter((r) => r.exceptionCount > 0).length,
    }),
    [rules, enabledMap],
  );

  const active = rules.find((r) => r.key === activeKey) ?? null;

  const toggleSelect = (key: string, on: boolean) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (on) {next.add(key);}
      else {next.delete(key);}
      return next;
    });
  };

  const batchSet = (enabled: boolean) => {
    const keys = [...selected];
    setUndo({keys, prev: enabledMap[keys[0]] ?? false});
    setEnabledMap((m) => ({...m, ...Object.fromEntries(keys.map((k) => [k, enabled]))}));
    setSelected(new Set());
  };

  return (
    <div className='relative flex h-full min-h-[560px] flex-col gap-3 bg-background p-4 text-foreground'>
      {defaultNotifyMissing ? (
        <div
          role='status'
          className='flex items-center gap-3 rounded-lg border border-(--warning)/40 bg-(--warning)/10 px-4 py-2.5 text-sm'
        >
          <AlertTriangle className='size-4 shrink-0 text-(--warning)' aria-hidden />
          <span className='min-w-0 flex-1 text-foreground/90'>{t('bannerMissingDefaultNotify')}</span>
          <Button type='button' size='sm' variant='outline' className='h-7 rounded-md text-xs'>
            {t('bannerAction')}
          </Button>
        </div>
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
              value={viewMode}
              onValueChange={(v) => setViewMode(v as 'grid' | 'list')}
              options={[
                {value: 'list', label: t('viewList')},
                {value: 'grid', label: t('viewGrid')},
              ]}
            />
          </>
        ) : null}
      </div>

      {scope === 'cluster' ? (
        <div className='flex min-h-0 flex-1 gap-4'>
          <div className='min-w-0 flex-1 overflow-y-auto pr-1'>
            {viewMode === 'grid' ? (
              <div className='grid grid-cols-[repeat(auto-fill,minmax(290px,1fr))] gap-3'>
                {filtered.map((rule) => (
                  <RuleCard
                    key={rule.key}
                    rule={rule}
                    enabled={enabledMap[rule.key] ?? rule.enabled}
                    active={activeKey === rule.key}
                    selected={selected.has(rule.key)}
                    selectionActive={selected.size > 0}
                    onOpen={() => setActiveKey(rule.key)}
                    onSelect={(on) => toggleSelect(rule.key, on)}
                    onToggleEnabled={(on) => setEnabledMap((m) => ({...m, [rule.key]: on}))}
                  />
                ))}
              </div>
            ) : (
              <RuleListView
                rules={filtered}
                enabledMap={enabledMap}
                activeKey={activeKey}
                selected={selected}
                onOpen={setActiveKey}
                onSelect={toggleSelect}
                onToggleEnabled={(key, on) => setEnabledMap((m) => ({...m, [key]: on}))}
              />
            )}
            {filtered.length === 0 ? (
              <div className='flex min-h-[200px] items-center justify-center rounded-lg border border-dashed border-border/60 text-sm text-muted-foreground'>
                {t('emptyFiltered')}
              </div>
            ) : null}
          </div>

          {active ? (
            <>
              <button
                type='button'
                aria-label={t('closeDetail')}
                className='fixed inset-0 z-40 cursor-default bg-black/40 lg:hidden'
                onClick={() => setActiveKey(null)}
              />
              <RuleDetailPanel
                key={active.key}
                rule={active}
                enabled={enabledMap[active.key] ?? active.enabled}
                onClose={() => setActiveKey(null)}
                onToggleEnabled={(on) => setEnabledMap((m) => ({...m, [active.key]: on}))}
              />
            </>
          ) : null}
        </div>
      ) : (
        <JobRulesStub />
      )}

      {selected.size > 0 ? (
        <div className='sticky bottom-0 z-30 flex items-center gap-3 rounded-lg border border-border/70 bg-card px-4 py-2.5 shadow-lg'>
          <span className='text-sm font-medium'>{t('selected', {count: selected.size})}</span>
          <div className='ml-auto flex items-center gap-2'>
            <Button type='button' size='sm' className='h-7 rounded-md text-xs' onClick={() => batchSet(true)}>
              {t('batchEnable')}
            </Button>
            <Button type='button' size='sm' variant='outline' className='h-7 rounded-md text-xs' onClick={() => batchSet(false)}>
              {t('batchDisable')}
            </Button>
            <Button type='button' size='sm' variant='ghost' className='h-7 rounded-md text-xs' onClick={() => setSelected(new Set())}>
              <X className='mr-1 size-3.5' aria-hidden />
              {t('batchClear')}
            </Button>
          </div>
        </div>
      ) : null}

      {undo ? (
        <div className='fixed bottom-4 left-1/2 z-50 flex -translate-x-1/2 items-center gap-3 rounded-lg border border-border/70 bg-card px-4 py-2 text-sm shadow-xl'>
          <span>{t('undoApplied', {count: undo.keys.length})}</span>
          <Button
            type='button'
            size='sm'
            variant='ghost'
            className='h-6 rounded-md px-2 text-xs'
            onClick={() => {
              setEnabledMap((m) => ({
                ...m,
                ...Object.fromEntries(undo.keys.map((k) => [k, undo.prev])),
              }));
              setUndo(null);
            }}
          >
            <Undo2 className='mr-1 size-3.5' aria-hidden />
            {t('undo')}
          </Button>
        </div>
      ) : null}
    </div>
  );
}

function RuleCard({
  rule,
  enabled,
  active,
  selected,
  selectionActive,
  onOpen,
  onSelect,
  onToggleEnabled,
}: {
  rule: ProtoRule;
  enabled: boolean;
  active: boolean;
  selected: boolean;
  selectionActive: boolean;
  onOpen: () => void;
  onSelect: (on: boolean) => void;
  onToggleEnabled: (on: boolean) => void;
}) {
  const t = useTranslations('monitoringProto');
  const locale = useLocale();
  const visualLabels = useRuleVisualLabels();
  const name = locale === 'en' ? (RULE_NAME_EN[rule.key] ?? rule.name) : rule.name;

  return (
    <div
      data-testid={`proto-rule-card-${rule.key}`}
      data-active={active || undefined}
      className={cn(
        'group relative overflow-hidden rounded-lg border bg-card text-left transition-shadow',
        'hover:shadow-md',
        active ? 'border-primary/60 shadow-sm ring-1 ring-primary/30' : 'border-border/70',
        selected && 'border-primary/50 bg-primary/[0.04]',
        !enabled && 'opacity-70',
      )}
    >
      <button
        type='button'
        onClick={onOpen}
        aria-label={name}
        className='block h-[104px] w-full border-b border-border/50 bg-muted/20'
      >
        <RuleVisual rule={rule} labels={visualLabels} showAxis={false} />
      </button>

      <div className='space-y-2 px-3.5 py-3'>
        <div className='flex items-center gap-2'>
          <Checkbox
            checked={selected}
            onCheckedChange={(v) => onSelect(v === true)}
            aria-label={t('selectRule', {name})}
            className={cn('shrink-0 transition-opacity', selected || selectionActive ? 'opacity-100' : 'opacity-0 group-hover:opacity-100 group-focus-within:opacity-100')}
          />
          <button
            type='button'
            onClick={onOpen}
            className='min-w-0 flex-1 truncate text-left text-sm font-medium hover:underline'
          >
            {name}
          </button>
          {rule.firingCount > 0 ? (
            <span className='flex shrink-0 items-center gap-1 text-xs text-destructive'>
              <span className='size-1.5 animate-pulse rounded-full bg-destructive' aria-hidden />
              {t('firing', {count: rule.firingCount})}
            </span>
          ) : null}
          <Badge tone={rule.severity === 'critical' ? 'danger' : 'warning'} size='sm' className='shrink-0'>
            {rule.severity === 'critical' ? t('severityCritical') : t('severityWarning')}
          </Badge>
        </div>
        <div className='flex items-center justify-between gap-2 text-xs text-muted-foreground'>
          <span className='min-w-0 truncate'>
            {formatThreshold(rule, t)}
            {rule.exceptionCount > 0 ? ` · ${t('exceptions', {count: rule.exceptionCount})}` : ''}
          </span>
          <Switch
            checked={enabled}
            onCheckedChange={onToggleEnabled}
            onClick={(e) => e.stopPropagation()}
            aria-label={t('fieldEnabled')}
            className='shrink-0'
          />
        </div>
      </div>
    </div>
  );
}

function RuleDetailPanel({
  rule,
  enabled,
  onClose,
  onToggleEnabled,
}: {
  rule: ProtoRule;
  enabled: boolean;
  onClose: () => void;
  onToggleEnabled: (on: boolean) => void;
}) {
  const t = useTranslations('monitoringProto');
  const locale = useLocale();
  const visualLabels = useRuleVisualLabels();
  const name = locale === 'en' ? (RULE_NAME_EN[rule.key] ?? rule.name) : rule.name;
  const [severity, setSeverity] = useState(rule.severity);
  // 「不通知」不对用户开放；历史数据兜底为默认通知
  // "none" is not a user-facing option; legacy data falls back to default
  const [notify, setNotify] = useState(rule.notifyMode === 'none' ? 'default' : rule.notifyMode);
  const [saved, setSaved] = useState(false);

  return (
    <aside
      data-testid='proto-rule-detail'
      className={cn(
        'z-50 flex w-[min(92vw,400px)] shrink-0 flex-col overflow-hidden rounded-lg border border-border/70 bg-card shadow-xl',
        'max-lg:fixed max-lg:inset-y-4 max-lg:right-4',
      )}
    >
      <div className='flex items-start justify-between gap-3 border-b border-border/60 px-4 py-3'>
        <div className='min-w-0'>
          <div className='flex items-center gap-2'>
            <h3 className='truncate text-base font-medium'>{name}</h3>
            <Badge tone={severity === 'critical' ? 'danger' : 'warning'} size='sm'>
              {severity === 'critical' ? t('severityCritical') : t('severityWarning')}
            </Badge>
          </div>
          <p className='mt-0.5 truncate font-mono text-xs text-muted-foreground'>{rule.key}</p>
        </div>
        <Button type='button' variant='ghost' size='icon' className='size-7 shrink-0' onClick={onClose} aria-label={t('closeDetail')}>
          <X className='size-4' aria-hidden />
        </Button>
      </div>

      <div className='min-h-0 flex-1 space-y-4 overflow-y-auto px-4 py-4'>
        <div className='overflow-hidden rounded-lg border border-border/60'>
          <div className='h-[120px] border-b border-border/50 bg-muted/20'>
            <RuleVisual rule={rule} labels={visualLabels} />
          </div>
          <div className='grid grid-cols-3 divide-x divide-border/50 text-center'>
            <div className='px-2 py-2.5'>
              <div className='text-xs text-muted-foreground'>{t('colStatus')}</div>
              <div className='mt-0.5 text-sm font-medium'>
                {rule.firingCount > 0 ? t('firing', {count: rule.firingCount}) : t('firingNone')}
              </div>
            </div>
            <div className='px-2 py-2.5'>
              <div className='text-xs text-muted-foreground'>{t('detailExceptions')}</div>
              <div className='mt-0.5 text-sm font-medium'>{rule.exceptionCount}</div>
            </div>
            <div className='px-2 py-2.5'>
              <div className='text-xs text-muted-foreground'>{t('fieldNotify')}</div>
              <div className='mt-0.5 truncate text-sm font-medium'>
                {severity === 'critical' ? t(`notify_${notify}`) : t('notify_none')}
              </div>
            </div>
          </div>
        </div>

        <p className='text-sm text-muted-foreground'>{rule.description}</p>

        <div className='flex items-center justify-between rounded-lg border border-border/60 px-3 py-2.5'>
          <span className='text-sm'>{t('fieldEnabled')}</span>
          <Switch checked={enabled} onCheckedChange={onToggleEnabled} aria-label={t('fieldEnabled')} />
        </div>

        <div className='flex items-start gap-3'>
          {rule.threshold ? (
            <div className='w-40'>
              <NumberField
                label={t('fieldThreshold')}
                defaultValue={rule.threshold.value}
                min={rule.threshold.min}
                max={rule.threshold.max}
                prefix={rule.threshold.operator}
                suffix={rule.threshold.unit || undefined}
                description={t('thresholdRange', {min: rule.threshold.min, max: rule.threshold.max})}
              />
            </div>
          ) : (
            <div className='flex-1 rounded-lg border border-dashed border-border/60 px-3 py-2.5 text-xs text-muted-foreground'>
              {t('instantEvent')}
            </div>
          )}
          <Select
            label={t('fieldFor')}
            defaultValue={String(rule.forMinutes)}
            disabled={rule.engine === 'event'}
            className='w-36'
            options={[0, 1, 2, 5, 10, 15, 30].map((m) => ({
              value: String(m),
              label: m === 0 ? t('realtime') : t('forMinutes', {minutes: m}),
            }))}
          />
        </div>

        <div className='space-y-2'>
          <span className='text-sm font-medium'>{t('fieldSeverity')}</span>
          <RadioCards
            layout='list'
            value={severity}
            onValueChange={(v) => setSeverity(v as typeof severity)}
            options={[
              {value: 'warning', label: t('severityWarning'), description: t('severityWarningDesc')},
              {value: 'critical', label: t('severityCritical'), description: t('severityCriticalDesc')},
            ]}
          />
        </div>

        {severity === 'critical' ? (
          <div className='space-y-2'>
            <span className='text-sm font-medium'>{t('fieldNotify')}</span>
            <RadioCards
              layout='list'
              value={notify}
              onValueChange={(v) => setNotify(v as typeof notify)}
              options={[
                {value: 'default', label: t('notifyDefault'), description: t('notifyDefaultDesc')},
                {value: 'custom', label: t('notifyCustom'), description: t('notifyCustomDesc')},
              ]}
            />
            {notify === 'custom' ? (
              <div className='space-y-2 rounded-lg border border-dashed border-border/60 p-3 text-sm text-muted-foreground'>
                {t('channelsStub')}
              </div>
            ) : null}
          </div>
        ) : (
          <p className='rounded-lg border border-dashed border-border/60 px-3 py-2.5 text-xs text-muted-foreground'>
            {t('severityWarningDesc')}
          </p>
        )}

        <div className='space-y-2'>
          <div className='flex items-center justify-between'>
            <span className='text-sm font-medium'>{t('detailExceptions')}</span>
            <Button type='button' variant='ghost' size='sm' className='h-7 rounded-md text-xs'>
              {t('detailAddException')}
            </Button>
          </div>
          {rule.exceptions?.length ? (
            <ul className='space-y-1.5'>
              {rule.exceptions.map((ex) => (
                <li
                  key={ex.cluster}
                  className='flex items-center justify-between gap-2 rounded-lg border border-border/60 px-3 py-2 text-sm'
                >
                  <span className='truncate font-mono text-xs'>{ex.cluster}</span>
                  <span className='flex shrink-0 gap-1'>
                    {ex.overridden.map((field) => (
                      <Badge key={field} size='sm' outline>
                        {field}
                      </Badge>
                    ))}
                  </span>
                </li>
              ))}
            </ul>
          ) : (
            <p className='rounded-lg border border-dashed border-border/60 px-3 py-2.5 text-xs text-muted-foreground'>
              {t('exceptionsEmpty')}
            </p>
          )}
        </div>
      </div>

      <div className='flex items-center gap-2 border-t border-border/60 px-4 py-3'>
        <Button
          type='button'
          size='sm'
          className='h-8 rounded-md text-xs'
          onClick={() => {
            setSaved(true);
            setTimeout(() => setSaved(false), 1600);
          }}
        >
          {saved ? <Check className='mr-1 size-3.5' aria-hidden /> : null}
          {saved ? t('saved') : t('save')}
        </Button>
        <Button type='button' size='sm' variant='ghost' className='ml-auto h-8 rounded-md text-xs text-muted-foreground'>
          {t('restoreDefault')}
        </Button>
      </div>
    </aside>
  );
}

function RuleListView({
  rules,
  enabledMap,
  activeKey,
  selected,
  onOpen,
  onSelect,
  onToggleEnabled,
}: {
  rules: ProtoRule[];
  enabledMap: Record<string, boolean>;
  activeKey: string | null;
  selected: Set<string>;
  onOpen: (key: string) => void;
  onSelect: (key: string, on: boolean) => void;
  onToggleEnabled: (key: string, on: boolean) => void;
}) {
  const t = useTranslations('monitoringProto');
  const locale = useLocale();
  const visualLabels = useRuleVisualLabels();
  const cols =
    'grid-cols-[1.25rem_2.75rem_minmax(0,1.4fr)_4.5rem_6rem_minmax(0,1fr)_3.5rem_4.75rem]';
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
        <span className='size-11' />
        <span>{t('colRule')}</span>
        <span>{t('fieldSeverity')}</span>
        <span>{t('colStatus')}</span>
        <span className='hidden md:block'>{t('colThreshold')}</span>
        <span className='hidden text-right lg:block'>{t('detailExceptions')}</span>
        <span className='text-right'>{t('fieldEnabled')}</span>
      </div>
      {rules.map((rule) => {
        const enabled = enabledMap[rule.key] ?? rule.enabled;
        const name = locale === 'en' ? (RULE_NAME_EN[rule.key] ?? rule.name) : rule.name;
        return (
          <div
            key={rule.key}
            data-testid={`proto-rule-row-${rule.key}`}
            className={cn(
              'grid items-center gap-3 border-b border-border/40 px-4 py-3.5 last:border-b-0 hover:bg-muted/30',
              cols,
              activeKey === rule.key && 'bg-muted/40',
              selected.has(rule.key) && 'bg-primary/[0.04]',
              !enabled && 'opacity-60',
            )}
          >
            <Checkbox
              checked={selected.has(rule.key)}
              onCheckedChange={(v) => onSelect(rule.key, v === true)}
              aria-label={t('selectRule', {name})}
            />
            <button
              type='button'
              onClick={() => onOpen(rule.key)}
              aria-label={name}
              className='size-11 overflow-hidden rounded-lg border border-border/50 bg-muted/30'
            >
              <RuleVisual rule={rule} labels={visualLabels} compact />
            </button>
            <div className='min-w-0'>
              <button
                type='button'
                onClick={() => onOpen(rule.key)}
                className='block w-full truncate text-left text-sm font-medium hover:underline'
              >
                {name}
              </button>
              <div className='truncate font-mono text-xs text-muted-foreground'>{rule.key}</div>
            </div>
            <Badge tone={rule.severity === 'critical' ? 'danger' : 'warning'} size='sm'>
              {rule.severity === 'critical' ? t('severityCritical') : t('severityWarning')}
            </Badge>
            <span className='whitespace-nowrap text-xs text-muted-foreground'>
              {rule.firingCount > 0 ? (
                <span className='flex items-center gap-1.5 text-destructive'>
                  <span className='size-1.5 animate-pulse rounded-full bg-destructive' aria-hidden />
                  {t('firing', {count: rule.firingCount})}
                </span>
              ) : (
                t('firingNone')
              )}
            </span>
            <span className='hidden min-w-0 truncate text-xs tabular-nums text-muted-foreground md:block'>
              {formatThreshold(rule, t)}
            </span>
            <span className='hidden truncate text-right text-xs tabular-nums text-muted-foreground lg:block'>
              {rule.exceptionCount > 0 ? rule.exceptionCount : '—'}
            </span>
            <div className='flex items-center justify-end gap-1.5'>
              <Switch
                checked={enabled}
                onCheckedChange={(on) => onToggleEnabled(rule.key, on)}
                aria-label={t('fieldEnabled')}
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

function JobRulesStub() {
  const t = useTranslations('monitoringProto');
  return (
    <div className='min-h-0 flex-1 overflow-y-auto rounded-lg border border-border/60 bg-card/40'>
      <div className='border-b border-border/60 px-4 py-2.5 text-xs text-muted-foreground'>
        {t('jobRulesStub')}
      </div>
      {PROTO_JOB_RULES.map((row) => (
        <div
          key={row.key}
          className='flex items-center gap-3 border-b border-border/50 px-4 py-3 last:border-b-0 hover:bg-muted/30'
        >
          <div className='min-w-0 flex-1'>
            <div className='truncate font-mono text-sm'>{row.task}</div>
            <div className='mt-1 flex flex-wrap gap-1'>
              {row.rules.map((r) => (
                <Badge key={r} size='sm' outline>
                  {t(`jobRule.${r}`)}
                </Badge>
              ))}
              <Badge size='sm' tone='info' outline>
                {t(`notify_${row.notify}`)}
              </Badge>
            </div>
          </div>
          <Switch defaultChecked={row.enabled} aria-label={t('fieldEnabled')} />
          <ChevronRight className='size-4 text-muted-foreground' aria-hidden />
        </div>
      ))}
      <div className='px-4 py-2.5 text-xs text-muted-foreground/70'>
        {PROTO_CLUSTERS.length} clusters · {t('jobRulesFootnote')}
      </div>
    </div>
  );
}
