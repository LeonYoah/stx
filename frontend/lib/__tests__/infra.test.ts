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

// 全局部署模式 helper 契约测试：验证 Standalone / K8s Session / K8s Application / 预留 YARN 分类与防误判。
// Contract tests for global deployment mode helpers covering Standalone, K8s Session/App, and reserved YARN modes.

import {describe, expect, it} from 'vitest';
import {
  getClusterDeployMode,
  getClusterDeployModeLabel,
  getClusterExecutionMode,
  getRuntimePlatform,
  isApplicationInfra,
  isBareMetalInfra,
  isK8sInfra,
  isK8sSessionInfra,
  isSessionInfra,
  isYarnInfra,
} from '../infra';

describe('lib/infra deployment mode contract', () => {
  it('classifies empty and bare_metal as Standalone Session', () => {
    for (const value of [undefined, null, '', 'bare_metal']) {
      expect(isBareMetalInfra(value)).toBe(true);
      expect(isK8sInfra(value)).toBe(false);
      expect(isYarnInfra(value)).toBe(false);
      expect(isSessionInfra(value)).toBe(true);
      expect(isApplicationInfra(value)).toBe(false);
      expect(getRuntimePlatform(value)).toBe('standalone');
      expect(getClusterExecutionMode(value)).toBe('session');
      expect(getClusterDeployMode(value)).toBe('standalone');
      expect(getClusterDeployModeLabel(value)).toBe('Standalone');
    }
  });

  it('classifies legacy kubernetes and kubernetes_session as Kubernetes Session', () => {
    for (const value of ['kubernetes', 'kubernetes_session']) {
      expect(isK8sInfra(value)).toBe(true);
      expect(isK8sSessionInfra(value)).toBe(true);
      expect(isBareMetalInfra(value)).toBe(false);
      expect(isSessionInfra(value)).toBe(true);
      expect(getClusterDeployMode(value)).toBe('kubernetes_session');
      expect(getClusterDeployModeLabel(value)).toBe('Kubernetes Session');
    }
  });

  it('classifies kubernetes_application_* as Kubernetes Application', () => {
    for (const value of [
      'kubernetes_application_template',
      'kubernetes_application_instance',
    ]) {
      expect(isK8sInfra(value)).toBe(true);
      expect(isK8sSessionInfra(value)).toBe(false);
      expect(isApplicationInfra(value)).toBe(true);
      expect(getClusterExecutionMode(value)).toBe('application');
      expect(getClusterDeployMode(value)).toBe('kubernetes_application');
      expect(getClusterDeployModeLabel(value)).toBe('Kubernetes Application');
    }
  });

  it('never misclassifies future YARN infra types as Kubernetes', () => {
    expect(isK8sInfra('yarn_session')).toBe(false);
    expect(isYarnInfra('yarn_session')).toBe(true);
    expect(getClusterDeployMode('yarn_session')).toBe('yarn_session');
    expect(getClusterDeployModeLabel('yarn_session')).toBe('YARN Session');

    expect(isK8sInfra('yarn_application_instance')).toBe(false);
    expect(isYarnInfra('yarn_application_instance')).toBe(true);
    expect(isApplicationInfra('yarn_application_instance')).toBe(true);
    expect(getClusterDeployMode('yarn_application_instance')).toBe(
      'yarn_application',
    );
    expect(getClusterDeployModeLabel('yarn_application_instance')).toBe(
      'YARN Application',
    );
  });
});
