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
import {Info} from 'lucide-react';
import {Button} from '@/components/arc/button/button';
import {Checkbox} from '@/components/arc/checkbox/checkbox';
import {MultiSelect} from '@/components/arc/multi-select/multi-select';
import {NumberField} from '@/components/arc/number-field/number-field';
import {Switch} from '@/components/arc/switch/switch';
import {Tooltip} from '@/components/arc/tooltip/tooltip';
import ConfirmMorph from '@/components/arc/confirm-morph/confirm-morph';
import {Label} from '@/components/ui/label';
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
  parseCdcDelayDuration,
  parseCdcDelayThreshold,
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
  onDirtyChange,
  pinnedFooter,
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
  onDelete?: () => void | Promise<void>;
  /** 表单相对 initial 是否被改，供宿主抽屉拦截未保存关闭。Whether form diverges from initial, so the host drawer can intercept an unsaved close. */
  onDirtyChange?: (dirty: boolean) => void;
  /** 抽屉整高布局：字段区独立滚动、操作栏钉在底部；宿主需给 drawer body flex-1。Drawer layout: fields scroll, action bar pinned to the bottom. */
  pinnedFooter?: boolean;
}) {
  const t = useTranslations('syncAlert');
  const [form, setForm] = useState<TaskAlertFormState>(() => initialTaskAlertForm(initial));
  const [showErrors, setShowErrors] = useState(false);
  useEffect(() => {
    setForm(initialTaskAlertForm(initial));
    setShowErrors(false);
  }, [initial]);
  useEffect(() => {
    onDirtyChange?.(JSON.stringify(form) !== JSON.stringify(initialTaskAlertForm(initial)));
  }, [form, initial, onDirtyChange]);

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
  // B4 参数错误显示在输入框下方（与任务超时输入一致），底部不再重复。
  // B4 parameter errors show under their inputs (like the task timeout input) and are not repeated at the bottom.
  const cdcThresholdInvalid = cdcSelected && parseCdcDelayThreshold(form.cdcDelayThreshold) === null;
  const cdcDurationInvalid = cdcSelected && parseCdcDelayDuration(form.cdcDelayDuration) === null;
  const bottomError = error === 'errorCdcDelayThreshold' || error === 'errorCdcDelayDuration' ? null : error;

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

  const body = (
    <>
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
          <div className='flex flex-wrap items-start gap-3'>
            <div className='space-y-1'>
              <div data-testid='task-alert-cdc-threshold'>
                <NumberField
                  label={t('cdcDelayThresholdLabel')}
                  labelAddon={
                    <Tooltip
                      content={
                        <span className='block max-w-xs space-y-1 text-[11px] leading-snug'>
                          <span className='block'>{t('cdcDelayThresholdHint')}</span>
                          <span className='block'>{t('cdcDelayScopeHint')}</span>
                        </span>
                      }
                    >
                      <button
                        type='button'
                        className='text-muted-foreground hover:text-foreground'
                        data-testid='task-alert-cdc-hint'
                        aria-label={`${t('cdcDelayThresholdHint')} ${t('cdcDelayScopeHint')}`}
                      >
                        <Info className='size-3.5' />
                      </button>
                    </Tooltip>
                  }
                  value={Number(form.cdcDelayThreshold) || 0}
                  min={10}
                  onValueChange={(v) => update({cdcDelayThreshold: String(v)})}
                />
              </div>
              {cdcThresholdInvalid ? (
                <p className='max-w-56 text-[10px] text-destructive' data-testid='task-alert-cdc-threshold-error'>
                  {t('errorCdcDelayThreshold')}
                </p>
              ) : null}
            </div>
            <div className='space-y-1'>
              <div data-testid='task-alert-cdc-duration'>
                <NumberField
                  label={t('cdcDelayDurationLabel')}
                  value={Number(form.cdcDelayDuration) || 0}
                  min={1}
                  max={60}
                  onValueChange={(v) => update({cdcDelayDuration: String(v)})}
                />
              </div>
              {cdcDurationInvalid ? (
                <p className='max-w-56 text-[10px] text-destructive' data-testid='task-alert-cdc-duration-error'>
                  {t('errorCdcDelayDuration')}
                </p>
              ) : null}
            </div>
          </div>
          {channelWarning ? (
            <p className='text-[11px] leading-snug text-amber-600 dark:text-amber-400' data-testid='task-alert-cdc-channel-warning'>
              {t(channelWarning)}
            </p>
          ) : null}
        </fieldset>
      ) : null}

      <fieldset className='space-y-2' disabled={readOnly}>
        {visibleChannels.length === 0 ? (
          <>
            <Label className='text-xs font-semibold'>{t('channelsLabel')}</Label>
            <p className='text-xs text-muted-foreground'>{t('noChannels')}</p>
          </>
        ) : (
          <div data-testid='task-alert-channels'>
            <MultiSelect
              label={t('channelsLabel')}
              options={visibleChannels.map((channel) => ({
                value: String(channel.id),
                label: channel.name,
              }))}
              value={form.channelIds.map(String)}
              onValueChange={(v) => update({channelIds: v.map(Number)})}
              placeholder={t('channelsLabel')}
              searchPlaceholder={t('selectSearchPlaceholder')}
              noMatchMessage={t('comboboxEmpty')}
            />
          </div>
        )}
      </fieldset>

      {emailSelected ? (
        <fieldset className='space-y-2 rounded-md border border-dashed border-border/70 p-2.5' disabled={readOnly}>
          <MultiSelect
            label={t('receiversLabel')}
            options={users.map((user) => ({
              value: String(user.id),
              label: user.nickname || user.username,
            }))}
            value={form.receiverUserIds.map(String)}
            onValueChange={(v) => update({receiverUserIds: v.map(Number)})}
            placeholder={t('receiversPlaceholder')}
            searchPlaceholder={t('selectSearchPlaceholder')}
            noMatchMessage={t('comboboxEmpty')}
          />
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
          <div className='w-36' data-testid='task-alert-cooldown'>
            <NumberField
              label={t('cooldownLabel')}
              value={Number(form.cooldown) || 0}
              min={0}
              onValueChange={(v) => update({cooldown: String(v)})}
            />
          </div>
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

      {showErrors && bottomError ? (
        <p className='text-xs text-destructive' data-testid='task-alert-error'>
          {t(bottomError)}
        </p>
      ) : null}
    </>
  );

  return (
    <div
      className={pinnedFooter ? 'flex h-full flex-col' : 'space-y-4'}
      data-testid='task-alert-form'
    >
      {pinnedFooter ? (
        // 抽屉内字段区独立滚动，操作栏由 flex 钉在底部（宿主需已抵消 body 内边距）。
        // In a drawer the fields scroll on their own and the action bar is pinned
        // by flex; the host is expected to have cancelled the body padding.
        <div className='min-h-0 flex-1 space-y-4 overflow-y-auto overscroll-contain px-5 py-5 sm:px-6 sm:py-6'>
          {body}
        </div>
      ) : (
        body
      )}
      <div
        className={
          pinnedFooter
            ? 'flex flex-wrap items-center justify-between gap-2 border-t border-(--border-subtle) bg-(--surface-raised) px-5 py-3 sm:px-6'
            : 'flex flex-wrap items-center justify-between gap-2 border-t border-border/60 pt-3'
        }
      >
        <div>
          {onDelete && initial?.configured && !readOnly ? (
            <ConfirmMorph
              label={t('delete')}
              prompt={t('deleteConfirm')}
              confirmLabel={t('delete')}
              cancelLabel={t('cancel')}
              pendingLabel={t('saving')}
              onConfirm={onDelete}
            />
          ) : null}
        </div>
        <div className='flex items-center gap-2'>
          <Button type='button' variant='secondary' size='sm' onClick={onCancel} disabled={saving}>
            {t('cancel')}
          </Button>
          {!readOnly ? (
            <Button type='button' size='sm' loading={saving} onClick={submit} disabled={saving} data-testid='task-alert-save'>
              {saving ? t('saving') : t('save')}
            </Button>
          ) : null}
        </div>
      </div>
    </div>
  );
}
