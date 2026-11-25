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
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
)

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
	LabelClusterIDKey   = "stx.seatunnel.apache.org/cluster-id"
	LabelComponentKey   = "app.kubernetes.io/component"
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
}

// ResourceNamesFor 计算指定 cluster code 的资源命名。
// ResourceNamesFor computes resource names for a cluster code.
func ResourceNamesFor(clusterCode string) *SessionResourceNames {
	return &SessionResourceNames{
		ConfigMap:      clusterCode + "-configs",
		ClusterSvc:     clusterCode,
		MasterSvc:      clusterCode + "-master",
		MasterSTS:      clusterCode + "-master",
		WorkerSTS:      clusterCode + "-worker",
		ServiceAccount: clusterCode,
		Role:           clusterCode,
		RoleBinding:    clusterCode,
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

	if err := ensureSessionStatefulSet(ctx, clientset, cfg, names, code, opts.ClusterID, "master"); err != nil {
		return nil, err
	}
	track("statefulset", names.MasterSTS)
	if err := ensureSessionStatefulSet(ctx, clientset, cfg, names, code, opts.ClusterID, "worker"); err != nil {
		return nil, err
	}
	track("statefulset", names.WorkerSTS)
	return result, nil
}

func ensureNamespace(ctx context.Context, clientset kubernetes.Interface, ns string) error {
	_, err := clientset.CoreV1().Namespaces().Get(ctx, ns, metav1.GetOptions{})
	if err == nil {
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("k8s: get namespace %q failed: %w", ns, err)
	}
	// STX 创建的 Namespace 打标，卸载时仅删除带标 Namespace；用户预建的无标 Namespace 永不删除。
	// STX-created namespaces are labeled so uninstall only removes labeled ones; pre-existing
	// user namespaces are never deleted.
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
	if _, err := clientset.CoreV1().ServiceAccounts(ns).Create(ctx, sa, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("k8s: create serviceaccount failed: %w", err)
	}

	// Hazelcast K8s 发现的 DNS 模式只需要 endpoints/pods 只读；services patch 预留给 App Master HTTP 自发布（P2）。
	// Hazelcast DNS discovery only needs read access to endpoints/pods; services patch is reserved
	// for the App master HTTP self-publish flow (P2).
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: names.Role, Namespace: ns, Labels: labels},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"endpoints", "pods", "services"},
				Verbs:     []string{"get", "list", "watch"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"services"},
				Verbs:     []string{"patch"},
			},
		},
	}
	if _, err := clientset.RbacV1().Roles(ns).Create(ctx, role, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("k8s: create role failed: %w", err)
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
	if _, err := clientset.RbacV1().RoleBindings(ns).Create(ctx, binding, metav1.CreateOptions{}); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("k8s: create rolebinding failed: %w", err)
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
			Selector:  podSelectorLabels(code, "master"),
			Ports: []corev1.ServicePort{
				{Name: "master-port", Port: int32(cfg.MasterHTTPPort), TargetPort: intstrPort(cfg.MasterHTTPPort)},
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
	existing.Spec.Ports = svc.Spec.Ports
	existing.Spec.Selector = svc.Spec.Selector
	existing.Spec.ClusterIP = svc.Spec.ClusterIP
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

func ensureSessionStatefulSet(ctx context.Context, clientset kubernetes.Interface, cfg *SessionConfig, names *SessionResourceNames, code string, clusterID uint, role string) error {
	var stsName string
	var replicas int
	var req ResourceRequirement
	if role == "master" {
		stsName = names.MasterSTS
		replicas = cfg.MasterReplicas
		req = cfg.Resources.Master
	} else {
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
	for _, key := range sessionConfigMountKeys() {
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
	if role == "master" && strings.TrimSpace(cfg.Storage.CheckpointPVC) != "" {
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
	if role == "master" {
		ports = append(ports, corev1.ContainerPort{Name: "master-port", ContainerPort: int32(cfg.MasterHTTPPort)})
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
	if role == "master" {
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
		Command:         []string{"/opt/seatunnel/bin/seatunnel-cluster.sh", "-r", role},
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
	if serviceAccount == "" {
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
	deleteOne := func(kind string, del func() error) {
		if err := del(); err != nil && !apierrors.IsNotFound(err) {
			errs = append(errs, fmt.Sprintf("%s: %v", kind, err))
		}
	}
	policy := metav1.DeletePropagationBackground
	deleteOne("statefulset/"+names.MasterSTS, func() error {
		return clientset.AppsV1().StatefulSets(ns).Delete(ctx, names.MasterSTS, metav1.DeleteOptions{PropagationPolicy: &policy})
	})
	deleteOne("statefulset/"+names.WorkerSTS, func() error {
		return clientset.AppsV1().StatefulSets(ns).Delete(ctx, names.WorkerSTS, metav1.DeleteOptions{PropagationPolicy: &policy})
	})
	deleteOne("service/"+names.ClusterSvc, func() error {
		return clientset.CoreV1().Services(ns).Delete(ctx, names.ClusterSvc, metav1.DeleteOptions{})
	})
	deleteOne("service/"+names.MasterSvc, func() error {
		return clientset.CoreV1().Services(ns).Delete(ctx, names.MasterSvc, metav1.DeleteOptions{})
	})
	deleteOne("configmap/"+names.ConfigMap, func() error {
		return clientset.CoreV1().ConfigMaps(ns).Delete(ctx, names.ConfigMap, metav1.DeleteOptions{})
	})
	deleteOne("serviceaccount/"+names.ServiceAccount, func() error {
		return clientset.CoreV1().ServiceAccounts(ns).Delete(ctx, names.ServiceAccount, metav1.DeleteOptions{})
	})
	deleteOne("role/"+names.Role, func() error {
		return clientset.RbacV1().Roles(ns).Delete(ctx, names.Role, metav1.DeleteOptions{})
	})
	deleteOne("rolebinding/"+names.RoleBinding, func() error {
		return clientset.RbacV1().RoleBindings(ns).Delete(ctx, names.RoleBinding, metav1.DeleteOptions{})
	})
	// 伴生 Proxy 残留一并清理（uninstall 语义是全量回收 STX 资源）。
	// Companion proxy leftovers are cleaned up too (uninstall means full reclamation of STX resources).
	proxyDeployment, proxyService, proxyJarCM := ProxyResourceNames(clusterCode)
	deleteOne("deployment/"+proxyDeployment, func() error {
		return clientset.AppsV1().Deployments(ns).Delete(ctx, proxyDeployment, metav1.DeleteOptions{PropagationPolicy: &policy})
	})
	deleteOne("service/"+proxyService, func() error {
		return clientset.CoreV1().Services(ns).Delete(ctx, proxyService, metav1.DeleteOptions{})
	})
	deleteOne("configmap/"+proxyJarCM, func() error {
		return clientset.CoreV1().ConfigMaps(ns).Delete(ctx, proxyJarCM, metav1.DeleteOptions{})
	})
	if nsObj, getErr := clientset.CoreV1().Namespaces().Get(ctx, ns, metav1.GetOptions{}); getErr == nil {
		if nsObj.Labels[LabelManagedByKey] == LabelManagedByValue {
			deleteOne("namespace/"+ns, func() error {
				return clientset.CoreV1().Namespaces().Delete(ctx, ns, metav1.DeleteOptions{})
			})
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("k8s: uninstall partially failed: %s", strings.Join(errs, "; "))
	}
	return nil
}

func int32ptr(v int32) *int32 { return &v }
