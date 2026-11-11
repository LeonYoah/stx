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
 * PluginCard state visibility and reduced-motion presentation tests.
 * PluginCard 状态可见性与减少动态效果呈现测试。
 */

import React from 'react';
import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest';
import {cleanup, render, screen} from '@testing-library/react';
import {PluginCard} from '../PluginCard';
import type {Plugin} from '@/lib/services/plugin';

vi.mock('next-intl', () => ({
  useTranslations: () => (key: string) => key,
}));

vi.mock('@/lib/i18n', () => ({
  useLocale: () => ({locale: 'zh-CN'}),
}));

vi.mock('@/lib/seatunnel-version', () => ({
  buildSeatunnelConnectorDocsUrl: () => 'https://example.com/docs',
}));

vi.mock('../connector-quick-helper', () => ({
  generateConnectorDocUrls: () => ({
    sourceDocUrl: '',
    sinkDocUrl: '',
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
    label: 'plugin.dependencyStatus.ready',
    description: 'ready-desc',
    className: 'border-emerald-200',
  }),
}));

vi.mock('motion/react', async () => {
  const ReactMod = await import('react');
  const reduceFlag = {current: false};
  const MotionDiv = ReactMod.forwardRef(function MotionDiv(
    props: React.HTMLAttributes<HTMLDivElement> & {
      'data-reduce-motion'?: string;
      variants?: unknown;
      whileHover?: unknown;
    },
    ref: React.Ref<HTMLDivElement>,
  ) {
    const {children, className, 'data-reduce-motion': dataReduceMotion} = props;
    return (
      <div
        ref={ref}
        className={className}
        data-reduce-motion={dataReduceMotion}
        data-testid='plugin-card-motion-shell'
      >
        {children}
      </div>
    );
  });

  return {
    __esModule: true,
    motion: {div: MotionDiv},
    useReducedMotion: () => reduceFlag.current,
    __setReducedMotionForTest: (value: boolean) => {
      reduceFlag.current = value;
    },
  };
});

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

afterEach(() => {
  cleanup();
});

beforeEach(async () => {
  const motionMock = await import('motion/react');
  (
    motionMock as unknown as {__setReducedMotionForTest: (v: boolean) => void}
  ).__setReducedMotionForTest(false);
});

describe('PluginCard motion and states', () => {
  it('keeps dependency and download progress readable without changing card fields', () => {
    render(
      <PluginCard
        plugin={plugin}
        onClick={() => undefined}
        showInstallButton
        isDownloading
        downloadProgress={42}
      />,
    );

    expect(screen.getByText('plugin.officialDependencyStatus')).toBeInTheDocument();
    expect(screen.getByText('plugin.dependencyStatus.ready')).toBeInTheDocument();
    expect(screen.getByText('plugin.downloading')).toBeInTheDocument();
    expect(screen.getByText('42%')).toBeInTheDocument();
    expect(screen.getByTestId('connector-icon')).toBeInTheDocument();
  });

  it('surfaces download failure on the available card with production wording', () => {
    render(
      <PluginCard
        plugin={plugin}
        onClick={() => undefined}
        showInstallButton
        downloadFailedMessage='mirror missing jar'
      />,
    );

    expect(screen.getByText('plugin.downloadFailed')).toBeInTheDocument();
    expect(screen.getByText('mirror missing jar')).toBeInTheDocument();
    expect(screen.getByText('plugin.download')).toBeInTheDocument();
  });

  it('shows a readable install-count badge without locking download actions', () => {
    render(
      <PluginCard
        plugin={plugin}
        onClick={() => undefined}
        showInstallButton
        installedClusterCount={3}
      />,
    );

    expect(
      screen.getByTestId('plugin-card-install-count-kafka'),
    ).toHaveTextContent('plugin.installed · 3');
    // 安装量可读，但仍保留下载入口。 / Count is readable while download remains available.
    expect(screen.getByText('plugin.download')).toBeInTheDocument();
  });

  it('marks reduced-motion on the motion shell', async () => {
    const motionMock = await import('motion/react');
    (
      motionMock as unknown as {__setReducedMotionForTest: (v: boolean) => void}
    ).__setReducedMotionForTest(true);

    render(
      <PluginCard plugin={plugin} onClick={() => undefined} showInstallButton />,
    );

    expect(screen.getByTestId('plugin-card-motion-shell')).toHaveAttribute(
      'data-reduce-motion',
      'true',
    );
  });
});
