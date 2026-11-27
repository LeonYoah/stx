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
  renderControlPlaneK8sRbacYaml,
  renderK8sCleanupCommand,
  renderK8sExportKubeconfigCommand,
  renderK8sGenerateKubeconfigCommand,
  renderK8sTokenCommand,
  renderSeaTunnelRunnerK8sRbacYaml,
  sanitizeK8sToken,
} from '../k8s-rbac-templates';

describe('k8s-rbac-templates', () => {
  it('renderControlPlaneK8sRbacYaml emits pure single-namespace control-plane RBAC with zero runner SA, ClusterRole, or proxy rules', () => {
    const yaml = renderControlPlaneK8sRbacYaml('seatunnel-prod', 'ops-stx');
    expect(yaml).not.toContain('ClusterRole');
    expect(yaml).not.toContain('seatunnel-runner');
    expect(yaml).toContain('name: ops-stx');
    expect(yaml).toContain('name: ops-stx-role');
    expect(yaml).toContain('name: ops-stx-binding');
    expect(yaml).toContain('namespace: seatunnel-prod');
    expect(yaml).not.toContain('services/proxy');
    expect(yaml).not.toContain('pods/proxy');
    expect(yaml).not.toContain('"delete"');
    expect(yaml).not.toContain('resources: ["secrets"]');
    expect(yaml).not.toContain('resources: ["jobs"]');
    expect(yaml).not.toContain('resources: ["events"]');
  });

  it('renderControlPlaneK8sRbacYaml honors optional delete/secret toggles and never includes proxy or runner SA', () => {
    const expanded = renderControlPlaneK8sRbacYaml('ns-a', 'stx-mgr', {
      includeDeleteRules: true,
      includeSecretRules: true,
    });
    expect(expanded).not.toContain('seatunnel-runner');
    expect(expanded).not.toContain('services/proxy');
    expect(expanded).not.toContain('pods/proxy');
    expect(expanded).toContain('verbs: ["delete"]');
    expect(expanded).toContain('resources: ["secrets"]');
  });

  it('renderSeaTunnelRunnerK8sRbacYaml emits standalone read-only runner RBAC isolated from control-plane rules', () => {
    const yaml = renderSeaTunnelRunnerK8sRbacYaml('seatunnel-prod', 'custom-runner');
    expect(yaml).not.toContain('ClusterRole');
    expect(yaml).not.toContain('stx-manager');
    expect(yaml).toContain('name: custom-runner');
    expect(yaml).toContain('name: custom-runner-role');
    expect(yaml).toContain('name: custom-runner-binding');
    expect(yaml).toContain('namespace: seatunnel-prod');
    expect(yaml).toContain('resources: ["endpoints", "pods", "services"]');
    expect(yaml).toContain('verbs: ["get", "list", "watch"]');
    expect(yaml).not.toContain('deployments');
    expect(yaml).not.toContain('statefulsets');
    expect(yaml).not.toContain('secrets');
    expect(yaml).not.toContain('"delete"');
  });

  it('defaults blank inputs to seatunnel, stx-manager, and seatunnel-runner', () => {
    const cpYaml = renderControlPlaneK8sRbacYaml('  ', ' ');
    expect(cpYaml).toContain('namespace: seatunnel');
    expect(cpYaml).toContain('name: stx-manager');
    const runnerYaml = renderSeaTunnelRunnerK8sRbacYaml('', '  ');
    expect(runnerYaml).toContain('namespace: seatunnel');
    expect(runnerYaml).toContain('name: seatunnel-runner');
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

  it('renderK8sCleanupCommand omits runner SA by default and appends clusterrole or runner only when opted in', () => {
    const defaultCleanup = renderK8sCleanupCommand('seatunnel-min', 'stx-manager');
    expect(defaultCleanup).toContain('kubectl -n seatunnel-min delete rolebinding stx-manager-binding');
    expect(defaultCleanup).not.toContain('seatunnel-runner');
    expect(defaultCleanup).not.toContain('clusterrolebinding');

    const fullAndRunnerCleanup = renderK8sCleanupCommand('seatunnel-full', 'ops-stx', {
      includeClusterRole: true,
      runnerServiceAccount: 'custom-runner',
      includeRunner: true,
    });
    expect(fullAndRunnerCleanup).toContain('kubectl delete clusterrolebinding ops-stx-cluster-binding');
    expect(fullAndRunnerCleanup).toContain('kubectl delete clusterrole ops-stx-cluster-role');
    expect(fullAndRunnerCleanup).toContain('kubectl -n seatunnel-full delete serviceaccount custom-runner');
  });
});
