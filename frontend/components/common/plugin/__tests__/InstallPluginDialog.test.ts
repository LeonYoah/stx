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

import {describe, expect, it} from 'vitest';
import {
  getClusterDisplayState,
  isClusterRuntimeAvailable,
  shouldNotifyParentDownloadComplete,
} from '../InstallPluginDialog';
import {
  ClusterStatus,
  type ClusterInfo,
  type DeploymentMode,
} from '@/lib/services/cluster';

function createCluster(overrides: Partial<ClusterInfo>): ClusterInfo {
  return {
    id: 1,
    name: 't13',
    description: '',
    deployment_mode: 'hybrid' as DeploymentMode,
    version: '2.3.13',
    status: ClusterStatus.RUNNING,
    install_dir: '/opt/seatunnel',
    config: {},
    node_count: 1,
    online_nodes: 1,
    health_status: 'healthy',
    created_at: '',
    updated_at: '',
    ...overrides,
  };
}

describe('shouldNotifyParentDownloadComplete', () => {
  it('gates parent notify on in-session downloading observation', () => {
    expect(shouldNotifyParentDownloadComplete(false)).toBe(false);
    expect(shouldNotifyParentDownloadComplete(true)).toBe(true);
  });
});

describe('InstallPluginDialog cluster runtime state', () => {
  it('marks running but unhealthy clusters as unavailable', () => {
    const cluster = createCluster({
      online_nodes: 0,
      health_status: 'unhealthy',
    });

    expect(isClusterRuntimeAvailable(cluster)).toBe(false);
    expect(getClusterDisplayState(cluster)).toEqual({
      variant: 'destructive',
      labelKey: 'cluster.healthStatuses.unhealthy',
    });
  });

  it('keeps healthy running clusters available', () => {
    const cluster = createCluster({});

    expect(isClusterRuntimeAvailable(cluster)).toBe(true);
    expect(getClusterDisplayState(cluster)).toEqual({
      variant: 'default',
      labelKey: 'cluster.statuses.running',
    });
  });
});
