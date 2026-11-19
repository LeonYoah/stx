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

/**
 * Shared helpers for the monitoring workspace panels.
 * 监控工作台各面板共用的展示辅助函数。
 */
import type {useTranslations} from 'next-intl';
import type {
  AlertInstance,
  AlertScope,
  AlertSeverity,
  ClusterRule,
  ClusterRuleLayerRequest,
  ClusterRuleOverride,
  ClusterRuleThreshold,
  NotificationChannel,
} from '@/lib/services/monitoring';

export type MonitoringTranslator = ReturnType<typeof useTranslations<'monitoring'>>;

/** 展示状态：firing 未认领 / acknowledged / silenced / resolved / closed（closed 独立展示，不并入 resolved）。 / Display status; closed shows its own label, not folded into resolved. */
export type AlertRowStatus = 'firing' | 'acknowledged' | 'silenced' | 'resolved' | 'closed';

export function alertRowStatus(a: AlertInstance): AlertRowStatus {
  if (a.status === 'closed') {
    return 'closed';
  }
  if (a.status !== 'firing') {
    return 'resolved';
  }
  if (a.handling_status === 'silenced' || a.silenced_until) {
    const until = a.silenced_until ? Date.parse(a.silenced_until) : NaN;
    if (Number.isNaN(until) || until > Date.now()) {
      return 'silenced';
    }
  }
  if (a.handling_status === 'acknowledged' || a.acknowledged_at) {
    return 'acknowledged';
  }
  return 'firing';
}

export function alertScopeOf(a: AlertInstance): AlertScope {
  return a.scope === 'job' || a.source_type === 'job' ? 'job' : 'cluster';
}

export function isCritical(a: Pick<AlertInstance, 'severity'>): boolean {
  return a.severity === 'critical';
}

/** 相对时间的分钟数，喂给原型文案 ago*。Minutes ago for the proto-style relative labels. */
export function minutesAgo(iso?: string | null): number {
  if (!iso) {
    return 0;
  }
  const ts = Date.parse(iso);
  if (Number.isNaN(ts)) {
    return 0;
  }
  return Math.max(0, Math.round((Date.now() - ts) / 60000));
}

export function formatAgo(minutes: number, t: MonitoringTranslator): string {
  if (minutes < 60) {
    return t('agoMinutes', {value: minutes});
  }
  if (minutes < 1440) {
    return t('agoHours', {value: Math.round(minutes / 60)});
  }
  return t('agoDays', {value: Math.round(minutes / 1440)});
}

/** 阈值单位的展示标签：percent→%、count 无单位、workers 本地化。Display label for a threshold unit. */
export function thresholdUnitLabel(unit: string, t: MonitoringTranslator): string {
  switch (unit) {
    case 'percent':
      return '%';
    case 'count':
      return '';
    case 'workers':
      return t('unitWorkers');
    default:
      return unit;
  }
}

/** 持续时间展示文案：0 秒为实时，其余按分钟。 / Duration label: 0s is realtime, otherwise minutes. */
export function formatForDuration(forSeconds: number, t: MonitoringTranslator): string {
  return forSeconds === 0
    ? t('realtime')
    : t('forMinutes', {minutes: forSeconds / 60});
}

/** 仅阈值部分展示（不含持续时间），供摘要与继承提示共用。 / Threshold value text only, shared by summaries and inherit hints. */
export function formatThresholdValue(
  threshold: Pick<ClusterRuleThreshold, 'operator' | 'value' | 'unit'>,
  t: MonitoringTranslator,
): string {
  const {operator, value, unit} = threshold;
  // value 为 null 表示"等于部署数"动态语义，没有可展示的数字
  // a null value is the dynamic "equals deployed count" semantic, with no number to render
  if (value == null) {
    return t('thresholdDeployed');
  }
  const unitLabel = thresholdUnitLabel(unit, t);
  // % 与空单位紧贴数字，其余单位与数字之间留空格
  // '%' and empty units stay attached; other units get a separating space
  const sep = unitLabel === '%' || unitLabel === '' ? '' : ' ';
  return unitLabel
    ? `${operator} ${value}${sep}${unitLabel}`
    : `${operator} ${value}`;
}

/** 阈值摘要：worker_insufficient 之类 null 阈值显示默认语义说明。 Threshold summary text for rule surfaces. */
export function formatRuleThreshold(
  rule: {
    threshold: {operator: string; value: number | null; unit: string} | null;
    for_seconds: number;
    key?: string;
  },
  t: MonitoringTranslator,
): string {
  const forText = formatForDuration(rule.for_seconds, t);
  if (rule.threshold) {
    return `${formatThresholdValue(rule.threshold, t)} · ${forText}`;
  }
  return forText;
}

/**
 * 解析规则在指定视角（全局或单集群）下的生效展示视图。
 * Resolve a rule's effective display fields for either the global baseline or a specific cluster.
 */
export interface ResolvedClusterRuleView {
  isClusterView: boolean;
  override: ClusterRuleOverride | null;
  isOverridden: boolean;
  enabled: boolean;
  severity: AlertSeverity;
  threshold: ClusterRuleThreshold | null;
  forSeconds: number;
  externalChannelMissing: boolean;
}

export function resolveRuleForCluster(
  rule: ClusterRule,
  clusterId: number | null,
): ResolvedClusterRuleView {
  if (clusterId == null) {
    return {
      isClusterView: false,
      override: null,
      isOverridden: rule.override_count > 0,
      enabled: rule.enabled,
      severity: rule.severity,
      threshold: rule.threshold,
      forSeconds: rule.for_seconds,
      externalChannelMissing: rule.external_channel_missing,
    };
  }
  const override = rule.overrides.find((o) => o.cluster_id === clusterId) ?? null;
  if (!override) {
    return {
      isClusterView: true,
      override: null,
      isOverridden: false,
      enabled: rule.enabled,
      severity: rule.severity,
      threshold: rule.threshold,
      forSeconds: rule.for_seconds,
      externalChannelMissing: rule.external_channel_missing,
    };
  }
  const eff = override.effective;
  return {
    isClusterView: true,
    override,
    isOverridden: true,
    enabled: eff.enabled,
    severity: eff.severity,
    threshold: rule.threshold
      ? {...rule.threshold, value: eff.threshold}
      : null,
    forSeconds: eff.for_seconds,
    externalChannelMissing: eff.external_channel_missing,
  };
}

/**
 * 拼装单集群覆盖的紧凑差异摘要，仅列出相对全局已修改的字段。
 * Format a compact one-line diff summary showing only fields overridden relative to global.
 */
export function formatOverrideDiffSummary(
  rule: Pick<ClusterRule, 'threshold'>,
  override: ClusterRuleOverride,
  channels: Pick<NotificationChannel, 'id' | 'name'>[],
  t: MonitoringTranslator,
): string {
  const parts: string[] = [];
  if (override.enabled != null) {
    parts.push(override.enabled ? t('diffEnabledOn') : t('diffEnabledOff'));
  }
  if (override.threshold != null && rule.threshold) {
    parts.push(
      formatThresholdValue(
        {
          operator: rule.threshold.operator,
          value: override.threshold,
          unit: rule.threshold.unit,
        },
        t,
      ),
    );
  }
  if (override.for_seconds != null) {
    parts.push(formatForDuration(override.for_seconds, t));
  }
  if (override.severity != null) {
    parts.push(
      override.severity === 'critical'
        ? t('severityCritical')
        : t('severityWarning'),
    );
  }
  if (override.notify_mode != null) {
    if (override.notify_mode === 'custom') {
      const names = (override.channel_ids ?? [])
        .map((id) => channels.find((c) => c.id === id)?.name)
        .filter((n): n is string => Boolean(n));
      parts.push(
        names.length ? `${t('diffNotifyCustom')} (${names.join('、')})` : t('diffNotifyCustom'),
      );
    } else if (override.notify_mode === 'default') {
      parts.push(t('diffNotifyDefault'));
    } else {
      parts.push(t('notifyNone'));
    }
  }
  return parts.length > 0 ? parts.join(' · ') : t('overrideAllInheritedDiff');
}

/** 检查集群覆盖草稿是否所有字段都处于继承态（无任何有效覆盖）。 / Whether every field in an override draft inherits global. */
export function isEmptyOverrideDraft(draft: ClusterRuleLayerRequest): boolean {
  return (
    draft.enabled == null &&
    draft.threshold == null &&
    draft.for_seconds == null &&
    draft.severity == null &&
    draft.notify_mode == null
  );
}

/** 严重度的 badge 语义。Severity → badge tone. */
export function severityTone(severity: AlertSeverity | string): 'danger' | 'warning' {
  return severity === 'critical' ? 'danger' : 'warning';
}
