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
	version    string
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
	// Tasks/Relations 用原始 JSON 回写：PUT 会整图覆盖，类型化结构会丢掉未知字段。
	// Tasks/Relations stay raw JSON because PUT replaces the whole graph and typed structs drop unknown fields.
	Tasks     []json.RawMessage
	Relations []json.RawMessage
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
	IsCache               string                 `json:"isCache"`
	TaskExecuteType       string                 `json:"taskExecuteType"`
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
	version := strings.TrimSpace(conn.DolphinVersion)
	if style == SchedulerAPIStyleAuto {
		if family, err := dolphinAPIFamilyFromVersion(version); err == nil && family != "" {
			style = family
		}
	}
	return &DolphinClient{
		baseURL:    strings.TrimRight(strings.TrimSpace(conn.BaseURL), "/"),
		token:      strings.TrimSpace(conn.Token),
		style:      style,
		version:    version,
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
	if family, err := dolphinAPIFamilyFromVersion(c.version); err != nil {
		return err
	} else if family != "" {
		c.style = family
		return nil
	}
	// 先探 3.3+ 的 workflow-definition，再回退 3.1/3.2 的 process-definition。
	// Probe 3.3+ workflow-definition first, then fall back to 3.1/3.2 process-definition.
	_, workflowErr := c.do(ctx, http.MethodGet, fmt.Sprintf("/projects/%d/workflow-definition?pageNo=1&pageSize=1", projectCode), nil)
	if workflowErr == nil {
		c.style = SchedulerAPIStyleWorkflow
		return nil
	}
	_, processErr := c.do(ctx, http.MethodGet, fmt.Sprintf("/projects/%d/process-definition?pageNo=1&pageSize=1", projectCode), nil)
	if processErr == nil {
		c.style = SchedulerAPIStyleProcess
		return nil
	}
	return fmt.Errorf("%w: neither workflow-definition (3.3+) nor process-definition (3.1/3.2) is available", ErrDolphinUnsupportedVersion)
}

func (c *DolphinClient) ListProjects(ctx context.Context) ([]DolphinProject, error) {
	raw, err := c.do(ctx, http.MethodGet, "/projects?pageNo=1&pageSize=100", nil)
	if err != nil {
		return nil, err
	}
	if looksLikeDolphinV2Projects(raw) {
		return nil, fmt.Errorf("%w: project list uses id instead of code", ErrDolphinUnsupportedVersion)
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
	if err := c.ensureStyle(ctx, projectCode); err != nil {
		return err
	}
	form := url.Values{}
	code := strconv.FormatInt(workflowCode, 10)
	// 3.1/3.2 必填 processDefinitionCode；3.3+ 必填 workflowDefinitionCode。
	// 3.1/3.2 require processDefinitionCode; 3.3+ require workflowDefinitionCode.
	if c.style == SchedulerAPIStyleWorkflow {
		form.Set("workflowDefinitionCode", code)
	} else {
		form.Set("processDefinitionCode", code)
	}
	form.Set("schedule", fmt.Sprintf(`{"startTime":"2000-01-01 00:00:00","endTime":"2099-12-31 23:59:59","crontab":%q,"timezoneId":%q}`, crontab, timezone))
	_, err := c.doForm(ctx, http.MethodPost, fmt.Sprintf("/projects/%d/schedules", projectCode), form)
	return err
}

// ListSchedules 列出工作流的定时及其上下线状态。成功响应但无法解析时视为没有定时（与旧行为一致）；请求失败返回错误。
// ListSchedules lists a workflow's schedules with their release states. A successful but unparsable response counts
// as no schedules (as before); a failed request returns an error.
func (c *DolphinClient) ListSchedules(ctx context.Context, projectCode, workflowCode int64) ([]DolphinScheduleInfo, error) {
	if err := c.ensureStyle(ctx, projectCode); err != nil {
		return nil, err
	}
	codeField := "processDefinitionCode"
	if c.style == SchedulerAPIStyleWorkflow {
		codeField = "workflowDefinitionCode"
	}
	query := fmt.Sprintf("/projects/%d/schedules?%s=%d&pageNo=1&pageSize=50", projectCode, codeField, workflowCode)
	raw, err := c.do(ctx, http.MethodGet, query, nil)
	if err != nil {
		return nil, err
	}
	var page struct {
		TotalList []struct {
			ID           int64  `json:"id"`
			Crontab      string `json:"crontab"`
			TimezoneID   string `json:"timezoneId"`
			ReleaseState string `json:"releaseState"`
		} `json:"totalList"`
	}
	schedules := make([]DolphinScheduleInfo, 0)
	if json.Unmarshal(raw, &page) != nil {
		return schedules, nil
	}
	for _, item := range page.TotalList {
		schedules = append(schedules, DolphinScheduleInfo{
			ID:           item.ID,
			Crontab:      strings.TrimSpace(item.Crontab),
			Timezone:     strings.TrimSpace(item.TimezoneID),
			ReleaseState: strings.ToUpper(strings.TrimSpace(item.ReleaseState)),
		})
	}
	return schedules, nil
}

// OnlineSchedule 上线一条定时。
// OnlineSchedule brings one schedule online.
func (c *DolphinClient) OnlineSchedule(ctx context.Context, projectCode, scheduleID int64) error {
	_, err := c.doForm(ctx, http.MethodPost, fmt.Sprintf("/projects/%d/schedules/%d/online", projectCode, scheduleID), url.Values{})
	return err
}

// OnlineSchedules 上线工作流的全部定时；只用于 STX 新建的工作流（其定时都由本次推送创建）。复用 / 更新已有工作流时
// 必须按推送前状态恢复（见 restoreOnlineSchedules），不能调用本方法，否则会把用户暂停的定时重新打开。
// OnlineSchedules brings every schedule of a workflow online; only for workflows created by STX (all their schedules
// come from this push). Reuse / update of an existing workflow must restore the pre-push states instead (see
// restoreOnlineSchedules); calling this there would re-enable schedules the user had paused.
func (c *DolphinClient) OnlineSchedules(ctx context.Context, projectCode, workflowCode int64) error {
	schedules, err := c.ListSchedules(ctx, projectCode, workflowCode)
	if err != nil {
		return err
	}
	for _, item := range schedules {
		if err := c.OnlineSchedule(ctx, projectCode, item.ID); err != nil {
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
		// 3.2 对不存在的 workflow-definition 常返回 200 HTML 控制台页，不能当成探测成功。
		// 3.2 often returns the 200 HTML console for missing workflow-definition; that is not a probe hit.
		trimmed := strings.TrimSpace(string(content))
		if status >= 200 && status < 300 && looksLikeJSON(trimmed) {
			return content, nil
		}
		snippet := trimmed
		if len(snippet) > 180 {
			snippet = snippet[:180]
		}
		return nil, fmt.Errorf("sync: dolphin http %d: %s", status, snippet)
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

// looksLikeJSON 判断响应是不是 JSON 对象/数组，用来挡住 3.2 的 HTML 回退页。
// looksLikeJSON reports whether the body is a JSON object or array so 3.2 HTML fallback pages are rejected.
func looksLikeJSON(raw string) bool {
	if raw == "" {
		return false
	}
	return raw[0] == '{' || raw[0] == '['
}

func looksLikeDolphinV2Projects(raw json.RawMessage) bool {
	type legacyItem struct {
		ID   int64  `json:"id"`
		Code int64  `json:"code"`
		Name string `json:"name"`
	}
	hasLegacyID := func(items []legacyItem) bool {
		if len(items) == 0 {
			return false
		}
		for _, item := range items {
			if item.ID != 0 && item.Code == 0 {
				return true
			}
		}
		return false
	}
	var page struct {
		TotalList []legacyItem `json:"totalList"`
	}
	if json.Unmarshal(raw, &page) == nil && hasLegacyID(page.TotalList) {
		return true
	}
	var list []legacyItem
	return json.Unmarshal(raw, &list) == nil && hasLegacyID(list)
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
		ProcessDefinition        json.RawMessage `json:"processDefinition"`
		WorkflowDefinition       json.RawMessage `json:"workflowDefinition"`
		ProcessTaskRelationList  json.RawMessage `json:"processTaskRelationList"`
		WorkflowTaskRelationList json.RawMessage `json:"workflowTaskRelationList"`
		TaskDefinitionList       json.RawMessage `json:"taskDefinitionList"`
		Locations                json.RawMessage `json:"locations"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	processMeta := decodeDefinitionMeta(payload.ProcessDefinition)
	workflowMeta := decodeDefinitionMeta(payload.WorkflowDefinition)
	meta := processMeta
	if meta.Code == 0 && workflowMeta.Code != 0 {
		meta = workflowMeta
	} else if meta.Code == 0 {
		// 3.3 只有 workflowDefinition；若 process 解码为空对象，仍用 workflow 元数据。
		// 3.3 only has workflowDefinition; use that metadata when process is empty.
		if workflowMeta.Name != "" || workflowMeta.Version != 0 || workflowMeta.ReleaseState != "" {
			meta = workflowMeta
		}
	}
	relations := decodeRawArray(payload.ProcessTaskRelationList)
	if len(relations) == 0 {
		relations = decodeRawArray(payload.WorkflowTaskRelationList)
	}
	graph := &dolphinGraph{
		Code:          meta.Code,
		Name:          meta.Name,
		Version:       meta.Version,
		ReleaseState:  meta.ReleaseState,
		TenantCode:    meta.TenantCode,
		ExecutionType: meta.ExecutionType,
		Description:   meta.Description,
		GlobalParams:  decodeFlexibleJSONString(meta.GlobalParams),
		Timeout:       meta.Timeout,
		Tasks:         decodeRawArray(payload.TaskDefinitionList),
		Relations:     relations,
		// 3.2 GET 把 locations 放在 processDefinition 里，根上经常没有。
		// 3.2 GET stores locations on processDefinition; the root payload often omits them.
		Locations: firstNonEmptyLocations(
			decodeFlexibleJSONString(payload.Locations),
			decodeFlexibleJSONString(processMeta.Locations),
			decodeFlexibleJSONString(workflowMeta.Locations),
		),
	}
	return graph, nil
}

func encodeDefinitionForm(graph *dolphinGraph) url.Values {
	form := url.Values{}
	form.Set("name", graph.Name)
	form.Set("description", graph.Description)
	form.Set("globalParams", defaultString(graph.GlobalParams, "[]"))
	// 3.2 UI 使用 locations 数组；已有工作流必须原样带回，不能默认成 {} 清空画布。
	// 3.2 UI uses a locations array; existing workflows must round-trip it, never default to {}.
	form.Set("locations", graph.Locations)
	form.Set("timeout", strconv.Itoa(graph.Timeout))
	form.Set("tenantCode", defaultString(graph.TenantCode, "default"))
	form.Set("executionType", defaultString(graph.ExecutionType, "PARALLEL"))
	tasks := graph.Tasks
	if tasks == nil {
		tasks = []json.RawMessage{}
	}
	relations := graph.Relations
	if relations == nil {
		relations = []json.RawMessage{}
	}
	taskJSON, _ := json.Marshal(tasks)
	relJSON, _ := json.Marshal(relations)
	form.Set("taskDefinitionJson", string(taskJSON))
	form.Set("taskRelationJson", string(relJSON))
	return form
}

// ResolveEnvironmentCode 按环境名查找 Dolphin environment code；找不到时返回 -1。
// ResolveEnvironmentCode looks up a Dolphin environment code by name; missing names return -1.
func (c *DolphinClient) ResolveEnvironmentCode(ctx context.Context, name string) (int64, error) {
	name = strings.TrimSpace(name)
	if c == nil || name == "" {
		return -1, nil
	}
	// Soft-fail: missing/unsupported environment APIs keep EnvironmentCode=-1 so push still works.
	// 软失败：环境列表不可用时保持 -1，推送本身仍可继续。
	raw, err := c.do(ctx, http.MethodGet, "/environment/query-environment-list", nil)
	if err != nil {
		return -1, nil
	}
	var items []struct {
		Code int64  `json:"code"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(raw, &items); err != nil {
		return -1, nil
	}
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item.Name), name) && item.Code != 0 {
			return item.Code, nil
		}
	}
	return -1, nil
}

func buildShellTask(code int64, name, script, workerGroup string, environmentCode int64) dolphinTaskDef {
	if environmentCode == 0 {
		environmentCode = -1
	}
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
		Flag:            "YES",
		TaskPriority:    "MEDIUM",
		WorkerGroup:     defaultString(workerGroup, defaultDolphinWorkerGroup),
		EnvironmentCode: environmentCode,
		// SHELL runs `stx … --wait`; a DS-level retry re-runs a long CLI and
		// costs ~5m of wall clock on every intentional engine failure. Leave
		// retries to the operator in Dolphin UI if they want them.
		FailRetryTimes:    0,
		FailRetryInterval: 1,
		// 新建节点关闭 Dolphin 超时，时长由作业自己结束或调度侧手动停。
		// New nodes leave Dolphin timeout off; duration is the job itself or an operator stop.
		TimeoutFlag:           "CLOSE",
		TimeoutNotifyStrategy: "",
		Timeout:               0,
		DelayTime:             0,
		CpuQuota:              -1,
		MemoryMax:             -1,
		// 3.2.x 缺 isCache / taskExecuteType 时 transformTask 会 NPE，创建工作流失败。
		// 3.2.x NPEs in transformTask when isCache / taskExecuteType are omitted, so create fails.
		IsCache:         "NO",
		TaskExecuteType: "BATCH",
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

func dolphinConsoleURL(baseURL string, projectCode, workflowCode int64, style, version string) string {
	trimmed := strings.TrimRight(baseURL, "/")
	trimmed = strings.TrimSuffix(trimmed, "/dolphinscheduler")
	origin := trimmed + "/dolphinscheduler/ui"
	major, minor, _, _ := parseDolphinVersion(version)
	// 3.0/3.1 控制台仍是 hash 的 process-definition；3.2 起 UI 已改成 history 的 workflow-definition，尽管 API 在 3.3 前仍是 process-definition。
	// 3.0/3.1 console still uses hash process-definition; from 3.2 the UI is history workflow-definition even while the API stays process-definition until 3.3.
	useHashProcess := style == SchedulerAPIStyleProcess && major == 3 && minor < 2
	if useHashProcess {
		return fmt.Sprintf("%s/#/projects/%d/process-definition/list/%d", origin, projectCode, workflowCode)
	}
	return fmt.Sprintf("%s/projects/%d/workflow-definition/%d", origin, projectCode, workflowCode)
}
