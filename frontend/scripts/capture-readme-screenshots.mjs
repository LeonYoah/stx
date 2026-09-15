/**
 * Capture full-page README screenshots.
 * Dark theme by default; audit logs and user management use light theme.
 */
import {chromium} from '@playwright/test';
import path from 'node:path';
import {fileURLToPath} from 'node:url';
import fs from 'node:fs';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const outDir = path.resolve(__dirname, '../../docs/screenshots');
const baseURL = process.env.README_BASE_URL ?? 'http://localhost:3000';
const username = process.env.E2E_USERNAME ?? 'admin';
const password = process.env.E2E_PASSWORD ?? 'admin123';

fs.mkdirSync(outDir, {recursive: true});

async function applyTheme(page, theme) {
  await page.addInitScript((value) => {
    try {
      localStorage.setItem('theme', value);
    } catch {
      // ignore
    }
  }, theme);

  await page.evaluate((value) => {
    try {
      localStorage.setItem('theme', value);
    } catch {
      // ignore
    }
    const root = document.documentElement;
    root.classList.toggle('dark', value === 'dark');
    root.style.colorScheme = value;
  }, theme);
}

async function settle(page) {
  await page.waitForLoadState('domcontentloaded');
  await page.waitForTimeout(1200);
}

async function shot(page, name, {fullPage = true} = {}) {
  const file = path.join(outDir, name);
  await page.screenshot({path: file, fullPage});
  console.log(`saved ${file}`);
}

async function gotoThemed(page, route, theme) {
  await applyTheme(page, theme);
  await page.goto(`${baseURL}${route}`, {waitUntil: 'domcontentloaded'});
  await applyTheme(page, theme);
  await settle(page);
}

async function main() {
  const browser = await chromium.launch({headless: true});
  const context = await browser.newContext({
    viewport: {width: 1440, height: 960},
    deviceScaleFactor: 1,
  });
  const page = await context.newPage();

  await applyTheme(page, 'dark');
  await page.goto(`${baseURL}/login`, {waitUntil: 'domcontentloaded'});
  await applyTheme(page, 'dark');
  await settle(page);
  await page.locator('#username').waitFor({state: 'visible', timeout: 20000});
  await shot(page, '00-login.png');

  await page.locator('#username').fill(username);
  await page.locator('#password').fill(password);
  await Promise.all([
    page.waitForURL((url) => !url.pathname.includes('/login'), {
      timeout: 45000,
    }),
    page.locator("button[type='submit']").click(),
  ]);
  await applyTheme(page, 'dark');
  await settle(page);

  const darkRoutes = [
    ['01-dashboard.png', '/dashboard'],
    ['02-workbench.png', '/workbench'],
    ['03-hosts.png', '/hosts'],
    ['04-clusters.png', '/clusters'],
    ['05-monitoring.png', '/monitoring'],
    ['06-diagnostics.png', '/diagnostics'],
    ['07-packages.png', '/packages'],
    ['08-plugins.png', '/plugins'],
  ];

  for (const [name, route] of darkRoutes) {
    await gotoThemed(page, route, 'dark');
    await shot(page, name);
  }

  // Cluster detail
  await gotoThemed(page, '/clusters', 'dark');
  const clusterLink = page.locator('a[href^="/clusters/"]').first();
  if ((await clusterLink.count()) > 0) {
    await clusterLink.click();
    await settle(page);
    await applyTheme(page, 'dark');
    await settle(page);
    await shot(page, '09-cluster-detail.png');
  } else {
    console.log('skip cluster detail: no cluster link found');
  }

  // Sync studio
  await gotoThemed(page, '/sync', 'dark');
  if (!page.url().includes('/login')) {
    await shot(page, '10-sync.png');
  } else {
    console.log('skip sync: redirected to login');
  }

  // User center dialog
  await gotoThemed(page, '/dashboard', 'dark');
  const profileBtn = page
    .locator('div.fixed.z-50')
    .locator('button, div[role="button"], div.cursor-pointer')
    .filter({has: page.locator('svg')})
    .last();
  // Click the dock profile User icon wrapper
  const userIcon = page.locator('div.fixed.z-50 svg.lucide-user').last();
  if ((await userIcon.count()) > 0) {
    await userIcon.click({force: true});
  } else if ((await profileBtn.count()) > 0) {
    await profileBtn.click({force: true});
  }
  await page.waitForTimeout(800);
  await shot(page, '11-user-center.png', {fullPage: false});
  await page.keyboard.press('Escape').catch(() => undefined);

  // Light theme pages
  for (const [name, route] of [
    ['12-audit-logs.png', '/audit-logs'],
    ['13-users.png', '/admin/users'],
  ]) {
    await gotoThemed(page, route, 'light');
    await shot(page, name);
  }

  await browser.close();
}

main().catch((error) => {
  console.error(error);
  process.exit(1);
});
