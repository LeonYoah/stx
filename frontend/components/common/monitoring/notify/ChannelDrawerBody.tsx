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
 * (label above control) + deliveries list, actions pinned to the bottom bar
 * with the dirty-close confirm strip. Used by the persistent rail and the
 * narrow drawer alike.
 * 渠道编辑体：detailCol 三段式（滚动表单 + 底部操作栏 + 脏关闭确认条），
 * 常驻栏与窄抽屉共用。
 */
import {useEffect, useMemo, useState} from 'react';
import {useTranslations} from 'next-intl';
import {DeliveryChip} from '../status-chips';
import {Button} from '@/components/arc/button/button';
import {Input} from '@/components/arc/input/input';
import {Select} from '@/components/arc/select/select';
import {Switch} from '@/components/arc/switch/switch';
import services from '@/lib/services';
import type {
  NotificationChannel,
  NotificationDelivery,
} from '@/lib/services/monitoring';
import {cn} from '@/lib/utils';
import {DiscardConfirmStrip} from '../dirty-close';
import {formatAgo, minutesAgo, type MonitoringTranslator} from '../monitoring-utils';
import styles from '../monitoring.module.css';

const CHANNEL_TYPES = ['webhook', 'email', 'dingtalk', 'wecom', 'feishu'] as const;

export function ChannelDrawerBody({
  channel,
  onSaved,
  onCancel,
  onDirtyChange,
  discardArmed,
  onDiscard,
  onKeepEditing,
}: {
  channel: NotificationChannel | null;
  onSaved: () => void;
  onCancel: () => void;
  onDirtyChange?: (dirty: boolean) => void;
  discardArmed?: boolean;
  onDiscard?: () => void;
  onKeepEditing?: () => void;
}) {
  const t = useTranslations('monitoring');
  const [form, setForm] = useState({
    name: channel?.name ?? '',
    type: channel?.type ?? 'webhook',
    enabled: channel?.enabled ?? true,
    endpoint: channel?.endpoint ?? '',
    secret: '',
    emailTo: (channel?.config?.email?.recipients ?? []).join(', '),
  });
  const [deliveries, setDeliveries] = useState<NotificationDelivery[]>([]);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const base = useMemo(
    () => ({
      name: channel?.name ?? '',
      type: channel?.type ?? 'webhook',
      enabled: channel?.enabled ?? true,
      endpoint: channel?.endpoint ?? '',
      secret: '',
      emailTo: (channel?.config?.email?.recipients ?? []).join(', '),
    }),
    [channel],
  );
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

  const save = async () => {
    setSaving(true);
    setError(null);
    try {
      const body: Record<string, unknown> = {
        name: form.name,
        type: form.type,
        enabled: form.enabled,
        endpoint: form.endpoint || undefined,
        secret: form.secret || undefined,
      };
      if (form.type === 'email') {
        // 收件地址走 config.email.recipients（逗号分隔），其余 SMTP 字段沿用已存配置。
        // Recipient addresses map to config.email.recipients (comma-separated); the
        // other SMTP fields keep their stored values.
        const email = channel?.config?.email;
        body.config = {
          email: {
            protocol: email?.protocol ?? 'smtp',
            security: email?.security ?? 'none',
            host: email?.host ?? '',
            port: email?.port ?? 25,
            username: email?.username,
            from: email?.from ?? '',
            from_name: email?.from_name,
            recipients: form.emailTo.split(',').map((x) => x.trim()).filter(Boolean),
          },
        };
      }
      if (channel) {
        await services.monitoring.updateNotificationChannel(channel.id, body as never);
      } else {
        await services.monitoring.createNotificationChannel(body as never);
      }
      onSaved();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className={styles.detailCol}>
      <div className={styles.detailScroll}>
        <div className={styles.field}>
          <Input label={t('channelName')} value={form.name} onChange={(e) => setForm((f) => ({...f, name: e.target.value}))} />
        </div>
        <div className={styles.field}>
          <Select
            label={t('channelTypeLabel')}
            options={CHANNEL_TYPES.map((type) => ({value: type, label: t(`channelType.${type}`)}))}
            value={form.type}
            onValueChange={(v) => setForm((f) => ({...f, type: v as typeof f.type}))}
          />
        </div>
        {form.type !== 'email' ? (
          <div className={styles.field}>
            <Input
              label={t('channelEndpoint')}
              value={form.endpoint}
              onChange={(e) => setForm((f) => ({...f, endpoint: e.target.value}))}
              placeholder='https://…'
            />
            <span className={styles.fieldDesc}>{t('secretKeepHint')}</span>
          </div>
        ) : (
          <div className={styles.field}>
            <Input
              label={t('channelEmailTo')}
              value={form.emailTo}
              onChange={(e) => setForm((f) => ({...f, emailTo: e.target.value}))}
              placeholder='ops@example.com'
            />
            <span className={styles.fieldDesc}>{t('channelEmailToHint')}</span>
          </div>
        )}
        <div className={styles.field}>
          <span className={styles.fieldLabel}>{t('fieldEnabled')}</span>
          <Switch
            checked={form.enabled}
            onCheckedChange={(v) => setForm((f) => ({...f, enabled: v}))}
            aria-label={t('fieldEnabled')}
          />
        </div>
        {error ? (
          <div className={cn(styles.hintBox, styles.dangerText)} role='alert'>
            {error}
          </div>
        ) : null}
        {channel && deliveries.length > 0 ? (
          <div className={styles.sectionStack} style={{gap: 8}}>
            <span className={styles.sectionTitle}>{t('deliveries')}</span>
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

      {discardArmed ? (
        <DiscardConfirmStrip onDiscard={onDiscard ?? (() => {})} onKeepEditing={onKeepEditing ?? (() => {})} />
      ) : (
        <div className={styles.detailFoot}>
          <Button
            type='button'
            size='sm'
            loading={saving}
            disabled={!form.name.trim()}
            onClick={() => void save()}
          >
            {t('save')}
          </Button>
          <Button type='button' size='sm' variant='ghost' onClick={onCancel}>
            {t('close')}
          </Button>
        </div>
      )}
    </div>
  );
}

function DeliveryLine({d, t}: {d: NotificationDelivery; t: MonitoringTranslator}) {
  return (
    <>
      <span className={cn(styles.truncate, styles.mono, styles.xsText)}>
        {d.alert_name || d.alert_id}
      </span>
      <span className='flex shrink-0 items-center gap-2'>
        <DeliveryChip
          status={d.status}
          label={t(`delivery.${d.status}`)}
        />
        <span className={cn(styles.sub, styles.num, styles.nowrap)}>
          {formatAgo(minutesAgo(d.sent_at || d.created_at), t)}
        </span>
      </span>
    </>
  );
}
