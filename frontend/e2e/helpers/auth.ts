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
import {ONBOARDING_STORAGE_PREFIX} from '@/lib/onboarding/storage';

export const e2eCredentials = {
  username: process.env.E2E_USERNAME ?? 'admin',
  password: process.env.E2E_PASSWORD ?? 'admin123',
};

/**
 * auth.setup / dismiss 写入的「暂时跳过」载荷（displayMode=hidden，path 仍为 null）。
 * Payload written by auth.setup / dismiss for “skip for now” (displayMode=hidden, path still null).
 */
const ONBOARDING_SKIPPED_STATE_JSON = JSON.stringify({
  version: 2,
  path: null,
  displayMode: 'hidden',
  targetClusterId: null,
  finished: false,
  completedStepIds: [],
});

/**
 * 用例需要清空 localStorage 做隔离时：清库后保留（或回种）上手跳过状态，
 * 避免 OnboardingPathChooser 再次挡住点击 / 把 h1 aria-hidden。
 * When a spec clears localStorage for isolation, keep (or re-seed) the skipped
 * onboarding state so OnboardingPathChooser does not block clicks or aria-hide h1.
 *
 * 仅影响 stx:onboarding:v1:*；引导专用用例不要调用。
 * Only touches stx:onboarding:v1:*; specs that assert the guide must not use this.
 */
export async function addInitScriptClearLocalStorageKeepOnboardingSkipped(
  page: Page,
): Promise<void> {
  await page.addInitScript(
    ({prefix, skippedJson}) => {
      const preserved: Record<string, string> = {};
      for (let i = 0; i < window.localStorage.length; i += 1) {
        const key = window.localStorage.key(i);
        if (key && key.startsWith(prefix)) {
          const value = window.localStorage.getItem(key);
          if (value != null) {
            preserved[key] = value;
          }
        }
      }
      window.localStorage.clear();
      for (const [key, value] of Object.entries(preserved)) {
        window.localStorage.setItem(key, value);
      }
      // 无既有键时回种默认管理员 id=1（E2E SQLite 种子）；仍建议配合 dismiss。
      // If nothing was preserved, re-seed default admin id=1 (E2E SQLite seed); still pair with dismiss.
      if (Object.keys(preserved).length === 0) {
        window.localStorage.setItem(`${prefix}1`, skippedJson);
      }
    },
    {
      prefix: ONBOARDING_STORAGE_PREFIX,
      skippedJson: ONBOARDING_SKIPPED_STATE_JSON,
    },
  );
}

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

/**
 * 若登录后出现上手路径选择弹窗则点「跳过」并等到消失；没有弹窗则立刻继续。
 * If the onboarding path chooser appears after login, click Skip and wait until
 * it is gone; otherwise continue immediately (do not hard-wait for a timeout).
 *
 * 产品侧仍会在新会话弹出该 Dialog；E2E 公共登录路径用本 helper 避开遮罩拦截，
 * 专门覆盖引导的用例不要调用它。
 * Product still opens the Dialog for new sessions; shared E2E login paths use
 * this helper to clear the overlay. Specs that assert the chooser must not call it.
 */
export async function dismissOnboardingPathChooserIfPresent(
  page: Page,
): Promise<void> {
  const chooser = page.getByTestId('stx-onboarding-path-chooser');
  const skip = page.getByTestId('stx-onboarding-path-skip');
  const heading = page.getByRole('heading', {level: 1});

  // 弹窗依赖 Provider ready，可能略晚于路由；与首页标题竞态，避免空等满超时。
  // Chooser may lag the route slightly; race the dashboard heading so we don't idle the full timeout.
  const first = await Promise.race([
    chooser.waitFor({state: 'visible', timeout: 5000}).then(() => 'chooser' as const),
    heading.waitFor({state: 'visible', timeout: 5000}).then(() => 'heading' as const),
  ]).catch(() => 'none' as const);

  const dismiss = async () => {
    await skip.click();
    await expect(chooser).toHaveCount(0);
  };

  if (first === 'chooser' || (await chooser.isVisible().catch(() => false))) {
    await dismiss();
    return;
  }
  if (first === 'heading') {
    // 首屏标题已出后仍可能刚挂上 Dialog，再短等一次。
    // Dialog can mount just after the first paint — brief follow-up wait.
    const late = await chooser
      .waitFor({state: 'visible', timeout: 1500})
      .then(() => true)
      .catch(() => false);
    if (late) {
      await dismiss();
    }
  }
}

export async function loginThroughUI(page: Page): Promise<void> {
  await page.goto('/login');
  await expectLoginForm(page);
  await fillAndSubmitLoginForm(page);
  await page.waitForURL('**/dashboard');
  // auth.setup / 复用登录态的业务用例都经此入口，需清掉路径选择遮罩。
  // auth.setup and specs that reuse storage go through here — clear the chooser overlay.
  await dismissOnboardingPathChooserIfPresent(page);
  await expect(page.getByRole('heading', {level: 1})).toHaveText(
    /控制台|Dashboard/i,
  );
}
