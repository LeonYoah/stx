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

/*
 * 首页大盘截图脚本：真实登录 + /api/v1/dashboard/home/** 定向夹具。
 * 用法（在 frontend/ 下执行，playwright 用仓库依赖）：
 *   node ../.trellis/tasks/11-19-dashboard-visual-refresh/screenshots/capture.mjs
 * 环境变量：BASE_URL（默认 http://127.0.0.1:3010）、CAPTURE_PREFIX（before/after）、
 * CAPTURE_WIDTHS（逗号分隔，默认 1440）、ADMIN_USER / ADMIN_PASSWORD。
 */

import path from 'node:path';
import {createRequire} from 'node:module';
import {fileURLToPath} from 'node:url';

// 从 frontend/node_modules 解析 playwright（脚本放在 .trellis 下，不在包内）。
// Resolve playwright from frontend/node_modules since this script lives outside the package.
const require = createRequire(
  path.resolve(path.dirname(fileURLToPath(import.meta.url)), '../../../../frontend/package.json'),
);
const {chromium} = require('@playwright/test');

const OUT_DIR = path.dirname(fileURLToPath(import.meta.url));
const BASE_URL = process.env.BASE_URL ?? 'http://127.0.0.1:3010';
const PREFIX = process.env.CAPTURE_PREFIX ?? 'before';
const WIDTHS = (process.env.CAPTURE_WIDTHS ?? '1440')
  .split(',')
  .map((v) => Number(v.trim()))
  .filter(Boolean);
const USER = process.env.ADMIN_USER ?? 'admin';
const PASSWORD = process.env.ADMIN_PASSWORD ?? 'admin123';

const ONBOARDING_SKIPPED = JSON.stringify({
  version: 2,
  path: null,
  displayMode: 'hidden',
  targetClusterId: null,
  finished: false,
  completedStepIds: [],
});

const now = Date.now();
const HOUR = 3_600_000;
const startMs = now - HOUR;

function series(base, jitter, points = 60) {
  const step = HOUR / points;
  return Array.from({length: points}, (_, i) => [
    startMs + i * step,
    Math.max(0, Math.round(base + jitter * Math.sin(i / 6) + i * jitter * 0.15)),
  ]);
}

const summaryWithData = {
  generated_at: new Date(now).toISOString(),
  jobs: {
    running: 9,
    running_streaming: 7,
    running_batch: 2,
    failed_24h: 3,
    failed_prev_24h: 5,
    write_qps: 48210.5,
    read_qps: 39600,
    max_cdc_delay_seconds: 125.4,
    max_cdc_job_name: 'pg-cdc-ledger',
    cdc_threshold_seconds: 60,
    trend_source: 'prometheus',
    write_trend: [
      [startMs, 30000],
      [now, 48200],
    ],
    cdc_trend: [],
    attention: [],
  },
  clusters: {
    total: 3,
    healthy: 2,
    degraded: 1,
    unhealthy: 0,
    nodes_online: 5,
    nodes_total: 6,
  },
  alerts: {active: 4, critical: 2},
  attention: [
    {
      kind: 'job_failed',
      severity: 'critical',
      title: 'orders-cdc-sync',
      detail: 'Sink 写入超时',
      job_id: 11,
      task_id: 5,
      at: new Date(now - 600_000).toISOString(),
      viewable: true,
    },
    {
      kind: 'cdc_delay',
      severity: 'warning',
      title: 'pg-cdc-ledger',
      value: 125.4,
      job_id: 12,
      task_id: 5,
      at: new Date(now - 300_000).toISOString(),
      viewable: true,
    },
    {
      kind: 'alert',
      severity: 'warning',
      title: 'CPU 使用率过高',
      alert_id: 'demo-alert-1',
      at: new Date(now - 120_000).toISOString(),
      viewable: true,
    },
  ],
};

const summaryEmpty = {
  ...summaryWithData,
  jobs: {
    ...summaryWithData.jobs,
    running: 0,
    running_streaming: 0,
    running_batch: 0,
    failed_24h: 0,
    failed_prev_24h: 0,
    write_qps: 0,
    read_qps: 0,
    max_cdc_delay_seconds: 0,
    max_cdc_job_name: '',
    write_trend: [],
  },
  clusters: {
    total: 1,
    healthy: 1,
    degraded: 0,
    unhealthy: 0,
    nodes_online: 2,
    nodes_total: 2,
  },
  alerts: {active: 0, critical: 0},
  attention: [],
};

const week = () => Array.from({length: 12}, (_, w) =>
  Array.from({length: 7}, (_, d) => (w * 7 + d) % 5 === 0 ? 0 : ((w * 7 + d) * 3) % 9),
);
const failedWeek = () =>
  Array.from({length: 12}, (_, w) =>
    Array.from({length: 7}, (_, d) => ((w * 7 + d) % 11 === 0 ? 2 : 0)),
  );

const jobsWithData = {
  source: 'prometheus',
  range: '1h',
  start_ms: startMs,
  end_ms: now,
  step_seconds: 60,
  window_seconds: 3600,
  throughput: [
    {key: 'write', points: series(42000, 6000)},
    {key: 'read', points: series(36000, 5000)},
    // 一条空序列：字节类指标无数据时指标格显示「—」。
    {key: 'write_bytes', points: []},
    {key: 'read_bytes', points: series(18_500_000, 3_000_000)},
  ],
  annotations: [
    {
      ts_ms: startMs + HOUR / 2,
      kind: 'job_failed',
      title: 'orders-cdc-sync 失败',
      job_id: 11,
      task_id: 5,
      viewable: true,
    },
  ],
  top_write: [],
  top_cdc: [],
  flow: [
    {source: 'MySQL CDC', sink: 'Doris', value: 1_234_560},
    {source: 'Kafka', sink: 'ClickHouse', value: 890_000},
    {source: 'MySQL CDC', sink: 'Kafka', value: 456_000},
  ],
  checkpoints: [],
  hotspots: [
    {
      job_id: 11,
      task_id: 5,
      name: 'orders-cdc-sync',
      value: 0.62,
      kind: 'busy',
      vertex: 'Source: MySQL',
      viewable: true,
    },
    {
      job_id: 12,
      task_id: 5,
      name: 'pg-cdc-ledger',
      value: 0.41,
      kind: 'backpressure',
      vertex: 'Sink: Doris',
      viewable: true,
    },
  ],
  calendar: {weeks: 12, end_date: '2026-11-19', runs: week(), failed: failedWeek()},
};

const jobsEmpty = {
  ...jobsWithData,
  throughput: [],
  annotations: [],
  top_write: [],
  top_cdc: [],
  flow: [],
  checkpoints: [],
  hotspots: [],
  calendar: {weeks: 12, end_date: '2026-11-19', runs: Array.from({length: 12}, () => Array(7).fill(0)), failed: Array.from({length: 12}, () => Array(7).fill(0))},
};

const volumeBuckets = Array.from({length: 12}, (_, i) => ({
  start_ms: startMs + (i * HOUR) / 12,
  end_ms: startMs + ((i + 1) * HOUR) / 12,
  label: `2026-11-19 ${String(8 + i).padStart(2, '0')}:00`,
  partial: false,
}));

const volumeWithData = {
  oldest_sample_ms: now - 30 * 24 * HOUR,
  source: 'prometheus',
  range: '1h',
  start_ms: startMs,
  end_ms: now,
  granularity: 'hour',
  split: 'none',
  totals: {
    window: {read_rows: 2_400_000, write_rows: 1_523_000, read_bytes: 1_800_000_000, write_bytes: 1_100_000_000},
    today: {read_rows: 9_000_000, write_rows: 5_000_000, read_bytes: 7_000_000_000, write_bytes: 4_000_000_000},
    month: {read_rows: 90_000_000, write_rows: 60_000_000, read_bytes: 70_000_000_000, write_bytes: 45_000_000_000},
    today_start_ms: startMs,
    month_start_ms: startMs,
  },
  buckets: volumeBuckets,
  series: [
    {metric: 'write_rows', group_id: 'all', totals: volumeBuckets.map((_, i) => 120_000 + i * 3_000)},
    {metric: 'read_rows', group_id: 'all', totals: volumeBuckets.map((_, i) => 200_000 + i * 4_000)},
    {metric: 'write_bytes', group_id: 'all', totals: volumeBuckets.map((_, i) => 90_000_000 + i * 2_000_000)},
    {metric: 'read_bytes', group_id: 'all', totals: volumeBuckets.map((_, i) => 150_000_000 + i * 3_000_000)},
  ],
};

const volumeEmpty = {
  ...volumeWithData,
  totals: {
    window: {read_rows: 0, write_rows: 0, read_bytes: 0, write_bytes: 0},
    today: {read_rows: 0, write_rows: 0, read_bytes: 0, write_bytes: 0},
    month: {read_rows: 0, write_rows: 0, read_bytes: 0, write_bytes: 0},
    today_start_ms: startMs,
    month_start_ms: startMs,
  },
  buckets: [],
  series: [],
};

const clustersWithData = {
  source: 'prometheus',
  range: '1h',
  start_ms: startMs,
  end_ms: now,
  step_seconds: 60,
  cell_seconds: 120,
  clusters: [
    {id: 1, name: 'prod-st', version: '2.3.12', health_status: 'healthy', nodes_online: 3, nodes_total: 3, running_jobs: 6, active_alerts: 1, max_heap_percent: 72, endpoints_down: 0},
    {id: 2, name: 'staging', version: '2.3.12', health_status: 'degraded', nodes_online: 1, nodes_total: 2, running_jobs: 3, active_alerts: 2, max_heap_percent: 88, endpoints_down: 1},
    {id: 3, name: 'bench', version: '2.3.11', health_status: 'healthy', nodes_online: 1, nodes_total: 1, running_jobs: 0, active_alerts: 1, max_heap_percent: 30, endpoints_down: 0},
  ],
  nodes: [
    {cluster_id: 1, cluster_name: 'prod-st', node_id: 1, instance: 'prod-st-1:5801', role: 'master', status: 'running', agent_online: true, master: true, scrape_up: true, node_active: true, heap_percent: 68, cpu_cores: 4, threads: 210, fd_percent: 12, uptime_seconds: 2_600_000},
    {cluster_id: 1, cluster_name: 'prod-st', node_id: 2, instance: 'prod-st-2:5801', role: 'worker', status: 'running', agent_online: true, scrape_up: true, node_active: true, heap_percent: 72, cpu_cores: 4, threads: 198, fd_percent: 9, uptime_seconds: 2_600_000},
    {cluster_id: 2, cluster_name: 'staging', node_id: 3, instance: 'staging-1:5801', role: 'worker', status: 'running', agent_online: true, scrape_up: false, node_active: true, heap_percent: 88, cpu_cores: 2, threads: 130, fd_percent: 41, uptime_seconds: 400_000},
  ],
  signals: {
    clusters_safe: 2,
    clusters_total: 3,
    deadlocked_threads: 0,
    fd_percent_max: 41,
    pool_reject_rate: 0,
    hz_queue_util_max: 12,
    endpoints_down: 1,
  },
  series: [
    {key: 'availability', label: 'prod-st-1:5801', points: series(1, 0, 30)},
    {key: 'availability', label: 'prod-st-2:5801', points: series(1, 0, 30)},
    {key: 'availability', label: 'staging-1:5801', points: series(0.7, 0.4, 30)},
    {key: 'heap', label: 'prod-st-1:5801', points: series(60, 8, 30)},
    {key: 'heap', label: 'prod-st-2:5801', points: series(66, 9, 30)},
    {key: 'heap', label: 'staging-1:5801', points: series(80, 6, 30)},
    {key: 'cpu', label: 'prod-st-1:5801', points: series(1.8, 0.5, 30)},
    {key: 'cpu', label: 'prod-st-2:5801', points: series(2.2, 0.6, 30)},
    {key: 'gc', label: 'prod-st-1:5801', points: series(3, 1.2, 30)},
    {key: 'job_state', label: 'running', points: series(9, 2, 30)},
    {key: 'job_state', label: 'failed', points: series(1, 1, 30)},
    {key: 'job_state', label: 'finished', points: series(14, 4, 30)},
  ],
};

const clustersEmpty = {
  ...clustersWithData,
  clusters: [clustersWithData.clusters[0]],
  nodes: [],
  signals: null,
  series: [],
};

const hostsFixture = {
  source: 'prometheus',
  range: '1h',
  step_seconds: 60,
  start_ms: startMs,
  end_ms: now,
  hosts: [
    {
      id: 1,
      name: 'macdeMacBook-Pro.local',
      ip: '172.18.196.113',
      host_type: 'physical',
      agent_status: 'online',
      cpu_cores: 8,
      total_memory_bytes: 32 * 1024 ** 3,
      last_heartbeat_ms: now - 5_000,
      boot_time_ms: now - 2_600_000_000,
      metrics_supported: true,
      panels: [],
    },
  ],
};

const detailFixture = {
  source: 'prometheus',
  range: '1h',
  start_ms: startMs,
  end_ms: now,
  step_seconds: 60,
  group: 'jvm',
  panels: [],
  events: [],
};

function fixturesFor(withData) {
  return {
    '/summary': withData ? summaryWithData : summaryEmpty,
    '/jobs': withData ? jobsWithData : jobsEmpty,
    '/volume': withData ? volumeWithData : volumeEmpty,
    '/clusters/detail': detailFixture,
    '/clusters': withData ? clustersWithData : clustersEmpty,
    '/hosts': hostsFixture,
  };
}

async function installRoutes(page, withData) {
  const table = fixturesFor(withData);
  await page.route('**/api/v1/dashboard/home/**', async (route) => {
    const url = new URL(route.request().url());
    const suffix = url.pathname.replace('/api/v1/dashboard/home', '');
    const payload = table[suffix];
    if (payload) {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({error_msg: '', data: payload}),
      });
    } else {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({error_msg: '', data: {}}),
      });
    }
  });
}

async function login(page) {
  await page.goto(`${BASE_URL}/login`, {waitUntil: 'domcontentloaded'});
  // 等 React 水合完成再填：过早 fill 会被受控组件重置为空。/ Wait for hydration
  // before filling — an early fill gets reset by the controlled inputs.
  const username = page.locator('#username');
  await username.waitFor({state: 'visible'});
  await page.waitForLoadState('networkidle');
  await username.fill(USER);
  await page.locator('#password').fill(PASSWORD);
  await page.locator("button[type='submit']").click();
  await page.waitForURL(/\/dashboard/, {timeout: 30_000, waitUntil: 'commit'});
}

async function settle(page) {
  // beUI 图表进入视口才播放生长动画：先滚到底再滚回来。/ beUI charts animate on view;
  // scroll through the page first so every chart has drawn.
  await page.evaluate(async () => {
    await new Promise((resolve) => {
      let y = 0;
      const timer = setInterval(() => {
        y += 600;
        window.scrollTo(0, y);
        if (y >= document.body.scrollHeight) {
          clearInterval(timer);
          resolve();
        }
      }, 80);
    });
  });
  await page.evaluate(() => window.scrollTo(0, 0));
  await page.waitForTimeout(1600);
}

async function shoot(browser, {theme, width, tab, withData, name}) {
  const context = await browser.newContext({
    baseURL: BASE_URL,
    viewport: {width, height: 900},
    deviceScaleFactor: 2,
  });
  await context.addInitScript(
    ({theme: t, skipped}) => {
      window.localStorage.setItem('theme', t);
      window.localStorage.setItem('stx:onboarding:v1:1', skipped);
    },
    {theme, skipped: ONBOARDING_SKIPPED},
  );
  const page = await context.newPage();
  await installRoutes(page, withData);
  await login(page);
  const url = tab === 'jobs' ? '/dashboard?tab=jobs' : '/dashboard';
  await page.goto(url, {waitUntil: 'domcontentloaded'});
  await page.waitForSelector('[data-testid="home-dashboard"]', {timeout: 30_000});
  await page.waitForTimeout(2500);
  await settle(page);
  const file = path.join(OUT_DIR, `${PREFIX}-${name}.png`);
  await page.screenshot({path: file, fullPage: true});
  console.log(`saved ${file}`);
  await context.close();
}

const browser = await chromium.launch();
try {
  for (const theme of ['dark', 'light']) {
    for (const width of WIDTHS) {
      const suffix = WIDTHS.length > 1 ? `-${width}` : '';
      await shoot(browser, {
        theme, width, tab: 'cluster', withData: true,
        name: `${theme}-cluster${suffix}`,
      });
      await shoot(browser, {
        theme, width, tab: 'jobs', withData: true,
        name: `${theme}-jobs${suffix}`,
      });
      await shoot(browser, {
        theme, width, tab: 'jobs', withData: false,
        name: `${theme}-jobs-empty${suffix}`,
      });
    }
  }
} finally {
  await browser.close();
}
