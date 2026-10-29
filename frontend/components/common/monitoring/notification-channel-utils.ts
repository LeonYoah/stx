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
  /**
   * 用户点了"清除密钥"：保存时带 clear_secret=true，后端清空已保存的密钥。
   * The user chose "clear secret": saving sends clear_secret=true and the backend drops the stored secret.
   */
  clearSecret: boolean;
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
 * 是否显示"加签密钥"：钉钉、飞书按平台规则加签；通用 Webhook 用它生成 X-STX-Signature 请求头；企业微信不支持。
 * Whether to show the signing secret: DingTalk and Feishu sign per platform rules, the generic webhook uses it for the
 * X-STX-Signature header, WeCom has none.
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
    clearSecret: false,
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
    clearSecret: false,
  };
}

/** 标记清除已保存的密钥。/ Marks the stored secret for removal. */
export function clearWebhookChannelSecret(
  form: WebhookChannelFormState,
): WebhookChannelFormState {
  return {...form, secret: '', clearSecret: true};
}

/**
 * 编辑中的已保存渠道：类型和是否有已保存的密钥（GET 返回 ******）。
 * The saved channel being edited: its type and whether it has a stored secret (GET returns ******).
 */
export type SavedWebhookChannelSecret = {
  type: WebhookLikeChannelType | string;
  hasSecret: boolean;
};

export function savedWebhookChannelSecret(
  channel?: Pick<NotificationChannel, 'type' | 'secret'> | null,
): SavedWebhookChannelSecret | null {
  if (!channel) {
    return null;
  }
  return {
    type: channel.type,
    hasSecret: Boolean((channel.secret || '').trim()),
  };
}

/**
 * 输入密钥。按规范 §9"留空保持不变"：把输入框清空不会清除已保存的密钥（后端对空值 / 占位符同样保持原密钥）；只有点
 * "清除"按钮（clearWebhookChannelSecret）才会带 clear_secret，保存前还要确认。输入新密钥会取消已选择的清除。
 * Types a secret. Per spec §9 ("leave empty to keep"): emptying the input never clears the stored secret (the backend
 * also keeps it for an empty value or the placeholder); only the explicit "clear" button (clearWebhookChannelSecret)
 * sends clear_secret, and saving asks for confirmation. Typing a new secret cancels a pending clear.
 */
export function setWebhookChannelSecret(
  form: WebhookChannelFormState,
  value: string,
): WebhookChannelFormState {
  return {
    ...form,
    secret: value,
    clearSecret: value.trim() === '' ? form.clearSecret : false,
  };
}

/**
 * 保存前是否需要确认"清除已保存的密钥"：仅编辑已保存渠道且用户点了"清除"时。
 * Whether saving must first confirm clearing the stored secret: only for a saved channel the user chose to clear.
 */
export function webhookChannelSaveNeedsClearSecretConfirm(
  form: WebhookChannelFormState,
): boolean {
  return form.id !== null && form.clearSecret;
}

/**
 * 切换渠道类型：已保存的密钥属于原平台，换到别的类型时不沿用（后端同样会丢弃），所以清掉占位符；切回原类型时恢复
 * 占位符（表示保持原密钥），除非用户已经选择清除或填了新密钥。
 * Switching the channel type: the stored secret belongs to the original platform and is not reused for another type
 * (the backend drops it too), so the placeholder is cleared; switching back restores the placeholder (keep the stored
 * secret) unless the user chose to clear it or typed a new one.
 */
export function changeWebhookChannelType(
  form: WebhookChannelFormState,
  type: WebhookLikeChannelType,
  saved?: SavedWebhookChannelSecret | null,
): WebhookChannelFormState {
  if (type === form.type) {
    return form;
  }
  const typed = form.secret.trim();
  if (saved?.hasSecret && form.id !== null && type === saved.type) {
    if (typed === '' && !form.clearSecret) {
      return {...form, type, secret: NOTIFICATION_SECRET_MASK};
    }
    return {...form, type};
  }
  const secret = typed === NOTIFICATION_SECRET_MASK ? '' : form.secret;
  return {...form, type, secret};
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
    ...(form.clearSecret ? {clear_secret: true} : {}),
  };
}

/**
 * "失败"筛选 / 计数包含的投递状态：发送失败和重试已用完（exhausted）。
 * Delivery statuses the "failed" pill counts and filters: failed and retries exhausted.
 */
export function isFailedDeliveryStatus(status?: string | null): boolean {
  return status === 'failed' || status === 'exhausted';
}

/**
 * 投递记录的重试次数：attempt_count 含首次发送，所以重试次数是 attempt_count - 1。
 * Retries of a delivery: attempt_count includes the first send, so retries are attempt_count - 1.
 */
export function deliveryRetriedTimes(attemptCount?: number | null): number {
  return Math.max((attemptCount ?? 0) - 1, 0);
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
