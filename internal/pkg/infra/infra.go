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

// Package infra 定义集群基础设施类型契约，归属公共层（叶子包，零依赖），
// 避免 cluster 域反向依赖 pkg/k8s，并为未来 yarn_* 形态预留扩展口子。
// Package infra defines the cluster infrastructure type contract in a leaf
// dependency-free package, avoiding reverse dependencies from the cluster domain
// into pkg/k8s and leaving room for future yarn_* shapes.
package infra

import "strings"

// Type 集群基础设施类型，与 clusters.infra_type 列一一对应。
// Type enumerates cluster infrastructure types matching the clusters.infra_type column.
type Type string

const (
	// TypeBareMetal 物理机/虚拟机集群（存量默认）。
	// TypeBareMetal is the legacy bare-metal/VM cluster (default for existing rows).
	TypeBareMetal Type = "bare_metal"
	// TypeK8sSession K8s 常驻容器集群（纳管 adopted 或一键部署 stx_deployed）。
	// TypeK8sSession is a K8s long-running session cluster (adopted or stx_deployed).
	TypeK8sSession Type = "kubernetes_session"
	// TypeK8sAppTemplate K8s App 模板（P2 预留）。
	// TypeK8sAppTemplate is a K8s application template (reserved for P2).
	TypeK8sAppTemplate Type = "kubernetes_application_template"
	// TypeK8sAppInstance K8s App 临时实例（P2 预留，自动登记）。
	// TypeK8sAppInstance is a K8s application ephemeral instance (reserved for P2, auto-registered).
	TypeK8sAppInstance Type = "kubernetes_application_instance"
)

// legacyK8s 是过渡期写入的字面值，读取侧必须平滑兼容。
// legacyK8s is the transitional literal written before the contract settled;
// readers must tolerate it smoothly.
const legacyK8s = "kubernetes"

// IsK8s 判断是否为任意 K8s 形态（含历史过渡值 kubernetes）。
// IsK8s reports whether the type is any Kubernetes shape, including the legacy literal.
func IsK8s(v string) bool {
	if v == legacyK8s {
		return true
	}
	return strings.HasPrefix(v, "kubernetes_")
}

// IsK8sSession 判断是否为 K8s 常驻容器集群（含历史过渡值）。
// IsK8sSession reports whether the type is a K8s session cluster, including the legacy literal.
func IsK8sSession(v string) bool {
	return v == string(TypeK8sSession) || v == legacyK8s
}

// Effective 兼容旧行：空值按 bare_metal 处理，避免前端分支判断空串。
// Effective treats empty legacy rows as bare_metal so frontends never branch on empty strings.
func Effective(v string) string {
	if v == "" {
		return string(TypeBareMetal)
	}
	return v
}
