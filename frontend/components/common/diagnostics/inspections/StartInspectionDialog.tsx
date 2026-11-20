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
 * 发起巡检居中 Arc 弹窗：回溯窗口（含档位分段）+ 错误阈值 + 诊断资源选择，
 * 字段与校验规则从旧 DiagnosticsInspectionCenter 迁移；成功后把报告 / 发现项 /
 * 诊断包任务回交给面板刷新并打开详情。
 * Centered Arc dialog for starting an inspection: lookback window (with preset
 * segments) + error threshold + resource selection; fields and validation were
 * migrated from the old DiagnosticsInspectionCenter. On success the report /
 * findings / bundle task are handed back so the panel can refresh and open the
 * detail.
 */
import {useCallback, useState} from 'react';
import {useTranslations} from 'next-intl';
import {Loader2, Play} from 'lucide-react';
import {toast} from '@/lib/toast';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {Button} from '@/components/arc/button/button';
import {Dialog, DialogContent} from '@/components/arc/dialog/dialog';
import {NumberField} from '@/components/arc/number-field/number-field';
import {HelpHint} from '@/components/common/HelpHint';
import services from '@/lib/services';
import type {
  DiagnosticsInspectionFinding,
  DiagnosticsInspectionReport,
  DiagnosticsResourceCode,
  DiagnosticsTask,
} from '@/lib/services/diagnostics';
import {DiagnosticResourceSelector} from '../DiagnosticResourceSelector';
import {LOOKBACK_PRESET_MINUTES} from './inspection-utils';

export interface InspectionStartedResult {
  report: DiagnosticsInspectionReport;
  findings: DiagnosticsInspectionFinding[];
  task: DiagnosticsTask | null;
}

export interface StartInspectionDialogProps {
  /** 目标集群；未选时整个弹窗不会打开（入口按钮已禁用）。Target cluster; the dialog never opens without it (the entry button is disabled). */
  clusterId?: number;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onStarted: (result: InspectionStartedResult) => void;
}

export function StartInspectionDialog({
  clusterId,
  open,
  onOpenChange,
  onStarted,
}: StartInspectionDialogProps) {
  const t = useTranslations('diagnosticsCenter.inspections');
  const commonT = useTranslations('common');
  const [lookbackMinutes, setLookbackMinutes] = useState(30);
  const [errorThreshold, setErrorThreshold] = useState(1);
  const [resources, setResources] = useState<DiagnosticsResourceCode[]>([]);
  const [submitting, setSubmitting] = useState(false);

  const handleStart = useCallback(async () => {
    if (!clusterId || submitting) {
      return;
    }
    if (lookbackMinutes < 5 || lookbackMinutes > 1440) {
      toast.error(t('lookbackRangeError'));
      return;
    }
    if (errorThreshold < 1 || errorThreshold > 1000) {
      toast.error(t('errorThresholdRangeError'));
      return;
    }
    if (resources.length === 0) {
      toast.error(t('resourceRequired'));
      return;
    }
    setSubmitting(true);
    try {
      const result = await services.diagnostics.startInspectionSafe({
        cluster_id: clusterId,
        trigger_source: 'diagnostics_workspace',
        lookback_minutes: lookbackMinutes,
        error_threshold: errorThreshold,
      });
      if (!result.success || !result.data?.report) {
        toast.error(result.error || t('startError'));
        return;
      }
      const report = result.data.report;
      const taskResult = await services.diagnostics.createTaskSafe({
        cluster_id: clusterId,
        trigger_source: 'inspection_finding',
        source_ref: {inspection_report_id: report.id},
        node_scope: 'all',
        options: {
          include_thread_dump: resources.includes('thread_dump'),
          include_jvm_dump: resources.includes('jvm_dump'),
          jvm_dump_min_free_mb: 2048,
          selected_resources: resources,
        },
        lookback_minutes: lookbackMinutes,
        auto_start: true,
      });
      if (!taskResult.success) {
        toast.warning(
          t('resourceTaskError', {error: taskResult.error || t('startError')}),
        );
      } else {
        toast.success(t('startSuccess'));
      }
      onOpenChange(false);
      onStarted({
        report,
        findings: result.data.findings || [],
        task: taskResult.data ?? null,
      });
    } finally {
      setSubmitting(false);
    }
  }, [
    clusterId,
    errorThreshold,
    lookbackMinutes,
    onOpenChange,
    onStarted,
    resources,
    submitting,
    t,
  ]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        title={t('startInspection')}
        className='w-[min(calc(100vw-2rem),680px)]!'
        bodyClassName='max-h-[min(70dvh,560px)] overflow-y-auto'
      >
        <div className='space-y-4 text-sm'>
          <NumberField
            label={t('lookbackField')}
            labelAddon={
              <HelpHint
                label={t('lookbackField')}
                content={t('lookbackHint')}
                data-testid='inspection-lookback-hint'
              />
            }
            value={lookbackMinutes}
            onValueChange={setLookbackMinutes}
            min={5}
            max={1440}
            step={5}
            suffix={t('minutesSuffix')}
            disabled={submitting}
          />
          <SegmentedControl
            label={t('lookbackPresetLabel')}
            value={
              LOOKBACK_PRESET_MINUTES.includes(
                lookbackMinutes as (typeof LOOKBACK_PRESET_MINUTES)[number],
              )
                ? String(lookbackMinutes)
                : ''
            }
            onValueChange={(v) => setLookbackMinutes(Number(v))}
            options={LOOKBACK_PRESET_MINUTES.map((minutes) => ({
              value: String(minutes),
              label:
                minutes >= 60
                  ? t('presetHours', {count: minutes / 60})
                  : t('presetMinutes', {count: minutes}),
            }))}
          />
          <NumberField
            label={t('errorThresholdLabel')}
            labelAddon={
              <HelpHint
                label={t('errorThresholdLabel')}
                content={t('errorThresholdTip')}
                data-testid='inspection-error-threshold-hint'
              />
            }
            value={errorThreshold}
            onValueChange={setErrorThreshold}
            min={1}
            max={1000}
            disabled={submitting}
          />
          <div className='space-y-2 border-t border-border/70 pt-4'>
            <div className='text-xs font-medium text-foreground'>
              {t('resourcesLabel')}
            </div>
            <DiagnosticResourceSelector
              selectedResources={resources}
              onChange={setResources}
              disabled={submitting}
            />
          </div>
        </div>
        <div className='mt-5 flex items-center justify-end gap-2 border-t border-border/70 pt-4'>
          <Button
            type='button'
            variant='ghost'
            size='sm'
            onClick={() => onOpenChange(false)}
          >
            {commonT('cancel')}
          </Button>
          <Button
            type='button'
            size='sm'
            onClick={() => void handleStart()}
            disabled={!clusterId || submitting || resources.length === 0}
            data-testid='inspection-start-submit'
          >
            {submitting ? (
              <Loader2 className='size-3.5 animate-spin' aria-hidden />
            ) : (
              <Play className='size-3 fill-current' aria-hidden />
            )}
            {t('startInspection')}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
