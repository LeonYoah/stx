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
  upgradePrepareBlockedTemplate,
} from './helpers/upgrade-prepare-template';

test.describe('upgrade prepare blocked template', () => {
  test.beforeEach(async ({page}) => {
    // 保留隔离清库，但勿抹掉 auth.setup 的上手跳过，否则路径选择 Dialog 会再次遮罩。
    // Keep LS wipe for isolation, but do not erase auth.setup's onboarding skip or the path Dialog returns.
    await addInitScriptClearLocalStorageKeepOnboardingSkipped(page);
    await installUpgradePrepareTemplateRoutes(page, {
      precheckScenario: 'blocked',
    });
  });

  test('keeps continue disabled and surfaces blocking package guidance', async ({
    page,
  }) => {
    await page.goto(
      `/clusters/${upgradePrepareBlockedTemplate.clusterId}/upgrade/prepare`,
    );
    // 双保险：清库回种异常时仍点跳过 / Belt-and-suspenders if re-seed missed the active user id.
    await dismissOnboardingPathChooserIfPresent(page);

    await page.getByTestId('upgrade-prepare-run-precheck').click();

    await expect(page.getByTestId('upgrade-prepare-ready-badge')).toContainText(
      /存在阻断|Blocked/i,
    );
    await expect(page.getByTestId('upgrade-prepare-continue')).toBeDisabled();
    await expect(page.getByTestId('upgrade-prepare-issues')).toContainText(
      upgradePrepareBlockedTemplate.blockedIssueCode,
    );
    await expect(page.getByTestId('upgrade-prepare-issues')).toContainText(
      upgradePrepareBlockedTemplate.blockedIssueMessage,
    );
    await expect(page.getByTestId('upgrade-prepare-go-packages')).toBeVisible();
    await expect(
      page.getByTestId('upgrade-prepare-node-targets'),
    ).toContainText(upgradePrepareBlockedTemplate.targetInstallDir);
  });
});
