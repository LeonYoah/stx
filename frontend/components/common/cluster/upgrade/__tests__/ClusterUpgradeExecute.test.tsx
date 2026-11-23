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

import {act, fireEvent, render, screen, waitFor} from '@testing-library/react';
import {NextIntlClientProvider} from 'next-intl';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import zh from '@/lib/i18n/locales/zh.json';
import {toast} from '@/lib/toast';
import {ClusterUpgradeExecute} from '../ClusterUpgradeExecute';

const mockPush = vi.hoisted(() => vi.fn());
let currentSearch = 'planId=42';

vi.mock('next/navigation', () => ({
  useRouter: () => ({push: mockPush}),
  useSearchParams: () => new URLSearchParams(currentSearch),
}));

vi.mock('@/lib/toast', () => ({
  toast: {error: vi.fn(), success: vi.fn()},
}));

vi.mock('@/lib/st-upgrade-session', () => ({
  loadStUpgradeSession: () => null,
  patchStUpgradeSession: vi.fn(),
}));

const getPlan = vi.hoisted(() => vi.fn());
const getTask = vi.hoisted(() => vi.fn());
const getTaskSteps = vi.hoisted(() => vi.fn());
const getTaskLogs = vi.hoisted(() => vi.fn());

vi.mock('@/lib/services', () => ({
  default: {
    stUpgrade: {
      getPlan,
      getTask,
      getTaskSteps,
      getTaskLogs,
      executePlan: vi.fn(),
      executePlanSafe: vi.fn(),
    },
  },
}));

function renderExecute() {
  return render(
    <NextIntlClientProvider locale='zh' messages={zh}>
      <ClusterUpgradeExecute clusterId={1} />
    </NextIntlClientProvider>,
  );
}

function makeRunningTask() {
  return {
    id: 9,
    plan_id: 42,
    cluster_id: 1,
    status: 'running' as const,
    rollback_status: '',
    plan: {
      id: 42,
      cluster_id: 1,
      source_version: '2.3.11',
      target_version: '2.3.12',
      status: 'ready',
      snapshot: {
        cluster_id: 1,
        source_version: '2.3.11',
        target_version: '2.3.12',
        steps: [],
        node_targets: [],
        package_manifest: {},
        connector_manifest: {connectors: [], libraries: [], plugin_dependencies: []},
        config_merge_plan: {files: [], conflict_count: 0},
      },
    },
  };
}

describe('ClusterUpgradeExecute plan load states', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    currentSearch = 'planId=42';
    Object.defineProperty(window, 'matchMedia', {
      writable: true,
      value: vi.fn().mockImplementation((query: string) => ({
        matches: false,
        media: query,
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
      })),
    });
  });

  it('shows localized read-error card and toasts raw backend error', async () => {
    const raw = 'stupgrade: failed to scan UpgradePlanSnapshot';
    const err = Object.assign(new Error(raw), {status: 500});
    getPlan.mockRejectedValue(err);
    renderExecute();

    await waitFor(() => {
      expect(screen.getByTestId('upgrade-plan-load-error')).toBeInTheDocument();
    });
    expect(screen.getByText(zh.stUpgrade.loadPlanFailed)).toBeInTheDocument();
    expect(
      screen.getByText(zh.stUpgrade.loadPlanFailedDesc),
    ).toBeInTheDocument();
    expect(screen.queryByText(raw)).not.toBeInTheDocument();
    expect(screen.queryByText(zh.stUpgrade.missingPlan)).not.toBeInTheDocument();
    expect(toast.error).toHaveBeenCalledWith(raw);
    expect(
      screen.getByTestId('upgrade-plan-load-error').getAttribute('role'),
    ).not.toBe('alert');
  });

  it('keeps error card on retry and disables the retry button while pending', async () => {
    const err = Object.assign(
      new Error('stupgrade: failed to scan UpgradePlanSnapshot'),
      {status: 500},
    );
    let resolveRetry: ((value: never) => void) | undefined;
    getPlan
      .mockRejectedValueOnce(err)
      .mockImplementationOnce(
        () =>
          new Promise((_, reject) => {
            resolveRetry = reject as (value: never) => void;
          }),
      );
    renderExecute();

    await waitFor(() => {
      expect(screen.getByTestId('upgrade-plan-load-error')).toBeInTheDocument();
    });

    const retryButton = screen.getByRole('button', {name: zh.common.retry});
    fireEvent.click(retryButton);

    await waitFor(() => {
      expect(retryButton).toBeDisabled();
    });
    // 重试中仍保留错误卡，不退回整页骨架。
    // Error card stays during retry; the full-page skeleton must not replace it.
    expect(screen.getByTestId('upgrade-plan-load-error')).toBeInTheDocument();
    expect(screen.getByTestId('upgrade-execute-page')).toBeInTheDocument();

    await act(async () => {
      resolveRetry?.(err as never);
    });

    await waitFor(() => {
      expect(
        screen.getByRole('button', {name: zh.common.retry}),
      ).not.toBeDisabled();
    });
    expect(screen.getByTestId('upgrade-plan-load-error')).toBeInTheDocument();
  });

  it('shows missing-plan empty state on 404', async () => {
    const err = Object.assign(new Error('st upgrade plan not found'), {
      status: 404,
    });
    getPlan.mockRejectedValue(err);
    renderExecute();

    await waitFor(() => {
      expect(screen.getByText(zh.stUpgrade.missingPlan)).toBeInTheDocument();
    });
    expect(
      screen.queryByTestId('upgrade-plan-load-error'),
    ).not.toBeInTheDocument();
  });

  it('polls silently without toasting or clearing plan on transient errors', async () => {
    vi.useFakeTimers({shouldAdvanceTime: true});
    currentSearch = 'planId=42&taskId=9';
    const task = makeRunningTask();
    getTask.mockResolvedValue(task);
    getTaskSteps.mockResolvedValue({items: []});
    getTaskLogs.mockResolvedValue({
      items: [],
      total: 0,
      page: 1,
      page_size: 100,
      total_pages: 1,
    });

    renderExecute();

    await waitFor(() => {
      expect(screen.getByText('2.3.11 → 2.3.12')).toBeInTheDocument();
    });
    const toastsAfterLoad = vi.mocked(toast.error).mock.calls.length;

    const pollErr = Object.assign(new Error('temporary backend error'), {
      status: 500,
    });
    getTask.mockRejectedValue(pollErr);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(3100);
    });

    expect(vi.mocked(toast.error).mock.calls.length).toBe(toastsAfterLoad);
    expect(screen.getByText('2.3.11 → 2.3.12')).toBeInTheDocument();
    expect(screen.queryByText(zh.stUpgrade.missingPlan)).not.toBeInTheDocument();
    expect(
      screen.queryByTestId('upgrade-plan-load-error'),
    ).not.toBeInTheDocument();

    const notFound = Object.assign(new Error('st upgrade task not found'), {
      status: 404,
    });
    getTask.mockRejectedValue(notFound);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(3100);
    });

    expect(vi.mocked(toast.error).mock.calls.length).toBe(toastsAfterLoad);
    expect(screen.getByText('2.3.11 → 2.3.12')).toBeInTheDocument();
    expect(screen.queryByText(zh.stUpgrade.missingPlan)).not.toBeInTheDocument();

    vi.useRealTimers();
  });
});
