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
 * Notification panel prototype: default notify card + channel table + edit drawer.
 * 通知面板原型：默认通知卡片、渠道表（测试 / 编辑 / 删除）、渠道编辑抽屉（含最近投递）。
 */
import {useState} from 'react';
import {useTranslations} from 'next-intl';
import {CheckCircle2, Pencil, Plus, Send, Trash2, X, XCircle} from 'lucide-react';
import {ActionButton} from '@/components/arc/action-button/action-button';
import {Badge} from '@/components/arc/badge/badge';
import {ConfirmMorph} from '@/components/arc/confirm-morph/confirm-morph';
import {Switch} from '@/components/arc/switch/switch';
import {Button} from '@/components/ui/button';
import {PROTO_CHANNELS, type ProtoChannel} from './alerts-mock';
import {cn} from '@/lib/utils';

function useTimeAgo() {
  const t = useTranslations('monitoringProto');
  return (min: number | null) =>
    min == null
      ? '—'
      : min < 60
        ? t('agoMinutes', {value: min})
        : min < 1440
          ? t('agoHours', {value: Math.round(min / 60)})
          : t('agoDays', {value: Math.round(min / 1440)});
}

export function NotifyPanel() {
  const t = useTranslations('monitoringProto');
  const ago = useTimeAgo();
  const [channels, setChannels] = useState(PROTO_CHANNELS);
  const [editKey, setEditKey] = useState<string | null>(null);
  const [selectedChannels, setSelectedChannels] = useState<string[]>(['ops-email']);
  const [cooldown, setCooldown] = useState(30);
  const [recoveryNotify, setRecoveryNotify] = useState(true);

  const editChannel = channels.find((c) => c.key === editKey) ?? null;

  return (
    <div className='relative flex h-full flex-col gap-4 overflow-y-auto'>
      <section className='rounded-lg border border-border/70 bg-card p-4'>
        <div className='flex items-center justify-between'>
          <h3 className='text-sm font-medium'>{t('defaultNotify')}</h3>
          <ActionButton
            label={t('save')}
            pendingLabel={t('saving')}
            successLabel={t('saved')}
            onAction={() => new Promise<void>((r) => setTimeout(r, 900))}
            className='rounded-md bg-primary px-3 py-1.5 text-xs text-primary-foreground'
          />
        </div>
        <div className='mt-3 grid gap-4 md:grid-cols-2'>
          <div className='space-y-2'>
            <span className='text-sm text-muted-foreground'>{t('channels')}</span>
            <div className='flex flex-wrap gap-1.5'>
              {channels.map((c) => (
                <button
                  key={c.key}
                  type='button'
                  onClick={() =>
                    setSelectedChannels((prev) =>
                      prev.includes(c.key) ? prev.filter((k) => k !== c.key) : [...prev, c.key],
                    )
                  }
                  className={cn(
                    'rounded-md border px-2.5 py-1 font-mono text-xs transition-colors',
                    selectedChannels.includes(c.key)
                      ? 'border-primary/50 bg-primary/10 text-primary'
                      : 'border-border/60 text-muted-foreground hover:border-border',
                  )}
                >
                  {c.name}
                </button>
              ))}
            </div>
          </div>
          <div className='space-y-2'>
            <span className='text-sm text-muted-foreground'>{t('recipients')}</span>
            <div className='rounded-lg border border-dashed border-border/60 px-3 py-2 text-xs text-muted-foreground'>
              {t('recipientsStub')}
            </div>
          </div>
          <div className='flex items-center gap-3'>
            <span className='text-sm text-muted-foreground'>{t('cooldown')}</span>
            <input
              type='number'
              value={cooldown}
              min={0}
              max={1440}
              onChange={(e) => setCooldown(Number(e.target.value))}
              className='h-8 w-24 rounded-md border border-border/60 bg-background px-2 text-sm'
              aria-label={t('cooldown')}
            />
            <span className='text-xs text-muted-foreground'>{t('cooldownUnit')}</span>
          </div>
          <div className='flex items-center justify-between gap-3'>
            <span className='text-sm text-muted-foreground'>{t('recoveryNotify')}</span>
            <Switch checked={recoveryNotify} onCheckedChange={setRecoveryNotify} aria-label={t('recoveryNotify')} />
          </div>
        </div>
      </section>

      <section className='min-h-0 flex-1 rounded-lg border border-border/60 bg-card/40'>
        <div className='flex items-center justify-between border-b border-border/60 px-4 py-2.5'>
          <span className='text-sm font-medium'>{t('channels')}</span>
          <Button type='button' size='sm' className='h-7 rounded-md text-xs' onClick={() => setEditKey('__new__')}>
            <Plus className='mr-1 size-3.5' aria-hidden />
            {t('newChannel')}
          </Button>
        </div>
        {channels.map((c) => (
          <div
            key={c.key}
            className='grid grid-cols-[minmax(0,1.2fr)_auto_minmax(0,1.4fr)_auto_auto] items-center gap-3 border-b border-border/50 px-4 py-3 last:border-b-0 hover:bg-muted/30'
            data-testid={`proto-channel-${c.key}`}
          >
            <div className='min-w-0'>
              <div className='truncate text-sm font-medium'>{c.name}</div>
              <div className='truncate font-mono text-xs text-muted-foreground'>{c.target}</div>
            </div>
            <Badge size='sm' outline>
              {t(`channelType.${c.type}`)}
            </Badge>
            <ChannelHealth channel={c} ago={ago} />
            <Switch
              checked={c.enabled}
              onCheckedChange={(on) =>
                setChannels((prev) => prev.map((x) => (x.key === c.key ? {...x, enabled: on} : x)))
              }
              aria-label={t('fieldEnabled')}
            />
            <div className='flex items-center gap-1'>
              <ActionButton
                label={t('testChannel')}
                pendingLabel={t('testing')}
                successLabel={t('testOk')}
                onAction={() => new Promise<void>((r) => setTimeout(r, 700))}
                className='h-7 rounded-md border border-border/60 px-2 text-xs'
              />
              <Button type='button' variant='ghost' size='icon' className='size-7' onClick={() => setEditKey(c.key)} aria-label={t('editChannel')}>
                <Pencil className='size-3.5' aria-hidden />
              </Button>
              <ConfirmMorph
                label={<Trash2 className='size-3.5' aria-hidden />}
                prompt={t('deleteChannelPrompt', {name: c.name})}
                confirmLabel={t('deleteConfirm')}
                cancelLabel={t('deleteCancel')}
                onConfirm={() =>
                  new Promise<void>((r) => {
                    setTimeout(() => {
                      setChannels((prev) => prev.filter((x) => x.key !== c.key));
                      r();
                    }, 500);
                  })
                }
              />
            </div>
          </div>
        ))}
      </section>

      {editKey !== null ? (
        <>
          <button type='button' aria-label={t('closeDetail')} className='fixed inset-0 z-40 cursor-default bg-foreground/20' onClick={() => setEditKey(null)} />
          <ChannelDrawer
            channel={editChannel}
            ago={ago}
            onClose={() => setEditKey(null)}
            onSave={(next) => {
              setChannels((prev) =>
                editChannel ? prev.map((c) => (c.key === editChannel.key ? next : c)) : [...prev, next],
              );
              setEditKey(null);
            }}
          />
        </>
      ) : null}
    </div>
  );
}

function ChannelHealth({channel, ago}: {channel: ProtoChannel; ago: (m: number | null) => string}) {
  const t = useTranslations('monitoringProto');
  return (
    <div className='flex min-w-0 items-center gap-3 text-xs text-muted-foreground'>
      <span className='flex items-center gap-1'>
        <CheckCircle2 className='size-3.5 text-(--success)' aria-hidden />
        {ago(channel.health.lastOkMinAgo)}
      </span>
      <span className='flex items-center gap-1'>
        <XCircle className={cn('size-3.5', channel.health.lastFailMinAgo != null ? 'text-destructive' : 'text-muted-foreground/30')} aria-hidden />
        {ago(channel.health.lastFailMinAgo)}
      </span>
      {channel.health.pending > 0 ? (
        <Badge size='sm' tone='warning' outline>
          {t('pendingCount', {count: channel.health.pending})}
        </Badge>
      ) : null}
    </div>
  );
}

function ChannelDrawer({
  channel,
  ago,
  onClose,
  onSave,
}: {
  channel: ProtoChannel | null;
  ago: (m: number | null) => string;
  onClose: () => void;
  onSave: (c: ProtoChannel) => void;
}) {
  const t = useTranslations('monitoringProto');
  const [type, setType] = useState<ProtoChannel['type']>(channel?.type ?? 'webhook');
  const isNew = channel == null;

  return (
    <aside
      data-testid='proto-channel-drawer'
      className='fixed inset-y-4 right-4 z-50 flex w-[min(92vw,420px)] flex-col overflow-hidden rounded-lg border border-border/70 bg-card shadow-xl'
    >
      <div className='flex items-center justify-between border-b border-border/60 px-4 py-3'>
        <h3 className='text-base font-medium'>{isNew ? t('newChannel') : channel.name}</h3>
        <Button type='button' variant='ghost' size='icon' className='size-7' onClick={onClose} aria-label={t('closeDetail')}>
          <X className='size-4' aria-hidden />
        </Button>
      </div>
      <div className='min-h-0 flex-1 space-y-4 overflow-y-auto px-4 py-4'>
        <div className='space-y-1.5'>
          <span className='text-sm font-medium'>{t('channelName')}</span>
          <input
            defaultValue={channel?.name}
            placeholder='oncall-bot'
            className='h-8 w-full rounded-md border border-border/60 bg-background px-2 font-mono text-sm'
          />
        </div>
        <div className='space-y-1.5'>
          <span className='text-sm font-medium'>{t('channelTypeLabel')}</span>
          <div className='flex gap-1.5'>
            {(['webhook', 'email', 'bot'] as const).map((v) => (
              <button
                key={v}
                type='button'
                onClick={() => setType(v)}
                className={cn(
                  'rounded-md border px-3 py-1.5 text-xs',
                  type === v ? 'border-primary/50 bg-primary/10 text-primary' : 'border-border/60 text-muted-foreground',
                )}
              >
                {t(`channelType.${v}`)}
              </button>
            ))}
          </div>
        </div>
        <div className='space-y-1.5'>
          <span className='text-sm font-medium'>{t('channelTarget')}</span>
          <input
            defaultValue={channel?.target}
            placeholder='https://…'
            className='h-8 w-full rounded-md border border-border/60 bg-background px-2 font-mono text-xs'
          />
          <p className='text-xs text-muted-foreground'>{t('channelTargetHint')}</p>
        </div>
        <div className='flex items-center justify-between rounded-lg border border-border/60 px-3 py-2.5'>
          <span className='text-sm'>{t('fieldEnabled')}</span>
          <Switch defaultChecked={channel?.enabled ?? true} aria-label={t('fieldEnabled')} />
        </div>

        {channel ? (
          <div className='space-y-2'>
            <span className='text-sm font-medium'>{t('recentDeliveries')}</span>
            <ul className='space-y-1.5'>
              {[
                {status: 'success' as const, min: channel.health.lastOkMinAgo},
                {status: 'failed' as const, min: channel.health.lastFailMinAgo},
                ...Array.from({length: channel.health.pending}, () => ({status: 'pending' as const, min: 0})),
              ]
                .filter((d) => d.min != null)
                .map((d, i) => (
                  <li key={i} className='flex items-center justify-between rounded-lg border border-border/60 px-3 py-2 text-sm'>
                    <span className='flex items-center gap-2 text-xs text-muted-foreground'>
                      <Send className='size-3' aria-hidden />
                      {t(`delivery.${d.status}`)}
                    </span>
                    <span className='text-xs tabular-nums text-muted-foreground'>{ago(d.min)}</span>
                  </li>
                ))}
            </ul>
          </div>
        ) : null}
      </div>
      <div className='flex items-center gap-2 border-t border-border/60 px-4 py-3'>
        <Button
          type='button'
          size='sm'
          className='h-8 rounded-md text-xs'
          onClick={() =>
            onSave({
              key: channel?.key ?? `new-${Date.now()}`,
              name: channel?.name ?? 'new-channel',
              type,
              target: channel?.target ?? '',
              enabled: channel?.enabled ?? true,
              health: channel?.health ?? {lastOkMinAgo: null, lastFailMinAgo: null, pending: 0},
            })
          }
        >
          {t('save')}
        </Button>
      </div>
    </aside>
  );
}
