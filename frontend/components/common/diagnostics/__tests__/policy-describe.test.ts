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

import {describe, expect, it} from 'vitest';
import zh from '@/lib/i18n/locales/zh.json';
import {describePolicySentence} from '../policies/policy-describe';
import type {
  InspectionAutoPolicy,
  InspectionConditionTemplate,
} from '@/lib/services/diagnostics';
import type {AlertRuleCatalogItem} from '@/lib/services/monitoring/types';

// 直接用 zh 文案做断言翻译源，保证句子结构与 i18n 模板一致。
// Assert against the zh locale directly so the sentence structure stays tied
// to the i18n templates.
const dict = zh.diagnosticsCenter.autoPolicies;
const t = (key: string, values?: Record<string, string | number>) => {
  const [head, ...rest] = key.split('.');
  let node: unknown = (dict as Record<string, unknown>)[head];
  for (const part of rest) {
    node = (node as Record<string, unknown>)?.[part];
  }
  let text = String(node);
  for (const [k, v] of Object.entries(values ?? {})) {
    text = text.replace(`{${k}}`, String(v));
  }
  return text;
};

const templates: InspectionConditionTemplate[] = [
  {
    code: 'SCHEDULED',
    name: '定时巡检',
    description: '',
    default_cron_expr: '0 3 * * *',
  } as InspectionConditionTemplate,
  {
    code: 'ALERT_FIRING',
    name: '告警触发',
    description: '',
  } as InspectionConditionTemplate,
];

const ruleCatalog: AlertRuleCatalogItem[] = [
  {rule_key: 'jvm_heap', name: 'JVM 堆内存过高'} as AlertRuleCatalogItem,
];

const clusterOptions = [
  {cluster_id: 1, cluster_name: 'stx-prod'},
  {cluster_id: 2, cluster_name: 'stx-dev'},
];

const deps = {templates, ruleCatalog, clusterOptions, t};

function policy(
  overrides: Partial<InspectionAutoPolicy>,
): InspectionAutoPolicy {
  return {
    id: 1,
    name: 'p',
    cluster_id: 0,
    enabled: true,
    cooldown_minutes: 30,
    auto_create_task: false,
    auto_start_task: false,
    conditions: [],
    invalid_reason: '',
    created_at: '',
    updated_at: '',
    ...overrides,
  } as InspectionAutoPolicy;
}

describe('describePolicySentence', () => {
  it('renders a scheduled condition with its cron expression', () => {
    const text = describePolicySentence(
      policy({
        conditions: [
          {template_code: 'SCHEDULED', enabled: true, rule_keys: []},
        ],
      }),
      deps,
    );
    expect(text).toBe('当 定时 0 3 * * * 时，自动巡检 全局');
  });

  it('prefers the condition cron override over the template default', () => {
    const text = describePolicySentence(
      policy({
        conditions: [
          {
            template_code: 'SCHEDULED',
            enabled: true,
            rule_keys: [],
            cron_expr_override: '*/10 * * * *',
          },
        ],
      }),
      deps,
    );
    expect(text).toBe('当 定时 */10 * * * * 时，自动巡检 全局');
  });

  it('translates alert-firing rule keys into catalog names joined by 或', () => {
    const text = describePolicySentence(
      policy({
        conditions: [
          {
            template_code: 'ALERT_FIRING',
            enabled: true,
            rule_keys: ['jvm_heap', 'unknown_rule'],
          },
        ],
      }),
      deps,
    );
    expect(text).toBe('当 告警 JVM 堆内存过高 或 unknown_rule 触发 时，自动巡检 全局');
  });

  it('joins multiple enabled conditions with 或 and appends the bundle suffix', () => {
    const text = describePolicySentence(
      policy({
        cluster_id: 1,
        auto_create_task: true,
        conditions: [
          {template_code: 'SCHEDULED', enabled: true, rule_keys: []},
          {
            template_code: 'ALERT_FIRING',
            enabled: true,
            rule_keys: ['jvm_heap'],
          },
        ],
      }),
      deps,
    );
    expect(text).toBe(
      '当 定时 0 3 * * * 或 告警 JVM 堆内存过高 触发 时，自动巡检 stx-prod，并生成诊断包',
    );
  });

  it('skips disabled conditions and reports no enabled condition', () => {
    const text = describePolicySentence(
      policy({
        conditions: [
          {template_code: 'SCHEDULED', enabled: false, rule_keys: []},
        ],
      }),
      deps,
    );
    expect(text).toBe('当 未启用条件 时，自动巡检 全局');
  });

  it('falls back to raw keys for retired templates and missing clusters', () => {
    const text = describePolicySentence(
      policy({
        cluster_id: 99,
        conditions: [
          {template_code: 'RETIRED_CODE', enabled: true, rule_keys: []},
        ],
      }),
      deps,
    );
    expect(text).toBe('当 RETIRED_CODE 时，自动巡检 集群 #99');
  });
});
