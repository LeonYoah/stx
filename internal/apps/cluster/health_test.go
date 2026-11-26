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

package cluster

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"

	"github.com/LeonYoah/stx/internal/pkg/infra"
	"github.com/LeonYoah/stx/internal/pkg/k8s"
)

type healthHostProvider struct{}

func (healthHostProvider) GetHostByID(_ context.Context, id uint) (*HostInfo, error) {
	return &HostInfo{ID: id, AgentID: fmt.Sprintf("agent-%d", id)}, nil
}

// healthMemberSender 按 Agent 返回其看到的成员数；0 表示不支持（旧 Agent）。
// healthMemberSender returns the member count each Agent sees; 0 means unsupported (old Agent).
type healthMemberSender struct {
	counts map[string]int
	calls  int
}

func (s *healthMemberSender) SendCommand(_ context.Context, agentID, commandType string, _ map[string]string) (bool, string, error) {
	s.calls++
	if commandType != "check_cluster_members" {
		return false, "unexpected", nil
	}
	count := s.counts[agentID]
	if count == 0 {
		return false, `{"success":false,"message":"unsupported sub_command"}`, nil
	}
	return true, fmt.Sprintf(`{"success":true,"details":{"member_count":"%d"}}`, count), nil
}

func healthTestCluster(status ClusterStatus, ids ...uint) (*Cluster, *ClusterStatusInfo) {
	cluster := &Cluster{ID: 1, Status: status}
	info := &ClusterStatusInfo{ClusterID: 1, TotalNodes: len(ids)}
	for _, id := range ids {
		cluster.Nodes = append(cluster.Nodes, ClusterNode{ID: id, HostID: id, HazelcastPort: 5801})
		info.Nodes = append(info.Nodes, &NodeStatusInfo{NodeID: id, HostID: id, HostName: fmt.Sprintf("n%d", id), IsOnline: true, Status: NodeStatusRunning})
	}
	return cluster, info
}

func newHealthTestService(sender AgentCommandSender) *Service {
	s := &Service{hostProvider: healthHostProvider{}}
	if sender != nil {
		s.agentSender = sender
	}
	return s
}

func TestEvaluateClusterHealth_AllRunningAndAgreeingIsHealthy(t *testing.T) {
	sender := &healthMemberSender{counts: map[string]int{"agent-1": 3, "agent-2": 3, "agent-3": 3}}
	cluster, info := healthTestCluster(ClusterStatusRunning, 1, 2, 3)
	newHealthTestService(sender).evaluateClusterHealth(context.Background(), cluster, info)
	if info.HealthStatus != HealthStatusHealthy || len(info.HealthReasons) != 0 || info.RunningNodes != 3 {
		t.Fatalf("expected healthy, got %s %v running=%d", info.HealthStatus, info.HealthReasons, info.RunningNodes)
	}
	if info.Nodes[0].MemberCount == nil || *info.Nodes[0].MemberCount != 3 {
		t.Fatalf("expected member count to be exposed, got %+v", info.Nodes[0].MemberCount)
	}
}

func TestEvaluateClusterHealth_StoppedProcessOnOnlineHostIsUnhealthy(t *testing.T) {
	cluster, info := healthTestCluster(ClusterStatusRunning, 1, 2, 3)
	info.Nodes[1].Status = NodeStatusStopped
	newHealthTestService(nil).evaluateClusterHealth(context.Background(), cluster, info)
	if info.HealthStatus != HealthStatusUnhealthy {
		t.Fatalf("expected unhealthy, got %s", info.HealthStatus)
	}
	if len(info.HealthReasons) != 1 || !strings.Contains(info.HealthReasons[0], "n2") {
		t.Fatalf("expected one reason naming n2, got %v", info.HealthReasons)
	}
	if len(info.HealthReasonDetails) != 1 ||
		info.HealthReasonDetails[0].Code != ClusterHealthReasonProcessNotRunning ||
		info.HealthReasonDetails[0].Status != string(NodeStatusStopped) {
		t.Fatalf("expected process_not_running detail, got %+v", info.HealthReasonDetails)
	}
}

func TestEvaluateClusterHealth_SplitMembersIsUnhealthy(t *testing.T) {
	sender := &healthMemberSender{counts: map[string]int{"agent-1": 2, "agent-2": 2, "agent-3": 1}}
	cluster, info := healthTestCluster(ClusterStatusRunning, 1, 2, 3)
	newHealthTestService(sender).evaluateClusterHealth(context.Background(), cluster, info)
	if info.HealthStatus != HealthStatusUnhealthy || len(info.HealthReasons) != 3 {
		t.Fatalf("expected 3 split-brain reasons, got %s %v", info.HealthStatus, info.HealthReasons)
	}
	for i, detail := range info.HealthReasonDetails {
		if detail.Code != ClusterHealthReasonMemberSplitBrain ||
			detail.ExpectedMembers != 3 || detail.ObservedMembers != []int{2, 2, 1}[i] {
			t.Fatalf("expected member_split_brain detail at %d, got %+v", i, detail)
		}
	}
}

func TestEvaluateClusterHealth_UnknownMemberCountDoesNotAlarm(t *testing.T) {
	sender := &healthMemberSender{counts: map[string]int{}}
	cluster, info := healthTestCluster(ClusterStatusRunning, 1, 2)
	svc := newHealthTestService(sender)
	svc.evaluateClusterHealth(context.Background(), cluster, info)
	if info.HealthStatus != HealthStatusHealthy {
		t.Fatalf("unknown member counts must not mark unhealthy, got %s %v", info.HealthStatus, info.HealthReasons)
	}
	// 缓存：TTL 内不重复探测。Cache: no re-probe within the TTL.
	calls := sender.calls
	_, info = healthTestCluster(ClusterStatusRunning, 1, 2)
	svc.evaluateClusterHealth(context.Background(), cluster, info)
	if sender.calls != calls {
		t.Fatalf("expected cached probes, calls %d -> %d", calls, sender.calls)
	}
}

func TestEvaluateClusterHealth_StoppedClusterDoesNotRequireProcesses(t *testing.T) {
	cluster, info := healthTestCluster(ClusterStatusStopped, 1, 2)
	for _, node := range info.Nodes {
		node.Status = NodeStatusStopped
	}
	newHealthTestService(nil).evaluateClusterHealth(context.Background(), cluster, info)
	if info.HealthStatus != HealthStatusHealthy {
		t.Fatalf("a stopped cluster with online hosts should not be unhealthy, got %s %v", info.HealthStatus, info.HealthReasons)
	}

	info.Nodes[0].IsOnline = false
	newHealthTestService(nil).evaluateClusterHealth(context.Background(), cluster, info)
	if info.HealthStatus != HealthStatusUnhealthy {
		t.Fatalf("an offline host is always unhealthy, got %s", info.HealthStatus)
	}
}

// healthEngineSender 按 Agent 返回 check_engine_health 的原始 JSON；空串模拟旧 Agent 不支持。
// healthEngineSender returns a check_engine_health JSON payload per agent; an empty string
// simulates an old Agent that does not support the command.
type healthEngineSender struct {
	payloads    map[string]string
	engineCalls int
}

func (s *healthEngineSender) SendCommand(_ context.Context, agentID, commandType string, _ map[string]string) (bool, string, error) {
	if commandType != "check_engine_health" {
		return false, "unexpected", nil
	}
	s.engineCalls++
	payload := s.payloads[agentID]
	if payload == "" {
		return false, `{"success":false,"message":"unsupported sub_command"}`, nil
	}
	return true, payload, nil
}

// TestEvaluateClusterHealth_EngineNotReadyIsUnhealthy 半初始化引擎（known+unhealthy）在
// 单节点集群也要被判为不健康，原因透传。
// TestEvaluateClusterHealth_EngineNotReadyIsUnhealthy: a half-initialized engine (known+unhealthy)
// makes even a single-node cluster unhealthy and propagates the reason.
func TestEvaluateClusterHealth_EngineNotReadyIsUnhealthy(t *testing.T) {
	sender := &healthEngineSender{payloads: map[string]string{
		"agent-1": `{"success":false,"details":{"known":"true","healthy":"false","reason":"coordinatorService is null"}}`,
	}}
	cluster, info := healthTestCluster(ClusterStatusRunning, 1)
	cluster.Nodes[0].Role = NodeRoleMasterWorker
	newHealthTestService(sender).evaluateClusterHealth(context.Background(), cluster, info)
	if info.HealthStatus != HealthStatusUnhealthy {
		t.Fatalf("expected unhealthy, got %s %v", info.HealthStatus, info.HealthReasons)
	}
	if len(info.HealthReasons) != 1 || !strings.Contains(info.HealthReasons[0], "coordinatorService is null") {
		t.Fatalf("expected engine reason, got %v", info.HealthReasons)
	}
	if len(info.HealthReasonDetails) != 1 ||
		info.HealthReasonDetails[0].Code != ClusterHealthReasonEngineNotReady ||
		info.HealthReasonDetails[0].Detail != "coordinatorService is null" {
		t.Fatalf("expected engine_not_ready detail, got %+v", info.HealthReasonDetails)
	}
}

// TestEvaluateClusterHealth_EngineHealthyStaysHealthy 引擎就绪（known+healthy）不产生原因。
// TestEvaluateClusterHealth_EngineHealthyStaysHealthy: a ready engine adds no reasons.
func TestEvaluateClusterHealth_EngineHealthyStaysHealthy(t *testing.T) {
	sender := &healthEngineSender{payloads: map[string]string{
		"agent-1": `{"success":true,"details":{"known":"true","healthy":"true"}}`,
	}}
	cluster, info := healthTestCluster(ClusterStatusRunning, 1)
	cluster.Nodes[0].Role = NodeRoleMasterWorker
	newHealthTestService(sender).evaluateClusterHealth(context.Background(), cluster, info)
	if info.HealthStatus != HealthStatusHealthy || len(info.HealthReasons) != 0 {
		t.Fatalf("expected healthy, got %s %v", info.HealthStatus, info.HealthReasons)
	}
}

// TestEvaluateClusterHealth_UnknownEngineStateDoesNotAlarm 未知状态（旧 Agent、REST 不可达）
// 绝不误报；纯 worker 角色不探测。
// TestEvaluateClusterHealth_UnknownEngineStateDoesNotAlarm: unknown states (old Agent,
// unreachable REST) never alarm; worker-only roles are not probed.
func TestEvaluateClusterHealth_UnknownEngineStateDoesNotAlarm(t *testing.T) {
	sender := &healthEngineSender{payloads: map[string]string{
		"agent-1": `{"success":false,"details":{"known":"false","healthy":"false","reason":"connection refused"}}`,
	}}
	cluster, info := healthTestCluster(ClusterStatusRunning, 1, 2)
	cluster.Nodes[0].Role = NodeRoleMasterWorker
	cluster.Nodes[1].Role = NodeRoleWorker
	svc := newHealthTestService(sender)
	svc.evaluateClusterHealth(context.Background(), cluster, info)
	if info.HealthStatus != HealthStatusHealthy {
		t.Fatalf("unknown engine state must not mark unhealthy, got %s %v", info.HealthStatus, info.HealthReasons)
	}
	if sender.engineCalls != 1 {
		t.Fatalf("worker-only node must not be probed, got %d calls", sender.engineCalls)
	}
	// 缓存：TTL 内不重复探测。Cache: no re-probe within the TTL.
	_, info = healthTestCluster(ClusterStatusRunning, 1, 2)
	svc.evaluateClusterHealth(context.Background(), cluster, info)
	if sender.engineCalls != 1 {
		t.Fatalf("expected cached probe, calls %d -> %d", 1, sender.engineCalls)
	}
}

// TestEvaluateClusterHealth_HealthReasonDetailsAligned 结构化明细与文案一一对应，
// 覆盖全部 4 类 code，供前端按 code 单语渲染。
// TestEvaluateClusterHealth_HealthReasonDetailsAligned: structured details align 1:1
// with the flat reasons and cover all four codes for locale-aware frontend rendering.
func TestEvaluateClusterHealth_HealthReasonDetailsAligned(t *testing.T) {
	sender := &healthMemberSender{counts: map[string]int{"agent-2": 1}}
	cluster, info := healthTestCluster(ClusterStatusRunning, 1, 2, 3, 4)
	// 节点 1 离线（node_offline）、节点 4 进程停止（process_not_running）、
	// 节点 2/3 运行且节点 2 只看到 1/2 成员（member_split_brain）。
	// Node 1 offline (node_offline), node 4 process stopped (process_not_running),
	// nodes 2/3 running while node 2 sees only 1/2 members (member_split_brain).
	info.Nodes[0].IsOnline = false
	info.Nodes[3].Status = NodeStatusStopped
	newHealthTestService(sender).evaluateClusterHealth(context.Background(), cluster, info)

	if info.HealthStatus != HealthStatusUnhealthy {
		t.Fatalf("expected unhealthy, got %s", info.HealthStatus)
	}
	if len(info.HealthReasonDetails) != len(info.HealthReasons) {
		t.Fatalf("details must align 1:1 with reasons, got %d details vs %d reasons",
			len(info.HealthReasonDetails), len(info.HealthReasons))
	}
	wantCodes := []ClusterHealthReasonCode{
		ClusterHealthReasonNodeOffline,
		ClusterHealthReasonProcessNotRunning,
		ClusterHealthReasonMemberSplitBrain,
	}
	gotCodes := make([]ClusterHealthReasonCode, 0, len(info.HealthReasonDetails))
	for _, detail := range info.HealthReasonDetails {
		gotCodes = append(gotCodes, detail.Code)
	}
	for i, want := range wantCodes {
		if i >= len(gotCodes) || gotCodes[i] != want {
			t.Fatalf("expected codes %v, got %v", wantCodes, gotCodes)
		}
	}
}

func TestParseMemberCount(t *testing.T) {
	if n, ok := parseMemberCount(`{"details":{"member_count":"3"}}`); !ok || n != 3 {
		t.Fatalf("got %d %v", n, ok)
	}
	for _, raw := range []string{"", "not json", `{"details":{}}`, `{"details":{"member_count":"0"}}`} {
		if _, ok := parseMemberCount(raw); ok {
			t.Fatalf("expected unknown for %q", raw)
		}
	}
}

// 启动/重启受理后的宽限期内不做成员一致性告警（Hazelcast 合并需数十秒），
// 宽限期结束后再看到成员缺失才报脑裂。
// Within the grace window after an accepted start/restart, member-agreement alarms are
// skipped (Hazelcast merge takes tens of seconds); missing members alarm only after it ends.
func TestEvaluateClusterHealth_MemberGraceSuppressesSplitBrain(t *testing.T) {
	sender := &healthMemberSender{counts: map[string]int{"agent-1": 1, "agent-2": 1, "agent-3": 1}}
	svc := newHealthTestService(sender)
	svc.beginMemberGrace(1)

	cluster, info := healthTestCluster(ClusterStatusRunning, 1, 2, 3)
	svc.evaluateClusterHealth(context.Background(), cluster, info)
	if info.HealthStatus != HealthStatusHealthy || len(info.HealthReasons) != 0 {
		t.Fatalf("grace window must suppress split-brain alarms, got %s %v", info.HealthStatus, info.HealthReasons)
	}

	// 宽限期过期：同样的成员缺失必须告警。/ Expired grace: the same missing members must alarm.
	svc.memberGraceUntil.Store(uint(1), time.Now().Add(-time.Second))
	svc.memberProbes = memberProbeCache{} // 清缓存，强制重新探测 / clear the cache to force a re-probe
	_, info = healthTestCluster(ClusterStatusRunning, 1, 2, 3)
	svc.evaluateClusterHealth(context.Background(), cluster, info)
	if info.HealthStatus != HealthStatusUnhealthy || len(info.HealthReasons) != 3 {
		t.Fatalf("expired grace must report split brain again, got %s %v", info.HealthStatus, info.HealthReasons)
	}
}

// TestEvaluateK8sClusterHealth_NonRunningZeroPodsAndServiceFallback 验证 K8s 健康检查在非运行态返回 unknown、
// 运行态 0 Pod 返回 unhealthy，且缺少 MasterServiceName 时自动回落 ClusterServiceName 探活 /overview。
// TestEvaluateK8sClusterHealth_NonRunningZeroPodsAndServiceFallback verifies K8s health returns unknown
// when not expecting running, unhealthy when running with 0 pods, and falls back to ClusterServiceName
// for the /overview probe when MasterServiceName is empty.
func TestEvaluateK8sClusterHealth_NonRunningZeroPodsAndServiceFallback(t *testing.T) {
	var overviewPaths []string
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		overviewPaths = append(overviewPaths, r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"projectVersion":"3.1.0"}`))
	}))
	defer apiServer.Close()

	newK8sSvc := func(pods ...*corev1.Pod) *Service {
		objs := make([]*corev1.Pod, 0, len(pods))
		objs = append(objs, pods...)
		client := fake.NewSimpleClientset()
		for _, p := range objs {
			_, _ = client.CoreV1().Pods(p.Namespace).Create(context.Background(), p, metav1.CreateOptions{})
		}
		mgr := k8s.NewClientManagerWithFactory(
			func(_ context.Context, _ uint) (*k8s.Credentials, error) {
				return &k8s.Credentials{
					AuthMode: k8s.AuthModeToken,
					APIURL:   apiServer.URL,
					Token:    "test-token",
				}, nil
			},
			func(_ *rest.Config) (kubernetes.Interface, error) {
				return client, nil
			},
		)
		return &Service{k8sManager: mgr}
	}

	makeCluster := func(status ClusterStatus, masterReplicas, workerReplicas int) *Cluster {
		return &Cluster{
			ID:          7,
			ClusterCode: "demo",
			Status:      status,
			InfraType:   string(infra.TypeK8sSession),
			Config: ClusterConfig{
				"kubernetes": map[string]interface{}{
					"host_id":              1,
					"namespace":            "seatunnel",
					"access_mode":          "apiserver_proxy",
					"cluster_service_name": "adopted-svc",
					"master_http_port":     8080,
					"master_replicas":      masterReplicas,
					"worker_replicas":      workerReplicas,
					"label_selector":       "app=seatunnel",
				},
			},
		}
	}

	// 1. 非运行态 + 0 Pod 必须是 unknown，不能误报 healthy。
	// 1. Non-running status with 0 pods must be unknown, never falsely healthy.
	infoCreated := &ClusterStatusInfo{ClusterID: 7}
	newK8sSvc().evaluateClusterHealth(context.Background(), makeCluster(ClusterStatusCreated, 1, 2), infoCreated)
	if infoCreated.HealthStatus != HealthStatusUnknown {
		t.Fatalf("created k8s cluster with 0 pods must be unknown, got %s %v", infoCreated.HealthStatus, infoCreated.HealthReasons)
	}

	// 2. 运行态 + 0 Pod 必须是 unhealthy（0/3）。
	// 2. Running status with 0 pods must be unhealthy (0/3).
	infoZero := &ClusterStatusInfo{ClusterID: 7}
	newK8sSvc().evaluateClusterHealth(context.Background(), makeCluster(ClusterStatusRunning, 1, 2), infoZero)
	if infoZero.HealthStatus != HealthStatusUnhealthy || len(infoZero.HealthReasonDetails) != 1 ||
		infoZero.HealthReasonDetails[0].Code != ClusterHealthReasonProcessNotRunning || infoZero.HealthReasonDetails[0].Status != "0/3" {
		t.Fatalf("running k8s cluster with 0 pods must be unhealthy (0/3), got %s %+v", infoZero.HealthStatus, infoZero.HealthReasonDetails)
	}

	// 3. 运行态 + 就绪 Pod 满足副本数 + MasterServiceName 为空时回落 ClusterServiceName 探活 /overview。
	// 3. Running status with ready pods matching replicas and empty MasterServiceName falls back to ClusterServiceName for /overview.
	readyPod := func(name string) *corev1.Pod {
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: "seatunnel",
				Labels:    map[string]string{"app": "seatunnel"},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				Conditions: []corev1.PodCondition{
					{Type: corev1.PodReady, Status: corev1.ConditionTrue},
				},
			},
		}
	}
	infoHealthy := &ClusterStatusInfo{ClusterID: 7}
	newK8sSvc(readyPod("seatunnel-master-0"), readyPod("seatunnel-worker-0")).
		evaluateClusterHealth(context.Background(), makeCluster(ClusterStatusRunning, 1, 1), infoHealthy)
	if infoHealthy.HealthStatus != HealthStatusHealthy || infoHealthy.RunningNodes != 2 {
		t.Fatalf("expected healthy with 2 ready pods, got %s running=%d reasons=%v", infoHealthy.HealthStatus, infoHealthy.RunningNodes, infoHealthy.HealthReasons)
	}
	wantPath := "/api/v1/namespaces/seatunnel/services/http:adopted-svc:8080/proxy/overview"
	if len(overviewPaths) != 1 || overviewPaths[0] != wantPath {
		t.Fatalf("expected /overview probe via ClusterServiceName %q, got %v", wantPath, overviewPaths)
	}
}

// TestEvaluateK8sClusterHealth_UsesLiveWorkloadReplicas 验证缩容后健康检查按实时
// Workload spec.replicas 判定：持久化配置仍是旧副本数，但全部当前 Pod 就绪时必须 healthy。
// TestEvaluateK8sClusterHealth_UsesLiveWorkloadReplicas verifies health uses live workload
// spec.replicas after a downscale: the persisted config still holds the old replica count,
// yet all current pods being ready must yield healthy.
func TestEvaluateK8sClusterHealth_UsesLiveWorkloadReplicas(t *testing.T) {
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"projectVersion":"3.1.0"}`))
	}))
	defer apiServer.Close()

	ctx := context.Background()
	client := fake.NewSimpleClientset()
	one := int32(1)
	for _, name := range []string{"demo-master", "demo-worker"} {
		sts := &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "seatunnel"},
			Spec:       appsv1.StatefulSetSpec{Replicas: &one},
		}
		if _, err := client.AppsV1().StatefulSets("seatunnel").Create(ctx, sts, metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	readyPod := func(name string) *corev1.Pod {
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: "seatunnel",
				Labels:    map[string]string{"app": "seatunnel"},
			},
			Status: corev1.PodStatus{
				Phase:      corev1.PodRunning,
				Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
			},
		}
	}
	for _, name := range []string{"demo-master-0", "demo-worker-0"} {
		if _, err := client.CoreV1().Pods("seatunnel").Create(ctx, readyPod(name), metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	mgr := k8s.NewClientManagerWithFactory(
		func(_ context.Context, _ uint) (*k8s.Credentials, error) {
			return &k8s.Credentials{AuthMode: k8s.AuthModeToken, APIURL: apiServer.URL, Token: "test-token"}, nil
		},
		func(_ *rest.Config) (kubernetes.Interface, error) {
			return client, nil
		},
	)
	svc := &Service{k8sManager: mgr}

	// 持久化配置停留在缩容前的 1 master + 2 worker，实时 Workload 已缩到 1+1。
	// The persisted config still holds the pre-downscale 1 master + 2 workers while the
	// live workloads are already scaled to 1+1.
	cluster := &Cluster{
		ID:          9,
		ClusterCode: "demo",
		Status:      ClusterStatusRunning,
		InfraType:   string(infra.TypeK8sSession),
		Config: ClusterConfig{
			"kubernetes": map[string]interface{}{
				"host_id":              1,
				"namespace":            "seatunnel",
				"access_mode":          "apiserver_proxy",
				"cluster_service_name": "demo",
				"master_http_port":     8080,
				"master_replicas":      1,
				"worker_replicas":      2,
				"label_selector":       "app=seatunnel",
				"master_workload_name": "demo-master",
				"worker_workload_name": "demo-worker",
			},
		},
	}
	info := &ClusterStatusInfo{ClusterID: 9}
	svc.evaluateClusterHealth(ctx, cluster, info)
	if info.HealthStatus != HealthStatusHealthy {
		t.Fatalf("scaled-down cluster with all live pods ready must be healthy, got %s %v", info.HealthStatus, info.HealthReasons)
	}
}

// TestScaleK8sComponentPersistsReplicas 验证扩缩容成功后持久化副本数同步更新。
// TestScaleK8sComponentPersistsReplicas verifies scaling persists the new replica count.
func TestScaleK8sComponentPersistsReplicas(t *testing.T) {
	ctx := context.Background()
	client := fake.NewSimpleClientset()
	three := int32(3)
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "demo-worker", Namespace: "seatunnel"},
		Spec:       appsv1.StatefulSetSpec{Replicas: &three},
	}
	if _, err := client.AppsV1().StatefulSets("seatunnel").Create(ctx, sts, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
	mgr := k8s.NewClientManagerWithFactory(
		func(_ context.Context, _ uint) (*k8s.Credentials, error) {
			return &k8s.Credentials{AuthMode: k8s.AuthModeToken, APIURL: "https://unused", Token: "test-token"}, nil
		},
		func(_ *rest.Config) (kubernetes.Interface, error) {
			return client, nil
		},
	)

	db, cleanup := setupServiceTestDB(t)
	defer cleanup()
	repo := NewRepository(db)
	svc := &Service{k8sManager: mgr, repo: repo}

	cluster := &Cluster{
		Name:        "scale-persist",
		ClusterCode: "demo",
		Status:      ClusterStatusRunning,
		InfraType:   string(infra.TypeK8sSession),
		Config: ClusterConfig{
			"kubernetes": map[string]interface{}{
				"host_id":              1,
				"namespace":            "seatunnel",
				"access_mode":          "apiserver_proxy",
				"management_source":    "stx_deployed",
				"image":                "apache/seatunnel:3.1.0",
				"master_replicas":      1,
				"worker_replicas":      3,
				"master_workload_name": "demo-master",
				"worker_workload_name": "demo-worker",
			},
		},
	}
	if err := repo.Create(ctx, cluster); err != nil {
		t.Fatalf("create cluster: %v", err)
	}

	if err := svc.ScaleK8sComponent(ctx, cluster, "worker", 1); err != nil {
		t.Fatalf("scale: %v", err)
	}
	reloaded, err := repo.GetByID(ctx, cluster.ID, false)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	cfg := reloaded.K8sSessionConfig()
	if cfg == nil {
		t.Fatal("missing k8s session config after scale")
	}
	if cfg.WorkerReplicas != 1 {
		t.Fatalf("persisted worker replicas must be 1 after downscale, got %d", cfg.WorkerReplicas)
	}
	if cfg.MasterReplicas != 1 {
		t.Fatalf("master replicas must stay 1, got %d", cfg.MasterReplicas)
	}
}

// TestEvaluateK8sClusterHealth_NodeCounts 验证 K8s 健康检查填充 TotalNodes/OnlineNodes/OfflineNodes。
// TestEvaluateK8sClusterHealth_NodeCounts verifies K8s health evaluation fills
// TotalNodes/OnlineNodes/OfflineNodes.
func TestEvaluateK8sClusterHealth_NodeCounts(t *testing.T) {
	client := fake.NewSimpleClientset()
	mgr := k8s.NewClientManagerWithFactory(
		func(_ context.Context, _ uint) (*k8s.Credentials, error) {
			return &k8s.Credentials{AuthMode: k8s.AuthModeToken, APIURL: "https://unused", Token: "t"}, nil
		},
		func(_ *rest.Config) (kubernetes.Interface, error) {
			return client, nil
		},
	)
	svc := &Service{k8sManager: mgr}
	cluster := &Cluster{
		ID:          9,
		ClusterCode: "demo",
		Status:      ClusterStatusRunning,
		InfraType:   string(infra.TypeK8sSession),
		Config: ClusterConfig{
			"kubernetes": map[string]interface{}{
				"host_id":           1,
				"namespace":         "seatunnel",
				"access_mode":       "apiserver_proxy",
				"management_source": "stx_deployed",
				"master_replicas":   1,
				"worker_replicas":   2,
			},
		},
	}
	info := &ClusterStatusInfo{}
	svc.evaluateK8sClusterHealth(context.Background(), cluster, info)
	if info.TotalNodes != 3 {
		t.Fatalf("TotalNodes = %d, want 3", info.TotalNodes)
	}
	if info.OnlineNodes != info.RunningNodes {
		t.Fatalf("OnlineNodes = %d must equal RunningNodes = %d", info.OnlineNodes, info.RunningNodes)
	}
	if info.OfflineNodes != info.TotalNodes-info.OnlineNodes {
		t.Fatalf("OfflineNodes = %d inconsistent with total %d online %d", info.OfflineNodes, info.TotalNodes, info.OnlineNodes)
	}
	// stopped 集群同样必须填充计数 / stopped clusters must still expose the counts
	cluster.Status = ClusterStatusStopped
	info2 := &ClusterStatusInfo{}
	svc.evaluateK8sClusterHealth(context.Background(), cluster, info2)
	if info2.TotalNodes != 3 {
		t.Fatalf("stopped TotalNodes = %d, want 3", info2.TotalNodes)
	}
}
