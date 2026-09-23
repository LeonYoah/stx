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
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"

	"gopkg.in/yaml.v3"
)

// Renderer 将安全业务结果写入 stdout，并将处理事件写入 stderr。
// Renderer writes safe business results to stdout and processing events to stderr.
type Renderer struct {
	stdout io.Writer
	events *EventWriter
}

// NewRenderer 创建结果渲染器。
// NewRenderer creates a result renderer.
func NewRenderer(stdout io.Writer, events *EventWriter) *Renderer {
	return &Renderer{stdout: stdout, events: events}
}

// Render 应用字段选择后输出结果。
// Render applies field selection and writes the result.
func (r *Renderer) Render(result Result, options Options) error {
	if options.Format == "" {
		options.Format = FormatJSON
	}
	if len(options.Pick) > 0 {
		picked, missing, err := pickData(result.Data, options.Pick)
		if err != nil {
			return WrapError(err, CodeServer, "failed to apply --pick", ExitServer, false)
		}
		if len(missing) > 0 {
			if r.events != nil {
				if err := r.events.Emit(Event{
					Event:         "pick_fallback",
					OperationID:   result.OperationID,
					RequestID:     result.RequestID,
					MissingFields: missing,
				}); err != nil {
					return WrapError(err, CodeServer, "failed to write pick fallback event", ExitServer, false)
				}
			}
		} else {
			result.Data = picked
		}
	}

	switch options.Format {
	case FormatJSON:
		encoder := json.NewEncoder(r.stdout)
		encoder.SetEscapeHTML(false)
		return encoder.Encode(result)
	case FormatYAML:
		return yaml.NewEncoder(r.stdout).Encode(result)
	case FormatTable:
		return writeTable(r.stdout, result)
	case FormatRaw:
		return writeRaw(r.stdout, result.Data)
	default:
		return NewError(CodeUsage, fmt.Sprintf("unsupported output format %q", options.Format), ExitUsage, false)
	}
}

func pickData(data any, fields []string) (any, []string, error) {
	normalized, err := normalizeJSONValue(data)
	if err != nil {
		return nil, nil, err
	}
	switch value := normalized.(type) {
	case map[string]any:
		picked, missing := pickObject(value, fields)
		if len(missing) > 0 {
			return data, missing, nil
		}
		return picked, nil, nil
	case []any:
		if len(value) == 0 {
			return value, nil, nil
		}
		picked := make([]any, 0, len(value))
		missingSet := make(map[string]struct{})
		for _, item := range value {
			object, ok := item.(map[string]any)
			if !ok {
				for _, field := range fields {
					missingSet[field] = struct{}{}
				}
				continue
			}
			selected, missing := pickObject(object, fields)
			picked = append(picked, selected)
			for _, field := range missing {
				missingSet[field] = struct{}{}
			}
		}
		if len(missingSet) > 0 {
			return data, orderedMissingFields(fields, missingSet), nil
		}
		return picked, nil, nil
	default:
		missingSet := make(map[string]struct{}, len(fields))
		for _, field := range fields {
			missingSet[field] = struct{}{}
		}
		return data, orderedMissingFields(fields, missingSet), nil
	}
}

func pickObject(object map[string]any, fields []string) (map[string]any, []string) {
	result := make(map[string]any, len(fields))
	missing := make([]string, 0)
	for _, field := range fields {
		value, ok := object[field]
		if !ok {
			missing = append(missing, field)
			continue
		}
		result[field] = value
	}
	return result, missing
}

func orderedMissingFields(fields []string, missingSet map[string]struct{}) []string {
	result := make([]string, 0, len(missingSet))
	for _, field := range fields {
		if _, ok := missingSet[field]; ok {
			result = append(result, field)
		}
	}
	return result
}

func normalizeJSONValue(value any) (any, error) {
	content, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var normalized any
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.UseNumber()
	if err := decoder.Decode(&normalized); err != nil {
		return nil, err
	}
	return normalized, nil
}

func writeRaw(writer io.Writer, data any) error {
	switch value := data.(type) {
	case string:
		_, err := fmt.Fprintln(writer, value)
		return err
	case []byte:
		if _, err := writer.Write(value); err != nil {
			return err
		}
		if len(value) == 0 || value[len(value)-1] != '\n' {
			_, err := fmt.Fprintln(writer)
			return err
		}
		return nil
	default:
		return json.NewEncoder(writer).Encode(data)
	}
}

func writeTable(writer io.Writer, result Result) error {
	table := tabwriter.NewWriter(writer, 0, 4, 2, ' ', 0)
	if err := writeTableData(table, result.Data); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(table); err != nil {
		return err
	}
	metadata := [][2]string{
		{"API_VERSION", result.APIVersion},
		{"OPERATION_ID", result.OperationID},
		{"REQUEST_ID", result.RequestID},
		{"COMPLETE", fmt.Sprintf("%t", result.ResultMeta.Complete)},
	}
	if result.ResultMeta.Reason != "" {
		metadata = append(metadata, [2]string{"REASON", result.ResultMeta.Reason})
	}
	if result.ResultMeta.NextCommand != "" {
		metadata = append(metadata, [2]string{"NEXT_COMMAND", result.ResultMeta.NextCommand})
	}
	if result.Impact != nil {
		metadata = append(metadata, [2]string{"IMPACT", tableCell(result.Impact)})
	}
	for _, item := range metadata {
		if _, err := fmt.Fprintf(table, "%s\t%s\n", item[0], item[1]); err != nil {
			return err
		}
	}
	return table.Flush()
}

func writeTableData(writer io.Writer, data any) error {
	normalized, err := normalizeJSONValue(data)
	if err != nil {
		return err
	}
	switch value := normalized.(type) {
	case []any:
		if len(value) == 0 {
			_, err := fmt.Fprintln(writer, "NO RESULTS")
			return err
		}
		objects := make([]map[string]any, 0, len(value))
		columnsSet := make(map[string]struct{})
		for _, item := range value {
			object, ok := item.(map[string]any)
			if !ok {
				object = map[string]any{"value": item}
			}
			objects = append(objects, object)
			for column := range object {
				columnsSet[column] = struct{}{}
			}
		}
		columns := make([]string, 0, len(columnsSet))
		for column := range columnsSet {
			columns = append(columns, column)
		}
		sort.Strings(columns)
		if _, err := fmt.Fprintln(writer, strings.ToUpper(strings.Join(columns, "\t"))); err != nil {
			return err
		}
		for _, object := range objects {
			cells := make([]string, len(columns))
			for index, column := range columns {
				cells[index] = tableCell(object[column])
			}
			if _, err := fmt.Fprintln(writer, strings.Join(cells, "\t")); err != nil {
				return err
			}
		}
		return nil
	case map[string]any:
		keys := make([]string, 0, len(value))
		for key := range value {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		if _, err := fmt.Fprintln(writer, "FIELD\tVALUE"); err != nil {
			return err
		}
		for _, key := range keys {
			if _, err := fmt.Fprintf(writer, "%s\t%s\n", key, tableCell(value[key])); err != nil {
				return err
			}
		}
		return nil
	default:
		_, err := fmt.Fprintf(writer, "VALUE\n%s\n", tableCell(value))
		return err
	}
}

func tableCell(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	case bool:
		return fmt.Sprintf("%t", typed)
	default:
		content, err := json.Marshal(typed)
		if err != nil {
			return fmt.Sprint(typed)
		}
		return string(content)
	}
}
