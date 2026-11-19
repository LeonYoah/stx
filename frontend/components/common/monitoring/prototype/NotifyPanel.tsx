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
 * 通知面板原型：默认通知卡片、渠道表（ghost 测试按钮 + 编辑/删除收进 dropdown-menu）、
 * Arc drawer 渠道编辑（含最近投递）。删除走行内二次确认，不用弹窗打断。
 */
import {useState} from 'react';
import {useTranslations} from 'next-intl';
import {Check, CheckCircle2, MoreHorizontal, Pencil, Plus, Send, Trash2, X, XCircle} from 'lucide-react';
import {ActionButton} from '@/components/arc/action-button/action-button';
import {Badge} from '@/components/arc/badge/badge';
import {Button as ArcButton} from '@/components/arc/button/button';
import {Drawer, DrawerContent} from '@/components/arc/drawer/drawer';
import {DropdownMenu} from '@/components/arc/dropdown-menu/dropdown-menu';
import {Input} from '@/components/arc/input/input';
import {NumberField} from '@/components/arc/number-field/number-field';
import {Select} from '@/components/arc/select/select';
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
  const [deleteArmed, setDeleteArmed] = useState<string | null>(null);
  const [selectedChannels, setSelectedChannels] = useState<string[]>(['ops-email']);
  const [cooldown, setCooldown] = useState(30);
  const [recoveryNotify, setRecoveryNotify] = useState(true);
  const [saving, setSaving] = useState(false);

  const editChannel = channels.find((c) => c.key === editKey) ?? null;
  const drawerOpen = editKey !== null;

  const saveDefaults = () => {
    setSaving(true);
    setTimeout(() => setSaving(false), 900);
  };

  return (
    <div className='relative flex h-full flex-col gap-4 overflow-y-auto'>
      <section className='rounded-lg border border-border/70 bg-card p-4'>
        <div className='flex items-center justify-between'>
          <h3 className='text-sm font-medium'>{t('defaultNotify')}</h3>
          <ArcButton variant='primary' size='sm' loading={saving} onClick={saveDefaults}>
            {saving ? t('saving') : t('save')}
          </ArcButton>
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
          <div className='w-44'>
            <NumberField
              label={t('cooldown')}
              value={cooldown}
              onValueChange={setCooldown}
              min={0}
              max={1440}
              suffix={t('cooldownUnit')}
            />
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
            {deleteArmed === c.key ? (
              <span className='flex items-center gap-1'>
                <span className='whitespace-nowrap text-xs text-destructive'>{t('deleteConfirm')}</span>
                <Button
                  type='button'
                  variant='ghost'
                  size='icon'
                  className='size-7 text-destructive'
                  aria-label={t('deleteConfirm')}
                  onClick={() => {
                    setChannels((prev) => prev.filter((x) => x.key !== c.key));
                    setDeleteArmed(null);
                  }}
                >
                  <Check className='size-3.5' aria-hidden />
                </Button>
                <Button
                  type='button'
                  variant='ghost'
                  size='icon'
                  className='size-7'
                  aria-label={t('deleteCancel')}
                  onClick={() => setDeleteArmed(null)}
                >
                  <X className='size-3.5' aria-hidden />
                </Button>
              </span>
            ) : (
              <div className='flex items-center gap-1'>
                <ActionButton
                  label={t('testChannel')}
                  pendingLabel={t('testing')}
                  successLabel={t('testOk')}
                  onAction={() => new Promise<void>((r) => setTimeout(r, 700))}
                  className='h-7 rounded-md border-transparent! bg-transparent! px-2 text-xs text-muted-foreground hover:bg-muted/60! hover:text-foreground'
                />
                <DropdownMenu
                  label={t('moreActions')}
                  icon={<MoreHorizontal className='size-4' aria-hidden />}
                  items={[
                    {
                      label: t('editChannel'),
                      icon: <Pencil className='size-4' aria-hidden />,
                      onSelect: () => setEditKey(c.key),
                    },
                    {
                      label: t('deleteChannel'),
                      icon: <Trash2 className='size-4' aria-hidden />,
                      destructive: true,
                      separatorBefore: true,
                      onSelect: () => setDeleteArmed(c.key),
                    },
                  ]}
                />
              </div>
            )}
          </div>
        ))}
      </section>

      <Drawer open={drawerOpen} onOpenChange={(open) => !open && setEditKey(null)}>
        {drawerOpen ? (
          <DrawerContent
            side='right'
            title={editChannel ? editChannel.name : t('newChannel')}
            description={editChannel ? t(`channelType.${editChannel.type}`) : undefined}
            className='w-[min(92vw,420px)]'
          >
            <ChannelDrawerBody
              channel={editChannel}
              ago={ago}
              onSave={(next) => {
                setChannels((prev) =>
                  editChannel ? prev.map((c) => (c.key === editChannel.key ? next : c)) : [...prev, next],
                );
                setEditKey(null);
              }}
            />
          </DrawerContent>
        ) : null}
      </Drawer>
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

function ChannelDrawerBody({
  channel,
  ago,
  onSave,
}: {
  channel: ProtoChannel | null;
  ago: (m: number | null) => string;
  onSave: (c: ProtoChannel) => void;
}) {
  const t = useTranslations('monitoringProto');
  const [type, setType] = useState<ProtoChannel['type']>(channel?.type ?? 'webhook');
  const [name, setName] = useState(channel?.name ?? '');
  const [target, setTarget] = useState(channel?.target ?? '');
  const [enabled, setEnabled] = useState(channel?.enabled ?? true);

  return (
    <div className='flex min-h-0 flex-1 flex-col'>
      <div className='min-h-0 flex-1 space-y-4 overflow-y-auto px-1 py-1'>
        <Input
          label={t('channelName')}
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder='oncall-bot'
        />
        <Select
          label={t('channelTypeLabel')}
          value={type}
          onValueChange={(v) => setType(v as ProtoChannel['type'])}
          options={(['webhook', 'email', 'bot'] as const).map((v) => ({
            value: v,
            label: t(`channelType.${v}`),
          }))}
        />
        <Input
          label={t('channelTarget')}
          value={target}
          onChange={(e) => setTarget(e.target.value)}
          placeholder='https://…'
          description={t('channelTargetHint')}
        />
        <div className='flex items-center justify-between rounded-lg border border-border/60 px-3 py-2.5'>
          <span className='text-sm'>{t('fieldEnabled')}</span>
          <Switch checked={enabled} onCheckedChange={setEnabled} aria-label={t('fieldEnabled')} />
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
      <div className='border-t border-border/60 pt-3'>
        <ArcButton
          variant='primary'
          size='sm'
          onClick={() =>
            onSave({
              key: channel?.key ?? `new-${Date.now()}`,
              name: name || 'new-channel',
              type,
              target,
              enabled,
              health: channel?.health ?? {lastOkMinAgo: null, lastFailMinAgo: null, pending: 0},
            })
          }
        >
          {t('save')}
        </ArcButton>
      </div>
    </div>
  );
}
