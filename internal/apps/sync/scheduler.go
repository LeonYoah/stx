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

package sync

import (
	"strings"
	"time"
)

const (
	SchedulerKindDolphin      = "dolphin"
	SchedulerAPIStyleAuto     = "auto"
	SchedulerAPIStyleProcess  = "process"
	SchedulerAPIStyleWorkflow = "workflow"
)

// SchedulerConnection 保存访问外部调度器的 API 地址和令牌。
// SchedulerConnection stores the API endpoint and token for an external scheduler.
type SchedulerConnection struct {
	ID                 uint   `json:"id" gorm:"primaryKey;autoIncrement"`
	Name               string `json:"name" gorm:"size:120;not null"`
	Kind               string `json:"kind" gorm:"size:32;not null;index;default:dolphin"`
	BaseURL            string `json:"base_url" gorm:"size:512;not null"`
	Token              string `json:"-" gorm:"type:text"`
	TokenSet           bool   `json:"token_set" gorm:"-"`
	DefaultProjectCode int64  `json:"default_project_code"`
	APIStyle           string `json:"api_style" gorm:"size:32;not null;default:auto"`
	// DolphinVersion 可选，例如 3.2.2 / 3.3.1；空则运行时探测 3.1/3.2 与 3.3+ 路径。
	// DolphinVersion is optional, e.g. 3.2.2 / 3.3.1; empty means probe 3.1/3.2 vs 3.3+ paths at runtime.
	DolphinVersion string    `json:"dolphin_version,omitempty" gorm:"size:32"`
	CreatedBy      uint      `json:"created_by"`
	CreatedAt      time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt      time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

// TableName 指定调度器连接表名。
// TableName specifies the scheduler connection table.
func (SchedulerConnection) TableName() string {
	return "sync_scheduler_connections"
}

// MaskToken 把令牌从 API 响应当中去掉，只保留是否已配置。
// MaskToken removes the raw token from API responses and only reports whether it is set.
func (c *SchedulerConnection) MaskToken() {
	if c == nil {
		return
	}
	c.TokenSet = strings.TrimSpace(c.Token) != ""
	c.Token = ""
}

// TaskSchedulerBinding 记录任务与 Dolphin 项目/工作流/节点的绑定，不保存上下游。
// TaskSchedulerBinding records the task-to-Dolphin project/workflow/node mapping without upstream/downstream.
type TaskSchedulerBinding struct {
	ID           uint       `json:"id" gorm:"primaryKey;autoIncrement"`
	TaskID       uint       `json:"task_id" gorm:"uniqueIndex;not null"`
	Kind         string     `json:"kind" gorm:"size:32;not null;index;default:dolphin"`
	ConnectionID uint       `json:"connection_id" gorm:"index;not null"`
	ProjectCode  int64      `json:"project_code" gorm:"not null"`
	WorkflowCode int64      `json:"workflow_code" gorm:"not null"`
	TaskCode     int64      `json:"task_code" gorm:"not null"`
	WorkflowName string     `json:"workflow_name" gorm:"size:255"`
	ConsoleURL   string     `json:"console_url" gorm:"size:1024"`
	PushedAt     *time.Time `json:"pushed_at"`
	PushedBy     uint       `json:"pushed_by"`
	// TaskVersion 记录这次推送对应的 STX 已发布版本；同一版本不能再更新推送。
	// TaskVersion is the STX published version this push deployed; the same version cannot be pushed again.
	TaskVersion int       `json:"task_version"`
	CreatedAt   time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt   time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

// TableName 指定任务调度绑定表名。
// TableName specifies the task scheduler binding table.
func (TaskSchedulerBinding) TableName() string {
	return "sync_task_scheduler_bindings"
}

// SchedulerConnectionRequest 创建或更新调度器连接。
// SchedulerConnectionRequest creates or updates a scheduler connection.
type SchedulerConnectionRequest struct {
	Name               string `json:"name"`
	Kind               string `json:"kind"`
	BaseURL            string `json:"base_url"`
	Token              string `json:"token"`
	DefaultProjectCode int64  `json:"default_project_code"`
	APIStyle           string `json:"api_style"`
	DolphinVersion     string `json:"dolphin_version"`
}

// DolphinPushRequest 描述一次推送到 Dolphin 的请求。
// DolphinPushRequest describes one push into DolphinScheduler.
type DolphinPushRequest struct {
	Mode               string  `json:"mode"`
	ConnectionID       uint    `json:"connection_id"`
	ProjectCode        int64   `json:"project_code"`
	WorkflowName       string  `json:"workflow_name"`
	WorkflowCode       int64   `json:"workflow_code"`
	Upstreams          []int64 `json:"upstreams"`
	Downstreams        []int64 `json:"downstreams"`
	UseBuiltinSchedule bool    `json:"use_builtin_schedule"`
	// Timeout 已忽略：生成脚本不再写入 --timeout，保留字段只为兼容旧请求体。
	// Timeout is ignored: generated scripts omit --timeout; kept only for old request bodies.
	Timeout         string            `json:"timeout"`
	Variables       map[string]string `json:"variables"`
	WorkerGroup     string            `json:"worker_group"`
	Environment     string            `json:"environment"`
	DryRun          bool              `json:"dry_run"`
	ExpectedVersion int               `json:"expected_version"`
}

// DolphinScriptResult 是给 Dolphin SHELL 节点使用的脚本和环境说明。
// DolphinScriptResult is the SHELL script and environment notes for a Dolphin node.
type DolphinScriptResult struct {
	Script      string            `json:"script"`
	EnvNotes    string            `json:"env_notes"`
	Timeout     string            `json:"timeout,omitempty"`
	WorkerGroup string            `json:"worker_group"`
	Environment string            `json:"environment"`
	Variables   map[string]string `json:"variables,omitempty"`
}

// DolphinPushResult 返回推送预览或实际写入结果。
// DolphinPushResult returns a dry-run preview or the written binding.
type DolphinPushResult struct {
	DryRun       bool                  `json:"dry_run"`
	Mode         string                `json:"mode"`
	Script       *DolphinScriptResult  `json:"script,omitempty"`
	Binding      *TaskSchedulerBinding `json:"binding,omitempty"`
	WorkflowCode int64                 `json:"workflow_code,omitempty"`
	TaskCode     int64                 `json:"task_code,omitempty"`
	Version      int                   `json:"version,omitempty"`
	ReleaseState string                `json:"release_state,omitempty"`
	WouldOffline bool                  `json:"would_offline,omitempty"`
	Cycle        bool                  `json:"cycle"`
	Message      string                `json:"message,omitempty"`
	Warning      string                `json:"warning,omitempty"`
}

// DolphinTaskStatus 是任务侧边栏需要的绑定、脚本和最近作业摘要。
// DolphinTaskStatus is the sidebar summary of binding, script, and recent Dolphin-triggered jobs.
type DolphinTaskStatus struct {
	Binding       *TaskSchedulerBinding `json:"binding"`
	Connection    *SchedulerConnection  `json:"connection,omitempty"`
	Script        *DolphinScriptResult  `json:"script,omitempty"`
	RecentJobs    []*JobInstance        `json:"recent_jobs"`
	BuiltinLocked bool                  `json:"builtin_locked"`
	// PublishedVersion 是任务当前已发布版本；PushedVersion 是上次推到 Dolphin 的版本。
	// PublishedVersion is the task's current published version; PushedVersion is the last version pushed to Dolphin.
	PublishedVersion int `json:"published_version"`
	PushedVersion    int `json:"pushed_version"`
}

// DolphinProject 是连接上可选的 Dolphin 项目。
// DolphinProject is a Dolphin project available on a connection.
type DolphinProject struct {
	Code int64  `json:"code"`
	Name string `json:"name"`
}

// DolphinWorkflowSummary 是复用工作流时的候选列表项。
// DolphinWorkflowSummary is one reusable workflow candidate.
type DolphinWorkflowSummary struct {
	Code         int64  `json:"code"`
	Name         string `json:"name"`
	ReleaseState string `json:"release_state"`
	Version      int    `json:"version"`
}

// DolphinWorkflowTask 是上下游勾选项，可选带回当前 SHELL 脚本供 Diff。
// DolphinWorkflowTask is one upstream/downstream checkbox candidate and may include the current SHELL script for diffs.
type DolphinWorkflowTask struct {
	Code      int64  `json:"code"`
	Name      string `json:"name"`
	TaskType  string `json:"task_type"`
	RawScript string `json:"raw_script,omitempty"`
}

// DolphinWorkflowRelation 是工作流里的一条有向边，供前端成环预检。
// DolphinWorkflowRelation is one directed edge used by the frontend cycle preview.
type DolphinWorkflowRelation struct {
	PreTaskCode  int64 `json:"pre_task_code"`
	PostTaskCode int64 `json:"post_task_code"`
}

// DolphinWorkflowDetail 供复用工作流时展示节点列表、连线和当前版本。
// DolphinWorkflowDetail is the workflow snapshot used when reusing a graph.
type DolphinWorkflowDetail struct {
	Code         int64                     `json:"code"`
	Name         string                    `json:"name"`
	Version      int                       `json:"version"`
	ReleaseState string                    `json:"release_state"`
	Tasks        []DolphinWorkflowTask     `json:"tasks"`
	Relations    []DolphinWorkflowRelation `json:"relations"`
}

// SchedulerConnectionListData 是调度器连接列表。
// SchedulerConnectionListData is the scheduler connection list payload.
type SchedulerConnectionListData struct {
	Items []*SchedulerConnection `json:"items"`
}
