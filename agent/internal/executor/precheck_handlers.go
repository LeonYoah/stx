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

package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	pb "github.com/LeonYoah/stx/agent"
	"github.com/LeonYoah/stx/agent/internal/installer"
)

// PrecheckSubCommand defines the sub-command types for precheck
// PrecheckSubCommand 定义预检查的子命令类型
type PrecheckSubCommand string

const (
	// PrecheckSubCommandCheckPort checks if a port is available or listening
	// PrecheckSubCommandCheckPort 检查端口是否可用或正在监听
	PrecheckSubCommandCheckPort PrecheckSubCommand = "check_port"

	// PrecheckSubCommandCheckDirectory checks if a directory exists and is writable
	// PrecheckSubCommandCheckDirectory 检查目录是否存在且可写
	PrecheckSubCommandCheckDirectory PrecheckSubCommand = "check_directory"

	// PrecheckSubCommandCheckHTTP checks if an HTTP endpoint is accessible
	// PrecheckSubCommandCheckHTTP 检查 HTTP 端点是否可访问
	PrecheckSubCommandCheckHTTP PrecheckSubCommand = "check_http"

	// PrecheckSubCommandCheckProcess checks if a SeaTunnel process is running
	// PrecheckSubCommandCheckProcess 检查 SeaTunnel 进程是否正在运行
	PrecheckSubCommandCheckProcess PrecheckSubCommand = "check_process"

	// PrecheckSubCommandCheckJava checks if Java is installed and its version
	// PrecheckSubCommandCheckJava 检查 Java 是否已安装及其版本
	PrecheckSubCommandCheckJava PrecheckSubCommand = "check_java"

	// PrecheckSubCommandCheckTCP checks whether a remote TCP endpoint is reachable.
	// PrecheckSubCommandCheckTCP 检查远程 TCP 端点是否可达。
	PrecheckSubCommandCheckTCP PrecheckSubCommand = "check_tcp"

	// PrecheckSubCommandCheckPathReady checks whether a local path exists or can be created.
	// PrecheckSubCommandCheckPathReady 检查本地路径是否已存在或可创建。
	PrecheckSubCommandCheckPathReady PrecheckSubCommand = "check_path_ready"

	// PrecheckSubCommandStatPath inspects local path size and existence.
	// PrecheckSubCommandStatPath 检查本地路径是否存在及其大小。
	PrecheckSubCommandStatPath PrecheckSubCommand = "stat_path"

	// PrecheckSubCommandCleanupPath clears the contents under a local path.
	// PrecheckSubCommandCleanupPath 清理本地路径下的内容。
	PrecheckSubCommandCleanupPath PrecheckSubCommand = "cleanup_path"

	// PrecheckSubCommandSTXJavaProxyProbe performs a real runtime read/write probe.
	// PrecheckSubCommandSTXJavaProxyProbe 执行真实运行时读写探测。
	PrecheckSubCommandSTXJavaProxyProbe PrecheckSubCommand = "stx_java_proxy_probe"

	// PrecheckSubCommandSTXJavaProxyStat performs runtime storage stat through stx-java-proxy.
	// PrecheckSubCommandSTXJavaProxyStat 通过 stx-java-proxy 统计运行时存储占用。
	PrecheckSubCommandSTXJavaProxyStat PrecheckSubCommand = "stx_java_proxy_stat"

	// PrecheckSubCommandSTXJavaProxyList lists runtime storage entries through stx-java-proxy.
	PrecheckSubCommandSTXJavaProxyList PrecheckSubCommand = "stx_java_proxy_list"

	// PrecheckSubCommandSTXJavaProxyPreview previews runtime storage file content through stx-java-proxy.
	PrecheckSubCommandSTXJavaProxyPreview PrecheckSubCommand = "stx_java_proxy_preview"

	// PrecheckSubCommandSTXJavaProxyInspectCheckpoint deserializes checkpoint files through stx-java-proxy.
	PrecheckSubCommandSTXJavaProxyInspectCheckpoint PrecheckSubCommand = "stx_java_proxy_inspect_checkpoint"

	// PrecheckSubCommandSTXJavaProxyInspectCheckpointSourceState inspects checkpoint source state through stx-java-proxy.
	PrecheckSubCommandSTXJavaProxyInspectCheckpointSourceState PrecheckSubCommand = "stx_java_proxy_inspect_checkpoint_source_state"

	// PrecheckSubCommandSTXJavaProxyInspectIMAPWAL inspects IMAP WAL files through stx-java-proxy.
	PrecheckSubCommandSTXJavaProxyInspectIMAPWAL PrecheckSubCommand = "stx_java_proxy_inspect_imap_wal"

	// PrecheckSubCommandSyncLocalRun executes one local sync job through seatunnel.sh.
	PrecheckSubCommandSyncLocalRun PrecheckSubCommand = "sync_local_run"

	// PrecheckSubCommandSyncLocalStatus inspects one local sync job status.
	PrecheckSubCommandSyncLocalStatus PrecheckSubCommand = "sync_local_status"

	// PrecheckSubCommandSyncLocalStop stops one local sync job.
	PrecheckSubCommandSyncLocalStop PrecheckSubCommand = "sync_local_stop"

	// PrecheckSubCommandSyncLocalLogs reads one local sync job stdout/stderr buffer.
	PrecheckSubCommandSyncLocalLogs PrecheckSubCommand = "sync_local_logs"

	// PrecheckSubCommandSyncJobLogs reads one job log file through the agent.
	PrecheckSubCommandSyncJobLogs PrecheckSubCommand = "sync_job_logs"

	// PrecheckSubCommandFull runs all precheck items
	// PrecheckSubCommandFull 运行所有预检查项
	PrecheckSubCommandFull PrecheckSubCommand = "full"
)

// PrecheckResult represents the result of a precheck operation
// PrecheckResult 表示预检查操作的结果
type PrecheckResult struct {
	Success bool              `json:"success"`
	Message string            `json:"message"`
	Details map[string]string `json:"details,omitempty"`
}

// RegisterPrecheckHandlers registers all precheck-related command handlers
// RegisterPrecheckHandlers 注册所有预检查相关的命令处理器
func RegisterPrecheckHandlers(executor *CommandExecutor) {
	executor.RegisterHandler(pb.CommandType_PRECHECK, HandlePrecheckCommand)
}

// HandlePrecheckCommand handles the PRECHECK command type
// HandlePrecheckCommand 处理 PRECHECK 命令类型
func HandlePrecheckCommand(ctx context.Context, cmd *pb.CommandRequest, reporter ProgressReporter) (*pb.CommandResponse, error) {
	subCommand := PrecheckSubCommand(cmd.Parameters["sub_command"])
	if subCommand == "" {
		subCommand = PrecheckSubCommandFull
	}
	// 探测/托管命令带上集群配置的 java-proxy 端口，避免回落到默认 18080。
	// Carry the cluster-configured java-proxy port so probes do not fall back to default 18080.
	if port, err := strconv.Atoi(strings.TrimSpace(cmd.Parameters["java_proxy_port"])); err == nil && port > 0 {
		ctx = installer.ContextWithSTXJavaProxyPort(ctx, port)
	}

	var result *PrecheckResult
	var err error

	switch subCommand {
	case PrecheckSubCommandCheckPort:
		result, err = handleCheckPort(ctx, cmd.Parameters)
	case PrecheckSubCommandCheckDirectory:
		result, err = handleCheckDirectory(ctx, cmd.Parameters)
	case PrecheckSubCommandCheckHTTP:
		result, err = handleCheckHTTP(ctx, cmd.Parameters)
	case PrecheckSubCommandCheckProcess:
		result, err = handleCheckProcess(ctx, cmd.Parameters)
	case PrecheckSubCommandCheckJava:
		result, err = handleCheckJava(ctx, cmd.Parameters)
	case PrecheckSubCommandCheckTCP:
		result, err = handleCheckTCP(ctx, cmd.Parameters)
	case PrecheckSubCommandCheckPathReady:
		result, err = handleCheckPathReady(ctx, cmd.Parameters)
	case PrecheckSubCommandStatPath:
		result, err = handleStatPath(ctx, cmd.Parameters)
	case PrecheckSubCommandCleanupPath:
		result, err = handleCleanupPath(ctx, cmd.Parameters)
	case PrecheckSubCommandSTXJavaProxyProbe:
		result, err = handleSTXJavaProxyProbe(ctx, cmd.Parameters)
	case PrecheckSubCommandSTXJavaProxyStat:
		result, err = handleSTXJavaProxyStat(ctx, cmd.Parameters)
	case PrecheckSubCommandSTXJavaProxyList:
		result, err = handleSTXJavaProxyList(ctx, cmd.Parameters)
	case PrecheckSubCommandSTXJavaProxyPreview:
		result, err = handleSTXJavaProxyPreview(ctx, cmd.Parameters)
	case PrecheckSubCommandSTXJavaProxyInspectCheckpoint:
		result, err = handleSTXJavaProxyInspectCheckpoint(ctx, cmd.Parameters)
	case PrecheckSubCommandSTXJavaProxyInspectCheckpointSourceState:
		result, err = handleSTXJavaProxyInspectCheckpointSourceState(ctx, cmd.Parameters)
	case PrecheckSubCommandSTXJavaProxyInspectIMAPWAL:
		result, err = handleSTXJavaProxyInspectIMAPWAL(ctx, cmd.Parameters)
	case PrecheckSubCommandSyncLocalRun:
		result, err = handleSyncLocalRun(ctx, cmd.Parameters)
	case PrecheckSubCommandSyncLocalStatus:
		result, err = handleSyncLocalStatus(ctx, cmd.Parameters)
	case PrecheckSubCommandSyncLocalStop:
		result, err = handleSyncLocalStop(ctx, cmd.Parameters)
	case PrecheckSubCommandSyncLocalLogs:
		result, err = handleSyncLocalLogs(ctx, cmd.Parameters)
	case PrecheckSubCommandSyncJobLogs:
		result, err = handleSyncJobLogs(ctx, cmd.Parameters)
	case PrecheckSubCommandFull:
		result, err = handleFullPrecheck(ctx, cmd.Parameters, reporter)
	default:
		return CreateErrorResponse(cmd.CommandId, fmt.Sprintf("unknown precheck sub-command: %s", subCommand)), nil
	}

	if err != nil {
		return CreateErrorResponse(cmd.CommandId, err.Error()), nil
	}

	output, err := json.Marshal(result)
	if err != nil {
		return CreateErrorResponse(cmd.CommandId, fmt.Sprintf("failed to serialize result: %v", err)), nil
	}

	if result.Success {
		return CreateSuccessResponse(cmd.CommandId, string(output)), nil
	}
	return CreateErrorResponse(cmd.CommandId, string(output)), nil
}

// handleCheckPort handles the check_port sub-command
// handleCheckPort 处理 check_port 子命令
func handleCheckPort(ctx context.Context, params map[string]string) (*PrecheckResult, error) {
	portStr := params["port"]
	if portStr == "" {
		return &PrecheckResult{
			Success: false,
			Message: "port parameter is required",
		}, nil
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return &PrecheckResult{
			Success: false,
			Message: fmt.Sprintf("invalid port number: %s", portStr),
		}, nil
	}

	checkResult := installer.CheckPortListening(port)

	return &PrecheckResult{
		Success: checkResult.Success,
		Message: checkResult.Message,
		Details: map[string]string{
			"port": portStr,
		},
	}, nil
}

// handleCheckDirectory handles the check_directory sub-command
// handleCheckDirectory 处理 check_directory 子命令
func handleCheckDirectory(ctx context.Context, params map[string]string) (*PrecheckResult, error) {
	path := params["path"]
	if path == "" {
		return &PrecheckResult{
			Success: false,
			Message: "path parameter is required",
		}, nil
	}

	checkResult := installer.CheckDirectoryExists(path)

	return &PrecheckResult{
		Success: checkResult.Success,
		Message: checkResult.Message,
		Details: map[string]string{
			"path": path,
		},
	}, nil
}

// handleCheckHTTP handles the check_http sub-command
// handleCheckHTTP 处理 check_http 子命令
func handleCheckHTTP(ctx context.Context, params map[string]string) (*PrecheckResult, error) {
	url := params["url"]
	if url == "" {
		return &PrecheckResult{
			Success: false,
			Message: "url parameter is required",
		}, nil
	}

	checkResult := installer.CheckHTTPEndpoint(url)

	return &PrecheckResult{
		Success: checkResult.Success,
		Message: checkResult.Message,
		Details: map[string]string{
			"url": url,
		},
	}, nil
}

// handleCheckProcess handles the check_process sub-command
// handleCheckProcess 处理 check_process 子命令
func handleCheckProcess(ctx context.Context, params map[string]string) (*PrecheckResult, error) {
	role := params["role"]
	if role == "" {
		role = "hybrid"
	}

	checkResult := installer.CheckSeaTunnelRunning(ctx, role)

	details := map[string]string{
		"role": role,
	}

	if checkResult.Success {
		processInfo, err := installer.CheckSeaTunnelProcess(ctx, role)
		if err == nil && processInfo != nil {
			details["pid"] = strconv.Itoa(processInfo.PID)
			details["actual_role"] = processInfo.Role
			details["start_time"] = processInfo.StartTime
		}
	}

	return &PrecheckResult{
		Success: checkResult.Success,
		Message: checkResult.Message,
		Details: details,
	}, nil
}

// handleCheckJava handles the check_java sub-command
// handleCheckJava 处理 check_java 子命令
func handleCheckJava(ctx context.Context, params map[string]string) (*PrecheckResult, error) {
	prechecker := installer.NewPrechecker(nil)
	item := prechecker.CheckJava(ctx)

	details := make(map[string]string)
	for k, v := range item.Details {
		details[k] = fmt.Sprintf("%v", v)
	}

	return &PrecheckResult{
		Success: item.Status == installer.CheckStatusPassed || item.Status == installer.CheckStatusWarning,
		Message: item.Message,
		Details: details,
	}, nil
}

// handleCheckTCP handles the check_tcp sub-command.
// handleCheckTCP 处理 check_tcp 子命令。
func handleCheckTCP(ctx context.Context, params map[string]string) (*PrecheckResult, error) {
	host := params["host"]
	portStr := params["port"]
	if host == "" || portStr == "" {
		return &PrecheckResult{
			Success: false,
			Message: "host and port parameters are required",
		}, nil
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return &PrecheckResult{
			Success: false,
			Message: fmt.Sprintf("invalid port number: %s", portStr),
		}, nil
	}
	timeout := 5 * time.Second
	if timeoutStr := params["timeout_seconds"]; timeoutStr != "" {
		if sec, convErr := strconv.Atoi(timeoutStr); convErr == nil && sec > 0 {
			timeout = time.Duration(sec) * time.Second
		}
	}
	checkResult := installer.CheckTCPConnection(host, port, timeout)
	return &PrecheckResult{
		Success: checkResult.Success,
		Message: checkResult.Message,
		Details: checkResult.Details,
	}, nil
}

// handleCheckPathReady handles the check_path_ready sub-command.
// handleCheckPathReady 处理 check_path_ready 子命令。
func handleCheckPathReady(ctx context.Context, params map[string]string) (*PrecheckResult, error) {
	path := params["path"]
	checkResult := installer.CheckPathReady(path)
	return &PrecheckResult{
		Success: checkResult.Success,
		Message: checkResult.Message,
		Details: checkResult.Details,
	}, nil
}

// handleStatPath handles the stat_path sub-command.
// handleStatPath 处理 stat_path 子命令。
func handleStatPath(ctx context.Context, params map[string]string) (*PrecheckResult, error) {
	path := params["path"]
	statResult, err := installer.StatPath(path)
	if err != nil {
		return &PrecheckResult{
			Success: false,
			Message: fmt.Sprintf("failed to stat path %s: %v", path, err),
		}, nil
	}
	details := map[string]string{
		"path":       statResult.Path,
		"exists":     strconv.FormatBool(statResult.Exists),
		"is_dir":     strconv.FormatBool(statResult.IsDir),
		"size_bytes": strconv.FormatInt(statResult.SizeBytes, 10),
	}
	message := fmt.Sprintf("Path %s does not exist", statResult.Path)
	if statResult.Exists {
		message = fmt.Sprintf("Path %s exists", statResult.Path)
	}
	return &PrecheckResult{
		Success: true,
		Message: message,
		Details: details,
	}, nil
}

// handleCleanupPath handles the cleanup_path sub-command.
// handleCleanupPath 处理 cleanup_path 子命令。
func handleCleanupPath(ctx context.Context, params map[string]string) (*PrecheckResult, error) {
	path := params["path"]
	checkResult := installer.CleanupDirectoryContents(path)
	return &PrecheckResult{
		Success: checkResult.Success,
		Message: checkResult.Message,
		Details: checkResult.Details,
	}, nil
}

func handleSTXJavaProxyProbe(ctx context.Context, params map[string]string) (*PrecheckResult, error) {
	kind := params["kind"]
	installDir := params["install_dir"]
	version := params["version"]
	if kind == "" || installDir == "" {
		return &PrecheckResult{Success: false, Message: "kind and install_dir parameters are required"}, nil
	}

	switch kind {
	case "checkpoint":
		cfg, err := checkpointConfigFromParams(params)
		if err != nil {
			return &PrecheckResult{Success: false, Message: err.Error()}, nil
		}
		result, err := installer.ExecuteCheckpointRuntimeStorageProbe(ctx, installDir, version, cfg)
		if err != nil {
			return &PrecheckResult{Success: false, Message: err.Error()}, nil
		}
		return &PrecheckResult{
			Success: result.OK && result.Writable && result.Readable,
			Message: firstNonEmpty(result.Message, "checkpoint runtime probe completed"),
			Details: map[string]string{
				"kind":       kind,
				"ok":         strconv.FormatBool(result.OK),
				"writable":   strconv.FormatBool(result.Writable),
				"readable":   strconv.FormatBool(result.Readable),
				"statusCode": strconv.Itoa(result.StatusCode),
			},
		}, nil
	case "imap":
		cfg, err := imapConfigFromParams(params)
		if err != nil {
			return &PrecheckResult{Success: false, Message: err.Error()}, nil
		}
		result, err := installer.ExecuteIMAPRuntimeStorageProbe(ctx, installDir, version, cfg)
		if err != nil {
			return &PrecheckResult{Success: false, Message: err.Error()}, nil
		}
		return &PrecheckResult{
			Success: result.OK && result.Writable && result.Readable,
			Message: firstNonEmpty(result.Message, "imap runtime probe completed"),
			Details: map[string]string{
				"kind":       kind,
				"ok":         strconv.FormatBool(result.OK),
				"writable":   strconv.FormatBool(result.Writable),
				"readable":   strconv.FormatBool(result.Readable),
				"statusCode": strconv.Itoa(result.StatusCode),
			},
		}, nil
	default:
		return &PrecheckResult{Success: false, Message: fmt.Sprintf("unsupported kind: %s", kind)}, nil
	}
}

func handleSTXJavaProxyStat(ctx context.Context, params map[string]string) (*PrecheckResult, error) {
	kind := params["kind"]
	installDir := params["install_dir"]
	version := params["version"]
	if kind == "" || installDir == "" {
		return &PrecheckResult{Success: false, Message: "kind and install_dir parameters are required"}, nil
	}

	switch kind {
	case "checkpoint":
		cfg, err := checkpointConfigFromParams(params)
		if err != nil {
			return &PrecheckResult{Success: false, Message: err.Error()}, nil
		}
		result, err := installer.ExecuteCheckpointRuntimeStorageStat(ctx, installDir, version, cfg)
		if err != nil {
			return &PrecheckResult{Success: false, Message: err.Error()}, nil
		}
		return runtimeStorageStatPrecheckResult(kind, result), nil
	case "imap":
		cfg, err := imapConfigFromParams(params)
		if err != nil {
			return &PrecheckResult{Success: false, Message: err.Error()}, nil
		}
		result, err := installer.ExecuteIMAPRuntimeStorageStat(ctx, installDir, version, cfg)
		if err != nil {
			return &PrecheckResult{Success: false, Message: err.Error()}, nil
		}
		return runtimeStorageStatPrecheckResult(kind, result), nil
	default:
		return &PrecheckResult{Success: false, Message: fmt.Sprintf("unsupported kind: %s", kind)}, nil
	}
}

func handleSTXJavaProxyList(ctx context.Context, params map[string]string) (*PrecheckResult, error) {
	kind := params["kind"]
	installDir := params["install_dir"]
	version := params["version"]
	if kind == "" || installDir == "" {
		return &PrecheckResult{Success: false, Message: "kind and install_dir parameters are required"}, nil
	}
	path := params["path"]
	recursive := parseParamBool(params["recursive"])
	limit := parseParamInt(params["limit"])
	if limit <= 0 {
		limit = 200
	}

	switch kind {
	case "checkpoint":
		cfg, err := checkpointConfigFromParams(params)
		if err != nil {
			return &PrecheckResult{Success: false, Message: err.Error()}, nil
		}
		result, err := installer.ExecuteCheckpointRuntimeStorageList(ctx, installDir, version, cfg, path, recursive, limit)
		if err != nil {
			return &PrecheckResult{Success: false, Message: err.Error()}, nil
		}
		return runtimeStorageListPrecheckResult(kind, result), nil
	case "imap":
		cfg, err := imapConfigFromParams(params)
		if err != nil {
			return &PrecheckResult{Success: false, Message: err.Error()}, nil
		}
		result, err := installer.ExecuteIMAPRuntimeStorageList(ctx, installDir, version, cfg, path, recursive, limit)
		if err != nil {
			return &PrecheckResult{Success: false, Message: err.Error()}, nil
		}
		return runtimeStorageListPrecheckResult(kind, result), nil
	default:
		return &PrecheckResult{Success: false, Message: fmt.Sprintf("unsupported kind: %s", kind)}, nil
	}
}

func runtimeStorageListPrecheckResult(kind string, result *installer.RuntimeStorageListResult) *PrecheckResult {
	if result == nil {
		return &PrecheckResult{Success: false, Message: "runtime storage list returned no result"}
	}
	details := map[string]string{
		"kind":         kind,
		"ok":           strconv.FormatBool(result.OK),
		"path":         result.Path,
		"storage_type": result.StorageType,
	}
	if len(result.Items) > 0 {
		if bytes, err := json.Marshal(result.Items); err == nil {
			details["items_json"] = string(bytes)
		}
	}
	return &PrecheckResult{
		Success: result.OK,
		Message: firstNonEmpty(result.Message, fmt.Sprintf("%s runtime storage list completed", kind)),
		Details: details,
	}
}

func runtimeStorageStatPrecheckResult(kind string, result *installer.RuntimeStorageStatResult) *PrecheckResult {
	if result == nil {
		return &PrecheckResult{Success: false, Message: "runtime storage stat returned no result"}
	}
	return &PrecheckResult{
		Success: result.OK,
		Message: firstNonEmpty(result.Message, fmt.Sprintf("%s runtime storage stat completed", kind)),
		Details: map[string]string{
			"kind":             kind,
			"ok":               strconv.FormatBool(result.OK),
			"exists":           strconv.FormatBool(result.Exists),
			"path":             result.Path,
			"storage_type":     result.StorageType,
			"total_size_bytes": strconv.FormatInt(result.TotalSizeBytes, 10),
			"file_count":       strconv.FormatInt(result.FileCount, 10),
			"statusCode":       strconv.Itoa(result.StatusCode),
		},
	}
}

func handleSTXJavaProxyPreview(ctx context.Context, params map[string]string) (*PrecheckResult, error) {
	kind := params["kind"]
	installDir := params["install_dir"]
	version := params["version"]
	path := params["path"]
	maxBytes := parseParamInt(params["max_bytes"])
	if kind == "" || installDir == "" || strings.TrimSpace(path) == "" {
		return &PrecheckResult{Success: false, Message: "kind, install_dir, and path parameters are required"}, nil
	}
	switch kind {
	case "checkpoint":
		cfg, err := checkpointConfigFromParams(params)
		if err != nil {
			return &PrecheckResult{Success: false, Message: err.Error()}, nil
		}
		result, err := installer.ExecuteCheckpointRuntimeStoragePreview(ctx, installDir, version, cfg, path, maxBytes)
		if err != nil {
			return &PrecheckResult{Success: false, Message: err.Error()}, nil
		}
		return runtimeStoragePreviewPrecheckResult(kind, result), nil
	case "imap":
		cfg, err := imapConfigFromParams(params)
		if err != nil {
			return &PrecheckResult{Success: false, Message: err.Error()}, nil
		}
		result, err := installer.ExecuteIMAPRuntimeStoragePreview(ctx, installDir, version, cfg, path, maxBytes)
		if err != nil {
			return &PrecheckResult{Success: false, Message: err.Error()}, nil
		}
		return runtimeStoragePreviewPrecheckResult(kind, result), nil
	default:
		return &PrecheckResult{Success: false, Message: fmt.Sprintf("unsupported kind: %s", kind)}, nil
	}
}

func handleSTXJavaProxyInspectCheckpoint(ctx context.Context, params map[string]string) (*PrecheckResult, error) {
	installDir := params["install_dir"]
	version := params["version"]
	path := params["path"]
	contentBase64 := params["content_base64"]
	if installDir == "" || strings.TrimSpace(path) == "" {
		return &PrecheckResult{Success: false, Message: "install_dir and path parameters are required"}, nil
	}
	var (
		result *installer.RuntimeStorageCheckpointInspectResult
		err    error
	)
	if strings.TrimSpace(contentBase64) != "" {
		result, err = installer.ExecuteCheckpointInspectFromBase64(ctx, installDir, version, path, contentBase64)
	} else {
		cfg, cfgErr := checkpointConfigFromParams(params)
		if cfgErr != nil {
			return &PrecheckResult{Success: false, Message: cfgErr.Error()}, nil
		}
		result, err = installer.ExecuteCheckpointInspect(ctx, installDir, version, cfg, path)
	}
	if err != nil {
		return &PrecheckResult{Success: false, Message: err.Error()}, nil
	}
	return runtimeStorageCheckpointInspectPrecheckResult(result), nil
}

func handleSTXJavaProxyInspectCheckpointSourceState(
	ctx context.Context,
	params map[string]string,
) (*PrecheckResult, error) {
	installDir := params["install_dir"]
	version := params["version"]
	path := params["path"]
	contentBase64 := params["content_base64"]
	jobConfigJSON := strings.TrimSpace(params["job_config_json"])
	if installDir == "" || strings.TrimSpace(path) == "" {
		return &PrecheckResult{Success: false, Message: "install_dir and path parameters are required"}, nil
	}
	if jobConfigJSON == "" {
		return &PrecheckResult{Success: false, Message: "job_config_json parameter is required"}, nil
	}
	jobConfig := make(map[string]interface{})
	if err := json.Unmarshal([]byte(jobConfigJSON), &jobConfig); err != nil {
		return &PrecheckResult{Success: false, Message: fmt.Sprintf("invalid job_config_json: %v", err)}, nil
	}
	var (
		result *installer.RuntimeStorageCheckpointSourceStateInspectResult
		err    error
	)
	if strings.TrimSpace(contentBase64) != "" {
		result, err = installer.ExecuteCheckpointInspectSourceStateFromBase64(
			ctx, installDir, version, path, contentBase64, jobConfig)
	} else {
		cfg, cfgErr := checkpointConfigFromParams(params)
		if cfgErr != nil {
			return &PrecheckResult{Success: false, Message: cfgErr.Error()}, nil
		}
		result, err = installer.ExecuteCheckpointInspectSourceState(
			ctx, installDir, version, cfg, path, jobConfig)
	}
	if err != nil {
		return &PrecheckResult{Success: false, Message: err.Error()}, nil
	}
	return runtimeStorageCheckpointSourceStateInspectPrecheckResult(result), nil
}

func handleSTXJavaProxyInspectIMAPWAL(ctx context.Context, params map[string]string) (*PrecheckResult, error) {
	installDir := params["install_dir"]
	version := params["version"]
	path := params["path"]
	contentBase64 := params["content_base64"]
	if installDir == "" || strings.TrimSpace(path) == "" {
		return &PrecheckResult{Success: false, Message: "install_dir and path parameters are required"}, nil
	}
	var (
		result *installer.RuntimeStorageIMAPInspectResult
		err    error
	)
	if strings.TrimSpace(contentBase64) != "" {
		result, err = installer.ExecuteIMAPWALInspectFromBase64(ctx, installDir, version, path, contentBase64)
	} else {
		cfg, cfgErr := imapConfigFromParams(params)
		if cfgErr != nil {
			return &PrecheckResult{Success: false, Message: cfgErr.Error()}, nil
		}
		result, err = installer.ExecuteIMAPWALInspect(ctx, installDir, version, cfg, path)
	}
	if err != nil {
		return &PrecheckResult{Success: false, Message: err.Error()}, nil
	}
	return runtimeStorageIMAPInspectPrecheckResult(result), nil
}

func runtimeStoragePreviewPrecheckResult(kind string, result *installer.RuntimeStoragePreviewResult) *PrecheckResult {
	if result == nil {
		return &PrecheckResult{Success: false, Message: "runtime storage preview returned no result"}
	}
	details := map[string]string{
		"kind":         kind,
		"ok":           strconv.FormatBool(result.OK),
		"path":         result.Path,
		"file_name":    result.FileName,
		"storage_type": result.StorageType,
		"size_bytes":   strconv.FormatInt(result.SizeBytes, 10),
		"truncated":    strconv.FormatBool(result.Truncated),
		"binary":       strconv.FormatBool(result.Binary),
		"encoding":     result.Encoding,
		"text_preview": result.TextPreview,
		"hex_preview":  result.HexPreview,
	}
	return &PrecheckResult{
		Success: result.OK,
		Message: firstNonEmpty(result.Message, fmt.Sprintf("%s runtime storage preview completed", kind)),
		Details: details,
	}
}

func runtimeStorageCheckpointInspectPrecheckResult(result *installer.RuntimeStorageCheckpointInspectResult) *PrecheckResult {
	if result == nil {
		return &PrecheckResult{Success: false, Message: "checkpoint inspect returned no result"}
	}
	details := map[string]string{
		"ok":           strconv.FormatBool(result.OK),
		"path":         result.Path,
		"file_name":    result.FileName,
		"storage_type": result.StorageType,
		"size_bytes":   strconv.FormatInt(result.SizeBytes, 10),
		"truncated":    strconv.FormatBool(result.Truncated),
		"binary":       strconv.FormatBool(result.Binary),
		"encoding":     result.Encoding,
		"text_preview": result.TextPreview,
		"hex_preview":  result.HexPreview,
	}
	if bytes, err := json.Marshal(result.PipelineState); err == nil {
		details["pipeline_state_json"] = string(bytes)
	}
	if bytes, err := json.Marshal(result.CompletedCheckpoint); err == nil {
		details["completed_checkpoint_json"] = string(bytes)
	}
	if bytes, err := json.Marshal(result.ActionStates); err == nil {
		details["action_states_json"] = string(bytes)
	}
	if bytes, err := json.Marshal(result.TaskStatistics); err == nil {
		details["task_statistics_json"] = string(bytes)
	}
	return &PrecheckResult{
		Success: result.OK,
		Message: firstNonEmpty(result.Message, "checkpoint deserialize completed"),
		Details: details,
	}
}

func runtimeStorageCheckpointSourceStateInspectPrecheckResult(
	result *installer.RuntimeStorageCheckpointSourceStateInspectResult,
) *PrecheckResult {
	if result == nil {
		return &PrecheckResult{Success: false, Message: "checkpoint source state inspect returned no result"}
	}
	details := map[string]string{
		"ok": strconv.FormatBool(result.OK),
	}
	if bytes, err := json.Marshal(result.PipelineState); err == nil {
		details["pipeline_state_json"] = string(bytes)
	}
	if bytes, err := json.Marshal(result.CompletedCheckpoint); err == nil {
		details["completed_checkpoint_json"] = string(bytes)
	}
	if bytes, err := json.Marshal(result.Sources); err == nil {
		details["sources_json"] = string(bytes)
	}
	if bytes, err := json.Marshal(result.UnsupportedSources); err == nil {
		details["unsupported_sources_json"] = string(bytes)
	}
	if bytes, err := json.Marshal(result.Warnings); err == nil {
		details["warnings_json"] = string(bytes)
	}
	return &PrecheckResult{
		Success: result.OK,
		Message: firstNonEmpty(result.Message, "checkpoint source state inspect completed"),
		Details: details,
	}
}

func runtimeStorageIMAPInspectPrecheckResult(result *installer.RuntimeStorageIMAPInspectResult) *PrecheckResult {
	if result == nil {
		return &PrecheckResult{Success: false, Message: "imap wal inspect returned no result"}
	}
	details := map[string]string{
		"ok":           strconv.FormatBool(result.OK),
		"path":         result.Path,
		"file_name":    result.FileName,
		"storage_type": result.StorageType,
		"size_bytes":   strconv.FormatInt(result.SizeBytes, 10),
		"truncated":    strconv.FormatBool(result.Truncated),
		"binary":       strconv.FormatBool(result.Binary),
		"encoding":     result.Encoding,
		"text_preview": result.TextPreview,
		"hex_preview":  result.HexPreview,
		"entry_count":  strconv.Itoa(result.EntryCount),
	}
	if bytes, err := json.Marshal(result.Entries); err == nil {
		details["entries_json"] = string(bytes)
	}
	return &PrecheckResult{
		Success: result.OK,
		Message: firstNonEmpty(result.Message, "imap wal inspect completed"),
		Details: details,
	}
}

func checkpointConfigFromParams(params map[string]string) (*installer.CheckpointConfig, error) {
	storageType := installer.CheckpointStorageType(params["storage_type"])
	cfg := &installer.CheckpointConfig{
		StorageType:               storageType,
		Namespace:                 params["namespace"],
		HDFSNameNodeHost:          params["hdfs_namenode_host"],
		HDFSNameNodePort:          parseParamInt(params["hdfs_namenode_port"]),
		KerberosPrincipal:         params["kerberos_principal"],
		KerberosKeytabFilePath:    params["kerberos_keytab_file_path"],
		HDFSHAEnabled:             parseParamBool(params["hdfs_ha_enabled"]),
		HDFSNameServices:          params["hdfs_name_services"],
		HDFSHANamenodes:           params["hdfs_ha_namenodes"],
		HDFSNamenodeRPCAddress1:   params["hdfs_namenode_rpc_address_1"],
		HDFSNamenodeRPCAddress2:   params["hdfs_namenode_rpc_address_2"],
		HDFSFailoverProxyProvider: params["hdfs_failover_proxy_provider"],
		StorageEndpoint:           params["storage_endpoint"],
		StorageAccessKey:          params["storage_access_key"],
		StorageSecretKey:          params["storage_secret_key"],
		StorageBucket:             params["storage_bucket"],
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func imapConfigFromParams(params map[string]string) (*installer.IMAPConfig, error) {
	storageType := installer.IMAPStorageType(params["storage_type"])
	cfg := &installer.IMAPConfig{
		StorageType:               storageType,
		Namespace:                 params["namespace"],
		HDFSNameNodeHost:          params["hdfs_namenode_host"],
		HDFSNameNodePort:          parseParamInt(params["hdfs_namenode_port"]),
		KerberosPrincipal:         params["kerberos_principal"],
		KerberosKeytabFilePath:    params["kerberos_keytab_file_path"],
		HDFSHAEnabled:             parseParamBool(params["hdfs_ha_enabled"]),
		HDFSNameServices:          params["hdfs_name_services"],
		HDFSHANamenodes:           params["hdfs_ha_namenodes"],
		HDFSNamenodeRPCAddress1:   params["hdfs_namenode_rpc_address_1"],
		HDFSNamenodeRPCAddress2:   params["hdfs_namenode_rpc_address_2"],
		HDFSFailoverProxyProvider: params["hdfs_failover_proxy_provider"],
		StorageEndpoint:           params["storage_endpoint"],
		StorageAccessKey:          params["storage_access_key"],
		StorageSecretKey:          params["storage_secret_key"],
		StorageBucket:             params["storage_bucket"],
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func parseParamInt(raw string) int {
	parsed, _ := strconv.Atoi(raw)
	return parsed
}

func parseParamBool(raw string) bool {
	parsed, _ := strconv.ParseBool(raw)
	return parsed
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// handleFullPrecheck handles the full precheck sub-command
// handleFullPrecheck 处理完整预检查子命令
func handleFullPrecheck(ctx context.Context, params map[string]string, reporter ProgressReporter) (*PrecheckResult, error) {
	results := make(map[string]string)
	allPassed := true

	if reporter != nil {
		reporter.Report(0, "Starting full precheck...")
	}

	// 1. Check install directory if provided
	if path := params["install_dir"]; path != "" {
		if reporter != nil {
			reporter.Report(20, fmt.Sprintf("Checking directory: %s", path))
		}
		dirResult := installer.CheckDirectoryExists(path)
		results["directory_check"] = dirResult.Message
		if !dirResult.Success {
			allPassed = false
		}
	}

	// 2. Check Hazelcast port if provided
	if portStr := params["hazelcast_port"]; portStr != "" {
		if reporter != nil {
			reporter.Report(40, fmt.Sprintf("Checking Hazelcast port: %s", portStr))
		}
		port, err := strconv.Atoi(portStr)
		if err == nil {
			portResult := installer.CheckPortListening(port)
			if portResult.Success {
				results["hazelcast_port_check"] = fmt.Sprintf("Port %d is already in use", port)
				allPassed = false
			} else {
				results["hazelcast_port_check"] = fmt.Sprintf("Port %d is available", port)
			}
		}
	}

	// 3. Check API port if provided
	if portStr := params["api_port"]; portStr != "" {
		if reporter != nil {
			reporter.Report(60, fmt.Sprintf("Checking API port: %s", portStr))
		}
		port, err := strconv.Atoi(portStr)
		if err == nil && port > 0 {
			portResult := installer.CheckPortListening(port)
			if portResult.Success {
				results["api_port_check"] = fmt.Sprintf("Port %d is already in use", port)
				allPassed = false
			} else {
				results["api_port_check"] = fmt.Sprintf("Port %d is available", port)
			}
		}
	}

	// 4. Check if SeaTunnel process is already running
	if reporter != nil {
		reporter.Report(80, "Checking for existing SeaTunnel processes...")
	}
	role := params["role"]
	if role == "" {
		role = "hybrid"
	}
	processResult := installer.CheckSeaTunnelRunning(ctx, role)
	if processResult.Success {
		results["process_check"] = fmt.Sprintf("SeaTunnel process is already running: %s", processResult.Message)
	} else {
		results["process_check"] = "No existing SeaTunnel process found"
	}

	if reporter != nil {
		reporter.Report(100, "Full precheck completed")
	}

	message := "All precheck items passed"
	if !allPassed {
		message = "Some precheck items failed"
	}

	return &PrecheckResult{
		Success: allPassed,
		Message: message,
		Details: results,
	}, nil
}
