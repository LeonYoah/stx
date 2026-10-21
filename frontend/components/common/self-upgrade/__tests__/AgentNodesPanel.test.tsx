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

import React from 'react';
import {describe, expect, it, vi} from 'vitest';
import {render, screen} from '@testing-library/react';
import {AgentNodeList, AgentVersionsPanel} from '../AgentNodesPanel';

vi.mock('next-intl', () => ({
  useTranslations: () => (key: string, values?: Record<string, unknown>) =>
    values ? `${key}:${JSON.stringify(values)}` : key,
}));

describe('AgentNodeList', () => {
  it('renders each node with its status, versions and proxy result', () => {
    render(
      <AgentNodeList
        nodes={[
          {
            host_id: 1,
            host_name: 'worker-1',
            ip_address: '10.0.0.1',
            from_version: '1.0.0',
            current_version: '1.0.1',
            target_version: '1.0.1',
            status: 'succeeded',
            proxy_status: 'updated',
            proxy_message: '2 jar(s) updated',
          },
          {
            host_id: 2,
            host_name: 'worker-2',
            from_version: '1.0.0',
            current_version: '1.0.0',
            target_version: '1.0.1',
            status: 'offline',
            message: 'agent offline',
          },
        ]}
      />,
    );
    expect(screen.getByText('worker-1')).toBeTruthy();
    expect(screen.getByText('1.0.0 → 1.0.1')).toBeTruthy();
    expect(screen.getByText('nodeSucceeded')).toBeTruthy();
    expect(screen.getByText('nodeOffline')).toBeTruthy();
    expect(screen.getByText('Proxy proxyUpdated')).toBeTruthy();
    expect(screen.getByText('agent offline')).toBeTruthy();
    expect(
      screen.getByText(
        'agentNodesSummary:{"succeeded":1,"total":2,"failed":0,"offline":1}',
      ),
    ).toBeTruthy();
  });
});

describe('AgentVersionsPanel', () => {
  it('flags outdated agents', () => {
    render(
      <AgentVersionsPanel
        controlPlaneVersion='1.0.1'
        outdated={1}
        agents={[
          {
            host_id: 1,
            host_name: 'w1',
            version: '1.0.1',
            online: true,
            up_to_date: true,
          },
          {
            host_id: 2,
            host_name: 'w2',
            version: '1.0.0',
            online: false,
            up_to_date: false,
          },
        ]}
      />,
    );
    expect(
      screen.getByText('agentsOutdated:{"count":1,"version":"1.0.1"}'),
    ).toBeTruthy();
    expect(screen.getByText('1.0.0')).toBeTruthy();
  });

  it('renders nothing without agents', () => {
    const {container} = render(
      <AgentVersionsPanel
        controlPlaneVersion='1.0.1'
        outdated={0}
        agents={[]}
      />,
    );
    expect(container.innerHTML).toBe('');
  });
});
