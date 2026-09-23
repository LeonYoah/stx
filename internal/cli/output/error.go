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
package output

import (
	"context"
	"errors"
	"net"
)

// ExitCode 是 STX CLI 的稳定进程退出码。
// ExitCode is a stable STX CLI process exit code.
type ExitCode int

const (
	ExitSuccess        ExitCode = 0
	ExitUsage          ExitCode = 2
	ExitAuthentication ExitCode = 3
	ExitPermission     ExitCode = 4
	ExitNotFound       ExitCode = 5
	ExitConflict       ExitCode = 6
	ExitNetwork        ExitCode = 7
	ExitTimeout        ExitCode = 8
	ExitServer         ExitCode = 9
	ExitExecution      ExitCode = 10
	ExitFileTransfer   ExitCode = 11
)

const (
	CodeUsage          = "usage_error"
	CodeAuthentication = "authentication_required"
	CodePermission     = "permission_denied"
	CodeNotFound       = "not_found"
	CodeConflict       = "conflict"
	CodeNetwork        = "network_error"
	CodeTimeout        = "timeout"
	CodeServer         = "server_error"
	CodeExecution      = "execution_failed"
	CodeFileTransfer   = "file_transfer_failed"
)

// CLIError 保存可稳定输出和映射退出码的错误信息。
// CLIError stores error information that can be rendered and mapped to a stable exit code.
type CLIError struct {
	Code      string
	Message   string
	Retryable bool
	RequestID string
	ExitCode  ExitCode
	Cause     error
}

func (e *CLIError) Error() string {
	return e.Message
}

func (e *CLIError) Unwrap() error {
	return e.Cause
}

// NewError 创建带有稳定分类的 CLI 错误。
// NewError creates a CLI error with a stable classification.
func NewError(code, message string, exitCode ExitCode, retryable bool) *CLIError {
	return &CLIError{Code: code, Message: message, ExitCode: exitCode, Retryable: retryable}
}

// WrapError 创建保留原始原因的 CLI 错误。
// WrapError creates a CLI error that retains its original cause.
func WrapError(err error, code, message string, exitCode ExitCode, retryable bool) *CLIError {
	return &CLIError{Code: code, Message: message, ExitCode: exitCode, Retryable: retryable, Cause: err}
}

// ClassifyError 将未知错误转换为稳定的 CLI 错误分类。
// ClassifyError converts an unknown error into a stable CLI error classification.
func ClassifyError(err error) *CLIError {
	if err == nil {
		return nil
	}
	var cliErr *CLIError
	if errors.As(err, &cliErr) {
		return cliErr
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return WrapError(err, CodeTimeout, err.Error(), ExitTimeout, true)
	}
	var networkErr net.Error
	if errors.As(err, &networkErr) {
		return WrapError(err, CodeNetwork, err.Error(), ExitNetwork, networkErr.Timeout() || networkErr.Temporary())
	}
	return WrapError(err, CodeServer, err.Error(), ExitServer, false)
}
