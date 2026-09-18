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

	cliconfig "github.com/LeonYoah/stx/internal/cli/config"
	clioutput "github.com/LeonYoah/stx/internal/cli/output"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

type namespaceStoreProvider func() (*cliconfig.Store, error)

type namespaceListItem struct {
	Name    string `json:"name" yaml:"name"`
	Current bool   `json:"current" yaml:"current"`
	Server  string `json:"server,omitempty" yaml:"server,omitempty"`
}

type namespaceListResult struct {
	CurrentNamespace string              `json:"current_namespace,omitempty" yaml:"current_namespace,omitempty"`
	Namespaces       []namespaceListItem `json:"namespaces" yaml:"namespaces"`
}

type namespaceShowResult struct {
	Name           string `json:"name" yaml:"name"`
	Current        bool   `json:"current" yaml:"current"`
	Server         string `json:"server,omitempty" yaml:"server,omitempty"`
	TokenSet       bool   `json:"token_set" yaml:"token_set"`
	TokenExpiresAt string `json:"token_expires_at,omitempty" yaml:"token_expires_at,omitempty"`
	Output         string `json:"output,omitempty" yaml:"output,omitempty"`
	Timeout        string `json:"timeout,omitempty" yaml:"timeout,omitempty"`
}

type namespaceMutationResult struct {
	Name             string `json:"name" yaml:"name"`
	CurrentNamespace string `json:"current_namespace,omitempty" yaml:"current_namespace,omitempty"`
	Deleted          bool   `json:"deleted,omitempty" yaml:"deleted,omitempty"`
}

func newNamespaceCommand() *cobra.Command {
	return newNamespaceCommandWithStore(cliconfig.NewDefaultStore)
}

func newNamespaceCommandWithStore(storeProvider namespaceStoreProvider) *cobra.Command {
	command := &cobra.Command{
		Use:   "namespace",
		Short: "Manage local STX server namespaces",
		Args:  usageArgs(cobra.NoArgs),
	}
	command.AddCommand(
		newNamespaceListCommand(storeProvider),
		newNamespaceShowCommand(storeProvider),
		newNamespaceUseCommand(storeProvider),
		newNamespaceDeleteCommand(storeProvider),
	)
	return command
}

func newNamespaceListCommand(storeProvider namespaceStoreProvider) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List configured namespaces",
		Args:  usageArgs(cobra.NoArgs),
		RunE: func(command *cobra.Command, _ []string) error {
			store, err := storeProvider()
			if err != nil {
				return classifyNamespaceError(err)
			}
			items, err := store.List()
			if err != nil {
				return classifyNamespaceError(err)
			}
			result := namespaceListResult{Namespaces: make([]namespaceListItem, 0, len(items))}
			for _, item := range items {
				result.Namespaces = append(result.Namespaces, namespaceListItem{
					Name:    item.Name,
					Current: item.Current,
					Server:  item.Server,
				})
				if item.Current {
					result.CurrentNamespace = item.Name
				}
			}
			return renderCommandResult(command, "namespace.list", result)
		},
	}
}

func newNamespaceShowCommand(storeProvider namespaceStoreProvider) *cobra.Command {
	return &cobra.Command{
		Use:   "show [name]",
		Short: "Show namespace metadata without revealing its token",
		Args:  usageArgs(cobra.MaximumNArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			store, err := storeProvider()
			if err != nil {
				return classifyNamespaceError(err)
			}
			name := ""
			if len(args) == 1 {
				name = args[0]
			}
			resolvedName, namespace, err := store.Get(name)
			if err != nil {
				return classifyNamespaceError(err)
			}
			file, err := store.Load()
			if err != nil {
				return classifyNamespaceError(err)
			}
			return renderCommandResult(command, "namespace.get", namespaceShowResult{
				Name:           resolvedName,
				Current:        resolvedName == file.CurrentNamespace,
				Server:         namespace.Server,
				TokenSet:       namespace.Token != "",
				TokenExpiresAt: namespace.TokenExpiresAt,
				Output:         namespace.Output,
				Timeout:        namespace.Timeout,
			})
		},
	}
}

func newNamespaceUseCommand(storeProvider namespaceStoreProvider) *cobra.Command {
	return &cobra.Command{
		Use:   "use <name>",
		Short: "Select the current namespace",
		Args:  usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			store, err := storeProvider()
			if err != nil {
				return classifyNamespaceError(err)
			}
			if err := store.Use(args[0]); err != nil {
				return classifyNamespaceError(err)
			}
			return renderCommandResult(command, "namespace.use", namespaceMutationResult{
				Name:             args[0],
				CurrentNamespace: args[0],
			})
		},
	}
}

func newNamespaceDeleteCommand(storeProvider namespaceStoreProvider) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a local namespace",
		Args:  usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			store, err := storeProvider()
			if err != nil {
				return classifyNamespaceError(err)
			}
			if err := store.Delete(args[0]); err != nil {
				return classifyNamespaceError(err)
			}
			file, err := store.Load()
			if err != nil {
				return classifyNamespaceError(err)
			}
			return renderCommandResult(command, "namespace.delete", namespaceMutationResult{
				Name:             args[0],
				CurrentNamespace: file.CurrentNamespace,
				Deleted:          true,
			})
		},
	}
}

func renderCommandResult(command *cobra.Command, operationID string, data any) error {
	return renderCommandResultWithRequestID(command, operationID, "local_"+uuid.NewString(), data)
}

// renderCommandResultWithRequestID 使用指定请求编号输出普通 CLI 结果。
// renderCommandResultWithRequestID renders a regular CLI result with a supplied request ID.
func renderCommandResultWithRequestID(command *cobra.Command, operationID, requestID string, data any) error {
	options, err := clioutput.OptionsFromCommand(command)
	if err != nil {
		return err
	}
	result := clioutput.NewResult(operationID, requestID, data)
	renderer := clioutput.NewRenderer(command.OutOrStdout(), clioutput.NewEventWriter(command.ErrOrStderr()))
	return renderer.Render(result, options)
}

func classifyNamespaceError(err error) error {
	switch {
	case errors.Is(err, cliconfig.ErrNamespaceNameRequired):
		return clioutput.WrapError(err, clioutput.CodeUsage, err.Error(), clioutput.ExitUsage, false)
	case errors.Is(err, cliconfig.ErrNamespaceNotFound), errors.Is(err, cliconfig.ErrNoCurrentNamespace):
		return clioutput.WrapError(err, clioutput.CodeNotFound, err.Error(), clioutput.ExitNotFound, false)
	default:
		return err
	}
}
