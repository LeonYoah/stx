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

import path from 'node:path';
import {expect, test} from '@playwright/test';
import {installSourceCluster} from './helpers/upgrade-real';
import {
  assertLocalPluginAssets,
  downloadPluginApi,
  installPluginToClusterApi,
  waitForPluginDownloadCompleted,
  waitForInstalledPlugin,
  waitForLocalPlugin,
} from './helpers/plugin-real';

const seatunnelVersion = process.env.E2E_INSTALLER_REAL_VERSION ?? '2.3.13';
const tmpDirRoot =
  process.env.E2E_INSTALLER_REAL_TMP_DIR ??
  path.resolve(process.cwd(), '../tmp/e2e/installer-real');
const installDir = path.join(
  tmpDirRoot,
  'install',
  `plugin-real-seatunnel-${seatunnelVersion}`,
);
const clusterPort = Number(
  process.env.E2E_INSTALLER_REAL_CLUSTER_PORT_PRIMARY || '38181',
);
const httpPort = Number(
  process.env.E2E_INSTALLER_REAL_HTTP_PORT_PRIMARY || '38080',
);

test.describe.serial('plugin real e2e', () => {
  test('downloads real plugin assets and shows installed plugin details in cluster view', async ({
    page,
  }) => {
    test.slow();
    const request = page.context().request;
    console.log(
      `[plugin-real] installing real source cluster ${seatunnelVersion} at ${installDir}`,
    );

    const cluster = await installSourceCluster(page, {
      sourceVersion: seatunnelVersion,
      installDir,
      clusterPort,
      httpPort,
    });

    await page.goto('/plugins');
    await expect(page.getByTestId('plugin-marketplace-root')).toBeVisible({
      timeout: 120000,
    });

    await page.getByTestId('plugin-search-input').fill('jdbc');
    await expect(page.getByTestId('plugin-card-jdbc')).toBeVisible({
      timeout: 120000,
    });
    await page.getByTestId('plugin-card-jdbc').click();
    await expect(page.getByTestId('plugin-detail-dialog-jdbc')).toBeVisible();
    await page.getByTestId('plugin-detail-tab-dependencies').click();
    await page.getByTestId('plugin-profile-mysql').click();
    await expect(
      page.getByTestId('plugin-active-official-dependencies'),
    ).toContainText(/mysql-connector-java/i, {timeout: 30000});
    await downloadPluginApi(request, 'jdbc', seatunnelVersion, ['mysql']);
    await waitForPluginDownloadCompleted(request, 'jdbc', seatunnelVersion, [
      'mysql',
    ]);
    console.log(
      '[plugin-real] jdbc mysql download triggered via api with fixed version',
    );

    const jdbcLocalPlugin = await waitForLocalPlugin(
      request,
      'jdbc',
      seatunnelVersion,
      (plugin) =>
        (plugin.selected_profile_keys || []).includes('mysql') &&
        (plugin.dependencies || []).some(
          (dependency) => dependency.artifact_id === 'mysql-connector-java',
        ),
    );
    await assertLocalPluginAssets(jdbcLocalPlugin);
    console.log('[plugin-real] jdbc metadata and local assets verified');

    await installPluginToClusterApi(
      request,
      cluster.clusterId,
      'jdbc',
      seatunnelVersion,
      ['mysql'],
    );
    await waitForInstalledPlugin(
      request,
      cluster.clusterId,
      'jdbc',
      seatunnelVersion,
      (plugin) =>
        (plugin.selected_profile_keys || []).includes('mysql') &&
        (plugin.dependencies || []).some(
          (dependency) => dependency.artifact_id === 'mysql-connector-java',
        ),
    );
    console.log('[plugin-real] jdbc installed onto real cluster');

    await page.goto(`/clusters/${cluster.clusterId}`);
    await page.getByTestId('cluster-detail-tab-plugins').click();
    await expect(
      page.getByTestId(`cluster-plugin-row-jdbc-${seatunnelVersion}`),
    ).toBeVisible({timeout: 30000});
    await page
      .getByTestId(`cluster-plugin-view-jdbc-${seatunnelVersion}`)
      .click();
    await expect(
      page.getByTestId('cluster-plugin-detail-dialog-jdbc'),
    ).toBeVisible({timeout: 30000});
    await expect(
      page.getByTestId('cluster-plugin-detail-dialog-jdbc'),
    ).toContainText(/mysql/);
    await expect(
      page.getByTestId('cluster-plugin-detail-dialog-jdbc'),
    ).toContainText(/mysql-connector-java/i);
    console.log(
      '[plugin-real] cluster plugin detail shows selected profile and dependency',
    );

    await page.goto('/plugins');
    await page.getByTestId('plugin-search-input').fill('file-obs');
    await expect(page.getByTestId('plugin-card-file-obs')).toBeVisible({
      timeout: 120000,
    });
    await downloadPluginApi(request, 'file-obs', seatunnelVersion);
    await waitForPluginDownloadCompleted(
      request,
      'file-obs',
      seatunnelVersion,
      undefined,
      900000,
    );
    console.log(
      '[plugin-real] file-obs download triggered via api with fixed version',
    );

    const obsLocalPlugin = await waitForLocalPlugin(
      request,
      'file-obs',
      seatunnelVersion,
    );
    await assertLocalPluginAssets(obsLocalPlugin);
    expect(
      (obsLocalPlugin.dependencies || []).length === 0 ||
        (obsLocalPlugin.dependencies || []).some((dependency) =>
          ['lib', 'connectors'].includes(dependency.target_dir || ''),
        ),
    ).toBeTruthy();
    console.log('[plugin-real] file-obs lib assets verified');
  });
});
