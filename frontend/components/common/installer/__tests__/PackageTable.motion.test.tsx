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
 * PackageTable row motion + status visibility tests (table-only, no card wall).
 * PackageTable 行动效与状态可见性测试（仅表格，无卡片墙）。
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
    return next;
  }

  const MotionTr = ReactMod.forwardRef(function MotionTr(
    props: React.HTMLAttributes<HTMLTableRowElement> & {
      variants?: unknown;
      initial?: unknown;
      animate?: unknown;
      whileHover?: unknown;
      'data-reduce-motion'?: string;
      'data-testid'?: string;
    },
    ref: React.Ref<HTMLTableRowElement>,
  ) {
    const {
      children,
      className,
      'data-reduce-motion': dataReduceMotion,
      'data-testid': dataTestId,
      ...rest
    } = props;
    return (
      <tr
        ref={ref}
        className={className}
        data-reduce-motion={dataReduceMotion}
        data-testid={dataTestId}
        {...stripMotionProps(rest as Record<string, unknown>)}
      >
        {children}
      </tr>
    );
  });

  const MotionTbody = ReactMod.forwardRef(function MotionTbody(
    props: React.HTMLAttributes<HTMLTableSectionElement> & {
      variants?: unknown;
      initial?: unknown;
      animate?: unknown;
      'data-testid'?: string;
    },
    ref: React.Ref<HTMLTableSectionElement>,
  ) {
    const {children, className, 'data-testid': dataTestId, ...rest} = props;
    return (
      <tbody
        ref={ref}
        className={className}
        data-testid={dataTestId}
        {...stripMotionProps(rest as Record<string, unknown>)}
      >
        {children}
      </tbody>
    );
  });

  return {
    __esModule: true,
    motion: {
      tr: MotionTr,
      tbody: MotionTbody,
      div: 'div',
    },
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

  // TableLoadingBar / gsap 在 loading=true 时会读 matchMedia。
  // TableLoadingBar / gsap read matchMedia when loading is true.
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    configurable: true,
    value: vi.fn().mockImplementation((query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })),
  });
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

describe('PackageTable motion and status (table-only)', () => {
  it('renders online versions as a table with recommended and download states', () => {
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
        versions={['2.3.12', '2.3.11', '2.3.10']}
        localPackages={localPackages}
        recommendedVersion='2.3.12'
        downloads={downloads}
        onDownloadRequest={() => undefined}
      />,
    );

    expect(screen.getByTestId('package-online-table')).toBeInTheDocument();
    expect(screen.queryByTestId('plugin-available-grid')).not.toBeInTheDocument();
    // 仍然是表格行，不是卡片墙。 / Still table rows, not a card wall.
    expect(screen.getByRole('table')).toBeInTheDocument();
    expect(screen.getByTestId('package-recommended-2.3.12')).toHaveTextContent(
      'installer.recommended',
    );
    expect(screen.getByTestId('package-download-done-2.3.12')).toHaveTextContent(
      'installer.downloaded',
    );
    expect(screen.getByTestId('package-download-failed-2.3.11')).toHaveTextContent(
      'installer.downloadFailed',
    );
    expect(screen.getByText('installer.retry')).toBeInTheDocument();
    expect(screen.getByTestId('package-download-progress-2.3.10')).toHaveTextContent(
      '44%',
    );
    expect(screen.getByTestId('package-source-progress-2.3.10')).toHaveTextContent(
      '30%',
    );
  });

  it('keeps local packages as a table with honest missing source state', () => {
    render(
      <PackageTable
        type='local'
        localPackages={localPackages}
        onDelete={() => undefined}
      />,
    );

    expect(screen.getByTestId('package-local-table')).toBeInTheDocument();
    expect(screen.getByRole('table')).toBeInTheDocument();
    expect(
      screen.getByTestId('package-local-source-missing-2.3.12'),
    ).toHaveTextContent('installer.sourceMissing');
    expect(screen.getByText('apache-seatunnel-2.3.12-bin.tar.gz')).toBeInTheDocument();
  });

  it('marks reduced-motion on online rows without inventing a card view', async () => {
    const motionMock = await import('motion/react');
    (
      motionMock as unknown as {__setReducedMotionForTest: (v: boolean) => void}
    ).__setReducedMotionForTest(true);

    render(
      <PackageTable
        type='online'
        versions={['2.3.12']}
        recommendedVersion='2.3.12'
      />,
    );

    expect(screen.getByTestId('package-online-table')).toHaveAttribute(
      'data-reduce-motion',
      'true',
    );
    expect(screen.getByTestId('package-online-row-2.3.12')).toHaveAttribute(
      'data-reduce-motion',
      'true',
    );
    expect(screen.getByRole('table')).toBeInTheDocument();
  });

  it('dims the tbody on refresh instead of leaving opacity on each row', () => {
    const {rerender} = render(
      <PackageTable
        type='online'
        versions={['2.3.12']}
        recommendedVersion='2.3.12'
        loading={false}
      />,
    );

    const tbody = screen.getByTestId('package-online-tbody');
    const row = screen.getByTestId('package-online-row-2.3.12');
    expect(tbody.className).not.toMatch(/opacity-60/);
    expect(row.className).not.toMatch(/opacity-60/);

    rerender(
      <PackageTable
        type='online'
        versions={['2.3.12']}
        recommendedVersion='2.3.12'
        loading
      />,
    );

    expect(screen.getByTestId('package-online-tbody').className).toMatch(
      /opacity-60/,
    );
    expect(screen.getByTestId('package-online-row-2.3.12').className).not.toMatch(
      /opacity-60/,
    );
  });
});
