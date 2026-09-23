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
	"strings"

	cliClient "github.com/LeonYoah/stx/internal/cli/client"
	cliConfig "github.com/LeonYoah/stx/internal/cli/config"
	clioutput "github.com/LeonYoah/stx/internal/cli/output"
	"github.com/spf13/cobra"
)

type capabilityGetResult struct {
	APIVersion       string                        `json:"api_version" yaml:"api_version"`
	ServerVersion    string                        `json:"server_version" yaml:"server_version"`
	MinCLIVersion    string                        `json:"min_cli_version" yaml:"min_cli_version"`
	RegistryRevision string                        `json:"registry_revision" yaml:"registry_revision"`
	Operation        cliClient.CapabilityOperation `json:"operation" yaml:"operation"`
}

func newCapabilityCommand() *cobra.Command {
	return newCapabilityCommandWithStore(cliConfig.NewDefaultStore)
}

func newCapabilityCommandWithStore(storeProvider authStoreProvider) *cobra.Command {
	command := &cobra.Command{
		Use:   "capability",
		Short: "Inspect remote STX server capabilities",
		Args:  usageArgs(cobra.NoArgs),
	}
	command.AddCommand(
		newCapabilityListCommand(storeProvider),
		newCapabilityGetCommand(storeProvider),
	)
	return command
}

func newCapabilityListCommand(storeProvider authStoreProvider) *cobra.Command {
	var namespace string
	command := &cobra.Command{
		Use:   "list",
		Short: "List operations supported by the remote STX server",
		Args:  usageArgs(cobra.NoArgs),
		RunE: func(command *cobra.Command, _ []string) error {
			client, err := clientForNamespace(storeProvider, namespace)
			if err != nil {
				return err
			}
			requestID, data, err := client.Capabilities(command.Context())
			if err != nil {
				return err
			}
			return renderCommandResultWithRequestID(command, "capability.list", requestID, data)
		},
	}
	command.Flags().StringVar(&namespace, "namespace", "", "Local namespace to use")
	return command
}

func newCapabilityGetCommand(storeProvider authStoreProvider) *cobra.Command {
	var namespace string
	command := &cobra.Command{
		Use:   "get <operation-id>",
		Short: "Show one remote STX operation capability",
		Args:  usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			client, err := clientForNamespace(storeProvider, namespace)
			if err != nil {
				return err
			}
			requestID, data, err := client.Capabilities(command.Context())
			if err != nil {
				return err
			}
			operationID := strings.TrimSpace(args[0])
			for _, item := range data.Operations {
				if item.OperationID == operationID {
					return renderCommandResultWithRequestID(command, "capability.get", requestID, capabilityGetResult{
						APIVersion:       data.APIVersion,
						ServerVersion:    data.ServerVersion,
						MinCLIVersion:    data.MinCLIVersion,
						RegistryRevision: data.RegistryRevision,
						Operation:        item,
					})
				}
			}
			return clioutput.NewError(clioutput.CodeNotFound, "operation is not supported by the remote STX server: "+operationID, clioutput.ExitNotFound, false)
		},
	}
	command.Flags().StringVar(&namespace, "namespace", "", "Local namespace to use")
	return command
}

func clientForNamespace(storeProvider authStoreProvider, namespace string) (*cliClient.Client, error) {
	store, err := storeProvider()
	if err != nil {
		return nil, classifyAuthCommandError(err)
	}
	resolved, err := store.Resolve(cliConfig.Overrides{Namespace: namespace})
	if err != nil {
		return nil, classifyAuthCommandError(err)
	}
	if strings.TrimSpace(resolved.Token) == "" {
		return nil, clioutput.NewError(clioutput.CodeAuthentication, "CLI token is not configured", clioutput.ExitAuthentication, false)
	}
	return cliClient.New(resolved, nil)
}
