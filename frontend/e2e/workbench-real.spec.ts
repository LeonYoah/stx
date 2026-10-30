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

import {expect, test, type Browser, type Page} from '@playwright/test';

import {
  buildMysqlHundredTablesTaskContent,
  buildMysqlMultiSourceMultiSinkContent,
  buildMysqlMultiTransformContent,
  buildMysqlUsersTaskContent,
  buildScheduleDefinition,
  buildStreamingFakeTaskContent,
  cleanupWorkbenchRealMySQLFixture,
  createSyncTask,
  getJobCheckpointSnapshot,
  getSyncDAG,
  getSyncJob,
  getSyncJobLogs,
  inspectCheckpointFile,
  listCheckpointFiles,
  listSyncTree,
  nextMinuteCronExpression,
  prepareWorkbenchRealCluster,
  previewSyncTask,
  publishSyncTask,
  recoverSyncJob,
  runSyncTask,
  seedWorkbenchRealMySQLFixture,
  ensureWorkbenchRealMySQLFixture,
  testSyncConnections,
  validateSyncTask,
  waitForJobStatus,
  waitForPreviewCleanup,
  waitForPreviewRows,
  waitForScheduledJob,
  cancelSyncJob,
  type SyncTaskFixture,
} from './helpers/workbench-real';

const seatunnelVersion = process.env.E2E_INSTALLER_REAL_VERSION ?? '2.3.13';

test.describe.serial('workbench real flows', () => {
  test.setTimeout(30 * 60 * 1000);

  let clusterId = 0;
  let clusterHttpPort = 0;
  let preparedPage: Page | null = null;
  let usersTask: SyncTaskFixture;
  let hundredTask: SyncTaskFixture;
  let multiSourceTask: SyncTaskFixture;
  let multiTransformTask: SyncTaskFixture;
  let streamingTask: SyncTaskFixture;
  let scheduledTask: SyncTaskFixture;

  test.beforeAll(async ({browser}) => {
    preparedPage = await browser.newPage();
    await preparedPage.goto('/dashboard');
    const cluster = await prepareWorkbenchRealCluster(preparedPage);
    clusterId = cluster.clusterId;
    clusterHttpPort = cluster.httpPort;
    await ensureWorkbenchRealMySQLFixture();
    await seedWorkbenchRealMySQLFixture();
  });

  test.afterAll(async () => {
    if (preparedPage) {
      await preparedPage.close();
    }
    await cleanupWorkbenchRealMySQLFixture();
  });

  test('covers workbench batch, preview, dag, metrics, heavy graph and preview ttl cleanup', async ({page}) => {
    const request = page.context().request;

    await page.goto('/workbench');
    await expect(page.getByLabel(/新建目录|New folder/i)).toBeVisible({timeout: 120000});
    await expect(page.getByRole('button', {name: /^(任务|Jobs)$/i})).toBeVisible();

    usersTask = await createSyncTask(request, {
      clusterId,
      engineVersion: seatunnelVersion,
      name: `real-ci-users-${Date.now()}`,
      content: buildMysqlUsersTaskContent(),
    });
    hundredTask = await createSyncTask(request, {
      parentId: usersTask.folderId,
      clusterId,
      engineVersion: seatunnelVersion,
      name: `real-ci-hundred-${Date.now()}`,
      content: buildMysqlHundredTablesTaskContent(),
    });
    multiSourceTask = await createSyncTask(request, {
      parentId: usersTask.folderId,
      clusterId,
      engineVersion: seatunnelVersion,
      name: `real-ci-multi-source-${Date.now()}`,
      content: buildMysqlMultiSourceMultiSinkContent(),
    });
    multiTransformTask = await createSyncTask(request, {
      parentId: usersTask.folderId,
      clusterId,
      engineVersion: seatunnelVersion,
      name: `real-ci-multi-transform-${Date.now()}`,
      content: buildMysqlMultiTransformContent(),
    });

    await publishSyncTask(request, usersTask.taskId);
    await publishSyncTask(request, hundredTask.taskId);
    await publishSyncTask(request, multiSourceTask.taskId);
    await publishSyncTask(request, multiTransformTask.taskId);

    const tree = await listSyncTree(request);
    const treeText = JSON.stringify(tree);
    expect(treeText).toContain(usersTask.name);
    expect(treeText).toContain(hundredTask.name);
    expect(treeText).toContain(multiSourceTask.name);
    expect(treeText).toContain(multiTransformTask.name);

    const validation = await validateSyncTask(request, usersTask.taskId);
    expect(validation.valid).toBe(true);

    const connections = await testSyncConnections(request, usersTask.taskId);
    expect(connections.valid).toBe(true);

    const hundredDag = await getSyncDAG(request, hundredTask.taskId);
    const hundredDagText = JSON.stringify(hundredDag);
    expect(hundredDagText).toContain('seatunnel_demo.bulk_001');
    expect(hundredDagText).toContain('seatunnel_demo.bulk_100');

    const multiSourceDag = await getSyncDAG(request, multiSourceTask.taskId);
    expect((multiSourceDag.nodes ?? []).length).toBeGreaterThanOrEqual(6);
    expect((multiSourceDag.edges ?? []).length).toBeGreaterThanOrEqual(4);

    const multiTransformDag = await getSyncDAG(request, multiTransformTask.taskId);
    expect((multiTransformDag.nodes ?? []).length).toBeGreaterThanOrEqual(4);
    expect(JSON.stringify(multiTransformDag)).toContain('copied_name_2');

    const usersDag = await getSyncDAG(request, usersTask.taskId);
    const vertexInfoMap = usersDag.webui_job?.jobDag?.vertexInfoMap ?? {};
    const schemaJson = JSON.stringify(vertexInfoMap);
    expect(schemaJson).toContain('tableSchemas');
    expect(schemaJson).toContain('seatunnel_demo.users');
    expect(schemaJson).toContain('INT');
    expect(schemaJson).toContain('PRIMARY');

    const previewJob = await previewSyncTask(request, usersTask.taskId, 10);
    const previewSnapshot = await waitForPreviewRows(request, previewJob.id);
    expect((previewSnapshot.tables ?? []).length).toBeGreaterThanOrEqual(1);
    expect(JSON.stringify(previewSnapshot.tables)).toContain('seatunnel_demo.users');
    await waitForPreviewCleanup(request, previewJob.id, 240000);

    const batchJob = await runSyncTask(request, usersTask.taskId);
    const finishedBatchJob = await waitForJobStatus(
      request,
      batchJob.id,
      (job) => ['success', 'failed', 'canceled'].includes((job.status ?? '').toLowerCase()),
      240000,
    );
    expect((finishedBatchJob.status ?? '').toLowerCase()).toBe('success');

    const batchJobDetails = await getSyncJob(request, batchJob.id);
    expect(JSON.stringify(batchJobDetails.result_preview ?? {})).toContain('metrics');
    const jobLogs = await getSyncJobLogs(request, batchJob.id);
    expect(typeof (jobLogs.logs ?? '')).toBe('string');

    const hundredJob = await runSyncTask(request, hundredTask.taskId);
    const finishedHundredJob = await waitForJobStatus(
      request,
      hundredJob.id,
      (job) => ['success', 'failed', 'canceled'].includes((job.status ?? '').toLowerCase()),
      300000,
    );
    expect((finishedHundredJob.status ?? '').toLowerCase()).toBe('success');
  });

  test('covers streaming savepoint, recover, checkpoint details and schedule trigger', async ({page}) => {
    const request = page.context().request;

    streamingTask = await createSyncTask(request, {
      clusterId,
      engineVersion: seatunnelVersion,
      parentId: usersTask.folderId,
      name: `real-ci-stream-${Date.now()}`,
      content: buildStreamingFakeTaskContent(),
    });
    await publishSyncTask(request, streamingTask.taskId);

    const streamingJob = await runSyncTask(request, streamingTask.taskId);
    const runningJob = await waitForJobStatus(
      request,
      streamingJob.id,
      (job) => (job.status ?? '').toLowerCase() === 'running',
      240000,
    );
    expect((runningJob.status ?? '').toLowerCase()).toBe('running');

    await expect
      .poll(async () => {
        const snapshot = await getJobCheckpointSnapshot(request, streamingJob.id);
        return (snapshot.history?.length ?? 0) > 0 || Boolean(snapshot.overview?.pipelines?.[0]?.latestCompleted?.checkpointId);
      }, {timeout: 240000, intervals: [2000, 5000, 10000]})
      .toBeTruthy();

    const checkpointSnapshot = await getJobCheckpointSnapshot(request, streamingJob.id);
    expect(JSON.stringify(checkpointSnapshot)).toContain('COMPLETED');

    await cancelSyncJob(request, streamingJob.id, true);
    const stoppedJob = await waitForJobStatus(
      request,
      streamingJob.id,
      (job) => Boolean(job.finished_at),
      240000,
    );
    expect(stoppedJob.platform_job_id).toBeTruthy();

    const checkpointFiles = await listCheckpointFiles(request, clusterId, stoppedJob.platform_job_id as string);
    expect(checkpointFiles.length).toBeGreaterThan(0);
    const ckFile = checkpointFiles.find((item) => item.name?.endsWith('.ser')) ?? checkpointFiles[0];
    expect(ckFile?.path).toBeTruthy();

    const ckInspect = await inspectCheckpointFile(
      request,
      clusterId,
      ckFile.path as string,
      buildStreamingFakeTaskContent(),
    );
    expect(ckInspect.completed_checkpoint).toBeTruthy();
    expect(Array.isArray(ckInspect.action_states ?? [])).toBe(true);
    expect(Array.isArray(ckInspect.task_statistics ?? [])).toBe(true);

    const recoveredJob = await recoverSyncJob(request, streamingJob.id);
    const recoveredRunningJob = await waitForJobStatus(
      request,
      recoveredJob.id,
      (job) => (job.status ?? '').toLowerCase() === 'running',
      240000,
    );
    expect((recoveredRunningJob.status ?? '').toLowerCase()).toBe('running');

    await cancelSyncJob(request, recoveredJob.id, false);
    await waitForJobStatus(request, recoveredJob.id, (job) => Boolean(job.finished_at), 240000);

    scheduledTask = await createSyncTask(request, {
      clusterId,
      engineVersion: seatunnelVersion,
      parentId: usersTask.folderId,
      name: `real-ci-schedule-${Date.now()}`,
      content: buildMysqlUsersTaskContent(),
      definition: buildScheduleDefinition(nextMinuteCronExpression()),
    });
    await publishSyncTask(request, scheduledTask.taskId);

    const scheduledJob = await waitForScheduledJob(request, scheduledTask.taskId, 240000);
    expect(scheduledJob.run_type).toBe('schedule');
    const finishedScheduledJob = await waitForJobStatus(
      request,
      scheduledJob.id,
      (job) => ['success', 'failed', 'canceled'].includes((job.status ?? '').toLowerCase()),
      240000,
    );
    expect((finishedScheduledJob.status ?? '').toLowerCase()).toBe('success');

    await page.goto('/workbench');
    await expect(page.getByRole('button', {name: /^(任务|Jobs)$/i})).toBeVisible({timeout: 120000});
    expect(clusterHttpPort).toBeGreaterThan(0);
  });
});
