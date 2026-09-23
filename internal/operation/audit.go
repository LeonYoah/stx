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

// auditOperationSpecs 登记审计日志与 Agent 实际命令记录的只读接口。
// auditOperationSpecs registers read-only audit logs and Agent command records.
func auditOperationSpecs() []OperationSpec {
	commandListInputs := []InputSpec{
		{Name: "command_id", Location: InputQuery, Description: "Agent command ID"},
		{Name: "agent_id", Location: InputQuery, Description: "Agent ID"},
		{Name: "host_id", Location: InputQuery, Description: "Host ID"},
		{Name: "command_type", Location: InputQuery, Description: "Agent command type"},
		{Name: "request_id", Location: InputQuery, Description: "STX request ID"},
		{Name: "status", Location: InputQuery, Description: "Command status"},
		{Name: "start_time", Location: InputQuery, Description: "Start time in RFC3339 format"},
		{Name: "end_time", Location: InputQuery, Description: "End time in RFC3339 format"},
		{Name: "current", Location: InputQuery, Description: "Page number starting from 1"},
		{Name: "size", Location: InputQuery, Description: "Number of items per page"},
	}
	auditListInputs := []InputSpec{
		{Name: "user_id", Location: InputQuery, Description: "User ID; administrators may filter by another user"},
		{Name: "username", Location: InputQuery, Description: "Username; administrators may filter by username"},
		{Name: "action", Location: InputQuery, Description: "Audit action"},
		{Name: "action_group", Location: InputQuery, Description: "Audit action group"},
		{Name: "resource_type", Location: InputQuery, Description: "Resource type"},
		{Name: "resource_id", Location: InputQuery, Description: "Resource ID"},
		{Name: "request_id", Location: InputQuery, Description: "STX request ID"},
		{Name: "execution_id", Location: InputQuery, Description: "Shared execution ID"},
		{Name: "command_id", Location: InputQuery, Description: "Agent command ID"},
		{Name: "client_type", Location: InputQuery, Description: "Client type, such as cli or web"},
		{Name: "result_status", Location: InputQuery, Description: "Operation result status"},
		{Name: "trigger", Location: InputQuery, Description: "Trigger type: auto or manual"},
		{Name: "start_time", Location: InputQuery, Description: "Start time in RFC3339 format"},
		{Name: "end_time", Location: InputQuery, Description: "End time in RFC3339 format"},
		{Name: "current", Location: InputQuery, Description: "Page number starting from 1"},
		{Name: "size", Location: InputQuery, Description: "Number of items per page"},
	}
	return []OperationSpec{
		{
			ID: "audit.command.list", CommandPath: []string{"audit", "command", "list"}, Summary: "List Agent command records",
			GeneratedCLI: true, Method: "GET", Route: "/api/v1/commands", Mode: ModeNormal,
			AuthRequired: true, Risk: RiskR0, Revision: 1, SupportsPick: true,
			Input:         commandListInputs,
			Example:       "stx audit command list --agent_id agent-123 --status failed --size 50",
			OutputExample: `{"api_version":"v1","operation_id":"audit.command.list","request_id":"req_example","data":{"total":1,"commands":[]},"result_meta":{"complete":true}}`,
		},
		{
			ID: "audit.command.get", CommandPath: []string{"audit", "command", "get"}, Summary: "Get an Agent command record",
			GeneratedCLI: true, Method: "GET", Route: "/api/v1/commands/:id", Mode: ModeNormal,
			AuthRequired: true, Risk: RiskR0, Revision: 1, SupportsPick: true,
			Input:         []InputSpec{{Name: "id", Location: InputPath, Required: true, Description: "Command log ID"}},
			Example:       "stx audit command get 42",
			OutputExample: `{"api_version":"v1","operation_id":"audit.command.get","request_id":"req_example","data":{"id":42,"command_id":"cmd-123","status":"success"},"result_meta":{"complete":true}}`,
		},
		{
			ID: "audit.log.list", CommandPath: []string{"audit", "list"}, Summary: "List audit logs",
			GeneratedCLI: true, Method: "GET", Route: "/api/v1/audit-logs", Mode: ModeNormal,
			AuthRequired: true, Risk: RiskR0, Revision: 1, SupportsPick: true,
			Input:         auditListInputs,
			Example:       "stx audit list --client_type cli --result_status failed --size 50",
			OutputExample: `{"api_version":"v1","operation_id":"audit.log.list","request_id":"req_example","data":{"total":1,"logs":[]},"result_meta":{"complete":true}}`,
		},
		{
			ID: "audit.log.get", CommandPath: []string{"audit", "get"}, Summary: "Get an audit log",
			GeneratedCLI: true, Method: "GET", Route: "/api/v1/audit-logs/:id", Mode: ModeNormal,
			AuthRequired: true, Risk: RiskR0, Revision: 1, SupportsPick: true,
			Input:         []InputSpec{{Name: "id", Location: InputPath, Required: true, Description: "Audit log ID"}},
			Example:       "stx audit get 42",
			OutputExample: `{"api_version":"v1","operation_id":"audit.log.get","request_id":"req_example","data":{"id":42,"action":"start"},"result_meta":{"complete":true}}`,
		},
	}
}
