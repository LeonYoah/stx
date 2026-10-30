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

import {fireEvent, render, screen, waitFor} from '@testing-library/react';
import {NextIntlClientProvider} from 'next-intl';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import zh from '@/lib/i18n/locales/zh.json';
import {TooltipProvider} from '@/components/ui/tooltip';
import {SettingsSidebarPanel} from '../SettingsSidebarPanel';

function renderPanel(props: {
  submitTimeoutMinutes?: string;
  stopTimeoutMinutes?: string;
}) {
  const onTaskTimeoutChange = vi.fn();
  render(
    <NextIntlClientProvider locale='zh' messages={zh}>
      <TooltipProvider>
        <SettingsSidebarPanel
          executionMode='cluster'
          observabilityMode='auto'
          onObservabilityModeChange={() => undefined}
          submitTimeoutMinutes={props.submitTimeoutMinutes ?? ''}
          stopTimeoutMinutes={props.stopTimeoutMinutes ?? ''}
          timeoutDefaults={{submit_timeout_seconds: 300, stop_timeout_seconds: 300}}
          onTaskTimeoutChange={onTaskTimeoutChange}
          clusterId='1'
          clusters={[{id: 1, name: 'c1'} as never]}
          pluginPanelLoading={false}
          pluginTemplatePendingType={null}
          pluginTemplateLoadingText={null}
          sourceTemplateItems={[]}
          transformTemplateItems={[]}
          sinkTemplateItems={[]}
          detectedVariables={[]}
          customVariableRows={[]}
          onExecutionModeChange={() => undefined}
          onClusterChange={() => undefined}
          onInsertPluginTemplate={() => undefined}
          onInsertCuratedTemplate={() => undefined}
          onOpenCreateCustomVariable={() => undefined}
          onOpenEditCustomVariable={() => undefined}
          onDeleteCustomVariable={() => undefined}
          onCopyCustomVariableReference={() => undefined}
          onCopyCustomVariableValue={() => undefined}
        />
      </TooltipProvider>
    </NextIntlClientProvider>,
  );
  return {onTaskTimeoutChange};
}

describe('TimeoutSettingsSection', () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it('defaults to collapsed with a default summary', async () => {
    renderPanel({});
    const trigger = await screen.findByTestId('sync-timeout-section-trigger');
    expect(trigger.textContent).toMatch(/默认/);
    expect(trigger.textContent).toMatch(/提交 5 分钟/);
    expect(screen.getByTestId('sync-submit-timeout-minutes')).toBeInTheDocument();
    expect(screen.getByTestId('sync-stop-timeout-minutes')).toBeInTheDocument();
  });

  it('shows a custom badge when a timeout is set', async () => {
    renderPanel({submitTimeoutMinutes: '10'});
    const trigger = await screen.findByTestId('sync-timeout-section-trigger');
    expect(trigger.textContent).toMatch(/自定义/);
    expect(trigger.textContent).toMatch(/10/);
  });

  it('force-expands when input is invalid', async () => {
    renderPanel({});
    const trigger = await screen.findByTestId('sync-timeout-section-trigger');
    fireEvent.click(trigger);
    const input = screen.getByTestId('sync-submit-timeout-minutes');
    fireEvent.change(input, {target: {value: 'abc'}});
    await waitFor(() => {
      expect(screen.getByText(/请输入 1~120/)).toBeInTheDocument();
    });
    fireEvent.click(trigger);
    expect(screen.getByText(/请输入 1~120/)).toBeInTheDocument();
  });
});
