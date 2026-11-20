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

import type {
  InspectionConditionItem,
  InspectionConditionTemplate,
  DiagnosticsResourceCode,
  DiagnosticsTaskOptions,
} from '@/lib/services/diagnostics';

// 后端返回的失效原因码白名单：命中才走对应 i18n 文案，未知码回落到 unknown，防止未来新增
// 原因码时 t() 命中缺失的翻译键。
// Whitelist of backend invalid-reason codes: known codes map to their i18n message while any
// future code falls back to "unknown" instead of hitting a missing translation key.
export const INVALID_REASON_CODES = new Set([
  'legacy_condition_code',
  'alert_firing_without_rule_keys',
  'unknown_alert_rule_key',
]);

export function shouldRenderCronExprInput(
  template: InspectionConditionTemplate,
): boolean {
  return Boolean(template.default_cron_expr?.trim());
}

export function applyConditionTextOverride(
  conditions: InspectionConditionItem[],
  templateCode: string,
  field: 'cron_expr_override',
  value: string,
): InspectionConditionItem[] {
  return conditions.map((condition) =>
    condition.template_code === templateCode
      ? {...condition, [field]: value}
      : condition,
  );
}

export function normalizeConditionItemsForSave(
  conditions: InspectionConditionItem[],
): InspectionConditionItem[] {
  return conditions.map((condition) => {
    const cronExprOverride = condition.cron_expr_override?.trim();
    const ruleKeys = (condition.rule_keys || [])
      .map((item) => item.trim())
      .filter(Boolean);

    const result: InspectionConditionItem = {
      template_code: condition.template_code,
      enabled: condition.enabled,
    };

    if (condition.threshold_override !== undefined) {
      result.threshold_override = condition.threshold_override;
    }
    if (condition.window_minutes_override !== undefined) {
      result.window_minutes_override = condition.window_minutes_override;
    }
    if (cronExprOverride) {
      result.cron_expr_override = cronExprOverride;
    }
    // ALERT_FIRING 输出 rule_keys，error_rate 继续输出 extra_keywords，两者不混用。
    // ALERT_FIRING serializes rule_keys while error_rate keeps extra_keywords; never mix them.
    if (condition.template_code === 'ALERT_FIRING') {
      result.rule_keys = ruleKeys;
    } else {
      const extraKeywords = (condition.extra_keywords || [])
        .map((item) => item.trim())
        .filter(Boolean);
      if (extraKeywords.length > 0) {
        result.extra_keywords = extraKeywords;
      }
    }

    return result;
  });
}

/**
 * 将旧策略的两个 Dump 开关转换为完整资源列表，避免编辑后改变原有行为。
 * Convert legacy dump switches into a complete resource list without changing saved behavior.
 */
export function normalizeAutoPolicyTaskOptions(
  options?: DiagnosticsTaskOptions | null,
): DiagnosticsTaskOptions {
  if (!options) {
    return {
      include_thread_dump: false,
      include_jvm_dump: false,
      jvm_dump_min_free_mb: 2048,
      selected_resources: [],
    };
  }
  if (options.selected_resources?.length) {
    return {...options, selected_resources: [...options.selected_resources]};
  }
  const legacyResources: DiagnosticsResourceCode[] = [
    'error_context',
    'process_events',
    'alert_snapshot',
    'config_snapshot',
    'log_sample',
  ];
  if (options.include_thread_dump) {
    legacyResources.push('thread_dump');
  }
  if (options.include_jvm_dump) {
    legacyResources.push('jvm_dump');
  }
  return {...options, selected_resources: legacyResources};
}
