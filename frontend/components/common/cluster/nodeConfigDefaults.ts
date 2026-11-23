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

import {ConfigInfo, ConfigType} from '@/lib/services/config';
import {ClusterConfig, DeploymentMode, getClusterJVMConfig} from '@/lib/services/cluster/types';

function normalizeHeapSizeToMB(rawValue: string, unit: string): number | undefined {
  const parsed = Number(rawValue);
  if (!Number.isFinite(parsed) || parsed <= 0) {
    return undefined;
  }

  switch (unit.toLowerCase()) {
    case 'g':
      return Math.ceil(parsed * 1024);
    case 'm':
      return Math.ceil(parsed);
    case 'k':
      return Math.max(1, Math.ceil(parsed / 1024));
    default:
      return undefined;
  }
}

function parseHeapSizeFromJVMContent(content?: string): number | undefined {
  if (!content) {
    return undefined;
  }

  const lines = content.split(/\r?\n/);
  for (const line of lines) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith('#')) {
      continue;
    }

    const match = trimmed.match(/^-Xmx(\d+(?:\.\d+)?)([gGmMkK])$/);
    if (!match) {
      continue;
    }

    return normalizeHeapSizeToMB(match[1], match[2]);
  }

  return undefined;
}

function findConfigContent(configs: ConfigInfo[], configType: ConfigType): string | undefined {
  return (
    configs.find((config) => config.config_type === configType && config.is_template)?.content ||
    configs.find((config) => config.config_type === configType)?.content
  );
}

export function buildResolvedClusterConfigFromConfigs(
  clusterConfig: ClusterConfig | undefined,
  deploymentMode: DeploymentMode,
  configs: ConfigInfo[],
): ClusterConfig | undefined {
  const existingJVM = getClusterJVMConfig(clusterConfig);
  if (existingJVM) {
    return clusterConfig;
  }

  const resolvedJVM =
    deploymentMode === DeploymentMode.HYBRID
      ? {
          hybrid_heap_size: parseHeapSizeFromJVMContent(
            findConfigContent(configs, ConfigType.JVM_OPTIONS),
          ),
        }
      : {
          master_heap_size: parseHeapSizeFromJVMContent(
            findConfigContent(configs, ConfigType.JVM_MASTER_OPTIONS),
          ),
          worker_heap_size: parseHeapSizeFromJVMContent(
            findConfigContent(configs, ConfigType.JVM_WORKER_OPTIONS),
          ),
        };

  if (Object.values(resolvedJVM).every((value) => value === undefined)) {
    return clusterConfig;
  }

  return {
    ...(clusterConfig ?? {}),
    jvm: {
      ...(clusterConfig?.jvm && typeof clusterConfig.jvm === 'object'
        ? (clusterConfig.jvm as Record<string, unknown>)
        : {}),
      ...resolvedJVM,
    },
  };
}
