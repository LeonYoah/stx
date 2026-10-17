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
import {execFileSync} from 'node:child_process';

import {
  ensureClusterRunning,
  waitForRuntimeStorageReady,
  waitForStxJavaProxyHealthy,
} from './install-wizard-real';
import {
  createSyncTask,
  publishSyncTask,
  type SyncTaskFixture,
} from './workbench-real';
import {installSourceCluster} from './upgrade-real';

const backendBaseURL =
  process.env.E2E_BACKEND_BASE_URL ?? 'http://127.0.0.1:18000';
const seatunnelVersion = process.env.E2E_INSTALLER_REAL_VERSION ?? '2.3.13';
const dsBaseURL =
  process.env.E2E_DS_BASE_URL ?? 'http://127.0.0.1:12345/dolphinscheduler';
const dsContainer =
  process.env.E2E_DS_CONTAINER_NAME ?? 'stx-ds-e2e-scheduler';
const envName = 'stx-cli-prod';

export type SchedulerCluster = {
  clusterId: number;
  httpPort: number;
};

export type DSBootstrap = {
  token: string;
  projectCode: number;
  environmentCode: number;
};

export function buildFakeConsoleBatchContent(): string {
  // Multi-line schema: single-line `fields { a = "x" b = "y" }` trips HOCON
  // ("Expecting close brace } or a comma, got '='") on SeaTunnel submit.
  return `
env {
  parallelism = 1
  job.mode = "BATCH"
}
source {
  FakeSource {
    plugin_output = "fake"
    row.num = 20
    schema = {
      fields {
        name = "string"
        age = "int"
      }
    }
  }
}
sink {
  Console {
    plugin_input = "fake"
  }
}
`.trim();
}

export function buildFakeSqlFailContent(): string {
  return `
env {
  parallelism = 1
  job.mode = "BATCH"
}
source {
  FakeSource {
    plugin_output = "fake"
    row.num = 10
    split.num = 2
    split.read-interval = 1000
    schema = {
      fields {
        name = "string"
        age = "int"
      }
    }
  }
}
transform {
  Sql {
    plugin_input = "fake"
    plugin_output = "bad"
    query = "select name, age / 0 as bad from fake"
  }
}
sink {
  Console {
    plugin_input = "bad"
  }
}
`.trim();
}

/** Light cluster: SeaTunnel only, no JDBC plugin download (FakeSource/Console/Sql). */
export async function prepareSchedulerRealCluster(
  page: Page,
): Promise<SchedulerCluster> {
  const installDir = `${process.env.E2E_INSTALLER_REAL_INSTALL_DIR ?? '/tmp/e2e/scheduler-real/seatunnel'}-sched-${Date.now()}`;
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
  return {clusterId: cluster.clusterId, httpPort: cluster.httpPort};
}

async function dsForm(
  token: string,
  path: string,
  body: Record<string, string>,
): Promise<unknown> {
  const encoded = new URLSearchParams(body).toString();
  const response = await fetch(`${dsBaseURL}${path}`, {
    method: 'POST',
    headers: {
      token,
      'Content-Type': 'application/x-www-form-urlencoded',
    },
    body: encoded,
  });
  const json = (await response.json()) as {
    code?: number;
    msg?: string;
    data?: unknown;
    success?: boolean;
  };
  if (json.code !== 0 && json.success === false) {
    throw new Error(`DS ${path} failed: ${JSON.stringify(json)}`);
  }
  return json.data;
}

async function dsGet(token: string, path: string): Promise<unknown> {
  const response = await fetch(`${dsBaseURL}${path}`, {
    headers: {token},
  });
  const json = (await response.json()) as {
    code?: number;
    data?: unknown;
    success?: boolean;
  };
  if (json.code !== 0 && json.success === false) {
    throw new Error(`DS GET ${path} failed: ${JSON.stringify(json)}`);
  }
  return json.data;
}

export async function bootstrapDolphinScheduler(
  stxServerURL: string,
  stxToken: string,
): Promise<DSBootstrap> {
  // Login → session, then create Open API token (H2 is empty each container start).
  const loginRes = await fetch(`${dsBaseURL}/login`, {
    method: 'POST',
    headers: {'Content-Type': 'application/x-www-form-urlencoded'},
    body: 'userName=admin&userPassword=dolphinscheduler123',
  });
  const loginJson = (await loginRes.json()) as {
    data?: {sessionId?: string};
  };
  const sessionId = loginJson.data?.sessionId;
  if (!sessionId) {
    throw new Error(`DS login failed: ${JSON.stringify(loginJson)}`);
  }

  const tokenRes = await fetch(`${dsBaseURL}/access-tokens`, {
    method: 'POST',
    headers: {
      sessionId,
      'Content-Type': 'application/x-www-form-urlencoded',
    },
    body: 'userId=1&expireTime=2030-12-31%2023%3A59%3A59',
  });
  const tokenJson = (await tokenRes.json()) as {data?: {token?: string}};
  const token = tokenJson.data?.token;
  if (!token) {
    throw new Error(`DS token create failed: ${JSON.stringify(tokenJson)}`);
  }

  const project = (await dsForm(token, '/projects', {
    projectName: `stx-e2e-${Date.now()}`,
    description: 'STX real scheduler E2E',
  })) as {code?: number};
  const projectCode = Number(project?.code || 0);
  if (!projectCode) {
    throw new Error(`DS project create failed: ${JSON.stringify(project)}`);
  }

  // EnvironmentConfig must be shell exports (not JSON) — Worker sources this file.
  // Quote STX_* so tokens with -/_ are safe; PATH keeps ${PATH} for the Worker shell.
  const envConfig = [
    'export PATH="/usr/local/bin:${PATH}"',
    `export STX_SERVER=${JSON.stringify(stxServerURL)}`,
    `export STX_TOKEN=${JSON.stringify(stxToken)}`,
    // Job-guard setsid fails under some Docker seccomp profiles → CLI exit 9
    // before submit. E2E skips the guard; production Workers leave this unset.
    'export STX_DISABLE_JOB_GUARD=1',
    '',
  ].join('\n');
  const environmentCode = Number(
    await dsForm(token, '/environment/create', {
      name: envName,
      config: envConfig,
      description: 'STX CLI for DS worker',
    }),
  );
  if (!environmentCode) {
    throw new Error('DS environment create returned empty code');
  }

  return {token, projectCode, environmentCode};
}

export async function issueStxCLIToken(
  request: APIRequestContext,
): Promise<string> {
  const response = await request.post(`${backendBaseURL}/api/v1/admin/cli-tokens`, {
    data: {
      user_id: 1,
      name: `ds-worker-${Date.now()}`,
      expires_in: 'never',
    },
  });
  expect(response.ok(), await response.text()).toBeTruthy();
  const body = (await response.json()) as {
    data?: {token?: string};
    error_msg?: string;
  };
  expect(body.error_msg ?? '').toBe('');
  const token = body.data?.token;
  if (!token?.startsWith('stx_')) {
    throw new Error(`CLI token missing: ${JSON.stringify(body)}`);
  }
  return token;
}

export async function upsertSchedulerConnection(
  request: APIRequestContext,
  options: {token: string; projectCode: number},
): Promise<number> {
  const list = await request.get(
    `${backendBaseURL}/api/v1/sync/scheduler-connections`,
  );
  expect(list.ok()).toBeTruthy();
  const listed = (await list.json()) as {
    data?: {items?: Array<{id?: number; name?: string}>};
  };
  const existing = (listed.data?.items || []).find((i) => i.name === 'e2e-ds');
  const payload = {
    name: 'e2e-ds',
    kind: 'dolphin',
    base_url: dsBaseURL,
    token: options.token,
    dolphin_version: '3.2.2',
    api_style: 'process',
    default_project_code: options.projectCode,
  };
  if (existing?.id) {
    const upd = await request.put(
      `${backendBaseURL}/api/v1/sync/scheduler-connections/${existing.id}`,
      {data: payload},
    );
    expect(upd.ok(), await upd.text()).toBeTruthy();
    return Number(existing.id);
  }
  const created = await request.post(
    `${backendBaseURL}/api/v1/sync/scheduler-connections`,
    {data: payload},
  );
  expect(created.ok(), await created.text()).toBeTruthy();
  const body = (await created.json()) as {data?: {id?: number}};
  const id = Number(body.data?.id || 0);
  expect(id).toBeGreaterThan(0);
  return id;
}

export async function pushTaskToDolphin(
  request: APIRequestContext,
  taskId: number,
  options: {
    connectionId: number;
    projectCode: number;
    workflowName: string;
  },
): Promise<{workflowCode: number; taskCode: number}> {
  const response = await request.post(
    `${backendBaseURL}/api/v1/sync/tasks/${taskId}/dolphin/push`,
    {
      data: {
        mode: 'new',
        connection_id: options.connectionId,
        project_code: options.projectCode,
        workflow_name: options.workflowName,
        worker_group: 'default',
        environment: envName,
      },
    },
  );
  expect(response.ok(), await response.text()).toBeTruthy();
  const body = (await response.json()) as {
    error_msg?: string;
    data?: {
      workflow_code?: number;
      task_code?: number;
      binding?: {workflow_code?: number; task_code?: number};
    };
  };
  expect(body.error_msg ?? '').toBe('');
  const workflowCode = Number(
    body.data?.workflow_code || body.data?.binding?.workflow_code || 0,
  );
  const taskCode = Number(
    body.data?.task_code || body.data?.binding?.task_code || 0,
  );
  expect(workflowCode).toBeGreaterThan(0);
  expect(taskCode).toBeGreaterThan(0);
  return {workflowCode, taskCode};
}

export async function startDSWorkflow(
  ds: DSBootstrap,
  workflowCode: number,
): Promise<void> {
  await dsForm(ds.token, `/projects/${ds.projectCode}/executors/start-process-instance`, {
    processDefinitionCode: String(workflowCode),
    failureStrategy: 'CONTINUE',
    warningType: 'NONE',
    execType: 'START_PROCESS',
    taskDependType: 'TASK_POST',
    complementDependentMode: 'OFF_MODE',
    runMode: 'RUN_MODE_SERIAL',
    processInstancePriority: 'MEDIUM',
    workerGroup: 'default',
    environmentCode: String(ds.environmentCode),
    startNodeList: '',
    scheduleTime: '',
    dryRun: '0',
  });
}

export async function waitForDSProcessTerminal(
  ds: DSBootstrap,
  timeoutMs = 360000,
): Promise<Array<{id: number; name: string; state: string}>> {
  const started = Date.now();
  let latest: Array<{id: number; name: string; state: string}> = [];
  while (Date.now() - started < timeoutMs) {
    const data = (await dsGet(
      ds.token,
      `/projects/${ds.projectCode}/process-instances?pageNo=1&pageSize=20`,
    )) as {totalList?: Array<{id?: number; name?: string; state?: string}>};
    latest = (data?.totalList || []).map((it) => ({
      id: Number(it.id || 0),
      name: String(it.name || ''),
      state: String(it.state || ''),
    }));
    const states = latest.map((i) => i.state);
    const running = states.some((s) =>
      /RUNNING|SUBMITTED|ACCEPT|DELAY|SERIAL_WAIT/.test(s),
    );
    if (latest.length > 0 && !running) {
      return latest;
    }
    await new Promise((r) => setTimeout(r, 5000));
  }
  // Fail path may linger RUNNING_EXECUTION while dumping stacks — still return snapshot.
  return latest;
}

export async function fetchLatestDSTaskLogs(
  ds: DSBootstrap,
): Promise<Array<{name: string; state: string; log: string; logPath: string}>> {
  const data = (await dsGet(
    ds.token,
    `/projects/${ds.projectCode}/task-instances?pageNo=1&pageSize=20`,
  )) as {
    totalList?: Array<{
      name?: string;
      state?: string;
      logPath?: string;
    }>;
  };
  const out: Array<{name: string; state: string; log: string; logPath: string}> =
    [];
  for (const it of data?.totalList || []) {
    const logPath = String(it.logPath || '');
    let log = '';
    if (logPath) {
      try {
        log = execFileSync(
          'docker',
          ['exec', dsContainer, 'cat', logPath],
          {encoding: 'utf8', maxBuffer: 8 * 1024 * 1024},
        );
      } catch (error) {
        log = `<<failed to read ${logPath}: ${String(error)}>>`;
      }
    }
    out.push({
      name: String(it.name || ''),
      state: String(it.state || ''),
      log,
      logPath,
    });
  }
  return out;
}

export function assertHumanReadableDSLog(log: string, label: string): void {
  expect(log, label).not.toContain('"event":');
  expect(log, label).toMatch(/\[stx\] waiting for job |\[stx\] job_status=/);
  expect(log, label).toContain('STX metrics final');
}

export async function createPublishedFakeTasks(
  request: APIRequestContext,
  clusterId: number,
): Promise<{success: SyncTaskFixture; fail: SyncTaskFixture}> {
  const success = await createSyncTask(request, {
    clusterId,
    engineVersion: seatunnelVersion,
    name: `sched-ok-${Date.now()}`,
    content: buildFakeConsoleBatchContent(),
  });
  await publishSyncTask(request, success.taskId, 'scheduler real success');

  const fail = await createSyncTask(request, {
    parentId: success.folderId,
    clusterId,
    engineVersion: seatunnelVersion,
    name: `sched-fail-${Date.now()}`,
    content: buildFakeSqlFailContent(),
  });
  await publishSyncTask(request, fail.taskId, 'scheduler real fail');
  return {success, fail};
}
