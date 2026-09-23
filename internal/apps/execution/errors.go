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

package execution

import "errors"

var (
	ErrExecutionNotFound     = errors.New("execution not found")
	ErrPermissionDenied      = errors.New("permission denied")
	ErrInvalidTransition     = errors.New("invalid execution status transition")
	ErrConcurrentUpdate      = errors.New("execution status changed concurrently")
	ErrIdempotencyConflict   = errors.New("idempotency key was already used with a different request")
	ErrIdempotencyKeyMissing = errors.New("idempotency key is required")
	ErrConfirmationRequired  = errors.New("operation confirmation is required")
	ErrConfirmationInvalid   = errors.New("operation confirmation is invalid or expired")
	ErrExplicitConfirmNeeded = errors.New("explicit confirmation is required")
	ErrAdminRequired         = errors.New("administrator permission is required")
	ErrNotCancellable        = errors.New("execution is not cancellable")
	ErrProviderNotRegistered = errors.New("execution cancellation provider is not registered")
	ErrInvalidExecution      = errors.New("invalid execution")
)

const (
	ErrorCodeNotFound             = "execution_not_found"
	ErrorCodePermissionDenied     = "permission_denied"
	ErrorCodeInvalidTransition    = "invalid_transition"
	ErrorCodeConcurrentUpdate     = "concurrent_update"
	ErrorCodeIdempotencyConflict  = "idempotency_conflict"
	ErrorCodeIdempotencyRequired  = "idempotency_key_required"
	ErrorCodeConfirmationRequired = "confirmation_required"
	ErrorCodeConfirmationInvalid  = "confirmation_invalid"
	ErrorCodeAdminRequired        = "admin_required"
	ErrorCodeNotCancellable       = "not_cancellable"
)
