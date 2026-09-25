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
	"net/url"
	"strings"

	clioutput "github.com/LeonYoah/stx/internal/cli/output"
	"github.com/LeonYoah/stx/internal/operation"
	"github.com/spf13/cobra"
)

// addHostInstallCommands 将主机预检和安装写命令挂到生成的 host 命令树。
// addHostInstallCommands attaches host precheck and installation writes to the generated host tree.
func addHostInstallCommands(root *cobra.Command, storeProvider authStoreProvider) {
	hostCommand := childCommand(root, "host")
	if hostCommand == nil {
		panic("generated host command is missing")
	}
	hostCommand.AddCommand(newHostPrecheckCommand(storeProvider))
	installCommand := childCommand(hostCommand, "install")
	if installCommand == nil {
		panic("generated host install command is missing")
	}
	installCommand.AddCommand(
		newHostInstallStartCommand(storeProvider),
		newHostInstallRetryCommand(storeProvider),
		newHostInstallCancelCommand(storeProvider),
	)
}

func newHostPrecheckCommand(storeProvider authStoreProvider) *cobra.Command {
	var namespace, installDir, requestFile string
	var minMemoryMB, minDiskSpaceMB int64
	var minCPUCores int
	var ports []int
	command := &cobra.Command{
		Use:     "precheck <host-id>",
		Short:   "Run installation precheck on a host",
		Long:    hostOperationDescription("host.precheck"),
		Example: "stx host precheck 10 --install-dir /tmp/seatunnel-2.3.12 --port 15812 --port 18092",
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			body, err := requestBodyFromFile(requestFile)
			if err != nil {
				return err
			}
			if body == nil {
				body = make(map[string]any)
			}
			setChangedString(command, body, "install-dir", installDir)
			if command.Flags().Changed("min-memory-mb") {
				body["min_memory_mb"] = minMemoryMB
			}
			if command.Flags().Changed("min-cpu-cores") {
				body["min_cpu_cores"] = minCPUCores
			}
			if command.Flags().Changed("min-disk-space-mb") {
				body["min_disk_space_mb"] = minDiskSpaceMB
			}
			if command.Flags().Changed("port") {
				body["ports"] = ports
			}
			client, err := clientForNamespace(storeProvider, namespace)
			if err != nil {
				return err
			}
			if err := checkSpecialOperation(command, client, "host.precheck"); err != nil {
				return err
			}
			var data any
			path := "/api/v1/hosts/" + url.PathEscape(args[0]) + "/precheck"
			requestID, err := client.Request(command.Context(), http.MethodPost, path, body, &data)
			if err != nil {
				return err
			}
			return renderWriteResult(command, "host.precheck", requestID, data, "")
		},
	}
	command.Flags().StringVar(&namespace, "namespace", "", "Local namespace to use")
	command.Flags().StringVar(&installDir, "install-dir", "", "SeaTunnel installation directory to check")
	command.Flags().Int64Var(&minMemoryMB, "min-memory-mb", 0, "Minimum available memory in MiB")
	command.Flags().IntVar(&minCPUCores, "min-cpu-cores", 0, "Minimum CPU core count")
	command.Flags().Int64Var(&minDiskSpaceMB, "min-disk-space-mb", 0, "Minimum available disk space in MiB")
	command.Flags().IntSliceVar(&ports, "port", nil, "Port that must be available; may be specified more than once")
	command.Flags().StringVar(&requestFile, "request-file", "", "JSON file containing the complete request body")
	return command
}

func newHostInstallStartCommand(storeProvider authStoreProvider) *cobra.Command {
	var options secureWriteOptions
	var version, installDir, installMode, mirror, packagePath, deploymentMode, nodeRole, clusterID, requestFile string
	var masterAddresses, workerAddresses []string
	var clusterPort, workerPort, httpPort, javaProxyPort int
	var enableHTTP bool
	command := &cobra.Command{
		Use:     "start <host-id>",
		Short:   "Install SeaTunnel on a host",
		Long:    hostOperationDescription("host.install.start"),
		Example: "stx host install start 10 --version 2.3.12 --install-dir /tmp/seatunnel-2.3.12 --deployment-mode hybrid --node-role master/worker --cluster-port 15812 --http-port 18092 --confirm",
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			body, err := requestBodyFromFile(requestFile)
			if err != nil {
				return err
			}
			if body == nil {
				body = make(map[string]any)
			}
			setChangedString(command, body, "version", version)
			setChangedString(command, body, "install-dir", installDir)
			setChangedString(command, body, "install-mode", installMode)
			setChangedString(command, body, "mirror", mirror)
			setChangedString(command, body, "package-path", packagePath)
			setChangedString(command, body, "deployment-mode", deploymentMode)
			setChangedString(command, body, "node-role", nodeRole)
			setChangedString(command, body, "cluster-id", clusterID)
			setDefaultString(body, "install_mode", installMode)
			setDefaultString(body, "deployment_mode", deploymentMode)
			setDefaultString(body, "node_role", nodeRole)
			setChangedInt(command, body, "cluster-port", clusterPort)
			setChangedInt(command, body, "worker-port", workerPort)
			setChangedInt(command, body, "http-port", httpPort)
			setChangedInt(command, body, "java-proxy-port", javaProxyPort)
			if command.Flags().Changed("master-address") {
				body["master_addresses"] = masterAddresses
			}
			if command.Flags().Changed("worker-address") {
				body["worker_addresses"] = workerAddresses
			}
			if command.Flags().Changed("enable-http") {
				body["enable_http"] = enableHTTP
			}
			if strings.TrimSpace(stringValue(body["version"])) == "" {
				return clioutput.NewError(clioutput.CodeUsage, "--version or version in --request-file is required", clioutput.ExitUsage, false)
			}
			path := "/api/v1/hosts/" + url.PathEscape(args[0]) + "/install"
			next := fmt.Sprintf("stx host install status get %s", args[0])
			return executeHostInstallWrite(command, storeProvider, "host.install.start", &options, path, body, next)
		},
	}
	addSecureWriteFlags(command, &options)
	command.Flags().StringVar(&version, "version", "", "SeaTunnel version")
	command.Flags().StringVar(&installDir, "install-dir", "", "SeaTunnel installation directory")
	command.Flags().StringVar(&installMode, "install-mode", "online", "Installation mode: online or offline")
	command.Flags().StringVar(&mirror, "mirror", "", "Download mirror: apache, aliyun, or huaweicloud")
	command.Flags().StringVar(&packagePath, "package-path", "", "Local package path for offline mode")
	command.Flags().StringVar(&deploymentMode, "deployment-mode", "hybrid", "Deployment mode: hybrid or separated")
	command.Flags().StringVar(&nodeRole, "node-role", "master/worker", "Node role: master, worker, or master/worker")
	command.Flags().StringVar(&clusterID, "cluster-id", "", "Cluster ID to join after installation")
	command.Flags().StringSliceVar(&masterAddresses, "master-address", nil, "Master address; may be specified more than once")
	command.Flags().StringSliceVar(&workerAddresses, "worker-address", nil, "Worker address; may be specified more than once")
	command.Flags().IntVar(&clusterPort, "cluster-port", 0, "Master or hybrid Hazelcast port")
	command.Flags().IntVar(&workerPort, "worker-port", 0, "Worker Hazelcast port")
	command.Flags().IntVar(&httpPort, "http-port", 0, "SeaTunnel HTTP API port")
	command.Flags().IntVar(&javaProxyPort, "java-proxy-port", 0, "Managed stx-java-proxy port")
	command.Flags().BoolVar(&enableHTTP, "enable-http", false, "Enable the SeaTunnel HTTP API")
	command.Flags().StringVar(&requestFile, "request-file", "", "JSON file containing the complete installation request")
	return command
}

func newHostInstallRetryCommand(storeProvider authStoreProvider) *cobra.Command {
	var options secureWriteOptions
	var step string
	command := &cobra.Command{
		Use:     "retry <host-id>",
		Short:   "Retry a failed installation step",
		Long:    hostOperationDescription("host.install.retry"),
		Example: "stx host install retry 10 --step extract --confirm",
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			if strings.TrimSpace(step) == "" {
				return clioutput.NewError(clioutput.CodeUsage, "--step is required", clioutput.ExitUsage, false)
			}
			path := "/api/v1/hosts/" + url.PathEscape(args[0]) + "/install/retry"
			return executeHostInstallWrite(command, storeProvider, "host.install.retry", &options, path, map[string]any{"step": strings.TrimSpace(step)}, fmt.Sprintf("stx host install status get %s", args[0]))
		},
	}
	addSecureWriteFlags(command, &options)
	command.Flags().StringVar(&step, "step", "", "Failed installation step name")
	return command
}

func newHostInstallCancelCommand(storeProvider authStoreProvider) *cobra.Command {
	var options secureWriteOptions
	command := &cobra.Command{
		Use:     "cancel <host-id>",
		Short:   "Request installation cancellation",
		Long:    hostOperationDescription("host.install.cancel"),
		Example: "stx host install cancel 10 --confirm",
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			path := "/api/v1/hosts/" + url.PathEscape(args[0]) + "/install/cancel"
			return executeHostInstallWrite(command, storeProvider, "host.install.cancel", &options, path, nil, fmt.Sprintf("stx host install status get %s", args[0]))
		},
	}
	addSecureWriteFlags(command, &options)
	return command
}

// executeHostInstallWrite 通过统一确认和结果格式执行主机安装写请求。
// executeHostInstallWrite runs a host-install write through common confirmation and result rendering.
func executeHostInstallWrite(command *cobra.Command, storeProvider authStoreProvider, operationID string, options *secureWriteOptions, path string, body map[string]any, nextCommand string) error {
	client, headers, err := prepareSecureWrite(command, storeProvider, operationID, options, hostOperationImpact(operationID))
	if err != nil {
		return err
	}
	var data any
	requestID, err := client.RequestWithHeaders(command.Context(), http.MethodPost, path, body, headers, &data)
	if err != nil {
		return handleSecureWriteError(command, operationID, err)
	}
	return renderWriteResult(command, operationID, requestID, data, nextCommand)
}

func hostOperationImpact(operationID string) string {
	for _, spec := range operation.Registry() {
		if spec.ID == operationID && spec.Impact != nil {
			return spec.Impact.Message
		}
	}
	return "this operation changes host installation state"
}

func hostOperationDescription(operationID string) string {
	for _, spec := range operation.Registry() {
		if spec.ID == operationID {
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
	}
	return "Run registered operation " + operationID + "."
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}

// setDefaultString 只在请求正文缺少字段时写入 CLI 默认值。
// setDefaultString writes a CLI default only when the request body omits the field.
func setDefaultString(body map[string]any, field, value string) {
	if strings.TrimSpace(stringValue(body[field])) == "" && strings.TrimSpace(value) != "" {
		body[field] = strings.TrimSpace(value)
	}
}
