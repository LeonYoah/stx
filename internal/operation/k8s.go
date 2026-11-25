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

package operation

// k8sOperationSpecs 登记 K8s 会话底座全部新增操作（host 侧只读探活 + cluster 侧生命周期）。
// k8sOperationSpecs registers every new K8s session-foundation operation (host-side read-only
// probes plus cluster-side lifecycle operations).
func k8sOperationSpecs() []OperationSpec {
	return []OperationSpec{
		// Host 侧只读操作（R0 无需确认）。
		// Host-side read-only operations (R0, no confirmation needed).
		k8sReadOperation(
			"host.test-connection", []string{"host", "test-connection"}, "Test kubernetes host connectivity and RBAC",
			"POST", "/api/v1/hosts/:id/test-connection",
			[]InputSpec{{Name: "id", Location: InputPath, Required: true, Description: "Host ID"}},
			"stx host test-connection 1",
		),
		k8sReadOperation(
			"host.k8s-namespaces", []string{"host", "k8s-namespaces"}, "List visible kubernetes namespaces",
			"GET", "/api/v1/hosts/:id/k8s/namespaces",
			[]InputSpec{{Name: "id", Location: InputPath, Required: true, Description: "Host ID"}},
			"stx host k8s-namespaces 1",
		),
		k8sReadOperation(
			"host.k8s-discover", []string{"host", "k8s-discover"}, "Discover adoptable SeaTunnel clusters in a namespace",
			"GET", "/api/v1/hosts/:id/k8s/discover",
			// 不登记 namespace 查询参数：生成式 CLI 的 --namespace 已被本地命名空间占用，
			// 服务端会回退到 Host 默认 Namespace，扫描其他 Namespace 走 API/Web。
			// The namespace query input is not registered: the generated CLI's --namespace flag is
			// already taken by the local CLI namespace; the server falls back to the host default
			// namespace, and scanning other namespaces goes through API/Web.
			[]InputSpec{{Name: "id", Location: InputPath, Required: true, Description: "Host ID"}},
			"stx host k8s-discover 1",
		),

		// Cluster 侧生命周期操作。
		// Cluster-side lifecycle operations.
		clusterBodyOperation(
			"cluster.kubernetes-create", []string{"cluster", "kubernetes-create"}, "Create a kubernetes cluster (adopt or deploy)",
			"POST", "/api/v1/clusters/kubernetes", RiskR1,
			"创建 K8s 集群会写入集群登记；stx_deployed 会在目标 Namespace 创建 StatefulSet/Service/ConfigMap 与伴生 Proxy。",
			[]InputSpec{
				{Name: "name", Location: InputBody, Required: true, Description: "Cluster name"},
				{Name: "host_id", Location: InputBody, Required: true, Description: "Kubernetes host ID"},
				{Name: "management_source", Location: InputBody, Required: true, Description: "adopted or stx_deployed"},
				{Name: "namespace", Location: InputBody, Required: true, Description: "Target namespace"},
			},
			"stx cluster kubernetes-create --request-file cluster.json --confirm",
		),
		clusterGeneratedOperation(
			"cluster.k8s-scale", []string{"cluster", "k8s-scale"}, "Scale kubernetes cluster master/worker replicas",
			"POST", "/api/v1/clusters/:id/k8s/scale", RiskR1,
			"扩缩容直接修改 StatefulSet 副本数，可能影响正在执行的作业。",
			"stx cluster k8s-scale 1 --component worker --replicas 3 --confirm",
			[]InputSpec{
				{Name: "id", Location: InputPath, Required: true, Description: "Cluster ID"},
				{Name: "component", Location: InputBody, Required: true, Description: "master or worker"},
				{Name: "replicas", Location: InputBody, Required: true, Description: "Target replicas"},
			},
		),
		clusterGeneratedOperation(
			"cluster.k8s-rollout-restart", []string{"cluster", "k8s-rollout-restart"}, "Rolling restart kubernetes cluster components",
			"POST", "/api/v1/clusters/:id/k8s/rollout-restart", RiskR1,
			"滚动重启会逐个重建 Pod，期间作业可能 failover。",
			"stx cluster k8s-rollout-restart 1 --component worker --confirm",
			[]InputSpec{
				{Name: "id", Location: InputPath, Required: true, Description: "Cluster ID"},
				{Name: "component", Location: InputBody, Required: true, Description: "master or worker"},
			},
		),
		clusterGeneratedOperation(
			"cluster.k8s-upgrade-image", []string{"cluster", "k8s-upgrade-image"}, "Rolling upgrade kubernetes cluster image tag",
			"POST", "/api/v1/clusters/:id/k8s/upgrade-image", RiskR1,
			"镜像滚动升级会重建全部引擎 Pod 与伴生 Proxy，是唯一合法的插件变更路径。",
			"stx cluster k8s-upgrade-image 1 --image apache/seatunnel:3.1.1 --confirm",
			[]InputSpec{
				{Name: "id", Location: InputPath, Required: true, Description: "Cluster ID"},
				{Name: "image", Location: InputBody, Required: true, Description: "New image tag"},
			},
		),
		clusterGeneratedOperation(
			"cluster.k8s-java-proxy-retry", []string{"cluster", "k8s-java-proxy-retry"}, "Retry companion java proxy provisioning",
			"POST", "/api/v1/clusters/:id/k8s/java-proxy/retry", RiskR1,
			"重试会重建伴生 Proxy 工作负载，不影响引擎集群本身。",
			"stx cluster k8s-java-proxy-retry 1 --confirm",
			[]InputSpec{{Name: "id", Location: InputPath, Required: true, Description: "Cluster ID"}},
		),
		k8sClusterReadOperation(
			"cluster.k8s-pods", []string{"cluster", "k8s-pods"}, "List kubernetes cluster pod projection",
			"GET", "/api/v1/clusters/:id/k8s/pods",
			[]InputSpec{{Name: "id", Location: InputPath, Required: true, Description: "Cluster ID"}},
			"stx cluster k8s-pods 1",
		),
		k8sClusterReadOperation(
			"cluster.k8s-pod-logs", []string{"cluster", "k8s-pod-logs"}, "Fetch kubernetes pod logs",
			"GET", "/api/v1/clusters/:id/k8s/pods/:pod/logs",
			[]InputSpec{
				{Name: "id", Location: InputPath, Required: true, Description: "Cluster ID"},
				{Name: "pod", Location: InputPath, Required: true, Description: "Pod name"},
				{Name: "tail_lines", Location: InputQuery, Required: false, Description: "Tail line count"},
			},
			"stx cluster k8s-pod-logs 1 seatunnel-worker-0 --tail-lines 200",
		),
		k8sClusterReadOperation(
			"cluster.k8s-configmap", []string{"cluster", "k8s-configmap"}, "Read kubernetes cluster configmap entries",
			"GET", "/api/v1/clusters/:id/k8s/configmap",
			[]InputSpec{{Name: "id", Location: InputPath, Required: true, Description: "Cluster ID"}},
			"stx cluster k8s-configmap 1",
		),
		clusterBodyOperation(
			"cluster.k8s-configmap-update", []string{"cluster", "k8s-configmap-update"}, "Update one configmap file with optional rolling restart",
			"PUT", "/api/v1/clusters/:id/k8s/configmap/:file", RiskR1,
			"修改 ConfigMap 影响全部引擎节点配置；勾选 restart 会触发滚动重启。",
			[]InputSpec{
				{Name: "id", Location: InputPath, Required: true, Description: "Cluster ID"},
				{Name: "file", Location: InputPath, Required: true, Description: "ConfigMap file key"},
				{Name: "content", Location: InputBody, Required: true, Description: "File content"},
				{Name: "restart", Location: InputBody, Required: false, Description: "Roll restart after update"},
			},
			"stx cluster k8s-configmap-update 1 seatunnel.yaml --request-file cm.json --confirm",
		),
	}
}

// k8sReadOperation 登记 host 侧只读操作（test-connection 用 POST 但语义只读，不写 K8s 对象）。
// k8sReadOperation registers host-side read-only operations (test-connection uses POST but is
// semantically read-only and never writes K8s objects).
func k8sReadOperation(id string, commandPath []string, summary, method, route string, inputs []InputSpec, example string) OperationSpec {
	return OperationSpec{
		ID: id, CommandPath: commandPath, Summary: summary, GeneratedCLI: true, Method: method, Route: route,
		Mode: ModeNormal, AuthRequired: true, Risk: RiskR0, Revision: 1, SupportsPick: true,
		Input:         inputs,
		OutputExample: `{"api_version":"v1","operation_id":"` + id + `","request_id":"req_example","data":{},"result_meta":{"complete":true}}`,
		Example:       example,
	}
}

// k8sClusterReadOperation 登记 cluster 侧只读操作。
// k8sClusterReadOperation registers cluster-side read-only operations.
func k8sClusterReadOperation(id string, commandPath []string, summary, method, route string, inputs []InputSpec, example string) OperationSpec {
	spec := k8sReadOperation(id, commandPath, summary, method, route, inputs, example)
	return spec
}
