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

import {render, screen, waitFor} from '@testing-library/react';
import {NextIntlClientProvider} from 'next-intl';
import {beforeEach, describe, expect, it, vi} from 'vitest';
import zh from '@/lib/i18n/locales/zh.json';
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

  it('shows read-error state with retry when plan snapshot cannot be scanned', async () => {
    const err = Object.assign(new Error('stupgrade: failed to scan UpgradePlanSnapshot'), {
      status: 500,
    });
    getPlan.mockRejectedValue(err);
    renderExecute();

    await waitFor(() => {
      expect(screen.getByTestId('upgrade-plan-load-error')).toBeInTheDocument();
    });
    expect(screen.getByText(zh.stUpgrade.loadPlanFailed)).toBeInTheDocument();
    expect(screen.queryByText(zh.stUpgrade.missingPlan)).not.toBeInTheDocument();
    expect(
      screen.getByRole('button', {name: zh.common.retry}),
    ).toBeInTheDocument();
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
});
