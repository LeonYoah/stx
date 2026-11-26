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

// containerImageFlags 镜像写命令的共用输入字段。
// containerImageFlags carries the shared input fields of image write commands.
type containerImageFlags struct {
	name             string
	image            string
	seatunnelVersion string
	pullPolicy       string
	pullSecrets      string
	isDefault        bool
	description      string
	registryHost          string
	registryUsername      string
	registryPassword      string
	registryPasswordStdin bool
}

func addContainerImageFlags(command *cobra.Command, fields *containerImageFlags) {
	command.Flags().StringVar(&fields.name, "name", "", "Image display name")
	command.Flags().StringVar(&fields.image, "image", "", "Full image reference with tag or digest")
	command.Flags().StringVar(&fields.seatunnelVersion, "seatunnel-version", "", "SeaTunnel version the image carries")
	command.Flags().StringVar(&fields.pullPolicy, "image-pull-policy", "", "Always, IfNotPresent or Never")
	command.Flags().StringVar(&fields.pullSecrets, "image-pull-secrets", "", "Comma separated pull secret names")
	command.Flags().BoolVar(&fields.isDefault, "default", false, "Mark as default image")
	command.Flags().StringVar(&fields.description, "description", "", "Description")
	command.Flags().StringVar(&fields.registryHost, "registry-host", "", "Private registry host for credential materialization")
	command.Flags().StringVar(&fields.registryUsername, "registry-username", "", "Private registry username")
	// --registry-password 仅为兼容保留：明文密码会进入 shell 历史与进程列表，
	// 推荐 --registry-password-stdin 或 TTY 隐藏输入。
	// --registry-password stays only for compatibility: plaintext passwords leak into
	// shell history and process lists; prefer --registry-password-stdin or the hidden
	// TTY prompt.
	command.Flags().StringVar(&fields.registryPassword, "registry-password", "", "Private registry password or token (not recommended; prefer --registry-password-stdin)")
	command.Flags().BoolVar(&fields.registryPasswordStdin, "registry-password-stdin", false, "Read the registry password from stdin")
}

func containerImageBody(fields *containerImageFlags) map[string]any {
	return map[string]any{
		"name":               fields.name,
		"image":              fields.image,
		"seatunnel_version":  fields.seatunnelVersion,
		"image_pull_policy":  fields.pullPolicy,
		"image_pull_secrets": fields.pullSecrets,
		"is_default":         fields.isDefault,
		"description":        fields.description,
		"registry_host":      fields.registryHost,
		"registry_username":  fields.registryUsername,
		"registry_password":  fields.registryPassword,
	}
}

// newPackageImageCreateCommand stx package image create。
// newPackageImageCreateCommand registers a container image.
func newPackageImageCreateCommand(storeProvider authStoreProvider) *cobra.Command {
	var options packageWriteOptions
	var fields containerImageFlags
	command := &cobra.Command{
		Use:     "create",
		Short:   "Register a container image for Kubernetes deploys",
		Example: "stx package image create --name st-2.3.13 --image apache/seatunnel:2.3.13 --seatunnel-version 2.3.13 --confirm",
		Args:    usageArgs(cobra.NoArgs),
		RunE: func(command *cobra.Command, args []string) error {
			client, headers, err := preparePackageWrite(command, storeProvider, "package.image.create", &options,
				"registering a container image affects later Kubernetes deploy and upgrade choices")
			if err != nil {
				return err
			}
			if err := resolveRegistryPassword(command, &fields, command.InOrStdin()); err != nil {
				return err
			}
			var data any
			requestID, err := client.RequestWithHeaders(command.Context(), http.MethodPost,
				"/api/v1/packages/images", containerImageBody(&fields), headers, &data)
			if err != nil {
				return err
			}
			return renderPackageResult(command, "package.image.create", requestID, data, "")
		},
	}
	addPackageWriteFlags(command, &options)
	addContainerImageFlags(command, &fields)
	return command
}

// newPackageImageUpdateCommand stx package image update。
// newPackageImageUpdateCommand updates a registered container image.
func newPackageImageUpdateCommand(storeProvider authStoreProvider) *cobra.Command {
	var options packageWriteOptions
	var fields containerImageFlags
	command := &cobra.Command{
		Use:     "update <id>",
		Short:   "Update a registered container image",
		Example: "stx package image update 1 --name st-2.3.13 --image apache/seatunnel:2.3.13 --confirm",
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			client, headers, err := preparePackageWrite(command, storeProvider, "package.image.update", &options,
				"updating a container image affects later Kubernetes deploy and upgrade choices")
			if err != nil {
				return err
			}
			if err := resolveRegistryPassword(command, &fields, command.InOrStdin()); err != nil {
				return err
			}
			var data any
			requestID, err := client.RequestWithHeaders(command.Context(), http.MethodPut,
				"/api/v1/packages/images/"+args[0], containerImageBody(&fields), headers, &data)
			if err != nil {
				return err
			}
			return renderPackageResult(command, "package.image.update", requestID, data, "")
		},
	}
	addPackageWriteFlags(command, &options)
	addContainerImageFlags(command, &fields)
	return command
}

// resolveRegistryPassword 密码输入收口：stdin 优先、显式明文兼容、TTY 隐藏交互，
// 非 TTY 且无输入时明确报错而不是静默发空密码。
// resolveRegistryPassword funnels password input: stdin wins, explicit plaintext stays
// for compatibility, a TTY falls back to the hidden prompt, and non-TTY without input
// errors out explicitly instead of silently sending an empty password.
func resolveRegistryPassword(command *cobra.Command, fields *containerImageFlags, stdin io.Reader) error {
	if fields.registryPassword != "" && fields.registryPasswordStdin {
		return clioutput.NewError(clioutput.CodeUsage, "--registry-password cannot be combined with --registry-password-stdin", clioutput.ExitUsage, false)
	}
	needsPassword := strings.TrimSpace(fields.registryHost) != "" || strings.TrimSpace(fields.registryUsername) != ""
	if fields.registryPasswordStdin {
		content, err := io.ReadAll(stdin)
		if err != nil {
			return clioutput.WrapError(err, clioutput.CodeUsage, "read registry password from stdin", clioutput.ExitUsage, false)
		}
		password := strings.TrimRight(string(content), "\r\n")
		if password == "" {
			return clioutput.NewError(clioutput.CodeUsage, "registry-password-stdin is empty", clioutput.ExitUsage, false)
		}
		fields.registryPassword = password
		return nil
	}
	if fields.registryPassword != "" || !needsPassword {
		return nil
	}
	if !isTerminalReader(stdin) {
		return clioutput.NewError(clioutput.CodeUsage, "registry credentials in non-interactive mode require --registry-password-stdin", clioutput.ExitUsage, false)
	}
	password, err := readTerminalPassword(stdin, command.ErrOrStderr())
	if err != nil {
		return clioutput.WrapError(err, clioutput.CodeUsage, "read registry password", clioutput.ExitUsage, false)
	}
	if password == "" {
		return clioutput.NewError(clioutput.CodeUsage, "registry password is empty", clioutput.ExitUsage, false)
	}
	fields.registryPassword = password
	return nil
}
