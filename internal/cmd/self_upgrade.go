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
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/LeonYoah/stx/internal/apps/selfupgrade"
	cliClient "github.com/LeonYoah/stx/internal/cli/client"
	cliConfig "github.com/LeonYoah/stx/internal/cli/config"
	clioutput "github.com/LeonYoah/stx/internal/cli/output"
	"github.com/LeonYoah/stx/internal/operation"
	stxversion "github.com/LeonYoah/stx/internal/version"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

// selfUpgradeTaskPollInterval 是 task get --wait 的轮询间隔；测试可调小以加速。
// selfUpgradeTaskPollInterval is the poll interval for `task get --wait`; tests may shrink it.
var selfUpgradeTaskPollInterval = 2 * time.Second

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

	importCmd := newSelfUpgradeImportCommand(storeProvider, &server, &namespace)
	taskCmd := newSelfUpgradeTaskCommand(storeProvider, &server, &namespace)

	cmd.PersistentFlags().StringVar(&server, "server", "", "Remote STX server URL")
	cmd.PersistentFlags().StringVar(&namespace, "namespace", "", "Local namespace to use")

	cmd.AddCommand(statusCmd, checkCmd, stageCmd, executeCmd, historyCmd, importCmd, taskCmd)
	return cmd
}

// newSelfUpgradeImportCommand 创建离线升级包导入命令（multipart 流式上传，不把包读进内存）。
// newSelfUpgradeImportCommand creates the offline upgrade package import command (streaming multipart upload
// without buffering the package in memory).
func newSelfUpgradeImportCommand(storeProvider authStoreProvider, server, namespace *string) *cobra.Command {
	var options secureWriteOptions
	command := &cobra.Command{
		Use:   "import <file>",
		Short: "Import an offline STX upgrade package",
		Long: `Upload a locally built or downloaded offline upgrade package (.tar.gz, .tgz, or .zip)
into the STX self-upgrade staging directory. The file is streamed to the server, so
packages up to the server limit are not buffered in CLI memory.

导入离线升级包（.tar.gz、.tgz 或 .zip）到本机自升级预置目录；文件流式上传，不会在 CLI 内存中缓冲整个包。`,
		Example: `  stx self-upgrade import ./stx-upgrade-1.4.1.tar.gz --confirm`,
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			file, err := os.Open(args[0])
			if err != nil {
				return clioutput.WrapError(err, clioutput.CodeUsage, "open upgrade package", clioutput.ExitUsage, false)
			}
			defer file.Close()
			info, err := file.Stat()
			if err != nil || !info.Mode().IsRegular() {
				return clioutput.NewError(clioutput.CodeUsage, "upgrade package must be a regular file", clioutput.ExitUsage, false)
			}
			impact := selfUpgradeOperationImpact("self_upgrade.stage.import", "Uploads and unpacks an offline package into the local self-upgrade staging directory.")
			client, headers, err := prepareSelfUpgradeWrite(command, storeProvider, *server, *namespace, &options, "self_upgrade.stage.import", impact)
			if err != nil {
				return err
			}
			var data any
			requestID, err := client.RequestMultipart(command.Context(), http.MethodPost, "/api/v1/self-upgrade/stage/import",
				nil, "file", filepath.Base(args[0]), file, headers, &data)
			if err != nil {
				return handleSecureWriteError(command, "self_upgrade.stage.import", err)
			}
			return renderWriteResult(command, "self_upgrade.stage.import", requestID, data, "stx self-upgrade status")
		},
	}
	addSelfUpgradeWriteFlags(command, &options)
	return command
}

// newSelfUpgradeTaskCommand 创建自升级任务查询与取消子命令组。
// newSelfUpgradeTaskCommand creates the self-upgrade task inspection and cancellation command group.
func newSelfUpgradeTaskCommand(storeProvider authStoreProvider, server, namespace *string) *cobra.Command {
	taskCmd := &cobra.Command{
		Use:   "task",
		Short: "Inspect and manage STX self-upgrade tasks",
	}

	var wait bool
	var eventsJSON bool
	var timeout time.Duration
	getCmd := &cobra.Command{
		Use:   "get <task-id>",
		Short: "Get an STX self-upgrade task",
		Example: `  stx self-upgrade task get task-123
  stx self-upgrade task get task-123 --wait --timeout 6m`,
		Args: usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			if wait && timeout <= 0 {
				return clioutput.NewError(clioutput.CodeUsage, "--timeout must be greater than zero", clioutput.ExitUsage, false)
			}
			client, err := resolveSelfUpgradeClient(command, storeProvider, *server, *namespace)
			if err != nil {
				return err
			}
			taskID := strings.TrimSpace(args[0])
			path := "/api/v1/self-upgrade/tasks/" + url.PathEscape(taskID)
			if !wait {
				var data any
				requestID, err := client.Request(command.Context(), http.MethodGet, path, nil, &data)
				if err != nil {
					return err
				}
				return renderCommandResultWithRequestID(command, "self_upgrade.task.get", requestID, data)
			}
			return waitSelfUpgradeTask(command, client, path, taskID, timeout)
		},
	}
	getCmd.Flags().BoolVar(&wait, "wait", false, "Poll the task until it reaches a terminal status (succeeded, failed, cancelled)")
	getCmd.Flags().DurationVar(&timeout, "timeout", 6*time.Minute, "Maximum time to wait when --wait is set")
	getCmd.Flags().BoolVar(&eventsJSON, "events-json", false, "Emit --wait progress as NDJSON events on stderr (default: plain human-readable text)")

	var cancelOptions secureWriteOptions
	cancelCmd := &cobra.Command{
		Use:     "cancel <task-id>",
		Short:   "Cancel a running STX self-upgrade task",
		Example: `  stx self-upgrade task cancel task-123 --confirm`,
		Args:    usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			impact := selfUpgradeOperationImpact("self_upgrade.task.cancel", "Cancels a running self-upgrade task when it is still cancellable.")
			client, headers, err := prepareSelfUpgradeWrite(command, storeProvider, *server, *namespace, &cancelOptions, "self_upgrade.task.cancel", impact)
			if err != nil {
				return err
			}
			var data any
			taskID := strings.TrimSpace(args[0])
			requestID, err := client.RequestWithHeaders(command.Context(), http.MethodPost,
				"/api/v1/self-upgrade/tasks/"+url.PathEscape(taskID)+"/cancel", nil, headers, &data)
			if err != nil {
				return handleSecureWriteError(command, "self_upgrade.task.cancel", err)
			}
			return renderWriteResult(command, "self_upgrade.task.cancel", requestID, data, "stx self-upgrade task get "+taskID)
		},
	}
	addSelfUpgradeWriteFlags(cancelCmd, &cancelOptions)

	taskCmd.AddCommand(getCmd, cancelCmd)
	return taskCmd
}

// waitSelfUpgradeTask 轮询自升级任务直到终态；控制面重启期间的连接级错误视为可恢复。
// waitSelfUpgradeTask polls the self-upgrade task until a terminal status; transport-level errors while the
// control plane restarts are treated as recoverable and retried until the timeout.
func waitSelfUpgradeTask(command *cobra.Command, client *cliClient.Client, path, taskID string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(command.Context(), timeout)
	defer cancel()
	events := stderrEventWriter(command)
	lastStatus := ""
	lastRequestID := ""
	for {
		var task selfupgrade.TaskState
		requestID, err := client.Request(ctx, http.MethodGet, path, nil, &task)
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			// 未拿到 HTTP 响应的传输层错误（连接拒绝/重置/超时）在控制面重启期间重试；API 级错误直接返回。
			// Transport-level errors without an HTTP response (refused/reset/timeout) are retried while the
			// control plane restarts; API-level errors are returned immediately.
			var apiErr *cliClient.APIError
			if errors.As(err, &apiErr) {
				return err
			}
			_ = events.Emit(clioutput.Event{
				Event: "self_upgrade_task_unreachable", OperationID: "self_upgrade.task.get", Level: "warning",
				Message: fmt.Sprintf("task %s unreachable, retrying: %v", taskID, err),
			})
			if !sleepSelfUpgradeTaskPoll(ctx) {
				break
			}
			continue
		}
		lastRequestID = requestID
		if task.Status != lastStatus {
			_ = events.Emit(clioutput.Event{
				Event: "self_upgrade_task_status", OperationID: "self_upgrade.task.get", RequestID: requestID,
				Message: task.Status,
			})
			lastStatus = task.Status
		}
		if isSelfUpgradeTaskTerminal(task.Status) {
			if err := renderCommandResultWithRequestID(command, "self_upgrade.task.get", requestID, task); err != nil {
				return err
			}
			if task.Status == selfupgrade.StatusSucceeded {
				return nil
			}
			message := strings.TrimSpace(task.FailureReason)
			if message == "" {
				message = "self-upgrade task finished with status " + task.Status
			}
			return clioutput.NewError(clioutput.CodeExecution, message, clioutput.ExitExecution, false)
		}
		if !sleepSelfUpgradeTaskPoll(ctx) {
			break
		}
	}
	// 超时返回非零并汇报最后观察到的状态。/ Timeout exits non-zero and reports the last observed status.
	message := fmt.Sprintf("timed out waiting for self-upgrade task %s", taskID)
	if lastStatus != "" {
		message += " (last status: " + lastStatus + ", last request: " + lastRequestID + ")"
	}
	return clioutput.NewError(clioutput.CodeTimeout, message, clioutput.ExitTimeout, true)
}

// sleepSelfUpgradeTaskPoll 按轮询间隔休眠，上下文结束时返回 false。
// sleepSelfUpgradeTaskPoll sleeps for the poll interval and returns false when the context ends.
func sleepSelfUpgradeTaskPoll(ctx context.Context) bool {
	timer := time.NewTimer(selfUpgradeTaskPollInterval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// isSelfUpgradeTaskTerminal 判断自升级任务状态是否为终态。
// isSelfUpgradeTaskTerminal reports whether a self-upgrade task status is terminal.
func isSelfUpgradeTaskTerminal(status string) bool {
	switch status {
	case selfupgrade.StatusSucceeded, selfupgrade.StatusFailed, selfupgrade.StatusCancelled:
		return true
	}
	return false
}

// addSelfUpgradeWriteFlags 为自升级写命令补 --confirm/--idempotency-key/--confirmation-id（namespace/server 用父级持久参数）。
// addSelfUpgradeWriteFlags adds --confirm/--idempotency-key/--confirmation-id to self-upgrade write commands
// (namespace/server come from the parent persistent flags).
func addSelfUpgradeWriteFlags(command *cobra.Command, options *secureWriteOptions) {
	command.Flags().BoolVar(&options.confirmed, "confirm", false, "Confirm the operation impact")
	command.Flags().StringVar(&options.idempotencyKey, "idempotency-key", "", "Stable key for retrying the same request")
	command.Flags().StringVar(&options.confirmationID, "confirmation-id", "", "One-time confirmation ID returned by STX")
}

// prepareSelfUpgradeWrite 复用写命令的确认、能力核对与安全请求头流程，但经由自升级命令树的 --server/--namespace 解析客户端。
// prepareSelfUpgradeWrite mirrors the shared write-command confirmation, capability and header flow while
// resolving the client through the self-upgrade command tree's --server/--namespace flags.
func prepareSelfUpgradeWrite(command *cobra.Command, storeProvider authStoreProvider, server, namespace string, options *secureWriteOptions, operationID, impact string) (*cliClient.Client, map[string]string, error) {
	if !options.confirmed {
		return nil, nil, clioutput.NewError(clioutput.CodeConflict, "write operation requires --confirm", clioutput.ExitConflict, false)
	}
	client, err := resolveSelfUpgradeClient(command, storeProvider, server, namespace)
	if err != nil {
		return nil, nil, err
	}
	if err := checkSpecialOperation(command, client, operationID); err != nil {
		return nil, nil, err
	}
	if strings.TrimSpace(options.idempotencyKey) == "" {
		options.idempotencyKey = uuid.NewString()
		_ = stderrEventWriter(command).Emit(clioutput.Event{
			Event: "idempotency_key", OperationID: operationID, Level: "info", Message: options.idempotencyKey,
		})
	}
	_ = stderrEventWriter(command).Emit(clioutput.Event{
		Event: "warning", OperationID: operationID, Level: "warning", Message: impact,
	})
	headers := map[string]string{
		"Idempotency-Key": options.idempotencyKey,
		"X-STX-Confirm":   "true",
	}
	if strings.TrimSpace(options.confirmationID) != "" {
		headers["X-STX-Confirmation-ID"] = options.confirmationID
	}
	return client, headers, nil
}

// selfUpgradeOperationImpact 取操作登记的影响文案，登记缺失时退回兜底文案。
// selfUpgradeOperationImpact returns the registered operation impact message, falling back to a generic text.
func selfUpgradeOperationImpact(operationID, fallback string) string {
	for _, spec := range operation.Registry() {
		if spec.ID == operationID && spec.Impact != nil && strings.TrimSpace(spec.Impact.Message) != "" {
			return spec.Impact.Message
		}
	}
	return fallback
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
