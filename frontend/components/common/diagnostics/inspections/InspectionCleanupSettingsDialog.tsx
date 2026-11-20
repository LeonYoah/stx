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
 * 「巡检记录清理」全局设置弹窗：保留天数 + 最多保留份数，0 表示该维度不清理；
 * 打开时载入当前值，保存后提示下一轮清理按新值执行。
 * Global "inspection record cleanup" settings dialog: retention days + max
 * retained reports, 0 disables each dimension; loads current values on open and
 * confirms the next sweep picks them up after save.
 */
import {useCallback, useEffect, useState} from 'react';
import {useTranslations} from 'next-intl';
import {Loader2, Save} from 'lucide-react';
import {toast} from '@/lib/toast';
import {Button} from '@/components/arc/button/button';
import {Dialog, DialogContent} from '@/components/arc/dialog/dialog';
import {NumberField} from '@/components/arc/number-field/number-field';
import {HelpHint} from '@/components/common/HelpHint';
import services from '@/lib/services';

export interface InspectionCleanupSettingsDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function InspectionCleanupSettingsDialog({
  open,
  onOpenChange,
}: InspectionCleanupSettingsDialogProps) {
  const t = useTranslations('diagnosticsCenter.inspections.cleanupSettings');
  const commonT = useTranslations('common');
  const [retentionDays, setRetentionDays] = useState(0);
  const [maxReports, setMaxReports] = useState(0);
  const [loading, setLoading] = useState(false);
  const [saving, setSaving] = useState(false);
  const [loadError, setLoadError] = useState<string | null>(null);

  // 每次打开重新拉取：设置可能被另一处修改，展示必须反映服务端当前值。
  // Refetch on every open: settings may change elsewhere, so the form must show
  // the server's current values.
  useEffect(() => {
    if (!open) {
      return;
    }
    let active = true;
    setLoading(true);
    setLoadError(null);
    void services.diagnostics
      .getInspectionCleanupSettingsSafe()
      .then((result) => {
        if (!active) {
          return;
        }
        if (!result.success || !result.data) {
          setLoadError(result.error || t('loadError'));
          return;
        }
        setRetentionDays(result.data.retention_days ?? 0);
        setMaxReports(result.data.max_reports ?? 0);
      })
      .finally(() => {
        if (active) {
          setLoading(false);
        }
      });
    return () => {
      active = false;
    };
  }, [open, t]);

  const handleSave = useCallback(async () => {
    if (saving) {
      return;
    }
    setSaving(true);
    try {
      const result =
        await services.diagnostics.updateInspectionCleanupSettingsSafe({
          retention_days: retentionDays,
          max_reports: maxReports,
        });
      if (!result.success) {
        toast.error(result.error || t('saveError'));
        return;
      }
      toast.success(t('saveSuccess'));
      onOpenChange(false);
    } finally {
      setSaving(false);
    }
  }, [maxReports, onOpenChange, retentionDays, saving, t]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        title={t('title')}
        className='w-[min(calc(100vw-2rem),520px)]!'
        bodyClassName='max-h-[min(70dvh,480px)] overflow-y-auto'
      >
        {loading ? (
          <div
            className='flex items-center justify-center py-10 text-muted-foreground'
            role='status'
          >
            <Loader2 className='size-4 animate-spin' aria-hidden />
            <span className='ml-2 text-sm'>{commonT('loading')}</span>
          </div>
        ) : loadError ? (
          <div className='py-6 text-sm text-destructive' role='alert'>
            {loadError}
          </div>
        ) : (
          <div className='space-y-4 text-sm'>
            <p className='text-xs leading-relaxed text-muted-foreground'>
              {t('description')}
            </p>
            <NumberField
              label={t('retentionDays')}
              labelAddon={
                <HelpHint
                  label={t('retentionDays')}
                  content={t('retentionDaysHint')}
                  data-testid='cleanup-retention-hint'
                />
              }
              value={retentionDays}
              onValueChange={setRetentionDays}
              min={0}
              max={3650}
              suffix={t('daysSuffix')}
              disabled={saving}
            />
            <NumberField
              label={t('maxReports')}
              labelAddon={
                <HelpHint
                  label={t('maxReports')}
                  content={t('maxReportsHint')}
                  data-testid='cleanup-max-reports-hint'
                />
              }
              value={maxReports}
              onValueChange={setMaxReports}
              min={0}
              max={100000}
              disabled={saving}
            />
            <p className='text-xs leading-relaxed text-muted-foreground'>
              {t('zeroMeansDisabled')}
            </p>
          </div>
        )}
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
            onClick={() => void handleSave()}
            disabled={loading || loadError != null || saving}
            data-testid='cleanup-settings-save'
          >
            {saving ? (
              <Loader2 className='size-3.5 animate-spin' aria-hidden />
            ) : (
              <Save className='size-3.5' aria-hidden />
            )}
            {commonT('save')}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
