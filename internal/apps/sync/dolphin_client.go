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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	dolphinReleaseOnline  = "ONLINE"
	dolphinReleaseOffline = "OFFLINE"
)

// DolphinClient 按版本切换 process-definition / workflow-definition 路径访问 Dolphin API。
// DolphinClient talks to DolphinScheduler and switches process/workflow paths by version.
type DolphinClient struct {
	baseURL    string
	token      string
	style      string
	httpClient *http.Client
}

type dolphinEnvelope struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

type dolphinGraph struct {
	Code          int64
	Name          string
	Version       int
	ReleaseState  string
	TenantCode    string
	ExecutionType string
	Description   string
	GlobalParams  string
	Locations     string
	Timeout       int
	Tasks         []dolphinTaskDef
	Relations     []dolphinRelation
}

type dolphinTaskDef struct {
	Code                  int64                  `json:"code"`
	Name                  string                 `json:"name"`
	Version               int                    `json:"version"`
	Description           string                 `json:"description"`
	TaskType              string                 `json:"taskType"`
	TaskParams            map[string]interface{} `json:"taskParams"`
	Flag                  string                 `json:"flag"`
	TaskPriority          string                 `json:"taskPriority"`
	WorkerGroup           string                 `json:"workerGroup"`
	EnvironmentCode       int64                  `json:"environmentCode"`
	FailRetryTimes        int                    `json:"failRetryTimes"`
	FailRetryInterval     int                    `json:"failRetryInterval"`
	TimeoutFlag           string                 `json:"timeoutFlag"`
	TimeoutNotifyStrategy string                 `json:"timeoutNotifyStrategy"`
	Timeout               int                    `json:"timeout"`
	DelayTime             int                    `json:"delayTime"`
	CpuQuota              int                    `json:"cpuQuota"`
	MemoryMax             int                    `json:"memoryMax"`
}

type dolphinRelation struct {
	Name            string      `json:"name"`
	PreTaskCode     int64       `json:"preTaskCode"`
	PreTaskVersion  int         `json:"preTaskVersion"`
	PostTaskCode    int64       `json:"postTaskCode"`
	PostTaskVersion int         `json:"postTaskVersion"`
	ConditionType   string      `json:"conditionType"`
	ConditionParams interface{} `json:"conditionParams"`
}

func newDolphinClient(conn *SchedulerConnection, httpClient *http.Client) *DolphinClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	style := strings.TrimSpace(conn.APIStyle)
	if style == "" {
		style = SchedulerAPIStyleAuto
	}
	return &DolphinClient{
		baseURL:    strings.TrimRight(strings.TrimSpace(conn.BaseURL), "/"),
		token:      strings.TrimSpace(conn.Token),
		style:      style,
		httpClient: httpClient,
	}
}

func (c *DolphinClient) definitionPath() string {
	if c.style == SchedulerAPIStyleWorkflow {
		return "workflow-definition"
	}
	return "process-definition"
}

func (c *DolphinClient) ensureStyle(ctx context.Context, projectCode int64) error {
	if c.style == SchedulerAPIStyleProcess || c.style == SchedulerAPIStyleWorkflow {
		return nil
	}
	_, err := c.do(ctx, http.MethodGet, fmt.Sprintf("/projects/%d/workflow-definition?pageNo=1&pageSize=1", projectCode), nil)
	if err == nil {
		c.style = SchedulerAPIStyleWorkflow
		return nil
	}
	_, err = c.do(ctx, http.MethodGet, fmt.Sprintf("/projects/%d/process-definition?pageNo=1&pageSize=1", projectCode), nil)
	if err == nil {
		c.style = SchedulerAPIStyleProcess
		return nil
	}
	return err
}

func (c *DolphinClient) ListProjects(ctx context.Context) ([]DolphinProject, error) {
	raw, err := c.do(ctx, http.MethodGet, "/projects?pageNo=1&pageSize=100", nil)
	if err != nil {
		return nil, err
	}
	return decodeNamedCodeList(raw), nil
}

func (c *DolphinClient) ListWorkflows(ctx context.Context, projectCode int64) ([]DolphinWorkflowSummary, error) {
	if err := c.ensureStyle(ctx, projectCode); err != nil {
		return nil, err
	}
	raw, err := c.do(ctx, http.MethodGet, fmt.Sprintf("/projects/%d/%s?pageNo=1&pageSize=100", projectCode, c.definitionPath()), nil)
	if err != nil {
		return nil, err
	}
	items := decodeNamedCodeList(raw)
	result := make([]DolphinWorkflowSummary, 0, len(items))
	for _, item := range items {
		result = append(result, DolphinWorkflowSummary{Code: item.Code, Name: item.Name})
	}
	var page struct {
		TotalList []struct {
			Code         int64  `json:"code"`
			Name         string `json:"name"`
			ReleaseState string `json:"releaseState"`
			Version      int    `json:"version"`
		} `json:"totalList"`
	}
	if json.Unmarshal(raw, &page) == nil {
		for i := range result {
			if i < len(page.TotalList) {
				result[i].ReleaseState = page.TotalList[i].ReleaseState
				result[i].Version = page.TotalList[i].Version
			}
		}
	}
	return result, nil
}

func (c *DolphinClient) GetDefinition(ctx context.Context, projectCode, workflowCode int64) (*dolphinGraph, error) {
	if err := c.ensureStyle(ctx, projectCode); err != nil {
		return nil, err
	}
	raw, err := c.do(ctx, http.MethodGet, fmt.Sprintf("/projects/%d/%s/%d", projectCode, c.definitionPath(), workflowCode), nil)
	if err != nil {
		return nil, err
	}
	return decodeDolphinGraph(raw)
}

func (c *DolphinClient) GenTaskCodes(ctx context.Context, projectCode int64, n int) ([]int64, error) {
	if n <= 0 {
		n = 1
	}
	raw, err := c.do(ctx, http.MethodGet, fmt.Sprintf("/projects/%d/task-definition/gen-task-codes?genNum=%d", projectCode, n), nil)
	if err != nil {
		return nil, err
	}
	var codes []int64
	if json.Unmarshal(raw, &codes) == nil && len(codes) > 0 {
		return codes, nil
	}
	var wrapped struct {
		Codes []int64 `json:"codes"`
		Data  []int64 `json:"data"`
	}
	if json.Unmarshal(raw, &wrapped) == nil {
		if len(wrapped.Codes) > 0 {
			return wrapped.Codes, nil
		}
		if len(wrapped.Data) > 0 {
			return wrapped.Data, nil
		}
	}
	return nil, fmt.Errorf("sync: dolphin gen-task-codes returned unexpected payload")
}

func (c *DolphinClient) CreateDefinition(ctx context.Context, projectCode int64, graph *dolphinGraph) (*dolphinGraph, error) {
	if err := c.ensureStyle(ctx, projectCode); err != nil {
		return nil, err
	}
	raw, err := c.doForm(ctx, http.MethodPost, fmt.Sprintf("/projects/%d/%s", projectCode, c.definitionPath()), encodeDefinitionForm(graph))
	if err != nil {
		return nil, err
	}
	if created, decodeErr := decodeDolphinGraph(raw); decodeErr == nil && created != nil && created.Code > 0 {
		return created, nil
	}
	var code int64
	if json.Unmarshal(raw, &code) == nil && code > 0 {
		graph.Code = code
		return graph, nil
	}
	var obj struct {
		Code int64 `json:"code"`
	}
	if json.Unmarshal(raw, &obj) == nil && obj.Code > 0 {
		graph.Code = obj.Code
	}
	return graph, nil
}

func (c *DolphinClient) UpdateDefinition(ctx context.Context, projectCode int64, graph *dolphinGraph) error {
	if err := c.ensureStyle(ctx, projectCode); err != nil {
		return err
	}
	_, err := c.doForm(ctx, http.MethodPut, fmt.Sprintf("/projects/%d/%s/%d", projectCode, c.definitionPath(), graph.Code), encodeDefinitionForm(graph))
	return err
}

func (c *DolphinClient) Release(ctx context.Context, projectCode, workflowCode int64, state string) error {
	if err := c.ensureStyle(ctx, projectCode); err != nil {
		return err
	}
	form := url.Values{}
	form.Set("releaseState", state)
	_, err := c.doForm(ctx, http.MethodPost, fmt.Sprintf("/projects/%d/%s/%d/release", projectCode, c.definitionPath(), workflowCode), form)
	return err
}

func (c *DolphinClient) CreateSchedule(ctx context.Context, projectCode, workflowCode int64, crontab, timezone string) error {
	form := url.Values{}
	code := strconv.FormatInt(workflowCode, 10)
	form.Set("processDefinitionCode", code)
	form.Set("workflowDefinitionCode", code)
	form.Set("schedule", fmt.Sprintf(`{"startTime":"2000-01-01 00:00:00","endTime":"2099-12-31 23:59:59","crontab":%q,"timezoneId":%q}`, crontab, timezone))
	_, err := c.doForm(ctx, http.MethodPost, fmt.Sprintf("/projects/%d/schedules", projectCode), form)
	return err
}

func (c *DolphinClient) OnlineSchedules(ctx context.Context, projectCode, workflowCode int64) error {
	query := fmt.Sprintf("/projects/%d/schedules?processDefinitionCode=%d&workflowDefinitionCode=%d&pageNo=1&pageSize=20", projectCode, workflowCode, workflowCode)
	raw, err := c.do(ctx, http.MethodGet, query, nil)
	if err != nil {
		return err
	}
	var page struct {
		TotalList []struct {
			ID int64 `json:"id"`
		} `json:"totalList"`
	}
	if json.Unmarshal(raw, &page) != nil {
		return nil
	}
	for _, item := range page.TotalList {
		if _, err := c.doForm(ctx, http.MethodPost, fmt.Sprintf("/projects/%d/schedules/%d/online", projectCode, item.ID), url.Values{}); err != nil {
			return err
		}
	}
	return nil
}

func (c *DolphinClient) do(ctx context.Context, method, path string, body io.Reader) (json.RawMessage, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("token", c.token)
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return decodeDolphinEnvelope(resp.StatusCode, content)
}

func (c *DolphinClient) doForm(ctx context.Context, method, path string, form url.Values) (json.RawMessage, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("token", c.token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return decodeDolphinEnvelope(resp.StatusCode, content)
}

func decodeDolphinEnvelope(status int, content []byte) (json.RawMessage, error) {
	var envelope dolphinEnvelope
	if err := json.Unmarshal(content, &envelope); err != nil {
		if status >= 200 && status < 300 {
			return content, nil
		}
		return nil, fmt.Errorf("sync: dolphin http %d: %s", status, strings.TrimSpace(string(content)))
	}
	if envelope.Code != 0 {
		msg := strings.TrimSpace(envelope.Msg)
		if msg == "" {
			msg = fmt.Sprintf("dolphin code %d", envelope.Code)
		}
		return nil, fmt.Errorf("sync: %s", msg)
	}
	return envelope.Data, nil
}

func decodeNamedCodeList(raw json.RawMessage) []DolphinProject {
	var page struct {
		TotalList []struct {
			Code int64  `json:"code"`
			Name string `json:"name"`
		} `json:"totalList"`
	}
	if json.Unmarshal(raw, &page) == nil && len(page.TotalList) > 0 {
		items := make([]DolphinProject, 0, len(page.TotalList))
		for _, item := range page.TotalList {
			items = append(items, DolphinProject{Code: item.Code, Name: item.Name})
		}
		return items
	}
	var list []struct {
		Code int64  `json:"code"`
		Name string `json:"name"`
	}
	if json.Unmarshal(raw, &list) == nil {
		items := make([]DolphinProject, 0, len(list))
		for _, item := range list {
			items = append(items, DolphinProject{Code: item.Code, Name: item.Name})
		}
		return items
	}
	return []DolphinProject{}
}

func decodeDolphinGraph(raw json.RawMessage) (*dolphinGraph, error) {
	var payload struct {
		ProcessDefinition *struct {
			Code          int64  `json:"code"`
			Name          string `json:"name"`
			Version       int    `json:"version"`
			ReleaseState  string `json:"releaseState"`
			TenantCode    string `json:"tenantCode"`
			ExecutionType string `json:"executionType"`
			Description   string `json:"description"`
			GlobalParams  string `json:"globalParams"`
			Timeout       int    `json:"timeout"`
		} `json:"processDefinition"`
		WorkflowDefinition *struct {
			Code          int64  `json:"code"`
			Name          string `json:"name"`
			Version       int    `json:"version"`
			ReleaseState  string `json:"releaseState"`
			TenantCode    string `json:"tenantCode"`
			ExecutionType string `json:"executionType"`
			Description   string `json:"description"`
			GlobalParams  string `json:"globalParams"`
			Timeout       int    `json:"timeout"`
		} `json:"workflowDefinition"`
		ProcessTaskRelationList []dolphinRelation `json:"processTaskRelationList"`
		TaskDefinitionList      []dolphinTaskDef  `json:"taskDefinitionList"`
		Locations               json.RawMessage   `json:"locations"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	def := payload.ProcessDefinition
	if def == nil {
		def = payload.WorkflowDefinition
	}
	graph := &dolphinGraph{Tasks: payload.TaskDefinitionList, Relations: payload.ProcessTaskRelationList}
	if graph.Tasks == nil {
		graph.Tasks = []dolphinTaskDef{}
	}
	if graph.Relations == nil {
		graph.Relations = []dolphinRelation{}
	}
	if def != nil {
		graph.Code = def.Code
		graph.Name = def.Name
		graph.Version = def.Version
		graph.ReleaseState = def.ReleaseState
		graph.TenantCode = def.TenantCode
		graph.ExecutionType = def.ExecutionType
		graph.Description = def.Description
		graph.GlobalParams = def.GlobalParams
		graph.Timeout = def.Timeout
	}
	if len(payload.Locations) > 0 {
		graph.Locations = strings.TrimSpace(string(payload.Locations))
		if strings.HasPrefix(graph.Locations, `"`) {
			var asString string
			if json.Unmarshal(payload.Locations, &asString) == nil {
				graph.Locations = asString
			}
		}
	}
	return graph, nil
}

func encodeDefinitionForm(graph *dolphinGraph) url.Values {
	form := url.Values{}
	form.Set("name", graph.Name)
	form.Set("description", graph.Description)
	form.Set("globalParams", defaultString(graph.GlobalParams, "[]"))
	form.Set("locations", defaultString(graph.Locations, "{}"))
	form.Set("timeout", strconv.Itoa(graph.Timeout))
	form.Set("tenantCode", defaultString(graph.TenantCode, "default"))
	form.Set("executionType", defaultString(graph.ExecutionType, "PARALLEL"))
	taskJSON, _ := json.Marshal(graph.Tasks)
	relJSON, _ := json.Marshal(graph.Relations)
	form.Set("taskDefinitionJson", string(taskJSON))
	form.Set("taskRelationJson", string(relJSON))
	return form
}

func dolphinHasCycle(relations []dolphinRelation, extra [][2]int64) bool {
	edges := map[int64][]int64{}
	nodes := map[int64]struct{}{}
	add := func(from, to int64) {
		if to == 0 {
			return
		}
		nodes[to] = struct{}{}
		if from == 0 {
			return
		}
		nodes[from] = struct{}{}
		edges[from] = append(edges[from], to)
	}
	for _, rel := range relations {
		add(rel.PreTaskCode, rel.PostTaskCode)
	}
	for _, edge := range extra {
		add(edge[0], edge[1])
	}
	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := map[int64]int{}
	var visit func(int64) bool
	visit = func(node int64) bool {
		color[node] = gray
		for _, next := range edges[node] {
			switch color[next] {
			case gray:
				return true
			case white:
				if visit(next) {
					return true
				}
			}
		}
		color[node] = black
		return false
	}
	for node := range nodes {
		if color[node] == white && visit(node) {
			return true
		}
	}
	return false
}

func buildShellTask(code int64, name, script, workerGroup string, timeoutMinutes int) dolphinTaskDef {
	return dolphinTaskDef{
		Code:        code,
		Name:        name,
		Version:     1,
		Description: "STX sync task",
		TaskType:    "SHELL",
		TaskParams: map[string]interface{}{
			"resourceList": []any{},
			"localParams":  []any{},
			"rawScript":    script,
			"dependence":   map[string]any{},
			"conditionResult": map[string]any{
				"successNode": []any{""},
				"failedNode":  []any{""},
			},
			"waitStartTimeout": map[string]any{},
			"switchResult":     map[string]any{},
		},
		Flag:                  "YES",
		TaskPriority:          "MEDIUM",
		WorkerGroup:           defaultString(workerGroup, defaultDolphinWorkerGroup),
		EnvironmentCode:       -1,
		FailRetryTimes:        1,
		FailRetryInterval:     5,
		TimeoutFlag:           "OPEN",
		TimeoutNotifyStrategy: "FAILED",
		Timeout:               timeoutMinutes,
		DelayTime:             0,
		CpuQuota:              -1,
		MemoryMax:             -1,
	}
}

func startRelation(task dolphinTaskDef) dolphinRelation {
	return dolphinRelation{
		PreTaskCode:     0,
		PreTaskVersion:  0,
		PostTaskCode:    task.Code,
		PostTaskVersion: task.Version,
		ConditionType:   "NONE",
		ConditionParams: map[string]any{},
	}
}

func edgeRelation(fromCode int64, fromVersion int, to dolphinTaskDef) dolphinRelation {
	if fromVersion <= 0 {
		fromVersion = 1
	}
	return dolphinRelation{
		PreTaskCode:     fromCode,
		PreTaskVersion:  fromVersion,
		PostTaskCode:    to.Code,
		PostTaskVersion: to.Version,
		ConditionType:   "NONE",
		ConditionParams: map[string]any{},
	}
}

func dolphinConsoleURL(baseURL string, projectCode, workflowCode int64) string {
	trimmed := strings.TrimRight(baseURL, "/")
	trimmed = strings.TrimSuffix(trimmed, "/dolphinscheduler")
	return fmt.Sprintf("%s/dolphinscheduler/ui/projects/%d/workflow/definitions/%d", trimmed, projectCode, workflowCode)
}

func taskVersionByCode(tasks []dolphinTaskDef, code int64) int {
	for _, task := range tasks {
		if task.Code == code {
			if task.Version > 0 {
				return task.Version
			}
			return 1
		}
	}
	return 1
}
