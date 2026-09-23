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

// addHostWriteCommands 将主机新增和修改命令挂到现有主机命令树。
// addHostWriteCommands attaches host create and update commands to the existing host command tree.
func addHostWriteCommands(root *cobra.Command, storeProvider authStoreProvider) {
	hostCommand := childCommand(root, "host")
	if hostCommand == nil {
		panic("generated host command is missing")
	}
	hostCommand.AddCommand(newHostCreateCommand(storeProvider), newHostUpdateCommand(storeProvider))
}

func newHostCreateCommand(storeProvider authStoreProvider) *cobra.Command {
	var options secureWriteOptions
	var name, hostType, description, ipAddress, dockerAPIURL, dockerCertPath, k8sAPIURL, k8sNamespace, requestFile string
	var sshPort int
	var dockerTLSEnabled bool
	command := &cobra.Command{
		Use:     "create",
		Short:   "Create a host",
		Example: "stx host create --name node-2 --ip-address 192.0.2.20 --confirm",
		Args:    usageArgs(cobra.NoArgs),
		RunE: func(command *cobra.Command, _ []string) error {
			body, err := requestBodyFromFile(requestFile)
			if err != nil {
				return err
			}
			if body == nil {
				if strings.TrimSpace(name) == "" {
					return clioutput.NewError(clioutput.CodeUsage, "--name is required", clioutput.ExitUsage, false)
				}
				body = map[string]any{"name": strings.TrimSpace(name)}
				setChangedString(command, body, "host-type", hostType)
				setChangedString(command, body, "description", description)
				setChangedString(command, body, "ip-address", ipAddress)
				setChangedInt(command, body, "ssh-port", sshPort)
				setChangedString(command, body, "docker-api-url", dockerAPIURL)
				setChangedBool(command, body, "docker-tls-enabled", dockerTLSEnabled)
				setChangedString(command, body, "docker-cert-path", dockerCertPath)
				setChangedString(command, body, "k8s-api-url", k8sAPIURL)
				setChangedString(command, body, "k8s-namespace", k8sNamespace)
			}
			return executeHostWrite(command, storeProvider, "host.create", &options, http.MethodPost, "/api/v1/hosts", body, "stx host list")
		},
	}
	addSecureWriteFlags(command, &options)
	addHostFields(command, &name, &description, &ipAddress, &sshPort, &dockerAPIURL, &dockerTLSEnabled, &dockerCertPath, &k8sAPIURL, &k8sNamespace)
	command.Flags().StringVar(&hostType, "host-type", "", "Host type: bare_metal, docker, or kubernetes")
	command.Flags().StringVar(&requestFile, "request-file", "", "JSON file containing the complete request body, including Kubernetes credentials when needed")
	return command
}

func newHostUpdateCommand(storeProvider authStoreProvider) *cobra.Command {
	var options secureWriteOptions
	var name, description, ipAddress, dockerAPIURL, dockerCertPath, k8sAPIURL, k8sNamespace, requestFile string
	var sshPort int
	var dockerTLSEnabled bool
	command := &cobra.Command{
		Use:     "update <id>",
		Short:   "Update a host",
		Example: "stx host update 10 --description test-node --confirm",
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
				setChangedString(command, body, "ip-address", ipAddress)
				setChangedInt(command, body, "ssh-port", sshPort)
				setChangedString(command, body, "docker-api-url", dockerAPIURL)
				setChangedBool(command, body, "docker-tls-enabled", dockerTLSEnabled)
				setChangedString(command, body, "docker-cert-path", dockerCertPath)
				setChangedString(command, body, "k8s-api-url", k8sAPIURL)
				setChangedString(command, body, "k8s-namespace", k8sNamespace)
				if len(body) == 0 {
					return clioutput.NewError(clioutput.CodeUsage, "at least one update flag or --request-file is required", clioutput.ExitUsage, false)
				}
			}
			path := "/api/v1/hosts/" + url.PathEscape(args[0])
			return executeHostWrite(command, storeProvider, "host.update", &options, http.MethodPut, path, body, "stx host get "+args[0])
		},
	}
	addSecureWriteFlags(command, &options)
	addHostFields(command, &name, &description, &ipAddress, &sshPort, &dockerAPIURL, &dockerTLSEnabled, &dockerCertPath, &k8sAPIURL, &k8sNamespace)
	command.Flags().StringVar(&requestFile, "request-file", "", "JSON file containing the complete request body, including Kubernetes credentials when needed")
	return command
}

// addHostFields 登记主机新增和修改共用的非敏感参数。
// addHostFields registers non-sensitive flags shared by host create and update commands.
func addHostFields(command *cobra.Command, name, description, ipAddress *string, sshPort *int, dockerAPIURL *string, dockerTLSEnabled *bool, dockerCertPath, k8sAPIURL, k8sNamespace *string) {
	command.Flags().StringVar(name, "name", "", "Host name")
	command.Flags().StringVar(description, "description", "", "Host description")
	command.Flags().StringVar(ipAddress, "ip-address", "", "Bare-metal host IP address")
	command.Flags().IntVar(sshPort, "ssh-port", 0, "Bare-metal SSH port")
	command.Flags().StringVar(dockerAPIURL, "docker-api-url", "", "Docker API URL")
	command.Flags().BoolVar(dockerTLSEnabled, "docker-tls-enabled", false, "Enable Docker TLS")
	command.Flags().StringVar(dockerCertPath, "docker-cert-path", "", "Docker certificate path")
	command.Flags().StringVar(k8sAPIURL, "k8s-api-url", "", "Kubernetes API URL")
	command.Flags().StringVar(k8sNamespace, "k8s-namespace", "", "Kubernetes namespace")
}

// executeHostWrite 使用公共确认、能力检查和结果格式执行主机写请求。
// executeHostWrite executes a host write using shared confirmation, capability checks, and result rendering.
func executeHostWrite(command *cobra.Command, storeProvider authStoreProvider, operationID string, options *secureWriteOptions, method, path string, body map[string]any, nextCommand string) error {
	client, headers, err := prepareSecureWrite(command, storeProvider, operationID, options, hostOperationImpact(operationID))
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
