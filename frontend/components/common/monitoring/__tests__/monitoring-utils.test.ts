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

// 阈值摘要的单位标签与 null 阈值语义：percent→%、count 无单位、workers 本地化、null→等于部署数。
// Threshold-summary unit labels and the null-threshold semantic: percent→%,
// count has no unit, workers is localized, null means "equals deployed count".

import {describe, expect, it} from 'vitest';
import type {ClusterRule} from '@/lib/services/monitoring';
import {
  formatForDuration,
  formatOverrideDiffSummary,
  formatRuleThreshold,
  isEmptyOverrideDraft,
  resolveRuleForCluster,
  thresholdUnitLabel,
  type MonitoringTranslator,
} from '../monitoring-utils';

const zh = ((key: string, params?: Record<string, unknown>) =>
  ({
    realtime: '实时',
    forMinutes: `持续 ${params?.minutes ?? 2} 分钟`,
    unitWorkers: '台',
    thresholdDeployed: '等于部署数',
    diffEnabledOn: '开启',
    diffEnabledOff: '关闭',
    severityCritical: '严重',
    severityWarning: '警告',
    diffNotifyDefault: '默认通知',
    diffNotifyCustom: '自定义通知',
    notifyNone: '仅告警中心',
    overrideAllInheritedDiff: '与全局一致',
  })[key] ?? key) as MonitoringTranslator;

describe('thresholdUnitLabel', () => {
  it('maps backend unit strings to display labels', () => {
    expect(thresholdUnitLabel('percent', zh)).toBe('%');
    expect(thresholdUnitLabel('count', zh)).toBe('');
    expect(thresholdUnitLabel('workers', zh)).toBe('台');
    expect(thresholdUnitLabel('other_unit', zh)).toBe('other_unit');
  });
});

describe('formatRuleThreshold', () => {
  it('percent shows the number with % attached', () => {
    expect(
      formatRuleThreshold(
        {threshold: {operator: '>', value: 85, unit: 'percent'}, for_seconds: 120},
        zh,
      ),
    ).toBe('> 85% · 持续 2 分钟');
  });

  it('count shows only the number', () => {
    expect(
      formatRuleThreshold(
        {threshold: {operator: '>', value: 0, unit: 'count'}, for_seconds: 120},
        zh,
      ),
    ).toBe('> 0 · 持续 2 分钟');
  });

  it('workers uses the localized unit with a space', () => {
    expect(
      formatRuleThreshold(
        {threshold: {operator: '<', value: 3, unit: 'workers'}, for_seconds: 120},
        zh,
      ),
    ).toBe('< 3 台 · 持续 2 分钟');
  });

  it('null threshold value renders the deployed-count semantic', () => {
    expect(
      formatRuleThreshold(
        {threshold: {operator: '<', value: null, unit: 'workers'}, for_seconds: 120},
        zh,
      ),
    ).toBe('等于部署数 · 持续 2 分钟');
  });

  it('no threshold falls back to the duration alone', () => {
    expect(formatRuleThreshold({threshold: null, for_seconds: 120}, zh)).toBe('持续 2 分钟');
    expect(formatRuleThreshold({threshold: null, for_seconds: 0}, zh)).toBe('实时');
  });
});

const sampleRule: ClusterRule = {
  key: 'cpu_usage_high',
  name: 'CPU 高',
  name_en: 'High CPU',
  description: 'desc',
  scope: 'cluster',
  default_enabled: true,
  enabled: true,
  severity: 'warning',
  threshold: {operator: '>', value: 85, default: 85, unit: 'percent', min: 1, max: 100},
  for_seconds: 600,
  default_for_seconds: 600,
  for_editable: true,
  notify_mode: 'default',
  channel_ids: [],
  receiver_user_ids: [],
  has_global_config: false,
  global: {
    enabled: null,
    threshold: null,
    for_seconds: null,
    severity: null,
    notify_mode: null,
    channel_ids: [],
    receiver_user_ids: [],
  },
  overrides: [
    {
      cluster_id: 6,
      cluster_name: 'prod-st-1',
      enabled: false,
      threshold: 75,
      for_seconds: 300,
      severity: 'critical',
      notify_mode: 'custom',
      channel_ids: [1],
      receiver_user_ids: [],
      effective: {
        enabled: false,
        severity: 'critical',
        threshold: 75,
        for_seconds: 300,
        external_channel_missing: false,
      },
    },
  ],
  override_count: 1,
  external_channel_missing: false,
};

describe('resolveRuleForCluster', () => {
  it('returns global effective fields when clusterId is null', () => {
    const view = resolveRuleForCluster(sampleRule, null);
    expect(view.isClusterView).toBe(false);
    expect(view.isOverridden).toBe(true);
    expect(view.enabled).toBe(true);
    expect(view.severity).toBe('warning');
    expect(view.threshold?.value).toBe(85);
  });

  it('returns cluster override effective fields when matching cluster exists', () => {
    const view = resolveRuleForCluster(sampleRule, 6);
    expect(view.isClusterView).toBe(true);
    expect(view.isOverridden).toBe(true);
    expect(view.enabled).toBe(false);
    expect(view.severity).toBe('critical');
    expect(view.threshold?.value).toBe(75);
    expect(view.forSeconds).toBe(300);
  });

  it('falls back to global and marks non-overridden when cluster has no override', () => {
    const view = resolveRuleForCluster(sampleRule, 99);
    expect(view.isClusterView).toBe(true);
    expect(view.isOverridden).toBe(false);
    expect(view.enabled).toBe(true);
    expect(view.threshold?.value).toBe(85);
  });
});

describe('formatOverrideDiffSummary & formatForDuration', () => {
  it('formats duration only without threshold text', () => {
    expect(formatForDuration(300, zh)).toBe('持续 5 分钟');
    expect(formatForDuration(0, zh)).toBe('实时');
  });

  it('summarizes only overridden fields including channel names', () => {
    const summary = formatOverrideDiffSummary(
      sampleRule,
      sampleRule.overrides[0],
      [{id: 1, name: '值班飞书'}],
      zh,
    );
    expect(summary).toBe('关闭 · > 75% · 持续 5 分钟 · 严重 · 自定义通知 (值班飞书)');
  });

  it('detects empty all-inherit drafts', () => {
    expect(
      isEmptyOverrideDraft({
        enabled: null,
        threshold: null,
        for_seconds: null,
        severity: null,
        notify_mode: null,
      }),
    ).toBe(true);
    expect(isEmptyOverrideDraft({enabled: false})).toBe(false);
  });
});
