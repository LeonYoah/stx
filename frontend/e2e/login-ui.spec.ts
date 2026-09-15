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
import {expectLoginForm, fillAndSubmitLoginForm} from './helpers/auth';

test('renders the credential login form', async ({page}) => {
  await page.goto('/login');

  await expectLoginForm(page);
  // 登录页 h1 为品牌定位文案，h2 为欢迎登录标题。
  // Login page h1 is brand headline; h2 is the welcome title.
  await expect(page.getByRole('heading', {level: 1})).toHaveText(
    /让 SeaTunnel 运维清晰可见|Make SeaTunnel ops clearly visible/i,
  );
  await expect(page.getByRole('heading', {level: 2})).toHaveText(
    /欢迎登录 STX|Welcome to STX/i,
  );
});

test('authenticates with the default admin account', async ({page}) => {
  await page.goto('/login');

  await fillAndSubmitLoginForm(page);

  await page.waitForURL('**/dashboard');
  await expect(page.getByRole('heading', {level: 1})).toHaveText(
    /控制台|Dashboard/i,
  );
  await expect(page.getByRole('button', {name: /刷新|Refresh/i})).toBeVisible();
});
