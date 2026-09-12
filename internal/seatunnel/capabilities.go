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

// VersionCapabilities describes which advanced runtime settings are supported by
// one SeaTunnel release from the install wizard perspective.
// VersionCapabilities 描述安装向导视角下某个 SeaTunnel 版本支持的高级运行时配置能力。
type VersionCapabilities struct {
	SupportsDynamicSlot             bool   `json:"supports_dynamic_slot"`
	SupportsSlotNum                 bool   `json:"supports_slot_num"`
	SupportsHistoryJobExpireMinutes bool   `json:"supports_history_job_expire_minutes"`
	SupportsScheduledDeletionEnable bool   `json:"supports_scheduled_deletion_enable"`
	SupportsJobScheduleStrategy     bool   `json:"supports_job_schedule_strategy"`
	SupportsSlotAllocationStrategy  bool   `json:"supports_slot_allocation_strategy"`
	SupportsHTTPService             bool   `json:"supports_http_service"`
	SupportsJobLogMode              bool   `json:"supports_job_log_mode"`
	DefaultDynamicSlot              bool   `json:"default_dynamic_slot"`
	DefaultStaticSlotNum            int    `json:"default_static_slot_num"`
	DefaultHistoryJobExpireMinutes  int    `json:"default_history_job_expire_minutes"`
	DefaultScheduledDeletionEnable  bool   `json:"default_scheduled_deletion_enable"`
	DefaultJobScheduleStrategy      string `json:"default_job_schedule_strategy"`
	DefaultSlotAllocationStrategy   string `json:"default_slot_allocation_strategy"`
	DefaultHTTPEnabled              bool   `json:"default_http_enabled"`
	DefaultJobLogMode               string `json:"default_job_log_mode"`
}

// CapabilitiesForVersion returns the SeaTunnel runtime capability matrix used by
// the installer and cluster deployment wizard.
// CapabilitiesForVersion 返回安装器和集群部署向导使用的 SeaTunnel 运行时能力矩阵。
func CapabilitiesForVersion(version string) VersionCapabilities {
	capabilities := VersionCapabilities{
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
	}

	return capabilities
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
