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
import {useTranslations} from 'next-intl';
import {BellRing, HelpCircle} from 'lucide-react';
import {toast} from 'sonner';
import {Badge} from '@/components/ui/badge';
import {Button} from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {Label} from '@/components/ui/label';
import {Skeleton} from '@/components/ui/skeleton';
import {Tooltip, TooltipContent, TooltipTrigger} from '@/components/ui/tooltip';
import services from '@/lib/services';
import type {
  NotificationChannel,
  NotificationRecipientUser,
} from '@/lib/services/monitoring';
import type {SyncTaskAlertConfig, SyncTaskAlertRequest} from '@/lib/services/sync';
import {TaskAlertForm} from './TaskAlertForm';
import {summarizeTaskAlert} from './task-alert-utils';

/**
 * 编辑任务告警配置的弹窗：任务侧栏和策略中心共用，保存走任务告警接口。
 * Dialog for editing a task's alert config, shared by the task sidebar and the policy center; it saves through the
 * task alert API.
 */
export function TaskAlertDialog({
  open,
  taskId,
  taskName,
  config,
  channels,
  users,
  readOnly,
  onOpenChange,
  onSaved,
}: {
  open: boolean;
  taskId: number;
  taskName?: string;
  config: SyncTaskAlertConfig | null;
  channels: NotificationChannel[];
  users: NotificationRecipientUser[];
  readOnly?: boolean;
  onOpenChange: (open: boolean) => void;
  onSaved: (config: SyncTaskAlertConfig) => void;
}) {
  const t = useTranslations('syncAlert');
  const [saving, setSaving] = useState(false);

  const save = async (request: SyncTaskAlertRequest) => {
    setSaving(true);
    try {
      const saved = await services.sync.updateTaskAlert(taskId, request);
      toast.success(t('saved'));
      onSaved(saved);
      onOpenChange(false);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : String(error));
    } finally {
      setSaving(false);
    }
  };

  const remove = async () => {
    if (!window.confirm(t('deleteConfirm'))) {
      return;
    }
    setSaving(true);
    try {
      const cleared = await services.sync.deleteTaskAlert(taskId);
      toast.success(t('deleted'));
      onSaved(cleared);
      onOpenChange(false);
    } catch (error) {
      toast.error(error instanceof Error ? error.message : String(error));
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={(next) => !saving && onOpenChange(next)}>
      <DialogContent className='max-h-[88vh] overflow-y-auto border border-border/80 shadow-2xl dark:border-border/60 sm:max-w-[640px]'>
        <DialogHeader>
          <DialogTitle className='flex items-center gap-2'>
            <BellRing className='size-4 text-primary' />
            <span>{t('dialogTitle')}</span>
            {taskName ? <span className='truncate text-sm font-normal text-muted-foreground'>· {taskName}</span> : null}
          </DialogTitle>
          <DialogDescription className='text-xs'>{t('dialogDescription')}</DialogDescription>
        </DialogHeader>
        <TaskAlertForm
          initial={config}
          channels={channels}
          users={users}
          saving={saving}
          readOnly={readOnly}
          onSubmit={(request) => void save(request)}
          onCancel={() => onOpenChange(false)}
          onDelete={() => void remove()}
        />
      </DialogContent>
    </Dialog>
  );
}

/**
 * 任务设置侧栏里的"告警"分组：未配置时只显示一行和"配置"按钮；已配置时显示摘要，点击后在弹窗里编辑。
 * The "Alerts" group in the task settings sidebar: one line plus a Configure button when nothing is configured, a
 * summary otherwise; editing happens in a dialog.
 */
export function TaskAlertSection({
  taskId,
  taskName,
  canEdit,
  users,
}: {
  taskId: number | null;
  taskName?: string;
  canEdit: boolean;
  users: NotificationRecipientUser[];
}) {
  const t = useTranslations('syncAlert');
  const [config, setConfig] = useState<SyncTaskAlertConfig | null>(null);
  const [channels, setChannels] = useState<NotificationChannel[]>([]);
  const [loading, setLoading] = useState(false);
  const [unavailable, setUnavailable] = useState(false);
  const [open, setOpen] = useState(false);

  const load = useCallback(async (id: number) => {
    setLoading(true);
    setUnavailable(false);
    try {
      const [loaded, channelData] = await Promise.all([
        services.sync.getTaskAlert(id),
        services.monitoring.listNotificationChannels().catch(() => ({channels: [] as NotificationChannel[]})),
      ]);
      setConfig(loaded);
      setChannels(channelData.channels ?? []);
    } catch (error) {
      // api-client 把 HTTP 状态放在 error.status 上；503 表示告警模块没有启用。
      // api-client puts the HTTP status on error.status; 503 means the alert module is not enabled.
      const status = (error as {status?: number})?.status;
      setConfig(null);
      setUnavailable(status === 503);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    setConfig(null);
    if (taskId) {
      void load(taskId);
    }
  }, [taskId, load]);

  if (!taskId) {
    return null;
  }

  const summary = config?.configured
    ? summarizeTaskAlert(config, channels, (rule) => t(`rule_${rule}`), t('listSeparator'))
    : null;

  return (
    <div className='space-y-1.5' data-testid='task-alert-section'>
      <div className='flex items-center gap-1'>
        <Label className='text-[11px] text-muted-foreground'>{t('sectionTitle')}</Label>
        <Tooltip>
          <TooltipTrigger asChild>
            <HelpCircle className='size-3 cursor-help text-muted-foreground/70' />
          </TooltipTrigger>
          <TooltipContent side='top' className='max-w-[260px] text-xs'>
            {t('sectionHint')}
          </TooltipContent>
        </Tooltip>
      </div>
      {loading && !config ? (
        <Skeleton className='h-8 w-full' />
      ) : unavailable ? (
        <p className='text-xs text-muted-foreground'>{t('unavailable')}</p>
      ) : (
        <div className='flex min-w-0 items-center gap-2 rounded-md border border-border/50 bg-background/60 px-2 py-1.5'>
          <div className='min-w-0 flex-1'>
            {summary ? (
              <Tooltip>
                <TooltipTrigger asChild>
                  <p className='truncate text-xs' data-testid='task-alert-summary'>
                    {config?.cooldown_minutes
                      ? t('summary', {rules: summary.rules, channels: summary.channels, cooldown: summary.cooldown})
                      : t('summaryNoReminder', {rules: summary.rules, channels: summary.channels})}
                  </p>
                </TooltipTrigger>
                <TooltipContent side='top' className='max-w-[300px] text-xs'>
                  {config?.cooldown_minutes
                    ? t('summary', {rules: summary.rules, channels: summary.channels, cooldown: summary.cooldown})
                    : t('summaryNoReminder', {rules: summary.rules, channels: summary.channels})}
                </TooltipContent>
              </Tooltip>
            ) : (
              <p className='truncate text-xs text-muted-foreground' data-testid='task-alert-empty'>
                {t('notConfigured')}
              </p>
            )}
          </div>
          {config?.configured && !config.enabled ? (
            <Badge variant='outline' className='shrink-0 whitespace-nowrap text-[10px]'>
              {t('disabledBadge')}
            </Badge>
          ) : null}
          <Button
            type='button'
            variant='ghost'
            size='sm'
            className='h-6 shrink-0 px-2 text-xs'
            data-testid='task-alert-open'
            disabled={!config?.configured && !canEdit}
            onClick={() => setOpen(true)}
          >
            {config?.configured ? t('edit') : t('configure')}
          </Button>
        </div>
      )}
      <TaskAlertDialog
        open={open}
        taskId={taskId}
        taskName={taskName}
        config={config}
        channels={channels}
        users={users}
        readOnly={!canEdit}
        onOpenChange={setOpen}
        onSaved={setConfig}
      />
    </div>
  );
}
