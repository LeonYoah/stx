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

// Format 表示 CLI 最终输出格式。
// Format represents the final CLI output format.
type Format string

const (
	FormatJSON  Format = "json"
	FormatTable Format = "table"
	FormatYAML  Format = "yaml"
	FormatRaw   Format = "raw"
)

// ResultMeta 只保存跨命令通用的完整性信息。
// ResultMeta stores only completeness information shared across commands.
type ResultMeta struct {
	Complete    bool   `json:"complete" yaml:"complete"`
	Reason      string `json:"reason,omitempty" yaml:"reason,omitempty"`
	NextCommand string `json:"next_command,omitempty" yaml:"next_command,omitempty"`
}

// Result 是普通 CLI 命令的稳定输出结构。
// Result is the stable output envelope for regular CLI commands.
type Result struct {
	APIVersion  string     `json:"api_version" yaml:"api_version"`
	OperationID string     `json:"operation_id" yaml:"operation_id"`
	RequestID   string     `json:"request_id" yaml:"request_id"`
	Data        any        `json:"data" yaml:"data"`
	Impact      any        `json:"impact,omitempty" yaml:"impact,omitempty"`
	ResultMeta  ResultMeta `json:"result_meta" yaml:"result_meta"`
}

// Options 控制本次结果的格式和字段选择。
// Options controls formatting and field selection for one result.
type Options struct {
	Format Format
	Pick   []string
}

// NewResult 创建完整的普通命令结果。
// NewResult creates a complete regular command result.
func NewResult(operationID, requestID string, data any) Result {
	return Result{
		APIVersion:  "v1",
		OperationID: operationID,
		RequestID:   requestID,
		Data:        data,
		ResultMeta: ResultMeta{
			Complete: true,
		},
	}
}
