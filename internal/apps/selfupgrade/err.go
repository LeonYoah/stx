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

package selfupgrade

import "errors"

// Typed error codes returned to API clients. / 返回给 API 客户端的错误码。
const (
	CodeCheckFail           = "check_fail"
	CodeAlreadyLatest       = "already_latest"
	CodeDownloadPartial     = "download_partial"
	CodeBadOfflineImport    = "bad_offline_import"
	CodeForbiddenNonAdmin   = "forbidden_non_admin"
	CodeStepFailControlPlane = "step_fail_control_plane"
	CodeStepFailAgent       = "step_fail_agent"
	CodeStepFailProxy       = "step_fail_proxy"
	CodeVerifyFail          = "verify_fail"
	CodeConcurrentUpgrade   = "concurrent_upgrade"
	CodeDiskFull            = "disk_full"
	CodeCancelled           = "cancelled"
	CodeNotStaged           = "not_staged"
	CodeInvalidRequest      = "invalid_request"
)

var (
	ErrCheckFail            = errors.New(CodeCheckFail)
	ErrAlreadyLatest        = errors.New(CodeAlreadyLatest)
	ErrDownloadPartial      = errors.New(CodeDownloadPartial)
	ErrBadOfflineImport     = errors.New(CodeBadOfflineImport)
	ErrForbiddenNonAdmin    = errors.New(CodeForbiddenNonAdmin)
	ErrStepFailControlPlane = errors.New(CodeStepFailControlPlane)
	ErrStepFailAgent        = errors.New(CodeStepFailAgent)
	ErrStepFailProxy        = errors.New(CodeStepFailProxy)
	ErrVerifyFail           = errors.New(CodeVerifyFail)
	ErrConcurrentUpgrade    = errors.New(CodeConcurrentUpgrade)
	ErrDiskFull             = errors.New(CodeDiskFull)
	ErrCancelled            = errors.New(CodeCancelled)
	ErrNotStaged            = errors.New(CodeNotStaged)
)

// ErrorCode extracts a stable code from an error. / 从错误中提取稳定错误码。
func ErrorCode(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, ErrCheckFail):
		return CodeCheckFail
	case errors.Is(err, ErrAlreadyLatest):
		return CodeAlreadyLatest
	case errors.Is(err, ErrDownloadPartial):
		return CodeDownloadPartial
	case errors.Is(err, ErrBadOfflineImport):
		return CodeBadOfflineImport
	case errors.Is(err, ErrForbiddenNonAdmin):
		return CodeForbiddenNonAdmin
	case errors.Is(err, ErrStepFailControlPlane):
		return CodeStepFailControlPlane
	case errors.Is(err, ErrStepFailAgent):
		return CodeStepFailAgent
	case errors.Is(err, ErrStepFailProxy):
		return CodeStepFailProxy
	case errors.Is(err, ErrVerifyFail):
		return CodeVerifyFail
	case errors.Is(err, ErrConcurrentUpgrade):
		return CodeConcurrentUpgrade
	case errors.Is(err, ErrDiskFull):
		return CodeDiskFull
	case errors.Is(err, ErrCancelled):
		return CodeCancelled
	case errors.Is(err, ErrNotStaged):
		return CodeNotStaged
	default:
		return CodeInvalidRequest
	}
}
