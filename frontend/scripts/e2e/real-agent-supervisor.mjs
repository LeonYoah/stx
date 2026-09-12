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

import http from 'node:http';
import process from 'node:process';
import path from 'node:path';
import fs from 'node:fs/promises';
import {fileURLToPath} from 'node:url';
import {spawn} from 'node:child_process';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(__dirname, '../../..');
const backendBaseURL =
  process.env.E2E_BACKEND_BASE_URL ?? 'http://127.0.0.1:18000';
const supervisorPort = Number(process.env.E2E_AGENT_SUPERVISOR_PORT || 18181);
const goBin = process.env.GO_BIN || 'go';
const agentConfigPath =
  process.env.E2E_AGENT_REAL_CONFIG_PATH ??
  path.resolve(repoRoot, 'config.e2e.agent-real.yaml');

let shuttingDown = false;
let backendReady = false;
let agentStarted = false;
let agentExited = false;
let lastError = '';

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

async function waitForBackendHealth(timeoutMs = 120000) {
  const startedAt = Date.now();
  while (Date.now() - startedAt < timeoutMs) {
    try {
      const response = await fetch(`${backendBaseURL}/api/v1/health`);
      if (response.ok) {
        backendReady = true;
        return;
      }
    } catch {
      // Ignore until timeout.
    }
    await sleep(1000);
  }
  throw new Error(`backend did not become healthy within ${timeoutMs}ms`);
}

/**
 * When Control Plane auto-enables gRPC TLS, download CA and patch Agent config.
 * Control Plane 自动开启 gRPC TLS 时，下载 CA 并改写 Agent 配置。
 * If TLS is off (404), keep the original plaintext Agent config.
 * 若 TLS 未开启（404），保留原始明文 Agent 配置。
 */
async function resolveAgentConfigWithTLS(baseConfigPath) {
  const caURL = `${backendBaseURL}/api/v1/agent/ca.crt`;
  let response;
  try {
    response = await fetch(caURL);
  } catch (error) {
    process.stderr.write(
      `[installer-real-e2e] skip TLS patch; CA fetch failed: ${error}\n`,
    );
    return baseConfigPath;
  }

  if (response.status === 404) {
    process.stderr.write(
      '[installer-real-e2e] Control Plane gRPC TLS disabled; using plaintext Agent config\n',
    );
    return baseConfigPath;
  }
  if (!response.ok) {
    throw new Error(
      `failed to download CA from ${caURL}: HTTP ${response.status}`,
    );
  }

  const caPEM = await response.text();
  if (!caPEM.includes('BEGIN CERTIFICATE')) {
    throw new Error(
      `CA download from ${caURL} did not return a PEM certificate`,
    );
  }

  const configDir = path.dirname(baseConfigPath);
  const caDir = path.join(configDir, 'certs');
  const caPath = path.join(caDir, 'ca.crt');
  const patchedConfigPath = path.join(
    configDir,
    'config.e2e.agent-real.tls.yaml',
  );
  const caPathYAML = JSON.stringify(caPath);

  await fs.mkdir(caDir, {recursive: true});
  await fs.writeFile(caPath, caPEM, 'utf8');

  const original = await fs.readFile(baseConfigPath, 'utf8');
  // Replace only the tls: block and its nested keys (deeper indent), not siblings like token.
  // 只替换 tls: 及其嵌套字段（更深缩进），不要吃掉同级的 token 等字段。
  let patched = original.replace(
    /^([ \t]*)tls:\n(?:\1[ \t]+.+\n)*/m,
    `$1tls:\n$1  enabled: true\n$1  ca_file: ${caPathYAML}\n`,
  );
  if (patched === original) {
    // Fallback: insert a TLS block under control_plane if the template shape drifts.
    // 兜底：模板结构变化时，在 control_plane 下插入 TLS 块。
    patched = original.replace(
      /(control_plane:\n(?:[ \t]+.+\n)*?)([ \t]+token:)/m,
      `$1  tls:\n    enabled: true\n    ca_file: ${caPathYAML}\n$2`,
    );
  }
  if (!patched.includes('enabled: true') || !patched.includes(caPath)) {
    throw new Error('failed to patch Agent config for gRPC TLS');
  }
  await fs.writeFile(patchedConfigPath, patched, 'utf8');

  process.stderr.write(
    `[installer-real-e2e] Control Plane gRPC TLS enabled; Agent CA written to ${caPath}\n`,
  );
  return patchedConfigPath;
}

function startHealthServer() {
  const server = http.createServer((_req, res) => {
    if (backendReady && agentStarted && !agentExited) {
      res.writeHead(200, {'Content-Type': 'application/json'});
      res.end(JSON.stringify({status: 'ok'}));
      return;
    }
    res.writeHead(503, {'Content-Type': 'application/json'});
    res.end(
      JSON.stringify({
        status: 'starting',
        backendReady,
        agentStarted,
        agentExited,
        lastError,
      }),
    );
  });

  return new Promise((resolve, reject) => {
    server.once('error', reject);
    server.listen(supervisorPort, '127.0.0.1', () => resolve(server));
  });
}

async function main() {
  const server = await startHealthServer();

  try {
    await waitForBackendHealth();

    // Align Agent transport with Control Plane TLS bootstrap (openssl auto-enable).
    // 与 Control Plane TLS 自动引导对齐（openssl 可用时默认开启）。
    const effectiveAgentConfigPath =
      await resolveAgentConfigWithTLS(agentConfigPath);

    const agentChild = spawn(
      goBin,
      ['run', './cmd', '--config', effectiveAgentConfigPath],
      {
        cwd: path.join(repoRoot, 'agent'),
        env: {
          ...process.env,
          AGENT_LOG_FILE: path.join(
            repoRoot,
            'tmp/e2e/installer-real/logs/stx-agent.log',
          ),
        },
        stdio: 'inherit',
      },
    );

    agentStarted = true;

    agentChild.once('exit', (code, signal) => {
      agentExited = true;
      if (!shuttingDown) {
        lastError = `agent exited unexpectedly (code=${code ?? 'null'}, signal=${signal ?? 'null'})`;
      }
    });

    agentChild.once('error', (error) => {
      agentExited = true;
      lastError = error.message;
    });

    const shutdown = async () => {
      if (shuttingDown) {
        return;
      }
      shuttingDown = true;
      if (!agentExited) {
        agentChild.kill('SIGTERM');
        await sleep(1500);
        if (!agentExited) {
          agentChild.kill('SIGKILL');
        }
      }
      await new Promise((resolve) => server.close(resolve));
      process.exit(0);
    };

    process.on('SIGINT', shutdown);
    process.on('SIGTERM', shutdown);

    // Give the agent a short window to register before Playwright starts.
    await sleep(4000);
  } catch (error) {
    lastError = error instanceof Error ? error.message : String(error);
    process.stderr.write(`[installer-real-e2e] ${lastError}\n`);
    process.exitCode = 1;
  }
}

await main();
