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
 * Job-level rules: sync_job alert policies with inline enable switch and a drawer
 * hosting the shared TaskAlertForm (same save API as task settings).
 * 作业级规则：sync_job 策略列表，行内开关；编辑在 drawer 里承载 TaskAlertForm，
 * 保存契约与任务设置页一致（PUT /sync/tasks/:id/alert）。
 */
import {useCallback, useEffect, useState} from 'react';
import {useTranslations} from 'next-intl';
import {ChevronRight} from 'lucide-react';
import {Badge} from '@/components/arc/badge/badge';
import {Drawer, DrawerContent} from '@/components/arc/drawer/drawer';
import {Skeleton} from '@/components/arc/skeleton/skeleton';
import {Switch} from '@/components/arc/switch/switch';
import services from '@/lib/services';
import type {
  AlertPolicy,
  NotificationChannel,
  NotificationRecipientUser,
} from '@/lib/services/monitoring';
import type {SyncTaskAlertConfig, SyncTaskAlertRequest} from '@/lib/services/sync';
import {TaskAlertForm} from '@/components/common/sync/TaskAlertForm';
import {DiscardConfirmStrip, useBeforeUnload} from '../dirty-close';

export function JobRulesPanel() {
  const t = useTranslations('monitoring');
  const ts = useTranslations('syncAlert');
  const [policies, setPolicies] = useState<AlertPolicy[]>([]);
  const [channels, setChannels] = useState<NotificationChannel[]>([]);
  const [users, setUsers] = useState<NotificationRecipientUser[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [toggling, setToggling] = useState<number | null>(null);
  const [editing, setEditing] = useState<{
    taskId: number;
    taskName?: string;
    clusterId?: number | null;
    config: SyncTaskAlertConfig | null;
  } | null>(null);
  const [saving, setSaving] = useState(false);
  const [formDirty, setFormDirty] = useState(false);
  const [discardArmed, setDiscardArmed] = useState(false);
  useBeforeUnload(formDirty && editing != null);

  // dirty 时关闭（X / Esc / 遮罩 / 表单取消）先拦截，footer 出确认行；放弃才关。
  // A dirty close (X / Esc / overlay / form cancel) is intercepted first; only
  // discard actually closes.
  const requestClose = useCallback(() => {
    if (saving) {return;}
    if (formDirty) {
      setDiscardArmed(true);
      return;
    }
    setEditing(null);
    setDiscardArmed(false);
  }, [formDirty, saving]);

  const discardAndClose = useCallback(() => {
    setEditing(null);
    setFormDirty(false);
    setDiscardArmed(false);
  }, []);

  useEffect(() => {
    if (!formDirty) {
      setDiscardArmed(false);
    }
  }, [formDirty]);

  const load = useCallback(async () => {
    const [pol, ch, us] = await Promise.all([
      services.monitoring.listAlertPolicies({policy_type: 'sync_job'}),
      services.monitoring.listNotificationChannels().catch(() => ({channels: []})),
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

  const toggle = useCallback(
    async (policy: AlertPolicy, enabled: boolean) => {
      if (toggling != null) {return;}
      setToggling(policy.id);
      setError(null);
      const prev = policy.enabled;
      setPolicies((ps) => ps.map((p) => (p.id === policy.id ? {...p, enabled} : p)));
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
        setPolicies((ps) => ps.map((p) => (p.id === policy.id ? {...p, ...updated} : p)));
      } catch (err) {
        setPolicies((ps) => ps.map((p) => (p.id === policy.id ? {...p, enabled: prev} : p)));
        setError(err instanceof Error ? err.message : String(err));
      } finally {
        setToggling(null);
      }
    },
    [toggling],
  );

  const openEditor = useCallback((policy: AlertPolicy) => {
    if (!policy.task_id) {return;}
    const taskId = policy.task_id;
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

  const save = useCallback(
    async (request: SyncTaskAlertRequest) => {
      if (!editing) {return;}
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
    if (!editing) {return;}
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

  return (
    <div className='min-h-0 flex-1 overflow-y-auto rounded-lg border border-border/60 bg-card/40'>
      {loading ? (
        <div className='space-y-2 p-3' aria-busy>
          {Array.from({length: 6}).map((_, i) => (
            <Skeleton key={i} className='h-12 w-full' lines={1} />
          ))}
        </div>
      ) : null}

      {error ? (
        <div className='m-3 rounded-lg border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive'>
          {error}
        </div>
      ) : null}

      {!loading && policies.length === 0 ? (
        <div className='flex min-h-[200px] items-center justify-center text-sm text-muted-foreground'>
          {t('jobEmpty')}
        </div>
      ) : null}

      {policies.map((policy) => (
        <button
          key={policy.id}
          type='button'
          data-testid={`job-rule-row-${policy.id}`}
          className='flex w-full items-center gap-3 border-b border-border/50 px-4 py-3 text-left last:border-b-0 hover:bg-muted/30'
          onClick={() => openEditor(policy)}
        >
          <div className='min-w-0 flex-1'>
            <div className='truncate font-mono text-sm'>{policy.task_name ?? `#${policy.task_id}`}</div>
            <div className='mt-1 flex flex-wrap gap-1'>
              {(policy.conditions ?? []).map((c, i) => (
                <Badge key={i} size='sm' outline>
                  {ts.has?.(`rule_${c.metric_key}`) ? ts(`rule_${c.metric_key}`) : c.metric_key}
                </Badge>
              ))}
              <Badge size='sm' tone='info' outline>
                {policy.notification_channel_ids?.length
                  ? t('notify_custom')
                  : policy.receiver_user_ids?.length
                    ? t('notify_custom')
                    : t('notify_none')}
              </Badge>
            </div>
          </div>
          <Switch
            checked={policy.enabled}
            disabled={toggling === policy.id}
            onCheckedChange={(on) => void toggle(policy, on)}
            onClick={(e) => e.stopPropagation()}
            aria-label={t('fieldEnabled')}
          />
          <ChevronRight className='size-4 text-muted-foreground' aria-hidden />
        </button>
      ))}

      <Drawer open={editing != null} onOpenChange={(open) => !open && requestClose()}>
        {editing?.config ? (
          <DrawerContent
            side='right'
            title={ts('dialogTitle')}
            description={editing.taskName}
            className='w-[min(92vw,560px)] rounded-none! [&>div[class*=body]]:flex-1'
          >
            {/* 与集群规则抽屉同布局：负 margin 抵消 body 内边距，表单 footer 钉底；
                关闭被拦截时确认行覆盖在表单 footer 上原位确认。
                Same layout as the cluster-rule drawer: negative margins cancel the
                body padding and the form footer is pinned; an intercepted close
                overlays the discard row exactly where the footer sits. */}
            <div className='relative -m-5 flex h-full flex-col sm:-m-6'>
              <TaskAlertForm
                initial={editing.config}
                clusterId={editing.clusterId}
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
                <div className='absolute inset-x-0 bottom-0 border-t border-border/60 bg-(--surface-raised) px-5 py-3 sm:px-6'>
                  <DiscardConfirmStrip
                    onDiscard={discardAndClose}
                    onKeepEditing={() => setDiscardArmed(false)}
                  />
                </div>
              ) : null}
            </div>
          </DrawerContent>
        ) : editing ? (
          <DrawerContent side='right' title={ts('dialogTitle')} className='w-[min(92vw,560px)] rounded-none! [&>div[class*=body]]:flex-1'>
            <div className='space-y-2 px-1 py-4' aria-busy>
              {Array.from({length: 6}).map((_, i) => (
                <Skeleton key={i} className='h-10 w-full' lines={1} />
              ))}
            </div>
          </DrawerContent>
        ) : null}
      </Drawer>
    </div>
  );
}
