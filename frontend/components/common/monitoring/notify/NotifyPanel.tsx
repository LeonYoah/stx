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
 * Notify panel: default notification as a settings group (four label/control
 * rows, save in the group head) plus the channel list following the shared
 * list-page layout (search + type filter + table + detail rail / drawer).
 * 通知面板：默认通知 = 设置分组（渠道/接收人/冷却/恢复通知 四行，左标签右控件，
 * 保存在分组标题行右侧）；渠道列表按列表页版式（搜索 + 类型筛选 + 表格 +
 * 常驻详情栏 / 抽屉），行内测试与启用开关，删除收敛进详情抽屉。
 */
import {useCallback, useEffect, useMemo, useRef, useState} from 'react';
import {useTranslations} from 'next-intl';
import {Plus} from 'lucide-react';
import {NeutralChip, SoftChip} from '../status-chips';
import {Button} from '@/components/arc/button/button';
import {useDrawerItem} from '@/components/arc/drawer/drawer';
import {EmptyState} from '@/components/arc/empty-state/empty-state';
import {MultiSelect} from '@/components/arc/multi-select/multi-select';
import {NumberField} from '@/components/arc/number-field/number-field';
import {SearchField} from '@/components/arc/search-field/search-field';
import {Select} from '@/components/arc/select/select';
import {Skeleton} from '@/components/arc/skeleton/skeleton';
import {Switch} from '@/components/arc/switch/switch';
import {TableCard} from '@/components/common/layout/TableCard';
import services from '@/lib/services';
import type {
  NotificationChannel,
  NotificationRecipientUser,
} from '@/lib/services/monitoring';
import {cn} from '@/lib/utils';
import {ChannelDrawerBody} from './ChannelDrawerBody';
import {useBeforeUnload} from '../dirty-close';
import {DetailHost, useDetailNarrow} from '../detail-host';
import styles from '../monitoring.module.css';

const CHANNEL_TYPES = ['webhook', 'email', 'dingtalk', 'wecom', 'feishu'];
/** Radix Select 不允许空字符串选项值，「全部」用哨兵值再映射回 ''。 / Sentinel for "all" (Radix rejects ''). */
const ALL_VALUE = '__all__';

export function NotifyPanel() {
  const t = useTranslations('monitoring');
  const narrow = useDetailNarrow();
  const [defaults, setDefaults] = useState<{
    channel_ids: number[];
    receiver_user_ids: number[];
    cooldown_minutes: number;
    send_recovery: boolean;
  }>({
    channel_ids: [],
    receiver_user_ids: [],
    cooldown_minutes: 30,
    send_recovery: true,
  });
  const [configured, setConfigured] = useState(false);
  const [channels, setChannels] = useState<NotificationChannel[]>([]);
  const [users, setUsers] = useState<NotificationRecipientUser[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [editKey, setEditKey] = useState<number | '__new__' | null>(null);
  // 测试结果内联在渠道行上，不走 toast。Inline row-level test results.
  const [testing, setTesting] = useState<Set<number>>(new Set());
  const [testResult, setTestResult] = useState<
    Record<number, {ok: boolean; detail?: string}>
  >({});
  const [defaultsStatus, setDefaultsStatus] = useState<
    'saving' | 'saved' | 'error' | null
  >(null);
  const [defaultsError, setDefaultsError] = useState<string | null>(null);
  const [channelDirty, setChannelDirty] = useState(false);
  const [discardArmed, setDiscardArmed] = useState(false);
  const [query, setQuery] = useState('');
  const [typeFilter, setTypeFilter] = useState('');
  const pendingRef = useRef<number | '__new__' | 'close' | null>(null);
  const searchRef = useRef<HTMLInputElement>(null);
  useBeforeUnload(channelDirty && editKey !== null);

  const editingChannel = channels.find((c) => c.id === editKey) ?? null;
  const drawerOpen = editKey !== null;
  const drawerChannel = useDrawerItem(
    editingChannel ?? (editKey === '__new__' ? 'new' : null),
  );

  // dirty 时关闭（X / Esc / 遮罩 / 取消 / 切换行）先拦截，footer 出确认行；放弃才落地。
  // A dirty close or row switch is intercepted first; only discard lands it.
  const requestEdit = useCallback(
    (key: number | '__new__' | null) => {
      if (key === editKey) {
        return;
      }
      if (channelDirty && editKey !== null) {
        pendingRef.current = key ?? 'close';
        setDiscardArmed(true);
        return;
      }
      setEditKey(key);
      setDiscardArmed(false);
    },
    [channelDirty, editKey],
  );

  const discardAndCommit = useCallback(() => {
    const target = pendingRef.current;
    pendingRef.current = null;
    setChannelDirty(false);
    setDiscardArmed(false);
    setEditKey(target && target !== 'close' ? target : null);
  }, []);

  useEffect(() => {
    if (!channelDirty) {
      setDiscardArmed(false);
    }
  }, [channelDirty]);

  const load = useCallback(async () => {
    const [def, ch, us] = await Promise.all([
      services.monitoring.getAlertNotificationDefaults().catch(() => null),
      services.monitoring
        .listNotificationChannels()
        .catch(() => ({channels: []})),
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

  // 「/」聚焦搜索。「/」 focuses the search field.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const el = e.target as HTMLElement | null;
      if (
        e.key === '/' &&
        !el?.closest('input, textarea, select, [contenteditable="true"]')
      ) {
        e.preventDefault();
        searchRef.current?.focus();
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, []);

  const saveDefaults = useCallback(async () => {
    setDefaultsStatus('saving');
    setDefaultsError(null);
    try {
      const saved =
        await services.monitoring.updateAlertNotificationDefaults(defaults);
      setConfigured(saved.configured);
      setDefaults({
        channel_ids: saved.channel_ids ?? [],
        receiver_user_ids: saved.receiver_user_ids ?? [],
        cooldown_minutes: saved.cooldown_minutes,
        send_recovery: saved.send_recovery,
      });
      setDefaultsStatus('saved');
      setTimeout(
        () => setDefaultsStatus((s) => (s === 'saved' ? null : s)),
        2000,
      );
    } catch (err) {
      setDefaultsStatus('error');
      setDefaultsError(err instanceof Error ? err.message : String(err));
    }
  }, [defaults]);

  const toggleChannel = useCallback(
    async (channel: NotificationChannel, enabled: boolean) => {
      setChannels((cs) =>
        cs.map((c) => (c.id === channel.id ? {...c, enabled} : c)),
      );
      try {
        const updated = await services.monitoring.updateNotificationChannel(
          channel.id,
          {
            name: channel.name,
            type: channel.type,
            enabled,
            endpoint: channel.endpoint,
            secret: channel.secret,
            config: channel.config,
            description: channel.description,
          },
        );
        setChannels((cs) =>
          cs.map((c) => (c.id === channel.id ? {...c, ...updated} : c)),
        );
      } catch (err) {
        setChannels((cs) =>
          cs.map((c) => (c.id === channel.id ? {...c, enabled: !enabled} : c)),
        );
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
      const result = await services.monitoring.testNotificationChannel(
        channel.id,
        {},
      );
      const ok = result.status === 'sent' || result.status === 'success';
      setTestResult((prev) => ({
        ...prev,
        [channel.id]: {
          ok,
          detail: ok ? undefined : result.last_error || result.status,
        },
      }));
    } catch (err) {
      setTestResult((prev) => ({
        ...prev,
        [channel.id]: {
          ok: false,
          detail: err instanceof Error ? err.message : String(err),
        },
      }));
    } finally {
      setTesting((s) => {
        const next = new Set(s);
        next.delete(channel.id);
        return next;
      });
    }
  }, []);

  const deleteChannel = useCallback(async (id: number) => {
    try {
      await services.monitoring.deleteNotificationChannel(id);
      setChannels((cs) => cs.filter((c) => c.id !== id));
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
      throw err;
    }
  }, []);

  const channelOptions = useMemo(
    () => channels.map((c) => ({value: String(c.id), label: c.name})),
    [channels],
  );
  const userOptions = useMemo(
    () =>
      users.map((u) => ({
        value: String(u.id),
        label: u.nickname || u.username,
      })),
    [users],
  );
  const typeOptions = useMemo(
    () => [
      {value: ALL_VALUE, label: t('scopeAll')},
      ...CHANNEL_TYPES.map((type) => ({
        value: type,
        label: t(`channelType.${type}`),
      })),
    ],
    [t],
  );

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    return channels.filter((c) => {
      if (typeFilter && c.type !== typeFilter) {
        return false;
      }
      if (!q) {
        return true;
      }
      return (
        c.name.toLowerCase().includes(q) ||
        (c.endpoint ?? '').toLowerCase().includes(q)
      );
    });
  }, [channels, typeFilter, query]);

  const activeIndex = editingChannel
    ? filtered.findIndex((c) => c.id === editingChannel.id)
    : -1;
  const detailNav =
    editingChannel && activeIndex >= 0
      ? {
          index: activeIndex,
          total: filtered.length,
          onPrev: () =>
            activeIndex > 0 && requestEdit(filtered[activeIndex - 1].id),
          onNext: () =>
            activeIndex < filtered.length - 1 &&
            requestEdit(filtered[activeIndex + 1].id),
        }
      : undefined;

  const onRowKeyDown = (e: React.KeyboardEvent<HTMLElement>, id: number) => {
    if (e.target !== e.currentTarget) {
      return;
    }
    const rows = Array.from(
      e.currentTarget.parentElement?.querySelectorAll<HTMLElement>(
        '[data-mrow]',
      ) ?? [],
    );
    const i = rows.indexOf(e.currentTarget);
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault();
      rows[i + (e.key === 'ArrowDown' ? 1 : -1)]?.focus();
    } else if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault();
      requestEdit(id);
    }
  };

  const retryLoad = useCallback(() => {
    setError(null);
    setLoading(true);
    void load().catch((err) => {
      setError(err instanceof Error ? err.message : String(err));
      setLoading(false);
    });
  }, [load]);

  return (
    <div className={styles.page} style={{overflowY: 'auto'}}>
      {/* 默认通知：设置分组，保存在分组标题行右侧；首屏加载在分组内出骨架行。
          Default notification as a settings group; save sits in the group head,
          and the first load shows skeleton rows inside the group. */}
      <section className={styles.group}>
        <div className={styles.groupHead}>
          <span className={styles.groupTitle}>
            {t('defaultNotify')}
            {!configured && !loading ? (
              <SoftChip tone='amber'>{t('unset')}</SoftChip>
            ) : null}
          </span>
          <span className='flex items-center gap-2'>
            {defaultsStatus === 'saved' ? (
              <span className={cn(styles.xsText, styles.muted)}>
                {t('saved')}
              </span>
            ) : null}
            {defaultsStatus === 'error' ? (
              <span
                className={cn(
                  styles.xsText,
                  styles.dangerText,
                  'max-w-56 truncate',
                )}
                title={defaultsError ?? undefined}
              >
                {defaultsError}
              </span>
            ) : null}
            <Button
              type='button'
              size='sm'
              loading={defaultsStatus === 'saving'}
              disabled={loading}
              onClick={() => void saveDefaults()}
            >
              {t('save')}
            </Button>
          </span>
        </div>
        {loading ? (
          <>
            {[0, 1, 2].map((i) => (
              <div key={i} className={styles.groupRow}>
                <Skeleton className='h-5 w-1/2' lines={1} />
              </div>
            ))}
          </>
        ) : (
          <>
            <div className={cn(styles.groupRow, styles.groupRowStack)}>
              <div className={cn(styles.rowControl, 'w-72 max-w-full')}>
                <MultiSelect
                  label={t('channels')}
                  options={channelOptions}
                  value={defaults.channel_ids.map(String)}
                  onValueChange={(v) =>
                    setDefaults((d) => ({...d, channel_ids: v.map(Number)}))
                  }
                  placeholder={t('notifyChannelsPlaceholder')}
                  emptyMessage={t('selectEmpty')}
                />
              </div>
            </div>
            <div className={cn(styles.groupRow, styles.groupRowStack)}>
              <div className={cn(styles.rowControl, 'w-72 max-w-full')}>
                <MultiSelect
                  label={t('recipients')}
                  options={userOptions}
                  value={defaults.receiver_user_ids.map(String)}
                  onValueChange={(v) =>
                    setDefaults((d) => ({
                      ...d,
                      receiver_user_ids: v.map(Number),
                    }))
                  }
                  placeholder={t('notifyReceiversPlaceholder')}
                  emptyMessage={t('selectEmpty')}
                />
              </div>
            </div>
            <div className={cn(styles.groupRow, styles.groupRowStack)}>
              <div className={cn(styles.rowControl, styles.fieldWsm)}>
                <NumberField
                  label={t('cooldown')}
                  value={defaults.cooldown_minutes}
                  onValueChange={(v) =>
                    setDefaults((d) => ({...d, cooldown_minutes: v}))
                  }
                  min={0}
                  suffix={t('cooldownUnit')}
                />
              </div>
            </div>
            <div className={styles.groupRow}>
              <div className={styles.rowLabel}>
                <span className={styles.rowLabelText}>
                  {t('recoveryNotify')}
                </span>
              </div>
              <Switch
                checked={defaults.send_recovery}
                onCheckedChange={(v) =>
                  setDefaults((d) => ({...d, send_recovery: v}))
                }
                aria-label={t('recoveryNotify')}
              />
            </div>
          </>
        )}
      </section>

      {/* 渠道 / Channels */}
      <section className={styles.sectionStack} style={{flex: 1, minHeight: 0}}>
        <div className={styles.toolbar}>
          <span className={styles.sectionTitle}>{t('channels')}</span>
          <div className={styles.toolbarSearch}>
            <SearchField
              ref={searchRef}
              label={t('channelsSearchLabel')}
              value={query}
              onValueChange={setQuery}
              placeholder={t('channelSearchPlaceholder')}
              clearLabel={t('searchClear')}
              onKeyDown={(e) => {
                if (e.key === 'Escape' && query) {
                  setQuery('');
                }
              }}
            />
          </div>
          <div className={cn(styles.filterCtrl, styles.filterW)}>
            <Select
              label={t('channelTypeLabel')}
              value={typeFilter || ALL_VALUE}
              onValueChange={(v) => setTypeFilter(v === ALL_VALUE ? '' : v)}
              options={typeOptions}
            />
          </div>
          <div className={styles.toolbarRight}>
            <Button
              type='button'
              size='sm'
              variant='secondary'
              onClick={() => requestEdit('__new__')}
            >
              <Plus className='size-3.5' aria-hidden />
              {t('newChannel')}
            </Button>
          </div>
        </div>

        <div className={styles.split}>
          <div className={styles.listCol} data-mcols='channels'>
            {/* 唯一主卡片：骨架/错误/空态由 TableCard 统一承载（§4.4）。
                Single main card; skeleton/error/empty states are owned by TableCard. */}
            <TableCard
              loading={loading}
              hasData={filtered.length > 0}
              skeletonColumns={6}
              skeletonRows={5}
              error={error}
              onRetry={retryLoad}
              empty={
                <EmptyState
                  title={
                    channels.length === 0
                      ? t('alertsEmpty')
                      : t('emptyFiltered')
                  }
                  description={
                    channels.length === 0
                      ? t('emptyAlertsDesc')
                      : t('emptyFilteredDesc')
                  }
                  action={
                    typeFilter || query ? (
                      <Button
                        type='button'
                        size='sm'
                        variant='secondary'
                        onClick={() => {
                          setTypeFilter('');
                          setQuery('');
                        }}
                      >
                        {t('clearFilters')}
                      </Button>
                    ) : undefined
                  }
                />
              }
            >
              <div className={styles.rowHead} aria-hidden>
                <span>{t('channelName')}</span>
                <span>{t('channelTypeLabel')}</span>
                <span>{t('channelTarget')}</span>
                <span>{t('testChannel')}</span>
                <span className={styles.right}>{t('fieldEnabled')}</span>
              </div>
              <div className={styles.tableScroll}>
                {filtered.map((c) => (
                  <div
                    key={c.id}
                    data-mrow={c.id}
                    data-testid={`channel-row-${c.id}`}
                    role='button'
                    tabIndex={0}
                    onClick={(e) => {
                      if (
                        (e.target as HTMLElement).closest(
                          'button, input, [role="switch"], [role="checkbox"]',
                        )
                      ) {
                        return;
                      }
                      requestEdit(c.id);
                    }}
                    onKeyDown={(e) => onRowKeyDown(e, c.id)}
                    className={cn(
                      styles.row,
                      editKey === c.id && styles.rowActive,
                      !c.enabled && styles.rowMuted,
                    )}
                  >
                    <div className={styles.rowName}>
                      <span className={styles.name} title={c.name}>
                        {c.name}
                      </span>
                    </div>
                    <NeutralChip>
                      {t(`channelType.${c.type}`)}
                    </NeutralChip>
                    <span
                      className={cn(styles.sub, styles.mono)}
                      title={c.endpoint || c.config?.email?.host || ''}
                    >
                      {c.endpoint || c.config?.email?.host || '—'}
                    </span>
                    <span
                      className={styles.rowActions}
                      style={{justifyContent: 'flex-start'}}
                    >
                      <Button
                        type='button'
                        variant='secondary'
                        size='sm'
                        loading={testing.has(c.id)}
                        onClick={() => void testChannel(c)}
                      >
                        {t('testChannel')}
                      </Button>
                      {testResult[c.id] ? (
                        <span
                          className={cn(
                            styles.xsText,
                            'max-w-28 truncate',
                            testResult[c.id].ok
                              ? styles.muted
                              : styles.dangerText,
                          )}
                          title={testResult[c.id].detail ?? t('testOk')}
                        >
                          {testResult[c.id].ok ? t('testOk') : t('testFailed')}
                        </span>
                      ) : null}
                    </span>
                    <div className={styles.rowActions}>
                      <Switch
                        checked={c.enabled}
                        onCheckedChange={(v) => void toggleChannel(c, v)}
                        aria-label={t('fieldEnabled')}
                      />
                    </div>
                  </div>
                ))}
              </div>
            </TableCard>
          </div>

          <DetailHost
            open={drawerOpen}
            hasContent={drawerChannel != null}
            onOpenChange={(o) => !o && requestEdit(null)}
            narrow={narrow}
            title={editKey === '__new__' ? t('newChannel') : t('editChannel')}
            meta={editingChannel?.name}
            nav={detailNav}
            drawerWidth='w-[min(92vw,480px)]'
          >
            {drawerChannel ? (
              <ChannelDrawerBody
                key={String(editKey)}
                channel={editKey === '__new__' ? null : editingChannel}
                onDelete={
                  editingChannel
                    ? async () => {
                        await deleteChannel(editingChannel.id);
                        setEditKey(null);
                        setChannelDirty(false);
                      }
                    : undefined
                }
                onSaved={async () => {
                  setEditKey(null);
                  setChannelDirty(false);
                  await load().catch(() => undefined);
                }}
                onCancel={() => requestEdit(null)}
                onDirtyChange={setChannelDirty}
                discardArmed={discardArmed}
                onDiscard={discardAndCommit}
                onKeepEditing={() => {
                  setDiscardArmed(false);
                  pendingRef.current = null;
                }}
              />
            ) : null}
          </DetailHost>
        </div>
      </section>
    </div>
  );
}
