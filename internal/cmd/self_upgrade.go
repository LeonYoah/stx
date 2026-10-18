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

	cliClient "github.com/LeonYoah/stx/internal/cli/client"
	cliConfig "github.com/LeonYoah/stx/internal/cli/config"
	"github.com/spf13/cobra"
)

// newSelfUpgradeCommand 创建控制台与组件自升级 CLI 命令树。
// newSelfUpgradeCommand constructs the STX self-upgrade CLI command tree.
func newSelfUpgradeCommand(storeProvider authStoreProvider) *cobra.Command {
	var server, namespace string
	cmd := &cobra.Command{
		Use:   "self-upgrade",
		Short: "Manage STX console and component self-upgrades",
		Long:  "Check, stage, and execute self-upgrades for STX control plane and components.",
	}

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Get current STX self-upgrade and deployment status",
		Args:  usageArgs(cobra.NoArgs),
		RunE: func(command *cobra.Command, _ []string) error {
			client, err := resolveSelfUpgradeClient(command, storeProvider, server, namespace)
			if err != nil {
				return err
			}
			var data any
			requestID, err := client.Request(command.Context(), http.MethodGet, "/api/v1/self-upgrade/status", nil, &data)
			if err != nil {
				return err
			}
			return renderCommandResultWithRequestID(command, "self_upgrade.status", requestID, data)
		},
	}

	var sim string
	var force string
	checkCmd := &cobra.Command{
		Use:   "check",
		Short: "Check for new STX releases",
		Args:  usageArgs(cobra.NoArgs),
		RunE: func(command *cobra.Command, _ []string) error {
			client, err := resolveSelfUpgradeClient(command, storeProvider, server, namespace)
			if err != nil {
				return err
			}
			body := map[string]string{}
			if sim != "" {
				body["simulate_failure"] = sim
			}
			if force != "" {
				body["force_latest"] = force
			}
			var data any
			requestID, err := client.Request(command.Context(), http.MethodPost, "/api/v1/self-upgrade/check", body, &data)
			if err != nil {
				return err
			}
			return renderCommandResultWithRequestID(command, "self_upgrade.check", requestID, data)
		},
	}
	checkCmd.Flags().StringVar(&sim, "simulate-failure", "", "Simulate failure code for testing (check_fail, already_latest, etc.)")
	checkCmd.Flags().StringVar(&force, "force-latest", "", "Force a target version tag for verification")

	cmd.PersistentFlags().StringVar(&server, "server", "", "Remote STX server URL")
	cmd.PersistentFlags().StringVar(&namespace, "namespace", "", "Local namespace to use")

	cmd.AddCommand(statusCmd, checkCmd)
	return cmd
}

// resolveSelfUpgradeClient 解析自升级命令专用的 API 客户端。
// resolveSelfUpgradeClient resolves an API client instance for self-upgrade commands.
func resolveSelfUpgradeClient(command *cobra.Command, storeProvider authStoreProvider, server, namespace string) (*cliClient.Client, error) {
	store, err := storeProvider()
	if err != nil {
		return nil, classifyAuthCommandError(err)
	}
	resolved, err := store.Resolve(cliConfig.Overrides{Namespace: namespace, Server: server})
	if err != nil {
		return nil, classifyAuthCommandError(err)
	}
	return cliClient.New(resolved, nil)
}
