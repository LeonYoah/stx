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
  diagnosticsInspectionsTemplate,
  installDiagnosticsInspectionsTemplateRoutes,
} from './helpers/diagnostics-inspections-template';

async function openInspectionsTab(page: Page, clusterId?: number) {
  const clusterQuery = clusterId ? `&cluster_id=${clusterId}` : '';
  await page.goto(`/diagnostics?tab=inspections${clusterQuery}`);
  await expect(page).toHaveURL(/tab=inspections/);
  await expect(
    page.getByTestId('diagnostics-tab-inspections'),
  ).toBeVisible();
}

test.describe('diagnostics inspections panel template', () => {
  test.beforeEach(async ({page}) => {
    await installDiagnosticsInspectionsTemplateRoutes(page);
  });

  test('loads the report list with status chips and finding badges', async ({
    page,
  }) => {
    await openInspectionsTab(page);

    await expect(page.getByTestId('inspection-row-1')).toBeVisible();
    await expect(page.getByTestId('inspection-row-2')).toBeVisible();
    await expect(page.getByTestId('inspection-row-3')).toBeVisible();
    await expect(
      page.getByTestId('inspection-row-1').getByText(/1 严重|1 critical/),
    ).toBeVisible();
    await expect(
      page.getByTestId('inspection-row-1').getByText(/已完成|Completed/),
    ).toBeVisible();
    // 状态筛选是下拉：打开确认「失败」项在列，且不显示与服务端总数矛盾的
    // 分桶计数。
    // The status filter is a select: open it and confirm the Failed option is
    // listed without bucket counts that could contradict the server total.
    await page
      .getByRole('combobox', {name: /按状态筛选|Filter by status/i})
      .click();
    const failedOption = page.getByRole('option', {
      name: /^失败$|^Failed$/,
    });
    await expect(failedOption).toBeVisible();
    await expect(failedOption).not.toContainText(/\d/);
    await page.keyboard.press('Escape');
  });

  test('filters by the failed status option via the status param', async ({
    page,
  }) => {
    const requests: string[] = [];
    page.on('request', (req) => {
      if (req.url().includes('/api/v1/diagnostics/inspections?')) {
        requests.push(req.url());
      }
    });
    await openInspectionsTab(page);
    await expect(page.getByTestId('inspection-row-1')).toBeVisible();

    await page
      .getByRole('combobox', {name: /按状态筛选|Filter by status/i})
      .click();
    await page.getByRole('option', {name: /^失败$|^Failed$/}).click();

    // 筛选走服务端参数：只剩失败行，请求带 status=failed。
    // The filter is server-side: only the failed row remains and the request
    // carries status=failed.
    await expect(page.getByTestId('inspection-row-2')).toBeVisible();
    await expect(page.getByTestId('inspection-row-1')).toHaveCount(0);
    await expect(page.getByTestId('inspection-row-3')).toHaveCount(0);
    expect(requests.at(-1)).toContain('status=failed');
  });

  test('opens a report detail, resolves its owning cluster, and keeps report_id in the URL', async ({
    page,
  }) => {
    await openInspectionsTab(page);
    await page.getByTestId('inspection-row-3').click();

    await expect(page.getByTestId('detail-rail')).toBeVisible();
    await expect(page).toHaveURL(/report_id=3/);
    // 归属集群按报告自身 cluster_id=2 解析为 stx-dev，与当前筛选无关。
    // The owning cluster resolves to stx-dev via the report's own cluster_id,
    // independent of the current filter.
    const detail = page.getByTestId('inspection-report-detail');
    await expect(detail.getByText('stx-dev')).toBeVisible();
    // 详情栏信息完整（含摘要），不再有独立的完整报告入口。
    // The rail is information-complete (summary included); the separate
    // full-report entry was removed.
    await expect(
      detail.getByRole('link', {name: /查看完整报告|View full report/i}),
    ).toHaveCount(0);
  });

  test('switches reports with J/K while the detail is open', async ({
    page,
  }) => {
    await openInspectionsTab(page);
    await page.getByTestId('inspection-row-1').click();
    await expect(page.getByTestId('detail-rail')).toBeVisible();
    await expect(page).toHaveURL(/report_id=1/);

    await page.keyboard.press('j');
    await expect(page).toHaveURL(/report_id=2/);
    await page.keyboard.press('k');
    await expect(page).toHaveURL(/report_id=1/);
  });

  test('starts an inspection and opens the new report', async ({page}) => {
    await openInspectionsTab(page, 1);
    await expect(page.getByTestId('inspection-row-1')).toBeVisible();

    await page.getByTestId('inspection-start').click();
    await page.getByTestId('inspection-start-submit').click();

    // 发起成功后自动打开新报告详情，report_id 同步到 URL。
    // A successful start auto-opens the new report detail and syncs report_id.
    await expect(page.getByTestId('detail-rail')).toBeVisible();
    await expect(page).toHaveURL(/report_id=10/);
    const detail = page.getByTestId('inspection-report-detail');
    await expect(detail.getByText('stx-prod')).toBeVisible();
  });

  test('keeps the start dialog open and shows the error when start fails', async ({
    page,
  }) => {
    // 模式必须带尾部通配且不能限次：请求统一带 ?lang= 查询串，首个命中的其实是
    // 列表 GET，times:1 会被它消耗掉。
    // The pattern needs a trailing wildcard and no call cap: every request
    // carries a ?lang= query, and the first match is the list GET, which would
    // consume a times:1 allowance.
    await page.route(
      '**/api/v1/diagnostics/inspections*',
      async (route) => {
        const {pathname} = new URL(route.request().url());
        if (
          route.request().method() === 'POST' &&
          pathname === '/api/v1/diagnostics/inspections'
        ) {
          await route.fulfill({
            status: 500,
            json: {error_msg: 'inspection start exploded', data: null},
          });
          return;
        }
        // fallback 走夹具路由；continue 会绕过其余桩直接打真实后端。
        // fallback reaches the fixture route; continue would bypass every stub
        // and hit the real backend.
        await route.fallback();
      },
    );

    await openInspectionsTab(page, 1);
    await expect(page.getByTestId('inspection-row-1')).toBeVisible();
    await page.getByTestId('inspection-start').click();
    await page.getByTestId('inspection-start-submit').click();

    await expect(
      page.getByText('inspection start exploded'),
    ).toBeVisible();
    // 失败时弹窗保留，可再次提交。
    // On failure the dialog stays open so the user can retry.
    await expect(
      page.getByTestId('inspection-start-submit'),
    ).toBeVisible();
  });

  test('switches to the auto view and opens the policy from the auto chip', async ({
    page,
  }) => {
    await openInspectionsTab(page);
    // 自动触发报告带「自动 · 规则名」徽章。
    // The auto-triggered report carries the "auto · <policy>" chip.
    const chip = page.getByTestId('inspection-auto-chip-4');
    await expect(chip).toContainText(
      `自动 · ${diagnosticsInspectionsTemplate.autoPolicyName}`,
    );
    await chip.click();

    // 点击跳自动巡检视图并打开该策略详情，URL 同步 view / policy_id。
    // Clicking jumps to the auto view with the policy detail open; the URL
    // syncs view / policy_id.
    await expect(page).toHaveURL(/view=auto/);
    await expect(page).toHaveURL(/policy_id=7/);
    await expect(page.getByTestId('detail-rail')).toBeVisible();
  });

  test('filters records by auto_policy_id and removes the badge', async ({
    page,
  }) => {
    await page.goto('/diagnostics?tab=inspections&auto_policy_id=7');
    // 只剩该策略触发的报告，工具栏出现可移除筛选徽章。
    // Only the policy-triggered report remains and the toolbar shows the
    // removable filter badge.
    await expect(page.getByTestId('inspection-row-4')).toBeVisible();
    await expect(page.getByTestId('inspection-row-1')).toHaveCount(0);
    await expect(
      page.getByText(
        `触发规则：${diagnosticsInspectionsTemplate.autoPolicyName}`,
      ),
    ).toBeVisible();

    await page.getByTestId('inspection-policy-filter-clear').click();
    await expect(page).not.toHaveURL(/auto_policy_id/);
    await expect(page.getByTestId('inspection-row-1')).toBeVisible();
  });

  test('switches to the auto view via the segment and back', async ({page}) => {
    await openInspectionsTab(page);
    await expect(page.getByTestId('inspection-row-1')).toBeVisible();

    await page.getByRole('button', {name: /^自动巡检$|^Auto$/}).click();
    await expect(page).toHaveURL(/view=auto/);
    await expect(page.getByTestId('policy-row-7')).toBeVisible();

    // 回记录视图时 URL 清掉 view，报告列表原样恢复。
    // Switching back clears view from the URL and restores the report list.
    await page.getByRole('button', {name: /^巡检记录$|^Records$/}).click();
    await expect(page).not.toHaveURL(/view=/);
    await expect(page.getByTestId('inspection-row-1')).toBeVisible();
  });

  test('shows the error state and recovers on retry', async ({page}) => {
    // 首个列表请求置为失败，验证错误态与重试恢复；times:1 后回落到夹具路由。
    // Fail the first list request to verify the error state and retry
    // recovery; after times:1 the fixture route takes over again.
    await page.route(
      '**/api/v1/diagnostics/inspections**',
      async (route) => {
        await route.fulfill({
          status: 500,
          json: {error_msg: 'inspection list exploded', data: null},
        });
      },
      {times: 1},
    );

    await openInspectionsTab(page);
    await expect(
      page.getByText('inspection list exploded'),
    ).toBeVisible();

    await page.getByRole('button', {name: /重试|Retry/i}).click();
    await expect(page.getByTestId('inspection-row-1')).toBeVisible();
  });
});
