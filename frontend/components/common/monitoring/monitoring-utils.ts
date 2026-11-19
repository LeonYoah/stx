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
import type {AlertInstance, AlertScope, AlertSeverity} from '@/lib/services/monitoring';

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

/** 阈值摘要：worker_insufficient 之类 null 阈值显示默认语义说明。 Threshold summary text for rule surfaces. */
export function formatRuleThreshold(
  rule: {
    threshold: {operator: string; value: number | null; unit: string} | null;
    for_seconds: number;
    key?: string;
  },
  t: MonitoringTranslator,
): string {
  const forText =
    rule.for_seconds === 0
      ? t('realtime')
      : t('forMinutes', {minutes: rule.for_seconds / 60});
  if (rule.threshold) {
    const {operator, value, unit} = rule.threshold;
    // value 为 null 表示"等于部署数"动态语义，没有可展示的数字
    // a null value is the dynamic "equals deployed count" semantic, with no number to render
    if (value == null) {
      return `${t('thresholdDeployed')} · ${forText}`;
    }
    const unitLabel = thresholdUnitLabel(unit, t);
    // % 与空单位紧贴数字，其余单位与数字之间留空格
    // '%' and empty units stay attached; other units get a separating space
    const sep = unitLabel === '%' || unitLabel === '' ? '' : ' ';
    return unitLabel
      ? `${operator} ${value}${sep}${unitLabel} · ${forText}`
      : `${operator} ${value} · ${forText}`;
  }
  return forText;
}

/** 严重度的 badge 语义。Severity → badge tone. */
export function severityTone(severity: AlertSeverity | string): 'danger' | 'warning' {
  return severity === 'critical' ? 'danger' : 'warning';
}
