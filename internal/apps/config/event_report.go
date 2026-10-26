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

package config

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// ErrSeatunnelConfigMissing 表示集群在 STX 中还没有 seatunnel.yaml 记录（需先初始化集群配置）。
// ErrSeatunnelConfigMissing means the cluster has no seatunnel.yaml record in STX yet (initialize cluster configs first).
var ErrSeatunnelConfigMissing = errors.New("config: cluster has no seatunnel.yaml in STX; initialize cluster configs first")

const eventReportHTTPKey = "event-report-http"

// EventReportSettings 是写入 seatunnel.engine.event-report-http 的内容；nil 表示移除该配置块。
// EventReportSettings is written to seatunnel.engine.event-report-http; nil removes the block.
type EventReportSettings struct {
	URL               string
	Headers           map[string]string
	ReportNonTerminal bool
}

// EventReportNodeResult 是一份 seatunnel.yaml（模板或节点）的写入结果。
// EventReportNodeResult is the outcome for one seatunnel.yaml (template or node).
type EventReportNodeResult struct {
	ConfigID uint   `json:"config_id"`
	HostID   *uint  `json:"host_id,omitempty"`
	Template bool   `json:"template"`
	Changed  bool   `json:"changed"`
	Pushed   bool   `json:"pushed"`
	Pending  bool   `json:"pending"`
	Error    string `json:"error,omitempty"`
}

// EventReportApplyResult 汇总一次事件上报配置写入。
// EventReportApplyResult summarizes one event-report config write.
type EventReportApplyResult struct {
	Configs []EventReportNodeResult `json:"configs"`
}

// PatchEventReportHTTP 在 seatunnel.yaml 中写入或移除 seatunnel.engine.event-report-http，其余内容保持不变。
// report-non-terminal-job-state 必须写在 event-report-http 下面（引擎从该节点的属性读取）。
// PatchEventReportHTTP writes or removes seatunnel.engine.event-report-http in seatunnel.yaml and leaves everything
// else untouched. report-non-terminal-job-state must live under event-report-http because the engine reads it there.
func PatchEventReportHTTP(content string, settings *EventReportSettings) (string, error) {
	var root yaml.Node
	if strings.TrimSpace(content) != "" {
		if err := yaml.Unmarshal([]byte(content), &root); err != nil {
			return "", fmt.Errorf("invalid seatunnel yaml: %w", err)
		}
	}
	if len(root.Content) == 0 {
		root = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}}
	}
	document := root.Content[0]
	if document.Kind != yaml.MappingNode {
		return "", fmt.Errorf("invalid seatunnel yaml: top level must be a mapping")
	}

	if settings == nil {
		seatunnel := findYAMLMapChild(document, "seatunnel")
		engine := findYAMLMapChild(seatunnel, "engine")
		if engine == nil || !removeYAMLMapChild(engine, eventReportHTTPKey) {
			return content, nil
		}
		return encodeYAMLDocument(&root)
	}

	seatunnel, err := ensureYAMLMapChild(document, "seatunnel")
	if err != nil {
		return "", err
	}
	engine, err := ensureYAMLMapChild(seatunnel, "engine")
	if err != nil {
		return "", err
	}
	block := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	appendYAMLScalar(block, "url", settings.URL, "!!str")
	if len(settings.Headers) > 0 {
		headers := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		names := make([]string, 0, len(settings.Headers))
		for name := range settings.Headers {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			appendYAMLScalar(headers, name, settings.Headers[name], "!!str")
		}
		block.Content = append(block.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "headers"}, headers)
	}
	if settings.ReportNonTerminal {
		appendYAMLScalar(block, "report-non-terminal-job-state", "true", "!!bool")
	}
	setYAMLMapChild(engine, eventReportHTTPKey, block)
	return encodeYAMLDocument(&root)
}

func appendYAMLScalar(parent *yaml.Node, key, value, tag string) {
	parent.Content = append(parent.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: value},
	)
}

// ensureYAMLMapChild 返回 key 对应的映射节点，不存在时新建；已存在但不是映射时报错。
// ensureYAMLMapChild returns the mapping node for key, creating it when absent; a non-mapping value is an error.
func ensureYAMLMapChild(parent *yaml.Node, key string) (*yaml.Node, error) {
	if child := findYAMLMapChild(parent, key); child != nil {
		if child.Kind != yaml.MappingNode {
			if child.Kind == yaml.ScalarNode && (child.Tag == "!!null" || strings.TrimSpace(child.Value) == "") {
				child.Kind = yaml.MappingNode
				child.Tag = "!!map"
				child.Value = ""
				return child, nil
			}
			return nil, fmt.Errorf("invalid seatunnel yaml: %s must be a mapping", key)
		}
		return child, nil
	}
	child := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	parent.Content = append(parent.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, child)
	return child, nil
}

func setYAMLMapChild(parent *yaml.Node, key string, value *yaml.Node) {
	for i := 0; i < len(parent.Content)-1; i += 2 {
		if parent.Content[i].Value == key {
			// 保留原键上的注释。 / Keep comments attached to the existing key.
			value.HeadComment = parent.Content[i+1].HeadComment
			parent.Content[i+1] = value
			return
		}
	}
	parent.Content = append(parent.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, value)
}

func removeYAMLMapChild(parent *yaml.Node, key string) bool {
	if parent == nil || parent.Kind != yaml.MappingNode {
		return false
	}
	for i := 0; i < len(parent.Content)-1; i += 2 {
		if parent.Content[i].Value == key {
			parent.Content = append(parent.Content[:i], parent.Content[i+2:]...)
			return true
		}
	}
	return false
}

func encodeYAMLDocument(root *yaml.Node) (string, error) {
	var buffer bytes.Buffer
	encoder := yaml.NewEncoder(&buffer)
	encoder.SetIndent(2)
	if err := encoder.Encode(root); err != nil {
		return "", err
	}
	if err := encoder.Close(); err != nil {
		return "", err
	}
	return buffer.String(), nil
}

// ApplyEventReportConfig 把 event-report-http 写入（settings 为 nil 时移除）集群的 seatunnel.yaml 模板与每个节点配置。
// 逐个更新而不是用模板覆盖节点，避免抹掉节点上各自的配置；节点配置沿用 Update 的“先推送再提交、失败标记待推送”。
// ApplyEventReportConfig writes (or, with nil settings, removes) event-report-http in the cluster seatunnel.yaml
// template and every node config. Configs are patched one by one instead of overwriting nodes with the template so
// node-specific settings survive; node configs keep Update's push-then-commit semantics with pending on failure.
func (s *Service) ApplyEventReportConfig(ctx context.Context, clusterID uint, settings *EventReportSettings, userID uint) (*EventReportApplyResult, error) {
	configs, err := s.repo.ListByCluster(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	comment := "Enable SeaTunnel event reporting to STX / 开启 SeaTunnel 事件上报"
	if settings == nil {
		comment = "Disable SeaTunnel event reporting to STX / 关闭 SeaTunnel 事件上报"
	}
	result := &EventReportApplyResult{Configs: []EventReportNodeResult{}}
	for _, item := range configs {
		if item == nil || item.ConfigType != ConfigTypeSeatunnel {
			continue
		}
		base := item.Content.String()
		if item.HasPendingPush() {
			base = item.PendingContent.String()
		}
		outcome := EventReportNodeResult{ConfigID: item.ID, HostID: item.HostID, Template: item.IsTemplate()}
		patched, patchErr := PatchEventReportHTTP(base, settings)
		if patchErr != nil {
			outcome.Error = patchErr.Error()
			result.Configs = append(result.Configs, outcome)
			continue
		}
		if patched == base && !item.HasPendingPush() {
			result.Configs = append(result.Configs, outcome)
			continue
		}
		outcome.Changed = true
		info, updateErr := s.Update(ctx, item.ID, &UpdateConfigRequest{Content: patched, Comment: comment}, userID)
		switch {
		case updateErr != nil:
			outcome.Error = updateErr.Error()
		case info != nil && info.PushError != "":
			outcome.Pending = true
			outcome.Error = info.PushError
		default:
			outcome.Pushed = !outcome.Template
		}
		result.Configs = append(result.Configs, outcome)
	}
	if len(result.Configs) == 0 {
		return nil, ErrSeatunnelConfigMissing
	}
	return result, nil
}
