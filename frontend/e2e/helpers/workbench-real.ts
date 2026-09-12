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

import {execFile, spawn} from 'node:child_process';
import fs from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import {promisify} from 'node:util';
import {expect, type APIRequestContext, type Page} from '@playwright/test';

import {
  ensureClusterRunning,
  waitForRuntimeStorageReady,
  waitForStxJavaProxyHealthy,
} from './install-wizard-real';
import {
  downloadPluginApi,
  installPluginToClusterApi,
  waitForInstalledPlugin,
  waitForPluginDownloadCompleted,
} from './plugin-real';
import {
  installSourceCluster,
  type InstalledClusterFixture,
} from './upgrade-real';

const execFileAsync = promisify(execFile);
const backendBaseURL =
  process.env.E2E_BACKEND_BASE_URL ?? 'http://127.0.0.1:18000';
const seatunnelVersion = process.env.E2E_INSTALLER_REAL_VERSION ?? '2.3.13';
const mysqlContainerName =
  process.env.E2E_WORKBENCH_REAL_MYSQL_NAME ?? 'stx-workbench-real-mysql';
const mysqlPort = Number(process.env.E2E_WORKBENCH_REAL_MYSQL_PORT ?? '3307');
const mysqlImage = process.env.E2E_WORKBENCH_REAL_MYSQL_IMAGE ?? 'mysql:8.0.39';
const mysqlPassword =
  process.env.E2E_WORKBENCH_REAL_MYSQL_PASSWORD ?? 'seatunnel';

interface ErrorResponse {
  error_msg?: string;
}

interface TaskResponse extends ErrorResponse {
  data?: {
    id: number;
    name: string;
    definition?: Record<string, unknown>;
  };
}

interface TaskTreeResponse extends ErrorResponse {
  data?: {
    items?: Array<Record<string, unknown>>;
  };
}

interface TaskVersionResponse extends ErrorResponse {
  data?: {
    version: number;
  };
}

interface ValidateResponse extends ErrorResponse {
  data?: {
    valid?: boolean;
    errors?: string[];
    warnings?: string[];
  };
}

interface SyncTaskDefinition extends Record<string, unknown> {
  preview_mode?: string;
  preview_output_format?: string;
  preview_row_limit?: number;
  preview_timeout_minutes?: number;
  preview_http_sink?: {
    url: string;
    array_mode?: boolean;
  };
}

interface DAGResponse extends ErrorResponse {
  data?: {
    nodes?: Array<Record<string, unknown>>;
    edges?: Array<Record<string, unknown>>;
    webui_job?: {
      jobDag?: {
        vertexInfoMap?: Record<string, Record<string, unknown>>;
      };
    };
  };
}

interface JobResponse extends ErrorResponse {
  data?: {
    id: number;
    task_id: number;
    platform_job_id?: string;
    engine_job_id?: string;
    run_type?: string;
    status?: string;
    started_at?: string;
    finished_at?: string;
    submit_spec?: Record<string, unknown>;
    result_preview?: Record<string, unknown>;
  };
}

interface JobListResponse extends ErrorResponse {
  data?: {
    items?: Array<NonNullable<JobResponse['data']>>;
  };
}

interface JobLogsResponse extends ErrorResponse {
  data?: {
    logs?: string;
    empty_reason?: string;
  };
}

interface PreviewSnapshotResponse extends ErrorResponse {
  data?: {
    tables?: Array<{
      table_path?: string;
      row_count?: number;
      total?: number;
      rows?: Array<Record<string, unknown>>;
    }>;
    selected_table?: {
      table_path?: string;
      row_count?: number;
      rows?: Array<Record<string, unknown>>;
    };
    total_rows?: number;
    rows?: Array<Record<string, unknown>>;
    empty_reason?: string;
    status?: string;
  };
}

interface CheckpointSnapshotResponse extends ErrorResponse {
  data?: {
    overview?: {
      pipelines?: Array<{
        pipelineId?: number;
        latestCompleted?: {
          checkpointId?: number;
          status?: string;
        };
      }>;
    };
    history?: Array<{
      pipelineId?: number;
      checkpoint?: {
        checkpointId?: number;
        status?: string;
      };
    }>;
  };
}

interface RuntimeStorageListResponse extends ErrorResponse {
  data?: {
    path?: string;
    items?: Array<{
      name?: string;
      path?: string;
      directory?: boolean;
    }>;
  };
}

interface RuntimeStorageDetailsResponse extends ErrorResponse {
  data?: {
    checkpoint?: {
      enabled?: boolean;
      storage_type?: string;
      namespace?: string;
    };
  };
}

interface RuntimeStorageInspectResponse extends ErrorResponse {
  data?: {
    file_name?: string;
    pipeline_state?: Record<string, unknown>;
    completed_checkpoint?: Record<string, unknown>;
    action_states?: Array<Record<string, unknown>>;
    task_statistics?: Array<Record<string, unknown>>;
  };
}

export interface SyncTaskFixture {
  folderId: number;
  taskId: number;
  name: string;
}

async function readJSON<T>(
  response:
    | Awaited<ReturnType<APIRequestContext['get']>>
    | Awaited<ReturnType<APIRequestContext['post']>>
    | Awaited<ReturnType<APIRequestContext['put']>>
    | Awaited<ReturnType<APIRequestContext['delete']>>,
): Promise<T> {
  return (await response.json()) as T;
}

async function assertOK(
  response:
    | Awaited<ReturnType<APIRequestContext['get']>>
    | Awaited<ReturnType<APIRequestContext['post']>>
    | Awaited<ReturnType<APIRequestContext['put']>>
    | Awaited<ReturnType<APIRequestContext['delete']>>,
  label: string,
): Promise<void> {
  if (!response.ok()) {
    let body = '';
    try {
      body = await response.text();
    } catch {
      // ignore
    }
    throw new Error(
      `${label} failed: HTTP ${response.status()}${body ? ` ${body}` : ''}`,
    );
  }
}

function sqlString(value: string): string {
  return `'${value.replace(/'/g, "''")}'`;
}

async function docker(...args: string[]): Promise<string> {
  const result = await execFileAsync('docker', args);
  return result.stdout.trim();
}

export async function ensureWorkbenchRealMySQLFixture(): Promise<void> {
  await docker('rm', '-f', mysqlContainerName).catch(() => '');
  await docker(
    'run',
    '-d',
    '--name',
    mysqlContainerName,
    '-e',
    `MYSQL_ROOT_PASSWORD=${mysqlPassword}`,
    '-p',
    `127.0.0.1:${mysqlPort}:3306`,
    mysqlImage,
    '--default-authentication-plugin=mysql_native_password',
  );

  for (let i = 0; i < 60; i += 1) {
    try {
      await docker(
        'exec',
        mysqlContainerName,
        'mysqladmin',
        '--protocol=TCP',
        '-h127.0.0.1',
        '-uroot',
        `-p${mysqlPassword}`,
        'ping',
        '--silent',
      );
      return;
    } catch {
      await new Promise((resolve) => setTimeout(resolve, 2000));
    }
  }
  throw new Error('Timed out waiting for workbench real MySQL fixture');
}

export async function cleanupWorkbenchRealMySQLFixture(): Promise<void> {
  await docker('rm', '-f', mysqlContainerName).catch(() => '');
}

export async function seedWorkbenchRealMySQLFixture(): Promise<void> {
  const statements: string[] = [
    'CREATE DATABASE IF NOT EXISTS seatunnel_demo',
    'CREATE DATABASE IF NOT EXISTS demo2',
    'DROP TABLE IF EXISTS seatunnel_demo.users',
    'CREATE TABLE seatunnel_demo.users (id INT PRIMARY KEY, name VARCHAR(32))',
    `INSERT INTO seatunnel_demo.users (id, name) VALUES (1, ${sqlString('alice')}), (2, ${sqlString('bob')})`,
    'DROP TABLE IF EXISTS seatunnel_demo.orders',
    'CREATE TABLE seatunnel_demo.orders (id INT PRIMARY KEY, amount INT)',
    'INSERT INTO seatunnel_demo.orders (id, amount) VALUES (1, 8), (2, 10)',
  ];

  for (let index = 1; index <= 100; index += 1) {
    const suffix = String(index).padStart(3, '0');
    statements.push(`DROP TABLE IF EXISTS seatunnel_demo.bulk_${suffix}`);
    statements.push(
      `CREATE TABLE seatunnel_demo.bulk_${suffix} (id INT PRIMARY KEY, name VARCHAR(32), updated_at TIMESTAMP NULL DEFAULT CURRENT_TIMESTAMP)`,
    );
    statements.push(
      `INSERT INTO seatunnel_demo.bulk_${suffix} (id, name) VALUES (${index}, ${sqlString(`bulk_${suffix}`)})`,
    );
  }

  const sqlFile = path.join(
    os.tmpdir(),
    `workbench-real-mysql-${Date.now()}.sql`,
  );
  const sqlContent = `${statements.join(';\n')};\n`;
  const containerSqlPath = `/tmp/${path.basename(sqlFile)}`;
  await fs.writeFile(sqlFile, sqlContent, 'utf8');
  try {
    await docker('cp', sqlFile, `${mysqlContainerName}:${containerSqlPath}`);
    await runDockerExecSQLFile(containerSqlPath);
  } finally {
    await fs.rm(sqlFile, {force: true}).catch(() => undefined);
    await docker(
      'exec',
      mysqlContainerName,
      'rm',
      '-f',
      containerSqlPath,
    ).catch(() => '');
  }
}

async function runDockerExecSQLFile(containerSqlPath: string): Promise<void> {
  await new Promise<void>((resolve, reject) => {
    const child = spawn(
      'docker',
      [
        'exec',
        '-i',
        mysqlContainerName,
        'mysql',
        '--protocol=TCP',
        '-h127.0.0.1',
        '-uroot',
        `-p${mysqlPassword}`,
      ],
      {
        stdio: ['pipe', 'pipe', 'pipe'],
      },
    );

    let stderr = '';
    child.stderr.setEncoding('utf8');
    child.stderr.on('data', (chunk) => {
      stderr += chunk;
    });
    child.on('error', reject);
    child.on('close', (code) => {
      if (code === 0) {
        resolve();
        return;
      }
      reject(
        new Error(
          `docker exec mysql import failed with code ${code}: ${stderr.trim()}`,
        ),
      );
    });

    const cat = spawn(
      'docker',
      ['exec', mysqlContainerName, 'cat', containerSqlPath],
      {
        stdio: ['ignore', 'pipe', 'pipe'],
      },
    );
    let catStderr = '';
    cat.stderr.setEncoding('utf8');
    cat.stderr.on('data', (chunk) => {
      catStderr += chunk;
    });
    cat.on('error', reject);
    cat.on('close', (code) => {
      if (code !== 0) {
        child.kill('SIGTERM');
        reject(
          new Error(
            `docker exec cat sql failed with code ${code}: ${catStderr.trim()}`,
          ),
        );
      }
    });
    cat.stdout.pipe(child.stdin);
  });
}

export async function prepareWorkbenchRealCluster(
  page: Page,
): Promise<InstalledClusterFixture> {
  const installDir = `${process.env.E2E_INSTALLER_REAL_INSTALL_DIR ?? '/tmp/e2e/workbench-real/seatunnel'}-workbench-${Date.now()}`;
  const clusterPort = Number(
    process.env.E2E_INSTALLER_REAL_CLUSTER_PORT_PRIMARY ?? '38181',
  );
  const httpPort = Number(
    process.env.E2E_INSTALLER_REAL_HTTP_PORT_PRIMARY ?? '38080',
  );
  const cluster = await installSourceCluster(page, {
    sourceVersion: seatunnelVersion,
    installDir,
    clusterPort,
    httpPort,
  });
  await ensureClusterRunning(page, cluster.clusterId);
  await waitForRuntimeStorageReady(page, cluster.clusterId);
  await waitForStxJavaProxyHealthy(page, cluster.clusterId);
  await downloadPluginApi(page.context().request, 'jdbc', seatunnelVersion, [
    'mysql',
  ]);
  await waitForPluginDownloadCompleted(
    page.context().request,
    'jdbc',
    seatunnelVersion,
    ['mysql'],
    900000,
  );
  await installPluginToClusterApi(
    page.context().request,
    cluster.clusterId,
    'jdbc',
    seatunnelVersion,
    ['mysql'],
  );
  await waitForInstalledPlugin(
    page.context().request,
    cluster.clusterId,
    'jdbc',
    seatunnelVersion,
    (plugin) => (plugin.selected_profile_keys || []).includes('mysql'),
  );
  await downloadPluginApi(
    page.context().request,
    'http-base',
    seatunnelVersion,
  );
  await waitForPluginDownloadCompleted(
    page.context().request,
    'http-base',
    seatunnelVersion,
    undefined,
    900000,
  );
  await installPluginToClusterApi(
    page.context().request,
    cluster.clusterId,
    'http-base',
    seatunnelVersion,
  );
  await waitForInstalledPlugin(
    page.context().request,
    cluster.clusterId,
    'http-base',
    seatunnelVersion,
  );
  return cluster;
}

export async function createSyncFolder(
  request: APIRequestContext,
  name: string,
  parentId?: number,
): Promise<number> {
  const response = await request.post(`${backendBaseURL}/api/v1/sync/tasks`, {
    data: {
      parent_id: parentId,
      node_type: 'folder',
      name,
    },
  });
  await assertOK(response, 'create sync folder');
  const payload = await readJSON<TaskResponse>(response);
  expect(payload.error_msg ?? '').toBe('');
  expect(payload.data?.id).toBeTruthy();
  return Number(payload.data?.id);
}

function buildWorkbenchTaskDefinition(
  definition?: Record<string, unknown>,
): SyncTaskDefinition {
  const next: SyncTaskDefinition = {...(definition || {})};
  next.preview_mode =
    typeof next.preview_mode === 'string' ? next.preview_mode : 'source';
  next.preview_output_format =
    typeof next.preview_output_format === 'string'
      ? next.preview_output_format
      : 'hocon';
  next.preview_row_limit =
    typeof next.preview_row_limit === 'number' && next.preview_row_limit > 0
      ? next.preview_row_limit
      : 100;
  next.preview_timeout_minutes =
    typeof next.preview_timeout_minutes === 'number' &&
    next.preview_timeout_minutes > 0
      ? next.preview_timeout_minutes
      : 10;
  const existingSink =
    next.preview_http_sink && typeof next.preview_http_sink === 'object'
      ? next.preview_http_sink
      : undefined;
  next.preview_http_sink = {
    url:
      typeof existingSink?.url === 'string' && existingSink.url.trim()
        ? existingSink.url
        : 'http://127.0.0.1:18000/api/v1/sync/preview/collect',
    array_mode: existingSink?.array_mode === true,
  };
  return next;
}

export async function createSyncTask(
  request: APIRequestContext,
  options: {
    parentId?: number;
    clusterId: number;
    engineVersion: string;
    name: string;
    content: string;
    definition?: Record<string, unknown>;
    contentFormat?: string;
    jobName?: string;
  },
): Promise<SyncTaskFixture> {
  const folderId =
    options.parentId ??
    (await createSyncFolder(request, `workbench-real-${Date.now()}`));
  const response = await request.post(`${backendBaseURL}/api/v1/sync/tasks`, {
    data: {
      parent_id: folderId,
      node_type: 'file',
      name: options.name,
      cluster_id: options.clusterId,
      engine_version: options.engineVersion,
      content_format: options.contentFormat ?? 'hocon',
      content: options.content,
      job_name: options.jobName ?? options.name,
      definition: buildWorkbenchTaskDefinition(options.definition),
    },
  });
  await assertOK(response, 'create sync task');
  const payload = await readJSON<TaskResponse>(response);
  expect(payload.error_msg ?? '').toBe('');
  expect(payload.data?.id).toBeTruthy();
  return {folderId, taskId: Number(payload.data?.id), name: options.name};
}

export async function updateSyncTask(
  request: APIRequestContext,
  taskId: number,
  payload: Record<string, unknown>,
): Promise<void> {
  const response = await request.put(
    `${backendBaseURL}/api/v1/sync/tasks/${taskId}`,
    {data: payload},
  );
  await assertOK(response, 'update sync task');
  const body = await readJSON<TaskResponse>(response);
  expect(body.error_msg ?? '').toBe('');
}

export async function publishSyncTask(
  request: APIRequestContext,
  taskId: number,
  comment = 'real ci publish',
): Promise<number> {
  const response = await request.post(
    `${backendBaseURL}/api/v1/sync/tasks/${taskId}/publish`,
    {data: {comment}},
  );
  await assertOK(response, 'publish sync task');
  const body = await readJSON<TaskVersionResponse>(response);
  expect(body.error_msg ?? '').toBe('');
  expect(body.data?.version).toBeTruthy();
  return Number(body.data?.version);
}

export async function listSyncTree(
  request: APIRequestContext,
): Promise<Array<Record<string, unknown>>> {
  const response = await request.get(`${backendBaseURL}/api/v1/sync/tree`);
  await assertOK(response, 'list sync tree');
  const body = await readJSON<TaskTreeResponse>(response);
  expect(body.error_msg ?? '').toBe('');
  return body.data?.items ?? [];
}

export async function validateSyncTask(
  request: APIRequestContext,
  taskId: number,
): Promise<NonNullable<ValidateResponse['data']>> {
  const response = await request.post(
    `${backendBaseURL}/api/v1/sync/tasks/${taskId}/validate`,
  );
  await assertOK(response, 'validate sync task');
  const body = await readJSON<ValidateResponse>(response);
  expect(body.error_msg ?? '').toBe('');
  expect(body.data).toBeTruthy();
  return body.data as NonNullable<ValidateResponse['data']>;
}

export async function testSyncConnections(
  request: APIRequestContext,
  taskId: number,
): Promise<NonNullable<ValidateResponse['data']>> {
  const response = await request.post(
    `${backendBaseURL}/api/v1/sync/tasks/${taskId}/test-connections`,
  );
  await assertOK(response, 'test sync connections');
  const body = await readJSON<ValidateResponse>(response);
  expect(body.error_msg ?? '').toBe('');
  expect(body.data).toBeTruthy();
  return body.data as NonNullable<ValidateResponse['data']>;
}

export async function getSyncDAG(
  request: APIRequestContext,
  taskId: number,
): Promise<NonNullable<DAGResponse['data']>> {
  const response = await request.post(
    `${backendBaseURL}/api/v1/sync/tasks/${taskId}/dag`,
  );
  await assertOK(response, 'get sync dag');
  const body = await readJSON<DAGResponse>(response);
  expect(body.error_msg ?? '').toBe('');
  expect(body.data).toBeTruthy();
  return body.data as NonNullable<DAGResponse['data']>;
}

export async function previewSyncTask(
  request: APIRequestContext,
  taskId: number,
  rowLimit = 20,
): Promise<NonNullable<JobResponse['data']>> {
  const response = await request.post(
    `${backendBaseURL}/api/v1/sync/tasks/${taskId}/preview`,
    {
      data: {row_limit: rowLimit, timeout_minutes: 5},
    },
  );
  await assertOK(response, 'preview sync task');
  const body = await readJSON<JobResponse>(response);
  expect(body.error_msg ?? '').toBe('');
  expect(body.data?.id).toBeTruthy();
  return body.data as NonNullable<JobResponse['data']>;
}

export async function runSyncTask(
  request: APIRequestContext,
  taskId: number,
): Promise<NonNullable<JobResponse['data']>> {
  const response = await request.post(
    `${backendBaseURL}/api/v1/sync/tasks/${taskId}/submit`,
  );
  await assertOK(response, 'submit sync task');
  const body = await readJSON<JobResponse>(response);
  expect(body.error_msg ?? '').toBe('');
  expect(body.data?.id).toBeTruthy();
  return body.data as NonNullable<JobResponse['data']>;
}

export async function recoverSyncJob(
  request: APIRequestContext,
  jobId: number,
): Promise<NonNullable<JobResponse['data']>> {
  const response = await request.post(
    `${backendBaseURL}/api/v1/sync/jobs/${jobId}/recover`,
  );
  await assertOK(response, 'recover sync job');
  const body = await readJSON<JobResponse>(response);
  expect(body.error_msg ?? '').toBe('');
  expect(body.data?.id).toBeTruthy();
  return body.data as NonNullable<JobResponse['data']>;
}

export async function cancelSyncJob(
  request: APIRequestContext,
  jobId: number,
  stopWithSavepoint: boolean,
): Promise<NonNullable<JobResponse['data']>> {
  const response = await request.post(
    `${backendBaseURL}/api/v1/sync/jobs/${jobId}/cancel`,
    {
      data: {stop_with_savepoint: stopWithSavepoint},
    },
  );
  await assertOK(response, 'cancel sync job');
  const body = await readJSON<JobResponse>(response);
  expect(body.error_msg ?? '').toBe('');
  expect(body.data?.id).toBeTruthy();
  return body.data as NonNullable<JobResponse['data']>;
}

export async function getSyncJobLogs(
  request: APIRequestContext,
  jobId: number,
): Promise<NonNullable<JobLogsResponse['data']>> {
  const response = await request.get(
    `${backendBaseURL}/api/v1/sync/jobs/${jobId}/logs`,
    {
      params: {limit_bytes: '65536'},
    },
  );
  await assertOK(response, 'get sync job logs');
  const body = await readJSON<JobLogsResponse>(response);
  expect(body.error_msg ?? '').toBe('');
  expect(body.data).toBeTruthy();
  return body.data as NonNullable<JobLogsResponse['data']>;
}

export async function getSyncJob(
  request: APIRequestContext,
  jobId: number,
): Promise<NonNullable<JobResponse['data']>> {
  const response = await request.get(
    `${backendBaseURL}/api/v1/sync/jobs/${jobId}`,
  );
  await assertOK(response, 'get sync job');
  const body = await readJSON<JobResponse>(response);
  expect(body.error_msg ?? '').toBe('');
  expect(body.data?.id).toBeTruthy();
  return body.data as NonNullable<JobResponse['data']>;
}

export async function listSyncJobs(
  request: APIRequestContext,
  taskId: number,
): Promise<Array<NonNullable<JobResponse['data']>>> {
  const response = await request.get(`${backendBaseURL}/api/v1/sync/jobs`, {
    params: {task_id: String(taskId), current: '1', size: '100'},
  });
  await assertOK(response, 'list sync jobs');
  const body = await readJSON<JobListResponse>(response);
  expect(body.error_msg ?? '').toBe('');
  return body.data?.items ?? [];
}

export async function waitForJobStatus(
  request: APIRequestContext,
  jobId: number,
  predicate: (job: NonNullable<JobResponse['data']>) => boolean,
  timeoutMs = 180000,
): Promise<NonNullable<JobResponse['data']>> {
  let latest: NonNullable<JobResponse['data']> | null = null;
  await expect
    .poll(
      async () => {
        latest = await getSyncJob(request, jobId);
        return latest ? predicate(latest) : false;
      },
      {timeout: timeoutMs, intervals: [1000, 2000, 3000]},
    )
    .toBeTruthy();
  return latest as unknown as NonNullable<JobResponse['data']>;
}

export async function waitForScheduledJob(
  request: APIRequestContext,
  taskId: number,
  timeoutMs = 180000,
): Promise<NonNullable<JobResponse['data']>> {
  let latest: NonNullable<JobResponse['data']> | null = null;
  await expect
    .poll(
      async () => {
        const jobs = await listSyncJobs(request, taskId);
        latest = jobs.find((job) => job.run_type === 'schedule') ?? null;
        return Boolean(latest);
      },
      {timeout: timeoutMs, intervals: [1000, 2000, 3000]},
    )
    .toBeTruthy();
  return latest as unknown as NonNullable<JobResponse['data']>;
}

export async function waitForPreviewRows(
  request: APIRequestContext,
  jobId: number,
  tablePath?: string,
  timeoutMs = 180000,
): Promise<NonNullable<PreviewSnapshotResponse['data']>> {
  let latest: NonNullable<PreviewSnapshotResponse['data']> | null = null;
  await expect
    .poll(
      async () => {
        const response = await request.get(
          `${backendBaseURL}/api/v1/sync/jobs/${jobId}/preview`,
          {
            params: tablePath ? {table_path: tablePath} : undefined,
          },
        );
        await assertOK(response, 'get preview snapshot');
        const body = await readJSON<PreviewSnapshotResponse>(response);
        expect(body.error_msg ?? '').toBe('');
        latest = body.data as NonNullable<PreviewSnapshotResponse['data']>;
        const tables = latest?.tables ?? [];
        const selectedRows = latest?.selected_table?.rows?.length ?? 0;
        const totalRows = latest?.total_rows ?? 0;
        return (
          totalRows > 0 ||
          selectedRows > 0 ||
          tables.some(
            (item) =>
              (item.row_count ?? item.total ?? item.rows?.length ?? 0) > 0,
          ) ||
          (latest?.rows?.length ?? 0) > 0
        );
      },
      {timeout: timeoutMs, intervals: [1000, 2000, 3000]},
    )
    .toBeTruthy();
  return latest as unknown as NonNullable<PreviewSnapshotResponse['data']>;
}

export async function waitForPreviewCleanup(
  request: APIRequestContext,
  jobId: number,
  timeoutMs = 180000,
): Promise<void> {
  await expect
    .poll(
      async () => {
        const response = await request.get(
          `${backendBaseURL}/api/v1/sync/jobs/${jobId}/preview`,
        );
        await assertOK(response, 'get preview snapshot cleanup');
        const body = await readJSON<PreviewSnapshotResponse>(response);
        expect(body.error_msg ?? '').toBe('');
        const data = body.data;
        const tables = data?.tables ?? [];
        if ((data?.rows?.length ?? 0) > 0) {
          return false;
        }
        if ((data?.selected_table?.rows?.length ?? 0) > 0) {
          return false;
        }
        if ((data?.total_rows ?? 0) > 0) {
          return false;
        }
        if (
          tables.some(
            (item) =>
              (item.row_count ?? item.total ?? item.rows?.length ?? 0) > 0,
          )
        ) {
          return false;
        }
        return Boolean(data?.empty_reason);
      },
      {timeout: timeoutMs, intervals: [2000, 5000, 10000]},
    )
    .toBeTruthy();
}

export async function getJobCheckpointSnapshot(
  request: APIRequestContext,
  jobId: number,
): Promise<NonNullable<CheckpointSnapshotResponse['data']>> {
  const response = await request.get(
    `${backendBaseURL}/api/v1/sync/jobs/${jobId}/checkpoint`,
    {
      params: {limit: '20'},
    },
  );
  await assertOK(response, 'get job checkpoint snapshot');
  const body = await readJSON<CheckpointSnapshotResponse>(response);
  expect(body.error_msg ?? '').toBe('');
  expect(body.data).toBeTruthy();
  return body.data as NonNullable<CheckpointSnapshotResponse['data']>;
}

export async function listCheckpointFiles(
  request: APIRequestContext,
  clusterId: number,
  jobPlatformId: string,
): Promise<Array<{name?: string; path?: string}>> {
  const detailsResponse = await request.get(
    `${backendBaseURL}/api/v1/clusters/${clusterId}/runtime-storage`,
  );
  await assertOK(detailsResponse, 'get runtime storage details');
  const detailsBody =
    await readJSON<RuntimeStorageDetailsResponse>(detailsResponse);
  expect(detailsBody.error_msg ?? '').toBe('');
  const namespace = detailsBody.data?.checkpoint?.namespace?.trim();
  expect(namespace, 'checkpoint namespace should be available').toBeTruthy();

  const normalizedNamespace = namespace!.replace(/\/$/, '');
  const jobDirectory = `${normalizedNamespace}/${jobPlatformId}`;
  const candidatePaths = [jobDirectory, namespace!];
  let lastError = '';

  for (const candidatePath of candidatePaths) {
    const response = await request.post(
      `${backendBaseURL}/api/v1/clusters/${clusterId}/runtime-storage/checkpoint/list`,
      {
        data: {
          path: candidatePath,
          recursive: true,
          limit: 500,
        },
      },
    );
    const body = await readJSON<RuntimeStorageListResponse>(response);
    if (!response.ok()) {
      lastError = body.error_msg || `HTTP ${response.status()}`;
      continue;
    }
    expect(body.error_msg ?? '').toBe('');
    const items = (body.data?.items ?? []).filter((item) => {
      if (item.directory) {
        return false;
      }
      if (candidatePath !== namespace) {
        return true;
      }
      return (item.path || '').startsWith(`${jobDirectory}/`);
    });
    if (items.length > 0 || candidatePath !== namespace) {
      return items;
    }
  }

  throw new Error(
    `list checkpoint files failed: ${lastError || 'no checkpoint files found'}`,
  );
}

export async function inspectCheckpointFile(
  request: APIRequestContext,
  clusterId: number,
  path: string,
  content: string,
): Promise<NonNullable<RuntimeStorageInspectResponse['data']>> {
  const response = await request.post(
    `${backendBaseURL}/api/v1/clusters/${clusterId}/runtime-storage/checkpoint/inspect`,
    {
      data: {
        path,
        job_config: {
          content,
          content_format: 'hocon',
        },
      },
    },
  );
  await assertOK(response, 'inspect checkpoint file');
  const body = await readJSON<RuntimeStorageInspectResponse>(response);
  expect(body.error_msg ?? '').toBe('');
  expect(body.data).toBeTruthy();
  return body.data as NonNullable<RuntimeStorageInspectResponse['data']>;
}

export function buildMysqlUsersTaskContent(): string {
  return `env {\n  job.mode = "batch"\n}\n\nsource {\n  Jdbc {\n    plugin_output = "users_src"\n    url = "jdbc:mysql://127.0.0.1:${mysqlPort}/seatunnel_demo"\n    username = "root"\n    password = "${mysqlPassword}"\n    driver = "com.mysql.cj.jdbc.Driver"\n    table_list = [\n      { table_path = "seatunnel_demo.users" }\n    ]\n  }\n}\n\nsink {\n  Jdbc {\n    plugin_input = ["users_src"]\n    url = "jdbc:mysql://127.0.0.1:${mysqlPort}/demo2"\n    username = "root"\n    password = "${mysqlPassword}"\n    driver = "com.mysql.cj.jdbc.Driver"\n    database = "demo2"\n    table = "archive_users"\n    generate_sink_sql = true\n  }\n}`;
}

export function buildMysqlHundredTablesTaskContent(): string {
  const tableList = Array.from({length: 100}, (_, index) => {
    const suffix = String(index + 1).padStart(3, '0');
    return `      { table_path = "seatunnel_demo.bulk_${suffix}" }`;
  }).join('\n');
  return `env {\n  job.mode = "batch"\n}\n\nsource {\n  Jdbc {\n    plugin_output = "bulk_src"\n    url = "jdbc:mysql://127.0.0.1:${mysqlPort}/seatunnel_demo"\n    username = "root"\n    password = "${mysqlPassword}"\n    driver = "com.mysql.cj.jdbc.Driver"\n    table_list = [\n${tableList}\n    ]\n  }\n}\n\nsink {\n  Jdbc {\n    plugin_input = ["bulk_src"]\n    url = "jdbc:mysql://127.0.0.1:${mysqlPort}/demo2"\n    username = "root"\n    password = "${mysqlPassword}"\n    driver = "com.mysql.cj.jdbc.Driver"\n    database = "demo2"\n    table = "archive_${'${table_name}'}"\n    generate_sink_sql = true\n  }\n}`;
}

export function buildMysqlMultiSourceMultiSinkContent(): string {
  return `env {\n  job.mode = "batch"\n}\n\nsource {\n  Jdbc {\n    plugin_output = "users_src"\n    url = "jdbc:mysql://127.0.0.1:${mysqlPort}/seatunnel_demo"\n    username = "root"\n    password = "${mysqlPassword}"\n    driver = "com.mysql.cj.jdbc.Driver"\n    table_list = [\n      { table_path = "seatunnel_demo.users" }\n    ]\n  }\n\n  Jdbc {\n    plugin_output = "orders_src"\n    url = "jdbc:mysql://127.0.0.1:${mysqlPort}/seatunnel_demo"\n    username = "root"\n    password = "${mysqlPassword}"\n    driver = "com.mysql.cj.jdbc.Driver"\n    table_list = [\n      { table_path = "seatunnel_demo.orders" }\n    ]\n  }\n}\n\ntransform {\n  Copy {\n    plugin_input = "users_src"\n    plugin_output = "users_stage"\n    fields {\n      copied_name = name\n    }\n  }\n  Copy {\n    plugin_input = "orders_src"\n    plugin_output = "orders_stage"\n    fields {\n      copied_id = id\n    }\n  }\n}\n\nsink {\n  Jdbc {\n    plugin_input = ["users_stage"]\n    url = "jdbc:mysql://127.0.0.1:${mysqlPort}/demo2"\n    username = "root"\n    password = "${mysqlPassword}"\n    driver = "com.mysql.cj.jdbc.Driver"\n    database = "demo2"\n    table = "archive_users_multi"\n    generate_sink_sql = true\n  }\n\n  Jdbc {\n    plugin_input = ["orders_stage"]\n    url = "jdbc:mysql://127.0.0.1:${mysqlPort}/demo2"\n    username = "root"\n    password = "${mysqlPassword}"\n    driver = "com.mysql.cj.jdbc.Driver"\n    database = "demo2"\n    table = "archive_orders_multi"\n    generate_sink_sql = true\n  }\n}`;
}

export function buildMysqlMultiTransformContent(): string {
  return `env {\n  job.mode = "batch"\n}\n\nsource {\n  Jdbc {\n    plugin_output = "users_src"\n    url = "jdbc:mysql://127.0.0.1:${mysqlPort}/seatunnel_demo"\n    username = "root"\n    password = "${mysqlPassword}"\n    driver = "com.mysql.cj.jdbc.Driver"\n    table_list = [\n      { table_path = "seatunnel_demo.users" }\n    ]\n  }\n}\n\ntransform {\n  Copy {\n    plugin_input = "users_src"\n    plugin_output = "stage_1"\n    fields {\n      copied_name = name\n    }\n  }\n\n  Copy {\n    plugin_input = "stage_1"\n    plugin_output = "stage_2"\n    fields {\n      copied_name_2 = copied_name\n    }\n  }\n}\n\nsink {\n  Jdbc {\n    plugin_input = ["stage_2"]\n    url = "jdbc:mysql://127.0.0.1:${mysqlPort}/demo2"\n    username = "root"\n    password = "${mysqlPassword}"\n    driver = "com.mysql.cj.jdbc.Driver"\n    database = "demo2"\n    table = "archive_users_transform"\n    generate_sink_sql = true\n  }\n}`;
}

export function buildStreamingFakeTaskContent(): string {
  return `env {\n  parallelism = 2\n  job.mode = "STREAMING"\n  checkpoint.interval = 2000\n}\n\nsource {\n  FakeSource {\n    parallelism = 2\n    plugin_output = "fake"\n    row.num = 20000\n    schema = {\n      fields {\n        name = "string"\n        age = "int"\n      }\n    }\n  }\n}\n\nsink {\n  Console {\n  }\n}`;
}

export function buildScheduleDefinition(
  expr: string,
  timezone = 'Asia/Shanghai',
): Record<string, unknown> {
  return {
    schedule: {
      enabled: true,
      cron_expr: expr,
      timezone,
    },
  };
}

export function nextMinuteCronExpression(timezone = 'Asia/Shanghai'): string {
  const now = new Date();
  const formatter = new Intl.DateTimeFormat('en-US', {
    timeZone: timezone,
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  });
  const parts = formatter.formatToParts(new Date(now.getTime() + 60_000));
  const minute = parts.find((part) => part.type === 'minute')?.value ?? '0';
  const hour = parts.find((part) => part.type === 'hour')?.value ?? '0';
  return `${Number(minute)} ${Number(hour)} * * *`;
}
