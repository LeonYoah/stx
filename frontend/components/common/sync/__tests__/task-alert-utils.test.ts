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
import en from '@/lib/i18n/locales/en.json';
import zh from '@/lib/i18n/locales/zh.json';
import type {NotificationChannel} from '@/lib/services/monitoring';
import type {SyncTaskAlertConfig} from '@/lib/services/sync';
import {
  SYNC_AUTO_CLOSE_REASONS,
  TASK_ALERT_DEFAULT_RULES,
  TASK_ALERT_RULES,
  buildTaskAlertRequest,
  buildWorkbenchDeepLinkQuery,
  initialTaskAlertForm,
  parseCdcDelayDuration,
  parseCdcDelayThreshold,
  parseJobDeepLink,
  parseTaskAlertCooldown,
  summarizeTaskAlert,
  syncAutoCloseReason,
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
  cdc_delay_threshold_seconds: 120,
  cdc_delay_duration_minutes: 10,
};

describe('task alert form', () => {
  it('defaults to every rule except CDC delay for a new config and keeps an existing one', () => {
    const fresh = initialTaskAlertForm(null);
    expect(fresh.rules).toEqual(TASK_ALERT_DEFAULT_RULES);
    expect(fresh.rules).not.toContain('sync_cdc_delay_high');
    expect(TASK_ALERT_RULES).toContain('sync_cdc_delay_high');
    expect(fresh.cooldown).toBe('60');
    expect(fresh).toMatchObject({cdcDelayThreshold: '60', cdcDelayDuration: '5'});
    expect(fresh.enabled && fresh.sendRecovery).toBe(true);

    const existing = initialTaskAlertForm(configured);
    expect(existing.rules).toEqual(['sync_job_failed', 'sync_stop_timeout']);
    expect(existing).toMatchObject({enabled: false, cooldown: '30', notifyTaskOwner: true, sendRecovery: false});
    expect(existing).toMatchObject({cdcDelayThreshold: '120', cdcDelayDuration: '10'});
    // 旧配置没有 B4 参数（0）时回落到默认值。/ Old configs without B4 params (0) fall back to defaults.
    const legacy = initialTaskAlertForm({...configured, cdc_delay_threshold_seconds: 0, cdc_delay_duration_minutes: 0});
    expect(legacy).toMatchObject({cdcDelayThreshold: '60', cdcDelayDuration: '5'});
  });

  it('parses and validates the CDC delay threshold and duration only when the rule is selected', () => {
    expect(parseCdcDelayThreshold('10')).toBe(10);
    expect(parseCdcDelayThreshold('86400')).toBe(86400);
    expect(parseCdcDelayThreshold('9')).toBeNull();
    expect(parseCdcDelayThreshold('86401')).toBeNull();
    expect(parseCdcDelayDuration('1')).toBe(1);
    expect(parseCdcDelayDuration('60')).toBe(60);
    expect(parseCdcDelayDuration('0')).toBeNull();
    expect(parseCdcDelayDuration('61')).toBeNull();

    const channels = [channel(1, 'webhook')];
    const base = {...initialTaskAlertForm(null), channelIds: [1]};
    // 未勾选 B4：参数不合法也不拦截，提交默认值。/ B4 unselected: bad params do not block and defaults are sent.
    expect(validateTaskAlertForm({...base, cdcDelayThreshold: '5'}, channels)).toBeNull();
    expect(buildTaskAlertRequest({...base, cdcDelayThreshold: '5'}, channels)).toMatchObject({
      cdc_delay_threshold_seconds: 60,
      cdc_delay_duration_minutes: 5,
    });
    const withCdc = {...base, rules: [...base.rules, 'sync_cdc_delay_high' as const]};
    expect(validateTaskAlertForm({...withCdc, cdcDelayThreshold: '5'}, channels)).toBe('errorCdcDelayThreshold');
    expect(validateTaskAlertForm({...withCdc, cdcDelayDuration: '61'}, channels)).toBe('errorCdcDelayDuration');
    expect(
      buildTaskAlertRequest({...withCdc, cdcDelayThreshold: ' 300 ', cdcDelayDuration: '15'}, channels),
    ).toMatchObject({rules: TASK_ALERT_RULES, cdc_delay_threshold_seconds: 300, cdc_delay_duration_minutes: 15});
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

describe('sync alert close reasons', () => {
  it('maps only silent auto-close reasons and has zh/en copy for each', () => {
    expect(syncAutoCloseReason('job_ended')).toBe('job_ended');
    expect(syncAutoCloseReason('rule_changed')).toBe('rule_changed');
    expect(syncAutoCloseReason('recovered')).toBeNull();
    expect(syncAutoCloseReason('closed')).toBeNull();
    expect(syncAutoCloseReason(undefined)).toBeNull();
    for (const reason of SYNC_AUTO_CLOSE_REASONS) {
      const key = `closeReason_${reason}` as keyof typeof zh.syncAlert;
      expect(zh.syncAlert[key]).toBeTruthy();
      expect(en.syncAlert[key as keyof typeof en.syncAlert]).toBeTruthy();
    }
    for (const rule of TASK_ALERT_RULES) {
      expect(zh.syncAlert[`rule_${rule}` as keyof typeof zh.syncAlert]).toBeTruthy();
      expect(en.syncAlert[`rule_${rule}_hint` as keyof typeof en.syncAlert]).toBeTruthy();
    }
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
