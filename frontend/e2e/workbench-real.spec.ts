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
  buildCheckpointRestoreTaskContent,
  buildMysqlHundredTablesTaskContent,
  buildMysqlMultiSourceMultiSinkContent,
  buildMysqlMultiTransformContent,
  buildMysqlUsersTaskContent,
  buildScheduleDefinition,
  buildStreamingFakeTaskContent,
  checkpointJobDirectory,
  cleanupWorkbenchRealMySQLFixture,
  createSyncTask,
  engineSupportsRestoreMode,
  findEngineLogLine,
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
  readEngineSourceReceivedCount,
  recoverSyncJob,
  runSyncTask,
  seedWorkbenchRealMySQLFixture,
  setCheckpointDirectoryMode,
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
const reuseCluster = process.env.E2E_WORKBENCH_REUSE_CLUSTER === '1';
const skipMysql = process.env.E2E_WORKBENCH_SKIP_MYSQL === '1';
const restoreOnSeaTunnel3 = engineSupportsRestoreMode(seatunnelVersion);

test.describe.serial('workbench real flows', () => {
  test.setTimeout(30 * 60 * 1000);

  let clusterId = 0;
  let clusterHttpPort = 0;
  let installDir = process.env.E2E_WORKBENCH_INSTALL_DIR ?? '';
  let preparedPage: Page | null = null;
  let usersTask: SyncTaskFixture | undefined;
  let hundredTask: SyncTaskFixture | undefined;
  let multiSourceTask: SyncTaskFixture | undefined;
  let multiTransformTask: SyncTaskFixture | undefined;
  let streamingTask: SyncTaskFixture | undefined;
  let scheduledTask: SyncTaskFixture | undefined;

  test.beforeAll(async ({browser}) => {
    preparedPage = await browser.newPage();
    await preparedPage.goto('/dashboard');
    if (reuseCluster) {
      // 本机没有 Docker 时，复用已经注册好的集群，只跑不依赖 MySQL 的恢复用例。
      // Without Docker, reuse a registered cluster and run only the restore cases that do not need MySQL.
      clusterId = Number(process.env.E2E_WORKBENCH_CLUSTER_ID ?? '0');
      clusterHttpPort = Number(process.env.E2E_WORKBENCH_HTTP_PORT ?? '0');
      expect(clusterId).toBeGreaterThan(0);
      expect(clusterHttpPort).toBeGreaterThan(0);
      expect(installDir).toBeTruthy();
      return;
    }
    const cluster = await prepareWorkbenchRealCluster(preparedPage);
    clusterId = cluster.clusterId;
    clusterHttpPort = cluster.httpPort;
    installDir = cluster.installDir;
    await ensureWorkbenchRealMySQLFixture();
    await seedWorkbenchRealMySQLFixture();
  });

  test.afterAll(async () => {
    if (preparedPage) {
      await preparedPage.close();
    }
    if (!reuseCluster) {
      await cleanupWorkbenchRealMySQLFixture();
    }
  });

  test('covers workbench batch, preview, dag, metrics, heavy graph and preview ttl cleanup', async ({page}) => {
    test.skip(skipMysql, 'MySQL fixture is skipped for this run');
    const request = page.context().request;

    await page.goto('/workbench');
    await expect(page.getByLabel(/新建目录|New folder/i)).toBeVisible({timeout: 120000});
    await expect(page.getByLabel(/任务|Jobs/i)).toBeVisible();

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
      parentId: usersTask?.folderId,
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
    expect((stoppedJob.status ?? '').toLowerCase()).toBe('savepoint_done');

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
    const recoveredDetails = await getSyncJob(request, recoveredJob.id);
    assertRecoverSubmit(
      recoveredDetails,
      restoreOnSeaTunnel3 ? 'savepoint' : 'legacy',
      String(stoppedJob.platform_job_id),
    );
    await expect
      .poll(async () => {
        if (restoreOnSeaTunnel3) {
          return findEngineLogLine(
            installDir,
            `restore with SAVEPOINT.*${stoppedJob.platform_job_id}|${stoppedJob.platform_job_id}.*restore with SAVEPOINT`,
          );
        }
        return findEngineLogLine(
          installDir,
          `${stoppedJob.platform_job_id}.*SAVEPOINT_TYPE|SAVEPOINT_TYPE.*${stoppedJob.platform_job_id}`,
        );
      }, {timeout: 60000, intervals: [2000, 5000]})
      .not.toBe('');

    await cancelSyncJob(request, recoveredJob.id, false);
    await waitForJobStatus(request, recoveredJob.id, (job) => Boolean(job.finished_at), 240000);

    if (!skipMysql) {
      expect(usersTask?.folderId).toBeTruthy();
      scheduledTask = await createSyncTask(request, {
        clusterId,
        engineVersion: seatunnelVersion,
        parentId: usersTask?.folderId,
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
    }

    await page.goto('/workbench');
    await expect(page.getByLabel(/任务|Jobs/i)).toBeVisible({timeout: 120000});
    expect(clusterHttpPort).toBeGreaterThan(0);
  });

  test('recovers a failed SeaTunnel 3 job from checkpoint without replaying rows', async ({page}) => {
    test.skip(!restoreOnSeaTunnel3, 'checkpoint restoreMode applies to SeaTunnel 3.x');
    const request = page.context().request;
    const folderId = streamingTask?.folderId;

    const task = await createSyncTask(request, {
      clusterId,
      engineVersion: seatunnelVersion,
      parentId: folderId,
      name: `real-ci-ckpt-restore-${Date.now()}`,
      content: buildCheckpointRestoreTaskContent(),
    });
    await publishSyncTask(request, task.taskId);

    const failedSource = await runSyncTask(request, task.taskId);
    await waitForJobStatus(
      request,
      failedSource.id,
      (job) => (job.status ?? '').toLowerCase() === 'running',
      240000,
    );
    let checkpointBefore = 0;
    await expect
      .poll(async () => {
        const snapshot = await getJobCheckpointSnapshot(request, failedSource.id);
        checkpointBefore = Number(
          snapshot.overview?.pipelines?.[0]?.latestCompleted?.checkpointId ??
            snapshot.history?.[0]?.checkpoint?.checkpointId ??
            0,
        );
        return checkpointBefore > 0;
      }, {timeout: 240000, intervals: [2000, 5000, 10000]})
      .toBeTruthy();

    const sourceDetails = await getSyncJob(request, failedSource.id);
    const checkpointDir = await checkpointJobDirectory(
      request,
      clusterId,
      String(sourceDetails.platform_job_id),
    );
    await setCheckpointDirectoryMode(checkpointDir, 0o555);
    try {
      const failedJob = await waitForJobStatus(
        request,
        failedSource.id,
        (job) => (job.status ?? '').toLowerCase() === 'failed',
        240000,
      );
      expect((failedJob.status ?? '').toLowerCase()).toBe('failed');
    } finally {
      await setCheckpointDirectoryMode(checkpointDir, 0o755);
    }

    const recoveredJob = await recoverSyncJob(request, failedSource.id);
    const recoveredDetails = await getSyncJob(request, recoveredJob.id);
    assertRecoverSubmit(
      recoveredDetails,
      'checkpoint',
      String(sourceDetails.platform_job_id),
    );
    await waitForJobStatus(
      request,
      recoveredJob.id,
      (job) => (job.status ?? '').toLowerCase() === 'running',
      240000,
    );
    await expect
      .poll(async () => {
        const snapshot = await getJobCheckpointSnapshot(request, recoveredJob.id);
        const checkpointId = Number(
          snapshot.overview?.pipelines?.[0]?.latestCompleted?.checkpointId ?? 0,
        );
        return checkpointId > checkpointBefore;
      }, {timeout: 180000, intervals: [2000, 5000, 10000]})
      .toBeTruthy();
    const received = await readEngineSourceReceivedCount(
      request,
      clusterHttpPort,
      String(recoveredDetails.platform_job_id),
    );
    expect(received).toBeLessThan(200000);
    await expect
      .poll(
        () =>
          findEngineLogLine(
            installDir,
            `${recoveredDetails.platform_job_id}.*restore with CHECKPOINT|restore with CHECKPOINT.*${recoveredDetails.platform_job_id}`,
          ),
        {timeout: 60000, intervals: [2000, 5000]},
      )
      .not.toBe('');

    await cancelSyncJob(request, recoveredJob.id, false);
    await waitForJobStatus(request, recoveredJob.id, (job) => Boolean(job.finished_at), 240000);
  });
});

function assertRecoverSubmit(
  job: {platform_job_id?: string; submit_spec?: Record<string, unknown>},
  mode: 'checkpoint' | 'savepoint' | 'legacy',
  sourcePlatformJobId: string,
) {
  const url = String(job.submit_spec?.engine_url ?? '');
  const platformJobId = String(job.platform_job_id ?? '');
  expect(url).toBeTruthy();
  if (mode === 'legacy') {
    expect(url).toContain('isStartWithSavePoint=true');
    expect(url).not.toContain('restoreMode=');
    expect(platformJobId).toBe(sourcePlatformJobId);
    return;
  }
  expect(url).toContain(
    `restoreMode=${mode === 'checkpoint' ? 'CHECKPOINT' : 'SAVEPOINT'}`,
  );
  expect(url).toContain(`restoreSourceJobId=${sourcePlatformJobId}`);
  expect(url).not.toContain('isStartWithSavePoint=');
  if (mode === 'savepoint') {
    expect(platformJobId).toBe(sourcePlatformJobId);
    return;
  }
  expect(platformJobId).not.toBe(sourcePlatformJobId);
}
