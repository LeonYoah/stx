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

// 推到已有工作流前必须告知：上下线行为、工作流定时（含暂停）、STX cron 不沿用、无定时不会自动运行。
// Before pushing into an existing workflow the user must learn: the offline/online cycle, the workflow schedules
// (including paused ones), that the STX cron is not carried over, and that no schedule means no automatic runs.

import {render, screen} from '@testing-library/react';
import {NextIntlClientProvider} from 'next-intl';
import {describe, expect, it} from 'vitest';
import zh from '@/lib/i18n/locales/zh.json';
import type {DolphinWorkflowDetail} from '@/lib/services/sync/types';
import {DolphinScheduleNotice} from '../DolphinScheduleNotice';

function workflow(
  overrides: Partial<DolphinWorkflowDetail> = {},
): DolphinWorkflowDetail {
  return {
    code: 9,
    name: 'wf',
    version: 3,
    release_state: 'ONLINE',
    tasks: [],
    relations: [],
    schedules: [],
    ...overrides,
  };
}

function renderNotice(ui: React.ReactElement) {
  return render(
    <NextIntlClientProvider locale='zh' messages={zh}>
      {ui}
    </NextIntlClientProvider>,
  );
}

describe('DolphinScheduleNotice', () => {
  it('在线工作流：说明先下线再上线，列出启用与暂停的定时，提示 cron 不沿用', () => {
    renderNotice(
      <DolphinScheduleNotice
        workflow={workflow({
          schedules: [
            {
              id: 11,
              crontab: '0 0 2 * * ? *',
              timezone: 'Asia/Shanghai',
              release_state: 'ONLINE',
            },
            {
              id: 12,
              crontab: '0 30 3 * * ? *',
              timezone: 'Asia/Shanghai',
              release_state: 'OFFLINE',
            },
          ],
        })}
        builtinSchedule={{
          cron_expr: '0 1 * * *',
          timezone: 'Asia/Shanghai',
          enabled: true,
        }}
      />,
    );

    expect(
      screen.getByText(/推送时会先下线、写入后再上线/),
    ).toBeInTheDocument();
    expect(screen.getByText('0 0 2 * * ? *')).toBeInTheDocument();
    expect(screen.getByText('启用')).toBeInTheDocument();
    expect(screen.getByText('已暂停')).toBeInTheDocument();
    expect(
      screen.getByText(/STX 内置定时（0 1 \* \* \*）不会沿用/),
    ).toBeInTheDocument();
    expect(
      screen.queryByText(/推送后任务不会自动运行/),
    ).not.toBeInTheDocument();
  });

  it('下线工作流：说明保持下线', () => {
    renderNotice(
      <DolphinScheduleNotice workflow={workflow({release_state: 'OFFLINE'})} />,
    );
    expect(
      screen.getByText(/推送后保持下线，不会自动上线/),
    ).toBeInTheDocument();
  });

  it('工作流没有定时：警告推送后不会自动运行', () => {
    renderNotice(
      <DolphinScheduleNotice workflow={workflow({schedules: []})} />,
    );
    expect(screen.getByText('该工作流没有配置定时')).toBeInTheDocument();
    expect(screen.getByText(/推送后任务不会自动运行/)).toBeInTheDocument();
  });

  it('定时读取失败：展示原因而不是「没有定时」', () => {
    renderNotice(
      <DolphinScheduleNotice
        workflow={workflow({schedules: [], schedule_error: 'timeout'})}
      />,
    );
    expect(screen.getByText(/无法读取工作流定时：timeout/)).toBeInTheDocument();
    expect(screen.queryByText('该工作流没有配置定时')).not.toBeInTheDocument();
  });
});
