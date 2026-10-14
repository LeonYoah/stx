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

import type {Page} from '@playwright/test';

function ok<T>(data: T) {
  return {
    error_msg: '',
    data,
  };
}

const now = '2026-03-20T12:30:00Z';
const clusterId = 1;
const sourceVersion = '2.3.13';
const targetVersion = '2.3.13';
const targetInstallDir = '/opt/seatunnel-2.3.13';
const blockedIssueCode = 'package_missing';
const blockedIssueMessage =
  'Target package 2.3.13 is not available yet; upload it before continuing.';

function buildReadyPrecheck() {
  return {
    cluster_id: clusterId,
    target_version: targetVersion,
    ready: true,
    issues: [],
    package_manifest: {
      version: targetVersion,
      file_name: `apache-seatunnel-${targetVersion}-bin.tar.gz`,
      local_path: `/tmp/packages/apache-seatunnel-${targetVersion}-bin.tar.gz`,
      checksum: 'sha256:seatunnel-upgrade-bundle',
      arch: 'linux-amd64',
      size_bytes: 512 * 1024 * 1024,
      source: 'local_package',
    },
    connector_manifest: {
      version: targetVersion,
      replacement_mode: 'incremental',
      connectors: [
        {
          plugin_name: 'jdbc',
          artifact_id: 'connector-jdbc',
          version: targetVersion,
          category: 'connector',
          file_name: `connector-jdbc-${targetVersion}.jar`,
          local_path: `/tmp/plugins/connector-jdbc-${targetVersion}.jar`,
          checksum: 'sha256:connector-jdbc',
          source: 'local_plugin',
          required: true,
        },
      ],
      libraries: [
        {
          group_id: 'org.apache.seatunnel',
          artifact_id: 'seatunnel-transforms-v2',
          version: targetVersion,
          file_name: `seatunnel-transforms-v2-${targetVersion}.jar`,
          local_path: `/tmp/plugins/seatunnel-transforms-v2-${targetVersion}.jar`,
          checksum: 'sha256:transforms',
          source: 'local_plugin',
          scope: 'shared',
        },
      ],
      plugin_dependencies: [
        {
          group_id: 'com.mysql',
          artifact_id: 'mysql-connector-j',
          version: '8.0.33',
          file_name: 'mysql-connector-j-8.0.33.jar',
          local_path: '/tmp/plugins/mysql-connector-j-8.0.33.jar',
          checksum: 'sha256:mysql-driver',
          source: 'local_plugin',
          target_dir: 'plugins/Jdbc/lib',
          relative_path: 'plugins/Jdbc/lib/mysql-connector-j-8.0.33.jar',
        },
      ],
    },
    config_merge_plan: {
      ready: true,
      has_conflicts: false,
      conflict_count: 0,
      generated_at: now,
      files: [
        {
          config_type: 'seatunnel.yaml',
          target_path: `${targetInstallDir}/config/seatunnel.yaml`,
          base_content: 'base: value\n',
          local_content: 'local: value\n',
          target_content: 'target: value\n',
          merged_content: 'local: value\ntarget: value\n',
          conflict_count: 0,
          resolved: true,
          conflicts: [],
        },
      ],
    },
    node_targets: [
      {
        cluster_node_id: 101,
        host_id: 11,
        host_name: 'worker-a',
        host_ip: '10.0.0.11',
        role: 'master/worker',
        arch: 'linux-amd64',
        source_version: sourceVersion,
        target_version: targetVersion,
        source_install_dir: '/opt/seatunnel-2.3.13',
        target_install_dir: targetInstallDir,
      },
    ],
    generated_at: now,
  };
}

function buildBlockedPrecheck() {
  return {
    cluster_id: clusterId,
    target_version: targetVersion,
    ready: false,
    issues: [
      {
        category: 'package',
        code: blockedIssueCode,
        message: blockedIssueMessage,
        blocking: true,
        metadata: {
          target_version: targetVersion,
        },
      },
    ],
    connector_manifest: {
      version: targetVersion,
      replacement_mode: 'incremental',
      connectors: [],
      libraries: [],
      plugin_dependencies: [],
    },
    config_merge_plan: {
      ready: false,
      has_conflicts: false,
      conflict_count: 0,
      generated_at: now,
      files: [],
    },
    node_targets: [
      {
        cluster_node_id: 101,
        host_id: 11,
        host_name: 'worker-a',
        host_ip: '10.0.0.11',
        role: 'master/worker',
        arch: 'linux-amd64',
        source_version: sourceVersion,
        target_version: targetVersion,
        source_install_dir: '/opt/seatunnel-2.3.13',
        target_install_dir: targetInstallDir,
      },
    ],
    generated_at: now,
  };
}

export interface UpgradePrepareTemplateRouteOptions {
  precheckScenario?: 'ready' | 'blocked';
}

export async function installUpgradePrepareTemplateRoutes(
  page: Page,
  options?: UpgradePrepareTemplateRouteOptions,
): Promise<void> {
  const precheckFixture =
    options?.precheckScenario === 'blocked'
      ? buildBlockedPrecheck()
      : buildReadyPrecheck();

  await page.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const {pathname} = url;

    if (
      request.method() === 'GET' &&
      pathname === `/api/v1/clusters/${clusterId}`
    ) {
      await route.fulfill({
        status: 200,
        json: ok({
          id: clusterId,
          name: 'orders-cluster',
          description: 'Upgrade rehearsal cluster',
          deployment_mode: 'hybrid',
          version: sourceVersion,
          status: 'running',
          install_dir: '/opt/seatunnel-2.3.13',
          config: {},
          node_count: 1,
          online_nodes: 1,
          health_status: 'healthy',
          created_at: now,
          updated_at: now,
        }),
      });
      return;
    }

    if (request.method() === 'GET' && pathname === '/api/v1/packages') {
      await route.fulfill({
        status: 200,
        json: ok({
          versions: [sourceVersion, targetVersion],
          recommended_version: targetVersion,
          local_packages: [
            {
              version: targetVersion,
              file_name: `apache-seatunnel-${targetVersion}-bin.tar.gz`,
              file_size: 512 * 1024 * 1024,
              checksum: 'sha256:seatunnel-upgrade-bundle',
              download_urls: {
                aliyun: 'https://mirror.aliyun.com/apache/seatunnel/',
                apache: 'https://archive.apache.org/dist/seatunnel/',
                huaweicloud: 'https://repo.huaweicloud.com/apache/seatunnel/',
              },
              is_local: true,
              local_path: `/tmp/packages/apache-seatunnel-${targetVersion}-bin.tar.gz`,
              uploaded_at: now,
            },
          ],
        }),
      });
      return;
    }

    if (
      request.method() === 'POST' &&
      pathname === '/api/v1/st-upgrade/precheck'
    ) {
      await route.fulfill({
        status: 200,
        json: ok(precheckFixture),
      });
      return;
    }

    await route.continue();
  });
}

export const upgradePrepareTemplate = {
  clusterId,
  targetVersion,
  targetInstallDir,
};

export const upgradePrepareBlockedTemplate = {
  clusterId,
  targetVersion,
  targetInstallDir,
  blockedIssueCode,
  blockedIssueMessage,
};
