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
	"strings"

	"github.com/LeonYoah/stx/internal/pkg/k8s"
	"github.com/LeonYoah/stx/internal/pkg/locale"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// verifyAdoptedK8sObjectsExist 纳管前真查 namespace/workload/service/configmap，
// 返回缺失对象的可读列表（空切片表示全部存在）。
// verifyAdoptedK8sObjectsExist verifies namespace/workload/service/configmap exist
// before adoption, returning a readable list of missing objects (empty means all present).
func verifyAdoptedK8sObjectsExist(ctx context.Context, mgr *k8s.ClientManager, cfg *k8s.SessionConfig) []string {
	missing := make([]string, 0)
	if cfg == nil || mgr == nil || cfg.HostID == 0 {
		return missing
	}
	clientset, err := mgr.Clientset(ctx, cfg.HostID)
	if err != nil {
		missing = append(missing, fmt.Sprintf("kube-client: %v", err))
		return missing
	}
	ns := cfg.Namespace
	if _, err := clientset.CoreV1().Namespaces().Get(ctx, ns, metav1.GetOptions{}); err != nil && !apierrors.IsForbidden(err) {
		// 单命名空间最小权限凭据无 namespaces get 权限，此时允许 403 并继续验证命名空间内对象存在性。
		// Single-namespace minimal credentials lack cluster-scoped namespaces get permission; allow 403
		// and rely on the subsequent namespace-scoped object checks.
		if apierrors.IsNotFound(err) {
			missing = append(missing, fmt.Sprintf("namespace/%s", ns))
		} else {
			missing = append(missing, fmt.Sprintf("namespace/%s: %v", ns, err))
		}
		return missing
	}
	type namedObject struct {
		kind string
		name string
	}
	objects := make([]namedObject, 0)
	if cfg.IsHybrid() {
		if cfg.HybridWorkloadName != "" {
			objects = append(objects, namedObject{"statefulset", cfg.HybridWorkloadName})
		}
	} else {
		if cfg.MasterWorkloadName != "" {
			objects = append(objects, namedObject{"statefulset", cfg.MasterWorkloadName})
		}
		if cfg.WorkerWorkloadName != "" {
			objects = append(objects, namedObject{"statefulset", cfg.WorkerWorkloadName})
		}
	}
	if cfg.MasterServiceName != "" {
		objects = append(objects, namedObject{"service", cfg.MasterServiceName})
	}
	if cfg.ConfigMapName != "" {
		objects = append(objects, namedObject{"configmap", cfg.ConfigMapName})
	}
	for _, obj := range objects {
		var getErr error
		switch obj.kind {
		case "statefulset":
			_, getErr = clientset.AppsV1().StatefulSets(ns).Get(ctx, obj.name, metav1.GetOptions{})
		case "service":
			_, getErr = clientset.CoreV1().Services(ns).Get(ctx, obj.name, metav1.GetOptions{})
		case "configmap":
			_, getErr = clientset.CoreV1().ConfigMaps(ns).Get(ctx, obj.name, metav1.GetOptions{})
		}
		if getErr != nil {
			if apierrors.IsNotFound(getErr) {
				missing = append(missing, fmt.Sprintf("%s/%s (ns %s)", obj.kind, obj.name, ns))
			} else {
				missing = append(missing, fmt.Sprintf("%s/%s: %v", obj.kind, obj.name, getErr))
			}
		}
	}
	return missing
}

// cleanupFailedK8sDeploy 中途失败回收：按归属标签（managed-by=stx + cluster code）逆序
// 删除 stx 自建资源，用户既有资源不带这两个标签天然不会被触碰。
// cleanupFailedK8sDeploy reclaims a failed deploy: stx-created objects carry the
// ownership labels (managed-by=stx + cluster code) and are deleted in reverse order;
// user resources never carry both labels and are therefore never touched.
func cleanupFailedK8sDeploy(ctx context.Context, mgr *k8s.ClientManager, cfg *k8s.SessionConfig, clusterCode string) error {
	if cfg == nil || mgr == nil || cfg.HostID == 0 {
		return nil
	}
	clientset, err := mgr.Clientset(ctx, cfg.HostID)
	if err != nil {
		return err
	}
	ns := cfg.Namespace
	selector := "app.kubernetes.io/managed-by=stx," + k8s.LabelClusterCodeKey + "=" + clusterCode
	listOpts := metav1.ListOptions{LabelSelector: selector}
	deletePolicy := metav1.DeletePropagationBackground
	deleteOpts := metav1.DeleteOptions{PropagationPolicy: &deletePolicy}
	failures := make([]string, 0)
	forbidden := make([]string, 0)
	scaleZeroPayload := []byte(`{"spec":{"replicas":0}}`)
	remove := func(kind string, items []string) {
		// 逆序回收：后创建的先删，降低依赖悬空（workload 先于 service/configmap 建则反向）。
		// Reverse-order reclamation: later-created objects go first, reducing dangling
		// dependencies.
		for i := len(items) - 1; i >= 0; i-- {
			name := items[i]
			var delErr error
			switch kind {
			case "statefulset":
				delErr = clientset.AppsV1().StatefulSets(ns).Delete(ctx, name, deleteOpts)
			case "deployment":
				delErr = clientset.AppsV1().Deployments(ns).Delete(ctx, name, deleteOpts)
			case "service":
				delErr = clientset.CoreV1().Services(ns).Delete(ctx, name, deleteOpts)
			case "configmap":
				delErr = clientset.CoreV1().ConfigMaps(ns).Delete(ctx, name, deleteOpts)
			case "secret":
				delErr = clientset.CoreV1().Secrets(ns).Delete(ctx, name, deleteOpts)
			}
			if delErr != nil && !apierrors.IsNotFound(delErr) {
				if apierrors.IsForbidden(delErr) {
					if kind == "secret" {
						continue
					}
					// 无 delete 权限时尽力先缩容至 0，避免残留空转 Pod，并向运维输出精确清理命令。
					// Without delete permission, best-effort scale workloads to 0 and surface a manual kubectl command.
					switch kind {
					case "statefulset":
						_, _ = clientset.AppsV1().StatefulSets(ns).Patch(ctx, name, types.MergePatchType, scaleZeroPayload, metav1.PatchOptions{})
					case "deployment":
						_, _ = clientset.AppsV1().Deployments(ns).Patch(ctx, name, types.MergePatchType, scaleZeroPayload, metav1.PatchOptions{})
					}
					forbidden = append(forbidden, fmt.Sprintf("%s/%s", kind, name))
					continue
				}
				failures = append(failures, fmt.Sprintf("%s/%s: %v", kind, name, delErr))
			}
		}
	}
	// 回收顺序：statefulset/deployment（含伴生 proxy）→ service → configmap → secret。
	// Reclamation order: statefulsets/deployments (companion proxy included) → services →
	// configmaps → secrets.
	if list, err := clientset.AppsV1().StatefulSets(ns).List(ctx, listOpts); err == nil {
		names := make([]string, 0, len(list.Items))
		for _, item := range list.Items {
			names = append(names, item.Name)
		}
		remove("statefulset", names)
	} else {
		failures = append(failures, fmt.Sprintf("list statefulsets: %v", err))
	}
	if list, err := clientset.AppsV1().Deployments(ns).List(ctx, listOpts); err == nil {
		names := make([]string, 0, len(list.Items))
		for _, item := range list.Items {
			names = append(names, item.Name)
		}
		remove("deployment", names)
	} else {
		failures = append(failures, fmt.Sprintf("list deployments: %v", err))
	}
	if list, err := clientset.CoreV1().Services(ns).List(ctx, listOpts); err == nil {
		names := make([]string, 0, len(list.Items))
		for _, item := range list.Items {
			names = append(names, item.Name)
		}
		remove("service", names)
	}
	if list, err := clientset.CoreV1().ConfigMaps(ns).List(ctx, listOpts); err == nil {
		names := make([]string, 0, len(list.Items))
		for _, item := range list.Items {
			names = append(names, item.Name)
		}
		remove("configmap", names)
	}
	if list, err := clientset.CoreV1().Secrets(ns).List(ctx, listOpts); err == nil {
		names := make([]string, 0, len(list.Items))
		for _, item := range list.Items {
			names = append(names, item.Name)
		}
		remove("secret", names)
	}
	if len(forbidden) > 0 {
		cmd := k8s.ManualCleanupCommand(ns, clusterCode)
		msg := fmt.Sprintf(locale.Pick(ctx,
			"当前凭据缺少删除权限（已尽力将工作负载缩容至 0），请手动执行清理：%s（未删：%s）",
			"credential lacks delete permission (workloads scaled to 0 where permitted); run manually: %s (remaining: %s)"),
			cmd, strings.Join(forbidden, ", "))
		if len(failures) > 0 {
			return fmt.Errorf("%s；%s", msg, strings.Join(failures, "；"))
		}
		return fmt.Errorf("%s", msg)
	}
	if len(failures) > 0 {
		return fmt.Errorf("%s: %s", locale.Pick(ctx, "部分资源回收失败", "partial cleanup failure"), strings.Join(failures, "；"))
	}
	return nil
}
