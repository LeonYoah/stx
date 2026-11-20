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
 * 自动巡检策略表单体：detailCol 两段式（滚动表单 + 底部操作栏），常驻详情栏、
 * 窄屏抽屉与新建弹窗共用。保存校验沿用旧 AutoPolicyConfigPanel：名称必填、
 * 开启自动诊断包时至少选一项资源；规则选择器保持集群级 / 作业级分组与失效键可移除。
 * Auto-inspection policy form body: a two-part detail column (scrolling form +
 * pinned action bar) shared by the persistent rail, the narrow drawer, and the
 * centered create dialog. Save validation is migrated as-is from the old
 * AutoPolicyConfigPanel (name required, at least one resource when the auto
 * bundle is enabled); the rule picker keeps the cluster/job grouping and the
 * removable dead-key badge.
 */
import {useCallback, useEffect, useMemo, useState} from 'react';
import {useTranslations} from 'next-intl';
import {Trash2, X} from 'lucide-react';
import {toast} from '@/lib/toast';
import services from '@/lib/services';
import {cn} from '@/lib/utils';
import {
  isSoleDefaultCluster,
  rememberPreferredClusterId,
} from '@/lib/cluster-preference';
import type {
  InspectionAutoPolicy,
  InspectionConditionItem,
  InspectionConditionTemplate,
  DiagnosticsClusterOption,
  DiagnosticsResourceCode,
  DiagnosticsTaskOptions,
} from '@/lib/services/diagnostics';
import type {AlertRuleCatalogItem} from '@/lib/services/monitoring/types';
import {Badge} from '@/components/arc/badge/badge';
import {Button} from '@/components/arc/button/button';
import {Checkbox} from '@/components/arc/checkbox/checkbox';
import ConfirmMorph from '@/components/arc/confirm-morph/confirm-morph';
import {Input} from '@/components/arc/input/input';
import {NumberField} from '@/components/arc/number-field/number-field';
import {Select} from '@/components/arc/select/select';
import {Switch} from '@/components/arc/switch/switch';
import {
  CronExpressionEditor,
  type CronPresetOption,
} from '@/components/common/schedule/CronExpressionEditor';
import {HelpHint} from '@/components/common/HelpHint';
import {DiagnosticResourceSelector} from '../DiagnosticResourceSelector';
import {
  applyConditionTextOverride,
  normalizeAutoPolicyTaskOptions,
  normalizeConditionItemsForSave,
  shouldRenderCronExprInput,
} from './policy-form-utils';
import layout from '@/components/common/layout/list-detail.module.css';

const DEFAULT_TASK_OPTIONS: DiagnosticsTaskOptions = {
  include_thread_dump: false,
  include_jvm_dump: false,
  jvm_dump_min_free_mb: 2048,
  selected_resources: [],
};

const CRON_PRESETS: CronPresetOption[] = [
  {key: 'cronPresetEveryDayMidnight', expr: '0 0 * * *'},
  {key: 'cronPresetEveryHour', expr: '0 * * * *'},
  {key: 'cronPresetWeekdaysNine', expr: '0 9 * * MON-FRI'},
  {key: 'cronPresetEveryFifteenMinutes', expr: '*/15 * * * *'},
  {key: 'cronPresetEveryThirtyMinutes', expr: '*/30 * * * *'},
];

/** 可序列化的表单快照，JSON 比对即脏态。Serializable form snapshot; JSON diff is the dirty signal. */
interface PolicyFormState {
  name: string;
  clusterId: number;
  enabled: boolean;
  cooldownMinutes: number;
  conditions: InspectionConditionItem[];
  autoCreateTask: boolean;
  autoStartTask: boolean;
  taskOptions: DiagnosticsTaskOptions;
}

function formFromPolicy(
  policy: InspectionAutoPolicy | null,
  templates: InspectionConditionTemplate[],
  defaultClusterId: number,
): PolicyFormState {
  if (!policy) {
    return {
      name: '',
      clusterId: defaultClusterId,
      enabled: true,
      cooldownMinutes: 30,
      conditions: [],
      autoCreateTask: false,
      autoStartTask: false,
      taskOptions: {...DEFAULT_TASK_OPTIONS},
    };
  }
  // 加载时丢掉模板列表里不存在的旧条件码（如已删除的 PROM_* / NODE_UNHEALTHY），
  // 下次保存即为干净数据。
  // Drop condition codes no longer in the template list (e.g. deleted PROM_* /
  // NODE_UNHEALTHY) when loading; the next save persists clean data.
  const supported = new Set(templates.map((tpl) => tpl.code));
  return {
    name: policy.name,
    clusterId: policy.cluster_id,
    enabled: policy.enabled,
    cooldownMinutes: policy.cooldown_minutes,
    conditions: (policy.conditions || []).filter((condition) =>
      supported.has(condition.template_code),
    ),
    autoCreateTask: policy.auto_create_task,
    autoStartTask: policy.auto_start_task,
    taskOptions: normalizeAutoPolicyTaskOptions(policy.task_options),
  };
}

export interface PolicyFormBodyProps {
  policy: InspectionAutoPolicy | null;
  templates: InspectionConditionTemplate[];
  ruleCatalog: AlertRuleCatalogItem[];
  clusterOptions: DiagnosticsClusterOption[];
  /** 新建策略的默认作用范围（当前所选集群）。Default scope for new policies (the selected cluster). */
  defaultClusterId: number;
  onSaved: () => void;
  onCancel: () => void;
  /** 已有策略的删除入口（详情页内承载破坏性操作）。Delete entry for an existing policy. */
  onDelete?: () => Promise<void>;
  /** 「查看触发记录」：跳到按本策略过滤的巡检记录。"View trigger records": jump to inspection records filtered by this policy. */
  onViewRecords?: () => void;
  onDirtyChange?: (dirty: boolean) => void;
}

export function PolicyFormBody({
  policy,
  templates,
  ruleCatalog,
  clusterOptions,
  defaultClusterId,
  onSaved,
  onCancel,
  onDelete,
  onViewRecords,
  onDirtyChange,
}: PolicyFormBodyProps) {
  const t = useTranslations('diagnosticsCenter.autoPolicies');
  const tMonitoring = useTranslations('monitoring');
  const tCluster = useTranslations('cluster');
  const commonT = useTranslations('common');
  const scheduleT = useTranslations('workbenchStudio');

  const base = useMemo(
    () => formFromPolicy(policy, templates, defaultClusterId),
    [policy, templates, defaultClusterId],
  );
  const [form, setForm] = useState<PolicyFormState>(() =>
    formFromPolicy(policy, templates, defaultClusterId),
  );
  const [saving, setSaving] = useState(false);

  const dirty = JSON.stringify(form) !== JSON.stringify(base);
  useEffect(() => {
    onDirtyChange?.(dirty);
    return () => onDirtyChange?.(false);
  }, [dirty, onDirtyChange]);

  const setField = useCallback(
    <K extends keyof PolicyFormState>(key: K, value: PolicyFormState[K]) => {
      setForm((f) => ({...f, [key]: value}));
    },
    [],
  );

  const handleToggleCondition = useCallback(
    (templateCode: string, checked: boolean) => {
      setForm((f) => {
        if (checked) {
          if (f.conditions.some((c) => c.template_code === templateCode)) {
            return {
              ...f,
              conditions: f.conditions.map((c) =>
                c.template_code === templateCode ? {...c, enabled: true} : c,
              ),
            };
          }
          return {
            ...f,
            conditions: [
              ...f.conditions,
              {template_code: templateCode, enabled: true},
            ],
          };
        }
        return {
          ...f,
          conditions: f.conditions.filter(
            (c) => c.template_code !== templateCode,
          ),
        };
      });
    },
    [],
  );

  const handleConditionOverride = useCallback(
    (
      templateCode: string,
      field: 'threshold_override' | 'window_minutes_override',
      value: number | null,
    ) => {
      setForm((f) => ({
        ...f,
        conditions: f.conditions.map((c) =>
          c.template_code === templateCode ? {...c, [field]: value} : c,
        ),
      }));
    },
    [],
  );

  const handleConditionTextOverride = useCallback(
    (templateCode: string, field: 'cron_expr_override', value: string) => {
      setForm((f) => ({
        ...f,
        conditions: applyConditionTextOverride(
          f.conditions,
          templateCode,
          field,
          value,
        ),
      }));
    },
    [],
  );

  const handleConditionKeywordsOverride = useCallback(
    (templateCode: string, value: string) => {
      const parsed = value
        .split(',')
        .map((item) => item.trim())
        .filter(Boolean);
      setForm((f) => ({
        ...f,
        conditions: f.conditions.map((condition) =>
          condition.template_code === templateCode
            ? {
                ...condition,
                extra_keywords: parsed.length > 0 ? parsed : undefined,
              }
            : condition,
        ),
      }));
    },
    [],
  );

  /**
   * 切换 ALERT_FIRING 条件的某个规则键选中态。
   * Toggle one rule key on an ALERT_FIRING condition.
   */
  const handleConditionRuleKeyToggle = useCallback(
    (templateCode: string, ruleKey: string, checked: boolean) => {
      setForm((f) => ({
        ...f,
        conditions: f.conditions.map((condition) => {
          if (condition.template_code !== templateCode) {
            return condition;
          }
          const current = new Set(condition.rule_keys || []);
          if (checked) {
            current.add(ruleKey);
          } else {
            current.delete(ruleKey);
          }
          return {...condition, rule_keys: [...current]};
        }),
      }));
    },
    [],
  );

  // 已选规则键 → 目录项的索引：命中显示目录名，未命中说明是目录外失效键。
  // Index of selected rule keys to catalog entries: hits render the catalog name, misses
  // mark a dead out-of-catalog key.
  const ruleCatalogByKey = useMemo(() => {
    const map = new Map<string, AlertRuleCatalogItem>();
    for (const item of ruleCatalog) {
      map.set(item.rule_key, item);
    }
    return map;
  }, [ruleCatalog]);

  const getCategoryLabel = useCallback(
    (category: string) => {
      switch (category) {
        case 'java_error':
          return t('categories.javaError');
        case 'error_rate':
          return t('categories.errorRate');
        case 'alert_firing':
          return t('categories.alertFiring');
        case 'schedule':
          return t('categories.schedule');
        default:
          return category;
      }
    },
    [t],
  );

  const scopeLabel = useMemo(() => {
    const clusterId = policy ? policy.cluster_id : form.clusterId;
    if (clusterId === 0) {
      return t('scopeGlobal');
    }
    const found = clusterOptions.find((c) => c.cluster_id === clusterId);
    return found?.cluster_name ?? t('scopeCluster', {id: clusterId});
  }, [clusterOptions, form.clusterId, policy, t]);

  const clusterSelectOptions = useMemo(
    () => [
      {value: '0', label: t('globalPolicyOption')},
      ...clusterOptions.map((cluster) => ({
        value: String(cluster.cluster_id),
        label: `${t('clusterOption', {
          name: cluster.cluster_name,
          id: cluster.cluster_id,
        })}${
          isSoleDefaultCluster(
            clusterOptions.map((item) => ({id: item.cluster_id})),
            cluster.cluster_id,
          )
            ? ` · ${tCluster('defaultBadge')}`
            : ''
        }`,
      })),
    ],
    [clusterOptions, t, tCluster],
  );

  const handleSave = useCallback(async () => {
    if (!form.name.trim()) {
      toast.error(t('nameRequired'));
      return;
    }
    if (
      form.autoCreateTask &&
      (form.taskOptions.selected_resources || []).length === 0
    ) {
      toast.error(t('resourceRequired'));
      return;
    }
    setSaving(true);
    try {
      if (policy) {
        const result = await services.diagnostics.updateAutoPolicySafe(
          policy.id,
          {
            name: form.name,
            enabled: form.enabled,
            conditions: normalizeConditionItemsForSave(form.conditions),
            cooldown_minutes: form.cooldownMinutes,
            auto_create_task: form.autoCreateTask,
            auto_start_task: form.autoStartTask,
            task_options: form.autoCreateTask ? form.taskOptions : undefined,
          },
        );
        if (!result.success) {
          toast.error(result.error || t('updateError'));
          return;
        }
        toast.success(t('updateSuccess'));
      } else {
        const result = await services.diagnostics.createAutoPolicySafe({
          cluster_id: form.clusterId,
          name: form.name,
          enabled: form.enabled,
          conditions: normalizeConditionItemsForSave(form.conditions),
          cooldown_minutes: form.cooldownMinutes,
          auto_create_task: form.autoCreateTask,
          auto_start_task: form.autoStartTask,
          task_options: form.autoCreateTask ? form.taskOptions : undefined,
        });
        if (!result.success) {
          toast.error(result.error || t('createError'));
          return;
        }
        toast.success(t('createSuccess'));
      }
      onSaved();
    } finally {
      setSaving(false);
    }
  }, [form, onSaved, policy, t]);

  const groupedTemplates = useMemo(
    () =>
      templates.reduce<Record<string, InspectionConditionTemplate[]>>(
        (acc, tpl) => {
          const key = tpl.category;
          if (!acc[key]) {
            acc[key] = [];
          }
          acc[key].push(tpl);
          return acc;
        },
        {},
      ),
    [templates],
  );

  return (
    <div className={layout.detailCol} data-testid='policy-form-body'>
      <div className={layout.detailScroll}>
        <div className={layout.field}>
          <Input
            label={t('nameLabel')}
            value={form.name}
            onChange={(e) => setField('name', e.target.value)}
            placeholder={t('namePlaceholder')}
            autoComplete='off'
          />
        </div>

        <div className={layout.field}>
          {policy ? (
            <>
              <span className={layout.fieldLabel}>{t('clusterLabel')}</span>
              <span className='text-sm'>{scopeLabel}</span>
            </>
          ) : (
            <Select
              label={t('clusterLabel')}
              value={String(form.clusterId)}
              onValueChange={(value) => {
                const next = Number.parseInt(value, 10) || 0;
                setField('clusterId', next);
                if (next > 0) {
                  rememberPreferredClusterId(next);
                }
              }}
              options={clusterSelectOptions}
              placeholder={t('clusterPlaceholder')}
            />
          )}
        </div>

        <div className={layout.field}>
          <NumberField
            label={t('cooldownLabel')}
            value={form.cooldownMinutes}
            onValueChange={(v) => setField('cooldownMinutes', v)}
            min={1}
            max={1440}
          />
        </div>

        <div className={layout.field}>
          <Switch
            label={t('enabledLabel')}
            checked={form.enabled}
            onCheckedChange={(v) => setField('enabled', v)}
          />
        </div>

        {/* 巡检触发条件（单列，适配 520px 详情栏） / Inspection conditions (single column for the 520px rail) */}
        <div className={layout.sectionStack} style={{gap: 10}}>
          <div className={layout.fieldHeadRow}>
            <span className={layout.sectionTitle}>{t('conditionsLabel')}</span>
            <span className={cn(layout.xsText, layout.muted)}>
              {t('selectedConditionsCount', {
                count: form.conditions.filter((c) => c.enabled).length,
              })}
            </span>
          </div>
          {templates.length === 0 ? (
            <span className={cn(layout.xsText, layout.muted)}>
              {t('noTemplates')}
            </span>
          ) : (
            Object.entries(groupedTemplates).map(
              ([category, categoryTemplates]) => (
                <div
                  key={category}
                  className='rounded-lg border bg-card/60 p-3 space-y-2.5 shadow-2xs'
                >
                  <div className='flex items-center justify-between border-b pb-2'>
                    <span className='text-xs font-semibold uppercase tracking-wider text-primary'>
                      {getCategoryLabel(category)}
                    </span>
                    <Badge tone='neutral' className='text-[10px]'>
                      {categoryTemplates.length}
                    </Badge>
                  </div>
                  <div className='space-y-2.5'>
                    {categoryTemplates.map((tpl) => {
                      const isChecked = form.conditions.some(
                        (c) => c.template_code === tpl.code && c.enabled,
                      );
                      const condition = form.conditions.find(
                        (c) => c.template_code === tpl.code,
                      );
                      return (
                        <div
                          key={tpl.code}
                          className={cn(
                            'space-y-2 rounded-md border p-3 transition-colors',
                            isChecked
                              ? 'border-primary/40 bg-primary/5'
                              : 'bg-muted/20 border-border/60',
                          )}
                        >
                          <Checkbox
                            id={`chk-${tpl.code}`}
                            label={tpl.name}
                            description={tpl.description}
                            checked={isChecked}
                            onCheckedChange={(checked) =>
                              handleToggleCondition(tpl.code, checked === true)
                            }
                          />

                          {isChecked && !tpl.immediate_on_match ? (
                            <div className='mt-2.5 pl-6 space-y-2 border-t border-border/40 pt-2'>
                              {shouldRenderCronExprInput(tpl) ? (
                                <div className='space-y-2 w-full'>
                                  <CronExpressionEditor
                                    expression={
                                      condition?.cron_expr_override ??
                                      tpl.default_cron_expr ??
                                      ''
                                    }
                                    onExpressionChange={(nextExpr) =>
                                      handleConditionTextOverride(
                                        tpl.code,
                                        'cron_expr_override',
                                        nextExpr,
                                      )
                                    }
                                    translator={(key, values) =>
                                      scheduleT(key as never, values as never)
                                    }
                                    labels={{
                                      cronExpression: t('cronExprLabel'),
                                      timezone: commonT('timezone'),
                                      cronFiveField: scheduleT('cronFiveField'),
                                      cronTimezonePlaceholder: scheduleT(
                                        'cronTimezonePlaceholder',
                                      ),
                                      cronExpressionPlaceholder:
                                        tpl.default_cron_expr ?? '',
                                      cronMinute: scheduleT('cronMinute'),
                                      cronHour: scheduleT('cronHour'),
                                      cronDayOfMonth:
                                        scheduleT('cronDayOfMonth'),
                                      cronMonth: scheduleT('cronMonth'),
                                      cronDayOfWeek: scheduleT('cronDayOfWeek'),
                                      schedulePreview:
                                        scheduleT('schedulePreview'),
                                      nextRuns: scheduleT('nextRuns'),
                                      invalidCronExpression: scheduleT(
                                        'invalidCronExpression',
                                      ),
                                    }}
                                    presets={CRON_PRESETS}
                                    renderPresetLabel={(key) =>
                                      scheduleT(key as never)
                                    }
                                    helper={t('cronExprHint', {
                                      defaultExpr: tpl.default_cron_expr,
                                    })}
                                    footer={
                                      <Button
                                        type='button'
                                        variant='secondary'
                                        size='sm'
                                        onClick={() =>
                                          handleConditionTextOverride(
                                            tpl.code,
                                            'cron_expr_override',
                                            '',
                                          )
                                        }
                                      >
                                        {t('cronUseDefault')}
                                      </Button>
                                    }
                                  />
                                </div>
                              ) : null}

                              <div className='grid grid-cols-2 gap-2'>
                                {tpl.default_threshold > 0 ? (
                                  <Input
                                    type='number'
                                    label={
                                      tpl.code === 'ALERT_FIRING'
                                        ? t('alertFiringThresholdLabel')
                                        : t('thresholdLabel', {
                                            value: tpl.default_threshold,
                                          })
                                    }
                                    placeholder={String(tpl.default_threshold)}
                                    value={condition?.threshold_override ?? ''}
                                    onChange={(e) =>
                                      handleConditionOverride(
                                        tpl.code,
                                        'threshold_override',
                                        e.target.value
                                          ? Number(e.target.value)
                                          : null,
                                      )
                                    }
                                  />
                                ) : null}

                                {tpl.default_window_minutes > 0 ||
                                tpl.code === 'ALERT_FIRING' ? (
                                  <Input
                                    type='number'
                                    label={
                                      tpl.code === 'ALERT_FIRING'
                                        ? t('alertFiringWindowLabel')
                                        : t('windowLabel', {
                                            minutes: tpl.default_window_minutes,
                                          })
                                    }
                                    placeholder={String(
                                      tpl.default_window_minutes,
                                    )}
                                    value={
                                      condition?.window_minutes_override ?? ''
                                    }
                                    onChange={(e) =>
                                      handleConditionOverride(
                                        tpl.code,
                                        'window_minutes_override',
                                        e.target.value
                                          ? Number(e.target.value)
                                          : null,
                                      )
                                    }
                                  />
                                ) : null}
                              </div>

                              {tpl.category === 'error_rate' ? (
                                <Input
                                  label={t('keywordsLabel')}
                                  description={t('keywordsHint')}
                                  placeholder={t('keywordsPlaceholder')}
                                  value={(condition?.extra_keywords || []).join(
                                    ', ',
                                  )}
                                  onChange={(e) =>
                                    handleConditionKeywordsOverride(
                                      tpl.code,
                                      e.target.value,
                                    )
                                  }
                                />
                              ) : null}

                              {tpl.category === 'alert_firing' ? (
                                <div className='space-y-1.5'>
                                  <span className={layout.fieldLabel}>
                                    {t('ruleKeysLabel')}
                                  </span>
                                  {(condition?.rule_keys || []).length > 0 ? (
                                    <div className='flex flex-wrap gap-1'>
                                      {(condition?.rule_keys || []).map(
                                        (ruleKey) => {
                                          const catalogItem =
                                            ruleCatalogByKey.get(ruleKey);
                                          // 目录外规则键（迁移后失效）：警示样式展示原始键并允许移除；目录未加载时不误判。
                                          // Out-of-catalog rule key (dead after migration): warn with the
                                          // raw key and offer removal; skipped while the catalog is empty.
                                          if (
                                            !catalogItem &&
                                            ruleCatalog.length > 0
                                          ) {
                                            return (
                                              <Badge
                                                key={ruleKey}
                                                tone='danger'
                                                className='text-[10px] font-mono gap-0.5'
                                              >
                                                {ruleKey}
                                                <button
                                                  type='button'
                                                  aria-label={t(
                                                    'ruleKeyRemove',
                                                    {key: ruleKey},
                                                  )}
                                                  className='inline-flex items-center hover:opacity-70'
                                                  onClick={() =>
                                                    handleConditionRuleKeyToggle(
                                                      tpl.code,
                                                      ruleKey,
                                                      false,
                                                    )
                                                  }
                                                >
                                                  <X className='h-3 w-3' />
                                                </button>
                                              </Badge>
                                            );
                                          }
                                          return (
                                            <Badge
                                              key={ruleKey}
                                              tone='neutral'
                                              title={ruleKey}
                                              className='text-[10px]'
                                            >
                                              {catalogItem?.name ?? ruleKey}
                                            </Badge>
                                          );
                                        },
                                      )}
                                    </div>
                                  ) : null}
                                  <div className='max-h-40 overflow-y-auto rounded-md border border-border/60 divide-y divide-border/40'>
                                    {ruleCatalog.length === 0 ? (
                                      <div className='px-2.5 py-2 text-[11px] text-muted-foreground'>
                                        {t('ruleCatalogEmpty')}
                                      </div>
                                    ) : (
                                      (['cluster', 'job'] as const).map(
                                        (scope) => {
                                          const items = ruleCatalog.filter(
                                            (item) => item.scope === scope,
                                          );
                                          if (items.length === 0) {
                                            return null;
                                          }
                                          return (
                                            <div key={scope}>
                                              <div className='px-2.5 py-1 text-[10px] font-medium text-muted-foreground bg-muted/40'>
                                                {scope === 'cluster'
                                                  ? tMonitoring('scopeCluster')
                                                  : tMonitoring('scopeJob')}
                                              </div>
                                              {items.map((item) => {
                                                const checked = (
                                                  condition?.rule_keys || []
                                                ).includes(item.rule_key);
                                                return (
                                                  <div
                                                    key={item.rule_key}
                                                    className='flex items-center gap-2 px-2.5 py-1.5 text-xs cursor-pointer hover:bg-muted/30'
                                                  >
                                                    <Checkbox
                                                      label={item.name}
                                                      checked={checked}
                                                      onCheckedChange={(
                                                        value,
                                                      ) =>
                                                        handleConditionRuleKeyToggle(
                                                          tpl.code,
                                                          item.rule_key,
                                                          value === true,
                                                        )
                                                      }
                                                    />
                                                    <span className='ml-auto shrink-0 text-[10px] text-muted-foreground font-mono'>
                                                      {item.rule_key}
                                                    </span>
                                                  </div>
                                                );
                                              })}
                                            </div>
                                          );
                                        },
                                      )
                                    )}
                                  </div>
                                </div>
                              ) : null}
                            </div>
                          ) : null}
                        </div>
                      );
                    })}
                  </div>
                </div>
              ),
            )
          )}
        </div>

        {/* 自动诊断任务收集编排 / Auto Task Bundle */}
        <div className='rounded-lg border bg-card/60 p-3 space-y-2.5 shadow-2xs'>
          <div className='flex items-center justify-between'>
            <div className='flex items-center gap-2'>
              <label
                htmlFor='auto-policy-auto-create-task'
                className='text-xs font-medium'
              >
                {t('autoBundleTitle')}
              </label>
              <HelpHint
                label={t('autoBundleTitle')}
                content={t('autoBundleHint')}
                data-testid='auto-policy-auto-bundle-hint'
              />
            </div>
            <Switch
              id='auto-policy-auto-create-task'
              checked={form.autoCreateTask}
              onCheckedChange={(v) => setField('autoCreateTask', v)}
            />
          </div>

          {form.autoCreateTask ? (
            <div className='space-y-3 border-t pt-3'>
              <DiagnosticResourceSelector
                selectedResources={form.taskOptions.selected_resources || []}
                onChange={(resources: DiagnosticsResourceCode[]) =>
                  setForm((f) => ({
                    ...f,
                    taskOptions: {
                      ...f.taskOptions,
                      selected_resources: resources,
                      include_thread_dump: resources.includes('thread_dump'),
                      include_jvm_dump: resources.includes('jvm_dump'),
                    },
                  }))
                }
                disabled={saving}
              />

              {(form.taskOptions.selected_resources || []).includes(
                'jvm_dump',
              ) ? (
                <div className='rounded-lg border bg-background p-3'>
                  <NumberField
                    label={t('jvmMinFreeMBLabel')}
                    value={form.taskOptions.jvm_dump_min_free_mb ?? 2048}
                    onValueChange={(v) =>
                      setForm((f) => ({
                        ...f,
                        taskOptions: {
                          ...f.taskOptions,
                          jvm_dump_min_free_mb: v,
                        },
                      }))
                    }
                    min={256}
                    step={256}
                  />
                </div>
              ) : null}

              <div className='flex items-center gap-2'>
                <Switch
                  id='auto-policy-auto-start-task'
                  checked={form.autoStartTask}
                  onCheckedChange={(v) => setField('autoStartTask', v)}
                />
                <label
                  htmlFor='auto-policy-auto-start-task'
                  className='text-xs font-normal text-muted-foreground'
                >
                  {t('autoStartTaskLabel')}
                </label>
                <HelpHint
                  label={t('autoStartTaskLabel')}
                  content={t('autoStartTaskHint')}
                  data-testid='auto-policy-auto-start-hint'
                />
              </div>
            </div>
          ) : null}
        </div>
      </div>

      <div className={layout.detailFoot}>
        <Button
          type='button'
          size='sm'
          loading={saving}
          onClick={() => void handleSave()}
          data-testid='policy-form-save'
        >
          {policy ? t('saveEdit') : t('createConfirm')}
        </Button>
        <Button type='button' size='sm' variant='ghost' onClick={onCancel}>
          {commonT('cancel')}
        </Button>
        {policy && onViewRecords ? (
          <Button
            type='button'
            size='sm'
            variant='ghost'
            onClick={onViewRecords}
            data-testid='policy-view-records'
          >
            {t('viewRecords')}
          </Button>
        ) : null}
        {policy && onDelete ? (
          <>
            <span className={layout.detailFootGrow} />
            <ConfirmMorph
              label={t('deletePolicy')}
              icon={<Trash2 className='size-3.5' aria-hidden />}
              prompt={t('deletePolicyPrompt')}
              confirmLabel={t('deleteConfirm')}
              cancelLabel={commonT('cancel')}
              pendingLabel={t('deleting')}
              onConfirm={onDelete}
            />
          </>
        ) : null}
      </div>
    </div>
  );
}
