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
	"fmt"
	"strings"
	"time"

	cliClient "github.com/LeonYoah/stx/internal/cli/client"
	cliConfig "github.com/LeonYoah/stx/internal/cli/config"
	clioutput "github.com/LeonYoah/stx/internal/cli/output"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

const executionWaitChunkSeconds = 25

func newExecutionCommand() *cobra.Command {
	return newExecutionCommandWithStore(cliConfig.NewDefaultStore)
}

func newExecutionCommandWithStore(storeProvider authStoreProvider) *cobra.Command {
	command := &cobra.Command{
		Use:   "execution",
		Short: "Inspect and cancel remote STX executions",
		Args:  usageArgs(cobra.NoArgs),
	}
	command.AddCommand(
		newExecutionGetCommand(storeProvider),
		newExecutionWaitCommand(storeProvider),
		newExecutionCancelCommand(storeProvider),
	)
	return command
}

func newExecutionGetCommand(storeProvider authStoreProvider) *cobra.Command {
	var namespace string
	command := &cobra.Command{
		Use:   "get <execution-id>",
		Short: "Get one remote execution",
		Args:  usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			client, err := clientForNamespace(storeProvider, namespace)
			if err != nil {
				return err
			}
			requestID, data, err := client.GetExecution(command.Context(), args[0])
			if err != nil {
				return err
			}
			return renderExecutionResult(command, "execution.get", requestID, data)
		},
	}
	command.Flags().StringVar(&namespace, "namespace", "", "Local namespace to use")
	return command
}

func newExecutionWaitCommand(storeProvider authStoreProvider) *cobra.Command {
	var namespace string
	var timeout time.Duration
	command := &cobra.Command{
		Use:   "wait <execution-id>",
		Short: "Wait for a remote execution to finish",
		Args:  usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			if timeout <= 0 {
				return clioutput.NewError(clioutput.CodeUsage, "--timeout must be greater than zero", clioutput.ExitUsage, false)
			}
			client, err := clientForNamespace(storeProvider, namespace)
			if err != nil {
				return err
			}
			executionID := strings.TrimSpace(args[0])
			ctx, cancel := context.WithTimeout(command.Context(), timeout)
			defer cancel()
			events := clioutput.NewEventWriter(command.ErrOrStderr())
			lastStatus := ""
			lastProgress := -1
			for {
				requestID, data, err := client.WaitExecution(ctx, executionID, executionWaitChunkSeconds)
				if err != nil {
					return err
				}
				item := data.Execution
				if item.Status != lastStatus || item.Progress != lastProgress {
					progress := item.Progress
					if err := events.Emit(clioutput.Event{
						Event:       "execution_progress",
						OperationID: "execution.wait",
						RequestID:   requestID,
						ExecutionID: item.ExecutionID,
						Message:     item.Status,
						Progress:    &progress,
					}); err != nil {
						return clioutput.WrapError(err, clioutput.CodeServer, "write execution progress event", clioutput.ExitServer, false)
					}
					lastStatus, lastProgress = item.Status, item.Progress
				}
				if executionStatusTerminal(item.Status) {
					if err := renderExecutionResult(command, "execution.wait", requestID, item); err != nil {
						return err
					}
					if item.Status != "succeeded" {
						message := item.ErrorMessage
						if strings.TrimSpace(message) == "" {
							message = "execution finished with status " + item.Status
						}
						return clioutput.NewError(clioutput.CodeExecution, message, clioutput.ExitExecution, false)
					}
					return nil
				}
			}
		},
	}
	command.Flags().StringVar(&namespace, "namespace", "", "Local namespace to use")
	command.Flags().DurationVar(&timeout, "timeout", 10*time.Minute, "Maximum time to wait")
	return command
}

func newExecutionCancelCommand(storeProvider authStoreProvider) *cobra.Command {
	var namespace string
	var confirmed bool
	var idempotencyKey string
	command := &cobra.Command{
		Use:   "cancel <execution-id>",
		Short: "Request cancellation of a remote execution",
		Args:  usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			client, err := clientForNamespace(storeProvider, namespace)
			if err != nil {
				return err
			}
			if strings.TrimSpace(idempotencyKey) == "" {
				idempotencyKey = uuid.NewString()
			}
			requestID, data, err := client.CancelExecution(command.Context(), args[0], idempotencyKey, confirmed)
			if err != nil {
				return err
			}
			return renderExecutionResult(command, "execution.cancel", requestID, data)
		},
	}
	command.Flags().StringVar(&namespace, "namespace", "", "Local namespace to use")
	command.Flags().BoolVar(&confirmed, "confirm", false, "Confirm the cancellation impact")
	command.Flags().StringVar(&idempotencyKey, "idempotency-key", "", "Stable key for retrying the same cancellation request")
	return command
}

func renderExecutionResult(command *cobra.Command, operationID, requestID string, data cliClient.ExecutionData) error {
	options, err := clioutput.OptionsFromCommand(command)
	if err != nil {
		return err
	}
	result := clioutput.NewResult(operationID, requestID, data)
	if !executionStatusTerminal(data.Status) {
		result.ResultMeta.NextCommand = fmt.Sprintf("stx execution wait %s", data.ExecutionID)
	}
	renderer := clioutput.NewRenderer(command.OutOrStdout(), clioutput.NewEventWriter(command.ErrOrStderr()))
	return renderer.Render(result, options)
}

func executionStatusTerminal(status string) bool {
	switch status {
	case "cancelled", "succeeded", "failed", "timed_out":
		return true
	default:
		return false
	}
}
