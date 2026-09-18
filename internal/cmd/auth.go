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
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	cliClient "github.com/LeonYoah/stx/internal/cli/client"
	cliConfig "github.com/LeonYoah/stx/internal/cli/config"
	clioutput "github.com/LeonYoah/stx/internal/cli/output"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

type authStoreProvider func() (*cliConfig.Store, error)

type authCommandOptions struct {
	storeProvider authStoreProvider
	stdin         io.Reader
}

type loginResult struct {
	Server    string             `json:"server" yaml:"server"`
	Namespace string             `json:"namespace" yaml:"namespace"`
	TokenSet  bool               `json:"token_set" yaml:"token_set"`
	TokenType string             `json:"token_type" yaml:"token_type"`
	ExpiresAt time.Time          `json:"expires_at" yaml:"expires_at"`
	User      cliClient.UserInfo `json:"user" yaml:"user"`
}

type logoutResult struct {
	Namespace string `json:"namespace" yaml:"namespace"`
	Revoked   bool   `json:"revoked" yaml:"revoked"`
}

func newAuthCommands() []*cobra.Command {
	return newAuthCommandsWithOptions(authCommandOptions{
		storeProvider: cliConfig.NewDefaultStore,
		stdin:         os.Stdin,
	})
}

func newAuthCommandsWithOptions(options authCommandOptions) []*cobra.Command {
	return []*cobra.Command{
		newLoginCommand(options),
		newLogoutCommand(options),
		newWhoAmICommand(options),
	}
}

func newLoginCommand(options authCommandOptions) *cobra.Command {
	var server, namespace, username, expiresIn string
	var passwordStdin bool
	command := &cobra.Command{
		Use:   "login",
		Short: "Log in to a remote STX server",
		Args:  usageArgs(cobra.NoArgs),
		RunE: func(command *cobra.Command, _ []string) error {
			store, err := options.storeProvider()
			if err != nil {
				return classifyAuthCommandError(err)
			}
			resolved, err := store.Resolve(cliConfig.Overrides{
				Namespace: namespace,
				Server:    server,
				Output:    "json",
			})
			if err != nil {
				return classifyAuthCommandError(err)
			}
			if resolved.Server == "" {
				return clioutput.NewError(clioutput.CodeUsage, "--server or an existing namespace is required", clioutput.ExitUsage, false)
			}
			if username == "" {
				username = strings.TrimSpace(os.Getenv("STX_USERNAME"))
			}
			if username == "" {
				return clioutput.NewError(clioutput.CodeUsage, "--username or STX_USERNAME is required", clioutput.ExitUsage, false)
			}
			password, err := readLoginPassword(command, options.stdin, passwordStdin)
			if err != nil {
				return err
			}

			client, err := cliClient.New(resolved, nil)
			if err != nil {
				return err
			}
			requestID, data, err := client.Login(command.Context(), username, password, expiresIn)
			if err != nil {
				return err
			}

			name := strings.TrimSpace(namespace)
			if name == "" {
				name = resolved.Name
			}
			if name == "" {
				name = "default"
			}
			file, err := store.Load()
			if err != nil {
				return classifyAuthCommandError(err)
			}
			stored := file.Namespaces[name]
			stored.Server = resolved.Server
			stored.Token = data.Token
			stored.TokenExpiresAt = data.ExpiresAt.Format(time.RFC3339)
			if stored.Output == "" {
				stored.Output = resolved.Output
			}
			if stored.Timeout == "" {
				stored.Timeout = resolved.Timeout.String()
			}
			if err := store.Upsert(name, stored, true); err != nil {
				return classifyAuthCommandError(err)
			}
			return renderCommandResultWithRequestID(command, "auth.cli.login", requestID, loginResult{
				Server:    resolved.Server,
				Namespace: name,
				TokenSet:  true,
				TokenType: data.TokenType,
				ExpiresAt: data.ExpiresAt,
				User:      data.User,
			})
		},
	}
	command.Flags().StringVar(&server, "server", "", "Remote STX server URL")
	command.Flags().StringVar(&namespace, "namespace", "", "Local namespace to update")
	command.Flags().StringVar(&username, "username", "", "STX username")
	command.Flags().StringVar(&expiresIn, "expires-in", "7d", "Token lifetime from 7d to 30d")
	command.Flags().BoolVar(&passwordStdin, "password-stdin", false, "Read the password from stdin")
	return command
}

func newLogoutCommand(options authCommandOptions) *cobra.Command {
	var namespace string
	command := &cobra.Command{
		Use:   "logout",
		Short: "Revoke the current STX CLI token",
		Args:  usageArgs(cobra.NoArgs),
		RunE: func(command *cobra.Command, _ []string) error {
			store, err := options.storeProvider()
			if err != nil {
				return classifyAuthCommandError(err)
			}
			resolved, err := store.Resolve(cliConfig.Overrides{Namespace: namespace})
			if err != nil {
				return classifyAuthCommandError(err)
			}
			name := resolved.Name
			if strings.TrimSpace(resolved.Token) == "" {
				return clioutput.NewError(clioutput.CodeAuthentication, "CLI token is not configured", clioutput.ExitAuthentication, false)
			}
			client, err := cliClient.New(resolved, nil)
			if err != nil {
				return err
			}
			requestID, err := client.Logout(command.Context())
			if err != nil {
				// 服务端明确表示令牌已不可用时，清掉本地凭据，避免后续命令继续使用它。
				// Remove a locally unusable token when the server explicitly rejects it.
				var cliErr *clioutput.CLIError
				if errors.As(err, &cliErr) && cliErr.ExitCode == clioutput.ExitAuthentication {
					_ = clearStoredToken(store, name)
				}
				return err
			}
			if err := clearStoredToken(store, name); err != nil {
				return classifyAuthCommandError(err)
			}
			return renderCommandResultWithRequestID(command, "auth.cli.logout", requestID, logoutResult{Namespace: name, Revoked: true})
		},
	}
	command.Flags().StringVar(&namespace, "namespace", "", "Local namespace to use")
	return command
}

func newWhoAmICommand(options authCommandOptions) *cobra.Command {
	var namespace string
	command := &cobra.Command{
		Use:   "whoami",
		Short: "Show the current STX CLI user",
		Args:  usageArgs(cobra.NoArgs),
		RunE: func(command *cobra.Command, _ []string) error {
			store, err := options.storeProvider()
			if err != nil {
				return classifyAuthCommandError(err)
			}
			resolved, err := store.Resolve(cliConfig.Overrides{Namespace: namespace})
			if err != nil {
				return classifyAuthCommandError(err)
			}
			if strings.TrimSpace(resolved.Token) == "" {
				return clioutput.NewError(clioutput.CodeAuthentication, "CLI token is not configured", clioutput.ExitAuthentication, false)
			}
			client, err := cliClient.New(resolved, nil)
			if err != nil {
				return err
			}
			requestID, user, err := client.WhoAmI(command.Context())
			if err != nil {
				return err
			}
			return renderCommandResultWithRequestID(command, "auth.cli.whoami", requestID, user)
		},
	}
	command.Flags().StringVar(&namespace, "namespace", "", "Local namespace to use")
	return command
}

func readLoginPassword(command *cobra.Command, stdin io.Reader, fromStdin bool) (string, error) {
	if fromStdin {
		content, err := io.ReadAll(stdin)
		if err != nil {
			return "", clioutput.WrapError(err, clioutput.CodeUsage, "read password from stdin", clioutput.ExitUsage, false)
		}
		password := strings.TrimRight(string(content), "\r\n")
		if password == "" {
			return "", clioutput.NewError(clioutput.CodeUsage, "password-stdin is empty", clioutput.ExitUsage, false)
		}
		return password, nil
	}

	file, ok := stdin.(*os.File)
	if !ok || !term.IsTerminal(int(file.Fd())) {
		return "", clioutput.NewError(clioutput.CodeUsage, "non-interactive login requires --password-stdin", clioutput.ExitUsage, false)
	}
	_, _ = fmt.Fprint(command.ErrOrStderr(), "Password: ")
	content, err := term.ReadPassword(int(file.Fd()))
	_, _ = fmt.Fprintln(command.ErrOrStderr())
	if err != nil {
		return "", clioutput.WrapError(err, clioutput.CodeUsage, "read password", clioutput.ExitUsage, false)
	}
	if len(content) == 0 {
		return "", clioutput.NewError(clioutput.CodeUsage, "password is empty", clioutput.ExitUsage, false)
	}
	return string(content), nil
}

func clearStoredToken(store *cliConfig.Store, name string) error {
	file, err := store.Load()
	if err != nil {
		return err
	}
	namespace, ok := file.Namespaces[name]
	if !ok {
		return cliConfig.ErrNamespaceNotFound
	}
	namespace.Token = ""
	namespace.TokenExpiresAt = ""
	return store.Upsert(name, namespace, file.CurrentNamespace == name)
}

func classifyAuthCommandError(err error) error {
	switch {
	case errors.Is(err, cliConfig.ErrNamespaceNameRequired), errors.Is(err, cliConfig.ErrNoCurrentNamespace):
		return clioutput.WrapError(err, clioutput.CodeUsage, err.Error(), clioutput.ExitUsage, false)
	case errors.Is(err, cliConfig.ErrNamespaceNotFound):
		return clioutput.WrapError(err, clioutput.CodeNotFound, err.Error(), clioutput.ExitNotFound, false)
	default:
		return err
	}
}
