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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
)

// ManualCleanupCommand 生成按 STX 归属标签精确清理单集群资源的 kubectl 命令（供零 delete 权限环境使用）。
// ManualCleanupCommand builds the label-scoped kubectl cleanup command for zero-delete RBAC environments.
func ManualCleanupCommand(namespace, clusterCode string) string {
	ns := strings.TrimSpace(namespace)
	if ns == "" {
		ns = "default"
	}
	return fmt.Sprintf("kubectl delete statefulset,deployment,service,configmap -n %s -l %s=%s,%s=%s",
		ns, LabelManagedByKey, LabelManagedByValue, LabelClusterCodeKey, strings.TrimSpace(clusterCode))
}

// 标签键统一管理：stx_deployed 创建的全部资源带统一标签，卸载按标签+名字精确删除，绝不误伤用户资源。
// Label keys are centralized: every stx_deployed resource carries unified labels so teardown
// deletes by exact name+label and never touches user resources.
const (
	LabelManagedByKey   = "app.kubernetes.io/managed-by"
	LabelManagedByValue = "stx"
	LabelClusterCodeKey = "stx.seatunnel.apache.org/cluster-code"
	// LabelEngineMemberKey 标记真正的引擎成员 Pod（master/worker），伴生 Proxy 不带此标签。
	// LabelEngineMemberKey marks real engine member pods (master/worker); the companion proxy never carries it.
	LabelEngineMemberKey = "stx.seatunnel.apache.org/engine-member"
	LabelClusterIDKey    = "stx.seatunnel.apache.org/cluster-id"
	LabelComponentKey    = "app.kubernetes.io/component"
)

// SessionManager 负责常驻集群的一键部署、安全卸载与运维操作。
// SessionManager handles one-click deploy, safe teardown and ops of session clusters.
type SessionManager struct {
	manager *ClientManager
}

// NewSessionManager 创建集群管理器。
// NewSessionManager creates a session manager.
func NewSessionManager(manager *ClientManager) *SessionManager {
	return &SessionManager{manager: manager}
}

func sessionLabels(clusterCode string, clusterID uint, component string) map[string]string {
	labels := map[string]string{
		LabelManagedByKey:   LabelManagedByValue,
		LabelClusterCodeKey: clusterCode,
		LabelComponentKey:   component,
	}
	if clusterID > 0 {
		labels[LabelClusterIDKey] = fmt.Sprintf("%d", clusterID)
	}
	return labels
}

// isOwnedByCluster 校验资源是否归属本 STX 集群：必须同时携带 managed-by=stx 与匹配的 cluster-code。
// 缺少任一标签即视为用户自有资源，upsert 不得覆盖、uninstall 不得删除。
// isOwnedByCluster reports whether a resource belongs to this STX cluster: it must carry
// both managed-by=stx and the matching cluster-code. Missing either label means the resource
// is user-owned and must neither be overwritten by upserts nor deleted by uninstall.
func isOwnedByCluster(labels map[string]string, clusterCode string) bool {
	if labels == nil {
		return false
	}
	return labels[LabelManagedByKey] == LabelManagedByValue && labels[LabelClusterCodeKey] == clusterCode
}

func errResourceConflict(kind, name string) error {
	return fmt.Errorf("k8s: %s %q already exists and is not owned by this cluster; refusing to overwrite", kind, name)
}

// masterServiceComponent 计算 master Service 选择器的目标组件标签。
// masterServiceComponent computes the component label the master Service selects.
func masterServiceComponent(cfg *SessionConfig) string {
	if cfg.IsHybrid() {
		return "hybrid"
	}
	return "master"
}

func podSelectorLabels(clusterCode, component string) map[string]string {
	return map[string]string{
		LabelClusterCodeKey:  clusterCode,
		LabelComponentKey:    component,
		LabelEngineMemberKey: "true",
	}
}

// SessionResourceNames 一套常驻集群的确定性资源命名。
// SessionResourceNames is the deterministic resource naming of one session cluster.
type SessionResourceNames struct {
	ConfigMap      string
	ClusterSvc     string
	MasterSvc      string
	MasterSTS      string
	WorkerSTS      string
	ServiceAccount string
	Role           string
	RoleBinding    string
	// RegistryCredentialSecret 私有仓库凭据 Secret 名（仅注册镜像带凭据时创建）。
	// RegistryCredentialSecret is the private-registry credential secret name, created
	// only when the registered image carries credentials.
	RegistryCredentialSecret string
}

// ResourceNamesFor 计算指定 cluster code 的资源命名。
// ResourceNamesFor computes resource names for a cluster code.
func ResourceNamesFor(clusterCode string) *SessionResourceNames {
	return &SessionResourceNames{
		ConfigMap:                clusterCode + "-configs",
		ClusterSvc:               clusterCode,
		MasterSvc:                clusterCode + "-master",
		MasterSTS:                clusterCode + "-master",
		WorkerSTS:                clusterCode + "-worker",
		ServiceAccount:           clusterCode,
		Role:                     clusterCode,
		RoleBinding:              clusterCode,
		RegistryCredentialSecret: "stx-regcred-" + clusterCode,
	}
}

// DeployOptions 一键部署参数。
// DeployOptions holds one-click deploy inputs.
type DeployOptions struct {
	Config      *SessionConfig
	ClusterCode string
	ClusterID   uint
	// CreateRBAC 为 true 时创建专用 ServiceAccount + Role/RoleBinding（仅 Hazelcast 发现所需最小权限）。
	// CreateRBAC creates a dedicated ServiceAccount + Role/RoleBinding with the minimal
	// permissions Hazelcast discovery needs.
	CreateRBAC bool
}

// DeployResult 部署结果摘要。
// DeployResult summarizes what was created.
type DeployResult struct {
	CreatedResources []string `json:"created_resources"`
}

// Deploy 按上游生产规范创建 Namespace(若缺失)、可选 RBAC、ConfigMap、Headless Services
// 与 Master/Worker StatefulSet；已存在同名资源时幂等更新为期望规格。
// Deploy creates the namespace (if missing), optional RBAC, ConfigMap, headless services
// and master/worker StatefulSets per upstream production guidance; existing resources are
// idempotently updated to the desired spec.
func (s *SessionManager) Deploy(ctx context.Context, opts *DeployOptions) (*DeployResult, error) {
	if opts == nil || opts.Config == nil {
		return nil, fmt.Errorf("k8s: deploy options are required")
	}
	cfg := opts.Config
	code := strings.TrimSpace(opts.ClusterCode)
	if code == "" {
		return nil, fmt.Errorf("k8s: cluster code is required")
	}
	names := ResourceNamesFor(code)
	clientset, err := s.manager.Clientset(ctx, cfg.HostID)
	if err != nil {
		return nil, err
	}
	ns := cfg.Namespace
	result := &DeployResult{CreatedResources: []string{}}
	track := func(kind, name string) {
		result.CreatedResources = append(result.CreatedResources, kind+"/"+name)
	}

	if err := ensureNamespace(ctx, clientset, ns); err != nil {
		return nil, err
	}
	if opts.CreateRBAC {
		if err := ensureRBAC(ctx, clientset, ns, names, code, opts.ClusterID); err != nil {
			return nil, err
		}
		track("serviceaccount", names.ServiceAccount)
	}

	if err := ensureSessionConfigMap(ctx, clientset, cfg, names, code, opts.ClusterID); err != nil {
		return nil, err
	}
	track("configmap", names.ConfigMap)

	if err := ensureHeadlessServices(ctx, clientset, cfg, names, code, opts.ClusterID); err != nil {
		return nil, err
	}
	track("service", names.ClusterSvc)
	track("service", names.MasterSvc)

	if cfg.IsHybrid() {
		if err := ensureSessionStatefulSet(ctx, clientset, cfg, names, code, opts.ClusterID, "hybrid", opts.CreateRBAC); err != nil {
			return nil, err
		}
		track("statefulset", names.MasterSTS+"-hybrid")
		return result, nil
	}
	if err := ensureSessionStatefulSet(ctx, clientset, cfg, names, code, opts.ClusterID, "master", opts.CreateRBAC); err != nil {
		return nil, err
	}
	track("statefulset", names.MasterSTS)
	if err := ensureSessionStatefulSet(ctx, clientset, cfg, names, code, opts.ClusterID, "worker", opts.CreateRBAC); err != nil {
		return nil, err
	}
	track("statefulset", names.WorkerSTS)
	return result, nil
}

// engineRoleArg 把 STX 角色映射为 Zeta 启动角色参数：hybrid 即 master_and_worker。
// engineRoleArg maps the STX role to the Zeta startup role argument: hybrid means
// master_and_worker.
func engineRoleArg(role string) string {
	if role == "hybrid" {
		return "master_and_worker"
	}
	return role
}

// EnsureNamespace 导出命名空间确保逻辑，供私仓凭据代建等部署前置步骤复用。
// EnsureNamespace exports the namespace ensure logic for pre-deploy steps like
// registry credential materialization.
func EnsureNamespace(ctx context.Context, clientset kubernetes.Interface, ns string) error {
	return ensureNamespace(ctx, clientset, ns)
}

func ensureNamespace(ctx context.Context, clientset kubernetes.Interface, ns string) error {
	_, err := clientset.CoreV1().Namespaces().Get(ctx, ns, metav1.GetOptions{})
	if err == nil {
		return nil
	}
	if apierrors.IsForbidden(err) {
		// 单命名空间最小权限凭据无集群级 namespaces 查询权限；假定运维已预先创建目标命名空间并继续。
		// Single-namespace minimal credentials cannot read cluster-scoped Namespace objects;
		// assume the operator pre-created the target namespace and proceed.
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("k8s: get namespace %q failed: %w", ns, err)
	}
	// Namespace 只做标记永不删除：卸载仅回收本集群确定性命名资源，防止级联抹掉外部 PVC 或同 Namespace 其他集群。
	// The namespace is only labeled and never deleted: uninstall reclaims solely this cluster's
	// deterministically named resources, preventing cascading loss of external PVCs or sibling clusters.
	if _, err := clientset.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: ns, Labels: map[string]string{LabelManagedByKey: LabelManagedByValue}},
	}, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("k8s: create namespace %q failed: %w", ns, err)
	}
	return nil
}

func ensureRBAC(ctx context.Context, clientset kubernetes.Interface, ns string, names *SessionResourceNames, code string, clusterID uint) error {
	labels := sessionLabels(code, clusterID, "rbac")
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: names.ServiceAccount, Namespace: ns, Labels: labels},
	}
	if _, err := clientset.CoreV1().ServiceAccounts(ns).Create(ctx, sa, metav1.CreateOptions{}); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("k8s: create serviceaccount failed: %w", err)
		}
		existing, getErr := clientset.CoreV1().ServiceAccounts(ns).Get(ctx, names.ServiceAccount, metav1.GetOptions{})
		if getErr != nil {
			return fmt.Errorf("k8s: get serviceaccount failed: %w", getErr)
		}
		if !isOwnedByCluster(existing.Labels, code) {
			return errResourceConflict("serviceaccount", names.ServiceAccount)
		}
	}

	// Session 模式 SA 保持最小权限：Hazelcast DNS 发现只需 endpoints/pods/services 只读，
	// Master HTTP 自发布归属仍为 Open Discussion，不预授 services patch。
	// Session-mode SA keeps least privilege: Hazelcast DNS discovery only needs read access to
	// endpoints/pods/services; master HTTP self-publish ownership is still an open discussion,
	// so services patch is not granted.
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: names.Role, Namespace: ns, Labels: labels},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"endpoints", "pods", "services"},
				Verbs:     []string{"get", "list", "watch"},
			},
		},
	}
	if _, err := clientset.RbacV1().Roles(ns).Create(ctx, role, metav1.CreateOptions{}); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("k8s: create role failed: %w", err)
		}
		existing, getErr := clientset.RbacV1().Roles(ns).Get(ctx, names.Role, metav1.GetOptions{})
		if getErr != nil {
			return fmt.Errorf("k8s: get role failed: %w", getErr)
		}
		if !isOwnedByCluster(existing.Labels, code) {
			return errResourceConflict("role", names.Role)
		}
	}

	binding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: names.RoleBinding, Namespace: ns, Labels: labels},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "Role",
			Name:     names.Role,
		},
		Subjects: []rbacv1.Subject{
			{Kind: rbacv1.ServiceAccountKind, Name: names.ServiceAccount, Namespace: ns},
		},
	}
	if _, err := clientset.RbacV1().RoleBindings(ns).Create(ctx, binding, metav1.CreateOptions{}); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("k8s: create rolebinding failed: %w", err)
		}
		existing, getErr := clientset.RbacV1().RoleBindings(ns).Get(ctx, names.RoleBinding, metav1.GetOptions{})
		if getErr != nil {
			return fmt.Errorf("k8s: get rolebinding failed: %w", getErr)
		}
		if !isOwnedByCluster(existing.Labels, code) {
			return errResourceConflict("rolebinding", names.RoleBinding)
		}
	}
	return nil
}

func ensureSessionConfigMap(ctx context.Context, clientset kubernetes.Interface, cfg *SessionConfig, names *SessionResourceNames, code string, clusterID uint) error {
	files := sessionConfigFiles(cfg, code)
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.ConfigMap,
			Namespace: cfg.Namespace,
			Labels:    sessionLabels(code, clusterID, "configs"),
		},
		Data: files,
	}
	existing, err := clientset.CoreV1().ConfigMaps(cfg.Namespace).Get(ctx, names.ConfigMap, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		if _, err := clientset.CoreV1().ConfigMaps(cfg.Namespace).Create(ctx, cm, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("k8s: create configmap failed: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("k8s: get configmap failed: %w", err)
	}
	if !isOwnedByCluster(existing.Labels, code) {
		return errResourceConflict("configmap", names.ConfigMap)
	}
	existing.Data = files
	if existing.Labels == nil {
		existing.Labels = map[string]string{}
	}
	for k, v := range sessionLabels(code, clusterID, "configs") {
		existing.Labels[k] = v
	}
	if _, err := clientset.CoreV1().ConfigMaps(cfg.Namespace).Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("k8s: update configmap failed: %w", err)
	}
	return nil
}

func ensureHeadlessServices(ctx context.Context, clientset kubernetes.Interface, cfg *SessionConfig, names *SessionResourceNames, code string, clusterID uint) error {
	clusterSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.ClusterSvc,
			Namespace: cfg.Namespace,
			Labels:    sessionLabels(code, clusterID, "cluster-service"),
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: corev1.ClusterIPNone,
			// Hazelcast 组网依赖 service-dns 在启动即解析全部成员：未就绪地址必须提前发布，
			// 否则 master 就绪前 worker 的 DNS 查询持续失败并耗尽 join 超时（鸡生蛋问题）。
			// selector 追加 engine-member 标签排除伴生 Proxy（5801 上的死地址会拖慢发现）。
			// Hazelcast clustering needs service-dns to resolve every member at startup: not-ready
			// addresses must be published early, or worker DNS lookups keep failing until the join
			// timeout is exhausted (chicken-and-egg). The selector adds the engine-member label to
			// exclude the companion proxy (a dead 5801 address would slow discovery).
			PublishNotReadyAddresses: true,
			Selector:                 map[string]string{LabelClusterCodeKey: code, LabelEngineMemberKey: "true"},
			Ports: []corev1.ServicePort{
				{Name: "hazelcast-port", Port: int32(cfg.HazelcastPort), TargetPort: intstrPort(cfg.HazelcastPort)},
			},
		},
	}
	if err := upsertService(ctx, clientset, cfg.Namespace, clusterSvc); err != nil {
		return err
	}
	masterSvc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.MasterSvc,
			Namespace: cfg.Namespace,
			Labels:    sessionLabels(code, clusterID, "master-service"),
		},
		Spec: corev1.ServiceSpec{
			ClusterIP: corev1.ClusterIPNone,
			// Hybrid 形态没有独立 master Pod，REST/WebUI 流量必须落到 hybrid Pod，
			// 否则 Endpoints 为空导致 /overview 与作业提交全 503。
			// Hybrid topology has no dedicated master pod, so REST/WebUI traffic must land
			// on hybrid pods; otherwise endpoints stay empty and /overview plus job
			// submission all 503.
			Selector: podSelectorLabels(code, masterServiceComponent(cfg)),
			Ports: []corev1.ServicePort{
				{Name: "http", Port: int32(cfg.MasterHTTPPort), TargetPort: intstrPort(cfg.MasterHTTPPort)},
			},
		},
	}
	return upsertService(ctx, clientset, cfg.Namespace, masterSvc)
}

func upsertService(ctx context.Context, clientset kubernetes.Interface, ns string, svc *corev1.Service) error {
	existing, err := clientset.CoreV1().Services(ns).Get(ctx, svc.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		if _, err := clientset.CoreV1().Services(ns).Create(ctx, svc, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("k8s: create service %q failed: %w", svc.Name, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("k8s: get service %q failed: %w", svc.Name, err)
	}
	if !isOwnedByCluster(existing.Labels, svc.Labels[LabelClusterCodeKey]) {
		return errResourceConflict("service", svc.Name)
	}
	existing.Spec.Ports = svc.Spec.Ports
	existing.Spec.Selector = svc.Spec.Selector
	// ClusterIP 分配后不可变，更新必须保留已有值；仅新建 Headless 时才写 None。
	// ClusterIP is immutable once allocated, so updates must keep the existing value;
	// only freshly created headless services carry None.
	existing.Spec.PublishNotReadyAddresses = svc.Spec.PublishNotReadyAddresses
	if existing.Labels == nil {
		existing.Labels = map[string]string{}
	}
	for k, v := range svc.Labels {
		existing.Labels[k] = v
	}
	if _, err := clientset.CoreV1().Services(ns).Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("k8s: update service %q failed: %w", svc.Name, err)
	}
	return nil
}

func intstrPort(port int) intstr.IntOrString {
	return intstr.FromInt32(int32(port))
}

func ensureSessionStatefulSet(ctx context.Context, clientset kubernetes.Interface, cfg *SessionConfig, names *SessionResourceNames, code string, clusterID uint, role string, createRBAC bool) error {
	var stsName string
	var replicas int
	var req ResourceRequirement
	switch role {
	case "master":
		stsName = names.MasterSTS
		replicas = cfg.MasterReplicas
		req = cfg.Resources.Master
	case "hybrid":
		// hybrid 单 StatefulSet 同时承载 master+worker 角色（Zeta master_and_worker）。
		// Hybrid runs one StatefulSet carrying both roles (Zeta master_and_worker).
		stsName = names.MasterSTS + "-hybrid"
		if cfg.HybridWorkloadName != "" {
			stsName = cfg.HybridWorkloadName
		}
		replicas = cfg.HybridReplicas
		if replicas <= 0 {
			replicas = 2
		}
		if cfg.Resources.Hybrid != nil {
			req = *cfg.Resources.Hybrid
		}
	default:
		stsName = names.WorkerSTS
		replicas = cfg.WorkerReplicas
		req = cfg.Resources.Worker
	}

	selectorLabels := podSelectorLabels(code, role)
	// 模板标签必须包含全部 selector 标签（K8s 强制 selector⊆template.labels），
	// 在 sessionLabels 基础上合并而不是替换，保留 cluster-id/managed-by 等管理面标签。
	// Template labels must include every selector label (K8s enforces selector⊆template.labels);
	// merge into sessionLabels instead of replacing so management labels like
	// cluster-id/managed-by survive.
	podLabels := sessionLabels(code, clusterID, role)
	for k, v := range selectorLabels {
		podLabels[k] = v
	}

	volumeMounts := []corev1.VolumeMount{}
	for _, key := range sessionConfigMountKeys(cfg) {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "seatunnel-configs",
			MountPath: "/opt/seatunnel/config/" + key,
			SubPath:   key,
		})
	}
	volumes := []corev1.Volume{
		{
			Name: "seatunnel-configs",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: names.ConfigMap},
				},
			},
		},
	}
	if (role == "master" || role == "hybrid") && strings.TrimSpace(cfg.Storage.CheckpointPVC) != "" {
		volumes = append(volumes, corev1.Volume{
			Name: "checkpoint-data",
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: strings.TrimSpace(cfg.Storage.CheckpointPVC),
				},
			},
		})
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      "checkpoint-data",
			MountPath: "/tmp/seatunnel",
		})
	}

	ports := []corev1.ContainerPort{
		{Name: "hazelcast-port", ContainerPort: int32(cfg.HazelcastPort)},
	}
	if role == "master" || role == "hybrid" {
		ports = append(ports, corev1.ContainerPort{Name: "http", ContainerPort: int32(cfg.MasterHTTPPort)})
	}

	tcpProbe := func() *corev1.Probe {
		return &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				TCPSocket: &corev1.TCPSocketAction{Port: intstrPort(cfg.HazelcastPort)},
			},
			InitialDelaySeconds: 30,
			PeriodSeconds:       30,
			TimeoutSeconds:      5,
			FailureThreshold:    3,
			SuccessThreshold:    1,
		}
	}
	liveness := tcpProbe()
	readiness := tcpProbe()
	if role == "master" || role == "hybrid" {
		readiness = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/overview",
					Port: intstrPort(cfg.MasterHTTPPort),
				},
			},
			InitialDelaySeconds: 30,
			PeriodSeconds:       30,
			TimeoutSeconds:      5,
			FailureThreshold:    3,
			SuccessThreshold:    1,
		}
	}

	container := corev1.Container{
		Name:            stsName,
		Image:           cfg.Image,
		ImagePullPolicy: corev1.PullPolicy(cfg.ImagePullPolicy),
		Command:         []string{"/opt/seatunnel/bin/seatunnel-cluster.sh", "-r", engineRoleArg(role)},
		Ports:           ports,
		VolumeMounts:    volumeMounts,
		LivenessProbe:   liveness,
		ReadinessProbe:  readiness,
	}
	if req.CPU != "" || req.Memory != "" {
		resources := corev1.ResourceRequirements{
			Requests: corev1.ResourceList{},
			Limits:   corev1.ResourceList{},
		}
		if req.CPU != "" {
			q, err := resource.ParseQuantity(req.CPU)
			if err != nil {
				return fmt.Errorf("k8s: invalid %s cpu quantity %q: %w", role, req.CPU, err)
			}
			resources.Requests[corev1.ResourceCPU] = q
			resources.Limits[corev1.ResourceCPU] = q
		}
		if req.Memory != "" {
			q, err := resource.ParseQuantity(req.Memory)
			if err != nil {
				return fmt.Errorf("k8s: invalid %s memory quantity %q: %w", role, req.Memory, err)
			}
			resources.Requests[corev1.ResourceMemory] = q
			resources.Limits[corev1.ResourceMemory] = q
		}
		container.Resources = resources
	}

	pullSecrets := []corev1.LocalObjectReference{}
	for _, name := range cfg.ImagePullSecrets {
		if strings.TrimSpace(name) != "" {
			pullSecrets = append(pullSecrets, corev1.LocalObjectReference{Name: strings.TrimSpace(name)})
		}
	}

	serviceAccount := cfg.ServiceAccount
	if serviceAccount == "" && createRBAC {
		serviceAccount = names.ServiceAccount
	}

	gracePeriod := int64(120)
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      stsName,
			Namespace: cfg.Namespace,
			Labels:    sessionLabels(code, clusterID, role),
		},
		Spec: appsv1.StatefulSetSpec{
			ServiceName:         names.MasterSvc,
			Replicas:            int32ptr(int32(replicas)),
			PodManagementPolicy: appsv1.ParallelPodManagement,
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type: appsv1.RollingUpdateStatefulSetStrategyType,
			},
			Selector: &metav1.LabelSelector{MatchLabels: selectorLabels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: podLabels},
				Spec: corev1.PodSpec{
					ServiceAccountName:            serviceAccount,
					TerminationGracePeriodSeconds: &gracePeriod,
					ImagePullSecrets:              pullSecrets,
					Containers:                    []corev1.Container{container},
					Volumes:                       volumes,
				},
			},
		},
	}
	if role == "worker" {
		sts.Spec.ServiceName = names.ClusterSvc
	}

	existing, err := clientset.AppsV1().StatefulSets(cfg.Namespace).Get(ctx, stsName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		if _, err := clientset.AppsV1().StatefulSets(cfg.Namespace).Create(ctx, sts, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("k8s: create statefulset %q failed: %w", stsName, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("k8s: get statefulset %q failed: %w", stsName, err)
	}
	if !isOwnedByCluster(existing.Labels, code) {
		return errResourceConflict("statefulset", stsName)
	}
	existing.Spec.Replicas = sts.Spec.Replicas
	existing.Spec.Template = sts.Spec.Template
	existing.Spec.ServiceName = sts.Spec.ServiceName
	if existing.Labels == nil {
		existing.Labels = map[string]string{}
	}
	for k, v := range sts.Labels {
		existing.Labels[k] = v
	}
	if _, err := clientset.AppsV1().StatefulSets(cfg.Namespace).Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
		return fmt.Errorf("k8s: update statefulset %q failed: %w", stsName, err)
	}
	return nil
}

// Uninstall 卸载 stx_deployed 创建的 K8s 资源（StatefulSet/Service/ConfigMap/RBAC）；
// 严格按确定性命名删除且忽略 NotFound，绝不删除任何 PVC（含外部 Checkpoint PVC）。
// Uninstall removes resources created by stx_deployed (StatefulSet/Service/ConfigMap/RBAC),
// deleting strictly by deterministic names while ignoring NotFound, and never deletes any PVC
// (including external checkpoint PVCs).
func (s *SessionManager) Uninstall(ctx context.Context, cfg *SessionConfig, clusterCode string) error {
	clientset, err := s.manager.Clientset(ctx, cfg.HostID)
	if err != nil {
		return err
	}
	ns := cfg.Namespace
	names := ResourceNamesFor(clusterCode)
	var errs []string
	var forbidden []string
	scaleZeroPayload := []byte(`{"spec":{"replicas":0}}`)
	deleteOne := func(kind string, del func() error, onForbidden func()) {
		err := del()
		if err == nil || apierrors.IsNotFound(err) {
			return
		}
		if apierrors.IsForbidden(err) {
			if onForbidden != nil {
				onForbidden()
			}
			forbidden = append(forbidden, kind)
			return
		}
		errs = append(errs, fmt.Sprintf("%s: %v", kind, err))
	}
	policy := metav1.DeletePropagationBackground
	// 删除前一律先 Get 校验归属标签：同名但非本集群创建的用户资源绝不删除。
	// 单命名空间最小权限凭据对集群/RBAC 对象返回 403 时视作「无权且未代建」，忽略并继续回收其余工作负载。
	// Every delete first Gets the object and verifies ownership labels: a same-named
	// user resource not created by this cluster is never deleted. When RBAC reads return
	// 403 under single-namespace minimal mode, skip gracefully so workload teardown proceeds.
	deleteOneRBAC := func(kind string, del func() error) {
		if err := del(); err != nil && !apierrors.IsNotFound(err) && !apierrors.IsForbidden(err) {
			errs = append(errs, fmt.Sprintf("%s: %v", kind, err))
		}
	}
	scaleSTSZero := func(name string) {
		_, _ = clientset.AppsV1().StatefulSets(ns).Patch(ctx, name, types.MergePatchType, scaleZeroPayload, metav1.PatchOptions{})
	}
	scaleDeployZero := func(name string) {
		_, _ = clientset.AppsV1().Deployments(ns).Patch(ctx, name, types.MergePatchType, scaleZeroPayload, metav1.PatchOptions{})
	}
	deleteOne("statefulset/"+names.MasterSTS, func() error {
		obj, err := clientset.AppsV1().StatefulSets(ns).Get(ctx, names.MasterSTS, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if !isOwnedByCluster(obj.Labels, clusterCode) {
			return nil
		}
		return clientset.AppsV1().StatefulSets(ns).Delete(ctx, names.MasterSTS, metav1.DeleteOptions{PropagationPolicy: &policy})
	}, func() { scaleSTSZero(names.MasterSTS) })
	deleteOne("statefulset/"+names.WorkerSTS, func() error {
		obj, err := clientset.AppsV1().StatefulSets(ns).Get(ctx, names.WorkerSTS, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if !isOwnedByCluster(obj.Labels, clusterCode) {
			return nil
		}
		return clientset.AppsV1().StatefulSets(ns).Delete(ctx, names.WorkerSTS, metav1.DeleteOptions{PropagationPolicy: &policy})
	}, func() { scaleSTSZero(names.WorkerSTS) })
	deleteOne("statefulset/"+names.MasterSTS+"-hybrid", func() error {
		obj, err := clientset.AppsV1().StatefulSets(ns).Get(ctx, names.MasterSTS+"-hybrid", metav1.GetOptions{})
		if err != nil {
			return err
		}
		if !isOwnedByCluster(obj.Labels, clusterCode) {
			return nil
		}
		return clientset.AppsV1().StatefulSets(ns).Delete(ctx, names.MasterSTS+"-hybrid", metav1.DeleteOptions{PropagationPolicy: &policy})
	}, func() { scaleSTSZero(names.MasterSTS + "-hybrid") })
	deleteOne("service/"+names.ClusterSvc, func() error {
		obj, err := clientset.CoreV1().Services(ns).Get(ctx, names.ClusterSvc, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if !isOwnedByCluster(obj.Labels, clusterCode) {
			return nil
		}
		return clientset.CoreV1().Services(ns).Delete(ctx, names.ClusterSvc, metav1.DeleteOptions{})
	}, nil)
	deleteOne("service/"+names.MasterSvc, func() error {
		obj, err := clientset.CoreV1().Services(ns).Get(ctx, names.MasterSvc, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if !isOwnedByCluster(obj.Labels, clusterCode) {
			return nil
		}
		return clientset.CoreV1().Services(ns).Delete(ctx, names.MasterSvc, metav1.DeleteOptions{})
	}, nil)
	deleteOne("configmap/"+names.ConfigMap, func() error {
		obj, err := clientset.CoreV1().ConfigMaps(ns).Get(ctx, names.ConfigMap, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if !isOwnedByCluster(obj.Labels, clusterCode) {
			return nil
		}
		return clientset.CoreV1().ConfigMaps(ns).Delete(ctx, names.ConfigMap, metav1.DeleteOptions{})
	}, nil)
	deleteOneRBAC("serviceaccount/"+names.ServiceAccount, func() error {
		obj, err := clientset.CoreV1().ServiceAccounts(ns).Get(ctx, names.ServiceAccount, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if !isOwnedByCluster(obj.Labels, clusterCode) {
			return nil
		}
		return clientset.CoreV1().ServiceAccounts(ns).Delete(ctx, names.ServiceAccount, metav1.DeleteOptions{})
	})
	deleteOneRBAC("secret/"+names.RegistryCredentialSecret, func() error {
		obj, err := clientset.CoreV1().Secrets(ns).Get(ctx, names.RegistryCredentialSecret, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if !isOwnedByCluster(obj.Labels, clusterCode) {
			return nil
		}
		return clientset.CoreV1().Secrets(ns).Delete(ctx, names.RegistryCredentialSecret, metav1.DeleteOptions{})
	})
	deleteOneRBAC("role/"+names.Role, func() error {
		obj, err := clientset.RbacV1().Roles(ns).Get(ctx, names.Role, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if !isOwnedByCluster(obj.Labels, clusterCode) {
			return nil
		}
		return clientset.RbacV1().Roles(ns).Delete(ctx, names.Role, metav1.DeleteOptions{})
	})
	deleteOneRBAC("rolebinding/"+names.RoleBinding, func() error {
		obj, err := clientset.RbacV1().RoleBindings(ns).Get(ctx, names.RoleBinding, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if !isOwnedByCluster(obj.Labels, clusterCode) {
			return nil
		}
		return clientset.RbacV1().RoleBindings(ns).Delete(ctx, names.RoleBinding, metav1.DeleteOptions{})
	})
	// 伴生 Proxy 残留一并清理（uninstall 语义是全量回收 STX 资源）。
	// Companion proxy leftovers are cleaned up too (uninstall means full reclamation of STX resources).
	proxyDeployment, proxyService, proxyJarCM := ProxyResourceNames(clusterCode)
	deleteOne("deployment/"+proxyDeployment, func() error {
		obj, err := clientset.AppsV1().Deployments(ns).Get(ctx, proxyDeployment, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if !isOwnedByCluster(obj.Labels, clusterCode) {
			return nil
		}
		return clientset.AppsV1().Deployments(ns).Delete(ctx, proxyDeployment, metav1.DeleteOptions{PropagationPolicy: &policy})
	}, func() { scaleDeployZero(proxyDeployment) })
	deleteOne("service/"+proxyService, func() error {
		obj, err := clientset.CoreV1().Services(ns).Get(ctx, proxyService, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if !isOwnedByCluster(obj.Labels, clusterCode) {
			return nil
		}
		return clientset.CoreV1().Services(ns).Delete(ctx, proxyService, metav1.DeleteOptions{})
	}, nil)
	deleteOne("configmap/"+proxyJarCM, func() error {
		obj, err := clientset.CoreV1().ConfigMaps(ns).Get(ctx, proxyJarCM, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if !isOwnedByCluster(obj.Labels, clusterCode) {
			return nil
		}
		return clientset.CoreV1().ConfigMaps(ns).Delete(ctx, proxyJarCM, metav1.DeleteOptions{})
	}, nil)
	if len(forbidden) > 0 {
		msg := fmt.Sprintf("k8s: workload delete forbidden (%s); scaled workloads to 0 where permitted — run manually: %s",
			strings.Join(forbidden, ", "), ManualCleanupCommand(ns, clusterCode))
		if len(errs) > 0 {
			return fmt.Errorf("%s; additional errors: %s", msg, strings.Join(errs, "; "))
		}
		return fmt.Errorf("%s", msg)
	}
	if len(errs) > 0 {
		return fmt.Errorf("k8s: uninstall partially failed: %s", strings.Join(errs, "; "))
	}
	return nil
}

func int32ptr(v int32) *int32 { return &v }

type dockerConfigAuthEntry struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Auth     string `json:"auth"`
}

type dockerConfigPayload struct {
	Auths map[string]dockerConfigAuthEntry `json:"auths"`
}

// EnsureRegistryCredentialSecret 为私有仓库镜像代建 dockerconfigjson 拉取凭据 Secret：
// 命名按集群确定性生成并打归属标签，随 Uninstall 级联回收；已存在且归属本集群时
// 保留既有 auths 条目并合并更新当前仓库凭据，避免跨仓库滚动升级期间旧 Pod 失联。
// EnsureRegistryCredentialSecret materializes a dockerconfigjson pull secret for private
// registry images: the name is deterministic per cluster and carries ownership labels so
// Uninstall cascades it; an existing cluster-owned secret merges new host entries while
// keeping existing ones so cross-registry rolling upgrades do not break old pods.
func EnsureRegistryCredentialSecret(ctx context.Context, clientset kubernetes.Interface, namespace, clusterCode, host, username, password string) (string, error) {
	names := ResourceNamesFor(clusterCode)
	secretName := names.RegistryCredentialSecret
	authKey := DockerConfigAuthKey(host)
	if authKey == "" || username == "" || password == "" {
		return "", fmt.Errorf("k8s: registry credential requires host/username/password")
	}
	auth := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
	entry := dockerConfigAuthEntry{
		Username: username,
		Password: password,
		Auth:     auth,
	}
	labels := map[string]string{
		"app.kubernetes.io/managed-by": "stx",
		LabelClusterCodeKey:            clusterCode,
	}
	existing, err := clientset.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		payload, marshalErr := json.Marshal(dockerConfigPayload{
			Auths: map[string]dockerConfigAuthEntry{authKey: entry},
		})
		if marshalErr != nil {
			return "", fmt.Errorf("k8s: marshal dockerconfigjson: %w", marshalErr)
		}
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: namespace, Labels: labels},
			Type:       corev1.SecretTypeDockerConfigJson,
			Data:       map[string][]byte{corev1.DockerConfigJsonKey: payload},
		}
		if _, err := clientset.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{}); err != nil {
			return "", fmt.Errorf("k8s: create registry credential secret: %w", err)
		}
		return secretName, nil
	}
	if err != nil {
		return "", fmt.Errorf("k8s: get registry credential secret: %w", err)
	}
	if !isOwnedByCluster(existing.Labels, clusterCode) {
		return "", errResourceConflict("secret", secretName)
	}
	merged := dockerConfigPayload{Auths: map[string]dockerConfigAuthEntry{}}
	if raw := existing.Data[corev1.DockerConfigJsonKey]; len(raw) > 0 {
		_ = json.Unmarshal(raw, &merged)
		if merged.Auths == nil {
			merged.Auths = map[string]dockerConfigAuthEntry{}
		}
	}
	merged.Auths[authKey] = entry
	payload, marshalErr := json.Marshal(merged)
	if marshalErr != nil {
		return "", fmt.Errorf("k8s: marshal merged dockerconfigjson: %w", marshalErr)
	}
	existing.Type = corev1.SecretTypeDockerConfigJson
	if existing.Data == nil {
		existing.Data = map[string][]byte{}
	}
	existing.Data[corev1.DockerConfigJsonKey] = payload
	if _, err := clientset.CoreV1().Secrets(namespace).Update(ctx, existing, metav1.UpdateOptions{}); err != nil {
		return "", fmt.Errorf("k8s: update registry credential secret: %w", err)
	}
	return secretName, nil
}

// DockerConfigAuthKey 返回写入 .dockerconfigjson auths 的规范键名（docker.io 映射为官方 index 地址）。
// DockerConfigAuthKey maps docker.io aliases to https://index.docker.io/v1/ and keeps private hosts bare.
func DockerConfigAuthKey(host string) string {
	trimmed := SanitizeRegistryHost(host)
	if strings.EqualFold(trimmed, "docker.io") || strings.EqualFold(trimmed, "index.docker.io") || strings.EqualFold(trimmed, "registry-1.docker.io") {
		return "https://index.docker.io/v1/"
	}
	return strings.ToLower(trimmed)
}

// SanitizeRegistryHost 清洗 registry host：剥离误带的 http://、https:// 与尾部斜杠，
// dockerconfigjson 的 auths 键必须是纯 host[:port]，带 scheme 会静默不匹配导致拉取认证失败。
// SanitizeRegistryHost strips an accidental http:// or https:// scheme and trailing
// slashes: dockerconfigjson auths keys must be bare host[:port], and a scheme silently
// mismatches so pulls fail authentication.
func SanitizeRegistryHost(host string) string {
	trimmed := strings.TrimSpace(host)
	trimmed = strings.TrimPrefix(trimmed, "http://")
	trimmed = strings.TrimPrefix(trimmed, "https://")
	return strings.TrimRight(trimmed, "/")
}
