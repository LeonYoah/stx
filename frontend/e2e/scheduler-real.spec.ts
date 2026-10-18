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

import {expect, test, type Page} from '@playwright/test';

import {
  assertHumanReadableDSLog,
  bootstrapDolphinScheduler,
  createPublishedFakeTasks,
  fetchLatestDSTaskLogs,
  issueStxCLIToken,
  prepareSchedulerRealCluster,
  pushTaskToDolphin,
  startDSWorkflow,
  upsertSchedulerConnection,
  waitForDSProcessTerminal,
  type DSBootstrap,
} from './helpers/scheduler-real';

test.describe.serial('scheduler real DolphinScheduler flows', () => {
  test.setTimeout(12 * 60 * 1000);

  let preparedPage: Page | null = null;
  let clusterId = 0;
  let ds: DSBootstrap;
  let connectionId = 0;

  test.beforeAll(async ({browser}) => {
    expect(
      process.env.E2E_SCHEDULER_REAL,
      'E2E_SCHEDULER_REAL=1 required (use run-real-scheduler.sh)',
    ).toBe('1');
    preparedPage = await browser.newPage();
    await preparedPage.goto('/dashboard');
    const cluster = await prepareSchedulerRealCluster(preparedPage);
    clusterId = cluster.clusterId;

    const request = preparedPage.context().request;
    const stxToken = await issueStxCLIToken(request);
    const stxServer =
      process.env.E2E_BACKEND_BASE_URL ?? 'http://127.0.0.1:18000';
    // Host-network DS Worker reaches STX on loopback.
    ds = await bootstrapDolphinScheduler(stxServer, stxToken);
    connectionId = await upsertSchedulerConnection(request, {
      token: ds.token,
      projectCode: ds.projectCode,
    });
  });

  test.afterAll(async () => {
    if (preparedPage) {
      await preparedPage.close();
    }
  });

  test('DS submit --wait success and fail with plain-text logs and final metrics', async ({
    page,
  }) => {
    const request = page.context().request;
    const {success, fail} = await createPublishedFakeTasks(request, clusterId);

    const okPush = await pushTaskToDolphin(request, success.taskId, {
      connectionId,
      projectCode: ds.projectCode,
      workflowName: `ok-${success.name}`,
    });
    const failPush = await pushTaskToDolphin(request, fail.taskId, {
      connectionId,
      projectCode: ds.projectCode,
      workflowName: `fail-${fail.name}`,
    });

    await startDSWorkflow(ds, okPush.workflowCode);
    await startDSWorkflow(ds, failPush.workflowCode);

    // Success should finish quickly; fail may linger RUNNING_EXECUTION while stacks flush.
    const processes = await waitForDSProcessTerminal(ds, 180000);
    expect(processes.length).toBeGreaterThanOrEqual(2);

    const logs = await fetchLatestDSTaskLogs(ds);
    expect(logs.length).toBeGreaterThanOrEqual(2);

    const successLog = logs.find((l) => /ok-|sched-ok|console|SUCCESS/i.test(l.name + l.state) && l.state === 'SUCCESS')
      || logs.find((l) => l.state === 'SUCCESS');
    const failLog = logs.find((l) => l.state === 'FAILURE')
      || logs.find((l) => /fail|assert/i.test(l.name));

    const snippet = (log: string) => {
      const markers = [
        /start job guard/i,
        /\[stx\]/,
        /exitStatusCode/,
        /STX_DISABLE_JOB_GUARD/,
        /Error:/,
        /GLIBC/,
        /not found/i,
        /permission/i,
      ];
      for (const re of markers) {
        const idx = log.search(re);
        if (idx >= 0) {
          return log.slice(Math.max(0, idx - 240), idx + 900);
        }
      }
      return `${log.slice(0, 400)}\n...\n${log.slice(-400)}`;
    };
    const logDigest = logs.map((l) => ({
      n: l.name,
      s: l.state,
      path: l.logPath,
      snippet: snippet(l.log || ''),
    }));
    expect(
      successLog,
      `success task log missing: ${JSON.stringify(logDigest)}`,
    ).toBeTruthy();
    expect(
      failLog,
      `fail task log missing: ${JSON.stringify(logDigest)}`,
    ).toBeTruthy();

    assertHumanReadableDSLog(successLog!.log, 'success DS log');
    expect(successLog!.log).toContain('[stx] job_status=success');
    expect(successLog!.log).toMatch(/\[stx\] done job_id=\d+ status=success/);

    assertHumanReadableDSLog(failLog!.log, 'fail DS log');
    expect(failLog!.log).toContain('[stx] job_status=failed');
    // Engine failure evidence (division by zero) with real newlines.
    expect(failLog!.log).toMatch(/ArithmeticException|\/ by zero|TRANSFORM_COMMON/);
  });
});
