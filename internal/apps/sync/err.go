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

import "errors"

var (
	ErrTaskNotFound                    = errors.New("sync: task not found")
	ErrTaskVersionNotFound             = errors.New("sync: task version not found")
	ErrJobInstanceNotFound             = errors.New("sync: job instance not found")
	ErrTaskNameRequired                = errors.New("sync: task name is required")
	ErrInvalidTaskMode                 = errors.New("sync: invalid task mode")
	ErrInvalidTaskStatus               = errors.New("sync: invalid task status")
	ErrInvalidRunType                  = errors.New("sync: invalid run type")
	ErrTaskArchived                    = errors.New("sync: archived task cannot be modified")
	ErrTaskNotPublished                = errors.New("sync: task has not been published")
	ErrTaskDefinitionEmpty             = errors.New("sync: task definition content is required")
	ErrInvalidPreviewMode              = errors.New("sync: invalid preview mode")
	ErrPreviewHTTPSinkEmpty            = errors.New("sync: preview http sink configuration is required")
	ErrJobAlreadyFinished              = errors.New("sync: job instance already finished")
	ErrJobPermissionDenied             = errors.New("sync: job instance permission denied")
	ErrJobStatusChanged                = errors.New("sync: job instance status changed concurrently")
	ErrInvalidNodeType                 = errors.New("sync: invalid node type")
	ErrParentTaskNotFolder             = errors.New("sync: parent node must be a folder")
	ErrFolderContentUnsupported        = errors.New("sync: folder node does not support job content")
	ErrTaskNotFile                     = errors.New("sync: operation only supports file nodes")
	ErrInvalidContentFormat            = errors.New("sync: invalid content format")
	ErrRecoverSourceRequired           = errors.New("sync: recover source job instance is required")
	ErrTaskNameInvalid                 = errors.New("sync: task name contains unsupported characters")
	ErrTaskParentCycle                 = errors.New("sync: parent node cannot be current node or its descendant")
	ErrRootFileNotAllowed              = errors.New("sync: files must be created inside a folder")
	ErrTaskNameDuplicate               = errors.New("sync: task name already exists in the current folder")
	ErrLocalExecutionUnavailable       = errors.New("sync: local execution is unavailable")
	ErrLocalClusterRequired            = errors.New("sync: local execution requires a cluster context")
	ErrLocalSavepointUnsupported       = errors.New("sync: local execution does not support savepoint")
	ErrExecutionTargetUnavailable      = errors.New("sync: execution target is unavailable")
	ErrExecutionTargetClusterMismatch  = errors.New("sync: execution target does not belong to task cluster")
	ErrJobLogsUnavailable              = errors.New("sync: job logs are unavailable")
	ErrPreviewPayloadInvalid           = errors.New("sync: preview collect payload is invalid")
	ErrGlobalVariableNotFound          = errors.New("sync: global variable not found")
	ErrGlobalVariableKeyRequired       = errors.New("sync: global variable key is required")
	ErrGlobalVariableKeyInvalid        = errors.New("sync: global variable key contains unsupported characters")
	ErrGlobalVariableKeyDuplicate      = errors.New("sync: global variable key already exists")
	ErrReservedBuiltinVariableKey      = errors.New("sync: variable key is reserved for built-in time variables")
	ErrPreviewSessionNotFound          = errors.New("sync: preview session not found")
	ErrInvalidTaskSchedule             = errors.New("sync: invalid task schedule")
	ErrMaskedSecretCannotBeRestored    = errors.New("sync: masked secret cannot be restored from the saved task")
	ErrTaskReadOnly                    = errors.New("sync: task is read-only for current user")
	ErrTaskPermissionDenied            = errors.New("sync: task permission denied")
	ErrInvalidTaskCollaborator         = errors.New("sync: collaborator user id must be greater than zero")
	ErrGlobalVariablePermissionDenied  = errors.New("sync: global variable permission denied")
	ErrCuratedTemplateNotFound         = errors.New("sync: curated template not found")
	ErrCuratedTemplateNameRequired     = errors.New("sync: curated template name is required")
	ErrCuratedTemplateContentRequired  = errors.New("sync: curated template content is required")
	ErrCuratedTemplateSectionInvalid   = errors.New("sync: curated template section is invalid")
	ErrCuratedTemplateBuiltinReadonly  = errors.New("sync: built-in curated template is read-only")
	ErrCuratedTemplatePermissionDenied = errors.New("sync: curated template permission denied")
	ErrCuratedBuiltinIDRequired        = errors.New("sync: curated builtin_id is required")
	ErrUnreplacedVariable              = errors.New("sync: variable value still contains an unreplaced ${...} placeholder")
	ErrInvalidScheduleTime             = errors.New("sync: schedule_time must be RFC3339 or yyyyMMddHHmmss")
	ErrAttachGenerationMismatch        = errors.New("sync: attach_generation does not match the current job")
	ErrJobCancellingTimeout            = errors.New("sync: timed out waiting for the previous job on this external_ref to finish cancelling")
	ErrScheduleLockedByDolphin         = errors.New("sync: built-in schedule cannot be enabled while the task is bound to DolphinScheduler")
	ErrSchedulerConnectionNotFound     = errors.New("sync: scheduler connection not found")
	ErrSchedulerConnectionInvalid      = errors.New("sync: scheduler connection is invalid")
	ErrSchedulerBindingNotFound        = errors.New("sync: scheduler binding not found")
	ErrSchedulerBindingExists          = errors.New("sync: task already has a scheduler binding")
	ErrDolphinCycle                    = errors.New("sync: adding the selected upstream/downstream edges would create a cycle")
	ErrDolphinVersionConflict          = errors.New("sync: DolphinScheduler workflow version changed")
	ErrDolphinNodeMissing              = errors.New("sync: bound DolphinScheduler task node is missing; use first-time push")
	ErrDolphinPushInvalid              = errors.New("sync: invalid DolphinScheduler push request")
	ErrDolphinPushNoNewVersion         = errors.New("sync: dolphin push requires a new published version")
	ErrDolphinUnsupportedVersion       = errors.New("sync: only DolphinScheduler 3.x is supported")
	// 事件通道错误 / Event channel errors.
	ErrEventChannelNotFound            = errors.New("sync: engine event channel not found")
	ErrEventChannelVersionUnsupported  = errors.New("sync: engine event reporting requires SeaTunnel 2.3.12 or later")
	ErrEventChannelExternalURLMissing  = errors.New("sync: app.external_url must be configured so SeaTunnel can reach STX")
	ErrEventChannelSecretMissing       = errors.New("sync: app.session_secret must be configured to sign engine event tokens")
	ErrEventChannelApplierMissing      = errors.New("sync: seatunnel.yaml config applier is not configured")
	ErrEventChannelUnauthorized        = errors.New("sync: invalid engine event token")
	ErrEventChannelPermissionDenied    = errors.New("sync: only administrators can change the engine event channel")
)

// 写操作结果未知与超时配置相关错误。
// Errors for unknown write outcomes and timeout settings.
var (
	// ErrSubmitOutcomePending 表示同一任务上一次提交结果还在确认中，确认前拒绝新的提交。
	// ErrSubmitOutcomePending means the task's previous submit is still being confirmed, so new submits are refused.
	ErrSubmitOutcomePending = errors.New("sync: previous submit outcome is still being confirmed")
	// ErrInvalidTaskTimeout 表示任务设置里的提交/停止超时不在允许范围内。
	// ErrInvalidTaskTimeout means a task's submit/stop timeout override is outside the allowed range.
	ErrInvalidTaskTimeout = errors.New("sync: task timeout override must be between 1 and 120 minutes")
)
