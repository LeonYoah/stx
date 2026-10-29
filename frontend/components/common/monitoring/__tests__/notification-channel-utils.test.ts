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
import zh from '@/lib/i18n/locales/zh.json';
import en from '@/lib/i18n/locales/en.json';
import type {NotificationChannel} from '@/lib/services/monitoring';
import {
  NOTIFICATION_SECRET_MASK,
  WEBHOOK_LIKE_CHANNEL_TYPES,
  buildWebhookChannelPayload,
  channelTypeSupportsSecret,
  createDefaultWebhookChannelForm,
  createWebhookChannelFormFromChannel,
  endpointPlaceholderForType,
  formatChannelTestResponseBody,
  isWebhookLikeChannel,
} from '../notification-channel-utils';

function channel(overrides: Partial<NotificationChannel>): NotificationChannel {
  return {
    id: 1,
    name: 'ops',
    type: 'webhook',
    enabled: true,
    endpoint: '',
    secret: '',
    description: '',
    created_at: '',
    updated_at: '',
    ...overrides,
  };
}

describe('notification channel form (DingTalk / WeCom / Feishu)', () => {
  it('treats every non-email channel type as webhook-like', () => {
    expect(isWebhookLikeChannel(channel({type: 'webhook'}))).toBe(true);
    expect(isWebhookLikeChannel(channel({type: 'dingtalk'}))).toBe(true);
    expect(isWebhookLikeChannel(channel({type: 'wecom'}))).toBe(true);
    expect(isWebhookLikeChannel(channel({type: 'feishu'}))).toBe(true);
    expect(isWebhookLikeChannel(channel({type: 'email'}))).toBe(false);
  });

  it('builds create payloads per type, including the signing secret', () => {
    const ding = {
      ...createDefaultWebhookChannelForm('dingtalk'),
      name: ' 钉钉值班群 ',
      endpoint: ' https://oapi.dingtalk.com/robot/send?access_token=abc ',
      secret: ' SECabc ',
    };
    expect(buildWebhookChannelPayload(ding)).toEqual({
      name: '钉钉值班群',
      type: 'dingtalk',
      enabled: true,
      description: '',
      endpoint: 'https://oapi.dingtalk.com/robot/send?access_token=abc',
      secret: 'SECabc',
    });

    const feishu = {
      ...createDefaultWebhookChannelForm('feishu'),
      name: 'feishu',
      endpoint: 'https://open.feishu.cn/open-apis/bot/v2/hook/x',
      secret: 'fs',
    };
    expect(buildWebhookChannelPayload(feishu, false)).toMatchObject({
      type: 'feishu',
      enabled: false,
      secret: 'fs',
    });

    // 企业微信机器人没有加签，残留的密钥不会提交。
    // WeCom bots have no signing, so a leftover secret is not submitted.
    const wecom = {
      ...createDefaultWebhookChannelForm('wecom'),
      name: 'wecom',
      endpoint: 'https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=k',
      secret: 'stale',
    };
    expect(buildWebhookChannelPayload(wecom)).toMatchObject({
      type: 'wecom',
      secret: '',
    });
    expect(channelTypeSupportsSecret('wecom')).toBe(false);
    expect(channelTypeSupportsSecret('dingtalk')).toBe(true);
    expect(channelTypeSupportsSecret('feishu')).toBe(true);
  });

  it('round-trips masked values so the backend keeps the stored secret', () => {
    const form = createWebhookChannelFormFromChannel(
      channel({
        id: 9,
        type: 'feishu',
        endpoint: 'https://open.feishu.cn/open-apis/bot/v2/hook/******',
        secret: NOTIFICATION_SECRET_MASK,
      }),
    );
    expect(form.type).toBe('feishu');
    expect(buildWebhookChannelPayload(form)).toMatchObject({
      type: 'feishu',
      endpoint: 'https://open.feishu.cn/open-apis/bot/v2/hook/******',
      secret: NOTIFICATION_SECRET_MASK,
    });
  });

  it('has endpoint placeholders and zh/en labels for every type', () => {
    for (const type of WEBHOOK_LIKE_CHANNEL_TYPES) {
      expect(endpointPlaceholderForType(type)).toMatch(/^https:\/\//);
      for (const locale of [zh, en]) {
        const webhook = locale.monitoringCenter.policyCenterV2.webhook;
        expect(webhook.types[type]).toBeTruthy();
        expect(webhook.typeHints[type]).toBeTruthy();
      }
    }
    for (const locale of [zh, en]) {
      const webhook = locale.monitoringCenter.policyCenterV2.webhook;
      expect(webhook.secretHints.dingtalk).toBeTruthy();
      expect(webhook.secretHints.feishu).toBeTruthy();
      expect(webhook.fields.signSecret).toBeTruthy();
      expect(webhook.lastTest.responseBody).toBeTruthy();
      expect(webhook.maskedKeepHint).toBeTruthy();
    }
  });

  it('pretty-prints the platform response shown by the test button', () => {
    expect(
      formatChannelTestResponseBody(
        '{"errcode":310000,"errmsg":"sign not match"}',
      ),
    ).toBe('{\n  "errcode": 310000,\n  "errmsg": "sign not match"\n}');
    expect(formatChannelTestResponseBody('bad gateway')).toBe('bad gateway');
    expect(formatChannelTestResponseBody('')).toBe('');
  });
});
