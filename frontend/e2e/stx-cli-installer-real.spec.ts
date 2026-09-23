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
import {expect, test} from '@playwright/test';

interface CLIResult<T = unknown> {
  api_version: string;
  operation_id: string;
  request_id: string;
  data: T;
  result_meta: {
    complete: boolean;
    next_command?: string;
  };
}

interface CLIEvent {
  event?: string;
  code?: string;
  message?: string;
  operation_id?: string;
}

interface CLIExecution<T = unknown> {
  result: CLIResult<T>;
  events: CLIEvent[];
}

const repoRoot = path.resolve(process.cwd(), '..');
const stxBinary =
  process.env.E2E_STX_CLI_BIN ?? path.join(repoRoot, 'dist', 'stx');
const serverURL = process.env.E2E_BACKEND_BASE_URL ?? 'http://127.0.0.1:18000';
const cliConfigHome = path.join(
  process.env.E2E_INSTALLER_REAL_TMP_DIR ?? path.join(repoRoot, 'tmp', 'e2e'),
  'stx-cli-config',
);
const seatunnelVersion = process.env.E2E_INSTALLER_REAL_VERSION ?? '2.3.13';
const installDir = `${
  process.env.E2E_INSTALLER_REAL_INSTALL_DIR ??
  path.join(repoRoot, 'tmp', 'e2e', 'installer-real', 'seatunnel-2.3.13')
}-cli`;
const clusterPort = Number(
  process.env.E2E_INSTALLER_REAL_CLI_CLUSTER_PORT ?? '38183',
);
const workerPort = Number(
  process.env.E2E_INSTALLER_REAL_CLI_WORKER_PORT ?? '38184',
);
const httpPort = Number(
  process.env.E2E_INSTALLER_REAL_CLI_HTTP_PORT ?? '38082',
);
const javaProxyPort = Number(
  process.env.E2E_INSTALLER_REAL_CLI_JAVA_PROXY_PORT ?? '38083',
);

function parseEvents(stderr: string): CLIEvent[] {
  return stderr
    .split('\n')
    .map((line) => line.trim())
    .filter(Boolean)
    .map((line) => JSON.parse(line) as CLIEvent);
}

// 直接执行打包后的 CLI，并分别校验 stdout 单结果与 stderr 事件流。
// Execute the packaged CLI directly and validate the single stdout result and stderr event stream separately.
async function runSTX<T>(
  args: string[],
  options: {stdin?: string; timeoutMs?: number} = {},
): Promise<CLIExecution<T>> {
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
      if (code !== 0) {
        reject(
          new Error(
            `stx ${args.join(' ')} failed: code=${code} signal=${signal || '-'} stdout=${stdout.trim()} stderr=${stderr.trim()}`,
          ),
        );
        return;
      }
      try {
        resolve({
          result: JSON.parse(stdout.trim()) as CLIResult<T>,
          events: parseEvents(stderr),
        });
      } catch (error) {
        reject(
          new Error(
            `stx ${args.join(' ')} returned invalid output: ${String(error)} stdout=${stdout.trim()} stderr=${stderr.trim()}`,
          ),
        );
      }
    });

    child.stdin.end(options.stdin ?? '');
  });
}

test.describe.serial('STX CLI real installer', () => {
  test('logs in, prechecks, installs and verifies through dist/stx', async () => {
    await fs.access(stxBinary);
    await fs.mkdir(cliConfigHome, {recursive: true});

    const login = await runSTX<{
      namespace?: string;
      token_set?: boolean;
      user?: {username?: string};
    }>(
      [
        'login',
        '--server',
        serverURL,
        '--namespace',
        'installer-real',
        '--username',
        process.env.E2E_USERNAME ?? 'admin',
        '--password-stdin',
      ],
      {stdin: `${process.env.E2E_PASSWORD ?? 'admin123'}\n`},
    );
    expect(login.result.operation_id).toBe('auth.cli.login');
    expect(login.result.data.token_set).toBe(true);
    expect(JSON.stringify(login.result)).not.toContain('admin123');

    const capability = await runSTX<{
      operation?: {operation_id?: string; allowed?: boolean};
    }>(['capability', 'get', 'host.install.start']);
    expect(capability.result.data.operation?.operation_id).toBe(
      'host.install.start',
    );
    expect(capability.result.data.operation?.allowed).toBe(true);

    const hosts = await runSTX<{
      hosts?: Array<{
        id?: number | string;
        is_online?: boolean;
        agent_status?: string;
      }>;
    }>(['host', 'list', '--current', '1', '--size', '100']);
    const host = (hosts.result.data.hosts ?? []).find(
      (item) => item.is_online && item.agent_status === 'installed',
    );
    expect(host?.id).toBeTruthy();
    const hostId = String(host?.id);

    const clusterName = `e2e-cli-installer-${Date.now()}`;
    const cluster = await runSTX<{id?: number | string; name?: string}>([
      'cluster',
      'create',
      '--name',
      clusterName,
      '--deployment-mode',
      'hybrid',
      '--version',
      seatunnelVersion,
      '--install-dir',
      installDir,
      '--confirm',
      '--idempotency-key',
      `e2e-cli-cluster-${Date.now()}`,
    ]);
    expect(cluster.result.data.id).toBeTruthy();
    expect(cluster.events.some((event) => event.event === 'warning')).toBe(
      true,
    );
    const clusterId = String(cluster.result.data.id);

    const precheck = await runSTX<{
      overall_status?: string;
      checks?: Array<{status?: string}>;
    }>([
      'host',
      'precheck',
      hostId,
      '--install-dir',
      installDir,
      '--port',
      `${clusterPort},${workerPort},${httpPort},${javaProxyPort}`,
    ]);
    expect(['passed', 'warning']).toContain(
      precheck.result.data.overall_status,
    );

    const started = await runSTX<{status?: string; host_id?: string}>([
      'host',
      'install',
      'start',
      hostId,
      '--version',
      seatunnelVersion,
      '--install-dir',
      installDir,
      '--install-mode',
      'online',
      '--mirror',
      'apache',
      '--deployment-mode',
      'hybrid',
      '--node-role',
      'master',
      '--cluster-id',
      clusterId,
      '--cluster-port',
      String(clusterPort),
      '--worker-port',
      String(workerPort),
      '--http-port',
      String(httpPort),
      '--java-proxy-port',
      String(javaProxyPort),
      '--enable-http',
      '--confirm',
      '--idempotency-key',
      `e2e-cli-install-${Date.now()}`,
    ]);
    expect(started.result.data.status).toBe('running');
    expect(started.result.result_meta.next_command).toBe(
      `stx host install status get ${hostId}`,
    );
    expect(started.events.some((event) => event.event === 'warning')).toBe(
      true,
    );

    const deadline = Date.now() + 300000;
    let finalStatus: CLIResult<{
      status?: string;
      current_step?: string;
      error?: string;
      message?: string;
    }> | null = null;
    while (Date.now() < deadline) {
      const status = await runSTX<{
        status?: string;
        current_step?: string;
        error?: string;
        message?: string;
      }>(['host', 'install', 'status', 'get', hostId]);
      finalStatus = status.result;
      if (status.result.data.status === 'failed') {
        throw new Error(
          `CLI installation failed at ${status.result.data.current_step}: ${status.result.data.error || status.result.data.message}`,
        );
      }
      if (status.result.data.status === 'success') {
        break;
      }
      await new Promise((resolve) => setTimeout(resolve, 1000));
    }
    expect(finalStatus?.data.status).toBe('success');
    expect(finalStatus?.data.current_step).toBe('complete');

    const nodes = await runSTX<
      Array<{role?: string; install_dir?: string; cluster_id?: number | string}>
    >(['cluster', 'node', 'list', clusterId]);
    expect(nodes.result.data).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          role: 'master/worker',
          install_dir: installDir,
        }),
      ]),
    );

    await expect(
      fs.stat(path.join(installDir, 'bin', 'seatunnel-cluster.sh')),
    ).resolves.toBeTruthy();
    await expect(
      fs.stat(path.join(installDir, 'config', 'seatunnel.yaml')),
    ).resolves.toBeTruthy();
  });
});
