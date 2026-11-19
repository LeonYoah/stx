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
 * Job-level rules: sync_job alert policies list + detail rail hosting the shared
 * TaskAlertForm (same save API as task settings). 版式同集群级列表（§9.7）。
 * 作业级规则：sync_job 策略列表 + 常驻详情栏 / 抽屉，承载 TaskAlertForm，
 * 保存契约与任务设置页一致（PUT /sync/tasks/:id/alert）。
 */
import {useCallback, useEffect, useMemo, useRef, useState} from 'react';
import {useTranslations} from 'next-intl';
import {NeutralChip, SoftChip} from '../status-chips';
import {Button} from '@/components/arc/button/button';
import {useDrawerItem} from '@/components/arc/drawer/drawer';
import {EmptyState} from '@/components/arc/empty-state/empty-state';
import {SearchField} from '@/components/arc/search-field/search-field';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {Skeleton} from '@/components/arc/skeleton/skeleton';
import {Switch} from '@/components/arc/switch/switch';
import {TableCard} from '@/components/common/layout/TableCard';
import services from '@/lib/services';
import type {
  AlertPolicy,
  NotificationChannel,
  NotificationRecipientUser,
} from '@/lib/services/monitoring';
import type {
  SyncTaskAlertConfig,
  SyncTaskAlertRequest,
} from '@/lib/services/sync';
import {TaskAlertForm} from '@/components/common/sync/TaskAlertForm';
import {cn} from '@/lib/utils';
import {DiscardConfirmStrip, useBeforeUnload} from '../dirty-close';
import {DetailHost, useDetailNarrow} from '../detail-host';
import styles from '../monitoring.module.css';

type JobStatusFilter = 'all' | 'enabled' | 'disabled';

export function JobRulesPanel() {
  const t = useTranslations('monitoring');
  const ts = useTranslations('syncAlert');
  const narrow = useDetailNarrow();
  const [policies, setPolicies] = useState<AlertPolicy[]>([]);
  const [channels, setChannels] = useState<NotificationChannel[]>([]);
  const [users, setUsers] = useState<NotificationRecipientUser[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [toggling, setToggling] = useState<number | null>(null);
  const [statusFilter, setStatusFilter] = useState<JobStatusFilter>('all');
  const [query, setQuery] = useState('');
  const [editing, setEditing] = useState<{
    taskId: number;
    taskName?: string;
    clusterId?: number | null;
    config: SyncTaskAlertConfig | null;
  } | null>(null);
  const [saving, setSaving] = useState(false);
  const [formDirty, setFormDirty] = useState(false);
  const [discardArmed, setDiscardArmed] = useState(false);
  // 放弃后要跳转的目标：null = 直接关闭，否则换到另一行。
  // Where to land after discard: null closes, a policy switches rows.
  const pendingRef = useRef<AlertPolicy | null | 'close'>('close');
  const searchRef = useRef<HTMLInputElement>(null);
  useBeforeUnload(formDirty && editing != null);

  // 打开动画期间保留最后一次编辑对象，抽屉滑出时表单不先消失。
  // Keep the last editing object mounted while the drawer slides out.
  const drawerEditing = useDrawerItem(editing);

  // dirty 时关闭（X / Esc / 遮罩 / 表单取消 / 切换行）先拦截，footer 出确认行；放弃才关。
  // A dirty close is intercepted first; only discard actually closes.
  const requestClose = useCallback(() => {
    if (saving) {
      return;
    }
    if (formDirty) {
      pendingRef.current = 'close';
      setDiscardArmed(true);
      return;
    }
    setEditing(null);
    setDiscardArmed(false);
  }, [formDirty, saving]);

  const openPolicy = useCallback((policy: AlertPolicy) => {
    const taskId = policy.task_id;
    if (!taskId) {
      return;
    }
    setEditing({
      taskId,
      taskName: policy.task_name,
      clusterId: Number(policy.cluster_id) || null,
      config: null,
    });
    void services.sync
      .getTaskAlert(taskId)
      .then((config) =>
        setEditing((cur) => (cur?.taskId === taskId ? {...cur, config} : cur)),
      )
      .catch((err) => {
        setError(err instanceof Error ? err.message : String(err));
        setEditing(null);
      });
  }, []);

  const discardAndClose = useCallback(() => {
    const target = pendingRef.current;
    pendingRef.current = 'close';
    setFormDirty(false);
    setDiscardArmed(false);
    setEditing(null);
    if (target && target !== 'close') {
      openPolicy(target);
    }
  }, [openPolicy]);

  useEffect(() => {
    if (!formDirty) {
      setDiscardArmed(false);
    }
  }, [formDirty]);

  const load = useCallback(async () => {
    const [pol, ch, us] = await Promise.all([
      services.monitoring.listAlertPolicies({policy_type: 'sync_job'}),
      services.monitoring
        .listNotificationChannels()
        .catch(() => ({channels: []})),
      services.monitoring.listNotifiableUsers().catch(() => ({users: []})),
    ]);
    setPolicies(pol?.policies ?? []);
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

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    return policies.filter((p) => {
      if (statusFilter === 'enabled' && !p.enabled) {
        return false;
      }
      if (statusFilter === 'disabled' && p.enabled) {
        return false;
      }
      const name = p.task_name ?? `#${p.task_id}`;
      if (q && !name.toLowerCase().includes(q)) {
        return false;
      }
      return true;
    });
  }, [policies, statusFilter, query]);

  const segCounts = useMemo(
    () => ({
      all: policies.length,
      enabled: policies.filter((p) => p.enabled).length,
      disabled: policies.filter((p) => !p.enabled).length,
    }),
    [policies],
  );

  const toggle = useCallback(
    async (policy: AlertPolicy, enabled: boolean) => {
      if (toggling != null) {
        return;
      }
      setToggling(policy.id);
      setError(null);
      const prev = policy.enabled;
      setPolicies((ps) =>
        ps.map((p) => (p.id === policy.id ? {...p, enabled} : p)),
      );
      try {
        const updated = await services.monitoring.updateAlertPolicy(policy.id, {
          name: policy.name,
          description: policy.description,
          cluster_id: policy.cluster_id,
          policy_type: 'sync_job',
          severity: policy.severity,
          enabled,
          cooldown_minutes: policy.cooldown_minutes,
          send_recovery: policy.send_recovery,
          conditions: policy.conditions || undefined,
          receiver_user_ids: policy.receiver_user_ids,
          notification_channel_ids: policy.notification_channel_ids,
          task_id: policy.task_id,
          notify_task_owner: policy.notify_task_owner,
        });
        setPolicies((ps) =>
          ps.map((p) => (p.id === policy.id ? {...p, ...updated} : p)),
        );
      } catch (err) {
        setPolicies((ps) =>
          ps.map((p) => (p.id === policy.id ? {...p, enabled: prev} : p)),
        );
        setError(err instanceof Error ? err.message : String(err));
      } finally {
        setToggling(null);
      }
    },
    [toggling],
  );

  const openEditor = useCallback(
    (policy: AlertPolicy) => {
      if (!policy.task_id) {
        return;
      }
      if (formDirty) {
        // 未保存草稿时切换行同样走放弃确认。Switching rows is intercepted like closing.
        pendingRef.current = policy;
        setDiscardArmed(true);
        return;
      }
      openPolicy(policy);
    },
    [formDirty, openPolicy],
  );

  const save = useCallback(
    async (request: SyncTaskAlertRequest) => {
      if (!editing) {
        return;
      }
      setSaving(true);
      try {
        await services.sync.updateTaskAlert(editing.taskId, request);
        setEditing(null);
        await load();
      } catch (err) {
        setError(err instanceof Error ? err.message : String(err));
      } finally {
        setSaving(false);
      }
    },
    [editing, load],
  );

  const remove = useCallback(async () => {
    if (!editing) {
      return;
    }
    setSaving(true);
    try {
      await services.sync.deleteTaskAlert(editing.taskId);
      setEditing(null);
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setSaving(false);
    }
  }, [editing, load]);

  const onRowKeyDown = (
    e: React.KeyboardEvent<HTMLElement>,
    policy: AlertPolicy,
  ) => {
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
      openEditor(policy);
    }
  };

  const activeIndex = editing
    ? filtered.findIndex((p) => p.task_id === editing.taskId)
    : -1;
  const detailNav =
    editing && activeIndex >= 0
      ? {
          index: activeIndex,
          total: filtered.length,
          onPrev: () =>
            activeIndex > 0 && openEditor(filtered[activeIndex - 1]),
          onNext: () =>
            activeIndex < filtered.length - 1 &&
            openEditor(filtered[activeIndex + 1]),
        }
      : undefined;

  const seg = (v: JobStatusFilter, label: string, count: number) => ({
    value: v,
    label,
    accessory: <span className={styles.count}>{count}</span>,
  });

  return (
    <div className={styles.split}>
      <div className={styles.listCol} data-mcols='job'>
        <div className={styles.toolbar}>
          <SegmentedControl
            label={t('alertStatusLabel')}
            value={statusFilter}
            onValueChange={(v) => setStatusFilter(v as JobStatusFilter)}
            options={[
              seg('all', t('chipAll'), segCounts.all),
              seg('enabled', t('chipEnabled'), segCounts.enabled),
              seg('disabled', t('chipDisabled'), segCounts.disabled),
            ]}
          />
          <div className={styles.toolbarSearch}>
            <SearchField
              ref={searchRef}
              label={t('searchLabel')}
              value={query}
              onValueChange={setQuery}
              placeholder={t('jobSearchPlaceholder')}
              clearLabel={t('searchClear')}
              onKeyDown={(e) => {
                if (e.key === 'Escape' && query) {
                  setQuery('');
                }
              }}
            />
          </div>
        </div>

        {/* 唯一主卡片：骨架/错误/空态由 TableCard 统一承载（§4.4）。
            Single main card; skeleton/error/empty states are owned by TableCard. */}
        <TableCard
          loading={loading}
          hasData={filtered.length > 0}
          skeletonColumns={3}
          skeletonRows={6}
          error={error}
          onRetry={() => {
            setError(null);
            setLoading(true);
            void load().catch((err) => {
              setError(err instanceof Error ? err.message : String(err));
              setLoading(false);
            });
          }}
          empty={
            <EmptyState
              title={policies.length === 0 ? t('jobEmpty') : t('emptyFiltered')}
              description={
                policies.length === 0
                  ? t('jobRulesEmptyDesc')
                  : t('emptyFilteredDesc')
              }
              action={
                statusFilter !== 'all' || query ? (
                  <Button
                    type='button'
                    size='sm'
                    variant='secondary'
                    onClick={() => {
                      setStatusFilter('all');
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
            <span>{t('jobColTask')}</span>
            <span>{t('colAlert')}</span>
            <span className={styles.right}>{t('fieldEnabled')}</span>
          </div>
          <div className={styles.tableScroll}>
            {filtered.map((policy) => (
              <div
                key={policy.id}
                data-testid={`job-rule-row-${policy.id}`}
                data-mrow={policy.id}
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
                  openEditor(policy);
                }}
                onKeyDown={(e) => onRowKeyDown(e, policy)}
                className={cn(
                  styles.row,
                  editing?.taskId === policy.task_id && styles.rowActive,
                  !policy.enabled && styles.rowMuted,
                )}
              >
                <div className={styles.rowName}>
                  <span className={cn(styles.name, styles.mono)}>
                    {policy.task_name ?? `#${policy.task_id}`}
                  </span>
                </div>
                <span className={cn(styles.inlineBadgeRow, 'min-w-0')}>
                  {(policy.conditions ?? []).slice(0, 3).map((c, i) => (
                    <NeutralChip key={i}>
                      {ts.has?.(`rule_${c.metric_key}`)
                        ? ts(`rule_${c.metric_key}`)
                        : c.metric_key}
                    </NeutralChip>
                  ))}
                  <SoftChip
                    tone={
                      policy.notification_channel_ids?.length ||
                      policy.receiver_user_ids?.length
                        ? 'blue'
                        : 'muted'
                    }
                  >
                    {policy.notification_channel_ids?.length ||
                    policy.receiver_user_ids?.length
                      ? t('notify_custom')
                      : t('notify_none')}
                  </SoftChip>
                </span>
                <div className={styles.rowActions}>
                  <Switch
                    checked={policy.enabled}
                    disabled={toggling === policy.id}
                    onCheckedChange={(on) => void toggle(policy, on)}
                    onClick={(e) => e.stopPropagation()}
                    aria-label={t('fieldEnabled')}
                  />
                </div>
              </div>
            ))}
          </div>
        </TableCard>
      </div>

      <DetailHost
        open={editing != null}
        hasContent={drawerEditing != null}
        onOpenChange={(o) => !o && requestClose()}
        narrow={narrow}
        title={ts('dialogTitle')}
        meta={drawerEditing?.taskName}
        nav={detailNav}
        wide
        drawerWidth='w-[min(92vw,560px)]'
      >
        {drawerEditing ? (
          drawerEditing.config ? (
            <div className={styles.detailCol}>
              <div className={styles.detailFill} style={{position: 'relative'}}>
                <TaskAlertForm
                  initial={drawerEditing.config}
                  clusterId={drawerEditing.clusterId}
                  channels={channels}
                  users={users}
                  saving={saving}
                  pinnedFooter
                  onSubmit={(request) => void save(request)}
                  onCancel={requestClose}
                  onDelete={() => void remove()}
                  onDirtyChange={setFormDirty}
                />
                {discardArmed ? (
                  <div
                    className={styles.detailFoot}
                    style={{position: 'absolute', left: 0, right: 0, bottom: 0}}
                  >
                    <DiscardConfirmStrip
                      onDiscard={discardAndClose}
                      onKeepEditing={() => setDiscardArmed(false)}
                    />
                  </div>
                ) : null}
              </div>
            </div>
          ) : (
            <div className={styles.detailCol}>
              <div className={styles.detailScroll} aria-busy>
                {Array.from({length: 6}).map((_, i) => (
                  <Skeleton key={i} className='h-10 w-full' lines={1} />
                ))}
              </div>
            </div>
          )
        ) : null}
      </DetailHost>
    </div>
  );
}
