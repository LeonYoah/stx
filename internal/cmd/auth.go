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
	storeProvider        authStoreProvider
	stdin                io.Reader
	isTerminal           func(io.Reader) bool
	readTerminalPassword func(io.Reader, io.Writer) (string, error)
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
		storeProvider:        cliConfig.NewDefaultStore,
		stdin:                os.Stdin,
		isTerminal:           isTerminalReader,
		readTerminalPassword: readTerminalPassword,
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
	if options.isTerminal == nil {
		options.isTerminal = isTerminalReader
	}
	if options.readTerminalPassword == nil {
		options.readTerminalPassword = readTerminalPassword
	}
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
			username, err = readLoginUsername(command, options.stdin, username, options.isTerminal)
			if err != nil {
				return err
			}
			password, err := readLoginPassword(command, options.stdin, passwordStdin, options.isTerminal, options.readTerminalPassword)
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

// readLoginUsername 按参数、环境变量、终端交互的顺序读取用户名。
// readLoginUsername reads the username from the flag, environment, or interactive terminal in that order.
func readLoginUsername(command *cobra.Command, stdin io.Reader, provided string, isTerminal func(io.Reader) bool) (string, error) {
	username := strings.TrimSpace(provided)
	if username == "" {
		username = strings.TrimSpace(os.Getenv("STX_USERNAME"))
	}
	if username != "" {
		return username, nil
	}
	if !isTerminal(stdin) {
		return "", clioutput.NewError(clioutput.CodeUsage, "--username or STX_USERNAME is required in non-interactive mode", clioutput.ExitUsage, false)
	}
	if _, err := fmt.Fprint(command.ErrOrStderr(), "Username: "); err != nil {
		return "", clioutput.WrapError(err, clioutput.CodeUsage, "write username prompt", clioutput.ExitUsage, false)
	}
	username, err := readTerminalLine(stdin)
	if err != nil {
		return "", clioutput.WrapError(err, clioutput.CodeUsage, "read username", clioutput.ExitUsage, false)
	}
	username = strings.TrimSpace(username)
	if username == "" {
		return "", clioutput.NewError(clioutput.CodeUsage, "username is empty", clioutput.ExitUsage, false)
	}
	return username, nil
}

// readLoginPassword 从标准输入或隐藏的终端输入中读取密码。
// readLoginPassword reads the password from stdin or hidden terminal input.
func readLoginPassword(command *cobra.Command, stdin io.Reader, fromStdin bool, isTerminal func(io.Reader) bool, terminalPassword func(io.Reader, io.Writer) (string, error)) (string, error) {
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

	if !isTerminal(stdin) {
		return "", clioutput.NewError(clioutput.CodeUsage, "non-interactive login requires --password-stdin", clioutput.ExitUsage, false)
	}
	password, err := terminalPassword(stdin, command.ErrOrStderr())
	if err != nil {
		return "", clioutput.WrapError(err, clioutput.CodeUsage, "read password", clioutput.ExitUsage, false)
	}
	if password == "" {
		return "", clioutput.NewError(clioutput.CodeUsage, "password is empty", clioutput.ExitUsage, false)
	}
	return password, nil
}

func isTerminalReader(reader io.Reader) bool {
	file, ok := reader.(*os.File)
	return ok && term.IsTerminal(int(file.Fd()))
}

// readTerminalPassword 先关闭终端回显，再显示提示并读取密码。
// readTerminalPassword disables terminal echo before showing the prompt and reading the password.
func readTerminalPassword(reader io.Reader, promptWriter io.Writer) (password string, returnErr error) {
	file, ok := reader.(*os.File)
	if !ok {
		return "", errors.New("terminal password input requires a file")
	}
	fd := int(file.Fd())
	state, err := term.MakeRaw(fd)
	if err != nil {
		return "", err
	}
	restored := false
	defer func() {
		if !restored {
			if err := term.Restore(fd, state); returnErr == nil && err != nil {
				returnErr = err
			}
		}
	}()
	if _, err := fmt.Fprint(promptWriter, "Password: "); err != nil {
		return "", err
	}
	password, err = readRawPassword(file)
	if restoreErr := term.Restore(fd, state); restoreErr == nil {
		restored = true
	} else if err == nil {
		err = restoreErr
	}
	if _, newlineErr := fmt.Fprintln(promptWriter); err == nil && newlineErr != nil {
		err = newlineErr
	}
	return password, err
}

func readRawPassword(reader io.Reader) (string, error) {
	var value []byte
	buffer := make([]byte, 1)
	for {
		count, err := reader.Read(buffer)
		if count > 0 {
			switch buffer[0] {
			case '\r', '\n':
				return string(value), nil
			case 3, 4:
				return "", errors.New("password input cancelled")
			case 8, 127:
				if len(value) > 0 {
					value = value[:len(value)-1]
				}
			default:
				value = append(value, buffer[0])
			}
		}
		if err != nil {
			return "", err
		}
	}
}

func readTerminalLine(reader io.Reader) (string, error) {
	var value strings.Builder
	buffer := make([]byte, 1)
	for {
		count, err := reader.Read(buffer)
		if count > 0 {
			switch buffer[0] {
			case '\n':
				return value.String(), nil
			case '\r':
			default:
				value.WriteByte(buffer[0])
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) && value.Len() > 0 {
				return value.String(), nil
			}
			return "", err
		}
	}
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
