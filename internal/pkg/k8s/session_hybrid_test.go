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

package k8s

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/fake"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func hybridSessionConfig() *SessionConfig {
	cfg := &SessionConfig{
		HostID:             1,
		Namespace:          "e2e-ns",
		Image:              "apache/seatunnel:3.1.0",
		DeploymentMode:     "hybrid",
		HybridReplicas:     2,
		MasterHTTPPort:     8080,
		HazelcastPort:      5801,
		HybridWorkloadName: "e2e-master-hybrid",
		ManagementSource:   ManagementSourceSTXDeployed,
	}
	cfg.ApplyDefaults()
	return cfg
}

// TestDeployHybridMasterServiceSelectsHybridPods 验证 P0-1 核心修复：hybrid 形态
// master Service 的 selector 命中 component=hybrid 而非不存在的 master Pod。
func TestDeployHybridMasterServiceSelectsHybridPods(t *testing.T) {
	cfg := hybridSessionConfig()
	client := fake.NewSimpleClientset()
	manager := newTestManager(client)
	deployer := NewSessionManager(manager)
	_, err := deployer.Deploy(context.Background(), &DeployOptions{Config: cfg, ClusterCode: "e2e", ClusterID: 1, CreateRBAC: true})
	require.NoError(t, err)
	svc, err := client.CoreV1().Services("e2e-ns").Get(context.Background(), "e2e-master", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "hybrid", svc.Spec.Selector[LabelComponentKey])
	sts, err := client.AppsV1().StatefulSets("e2e-ns").Get(context.Background(), "e2e-master-hybrid", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Equal(t, "hybrid", sts.Spec.Template.Labels[LabelComponentKey])
}

// TestHybridScaleGetReplicasRolloutUpgrade 验证 hybrid 形态运维操作收敛到单一工作负载。
func TestHybridScaleGetReplicasRolloutUpgrade(t *testing.T) {
	cfg := hybridSessionConfig()
	client := fake.NewSimpleClientset()
	manager := newTestManager(client)
	deployer := NewSessionManager(manager)
	_, err := deployer.Deploy(context.Background(), &DeployOptions{Config: cfg, ClusterCode: "e2e", ClusterID: 1, CreateRBAC: true})
	require.NoError(t, err)
	ops := NewSessionManager(manager)
	require.NoError(t, ops.Scale(context.Background(), cfg, "hybrid", 3))
	n, err := ops.GetReplicas(context.Background(), cfg, "hybrid")
	require.NoError(t, err)
	assert.Equal(t, 3, n)
	// master 角色在 hybrid 形态下也归一到混合工作负载
	n2, err := ops.GetReplicas(context.Background(), cfg, "master")
	require.NoError(t, err)
	assert.Equal(t, 3, n2)
	require.NoError(t, ops.RolloutRestart(context.Background(), cfg, ""))
	require.NoError(t, ops.UpgradeImage(context.Background(), cfg, "apache/seatunnel:3.1.1"))
	sts, err := client.AppsV1().StatefulSets("e2e-ns").Get(context.Background(), "e2e-master-hybrid", metav1.GetOptions{})
	require.NoError(t, err)
	assert.Contains(t, sts.Spec.Template.Spec.Containers[0].Image, "3.1.1")
}

// TestPodToInfoHybridRolePrecedesMaster 验证 -master-hybrid 不被 -master 子串误判。
func TestPodToInfoHybridRolePrecedesMaster(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "e2e-master-hybrid-0",
			Labels: map[string]string{LabelComponentKey: "hybrid"},
			OwnerReferences: []metav1.OwnerReference{
				{Kind: "StatefulSet", Name: "e2e-master-hybrid"},
			},
		},
	}
	info := podToInfo(pod)
	assert.Equal(t, "hybrid", info.Role)
}

// TestHybridProxyCloneResolvesHybridWorkload 验证 hybrid 形态 Proxy 克隆目标解析。
func TestHybridProxyCloneResolvesHybridWorkload(t *testing.T) {
	cfg := hybridSessionConfig()
	manager := newTestManager(fake.NewSimpleClientset())
	deployer := NewSessionManager(manager)
	_, err := deployer.Deploy(context.Background(), &DeployOptions{Config: cfg, ClusterCode: "e2e", ClusterID: 1, CreateRBAC: true})
	require.NoError(t, err)
	clientset, err := manager.Clientset(context.Background(), cfg.HostID)
	require.NoError(t, err)
	_ = clientset
	proxyMgr := NewJavaProxyManager(manager, nil)
	spec, err := proxyMgr.resolveTargetPodTemplate(context.Background(), clientset, cfg)
	require.NoError(t, err)
	require.NotNil(t, spec)
	assert.NotEmpty(t, spec.Containers)
}
