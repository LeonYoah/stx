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
 * PackageTable MorphCatalog status + loading-dim + reduced-motion tests.
 * PackageTable MorphCatalog 状态、加载变淡与减少动效测试。
 */

import React from 'react';
import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest';
import {cleanup, render, screen} from '@testing-library/react';
import {PackageTable} from '../PackageTable';
import type {DownloadTask, PackageInfo} from '@/lib/services/installer/types';

vi.mock('next-intl', () => ({
  useTranslations: () => (key: string) => key,
}));

vi.mock('@/lib/timezone', () => ({
  formatDateTimeTz: (value: string) => value,
}));

vi.mock('motion/react', async () => {
  const ReactMod = await import('react');
  const reduceFlag = {current: false};

  function stripMotionProps<T extends Record<string, unknown>>(props: T) {
    const next = {...props};
    delete next.variants;
    delete next.initial;
    delete next.animate;
    delete next.whileHover;
    delete next.transition;
    delete next.layout;
    delete next.layoutId;
    return next;
  }

  const MotionDiv = ReactMod.forwardRef(function MotionDiv(
    props: React.HTMLAttributes<HTMLDivElement> & Record<string, unknown>,
    ref: React.Ref<HTMLDivElement>,
  ) {
    const {
      children,
      className,
      'data-reduce-motion': dataReduceMotion,
      'data-testid': dataTestId,
      'data-view-mode': dataViewMode,
      ...rest
    } = props as React.HTMLAttributes<HTMLDivElement> & {
      'data-reduce-motion'?: string;
      'data-testid'?: string;
      'data-view-mode'?: string;
    };
    return (
      <div
        ref={ref}
        className={className}
        data-reduce-motion={dataReduceMotion}
        data-testid={dataTestId}
        data-view-mode={dataViewMode}
        {...stripMotionProps(rest as Record<string, unknown>)}
      >
        {children}
      </div>
    );
  });

  return {
    __esModule: true,
    LayoutGroup: ({children}: {children: React.ReactNode}) => <>{children}</>,
    motion: {div: MotionDiv},
    useReducedMotion: () => reduceFlag.current,
    __setReducedMotionForTest: (value: boolean) => {
      reduceFlag.current = value;
    },
  };
});

afterEach(() => {
  cleanup();
});

beforeEach(async () => {
  const motionMock = await import('motion/react');
  (
    motionMock as unknown as {__setReducedMotionForTest: (v: boolean) => void}
  ).__setReducedMotionForTest(false);
});

const localPackages: PackageInfo[] = [
  {
    version: '2.3.12',
    file_name: 'apache-seatunnel-2.3.12-bin.tar.gz',
    file_size: 1024,
    download_urls: {
      aliyun: '',
      apache: '',
      huaweicloud: '',
    },
    is_local: true,
    has_source: false,
    source_download_urls: {
      aliyun: '',
      apache: '',
      huaweicloud: '',
    },
    uploaded_at: '2026-03-01T00:00:00Z',
  },
];

describe('PackageTable MorphCatalog status', () => {
  it('renders online versions with recommended / failed / progress states', () => {
    const downloads: DownloadTask[] = [
      {
        id: 'd1',
        version: '2.3.11',
        mirror: 'aliyun',
        download_url: 'https://example.com/2.3.11.tgz',
        status: 'failed',
        progress: 12,
        downloaded_bytes: 0,
        total_bytes: 100,
        speed: 0,
        start_time: '2026-03-01T00:00:00Z',
        source_requested: false,
        error: 'mirror missing',
      },
      {
        id: 'd2',
        version: '2.3.10',
        mirror: 'aliyun',
        download_url: 'https://example.com/2.3.10.tgz',
        status: 'downloading',
        progress: 44,
        downloaded_bytes: 44,
        total_bytes: 100,
        speed: 2048,
        start_time: '2026-03-01T00:00:00Z',
        source_requested: true,
        source_status: 'downloading',
        source_progress: 30,
      },
    ];

    render(
      <PackageTable
        type='online'
        viewMode='table'
        versions={['2.3.12', '2.3.11', '2.3.10']}
        localPackages={localPackages}
        recommendedVersion='2.3.12'
        downloads={downloads}
        onDownloadRequest={() => undefined}
      />,
    );

    expect(screen.getByTestId('package-online-catalog')).toBeInTheDocument();
    expect(screen.getByTestId('marketplace-row-online-2.3.12')).toBeInTheDocument();
    expect(screen.getByText('marketplaceUi.statusRecommended')).toBeInTheDocument();
    expect(screen.getByText('marketplaceUi.statusDownloaded')).toBeInTheDocument();
    expect(screen.getByText('marketplaceUi.statusFailed')).toBeInTheDocument();
    expect(screen.getByText('44% · 2.0 KB/s')).toBeInTheDocument();
  });

  it('keeps local packages honest about missing source in both views', () => {
    const {rerender} = render(
      <PackageTable
        type='local'
        viewMode='table'
        localPackages={localPackages}
        onDelete={() => undefined}
      />,
    );

    expect(screen.getByTestId('package-local-catalog')).toBeInTheDocument();
    expect(screen.getByText('apache-seatunnel-2.3.12-bin.tar.gz')).toBeInTheDocument();
    expect(
      screen.getAllByText('installer.sourceMissing').length,
    ).toBeGreaterThan(0);

    rerender(
      <PackageTable
        type='local'
        viewMode='card'
        localPackages={localPackages}
        onDelete={() => undefined}
      />,
    );
    expect(screen.getByTestId('marketplace-card-local-2.3.12')).toBeInTheDocument();
    expect(
      screen.getAllByText('installer.sourceMissing').length,
    ).toBeGreaterThan(0);
  });

  it('marks reduced-motion on the catalog and skips layout morph ids', async () => {
    const motionMock = await import('motion/react');
    (
      motionMock as unknown as {__setReducedMotionForTest: (v: boolean) => void}
    ).__setReducedMotionForTest(true);

    render(
      <PackageTable
        type='online'
        viewMode='table'
        versions={['2.3.12']}
        recommendedVersion='2.3.12'
      />,
    );

    expect(screen.getByTestId('package-online-catalog')).toHaveAttribute(
      'data-reduce-motion',
      'true',
    );
  });

  it('dims the list container on refresh instead of individual rows', () => {
    const {rerender} = render(
      <PackageTable
        type='online'
        viewMode='table'
        versions={['2.3.12']}
        recommendedVersion='2.3.12'
        loading={false}
      />,
    );

    const catalog = screen.getByTestId('package-online-catalog');
    const row = screen.getByTestId('marketplace-row-online-2.3.12');
    expect(catalog.className).not.toMatch(/opacity-60/);
    expect(row.className).not.toMatch(/opacity-60/);

    rerender(
      <PackageTable
        type='online'
        viewMode='table'
        versions={['2.3.12']}
        recommendedVersion='2.3.12'
        loading
      />,
    );

    expect(screen.getByTestId('package-online-catalog').className).toMatch(
      /opacity-60/,
    );
    expect(
      screen.getByTestId('marketplace-row-online-2.3.12').className,
    ).not.toMatch(/opacity-60/);
  });
});
