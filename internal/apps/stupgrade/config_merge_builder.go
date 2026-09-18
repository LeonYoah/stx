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
			uniqueNodeContents := uniqueConfigContents(nodes)
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
