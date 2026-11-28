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
import {expect, test} from '@playwright/test';
import {ConfigType} from '@/lib/services/config';
import {
  assertFileContains,
  buildInstallWizardLabURL,
  chooseSelectOption,
  expectInstallationSuccess,
  getCurrentInstallationId,
  prepareClusterForInstallWizard,
  resolveInstalledConfigPaths,
  ensureClusterRunning,
  waitForOnlineHost,
  waitForStxJavaProxyHealthy,
  type OnlineHostFixture,
} from './helpers/install-wizard-real';
import {
  initClusterConfigsFromNodeApi,
  updateClusterConfig,
  waitForTemplateConfig,
} from './helpers/config-real';
import {inspectCheckpointFile} from './helpers/workbench-real';
import {
  getRuntimeStorageDetailsApi,
  inspectImapWalApi,
  isImapWalFileName,
  listRuntimeStorageApi,
  openClusterRuntimeStorageView,
  previewRuntimeStorageApi,
  readHdfsHaFixture,
  readMinioFixture,
  replaceYamlPluginConfigBlock,
  runStreamingJobForCheckpoint,
  runStxCli,
  setCheckboxChecked,
  syncTemplatesAndRestartCluster,
  waitForEngineRestReady,
  waitForRuntimeStorageMatchingFile,
  type HdfsHaFixture,
  type MinioFixture,
} from './helpers/runtime-storage-real';

const seatunnelVersion = process.env.E2E_INSTALLER_REAL_VERSION ?? '2.3.13';
const backendBaseURL =
  process.env.E2E_BACKEND_BASE_URL ?? 'http://127.0.0.1:18000';
const tmpDirRoot =
  process.env.E2E_INSTALLER_REAL_TMP_DIR ??
  path.resolve(process.cwd(), '../tmp/e2e/runtime-storage-real');
const installDir = path.join(
  tmpDirRoot,
  'install',
  `runtime-storage-seatunnel-${seatunnelVersion}`,
);
const clusterPort = Number(
  process.env.E2E_INSTALLER_REAL_CLUSTER_PORT_PRIMARY || '38181',
);
const httpPort = Number(
  process.env.E2E_INSTALLER_REAL_HTTP_PORT_PRIMARY || '38080',
);
const username = process.env.E2E_USERNAME ?? 'admin';
const password = process.env.E2E_PASSWORD ?? 'admin123';

test.describe.serial('runtime storage real e2e (Local + S3 + HDFS HA)', () => {
  // 限制单动作超时与重试次数，防止缺省动作超时沿用整段测试超时拖慢云端 CI。
  // Bound per-action timeout and retries so a blocked locator fails fast instead of hanging CI.
  test.describe.configure({retries: 0, timeout: 10 * 60 * 1000});
  test.use({actionTimeout: 30_000});

  let host: OnlineHostFixture;
  let clusterId = 0;
  let hdfsHa: HdfsHaFixture;
  let minio: MinioFixture;
  let lastHdfsCkPath = '';
  let lastHdfsJobContent = '';
  let lastLocalCkPath = '';
  let lastLocalImapPath = '';

  test('1. validates and installs HDFS HA (custom namenode aliases) in wizards', async ({
    page,
  }) => {
    hdfsHa = readHdfsHaFixture();
    minio = readMinioFixture();
    host = await waitForOnlineHost(page);
    console.log(
      `[runtime-storage-real] stage 1: online host=${host.name}(${host.id}), hdfs=${hdfsHa.nameServices}(${hdfsHa.haNamenodes})`,
    );

    const cluster = await prepareClusterForInstallWizard(page, {
      hostId: host.id,
      hostName: host.name,
      version: seatunnelVersion,
      installDir,
      clusterPort,
      httpPort,
    });
    clusterId = cluster.clusterId;

    await page.goto(
      buildInstallWizardLabURL({
        hostId: host.id,
        hostName: host.name,
        version: seatunnelVersion,
        installDir,
        clusterPort,
        httpPort,
        clusterId,
      }),
    );

    await page.getByTestId('install-precheck-run').click();
    await expect(page.getByTestId('install-wizard-next')).toBeEnabled({
      timeout: 120000,
    });
    await page.getByTestId('install-wizard-next').click();
    await expect(page.getByTestId('install-wizard-step-config')).toBeVisible();

    await chooseSelectOption(page, 'install-config-mirror', /Apache/i);
    await page.getByTestId('install-config-install-dir').fill(installDir);
    await page.getByTestId('install-jvm-hybrid-heap').fill('1024');
    await page.getByTestId('install-runtime-http-port').fill(String(httpPort));

    // 先验证错误 HDFS HA RPC 地址会被前置校验明确拦下 / Verify invalid HDFS HA RPC fails pre-validation first
    await chooseSelectOption(
      page,
      'install-checkpoint-storage-type',
      /^HDFS$/i,
    );
    await page
      .getByTestId('install-checkpoint-namespace')
      .fill('/seatunnel/checkpoint/');
    await setCheckboxChecked(page, 'install-checkpoint-hdfs-ha-enabled', true);
    await page
      .getByTestId('install-checkpoint-hdfs-nameservices')
      .fill(hdfsHa.nameServices);
    await page
      .getByTestId('install-checkpoint-hdfs-ha-namenodes')
      .fill(hdfsHa.haNamenodes);
    await page.getByTestId('install-checkpoint-hdfs-rpc-1').fill('127.0.0.1:1');
    await page.getByTestId('install-checkpoint-hdfs-rpc-2').fill('127.0.0.1:2');
    await page.getByTestId('install-checkpoint-validate').click();
    const badResult = page.getByTestId('install-checkpoint-validation-result');
    await expect(badResult).toBeVisible({timeout: 120000});
    await expect(badResult).not.toContainText(/校验通过|Validation passed/i);
    console.log(
      '[runtime-storage-real] negative invalid HDFS HA pre-validation rejected as expected',
    );

    // 填入真实 HDFS HA（自定义 nn-alpha,nn-beta）并一键同步到 IMAP / Fill real custom-alias HDFS HA and sync to IMAP
    await page.getByTestId('install-checkpoint-hdfs-rpc-1').fill(hdfsHa.rpc1);
    await page.getByTestId('install-checkpoint-hdfs-rpc-2').fill(hdfsHa.rpc2);
    await page.getByTestId('install-checkpoint-apply-to-imap').click();

    await expect(
      page.getByTestId('install-imap-hdfs-nameservices'),
    ).toHaveValue(hdfsHa.nameServices);
    await expect(
      page.getByTestId('install-imap-hdfs-ha-namenodes'),
    ).toHaveValue(hdfsHa.haNamenodes);
    await expect(page.getByTestId('install-imap-hdfs-rpc-1')).toHaveValue(
      hdfsHa.rpc1,
    );
    await expect(page.getByTestId('install-imap-hdfs-rpc-2')).toHaveValue(
      hdfsHa.rpc2,
    );

    await page.getByTestId('install-checkpoint-validate').click();
    await expect(
      page
        .getByTestId('install-checkpoint-validation-result')
        .getByText(/校验通过|Validation passed/i),
    ).toBeVisible({timeout: 120000});
    console.log('[runtime-storage-real] install wizard HDFS HA checkpoint validation passed');

    await page.getByTestId('install-imap-validate').click();
    await expect(
      page
        .getByTestId('install-imap-validation-result')
        .getByText(/校验通过|Validation passed/i),
    ).toBeVisible({timeout: 120000});
    console.log('[runtime-storage-real] install wizard HDFS HA IMAP validation passed');

    await page.getByTestId('install-wizard-next').click();
    await expect(page.getByTestId('install-wizard-step-plugins')).toBeVisible();
    const previousInstallationId = await getCurrentInstallationId(
      page,
      host.id,
    );
    await page.getByTestId('install-wizard-next').click();
    await expectInstallationSuccess(page, host.id, previousInstallationId);
    await ensureClusterRunning(page, clusterId, 240000);
    await waitForStxJavaProxyHealthy(page, clusterId, 240000);
    await waitForEngineRestReady(
      page.context().request,
      host.ipAddress,
      httpPort,
      installDir,
    );
    console.log('[runtime-storage-real] HDFS HA cluster installed and running');

    const files = resolveInstalledConfigPaths(installDir);
    await assertFileContains(files.seatunnel, [
      'enable-http: true',
      `port: ${httpPort}`,
      'storage.type: hdfs',
      'namespace: /seatunnel/checkpoint/',
      `fs.defaultFS: hdfs://${hdfsHa.nameServices}`,
      `seatunnel.hadoop.dfs.nameservices: ${hdfsHa.nameServices}`,
      `seatunnel.hadoop.dfs.ha.namenodes.${hdfsHa.nameServices}: ${hdfsHa.haNamenodes}`,
      `seatunnel.hadoop.dfs.namenode.rpc-address.${hdfsHa.nameServices}.nn-alpha: ${hdfsHa.rpc1}`,
      `seatunnel.hadoop.dfs.namenode.rpc-address.${hdfsHa.nameServices}.nn-beta: ${hdfsHa.rpc2}`,
    ]);
    await assertFileContains(files.hazelcast, [
      'enabled: true',
      'engine_checkpoint_monitor:',
      'enabled: false',
      'storage.type: hdfs',
      'namespace: /seatunnel/imap/',
      `seatunnel.hadoop.dfs.ha.namenodes.${hdfsHa.nameServices}: ${hdfsHa.haNamenodes}`,
      `seatunnel.hadoop.dfs.namenode.rpc-address.${hdfsHa.nameServices}.nn-alpha: ${hdfsHa.rpc1}`,
      `seatunnel.hadoop.dfs.namenode.rpc-address.${hdfsHa.nameServices}.nn-beta: ${hdfsHa.rpc2}`,
    ]);

    // 从节点初始化集群配置模板，供后续 RuntimeStorageEditor 保存写回 / Initialize cluster config templates from node
    await initClusterConfigsFromNodeApi(
      page.context().request,
      clusterId,
      host.id,
      installDir,
    );

    // 验证集群部署向导（ClusterConfigStep）的 HDFS HA 字段与 CK→IMAP 同步 / Verify ClusterDeployWizard ClusterConfigStep UI sync
    await page.goto('/clusters');
    const deployBtn = page
      .getByRole('button', {
        name: /创建集群|部署集群|Create Cluster|Deploy Cluster/i,
      })
      .first();
    await expect(deployBtn).toBeVisible({timeout: 60000});
    await deployBtn.click();

    const dialog = page.getByRole('dialog').last();
    await expect(dialog).toBeVisible({timeout: 30000});
    const nameInput = dialog.locator('input').first();
    await nameInput.fill(`wizard-hdfs-check-${Date.now()}`);
    await dialog
      .getByRole('button', {name: /下一步|Next/i})
      .first()
      .click();
    // 选中在线主机并在分离模式下补勾 Master 角色，避免「下一步」按钮保持禁用。
    // Select the online host and check Master in separated mode so the Next button becomes enabled.
    const hostRow = dialog.getByText(host.name).first();
    await expect(hostRow).toBeVisible({timeout: 30000});
    await hostRow.click();
    const masterRoleCheckbox = dialog
      .getByRole('checkbox', {name: /^Master$/i})
      .first();
    if (
      (await masterRoleCheckbox.count()) > 0 &&
      (await masterRoleCheckbox.isVisible())
    ) {
      const checked =
        (await masterRoleCheckbox.getAttribute('aria-checked')) === 'true' ||
        (await masterRoleCheckbox.getAttribute('data-state')) === 'checked';
      if (!checked) {
        await masterRoleCheckbox.click();
      }
    }
    const hostsNextBtn = dialog
      .getByRole('button', {name: /下一步|Next/i})
      .first();
    await expect(hostsNextBtn).toBeEnabled({timeout: 30000});
    await hostsNextBtn.click();

    // ClusterConfigStep 使用 Arc SegmentedControl（button[data-value="HDFS"]）。
    // ClusterConfigStep uses Arc SegmentedControl (button[data-value="HDFS"]).
    const ckHdfsSegment = dialog.locator('button[data-value="HDFS"]').first();
    await expect(ckHdfsSegment).toBeVisible({timeout: 30000});
    await ckHdfsSegment.click();
    await setCheckboxChecked(page, 'cluster-wizard-checkpoint-hdfs-ha', true);
    await page
      .getByTestId('cluster-wizard-checkpoint-hdfs-nameservices')
      .fill(hdfsHa.nameServices);
    await page
      .getByTestId('cluster-wizard-checkpoint-hdfs-ha-namenodes')
      .fill(hdfsHa.haNamenodes);
    await page
      .getByTestId('cluster-wizard-checkpoint-hdfs-rpc-1')
      .fill(hdfsHa.rpc1);
    await page
      .getByTestId('cluster-wizard-checkpoint-hdfs-rpc-2')
      .fill(hdfsHa.rpc2);
    await page
      .getByTestId('cluster-wizard-checkpoint-hdfs-site-path')
      .fill(hdfsHa.siteXmlPath);

    const imapToggle = dialog.locator('#imap-external-toggle');
    await expect(imapToggle).toBeVisible({timeout: 30000});
    await imapToggle.click();
    await page.getByTestId('cluster-wizard-apply-checkpoint-to-imap').click();

    await expect(
      page.getByTestId('cluster-wizard-imap-hdfs-ha-namenodes'),
    ).toHaveValue(hdfsHa.haNamenodes);
    await expect(
      page.getByTestId('cluster-wizard-imap-hdfs-site-path'),
    ).toHaveValue(hdfsHa.siteXmlPath);
    console.log(
      '[runtime-storage-real] cluster deploy wizard ClusterConfigStep HDFS HA sync verified',
    );
  });

  test('2. runs streaming job on HDFS HA and verifies CK + IMAP validate, browse, preview, and inspect', async ({
    page,
  }) => {
    const request = page.context().request;
    console.log('[runtime-storage-real] stage 2: running streaming job on HDFS HA');

    const job = await runStreamingJobForCheckpoint(
      request,
      clusterId,
      seatunnelVersion,
      'rt-hdfs-ha',
    );
    lastHdfsJobContent = job.content;
    console.log(
      `[runtime-storage-real] HDFS HA streaming job completed checkpoint + savepoint: jobId=${job.jobId}, platformJobId=${job.platformJobId}`,
    );

    // 验证集群详情 → 运行时存储 Checkpoint UI / Verify ClusterDetail Runtime Storage Checkpoint UI
    await openClusterRuntimeStorageView(page, clusterId, 'checkpoint');
    await expect(
      page.getByTestId('cluster-runtime-storage-checkpoint-mode'),
    ).toContainText(/HDFS/i);
    await page.getByTestId('cluster-runtime-storage-checkpoint-validate').click();
    await expect(
      page.getByTestId('cluster-runtime-storage-checkpoint-validate-result'),
    ).toContainText(/通过|Passed/i, {timeout: 120000});

    const details = await getRuntimeStorageDetailsApi(request, clusterId);
    expect(details.checkpoint?.storage_type?.toUpperCase()).toBe('HDFS');
    expect(details.checkpoint?.hdfs_ha_enabled).toBe(true);
    expect(details.checkpoint?.hdfs_name_services).toBe(hdfsHa.nameServices);
    expect(details.checkpoint?.hdfs_ha_namenodes).toBe(hdfsHa.haNamenodes);
    expect(details.checkpoint?.size_available).toBe(true);
    expect(details.checkpoint?.total_size_bytes ?? 0).toBeGreaterThan(0);

    const ckFile = await waitForRuntimeStorageMatchingFile(
      request,
      clusterId,
      'checkpoint',
      '/seatunnel/checkpoint/',
      (item) =>
        Boolean(item.path?.includes(job.platformJobId)) &&
        Boolean(item.name?.endsWith('.ser')),
    );
    lastHdfsCkPath = ckFile.path!;
    console.log(`[runtime-storage-real] located HDFS HA checkpoint file: ${lastHdfsCkPath}`);

    const ckPreview = await previewRuntimeStorageApi(
      request,
      clusterId,
      'checkpoint',
      lastHdfsCkPath,
    );
    expect(ckPreview.size_bytes ?? 0).toBeGreaterThan(0);

    const ckInspect = await inspectCheckpointFile(
      request,
      clusterId,
      lastHdfsCkPath,
      lastHdfsJobContent,
    );
    expect(ckInspect.completed_checkpoint).toBeTruthy();

    // 在 UI 文件浏览器中展开作业目录并点击反序列化弹窗 / Navigate into job dir in UI explorer and open inspect dialog
    const explorer = page.getByTestId('runtime-storage-explorer-checkpoint');
    await page.getByTestId('runtime-storage-explorer-checkpoint-refresh').click();
    const jobDirRow = explorer
      .locator(
        `[data-testid="runtime-storage-explorer-checkpoint-row"][data-item-name="${job.platformJobId}"]`,
      )
      .first();
    await expect(jobDirRow).toBeVisible({timeout: 60000});
    await jobDirRow.click();

    const inspectBtn = explorer
      .getByTestId('runtime-storage-explorer-inspect-checkpoint')
      .first();
    await expect(inspectBtn).toBeVisible({timeout: 60000});
    await inspectBtn.click();
    await expect(page.getByRole('dialog').last()).toBeVisible({timeout: 60000});
    await page.keyboard.press('Escape');

    // 验证集群详情 → 运行时存储 IMAP UI + WAL 反序列化 / Verify Runtime Storage IMAP UI + WAL inspect
    await openClusterRuntimeStorageView(page, clusterId, 'imap');
    await expect(
      page.getByTestId('cluster-runtime-storage-imap-mode'),
    ).toContainText(/HDFS/i);
    await expect(
      page.getByTestId('cluster-runtime-storage-imap-cleanup'),
    ).toHaveCount(0);
    await page.getByTestId('cluster-runtime-storage-imap-validate').click();
    await expect(
      page.getByTestId('cluster-runtime-storage-imap-validate-result'),
    ).toContainText(/通过|Passed/i, {timeout: 120000});

    const imapWalFile = await waitForRuntimeStorageMatchingFile(
      request,
      clusterId,
      'imap',
      '/seatunnel/imap/',
      (item) =>
        isImapWalFileName(item.name) && (item.size_bytes ?? 0) > 0,
    );
    console.log(`[runtime-storage-real] located HDFS HA IMAP WAL file: ${imapWalFile.path}`);
    const imapInspect = await inspectImapWalApi(
      request,
      clusterId,
      imapWalFile.path!,
    );
    expect(imapInspect.entry_count ?? 0).toBeGreaterThan(0);
    console.log(
      `[runtime-storage-real] HDFS HA IMAP WAL inspected: entries=${imapInspect.entry_count}`,
    );
  });

  test('3. verifies hdfs_site_path mode, UI hot-switch to S3 MinIO, and UI hot-switch to Local + IMAP cleanup', async ({
    page,
  }) => {
    const request = page.context().request;

    // 3A: 配置页直改 seatunnel.yaml 为纯 hdfs_site_path 承接 HA 拓扑（无内联 seatunnel.hadoop.*）
    // 3A: Edit seatunnel.yaml to load HA topology purely from hdfs_site_path (no inline seatunnel.hadoop.* keys)
    console.log(
      '[runtime-storage-real] stage 3A: switching seatunnel.yaml to hdfs_site_path-only HA mode',
    );
    const seatunnelTemplate = await waitForTemplateConfig(
      request,
      clusterId,
      ConfigType.SEATUNNEL,
      () => true,
    );
    // SeaTunnel 2.3.13 HdfsStorage 缺省 storage.type 会落入 LocalConfiguration 且忽略 hdfs_site_path，
    // HdfsConfiguration 启动需要 storage.type: hdfs 与 fs.defaultFS，HA 拓扑全部由 hdfs_site_path 提供。
    // SeaTunnel 2.3.13 HdfsStorage defaults omitted storage.type to LocalConfiguration (which ignores hdfs_site_path);
    // HdfsConfiguration requires storage.type: hdfs + fs.defaultFS while loading all HA topology from hdfs_site_path.
    const siteOnlyContent = replaceYamlPluginConfigBlock(
      seatunnelTemplate.content,
      [
        'storage.type: hdfs',
        'namespace: /seatunnel/checkpoint/',
        `fs.defaultFS: hdfs://${hdfsHa.nameServices}`,
        `hdfs_site_path: ${hdfsHa.siteXmlPath}`,
        'disable.cache: "true"',
      ],
    );
    expect(siteOnlyContent).toContain('storage.type: hdfs');
    expect(siteOnlyContent).toContain(`fs.defaultFS: hdfs://${hdfsHa.nameServices}`);
    expect(siteOnlyContent).toContain(`hdfs_site_path: ${hdfsHa.siteXmlPath}`);
    expect(siteOnlyContent).toContain('enable-http: true');
    expect(siteOnlyContent).not.toContain('seatunnel.hadoop.dfs.nameservices');

    await updateClusterConfig(
      request,
      seatunnelTemplate.id,
      siteOnlyContent,
      'switch checkpoint to hdfs_site_path-only HA mode',
    );
    await syncTemplatesAndRestartCluster(
      page,
      clusterId,
      [ConfigType.SEATUNNEL],
      {hostIP: host.ipAddress, httpPort, installDir},
    );

    const siteDetails = await getRuntimeStorageDetailsApi(request, clusterId);
    expect(siteDetails.checkpoint?.storage_type?.toUpperCase()).toBe('HDFS');
    expect(siteDetails.checkpoint?.hdfs_site_path).toBe(hdfsHa.siteXmlPath);
    expect(siteDetails.checkpoint?.disable_cache).toBe(true);

    await openClusterRuntimeStorageView(page, clusterId, 'checkpoint');
    await page.getByTestId('cluster-runtime-storage-checkpoint-validate').click();
    await expect(
      page.getByTestId('cluster-runtime-storage-checkpoint-validate-result'),
    ).toContainText(/通过|Passed/i, {timeout: 120000});

    const siteListed = await listRuntimeStorageApi(
      request,
      clusterId,
      'checkpoint',
      '/seatunnel/checkpoint/',
      true,
    );
    expect(
      siteListed.items.some((item) => item.name?.endsWith('.ser')),
      'hdfs_site_path mode must list checkpoint files via stx-java-proxy',
    ).toBeTruthy();
    const siteInspected = await inspectCheckpointFile(
      request,
      clusterId,
      lastHdfsCkPath,
      lastHdfsJobContent,
    );
    expect(siteInspected.completed_checkpoint).toBeTruthy();
    console.log(
      '[runtime-storage-real] stage 3A passed: hdfs_site_path validate/list/inspect all succeeded',
    );

    // 3B: 在 RuntimeStorageEditor UI 中切换到 S3 (MinIO) / 3B: UI hot-switch to S3 MinIO
    console.log('[runtime-storage-real] stage 3B: switching CK & IMAP to MinIO S3 via UI');
    await openClusterRuntimeStorageView(page, clusterId, 'checkpoint');
    await chooseSelectOption(page, 'runtime-storage-checkpoint-storage-type', /^S3$/i);
    await page
      .getByTestId('runtime-storage-checkpoint-namespace')
      .fill('/seatunnel/checkpoint/');
    await page
      .getByTestId('runtime-storage-checkpoint-s3-bucket')
      .fill(minio.checkpointBucket);
    await page
      .getByTestId('runtime-storage-checkpoint-s3-endpoint')
      .fill(minio.endpoint);
    await page
      .getByTestId('runtime-storage-checkpoint-s3-access-key')
      .fill(minio.accessKey);
    await page
      .getByTestId('runtime-storage-checkpoint-s3-secret-key')
      .fill(minio.secretKey);
    await page.getByTestId('runtime-storage-checkpoint-save').click();
    await expect(
      page.getByTestId('runtime-storage-checkpoint-notice'),
    ).toBeVisible({timeout: 120000});

    // 验证敏感密钥不回显明文 / Verify secrets stay blank after save
    await expect(
      page.getByTestId('runtime-storage-checkpoint-s3-access-key'),
    ).toHaveValue('');
    await expect(
      page.getByTestId('runtime-storage-checkpoint-s3-secret-key'),
    ).toHaveValue('');

    await openClusterRuntimeStorageView(page, clusterId, 'imap');
    await chooseSelectOption(page, 'runtime-storage-imap-storage-type', /^S3$/i);
    await page
      .getByTestId('runtime-storage-imap-namespace')
      .fill('/seatunnel/imap/');
    await page
      .getByTestId('runtime-storage-imap-s3-bucket')
      .fill(minio.imapBucket);
    await page
      .getByTestId('runtime-storage-imap-s3-endpoint')
      .fill(minio.endpoint);
    await page
      .getByTestId('runtime-storage-imap-s3-access-key')
      .fill(minio.accessKey);
    await page
      .getByTestId('runtime-storage-imap-s3-secret-key')
      .fill(minio.secretKey);
    await page.getByTestId('runtime-storage-imap-save').click();
    await expect(page.getByTestId('runtime-storage-imap-notice')).toBeVisible({
      timeout: 120000,
    });

    await syncTemplatesAndRestartCluster(
      page,
      clusterId,
      [ConfigType.SEATUNNEL, ConfigType.HAZELCAST],
      {hostIP: host.ipAddress, httpPort, installDir},
    );

    const s3Job = await runStreamingJobForCheckpoint(
      request,
      clusterId,
      seatunnelVersion,
      'rt-s3-minio',
    );
    const s3CkFile = await waitForRuntimeStorageMatchingFile(
      request,
      clusterId,
      'checkpoint',
      '/seatunnel/checkpoint/',
      (item) =>
        Boolean(item.path?.includes(s3Job.platformJobId)) &&
        Boolean(item.name?.endsWith('.ser')),
    );
    const s3CkInspect = await inspectCheckpointFile(
      request,
      clusterId,
      s3CkFile.path!,
      s3Job.content,
    );
    expect(s3CkInspect.completed_checkpoint).toBeTruthy();

    const s3ImapWal = await waitForRuntimeStorageMatchingFile(
      request,
      clusterId,
      'imap',
      '/seatunnel/imap/',
      (item) =>
        isImapWalFileName(item.name) && (item.size_bytes ?? 0) > 0,
    );
    const s3ImapInspect = await inspectImapWalApi(
      request,
      clusterId,
      s3ImapWal.path!,
    );
    expect(s3ImapInspect.entry_count ?? 0).toBeGreaterThan(0);

    // 远端 S3 拒绝 IMAP 本地清理 / Remote S3 rejects local IMAP cleanup
    const s3CleanupResp = await request.post(
      `${backendBaseURL}/api/v1/clusters/${clusterId}/runtime-storage/imap/cleanup`,
    );
    expect(s3CleanupResp.ok()).toBeFalsy();
    console.log(
      `[runtime-storage-real] stage 3B passed: MinIO S3 CK & IMAP WAL inspected (imap entries=${s3ImapInspect.entry_count})`,
    );

    // 3C: 在 RuntimeStorageEditor UI 中切换到 Local 并验证本地 IMAP 清理 / 3C: UI switch to LOCAL_FILE + IMAP cleanup
    const localCkDir = path.join(tmpDirRoot, 'local-runtime-ck') + '/';
    const localImapDir = path.join(tmpDirRoot, 'local-runtime-imap') + '/';
    await fs.mkdir(localCkDir, {recursive: true});
    await fs.mkdir(localImapDir, {recursive: true});
    console.log(
      `[runtime-storage-real] stage 3C: switching CK & IMAP to LOCAL_FILE (${localCkDir}, ${localImapDir})`,
    );

    await openClusterRuntimeStorageView(page, clusterId, 'checkpoint');
    await chooseSelectOption(
      page,
      'runtime-storage-checkpoint-storage-type',
      /^LOCAL_FILE$/i,
    );
    await page
      .getByTestId('runtime-storage-checkpoint-namespace')
      .fill(localCkDir);
    await page.getByTestId('runtime-storage-checkpoint-save').click();
    await expect(
      page.getByTestId('runtime-storage-checkpoint-notice'),
    ).toBeVisible({timeout: 120000});

    await openClusterRuntimeStorageView(page, clusterId, 'imap');
    await chooseSelectOption(
      page,
      'runtime-storage-imap-storage-type',
      /^LOCAL_FILE$/i,
    );
    await page
      .getByTestId('runtime-storage-imap-namespace')
      .fill(localImapDir);
    await page.getByTestId('runtime-storage-imap-save').click();
    await expect(page.getByTestId('runtime-storage-imap-notice')).toBeVisible({
      timeout: 120000,
    });

    await syncTemplatesAndRestartCluster(
      page,
      clusterId,
      [ConfigType.SEATUNNEL, ConfigType.HAZELCAST],
      {hostIP: host.ipAddress, httpPort, installDir},
    );

    const localJob = await runStreamingJobForCheckpoint(
      request,
      clusterId,
      seatunnelVersion,
      'rt-local',
    );
    const localCkFile = await waitForRuntimeStorageMatchingFile(
      request,
      clusterId,
      'checkpoint',
      localCkDir,
      (item) =>
        Boolean(item.path?.includes(localJob.platformJobId)) &&
        Boolean(item.name?.endsWith('.ser')),
    );
    lastLocalCkPath = localCkFile.path!;
    const localCkInspect = await inspectCheckpointFile(
      request,
      clusterId,
      lastLocalCkPath,
      localJob.content,
    );
    expect(localCkInspect.completed_checkpoint).toBeTruthy();

    const localImapWal = await waitForRuntimeStorageMatchingFile(
      request,
      clusterId,
      'imap',
      localImapDir,
      (item) =>
        isImapWalFileName(item.name) && (item.size_bytes ?? 0) > 0,
    );
    lastLocalImapPath = localImapWal.path!;
    const localImapInspect = await inspectImapWalApi(
      request,
      clusterId,
      lastLocalImapPath,
    );
    expect(localImapInspect.entry_count ?? 0).toBeGreaterThan(0);

    console.log('[runtime-storage-real] stage 3C local CK & IMAP WAL verified');
  });

  test('4. verifies packaged dist/stx CLI runtime-storage commands and local IMAP UI cleanup', async ({
    page,
  }) => {
    const cliConfigHome = path.join(tmpDirRoot, 'stx-cli-runtime-storage');
    await fs.mkdir(cliConfigHome, {recursive: true});
    console.log('[runtime-storage-real] stage 4: verifying dist/stx CLI commands');

    await runStxCli(
      cliConfigHome,
      [
        'login',
        '--server',
        backendBaseURL,
        '--username',
        username,
        '--password-stdin',
        '--output',
        'json',
      ],
      {stdin: `${password}\n`},
    );

    // 安装前校验：分别测 HDFS HA（自定义别名）与 S3 MinIO，严格按注册 CLI 的 --request-file 契约传参
    // Pre-install CLI validate for HDFS HA & S3 using the registered --request-file contract
    const hdfsValidateFile = path.join(cliConfigHome, 'hdfs-ha-ck.json');
    await fs.writeFile(
      hdfsValidateFile,
      JSON.stringify({
        host_ids: [host.id],
        kind: 'checkpoint',
        checkpoint: {
          storage_type: 'HDFS',
          namespace: '/seatunnel/checkpoint/',
          hdfs_ha_enabled: true,
          hdfs_name_services: hdfsHa.nameServices,
          hdfs_ha_namenodes: hdfsHa.haNamenodes,
          hdfs_namenode_rpc_address_1: hdfsHa.rpc1,
          hdfs_namenode_rpc_address_2: hdfsHa.rpc2,
          hdfs_failover_proxy_provider: hdfsHa.failoverProvider,
        },
      }),
      'utf8',
    );
    const cliHdfsValidate = await runStxCli<{success?: boolean}>(
      cliConfigHome,
      [
        'installer',
        'runtime-storage',
        'validate',
        '--request-file',
        hdfsValidateFile,
        '--output',
        'json',
      ],
    );
    expect(cliHdfsValidate.result?.data?.success).toBe(true);

    const s3ValidateFile = path.join(cliConfigHome, 's3-imap.json');
    await fs.writeFile(
      s3ValidateFile,
      JSON.stringify({
        host_ids: [host.id],
        kind: 'imap',
        imap: {
          storage_type: 'S3',
          namespace: '/seatunnel/imap/',
          storage_endpoint: minio.endpoint,
          storage_bucket: minio.imapBucket,
          storage_access_key: minio.accessKey,
          storage_secret_key: minio.secretKey,
        },
      }),
      'utf8',
    );
    const cliS3Validate = await runStxCli<{success?: boolean}>(
      cliConfigHome,
      [
        'installer',
        'runtime-storage',
        'validate',
        '--request-file',
        s3ValidateFile,
        '--output',
        'json',
      ],
    );
    expect(cliS3Validate.result?.data?.success).toBe(true);

    const cliGet = await runStxCli<{checkpoint?: {storage_type?: string}}>(
      cliConfigHome,
      ['cluster', 'runtime-storage', 'get', String(clusterId), '--output', 'json'],
    );
    expect(cliGet.result?.data?.checkpoint?.storage_type?.toUpperCase()).toBe(
      'LOCAL_FILE',
    );

    const cliValidateCk = await runStxCli<{success?: boolean}>(cliConfigHome, [
      'cluster',
      'runtime-storage',
      'validate',
      String(clusterId),
      'checkpoint',
      '--output',
      'json',
    ]);
    expect(cliValidateCk.result?.data?.success).toBe(true);

    const listReqFile = path.join(cliConfigHome, 'list-ck.json');
    await fs.writeFile(
      listReqFile,
      JSON.stringify({recursive: true, limit: 200}),
      'utf8',
    );
    const cliListCk = await runStxCli<{items?: Array<{path?: string}>}>(
      cliConfigHome,
      [
        'cluster',
        'runtime-storage',
        'list',
        String(clusterId),
        'checkpoint',
        '--request-file',
        listReqFile,
        '--output',
        'json',
      ],
    );
    expect((cliListCk.result?.data?.items ?? []).length).toBeGreaterThan(0);

    const previewReqFile = path.join(cliConfigHome, 'preview-ck.json');
    await fs.writeFile(
      previewReqFile,
      JSON.stringify({path: lastLocalCkPath, max_bytes: 65536}),
      'utf8',
    );
    const cliPreviewCk = await runStxCli<{size_bytes?: number}>(cliConfigHome, [
      'cluster',
      'runtime-storage',
      'preview',
      String(clusterId),
      'checkpoint',
      '--request-file',
      previewReqFile,
      '--output',
      'json',
    ]);
    expect(cliPreviewCk.result?.data?.size_bytes ?? 0).toBeGreaterThan(0);

    const inspectCkReqFile = path.join(cliConfigHome, 'inspect-ck.json');
    await fs.writeFile(
      inspectCkReqFile,
      JSON.stringify({path: lastLocalCkPath}),
      'utf8',
    );
    const cliInspectCk = await runStxCli<{
      completed_checkpoint?: Record<string, unknown>;
    }>(cliConfigHome, [
      'cluster',
      'runtime-storage',
      'checkpoint',
      'inspect',
      String(clusterId),
      '--request-file',
      inspectCkReqFile,
      '--output',
      'json',
    ]);
    expect(cliInspectCk.result?.data?.completed_checkpoint).toBeTruthy();

    const inspectImapReqFile = path.join(cliConfigHome, 'inspect-imap.json');
    await fs.writeFile(
      inspectImapReqFile,
      JSON.stringify({path: lastLocalImapPath}),
      'utf8',
    );
    const cliInspectImap = await runStxCli<{entry_count?: number}>(
      cliConfigHome,
      [
        'cluster',
        'runtime-storage',
        'imap',
        'inspect',
        String(clusterId),
        '--request-file',
        inspectImapReqFile,
        '--output',
        'json',
      ],
    );
    expect(cliInspectImap.result?.data?.entry_count ?? 0).toBeGreaterThan(0);
    console.log('[runtime-storage-real] stage 4 CLI commands all passed');

    // 通过 UI 按钮执行本地 IMAP 清理 / Trigger local IMAP cleanup via UI confirm dialog
    await openClusterRuntimeStorageView(page, clusterId, 'imap');
    const cleanupBtn = page.getByTestId('cluster-runtime-storage-imap-cleanup');
    await expect(cleanupBtn).toBeVisible({timeout: 30000});
    await cleanupBtn.click();
    const confirmDialog = page.getByRole('alertdialog').last();
    await expect(confirmDialog).toBeVisible({timeout: 15000});
    await confirmDialog
      .getByRole('button', {name: /清理|Clean/i})
      .last()
      .click();
    await expect(confirmDialog).toBeHidden({timeout: 60000});
    console.log('[runtime-storage-real] local IMAP UI cleanup completed');
  });

  test('5. enforces negative guardrails without corrupting saved config versions', async ({
    page,
  }) => {
    const request = page.context().request;
    console.log('[runtime-storage-real] stage 5: negative guardrails');

    const beforeTemplate = await waitForTemplateConfig(
      request,
      clusterId,
      ConfigType.SEATUNNEL,
      () => true,
    );

    // 错误 S3 凭证在 RuntimeStorageEditor 中必须保存失败且不产生新配置版本 / Invalid S3 credentials must fail without bumping template version
    await openClusterRuntimeStorageView(page, clusterId, 'checkpoint');
    await chooseSelectOption(
      page,
      'runtime-storage-checkpoint-storage-type',
      /^S3$/i,
    );
    await page
      .getByTestId('runtime-storage-checkpoint-namespace')
      .fill('/seatunnel/checkpoint/');
    await page
      .getByTestId('runtime-storage-checkpoint-s3-bucket')
      .fill(minio.checkpointBucket);
    await page
      .getByTestId('runtime-storage-checkpoint-s3-endpoint')
      .fill(minio.endpoint);
    await page
      .getByTestId('runtime-storage-checkpoint-s3-access-key')
      .fill('wrong-ak');
    await page
      .getByTestId('runtime-storage-checkpoint-s3-secret-key')
      .fill('wrong-sk');
    await page.getByTestId('runtime-storage-checkpoint-save').click();
    await expect(
      page.getByTestId('runtime-storage-checkpoint-error'),
    ).toBeVisible({timeout: 120000});

    const afterTemplate = await waitForTemplateConfig(
      request,
      clusterId,
      ConfigType.SEATUNNEL,
      () => true,
    );
    expect(afterTemplate.version).toBe(beforeTemplate.version);

    // 非法 HDFS HA 别名数量（3 个别名配 2 个 RPC）必须报错拒绝 / Mismatched HA namenode count must be rejected
    const invalidHaResp = await request.post(
      `${backendBaseURL}/api/v1/installer/runtime-storage/validate`,
      {
        data: {
          host_ids: [host.id],
          kind: 'checkpoint',
          checkpoint: {
            storage_type: 'HDFS',
            namespace: '/seatunnel/checkpoint/',
            hdfs_ha_enabled: true,
            hdfs_name_services: hdfsHa.nameServices,
            hdfs_ha_namenodes: 'nn-alpha,nn-beta,nn-gamma',
            hdfs_namenode_rpc_address_1: hdfsHa.rpc1,
            hdfs_namenode_rpc_address_2: hdfsHa.rpc2,
          },
        },
      },
    );
    const invalidHaBody = (await invalidHaResp.json()) as {
      error_msg?: string;
      data?: {success?: boolean};
    };
    const rejected =
      !invalidHaResp.ok() ||
      Boolean(invalidHaBody.error_msg) ||
      invalidHaBody.data?.success === false;
    expect(
      rejected,
      `expected 3-namenode HA config to fail: ${JSON.stringify(invalidHaBody)}`,
    ).toBeTruthy();
    console.log('[runtime-storage-real] stage 5 negative guardrails passed');
  });
});
