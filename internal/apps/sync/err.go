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
	ErrTaskNotFound                   = errors.New("sync: task not found")
	ErrTaskVersionNotFound            = errors.New("sync: task version not found")
	ErrJobInstanceNotFound            = errors.New("sync: job instance not found")
	ErrTaskNameRequired               = errors.New("sync: task name is required")
	ErrInvalidTaskMode                = errors.New("sync: invalid task mode")
	ErrInvalidTaskStatus              = errors.New("sync: invalid task status")
	ErrInvalidRunType                 = errors.New("sync: invalid run type")
	ErrTaskArchived                   = errors.New("sync: archived task cannot be modified")
	ErrTaskNotPublished               = errors.New("sync: task has not been published")
	ErrTaskDefinitionEmpty            = errors.New("sync: task definition content is required")
	ErrInvalidPreviewMode             = errors.New("sync: invalid preview mode")
	ErrPreviewHTTPSinkEmpty           = errors.New("sync: preview http sink configuration is required")
	ErrJobAlreadyFinished             = errors.New("sync: job instance already finished")
	ErrJobPermissionDenied            = errors.New("sync: job instance permission denied")
	ErrJobStatusChanged               = errors.New("sync: job instance status changed concurrently")
	ErrInvalidNodeType                = errors.New("sync: invalid node type")
	ErrParentTaskNotFolder            = errors.New("sync: parent node must be a folder")
	ErrFolderContentUnsupported       = errors.New("sync: folder node does not support job content")
	ErrTaskNotFile                    = errors.New("sync: operation only supports file nodes")
	ErrInvalidContentFormat           = errors.New("sync: invalid content format")
	ErrRecoverSourceRequired          = errors.New("sync: recover source job instance is required")
	ErrTaskNameInvalid                = errors.New("sync: task name contains unsupported characters")
	ErrTaskParentCycle                = errors.New("sync: parent node cannot be current node or its descendant")
	ErrRootFileNotAllowed             = errors.New("sync: files must be created inside a folder")
	ErrTaskNameDuplicate              = errors.New("sync: task name already exists in the current folder")
	ErrLocalExecutionUnavailable      = errors.New("sync: local execution is unavailable")
	ErrLocalClusterRequired           = errors.New("sync: local execution requires a cluster context")
	ErrLocalSavepointUnsupported      = errors.New("sync: local execution does not support savepoint")
	ErrExecutionTargetUnavailable     = errors.New("sync: execution target is unavailable")
	ErrExecutionTargetClusterMismatch = errors.New("sync: execution target does not belong to task cluster")
	ErrJobLogsUnavailable             = errors.New("sync: job logs are unavailable")
	ErrPreviewPayloadInvalid          = errors.New("sync: preview collect payload is invalid")
	ErrGlobalVariableNotFound         = errors.New("sync: global variable not found")
	ErrGlobalVariableKeyRequired      = errors.New("sync: global variable key is required")
	ErrGlobalVariableKeyInvalid       = errors.New("sync: global variable key contains unsupported characters")
	ErrGlobalVariableKeyDuplicate     = errors.New("sync: global variable key already exists")
	ErrReservedBuiltinVariableKey     = errors.New("sync: variable key is reserved for built-in time variables")
	ErrPreviewSessionNotFound         = errors.New("sync: preview session not found")
	ErrInvalidTaskSchedule            = errors.New("sync: invalid task schedule")
	ErrMaskedSecretCannotBeRestored   = errors.New("sync: masked secret cannot be restored from the saved task")
	ErrTaskReadOnly                   = errors.New("sync: task is read-only for current user")
	ErrTaskPermissionDenied           = errors.New("sync: task permission denied")
	ErrInvalidTaskCollaborator        = errors.New("sync: collaborator user id must be greater than zero")
	ErrGlobalVariablePermissionDenied = errors.New("sync: global variable permission denied")
	ErrCuratedTemplateNotFound        = errors.New("sync: curated template not found")
	ErrCuratedTemplateNameRequired    = errors.New("sync: curated template name is required")
	ErrCuratedTemplateContentRequired = errors.New("sync: curated template content is required")
	ErrCuratedTemplateSectionInvalid  = errors.New("sync: curated template section is invalid")
	ErrCuratedTemplateBuiltinReadonly = errors.New("sync: built-in curated template is read-only")
	ErrCuratedTemplatePermissionDenied = errors.New("sync: curated template permission denied")
	ErrCuratedBuiltinIDRequired       = errors.New("sync: curated builtin_id is required")
)
