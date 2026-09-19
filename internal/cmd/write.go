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
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	cliClient "github.com/LeonYoah/stx/internal/cli/client"
	clioutput "github.com/LeonYoah/stx/internal/cli/output"
	"github.com/LeonYoah/stx/internal/operation"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

// secureWriteOptions 保存写命令共用的命名空间、确认和幂等参数。
// secureWriteOptions stores namespace, confirmation, and idempotency options shared by write commands.
type secureWriteOptions struct {
	namespace      string
	confirmed      bool
	idempotencyKey string
	confirmationID string
}

// handleSecureWriteError 在最终错误前补充一次性确认信息。
// handleSecureWriteError emits one-time confirmation details before the final error event.
func handleSecureWriteError(command *cobra.Command, operationID string, err error) error {
	var apiErr *cliClient.APIError
	if !errors.As(err, &apiErr) || len(apiErr.Data) == 0 {
		return err
	}
	if apiErr.ErrorCode != "confirmation_required" && apiErr.StatusCode != http.StatusPreconditionRequired {
		return err
	}
	var data struct {
		ConfirmationID string `json:"confirmation_id"`
		RiskLevel      string `json:"risk_level"`
		Impact         string `json:"impact"`
		ExpiresAt      string `json:"expires_at"`
	}
	if decodeErr := json.Unmarshal(apiErr.Data, &data); decodeErr != nil || strings.TrimSpace(data.ConfirmationID) == "" {
		return err
	}
	_ = clioutput.NewEventWriter(command.ErrOrStderr()).Emit(clioutput.Event{
		Event:          "confirmation_required",
		OperationID:    operationID,
		RequestID:      apiErr.RequestID,
		Level:          "warning",
		Message:        data.Impact,
		ConfirmationID: data.ConfirmationID,
		RiskLevel:      data.RiskLevel,
		ExpiresAt:      data.ExpiresAt,
	})
	return err
}

// addSecureWriteFlags 为写命令增加统一的安全参数。
// addSecureWriteFlags adds the common safety flags to a write command.
func addSecureWriteFlags(command *cobra.Command, options *secureWriteOptions) {
	command.Flags().StringVar(&options.namespace, "namespace", "", "Local namespace to use")
	command.Flags().BoolVar(&options.confirmed, "confirm", false, "Confirm the operation impact")
	command.Flags().StringVar(&options.idempotencyKey, "idempotency-key", "", "Stable key for retrying the same request")
	command.Flags().StringVar(&options.confirmationID, "confirmation-id", "", "One-time confirmation ID returned by STX")
}

// prepareSecureWrite 完成写命令的本地确认、能力检查和请求头构造。
// prepareSecureWrite performs local confirmation, capability checks, and request-header construction for a write command.
func prepareSecureWrite(command *cobra.Command, storeProvider authStoreProvider, operationID string, options *secureWriteOptions, impact string) (*cliClient.Client, map[string]string, error) {
	if !options.confirmed {
		return nil, nil, clioutput.NewError(clioutput.CodeConflict, "write operation requires --confirm", clioutput.ExitConflict, false)
	}
	client, err := clientForNamespace(storeProvider, options.namespace)
	if err != nil {
		return nil, nil, err
	}
	if err := checkSpecialOperation(command.Context(), client, operationID); err != nil {
		return nil, nil, err
	}
	if strings.TrimSpace(options.idempotencyKey) == "" {
		options.idempotencyKey = uuid.NewString()
		_ = clioutput.NewEventWriter(command.ErrOrStderr()).Emit(clioutput.Event{
			Event: "idempotency_key", OperationID: operationID, Level: "info", Message: options.idempotencyKey,
		})
	}
	_ = clioutput.NewEventWriter(command.ErrOrStderr()).Emit(clioutput.Event{
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

// checkSpecialOperation 在调用专用写接口前核对服务端能力和登记版本。
// checkSpecialOperation verifies remote capability and registry compatibility before calling a dedicated write endpoint.
func checkSpecialOperation(ctx context.Context, client *cliClient.Client, operationID string) error {
	var local *operation.OperationSpec
	for _, item := range operation.Registry() {
		if item.ID == operationID {
			copy := item
			local = &copy
			break
		}
	}
	if local == nil {
		return clioutput.NewError(clioutput.CodeServer, "local operation is not registered: "+operationID, clioutput.ExitServer, false)
	}
	requestID, capabilities, err := client.Capabilities(ctx)
	if err != nil {
		return err
	}
	for _, remote := range capabilities.Operations {
		if remote.OperationID != operationID {
			continue
		}
		if !remote.Allowed {
			return &clioutput.CLIError{Code: clioutput.CodePermission, Message: "operation is not allowed: " + remote.DenialCode, ExitCode: clioutput.ExitPermission, RequestID: requestID}
		}
		if remote.Revision < local.Revision || remote.Mode != string(local.Mode) {
			return &clioutput.CLIError{Code: clioutput.CodeConflict, Message: "operation revision or mode is incompatible", ExitCode: clioutput.ExitConflict, RequestID: requestID}
		}
		return nil
	}
	return &clioutput.CLIError{Code: clioutput.CodeNotFound, Message: "operation is not supported by the remote STX server: " + operationID, ExitCode: clioutput.ExitNotFound, RequestID: requestID}
}

// renderWriteResult 使用公共结果格式输出写命令结果。
// renderWriteResult renders a write-command result through the common result envelope.
func renderWriteResult(command *cobra.Command, operationID, requestID string, data any, nextCommand string) error {
	options, err := clioutput.OptionsFromCommand(command)
	if err != nil {
		return err
	}
	result := clioutput.NewResult(operationID, requestID, data)
	result.ResultMeta.NextCommand = nextCommand
	renderer := clioutput.NewRenderer(command.OutOrStdout(), clioutput.NewEventWriter(command.ErrOrStderr()))
	return renderer.Render(result, options)
}
