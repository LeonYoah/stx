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
 * Shared threshold meta text for rule surfaces (card meta, list column).
 * 规则卡片 / 列表共用的阈值摘要文案。单位空格规则：% 紧贴数字，其他单位前加空格，空单位不加。
 */
import type {useTranslations} from 'next-intl';
import type {ProtoRule} from './rules-mock';

type Translator = ReturnType<typeof useTranslations<'monitoringProto'>>;

export function formatThreshold(rule: ProtoRule, t: Translator): string {
  if (rule.threshold) {
    const {operator, value, unit} = rule.threshold;
    // % 是符号紧贴数字，其余单位与数字之间留空格
    // '%' stays attached; other units get a separating space
    const sep = unit === '%' || unit === '' ? '' : ' ';
    return `${operator} ${value}${sep}${unit} · ${t('forMinutes', {minutes: rule.forMinutes})}`;
  }
  if (rule.engine === 'event') {
    return t('instantEvent');
  }
  return rule.forMinutes === 0 ? t('realtime') : t('forMinutes', {minutes: rule.forMinutes});
}
