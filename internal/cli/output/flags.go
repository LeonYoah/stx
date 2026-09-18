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
package output

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// AddGlobalFlags 为根命令登记统一输出参数。
// AddGlobalFlags registers shared output flags on the root command.
func AddGlobalFlags(command *cobra.Command) {
	command.PersistentFlags().String("output", string(FormatJSON), "Output format: json, table, yaml, or raw")
	command.PersistentFlags().StringP("format", "f", "", "Alias for --output")
	command.PersistentFlags().String("pick", "", "Comma-separated top-level data fields to keep")
}

// OptionsFromCommand 从 Cobra 命令解析统一输出参数。
// OptionsFromCommand parses shared output options from a Cobra command.
func OptionsFromCommand(command *cobra.Command) (Options, error) {
	outputValue := string(FormatJSON)
	if command.Flag("output") != nil {
		var err error
		outputValue, err = command.Flags().GetString("output")
		if err != nil {
			return Options{}, WrapError(err, CodeUsage, err.Error(), ExitUsage, false)
		}
	}
	formatValue := ""
	if command.Flag("format") != nil {
		var err error
		formatValue, err = command.Flags().GetString("format")
		if err != nil {
			return Options{}, WrapError(err, CodeUsage, err.Error(), ExitUsage, false)
		}
	}
	outputChanged := command.Flag("output") != nil && command.Flag("output").Changed
	formatChanged := command.Flag("format") != nil && command.Flag("format").Changed
	if outputChanged && formatChanged && !strings.EqualFold(strings.TrimSpace(outputValue), strings.TrimSpace(formatValue)) {
		return Options{}, NewError(CodeUsage, "--output and --format must use the same value when both are provided", ExitUsage, false)
	}
	if formatChanged {
		outputValue = formatValue
	}

	format, err := ParseFormat(outputValue)
	if err != nil {
		return Options{}, err
	}
	pickValue := ""
	if command.Flag("pick") != nil {
		var err error
		pickValue, err = command.Flags().GetString("pick")
		if err != nil {
			return Options{}, WrapError(err, CodeUsage, err.Error(), ExitUsage, false)
		}
	}
	return Options{Format: format, Pick: ParsePick(pickValue)}, nil
}

// ParseFormat 校验输出格式。
// ParseFormat validates an output format.
func ParseFormat(value string) (Format, error) {
	format := Format(strings.ToLower(strings.TrimSpace(value)))
	switch format {
	case FormatJSON, FormatTable, FormatYAML, FormatRaw:
		return format, nil
	default:
		return "", NewError(CodeUsage, fmt.Sprintf("unsupported output format %q", value), ExitUsage, false)
	}
}

// ParsePick 解析并去重顶层字段名称。
// ParsePick parses and deduplicates top-level field names.
func ParsePick(value string) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0)
	for _, field := range strings.Split(value, ",") {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		if _, ok := seen[field]; ok {
			continue
		}
		seen[field] = struct{}{}
		result = append(result, field)
	}
	return result
}
