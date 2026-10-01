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
	"sort"
	"strings"
	"sync"
)

type SyncPluginType string

const (
	SyncPluginTypeSource    SyncPluginType = "source"
	SyncPluginTypeTransform SyncPluginType = "transform"
	SyncPluginTypeSink      SyncPluginType = "sink"
	SyncPluginTypeCatalog   SyncPluginType = "catalog"
)

type SyncPluginFactoryInfo struct {
	FactoryIdentifier string `json:"factory_identifier"`
	ClassName         string `json:"class_name,omitempty"`
	Origin            string `json:"origin,omitempty"`
}

type SyncPluginOptionDescriptor struct {
	Key                 string      `json:"key"`
	Type                string      `json:"type,omitempty"`
	ElementType         string      `json:"element_type,omitempty"`
	DefaultValue        interface{} `json:"default_value,omitempty"`
	Description         string      `json:"description,omitempty"`
	FallbackKeys        []string    `json:"fallback_keys,omitempty"`
	EnumValues          []string    `json:"enum_values,omitempty"`
	EnumDisplayValues   []string    `json:"enum_display_values,omitempty"`
	RequiredMode        string      `json:"required_mode,omitempty"`
	ConditionExpression string      `json:"condition_expression,omitempty"`
	ConstraintGroup     string      `json:"constraint_group,omitempty"`
	Origins             []string    `json:"origins,omitempty"`
	DeclaredClasses     []string    `json:"declared_classes,omitempty"`
	Advanced            bool        `json:"advanced"`
}

type SyncPluginFactoryListResult struct {
	PluginType string                  `json:"plugin_type"`
	Plugins    []SyncPluginFactoryInfo `json:"plugins"`
	Warnings   []string                `json:"warnings,omitempty"`
}

type SyncPluginOptionSchemaResult struct {
	PluginType        string                       `json:"plugin_type"`
	FactoryIdentifier string                       `json:"factory_identifier"`
	Options           []SyncPluginOptionDescriptor `json:"options"`
	ValueConstraints  []map[string]interface{}     `json:"value_constraints,omitempty"`
	ConditionRules    []map[string]interface{}     `json:"condition_rules,omitempty"`
	Warnings          []string                     `json:"warnings,omitempty"`
}

type SyncPluginTemplateResult struct {
	PluginType        string   `json:"plugin_type"`
	FactoryIdentifier string   `json:"factory_identifier"`
	ContentFormat     string   `json:"content_format"`
	Template          string   `json:"template"`
	Warnings          []string `json:"warnings,omitempty"`
}

type SyncPluginEnumValuesResult struct {
	PluginType        string   `json:"plugin_type"`
	FactoryIdentifier string   `json:"factory_identifier"`
	OptionKey         string   `json:"option_key"`
	EnumValues        []string `json:"enum_values"`
	Warnings          []string `json:"warnings,omitempty"`
}

type SyncPluginEnumCatalogPlugin struct {
	PluginType        string                       `json:"plugin_type"`
	FactoryIdentifier string                       `json:"factory_identifier"`
	Options           []SyncPluginOptionDescriptor `json:"options"`
}

type SyncPluginEnumCatalogResult struct {
	EnvOptions []SyncPluginOptionDescriptor  `json:"env_options"`
	Plugins    []SyncPluginEnumCatalogPlugin `json:"plugins"`
	Warnings   []string                      `json:"warnings,omitempty"`
}

type SyncSinkSaveModePreviewAction struct {
	Phase      string `json:"phase,omitempty"`
	ActionType string `json:"action_type,omitempty"`
	ResultType string `json:"result_type,omitempty"`
	Content    string `json:"content,omitempty"`
	Native     bool   `json:"native"`
}

type SyncSinkSaveModePreviewTable struct {
	TablePath      string                          `json:"table_path,omitempty"`
	Supported      bool                            `json:"supported"`
	Completeness   string                          `json:"completeness,omitempty"`
	SchemaSaveMode string                          `json:"schema_save_mode,omitempty"`
	DataSaveMode   string                          `json:"data_save_mode,omitempty"`
	Actions        []SyncSinkSaveModePreviewAction `json:"actions,omitempty"`
	Warnings       []string                        `json:"warnings,omitempty"`
}

type SyncSinkSaveModePreviewResult struct {
	Connector      string                          `json:"connector,omitempty"`
	SinkIndex      int                             `json:"sink_index,omitempty"`
	Supported      bool                            `json:"supported"`
	Completeness   string                          `json:"completeness,omitempty"`
	SchemaSaveMode string                          `json:"schema_save_mode,omitempty"`
	DataSaveMode   string                          `json:"data_save_mode,omitempty"`
	TablePath      string                          `json:"table_path,omitempty"`
	Actions        []SyncSinkSaveModePreviewAction `json:"actions,omitempty"`
	Tables         []SyncSinkSaveModePreviewTable  `json:"tables,omitempty"`
	Warnings       []string                        `json:"warnings,omitempty"`
}

type PreviewSyncSinkSaveModeRequest struct {
	ClusterID          uint              `json:"cluster_id" binding:"required"`
	SinkIndex          *int              `json:"sink_index,omitempty"`
	SinkNodeID         string            `json:"sink_node_id,omitempty"`
	IncludeInfoPreview *bool             `json:"include_info_preview,omitempty"`
	Draft              *TaskDraftPayload `json:"draft,omitempty"`
}

type ListSyncPluginFactoriesRequest struct {
	ClusterID  uint   `json:"cluster_id" binding:"required"`
	PluginType string `json:"plugin_type" binding:"required"`
}

type GetSyncPluginOptionsRequest struct {
	ClusterID         uint   `json:"cluster_id" binding:"required"`
	PluginType        string `json:"plugin_type" binding:"required"`
	FactoryIdentifier string `json:"factory_identifier" binding:"required"`
	IncludeSupplement *bool  `json:"include_supplement,omitempty"`
}

type RenderSyncPluginTemplateRequest struct {
	ClusterID         uint   `json:"cluster_id" binding:"required"`
	PluginType        string `json:"plugin_type" binding:"required"`
	FactoryIdentifier string `json:"factory_identifier" binding:"required"`
	IncludeSupplement *bool  `json:"include_supplement,omitempty"`
	IncludeComments   *bool  `json:"include_comments,omitempty"`
	IncludeAdvanced   *bool  `json:"include_advanced,omitempty"`
}

type ListSyncPluginEnumValuesRequest struct {
	ClusterID         uint   `json:"cluster_id" binding:"required"`
	PluginType        string `json:"plugin_type" binding:"required"`
	FactoryIdentifier string `json:"factory_identifier" binding:"required"`
	OptionKey         string `json:"option_key" binding:"required"`
	IncludeSupplement *bool  `json:"include_supplement,omitempty"`
}

type ListSyncPluginEnumCatalogRequest struct {
	ClusterID         uint  `json:"cluster_id" binding:"required"`
	IncludeSupplement *bool `json:"include_supplement,omitempty"`
}

func (s *Service) ListPluginFactories(
	ctx context.Context,
	req *ListSyncPluginFactoriesRequest,
) (*SyncPluginFactoryListResult, error) {
	if req == nil {
		return nil, fmt.Errorf("sync: plugin list request is required")
	}
	endpoint, err := s.resolvePluginProxyEndpoint(ctx, req.ClusterID)
	if err != nil {
		return nil, err
	}
	result, err := s.configToolClient.ListPlugins(ctx, endpoint, &ConfigToolPluginListRequest{PluginType: normalizeSyncPluginType(req.PluginType)})
	if err != nil {
		return nil, err
	}
	items := make([]SyncPluginFactoryInfo, 0, len(result.Plugins))
	for _, item := range result.Plugins {
		items = append(items, SyncPluginFactoryInfo{FactoryIdentifier: item.FactoryIdentifier, ClassName: item.ClassName, Origin: item.Origin})
	}
	return &SyncPluginFactoryListResult{PluginType: result.PluginType, Plugins: items, Warnings: result.Warnings}, nil
}

func (s *Service) GetPluginOptions(
	ctx context.Context,
	req *GetSyncPluginOptionsRequest,
) (*SyncPluginOptionSchemaResult, error) {
	if req == nil {
		return nil, fmt.Errorf("sync: plugin options request is required")
	}
	endpoint, err := s.resolvePluginProxyEndpoint(ctx, req.ClusterID)
	if err != nil {
		return nil, err
	}
	result, err := s.configToolClient.GetPluginOptions(ctx, endpoint, &ConfigToolPluginOptionsRequest{PluginType: normalizeSyncPluginType(req.PluginType), FactoryIdentifier: strings.TrimSpace(req.FactoryIdentifier), IncludeSupplement: boolValue(req.IncludeSupplement, true)})
	if err != nil {
		return nil, err
	}
	return &SyncPluginOptionSchemaResult{PluginType: result.PluginType, FactoryIdentifier: result.FactoryIdentifier, Options: mapSyncPluginOptions(result.Options), ValueConstraints: result.ValueConstraints, ConditionRules: result.ConditionRules, Warnings: result.Warnings}, nil
}

func (s *Service) RenderPluginTemplate(
	ctx context.Context,
	req *RenderSyncPluginTemplateRequest,
) (*SyncPluginTemplateResult, error) {
	if req == nil {
		return nil, fmt.Errorf("sync: plugin template request is required")
	}
	endpoint, err := s.resolvePluginProxyEndpoint(ctx, req.ClusterID)
	if err != nil {
		return nil, err
	}
	result, err := s.configToolClient.RenderPluginTemplate(ctx, endpoint, &ConfigToolPluginTemplateRequest{PluginType: normalizeSyncPluginType(req.PluginType), FactoryIdentifier: strings.TrimSpace(req.FactoryIdentifier), IncludeSupplement: boolValue(req.IncludeSupplement, true), IncludeComments: boolValue(req.IncludeComments, true), IncludeAdvanced: boolValue(req.IncludeAdvanced, false)})
	if err != nil {
		return nil, err
	}
	result.Template = rewritePluginIOKeysForLegacy(result.Template, s.usesLegacyPluginIOKeys(ctx, req.ClusterID))
	return &SyncPluginTemplateResult{PluginType: result.PluginType, FactoryIdentifier: result.FactoryIdentifier, ContentFormat: result.ContentFormat, Template: result.Template, Warnings: result.Warnings}, nil
}

func (s *Service) ListPluginEnumValues(
	ctx context.Context,
	req *ListSyncPluginEnumValuesRequest,
) (*SyncPluginEnumValuesResult, error) {
	if req == nil {
		return nil, fmt.Errorf("sync: plugin enum values request is required")
	}
	endpoint, err := s.resolvePluginProxyEndpoint(ctx, req.ClusterID)
	if err != nil {
		return nil, err
	}
	result, err := s.configToolClient.ListPluginEnumValues(ctx, endpoint, &ConfigToolPluginEnumValuesRequest{PluginType: normalizeSyncPluginType(req.PluginType), FactoryIdentifier: strings.TrimSpace(req.FactoryIdentifier), OptionKey: strings.TrimSpace(req.OptionKey), IncludeSupplement: boolValue(req.IncludeSupplement, true)})
	if err != nil {
		return nil, err
	}
	return &SyncPluginEnumValuesResult{PluginType: result.PluginType, FactoryIdentifier: result.FactoryIdentifier, OptionKey: result.OptionKey, EnumValues: result.EnumValues, Warnings: result.Warnings}, nil
}

func (s *Service) ListPluginEnumCatalog(
	ctx context.Context,
	req *ListSyncPluginEnumCatalogRequest,
) (*SyncPluginEnumCatalogResult, error) {
	if req == nil {
		return nil, fmt.Errorf("sync: plugin enum catalog request is required")
	}
	endpoint, err := s.resolvePluginProxyEndpoint(ctx, req.ClusterID)
	if err != nil {
		return nil, err
	}

	pluginTypes := []string{
		string(SyncPluginTypeSource),
		string(SyncPluginTypeTransform),
		string(SyncPluginTypeSink),
	}
	includeSupplement := boolValue(req.IncludeSupplement, true)
	result := &SyncPluginEnumCatalogResult{
		EnvOptions: builtinEnvEnumOptions(),
		Plugins:    make([]SyncPluginEnumCatalogPlugin, 0),
		Warnings:   make([]string, 0),
	}

	var (
		mu      sync.Mutex
		wg      sync.WaitGroup
		sem     = make(chan struct{}, 6)
		plugins = make([]SyncPluginEnumCatalogPlugin, 0)
	)

	for _, pluginType := range pluginTypes {
		listResult, listErr := s.configToolClient.ListPlugins(
			ctx,
			endpoint,
			&ConfigToolPluginListRequest{PluginType: pluginType},
		)
		if listErr != nil {
			result.Warnings = append(
				result.Warnings,
				fmt.Sprintf("%s plugin enum preload skipped: %v", pluginType, listErr),
			)
			continue
		}
		result.Warnings = append(result.Warnings, listResult.Warnings...)
		for _, item := range listResult.Plugins {
			factoryIdentifier := strings.TrimSpace(item.FactoryIdentifier)
			if factoryIdentifier == "" {
				continue
			}
			wg.Add(1)
			go func(pluginType string, factoryIdentifier string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				optionsResult, optionsErr := s.configToolClient.GetPluginOptions(
					ctx,
					endpoint,
					&ConfigToolPluginOptionsRequest{
						PluginType:        pluginType,
						FactoryIdentifier: factoryIdentifier,
						IncludeSupplement: includeSupplement,
					},
				)
				if optionsErr != nil {
					mu.Lock()
					result.Warnings = append(
						result.Warnings,
						fmt.Sprintf(
							"%s/%s enum preload skipped: %v",
							pluginType,
							factoryIdentifier,
							optionsErr,
						),
					)
					mu.Unlock()
					return
				}
				filtered := make([]SyncPluginOptionDescriptor, 0)
				for _, option := range mapSyncPluginOptions(optionsResult.Options) {
					if len(option.EnumValues) == 0 {
						continue
					}
					filtered = append(filtered, option)
				}
				if len(filtered) == 0 {
					return
				}
				sort.Slice(filtered, func(i, j int) bool {
					return filtered[i].Key < filtered[j].Key
				})
				mu.Lock()
				plugins = append(plugins, SyncPluginEnumCatalogPlugin{
					PluginType:        pluginType,
					FactoryIdentifier: factoryIdentifier,
					Options:           filtered,
				})
				result.Warnings = append(result.Warnings, optionsResult.Warnings...)
				mu.Unlock()
			}(pluginType, factoryIdentifier)
		}
	}

	wg.Wait()
	sort.Slice(plugins, func(i, j int) bool {
		if plugins[i].PluginType != plugins[j].PluginType {
			return plugins[i].PluginType < plugins[j].PluginType
		}
		return plugins[i].FactoryIdentifier < plugins[j].FactoryIdentifier
	})
	result.Plugins = plugins
	return result, nil
}

func (s *Service) resolvePluginProxyEndpoint(ctx context.Context, clusterID uint) (string, error) {
	if s == nil || s.configToolResolver == nil {
		return "", fmt.Errorf("sync: config tool resolver is not configured")
	}
	return s.configToolResolver.ResolveConfigToolEndpoint(ctx, clusterID, nil)
}

func normalizeSyncPluginType(pluginType string) string {
	switch strings.ToLower(strings.TrimSpace(pluginType)) {
	case string(SyncPluginTypeSource):
		return string(SyncPluginTypeSource)
	case string(SyncPluginTypeSink):
		return string(SyncPluginTypeSink)
	case string(SyncPluginTypeTransform):
		return string(SyncPluginTypeTransform)
	case string(SyncPluginTypeCatalog):
		return string(SyncPluginTypeCatalog)
	default:
		return strings.ToLower(strings.TrimSpace(pluginType))
	}
}

func mapSyncPluginOptions(items []ConfigToolPluginOptionDescriptor) []SyncPluginOptionDescriptor {
	result := make([]SyncPluginOptionDescriptor, 0, len(items))
	for _, item := range items {
		result = append(result, SyncPluginOptionDescriptor{Key: item.Key, Type: item.Type, ElementType: item.ElementType, DefaultValue: item.DefaultValue, Description: item.Description, FallbackKeys: item.FallbackKeys, EnumValues: item.EnumValues, EnumDisplayValues: item.EnumDisplayValues, RequiredMode: item.RequiredMode, ConditionExpression: item.ConditionExpression, ConstraintGroup: item.ConstraintGroup, Origins: item.Origins, DeclaredClasses: item.DeclaredClasses, Advanced: item.Advanced})
	}
	return result
}

func boolValue(value *bool, defaultValue bool) bool {
	if value == nil {
		return defaultValue
	}
	return *value
}

func builtinEnvEnumOptions() []SyncPluginOptionDescriptor {
	return []SyncPluginOptionDescriptor{
		{
			Key:          "job.mode",
			Type:         "string",
			DefaultValue: "BATCH",
			Description:  "SeaTunnel 作业模式",
			EnumValues:   []string{"BATCH", "STREAMING"},
			RequiredMode: "OPTIONAL",
		},
		{
			Key:          "savemode.execute.location",
			Type:         "string",
			DefaultValue: "CLUSTER",
			Description:  "SaveMode 执行位置",
			EnumValues:   []string{"CLUSTER", "ENGINE"},
			RequiredMode: "OPTIONAL",
		},
	}
}

func (s *Service) PreviewSinkSaveMode(
	ctx context.Context,
	taskID uint,
	req *PreviewSyncSinkSaveModeRequest,
) (*SyncSinkSaveModePreviewResult, error) {
	if req == nil {
		return nil, fmt.Errorf("sync: sink save mode preview request is required")
	}
	task, err := s.getTaskForExecution(ctx, taskID, req.Draft)
	if err != nil {
		return nil, err
	}
	if task.NodeType != TaskNodeTypeFile {
		return nil, ErrTaskNotFile
	}
	endpoint, err := s.configToolResolver.ResolveConfigToolEndpoint(ctx, req.ClusterID, task.Definition)
	if err != nil {
		return nil, err
	}
	contentReq, err := s.buildConfigToolContentRequest(ctx, task, nil)
	if err != nil {
		return nil, err
	}
	proxyReq := &ConfigToolSinkSaveModePreviewRequest{
		ConfigToolContentRequest: *contentReq,
		SinkIndex:                req.SinkIndex,
		SinkNodeID:               strings.TrimSpace(req.SinkNodeID),
		IncludeInfoPreview:       boolValue(req.IncludeInfoPreview, true),
	}
	result, err := s.configToolClient.PreviewSinkSaveMode(ctx, endpoint, proxyReq)
	if err != nil {
		return nil, err
	}
	return mapSyncSinkSaveModePreviewResult(result), nil
}

func mapSyncSinkSaveModePreviewResult(result *ConfigToolSinkSaveModePreviewResponse) *SyncSinkSaveModePreviewResult {
	if result == nil {
		return nil
	}
	items := make([]SyncSinkSaveModePreviewTable, 0, len(result.Tables))
	for _, item := range result.Tables {
		items = append(items, SyncSinkSaveModePreviewTable{
			TablePath:      item.TablePath,
			Supported:      item.Supported,
			Completeness:   item.Completeness,
			SchemaSaveMode: item.SchemaSaveMode,
			DataSaveMode:   item.DataSaveMode,
			Actions:        mapSyncSinkSaveModePreviewActions(item.Actions),
			Warnings:       append([]string{}, item.Warnings...),
		})
	}
	return &SyncSinkSaveModePreviewResult{
		Connector:      result.Connector,
		SinkIndex:      result.SinkIndex,
		Supported:      result.Supported,
		Completeness:   result.Completeness,
		SchemaSaveMode: result.SchemaSaveMode,
		DataSaveMode:   result.DataSaveMode,
		TablePath:      result.TablePath,
		Actions:        mapSyncSinkSaveModePreviewActions(result.Actions),
		Tables:         items,
		Warnings:       result.Warnings,
	}
}

func mapSyncSinkSaveModePreviewActions(items []ConfigToolSinkSaveModePreviewAction) []SyncSinkSaveModePreviewAction {
	result := make([]SyncSinkSaveModePreviewAction, 0, len(items))
	for _, item := range items {
		result = append(result, SyncSinkSaveModePreviewAction{
			Phase:      item.Phase,
			ActionType: item.ActionType,
			ResultType: item.ResultType,
			Content:    item.Content,
			Native:     item.Native,
		})
	}
	return result
}
