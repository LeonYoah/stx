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

export type AuditLabelGroup = 'actions' | 'resourceTypes' | 'clientTypes';

export interface AuditTranslator {
  (key: string): string;
  has: (key: string) => boolean;
}

/**
 * 动态审计值只有在文案存在时才调用翻译函数，避免 next-intl 报缺键错误。
 * Translate dynamic audit values only when the message exists to avoid next-intl missing-key errors.
 */
export function lookupAuditLabel(
  group: AuditLabelGroup,
  value: string,
  fallback: string,
  t: AuditTranslator,
): string {
  const key = `audit.${group}.${value.replace(/\./g, '_')}`;
  if (!t.has(key)) {
    return fallback;
  }

  try {
    const translated = t(key);
    return translated && translated !== key ? translated : fallback;
  } catch {
    return fallback;
  }
}
