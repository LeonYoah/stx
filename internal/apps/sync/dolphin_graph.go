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
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// dolphinTaskPeek 只读取回写时需要的任务标识，完整节点 JSON 仍走 RawMessage。
// dolphinTaskPeek reads identifiers needed for a write-back; the full node stays RawMessage.
type dolphinTaskPeek struct {
	Code       int64           `json:"code"`
	Name       string          `json:"name"`
	Version    int             `json:"version"`
	TaskType   string          `json:"taskType"`
	TaskParams json.RawMessage `json:"taskParams"`
}

// dolphinRelationPeek 只读取边的 code/version，未知字段仍保留在原始 JSON。
// dolphinRelationPeek reads edge codes/versions; unknown fields stay on the original JSON.
type dolphinRelationPeek struct {
	PreTaskCode     int64 `json:"preTaskCode"`
	PreTaskVersion  int   `json:"preTaskVersion"`
	PostTaskCode    int64 `json:"postTaskCode"`
	PostTaskVersion int   `json:"postTaskVersion"`
}

type dolphinDefinitionMeta struct {
	Code          int64           `json:"code"`
	Name          string          `json:"name"`
	Version       int             `json:"version"`
	ReleaseState  string          `json:"releaseState"`
	TenantCode    string          `json:"tenantCode"`
	ExecutionType string          `json:"executionType"`
	Description   string          `json:"description"`
	GlobalParams  json.RawMessage `json:"globalParams"`
	Timeout       int             `json:"timeout"`
	Locations     json.RawMessage `json:"locations"`
}

func marshalRaw(value any) json.RawMessage {
	encoded, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage("null")
	}
	return encoded
}

func decodeRawArray(raw json.RawMessage) []json.RawMessage {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return []json.RawMessage{}
	}
	var items []json.RawMessage
	if err := json.Unmarshal(trimmed, &items); err != nil || items == nil {
		return []json.RawMessage{}
	}
	return items
}

func decodeDefinitionMeta(raw json.RawMessage) dolphinDefinitionMeta {
	var meta dolphinDefinitionMeta
	if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return meta
	}
	_ = json.Unmarshal(raw, &meta)
	return meta
}

// decodeFlexibleJSONString 接受 JSON 字符串、对象或数组，回写成可放入 form 的文本。
// decodeFlexibleJSONString accepts a JSON string, object, or array and returns form text.
func decodeFlexibleJSONString(raw json.RawMessage) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return ""
	}
	if trimmed[0] == '"' {
		var asString string
		if json.Unmarshal(trimmed, &asString) == nil {
			return strings.TrimSpace(asString)
		}
	}
	return string(trimmed)
}

func locationsEmpty(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	return trimmed == "" || trimmed == "null" || trimmed == "{}" || trimmed == "[]"
}

// firstNonEmptyLocations 优先用当前 GET 的坐标；下线后若被清空则回退到下线前那份。
// firstNonEmptyLocations prefers the latest GET locations and falls back to the pre-OFFLINE copy.
func firstNonEmptyLocations(candidates ...string) string {
	for _, candidate := range candidates {
		if !locationsEmpty(candidate) {
			return candidate
		}
	}
	return ""
}

func firstNonEmptyText(candidates ...string) string {
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate) != "" {
			return candidate
		}
	}
	return ""
}

func globalParamsEmpty(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	return trimmed == "" || trimmed == "null" || trimmed == "[]"
}

// snapshotDolphinGraph 复制整图正文，后续 PUT 以下线前副本为准。
// snapshotDolphinGraph copies the graph payload so later PUTs keep the pre-OFFLINE body.
func snapshotDolphinGraph(graph *dolphinGraph) *dolphinGraph {
	if graph == nil {
		return nil
	}
	cloned := *graph
	cloned.Tasks = append([]json.RawMessage(nil), graph.Tasks...)
	cloned.Relations = append([]json.RawMessage(nil), graph.Relations...)
	return &cloned
}

// restoreGraphForWriteBack 把下线前的节点/边/坐标写回刷新结果。
// Dolphin 3.2 OFFLINE 后再 GET 可能清空 locations，甚至只返回部分节点；
// PUT 是整图覆盖，所以正文必须用下线前那份，刷新结果只保留 version / releaseState。
// restoreGraphForWriteBack writes the pre-OFFLINE nodes/edges/coordinates onto the refreshed graph.
// A 3.2 GET after OFFLINE may empty locations or drop siblings; PUT replaces the whole graph,
// so the body must come from the snapshot while version/releaseState stay on the refresh.
func restoreGraphForWriteBack(graph, saved *dolphinGraph) {
	if graph == nil || saved == nil {
		return
	}
	if graph.Code == 0 {
		graph.Code = saved.Code
	}
	graph.Name = firstNonEmptyText(graph.Name, saved.Name)
	graph.Description = firstNonEmptyText(graph.Description, saved.Description)
	graph.TenantCode = firstNonEmptyText(graph.TenantCode, saved.TenantCode)
	graph.ExecutionType = firstNonEmptyText(graph.ExecutionType, saved.ExecutionType)
	graph.Locations = firstNonEmptyLocations(graph.Locations, saved.Locations)
	if globalParamsEmpty(graph.GlobalParams) && !globalParamsEmpty(saved.GlobalParams) {
		graph.GlobalParams = saved.GlobalParams
	}
	if graph.Timeout == 0 && saved.Timeout != 0 {
		graph.Timeout = saved.Timeout
	}
	if len(saved.Tasks) > 0 {
		graph.Tasks = append([]json.RawMessage(nil), saved.Tasks...)
	}
	if len(saved.Relations) > 0 {
		graph.Relations = append([]json.RawMessage(nil), saved.Relations...)
	}
}

// assertGraphSafeToWrite 拒绝用空节点/边/坐标覆盖已有工作流。
// assertGraphSafeToWrite refuses to overwrite an existing workflow with an empty graph.
func assertGraphSafeToWrite(graph, saved *dolphinGraph) error {
	if graph == nil || saved == nil {
		return nil
	}
	if len(saved.Tasks) > 0 && len(graph.Tasks) == 0 {
		return fmt.Errorf("%w: refusing to overwrite workflow with empty task list", ErrDolphinPushInvalid)
	}
	if len(saved.Relations) > 0 && len(graph.Relations) == 0 {
		return fmt.Errorf("%w: refusing to overwrite workflow with empty relations", ErrDolphinPushInvalid)
	}
	if !locationsEmpty(saved.Locations) && locationsEmpty(graph.Locations) {
		return fmt.Errorf("%w: refusing to overwrite workflow with empty locations", ErrDolphinPushInvalid)
	}
	for _, raw := range saved.Tasks {
		code := peekTask(raw).Code
		if code != 0 && !graphHasTask(graph, code) {
			return fmt.Errorf("%w: refusing to drop existing task %d on full-graph PUT", ErrDolphinPushInvalid, code)
		}
	}
	return nil
}

func peekTask(raw json.RawMessage) dolphinTaskPeek {
	var peek dolphinTaskPeek
	_ = json.Unmarshal(raw, &peek)
	return peek
}

func peekRelation(raw json.RawMessage) dolphinRelationPeek {
	var peek dolphinRelationPeek
	_ = json.Unmarshal(raw, &peek)
	return peek
}

func peekTaskByCode(graph *dolphinGraph, code int64) (dolphinTaskPeek, int, bool) {
	if graph == nil {
		return dolphinTaskPeek{}, -1, false
	}
	for i, raw := range graph.Tasks {
		peek := peekTask(raw)
		if peek.Code == code {
			return peek, i, true
		}
	}
	return dolphinTaskPeek{}, -1, false
}

func graphHasTask(graph *dolphinGraph, code int64) bool {
	_, _, ok := peekTaskByCode(graph, code)
	return ok
}

func dolphinTaskRawScript(raw json.RawMessage) string {
	params := peekTask(raw).TaskParams
	if len(bytes.TrimSpace(params)) == 0 || bytes.Equal(bytes.TrimSpace(params), []byte("null")) {
		return ""
	}
	if params[0] == '"' {
		var asString string
		if json.Unmarshal(params, &asString) == nil {
			params = json.RawMessage(asString)
		}
	}
	var obj map[string]any
	if json.Unmarshal(params, &obj) != nil {
		return ""
	}
	for _, key := range []string{"rawScript", "raw_script"} {
		if value, ok := obj[key].(string); ok {
			return value
		}
	}
	return ""
}

func taskVersionByCode(tasks []json.RawMessage, code int64) int {
	for _, raw := range tasks {
		peek := peekTask(raw)
		if peek.Code == code {
			if peek.Version > 0 {
				return peek.Version
			}
			return 1
		}
	}
	return 1
}

func jsonNumberInt(value any) int {
	switch typed := value.(type) {
	case json.Number:
		n, _ := typed.Int64()
		return int(n)
	case float64:
		return int(typed)
	case int:
		return typed
	case int64:
		return int(typed)
	default:
		return 0
	}
}

func decodeJSONObject(raw json.RawMessage) (map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var obj map[string]any
	if err := dec.Decode(&obj); err != nil {
		return nil, err
	}
	if obj == nil {
		obj = map[string]any{}
	}
	return obj, nil
}

func asJSONObject(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return typed
	case json.RawMessage:
		obj, err := decodeJSONObject(typed)
		if err == nil {
			return obj
		}
	case string:
		obj, err := decodeJSONObject(json.RawMessage(typed))
		if err == nil {
			return obj
		}
	}
	return nil
}

func patchJSONObject(raw json.RawMessage, mutate func(map[string]any)) (json.RawMessage, error) {
	obj, err := decodeJSONObject(raw)
	if err != nil {
		return nil, err
	}
	mutate(obj)
	encoded, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	return encoded, nil
}

// patchShellTaskRaw 只改绑定 SHELL 节点的脚本/名称/worker，超时字段原样带回。
// patchShellTaskRaw updates the bound SHELL script/name/worker and leaves timeout fields untouched.
func patchShellTaskRaw(raw json.RawMessage, name, script, workerGroup string) (json.RawMessage, int, error) {
	var version int
	encoded, err := patchJSONObject(raw, func(obj map[string]any) {
		if strings.TrimSpace(name) != "" {
			obj["name"] = strings.TrimSpace(name)
		}
		if strings.TrimSpace(workerGroup) != "" {
			obj["workerGroup"] = strings.TrimSpace(workerGroup)
		}
		if _, ok := obj["isCache"]; !ok {
			obj["isCache"] = "NO"
		}
		if _, ok := obj["taskExecuteType"]; !ok {
			obj["taskExecuteType"] = "BATCH"
		}
		params := asJSONObject(obj["taskParams"])
		if params == nil {
			params = map[string]any{}
		}
		params["rawScript"] = script
		obj["taskParams"] = params
		version = jsonNumberInt(obj["version"])
		if version > 0 {
			version++
		} else {
			version = 1
		}
		obj["version"] = version
	})
	if err != nil {
		return nil, 0, err
	}
	return encoded, version, nil
}

// bumpRelationTaskVersion 同步边两端 version，避免只升任务 version 导致 Dolphin 丢掉连线。
// bumpRelationTaskVersion keeps edge endpoint versions in sync so Dolphin does not drop relations.
func bumpRelationTaskVersion(relations []json.RawMessage, code int64, version int) []json.RawMessage {
	if version <= 0 || code == 0 {
		return relations
	}
	out := make([]json.RawMessage, 0, len(relations))
	for _, raw := range relations {
		peek := peekRelation(raw)
		if peek.PreTaskCode != code && peek.PostTaskCode != code {
			out = append(out, raw)
			continue
		}
		patched, err := patchJSONObject(raw, func(obj map[string]any) {
			if peek.PreTaskCode == code {
				obj["preTaskVersion"] = version
			}
			if peek.PostTaskCode == code {
				obj["postTaskVersion"] = version
			}
		})
		if err != nil {
			out = append(out, raw)
			continue
		}
		out = append(out, patched)
	}
	return out
}

func stripStartRelation(relations []json.RawMessage, postCode int64) []json.RawMessage {
	filtered := make([]json.RawMessage, 0, len(relations))
	for _, raw := range relations {
		rel := peekRelation(raw)
		if rel.PostTaskCode == postCode && rel.PreTaskCode == 0 {
			continue
		}
		filtered = append(filtered, raw)
	}
	return filtered
}

func peekLocationTaskCode(raw json.RawMessage) int64 {
	var obj struct {
		TaskCode *int64 `json:"taskCode"`
		Code     *int64 `json:"code"`
	}
	if json.Unmarshal(raw, &obj) != nil {
		return 0
	}
	if obj.TaskCode != nil {
		return *obj.TaskCode
	}
	if obj.Code != nil {
		return *obj.Code
	}
	return 0
}

func nextLocationPoint(count int) (int, int) {
	return 240 + 80*count, 160 + 40*(count%4)
}

func newLocationNode(code int64, x, y int, arrayForm bool) json.RawMessage {
	if arrayForm {
		return marshalRaw(map[string]any{"taskCode": code, "x": x, "y": y})
	}
	return marshalRaw(map[string]any{"x": x, "y": y})
}

// mergeDolphinLocation 把新节点并进已有 coordinates，支持 3.2 数组与旧版 map，且不丢额外字段。
// mergeDolphinLocation merges a new node into existing coordinates, supporting 3.2 arrays and legacy maps without dropping extra fields.
func mergeDolphinLocation(raw string, code int64) string {
	trimmed := strings.TrimSpace(raw)
	x, y := nextLocationPoint(0)
	if locationsEmpty(trimmed) {
		return string(marshalRaw([]any{map[string]any{"taskCode": code, "x": x, "y": y}}))
	}
	var probe json.RawMessage
	if err := json.Unmarshal([]byte(trimmed), &probe); err != nil || len(bytes.TrimSpace(probe)) == 0 {
		// 原串都解析不了时绝不能改写成 {}，否则画布坐标会被整份清空。
		// Never rewrite unparsable locations to {}; that would wipe the canvas.
		return trimmed
	}
	probe = bytes.TrimSpace(probe)
	switch probe[0] {
	case '[':
		var items []json.RawMessage
		if err := json.Unmarshal(probe, &items); err != nil {
			return trimmed
		}
		for _, item := range items {
			if peekLocationTaskCode(item) == code {
				return trimmed
			}
		}
		x, y = nextLocationPoint(len(items))
		items = append(items, newLocationNode(code, x, y, true))
		return string(marshalRaw(items))
	case '{':
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(probe, &obj); err != nil {
			return trimmed
		}
		key := strconv.FormatInt(code, 10)
		if _, exists := obj[key]; exists {
			return trimmed
		}
		x, y = nextLocationPoint(len(obj))
		obj[key] = newLocationNode(code, x, y, false)
		encoded, err := json.Marshal(obj)
		if err != nil {
			return trimmed
		}
		return string(encoded)
	default:
		return trimmed
	}
}

func dolphinHasCycle(relations []json.RawMessage, extra [][2]int64) bool {
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
	for _, raw := range relations {
		rel := peekRelation(raw)
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

func shellTaskType(taskType string) bool {
	return strings.EqualFold(strings.TrimSpace(taskType), "SHELL")
}

func boundNodeNotShellError(code int64, taskType string) error {
	return fmt.Errorf("%w: bound node %d is %s, not SHELL", ErrDolphinPushInvalid, code, taskType)
}
