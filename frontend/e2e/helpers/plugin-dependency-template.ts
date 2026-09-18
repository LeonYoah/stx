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

import fs from 'node:fs/promises';
import type {Page} from '@playwright/test';

function ok<T>(data: T) {
  return {
    error_msg: '',
    data,
  };
}

type DependencyItem = {
  id: number;
  group_id: string;
  artifact_id: string;
  version: string;
  target_dir: string;
  required: boolean;
  source_url: string;
  note: string;
  disabled?: boolean;
  disable_id?: number;
  created_at: string;
  updated_at: string;
};

type CustomDependency = {
  id: number;
  plugin_name: string;
  seatunnel_version: string;
  group_id: string;
  artifact_id: string;
  version: string;
  target_dir: string;
  source_type: 'upload';
  original_file_name: string;
  created_at: string;
  updated_at: string;
};

const now = '2026-03-20T10:00:00Z';
const pluginName = 'jdbc';
const seatunnelVersion = '2.3.13';
const uploadFileName = 'mysql-connector-j-8.0.33.jar';
const uploadArtifactId = 'mysql-connector-j';

const mysqlConnectorItem: DependencyItem = {
  id: 11,
  group_id: 'org.apache.seatunnel',
  artifact_id: 'connector-cdc-mysql',
  version: '2.3.13',
  target_dir: 'connectors',
  required: true,
  source_url: 'https://seatunnel.apache.org/docs/2.3.13/connectors/sink/Mysql/',
  note: 'MySQL CDC companion connector',
  created_at: now,
  updated_at: now,
};

const mysqlDriverItem: DependencyItem = {
  id: 12,
  group_id: 'com.mysql',
  artifact_id: uploadArtifactId,
  version: '8.0.33',
  target_dir: 'plugins/Jdbc/lib',
  required: true,
  source_url:
    'https://repo1.maven.org/maven2/com/mysql/mysql-connector-j/8.0.33/',
  note: 'MySQL runtime driver',
  created_at: now,
  updated_at: now,
};

const postgresDriverItem: DependencyItem = {
  id: 21,
  group_id: 'org.postgresql',
  artifact_id: 'postgresql',
  version: '42.4.3',
  target_dir: 'plugins/Jdbc/lib',
  required: true,
  source_url:
    'https://repo1.maven.org/maven2/org/postgresql/postgresql/42.4.3/',
  note: 'PostgreSQL runtime driver',
  created_at: now,
  updated_at: now,
};

const pluginFixture = {
  name: pluginName,
  display_name: 'JDBC',
  category: 'connector',
  version: seatunnelVersion,
  description:
    'Universal JDBC connector with profile-based dependency baseline.',
  group_id: 'org.apache.seatunnel',
  artifact_id: 'connector-jdbc',
  doc_url: 'https://seatunnel.apache.org/docs/2.3.13/connectors/sink/Jdbc/',
  dependency_status: 'ready_exact',
  dependency_count: 3,
  dependency_baseline_version: seatunnelVersion,
  dependency_resolution_mode: 'exact',
};

type PluginDependencyFixtureState = {
  localPluginDeleted: boolean;
  installedPluginNames: Set<string>;
  disabledDependencies: Array<{
    id: number;
    plugin_name: string;
    seatunnel_version: string;
    group_id: string;
    artifact_id: string;
    version: string;
    target_dir: string;
    created_at: string;
    updated_at: string;
  }>;
  customDependencies: CustomDependency[];
  nextDisableId: number;
  nextCustomDependencyId: number;
};

function createInitialState(): PluginDependencyFixtureState {
  return {
    localPluginDeleted: false,
    installedPluginNames: new Set([pluginName]),
    disabledDependencies: [],
    customDependencies: [],
    nextDisableId: 300,
    nextCustomDependencyId: 700,
  };
}

function buildOfficialDependenciesResponse(
  state: PluginDependencyFixtureState,
) {
  const disabledByArtifact = new Map(
    state.disabledDependencies.map((item) => [item.artifact_id, item]),
  );

  const markDisabled = (item: DependencyItem) => {
    const disabled = disabledByArtifact.get(item.artifact_id);
    return disabled
      ? {
          ...item,
          disabled: true,
          disable_id: disabled.id,
        }
      : item;
  };

  return {
    plugin_name: pluginName,
    seatunnel_version: seatunnelVersion,
    dependency_status: 'ready_exact',
    dependency_count: 3,
    baseline_version_used: seatunnelVersion,
    dependency_resolution_mode: 'exact',
    profiles: [
      {
        id: 1,
        seatunnel_version: seatunnelVersion,
        plugin_name: pluginName,
        artifact_id: 'connector-jdbc',
        profile_key: 'mysql',
        profile_name: 'MySQL 8.x',
        engine_scope: 'zeta',
        target_dir: 'plugins/Jdbc/lib',
        applies_to: 'mysql',
        include_versions: '',
        excluded_versions: '',
        source_kind: 'official',
        baseline_version_used: seatunnelVersion,
        doc_slug: 'connectors/sink/Mysql',
        doc_source_url:
          'https://seatunnel.apache.org/docs/2.3.13/connectors/sink/Mysql/',
        confidence: 'high',
        no_additional_dependencies: false,
        is_default: true,
        note: 'Default profile for MySQL sink/source.',
        content_hash: 'mysql-profile',
        created_at: now,
        updated_at: now,
        items: [
          markDisabled(mysqlConnectorItem),
          markDisabled(mysqlDriverItem),
        ],
      },
      {
        id: 2,
        seatunnel_version: seatunnelVersion,
        plugin_name: pluginName,
        artifact_id: 'connector-jdbc',
        profile_key: 'postgres',
        profile_name: 'PostgreSQL',
        engine_scope: 'zeta',
        target_dir: 'plugins/Jdbc/lib',
        applies_to: 'postgresql',
        include_versions: '',
        excluded_versions: '',
        source_kind: 'official',
        baseline_version_used: seatunnelVersion,
        doc_slug: 'connectors/sink/Postgres',
        doc_source_url:
          'https://seatunnel.apache.org/docs/2.3.13/connectors/sink/Postgres/',
        confidence: 'high',
        no_additional_dependencies: false,
        is_default: false,
        note: 'Alternative profile for PostgreSQL.',
        content_hash: 'postgres-profile',
        created_at: now,
        updated_at: now,
        items: [markDisabled(postgresDriverItem)],
      },
    ],
    effective_dependencies: [],
    disabled_dependencies: state.disabledDependencies,
  };
}

/**
 * 构造本地插件列表夹具响应
 * Build local plugin list fixture response
 */
function buildLocalPluginResponse(state: PluginDependencyFixtureState) {
  if (state.localPluginDeleted) {
    return [];
  }

  return [
    {
      name: pluginName,
      artifact_id: 'connector-jdbc',
      version: seatunnelVersion,
      category: 'connector',
      connector_path: '/tmp/stx/plugins/connector-jdbc-2.3.13.jar',
      size: 1024,
      downloaded_at: now,
      selected_profile_keys: ['mysql'],
      attached_connectors: [],
      dependencies: [mysqlDriverItem],
    },
  ];
}

/**
 * 构造集群列表夹具响应项
 * Build cluster list fixture response item
 */
function buildClusterResponse() {
  return {
    id: 101,
    name: 'template-cluster',
    description: 'Template cluster for plugin delete flow',
    deployment_mode: 'hybrid',
    version: seatunnelVersion,
    status: 'running',
    install_dir: '/tmp/seatunnel',
    config: {},
    node_count: 1,
    online_nodes: 1,
    health_status: 'healthy',
    created_at: now,
    updated_at: now,
  };
}

/**
 * 构造已安装插件夹具响应
 * Build installed plugins fixture response
 */
function buildInstalledPluginsResponse(state: PluginDependencyFixtureState) {
  if (!state.installedPluginNames.has(pluginName)) {
    return [];
  }

  return [
    {
      id: 501,
      cluster_id: 101,
      plugin_name: pluginName,
      artifact_id: 'connector-jdbc',
      category: 'connector',
      version: seatunnelVersion,
      status: 'installed',
      install_path: '/tmp/seatunnel/connectors/connector-jdbc-2.3.13.jar',
      installed_at: now,
      updated_at: now,
      selected_profile_keys: ['mysql'],
      attached_connectors: [],
      dependencies: [mysqlDriverItem],
    },
  ];
}

export async function installPluginDependencyTemplateRoutes(
  page: Page,
): Promise<void> {
  const state = createInitialState();

  await page.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const {pathname} = url;

    if (request.method() === 'GET' && pathname === '/api/v1/packages') {
      await route.fulfill({
        status: 200,
        json: ok({
          versions: [seatunnelVersion],
          recommended_version: seatunnelVersion,
          local_packages: [],
        }),
      });
      return;
    }

    if (
      request.method() === 'GET' &&
      pathname === '/api/v1/packages/downloads'
    ) {
      await route.fulfill({
        status: 200,
        json: ok([]),
      });
      return;
    }

    if (request.method() === 'GET' && pathname === '/api/v1/plugins') {
      await route.fulfill({
        status: 200,
        json: ok({
          plugins: [pluginFixture],
          total: 1,
          version: seatunnelVersion,
          mirror: 'aliyun',
          source: 'database',
          cache_hit: true,
          catalog_refreshed_at: now,
        }),
      });
      return;
    }

    if (request.method() === 'GET' && pathname === '/api/v1/plugins/local') {
      await route.fulfill({
        status: 200,
        json: ok(buildLocalPluginResponse(state)),
      });
      return;
    }

    if (
      request.method() === 'DELETE' &&
      pathname === `/api/v1/plugins/${pluginName}/local`
    ) {
      state.localPluginDeleted = true;
      await route.fulfill({
        status: 200,
        json: ok(null),
      });
      return;
    }

    if (
      request.method() === 'GET' &&
      pathname === '/api/v1/plugins/downloads'
    ) {
      await route.fulfill({
        status: 200,
        json: ok([]),
      });
      return;
    }

    if (request.method() === 'GET' && pathname === '/api/v1/clusters') {
      await route.fulfill({
        status: 200,
        json: ok({
          total: 1,
          clusters: [buildClusterResponse()],
        }),
      });
      return;
    }

    if (
      request.method() === 'GET' &&
      pathname === '/api/v1/clusters/101/plugins'
    ) {
      await route.fulfill({
        status: 200,
        json: ok(buildInstalledPluginsResponse(state)),
      });
      return;
    }

    if (
      request.method() === 'DELETE' &&
      pathname === `/api/v1/clusters/101/plugins/${pluginName}`
    ) {
      state.installedPluginNames.delete(pluginName);
      await route.fulfill({
        status: 200,
        json: ok(null),
      });
      return;
    }

    if (
      request.method() === 'GET' &&
      pathname === `/api/v1/plugins/${pluginName}/official-dependencies`
    ) {
      await route.fulfill({
        status: 200,
        json: ok(buildOfficialDependenciesResponse(state)),
      });
      return;
    }

    if (
      request.method() === 'POST' &&
      pathname === `/api/v1/plugins/${pluginName}/dependencies/disables`
    ) {
      const payload = request.postDataJSON() as {
        group_id: string;
        artifact_id: string;
        version: string;
        target_dir: string;
      };
      const created = {
        id: state.nextDisableId,
        plugin_name: pluginName,
        seatunnel_version: seatunnelVersion,
        group_id: payload.group_id,
        artifact_id: payload.artifact_id,
        version: payload.version,
        target_dir: payload.target_dir,
        created_at: now,
        updated_at: now,
      };
      state.nextDisableId += 1;
      state.disabledDependencies = [...state.disabledDependencies, created];
      await route.fulfill({
        status: 200,
        json: ok(created),
      });
      return;
    }

    if (
      request.method() === 'DELETE' &&
      pathname.startsWith(
        `/api/v1/plugins/${pluginName}/dependencies/disables/`,
      )
    ) {
      const disableId = Number(pathname.split('/').pop());
      state.disabledDependencies = state.disabledDependencies.filter(
        (item) => item.id !== disableId,
      );
      await route.fulfill({
        status: 200,
        json: ok(null),
      });
      return;
    }

    if (
      request.method() === 'GET' &&
      pathname === `/api/v1/plugins/${pluginName}/dependencies`
    ) {
      await route.fulfill({
        status: 200,
        json: ok(state.customDependencies),
      });
      return;
    }

    if (
      request.method() === 'POST' &&
      pathname === `/api/v1/plugins/${pluginName}/dependencies/upload`
    ) {
      const created: CustomDependency = {
        id: state.nextCustomDependencyId,
        plugin_name: pluginName,
        seatunnel_version: seatunnelVersion,
        group_id: 'com.mysql',
        artifact_id: uploadArtifactId,
        version: '8.0.33',
        target_dir: 'plugins/Jdbc/lib',
        source_type: 'upload',
        original_file_name: uploadFileName,
        created_at: now,
        updated_at: now,
      };
      state.nextCustomDependencyId += 1;
      state.customDependencies = [...state.customDependencies, created];
      await route.fulfill({
        status: 200,
        json: ok(created),
      });
      return;
    }

    if (
      request.method() === 'DELETE' &&
      pathname.startsWith(`/api/v1/plugins/${pluginName}/dependencies/`)
    ) {
      const dependencyId = Number(pathname.split('/').pop());
      state.customDependencies = state.customDependencies.filter(
        (item) => item.id !== dependencyId,
      );
      await route.fulfill({
        status: 200,
        json: ok(null),
      });
      return;
    }

    await route.continue();
  });
}

export async function createPlaceholderJar(
  outputPath: string,
): Promise<string> {
  await fs.writeFile(outputPath, 'placeholder jar for e2e upload flow\n');
  return outputPath;
}

export const pluginDependencyTemplate = {
  pluginName,
  seatunnelVersion,
  mysqlProfileLabel: 'MySQL 8.x',
  mysqlConnectorArtifactId: mysqlConnectorItem.artifact_id,
  mysqlDriverArtifactId: mysqlDriverItem.artifact_id,
  uploadFileName,
};
