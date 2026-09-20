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
import {
  applyConditionTextOverride,
  normalizeAutoPolicyTaskOptions,
  normalizeConditionItemsForSave,
  shouldRenderCronExprInput,
} from '../AutoPolicyConfigPanel';

describe('AutoPolicyConfigPanel helpers', () => {
  it('renders cron input for scheduled templates with a default cron expression', () => {
    expect(
      shouldRenderCronExprInput({
        code: 'SCHEDULED',
        category: 'schedule',
        name: 'Scheduled Inspection',
        description: 'Trigger inspection on schedule',
        default_threshold: 0,
        default_window_minutes: 0,
        default_cron_expr: '0 0 * * *',
        immediate_on_match: false,
      }),
    ).toBe(true);

    expect(
      shouldRenderCronExprInput({
        code: 'JAVA_OOM',
        category: 'java_error',
        name: 'Java OOM',
        description: 'Trigger on OOM',
        default_threshold: 0,
        default_window_minutes: 0,
        default_cron_expr: '',
        immediate_on_match: true,
      }),
    ).toBe(false);
  });

  it('preserves spaces while editing cron override on the matching condition only', () => {
    const nextConditions = applyConditionTextOverride(
      [
        {template_code: 'SCHEDULED', enabled: true},
        {template_code: 'JAVA_OOM', enabled: true},
      ],
      'SCHEDULED',
      'cron_expr_override',
      ' */15 * * * * ',
    );

    expect(nextConditions).toEqual([
      {
        template_code: 'SCHEDULED',
        enabled: true,
        cron_expr_override: ' */15 * * * * ',
      },
      {template_code: 'JAVA_OOM', enabled: true},
    ]);
  });

  it('trims cron override only when preparing data for save', () => {
    expect(
      normalizeConditionItemsForSave([
        {
          template_code: 'SCHEDULED',
          enabled: true,
          cron_expr_override: ' */15 * * * * ',
        },
        {template_code: 'JAVA_OOM', enabled: true, cron_expr_override: '   '},
      ]),
    ).toEqual([
      {
        template_code: 'SCHEDULED',
        enabled: true,
        cron_expr_override: '*/15 * * * *',
      },
      {template_code: 'JAVA_OOM', enabled: true},
    ]);
  });

  it('preserves legacy dump switches when deriving selected resources', () => {
    expect(
      normalizeAutoPolicyTaskOptions({
        include_thread_dump: false,
        include_jvm_dump: true,
        jvm_dump_min_free_mb: 4096,
      }).selected_resources,
    ).toEqual([
      'error_context',
      'process_events',
      'alert_snapshot',
      'config_snapshot',
      'log_sample',
      'jvm_dump',
    ]);
  });
});
