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
	"github.com/spf13/cobra"
)

// addRuntimeStorageCommands 挂载需要 JSON 正文的运行时存储查询命令。
// addRuntimeStorageCommands attaches runtime storage queries that require JSON request bodies.
func addRuntimeStorageCommands(root *cobra.Command, storeProvider authStoreProvider) {
	clusterCommand := childCommand(root, "cluster")
	if clusterCommand == nil {
		panic("generated cluster command is missing")
	}
	runtimeStorageCommand := childCommand(clusterCommand, "runtime-storage")
	if runtimeStorageCommand == nil {
		panic("generated cluster runtime-storage command is missing")
	}
	runtimeStorageCommand.AddCommand(
		newClusterRuntimeStorageValidateCommand(storeProvider),
		newClusterRuntimeStorageListCommand(storeProvider),
		newClusterRuntimeStoragePreviewCommand(storeProvider),
	)
	checkpointCommand := &cobra.Command{Use: "checkpoint", Short: "Inspect checkpoint runtime storage", Args: usageArgs(cobra.NoArgs)}
	checkpointCommand.AddCommand(newClusterCheckpointInspectCommand(storeProvider))
	runtimeStorageCommand.AddCommand(checkpointCommand)
	imapCommand := &cobra.Command{Use: "imap", Short: "Inspect IMAP runtime storage", Args: usageArgs(cobra.NoArgs)}
	imapCommand.AddCommand(newClusterIMAPInspectCommand(storeProvider))
	runtimeStorageCommand.AddCommand(imapCommand)

	installerCommand := &cobra.Command{Use: "installer", Short: "Validate installation inputs", Args: usageArgs(cobra.NoArgs)}
	installerRuntimeStorage := &cobra.Command{Use: "runtime-storage", Short: "Validate runtime storage before installation", Args: usageArgs(cobra.NoArgs)}
	installerRuntimeStorage.AddCommand(newInstallerRuntimeStorageValidateCommand(storeProvider))
	installerCommand.AddCommand(installerRuntimeStorage)
	root.AddCommand(installerCommand)
}

func newClusterRuntimeStorageValidateCommand(storeProvider authStoreProvider) *cobra.Command {
	var namespace string
	command := &cobra.Command{
		Use:     "validate <cluster-id> <kind>",
		Short:   "Validate configured checkpoint or IMAP storage",
		Example: "stx cluster runtime-storage validate 6 checkpoint",
		Args:    usageArgs(cobra.ExactArgs(2)),
		RunE: func(command *cobra.Command, args []string) error {
			kind, err := normalizeRuntimeStorageKind(args[1])
			if err != nil {
				return err
			}
			path := "/api/v1/clusters/" + url.PathEscape(args[0]) + "/runtime-storage/" + kind + "/validate"
			return executeRuntimeStorageQuery(command, storeProvider, namespace, "cluster.runtime-storage.validate", path, nil)
		},
	}
	command.Flags().StringVar(&namespace, "namespace", "", "Local namespace to use")
	return command
}

func newClusterRuntimeStorageListCommand(storeProvider authStoreProvider) *cobra.Command {
	var namespace, requestFile, storagePath string
	var recursive bool
	var limit int
	command := &cobra.Command{
		Use:     "list <cluster-id> <kind>",
		Short:   "List files in checkpoint or IMAP storage",
		Example: "stx cluster runtime-storage list 6 checkpoint --path jobs --recursive --limit 100",
		Args:    usageArgs(cobra.ExactArgs(2)),
		RunE: func(command *cobra.Command, args []string) error {
			kind, err := normalizeRuntimeStorageKind(args[1])
			if err != nil {
				return err
			}
			body, err := requestBodyFromFile(requestFile)
			if err != nil {
				return err
			}
			if body == nil {
				body = make(map[string]any)
				setChangedString(command, body, "path", storagePath)
				setChangedBool(command, body, "recursive", recursive)
				setChangedInt(command, body, "limit", limit)
			}
			path := "/api/v1/clusters/" + url.PathEscape(args[0]) + "/runtime-storage/" + kind + "/list"
			return executeRuntimeStorageQuery(command, storeProvider, namespace, "cluster.runtime-storage.list", path, body)
		},
	}
	command.Flags().StringVar(&namespace, "namespace", "", "Local namespace to use")
	command.Flags().StringVar(&storagePath, "path", "", "Storage path to list")
	command.Flags().BoolVar(&recursive, "recursive", false, "List files recursively")
	command.Flags().IntVar(&limit, "limit", 0, "Maximum number of entries")
	command.Flags().StringVar(&requestFile, "request-file", "", "JSON file containing the complete request body")
	return command
}

func newClusterRuntimeStoragePreviewCommand(storeProvider authStoreProvider) *cobra.Command {
	var namespace, requestFile, storagePath string
	var maxBytes int
	command := &cobra.Command{
		Use:     "preview <cluster-id> <kind>",
		Short:   "Read a bounded preview from one runtime storage file",
		Long:    "Read a bounded preview from one runtime storage file. Use the path returned by the list command.",
		Example: "stx cluster runtime-storage preview 6 checkpoint --path /tmp/seatunnel/checkpoint/checkpoint.dat --max-bytes 65536",
		Args:    usageArgs(cobra.ExactArgs(2)),
		RunE: func(command *cobra.Command, args []string) error {
			kind, err := normalizeRuntimeStorageKind(args[1])
			if err != nil {
				return err
			}
			body, err := runtimeStoragePathBody(command, requestFile, storagePath)
			if err != nil {
				return err
			}
			if strings.TrimSpace(requestFile) == "" {
				setChangedInt(command, body, "max-bytes", maxBytes)
			}
			path := "/api/v1/clusters/" + url.PathEscape(args[0]) + "/runtime-storage/" + kind + "/preview"
			return executeRuntimeStorageQuery(command, storeProvider, namespace, "cluster.runtime-storage.preview", path, body)
		},
	}
	command.Flags().StringVar(&namespace, "namespace", "", "Local namespace to use")
	command.Flags().StringVar(&storagePath, "path", "", "Storage file path")
	command.Flags().IntVar(&maxBytes, "max-bytes", 0, "Maximum number of bytes to preview")
	command.Flags().StringVar(&requestFile, "request-file", "", "JSON file containing the complete request body")
	return command
}

func newClusterCheckpointInspectCommand(storeProvider authStoreProvider) *cobra.Command {
	var namespace, requestFile, storagePath, jobConfigFile string
	command := &cobra.Command{
		Use:     "inspect <cluster-id>",
		Short:   "Deserialize and inspect one checkpoint file",
		Long:    "Deserialize and inspect one checkpoint file. Use the path returned by the list command.",
		Example: "stx cluster runtime-storage checkpoint inspect 6 --path /tmp/seatunnel/checkpoint/checkpoint.dat",
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			body, err := runtimeStoragePathBody(command, requestFile, storagePath)
			if err != nil {
				return err
			}
			if strings.TrimSpace(requestFile) == "" {
				if err := setJSONFileValue(body, "job_config", jobConfigFile); err != nil {
					return err
				}
			}
			path := "/api/v1/clusters/" + url.PathEscape(args[0]) + "/runtime-storage/checkpoint/inspect"
			return executeRuntimeStorageQuery(command, storeProvider, namespace, "cluster.runtime-storage.checkpoint.inspect", path, body)
		},
	}
	command.Flags().StringVar(&namespace, "namespace", "", "Local namespace to use")
	command.Flags().StringVar(&storagePath, "path", "", "Checkpoint file path")
	command.Flags().StringVar(&jobConfigFile, "job-config-file", "", "JSON file containing content, content_format, and variables")
	command.Flags().StringVar(&requestFile, "request-file", "", "JSON file containing the complete request body")
	return command
}

func newClusterIMAPInspectCommand(storeProvider authStoreProvider) *cobra.Command {
	var namespace, requestFile, storagePath string
	command := &cobra.Command{
		Use:     "inspect <cluster-id>",
		Short:   "Parse and inspect one IMAP WAL file",
		Long:    "Parse and inspect one IMAP WAL file. Use the path returned by the list command.",
		Example: "stx cluster runtime-storage imap inspect 6 --path /tmp/seatunnel/imap/imap.wal",
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			body, err := runtimeStoragePathBody(command, requestFile, storagePath)
			if err != nil {
				return err
			}
			path := "/api/v1/clusters/" + url.PathEscape(args[0]) + "/runtime-storage/imap/inspect"
			return executeRuntimeStorageQuery(command, storeProvider, namespace, "cluster.runtime-storage.imap.inspect", path, body)
		},
	}
	command.Flags().StringVar(&namespace, "namespace", "", "Local namespace to use")
	command.Flags().StringVar(&storagePath, "path", "", "IMAP WAL file path")
	command.Flags().StringVar(&requestFile, "request-file", "", "JSON file containing the complete request body")
	return command
}

func newInstallerRuntimeStorageValidateCommand(storeProvider authStoreProvider) *cobra.Command {
	var namespace, requestFile, kind, checkpointFile, imapFile string
	var hostIDs []uint
	command := &cobra.Command{
		Use:     "validate",
		Short:   "Validate runtime storage from selected hosts",
		Example: "stx installer runtime-storage validate --host-id 10 --kind checkpoint --checkpoint-file checkpoint.json",
		Args:    usageArgs(cobra.NoArgs),
		RunE: func(command *cobra.Command, _ []string) error {
			body, err := requestBodyFromFile(requestFile)
			if err != nil {
				return err
			}
			if body == nil {
				normalizedKind, kindErr := normalizeRuntimeStorageKind(kind)
				if kindErr != nil {
					return kindErr
				}
				if len(hostIDs) == 0 {
					return clioutput.NewError(clioutput.CodeUsage, "at least one --host-id is required", clioutput.ExitUsage, false)
				}
				body = map[string]any{"host_ids": hostIDs, "kind": normalizedKind}
				configFile := checkpointFile
				if normalizedKind == "imap" {
					configFile = imapFile
				}
				if strings.TrimSpace(configFile) == "" {
					return clioutput.NewError(clioutput.CodeUsage, "--"+normalizedKind+"-file is required", clioutput.ExitUsage, false)
				}
				if err := setJSONFileValue(body, normalizedKind, configFile); err != nil {
					return err
				}
			}
			return executeRuntimeStorageQuery(command, storeProvider, namespace, "installer.runtime-storage.validate", "/api/v1/installer/runtime-storage/validate", body)
		},
	}
	command.Flags().StringVar(&namespace, "namespace", "", "Local namespace to use")
	command.Flags().UintSliceVar(&hostIDs, "host-id", nil, "Host ID; repeat the flag for multiple hosts")
	command.Flags().StringVar(&kind, "kind", "", "Runtime storage kind: checkpoint or imap")
	command.Flags().StringVar(&checkpointFile, "checkpoint-file", "", "JSON file containing checkpoint storage configuration")
	command.Flags().StringVar(&imapFile, "imap-file", "", "JSON file containing IMAP storage configuration")
	command.Flags().StringVar(&requestFile, "request-file", "", "JSON file containing the complete request body")
	return command
}

// runtimeStoragePathBody 读取完整正文，或根据 --path 构建最小正文。
// runtimeStoragePathBody reads a complete request body or builds the minimum body from --path.
func runtimeStoragePathBody(command *cobra.Command, requestFile, storagePath string) (map[string]any, error) {
	body, err := requestBodyFromFile(requestFile)
	if err != nil {
		return nil, err
	}
	if body != nil {
		return body, nil
	}
	if strings.TrimSpace(storagePath) == "" {
		return nil, clioutput.NewError(clioutput.CodeUsage, "--path or --request-file is required", clioutput.ExitUsage, false)
	}
	body = make(map[string]any)
	setChangedString(command, body, "path", storagePath)
	return body, nil
}

// normalizeRuntimeStorageKind 只接受服务端支持的两种公开存储类型。
// normalizeRuntimeStorageKind accepts only the two public storage kinds supported by the server.
func normalizeRuntimeStorageKind(kind string) (string, error) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind != "checkpoint" && kind != "imap" {
		return "", clioutput.NewError(clioutput.CodeUsage, "runtime storage kind must be checkpoint or imap", clioutput.ExitUsage, false)
	}
	return kind, nil
}

// executeRuntimeStorageQuery 先检查服务端能力，再执行带正文的 R0 查询。
// executeRuntimeStorageQuery checks server capability before executing an R0 query with a JSON body.
func executeRuntimeStorageQuery(command *cobra.Command, storeProvider authStoreProvider, namespace, operationID, path string, body any) error {
	client, err := clientForNamespace(storeProvider, namespace)
	if err != nil {
		return err
	}
	if err := checkSpecialOperation(command, client, operationID); err != nil {
		return err
	}
	var data any
	requestID, err := client.Request(command.Context(), http.MethodPost, path, body, &data)
	if err != nil {
		return err
	}
	return renderCommandResultWithRequestID(command, operationID, requestID, data)
}
