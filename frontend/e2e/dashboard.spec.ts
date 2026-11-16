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
  // 首页监控大盘：指标卡、需要处理条与三个页签（默认停在集群大盘）
  // Home dashboard: KPI cards, the needs-attention strip and three tabs (opens on the cluster dashboard)
  await expect(page.getByTestId('home-kpi-row')).toBeVisible();
  await expect(page.getByTestId('home-attention')).toBeVisible();
  for (const name of [/^集群$|^Clusters$/i, /^作业$|^Jobs$/i, /^作业列表$|^Job list$/i]) {
    await expect(page.getByRole('tab', {name})).toBeVisible();
  }
  await expect(page.getByTestId('home-cluster-tab')).toBeVisible();
});
