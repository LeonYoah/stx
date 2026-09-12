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
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	platformVariablePattern        = regexp.MustCompile(`\{\{\s*([^{}]+?)\s*\}\}`)
	reservedBuiltinVariablePattern = regexp.MustCompile(`^(system\.[A-Za-z0-9_.-]+|[A-Za-z0-9_:\-().,+*/\s]+)$`)
	timeOffsetExpressionPattern    = regexp.MustCompile(`^(.+?)([+-])([0-9*/. ]+)$`)
	functionCallPattern            = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)\((.*)\)$`)
	javaFormatTokenPattern         = regexp.MustCompile(`yyyy|MM|dd|HH|mm|ss`)
	supportedBuiltinVariableKeySet = map[string]struct{}{
		"system.biz.date":                 {},
		"system.biz.curdate":              {},
		"system.datetime":                 {},
		"system.task.execute.path":        {},
		"system.task.instance.id":         {},
		"system.task.definition.name":     {},
		"system.task.definition.code":     {},
		"system.workflow.instance.id":     {},
		"system.workflow.definition.name": {},
		"system.workflow.definition.code": {},
		"system.project.name":             {},
		"system.project.code":             {},
	}
)

type taskVariableRuntime struct {
	ReferenceTime          time.Time
	PlatformJobID          string
	TaskDefinitionName     string
	TaskDefinitionCode     string
	WorkflowInstanceID     string
	WorkflowDefinitionName string
	WorkflowDefinitionCode string
	ProjectName            string
	ProjectCode            string
	TaskExecutePath        string
}

func detectTemplateVariables(content string) []string {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}
	seen := map[string]struct{}{}
	result := make([]string, 0)
	matches := platformVariablePattern.FindAllStringSubmatch(content, -1)
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		name := strings.TrimSpace(match[1])
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

func isReservedBuiltinVariableKey(key string) bool {
	key = strings.TrimSpace(key)
	if key == "" {
		return false
	}
	if _, ok := supportedBuiltinVariableKeySet[key]; ok {
		return true
	}
	if !reservedBuiltinVariablePattern.MatchString(key) {
		return false
	}
	return looksLikeTimeExpression(key)
}

func validateVariableKeyConflicts(variables map[string]string) error {
	for key := range variables {
		if isReservedBuiltinVariableKey(key) {
			return fmt.Errorf("%w: {{%s}}", ErrReservedBuiltinVariableKey, strings.TrimSpace(key))
		}
	}
	return nil
}

func resolveTemplateExpression(expr string, variables map[string]string, runtime *taskVariableRuntime) (string, bool) {
	key := strings.TrimSpace(expr)
	if key == "" {
		return "", false
	}
	if value, ok := variables[key]; ok {
		return value, true
	}
	if value, ok := resolveBuiltinVariable(key, runtime); ok {
		return value, true
	}
	return "", false
}

func replaceTemplateVariables(content string, variables map[string]string, runtime *taskVariableRuntime) string {
	if strings.TrimSpace(content) == "" {
		return content
	}
	return platformVariablePattern.ReplaceAllStringFunc(content, func(match string) string {
		parts := platformVariablePattern.FindStringSubmatch(match)
		if len(parts) < 2 {
			return match
		}
		if value, ok := resolveTemplateExpression(parts[1], variables, runtime); ok {
			return value
		}
		return match
	})
}

func resolveBuiltinVariable(key string, runtime *taskVariableRuntime) (string, bool) {
	now := time.Now()
	if runtime != nil && !runtime.ReferenceTime.IsZero() {
		now = runtime.ReferenceTime
	}
	switch strings.TrimSpace(key) {
	case "system.biz.date":
		return now.AddDate(0, 0, -1).Format("20060102"), true
	case "system.biz.curdate":
		return now.Format("20060102"), true
	case "system.datetime":
		return now.Format("20060102150405"), true
	case "system.task.execute.path":
		if runtime != nil {
			return strings.TrimSpace(runtime.TaskExecutePath), true
		}
		return "", true
	case "system.task.instance.id":
		if runtime != nil {
			return strings.TrimSpace(runtime.PlatformJobID), true
		}
		return "", true
	case "system.task.definition.name":
		if runtime != nil {
			return strings.TrimSpace(runtime.TaskDefinitionName), true
		}
		return "", true
	case "system.task.definition.code":
		if runtime != nil {
			return strings.TrimSpace(runtime.TaskDefinitionCode), true
		}
		return "", true
	case "system.workflow.instance.id":
		if runtime != nil {
			return strings.TrimSpace(runtime.WorkflowInstanceID), true
		}
		return "", true
	case "system.workflow.definition.name":
		if runtime != nil {
			return strings.TrimSpace(runtime.WorkflowDefinitionName), true
		}
		return "", true
	case "system.workflow.definition.code":
		if runtime != nil {
			return strings.TrimSpace(runtime.WorkflowDefinitionCode), true
		}
		return "", true
	case "system.project.name":
		if runtime != nil && strings.TrimSpace(runtime.ProjectName) != "" {
			return strings.TrimSpace(runtime.ProjectName), true
		}
		return "STX", true
	case "system.project.code":
		if runtime != nil && strings.TrimSpace(runtime.ProjectCode) != "" {
			return strings.TrimSpace(runtime.ProjectCode), true
		}
		return "stx", true
	}
	if !looksLikeTimeExpression(key) {
		return "", false
	}
	value, err := formatBuiltinTimeExpression(key, now)
	if err != nil {
		return "", false
	}
	return value, true
}

func looksLikeTimeExpression(expr string) bool {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return false
	}
	if functionCallPattern.MatchString(expr) {
		return true
	}
	return javaFormatTokenPattern.MatchString(expr)
}

func formatBuiltinTimeExpression(expr string, base time.Time) (string, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return "", fmt.Errorf("empty time expression")
	}
	if matches := functionCallPattern.FindStringSubmatch(expr); len(matches) == 3 {
		return resolveTimeFunction(matches[1], matches[2], base)
	}
	return formatOffsetTimeExpression(expr, base)
}

func resolveTimeFunction(name string, rawArgs string, base time.Time) (string, error) {
	args := splitFunctionArguments(rawArgs)
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "add_months":
		if len(args) != 2 {
			return "", fmt.Errorf("add_months requires 2 arguments")
		}
		offset, err := evaluateNumericExpression(args[1])
		if err != nil {
			return "", err
		}
		return formatWithJavaPattern(base.AddDate(0, int(math.Round(offset)), 0), args[0])
	case "this_day":
		if len(args) != 1 {
			return "", fmt.Errorf("this_day requires 1 argument")
		}
		return formatWithJavaPattern(base, args[0])
	case "last_day":
		if len(args) != 1 {
			return "", fmt.Errorf("last_day requires 1 argument")
		}
		return formatWithJavaPattern(base.AddDate(0, 0, -1), args[0])
	case "year_week":
		if len(args) < 1 || len(args) > 2 {
			return "", fmt.Errorf("year_week requires 1 or 2 arguments")
		}
		weekStart := time.Monday
		if len(args) == 2 {
			value, err := strconv.Atoi(strings.TrimSpace(args[1]))
			if err != nil {
				return "", err
			}
			weekStart = weekStartFromNumber(value)
		}
		year, week := yearWeek(base, weekStart)
		return formatYearWeek(args[0], year, week), nil
	case "month_first_day":
		return resolveMonthBoundaryFunction(args, base, true)
	case "month_last_day":
		return resolveMonthBoundaryFunction(args, base, false)
	case "week_first_day":
		return resolveWeekBoundaryFunction(args, base, true)
	case "week_last_day":
		return resolveWeekBoundaryFunction(args, base, false)
	default:
		return "", fmt.Errorf("unsupported time function: %s", name)
	}
}

func resolveMonthBoundaryFunction(args []string, base time.Time, first bool) (string, error) {
	if len(args) != 2 {
		return "", fmt.Errorf("month boundary function requires 2 arguments")
	}
	offset, err := evaluateNumericExpression(args[1])
	if err != nil {
		return "", err
	}
	target := base.AddDate(0, int(math.Round(offset)), 0)
	if first {
		target = time.Date(target.Year(), target.Month(), 1, target.Hour(), target.Minute(), target.Second(), target.Nanosecond(), target.Location())
	} else {
		target = time.Date(target.Year(), target.Month()+1, 0, target.Hour(), target.Minute(), target.Second(), target.Nanosecond(), target.Location())
	}
	return formatWithJavaPattern(target, args[0])
}

func resolveWeekBoundaryFunction(args []string, base time.Time, first bool) (string, error) {
	if len(args) != 2 {
		return "", fmt.Errorf("week boundary function requires 2 arguments")
	}
	offset, err := evaluateNumericExpression(args[1])
	if err != nil {
		return "", err
	}
	target := base.AddDate(0, 0, int(math.Round(offset))*7)
	start := beginningOfWeek(target, time.Monday)
	if !first {
		start = start.AddDate(0, 0, 6)
	}
	return formatWithJavaPattern(start, args[0])
}

func formatOffsetTimeExpression(expr string, base time.Time) (string, error) {
	formatExpr := strings.TrimSpace(expr)
	sign := ""
	offsetExpr := ""
	if matches := timeOffsetExpressionPattern.FindStringSubmatch(expr); len(matches) == 4 {
		formatExpr = strings.TrimSpace(matches[1])
		sign = matches[2]
		offsetExpr = strings.TrimSpace(matches[3])
	}
	target := base
	if offsetExpr != "" {
		offset, err := evaluateNumericExpression(offsetExpr)
		if err != nil {
			return "", err
		}
		if sign == "-" {
			offset = -offset
		}
		target = target.Add(time.Duration(offset * float64(24*time.Hour)))
	}
	return formatWithJavaPattern(target, formatExpr)
}

func evaluateNumericExpression(expr string) (float64, error) {
	expr = strings.ReplaceAll(strings.TrimSpace(expr), " ", "")
	if expr == "" {
		return 0, fmt.Errorf("empty numeric expression")
	}
	parts := strings.FieldsFunc(expr, func(r rune) bool { return r == '*' || r == '/' })
	if len(parts) == 0 {
		return 0, fmt.Errorf("invalid numeric expression")
	}
	ops := make([]rune, 0)
	for _, r := range expr {
		if r == '*' || r == '/' {
			ops = append(ops, r)
		}
	}
	value, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return 0, err
	}
	for index, op := range ops {
		next, err := strconv.ParseFloat(parts[index+1], 64)
		if err != nil {
			return 0, err
		}
		if op == '*' {
			value *= next
		} else {
			value /= next
		}
	}
	return value, nil
}

func formatWithJavaPattern(target time.Time, pattern string) (string, error) {
	layout := javaDateFormatToGoLayout(strings.TrimSpace(pattern))
	if layout == "" {
		return "", fmt.Errorf("unsupported date format")
	}
	return target.Format(layout), nil
}

func javaDateFormatToGoLayout(pattern string) string {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return ""
	}
	replacements := []struct{ from, to string }{
		{"yyyy", "2006"},
		{"MM", "01"},
		{"dd", "02"},
		{"HH", "15"},
		{"mm", "04"},
		{"ss", "05"},
	}
	result := pattern
	for _, item := range replacements {
		result = strings.ReplaceAll(result, item.from, item.to)
	}
	if strings.ContainsAny(result, "yMdHms") {
		return ""
	}
	return result
}

func splitFunctionArguments(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		result = append(result, strings.TrimSpace(part))
	}
	return result
}

func weekStartFromNumber(value int) time.Weekday {
	switch value {
	case 1:
		return time.Monday
	case 2:
		return time.Tuesday
	case 3:
		return time.Wednesday
	case 4:
		return time.Thursday
	case 5:
		return time.Friday
	case 6:
		return time.Saturday
	case 7:
		return time.Sunday
	default:
		return time.Monday
	}
}

func beginningOfWeek(target time.Time, start time.Weekday) time.Time {
	diff := (7 + int(target.Weekday()) - int(start)) % 7
	day := target.AddDate(0, 0, -diff)
	return time.Date(day.Year(), day.Month(), day.Day(), target.Hour(), target.Minute(), target.Second(), target.Nanosecond(), target.Location())
}

func yearWeek(target time.Time, start time.Weekday) (int, int) {
	startOfWeek := beginningOfWeek(target, start)
	firstOfYear := time.Date(startOfWeek.Year(), time.January, 1, startOfWeek.Hour(), startOfWeek.Minute(), startOfWeek.Second(), startOfWeek.Nanosecond(), startOfWeek.Location())
	firstWeekStart := beginningOfWeek(firstOfYear, start)
	week := int(startOfWeek.Sub(firstWeekStart).Hours()/(24*7)) + 1
	return startOfWeek.Year(), week
}

func formatYearWeek(pattern string, year, week int) string {
	formatted := strings.TrimSpace(pattern)
	formatted = strings.ReplaceAll(formatted, "yyyy", fmt.Sprintf("%04d", year))
	formatted = strings.ReplaceAll(formatted, "MM", fmt.Sprintf("%02d", week))
	return formatted
}
