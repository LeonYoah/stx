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

import type {SyncPluginType} from '@/lib/services/sync/types';
import {formatMetadataValue} from './sync-studio-utils';

export type OptionRuleAssistOption = {
  key: string;
  required_mode?: string;
  condition_expression?: string;
  description?: string;
  advanced?: boolean;
};

export type OptionRuleAssistSchema = {
  options: OptionRuleAssistOption[];
  value_constraints?: Record<string, unknown>[];
  condition_rules?: Record<string, unknown>[];
};

export type PluginBlockAssignment = {
  key: string;
  rawValue: string;
  /** 去掉引号后的字面量；含 {{ }} 时为 null（跳过值校验）。 Literal without quotes; null when templated. */
  literal: string | null;
  lineNumber: number;
  startColumn: number;
  endColumn: number;
};

export type PluginBlockSnapshot = {
  pluginType: SyncPluginType;
  factoryIdentifier: string;
  startLine: number;
  endLine: number;
  assignments: PluginBlockAssignment[];
  values: Record<string, string | null>;
};

export type OptionRuleDiagnostic = {
  lineNumber: number;
  startColumn: number;
  endColumn: number;
  message: string;
  severity: 'error' | 'warning';
  optionKey: string;
};

export type OptionKeyCompletionRank = {
  key: string;
  description?: string;
  /** 越小越靠前。Lower sorts first. */
  sortBucket: number;
  active: boolean;
};

const TEMPLATE_VALUE_RE = /\{\{[^}]+\}\}/;
const ASSIGNMENT_RE = /^(\s*)([A-Za-z0-9_.-]+)\s*=\s*(.*?)\s*$/;

/**
 * 扫描全文，抽出每个 source/transform/sink 工厂块及其简单赋值。
 * Scan content for plugin factory blocks and simple key=value assignments.
 */
export function extractPluginBlockSnapshots(
  content: string,
): PluginBlockSnapshot[] {
  const lines = content.split('\n');
  const snapshots: PluginBlockSnapshot[] = [];
  let pluginType: SyncPluginType | null = null;
  let factoryIdentifier: string | null = null;
  let startLine = 0;
  let depth = 0;
  let inFactory = false;
  const assignments: PluginBlockAssignment[] = [];

  const flush = (endLine: number) => {
    if (!pluginType || !factoryIdentifier || !inFactory) {
      return;
    }
    const values: Record<string, string | null> = {};
    for (const item of assignments) {
      values[item.key] = item.literal;
    }
    snapshots.push({
      pluginType,
      factoryIdentifier,
      startLine,
      endLine,
      assignments: [...assignments],
      values,
    });
  };

  for (let i = 0; i < lines.length; i += 1) {
    const lineNumber = i + 1;
    const rawLine = lines[i];
    const line = rawLine.replace(/#.*$/, '').trim();
    if (!line) {
      continue;
    }
    const opens = (line.match(/\{/g) || []).length;
    const closes = (line.match(/\}/g) || []).length;

    const typeMatch = line.match(/^(source|transform|sink|catalog)\s*\{$/i);
    if (typeMatch && depth === 0) {
      pluginType = typeMatch[1].toLowerCase() as SyncPluginType;
      factoryIdentifier = null;
      inFactory = false;
      assignments.length = 0;
      depth = 1;
      continue;
    }

    if (pluginType && !inFactory && depth === 1 && opens > 0 && closes === 0) {
      const pluginMatch = line.match(/^([A-Za-z0-9_.-]+)\s*\{$/);
      if (pluginMatch) {
        factoryIdentifier = pluginMatch[1];
        startLine = lineNumber;
        inFactory = true;
        assignments.length = 0;
        depth += opens - closes;
        continue;
      }
    }

    if (inFactory && depth === 2) {
      const assignment = parseAssignmentLine(rawLine, lineNumber);
      if (assignment) {
        assignments.push(assignment);
      }
    }

    if (depth > 0) {
      depth += opens - closes;
      if (depth < 0) {
        depth = 0;
      }
      if (inFactory && depth <= 1) {
        flush(lineNumber);
        inFactory = false;
        factoryIdentifier = null;
        assignments.length = 0;
      }
      if (depth === 0) {
        pluginType = null;
      }
    }
  }

  if (inFactory) {
    flush(lines.length);
  }
  return snapshots;
}

export function parseAssignmentLine(
  rawLine: string,
  lineNumber: number,
): PluginBlockAssignment | null {
  const withoutComment = rawLine.replace(/(^|[^:])#.*$/, '$1');
  const match = withoutComment.match(ASSIGNMENT_RE);
  if (!match) {
    return null;
  }
  // 跳过嵌套对象/数组赋值（本行以 { [ 开头）。
  // Skip nested object/array assignments starting with { or [.
  const rawValue = (match[3] || '').trim();
  // 跳过嵌套对象/数组；保留 {{var}} 模板字面量。
  // Skip nested object/array; keep {{var}} template literals.
  if (
    !rawValue ||
    ((rawValue.startsWith('{') || rawValue.startsWith('[')) &&
      !rawValue.startsWith('{{'))
  ) {
    return null;
  }
  const key = match[2];
  const keyIndex = rawLine.indexOf(key);
  const equalsIndex = rawLine.indexOf('=', keyIndex);
  const startColumn = equalsIndex + 2;
  const endColumn = Math.max(startColumn + 1, withoutComment.replace(/\s+$/, '').length + 1);
  return {
    key,
    rawValue,
    literal: normalizeLiteral(rawValue),
    lineNumber,
    startColumn,
    endColumn,
  };
}

function normalizeLiteral(rawValue: string): string | null {
  if (TEMPLATE_VALUE_RE.test(rawValue)) {
    return null;
  }
  const trimmed = rawValue.trim();
  if (
    (trimmed.startsWith('"') && trimmed.endsWith('"')) ||
    (trimmed.startsWith("'") && trimmed.endsWith("'"))
  ) {
    return trimmed.slice(1, -1);
  }
  return trimmed;
}

/**
 * 评估单条 Condition / Expression 节点（与 proxy 序列化形状对齐）。
 * Evaluate one Condition / Expression node (matches proxy JSON shape).
 */
export function evaluateConditionNode(
  node: unknown,
  values: Record<string, string | null>,
  depth = 0,
): boolean {
  if (!node || typeof node !== 'object' || depth > 32) {
    return false;
  }
  const item = node as Record<string, unknown>;
  // Expression wrapper: { condition, and, next }
  if (item.condition && typeof item.condition === 'object') {
    const head = evaluateConditionNode(item.condition, values, depth + 1);
    if (!item.next) {
      return head;
    }
    const next = evaluateConditionNode(item.next, values, depth + 1);
    if (item.and === false) {
      return head || next;
    }
    return head && next;
  }

  const optionKey = String(item.optionKey || item.option_key || '');
  if (!optionKey) {
    return false;
  }
  const actual = values[optionKey];
  // 缺字段或模板变量：条件视为未命中（保守，避免误收敛）。
  // Missing/templated values: treat as unmatched (conservative).
  if (actual == null) {
    return false;
  }
  const operator = String(item.operator || 'EQUAL').toUpperCase();
  const expectValue = item.expectValue ?? item.expect_value;
  const compareKey = String(
    item.compareOptionKey || item.compare_option_key || '',
  );
  const compareActual = compareKey ? values[compareKey] : null;
  const head = matchOperator(operator, actual, expectValue, compareActual);
  if (!item.next) {
    return head;
  }
  const next = evaluateConditionNode(item.next, values, depth + 1);
  if (item.and === false) {
    return head || next;
  }
  return head && next;
}

function matchOperator(
  operator: string,
  actual: string,
  expectValue: unknown,
  compareActual: string | null,
): boolean {
  const expect = expectValue == null ? '' : String(expectValue);
  const actualNum = Number(actual);
  const expectNum = Number(expect);
  const compareNum =
    compareActual == null || compareActual === ''
      ? Number.NaN
      : Number(compareActual);

  switch (operator) {
    case 'EQUAL':
      return actual === expect || actual.toUpperCase() === expect.toUpperCase();
    case 'NOT_EQUAL':
      return actual !== expect;
    case 'GREATER_THAN':
      return !Number.isNaN(actualNum) && !Number.isNaN(expectNum) && actualNum > expectNum;
    case 'GREATER_OR_EQUAL':
      return !Number.isNaN(actualNum) && !Number.isNaN(expectNum) && actualNum >= expectNum;
    case 'LESS_THAN':
      return !Number.isNaN(actualNum) && !Number.isNaN(expectNum) && actualNum < expectNum;
    case 'LESS_OR_EQUAL':
      return !Number.isNaN(actualNum) && !Number.isNaN(expectNum) && actualNum <= expectNum;
    case 'FIELD_GREATER_THAN':
      return !Number.isNaN(actualNum) && !Number.isNaN(compareNum) && actualNum > compareNum;
    case 'FIELD_GREATER_OR_EQUAL':
      return !Number.isNaN(actualNum) && !Number.isNaN(compareNum) && actualNum >= compareNum;
    case 'FIELD_LESS_THAN':
      return !Number.isNaN(actualNum) && !Number.isNaN(compareNum) && actualNum < compareNum;
    case 'FIELD_LESS_OR_EQUAL':
      return !Number.isNaN(actualNum) && !Number.isNaN(compareNum) && actualNum <= compareNum;
    case 'NOT_BLANK':
      return actual.trim().length > 0;
    case 'NOT_EMPTY':
    case 'MAP_NOT_EMPTY':
      return actual.length > 0;
    case 'STARTS_WITH':
      return actual.startsWith(expect);
    case 'CONTAINS':
      return actual.includes(expect);
    case 'MATCHES':
      try {
        return new RegExp(expect).test(actual);
      } catch {
        return false;
      }
    case 'EXTENSION':
      // 自定义扩展无法在前端执行。Cannot execute server-only extensions locally.
      return true;
    default:
      return false;
  }
}

function conditionRuleMatched(
  rule: Record<string, unknown>,
  values: Record<string, string | null>,
): boolean {
  const tree = rule.expressionTree || rule.expression_tree;
  if (tree) {
    return evaluateConditionNode(tree, values);
  }
  const expression = String(rule.expression || '');
  return evaluateConditionExpressionString(expression, values);
}

/**
 * 解析引擎 toString 风格条件：`'mode' == TIMESTAMP` / `"x" == "y"`。
 * Parse engine toString-style conditions like `'mode' == TIMESTAMP`.
 */
export function evaluateConditionExpressionString(
  expression: string,
  values: Record<string, string | null>,
): boolean {
  const text = expression.trim();
  if (!text) {
    return false;
  }
  const match = text.match(
    /^['"]?([A-Za-z0-9_.-]+)['"]?\s*(==|!=)\s*['"]?(.+?)['"]?\s*$/,
  );
  if (!match) {
    return false;
  }
  const key = match[1];
  const op = match[2];
  const expect = match[3].replace(/^['"]|['"]$/g, '');
  const actual = values[key];
  if (actual == null) {
    return false;
  }
  if (op === '==') {
    return actual === expect || actual.toUpperCase() === expect.toUpperCase();
  }
  return actual !== expect;
}

function collectConditionRuleOptionKeys(rule: Record<string, unknown>): string[] {
  const optionRule = (rule.optionRule || rule.option_rule || {}) as Record<
    string,
    unknown
  >;
  const keys = optionRule.optionKeys || optionRule.option_keys;
  if (Array.isArray(keys)) {
    return keys.map((item) => String(item)).filter(Boolean);
  }
  return [];
}

/**
 * 根据当前赋值与 conditionRules，给出补全排序：命中子规则的 key 优先，仅存在于未命中子规则的 key 降权。
 * Rank option keys: prefer keys from matched conditionRules; demote keys only in unmatched rules.
 */
export function rankOptionKeyCompletions(
  schema: OptionRuleAssistSchema,
  values: Record<string, string | null>,
): OptionKeyCompletionRank[] {
  const baseKeys = (schema.options || [])
    .map((item) => item.key)
    .filter(Boolean);
  const descriptions = new Map(
    (schema.options || []).map((item) => [item.key, item.description || '']),
  );
  const rules = Array.isArray(schema.condition_rules)
    ? schema.condition_rules
    : [];

  const matchedKeys = new Set<string>();
  const unmatchedOnlyKeys = new Set<string>();
  let anyMatched = false;

  for (const rule of rules) {
    if (!rule || typeof rule !== 'object') {
      continue;
    }
    const keys = collectConditionRuleOptionKeys(rule as Record<string, unknown>);
    const matched = conditionRuleMatched(
      rule as Record<string, unknown>,
      values,
    );
    if (matched) {
      anyMatched = true;
      for (const key of keys) {
        matchedKeys.add(key);
      }
    } else {
      for (const key of keys) {
        unmatchedOnlyKeys.add(key);
      }
    }
  }

  // 命中规则的 key 从「仅未命中」集合移除。
  // Remove matched keys from unmatched-only set.
  for (const key of matchedKeys) {
    unmatchedOnlyKeys.delete(key);
  }

  // CONDITIONAL 扁平字段：条件命中则 boost。
  // Boost flat CONDITIONAL options when their expression matches.
  const conditionalBoost = new Set<string>();
  for (const option of schema.options || []) {
    if (
      String(option.required_mode || '').toUpperCase() === 'CONDITIONAL' &&
      option.condition_expression &&
      evaluateConditionExpressionString(option.condition_expression, values)
    ) {
      conditionalBoost.add(option.key);
    }
  }

  const allKeys = new Set<string>([...baseKeys, ...matchedKeys]);
  const ranks: OptionKeyCompletionRank[] = [];
  for (const key of allKeys) {
    let sortBucket = 2;
    let active = true;
    if (matchedKeys.has(key) || conditionalBoost.has(key)) {
      sortBucket = 0;
      active = true;
    } else if (anyMatched && unmatchedOnlyKeys.has(key)) {
      sortBucket = 5;
      active = false;
    } else if (!anyMatched && unmatchedOnlyKeys.has(key)) {
      // 尚无模式命中时，子规则专有字段仍展示但靠后。
      // Before a mode matches, keep nested-only keys but sort later.
      sortBucket = 4;
      active = false;
    }
    ranks.push({
      key,
      description: descriptions.get(key),
      sortBucket,
      active,
    });
  }

  return ranks.sort((left, right) => {
    if (left.sortBucket !== right.sortBucket) {
      return left.sortBucket - right.sortBucket;
    }
    return left.key.localeCompare(right.key);
  });
}

/**
 * 基于 valueConstraints + 条件必填，生成行内诊断。
 * Build inline diagnostics from valueConstraints and conditional required options.
 */
export function buildOptionRuleDiagnostics(
  schema: OptionRuleAssistSchema,
  snapshot: PluginBlockSnapshot,
): OptionRuleDiagnostic[] {
  const diagnostics: OptionRuleDiagnostic[] = [];
  const values = snapshot.values;
  const assignmentByKey = new Map(
    snapshot.assignments.map((item) => [item.key, item]),
  );

  for (const root of schema.value_constraints || []) {
    collectValueConstraintDiagnostics(
      root,
      values,
      assignmentByKey,
      diagnostics,
    );
  }

  // 嵌套 conditionRules 内的值约束（仅条件命中时）。
  // Nested value constraints under matched conditionRules only.
  for (const rule of schema.condition_rules || []) {
    if (!rule || typeof rule !== 'object') {
      continue;
    }
    const record = rule as Record<string, unknown>;
    if (!conditionRuleMatched(record, values)) {
      continue;
    }
    const optionRule = (record.optionRule ||
      record.option_rule ||
      {}) as Record<string, unknown>;
    const nestedConstraints = optionRule.valueConstraints ||
      optionRule.value_constraints;
    if (Array.isArray(nestedConstraints)) {
      for (const root of nestedConstraints) {
        collectValueConstraintDiagnostics(
          root,
          values,
          assignmentByKey,
          diagnostics,
        );
      }
    }
    for (const key of collectConditionRuleOptionKeys(record)) {
      // 子规则 optionKeys 在此仅作补全；必填由 ConfigValidator 权威校验。
      // Nested required presence is authoritative in ConfigValidator; skip hard missing here.
      void key;
    }
  }

  for (const option of schema.options || []) {
    const mode = String(option.required_mode || '').toUpperCase();
    if (mode !== 'CONDITIONAL' || !option.condition_expression) {
      continue;
    }
    if (
      !evaluateConditionExpressionString(option.condition_expression, values)
    ) {
      continue;
    }
    if (values[option.key] != null && String(values[option.key]).length > 0) {
      continue;
    }
    diagnostics.push({
      lineNumber: snapshot.startLine,
      startColumn: 1,
      endColumn: 2,
      message: `条件已满足（${option.condition_expression}），建议配置 \`${option.key}\``,
      severity: 'warning',
      optionKey: option.key,
    });
  }

  return diagnostics;
}

function collectValueConstraintDiagnostics(
  node: unknown,
  values: Record<string, string | null>,
  assignmentByKey: Map<string, PluginBlockAssignment>,
  diagnostics: OptionRuleDiagnostic[],
  depth = 0,
): void {
  if (!node || typeof node !== 'object' || depth > 32) {
    return;
  }
  // 逐节检查 and 链上每个条件，避免整链失败时误报首个运算符。
  // Check each node on an AND chain so failures point at the real operator.
  let current: unknown = node;
  let currentDepth = depth;
  while (current && typeof current === 'object' && currentDepth <= 32) {
    const item = current as Record<string, unknown>;
    const optionKey = String(item.optionKey || item.option_key || '');
    if (optionKey) {
      const actual = values[optionKey];
      if (actual != null) {
        const operator = String(item.operator || 'EQUAL').toUpperCase();
        const expectValue = item.expectValue ?? item.expect_value;
        const compareKey = String(
          item.compareOptionKey || item.compare_option_key || '',
        );
        const compareActual = compareKey ? values[compareKey] : null;
        const ok = matchOperator(operator, actual, expectValue, compareActual);
        if (!ok) {
          const assignment = assignmentByKey.get(optionKey);
          const hints = describeFailedConstraint(item);
          if (assignment && hints) {
            const message = `\`${optionKey}\` ${hints}`;
            if (
              !diagnostics.some(
                (itemDiag) =>
                  itemDiag.lineNumber === assignment.lineNumber &&
                  itemDiag.message === message,
              )
            ) {
              diagnostics.push({
                lineNumber: assignment.lineNumber,
                startColumn: assignment.startColumn,
                endColumn: assignment.endColumn,
                message,
                severity: 'error',
                optionKey,
              });
            }
          }
        }
      }
    }
    // OR 链：任一分枝失败都不应单独标红整条；仅展开 and!==false 的续链。
    // For OR chains, do not mark each branch; only walk AND continuations.
    if (item.and === false) {
      break;
    }
    current = item.next;
    currentDepth += 1;
  }
}

function describeFailedConstraint(item: Record<string, unknown>): string {
  const operator = String(item.operator || '').toUpperCase();
  const expectValue = item.expectValue ?? item.expect_value;
  const compareKey = String(
    item.compareOptionKey || item.compare_option_key || '',
  );
  switch (operator) {
    case 'GREATER_THAN':
      return compareKey
        ? `应 > ${compareKey}`
        : `应 > ${formatMetadataValue(expectValue)}`;
    case 'GREATER_OR_EQUAL':
      return compareKey
        ? `应 ≥ ${compareKey}`
        : `应 ≥ ${formatMetadataValue(expectValue)}`;
    case 'LESS_THAN':
    case 'FIELD_LESS_THAN':
      return compareKey
        ? `应 < ${compareKey}`
        : `应 < ${formatMetadataValue(expectValue)}`;
    case 'LESS_OR_EQUAL':
    case 'FIELD_LESS_OR_EQUAL':
      return compareKey
        ? `应 ≤ ${compareKey}`
        : `应 ≤ ${formatMetadataValue(expectValue)}`;
    case 'FIELD_GREATER_THAN':
      return compareKey ? `应 > ${compareKey}` : '值不满足约束';
    case 'FIELD_GREATER_OR_EQUAL':
      return compareKey ? `应 ≥ ${compareKey}` : '值不满足约束';
    case 'EQUAL':
      return `应为 ${formatMetadataValue(expectValue)}`;
    case 'NOT_EQUAL':
      return `不应为 ${formatMetadataValue(expectValue)}`;
    case 'NOT_BLANK':
      return '不能为空白';
    case 'NOT_EMPTY':
    case 'MAP_NOT_EMPTY':
      return '不能为空';
    case 'STARTS_WITH':
      return `应以 ${formatMetadataValue(expectValue)} 开头`;
    case 'CONTAINS':
      return `应包含 ${formatMetadataValue(expectValue)}`;
    case 'MATCHES':
      return `应匹配 ${formatMetadataValue(expectValue)}`;
    case 'EXTENSION':
      return String(
        item.extensionDescription ||
          item.extension_description ||
          '不满足自定义约束',
      );
    default:
      return '不满足值约束';
  }
}

/**
 * 判断光标是否在插件工厂块内、且处于「写 option key」区域（非赋值右侧）。
 * Whether cursor is inside a factory block writing an option key (not value side).
 */
export function resolveOptionKeyCompletionContext(
  content: string,
  lineNumber: number,
  column: number,
): {
  inKeyRegion: boolean;
  prefix: string;
  snapshot: PluginBlockSnapshot | null;
} {
  const snapshots = extractPluginBlockSnapshots(content);
  const snapshot =
    snapshots.find(
      (item) => lineNumber >= item.startLine && lineNumber <= item.endLine,
    ) || null;
  if (!snapshot) {
    return {inKeyRegion: false, prefix: '', snapshot: null};
  }
  const line =
    content.split('\n')[lineNumber - 1]?.replace(/#.*$/, '') || '';
  const equalsIndex = line.indexOf('=');
  if (equalsIndex >= 0 && column > equalsIndex + 1) {
    return {inKeyRegion: false, prefix: '', snapshot};
  }
  const before = line.slice(0, Math.max(0, column - 1));
  const prefixMatch = before.match(/(?:^|\s)([A-Za-z0-9_.-]*)$/);
  const prefix = prefixMatch?.[1] || '';
  // 工厂块首行 `FakeSource {` 不提供 key 补全。
  // Skip key completion on the factory opening line.
  if (lineNumber === snapshot.startLine) {
    return {inKeyRegion: false, prefix, snapshot};
  }
  return {inKeyRegion: true, prefix, snapshot};
}

/**
 * 从 Studio schema 缓存（含 __schema_meta__）还原辅助结构。
 * Rebuild assist schema from Studio cache including __schema_meta__.
 */
export function toOptionRuleAssistSchema(
  cached: Record<string, any> | null | undefined,
): OptionRuleAssistSchema {
  if (!cached) {
    return {options: []};
  }
  const meta = cached.__schema_meta__ || {};
  const options: OptionRuleAssistOption[] = [];
  for (const [key, value] of Object.entries(cached)) {
    if (key === '__schema_meta__' || !value || typeof value !== 'object') {
      continue;
    }
    const item = value as Record<string, unknown>;
    options.push({
      key: String(item.key || key),
      required_mode: item.required_mode
        ? String(item.required_mode)
        : undefined,
      condition_expression: item.condition_expression
        ? String(item.condition_expression)
        : undefined,
      description: item.description ? String(item.description) : undefined,
      advanced: Boolean(item.advanced),
    });
  }
  return {
    options,
    value_constraints: Array.isArray(meta.value_constraints)
      ? meta.value_constraints
      : [],
    condition_rules: Array.isArray(meta.condition_rules)
      ? meta.condition_rules
      : [],
  };
}
