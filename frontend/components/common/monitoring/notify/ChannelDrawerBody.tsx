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
 * Channel drawer body: create/edit form for webhook-like and email channels,
 * plus the channel's recent deliveries for saved channels.
 * 渠道抽屉表单：webhook 系与邮件渠道的新建/编辑；已保存渠道底部展示最近投递记录。
 */
import {useCallback, useEffect, useState} from 'react';
import {useTranslations} from 'next-intl';
import {Badge} from '@/components/arc/badge/badge';
import {Input} from '@/components/arc/input/input';
import {Select} from '@/components/arc/select/select';
import {Switch} from '@/components/arc/switch/switch';
import {Button} from '@/components/ui/button';
import services from '@/lib/services';
import type {
  NotificationChannel,
  NotificationChannelEmailConfig,
  NotificationChannelType,
  NotificationDelivery,
  UpsertNotificationChannelRequest,
} from '@/lib/services/monitoring';
import {
  channelTypeSupportsSecret,
  createDefaultWebhookChannelForm,
  createWebhookChannelFormFromChannel,
  endpointPlaceholderForType,
  isWebhookLikeChannelType,
  type WebhookChannelFormState,
} from '../notification-channel-utils';
import {formatAgo, minutesAgo} from '../monitoring-utils';
import {DiscardConfirmStrip} from '../dirty-close';

type FormState =
  | {kind: 'webhook'; form: WebhookChannelFormState}
  | {kind: 'email'; form: EmailDraft};

interface EmailDraft {
  name: string;
  enabled: boolean;
  description: string;
  protocol: string;
  security: 'none' | 'starttls' | 'ssl';
  host: string;
  port: string;
  username: string;
  password: string;
  from: string;
  from_name: string;
  recipients: string;
}

const CHANNEL_TYPES: NotificationChannelType[] = ['webhook', 'email', 'dingtalk', 'wecom', 'feishu'];

function defaultEmailDraft(): EmailDraft {
  return {name: '', enabled: true, description: '', protocol: 'smtp', security: 'ssl', host: '', port: '465', username: '', password: '', from: '', from_name: '', recipients: ''};
}

function emailDraftFromChannel(c: NotificationChannel): EmailDraft {
  const e = c.config?.email;
  return {
    name: c.name,
    enabled: c.enabled,
    description: c.description || '',
    protocol: e?.protocol || 'smtp',
    security: e?.security || 'ssl',
    host: e?.host || '',
    port: String(e?.port ?? 465),
    username: e?.username || '',
    password: e?.password || '',
    from: e?.from || '',
    from_name: e?.from_name || '',
    recipients: (e?.recipients ?? []).join(', '),
  };
}

function buildPayload(state: FormState): UpsertNotificationChannelRequest {
  if (state.kind === 'webhook') {
    const f = state.form;
    return {
      name: f.name.trim(),
      type: f.type,
      enabled: f.enabled,
      endpoint: f.endpoint.trim(),
      secret: f.secret,
      clear_secret: f.clearSecret || undefined,
      description: f.description.trim(),
    };
  }
  const f = state.form;
  const email: NotificationChannelEmailConfig = {
    protocol: f.protocol,
    security: f.security,
    host: f.host.trim(),
    port: Number.parseInt(f.port, 10) || 0,
    username: f.username.trim() || undefined,
    password: f.password || undefined,
    from: f.from.trim(),
    from_name: f.from_name.trim() || undefined,
    recipients: f.recipients.split(/[,;\s]+/).map((s) => s.trim()).filter(Boolean),
  };
  return {
    name: f.name.trim(),
    type: 'email',
    enabled: f.enabled,
    description: f.description.trim(),
    config: {email},
  };
}

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
  onSaved: () => void | Promise<void>;
  onCancel: () => void;
  /** 表单相对打开快照是否已修改，供宿主拦截关闭。Whether the form diverges from its open-time snapshot. */
  onDirtyChange?: (dirty: boolean) => void;
  /** 关闭被拦截后置位，footer 切换为放弃确认行。Set when a close attempt was intercepted. */
  discardArmed?: boolean;
  onDiscard?: () => void;
  onKeepEditing?: () => void;
}) {
  const t = useTranslations('monitoring');
  const [state, setState] = useState<FormState>(() => {
    if (!channel) {
      return {kind: 'webhook', form: createDefaultWebhookChannelForm()};
    }
    if (isWebhookLikeChannelType(channel.type)) {
      return {kind: 'webhook', form: createWebhookChannelFormFromChannel(channel)};
    }
    return {kind: 'email', form: emailDraftFromChannel(channel)};
  });
  const [snapshot, setSnapshot] = useState(() => JSON.stringify(state));
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [deliveries, setDeliveries] = useState<NotificationDelivery[]>([]);

  const dirty = JSON.stringify(state) !== snapshot;
  useEffect(() => {
    onDirtyChange?.(dirty);
  }, [dirty, onDirtyChange]);

  useEffect(() => {
    if (!channel) {return;}
    let alive = true;
    services.monitoring
      .listNotificationDeliveries({channel_id: channel.id, page: 1, page_size: 5})
      .then((d) => alive && setDeliveries(d?.deliveries ?? []))
      .catch(() => undefined);
    return () => {
      alive = false;
    };
  }, [channel]);

  const currentType: NotificationChannelType =
    state.kind === 'webhook' ? state.form.type : 'email';

  const setType = (type: NotificationChannelType) => {
    setState((s) => {
      if (type === 'email') {
        return {
          kind: 'email',
          form: {
            ...defaultEmailDraft(),
            name: s.kind === 'webhook' ? s.form.name : s.form.name,
            enabled: s.form.enabled,
            description: s.form.description,
          },
        };
      }
      const prevName = s.form.name;
      const prevDesc = s.form.description;
      const prevEnabled = s.form.enabled;
      const form =
        channel && isWebhookLikeChannelType(channel.type) && type === channel.type
          ? createWebhookChannelFormFromChannel(channel)
          : {...createDefaultWebhookChannelForm(type as WebhookChannelFormState['type']), name: prevName, description: prevDesc, enabled: prevEnabled};
      return {kind: 'webhook', form};
    });
  };

  const save = useCallback(async () => {
    setSaving(true);
    setError(null);
    try {
      const payload = buildPayload(state);
      if (channel) {
        await services.monitoring.updateNotificationChannel(channel.id, payload);
      } else {
        await services.monitoring.createNotificationChannel(payload);
      }
      setSnapshot(JSON.stringify(state));
      await onSaved();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setSaving(false);
    }
  }, [state, channel, onSaved]);

  return (
    // 同规则抽屉：负 margin 抵消 body 内边距，footer 由 flex 布局钉在抽屉底部。
    // Same as the rule drawer: negative margins cancel the body padding and the
    // footer is pinned to the drawer bottom by flex layout.
    <div className='-m-5 flex h-full flex-col sm:-m-6'>
      <div className='min-h-0 flex-1 space-y-4 overflow-y-auto overscroll-contain px-5 py-5 sm:px-6 sm:py-6'>
        {error ? <p className='rounded-lg border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive'>{error}</p> : null}

        <Select
          label={t('channelTypeLabel')}
          value={currentType}
          onValueChange={(v) => setType(v as NotificationChannelType)}
          options={CHANNEL_TYPES.map((type) => ({value: type, label: t(`channelType.${type}`)}))}
          disabled={channel != null}
        />

        {state.kind === 'webhook' ? (
          <WebhookFields
            form={state.form}
            onChange={(form) => setState({kind: 'webhook', form})}
          />
        ) : (
          <EmailFields
            form={state.form}
            onChange={(form) => setState({kind: 'email', form})}
          />
        )}

        {channel ? (
          <div className='space-y-2 border-t border-border/60 pt-3'>
            <span className='text-sm font-medium'>{t('recentDeliveries')}</span>
            {deliveries.length === 0 ? (
              <p className='rounded-lg border border-dashed border-border/60 px-3 py-2.5 text-xs text-muted-foreground'>
                {t('deliveriesEmpty')}
              </p>
            ) : (
              <ul className='space-y-1.5'>
                {deliveries.map((d) => (
                  <li key={d.id} className='flex items-center justify-between gap-2 rounded-lg border border-border/60 px-3 py-2 text-sm'>
                    <span className='truncate text-xs'>{d.alert_name || d.source_key}</span>
                    <span className='flex shrink-0 items-center gap-2'>
                      <Badge size='sm' tone={d.status === 'sent' ? 'success' : d.status === 'failed' ? 'danger' : 'neutral'} outline>
                        {t(`delivery.${d.status}`)}
                      </Badge>
                      <span className='whitespace-nowrap text-xs tabular-nums text-muted-foreground'>
                        {formatAgo(minutesAgo(d.sent_at || d.created_at), t)}
                      </span>
                    </span>
                  </li>
                ))}
              </ul>
            )}
          </div>
        ) : null}
      </div>

      {/* 底栏常驻抽屉底部；关闭被拦截时原位换确认行。
          Persistent bottom bar; an intercepted close swaps in the discard row. */}
      <div className='flex items-center gap-2 border-t border-border/60 bg-(--surface-raised) px-5 py-3 sm:px-6'>
        {discardArmed ? (
          <DiscardConfirmStrip
            onDiscard={() => onDiscard?.()}
            onKeepEditing={() => onKeepEditing?.()}
          />
        ) : (
          <>
            <Button
              type='button'
              size='sm'
              className='h-7 rounded-md text-xs'
              disabled={saving}
              onClick={() => void save()}
            >
              {t('save')}
            </Button>
            {saving ? <span className='text-xs text-muted-foreground'>{t('saving')}</span> : null}
            <Button
              type='button'
              variant='ghost'
              size='sm'
              className='ml-auto h-7 rounded-md text-xs'
              onClick={onCancel}
            >
              {t('deleteCancel')}
            </Button>
          </>
        )}
      </div>
    </div>
  );
}

function WebhookFields({
  form,
  onChange,
}: {
  form: WebhookChannelFormState;
  onChange: (f: WebhookChannelFormState) => void;
}) {
  const t = useTranslations('monitoring');
  return (
    <>
      <Input
        label={t('channelName')}
        value={form.name}
        onChange={(e) => onChange({...form, name: e.target.value})}
      />
      <Input
        label={t('channelTarget')}
        value={form.endpoint}
        onChange={(e) => onChange({...form, endpoint: e.target.value})}
        placeholder={endpointPlaceholderForType(form.type)}
      />
      {channelTypeSupportsSecret(form.type) ? (
        <Input
          label={t('channelSecret')}
          type='password'
          value={form.secret}
          onChange={(e) => onChange({...form, secret: e.target.value, clearSecret: false})}
          placeholder='••••••'
        />
      ) : null}
      <Input
        label={t('fieldDescription')}
        value={form.description}
        onChange={(e) => onChange({...form, description: e.target.value})}
      />
      <div className='flex items-center gap-2'>
        <Switch checked={form.enabled} onCheckedChange={(v) => onChange({...form, enabled: v})} aria-label={t('fieldEnabled')} />
        <span className='text-sm'>{t('fieldEnabled')}</span>
      </div>
    </>
  );
}

function EmailFields({form, onChange}: {form: EmailDraft; onChange: (f: EmailDraft) => void}) {
  const t = useTranslations('monitoring');
  const set = (patch: Partial<EmailDraft>) => onChange({...form, ...patch});
  return (
    <>
      <Input label={t('channelName')} value={form.name} onChange={(e) => set({name: e.target.value})} />
      <div className='grid grid-cols-2 gap-3'>
        <Select
          label={t('emailSecurity')}
          value={form.security}
          onValueChange={(v) => set({security: v as EmailDraft['security']})}
          options={[
            {value: 'ssl', label: 'SSL/TLS'},
            {value: 'starttls', label: 'STARTTLS'},
            {value: 'none', label: t('emailSecurityNone')},
          ]}
        />
        <Input label={t('emailPort')} value={form.port} onChange={(e) => set({port: e.target.value})} />
      </div>
      <Input label={t('emailHost')} value={form.host} onChange={(e) => set({host: e.target.value})} placeholder='smtp.example.com' />
      <Input label={t('emailUsername')} value={form.username} onChange={(e) => set({username: e.target.value})} />
      <Input label={t('emailPassword')} type='password' value={form.password} onChange={(e) => set({password: e.target.value})} />
      <Input label={t('emailFrom')} value={form.from} onChange={(e) => set({from: e.target.value})} placeholder='alerts@example.com' />
      <Input label={t('emailFromName')} value={form.from_name} onChange={(e) => set({from_name: e.target.value})} />
      <Input
        label={t('emailRecipients')}
        value={form.recipients}
        onChange={(e) => set({recipients: e.target.value})}
        placeholder='a@x.com, b@x.com'
      />
      <Input label={t('fieldDescription')} value={form.description} onChange={(e) => set({description: e.target.value})} />
      <div className='flex items-center gap-2'>
        <Switch checked={form.enabled} onCheckedChange={(v) => set({enabled: v})} aria-label={t('fieldEnabled')} />
        <span className='text-sm'>{t('fieldEnabled')}</span>
      </div>
    </>
  );
}
