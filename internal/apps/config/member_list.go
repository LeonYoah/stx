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
	"errors"
	"fmt"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	// ErrMemberListNotFound 配置中没有 hazelcast.network.join.tcp-ip.member-list。
	// ErrMemberListNotFound means the config has no hazelcast.network.join.tcp-ip.member-list.
	ErrMemberListNotFound = errors.New("hazelcast.network.join.tcp-ip.member-list not found / 未找到 hazelcast.network.join.tcp-ip.member-list")
	// ErrEmptyMemberList 拒绝写入空成员列表。
	// ErrEmptyMemberList refuses to write an empty member list.
	ErrEmptyMemberList = errors.New("member list must not be empty / 成员列表不能为空")
)

// memberListPath 是 Hazelcast 成员列表在 YAML 中的路径。
// memberListPath is the YAML path of the Hazelcast member list.
var memberListPath = []string{"hazelcast", "network", "join", "tcp-ip", "member-list"}

// IsMemberListConfigType 判断配置类型是否包含 Hazelcast member-list。
// IsMemberListConfigType reports whether a config type carries a Hazelcast member-list.
func IsMemberListConfigType(configType ConfigType) bool {
	switch configType {
	case ConfigTypeHazelcast, ConfigTypeHazelcastMaster, ConfigTypeHazelcastWorker:
		return true
	default:
		return false
	}
}

// MemberListConfigTypes 返回指定部署模式下需要修补 member-list 的配置类型：混合模式只有 hazelcast.yaml；
// 分离模式为 hazelcast-master.yaml、hazelcast-worker.yaml，若存在 hazelcast.yaml 也一并修补；模式未知时三者都修补。
// MemberListConfigTypes returns the config types whose member-list must be patched for a deployment mode:
// hybrid only hazelcast.yaml; separated hazelcast-master.yaml and hazelcast-worker.yaml plus hazelcast.yaml
// when present; an unknown mode patches all three.
func MemberListConfigTypes(deploymentMode string) []ConfigType {
	switch strings.ToLower(strings.TrimSpace(deploymentMode)) {
	case "hybrid":
		return []ConfigType{ConfigTypeHazelcast}
	default:
		return []ConfigType{ConfigTypeHazelcastMaster, ConfigTypeHazelcastWorker, ConfigTypeHazelcast}
	}
}

// ReadMemberList 读取配置中的 member-list；值为空时返回空切片。
// ReadMemberList reads the member-list from a config; an empty value yields an empty slice.
func ReadMemberList(content string) ([]string, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
		return nil, fmt.Errorf("invalid hazelcast yaml: %w", err)
	}
	_, value, _, err := findMemberListNode(&doc)
	if err != nil {
		return nil, err
	}
	return memberListValues(value)
}

// PatchMemberListContent 只替换 hazelcast.network.join.tcp-ip.member-list，其余内容（注释、空行、缩进、顺序）保持原样。
// 成员已一致时原样返回且 changed=false。优先按行精确替换；无法安全替换时退回到 YAML 节点重写，
// 两种方式都会重新解析校验：除 member-list 外的语义必须与原文完全一致。
// PatchMemberListContent replaces only hazelcast.network.join.tcp-ip.member-list and keeps everything else
// (comments, blank lines, indentation, order) unchanged. When members already match it returns the input
// with changed=false. It prefers an exact line splice and falls back to a YAML node rewrite when the splice
// is not safe; both are re-parsed and verified so that everything except member-list stays semantically equal.
func PatchMemberListContent(content string, members []string) (string, bool, error) {
	cleaned := normalizeMembers(members)
	if len(cleaned) == 0 {
		return "", false, ErrEmptyMemberList
	}

	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
		return "", false, fmt.Errorf("invalid hazelcast yaml: %w", err)
	}
	key, value, parent, err := findMemberListNode(&doc)
	if err != nil {
		return "", false, err
	}
	if current, readErr := memberListValues(value); readErr == nil && equalStrings(current, cleaned) {
		return content, false, nil
	}

	if spliced, ok := spliceMemberList(content, key, value, cleaned); ok {
		if verifyErr := verifyMemberListPatch(content, spliced, cleaned); verifyErr == nil {
			return spliced, true, nil
		}
	}

	rewritten, err := rewriteMemberList(content, &doc, parent, value, cleaned)
	if err != nil {
		return "", false, err
	}
	if err := verifyMemberListPatch(content, rewritten, cleaned); err != nil {
		return "", false, err
	}
	return rewritten, true, nil
}

// findMemberListNode 返回 member-list 的键节点、值节点和所在映射。
// findMemberListNode returns the member-list key node, value node and the mapping that holds them.
func findMemberListNode(doc *yaml.Node) (*yaml.Node, *yaml.Node, *yaml.Node, error) {
	if doc == nil || len(doc.Content) == 0 {
		return nil, nil, nil, ErrMemberListNotFound
	}
	current := doc.Content[0]
	for depth, segment := range memberListPath {
		if current == nil || current.Kind != yaml.MappingNode {
			return nil, nil, nil, ErrMemberListNotFound
		}
		var next *yaml.Node
		for i := 0; i+1 < len(current.Content); i += 2 {
			if current.Content[i].Value != segment {
				continue
			}
			if depth == len(memberListPath)-1 {
				return current.Content[i], current.Content[i+1], current, nil
			}
			next = current.Content[i+1]
			break
		}
		current = next
	}
	return nil, nil, nil, ErrMemberListNotFound
}

func memberListValues(value *yaml.Node) ([]string, error) {
	switch {
	case value == nil:
		return []string{}, nil
	case value.Kind == yaml.ScalarNode && value.Tag == "!!null":
		return []string{}, nil
	case value.Kind == yaml.SequenceNode:
		result := make([]string, 0, len(value.Content))
		for _, item := range value.Content {
			if item.Kind != yaml.ScalarNode {
				return nil, fmt.Errorf("member-list item is not a scalar / member-list 条目不是标量")
			}
			result = append(result, item.Value)
		}
		return result, nil
	default:
		return nil, fmt.Errorf("member-list is not a list / member-list 不是列表")
	}
}

// spliceMemberList 在原文中按行替换 member-list 的值；只处理逐行块序列、单行流序列和隐式空值三种安全形态。
// spliceMemberList replaces the member-list value in place line by line; it only handles three safe shapes:
// one-item-per-line block sequences, single-line flow sequences and an implicit null value.
func spliceMemberList(content string, key, value *yaml.Node, members []string) (string, bool) {
	lines := strings.SplitAfter(content, "\n")
	if key == nil || value == nil || key.Line < 1 || key.Line > len(lines) {
		return "", false
	}

	switch {
	case value.Kind == yaml.SequenceNode && value.Style&yaml.FlowStyle == 0 && len(value.Content) > 0:
		return spliceBlockSequence(lines, key, value, members)
	case value.Kind == yaml.SequenceNode && value.Style&yaml.FlowStyle != 0:
		return spliceFlowSequence(lines, value, members)
	case value.Kind == yaml.ScalarNode && value.Tag == "!!null" && value.Value == "":
		return spliceAfterKey(lines, key, members)
	default:
		return "", false
	}
}

func spliceBlockSequence(lines []string, key, value *yaml.Node, members []string) (string, bool) {
	start := value.Line
	if start <= key.Line || start > len(lines) || value.Column < 1 {
		return "", false
	}
	end := start
	previous := 0
	for _, item := range value.Content {
		if item.Kind != yaml.ScalarNode || item.Style&(yaml.LiteralStyle|yaml.FoldedStyle) != 0 {
			return "", false
		}
		if item.Line <= previous || item.Line > len(lines) {
			return "", false
		}
		runes := []rune(lines[item.Line-1])
		if item.Column < 1 || item.Column-1 > len(runes) || strings.TrimSpace(string(runes[:item.Column-1])) != "-" {
			return "", false
		}
		previous = item.Line
		end = item.Line
	}

	firstLine := []rune(lines[start-1])
	if value.Column-1 > len(firstLine) {
		return "", false
	}
	indent := string(firstLine[:value.Column-1])
	if strings.TrimSpace(indent) != "" {
		return "", false
	}
	eol := lineEnding(lines[end-1])
	defaultEOL := eol
	if defaultEOL == "" {
		defaultEOL = detectLineEnding(lines)
	}

	var replacement strings.Builder
	for i, member := range members {
		replacement.WriteString(indent)
		replacement.WriteString("- ")
		replacement.WriteString(yamlScalar(member))
		if i == len(members)-1 {
			replacement.WriteString(eol)
		} else {
			replacement.WriteString(defaultEOL)
		}
	}
	return strings.Join(lines[:start-1], "") + replacement.String() + strings.Join(lines[end:], ""), true
}

func spliceFlowSequence(lines []string, value *yaml.Node, members []string) (string, bool) {
	if value.Line < 1 || value.Line > len(lines) || value.Column < 1 {
		return "", false
	}
	for _, item := range value.Content {
		if item.Kind != yaml.ScalarNode || item.Line != value.Line {
			return "", false
		}
	}
	runes := []rune(lines[value.Line-1])
	open := value.Column - 1
	if open >= len(runes) || runes[open] != '[' {
		return "", false
	}
	closeIndex := matchingBracket(runes, open)
	if closeIndex < 0 {
		return "", false
	}
	items := make([]string, 0, len(members))
	for _, member := range members {
		items = append(items, yamlScalar(member))
	}
	replaced := string(runes[:open]) + "[" + strings.Join(items, ", ") + "]" + string(runes[closeIndex+1:])
	result := append([]string(nil), lines...)
	result[value.Line-1] = replaced
	return strings.Join(result, ""), true
}

func spliceAfterKey(lines []string, key *yaml.Node, members []string) (string, bool) {
	if key.Column < 1 {
		return "", false
	}
	keyLine := lines[key.Line-1]
	indent := strings.Repeat(" ", key.Column-1+2)
	eol := lineEnding(keyLine)
	if eol == "" {
		eol = detectLineEnding(lines)
		lines = append([]string(nil), lines...)
		lines[key.Line-1] = keyLine + eol
	}
	var inserted strings.Builder
	for _, member := range members {
		inserted.WriteString(indent)
		inserted.WriteString("- ")
		inserted.WriteString(yamlScalar(member))
		inserted.WriteString(eol)
	}
	return strings.Join(lines[:key.Line], "") + inserted.String() + strings.Join(lines[key.Line:], ""), true
}

// rewriteMemberList 退回方案：替换 YAML 节点后整体重新编码，缩进沿用原文。
// rewriteMemberList is the fallback: swap the YAML node and re-encode the document with the original indent.
func rewriteMemberList(content string, doc, parent, value *yaml.Node, members []string) (string, error) {
	sequence := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	if value != nil {
		if value.Kind == yaml.SequenceNode {
			sequence.Style = value.Style & yaml.FlowStyle
		}
		sequence.HeadComment = value.HeadComment
		sequence.LineComment = value.LineComment
		sequence.FootComment = value.FootComment
	}
	for _, member := range members {
		sequence.Content = append(sequence.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: member})
	}
	replaced := false
	for i := 1; i < len(parent.Content); i += 2 {
		if parent.Content[i] == value {
			parent.Content[i] = sequence
			replaced = true
			break
		}
	}
	if !replaced {
		return "", ErrMemberListNotFound
	}

	var buffer bytes.Buffer
	encoder := yaml.NewEncoder(&buffer)
	encoder.SetIndent(detectIndent(content))
	if err := encoder.Encode(doc); err != nil {
		return "", fmt.Errorf("encode hazelcast yaml: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return "", fmt.Errorf("encode hazelcast yaml: %w", err)
	}
	return buffer.String(), nil
}

// verifyMemberListPatch 重新解析新旧内容：新内容的 member-list 必须等于目标，其余语义必须与原文一致。
// verifyMemberListPatch re-parses both contents: the new member-list must equal the target and everything
// else must be semantically identical to the original.
func verifyMemberListPatch(original, patched string, members []string) error {
	var before any
	if err := yaml.Unmarshal([]byte(original), &before); err != nil {
		return fmt.Errorf("verify member-list patch: %w", err)
	}
	var after any
	if err := yaml.Unmarshal([]byte(patched), &after); err != nil {
		return fmt.Errorf("verify member-list patch: patched yaml is invalid: %w", err)
	}
	expectedList := make([]any, 0, len(members))
	for _, member := range members {
		expectedList = append(expectedList, member)
	}
	if !setDecodedPath(before, memberListPath, expectedList) {
		return ErrMemberListNotFound
	}
	if !reflect.DeepEqual(before, after) {
		return errors.New("member-list patch changed other content / 修补 member-list 时意外改动了其他内容")
	}
	return nil
}

func setDecodedPath(root any, path []string, value any) bool {
	current, ok := root.(map[string]any)
	if !ok {
		return false
	}
	for i, segment := range path {
		if i == len(path)-1 {
			if _, exists := current[segment]; !exists {
				return false
			}
			current[segment] = value
			return true
		}
		next, ok := current[segment].(map[string]any)
		if !ok {
			return false
		}
		current = next
	}
	return false
}

// yamlScalar 生成安全的单行 YAML 标量（必要时自动加引号）。
// yamlScalar renders a safe single-line YAML scalar (quoted when needed).
func yamlScalar(value string) string {
	out, err := yaml.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%q", value)
	}
	return strings.TrimRight(string(out), "\n")
}

func matchingBracket(runes []rune, open int) int {
	depth := 0
	var quote rune
	for i := open; i < len(runes); i++ {
		r := runes[i]
		if quote != 0 {
			if r == quote {
				quote = 0
			}
			continue
		}
		switch r {
		case '\'', '"':
			quote = r
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func lineEnding(line string) string {
	switch {
	case strings.HasSuffix(line, "\r\n"):
		return "\r\n"
	case strings.HasSuffix(line, "\n"):
		return "\n"
	default:
		return ""
	}
}

func detectLineEnding(lines []string) string {
	for _, line := range lines {
		if eol := lineEnding(line); eol != "" {
			return eol
		}
	}
	return "\n"
}

// detectIndent 推断原文的缩进宽度（取非注释行的最小正缩进），默认 2。
// detectIndent infers the original indent width (smallest positive indent of non-comment lines), default 2.
func detectIndent(content string) int {
	indent := 0
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimLeft(line, " ")
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "- ") {
			continue
		}
		width := len(line) - len(trimmed)
		if width > 0 && (indent == 0 || width < indent) {
			indent = width
		}
	}
	if indent < 2 || indent > 8 {
		return 2
	}
	return indent
}

func normalizeMembers(members []string) []string {
	result := make([]string, 0, len(members))
	for _, member := range members {
		if trimmed := strings.TrimSpace(member); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
