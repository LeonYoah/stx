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
	"net/url"
	"os"
	"strings"

	cliConfig "github.com/LeonYoah/stx/internal/cli/config"
	clioutput "github.com/LeonYoah/stx/internal/cli/output"
	"github.com/spf13/cobra"
)

type userWriteCommandOptions struct {
	storeProvider        authStoreProvider
	stdin                io.Reader
	isTerminal           func(io.Reader) bool
	readTerminalPassword func(io.Reader, io.Writer) (string, error)
}

// addUserWriteCommands 将个人资料和管理员用户写命令挂到现有命令树。
// addUserWriteCommands attaches profile and administrator user writes to the existing command tree.
func addUserWriteCommands(root *cobra.Command, options userWriteCommandOptions) {
	authCommand := childCommand(root, "auth")
	if authCommand == nil {
		panic("generated auth command is missing")
	}
	profileCommand := childCommand(authCommand, "profile")
	if profileCommand == nil {
		profileCommand = &cobra.Command{Use: "profile", Short: "Manage the current user profile"}
		authCommand.AddCommand(profileCommand)
	}
	profileCommand.AddCommand(newProfileUpdateCommand(options.storeProvider))

	adminCommand := childCommand(root, "admin")
	userCommand := childCommand(adminCommand, "user")
	if userCommand == nil {
		panic("generated admin user command is missing")
	}
	userCommand.AddCommand(newAdminUserCreateCommand(options), newAdminUserUpdateCommand(options), newAdminUserDeleteCommand(options.storeProvider))
}

func defaultUserWriteCommandOptions() userWriteCommandOptions {
	return userWriteCommandOptions{
		storeProvider:        defaultAuthStoreProvider,
		stdin:                os.Stdin,
		isTerminal:           isTerminalReader,
		readTerminalPassword: readTerminalPassword,
	}
}

func defaultAuthStoreProvider() (*cliConfig.Store, error) {
	return cliConfig.NewDefaultStore()
}

func newProfileUpdateCommand(storeProvider authStoreProvider) *cobra.Command {
	var options secureWriteOptions
	var email, language string
	command := &cobra.Command{
		Use:     "update",
		Short:   "Update the current user profile",
		Example: "stx auth profile update --language en --confirm",
		Args:    usageArgs(cobra.NoArgs),
		RunE: func(command *cobra.Command, _ []string) error {
			body := make(map[string]any)
			if command.Flags().Changed("email") {
				body["email"] = strings.TrimSpace(email)
			}
			if command.Flags().Changed("language") {
				body["language"] = strings.TrimSpace(language)
			}
			if len(body) == 0 {
				return clioutput.NewError(clioutput.CodeUsage, "--email or --language is required", clioutput.ExitUsage, false)
			}
			client, headers, err := prepareSecureWrite(command, storeProvider, "auth.profile.update", &options,
				"updating the profile changes the current user's email or language preference")
			if err != nil {
				return err
			}
			var data any
			requestID, err := client.RequestWithHeaders(command.Context(), http.MethodPut, "/api/v1/auth/profile", body, headers, &data)
			if err != nil {
				return handleSecureWriteError(command, "auth.profile.update", err)
			}
			return renderWriteResult(command, "auth.profile.update", requestID, data, "")
		},
	}
	addSecureWriteFlags(command, &options)
	command.Flags().StringVar(&email, "email", "", "Email address")
	command.Flags().StringVar(&language, "language", "", "Language preference: zh or en")
	return command
}

func newAdminUserCreateCommand(options userWriteCommandOptions) *cobra.Command {
	var writeOptions secureWriteOptions
	var username, nickname, email string
	var isAdmin, passwordStdin bool
	command := &cobra.Command{
		Use:     "create",
		Short:   "Create a user",
		Example: "stx admin user create --username test-user --password-stdin --confirm",
		Args:    usageArgs(cobra.NoArgs),
		RunE: func(command *cobra.Command, _ []string) error {
			if strings.TrimSpace(username) == "" {
				return clioutput.NewError(clioutput.CodeUsage, "--username is required", clioutput.ExitUsage, false)
			}
			password, err := readLoginPassword(command, options.stdin, passwordStdin, options.isTerminal, options.readTerminalPassword)
			if err != nil {
				return err
			}
			body := map[string]any{
				"username": strings.TrimSpace(username), "password": password,
				"nickname": strings.TrimSpace(nickname), "email": strings.TrimSpace(email), "is_admin": isAdmin,
			}
			client, headers, err := prepareSecureWrite(command, options.storeProvider, "admin.user.create", &writeOptions,
				"creating a user adds a new account that can sign in to STX")
			if err != nil {
				return err
			}
			var data any
			requestID, err := client.RequestWithHeaders(command.Context(), http.MethodPost, "/api/v1/admin/users", body, headers, &data)
			if err != nil {
				return handleSecureWriteError(command, "admin.user.create", err)
			}
			return renderWriteResult(command, "admin.user.create", requestID, data, "")
		},
	}
	addSecureWriteFlags(command, &writeOptions)
	command.Flags().StringVar(&username, "username", "", "Username")
	command.Flags().StringVar(&nickname, "nickname", "", "Display name")
	command.Flags().StringVar(&email, "email", "", "Email address")
	command.Flags().BoolVar(&isAdmin, "admin", false, "Create an administrator")
	command.Flags().BoolVar(&passwordStdin, "password-stdin", false, "Read the password from stdin")
	return command
}

func newAdminUserUpdateCommand(options userWriteCommandOptions) *cobra.Command {
	var writeOptions secureWriteOptions
	var nickname, email string
	var active, isAdmin, passwordStdin bool
	command := &cobra.Command{
		Use:     "update <id>",
		Short:   "Update a user",
		Example: "stx admin user update 2 --active=false --admin=false --confirm",
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			body := make(map[string]any)
			for name, value := range map[string]string{"nickname": nickname, "email": email} {
				if command.Flags().Changed(name) {
					body[name] = strings.TrimSpace(value)
				}
			}
			if command.Flags().Changed("active") {
				body["is_active"] = active
			}
			if command.Flags().Changed("admin") {
				body["is_admin"] = isAdmin
			}
			if passwordStdin {
				password, err := readLoginPassword(command, options.stdin, true, options.isTerminal, options.readTerminalPassword)
				if err != nil {
					return err
				}
				body["password"] = password
			}
			if len(body) == 0 {
				return clioutput.NewError(clioutput.CodeUsage, "at least one update flag is required", clioutput.ExitUsage, false)
			}
			client, headers, err := prepareSecureWrite(command, options.storeProvider, "admin.user.update", &writeOptions,
				"updating a user may change account status, administrator permission, or password")
			if err != nil {
				return err
			}
			var data any
			requestID, err := client.RequestWithHeaders(command.Context(), http.MethodPut, "/api/v1/admin/users/"+url.PathEscape(args[0]), body, headers, &data)
			if err != nil {
				return handleSecureWriteError(command, "admin.user.update", err)
			}
			return renderWriteResult(command, "admin.user.update", requestID, data, "")
		},
	}
	addSecureWriteFlags(command, &writeOptions)
	command.Flags().StringVar(&nickname, "nickname", "", "Display name")
	command.Flags().StringVar(&email, "email", "", "Email address")
	command.Flags().BoolVar(&active, "active", true, "Whether the user is active")
	command.Flags().BoolVar(&isAdmin, "admin", false, "Whether the user is an administrator")
	command.Flags().BoolVar(&passwordStdin, "password-stdin", false, "Read a new password from stdin")
	return command
}

func newAdminUserDeleteCommand(storeProvider authStoreProvider) *cobra.Command {
	var options secureWriteOptions
	command := &cobra.Command{
		Use:     "delete <id>",
		Short:   "Delete a user",
		Long:    "Delete a user permanently. CLI and direct API calls require a one-time confirmation ID.",
		Example: "stx admin user delete 2 --confirm --idempotency-key delete-user-2",
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			client, headers, err := prepareSecureWrite(command, storeProvider, "admin.user.delete", &options,
				"deleting a user cannot be restored through STX")
			if err != nil {
				return err
			}
			var data any
			requestID, err := client.RequestWithHeaders(command.Context(), http.MethodDelete, "/api/v1/admin/users/"+url.PathEscape(args[0]), nil, headers, &data)
			if err != nil {
				return handleSecureWriteError(command, "admin.user.delete", err)
			}
			return renderWriteResult(command, "admin.user.delete", requestID, data, "")
		},
	}
	addSecureWriteFlags(command, &options)
	return command
}
