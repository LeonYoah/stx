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

import {expect, test} from '@playwright/test';

const backendBaseURL =
  process.env.E2E_BACKEND_BASE_URL ?? 'http://127.0.0.1:18000';

interface IntegrationStatusResponse {
  data?: {
    components?: Array<{
      name?: string;
      healthy?: boolean;
      error?: string;
      meta?: Record<string, string>;
    }>;
  };
}

/**
 * 真实可观测套件：需要独立打开 observability.enabled，并准备 SeaTunnel 包缓存。
 * 默认跳过，不进 PR smoke；本地或专用 CI 设 E2E_OBSERVABILITY_REAL=1。
 * Real observability suite: needs observability.enabled and a SeaTunnel package cache.
 * Skipped by default, not in PR smoke; set E2E_OBSERVABILITY_REAL=1 locally or in a dedicated CI job.
 */
test.describe('observability real', () => {
  test.skip(
    process.env.E2E_OBSERVABILITY_REAL !== '1',
    'set E2E_OBSERVABILITY_REAL=1 to run the gated observability suite',
  );

  test('integration status exposes embedded tsdb storage meta', async ({
    page,
  }) => {
    const response = await page
      .context()
      .request.get(`${backendBaseURL}/api/v1/monitoring/integration/status`);
    expect(response.ok()).toBeTruthy();
    const payload = (await response.json()) as IntegrationStatusResponse;
    const tsdb = (payload.data?.components ?? []).find(
      (item) => item.name === 'embedded_tsdb',
    );
    expect(tsdb).toBeTruthy();
    expect(tsdb?.healthy).toBe(true);
    expect(tsdb?.meta?.head_series).toBeTruthy();
    expect(tsdb?.meta?.compactions_failed).toBeTruthy();
    expect(tsdb?.meta?.sample_limit).toBeTruthy();
  });

  test('home dashboard leaves the observability-disabled empty state', async ({
    page,
  }) => {
    await page.goto('/dashboard');
    await expect(page.getByTestId('home-cluster-tab')).toBeVisible();
    const empty = page.getByTestId('home-trend-empty');
    if (await empty.count()) {
      await expect(empty).toHaveAttribute(
        'data-reason',
        /disabled|unavailable/,
      );
      return;
    }
    await expect(page.getByTestId('home-availability')).toBeVisible();
  });
});
