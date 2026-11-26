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
	"io"
	"net/http"
	"strings"

	clioutput "github.com/LeonYoah/stx/internal/cli/output"
	"github.com/spf13/cobra"
)

// containerRegistryFlags 仓库写命令的共用输入字段。
// containerRegistryFlags carries the shared input fields of registry write commands.
type containerRegistryFlags struct {
	host              string
	username          string
	password          string
	passwordStdin     bool
}

func addContainerRegistryFlags(command *cobra.Command, fields *containerRegistryFlags) {
	command.Flags().StringVar(&fields.host, "host", "", "Bare registry host[:port] without scheme")
	command.Flags().StringVar(&fields.username, "username", "", "Registry username; empty means anonymous")
	// --password 仅为兼容保留：明文密码会进入 shell 历史与进程列表，
	// 推荐 --password-stdin 或 TTY 隐藏输入。
	// --password stays only for compatibility: plaintext passwords leak into shell
	// history and process lists; prefer --password-stdin or the hidden TTY prompt.
	command.Flags().StringVar(&fields.password, "password", "", "Registry password or token (not recommended; prefer --password-stdin)")
	command.Flags().BoolVar(&fields.passwordStdin, "password-stdin", false, "Read the registry password from stdin")
}

func containerRegistryBody(fields *containerRegistryFlags) map[string]any {
	return map[string]any{
		"host":     fields.host,
		"username": fields.username,
		"password": fields.password,
	}
}

// newPackageRegistryCreateCommand stx package registry create。
// newPackageRegistryCreateCommand registers a container registry.
func newPackageRegistryCreateCommand(storeProvider authStoreProvider) *cobra.Command {
	var options packageWriteOptions
	var fields containerRegistryFlags
	command := &cobra.Command{
		Use:     "create",
		Short:   "Register a container registry for Kubernetes deploys",
		Example: "stx package registry create --host harbor.example.com --username robot --password-stdin --confirm",
		Args:    usageArgs(cobra.NoArgs),
		RunE: func(command *cobra.Command, args []string) error {
			client, headers, err := preparePackageWrite(command, storeProvider, "package.registry.create", &options,
				"registering a container registry affects credential materialization of later Kubernetes deploys and upgrades")
			if err != nil {
				return err
			}
			if err := resolveRegistryPassword(command, &fields, command.InOrStdin()); err != nil {
				return err
			}
			var data any
			requestID, err := client.RequestWithHeaders(command.Context(), http.MethodPost,
				"/api/v1/packages/registries", containerRegistryBody(&fields), headers, &data)
			if err != nil {
				return err
			}
			return renderPackageResult(command, "package.registry.create", requestID, data, "")
		},
	}
	addPackageWriteFlags(command, &options)
	addContainerRegistryFlags(command, &fields)
	return command
}

// newPackageRegistryUpdateCommand stx package registry update。
// newPackageRegistryUpdateCommand updates a registered container registry.
func newPackageRegistryUpdateCommand(storeProvider authStoreProvider) *cobra.Command {
	var options packageWriteOptions
	var fields containerRegistryFlags
	command := &cobra.Command{
		Use:     "update <id>",
		Short:   "Update a registered container registry",
		Example: "stx package registry update 2 --host harbor.example.com --username robot --password-stdin --confirm",
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			client, headers, err := preparePackageWrite(command, storeProvider, "package.registry.update", &options,
				"updating a container registry affects credential materialization of later Kubernetes deploys and upgrades")
			if err != nil {
				return err
			}
			if err := resolveRegistryPassword(command, &fields, command.InOrStdin()); err != nil {
				return err
			}
			var data any
			requestID, err := client.RequestWithHeaders(command.Context(), http.MethodPut,
				"/api/v1/packages/registries/"+args[0], containerRegistryBody(&fields), headers, &data)
			if err != nil {
				return err
			}
			return renderPackageResult(command, "package.registry.update", requestID, data, "")
		},
	}
	addPackageWriteFlags(command, &options)
	addContainerRegistryFlags(command, &fields)
	return command
}

// newPackageRegistryTestCommand stx package registry test。
// newPackageRegistryTestCommand probes connectivity and refreshes the catalog cache.
func newPackageRegistryTestCommand(storeProvider authStoreProvider) *cobra.Command {
	var options packageWriteOptions
	command := &cobra.Command{
		Use:     "test <id>",
		Short:   "Test registry connectivity and refresh the catalog cache",
		Example: "stx package registry test 2",
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			client, headers, err := preparePackageWrite(command, storeProvider, "package.registry.test", &options, "")
			if err != nil {
				return err
			}
			var data any
			requestID, err := client.RequestWithHeaders(command.Context(), http.MethodPost,
				"/api/v1/packages/registries/"+args[0]+"/test", map[string]any{}, headers, &data)
			if err != nil {
				return err
			}
			return renderPackageResult(command, "package.registry.test", requestID, data, "")
		},
	}
	addPackageWriteFlags(command, &options)
	return command
}

// resolveRegistryPassword 密码输入收口：stdin 优先、显式明文兼容、TTY 隐藏交互，
// 非 TTY 且无输入时明确报错而不是静默发空密码。
// resolveRegistryPassword funnels password input: stdin wins, explicit plaintext stays
// for compatibility, a TTY falls back to the hidden prompt, and non-TTY without input
// errors out explicitly instead of silently sending an empty password.
func resolveRegistryPassword(command *cobra.Command, fields *containerRegistryFlags, stdin io.Reader) error {
	if fields.password != "" && fields.passwordStdin {
		return clioutput.NewError(clioutput.CodeUsage, "--password cannot be combined with --password-stdin", clioutput.ExitUsage, false)
	}
	needsPassword := strings.TrimSpace(fields.username) != ""
	if fields.passwordStdin {
		content, err := io.ReadAll(stdin)
		if err != nil {
			return clioutput.WrapError(err, clioutput.CodeUsage, "read registry password from stdin", clioutput.ExitUsage, false)
		}
		password := strings.TrimRight(string(content), "\r\n")
		if password == "" {
			return clioutput.NewError(clioutput.CodeUsage, "password-stdin is empty", clioutput.ExitUsage, false)
		}
		fields.password = password
		return nil
	}
	if fields.password != "" || !needsPassword {
		return nil
	}
	if !isTerminalReader(stdin) {
		return clioutput.NewError(clioutput.CodeUsage, "registry credentials in non-interactive mode require --password-stdin", clioutput.ExitUsage, false)
	}
	password, err := readTerminalPassword(stdin, command.ErrOrStderr())
	if err != nil {
		return clioutput.WrapError(err, clioutput.CodeUsage, "read registry password", clioutput.ExitUsage, false)
	}
	if password == "" {
		return clioutput.NewError(clioutput.CodeUsage, "registry password is empty", clioutput.ExitUsage, false)
	}
	fields.password = password
	return nil
}
