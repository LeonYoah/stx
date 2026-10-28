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
	"regexp"
	"strings"
)

var (
	// fieldsKeywordRegex 匹配 HOCON 中的 fields { 定义块
	// fieldsKeywordRegex matches fields { definition blocks in HOCON
	fieldsKeywordRegex = regexp.MustCompile(`(?i)\bfields\s*[:=]?\s*\{`)

	// fieldLineRegex 匹配 fields 块内的字段名声明行（支持带引号或不带引号）
	// fieldLineRegex matches field name declaration lines within fields blocks (with or without quotes)
	fieldLineRegex = regexp.MustCompile(`(?m)^\s*["']?([a-zA-Z0-9_\-\.]+)["']?\s*[:=]`)
)

// extractOrderedKeysFromJSON 从 JSON 原始字节流中按词法解析提取顶层或行对象的字段顺序。
// extractOrderedKeysFromJSON lexically extracts field keys in their exact appearance order from raw JSON bytes.
func extractOrderedKeysFromJSON(data []byte) []string {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	return scanJSONForKeys(dec)
}

func scanJSONForKeys(dec *json.Decoder) []string {
	var keys []string
	seen := make(map[string]bool)

	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		delim, ok := tok.(json.Delim)
		if !ok {
			continue
		}
		if delim == '{' {
			for dec.More() {
				keyTok, err := dec.Token()
				if err != nil {
					break
				}
				keyStr, ok := keyTok.(string)
				if !ok {
					break
				}
				var val json.RawMessage
				if err := dec.Decode(&val); err != nil {
					break
				}
				if keyStr == "columns" {
					var colList []string
					if err := json.Unmarshal(val, &colList); err == nil && len(colList) > 0 {
						return colList
					}
				}
				if keyStr == "rows" || keyStr == "data" {
					if subKeys := extractOrderedKeysFromJSON(val); len(subKeys) > 0 {
						return subKeys
					}
				}
				trimmed := strings.TrimSpace(keyStr)
				if trimmed == "__st_debug_rowkind" {
					trimmed = "RowKind"
				} else if strings.HasPrefix(trimmed, "__st_debug_") {
					continue
				}
				if trimmed != "" && !seen[trimmed] {
					seen[trimmed] = true
					keys = append(keys, trimmed)
				}
			}
			if len(keys) > 0 {
				return keys
			}
		}
	}
	return keys
}

// extractSchemaFieldNamesFromHocon 从 HOCON 脚本（如 preview_content 或 task.Content）中解析 schema.fields 声明的字段顺序。
// extractSchemaFieldNamesFromHocon parses field names in declaration order from schema.fields in HOCON script.
func extractSchemaFieldNamesFromHocon(content string) []string {
	if strings.TrimSpace(content) == "" {
		return nil
	}
	var fieldNames []string
	seen := make(map[string]bool)

	locs := fieldsKeywordRegex.FindAllStringIndex(content, -1)
	for _, loc := range locs {
		openBraceIdx := loc[1] - 1
		depth := 1
		endIdx := -1
		for i := openBraceIdx + 1; i < len(content); i++ {
			if content[i] == '{' {
				depth++
			} else if content[i] == '}' {
				depth--
				if depth == 0 {
					endIdx = i
					break
				}
			}
		}
		if endIdx == -1 {
			continue
		}
		block := content[openBraceIdx+1 : endIdx]
		lines := fieldLineRegex.FindAllStringSubmatch(block, -1)
		for _, lineMatch := range lines {
			if len(lineMatch) < 2 {
				continue
			}
			fieldName := strings.TrimSpace(lineMatch[1])
			if fieldName != "" && !seen[fieldName] {
				seen[fieldName] = true
				fieldNames = append(fieldNames, fieldName)
			}
		}
	}
	return fieldNames
}

// alignPreviewColumnsOrder 按照标记/定义优先级重排预览列：RowKind 首位、Schema 声明顺序优先、原始 JSON 顺序次之。
// alignPreviewColumnsOrder reorders preview columns: RowKind first, schema declaration order next, then raw JSON order.
func alignPreviewColumnsOrder(columns []string, schemaOrder []string, rawOrder []string) []string {
	if len(columns) == 0 {
		return columns
	}

	// 建立当前列的快速查找索引（原名 -> 大小写映射）
	// Build fast lookup for current columns (original name -> case-insensitive map)
	colMap := make(map[string]string, len(columns))
	hasRowKind := false
	for _, c := range columns {
		trimmed := strings.TrimSpace(c)
		if strings.EqualFold(trimmed, "RowKind") {
			hasRowKind = true
			continue
		}
		colMap[strings.ToLower(trimmed)] = trimmed
	}

	result := make([]string, 0, len(columns))
	used := make(map[string]bool, len(columns))

	// 1. RowKind 始终保持在最前列
	// 1. RowKind always stays at index 0
	if hasRowKind {
		result = append(result, "RowKind")
		used["rowkind"] = true
	}

	// 2. 优先按 Schema 声明顺序（标记顺序）排布
	// 2. Prioritize schema declaration order (marked order)
	for _, sf := range schemaOrder {
		lower := strings.ToLower(strings.TrimSpace(sf))
		if actual, ok := colMap[lower]; ok && !used[lower] {
			result = append(result, actual)
			used[lower] = true
		}
	}

	// 3. 次选按原始 JSON 顺序排布未在 Schema 中声明的字段
	// 3. Next, append remaining fields in raw JSON appearance order
	for _, rf := range rawOrder {
		lower := strings.ToLower(strings.TrimSpace(rf))
		if actual, ok := colMap[lower]; ok && !used[lower] {
			result = append(result, actual)
			used[lower] = true
		}
	}

	// 4. 兜底追加其余未匹配字段（保持原列表顺序）
	// 4. Fallback: append any remaining unmatched fields in original list order
	for _, c := range columns {
		lower := strings.ToLower(strings.TrimSpace(c))
		if actual, ok := colMap[lower]; ok && !used[lower] {
			result = append(result, actual)
			used[lower] = true
		}
	}

	return result
}

// extractPreviewSchemaOrderFromJob 从作业实例中提取 Schema 字段声明顺序。
// extractPreviewSchemaOrderFromJob extracts schema field declaration order from a job instance.
func extractPreviewSchemaOrderFromJob(instance *JobInstance, taskContent string) []string {
	if instance != nil && instance.ResultPreview != nil {
		if script := strings.TrimSpace(stringValue(instance.ResultPreview, "preview_content")); script != "" {
			if fields := extractSchemaFieldNamesFromHocon(script); len(fields) > 0 {
				return fields
			}
		}
	}
	if strings.TrimSpace(taskContent) != "" {
		if fields := extractSchemaFieldNamesFromHocon(taskContent); len(fields) > 0 {
			return fields
		}
	}
	return nil
}

// equalStringSlices 比较两个字符串切片内容与顺序是否完全一致。
// equalStringSlices checks if two string slices have identical content and order.
func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
