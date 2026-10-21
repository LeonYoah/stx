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

	"github.com/LeonYoah/stx/internal/apps/selfupgrade"
	cliClient "github.com/LeonYoah/stx/internal/cli/client"
	cliConfig "github.com/LeonYoah/stx/internal/cli/config"
	clioutput "github.com/LeonYoah/stx/internal/cli/output"
	stxversion "github.com/LeonYoah/stx/internal/version"
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
			if force != "" {
				if err := validateSelfUpgradeVersionFlag("--force-latest", force); err != nil {
					return err
				}
			}
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

	var targetVersion string
	var stageSim string
	stageCmd := &cobra.Command{
		Use:   "stage",
		Short: "Stage and download release assets from network",
		Args:  usageArgs(cobra.NoArgs),
		RunE: func(command *cobra.Command, _ []string) error {
			if targetVersion != "" {
				if err := validateSelfUpgradeVersionFlag("--target-version", targetVersion); err != nil {
					return err
				}
			}
			client, err := resolveSelfUpgradeClient(command, storeProvider, server, namespace)
			if err != nil {
				return err
			}
			body := map[string]string{}
			if targetVersion != "" {
				body["target_version"] = targetVersion
			}
			if stageSim != "" {
				body["simulate_failure"] = stageSim
			}
			var data any
			requestID, err := client.Request(command.Context(), http.MethodPost, "/api/v1/self-upgrade/stage/download", body, &data)
			if err != nil {
				return err
			}
			return renderCommandResultWithRequestID(command, "self_upgrade.stage", requestID, data)
		},
	}
	stageCmd.Flags().StringVar(&targetVersion, "target-version", "", "Target release version to stage")
	stageCmd.Flags().StringVar(&stageSim, "simulate-failure", "", "Simulate failure code for staging tests (download_partial, disk_full)")

	var execSim string
	var execDryRun bool
	executeCmd := &cobra.Command{
		Use:   "execute",
		Short: "Execute STX upgrade from local staged assets (applies by default; --dry-run to rehearse)",
		Long: `Execute the STX self-upgrade from the locally staged package.

By default this APPLIES the upgrade for real (mode=apply): it replaces the control
plane binary, rolls the Agent out to every node (offline nodes upgrade when they
reconnect), replaces stx-java-proxy jars, and restarts the STX processes involved:
the control plane, the bundled frontend, each Agent and every running
stx-java-proxy. SeaTunnel/Hazelcast processes and jobs are never stopped or
restarted. If the new control plane fails to start it is rolled back to the backup.

Use --dry-run (mode=dry_run) to rehearse: every step is planned and reported but
nothing is applied and no process is restarted.

执行自升级：默认真实应用（mode=apply），会替换控制面、下发 Agent、替换 proxy jar，
并重启涉及的 STX 进程（控制面、随包前端、Agent、运行中的 stx-java-proxy）；
SeaTunnel/Hazelcast 进程与作业不受影响。--dry-run（mode=dry_run）只演练，不做任何变更。`,
		Example: `  stx self-upgrade execute            # apply the staged upgrade for real
  stx self-upgrade execute --dry-run  # rehearse only; nothing is applied`,
		Args: usageArgs(cobra.NoArgs),
		RunE: func(command *cobra.Command, _ []string) error {
			client, err := resolveSelfUpgradeClient(command, storeProvider, server, namespace)
			if err != nil {
				return err
			}
			body := map[string]any{"mode": selfUpgradeExecuteMode(execDryRun)}
			if execSim != "" {
				body["simulate_failure"] = execSim
			}
			var data any
			requestID, err := client.Request(command.Context(), http.MethodPost, "/api/v1/self-upgrade/execute", body, &data)
			if err != nil {
				return err
			}
			return renderCommandResultWithRequestID(command, "self_upgrade.execute", requestID, data)
		},
	}
	executeCmd.Flags().BoolVar(&execDryRun, "dry-run", false, "Rehearse only (mode=dry_run): plan every step but apply nothing and restart nothing. Without it the upgrade is APPLIED (mode=apply)")
	executeCmd.Flags().StringVar(&execSim, "simulate-failure", "", "Simulate failure code during execution (step_fail_control_plane, step_fail_agent, step_fail_proxy, verify_fail)")

	historyCmd := &cobra.Command{
		Use:   "history",
		Short: "List STX self-upgrade execution history",
		Args:  usageArgs(cobra.NoArgs),
		RunE: func(command *cobra.Command, _ []string) error {
			client, err := resolveSelfUpgradeClient(command, storeProvider, server, namespace)
			if err != nil {
				return err
			}
			var data any
			requestID, err := client.Request(command.Context(), http.MethodGet, "/api/v1/self-upgrade/history", nil, &data)
			if err != nil {
				return err
			}
			return renderCommandResultWithRequestID(command, "self_upgrade.history", requestID, data)
		},
	}

	cmd.PersistentFlags().StringVar(&server, "server", "", "Remote STX server URL")
	cmd.PersistentFlags().StringVar(&namespace, "namespace", "", "Local namespace to use")

	cmd.AddCommand(statusCmd, checkCmd, stageCmd, executeCmd, historyCmd)
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

// validateSelfUpgradeVersionFlag 在发送请求前拒绝非法版本号（如路径穿越）。
// validateSelfUpgradeVersionFlag rejects invalid versions (such as path traversal) before sending requests.
func validateSelfUpgradeVersionFlag(flag, value string) error {
	if err := stxversion.ValidateReleaseVersion(value); err != nil {
		return clioutput.NewError(clioutput.CodeUsage, "invalid "+flag+": "+err.Error(), clioutput.ExitUsage, false)
	}
	return nil
}

// selfUpgradeExecuteMode 把 --dry-run 映射为显式的执行模式（默认 apply）。
// selfUpgradeExecuteMode maps --dry-run to the explicit execution mode (apply by default).
func selfUpgradeExecuteMode(dryRun bool) string {
	if dryRun {
		return selfupgrade.ExecModeDryRun
	}
	return selfupgrade.ExecModeApply
}

// newSelfUpgradeWatchdogCommand 是自升级重启 watchdog 的隐藏入口，由旧控制面在独立会话中启动。
// newSelfUpgradeWatchdogCommand is the hidden entry of the self-upgrade restart watchdog, started detached by the
// old control plane.
func newSelfUpgradeWatchdogCommand() *cobra.Command {
	var planPath string
	command := &cobra.Command{
		Use:    selfupgrade.WatchdogCommand,
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return selfupgrade.RunRestartWatchdog(planPath)
		},
	}
	command.Flags().StringVar(&planPath, "plan", "", "restart plan path")
	_ = command.MarkFlagRequired("plan")
	return command
}
