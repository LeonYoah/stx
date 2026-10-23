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
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	clioutput "github.com/LeonYoah/stx/internal/cli/output"
	"github.com/spf13/cobra"
)

var unreplacedCLIPlaceholder = regexp.MustCompile(`^\$\{.+\}$`)

func newSyncTaskDolphinPushCommand(storeProvider authStoreProvider) *cobra.Command {
	var options secureWriteOptions
	var newWorkflow, dryRun, useBuiltinSchedule bool
	var connectionID, projectCode, workflowCode uint64
	var expectedVersion int
	var workflowName, workerGroup, environment string
	var upstreams, downstreams, varFlags []string

	command := &cobra.Command{
		Use:     "push <id>",
		Short:   "Push a sync task into DolphinScheduler",
		Example: "stx sync task dolphin push 1 --confirm",
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			taskID, err := parseTaskID(args[0])
			if err != nil {
				return err
			}
			variables, err := parseVarFlags(varFlags)
			if err != nil {
				return err
			}
			operationID := "sync.task.dolphin.push"
			client, headers, err := prepareSecureWrite(command, storeProvider, operationID, &options, "向 DolphinScheduler 创建或更新 SHELL 节点；已上线工作流会先下线、写入后再上线，期间其定时不触发，原本暂停的定时保持暂停。")
			if err != nil {
				return err
			}
			mode := "update"
			if newWorkflow {
				mode = "new"
			} else if workflowCode > 0 {
				mode = "reuse"
			}
			body := map[string]any{
				"mode":                 mode,
				"connection_id":        connectionID,
				"project_code":         projectCode,
				"workflow_name":        workflowName,
				"workflow_code":        workflowCode,
				"upstreams":            parseInt64Flags(upstreams),
				"downstreams":          parseInt64Flags(downstreams),
				"use_builtin_schedule": useBuiltinSchedule,
				"worker_group":         workerGroup,
				"environment":          environment,
				"dry_run":              dryRun,
			}
			if expectedVersion > 0 {
				body["expected_version"] = expectedVersion
			}
			if len(variables) > 0 {
				body["variables"] = variables
			}
			var data any
			requestID, err := client.RequestWithHeaders(command.Context(), http.MethodPost, fmt.Sprintf("/api/v1/sync/tasks/%d/dolphin/push", taskID), body, headers, &data)
			if err != nil {
				return handleSecureWriteError(command, operationID, err)
			}
			return renderCommandResultWithRequestID(command, operationID, requestID, data)
		},
	}
	addSecureWriteFlags(command, &options)
	command.Flags().BoolVar(&newWorkflow, "new-workflow", false, "Create a one-node DolphinScheduler workflow")
	command.Flags().Uint64Var(&workflowCode, "workflow", 0, "Reuse an existing DolphinScheduler workflow code")
	command.Flags().Uint64Var(&connectionID, "connection-id", 0, "Scheduler connection ID")
	command.Flags().Uint64Var(&projectCode, "project-code", 0, "DolphinScheduler project code")
	command.Flags().StringVar(&workflowName, "workflow-name", "", "Name for a newly created workflow")
	command.Flags().StringArrayVar(&upstreams, "upstream", nil, "Upstream task codes when reusing a workflow")
	command.Flags().StringArrayVar(&downstreams, "downstream", nil, "Downstream task codes when reusing a workflow")
	command.Flags().BoolVar(&useBuiltinSchedule, "use-builtin-schedule", false, "Copy the STX built-in cron onto a new workflow (ignored for existing workflows, which keep their own schedule)")
	command.Flags().StringVar(&workerGroup, "worker-group", "", "DolphinScheduler worker group")
	command.Flags().StringVar(&environment, "environment", "", "DolphinScheduler worker environment name")
	command.Flags().BoolVar(&dryRun, "dry-run", false, "Validate the push without writing DolphinScheduler")
	command.Flags().IntVar(&expectedVersion, "expected-version", 0, "Reject the push when the Dolphin workflow version differs")
	command.Flags().StringArrayVar(&varFlags, "var", nil, "Per-run variable override as key=value")
	return command
}

func newSyncJobCancelByRefCommand(storeProvider authStoreProvider) *cobra.Command {
	var options secureWriteOptions
	var externalRef string
	var generation int

	command := &cobra.Command{
		Use:     "cancel-by-ref",
		Short:   "Cancel a SeaTunnel job by external scheduler reference",
		Example: "stx sync job cancel-by-ref --confirm",
		Args:    usageArgs(cobra.NoArgs),
		RunE: func(command *cobra.Command, _ []string) error {
			if strings.TrimSpace(externalRef) == "" {
				return clioutput.NewError(clioutput.CodeUsage, "--external-ref is required", clioutput.ExitUsage, false)
			}
			operationID := "sync.job.cancel-by-ref"
			client, headers, err := prepareSecureWrite(command, storeProvider, operationID, &options, "按外部调度引用停止作业；接管序号不匹配时忽略。")
			if err != nil {
				return err
			}
			body := map[string]any{"external_ref": strings.TrimSpace(externalRef), "attach_generation": generation}
			var data any
			requestID, err := client.RequestWithHeaders(command.Context(), http.MethodPost, "/api/v1/sync/jobs/cancel-by-ref", body, headers, &data)
			if err != nil {
				return handleSecureWriteError(command, operationID, err)
			}
			return renderCommandResultWithRequestID(command, operationID, requestID, data)
		},
	}
	addSecureWriteFlags(command, &options)
	command.Flags().StringVar(&externalRef, "external-ref", "", "External scheduler reference such as ds:{workflowInstance}:{taskCode}")
	command.Flags().IntVar(&generation, "attach-generation", 0, "Attach generation; 0 means the current generation")
	return command
}

func newSchedulerConnectionCreateCommand(storeProvider authStoreProvider) *cobra.Command {
	var options secureWriteOptions
	var name, baseURL, token, apiStyle, dolphinVersion string
	var defaultProjectCode int64

	command := &cobra.Command{
		Use:     "create",
		Short:   "Create a scheduler connection",
		Example: "stx sync scheduler connection create --confirm",
		Args:    usageArgs(cobra.NoArgs),
		RunE: func(command *cobra.Command, _ []string) error {
			if strings.TrimSpace(name) == "" || strings.TrimSpace(baseURL) == "" || strings.TrimSpace(token) == "" {
				return clioutput.NewError(clioutput.CodeUsage, "--name, --base-url and --token are required", clioutput.ExitUsage, false)
			}
			operationID := "sync.scheduler.connection.create"
			client, headers, err := prepareSecureWrite(command, storeProvider, operationID, &options, "保存 DolphinScheduler API 地址和令牌，供后续推送使用。")
			if err != nil {
				return err
			}
			body := map[string]any{
				"name":                 name,
				"base_url":             baseURL,
				"token":                token,
				"api_style":            apiStyle,
				"dolphin_version":      dolphinVersion,
				"default_project_code": defaultProjectCode,
			}
			var data any
			requestID, err := client.RequestWithHeaders(command.Context(), http.MethodPost, "/api/v1/sync/scheduler-connections", body, headers, &data)
			if err != nil {
				return handleSecureWriteError(command, operationID, err)
			}
			return renderCommandResultWithRequestID(command, operationID, requestID, data)
		},
	}
	addSecureWriteFlags(command, &options)
	command.Flags().StringVar(&name, "name", "", "Connection name")
	command.Flags().StringVar(&baseURL, "base-url", "", "DolphinScheduler API base URL")
	command.Flags().StringVar(&token, "token", "", "DolphinScheduler API token")
	command.Flags().StringVar(&apiStyle, "api-style", "auto", "API style: auto, process (3.1/3.2), or workflow (3.3+)")
	command.Flags().StringVar(&dolphinVersion, "dolphin-version", "", "DolphinScheduler 3.x version such as 3.2.2 or 3.3.1")
	command.Flags().Int64Var(&defaultProjectCode, "default-project-code", 0, "Default project code")
	return command
}

func newSchedulerConnectionUpdateCommand(storeProvider authStoreProvider) *cobra.Command {
	var options secureWriteOptions
	var name, baseURL, token, apiStyle, dolphinVersion string
	var defaultProjectCode int64

	command := &cobra.Command{
		Use:     "update <id>",
		Short:   "Update a scheduler connection",
		Example: "stx sync scheduler connection update 1 --confirm",
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			id, err := parseTaskID(args[0])
			if err != nil {
				return err
			}
			operationID := "sync.scheduler.connection.update"
			client, headers, err := prepareSecureWrite(command, storeProvider, operationID, &options, "更新调度器连接；空令牌表示保留原值。")
			if err != nil {
				return err
			}
			body := map[string]any{
				"name":                 name,
				"base_url":             baseURL,
				"token":                token,
				"api_style":            apiStyle,
				"dolphin_version":      dolphinVersion,
				"default_project_code": defaultProjectCode,
			}
			var data any
			requestID, err := client.RequestWithHeaders(command.Context(), http.MethodPut, fmt.Sprintf("/api/v1/sync/scheduler-connections/%d", id), body, headers, &data)
			if err != nil {
				return handleSecureWriteError(command, operationID, err)
			}
			return renderCommandResultWithRequestID(command, operationID, requestID, data)
		},
	}
	addSecureWriteFlags(command, &options)
	command.Flags().StringVar(&name, "name", "", "Connection name")
	command.Flags().StringVar(&baseURL, "base-url", "", "DolphinScheduler API base URL")
	command.Flags().StringVar(&token, "token", "", "DolphinScheduler API token; empty keeps the current token")
	command.Flags().StringVar(&apiStyle, "api-style", "", "API style: auto, process (3.1/3.2), or workflow (3.3+)")
	command.Flags().StringVar(&dolphinVersion, "dolphin-version", "", "DolphinScheduler 3.x version such as 3.2.2 or 3.3.1; empty keeps the current value")
	command.Flags().Int64Var(&defaultProjectCode, "default-project-code", 0, "Default project code")
	return command
}

func parseVarFlags(values []string) (map[string]string, error) {
	if len(values) == 0 {
		return nil, nil
	}
	result := map[string]string{}
	for _, item := range values {
		key, value, found := strings.Cut(item, "=")
		key = strings.TrimSpace(key)
		if !found || key == "" {
			return nil, clioutput.NewError(clioutput.CodeUsage, "--var must be key=value", clioutput.ExitUsage, false)
		}
		if unreplacedCLIPlaceholder.MatchString(strings.TrimSpace(value)) {
			return nil, clioutput.NewError(clioutput.CodeUsage, fmt.Sprintf("variable %s still contains an unreplaced ${...} placeholder", key), clioutput.ExitUsage, false)
		}
		result[key] = value
	}
	return result, nil
}

func parseCLIScheduleTime(raw string) (time.Time, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return time.Time{}, nil
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed, nil
	}
	if parsed, err := time.ParseInLocation("20060102150405", value, time.Local); err == nil {
		return parsed, nil
	}
	return time.Time{}, clioutput.NewError(clioutput.CodeUsage, "--schedule-time must be RFC3339 or yyyyMMddHHmmss", clioutput.ExitUsage, false)
}

func parseInt64Flags(values []string) []int64 {
	result := make([]int64, 0, len(values))
	for _, item := range values {
		parsed, err := strconv.ParseInt(strings.TrimSpace(item), 10, 64)
		if err != nil || parsed == 0 {
			continue
		}
		result = append(result, parsed)
	}
	return result
}
