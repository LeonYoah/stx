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
  createPlaceholderJar,
  installPluginDependencyTemplateRoutes,
  pluginDependencyTemplate,
} from './helpers/plugin-dependency-template';

async function openJdbcTemplateDialog(
  page: Page,
  tab: 'dependencies' | 'custom',
) {
  await page.goto('/plugins');
  await expect(page).toHaveURL(/\/plugins$/);
  await expect(page.getByRole('heading', {level: 1})).toHaveText(
    /插件市场|Plugin Marketplace/i,
  );
  await expect(page.getByTestId('plugin-card-jdbc')).toBeVisible();
  await page.getByTestId('plugin-card-jdbc').click();
  await expect(page.getByTestId('plugin-detail-dialog-jdbc')).toBeVisible();
  await page.getByTestId(`plugin-detail-tab-${tab}`).click();
}

test.describe('plugin dependency template', () => {
  test.beforeEach(async ({page}) => {
    await installPluginDependencyTemplateRoutes(page);
  });

  test('disables and restores an official dependency with a deterministic fixture', async ({
    page,
  }) => {
    await openJdbcTemplateDialog(page, 'dependencies');

    await page.getByTestId('plugin-profile-mysql').click();
    const officialDependencies = page.getByTestId(
      'plugin-official-dependencies',
    );
    const activeDependencies = page.getByTestId(
      'plugin-active-official-dependencies',
    );
    await expect(officialDependencies).toContainText(
      pluginDependencyTemplate.mysqlConnectorArtifactId,
    );
    await expect(activeDependencies).toContainText(
      pluginDependencyTemplate.mysqlDriverArtifactId,
    );

    await page
      .getByTestId(
        `plugin-disable-official-${pluginDependencyTemplate.mysqlDriverArtifactId}`,
      )
      .click();
    const disabledDependencies = page.getByTestId(
      'plugin-disabled-official-dependencies',
    );
    await expect(disabledDependencies).toContainText(
      pluginDependencyTemplate.mysqlDriverArtifactId,
    );

    await page
      .getByTestId(
        `plugin-enable-official-${pluginDependencyTemplate.mysqlDriverArtifactId}`,
      )
      .click();
    await expect(
      page.getByTestId(
        `plugin-disable-official-${pluginDependencyTemplate.mysqlDriverArtifactId}`,
      ),
    ).toBeVisible();
    await expect(
      page.getByTestId('plugin-disabled-official-dependencies'),
    ).toHaveCount(0);
  });

  test('uploads and removes a custom dependency within the same template flow', async ({
    page,
  }, testInfo) => {
    await openJdbcTemplateDialog(page, 'custom');

    const uploadFilePath = await createPlaceholderJar(
      testInfo.outputPath(pluginDependencyTemplate.uploadFileName),
    );

    await page.getByTestId('plugin-upload-dependency-trigger').click();
    await expect(
      page.getByTestId('plugin-upload-dependency-form'),
    ).toBeVisible();
    await page
      .getByTestId('plugin-upload-jar-input')
      .setInputFiles(uploadFilePath);
    await page
      .getByTestId('plugin-upload-dependency-form')
      .getByRole('button', {name: /上传 JAR|Upload JAR/i})
      .click();

    const customDependencyRow = page.getByTestId(
      `plugin-custom-dependency-${pluginDependencyTemplate.mysqlDriverArtifactId}`,
    );
    await expect(customDependencyRow).toBeVisible();
    await expect(customDependencyRow).toContainText(
      pluginDependencyTemplate.uploadFileName,
    );

    await page
      .getByTestId(
        `plugin-delete-custom-dependency-${pluginDependencyTemplate.mysqlDriverArtifactId}`,
      )
      .click();
    await expect(
      page.getByTestId(
        `plugin-custom-dependency-${pluginDependencyTemplate.mysqlDriverArtifactId}`,
      ),
    ).toHaveCount(0);
    await expect(
      page.getByText(/暂无自定义驱动配置|No custom drivers configured/i),
    ).toBeVisible();
  });

  test('offers cluster uninstall choices when deleting an installed local plugin', async ({
    page,
  }) => {
    await page.goto('/plugins');
    await expect(page.getByTestId('plugin-tab-local')).toBeVisible();

    await page.getByTestId('plugin-tab-local').click();
    await expect(
      page.getByTestId(
        `plugin-local-row-${pluginDependencyTemplate.pluginName}-${pluginDependencyTemplate.seatunnelVersion}`,
      ),
    ).toContainText('template-cluster');

    await page
      .getByTestId(
        `plugin-local-delete-${pluginDependencyTemplate.pluginName}-${pluginDependencyTemplate.seatunnelVersion}`,
      )
      .click();
    await expect(
      page.getByText(
        /该插件已安装到以下集群|This plugin is installed on these clusters/i,
      ),
    ).toBeVisible();
    await page.getByTestId('plugin-delete-uninstall-cluster-101').click();
    await page
      .getByRole('button', {
        name: /卸载 1 个集群并删除|Uninstall 1 clusters and delete/i,
      })
      .click();

    await expect(
      page.getByTestId(
        `plugin-local-row-${pluginDependencyTemplate.pluginName}-${pluginDependencyTemplate.seatunnelVersion}`,
      ),
    ).toHaveCount(0);
  });
});
