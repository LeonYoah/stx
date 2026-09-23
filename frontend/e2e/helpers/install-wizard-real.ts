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
import os from 'node:os';
import {execFile} from 'node:child_process';
import {promisify} from 'node:util';
import {expect, type Page} from '@playwright/test';

const backendBaseURL =
  process.env.E2E_BACKEND_BASE_URL ?? 'http://127.0.0.1:18000';
const repoRoot = path.resolve(process.cwd(), '..');
const execFileAsync = promisify(execFile);

export interface OnlineHostFixture {
  id: number;
  name: string;
  ipAddress: string;
}

interface HostListResponse {
  data?: {
    hosts?: Array<{
      id?: number | string;
      name?: string;
      ip_address?: string;
      is_online?: boolean;
      agent_status?: string;
    }>;
  };
}

interface ClusterListResponse {
  data?: {
    clusters?: Array<{
      id?: number | string;
      name?: string;
      status?: string;
      install_dir?: string;
    }>;
  };
}

interface ClusterNodesResponse {
  data?: Array<{
    id?: number | string;
    role?: string;
    status?: string;
    is_online?: boolean;
    process_pid?: number;
    install_dir?: string;
  }>;
}

interface RuntimeStorageResponse {
  data?: {
    checkpoint?: {
      enabled?: boolean;
      endpoint?: string;
      bucket?: string;
      namespace?: string;
    };
    imap?: {
      enabled?: boolean;
      endpoint?: string;
      bucket?: string;
      namespace?: string;
    };
  };
}

interface StxJavaProxyStatusResponse {
  data?: {
    managed?: boolean;
    running?: boolean;
    healthy?: boolean;
    pid?: number;
    endpoint?: string;
  };
}

interface RuntimeStorageValidationResponse {
  data?: {
    success?: boolean;
    warning?: string;
    hosts?: Array<{
      success?: boolean;
      message?: string;
    }>;
  };
}

interface RuntimeStorageListResponse {
  data?: {
    path?: string;
    items?: Array<{
      path?: string;
      name?: string;
      directory?: boolean;
    }>;
  };
}

interface CreateClusterResponse {
  data?: {
    id?: number | string;
  };
  error_msg?: string;
}

interface AddNodeResponse {
  data?: {
    id?: number | string;
  };
  error_msg?: string;
}

export interface PreparedInstallClusterFixture {
  clusterId: number;
  hostId: number;
  hostName: string;
  installDir: string;
  version: string;
  clusterPort: number;
  httpPort: number;
}

export async function waitForOnlineHost(
  page: Page,
  timeoutMs: number = 120000,
): Promise<OnlineHostFixture> {
  const startedAt = Date.now();

  while (Date.now() - startedAt < timeoutMs) {
    const response = await page
      .context()
      .request.get(`${backendBaseURL}/api/v1/hosts`);
    if (response.ok()) {
      const payload = (await response.json()) as HostListResponse;
      const hosts = payload?.data?.hosts ?? [];
      const onlineHost = hosts.find(
        (host) => host?.is_online && host?.agent_status === 'installed',
      );
      if (onlineHost?.id) {
        return {
          id: Number(onlineHost.id),
          name: String(onlineHost.name || `Host-${onlineHost.id}`),
          ipAddress: String(onlineHost.ip_address || '127.0.0.1'),
        };
      }
    }
    await page.waitForTimeout(2000);
  }

  throw new Error('Timed out waiting for an online agent-backed host');
}

export function buildInstallWizardLabURL(options: {
  hostId: number;
  hostName: string;
  version: string;
  installDir: string;
  clusterPort: number;
  httpPort: number;
  clusterId?: number;
}): string {
  const params = new URLSearchParams({
    hostId: String(options.hostId),
    hostName: options.hostName,
    initialVersion: options.version,
    initialInstallDir: options.installDir,
    initialClusterPort: String(options.clusterPort),
    initialHttpPort: String(options.httpPort),
    ...(options.clusterId ? {clusterId: String(options.clusterId)} : {}),
  });
  return `/e2e-lab/install-wizard?${params.toString()}`;
}

export async function prepareClusterForInstallWizard(
  page: Page,
  options: {
    hostId: number;
    hostName: string;
    version: string;
    installDir: string;
    clusterPort: number;
    httpPort: number;
    deploymentMode?: 'hybrid' | 'separated';
    nodeRole?: 'master' | 'worker' | 'master/worker';
  },
): Promise<PreparedInstallClusterFixture> {
  const deploymentMode = options.deploymentMode ?? 'hybrid';
  const nodeRole = options.nodeRole ?? 'master/worker';
  const createResponse = await page
    .context()
    .request.post(`${backendBaseURL}/api/v1/clusters`, {
      data: {
        name: `e2e-installer-${Date.now()}`,
        description: 'Real E2E installer managed cluster',
        deployment_mode: deploymentMode,
        version: options.version,
        install_dir: options.installDir,
      },
    });
  if (!createResponse.ok()) {
    throw new Error(
      `create installer cluster failed: HTTP ${createResponse.status()} ${await createResponse.text()}`,
    );
  }
  const createPayload = (await createResponse.json()) as CreateClusterResponse;
  const clusterId = Number(createPayload.data?.id || 0);
  if (!clusterId) {
    throw new Error(
      `create installer cluster returned no id: ${JSON.stringify(createPayload)}`,
    );
  }

  const addNodeResponse = await page
    .context()
    .request.post(`${backendBaseURL}/api/v1/clusters/${clusterId}/nodes`, {
      data: {
        host_id: options.hostId,
        role: nodeRole,
        install_dir: options.installDir,
        hazelcast_port: options.clusterPort,
        api_port: options.httpPort,
        worker_port:
          deploymentMode === 'hybrid' ? options.clusterPort + 1 : undefined,
        skip_precheck: true,
      },
    });
  if (!addNodeResponse.ok()) {
    throw new Error(
      `add installer node failed: HTTP ${addNodeResponse.status()} ${await addNodeResponse.text()}`,
    );
  }
  const addNodePayload = (await addNodeResponse.json()) as AddNodeResponse;
  if (!addNodePayload.data?.id) {
    throw new Error(
      `add installer node returned no id: ${JSON.stringify(addNodePayload)}`,
    );
  }

  return {
    clusterId,
    hostId: options.hostId,
    hostName: options.hostName,
    installDir: options.installDir,
    version: options.version,
    clusterPort: options.clusterPort,
    httpPort: options.httpPort,
  };
}

export async function chooseSelectOption(
  page: Page,
  testId: string,
  optionName: RegExp | string,
): Promise<void> {
  await page.getByTestId(testId).click();
  await page.getByRole('option', {name: optionName}).click();
}

export async function waitForFileContent(
  filePath: string,
  timeoutMs: number = 120000,
): Promise<string> {
  const startedAt = Date.now();

  while (Date.now() - startedAt < timeoutMs) {
    try {
      return await fs.readFile(filePath, 'utf8');
    } catch {
      // Ignore until timeout.
    }
    await new Promise((resolve) => setTimeout(resolve, 1000));
  }

  throw new Error(`Timed out waiting for file: ${filePath}`);
}

function normalizeInstallDir(input: string): string {
  return input.replace(/[\/]+$/, '');
}

function matchesInstallDir(
  candidate: string | undefined,
  installDir: string,
): boolean {
  const normalizedInstallDir = normalizeInstallDir(installDir);
  const candidateNormalized = normalizeInstallDir(String(candidate || ''));
  if (!candidateNormalized) {
    return false;
  }
  const installDirBaseName = path.basename(normalizedInstallDir);
  return (
    candidateNormalized === normalizedInstallDir ||
    candidateNormalized.endsWith(normalizedInstallDir) ||
    normalizedInstallDir.endsWith(candidateNormalized) ||
    path.basename(candidateNormalized) === installDirBaseName
  );
}

async function clusterMatchesInstallDir(
  page: Page,
  clusterId: number,
  installDir: string,
): Promise<boolean> {
  const detailResponse = await page
    .context()
    .request.get(`${backendBaseURL}/api/v1/clusters/${clusterId}`);
  if (detailResponse.ok()) {
    const detailPayload = (await detailResponse.json()) as {
      data?: {install_dir?: string};
    };
    if (matchesInstallDir(detailPayload?.data?.install_dir, installDir)) {
      return true;
    }
  }

  const nodesResponse = await page
    .context()
    .request.get(`${backendBaseURL}/api/v1/clusters/${clusterId}/nodes`);
  if (!nodesResponse.ok()) {
    return false;
  }
  const nodesPayload = (await nodesResponse.json()) as ClusterNodesResponse;
  return (nodesPayload?.data ?? []).some((node) =>
    matchesInstallDir(node?.install_dir, installDir),
  );
}

export async function expectInstallationSuccess(
  page: Page,
  hostId: number,
  previousInstallationId: string | null = null,
): Promise<void> {
  const timeoutMs = 900000;
  const startedAt = Date.now();
  let completed = false;

  // 先检查真实任务终态，业务失败时立即输出服务端错误，避免只等待完成页面直到超时。
  // Check the real terminal status first so business failures surface immediately instead of timing out on the completion page.
  while (Date.now() - startedAt < timeoutMs) {
    const response = await page
      .context()
      .request.get(`${backendBaseURL}/api/v1/hosts/${hostId}/install/status`);
    if (response.ok()) {
      const payload = (await response.json()) as {
        data?: {
          id?: string;
          status?: string;
          current_step?: string;
          error?: string;
          message?: string;
        };
      };
      // 同一主机只保留一份安装状态；新请求写入前可能暂时读到上一次成功任务。
      // A host exposes one installation status, so a new request can briefly return the previous successful task before replacement.
      if (
        previousInstallationId &&
        payload.data?.id === previousInstallationId
      ) {
        await page.waitForTimeout(1000);
        continue;
      }
      const status = payload.data?.status;
      if (status === 'failed') {
        throw new Error(
          `installation failed at ${payload.data?.current_step || 'unknown'}: ${payload.data?.error || payload.data?.message || 'unknown error'}`,
        );
      }
      if (status === 'success') {
        completed = true;
        break;
      }
    }
    await page.waitForTimeout(1000);
  }

  if (!completed) {
    throw new Error(`installation did not finish within ${timeoutMs}ms`);
  }

  await expect(page.getByTestId('install-wizard-step-complete')).toBeVisible({
    timeout: 30000,
  });
  await expect(page.getByTestId('install-complete-result')).toContainText(
    /安装成功|Installation Success/i,
    {timeout: 900000},
  );
}

// 读取当前安装任务编号，用于区分同一主机上的前后两次安装。
// Read the current installation ID so consecutive installs on the same host can be distinguished.
export async function getCurrentInstallationId(
  page: Page,
  hostId: number,
): Promise<string | null> {
  const response = await page
    .context()
    .request.get(`${backendBaseURL}/api/v1/hosts/${hostId}/install/status`);
  if (!response.ok()) {
    return null;
  }
  const payload = (await response.json()) as {data?: {id?: string}};
  return payload.data?.id || null;
}

export async function waitForClusterByInstallDir(
  page: Page,
  installDir: string,
  timeoutMs: number = 180000,
): Promise<{id: number; status: string; name: string}> {
  const startedAt = Date.now();

  while (Date.now() - startedAt < timeoutMs) {
    const candidates: Array<{id: number; status: string; name: string}> = [];
    for (let current = 1; current <= 10; current += 1) {
      const response = await page
        .context()
        .request.get(`${backendBaseURL}/api/v1/clusters`, {
          params: {
            current: String(current),
            size: '200',
          },
        });
      if (!response.ok()) {
        continue;
      }
      const payload = (await response.json()) as ClusterListResponse;
      for (const item of payload?.data?.clusters ?? []) {
        if (!item?.id) {
          continue;
        }
        if (matchesInstallDir(item?.install_dir, installDir)) {
          return {
            id: Number(item.id),
            status: String(item.status || 'unknown'),
            name: String(item.name || `Cluster-${item.id}`),
          };
        }
        candidates.push({
          id: Number(item.id),
          status: String(item.status || 'unknown'),
          name: String(item.name || `Cluster-${item.id}`),
        });
      }
    }

    for (const cluster of candidates.slice(0, 20)) {
      if (await clusterMatchesInstallDir(page, cluster.id, installDir)) {
        return cluster;
      }
    }

    await page.waitForTimeout(2000);
  }

  throw new Error(
    `Timed out waiting for cluster with install_dir=${installDir}`,
  );
}

export async function ensureClusterRunning(
  page: Page,
  clusterId: number,
  timeoutMs: number = 180000,
): Promise<void> {
  const startedAt = Date.now();

  while (Date.now() - startedAt < timeoutMs) {
    const response = await page
      .context()
      .request.get(`${backendBaseURL}/api/v1/clusters/${clusterId}`);
    if (response.ok()) {
      const payload = (await response.json()) as {
        data?: {status?: string};
      };
      const status = payload?.data?.status;
      if (status === 'running') {
        return;
      }
      if (
        status === 'installed' ||
        status === 'stopped' ||
        status === 'unknown'
      ) {
        await page
          .context()
          .request.post(`${backendBaseURL}/api/v1/clusters/${clusterId}/start`);
      }
    }
    await page.waitForTimeout(3000);
  }

  throw new Error(
    `Timed out waiting for cluster ${clusterId} to become running`,
  );
}

export async function waitForRuntimeStorageReady(
  page: Page,
  clusterId: number,
  timeoutMs: number = 180000,
): Promise<NonNullable<RuntimeStorageResponse['data']>> {
  const startedAt = Date.now();

  while (Date.now() - startedAt < timeoutMs) {
    const response = await page
      .context()
      .request.get(
        `${backendBaseURL}/api/v1/clusters/${clusterId}/runtime-storage`,
      );
    if (response.ok()) {
      const payload = (await response.json()) as RuntimeStorageResponse;
      if (
        payload?.data?.checkpoint?.namespace ||
        payload?.data?.imap?.namespace
      ) {
        return payload.data!;
      }
    }
    await page.waitForTimeout(3000);
  }

  throw new Error(`Timed out waiting for cluster ${clusterId} runtime storage`);
}

async function waitForClusterMasterNodeReady(
  page: Page,
  clusterId: number,
  timeoutMs: number = 180000,
): Promise<void> {
  const startedAt = Date.now();

  while (Date.now() - startedAt < timeoutMs) {
    const response = await page
      .context()
      .request.get(`${backendBaseURL}/api/v1/clusters/${clusterId}/nodes`);
    if (response.ok()) {
      const payload = (await response.json()) as ClusterNodesResponse;
      const readyNode = (payload?.data ?? []).find((node) => {
        const role = String(node?.role || '').toLowerCase();
        return (
          (role === 'master' ||
            role === 'master/worker' ||
            role === 'master_worker') &&
          Boolean(node?.is_online)
        );
      });
      if (readyNode) {
        return;
      }
    }
    await page.waitForTimeout(3000);
  }

  throw new Error(
    `Timed out waiting for cluster ${clusterId} master node to become online`,
  );
}

export async function waitForStxJavaProxyHealthy(
  page: Page,
  clusterId: number,
  timeoutMs: number = 300000,
): Promise<NonNullable<StxJavaProxyStatusResponse['data']>> {
  const startedAt = Date.now();
  let lastError = '';
  let lastStatus = '';
  let lastStartAt = 0;

  await waitForClusterMasterNodeReady(page, clusterId, timeoutMs);

  while (Date.now() - startedAt < timeoutMs) {
    const response = await page
      .context()
      .request.get(
        `${backendBaseURL}/api/v1/clusters/${clusterId}/stx-java-proxy/status`,
      );
    if (response.ok()) {
      const payload = (await response.json()) as StxJavaProxyStatusResponse;
      lastStatus = JSON.stringify(payload?.data || {});
      if (
        payload?.data?.managed &&
        payload?.data?.running &&
        payload?.data?.healthy
      ) {
        return payload.data!;
      }
      if (payload?.data?.endpoint) {
        const healthz = await page
          .context()
          .request.get(`${payload.data.endpoint.replace(/\/$/, '')}/healthz`);
        if (healthz.ok()) {
          return {
            ...payload.data!,
            healthy: true,
            running: true,
            managed: payload.data?.managed ?? true,
          };
        }
      }
    } else {
      try {
        lastError = await response.text();
      } catch {
        lastError = `HTTP ${response.status()}`;
      }
    }

    if (Date.now() - lastStartAt >= 10000) {
      lastStartAt = Date.now();
      const startResponse = await page
        .context()
        .request.post(
          `${backendBaseURL}/api/v1/clusters/${clusterId}/stx-java-proxy/start`,
        );
      if (!startResponse.ok()) {
        try {
          lastError = await startResponse.text();
        } catch {
          lastError = `HTTP ${startResponse.status()}`;
        }
      }
    }
    await page.waitForTimeout(3000);
  }

  let logPreview = '';
  try {
    const logsResponse = await page
      .context()
      .request.get(
        `${backendBaseURL}/api/v1/clusters/${clusterId}/stx-java-proxy/logs`,
        {params: {lines: '80'}},
      );
    if (logsResponse.ok()) {
      logPreview = await logsResponse.text();
    }
  } catch {
    // ignore log fetch errors on timeout path
  }

  throw new Error(
    `Timed out waiting for cluster ${clusterId} stx-java-proxy; lastStatus=${lastStatus}; lastError=${lastError}; logs=${logPreview}`,
  );
}

export async function validateClusterRuntimeStorage(
  page: Page,
  clusterId: number,
  kind: 'checkpoint' | 'imap',
): Promise<NonNullable<RuntimeStorageValidationResponse['data']>> {
  const response = await page
    .context()
    .request.post(
      `${backendBaseURL}/api/v1/clusters/${clusterId}/runtime-storage/${kind}/validate`,
    );
  expect(response.ok()).toBeTruthy();
  const payload = (await response.json()) as RuntimeStorageValidationResponse;
  expect(payload?.data?.success).toBeTruthy();
  expect(
    (payload?.data?.hosts ?? []).every((host) => host.success),
    JSON.stringify(payload?.data),
  ).toBeTruthy();
  return payload.data!;
}

export async function listClusterRuntimeStorage(
  page: Page,
  clusterId: number,
  kind: 'checkpoint' | 'imap',
  path: string,
): Promise<NonNullable<RuntimeStorageListResponse['data']>> {
  const response = await page
    .context()
    .request.post(
      `${backendBaseURL}/api/v1/clusters/${clusterId}/runtime-storage/${kind}/list`,
      {
        data: {
          path,
        },
      },
    );
  expect(response.ok()).toBeTruthy();
  const payload = (await response.json()) as RuntimeStorageListResponse;
  expect(payload?.data?.path).toBeTruthy();
  expect(Array.isArray(payload?.data?.items ?? [])).toBeTruthy();
  return payload.data!;
}

async function resolveStxJavaProxyAssets(version: string) {
  const script = path.join(repoRoot, 'scripts', 'stx-java-proxy.sh');
  const candidates = [
    path.join(repoRoot, 'lib', 'stx-java-proxy-v2.jar'),
    path.join(
      repoRoot,
      'tools',
      'stx-java-proxy',
      'target',
      'stx-java-proxy-v2.jar',
    ),
    path.join(repoRoot, 'lib', `stx-java-proxy-${version}.jar`),
    path.join(repoRoot, 'lib', 'stx-java-proxy.jar'),
    path.join(
      repoRoot,
      'tools',
      'stx-java-proxy',
      'target',
      `stx-java-proxy-${version}-2.12.15.jar`,
    ),
  ];

  await fs.access(script);
  for (const candidate of candidates) {
    try {
      await fs.access(candidate);
      return {
        script,
        jar: candidate,
        home: repoRoot,
      };
    } catch {
      // try next candidate
    }
  }

  throw new Error(
    `stx-java-proxy jar not found, checked: ${candidates.join(', ')}`,
  );
}

export async function expectStxJavaProxyProbeSuccess(options: {
  installDir: string;
  version: string;
  kind: 'checkpoint' | 'imap';
  request: Record<string, unknown>;
}) {
  const {script, jar, home} = await resolveStxJavaProxyAssets(options.version);

  const tempDir = await fs.mkdtemp(
    path.join(os.tmpdir(), 'stx-java-proxy-e2e-'),
  );
  const requestFile = path.join(tempDir, `${options.kind}.request.json`);
  const responseFile = path.join(tempDir, `${options.kind}.response.json`);
  await fs.writeFile(
    requestFile,
    JSON.stringify(options.request, null, 2),
    'utf8',
  );

  try {
    await execFileAsync(
      'bash',
      [
        script,
        'probe-once',
        options.kind,
        '--request-file',
        requestFile,
        '--response-file',
        responseFile,
      ],
      {
        env: {
          ...process.env,
          SEATUNNEL_HOME: options.installDir,
          STX_JAVA_PROXY_HOME: home,
          STX_JAVA_PROXY_JAR: jar,
          STX_JAVA_PROXY_VERSION: options.version,
        },
      },
    );
    const payload = JSON.parse(await fs.readFile(responseFile, 'utf8')) as {
      ok?: boolean;
      writable?: boolean;
      readable?: boolean;
      message?: string;
    };
    expect(payload?.ok, JSON.stringify(payload)).toBeTruthy();
    expect(payload?.writable, JSON.stringify(payload)).toBeTruthy();
    expect(payload?.readable, JSON.stringify(payload)).toBeTruthy();
    return payload;
  } finally {
    await fs.rm(tempDir, {recursive: true, force: true});
  }
}

export async function assertFileContains(
  filePath: string,
  snippets: string[],
): Promise<void> {
  const content = await waitForFileContent(filePath);
  for (const snippet of snippets) {
    expect(content).toContain(snippet);
  }
}

export function resolveInstalledConfigPaths(installDir: string) {
  const configDir = path.join(installDir, 'config');
  return {
    seatunnel: path.join(configDir, 'seatunnel.yaml'),
    hazelcast: path.join(configDir, 'hazelcast.yaml'),
    hazelcastClient: path.join(configDir, 'hazelcast-client.yaml'),
    log4j2: path.join(configDir, 'log4j2.properties'),
  };
}
