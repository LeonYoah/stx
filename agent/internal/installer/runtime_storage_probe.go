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

package installer

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/LeonYoah/stx/agent/internal/logger"
	seatunnelmeta "github.com/LeonYoah/stx/internal/seatunnel"
	"gopkg.in/yaml.v3"
)

const (
	stxJavaProxyHomeEnvVar        = "STX_JAVA_PROXY_HOME"
	stxJavaProxyJarEnvVar         = "STX_JAVA_PROXY_JAR"
	stxJavaProxyScriptEnvVar      = "STX_JAVA_PROXY_SCRIPT"
	stxJavaProxyEndpointEnvVar    = "STX_JAVA_PROXY_ENDPOINT"
	stxJavaProxyPortEnvVar        = "STX_JAVA_PROXY_PORT"
	stxJavaProxyVersionEnvVar     = "STX_JAVA_PROXY_VERSION"
	stxJavaProxyJvmOptsEnvVar     = "STX_JAVA_PROXY_JVM_OPTS"
	stxJavaProxyDefaultSupportDir = "/usr/local/lib/stx-agent"
	// stxJavaProxyUserSupportDirName is the relative Agent home under $HOME for non-root installs.
	// stxJavaProxyUserSupportDirName 是非 root 安装时位于 $HOME 下的 Agent 主目录相对路径。
	stxJavaProxyUserSupportDirName = ".stx/agent"
	runtimeProbeTimeout            = 20 * time.Second
	runtimeProbeBusinessName       = "imap-probe"
	runtimeProbeClusterName        = "seatunnel-cluster"
	stxJavaProxyDefaultHost        = "127.0.0.1"
	stxJavaProxyDefaultPort        = 18080
	stxJavaProxyHealthPath         = "/healthz"
	stxJavaProxyServiceDirName     = "stx-java-proxy"
	stxJavaProxyStartupWait        = 12 * time.Second
)

type runtimeStorageProbeResponse struct {
	OK         bool   `json:"ok"`
	StatusCode int    `json:"statusCode,omitempty"`
	Message    string `json:"message,omitempty"`
	Writable   bool   `json:"writable,omitempty"`
	Readable   bool   `json:"readable,omitempty"`
}

// RuntimeStorageProbeResult is the exported read/write probe result.
type RuntimeStorageProbeResult struct {
	OK         bool   `json:"ok"`
	StatusCode int    `json:"statusCode,omitempty"`
	Message    string `json:"message,omitempty"`
	Writable   bool   `json:"writable,omitempty"`
	Readable   bool   `json:"readable,omitempty"`
}

// RuntimeStorageStatResult is the exported runtime storage stat result.
type RuntimeStorageStatResult struct {
	OK             bool   `json:"ok"`
	StatusCode     int    `json:"statusCode,omitempty"`
	Message        string `json:"message,omitempty"`
	Exists         bool   `json:"exists,omitempty"`
	StorageType    string `json:"storageType,omitempty"`
	Path           string `json:"path,omitempty"`
	TotalSizeBytes int64  `json:"totalSizeBytes,omitempty"`
	FileCount      int64  `json:"fileCount,omitempty"`
}

// RuntimeStorageListItem describes one runtime storage entry.
type RuntimeStorageListItem struct {
	Path       string `json:"path,omitempty"`
	Name       string `json:"name,omitempty"`
	Directory  bool   `json:"directory,omitempty"`
	SizeBytes  int64  `json:"sizeBytes,omitempty"`
	ModifiedAt string `json:"modifiedAt,omitempty"`
}

// RuntimeStorageListResult is the exported runtime storage listing result.
type RuntimeStorageListResult struct {
	OK          bool                     `json:"ok"`
	StatusCode  int                      `json:"statusCode,omitempty"`
	Message     string                   `json:"message,omitempty"`
	StorageType string                   `json:"storageType,omitempty"`
	Path        string                   `json:"path,omitempty"`
	Items       []RuntimeStorageListItem `json:"items,omitempty"`
}

// RuntimeStoragePreviewResult is the exported runtime storage preview result.
type RuntimeStoragePreviewResult struct {
	OK          bool   `json:"ok"`
	StatusCode  int    `json:"statusCode,omitempty"`
	Message     string `json:"message,omitempty"`
	StorageType string `json:"storageType,omitempty"`
	Path        string `json:"path,omitempty"`
	FileName    string `json:"fileName,omitempty"`
	SizeBytes   int64  `json:"sizeBytes,omitempty"`
	Truncated   bool   `json:"truncated,omitempty"`
	Binary      bool   `json:"binary,omitempty"`
	Encoding    string `json:"encoding,omitempty"`
	TextPreview string `json:"textPreview,omitempty"`
	HexPreview  string `json:"hexPreview,omitempty"`
}

// RuntimeStorageCheckpointInspectResult is the exported checkpoint deserialize result.
type RuntimeStorageCheckpointInspectResult struct {
	OK                  bool                     `json:"ok"`
	StatusCode          int                      `json:"statusCode,omitempty"`
	Message             string                   `json:"message,omitempty"`
	StorageType         string                   `json:"storageType,omitempty"`
	Path                string                   `json:"path,omitempty"`
	FileName            string                   `json:"fileName,omitempty"`
	SizeBytes           int64                    `json:"sizeBytes,omitempty"`
	Truncated           bool                     `json:"truncated,omitempty"`
	Binary              bool                     `json:"binary,omitempty"`
	Encoding            string                   `json:"encoding,omitempty"`
	TextPreview         string                   `json:"textPreview,omitempty"`
	HexPreview          string                   `json:"hexPreview,omitempty"`
	PipelineState       map[string]interface{}   `json:"pipelineState,omitempty"`
	CompletedCheckpoint map[string]interface{}   `json:"completedCheckpoint,omitempty"`
	ActionStates        []map[string]interface{} `json:"actionStates,omitempty"`
	TaskStatistics      []map[string]interface{} `json:"taskStatistics,omitempty"`
}

// RuntimeStorageCheckpointSourceStateInspectResult is the exported checkpoint source state inspect result.
type RuntimeStorageCheckpointSourceStateInspectResult struct {
	OK                  bool                     `json:"ok"`
	StatusCode          int                      `json:"statusCode,omitempty"`
	Message             string                   `json:"message,omitempty"`
	PipelineState       map[string]interface{}   `json:"pipelineState,omitempty"`
	CompletedCheckpoint map[string]interface{}   `json:"completedCheckpoint,omitempty"`
	Sources             []map[string]interface{} `json:"sources,omitempty"`
	Sinks               []map[string]interface{} `json:"sinks,omitempty"`
	UnsupportedSources  []map[string]interface{} `json:"unsupportedSources,omitempty"`
	Warnings            []string                 `json:"warnings,omitempty"`
}

// RuntimeStorageIMAPInspectResult is the exported IMAP WAL inspect result.
type RuntimeStorageIMAPInspectResult struct {
	OK          bool                     `json:"ok"`
	StatusCode  int                      `json:"statusCode,omitempty"`
	Message     string                   `json:"message,omitempty"`
	StorageType string                   `json:"storageType,omitempty"`
	Path        string                   `json:"path,omitempty"`
	FileName    string                   `json:"fileName,omitempty"`
	SizeBytes   int64                    `json:"sizeBytes,omitempty"`
	Truncated   bool                     `json:"truncated,omitempty"`
	Binary      bool                     `json:"binary,omitempty"`
	Encoding    string                   `json:"encoding,omitempty"`
	TextPreview string                   `json:"textPreview,omitempty"`
	HexPreview  string                   `json:"hexPreview,omitempty"`
	EntryCount  int                      `json:"entryCount,omitempty"`
	Entries     []map[string]interface{} `json:"entries,omitempty"`
}

func buildCheckpointPluginConfig(cfg *CheckpointConfig) (map[string]string, error) {
	namespace := normalizeRuntimeStorageNamespace(cfg.Namespace)
	pluginConfig := make(map[string]string)

	switch cfg.StorageType {
	case CheckpointStorageLocalFile:
		pluginConfig["storage.type"] = "hdfs"
		pluginConfig["namespace"] = namespace
		pluginConfig["fs.defaultFS"] = "file:///"

	case CheckpointStorageHDFS:
		pluginConfig["storage.type"] = "hdfs"
		pluginConfig["namespace"] = namespace
		if cfg.HDFSHAEnabled {
			if strings.TrimSpace(cfg.HDFSNameServices) == "" {
				return nil, fmt.Errorf("hdfs_name_services is required for HDFS HA storage")
			}
			haEndpoints, err := seatunnelmeta.ResolveHDFSHARPCAddresses(
				cfg.HDFSHANamenodes,
				cfg.HDFSNamenodeRPCAddress1,
				cfg.HDFSNamenodeRPCAddress2,
			)
			if err != nil {
				return nil, err
			}
			pluginConfig["fs.defaultFS"] = fmt.Sprintf("hdfs://%s", cfg.HDFSNameServices)
			pluginConfig["seatunnel.hadoop.dfs.nameservices"] = cfg.HDFSNameServices
			pluginConfig[fmt.Sprintf("seatunnel.hadoop.dfs.ha.namenodes.%s", cfg.HDFSNameServices)] = cfg.HDFSHANamenodes
			for _, endpoint := range haEndpoints {
				pluginConfig[fmt.Sprintf("seatunnel.hadoop.dfs.namenode.rpc-address.%s.%s", cfg.HDFSNameServices, endpoint.Name)] = endpoint.Address
			}
			failoverProvider := cfg.HDFSFailoverProxyProvider
			if failoverProvider == "" {
				failoverProvider = "org.apache.hadoop.hdfs.server.namenode.ha.ConfiguredFailoverProxyProvider"
			}
			pluginConfig[fmt.Sprintf("seatunnel.hadoop.dfs.client.failover.proxy.provider.%s", cfg.HDFSNameServices)] = failoverProvider
		} else {
			pluginConfig["fs.defaultFS"] = fmt.Sprintf("hdfs://%s:%d", cfg.HDFSNameNodeHost, cfg.HDFSNameNodePort)
		}
		if cfg.KerberosPrincipal != "" {
			pluginConfig["kerberosPrincipal"] = cfg.KerberosPrincipal
		}
		if cfg.KerberosKeytabFilePath != "" {
			pluginConfig["kerberosKeytabFilePath"] = cfg.KerberosKeytabFilePath
		}
		if strings.TrimSpace(cfg.HdfsSitePath) != "" {
			pluginConfig["hdfs_site_path"] = strings.TrimSpace(cfg.HdfsSitePath)
		}
		if cfg.DisableCache != nil {
			pluginConfig["disable.cache"] = strconv.FormatBool(*cfg.DisableCache)
		}

	case CheckpointStorageOSS:
		pluginConfig["storage.type"] = "oss"
		pluginConfig["namespace"] = namespace
		if cfg.StorageBucket != "" {
			pluginConfig["oss.bucket"] = cfg.StorageBucket
		}
		if cfg.StorageEndpoint != "" {
			pluginConfig["fs.oss.endpoint"] = cfg.StorageEndpoint
		}
		if cfg.StorageAccessKey != "" {
			pluginConfig["fs.oss.accessKeyId"] = cfg.StorageAccessKey
		}
		if cfg.StorageSecretKey != "" {
			pluginConfig["fs.oss.accessKeySecret"] = cfg.StorageSecretKey
		}

	case CheckpointStorageS3:
		pluginConfig["storage.type"] = "s3"
		pluginConfig["namespace"] = namespace
		if cfg.StorageBucket != "" {
			pluginConfig["s3.bucket"] = cfg.StorageBucket
		}
		if cfg.StorageEndpoint != "" {
			pluginConfig["fs.s3a.endpoint"] = cfg.StorageEndpoint
		}
		if cfg.StorageAccessKey != "" {
			pluginConfig["fs.s3a.access.key"] = cfg.StorageAccessKey
		}
		if cfg.StorageSecretKey != "" {
			pluginConfig["fs.s3a.secret.key"] = cfg.StorageSecretKey
		}
		provider := strings.TrimSpace(cfg.S3CredentialsProvider)
		if provider == "" {
			provider = "org.apache.hadoop.fs.s3a.SimpleAWSCredentialsProvider"
		}
		pluginConfig["fs.s3a.aws.credentials.provider"] = provider

	default:
		pluginConfig["storage.type"] = "hdfs"
		pluginConfig["namespace"] = namespace
		pluginConfig["fs.defaultFS"] = "file:///"
	}

	return pluginConfig, nil
}

func normalizeRuntimeStorageNamespace(namespace string) string {
	trimmed := strings.TrimSpace(namespace)
	if trimmed != "" && !strings.HasSuffix(trimmed, "/") {
		trimmed += "/"
	}
	return trimmed
}

func (m *InstallerManager) maybeProbeCheckpointRuntimeStorage(ctx context.Context, params *InstallParams) string {
	if params == nil || params.Checkpoint == nil || !isRemoteCheckpointStorage(params.Checkpoint.StorageType) {
		return ""
	}
	request, err := buildCheckpointRuntimeProbeRequest(params.Checkpoint)
	if err != nil {
		return fmt.Sprintf("failed to build checkpoint probe request: %v", err)
	}
	response, err := m.executeRuntimeStorageProbe(ctx, params.InstallDir, params.Version, "checkpoint", params.JavaProxyPort, request)
	if err != nil {
		logger.WarnF(ctx, "[Install] checkpoint runtime probe execution failed: install_dir=%s, error=%v", params.InstallDir, err)
		return err.Error()
	}
	if !response.OK {
		return firstNonBlank(response.Message, "checkpoint runtime probe returned a non-success response")
	}
	if !response.Writable || !response.Readable {
		return fmt.Sprintf(
			"checkpoint runtime probe reported incomplete access (writable=%t, readable=%t)",
			response.Writable,
			response.Readable,
		)
	}
	logger.InfoF(ctx, "[Install] checkpoint runtime probe succeeded: install_dir=%s", params.InstallDir)
	return ""
}

func (m *InstallerManager) maybeProbeIMAPRuntimeStorage(ctx context.Context, params *InstallParams) string {
	if params == nil || params.IMAP == nil || !isRemoteIMAPStorage(params.IMAP.StorageType) {
		return ""
	}
	request, err := buildIMAPRuntimeProbeRequest(params)
	if err != nil {
		return fmt.Sprintf("failed to build IMAP probe request: %v", err)
	}
	response, err := m.executeRuntimeStorageProbe(ctx, params.InstallDir, params.Version, "imap", params.JavaProxyPort, request)
	if err != nil {
		logger.WarnF(ctx, "[Install] IMAP runtime probe execution failed: install_dir=%s, error=%v", params.InstallDir, err)
		return err.Error()
	}
	if !response.OK {
		return firstNonBlank(response.Message, "IMAP runtime probe returned a non-success response")
	}
	if !response.Writable || !response.Readable {
		return fmt.Sprintf(
			"IMAP runtime probe reported incomplete access (writable=%t, readable=%t)",
			response.Writable,
			response.Readable,
		)
	}
	logger.InfoF(ctx, "[Install] IMAP runtime probe succeeded: install_dir=%s", params.InstallDir)
	return ""
}

func buildCheckpointRuntimeProbeRequest(cfg *CheckpointConfig) (map[string]interface{}, error) {
	pluginConfig, err := buildCheckpointPluginConfig(cfg)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"plugin":         "hdfs",
		"mode":           "read_write",
		"probeTimeoutMs": runtimeProbeTimeout.Milliseconds(),
		"config":         pluginConfig,
	}, nil
}

func buildIMAPRuntimeProbeRequest(params *InstallParams) (map[string]interface{}, error) {
	clusterName := resolveRuntimeProbeClusterName(params.InstallDir, params.DeploymentMode)
	properties, err := buildIMAPProperties(
		params.IMAP,
		normalizeRuntimeStorageNamespace(params.IMAP.Namespace),
		clusterName,
	)
	if err != nil {
		return nil, err
	}
	config := make(map[string]interface{}, len(properties)+1)
	for key, value := range properties {
		config[key] = value
	}
	config["businessName"] = runtimeProbeBusinessName

	return map[string]interface{}{
		"plugin":             "hdfs",
		"mode":               "read_write",
		"deleteAllOnDestroy": true,
		"probeTimeoutMs":     runtimeProbeTimeout.Milliseconds(),
		"config":             config,
	}, nil
}

func buildCheckpointRuntimeStatRequest(cfg *CheckpointConfig) (map[string]interface{}, error) {
	pluginConfig, err := buildCheckpointPluginConfig(cfg)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"plugin": "hdfs",
		"config": pluginConfig,
	}, nil
}

func buildIMAPRuntimeStatRequest(params *InstallParams) (map[string]interface{}, error) {
	clusterName := resolveRuntimeProbeClusterName(params.InstallDir, params.DeploymentMode)
	properties, err := buildIMAPProperties(
		params.IMAP,
		normalizeRuntimeStorageNamespace(params.IMAP.Namespace),
		clusterName,
	)
	if err != nil {
		return nil, err
	}
	config := make(map[string]interface{}, len(properties)+1)
	for key, value := range properties {
		config[key] = value
	}
	config["businessName"] = runtimeProbeBusinessName
	return map[string]interface{}{
		"plugin": "hdfs",
		"config": config,
	}, nil
}

func (m *InstallerManager) executeRuntimeStorageProbe(
	ctx context.Context,
	installDir string,
	seatunnelVersion string,
	kind string,
	javaProxyPort int,
	request map[string]interface{},
) (*runtimeStorageProbeResponse, error) {
	response, err := m.executeRuntimeStorageProbeViaManagedService(ctx, installDir, seatunnelVersion, kind, javaProxyPort, request)
	if err == nil && response != nil {
		return response, nil
	}
	if err != nil {
		logger.WarnF(
			ctx,
			"[Install] managed stx-java-proxy service unavailable, falling back to probe-once CLI: install_dir=%s, kind=%s, error=%v",
			installDir,
			kind,
			err,
		)
	}

	return m.executeRuntimeStorageProbeWithCLI(ctx, installDir, seatunnelVersion, kind, javaProxyPort, request)
}

func (m *InstallerManager) executeRuntimeStorageProbeViaManagedService(
	ctx context.Context,
	installDir string,
	seatunnelVersion string,
	kind string,
	javaProxyPort int,
	request map[string]interface{},
) (*runtimeStorageProbeResponse, error) {
	// 安装参数里的 java_proxy_port 必须传进来，否则会落到默认 18080。
	// Pass the install-configured java_proxy_port; otherwise startup falls back to default 18080.
	baseURL, err := ensureSTXJavaProxyService(ctx, installDir, seatunnelVersion, javaProxyPort)
	if err != nil {
		return nil, err
	}

	payload, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("marshal runtime probe request: %w", err)
	}

	probeCtx, cancel := context.WithTimeout(ctx, runtimeProbeTimeout+(5*time.Second))
	defer cancel()

	url := strings.TrimRight(baseURL, "/") + "/api/v1/storage/" + kind + "/probe"
	req, err := http.NewRequestWithContext(probeCtx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create managed stx-java-proxy request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("call managed stx-java-proxy service %s: %w", url, err)
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if readErr != nil {
		return nil, fmt.Errorf("read managed stx-java-proxy response: %w", readErr)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("managed stx-java-proxy returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("managed stx-java-proxy returned an empty response")
	}

	var response runtimeStorageProbeResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("parse managed stx-java-proxy response: %w", err)
	}
	return &response, nil
}

func (m *InstallerManager) executeRuntimeStorageProbeWithCLI(
	ctx context.Context,
	installDir string,
	seatunnelVersion string,
	kind string,
	javaProxyPort int,
	request map[string]interface{},
) (*runtimeStorageProbeResponse, error) {
	scriptPath, err := resolveSTXJavaProxyScriptPath(installDir)
	if err != nil {
		return nil, err
	}
	jarPath, err := resolveSTXJavaProxyJarPath(installDir, seatunnelVersion)
	if err != nil {
		return nil, err
	}

	tempDir, err := os.MkdirTemp("", "seatunnel-runtime-probe-*")
	if err != nil {
		return nil, fmt.Errorf("create runtime probe temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	requestPath := filepath.Join(tempDir, "request.json")
	responsePath := filepath.Join(tempDir, "response.json")
	payload, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("marshal runtime probe request: %w", err)
	}
	if err := os.WriteFile(requestPath, payload, 0600); err != nil {
		return nil, fmt.Errorf("write runtime probe request file: %w", err)
	}

	probeCtx, cancel := context.WithTimeout(ctx, runtimeProbeTimeout+(5*time.Second))
	defer cancel()

	cmd := exec.CommandContext(
		probeCtx,
		"bash",
		scriptPath,
		"probe-once",
		kind,
		"--request-file",
		requestPath,
		"--response-file",
		responsePath,
	)
	env := []string{
		fmt.Sprintf("SEATUNNEL_HOME=%s", installDir),
		fmt.Sprintf("%s=%s", stxJavaProxyJarEnvVar, jarPath),
		fmt.Sprintf("%s=%s", stxJavaProxyVersionEnvVar, defaultSTXJavaProxyVersion(seatunnelVersion)),
	}
	if javaProxyPort > 0 {
		env = append(env, fmt.Sprintf("%s=%d", stxJavaProxyPortEnvVar, javaProxyPort))
	}
	if jvmOpts := strings.TrimSpace(os.Getenv(stxJavaProxyJvmOptsEnvVar)); jvmOpts != "" {
		env = append(env, fmt.Sprintf("%s=%s", stxJavaProxyJvmOptsEnvVar, jvmOpts))
	}
	cmd.Env = append(os.Environ(), env...)
	output, execErr := cmd.CombinedOutput()

	response, responseErr := readRuntimeStorageProbeResponse(responsePath)
	if responseErr == nil && response != nil {
		return response, nil
	}
	if execErr != nil {
		return nil, fmt.Errorf(
			"run stx-java-proxy probe with script %s and jar %s: %v: %s",
			scriptPath,
			jarPath,
			execErr,
			strings.TrimSpace(string(output)),
		)
	}
	if responseErr != nil {
		return nil, responseErr
	}
	return nil, fmt.Errorf("runtime probe returned no response")
}

func stxJavaProxyPortFromContext(ctx context.Context) int {
	if ctx == nil {
		return 0
	}
	port, _ := ctx.Value(stxJavaProxyPortContextKey{}).(int)
	return port
}

type stxJavaProxyPortContextKey struct{}

// ContextWithSTXJavaProxyPort 把集群配置的 java-proxy 端口放进上下文，供后续托管启动读取。
// ContextWithSTXJavaProxyPort stores the configured java-proxy port for later managed startup.
func ContextWithSTXJavaProxyPort(ctx context.Context, port int) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if port <= 0 {
		return ctx
	}
	return context.WithValue(ctx, stxJavaProxyPortContextKey{}, port)
}

func ensureSTXJavaProxyService(
	ctx context.Context,
	installDir string,
	seatunnelVersion string,
	preferredPort int,
	optionalJvmOpts ...string,
) (string, error) {
	// 命令上下文里的端口优先于默认值，安装参数显式传入时仍然最高。
	// A port carried on the command context overrides the default. An explicit argument still wins.
	if preferredPort <= 0 {
		preferredPort = stxJavaProxyPortFromContext(ctx)
	}
	if endpoint := strings.TrimSpace(os.Getenv(stxJavaProxyEndpointEnvVar)); endpoint != "" {
		normalized := strings.TrimRight(endpoint, "/")
		if err := waitForSTXJavaProxyHealthy(ctx, normalized, 2*time.Second); err != nil {
			return "", fmt.Errorf("configured stx-java-proxy endpoint %s is unhealthy: %w", normalized, err)
		}
		return normalized, nil
	}

	if !fileExists(filepath.Join(installDir, "starter", "seatunnel-starter.jar")) {
		return "", fmt.Errorf("seatunnel runtime is unavailable under %s; managed stx-java-proxy service requires extracted runtime", installDir)
	}

	scriptPath, err := resolveSTXJavaProxyScriptPath(installDir)
	if err != nil {
		return "", err
	}
	jarPath, err := resolveSTXJavaProxyJarPath(installDir, seatunnelVersion)
	if err != nil {
		return "", err
	}

	stateDir := stxJavaProxyServiceStateDir(installDir)
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return "", fmt.Errorf("create stx-java-proxy state dir: %w", err)
	}

	// Persist preferred jvm_opts if provided
	var jvmOpts string
	if len(optionalJvmOpts) > 0 && strings.TrimSpace(optionalJvmOpts[0]) != "" {
		jvmOpts = strings.TrimSpace(optionalJvmOpts[0])
		_ = os.WriteFile(filepath.Join(stateDir, "service.jvm_opts"), []byte(jvmOpts+"\n"), 0o644)
	} else if bytes, err := os.ReadFile(filepath.Join(stateDir, "service.jvm_opts")); err == nil {
		jvmOpts = strings.TrimSpace(string(bytes))
	}

	// 用户/集群指定端口优先写入，后续候选与启动都会认这个端口。
	// Persist the preferred port first so later candidates / startup honor it.
	if preferredPort > 0 {
		_ = os.WriteFile(filepath.Join(stateDir, "service.port"), []byte(strconv.Itoa(preferredPort)+"\n"), 0o644)
	}

	// 指定端口时只认该端口是否已健康，避免误复用旧端口上的实例。
	// When a preferred port is set, only accept that port as already healthy.
	checkPorts := stxJavaProxyPortCandidates(stateDir, preferredPort)
	if preferredPort > 0 {
		checkPorts = []int{preferredPort}
	}
	for _, port := range checkPorts {
		if port <= 0 {
			continue
		}
		baseURL := stxJavaProxyServiceBaseURL(port)
		if err := waitForSTXJavaProxyHealthy(ctx, baseURL, 1500*time.Millisecond); err == nil {
			_ = os.WriteFile(filepath.Join(stateDir, "service.port"), []byte(strconv.Itoa(port)+"\n"), 0o644)
			return baseURL, nil
		}
	}

	port := stxJavaProxyPreferredPort(stateDir, preferredPort)
	baseURL, err := startSTXJavaProxyService(ctx, installDir, seatunnelVersion, scriptPath, jarPath, stateDir, port, jvmOpts)
	if err == nil {
		return baseURL, nil
	}
	if preferredPort > 0 || os.Getenv(stxJavaProxyPortEnvVar) != "" {
		return "", err
	}

	fallbackPort, portErr := findOpenSTXJavaProxyPort()
	if portErr != nil || fallbackPort == port {
		return "", err
	}
	return startSTXJavaProxyService(ctx, installDir, seatunnelVersion, scriptPath, jarPath, stateDir, fallbackPort, jvmOpts)
}

func startSTXJavaProxyService(
	ctx context.Context,
	installDir string,
	seatunnelVersion string,
	scriptPath string,
	jarPath string,
	stateDir string,
	port int,
	optionalJvmOpts ...string,
) (string, error) {
	logPath := filepath.Join(stateDir, "service.log")
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		if err := os.WriteFile(logPath, []byte{}, 0o644); err != nil {
			return "", fmt.Errorf("create stx-java-proxy log file: %w", err)
		}
	}

	command := fmt.Sprintf(
		"nohup bash %q -Dstx.java.proxy.port=%d >> %q 2>&1 < /dev/null & echo $!",
		scriptPath,
		port,
		logPath,
	)
	startCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(startCtx, "bash", "-lc", command)
	env := append(
		os.Environ(),
		fmt.Sprintf("SEATUNNEL_HOME=%s", installDir),
		fmt.Sprintf("%s=%s", stxJavaProxyHomeEnvVar, resolveSTXJavaProxyHome(installDir)),
		fmt.Sprintf("%s=%s", stxJavaProxyJarEnvVar, jarPath),
		fmt.Sprintf("%s=%d", stxJavaProxyPortEnvVar, port),
		fmt.Sprintf("%s=%s", stxJavaProxyVersionEnvVar, defaultSTXJavaProxyVersion(seatunnelVersion)),
	)
	effectiveJvmOpts := ""
	if len(optionalJvmOpts) > 0 && strings.TrimSpace(optionalJvmOpts[0]) != "" {
		effectiveJvmOpts = strings.TrimSpace(optionalJvmOpts[0])
	} else if bytes, err := os.ReadFile(filepath.Join(stateDir, "service.jvm_opts")); err == nil {
		effectiveJvmOpts = strings.TrimSpace(string(bytes))
	} else if envVal := strings.TrimSpace(os.Getenv(stxJavaProxyJvmOptsEnvVar)); envVal != "" {
		effectiveJvmOpts = envVal
	}
	if effectiveJvmOpts != "" {
		env = append(env, fmt.Sprintf("%s=%s", stxJavaProxyJvmOptsEnvVar, effectiveJvmOpts))
	}
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("start managed stx-java-proxy service: %v: %s", err, strings.TrimSpace(string(output)))
	}

	pidText := strings.TrimSpace(string(output))
	if pidText != "" {
		_ = os.WriteFile(filepath.Join(stateDir, "service.pid"), []byte(pidText+"\n"), 0o644)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "service.port"), []byte(strconv.Itoa(port)+"\n"), 0o644); err != nil {
		return "", fmt.Errorf("persist stx-java-proxy port: %w", err)
	}

	baseURL := stxJavaProxyServiceBaseURL(port)
	if err := waitForSTXJavaProxyHealthy(ctx, baseURL, stxJavaProxyStartupWait); err != nil {
		return "", fmt.Errorf("wait for managed stx-java-proxy service on %s: %w", baseURL, err)
	}
	return baseURL, nil
}

func probeSTXJavaProxyHealth(ctx context.Context, baseURL string, timeout time.Duration) (error, map[string]interface{}) {
	healthURL := strings.TrimRight(baseURL, "/") + stxJavaProxyHealthPath
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 1500 * time.Millisecond}
	var lastErr error

	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
		if err != nil {
			return err, nil
		}

		resp, err := client.Do(req)
		if err == nil {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				var payload map[string]interface{}
				var jvmMemory map[string]interface{}
				if json.Unmarshal(body, &payload) == nil {
					if mem, ok := payload["jvmMemory"].(map[string]interface{}); ok {
						jvmMemory = mem
					}
				}
				return nil, jvmMemory
			}
			lastErr = fmt.Errorf("unexpected status %d", resp.StatusCode)
		} else {
			lastErr = err
		}

		if time.Now().After(deadline) {
			if lastErr == nil {
				lastErr = fmt.Errorf("timed out waiting for stx-java-proxy health")
			}
			return lastErr, nil
		}

		select {
		case <-ctx.Done():
			if lastErr == nil {
				lastErr = ctx.Err()
			}
			return lastErr, nil
		case <-time.After(300 * time.Millisecond):
		}
	}
}

func waitForSTXJavaProxyHealthy(ctx context.Context, baseURL string, timeout time.Duration) error {
	err, _ := probeSTXJavaProxyHealth(ctx, baseURL, timeout)
	return err
}

func stxJavaProxyServiceStateDir(installDir string) string {
	// 状态与日志统一落在 Agent 主目录下，不再挂到 SeaTunnel installDir/.stx。
	// Persist state/logs under Agent home instead of SeaTunnel installDir/.stx.
	return filepath.Join(resolveSTXJavaProxyHome(installDir), "logs", stxJavaProxyServiceDirName)
}

// resolveSTXJavaProxyHome 解析 Agent 主目录（jar/scripts/logs 的统一根）。
// 优先 STX_JAVA_PROXY_HOME，其次脚本所在 Agent 布局，再回退 ~/.stx/agent 与系统默认目录。
// resolveSTXJavaProxyHome resolves the Agent home used for jar/scripts/logs.
// Prefer STX_JAVA_PROXY_HOME, then the Agent layout that owns the script, then ~/.stx/agent / system default.
func resolveSTXJavaProxyHome(seatunnelInstallDir string) string {
	if home := strings.TrimSpace(os.Getenv(stxJavaProxyHomeEnvVar)); home != "" {
		return filepath.Clean(home)
	}
	if script, err := resolveSTXJavaProxyScriptPath(seatunnelInstallDir); err == nil {
		scriptsDir := filepath.Dir(script)
		if filepath.Base(scriptsDir) == "scripts" {
			return filepath.Clean(filepath.Dir(scriptsDir))
		}
	}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		candidate := filepath.Join(home, stxJavaProxyUserSupportDirName)
		if dirExists(candidate) {
			return candidate
		}
	}
	if dirExists(stxJavaProxyDefaultSupportDir) {
		return stxJavaProxyDefaultSupportDir
	}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		return filepath.Join(home, stxJavaProxyUserSupportDirName)
	}
	return stxJavaProxyDefaultSupportDir
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func stxJavaProxyServiceBaseURL(port int) string {
	return fmt.Sprintf("http://%s:%d", stxJavaProxyDefaultHost, port)
}

func stxJavaProxyPortCandidates(stateDir string, preferredPort int) []int {
	candidates := make([]int, 0, 4)
	// 用户/集群显式端口优先于环境变量与落盘记录。
	// Explicit user/cluster port takes precedence over env and persisted state.
	if preferredPort > 0 {
		candidates = append(candidates, preferredPort)
	}
	if port, ok := parseSTXJavaProxyPort(strings.TrimSpace(os.Getenv(stxJavaProxyPortEnvVar))); ok {
		candidates = append(candidates, port)
	}
	if bytes, err := os.ReadFile(filepath.Join(stateDir, "service.port")); err == nil {
		if port, ok := parseSTXJavaProxyPort(strings.TrimSpace(string(bytes))); ok {
			candidates = append(candidates, port)
		}
	}
	candidates = append(candidates, stxJavaProxyDefaultPort)

	seen := make(map[int]struct{}, len(candidates))
	result := make([]int, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate <= 0 {
			continue
		}
		if _, exists := seen[candidate]; exists {
			continue
		}
		seen[candidate] = struct{}{}
		result = append(result, candidate)
	}
	return result
}

func stxJavaProxyPreferredPort(stateDir string, preferredPort int) int {
	candidates := stxJavaProxyPortCandidates(stateDir, preferredPort)
	if len(candidates) > 0 {
		return candidates[0]
	}
	return stxJavaProxyDefaultPort
}

func parseSTXJavaProxyPort(value string) (int, bool) {
	if strings.TrimSpace(value) == "" {
		return 0, false
	}
	port, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || port <= 0 || port > 65535 {
		return 0, false
	}
	return port, true
}

func findOpenSTXJavaProxyPort() (int, error) {
	listener, err := net.Listen("tcp", net.JoinHostPort(stxJavaProxyDefaultHost, "0"))
	if err != nil {
		return 0, err
	}
	defer listener.Close()

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok || addr.Port <= 0 {
		return 0, fmt.Errorf("failed to resolve stx-java-proxy port from listener address")
	}
	return addr.Port, nil
}

func readRuntimeStorageProbeResponse(path string) (*runtimeStorageProbeResponse, error) {
	bytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read runtime probe response file: %w", err)
	}
	if len(bytes) == 0 {
		return nil, fmt.Errorf("runtime probe response file is empty")
	}
	var response runtimeStorageProbeResponse
	if err := json.Unmarshal(bytes, &response); err != nil {
		return nil, fmt.Errorf("parse runtime probe response: %w", err)
	}
	return &response, nil
}

func executeRuntimeStorageStatViaManagedService(
	ctx context.Context,
	installDir string,
	seatunnelVersion string,
	kind string,
	request map[string]interface{},
) (*RuntimeStorageStatResult, error) {
	baseURL, err := ensureSTXJavaProxyService(ctx, installDir, seatunnelVersion, 0)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("marshal runtime stat request: %w", err)
	}
	statCtx, cancel := context.WithTimeout(ctx, runtimeProbeTimeout+(5*time.Second))
	defer cancel()
	url := strings.TrimRight(baseURL, "/") + "/api/v1/storage/" + kind + "/stat"
	req, err := http.NewRequestWithContext(statCtx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create managed stx-java-proxy stat request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("call managed stx-java-proxy stat service %s: %w", url, err)
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if readErr != nil {
		return nil, fmt.Errorf("read managed stx-java-proxy stat response: %w", readErr)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("managed stx-java-proxy stat returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("managed stx-java-proxy stat returned an empty response")
	}
	var result RuntimeStorageStatResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse managed stx-java-proxy stat response: %w", err)
	}
	return &result, nil
}

func buildCheckpointRuntimeListRequest(cfg *CheckpointConfig, path string, recursive bool, limit int) (map[string]interface{}, error) {
	request, err := buildCheckpointRuntimeStatRequest(cfg)
	if err != nil {
		return nil, err
	}
	fillRuntimeStorageListRequest(request, path, recursive, limit)
	return request, nil
}

func buildIMAPRuntimeListRequest(cfg *IMAPConfig, installDir string, seatunnelVersion string, path string, recursive bool, limit int) (map[string]interface{}, error) {
	request, err := buildIMAPRuntimeStatRequest(&InstallParams{
		InstallDir: installDir,
		Version:    seatunnelVersion,
		IMAP:       cfg,
	})
	if err != nil {
		return nil, err
	}
	fillRuntimeStorageListRequest(request, path, recursive, limit)
	return request, nil
}

func fillRuntimeStorageListRequest(request map[string]interface{}, path string, recursive bool, limit int) {
	if strings.TrimSpace(path) != "" {
		request["path"] = strings.TrimSpace(path)
	}
	request["recursive"] = recursive
	if limit > 0 {
		request["limit"] = limit
	}
}

func executeRuntimeStorageListViaManagedService(
	ctx context.Context,
	installDir string,
	seatunnelVersion string,
	kind string,
	request map[string]interface{},
) (*RuntimeStorageListResult, error) {
	baseURL, err := ensureSTXJavaProxyService(ctx, installDir, seatunnelVersion, 0)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("marshal runtime list request: %w", err)
	}
	listCtx, cancel := context.WithTimeout(ctx, runtimeProbeTimeout+(10*time.Second))
	defer cancel()
	url := strings.TrimRight(baseURL, "/") + "/api/v1/storage/" + kind + "/list"
	req, err := http.NewRequestWithContext(listCtx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create managed stx-java-proxy list request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("call managed stx-java-proxy list service %s: %w", url, err)
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if readErr != nil {
		return nil, fmt.Errorf("read managed stx-java-proxy list response: %w", readErr)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("managed stx-java-proxy list returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("managed stx-java-proxy list returned an empty response")
	}
	var result RuntimeStorageListResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse managed stx-java-proxy list response: %w", err)
	}
	return &result, nil
}

func ExecuteCheckpointRuntimeStorageList(
	ctx context.Context,
	installDir string,
	seatunnelVersion string,
	cfg *CheckpointConfig,
	path string,
	recursive bool,
	limit int,
) (*RuntimeStorageListResult, error) {
	request, err := buildCheckpointRuntimeListRequest(cfg, path, recursive, limit)
	if err != nil {
		return nil, err
	}
	return executeRuntimeStorageListViaManagedService(ctx, installDir, seatunnelVersion, "checkpoint", request)
}

func ExecuteIMAPRuntimeStorageList(
	ctx context.Context,
	installDir string,
	seatunnelVersion string,
	cfg *IMAPConfig,
	path string,
	recursive bool,
	limit int,
) (*RuntimeStorageListResult, error) {
	request, err := buildIMAPRuntimeListRequest(cfg, installDir, seatunnelVersion, path, recursive, limit)
	if err != nil {
		return nil, err
	}
	return executeRuntimeStorageListViaManagedService(ctx, installDir, seatunnelVersion, "imap", request)
}

func ExecuteCheckpointRuntimeStorageProbe(
	ctx context.Context,
	installDir string,
	seatunnelVersion string,
	cfg *CheckpointConfig,
) (*RuntimeStorageProbeResult, error) {
	request, err := buildCheckpointRuntimeProbeRequest(cfg)
	if err != nil {
		return nil, err
	}
	manager := &InstallerManager{}
	resp, err := manager.executeRuntimeStorageProbe(ctx, installDir, seatunnelVersion, "checkpoint", 0, request)
	if err != nil {
		return nil, err
	}
	return &RuntimeStorageProbeResult{
		OK:         resp.OK,
		StatusCode: resp.StatusCode,
		Message:    resp.Message,
		Writable:   resp.Writable,
		Readable:   resp.Readable,
	}, nil
}

func ExecuteIMAPRuntimeStorageProbe(
	ctx context.Context,
	installDir string,
	seatunnelVersion string,
	cfg *IMAPConfig,
) (*RuntimeStorageProbeResult, error) {
	request, err := buildIMAPRuntimeProbeRequest(&InstallParams{
		InstallDir: installDir,
		Version:    seatunnelVersion,
		IMAP:       cfg,
	})
	if err != nil {
		return nil, err
	}
	manager := &InstallerManager{}
	resp, err := manager.executeRuntimeStorageProbe(ctx, installDir, seatunnelVersion, "imap", 0, request)
	if err != nil {
		return nil, err
	}
	return &RuntimeStorageProbeResult{
		OK:         resp.OK,
		StatusCode: resp.StatusCode,
		Message:    resp.Message,
		Writable:   resp.Writable,
		Readable:   resp.Readable,
	}, nil
}

func ExecuteCheckpointRuntimeStorageStat(
	ctx context.Context,
	installDir string,
	seatunnelVersion string,
	cfg *CheckpointConfig,
) (*RuntimeStorageStatResult, error) {
	request, err := buildCheckpointRuntimeStatRequest(cfg)
	if err != nil {
		return nil, err
	}
	return executeRuntimeStorageStatViaManagedService(ctx, installDir, seatunnelVersion, "checkpoint", request)
}

func ExecuteIMAPRuntimeStorageStat(
	ctx context.Context,
	installDir string,
	seatunnelVersion string,
	cfg *IMAPConfig,
) (*RuntimeStorageStatResult, error) {
	request, err := buildIMAPRuntimeStatRequest(&InstallParams{
		InstallDir: installDir,
		Version:    seatunnelVersion,
		IMAP:       cfg,
	})
	if err != nil {
		return nil, err
	}
	return executeRuntimeStorageStatViaManagedService(ctx, installDir, seatunnelVersion, "imap", request)
}

func isRemoteCheckpointStorage(storageType CheckpointStorageType) bool {
	return storageType == CheckpointStorageHDFS ||
		storageType == CheckpointStorageOSS ||
		storageType == CheckpointStorageS3
}

func isRemoteIMAPStorage(storageType IMAPStorageType) bool {
	return storageType == IMAPStorageHDFS ||
		storageType == IMAPStorageOSS ||
		storageType == IMAPStorageS3
}

func resolveRuntimeProbeClusterName(installDir string, deploymentMode DeploymentMode) string {
	configFiles := []string{
		filepath.Join(installDir, "config", "hazelcast.yaml"),
		filepath.Join(installDir, "config", "hazelcast-master.yaml"),
		filepath.Join(installDir, "config", "hazelcast-worker.yaml"),
	}
	if deploymentMode == DeploymentModeSeparated {
		configFiles = []string{
			filepath.Join(installDir, "config", "hazelcast-master.yaml"),
			filepath.Join(installDir, "config", "hazelcast-worker.yaml"),
			filepath.Join(installDir, "config", "hazelcast.yaml"),
		}
	}

	for _, configFile := range configFiles {
		content, err := os.ReadFile(configFile)
		if err != nil {
			continue
		}
		var root yaml.Node
		if err := yaml.Unmarshal(content, &root); err != nil {
			continue
		}
		documentRoot := ensureDocumentMappingNode(&root)
		hazelcastNode := findMappingChildNode(documentRoot, "hazelcast")
		if hazelcastNode == nil {
			continue
		}
		clusterName := strings.TrimSpace(getMappingString(hazelcastNode, "cluster-name"))
		if clusterName != "" {
			return clusterName
		}
	}

	return runtimeProbeClusterName
}

func resolveSTXJavaProxyScriptPath(installDir string) (string, error) {
	if envPath := strings.TrimSpace(os.Getenv(stxJavaProxyScriptEnvVar)); envPath != "" {
		if fileExists(envPath) {
			return envPath, nil
		}
		return "", fmt.Errorf("stx-java-proxy script not found at %s", envPath)
	}

	for _, candidate := range stxJavaProxyScriptCandidates(installDir) {
		if fileExists(candidate) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("stx-java-proxy script is unavailable")
}

func resolveSTXJavaProxyJarPath(installDir string, seatunnelVersion string) (string, error) {
	if envPath := strings.TrimSpace(os.Getenv(stxJavaProxyJarEnvVar)); envPath != "" {
		if fileExists(envPath) {
			return envPath, nil
		}
		return "", fmt.Errorf("stx-java-proxy jar not found at %s", envPath)
	}

	for _, candidate := range stxJavaProxyJarCandidates(installDir, seatunnelVersion) {
		if strings.Contains(candidate, "*") {
			matches, _ := filepath.Glob(candidate)
			sort.Strings(matches)
			for _, match := range matches {
				if fileExists(match) && !strings.HasSuffix(match, "-bin.jar") {
					return match, nil
				}
			}
			continue
		}
		if fileExists(candidate) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("stx-java-proxy jar is unavailable")
}

func stxJavaProxyScriptCandidates(installDir string) []string {
	candidates := make([]string, 0, 10)
	if homeDir := strings.TrimSpace(os.Getenv(stxJavaProxyHomeEnvVar)); homeDir != "" {
		candidates = append(candidates, filepath.Join(homeDir, "scripts", seatunnelmeta.STXJavaProxyScriptFileName))
	}
	candidates = append(candidates,
		filepath.Join(stxJavaProxyDefaultSupportDir, "scripts", seatunnelmeta.STXJavaProxyScriptFileName),
		filepath.Join(installDir, "scripts", "stx-java-proxy.sh"),
		filepath.Join(installDir, "bin", "stx-java-proxy.sh"),
		filepath.Join("scripts", "stx-java-proxy.sh"),
		filepath.Join("tools", "stx-java-proxy", "bin", "stx-java-proxy.sh"),
	)
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		candidates = append(candidates, filepath.Join(home, stxJavaProxyUserSupportDirName, "scripts", seatunnelmeta.STXJavaProxyScriptFileName))
	}
	if executable, err := os.Executable(); err == nil {
		execDir := filepath.Dir(executable)
		candidates = append(
			candidates,
			filepath.Join(execDir, "..", "lib", "stx-agent", "scripts", seatunnelmeta.STXJavaProxyScriptFileName),
			filepath.Join(execDir, "..", "..", "scripts", seatunnelmeta.STXJavaProxyScriptFileName),
			filepath.Join(execDir, "..", "scripts", seatunnelmeta.STXJavaProxyScriptFileName),
			filepath.Join(execDir, "tools", "stx-java-proxy", "bin", "stx-java-proxy.sh"),
			filepath.Join(execDir, "..", "tools", "stx-java-proxy", "bin", "stx-java-proxy.sh"),
			filepath.Join(execDir, "..", "..", "tools", "stx-java-proxy", "bin", "stx-java-proxy.sh"),
		)
	}
	return dedupeStrings(candidates)
}

func stxJavaProxyJarCandidates(installDir string, seatunnelVersion string) []string {
	candidates := make([]string, 0, 16)
	for _, libDir := range stxJavaProxyLibDirCandidates(installDir) {
		for _, version := range stxJavaProxyVersionCandidates(seatunnelVersion) {
			candidates = append(candidates, filepath.Join(libDir, seatunnelmeta.STXJavaProxyJarFileName(version)))
		}
		candidates = append(candidates, filepath.Join(libDir, "stx-java-proxy.jar"))
	}
	candidates = append(candidates, filepath.Join(installDir, "tools", "stx-java-proxy.jar"))
	for _, targetDir := range stxJavaProxyDevelopmentJarDirs() {
		for _, version := range stxJavaProxyVersionCandidates(seatunnelVersion) {
			candidates = append(candidates, filepath.Join(targetDir, fmt.Sprintf("stx-java-proxy-%s*.jar", version)))
		}
		candidates = append(candidates, filepath.Join(targetDir, "stx-java-proxy-*.jar"))
	}
	return dedupeStrings(candidates)
}

func stxJavaProxyLibDirCandidates(installDir string) []string {
	candidates := make([]string, 0, 9)
	if homeDir := strings.TrimSpace(os.Getenv(stxJavaProxyHomeEnvVar)); homeDir != "" {
		candidates = append(candidates, filepath.Join(homeDir, "lib"))
	}
	candidates = append(candidates, filepath.Join(stxJavaProxyDefaultSupportDir, "lib"), filepath.Join(installDir, "lib"), "lib")
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		candidates = append(candidates, filepath.Join(home, stxJavaProxyUserSupportDirName, "lib"))
	}
	if executable, err := os.Executable(); err == nil {
		execDir := filepath.Dir(executable)
		candidates = append(
			candidates,
			filepath.Join(execDir, "..", "lib", "stx-agent", "lib"),
			filepath.Join(execDir, ".."),
			filepath.Join(execDir, "..", "..", "lib"),
		)
	}
	return dedupeStrings(candidates)
}

func stxJavaProxyDevelopmentJarDirs() []string {
	candidates := []string{
		filepath.Join("tools", "stx-java-proxy", "target"),
	}
	if executable, err := os.Executable(); err == nil {
		execDir := filepath.Dir(executable)
		candidates = append(
			candidates,
			filepath.Join(execDir, "tools", "stx-java-proxy", "target"),
			filepath.Join(execDir, "..", "tools", "stx-java-proxy", "target"),
			filepath.Join(execDir, "..", "..", "tools", "stx-java-proxy", "target"),
		)
	}
	return dedupeStrings(candidates)
}

func stxJavaProxyVersionCandidates(seatunnelVersion string) []string {
	candidates := []string{}
	if version := strings.TrimSpace(seatunnelVersion); version != "" {
		candidates = append(candidates, version)
	}
	candidates = append(candidates, seatunnelmeta.DefaultSTXJavaProxyVersion)
	return dedupeStrings(candidates)
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		clean := filepath.Clean(value)
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		result = append(result, clean)
	}
	return result
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func buildCheckpointRuntimePreviewRequest(cfg *CheckpointConfig, path string, maxBytes int) (map[string]interface{}, error) {
	request, err := buildCheckpointRuntimeStatRequest(cfg)
	if err != nil {
		return nil, err
	}
	fillRuntimeStoragePreviewRequest(request, path, maxBytes)
	return request, nil
}

func buildIMAPRuntimePreviewRequest(cfg *IMAPConfig, installDir string, seatunnelVersion string, path string, maxBytes int) (map[string]interface{}, error) {
	request, err := buildIMAPRuntimeStatRequest(&InstallParams{
		InstallDir: installDir,
		Version:    seatunnelVersion,
		IMAP:       cfg,
	})
	if err != nil {
		return nil, err
	}
	fillRuntimeStoragePreviewRequest(request, path, maxBytes)
	return request, nil
}

func fillRuntimeStoragePreviewRequest(request map[string]interface{}, path string, maxBytes int) {
	if strings.TrimSpace(path) != "" {
		request["path"] = strings.TrimSpace(path)
	}
	if maxBytes > 0 {
		request["maxBytes"] = maxBytes
	}
}

func executeRuntimeStoragePreviewViaManagedService(
	ctx context.Context,
	installDir string,
	seatunnelVersion string,
	kind string,
	request map[string]interface{},
) (*RuntimeStoragePreviewResult, error) {
	baseURL, err := ensureSTXJavaProxyService(ctx, installDir, seatunnelVersion, 0)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("marshal runtime preview request: %w", err)
	}
	previewCtx, cancel := context.WithTimeout(ctx, runtimeProbeTimeout+(10*time.Second))
	defer cancel()
	url := strings.TrimRight(baseURL, "/") + "/api/v1/storage/" + kind + "/preview"
	req, err := http.NewRequestWithContext(previewCtx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create managed stx-java-proxy preview request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("call managed stx-java-proxy preview service %s: %w", url, err)
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if readErr != nil {
		return nil, fmt.Errorf("read managed stx-java-proxy preview response: %w", readErr)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("managed stx-java-proxy preview returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("managed stx-java-proxy preview returned an empty response")
	}
	var result RuntimeStoragePreviewResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse managed stx-java-proxy preview response: %w", err)
	}
	return &result, nil
}

func executeCheckpointRuntimeStorageInspectViaManagedService(
	ctx context.Context,
	installDir string,
	seatunnelVersion string,
	request map[string]interface{},
) (*RuntimeStorageCheckpointInspectResult, error) {
	baseURL, err := ensureSTXJavaProxyService(ctx, installDir, seatunnelVersion, 0)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("marshal checkpoint inspect request: %w", err)
	}
	inspectCtx, cancel := context.WithTimeout(ctx, runtimeProbeTimeout+(10*time.Second))
	defer cancel()
	url := strings.TrimRight(baseURL, "/") + "/api/v1/storage/checkpoint/inspect"
	req, err := http.NewRequestWithContext(inspectCtx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create managed stx-java-proxy inspect request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("call managed stx-java-proxy inspect service %s: %w", url, err)
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if readErr != nil {
		return nil, fmt.Errorf("read managed stx-java-proxy inspect response: %w", readErr)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("managed stx-java-proxy inspect returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("managed stx-java-proxy inspect returned an empty response")
	}
	var result RuntimeStorageCheckpointInspectResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse managed stx-java-proxy inspect response: %w", err)
	}
	return &result, nil
}

func executeCheckpointRuntimeStorageInspectSourceStateViaManagedService(
	ctx context.Context,
	installDir string,
	seatunnelVersion string,
	request map[string]interface{},
) (*RuntimeStorageCheckpointSourceStateInspectResult, error) {
	baseURL, err := ensureSTXJavaProxyService(ctx, installDir, seatunnelVersion, 0)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("marshal checkpoint source state inspect request: %w", err)
	}
	inspectCtx, cancel := context.WithTimeout(ctx, runtimeProbeTimeout+(10*time.Second))
	defer cancel()
	url := strings.TrimRight(baseURL, "/") + "/api/v1/storage/checkpoint/inspect-source-state"
	req, err := http.NewRequestWithContext(inspectCtx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create managed stx-java-proxy source state inspect request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("call managed stx-java-proxy source state inspect service %s: %w", url, err)
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if readErr != nil {
		return nil, fmt.Errorf("read managed stx-java-proxy source state inspect response: %w", readErr)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("managed stx-java-proxy source state inspect returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("managed stx-java-proxy source state inspect returned an empty response")
	}
	var result RuntimeStorageCheckpointSourceStateInspectResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse managed stx-java-proxy source state inspect response: %w", err)
	}
	return &result, nil
}

func executeIMAPRuntimeStorageInspectViaManagedService(
	ctx context.Context,
	installDir string,
	seatunnelVersion string,
	request map[string]interface{},
) (*RuntimeStorageIMAPInspectResult, error) {
	baseURL, err := ensureSTXJavaProxyService(ctx, installDir, seatunnelVersion, 0)
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("marshal imap inspect request: %w", err)
	}
	inspectCtx, cancel := context.WithTimeout(ctx, runtimeProbeTimeout+(10*time.Second))
	defer cancel()
	url := strings.TrimRight(baseURL, "/") + "/api/v1/storage/imap/inspect-wal"
	req, err := http.NewRequestWithContext(inspectCtx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create managed stx-java-proxy imap inspect request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, fmt.Errorf("call managed stx-java-proxy imap inspect service %s: %w", url, err)
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if readErr != nil {
		return nil, fmt.Errorf("read managed stx-java-proxy imap inspect response: %w", readErr)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("managed stx-java-proxy imap inspect returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if len(body) == 0 {
		return nil, fmt.Errorf("managed stx-java-proxy imap inspect returned an empty response")
	}
	var result RuntimeStorageIMAPInspectResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse managed stx-java-proxy imap inspect response: %w", err)
	}
	return &result, nil
}

func ExecuteCheckpointRuntimeStoragePreview(
	ctx context.Context,
	installDir string,
	seatunnelVersion string,
	cfg *CheckpointConfig,
	path string,
	maxBytes int,
) (*RuntimeStoragePreviewResult, error) {
	request, err := buildCheckpointRuntimePreviewRequest(cfg, path, maxBytes)
	if err != nil {
		return nil, err
	}
	return executeRuntimeStoragePreviewViaManagedService(ctx, installDir, seatunnelVersion, "checkpoint", request)
}

func ExecuteIMAPRuntimeStoragePreview(
	ctx context.Context,
	installDir string,
	seatunnelVersion string,
	cfg *IMAPConfig,
	path string,
	maxBytes int,
) (*RuntimeStoragePreviewResult, error) {
	request, err := buildIMAPRuntimePreviewRequest(cfg, installDir, seatunnelVersion, path, maxBytes)
	if err != nil {
		return nil, err
	}
	return executeRuntimeStoragePreviewViaManagedService(ctx, installDir, seatunnelVersion, "imap", request)
}

func ExecuteCheckpointInspectFromBase64(
	ctx context.Context,
	installDir string,
	seatunnelVersion string, path string,
	contentBase64 string,
) (*RuntimeStorageCheckpointInspectResult, error) {
	request := map[string]interface{}{
		"path":          strings.TrimSpace(path),
		"fileName":      filepath.Base(strings.TrimSpace(path)),
		"contentBase64": strings.TrimSpace(contentBase64),
	}
	if jars := collectConnectorJars(installDir); len(jars) > 0 {
		request["pluginJars"] = jars
	}
	if strings.TrimSpace(seatunnelVersion) != "" {
		request["version"] = strings.TrimSpace(seatunnelVersion)
	}
	return executeCheckpointRuntimeStorageInspectViaManagedService(ctx, installDir, seatunnelVersion, request)
}

func collectConnectorJars(installDir string) []string {
	if strings.TrimSpace(installDir) == "" {
		return nil
	}
	var jars []string
	searchDirs := []string{
		filepath.Join(installDir, "connectors"),
		filepath.Join(installDir, "plugins"),
		filepath.Join(installDir, "lib"),
		filepath.Join(installDir, "starter"),
		filepath.Join(installDir, "seatunnel-dist"),
	}
	visited := make(map[string]bool)
	for _, rootDir := range searchDirs {
		if _, err := os.Stat(rootDir); err != nil {
			continue
		}
		_ = filepath.WalkDir(rootDir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			rel, _ := filepath.Rel(rootDir, path)
			if strings.Count(rel, string(filepath.Separator)) > 3 {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if !d.IsDir() && strings.HasSuffix(strings.ToLower(d.Name()), ".jar") {
				if !visited[path] {
					visited[path] = true
					jars = append(jars, path)
				}
			}
			return nil
		})
	}
	return jars
}

func ExecuteCheckpointInspectSourceState(
	ctx context.Context,
	installDir string,
	seatunnelVersion string,
	cfg *CheckpointConfig,
	path string,
	jobConfig map[string]interface{},
) (*RuntimeStorageCheckpointSourceStateInspectResult, error) {
	pluginConfig, err := buildCheckpointPluginConfig(cfg)
	if err != nil {
		return nil, err
	}
	request := map[string]interface{}{
		"config":    pluginConfig,
		"path":      strings.TrimSpace(path),
		"jobConfig": jobConfig,
	}
	if jars := collectConnectorJars(installDir); len(jars) > 0 {
		request["pluginJars"] = jars
	}
	if strings.TrimSpace(seatunnelVersion) != "" {
		request["version"] = strings.TrimSpace(seatunnelVersion)
	}
	return executeCheckpointRuntimeStorageInspectSourceStateViaManagedService(
		ctx, installDir, seatunnelVersion, request)
}

func ExecuteCheckpointInspectSourceStateFromBase64(
	ctx context.Context,
	installDir string,
	seatunnelVersion string,
	path string,
	contentBase64 string,
	jobConfig map[string]interface{},
) (*RuntimeStorageCheckpointSourceStateInspectResult, error) {
	request := map[string]interface{}{
		"path":          strings.TrimSpace(path),
		"contentBase64": strings.TrimSpace(contentBase64),
		"jobConfig":     jobConfig,
	}
	if jars := collectConnectorJars(installDir); len(jars) > 0 {
		request["pluginJars"] = jars
	}
	if strings.TrimSpace(seatunnelVersion) != "" {
		request["version"] = strings.TrimSpace(seatunnelVersion)
	}
	return executeCheckpointRuntimeStorageInspectSourceStateViaManagedService(
		ctx, installDir, seatunnelVersion, request)
}

func ExecuteCheckpointInspect(
	ctx context.Context,
	installDir string,
	seatunnelVersion string,
	cfg *CheckpointConfig,
	path string,
) (*RuntimeStorageCheckpointInspectResult, error) {
	request, err := buildCheckpointRuntimeStatRequest(cfg)
	if err != nil {
		return nil, err
	}
	request["path"] = strings.TrimSpace(path)
	if jars := collectConnectorJars(installDir); len(jars) > 0 {
		request["pluginJars"] = jars
	}
	if strings.TrimSpace(seatunnelVersion) != "" {
		request["version"] = strings.TrimSpace(seatunnelVersion)
	}
	return executeCheckpointRuntimeStorageInspectViaManagedService(ctx, installDir, seatunnelVersion, request)
}

func ExecuteIMAPWALInspect(
	ctx context.Context,
	installDir string,
	seatunnelVersion string,
	cfg *IMAPConfig,
	path string,
) (*RuntimeStorageIMAPInspectResult, error) {
	request, err := buildIMAPRuntimePreviewRequest(cfg, installDir, seatunnelVersion, path, 8<<20)
	if err != nil {
		return nil, err
	}
	return executeIMAPRuntimeStorageInspectViaManagedService(ctx, installDir, seatunnelVersion, request)
}

func ExecuteIMAPWALInspectFromBase64(
	ctx context.Context,
	installDir string,
	seatunnelVersion string,
	path string,
	contentBase64 string,
) (*RuntimeStorageIMAPInspectResult, error) {
	request := map[string]interface{}{
		"path":          strings.TrimSpace(path),
		"fileName":      filepath.Base(strings.TrimSpace(path)),
		"contentBase64": strings.TrimSpace(contentBase64),
	}
	return executeIMAPRuntimeStorageInspectViaManagedService(ctx, installDir, seatunnelVersion, request)
}

func EncodeRuntimeStorageContentBase64(content []byte) string {
	return base64.StdEncoding.EncodeToString(content)
}
