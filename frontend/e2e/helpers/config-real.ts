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

import {expect, type APIRequestContext, type Page} from '@playwright/test';
import fs from 'node:fs/promises';
import {ConfigType} from '@/lib/services/config';

const backendBaseURL =
  process.env.E2E_BACKEND_BASE_URL ?? 'http://127.0.0.1:18000';

export interface RealConfigInfo {
  id: number;
  config_type: ConfigType | string;
  is_template: boolean;
  match_template: boolean;
  version: number;
  content: string;
}

interface ClusterConfigsResponse {
  error_msg?: string;
  data?: RealConfigInfo[];
}

interface ClusterConfigResponse {
  error_msg?: string;
  data?: RealConfigInfo;
}

interface ClusterConfigVersionsResponse {
  error_msg?: string;
  data?: Array<{
    id: number;
    config_id: number;
    version: number;
    content: string;
    comment: string;
    created_by: number;
    created_at: string;
  }>;
}

interface ClusterConfigSyncAllResponse {
  error_msg?: string;
  data?: {
    message: string;
    synced_count: number;
    push_errors: Array<{host_id: number; host_ip?: string; message: string}>;
  };
}

export async function openClusterConfigsTab(
  page: Page,
  clusterId: number,
): Promise<void> {
  await page.goto(`/clusters/${clusterId}`);
  await page.getByTestId('cluster-detail-tab-configs').click();
  await expect(page.getByTestId('cluster-configs-root')).toBeVisible({
    timeout: 120000,
  });
}

export async function selectClusterConfigType(
  page: Page,
  optionName: RegExp | string,
): Promise<void> {
  const fileList = page.getByTestId('cluster-configs-file-list');
  await expect(fileList).toBeVisible({timeout: 30000});
  const target = fileList
    .getByRole('button', {name: optionName})
    .first();
  await expect(target).toBeVisible({timeout: 30000});
  await target.click();
}

export async function initClusterConfigsFromNode(page: Page): Promise<void> {
  await page.getByTestId('cluster-configs-init-button').click();
  const nodeChoices = page.locator('[data-testid^="cluster-configs-init-node-"]');
  await expect(nodeChoices.first()).toBeVisible({timeout: 30000});
  await nodeChoices.first().click();
  await page.getByTestId('cluster-configs-init-confirm').click();
}

export async function initClusterConfigsFromNodeApi(
  request: APIRequestContext,
  clusterId: number,
  hostId: number,
  installDir: string,
): Promise<void> {
  const response = await request.post(
    `${backendBaseURL}/api/v1/clusters/${clusterId}/configs/init`,
    {
      data: {host_id: hostId, install_dir: installDir},
    },
  );
  expect(response.ok()).toBeTruthy();
  const payload = (await response.json()) as {error_msg?: string};
  expect(payload.error_msg ?? '').toBe('');
}

export async function openTemplateConfigEditor(page: Page): Promise<void> {
  await page.getByTestId('cluster-configs-template-edit').click();
  await expect(page.getByTestId('cluster-configs-edit-dialog')).toBeVisible({
    timeout: 30000,
  });
}

export async function syncTemplateConfigToAllNodes(page: Page): Promise<void> {
  const bannerAction = page.getByTestId('cluster-configs-template-sync-all-banner');
  if ((await bannerAction.count()) > 0 && (await bannerAction.first().isVisible())) {
    await bannerAction.first().click();
    return;
  }

  await page.getByTestId('cluster-configs-template-sync-all').click();
}

export async function waitForFileToContain(
  filePath: string,
  snippets: string[],
  timeoutMs: number = 120000,
): Promise<void> {
  for (const snippet of snippets) {
    await expect
      .poll(
        async () => {
          try {
            return await fs.readFile(filePath, 'utf8');
          } catch {
            return '';
          }
        },
        {timeout: timeoutMs},
      )
      .toContain(snippet);
  }
}

export async function listClusterConfigs(
  request: APIRequestContext,
  clusterId: number,
): Promise<RealConfigInfo[]> {
  const response = await request.get(
    `${backendBaseURL}/api/v1/clusters/${clusterId}/configs`,
  );
  expect(response.ok()).toBeTruthy();
  const payload = (await response.json()) as ClusterConfigsResponse;
  return payload.data ?? [];
}

export async function updateClusterConfig(
  request: APIRequestContext,
  configId: number,
  content: string,
  comment?: string,
): Promise<RealConfigInfo> {
  const response = await request.put(
    `${backendBaseURL}/api/v1/configs/${configId}`,
    {
      data: {content, comment},
    },
  );
  expect(response.ok()).toBeTruthy();
  const payload = (await response.json()) as ClusterConfigResponse;
  expect(payload.error_msg ?? '').toBe('');
  expect(payload.data).toBeTruthy();
  return payload.data as RealConfigInfo;
}

export async function listClusterConfigVersions(
  request: APIRequestContext,
  configId: number,
): Promise<NonNullable<ClusterConfigVersionsResponse['data']>> {
  const response = await request.get(
    `${backendBaseURL}/api/v1/configs/${configId}/versions`,
  );
  expect(response.ok()).toBeTruthy();
  const payload = (await response.json()) as ClusterConfigVersionsResponse;
  expect(payload.error_msg ?? '').toBe('');
  return (payload.data ?? []) as NonNullable<ClusterConfigVersionsResponse['data']>;
}

export async function rollbackClusterConfig(
  request: APIRequestContext,
  configId: number,
  version: number,
  comment?: string,
): Promise<RealConfigInfo> {
  const response = await request.post(
    `${backendBaseURL}/api/v1/configs/${configId}/rollback`,
    {
      data: {version, comment},
    },
  );
  expect(response.ok()).toBeTruthy();
  const payload = (await response.json()) as ClusterConfigResponse;
  expect(payload.error_msg ?? '').toBe('');
  expect(payload.data).toBeTruthy();
  return payload.data as RealConfigInfo;
}

export async function syncClusterTemplateToAllNodesApi(
  request: APIRequestContext,
  clusterId: number,
  configType: ConfigType,
): Promise<NonNullable<ClusterConfigSyncAllResponse['data']>> {
  const response = await request.post(
    `${backendBaseURL}/api/v1/clusters/${clusterId}/configs/sync-all`,
    {
      data: {config_type: configType},
    },
  );
  expect(response.ok()).toBeTruthy();
  const payload = (await response.json()) as ClusterConfigSyncAllResponse;
  expect(payload.error_msg ?? '').toBe('');
  expect(payload.data).toBeTruthy();
  return payload.data as NonNullable<ClusterConfigSyncAllResponse['data']>;
}

export async function waitForTemplateConfig(
  request: APIRequestContext,
  clusterId: number,
  configType: ConfigType,
  predicate: (config: RealConfigInfo) => boolean,
  timeoutMs: number = 60000,
): Promise<RealConfigInfo> {
  await expect
    .poll(
      async () => {
        const configs = await listClusterConfigs(request, clusterId);
        const matched = configs.find(
          (config) =>
            config.is_template &&
            config.config_type === configType &&
            predicate(config),
        );
        return matched ?? null;
      },
      {timeout: timeoutMs},
    )
    .not.toBeNull();

  const configs = await listClusterConfigs(request, clusterId);
  return configs.find(
    (config) =>
      config.is_template &&
      config.config_type === configType &&
      predicate(config),
  ) as RealConfigInfo;
}

export async function waitForAllNodeConfigsMatchingTemplate(
  request: APIRequestContext,
  clusterId: number,
  configType: ConfigType,
  timeoutMs: number = 60000,
): Promise<RealConfigInfo[]> {
  await expect
    .poll(
      async () => {
        const configs = await listClusterConfigs(request, clusterId);
        const nodes = configs.filter(
          (config) =>
            !config.is_template && config.config_type === configType,
        );
        return nodes.length > 0 && nodes.every((config) => config.match_template);
      },
      {timeout: timeoutMs},
    )
    .toBeTruthy();

  const configs = await listClusterConfigs(request, clusterId);
  return configs.filter(
    (config) => !config.is_template && config.config_type === configType,
  );
}
