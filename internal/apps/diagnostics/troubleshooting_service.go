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

package diagnostics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// TroubleshootingService 负责排障经验记忆库的业务逻辑与数据转换
// TroubleshootingService manages business logic and model translation for troubleshooting memories
type TroubleshootingService struct {
	repo *TroubleshootingRepository
}

// NewTroubleshootingService 创建排障经验服务实例
// NewTroubleshootingService creates a new TroubleshootingService instance
func NewTroubleshootingService(repo *TroubleshootingRepository) *TroubleshootingService {
	return &TroubleshootingService{repo: repo}
}

// ToItem 将数据库实体转换为前端友好的视图模型
// ToItem converts database model to client-facing DTO item
func (s *TroubleshootingService) ToItem(m *TroubleshootingMemory) *TroubleshootingMemoryItem {
	if m == nil {
		return nil
	}

	var actions []string
	if m.ActionsTaken != "" {
		_ = json.Unmarshal([]byte(m.ActionsTaken), &actions)
	}

	var tags []string
	if m.Tags != "" {
		if strings.HasPrefix(m.Tags, "[") {
			_ = json.Unmarshal([]byte(m.Tags), &tags)
		} else {
			for _, part := range strings.Split(m.Tags, ",") {
				if t := strings.TrimSpace(part); t != "" {
					tags = append(tags, t)
				}
			}
		}
	}
	if tags == nil {
		tags = []string{}
	}

	return &TroubleshootingMemoryItem{
		ID:             strconv.FormatUint(uint64(m.ID), 10),
		TargetType:     m.TargetType,
		Fingerprint:    m.Fingerprint,
		PresetKey:      m.PresetKey,
		Language:       m.Language,
		Title:          m.Title,
		ErrorSummary:   m.ErrorSummary,
		RootCause:      m.RootCause,
		Solution:       m.Solution,
		ActionsTaken:   actions,
		PreventiveTips: m.PreventiveTips,
		ClusterID:      m.ClusterID,
		ClusterName:    m.ClusterName,
		Tags:           tags,
		Author:         m.Author,
		IsPreset:       m.IsPreset,
		CreatedAt:      m.CreatedAt,
		UpdatedAt:      m.UpdatedAt,
	}
}

// ListMemories 查询排障经验列表
// ListMemories queries troubleshooting memories matching given filter
func (s *TroubleshootingService) ListMemories(ctx context.Context, query *TroubleshootingMemoryQuery) ([]*TroubleshootingMemoryItem, int64, error) {
	// 尝试自动播种经典官方案例
	// Attempt seeding preset cases if database table is currently empty
	_ = s.repo.SeedPresetMemories(ctx)

	records, total, err := s.repo.List(ctx, query)
	if err != nil {
		return nil, 0, err
	}

	items := make([]*TroubleshootingMemoryItem, 0, len(records))
	for _, r := range records {
		items = append(items, s.ToItem(r))
	}

	return items, total, nil
}

// GetMemory 根据 ID 获取单条排障经验
// GetMemory retrieves single troubleshooting memory by ID
func (s *TroubleshootingService) GetMemory(ctx context.Context, id uint) (*TroubleshootingMemoryItem, error) {
	m, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return s.ToItem(m), nil
}

// CreateMemory 新增排障经验（强制校验解决方案非空）
// CreateMemory inserts new troubleshooting memory (enforces non-empty solution)
func (s *TroubleshootingService) CreateMemory(ctx context.Context, req *CreateTroubleshootingMemoryRequest) (*TroubleshootingMemoryItem, error) {
	if req == nil {
		return nil, errors.New("request payload is nil / 请求体不能为空")
	}

	solution := strings.TrimSpace(req.Solution)
	if solution == "" {
		return nil, errors.New("解决方案内容不能为空，排障经验必须包含具体的处理措施与步骤 / solution cannot be empty")
	}

	title := strings.TrimSpace(req.Title)
	if title == "" {
		return nil, errors.New("方案标题不能为空 / title cannot be empty")
	}

	fingerprint := strings.TrimSpace(req.Fingerprint)
	if fingerprint == "" {
		fingerprint = title
	}

	targetType := strings.TrimSpace(req.TargetType)
	if targetType != "alert" {
		targetType = "error"
	}

	lang := strings.TrimSpace(req.Language)
	if lang == "" {
		lang = "zh"
	}

	actionsJSON := ""
	if len(req.ActionsTaken) > 0 {
		b, _ := json.Marshal(req.ActionsTaken)
		actionsJSON = string(b)
	}

	tagsJSON := "[]"
	if len(req.Tags) > 0 {
		b, _ := json.Marshal(req.Tags)
		tagsJSON = string(b)
	}

	author := strings.TrimSpace(req.Author)
	if author == "" {
		author = "运维工程师"
	}

	var clusterID uint
	if req.ClusterID != nil {
		clusterID = *req.ClusterID
	}

	record := &TroubleshootingMemory{
		TargetType:     targetType,
		Fingerprint:    fingerprint,
		Language:       lang,
		Title:          title,
		ErrorSummary:   strings.TrimSpace(req.ErrorSummary),
		RootCause:      strings.TrimSpace(req.RootCause),
		Solution:       solution,
		ActionsTaken:   actionsJSON,
		PreventiveTips: strings.TrimSpace(req.PreventiveTips),
		ClusterID:      clusterID,
		ClusterName:    strings.TrimSpace(req.ClusterName),
		Tags:           tagsJSON,
		Author:         author,
		IsPreset:       false,
	}

	if err := s.repo.Create(ctx, record); err != nil {
		return nil, fmt.Errorf("failed to create troubleshooting memory: %w", err)
	}

	return s.ToItem(record), nil
}

// UpdateMemory 更新指定的排障经验记录
// UpdateMemory modifies an existing troubleshooting memory entry
func (s *TroubleshootingService) UpdateMemory(ctx context.Context, id uint, req *UpdateTroubleshootingMemoryRequest) (*TroubleshootingMemoryItem, error) {
	if req == nil {
		return nil, errors.New("request payload is nil / 请求体不能为空")
	}

	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("troubleshooting memory not found: %w", err)
	}

	if req.Title != "" {
		existing.Title = strings.TrimSpace(req.Title)
	}
	if req.Solution != "" {
		existing.Solution = strings.TrimSpace(req.Solution)
	}
	if req.ErrorSummary != "" {
		existing.ErrorSummary = strings.TrimSpace(req.ErrorSummary)
	}
	if req.RootCause != "" {
		existing.RootCause = strings.TrimSpace(req.RootCause)
	}
	if req.PreventiveTips != "" {
		existing.PreventiveTips = strings.TrimSpace(req.PreventiveTips)
	}
	if req.Author != "" {
		existing.Author = strings.TrimSpace(req.Author)
	}
	if req.ActionsTaken != nil {
		b, _ := json.Marshal(req.ActionsTaken)
		existing.ActionsTaken = string(b)
	}
	if req.Tags != nil {
		b, _ := json.Marshal(req.Tags)
		existing.Tags = string(b)
	}

	if err := s.repo.Update(ctx, id, existing); err != nil {
		return nil, fmt.Errorf("failed to update troubleshooting memory: %w", err)
	}

	return s.ToItem(existing), nil
}

// DeleteMemory 删除指定的排障经验记录
// DeleteMemory removes a troubleshooting memory entry by ID
func (s *TroubleshootingService) DeleteMemory(ctx context.Context, id uint) error {
	return s.repo.Delete(ctx, id)
}
