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
import {chooseSelectOption} from './helpers/install-wizard-real';
import {
  ensureLocalPackage,
  installSourceCluster,
} from './helpers/upgrade-real';
import {
  downloadPluginApi,
  installPluginToClusterApi,
  waitForInstalledPlugin,
  waitForPluginDownloadCompleted,
} from './helpers/plugin-real';

const sourceVersion = process.env.E2E_UPGRADE_REAL_SOURCE_VERSION ?? '2.3.13';
const targetVersion = process.env.E2E_UPGRADE_REAL_TARGET_VERSION ?? '3.0.0';
const tmpDirRoot =
  process.env.E2E_INSTALLER_REAL_TMP_DIR ??
  path.resolve(process.cwd(), '../tmp/e2e/installer-real');
const sourceInstallDir = path.join(
  tmpDirRoot,
  'install',
  `upgrade-profiles-missing-source-${sourceVersion}`,
);
const targetInstallDir = path.join(
  tmpDirRoot,
  'install',
  `upgrade-profiles-missing-target-${targetVersion}`,
);
const clusterPort = Number(
  process.env.E2E_INSTALLER_REAL_CLUSTER_PORT_PRIMARY || '38181',
);
const httpPort = Number(
  process.env.E2E_INSTALLER_REAL_HTTP_PORT_PRIMARY || '38080',
);

/**
 * AC1: jdbc installed on source with empty selected_profile_keys + target jdbc
 * local with empty/no profiles metadata + HasApplicableDependencyProfiles →
 * upgrade precheck Blocked with profiles_missing (Continue disabled).
 *
 * Critical: download target jdbc with profile_keys [] so local metadata does NOT
 * fill selected_profile_keys (a mysql target download would make ResolvePluginProfiles
 * succeed and profiles_missing would not fire). See planning.go ~400-428.
 *
 * Fallback if download with [] is rejected by product: seed connector jar for
 * target version under storage/plugins and install source with profile_keys [].
 */
test.describe.serial('upgrade real profiles_missing fail-closed', () => {
  test('precheck blocks jdbc when dependency profiles exist but none resolve', async ({
    page,
  }) => {
    test.slow();
    const request = page.context().request;
    console.log(
      `[upgrade-profiles-missing] installing source cluster ${sourceVersion} at ${sourceInstallDir}`,
    );

    const cluster = await installSourceCluster(page, {
      sourceVersion,
      installDir: sourceInstallDir,
      clusterPort,
      httpPort,
    });
    await ensureLocalPackage(page, targetVersion, 'apache');

    // Source: install jdbc without selecting a profile → empty SelectedProfileKeys.
    await downloadPluginApi(request, 'jdbc', sourceVersion, []);
    await waitForPluginDownloadCompleted(request, 'jdbc', sourceVersion, []);
    await installPluginToClusterApi(
      request,
      cluster.clusterId,
      'jdbc',
      sourceVersion,
      [],
    );
    await waitForInstalledPlugin(
      request,
      cluster.clusterId,
      'jdbc',
      sourceVersion,
      (p) => !(p.selected_profile_keys || []).length,
    );
    console.log(
      '[upgrade-profiles-missing] source jdbc installed with empty selected_profile_keys',
    );

    // Target local package must exist WITHOUT selected_profile_keys metadata.
    await downloadPluginApi(request, 'jdbc', targetVersion, []);
    await waitForPluginDownloadCompleted(request, 'jdbc', targetVersion, []);
    console.log(
      `[upgrade-profiles-missing] target jdbc ${targetVersion} downloaded with empty profiles`,
    );

    await page.goto(`/clusters/${cluster.clusterId}/upgrade/prepare`);
    await expect(page.getByTestId('upgrade-prepare-page')).toBeVisible();
    await chooseSelectOption(
      page,
      'upgrade-prepare-target-version',
      new RegExp(targetVersion.replace(/\./g, '\\.')),
    );
    await page
      .getByTestId('upgrade-prepare-target-install-dir')
      .fill(targetInstallDir);
    await page.getByTestId('upgrade-prepare-run-precheck').click();
    console.log('[upgrade-profiles-missing] precheck requested');

    await expect(page.getByTestId('upgrade-prepare-ready-badge')).toContainText(
      /存在阻断|Blocked/i,
      {timeout: 180000},
    );
    await expect(page.getByTestId('upgrade-prepare-continue')).toBeDisabled();
    await expect(page.getByTestId('upgrade-prepare-issues')).toContainText(
      /profiles_missing/,
    );
    await expect(page.getByTestId('upgrade-prepare-issues')).toContainText(
      /jdbc/i,
    );
    console.log(
      '[upgrade-profiles-missing] precheck blocked with profiles_missing for jdbc',
    );
  });
});
