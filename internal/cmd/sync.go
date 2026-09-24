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
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	clioutput "github.com/LeonYoah/stx/internal/cli/output"
	"github.com/spf13/cobra"
)

// addSyncWriteCommands 将数据同步工作台写命令与动作挂载到生成命令树。
// addSyncWriteCommands attaches sync workbench write and action commands to the generated command tree.
func addSyncWriteCommands(root *cobra.Command, storeProvider authStoreProvider) {
	syncCommand := childCommand(root, "sync")
	if syncCommand == nil {
		panic("generated sync command is missing")
	}

	taskCommand := childCommand(syncCommand, "task")
	if taskCommand == nil {
		panic("generated sync task command is missing")
	}
	taskCommand.AddCommand(
		newSyncTaskCreateCommand(storeProvider),
		newSyncTaskUpdateCommand(storeProvider),
		newSyncTaskPermissionsCommand(storeProvider),
		newSyncTaskPublishCommand(storeProvider),
		newSyncTaskValidateCommand(storeProvider),
		newSyncTaskTestConnectionsCommand(storeProvider),
		newSyncTaskDAGCommand(storeProvider),
		newSyncTaskPreviewCommand(storeProvider),
		newSyncTaskPreviewSaveModeCommand(storeProvider),
		newSyncTaskSubmitCommand(storeProvider),
	)

	versionCommand := childCommand(taskCommand, "version")
	if versionCommand == nil {
		panic("generated sync task version command is missing")
	}
	versionCommand.AddCommand(
		newSyncTaskVersionRollbackCommand(storeProvider),
	)

	jobCommand := childCommand(syncCommand, "job")
	if jobCommand == nil {
		panic("generated sync job command is missing")
	}
	jobCommand.AddCommand(
		newSyncJobCancelCommand(storeProvider),
		newSyncJobRecoverCommand(storeProvider),
	)

	variableCommand := childCommand(syncCommand, "variable")
	if variableCommand == nil {
		panic("generated sync variable command is missing")
	}
	variableCommand.AddCommand(
		newSyncVariableCreateCommand(storeProvider),
		newSyncVariableUpdateCommand(storeProvider),
	)

	pluginCommand := childCommand(syncCommand, "plugin")
	if pluginCommand == nil {
		pluginCommand = &cobra.Command{
			Use:   "plugin",
			Short: "STX sync plugin commands",
		}
		syncCommand.AddCommand(pluginCommand)
	}
	pluginCommand.AddCommand(
		newSyncPluginListCommand(storeProvider),
		newSyncPluginOptionsCommand(storeProvider),
		newSyncPluginTemplateCommand(storeProvider),
		newSyncPluginEnumValuesCommand(storeProvider),
		newSyncPluginEnumCatalogCommand(storeProvider),
	)

	curatedCommand := childCommand(syncCommand, "curated")
	if curatedCommand == nil {
		curatedCommand = &cobra.Command{
			Use:   "curated",
			Short: "STX sync curated template commands",
		}
		syncCommand.AddCommand(curatedCommand)
	}
	curatedCommand.AddCommand(
		newSyncCuratedListCommand(storeProvider),
		newSyncCuratedCreateCommand(storeProvider),
		newSyncCuratedUpdateCommand(storeProvider),
		newSyncCuratedDeleteCommand(storeProvider),
		newSyncCuratedForkCommand(storeProvider),
		newSyncCuratedRenderCommand(storeProvider),
		newSyncCuratedParseComboCommand(storeProvider),
	)
}

// newSyncTaskPermissionsCommand 提供独立的工作台权限命令，避免调用方手写完整任务更新正文。
// newSyncTaskPermissionsCommand exposes the workbench sharing permissions without
// forcing callers to construct the full task update payload.
func newSyncTaskPermissionsCommand(storeProvider authStoreProvider) *cobra.Command {
	command := &cobra.Command{
		Use:   "permissions",
		Short: "View or update sync task sharing permissions",
		Long:  "View or update whether a task is public and which users may collaborate on it.",
	}
	command.AddCommand(
		newSyncTaskPermissionsGetCommand(storeProvider),
		newSyncTaskPermissionsUpdateCommand(storeProvider),
	)
	return command
}

// newSyncTaskPermissionsGetCommand 读取任务的权限字段。
// newSyncTaskPermissionsGetCommand reads the permission fields from a task.
func newSyncTaskPermissionsGetCommand(storeProvider authStoreProvider) *cobra.Command {
	var namespace string

	command := &cobra.Command{
		Use:     "get <id>",
		Short:   "Get sync task sharing permissions",
		Example: "stx sync task permissions get 1",
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			taskID, err := parseTaskID(args[0])
			if err != nil {
				return err
			}
			client, err := clientForNamespace(storeProvider, namespace)
			if err != nil {
				return err
			}
			operationID := "sync.task.permissions.get"
			if err := checkSpecialOperation(command.Context(), client, operationID); err != nil {
				return err
			}

			var data any
			requestID, err := client.Request(command.Context(), http.MethodGet, fmt.Sprintf("/api/v1/sync/tasks/%d/permissions", taskID), nil, &data)
			if err != nil {
				return err
			}
			return renderCommandResultWithRequestID(command, operationID, requestID, data)
		},
	}
	command.Flags().StringVar(&namespace, "namespace", "", "Local namespace to use")
	return command
}

// newSyncTaskPermissionsUpdateCommand 通过独立权限接口修改共享字段，不回传任务正文。
// newSyncTaskPermissionsUpdateCommand updates only the sharing fields through
// the dedicated permissions endpoint, so task content is never sent back.
func newSyncTaskPermissionsUpdateCommand(storeProvider authStoreProvider) *cobra.Command {
	var options secureWriteOptions
	var public, private, clearCollaborators bool
	var collaboratorIDs []uint

	command := &cobra.Command{
		Use:     "update <id>",
		Short:   "Update sync task sharing permissions",
		Long:    "Replace the public flag and/or collaborator list for a sync task. Only the owner or an administrator may change these fields.",
		Example: "stx sync task permissions update 1 --public --collaborator-id 2 --collaborator-id 3 --confirm",
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			taskID, err := parseTaskID(args[0])
			if err != nil {
				return err
			}
			if public && private {
				return clioutput.NewError(clioutput.CodeUsage, "--public and --private cannot be used together", clioutput.ExitUsage, false)
			}
			if clearCollaborators && command.Flags().Changed("collaborator-id") {
				return clioutput.NewError(clioutput.CodeUsage, "--clear-collaborators cannot be used with --collaborator-id", clioutput.ExitUsage, false)
			}
			if !public && !private && !command.Flags().Changed("collaborator-id") && !clearCollaborators {
				return clioutput.NewError(clioutput.CodeUsage, "one of --public, --private, --collaborator-id, or --clear-collaborators is required", clioutput.ExitUsage, false)
			}

			operationID := "sync.task.permissions.update"
			client, headers, err := prepareSecureWrite(command, storeProvider, operationID, &options, "修改工作台任务的公开状态或协作者列表，不会修改任务正文。")
			if err != nil {
				return err
			}

			body := make(map[string]any, 2)
			if public || private {
				body["is_public"] = public
			}
			if clearCollaborators {
				body["collaborator_ids"] = []uint{}
			} else if command.Flags().Changed("collaborator-id") {
				body["collaborator_ids"] = collaboratorIDs
			}

			var data any
			requestID, err := client.RequestWithHeaders(command.Context(), http.MethodPut, fmt.Sprintf("/api/v1/sync/tasks/%d/permissions", taskID), body, headers, &data)
			if err != nil {
				return handleSecureWriteError(command, operationID, err)
			}
			return renderCommandResultWithRequestID(command, operationID, requestID, data)
		},
	}
	addSecureWriteFlags(command, &options)
	command.Flags().BoolVar(&public, "public", false, "Make the task visible to all users")
	command.Flags().BoolVar(&private, "private", false, "Make the task visible only to permitted users")
	command.Flags().UintSliceVar(&collaboratorIDs, "collaborator-id", nil, "Replace collaborators with this user ID; repeat the flag for multiple users")
	command.Flags().BoolVar(&clearCollaborators, "clear-collaborators", false, "Remove all collaborators")
	return command
}

// ----------------------------------------------------------------------
// Task Write Commands
// ----------------------------------------------------------------------

func newSyncTaskCreateCommand(storeProvider authStoreProvider) *cobra.Command {
	var options secureWriteOptions
	var name, nodeType, description, configFile, contentFormat, mode, definitionFile string
	var clusterID uint
	var parentID uint

	command := &cobra.Command{
		Use:     "create",
		Short:   "Create a sync task or folder",
		Long:    "Create a sync task node in the workspace. Supported types are 'file' and 'folder'.",
		Example: "stx sync task create --name my-task --cluster-id 1 --config-file task.conf --confirm",
		Args:    usageArgs(cobra.NoArgs),
		RunE: func(command *cobra.Command, _ []string) error {
			if strings.TrimSpace(name) == "" {
				return clioutput.NewError(clioutput.CodeUsage, "--name is required", clioutput.ExitUsage, false)
			}
			content := ""
			if strings.TrimSpace(configFile) != "" {
				data, err := os.ReadFile(configFile)
				if err != nil {
					return clioutput.WrapError(err, clioutput.CodeUsage, "read config file", clioutput.ExitUsage, false)
				}
				content = string(data)
			}

			definition := map[string]any{
				"execution_mode": "cluster",
				"preview_http_sink": map[string]any{
					"url":        "http://10.0.0.211:8000/api/v1/sync/preview/collect",
					"array_mode": false,
				},
				"preview_mode":            "source",
				"preview_output_format":   "hocon",
				"preview_row_limit":       100,
				"preview_timeout_minutes": 10,
			}
			if strings.TrimSpace(definitionFile) != "" {
				defData, err := os.ReadFile(definitionFile)
				if err != nil {
					return clioutput.WrapError(err, clioutput.CodeUsage, "read definition file", clioutput.ExitUsage, false)
				}
				if err := json.Unmarshal(defData, &definition); err != nil {
					return clioutput.WrapError(err, clioutput.CodeUsage, "decode definition file", clioutput.ExitUsage, false)
				}
			}

			body := map[string]any{
				"name":           name,
				"node_type":      nodeType,
				"description":    description,
				"cluster_id":     clusterID,
				"mode":           mode,
				"content_format": contentFormat,
				"content":        content,
				"definition":     definition,
			}
			if command.Flags().Changed("parent-id") {
				body["parent_id"] = parentID
			}

			operationID := "sync.task.create"
			client, headers, err := prepareSecureWrite(command, storeProvider, operationID, &options, "创建同步任务或目录节点。")
			if err != nil {
				return err
			}

			var data any
			requestID, err := client.RequestWithHeaders(command.Context(), http.MethodPost, "/api/v1/sync/tasks", body, headers, &data)
			if err != nil {
				return handleSecureWriteError(command, operationID, err)
			}
			return renderCommandResultWithRequestID(command, operationID, requestID, data)
		},
	}
	addSecureWriteFlags(command, &options)
	command.Flags().StringVar(&name, "name", "", "Task or folder name")
	command.Flags().StringVar(&nodeType, "type", "file", "Node type: file or folder")
	command.Flags().UintVar(&clusterID, "cluster-id", 0, "Associated SeaTunnel cluster ID")
	command.Flags().UintVar(&parentID, "parent-id", 0, "Parent folder ID")
	command.Flags().StringVar(&description, "description", "", "Task description")
	command.Flags().StringVar(&configFile, "config-file", "", "Path to SeaTunnel V2 configuration file (.conf/.json/.yaml)")
	command.Flags().StringVar(&contentFormat, "content-format", "hocon", "Configuration content format (hocon/json)")
	command.Flags().StringVar(&mode, "mode", "batch", "Job execution mode: batch or streaming")
	command.Flags().StringVar(&definitionFile, "definition-file", "", "Path to JSON file containing task definition options")
	return command
}

func newSyncTaskUpdateCommand(storeProvider authStoreProvider) *cobra.Command {
	var options secureWriteOptions
	var name, description, configFile, contentFormat, mode, definitionFile string
	var clusterID uint

	command := &cobra.Command{
		Use:     "update <id>",
		Short:   "Update a sync task",
		Long:    "Update properties or configuration content of an existing sync task.",
		Example: "stx sync task update 1 --name updated-task --config-file new.conf --confirm",
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			taskID, err := parseTaskID(args[0])
			if err != nil {
				return err
			}

			operationID := "sync.task.update"
			client, headers, err := prepareSecureWrite(command, storeProvider, operationID, &options, "更新同步任务的基础信息或配置内容。")
			if err != nil {
				return err
			}

			var existing struct {
				Name          string         `json:"name"`
				Description   string         `json:"description"`
				ClusterID     uint           `json:"cluster_id"`
				NodeType      string         `json:"node_type"`
				ParentID      *uint          `json:"parent_id"`
				Mode          string         `json:"mode"`
				ContentFormat string         `json:"content_format"`
				Content       string         `json:"content"`
				Definition    map[string]any `json:"definition"`
			}
			_, _ = client.Request(command.Context(), http.MethodGet, fmt.Sprintf("/api/v1/sync/tasks/%d", taskID), nil, &existing)

			body := map[string]any{
				"name":           existing.Name,
				"description":    existing.Description,
				"cluster_id":     existing.ClusterID,
				"node_type":      existing.NodeType,
				"mode":           existing.Mode,
				"content_format": existing.ContentFormat,
				"content":        existing.Content,
				"definition":     existing.Definition,
			}
			if existing.ParentID != nil {
				body["parent_id"] = *existing.ParentID
			}

			if command.Flags().Changed("name") {
				body["name"] = name
			}
			if command.Flags().Changed("description") {
				body["description"] = description
			}
			if command.Flags().Changed("cluster-id") {
				body["cluster_id"] = clusterID
			}
			if command.Flags().Changed("mode") {
				body["mode"] = mode
			}
			if command.Flags().Changed("content-format") {
				body["content_format"] = contentFormat
			}
			if strings.TrimSpace(configFile) != "" {
				data, err := os.ReadFile(configFile)
				if err != nil {
					return clioutput.WrapError(err, clioutput.CodeUsage, "read config file", clioutput.ExitUsage, false)
				}
				body["content"] = string(data)
			}
			if strings.TrimSpace(definitionFile) != "" {
				defData, err := os.ReadFile(definitionFile)
				if err != nil {
					return clioutput.WrapError(err, clioutput.CodeUsage, "read definition file", clioutput.ExitUsage, false)
				}
				var definition map[string]any
				if err := json.Unmarshal(defData, &definition); err != nil {
					return clioutput.WrapError(err, clioutput.CodeUsage, "decode definition file", clioutput.ExitUsage, false)
				}
				body["definition"] = definition
			}

			var data any
			requestID, err := client.RequestWithHeaders(command.Context(), http.MethodPut, fmt.Sprintf("/api/v1/sync/tasks/%d", taskID), body, headers, &data)
			if err != nil {
				return handleSecureWriteError(command, operationID, err)
			}
			return renderCommandResultWithRequestID(command, operationID, requestID, data)
		},
	}
	addSecureWriteFlags(command, &options)
	command.Flags().StringVar(&name, "name", "", "Task name")
	command.Flags().UintVar(&clusterID, "cluster-id", 0, "Associated SeaTunnel cluster ID")
	command.Flags().StringVar(&description, "description", "", "Task description")
	command.Flags().StringVar(&configFile, "config-file", "", "Path to SeaTunnel V2 configuration file")
	command.Flags().StringVar(&contentFormat, "content-format", "hocon", "Configuration content format")
	command.Flags().StringVar(&mode, "mode", "batch", "Job execution mode: batch or streaming")
	command.Flags().StringVar(&definitionFile, "definition-file", "", "Path to JSON file containing task definition options")
	return command
}

func newSyncTaskPublishCommand(storeProvider authStoreProvider) *cobra.Command {
	var options secureWriteOptions
	var comment string

	command := &cobra.Command{
		Use:     "publish <id>",
		Short:   "Publish a sync task version",
		Long:    "Freeze current sync task configuration into a new immutable historical version.",
		Example: "stx sync task publish 1 --comment 'Release v1.0' --confirm",
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			taskID, err := parseTaskID(args[0])
			if err != nil {
				return err
			}

			body := map[string]any{"comment": comment}
			operationID := "sync.task.publish"
			client, headers, err := prepareSecureWrite(command, storeProvider, operationID, &options, "将当前任务配置冻结发布为一个新的历史版本。")
			if err != nil {
				return err
			}

			var data any
			requestID, err := client.RequestWithHeaders(command.Context(), http.MethodPost, fmt.Sprintf("/api/v1/sync/tasks/%d/publish", taskID), body, headers, &data)
			if err != nil {
				return handleSecureWriteError(command, operationID, err)
			}
			return renderCommandResultWithRequestID(command, operationID, requestID, data)
		},
	}
	addSecureWriteFlags(command, &options)
	command.Flags().StringVar(&comment, "comment", "", "Publish commit comment or release note")
	return command
}

func newSyncTaskValidateCommand(storeProvider authStoreProvider) *cobra.Command {
	var namespace, configFile string
	command := &cobra.Command{
		Use:     "validate <id>",
		Short:   "Validate a sync task configuration",
		Long:    "Validate whether a sync task configuration conforms to SeaTunnel grammar and plugin schemas.",
		Example: "stx sync task validate 1",
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			taskID, err := parseTaskID(args[0])
			if err != nil {
				return err
			}
			client, err := clientForNamespace(storeProvider, namespace)
			if err != nil {
				return err
			}

			var body any
			if strings.TrimSpace(configFile) != "" {
				data, err := os.ReadFile(configFile)
				if err != nil {
					return clioutput.WrapError(err, clioutput.CodeUsage, "read config file", clioutput.ExitUsage, false)
				}
				body = map[string]any{"draft": map[string]any{"content": string(data)}}
			}

			var data any
			operationID := "sync.task.validate"
			requestID, err := client.Request(command.Context(), http.MethodPost, fmt.Sprintf("/api/v1/sync/tasks/%d/validate", taskID), body, &data)
			if err != nil {
				return err
			}
			return renderCommandResultWithRequestID(command, operationID, requestID, data)
		},
	}
	command.Flags().StringVar(&namespace, "namespace", "", "Local namespace to use")
	command.Flags().StringVar(&configFile, "config-file", "", "Optional configuration file to validate instead of saved content")
	return command
}

func newSyncTaskTestConnectionsCommand(storeProvider authStoreProvider) *cobra.Command {
	var namespace, configFile string
	command := &cobra.Command{
		Use:     "test-connections <id>",
		Short:   "Test data source connections in a sync task",
		Long:    "Proactively test reachability and authentication for all source and sink data sources in the task.",
		Example: "stx sync task test-connections 1",
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			taskID, err := parseTaskID(args[0])
			if err != nil {
				return err
			}
			client, err := clientForNamespace(storeProvider, namespace)
			if err != nil {
				return err
			}

			var body any
			if strings.TrimSpace(configFile) != "" {
				data, err := os.ReadFile(configFile)
				if err != nil {
					return clioutput.WrapError(err, clioutput.CodeUsage, "read config file", clioutput.ExitUsage, false)
				}
				body = map[string]any{"draft": map[string]any{"content": string(data)}}
			}

			var data any
			operationID := "sync.task.test-connections"
			requestID, err := client.Request(command.Context(), http.MethodPost, fmt.Sprintf("/api/v1/sync/tasks/%d/test-connections", taskID), body, &data)
			if err != nil {
				return err
			}
			return renderCommandResultWithRequestID(command, operationID, requestID, data)
		},
	}
	command.Flags().StringVar(&namespace, "namespace", "", "Local namespace to use")
	command.Flags().StringVar(&configFile, "config-file", "", "Optional configuration file to test instead of saved content")
	return command
}

func newSyncTaskDAGCommand(storeProvider authStoreProvider) *cobra.Command {
	var namespace, configFile string
	command := &cobra.Command{
		Use:     "dag <id>",
		Short:   "Get DAG graph for a sync task",
		Long:    "Parse and generate visual Directed Acyclic Graph (DAG) nodes and edges for the sync task.",
		Example: "stx sync task dag 1",
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			taskID, err := parseTaskID(args[0])
			if err != nil {
				return err
			}
			client, err := clientForNamespace(storeProvider, namespace)
			if err != nil {
				return err
			}

			var body any
			if strings.TrimSpace(configFile) != "" {
				data, err := os.ReadFile(configFile)
				if err != nil {
					return clioutput.WrapError(err, clioutput.CodeUsage, "read config file", clioutput.ExitUsage, false)
				}
				body = map[string]any{"draft": map[string]any{"content": string(data)}}
			}

			var data any
			operationID := "sync.task.dag"
			requestID, err := client.Request(command.Context(), http.MethodPost, fmt.Sprintf("/api/v1/sync/tasks/%d/dag", taskID), body, &data)
			if err != nil {
				return err
			}
			return renderCommandResultWithRequestID(command, operationID, requestID, data)
		},
	}
	command.Flags().StringVar(&namespace, "namespace", "", "Local namespace to use")
	command.Flags().StringVar(&configFile, "config-file", "", "Optional configuration file to parse instead of saved content")
	return command
}

func newSyncTaskPreviewCommand(storeProvider authStoreProvider) *cobra.Command {
	var options secureWriteOptions
	var rowLimit, timeoutMinutes int
	var wait bool

	command := &cobra.Command{
		Use:     "preview <id>",
		Short:   "Start a preview sampling session for a sync task",
		Long:    "Execute a lightweight sampling run to preview transformed and extracted records before submitting.",
		Example: "stx sync task preview 1 --row-limit 10 --wait --confirm",
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			taskID, err := parseTaskID(args[0])
			if err != nil {
				return err
			}

			body := map[string]any{
				"row_limit":       rowLimit,
				"timeout_minutes": timeoutMinutes,
			}

			operationID := "sync.task.preview"
			client, headers, err := prepareSecureWrite(command, storeProvider, operationID, &options, "启动一次轻量数据抽样预览会话，消耗短暂的引擎计算资源。")
			if err != nil {
				return err
			}

			var data struct {
				ID          uint   `json:"id"`
				ExecutionID string `json:"execution_id"`
				Status      string `json:"status"`
			}
			requestID, err := client.RequestWithHeaders(command.Context(), http.MethodPost, fmt.Sprintf("/api/v1/sync/tasks/%d/preview", taskID), body, headers, &data)
			if err != nil {
				return handleSecureWriteError(command, operationID, err)
			}

			if wait && strings.TrimSpace(data.ExecutionID) != "" {
				_ = clioutput.NewEventWriter(command.ErrOrStderr()).Emit(clioutput.Event{
					Event:       "execution_wait",
					OperationID: operationID,
					RequestID:   requestID,
					Level:       "info",
					Message:     fmt.Sprintf("waiting for preview execution %s", data.ExecutionID),
				})
				waitRequestID, waitData, waitErr := client.WaitExecution(command.Context(), data.ExecutionID, 60)
				if waitErr == nil {
					return renderCommandResultWithRequestID(command, operationID, waitRequestID, waitData)
				}
			}

			return renderCommandResultWithRequestID(command, operationID, requestID, data)
		},
	}
	addSecureWriteFlags(command, &options)
	command.Flags().IntVar(&rowLimit, "row-limit", 10, "Maximum number of rows to sample")
	command.Flags().IntVar(&timeoutMinutes, "timeout-minutes", 2, "Preview session timeout in minutes")
	command.Flags().BoolVar(&wait, "wait", false, "Wait for the preview session execution to complete")
	return command
}

func newSyncTaskPreviewSaveModeCommand(storeProvider authStoreProvider) *cobra.Command {
	var namespace, configFile string
	var clusterID uint

	command := &cobra.Command{
		Use:     "preview-savemode <id>",
		Short:   "Preview sink SaveMode SQL and actions for a sync task",
		Long:    "Inspect generated DDL, table-drop, or table-create SQL generated by SeaTunnel sink SaveMode.",
		Example: "stx sync task preview-savemode 1 --cluster-id 1",
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			taskID, err := parseTaskID(args[0])
			if err != nil {
				return err
			}
			if clusterID == 0 {
				clusterID = 1
			}

			client, err := clientForNamespace(storeProvider, namespace)
			if err != nil {
				return err
			}

			body := map[string]any{
				"cluster_id": clusterID,
			}
			if strings.TrimSpace(configFile) != "" {
				data, err := os.ReadFile(configFile)
				if err != nil {
					return clioutput.WrapError(err, clioutput.CodeUsage, "read config file", clioutput.ExitUsage, false)
				}
				body["draft"] = map[string]any{"content": string(data)}
			}

			var response struct {
				ErrorMsg string `json:"error_msg"`
				Data     any    `json:"data"`
			}
			operationID := "sync.task.preview-savemode"
			requestID, err := client.Request(command.Context(), http.MethodPost, fmt.Sprintf("/api/v1/sync/tasks/%d/preview/sink-savemode", taskID), body, &response)
			if err != nil {
				return err
			}
			return renderCommandResultWithRequestID(command, operationID, requestID, response.Data)
		},
	}
	command.Flags().StringVar(&namespace, "namespace", "", "Local namespace to use")
	command.Flags().UintVar(&clusterID, "cluster-id", 1, "Target SeaTunnel cluster ID")
	command.Flags().StringVar(&configFile, "config-file", "", "Optional configuration file to test")
	return command
}

func newSyncTaskSubmitCommand(storeProvider authStoreProvider) *cobra.Command {
	var options secureWriteOptions
	var wait bool

	command := &cobra.Command{
		Use:     "submit <id>",
		Short:   "Submit a sync task to execute on the cluster",
		Long:    "Submit a SeaTunnel synchronization task to the assigned cluster and create an active job instance.",
		Example: "stx sync task submit 1 --wait --confirm",
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			taskID, err := parseTaskID(args[0])
			if err != nil {
				return err
			}

			operationID := "sync.task.submit"
			client, headers, err := prepareSecureWrite(command, storeProvider, operationID, &options, "提交分布式 SeaTunnel 作业运行，占用集群 Worker 槽位与资源。")
			if err != nil {
				return err
			}

			var data struct {
				ID            uint   `json:"id"`
				TaskID        uint   `json:"task_id"`
				PlatformJobID string `json:"platform_job_id"`
				EngineJobID   string `json:"engine_job_id"`
				Status        string `json:"status"`
				ExecutionID   string `json:"execution_id"`
			}
			requestID, err := client.RequestWithHeaders(command.Context(), http.MethodPost, fmt.Sprintf("/api/v1/sync/tasks/%d/submit", taskID), map[string]any{}, headers, &data)
			if err != nil {
				return handleSecureWriteError(command, operationID, err)
			}

			if wait && strings.TrimSpace(data.ExecutionID) != "" {
				_ = clioutput.NewEventWriter(command.ErrOrStderr()).Emit(clioutput.Event{
					Event:       "execution_wait",
					OperationID: operationID,
					RequestID:   requestID,
					Level:       "info",
					Message:     fmt.Sprintf("waiting for job execution %s", data.ExecutionID),
				})
				waitRequestID, waitData, waitErr := client.WaitExecution(command.Context(), data.ExecutionID, 300)
				if waitErr == nil {
					return renderCommandResultWithRequestID(command, operationID, waitRequestID, waitData)
				}
			}

			return renderCommandResultWithRequestID(command, operationID, requestID, data)
		},
	}
	addSecureWriteFlags(command, &options)
	command.Flags().BoolVar(&wait, "wait", false, "Wait for the execution lifecycle to complete or reach terminal state")
	return command
}

// ----------------------------------------------------------------------
// Task Version Commands
// ----------------------------------------------------------------------

func newSyncTaskVersionRollbackCommand(storeProvider authStoreProvider) *cobra.Command {
	var options secureWriteOptions
	command := &cobra.Command{
		Use:     "rollback <task_id> <version_id>",
		Short:   "Rollback a sync task to a historical version",
		Long:    "Restore a sync task's draft content from a specific historical version.",
		Example: "stx sync task version rollback 1 2 --confirm",
		Args:    usageArgs(cobra.ExactArgs(2)),
		RunE: func(command *cobra.Command, args []string) error {
			taskID, err := parseTaskID(args[0])
			if err != nil {
				return err
			}
			versionID, err := parseTaskID(args[1])
			if err != nil {
				return err
			}

			operationID := "sync.task.version.rollback"
			client, headers, err := prepareSecureWrite(command, storeProvider, operationID, &options, "将任务当前草稿内容回滚至指定历史版本。")
			if err != nil {
				return err
			}

			var data any
			path := fmt.Sprintf("/api/v1/sync/tasks/%d/versions/%d/rollback", taskID, versionID)
			requestID, err := client.RequestWithHeaders(command.Context(), http.MethodPost, path, map[string]any{}, headers, &data)
			if err != nil {
				return handleSecureWriteError(command, operationID, err)
			}
			return renderCommandResultWithRequestID(command, operationID, requestID, data)
		},
	}
	addSecureWriteFlags(command, &options)
	return command
}

// ----------------------------------------------------------------------
// Job Commands
// ----------------------------------------------------------------------

func newSyncJobCancelCommand(storeProvider authStoreProvider) *cobra.Command {
	var options secureWriteOptions
	var savepoint bool

	command := &cobra.Command{
		Use:     "cancel <id>",
		Short:   "Cancel a running SeaTunnel job",
		Long:    "Request graceful termination of a running SeaTunnel job. Use --savepoint to trigger a savepoint before stopping.",
		Example: "stx sync job cancel 1 --savepoint --confirm",
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			jobID, err := parseTaskID(args[0])
			if err != nil {
				return err
			}

			operationID := "sync.job.cancel"
			client, headers, err := prepareSecureWrite(command, storeProvider, operationID, &options, "终止集群中正在运行的作业，可指定 --savepoint 保存当前状态。")
			if err != nil {
				return err
			}

			body := map[string]any{"stop_with_savepoint": savepoint}
			var data any
			requestID, err := client.RequestWithHeaders(command.Context(), http.MethodPost, fmt.Sprintf("/api/v1/sync/jobs/%d/cancel", jobID), body, headers, &data)
			if err != nil {
				return handleSecureWriteError(command, operationID, err)
			}
			return renderCommandResultWithRequestID(command, operationID, requestID, data)
		},
	}
	addSecureWriteFlags(command, &options)
	command.Flags().BoolVar(&savepoint, "savepoint", false, "Trigger SeaTunnel Savepoint before stopping the job")
	return command
}

func newSyncJobRecoverCommand(storeProvider authStoreProvider) *cobra.Command {
	var options secureWriteOptions
	var wait bool

	command := &cobra.Command{
		Use:     "recover <id>",
		Short:   "Recover a stopped or failed job from checkpoint or savepoint",
		Long:    "Resume a SeaTunnel job from its last recorded checkpoint or savepoint state.",
		Example: "stx sync job recover 1 --wait --confirm",
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			jobID, err := parseTaskID(args[0])
			if err != nil {
				return err
			}

			operationID := "sync.job.recover"
			client, headers, err := prepareSecureWrite(command, storeProvider, operationID, &options, "从最后 Checkpoint 或 Savepoint 恢复作业，重新提交至集群运行。")
			if err != nil {
				return err
			}

			var data struct {
				ID          uint   `json:"id"`
				ExecutionID string `json:"execution_id"`
				Status      string `json:"status"`
			}
			requestID, err := client.RequestWithHeaders(command.Context(), http.MethodPost, fmt.Sprintf("/api/v1/sync/jobs/%d/recover", jobID), map[string]any{}, headers, &data)
			if err != nil {
				return handleSecureWriteError(command, operationID, err)
			}

			if wait && strings.TrimSpace(data.ExecutionID) != "" {
				_ = clioutput.NewEventWriter(command.ErrOrStderr()).Emit(clioutput.Event{
					Event:       "execution_wait",
					OperationID: operationID,
					RequestID:   requestID,
					Level:       "info",
					Message:     fmt.Sprintf("waiting for recovery execution %s", data.ExecutionID),
				})
				waitRequestID, waitData, waitErr := client.WaitExecution(command.Context(), data.ExecutionID, 300)
				if waitErr == nil {
					return renderCommandResultWithRequestID(command, operationID, waitRequestID, waitData)
				}
			}

			return renderCommandResultWithRequestID(command, operationID, requestID, data)
		},
	}
	addSecureWriteFlags(command, &options)
	command.Flags().BoolVar(&wait, "wait", false, "Wait for the recovery execution to complete")
	return command
}

// ----------------------------------------------------------------------
// Variable Commands
// ----------------------------------------------------------------------

func newSyncVariableCreateCommand(storeProvider authStoreProvider) *cobra.Command {
	var options secureWriteOptions
	var key, value, valueType, description string

	command := &cobra.Command{
		Use:     "create",
		Short:   "Create a global sync variable",
		Long:    "Define a new global key-value variable accessible by all synchronization tasks.",
		Example: "stx sync variable create --key DEFAULT_BATCH --value 1000 --confirm",
		Args:    usageArgs(cobra.NoArgs),
		RunE: func(command *cobra.Command, _ []string) error {
			if strings.TrimSpace(key) == "" {
				return clioutput.NewError(clioutput.CodeUsage, "--key is required", clioutput.ExitUsage, false)
			}
			body := map[string]any{
				"key":         key,
				"value":       value,
				"value_type":  valueType,
				"description": description,
			}

			operationID := "sync.variable.create"
			client, headers, err := prepareSecureWrite(command, storeProvider, operationID, &options, "创建全局变量，供所有同步任务引用。")
			if err != nil {
				return err
			}

			var data any
			requestID, err := client.RequestWithHeaders(command.Context(), http.MethodPost, "/api/v1/sync/global-variables", body, headers, &data)
			if err != nil {
				return handleSecureWriteError(command, operationID, err)
			}
			return renderCommandResultWithRequestID(command, operationID, requestID, data)
		},
	}
	addSecureWriteFlags(command, &options)
	command.Flags().StringVar(&key, "key", "", "Variable key name")
	command.Flags().StringVar(&value, "value", "", "Variable value string")
	command.Flags().StringVar(&valueType, "type", "string", "Variable type (string/int/password)")
	command.Flags().StringVar(&description, "description", "", "Variable description")
	return command
}

func newSyncVariableUpdateCommand(storeProvider authStoreProvider) *cobra.Command {
	var options secureWriteOptions
	var key, value, valueType, description string

	command := &cobra.Command{
		Use:     "update <id>",
		Short:   "Update a global sync variable",
		Long:    "Modify value or description of an existing global variable.",
		Example: "stx sync variable update 1 --value 2000 --confirm",
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			varID, err := parseTaskID(args[0])
			if err != nil {
				return err
			}

			body := map[string]any{}
			if command.Flags().Changed("key") {
				body["key"] = key
			}
			if command.Flags().Changed("value") {
				body["value"] = value
			}
			if command.Flags().Changed("type") {
				body["value_type"] = valueType
			}
			if command.Flags().Changed("description") {
				body["description"] = description
			}

			operationID := "sync.variable.update"
			client, headers, err := prepareSecureWrite(command, storeProvider, operationID, &options, "更新全局变量值，会影响后续引用该变量的任务运行。")
			if err != nil {
				return err
			}

			var data any
			requestID, err := client.RequestWithHeaders(command.Context(), http.MethodPut, fmt.Sprintf("/api/v1/sync/global-variables/%d", varID), body, headers, &data)
			if err != nil {
				return handleSecureWriteError(command, operationID, err)
			}
			return renderCommandResultWithRequestID(command, operationID, requestID, data)
		},
	}
	addSecureWriteFlags(command, &options)
	command.Flags().StringVar(&key, "key", "", "Variable key name")
	command.Flags().StringVar(&value, "value", "", "Variable value string")
	command.Flags().StringVar(&valueType, "type", "string", "Variable type")
	command.Flags().StringVar(&description, "description", "", "Variable description")
	return command
}

// ----------------------------------------------------------------------
// Plugin Commands
// ----------------------------------------------------------------------

func newSyncPluginListCommand(storeProvider authStoreProvider) *cobra.Command {
	var namespace, pluginType string
	var clusterID uint

	command := &cobra.Command{
		Use:     "list",
		Short:   "List supported sync plugin factories",
		Long:    "Query connectors and transform plugins supported by the SeaTunnel engine.",
		Example: "stx sync plugin list --cluster-id 1 --type source",
		Args:    usageArgs(cobra.NoArgs),
		RunE: func(command *cobra.Command, _ []string) error {
			if strings.TrimSpace(pluginType) == "" {
				pluginType = "source"
			}
			if clusterID == 0 {
				clusterID = 1
			}

			client, err := clientForNamespace(storeProvider, namespace)
			if err != nil {
				return err
			}

			body := map[string]any{
				"cluster_id":  clusterID,
				"plugin_type": pluginType,
			}

			var data any
			operationID := "sync.plugin.list"
			requestID, err := client.Request(command.Context(), http.MethodPost, "/api/v1/sync/plugins/list", body, &data)
			if err != nil {
				return err
			}
			return renderCommandResultWithRequestID(command, operationID, requestID, data)
		},
	}
	command.Flags().StringVar(&namespace, "namespace", "", "Local namespace to use")
	command.Flags().UintVar(&clusterID, "cluster-id", 1, "Target SeaTunnel cluster ID")
	command.Flags().StringVar(&pluginType, "type", "source", "Filter by plugin type: source, transform, or sink")
	return command
}

func newSyncPluginOptionsCommand(storeProvider authStoreProvider) *cobra.Command {
	var namespace, factoryIdentifier, pluginType string
	var clusterID uint
	var includeSupplement bool

	command := &cobra.Command{
		Use:     "options",
		Short:   "Get plugin option configuration schema",
		Long:    "Inspect option schemas, defaults, and descriptions for a specific SeaTunnel connector.",
		Example: "stx sync plugin options --cluster-id 1 --plugin MySQL-CDC --type source",
		Args:    usageArgs(cobra.NoArgs),
		RunE: func(command *cobra.Command, _ []string) error {
			if strings.TrimSpace(factoryIdentifier) == "" {
				return clioutput.NewError(clioutput.CodeUsage, "--plugin is required", clioutput.ExitUsage, false)
			}
			if strings.TrimSpace(pluginType) == "" {
				pluginType = "source"
			}
			if clusterID == 0 {
				clusterID = 1
			}

			client, err := clientForNamespace(storeProvider, namespace)
			if err != nil {
				return err
			}

			body := map[string]any{
				"cluster_id":         clusterID,
				"plugin_type":        pluginType,
				"factory_identifier": factoryIdentifier,
				"include_supplement": includeSupplement,
			}

			var data any
			operationID := "sync.plugin.options"
			requestID, err := client.Request(command.Context(), http.MethodPost, "/api/v1/sync/plugins/options", body, &data)
			if err != nil {
				return err
			}
			return renderCommandResultWithRequestID(command, operationID, requestID, data)
		},
	}
	command.Flags().StringVar(&namespace, "namespace", "", "Local namespace to use")
	command.Flags().UintVar(&clusterID, "cluster-id", 1, "Target SeaTunnel cluster ID")
	command.Flags().StringVar(&factoryIdentifier, "plugin", "", "Plugin factory identifier (e.g. MySQL-CDC, Jdbc, FakeSource)")
	command.Flags().StringVar(&pluginType, "type", "source", "Plugin type: source, transform, or sink")
	command.Flags().BoolVar(&includeSupplement, "include-supplement", true, "Include supplemental schema annotations")
	return command
}

func newSyncPluginTemplateCommand(storeProvider authStoreProvider) *cobra.Command {
	var namespace, factoryIdentifier, pluginType string
	var clusterID uint
	var includeSupplement, includeComments, includeAdvanced bool

	command := &cobra.Command{
		Use:     "template",
		Short:   "Render plugin configuration template",
		Long:    "Generate template configuration for a SeaTunnel plugin factory.",
		Example: "stx sync plugin template --cluster-id 1 --plugin FakeSource --type source",
		Args:    usageArgs(cobra.NoArgs),
		RunE: func(command *cobra.Command, _ []string) error {
			if strings.TrimSpace(factoryIdentifier) == "" {
				return clioutput.NewError(clioutput.CodeUsage, "--plugin is required", clioutput.ExitUsage, false)
			}
			if strings.TrimSpace(pluginType) == "" {
				pluginType = "source"
			}
			if clusterID == 0 {
				clusterID = 1
			}

			client, err := clientForNamespace(storeProvider, namespace)
			if err != nil {
				return err
			}

			body := map[string]any{
				"cluster_id":         clusterID,
				"plugin_type":        pluginType,
				"factory_identifier": factoryIdentifier,
				"include_supplement": includeSupplement,
				"include_comments":   includeComments,
				"include_advanced":   includeAdvanced,
			}

			var data any
			operationID := "sync.plugin.template"
			requestID, err := client.Request(command.Context(), http.MethodPost, "/api/v1/sync/plugins/template", body, &data)
			if err != nil {
				return err
			}
			return renderCommandResultWithRequestID(command, operationID, requestID, data)
		},
	}
	command.Flags().StringVar(&namespace, "namespace", "", "Local namespace to use")
	command.Flags().UintVar(&clusterID, "cluster-id", 1, "Target SeaTunnel cluster ID")
	command.Flags().StringVar(&factoryIdentifier, "plugin", "", "Plugin factory identifier")
	command.Flags().StringVar(&pluginType, "type", "source", "Plugin type: source, transform, or sink")
	command.Flags().BoolVar(&includeSupplement, "include-supplement", true, "Include supplemental comments")
	command.Flags().BoolVar(&includeComments, "include-comments", true, "Include option description comments")
	command.Flags().BoolVar(&includeAdvanced, "include-advanced", false, "Include advanced options")
	return command
}

func newSyncPluginEnumValuesCommand(storeProvider authStoreProvider) *cobra.Command {
	var namespace, factoryIdentifier, pluginType, optionKey string
	var clusterID uint

	command := &cobra.Command{
		Use:     "enum-values",
		Short:   "Get dynamic enum values for plugin options",
		Long:    "Retrieve allowable enumeration values for a plugin option field.",
		Example: "stx sync plugin enum-values --cluster-id 1 --plugin Jdbc --type sink --field primary_keys",
		Args:    usageArgs(cobra.NoArgs),
		RunE: func(command *cobra.Command, _ []string) error {
			if strings.TrimSpace(factoryIdentifier) == "" || strings.TrimSpace(optionKey) == "" {
				return clioutput.NewError(clioutput.CodeUsage, "--plugin and --field are required", clioutput.ExitUsage, false)
			}
			if strings.TrimSpace(pluginType) == "" {
				pluginType = "source"
			}
			if clusterID == 0 {
				clusterID = 1
			}

			client, err := clientForNamespace(storeProvider, namespace)
			if err != nil {
				return err
			}

			body := map[string]any{
				"cluster_id":         clusterID,
				"plugin_type":        pluginType,
				"factory_identifier": factoryIdentifier,
				"option_key":         optionKey,
			}

			var data any
			operationID := "sync.plugin.enum-values"
			requestID, err := client.Request(command.Context(), http.MethodPost, "/api/v1/sync/plugins/enum-values", body, &data)
			if err != nil {
				return err
			}
			return renderCommandResultWithRequestID(command, operationID, requestID, data)
		},
	}
	command.Flags().StringVar(&namespace, "namespace", "", "Local namespace to use")
	command.Flags().UintVar(&clusterID, "cluster-id", 1, "Target SeaTunnel cluster ID")
	command.Flags().StringVar(&factoryIdentifier, "plugin", "", "Plugin factory identifier")
	command.Flags().StringVar(&pluginType, "type", "source", "Plugin type: source, transform, or sink")
	command.Flags().StringVar(&optionKey, "field", "", "Option key/field name")
	return command
}

func newSyncPluginEnumCatalogCommand(storeProvider authStoreProvider) *cobra.Command {
	var namespace string
	var clusterID uint
	var includeSupplement bool

	command := &cobra.Command{
		Use:     "enum-catalog",
		Short:   "Get dynamic enum catalog for plugin options",
		Long:    "Retrieve catalog of all option enumerations available for connectors on a cluster.",
		Example: "stx sync plugin enum-catalog --cluster-id 1",
		Args:    usageArgs(cobra.NoArgs),
		RunE: func(command *cobra.Command, _ []string) error {
			if clusterID == 0 {
				clusterID = 1
			}

			client, err := clientForNamespace(storeProvider, namespace)
			if err != nil {
				return err
			}

			body := map[string]any{
				"cluster_id":         clusterID,
				"include_supplement": includeSupplement,
			}

			var data any
			operationID := "sync.plugin.enum-catalog"
			requestID, err := client.Request(command.Context(), http.MethodPost, "/api/v1/sync/plugins/enum-catalog", body, &data)
			if err != nil {
				return err
			}
			return renderCommandResultWithRequestID(command, operationID, requestID, data)
		},
	}
	command.Flags().StringVar(&namespace, "namespace", "", "Local namespace to use")
	command.Flags().UintVar(&clusterID, "cluster-id", 1, "Target SeaTunnel cluster ID")
	command.Flags().BoolVar(&includeSupplement, "include-supplement", true, "Include supplemental catalog entries")
	return command
}

// ----------------------------------------------------------------------
// Curated Template Commands
// ----------------------------------------------------------------------

func newSyncCuratedListCommand(storeProvider authStoreProvider) *cobra.Command {
	var namespace, section, query, origin string

	command := &cobra.Command{
		Use:     "list",
		Short:   "List curated sync templates",
		Long:    "List built-in and user curated sync templates, optionally filtered by section or keyword.",
		Example: "stx sync curated list --section source",
		Args:    usageArgs(cobra.NoArgs),
		RunE: func(command *cobra.Command, _ []string) error {
			client, err := clientForNamespace(storeProvider, namespace)
			if err != nil {
				return err
			}
			body := map[string]any{}
			if strings.TrimSpace(section) != "" {
				body["section"] = strings.TrimSpace(section)
			}
			if strings.TrimSpace(query) != "" {
				body["q"] = strings.TrimSpace(query)
			}
			if strings.TrimSpace(origin) != "" {
				body["origin"] = strings.TrimSpace(origin)
			}
			var data any
			operationID := "sync.curated.list"
			requestID, err := client.Request(command.Context(), http.MethodPost, "/api/v1/sync/curated-templates/list", body, &data)
			if err != nil {
				return err
			}
			return renderCommandResultWithRequestID(command, operationID, requestID, data)
		},
	}
	command.Flags().StringVar(&namespace, "namespace", "", "Local namespace to use")
	command.Flags().StringVar(&section, "section", "", "Filter by section: env|source|transform|sink|combo")
	command.Flags().StringVar(&query, "q", "", "Keyword search against name/description")
	command.Flags().StringVar(&origin, "origin", "", "Filter by origin: builtin|user|override|all")
	return command
}

func newSyncCuratedCreateCommand(storeProvider authStoreProvider) *cobra.Command {
	var options secureWriteOptions
	var name, description, section, mode, pattern, content, contentFile, builtinID string

	command := &cobra.Command{
		Use:     "create",
		Short:   "Create a curated sync template",
		Long:    "Save HOCON content as a user curated template for Studio reuse.",
		Example: "stx sync curated create --name my-jdbc --section source --content-file ./fragment.conf --confirm",
		Args:    usageArgs(cobra.NoArgs),
		RunE: func(command *cobra.Command, _ []string) error {
			if strings.TrimSpace(name) == "" {
				return clioutput.NewError(clioutput.CodeUsage, "--name is required", clioutput.ExitUsage, false)
			}
			bodyContent, err := resolveCuratedContent(content, contentFile)
			if err != nil {
				return err
			}
			body := map[string]any{
				"name":    name,
				"content": bodyContent,
			}
			if strings.TrimSpace(description) != "" {
				body["description"] = description
			}
			if strings.TrimSpace(section) != "" {
				body["section"] = section
			}
			if strings.TrimSpace(mode) != "" {
				body["mode"] = mode
			}
			if strings.TrimSpace(pattern) != "" {
				body["pattern"] = pattern
			}
			if strings.TrimSpace(builtinID) != "" {
				body["builtin_id"] = builtinID
			}

			operationID := "sync.curated.create"
			client, headers, err := prepareSecureWrite(command, storeProvider, operationID, &options, "创建精选模板，供工作台复用。")
			if err != nil {
				return err
			}
			var data any
			requestID, err := client.RequestWithHeaders(command.Context(), http.MethodPost, "/api/v1/sync/curated-templates", body, headers, &data)
			if err != nil {
				return handleSecureWriteError(command, operationID, err)
			}
			return renderCommandResultWithRequestID(command, operationID, requestID, data)
		},
	}
	addSecureWriteFlags(command, &options)
	command.Flags().StringVar(&name, "name", "", "Template name")
	command.Flags().StringVar(&description, "description", "", "Template description")
	command.Flags().StringVar(&section, "section", "", "Section: env|source|transform|sink|combo")
	command.Flags().StringVar(&mode, "mode", "", "Mode hint: BATCH|STREAMING|ANY")
	command.Flags().StringVar(&pattern, "pattern", "", "Pattern hint")
	command.Flags().StringVar(&content, "content", "", "Inline HOCON content")
	command.Flags().StringVar(&contentFile, "content-file", "", "File containing HOCON content")
	command.Flags().StringVar(&builtinID, "builtin-id", "", "Optional builtin id when creating an override row")
	return command
}

func newSyncCuratedUpdateCommand(storeProvider authStoreProvider) *cobra.Command {
	var options secureWriteOptions
	var name, description, section, mode, pattern, content, contentFile string
	var enabled bool

	command := &cobra.Command{
		Use:     "update <id>",
		Short:   "Update a curated sync template",
		Long:    "Update an owned curated template (user or override).",
		Example: "stx sync curated update 12 --content-file ./fragment.conf --confirm",
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			id, err := parseTaskID(args[0])
			if err != nil {
				return err
			}
			body := map[string]any{}
			if command.Flags().Changed("name") {
				body["name"] = name
			}
			if command.Flags().Changed("description") {
				body["description"] = description
			}
			if command.Flags().Changed("section") {
				body["section"] = section
			}
			if command.Flags().Changed("mode") {
				body["mode"] = mode
			}
			if command.Flags().Changed("pattern") {
				body["pattern"] = pattern
			}
			if command.Flags().Changed("content") || command.Flags().Changed("content-file") {
				bodyContent, readErr := resolveCuratedContent(content, contentFile)
				if readErr != nil {
					return readErr
				}
				body["content"] = bodyContent
			}
			if command.Flags().Changed("enabled") {
				body["enabled"] = enabled
			}
			if len(body) == 0 {
				return clioutput.NewError(clioutput.CodeUsage, "at least one update field is required", clioutput.ExitUsage, false)
			}

			operationID := "sync.curated.update"
			client, headers, err := prepareSecureWrite(command, storeProvider, operationID, &options, "更新精选模板内容。")
			if err != nil {
				return err
			}
			var data any
			requestID, err := client.RequestWithHeaders(command.Context(), http.MethodPut, fmt.Sprintf("/api/v1/sync/curated-templates/%d", id), body, headers, &data)
			if err != nil {
				return handleSecureWriteError(command, operationID, err)
			}
			return renderCommandResultWithRequestID(command, operationID, requestID, data)
		},
	}
	addSecureWriteFlags(command, &options)
	command.Flags().StringVar(&name, "name", "", "Template name")
	command.Flags().StringVar(&description, "description", "", "Template description")
	command.Flags().StringVar(&section, "section", "", "Section: env|source|transform|sink|combo")
	command.Flags().StringVar(&mode, "mode", "", "Mode hint")
	command.Flags().StringVar(&pattern, "pattern", "", "Pattern hint")
	command.Flags().StringVar(&content, "content", "", "Inline HOCON content")
	command.Flags().StringVar(&contentFile, "content-file", "", "File containing HOCON content")
	command.Flags().BoolVar(&enabled, "enabled", true, "Enable or disable the template")
	return command
}

func newSyncCuratedDeleteCommand(storeProvider authStoreProvider) *cobra.Command {
	var options secureWriteOptions

	command := &cobra.Command{
		Use:     "delete <id>",
		Short:   "Delete a curated sync template",
		Long:    "Delete an owned curated template. Built-in seeds cannot be deleted.",
		Example: "stx sync curated delete 12 --confirm",
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			id, err := parseTaskID(args[0])
			if err != nil {
				return err
			}
			operationID := "sync.curated.delete"
			client, headers, err := prepareSecureWrite(command, storeProvider, operationID, &options, "删除用户精选模板。")
			if err != nil {
				return err
			}
			var data any
			requestID, err := client.RequestWithHeaders(command.Context(), http.MethodDelete, fmt.Sprintf("/api/v1/sync/curated-templates/%d", id), nil, headers, &data)
			if err != nil {
				return handleSecureWriteError(command, operationID, err)
			}
			return renderCommandResultWithRequestID(command, operationID, requestID, data)
		},
	}
	addSecureWriteFlags(command, &options)
	return command
}

func newSyncCuratedForkCommand(storeProvider authStoreProvider) *cobra.Command {
	var options secureWriteOptions
	var builtinID, name, description string

	command := &cobra.Command{
		Use:     "fork",
		Short:   "Fork a built-in curated template",
		Long:    "Copy a built-in curated seed into an editable user override.",
		Example: "stx sync curated fork --builtin-id source-fake --confirm",
		Args:    usageArgs(cobra.NoArgs),
		RunE: func(command *cobra.Command, _ []string) error {
			if strings.TrimSpace(builtinID) == "" {
				return clioutput.NewError(clioutput.CodeUsage, "--builtin-id is required", clioutput.ExitUsage, false)
			}
			body := map[string]any{"builtin_id": strings.TrimSpace(builtinID)}
			if strings.TrimSpace(name) != "" {
				body["name"] = name
			}
			if strings.TrimSpace(description) != "" {
				body["description"] = description
			}
			operationID := "sync.curated.fork"
			client, headers, err := prepareSecureWrite(command, storeProvider, operationID, &options, "从内置精选生成可编辑副本。")
			if err != nil {
				return err
			}
			var data any
			requestID, err := client.RequestWithHeaders(command.Context(), http.MethodPost, "/api/v1/sync/curated-templates/fork", body, headers, &data)
			if err != nil {
				return handleSecureWriteError(command, operationID, err)
			}
			return renderCommandResultWithRequestID(command, operationID, requestID, data)
		},
	}
	addSecureWriteFlags(command, &options)
	command.Flags().StringVar(&builtinID, "builtin-id", "", "Built-in curated template id")
	command.Flags().StringVar(&name, "name", "", "Optional override name")
	command.Flags().StringVar(&description, "description", "", "Optional override description")
	return command
}

func newSyncCuratedRenderCommand(storeProvider authStoreProvider) *cobra.Command {
	var namespace, builtinID string
	var id, clusterID uint

	command := &cobra.Command{
		Use:     "render",
		Short:   "Render curated template content for insert",
		Long:    "Render curated content, rewriting plugin_input/output keys for legacy clusters when needed.",
		Example: "stx sync curated render --builtin-id env-batch --cluster-id 1",
		Args:    usageArgs(cobra.NoArgs),
		RunE: func(command *cobra.Command, _ []string) error {
			if strings.TrimSpace(builtinID) == "" && id == 0 {
				return clioutput.NewError(clioutput.CodeUsage, "--builtin-id or --id is required", clioutput.ExitUsage, false)
			}
			client, err := clientForNamespace(storeProvider, namespace)
			if err != nil {
				return err
			}
			body := map[string]any{}
			if strings.TrimSpace(builtinID) != "" {
				body["builtin_id"] = strings.TrimSpace(builtinID)
			}
			if id > 0 {
				body["id"] = id
			}
			if clusterID > 0 {
				body["cluster_id"] = clusterID
			}
			var data any
			operationID := "sync.curated.render"
			requestID, err := client.Request(command.Context(), http.MethodPost, "/api/v1/sync/curated-templates/render", body, &data)
			if err != nil {
				return err
			}
			return renderCommandResultWithRequestID(command, operationID, requestID, data)
		},
	}
	command.Flags().StringVar(&namespace, "namespace", "", "Local namespace to use")
	command.Flags().StringVar(&builtinID, "builtin-id", "", "Built-in curated template id")
	command.Flags().UintVar(&id, "id", 0, "User curated template id")
	command.Flags().UintVar(&clusterID, "cluster-id", 0, "Optional cluster id for legacy plugin IO rewrite")
	return command
}

func newSyncCuratedParseComboCommand(storeProvider authStoreProvider) *cobra.Command {
	var namespace, content, contentFile string

	command := &cobra.Command{
		Use:     "parse-combo",
		Short:   "Parse env/source/transform/sink sections",
		Long:    "Parse a full HOCON job into the four top-level sections used by combo save-as.",
		Example: "stx sync curated parse-combo --content-file ./job.conf",
		Args:    usageArgs(cobra.NoArgs),
		RunE: func(command *cobra.Command, _ []string) error {
			bodyContent, err := resolveCuratedContent(content, contentFile)
			if err != nil {
				return err
			}
			client, err := clientForNamespace(storeProvider, namespace)
			if err != nil {
				return err
			}
			body := map[string]any{"content": bodyContent}
			var data any
			operationID := "sync.curated.parse-combo"
			requestID, err := client.Request(command.Context(), http.MethodPost, "/api/v1/sync/curated-templates/parse-combo", body, &data)
			if err != nil {
				return err
			}
			return renderCommandResultWithRequestID(command, operationID, requestID, data)
		},
	}
	command.Flags().StringVar(&namespace, "namespace", "", "Local namespace to use")
	command.Flags().StringVar(&content, "content", "", "Inline HOCON content")
	command.Flags().StringVar(&contentFile, "content-file", "", "File containing HOCON content")
	return command
}

func resolveCuratedContent(content, contentFile string) (string, error) {
	filePath := strings.TrimSpace(contentFile)
	inline := strings.TrimSpace(content)
	switch {
	case filePath != "" && inline != "":
		return "", clioutput.NewError(clioutput.CodeUsage, "use either --content or --content-file, not both", clioutput.ExitUsage, false)
	case filePath != "":
		raw, err := os.ReadFile(filePath)
		if err != nil {
			return "", clioutput.NewError(clioutput.CodeUsage, fmt.Sprintf("read content file: %v", err), clioutput.ExitUsage, false)
		}
		text := strings.TrimSpace(string(raw))
		if text == "" {
			return "", clioutput.NewError(clioutput.CodeUsage, "content file is empty", clioutput.ExitUsage, false)
		}
		return text, nil
	case inline != "":
		return inline, nil
	default:
		return "", clioutput.NewError(clioutput.CodeUsage, "--content or --content-file is required", clioutput.ExitUsage, false)
	}
}

func parseTaskID(raw string) (uint, error) {
	parsed, err := strconv.ParseUint(strings.TrimSpace(raw), 10, 64)
	if err != nil || parsed == 0 {
		return 0, clioutput.NewError(clioutput.CodeUsage, fmt.Sprintf("invalid ID %q: must be a positive integer", raw), clioutput.ExitUsage, false)
	}
	return uint(parsed), nil
}
