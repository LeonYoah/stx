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

import {expect, test} from '@playwright/test';

test('opens dashboard with a reused authenticated session', async ({page}) => {
  await page.goto('/dashboard');

  await expect(page).toHaveURL(/\/dashboard$/);
  await expect(page.getByRole('heading', {level: 1})).toHaveText(
    /控制台|Console|Dashboard/i,
  );
  await expect(page.getByRole('button', {name: /刷新|Refresh/i})).toBeVisible();
  // 首页监控大盘：7 张指标卡（含数据量）、需要处理条与四个页签（默认停在集群大盘）。
  // Home dashboard: seven KPI cards (including volume), the needs-attention strip and four tabs
  // (opens on the cluster dashboard).
  await expect(page.getByTestId('home-kpi-row')).toBeVisible();
  for (const key of [
    'running',
    'write',
    'cdc',
    'failed',
    'volume',
    'clusters',
    'alerts',
  ]) {
    await expect(page.getByTestId(`home-kpi-${key}`)).toBeVisible();
  }
  await expect(page.getByTestId('home-attention')).toBeVisible();
  for (const name of [
    /^集群$|^Clusters$/i,
    /^作业$|^Jobs$/i,
    /^主机$|^Hosts$/i,
    /^作业列表$|^Job list$/i,
  ]) {
    await expect(page.getByRole('tab', {name})).toBeVisible();
  }
  await expect(page.getByTestId('home-cluster-tab')).toBeVisible();
  // 可观测关闭时走空态，开启时出现可用性条；两种环境都不能塌成空白。
  // Observability-off shows the empty state; observability-on shows the availability bar.
  // Neither environment may collapse into a blank panel.
  await expect(
    page
      .getByTestId('home-trend-empty')
      .or(page.getByTestId('home-availability')),
  ).toBeVisible();

  await page.getByRole('tab', {name: /^作业$|^Jobs$/i}).click();
  await expect(page.getByTestId('home-jobs-tab')).toBeVisible();
  await expect(
    page.getByTestId('home-trend-empty').or(page.getByTestId('home-volume')),
  ).toBeVisible();
  await expect(page.getByTestId('home-calendar')).toBeVisible();

  await page.getByRole('tab', {name: /^主机$|^Hosts$/i}).click();
  await expect(page.getByTestId('home-hosts')).toBeVisible();

  await page.getByRole('tab', {name: /^作业列表$|^Job list$/i}).click();
  await expect(page.getByTestId('home-job-list')).toBeVisible();
});
