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

import {useEffect, useMemo, useState} from 'react';
import {useTranslations} from 'next-intl';
import {Loader2} from 'lucide-react';
import {Button} from '@/components/ui/button';
import {Checkbox} from '@/components/ui/checkbox';
import {Input} from '@/components/ui/input';
import {Label} from '@/components/ui/label';
import {Switch} from '@/components/ui/switch';
import type {
  NotificationChannel,
  NotificationRecipientUser,
} from '@/lib/services/monitoring';
import services from '@/lib/services';
import type {
  SyncEventChannelStatus,
  SyncTaskAlertConfig,
  SyncTaskAlertRequest,
  SyncTaskAlertRule,
} from '@/lib/services/sync';
import {
  TASK_ALERT_RULES,
  TASK_ALERT_RULE_CDC_DELAY,
  buildTaskAlertRequest,
  hasEmailChannel,
  initialTaskAlertForm,
  validateTaskAlertForm,
  type TaskAlertFormState,
} from './task-alert-utils';

/**
 * 任务告警配置表单：任务设置侧栏和告警策略中心共用。只负责编辑与校验，保存由调用方完成。
 * Task alert config form shared by the task settings sidebar and the alert policy center. It only edits and
 * validates; the caller saves.
 */
export function TaskAlertForm({
  initial,
  clusterId,
  channels,
  users,
  saving,
  readOnly,
  onSubmit,
  onCancel,
  onDelete,
}: {
  initial: SyncTaskAlertConfig | null;
  /** 任务所在集群，用于提示 B4 的事件通道前置条件；拿不到时不提示。/ Task cluster for the B4 channel hint. */
  clusterId?: number | null;
  channels: NotificationChannel[];
  users: NotificationRecipientUser[];
  saving?: boolean;
  readOnly?: boolean;
  onSubmit: (request: SyncTaskAlertRequest) => void;
  onCancel: () => void;
  onDelete?: () => void;
}) {
  const t = useTranslations('syncAlert');
  const [form, setForm] = useState<TaskAlertFormState>(() => initialTaskAlertForm(initial));
  const [showErrors, setShowErrors] = useState(false);
  useEffect(() => {
    setForm(initialTaskAlertForm(initial));
    setShowErrors(false);
  }, [initial]);

  // 只列出启用的渠道；已选但被停用的渠道仍显示，方便取消勾选。
  // List enabled channels only; selected-but-disabled ones stay visible so they can be unticked.
  const visibleChannels = useMemo(
    () => channels.filter((channel) => channel.enabled || form.channelIds.includes(channel.id)),
    [channels, form.channelIds],
  );
  const emailSelected = hasEmailChannel(form.channelIds, channels);
  const cdcSelected = form.rules.includes(TASK_ALERT_RULE_CDC_DELAY);

  // 勾选 B4 后查询集群事件通道：未开启或不支持时提示这条规则不会触发（保存不拦截）。
  // Once B4 is selected, look up the cluster event channel: warn when it is off or unsupported (saving is allowed).
  const [channelStatus, setChannelStatus] = useState<SyncEventChannelStatus | null>(null);
  useEffect(() => {
    if (!cdcSelected || !clusterId) {
      setChannelStatus(null);
      return;
    }
    let cancelled = false;
    services.sync
      .getEventChannel(clusterId)
      .then((status) => {
        if (!cancelled) {
          setChannelStatus(status);
        }
      })
      .catch(() => {
        if (!cancelled) {
          setChannelStatus(null);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [cdcSelected, clusterId]);
  const channelWarning = channelStatus
    ? !channelStatus.supported
      ? 'cdcDelayChannelUnsupported'
      : !channelStatus.enabled || channelStatus.state === 'disabled'
        ? 'cdcDelayChannelOff'
        : channelStatus.state === 'pending_restart'
          ? 'cdcDelayChannelPendingRestart'
          : null
    : null;
  const error = validateTaskAlertForm(form, channels);

  const update = (patch: Partial<TaskAlertFormState>) => setForm((current) => ({...current, ...patch}));
  const toggle = <T,>(values: T[], value: T, checked: boolean) =>
    checked ? (values.includes(value) ? values : [...values, value]) : values.filter((item) => item !== value);

  const submit = () => {
    if (error) {
      setShowErrors(true);
      return;
    }
    onSubmit(buildTaskAlertRequest(form, channels));
  };

  return (
    <div className='space-y-4' data-testid='task-alert-form'>
      {readOnly ? <p className='rounded-md bg-muted/40 px-3 py-2 text-xs text-muted-foreground'>{t('readOnly')}</p> : null}

      <fieldset className='space-y-2' disabled={readOnly}>
        <Label className='text-xs font-semibold'>{t('rulesLabel')}</Label>
        <div className='grid gap-2 sm:grid-cols-2'>
          {TASK_ALERT_RULES.map((rule: SyncTaskAlertRule) => (
            <label
              key={rule}
              className='flex cursor-pointer items-start gap-2 rounded-md border border-border/60 px-2.5 py-2 text-xs hover:bg-muted/30'
            >
              <Checkbox
                checked={form.rules.includes(rule)}
                data-testid={`task-alert-rule-${rule}`}
                onCheckedChange={(checked) => update({rules: toggle(form.rules, rule, checked === true)})}
              />
              <span className='min-w-0'>
                <span className='block font-medium'>{t(`rule_${rule}`)}</span>
                <span className='block text-[11px] leading-snug text-muted-foreground'>{t(`rule_${rule}_hint`)}</span>
              </span>
            </label>
          ))}
        </div>
      </fieldset>

      {cdcSelected ? (
        <fieldset
          className='space-y-2 rounded-md border border-dashed border-border/70 p-2.5'
          disabled={readOnly}
          data-testid='task-alert-cdc-delay'
        >
          <div className='flex flex-wrap items-end gap-3'>
            <div className='space-y-1'>
              <Label className='whitespace-nowrap text-xs font-semibold'>{t('cdcDelayThresholdLabel')}</Label>
              <Input
                value={form.cdcDelayThreshold}
                inputMode='numeric'
                className='h-8 w-28 text-xs'
                data-testid='task-alert-cdc-threshold'
                aria-invalid={showErrors && error === 'errorCdcDelayThreshold'}
                onChange={(event) => update({cdcDelayThreshold: event.target.value})}
              />
            </div>
            <div className='space-y-1'>
              <Label className='whitespace-nowrap text-xs font-semibold'>{t('cdcDelayDurationLabel')}</Label>
              <Input
                value={form.cdcDelayDuration}
                inputMode='numeric'
                className='h-8 w-28 text-xs'
                data-testid='task-alert-cdc-duration'
                aria-invalid={showErrors && error === 'errorCdcDelayDuration'}
                onChange={(event) => update({cdcDelayDuration: event.target.value})}
              />
            </div>
          </div>
          <p className='text-[11px] leading-snug text-muted-foreground'>{t('cdcDelayThresholdHint')}</p>
          <p className='text-[11px] leading-snug text-muted-foreground'>{t('cdcDelayScopeHint')}</p>
          {channelWarning ? (
            <p className='text-[11px] leading-snug text-amber-600 dark:text-amber-400' data-testid='task-alert-cdc-channel-warning'>
              {t(channelWarning)}
            </p>
          ) : null}
        </fieldset>
      ) : null}

      <fieldset className='space-y-2' disabled={readOnly}>
        <Label className='text-xs font-semibold'>{t('channelsLabel')}</Label>
        {visibleChannels.length === 0 ? (
          <p className='text-xs text-muted-foreground'>{t('noChannels')}</p>
        ) : (
          <div className='flex flex-wrap gap-2'>
            {visibleChannels.map((channel) => (
              <label
                key={channel.id}
                className='flex cursor-pointer items-center gap-1.5 rounded-md border border-border/60 px-2 py-1 text-xs hover:bg-muted/30'
              >
                <Checkbox
                  checked={form.channelIds.includes(channel.id)}
                  data-testid={`task-alert-channel-${channel.id}`}
                  onCheckedChange={(checked) => update({channelIds: toggle(form.channelIds, channel.id, checked === true)})}
                />
                <span className='whitespace-nowrap'>{channel.name}</span>
                <span className='whitespace-nowrap text-[10px] uppercase text-muted-foreground'>{channel.type}</span>
              </label>
            ))}
          </div>
        )}
      </fieldset>

      {emailSelected ? (
        <fieldset className='space-y-2 rounded-md border border-dashed border-border/70 p-2.5' disabled={readOnly}>
          <Label className='text-xs font-semibold'>{t('receiversLabel')}</Label>
          <div className='flex max-h-32 flex-wrap gap-1.5 overflow-y-auto'>
            {users.map((user) => (
              <label key={user.id} className='flex cursor-pointer items-center gap-1.5 rounded border border-border/50 px-1.5 py-0.5 text-[11px]'>
                <Checkbox
                  checked={form.receiverUserIds.includes(user.id)}
                  onCheckedChange={(checked) => update({receiverUserIds: toggle(form.receiverUserIds, user.id, checked === true)})}
                />
                <span className='whitespace-nowrap'>{user.nickname || user.username}</span>
              </label>
            ))}
          </div>
          <label className='flex cursor-pointer items-center gap-2 text-xs'>
            <Checkbox
              checked={form.notifyTaskOwner}
              data-testid='task-alert-notify-owner'
              onCheckedChange={(checked) => update({notifyTaskOwner: checked === true})}
            />
            <span>{t('notifyOwner')}</span>
            <span className='text-[11px] text-muted-foreground'>{t('notifyOwnerHint')}</span>
          </label>
        </fieldset>
      ) : null}

      <fieldset className='grid gap-3 sm:grid-cols-[minmax(0,1fr)_auto_auto] sm:items-end' disabled={readOnly}>
        <div className='space-y-1'>
          <Label className='whitespace-nowrap text-xs font-semibold'>{t('cooldownLabel')}</Label>
          <Input
            value={form.cooldown}
            inputMode='numeric'
            className='h-8 w-28 text-xs'
            data-testid='task-alert-cooldown'
            aria-invalid={showErrors && error === 'errorCooldown'}
            onChange={(event) => update({cooldown: event.target.value})}
          />
          <p className='text-[11px] text-muted-foreground'>{t('cooldownHint')}</p>
        </div>
        <label className='flex items-center gap-2 whitespace-nowrap text-xs'>
          <Switch checked={form.sendRecovery} onCheckedChange={(checked) => update({sendRecovery: checked})} />
          {t('sendRecovery')}
        </label>
        <label className='flex items-center gap-2 whitespace-nowrap text-xs'>
          <Switch checked={form.enabled} data-testid='task-alert-enabled' onCheckedChange={(checked) => update({enabled: checked})} />
          {t('enabled')}
        </label>
      </fieldset>

      {showErrors && error ? (
        <p className='text-xs text-destructive' data-testid='task-alert-error'>
          {t(error)}
        </p>
      ) : null}

      <div className='flex flex-wrap items-center justify-between gap-2 border-t border-border/60 pt-3'>
        <div>
          {onDelete && initial?.configured && !readOnly ? (
            <Button type='button' variant='ghost' size='sm' className='text-destructive' disabled={saving} onClick={onDelete}>
              {t('delete')}
            </Button>
          ) : null}
        </div>
        <div className='flex items-center gap-2'>
          <Button type='button' variant='outline' size='sm' onClick={onCancel} disabled={saving}>
            {t('cancel')}
          </Button>
          {!readOnly ? (
            <Button type='button' size='sm' onClick={submit} disabled={saving} data-testid='task-alert-save'>
              {saving ? <Loader2 className='mr-1 size-3.5 animate-spin' /> : null}
              {saving ? t('saving') : t('save')}
            </Button>
          ) : null}
        </div>
      </div>
    </div>
  );
}
