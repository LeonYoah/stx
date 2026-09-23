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

package operation

import "fmt"

// hostWriteOperationSpecs 登记主机新增和修改操作。
// hostWriteOperationSpecs registers host create and update operations.
func hostWriteOperationSpecs() []OperationSpec {
	return []OperationSpec{
		hostWriteOperation(
			"host.create", []string{"host", "create"}, "Create a host", "POST", "/api/v1/hosts",
			"新增主机会保存连接信息，并允许后续安装 Agent 或管理运行环境。",
			"stx host create --name node-2 --ip-address 192.0.2.20 --confirm",
		),
		hostWriteOperation(
			"host.update", []string{"host", "update"}, "Update a host", "PUT", "/api/v1/hosts/:id",
			"修改主机连接信息可能影响后续 Agent 安装、状态检查和集群操作。",
			"stx host update 10 --description test-node --confirm",
		),
	}
}

// hostWriteOperation 构造由专用 CLI 处理的主机写操作登记。
// hostWriteOperation builds a host write registration handled by dedicated CLI code.
func hostWriteOperation(id string, commandPath []string, summary, method, route, impact, example string) OperationSpec {
	return OperationSpec{
		ID: id, CommandPath: commandPath, Summary: summary, GeneratedCLI: false, Method: method, Route: route,
		Mode: ModeNormal, AuthRequired: true, Risk: RiskR1, Revision: 1, SupportsPick: true,
		Impact: &ImpactSpec{Level: RiskR1, Message: impact},
		Input: []InputSpec{
			{Name: "Idempotency-Key", Location: InputHeader, Required: true, Description: "Stable retry key"},
			{Name: "X-STX-Confirm", Location: InputHeader, Required: true, Description: "Explicit confirmation"},
		},
		Example:       example,
		OutputExample: fmt.Sprintf(`{"api_version":"v1","operation_id":%q,"request_id":"req_example","data":{"id":10,"name":"node-2"},"result_meta":{"complete":true}}`, id),
	}
}
