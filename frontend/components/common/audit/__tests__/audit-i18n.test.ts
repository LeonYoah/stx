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

import {describe, expect, it, vi} from 'vitest';
import {AuditTranslator, lookupAuditLabel} from '../audit-i18n';

function createTranslator(messages: Record<string, string>): AuditTranslator {
  const translate = vi.fn((key: string) => {
    if (!(key in messages)) {
      throw new Error(`missing message: ${key}`);
    }
    return messages[key];
  });
  return Object.assign(translate, {
    has: vi.fn((key: string) => key in messages),
  });
}

describe('lookupAuditLabel', () => {
  it('存在文案时返回翻译值', () => {
    const t = createTranslator({'audit.actions.create': '创建'});

    expect(lookupAuditLabel('actions', 'create', 'create', t)).toBe('创建');
    expect(t).toHaveBeenCalledWith('audit.actions.create');
  });

  it('未知动态值直接回退，不调用缺失的翻译键', () => {
    const t = createTranslator({});

    expect(
      lookupAuditLabel(
        'actions',
        'diagnostics.inspection.create',
        'diagnostics.inspection.create',
        t,
      ),
    ).toBe('diagnostics.inspection.create');
    expect(t).not.toHaveBeenCalled();
    expect(t.has).toHaveBeenCalledWith(
      'audit.actions.diagnostics_inspection_create',
    );
  });
});
