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

// Package executor provides command execution functionality for the Agent.
// executor 包提供 Agent 的命令执行功能。
//
// The CommandExecutor is responsible for:
// CommandExecutor 负责：
// - Routing commands to appropriate handlers based on type / 根据类型将命令路由到适当的处理器
// - Managing command execution lifecycle / 管理命令执行生命周期
// - Reporting progress and results / 上报进度和结果
// - Integrating with InstallerManager and ProcessManager / 与 InstallerManager 和 ProcessManager 集成
package executor

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	pb "github.com/LeonYoah/stx/agent"
)

// Common errors for command execution
// 命令执行的常见错误
var (
	// ErrUnknownCommandType indicates an unrecognized command type
	// ErrUnknownCommandType 表示无法识别的命令类型
	ErrUnknownCommandType = errors.New("unknown command type")

	// ErrCommandTimeout indicates the command execution timed out
	// ErrCommandTimeout 表示命令执行超时
	ErrCommandTimeout = errors.New("command execution timed out")

	// ErrCommandCancelled indicates the command was cancelled
	// ErrCommandCancelled 表示命令被取消
	ErrCommandCancelled = errors.New("command execution cancelled")

	// ErrHandlerNotRegistered indicates no handler is registered for the command type
	// ErrHandlerNotRegistered 表示没有为该命令类型注册处理器
	ErrHandlerNotRegistered = errors.New("no handler registered for command type")

	// ErrExecutorNotInitialized indicates the executor is not properly initialized
	// ErrExecutorNotInitialized 表示执行器未正确初始化
	ErrExecutorNotInitialized = errors.New("executor not initialized")
)

// ProgressReporter is an interface for reporting command execution progress
// ProgressReporter 是用于上报命令执行进度的接口
type ProgressReporter interface {
	// Report sends a progress update with the current progress percentage and output
	// Report 发送进度更新，包含当前进度百分比和输出
	Report(progress int32, output string) error
}

// CommandHandler is a function type that handles a specific command type
// CommandHandler 是处理特定命令类型的函数类型
// It receives the context, command request, and a progress reporter
// 它接收上下文、命令请求和进度上报器
type CommandHandler func(ctx context.Context, cmd *pb.CommandRequest, reporter ProgressReporter) (*pb.CommandResponse, error)

// CommandExecutor manages command execution and routing
// CommandExecutor 管理命令执行和路由
type CommandExecutor struct {
	// handlers maps command types to their handlers
	// handlers 将命令类型映射到其处理器
	handlers map[pb.CommandType]CommandHandler

	// mu protects the handlers map
	// mu 保护 handlers 映射
	mu sync.RWMutex

	// defaultTimeout is the default timeout for command execution
	// defaultTimeout 是命令执行的默认超时时间
	defaultTimeout time.Duration
}

// NewCommandExecutor creates a new CommandExecutor instance
// NewCommandExecutor 创建一个新的 CommandExecutor 实例
func NewCommandExecutor() *CommandExecutor {
	return &CommandExecutor{
		handlers:       make(map[pb.CommandType]CommandHandler),
		defaultTimeout: 5 * time.Minute, // Default 5 minutes timeout / 默认 5 分钟超时
	}
}

// RegisterHandler registers a handler for a specific command type
// RegisterHandler 为特定命令类型注册处理器
func (e *CommandExecutor) RegisterHandler(cmdType pb.CommandType, handler CommandHandler) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.handlers[cmdType] = handler
}

// UnregisterHandler removes a handler for a specific command type
// UnregisterHandler 移除特定命令类型的处理器
func (e *CommandExecutor) UnregisterHandler(cmdType pb.CommandType) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.handlers, cmdType)
}

// HasHandler checks if a handler is registered for the given command type
// HasHandler 检查是否为给定命令类型注册了处理器
func (e *CommandExecutor) HasHandler(cmdType pb.CommandType) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	_, exists := e.handlers[cmdType]
	return exists
}

// GetRegisteredTypes returns all registered command types
// GetRegisteredTypes 返回所有已注册的命令类型
func (e *CommandExecutor) GetRegisteredTypes() []pb.CommandType {
	e.mu.RLock()
	defer e.mu.RUnlock()
	types := make([]pb.CommandType, 0, len(e.handlers))
	for t := range e.handlers {
		types = append(types, t)
	}
	return types
}

// SetDefaultTimeout sets the default timeout for command execution
// SetDefaultTimeout 设置命令执行的默认超时时间
func (e *CommandExecutor) SetDefaultTimeout(timeout time.Duration) {
	e.defaultTimeout = timeout
}

// Execute executes a command and returns the result
// Execute 执行命令并返回结果
// It routes the command to the appropriate handler based on command type
// 它根据命令类型将命令路由到适当的处理器
func (e *CommandExecutor) Execute(ctx context.Context, cmd *pb.CommandRequest, reporter ProgressReporter) (*pb.CommandResponse, error) {
	// Validate command / 验证命令
	if cmd == nil {
		return nil, errors.New("command request is nil")
	}

	if cmd.CommandId == "" {
		return nil, errors.New("command ID is required")
	}

	// Get handler for command type / 获取命令类型的处理器
	e.mu.RLock()
	handler, exists := e.handlers[cmd.Type]
	e.mu.RUnlock()

	if !exists {
		return e.createErrorResponse(cmd.CommandId, ErrHandlerNotRegistered), ErrHandlerNotRegistered
	}

	// Determine timeout / 确定超时时间
	timeout := e.defaultTimeout
	if cmd.Timeout > 0 {
		timeout = time.Duration(cmd.Timeout) * time.Second
	}

	// Create context with timeout / 创建带超时的上下文
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Create a channel for the result / 创建结果通道
	resultCh := make(chan *pb.CommandResponse, 1)
	errCh := make(chan error, 1)

	// Execute handler in goroutine / 在 goroutine 中执行处理器
	go func() {
		resp, err := handler(execCtx, cmd, reporter)
		if err != nil {
			errCh <- err
			return
		}
		resultCh <- resp
	}()

	// Wait for result or timeout / 等待结果或超时
	select {
	case resp := <-resultCh:
		return resp, nil
	case err := <-errCh:
		return e.createErrorResponse(cmd.CommandId, err), err
	case <-execCtx.Done():
		if errors.Is(execCtx.Err(), context.DeadlineExceeded) {
			return e.createErrorResponse(cmd.CommandId, ErrCommandTimeout), ErrCommandTimeout
		}
		return e.createErrorResponse(cmd.CommandId, ErrCommandCancelled), ErrCommandCancelled
	}
}

// RouteCommand determines the appropriate handler category for a command type
// RouteCommand 确定命令类型的适当处理器类别
// Returns the category name for logging/debugging purposes
// 返回类别名称用于日志/调试目的
func RouteCommand(cmdType pb.CommandType) string {
	switch cmdType {
	case pb.CommandType_PRECHECK:
		return "precheck"
	case pb.CommandType_INSTALL, pb.CommandType_UNINSTALL, pb.CommandType_UPGRADE:
		return "installer"
	case pb.CommandType_START, pb.CommandType_STOP, pb.CommandType_RESTART, pb.CommandType_STATUS:
		return "process"
	case pb.CommandType_COLLECT_LOGS, pb.CommandType_JVM_DUMP, pb.CommandType_THREAD_DUMP:
		return "diagnostic"
	case pb.CommandType_UPDATE_CONFIG, pb.CommandType_ROLLBACK_CONFIG, pb.CommandType_PULL_CONFIG:
		return "config"
	default:
		return "unknown"
	}
}

// createErrorResponse creates a CommandResponse with error status
// createErrorResponse 创建带有错误状态的 CommandResponse
func (e *CommandExecutor) createErrorResponse(commandID string, err error) *pb.CommandResponse {
	return &pb.CommandResponse{
		CommandId: commandID,
		Status:    pb.CommandStatus_FAILED,
		Progress:  0,
		Output:    "",
		Error:     err.Error(),
		Timestamp: time.Now().UnixMilli(),
	}
}

// CreateSuccessResponse creates a CommandResponse with success status
// CreateSuccessResponse 创建带有成功状态的 CommandResponse
func CreateSuccessResponse(commandID string, output string) *pb.CommandResponse {
	return &pb.CommandResponse{
		CommandId: commandID,
		Status:    pb.CommandStatus_SUCCESS,
		Progress:  100,
		Output:    output,
		Error:     "",
		Timestamp: time.Now().UnixMilli(),
	}
}

// CreateProgressResponse creates a CommandResponse with running status and progress
// CreateProgressResponse 创建带有运行状态和进度的 CommandResponse
func CreateProgressResponse(commandID string, progress int32, output string) *pb.CommandResponse {
	return &pb.CommandResponse{
		CommandId: commandID,
		Status:    pb.CommandStatus_RUNNING,
		Progress:  progress,
		Output:    output,
		Error:     "",
		Timestamp: time.Now().UnixMilli(),
	}
}

// CreateErrorResponse creates a CommandResponse with failed status
// CreateErrorResponse 创建带有失败状态的 CommandResponse
func CreateErrorResponse(commandID string, errMsg string) *pb.CommandResponse {
	return &pb.CommandResponse{
		CommandId: commandID,
		Status:    pb.CommandStatus_FAILED,
		Progress:  0,
		Output:    "",
		Error:     errMsg,
		Timestamp: time.Now().UnixMilli(),
	}
}

// NoOpReporter is a ProgressReporter that does nothing
// NoOpReporter 是一个不执行任何操作的 ProgressReporter
type NoOpReporter struct{}

// Report implements ProgressReporter interface but does nothing
// Report 实现 ProgressReporter 接口但不执行任何操作
func (r *NoOpReporter) Report(progress int32, output string) error {
	return nil
}

// ChannelReporter is a ProgressReporter that sends progress to a channel
// ChannelReporter 是一个将进度发送到通道的 ProgressReporter
type ChannelReporter struct {
	// CommandID is the ID of the command being executed
	// CommandID 是正在执行的命令的 ID
	CommandID string

	// Ch is the channel to send progress updates to
	// Ch 是发送进度更新的通道
	Ch chan<- *pb.CommandResponse
}

// Report sends a progress update to the channel
// Report 将进度更新发送到通道
func (r *ChannelReporter) Report(progress int32, output string) error {
	if r.Ch == nil {
		return nil
	}

	select {
	case r.Ch <- CreateProgressResponse(r.CommandID, progress, output):
		return nil
	default:
		// Channel is full, skip this update / 通道已满，跳过此更新
		return nil
	}
}

// CallbackReporter is a ProgressReporter that calls a callback function
// CallbackReporter 是一个调用回调函数的 ProgressReporter
type CallbackReporter struct {
	// CommandID is the ID of the command being executed
	// CommandID 是正在执行的命令的 ID
	CommandID string

	// Callback is the function to call with progress updates
	// Callback 是用于进度更新的回调函数
	Callback func(commandID string, progress int32, output string) error
}

// Report calls the callback function with the progress update
// Report 使用进度更新调用回调函数
func (r *CallbackReporter) Report(progress int32, output string) error {
	if r.Callback == nil {
		return nil
	}
	return r.Callback(r.CommandID, progress, output)
}

// CommandTypeToString converts a CommandType to its string representation
// CommandTypeToString 将 CommandType 转换为其字符串表示
func CommandTypeToString(cmdType pb.CommandType) string {
	return cmdType.String()
}

// ValidateCommandRequest validates a CommandRequest
// ValidateCommandRequest 验证 CommandRequest
func ValidateCommandRequest(cmd *pb.CommandRequest) error {
	if cmd == nil {
		return errors.New("command request is nil")
	}
	if cmd.CommandId == "" {
		return errors.New("command ID is required")
	}
	if cmd.Type == pb.CommandType_COMMAND_TYPE_UNSPECIFIED {
		return fmt.Errorf("command type is unspecified")
	}
	return nil
}
