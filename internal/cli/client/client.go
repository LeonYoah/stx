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

// Package client 提供 STX CLI 访问远端 API 的最小 HTTP 客户端。
// Package client provides the minimal HTTP client used by the STX CLI.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"
	"time"

	cliConfig "github.com/LeonYoah/stx/internal/cli/config"
	clioutput "github.com/LeonYoah/stx/internal/cli/output"
	stxversion "github.com/LeonYoah/stx/internal/version"
	"github.com/google/uuid"
)

const (
	clientHeader = "cli"
)

// Client 是带有命名空间配置的远端 STX API 客户端。
// Client is a remote STX API client backed by a resolved namespace configuration.
type Client struct {
	server     string
	token      string
	timeout    time.Duration
	httpClient *http.Client
}

// Response 是 STX API 的通用响应封装。
// Response is the generic STX API response envelope.
type Response struct {
	ErrorCode string          `json:"error_code"`
	ErrorMsg  string          `json:"error_msg"`
	Data      json.RawMessage `json:"data"`
}

// APIError 保存服务端错误的 HTTP 状态和请求编号。
// APIError stores the HTTP status and request ID returned by the server.
type APIError struct {
	StatusCode int
	ErrorCode  string
	Message    string
	RequestID  string
	Data       json.RawMessage
}

// MultipartFile describes one streamed file part in a multipart request.
// MultipartFile 描述 multipart 请求中的一个流式文件字段。
type MultipartFile struct {
	FieldName string
	FileName  string
	Reader    io.Reader
}

func (e *APIError) Error() string {
	if e.RequestID == "" {
		return e.Message
	}
	return fmt.Sprintf("%s (request_id=%s)", e.Message, e.RequestID)
}

// New 创建远端 STX API 客户端，并校验地址和超时。
// New creates a remote STX API client and validates the URL and timeout.
func New(resolved cliConfig.Resolved, httpClient *http.Client) (*Client, error) {
	server := strings.TrimRight(strings.TrimSpace(resolved.Server), "/")
	if server == "" {
		return nil, clioutput.NewError(clioutput.CodeUsage, "STX server is not configured", clioutput.ExitUsage, false)
	}
	parsed, err := url.Parse(server)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, clioutput.NewError(clioutput.CodeUsage, fmt.Sprintf("invalid STX server URL %q", resolved.Server), clioutput.ExitUsage, false)
	}
	timeout := resolved.Timeout
	if timeout <= 0 {
		timeout = cliConfig.DefaultTimeout
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: timeout}
	}
	return &Client{server: server, token: strings.TrimSpace(resolved.Token), timeout: timeout, httpClient: httpClient}, nil
}

// Server 返回客户端当前使用的服务地址，不包含令牌。
// Server returns the server URL used by this client without exposing the token.
func (c *Client) Server() string {
	return c.server
}

// WithTimeout returns an isolated client copy for one long-running command.
// WithTimeout 返回仅供单个长耗时命令使用的独立客户端副本。
func (c *Client) WithTimeout(timeout time.Duration) *Client {
	if c == nil || timeout <= 0 {
		return c
	}
	copyClient := *c
	copyClient.timeout = timeout
	if c.httpClient != nil {
		copyHTTPClient := *c.httpClient
		copyHTTPClient.Timeout = timeout
		copyClient.httpClient = &copyHTTPClient
	}
	return &copyClient
}

// Request 向远端 STX API 发起 JSON 请求。
// Request sends a JSON request to the remote STX API.
func (c *Client) Request(ctx context.Context, method, path string, requestBody any, result any) (string, error) {
	return c.request(ctx, method, path, requestBody, nil, result)
}

// RequestWithHeaders 向远端 STX API 发起带安全执行请求头的 JSON 请求。
// RequestWithHeaders sends a JSON request with safe-execution headers to the remote STX API.
func (c *Client) RequestWithHeaders(ctx context.Context, method, path string, requestBody any, headers map[string]string, result any) (string, error) {
	return c.request(ctx, method, path, requestBody, headers, result)
}

// RequestMultipart streams a multipart request without buffering the uploaded file in memory.
// RequestMultipart 以流式方式发送 multipart 请求，不把上传文件完整读入内存。
func (c *Client) RequestMultipart(ctx context.Context, method, path string, fields map[string]string, fileField, fileName string, file io.Reader, headers map[string]string, result any) (string, error) {
	return c.RequestMultipartFiles(ctx, method, path, fields, []MultipartFile{{FieldName: fileField, FileName: fileName, Reader: file}}, headers, result)
}

// RequestMultipartFiles streams multiple file fields without buffering complete files in memory.
// RequestMultipartFiles 流式发送多个文件字段，不把完整文件读入内存。
func (c *Client) RequestMultipartFiles(ctx context.Context, method, path string, fields map[string]string, files []MultipartFile, headers map[string]string, result any) (string, error) {
	if len(files) == 0 {
		return "", clioutput.NewError(clioutput.CodeUsage, "upload file is required", clioutput.ExitUsage, false)
	}
	pipeReader, pipeWriter := io.Pipe()
	writer := multipart.NewWriter(pipeWriter)
	go func() {
		for name, value := range fields {
			if err := writer.WriteField(name, value); err != nil {
				_ = pipeWriter.CloseWithError(err)
				return
			}
		}
		for _, file := range files {
			if file.Reader == nil || strings.TrimSpace(file.FieldName) == "" {
				_ = pipeWriter.CloseWithError(errors.New("multipart file reader and field name are required"))
				return
			}
			part, err := writer.CreateFormFile(file.FieldName, file.FileName)
			if err != nil {
				_ = pipeWriter.CloseWithError(err)
				return
			}
			if _, err := io.Copy(part, file.Reader); err != nil {
				_ = pipeWriter.CloseWithError(err)
				return
			}
		}
		if err := writer.Close(); err != nil {
			_ = pipeWriter.CloseWithError(err)
			return
		}
		_ = pipeWriter.Close()
	}()
	return c.requestReader(ctx, method, path, pipeReader, writer.FormDataContentType(), headers, result)
}

// Download streams a successful response body to the supplied writer.
// Download 将成功响应正文流式写入调用方提供的 writer。
func (c *Client) Download(ctx context.Context, path string, destination io.Writer) (string, error) {
	if destination == nil {
		return "", clioutput.NewError(clioutput.CodeUsage, "download destination is required", clioutput.ExitUsage, false)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint(path), nil)
	if err != nil {
		return "", clioutput.WrapError(err, clioutput.CodeUsage, "build HTTP request", clioutput.ExitUsage, false)
	}
	requestID := uuid.NewString()
	request.Header.Set("Accept", "application/octet-stream")
	request.Header.Set("X-STX-Client", clientHeader)
	request.Header.Set("User-Agent", "stx-cli/"+stxversion.Version)
	request.Header.Set("X-Request-ID", requestID)
	if c.token != "" {
		request.Header.Set("Authorization", "Bearer "+c.token)
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return requestID, clioutput.WrapError(err, clioutput.CodeNetwork, "cannot reach STX server", clioutput.ExitNetwork, true)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		content, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
		envelope, _, legacyError, _ := decodeResponseEnvelope(bytes.TrimSpace(content))
		message := strings.TrimSpace(envelope.ErrorMsg)
		if message == "" {
			message = strings.TrimSpace(legacyError)
		}
		if message == "" {
			message = http.StatusText(response.StatusCode)
		}
		return requestID, classifyHTTPError(response.StatusCode, envelope.ErrorCode, message, requestID, envelope.Data)
	}
	if _, err := io.Copy(destination, response.Body); err != nil {
		return requestID, clioutput.WrapError(err, clioutput.CodeFileTransfer, "download source package", clioutput.ExitFileTransfer, true)
	}
	return requestID, nil
}

func (c *Client) request(ctx context.Context, method, path string, requestBody any, headers map[string]string, result any) (string, error) {
	var body io.Reader
	contentType := ""
	if requestBody != nil {
		content, err := json.Marshal(requestBody)
		if err != nil {
			return "", clioutput.WrapError(err, clioutput.CodeUsage, "encode request body", clioutput.ExitUsage, false)
		}
		body = bytes.NewReader(content)
		contentType = "application/json"
	}
	return c.requestReader(ctx, method, path, body, contentType, headers, result)
}

func (c *Client) requestReader(ctx context.Context, method, path string, body io.Reader, contentType string, headers map[string]string, result any) (string, error) {
	if c == nil || c.httpClient == nil {
		return "", errors.New("STX client is not initialized")
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	endpoint := c.endpoint(path)
	request, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return "", clioutput.WrapError(err, clioutput.CodeUsage, "build HTTP request", clioutput.ExitUsage, false)
	}
	requestID := uuid.NewString()
	request.Header.Set("Accept", "application/json")
	request.Header.Set("X-STX-Client", clientHeader)
	request.Header.Set("User-Agent", "stx-cli/"+stxversion.Version)
	request.Header.Set("X-Request-ID", requestID)
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	if c.token != "" {
		request.Header.Set("Authorization", "Bearer "+c.token)
	}
	for name, value := range headers {
		switch http.CanonicalHeaderKey(name) {
		case "Idempotency-Key", "X-Stx-Confirm", "X-Stx-Confirmation-Id":
			request.Header.Set(name, value)
		}
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return requestID, clioutput.WrapError(err, clioutput.CodeTimeout, "STX request timed out", clioutput.ExitTimeout, true)
		}
		if c.timeout > 0 {
			var netErr interface{ Timeout() bool }
			if errors.As(err, &netErr) && netErr.Timeout() {
				return requestID, clioutput.WrapError(err, clioutput.CodeTimeout, "STX request timed out", clioutput.ExitTimeout, true)
			}
		}
		return requestID, clioutput.WrapError(err, clioutput.CodeNetwork, "cannot reach STX server", clioutput.ExitNetwork, true)
	}
	defer response.Body.Close()

	content, readErr := io.ReadAll(response.Body)
	if readErr != nil {
		return requestID, clioutput.WrapError(readErr, clioutput.CodeNetwork, "read STX response", clioutput.ExitNetwork, true)
	}
	trimmedContent := bytes.TrimSpace(content)
	if len(trimmedContent) > 0 && !json.Valid(trimmedContent) && response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		return requestID, clioutput.NewError(clioutput.CodeServer, "invalid STX response", clioutput.ExitServer, false)
	}
	envelope, envelopeResponse, legacyError, decodeErr := decodeResponseEnvelope(trimmedContent)
	if decodeErr != nil && response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		return requestID, clioutput.WrapError(decodeErr, clioutput.CodeServer, "invalid STX response", clioutput.ExitServer, false)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		message := strings.TrimSpace(envelope.ErrorMsg)
		if message == "" {
			message = strings.TrimSpace(legacyError)
		}
		if message == "" {
			message = http.StatusText(response.StatusCode)
		}
		return requestID, classifyHTTPError(response.StatusCode, envelope.ErrorCode, message, requestID, envelope.Data)
	}
	if result != nil {
		payload := trimmedContent
		if envelopeResponse {
			payload = envelope.Data
		}
		if len(payload) > 0 && string(payload) != "null" {
			if err := json.Unmarshal(payload, result); err != nil {
				return requestID, clioutput.WrapError(err, clioutput.CodeServer, "decode STX response", clioutput.ExitServer, false)
			}
		}
	}
	return requestID, nil
}

// decodeResponseEnvelope 识别通用响应外层，同时兼容仍返回裸 JSON 的遗留接口。
// decodeResponseEnvelope detects the common response envelope while supporting legacy APIs that still return bare JSON.
func decodeResponseEnvelope(content []byte) (Response, bool, string, error) {
	if len(content) == 0 {
		return Response{}, false, "", nil
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(content, &fields); err != nil {
		return Response{}, false, "", nil
	}
	_, hasData := fields["data"]
	_, hasErrorCode := fields["error_code"]
	_, hasErrorMsg := fields["error_msg"]
	envelopeResponse := hasData || hasErrorCode || hasErrorMsg
	var envelope Response
	if envelopeResponse {
		if err := json.Unmarshal(content, &envelope); err != nil {
			return Response{}, true, "", err
		}
	}
	var legacyError string
	if raw, exists := fields["error"]; exists {
		if err := json.Unmarshal(raw, &legacyError); err != nil {
			legacyError = ""
		}
	}
	return envelope, envelopeResponse, legacyError, nil
}

// Login 调用 CLI 登录接口。
// Login calls the CLI login endpoint.
func (c *Client) Login(ctx context.Context, username, password, expiresIn string) (string, LoginData, error) {
	var data LoginData
	requestID, err := c.Request(ctx, http.MethodPost, "/api/v1/auth/cli/login", map[string]string{
		"username":   username,
		"password":   password,
		"expires_in": expiresIn,
	}, &data)
	return requestID, data, err
}

// Logout 撤销当前令牌。
// Logout revokes the current token.
func (c *Client) Logout(ctx context.Context) (string, error) {
	return c.Request(ctx, http.MethodPost, "/api/v1/auth/cli/logout", nil, nil)
}

// WhoAmI 查询当前令牌对应的用户。
// WhoAmI returns the user associated with the current token.
func (c *Client) WhoAmI(ctx context.Context) (string, UserInfo, error) {
	var data UserInfo
	requestID, err := c.Request(ctx, http.MethodGet, "/api/v1/auth/cli/whoami", nil, &data)
	return requestID, data, err
}

// LoginData 是 CLI 登录接口返回的数据。
// LoginData is returned by the CLI login endpoint.
type LoginData struct {
	Token     string    `json:"token"`
	TokenType string    `json:"token_type"`
	ExpiresAt time.Time `json:"expires_at"`
	User      UserInfo  `json:"user"`
}

// UserInfo 是 CLI 所需的用户信息子集。
// UserInfo is the user information subset needed by the CLI.
type UserInfo struct {
	ID       uint64 `json:"id"`
	Username string `json:"username"`
	Nickname string `json:"nickname"`
	Email    string `json:"email"`
	Language string `json:"language"`
	IsActive bool   `json:"is_active"`
	IsAdmin  bool   `json:"is_admin"`
}

// CapabilityData 是服务端能力查询结果。
// CapabilityData is the server capability discovery result.
type CapabilityData struct {
	APIVersion       string                `json:"api_version"`
	ServerVersion    string                `json:"server_version"`
	MinCLIVersion    string                `json:"min_cli_version"`
	RegistryRevision string                `json:"registry_revision"`
	Operations       []CapabilityOperation `json:"operations"`
}

// CapabilityOperation 描述当前用户是否可调用一个服务端操作。
// CapabilityOperation describes whether the current user may invoke a server operation.
type CapabilityOperation struct {
	OperationID string         `json:"operation_id"`
	Revision    int            `json:"revision"`
	Allowed     bool           `json:"allowed"`
	DenialCode  string         `json:"denial_code,omitempty"`
	Mode        string         `json:"mode"`
	Risk        string         `json:"risk"`
	Impact      map[string]any `json:"impact,omitempty"`
}

// Capabilities 查询当前服务端及用户可用的操作。
// Capabilities queries operations available on the server for the current user.
func (c *Client) Capabilities(ctx context.Context) (string, CapabilityData, error) {
	var data CapabilityData
	requestID, err := c.Request(ctx, http.MethodGet, "/api/v1/capabilities", nil, &data)
	return requestID, data, err
}

// Health 查询 STX 服务健康状态。
// Health queries the STX server health status.
func (c *Client) Health(ctx context.Context) (string, map[string]any, error) {
	data := make(map[string]any)
	requestID, err := c.Request(ctx, http.MethodGet, "/api/v1/health", nil, &data)
	return requestID, data, err
}

// ExecutionData 是 CLI 使用的公共执行记录。
// ExecutionData is the shared execution record consumed by the CLI.
type ExecutionData struct {
	ExecutionID       string     `json:"execution_id"`
	OperationID       string     `json:"operation_id"`
	OwnerUserID       uint64     `json:"owner_user_id"`
	ActorType         string     `json:"actor_type"`
	Module            string     `json:"module"`
	ModuleRef         string     `json:"module_ref"`
	RequestID         string     `json:"request_id"`
	RiskLevel         string     `json:"risk_level"`
	Status            string     `json:"status"`
	Cancellable       bool       `json:"cancellable"`
	CancellableReason string     `json:"cancellable_reason,omitempty"`
	Progress          int        `json:"progress"`
	ResultRef         string     `json:"result_ref,omitempty"`
	ErrorCode         string     `json:"error_code,omitempty"`
	ErrorMessage      string     `json:"error_message,omitempty"`
	StartedAt         *time.Time `json:"started_at,omitempty"`
	FinishedAt        *time.Time `json:"finished_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// ExecutionWaitData 是公共等待接口的返回结果。
// ExecutionWaitData is returned by the shared execution wait endpoint.
type ExecutionWaitData struct {
	Execution ExecutionData `json:"execution"`
	TimedOut  bool          `json:"wait_timed_out"`
}

// GetExecution 查询公共执行记录。
// GetExecution retrieves a shared execution record.
func (c *Client) GetExecution(ctx context.Context, executionID string) (string, ExecutionData, error) {
	var data ExecutionData
	requestID, err := c.Request(ctx, http.MethodGet, "/api/v1/executions/"+url.PathEscape(executionID), nil, &data)
	return requestID, data, err
}

// WaitExecution 在服务端等待一段时间并返回最新公共执行状态。
// WaitExecution waits on the server for a bounded period and returns the latest shared execution state.
func (c *Client) WaitExecution(ctx context.Context, executionID string, timeoutSeconds int) (string, ExecutionWaitData, error) {
	var data ExecutionWaitData
	path := fmt.Sprintf("/api/v1/executions/%s/wait?timeout_seconds=%d", url.PathEscape(executionID), timeoutSeconds)
	requestID, err := c.Request(ctx, http.MethodGet, path, nil, &data)
	return requestID, data, err
}

// CancelExecution 请求取消公共执行记录。
// CancelExecution requests cancellation of a shared execution record.
func (c *Client) CancelExecution(ctx context.Context, executionID, idempotencyKey string, confirmed bool) (string, ExecutionData, error) {
	var data ExecutionData
	headers := map[string]string{"Idempotency-Key": idempotencyKey}
	if confirmed {
		headers["X-STX-Confirm"] = "true"
	}
	requestID, err := c.RequestWithHeaders(ctx, http.MethodPost, "/api/v1/executions/"+url.PathEscape(executionID)+"/cancel", nil, headers, &data)
	return requestID, data, err
}

func (c *Client) endpoint(path string) string {
	server := strings.TrimRight(c.server, "/")
	if strings.HasSuffix(server, "/api") {
		return server + strings.TrimPrefix(path, "/api")
	}
	return server + path
}

func classifyHTTPError(status int, errorCode, message, requestID string, data json.RawMessage) *clioutput.CLIError {
	cause := &APIError{StatusCode: status, ErrorCode: errorCode, Message: message, RequestID: requestID, Data: data}
	switch status {
	case http.StatusUnauthorized:
		return &clioutput.CLIError{Code: clioutput.CodeAuthentication, Message: message, ExitCode: clioutput.ExitAuthentication, RequestID: requestID, Cause: cause}
	case http.StatusForbidden:
		return &clioutput.CLIError{Code: clioutput.CodePermission, Message: message, ExitCode: clioutput.ExitPermission, RequestID: requestID, Cause: cause}
	case http.StatusNotFound:
		return &clioutput.CLIError{Code: clioutput.CodeNotFound, Message: message, ExitCode: clioutput.ExitNotFound, RequestID: requestID, Cause: cause}
	case http.StatusConflict, http.StatusPreconditionRequired:
		return &clioutput.CLIError{Code: clioutput.CodeConflict, Message: message, ExitCode: clioutput.ExitConflict, RequestID: requestID, Cause: cause}
	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		return &clioutput.CLIError{Code: clioutput.CodeTimeout, Message: message, ExitCode: clioutput.ExitTimeout, Retryable: true, RequestID: requestID, Cause: cause}
	default:
		retryable := status >= http.StatusInternalServerError
		return &clioutput.CLIError{Code: clioutput.CodeServer, Message: message, ExitCode: clioutput.ExitServer, Retryable: retryable, RequestID: requestID, Cause: cause}
	}
}
