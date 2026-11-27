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

// Kubernetes 主机接入 3 步宽抽屉、双 SA 职责拆分、分层预检阻断及主机详情/安全清理 E2E 用例。
// E2E spec for Kubernetes host onboarding 3-step Drawer, dual-SA split, pre-save probe gates, and safe cleanup.

import {expect, test} from '@playwright/test';

const SAMPLE_KUBECONFIG = `apiVersion: v1
kind: Config
clusters:
- name: dev
  cluster:
    server: https://127.0.0.1:6443
- name: prod
  cluster:
    server: https://10.0.0.10:6443
contexts:
- name: dev
  context:
    cluster: dev
    user: admin
    namespace: seatunnel-dev
- name: prod
  context:
    cluster: prod
    user: admin
    namespace: seatunnel-prod
current-context: dev
users:
- name: admin
  user:
    token: kube-token
`;

const SAMPLE_VALID_JWT = `${'a'.repeat(40)}.${'b'.repeat(80)}.${'c'.repeat(60)}`;

function makeHostRecord(overrides: Record<string, unknown> = {}) {
  return {
    id: 42,
    name: 'k8s-prod',
    host_type: 'kubernetes',
    description: 'Production K8s cluster',
    status: 'connected',
    agent_status: 'not_installed',
    k8s_api_url: 'https://10.0.0.10:6443',
    k8s_namespace: 'seatunnel-full',
    k8s_auth_mode: 'kubeconfig',
    k8s_access_mode: 'in_cluster',
    k8s_permission_mode: 'full',
    k8s_default_service_account: 'seatunnel-runner',
    k8s_credential_expires_at: '2020-01-01T00:00:00Z',
    has_k8s_kubeconfig: true,
    has_k8s_token: false,
    k8s_insecure_skip_tls: false,
    cpu_usage: 0,
    memory_usage: 0,
    disk_usage: 0,
    last_check: null,
    is_online: true,
    created_at: '2026-11-20T08:00:00Z',
    updated_at: '2026-11-20T08:00:00Z',
    ...overrides,
  };
}

test.describe('kubernetes host onboarding & dual-SA split', () => {
  test('walks 3-step K8s drawer, enforces Plan A control-plane YAML, probes before persist, and shows post-save handoff', async ({
    page,
  }) => {
    const callOrder: string[] = [];
    let createPayload: Record<string, unknown> | null = null;
    const savedHost = makeHostRecord({
      id: 101,
      name: 'e2e-k8s-prod',
      k8s_access_mode: 'external',
      k8s_permission_mode: 'minimal',
      k8s_namespace: 'seatunnel-dev',
      k8s_api_url: 'https://10.20.30.40:6443',
      k8s_default_service_account: '',
      k8s_credential_expires_at: '2030-01-01T00:00:00Z',
    });
    const hostList: Array<Record<string, unknown>> = [];

    await page.route('**/api/v1/hosts**', async (route) => {
      const req = route.request();
      const url = new URL(req.url());
      const path = url.pathname;
      const method = req.method();

      if (path === '/api/v1/hosts/k8s/inspect-kubeconfig' && method === 'POST') {
        const body = req.postDataJSON() as {context?: string};
        const selected = body.context || 'dev';
        await route.fulfill({
          json: {
            error_msg: '',
            data: {
              valid: true,
              current_context: 'dev',
              selected_context: selected,
              contexts: [
                {
                  name: 'dev',
                  cluster: 'dev',
                  user: 'admin',
                  namespace: 'seatunnel-dev',
                  server: 'https://127.0.0.1:6443',
                  has_external_files: false,
                  has_exec_plugin: false,
                  has_loopback_server: true,
                  credential_kind: 'token',
                },
                {
                  name: 'prod',
                  cluster: 'prod',
                  user: 'admin',
                  namespace: 'seatunnel-prod',
                  server: 'https://10.0.0.10:6443',
                  has_external_files: false,
                  has_exec_plugin: false,
                  has_loopback_server: false,
                  credential_expires_at: '2030-01-01T00:00:00Z',
                  credential_kind: 'service_account_jwt',
                },
              ],
              warnings: selected === 'prod' ? [] : ['loopback_server'],
            },
          },
        });
        return;
      }

      if (path === '/api/v1/hosts/k8s/test-connection' && method === 'POST') {
        callOrder.push('draft-probe');
        await route.fulfill({
          json: {
            error_msg: '',
            data: {
              connected: true,
              server_version: 'v1.33.9',
              permission_mode: 'minimal',
              control_plane_sa: 'stx-manager',
              can_manage_rbac: false,
              can_list_namespaces: false,
              credential_expires_at: '2030-01-01T00:00:00Z',
              rbac: [
                {
                  group: '',
                  resource: 'pods',
                  verb: 'get',
                  scope: 'required',
                  allowed: true,
                },
                {
                  group: '',
                  resource: 'secrets',
                  verb: 'create',
                  scope: 'optional',
                  allowed: false,
                },
                {
                  group: '',
                  resource: 'namespaces',
                  verb: 'list',
                  scope: 'extended',
                  allowed: false,
                },
              ],
              elapsed_ms: 15,
            },
          },
        });
        return;
      }

      if (path === '/api/v1/hosts' && method === 'POST') {
        callOrder.push('create-host');
        createPayload = req.postDataJSON() as Record<string, unknown>;
        hostList.splice(0, hostList.length, savedHost);
        await route.fulfill({json: {error_msg: '', data: savedHost}});
        return;
      }

      if (path === '/api/v1/hosts/101/test-connection' && method === 'POST') {
        await route.fulfill({
          json: {
            error_msg: '',
            data: {
              connected: true,
              server_version: 'v1.33.9',
              permission_mode: 'minimal',
              control_plane_sa: 'stx-manager',
              rbac: [],
              elapsed_ms: 11,
            },
          },
        });
        return;
      }

      if (path === '/api/v1/hosts/101' && method === 'GET') {
        await route.fulfill({json: {error_msg: '', data: savedHost}});
        return;
      }

      if (path === '/api/v1/hosts/101/k8s/namespaces/seatunnel-dev/objects' && method === 'GET') {
        await route.fulfill({json: {error_msg: '', data: ['default', 'seatunnel-runner']}});
        return;
      }

      if (path === '/api/v1/hosts/101' && method === 'PUT') {
        const body = req.postDataJSON() as Record<string, unknown>;
        const updated = {
          ...savedHost,
          k8s_default_service_account: body.k8s_default_service_account || '',
        };
        await route.fulfill({json: {error_msg: '', data: updated}});
        return;
      }

      if (path === '/api/v1/hosts' && method === 'GET') {
        await route.fulfill({
          json: {error_msg: '', data: {hosts: hostList, total: hostList.length}},
        });
        return;
      }

      await route.continue();
    });

    await page.goto('/hosts');
    await page.getByRole('button', {name: /创建主机|新增主机|Create Host/i}).click();
    await page.getByTestId('host-type-kubernetes').click();

    await expect(page.getByTestId('k8s-step-1')).toBeVisible();
    await expect(page.locator('#k8s_default_service_account')).toHaveCount(0);

    // 切换集群外入口，确认通信路径提示同步变化 / Pick external endpoint & check traffic flow hint
    await page.getByTestId('k8s-access-mode-external').click();
    await expect(
      page.getByText(/API Server（状态与日志）|API Server \(status & logs\)/i),
    ).toBeVisible();

    // 空名称触发行内报错 / Empty host name validates inline
    await page.getByTestId('k8s-step1-next').click();
    await expect(page.locator('#name-error')).toBeVisible();

    await page.locator('#name').fill('e2e-k8s-prod');
    await page.getByTestId('k8s-step1-next').click();

    // Step 2：Plan A 仅单命名空间控制面模板，无 ClusterRole、无 runner SA、无 proxy / Step 2 checks
    await expect(page.getByTestId('k8s-step-2')).toBeVisible();
    const yamlPre = page.getByTestId('k8s-rbac-template-yaml');
    await expect(yamlPre).toContainText('name: stx-manager');
    await expect(yamlPre).not.toContainText('ClusterRole');
    await expect(yamlPre).not.toContainText('seatunnel-runner');
    await expect(yamlPre).not.toContainText('services/proxy');
    await expect(yamlPre).not.toContainText('pods/proxy');

    // 粘贴多上下文回环 Kubeconfig → 警告 → 覆写公网/内网可达地址后警告消失 / Kubeconfig inspect + override
    await page.locator('#k8s_kubeconfig').fill(SAMPLE_KUBECONFIG);
    await expect(page.getByTestId('create-kubeconfig-warnings')).toBeVisible();
    await page
      .getByTestId('create-kubeconfig-server-override')
      .fill('https://10.20.30.40:6443');
    await expect(page.getByTestId('create-kubeconfig-warnings')).toHaveCount(0);

    // Step 3：先无状态预检再落库 / Step 3 pre-save probe gates persist
    await page.getByTestId('k8s-step2-next').click();
    await expect(page.getByTestId('k8s-guide-stage-list')).toBeVisible();
    await expect(page.getByTestId('k8s-guide-draft-notice')).toBeVisible();
    await expect(page.getByTestId('k8s-connect-guide')).toContainText(/核心 1\/1|Required 1\/1/);
    expect(callOrder).toEqual(['draft-probe']);

    const saveBtn = page.getByTestId('k8s-guide-save-host');
    await expect(saveBtn).toBeEnabled();
    await saveBtn.click();

    await expect(page.getByTestId('k8s-guide-saved-banner')).toBeVisible();
    await expect(page.getByTestId('k8s-guide-engine-sa-card')).toBeVisible();
    await page.getByTestId('k8s-engine-sa-bind-btn').click();
    await expect(page.getByTestId('k8s-guide-saved-engine-sa-chip')).toContainText('seatunnel-runner');
    await expect(page.getByTestId('k8s-guide-deploy-cluster')).toBeVisible();
    expect(callOrder).toEqual(['draft-probe', 'create-host']);
    expect(createPayload).toBeTruthy();
    expect(createPayload).not.toHaveProperty('k8s_default_service_account');
  });

  test('blocks persistence on 401 unauthorized and 403 insufficient RBAC probes', async ({
    page,
  }) => {
    let probeBehavior: 'unauthorized' | 'insufficient' = 'unauthorized';
    let createCalled = false;

    await page.route('**/api/v1/hosts**', async (route) => {
      const req = route.request();
      const path = new URL(req.url()).pathname;
      const method = req.method();

      if (path === '/api/v1/hosts/k8s/test-connection' && method === 'POST') {
        if (probeBehavior === 'unauthorized') {
          await route.fulfill({
            json: {
              error_msg: '',
              data: {
                connected: false,
                error_code: 'unauthorized',
                message: 'Unauthorized token rejected by cluster',
                rbac: [],
                elapsed_ms: 9,
              },
            },
          });
          return;
        }
        await route.fulfill({
          json: {
            error_msg: '',
            data: {
              connected: true,
              server_version: 'v1.33.9',
              permission_mode: 'insufficient',
              rbac: [
                {
                  group: 'apps',
                  resource: 'deployments',
                  verb: 'create',
                  scope: 'required',
                  allowed: false,
                },
              ],
              elapsed_ms: 12,
            },
          },
        });
        return;
      }

      if (path === '/api/v1/hosts' && method === 'POST') {
        createCalled = true;
        await route.fulfill({status: 500, json: {error_msg: 'should not save', data: null}});
        return;
      }

      if (path === '/api/v1/hosts' && method === 'GET') {
        await route.fulfill({json: {error_msg: '', data: {hosts: [], total: 0}}});
        return;
      }

      await route.continue();
    });

    await page.goto('/hosts');
    await page.getByRole('button', {name: /创建主机|新增主机|Create Host/i}).click();
    await page.getByTestId('host-type-kubernetes').click();

    await page.locator('#name').fill('e2e-k8s-negative');
    await page.getByTestId('k8s-prep-mode-existing').click();
    await page.getByTestId('k8s-step1-next').click();

    // 默认已折叠模板；切 Token 并验证 401 阻断保存 / Prep=existing starts collapsed; Token mode 401 blocks save
    await expect(page.getByTestId('k8s-rbac-template-panel')).toHaveCount(0);
    await page.getByRole('button', {name: 'Token'}).click();
    await page.locator('#k8s_api_url').fill('https://10.0.0.20:6443');
    await page.locator('#k8s_token').fill(SAMPLE_VALID_JWT);
    await page.getByTestId('k8s-step2-next').click();

    await expect(page.getByText(/401/)).toBeVisible();
    await expect(page.getByTestId('k8s-guide-save-host')).toBeDisabled();

    // 返回修改后保留表单值，切为 403 权限不足再进入预检 / Back retains inputs; 403 auto-opens template and blocks save
    await page.getByTestId('k8s-guide-back-edit').click();
    await expect(page.locator('#k8s_api_url')).toHaveValue('https://10.0.0.20:6443');
    probeBehavior = 'insufficient';
    await page.getByTestId('k8s-step2-next').click();

    await expect(page.getByTestId('k8s-rbac-template-panel')).toBeVisible();
    await expect(page.getByTestId('k8s-guide-save-host')).toBeDisabled();
    expect(createCalled).toBe(false);
  });

  test('separates control-plane and engine SA in HostDetail and requires explicit opt-in to clean runner SA on delete', async ({
    page,
  }) => {
    const k8sHost = makeHostRecord();

    await page.route('**/api/v1/hosts**', async (route) => {
      const req = route.request();
      const path = new URL(req.url()).pathname;
      const method = req.method();

      if (path === '/api/v1/hosts' && method === 'GET') {
        await route.fulfill({
          json: {error_msg: '', data: {hosts: [k8sHost], total: 1}},
        });
        return;
      }

      if (path === '/api/v1/hosts/42' && method === 'GET') {
        await route.fulfill({json: {error_msg: '', data: k8sHost}});
        return;
      }

      await route.continue();
    });

    await page.goto('/hosts');
    const row = page.getByTestId('host-row-42');
    await expect(row).toBeVisible();
    await row.click();

    // 详情侧栏：验证过期提醒、独立引擎运行身份区块与纯 runner YAML / Detail rail dual-SA split
    await expect(page.getByTestId('host-detail-k8s-expiry')).toContainText(/已过期|Expired/i);
    const engineSection = page.getByTestId('host-detail-engine-sa-section');
    await expect(engineSection).toBeVisible();
    await engineSection.getByTestId('k8s-engine-sa-template-toggle').click();
    const engineYaml = engineSection.getByTestId('k8s-engine-sa-template-yaml');
    await expect(engineYaml).toContainText('name: seatunnel-runner');
    await expect(engineYaml).not.toContainText('stx-manager');

    // 轮换凭据弹窗需先探测才解锁保存 / Rotation dialog locks apply before test
    await page.getByTestId('k8s-rotate-credential-btn').click();
    await expect(page.getByTestId('k8s-rotate-dialog')).toBeVisible();
    await expect(page.getByTestId('k8s-rotate-apply-btn')).toBeDisabled();
    await page.keyboard.press('Escape');

    // 行菜单删除：默认只清 stx-manager，勾选后才清 seatunnel-runner / Delete cleanup defaults safe
    await row.locator('button[aria-haspopup="menu"]').click();
    await page.getByRole('menuitem', {name: /删除|Delete/i}).click();
    const cleanupPanel = page.getByTestId('k8s-delete-cleanup-panel');
    await expect(cleanupPanel).toContainText('stx-manager-binding');
    await expect(cleanupPanel).toContainText('stx-manager-cluster-binding');
    await expect(cleanupPanel).not.toContainText('seatunnel-runner');

    await page.getByTestId('k8s-delete-include-runner').click();
    await expect(cleanupPanel).toContainText(
      'kubectl -n seatunnel-full delete serviceaccount seatunnel-runner',
    );
  });
});
