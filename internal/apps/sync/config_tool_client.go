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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	clusterapp "github.com/LeonYoah/stx/internal/apps/cluster"
)

// ConfigToolClient talks to stx-java-proxy config APIs.
type ConfigToolClient interface {
	InspectDAG(ctx context.Context, endpoint string, req *ConfigToolContentRequest) (*ConfigToolDAGResponse, error)
	InspectWebUIDAG(ctx context.Context, endpoint string, req *ConfigToolContentRequest) (*ConfigToolWebUIDAGResponse, error)
	ValidateConfig(ctx context.Context, endpoint string, req *ConfigToolValidateRequest) (*ConfigToolValidateResponse, error)
	DeriveSourcePreview(ctx context.Context, endpoint string, req *ConfigToolPreviewRequest) (*ConfigToolPreviewResponse, error)
	DeriveTransformPreview(ctx context.Context, endpoint string, req *ConfigToolPreviewRequest) (*ConfigToolPreviewResponse, error)
	ListPlugins(ctx context.Context, endpoint string, req *ConfigToolPluginListRequest) (*ConfigToolPluginListResponse, error)
	GetPluginOptions(ctx context.Context, endpoint string, req *ConfigToolPluginOptionsRequest) (*ConfigToolPluginOptionsResponse, error)
	RenderPluginTemplate(ctx context.Context, endpoint string, req *ConfigToolPluginTemplateRequest) (*ConfigToolPluginTemplateResponse, error)
	ListPluginEnumValues(ctx context.Context, endpoint string, req *ConfigToolPluginEnumValuesRequest) (*ConfigToolPluginEnumValuesResponse, error)
	PreviewSinkSaveMode(ctx context.Context, endpoint string, req *ConfigToolSinkSaveModePreviewRequest) (*ConfigToolSinkSaveModePreviewResponse, error)
}

// ConfigToolResolver resolves java-proxy endpoint for a sync task.
type ConfigToolResolver interface {
	ResolveConfigToolEndpoint(ctx context.Context, clusterID uint, taskDefinition JSONMap) (string, error)
}

// ConfigToolContentRequest represents shared content/filePath input.
type ConfigToolContentRequest struct {
	Content       string   `json:"content,omitempty"`
	ContentFormat string   `json:"contentFormat,omitempty"`
	FilePath      string   `json:"filePath,omitempty"`
	Variables     []string `json:"variables,omitempty"`
}

// ConfigToolPreviewRequest represents preview derive request.
type ConfigToolPreviewRequest struct {
	ConfigToolContentRequest
	SourceNodeID          string                 `json:"sourceNodeId,omitempty"`
	SourceIndex           *int                   `json:"sourceIndex,omitempty"`
	TransformNodeID       string                 `json:"transformNodeId,omitempty"`
	TransformIndex        *int                   `json:"transformIndex,omitempty"`
	PlatformJobID         string                 `json:"platformJobId,omitempty"`
	EngineJobID           string                 `json:"engineJobId,omitempty"`
	PreviewRowLimit       *int                   `json:"previewRowLimit,omitempty"`
	OutputFormat          string                 `json:"outputFormat,omitempty"`
	MetadataOutputDataset string                 `json:"metadataOutputDataset,omitempty"`
	MetadataFields        map[string]interface{} `json:"metadataFields,omitempty"`
	EnvOverrides          map[string]interface{} `json:"envOverrides,omitempty"`
	HttpSink              map[string]interface{} `json:"httpSink,omitempty"`
}

// ConfigToolValidateRequest represents config validation / connection test input.
type ConfigToolValidateRequest struct {
	ConfigToolContentRequest
	TestConnection bool `json:"testConnection"`
}

// ConfigToolPluginListRequest represents plugin discovery input.
type ConfigToolPluginListRequest struct {
	PluginType string   `json:"pluginType"`
	PluginJars []string `json:"pluginJars,omitempty"`
}

// ConfigToolPluginFactoryInfo mirrors java-proxy plugin list entry.
type ConfigToolPluginFactoryInfo struct {
	FactoryIdentifier string `json:"factoryIdentifier"`
	ClassName         string `json:"className,omitempty"`
	Origin            string `json:"origin,omitempty"`
}

// ConfigToolPluginListResponse mirrors java-proxy plugin list response.
type ConfigToolPluginListResponse struct {
	OK         bool                          `json:"ok"`
	PluginType string                        `json:"pluginType"`
	Plugins    []ConfigToolPluginFactoryInfo `json:"plugins"`
	Warnings   []string                      `json:"warnings"`
}

// ConfigToolPluginOptionsRequest represents plugin schema inspection input.
type ConfigToolPluginOptionsRequest struct {
	PluginType        string   `json:"pluginType"`
	FactoryIdentifier string   `json:"factoryIdentifier"`
	PluginJars        []string `json:"pluginJars,omitempty"`
	IncludeSupplement bool     `json:"includeSupplement"`
}

// ConfigToolPluginOptionDescriptor mirrors java-proxy plugin option descriptor.
type ConfigToolPluginOptionDescriptor struct {
	Key                 string      `json:"key"`
	Type                string      `json:"type,omitempty"`
	ElementType         string      `json:"elementType,omitempty"`
	DefaultValue        interface{} `json:"defaultValue,omitempty"`
	Description         string      `json:"description,omitempty"`
	FallbackKeys        []string    `json:"fallbackKeys,omitempty"`
	EnumValues          []string    `json:"enumValues,omitempty"`
	EnumDisplayValues   []string    `json:"enumDisplayValues,omitempty"`
	RequiredMode        string      `json:"requiredMode,omitempty"`
	ConditionExpression string      `json:"conditionExpression,omitempty"`
	ConstraintGroup     string      `json:"constraintGroup,omitempty"`
	Origins             []string    `json:"origins,omitempty"`
	DeclaredClasses     []string    `json:"declaredClasses,omitempty"`
	Advanced            bool        `json:"advanced"`
}

// ConfigToolPluginOptionsResponse mirrors java-proxy plugin options response.
type ConfigToolPluginOptionsResponse struct {
	OK                bool                               `json:"ok"`
	PluginType        string                             `json:"pluginType"`
	FactoryIdentifier string                             `json:"factoryIdentifier"`
	Options           []ConfigToolPluginOptionDescriptor `json:"options"`
	Warnings          []string                           `json:"warnings"`
}

// ConfigToolPluginTemplateRequest represents plugin template rendering input.
type ConfigToolPluginTemplateRequest struct {
	PluginType        string   `json:"pluginType"`
	FactoryIdentifier string   `json:"factoryIdentifier"`
	PluginJars        []string `json:"pluginJars,omitempty"`
	IncludeSupplement bool     `json:"includeSupplement"`
	IncludeComments   bool     `json:"includeComments"`
	IncludeAdvanced   bool     `json:"includeAdvanced"`
}

// ConfigToolPluginTemplateResponse mirrors java-proxy template response.
type ConfigToolPluginTemplateResponse struct {
	OK                bool     `json:"ok"`
	PluginType        string   `json:"pluginType"`
	FactoryIdentifier string   `json:"factoryIdentifier"`
	ContentFormat     string   `json:"contentFormat"`
	Template          string   `json:"template"`
	Warnings          []string `json:"warnings"`
}

// ConfigToolPluginEnumValuesRequest represents plugin enum values input.
type ConfigToolPluginEnumValuesRequest struct {
	PluginType        string   `json:"pluginType"`
	FactoryIdentifier string   `json:"factoryIdentifier"`
	OptionKey         string   `json:"optionKey"`
	PluginJars        []string `json:"pluginJars,omitempty"`
	IncludeSupplement bool     `json:"includeSupplement"`
}

// ConfigToolPluginEnumValuesResponse mirrors java-proxy enum values response.
type ConfigToolPluginEnumValuesResponse struct {
	OK                bool     `json:"ok"`
	PluginType        string   `json:"pluginType"`
	FactoryIdentifier string   `json:"factoryIdentifier"`
	OptionKey         string   `json:"optionKey"`
	EnumValues        []string `json:"enumValues"`
	Warnings          []string `json:"warnings"`
}

// ConfigToolSinkSaveModePreviewRequest represents sink save mode preview input.
type ConfigToolSinkSaveModePreviewRequest struct {
	ConfigToolContentRequest
	SinkIndex          *int     `json:"sinkIndex,omitempty"`
	SinkNodeID         string   `json:"sinkNodeId,omitempty"`
	PluginJars         []string `json:"pluginJars,omitempty"`
	IncludeInfoPreview bool     `json:"includeInfoPreview"`
}

// ConfigToolSinkSaveModePreviewAction mirrors one java-proxy save mode preview action.
type ConfigToolSinkSaveModePreviewAction struct {
	Phase      string `json:"phase,omitempty"`
	ActionType string `json:"actionType,omitempty"`
	ResultType string `json:"resultType,omitempty"`
	Content    string `json:"content,omitempty"`
	Native     bool   `json:"native"`
}

// ConfigToolSinkSaveModePreviewTable mirrors per-table preview details.
type ConfigToolSinkSaveModePreviewTable struct {
	TablePath      string                                `json:"tablePath,omitempty"`
	Supported      bool                                  `json:"supported"`
	Completeness   string                                `json:"completeness,omitempty"`
	SchemaSaveMode string                                `json:"schemaSaveMode,omitempty"`
	DataSaveMode   string                                `json:"dataSaveMode,omitempty"`
	Actions        []ConfigToolSinkSaveModePreviewAction `json:"actions,omitempty"`
	Warnings       []string                              `json:"warnings,omitempty"`
}

// ConfigToolSinkSaveModePreviewResponse mirrors java-proxy save mode preview response.
type ConfigToolSinkSaveModePreviewResponse struct {
	OK             bool                                  `json:"ok"`
	Connector      string                                `json:"connector,omitempty"`
	SinkIndex      int                                   `json:"sinkIndex,omitempty"`
	Supported      bool                                  `json:"supported"`
	Completeness   string                                `json:"completeness,omitempty"`
	SchemaSaveMode string                                `json:"schemaSaveMode,omitempty"`
	DataSaveMode   string                                `json:"dataSaveMode,omitempty"`
	TablePath      string                                `json:"tablePath,omitempty"`
	Actions        []ConfigToolSinkSaveModePreviewAction `json:"actions,omitempty"`
	Tables         []ConfigToolSinkSaveModePreviewTable  `json:"tables,omitempty"`
	Warnings       []string                              `json:"warnings,omitempty"`
}

// ConfigToolGraph mirrors java-proxy DAG payload.
type ConfigToolGraph struct {
	Nodes []map[string]interface{} `json:"nodes"`
	Edges []map[string]interface{} `json:"edges"`
}

// ConfigToolDAGResponse mirrors java-proxy dag response.
type ConfigToolDAGResponse struct {
	OK             bool            `json:"ok"`
	Message        string          `json:"message,omitempty"`
	SimpleGraph    bool            `json:"simpleGraph"`
	SourceCount    int             `json:"sourceCount"`
	TransformCount int             `json:"transformCount"`
	SinkCount      int             `json:"sinkCount"`
	Warnings       []string        `json:"warnings"`
	Graph          ConfigToolGraph `json:"graph"`
}

// ConfigToolPreviewResponse mirrors java-proxy preview response.
type ConfigToolPreviewResponse struct {
	OK             bool                   `json:"ok"`
	Message        string                 `json:"message,omitempty"`
	Mode           string                 `json:"mode,omitempty"`
	SelectedNodeID string                 `json:"selectedNodeId,omitempty"`
	SelectedIndex  int                    `json:"selectedIndex,omitempty"`
	Warnings       []string               `json:"warnings"`
	Content        string                 `json:"content,omitempty"`
	ContentFormat  string                 `json:"contentFormat,omitempty"`
	Config         map[string]interface{} `json:"config,omitempty"`
	Graph          ConfigToolGraph        `json:"graph"`
	SimpleGraph    bool                   `json:"simpleGraph"`
}

// ConfigToolWebUIDAGVertexInfo mirrors webui vertex payload from java-proxy.
type ConfigToolWebUIDAGVertexInfo struct {
	VertexID         int                                           `json:"vertexId"`
	Type             string                                        `json:"type"`
	ConnectorType    string                                        `json:"connectorType"`
	TablePaths       []string                                      `json:"tablePaths"`
	TableColumns     map[string][]string                           `json:"tableColumns"`
	TableSchemas     map[string]map[string]interface{}             `json:"tableSchemas"`
	SaveModePreviews map[string]ConfigToolSinkSaveModePreviewTable `json:"saveModePreviews,omitempty"`
	SaveModeWarnings []string                                      `json:"saveModeWarnings,omitempty"`
}

// ConfigToolWebUIDAGEdge mirrors webui edge payload from java-proxy.
type ConfigToolWebUIDAGEdge struct {
	InputVertexID  int `json:"inputVertexId"`
	TargetVertexID int `json:"targetVertexId"`
}

// ConfigToolWebUIJobDAG mirrors webui compatible jobDag payload.
type ConfigToolWebUIJobDAG struct {
	JobID         string                                  `json:"jobId"`
	PipelineEdges map[string][]ConfigToolWebUIDAGEdge     `json:"pipelineEdges"`
	VertexInfoMap map[string]ConfigToolWebUIDAGVertexInfo `json:"vertexInfoMap"`
	EnvOptions    map[string]interface{}                  `json:"envOptions"`
}

// ConfigToolWebUIDAGResponse mirrors java-proxy webui dag preview response.
type ConfigToolWebUIDAGResponse struct {
	JobID          string                 `json:"jobId"`
	JobName        string                 `json:"jobName"`
	JobStatus      string                 `json:"jobStatus"`
	ErrorMsg       string                 `json:"errorMsg"`
	CreateTime     string                 `json:"createTime"`
	FinishTime     string                 `json:"finishTime"`
	JobDag         ConfigToolWebUIJobDAG  `json:"jobDag"`
	Metrics        map[string]interface{} `json:"metrics"`
	PluginJarsUrls []string               `json:"pluginJarsUrls"`
	SimpleGraph    bool                   `json:"simpleGraph"`
	Warnings       []string               `json:"warnings"`
}

// ConfigToolValidationCheck represents one connector validation result.
type ConfigToolValidationCheck struct {
	NodeID        string `json:"nodeId"`
	Kind          string `json:"kind"`
	ConnectorType string `json:"connectorType"`
	Target        string `json:"target"`
	Status        string `json:"status"`
	Message       string `json:"message"`
}

// ConfigToolValidateResponse mirrors java-proxy config validation response.
type ConfigToolValidateResponse struct {
	OK       bool                        `json:"ok"`
	Valid    bool                        `json:"valid"`
	Summary  string                      `json:"summary"`
	Errors   []string                    `json:"errors"`
	Warnings []string                    `json:"warnings"`
	Checks   []ConfigToolValidationCheck `json:"checks"`
}

// ConfigToolError captures structured proxy request failures.
type ConfigToolError struct {
	StatusCode int
	Path       string
	Message    string
	RawBody    string
	ErrorType  string
}

func (e *ConfigToolError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("sync: config tool request failed: status=%d body=%s", e.StatusCode, e.RawBody)
}

// DefaultConfigToolClient is the default java-proxy HTTP client.
type DefaultConfigToolClient struct {
	httpClient *http.Client
}

// NewDefaultConfigToolClient creates a config tool client.
func NewDefaultConfigToolClient() *DefaultConfigToolClient {
	return &DefaultConfigToolClient{
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// InspectDAG calls /api/v1/config/dag.
func (c *DefaultConfigToolClient) InspectDAG(ctx context.Context, endpoint string, req *ConfigToolContentRequest) (*ConfigToolDAGResponse, error) {
	var result ConfigToolDAGResponse
	if err := c.postJSON(ctx, endpoint, "/api/v1/config/dag", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// InspectWebUIDAG calls /api/v1/config/webui-dag.
func (c *DefaultConfigToolClient) InspectWebUIDAG(ctx context.Context, endpoint string, req *ConfigToolContentRequest) (*ConfigToolWebUIDAGResponse, error) {
	var result ConfigToolWebUIDAGResponse
	if err := c.postJSON(ctx, endpoint, "/api/v1/config/webui-dag", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ValidateConfig calls /api/v1/config/validate.
func (c *DefaultConfigToolClient) ValidateConfig(ctx context.Context, endpoint string, req *ConfigToolValidateRequest) (*ConfigToolValidateResponse, error) {
	var result ConfigToolValidateResponse
	if err := c.postJSON(ctx, endpoint, "/api/v1/config/validate", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// DeriveSourcePreview calls /api/v1/config/preview/source.
func (c *DefaultConfigToolClient) DeriveSourcePreview(ctx context.Context, endpoint string, req *ConfigToolPreviewRequest) (*ConfigToolPreviewResponse, error) {
	var result ConfigToolPreviewResponse
	if err := c.postJSON(ctx, endpoint, "/api/v1/config/preview/source", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// DeriveTransformPreview calls /api/v1/config/preview/transform.
func (c *DefaultConfigToolClient) DeriveTransformPreview(ctx context.Context, endpoint string, req *ConfigToolPreviewRequest) (*ConfigToolPreviewResponse, error) {
	var result ConfigToolPreviewResponse
	if err := c.postJSON(ctx, endpoint, "/api/v1/config/preview/transform", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ListPlugins calls /api/v1/plugin/list.
func (c *DefaultConfigToolClient) ListPlugins(ctx context.Context, endpoint string, req *ConfigToolPluginListRequest) (*ConfigToolPluginListResponse, error) {
	var result ConfigToolPluginListResponse
	if err := c.postJSON(ctx, endpoint, "/api/v1/plugin/list", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetPluginOptions calls /api/v1/plugin/options.
func (c *DefaultConfigToolClient) GetPluginOptions(ctx context.Context, endpoint string, req *ConfigToolPluginOptionsRequest) (*ConfigToolPluginOptionsResponse, error) {
	var result ConfigToolPluginOptionsResponse
	if err := c.postJSON(ctx, endpoint, "/api/v1/plugin/options", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// RenderPluginTemplate calls /api/v1/plugin/template.
func (c *DefaultConfigToolClient) RenderPluginTemplate(ctx context.Context, endpoint string, req *ConfigToolPluginTemplateRequest) (*ConfigToolPluginTemplateResponse, error) {
	var result ConfigToolPluginTemplateResponse
	if err := c.postJSON(ctx, endpoint, "/api/v1/plugin/template", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// PreviewSinkSaveMode calls /api/v1/config/preview/sink-savemode.
func (c *DefaultConfigToolClient) PreviewSinkSaveMode(ctx context.Context, endpoint string, req *ConfigToolSinkSaveModePreviewRequest) (*ConfigToolSinkSaveModePreviewResponse, error) {
	var result ConfigToolSinkSaveModePreviewResponse
	if err := c.postJSON(ctx, endpoint, "/api/v1/config/preview/sink-savemode", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// ListPluginEnumValues calls /api/v1/plugin/enum-values.
func (c *DefaultConfigToolClient) ListPluginEnumValues(ctx context.Context, endpoint string, req *ConfigToolPluginEnumValuesRequest) (*ConfigToolPluginEnumValuesResponse, error) {
	var result ConfigToolPluginEnumValuesResponse
	if err := c.postJSON(ctx, endpoint, "/api/v1/plugin/enum-values", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *DefaultConfigToolClient) postJSON(ctx context.Context, endpoint string, path string, payload interface{}, out interface{}) error {
	if strings.TrimSpace(endpoint) == "" {
		return fmt.Errorf("sync: config tool endpoint is required")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	targetURL := strings.TrimRight(strings.TrimSpace(endpoint), "/") + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return parseConfigToolError(resp.StatusCode, path, respBody)
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return err
	}
	return nil
}

func parseConfigToolError(statusCode int, path string, body []byte) error {
	trimmed := strings.TrimSpace(string(body))
	payload := struct {
		OK      bool   `json:"ok"`
		Message string `json:"message"`
	}{}
	if err := json.Unmarshal(body, &payload); err == nil && strings.TrimSpace(payload.Message) != "" {
		message := strings.TrimSpace(payload.Message)
		return &ConfigToolError{
			StatusCode: statusCode,
			Path:       path,
			Message:    humanizeConfigToolMessage(path, message),
			RawBody:    trimmed,
			ErrorType:  inferConfigToolErrorType(message),
		}
	}
	return &ConfigToolError{
		StatusCode: statusCode,
		Path:       path,
		Message:    fmt.Sprintf("sync: config tool request failed: status=%d body=%s", statusCode, trimmed),
		RawBody:    trimmed,
		ErrorType:  "config_tool_request_failed",
	}
}

func inferConfigToolErrorType(message string) string {
	normalized := strings.ToLower(strings.TrimSpace(message))
	switch {
	case strings.Contains(normalized, "config parse failed"):
		return "config_parse_error"
	case strings.Contains(normalized, "expecting close brace"),
		strings.Contains(normalized, "json does not allow unescaped newline"),
		strings.Contains(normalized, "configexception"):
		return "config_parse_error"
	default:
		return "config_tool_request_failed"
	}
}

// humanizeResolutionDetail 全局转译常见数据库与连接器底层报错为人类可读说明
// humanizeResolutionDetail globally translates common database and connector errors into human-readable explanations
func humanizeResolutionDetail(detail string) string {
	lower := strings.ToLower(detail)
	switch {
	case strings.Contains(lower, "can not find catalog table with factoryid"):
		return fmt.Sprintf("目标数据库中未找到匹配的物理表元数据（物理表不存在或已被删除，请检查 table-names / table-pattern 配置或在数据库中建表） | %s", detail)
	case strings.Contains(lower, "doesn't exist") || strings.Contains(lower, "does not exist"):
		return fmt.Sprintf("指定数据表在数据库中不存在（请核对表名拼写或在数据库中建表） | %s", detail)
	case strings.Contains(lower, "access denied for user") || strings.Contains(lower, "password authentication failed"):
		return fmt.Sprintf("数据库账号或密码认证失败（请检查连接配置中的用户名密码） | %s", detail)
	case strings.Contains(lower, "communications link failure") || strings.Contains(lower, "connection refused") || strings.Contains(lower, "connection timed out"):
		return fmt.Sprintf("无法连接到目标数据库（请检查网络连通性、端口及数据库服务状态） | %s", detail)
	default:
		return detail
	}
}

func humanizeConfigToolMessage(path string, message string) string {
	trimmed := strings.TrimSpace(message)
	if inferConfigToolErrorType(trimmed) == "config_parse_error" {
		detail := strings.TrimSpace(strings.TrimPrefix(trimmed, "Config parse failed:"))
		detail = strings.TrimSpace(strings.TrimPrefix(detail, "Config dag inspection failed:"))
		if detail == "" {
			return "sync: 配置解析失败，请检查括号、引号、逗号和换行"
		}
		return fmt.Sprintf("sync: 配置解析失败，请检查括号、引号、逗号和换行：%s", detail)
	}
	switch path {
	case "/api/v1/config/webui-dag":
		return fmt.Sprintf("sync: DAG 解析失败：%s", humanizeResolutionDetail(trimmed))
	case "/api/v1/config/validate":
		return fmt.Sprintf("sync: 配置校验失败：%s", humanizeResolutionDetail(trimmed))
	default:
		return fmt.Sprintf("sync: config tool request failed: status? message=%s", humanizeResolutionDetail(trimmed))
	}
}

// DefaultConfigToolResolver resolves java-proxy endpoint using cluster managed status or task override.
type DefaultConfigToolResolver struct {
	clusterService *clusterapp.Service
}

// NewDefaultConfigToolResolver creates a config tool resolver.
func NewDefaultConfigToolResolver(clusterService *clusterapp.Service) *DefaultConfigToolResolver {
	return &DefaultConfigToolResolver{clusterService: clusterService}
}

// ResolveConfigToolEndpoint resolves java-proxy endpoint.
func (r *DefaultConfigToolResolver) ResolveConfigToolEndpoint(ctx context.Context, clusterID uint, taskDefinition JSONMap) (string, error) {
	if endpoint := strings.TrimSpace(stringValue(taskDefinition, "proxy_base_url", "java_proxy_base_url", "config_tool_base_url")); endpoint != "" {
		return endpoint, nil
	}
	if r == nil || r.clusterService == nil {
		return "", fmt.Errorf("sync: config tool resolver is not configured")
	}
	status, err := r.clusterService.GetSTXJavaProxyStatus(ctx, clusterID)
	if err != nil {
		return "", err
	}
	if status == nil || strings.TrimSpace(status.Endpoint) == "" {
		return "", fmt.Errorf("sync: stx-java-proxy endpoint is empty for cluster %d", clusterID)
	}
	return strings.TrimSpace(status.Endpoint), nil
}
