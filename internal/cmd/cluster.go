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
	"net/url"
	"os"
	"strings"

	clioutput "github.com/LeonYoah/stx/internal/cli/output"
	"github.com/LeonYoah/stx/internal/operation"
	"github.com/spf13/cobra"
)

// addClusterWriteCommands 将复杂 JSON 正文的集群写命令挂到生成命令树。
// addClusterWriteCommands attaches cluster writes with complex JSON bodies to the generated command tree.
func addClusterWriteCommands(root *cobra.Command, storeProvider authStoreProvider) {
	clusterCommand := childCommand(root, "cluster")
	if clusterCommand == nil {
		panic("generated cluster command is missing")
	}
	clusterCommand.AddCommand(newClusterCreateCommand(storeProvider), newClusterUpdateCommand(storeProvider))

	nodeCommand := childCommand(clusterCommand, "node")
	if nodeCommand == nil {
		panic("generated cluster node command is missing")
	}
	nodeCommand.AddCommand(
		newClusterNodeAddCommand(storeProvider),
		newClusterNodeAddBatchCommand(storeProvider),
		newClusterNodeUpdateCommand(storeProvider),
		newClusterNodePrecheckCommand(storeProvider),
	)
}

func newClusterCreateCommand(storeProvider authStoreProvider) *cobra.Command {
	var options secureWriteOptions
	var name, description, deploymentMode, version, installDir, requestFile, configFile, nodesFile string
	command := &cobra.Command{
		Use:     "create",
		Short:   "Create a cluster",
		Long:    "Create a cluster definition. This does not start SeaTunnel processes automatically.",
		Example: "stx cluster create --name demo --deployment-mode hybrid --version 2.3.13 --confirm",
		Args:    usageArgs(cobra.NoArgs),
		RunE: func(command *cobra.Command, _ []string) error {
			body, err := requestBodyFromFile(requestFile)
			if err != nil {
				return err
			}
			if body == nil {
				if strings.TrimSpace(name) == "" || strings.TrimSpace(deploymentMode) == "" || strings.TrimSpace(version) == "" {
					return clioutput.NewError(clioutput.CodeUsage, "--name, --deployment-mode, and --version are required", clioutput.ExitUsage, false)
				}
				body = map[string]any{"name": strings.TrimSpace(name), "deployment_mode": strings.TrimSpace(deploymentMode), "version": strings.TrimSpace(version)}
				setChangedString(command, body, "description", description)
				setChangedString(command, body, "install-dir", installDir)
				if err := setJSONFileValue(body, "config", configFile); err != nil {
					return err
				}
				if err := setJSONFileValue(body, "nodes", nodesFile); err != nil {
					return err
				}
			}
			return executeClusterWrite(command, storeProvider, "cluster.create", &options, http.MethodPost, "/api/v1/clusters", body)
		},
	}
	addSecureWriteFlags(command, &options)
	command.Flags().StringVar(&name, "name", "", "Cluster name")
	command.Flags().StringVar(&description, "description", "", "Cluster description")
	command.Flags().StringVar(&deploymentMode, "deployment-mode", "", "Deployment mode: separate or hybrid")
	command.Flags().StringVar(&version, "version", "", "SeaTunnel version")
	command.Flags().StringVar(&installDir, "install-dir", "", "Default SeaTunnel installation directory")
	command.Flags().StringVar(&configFile, "config-file", "", "JSON file containing the cluster config object")
	command.Flags().StringVar(&nodesFile, "nodes-file", "", "JSON file containing discovery node entries")
	command.Flags().StringVar(&requestFile, "request-file", "", "JSON file containing the complete request body")
	return command
}

func newClusterUpdateCommand(storeProvider authStoreProvider) *cobra.Command {
	var options secureWriteOptions
	var name, description, version, installDir, requestFile, configFile string
	command := &cobra.Command{
		Use:     "update <id>",
		Short:   "Update a cluster",
		Example: "stx cluster update 6 --description test --confirm",
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			body, err := requestBodyFromFile(requestFile)
			if err != nil {
				return err
			}
			if body == nil {
				body = make(map[string]any)
				setChangedString(command, body, "name", name)
				setChangedString(command, body, "description", description)
				setChangedString(command, body, "version", version)
				setChangedString(command, body, "install-dir", installDir)
				if err := setJSONFileValue(body, "config", configFile); err != nil {
					return err
				}
				if len(body) == 0 {
					return clioutput.NewError(clioutput.CodeUsage, "at least one update flag or --request-file is required", clioutput.ExitUsage, false)
				}
			}
			path := "/api/v1/clusters/" + url.PathEscape(args[0])
			return executeClusterWrite(command, storeProvider, "cluster.update", &options, http.MethodPut, path, body)
		},
	}
	addSecureWriteFlags(command, &options)
	command.Flags().StringVar(&name, "name", "", "Cluster name")
	command.Flags().StringVar(&description, "description", "", "Cluster description")
	command.Flags().StringVar(&version, "version", "", "SeaTunnel version")
	command.Flags().StringVar(&installDir, "install-dir", "", "Default SeaTunnel installation directory")
	command.Flags().StringVar(&configFile, "config-file", "", "JSON file containing the cluster config object")
	command.Flags().StringVar(&requestFile, "request-file", "", "JSON file containing the complete request body")
	return command
}

func newClusterNodeAddCommand(storeProvider authStoreProvider) *cobra.Command {
	var options secureWriteOptions
	var hostID uint
	var role, installDir, overridesFile, requestFile string
	var hazelcastPort, apiPort, workerPort int
	var skipPrecheck bool
	command := &cobra.Command{
		Use:     "add <cluster-id>",
		Short:   "Add a cluster node",
		Example: "stx cluster node add 6 --host-id 10 --role master/worker --skip-precheck --confirm",
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			body, err := requestBodyFromFile(requestFile)
			if err != nil {
				return err
			}
			if body == nil {
				if hostID == 0 || strings.TrimSpace(role) == "" {
					return clioutput.NewError(clioutput.CodeUsage, "--host-id and --role are required", clioutput.ExitUsage, false)
				}
				body = map[string]any{"host_id": hostID, "role": strings.TrimSpace(role), "skip_precheck": skipPrecheck}
				setChangedString(command, body, "install-dir", installDir)
				setChangedInt(command, body, "hazelcast-port", hazelcastPort)
				setChangedInt(command, body, "api-port", apiPort)
				setChangedInt(command, body, "worker-port", workerPort)
				if err := setJSONFileValue(body, "overrides", overridesFile); err != nil {
					return err
				}
			}
			path := fmt.Sprintf("/api/v1/clusters/%s/nodes", url.PathEscape(args[0]))
			return executeClusterWrite(command, storeProvider, "cluster.node.add", &options, http.MethodPost, path, body)
		},
	}
	addSecureWriteFlags(command, &options)
	addNodeDefinitionFlags(command, &hostID, &role, &installDir, &hazelcastPort, &apiPort, &workerPort)
	command.Flags().BoolVar(&skipPrecheck, "skip-precheck", false, "Skip the remote node precheck")
	command.Flags().StringVar(&overridesFile, "overrides-file", "", "JSON file containing node overrides")
	command.Flags().StringVar(&requestFile, "request-file", "", "JSON file containing the complete request body")
	return command
}

func newClusterNodeAddBatchCommand(storeProvider authStoreProvider) *cobra.Command {
	var options secureWriteOptions
	var hostID uint
	var installDir, entriesFile, requestFile string
	var skipPrecheck bool
	command := &cobra.Command{
		Use:     "add-batch <cluster-id>",
		Short:   "Add multiple logical nodes for one host",
		Example: "stx cluster node add-batch 6 --host-id 10 --entries-file nodes.json --confirm",
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			body, err := requestBodyFromFile(requestFile)
			if err != nil {
				return err
			}
			if body == nil {
				if hostID == 0 || strings.TrimSpace(entriesFile) == "" {
					return clioutput.NewError(clioutput.CodeUsage, "--host-id and --entries-file are required", clioutput.ExitUsage, false)
				}
				body = map[string]any{"host_id": hostID, "skip_precheck": skipPrecheck}
				setChangedString(command, body, "install-dir", installDir)
				if err := setJSONFileValue(body, "entries", entriesFile); err != nil {
					return err
				}
			}
			path := fmt.Sprintf("/api/v1/clusters/%s/nodes/batch", url.PathEscape(args[0]))
			return executeClusterWrite(command, storeProvider, "cluster.node.add-batch", &options, http.MethodPost, path, body)
		},
	}
	addSecureWriteFlags(command, &options)
	command.Flags().UintVar(&hostID, "host-id", 0, "Host ID")
	command.Flags().StringVar(&installDir, "install-dir", "", "Default node installation directory")
	command.Flags().StringVar(&entriesFile, "entries-file", "", "JSON file containing the node entry array")
	command.Flags().BoolVar(&skipPrecheck, "skip-precheck", false, "Skip the remote node precheck")
	command.Flags().StringVar(&requestFile, "request-file", "", "JSON file containing the complete request body")
	return command
}

func newClusterNodeUpdateCommand(storeProvider authStoreProvider) *cobra.Command {
	var options secureWriteOptions
	var installDir, overridesFile, requestFile string
	var hazelcastPort, apiPort, workerPort int
	command := &cobra.Command{
		Use:     "update <cluster-id> <node-id>",
		Short:   "Update a cluster node",
		Example: "stx cluster node update 6 1 --hazelcast-port 5801 --confirm",
		Args:    usageArgs(cobra.ExactArgs(2)),
		RunE: func(command *cobra.Command, args []string) error {
			body, err := requestBodyFromFile(requestFile)
			if err != nil {
				return err
			}
			if body == nil {
				body = make(map[string]any)
				setChangedString(command, body, "install-dir", installDir)
				setChangedInt(command, body, "hazelcast-port", hazelcastPort)
				setChangedInt(command, body, "api-port", apiPort)
				setChangedInt(command, body, "worker-port", workerPort)
				if err := setJSONFileValue(body, "overrides", overridesFile); err != nil {
					return err
				}
				if len(body) == 0 {
					return clioutput.NewError(clioutput.CodeUsage, "at least one update flag or --request-file is required", clioutput.ExitUsage, false)
				}
			}
			path := fmt.Sprintf("/api/v1/clusters/%s/nodes/%s", url.PathEscape(args[0]), url.PathEscape(args[1]))
			return executeClusterWrite(command, storeProvider, "cluster.node.update", &options, http.MethodPut, path, body)
		},
	}
	addSecureWriteFlags(command, &options)
	command.Flags().StringVar(&installDir, "install-dir", "", "SeaTunnel installation directory")
	command.Flags().IntVar(&hazelcastPort, "hazelcast-port", 0, "Hazelcast port")
	command.Flags().IntVar(&apiPort, "api-port", 0, "REST API port")
	command.Flags().IntVar(&workerPort, "worker-port", 0, "Worker Hazelcast port")
	command.Flags().StringVar(&overridesFile, "overrides-file", "", "JSON file containing node overrides")
	command.Flags().StringVar(&requestFile, "request-file", "", "JSON file containing the complete request body")
	return command
}

func newClusterNodePrecheckCommand(storeProvider authStoreProvider) *cobra.Command {
	var options secureWriteOptions
	var hostID uint
	var role, installDir, requestFile string
	var hazelcastPort, apiPort, workerPort int
	command := &cobra.Command{
		Use:     "precheck <cluster-id>",
		Short:   "Precheck a cluster node",
		Example: "stx cluster node precheck 6 --host-id 10 --role master/worker --hazelcast-port 5801 --confirm",
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			body, err := requestBodyFromFile(requestFile)
			if err != nil {
				return err
			}
			if body == nil {
				if hostID == 0 || strings.TrimSpace(role) == "" || hazelcastPort <= 0 {
					return clioutput.NewError(clioutput.CodeUsage, "--host-id, --role, and --hazelcast-port are required", clioutput.ExitUsage, false)
				}
				body = map[string]any{"host_id": hostID, "role": strings.TrimSpace(role), "hazelcast_port": hazelcastPort}
				setChangedString(command, body, "install-dir", installDir)
				setChangedInt(command, body, "api-port", apiPort)
				setChangedInt(command, body, "worker-port", workerPort)
			}
			path := fmt.Sprintf("/api/v1/clusters/%s/nodes/precheck", url.PathEscape(args[0]))
			return executeClusterWrite(command, storeProvider, "cluster.node.precheck", &options, http.MethodPost, path, body)
		},
	}
	addSecureWriteFlags(command, &options)
	addNodeDefinitionFlags(command, &hostID, &role, &installDir, &hazelcastPort, &apiPort, &workerPort)
	command.Flags().StringVar(&requestFile, "request-file", "", "JSON file containing the complete request body")
	return command
}

// executeClusterWrite 通过统一确认和结果格式执行集群写请求。
// executeClusterWrite runs a cluster write through common confirmation and result rendering.
func executeClusterWrite(command *cobra.Command, storeProvider authStoreProvider, operationID string, options *secureWriteOptions, method, path string, body map[string]any) error {
	client, headers, err := prepareSecureWrite(command, storeProvider, operationID, options, clusterOperationImpact(operationID))
	if err != nil {
		return err
	}
	var data any
	requestID, err := client.RequestWithHeaders(command.Context(), method, path, body, headers, &data)
	if err != nil {
		return handleSecureWriteError(command, operationID, err)
	}
	return renderWriteResult(command, operationID, requestID, data, "")
}

func clusterOperationImpact(operationID string) string {
	for _, spec := range operation.Registry() {
		if spec.ID == operationID && spec.Impact != nil {
			return spec.Impact.Message
		}
	}
	return "this operation changes cluster state"
}

func addNodeDefinitionFlags(command *cobra.Command, hostID *uint, role, installDir *string, hazelcastPort, apiPort, workerPort *int) {
	command.Flags().UintVar(hostID, "host-id", 0, "Host ID")
	command.Flags().StringVar(role, "role", "", "Node role: master, worker, or master/worker")
	command.Flags().StringVar(installDir, "install-dir", "", "SeaTunnel installation directory")
	command.Flags().IntVar(hazelcastPort, "hazelcast-port", 0, "Hazelcast port")
	command.Flags().IntVar(apiPort, "api-port", 0, "REST API port")
	command.Flags().IntVar(workerPort, "worker-port", 0, "Worker Hazelcast port")
}

func requestBodyFromFile(path string) (map[string]any, error) {
	if strings.TrimSpace(path) == "" {
		return nil, nil
	}
	var body map[string]any
	if err := readJSONFile(path, &body); err != nil {
		return nil, err
	}
	if body == nil {
		return nil, clioutput.NewError(clioutput.CodeUsage, "request file must contain a JSON object", clioutput.ExitUsage, false)
	}
	return body, nil
}

func setJSONFileValue(body map[string]any, field, path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	var value any
	if err := readJSONFile(path, &value); err != nil {
		return err
	}
	body[field] = value
	return nil
}

func readJSONFile(path string, target any) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return clioutput.WrapError(err, clioutput.CodeUsage, "read JSON file", clioutput.ExitUsage, false)
	}
	if err := json.Unmarshal(content, target); err != nil {
		return clioutput.WrapError(err, clioutput.CodeUsage, "decode JSON file", clioutput.ExitUsage, false)
	}
	return nil
}

func setChangedString(command *cobra.Command, body map[string]any, flagName, value string) {
	if command.Flags().Changed(flagName) {
		body[strings.ReplaceAll(flagName, "-", "_")] = strings.TrimSpace(value)
	}
}

func setChangedInt(command *cobra.Command, body map[string]any, flagName string, value int) {
	if command.Flags().Changed(flagName) {
		body[strings.ReplaceAll(flagName, "-", "_")] = value
	}
}
