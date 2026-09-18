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

// Package agent provides Agent distribution and management for the STX Control Plane.
// agent 包提供 STX Control Plane 的 Agent 分发和管理功能。
package agent

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/LeonYoah/stx/internal/config"
	"github.com/LeonYoah/stx/internal/logger"
	seatunnelmeta "github.com/LeonYoah/stx/internal/seatunnel"
	"github.com/gin-gonic/gin"
)

var stxJavaProxyVersionPattern = regexp.MustCompile(`^[0-9A-Za-z._-]+$`)

// Handler provides HTTP handlers for Agent distribution operations.
// Handler 提供 Agent 分发操作的 HTTP 处理器。
type Handler struct {
	// controlPlaneAddr is the address of the Control Plane for Agent to connect.
	// controlPlaneAddr 是 Agent 连接的 Control Plane 地址。
	controlPlaneAddr string

	// agentBinaryDir is the directory containing Agent binary files.
	// agentBinaryDir 是包含 Agent 二进制文件的目录。
	agentBinaryDir string

	// stxJavaProxyJarPath is the path to the packaged stx-java-proxy thin jar.
	// stxJavaProxyJarPath 是 stx-java-proxy 薄 jar 的打包路径。
	stxJavaProxyJarPath string

	// stxJavaProxyScriptPath is the path to the packaged stx-java-proxy launcher script.
	// stxJavaProxyScriptPath 是 stx-java-proxy 启动脚本的打包路径。
	stxJavaProxyScriptPath string

	// grpcPort is the gRPC port for Agent to connect.
	// grpcPort 是 Agent 连接的 gRPC 端口。
	grpcPort string

	// heartbeatInterval is the heartbeat interval in seconds from Control Plane config.
	// heartbeatInterval 是来自 Control Plane 配置的心跳间隔（秒）。
	heartbeatInterval int

	// tlsEnabled indicates whether Control Plane gRPC TLS is enabled.
	// tlsEnabled 表示 Control Plane gRPC TLS 是否已启用。
	tlsEnabled bool

	// caFile is the local CA certificate path for Agent trust (empty when TLS is off).
	// caFile 是供 Agent 信任的本地 CA 证书路径（TLS 关闭时为空）。
	caFile string
}

// HandlerConfig holds configuration for the Agent Handler.
// HandlerConfig 保存 Agent Handler 的配置。
type HandlerConfig struct {
	// ControlPlaneAddr is the address of the Control Plane.
	// ControlPlaneAddr 是 Control Plane 的地址。
	ControlPlaneAddr string

	// AgentBinaryDir is the directory containing Agent binary files.
	// AgentBinaryDir 是包含 Agent 二进制文件的目录。
	AgentBinaryDir string

	// STXJavaProxyJarPath is the path to the packaged stx-java-proxy thin jar.
	// STXJavaProxyJarPath 是 stx-java-proxy 薄 jar 的打包路径。
	STXJavaProxyJarPath string

	// STXJavaProxyScriptPath is the path to the packaged stx-java-proxy launcher script.
	// STXJavaProxyScriptPath 是 stx-java-proxy 启动脚本的打包路径。
	STXJavaProxyScriptPath string

	// GRPCPort is the gRPC port for Agent connections.
	// GRPCPort 是 Agent 连接的 gRPC 端口。
	GRPCPort string

	// HeartbeatInterval is the heartbeat interval in seconds.
	// HeartbeatInterval 是心跳间隔（秒）。
	HeartbeatInterval int

	// TLSEnabled indicates whether Control Plane gRPC TLS is enabled.
	// TLSEnabled 表示 Control Plane gRPC TLS 是否已启用。
	TLSEnabled bool

	// CAFile is the local path to the CA certificate Agents should trust.
	// CAFile 是 Agent 应信任的 CA 证书本地路径。
	CAFile string
}

// NewHandler creates a new Handler instance.
// NewHandler 创建一个新的 Handler 实例。
func NewHandler(cfg *HandlerConfig) *Handler {
	if cfg == nil {
		cfg = &HandlerConfig{}
	}

	// Set defaults
	// 设置默认值
	if cfg.ControlPlaneAddr == "" {
		cfg.ControlPlaneAddr = config.Config.App.Addr
	}
	if cfg.AgentBinaryDir == "" {
		cfg.AgentBinaryDir = "./lib/agent"
	}
	if cfg.STXJavaProxyJarPath == "" {
		cfg.STXJavaProxyJarPath = filepath.Join("./lib", seatunnelmeta.STXJavaProxyJarFileName(seatunnelmeta.DefaultSTXJavaProxyVersion))
	}
	if cfg.STXJavaProxyScriptPath == "" {
		cfg.STXJavaProxyScriptPath = filepath.Join("./scripts", seatunnelmeta.STXJavaProxyScriptFileName)
	}
	if cfg.GRPCPort == "" {
		cfg.GRPCPort = "50051"
	}
	if cfg.HeartbeatInterval <= 0 {
		cfg.HeartbeatInterval = 10 // Default 10 seconds
	}

	return &Handler{
		controlPlaneAddr:       cfg.ControlPlaneAddr,
		agentBinaryDir:         cfg.AgentBinaryDir,
		stxJavaProxyJarPath:    cfg.STXJavaProxyJarPath,
		stxJavaProxyScriptPath: cfg.STXJavaProxyScriptPath,
		grpcPort:               cfg.GRPCPort,
		heartbeatInterval:      cfg.HeartbeatInterval,
		tlsEnabled:             cfg.TLSEnabled,
		caFile:                 cfg.CAFile,
	}
}

// ==================== Response Types 响应类型 ====================

// ErrorResponse represents an error response.
// ErrorResponse 表示错误响应。
type ErrorResponse struct {
	ErrorMsg string `json:"error_msg"`
}

// ==================== Install Script Handler 安装脚本处理器 ====================

// GetInstallScript handles GET /api/v1/agent/install.sh - returns the Agent install script.
// GetInstallScript 处理 GET /api/v1/agent/install.sh - 返回 Agent 安装脚本。
// Requirements: 2.1 - Returns shell script with auto-detection logic for OS and architecture.
// @Tags agent
// @Produce text/x-shellscript
// @Success 200 {string} string "Install script"
// @Router /api/v1/agent/install.sh [get]
func (h *Handler) GetInstallScript(c *gin.Context) {
	// Use InstallScriptGenerator to generate the install script
	// 使用 InstallScriptGenerator 生成安装脚本
	generator, err := NewInstallScriptGenerator(&InstallScriptConfig{
		ControlPlaneAddr:  h.getControlPlaneURL(),
		GRPCAddr:          h.getGRPCAddr(),
		HeartbeatInterval: h.heartbeatInterval,
		TLSEnabled:        h.tlsEnabled,
	})
	if err != nil {
		logger.ErrorF(c.Request.Context(), "[Agent] Failed to create install script generator: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{ErrorMsg: "Failed to generate install script / 生成安装脚本失败"})
		return
	}

	// Generate the install script
	// 生成安装脚本
	script, err := generator.Generate()
	if err != nil {
		logger.ErrorF(c.Request.Context(), "[Agent] Failed to generate install script: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{ErrorMsg: "Failed to generate install script / 生成安装脚本失败"})
		return
	}

	// Set content type for shell script
	// 设置 shell 脚本的内容类型
	c.Header("Content-Type", "text/x-shellscript; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename=install.sh")

	// Write the script to response
	// 将脚本写入响应
	c.String(http.StatusOK, script)
}

// ==================== Download Handler 下载处理器 ====================

// supportedArchitectures defines the supported OS and architecture combinations.
// supportedArchitectures 定义支持的操作系统和架构组合。
var supportedArchitectures = map[string]map[string]string{
	"linux": {
		"amd64": "stx-agent-linux-amd64",
		"arm64": "stx-agent-linux-arm64",
	},
	"darwin": {
		"amd64": "stx-agent-darwin-amd64",
		"arm64": "stx-agent-darwin-arm64",
	},
}

// DownloadAgent handles GET /api/v1/agent/download - downloads the Agent binary.
// DownloadAgent 处理 GET /api/v1/agent/download - 下载 Agent 二进制文件。
// Requirements: 2.2 - Downloads Agent binary for the specified OS and architecture.
// @Tags agent
// @Param os query string true "Operating system (linux, darwin)"
// @Param arch query string true "CPU architecture (amd64, arm64)"
// @Produce application/octet-stream
// @Success 200 {file} binary "Agent binary file"
// @Failure 400 {object} ErrorResponse "Invalid parameters"
// @Failure 404 {object} ErrorResponse "Binary not found"
// @Router /api/v1/agent/download [get]
func (h *Handler) DownloadAgent(c *gin.Context) {
	// Get query parameters
	// 获取查询参数
	osType := strings.ToLower(c.Query("os"))
	arch := strings.ToLower(c.Query("arch"))

	// Validate parameters
	// 验证参数
	if osType == "" || arch == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			ErrorMsg: "Missing required parameters: os and arch / 缺少必需参数: os 和 arch",
		})
		return
	}

	// Check if OS is supported
	// 检查操作系统是否支持
	archMap, osSupported := supportedArchitectures[osType]
	if !osSupported {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			ErrorMsg: fmt.Sprintf("Unsupported operating system: %s. Supported: linux, darwin / 不支持的操作系统: %s. 支持: linux, darwin", osType, osType),
		})
		return
	}

	// Check if architecture is supported
	// 检查架构是否支持
	binaryName, archSupported := archMap[arch]
	if !archSupported {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			ErrorMsg: fmt.Sprintf("Unsupported architecture: %s. Supported: amd64, arm64 / 不支持的架构: %s. 支持: amd64, arm64", arch, arch),
		})
		return
	}

	// Build binary path
	// 构建二进制文件路径
	binaryPath := filepath.Join(h.agentBinaryDir, binaryName)

	// Check if binary exists
	// 检查二进制文件是否存在
	if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
		logger.WarnF(c.Request.Context(), "[Agent] Binary not found: %s", binaryPath)
		c.JSON(http.StatusNotFound, ErrorResponse{
			ErrorMsg: fmt.Sprintf("Agent binary not found for %s-%s. Please contact administrator / 未找到 %s-%s 的 Agent 二进制文件，请联系管理员", osType, arch, osType, arch),
		})
		return
	}

	// Set headers for binary download
	// 设置二进制下载的头信息
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", binaryName))

	// Serve the file
	// 提供文件
	c.File(binaryPath)

	logger.InfoF(c.Request.Context(), "[Agent] Binary downloaded: %s-%s", osType, arch)
}

// DownloadCA handles GET /api/v1/agent/ca.crt - downloads the gRPC TLS CA for Agent install.
// DownloadCA 处理 GET /api/v1/agent/ca.crt - 下载供 Agent 安装流使用的 gRPC TLS CA。
// @Tags agent
// @Produce application/x-pem-file
// @Success 200 {file} binary "CA certificate"
// @Failure 404 {object} ErrorResponse "TLS disabled or CA not found"
// @Router /api/v1/agent/ca.crt [get]
func (h *Handler) DownloadCA(c *gin.Context) {
	// TLS off → no CA to distribute.
	// TLS 未开启 → 没有可下发的 CA。
	if !h.tlsEnabled {
		c.JSON(http.StatusNotFound, ErrorResponse{
			ErrorMsg: "gRPC TLS is not enabled on Control Plane / Control Plane 未启用 gRPC TLS",
		})
		return
	}
	if h.caFile == "" {
		c.JSON(http.StatusNotFound, ErrorResponse{
			ErrorMsg: "CA certificate path is not configured / 未配置 CA 证书路径",
		})
		return
	}
	if _, err := os.Stat(h.caFile); os.IsNotExist(err) {
		logger.WarnF(c.Request.Context(), "[Agent] CA certificate not found: %s", h.caFile)
		c.JSON(http.StatusNotFound, ErrorResponse{
			ErrorMsg: "CA certificate not found. Please contact administrator / 未找到 CA 证书，请联系管理员",
		})
		return
	}

	c.Header("Content-Type", "application/x-pem-file")
	c.Header("Content-Disposition", "attachment; filename=ca.crt")
	c.File(h.caFile)
	logger.InfoF(c.Request.Context(), "[Agent] CA certificate downloaded: %s", h.caFile)
}

// DownloadSTXJavaProxyJar handles GET /api/v1/agent/assets/stx-java-proxy.jar - downloads the stx-java-proxy thin jar.
// DownloadSTXJavaProxyJar 处理 GET /api/v1/agent/assets/stx-java-proxy.jar - 下载 stx-java-proxy 薄 jar。
func (h *Handler) DownloadSTXJavaProxyJar(c *gin.Context) {
	assetPath, downloadName, err := h.resolveSTXJavaProxyJarAsset(c.Query("version"))
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{ErrorMsg: err.Error()})
		return
	}
	h.serveStaticAssetDownload(
		c,
		assetPath,
		downloadName,
		"application/java-archive",
		"Capability proxy jar",
		"Capability proxy jar",
	)
}

// DownloadSTXJavaProxyScript handles GET /api/v1/agent/assets/stx-java-proxy.sh - downloads the stx-java-proxy launcher script.
// DownloadSTXJavaProxyScript 处理 GET /api/v1/agent/assets/stx-java-proxy.sh - 下载 stx-java-proxy 启动脚本。
func (h *Handler) DownloadSTXJavaProxyScript(c *gin.Context) {
	h.serveStaticAssetDownload(
		c,
		h.stxJavaProxyScriptPath,
		seatunnelmeta.STXJavaProxyScriptFileName,
		"text/x-shellscript; charset=utf-8",
		"Capability proxy script",
		"Capability proxy script",
	)
}

func (h *Handler) resolveSTXJavaProxyJarAsset(version string) (string, string, error) {
	requestedVersion := strings.TrimSpace(version)
	if requestedVersion == "" || requestedVersion == seatunnelmeta.DefaultSTXJavaProxyVersion {
		return h.stxJavaProxyJarPath, filepath.Base(h.stxJavaProxyJarPath), nil
	}

	if !stxJavaProxyVersionPattern.MatchString(requestedVersion) {
		return "", "", fmt.Errorf("invalid version parameter: only letters, numbers, dot, underscore, and hyphen are allowed")
	}

	baseDir := filepath.Dir(h.stxJavaProxyJarPath)
	versionedPath := filepath.Join(
		baseDir,
		seatunnelmeta.STXJavaProxyJarFileName(requestedVersion),
	)
	relativePath, err := filepath.Rel(baseDir, versionedPath)
	if err != nil || relativePath == ".." || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("invalid version parameter")
	}
	if _, err := os.Stat(versionedPath); err == nil {
		return versionedPath, filepath.Base(versionedPath), nil
	}

	return h.stxJavaProxyJarPath, filepath.Base(h.stxJavaProxyJarPath), nil
}

func (h *Handler) serveStaticAssetDownload(
	c *gin.Context,
	assetPath string,
	downloadName string,
	contentType string,
	logLabel string,
	errorLabel string,
) {
	if _, err := os.Stat(assetPath); os.IsNotExist(err) {
		logger.WarnF(c.Request.Context(), "[Agent] %s not found: %s", logLabel, assetPath)
		c.JSON(http.StatusNotFound, ErrorResponse{
			ErrorMsg: fmt.Sprintf("%s not found. Please contact administrator / 未找到 %s，请联系管理员", errorLabel, errorLabel),
		})
		return
	}

	c.Header("Content-Type", contentType)
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", downloadName))
	c.File(assetPath)

	logger.InfoF(c.Request.Context(), "[Agent] %s downloaded: %s", logLabel, assetPath)
}

// ==================== Helper Methods 辅助方法 ====================

// getControlPlaneURL returns the full URL of the Control Plane.
// getControlPlaneURL 返回 Control Plane 的完整 URL。
func (h *Handler) getControlPlaneURL() string {
	addr := h.controlPlaneAddr
	if addr == "" {
		addr = "localhost:8080"
	}

	// Add http:// prefix if not present
	// 如果没有 http:// 前缀则添加
	if !strings.HasPrefix(addr, "http://") && !strings.HasPrefix(addr, "https://") {
		addr = "http://" + addr
	}

	return addr
}

// getGRPCAddr returns the gRPC address for Agent connection.
// getGRPCAddr 返回 Agent 连接的 gRPC 地址。
func (h *Handler) getGRPCAddr() string {
	// Extract host from control plane address
	// 从 Control Plane 地址提取主机
	addr := h.controlPlaneAddr
	addr = strings.TrimPrefix(addr, "http://")
	addr = strings.TrimPrefix(addr, "https://")

	// Remove port if present
	// 如果存在端口则移除
	if idx := strings.Index(addr, ":"); idx != -1 {
		addr = addr[:idx]
	}

	// Add gRPC port
	// 添加 gRPC 端口
	return fmt.Sprintf("%s:%s", addr, h.grpcPort)
}

// ==================== Uninstall Script Handler 卸载脚本处理器 ====================

// uninstallScriptTemplate is the template for the Agent uninstall script.
// uninstallScriptTemplate 是 Agent 卸载脚本的模板。
const uninstallScriptTemplate = `#!/bin/bash
# STX Agent Uninstall Script
# STX Agent 卸载脚本
# Generated by STX Control Plane
# 由 STX Control Plane 生成

set -e

DEFAULT_INSTALL_DIR="$HOME/.stx/agent"
INSTALL_DIR=""
AGENT_BINARY="stx-agent"
SERVICE_NAME="stx-agent"
LAUNCHD_LABEL="org.apache.stx.${SERVICE_NAME}"

BIN_DIR=""
CONFIG_DIR=""
LOG_DIR=""
SUPPORT_LIB_DIR=""
SUPPORT_SCRIPT_DIR=""
START_WRAPPER=""
SYSTEMD_UNIT_PATH=""
LAUNCHD_PLIST_PATH=""
USE_SYSTEMD_USER=0

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }

expand_path() {
    local raw="$1"
    local tilde_slash='~/'
    local home_var='$HOME'
    local home_brace='${HOME}'
    raw="${raw%\"}"; raw="${raw#\"}"
    raw="${raw%\'}"; raw="${raw#\'}"
    if [ "${raw}" = "~" ]; then
        raw="${HOME}"
    elif [ "${raw#"${tilde_slash}"}" != "${raw}" ]; then
        raw="${HOME}/${raw#"${tilde_slash}"}"
    elif [ "${raw#"${home_var}"}" != "${raw}" ]; then
        raw="${HOME}${raw#"${home_var}"}"
    elif [ "${raw#"${home_brace}"}" != "${raw}" ]; then
        raw="${HOME}${raw#"${home_brace}"}"
    fi
    while [ "${#raw}" -gt 1 ] && [ "${raw%/}" != "${raw}" ]; do
        raw="${raw%/}"
    done
    printf '%s' "${raw}"
}

print_usage() {
    cat << EOF
Usage: bash uninstall.sh [OPTIONS]
用法: bash uninstall.sh [选项]

Options / 选项:
  --install-dir=DIR   Agent home directory (default: ${DEFAULT_INSTALL_DIR})
                      Agent 主目录（默认: ${DEFAULT_INSTALL_DIR}）
  --remove-logs       Also remove log files / 同时移除日志文件
  -h, --help          Show this help / 显示帮助
EOF
}

resolve_paths() {
    if [ -z "${INSTALL_DIR}" ]; then
        INSTALL_DIR="${DEFAULT_INSTALL_DIR}"
    fi
    INSTALL_DIR="$(expand_path "${INSTALL_DIR}")"
    BIN_DIR="${INSTALL_DIR}/bin"
    CONFIG_DIR="${INSTALL_DIR}/etc"
    LOG_DIR="${INSTALL_DIR}/logs"
    SUPPORT_LIB_DIR="${INSTALL_DIR}/lib"
    SUPPORT_SCRIPT_DIR="${INSTALL_DIR}/scripts"
    START_WRAPPER="${BIN_DIR}/${AGENT_BINARY}-start.sh"
    LAUNCHD_PLIST_PATH="${HOME}/Library/LaunchAgents/${LAUNCHD_LABEL}.plist"
    if [ "$(id -u)" -eq 0 ]; then
        USE_SYSTEMD_USER=0
        SYSTEMD_UNIT_PATH="/etc/systemd/system/${SERVICE_NAME}.service"
    else
        USE_SYSTEMD_USER=1
        SYSTEMD_UNIT_PATH="${HOME}/.config/systemd/user/${SERVICE_NAME}.service"
    fi
}

systemctl_do() {
    if [ "${USE_SYSTEMD_USER}" -eq 1 ]; then
        systemctl --user "$@"
    else
        systemctl "$@"
    fi
}

stop_agent() {
    log_info "Stopping Agent service..."
    log_info "正在停止 Agent 服务..."

    local os_type
    os_type=$(uname -s | tr '[:upper:]' '[:lower:]')
    case "${os_type}" in
        linux)
            if command -v systemctl >/dev/null 2>&1; then
                if systemctl_do is-active --quiet "${SERVICE_NAME}" 2>/dev/null; then
                    systemctl_do stop "${SERVICE_NAME}"
                    log_info "Agent service stopped / Agent 服务已停止"
                else
                    log_info "Agent service is not running / Agent 服务未运行"
                fi
            fi
            ;;
        darwin)
            if [ -f "${LAUNCHD_PLIST_PATH}" ] && command -v launchctl >/dev/null 2>&1; then
                launchctl bootout "gui/$(id -u)" "${LAUNCHD_PLIST_PATH}" 2>/dev/null || true
                launchctl unload "${LAUNCHD_PLIST_PATH}" 2>/dev/null || true
                log_info "launchd job stopped / launchd 任务已停止"
            else
                log_info "launchd plist not found / 未找到 launchd plist"
            fi
            ;;
    esac
}

remove_service() {
    log_info "Removing service definition..."
    log_info "正在移除服务定义..."

    local os_type
    os_type=$(uname -s | tr '[:upper:]' '[:lower:]')
    case "${os_type}" in
        linux)
            if [ -f "${SYSTEMD_UNIT_PATH}" ]; then
                systemctl_do disable "${SERVICE_NAME}" 2>/dev/null || true
                rm -f "${SYSTEMD_UNIT_PATH}"
                systemctl_do daemon-reload 2>/dev/null || true
                log_info "Systemd unit removed: ${SYSTEMD_UNIT_PATH}"
                log_info "Systemd unit 已移除: ${SYSTEMD_UNIT_PATH}"
            else
                log_info "Systemd unit not found / 未找到 systemd unit"
            fi
            # Legacy system path cleanup / 清理旧版系统路径
            if [ -f "/etc/systemd/system/${SERVICE_NAME}.service" ] && [ "${SYSTEMD_UNIT_PATH}" != "/etc/systemd/system/${SERVICE_NAME}.service" ]; then
                rm -f "/etc/systemd/system/${SERVICE_NAME}.service"
                systemctl daemon-reload 2>/dev/null || true
            fi
            ;;
        darwin)
            if [ -f "${LAUNCHD_PLIST_PATH}" ]; then
                rm -f "${LAUNCHD_PLIST_PATH}"
                log_info "launchd plist removed: ${LAUNCHD_PLIST_PATH}"
                log_info "launchd plist 已移除: ${LAUNCHD_PLIST_PATH}"
            else
                log_info "launchd plist not found / 未找到 launchd plist"
            fi
            ;;
    esac
}

remove_files() {
    local remove_logs_flag=$1

    log_info "Removing Agent files under ${INSTALL_DIR}..."
    log_info "正在移除 ${INSTALL_DIR} 下的 Agent 文件..."

    rm -f "${BIN_DIR}/${AGENT_BINARY}" 2>/dev/null || true
    rm -f "${START_WRAPPER}" 2>/dev/null || true
    rm -rf "${CONFIG_DIR}" 2>/dev/null || true
    rm -rf "${SUPPORT_LIB_DIR}" 2>/dev/null || true
    rm -rf "${SUPPORT_SCRIPT_DIR}" 2>/dev/null || true

    if [ "${remove_logs_flag}" = "yes" ]; then
        rm -rf "${LOG_DIR}" 2>/dev/null || true
        log_info "Log directory removed / 日志目录已移除"
    else
        log_info "Keeping log files at ${LOG_DIR} / 保留日志于 ${LOG_DIR}"
    fi

    # Remove empty home dirs / 清理空的主目录层级
    rmdir "${BIN_DIR}" 2>/dev/null || true
    rmdir "${INSTALL_DIR}" 2>/dev/null || true

    # Legacy path cleanup (previous install layout) / 清理旧版安装路径
    rm -f "/usr/local/bin/${AGENT_BINARY}" 2>/dev/null || true
    rm -f "/usr/local/bin/${AGENT_BINARY}-start.sh" 2>/dev/null || true
    if [ "${remove_logs_flag}" = "yes" ]; then
        rm -rf "/var/log/${SERVICE_NAME}" 2>/dev/null || true
        rm -rf "/etc/stx-agent" 2>/dev/null || true
        rm -rf "/usr/local/lib/stx-agent" 2>/dev/null || true
    fi
}

main() {
    local remove_logs_flag="no"

    while [ $# -gt 0 ]; do
        case "$1" in
            --install-dir=*)
                INSTALL_DIR="${1#*=}"
                ;;
            --install-dir)
                shift
                if [ $# -eq 0 ]; then
                    log_error "--install-dir requires a value / --install-dir 需要参数值"
                    exit 1
                fi
                INSTALL_DIR="$1"
                ;;
            --remove-logs)
                remove_logs_flag="yes"
                ;;
            -h|--help)
                print_usage
                exit 0
                ;;
            *)
                log_error "Unknown option: $1 / 未知选项: $1"
                print_usage
                exit 1
                ;;
        esac
        shift
    done

    resolve_paths

    log_info "=========================================="
    log_info "STX Agent Uninstall Script"
    log_info "STX Agent 卸载脚本"
    log_info "Agent home: ${INSTALL_DIR}"
    log_info "=========================================="

    stop_agent
    remove_service
    remove_files "${remove_logs_flag}"

    log_info "=========================================="
    log_info "Uninstallation completed successfully!"
    log_info "卸载成功完成！"
    log_info "=========================================="
}

main "$@"
`

// GetUninstallScript handles GET /api/v1/agent/uninstall.sh - returns the Agent uninstall script.
// GetUninstallScript 处理 GET /api/v1/agent/uninstall.sh - 返回 Agent 卸载脚本。
// @Tags agent
// @Produce text/x-shellscript
// @Success 200 {string} string "Uninstall script"
// @Router /api/v1/agent/uninstall.sh [get]
func (h *Handler) GetUninstallScript(c *gin.Context) {
	// Set content type for shell script
	// 设置 shell 脚本的内容类型
	c.Header("Content-Type", "text/x-shellscript; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename=uninstall.sh")

	// Write the uninstall script directly (no template variables needed)
	// 直接写入卸载脚本（不需要模板变量）
	c.String(http.StatusOK, uninstallScriptTemplate)
}
