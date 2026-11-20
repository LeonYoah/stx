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
  diagnosticsPoliciesTemplate,
  installDiagnosticsPoliciesTemplateRoutes,
} from './helpers/diagnostics-policies-template';

async function openPoliciesTab(page: Page) {
  await page.goto('/diagnostics?tab=policies');
  await expect(page).toHaveURL(/tab=policies/);
  await expect(page.getByTestId('diagnostics-tab-policies')).toBeVisible();
}

test.describe('diagnostics policies panel template', () => {
  let state: Awaited<
    ReturnType<typeof installDiagnosticsPoliciesTemplateRoutes>
  >;

  test.beforeEach(async ({page}) => {
    state = await installDiagnosticsPoliciesTemplateRoutes(page);
  });

  test('loads the policy list under tab=policies with scope and invalid rows', async ({
    page,
  }) => {
    await openPoliciesTab(page);

    await expect(page.getByTestId('policy-row-1')).toBeVisible();
    await expect(page.getByTestId('policy-row-2')).toBeVisible();
    await expect(page.getByTestId('policy-row-3')).toBeVisible();
    // 失效策略行带徽标与翻译后的原因行。
    // The invalid row carries the badge plus its translated reason line.
    await expect(
      page.getByText(/已不在规则目录|no longer in the rule catalog/i),
    ).toBeVisible();
  });

  test('creates a policy through the centered dialog', async ({page}) => {
    await openPoliciesTab(page);
    await page.getByTestId('policy-create').click();
    const form = page.getByTestId('policy-form-body');
    await expect(form).toBeVisible();

    await form.getByLabel(/策略名称|Policy Name/i).fill('e2e-new-policy');
    await page.getByTestId('policy-form-save').click();

    await expect(page.getByTestId('policy-form-body')).toHaveCount(0);
    await expect(
      page.getByTestId(`policy-row-${state.policies.at(-1)!.id}`),
    ).toBeVisible();
    await expect(page.getByText('e2e-new-policy')).toBeVisible();
  });

  test('intercepts a dirty detail close with keep-editing then discard', async ({
    page,
  }) => {
    await openPoliciesTab(page);
    await page.getByTestId('policy-row-1').click();
    await expect(page.getByTestId('detail-rail')).toBeVisible();

    await page.getByLabel(/策略名称|Policy Name/i).fill('renamed-policy');
    await page.getByRole('button', {name: 'Close'}).click();
    await expect(page.getByTestId('discard-confirm-dialog')).toBeVisible();

    // 「继续编辑」保留修改与打开的详情。
    // Keep editing retains the change and the open detail.
    await page.getByTestId('discard-keep-editing').click();
    await expect(
      page.getByTestId('discard-confirm-dialog'),
    ).toHaveCount(0);
    await expect(page.getByLabel(/策略名称|Policy Name/i)).toHaveValue(
      'renamed-policy',
    );

    // 再次关闭并「放弃修改」→ 详情关闭。
    // Close again and discard → the detail closes.
    await page.getByRole('button', {name: 'Close'}).click();
    await page.getByTestId('discard-confirm-action').click();
    await expect(page.getByTestId('detail-rail')).toHaveCount(0);
  });

  test('intercepts a top-level tab switch while the policy form is dirty', async ({
    page,
  }) => {
    await openPoliciesTab(page);
    await page.getByTestId('policy-row-1').click();
    await page.getByLabel(/策略名称|Policy Name/i).fill('dirty-name');

    await page.getByTestId('diagnostics-tab-inspections').click();
    await expect(page.getByTestId('discard-confirm-dialog')).toBeVisible();

    // 放弃修改后执行原请求的切标签。
    // Discarding performs the originally requested tab switch.
    await page.getByTestId('discard-confirm-action').click();
    await expect(page).toHaveURL(/tab=inspections/);
  });

  test('deletes a policy through the two-step ConfirmMorph', async ({page}) => {
    await openPoliciesTab(page);
    await page.getByTestId('policy-row-2').click();
    await expect(page.getByTestId('detail-rail')).toBeVisible();

    const deleteRequest = page.waitForRequest(
      (req) =>
        req.method() === 'DELETE' &&
        req.url().includes('/api/v1/diagnostics/auto-policies/2'),
    );
    await page
      .getByRole('button', {name: /删除策略|Delete policy/i})
      .click();
    // 提示文案会同时渲染在 aria-live 状态区，这里以唯一确认按钮为准；
    // ConfirmMorph 280ms 防误触窗口后 Enter 作答（确认按钮带 autofocus），
    // 避开 face 动画期间的指针命中不稳定。
    // The prompt also renders inside the aria-live status region, so assert on
    // the unique confirm button; after the 280ms arm window Enter answers (the
    // confirm button carries autofocus), sidestepping hit-testing while the
    // face animation settles.
    await expect(
      page.getByRole('button', {name: /^(删除|Delete)$/i}),
    ).toBeVisible();
    await page.waitForTimeout(350);
    await page.keyboard.press('Enter');

    await deleteRequest;
    await expect(page.getByTestId('detail-rail')).toHaveCount(0);
    await expect(page.getByTestId('policy-row-2')).toHaveCount(0);
  });

  test('keeps the create dialog open and shows the error when create fails', async ({
    page,
  }) => {
    state.createShouldFail = true;
    await openPoliciesTab(page);
    await page.getByTestId('policy-create').click();
    const form = page.getByTestId('policy-form-body');
    await expect(form).toBeVisible();

    await form.getByLabel(/策略名称|Policy Name/i).fill('e2e-fail-policy');
    await page.getByTestId('policy-form-save').click();

    // 失败时弹窗保留并提示错误。
    // On failure the dialog stays open with a visible error.
    await expect(
      page.getByText(diagnosticsPoliciesTemplate.createErrorMessage),
    ).toBeVisible();
    await expect(form).toBeVisible();
    await expect(state.policies.map((p) => p.name)).not.toContain(
      'e2e-fail-policy',
    );
  });
});
