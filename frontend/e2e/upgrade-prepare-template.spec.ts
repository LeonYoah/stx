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
import {
  addInitScriptClearLocalStorageKeepOnboardingSkipped,
  dismissOnboardingPathChooserIfPresent,
} from './helpers/auth';
import {
  installUpgradePrepareTemplateRoutes,
  upgradePrepareTemplate,
} from './helpers/upgrade-prepare-template';

test.describe('upgrade prepare template', () => {
  test.beforeEach(async ({page}) => {
    // 保留隔离清库，但勿抹掉 auth.setup 的上手跳过，否则路径选择 Dialog 会再次遮罩。
    // Keep LS wipe for isolation, but do not erase auth.setup's onboarding skip or the path Dialog returns.
    await addInitScriptClearLocalStorageKeepOnboardingSkipped(page);
    await installUpgradePrepareTemplateRoutes(page);
  });

  test('runs upgrade precheck and continues into config review', async ({
    page,
  }) => {
    await page.goto(
      `/clusters/${upgradePrepareTemplate.clusterId}/upgrade/prepare`,
    );
    // 双保险：清库回种异常时仍点跳过 / Belt-and-suspenders if re-seed missed the active user id.
    await dismissOnboardingPathChooserIfPresent(page);

    await expect(page.getByTestId('upgrade-prepare-page')).toBeVisible();
    await expect(page.getByRole('heading', {level: 1})).toHaveText(
      /升级准备页|Upgrade Prepare/i,
    );
    await expect(page.getByTestId('upgrade-prepare-continue')).toBeDisabled();

    await page.getByTestId('upgrade-prepare-run-precheck').click();
    await expect(page.getByTestId('upgrade-prepare-summary')).toContainText(
      upgradePrepareTemplate.targetVersion,
    );
    await expect(page.getByTestId('upgrade-prepare-ready-badge')).toContainText(
      /可继续|可计划|Ready/i,
    );
    await expect(
      page.getByTestId('upgrade-prepare-node-targets'),
    ).toContainText(upgradePrepareTemplate.targetInstallDir);

    await expect(page.getByTestId('upgrade-prepare-continue')).toBeEnabled();
    await page.getByTestId('upgrade-prepare-continue').click();

    await page.waitForURL(
      `**/clusters/${upgradePrepareTemplate.clusterId}/upgrade/config`,
    );
    // 二次导航会再跑 init script；再清一次遮罩以免 h1 被 aria-hidden。
    // Soft nav re-runs the init script; dismiss again so h1 is not aria-hidden.
    await dismissOnboardingPathChooserIfPresent(page);
    await expect(page.getByRole('heading', {level: 1})).toHaveText(
      /配置差异处理|Config Difference Review/i,
    );
  });
});
