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

import {useCallback, useEffect, useState} from 'react';
import {Loader2, Plus, Trash2, ShieldAlert} from 'lucide-react';
import {useTranslations} from 'next-intl';
import {toast} from 'sonner';
import services from '@/lib/services';
import {cn} from '@/lib/utils';
import type {
  InspectionAutoPolicy,
  InspectionConditionItem,
  InspectionConditionTemplate,
  DiagnosticsClusterOption,
  DiagnosticsTaskOptions,
} from '@/lib/services/diagnostics';
import {Badge} from '@/components/ui/badge';
import {Button} from '@/components/ui/button';
import {Checkbox} from '@/components/ui/checkbox';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {Input} from '@/components/ui/input';
import {CronExpressionEditor, type CronPresetOption} from '@/components/common/schedule/CronExpressionEditor';
import {Label} from '@/components/ui/label';
import {Switch} from '@/components/ui/switch';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';

interface AutoPolicyConfigPanelProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  clusterOptions: DiagnosticsClusterOption[];
}

export function shouldRenderCronExprInput(
  template: InspectionConditionTemplate,
): boolean {
  return Boolean(template.default_cron_expr?.trim());
}

export function applyConditionTextOverride(
  conditions: InspectionConditionItem[],
  templateCode: string,
  field: 'cron_expr_override',
  value: string,
): InspectionConditionItem[] {
  return conditions.map((condition) =>
    condition.template_code === templateCode
      ? {...condition, [field]: value}
      : condition,
  );
}

export function normalizeConditionItemsForSave(
  conditions: InspectionConditionItem[],
): InspectionConditionItem[] {
  return conditions.map((condition) => {
    const cronExprOverride = condition.cron_expr_override?.trim();
    const extraKeywords = (condition.extra_keywords || [])
      .map((item) => item.trim())
      .filter(Boolean);

    const result: InspectionConditionItem = {
      template_code: condition.template_code,
      enabled: condition.enabled,
    };

    if (condition.threshold_override !== undefined) {
      result.threshold_override = condition.threshold_override;
    }
    if (condition.window_minutes_override !== undefined) {
      result.window_minutes_override = condition.window_minutes_override;
    }
    if (cronExprOverride) {
      result.cron_expr_override = cronExprOverride;
    }
    if (extraKeywords.length > 0) {
      result.extra_keywords = extraKeywords;
    }

    return result;
  });
}

export function AutoPolicyConfigPanel({
  open,
  onOpenChange,
  clusterOptions,
}: AutoPolicyConfigPanelProps) {
  const t = useTranslations('diagnosticsCenter.autoPolicies');
  const commonT = useTranslations('common');
  const scheduleT = useTranslations('workbenchStudio');
  const [policies, setPolicies] = useState<InspectionAutoPolicy[]>([]);
  const [templates, setTemplates] = useState<InspectionConditionTemplate[]>([]);
  const [loading, setLoading] = useState(false);
  const [formOpen, setFormOpen] = useState(false);
  const [editingPolicy, setEditingPolicy] =
    useState<InspectionAutoPolicy | null>(null);
  const [saving, setSaving] = useState(false);
  const [deleting, setDeleting] = useState<number | null>(null);

  // Form state
  const [formName, setFormName] = useState('');
  const [formClusterId, setFormClusterId] = useState(0);
  const [formEnabled, setFormEnabled] = useState(true);
  const [formCooldown, setFormCooldown] = useState(30);
  const [formConditions, setFormConditions] = useState<
    InspectionConditionItem[]
  >([]);
  const [formAutoCreateTask, setFormAutoCreateTask] = useState(false);
  const [formAutoStartTask, setFormAutoStartTask] = useState(true);

  const cronPresets = useState<CronPresetOption[]>([
    {key: 'cronPresetEveryDayMidnight', expr: '0 0 * * *'},
    {key: 'cronPresetEveryHour', expr: '0 * * * *'},
    {key: 'cronPresetWeekdaysNine', expr: '0 9 * * MON-FRI'},
    {key: 'cronPresetEveryFifteenMinutes', expr: '*/15 * * * *'},
    {key: 'cronPresetEveryThirtyMinutes', expr: '*/30 * * * *'},
  ])[0];
  const [formTaskOptions, setFormTaskOptions] =
    useState<DiagnosticsTaskOptions>({
      include_thread_dump: true,
      include_jvm_dump: false,
      jvm_dump_min_free_mb: 2048,
    });

  const getCategoryLabel = useCallback(
    (category: string) => {
      switch (category) {
        case 'java_error':
          return t('categories.javaError');
        case 'prometheus':
          return t('categories.prometheus');
        case 'error_rate':
          return t('categories.errorRate');
        case 'node_unhealthy':
          return t('categories.nodeUnhealthy');
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

  const getPolicyScopeLabel = useCallback(
    (clusterId: number) =>
      clusterId === 0 ? t('scopeGlobal') : t('scopeCluster', {id: clusterId}),
    [t],
  );

  const getAutoBundleModeLabel = useCallback(
    (autoStartTask: boolean) =>
      autoStartTask
        ? t('autoBundleModeCreateAndStart')
        : t('autoBundleModeCreateOnly'),
    [t],
  );

  const loadData = useCallback(async () => {
    setLoading(true);
    try {
      const [policiesResult, templatesResult] = await Promise.all([
        services.diagnostics.listAutoPoliciesSafe({page_size: 100}),
        services.diagnostics.listBuiltinConditionTemplatesSafe(),
      ]);
      if (policiesResult.success && policiesResult.data) {
        setPolicies(policiesResult.data.items || []);
      } else {
        toast.error(policiesResult.error || t('loadPoliciesError'));
      }
      if (templatesResult.success && templatesResult.data) {
        setTemplates(templatesResult.data);
      }
    } finally {
      setLoading(false);
    }
  }, [t]);

  useEffect(() => {
    if (open) {
      void loadData();
    }
  }, [loadData, open]);

  const openCreateForm = useCallback(() => {
    setEditingPolicy(null);
    setFormName('');
    setFormClusterId(0);
    setFormEnabled(true);
    setFormCooldown(30);
    setFormConditions([]);
    setFormAutoCreateTask(false);
    setFormAutoStartTask(true);
    setFormTaskOptions({
      include_thread_dump: true,
      include_jvm_dump: false,
      jvm_dump_min_free_mb: 2048,
    });
    setFormOpen(true);
  }, []);

  const openEditForm = useCallback((policy: InspectionAutoPolicy) => {
    setEditingPolicy(policy);
    setFormName(policy.name);
    setFormClusterId(policy.cluster_id);
    setFormEnabled(policy.enabled);
    setFormCooldown(policy.cooldown_minutes);
    setFormConditions(policy.conditions || []);
    setFormAutoCreateTask(policy.auto_create_task);
    setFormAutoStartTask(policy.auto_start_task);
    setFormTaskOptions(
      policy.task_options || {
        include_thread_dump: true,
        include_jvm_dump: false,
        jvm_dump_min_free_mb: 2048,
      },
    );
    setFormOpen(true);
  }, []);

  const handleToggleCondition = useCallback(
    (templateCode: string, checked: boolean) => {
      setFormConditions((prev) => {
        if (checked) {
          if (prev.some((c) => c.template_code === templateCode)) {
            return prev.map((c) =>
              c.template_code === templateCode ? {...c, enabled: true} : c,
            );
          }
          return [...prev, {template_code: templateCode, enabled: true}];
        }
        return prev.filter((c) => c.template_code !== templateCode);
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
      setFormConditions((prev) =>
        prev.map((c) =>
          c.template_code === templateCode ? {...c, [field]: value} : c,
        ),
      );
    },
    [],
  );

  const handleConditionTextOverride = useCallback(
    (templateCode: string, field: 'cron_expr_override', value: string) => {
      setFormConditions((prev) =>
        applyConditionTextOverride(prev, templateCode, field, value),
      );
    },
    [],
  );

  const handleConditionKeywordsOverride = useCallback(
    (templateCode: string, value: string) => {
      const parsed = value
        .split(',')
        .map((item) => item.trim())
        .filter(Boolean);
      setFormConditions((prev) =>
        prev.map((condition) =>
          condition.template_code === templateCode
            ? {
                ...condition,
                extra_keywords: parsed.length > 0 ? parsed : undefined,
              }
            : condition,
        ),
      );
    },
    [],
  );

  const handleSave = useCallback(async () => {
    if (!formName.trim()) {
      toast.error(t('nameRequired'));
      return;
    }
    setSaving(true);
    try {
      if (editingPolicy) {
        const result = await services.diagnostics.updateAutoPolicySafe(
          editingPolicy.id,
          {
            name: formName,
            enabled: formEnabled,
            conditions: normalizeConditionItemsForSave(formConditions),
            cooldown_minutes: formCooldown,
            auto_create_task: formAutoCreateTask,
            auto_start_task: formAutoStartTask,
            task_options: formAutoCreateTask ? formTaskOptions : undefined,
          },
        );
        if (!result.success) {
          toast.error(result.error || t('updateError'));
          return;
        }
        toast.success(t('updateSuccess'));
      } else {
        const result = await services.diagnostics.createAutoPolicySafe({
          cluster_id: formClusterId,
          name: formName,
          enabled: formEnabled,
          conditions: normalizeConditionItemsForSave(formConditions),
          cooldown_minutes: formCooldown,
          auto_create_task: formAutoCreateTask,
          auto_start_task: formAutoStartTask,
          task_options: formAutoCreateTask ? formTaskOptions : undefined,
        });
        if (!result.success) {
          toast.error(result.error || t('createError'));
          return;
        }
        toast.success(t('createSuccess'));
      }
      setFormOpen(false);
      void loadData();
    } finally {
      setSaving(false);
    }
  }, [
    editingPolicy,
    formAutoCreateTask,
    formAutoStartTask,
    formClusterId,
    formConditions,
    formCooldown,
    formEnabled,
    formName,
    formTaskOptions,
    loadData,
    t,
  ]);

  const handleDelete = useCallback(
    async (id: number) => {
      setDeleting(id);
      try {
        const result = await services.diagnostics.deleteAutoPolicySafe(id);
        if (!result.success) {
          toast.error(result.error || t('deleteError'));
          return;
        }
        toast.success(t('deleteSuccess'));
        void loadData();
      } finally {
        setDeleting(null);
      }
    },
    [loadData, t],
  );

  const handleToggleEnabled = useCallback(
    async (policy: InspectionAutoPolicy) => {
      const result = await services.diagnostics.updateAutoPolicySafe(
        policy.id,
        {enabled: !policy.enabled},
      );
      if (!result.success) {
        toast.error(result.error || t('updateError'));
        return;
      }
      void loadData();
    },
    [loadData, t],
  );

  // Group templates by category
  const groupedTemplates = templates.reduce<
    Record<string, InspectionConditionTemplate[]>
  >((acc, tpl) => {
    const key = tpl.category;
    if (!acc[key]) {
      acc[key] = [];
    }
    acc[key].push(tpl);
    return acc;
  }, {});

  return (
    <>
      {/* 自动巡检策略列表弹窗：加宽至 1120px 匹配全局规范 / Auto inspection policy list dialog: widened to 1120px matching global standards */}
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent className='flex h-[85vh] w-[95vw] max-w-[1120px] flex-col overflow-hidden p-0 sm:max-w-[1120px]'>
          <DialogHeader className='flex flex-row items-center justify-between border-b px-6 py-4 bg-muted/10'>
            <div>
              <div className='flex items-center gap-2'>
                <DialogTitle className='text-lg font-semibold flex items-center gap-2'>
                  <ShieldAlert className='h-5 w-5 text-primary' />
                  {t('title')}
                </DialogTitle>
                <Badge variant='outline' className='text-xs font-normal'>
                  {t('count', {count: policies.length})}
                </Badge>
              </div>
              <DialogDescription className='text-xs text-muted-foreground mt-1'>
                {t('description')}
              </DialogDescription>
            </div>
            <div className='flex items-center gap-2'>
              <Button size='sm' onClick={openCreateForm} className='h-8 text-xs'>
                <Plus className='mr-1.5 h-3.5 w-3.5' />
                {t('create')}
              </Button>
            </div>
          </DialogHeader>

          <div className='flex-1 overflow-y-auto p-6'>
            {loading ? (
              <div className='flex items-center justify-center py-16'>
                <Loader2 className='h-6 w-6 animate-spin text-muted-foreground' />
              </div>
            ) : policies.length === 0 ? (
              <div className='flex flex-col items-center justify-center rounded-lg border border-dashed p-12 text-sm text-muted-foreground'>
                {t('empty')}
              </div>
            ) : (
              <div className='grid grid-cols-1 md:grid-cols-2 gap-4'>
                {policies.map((policy) => (
                  <div
                    key={policy.id}
                    className='flex flex-col justify-between rounded-lg border bg-card/60 p-4 shadow-2xs hover:border-primary/40 transition-colors'
                  >
                    <div className='space-y-3'>
                      <div className='flex items-start justify-between gap-2'>
                        <div className='min-w-0 flex-1'>
                          <div className='flex items-center gap-2 flex-wrap'>
                            <span className='font-semibold text-sm truncate'>
                              {policy.name}
                            </span>
                            <Badge variant='secondary' className='text-[11px]'>
                              {getPolicyScopeLabel(policy.cluster_id)}
                            </Badge>
                          </div>
                        </div>
                        <Switch
                          checked={policy.enabled}
                          onCheckedChange={() => void handleToggleEnabled(policy)}
                        />
                      </div>

                      <div className='flex flex-wrap gap-2 text-xs text-muted-foreground'>
                        <span className='inline-flex items-center rounded-md bg-muted/70 px-2 py-0.5 text-[11px] font-medium'>
                          {t('conditionsSummary', {
                            count: (policy.conditions || []).length,
                            minutes: policy.cooldown_minutes,
                          })}
                        </span>
                        {policy.auto_create_task && (
                          <span className='inline-flex items-center rounded-md bg-primary/10 text-primary px-2 py-0.5 text-[11px] font-medium'>
                            {t('autoBundleSummary', {
                              mode: getAutoBundleModeLabel(policy.auto_start_task),
                            })}
                          </span>
                        )}
                      </div>
                    </div>

                    <div className='mt-4 flex items-center justify-end gap-2 border-t pt-3'>
                      <Button
                        variant='outline'
                        size='sm'
                        onClick={() => openEditForm(policy)}
                        className='h-7 px-2.5 text-xs'
                      >
                        {commonT('edit')}
                      </Button>
                      <Button
                        variant='ghost'
                        size='sm'
                        onClick={() => void handleDelete(policy.id)}
                        disabled={deleting === policy.id}
                        className='h-7 px-2 text-xs text-destructive hover:text-destructive hover:bg-destructive/10'
                      >
                        {deleting === policy.id ? (
                          <Loader2 className='h-3.5 w-3.5 animate-spin' />
                        ) : (
                          <Trash2 className='h-3.5 w-3.5' />
                        )}
                      </Button>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        </DialogContent>
      </Dialog>

      {/* 创建与编辑策略表单弹窗：加宽至 1240px / Create & edit policy form dialog: widened to 1240px */}
      <Dialog open={formOpen} onOpenChange={setFormOpen}>
        <DialogContent className='flex h-[88vh] w-[96vw] max-w-[1240px] flex-col overflow-hidden p-0 sm:max-w-[1240px]'>
          <DialogHeader className='border-b px-6 py-4 bg-muted/10'>
            <DialogTitle className='text-lg font-semibold'>
              {editingPolicy ? t('editTitle') : t('createTitle')}
            </DialogTitle>
            <DialogDescription className='text-xs text-muted-foreground'>
              {t('description')}
            </DialogDescription>
          </DialogHeader>

          <div className='flex-1 overflow-y-auto p-6 space-y-6'>
            {/* 1. 基本信息配置 / Basic Info */}
            <div className='rounded-lg border bg-card/60 p-4 space-y-4 shadow-2xs'>
              <div className='text-sm font-semibold flex items-center gap-2'>
                {t('nameLabel')} & {commonT('status')}
              </div>
              <div className='grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4 items-end'>
                <div className='space-y-1.5 md:col-span-2'>
                  <Label className='text-xs'>{t('nameLabel')}</Label>
                  <Input
                    value={formName}
                    onChange={(e) => setFormName(e.target.value)}
                    placeholder={t('namePlaceholder')}
                    className='h-8 text-xs'
                  />
                </div>

                {!editingPolicy ? (
                  <div className='space-y-1.5'>
                    <Label className='text-xs'>{t('clusterLabel')}</Label>
                    <Select
                      value={String(formClusterId)}
                      onValueChange={(value) =>
                        setFormClusterId(Number.parseInt(value, 10) || 0)
                      }
                    >
                      <SelectTrigger className='h-8 text-xs'>
                        <SelectValue placeholder={t('clusterPlaceholder')} />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value='0'>{t('globalPolicyOption')}</SelectItem>
                        {clusterOptions.map((cluster) => (
                          <SelectItem
                            key={cluster.cluster_id}
                            value={String(cluster.cluster_id)}
                          >
                            {t('clusterOption', {
                              name: cluster.cluster_name,
                              id: cluster.cluster_id,
                            })}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                ) : (
                  <div className='space-y-1.5'>
                    <Label className='text-xs'>{t('clusterLabel')}</Label>
                    <div className='h-8 px-3 flex items-center rounded-md border bg-muted/30 text-xs text-muted-foreground'>
                      {getPolicyScopeLabel(editingPolicy.cluster_id)}
                    </div>
                  </div>
                )}

                <div className='space-y-1.5'>
                  <Label className='text-xs'>{t('cooldownLabel')}</Label>
                  <Input
                    type='number'
                    min={1}
                    max={1440}
                    value={formCooldown}
                    onChange={(e) =>
                      setFormCooldown(Number.parseInt(e.target.value, 10) || 30)
                    }
                    className='h-8 text-xs'
                  />
                </div>
              </div>

              <div className='flex items-center gap-2 pt-2 border-t'>
                <Switch checked={formEnabled} onCheckedChange={setFormEnabled} />
                <Label className='text-xs'>{t('enabledLabel')}</Label>
              </div>
            </div>

            {/* 2. 自动诊断任务收集编排 / Auto Task Bundle */}
            <div className='rounded-lg border bg-card/60 p-4 space-y-3 shadow-2xs'>
              <div className='flex items-center justify-between'>
                <div>
                  <div className='text-sm font-semibold'>{t('autoBundleTitle')}</div>
                  <div className='text-xs text-muted-foreground'>
                    {t('autoBundleHint')}
                  </div>
                </div>
                <Switch
                  checked={formAutoCreateTask}
                  onCheckedChange={setFormAutoCreateTask}
                />
              </div>

              {formAutoCreateTask ? (
                <div className='mt-3 grid gap-4 md:grid-cols-3 pt-3 border-t'>
                  <div className='flex items-center justify-between rounded-lg border bg-background p-3'>
                    <div className='pr-2'>
                      <div className='text-xs font-medium'>{t('includeThreadDump')}</div>
                      <div className='text-[11px] text-muted-foreground'>
                        {t('includeThreadDumpHint')}
                      </div>
                    </div>
                    <Switch
                      checked={formTaskOptions.include_thread_dump}
                      onCheckedChange={(checked) =>
                        setFormTaskOptions((current: DiagnosticsTaskOptions) => ({
                          ...current,
                          include_thread_dump: checked,
                        }))
                      }
                    />
                  </div>

                  <div className='flex items-center justify-between rounded-lg border bg-background p-3'>
                    <div className='pr-2'>
                      <div className='text-xs font-medium'>{t('includeJVMDump')}</div>
                      <div className='text-[11px] text-muted-foreground'>
                        {t('includeJVMDumpHint')}
                      </div>
                    </div>
                    <Switch
                      checked={formTaskOptions.include_jvm_dump}
                      onCheckedChange={(checked) =>
                        setFormTaskOptions((current: DiagnosticsTaskOptions) => ({
                          ...current,
                          include_jvm_dump: checked,
                        }))
                      }
                    />
                  </div>

                  <div className='rounded-lg border bg-background p-3 space-y-1.5'>
                    <Label htmlFor='auto-policy-jvm-space' className='text-xs'>
                      {t('jvmMinFreeMBLabel')}
                    </Label>
                    <Input
                      id='auto-policy-jvm-space'
                      type='number'
                      min={256}
                      step={256}
                      value={formTaskOptions.jvm_dump_min_free_mb ?? 2048}
                      onChange={(event) =>
                        setFormTaskOptions((current: DiagnosticsTaskOptions) => ({
                          ...current,
                          jvm_dump_min_free_mb:
                            Number.parseInt(event.target.value, 10) || 2048,
                        }))
                      }
                      disabled={!formTaskOptions.include_jvm_dump}
                      className='h-8 text-xs'
                    />
                  </div>

                  <div className='flex items-center gap-2 md:col-span-3 pt-1'>
                    <Switch
                      checked={formAutoStartTask}
                      onCheckedChange={setFormAutoStartTask}
                    />
                    <div className='text-xs text-muted-foreground'>
                      {t('autoStartTaskLabel')}
                    </div>
                  </div>
                </div>
              ) : null}
            </div>

            {/* 3. 巡检触发条件（两列宽屏自适应布局） / Inspection Conditions (Wide 2-column layout) */}
            <div className='space-y-4'>
              <div className='flex items-center justify-between'>
                <Label className='text-sm font-semibold'>{t('conditionsLabel')}</Label>
                <span className='text-xs text-muted-foreground'>
                  已选条件：{formConditions.filter((c) => c.enabled).length}
                </span>
              </div>

              <div className='grid grid-cols-1 lg:grid-cols-2 gap-4'>
                {Object.entries(groupedTemplates).map(
                  ([category, categoryTemplates]) => (
                    <div
                      key={category}
                      className='rounded-lg border bg-card/60 p-4 space-y-3 shadow-2xs'
                    >
                      <div className='flex items-center justify-between border-b pb-2'>
                        <span className='text-xs font-semibold uppercase tracking-wider text-primary'>
                          {getCategoryLabel(category)}
                        </span>
                        <Badge variant='outline' className='text-[10px]'>
                          {categoryTemplates.length} 项规则
                        </Badge>
                      </div>

                      <div className='space-y-3'>
                        {categoryTemplates.map((tpl) => {
                          const isChecked = formConditions.some(
                            (c) => c.template_code === tpl.code && c.enabled,
                          );
                          const condition = formConditions.find(
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
                              <div className='flex items-start gap-2.5'>
                                <Checkbox
                                  id={`chk-${tpl.code}`}
                                  checked={isChecked}
                                  onCheckedChange={(checked) =>
                                    handleToggleCondition(
                                      tpl.code,
                                      checked === true,
                                    )
                                  }
                                  className='mt-0.5'
                                />
                                <div className='flex-1 min-w-0'>
                                  <label
                                    htmlFor={`chk-${tpl.code}`}
                                    className='text-xs font-medium cursor-pointer block text-foreground'
                                  >
                                    {tpl.name}
                                  </label>
                                  <p className='text-[11px] text-muted-foreground mt-0.5 leading-relaxed'>
                                    {tpl.description}
                                  </p>
                                </div>
                              </div>

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
                                          scheduleT(
                                            key as never,
                                            values as never,
                                          )
                                        }
                                        labels={{
                                          cronExpression: t('cronExprLabel'),
                                          timezone: commonT('timezone'),
                                          cronFiveField: scheduleT('cronFiveField'),
                                          cronTimezonePlaceholder: scheduleT('cronTimezonePlaceholder'),
                                          cronExpressionPlaceholder: tpl.default_cron_expr ?? '',
                                          cronMinute: scheduleT('cronMinute'),
                                          cronHour: scheduleT('cronHour'),
                                          cronDayOfMonth: scheduleT('cronDayOfMonth'),
                                          cronMonth: scheduleT('cronMonth'),
                                          cronDayOfWeek: scheduleT('cronDayOfWeek'),
                                          schedulePreview: scheduleT('schedulePreview'),
                                          nextRuns: scheduleT('nextRuns'),
                                          invalidCronExpression: scheduleT('invalidCronExpression'),
                                        }}
                                        presets={cronPresets}
                                        renderPresetLabel={(key) => scheduleT(key as never)}
                                        helper={t('cronExprHint', { defaultExpr: tpl.default_cron_expr })}
                                        footer={
                                          <Button
                                            type='button'
                                            variant='outline'
                                            size='sm'
                                            className='h-7 px-2 text-[10px]'
                                            onClick={() => handleConditionTextOverride(tpl.code, 'cron_expr_override', '')}
                                          >
                                            {t('cronUseDefault')}
                                          </Button>
                                        }
                                      />
                                    </div>
                                  ) : null}

                                  <div className='grid grid-cols-2 gap-2'>
                                    {tpl.default_threshold > 0 ? (
                                      <div className='space-y-1'>
                                        <Label className='text-[11px]'>
                                          {t('thresholdLabel', {
                                            value: tpl.default_threshold,
                                          })}
                                        </Label>
                                        <Input
                                          type='number'
                                          className='h-7 text-xs'
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
                                      </div>
                                    ) : null}

                                    {tpl.default_window_minutes > 0 ? (
                                      <div className='space-y-1'>
                                        <Label className='text-[11px]'>
                                          {t('windowLabel', {
                                            minutes: tpl.default_window_minutes,
                                          })}
                                        </Label>
                                        <Input
                                          type='number'
                                          className='h-7 text-xs'
                                          placeholder={String(tpl.default_window_minutes)}
                                          value={condition?.window_minutes_override ?? ''}
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
                                      </div>
                                    ) : null}
                                  </div>

                                  {['error_rate', 'node_unhealthy', 'alert_firing'].includes(
                                    tpl.category,
                                  ) ? (
                                    <div className='space-y-1'>
                                      <Label className='text-[11px]'>
                                        {t('keywordsLabel')}
                                      </Label>
                                      <Input
                                        className='h-7 text-xs'
                                        placeholder={t('keywordsPlaceholder')}
                                        value={(condition?.extra_keywords || []).join(', ')}
                                        onChange={(e) =>
                                          handleConditionKeywordsOverride(
                                            tpl.code,
                                            e.target.value,
                                          )
                                        }
                                      />
                                      <div className='text-[10px] text-muted-foreground'>
                                        {t('keywordsHint')}
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
                )}
              </div>
            </div>
          </div>

          <DialogFooter className='border-t px-6 py-3.5 bg-muted/15 flex items-center justify-end gap-2'>
            <Button variant='outline' size='sm' onClick={() => setFormOpen(false)} className='h-8 text-xs'>
              {commonT('cancel')}
            </Button>
            <Button size='sm' onClick={() => void handleSave()} disabled={saving} className='h-8 text-xs'>
              {saving ? <Loader2 className='mr-1.5 h-3.5 w-3.5 animate-spin' /> : null}
              {editingPolicy ? t('saveEdit') : t('createConfirm')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
