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
import {
  assertFileContains,
  chooseSelectOption,
  resolveInstalledConfigPaths,
} from './helpers/install-wizard-real';
import {
  ensureLocalPackage,
  installSourceCluster,
  waitForClusterVersion,
  waitForUpgradeTaskSuccess,
} from './helpers/upgrade-real';

const sourceVersion = process.env.E2E_UPGRADE_REAL_SOURCE_VERSION ?? '2.3.12';
const targetVersion =
  process.env.E2E_UPGRADE_REAL_TARGET_VERSION ??
  process.env.E2E_INSTALLER_REAL_VERSION ??
  '2.3.13';
const tmpDirRoot =
  process.env.E2E_INSTALLER_REAL_TMP_DIR ??
  path.resolve(process.cwd(), '../tmp/e2e/installer-real');
const sourceInstallDir = path.join(
  tmpDirRoot,
  'install',
  `upgrade-seatunnel-${sourceVersion}-source`,
);
const targetInstallDir = path.join(
  tmpDirRoot,
  'install',
  `upgrade-seatunnel-${targetVersion}-target`,
);
const clusterPort = Number(
  process.env.E2E_INSTALLER_REAL_CLUSTER_PORT_PRIMARY || '38181',
);
const httpPort = Number(
  process.env.E2E_INSTALLER_REAL_HTTP_PORT_PRIMARY || '38080',
);

test.describe.serial('upgrade real e2e', () => {
  test('upgrades an installed single-node cluster to the prepared target version', async ({
    page,
  }) => {
    test.slow();
    console.log(
      `[upgrade-real] installing source cluster ${sourceVersion} at ${sourceInstallDir}`,
    );

    const cluster = await installSourceCluster(page, {
      sourceVersion,
      installDir: sourceInstallDir,
      clusterPort,
      httpPort,
    });

    console.log(
      `[upgrade-real] source cluster installed clusterId=${cluster.clusterId}`,
    );

    await ensureLocalPackage(page, targetVersion, 'apache');
    console.log(
      `[upgrade-real] target package ${targetVersion} confirmed local`,
    );

    console.log(
      `[upgrade-real] opening prepare page /clusters/${cluster.clusterId}/upgrade/prepare`,
    );
    await page.goto(`/clusters/${cluster.clusterId}/upgrade/prepare`);
    await expect(page.getByTestId('upgrade-prepare-page')).toBeVisible();
    console.log('[upgrade-real] prepare page visible');

    await chooseSelectOption(
      page,
      'upgrade-prepare-target-version',
      new RegExp(targetVersion.replace(/\./g, '\\.')),
    );
    console.log(`[upgrade-real] selected target version ${targetVersion}`);
    await page
      .getByTestId('upgrade-prepare-target-install-dir')
      .fill(targetInstallDir);
    console.log(`[upgrade-real] filled target install dir ${targetInstallDir}`);

    await page.getByTestId('upgrade-prepare-run-precheck').click();
    console.log('[upgrade-real] precheck requested');
    await expect(page.getByTestId('upgrade-prepare-ready-badge')).toContainText(
      /可继续|就绪|Ready/i,
      {
        timeout: 180000,
      },
    );
    console.log('[upgrade-real] precheck indicates ready to continue');

    await expect(page.getByTestId('upgrade-prepare-continue')).toBeEnabled({
      timeout: 180000,
    });
    await page.getByTestId('upgrade-prepare-continue').click();
    console.log('[upgrade-real] continuing to config page');
    await page.waitForURL(
      new RegExp(`/clusters/${cluster.clusterId}/upgrade/config`),
      {
        timeout: 120000,
      },
    );
    await expect(page.getByTestId('upgrade-config-page')).toBeVisible();
    console.log('[upgrade-real] config page visible');

    const createPlanButton = page.getByTestId('upgrade-config-create-plan');
    if (!(await createPlanButton.isEnabled())) {
      console.log('[upgrade-real] merge conflicts detected, resolving by applying old files');
      const configTabs = page.getByRole('tab');
      const tabCount = await configTabs.count();
      for (let index = 0; index < tabCount; index += 1) {
        await configTabs.nth(index).click();
        const useOldFileButton = page.getByRole('button', {
          name: /整份采用旧版本|Use Old File/i,
        });
        if (await useOldFileButton.isVisible()) {
          await useOldFileButton.click();
          console.log(`[upgrade-real] applied old file on config tab ${index + 1}/${tabCount}`);
        }
      }
    }

    await expect(createPlanButton).toBeEnabled({timeout: 30000});
    await createPlanButton.click();
    console.log('[upgrade-real] create plan requested');
    await page.waitForURL(
      new RegExp(`/clusters/${cluster.clusterId}/upgrade/execute\\?planId=`),
      {
        timeout: 120000,
      },
    );
    await expect(page.getByTestId('upgrade-execute-page')).toBeVisible();

    await page.getByTestId('upgrade-execute-start').click();
    await page.waitForURL(/taskId=\d+/, {timeout: 120000});
    const taskId = Number(new URL(page.url()).searchParams.get('taskId'));
    expect(taskId).toBeGreaterThan(0);
    console.log(`[upgrade-real] upgrade execution started taskId=${taskId}`);

    await waitForUpgradeTaskSuccess(page, taskId, [
      'SWITCH_VERSION',
      'START_CLUSTER',
      'RESTART_JAVA_PROXY',
      'HEALTH_CHECK',
      'SMOKE_TEST',
      'COMPLETE',
    ]);

    await waitForClusterVersion(
      page,
      cluster.clusterId,
      targetVersion,
      targetInstallDir,
    );
    await page.reload();
    await expect(page.getByTestId('upgrade-execute-success')).toBeVisible({
      timeout: 120000,
    });

    const files = resolveInstalledConfigPaths(targetInstallDir);
    await assertFileContains(files.seatunnel, [
      'enable-http: true',
      `port: ${httpPort}`,
      'namespace: /tmp/seatunnel/checkpoint/',
      'fs.defaultFS: file:///',
    ]);
    await assertFileContains(files.hazelcast, ['enabled: false']);
    await assertFileContains(files.hazelcastClient, [
      'cluster-members:',
      `- ${cluster.hostIP}:${clusterPort}`,
    ]);
    await assertFileContains(files.log4j2, [
      'rootLogger.appenderRef.file.ref = routingAppender',
    ]);

    console.log(
      `[upgrade-real] upgrade finished successfully clusterId=${cluster.clusterId} target=${targetVersion}`,
    );
  });
});
