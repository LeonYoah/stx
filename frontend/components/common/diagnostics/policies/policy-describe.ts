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
 * 自动巡检策略的一句话描述：把启用条件翻译成「当 <条件> 时，自动巡检 <范围>」，
 * 多条件用本地化的「或 / or」连接；ALERT_FIRING 展示告警规则目录名，目录外
 * 规则键回退原始键值；auto_create_task 追加「并生成诊断包」后缀。冷却时间
 * 不进列表句（只在详情表单里展示）。
 * One-line description for an auto-inspection policy: enabled conditions are
 * rendered as "When <condition>, auto-inspect <scope>" joined by a localized
 * "或 / or"; ALERT_FIRING shows alert-rule catalog names with the raw key as
 * the out-of-catalog fallback; auto_create_task appends a "and generate a
 * diagnostic bundle" suffix. Cooldown stays out of the list sentence (the
 * detail form owns it).
 */
import type {
  DiagnosticsClusterOption,
  InspectionAutoPolicy,
  InspectionConditionItem,
  InspectionConditionTemplate,
} from '@/lib/services/diagnostics';
import type {AlertRuleCatalogItem} from '@/lib/services/monitoring/types';

/** describePolicySentence 所需的翻译与目录数据。Everything the sentence builder needs. */
export interface PolicyDescribeDeps {
  templates: InspectionConditionTemplate[];
  ruleCatalog: AlertRuleCatalogItem[];
  clusterOptions: DiagnosticsClusterOption[];
  /** diagnosticsCenter.autoPolicies 命名空间下的 t()。t() bound to diagnosticsCenter.autoPolicies. */
  t: (key: string, values?: Record<string, string | number>) => string;
}

const SCHEDULED_CODE = 'SCHEDULED';
const ALERT_FIRING_CODE = 'ALERT_FIRING';

function conditionText(
  condition: InspectionConditionItem,
  deps: PolicyDescribeDeps,
): string {
  const {templates, ruleCatalog, t} = deps;
  const template = templates.find(
    (tpl) => tpl.code === condition.template_code,
  );

  if (condition.template_code === SCHEDULED_CODE) {
    const expr =
      condition.cron_expr_override?.trim() ||
      template?.default_cron_expr?.trim() ||
      '';
    return t('describe.cron', {expr});
  }

  if (condition.template_code === ALERT_FIRING_CODE) {
    const names = (condition.rule_keys || []).map(
      (key) =>
        ruleCatalog.find((item) => item.rule_key === key)?.name ?? key,
    );
    if (names.length > 0) {
      return t('describe.alertFiring', {
        rules: names.join(` ${t('describe.or')} `),
      });
    }
    // 未选规则的 ALERT_FIRING 已在 invalidBadge 单独提示，句子退回模板名。
    // An ALERT_FIRING without rule keys is already flagged by the invalid
    // badge; the sentence falls back to the template name.
    return template?.name ?? condition.template_code;
  }

  // 已不在模板目录的旧条件码（失效策略）回退原始码，行内另有失效徽标解释。
  // Retired condition codes (invalid policies) fall back to the raw code;
  // the invalid badge explains it on the same row.
  return template?.name ?? condition.template_code;
}

/**
 * 生成列表用的一句话描述：当 <条件或/或> 时，自动巡检 <范围>[，并生成诊断包]。
 * Build the list sentence: When <conditions or-joined>, auto-inspect <scope>[, and generate a diagnostic bundle].
 */
export function describePolicySentence(
  policy: Pick<
    InspectionAutoPolicy,
    'cluster_id' | 'conditions' | 'auto_create_task'
  >,
  deps: PolicyDescribeDeps,
): string {
  const {clusterOptions, t} = deps;
  const enabled = (policy.conditions || []).filter(
    (condition) => condition.enabled,
  );
  const conditions = enabled.length
    ? enabled
        .map((condition) => conditionText(condition, deps))
        .join(` ${t('describe.or')} `)
    : t('describe.noCondition');
  const scope =
    policy.cluster_id === 0
      ? t('scopeGlobal')
      : (clusterOptions.find((c) => c.cluster_id === policy.cluster_id)
          ?.cluster_name ?? t('scopeCluster', {id: policy.cluster_id}));
  const sentence = t('describe.sentence', {condition: conditions, scope});
  return policy.auto_create_task
    ? `${sentence}${t('describe.bundleSuffix')}`
    : sentence;
}
