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
import type {NotificationChannel} from '@/lib/services/monitoring';
import type {SyncTaskAlertConfig} from '@/lib/services/sync';
import {
  TASK_ALERT_RULES,
  buildTaskAlertRequest,
  buildWorkbenchDeepLinkQuery,
  initialTaskAlertForm,
  parseJobDeepLink,
  parseTaskAlertCooldown,
  summarizeTaskAlert,
  validateTaskAlertForm,
} from '../task-alert-utils';

function channel(id: number, type: NotificationChannel['type'], recipients: string[] = []): NotificationChannel {
  return {
    id,
    name: `ch-${id}`,
    type,
    enabled: true,
    endpoint: '',
    secret: '',
    config: type === 'email' ? ({email: {recipients}} as NotificationChannel['config']) : null,
    description: '',
    created_at: '',
    updated_at: '',
  };
}

const configured: SyncTaskAlertConfig = {
  configured: true,
  policy_id: 3,
  enabled: false,
  rules: ['sync_stop_timeout', 'sync_job_failed'],
  channel_ids: [2],
  receiver_user_ids: [7],
  notify_task_owner: true,
  cooldown_minutes: 30,
  send_recovery: false,
};

describe('task alert form', () => {
  it('defaults to every rule for a new config and keeps an existing one', () => {
    const fresh = initialTaskAlertForm(null);
    expect(fresh.rules).toEqual(TASK_ALERT_RULES);
    expect(fresh.cooldown).toBe('60');
    expect(fresh.enabled && fresh.sendRecovery).toBe(true);

    const existing = initialTaskAlertForm(configured);
    expect(existing.rules).toEqual(['sync_job_failed', 'sync_stop_timeout']);
    expect(existing).toMatchObject({enabled: false, cooldown: '30', notifyTaskOwner: true, sendRecovery: false});
  });

  it('parses the cooldown as an integer between 0 and 1440', () => {
    expect(parseTaskAlertCooldown('0')).toBe(0);
    expect(parseTaskAlertCooldown(' 1440 ')).toBe(1440);
    expect(parseTaskAlertCooldown('1441')).toBeNull();
    expect(parseTaskAlertCooldown('1.5')).toBeNull();
    expect(parseTaskAlertCooldown('')).toBeNull();
  });

  it('validates rules, channels, cooldown and email recipients', () => {
    const channels = [channel(1, 'webhook'), channel(2, 'email'), channel(3, 'email', ['ops@example.com'])];
    const base = {...initialTaskAlertForm(null), channelIds: [1]};
    expect(validateTaskAlertForm(base, channels)).toBeNull();
    expect(validateTaskAlertForm({...base, rules: []}, channels)).toBe('errorNoRule');
    expect(validateTaskAlertForm({...base, channelIds: []}, channels)).toBe('errorNoChannel');
    expect(validateTaskAlertForm({...base, cooldown: 'x'}, channels)).toBe('errorCooldown');
    expect(validateTaskAlertForm({...base, channelIds: [2]}, channels)).toBe('errorEmailRecipient');
    expect(validateTaskAlertForm({...base, channelIds: [2], notifyTaskOwner: true}, channels)).toBeNull();
    expect(validateTaskAlertForm({...base, channelIds: [2], receiverUserIds: [5]}, channels)).toBeNull();
    expect(validateTaskAlertForm({...base, channelIds: [3]}, channels)).toBeNull();
  });

  it('drops email-only fields when no email channel is selected', () => {
    const channels = [channel(1, 'webhook'), channel(2, 'email')];
    const form = {...initialTaskAlertForm(configured), channelIds: [1]};
    expect(buildTaskAlertRequest(form, channels)).toMatchObject({
      channel_ids: [1],
      receiver_user_ids: [],
      notify_task_owner: false,
      cooldown_minutes: 30,
      enabled: false,
      send_recovery: false,
    });
    const withEmail = buildTaskAlertRequest({...form, channelIds: [2]}, channels);
    expect(withEmail.receiver_user_ids).toEqual([7]);
    expect(withEmail.notify_task_owner).toBe(true);
  });

  it('summarizes rules and channels', () => {
    const summary = summarizeTaskAlert(configured, [channel(2, 'webhook')], (rule) => rule.replace('sync_', ''), ', ');
    expect(summary).toEqual({rules: 'job_failed, stop_timeout', channels: 'ch-2', cooldown: 30});
    expect(summarizeTaskAlert({...configured, channel_ids: [9]}, [], (rule) => rule, '、').channels).toBe('#9');
  });
});

describe('job deep link', () => {
  it('parses task and job ids', () => {
    expect(parseJobDeepLink(new URLSearchParams('task=12&job=34'))).toEqual({taskId: 12, jobId: 34});
    expect(parseJobDeepLink(new URLSearchParams('task=12&job=x'))).toEqual({taskId: 12, jobId: null});
    expect(parseJobDeepLink(new URLSearchParams('task=0'))).toBeNull();
    expect(parseJobDeepLink(new URLSearchParams('job=3'))).toBeNull();
    expect(parseJobDeepLink(null)).toBeNull();
  });

  it('forwards only task and job from /sync to /workbench', () => {
    expect(buildWorkbenchDeepLinkQuery({task: '12', job: '34', other: 'x'})).toBe('task=12&job=34');
    expect(buildWorkbenchDeepLinkQuery({task: ['5', '6']})).toBe('task=5');
    expect(buildWorkbenchDeepLinkQuery({task: 'abc', job: '3'})).toBe('');
    expect(buildWorkbenchDeepLinkQuery(undefined)).toBe('');
  });
});
