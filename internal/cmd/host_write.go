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
				setK8sCredentialFlags(command, body)
			}
			return executeHostWrite(command, storeProvider, "host.create", &options, http.MethodPost, "/api/v1/hosts", body, "stx host list")
		},
	}
	addSecureWriteFlags(command, &options)
	addHostFields(command, &name, &description, &ipAddress, &sshPort, &dockerAPIURL, &dockerTLSEnabled, &dockerCertPath, &k8sAPIURL, &k8sNamespace)
	addK8sCredentialFlags(command)
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
				setK8sCredentialFlags(command, body)
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
	addK8sCredentialFlags(command)
	command.Flags().StringVar(&requestFile, "request-file", "", "JSON file containing the complete request body, including Kubernetes credentials when needed")
	return command
}

// setK8sCredentialFlags 把 K8s 凭据类参数写入请求体；kubeconfig/token 支持 @文件路径 读取，
// 避免长文本直接暴露在命令行与 shell 历史中。
// setK8sCredentialFlags writes K8s credential flags into the request body; kubeconfig/token
// support @file references so long secrets stay out of the command line and shell history.
func setK8sCredentialFlags(command *cobra.Command, body map[string]any) {
	setChangedString(command, body, "k8s-auth-mode", mustGetStringFlag(command, "k8s-auth-mode"))
	setChangedString(command, body, "k8s-ca-cert", resolveFileReferenceFlag(command, "k8s-ca-cert"))
	setChangedString(command, body, "k8s-kubeconfig", resolveFileReferenceFlag(command, "k8s-kubeconfig"))
	setChangedString(command, body, "k8s-token", resolveFileReferenceFlag(command, "k8s-token"))
	if command.Flags().Changed("k8s-insecure-skip-tls") {
		v, _ := command.Flags().GetBool("k8s-insecure-skip-tls")
		body["k8s_insecure_skip_tls"] = v
	}
}

func mustGetStringFlag(command *cobra.Command, name string) string {
	v, _ := command.Flags().GetString(name)
	return v
}

func resolveFileReference(value string) string {
	if !strings.HasPrefix(value, "@") || len(value) <= 1 {
		return value
	}
	data, err := os.ReadFile(strings.TrimPrefix(value, "@"))
	if err != nil {
		return value
	}
	return string(data)
}

// resolveFileReferenceFlag 解析 @路径 引用；非 @ 前缀按原文使用。
// resolveFileReferenceFlag resolves @path references; non-@ values pass through unchanged.
func resolveFileReferenceFlag(command *cobra.Command, name string) string {
	if !command.Flags().Changed(name) {
		return ""
	}
	v, _ := command.Flags().GetString(name)
	return resolveFileReference(v)
}

// addK8sCredentialFlags 登记 K8s 凭据类参数。
// addK8sCredentialFlags registers K8s credential flags.
func addK8sCredentialFlags(command *cobra.Command) {
	command.Flags().String("k8s-auth-mode", "", "Kubernetes auth mode: kubeconfig or token")
	command.Flags().String("k8s-kubeconfig", "", "Kubeconfig content or @path/to/kubeconfig")
	command.Flags().String("k8s-token", "", "Bearer token or @path/to/token")
	command.Flags().String("k8s-ca-cert", "", "CA certificate PEM or @path/to/ca.crt (token mode)")
	command.Flags().Bool("k8s-insecure-skip-tls", false, "Skip API server TLS verification (token mode, test only)")
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
