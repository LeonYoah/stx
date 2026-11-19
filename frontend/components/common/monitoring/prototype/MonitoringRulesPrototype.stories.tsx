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

import type {Meta, StoryObj} from '@storybook/nextjs-vite';
import {AlertsPanel} from './AlertsPanel';
import {MonitoringPagePrototype} from './MonitoringPagePrototype';
import {MonitoringRulesPrototype} from './MonitoringRulesPrototype';
import {NotifyPanel} from './NotifyPanel';
import {PROTO_CLUSTER_RULES} from './rules-mock';

const meta = {
  title: 'Monitoring/Rules prototype',
  component: MonitoringRulesPrototype,
  args: {
    rules: PROTO_CLUSTER_RULES,
  },
} satisfies Meta<typeof MonitoringRulesPrototype>;

export default meta;
type Story = StoryObj<typeof meta>;

/** 默认：网格 + 常驻详情面板（主从布局）。 */
export const WithDetailPanel: Story = {
  args: {initialActiveKey: 'memory_usage_high'},
};

/** 仅网格：不预选规则，看纯卡片墙。 */
export const GridOnly: Story = {};

/** 批量选择：预勾选三张卡，看底部批量操作条与撤销。 */
export const BatchSelection: Story = {
  args: {
    initialSelectedKeys: ['cpu_usage_high', 'gc_time_high', 'failed_jobs_high'],
  },
};

/** 默认通知缺失：顶部提示条。 */
export const MissingDefaultNotify: Story = {
  args: {defaultNotifyMissing: true, initialActiveKey: 'split_brain_risk'},
};

/** 告警面板：指标行 + 筛选 + 列表 + 详情抽屉。 */
export const Alerts: StoryObj<typeof AlertsPanel> = {
  render: () => <div className='h-[calc(100vh-2rem)] p-4'><AlertsPanel /></div>,
};

/** 通知面板：默认通知卡 + 渠道表 + 编辑抽屉。 */
export const Notify: StoryObj<typeof NotifyPanel> = {
  render: () => <div className='h-[calc(100vh-2rem)] p-4'><NotifyPanel /></div>,
};

/** 整页：告警 / 规则 / 通知 三个同级面板，告警详情可跳规则。 */
export const FullPage: StoryObj<typeof MonitoringPagePrototype> = {
  render: () => <MonitoringPagePrototype />,
};
