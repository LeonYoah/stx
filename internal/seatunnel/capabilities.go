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

package seatunnel

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	// DefaultInstallerHistoryJobExpireMinutes is the STX installer default
	// for historical job retention when the target version supports it.
	// DefaultInstallerHistoryJobExpireMinutes 是 STX 安装器在目标版本支持时使用的历史作业保留默认值。
	DefaultInstallerHistoryJobExpireMinutes = 1440

	// DefaultInstallerScheduledDeletionEnable is the STX installer default
	// for auto deleting logs after historical DAG data expires.
	// DefaultInstallerScheduledDeletionEnable 是 STX 安装器在历史 DAG 过期后自动删除日志的默认值。
	DefaultInstallerScheduledDeletionEnable = true

	// DefaultInstallerDynamicSlot is the STX installer default for dynamic slot.
	// DefaultInstallerDynamicSlot 是 STX 安装器的动态 slot 默认值。
	DefaultInstallerDynamicSlot = true

	// DefaultInstallerStaticSlotNum is the explicit static slot default written by
	// STX so upgrades do not drift with upstream implicit defaults.
	// DefaultInstallerStaticSlotNum 是 STX 显式写入的静态 slot 默认值，用于避免升级后受上游隐式默认值漂移影响。
	DefaultInstallerStaticSlotNum = 2

	// DefaultInstallerJobScheduleStrategy is the STX installer default
	// for static-slot scheduling.
	// DefaultInstallerJobScheduleStrategy 是 STX 安装器在静态 slot 模式下的调度策略默认值。
	DefaultInstallerJobScheduleStrategy = "REJECT"

	// DefaultInstallerSlotAllocationStrategy is the STX installer default
	// for slot allocation strategy when the target version supports it.
	// DefaultInstallerSlotAllocationStrategy 是 STX 安装器在目标版本支持时使用的 slot 分配策略默认值。
	DefaultInstallerSlotAllocationStrategy = "RANDOM"
)

// JavaRequirement describes the JDK range a SeaTunnel release can run on.
// MinSupported is a hard floor (below it the engine cannot start); Recommended
// lists the versions treated as passed; anything above MaxRecommended is a
// compatibility warning; anything above MaxSupported is a hard failure because
// JDK 24+ removed javax.security.auth.Subject.getSubject (JEP 486), which the
// embedded Hadoop 3 calls during startup.
// JavaRequirement 描述某个 SeaTunnel 版本可用的 JDK 范围。MinSupported 是硬下限
// （低于它引擎无法启动）；Recommended 是判定为通过的版本；高于 MaxRecommended
// 属于兼容性警告；高于 MaxSupported 是硬失败，因为 JDK 24+ 已移除
// javax.security.auth.Subject.getSubject（JEP 486），内嵌 Hadoop 3 启动期必崩。
type JavaRequirement struct {
	MinSupported   int   `json:"min_supported"`
	Recommended    []int `json:"recommended"`
	MaxRecommended int   `json:"max_recommended"`
	// MaxSupported 是可启动 SeaTunnel 的最高 JDK 主版本（硬上限，与目标版本无关）。
	// MaxSupported is the highest JDK major that can start SeaTunnel (hard ceiling, version-independent).
	MaxSupported int `json:"max_supported"`
}

// JavaRequirementForVersion returns the JDK requirement of a SeaTunnel release.
// SeaTunnel 3.1.0 dropped Java 8 and requires Java 11 or 17; earlier releases
// (2.x / 3.0.x) run on Java 8 or 11. Unknown or empty versions keep the legacy
// 8/11 profile so existing flows are not newly blocked.
// JavaRequirementForVersion 返回某个 SeaTunnel 版本的 JDK 要求。SeaTunnel 3.1.0
// 移除了 Java 8，要求 Java 11 或 17；更早版本（2.x / 3.0.x）使用 Java 8 或 11。
// 未知或空版本沿用 8/11 要求，避免存量流程被新增阻断。
func JavaRequirementForVersion(seatunnelVersion string) JavaRequirement {
	if strings.TrimSpace(seatunnelVersion) != "" && CompareVersions(seatunnelVersion, "3.1.0") >= 0 {
		return JavaRequirement{MinSupported: 11, Recommended: []int{11, 17}, MaxRecommended: 17, MaxSupported: 23}
	}
	return JavaRequirement{MinSupported: 8, Recommended: []int{8, 11}, MaxRecommended: 11, MaxSupported: 23}
}

// RecommendedText joins Recommended with the caller-provided separator (e.g.
// " 或 " / " or ") for bilingual messages.
// RecommendedText 用调用方给定的连接词（如 " 或 " / " or "）拼接 Recommended，供双语消息使用。
func (r JavaRequirement) RecommendedText(sep string) string {
	parts := make([]string, 0, len(r.Recommended))
	for _, v := range r.Recommended {
		parts = append(parts, strconv.Itoa(v))
	}
	return strings.Join(parts, sep)
}

// Contains reports whether a Java major version is in the recommended set.
// Contains 判断 Java 主版本是否属于推荐集合。
func (r JavaRequirement) Contains(version int) bool {
	for _, v := range r.Recommended {
		if v == version {
			return true
		}
	}
	return false
}

// VersionCapabilities describes which advanced runtime settings are supported by
// one SeaTunnel release from the install wizard perspective.
// VersionCapabilities 描述安装向导视角下某个 SeaTunnel 版本支持的高级运行时配置能力。
type VersionCapabilities struct {
	SupportsDynamicSlot             bool `json:"supports_dynamic_slot"`
	SupportsSlotNum                 bool `json:"supports_slot_num"`
	SupportsHistoryJobExpireMinutes bool `json:"supports_history_job_expire_minutes"`
	SupportsScheduledDeletionEnable bool `json:"supports_scheduled_deletion_enable"`
	SupportsJobScheduleStrategy     bool `json:"supports_job_schedule_strategy"`
	SupportsSlotAllocationStrategy  bool `json:"supports_slot_allocation_strategy"`
	SupportsHTTPService             bool `json:"supports_http_service"`
	SupportsJobLogMode              bool `json:"supports_job_log_mode"`
	// SupportsEventChannel 表示一键安装可以写入 event-report-http（3.0.0 起，安装向导默认勾选）。
	// SupportsEventChannel means one-click install can write event-report-http (since 3.0.0; the wizard checks it by default).
	SupportsEventChannel bool `json:"supports_event_channel"`
	// DefaultEventChannelEnabled 是安装向导在该版本上的事件通道默认值。
	// DefaultEventChannelEnabled is the install wizard default for the event channel on this version.
	DefaultEventChannelEnabled     bool   `json:"default_event_channel_enabled"`
	DefaultDynamicSlot             bool   `json:"default_dynamic_slot"`
	DefaultStaticSlotNum           int    `json:"default_static_slot_num"`
	DefaultHistoryJobExpireMinutes int    `json:"default_history_job_expire_minutes"`
	DefaultScheduledDeletionEnable bool   `json:"default_scheduled_deletion_enable"`
	DefaultJobScheduleStrategy     string `json:"default_job_schedule_strategy"`
	DefaultSlotAllocationStrategy  string `json:"default_slot_allocation_strategy"`
	DefaultHTTPEnabled             bool   `json:"default_http_enabled"`
	DefaultJobLogMode              string `json:"default_job_log_mode"`
	// MinJavaVersion 是该版本可启动的最低 JDK 主版本（低于则引擎无法运行）。
	// MinJavaVersion is the lowest JDK major version that can start this release.
	MinJavaVersion int `json:"min_java_version"`
	// RecommendedJavaVersions 是判定为通过的 JDK 主版本；高于 MaxRecommendedJavaVersion 仅告警。
	// RecommendedJavaVersions are the JDK majors treated as passed; anything above MaxRecommendedJavaVersion only warns.
	RecommendedJavaVersions   []int `json:"recommended_java_versions"`
	MaxRecommendedJavaVersion int   `json:"max_recommended_java_version"`
}

// CapabilitiesForVersion returns the SeaTunnel runtime capability matrix used by
// the installer and cluster deployment wizard.
// CapabilitiesForVersion 返回安装器和集群部署向导使用的 SeaTunnel 运行时能力矩阵。
func CapabilitiesForVersion(version string) VersionCapabilities {
	javaReq := JavaRequirementForVersion(version)
	capabilities := VersionCapabilities{
		MinJavaVersion:                 javaReq.MinSupported,
		RecommendedJavaVersions:        append([]int(nil), javaReq.Recommended...),
		MaxRecommendedJavaVersion:      javaReq.MaxRecommended,
		DefaultDynamicSlot:             DefaultInstallerDynamicSlot,
		DefaultStaticSlotNum:           DefaultInstallerStaticSlotNum,
		DefaultHistoryJobExpireMinutes: DefaultInstallerHistoryJobExpireMinutes,
		DefaultScheduledDeletionEnable: DefaultInstallerScheduledDeletionEnable,
		DefaultJobScheduleStrategy:     DefaultInstallerJobScheduleStrategy,
		DefaultSlotAllocationStrategy:  DefaultInstallerSlotAllocationStrategy,
		DefaultHTTPEnabled:             true,
		DefaultJobLogMode:              "mixed",
	}

	if CompareVersions(version, "2.3.0") >= 0 {
		capabilities.SupportsDynamicSlot = true
		capabilities.SupportsSlotNum = true
	}
	if CompareVersions(version, "2.3.3") >= 0 {
		capabilities.SupportsHistoryJobExpireMinutes = true
	}
	if CompareVersions(version, "2.3.9") >= 0 {
		capabilities.SupportsScheduledDeletionEnable = true
		capabilities.SupportsJobScheduleStrategy = true
		capabilities.SupportsHTTPService = true
	}
	if CompareVersions(version, "2.3.10") >= 0 {
		capabilities.SupportsSlotAllocationStrategy = true
	}
	if CompareVersions(version, "2.3.8") >= 0 {
		capabilities.SupportsJobLogMode = true
		capabilities.DefaultJobLogMode = "per_job"
	}
	// 3.0.0 起才默认打开事件通道：更早版本要么没有非终态上报，要么还要用户到集群详情里手动开。
	// Default the event channel on from 3.0.0: earlier versions either lack non-terminal reports or stay a manual cluster-detail toggle.
	if SupportsNonTerminalJobStateEvents(version) {
		capabilities.SupportsEventChannel = true
		capabilities.DefaultEventChannelEnabled = true
	}

	return capabilities
}

// SupportsRealtimeObservability 判断版本是否提供作业级 env.engine.observability 与 /metrics/realtime 接口（3.0.0 起）。
// SupportsRealtimeObservability reports whether the version provides job-level env.engine.observability and the /metrics/realtime API (since 3.0.0).
func SupportsRealtimeObservability(version string) bool {
	return strings.TrimSpace(version) != "" && CompareVersions(version, "3.0.0") >= 0
}

// SupportsJobStateEvents 判断版本的 event-report-http 是否会上报 JOB_STATUS 事件（2.3.12 起，2.3.12 只报终态）。
// SupportsJobStateEvents reports whether event-report-http emits JOB_STATUS events (since 2.3.12; 2.3.12 reports terminal states only).
func SupportsJobStateEvents(version string) bool {
	return strings.TrimSpace(version) != "" && CompareVersions(version, "2.3.12") >= 0
}

// SupportsCDCDelayEvents 判断 STX 能否从该版本收到 READER_MESSAGE_DELAYED（B4 CDC 延迟告警的数据来源），门槛 2.3.12。
// 依据（apache/seatunnel 源码）：引擎侧自 2.3.5 起就由 CDC 的 IncrementalSourceRecordEmitter 发 MessageDelayedEvent
// （#6634，同版本 #6419 引入 event-report-http 的 JobEventHttpReportHandler），2.3.12 到 3.0.0-release 之间
// MessageDelayedEvent、IncrementalSourceRecordEmitter、MessageDelayedEventLimiter 都没有改动；STX 只在 ≥2.3.12 的集群上允许
// 开启事件通道（SupportsJobStateEvents），所以 B4 实际生效的门槛就是事件通道的门槛 2.3.12。
// SupportsCDCDelayEvents reports whether STX can receive READER_MESSAGE_DELAYED (the data source of the B4 CDC delay
// alert) from the version; the gate is 2.3.12. Evidence (apache/seatunnel sources): the engine has emitted
// MessageDelayedEvent from the CDC IncrementalSourceRecordEmitter since 2.3.5 (#6634; #6419 in the same release added
// event-report-http's JobEventHttpReportHandler), and MessageDelayedEvent, IncrementalSourceRecordEmitter and
// MessageDelayedEventLimiter are unchanged between 2.3.12 and 3.0.0-release; STX only lets the event channel be
// enabled on ≥2.3.12 clusters (SupportsJobStateEvents), so the effective B4 gate is the event channel gate, 2.3.12.
func SupportsCDCDelayEvents(version string) bool {
	return SupportsJobStateEvents(version)
}

// SupportsNonTerminalJobStateEvents 判断版本是否支持 report-non-terminal-job-state 上报非终态（3.0.0 起）。
// SupportsNonTerminalJobStateEvents reports whether report-non-terminal-job-state is supported for non-terminal states (since 3.0.0).
func SupportsNonTerminalJobStateEvents(version string) bool {
	return strings.TrimSpace(version) != "" && CompareVersions(version, "3.0.0") >= 0
}

// CompareVersions compares two SeaTunnel version strings.
// CompareVersions 比较两个 SeaTunnel 版本字符串。
func CompareVersions(v1, v2 string) int {
	parts1 := strings.Split(strings.TrimSpace(v1), ".")
	parts2 := strings.Split(strings.TrimSpace(v2), ".")

	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}

	for i := 0; i < maxLen; i++ {
		var p1, p2 string
		if i < len(parts1) {
			p1 = parts1[i]
		}
		if i < len(parts2) {
			p2 = parts2[i]
		}

		n1, s1 := parseVersionPart(p1)
		n2, s2 := parseVersionPart(p2)
		if n1 != n2 {
			return n1 - n2
		}
		if s1 != s2 {
			if s1 == "" {
				return 1
			}
			if s2 == "" {
				return -1
			}
			return strings.Compare(s1, s2)
		}
	}

	return 0
}

func parseVersionPart(part string) (int, string) {
	if part == "" {
		return 0, ""
	}

	idx := strings.Index(part, "-")
	if idx == -1 {
		var num int
		fmt.Sscanf(part, "%d", &num)
		return num, ""
	}

	var num int
	fmt.Sscanf(part[:idx], "%d", &num)
	return num, part[idx:]
}

// InfraCapabilities 描述基础设施形态维度的工作台/运维能力位；与版本维度正交。
// InfraCapabilities describes workbench/operation capability bits along the infrastructure
// dimension, orthogonal to the version dimension.
type InfraCapabilities struct {
	// SupportsLocalExecution local 模式依赖宿主机进程语义，K8s 集群不可用。
	// SupportsLocalExecution is unavailable on K8s since local mode relies on host process semantics.
	SupportsLocalExecution bool `json:"supports_local_execution"`
	// SupportsDynamicPluginInstall K8s 遵循不可变镜像原则，禁止动态安装/卸载插件。
	// SupportsDynamicPluginInstall is forbidden on K8s under the immutable-image principle.
	SupportsDynamicPluginInstall bool `json:"supports_dynamic_plugin_install"`
	// SupportsImmutableImageUpgrade K8s 集群通过镜像 Tag 滚动升级。
	// SupportsImmutableImageUpgrade means the cluster upgrades by rolling the image tag.
	SupportsImmutableImageUpgrade bool `json:"supports_immutable_image_upgrade"`
	// SupportsPodLogRead K8s 集群经 pods/log 读取引擎日志。
	// SupportsPodLogRead means engine logs are read through pods/log.
	SupportsPodLogRead bool `json:"supports_pod_log_read"`
	// SupportsWebUIProxy 引擎 HTTP 服务可经控制面代理访问。
	// SupportsWebUIProxy means the engine HTTP service can be proxied through the control plane.
	SupportsWebUIProxy bool `json:"supports_web_ui_proxy"`
}

// InfraCapabilitiesFor 返回给定 infra_type 的能力位；未知形态按 bare_metal 全能力处理。
// InfraCapabilitiesFor returns capability bits for the given infra_type; unknown shapes fall
// back to bare_metal full capabilities.
func InfraCapabilitiesFor(infraType string) InfraCapabilities {
	switch {
	case infraType == "kubernetes" || len(infraType) > len("kubernetes_") && infraType[:len("kubernetes_")] == "kubernetes_":
		return InfraCapabilities{
			SupportsLocalExecution:          false,
			SupportsDynamicPluginInstall:    false,
			SupportsImmutableImageUpgrade:   true,
			SupportsPodLogRead:              true,
			SupportsWebUIProxy:              true,
		}
	default:
		return InfraCapabilities{
			SupportsLocalExecution:          true,
			SupportsDynamicPluginInstall:    true,
			SupportsImmutableImageUpgrade:   false,
			SupportsPodLogRead:              false,
			SupportsWebUIProxy:              true,
		}
	}
}
