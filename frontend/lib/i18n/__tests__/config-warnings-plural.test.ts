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

import {createTranslator} from 'next-intl';
import {describe, expect, it} from 'vitest';
import en from '../locales/en.json';
import zh from '../locales/zh.json';

describe('cluster.list.configWarnings ICU plural', () => {
  it('selects singular and plural English forms', () => {
    const t = createTranslator({locale: 'en', messages: en});
    expect(t('cluster.list.configWarnings', {count: 1})).toBe(
      '1 cluster failed to load config; other fields are still shown.',
    );
    expect(t('cluster.list.configWarnings', {count: 3})).toBe(
      '3 clusters failed to load config; other fields are still shown.',
    );
  });

  it('renders the Chinese other form for any count', () => {
    const t = createTranslator({locale: 'zh', messages: zh});
    expect(t('cluster.list.configWarnings', {count: 2})).toBe(
      '2 个集群的配置读取失败，列表已降级展示其余字段。',
    );
  });
});
