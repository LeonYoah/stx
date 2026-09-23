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

// addMonitoringWriteCommands 将监控中心写命令挂到现有查询命令树。
// addMonitoringWriteCommands attaches monitoring writes to the existing query command tree.
func addMonitoringWriteCommands(root *cobra.Command, storeProvider authStoreProvider) {
	monitoringCommand := childCommand(root, "monitoring")
	if monitoringCommand == nil {
		panic("generated monitoring command is missing")
	}
	alertPolicyCommand := childCommand(monitoringCommand, "alert-policy")
	alertInstanceCommand := childCommand(monitoringCommand, "alert-instance")
	alertCommand := childCommand(monitoringCommand, "alert")
	clusterRuleCommand := childCommand(childCommand(monitoringCommand, "cluster"), "rule")
	notificationChannelCommand := childCommand(monitoringCommand, "notification-channel")
	notificationRouteCommand := childCommand(monitoringCommand, "notification-route")
	if alertPolicyCommand == nil || alertInstanceCommand == nil || alertCommand == nil || clusterRuleCommand == nil || notificationChannelCommand == nil || notificationRouteCommand == nil {
		panic("generated monitoring subcommand is missing")
	}

	alertPolicyCommand.AddCommand(
		newMonitoringRequestFileCommand(storeProvider, monitoringRequestFileCommand{
			Use: "create", Short: "Create an alert policy", Example: "stx monitoring alert-policy create --request-file ./policy.json --confirm",
			OperationID: "monitoring.alert-policy.create", Method: http.MethodPost, Args: cobra.NoArgs,
			Path: func([]string) string { return "/api/v1/monitoring/alert-policies" }, NextCommand: func([]string) string { return "stx monitoring alert-policy list" },
		}),
		newMonitoringRequestFileCommand(storeProvider, monitoringRequestFileCommand{
			Use: "update <id>", Short: "Update an alert policy", Example: "stx monitoring alert-policy update 1 --request-file ./policy.json --confirm",
			OperationID: "monitoring.alert-policy.update", Method: http.MethodPut, Args: cobra.ExactArgs(1),
			Path: func(args []string) string { return "/api/v1/monitoring/alert-policies/" + url.PathEscape(args[0]) }, NextCommand: func([]string) string { return "stx monitoring alert-policy list" },
		}),
	)
	alertInstanceCommand.AddCommand(
		newMonitoringAlertActionCommand(storeProvider, "alert-instance", "ack"),
		newMonitoringAlertActionCommand(storeProvider, "alert-instance", "silence"),
		newMonitoringAlertActionCommand(storeProvider, "alert-instance", "close"),
	)
	alertCommand.AddCommand(
		newMonitoringAlertActionCommand(storeProvider, "alert", "ack"),
		newMonitoringAlertActionCommand(storeProvider, "alert", "silence"),
	)
	clusterRuleCommand.AddCommand(newMonitoringRequestFileCommand(storeProvider, monitoringRequestFileCommand{
		Use: "update <cluster-id> <rule-id>", Short: "Update a cluster alert rule", Example: "stx monitoring cluster rule update 6 1 --request-file ./rule.json --confirm",
		OperationID: "monitoring.cluster.rule.update", Method: http.MethodPut, Args: cobra.ExactArgs(2),
		Path: func(args []string) string {
			return "/api/v1/monitoring/clusters/" + url.PathEscape(args[0]) + "/rules/" + url.PathEscape(args[1])
		},
		NextCommand: func(args []string) string { return "stx monitoring cluster rule list " + args[0] },
	}))
	notificationChannelCommand.AddCommand(
		newMonitoringRequestFileCommand(storeProvider, monitoringRequestFileCommand{
			Use: "create", Short: "Create a notification channel", Example: "stx monitoring notification-channel create --request-file ./channel.json --confirm",
			OperationID: "monitoring.notification-channel.create", Method: http.MethodPost, Args: cobra.NoArgs,
			Path: func([]string) string { return "/api/v1/monitoring/notification-channels" }, NextCommand: func([]string) string { return "stx monitoring notification-channel list" },
		}),
		newMonitoringRequestFileCommand(storeProvider, monitoringRequestFileCommand{
			Use: "update <id>", Short: "Update a notification channel", Example: "stx monitoring notification-channel update 1 --request-file ./channel.json --confirm",
			OperationID: "monitoring.notification-channel.update", Method: http.MethodPut, Args: cobra.ExactArgs(1),
			Path: func(args []string) string {
				return "/api/v1/monitoring/notification-channels/" + url.PathEscape(args[0])
			}, NextCommand: func([]string) string { return "stx monitoring notification-channel list" },
		}),
		newMonitoringNotificationChannelTestCommand(storeProvider),
		newMonitoringRequestFileCommand(storeProvider, monitoringRequestFileCommand{
			Use: "test-draft", Short: "Test a notification channel draft", Example: "stx monitoring notification-channel test-draft --request-file ./draft-test.json --confirm",
			OperationID: "monitoring.notification-channel.test-draft", Method: http.MethodPost, Args: cobra.NoArgs,
			Path: func([]string) string { return "/api/v1/monitoring/notification-channels/test" }, NextCommand: func([]string) string { return "stx monitoring notification-delivery list" },
		}),
		newMonitoringRequestFileCommand(storeProvider, monitoringRequestFileCommand{
			Use: "test-connection", Short: "Test notification channel connectivity", Example: "stx monitoring notification-channel test-connection --request-file ./channel.json --confirm",
			OperationID: "monitoring.notification-channel.test-connection", Method: http.MethodPost, Args: cobra.NoArgs,
			Path: func([]string) string { return "/api/v1/monitoring/notification-channels/test-connection" }, NextCommand: func([]string) string { return "stx monitoring notification-channel list" },
		}),
	)
	notificationRouteCommand.AddCommand(
		newMonitoringRequestFileCommand(storeProvider, monitoringRequestFileCommand{
			Use: "create", Short: "Create a notification route", Example: "stx monitoring notification-route create --request-file ./route.json --confirm",
			OperationID: "monitoring.notification-route.create", Method: http.MethodPost, Args: cobra.NoArgs,
			Path: func([]string) string { return "/api/v1/monitoring/notification-routes" }, NextCommand: func([]string) string { return "stx monitoring notification-route list" },
		}),
		newMonitoringRequestFileCommand(storeProvider, monitoringRequestFileCommand{
			Use: "update <id>", Short: "Update a notification route", Example: "stx monitoring notification-route update 1 --request-file ./route.json --confirm",
			OperationID: "monitoring.notification-route.update", Method: http.MethodPut, Args: cobra.ExactArgs(1),
			Path: func(args []string) string { return "/api/v1/monitoring/notification-routes/" + url.PathEscape(args[0]) }, NextCommand: func([]string) string { return "stx monitoring notification-route list" },
		}),
	)
}

type monitoringRequestFileCommand struct {
	Use, Short, Example, OperationID, Method string
	Args                                     cobra.PositionalArgs
	Path                                     func([]string) string
	NextCommand                              func([]string) string
}

// newMonitoringRequestFileCommand 创建使用完整 JSON 文件的监控写命令。
// newMonitoringRequestFileCommand creates a monitoring write command backed by a complete JSON file.
func newMonitoringRequestFileCommand(storeProvider authStoreProvider, definition monitoringRequestFileCommand) *cobra.Command {
	var options secureWriteOptions
	var requestFile string
	command := &cobra.Command{
		Use: definition.Use, Short: definition.Short, Example: definition.Example, Args: usageArgs(definition.Args),
		RunE: func(command *cobra.Command, args []string) error {
			body, err := requestBodyFromFile(requestFile)
			if err != nil {
				return err
			}
			if body == nil {
				return clioutput.NewError(clioutput.CodeUsage, "--request-file is required", clioutput.ExitUsage, false)
			}
			return executeMonitoringWrite(command, storeProvider, definition.OperationID, &options, definition.Method, definition.Path(args), body, definition.NextCommand(args))
		},
	}
	addSecureWriteFlags(command, &options)
	command.Flags().StringVar(&requestFile, "request-file", "", "JSON file containing the complete request body")
	return command
}

func newMonitoringAlertActionCommand(storeProvider authStoreProvider, resource, action string) *cobra.Command {
	var options secureWriteOptions
	var note string
	var durationMinutes int
	operationID := "monitoring." + resource + "." + action
	argument := "<id>"
	if resource == "alert" {
		argument = "<event-id>"
	}
	command := &cobra.Command{
		Use: action + " " + argument, Short: strings.ToUpper(action[:1]) + action[1:] + " a monitoring " + resource,
		Example: "stx monitoring " + resource + " " + action + " 1 --confirm", Args: usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			body := map[string]any{}
			if strings.TrimSpace(note) != "" {
				body["note"] = strings.TrimSpace(note)
			}
			if action == "silence" {
				if durationMinutes < 1 || durationMinutes > 10080 {
					return clioutput.NewError(clioutput.CodeUsage, "--duration-minutes must be between 1 and 10080", clioutput.ExitUsage, false)
				}
				body["duration_minutes"] = durationMinutes
			}
			pathResource := "alert-instances"
			if resource == "alert" {
				pathResource = "alerts"
			}
			path := "/api/v1/monitoring/" + pathResource + "/" + url.PathEscape(args[0]) + "/" + action
			return executeMonitoringWrite(command, storeProvider, operationID, &options, http.MethodPost, path, body, "stx monitoring "+resource+" list")
		},
	}
	addSecureWriteFlags(command, &options)
	command.Flags().StringVar(&note, "note", "", "Operator note")
	if action == "silence" {
		command.Flags().IntVar(&durationMinutes, "duration-minutes", 0, "Silence duration from 1 to 10080 minutes")
	}
	return command
}

func newMonitoringNotificationChannelTestCommand(storeProvider authStoreProvider) *cobra.Command {
	var options secureWriteOptions
	var receiverUserID uint64
	command := &cobra.Command{
		Use: "test <id>", Short: "Test a saved notification channel", Example: "stx monitoring notification-channel test 1 --confirm",
		Args: usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			body := map[string]any{}
			if command.Flags().Changed("receiver-user-id") {
				body["receiver_user_id"] = receiverUserID
			}
			path := "/api/v1/monitoring/notification-channels/" + url.PathEscape(args[0]) + "/test"
			return executeMonitoringWrite(command, storeProvider, "monitoring.notification-channel.test", &options, http.MethodPost, path, body, "stx monitoring notification-delivery list")
		},
	}
	addSecureWriteFlags(command, &options)
	command.Flags().Uint64Var(&receiverUserID, "receiver-user-id", 0, "Receiver user ID for email test messages")
	return command
}

// executeMonitoringWrite 使用公共确认、能力检查和结果格式执行监控写请求。
// executeMonitoringWrite executes a monitoring write with common confirmation, capability checks, and output formatting.
func executeMonitoringWrite(command *cobra.Command, storeProvider authStoreProvider, operationID string, options *secureWriteOptions, method, path string, body map[string]any, nextCommand string) error {
	client, headers, err := prepareSecureWrite(command, storeProvider, operationID, options, monitoringOperationImpact(operationID))
	if err != nil {
		return err
	}
	var data any
	requestID, err := client.RequestWithHeaders(command.Context(), method, path, body, headers, &data)
	if err != nil {
		return handleSecureWriteError(command, operationID, err)
	}
	return renderWriteResult(command, operationID, requestID, data, nextCommand)
}

func monitoringOperationImpact(operationID string) string {
	for _, spec := range operation.Registry() {
		if spec.ID == operationID && spec.Impact != nil {
			return spec.Impact.Message
		}
	}
	return "this operation changes monitoring state"
}
