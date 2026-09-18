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

import {expect, type Page} from '@playwright/test';

export const e2eCredentials = {
  username: process.env.E2E_USERNAME ?? 'admin',
  password: process.env.E2E_PASSWORD ?? 'admin123',
};

export async function fillAndSubmitLoginForm(page: Page): Promise<void> {
  await page.locator('#username').fill(e2eCredentials.username);
  await page.locator('#password').fill(e2eCredentials.password);
  await page.locator("button[type='submit']").click();
}

export async function expectLoginForm(page: Page): Promise<void> {
  await expect(page.locator('#username')).toBeVisible();
  await expect(page.locator('#password')).toBeVisible();
  await expect(
    page.getByRole('button', {name: /进入控制台|Enter console|登录|Login/i}),
  ).toBeVisible();
}

export async function loginThroughUI(page: Page): Promise<void> {
  await page.goto('/login');
  await expectLoginForm(page);
  await fillAndSubmitLoginForm(page);
  await page.waitForURL('**/dashboard');
  await expect(page.getByRole('heading', {level: 1})).toHaveText(
    /控制台|Dashboard/i,
  );
}
