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

package audit

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"
)

const redactedValue = "******"

var (
	sensitiveKeySuffixes = []string{
		"password", "passwd", "pwd", "secret", "token", "credential",
		"authorization", "cookie", "private_key", "api_key", "access_key", "secret_key",
		"password_value", "secret_value", "token_value", "credential_value",
		"access_key_id", "secret_access_key", "client_secret",
	}
	assignmentPattern = regexp.MustCompile(`(?m)(["']?)([A-Za-z_][A-Za-z0-9_.-]*)(["']?)([ \t]*[:=][ \t]*)("[^"\\]*(?:\\.[^"\\]*)*"|'[^'\\]*(?:\\.[^'\\]*)*'|\{\{[^{}]*\}\}|\$\{[^{}]*\}|[^\s,;}\]]+)`)
	camelBoundary     = regexp.MustCompile(`([a-z0-9])([A-Z])`)
	// URL 用户信息中的密码不带 password 字段名，需单独处理。 / Passwords in URL userinfo have no password key and need separate handling.
	urlUserinfoPattern = regexp.MustCompile(`(?i)([a-z][a-z0-9+.-]*://[^\s/@:'"<>]+:)[^\s/@'"<>]+(@)`)
	// URL 查询参数的密钥可能藏在普通 url 字段值里。 / URL query secrets may hide inside a non-sensitive url field.
	urlQueryPattern    = regexp.MustCompile(`([?&])([A-Za-z_][A-Za-z0-9_.-]*)(=)([^&#\s'"<>]+)`)
	blockScalarPattern = regexp.MustCompile(`^([ \t]*)([A-Za-z_][A-Za-z0-9_.-]*)([ \t]*[:=][ \t]*)([|>][-+]?)`)
)

// IsVariablePlaceholder 判断给定值是否为模板变量或占位符表达式（如 {{var}}、${var}）。
// 这类变量引用不应被脱敏替换，因为它们是变量名而非明文凭据。
// IsVariablePlaceholder reports whether a value represents a template variable or placeholder.
func IsVariablePlaceholder(value string) bool {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) >= 2 && ((trimmed[0] == '"' && trimmed[len(trimmed)-1] == '"') || (trimmed[0] == '\'' && trimmed[len(trimmed)-1] == '\'')) {
		trimmed = strings.TrimSpace(trimmed[1 : len(trimmed)-1])
	}
	return (strings.HasPrefix(trimmed, "{{") && strings.HasSuffix(trimmed, "}}")) ||
		(strings.HasPrefix(trimmed, "${") && strings.HasSuffix(trimmed, "}"))
}

// RedactDetails 递归屏蔽审计详情中的密码、令牌、密钥和凭证。
// RedactDetails recursively masks passwords, tokens, keys, and credentials in audit details.
func RedactDetails(details AuditDetails) AuditDetails {
	if details == nil {
		return nil
	}
	redacted, ok := redactValue(details).(map[string]any)
	if !ok {
		return AuditDetails{}
	}
	return AuditDetails(redacted)
}

// RedactParameters 递归屏蔽 Agent 命令参数中的敏感值。
// RedactParameters recursively masks sensitive values in Agent command parameters.
func RedactParameters(parameters CommandParameters) CommandParameters {
	if parameters == nil {
		return nil
	}
	redacted, ok := redactValue(parameters).(map[string]any)
	if !ok {
		return CommandParameters{}
	}
	return CommandParameters(redacted)
}

// RedactText 屏蔽 JSON、HOCON 和常见 key=value 文本中的敏感值。
// RedactText masks sensitive values in JSON, HOCON, and common key=value text.
func RedactText(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return value
	}
	var decoded any
	if json.Unmarshal([]byte(trimmed), &decoded) == nil {
		if encoded, err := json.Marshal(redactValue(decoded)); err == nil {
			return urlUserinfoPattern.ReplaceAllString(string(encoded), `${1}******${2}`)
		}
	}
	// 块状 YAML 值必须整块移除，仅替换首行会把后续密钥留在产物中。 / Remove the entire YAML block; replacing only its header would leave the secret behind.
	value = redactSensitiveBlockScalars(value)
	// 先处理地址中的查询参数，避免外层 url 字段把内层 password=... 整段吞掉。
	// Mask query parameters first so an outer URL assignment cannot swallow password=... inside it.
	value = urlUserinfoPattern.ReplaceAllString(value, `${1}******${2}`)
	value = urlQueryPattern.ReplaceAllStringFunc(value, func(match string) string {
		parts := urlQueryPattern.FindStringSubmatch(match)
		if !isSensitiveKey(parts[2]) || IsVariablePlaceholder(parts[4]) {
			return match
		}
		return parts[1] + parts[2] + parts[3] + redactedValue
	})
	redacted := assignmentPattern.ReplaceAllStringFunc(value, func(match string) string {
		parts := assignmentPattern.FindStringSubmatch(match)
		if len(parts) < 6 || !isSensitiveKey(parts[2]) {
			return match
		}
		if IsVariablePlaceholder(parts[5]) {
			return match
		}
		return parts[1] + parts[2] + parts[3] + parts[4] + redactedValue
	})
	return urlUserinfoPattern.ReplaceAllString(redacted, `${1}******${2}`)
}

// redactSensitiveBlockScalars 移除敏感字段对应的 YAML 多行正文，同时保留其余行。
// redactSensitiveBlockScalars removes YAML multiline bodies for sensitive keys while keeping other lines.
func redactSensitiveBlockScalars(value string) string {
	lines := strings.Split(value, "\n")
	result := make([]string, 0, len(lines))
	blockIndent := -1
	for _, line := range lines {
		indent := len(line) - len(strings.TrimLeft(line, " \t"))
		if blockIndent >= 0 {
			if strings.TrimSpace(line) == "" || indent > blockIndent {
				continue
			}
			blockIndent = -1
		}
		matches := blockScalarPattern.FindStringSubmatch(line)
		if len(matches) == 5 && isSensitiveKey(matches[2]) {
			result = append(result, matches[1]+matches[2]+matches[3]+redactedValue)
			blockIndent = len(matches[1])
			continue
		}
		result = append(result, line)
	}
	return strings.Join(result, "\n")
}

func redactValue(value any) any {
	switch current := value.(type) {
	case nil:
		return nil
	case map[string]any:
		result := make(map[string]any, len(current))
		for key, item := range current {
			if isSensitiveKey(key) {
				if str, ok := item.(string); ok && IsVariablePlaceholder(str) {
					result[key] = item
					continue
				}
				result[key] = redactedValue
				continue
			}
			result[key] = redactValue(item)
		}
		return result
	case []any:
		result := make([]any, len(current))
		for index, item := range current {
			result[index] = redactValue(item)
		}
		return result
	case string:
		return RedactText(current)
	}

	reflected := reflect.ValueOf(value)
	if !reflected.IsValid() {
		return nil
	}
	switch reflected.Kind() {
	case reflect.Map, reflect.Slice, reflect.Array, reflect.Struct, reflect.Pointer:
		content, err := json.Marshal(value)
		if err != nil {
			return fmt.Sprint(value)
		}
		var decoded any
		if err := json.Unmarshal(content, &decoded); err != nil {
			return fmt.Sprint(value)
		}
		return redactValue(decoded)
	default:
		return value
	}
}

func isSensitiveKey(key string) bool {
	normalized := camelBoundary.ReplaceAllString(strings.TrimSpace(key), `${1}_${2}`)
	normalized = strings.ToLower(normalized)
	normalized = strings.NewReplacer("-", "_", ".", "_").Replace(normalized)
	for _, suffix := range sensitiveKeySuffixes {
		if normalized == suffix || strings.HasSuffix(normalized, "_"+suffix) {
			return true
		}
	}
	return false
}

// IsSensitiveKey 判断字段名是否表示密码、令牌或密钥。
// IsSensitiveKey reports whether a field name represents a password, token, or key.
func IsSensitiveKey(key string) bool {
	return isSensitiveKey(key)
}
