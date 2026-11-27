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
  looksTruncatedK8sToken,
  renderFullK8sRbacYaml,
  renderK8sExportKubeconfigCommand,
  renderK8sGenerateKubeconfigCommand,
  renderK8sTokenCommand,
  renderMinimalK8sRbacYaml,
  sanitizeK8sToken,
} from '../k8s-rbac-templates';

describe('k8s-rbac-templates', () => {
  it('renderMinimalK8sRbacYaml isolates control-plane manager SA and runner SA inside one namespace', () => {
    const yaml = renderMinimalK8sRbacYaml('seatunnel-prod', 'custom-runner', 'ops-stx');
    expect(yaml).not.toContain('ClusterRole');
    expect(yaml).toContain('name: ops-stx');
    expect(yaml).toContain('name: ops-stx-role');
    expect(yaml).toContain('name: ops-stx-binding');
    expect(yaml).toContain('name: custom-runner');
    expect(yaml).toContain('namespace: seatunnel-prod');
    expect(yaml).toContain('name: custom-runner-role');
    expect(yaml).toContain('resources: ["endpoints", "pods", "services"]');
  });

  it('renderMinimalK8sRbacYaml defaults blank inputs to seatunnel, seatunnel-runner, and stx-manager', () => {
    const yaml = renderMinimalK8sRbacYaml('  ', '', ' ');
    expect(yaml).toContain('namespace: seatunnel');
    expect(yaml).toContain('name: stx-manager');
    expect(yaml).toContain('name: seatunnel-runner');
  });

  it('renderFullK8sRbacYaml keeps ClusterRole, Binding, and roleRef in lockstep for custom manager SA', () => {
    const yaml = renderFullK8sRbacYaml('seatunnel-full', 'stx-manager-full');
    expect(yaml).toContain('kind: ClusterRole');
    expect(yaml).toContain('kind: ClusterRoleBinding');
    expect(yaml).toContain('name: stx-manager-full-cluster-role');
    expect(yaml).toContain('name: stx-manager-full-cluster-binding');
    expect(yaml).not.toContain('name: stx-manager-cluster-role');
    expect(yaml).toContain('resources: ["namespaces"]');
    expect(yaml).toContain('resources: ["endpoints"]');
    expect(yaml).toContain('resources: ["roles", "rolebindings"]');
  });

  it('renders token and kubeconfig commands with custom manager SA', () => {
    expect(renderK8sTokenCommand('ns-a', 'ops-stx')).toBe(
      'kubectl -n ns-a create token ops-stx --duration=8760h',
    );
    expect(renderK8sExportKubeconfigCommand()).toBe(
      'kubectl config view --raw --flatten --minify',
    );
    const kcCmd = renderK8sGenerateKubeconfigCommand('ns-a', 'ops-stx');
    expect(kcCmd).toContain('create token ops-stx --duration=8760h');
    expect(kcCmd).toContain('certificate-authority-data');
    expect(kcCmd).toContain('namespace: ns-a');
  });

  it('sanitizes wrapped tokens and flags truncated tokens', () => {
    const validJwt = `${'a'.repeat(40)}.${'b'.repeat(80)}.${'c'.repeat(60)}`;
    expect(sanitizeK8sToken(`  ${validJwt.slice(0, 50)}\n${validJwt.slice(50)}\r\n`)).toBe(
      validJwt,
    );
    expect(looksTruncatedK8sToken(validJwt)).toBe(false);
    expect(looksTruncatedK8sToken('only-signature-segment')).toBe(true);
    expect(looksTruncatedK8sToken('')).toBe(false);
  });
});
