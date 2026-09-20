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
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
	"github.com/LeonYoah/stx/internal/config"
	"github.com/LeonYoah/stx/internal/pkg/schedulex"
	"github.com/LeonYoah/stx/internal/seatunnel"
)

var safeTaskNamePattern = regexp.MustCompile(`^[\p{L}\p{N}._-]+$`)

// Service provides sync studio control-plane behavior.
type Service struct {
	repo                    *Repository
	engineClient            EngineClient
	runtimeResolver         ClusterRuntimeResolver
	configToolClient        ConfigToolClient
	configToolResolver      ConfigToolResolver
	jobIDGenerator          *JobIDGenerator
	agentSender             AgentCommandSender
	executionTargetResolver ExecutionTargetResolver
	clusterLogProvider      ClusterLogProvider
	clusterVersionProvider  ClusterVersionProvider
	executionService        *executionapp.Service
}

// ClusterVersionProvider provides SeaTunnel cluster version lookup.
type ClusterVersionProvider interface {
	GetClusterVersion(ctx context.Context, clusterID uint) (string, error)
}

const (
	defaultPreviewRowLimit       = 100
	maxPreviewRowLimit           = 10000
	defaultPreviewTimeoutMinutes = 10
	defaultPreviewTTLMinutes     = 24 * 60
)

// NewService creates a new sync service.
func NewService(repo *Repository) *Service {
	return &Service{repo: repo, jobIDGenerator: NewJobIDGenerator()}
}

// SetEngineClient sets the SeaTunnel engine client used by submit/get/cancel flows.
func (s *Service) SetEngineClient(client EngineClient) { s.engineClient = client }

// SetExecutionService 设置公共执行协议服务。
// SetExecutionService configures the shared execution-contract service.
func (s *Service) SetExecutionService(service *executionapp.Service) { s.executionService = service }

// SetRuntimeResolver sets the runtime endpoint resolver for cluster-backed submissions.
func (s *Service) SetRuntimeResolver(resolver ClusterRuntimeResolver) { s.runtimeResolver = resolver }

// SetConfigToolClient sets the java-proxy config tool client.
func (s *Service) SetConfigToolClient(client ConfigToolClient) { s.configToolClient = client }

// SetConfigToolResolver sets the java-proxy endpoint resolver.
func (s *Service) SetConfigToolResolver(resolver ConfigToolResolver) { s.configToolResolver = resolver }

// SetJobIDGenerator sets the platform job id generator.
func (s *Service) SetJobIDGenerator(generator *JobIDGenerator) { s.jobIDGenerator = generator }

// StartPreviewRuntime starts background maintenance for preview sessions.
// StartPreviewRuntime 启动预览会话后台维护任务。
func (s *Service) StartPreviewRuntime(ctx context.Context) {
	if s == nil || s.repo == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = s.cleanupExpiredPreviewSessions(ctx, s.previewDataTTL())
				_ = s.stopTimedOutPreviewSessions(ctx)
			}
		}
	}()
}

func (s *Service) previewDataTTL() time.Duration {
	minutes := defaultPreviewTTLMinutes
	if config.Config != nil {
		if config.Config.Sync.PreviewDataTTLMinutes > 0 {
			minutes = config.Config.Sync.PreviewDataTTLMinutes
		} else if config.Config.Sync.PreviewDataTTLHours > 0 {
			minutes = config.Config.Sync.PreviewDataTTLHours * 60
		}
	}
	return time.Duration(minutes) * time.Minute
}

// ListGlobalVariables returns all workspace-wide variables.
func (s *Service) ListGlobalVariables(ctx context.Context) ([]*GlobalVariable, error) {
	return s.repo.ListGlobalVariables(ctx)
}

// ListGlobalVariablesPaginated returns paginated workspace-wide variables with sensitive values masked.
// ListGlobalVariablesPaginated 返回分页全局变量，敏感凭据值将被脱敏为 ******。
func (s *Service) ListGlobalVariablesPaginated(ctx context.Context, page, size int) ([]*GlobalVariable, int64, error) {
	items, total, err := s.repo.ListGlobalVariablesPaginated(ctx, page, size)
	if err != nil {
		return nil, 0, err
	}
	masked := make([]*GlobalVariable, len(items))
	for i, item := range items {
		masked[i] = item.MaskSecret()
	}
	return masked, total, nil
}

// CreateGlobalVariable creates one workspace-wide variable and returns a masked safe copy.
// CreateGlobalVariable 创建一个工作台全局变量并返回脱敏后的安全副本。
func (s *Service) CreateGlobalVariable(ctx context.Context, req *CreateGlobalVariableRequest, createdBy uint) (*GlobalVariable, error) {
	key, err := normalizeGlobalVariableKey(req.Key)
	if err != nil {
		return nil, err
	}
	if isReservedBuiltinVariableKey(key) {
		return nil, fmt.Errorf("%w: {{%s}}", ErrReservedBuiltinVariableKey, key)
	}
	if _, err := s.repo.GetGlobalVariableByKey(ctx, key); err == nil {
		return nil, ErrGlobalVariableKeyDuplicate
	} else if err != nil && !errors.Is(err, ErrGlobalVariableNotFound) {
		return nil, err
	}
	valueType := normalizeGlobalVariableType(req.ValueType)
	item := &GlobalVariable{
		Key:         key,
		Value:       req.Value,
		ValueType:   valueType,
		Description: strings.TrimSpace(req.Description),
		CreatedBy:   createdBy,
	}
	if err := s.repo.CreateGlobalVariable(ctx, item); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") || strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, ErrGlobalVariableKeyDuplicate
		}
		return nil, err
	}
	return item.MaskSecret(), nil
}

// UpdateGlobalVariable updates one workspace-wide variable and returns a masked safe copy.
// UpdateGlobalVariable 更新工作台全局变量并返回脱敏后的安全副本。
func (s *Service) UpdateGlobalVariable(ctx context.Context, id uint, req *UpdateGlobalVariableRequest) (*GlobalVariable, error) {
	item, err := s.repo.GetGlobalVariableByID(ctx, id)
	if err != nil {
		return nil, err
	}
	key, err := normalizeGlobalVariableKey(req.Key)
	if err != nil {
		return nil, err
	}
	if isReservedBuiltinVariableKey(key) {
		return nil, fmt.Errorf("%w: {{%s}}", ErrReservedBuiltinVariableKey, key)
	}
	if other, err := s.repo.GetGlobalVariableByKey(ctx, key); err == nil && other.ID != id {
		return nil, ErrGlobalVariableKeyDuplicate
	} else if err != nil && !errors.Is(err, ErrGlobalVariableNotFound) {
		return nil, err
	}

	if req.ValueType != "" {
		item.ValueType = normalizeGlobalVariableType(req.ValueType)
	}
	if item.ValueType == GlobalVariableTypeSecret {
		trimmed := strings.TrimSpace(req.Value)
		// 敏感变量仅在输入了非空且非脱敏掩码值时才更新；留空或原掩码保持现有密文不变
		// Only update password when a non-empty, non-mask string is provided; empty or mask retains existing secret
		if trimmed != "" && trimmed != "******" {
			item.Value = req.Value
		}
	} else {
		item.Value = req.Value
	}

	item.Key = key
	item.Description = strings.TrimSpace(req.Description)
	if err := s.repo.UpdateGlobalVariable(ctx, item); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") || strings.Contains(strings.ToLower(err.Error()), "unique") {
			return nil, ErrGlobalVariableKeyDuplicate
		}
		return nil, err
	}
	return item.MaskSecret(), nil
}

// DeleteGlobalVariable deletes one workspace-wide variable.
func (s *Service) DeleteGlobalVariable(ctx context.Context, id uint) error {
	return s.repo.DeleteGlobalVariable(ctx, id)
}

// CreateTask creates one workspace node.
func (s *Service) CreateTask(ctx context.Context, req *CreateTaskRequest, createdBy uint) (*Task, error) {
	nodeType, err := normalizeNodeType(req.NodeType)
	if err != nil {
		return nil, err
	}
	mode, err := normalizeTaskMode(req.Mode)
	if err != nil {
		return nil, err
	}
	format, err := normalizeContentFormat(req.ContentFormat)
	if err != nil {
		return nil, err
	}
	parentID, err := s.validateParent(ctx, req.ParentID)
	if err != nil {
		return nil, err
	}
	if nodeType == TaskNodeTypeFile && (parentID == nil || *parentID == 0) {
		return nil, ErrRootFileNotAllowed
	}
	name, err := normalizeTaskName(req.Name)
	if err != nil {
		return nil, err
	}
	if err := s.ensureSiblingTaskNameAvailable(ctx, parentID, name, nil); err != nil {
		return nil, err
	}
	definition, err := restoreMaskedJSONMap(nil, cloneJSONMap(req.Definition))
	if err != nil {
		return nil, err
	}
	if err := validateTaskDefinition(definition); err != nil {
		return nil, err
	}
	content := strings.TrimSpace(req.Content)
	content, err = restoreMaskedTaskContent(format, "", content)
	if err != nil {
		return nil, err
	}
	jobName := strings.TrimSpace(req.JobName)
	if nodeType == TaskNodeTypeFolder {
		content = ""
		jobName = ""
	} else if content == "" {
		return nil, ErrTaskDefinitionEmpty
	}
	task := &Task{
		ParentID:       parentID,
		NodeType:       nodeType,
		Name:           name,
		Description:    strings.TrimSpace(req.Description),
		ClusterID:      req.ClusterID,
		EngineVersion:  strings.TrimSpace(req.EngineVersion),
		Mode:           mode,
		Status:         TaskStatusDraft,
		ContentFormat:  format,
		Content:        content,
		JobName:        jobName,
		Definition:     definition,
		SortOrder:      req.SortOrder,
		CurrentVersion: 0,
		CreatedBy:      createdBy,
	}
	if err := s.repo.CreateTask(ctx, task); err != nil {
		return nil, err
	}
	return task, nil
}

// ListTasks returns paginated workspace nodes.
func (s *Service) ListTasks(ctx context.Context, filter *TaskFilter) ([]*Task, int64, error) {
	tasks, total, err := s.repo.ListTasks(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	for _, task := range tasks {
		s.applyTaskDefaults(task)
	}
	if err := s.decorateTaskScheduleMetadata(ctx, tasks); err != nil {
		return nil, 0, err
	}
	return tasks, total, nil
}

// GetTask returns one workspace node.
func (s *Service) GetTask(ctx context.Context, id uint) (*Task, error) {
	return s.GetTaskForActor(ctx, executionapp.Actor{IsAdmin: true}, id)
}

// GetTaskForActor returns one workspace node decorated with actor permissions.
func (s *Service) GetTaskForActor(ctx context.Context, actor executionapp.Actor, id uint) (*Task, error) {
	task, err := s.repo.GetTaskByID(ctx, id)
	if err != nil {
		return nil, err
	}
	s.applyTaskDefaults(task)
	task.DecoratePermissions(uint(actor.UserID), actor.IsAdmin)
	if !task.CanUserView(uint(actor.UserID), actor.IsAdmin) {
		return nil, ErrTaskPermissionDenied
	}
	if err := s.decorateTaskScheduleMetadata(ctx, []*Task{task}); err != nil {
		return nil, err
	}
	return task, nil
}

// GetTaskTree returns nested workspace nodes for the left tree.
func (s *Service) GetTaskTree(ctx context.Context) ([]*TaskTreeNode, error) {
	return s.GetTaskTreeForActor(ctx, executionapp.Actor{IsAdmin: true})
}

// GetTaskTreeForActor returns nested workspace nodes filtered and decorated with actor permissions.
func (s *Service) GetTaskTreeForActor(ctx context.Context, actor executionapp.Actor) ([]*TaskTreeNode, error) {
	if err := s.ensureRootFilesNested(ctx); err != nil {
		return nil, err
	}
	tasks, err := s.repo.ListAllTasks(ctx)
	if err != nil {
		return nil, err
	}
	for _, task := range tasks {
		s.applyTaskDefaults(task)
		task.DecoratePermissions(uint(actor.UserID), actor.IsAdmin)
	}
	if err := s.decorateTaskScheduleMetadata(ctx, tasks); err != nil {
		return nil, err
	}

	visibleTasks := tasks
	if !actor.IsAdmin {
		visibleMap := make(map[uint]bool)
		for _, task := range tasks {
			if task.NodeType == TaskNodeTypeFile {
				if task.CanUserView(uint(actor.UserID), actor.IsAdmin) {
					visibleMap[task.ID] = true
				}
			} else if task.CreatedBy != 0 && task.CreatedBy == uint(actor.UserID) {
				visibleMap[task.ID] = true
			}
		}
		parentMap := make(map[uint]uint)
		for _, task := range tasks {
			if task.ParentID != nil {
				parentMap[task.ID] = *task.ParentID
			}
		}
		for id := range visibleMap {
			curr := id
			for {
				pID, exists := parentMap[curr]
				if !exists || pID == 0 {
					break
				}
				visibleMap[pID] = true
				curr = pID
			}
		}
		visibleTasks = make([]*Task, 0, len(tasks))
		for _, task := range tasks {
			if task.NodeType == TaskNodeTypeFolder {
				if visibleMap[task.ID] || task.IsPublic() {
					visibleTasks = append(visibleTasks, task)
				}
			} else if visibleMap[task.ID] {
				visibleTasks = append(visibleTasks, task)
			}
		}
	}

	return buildTaskTree(visibleTasks), nil
}

// UpdateTask updates one workspace node.
func (s *Service) UpdateTask(ctx context.Context, id uint, req *UpdateTaskRequest) (*Task, error) {
	return s.UpdateTaskForActor(ctx, executionapp.Actor{IsAdmin: true}, id, req)
}

// UpdateTaskForActor updates one workspace node after enforcing actor permissions.
func (s *Service) UpdateTaskForActor(ctx context.Context, actor executionapp.Actor, id uint, req *UpdateTaskRequest) (*Task, error) {
	task, err := s.repo.GetTaskByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if task.Status == TaskStatusArchived {
		return nil, ErrTaskArchived
	}
	if !task.CanUserEdit(uint(actor.UserID), actor.IsAdmin) {
		return nil, ErrTaskReadOnly
	}
	mode, err := normalizeTaskMode(req.Mode)
	if err != nil {
		return nil, err
	}
	nodeType, err := normalizeNodeType(defaultString(req.NodeType, string(task.NodeType)))
	if err != nil {
		return nil, err
	}
	format, err := normalizeContentFormat(defaultString(req.ContentFormat, string(task.ContentFormat)))
	if err != nil {
		return nil, err
	}
	parentID, err := s.validateParentForTask(ctx, id, nodeType, req.ParentID)
	if err != nil {
		return nil, err
	}
	if nodeType == TaskNodeTypeFile && (parentID == nil || *parentID == 0) {
		return nil, ErrRootFileNotAllowed
	}
	name, err := normalizeTaskName(req.Name)
	if err != nil {
		return nil, err
	}
	if err := s.ensureSiblingTaskNameAvailable(ctx, parentID, name, &task.ID); err != nil {
		return nil, err
	}
	content := strings.TrimSpace(req.Content)
	content, err = restoreMaskedTaskContent(format, task.Content, content)
	if err != nil {
		return nil, err
	}
	definition, err := restoreMaskedJSONMap(task.Definition, cloneJSONMap(req.Definition))
	if err != nil {
		return nil, err
	}
	// Non-owner and non-admin cannot alter visibility (is_public) or collaborator list
	if !actor.IsAdmin && (task.CreatedBy != 0 && task.CreatedBy != uint(actor.UserID)) {
		if definition == nil {
			definition = cloneJSONMap(task.Definition)
		} else {
			if origPub, exists := task.Definition["is_public"]; exists {
				definition["is_public"] = origPub
			} else {
				delete(definition, "is_public")
			}
			if origCollabs, exists := task.Definition["collaborators"]; exists {
				definition["collaborators"] = origCollabs
			} else {
				delete(definition, "collaborators")
			}
		}
	}
	jobName := strings.TrimSpace(req.JobName)
	if nodeType == TaskNodeTypeFolder {
		content = ""
		jobName = ""
	} else if content == "" {
		return nil, ErrTaskDefinitionEmpty
	}
	task.ParentID = parentID
	task.NodeType = nodeType
	task.Name = name
	task.Description = strings.TrimSpace(req.Description)
	task.ClusterID = req.ClusterID
	task.EngineVersion = strings.TrimSpace(req.EngineVersion)
	task.Mode = mode
	task.ContentFormat = format
	task.Content = content
	task.JobName = jobName
	task.SortOrder = req.SortOrder
	task.Definition = definition
	if err := validateTaskDefinition(task.Definition); err != nil {
		return nil, err
	}
	if err := s.repo.UpdateTask(ctx, task); err != nil {
		return nil, err
	}
	task.DecoratePermissions(uint(actor.UserID), actor.IsAdmin)
	return task, nil
}

// DeleteTask removes one workspace node and all nested descendants.
func (s *Service) DeleteTask(ctx context.Context, id uint) error {
	return s.DeleteTaskForActor(ctx, executionapp.Actor{IsAdmin: true}, id)
}

// DeleteTaskForActor removes one workspace node after verifying owner/admin rights.
func (s *Service) DeleteTaskForActor(ctx context.Context, actor executionapp.Actor, id uint) error {
	task, err := s.repo.GetTaskByID(ctx, id)
	if err != nil {
		return err
	}
	if !actor.IsAdmin && (task.CreatedBy != 0 && task.CreatedBy != uint(actor.UserID)) {
		return ErrTaskPermissionDenied
	}
	s.applyTaskDefaults(task)

	tasks, err := s.repo.ListAllTasks(ctx)
	if err != nil {
		return err
	}
	targetIDs := collectTaskSubtreeIDs(tasks, id)
	if len(targetIDs) == 0 {
		targetIDs = []uint{id}
	}

	return s.repo.Transaction(ctx, func(tx *Repository) error {
		if err := tx.DeletePreviewRowsByTaskIDs(ctx, targetIDs); err != nil {
			return err
		}
		if err := tx.DeletePreviewTablesByTaskIDs(ctx, targetIDs); err != nil {
			return err
		}
		if err := tx.DeletePreviewSessionsByTaskIDs(ctx, targetIDs); err != nil {
			return err
		}
		if err := tx.DeleteTaskVersionsByTaskIDs(ctx, targetIDs); err != nil {
			return err
		}
		if err := tx.DeleteJobInstancesByTaskIDs(ctx, targetIDs); err != nil {
			return err
		}
		return tx.DeleteTasksByIDs(ctx, targetIDs)
	})
}

func (s *Service) getTaskForExecution(ctx context.Context, id uint, draft *TaskDraftPayload) (*Task, error) {
	task, err := s.repo.GetTaskByID(ctx, id)
	if err != nil {
		return nil, err
	}
	s.applyTaskDefaults(task)
	if draft == nil {
		return task, nil
	}
	name, err := normalizeTaskName(draft.Name)
	if err != nil {
		return nil, err
	}
	mode, err := normalizeTaskMode(draft.Mode)
	if err != nil {
		return nil, err
	}
	format, err := normalizeContentFormat(draft.ContentFormat)
	if err != nil {
		return nil, err
	}
	definition, err := restoreMaskedJSONMap(task.Definition, cloneJSONMap(draft.Definition))
	if err != nil {
		return nil, err
	}
	if err := validateTaskDefinition(definition); err != nil {
		return nil, err
	}
	content, err := restoreMaskedTaskContent(format, task.Content, draft.Content)
	if err != nil {
		return nil, err
	}
	task.Name = name
	task.Description = strings.TrimSpace(draft.Description)
	task.ClusterID = draft.ClusterID
	task.EngineVersion = strings.TrimSpace(draft.EngineVersion)
	task.Mode = mode
	task.ContentFormat = format
	task.Content = content
	task.JobName = strings.TrimSpace(draft.JobName)
	task.Definition = definition
	return task, nil
}

// PublishTask snapshots current file definition and marks task as published.
func (s *Service) PublishTask(ctx context.Context, id uint, comment string, createdBy uint) (*Task, *TaskVersion, error) {
	return s.PublishTaskForActor(ctx, executionapp.Actor{UserID: uint64(createdBy), IsAdmin: true}, id, comment, createdBy)
}

// PublishTaskForActor snapshots current file definition after checking edit permission.
func (s *Service) PublishTaskForActor(ctx context.Context, actor executionapp.Actor, id uint, comment string, createdBy uint) (*Task, *TaskVersion, error) {
	var publishedTask *Task
	var version *TaskVersion
	err := s.repo.Transaction(ctx, func(tx *Repository) error {
		task, err := tx.GetTaskByID(ctx, id)
		if err != nil {
			return err
		}
		s.applyTaskDefaults(task)
		if task.Status == TaskStatusArchived {
			return ErrTaskArchived
		}
		if task.NodeType != TaskNodeTypeFile {
			return ErrTaskNotFile
		}
		if !task.CanUserEdit(uint(actor.UserID), actor.IsAdmin) {
			return ErrTaskReadOnly
		}
		version = &TaskVersion{
			TaskID:                task.ID,
			Version:               task.CurrentVersion + 1,
			NameSnapshot:          task.Name,
			DescriptionSnapshot:   task.Description,
			ClusterIDSnapshot:     task.ClusterID,
			EngineVersionSnapshot: task.EngineVersion,
			ModeSnapshot:          task.Mode,
			ContentFormatSnapshot: task.ContentFormat,
			ContentSnapshot:       task.Content,
			JobNameSnapshot:       task.JobName,
			DefinitionSnapshot:    cloneJSONMap(task.Definition),
			Comment:               strings.TrimSpace(comment),
			CreatedBy:             createdBy,
		}
		if err := tx.CreateTaskVersion(ctx, version); err != nil {
			return err
		}
		task.CurrentVersion = version.Version
		task.Status = TaskStatusPublished
		if err := tx.UpdateTask(ctx, task); err != nil {
			return err
		}
		publishedTask = task
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	publishedTask.DecoratePermissions(uint(actor.UserID), actor.IsAdmin)
	return publishedTask, version, nil
}

// ListTaskVersions returns all immutable snapshots for one file task.
func (s *Service) ListTaskVersions(ctx context.Context, id uint) ([]*TaskVersion, error) {
	task, err := s.repo.GetTaskByID(ctx, id)
	if err != nil {
		return nil, err
	}
	s.applyTaskDefaults(task)
	if task.NodeType != TaskNodeTypeFile {
		return nil, ErrTaskNotFile
	}
	return s.repo.ListTaskVersionsByTaskID(ctx, id)
}

// ListTaskVersionsPaginated returns paginated immutable snapshots for one file task.
func (s *Service) ListTaskVersionsPaginated(ctx context.Context, id uint, page, size int) ([]*TaskVersion, int64, error) {
	return s.ListTaskVersionsPaginatedForActor(ctx, executionapp.Actor{IsAdmin: true}, id, page, size)
}

// ListTaskVersionsPaginatedForActor returns paginated snapshots after verifying view permission.
func (s *Service) ListTaskVersionsPaginatedForActor(ctx context.Context, actor executionapp.Actor, id uint, page, size int) ([]*TaskVersion, int64, error) {
	task, err := s.repo.GetTaskByID(ctx, id)
	if err != nil {
		return nil, 0, err
	}
	s.applyTaskDefaults(task)
	if task.NodeType != TaskNodeTypeFile {
		return nil, 0, ErrTaskNotFile
	}
	if !task.CanUserView(uint(actor.UserID), actor.IsAdmin) {
		return nil, 0, ErrTaskPermissionDenied
	}
	return s.repo.ListTaskVersionsByTaskIDPaginated(ctx, id, page, size)
}

// RollbackTaskVersion restores one immutable snapshot back to the editable task.
func (s *Service) RollbackTaskVersion(ctx context.Context, id uint, versionID uint) (*Task, error) {
	return s.RollbackTaskVersionForActor(ctx, executionapp.Actor{IsAdmin: true}, id, versionID)
}

// RollbackTaskVersionForActor restores snapshot after checking edit permission.
func (s *Service) RollbackTaskVersionForActor(ctx context.Context, actor executionapp.Actor, id uint, versionID uint) (*Task, error) {
	task, err := s.repo.GetTaskByID(ctx, id)
	if err != nil {
		return nil, err
	}
	s.applyTaskDefaults(task)
	if task.NodeType != TaskNodeTypeFile {
		return nil, ErrTaskNotFile
	}
	if !task.CanUserEdit(uint(actor.UserID), actor.IsAdmin) {
		return nil, ErrTaskReadOnly
	}
	version, err := s.repo.GetTaskVersionByID(ctx, id, versionID)
	if err != nil {
		return nil, err
	}
	task.Name = version.NameSnapshot
	task.Description = version.DescriptionSnapshot
	task.ClusterID = version.ClusterIDSnapshot
	task.EngineVersion = version.EngineVersionSnapshot
	task.Mode = version.ModeSnapshot
	task.ContentFormat = version.ContentFormatSnapshot
	task.Content = version.ContentSnapshot
	task.JobName = version.JobNameSnapshot
	task.Definition = cloneJSONMap(version.DefinitionSnapshot)
	task.Status = TaskStatusDraft
	if err := s.repo.UpdateTask(ctx, task); err != nil {
		return nil, err
	}
	task.DecoratePermissions(uint(actor.UserID), actor.IsAdmin)
	return task, nil
}

// DeleteTaskVersion removes one immutable snapshot.
func (s *Service) DeleteTaskVersion(ctx context.Context, id uint, versionID uint) error {
	return s.DeleteTaskVersionForActor(ctx, executionapp.Actor{IsAdmin: true}, id, versionID)
}

// DeleteTaskVersionForActor removes one immutable snapshot after checking edit permission.
func (s *Service) DeleteTaskVersionForActor(ctx context.Context, actor executionapp.Actor, id uint, versionID uint) error {
	task, err := s.repo.GetTaskByID(ctx, id)
	if err != nil {
		return err
	}
	s.applyTaskDefaults(task)
	if task.NodeType != TaskNodeTypeFile {
		return ErrTaskNotFile
	}
	if !task.CanUserEdit(uint(actor.UserID), actor.IsAdmin) {
		return ErrTaskReadOnly
	}
	return s.repo.DeleteTaskVersion(ctx, id, versionID)
}

// ValidateTaskForActor validates task after checking view permission.
func (s *Service) ValidateTaskForActor(ctx context.Context, actor executionapp.Actor, id uint, draft *TaskDraftPayload) (*ValidateResult, error) {
	task, err := s.repo.GetTaskByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !task.CanUserView(uint(actor.UserID), actor.IsAdmin) {
		return nil, ErrTaskPermissionDenied
	}
	return s.ValidateTask(ctx, id, draft)
}

// TestTaskConnectionsForActor tests connections after checking run permission.
func (s *Service) TestTaskConnectionsForActor(ctx context.Context, actor executionapp.Actor, id uint, draft *TaskDraftPayload) (*ValidateResult, error) {
	task, err := s.repo.GetTaskByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !task.CanUserRun(uint(actor.UserID), actor.IsAdmin) {
		return nil, ErrTaskReadOnly
	}
	return s.TestTaskConnections(ctx, id, draft)
}

// BuildTaskDAGForActor builds DAG after checking view permission.
func (s *Service) BuildTaskDAGForActor(ctx context.Context, actor executionapp.Actor, id uint, draft *TaskDraftPayload) (*DAGResult, error) {
	task, err := s.repo.GetTaskByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if !task.CanUserView(uint(actor.UserID), actor.IsAdmin) {
		return nil, ErrTaskPermissionDenied
	}
	return s.BuildTaskDAG(ctx, id, draft)
}

// ValidateTask validates current file content.
func (s *Service) ValidateTask(ctx context.Context, id uint, draft *TaskDraftPayload) (*ValidateResult, error) {
	task, err := s.getTaskForExecution(ctx, id, draft)
	if err != nil {
		return nil, err
	}
	if task.NodeType != TaskNodeTypeFile {
		return nil, ErrTaskNotFile
	}
	errorsList := []string{}
	warnings := []string{}
	if strings.TrimSpace(task.Content) == "" {
		errorsList = append(errorsList, ErrTaskDefinitionEmpty.Error())
	}
	if task.ClusterID == 0 && taskExecutionMode(task) != "local" {
		warnings = append(warnings, "cluster_id is empty, runtime submission will fail until a target cluster is selected")
	}
	if s.configToolClient != nil && s.configToolResolver != nil {
		endpoint, endpointErr := s.configToolResolver.ResolveConfigToolEndpoint(ctx, task.ClusterID, task.Definition)
		if endpointErr == nil {
			req, buildErr := s.buildConfigToolContentRequest(ctx, task, nil)
			if buildErr == nil {
				validateResp, validateErr := s.configToolClient.ValidateConfig(ctx, endpoint, &ConfigToolValidateRequest{
					ConfigToolContentRequest: *req,
					TestConnection:           false,
				})
				if validateErr != nil {
					return nil, validateErr
				}
				errorsList = append(errorsList, validateResp.Errors...)
				warnings = append(warnings, validateResp.Warnings...)
				return &ValidateResult{
					Valid:        validateResp.Valid && len(errorsList) == 0,
					Errors:       dedupeStrings(errorsList),
					Warnings:     dedupeStrings(warnings),
					Summary:      defaultString(validateResp.Summary, "Studio validation finished."),
					Resolved:     map[string]string{"mode": string(task.Mode), "content_format": string(task.ContentFormat)},
					DetectedVars: detectTemplateVariables(task.Content),
					Checks:       toValidateChecks(validateResp.Checks),
				}, nil
			}
		}
	}
	return &ValidateResult{
		Valid:        len(errorsList) == 0,
		Errors:       errorsList,
		Warnings:     warnings,
		Summary:      "Studio validation finished.",
		Resolved:     map[string]string{"mode": string(task.Mode), "content_format": string(task.ContentFormat)},
		DetectedVars: detectTemplateVariables(task.Content),
	}, nil
}

// TestTaskConnections validates connector connectivity for the task definition.
func (s *Service) TestTaskConnections(ctx context.Context, id uint, draft *TaskDraftPayload) (*ValidateResult, error) {
	task, err := s.getTaskForExecution(ctx, id, draft)
	if err != nil {
		return nil, err
	}
	if task.NodeType != TaskNodeTypeFile {
		return nil, ErrTaskNotFile
	}
	if s.configToolClient == nil || s.configToolResolver == nil {
		return &ValidateResult{
			Valid:    false,
			Errors:   []string{"sync: config tool resolver is not configured"},
			Summary:  "Connection test failed.",
			Resolved: map[string]string{"mode": string(task.Mode), "content_format": string(task.ContentFormat)},
		}, nil
	}
	endpoint, err := s.configToolResolver.ResolveConfigToolEndpoint(ctx, task.ClusterID, task.Definition)
	if err != nil {
		return nil, err
	}
	req, err := s.buildConfigToolContentRequest(ctx, task, nil)
	if err != nil {
		return nil, err
	}
	validateResp, err := s.configToolClient.ValidateConfig(ctx, endpoint, &ConfigToolValidateRequest{
		ConfigToolContentRequest: *req,
		TestConnection:           true,
	})
	if err != nil {
		return nil, err
	}
	return &ValidateResult{
		Valid:        validateResp.Valid,
		Errors:       append([]string{}, validateResp.Errors...),
		Warnings:     append([]string{}, validateResp.Warnings...),
		Summary:      defaultString(validateResp.Summary, "Connection test finished."),
		Resolved:     map[string]string{"mode": string(task.Mode), "content_format": string(task.ContentFormat)},
		DetectedVars: detectTemplateVariables(task.Content),
		Checks:       toValidateChecks(validateResp.Checks),
	}, nil
}

// BuildTaskDAG returns DAG projection for the task definition.
func (s *Service) BuildTaskDAG(ctx context.Context, id uint, draft *TaskDraftPayload) (*DAGResult, error) {
	task, err := s.getTaskForExecution(ctx, id, draft)
	if err != nil {
		return nil, err
	}
	if task.NodeType != TaskNodeTypeFile {
		return nil, ErrTaskNotFile
	}
	if s.configToolClient != nil && s.configToolResolver != nil {
		endpoint, err := s.configToolResolver.ResolveConfigToolEndpoint(ctx, task.ClusterID, task.Definition)
		if err != nil {
			return nil, err
		}
		req, buildErr := s.buildConfigToolContentRequest(ctx, task, nil)
		if buildErr != nil {
			return nil, buildErr
		}
		webuiResp, dagErr := s.configToolClient.InspectWebUIDAG(ctx, endpoint, req)
		if dagErr != nil {
			return nil, dagErr
		}
		if webuiResp != nil {
			rawWebUIJob, marshalErr := structToJSONMap(webuiResp)
			if marshalErr != nil {
				return nil, marshalErr
			}
			return &DAGResult{
				Nodes:       extractWebUIDAGNodes(webuiResp.JobDag.VertexInfoMap),
				Edges:       extractWebUIDAGEdges(webuiResp.JobDag.PipelineEdges),
				WebUIJob:    rawWebUIJob,
				SimpleGraph: webuiResp.SimpleGraph,
				Warnings:    append([]string{}, webuiResp.Warnings...),
			}, nil
		}
	}
	if dag, ok := task.Definition["dag"].(map[string]interface{}); ok {
		return &DAGResult{Nodes: toJSONMapSlice(dag["nodes"]), Edges: toJSONMapSlice(dag["edges"])}, nil
	}
	return &DAGResult{Nodes: []JSONMap{{"id": fmt.Sprintf("task-%d", task.ID), "name": task.Name, "type": "pipeline"}}, Edges: []JSONMap{}}, nil
}

func toValidateChecks(items []ConfigToolValidationCheck) []ValidateCheck {
	if len(items) == 0 {
		return nil
	}
	result := make([]ValidateCheck, 0, len(items))
	for _, item := range items {
		result = append(result, ValidateCheck{
			NodeID:        item.NodeID,
			Kind:          item.Kind,
			ConnectorType: item.ConnectorType,
			Target:        item.Target,
			Status:        item.Status,
			Message:       item.Message,
		})
	}
	return result
}

func dedupeStrings(values []string) []string {
	if len(values) == 0 {
		return values
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

// PreviewTask derives preview config then submits a debug run if possible.
func (s *Service) PreviewTask(ctx context.Context, id uint, createdBy uint, opts *PreviewTaskRequest) (*JobInstance, error) {
	var draft *TaskDraftPayload
	if opts != nil {
		draft = opts.Draft
	}
	task, err := s.getTaskForExecution(ctx, id, draft)
	if err != nil {
		return nil, err
	}
	if task.NodeType != TaskNodeTypeFile {
		return nil, ErrTaskNotFile
	}
	now := time.Now()
	previewPlatformJobID := s.nextJobID()
	instance := &JobInstance{
		TaskID:        task.ID,
		TaskVersion:   task.CurrentVersion,
		RunType:       RunTypePreview,
		Status:        JobStatusSuccess,
		SubmitSpec:    JSONMap{"mode": task.Mode, "preview": true},
		ResultPreview: JSONMap{"rows": []map[string]interface{}{}},
		StartedAt:     &now,
		CreatedBy:     createdBy,
	}
	previewPayload, previewFormat, previewResult, err := s.derivePreviewPayload(ctx, task, previewPlatformJobID, opts)
	if err != nil {
		return nil, err
	}
	instance.SubmitSpec["preview_format"] = previewFormat
	if taskExecutionMode(task) == "local" {
		if len(previewPayload) == 0 {
			return nil, ErrPreviewHTTPSinkEmpty
		}
		instance, err := s.submitLocalTaskInstance(ctx, task, createdBy, RunTypePreview, previewPlatformJobID, previewPayload, previewFormat, s.previewJobName(task))
		if err != nil {
			return nil, err
		}
		if previewResult != nil {
			instance.ResultPreview = mergePreviewDeriveMetadata(instance.ResultPreview, previewResult)
			_ = s.repo.UpdateJobInstance(ctx, instance)
		}
		if err := s.ensurePreviewSession(ctx, task, instance, opts); err != nil {
			return nil, err
		}
		return instance, nil
	}
	if len(previewPayload) == 0 {
		instance.ResultPreview["note"] = "preview placeholder created; configure preview_http_sink to enable engine-backed debug preview"
		instance.FinishedAt = &now
	} else {
		instance.PlatformJobID = previewPlatformJobID
		instance.Status = JobStatusRunning
		instance.ResultPreview["note"] = "preview config derived and submitted to engine"
		instance.ResultPreview["payload_bytes"] = len(previewPayload)
		instance.ResultPreview["detected_vars"] = detectTemplateVariables(task.Content)
		if previewResult != nil {
			if strings.TrimSpace(previewResult.Content) != "" {
				instance.ResultPreview["preview_content"] = previewResult.Content
			}
			if previewResult.ContentFormat != "" {
				instance.ResultPreview["content_format"] = previewResult.ContentFormat
			}
			if len(previewResult.Warnings) > 0 {
				instance.ResultPreview["warnings"] = previewResult.Warnings
			}
			if len(previewResult.Graph.Nodes) > 0 {
				instance.ResultPreview["graph"] = map[string]interface{}{"nodes": previewResult.Graph.Nodes, "edges": previewResult.Graph.Edges}
			}
		}
		if s.engineClient != nil && s.runtimeResolver != nil {
			endpoint, err := s.runtimeResolver.ResolveEngineEndpoint(ctx, task.ClusterID, task.Definition)
			if err != nil {
				return nil, err
			}
			resp, err := s.engineClient.Submit(ctx, &EngineSubmitRequest{Endpoint: endpoint, Format: previewFormat, JobID: instance.PlatformJobID, JobName: s.previewJobName(task), Body: previewPayload})
			if err != nil {
				return nil, err
			}
			instance.EngineJobID = strings.TrimSpace(resp.JobID)
			instance.SubmitSpec["engine_api_mode"] = defaultString(resp.APIMode, "v2")
			instance.SubmitSpec["engine_base_url"] = defaultString(resp.EndpointBaseURL, endpoint.BaseURL)
			if strings.EqualFold(defaultString(resp.APIMode, "v2"), "v1") {
				instance.SubmitSpec["engine_legacy_base_url"] = defaultString(resp.EndpointBaseURL, endpoint.LegacyURL)
			} else if endpoint.ContextPath != "" {
				instance.SubmitSpec["engine_context_path"] = endpoint.ContextPath
			}
		} else {
			instance.Status = JobStatusSuccess
			instance.FinishedAt = &now
		}
	}
	if err := s.repo.CreateJobInstance(ctx, instance); err != nil {
		return nil, err
	}
	if err := s.ensurePreviewSession(ctx, task, instance, opts); err != nil {
		return nil, err
	}
	return instance, nil
}

func (s *Service) ensurePreviewSession(ctx context.Context, task *Task, instance *JobInstance, opts *PreviewTaskRequest) error {
	if task == nil || instance == nil || instance.ID == 0 {
		return nil
	}
	rowLimit := normalizePreviewRowLimit(intValueOrZero(task.Definition, "preview_row_limit"))
	if opts != nil && opts.RowLimit > 0 {
		rowLimit = normalizePreviewRowLimit(opts.RowLimit)
	}
	timeoutMinutes := normalizePreviewTimeoutMinutes(intValueOrZero(task.Definition, "preview_timeout_minutes"))
	if opts != nil && opts.TimeoutMinutes > 0 {
		timeoutMinutes = normalizePreviewTimeoutMinutes(opts.TimeoutMinutes)
	}
	now := time.Now()
	session := &PreviewSession{
		JobInstanceID:  instance.ID,
		TaskID:         task.ID,
		PlatformJobID:  strings.TrimSpace(instance.PlatformJobID),
		EngineJobID:    strings.TrimSpace(instance.EngineJobID),
		RowLimit:       rowLimit,
		TimeoutMinutes: timeoutMinutes,
		Status:         "collecting",
		StartedAt:      &now,
	}
	if instance.Status == JobStatusSuccess || instance.Status == JobStatusFailed || instance.Status == JobStatusCanceled {
		session.Status = string(instance.Status)
		session.FinishedAt = instance.FinishedAt
	}
	return s.repo.CreatePreviewSession(ctx, session)
}

// GetPreviewSnapshot returns one incremental preview snapshot for a job instance.
// GetPreviewSnapshot 返回一个作业实例的增量预览快照。
func (s *Service) GetPreviewSnapshot(ctx context.Context, jobID uint, tablePath string) (*PreviewSnapshot, error) {
	instance, err := s.GetJob(ctx, jobID)
	if err != nil {
		return nil, err
	}
	session, err := s.repo.GetPreviewSessionByJobInstanceID(ctx, jobID)
	if err != nil {
		if errors.Is(err, ErrPreviewSessionNotFound) {
			return &PreviewSnapshot{
				JobInstanceID:  jobID,
				PlatformJobID:  strings.TrimSpace(instance.PlatformJobID),
				EngineJobID:    strings.TrimSpace(instance.EngineJobID),
				Status:         string(instance.Status),
				EmptyReason:    "preview_not_ready",
				RowLimit:       normalizePreviewRowLimit(intValueOrZero(instance.SubmitSpec, "row_limit")),
				TimeoutMinutes: normalizePreviewTimeoutMinutes(intValueOrZero(instance.SubmitSpec, "timeout_minutes")),
				Tables:         []*PreviewTableData{},
				Warnings:       stringSliceValue(instance.ResultPreview, "warnings"),
			}, nil
		}
		return nil, err
	}
	tables, err := s.repo.ListPreviewTablesBySessionID(ctx, session.ID)
	if err != nil {
		return nil, err
	}
	tableData := make([]*PreviewTableData, 0, len(tables))
	var selected *PreviewTable
	for _, table := range tables {
		tableData = append(tableData, &PreviewTableData{
			ID:        table.ID,
			TablePath: table.TablePath,
			Columns:   append([]string(nil), table.Columns...),
			RowCount:  table.RowCount,
		})
		if selected == nil {
			selected = table
		}
		if strings.TrimSpace(tablePath) != "" && strings.EqualFold(strings.TrimSpace(tablePath), strings.TrimSpace(table.TablePath)) {
			selected = table
		}
	}
	var selectedData *PreviewTableData
	if selected != nil {
		rows, err := s.repo.ListPreviewRowsByTableID(ctx, selected.ID, 0, 10000)
		if err != nil {
			return nil, err
		}
		selectedRows := make([]map[string]interface{}, 0, len(rows))
		for _, row := range rows {
			selectedRows = append(selectedRows, cloneJSONMap(row.RowData))
		}
		selectedData = &PreviewTableData{
			ID:        selected.ID,
			TablePath: selected.TablePath,
			Columns:   append([]string(nil), selected.Columns...),
			RowCount:  selected.RowCount,
			Rows:      selectedRows,
		}
	}
	snapshot := &PreviewSnapshot{
		SessionID:      session.ID,
		JobInstanceID:  jobID,
		PlatformJobID:  session.PlatformJobID,
		EngineJobID:    session.EngineJobID,
		Status:         session.Status,
		RowLimit:       session.RowLimit,
		TimeoutMinutes: session.TimeoutMinutes,
		TotalRows:      session.TotalRows,
		TableCount:     session.TableCount,
		Truncated:      session.Truncated,
		Tables:         tableData,
		SelectedTable:  selectedData,
	}
	if len(tableData) == 0 {
		snapshot.EmptyReason = "no_preview_rows"
	}
	if instance != nil && instance.ResultPreview != nil {
		if script := strings.TrimSpace(stringValue(instance.ResultPreview, "preview_content")); script != "" {
			snapshot.InjectedScript = script
		}
		if format := strings.TrimSpace(stringValue(instance.ResultPreview, "content_format")); format != "" {
			snapshot.ContentFormat = format
		}
		snapshot.Warnings = stringSliceValue(instance.ResultPreview, "warnings")
	}
	return snapshot, nil
}

// GetJobCheckpointSnapshot returns checkpoint overview/history for one job.
// GetJobCheckpointSnapshot 返回单个作业的 checkpoint 概览与历史。
func (s *Service) GetJobCheckpointSnapshot(
	ctx context.Context,
	jobID uint,
	pipelineID *int,
	limit int,
	status string,
) (*CheckpointSnapshot, error) {
	instance, err := s.GetJob(ctx, jobID)
	if err != nil {
		return nil, err
	}
	snapshot := &CheckpointSnapshot{
		JobInstanceID: jobID,
		PlatformJobID: strings.TrimSpace(instance.PlatformJobID),
		EngineJobID:   strings.TrimSpace(instance.EngineJobID),
		Status:        string(instance.Status),
		History:       []*EngineCheckpointRecord{},
	}
	if submitSpecExecutionMode(instance.SubmitSpec) == "local" {
		snapshot.EmptyReason = "checkpoint_not_supported_local"
		snapshot.Message = "Checkpoint overview is unavailable for local execution mode."
		return snapshot, nil
	}
	if s.engineClient == nil || strings.TrimSpace(instance.EngineJobID) == "" {
		snapshot.EmptyReason = "checkpoint_not_ready"
		snapshot.Message = "Checkpoint overview is not ready yet."
		return snapshot, nil
	}
	endpoint := endpointFromSubmitSpec(instance.SubmitSpec)
	if endpoint == nil {
		snapshot.EmptyReason = "checkpoint_not_ready"
		snapshot.Message = "Checkpoint endpoint is unavailable."
		return snapshot, nil
	}
	overview, overviewErr := s.engineClient.GetJobCheckpointOverview(ctx, endpoint, instance.EngineJobID)
	if overviewErr != nil {
		if isCheckpointFeatureUnsupported(overviewErr) {
			snapshot.EmptyReason = "checkpoint_unsupported"
			snapshot.Message = "Checkpoint overview is only supported on SeaTunnel >= 2.3.13. Please upgrade the cluster version."
			return snapshot, nil
		}
		return nil, overviewErr
	}
	history, historyErr := s.engineClient.GetJobCheckpointHistory(ctx, endpoint, instance.EngineJobID, pipelineID, limit, status)
	if historyErr != nil {
		if isCheckpointFeatureUnsupported(historyErr) {
			snapshot.EmptyReason = "checkpoint_unsupported"
			snapshot.Message = "Checkpoint history is only supported on SeaTunnel >= 2.3.13. Please upgrade the cluster version."
			snapshot.Overview = overview
			return snapshot, nil
		}
		return nil, historyErr
	}
	snapshot.Overview = overview
	snapshot.History = history
	if overview == nil || len(overview.Pipelines) == 0 {
		snapshot.EmptyReason = "checkpoint_not_ready"
		snapshot.Message = "No checkpoint records have been reported for this job yet."
	}
	return snapshot, nil
}

func isCheckpointFeatureUnsupported(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(message, "status=404") ||
		strings.Contains(message, "no context found") ||
		strings.Contains(message, "not supported by legacy engine api")
}

func (s *Service) cleanupExpiredPreviewSessions(ctx context.Context, ttl time.Duration) error {
	if ttl <= 0 {
		ttl = s.previewDataTTL()
	}
	expired, err := s.repo.ListExpiredPreviewSessions(ctx, time.Now().Add(-ttl))
	if err != nil {
		return err
	}
	if len(expired) == 0 {
		return nil
	}
	sessionIDs := make([]uint, 0, len(expired))
	for _, item := range expired {
		if item != nil {
			sessionIDs = append(sessionIDs, item.ID)
		}
	}
	return s.repo.Transaction(ctx, func(tx *Repository) error {
		if err := tx.DeletePreviewRowsBySessionIDs(ctx, sessionIDs); err != nil {
			return err
		}
		if err := tx.DeletePreviewTablesBySessionIDs(ctx, sessionIDs); err != nil {
			return err
		}
		return tx.DeletePreviewSessionsByIDs(ctx, sessionIDs)
	})
}

func (s *Service) stopTimedOutPreviewSessions(ctx context.Context) error {
	sessions, err := s.repo.ListTimedOutPreviewSessions(ctx, time.Now())
	if err != nil {
		return err
	}
	for _, session := range sessions {
		if session == nil {
			continue
		}
		instance, getErr := s.repo.GetJobInstanceByID(ctx, session.JobInstanceID)
		if getErr != nil {
			continue
		}
		if instance.Status == JobStatusPending || instance.Status == JobStatusRunning {
			_, _ = s.CancelJob(ctx, instance.ID, false)
		}
		session.Status = "timeout"
		now := time.Now()
		session.FinishedAt = &now
		session.LastError = "preview timed out before reaching completion"
		_ = s.repo.UpdatePreviewSession(ctx, session)
	}
	return nil
}

// SubmitTask creates one formal run instance with platform-managed job id.
func (s *Service) SubmitTask(ctx context.Context, id uint, createdBy uint, draft *TaskDraftPayload) (*JobInstance, error) {
	task, err := s.getTaskForExecution(ctx, id, draft)
	if err != nil {
		return nil, err
	}
	if task.NodeType != TaskNodeTypeFile {
		return nil, ErrTaskNotFile
	}
	if task.CurrentVersion == 0 {
		return nil, ErrTaskNotPublished
	}
	if taskExecutionMode(task) == "local" {
		jobID := s.nextJobID()
		body, format, jobName, buildErr := s.buildSubmitPayload(ctx, task, buildTaskVariableRuntime(task, jobID))
		if buildErr != nil {
			return nil, buildErr
		}
		return s.submitLocalTaskInstance(ctx, task, createdBy, RunTypeRun, jobID, body, format, jobName)
	}
	return s.submitTaskInstance(ctx, task, createdBy, RunTypeRun, s.nextJobID(), false, nil)
}

// RecoverJob recreates a run using the original platform job id as savepoint recovery source.
func (s *Service) RecoverJob(ctx context.Context, sourceJobID uint, createdBy uint, draft *TaskDraftPayload) (*JobInstance, error) {
	source, err := s.repo.GetJobInstanceByID(ctx, sourceJobID)
	if err != nil {
		return nil, err
	}
	if source.TaskID == 0 || strings.TrimSpace(source.PlatformJobID) == "" {
		return nil, ErrRecoverSourceRequired
	}
	task, err := s.repo.GetTaskByID(ctx, source.TaskID)
	if err != nil {
		return nil, err
	}
	s.applyTaskDefaults(task)
	if draft != nil {
		task, err = s.getTaskForExecution(ctx, source.TaskID, draft)
		if err != nil {
			return nil, err
		}
	}
	if task.NodeType != TaskNodeTypeFile {
		return nil, ErrTaskNotFile
	}
	if draft != nil && task.CurrentVersion == 0 {
		return nil, ErrTaskNotPublished
	}
	if taskExecutionMode(task) == "local" {
		return nil, ErrLocalSavepointUnsupported
	}
	if draft == nil {
		submittedContent := strings.TrimSpace(stringValue(source.SubmitSpec, "submitted_content"))
		if submittedContent != "" {
			return s.submitTaskInstanceWithPayload(
				ctx,
				task,
				createdBy,
				RunTypeRecover,
				strings.TrimSpace(source.PlatformJobID),
				true,
				&source.ID,
				[]byte(submittedContent),
				normalizeSubmitFormat(stringValue(source.SubmitSpec, "submitted_format", "format")),
				strings.TrimSpace(stringValue(source.SubmitSpec, "job_name")),
			)
		}
	}
	return s.submitTaskInstance(ctx, task, createdBy, RunTypeRecover, strings.TrimSpace(source.PlatformJobID), true, &source.ID)
}

func (s *Service) submitTaskInstance(ctx context.Context, task *Task, createdBy uint, runType RunType, platformJobID string, startWithSavepoint bool, recoveredFrom *uint) (*JobInstance, error) {
	submitBody, submitFormat, jobName, err := s.buildSubmitPayload(ctx, task, buildTaskVariableRuntime(task, platformJobID))
	if err != nil {
		return nil, err
	}
	return s.submitTaskInstanceWithPayload(
		ctx,
		task,
		createdBy,
		runType,
		platformJobID,
		startWithSavepoint,
		recoveredFrom,
		submitBody,
		submitFormat,
		jobName,
	)
}

func (s *Service) submitTaskInstanceWithPayload(ctx context.Context, task *Task, createdBy uint, runType RunType, platformJobID string, startWithSavepoint bool, recoveredFrom *uint, submitBody []byte, submitFormat string, jobName string) (*JobInstance, error) {
	now := time.Now()
	instance := &JobInstance{
		TaskID:                  task.ID,
		TaskVersion:             task.CurrentVersion,
		RunType:                 runType,
		PlatformJobID:           platformJobID,
		RecoveredFromInstanceID: recoveredFrom,
		Status:                  JobStatusRunning,
		SubmitSpec: JSONMap{
			"mode":                 task.Mode,
			"engine_version":       task.EngineVersion,
			"cluster_id":           task.ClusterID,
			"format":               submitFormat,
			"submitted_content":    string(submitBody),
			"submitted_format":     submitFormat,
			"job_name":             jobName,
			"trigger_source":       triggerSourceForRunType(runType),
			"platform_job_id":      platformJobID,
			"start_with_savepoint": startWithSavepoint,
		},
		StartedAt: &now,
		CreatedBy: createdBy,
	}
	if s.engineClient != nil && s.runtimeResolver != nil {
		endpoint, err := s.runtimeResolver.ResolveEngineEndpoint(ctx, task.ClusterID, task.Definition)
		if err != nil {
			return nil, err
		}
		if s.executionTargetResolver != nil {
			if target, targetErr := s.executionTargetResolver.ResolveExecutionTarget(ctx, task.ClusterID, task.Definition); targetErr == nil && target != nil {
				instance.SubmitSpec["target_node_id"] = target.NodeID
				instance.SubmitSpec["target_host_id"] = target.HostID
				instance.SubmitSpec["target_agent_id"] = target.AgentID
				instance.SubmitSpec["install_dir"] = target.InstallDir
			}
		}
		resp, err := s.engineClient.Submit(ctx, &EngineSubmitRequest{Endpoint: endpoint, Format: submitFormat, JobID: platformJobID, JobName: jobName, StartWithSavepoint: startWithSavepoint, Body: submitBody})
		if err != nil {
			return nil, err
		}
		instance.EngineJobID = strings.TrimSpace(resp.JobID)
		instance.SubmitSpec["engine_api_mode"] = defaultString(resp.APIMode, "v2")
		instance.SubmitSpec["engine_base_url"] = defaultString(resp.EndpointBaseURL, endpoint.BaseURL)
		if strings.EqualFold(defaultString(resp.APIMode, "v2"), "v1") {
			instance.SubmitSpec["engine_legacy_base_url"] = defaultString(resp.EndpointBaseURL, endpoint.LegacyURL)
		} else if endpoint.ContextPath != "" {
			instance.SubmitSpec["engine_context_path"] = endpoint.ContextPath
		}
	} else {
		instance.EngineJobID = platformJobID
	}
	if err := s.repo.CreateJobInstance(ctx, instance); err != nil {
		return nil, err
	}
	return instance, nil
}

// ListJobs returns paginated job instances.
func (s *Service) ListJobs(ctx context.Context, filter *JobFilter) ([]*JobInstance, int64, error) {
	items, total, err := s.repo.ListJobInstances(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	for i, item := range items {
		refreshed, refreshErr := s.refreshJobInstance(ctx, item)
		if refreshErr == nil && refreshed != nil {
			items[i] = refreshed
		}
	}
	return items, total, nil
}

// GetJob returns one job instance and refreshes live runtime state when possible.
func (s *Service) GetJob(ctx context.Context, id uint) (*JobInstance, error) {
	instance, err := s.repo.GetJobInstanceByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.refreshJobInstance(ctx, instance)
}

// CancelJob 请求真实停止作业，并在确认停止前保留取消中间状态。
// CancelJob requests a real stop and keeps intermediate cancellation states until the stop is confirmed.
func (s *Service) CancelJob(ctx context.Context, id uint, stopWithSavepoint bool) (*JobInstance, error) {
	instance, err := s.repo.GetJobInstanceByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if instance.Status == JobStatusSuccess || instance.Status == JobStatusFailed || instance.Status == JobStatusCanceled {
		return nil, ErrJobAlreadyFinished
	}
	if instance.Status == JobStatusCancelRequested || instance.Status == JobStatusCancelling {
		return instance, nil
	}
	previousStatus := instance.Status
	if err := s.repo.UpdateJobStatus(ctx, instance.ID, []JobStatus{JobStatusPending, JobStatusRunning}, map[string]any{
		"status":        JobStatusCancelRequested,
		"error_message": "",
	}); err != nil {
		if errors.Is(err, ErrJobStatusChanged) {
			return s.repo.GetJobInstanceByID(ctx, id)
		}
		return nil, err
	}
	instance.Status = JobStatusCancelRequested

	targetStatus := JobStatusCancelling
	if submitSpecExecutionMode(instance.SubmitSpec) == "local" {
		if err := s.stopLocalJob(ctx, instance); err != nil {
			s.restoreJobAfterCancelFailure(ctx, instance, previousStatus)
			return nil, err
		}
		targetStatus = JobStatusCanceled
	} else {
		if s.engineClient == nil || strings.TrimSpace(instance.EngineJobID) == "" {
			s.restoreJobAfterCancelFailure(ctx, instance, previousStatus)
			return nil, ErrExecutionTargetUnavailable
		}
		endpoint := endpointFromSubmitSpec(instance.SubmitSpec)
		if endpoint == nil {
			s.restoreJobAfterCancelFailure(ctx, instance, previousStatus)
			return nil, ErrExecutionTargetUnavailable
		}
		if err := s.engineClient.StopJob(ctx, endpoint, instance.EngineJobID, stopWithSavepoint); err != nil {
			s.restoreJobAfterCancelFailure(ctx, instance, previousStatus)
			return nil, err
		}
	}

	now := time.Now()
	instance.Status = targetStatus
	if targetStatus == JobStatusCancelling {
		instance.FinishedAt = nil
		instance.ResultPreview = mergeJobRuntimeInfo(instance.ResultPreview, &EngineJobInfo{
			JobID:     instance.EngineJobID,
			JobName:   strings.TrimSpace(stringValue(instance.SubmitSpec, "job_name")),
			JobStatus: cancelRequestedRuntimeStatus(stopWithSavepoint),
		})
	} else {
		instance.FinishedAt = &now
		instance.ResultPreview = mergeJobRuntimeInfo(instance.ResultPreview, &EngineJobInfo{
			JobID:        instance.EngineJobID,
			JobName:      strings.TrimSpace(stringValue(instance.SubmitSpec, "job_name")),
			JobStatus:    "CANCELED",
			FinishedTime: now.Format(time.DateTime),
		})
	}
	if err := s.repo.UpdateJobStatus(ctx, instance.ID, []JobStatus{JobStatusCancelRequested}, map[string]any{
		"status":         instance.Status,
		"finished_at":    instance.FinishedAt,
		"result_preview": instance.ResultPreview,
		"error_message":  "",
	}); err != nil {
		return nil, err
	}
	if err := s.syncExecutionFromJob(ctx, instance); err != nil {
		return nil, err
	}
	return instance, nil
}

func (s *Service) restoreJobAfterCancelFailure(ctx context.Context, instance *JobInstance, previousStatus JobStatus) {
	if instance == nil {
		return
	}
	_ = s.repo.UpdateJobStatus(ctx, instance.ID, []JobStatus{JobStatusCancelRequested}, map[string]any{
		"status":        previousStatus,
		"error_message": "cancel request failed",
		"finished_at":   nil,
	})
	instance.Status = previousStatus
	instance.ErrorMessage = "cancel request failed"
	instance.FinishedAt = nil
	_ = s.syncExecutionFromJob(ctx, instance)
}

func cancelRequestedRuntimeStatus(stopWithSavepoint bool) string {
	if stopWithSavepoint {
		return "DOING_SAVEPOINT"
	}
	return "CANCEL_REQUESTED"
}

func (s *Service) validateParent(ctx context.Context, parentID *uint) (*uint, error) {
	if parentID == nil || *parentID == 0 {
		return nil, nil
	}
	parent, err := s.repo.GetTaskByID(ctx, *parentID)
	if err != nil {
		return nil, err
	}
	s.applyTaskDefaults(parent)
	if parent.NodeType != TaskNodeTypeFolder {
		return nil, ErrParentTaskNotFolder
	}
	return parentID, nil
}

func (s *Service) validateParentForTask(ctx context.Context, taskID uint, nodeType TaskNodeType, parentID *uint) (*uint, error) {
	validatedParentID, err := s.validateParent(ctx, parentID)
	if err != nil {
		return nil, err
	}
	if validatedParentID == nil || *validatedParentID == 0 {
		return nil, nil
	}
	if *validatedParentID == taskID {
		return nil, ErrTaskParentCycle
	}
	if nodeType != TaskNodeTypeFolder {
		return validatedParentID, nil
	}
	cursor := validatedParentID
	for cursor != nil && *cursor != 0 {
		if *cursor == taskID {
			return nil, ErrTaskParentCycle
		}
		parent, err := s.repo.GetTaskByID(ctx, *cursor)
		if err != nil {
			return nil, err
		}
		cursor = parent.ParentID
	}
	return validatedParentID, nil
}

func (s *Service) applyTaskDefaults(task *Task) {
	if task == nil {
		return
	}
	if task.NodeType == "" {
		task.NodeType = TaskNodeTypeFile
	}
	if task.ContentFormat == "" {
		task.ContentFormat = ContentFormatHOCON
	}
	if task.Mode == "" {
		task.Mode = TaskModeBatch
	}
	if task.Definition == nil {
		task.Definition = JSONMap{}
	}
}

func (s *Service) nextJobID() string {
	if s.jobIDGenerator == nil {
		s.jobIDGenerator = NewJobIDGenerator()
	}
	return s.jobIDGenerator.NextJobID()
}

func (s *Service) previewJobName(task *Task) string {
	base := strings.TrimSpace(task.JobName)
	if base == "" {
		base = strings.TrimSpace(task.Name)
	}
	if base == "" {
		base = "sync-preview"
	}
	return base + "-preview"
}

func normalizeTaskMode(raw string) (TaskMode, error) {
	switch strings.TrimSpace(raw) {
	case "", string(TaskModeBatch):
		return TaskModeBatch, nil
	case string(TaskModeStreaming):
		return TaskModeStreaming, nil
	default:
		return "", ErrInvalidTaskMode
	}
}

func normalizeTaskName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", ErrTaskNameRequired
	}
	if !safeTaskNamePattern.MatchString(name) {
		return "", ErrTaskNameInvalid
	}
	return name, nil
}

func normalizeNodeType(raw string) (TaskNodeType, error) {
	switch strings.TrimSpace(raw) {
	case "", string(TaskNodeTypeFile):
		return TaskNodeTypeFile, nil
	case string(TaskNodeTypeFolder):
		return TaskNodeTypeFolder, nil
	default:
		return "", ErrInvalidNodeType
	}
}

func normalizeContentFormat(raw string) (ContentFormat, error) {
	switch normalizeSubmitFormat(raw) {
	case "hocon":
		return ContentFormatHOCON, nil
	case "json":
		return ContentFormatJSON, nil
	default:
		return "", ErrInvalidContentFormat
	}
}

func cloneJSONMap(src JSONMap) JSONMap {
	if src == nil {
		return JSONMap{}
	}
	cloned := make(JSONMap, len(src))
	for k, v := range src {
		cloned[k] = v
	}
	return cloned
}

func (s *Service) decorateTaskScheduleMetadata(ctx context.Context, tasks []*Task) error {
	if len(tasks) == 0 {
		return nil
	}
	taskIDs := make([]uint, 0, len(tasks))
	for _, task := range tasks {
		if task == nil {
			continue
		}
		taskIDs = append(taskIDs, task.ID)
		cfg, err := parseTaskSchedule(task.Definition)
		if err != nil || cfg == nil {
			continue
		}
		task.ScheduleEnabled = cfg.Enabled
		task.ScheduleCronExpr = cfg.CronExpr
		task.ScheduleTimezone = cfg.Timezone
		if cfg.Enabled && strings.TrimSpace(cfg.CronExpr) != "" {
			nextRun, nextErr := schedulex.NextRun(cfg.CronExpr, time.Now(), cfg.Timezone)
			if nextErr == nil {
				task.ScheduleNextTriggeredAt = nextRun
			}
		}
	}
	latestScheduled, err := s.repo.GetLatestScheduledJobInstancesByTaskIDs(ctx, taskIDs)
	if err != nil {
		return err
	}
	for _, task := range tasks {
		if task == nil {
			continue
		}
		job := latestScheduled[task.ID]
		if job == nil {
			continue
		}
		if job.StartedAt != nil {
			task.ScheduleLastTriggeredAt = job.StartedAt
			continue
		}
		createdAt := job.CreatedAt
		task.ScheduleLastTriggeredAt = &createdAt
	}
	return nil
}

func buildTaskTree(tasks []*Task) []*TaskTreeNode {
	nodes := make(map[uint]*TaskTreeNode, len(tasks))
	roots := make([]*TaskTreeNode, 0)
	for _, task := range tasks {
		nodes[task.ID] = &TaskTreeNode{
			ID:                      task.ID,
			ParentID:                task.ParentID,
			NodeType:                task.NodeType,
			Name:                    task.Name,
			Description:             task.Description,
			ClusterID:               task.ClusterID,
			EngineVersion:           task.EngineVersion,
			Mode:                    task.Mode,
			Status:                  task.Status,
			ContentFormat:           task.ContentFormat,
			Content:                 task.Content,
			JobName:                 task.JobName,
			Definition:              cloneJSONMap(task.Definition),
			SortOrder:               task.SortOrder,
			CurrentVersion:          task.CurrentVersion,
			ScheduleEnabled:         task.ScheduleEnabled,
			ScheduleCronExpr:        task.ScheduleCronExpr,
			ScheduleTimezone:        task.ScheduleTimezone,
			ScheduleLastTriggeredAt: task.ScheduleLastTriggeredAt,
			ScheduleNextTriggeredAt: task.ScheduleNextTriggeredAt,
			CreatedBy:               task.CreatedBy,
			CanEdit:                 task.CanEdit,
			CanRun:                  task.CanRun,
			IsOwner:                 task.IsOwner,
			IsCollaborator:          task.IsCollaborator,
			IsPublicTask:            task.IsPublicTask,
			Children:                []*TaskTreeNode{},
		}
	}
	for _, task := range tasks {
		node := nodes[task.ID]
		if task.ParentID != nil {
			if parent, ok := nodes[*task.ParentID]; ok {
				parent.Children = append(parent.Children, node)
				continue
			}
		}
		roots = append(roots, node)
	}
	sortTreeNodes(roots)
	return roots
}

func collectTaskSubtreeIDs(tasks []*Task, rootID uint) []uint {
	if len(tasks) == 0 {
		return nil
	}
	childrenByParent := make(map[uint][]uint)
	for _, task := range tasks {
		if task.ParentID != nil {
			childrenByParent[*task.ParentID] = append(childrenByParent[*task.ParentID], task.ID)
		}
	}
	result := make([]uint, 0)
	stack := []uint{rootID}
	seen := make(map[uint]struct{})
	for len(stack) > 0 {
		last := len(stack) - 1
		current := stack[last]
		stack = stack[:last]
		if _, ok := seen[current]; ok {
			continue
		}
		seen[current] = struct{}{}
		result = append(result, current)
		stack = append(stack, childrenByParent[current]...)
	}
	return result
}

func sortTreeNodes(nodes []*TaskTreeNode) {
	sort.SliceStable(nodes, func(i, j int) bool {
		if nodes[i].SortOrder != nodes[j].SortOrder {
			return nodes[i].SortOrder < nodes[j].SortOrder
		}
		if nodes[i].NodeType != nodes[j].NodeType {
			return nodes[i].NodeType < nodes[j].NodeType
		}
		return strings.ToLower(nodes[i].Name) < strings.ToLower(nodes[j].Name)
	})
	for _, node := range nodes {
		if len(node.Children) > 0 {
			sortTreeNodes(node.Children)
		}
	}
}

func toJSONMapSlice(value interface{}) []JSONMap {
	items, ok := value.([]interface{})
	if !ok {
		return []JSONMap{}
	}
	result := make([]JSONMap, 0, len(items))
	for _, item := range items {
		if m, ok := item.(map[string]interface{}); ok {
			result = append(result, JSONMap(m))
		}
	}
	return result
}

func toJSONMapSliceFromMaps(items []map[string]interface{}) []JSONMap {
	result := make([]JSONMap, 0, len(items))
	for _, item := range items {
		result = append(result, JSONMap(item))
	}
	return result
}

func structToJSONMap(value interface{}) (JSONMap, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var result JSONMap
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func extractWebUIDAGNodes(vertexInfoMap map[string]ConfigToolWebUIDAGVertexInfo) []JSONMap {
	type keyedVertex struct {
		key   string
		value ConfigToolWebUIDAGVertexInfo
	}
	keyed := make([]keyedVertex, 0, len(vertexInfoMap))
	for key, value := range vertexInfoMap {
		keyed = append(keyed, keyedVertex{key: key, value: value})
	}
	sort.SliceStable(keyed, func(i, j int) bool {
		if keyed[i].value.VertexID != keyed[j].value.VertexID {
			return keyed[i].value.VertexID < keyed[j].value.VertexID
		}
		return keyed[i].key < keyed[j].key
	})
	nodes := make([]JSONMap, 0, len(keyed))
	for _, item := range keyed {
		nodes = append(nodes, JSONMap{
			"id":               item.value.VertexID,
			"vertexId":         item.value.VertexID,
			"name":             item.value.ConnectorType,
			"type":             item.value.Type,
			"connectorType":    item.value.ConnectorType,
			"tablePaths":       append([]string{}, item.value.TablePaths...),
			"tableColumns":     item.value.TableColumns,
			"tableSchemas":     item.value.TableSchemas,
			"saveModePreviews": item.value.SaveModePreviews,
			"saveModeWarnings": append([]string{}, item.value.SaveModeWarnings...),
		})
	}
	return nodes
}

func extractWebUIDAGEdges(pipelineEdges map[string][]ConfigToolWebUIDAGEdge) []JSONMap {
	keys := make([]string, 0, len(pipelineEdges))
	for key := range pipelineEdges {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	edges := make([]JSONMap, 0)
	for _, pipelineID := range keys {
		for _, edge := range pipelineEdges[pipelineID] {
			edges = append(edges, JSONMap{
				"pipelineId":     pipelineID,
				"inputVertexId":  edge.InputVertexID,
				"targetVertexId": edge.TargetVertexID,
				"source":         edge.InputVertexID,
				"target":         edge.TargetVertexID,
			})
		}
	}
	return edges
}

func (s *Service) usesLegacyPluginIOKeys(ctx context.Context, clusterID uint) bool {
	if s == nil || s.clusterVersionProvider == nil || clusterID == 0 {
		return false
	}
	version, err := s.clusterVersionProvider.GetClusterVersion(ctx, clusterID)
	if err != nil {
		return false
	}
	version = strings.TrimSpace(version)
	if version == "" {
		return false
	}
	return seatunnel.CompareVersions(version, "2.3.9") < 0
}

func rewritePluginIOKeysForLegacy(content string, legacy bool) string {
	if !legacy || strings.TrimSpace(content) == "" {
		return content
	}
	replacer := strings.NewReplacer(
		"plugin_input", "source_table_name",
		"plugin_output", "result_table_name",
	)
	return replacer.Replace(content)
}

func (s *Service) buildSubmitPayload(ctx context.Context, task *Task, runtime *taskVariableRuntime) ([]byte, string, string, error) {
	if task == nil {
		return nil, "", "", fmt.Errorf("sync: task is required")
	}
	format := normalizeSubmitFormat(string(task.ContentFormat))
	jobName := strings.TrimSpace(task.JobName)
	if jobName == "" {
		jobName = strings.TrimSpace(task.Name)
	}
	resolvedContent, err := s.resolveTaskContent(ctx, task, runtime)
	if err != nil {
		return nil, "", "", err
	}
	if raw := strings.TrimSpace(resolvedContent); raw != "" {
		return []byte(raw), format, jobName, nil
	}
	if payload := mapValue(task.Definition, "pipeline_json", "config_json"); payload != nil {
		body, err := jsonMarshal(payload)
		return body, "json", jobName, err
	}
	return nil, "", "", ErrTaskDefinitionEmpty
}

func buildPreviewPayload(definition JSONMap) ([]byte, string) {
	if definition == nil {
		return nil, ""
	}
	if raw := strings.TrimSpace(stringValue(definition, "preview_pipeline_text", "preview_config_text")); raw != "" {
		return []byte(raw), normalizeSubmitFormat(stringValue(definition, "preview_format", "format"))
	}
	if payload := mapValue(definition, "preview_pipeline_json", "preview_config_json"); payload != nil {
		body, err := jsonMarshal(payload)
		if err == nil {
			return body, "json"
		}
	}
	return nil, ""
}

func (s *Service) buildConfigToolContentRequest(ctx context.Context, task *Task, runtime *taskVariableRuntime) (*ConfigToolContentRequest, error) {
	if task == nil {
		return nil, ErrTaskDefinitionEmpty
	}
	resolvedContent, err := s.resolveTaskContent(ctx, task, runtime)
	if err != nil {
		return nil, err
	}
	if raw := strings.TrimSpace(resolvedContent); raw != "" {
		return &ConfigToolContentRequest{Content: raw, ContentFormat: normalizeSubmitFormat(string(task.ContentFormat)), Variables: stringSliceValue(task.Definition, "variables")}, nil
	}
	if path := strings.TrimSpace(stringValue(task.Definition, "file_path", "config_file_path")); path != "" {
		return &ConfigToolContentRequest{FilePath: path, ContentFormat: normalizeSubmitFormat(string(task.ContentFormat)), Variables: stringSliceValue(task.Definition, "variables")}, nil
	}
	return nil, ErrTaskDefinitionEmpty
}

func (s *Service) derivePreviewPayload(ctx context.Context, task *Task, platformJobID string, opts *PreviewTaskRequest) ([]byte, string, *ConfigToolPreviewResponse, error) {
	if payload, format := buildPreviewPayload(task.Definition); len(payload) > 0 {
		return payload, format, nil, nil
	}
	if s.configToolClient == nil || s.configToolResolver == nil {
		return nil, "", nil, nil
	}
	endpoint, err := s.configToolResolver.ResolveConfigToolEndpoint(ctx, task.ClusterID, task.Definition)
	if err != nil {
		return nil, "", nil, nil
	}
	req, err := s.buildConfigToolPreviewRequest(ctx, task, platformJobID, opts)
	if err != nil {
		return nil, "", nil, err
	}
	mode := strings.ToLower(strings.TrimSpace(stringValue(task.Definition, "preview_mode")))
	if opts != nil && strings.TrimSpace(opts.Mode) != "" {
		mode = strings.ToLower(strings.TrimSpace(opts.Mode))
	}
	var resp *ConfigToolPreviewResponse
	switch mode {
	case "", "source":
		resp, err = s.configToolClient.DeriveSourcePreview(ctx, endpoint, req)
	case "transform":
		resp, err = s.configToolClient.DeriveTransformPreview(ctx, endpoint, req)
	default:
		return nil, "", nil, ErrInvalidPreviewMode
	}
	if err != nil {
		return nil, "", nil, err
	}
	if resp == nil || !resp.OK || strings.TrimSpace(resp.Content) == "" {
		return nil, "", nil, fmt.Errorf("sync: preview derive returned empty content")
	}
	resp.Content = rewritePluginIOKeysForLegacy(resp.Content, s.usesLegacyPluginIOKeys(ctx, task.ClusterID))
	return []byte(resp.Content), normalizeSubmitFormat(resp.ContentFormat), resp, nil
}

func (s *Service) buildConfigToolPreviewRequest(ctx context.Context, task *Task, platformJobID string, opts *PreviewTaskRequest) (*ConfigToolPreviewRequest, error) {
	contentReq, err := s.buildConfigToolContentRequest(ctx, task, buildTaskVariableRuntime(task, platformJobID))
	if err != nil {
		return nil, err
	}
	httpSink := mapValue(task.Definition, "preview_http_sink", "http_sink")
	if len(httpSink) == 0 {
		return nil, ErrPreviewHTTPSinkEmpty
	}
	rowLimit := normalizePreviewRowLimit(intValueOrZero(task.Definition, "preview_row_limit"))
	if opts != nil && opts.RowLimit > 0 {
		rowLimit = normalizePreviewRowLimit(opts.RowLimit)
	}
	httpSink = injectPreviewCollectIdentifiers(httpSink, strings.TrimSpace(platformJobID), rowLimit)
	httpSink = injectPreviewCollectAuth(httpSink, strings.TrimSpace(platformJobID))
	req := &ConfigToolPreviewRequest{ConfigToolContentRequest: *contentReq, PlatformJobID: strings.TrimSpace(platformJobID), EngineJobID: strings.TrimSpace(platformJobID), PreviewRowLimit: &rowLimit, OutputFormat: normalizePreviewOutputFormat(stringValue(task.Definition, "preview_output_format")), HttpSink: httpSink, EnvOverrides: mapValue(task.Definition, "preview_env_overrides", "env_overrides"), MetadataFields: mapValue(task.Definition, "preview_metadata_fields", "metadata_fields"), MetadataOutputDataset: strings.TrimSpace(stringValue(task.Definition, "preview_metadata_output_dataset")), SourceNodeID: strings.TrimSpace(stringValue(task.Definition, "preview_source_node_id", "source_node_id")), TransformNodeID: strings.TrimSpace(stringValue(task.Definition, "preview_transform_node_id", "transform_node_id"))}
	if opts != nil {
		if strings.TrimSpace(opts.SourceNodeID) != "" {
			req.SourceNodeID = strings.TrimSpace(opts.SourceNodeID)
		}
		if opts.SourceIndex != nil {
			req.SourceIndex = opts.SourceIndex
		}
		if strings.TrimSpace(opts.TransformNodeID) != "" {
			req.TransformNodeID = strings.TrimSpace(opts.TransformNodeID)
		}
		if opts.TransformIndex != nil {
			req.TransformIndex = opts.TransformIndex
		}
	}
	if index, ok := intValue(task.Definition, "preview_source_index", "source_index"); ok {
		req.SourceIndex = &index
	}
	if index, ok := intValue(task.Definition, "preview_transform_index", "transform_index"); ok {
		req.TransformIndex = &index
	}
	return req, nil
}

func buildPreviewCollectToken(platformJobID string) string {
	secret := strings.TrimSpace(config.Config.App.SessionSecret)
	if secret == "" || strings.TrimSpace(platformJobID) == "" {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte("preview-collect:"))
	_, _ = mac.Write([]byte(strings.TrimSpace(platformJobID)))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func injectPreviewCollectAuth(httpSink map[string]interface{}, platformJobID string) map[string]interface{} {
	if len(httpSink) == 0 {
		return httpSink
	}
	token := buildPreviewCollectToken(platformJobID)
	if token == "" {
		return httpSink
	}
	cloned := cloneAnyMap(httpSink)
	headers := map[string]interface{}{}
	if existing := cloneAnyMap(mapValue(cloned, "headers")); len(existing) > 0 {
		headers = existing
	}
	headers["X-Preview-Token"] = token
	cloned["headers"] = headers
	return cloned
}

func injectPreviewCollectIdentifiers(httpSink map[string]interface{}, platformJobID string, rowLimit int) map[string]interface{} {
	if len(httpSink) == 0 {
		return httpSink
	}
	cloned := cloneAnyMap(httpSink)
	if platformJobID == "" {
		return cloned
	}
	urlValue, ok := cloned["url"].(string)
	if !ok || strings.TrimSpace(urlValue) == "" {
		return cloned
	}
	urlValue = strings.TrimSpace(urlValue)
	parsed, err := url.Parse(urlValue)
	if err != nil {
		return cloned
	}
	query := parsed.Query()
	query.Set("platform_job_id", platformJobID)
	query.Set("engine_job_id", platformJobID)
	query.Set("row_limit", strconv.Itoa(normalizePreviewRowLimit(rowLimit)))
	parsed.RawQuery = query.Encode()
	cloned["url"] = parsed.String()
	return cloned
}

func normalizePreviewCollectRequest(req *PreviewCollectRequest, rawBody []byte) {
	if req == nil {
		return
	}
	if strings.TrimSpace(req.PlatformJobID) == "" && strings.TrimSpace(req.EngineJobID) == "" && len(rawBody) == 0 {
		return
	}
	if len(req.Rows) == 0 && len(req.Columns) == 0 && strings.TrimSpace(req.Dataset) == "" {
		body := strings.TrimSpace(string(rawBody))
		if body != "" && body != "null" {
			var payload interface{}
			if err := json.Unmarshal(rawBody, &payload); err == nil {
				switch typed := payload.(type) {
				case map[string]interface{}:
					req.Rows = []map[string]interface{}{typed}
				case []interface{}:
					rows := make([]map[string]interface{}, 0, len(typed))
					for _, item := range typed {
						if mapped, ok := item.(map[string]interface{}); ok {
							rows = append(rows, mapped)
						}
					}
					req.Rows = rows
				}
			}
		}
	}
	normalizePreviewRowsAndDataset(req)
}

func normalizePreviewRowsAndDataset(req *PreviewCollectRequest) {
	if req == nil || len(req.Rows) == 0 {
		return
	}
	columnsSeen := map[string]struct{}{}
	columns := make([]string, 0)
	for rowIndex := range req.Rows {
		row := req.Rows[rowIndex]
		if row == nil {
			continue
		}
		if rowKind, ok := row["__st_debug_rowkind"].(string); ok && strings.TrimSpace(rowKind) != "" {
			row["RowKind"] = strings.TrimSpace(rowKind)
		}
		dbName, _ := row["__st_debug_db"].(string)
		tableName, _ := row["__st_debug_table"].(string)
		if strings.TrimSpace(req.Dataset) == "" {
			switch {
			case strings.TrimSpace(dbName) != "" && strings.TrimSpace(tableName) != "":
				req.Dataset = strings.TrimSpace(dbName) + "." + strings.TrimSpace(tableName)
			case strings.TrimSpace(tableName) != "":
				req.Dataset = strings.TrimSpace(tableName)
			}
		}
		if req.Catalog == nil && (strings.TrimSpace(dbName) != "" || strings.TrimSpace(tableName) != "") {
			req.Catalog = map[string]interface{}{}
			if strings.TrimSpace(dbName) != "" {
				req.Catalog["database"] = strings.TrimSpace(dbName)
			}
			if strings.TrimSpace(tableName) != "" {
				req.Catalog["table"] = strings.TrimSpace(tableName)
			}
		}
		delete(row, "__st_debug_db")
		delete(row, "__st_debug_table")
		delete(row, "__st_debug_rowkind")
		for key := range row {
			trimmed := strings.TrimSpace(key)
			if trimmed == "" {
				continue
			}
			if _, exists := columnsSeen[trimmed]; exists {
				continue
			}
			columnsSeen[trimmed] = struct{}{}
			columns = append(columns, trimmed)
		}
	}
	if _, exists := columnsSeen["RowKind"]; exists {
		reordered := make([]string, 0, len(columns))
		reordered = append(reordered, "RowKind")
		for _, column := range columns {
			if column == "RowKind" {
				continue
			}
			reordered = append(reordered, column)
		}
		columns = reordered
	}
	if len(req.Columns) == 0 && len(columns) > 0 {
		req.Columns = make([]interface{}, 0, len(columns))
		for _, column := range columns {
			req.Columns = append(req.Columns, column)
		}
	}
}

func endpointFromSubmitSpec(spec JSONMap) *EngineEndpoint {
	if spec == nil {
		return nil
	}
	baseURL := strings.TrimSpace(stringValue(spec, "engine_base_url"))
	legacyURL := strings.TrimSpace(stringValue(spec, "engine_legacy_base_url"))
	apiMode := strings.TrimSpace(stringValue(spec, "engine_api_mode"))
	if baseURL == "" && legacyURL == "" {
		return nil
	}
	endpoint := &EngineEndpoint{
		BaseURL:     baseURL,
		ContextPath: strings.TrimSpace(stringValue(spec, "engine_context_path")),
		LegacyURL:   legacyURL,
		APIMode:     apiMode,
	}
	if endpoint.BaseURL == "" && strings.EqualFold(endpoint.APIMode, "v1") {
		endpoint.BaseURL = endpoint.LegacyURL
	}
	if endpoint.LegacyURL == "" && strings.EqualFold(endpoint.APIMode, "v1") {
		endpoint.LegacyURL = endpoint.BaseURL
	}
	return endpoint
}

func mergeJobRuntimeInfo(existing JSONMap, info *EngineJobInfo) JSONMap {
	if existing == nil {
		existing = JSONMap{}
	}
	if info == nil {
		return existing
	}
	if strings.TrimSpace(info.JobStatus) != "" {
		existing["job_status"] = strings.TrimSpace(info.JobStatus)
	}
	if info.JobDag != nil {
		existing["job_dag"] = info.JobDag
	}
	if info.Metrics != nil {
		existing["metrics"] = info.Metrics
	}
	if info.CreateTime != "" {
		existing["create_time"] = info.CreateTime
	}
	if info.FinishedTime != "" {
		existing["finished_time"] = info.FinishedTime
	}
	if strings.TrimSpace(info.JobName) != "" {
		existing["job_name"] = strings.TrimSpace(info.JobName)
	}
	return existing
}

func parseEngineJobTime(value string) *time.Time {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	layouts := []string{
		time.DateTime,
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006/01/02 15:04:05",
	}
	for _, layout := range layouts {
		if parsed, err := time.ParseInLocation(layout, trimmed, time.Local); err == nil {
			return &parsed
		}
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, trimmed); err == nil {
			resolved := parsed.In(time.Local)
			return &resolved
		}
	}
	return nil
}

func (s *Service) refreshJobInstance(ctx context.Context, instance *JobInstance) (*JobInstance, error) {
	if instance == nil {
		return nil, nil
	}
	if submitSpecExecutionMode(instance.SubmitSpec) == "local" {
		refreshed, err := s.refreshLocalJob(ctx, instance)
		if err != nil {
			return nil, err
		}
		if err := s.syncPreviewSessionTerminalState(ctx, refreshed); err != nil {
			return nil, err
		}
		return refreshed, nil
	}
	if s.engineClient == nil || strings.TrimSpace(instance.EngineJobID) == "" {
		return instance, nil
	}
	if instance.Status == JobStatusSuccess || instance.Status == JobStatusFailed || instance.Status == JobStatusCanceled {
		jobStatus := strings.ToUpper(strings.TrimSpace(stringValue(instance.ResultPreview, "job_status")))
		if jobStatus == "" || isFinalEngineJobStatus(jobStatus) {
			return instance, nil
		}
	}
	endpoint := endpointFromSubmitSpec(instance.SubmitSpec)
	if endpoint == nil {
		return instance, nil
	}
	info, err := s.engineClient.GetJobInfo(ctx, endpoint, instance.EngineJobID)
	if err != nil {
		return instance, nil
	}
	if info == nil {
		return instance, nil
	}
	previousStatus := instance.Status
	observedStatus := normalizeJobStatus(info.JobStatus)
	if previousStatus == JobStatusCancelRequested || previousStatus == JobStatusCancelling {
		if observedStatus == JobStatusRunning {
			observedStatus = previousStatus
		} else if observedStatus == JobStatusSuccess {
			observedStatus = JobStatusCanceled
		}
	}
	instance.Status = observedStatus
	instance.ResultPreview = mergeJobRuntimeInfo(instance.ResultPreview, info)
	if isFinalNormalizedJobStatus(instance.Status) {
		if parsedFinishedAt := parseEngineJobTime(info.FinishedTime); parsedFinishedAt != nil {
			instance.FinishedAt = parsedFinishedAt
		} else if instance.FinishedAt == nil {
			now := time.Now()
			instance.FinishedAt = &now
		}
		if info.ErrorMsg != nil {
			instance.ErrorMessage = strings.TrimSpace(fmt.Sprintf("%v", info.ErrorMsg))
		}
	} else {
		instance.FinishedAt = nil
	}
	if err := s.repo.UpdateJobStatus(ctx, instance.ID, []JobStatus{previousStatus}, map[string]any{
		"status":         instance.Status,
		"result_preview": instance.ResultPreview,
		"error_message":  instance.ErrorMessage,
		"finished_at":    instance.FinishedAt,
	}); err != nil {
		if errors.Is(err, ErrJobStatusChanged) {
			return s.repo.GetJobInstanceByID(ctx, instance.ID)
		}
		return nil, err
	}
	if err := s.syncPreviewSessionTerminalState(ctx, instance); err != nil {
		return nil, err
	}
	if err := s.syncExecutionFromJob(ctx, instance); err != nil {
		return nil, err
	}
	return instance, nil
}

func (s *Service) syncPreviewSessionTerminalState(ctx context.Context, instance *JobInstance) error {
	if s == nil || s.repo == nil || instance == nil || instance.RunType != RunTypePreview || !isFinalNormalizedJobStatus(instance.Status) {
		return nil
	}
	session, err := s.repo.GetPreviewSessionByJobInstanceID(ctx, instance.ID)
	if err != nil {
		if errors.Is(err, ErrPreviewSessionNotFound) {
			return nil
		}
		return err
	}
	if session == nil {
		return nil
	}
	changed := false
	if strings.TrimSpace(session.Status) == "" || strings.EqualFold(strings.TrimSpace(session.Status), "collecting") {
		session.Status = string(instance.Status)
		changed = true
	}
	if session.FinishedAt == nil && instance.FinishedAt != nil {
		finished := *instance.FinishedAt
		session.FinishedAt = &finished
		changed = true
	}
	if !changed {
		return nil
	}
	return s.repo.UpdatePreviewSession(ctx, session)
}

func isFinalNormalizedJobStatus(status JobStatus) bool {
	return status == JobStatusSuccess || status == JobStatusFailed || status == JobStatusCanceled
}

func isFinalEngineJobStatus(status string) bool {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "FINISHED", "SUCCESS", "FAILED", "FAILING", "CANCELED", "CANCELLED", "SAVEPOINT_DONE":
		return true
	default:
		return false
	}
}

func stringValue(m map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value, ok := m[key]; ok {
			if str, ok := value.(string); ok {
				return str
			}
		}
	}
	return ""
}

func mapValue(m map[string]interface{}, keys ...string) map[string]interface{} {
	for _, key := range keys {
		if value, ok := m[key]; ok {
			if obj, ok := value.(map[string]interface{}); ok {
				return obj
			}
		}
	}
	return nil
}

func jsonMarshal(v interface{}) ([]byte, error) { return json.Marshal(v) }

func stringSliceValue(m map[string]interface{}, key string) []string {
	value, ok := m[key]
	if !ok {
		return nil
	}
	items, ok := value.([]interface{})
	if !ok {
		return nil
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		if str, ok := item.(string); ok && strings.TrimSpace(str) != "" {
			result = append(result, strings.TrimSpace(str))
		}
	}
	return result
}

func intValue(m map[string]interface{}, keys ...string) (int, bool) {
	for _, key := range keys {
		value, ok := m[key]
		if !ok {
			continue
		}
		switch v := value.(type) {
		case int:
			return v, true
		case float64:
			return int(v), true
		}
	}
	return 0, false
}

func normalizePreviewOutputFormat(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "hocon":
		return "hocon"
	case "json":
		return "json"
	case "":
		return "hocon"
	default:
		return "hocon"
	}
}

func (s *Service) resolveTaskContent(ctx context.Context, task *Task, runtime *taskVariableRuntime) (string, error) {
	if task == nil {
		return "", ErrTaskDefinitionEmpty
	}
	content := task.Content
	if strings.TrimSpace(content) == "" {
		return "", nil
	}
	variables, err := s.resolveTaskVariables(ctx, task)
	if err != nil {
		return "", err
	}
	return replaceTemplateVariables(content, variables, runtime), nil
}

func buildTaskVariableRuntime(task *Task, platformJobID string) *taskVariableRuntime {
	if task == nil {
		return &taskVariableRuntime{ReferenceTime: time.Now()}
	}
	return &taskVariableRuntime{
		ReferenceTime:          time.Now(),
		PlatformJobID:          strings.TrimSpace(platformJobID),
		TaskDefinitionName:     strings.TrimSpace(task.Name),
		TaskDefinitionCode:     strconv.FormatUint(uint64(task.ID), 10),
		WorkflowInstanceID:     strings.TrimSpace(platformJobID),
		WorkflowDefinitionName: strings.TrimSpace(task.Name),
		WorkflowDefinitionCode: strconv.FormatUint(uint64(task.ID), 10),
		ProjectName:            "STX",
		ProjectCode:            "stx",
		TaskExecutePath:        strings.TrimSpace(stringValue(task.Definition, "file_path", "config_file_path")),
	}
}

func (s *Service) resolveTaskVariables(ctx context.Context, task *Task) (map[string]string, error) {
	result := map[string]string{}
	items, err := s.repo.ListGlobalVariables(ctx)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if item != nil && strings.TrimSpace(item.Key) != "" {
			result[strings.TrimSpace(item.Key)] = item.Value
		}
	}
	for key, value := range extractDefinitionVariables(task.Definition, "custom_variables") {
		result[key] = value
	}
	if err := validateVariableKeyConflicts(result); err != nil {
		return nil, err
	}
	return result, nil
}

func extractDefinitionVariables(definition JSONMap, keys ...string) map[string]string {
	result := map[string]string{}
	for _, key := range keys {
		value := definition[key]
		mapped, ok := value.(map[string]interface{})
		if !ok {
			continue
		}
		for rawKey, rawValue := range mapped {
			name := strings.TrimSpace(rawKey)
			if name == "" {
				continue
			}
			result[name] = strings.TrimSpace(fmt.Sprint(rawValue))
		}
	}
	return result
}

func (s *Service) ensureRootFilesNested(ctx context.Context) error {
	tasks, err := s.repo.ListAllTasks(ctx)
	if err != nil {
		return err
	}
	rootFiles := make([]*Task, 0)
	var targetFolder *Task
	for _, task := range tasks {
		if task == nil {
			continue
		}
		if task.ParentID == nil && task.NodeType == TaskNodeTypeFolder && targetFolder == nil {
			targetFolder = task
		}
		if task.ParentID == nil && task.NodeType == TaskNodeTypeFile {
			rootFiles = append(rootFiles, task)
		}
	}
	if len(rootFiles) == 0 {
		return nil
	}
	return s.repo.Transaction(ctx, func(tx *Repository) error {
		folder := targetFolder
		if folder == nil {
			folder = &Task{
				NodeType:      TaskNodeTypeFolder,
				Name:          "workspace",
				ContentFormat: ContentFormatHOCON,
				Status:        TaskStatusDraft,
			}
			if err := tx.CreateTask(ctx, folder); err != nil {
				return err
			}
		}
		for _, file := range rootFiles {
			file.ParentID = &folder.ID
			if err := tx.UpdateTask(ctx, file); err != nil {
				return err
			}
		}
		return nil
	})
}

func normalizeGlobalVariableKey(raw string) (string, error) {
	key := strings.TrimSpace(raw)
	if key == "" {
		return "", ErrGlobalVariableKeyRequired
	}
	if !safeTaskNamePattern.MatchString(key) {
		return "", ErrGlobalVariableKeyInvalid
	}
	return key, nil
}

// normalizeGlobalVariableType normalizes and validates global variable value type.
// normalizeGlobalVariableType 归一化并校验全局变量类型。
func normalizeGlobalVariableType(raw string) string {
	t := strings.ToLower(strings.TrimSpace(raw))
	if t == GlobalVariableTypeSecret || t == "password" {
		return GlobalVariableTypeSecret
	}
	return GlobalVariableTypeString
}

func defaultString(current string, fallback string) string {
	if strings.TrimSpace(current) != "" {
		return current
	}
	return fallback
}

func (s *Service) ensureSiblingTaskNameAvailable(ctx context.Context, parentID *uint, name string, excludeID *uint) error {
	exists, err := s.repo.ExistsSiblingTaskName(ctx, parentID, name, excludeID)
	if err != nil {
		return err
	}
	if exists {
		return ErrTaskNameDuplicate
	}
	return nil
}
