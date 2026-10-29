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

// 任务侧栏的告警分组：未配置时只显示一行和"配置"，已配置时显示摘要；保存走任务告警接口。
// The task sidebar alerts group: one line plus Configure when empty, a summary when configured; saves go through
// the task alert API.

import {fireEvent, render, screen, waitFor} from '@testing-library/react';
import {NextIntlClientProvider} from 'next-intl';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import zh from '@/lib/i18n/locales/zh.json';
import {TaskAlertSection} from '../TaskAlertSection';

const getTaskAlert = vi.fn();
const updateTaskAlert = vi.fn();
const listNotificationChannels = vi.fn();
const getEventChannel = vi.fn();

vi.mock('@/lib/services', () => ({
  default: {
    sync: {
      getTaskAlert: (...args: unknown[]) => getTaskAlert(...args),
      updateTaskAlert: (...args: unknown[]) => updateTaskAlert(...args),
      deleteTaskAlert: vi.fn(),
      getEventChannel: (...args: unknown[]) => getEventChannel(...args),
    },
    monitoring: {
      listNotificationChannels: (...args: unknown[]) => listNotificationChannels(...args),
    },
  },
}));

vi.mock('sonner', () => ({
  toast: {success: vi.fn(), error: vi.fn(), warning: vi.fn()},
}));

const webhook = {
  id: 2,
  name: '运维群',
  type: 'webhook',
  enabled: true,
  endpoint: '',
  secret: '',
  config: null,
  description: '',
  created_at: '',
  updated_at: '',
};

function renderSection(canEdit = true, clusterId: number | null = null) {
  return render(
    <NextIntlClientProvider locale='zh' messages={zh}>
      <TaskAlertSection taskId={42} taskName='orders' clusterId={clusterId} canEdit={canEdit} users={[]} />
    </NextIntlClientProvider>,
  );
}

describe('TaskAlertSection', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    listNotificationChannels.mockResolvedValue({channels: [webhook]});
  });

  it('shows a single line with Configure when nothing is configured, and saves defaults', async () => {
    getTaskAlert.mockResolvedValue({
      configured: false,
      enabled: false,
      rules: [],
      channel_ids: [],
      receiver_user_ids: [],
      notify_task_owner: false,
      cooldown_minutes: 60,
      send_recovery: true,
    });
    updateTaskAlert.mockResolvedValue({
      configured: true,
      policy_id: 9,
      enabled: true,
      rules: ['sync_submit_failed', 'sync_submit_timeout', 'sync_job_failed', 'sync_stop_timeout'],
      channel_ids: [2],
      receiver_user_ids: [],
      notify_task_owner: false,
      cooldown_minutes: 60,
      send_recovery: true,
    });
    renderSection();

    expect(await screen.findByTestId('task-alert-empty')).toHaveTextContent('未配置告警');
    fireEvent.click(screen.getByTestId('task-alert-open'));

    // 没选渠道时保存被拦下并提示。
    // Saving without a channel is blocked with a message.
    fireEvent.click(await screen.findByTestId('task-alert-save'));
    expect(await screen.findByTestId('task-alert-error')).toHaveTextContent('至少选择一个通知渠道');
    expect(updateTaskAlert).not.toHaveBeenCalled();

    fireEvent.click(screen.getByTestId('task-alert-channel-2'));
    fireEvent.click(screen.getByTestId('task-alert-save'));
    await waitFor(() => expect(updateTaskAlert).toHaveBeenCalledTimes(1));
    expect(updateTaskAlert).toHaveBeenCalledWith(42, {
      enabled: true,
      rules: ['sync_submit_failed', 'sync_submit_timeout', 'sync_job_failed', 'sync_stop_timeout'],
      channel_ids: [2],
      receiver_user_ids: [],
      notify_task_owner: false,
      cooldown_minutes: 60,
      send_recovery: true,
      cdc_delay_threshold_seconds: 60,
      cdc_delay_duration_minutes: 5,
    });
    expect(await screen.findByTestId('task-alert-summary')).toHaveTextContent('运维群');
  });

  it('summarizes an existing config and keeps it read-only without edit rights', async () => {
    getTaskAlert.mockResolvedValue({
      configured: true,
      policy_id: 9,
      enabled: false,
      rules: ['sync_job_failed'],
      channel_ids: [2],
      receiver_user_ids: [],
      notify_task_owner: false,
      cooldown_minutes: 0,
      send_recovery: true,
    });
    renderSection(false);

    const summary = await screen.findByTestId('task-alert-summary');
    expect(summary).toHaveTextContent('作业失败 · 运维群 · 不重复提醒');
    expect(screen.getByText('已停用')).toBeInTheDocument();
    fireEvent.click(screen.getByTestId('task-alert-open'));
    expect(await screen.findByText('你没有这个任务的编辑权限，只能查看告警配置')).toBeInTheDocument();
    expect(screen.queryByTestId('task-alert-save')).not.toBeInTheDocument();
  });

  it('shows CDC delay parameters with hints and warns when the event channel is off', async () => {
    getTaskAlert.mockResolvedValue({
      configured: false,
      enabled: false,
      rules: [],
      channel_ids: [],
      receiver_user_ids: [],
      notify_task_owner: false,
      cooldown_minutes: 60,
      send_recovery: true,
      cdc_delay_threshold_seconds: 60,
      cdc_delay_duration_minutes: 5,
    });
    getEventChannel.mockResolvedValue({cluster_id: 7, enabled: false, state: 'disabled', supported: true});
    updateTaskAlert.mockResolvedValue({
      configured: true,
      policy_id: 9,
      enabled: true,
      rules: ['sync_cdc_delay_high'],
      channel_ids: [2],
      receiver_user_ids: [],
      notify_task_owner: false,
      cooldown_minutes: 60,
      send_recovery: true,
      cdc_delay_threshold_seconds: 30,
      cdc_delay_duration_minutes: 5,
    });
    renderSection(true, 7);

    fireEvent.click(await screen.findByTestId('task-alert-open'));
    expect(await screen.findByTestId('task-alert-rule-sync_cdc_delay_high')).toBeInTheDocument();
    expect(screen.queryByTestId('task-alert-cdc-delay')).not.toBeInTheDocument();

    fireEvent.click(screen.getByTestId('task-alert-rule-sync_cdc_delay_high'));
    expect(await screen.findByTestId('task-alert-cdc-delay')).toHaveTextContent(
      '建议 ≥30 秒；数据库与 SeaTunnel 节点时间不同步会让延迟偏大',
    );
    expect(await screen.findByTestId('task-alert-cdc-channel-warning')).toHaveTextContent('未开启事件通道');
    expect(getEventChannel).toHaveBeenCalledWith(7);

    fireEvent.change(screen.getByTestId('task-alert-cdc-threshold'), {target: {value: '5'}});
    fireEvent.click(screen.getByTestId('task-alert-channel-2'));
    fireEvent.click(screen.getByTestId('task-alert-save'));
    expect(await screen.findByTestId('task-alert-error')).toHaveTextContent('延迟阈值需要是 10 到 86400');
    expect(updateTaskAlert).not.toHaveBeenCalled();

    fireEvent.change(screen.getByTestId('task-alert-cdc-threshold'), {target: {value: '30'}});
    fireEvent.click(screen.getByTestId('task-alert-save'));
    await waitFor(() => expect(updateTaskAlert).toHaveBeenCalledTimes(1));
    expect(updateTaskAlert.mock.calls[0][1]).toMatchObject({
      rules: ['sync_submit_failed', 'sync_submit_timeout', 'sync_job_failed', 'sync_stop_timeout', 'sync_cdc_delay_high'],
      cdc_delay_threshold_seconds: 30,
      cdc_delay_duration_minutes: 5,
    });
  });
});
