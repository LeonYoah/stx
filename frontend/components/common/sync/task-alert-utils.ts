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

import type {NotificationChannel} from '@/lib/services/monitoring';
import type {
  SyncTaskAlertConfig,
  SyncTaskAlertRequest,
  SyncTaskAlertRule,
} from '@/lib/services/sync';

/** 任务上可以勾选的规则，顺序固定。/ Rules a task can select, in a fixed order. */
export const TASK_ALERT_RULES: SyncTaskAlertRule[] = [
  'sync_submit_failed',
  'sync_submit_timeout',
  'sync_job_failed',
  'sync_stop_timeout',
  'sync_cdc_delay_high',
];

/** 规则键：CDC 延迟（B4）。/ Rule key: CDC delay (B4). */
export const TASK_ALERT_RULE_CDC_DELAY: SyncTaskAlertRule = 'sync_cdc_delay_high';

/**
 * 新配置默认勾选的规则：B4 有前置条件（3.x 集群、开启事件通道），需要用户主动开启，所以不默认勾选。
 * Rules selected by default for a new config: B4 has prerequisites (3.x cluster, event channel on) so users opt in.
 */
export const TASK_ALERT_DEFAULT_RULES: SyncTaskAlertRule[] = TASK_ALERT_RULES.filter(
  (rule) => rule !== TASK_ALERT_RULE_CDC_DELAY,
);

/** B4 阈值（秒）与持续时间（分钟）的默认值和范围。/ B4 threshold (s) and duration (min) defaults and ranges. */
export const CDC_DELAY_DEFAULT_THRESHOLD = 60;
export const CDC_DELAY_MIN_THRESHOLD = 10;
export const CDC_DELAY_MAX_THRESHOLD = 86400;
export const CDC_DELAY_DEFAULT_DURATION = 5;
export const CDC_DELAY_MIN_DURATION = 1;
export const CDC_DELAY_MAX_DURATION = 60;

/** 冷却时间默认值与上限（分钟）。/ Cooldown default and cap (minutes). */
export const TASK_ALERT_DEFAULT_COOLDOWN = 60;
export const TASK_ALERT_MAX_COOLDOWN = 1440;

/** 表单状态。/ Form state. */
export interface TaskAlertFormState {
  enabled: boolean;
  rules: SyncTaskAlertRule[];
  channelIds: number[];
  receiverUserIds: number[];
  notifyTaskOwner: boolean;
  cooldown: string;
  sendRecovery: boolean;
  cdcDelayThreshold: string;
  cdcDelayDuration: string;
}

/**
 * 从已有配置生成表单初值；没有配置时默认勾选除 B4 外的全部规则、冷却 60 分钟、发恢复通知，B4 参数为 60 秒 / 5 分钟。
 * Build the initial form state from a config; without one every rule except B4 is selected, cooldown is 60 minutes,
 * recovery notifications are on and the B4 parameters are 60 s / 5 min.
 */
export function initialTaskAlertForm(config: SyncTaskAlertConfig | null | undefined): TaskAlertFormState {
  if (!config?.configured) {
    return {
      enabled: true,
      rules: [...TASK_ALERT_DEFAULT_RULES],
      channelIds: [],
      receiverUserIds: [],
      notifyTaskOwner: false,
      cooldown: String(TASK_ALERT_DEFAULT_COOLDOWN),
      sendRecovery: true,
      cdcDelayThreshold: String(config?.cdc_delay_threshold_seconds || CDC_DELAY_DEFAULT_THRESHOLD),
      cdcDelayDuration: String(config?.cdc_delay_duration_minutes || CDC_DELAY_DEFAULT_DURATION),
    };
  }
  return {
    enabled: config.enabled,
    rules: TASK_ALERT_RULES.filter((rule) => config.rules.includes(rule)),
    channelIds: [...config.channel_ids],
    receiverUserIds: [...config.receiver_user_ids],
    notifyTaskOwner: config.notify_task_owner,
    cooldown: String(config.cooldown_minutes),
    sendRecovery: config.send_recovery,
    cdcDelayThreshold: String(config.cdc_delay_threshold_seconds || CDC_DELAY_DEFAULT_THRESHOLD),
    cdcDelayDuration: String(config.cdc_delay_duration_minutes || CDC_DELAY_DEFAULT_DURATION),
  };
}

function parseIntInRange(raw: string, min: number, max: number): number | null {
  const trimmed = raw.trim();
  if (!/^\d+$/.test(trimmed)) {
    return null;
  }
  const value = Number(trimmed);
  return value >= min && value <= max ? value : null;
}

/** 解析 B4 阈值：10~86400 的整数秒，非法返回 null。/ Parse the B4 threshold: integer seconds 10–86400, or null. */
export function parseCdcDelayThreshold(raw: string): number | null {
  return parseIntInRange(raw, CDC_DELAY_MIN_THRESHOLD, CDC_DELAY_MAX_THRESHOLD);
}

/** 解析 B4 持续时间：1~60 的整数分钟，非法返回 null。/ Parse the B4 duration: integer minutes 1–60, or null. */
export function parseCdcDelayDuration(raw: string): number | null {
  return parseIntInRange(raw, CDC_DELAY_MIN_DURATION, CDC_DELAY_MAX_DURATION);
}

/** 解析冷却时间输入：0~1440 的整数，非法返回 null。/ Parse the cooldown input: an integer 0–1440, or null. */
export function parseTaskAlertCooldown(raw: string): number | null {
  const trimmed = raw.trim();
  if (!/^\d+$/.test(trimmed)) {
    return null;
  }
  const value = Number(trimmed);
  return value >= 0 && value <= TASK_ALERT_MAX_COOLDOWN ? value : null;
}

/** 选中的渠道里是否有邮件渠道。/ Whether any selected channel is an email channel. */
export function hasEmailChannel(channelIds: number[], channels: NotificationChannel[]): boolean {
  return channels.some((channel) => channelIds.includes(channel.id) && channel.type === 'email');
}

/** 表单校验错误键（对应 syncAlert.* 文案）。/ Form validation error keys (syncAlert.* messages). */
export type TaskAlertFormError =
  | 'errorNoRule'
  | 'errorNoChannel'
  | 'errorCooldown'
  | 'errorCdcDelayThreshold'
  | 'errorCdcDelayDuration'
  | 'errorEmailRecipient';

/**
 * 校验表单：至少一条规则、一个渠道；冷却时间合法；选了邮件渠道时必须有人收得到（接收人、创建人或渠道自带收件人）。
 * Validate the form: at least one rule and one channel, a valid cooldown, and an email channel must reach someone
 * (receivers, the task owner, or the channel's own recipients).
 */
export function validateTaskAlertForm(
  form: TaskAlertFormState,
  channels: NotificationChannel[],
): TaskAlertFormError | null {
  if (form.rules.length === 0) {
    return 'errorNoRule';
  }
  if (form.channelIds.length === 0) {
    return 'errorNoChannel';
  }
  if (parseTaskAlertCooldown(form.cooldown) === null) {
    return 'errorCooldown';
  }
  // B4 参数只在勾选 B4 时校验；没勾选时提交默认值。
  // B4 parameters are only checked when B4 is selected; otherwise defaults are sent.
  if (form.rules.includes(TASK_ALERT_RULE_CDC_DELAY)) {
    if (parseCdcDelayThreshold(form.cdcDelayThreshold) === null) {
      return 'errorCdcDelayThreshold';
    }
    if (parseCdcDelayDuration(form.cdcDelayDuration) === null) {
      return 'errorCdcDelayDuration';
    }
  }
  if (form.receiverUserIds.length === 0 && !form.notifyTaskOwner) {
    const emailWithoutRecipients = channels.some(
      (channel) =>
        form.channelIds.includes(channel.id) &&
        channel.type === 'email' &&
        (channel.config?.email?.recipients ?? []).length === 0,
    );
    if (emailWithoutRecipients) {
      return 'errorEmailRecipient';
    }
  }
  return null;
}

/** 把表单转成请求体；没有邮件渠道时不带接收人和"通知创建人"。/ Turn the form into a request body. */
export function buildTaskAlertRequest(
  form: TaskAlertFormState,
  channels: NotificationChannel[],
): SyncTaskAlertRequest {
  const email = hasEmailChannel(form.channelIds, channels);
  return {
    enabled: form.enabled,
    rules: TASK_ALERT_RULES.filter((rule) => form.rules.includes(rule)),
    channel_ids: [...form.channelIds],
    receiver_user_ids: email ? [...form.receiverUserIds] : [],
    notify_task_owner: email ? form.notifyTaskOwner : false,
    cooldown_minutes: parseTaskAlertCooldown(form.cooldown) ?? TASK_ALERT_DEFAULT_COOLDOWN,
    send_recovery: form.sendRecovery,
    cdc_delay_threshold_seconds: parseCdcDelayThreshold(form.cdcDelayThreshold) ?? CDC_DELAY_DEFAULT_THRESHOLD,
    cdc_delay_duration_minutes: parseCdcDelayDuration(form.cdcDelayDuration) ?? CDC_DELAY_DEFAULT_DURATION,
  };
}

/**
 * 配置摘要的组成部分：规则名、渠道名、冷却分钟；由调用方按语言拼接。
 * Parts of the config summary: rule labels, channel names and cooldown minutes; the caller joins them per language.
 */
export function summarizeTaskAlert(
  config: SyncTaskAlertConfig,
  channels: NotificationChannel[],
  ruleLabel: (rule: SyncTaskAlertRule) => string,
  separator: string,
): {rules: string; channels: string; cooldown: number} {
  const names = config.channel_ids.map(
    (id) => channels.find((channel) => channel.id === id)?.name ?? `#${id}`,
  );
  return {
    rules: TASK_ALERT_RULES.filter((rule) => config.rules.includes(rule)).map(ruleLabel).join(separator),
    channels: names.join(separator),
    cooldown: config.cooldown_minutes,
  };
}

/** 作业深链参数。/ Job deep link parameters. */
export interface JobDeepLink {
  taskId: number;
  jobId: number | null;
}

function positiveInt(raw: string | null | undefined): number | null {
  if (!raw || !/^\d+$/.test(raw.trim())) {
    return null;
  }
  const value = Number(raw.trim());
  return Number.isSafeInteger(value) && value > 0 ? value : null;
}

/**
 * 解析 ?task=<id>&job=<id>；task 非法时返回 null，job 非法时忽略。
 * Parse ?task=<id>&job=<id>; an invalid task yields null and an invalid job is ignored.
 */
export function parseJobDeepLink(params: {get(name: string): string | null} | null | undefined): JobDeepLink | null {
  const taskId = positiveInt(params?.get('task'));
  if (!taskId) {
    return null;
  }
  return {taskId, jobId: positiveInt(params?.get('job'))};
}

/**
 * 从 /sync 页面的查询参数里只保留 task、job，拼成转到 /workbench 的查询串。
 * Keep only task and job from the /sync page query and build the query string for /workbench.
 */
export function buildWorkbenchDeepLinkQuery(
  params: Record<string, string | string[] | undefined> | null | undefined,
): string {
  const pick = (name: string) => {
    const raw = params?.[name];
    return positiveInt(Array.isArray(raw) ? raw[0] : raw);
  };
  const taskId = pick('task');
  if (!taskId) {
    return '';
  }
  const query = new URLSearchParams({task: String(taskId)});
  const jobId = pick('job');
  if (jobId) {
    query.set('job', String(jobId));
  }
  return query.toString();
}

/**
 * 作业告警的自动关闭原因（静默关闭，没有恢复通知），告警中心按它显示 syncAlert.closeReason_*；恢复和人工关闭返回 null。
 * Auto-close reasons of job alerts (closed silently, no recovery notification); the alert center renders
 * syncAlert.closeReason_* from it. Recovery and manual close yield null.
 */
export const SYNC_AUTO_CLOSE_REASONS = [
  'job_ended',
  'channel_unavailable',
  'rule_removed',
  'rule_changed',
  'policy_deleted',
] as const;

export type SyncAutoCloseReason = (typeof SYNC_AUTO_CLOSE_REASONS)[number];

export function syncAutoCloseReason(reason: string | undefined | null): SyncAutoCloseReason | null {
  return SYNC_AUTO_CLOSE_REASONS.find((item) => item === reason) ?? null;
}
