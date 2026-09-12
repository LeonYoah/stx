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

# Configuration
# 配置
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/stx-agent"
LOG_DIR="/var/log/stx-agent"
AGENT_BINARY="stx-agent"
SERVICE_NAME="stx-agent"

# Colors for output
# 输出颜色
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Logging functions
# 日志函数
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if running as root
# 检查是否以 root 身份运行
check_root() {
    if [ "$(id -u)" -ne 0 ]; then
        log_error "This script must be run as root"
        log_error "此脚本必须以 root 身份运行"
        exit 1
    fi
}

# Stop Agent service
# 停止 Agent 服务
stop_agent() {
    log_info "Stopping Agent service..."
    log_info "正在停止 Agent 服务..."
    
    if systemctl is-active --quiet "${SERVICE_NAME}" 2>/dev/null; then
        systemctl stop "${SERVICE_NAME}"
        log_info "Agent service stopped"
        log_info "Agent 服务已停止"
    else
        log_info "Agent service is not running"
        log_info "Agent 服务未运行"
    fi
}

# Disable and remove systemd service
# 禁用并移除 systemd 服务
remove_systemd_service() {
    log_info "Removing systemd service..."
    log_info "正在移除 systemd 服务..."
    
    if [ -f "/etc/systemd/system/${SERVICE_NAME}.service" ]; then
        systemctl disable "${SERVICE_NAME}" 2>/dev/null || true
        rm -f "/etc/systemd/system/${SERVICE_NAME}.service"
        systemctl daemon-reload
        log_info "Systemd service removed"
        log_info "Systemd 服务已移除"
    else
        log_info "Systemd service file not found"
        log_info "未找到 Systemd 服务文件"
    fi
}

# Remove Agent binary
# 移除 Agent 二进制文件
remove_binary() {
    log_info "Removing Agent binary..."
    log_info "正在移除 Agent 二进制文件..."
    
    if [ -f "${INSTALL_DIR}/${AGENT_BINARY}" ]; then
        rm -f "${INSTALL_DIR}/${AGENT_BINARY}"
        log_info "Agent binary removed"
        log_info "Agent 二进制文件已移除"
    else
        log_info "Agent binary not found"
        log_info "未找到 Agent 二进制文件"
    fi
}

# Remove configuration files
# 移除配置文件
remove_config() {
    log_info "Removing configuration files..."
    log_info "正在移除配置文件..."
    
    if [ -d "${CONFIG_DIR}" ]; then
        rm -rf "${CONFIG_DIR}"
        log_info "Configuration directory removed"
        log_info "配置目录已移除"
    else
        log_info "Configuration directory not found"
        log_info "未找到配置目录"
    fi
}

# Remove log files (optional)
# 移除日志文件（可选）
remove_logs() {
    local remove_logs=$1
    
    if [ "${remove_logs}" = "yes" ]; then
        log_info "Removing log files..."
        log_info "正在移除日志文件..."
        
        if [ -d "${LOG_DIR}" ]; then
            rm -rf "${LOG_DIR}"
            log_info "Log directory removed"
            log_info "日志目录已移除"
        else
            log_info "Log directory not found"
            log_info "未找到日志目录"
        fi
    else
        log_info "Keeping log files at ${LOG_DIR}"
        log_info "保留日志文件于 ${LOG_DIR}"
    fi
}

# Main uninstallation process
# 主卸载流程
main() {
    local remove_logs_flag="no"
    
    # Parse arguments
    # 解析参数
    while [ $# -gt 0 ]; do
        case "$1" in
            --remove-logs)
                remove_logs_flag="yes"
                ;;
            -h|--help)
                echo "Usage: $0 [OPTIONS]"
                echo "用法: $0 [选项]"
                echo ""
                echo "Options / 选项:"
                echo "  --remove-logs    Also remove log files / 同时移除日志文件"
                echo "  -h, --help       Show this help message / 显示此帮助信息"
                exit 0
                ;;
            *)
                log_error "Unknown option: $1"
                log_error "未知选项: $1"
                exit 1
                ;;
        esac
        shift
    done
    
    log_info "=========================================="
    log_info "STX Agent Uninstall Script"
    log_info "STX Agent 卸载脚本"
    log_info "=========================================="
    
    # Check root
    # 检查 root
    check_root
    
    # Stop Agent service
    # 停止 Agent 服务
    stop_agent
    
    # Remove systemd service
    # 移除 systemd 服务
    remove_systemd_service
    
    # Remove binary
    # 移除二进制文件
    remove_binary
    
    # Remove configuration
    # 移除配置
    remove_config
    
    # Remove logs (optional)
    # 移除日志（可选）
    remove_logs "${remove_logs_flag}"
    
    log_info "=========================================="
    log_info "Uninstallation completed successfully!"
    log_info "卸载成功完成！"
    log_info "=========================================="
}

# Run main
# 运行主函数
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
