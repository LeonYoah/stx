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

import {execFileSync} from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';
import {expect, test, type Page} from '@playwright/test';

test.use({acceptDownloads: true});

const version = '2.3.13-e2e';
const bundleFileName = `stx-seatunnel-offline-${version}.tar.gz`;

function backendURL(pathname: string): string {
  const base = process.env.E2E_BACKEND_BASE_URL ?? 'http://localhost:17800';
  return new URL(pathname, base).toString();
}

async function deleteIfExists(
  page: Page,
  pathname: string,
  key: string,
): Promise<void> {
  const response = await page.request.delete(backendURL(pathname), {
    headers: {
      'Idempotency-Key': key,
      'X-STX-Confirm': 'true',
    },
  });
  if (response.ok() || response.status() === 400 || response.status() === 404) {
    return;
  }
  throw new Error(
    `cleanup failed ${response.status()}: ${await response.text()}`,
  );
}

function createOfficialStylePackage(workdir: string): string {
  const root = path.join(workdir, `apache-seatunnel-${version}`);
  fs.mkdirSync(path.join(root, 'bin'), {recursive: true});
  fs.writeFileSync(path.join(root, 'bin', 'seatunnel.sh'), '#!/bin/sh\n');
  const archive = path.join(workdir, `apache-seatunnel-${version}-bin.tar.gz`);
  execFileSync('tar', ['-czf', archive, '-C', workdir, path.basename(root)]);
  return archive;
}

test('offline assets export import and official package import', async ({
  page,
}, testInfo) => {
  const workdir = testInfo.outputPath('offline-assets');
  fs.mkdirSync(workdir, {recursive: true});
  const packageArchive = createOfficialStylePackage(workdir);

  await page.goto('/packages');
  await expect(page.getByTestId('package-management-root')).toBeVisible();

  await deleteIfExists(
    page,
    `/api/v1/packages/${encodeURIComponent(version)}`,
    `e2e-package-cleanup-${Date.now()}`,
  );
  await deleteIfExists(
    page,
    `/api/v1/packages/offline-bundles/${encodeURIComponent(bundleFileName)}`,
    `e2e-offline-bundle-cleanup-${Date.now()}`,
  );
  await page.reload();
  await expect(page.getByTestId('package-management-root')).toBeVisible();

  await page.getByRole('button', {name: /离线导入|Offline Import/i}).click();
  await page.getByTestId('offline-assets-tab-import').click();
  await page
    .getByTestId('offline-assets-import-input')
    .setInputFiles(packageArchive);
  await expect(page.getByTestId('offline-import-version')).toHaveValue(version);

  const uploadResponse = page.waitForResponse(
    (response) =>
      response.url().includes('/api/v1/packages/upload/chunk') &&
      response.request().method() === 'POST' &&
      response.ok(),
  );
  await page.getByTestId('offline-import-submit').click();
  await uploadResponse;
  await expect(
    page.getByText(/已上传安装包|Uploaded package/).first(),
  ).toBeVisible();

  await page.getByTestId('offline-assets-tab-export').click();
  await page.getByTestId('offline-bundle-export-version').click();
  await page.getByRole('option', {name: new RegExp(version)}).click();
  const includePlugins = page.getByTestId('offline-bundle-include-plugins');
  if ((await includePlugins.getAttribute('data-state')) === 'checked') {
    await includePlugins.click();
  }

  const createResponse = page.waitForResponse(
    (response) =>
      new URL(response.url()).pathname.endsWith(
        '/api/v1/packages/offline-bundles',
      ) &&
      response.request().method() === 'POST' &&
      response.ok(),
  );
  const downloadEvent = page.waitForEvent('download');
  await page.getByTestId('offline-bundle-create-submit').click();
  await createResponse;
  const download = await downloadEvent;
  expect(download.suggestedFilename()).toBe(bundleFileName);
  await expect(page.getByText(bundleFileName, {exact: true})).toBeVisible();

  await deleteIfExists(
    page,
    `/api/v1/packages/${encodeURIComponent(version)}`,
    `e2e-package-delete-before-import-${Date.now()}`,
  );

  await page.getByTestId('offline-assets-tab-import').click();
  const downloadedPath = await download.path();
  const bundleBuffer = fs.readFileSync(downloadedPath);
  await page.getByTestId('offline-assets-import-input').setInputFiles({
    name: bundleFileName,
    mimeType: 'application/gzip',
    buffer: bundleBuffer,
  });
  await expect(
    page.getByText(/已识别为离线资产包|Recognized as an offline asset bundle/),
  ).toBeVisible();
  await expect(page.getByTestId('offline-import-version')).toHaveCount(0);

  const importResponse = page.waitForResponse(
    (response) =>
      new URL(response.url()).pathname.endsWith(
        '/api/v1/packages/offline-bundles/import',
      ) &&
      response.request().method() === 'POST' &&
      response.ok(),
  );
  await page.getByTestId('offline-import-submit').click();
  await importResponse;
  await expect(
    page.getByText(/已导入离线资产包|Imported offline asset bundle/).first(),
  ).toBeVisible();

  const restoredPackage = await page.request.get(
    backendURL(`/api/v1/packages/${encodeURIComponent(version)}`),
  );
  expect(restoredPackage.ok()).toBe(true);

  await deleteIfExists(
    page,
    `/api/v1/packages/offline-bundles/${encodeURIComponent(bundleFileName)}`,
    `e2e-offline-bundle-final-cleanup-${Date.now()}`,
  );
  await deleteIfExists(
    page,
    `/api/v1/packages/${encodeURIComponent(version)}`,
    `e2e-package-final-cleanup-${Date.now()}`,
  );
});
