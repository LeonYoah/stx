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
	"os"
	"strings"

	clioutput "github.com/LeonYoah/stx/internal/cli/output"
	"github.com/LeonYoah/stx/internal/operation"
	"github.com/spf13/cobra"
)

// addConfigWriteCommands 将配置写命令挂到生成的 config 命令树。
// addConfigWriteCommands attaches config write commands to the generated config command tree.
func addConfigWriteCommands(root *cobra.Command, storeProvider authStoreProvider) {
	configCommand := childCommand(root, "config")
	if configCommand == nil {
		panic("generated config command is missing")
	}
	configCommand.AddCommand(newConfigNormalizeCommand(storeProvider), newConfigUpdateCommand(storeProvider), newConfigRollbackCommand(storeProvider), newConfigCommentCommand(storeProvider, "promote", "config.promote"), newConfigCommentCommand(storeProvider, "sync", "config.sync"), newConfigPushCommand(storeProvider))
	clusterCommand := childCommand(configCommand, "cluster")
	if clusterCommand == nil {
		panic("generated config cluster command is missing")
	}
	clusterCommand.AddCommand(newConfigClusterInitCommand(storeProvider), newConfigClusterSyncAllCommand(storeProvider))
}

func newConfigNormalizeCommand(storeProvider authStoreProvider) *cobra.Command {
	var namespace, requestFile, configType, contentFile string
	command := &cobra.Command{Use: "normalize", Short: "Normalize SeaTunnel configuration content", Example: "stx config normalize --config-type hazelcast-master.yaml --content-file ./hazelcast-master.yaml", Args: usageArgs(cobra.NoArgs), RunE: func(command *cobra.Command, _ []string) error {
		body, err := requestBodyFromFile(requestFile)
		if err != nil {
			return err
		}
		if body == nil {
			if strings.TrimSpace(configType) == "" || strings.TrimSpace(contentFile) == "" {
				return clioutput.NewError(clioutput.CodeUsage, "--config-type and --content-file are required", clioutput.ExitUsage, false)
			}
			content, err := readConfigContentFile(contentFile)
			if err != nil {
				return err
			}
			body = map[string]any{"config_type": strings.TrimSpace(configType), "content": content}
		}
		client, err := clientForNamespace(storeProvider, namespace)
		if err != nil {
			return err
		}
		if err := checkSpecialOperation(command.Context(), client, "config.normalize"); err != nil {
			return err
		}
		var data any
		requestID, err := client.Request(command.Context(), http.MethodPost, "/api/v1/configs/normalize", body, &data)
		if err != nil {
			return err
		}
		return renderWriteResult(command, "config.normalize", requestID, data, "")
	}}
	command.Flags().StringVar(&namespace, "namespace", "", "Local namespace to use")
	command.Flags().StringVar(&configType, "config-type", "", "Configuration type")
	command.Flags().StringVar(&contentFile, "content-file", "", "File containing raw configuration content")
	command.Flags().StringVar(&requestFile, "request-file", "", "JSON file containing the complete request body")
	return command
}

func newConfigUpdateCommand(storeProvider authStoreProvider) *cobra.Command {
	var options secureWriteOptions
	var requestFile, contentFile, comment string
	command := &cobra.Command{Use: "update <config-id>", Short: "Update one configuration", Example: "stx config update 1 --content-file ./seatunnel.yaml --confirm", Args: usageArgs(cobra.ExactArgs(1)), RunE: func(command *cobra.Command, args []string) error {
		body, err := requestBodyFromFile(requestFile)
		if err != nil {
			return err
		}
		if body == nil {
			if strings.TrimSpace(contentFile) == "" {
				return clioutput.NewError(clioutput.CodeUsage, "--content-file or --request-file is required", clioutput.ExitUsage, false)
			}
			content, err := readConfigContentFile(contentFile)
			if err != nil {
				return err
			}
			body = map[string]any{"content": content}
			setChangedString(command, body, "comment", comment)
		}
		return executeConfigWrite(command, storeProvider, "config.update", &options, http.MethodPut, "/api/v1/configs/"+url.PathEscape(args[0]), body, "stx config get "+args[0])
	}}
	addSecureWriteFlags(command, &options)
	command.Flags().StringVar(&contentFile, "content-file", "", "File containing raw configuration content")
	command.Flags().StringVar(&comment, "comment", "", "Version comment")
	command.Flags().StringVar(&requestFile, "request-file", "", "JSON file containing the complete request body")
	return command
}

func newConfigRollbackCommand(storeProvider authStoreProvider) *cobra.Command {
	var options secureWriteOptions
	var requestFile, comment string
	var version int
	command := &cobra.Command{Use: "rollback <config-id>", Short: "Rollback one configuration", Example: "stx config rollback 1 --version 2 --confirm", Args: usageArgs(cobra.ExactArgs(1)), RunE: func(command *cobra.Command, args []string) error {
		body, err := requestBodyFromFile(requestFile)
		if err != nil {
			return err
		}
		if body == nil {
			if !command.Flags().Changed("version") || version <= 0 {
				return clioutput.NewError(clioutput.CodeUsage, "--version must be greater than zero", clioutput.ExitUsage, false)
			}
			body = map[string]any{"version": version}
			setChangedString(command, body, "comment", comment)
		}
		return executeConfigWrite(command, storeProvider, "config.rollback", &options, http.MethodPost, "/api/v1/configs/"+url.PathEscape(args[0])+"/rollback", body, "stx config get "+args[0])
	}}
	addSecureWriteFlags(command, &options)
	command.Flags().IntVar(&version, "version", 0, "Historical version number")
	command.Flags().StringVar(&comment, "comment", "", "Rollback comment")
	command.Flags().StringVar(&requestFile, "request-file", "", "JSON file containing the complete request body")
	return command
}

func newConfigCommentCommand(storeProvider authStoreProvider, action, operationID string) *cobra.Command {
	var options secureWriteOptions
	var requestFile, comment string
	command := &cobra.Command{Use: action + " <config-id>", Short: map[string]string{"promote": "Promote a node configuration to the cluster template", "sync": "Sync one node configuration from the cluster template"}[action], Example: "stx config " + action + " 2 --confirm", Args: usageArgs(cobra.ExactArgs(1)), RunE: func(command *cobra.Command, args []string) error {
		body, err := requestBodyFromFile(requestFile)
		if err != nil {
			return err
		}
		if body == nil {
			body = make(map[string]any)
			setChangedString(command, body, "comment", comment)
		}
		return executeConfigWrite(command, storeProvider, operationID, &options, http.MethodPost, "/api/v1/configs/"+url.PathEscape(args[0])+"/"+action, body, "stx config get "+args[0])
	}}
	addSecureWriteFlags(command, &options)
	command.Flags().StringVar(&comment, "comment", "", "Operation comment")
	command.Flags().StringVar(&requestFile, "request-file", "", "JSON file containing the complete request body")
	return command
}

func newConfigPushCommand(storeProvider authStoreProvider) *cobra.Command {
	var options secureWriteOptions
	var requestFile, installDir string
	command := &cobra.Command{Use: "push <config-id>", Short: "Push one configuration to its node", Example: "stx config push 2 --install-dir /tmp/seatunnel-2.3.13 --confirm", Args: usageArgs(cobra.ExactArgs(1)), RunE: func(command *cobra.Command, args []string) error {
		body, err := requestBodyFromFile(requestFile)
		if err != nil {
			return err
		}
		if body == nil {
			if strings.TrimSpace(installDir) == "" {
				return clioutput.NewError(clioutput.CodeUsage, "--install-dir is required", clioutput.ExitUsage, false)
			}
			body = map[string]any{"install_dir": strings.TrimSpace(installDir)}
		}
		return executeConfigWrite(command, storeProvider, "config.push", &options, http.MethodPost, "/api/v1/configs/"+url.PathEscape(args[0])+"/push", body, "stx config get "+args[0])
	}}
	addSecureWriteFlags(command, &options)
	command.Flags().StringVar(&installDir, "install-dir", "", "SeaTunnel installation directory")
	command.Flags().StringVar(&requestFile, "request-file", "", "JSON file containing the complete request body")
	return command
}

func newConfigClusterInitCommand(storeProvider authStoreProvider) *cobra.Command {
	var options secureWriteOptions
	var requestFile, installDir string
	var hostID uint
	command := &cobra.Command{Use: "init <cluster-id>", Short: "Initialize cluster configurations from one node", Example: "stx config cluster init 6 --host-id 10 --install-dir /tmp/seatunnel-2.3.13 --confirm", Args: usageArgs(cobra.ExactArgs(1)), RunE: func(command *cobra.Command, args []string) error {
		body, err := requestBodyFromFile(requestFile)
		if err != nil {
			return err
		}
		if body == nil {
			if hostID == 0 || strings.TrimSpace(installDir) == "" {
				return clioutput.NewError(clioutput.CodeUsage, "--host-id and --install-dir are required", clioutput.ExitUsage, false)
			}
			body = map[string]any{"host_id": hostID, "install_dir": strings.TrimSpace(installDir)}
		}
		return executeConfigWrite(command, storeProvider, "config.cluster.init", &options, http.MethodPost, "/api/v1/clusters/"+url.PathEscape(args[0])+"/configs/init", body, "stx config cluster list "+args[0])
	}}
	addSecureWriteFlags(command, &options)
	command.Flags().UintVar(&hostID, "host-id", 0, "Host ID")
	command.Flags().StringVar(&installDir, "install-dir", "", "SeaTunnel installation directory")
	command.Flags().StringVar(&requestFile, "request-file", "", "JSON file containing the complete request body")
	return command
}

func newConfigClusterSyncAllCommand(storeProvider authStoreProvider) *cobra.Command {
	var options secureWriteOptions
	var requestFile, configType string
	command := &cobra.Command{Use: "sync-all <cluster-id>", Short: "Sync one cluster template to all nodes", Example: "stx config cluster sync-all 6 --config-type hazelcast-master.yaml --confirm", Args: usageArgs(cobra.ExactArgs(1)), RunE: func(command *cobra.Command, args []string) error {
		body, err := requestBodyFromFile(requestFile)
		if err != nil {
			return err
		}
		if body == nil {
			if strings.TrimSpace(configType) == "" {
				return clioutput.NewError(clioutput.CodeUsage, "--config-type is required", clioutput.ExitUsage, false)
			}
			body = map[string]any{"config_type": strings.TrimSpace(configType)}
		}
		return executeConfigWrite(command, storeProvider, "config.cluster.sync-all", &options, http.MethodPost, "/api/v1/clusters/"+url.PathEscape(args[0])+"/configs/sync-all", body, "stx config cluster list "+args[0])
	}}
	addSecureWriteFlags(command, &options)
	command.Flags().StringVar(&configType, "config-type", "", "Configuration type")
	command.Flags().StringVar(&requestFile, "request-file", "", "JSON file containing the complete request body")
	return command
}

// executeConfigWrite 使用公共确认、能力检查和结果格式执行配置写请求。
// executeConfigWrite executes a config write using shared confirmation, capability checks, and result rendering.
func executeConfigWrite(command *cobra.Command, storeProvider authStoreProvider, operationID string, options *secureWriteOptions, method, path string, body map[string]any, nextCommand string) error {
	client, headers, err := prepareSecureWrite(command, storeProvider, operationID, options, configOperationImpact(operationID))
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

func configOperationImpact(operationID string) string {
	for _, spec := range operation.Registry() {
		if spec.ID == operationID && spec.Impact != nil {
			return spec.Impact.Message
		}
	}
	return "this operation changes SeaTunnel configuration"
}

// readConfigContentFile 读取原始配置文本，不解析 HOCON 或 YAML。
// readConfigContentFile reads raw config text without parsing HOCON or YAML.
func readConfigContentFile(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", clioutput.WrapError(err, clioutput.CodeUsage, "read configuration content file", clioutput.ExitUsage, false)
	}
	return string(content), nil
}
