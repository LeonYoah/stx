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

import {render, screen} from '@testing-library/react';
import {describe, expect, it} from 'vitest';
import {DashboardPanel} from '../DashboardPanel';

describe('DashboardPanel', () => {
  it('renders the title, hint, actions and children on the same root testid', () => {
    render(
      <DashboardPanel
        title='全局吞吐'
        hint='所有作业合计'
        actions={<button type='button'>切换</button>}
        testId='home-demo'
      >
        <div data-testid='chart'>chart</div>
      </DashboardPanel>,
    );
    const panel = screen.getByTestId('home-demo');
    expect(panel).toHaveTextContent('全局吞吐');
    expect(panel).toHaveTextContent('所有作业合计');
    expect(screen.getByTestId('chart')).toBeInTheDocument();
    expect(
      screen.getByRole('button', {name: '切换'}),
    ).toBeInTheDocument();
  });

  it('collapses into a one-line strip when empty, hiding actions and children', () => {
    render(
      <DashboardPanel
        title='写入速率 Top 8'
        hint='窗口内平均'
        empty
        emptyLabel='该时间窗内没有数据'
        actions={<button type='button'>切换</button>}
        testId='home-top-write'
      >
        <div data-testid='chart'>chart</div>
      </DashboardPanel>,
    );
    const panel = screen.getByTestId('home-top-write');
    expect(panel).toHaveTextContent('写入速率 Top 8');
    expect(panel).toHaveTextContent('该时间窗内没有数据');
    expect(screen.queryByTestId('chart')).not.toBeInTheDocument();
    expect(
      screen.queryByRole('button', {name: '切换'}),
    ).not.toBeInTheDocument();
    // 矮条：h-11 = 44px，低于 48px 上限。/ Strip: h-11 = 44px, under the 48px cap.
    expect(panel.className).toContain('h-11');
  });
});
