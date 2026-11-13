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

/**
 * AC9 alignment self-test: measure header vs first-row column left edges
 * across 1440/1280/768/390 × light/dark for four marketplace table tabs.
 * Results + screenshots written under /opt/cursor/artifacts/issue129-v6-align/
 */

import {chromium} from '@playwright/test';
import fs from 'fs';
import path from 'path';

const outDir = '/opt/cursor/artifacts/issue129-v6-align';
fs.mkdirSync(outDir, {recursive: true});

const viewports = [
  {w: 1440, h: 900},
  {w: 1280, h: 800},
  {w: 768, h: 1024},
  {w: 390, h: 844},
];
const themes = ['light', 'dark'];

const tabs = [
  {
    page: 'plugins',
    url: 'http://127.0.0.1:3000/plugins',
    catalog: 'plugin-available-catalog',
    pill: '可用插件',
  },
  {
    page: 'plugins-local',
    url: 'http://127.0.0.1:3000/plugins',
    catalog: 'plugin-local-catalog',
    pill: '本地插件',
    after: async (page) => {
      const tab = page.locator('[data-testid="plugin-tab-local"]');
      if (await tab.count()) {
        await tab.first().click();
      } else {
        await page.getByRole('button', {name: /本地插件/}).first().click();
      }
      await page.waitForTimeout(1200);
    },
  },
  {
    page: 'packages-online',
    url: 'http://127.0.0.1:3000/packages',
    catalog: 'package-online-catalog',
    pill: '在线版本',
  },
  {
    page: 'packages-local',
    url: 'http://127.0.0.1:3000/packages',
    catalog: 'package-local-catalog',
    pill: '本地',
    after: async (page) => {
      await page.getByRole('button', {name: /本地安装包|本地/}).first().click();
      await page.waitForTimeout(1200);
    },
  },
];

function measure(page, catalogTestId) {
  return page.evaluate((id) => {
    const empty = document.querySelector(`[data-testid="${id}-empty"]`);
    if (empty) {
      return {ok: true, pass: true, skipped: true, reason: 'empty-list'};
    }
    const root = document.querySelector(`[data-testid="${id}"]`);
    if (!root) {
      return {ok: false, reason: 'catalog-missing'};
    }
    const header = root.querySelector(`[data-testid="${id}-header"]`);
    const row = root.querySelector(`[data-testid^="marketplace-row-"]`);
    if (!header || !row) {
      return {ok: false, reason: 'header-or-row-missing', hasHeader: Boolean(header), hasRow: Boolean(row)};
    }
    const keys = Array.from(header.querySelectorAll(':scope > [data-col]')).map(
      (el) => el.getAttribute('data-col'),
    );
    const cols = keys.map((key, index) => {
      const h = header.querySelector(`:scope > [data-col="${key}"]`);
      const r = row.querySelector(`:scope > [data-col="${key}"]`);
      const hx = h ? h.getBoundingClientRect().left : NaN;
      const rx = r ? r.getBoundingClientRect().left : NaN;
      return {
        index,
        key,
        headerX: hx,
        rowX: rx,
        dx: Number.isFinite(hx) && Number.isFinite(rx)
          ? Math.round((hx - rx) * 10) / 10
          : null,
      };
    });
    const finite = cols.filter((c) => c.dx != null);
    const maxAbs = finite.length
      ? Math.max(...finite.map((c) => Math.abs(c.dx)))
      : 999;
    return {
      ok: true,
      maxAbs,
      pass: maxAbs <= 1,
      cols,
      headerClass: header.className,
      rowClass: row.className,
      sameGrid:
        header.className.split(/\s+/).filter((c) => c.startsWith('grid-cols-'))[0] ===
        row.className.split(/\s+/).filter((c) => c.startsWith('grid-cols-'))[0],
    };
  }, catalogTestId);
}

const browser = await chromium.launch({
  headless: true,
  executablePath: '/usr/local/bin/google-chrome',
  args: ['--no-sandbox', '--disable-dev-shm-usage'],
});

const results = [];

for (const theme of themes) {
  for (const vp of viewports) {
    const context = await browser.newContext({
      viewport: {width: vp.w, height: vp.h},
      locale: 'zh-CN',
      colorScheme: theme,
    });
    const page = await context.newPage();
    await page.goto('http://127.0.0.1:3000/login', {waitUntil: 'networkidle'});
    await page.locator('input[type="text"], input[name="username"]').first().fill('admin');
    await page.locator('input[type="password"]').first().fill('admin123');
    await page.locator('button[type="submit"]').first().click();
    await page.waitForURL((url) => !url.pathname.includes('/login'), {timeout: 30000});

    if (theme === 'dark') {
      await page.evaluate(() => {
        document.documentElement.classList.add('dark');
      });
    } else {
      await page.evaluate(() => {
        document.documentElement.classList.remove('dark');
      });
    }

    for (const tab of tabs) {
      await page.goto(tab.url, {waitUntil: 'networkidle'});
      await page.waitForTimeout(1200);
      if (tab.after) {
        await tab.after(page);
      }
      // Force table view
      const tableToggle = page.getByRole('radio', {name: /表格|Table/i});
      if (await tableToggle.count()) {
        await tableToggle.first().click();
        await page.waitForTimeout(600);
      }

      const m = await measure(page, tab.catalog);
      const shot = `${tab.page}-${vp.w}x${vp.h}-${theme}.png`;
      const catalog = page.locator(`[data-testid="${tab.catalog}"]`);
      if (await catalog.count()) {
        await catalog.first().screenshot({path: path.join(outDir, shot)});
      } else {
        await page.screenshot({path: path.join(outDir, shot), fullPage: false});
      }
      results.push({
        tab: tab.page,
        viewport: `${vp.w}x${vp.h}`,
        theme,
        shot,
        ...m,
      });
    }
    await context.close();
  }
}

await browser.close();

const measured = results.filter((r) => !r.skipped);
const summary = {
  generatedAt: new Date().toISOString(),
  pass: measured.every((r) => r.ok && r.pass && r.sameGrid !== false),
  measuredCount: measured.length,
  skippedEmpty: results.filter((r) => r.skipped).length,
  results,
};
fs.writeFileSync(path.join(outDir, 'align-results.json'), JSON.stringify(summary, null, 2));
console.log(
  JSON.stringify(
    {
      pass: summary.pass,
      measuredCount: summary.measuredCount,
      skippedEmpty: summary.skippedEmpty,
      fail: measured.filter((r) => !r.pass || !r.ok),
    },
    null,
    2,
  ),
);
