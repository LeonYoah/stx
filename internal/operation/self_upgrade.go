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

// selfUpgradeOperationSpecs 登记 STX 控制台自升级（仅管理员）的全部 API 操作。
// selfUpgradeOperationSpecs registers all API operations for the admin-only STX console self-upgrade.
func selfUpgradeOperationSpecs() []OperationSpec {
	taskIDInput := []InputSpec{{Name: "id", Location: InputPath, Required: true, Description: "Self-upgrade task ID"}}
	return []OperationSpec{
		selfUpgradeOp("self_upgrade.status", []string{"self-upgrade", "status"}, "Get STX self-upgrade and deployment status",
			"GET", "/api/v1/self-upgrade/status", RiskR0, "", nil, "stx self-upgrade status", false),
		selfUpgradeOp("self_upgrade.check", []string{"self-upgrade", "check"}, "Check for new STX releases",
			"POST", "/api/v1/self-upgrade/check", RiskR0, "",
			[]InputSpec{{Name: "force_latest", Location: InputBody, Description: "Force a target release version such as v1.2.3 for verification"}},
			"stx self-upgrade check", false),
		selfUpgradeOp("self_upgrade.stage", []string{"self-upgrade", "stage"}, "Stage STX release assets from the network",
			"POST", "/api/v1/self-upgrade/stage/download", RiskR1, "Downloads the release bundle into the local self-upgrade staging directory.",
			[]InputSpec{{Name: "target_version", Location: InputBody, Description: "Target release version such as v1.2.3 or 1.2.3-beta.1; defaults to the latest checked version"}},
			"stx self-upgrade stage --target-version v1.2.3", true),
		selfUpgradeOp("self_upgrade.stage.import", nil, "Import an offline STX upgrade package",
			"POST", "/api/v1/self-upgrade/stage/import", RiskR1, "Uploads and unpacks an offline package into the local self-upgrade staging directory.",
			[]InputSpec{{Name: "file", Location: InputFile, Required: true, Description: "Offline upgrade archive (.tar.gz, .tgz, or .zip)"}},
			"", false),
		selfUpgradeOp("self_upgrade.stage.get", nil, "Get the STX self-upgrade staging snapshot",
			"GET", "/api/v1/self-upgrade/stage", RiskR0, "", nil, "", false),
		selfUpgradeOp("self_upgrade.stage.cancel", nil, "Cancel STX self-upgrade staging",
			"POST", "/api/v1/self-upgrade/stage/cancel", RiskR1, "Stops the running staging task and leaves partially downloaded assets unused.",
			nil, "", false),
		selfUpgradeOp("self_upgrade.execute", []string{"self-upgrade", "execute"}, "Execute an STX upgrade from staged assets",
			"POST", "/api/v1/self-upgrade/execute", RiskR3, "mode=apply replaces the STX control plane, Agent and Java Proxy with staged assets and restarts the STX processes involved (control plane, bundled frontend, Agents, running stx-java-proxy); SeaTunnel processes and jobs are not touched. mode=dry_run applies nothing.",
			[]InputSpec{
				{Name: "mode", Location: InputBody, Description: "Execution mode: apply (real upgrade) or dry_run (rehearsal, nothing applied); defaults to dry_run when omitted"},
				{Name: "apply", Location: InputBody, Description: "Deprecated: legacy flag, equivalent to mode=apply when mode is omitted"},
			},
			"stx self-upgrade execute", true),
		selfUpgradeOp("self_upgrade.task.get", nil, "Get an STX self-upgrade task",
			"GET", "/api/v1/self-upgrade/tasks/:id", RiskR0, "", taskIDInput, "", false),
		selfUpgradeOp("self_upgrade.task.cancel", nil, "Cancel an STX self-upgrade task",
			"POST", "/api/v1/self-upgrade/tasks/:id/cancel", RiskR1, "Cancels a running self-upgrade task when it is still cancellable.",
			taskIDInput, "", false),
		selfUpgradeOp("self_upgrade.history", []string{"self-upgrade", "history"}, "List STX self-upgrade execution history",
			"GET", "/api/v1/self-upgrade/history", RiskR0, "", nil, "stx self-upgrade history", false),
	}
}

func selfUpgradeOp(id string, commandPath []string, summary, method, route string, risk RiskLevel, impact string, inputs []InputSpec, example string, async bool) OperationSpec {
	var impactSpec *ImpactSpec
	if impact != "" {
		impactSpec = &ImpactSpec{Level: risk, Message: impact}
	}
	return OperationSpec{
		ID: id, CommandPath: commandPath, Summary: summary, GeneratedCLI: false, Method: method, Route: route,
		Mode: ModeNormal, AuthRequired: true, AdminOnly: true, Risk: risk, Revision: 1, Async: async,
		Impact: impactSpec, Input: inputs, Example: example,
		OutputExample: fmt.Sprintf(`{"api_version":"v1","operation_id":%q,"request_id":"req_example","data":{},"result_meta":{"complete":true}}`, id),
	}
}
