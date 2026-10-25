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

// 伸缩计划视图：阻断 / 提醒分级展示，回环重启与作业风险必须显式勾选确认。
// Scaling plan view: blocking / warning tiers, and loopback restart and job risk need explicit acknowledgement.

import {fireEvent, render, screen} from '@testing-library/react';
import {NextIntlClientProvider} from 'next-intl';
import {describe, expect, it, vi} from 'vitest';
import zh from '@/lib/i18n/locales/zh.json';
import type {ScalePlan} from '@/lib/services/cluster';
import {ScalePlanView} from '../ScalePlanView';

function basePlan(overrides: Partial<ScalePlan> = {}): ScalePlan {
  return {
    kind: 'scale_out',
    mode: 'install',
    detected_mode: 'install',
    cluster_id: 1,
    deployment_mode: 'hybrid' as ScalePlan['deployment_mode'],
    cluster_version: '3.0.0',
    target: {
      host_id: 2,
      host_name: 't2',
      host_ip: '10.66.0.1',
      role: 'master/worker' as ScalePlan['target']['role'],
      install_dir: '/opt/seatunnel',
      hazelcast_port: 5801,
      api_port: 8080,
      worker_port: 5802,
      address: '10.66.0.1:5801',
    },
    fingerprint: 'sha256:abc',
    members_before: ['10.66.0.2:5801'],
    members_after: ['10.66.0.1:5801', '10.66.0.2:5801'],
    checks: [],
    loopback_nodes: [],
    requires_restart_ack: false,
    plugins_to_replay: [],
    running_jobs: [],
    running_jobs_known: true,
    requires_jobs_ack: false,
    checkpoint_storage: null,
    java_proxy_follow: null,
    steps: ['validate_plan', 'install_files'],
    can_submit: true,
    blocking_reasons: [],
    warnings: [],
    ...overrides,
  };
}

function renderView(ui: React.ReactElement) {
  return render(
    <NextIntlClientProvider locale='zh' messages={zh}>
      {ui}
    </NextIntlClientProvider>,
  );
}

describe('ScalePlanView', () => {
  it('分级展示阻断与提醒，并显示成员变化 / shows blocking and warning tiers and member changes', () => {
    renderView(
      <ScalePlanView
        plan={basePlan({
          checks: [
            {
              name: 'java',
              status: 'failed',
              blocking: true,
              message: 'java not found',
            },
            {
              name: 'disk_space',
              status: 'warning',
              blocking: false,
              message: 'disk 92%',
            },
            {
              name: 'agent_online',
              status: 'passed',
              blocking: true,
              message: 'ok',
            },
          ],
        })}
      />,
    );

    expect(screen.getByText('1 项阻断')).toBeInTheDocument();
    expect(screen.getByText('java not found')).toBeInTheDocument();
    expect(screen.getByText('1 项提醒')).toBeInTheDocument();
    expect(screen.getByText('1 项通过')).toBeInTheDocument();
    expect(screen.getAllByText('10.66.0.1:5801').length).toBeGreaterThan(0);
  });

  it('回环节点需要勾选重启确认 / loopback nodes require the restart acknowledgement', () => {
    const onAck = vi.fn();
    renderView(
      <ScalePlanView
        plan={basePlan({
          requires_restart_ack: true,
          loopback_nodes: [
            {
              node_id: 1,
              host_id: 1,
              host_name: 't1',
              address: '127.0.0.1:5801',
              source: 'config',
            },
          ],
        })}
        restartAck={false}
        onRestartAckChange={onAck}
      />,
    );

    expect(
      screen.getByText('需要先修正 1 个节点的成员地址'),
    ).toBeInTheDocument();
    fireEvent.click(screen.getByRole('checkbox'));
    expect(onAck).toHaveBeenCalledWith(true);
  });

  it('本地 checkpoint 存储时给出风险提示并要求作业确认 / local checkpoint storage shows the risk and needs a jobs ack', () => {
    const onJobsAck = vi.fn();
    renderView(
      <ScalePlanView
        plan={basePlan({
          kind: 'scale_in',
          requires_jobs_ack: true,
          running_jobs: [{job_id: '7', job_name: 'cdc-orders'}],
          checkpoint_storage: {
            type: 'hdfs',
            location: 'file:///tmp/',
            local: true,
          },
        })}
        jobsAck={false}
        onJobsAckChange={onJobsAck}
      />,
    );

    expect(screen.getByText('cdc-orders')).toBeInTheDocument();
    expect(
      screen.getByText(/Checkpoint 存储为本地文件（file:\/\/\/tmp\/）/),
    ).toBeInTheDocument();
    fireEvent.click(screen.getByRole('checkbox'));
    expect(onJobsAck).toHaveBeenCalledWith(true);
  });

  it('追加用户选择的步骤到预览 / appends user-chosen steps to the preview', () => {
    renderView(
      <ScalePlanView plan={basePlan()} extraSteps={['rolling_restart']} />,
    );
    expect(screen.getByText('滚动重启现有节点')).toBeInTheDocument();
  });
});
