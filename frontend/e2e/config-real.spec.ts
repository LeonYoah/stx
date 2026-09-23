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

import fs from 'node:fs/promises';
import path from 'node:path';
import {expect, test} from '@playwright/test';
import {
  resolveInstalledConfigPaths,
  waitForFileContent,
} from './helpers/install-wizard-real';
import {
  initClusterConfigsFromNode,
  initClusterConfigsFromNodeApi,
  listClusterConfigs,
  listClusterConfigVersions,
  openClusterConfigsTab,
  openTemplateConfigEditor,
  rollbackClusterConfig,
  selectClusterConfigType,
  syncClusterTemplateToAllNodesApi,
  updateClusterConfig,
  waitForAllNodeConfigsMatchingTemplate,
  waitForFileToContain,
  waitForTemplateConfig,
} from './helpers/config-real';
import {installSourceCluster} from './helpers/upgrade-real';
import {ConfigType} from '@/lib/services/config';

const seatunnelVersion = process.env.E2E_INSTALLER_REAL_VERSION ?? '2.3.13';
const tmpDirRoot =
  process.env.E2E_INSTALLER_REAL_TMP_DIR ??
  path.resolve(process.cwd(), '../tmp/e2e/installer-real');
const installDir = path.join(
  tmpDirRoot,
  'install',
  `config-real-seatunnel-${seatunnelVersion}`,
);
const clusterPort = Number(
  process.env.E2E_INSTALLER_REAL_CLUSTER_PORT_PRIMARY || '38181',
);
const httpPort = Number(
  process.env.E2E_INSTALLER_REAL_HTTP_PORT_PRIMARY || '38080',
);
const seatunnelConfigOption = /^(SeaTunnel 配置|SeaTunnel Config)$/i;

test.describe.serial('config real e2e', () => {
  test('manages global cluster configs with smart repair, sync, history and live import', async ({
    page,
  }) => {
    test.slow();
    console.log(
      `[config-real] installing real source cluster ${seatunnelVersion} at ${installDir}`,
    );

    const cluster = await installSourceCluster(page, {
      sourceVersion: seatunnelVersion,
      installDir,
      clusterPort,
      httpPort,
    });
    const files = resolveInstalledConfigPaths(installDir);
    const request = page.context().request;
    const repairedNamespace = '/tmp/seatunnel/checkpoint-config-real/';
    const importedNamespace = '/tmp/seatunnel/checkpoint-imported-from-node/';

    await openClusterConfigsTab(page, cluster.clusterId);
    await selectClusterConfigType(page, seatunnelConfigOption);
    await initClusterConfigsFromNode(page);
    const initialTemplate = await waitForTemplateConfig(
      request,
      cluster.clusterId,
      ConfigType.SEATUNNEL,
      () => true,
    );
    console.log('[config-real] cluster configs initialized from node');

    await openTemplateConfigEditor(page);
    const brokenSeatunnelConfig = `seatunnel:
    engine:
        http:
            enable-http: true
            port: ${httpPort}
        checkpoint:
            interval: 10000
      storage:
        type: hdfs
        plugin-config:
          namespace: ${repairedNamespace}
          storage.type: hdfs
          fs.defaultFS: file:///
`;

    const editContent = page.getByTestId('cluster-configs-edit-content');
    await editContent.fill(brokenSeatunnelConfig);
    await page.getByTestId('cluster-configs-smart-repair').click();
    await expect(editContent).toHaveValue(new RegExp(repairedNamespace));
    await expect(editContent).toHaveValue(
      /checkpoint:\n\s+interval: 10000\n\s+storage:/,
    );
    console.log('[config-real] smart repair normalized seatunnel.yaml');

    const repairedSeatunnelContent = await editContent.inputValue();
    await updateClusterConfig(
      request,
      initialTemplate.id,
      repairedSeatunnelContent,
      'config-real normalized template',
    );
    await waitForTemplateConfig(
      request,
      cluster.clusterId,
      ConfigType.SEATUNNEL,
      (config) => config.content.includes(repairedNamespace),
    );
    await page.reload();
    await page.getByTestId('cluster-detail-tab-configs').click();
    await selectClusterConfigType(page, seatunnelConfigOption);
    await expect(
      page.getByTestId('cluster-configs-pending-sync'),
    ).toContainText(/1/);
    console.log('[config-real] template saved without node sync');

    await syncClusterTemplateToAllNodesApi(
      request,
      cluster.clusterId,
      ConfigType.SEATUNNEL,
    );
    await waitForFileToContain(
      files.seatunnel,
      [`namespace: ${repairedNamespace}`, `port: ${httpPort}`],
      60000,
    );
    await waitForAllNodeConfigsMatchingTemplate(
      request,
      cluster.clusterId,
      ConfigType.SEATUNNEL,
    );
    await page.reload();
    await page.getByTestId('cluster-detail-tab-configs').click();
    await selectClusterConfigType(page, seatunnelConfigOption);
    console.log('[config-real] template synced to node and file updated');

    await page.getByTestId('cluster-configs-template-versions').click();
    await expect(
      page.getByTestId('cluster-configs-versions-dialog'),
    ).toBeVisible({timeout: 30000});
    await expect(
      page.getByTestId('cluster-configs-version-preview-content'),
    ).toContainText(repairedNamespace);
    console.log('[config-real] version preview verified');

    const compareButton = page
      .locator(
        '[data-testid^="cluster-configs-version-compare-"]:not([disabled])',
      )
      .first();
    await expect(compareButton).toBeVisible({timeout: 30000});
    await compareButton.click();
    await expect(
      page.getByTestId('cluster-configs-version-compare-content'),
    ).toContainText('/tmp/seatunnel/checkpoint/');
    await expect(
      page.getByTestId('cluster-configs-version-compare-content'),
    ).toContainText(repairedNamespace);
    console.log('[config-real] version compare verified');

    const versions = await listClusterConfigVersions(
      request,
      initialTemplate.id,
    );
    const rollbackTarget = versions.find((version) =>
      version.content.includes('/tmp/seatunnel/checkpoint/'),
    );
    expect(rollbackTarget).toBeTruthy();
    await rollbackClusterConfig(
      request,
      initialTemplate.id,
      rollbackTarget!.version,
      `Rollback to v${rollbackTarget!.version}`,
    );
    await waitForTemplateConfig(
      request,
      cluster.clusterId,
      ConfigType.SEATUNNEL,
      (config) => config.content.includes('/tmp/seatunnel/checkpoint/'),
    );
    await page.reload();
    await page.getByTestId('cluster-detail-tab-configs').click();
    await selectClusterConfigType(page, seatunnelConfigOption);
    await expect(page.getByTestId('cluster-configs-pending-sync')).toBeVisible({
      timeout: 30000,
    });
    await syncClusterTemplateToAllNodesApi(
      request,
      cluster.clusterId,
      ConfigType.SEATUNNEL,
    );
    await waitForFileToContain(
      files.seatunnel,
      ['namespace: /tmp/seatunnel/checkpoint/'],
      60000,
    );
    await waitForAllNodeConfigsMatchingTemplate(
      request,
      cluster.clusterId,
      ConfigType.SEATUNNEL,
    );
    await page.reload();
    await page.getByTestId('cluster-detail-tab-configs').click();
    await selectClusterConfigType(page, seatunnelConfigOption);
    console.log('[config-real] rollback synced back to node');

    const liveSeatunnelConfig = await waitForFileContent(files.seatunnel);
    const importedSeatunnelConfig = liveSeatunnelConfig.replace(
      '/tmp/seatunnel/checkpoint/',
      importedNamespace,
    );
    expect(importedSeatunnelConfig).not.toBe(liveSeatunnelConfig);
    await fs.writeFile(files.seatunnel, importedSeatunnelConfig, 'utf8');
    console.log('[config-real] live seatunnel.yaml modified out-of-band');

    await initClusterConfigsFromNodeApi(
      request,
      cluster.clusterId,
      cluster.hostId,
      cluster.installDir,
    );
    await waitForTemplateConfig(
      request,
      cluster.clusterId,
      ConfigType.SEATUNNEL,
      (config) => config.content.includes(importedNamespace),
      60000,
    );
    const refreshedConfigs = await listClusterConfigs(
      request,
      cluster.clusterId,
    );
    expect(
      refreshedConfigs.some(
        (config) =>
          config.is_template &&
          config.config_type === ConfigType.SEATUNNEL &&
          config.content.includes(importedNamespace),
      ),
    ).toBeTruthy();
    await page.reload();
    await page.getByTestId('cluster-detail-tab-configs').click();
    await selectClusterConfigType(page, seatunnelConfigOption);
    await expect(
      page.getByTestId('cluster-configs-template-content'),
    ).toContainText(importedNamespace, {timeout: 15000});
    console.log(
      '[config-real] init from node refreshed template from live file',
    );
  });
});
