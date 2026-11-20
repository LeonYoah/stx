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
 * Channel edit/create body inside the shared detail column: scrolling form
 * (label above control) + deliveries list, actions pinned to the bottom bar.
 * Used by the persistent rail, the narrow drawer, and the centered create
 * dialog alike. Webhook-like types carry the masked endpoint/secret keep
 * contract; the email type carries a full SMTP form.
 * 渠道编辑体：detailCol 两段式（滚动表单 + 底部操作栏），常驻栏、窄抽屉与新建
 * 弹窗共用。Webhook 系渠道遵守地址/密钥脱敏保持契约，邮件渠道带完整 SMTP 表单；
 * 表单内可直接发送测试（草稿测通 / SMTP 连接测试），结果内联展示。
 */
import {useCallback, useEffect, useMemo, useRef, useState} from 'react';
import {useTranslations} from 'next-intl';
import {Trash2} from 'lucide-react';
import {DeliveryChip} from '../status-chips';

import {Button} from '@/components/arc/button/button';
import ConfirmMorph from '@/components/arc/confirm-morph/confirm-morph';
import {Input} from '@/components/arc/input/input';
import {NumberField} from '@/components/arc/number-field/number-field';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {Select} from '@/components/arc/select/select';
import {Switch} from '@/components/arc/switch/switch';
import services from '@/lib/services';
import type {
  NotificationChannel,
  NotificationChannelType,
  NotificationDelivery,
  NotificationEmailSecurity,
  UpsertNotificationChannelRequest,
} from '@/lib/services/monitoring';
import {cn} from '@/lib/utils';
import {formatAgo, minutesAgo, type MonitoringTranslator} from '../monitoring-utils';
import {
  NOTIFICATION_SECRET_MASK,
  buildWebhookChannelPayload,
  channelTypeSupportsSecret,
  changeWebhookChannelType,
  endpointPlaceholderForType,
  isWebhookLikeChannelType,
  savedWebhookChannelSecret,
  type WebhookLikeChannelType,
} from '../notification-channel-utils';
import styles from '../monitoring.module.css';
import layout from '@/components/common/layout/list-detail.module.css';

const CHANNEL_TYPES: NotificationChannelType[] = [
  'webhook',
  'email',
  'dingtalk',
  'wecom',
  'feishu',
];
const EMAIL_SECURITY_OPTIONS: NotificationEmailSecurity[] = [
  'ssl',
  'starttls',
  'none',
];

type EmailFormState = {
  host: string;
  port: number;
  security: NotificationEmailSecurity;
  username: string;
  password: string;
  from: string;
  fromName: string;
  recipients: string;
};

type ChannelFormState = {
  name: string;
  type: NotificationChannelType;
  enabled: boolean;
  description: string;
  endpoint: string;
  secret: string;
  clearSecret: boolean;
  email: EmailFormState;
};

function formFromChannel(channel: NotificationChannel | null): ChannelFormState {
  const email = channel?.config?.email;
  return {
    name: channel?.name ?? '',
    type: channel?.type ?? 'webhook',
    enabled: channel?.enabled ?? true,
    description: channel?.description ?? '',
    endpoint: channel?.endpoint ?? '',
    secret: channel?.secret ?? '',
    clearSecret: false,
    email: {
      host: email?.host ?? '',
      port: email?.port ?? 465,
      security: email?.security ?? 'ssl',
      username: email?.username ?? '',
      password: email?.password ?? '',
      from: email?.from ?? '',
      fromName: email?.from_name ?? '',
      recipients: (email?.recipients ?? []).join(', '),
    },
  };
}

export function ChannelDrawerBody({
  channel,
  onSaved,
  onCancel,
  onDelete,
  onDirtyChange,
}: {
  channel: NotificationChannel | null;
  onSaved: () => void;
  /** 已有渠道的删除入口（详情页内承载破坏性操作）。Delete entry for an existing channel. */
  onDelete?: () => Promise<void>;
  onCancel: () => void;
  onDirtyChange?: (dirty: boolean) => void;
}) {
  const t = useTranslations('monitoring');
  const [form, setForm] = useState<ChannelFormState>(() =>
    formFromChannel(channel),
  );
  const [deliveries, setDeliveries] = useState<NotificationDelivery[]>([]);
  const [saving, setSaving] = useState(false);
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<{
    ok: boolean;
    detail?: string;
  } | null>(null);
  const [error, setError] = useState<string | null>(null);
  // 发送测试时的表单快照：表单再变动时旧结果标记为"可能已过期"。
  // Snapshot of the form at test time; later edits mark the result stale.
  const testedSnapshot = useRef<string | null>(null);

  const base = useMemo(() => formFromChannel(channel), [channel]);
  const saved = useMemo(() => savedWebhookChannelSecret(channel), [channel]);
  const dirty = JSON.stringify(form) !== JSON.stringify(base);

  useEffect(() => {
    onDirtyChange?.(dirty);
    return () => onDirtyChange?.(false);
  }, [dirty, onDirtyChange]);

  useEffect(() => {
    if (channel) {
      services.monitoring
        .listNotificationDeliveries({channel_id: channel.id, page: 1, page_size: 10})
        .then((d) => setDeliveries(d.deliveries ?? []))
        .catch(() => undefined);
    }
  }, [channel]);

  const setEmail = useCallback((patch: Partial<EmailFormState>) => {
    setForm((f) => ({...f, email: {...f.email, ...patch}}));
  }, []);

  // 切换类型复用 utils 的密钥语义：密钥不跨平台沿用，切回原类型时恢复占位符。
  // Type switching reuses the util secret semantics: secrets never carry across
  // platforms, and the mask is restored when switching back to the saved type.
  const changeType = useCallback(
    (next: NotificationChannelType) => {
      setForm((f) => {
        if (next === f.type) {
          return f;
        }
        if (!isWebhookLikeChannelType(next)) {
          return {...f, type: next};
        }
        const moved = changeWebhookChannelType(
          {
            id: channel?.id ?? null,
            type: isWebhookLikeChannelType(f.type) ? f.type : 'webhook',
            name: f.name,
            enabled: f.enabled,
            description: f.description,
            endpoint: f.endpoint,
            secret: f.secret,
            clearSecret: f.clearSecret,
          },
          next as WebhookLikeChannelType,
          saved,
        );
        return {
          ...f,
          type: next,
          secret: moved.secret,
          clearSecret: moved.clearSecret,
        };
      });
    },
    [channel?.id, saved],
  );

  const buildPayload = useCallback((): UpsertNotificationChannelRequest => {
    if (isWebhookLikeChannelType(form.type)) {
      return buildWebhookChannelPayload({
        id: channel?.id ?? null,
        type: form.type,
        name: form.name,
        enabled: form.enabled,
        description: form.description,
        endpoint: form.endpoint,
        secret: form.secret,
        clearSecret: form.clearSecret,
      });
    }
    const email = form.email;
    // 密码留空或为脱敏占位符时后端沿用已存值（同 secret 契约）。
    // An empty or masked password keeps the stored value on the backend (same
    // contract as the webhook secret).
    return {
      name: form.name.trim(),
      type: 'email',
      enabled: form.enabled,
      description: form.description.trim(),
      config: {
        email: {
          protocol: 'smtp',
          security: email.security,
          host: email.host.trim(),
          port: email.port,
          username: email.username.trim() || undefined,
          password: email.password || undefined,
          from: email.from.trim(),
          from_name: email.fromName.trim() || undefined,
          recipients: email.recipients
            .split(',')
            .map((x) => x.trim())
            .filter(Boolean),
        },
      },
    };
  }, [channel?.id, form]);

  const canSave =
    form.name.trim() !== '' &&
    (form.type === 'email'
      ? form.email.host.trim() !== '' && form.email.from.trim() !== ''
      : form.endpoint.trim() !== '');

  const save = async () => {
    setSaving(true);
    setError(null);
    try {
      const payload = buildPayload();
      if (channel) {
        await services.monitoring.updateNotificationChannel(
          channel.id,
          payload,
        );
      } else {
        await services.monitoring.createNotificationChannel(payload);
      }
      onSaved();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setSaving(false);
    }
  };

  // webhook 系走草稿测通（真实发送），邮件走 SMTP 连接测试；编辑已保存渠道时
  // 传 channel_id，后端用真实值补齐脱敏的密钥 / 地址。
  // Webhook-like types send a real draft test; email runs the SMTP connection
  // probe. When editing a saved channel, channel_id lets the backend fill the
  // masked secrets with the stored values.
  const runTest = async () => {
    const payload = buildPayload();
    setTesting(true);
    try {
      if (form.type === 'email') {
        const res = await services.monitoring.testNotificationChannelConnection(
          {...payload, channel_id: channel?.id},
        );
        const ok = res.status === 'sent';
        setTestResult({
          ok,
          detail: ok ? undefined : res.last_error || res.status,
        });
      } else {
        const res = await services.monitoring.testNotificationChannelDraft({
          channel: payload,
          channel_id: channel?.id,
        });
        const ok = res.status === 'sent' || res.status === 'success';
        setTestResult({
          ok,
          detail: ok
            ? res.status_code
              ? String(res.status_code)
              : undefined
            : res.last_error || res.status,
        });
      }
    } catch (err) {
      setTestResult({
        ok: false,
        detail: err instanceof Error ? err.message : String(err),
      });
    } finally {
      setTesting(false);
      testedSnapshot.current = JSON.stringify(form);
    }
  };

  const testStale =
    testResult !== null &&
    testedSnapshot.current !== null &&
    testedSnapshot.current !== JSON.stringify(form);

  const isWebhookType = isWebhookLikeChannelType(form.type);
  const secretChangedType = isWebhookType && channel?.type !== form.type;

  return (
    <div className={layout.detailCol}>
      <div className={layout.detailScroll}>
        <div className={layout.field}>
          <Input
            label={t('channelName')}
            value={form.name}
            onChange={(e) =>
              setForm((f) => ({...f, name: e.target.value}))
            }
            autoComplete='off'
          />
        </div>
        <div className={layout.field}>
          <span className={layout.fieldLabel}>{t('channelTypeLabel')}</span>
          <SegmentedControl
            label={t('channelTypeLabel')}
            value={form.type}
            onValueChange={(v) => changeType(v as NotificationChannelType)}
            options={CHANNEL_TYPES.map((type) => ({
              value: type,
              label: t(`channelType.${type}`),
            }))}
          />
          {secretChangedType && saved?.hasSecret ? (
            <span className={styles.fieldDesc}>
              {t('secretDroppedOnTypeChange')}
            </span>
          ) : null}
        </div>
        {isWebhookType ? (
          <>
            <div className={layout.field}>
              <Input
                label={t('channelEndpoint')}
                value={form.endpoint}
                onChange={(e) =>
                  setForm((f) => ({...f, endpoint: e.target.value}))
                }
                placeholder={endpointPlaceholderForType(
                  form.type as WebhookLikeChannelType,
                )}
                autoComplete='off'
              />
              <span className={styles.fieldDesc}>
                {t(`endpointHint.${form.type}`)}
              </span>
              {channel && form.endpoint.includes(NOTIFICATION_SECRET_MASK) ? (
                <span className={styles.fieldDesc}>{t('maskedKeepHint')}</span>
              ) : null}
            </div>
            {channelTypeSupportsSecret(form.type as WebhookLikeChannelType) ? (
              <div className={layout.field}>
                <Input
                  label={t('channelSecret')}
                  type='password'
                  value={form.secret}
                  onChange={(e) =>
                    setForm((f) => ({
                      ...f,
                      secret: e.target.value,
                      // 留空不清除已保存密钥，填新值撤销已选清除（同
                      // utils.setWebhookChannelSecret 的语义）。
                      // Empty keeps the stored secret; a new value cancels a
                      // pending clear (same semantics as utils.setWebhookChannelSecret).
                      clearSecret:
                        e.target.value.trim() === '' ? f.clearSecret : false,
                    }))
                  }
                  onFocus={(e) => {
                    if (form.secret === NOTIFICATION_SECRET_MASK) {
                      e.target.select();
                    }
                  }}
                  placeholder={t('secretOptional')}
                  autoComplete='new-password'
                />
                <span className={styles.fieldDesc}>
                  {t(`secretHint.${form.type}`)}
                </span>
                {form.clearSecret ? (
                  <span className={cn(styles.fieldDesc, layout.dangerText)}>
                    {t('clearSecretDone')}{' '}
                    <button
                      type='button'
                      className='underline underline-offset-2'
                      onClick={() =>
                        setForm((f) => ({
                          ...f,
                          clearSecret: false,
                          secret: saved?.hasSecret
                            ? NOTIFICATION_SECRET_MASK
                            : '',
                        }))
                      }
                    >
                      {t('undoClear')}
                    </button>
                  </span>
                ) : channel &&
                  saved?.hasSecret &&
                  channel.type === form.type ? (
                  <ConfirmMorph
                    label={t('clearSecret')}
                    prompt={t('clearSecretPrompt')}
                    confirmLabel={t('clearSecretConfirm')}
                    cancelLabel={t('deleteCancel')}
                    pendingLabel={t('saving')}
                    onConfirm={async () =>
                      setForm((f) => ({...f, clearSecret: true, secret: ''}))
                    }
                  />
                ) : null}
              </div>
            ) : null}
          </>
        ) : (
          <>
            <div className={styles.fieldRow}>
              <div className={layout.field}>
                <Input
                  label={t('emailHost')}
                  value={form.email.host}
                  onChange={(e) => setEmail({host: e.target.value})}
                  placeholder='smtp.example.com'
                />
              </div>
              <div className={layout.field}>
                <NumberField
                  label={t('emailPort')}
                  value={form.email.port}
                  onValueChange={(v) => setEmail({port: v})}
                  min={1}
                  max={65535}
                />
              </div>
            </div>
            <div className={layout.field}>
              <Select
                label={t('emailSecurity')}
                options={EMAIL_SECURITY_OPTIONS.map((s) => ({
                  value: s,
                  label: t(`emailSecurityOption.${s}`),
                }))}
                value={form.email.security}
                onValueChange={(v) =>
                  setEmail({security: v as NotificationEmailSecurity})
                }
              />
            </div>
            <div className={styles.fieldRow}>
              <div className={layout.field}>
                <Input
                  label={t('emailUsername')}
                  value={form.email.username}
                  onChange={(e) => setEmail({username: e.target.value})}
                  autoComplete='off'
                />
              </div>
              <div className={layout.field}>
                <Input
                  label={t('emailPassword')}
                  type='password'
                  value={form.email.password}
                  onChange={(e) => setEmail({password: e.target.value})}
                  autoComplete='new-password'
                />
                {channel ? (
                  <span className={styles.fieldDesc}>
                    {t('emailPasswordHint')}
                  </span>
                ) : null}
              </div>
            </div>
            <div className={styles.fieldRow}>
              <div className={layout.field}>
                <Input
                  label={t('emailFrom')}
                  value={form.email.from}
                  onChange={(e) => setEmail({from: e.target.value})}
                  placeholder='alerts@example.com'
                />
              </div>
              <div className={layout.field}>
                <Input
                  label={t('emailFromName')}
                  value={form.email.fromName}
                  onChange={(e) => setEmail({fromName: e.target.value})}
                />
              </div>
            </div>
            <div className={layout.field}>
              <Input
                label={t('emailRecipients')}
                value={form.email.recipients}
                onChange={(e) => setEmail({recipients: e.target.value})}
                placeholder='ops@example.com, sre@example.com'
              />
              <span className={styles.fieldDesc}>{t('channelEmailToHint')}</span>
            </div>
          </>
        )}
        <div className={layout.field}>
          <Input
            label={t('fieldDescription')}
            value={form.description}
            onChange={(e) =>
              setForm((f) => ({...f, description: e.target.value}))
            }
          />
        </div>
        <div className={layout.field}>
          <span className={layout.fieldLabel}>{t('fieldEnabled')}</span>
          <Switch
            checked={form.enabled}
            onCheckedChange={(v) => setForm((f) => ({...f, enabled: v}))}
            aria-label={t('fieldEnabled')}
          />
        </div>
        {error ? (
          <div className={cn(styles.hintBox, layout.dangerText)} role='alert'>
            {error}
          </div>
        ) : null}
        {testResult ? (
          <div
            className={cn(
              styles.hintBox,
              testResult.ok ? layout.muted : layout.dangerText,
            )}
            role='status'
          >
            {testResult.ok ? t('testOk') : t('testFailed')}
            {testResult.detail ? `：${testResult.detail}` : ''}
            {testStale ? ` · ${t('testResultStale')}` : ''}
          </div>
        ) : null}
        {channel && deliveries.length > 0 ? (
          <div className={layout.sectionStack} style={{gap: 8}}>
            <span className={layout.sectionTitle}>{t('deliveries')}</span>
            <ul className={styles.lineList}>
              {deliveries.map((d) => (
                <li key={d.id} className={styles.lineItem}>
                  <DeliveryLine d={d} t={t} />
                </li>
              ))}
            </ul>
          </div>
        ) : null}
      </div>

      <div className={layout.detailFoot}>
        <Button
          type='button'
          size='sm'
          loading={saving}
          disabled={!canSave}
          onClick={() => void save()}
        >
          {t('save')}
        </Button>
        <Button
          type='button'
          size='sm'
          variant='secondary'
          loading={testing}
          disabled={!canSave}
          onClick={() => void runTest()}
        >
          {form.type === 'email' ? t('testConnection') : t('sendTest')}
        </Button>
        <Button type='button' size='sm' variant='ghost' onClick={onCancel}>
          {t('close')}
        </Button>
        {channel && onDelete ? (
          <>
            <span className={layout.detailFootGrow} />
            <ConfirmMorph
              label={t('deleteChannel')}
              icon={<Trash2 className='size-3.5' aria-hidden />}
              prompt={t('deleteChannelPrompt')}
              confirmLabel={t('deleteConfirm')}
              cancelLabel={t('deleteCancel')}
              pendingLabel={t('saving')}
              onConfirm={onDelete}
            />
          </>
        ) : null}
      </div>
    </div>
  );
}

function DeliveryLine({d, t}: {d: NotificationDelivery; t: MonitoringTranslator}) {
  return (
    <>
      <span className={cn(styles.truncate, styles.mono, layout.xsText)}>
        {d.alert_name || d.alert_id}
      </span>
      <span className='flex shrink-0 items-center gap-2'>
        <DeliveryChip
          status={d.status}
          label={t(`delivery.${d.status}`)}
        />
        <span className={cn(layout.sub, styles.num, styles.nowrap)}>
          {formatAgo(minutesAgo(d.sent_at || d.created_at), t)}
        </span>
      </span>
    </>
  );
}
