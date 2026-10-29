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

import type {
  NotificationChannel,
  NotificationChannelType,
  UpsertNotificationChannelRequest,
} from '@/lib/services/monitoring';

/**
 * Webhook 弹窗里可以创建的出站渠道类型：通用 Webhook 和三种 IM 机器人。
 * Outbound channel types the webhook dialog can create: the generic webhook and three IM bots.
 */
export const WEBHOOK_LIKE_CHANNEL_TYPES = [
  'webhook',
  'dingtalk',
  'wecom',
  'feishu',
] as const;

export type WebhookLikeChannelType =
  (typeof WEBHOOK_LIKE_CHANNEL_TYPES)[number];

/** 后端 GET 接口返回的脱敏占位符。/ Placeholder the backend GET returns for secrets. */
export const NOTIFICATION_SECRET_MASK = '******';

export type WebhookChannelFormState = {
  id: number | null;
  type: WebhookLikeChannelType;
  name: string;
  enabled: boolean;
  description: string;
  endpoint: string;
  secret: string;
};

export function isWebhookLikeChannelType(
  type?: NotificationChannelType | string | null,
): type is WebhookLikeChannelType {
  return (WEBHOOK_LIKE_CHANNEL_TYPES as readonly string[]).includes(
    String(type || ''),
  );
}

export function isWebhookLikeChannel(
  channel?: Pick<NotificationChannel, 'type'> | null,
): boolean {
  return isWebhookLikeChannelType(channel?.type);
}

/**
 * 是否显示"加签密钥"：钉钉、飞书支持加签；通用 Webhook 保留原有共享密钥；企业微信不支持。
 * Whether to show the signing secret: DingTalk and Feishu sign requests, the generic webhook keeps its shared secret,
 * WeCom has none.
 */
export function channelTypeSupportsSecret(
  type: WebhookLikeChannelType,
): boolean {
  return type !== 'wecom';
}

export function endpointPlaceholderForType(
  type: WebhookLikeChannelType,
): string {
  switch (type) {
    case 'dingtalk':
      return 'https://oapi.dingtalk.com/robot/send?access_token=...';
    case 'wecom':
      return 'https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=...';
    case 'feishu':
      return 'https://open.feishu.cn/open-apis/bot/v2/hook/...';
    default:
      return 'https://hooks.example.com/stx';
  }
}

export function createDefaultWebhookChannelForm(
  type: WebhookLikeChannelType = 'webhook',
): WebhookChannelFormState {
  return {
    id: null,
    type,
    name: '',
    enabled: true,
    description: '',
    endpoint: '',
    secret: '',
  };
}

/**
 * 从已保存渠道生成表单。endpoint / secret 是后端脱敏后的值，原样提交即表示"保持不变"。
 * Builds the form from a saved channel. endpoint / secret are the backend's redacted values; submitting them unchanged
 * means "keep the stored value".
 */
export function createWebhookChannelFormFromChannel(
  channel: NotificationChannel,
): WebhookChannelFormState {
  return {
    id: channel.id,
    type: isWebhookLikeChannelType(channel.type) ? channel.type : 'webhook',
    name: channel.name,
    enabled: channel.enabled,
    description: channel.description || '',
    endpoint: channel.endpoint || '',
    secret: channel.secret || '',
  };
}

export function buildWebhookChannelPayload(
  form: WebhookChannelFormState,
  enabledOverride?: boolean,
): UpsertNotificationChannelRequest {
  return {
    name: form.name.trim(),
    type: form.type,
    enabled: enabledOverride ?? form.enabled,
    description: form.description.trim(),
    endpoint: form.endpoint.trim(),
    secret: channelTypeSupportsSecret(form.type) ? form.secret.trim() : '',
  };
}

/**
 * 测试结果里的平台响应：能解析成 JSON 时格式化展示。
 * Platform response from a test result, pretty-printed when it is JSON.
 */
export function formatChannelTestResponseBody(body?: string | null): string {
  const raw = (body || '').trim();
  if (!raw) {
    return '';
  }
  try {
    return JSON.stringify(JSON.parse(raw), null, 2);
  } catch {
    return raw;
  }
}
