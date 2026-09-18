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
	cliClient "github.com/LeonYoah/stx/internal/cli/client"
	cliConfig "github.com/LeonYoah/stx/internal/cli/config"
	"github.com/spf13/cobra"
)

func newHealthCommand() *cobra.Command {
	return newHealthCommandWithStore(cliConfig.NewDefaultStore)
}

func newHealthCommandWithStore(storeProvider authStoreProvider) *cobra.Command {
	var server, namespace string
	command := &cobra.Command{
		Use:   "health",
		Short: "Check a remote STX server",
		Args:  usageArgs(cobra.NoArgs),
		RunE: func(command *cobra.Command, _ []string) error {
			store, err := storeProvider()
			if err != nil {
				return classifyAuthCommandError(err)
			}
			resolved, err := store.Resolve(cliConfig.Overrides{Namespace: namespace, Server: server})
			if err != nil {
				return classifyAuthCommandError(err)
			}
			client, err := cliClient.New(resolved, nil)
			if err != nil {
				return err
			}
			requestID, data, err := client.Health(command.Context())
			if err != nil {
				return err
			}
			return renderCommandResultWithRequestID(command, "health.get", requestID, data)
		},
	}
	command.Flags().StringVar(&server, "server", "", "Remote STX server URL")
	command.Flags().StringVar(&namespace, "namespace", "", "Local namespace to use")
	return command
}
