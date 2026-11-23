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

package stupgrade

import (
	"fmt"
	"sort"
	"strings"

	appconfig "github.com/LeonYoah/stx/internal/apps/config"
)

type configMergeInput struct {
	ConfigType   string
	TargetPath   string
	BaseContent  string
	LocalContent string
}

func buildConfigMergeInputs(configs []*appconfig.ConfigInfo) ([]configMergeInput, []BlockingIssue) {
	templates := make(map[string]*appconfig.ConfigInfo)
	nodeConfigs := make(map[string][]*appconfig.ConfigInfo)
	configTypes := make(map[string]struct{})
	issues := make([]BlockingIssue, 0)

	for _, cfg := range configs {
		configType := string(cfg.ConfigType)
		configTypes[configType] = struct{}{}
		if cfg.IsTemplate {
			templates[configType] = cfg
			continue
		}
		nodeConfigs[configType] = append(nodeConfigs[configType], cfg)
	}

	sortedTypes := make([]string, 0, len(configTypes))
	for configType := range configTypes {
		sortedTypes = append(sortedTypes, configType)
	}
	sort.Strings(sortedTypes)

	inputs := make([]configMergeInput, 0, len(sortedTypes))
	for _, configType := range sortedTypes {
		template := templates[configType]
		nodes := nodeConfigs[configType]

		baseContent := ""
		targetPath := ""
		if template != nil {
			baseContent = template.Content
			targetPath = normalizeConfigArchivePath(template.FilePath)
		}
		if targetPath == "" {
			targetPath = normalizeConfigArchivePath(appconfig.GetConfigFilePath(appconfig.ConfigType(configType)))
		}

		localContent := baseContent
		if len(nodes) > 0 {
			localContent = nodes[0].Content
			if targetPath == "" {
				targetPath = normalizeConfigArchivePath(nodes[0].FilePath)
			}
			nodeContents := make([]*appconfig.ConfigInfo, 0, len(nodes))
			if appconfig.IsMemberListConfigType(appconfig.ConfigType(configType)) {
				for _, node := range nodes {
					normalized := *node
					normalized.Content = appconfig.NormalizeHazelcastEndpoint(node.Content)
					nodeContents = append(nodeContents, &normalized)
				}
			} else {
				nodeContents = nodes
			}
			uniqueNodeContents := uniqueConfigContents(nodeContents)
			if len(uniqueNodeContents) > 1 {
				hostIDs := make([]string, 0, len(nodes))
				for _, node := range nodes {
					if node.HostID == nil {
						continue
					}
					hostIDs = append(hostIDs, fmt.Sprintf("%d", *node.HostID))
				}
				sort.Strings(hostIDs)
				issues = append(issues, blockingIssue(
					CheckCategoryConfig,
					"config_node_variants",
					fmt.Sprintf("config %s has multiple node-specific variants and requires manual review / 配置 %s 在不同节点上存在多个差异版本，需要人工确认", configType, configType),
					map[string]string{
						"config_type": configType,
						"host_ids":    strings.Join(hostIDs, ","),
					},
				))
			}
		}

		inputs = append(inputs, configMergeInput{
			ConfigType:   configType,
			TargetPath:   targetPath,
			BaseContent:  baseContent,
			LocalContent: localContent,
		})
	}
	return inputs, issues
}

func uniqueConfigContents(configs []*appconfig.ConfigInfo) []string {
	seen := make(map[string]struct{}, len(configs))
	result := make([]string, 0, len(configs))
	for _, cfg := range configs {
		if _, ok := seen[cfg.Content]; ok {
			continue
		}
		seen[cfg.Content] = struct{}{}
		result = append(result, cfg.Content)
	}
	sort.Strings(result)
	return result
}

func buildConfigMergeFile(input configMergeInput, targetContent string) ConfigMergeFile {
	mergedContent, conflicts := mergeConfigContents(input.ConfigType, input.BaseContent, input.LocalContent, targetContent)
	conflictCount := len(conflicts)
	return ConfigMergeFile{
		ConfigType:    input.ConfigType,
		TargetPath:    input.TargetPath,
		BaseContent:   input.BaseContent,
		LocalContent:  input.LocalContent,
		TargetContent: targetContent,
		MergedContent: mergedContent,
		ConflictCount: conflictCount,
		Resolved:      conflictCount == 0,
		Conflicts:     conflicts,
	}
}

func mergeConfigContents(configType, baseContent, localContent, targetContent string) (string, []ConfigConflict) {
	baseLines := splitConfigLines(baseContent)
	localLines := splitConfigLines(localContent)
	targetLines := splitConfigLines(targetContent)
	maxLines := maxInt(len(baseLines), len(localLines), len(targetLines))
	if maxLines == 0 {
		return "", []ConfigConflict{}
	}

	mergedLines := make([]string, 0, maxLines)
	conflicts := make([]ConfigConflict, 0)
	currentConflict := configConflictAccumulator{}

	flushConflict := func() {
		if currentConflict.StartLine == 0 {
			return
		}
		conflict := currentConflict.toConflict(configType)
		conflicts = append(conflicts, conflict)
		mergedLines = append(mergedLines, buildConflictMarker(conflict)...)
		currentConflict = configConflictAccumulator{}
	}

	for idx := 0; idx < maxLines; idx++ {
		baseLine := lineAt(baseLines, idx)
		localLine := lineAt(localLines, idx)
		targetLine := lineAt(targetLines, idx)

		if localLine == targetLine {
			flushConflict()
			mergedLines = append(mergedLines, localLine)
			continue
		}

		currentConflict.append(idx+1, baseLine, localLine, targetLine)
	}

	flushConflict()
	return strings.Join(mergedLines, "\n"), conflicts
}

type configConflictAccumulator struct {
	StartLine   int
	EndLine     int
	BaseLines   []string
	LocalLines  []string
	TargetLines []string
}

func (a *configConflictAccumulator) append(lineNumber int, baseLine, localLine, targetLine string) {
	if a.StartLine == 0 {
		a.StartLine = lineNumber
	}
	a.EndLine = lineNumber
	a.BaseLines = append(a.BaseLines, baseLine)
	a.LocalLines = append(a.LocalLines, localLine)
	a.TargetLines = append(a.TargetLines, targetLine)
}

func (a configConflictAccumulator) toConflict(configType string) ConfigConflict {
	path := fmt.Sprintf("lines %d-%d", a.StartLine, a.EndLine)
	if a.StartLine == a.EndLine {
		path = fmt.Sprintf("line %d", a.StartLine)
	}
	id := fmt.Sprintf("%s:%d-%d", configType, a.StartLine, a.EndLine)
	return ConfigConflict{
		ID:          id,
		ConfigType:  configType,
		Path:        path,
		BaseValue:   strings.Join(a.BaseLines, "\n"),
		LocalValue:  strings.Join(a.LocalLines, "\n"),
		TargetValue: strings.Join(a.TargetLines, "\n"),
		Status:      ConfigConflictPending,
	}
}

func buildConflictMarker(conflict ConfigConflict) []string {
	return []string{
		fmt.Sprintf("<<<<<<< LOCAL [%s]", conflict.ID),
		conflict.LocalValue,
		"||||||| BASE",
		conflict.BaseValue,
		"=======",
		conflict.TargetValue,
		fmt.Sprintf(">>>>>>> TARGET [%s]", conflict.ID),
	}
}

func splitConfigLines(content string) []string {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	if normalized == "" {
		return nil
	}
	return strings.Split(normalized, "\n")
}

func lineAt(lines []string, idx int) string {
	if idx < 0 || idx >= len(lines) {
		return ""
	}
	return lines[idx]
}

func maxInt(values ...int) int {
	result := 0
	for _, value := range values {
		if value > result {
			result = value
		}
	}
	return result
}

// MergeStrategy 指定批量解决合并冲突时选用的版本来源。
// MergeStrategy selects which side wins when bulk-resolving merge conflicts.
type MergeStrategy string

const (
	// MergeStrategyKeepLocal 对所有未决冲突保留本地配置内容。
	// MergeStrategyKeepLocal keeps the local config content for every pending conflict.
	MergeStrategyKeepLocal MergeStrategy = "keep-local"
	// MergeStrategyUseTarget 对所有未决冲突采用目标版本包内配置内容。
	// MergeStrategyUseTarget takes the target package config content for every pending conflict.
	MergeStrategyUseTarget MergeStrategy = "use-target"
)

// visualDeleteSentinel 与前端合并编辑器保持一致：resolved_value 取该值表示该行被用户删除。
// visualDeleteSentinel mirrors the frontend merge editor: this resolved_value means the line was dropped.
const visualDeleteSentinel = "__ST_UPGRADE_DELETE_LINE__"

// ParseMergeStrategy 校验并返回合并冲突批量解决策略。
// ParseMergeStrategy validates and returns a bulk conflict-resolution strategy.
func ParseMergeStrategy(value string) (MergeStrategy, error) {
	switch MergeStrategy(strings.TrimSpace(value)) {
	case MergeStrategyKeepLocal:
		return MergeStrategyKeepLocal, nil
	case MergeStrategyUseTarget:
		return MergeStrategyUseTarget, nil
	default:
		return "", fmt.Errorf("invalid merge strategy %q: expected %q or %q / 无效的合并策略 %q：仅支持 %q 或 %q", value, value, MergeStrategyKeepLocal, MergeStrategyUseTarget, MergeStrategyKeepLocal, MergeStrategyUseTarget)
	}
}

// ApplyMergeStrategy 按策略批量解决合并计划中所有未决冲突，并重建 merged_content，
// 使结果满足 plan create 的 ready 契约（无冲突标记残留）。
// ApplyMergeStrategy bulk-resolves every pending conflict of the merge plan with the
// chosen side and rebuilds merged_content so the result satisfies the plan-create
// ready contract (no leftover conflict markers).
func ApplyMergeStrategy(plan *ConfigMergePlan, strategy MergeStrategy) error {
	if plan == nil {
		return fmt.Errorf("config merge plan is required / 配置合并计划不能为空")
	}
	if strategy != MergeStrategyKeepLocal && strategy != MergeStrategyUseTarget {
		return fmt.Errorf("invalid merge strategy %q / 无效的合并策略 %q", strategy, strategy)
	}
	for index := range plan.Files {
		file := &plan.Files[index]
		conflictsByID := make(map[string]*ConfigConflict, len(file.Conflicts))
		for conflictIndex := range file.Conflicts {
			conflict := &file.Conflicts[conflictIndex]
			conflictsByID[conflict.ID] = conflict
			if conflict.Status == ConfigConflictPending {
				// 未决冲突按策略选定整段内容。
				// Pending conflicts take the whole side selected by the strategy.
				if strategy == MergeStrategyKeepLocal {
					conflict.ResolvedValue = conflict.LocalValue
				} else {
					conflict.ResolvedValue = conflict.TargetValue
				}
				conflict.Status = ConfigConflictResolved
			}
		}
		file.MergedContent = resolveMergedMarkers(file.MergedContent, conflictsByID)
		pending := 0
		for _, conflict := range file.Conflicts {
			if conflict.Status != ConfigConflictResolved && conflict.LocalValue != conflict.TargetValue {
				pending++
			}
		}
		file.ConflictCount = pending
		file.Resolved = pending == 0
	}
	total := 0
	for _, file := range plan.Files {
		total += file.ConflictCount
	}
	plan.ConflictCount = total
	plan.HasConflicts = total > 0
	plan.Ready = total == 0
	return nil
}

// resolveMergedMarkers 用各冲突的解决值替换 merged_content 中的三方冲突标记块；
// 未决且无 resolved_value 的块保持原样，避免静默写错配置。
// resolveMergedMarkers replaces three-way conflict marker blocks in merged_content
// with each conflict's resolved value; blocks that stay unresolved keep their
// markers instead of silently writing wrong config.
func resolveMergedMarkers(mergedContent string, conflictsByID map[string]*ConfigConflict) string {
	lines := splitConfigLines(mergedContent)
	out := make([]string, 0, len(lines))
	for index := 0; index < len(lines); index++ {
		line := lines[index]
		id, ok := conflictMarkerID(line, "<<<<<<< LOCAL [")
		if !ok {
			out = append(out, line)
			continue
		}
		// 跳过整个标记块直到匹配的 TARGET 结尾行。
		// Skip the whole marker block up to the matching TARGET end line.
		end := index
		for end < len(lines) {
			endID, endOK := conflictMarkerID(lines[end], ">>>>>>> TARGET [")
			if endOK && endID == id {
				break
			}
			end++
		}
		if end >= len(lines) {
			// 标记块不完整时保守保留原行。
			// Keep the original line when the marker block is incomplete.
			out = append(out, line)
			continue
		}
		conflict := conflictsByID[id]
		if conflict != nil && conflict.Status == ConfigConflictResolved && conflict.ResolvedValue != visualDeleteSentinel {
			resolved := strings.TrimRight(conflict.ResolvedValue, "\n")
			if resolved != "" {
				out = append(out, strings.Split(resolved, "\n")...)
			}
		} else {
			// 未解决的块原样保留，交由服务端契约继续拦截。
			// Unresolved blocks are preserved so the server-side contract still blocks them.
			out = append(out, lines[index:end+1]...)
		}
		index = end
	}
	return strings.Join(out, "\n")
}

// conflictMarkerID 从 <<<<<<< LOCAL [id] / >>>>>>> TARGET [id] 行提取冲突编号。
// conflictMarkerID extracts the conflict ID from marker lines like
// <<<<<<< LOCAL [id] or >>>>>>> TARGET [id].
func conflictMarkerID(line, prefix string) (string, bool) {
	if !strings.HasPrefix(line, prefix) || !strings.HasSuffix(line, "]") {
		return "", false
	}
	return strings.TrimSuffix(strings.TrimPrefix(line, prefix), "]"), true
}
