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
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	clioutput "github.com/LeonYoah/stx/internal/cli/output"
	"github.com/spf13/cobra"
)

// addDiagnosticsWriteCommands 将需要专用正文处理的诊断命令挂到生成的查询命令树。
// addDiagnosticsWriteCommands attaches diagnostics commands that need dedicated request-body handling.
func addDiagnosticsWriteCommands(root *cobra.Command, storeProvider authStoreProvider) {
	diagnosticsCommand := childCommand(root, "diagnostics")
	if diagnosticsCommand == nil {
		panic("generated diagnostics command is missing")
	}
	taskCommand := childCommand(diagnosticsCommand, "task")
	if taskCommand == nil {
		panic("generated diagnostics task command is missing")
	}
	taskCommand.AddCommand(
		newDiagnosticsTaskCreateCommand(storeProvider),
		newDiagnosticsTaskDownloadCommand(storeProvider, diagnosticsTaskDownloadOptions{
			use: "bundle <task-id>", short: "Download a diagnostic task bundle", operationID: "diagnostics.task.bundle.download",
			path: func(taskID string, _ []string) string {
				return "/api/v1/diagnostics/tasks/" + url.PathEscape(taskID) + "/bundle"
			},
			defaultFile: func(taskID string, _ []string) string { return "diagnostics-" + taskID + ".zip" },
		}),
		newDiagnosticsTaskDownloadCommand(storeProvider, diagnosticsTaskDownloadOptions{
			use: "html <task-id>", short: "Download a diagnostic task HTML report", operationID: "diagnostics.task.html.download",
			path: func(taskID string, _ []string) string {
				return "/api/v1/diagnostics/tasks/" + url.PathEscape(taskID) + "/html"
			},
			defaultFile: func(taskID string, _ []string) string { return "diagnostics-" + taskID + ".html" },
		}),
		newDiagnosticsTaskDownloadCommand(storeProvider, diagnosticsTaskDownloadOptions{
			use: "file <task-id> <artifact-path>", short: "Download one diagnostic task file", operationID: "diagnostics.task.file.download", exactArgs: 2,
			path: func(taskID string, args []string) string {
				return "/api/v1/diagnostics/tasks/" + url.PathEscape(taskID) + "/files/" + escapeArtifactPath(args[1])
			},
			defaultFile: func(_ string, args []string) string { return filepath.Base(strings.TrimSpace(args[1])) },
		}),
	)
}

type diagnosticsTaskDownloadOptions struct {
	use         string
	short       string
	operationID string
	exactArgs   int
	path        func(taskID string, args []string) string
	defaultFile func(taskID string, args []string) string
}

// newDiagnosticsTaskDownloadCommand 创建诊断资源下载命令，并用临时文件避免留下半成品。
// newDiagnosticsTaskDownloadCommand creates a diagnostics download command that uses a temporary file to avoid partial outputs.
func newDiagnosticsTaskDownloadCommand(storeProvider authStoreProvider, commandOptions diagnosticsTaskDownloadOptions) *cobra.Command {
	var namespace, outputPath string
	exactArgs := commandOptions.exactArgs
	if exactArgs == 0 {
		exactArgs = 1
	}
	command := &cobra.Command{
		Use:   commandOptions.use,
		Short: commandOptions.short,
		Args:  usageArgs(cobra.ExactArgs(exactArgs)),
		RunE: func(command *cobra.Command, args []string) error {
			if exactArgs == 2 {
				if err := validateArtifactPath(args[1]); err != nil {
					return err
				}
			}
			client, err := clientForNamespace(storeProvider, namespace)
			if err != nil {
				return err
			}
			if err := checkSpecialOperation(command.Context(), client, commandOptions.operationID); err != nil {
				return err
			}
			if strings.TrimSpace(outputPath) == "" {
				outputPath = commandOptions.defaultFile(args[0], args)
			}
			if strings.TrimSpace(outputPath) == "" || outputPath == "." {
				return clioutput.NewError(clioutput.CodeUsage, "download destination file is required", clioutput.ExitUsage, false)
			}
			finalPath, err := filepath.Abs(outputPath)
			if err != nil {
				return clioutput.WrapError(err, clioutput.CodeUsage, "resolve output path", clioutput.ExitUsage, false)
			}
			if _, err := os.Stat(finalPath); err == nil {
				return clioutput.NewError(clioutput.CodeConflict, "download target already exists", clioutput.ExitConflict, false)
			} else if !os.IsNotExist(err) {
				return clioutput.WrapError(err, clioutput.CodeFileTransfer, "inspect download target", clioutput.ExitFileTransfer, false)
			}
			if err := os.MkdirAll(filepath.Dir(finalPath), 0o755); err != nil {
				return clioutput.WrapError(err, clioutput.CodeFileTransfer, "create download directory", clioutput.ExitFileTransfer, false)
			}
			tempFile, err := os.CreateTemp(filepath.Dir(finalPath), ".stx-diagnostics-*.part")
			if err != nil {
				return clioutput.WrapError(err, clioutput.CodeFileTransfer, "create temporary download file", clioutput.ExitFileTransfer, false)
			}
			tempPath := tempFile.Name()
			defer func() { _ = os.Remove(tempPath) }()
			requestID, err := client.Download(command.Context(), commandOptions.path(args[0], args), tempFile)
			if closeErr := tempFile.Close(); err == nil && closeErr != nil {
				err = closeErr
			}
			if err != nil {
				return err
			}
			if err := os.Rename(tempPath, finalPath); err != nil {
				return clioutput.WrapError(err, clioutput.CodeFileTransfer, "store downloaded diagnostics file", clioutput.ExitFileTransfer, false)
			}
			checksum, size, err := checksumFile(finalPath)
			if err != nil {
				return err
			}
			return renderCommandResultWithRequestID(command, commandOptions.operationID, requestID, map[string]any{
				"task_id": args[0], "file": finalPath, "size": size, "sha256": checksum,
			})
		},
	}
	command.Flags().StringVar(&namespace, "namespace", "", "Local namespace to use")
	command.Flags().StringVar(&outputPath, "file", "", "Destination file path")
	return command
}

// escapeArtifactPath 保留诊断资源的路径层级，同时逐段转义特殊字符。
// escapeArtifactPath preserves the diagnostics artifact hierarchy while escaping every path segment.
func escapeArtifactPath(path string) string {
	parts := strings.Split(strings.Trim(strings.TrimSpace(path), "/"), "/")
	escaped := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		escaped = append(escaped, url.PathEscape(part))
	}
	return strings.Join(escaped, "/")
}

// validateArtifactPath 拒绝空路径和目录回退片段，避免请求含义不明确。
// validateArtifactPath rejects empty paths and parent-directory segments to keep the request unambiguous.
func validateArtifactPath(path string) error {
	trimmed := strings.Trim(strings.TrimSpace(path), "/")
	if trimmed == "" {
		return clioutput.NewError(clioutput.CodeUsage, "artifact path is required", clioutput.ExitUsage, false)
	}
	for _, part := range strings.Split(trimmed, "/") {
		if part == "" || part == "." || part == ".." {
			return clioutput.NewError(clioutput.CodeUsage, "artifact path must not contain empty, '.' or '..' segments", clioutput.ExitUsage, false)
		}
	}
	return nil
}

// newDiagnosticsTaskCreateCommand 创建诊断任务，默认只保存任务而不启动采集。
// newDiagnosticsTaskCreateCommand creates a diagnostics task without starting collection by default.
func newDiagnosticsTaskCreateCommand(storeProvider authStoreProvider) *cobra.Command {
	var options secureWriteOptions
	var requestFile string
	var clusterID uint
	var triggerSource, nodeScope string
	var selectedNodeIDs []uint
	var errorGroupID, inspectionReportID, inspectionFindingID uint
	var alertID string
	var includeThreadDump, includeJVMDump, autoStart bool
	var jvmDumpMinFreeMB, lookbackMinutes int
	var summary string

	command := &cobra.Command{
		Use:     "create",
		Short:   "Create a diagnostic task",
		Long:    "Create a diagnostic task. It stays ready by default; use --auto-start with --confirm to start collection.",
		Example: "stx diagnostics task create --cluster-id 6 --summary 'check recent errors' --confirm",
		Args:    usageArgs(cobra.NoArgs),
		RunE: func(command *cobra.Command, _ []string) error {
			body, err := requestBodyFromFile(requestFile)
			if err != nil {
				return err
			}
			if body == nil {
				body = make(map[string]any)
			}
			if command.Flags().Changed("cluster-id") {
				body["cluster_id"] = clusterID
			}
			if command.Flags().Changed("trigger-source") {
				body["trigger_source"] = strings.TrimSpace(triggerSource)
			}
			if command.Flags().Changed("node-scope") {
				body["node_scope"] = strings.TrimSpace(nodeScope)
			}
			if command.Flags().Changed("selected-node-id") {
				body["selected_node_ids"] = selectedNodeIDs
			}
			if command.Flags().Changed("lookback-minutes") {
				body["lookback_minutes"] = lookbackMinutes
			}
			if command.Flags().Changed("summary") {
				body["summary"] = summary
			}
			if command.Flags().Changed("auto-start") || requestFile == "" {
				body["auto_start"] = autoStart
			}
			if err := setDiagnosticsSourceRefFlags(command, body, errorGroupID, inspectionReportID, inspectionFindingID, alertID); err != nil {
				return err
			}
			if err := setDiagnosticsOptionsFlags(command, body, includeThreadDump, includeJVMDump, jvmDumpMinFreeMB); err != nil {
				return err
			}
			if len(body) == 0 {
				return clioutput.NewError(clioutput.CodeUsage, "diagnostic task request is empty; provide --cluster-id or --request-file", clioutput.ExitUsage, false)
			}
			if requestFile == "" && !command.Flags().Changed("cluster-id") && triggerSource == "" {
				return clioutput.NewError(clioutput.CodeUsage, "--cluster-id or --request-file is required", clioutput.ExitUsage, false)
			}
			return executeDiagnosticsTaskCreate(command, storeProvider, &options, body)
		},
	}
	addSecureWriteFlags(command, &options)
	command.Flags().StringVar(&requestFile, "request-file", "", "JSON file containing the complete request body")
	command.Flags().UintVar(&clusterID, "cluster-id", 0, "SeaTunnel cluster ID")
	command.Flags().StringVar(&triggerSource, "trigger-source", "", "Task source: manual, error_group, inspection_finding, or alert")
	command.Flags().StringVar(&nodeScope, "node-scope", "", "Node scope: all, related, or custom")
	command.Flags().UintSliceVar(&selectedNodeIDs, "selected-node-id", nil, "Selected node ID; may be repeated")
	command.Flags().UintVar(&errorGroupID, "error-group-id", 0, "Related error group ID")
	command.Flags().UintVar(&inspectionReportID, "inspection-report-id", 0, "Related inspection report ID")
	command.Flags().UintVar(&inspectionFindingID, "inspection-finding-id", 0, "Related inspection finding ID")
	command.Flags().StringVar(&alertID, "alert-id", "", "Related alert ID")
	command.Flags().BoolVar(&includeThreadDump, "include-thread-dump", false, "Include thread dump collection")
	command.Flags().BoolVar(&includeJVMDump, "include-jvm-dump", false, "Include JVM dump collection; the server restricts this to administrators")
	command.Flags().IntVar(&jvmDumpMinFreeMB, "jvm-dump-min-free-mb", 0, "Minimum free memory required for JVM dump collection")
	command.Flags().IntVar(&lookbackMinutes, "lookback-minutes", 0, "Recent time window to inspect, in minutes")
	command.Flags().StringVar(&summary, "summary", "", "Task summary")
	command.Flags().BoolVar(&autoStart, "auto-start", false, "Start collection immediately after task creation")
	return command
}

// setDiagnosticsSourceRefFlags 将来源快捷参数合并到 request body 的 source_ref。
// setDiagnosticsSourceRefFlags merges source-reference shortcut flags into the request body.
func setDiagnosticsSourceRefFlags(command *cobra.Command, body map[string]any, errorGroupID, inspectionReportID, inspectionFindingID uint, alertID string) error {
	if !command.Flags().Changed("error-group-id") && !command.Flags().Changed("inspection-report-id") &&
		!command.Flags().Changed("inspection-finding-id") && !command.Flags().Changed("alert-id") {
		return nil
	}
	sourceRef := make(map[string]any)
	if existing, ok := body["source_ref"].(map[string]any); ok {
		for key, value := range existing {
			sourceRef[key] = value
		}
	}
	if command.Flags().Changed("error-group-id") {
		sourceRef["error_group_id"] = errorGroupID
	}
	if command.Flags().Changed("inspection-report-id") {
		sourceRef["inspection_report_id"] = inspectionReportID
	}
	if command.Flags().Changed("inspection-finding-id") {
		sourceRef["inspection_finding_id"] = inspectionFindingID
	}
	if command.Flags().Changed("alert-id") {
		sourceRef["alert_id"] = strings.TrimSpace(alertID)
	}
	body["source_ref"] = sourceRef
	return nil
}

// setDiagnosticsOptionsFlags 将采集选项快捷参数合并到 request body 的 options。
// setDiagnosticsOptionsFlags merges collection shortcut flags into the request body's options.
func setDiagnosticsOptionsFlags(command *cobra.Command, body map[string]any, includeThreadDump, includeJVMDump bool, jvmDumpMinFreeMB int) error {
	if !command.Flags().Changed("include-thread-dump") && !command.Flags().Changed("include-jvm-dump") && !command.Flags().Changed("jvm-dump-min-free-mb") {
		return nil
	}
	options := make(map[string]any)
	if existing, ok := body["options"].(map[string]any); ok {
		for key, value := range existing {
			options[key] = value
		}
	}
	if command.Flags().Changed("include-thread-dump") {
		options["include_thread_dump"] = includeThreadDump
	}
	if command.Flags().Changed("include-jvm-dump") {
		options["include_jvm_dump"] = includeJVMDump
	}
	if command.Flags().Changed("jvm-dump-min-free-mb") {
		options["jvm_dump_min_free_mb"] = jvmDumpMinFreeMB
	}
	body["options"] = options
	return nil
}

// executeDiagnosticsTaskCreate 使用统一确认、幂等和结果格式提交诊断任务。
// executeDiagnosticsTaskCreate submits a diagnostics task with shared confirmation, idempotency, and output handling.
func executeDiagnosticsTaskCreate(command *cobra.Command, storeProvider authStoreProvider, options *secureWriteOptions, body map[string]any) error {
	const operationID = "diagnostics.task.create"
	client, headers, err := prepareSecureWrite(command, storeProvider, operationID, options,
		"默认只创建诊断任务；启动后会读取集群资源，JVM Dump 可能影响目标 Java 进程。")
	if err != nil {
		return err
	}
	var data any
	requestID, err := client.RequestWithHeaders(command.Context(), http.MethodPost, "/api/v1/diagnostics/tasks", body, headers, &data)
	if err != nil {
		return handleSecureWriteError(command, operationID, err)
	}
	return renderWriteResult(command, operationID, requestID, data, diagnosticsTaskNextCommand(data))
}

func diagnosticsTaskNextCommand(data any) string {
	item, ok := data.(map[string]any)
	if !ok {
		return "stx diagnostics task list"
	}
	id, ok := item["id"]
	if !ok {
		return "stx diagnostics task list"
	}
	return "stx diagnostics task get " + strings.TrimSpace(taskIDString(id))
}

func taskIDString(value any) string {
	switch item := value.(type) {
	case string:
		return item
	case json.Number:
		return item.String()
	case float64:
		return strconv.FormatFloat(item, 'f', -1, 64)
	default:
		return ""
	}
}
