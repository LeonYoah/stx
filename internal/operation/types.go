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

// Package operation 定义 STX API、CLI 与能力查询共用的操作登记结构。
// Package operation defines the shared operation registry used by the STX API, CLI, and capability discovery.
package operation

// OperationMode 表示操作需要采用的客户端处理方式。
// OperationMode identifies the client behavior required by an operation.
type OperationMode string

const (
	ModeNormal     OperationMode = "normal"
	ModeWatch      OperationMode = "watch"
	ModeDownload   OperationMode = "download"
	ModeServerOnly OperationMode = "server_only"
	ModeProxy      OperationMode = "proxy"
)

// RiskLevel 表示服务端评估的操作风险等级。
// RiskLevel identifies the server-assessed risk level of an operation.
type RiskLevel string

const (
	RiskR0 RiskLevel = "R0"
	RiskR1 RiskLevel = "R1"
	RiskR2 RiskLevel = "R2"
	RiskR3 RiskLevel = "R3"
)

// InputLocation 表示输入参数在 HTTP 请求中的位置。
// InputLocation identifies where an input is carried in an HTTP request.
type InputLocation string

const (
	InputPath   InputLocation = "path"
	InputQuery  InputLocation = "query"
	InputHeader InputLocation = "header"
	InputBody   InputLocation = "body"
	InputFile   InputLocation = "file"
)

// ImpactSpec 描述执行操作前需要展示的影响信息。
// ImpactSpec describes impact information that must be shown before execution.
type ImpactSpec struct {
	Level       RiskLevel `json:"level" yaml:"level"`
	Message     string    `json:"message" yaml:"message"`
	Performance string    `json:"performance,omitempty" yaml:"performance,omitempty"`
}

// InputSpec 描述一个可由 CLI 传入的操作参数。
// InputSpec describes an operation input accepted by the CLI.
type InputSpec struct {
	Name        string        `json:"name" yaml:"name"`
	Location    InputLocation `json:"location" yaml:"location"`
	Required    bool          `json:"required" yaml:"required"`
	Description string        `json:"description" yaml:"description"`
}

// OperationSpec 是普通 API 路由、CLI 命令和能力查询共用的稳定登记项。
// OperationSpec is the stable registry entry shared by API routes, CLI commands, and capability discovery.
type OperationSpec struct {
	ID               string        `json:"operation_id" yaml:"operation_id"`
	CommandPath      []string      `json:"command_path" yaml:"command_path"`
	Summary          string        `json:"summary,omitempty" yaml:"summary,omitempty"`
	GeneratedCLI     bool          `json:"generated_cli" yaml:"generated_cli"`
	Method           string        `json:"method" yaml:"method"`
	Route            string        `json:"route" yaml:"route"`
	Mode             OperationMode `json:"mode" yaml:"mode"`
	AuthRequired     bool          `json:"auth_required" yaml:"auth_required"`
	Risk             RiskLevel     `json:"risk" yaml:"risk"`
	AdminOnly        bool          `json:"admin_only" yaml:"admin_only"`
	Revision         int           `json:"revision" yaml:"revision"`
	UsesAgent        bool          `json:"uses_agent" yaml:"uses_agent"`
	Async            bool          `json:"async" yaml:"async"`
	SupportsPick     bool          `json:"supports_pick" yaml:"supports_pick"`
	SupportsRevision bool          `json:"supports_revision" yaml:"supports_revision"`
	Impact           *ImpactSpec   `json:"impact,omitempty" yaml:"impact,omitempty"`
	Input            []InputSpec   `json:"input,omitempty" yaml:"input,omitempty"`
	Example          string        `json:"example" yaml:"example"`
	OutputExample    string        `json:"output_example" yaml:"output_example"`
}

// RouteException 记录不生成普通 CLI 命令的特殊路由。
// RouteException records a special route that does not generate a normal CLI command.
type RouteException struct {
	Method string        `json:"method" yaml:"method"`
	Route  string        `json:"route" yaml:"route"`
	Mode   OperationMode `json:"mode" yaml:"mode"`
	Reason string        `json:"reason" yaml:"reason"`
}
