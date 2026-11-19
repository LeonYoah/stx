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
import {formatRuleThreshold, thresholdUnitLabel, type MonitoringTranslator} from '../monitoring-utils';

const zh = ((key: string) =>
  ({
    realtime: '实时',
    forMinutes: '持续 2 分钟',
    unitWorkers: '台',
    thresholdDeployed: '等于部署数',
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
