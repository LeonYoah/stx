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
  installWizardTemplate,
} from './helpers/install-wizard-template';

test.describe('install wizard template', () => {
  test.beforeEach(async ({page}) => {
    await installInstallWizardTemplateRoutes(page);
  });

  test('completes the main install wizard flow with a JDBC profile selection', async ({
    page,
  }) => {
    await page.goto('/e2e-lab/install-wizard');

    await expect(page.getByTestId('install-wizard-dialog')).toBeVisible();
    await expect(
      page.getByTestId('install-wizard-step-precheck'),
    ).toBeVisible();

    await page.getByTestId('install-precheck-run').click();
    await expect(
      page.getByText(
        /全部通过|Precheck Passed|Host passed all installation checks/i,
      ),
    ).toBeVisible();

    await expect(page.getByTestId('install-wizard-next')).toBeEnabled();
    await page.getByTestId('install-wizard-next').click();
    await expect(page.getByTestId('install-wizard-step-config')).toBeVisible();
    await expect(page.getByTestId('install-config-version')).toContainText(
      installWizardTemplate.seatunnelVersion,
    );

    await page.getByTestId('install-wizard-next').click();
    await expect(page.getByTestId('install-wizard-step-plugins')).toBeVisible();

    await page.getByTestId('install-plugin-card-jdbc').click();
    await expect(page.getByTestId('plugin-detail-dialog-jdbc')).toBeVisible();
    await page.getByTestId('plugin-detail-tab-dependencies').click();
    await page
      .getByTestId(`plugin-profile-${installWizardTemplate.profileKey}`)
      .click();
    await page.keyboard.press('Escape');
    await expect(page.getByTestId('plugin-detail-dialog-jdbc')).toHaveCount(0);

    await expect(page.getByTestId('install-wizard-next')).toBeEnabled();
    await page.getByTestId('install-wizard-next').click();

    await expect(
      page.getByTestId('install-wizard-step-complete'),
    ).toBeVisible();
    await expect(page.getByTestId('install-complete-result')).toContainText(
      /安装成功|Installation Success/i,
    );
    await expect(
      page
        .getByTestId('install-wizard-step-complete')
        .getByText(installWizardTemplate.pluginName),
    ).toBeVisible();
  });
});
