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
  installInstallWizardTemplateRoutes,
  installWizardFailureTemplate,
} from './helpers/install-wizard-template';

test.describe('install wizard failure template', () => {
  test.beforeEach(async ({page}) => {
    await installInstallWizardTemplateRoutes(page, {
      installationScenario: 'failed',
    });
  });

  test('stays on the install step and exposes retry guidance after plugin installation fails', async ({
    page,
  }) => {
    await page.goto('/e2e-lab/install-wizard');

    await page.getByTestId('install-precheck-run').click();
    await expect(
      page.getByText(
        /全部通过|Precheck Passed|Host passed all installation checks/i,
      ),
    ).toBeVisible();

    await page.getByTestId('install-wizard-next').click();
    await page.getByTestId('install-wizard-next').click();
    await page.getByTestId('install-plugin-card-jdbc').click();
    await page.getByTestId('plugin-detail-tab-dependencies').click();
    await page
      .getByTestId(`plugin-profile-${installWizardFailureTemplate.profileKey}`)
      .click();
    await page.keyboard.press('Escape');

    await page.getByTestId('install-wizard-next').click();

    const installStep = page.getByTestId('install-wizard-step-install');
    await expect(installStep).toBeVisible();
    await expect(page.getByTestId('install-step-overall')).toContainText(
      /安装失败|Installation Failed/i,
    );
    await expect(
      page.getByTestId('install-step-item-install_plugins'),
    ).toContainText(installWizardFailureTemplate.failureErrorMessage);
    await expect(
      page.getByTestId('install-step-item-install_plugins'),
    ).toContainText(installWizardFailureTemplate.failureRollbackMessage);
    await expect(
      page.getByTestId('install-step-retry-install_plugins'),
    ).toBeVisible();
    await expect(page.getByTestId('install-wizard-step-complete')).toHaveCount(
      0,
    );
    await expect(page.getByTestId('install-complete-result')).toHaveCount(0);
    await expect(page.getByTestId('install-wizard-next')).toHaveCount(0);
  });
});
