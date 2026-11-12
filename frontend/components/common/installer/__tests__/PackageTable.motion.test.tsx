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
 * PackageTable: no row-click download, busy button disabled, no empty … menu.
 * PackageTable：整行不下载、busy 按钮禁用、无空「…」菜单。
 */

import React from 'react';
import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest';
import {cleanup, fireEvent, render, screen} from '@testing-library/react';
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
      onClick,
      onKeyDown,
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
        onClick={onClick}
        onKeyDown={onKeyDown}
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
    download_urls: {aliyun: '', apache: '', huaweicloud: ''},
    is_local: true,
    has_source: false,
    source_download_urls: {aliyun: '', apache: '', huaweicloud: ''},
    uploaded_at: '2026-03-01T00:00:00Z',
  },
];

describe('PackageTable download interactions', () => {
  it('does not start a download when clicking the online row shell or decorative areas', () => {
    const onDownloadRequest = vi.fn();
    render(
      <PackageTable
        type='online'
        viewMode='table'
        versions={['2.3.11']}
        onDownloadRequest={onDownloadRequest}
      />,
    );

    fireEvent.click(screen.getByTestId('marketplace-row-online-2.3.11'));
    fireEvent.click(screen.getByTestId('marketplace-icon-online-2.3.11'));
    fireEvent.click(screen.getByTestId('marketplace-subtitle-online-2.3.11'));
    expect(onDownloadRequest).not.toHaveBeenCalled();
    // Online package rows stay non-pointer for open (no onOpen); download is explicit only.
    // 在线包行无 onOpen，不恢复打开手势；下载只走显式按钮。
    expect(screen.getByTestId('marketplace-row-online-2.3.11').className).not.toMatch(
      /cursor-pointer/,
    );
  });

  it('starts download only from the visible download button', () => {
    const onDownloadRequest = vi.fn();
    render(
      <PackageTable
        type='online'
        viewMode='card'
        versions={['2.3.11']}
        onDownloadRequest={onDownloadRequest}
      />,
    );

    fireEvent.click(screen.getByTestId('package-download-btn-2.3.11'));
    expect(onDownloadRequest).toHaveBeenCalledTimes(1);
    expect(onDownloadRequest).toHaveBeenCalledWith('2.3.11');
  });

  it('disables the download button while busy so clicks do not re-request', () => {
    const onDownloadRequest = vi.fn();
    const downloads: DownloadTask[] = [
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
        source_requested: false,
      },
    ];

    render(
      <PackageTable
        type='online'
        viewMode='table'
        versions={['2.3.10']}
        downloads={downloads}
        onDownloadRequest={onDownloadRequest}
      />,
    );

    const btn = screen.getByTestId('package-download-btn-2.3.10');
    expect(btn).toBeDisabled();
    fireEvent.click(btn);
    expect(onDownloadRequest).not.toHaveBeenCalled();
  });

  it('shows retry for failed downloads and no … menu for downloaded rows', () => {
    const onDownloadRequest = vi.fn();
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
    ];

    render(
      <PackageTable
        type='online'
        viewMode='table'
        versions={['2.3.12', '2.3.11']}
        localPackages={localPackages}
        recommendedVersion='2.3.12'
        downloads={downloads}
        onDownloadRequest={onDownloadRequest}
      />,
    );

    expect(screen.queryByTestId('marketplace-more-online-2.3.12')).not.toBeInTheDocument();
    expect(screen.queryByTestId('package-download-btn-2.3.12')).not.toBeInTheDocument();
    expect(screen.getByTestId('package-download-btn-2.3.11')).toHaveTextContent(
      'installer.retry',
    );
    fireEvent.click(screen.getByTestId('package-download-btn-2.3.11'));
    expect(onDownloadRequest).toHaveBeenCalledWith('2.3.11');
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
  });
});
