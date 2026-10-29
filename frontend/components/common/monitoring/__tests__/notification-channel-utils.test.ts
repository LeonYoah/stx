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
  changeWebhookChannelType,
  channelTypeSupportsSecret,
  clearWebhookChannelSecret,
  savedWebhookChannelSecret,
  setWebhookChannelSecret,
  webhookChannelSaveNeedsClearSecretConfirm,
  isFailedDeliveryStatus,
  deliveryRetriedTimes,
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

  it('sends clear_secret only when the user clears the saved secret', () => {
    const saved = createWebhookChannelFormFromChannel(
      channel({id: 3, type: 'dingtalk', secret: NOTIFICATION_SECRET_MASK}),
    );
    expect(saved.clearSecret).toBe(false);
    expect(buildWebhookChannelPayload(saved)).not.toHaveProperty(
      'clear_secret',
    );

    const cleared = clearWebhookChannelSecret(saved);
    expect(cleared).toMatchObject({secret: '', clearSecret: true});
    expect(buildWebhookChannelPayload(cleared)).toMatchObject({
      type: 'dingtalk',
      secret: '',
      clear_secret: true,
    });
  });

  it('drops the masked secret when the channel type changes', () => {
    const saved = createWebhookChannelFormFromChannel(
      channel({id: 4, type: 'webhook', secret: NOTIFICATION_SECRET_MASK}),
    );
    const feishu = changeWebhookChannelType(saved, 'feishu');
    expect(feishu).toMatchObject({type: 'feishu', secret: ''});
    expect(buildWebhookChannelPayload(feishu).secret).toBe('');
    // 新填的密钥在切换类型时保留；同类型不变。
    // A newly typed secret survives a type switch; the same type is a no-op.
    const typed = {...saved, secret: 'fresh'};
    expect(changeWebhookChannelType(typed, 'dingtalk').secret).toBe('fresh');
    expect(changeWebhookChannelType(saved, 'webhook')).toBe(saved);
  });

  it('keeps the saved secret when the input is emptied; only the clear button clears it (spec §9)', () => {
    const savedChannel = channel({
      id: 5,
      type: 'dingtalk',
      secret: NOTIFICATION_SECRET_MASK,
    });
    const form = createWebhookChannelFormFromChannel(savedChannel);

    // 清空输入框不等于清除：不带 clear_secret，后端对空值保持原密钥，保存时也不弹确认。
    // Emptying the input is not a clear: no clear_secret (the backend keeps the stored secret for an empty value) and
    // no confirmation on save.
    const emptied = setWebhookChannelSecret(form, '');
    expect(emptied).toMatchObject({secret: '', clearSecret: false});
    expect(buildWebhookChannelPayload(emptied)).not.toHaveProperty(
      'clear_secret',
    );
    expect(buildWebhookChannelPayload(emptied).secret).toBe('');
    expect(webhookChannelSaveNeedsClearSecretConfirm(emptied)).toBe(false);

    // 只有点"清除"才带 clear_secret，并且保存前需要确认。
    // Only the explicit clear sends clear_secret, and saving must confirm it first.
    const cleared = clearWebhookChannelSecret(form);
    expect(buildWebhookChannelPayload(cleared)).toMatchObject({
      secret: '',
      clear_secret: true,
    });
    expect(webhookChannelSaveNeedsClearSecretConfirm(cleared)).toBe(true);
    // 选择清除后输入框仍为空：保持清除状态。
    // Still empty after choosing to clear: the clear stays pending.
    expect(setWebhookChannelSecret(cleared, '').clearSecret).toBe(true);

    // 重新输入新密钥：取消清除，提交新密钥，不再需要确认。
    // Typing a new secret cancels the clear and submits the new one without confirmation.
    const retyped = setWebhookChannelSecret(cleared, 'SECnew');
    expect(retyped.clearSecret).toBe(false);
    expect(buildWebhookChannelPayload(retyped)).not.toHaveProperty(
      'clear_secret',
    );
    expect(webhookChannelSaveNeedsClearSecretConfirm(retyped)).toBe(false);

    // 新建渠道：清空就是空，不带 clear_secret、不确认。
    // A new channel: empty is just empty, no clear_secret and no confirmation.
    const created = setWebhookChannelSecret(
      createDefaultWebhookChannelForm('dingtalk'),
      '',
    );
    expect(created.clearSecret).toBe(false);
    expect(webhookChannelSaveNeedsClearSecretConfirm(created)).toBe(false);
  });

  it('counts exhausted deliveries as failed and reports retries without the first send', () => {
    expect(isFailedDeliveryStatus('failed')).toBe(true);
    expect(isFailedDeliveryStatus('exhausted')).toBe(true);
    for (const status of [
      'sent',
      'sending',
      'retrying',
      'pending',
      'canceled',
      '',
      undefined,
    ]) {
      expect(isFailedDeliveryStatus(status)).toBe(false);
    }
    expect(deliveryRetriedTimes(6)).toBe(5);
    expect(deliveryRetriedTimes(1)).toBe(0);
    expect(deliveryRetriedTimes(0)).toBe(0);
    expect(deliveryRetriedTimes(undefined)).toBe(0);
  });

  it('restores the masked secret when switching back to the saved type', () => {
    const savedChannel = channel({
      id: 7,
      type: 'webhook',
      secret: NOTIFICATION_SECRET_MASK,
    });
    const saved = savedWebhookChannelSecret(savedChannel);
    const form = createWebhookChannelFormFromChannel(savedChannel);

    const away = changeWebhookChannelType(form, 'feishu', saved);
    expect(away).toMatchObject({
      type: 'feishu',
      secret: '',
      clearSecret: false,
    });
    const back = changeWebhookChannelType(away, 'webhook', saved);
    // 切回原类型：恢复占位符（保持原密钥），清除按钮重新可用，提交时后端保留原密钥。
    // Back on the saved type: the placeholder returns (keep the stored secret) so the clear button is available again.
    expect(back).toMatchObject({
      type: 'webhook',
      secret: NOTIFICATION_SECRET_MASK,
      clearSecret: false,
    });
    expect(buildWebhookChannelPayload(back).secret).toBe(
      NOTIFICATION_SECRET_MASK,
    );

    // 用户已选择清除：切走再切回仍是清除状态。
    // The user chose to clear: switching away and back keeps the clear.
    const cleared = clearWebhookChannelSecret(form);
    const roundTrip = changeWebhookChannelType(
      changeWebhookChannelType(cleared, 'dingtalk', saved),
      'webhook',
      saved,
    );
    expect(roundTrip.secret).toBe('');
    expect(buildWebhookChannelPayload(roundTrip)).toMatchObject({
      type: 'webhook',
      secret: '',
    });

    // 在别的类型上填了新密钥再切回：保留新密钥。
    // A new secret typed on another type survives switching back.
    const typed = setWebhookChannelSecret(away, 'fresh');
    expect(changeWebhookChannelType(typed, 'webhook', saved).secret).toBe(
      'fresh',
    );
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
      expect(webhook.clearSecret).toBeTruthy();
      expect(webhook.undoClearSecret).toBeTruthy();
      expect(webhook.secretWillBeCleared).toBeTruthy();
      expect(webhook.secretDroppedOnTypeChange).toBeTruthy();
      expect(webhook.secretHint).toContain('X-STX-Signature');
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
