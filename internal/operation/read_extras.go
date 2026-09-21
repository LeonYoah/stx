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

// additionalReadOperationSpecs 登记与工作台无关的补充只读查询。
// additionalReadOperationSpecs registers supplemental read-only queries outside the workbench.
func additionalReadOperationSpecs() []OperationSpec {
	return []OperationSpec{
		{
			ID: "cluster.health.list", CommandPath: []string{"cluster", "health", "list"}, Summary: "List cluster health summaries",
			GeneratedCLI: true, Method: "GET", Route: "/api/v1/clusters/health", Mode: ModeNormal,
			AuthRequired: true, Risk: RiskR0, Revision: 1, SupportsPick: true,
			Example:       "stx cluster health list",
			OutputExample: `{"api_version":"v1","operation_id":"cluster.health.list","request_id":"req_example","data":[],"result_meta":{"complete":true}}`,
		},
		{
			ID: "cluster.runtime-storage.get", CommandPath: []string{"cluster", "runtime-storage", "get"}, Summary: "Get cluster runtime storage details",
			GeneratedCLI: true, Method: "GET", Route: "/api/v1/clusters/:id/runtime-storage", Mode: ModeNormal,
			AuthRequired: true, Risk: RiskR0, Revision: 1, UsesAgent: true, SupportsPick: true,
			Input: []InputSpec{{Name: "id", Location: InputPath, Required: true, Description: "Cluster ID"}},
			Impact: &ImpactSpec{
				Level:       RiskR0,
				Message:     "Reads checkpoint and IMAP runtime storage configuration and status for the cluster.",
				Performance: "May query managed nodes when local runtime storage statistics are unavailable.",
			},
			Example:       "stx cluster runtime-storage get 8",
			OutputExample: `{"api_version":"v1","operation_id":"cluster.runtime-storage.get","request_id":"req_example","data":{"cluster_id":8},"result_meta":{"complete":true}}`,
		},
		{
			ID: "host.task.list", CommandPath: []string{"host", "task", "list"}, Summary: "List tasks associated with a host",
			GeneratedCLI: true, Method: "GET", Route: "/api/v1/hosts/:id/tasks", Mode: ModeNormal,
			AuthRequired: true, Risk: RiskR0, Revision: 1, SupportsPick: true,
			Input:         []InputSpec{{Name: "id", Location: InputPath, Required: true, Description: "Host ID"}},
			Example:       "stx host task list 10",
			OutputExample: `{"api_version":"v1","operation_id":"host.task.list","request_id":"req_example","data":[],"result_meta":{"complete":true}}`,
		},
	}
}
