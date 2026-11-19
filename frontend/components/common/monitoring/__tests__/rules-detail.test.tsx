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

// 规则详情面板：继承展示、通知模式切换、critical 无启用外部渠道时禁止保存、例外增删。
// Rule detail panel: inheritance display, notify-mode switching, save blocked when critical has
// no enabled external channel, and override create/delete.

import {fireEvent, render, screen, waitFor} from '@testing-library/react';
import {NextIntlClientProvider} from 'next-intl';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import zh from '@/lib/i18n/locales/zh.json';
import type {ClusterRule} from '@/lib/services/monitoring';
import {RuleDetailPanel} from '../rules/RuleDetailPanel';
import {UnsavedChangesDialog} from '../dirty-close';
import {layerBody} from '../rules/RulesPanel';

const updateClusterRule = vi.fn();
const getClusterRule = vi.fn();
const updateClusterRuleOverride = vi.fn();
const deleteClusterRuleOverride = vi.fn();
const deleteClusterRule = vi.fn();

vi.mock('@/lib/services', () => ({
  default: {
    monitoring: {
      listNotificationChannels: vi.fn().mockResolvedValue({channels: []}),
      listNotifiableUsers: vi.fn().mockResolvedValue({users: []}),
      getAlertNotificationDefaults: vi.fn().mockResolvedValue(null),
      updateClusterRule: (...a: unknown[]) => updateClusterRule(...a),
      getClusterRule: (...a: unknown[]) => getClusterRule(...a),
      updateClusterRuleOverride: (...a: unknown[]) => updateClusterRuleOverride(...a),
      deleteClusterRuleOverride: (...a: unknown[]) => deleteClusterRuleOverride(...a),
      deleteClusterRule: (...a: unknown[]) => deleteClusterRule(...a),
    },
    cluster: {
      getClusters: vi.fn().mockResolvedValue({total: 1, clusters: [{id: 6, name: 'prod-st-1'}]}),
    },
  },
}));

const baseRule: ClusterRule = {
  key: 'node_offline',
  name: '节点离线',
  name_en: 'Node offline',
  description: 'desc',
  scope: 'cluster',
  default_enabled: true,
  enabled: true,
  severity: 'warning',
  threshold: null,
  for_seconds: 120,
  default_for_seconds: 120,
  for_editable: true,
  notify_mode: 'default',
  channel_ids: [],
  receiver_user_ids: [],
  has_global_config: true,
  global: {
    enabled: null,
    threshold: null,
    for_seconds: null,
    severity: 'critical',
    notify_mode: 'custom',
    channel_ids: [],
    receiver_user_ids: [],
  },
  overrides: [],
  override_count: 0,
  external_channel_missing: false,
};

function renderPanel(rule: ClusterRule, onSaved = vi.fn()) {
  return {
    onSaved,
    ...render(
      <NextIntlClientProvider locale='zh' messages={zh}>
        <RuleDetailPanel
          rule={rule}
          onClose={() => undefined}
          onSaved={onSaved}
          onToggleEnabled={() => undefined}
        />
      </NextIntlClientProvider>,
    ),
  };
}

beforeEach(() => vi.clearAllMocks());

describe('layerBody (whole-layer PUT)', () => {
  it('keeps raw nulls for inherited fields and applies the patch', () => {
    const body = layerBody(baseRule.global, {enabled: false});
    expect(body).toMatchObject({
      enabled: false,
      threshold: null,
      for_seconds: null,
      severity: 'critical',
      notify_mode: 'custom',
      channel_ids: null,
      receiver_user_ids: null,
    });
  });
});

describe('RuleDetailPanel', () => {
  it('shows raw global layer state: unset fields inherit via effective values', async () => {
    renderPanel(baseRule);
    // 全局层 enabled=null → 行内开关显示生效值 true。Raw enabled=null, effective switch shows true.
    const sw = await screen.findAllByRole('switch');
    expect(sw[0]).toHaveAttribute('aria-checked', 'true');
  });

  it('switching severity to critical reveals the notify-mode segmented control', async () => {
    renderPanel({...baseRule, severity: 'warning', global: {...baseRule.global, severity: 'warning'}});
    // SegmentedControl 用 button + aria-pressed，不是 radio。Buttons with aria-pressed, not radios.
    fireEvent.click(await screen.findByRole('button', {name: '严重'}));
    expect(await screen.findByRole('button', {name: '默认通知'})).toBeInTheDocument();
    expect(screen.getByRole('button', {name: '自定义'})).toBeInTheDocument();
  });

  it('custom notify mode shows channel and receiver multi-selects', async () => {
    renderPanel(baseRule); // severity critical, notify_mode custom
    const custom = await screen.findAllByRole('button', {name: '自定义'});
    fireEvent.click(custom[0]);
    expect(await screen.findAllByText('通知渠道')).not.toHaveLength(0);
    expect(await screen.findAllByText('接收人')).not.toHaveLength(0);
  });

  it('blocks save when critical + custom has no enabled channel or receiver', async () => {
    renderPanel(baseRule); // critical + custom, channel_ids empty, no enabled channels
    const save = await screen.findByRole('button', {name: '保存'});
    await waitFor(() => expect(save).toBeDisabled());
    fireEvent.click(save);
    expect(updateClusterRule).not.toHaveBeenCalled();
  });

  it('saves the global layer with whole-layer replacement', async () => {
    updateClusterRule.mockResolvedValue({...baseRule, enabled: false});
    const onSaved = vi.fn();
    renderPanel({...baseRule, severity: 'warning', global: {...baseRule.global, severity: 'warning', notify_mode: 'default'}}, onSaved);
    fireEvent.click(await screen.findByRole('button', {name: '保存'}));
    await waitFor(() => expect(updateClusterRule).toHaveBeenCalledTimes(1));
    const [key, body] = updateClusterRule.mock.calls[0];
    expect(key).toBe('node_offline');
    expect(body.severity).toBe('warning');
    expect(body.notify_mode).toBe('default');
    expect(onSaved).toHaveBeenCalled();
  });

  it('creates an override with an all-inherit layer for a picked cluster', async () => {
    updateClusterRuleOverride.mockResolvedValue(baseRule);
    renderPanel(baseRule);
    fireEvent.click(await screen.findByRole('button', {name: '添加集群例外'}));
    // Radix Select 的 trigger 也占用 combobox 角色，取真正的 input。
    // The Radix Select trigger also claims the combobox role; pick the real input.
    const input = await screen.findAllByRole('combobox').then((els) => els.find((el) => el.tagName === 'INPUT')!);
    fireEvent.focus(input);
    fireEvent.change(input, {target: {value: 'prod'}});
    const option = await screen.findByRole('option', {name: /prod-st-1/});
    fireEvent.click(option);
    await waitFor(() =>
      expect(updateClusterRuleOverride).toHaveBeenCalledWith('node_offline', 6, {}),
    );
  });

  it('deletes an override through confirm-morph', async () => {
    const withOverride: ClusterRule = {
      ...baseRule,
      override_count: 1,
      overrides: [
        {
          cluster_id: 6,
          cluster_name: 'prod-st-1',
          enabled: false,
          threshold: null,
          for_seconds: 300,
          severity: null,
          notify_mode: null,
          channel_ids: [],
          receiver_user_ids: [],
          effective: {enabled: false, severity: 'critical', threshold: null, for_seconds: 300, external_channel_missing: false},
        },
      ],
    };
    deleteClusterRuleOverride.mockResolvedValue({key: 'node_offline'});
    getClusterRule.mockResolvedValue(withOverride);
    renderPanel(withOverride);
    // accordion 默认展开第一项：删除按钮可见，按两次（armed→confirm）。
    const del = await screen.findByRole('button', {name: '删除例外'});
    fireEvent.click(del);
    // ConfirmMorph 有 280ms 防误触间隔，等确认键真正可用再按。
    // ConfirmMorph arms for 280ms; wait past the arm delay before confirming.
    const confirm = await screen.findByRole('button', {name: '删除'});
    await new Promise((resolve) => setTimeout(resolve, 320));
    fireEvent.click(confirm);
    await waitFor(() =>
      expect(deleteClusterRuleOverride).toHaveBeenCalledWith('node_offline', 6),
    );
  });
});

describe('dirty-close interception', () => {
  it('reports dirty when the global draft diverges from the open snapshot', async () => {
    const onDirtyChange = vi.fn();
    render(
      <NextIntlClientProvider locale='zh' messages={zh}>
        <RuleDetailPanel
          rule={baseRule}
          onClose={() => undefined}
          onSaved={() => undefined}
          onToggleEnabled={() => undefined}
          onDirtyChange={onDirtyChange}
        />
      </NextIntlClientProvider>,
    );
    // 初始快照等于草稿：先报 false。Initial draft equals snapshot → not dirty.
    await waitFor(() => expect(onDirtyChange).toHaveBeenLastCalledWith(false));
    // 改级别（critical→warning）让草稿偏离快照。Severity change makes the draft dirty.
    fireEvent.click(await screen.findByRole('button', {name: '警告'}));
    await waitFor(() => expect(onDirtyChange).toHaveBeenLastCalledWith(true));
  });

  it('opens a centered AlertDialog with initial focus on Keep editing', async () => {
    const onDiscard = vi.fn();
    const onKeepEditing = vi.fn();
    render(
      <NextIntlClientProvider locale='zh' messages={zh}>
        <UnsavedChangesDialog
          open
          onDiscard={onDiscard}
          onKeepEditing={onKeepEditing}
        />
      </NextIntlClientProvider>,
    );
    expect(await screen.findByText('放弃未保存的修改？')).toBeInTheDocument();
    const keepBtn = screen.getByRole('button', {name: '继续编辑'});
    const discardBtn = screen.getByRole('button', {name: '放弃修改'});
    await waitFor(() => expect(keepBtn).toHaveFocus());
    fireEvent.click(keepBtn);
    expect(onKeepEditing).toHaveBeenCalledTimes(1);
    fireEvent.click(discardBtn);
    expect(onDiscard).toHaveBeenCalledTimes(1);
  });

  it('a successful save resets the snapshot so dirty clears', async () => {
    updateClusterRule.mockResolvedValue(baseRule);
    const onDirtyChange = vi.fn();
    render(
      <NextIntlClientProvider locale='zh' messages={zh}>
        <RuleDetailPanel
          rule={baseRule}
          onClose={() => undefined}
          onSaved={() => undefined}
          onToggleEnabled={() => undefined}
          onDirtyChange={onDirtyChange}
        />
      </NextIntlClientProvider>,
    );
    fireEvent.click(await screen.findByRole('button', {name: '警告'}));
    await waitFor(() => expect(onDirtyChange).toHaveBeenLastCalledWith(true));
    fireEvent.click(screen.getByRole('button', {name: '保存'}));
    await waitFor(() => expect(updateClusterRule).toHaveBeenCalled());
    await waitFor(() => expect(onDirtyChange).toHaveBeenLastCalledWith(false));
  });
});
