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
 * Rule detail drawer body: enable switch, threshold/for editors, severity and
 * notification-mode segmented controls, cluster overrides, pinned save footer.
 * 规则详情 drawer 内容：开关、阈值/持续时间、级别与通知方式分段控件、集群例外、吸底保存栏。
 * 表单编辑的是全局层原值（null = 继承）；例外层在 accordion 里独立编辑。
 * 布局：自身 h-full flex-col + 内部独立滚动区，footer 固定在抽屉底部（依赖宿主给
 * drawer body 加 flex-1；.body 本身不拉伸）。
 */
import {useCallback, useEffect, useMemo, useState} from 'react';
import {useLocale, useTranslations} from 'next-intl';
import {Accordion, type AccordionItem} from '@/components/arc/accordion/accordion';
import {NeutralChip, SeverityChip} from '../status-chips';
import {Combobox} from '@/components/arc/combobox/combobox';
import ConfirmMorph from '@/components/arc/confirm-morph/confirm-morph';
import {MultiSelect} from '@/components/arc/multi-select/multi-select';
import {NumberField} from '@/components/arc/number-field/number-field';
import SegmentedControl from '@/components/arc/segmented-control/segmented-control';
import {Select} from '@/components/arc/select/select';
import {Switch} from '@/components/arc/switch/switch';
import {Button} from '@/components/arc/button/button';
import services from '@/lib/services';
import type {
  AlertNotificationDefaults,
  ClusterRule,
  ClusterRuleLayerRequest,
  ClusterRuleNotifyMode,
  NotificationChannel,
  NotificationRecipientUser,
} from '@/lib/services/monitoring';
import type {ClusterInfo} from '@/lib/services/cluster';
import {cn} from '@/lib/utils';
import {formatRuleThreshold, thresholdUnitLabel} from '../monitoring-utils';
import styles from '../monitoring.module.css';
import {DiscardConfirmStrip, useBeforeUnload} from '../dirty-close';
import {layerBody} from './RulesPanel';

const FOR_OPTIONS_SECONDS = [0, 60, 120, 300, 600, 900, 1800] as const;

interface RuleDetailPanelProps {
  rule: ClusterRule;
  onClose: () => void;
  onSaved: (rule: ClusterRule) => void;
  onToggleEnabled: (rule: ClusterRule, on: boolean) => void;
  onConfigureNotify?: () => void;
  /** 草稿相对打开快照是否已修改，供宿主拦截关闭。Whether drafts diverge from the open-time snapshot. */
  onDirtyChange?: (dirty: boolean) => void;
  /** 关闭被拦截后置位，footer 切换为放弃确认行。Set when a close attempt was intercepted. */
  discardArmed?: boolean;
  onDiscard?: () => void;
  onKeepEditing?: () => void;
}

/** 一个层（全局或例外）的可编辑字段；null = 继承上层。Editable layer fields; null inherits. */
interface LayerDraft {
  threshold: number | null;
  for_seconds: number | null;
  severity: 'warning' | 'critical' | null;
  notify_mode: ClusterRuleNotifyMode | null;
  channel_ids: number[];
  receiver_user_ids: number[];
}

export function RuleDetailPanel({
  rule,
  onClose,
  onSaved,
  onToggleEnabled,
  onConfigureNotify,
  onDirtyChange,
  discardArmed,
  onDiscard,
  onKeepEditing,
}: RuleDetailPanelProps) {
  const t = useTranslations('monitoring');
  const locale = useLocale();

  const [draft, setDraft] = useState<LayerDraft>({
    threshold: rule.global.threshold,
    for_seconds: rule.global.for_seconds,
    severity: (rule.global.severity as LayerDraft['severity']) ?? rule.severity,
    notify_mode:
      rule.global.notify_mode === 'none' ? 'default' : (rule.global.notify_mode ?? 'default'),
    channel_ids: rule.global.channel_ids ?? [],
    receiver_user_ids: rule.global.receiver_user_ids ?? [],
  });
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);
  const [error, setError] = useState<string | null>(null);
  // 打开时的全局层快照；保存成功后快照前移到刚保存的内容，脏判断随之复位。
  // Global-layer snapshot from open time; a successful save advances it so dirty resets.
  const [snapshot, setSnapshot] = useState(() => JSON.stringify(draft));
  const [dirtyOverrides, setDirtyOverrides] = useState<Set<number>>(new Set());
  const [channels, setChannels] = useState<NotificationChannel[]>([]);
  const [users, setUsers] = useState<NotificationRecipientUser[]>([]);
  const [clusters, setClusters] = useState<ClusterInfo[]>([]);
  const [defaults, setDefaults] = useState<AlertNotificationDefaults | null>(null);
  const [addingCluster, setAddingCluster] = useState(false);

  useEffect(() => {
    Promise.all([
      services.monitoring.listNotificationChannels().catch(() => ({channels: []})),
      services.monitoring.listNotifiableUsers().catch(() => ({users: []})),
      services.monitoring.getAlertNotificationDefaults().catch(() => null),
      services.cluster.getClusters({current: 1, size: 200}).catch(() => null),
    ]).then(([ch, us, def, cl]) => {
      setChannels(ch.channels ?? []);
      setUsers(us.users ?? []);
      setDefaults(def);
      setClusters((cl?.clusters ?? []) as ClusterInfo[]);
    });
  }, []);

  const severity = draft.severity ?? rule.severity;
  const notifyMode = draft.notify_mode ?? 'default';
  // critical 生效时必须至少有一个启用的外部渠道；default 模式靠默认通知兜底，由后端校验。
  // Effective critical requires one enabled external channel; default mode relies on defaults.
  const enabledChannels = useMemo(() => channels.filter((c) => c.enabled), [channels]);
  const customMissingChannel =
    severity === 'critical' &&
    notifyMode === 'custom' &&
    draft.channel_ids.filter((id) => enabledChannels.some((c) => c.id === id)).length === 0 &&
    draft.receiver_user_ids.length === 0;
  const saveDisabled = saving || customMissingChannel;

  const channelOptions = channels.map((c) => ({value: String(c.id), label: c.name}));
  const userOptions = users.map((u) => ({value: String(u.id), label: u.nickname || u.username}));

  // 例外编辑器各自的脏标记合并进抽屉级脏判断。
  // Per-override dirty flags fold into the drawer-level dirty state.
  const dirty = JSON.stringify(draft) !== snapshot || dirtyOverrides.size > 0;
  useEffect(() => {
    onDirtyChange?.(dirty);
  }, [dirty, onDirtyChange]);
  useBeforeUnload(dirty);

  // 必须在值无变化时返回原 Set：OverrideEditor 的 onDirtyChange 回调每次渲染都是新
  // 函数，effect 会随之重跑；不 bail 的话会 setState→渲染→effect 死循环。
  // Must bail out with the same Set on no-ops: OverrideEditor gets a fresh
  // callback each render, so its effect re-fires — without the early return
  // this would loop setState → render → effect forever.
  const setOverrideDirty = useCallback((clusterId: number, flag: boolean) => {
    setDirtyOverrides((prev) => {
      if (prev.has(clusterId) === flag) {
        return prev;
      }
      const next = new Set(prev);
      if (flag) {next.add(clusterId);} else {next.delete(clusterId);}
      return next;
    });
  }, []);

  // 默认通知摘要：把已配置渠道/接收人拼成一行说明，未配置时给未配置提示。
  // Default-notification summary as one muted line; unset defaults get an unset hint.
  const defaultsTargets = useMemo(() => {
    if (!defaults?.configured) {return null;}
    const names = [
      ...(defaults.channel_ids ?? []).map((id) => channels.find((c) => c.id === id)?.name),
      ...(defaults.receiver_user_ids ?? []).map(
        (id) => users.find((u) => u.id === id)?.nickname ?? users.find((u) => u.id === id)?.username,
      ),
    ].filter(Boolean);
    return names.length ? names.join('、') : null;
  }, [defaults, channels, users]);

  const save = async () => {
    setSaving(true);
    setError(null);
    try {
      const updated = await services.monitoring.updateClusterRule(
        rule.key,
        layerBody(rule.global, {
          threshold: draft.threshold,
          for_seconds: draft.for_seconds,
          severity: draft.severity,
          notify_mode: draft.notify_mode,
          channel_ids: draft.channel_ids.length ? draft.channel_ids : null,
          receiver_user_ids: draft.receiver_user_ids.length ? draft.receiver_user_ids : null,
        }),
      );
      onSaved(updated);
      setSnapshot(JSON.stringify(draft));
      setSaved(true);
      setTimeout(() => setSaved(false), 1600);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setSaving(false);
    }
  };

  const restoreDefault = async () => {
    await services.monitoring.deleteClusterRule(rule.key);
    const fresh = await services.monitoring.getClusterRule(rule.key);
    onSaved(fresh);
    onClose();
  };

  const addOverride = async (clusterId: number) => {
    // 空层 = 全部字段继承全局。Empty layer inherits every field from global.
    const updated = await services.monitoring.updateClusterRuleOverride(rule.key, clusterId, {});
    onSaved(updated);
    setAddingCluster(false);
  };

  const saveOverride = async (clusterId: number, layer: ClusterRuleLayerRequest) => {
    const updated = await services.monitoring.updateClusterRuleOverride(rule.key, clusterId, layer);
    onSaved(updated);
  };

  const deleteOverride = async (clusterId: number) => {
    await services.monitoring.deleteClusterRuleOverride(rule.key, clusterId);
    const fresh = await services.monitoring.getClusterRule(rule.key);
    onSaved(fresh);
  };

  const overriddenClusterIds = new Set(rule.overrides.map((o) => o.cluster_id));
  const addableClusters = clusters.filter((c) => !overriddenClusterIds.has(c.id));

  return (
    <div data-testid='rule-detail-panel' className={styles.detailCol}>
      {/* 级别 + key 作为头部视觉延伸，固定不滚动。
          Severity + key extend the header; pinned above the scroll area. */}
      <div className={styles.metaBand}>
        <SeverityChip
          severity={severity}
          label={
            severity === 'critical'
              ? t('severityCritical')
              : t('severityWarning')
          }
        />
        <span className={cn(styles.sub, styles.mono)}>{rule.key}</span>
      </div>

      <div className={styles.detailScroll}>
        <p className={styles.descText}>
          {locale === 'en' ? (rule.description_en || rule.description) : rule.description}
        </p>

        {/* 全局层设置分组：左标签右控件的分隔行，一个分组一个边框。
            Global-layer settings group: label-left/control-right rows, one border. */}
        <div className={styles.groupInner}>
          <div className={styles.groupRow}>
            <div className={styles.rowLabel}>
              <span className={styles.rowLabelText}>{t('fieldEnabled')}</span>
            </div>
            <Switch
              checked={rule.enabled}
              onCheckedChange={(on) => void onToggleEnabled(rule, on)}
              aria-label={t('fieldEnabled')}
            />
          </div>

          {/* 有阈值时阈值/持续时间两列并排（标签同基线）；无阈值只渲染持续时间。
              Threshold + duration share one two-column row (aligned labels);
              no-threshold rules render duration only, no placeholder. */}
          <div className={cn(styles.groupRow, styles.groupRowStack)}>
            <div className={rule.threshold ? styles.fieldRow : cn(styles.fieldRow, 'grid-cols-1 sm:grid-cols-2')}>
              {rule.threshold ? (
                rule.threshold.value == null && draft.threshold == null ? (
                  <div className={styles.field}>
                    <span className={styles.fieldLabel}>{t('fieldThreshold')}</span>
                    {/* null 阈值 = "等于部署数"动态语义，NumberField 无法表示空值，
                        提供入口切换到固定值编辑。
                        A null threshold is dynamic (= deployed count); NumberField
                        can't render emptiness, so offer a switch into fixed-value editing. */}
                    <span className={styles.fieldDesc}>
                      {t('thresholdDeployedHint')}{' '}
                      <button
                        type='button'
                        className={styles.linkBtn}
                        onClick={() => setDraft((d) => ({...d, threshold: rule.threshold?.min ?? 1}))}
                      >
                        {t('thresholdSetFixed')}
                      </button>
                    </span>
                  </div>
                ) : (
                  <div className={styles.field}>
                    <NumberField
                      label={t('fieldThreshold')}
                      value={draft.threshold ?? rule.threshold.value ?? undefined}
                      onValueChange={(v) => setDraft((d) => ({...d, threshold: v}))}
                      min={rule.threshold.min}
                      max={rule.threshold.max}
                      prefix={rule.threshold.operator}
                      suffix={thresholdUnitLabel(rule.threshold.unit, t) || undefined}
                    />
                    <span className={styles.fieldDesc}>
                      {t('thresholdRange', {min: rule.threshold.min, max: rule.threshold.max})}
                    </span>
                  </div>
                )
              ) : null}
              <div className={styles.field}>
                <Select
                  label={t('fieldFor')}
                  value={String(draft.for_seconds ?? rule.for_seconds)}
                  onValueChange={(v) => setDraft((d) => ({...d, for_seconds: Number(v)}))}
                  disabled={!rule.for_editable}
                  options={FOR_OPTIONS_SECONDS.map((sec) => ({
                    value: String(sec),
                    label: sec === 0 ? t('realtime') : t('durationMinutes', {minutes: sec / 60}),
                  }))}
                />
              </div>
            </div>
          </div>

          <div className={cn(styles.groupRow, styles.groupRowStack)}>
            <span className={styles.fieldLabel}>{t('fieldSeverity')}</span>
            <div>
              <SegmentedControl
                label={t('fieldSeverity')}
                value={severity}
                onValueChange={(v) => setDraft((d) => ({...d, severity: v as LayerDraft['severity']}))}
                options={[
                  {value: 'warning', label: t('severityWarning')},
                  {value: 'critical', label: t('severityCritical')},
                ]}
              />
            </div>
            <span className={styles.fieldDesc}>
              {severity === 'critical' ? t('severityCriticalDesc') : t('severityWarningDesc')}
            </span>
          </div>

          {severity === 'critical' ? (
            <>
              <div className={cn(styles.groupRow, styles.groupRowStack)}>
                <span className={styles.fieldLabel}>{t('fieldNotify')}</span>
                <div>
                  <SegmentedControl
                    label={t('fieldNotify')}
                    value={notifyMode}
                    onValueChange={(v) =>
                      setDraft((d) => ({...d, notify_mode: v as ClusterRuleNotifyMode}))
                    }
                    options={[
                      {value: 'default', label: t('notifyDefault')},
                      {value: 'custom', label: t('notifyCustom')},
                    ]}
                  />
                </div>
                <span className={styles.fieldDesc}>
                  {notifyMode === 'custom'
                    ? t('notifyCustomDesc')
                    : defaultsTargets
                      ? t('notifyDefaultSummary', {targets: defaultsTargets})
                      : defaults?.configured
                        ? t('notifyDefaultDesc')
                        : t('notifyDefaultUnset')}
                  {notifyMode === 'default' && !defaults?.configured && onConfigureNotify ? (
                    <>
                      {' '}
                      <button
                        type='button'
                        className={styles.linkBtn}
                        onClick={onConfigureNotify}
                      >
                        {t('goConfigureNotify')}
                      </button>
                    </>
                  ) : null}
                </span>
              </div>
              {notifyMode === 'custom' ? (
                <div className={cn(styles.groupRow, styles.groupRowStack)}>
                  <MultiSelect
                    label={t('notifyChannels')}
                    options={channelOptions}
                    value={draft.channel_ids.map(String)}
                    onValueChange={(v) => setDraft((d) => ({...d, channel_ids: v.map(Number)}))}
                    placeholder={t('notifyChannelsPlaceholder')}
                  />
                  <MultiSelect
                    label={t('notifyReceivers')}
                    options={userOptions}
                    value={draft.receiver_user_ids.map(String)}
                    onValueChange={(v) => setDraft((d) => ({...d, receiver_user_ids: v.map(Number)}))}
                    placeholder={t('notifyReceiversPlaceholder')}
                  />
                  {customMissingChannel ? (
                    <p className={cn(styles.rowHint, styles.dangerText)}>
                      {t('notifyMissingForCritical')}
                      {onConfigureNotify ? (
                        <>
                          {' '}
                          <button
                            type='button'
                            className={styles.linkBtn}
                            onClick={onConfigureNotify}
                          >
                            {t('goConfigureNotify')}
                          </button>
                        </>
                      ) : null}
                    </p>
                  ) : null}
                </div>
              ) : null}
            </>
          ) : null}
        </div>

        <OverridesSection
          rule={rule}
          addable={addableClusters}
          adding={addingCluster}
          onAdd={(id) => void addOverride(id)}
          onToggleAdd={setAddingCluster}
          onSave={saveOverride}
          onDelete={deleteOverride}
          onDirtyChange={setOverrideDirty}
          channels={channels}
          users={users}
        />
      </div>

      {/* 底栏常驻详情底部：保存只写全局层；保存中/已保存/错误内联在按钮旁；
          恢复默认走 ConfirmMorph；关闭被拦截时原位换成放弃确认行。
          Persistent bottom bar; save writes the global layer, status stays inline;
          restore-default goes through ConfirmMorph; an intercepted close swaps
          in the discard row. */}
      <div className={styles.detailFoot}>
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
              loading={saving}
              disabled={saveDisabled}
              onClick={() => void save()}
            >
              {t('save')}
            </Button>
            {saving ? <span className={cn(styles.xsText, styles.muted)}>{t('saving')}</span> : null}
            {!saving && saved ? <span className={cn(styles.xsText, styles.muted)}>{t('saved')}</span> : null}
            {error ? (
              <span className={cn(styles.xsText, styles.dangerText, 'min-w-0 truncate')} title={error}>
                {error}
              </span>
            ) : null}
            <span className={styles.detailFootGrow} />
            {/* 恢复默认 = 删除全局层配置回到目录默认，单项确认用 ConfirmMorph。
                Restore deletes the global layer back to catalog defaults. */}
            <ConfirmMorph
              label={t('restoreDefault')}
              prompt={t('restoreDefaultConfirm')}
              confirmLabel={t('restoreDefault')}
              cancelLabel={t('deleteCancel')}
              pendingLabel={t('saving')}
              tone='neutral'
              onConfirm={restoreDefault}
            />
          </>
        )}
      </div>
    </div>
  );
}

function OverridesSection({
  rule,
  addable,
  adding,
  onAdd,
  onToggleAdd,
  onSave,
  onDelete,
  onDirtyChange,
  channels,
  users,
}: {
  rule: ClusterRule;
  addable: ClusterInfo[];
  adding: boolean;
  onAdd: (clusterId: number) => void;
  onToggleAdd: (open: boolean) => void;
  onSave: (clusterId: number, layer: ClusterRuleLayerRequest) => Promise<void>;
  onDelete: (clusterId: number) => Promise<void>;
  onDirtyChange?: (clusterId: number, dirty: boolean) => void;
  channels: NotificationChannel[];
  users: NotificationRecipientUser[];
}) {
  const t = useTranslations('monitoring');

  const items: AccordionItem[] = rule.overrides.map((ov) => ({
    title: ov.cluster_name || `#${ov.cluster_id}`,
    content: (
      <OverrideEditor
        key={ov.cluster_id}
        rule={rule}
        override={ov}
        channels={channels}
        users={users}
        onSave={(layer) => onSave(ov.cluster_id, layer)}
        onDelete={() => onDelete(ov.cluster_id)}
        onDirtyChange={(flag) => onDirtyChange?.(ov.cluster_id, flag)}
      />
    ),
  }));

  return (
    <div className={styles.sectionStack} style={{gap: 8}}>
      <div className={styles.sectionHead}>
        <span className={styles.sectionTitle}>{t('detailExceptions')}</span>
        <Button
          type='button'
          variant='ghost'
          size='sm'
          disabled={addable.length === 0}
          onClick={() => onToggleAdd(!adding)}
        >
          {t('detailAddException')}
        </Button>
      </div>
      {adding ? (
        <Combobox
          label={t('colCluster')}
          options={addable.map((c) => ({value: String(c.id), label: c.name}))}
          onValueChange={(v) => {
            const id = Number(v);
            if (id) {onAdd(id);}
          }}
          placeholder={t('searchPlaceholder')}
          emptyMessage={t('comboboxEmpty')}
        />
      ) : null}
      {items.length ? (
        <Accordion items={items} />
      ) : (
        <p className={styles.hintBox}>{t('exceptionsEmpty')}</p>
      )}
    </div>
  );
}

function OverrideEditor({
  rule,
  override: ov,
  channels,
  users,
  onSave,
  onDelete,
  onDirtyChange,
}: {
  rule: ClusterRule;
  override: ClusterRule['overrides'][number];
  channels: NotificationChannel[];
  users: NotificationRecipientUser[];
  onSave: (layer: ClusterRuleLayerRequest) => Promise<void>;
  onDelete: () => Promise<void>;
  onDirtyChange?: (dirty: boolean) => void;
}) {
  const t = useTranslations('monitoring');
  const [draft, setDraft] = useState<ClusterRuleLayerRequest>({
    enabled: ov.enabled,
    threshold: ov.threshold,
    for_seconds: ov.for_seconds,
    severity: (ov.severity as ClusterRuleLayerRequest['severity']) ?? null,
    notify_mode: (ov.notify_mode as ClusterRuleLayerRequest['notify_mode']) ?? null,
    channel_ids: ov.channel_ids ?? null,
    receiver_user_ids: ov.receiver_user_ids ?? null,
  });
  const [snapshot, setSnapshot] = useState(() => JSON.stringify(draft));
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const dirty = JSON.stringify(draft) !== snapshot;
  useEffect(() => {
    onDirtyChange?.(dirty);
  }, [dirty, onDirtyChange]);

  const eff = ov.effective;
  const enabledChannels = channels.filter((c) => c.enabled);
  const effSeverity = draft.severity ?? eff.severity;
  const effNotify = draft.notify_mode ?? 'default';
  const missing =
    effSeverity === 'critical' &&
    effNotify === 'custom' &&
    (draft.channel_ids ?? []).filter((id) => enabledChannels.some((c) => c.id === id)).length === 0 &&
    (draft.receiver_user_ids ?? []).length === 0;

  const submit = async () => {
    setSaving(true);
    setErr(null);
    try {
      await onSave(draft);
      setSnapshot(JSON.stringify(draft));
      setSaved(true);
      setTimeout(() => setSaved(false), 1600);
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setSaving(false);
    }
  };

  const overriddenFields = [
    ov.enabled != null ? t('fieldEnabled') : null,
    ov.threshold != null ? t('fieldThreshold') : null,
    ov.for_seconds != null ? t('fieldFor') : null,
    ov.severity != null ? t('fieldSeverity') : null,
    ov.notify_mode != null ? t('fieldNotify') : null,
  ].filter(Boolean) as string[];

  return (
    <div className={cn(styles.groupInner, styles.sectionStack)} style={{gap: 0}}>
      {overriddenFields.length ? (
        <div className={styles.groupRow}>
          <div className={cn(styles.rowLabel, styles.inlineBadgeRow)}>
            {overriddenFields.map((f) => (
              <NeutralChip key={f}>{f}</NeutralChip>
            ))}
          </div>
        </div>
      ) : (
        <div className={styles.groupRow}>
          <span className={styles.rowHint}>{t('inheritGlobal')}</span>
        </div>
      )}

      <div className={styles.groupRow}>
        <div className={styles.rowLabel}>
          <span className={styles.rowLabelText}>{t('fieldEnabled')}</span>
        </div>
        <Switch
          checked={draft.enabled ?? eff.enabled}
          onCheckedChange={(on) => setDraft((d) => ({...d, enabled: on}))}
          aria-label={t('fieldEnabled')}
        />
      </div>

      {rule.threshold ? (
        <div className={cn(styles.groupRow, styles.groupRowStack)}>
          {draft.threshold != null ? (
            <div className={styles.field}>
              <NumberField
                label={t('fieldThreshold')}
                value={draft.threshold}
                onValueChange={(v) => setDraft((d) => ({...d, threshold: v}))}
                min={rule.threshold.min}
                max={rule.threshold.max}
                suffix={thresholdUnitLabel(rule.threshold.unit, t) || undefined}
              />
              <span className={styles.fieldDesc}>
                <button
                  type='button'
                  className={styles.linkBtn}
                  onClick={() => setDraft((d) => ({...d, threshold: null}))}
                >
                  {t('inheritGlobal')}
                </button>
              </span>
            </div>
          ) : (
            <div className={styles.field}>
              <span className={styles.fieldLabel}>{t('fieldThreshold')}</span>
              <span className={styles.fieldDesc}>
                {t('inheritGlobal')}{' '}
                <button
                  type='button'
                  className={styles.linkBtn}
                  onClick={() =>
                    setDraft((d) => ({
                      ...d,
                      // 生效阈值 null（=部署数）时用 min 兜底，否则空值永远进不了编辑态
                      // fall back to min when the effective threshold is null (= deployed count)
                      threshold:
                        ov.effective.threshold ?? rule.threshold?.value ?? rule.threshold?.min ?? 1,
                    }))
                  }
                >
                  {t('detailAddException')}
                </button>
              </span>
            </div>
          )}
        </div>
      ) : null}

      <div className={cn(styles.groupRow, styles.groupRowStack)}>
        <Select
          label={t('fieldFor')}
          value={draft.for_seconds != null ? String(draft.for_seconds) : 'inherit'}
          onValueChange={(v) =>
            setDraft((d) => ({...d, for_seconds: v === 'inherit' ? null : Number(v)}))
          }
          disabled={!rule.for_editable}
          options={[
            {value: 'inherit', label: `${t('inheritGlobal')} (${formatRuleThreshold(rule, t)})`},
            ...FOR_OPTIONS_SECONDS.map((sec) => ({
              value: String(sec),
              label: sec === 0 ? t('realtime') : t('durationMinutes', {minutes: sec / 60}),
            })),
          ]}
        />
      </div>

      <div className={cn(styles.groupRow, styles.groupRowStack)}>
        <Select
          label={t('fieldSeverity')}
          value={draft.severity ?? 'inherit'}
          onValueChange={(v) =>
            setDraft((d) => ({...d, severity: v === 'inherit' ? null : (v as 'warning' | 'critical')}))
          }
          options={[
            {value: 'inherit', label: `${t('inheritGlobal')} (${eff.severity === 'critical' ? t('severityCritical') : t('severityWarning')})`},
            {value: 'warning', label: t('severityWarning')},
            {value: 'critical', label: t('severityCritical')},
          ]}
        />
      </div>

      {effSeverity === 'critical' ? (
        <>
          <div className={cn(styles.groupRow, styles.groupRowStack)}>
            <Select
              label={t('fieldNotify')}
              value={draft.notify_mode ?? 'inherit'}
              onValueChange={(v) =>
                setDraft((d) => ({...d, notify_mode: v === 'inherit' ? null : (v as ClusterRuleNotifyMode)}))
              }
              options={[
                {value: 'inherit', label: t('inheritGlobal')},
                {value: 'default', label: t('notifyDefault')},
                {value: 'custom', label: t('notifyCustom')},
              ]}
            />
          </div>
          {effNotify === 'custom' ? (
            <div className={cn(styles.groupRow, styles.groupRowStack)}>
              <MultiSelect
                label={t('notifyChannels')}
                options={channels.map((c) => ({value: String(c.id), label: c.name}))}
                value={(draft.channel_ids ?? []).map(String)}
                onValueChange={(v) => setDraft((d) => ({...d, channel_ids: v.map(Number)}))}
                placeholder={t('notifyChannelsPlaceholder')}
              />
              <MultiSelect
                label={t('notifyReceivers')}
                options={users.map((u) => ({value: String(u.id), label: u.nickname || u.username}))}
                value={(draft.receiver_user_ids ?? []).map(String)}
                onValueChange={(v) => setDraft((d) => ({...d, receiver_user_ids: v.map(Number)}))}
                placeholder={t('notifyReceiversPlaceholder')}
              />
              {missing ? <p className={cn(styles.rowHint, styles.dangerText)}>{t('notifyMissingForCritical')}</p> : null}
            </div>
          ) : null}
        </>
      ) : null}

      {err ? <div className={styles.groupRow}><span className={cn(styles.rowHint, styles.dangerText)}>{err}</span></div> : null}

      <div className={styles.groupRow}>
        <span className={styles.rowLabel} />
        <Button
          type='button'
          size='sm'
          loading={saving}
          disabled={saving || missing}
          onClick={() => void submit()}
        >
          {saving ? t('saving') : saved ? t('saved') : t('save')}
        </Button>
        <ConfirmMorph
          label={t('deleteException')}
          prompt={t('deleteExceptionPrompt', {name: ov.cluster_name || `#${ov.cluster_id}`})}
          confirmLabel={t('deleteConfirm')}
          cancelLabel={t('deleteCancel')}
          pendingLabel={t('saving')}
          onConfirm={onDelete}
        />
      </div>
    </div>
  );
}
