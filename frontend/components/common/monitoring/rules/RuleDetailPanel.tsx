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
 * Rule detail drawer body: global config + compact per-cluster override diff list,
 * plus a focused subview for editing one cluster's override (entered via drill-down
 * or directly from a cluster context view).
 * 规则详情面板：全局配置 + 紧凑的按集群差异摘要列表，以及单集群专属配置的聚焦子视图
 * （可从全局视图下钻进入，或在顶部切换集群视角后直接进入）。
 * 布局：自身 h-full flex-col + 内部独立滚动区，底栏按当前子视图固定单一保存动作，
 * 避免全局层与集群层两套保存按钮同时出现造成冲突。
 */
import {useCallback, useEffect, useMemo, useState} from 'react';
import {useLocale, useTranslations} from 'next-intl';
import {ArrowLeft, Plus} from 'lucide-react';
import {SeverityChip} from '../status-chips';
import {NeutralChip} from '@/components/common/layout';
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
  ClusterRuleOverride,
  NotificationChannel,
  NotificationRecipientUser,
} from '@/lib/services/monitoring';
import type {ClusterInfo} from '@/lib/services/cluster';
import {cn} from '@/lib/utils';
import {
  formatForDuration,
  formatOverrideDiffSummary,
  formatThresholdValue,
  isEmptyOverrideDraft,
  thresholdUnitLabel,
} from '../monitoring-utils';
import styles from '../monitoring.module.css';
import layout from '@/components/common/layout/list-detail.module.css';
import {DirtyCloseDialog, useBeforeUnload} from '@/components/common/layout';
import {layerBody} from './RulesPanel';

const FOR_OPTIONS_SECONDS = [0, 60, 120, 300, 600, 900, 1800] as const;

interface RuleDetailPanelProps {
  rule: ClusterRule;
  /** 外层指定的集群视角；null = 全局默认视角。Active cluster context from RulesPanel; null = global view. */
  contextClusterId?: number | null;
  /** 复用外层集群列表；独立挂载（如单测）未传时自行拉取。Optional cluster list from parent; fetched when omitted. */
  clusters?: ClusterInfo[];
  onClose: () => void;
  onSaved: (rule: ClusterRule) => void;
  onToggleEnabled: (rule: ClusterRule, on: boolean) => void;
  onConfigureNotify?: () => void;
  onSwitchToGlobal?: () => void;
  /** 草稿相对打开快照是否已修改，供宿主拦截关闭。Whether drafts diverge from the open-time snapshot. */
  onDirtyChange?: (dirty: boolean) => void;
}

/** 全局层的可编辑字段；null = 继承目录默认。Editable global-layer fields; null inherits catalog default. */
interface GlobalDraft {
  threshold: number | null;
  for_seconds: number | null;
  severity: 'warning' | 'critical' | null;
  notify_mode: ClusterRuleNotifyMode | null;
  channel_ids: number[];
  receiver_user_ids: number[];
}

function buildGlobalDraft(rule: ClusterRule): GlobalDraft {
  return {
    threshold: rule.global.threshold,
    for_seconds: rule.global.for_seconds,
    severity: (rule.global.severity as GlobalDraft['severity']) ?? rule.severity,
    notify_mode:
      rule.global.notify_mode === 'none'
        ? 'default'
        : (rule.global.notify_mode ?? 'default'),
    channel_ids: rule.global.channel_ids ?? [],
    receiver_user_ids: rule.global.receiver_user_ids ?? [],
  };
}

function buildOverrideDraft(ov: ClusterRuleOverride | null): ClusterRuleLayerRequest {
  if (!ov) {
    return {
      enabled: null,
      threshold: null,
      for_seconds: null,
      severity: null,
      notify_mode: null,
      channel_ids: null,
      receiver_user_ids: null,
    };
  }
  return {
    enabled: ov.enabled,
    threshold: ov.threshold,
    for_seconds: ov.for_seconds,
    severity: (ov.severity as ClusterRuleLayerRequest['severity']) ?? null,
    notify_mode:
      ov.notify_mode === 'none'
        ? null
        : ((ov.notify_mode as ClusterRuleLayerRequest['notify_mode']) ?? null),
    channel_ids: ov.channel_ids?.length ? ov.channel_ids : null,
    receiver_user_ids: ov.receiver_user_ids?.length
      ? ov.receiver_user_ids
      : null,
  };
}

export function RuleDetailPanel({
  rule,
  contextClusterId = null,
  clusters: propClusters,
  onClose,
  onSaved,
  onToggleEnabled,
  onConfigureNotify,
  onSwitchToGlobal,
  onDirtyChange,
}: RuleDetailPanelProps) {
  const t = useTranslations('monitoring');
  const locale = useLocale();

  const [draft, setDraft] = useState<GlobalDraft>(() => buildGlobalDraft(rule));
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);
  const [error, setError] = useState<string | null>(null);
  // 打开时的全局层快照；保存成功后快照前移，脏判断随之复位。
  // Global-layer snapshot from open time; a successful save advances it so dirty resets.
  const [snapshot, setSnapshot] = useState(() => JSON.stringify(draft));
  const [channels, setChannels] = useState<NotificationChannel[]>([]);
  const [users, setUsers] = useState<NotificationRecipientUser[]>([]);
  const [fetchedClusters, setFetchedClusters] = useState<ClusterInfo[]>([]);
  const [defaults, setDefaults] = useState<AlertNotificationDefaults | null>(null);
  const [addingCluster, setAddingCluster] = useState(false);
  // 全局视角下下钻编辑的集群 ID；选定集群后直接进内存子视图，不再先发空 PUT。
  // Cluster ID being drilled into from global view; enters an in-memory subview without an empty PUT.
  const [subviewClusterId, setSubviewClusterId] = useState<number | null>(null);
  const [overrideDirty, setOverrideDirty] = useState(false);
  const [confirmBackOpen, setConfirmBackOpen] = useState(false);

  const clusters = propClusters ?? fetchedClusters;
  const activeOverrideClusterId = contextClusterId ?? subviewClusterId;

  useEffect(() => {
    Promise.all([
      services.monitoring.listNotificationChannels().catch(() => ({channels: []})),
      services.monitoring.listNotifiableUsers().catch(() => ({users: []})),
      services.monitoring.getAlertNotificationDefaults().catch(() => null),
      propClusters
        ? Promise.resolve(null)
        : // 后端 ListClustersRequest 限制 size<=100，超过会被 binding 拒绝。
          // Backend caps ListClustersRequest size at 100; larger values fail binding.
          services.cluster.getClusters({current: 1, size: 100}).catch(() => null),
    ]).then(([ch, us, def, cl]) => {
      setChannels(ch.channels ?? []);
      setUsers(us.users ?? []);
      setDefaults(def);
      if (cl) {
        setFetchedClusters((cl?.clusters ?? []) as ClusterInfo[]);
      }
    });
  }, [propClusters]);

  const severity = draft.severity ?? rule.severity;
  const notifyMode = draft.notify_mode ?? 'default';
  // critical 生效时必须至少有一个启用的外部渠道；default 模式靠默认通知兜底，由后端校验。
  // Effective critical requires one enabled external channel; default mode relies on defaults.
  const enabledChannels = useMemo(
    () => channels.filter((c) => c.enabled),
    [channels],
  );
  const customMissingChannel =
    severity === 'critical' &&
    notifyMode === 'custom' &&
    draft.channel_ids.filter((id) => enabledChannels.some((c) => c.id === id))
      .length === 0 &&
    draft.receiver_user_ids.length === 0;
  const saveDisabled = saving || customMissingChannel;

  const channelOptions = useMemo(
    () => channels.map((c) => ({value: String(c.id), label: c.name})),
    [channels],
  );
  const userOptions = useMemo(
    () => users.map((u) => ({value: String(u.id), label: u.nickname || u.username})),
    [users],
  );

  const globalDirty = JSON.stringify(draft) !== snapshot;
  const dirty = activeOverrideClusterId != null ? overrideDirty : globalDirty;
  useEffect(() => {
    onDirtyChange?.(dirty);
  }, [dirty, onDirtyChange]);
  useBeforeUnload(dirty);

  // 默认通知摘要：把已配置渠道/接收人拼成一行说明，未配置时给未配置提示。
  // Default-notification summary as one muted line; unset defaults get an unset hint.
  const defaultsTargets = useMemo(() => {
    if (!defaults?.configured) {
      return null;
    }
    const names = [
      ...(defaults.channel_ids ?? []).map(
        (id) => channels.find((c) => c.id === id)?.name,
      ),
      ...(defaults.receiver_user_ids ?? []).map(
        (id) =>
          users.find((u) => u.id === id)?.nickname ??
          users.find((u) => u.id === id)?.username,
      ),
    ].filter(Boolean);
    return names.length ? names.join('、') : null;
  }, [defaults, channels, users]);

  const saveGlobal = async () => {
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
          receiver_user_ids: draft.receiver_user_ids.length
            ? draft.receiver_user_ids
            : null,
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

  const saveOverride = async (
    clusterId: number,
    layer: ClusterRuleLayerRequest,
  ) => {
    const updated = await services.monitoring.updateClusterRuleOverride(
      rule.key,
      clusterId,
      layer,
    );
    onSaved(updated);
  };

  const deleteOverride = async (clusterId: number) => {
    await services.monitoring.deleteClusterRuleOverride(rule.key, clusterId);
    const fresh = await services.monitoring.getClusterRule(rule.key);
    onSaved(fresh);
    if (contextClusterId == null) {
      setSubviewClusterId(null);
      setOverrideDirty(false);
    }
  };

  const requestBackToGlobal = useCallback(() => {
    if (overrideDirty) {
      setConfirmBackOpen(true);
      return;
    }
    setSubviewClusterId(null);
    setOverrideDirty(false);
  }, [overrideDirty]);

  const forOptions = useMemo(
    () =>
      FOR_OPTIONS_SECONDS.filter(
        (sec) => !rule.for_editable || sec > 0 || sec === rule.for_seconds,
      ),
    [rule.for_editable, rule.for_seconds],
  );

  const overriddenClusterIds = new Set(rule.overrides.map((o) => o.cluster_id));
  const addableClusters = clusters.filter(
    (c) => !overriddenClusterIds.has(c.id),
  );

  // 子视图目标集群对象 / Active cluster metadata for the override subview.
  if (activeOverrideClusterId != null) {
    const existingOverride =
      rule.overrides.find((o) => o.cluster_id === activeOverrideClusterId) ??
      null;
    const clusterName =
      existingOverride?.cluster_name ||
      clusters.find((c) => c.id === activeOverrideClusterId)?.name ||
      `#${activeOverrideClusterId}`;
    const effSeverity =
      existingOverride?.effective.severity ?? rule.severity;

    return (
      <div data-testid='rule-detail-panel' className={layout.detailCol}>
        <div className={styles.metaBand}>
          <SeverityChip
            severity={effSeverity}
            label={
              effSeverity === 'critical'
                ? t('severityCritical')
                : t('severityWarning')
            }
          />
          <span className={cn(layout.sub, styles.mono)}>{rule.key}</span>
          <NeutralChip className='ml-auto'>{clusterName}</NeutralChip>
        </div>

        {contextClusterId == null ? (
          <div className={styles.subviewBar}>
            <button
              type='button'
              className={styles.subviewBack}
              onClick={requestBackToGlobal}
            >
              <ArrowLeft className='size-3.5' aria-hidden />
              {t('backToGlobal')}
            </button>
            <span className={layout.xsText}>
              {t('exceptionFor', {cluster: clusterName})}
            </span>
          </div>
        ) : onSwitchToGlobal ? (
          <div className={styles.subviewBar}>
            <span className={cn(layout.xsText, layout.muted)}>
              {t('exceptionFor', {cluster: clusterName})}
            </span>
            <button
              type='button'
              className={styles.subviewBack}
              onClick={onSwitchToGlobal}
            >
              {t('switchToGlobalView')}
            </button>
          </div>
        ) : null}

        <OverrideSubview
          key={`${rule.key}-${activeOverrideClusterId}`}
          rule={rule}
          clusterId={activeOverrideClusterId}
          clusterName={clusterName}
          override={existingOverride}
          channels={channels}
          users={users}
          defaults={defaults}
          defaultsTargets={defaultsTargets}
          forOptions={forOptions}
          onSave={(layer) => saveOverride(activeOverrideClusterId, layer)}
          onDelete={
            existingOverride
              ? () => deleteOverride(activeOverrideClusterId)
              : undefined
          }
          onDirtyChange={setOverrideDirty}
          onConfigureNotify={onConfigureNotify}
        />

        <DirtyCloseDialog
          open={confirmBackOpen}
          onDiscard={() => {
            setConfirmBackOpen(false);
            setSubviewClusterId(null);
            setOverrideDirty(false);
          }}
          onKeepEditing={() => setConfirmBackOpen(false)}
        />
      </div>
    );
  }

  return (
    <div data-testid='rule-detail-panel' className={layout.detailCol}>
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
        <span className={cn(layout.sub, styles.mono)}>{rule.key}</span>
      </div>

      <div className={layout.detailScroll}>
        <p className={styles.descText}>
          {locale === 'en'
            ? rule.description_en || rule.description
            : rule.description}
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
            <div
              className={
                rule.threshold
                  ? styles.fieldRow
                  : cn(styles.fieldRow, 'grid-cols-1 sm:grid-cols-2')
              }
            >
              {rule.threshold ? (
                rule.threshold.value == null && draft.threshold == null ? (
                  <div className={layout.field}>
                    <span className={layout.fieldLabel}>
                      {t('fieldThreshold')}
                    </span>
                    <span className={styles.fieldDesc}>
                      {t('thresholdDeployedHint')}{' '}
                      <button
                        type='button'
                        className={styles.linkBtn}
                        onClick={() =>
                          setDraft((d) => ({
                            ...d,
                            threshold: rule.threshold?.min ?? 1,
                          }))
                        }
                      >
                        {t('thresholdSetFixed')}
                      </button>
                    </span>
                  </div>
                ) : (
                  <div className={layout.field}>
                    <NumberField
                      label={t('fieldThreshold')}
                      value={
                        draft.threshold ?? rule.threshold.value ?? undefined
                      }
                      onValueChange={(v) =>
                        setDraft((d) => ({...d, threshold: v}))
                      }
                      min={rule.threshold.min}
                      max={rule.threshold.max}
                      prefix={rule.threshold.operator}
                      suffix={
                        thresholdUnitLabel(rule.threshold.unit, t) || undefined
                      }
                    />
                    <span className={styles.fieldDesc}>
                      {t('thresholdRange', {
                        min: rule.threshold.min,
                        max: rule.threshold.max,
                      })}
                    </span>
                  </div>
                )
              ) : null}
              <div className={layout.field}>
                <Select
                  label={t('fieldFor')}
                  value={String(draft.for_seconds ?? rule.for_seconds)}
                  onValueChange={(v) =>
                    setDraft((d) => ({...d, for_seconds: Number(v)}))
                  }
                  disabled={!rule.for_editable}
                  options={forOptions.map((sec) => ({
                    value: String(sec),
                    label:
                      sec === 0
                        ? t('realtime')
                        : t('durationMinutes', {minutes: sec / 60}),
                  }))}
                />
              </div>
            </div>
          </div>

          <div className={cn(styles.groupRow, styles.groupRowStack)}>
            <span className={layout.fieldLabel}>{t('fieldSeverity')}</span>
            <div>
              <SegmentedControl
                label={t('fieldSeverity')}
                value={severity}
                onValueChange={(v) =>
                  setDraft((d) => ({
                    ...d,
                    severity: v as GlobalDraft['severity'],
                  }))
                }
                options={[
                  {value: 'warning', label: t('severityWarning')},
                  {value: 'critical', label: t('severityCritical')},
                ]}
              />
            </div>
            <span className={styles.fieldDesc}>
              {severity === 'critical'
                ? t('severityCriticalDesc')
                : t('severityWarningDesc')}
            </span>
          </div>

          {severity === 'critical' ? (
            <>
              <div className={cn(styles.groupRow, styles.groupRowStack)}>
                <span className={layout.fieldLabel}>{t('fieldNotify')}</span>
                <div>
                  <SegmentedControl
                    label={t('fieldNotify')}
                    value={notifyMode}
                    onValueChange={(v) =>
                      setDraft((d) => ({
                        ...d,
                        notify_mode: v as ClusterRuleNotifyMode,
                      }))
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
                  {notifyMode === 'default' &&
                  !defaults?.configured &&
                  onConfigureNotify ? (
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
                    onValueChange={(v) =>
                      setDraft((d) => ({...d, channel_ids: v.map(Number)}))
                    }
                    placeholder={t('notifyChannelsPlaceholder')}
                    emptyMessage={t('selectEmpty')}
                    searchPlaceholder={t('selectSearchPlaceholder')}
                    noMatchMessage={t('comboboxEmpty')}
                  />
                  <MultiSelect
                    label={t('notifyReceivers')}
                    options={userOptions}
                    value={draft.receiver_user_ids.map(String)}
                    onValueChange={(v) =>
                      setDraft((d) => ({
                        ...d,
                        receiver_user_ids: v.map(Number),
                      }))
                    }
                    placeholder={t('notifyReceiversPlaceholder')}
                    emptyMessage={t('selectEmpty')}
                    searchPlaceholder={t('selectSearchPlaceholder')}
                    noMatchMessage={t('comboboxEmpty')}
                  />
                  {customMissingChannel ? (
                    <p className={cn(styles.rowHint, layout.dangerText)}>
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

        {/* 按集群单独配置：紧凑 Diff 摘要行 + 选集群后进聚焦子视图。
            Per-cluster overrides: compact diff summary rows + drill-down subview. */}
        <div className={layout.sectionStack} style={{gap: 8}}>
          <div className={styles.sectionHead}>
            <span className={layout.sectionTitle}>{t('detailExceptions')}</span>
            <Button
              type='button'
              variant='ghost'
              size='sm'
              disabled={addableClusters.length === 0}
              onClick={() => setAddingCluster((open) => !open)}
            >
              <Plus className='size-3.5' aria-hidden />
              {t('detailAddException')}
            </Button>
          </div>
          {addingCluster ? (
            <Combobox
              label={t('colCluster')}
              options={addableClusters.map((c) => ({
                value: String(c.id),
                label: c.name,
              }))}
              onValueChange={(v) => {
                const id = Number(v);
                if (id) {
                  setAddingCluster(false);
                  setOverrideDirty(false);
                  setSubviewClusterId(id);
                }
              }}
              placeholder={t('searchPlaceholder')}
              emptyMessage={t('comboboxEmpty')}
            />
          ) : null}
          {rule.overrides.length > 0 ? (
            <div>
              {rule.overrides.map((ov) => (
                <div
                  key={ov.cluster_id}
                  data-testid={`override-row-${ov.cluster_id}`}
                  className={styles.overrideRow}
                >
                  <div className={styles.overrideMeta}>
                    <span className={styles.overrideTitle}>
                      {ov.cluster_name || `#${ov.cluster_id}`}
                    </span>
                    <span className={styles.overrideDiff}>
                      {formatOverrideDiffSummary(rule, ov, channels, t)}
                    </span>
                  </div>
                  <div className={styles.overrideActions}>
                    <Button
                      type='button'
                      variant='ghost'
                      size='sm'
                      onClick={() => {
                        setAddingCluster(false);
                        setOverrideDirty(false);
                        setSubviewClusterId(ov.cluster_id);
                      }}
                    >
                      {t('overrideEdit')}
                    </Button>
                    <ConfirmMorph
                      label={t('deleteException')}
                      prompt={t('deleteExceptionPrompt', {
                        name: ov.cluster_name || `#${ov.cluster_id}`,
                      })}
                      confirmLabel={t('deleteConfirm')}
                      cancelLabel={t('deleteCancel')}
                      pendingLabel={t('saving')}
                      onConfirm={() => deleteOverride(ov.cluster_id)}
                    />
                  </div>
                </div>
              ))}
            </div>
          ) : (
            <p className={styles.hintBox}>{t('exceptionsEmpty')}</p>
          )}
        </div>
      </div>

      {/* 底栏常驻详情底部：全局视图只显示全局层保存；状态内联在按钮旁。
          Persistent bottom bar: global view shows only global save; status stays inline. */}
      <div className={layout.detailFoot}>
        <Button
          type='button'
          size='sm'
          loading={saving}
          disabled={saveDisabled}
          onClick={() => void saveGlobal()}
        >
          {t('save')}
        </Button>
        {saving ? (
          <span className={cn(layout.xsText, layout.muted)}>{t('saving')}</span>
        ) : null}
        {!saving && saved ? (
          <span className={cn(layout.xsText, layout.muted)}>{t('saved')}</span>
        ) : null}
        {error ? (
          <span
            className={cn(layout.xsText, layout.dangerText, 'min-w-0 truncate')}
            title={error}
          >
            {error}
          </span>
        ) : null}
        <span className={layout.detailFootGrow} />
        <ConfirmMorph
          label={t('restoreDefault')}
          prompt={t('restoreDefaultConfirm')}
          confirmLabel={t('restoreDefault')}
          cancelLabel={t('deleteCancel')}
          pendingLabel={t('saving')}
          tone='neutral'
          onConfirm={restoreDefault}
        />
      </div>
    </div>
  );
}

/**
 * 聚焦编辑单个集群覆盖的子视图：每个字段均支持「沿用全局」与「单独覆盖」显式切换。
 * Focused subview for editing one cluster's override: every field shows explicit
 * inherit-global vs overridden state with a one-click reset.
 */
function OverrideSubview({
  rule,
  clusterName,
  override,
  channels,
  users,
  defaults,
  defaultsTargets,
  forOptions,
  onSave,
  onDelete,
  onDirtyChange,
  onConfigureNotify,
}: {
  rule: ClusterRule;
  clusterId: number;
  clusterName: string;
  override: ClusterRuleOverride | null;
  channels: NotificationChannel[];
  users: NotificationRecipientUser[];
  defaults: AlertNotificationDefaults | null;
  defaultsTargets: string | null;
  forOptions: readonly number[];
  onSave: (layer: ClusterRuleLayerRequest) => Promise<void>;
  onDelete?: () => Promise<void>;
  onDirtyChange?: (dirty: boolean) => void;
  onConfigureNotify?: () => void;
}) {
  const t = useTranslations('monitoring');
  const [draft, setDraft] = useState<ClusterRuleLayerRequest>(() =>
    buildOverrideDraft(override),
  );
  const [snapshot, setSnapshot] = useState(() =>
    JSON.stringify(buildOverrideDraft(override)),
  );
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const dirty = JSON.stringify(draft) !== snapshot;
  useEffect(() => {
    onDirtyChange?.(dirty);
  }, [dirty, onDirtyChange]);

  // 外部列表开关直接更新 override 时，若当前无未保存改动则同步快照。
  // Sync external override updates (e.g. inline table switch) when the form is clean.
  const externalKey = JSON.stringify(buildOverrideDraft(override));
  useEffect(() => {
    if (!dirty && externalKey !== snapshot) {
      const next = buildOverrideDraft(override);
      setDraft(next);
      setSnapshot(JSON.stringify(next));
    }
  }, [dirty, externalKey, override, snapshot]);

  const enabledChannels = useMemo(
    () => channels.filter((c) => c.enabled),
    [channels],
  );
  const effSeverity = draft.severity ?? rule.severity;
  const globalNotifyMode =
    rule.notify_mode === 'none' ? 'default' : rule.notify_mode;
  const effNotify = draft.notify_mode ?? globalNotifyMode;
  const missingChannel =
    effSeverity === 'critical' &&
    draft.notify_mode === 'custom' &&
    (draft.channel_ids ?? []).filter((id) =>
      enabledChannels.some((c) => c.id === id),
    ).length === 0 &&
    (draft.receiver_user_ids ?? []).length === 0;

  const isAllInherited = isEmptyOverrideDraft(draft);
  const saveBlocked =
    saving || missingChannel || (!override && isAllInherited);

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

  const inheritedChannelsText = useMemo(() => {
    const names = [
      ...(rule.channel_ids ?? []).map(
        (id) => channels.find((c) => c.id === id)?.name,
      ),
      ...(rule.receiver_user_ids ?? []).map(
        (id) =>
          users.find((u) => u.id === id)?.nickname ??
          users.find((u) => u.id === id)?.username,
      ),
    ].filter(Boolean);
    return names.length ? names.join('、') : t('unset');
  }, [rule.channel_ids, rule.receiver_user_ids, channels, users, t]);

  const globalNotifyLabel =
    globalNotifyMode === 'custom' ? t('notifyCustom') : t('notifyDefault');

  return (
    <>
      <div className={layout.detailScroll}>
        <p className={styles.descText}>{t('clusterOverrideSubtitle')}</p>

        <div className={styles.groupInner}>
          {/* 启用开关：清晰标示当前是沿用全局还是已单独覆盖，支持一键恢复沿用。 */}
          <div className={styles.groupRow}>
            <div className={styles.rowLabel}>
              <span className={styles.rowLabelText}>{t('fieldEnabled')}</span>
              <span className={styles.rowHint}>
                {draft.enabled != null ? (
                  <>
                    {t('overriddenFieldHint')} ·{' '}
                    <button
                      type='button'
                      className={styles.linkBtn}
                      onClick={() => setDraft((d) => ({...d, enabled: null}))}
                    >
                      {t('restoreInherit')}
                    </button>
                  </>
                ) : (
                  t('inheritingGlobalValue', {
                    value: rule.enabled
                      ? t('diffEnabledOn')
                      : t('diffEnabledOff'),
                  })
                )}
              </span>
            </div>
            <Switch
              checked={draft.enabled ?? override?.effective.enabled ?? rule.enabled}
              onCheckedChange={(on) => setDraft((d) => ({...d, enabled: on}))}
              aria-label={t('fieldEnabled')}
            />
          </div>

          {/* 阈值与持续时间 */}
          <div className={cn(styles.groupRow, styles.groupRowStack)}>
            <div
              className={
                rule.threshold
                  ? styles.fieldRow
                  : cn(styles.fieldRow, 'grid-cols-1 sm:grid-cols-2')
              }
            >
              {rule.threshold ? (
                draft.threshold != null ? (
                  <div className={layout.field}>
                    <NumberField
                      label={t('fieldThreshold')}
                      value={draft.threshold}
                      onValueChange={(v) =>
                        setDraft((d) => ({...d, threshold: v}))
                      }
                      min={rule.threshold.min}
                      max={rule.threshold.max}
                      prefix={rule.threshold.operator}
                      suffix={
                        thresholdUnitLabel(rule.threshold.unit, t) || undefined
                      }
                    />
                    <span className={styles.fieldDesc}>
                      {t('thresholdRange', {
                        min: rule.threshold.min,
                        max: rule.threshold.max,
                      })}{' '}
                      ·{' '}
                      <button
                        type='button'
                        className={styles.linkBtn}
                        onClick={() =>
                          setDraft((d) => ({...d, threshold: null}))
                        }
                      >
                        {t('restoreInherit')}
                      </button>
                    </span>
                  </div>
                ) : (
                  <div className={layout.field}>
                    <div className={layout.fieldHeadRow}>
                      <span className={layout.fieldLabel}>
                        {t('fieldThreshold')}
                      </span>
                      <button
                        type='button'
                        className={styles.linkBtn}
                        onClick={() =>
                          setDraft((d) => ({
                            ...d,
                            // 生效阈值 null（=部署数）时用 min 兜底，否则空值进不了数字编辑态
                            // fall back to min when effective threshold is null (= deployed count)
                            threshold:
                              override?.effective.threshold ??
                              rule.threshold?.value ??
                              rule.threshold?.min ??
                              1,
                          }))
                        }
                      >
                        {t('overrideField')}
                      </button>
                    </div>
                    <span className={styles.fieldDesc}>
                      {t('inheritingGlobalValue', {
                        value: formatThresholdValue(rule.threshold, t),
                      })}
                    </span>
                  </div>
                )
              ) : null}

              <div className={layout.field}>
                <Select
                  label={t('fieldFor')}
                  value={
                    draft.for_seconds != null
                      ? String(draft.for_seconds)
                      : 'inherit'
                  }
                  onValueChange={(v) =>
                    setDraft((d) => ({
                      ...d,
                      for_seconds: v === 'inherit' ? null : Number(v),
                    }))
                  }
                  disabled={!rule.for_editable}
                  options={[
                    {
                      value: 'inherit',
                      label: t('inheritingGlobalValue', {
                        value: formatForDuration(rule.for_seconds, t),
                      }),
                    },
                    ...forOptions.map((sec) => ({
                      value: String(sec),
                      label:
                        sec === 0
                          ? t('realtime')
                          : t('durationMinutes', {minutes: sec / 60}),
                    })),
                  ]}
                />
              </div>
            </div>
          </div>

          <div className={cn(styles.groupRow, styles.groupRowStack)}>
            <Select
              label={t('fieldSeverity')}
              value={draft.severity ?? 'inherit'}
              onValueChange={(v) =>
                setDraft((d) => ({
                  ...d,
                  severity:
                    v === 'inherit' ? null : (v as 'warning' | 'critical'),
                }))
              }
              options={[
                {
                  value: 'inherit',
                  label: t('inheritingGlobalValue', {
                    value:
                      rule.severity === 'critical'
                        ? t('severityCritical')
                        : t('severityWarning'),
                  }),
                },
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
                    setDraft((d) => ({
                      ...d,
                      notify_mode:
                        v === 'inherit' ? null : (v as ClusterRuleNotifyMode),
                    }))
                  }
                  options={[
                    {
                      value: 'inherit',
                      label: t('inheritingGlobalValue', {
                        value: globalNotifyLabel,
                      }),
                    },
                    {value: 'default', label: t('notifyDefault')},
                    {value: 'custom', label: t('notifyCustom')},
                  ]}
                />
                {effNotify === 'default' ? (
                  <span className={styles.fieldDesc}>
                    {defaultsTargets
                      ? t('notifyDefaultSummary', {targets: defaultsTargets})
                      : defaults?.configured
                        ? t('notifyDefaultDesc')
                        : t('notifyDefaultUnset')}
                    {!defaults?.configured && onConfigureNotify ? (
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
                ) : draft.notify_mode == null ? (
                  <span className={styles.fieldDesc}>
                    {t('globalChannelsSummary', {
                      targets: inheritedChannelsText,
                    })}
                  </span>
                ) : null}
              </div>

              {draft.notify_mode === 'custom' ? (
                <div className={cn(styles.groupRow, styles.groupRowStack)}>
                  <MultiSelect
                    label={t('notifyChannels')}
                    options={channels.map((c) => ({
                      value: String(c.id),
                      label: c.name,
                    }))}
                    value={(draft.channel_ids ?? []).map(String)}
                    onValueChange={(v) =>
                      setDraft((d) => ({...d, channel_ids: v.map(Number)}))
                    }
                    placeholder={t('notifyChannelsPlaceholder')}
                    emptyMessage={t('selectEmpty')}
                    searchPlaceholder={t('selectSearchPlaceholder')}
                    noMatchMessage={t('comboboxEmpty')}
                  />
                  <MultiSelect
                    label={t('notifyReceivers')}
                    options={users.map((u) => ({
                      value: String(u.id),
                      label: u.nickname || u.username,
                    }))}
                    value={(draft.receiver_user_ids ?? []).map(String)}
                    onValueChange={(v) =>
                      setDraft((d) => ({
                        ...d,
                        receiver_user_ids: v.map(Number),
                      }))
                    }
                    placeholder={t('notifyReceiversPlaceholder')}
                    emptyMessage={t('selectEmpty')}
                    searchPlaceholder={t('selectSearchPlaceholder')}
                    noMatchMessage={t('comboboxEmpty')}
                  />
                  {missingChannel ? (
                    <p className={cn(styles.rowHint, layout.dangerText)}>
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
      </div>

      <div className={layout.detailFoot}>
        <Button
          type='button'
          size='sm'
          loading={saving}
          disabled={saveBlocked}
          onClick={() => void submit()}
        >
          {t('save')}
        </Button>
        {saving ? (
          <span className={cn(layout.xsText, layout.muted)}>{t('saving')}</span>
        ) : null}
        {!saving && saved ? (
          <span className={cn(layout.xsText, layout.muted)}>{t('saved')}</span>
        ) : null}
        {!saving && !saved && !override && isAllInherited ? (
          <span className={cn(layout.xsText, layout.muted)}>
            {t('overrideNoChangesHint')}
          </span>
        ) : null}
        {err ? (
          <span
            className={cn(layout.xsText, layout.dangerText, 'min-w-0 truncate')}
            title={err}
          >
            {err}
          </span>
        ) : null}
        <span className={layout.detailFootGrow} />
        {onDelete ? (
          <ConfirmMorph
            label={t('deleteException')}
            prompt={t('deleteExceptionPrompt', {name: clusterName})}
            confirmLabel={t('deleteConfirm')}
            cancelLabel={t('deleteCancel')}
            pendingLabel={t('saving')}
            onConfirm={onDelete}
          />
        ) : null}
      </div>
    </>
  );
}
