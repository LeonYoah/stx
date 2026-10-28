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

import type {APIRequestContext, Page} from '@playwright/test';
import {waitForOnlineHost} from './install-wizard-real';

const backendBaseURL =
  process.env.E2E_BACKEND_BASE_URL ?? 'http://127.0.0.1:18000';

interface ErrorResponse {
  error_msg?: string;
}

interface PackageInfoResponse extends ErrorResponse {
  data?: {
    version?: string;
    is_local?: boolean;
  };
}

interface DownloadStatusResponse extends ErrorResponse {
  data?: {
    status?: string;
    error?: string;
  };
}

interface CreateClusterResponse extends ErrorResponse {
  data?: {
    id?: number | string;
  };
}

interface AddNodeResponse extends ErrorResponse {
  data?: {
    id?: number | string;
  };
}

interface InstallationStatusResponse extends ErrorResponse {
  data?: {
    id?: string;
    status?: string;
    message?: string;
    error?: string;
    steps?: Array<{
      step?: string;
      status?: string;
      error?: string;
      message?: string;
    }>;
  };
}

interface ClusterDetailResponse extends ErrorResponse {
  data?: {
    id?: number | string;
    version?: string;
    install_dir?: string;
    status?: string;
  };
}

interface ClusterStatusResponse extends ErrorResponse {
  data?: {
    status?: string;
    nodes?: Array<{
      status?: string;
    }>;
  };
}

interface UpgradeTaskResponse extends ErrorResponse {
  data?: {
    id?: number;
    status?: string;
    current_step?: string;
    error?: string;
    rollback_status?: string;
  };
}

interface UpgradeTaskStepsResponse extends ErrorResponse {
  data?: {
    steps?: Array<{
      code?: string;
      status?: string;
      error?: string;
    }>;
  };
}

export interface InstalledClusterFixture {
  clusterId: number;
  hostId: number;
  hostName: string;
  hostIP: string;
  installDir: string;
  version: string;
  clusterPort: number;
  httpPort: number;
}

async function readJSON<T>(response: Awaited<ReturnType<APIRequestContext['get']>> | Awaited<ReturnType<APIRequestContext['post']>>): Promise<T> {
  return (await response.json()) as T;
}

async function throwResponseError(
  label: string,
  response: Awaited<ReturnType<APIRequestContext['get']>> | Awaited<ReturnType<APIRequestContext['post']>>,
): Promise<never> {
  let body = '';
  try {
    body = await response.text();
  } catch {
    // ignore
  }
  throw new Error(`${label} failed: HTTP ${response.status()}${body ? ` ${body}` : ''}`);
}

export async function installSourceCluster(
  page: Page,
  options: {
    sourceVersion: string;
    installDir: string;
    clusterPort: number;
    httpPort: number;
    /** Hybrid worker port; defaults to clusterPort+1 (install-wizard-real pattern). */
    workerPort?: number;
  },
): Promise<InstalledClusterFixture> {
  const host = await waitForOnlineHost(page);
  const request = page.context().request;
  const clusterName = `e2e-upgrade-${Date.now()}`;
  const workerPort = options.workerPort ?? options.clusterPort + 1;

  const createResponse = await request.post(`${backendBaseURL}/api/v1/clusters`, {
    data: {
      name: clusterName,
      description: 'Real E2E source cluster for upgrade flow',
      deployment_mode: 'hybrid',
      version: options.sourceVersion,
      install_dir: options.installDir,
      config: {
        runtime: {
          dynamic_slot: true,
          slot_num: 2,
          slot_allocation_strategy: 'RANDOM',
          job_schedule_strategy: 'REJECT',
          history_job_expire_minutes: 1440,
          scheduled_deletion_enable: true,
          enable_http: true,
          job_log_mode: 'per_job',
        },
        jvm: {
          hybrid_heap_size: 1,
          master_heap_size: 1,
          worker_heap_size: 1,
        },
        checkpoint: {
          storage_type: 'LOCAL_FILE',
          namespace: '/tmp/seatunnel/checkpoint/',
        },
        imap: {
          storage_type: 'DISABLED',
          namespace: '/tmp/seatunnel/imap/',
        },
        ports: {
          master_hazelcast_port: options.clusterPort,
          master_api_port: options.httpPort,
          worker_port: workerPort,
        },
      },
    },
  });
  if (!createResponse.ok()) {
    await throwResponseError('create source cluster', createResponse);
  }
  const createPayload = await readJSON<CreateClusterResponse>(createResponse);
  const clusterId = Number(createPayload.data?.id || 0);
  if (!clusterId) {
    throw new Error(`create source cluster returned no id: ${JSON.stringify(createPayload)}`);
  }

  const addNodeResponse = await request.post(
    `${backendBaseURL}/api/v1/clusters/${clusterId}/nodes`,
    {
      data: {
        host_id: host.id,
        role: 'master/worker',
        install_dir: options.installDir,
        hazelcast_port: options.clusterPort,
        api_port: options.httpPort,
        worker_port: workerPort,
      },
    },
  );
  if (!addNodeResponse.ok()) {
    await throwResponseError('add source cluster node', addNodeResponse);
  }
  const addNodePayload = await readJSON<AddNodeResponse>(addNodeResponse);
  if (!addNodePayload.data?.id) {
    throw new Error(`add source cluster node returned no id: ${JSON.stringify(addNodePayload)}`);
  }

  const installResponse = await request.post(
    `${backendBaseURL}/api/v1/hosts/${host.id}/install`,
    {
      data: {
        cluster_id: String(clusterId),
        version: options.sourceVersion,
        install_dir: options.installDir,
        install_mode: 'online',
        mirror: 'apache',
        deployment_mode: 'hybrid',
        node_role: 'master/worker',
        master_addresses: [host.ipAddress],
        cluster_port: options.clusterPort,
        worker_port: workerPort,
        http_port: options.httpPort,
        enable_http: true,
        dynamic_slot: true,
        slot_num: 2,
        slot_allocation_strategy: 'RANDOM',
        job_schedule_strategy: 'REJECT',
        history_job_expire_minutes: 1440,
        scheduled_deletion_enable: true,
        job_log_mode: 'per_job',
        jvm: {
          hybrid_heap_size: 1,
          master_heap_size: 1,
          worker_heap_size: 1,
        },
        checkpoint: {
          storage_type: 'LOCAL_FILE',
          namespace: '/tmp/seatunnel/checkpoint/',
        },
        imap: {
          storage_type: 'DISABLED',
          namespace: '/tmp/seatunnel/imap/',
        },
      },
    },
  );
  if (!installResponse.ok()) {
    await throwResponseError('start source cluster installation', installResponse);
  }

  await waitForHostInstallationSuccess(page, host.id);
  await waitForClusterReady(
    page,
    clusterId,
    options.sourceVersion,
    options.installDir,
  );

  return {
    clusterId,
    hostId: host.id,
    hostName: host.name,
    hostIP: host.ipAddress,
    installDir: options.installDir,
    version: options.sourceVersion,
    clusterPort: options.clusterPort,
    httpPort: options.httpPort,
  };
}


export async function stopSourceCluster(
  page: Page,
  clusterId: number,
  timeoutMs: number = 120000,
): Promise<void> {
  const request = page.context().request;
  const stopResponse = await request.post(
    `${backendBaseURL}/api/v1/clusters/${clusterId}/stop`,
  );
  if (!stopResponse.ok()) {
    // Best-effort: cluster may already be stopped / crashed.
    let body = '';
    try {
      body = await stopResponse.text();
    } catch {
      // ignore
    }
    console.warn(
      `[upgrade-real] stop cluster ${clusterId} returned HTTP ${stopResponse.status()}${body ? ` ${body}` : ''}`,
    );
  }

  const startedAt = Date.now();
  while (Date.now() - startedAt < timeoutMs) {
    const statusResponse = await request.get(
      `${backendBaseURL}/api/v1/clusters/${clusterId}/status`,
    );
    if (statusResponse.ok()) {
      const statusPayload = (await statusResponse.json()) as ClusterStatusResponse;
      const status = statusPayload?.data?.status;
      if (
        status === 'stopped' ||
        status === 'unknown' ||
        !(statusPayload?.data?.nodes || []).some((node) => node?.status === 'running')
      ) {
        return;
      }
    }
    await page.waitForTimeout(2000);
  }
  console.warn(
    `[upgrade-real] timed out waiting for cluster ${clusterId} to stop; continuing`,
  );
}

export async function ensureLocalPackage(
  page: Page,
  version: string,
  mirror: 'apache' | 'aliyun' | 'huaweicloud' = 'apache',
  timeoutMs: number = 900000,
): Promise<void> {
  const infoResponse = await page.context().request.get(
    `${backendBaseURL}/api/v1/packages/${version}`,
  );
  if (infoResponse.ok()) {
    const infoPayload = (await infoResponse.json()) as PackageInfoResponse;
    if (infoPayload?.data?.is_local) {
      return;
    }
  }

  const startResponse = await page.context().request.post(
    `${backendBaseURL}/api/v1/packages/download`,
    {
      data: {
        version,
        mirror,
      },
    },
  );
  if (!startResponse.ok()) {
    throw new Error(
      `Failed to start package download for ${version}: ${startResponse.status()}`,
    );
  }

  const startedAt = Date.now();
  while (Date.now() - startedAt < timeoutMs) {
    const statusResponse = await page.context().request.get(
      `${backendBaseURL}/api/v1/packages/download/${version}`,
    );
    if (!statusResponse.ok()) {
      await page.waitForTimeout(2000);
      continue;
    }

    const payload = (await statusResponse.json()) as DownloadStatusResponse;
    const status = payload?.data?.status;
    if (status === 'completed') {
      return;
    }
    if (status === 'failed' || status === 'cancelled') {
      throw new Error(
        `Package download for ${version} failed: ${payload?.data?.error || status}`,
      );
    }

    await page.waitForTimeout(2000);
  }

  throw new Error(`Timed out waiting for local package ${version}`);
}

export async function waitForHostInstallationSuccess(
  page: Page,
  hostId: number,
  timeoutMs: number = 1800000,
): Promise<void> {
  const startedAt = Date.now();

  while (Date.now() - startedAt < timeoutMs) {
    const response = await page.context().request.get(
      `${backendBaseURL}/api/v1/hosts/${hostId}/install/status`,
    );
    if (response.ok()) {
      const payload = (await response.json()) as InstallationStatusResponse;
      const status = payload?.data?.status;
      if (status === 'success') {
        return;
      }
      if (status === 'failed' || status === 'cancelled') {
        throw new Error(
          `Installation for host ${hostId} failed: ${payload?.data?.error || payload?.data?.message || status}`,
        );
      }
    }

    await page.waitForTimeout(2000);
  }

  throw new Error(`Timed out waiting for host installation on ${hostId}`);
}

export async function waitForClusterReady(
  page: Page,
  clusterId: number,
  expectedVersion: string,
  expectedInstallDir: string,
  timeoutMs: number = 300000,
): Promise<void> {
  const startedAt = Date.now();

  while (Date.now() - startedAt < timeoutMs) {
    const [detailResponse, statusResponse] = await Promise.all([
      page.context().request.get(`${backendBaseURL}/api/v1/clusters/${clusterId}`),
      page.context().request.get(`${backendBaseURL}/api/v1/clusters/${clusterId}/status`),
    ]);

    if (detailResponse.ok() && statusResponse.ok()) {
      const detailPayload = (await detailResponse.json()) as ClusterDetailResponse;
      const statusPayload = (await statusResponse.json()) as ClusterStatusResponse;
      if (
        detailPayload?.data?.version === expectedVersion &&
        detailPayload?.data?.install_dir === expectedInstallDir &&
        (statusPayload?.data?.status === 'running' ||
          (statusPayload?.data?.status === 'degraded' &&
            (statusPayload?.data?.nodes || []).some((node) => node?.status === 'running')))
      ) {
        return;
      }
    }

    await page.waitForTimeout(3000);
  }

  throw new Error(`Timed out waiting for cluster ${clusterId} to become ready`);
}

export async function waitForUpgradeTaskSuccess(
  page: Page,
  taskId: number,
  requiredStepCodes: string[],
  timeoutMs: number = 1800000,
): Promise<void> {
  const startedAt = Date.now();

  while (Date.now() - startedAt < timeoutMs) {
    const [taskResponse, stepsResponse] = await Promise.all([
      page.context().request.get(
        `${backendBaseURL}/api/v1/st-upgrade/tasks/${taskId}`,
      ),
      page.context().request.get(
        `${backendBaseURL}/api/v1/st-upgrade/tasks/${taskId}/steps`,
      ),
    ]);

    if (taskResponse.ok() && stepsResponse.ok()) {
      const taskPayload = (await taskResponse.json()) as UpgradeTaskResponse;
      const stepsPayload =
        (await stepsResponse.json()) as UpgradeTaskStepsResponse;
      const taskStatus = taskPayload?.data?.status;
      const taskError = taskPayload?.data?.error;
      const stepStatuses = new Map(
        (stepsPayload?.data?.steps || []).map((step) => [
          step.code || '',
          step.status || '',
        ]),
      );

      if (taskStatus === 'succeeded') {
        for (const stepCode of requiredStepCodes) {
          if (stepStatuses.get(stepCode) !== 'succeeded') {
            throw new Error(
              `Upgrade task ${taskId} completed without ${stepCode} succeeding`,
            );
          }
        }
        return;
      }

      if (taskStatus === 'failed' || taskStatus === 'rollback_failed') {
        throw new Error(
          `Upgrade task ${taskId} failed: ${taskError || taskStatus}`,
        );
      }
    }

    await page.waitForTimeout(3000);
  }

  throw new Error(`Timed out waiting for upgrade task ${taskId} to succeed`);
}

export async function waitForClusterVersion(
  page: Page,
  clusterId: number,
  targetVersion: string,
  targetInstallDir: string,
  timeoutMs: number = 300000,
): Promise<void> {
  const startedAt = Date.now();

  while (Date.now() - startedAt < timeoutMs) {
    const response = await page.context().request.get(
      `${backendBaseURL}/api/v1/clusters/${clusterId}`,
    );
    if (response.ok()) {
      const payload = (await response.json()) as ClusterDetailResponse;
      if (
        payload?.data?.version === targetVersion &&
        payload?.data?.install_dir === targetInstallDir
      ) {
        return;
      }
    }

    await page.waitForTimeout(2000);
  }

  throw new Error(
    `Timed out waiting for cluster ${clusterId} to switch to ${targetVersion}`,
  );
}
