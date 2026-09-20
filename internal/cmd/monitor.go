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
	"net/http"
	"net/url"
	"strings"

	clioutput "github.com/LeonYoah/stx/internal/cli/output"
	"github.com/LeonYoah/stx/internal/operation"
	"github.com/spf13/cobra"
)

// addMonitorWriteCommands 将监控配置写命令挂到生成的 monitor 命令树。
// addMonitorWriteCommands attaches monitor configuration writes to the generated monitor command tree.
func addMonitorWriteCommands(root *cobra.Command, storeProvider authStoreProvider) {
	monitorCommand := childCommand(root, "monitor")
	if monitorCommand == nil {
		panic("generated monitor command is missing")
	}
	configCommand := childCommand(monitorCommand, "config")
	if configCommand == nil {
		panic("generated monitor config command is missing")
	}
	configCommand.AddCommand(newMonitorConfigUpdateCommand(storeProvider))
}

// newMonitorConfigUpdateCommand 创建支持单字段参数和完整 JSON 的监控配置更新命令。
// newMonitorConfigUpdateCommand creates a monitor configuration update command with field flags and complete JSON input.
func newMonitorConfigUpdateCommand(storeProvider authStoreProvider) *cobra.Command {
	var options secureWriteOptions
	var requestFile string
	var autoMonitor, autoRestart bool
	var monitorInterval, restartDelay, maxRestarts, timeWindow, cooldownPeriod int
	command := &cobra.Command{
		Use:     "update <cluster-id>",
		Short:   "Update cluster monitor configuration",
		Long:    "Update process monitoring and automatic restart settings for one cluster.",
		Example: "stx monitor config update 6 --monitor-interval 5 --confirm",
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			body, err := requestBodyFromFile(requestFile)
			if err != nil {
				return err
			}
			if body == nil {
				body = make(map[string]any)
				setChangedBool(command, body, "auto-monitor", autoMonitor)
				setChangedBool(command, body, "auto-restart", autoRestart)
				setChangedInt(command, body, "monitor-interval", monitorInterval)
				setChangedInt(command, body, "restart-delay", restartDelay)
				setChangedInt(command, body, "max-restarts", maxRestarts)
				setChangedInt(command, body, "time-window", timeWindow)
				setChangedInt(command, body, "cooldown-period", cooldownPeriod)
				if len(body) == 0 {
					return clioutput.NewError(clioutput.CodeUsage, "at least one update flag or --request-file is required", clioutput.ExitUsage, false)
				}
			}
			path := "/api/v1/clusters/" + url.PathEscape(args[0]) + "/monitor-config"
			return executeMonitorWrite(command, storeProvider, &options, path, body, args[0])
		},
	}
	addSecureWriteFlags(command, &options)
	command.Flags().BoolVar(&autoMonitor, "auto-monitor", false, "Enable or disable automatic process monitoring")
	command.Flags().BoolVar(&autoRestart, "auto-restart", false, "Enable or disable automatic process restart")
	command.Flags().IntVar(&monitorInterval, "monitor-interval", 0, "Monitor interval in seconds, from 1 to 60")
	command.Flags().IntVar(&restartDelay, "restart-delay", 0, "Restart delay in seconds, from 1 to 300")
	command.Flags().IntVar(&maxRestarts, "max-restarts", 0, "Maximum restarts in one time window, from 1 to 10")
	command.Flags().IntVar(&timeWindow, "time-window", 0, "Restart counting window in seconds, from 60 to 3600")
	command.Flags().IntVar(&cooldownPeriod, "cooldown-period", 0, "Cooldown period in seconds, from 60 to 86400")
	command.Flags().StringVar(&requestFile, "request-file", "", "JSON file containing the complete request body")
	return command
}

// executeMonitorWrite 使用公共确认、能力检查和结果协议执行监控配置更新。
// executeMonitorWrite executes a monitor configuration update through common confirmation, capability, and result handling.
func executeMonitorWrite(command *cobra.Command, storeProvider authStoreProvider, options *secureWriteOptions, path string, body map[string]any, clusterID string) error {
	const operationID = "monitor.config.update"
	client, headers, err := prepareSecureWrite(command, storeProvider, operationID, options, monitorOperationImpact(operationID))
	if err != nil {
		return err
	}
	var data any
	requestID, err := client.RequestWithHeaders(command.Context(), http.MethodPut, path, body, headers, &data)
	if err != nil {
		return handleSecureWriteError(command, operationID, err)
	}
	return renderWriteResult(command, operationID, requestID, data, "stx monitor config get "+clusterID)
}

// monitorOperationImpact 返回登记表中的监控写操作影响说明。
// monitorOperationImpact returns the registered impact message for a monitor write operation.
func monitorOperationImpact(operationID string) string {
	for _, spec := range operation.Registry() {
		if spec.ID == operationID && spec.Impact != nil {
			return spec.Impact.Message
		}
	}
	return "this operation changes process monitoring behavior"
}

// setChangedBool 只在用户显式传入布尔参数时写入请求正文。
// setChangedBool writes a boolean request field only when the user explicitly supplied the flag.
func setChangedBool(command *cobra.Command, body map[string]any, flagName string, value bool) {
	if command.Flags().Changed(flagName) {
		body[strings.ReplaceAll(flagName, "-", "_")] = value
	}
}
