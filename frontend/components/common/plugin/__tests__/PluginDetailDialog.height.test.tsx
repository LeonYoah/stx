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

/**
 * PluginDetailDialog fixed-height shell (#129 AC8): tab switches must not resize;
 * body scroll resets to top.
 * 插件详情弹窗固定高度（#129 AC8）：切 Tab 尺寸不变；内容区滚动复位到顶部。
 */

import React from 'react';
import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest';
import {cleanup, fireEvent, render, screen, waitFor} from '@testing-library/react';
import type {Plugin} from '@/lib/services/plugin';

vi.mock('next-intl', () => ({
  useTranslations: () => (key: string) => key,
}));

vi.mock('@/lib/i18n', () => ({
  useLocale: () => ({locale: 'zh-CN'}),
}));

vi.mock('@/lib/services/plugin', () => ({
  PluginService: {
    getOfficialDependencies: vi.fn(async () => ({
      dependency_status: 'ready',
      profiles: [],
      dependencies: [
        {
          group_id: 'org.example',
          artifact_id: 'driver-a',
          version: '1.0.0',
          target_dir: 'lib',
        },
      ],
      disabled_dependencies: [],
    })),
    disableOfficialDependency: vi.fn(),
    enableOfficialDependency: vi.fn(),
  },
}));

vi.mock('@/lib/services/kapa-ai', () => ({
  openSeaTunnelAskAi: vi.fn(),
  isKapaAskAiAllowed: () => false,
}));

vi.mock('../connector-quick-helper', () => ({
  generateConnectorDocUrls: () => ({
    sourceDocUrl: 'https://example.com/source',
    sinkDocUrl: 'https://example.com/sink',
  }),
  getConnectorCapability: () => ({
    supportsSource: true,
    supportsSink: true,
    isTransform: false,
  }),
  getConnectorSemanticIcon: () => ({
    icon: (props: React.SVGProps<SVGSVGElement>) => (
      <svg data-testid='connector-icon' {...props} />
    ),
    bgColor: 'bg-sky-50',
    textColor: 'text-sky-700',
    iconUrl: undefined,
    tagLabel: undefined,
  }),
}));

vi.mock('../dependency-status', () => ({
  getPluginDependencyStatusMeta: () => ({
    label: 'ready',
    description: 'ready-desc',
    className: 'border-emerald-200',
  }),
}));

vi.mock('../DependencyConfigDialog', () => ({
  PluginDependencyConfigSection: () => (
    <div data-testid='plugin-custom-deps-stub'>custom</div>
  ),
}));

vi.mock('sonner', () => ({
  toast: {success: vi.fn(), error: vi.fn(), info: vi.fn()},
}));

import {PluginDetailDialog} from '../PluginDetailDialog';

const plugin: Plugin = {
  name: 'kafka',
  display_name: 'Kafka',
  category: 'connector',
  version: '2.3.12',
  description: 'Kafka connector',
  group_id: 'org.apache.seatunnel',
  artifact_id: 'connector-kafka',
  dependencies: [],
};

/** Radix Tabs activate on mouseDown (button 0), not click alone. / Radix Tabs 在 mouseDown 时激活。 */
function activateTab(testId: string) {
  const tab = screen.getByTestId(testId);
  fireEvent.mouseDown(tab, {button: 0});
  fireEvent.click(tab);
}

describe('PluginDetailDialog fixed height (#129 AC8)', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    cleanup();
  });

  it('uses a fixed dialog height class and keeps it after switching tabs', async () => {
    render(
      <PluginDetailDialog open onOpenChange={() => undefined} plugin={plugin} />,
    );

    const dialog = await screen.findByTestId('plugin-detail-dialog-kafka');
    const fixedHeightClass = 'h-[min(85vh,720px)]';
    expect(dialog.className).toContain(fixedHeightClass);
    expect(dialog.className).toContain('max-h-[min(85vh,720px)]');
    expect(dialog.className).not.toContain('min-h-[240px]');
    expect(dialog.className).toContain('flex-col');
    expect(dialog.className).toContain('overflow-hidden');

    const classBefore = dialog.className;
    activateTab('plugin-detail-tab-dependencies');
    await waitFor(() => {
      expect(
        screen.getByTestId('plugin-official-dependencies'),
      ).toBeInTheDocument();
    });
    expect(dialog.className).toBe(classBefore);
    expect(dialog.className).toContain(fixedHeightClass);

    activateTab('plugin-detail-tab-overview');
    await waitFor(() => {
      expect(
        screen.getByTestId('plugin-detail-overview-panel'),
      ).toBeInTheDocument();
    });
    expect(dialog.className).toBe(classBefore);
  });

  it('scrolls the body container and resets scrollTop on tab change', async () => {
    render(
      <PluginDetailDialog open onOpenChange={() => undefined} plugin={plugin} />,
    );

    const body = await screen.findByTestId('plugin-detail-body-scroll');
    expect(body.className).toContain('flex-1');
    expect(body.className).toContain('min-h-0');
    expect(body.className).toContain('overflow-y-auto');

    let scrollTopValue = 180;
    Object.defineProperty(body, 'scrollTop', {
      configurable: true,
      get: () => scrollTopValue,
      set: (next: number) => {
        scrollTopValue = next;
      },
    });
    expect(body.scrollTop).toBe(180);

    activateTab('plugin-detail-tab-dependencies');
    await waitFor(() => {
      expect(
        screen.getByTestId('plugin-official-dependencies'),
      ).toBeInTheDocument();
      expect(body.scrollTop).toBe(0);
    });

    scrollTopValue = 90;
    activateTab('plugin-detail-tab-custom');
    await waitFor(() => {
      expect(screen.getByTestId('plugin-custom-deps-stub')).toBeInTheDocument();
      expect(body.scrollTop).toBe(0);
    });
  });

  it('keeps overview content top-aligned without reclaiming the old min-height hug', async () => {
    render(
      <PluginDetailDialog open onOpenChange={() => undefined} plugin={plugin} />,
    );

    const overview = await screen.findByTestId('plugin-detail-overview-panel');
    expect(overview.className).toContain('justify-start');
    expect(overview.className).toContain('min-h-full');
    expect(overview.className).toContain('flex-col');
  });
});
