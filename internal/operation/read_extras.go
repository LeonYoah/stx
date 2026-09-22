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
		{
			ID: "cluster.runtime-storage.validate", CommandPath: []string{"cluster", "runtime-storage", "validate"}, Summary: "Validate cluster runtime storage",
			GeneratedCLI: false, Method: "POST", Route: "/api/v1/clusters/:id/runtime-storage/:kind/validate", Mode: ModeNormal,
			AuthRequired: true, Risk: RiskR0, Revision: 1, UsesAgent: true, SupportsPick: true,
			Input: []InputSpec{
				{Name: "id", Location: InputPath, Required: true, Description: "Cluster ID"},
				{Name: "kind", Location: InputPath, Required: true, Description: "Runtime storage kind: checkpoint or imap"},
			},
			Impact:        &ImpactSpec{Level: RiskR0, Message: "Checks the configured runtime storage from managed cluster nodes.", Performance: "Runs a short storage connectivity and read/write probe through the Agent."},
			Example:       "stx cluster runtime-storage validate 6 checkpoint",
			OutputExample: `{"api_version":"v1","operation_id":"cluster.runtime-storage.validate","request_id":"req_example","data":{"success":true},"result_meta":{"complete":true}}`,
		},
		{
			ID: "cluster.runtime-storage.list", CommandPath: []string{"cluster", "runtime-storage", "list"}, Summary: "List cluster runtime storage files",
			GeneratedCLI: false, Method: "POST", Route: "/api/v1/clusters/:id/runtime-storage/:kind/list", Mode: ModeNormal,
			AuthRequired: true, Risk: RiskR0, Revision: 1, UsesAgent: true, SupportsPick: true,
			Input: []InputSpec{
				{Name: "id", Location: InputPath, Required: true, Description: "Cluster ID"},
				{Name: "kind", Location: InputPath, Required: true, Description: "Runtime storage kind: checkpoint or imap"},
				{Name: "request", Location: InputBody, Description: "Path, recursion, and result limit"},
			},
			Impact:        &ImpactSpec{Level: RiskR0, Message: "Lists files from the configured runtime storage.", Performance: "Recursive listing or a large limit can increase Agent, storage, and network load."},
			Example:       "stx cluster runtime-storage list 6 checkpoint --limit 100",
			OutputExample: `{"api_version":"v1","operation_id":"cluster.runtime-storage.list","request_id":"req_example","data":{"items":[]},"result_meta":{"complete":true}}`,
		},
		{
			ID: "cluster.runtime-storage.preview", CommandPath: []string{"cluster", "runtime-storage", "preview"}, Summary: "Preview a runtime storage file",
			GeneratedCLI: false, Method: "POST", Route: "/api/v1/clusters/:id/runtime-storage/:kind/preview", Mode: ModeNormal,
			AuthRequired: true, Risk: RiskR0, Revision: 1, UsesAgent: true, SupportsPick: true,
			Input: []InputSpec{
				{Name: "id", Location: InputPath, Required: true, Description: "Cluster ID"},
				{Name: "kind", Location: InputPath, Required: true, Description: "Runtime storage kind: checkpoint or imap"},
				{Name: "request", Location: InputBody, Required: true, Description: "Storage path and preview byte limit"},
			},
			Impact:        &ImpactSpec{Level: RiskR0, Message: "Reads a bounded preview from one runtime storage file.", Performance: "The preview byte limit controls Agent, storage, and network usage."},
			Example:       "stx cluster runtime-storage preview 6 checkpoint --path /tmp/seatunnel/checkpoint/checkpoint.dat --max-bytes 65536",
			OutputExample: `{"api_version":"v1","operation_id":"cluster.runtime-storage.preview","request_id":"req_example","data":{"path":"checkpoint.dat"},"result_meta":{"complete":true}}`,
		},
		{
			ID: "cluster.runtime-storage.checkpoint.inspect", CommandPath: []string{"cluster", "runtime-storage", "checkpoint", "inspect"}, Summary: "Inspect a checkpoint file",
			GeneratedCLI: false, Method: "POST", Route: "/api/v1/clusters/:id/runtime-storage/checkpoint/inspect", Mode: ModeNormal,
			AuthRequired: true, Risk: RiskR0, Revision: 1, UsesAgent: true, SupportsPick: true,
			Input:         []InputSpec{{Name: "id", Location: InputPath, Required: true, Description: "Cluster ID"}, {Name: "request", Location: InputBody, Required: true, Description: "Checkpoint path and optional job configuration"}},
			Impact:        &ImpactSpec{Level: RiskR0, Message: "Reads and deserializes one checkpoint file.", Performance: "Checkpoint parsing can use noticeable CPU, memory, storage, and network resources."},
			Example:       "stx cluster runtime-storage checkpoint inspect 6 --path /tmp/seatunnel/checkpoint/checkpoint.dat",
			OutputExample: `{"api_version":"v1","operation_id":"cluster.runtime-storage.checkpoint.inspect","request_id":"req_example","data":{"path":"checkpoint.dat"},"result_meta":{"complete":true}}`,
		},
		{
			ID: "cluster.runtime-storage.imap.inspect", CommandPath: []string{"cluster", "runtime-storage", "imap", "inspect"}, Summary: "Inspect an IMAP WAL file",
			GeneratedCLI: false, Method: "POST", Route: "/api/v1/clusters/:id/runtime-storage/imap/inspect", Mode: ModeNormal,
			AuthRequired: true, Risk: RiskR0, Revision: 1, UsesAgent: true, SupportsPick: true,
			Input:         []InputSpec{{Name: "id", Location: InputPath, Required: true, Description: "Cluster ID"}, {Name: "request", Location: InputBody, Required: true, Description: "IMAP WAL path"}},
			Impact:        &ImpactSpec{Level: RiskR0, Message: "Reads and parses one IMAP WAL file.", Performance: "WAL parsing can use noticeable CPU, memory, storage, and network resources."},
			Example:       "stx cluster runtime-storage imap inspect 6 --path /tmp/seatunnel/imap/imap.wal",
			OutputExample: `{"api_version":"v1","operation_id":"cluster.runtime-storage.imap.inspect","request_id":"req_example","data":{"path":"imap.wal"},"result_meta":{"complete":true}}`,
		},
		{
			ID: "installer.runtime-storage.validate", CommandPath: []string{"installer", "runtime-storage", "validate"}, Summary: "Validate runtime storage before installation",
			GeneratedCLI: false, Method: "POST", Route: "/api/v1/installer/runtime-storage/validate", Mode: ModeNormal,
			AuthRequired: true, Risk: RiskR0, Revision: 1, UsesAgent: true, SupportsPick: true,
			Input:         []InputSpec{{Name: "request", Location: InputBody, Required: true, Description: "Host IDs, storage kind, and storage configuration"}},
			Impact:        &ImpactSpec{Level: RiskR0, Message: "Checks runtime storage reachability from selected hosts.", Performance: "Runs short path or endpoint checks through each selected host Agent."},
			Example:       "stx installer runtime-storage validate --request-file runtime-storage.json",
			OutputExample: `{"api_version":"v1","operation_id":"installer.runtime-storage.validate","request_id":"req_example","data":{"success":true},"result_meta":{"complete":true}}`,
		},
	}
}
