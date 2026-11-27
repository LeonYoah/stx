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

// Package infra 定义全局集群部署模式与基础设施类型契约（零依赖叶子包），
// 对齐 Flink 与上游 Apache SeaTunnel 语义（Standalone / Kubernetes Session / Kubernetes Application /
// 预留 YARN Session & Application），作为后端各域判定运行底座与执行模式的唯一真源。
// Package infra defines the global cluster deployment mode and infrastructure type contract
// (zero-dependency leaf package), aligned with Flink and upstream Apache SeaTunnel semantics
// (Standalone / Kubernetes Session / Kubernetes Application / reserved YARN Session & Application).
package infra

import "strings"

// Type 集群基础设施类型，与 clusters.infra_type 列一一对应。
// Type enumerates cluster infrastructure types matching the clusters.infra_type column.
type Type string

const (
	// TypeBareMetal 主机常驻集群（Standalone，存量默认）。
	// TypeBareMetal is a host/VM Standalone cluster (default for existing rows).
	TypeBareMetal Type = "bare_metal"
	// TypeK8sSession Kubernetes Session 常驻集群（纳管 adopted 或一键部署 stx_deployed）。
	// TypeK8sSession is a Kubernetes Session long-running cluster (adopted or stx_deployed).
	TypeK8sSession Type = "kubernetes_session"
	// TypeK8sAppTemplate Kubernetes Application 模板（P2 预留）。
	// TypeK8sAppTemplate is a Kubernetes Application template (reserved for P2).
	TypeK8sAppTemplate Type = "kubernetes_application_template"
	// TypeK8sAppInstance Kubernetes Application 临时运行实例（P2 预留，自动登记）。
	// TypeK8sAppInstance is a Kubernetes Application ephemeral instance (reserved for P2, auto-registered).
	TypeK8sAppInstance Type = "kubernetes_application_instance"
	// TypeYarnSession YARN Session 常驻集群（P3 预留）。
	// TypeYarnSession is a YARN Session long-running cluster (reserved for P3).
	TypeYarnSession Type = "yarn_session"
	// TypeYarnAppTemplate YARN Application 模板（P3 预留）。
	// TypeYarnAppTemplate is a YARN Application template (reserved for P3).
	TypeYarnAppTemplate Type = "yarn_application_template"
	// TypeYarnAppInstance YARN Application 临时运行实例（P3 预留）。
	// TypeYarnAppInstance is a YARN Application ephemeral instance (reserved for P3).
	TypeYarnAppInstance Type = "yarn_application_instance"
)

// RuntimePlatform 资源调度底座类型（与执行模式正交）。
// RuntimePlatform identifies the underlying resource manager (orthogonal to execution mode).
type RuntimePlatform string

const (
	PlatformStandalone RuntimePlatform = "standalone"
	PlatformKubernetes RuntimePlatform = "kubernetes"
	PlatformYarn       RuntimePlatform = "yarn"
)

// ExecutionMode 集群生命周期执行模式（常驻共享 Session vs 单作业独立 Application）。
// ExecutionMode distinguishes long-running shared Session clusters from per-job Application clusters.
type ExecutionMode string

const (
	ExecutionModeSession     ExecutionMode = "session"
	ExecutionModeApplication ExecutionMode = "application"
)

// DeployMode 面向产品层的全局标准部署模式（运行底座 × 执行模式，对齐 Flink / SeaTunnel MasterType）。
// DeployMode is the canonical user-facing deployment mode (RuntimePlatform x ExecutionMode).
type DeployMode string

const (
	DeployModeStandalone     DeployMode = "standalone"
	DeployModeK8sSession     DeployMode = "kubernetes_session"
	DeployModeK8sApplication DeployMode = "kubernetes_application"
	DeployModeYarnSession    DeployMode = "yarn_session"
	DeployModeYarnApplication DeployMode = "yarn_application"
)

// legacyK8s 是过渡期写入的字面值，读取侧必须平滑兼容。
// legacyK8s is the transitional literal written before the contract settled;
// readers must tolerate it smoothly.
const legacyK8s = "kubernetes"

// Effective 兼容旧行：空值按 bare_metal 处理，避免调用方分支判断空串。
// Effective treats empty legacy rows as bare_metal so callers never branch on empty strings.
func Effective(v string) string {
	trimmed := strings.TrimSpace(v)
	if trimmed == "" {
		return string(TypeBareMetal)
	}
	return trimmed
}

// IsBareMetal 判断是否为 Standalone（物理机/虚拟机）底座。
// IsBareMetal reports whether the infra type represents a Standalone host/VM cluster.
func IsBareMetal(v string) bool {
	return Effective(v) == string(TypeBareMetal)
}

// IsK8s 判断是否为任意 Kubernetes 形态（含历史过渡值 kubernetes）。
// IsK8s reports whether the type is any Kubernetes shape, including the legacy literal.
func IsK8s(v string) bool {
	eff := Effective(v)
	return eff == legacyK8s || strings.HasPrefix(eff, "kubernetes_")
}

// IsK8sSession 判断是否为 Kubernetes Session 常驻集群（含历史过渡值）。
// IsK8sSession reports whether the type is a Kubernetes Session cluster, including the legacy literal.
func IsK8sSession(v string) bool {
	eff := Effective(v)
	return eff == string(TypeK8sSession) || eff == legacyK8s
}

// IsYarn 判断是否为任意 YARN 形态（P3 预留）。
// IsYarn reports whether the type is any YARN shape (reserved for P3).
func IsYarn(v string) bool {
	eff := Effective(v)
	return eff == "yarn" || strings.HasPrefix(eff, "yarn_")
}

// IsApplication 判断是否为 Application（单作业独立集群）模式。
// IsApplication reports whether the infra type belongs to an Application execution mode.
func IsApplication(v string) bool {
	eff := Effective(v)
	return eff == string(TypeK8sAppTemplate) ||
		eff == string(TypeK8sAppInstance) ||
		eff == string(TypeYarnAppTemplate) ||
		eff == string(TypeYarnAppInstance)
}

// IsSession 判断是否为 Session（常驻共享集群）模式。
// IsSession reports whether the infra type belongs to a long-running Session cluster.
func IsSession(v string) bool {
	return !IsApplication(v)
}

// PlatformOf 归一化解析运行底座（Standalone / Kubernetes / YARN）。
// PlatformOf returns the normalized RuntimePlatform for the given infra_type.
func PlatformOf(v string) RuntimePlatform {
	switch {
	case IsK8s(v):
		return PlatformKubernetes
	case IsYarn(v):
		return PlatformYarn
	default:
		return PlatformStandalone
	}
}

// ExecutionModeOf 归一化解析执行模式（Session / Application）。
// ExecutionModeOf returns the normalized ExecutionMode for the given infra_type.
func ExecutionModeOf(v string) ExecutionMode {
	if IsApplication(v) {
		return ExecutionModeApplication
	}
	return ExecutionModeSession
}

// DeployModeOf 返回面向用户的标准全局部署模式。
// DeployModeOf resolves the canonical user-facing DeployMode from infra_type.
func DeployModeOf(v string) DeployMode {
	switch PlatformOf(v) {
	case PlatformKubernetes:
		if IsApplication(v) {
			return DeployModeK8sApplication
		}
		return DeployModeK8sSession
	case PlatformYarn:
		if IsApplication(v) {
			return DeployModeYarnApplication
		}
		return DeployModeYarnSession
	default:
		return DeployModeStandalone
	}
}
