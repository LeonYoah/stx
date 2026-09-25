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
	"fmt"
	"strings"

	"github.com/LeonYoah/stx/internal/db"
)

// CreateCuratedTemplateRequest creates one user curated template.
// CreateCuratedTemplateRequest 创建一条用户精选模板。
type CreateCuratedTemplateRequest struct {
	Name        string   `json:"name" binding:"required"`
	Description string   `json:"description"`
	Section     string   `json:"section"`
	Mode        string   `json:"mode"`
	Pattern     string   `json:"pattern"`
	Connectors  []string `json:"connectors"`
	Content     string   `json:"content" binding:"required"`
	BuiltinID   string   `json:"builtin_id"`
}

// UpdateCuratedTemplateRequest updates one user curated template.
// UpdateCuratedTemplateRequest 更新一条用户精选模板。
type UpdateCuratedTemplateRequest struct {
	Name        *string  `json:"name"`
	Description *string  `json:"description"`
	Section     *string  `json:"section"`
	Mode        *string  `json:"mode"`
	Pattern     *string  `json:"pattern"`
	Connectors  []string `json:"connectors"`
	Content     *string  `json:"content"`
	Enabled     *bool    `json:"enabled"`
}

// ForkCuratedTemplateRequest forks a built-in curated template.
// ForkCuratedTemplateRequest 从内置精选 fork 用户副本。
type ForkCuratedTemplateRequest struct {
	BuiltinID   string `json:"builtin_id" binding:"required"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// ListCuratedTemplatesRequest filters curated templates.
// ListCuratedTemplatesRequest 筛选精选模板列表。
type ListCuratedTemplatesRequest struct {
	Section string `json:"section"`
	Mode    string `json:"mode"`
	Q       string `json:"q"`
	Origin  string `json:"origin"` // builtin|user|all
}

// ParseCuratedComboRequest parses job content into four sections for combo save.
// ParseCuratedComboRequest 将作业正文解析为四节，供一键组合另存。
type ParseCuratedComboRequest struct {
	Content string `json:"content" binding:"required"`
}

// CuratedTemplateView is the API list/detail view for curated templates.
// CuratedTemplateView 是精选模板的列表/详情视图。
type CuratedTemplateView struct {
	ID          uint     `json:"id,omitempty"`
	BuiltinID   string   `json:"builtin_id,omitempty"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Section     string   `json:"section"`
	Mode        string   `json:"mode"`
	Pattern     string   `json:"pattern"`
	Connectors  []string `json:"connectors"`
	Content     string   `json:"content"`
	Enabled     bool     `json:"enabled"`
	Origin      string   `json:"origin"` // builtin|user|override
	HasOverride bool     `json:"has_override"`
	OwnerUserID uint     `json:"owner_user_id,omitempty"`
}

// CuratedTemplateListData wraps list payload.
// CuratedTemplateListData 包装列表载荷。
type CuratedTemplateListData struct {
	Items []*CuratedTemplateView `json:"items"`
	Total int                    `json:"total"`
}

// CuratedComboParseData is the four-section parse result.
// CuratedComboParseData 是四节解析结果。
type CuratedComboParseData struct {
	Env       string   `json:"env"`
	Source    string   `json:"source"`
	Transform string   `json:"transform"`
	Sink      string   `json:"sink"`
	Present   []string `json:"present"`
	Combined  string   `json:"combined"`
}

// CuratedTemplateListResponse is the HTTP list envelope.
type CuratedTemplateListResponse struct {
	ErrorMsg string                   `json:"error_msg"`
	Data     *CuratedTemplateListData `json:"data"`
}

// CuratedTemplateResponse is the HTTP item envelope.
type CuratedTemplateResponse struct {
	ErrorMsg string               `json:"error_msg"`
	Data     *CuratedTemplateView `json:"data"`
}

// CuratedComboParseResponse is the HTTP parse envelope.
type CuratedComboParseResponse struct {
	ErrorMsg string                 `json:"error_msg"`
	Data     *CuratedComboParseData `json:"data"`
}

// ListCuratedTemplates merges built-in seeds with the current user's DB rows.
// ListCuratedTemplates 合并内置种子与当前用户的数据库行。
func (s *Service) ListCuratedTemplates(ctx context.Context, req *ListCuratedTemplatesRequest, userID uint) (*CuratedTemplateListData, error) {
	if req == nil {
		req = &ListCuratedTemplatesRequest{}
	}
	origin := strings.ToLower(strings.TrimSpace(req.Origin))
	if origin == "" {
		origin = "all"
	}
	seeds, err := loadCuratedSeedTemplates()
	if err != nil {
		return nil, err
	}
	userRows, err := s.repo.ListCuratedTemplatesByOwner(ctx, userID)
	if err != nil {
		return nil, err
	}
	overrideByBuiltin := map[string]*CuratedTemplate{}
	userViews := make([]*CuratedTemplateView, 0)
	for _, row := range userRows {
		view := curatedRowToView(row)
		if row.BuiltinID != "" {
			overrideByBuiltin[row.BuiltinID] = row
			view.Origin = CuratedOriginOverride
			view.HasOverride = true
		} else {
			view.Origin = CuratedOriginUser
		}
		if origin == "all" || origin == "user" || (origin == "override" && row.BuiltinID != "") {
			if curatedViewMatches(view, req) {
				userViews = append(userViews, view)
			}
		}
	}

	items := make([]*CuratedTemplateView, 0, len(seeds)+len(userViews))
	if origin == "all" || origin == "builtin" {
		for _, seed := range seeds {
			view := curatedSeedToView(seed)
			if override, ok := overrideByBuiltin[seed.ID]; ok {
				view.HasOverride = true
				view.ID = override.ID
			}
			if curatedViewMatches(view, req) {
				items = append(items, view)
			}
		}
	}
	if origin == "all" || origin == "user" || origin == "override" {
		items = append(items, userViews...)
	}
	return &CuratedTemplateListData{Items: items, Total: len(items)}, nil
}

// CreateCuratedTemplate creates one user template (save-as / custom).
// CreateCuratedTemplate 创建用户精选（另存/自定义）。
func (s *Service) CreateCuratedTemplate(ctx context.Context, req *CreateCuratedTemplateRequest, userID uint) (*CuratedTemplateView, error) {
	if req == nil {
		return nil, fmt.Errorf("sync: curated create request is required")
	}
	name := strings.TrimSpace(req.Name)
	content := strings.TrimSpace(req.Content)
	if name == "" {
		return nil, ErrCuratedTemplateNameRequired
	}
	if content == "" {
		return nil, ErrCuratedTemplateContentRequired
	}
	section := normalizeCuratedSection(req.Section)
	if section == "" {
		section = inferCuratedSection(content)
	}
	if !isValidCuratedSection(section) {
		return nil, ErrCuratedTemplateSectionInvalid
	}
	item := &CuratedTemplate{
		BuiltinID:   strings.TrimSpace(req.BuiltinID),
		OwnerUserID: userID,
		Name:        name,
		Description: strings.TrimSpace(req.Description),
		Section:     section,
		Mode:        normalizeCuratedMode(req.Mode),
		Pattern:     normalizeCuratedPattern(req.Pattern),
		Connectors:  JSONStringSlice(req.Connectors),
		Content:     db.ScriptText(content),
		Enabled:     true,
	}
	if err := s.repo.CreateCuratedTemplate(ctx, item); err != nil {
		return nil, err
	}
	view := curatedRowToView(item)
	if item.BuiltinID != "" {
		view.Origin = CuratedOriginOverride
		view.HasOverride = true
	} else {
		view.Origin = CuratedOriginUser
	}
	return view, nil
}

// UpdateCuratedTemplate updates one owned curated template.
// UpdateCuratedTemplate 更新用户拥有的精选模板。
func (s *Service) UpdateCuratedTemplate(ctx context.Context, id uint, req *UpdateCuratedTemplateRequest, userID uint) (*CuratedTemplateView, error) {
	item, err := s.repo.GetCuratedTemplateByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if item.OwnerUserID != userID {
		return nil, ErrCuratedTemplatePermissionDenied
	}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			return nil, ErrCuratedTemplateNameRequired
		}
		item.Name = name
	}
	if req.Description != nil {
		item.Description = strings.TrimSpace(*req.Description)
	}
	if req.Section != nil {
		section := normalizeCuratedSection(*req.Section)
		if !isValidCuratedSection(section) {
			return nil, ErrCuratedTemplateSectionInvalid
		}
		item.Section = section
	}
	if req.Mode != nil {
		item.Mode = normalizeCuratedMode(*req.Mode)
	}
	if req.Pattern != nil {
		item.Pattern = normalizeCuratedPattern(*req.Pattern)
	}
	if req.Connectors != nil {
		item.Connectors = JSONStringSlice(req.Connectors)
	}
	if req.Content != nil {
		content := strings.TrimSpace(*req.Content)
		if content == "" {
			return nil, ErrCuratedTemplateContentRequired
		}
		item.Content = db.ScriptText(content)
	}
	if req.Enabled != nil {
		item.Enabled = *req.Enabled
	}
	if err := s.repo.UpdateCuratedTemplate(ctx, item); err != nil {
		return nil, err
	}
	view := curatedRowToView(item)
	if item.BuiltinID != "" {
		view.Origin = CuratedOriginOverride
		view.HasOverride = true
	} else {
		view.Origin = CuratedOriginUser
	}
	return view, nil
}

// DeleteCuratedTemplate deletes one owned curated template.
// DeleteCuratedTemplate 删除用户拥有的精选模板。
func (s *Service) DeleteCuratedTemplate(ctx context.Context, id uint, userID uint) error {
	item, err := s.repo.GetCuratedTemplateByID(ctx, id)
	if err != nil {
		return err
	}
	if item.OwnerUserID != userID {
		return ErrCuratedTemplatePermissionDenied
	}
	return s.repo.DeleteCuratedTemplate(ctx, id)
}

// ForkCuratedTemplate copies a built-in seed into the user's override row.
// ForkCuratedTemplate 将内置种子复制为用户的 override 行。
func (s *Service) ForkCuratedTemplate(ctx context.Context, req *ForkCuratedTemplateRequest, userID uint) (*CuratedTemplateView, error) {
	if req == nil || strings.TrimSpace(req.BuiltinID) == "" {
		return nil, ErrCuratedBuiltinIDRequired
	}
	builtinID := strings.TrimSpace(req.BuiltinID)
	seed, ok := findCuratedSeedByID(builtinID)
	if !ok {
		return nil, ErrCuratedTemplateNotFound
	}
	existing, err := s.repo.GetCuratedTemplateByOwnerAndBuiltin(ctx, userID, builtinID)
	if err == nil && existing != nil {
		view := curatedRowToView(existing)
		view.Origin = CuratedOriginOverride
		view.HasOverride = true
		return view, nil
	}
	if err != nil && err != ErrCuratedTemplateNotFound {
		return nil, err
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = seed.Name
	}
	desc := strings.TrimSpace(req.Description)
	if desc == "" {
		desc = seed.Description
	}
	return s.CreateCuratedTemplate(ctx, &CreateCuratedTemplateRequest{
		Name:        name,
		Description: desc,
		Section:     seed.Section,
		Mode:        seed.Mode,
		Pattern:     seed.Pattern,
		Connectors:  append([]string{}, seed.Connectors...),
		Content:     seed.Content,
		BuiltinID:   seed.ID,
	}, userID)
}

// GetCuratedTemplateContentForInsert returns content, rewriting plugin IO keys for legacy clusters.
// GetCuratedTemplateContentForInsert 返回待插入正文，并按低版本集群改写 plugin IO 键。
func (s *Service) GetCuratedTemplateContentForInsert(ctx context.Context, builtinID string, userID uint, dbID uint, clusterID uint) (string, error) {
	var content string
	if dbID > 0 {
		row, err := s.repo.GetCuratedTemplateByID(ctx, dbID)
		if err != nil {
			return "", err
		}
		if row.OwnerUserID != userID {
			return "", ErrCuratedTemplatePermissionDenied
		}
		content = row.Content.String()
	} else if strings.TrimSpace(builtinID) != "" {
		seed, ok := findCuratedSeedByID(strings.TrimSpace(builtinID))
		if !ok {
			return "", ErrCuratedTemplateNotFound
		}
		content = seed.Content
	} else {
		return "", ErrCuratedTemplateNotFound
	}
	legacy := s.usesLegacyPluginIOKeys(ctx, clusterID)
	return rewritePluginIOKeysForLegacy(content, legacy), nil
}

// ParseCuratedCombo extracts env/source/transform/sink from full job content.
// ParseCuratedCombo 从完整作业正文提取 env/source/transform/sink 四节。
func (s *Service) ParseCuratedCombo(_ context.Context, req *ParseCuratedComboRequest) (*CuratedComboParseData, error) {
	if req == nil || strings.TrimSpace(req.Content) == "" {
		return nil, ErrCuratedTemplateContentRequired
	}
	return parseFourSections(req.Content), nil
}

func curatedViewMatches(view *CuratedTemplateView, req *ListCuratedTemplatesRequest) bool {
	if view == nil || req == nil {
		return true
	}
	if section := normalizeCuratedSection(req.Section); section != "" && view.Section != section {
		return false
	}
	if mode := normalizeCuratedMode(req.Mode); mode != "" && mode != CuratedModeAny {
		if view.Mode != CuratedModeAny && !strings.EqualFold(view.Mode, mode) {
			return false
		}
	}
	q := strings.ToLower(strings.TrimSpace(req.Q))
	if q == "" {
		return true
	}
	blob := strings.ToLower(strings.Join([]string{view.Name, view.Description, view.BuiltinID, strings.Join(view.Connectors, " ")}, " "))
	return strings.Contains(blob, q)
}

func curatedRowToView(row *CuratedTemplate) *CuratedTemplateView {
	if row == nil {
		return nil
	}
	return &CuratedTemplateView{
		ID:          row.ID,
		BuiltinID:   row.BuiltinID,
		Name:        row.Name,
		Description: row.Description,
		Section:     row.Section,
		Mode:        row.Mode,
		Pattern:     row.Pattern,
		Connectors:  append([]string{}, row.Connectors...),
		Content:     row.Content.String(),
		Enabled:     row.Enabled,
		OwnerUserID: row.OwnerUserID,
	}
}

func curatedSeedToView(seed curatedSeedTemplate) *CuratedTemplateView {
	return &CuratedTemplateView{
		BuiltinID:   seed.ID,
		Name:        seed.Name,
		Description: seed.Description,
		Section:     seed.Section,
		Mode:        seed.Mode,
		Pattern:     seed.Pattern,
		Connectors:  append([]string{}, seed.Connectors...),
		Content:     seed.Content,
		Enabled:     seed.Enabled,
		Origin:      CuratedOriginBuiltin,
	}
}

func normalizeCuratedSection(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case CuratedSectionEnv:
		return CuratedSectionEnv
	case CuratedSectionSource:
		return CuratedSectionSource
	case CuratedSectionTransform:
		return CuratedSectionTransform
	case CuratedSectionSink:
		return CuratedSectionSink
	case CuratedSectionCombo:
		return CuratedSectionCombo
	default:
		return ""
	}
}

func isValidCuratedSection(section string) bool {
	return normalizeCuratedSection(section) != ""
}

func normalizeCuratedMode(raw string) string {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case CuratedModeBatch:
		return CuratedModeBatch
	case CuratedModeStreaming:
		return CuratedModeStreaming
	case CuratedModeAny, "":
		return CuratedModeAny
	default:
		return CuratedModeAny
	}
}

func normalizeCuratedPattern(raw string) string {
	v := strings.ToLower(strings.TrimSpace(raw))
	switch v {
	case "single", "multi", "cdc", "other":
		return v
	case "":
		return "other"
	default:
		return v
	}
}

func inferCuratedSection(content string) string {
	parsed := parseFourSections(content)
	count := len(parsed.Present)
	if count == 0 {
		return CuratedSectionCombo
	}
	if count >= 2 {
		return CuratedSectionCombo
	}
	return parsed.Present[0]
}

// parseFourSections extracts top-level env/source/transform/sink blocks (brace-balanced).
// parseFourSections 提取顶层 env/source/transform/sink 块（花括号平衡）。
func parseFourSections(content string) *CuratedComboParseData {
	out := &CuratedComboParseData{Present: make([]string, 0, 4)}
	sections := []string{CuratedSectionEnv, CuratedSectionSource, CuratedSectionTransform, CuratedSectionSink}
	for _, name := range sections {
		block := extractTopLevelSection(content, name)
		if block == "" {
			continue
		}
		switch name {
		case CuratedSectionEnv:
			out.Env = block
		case CuratedSectionSource:
			out.Source = block
		case CuratedSectionTransform:
			out.Transform = block
		case CuratedSectionSink:
			out.Sink = block
		}
		out.Present = append(out.Present, name)
	}
	parts := make([]string, 0, 4)
	for _, name := range out.Present {
		switch name {
		case CuratedSectionEnv:
			parts = append(parts, strings.TrimSpace(out.Env))
		case CuratedSectionSource:
			parts = append(parts, strings.TrimSpace(out.Source))
		case CuratedSectionTransform:
			parts = append(parts, strings.TrimSpace(out.Transform))
		case CuratedSectionSink:
			parts = append(parts, strings.TrimSpace(out.Sink))
		}
	}
	out.Combined = strings.Join(parts, "\n\n")
	return out
}

func extractTopLevelSection(content, section string) string {
	lower := strings.ToLower(content)
	key := strings.ToLower(section)
	idx := 0
	for {
		pos := strings.Index(lower[idx:], key)
		if pos < 0 {
			return ""
		}
		abs := idx + pos
		if abs > 0 {
			prev := content[abs-1]
			if (prev >= 'a' && prev <= 'z') || (prev >= 'A' && prev <= 'Z') || (prev >= '0' && prev <= '9') || prev == '_' {
				idx = abs + len(key)
				continue
			}
		}
		rest := strings.TrimLeft(content[abs+len(key):], " \t\r\n")
		if !strings.HasPrefix(rest, "{") {
			idx = abs + len(key)
			continue
		}
		braceStart := abs + len(key) + strings.Index(content[abs+len(key):], "{")
		end := findMatchingBrace(content, braceStart)
		if end < 0 {
			return ""
		}
		return strings.TrimSpace(content[abs : end+1])
	}
}

func findMatchingBrace(content string, openIdx int) int {
	if openIdx < 0 || openIdx >= len(content) || content[openIdx] != '{' {
		return -1
	}
	depth := 0
	inSingle := false
	inDouble := false
	escape := false
	for i := openIdx; i < len(content); i++ {
		ch := content[i]
		if escape {
			escape = false
			continue
		}
		if ch == '\\' && (inSingle || inDouble) {
			escape = true
			continue
		}
		if !inDouble && ch == '\'' {
			inSingle = !inSingle
			continue
		}
		if !inSingle && ch == '"' {
			inDouble = !inDouble
			continue
		}
		if inSingle || inDouble {
			continue
		}
		if ch == '{' {
			depth++
		} else if ch == '}' {
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}
