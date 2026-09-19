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

package cmd

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	clioutput "github.com/LeonYoah/stx/internal/cli/output"
	"github.com/LeonYoah/stx/internal/operation"
	"github.com/spf13/cobra"
)

type upgradeRequestOptions struct {
	targetVersion       string
	packageChecksum     string
	packageArch         string
	targetInstallDir    string
	connectorNames      []string
	nodeIDs             []uint
	requestFile         string
	configMergePlanFile string
}

type upgradeTaskCLIData struct {
	ID             uint   `json:"id"`
	ExecutionID    string `json:"execution_id"`
	Status         string `json:"status"`
	CurrentStep    string `json:"current_step"`
	FailureReason  string `json:"failure_reason"`
	RollbackReason string `json:"rollback_reason"`
}

// addUpgradeCommands 将需要请求正文或轮询的升级命令挂到登记表生成的命令树。
// addUpgradeCommands attaches upgrade commands that require request bodies or polling to the generated tree.
func addUpgradeCommands(root *cobra.Command, storeProvider authStoreProvider) {
	upgradeCommand := childCommand(root, "upgrade")
	if upgradeCommand == nil {
		panic("generated upgrade command is missing")
	}
	upgradeCommand.AddCommand(newUpgradePrecheckCommand(storeProvider))

	planCommand := childCommand(upgradeCommand, "plan")
	if planCommand == nil {
		panic("generated upgrade plan command is missing")
	}
	planCommand.AddCommand(
		newUpgradePlanCreateCommand(storeProvider),
		newUpgradePlanExecuteCommand(storeProvider),
	)

	taskCommand := childCommand(upgradeCommand, "task")
	if taskCommand == nil {
		panic("generated upgrade task command is missing")
	}
	taskCommand.AddCommand(newUpgradeTaskWaitCommand(storeProvider))
}

func newUpgradePrecheckCommand(storeProvider authStoreProvider) *cobra.Command {
	var namespace string
	var request upgradeRequestOptions
	command := &cobra.Command{
		Use:     "precheck <cluster-id>",
		Short:   "Precheck a SeaTunnel upgrade",
		Long:    upgradeOperationDescription("stupgrade.precheck"),
		Example: "stx upgrade precheck 8 --target-version 2.3.13 --target-install-dir /tmp/seatunnel-2.3.13-new",
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			body, clusterID, err := buildUpgradeRequest(command, args[0], &request, false)
			if err != nil {
				return err
			}
			client, err := clientForNamespace(storeProvider, namespace)
			if err != nil {
				return err
			}
			if err := checkSpecialOperation(command.Context(), client, "stupgrade.precheck"); err != nil {
				return err
			}
			var data any
			requestID, err := client.Request(command.Context(), http.MethodPost, "/api/v1/st-upgrade/precheck", body, &data)
			if err != nil {
				return err
			}
			nextCommand := fmt.Sprintf("stx upgrade plan create %d --request-file resolved-upgrade-plan.json", clusterID)
			return renderWriteResult(command, "stupgrade.precheck", requestID, data, nextCommand)
		},
	}
	command.Flags().StringVar(&namespace, "namespace", "", "Local namespace to use")
	addUpgradeRequestFlags(command, &request, false)
	return command
}

func newUpgradePlanCreateCommand(storeProvider authStoreProvider) *cobra.Command {
	var namespace string
	var request upgradeRequestOptions
	command := &cobra.Command{
		Use:     "create <cluster-id>",
		Short:   "Create a SeaTunnel upgrade plan",
		Long:    upgradeOperationDescription("stupgrade.plan.create"),
		Example: "stx upgrade plan create 8 --target-version 2.3.13 --target-install-dir /tmp/seatunnel-2.3.13-new --config-merge-plan-file merge-plan.json",
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			body, _, err := buildUpgradeRequest(command, args[0], &request, true)
			if err != nil {
				return err
			}
			client, err := clientForNamespace(storeProvider, namespace)
			if err != nil {
				return err
			}
			if err := checkSpecialOperation(command.Context(), client, "stupgrade.plan.create"); err != nil {
				return err
			}
			var data any
			requestID, err := client.Request(command.Context(), http.MethodPost, "/api/v1/st-upgrade/plan", body, &data)
			if err != nil {
				return err
			}
			nextCommand := ""
			if planID := nestedNumericID(data, "plan"); planID > 0 {
				nextCommand = fmt.Sprintf("stx upgrade plan execute %d --confirm", planID)
			}
			return renderWriteResult(command, "stupgrade.plan.create", requestID, data, nextCommand)
		},
	}
	command.Flags().StringVar(&namespace, "namespace", "", "Local namespace to use")
	addUpgradeRequestFlags(command, &request, true)
	return command
}

func newUpgradePlanExecuteCommand(storeProvider authStoreProvider) *cobra.Command {
	var options secureWriteOptions
	command := &cobra.Command{
		Use:     "execute <plan-id>",
		Short:   "Execute a SeaTunnel upgrade plan",
		Long:    upgradeOperationDescription("stupgrade.plan.execute"),
		Example: "stx upgrade plan execute 1 --confirm",
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			planID, err := parsePositiveUint(args[0], "plan id")
			if err != nil {
				return err
			}
			client, headers, err := prepareSecureWrite(command, storeProvider, "stupgrade.plan.execute", &options, upgradeOperationImpact("stupgrade.plan.execute"))
			if err != nil {
				return err
			}
			var data any
			requestID, err := client.RequestWithHeaders(command.Context(), http.MethodPost, "/api/v1/st-upgrade/execute", map[string]any{"plan_id": planID}, headers, &data)
			if err != nil {
				return handleSecureWriteError(command, "stupgrade.plan.execute", err)
			}
			nextCommand := ""
			if taskID := numericID(data); taskID > 0 {
				nextCommand = fmt.Sprintf("stx upgrade task wait %d", taskID)
			}
			return renderWriteResult(command, "stupgrade.plan.execute", requestID, data, nextCommand)
		},
	}
	addSecureWriteFlags(command, &options)
	return command
}

func newUpgradeTaskWaitCommand(storeProvider authStoreProvider) *cobra.Command {
	var namespace string
	var timeout, interval time.Duration
	command := &cobra.Command{
		Use:     "wait <task-id>",
		Short:   "Wait for a SeaTunnel upgrade task to finish",
		Example: "stx upgrade task wait 1 --timeout 20m",
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			if timeout <= 0 || interval <= 0 {
				return clioutput.NewError(clioutput.CodeUsage, "--timeout and --interval must be greater than zero", clioutput.ExitUsage, false)
			}
			taskID, err := parsePositiveUint(args[0], "task id")
			if err != nil {
				return err
			}
			client, err := clientForNamespace(storeProvider, namespace)
			if err != nil {
				return err
			}
			if err := checkSpecialOperation(command.Context(), client, "stupgrade.task.get"); err != nil {
				return err
			}

			ctx, cancel := context.WithTimeout(command.Context(), timeout)
			defer cancel()
			events := clioutput.NewEventWriter(command.ErrOrStderr())
			lastStatus, lastStep := "", ""
			for {
				var data upgradeTaskCLIData
				requestID, requestErr := client.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v1/st-upgrade/tasks/%d", taskID), nil, &data)
				if requestErr != nil {
					return requestErr
				}
				if data.Status != lastStatus || data.CurrentStep != lastStep {
					executionID := data.ExecutionID
					if executionID == "" {
						executionID = strconv.FormatUint(uint64(taskID), 10)
					}
					message := data.Status
					if data.CurrentStep != "" {
						message += ":" + data.CurrentStep
					}
					if err := events.Emit(clioutput.Event{Event: "upgrade_progress", OperationID: "stupgrade.task.wait", RequestID: requestID, ExecutionID: executionID, Message: message}); err != nil {
						return clioutput.WrapError(err, clioutput.CodeServer, "write upgrade progress event", clioutput.ExitServer, false)
					}
					lastStatus, lastStep = data.Status, data.CurrentStep
				}
				if upgradeTaskStatusTerminal(data.Status) {
					if err := renderWriteResult(command, "stupgrade.task.wait", requestID, data, ""); err != nil {
						return err
					}
					if data.Status != "succeeded" {
						message := strings.TrimSpace(data.FailureReason)
						if message == "" {
							message = strings.TrimSpace(data.RollbackReason)
						}
						if message == "" {
							message = "upgrade task finished with status " + data.Status
						}
						return clioutput.NewError(clioutput.CodeExecution, message, clioutput.ExitExecution, false)
					}
					return nil
				}

				select {
				case <-ctx.Done():
					return clioutput.WrapError(ctx.Err(), clioutput.CodeTimeout, "waiting for upgrade task timed out", clioutput.ExitTimeout, true)
				case <-time.After(interval):
				}
			}
		},
	}
	command.Flags().StringVar(&namespace, "namespace", "", "Local namespace to use")
	command.Flags().DurationVar(&timeout, "timeout", 30*time.Minute, "Maximum time to wait")
	command.Flags().DurationVar(&interval, "interval", 2*time.Second, "Polling interval")
	return command
}

// addUpgradeRequestFlags 注册预检查与计划创建共用的升级请求参数。
// addUpgradeRequestFlags registers upgrade request flags shared by precheck and plan creation.
func addUpgradeRequestFlags(command *cobra.Command, request *upgradeRequestOptions, includeMergePlan bool) {
	command.Flags().StringVar(&request.targetVersion, "target-version", "", "Target SeaTunnel version")
	command.Flags().StringVar(&request.packageChecksum, "package-checksum", "", "Expected target package SHA-256 checksum")
	command.Flags().StringVar(&request.packageArch, "package-arch", "", "Target package architecture")
	command.Flags().StringVar(&request.targetInstallDir, "target-install-dir", "", "Target SeaTunnel installation directory")
	command.Flags().StringSliceVar(&request.connectorNames, "connector", nil, "Connector to keep; may be specified more than once")
	command.Flags().UintSliceVar(&request.nodeIDs, "node-id", nil, "Cluster node ID to upgrade; may be specified more than once")
	command.Flags().StringVar(&request.requestFile, "request-file", "", "JSON file containing the complete request body")
	if includeMergePlan {
		command.Flags().StringVar(&request.configMergePlanFile, "config-merge-plan-file", "", "JSON file containing the resolved config merge plan")
	}
}

// buildUpgradeRequest 合并完整请求文件与显式参数，并强制使用命令行集群编号。
// buildUpgradeRequest merges a complete request file with explicit flags and enforces the positional cluster ID.
func buildUpgradeRequest(command *cobra.Command, clusterIDText string, request *upgradeRequestOptions, includeMergePlan bool) (map[string]any, uint, error) {
	clusterID, err := parsePositiveUint(clusterIDText, "cluster id")
	if err != nil {
		return nil, 0, err
	}
	body, err := requestBodyFromFile(request.requestFile)
	if err != nil {
		return nil, 0, err
	}
	if body == nil {
		body = make(map[string]any)
	}
	body["cluster_id"] = clusterID
	setChangedString(command, body, "target-version", request.targetVersion)
	setChangedString(command, body, "package-checksum", request.packageChecksum)
	setChangedString(command, body, "package-arch", request.packageArch)
	setChangedString(command, body, "target-install-dir", request.targetInstallDir)
	if command.Flags().Changed("connector") {
		body["connector_names"] = request.connectorNames
	}
	if command.Flags().Changed("node-id") {
		body["node_ids"] = request.nodeIDs
	}
	if includeMergePlan && strings.TrimSpace(request.configMergePlanFile) != "" {
		if err := setJSONFileValue(body, "config_merge_plan", request.configMergePlanFile); err != nil {
			return nil, 0, err
		}
	}
	if strings.TrimSpace(stringValue(body["target_version"])) == "" {
		return nil, 0, clioutput.NewError(clioutput.CodeUsage, "--target-version or target_version in --request-file is required", clioutput.ExitUsage, false)
	}
	if includeMergePlan {
		if _, exists := body["config_merge_plan"]; !exists {
			return nil, 0, clioutput.NewError(clioutput.CodeUsage, "--config-merge-plan-file or config_merge_plan in --request-file is required", clioutput.ExitUsage, false)
		}
	}
	return body, clusterID, nil
}

func parsePositiveUint(value, field string) (uint, error) {
	parsed, err := strconv.ParseUint(strings.TrimSpace(value), 10, 32)
	if err != nil || parsed == 0 {
		return 0, clioutput.NewError(clioutput.CodeUsage, "invalid "+field, clioutput.ExitUsage, false)
	}
	return uint(parsed), nil
}

func numericID(data any) uint {
	item, ok := data.(map[string]any)
	if !ok {
		return 0
	}
	return anyNumericID(item["id"])
}

func nestedNumericID(data any, field string) uint {
	item, ok := data.(map[string]any)
	if !ok {
		return 0
	}
	nested, ok := item[field].(map[string]any)
	if !ok {
		return 0
	}
	return anyNumericID(nested["id"])
}

func anyNumericID(value any) uint {
	switch typed := value.(type) {
	case float64:
		if typed > 0 {
			return uint(typed)
		}
	case uint:
		return typed
	case int:
		if typed > 0 {
			return uint(typed)
		}
	}
	return 0
}

func upgradeTaskStatusTerminal(status string) bool {
	switch status {
	case "succeeded", "failed", "blocked", "rollback_succeeded", "rollback_failed", "cancelled":
		return true
	default:
		return false
	}
}

func upgradeOperationImpact(operationID string) string {
	for _, spec := range operation.Registry() {
		if spec.ID == operationID && spec.Impact != nil {
			return spec.Impact.Message
		}
	}
	return "this operation changes the SeaTunnel upgrade state"
}

func upgradeOperationDescription(operationID string) string {
	for _, spec := range operation.Registry() {
		if spec.ID != operationID {
			continue
		}
		sections := []string{"Run registered operation " + operationID + "."}
		if spec.Impact != nil {
			impact := fmt.Sprintf("Impact:\nRisk level: %s\n%s", spec.Impact.Level, spec.Impact.Message)
			if spec.Impact.Performance != "" {
				impact += "\nPerformance: " + spec.Impact.Performance
			}
			sections = append(sections, impact)
		}
		sections = append(sections, "Output example:\n"+spec.OutputExample)
		return strings.Join(sections, "\n\n")
	}
	return "Run registered operation " + operationID + "."
}
