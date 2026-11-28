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
import {spawn} from 'node:child_process';
import {expect, type APIRequestContext, type Page} from '@playwright/test';
import {ConfigType} from '@/lib/services/config';
import {
  ensureClusterRunning,
  waitForStxJavaProxyHealthy,
} from './install-wizard-real';
import {syncClusterTemplateToAllNodesApi} from './config-real';
import {
  buildStreamingFakeTaskContent,
  cancelSyncJob,
  createSyncTask,
  getJobCheckpointSnapshot,
  publishSyncTask,
  runSyncTask,
  waitForJobStatus,
} from './workbench-real';

const backendBaseURL =
  process.env.E2E_BACKEND_BASE_URL ?? 'http://127.0.0.1:18000';
const repoRoot = path.resolve(process.cwd(), '..');
const stxBinary =
  process.env.E2E_STX_CLI_BIN ?? path.join(repoRoot, 'dist', 'stx');

export interface HdfsHaFixture {
  nameServices: string;
  haNamenodes: string;
  rpc1: string;
  rpc2: string;
  failoverProvider: string;
  siteXmlPath: string;
}

export interface MinioFixture {
  endpoint: string;
  accessKey: string;
  secretKey: string;
  checkpointBucket: string;
  imapBucket: string;
}

export interface RuntimeStorageSpecPayload {
  kind?: string;
  enabled?: boolean;
  storage_type?: string;
  namespace?: string;
  endpoint?: string;
  bucket?: string;
  external?: boolean;
  hdfs_ha_enabled?: boolean;
  hdfs_name_services?: string;
  hdfs_ha_namenodes?: string;
  hdfs_namenode_rpc_address_1?: string;
  hdfs_namenode_rpc_address_2?: string;
  hdfs_failover_proxy_provider?: string;
  hdfs_site_path?: string;
  disable_cache?: boolean;
  s3_credentials_provider?: string;
  size_available?: boolean;
  total_size_bytes?: number;
  cleanup_supported?: boolean;
  warning?: string;
}

export interface RuntimeStorageDetailsPayload {
  cluster_id?: number;
  config_source?: string;
  checkpoint?: RuntimeStorageSpecPayload;
  imap?: RuntimeStorageSpecPayload;
  configured_checkpoint?: RuntimeStorageSpecPayload;
  configured_imap?: RuntimeStorageSpecPayload;
}

export interface RuntimeStorageListItemPayload {
  path?: string;
  name?: string;
  directory?: boolean;
  size_bytes?: number;
  modified_at?: string;
}

export interface RuntimeStoragePreviewPayload {
  path?: string;
  file_name?: string;
  size_bytes?: number;
  binary?: boolean;
  encoding?: string;
  text_preview?: string;
  hex_preview?: string;
}

export interface RuntimeStorageImapInspectPayload {
  path?: string;
  file_name?: string;
  size_bytes?: number;
  entry_count?: number;
  entries?: Array<Record<string, unknown>>;
}

export interface CLIResultEnvelope<T = unknown> {
  api_version: string;
  operation_id: string;
  request_id: string;
  data: T;
  result_meta?: {
    complete?: boolean;
    next_command?: string;
  };
}

export interface CLIEventEnvelope {
  event?: string;
  code?: string;
  message?: string;
  operation_id?: string;
}

export function readHdfsHaFixture(): HdfsHaFixture {
  const nameServices = process.env.E2E_HDFS_HA_NAMESERVICE ?? 'stx-ha';
  const haNamenodes = process.env.E2E_HDFS_HA_NAMENODES ?? 'nn-alpha,nn-beta';
  const rpc1 = process.env.E2E_HDFS_HA_RPC_1 ?? '127.0.0.1:18020';
  const rpc2 = process.env.E2E_HDFS_HA_RPC_2 ?? '127.0.0.1:28020';
  const failoverProvider =
    process.env.E2E_HDFS_HA_FAILOVER_PROVIDER ??
    'org.apache.hadoop.hdfs.server.namenode.ha.ConfiguredFailoverProxyProvider';
  const siteXmlPath = process.env.E2E_HDFS_HA_SITE_XML ?? '';
  expect(
    siteXmlPath,
    'E2E_HDFS_HA_SITE_XML must be exported by run-real-runtime-storage.sh',
  ).toBeTruthy();
  return {
    nameServices,
    haNamenodes,
    rpc1,
    rpc2,
    failoverProvider,
    siteXmlPath,
  };
}

export function readMinioFixture(): MinioFixture {
  return {
    endpoint:
      process.env.E2E_INSTALLER_REAL_MINIO_ENDPOINT ?? 'http://127.0.0.1:19000',
    accessKey: process.env.E2E_INSTALLER_REAL_MINIO_ACCESS_KEY ?? 'minioadmin',
    secretKey: process.env.E2E_INSTALLER_REAL_MINIO_SECRET_KEY ?? 'minioadmin',
    checkpointBucket:
      process.env.E2E_INSTALLER_REAL_CHECKPOINT_BUCKET ??
      's3a://seatunnel-checkpoint',
    imapBucket:
      process.env.E2E_INSTALLER_REAL_IMAP_BUCKET ?? 's3a://seatunnel-imap',
  };
}

// 在集群详情页打开「运行时」页签并切换到指定子视图（Checkpoint / IMAP）。
// Open the cluster detail Runtime tab and switch to the requested sub-view (Checkpoint / IMAP).
export async function openClusterRuntimeStorageView(
  page: Page,
  clusterId: number,
  kind: 'checkpoint' | 'imap',
): Promise<void> {
  await page.goto(`/clusters/${clusterId}?tab=runtime&view=${kind}`);
  const panel = page.getByTestId('cluster-runtime-panel');
  await expect(panel).toBeVisible({timeout: 120000});

  // Arc SegmentedControl 使用带 data-value 与 aria-pressed 的 button 渲染选项。
  // Arc SegmentedControl renders options as buttons with data-value and aria-pressed.
  const targetSegment = panel.locator(`button[data-value="${kind}"]`).first();
  if ((await targetSegment.count()) > 0 && (await targetSegment.isVisible())) {
    const pressed = await targetSegment.getAttribute('aria-pressed');
    if (pressed !== 'true') {
      await targetSegment.click();
    }
  }
  await expect(page.getByTestId(`runtime-storage-${kind}-editor`)).toBeVisible({
    timeout: 60000,
  });
}

// 仅替换 YAML 中的 plugin-config 缩进块，保留后续 http/telemetry 等配置段不被截断。
// Replace only the indented plugin-config block in YAML so subsequent sections (http/telemetry) stay intact.
export function replaceYamlPluginConfigBlock(
  yamlContent: string,
  entries: string[],
): string {
  const lines = yamlContent.split('\n');
  const headerIndex = lines.findIndex((line) =>
    /^\s*plugin-config:\s*$/.test(line),
  );
  if (headerIndex < 0) {
    throw new Error('plugin-config block not found in seatunnel.yaml');
  }
  const baseIndent = lines[headerIndex].match(/^(\s*)/)?.[1].length ?? 0;
  let endIndex = headerIndex + 1;
  while (endIndex < lines.length) {
    const line = lines[endIndex];
    const trimmed = line.trim();
    if (trimmed === '' || trimmed.startsWith('#')) {
      endIndex += 1;
      continue;
    }
    const indent = line.match(/^(\s*)/)?.[1].length ?? 0;
    if (indent <= baseIndent) {
      break;
    }
    endIndex += 1;
  }
  const childIndent = ' '.repeat(baseIndent + 2);
  const replacementLines = [
    lines[headerIndex],
    ...entries.map((entry) => `${childIndent}${entry}`),
  ];
  return [
    ...lines.slice(0, headerIndex),
    ...replacementLines,
    ...lines.slice(endIndex),
  ].join('\n');
}

// 统一操作 Arc Checkbox / Radix Checkbox，按期望状态决定是否点击。
// Toggle an Arc or Radix Checkbox only when its checked state differs from desired.
export async function setCheckboxChecked(
  page: Page,
  testId: string,
  desired: boolean,
): Promise<void> {
  const checkbox = page.getByTestId(testId);
  await expect(checkbox).toBeVisible({timeout: 30000});
  const current =
    (await checkbox.getAttribute('aria-checked')) === 'true' ||
    (await checkbox.getAttribute('data-state')) === 'checked';
  if (current !== desired) {
    await checkbox.click();
  }
}

export async function getRuntimeStorageDetailsApi(
  request: APIRequestContext,
  clusterId: number,
): Promise<RuntimeStorageDetailsPayload> {
  const response = await request.get(
    `${backendBaseURL}/api/v1/clusters/${clusterId}/runtime-storage`,
  );
  expect(response.ok(), await response.text()).toBeTruthy();
  const body = (await response.json()) as {
    error_msg?: string;
    data?: RuntimeStorageDetailsPayload;
  };
  expect(body.error_msg ?? '').toBe('');
  expect(body.data).toBeTruthy();
  return body.data!;
}

export async function listRuntimeStorageApi(
  request: APIRequestContext,
  clusterId: number,
  kind: 'checkpoint' | 'imap',
  storagePath: string,
  recursive = true,
  limit = 500,
): Promise<{path: string; items: RuntimeStorageListItemPayload[]}> {
  const response = await request.post(
    `${backendBaseURL}/api/v1/clusters/${clusterId}/runtime-storage/${kind}/list`,
    {
      data: {
        path: storagePath,
        recursive,
        limit,
      },
    },
  );
  const rawText = await response.text();
  expect(response.ok(), `list ${kind} ${storagePath} failed: ${rawText}`).toBeTruthy();
  const body = JSON.parse(rawText) as {
    error_msg?: string;
    data?: {path?: string; items?: RuntimeStorageListItemPayload[]};
  };
  expect(body.error_msg ?? '').toBe('');
  return {
    path: body.data?.path ?? storagePath,
    items: body.data?.items ?? [],
  };
}

export async function waitForRuntimeStorageMatchingFile(
  request: APIRequestContext,
  clusterId: number,
  kind: 'checkpoint' | 'imap',
  rootPath: string,
  predicate: (item: RuntimeStorageListItemPayload) => boolean,
  timeoutMs = 120000,
): Promise<RuntimeStorageListItemPayload> {
  let matched: RuntimeStorageListItemPayload | null = null;
  let lastError = '';
  await expect
    .poll(
      async () => {
        try {
          const response = await request.post(
            `${backendBaseURL}/api/v1/clusters/${clusterId}/runtime-storage/${kind}/list`,
            {
              data: {
                path: rootPath,
                recursive: true,
                limit: 500,
              },
            },
          );
          if (!response.ok()) {
            lastError = await response.text();
            return false;
          }
          const body = (await response.json()) as {
            error_msg?: string;
            data?: {items?: RuntimeStorageListItemPayload[]};
          };
          if (body.error_msg) {
            lastError = body.error_msg;
            return false;
          }
          const found = (body.data?.items ?? []).find(
            (item) => !item.directory && predicate(item),
          );
          if (found) {
            matched = found;
            return true;
          }
          return false;
        } catch (error) {
          lastError = String(error);
          return false;
        }
      },
      {
        message: `waiting for ${kind} file under ${rootPath} (lastError=${lastError})`,
        timeout: timeoutMs,
        intervals: [2000, 3000, 5000],
      },
    )
    .toBeTruthy();
  return matched!;
}

export async function previewRuntimeStorageApi(
  request: APIRequestContext,
  clusterId: number,
  kind: 'checkpoint' | 'imap',
  filePath: string,
): Promise<RuntimeStoragePreviewPayload> {
  const response = await request.post(
    `${backendBaseURL}/api/v1/clusters/${clusterId}/runtime-storage/${kind}/preview`,
    {
      data: {
        path: filePath,
        max_bytes: 65536,
      },
    },
  );
  const rawText = await response.text();
  expect(response.ok(), `preview ${kind} ${filePath} failed: ${rawText}`).toBeTruthy();
  const body = JSON.parse(rawText) as {
    error_msg?: string;
    data?: RuntimeStoragePreviewPayload;
  };
  expect(body.error_msg ?? '').toBe('');
  expect(body.data).toBeTruthy();
  return body.data!;
}

export async function inspectImapWalApi(
  request: APIRequestContext,
  clusterId: number,
  filePath: string,
): Promise<RuntimeStorageImapInspectPayload> {
  const response = await request.post(
    `${backendBaseURL}/api/v1/clusters/${clusterId}/runtime-storage/imap/inspect`,
    {
      data: {path: filePath},
    },
  );
  const rawText = await response.text();
  expect(response.ok(), `imap inspect ${filePath} failed: ${rawText}`).toBeTruthy();
  const body = JSON.parse(rawText) as {
    error_msg?: string;
    data?: RuntimeStorageImapInspectPayload;
  };
  expect(body.error_msg ?? '').toBe('');
  expect(body.data).toBeTruthy();
  return body.data!;
}

// 匹配 SeaTunnel 2.3.x 的 wal.txt 以及历史 *_wal.txt 文件名。
// Match both SeaTunnel 2.3.x wal.txt and legacy *_wal.txt IMAP WAL filenames.
export function isImapWalFileName(name?: string): boolean {
  const normalized = String(name || '')
    .trim()
    .toLowerCase();
  return normalized === 'wal.txt' || normalized.endsWith('_wal.txt');
}

// 等待 SeaTunnel 引擎 REST v2（/overview）真正返回 200，避免进程刚拉起尚未监听 HTTP 即提交作业。
// Wait until SeaTunnel REST v2 (/overview) returns HTTP 200 so job submission never races engine HTTP boot.
export async function waitForEngineRestReady(
  request: APIRequestContext,
  hostIP: string,
  httpPort: number,
  installDir: string,
  timeoutMs = 150000,
): Promise<void> {
  const urls = Array.from(
    new Set([
      `http://${hostIP}:${httpPort}/overview`,
      `http://127.0.0.1:${httpPort}/overview`,
    ]),
  );
  let lastError = 'no probe executed';
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    for (const url of urls) {
      try {
        const resp = await request.get(url, {timeout: 5000});
        if (resp.ok()) {
          return;
        }
        lastError = `${url} -> HTTP ${resp.status()} ${(await resp.text()).slice(0, 200)}`;
      } catch (error) {
        lastError = `${url} -> ${String(error)}`;
      }
    }
    await new Promise((resolve) => setTimeout(resolve, 2000));
  }

  // 超时时附带 seatunnel-engine-server.log 尾部与关键配置，避免盲猜。
  // Attach seatunnel-engine-server.log tail and key YAML configs on timeout for instant triage.
  const readTail = async (filePath: string, maxChars = 2500): Promise<string> => {
    try {
      const content = await fs.readFile(filePath, 'utf8');
      return content.length > maxChars
        ? `...(truncated)\n${content.slice(-maxChars)}`
        : content;
    } catch (error) {
      return `<unreadable: ${String(error)}>`;
    }
  };
  const serverLog = await readTail(
    path.join(installDir, 'logs', 'seatunnel-engine-server.log'),
  );
  const seatunnelYaml = await readTail(
    path.join(installDir, 'config', 'seatunnel.yaml'),
    2000,
  );
  throw new Error(
    `Timed out waiting for SeaTunnel REST v2 (${urls.join(', ')}): lastError=${lastError}\n--- seatunnel.yaml ---\n${seatunnelYaml}\n--- seatunnel-engine-server.log ---\n${serverLog}`,
  );
}

// 将已保存的配置模板下发到节点并重启集群，等待引擎 REST v2 与 stx-java-proxy 重新健康就绪。
// Push saved config templates to nodes, restart the cluster, and wait for SeaTunnel REST v2 + stx-java-proxy readiness.
export async function syncTemplatesAndRestartCluster(
  page: Page,
  clusterId: number,
  configTypes: ConfigType[],
  engineTarget?: {hostIP: string; httpPort: number; installDir: string},
): Promise<void> {
  const request = page.context().request;
  for (const configType of configTypes) {
    const syncResult = await syncClusterTemplateToAllNodesApi(
      request,
      clusterId,
      configType,
    );
    expect(
      syncResult.push_errors ?? [],
      `sync ${configType} push errors`,
    ).toHaveLength(0);
  }

  const restartResp = await request.post(
    `${backendBaseURL}/api/v1/clusters/${clusterId}/restart`,
  );
  expect(
    restartResp.ok(),
    `restart cluster ${clusterId} failed: ${await restartResp.text()}`,
  ).toBeTruthy();

  await ensureClusterRunning(page, clusterId, 240000);
  await waitForStxJavaProxyHealthy(page, clusterId, 240000);
  if (engineTarget) {
    await waitForEngineRestReady(
      request,
      engineTarget.hostIP,
      engineTarget.httpPort,
      engineTarget.installDir,
    );
  }
}

// 运行一个短流式 FakeSource 作业，等待真实 Checkpoint 完成后触发 Savepoint 停止。
// Run a short FakeSource streaming job, wait for a COMPLETED checkpoint, then stop via savepoint.
export async function runStreamingJobForCheckpoint(
  request: APIRequestContext,
  clusterId: number,
  engineVersion: string,
  namePrefix: string,
): Promise<{
  taskId: number;
  jobId: number;
  platformJobId: string;
  content: string;
}> {
  const content = buildStreamingFakeTaskContent();
  const task = await createSyncTask(request, {
    clusterId,
    engineVersion,
    name: `${namePrefix}-${Date.now()}`,
    content,
  });
  await publishSyncTask(request, task.taskId);
  const submitted = await runSyncTask(request, task.taskId);
  expect(submitted.id).toBeGreaterThan(0);

  await expect
    .poll(
      async () => {
        const snapshot = await getJobCheckpointSnapshot(request, submitted.id);
        const hasCompletedHistory = (snapshot.history ?? []).some((item) =>
          String(item.checkpoint?.status || '')
            .toUpperCase()
            .includes('COMPLETED'),
        );
        const hasCompletedOverview = Boolean(
          snapshot.overview?.pipelines?.[0]?.latestCompleted?.checkpointId,
        );
        return hasCompletedHistory || hasCompletedOverview;
      },
      {
        message: `waiting for COMPLETED checkpoint on streaming job ${submitted.id}`,
        timeout: 180000,
        intervals: [2000, 3000, 5000],
      },
    )
    .toBeTruthy();

  await cancelSyncJob(request, submitted.id, true);
  const terminalStatuses = new Set([
    'canceled',
    'cancelled',
    'finished',
    'success',
    'stopped',
  ]);
  const stopped = await waitForJobStatus(
    request,
    submitted.id,
    (job) => terminalStatuses.has(String(job.status || '').toLowerCase()),
    180000,
  );
  expect(stopped.platform_job_id).toBeTruthy();
  return {
    taskId: task.taskId,
    jobId: submitted.id,
    platformJobId: String(stopped.platform_job_id),
    content,
  };
}

// 直接执行打包后的 dist/stx，校验 stdout 单 JSON 与 stderr NDJSON 契约。
// Execute the packaged dist/stx binary and validate single-JSON stdout + NDJSON stderr.
export async function runStxCli<T = unknown>(
  cliConfigHome: string,
  args: string[],
  options: {stdin?: string; timeoutMs?: number; expectFailure?: boolean} = {},
): Promise<{
  code: number;
  result: CLIResultEnvelope<T> | null;
  events: CLIEventEnvelope[];
  stdout: string;
  stderr: string;
}> {
  const timeoutMs = options.timeoutMs ?? 120000;
  return new Promise((resolve, reject) => {
    const child = spawn(stxBinary, args, {
      cwd: repoRoot,
      env: {
        ...process.env,
        XDG_CONFIG_HOME: cliConfigHome,
        NO_COLOR: '1',
      },
      stdio: ['pipe', 'pipe', 'pipe'],
    });
    let stdout = '';
    let stderr = '';
    const timer = setTimeout(() => child.kill('SIGKILL'), timeoutMs);

    child.stdout.setEncoding('utf8');
    child.stderr.setEncoding('utf8');
    child.stdout.on('data', (chunk: string) => {
      stdout += chunk;
    });
    child.stderr.on('data', (chunk: string) => {
      stderr += chunk;
    });
    child.on('error', (error) => {
      clearTimeout(timer);
      reject(error);
    });
    child.on('close', (code, signal) => {
      clearTimeout(timer);
      const exitCode = code ?? -1;
      if (!options.expectFailure && exitCode !== 0) {
        reject(
          new Error(
            `stx ${args.join(' ')} failed: code=${exitCode} signal=${signal || '-'} stdout=${stdout.trim()} stderr=${stderr.trim()}`,
          ),
        );
        return;
      }
      try {
        const events = stderr
          .split('\n')
          .map((line) => line.trim())
          .filter(Boolean)
          .map((line) => JSON.parse(line) as CLIEventEnvelope);
        const trimmedStdout = stdout.trim();
        const result = trimmedStdout
          ? (JSON.parse(trimmedStdout) as CLIResultEnvelope<T>)
          : null;
        resolve({code: exitCode, result, events, stdout, stderr});
      } catch (error) {
        reject(
          new Error(
            `stx ${args.join(' ')} returned non-JSON output: ${String(error)} stdout=${stdout.trim()} stderr=${stderr.trim()}`,
          ),
        );
      }
    });

    child.stdin.end(options.stdin ?? '');
  });
}
