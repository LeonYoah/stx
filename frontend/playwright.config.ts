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

import path from 'node:path';
import {defineConfig, devices} from '@playwright/test';

const apiMode = process.env.E2E_API_MODE ?? 'real';
const installerRealMode = process.env.E2E_INSTALLER_REAL === '1';
const frontendPort = process.env.E2E_FRONTEND_PORT ?? '3000';
const frontendHost =
  process.env.E2E_FRONTEND_HOST ??
  (installerRealMode ? '127.0.0.1' : 'localhost');
const frontendBaseURL =
  process.env.E2E_FRONTEND_BASE_URL ?? `http://${frontendHost}:${frontendPort}`;
const backendBaseURL =
  process.env.E2E_BACKEND_BASE_URL ??
  (installerRealMode
    ? 'http://127.0.0.1:18000'
    : apiMode === 'real'
      ? 'http://localhost:17800'
      : 'http://127.0.0.1:8010');
const authFile = path.join(__dirname, '.playwright', 'auth', 'admin.json');
const backendConfigPath =
  process.env.E2E_CONFIG_PATH ??
  process.env.CONFIG_PATH ??
  '../config.e2e.yaml';
const backendServer =
  apiMode === 'real'
    ? {
        command: installerRealMode
          ? `bash -lc 'CONFIG_PATH=${process.env.E2E_INSTALLER_REAL_CONFIG_PATH ?? '../config.e2e.installer-real.yaml'} \"\${GO_BIN:-go}\" run .. api'`
          : `bash -lc 'CONFIG_PATH=${backendConfigPath} "\${GO_BIN:-go}" run .. api'`,
        url: `${backendBaseURL}/api/v1/health`,
        reuseExistingServer: installerRealMode ? false : !process.env.CI,
        timeout: installerRealMode ? 600_000 : 300_000,
      }
    : {
        command: 'node ./scripts/e2e/mock-api-server.mjs',
        url: `${backendBaseURL}/api/v1/health`,
        reuseExistingServer: !process.env.CI,
        timeout: 30_000,
      };

export default defineConfig({
  testDir: './e2e',
  fullyParallel: false,
  timeout: installerRealMode ? 900_000 : 60_000,
  expect: {
    timeout: installerRealMode ? 30_000 : 10_000,
  },
  reporter: [
    ['list'],
    ['html', {open: 'never', outputFolder: 'playwright-report'}],
  ],
  retries: process.env.CI ? 1 : 0,
  workers: process.env.CI ? 1 : undefined,
  use: {
    baseURL: frontendBaseURL,
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
  },
  projects: [
    {
      name: 'setup',
      testMatch: /auth\.setup\.ts/,
    },
    {
      name: 'login-ui',
      testMatch: /login-ui\.spec\.ts/,
      use: {
        ...devices['Desktop Chrome'],
        storageState: {cookies: [], origins: []},
      },
      dependencies: ['setup'],
    },
    {
      name: 'chromium',
      testIgnore: [/auth\.setup\.ts/, /login-ui\.spec\.ts/],
      use: {
        ...devices['Desktop Chrome'],
        storageState: authFile,
      },
      dependencies: ['setup'],
    },
  ],
  webServer: [
    backendServer,
    ...(installerRealMode
      ? [
          {
            command: 'node ./scripts/e2e/real-agent-supervisor.mjs',
            url: `http://127.0.0.1:${process.env.E2E_AGENT_SUPERVISOR_PORT ?? '18181'}/health`,
            reuseExistingServer: false,
            timeout: 300_000,
          },
        ]
      : []),
    {
      command:
        `NEXT_PUBLIC_BACKEND_BASE_URL=${backendBaseURL} ` +
        `NEXT_PUBLIC_FRONTEND_BASE_URL=${frontendBaseURL} ` +
        `pnpm exec next dev --hostname ${frontendHost} --port ${frontendPort}`,
      url: `${frontendBaseURL}/login`,
      reuseExistingServer: installerRealMode ? false : !process.env.CI,
      timeout: installerRealMode ? 300_000 : 120_000,
    },
  ],
});
