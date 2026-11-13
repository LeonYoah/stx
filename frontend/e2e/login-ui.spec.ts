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
import {
  dismissOnboardingPathChooserIfPresent,
  expectLoginForm,
  fillAndSubmitLoginForm,
} from './helpers/auth';

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
  // 新会话会弹路径选择；公共登录断言前先跳过，避免 Dialog 遮罩/aria-hidden。
  // New sessions open the path chooser; skip before shared assertions so the Dialog overlay/aria-hidden does not block.
  await dismissOnboardingPathChooserIfPresent(page);
  await expect(page.getByRole('heading', {level: 1})).toHaveText(
    /控制台|Dashboard/i,
  );
  await expect(page.getByRole('button', {name: /刷新|Refresh/i})).toBeVisible();
  // 自动化浏览器默认不播放开屏，避免遮挡后续用例。
  // Automated browsers skip the splash by default so later steps aren't blocked.
  await expect(page.getByTestId('welcome-splash')).toHaveCount(0);
  await expect(page.locator('html')).not.toHaveAttribute('data-stx-splash');
});

test('plays the welcome splash after login and can be skipped', async ({
  page,
}) => {
  // 强制开关：让自动化浏览器也播放开屏 / Force switch so the automated browser plays it.
  await page.addInitScript(() => {
    localStorage.setItem('stx:welcome-splash-force', '1');
  });
  await page.goto('/login');
  await fillAndSubmitLoginForm(page);

  await page.waitForURL('**/dashboard');
  // 开屏与路径选择可能同时出现；先清路径选择再断言开屏与首页标题。
  // Splash and path chooser may both appear; clear the chooser before splash/dashboard asserts.
  await dismissOnboardingPathChooserIfPresent(page);
  const splash = page.getByTestId('welcome-splash');
  await expect(splash).toBeVisible();
  // 首页已在遮罩下渲染完毕，跳过即可直接露出 / The dashboard renders beneath and is revealed on skip.
  await expect(page.getByRole('heading', {level: 1})).toHaveText(
    /控制台|Dashboard/i,
  );

  await page.getByRole('button', {name: /跳过|Skip/i}).click();
  await expect(splash).toHaveCount(0);
  await expect(page.locator('html')).not.toHaveAttribute('data-stx-splash');
  await expect(page.getByRole('button', {name: /刷新|Refresh/i})).toBeVisible();
});

test('shows the onboarding path chooser after a fresh login and skip reveals the dashboard', async ({
  page,
}) => {
  // login-ui 项目使用空 storageState，保证新会话会弹出路径选择。
  // login-ui uses empty storageState so a fresh session opens the path chooser.
  await page.goto('/login');
  await fillAndSubmitLoginForm(page);
  await page.waitForURL('**/dashboard');

  const chooser = page.getByTestId('stx-onboarding-path-chooser');
  await expect(chooser).toBeVisible({timeout: 10_000});
  await page.getByTestId('stx-onboarding-path-skip').click();
  await expect(chooser).toHaveCount(0);

  await expect(page.getByRole('heading', {level: 1})).toHaveText(
    /控制台|Dashboard/i,
  );
  await expect(page.getByRole('button', {name: /刷新|Refresh/i})).toBeVisible();
});
