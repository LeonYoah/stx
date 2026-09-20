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

// Package diagnostics provides the diagnostics workspace skeleton for
// error center, inspections, and diagnostic bundle tasks.
// diagnostics 包提供诊断中心骨架，用于承载错误中心、巡检与诊断任务。
package diagnostics

import "time"

// Response is the standard API response for diagnostics endpoints.
// Response 是诊断接口的标准响应结构。
type Response struct {
	ErrorMsg string      `json:"error_msg"`
	Data     interface{} `json:"data"`
}

// WorkspaceTabKey identifies one diagnostics workspace tab.
// WorkspaceTabKey 标识诊断中心工作台中的一个标签页。
type WorkspaceTabKey string

const (
	// WorkspaceTabErrors is the error center tab.
	// WorkspaceTabErrors 表示错误中心标签页。
	WorkspaceTabErrors WorkspaceTabKey = "errors"
	// WorkspaceTabInspections is the inspection tab.
	// WorkspaceTabInspections 表示巡检标签页。
	WorkspaceTabInspections WorkspaceTabKey = "inspections"
	// WorkspaceTabMemories 表示排障经验库标签页。
	// WorkspaceTabMemories is the troubleshooting memory bank tab.
	WorkspaceTabMemories WorkspaceTabKey = "memories"
	// WorkspaceTabTasks is the diagnostic task / bundle tab.
	// WorkspaceTabTasks 表示诊断任务 / 诊断包标签页。
	WorkspaceTabTasks WorkspaceTabKey = "tasks"
)

// WorkspaceBootstrapRequest describes the contextual query passed into the
// diagnostics workspace.
// WorkspaceBootstrapRequest 描述传入诊断工作台的上下文查询参数。
type WorkspaceBootstrapRequest struct {
	ClusterID *uint
	Source    string
	AlertID   string
}

// WorkspaceBootstrapData is the initial payload for diagnostics workspace UI.
// WorkspaceBootstrapData 是诊断中心 UI 的初始化数据。
type WorkspaceBootstrapData struct {
	GeneratedAt    time.Time            `json:"generated_at"`
	DefaultTab     WorkspaceTabKey      `json:"default_tab"`
	Tabs           []*WorkspaceTab      `json:"tabs"`
	ClusterOptions []*ClusterOption     `json:"cluster_options"`
	EntryContext   *WorkspaceContext    `json:"entry_context,omitempty"`
	Boundaries     []*WorkspaceBoundary `json:"boundaries"`
}

// WorkspaceTab describes one diagnostics tab.
// WorkspaceTab 描述一个诊断中心标签页。
type WorkspaceTab struct {
	Key         WorkspaceTabKey `json:"key"`
	Label       string          `json:"label"`
	Description string          `json:"description"`
}

// ClusterOption is a selectable managed cluster for diagnostics workspace.
// ClusterOption 表示诊断中心筛选中可选的受管集群。
type ClusterOption struct {
	ClusterID   uint   `json:"cluster_id"`
	ClusterName string `json:"cluster_name"`
}

// WorkspaceContext describes where the user navigated from.
// WorkspaceContext 描述用户进入诊断中心时的来源上下文。
type WorkspaceContext struct {
	ClusterID   *uint  `json:"cluster_id,omitempty"`
	ClusterName string `json:"cluster_name,omitempty"`
	Source      string `json:"source,omitempty"`
	AlertID     string `json:"alert_id,omitempty"`
}

// WorkspaceBoundary documents the current diagnostics module boundaries.
// WorkspaceBoundary 说明当前诊断中心的模块边界。
type WorkspaceBoundary struct {
	Key         string `json:"key"`
	Title       string `json:"title"`
	Description string `json:"description"`
}
