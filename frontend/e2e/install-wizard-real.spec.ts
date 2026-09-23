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
import {expect, test} from '@playwright/test';
import {
  assertFileContains,
  buildInstallWizardLabURL,
  chooseSelectOption,
  expectStxJavaProxyProbeSuccess,
  expectInstallationSuccess,
  prepareClusterForInstallWizard,
  resolveInstalledConfigPaths,
  waitForOnlineHost,
  waitForStxJavaProxyHealthy,
} from './helpers/install-wizard-real';

const seatunnelVersion = process.env.E2E_INSTALLER_REAL_VERSION ?? '2.3.13';
const installDirRoot =
  process.env.E2E_INSTALLER_REAL_INSTALL_DIR ??
  path.resolve(
    process.cwd(),
    '../tmp/e2e/installer-real/install/seatunnel-2.3.13',
  );
const minioEndpoint =
  process.env.E2E_INSTALLER_REAL_MINIO_ENDPOINT ?? 'http://127.0.0.1:19000';
const minioAccessKey =
  process.env.E2E_INSTALLER_REAL_MINIO_ACCESS_KEY ?? 'minioadmin';
const minioSecretKey =
  process.env.E2E_INSTALLER_REAL_MINIO_SECRET_KEY ?? 'minioadmin';
const checkpointBucket =
  process.env.E2E_INSTALLER_REAL_CHECKPOINT_BUCKET ??
  's3a://seatunnel-checkpoint';
const imapBucket =
  process.env.E2E_INSTALLER_REAL_IMAP_BUCKET ?? 's3a://seatunnel-imap';
const clusterPortPrimary = Number(
  process.env.E2E_INSTALLER_REAL_CLUSTER_PORT_PRIMARY || '38181',
);
const clusterPortSecondary = Number(
  process.env.E2E_INSTALLER_REAL_CLUSTER_PORT_SECONDARY || '38182',
);
const httpPortPrimary = Number(
  process.env.E2E_INSTALLER_REAL_HTTP_PORT_PRIMARY || '38080',
);
const httpPortSecondary = Number(
  process.env.E2E_INSTALLER_REAL_HTTP_PORT_SECONDARY || '38081',
);

test.describe.serial('install wizard real installer', () => {
  test('installs a target version online and writes local checkpoint config', async ({
    page,
  }) => {
    console.log('[installer-real] starting local checkpoint scenario');
    const host = await waitForOnlineHost(page);
    const installDir = `${installDirRoot}-local`;
    const clusterPort = clusterPortPrimary;
    const httpPort = httpPortPrimary;
    const cluster = await prepareClusterForInstallWizard(page, {
      hostId: host.id,
      hostName: host.name,
      version: seatunnelVersion,
      installDir,
      clusterPort,
      httpPort,
    });

    await page.goto(
      buildInstallWizardLabURL({
        hostId: host.id,
        hostName: host.name,
        version: seatunnelVersion,
        installDir,
        clusterPort,
        httpPort,
        clusterId: cluster.clusterId,
      }),
    );

    await page.getByTestId('install-precheck-run').click();
    await expect(page.getByTestId('install-wizard-next')).toBeEnabled({
      timeout: 120000,
    });
    console.log(
      '[installer-real] precheck passed for local checkpoint scenario',
    );

    await page.getByTestId('install-wizard-next').click();
    await expect(page.getByTestId('install-wizard-step-config')).toBeVisible();

    await chooseSelectOption(page, 'install-config-mirror', /Apache/i);
    await page.getByTestId('install-config-install-dir').fill(installDir);
    await page.getByTestId('install-jvm-hybrid-heap').fill('1');
    await page.getByTestId('install-runtime-http-port').fill(String(httpPort));
    await chooseSelectOption(
      page,
      'install-runtime-log-mode',
      /单 Job 日志|Per-job logs/i,
    );

    await page.getByTestId('install-wizard-next').click();
    await expect(page.getByTestId('install-wizard-step-plugins')).toBeVisible();
    console.log(
      '[installer-real] config step completed for local checkpoint scenario',
    );
    await page.getByTestId('install-wizard-next').click();

    await expectInstallationSuccess(page, host.id);
    console.log(
      '[installer-real] installation succeeded for local checkpoint scenario',
    );

    const files = resolveInstalledConfigPaths(installDir);
    await assertFileContains(files.seatunnel, [
      'enable-http: true',
      `port: ${httpPort}`,
      'namespace: /tmp/seatunnel/checkpoint/',
      'fs.defaultFS: file:///',
    ]);
    await assertFileContains(files.hazelcast, ['enabled: false']);
    await assertFileContains(files.hazelcastClient, [
      'cluster-members:',
      `- 127.0.0.1:${clusterPort}`,
    ]);
    await assertFileContains(files.log4j2, [
      'rootLogger.appenderRef.file.ref = routingAppender',
    ]);
  });

  test('installs with MinIO-backed S3 checkpoint and IMAP configuration', async ({
    page,
  }) => {
    console.log(
      '[installer-real] starting MinIO-backed checkpoint/imap scenario',
    );
    const host = await waitForOnlineHost(page);
    const installDir = `${installDirRoot}-s3`;
    const clusterPort = clusterPortSecondary;
    const httpPort = httpPortSecondary;
    const cluster = await prepareClusterForInstallWizard(page, {
      hostId: host.id,
      hostName: host.name,
      version: seatunnelVersion,
      installDir,
      clusterPort,
      httpPort,
    });

    await page.goto(
      buildInstallWizardLabURL({
        hostId: host.id,
        hostName: host.name,
        version: seatunnelVersion,
        installDir,
        clusterPort,
        httpPort,
        clusterId: cluster.clusterId,
      }),
    );

    await page.getByTestId('install-precheck-run').click();
    await expect(page.getByTestId('install-wizard-next')).toBeEnabled({
      timeout: 120000,
    });
    console.log('[installer-real] precheck passed for MinIO-backed scenario');

    await page.getByTestId('install-wizard-next').click();
    await expect(page.getByTestId('install-wizard-step-config')).toBeVisible();

    await chooseSelectOption(page, 'install-config-mirror', /Apache/i);
    await page.getByTestId('install-config-install-dir').fill(installDir);
    await page.getByTestId('install-jvm-hybrid-heap').fill('1');

    await chooseSelectOption(
      page,
      'install-checkpoint-storage-type',
      /AWS S3/i,
    );
    await page
      .getByTestId('install-checkpoint-namespace')
      .fill('/seatunnel/checkpoint/');
    await page.getByTestId('install-checkpoint-endpoint').fill(minioEndpoint);
    await page
      .getByTestId('install-checkpoint-access-key')
      .fill(minioAccessKey);
    await page
      .getByTestId('install-checkpoint-secret-key')
      .fill(minioSecretKey);
    await page.getByTestId('install-checkpoint-bucket').fill(checkpointBucket);
    await page.getByTestId('install-checkpoint-validate').click();
    await expect(
      page
        .getByTestId('install-checkpoint-validation-result')
        .getByText(/校验通过|Validation passed/i),
    ).toBeVisible({timeout: 120000});
    console.log('[installer-real] checkpoint validation passed');

    await chooseSelectOption(page, 'install-imap-storage-type', /AWS S3/i);
    await page.getByTestId('install-imap-namespace').fill('/seatunnel/imap/');
    await page.getByTestId('install-imap-endpoint').fill(minioEndpoint);
    await page.getByTestId('install-imap-access-key').fill(minioAccessKey);
    await page.getByTestId('install-imap-secret-key').fill(minioSecretKey);
    await page.getByTestId('install-imap-bucket').fill(imapBucket);
    await page.getByTestId('install-imap-validate').click();
    await expect(
      page
        .getByTestId('install-imap-validation-result')
        .getByText(/校验通过|Validation passed/i),
    ).toBeVisible({timeout: 120000});
    console.log('[installer-real] imap validation passed');

    await page.getByTestId('install-wizard-next').click();
    await page.getByTestId('install-wizard-next').click();
    await expectInstallationSuccess(page, host.id);
    console.log(
      '[installer-real] installation succeeded for MinIO-backed scenario',
    );

    const files = resolveInstalledConfigPaths(installDir);
    await assertFileContains(files.seatunnel, [
      'storage.type: s3',
      'namespace: /seatunnel/checkpoint/',
      `s3.bucket: ${checkpointBucket}`,
      `fs.s3a.endpoint: ${minioEndpoint}`,
      `fs.s3a.access.key: ${minioAccessKey}`,
      `fs.s3a.secret.key: ${minioSecretKey}`,
    ]);
    await assertFileContains(files.hazelcast, [
      'enabled: true',
      'storage.type: s3',
      'namespace: /seatunnel/imap/',
      `s3.bucket: ${imapBucket}`,
      `fs.defaultFS: ${imapBucket}`,
      `fs.s3a.endpoint: ${minioEndpoint}`,
      `fs.s3a.access.key: ${minioAccessKey}`,
      `fs.s3a.secret.key: ${minioSecretKey}`,
    ]);

    await waitForStxJavaProxyHealthy(page, cluster.clusterId);

    const checkpointProbe = await expectStxJavaProxyProbeSuccess({
      installDir,
      version: seatunnelVersion,
      kind: 'checkpoint',
      request: {
        plugin: 'hdfs',
        mode: 'read_write',
        probeTimeoutMs: 15000,
        config: {
          'storage.type': 's3',
          namespace: '/seatunnel/checkpoint/',
          's3.bucket': checkpointBucket,
          'fs.s3a.endpoint': minioEndpoint,
          'fs.s3a.access.key': minioAccessKey,
          'fs.s3a.secret.key': minioSecretKey,
          'fs.s3a.path.style.access': 'true',
          'fs.s3a.connection.ssl.enabled': 'false',
          'fs.s3a.aws.credentials.provider':
            'org.apache.hadoop.fs.s3a.SimpleAWSCredentialsProvider',
        },
      },
    });
    expect(checkpointProbe.message || '').not.toContain('failed');

    const imapProbe = await expectStxJavaProxyProbeSuccess({
      installDir,
      version: seatunnelVersion,
      kind: 'imap',
      request: {
        plugin: 'hdfs',
        mode: 'read_write',
        deleteAllOnDestroy: true,
        probeTimeoutMs: 15000,
        config: {
          type: 'hdfs',
          'storage.type': 's3',
          namespace: '/seatunnel/imap/',
          clusterName: 'installer-real-e2e',
          businessName: 'stx-java-proxy-e2e',
          's3.bucket': imapBucket,
          'fs.defaultFS': imapBucket,
          'fs.s3a.endpoint': minioEndpoint,
          'fs.s3a.access.key': minioAccessKey,
          'fs.s3a.secret.key': minioSecretKey,
          'fs.s3a.path.style.access': 'true',
          'fs.s3a.connection.ssl.enabled': 'false',
          'fs.s3a.aws.credentials.provider':
            'org.apache.hadoop.fs.s3a.SimpleAWSCredentialsProvider',
        },
      },
    });
    expect(imapProbe.message || '').not.toContain('failed');
  });
});
