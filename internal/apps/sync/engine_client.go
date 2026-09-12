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
	neturl "net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	clusterapp "github.com/LeonYoah/stx/internal/apps/cluster"
	hostapp "github.com/LeonYoah/stx/internal/apps/host"
)

const engineEndpointHeartbeatTimeout = 30 * time.Second

// EngineClient submits and manages jobs on SeaTunnel Engine REST API V2.
type EngineClient interface {
	Submit(ctx context.Context, req *EngineSubmitRequest) (*EngineSubmitResponse, error)
	GetJobInfo(ctx context.Context, endpoint *EngineEndpoint, jobID string) (*EngineJobInfo, error)
	GetJobCheckpointOverview(ctx context.Context, endpoint *EngineEndpoint, jobID string) (*EngineCheckpointOverview, error)
	GetJobCheckpointHistory(ctx context.Context, endpoint *EngineEndpoint, jobID string, pipelineID *int, limit int, status string) ([]*EngineCheckpointRecord, error)
	StopJob(ctx context.Context, endpoint *EngineEndpoint, jobID string, stopWithSavepoint bool) error
	GetJobLogs(ctx context.Context, endpoint *EngineEndpoint, jobID string) (string, error)
}

// EngineEndpoint describes one SeaTunnel REST endpoint.
type EngineEndpoint struct {
	BaseURL     string
	ContextPath string
	LegacyURL   string
	APIMode     string
}

// EngineSubmitRequest describes one submit request.
type EngineSubmitRequest struct {
	Endpoint           *EngineEndpoint
	Format             string
	JobID              string
	JobName            string
	StartWithSavepoint bool
	Body               []byte
}

// EngineSubmitResponse describes submit response payload.
type EngineSubmitResponse struct {
	JobID           string `json:"jobId"`
	JobName         string `json:"jobName"`
	APIMode         string `json:"apiMode,omitempty"`
	EndpointBaseURL string `json:"endpointBaseUrl,omitempty"`
}

// EngineJobInfo describes job-info response subset used by sync studio.
type EngineJobInfo struct {
	JobID        string                 `json:"jobId"`
	JobName      string                 `json:"jobName"`
	JobStatus    string                 `json:"jobStatus"`
	CreateTime   string                 `json:"createTime"`
	FinishedTime string                 `json:"finishTime"`
	ErrorMsg     interface{}            `json:"errorMsg"`
	JobDag       map[string]interface{} `json:"jobDag"`
	Metrics      map[string]interface{} `json:"metrics"`
}

// EngineCheckpointOverview describes one checkpoint overview payload from the engine.
type EngineCheckpointOverview struct {
	JobID     string                      `json:"jobId"`
	UpdatedAt int64                       `json:"updatedAt"`
	Pipelines []*EngineCheckpointPipeline `json:"pipelines"`
}

// EngineCheckpointPipeline describes one pipeline checkpoint summary.
type EngineCheckpointPipeline struct {
	PipelineID      int                           `json:"pipelineId"`
	Counts          map[string]int64              `json:"counts"`
	LatestCompleted *EngineCheckpoint             `json:"latestCompleted,omitempty"`
	LatestFailed    *EngineCheckpoint             `json:"latestFailed,omitempty"`
	LatestSavepoint *EngineCheckpoint             `json:"latestSavepoint,omitempty"`
	InProgress      []*EngineCheckpointInProgress `json:"inProgress,omitempty"`
	History         []*EngineCheckpointRecord     `json:"history,omitempty"`
}

// EngineCheckpoint describes one checkpoint metadata record.
type EngineCheckpoint struct {
	CheckpointID       int64  `json:"checkpointId"`
	CheckpointType     string `json:"checkpointType"`
	Status             string `json:"status"`
	TriggerTimestamp   int64  `json:"triggerTimestamp"`
	CompletedTimestamp int64  `json:"completedTimestamp,omitempty"`
	DurationMillis     int64  `json:"durationMillis,omitempty"`
	StateSize          int64  `json:"stateSize,omitempty"`
	FailureReason      string `json:"failureReason,omitempty"`
}

// EngineCheckpointInProgress describes one running checkpoint progress row.
type EngineCheckpointInProgress struct {
	CheckpointID     int64  `json:"checkpointId"`
	CheckpointType   string `json:"checkpointType"`
	TriggerTimestamp int64  `json:"triggerTimestamp"`
	Acknowledged     int64  `json:"acknowledged"`
	Total            int64  `json:"total"`
}

// EngineCheckpointRecord describes one checkpoint history row.
type EngineCheckpointRecord struct {
	PipelineID int               `json:"pipelineId"`
	Checkpoint *EngineCheckpoint `json:"checkpoint"`
}

// ClusterRuntimeResolver resolves SeaTunnel engine API endpoints from cluster metadata.
type ClusterRuntimeResolver interface {
	ResolveEngineEndpoint(ctx context.Context, clusterID uint, taskDefinition JSONMap) (*EngineEndpoint, error)
}

// SeaTunnelEngineClient is the default REST V2 engine client.
type SeaTunnelEngineClient struct {
	httpClient *http.Client
}

// NewSeaTunnelEngineClient creates a SeaTunnel engine client.
func NewSeaTunnelEngineClient() *SeaTunnelEngineClient {
	return &SeaTunnelEngineClient{
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// Submit submits one SeaTunnel job.
func (c *SeaTunnelEngineClient) Submit(ctx context.Context, req *EngineSubmitRequest) (*EngineSubmitResponse, error) {
	if req == nil || req.Endpoint == nil {
		return nil, fmt.Errorf("sync: engine endpoint is required")
	}
	if strings.EqualFold(strings.TrimSpace(req.Endpoint.APIMode), "v1") {
		return c.submitV1(ctx, req)
	}
	params := map[string]string{
		"format":  normalizeSubmitFormat(req.Format),
		"jobName": strings.TrimSpace(req.JobName),
	}
	if jobID := strings.TrimSpace(req.JobID); jobID != "" {
		params["jobId"] = jobID
	}
	if req.StartWithSavepoint {
		params["isStartWithSavePoint"] = "true"
	}
	targetURL, err := buildEngineURL(req.Endpoint, "/submit-job", params)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(req.Body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "text/plain; charset=utf-8")
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		if req.Endpoint != nil && strings.TrimSpace(req.Endpoint.LegacyURL) != "" {
			if normalizeSubmitFormat(req.Format) != "json" {
				return nil, fmt.Errorf("sync: engine REST v2 is unavailable and legacy REST v1 fallback only supports json payloads")
			}
			return c.submitV1(ctx, req)
		}
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		fallbackResp, fallbackErr := c.trySubmitLegacy(ctx, req, resp.StatusCode, body)
		if fallbackErr == nil && fallbackResp != nil {
			return fallbackResp, nil
		}
		if fallbackErr != nil {
			return nil, fallbackErr
		}
		return nil, fmt.Errorf("sync: submit job failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var result EngineSubmitResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	if strings.TrimSpace(result.JobID) == "" {
		result.JobID = strings.TrimSpace(req.JobID)
	}
	if strings.TrimSpace(result.JobID) == "" {
		return nil, fmt.Errorf("sync: submit job succeeded but returned empty jobId")
	}
	if strings.TrimSpace(result.JobName) == "" {
		result.JobName = strings.TrimSpace(req.JobName)
	}
	result.APIMode = "v2"
	result.EndpointBaseURL = strings.TrimSpace(req.Endpoint.BaseURL)
	return &result, nil
}

// GetJobInfo fetches one job info.
func (c *SeaTunnelEngineClient) GetJobInfo(ctx context.Context, endpoint *EngineEndpoint, jobID string) (*EngineJobInfo, error) {
	if endpoint != nil && strings.EqualFold(strings.TrimSpace(endpoint.APIMode), "v1") {
		return c.getJobInfoV1(ctx, endpoint, jobID)
	}
	targetURL, err := buildEngineURL(endpoint, "/job-info/"+neturl.PathEscape(strings.TrimSpace(jobID)), nil)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("sync: get job info failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var result EngineJobInfo
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetJobCheckpointOverview fetches one job checkpoint overview from REST V2.
func (c *SeaTunnelEngineClient) GetJobCheckpointOverview(ctx context.Context, endpoint *EngineEndpoint, jobID string) (*EngineCheckpointOverview, error) {
	if endpoint != nil && strings.EqualFold(strings.TrimSpace(endpoint.APIMode), "v1") {
		return nil, fmt.Errorf("sync: checkpoint overview is not supported by legacy engine api")
	}
	targetURL, err := buildEngineURL(endpoint, "/jobs/checkpoints/"+neturl.PathEscape(strings.TrimSpace(jobID)), nil)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("sync: get checkpoint overview failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var result EngineCheckpointOverview
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetJobCheckpointHistory fetches job checkpoint history from REST V2.
func (c *SeaTunnelEngineClient) GetJobCheckpointHistory(ctx context.Context, endpoint *EngineEndpoint, jobID string, pipelineID *int, limit int, status string) ([]*EngineCheckpointRecord, error) {
	if endpoint != nil && strings.EqualFold(strings.TrimSpace(endpoint.APIMode), "v1") {
		return nil, fmt.Errorf("sync: checkpoint history is not supported by legacy engine api")
	}
	params := map[string]string{}
	if pipelineID != nil && *pipelineID > 0 {
		params["pipelineId"] = strconv.Itoa(*pipelineID)
	}
	if limit > 0 {
		params["limit"] = strconv.Itoa(limit)
	}
	if trimmed := strings.ToUpper(strings.TrimSpace(status)); trimmed != "" {
		params["status"] = trimmed
	}
	targetURL, err := buildEngineURL(endpoint, "/jobs/checkpoints/history/"+neturl.PathEscape(strings.TrimSpace(jobID)), params)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("sync: get checkpoint history failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	records := make([]*EngineCheckpointRecord, 0)
	if err := json.Unmarshal(body, &records); err != nil {
		return nil, err
	}
	return records, nil
}

// StopJob cancels one running job.
func (c *SeaTunnelEngineClient) StopJob(ctx context.Context, endpoint *EngineEndpoint, jobID string, stopWithSavepoint bool) error {
	if endpoint != nil && strings.EqualFold(strings.TrimSpace(endpoint.APIMode), "v1") {
		return c.stopJobV1(ctx, endpoint, jobID, stopWithSavepoint)
	}
	targetURL, err := buildEngineURL(endpoint, "/stop-job", nil)
	if err != nil {
		return err
	}
	body, err := json.Marshal(map[string]interface{}{
		"jobId":               strings.TrimSpace(jobID),
		"isStopWithSavePoint": stopWithSavepoint,
	})
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("sync: stop job failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

// GetJobLogs fetches one job log payload through SeaTunnel REST APIs.
func (c *SeaTunnelEngineClient) GetJobLogs(ctx context.Context, endpoint *EngineEndpoint, jobID string) (string, error) {
	if endpoint != nil && strings.EqualFold(strings.TrimSpace(endpoint.APIMode), "v1") {
		return c.getJobLogsV1(ctx, endpoint, jobID)
	}
	targetURL, err := buildEngineURL(endpoint, "/logs/"+neturl.PathEscape(strings.TrimSpace(jobID)), nil)
	if err != nil {
		return "", err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("sync: get job logs failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return c.resolveJobLogPayload(ctx, endpoint, body)
}

// DefaultClusterRuntimeResolver resolves engine endpoint via cluster and host records.
type DefaultClusterRuntimeResolver struct {
	clusterRepo *clusterapp.Repository
	hostRepo    *hostapp.Repository
}

// NewDefaultClusterRuntimeResolver creates a runtime resolver.
func NewDefaultClusterRuntimeResolver(clusterRepo *clusterapp.Repository, hostRepo *hostapp.Repository) *DefaultClusterRuntimeResolver {
	return &DefaultClusterRuntimeResolver{clusterRepo: clusterRepo, hostRepo: hostRepo}
}

// ResolveEngineEndpoint resolves one endpoint from cluster metadata.
func (r *DefaultClusterRuntimeResolver) ResolveEngineEndpoint(ctx context.Context, clusterID uint, taskDefinition JSONMap) (*EngineEndpoint, error) {
	if endpoint := endpointFromTaskDefinition(taskDefinition); endpoint != nil {
		return endpoint, nil
	}
	if r == nil || r.clusterRepo == nil || r.hostRepo == nil {
		return nil, fmt.Errorf("sync: runtime resolver is not configured")
	}
	clusterObj, err := r.clusterRepo.GetByID(ctx, clusterID, true)
	if err != nil {
		return nil, err
	}
	for _, node := range clusterObj.Nodes {
		if node.APIPort <= 0 {
			continue
		}
		hostObj, err := r.hostRepo.GetByID(ctx, node.HostID)
		if err != nil {
			continue
		}
		if hostObj == nil || strings.TrimSpace(hostObj.AgentID) == "" || !hostObj.IsOnline(engineEndpointHeartbeatTimeout) {
			continue
		}
		hostIP := strings.TrimSpace(hostObj.IPAddress)
		if hostIP == "" {
			continue
		}
		endpoint := &EngineEndpoint{BaseURL: fmt.Sprintf("http://%s:%d", hostIP, node.APIPort)}
		if node.HazelcastPort > 0 {
			endpoint.LegacyURL = fmt.Sprintf("http://%s:%d", hostIP, node.HazelcastPort)
		}
		return endpoint, nil
	}
	return nil, fmt.Errorf("sync: no cluster node with api_port configured for cluster %d", clusterID)
}

func endpointFromTaskDefinition(definition JSONMap) *EngineEndpoint {
	if definition == nil {
		return nil
	}
	baseURL := strings.TrimSpace(stringValue(definition, "engine_base_url", "rest_api_base_url"))
	if baseURL == "" {
		return nil
	}
	return &EngineEndpoint{
		BaseURL:     baseURL,
		ContextPath: strings.TrimSpace(stringValue(definition, "engine_context_path", "rest_api_context_path")),
		LegacyURL:   strings.TrimSpace(stringValue(definition, "engine_legacy_base_url", "legacy_rest_api_base_url")),
		APIMode:     strings.TrimSpace(stringValue(definition, "engine_api_mode", "api_mode")),
	}
}

func (c *SeaTunnelEngineClient) trySubmitLegacy(ctx context.Context, req *EngineSubmitRequest, statusCode int, responseBody []byte) (*EngineSubmitResponse, error) {
	if req == nil || req.Endpoint == nil || strings.TrimSpace(req.Endpoint.LegacyURL) == "" {
		return nil, nil
	}
	if !shouldFallbackToLegacy(statusCode, strings.TrimSpace(string(responseBody))) {
		return nil, nil
	}
	if normalizeSubmitFormat(req.Format) != "json" {
		return nil, fmt.Errorf("sync: engine REST v2 is unavailable and legacy REST v1 fallback only supports json payloads")
	}
	return c.submitV1(ctx, req)
}

func shouldFallbackToLegacy(statusCode int, responseBody string) bool {
	body := strings.ToLower(strings.TrimSpace(responseBody))
	if statusCode == http.StatusNotFound && (strings.Contains(body, "no context found") || strings.Contains(body, "404")) {
		return true
	}
	if statusCode == http.StatusBadGateway || statusCode == http.StatusServiceUnavailable || statusCode == http.StatusGatewayTimeout {
		return true
	}
	return false
}

func (c *SeaTunnelEngineClient) submitV1(ctx context.Context, req *EngineSubmitRequest) (*EngineSubmitResponse, error) {
	targetURL, err := buildLegacyEngineURL(req.Endpoint, "/hazelcast/rest/maps/submit-job", map[string]string{
		"jobId":   strings.TrimSpace(req.JobID),
		"jobName": strings.TrimSpace(req.JobName),
	})
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(req.Body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("sync: legacy submit job failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var result EngineSubmitResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	if strings.TrimSpace(result.JobID) == "" {
		result.JobID = strings.TrimSpace(req.JobID)
	}
	if strings.TrimSpace(result.JobName) == "" {
		result.JobName = strings.TrimSpace(req.JobName)
	}
	result.APIMode = "v1"
	result.EndpointBaseURL = strings.TrimSpace(req.Endpoint.LegacyURL)
	return &result, nil
}

func (c *SeaTunnelEngineClient) getJobInfoV1(ctx context.Context, endpoint *EngineEndpoint, jobID string) (*EngineJobInfo, error) {
	targetURL, err := buildLegacyEngineURL(endpoint, "/hazelcast/rest/maps/job-info/"+neturl.PathEscape(strings.TrimSpace(jobID)), nil)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("sync: legacy get job info failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var result EngineJobInfo
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *SeaTunnelEngineClient) stopJobV1(ctx context.Context, endpoint *EngineEndpoint, jobID string, stopWithSavepoint bool) error {
	path := "/hazelcast/rest/maps/cancel-job"
	if stopWithSavepoint {
		path = "/hazelcast/rest/maps/stop-job"
	}
	targetURL, err := buildLegacyEngineURL(endpoint, path, map[string]string{"jobId": strings.TrimSpace(jobID)})
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("sync: legacy stop job failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func (c *SeaTunnelEngineClient) getJobLogsV1(ctx context.Context, endpoint *EngineEndpoint, jobID string) (string, error) {
	targetURL, err := buildLegacyEngineURL(endpoint, "/hazelcast/rest/maps/logs/"+neturl.PathEscape(strings.TrimSpace(jobID)), nil)
	if err != nil {
		return "", err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("sync: legacy get job logs failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return c.resolveJobLogPayload(ctx, endpoint, body)
}

var htmlLogLinkPattern = regexp.MustCompile(`href="([^"]+job-[^"]+\.log[^"]*)"`)

func (c *SeaTunnelEngineClient) resolveJobLogPayload(ctx context.Context, endpoint *EngineEndpoint, body []byte) (string, error) {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return "", nil
	}
	if !looksLikeHTMLLogIndex(trimmed) {
		return formatLogPayload(body), nil
	}
	logURL, ok := extractJobLogURL(endpoint, trimmed)
	if !ok {
		return formatLogPayload(body), nil
	}
	content, err := c.fetchPlainLog(ctx, logURL)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(content) == "" {
		return formatLogPayload(body), nil
	}
	return content, nil
}

func (c *SeaTunnelEngineClient) fetchPlainLog(ctx context.Context, targetURL string) (string, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("sync: fetch job log file failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return strings.TrimSpace(string(body)), nil
}

func looksLikeHTMLLogIndex(body string) bool {
	body = strings.TrimSpace(strings.ToLower(body))
	return strings.Contains(body, "<html") && strings.Contains(body, "<a href=") && strings.Contains(body, ".log")
}

func extractJobLogURL(endpoint *EngineEndpoint, html string) (string, bool) {
	match := htmlLogLinkPattern.FindStringSubmatch(html)
	if len(match) < 2 {
		return "", false
	}
	link := strings.TrimSpace(match[1])
	if link == "" {
		return "", false
	}
	parsed, err := neturl.Parse(link)
	if err != nil {
		return "", false
	}
	if parsed.IsAbs() {
		base := strings.TrimSpace(endpointBaseURL(endpoint))
		if base == "" {
			return parsed.String(), true
		}
		baseURL, err := neturl.Parse(base)
		if err != nil {
			return parsed.String(), true
		}
		parsed.Scheme = baseURL.Scheme
		parsed.Host = baseURL.Host
		return parsed.String(), true
	}
	base := strings.TrimSpace(endpointBaseURL(endpoint))
	if base == "" {
		return "", false
	}
	baseURL, err := neturl.Parse(base)
	if err != nil {
		return "", false
	}
	return baseURL.ResolveReference(parsed).String(), true
}

func endpointBaseURL(endpoint *EngineEndpoint) string {
	if endpoint == nil {
		return ""
	}
	if strings.EqualFold(strings.TrimSpace(endpoint.APIMode), "v1") && strings.TrimSpace(endpoint.LegacyURL) != "" {
		return strings.TrimSpace(endpoint.LegacyURL)
	}
	if strings.TrimSpace(endpoint.BaseURL) != "" {
		return strings.TrimSpace(endpoint.BaseURL)
	}
	return strings.TrimSpace(endpoint.LegacyURL)
}

func formatLogPayload(body []byte) string {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return ""
	}
	var pretty interface{}
	if json.Unmarshal(body, &pretty) == nil {
		formatted, err := json.MarshalIndent(pretty, "", "  ")
		if err == nil {
			return string(formatted)
		}
	}
	return trimmed
}

func buildLegacyEngineURL(endpoint *EngineEndpoint, path string, params map[string]string) (string, error) {
	if endpoint == nil || strings.TrimSpace(endpoint.LegacyURL) == "" {
		return "", fmt.Errorf("sync: legacy engine base url is required")
	}
	legacyEndpoint := &EngineEndpoint{BaseURL: endpoint.LegacyURL}
	return buildEngineURL(legacyEndpoint, path, params)
}

func buildEngineURL(endpoint *EngineEndpoint, path string, params map[string]string) (string, error) {
	if endpoint == nil || strings.TrimSpace(endpoint.BaseURL) == "" {
		return "", fmt.Errorf("sync: engine base url is required")
	}
	baseURL := strings.TrimRight(strings.TrimSpace(endpoint.BaseURL), "/")
	contextPath := strings.TrimSpace(endpoint.ContextPath)
	if contextPath != "" {
		if !strings.HasPrefix(contextPath, "/") {
			contextPath = "/" + contextPath
		}
		baseURL += strings.TrimRight(contextPath, "/")
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	parsed, err := neturl.Parse(baseURL + path)
	if err != nil {
		return "", err
	}
	if len(params) > 0 {
		query := parsed.Query()
		for key, value := range params {
			if strings.TrimSpace(value) == "" {
				continue
			}
			query.Set(key, value)
		}
		parsed.RawQuery = query.Encode()
	}
	return parsed.String(), nil
}

func normalizeSubmitFormat(format string) string {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "json":
		return "json"
	case "sql":
		return "sql"
	case "hocon", "":
		return "hocon"
	default:
		return "hocon"
	}
}

func normalizeJobStatus(status string) JobStatus {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "RUNNING":
		return JobStatusRunning
	case "DOING_SAVEPOINT":
		return JobStatusRunning
	case "SAVEPOINT_DONE":
		return JobStatusSuccess
	case "FINISHED", "SUCCESS":
		return JobStatusSuccess
	case "FAILING", "FAILED":
		return JobStatusFailed
	case "CANCELED", "CANCELLED", "CANCELING":
		return JobStatusCanceled
	case "CREATED", "STARTING", "SCHEDULED", "SUBMITTED", "PENDING":
		return JobStatusPending
	default:
		return JobStatusRunning
	}
}

func safeParseInt64(value string) int64 {
	parsed, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return 0
	}
	return parsed
}
