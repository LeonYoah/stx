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
 * Notify panel: alert-level default notification card + channel list with
 * create/edit/test/delete in a drawer form.
 * 通知面板：默认通知卡片（渠道/接收人/冷却/恢复通知）+ 渠道列表（新建/编辑/测试/删除，
 * 编辑走 drawer 长表单，底部带该渠道最近投递记录）。
 */
import {useCallback, useEffect, useMemo, useState} from 'react';
import {useTranslations} from 'next-intl';
import {Pencil, Plus, Trash2} from 'lucide-react';
import {Badge} from '@/components/arc/badge/badge';
import {Drawer, DrawerContent} from '@/components/arc/drawer/drawer';
import {MultiSelect} from '@/components/arc/multi-select/multi-select';
import {NumberField} from '@/components/arc/number-field/number-field';
import {Skeleton} from '@/components/arc/skeleton/skeleton';
import {Switch} from '@/components/arc/switch/switch';
import {Button} from '@/components/ui/button';
import services from '@/lib/services';
import type {
  NotificationChannel,
  NotificationRecipientUser,
} from '@/lib/services/monitoring';
import {ChannelDrawerBody} from './ChannelDrawerBody';
import {useBeforeUnload} from '../dirty-close';

export function NotifyPanel() {
  const t = useTranslations('monitoring');
  const [defaults, setDefaults] = useState<{
    channel_ids: number[];
    receiver_user_ids: number[];
    cooldown_minutes: number;
    send_recovery: boolean;
  }>({channel_ids: [], receiver_user_ids: [], cooldown_minutes: 30, send_recovery: true});
  const [configured, setConfigured] = useState(false);
  const [channels, setChannels] = useState<NotificationChannel[]>([]);
  const [users, setUsers] = useState<NotificationRecipientUser[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [editKey, setEditKey] = useState<number | '__new__' | null>(null);
  // 测试结果与删除确认都内联在渠道行上，不走 toast。Row-level inline results/confirms.
  const [testing, setTesting] = useState<Set<number>>(new Set());
  const [testResult, setTestResult] = useState<Record<number, {ok: boolean; detail?: string}>>({});
  const [confirmDelete, setConfirmDelete] = useState<number | null>(null);
  const [deleting, setDeleting] = useState(false);
  const [defaultsStatus, setDefaultsStatus] = useState<'saving' | 'saved' | 'error' | null>(null);
  const [defaultsError, setDefaultsError] = useState<string | null>(null);
  const [channelDirty, setChannelDirty] = useState(false);
  const [discardArmed, setDiscardArmed] = useState(false);
  useBeforeUnload(channelDirty && editKey !== null);

  // dirty 时关闭（X / Esc / 遮罩 / 取消）先拦截，footer 出确认行；放弃才关。
  // A dirty close is intercepted first; only discard actually closes.
  const requestClose = useCallback(() => {
    if (channelDirty) {
      setDiscardArmed(true);
      return;
    }
    setEditKey(null);
    setDiscardArmed(false);
  }, [channelDirty]);

  const discardAndClose = useCallback(() => {
    setEditKey(null);
    setChannelDirty(false);
    setDiscardArmed(false);
  }, []);

  useEffect(() => {
    if (!channelDirty) {
      setDiscardArmed(false);
    }
  }, [channelDirty]);

  const load = useCallback(async () => {
    const [def, ch, us] = await Promise.all([
      services.monitoring.getAlertNotificationDefaults().catch(() => null),
      services.monitoring.listNotificationChannels().catch(() => ({channels: []})),
      services.monitoring.listNotifiableUsers().catch(() => ({users: []})),
    ]);
    if (def?.configured) {
      setDefaults({
        channel_ids: def.channel_ids ?? [],
        receiver_user_ids: def.receiver_user_ids ?? [],
        cooldown_minutes: def.cooldown_minutes ?? 30,
        send_recovery: def.send_recovery ?? true,
      });
      setConfigured(true);
    }
    setChannels(ch.channels ?? []);
    setUsers(us.users ?? []);
    setLoading(false);
  }, []);

  useEffect(() => {
    void load().catch((err) => {
      setError(err instanceof Error ? err.message : String(err));
      setLoading(false);
    });
  }, [load]);

  const saveDefaults = useCallback(async () => {
    setDefaultsStatus('saving');
    setDefaultsError(null);
    try {
      const saved = await services.monitoring.updateAlertNotificationDefaults(defaults);
      setConfigured(saved.configured);
      setDefaults({
        channel_ids: saved.channel_ids ?? [],
        receiver_user_ids: saved.receiver_user_ids ?? [],
        cooldown_minutes: saved.cooldown_minutes,
        send_recovery: saved.send_recovery,
      });
      setDefaultsStatus('saved');
      setTimeout(() => setDefaultsStatus((s) => (s === 'saved' ? null : s)), 2000);
    } catch (err) {
      setDefaultsStatus('error');
      setDefaultsError(err instanceof Error ? err.message : String(err));
    }
  }, [defaults]);

  const toggleChannel = useCallback(
    async (channel: NotificationChannel, enabled: boolean) => {
      setChannels((cs) => cs.map((c) => (c.id === channel.id ? {...c, enabled} : c)));
      try {
        const updated = await services.monitoring.updateNotificationChannel(channel.id, {
          name: channel.name,
          type: channel.type,
          enabled,
          endpoint: channel.endpoint,
          secret: channel.secret,
          config: channel.config,
          description: channel.description,
        });
        setChannels((cs) => cs.map((c) => (c.id === channel.id ? {...c, ...updated} : c)));
      } catch (err) {
        setChannels((cs) => cs.map((c) => (c.id === channel.id ? {...c, enabled: !enabled} : c)));
        setError(err instanceof Error ? err.message : String(err));
      }
    },
    [],
  );

  const testChannel = useCallback(async (channel: NotificationChannel) => {
    setTesting((s) => new Set(s).add(channel.id));
    setTestResult((prev) => {
      const next = {...prev};
      delete next[channel.id];
      return next;
    });
    try {
      const result = await services.monitoring.testNotificationChannel(channel.id, {});
      const ok = result.status === 'sent' || result.status === 'success';
      setTestResult((prev) => ({
        ...prev,
        [channel.id]: {ok, detail: ok ? undefined : result.last_error || result.status},
      }));
    } catch (err) {
      setTestResult((prev) => ({
        ...prev,
        [channel.id]: {ok: false, detail: err instanceof Error ? err.message : String(err)},
      }));
    } finally {
      setTesting((s) => {
        const next = new Set(s);
        next.delete(channel.id);
        return next;
      });
    }
  }, []);

  const deleteChannel = useCallback(
    async (id: number) => {
      setDeleting(true);
      try {
        await services.monitoring.deleteNotificationChannel(id);
        setChannels((cs) => cs.filter((c) => c.id !== id));
        setConfirmDelete(null);
      } catch (err) {
        setError(err instanceof Error ? err.message : String(err));
      } finally {
        setDeleting(false);
      }
    },
    [],
  );

  const channelOptions = useMemo(
    () => channels.map((c) => ({value: String(c.id), label: c.name})),
    [channels],
  );
  const userOptions = useMemo(
    () => users.map((u) => ({value: String(u.id), label: u.nickname || u.username})),
    [users],
  );

  const editingChannel = channels.find((c) => c.id === editKey) ?? null;
  const drawerOpen = editKey !== null;

  if (loading) {
    return (
      <div className='space-y-3 p-1' aria-busy>
        {Array.from({length: 4}).map((_, i) => (
          <Skeleton key={i} className='h-16 w-full' lines={1} />
        ))}
      </div>
    );
  }

  return (
    <div className='flex h-full flex-col gap-4 overflow-y-auto p-1'>
      {error ? (
        <div className='rounded-lg border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive'>
          {error}
        </div>
      ) : null}

      {/* 默认通知 / Default notification */}
      <section className='space-y-3 rounded-lg border border-border/60 bg-card p-4'>
        <div className='flex items-center justify-between gap-3'>
          <div className='flex items-center gap-2'>
            <h3 className='text-sm font-medium'>{t('defaultNotify')}</h3>
            {!configured ? (
              <Badge size='sm' tone='warning' outline>
                {t('unset')}
              </Badge>
            ) : null}
          </div>
          <div className='flex items-center gap-2'>
            {defaultsStatus === 'saving' ? (
              <span className='text-xs text-muted-foreground'>{t('saving')}</span>
            ) : null}
            {defaultsStatus === 'saved' ? (
              <span className='text-xs text-muted-foreground'>{t('saved')}</span>
            ) : null}
            {defaultsStatus === 'error' ? (
              <span className='max-w-56 truncate text-xs text-destructive' title={defaultsError ?? undefined}>
                {defaultsError}
              </span>
            ) : null}
            <Button
              type='button'
              size='sm'
              className='h-7 rounded-md text-xs'
              disabled={defaultsStatus === 'saving'}
              onClick={() => void saveDefaults()}
            >
              {t('save')}
            </Button>
          </div>
        </div>
        <div className='grid gap-3 sm:grid-cols-2'>
          <MultiSelect
            label={t('channels')}
            options={channelOptions}
            value={defaults.channel_ids.map(String)}
            onValueChange={(v) =>
              setDefaults((d) => ({...d, channel_ids: v.map(Number)}))
            }
            placeholder={t('notifyChannelsPlaceholder')}
          />
          <MultiSelect
            label={t('recipients')}
            options={userOptions}
            value={defaults.receiver_user_ids.map(String)}
            onValueChange={(v) =>
              setDefaults((d) => ({...d, receiver_user_ids: v.map(Number)}))
            }
            placeholder={t('notifyReceiversPlaceholder')}
          />
        </div>
        <div className='flex flex-wrap items-end gap-4'>
          <div className='w-40'>
            <NumberField
              label={t('cooldown')}
              value={defaults.cooldown_minutes}
              onValueChange={(v) => setDefaults((d) => ({...d, cooldown_minutes: v}))}
              min={0}
              suffix={t('cooldownUnit')}
            />
          </div>
          <div className='flex items-center gap-2 pb-1'>
            <Switch
              checked={defaults.send_recovery}
              onCheckedChange={(v) => setDefaults((d) => ({...d, send_recovery: v}))}
              aria-label={t('recoveryNotify')}
            />
            <span className='text-sm'>{t('recoveryNotify')}</span>
          </div>
        </div>
      </section>

      {/* 渠道 / Channels */}
      <section className='space-y-2'>
        <div className='flex items-center justify-between'>
          <h3 className='text-sm font-medium'>{t('channels')}</h3>
          <Button type='button' size='sm' className='h-7 rounded-md text-xs' onClick={() => setEditKey('__new__')}>
            <Plus className='mr-1 size-3.5' aria-hidden />
            {t('newChannel')}
          </Button>
        </div>
        {channels.length === 0 ? (
          <div className='flex min-h-[120px] items-center justify-center rounded-lg border border-dashed border-border/60 text-sm text-muted-foreground'>
            {t('alertsEmpty')}
          </div>
        ) : (
          <div className='divide-y divide-border/50 rounded-lg border border-border/60 bg-card'>
            {channels.map((c) => (
              <div
                key={c.id}
                role='button'
                tabIndex={0}
                onClick={(e) => {
                  if ((e.target as HTMLElement).closest('button, input, [role="switch"]')) {return;}
                  setEditKey(c.id);
                }}
                onKeyDown={(e) => {
                  if (e.target !== e.currentTarget) {return;}
                  if (e.key === 'Enter' || e.key === ' ') {
                    e.preventDefault();
                    setEditKey(c.id);
                  }
                }}
                className='flex cursor-pointer items-center gap-3 px-4 py-3 hover:bg-muted/30 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/40 focus-visible:ring-inset'
              >
                <div className='min-w-0 flex-1'>
                  <div className='flex items-center gap-2'>
                    <span className='truncate text-sm font-medium'>{c.name}</span>
                    <Badge size='sm' outline>
                      {t(`channelType.${c.type}`)}
                    </Badge>
                  </div>
                  <div className='mt-0.5 truncate font-mono text-xs text-muted-foreground'>
                    {c.endpoint || c.config?.email?.host || ''}
                  </div>
                </div>
                <Switch
                  checked={c.enabled}
                  onCheckedChange={(v) => void toggleChannel(c, v)}
                  aria-label={t('fieldEnabled')}
                />
                {testResult[c.id] ? (
                  <span
                    className={`max-w-40 shrink-0 truncate text-xs ${testResult[c.id].ok ? 'text-muted-foreground' : 'text-destructive'}`}
                    title={testResult[c.id].detail ?? t('testOk')}
                  >
                    {testResult[c.id].ok
                      ? t('testOk')
                      : `${t('testFailed')}${testResult[c.id].detail ? ` · ${testResult[c.id].detail}` : ''}`}
                  </span>
                ) : null}
                <Button
                  type='button'
                  variant='outline'
                  size='sm'
                  className='h-7 shrink-0 rounded-md text-xs'
                  disabled={testing.has(c.id)}
                  onClick={() => void testChannel(c)}
                >
                  {testing.has(c.id) ? t('testing') : t('testChannel')}
                </Button>
                <Button
                  type='button'
                  variant='ghost'
                  size='icon'
                  className='size-7 shrink-0'
                  aria-label={t('editChannel')}
                  onClick={() => setEditKey(c.id)}
                >
                  <Pencil className='size-3.5' aria-hidden />
                </Button>
                {confirmDelete === c.id ? (
                  <span className='flex shrink-0 items-center gap-1.5'>
                    <Button
                      type='button'
                      size='sm'
                      variant='destructive'
                      className='h-7 rounded-md text-xs'
                      disabled={deleting}
                      onClick={() => void deleteChannel(c.id)}
                    >
                      {deleting ? t('saving') : t('deleteChannel')}
                    </Button>
                    <Button
                      type='button'
                      size='sm'
                      variant='ghost'
                      className='h-7 rounded-md text-xs'
                      onClick={() => setConfirmDelete(null)}
                    >
                      {t('deleteCancel')}
                    </Button>
                  </span>
                ) : (
                  <Button
                    type='button'
                    variant='ghost'
                    size='icon'
                    className='size-7 shrink-0'
                    aria-label={t('deleteChannel')}
                    onClick={() => setConfirmDelete(c.id)}
                  >
                    <Trash2 className='size-3.5' aria-hidden />
                  </Button>
                )}
              </div>
            ))}
          </div>
        )}
      </section>

      <Drawer open={drawerOpen} onOpenChange={(open) => !open && requestClose()}>
        {drawerOpen ? (
          <DrawerContent
            side='right'
            title={editKey === '__new__' ? t('newChannel') : t('editChannel')}
            className='w-[min(92vw,460px)] rounded-none! [&>div[class*=body]]:flex-1'
          >
            <ChannelDrawerBody
              key={String(editKey)}
              channel={editKey === '__new__' ? null : editingChannel}
              onSaved={async () => {
                setEditKey(null);
                setChannelDirty(false);
                setLoading(true);
                await load().catch(() => undefined);
              }}
              onCancel={requestClose}
              onDirtyChange={setChannelDirty}
              discardArmed={discardArmed}
              onDiscard={discardAndClose}
              onKeepEditing={() => setDiscardArmed(false)}
            />
          </DrawerContent>
        ) : null}
      </Drawer>
    </div>
  );
}
