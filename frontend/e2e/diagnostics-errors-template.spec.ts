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

import {expect, test, type Page} from '@playwright/test';
import {
  diagnosticsErrorsTemplate,
  installDiagnosticsErrorsTemplateRoutes,
} from './helpers/diagnostics-errors-template';

async function openErrorsTab(page: Page) {
  await page.goto('/diagnostics?tab=errors');
  await expect(page).toHaveURL(/tab=errors/);
  await expect(page.getByTestId('diagnostics-tab-errors')).toBeVisible();
}

test.describe('diagnostics errors panel template', () => {
  test.beforeEach(async ({page}) => {
    await installDiagnosticsErrorsTemplateRoutes(page);
  });

  test('loads the error group list with heat chips', async ({page}) => {
    await openErrorsTab(page);

    await expect(page.getByTestId('error-row-1')).toBeVisible();
    await expect(page.getByTestId('error-row-2')).toBeVisible();
    await expect(page.getByTestId('error-row-3')).toBeVisible();
    // 频次软徽章随热度着色：15 次高频、5 次中频、1 次低频。
    // Heat chips colour by frequency: 15 high, 5 mid, 1 low.
    await expect(
      page.getByTestId('error-row-1').getByText(/15 次|15×/),
    ).toBeVisible();
    await expect(
      page.getByTestId('error-row-3').getByText(/1 次|1×/),
    ).toBeVisible();
  });

  test('filters by the high-frequency segment via min_occurrences', async ({
    page,
  }) => {
    const requests: string[] = [];
    page.on('request', (req) => {
      if (req.url().includes('/api/v1/diagnostics/errors/groups?')) {
        requests.push(req.url());
      }
    });
    await openErrorsTab(page);
    await expect(page.getByTestId('error-row-1')).toBeVisible();

    await page.getByRole('button', {name: /高频|High/i}).click();

    // 分段走服务端参数：只剩高频行，请求带 min_occurrences=10。
    // The segment is server-side: only the hot row remains and the request
    // carries min_occurrences=10.
    await expect(page.getByTestId('error-row-1')).toBeVisible();
    await expect(page.getByTestId('error-row-2')).toHaveCount(0);
    await expect(page.getByTestId('error-row-3')).toHaveCount(0);
    expect(requests.at(-1)).toContain('min_occurrences=10');
  });

  test('opens a group detail, resolves its owning cluster, and keeps group_id in the URL', async ({
    page,
  }) => {
    await openErrorsTab(page);
    await page.getByTestId('error-row-3').click();

    await expect(page.getByTestId('detail-rail')).toBeVisible();
    await expect(page).toHaveURL(/group_id=3/);
    // 归属集群按组自身 last_cluster_id=2 解析为 stx-dev，与当前筛选无关。
    // The owning cluster resolves to stx-dev via the group's last_cluster_id,
    // independent of the current filter.
    const detail = page.getByTestId('error-group-detail');
    await expect(detail.getByText('stx-dev')).toBeVisible();
    await expect(
      detail.getByRole('link', {name: /前往受影响集群|Go to cluster/i}),
    ).toHaveAttribute('href', '/clusters/2');
    // 近期事件样本出现在详情内：engine.log 会同时出现在行内与面板副标题，
    // 故定位唯一的堆栈 <pre>。
    // The recent event sample shows inside the detail: engine.log appears both
    // inline and in the panel subtitle, so assert the unique stack <pre>.
    await expect(
      detail.locator('pre', {hasText: /RuntimeException/}),
    ).toBeVisible();
  });

  test('switches groups with J/K while the detail is open', async ({page}) => {
    await openErrorsTab(page);
    await page.getByTestId('error-row-1').click();
    await expect(page.getByTestId('detail-rail')).toBeVisible();
    await expect(page).toHaveURL(/group_id=1/);

    await page.keyboard.press('j');
    await expect(page).toHaveURL(/group_id=2/);
    await page.keyboard.press('k');
    await expect(page).toHaveURL(/group_id=1/);
  });

  test('shows the error state and recovers on retry', async ({page}) => {
    // 首个列表请求置为失败，验证错误态与重试恢复；times:1 后回落到夹具路由。
    // Fail the first list request to verify the error state and retry recovery;
    // after times:1 the fixture route takes over again.
    await page.route(
      '**/api/v1/diagnostics/errors/groups**',
      async (route) => {
        await route.fulfill({
          status: 500,
          json: {error_msg: 'group list exploded', data: null},
        });
      },
      {times: 1},
    );

    await openErrorsTab(page);
    await expect(
      page.getByText(diagnosticsErrorsTemplate.listErrorMessage),
    ).toBeVisible();

    await page.getByRole('button', {name: /重试|Retry/i}).click();
    await expect(page.getByTestId('error-row-1')).toBeVisible();
  });
});
