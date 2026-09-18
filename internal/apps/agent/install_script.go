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
	"bytes"
	"fmt"
	"strings"
	"text/template"

	seatunnelmeta "github.com/LeonYoah/stx/internal/seatunnel"
)

// InstallScriptGenerator generates Agent installation scripts.
// InstallScriptGenerator 生成 Agent 安装脚本。
// Requirements: 2.1, 2.2, 2.3, 2.4, 2.5, 2.6 - Implements one-click Agent installation.
type InstallScriptGenerator struct {
	// controlPlaneAddr is the HTTP address of the Control Plane.
	// controlPlaneAddr 是 Control Plane 的 HTTP 地址。
	controlPlaneAddr string

	// grpcAddr is the gRPC address for Agent connection.
	// grpcAddr 是 Agent 连接的 gRPC 地址。
	grpcAddr string

	// heartbeatInterval is the heartbeat interval in seconds.
	// heartbeatInterval 是心跳间隔（秒）。
	heartbeatInterval int

	// tlsEnabled indicates whether Control Plane gRPC TLS is enabled.
	// tlsEnabled 表示 Control Plane gRPC TLS 是否已启用。
	tlsEnabled bool

	// template is the parsed install script template.
	// template 是解析后的安装脚本模板。
	template *template.Template
}

// InstallScriptConfig holds configuration for the install script generator.
// InstallScriptConfig 保存安装脚本生成器的配置。
type InstallScriptConfig struct {
	// ControlPlaneAddr is the HTTP address of the Control Plane.
	// ControlPlaneAddr 是 Control Plane 的 HTTP 地址。
	ControlPlaneAddr string

	// GRPCAddr is the gRPC address for Agent connection.
	// GRPCAddr 是 Agent 连接的 gRPC 地址。
	GRPCAddr string

	// HeartbeatInterval is the heartbeat interval in seconds from Control Plane config.
	// HeartbeatInterval 是来自 Control Plane 配置的心跳间隔（秒）。
	HeartbeatInterval int

	// TLSEnabled indicates whether Control Plane gRPC TLS is enabled (Agent should fetch CA).
	// TLSEnabled 表示 Control Plane gRPC TLS 是否已启用（Agent 应拉取 CA）。
	TLSEnabled bool
}

// InstallScriptData holds data for rendering the install script template.
// InstallScriptData 保存渲染安装脚本模板的数据。
type InstallScriptData struct {
	// ControlPlaneAddr is the HTTP address of the Control Plane.
	// ControlPlaneAddr 是 Control Plane 的 HTTP 地址。
	ControlPlaneAddr string

	// GRPCAddr is the gRPC address for Agent connection.
	// GRPCAddr 是 Agent 连接的 gRPC 地址。
	GRPCAddr string

	// InstallDir is the Agent home root (bin/etc/logs/lib live underneath).
	// InstallDir 是 Agent 主目录（其下包含 bin/etc/logs/lib）。
	InstallDir string

	// ConfigDir is kept for compatibility; the install script always derives etc/ under InstallDir.
	// ConfigDir 为兼容字段保留；安装脚本始终从 InstallDir 推导 etc/。
	ConfigDir string

	// AgentBinary is the name of the Agent binary file.
	// AgentBinary 是 Agent 二进制文件的名称。
	AgentBinary string

	// ServiceName is the service name (systemd unit / launchd label suffix).
	// ServiceName 是服务名（systemd unit / launchd label 后缀）。
	ServiceName string

	// SupportDir is kept for compatibility; the install script always uses InstallDir as support root.
	// SupportDir 为兼容字段保留；安装脚本始终以 InstallDir 作为辅助资产根目录。
	SupportDir string

	// STXJavaProxyVersion is the default packaged stx-java-proxy version.
	// STXJavaProxyVersion 是默认打包的 stx-java-proxy 版本。
	STXJavaProxyVersion string

	// STXJavaProxyJarFileName is the packaged default stx-java-proxy jar file name.
	// STXJavaProxyJarFileName 是默认打包的 stx-java-proxy jar 文件名。
	STXJavaProxyJarFileName string

	// STXJavaProxyScriptFileName is the packaged stx-java-proxy script file name.
	// STXJavaProxyScriptFileName 是打包的 stx-java-proxy 脚本文件名。
	STXJavaProxyScriptFileName string

	// HeartbeatInterval is the heartbeat interval string (e.g., "60s").
	// HeartbeatInterval 是心跳间隔字符串（如 "60s"）。
	HeartbeatInterval string

	// TLSEnabled indicates whether Agent should enable gRPC TLS and download CA.
	// TLSEnabled 表示 Agent 是否应启用 gRPC TLS 并下载 CA。
	TLSEnabled bool

	// AgentCAFile is the on-host path where the downloaded CA certificate is stored.
	// AgentCAFile 是本机保存已下载 CA 证书的路径。
	AgentCAFile string
}

// SupportedPlatform represents a supported OS and architecture combination.
// SupportedPlatform 表示支持的操作系统和架构组合。
type SupportedPlatform struct {
	// OS is the operating system (linux, darwin).
	// OS 是操作系统（linux, darwin）。
	OS string

	// Arch is the CPU architecture (amd64, arm64).
	// Arch 是 CPU 架构（amd64, arm64）。
	Arch string

	// BinaryName is the name of the binary file for this platform.
	// BinaryName 是此平台的二进制文件名称。
	BinaryName string
}

// DefaultAgentHomePath is the default Agent home shown in one-click install commands.
// Prefer $HOME form so zsh/bash do not mangle a leading tilde in --install-dir=...
// DefaultAgentHomePath 是一键安装命令中展示的默认 Agent 主目录。
// 优先使用 $HOME 形式，避免 zsh/bash 在 --install-dir=... 里误处理开头的 ~。
const DefaultAgentHomePath = "$HOME/.stx/agent"

// DefaultInstallDir is the default Agent home embedded in the install script ($HOME form).
// DefaultInstallDir 是安装脚本内嵌的默认 Agent 主目录（$HOME 形式）。
const DefaultInstallDir = "$HOME/.stx/agent"

// DefaultConfigDir is the default configuration directory derived from DefaultInstallDir.
// DefaultConfigDir 是由 DefaultInstallDir 推导的默认配置目录。
const DefaultConfigDir = DefaultInstallDir + "/etc"

// DefaultAgentBinary is the default name of the Agent binary.
// DefaultAgentBinary 是 Agent 二进制文件的默认名称。
const DefaultAgentBinary = "stx-agent"

// DefaultServiceName is the default service name (systemd unit / launchd label suffix).
// DefaultServiceName 是默认服务名（systemd unit / launchd label 后缀）。
const DefaultServiceName = "stx-agent"

// DefaultSupportDir is the default support-asset root (same as Agent home).
// DefaultSupportDir 是默认辅助资产根目录（与 Agent 主目录相同）。
const DefaultSupportDir = DefaultInstallDir

// DefaultAgentCAFile is the default on-host path for the Control Plane CA certificate.
// DefaultAgentCAFile 是 Control Plane CA 证书在本机的默认路径。
const DefaultAgentCAFile = DefaultConfigDir + "/certs/ca.crt"

// FormatInstallCommand builds the one-click Agent install command with an explicit --install-dir.
// FormatInstallCommand 生成带显式 --install-dir 的 Agent 一键安装命令。
func FormatInstallCommand(controlPlaneAddr string) string {
	addr := strings.TrimRight(strings.TrimSpace(controlPlaneAddr), "/")
	if addr == "" {
		addr = "http://localhost:8000"
	}
	if !strings.HasPrefix(addr, "http://") && !strings.HasPrefix(addr, "https://") {
		addr = "http://" + addr
	}
	return fmt.Sprintf(
		"curl -sSL %s/api/v1/agent/install.sh | bash -s -- --install-dir=%s/",
		addr,
		DefaultAgentHomePath,
	)
}

// SupportedPlatforms defines all supported OS and architecture combinations.
// SupportedPlatforms 定义所有支持的操作系统和架构组合。
// Requirements: 2.1, 2.2 - Supports linux-amd64 and linux-arm64.
var SupportedPlatforms = []SupportedPlatform{
	{OS: "linux", Arch: "amd64", BinaryName: "stx-agent-linux-amd64"},
	{OS: "linux", Arch: "arm64", BinaryName: "stx-agent-linux-arm64"},
	{OS: "darwin", Arch: "amd64", BinaryName: "stx-agent-darwin-amd64"},
	{OS: "darwin", Arch: "arm64", BinaryName: "stx-agent-darwin-arm64"},
}

// NewInstallScriptGenerator creates a new InstallScriptGenerator instance.
// NewInstallScriptGenerator 创建一个新的 InstallScriptGenerator 实例。
func NewInstallScriptGenerator(cfg *InstallScriptConfig) (*InstallScriptGenerator, error) {
	if cfg == nil {
		cfg = &InstallScriptConfig{}
	}

	// Set defaults
	// 设置默认值
	controlPlaneAddr := cfg.ControlPlaneAddr
	if controlPlaneAddr == "" {
		controlPlaneAddr = "localhost:8080"
	}

	grpcAddr := cfg.GRPCAddr
	if grpcAddr == "" {
		grpcAddr = "localhost:50051"
	}

	heartbeatInterval := cfg.HeartbeatInterval
	if heartbeatInterval <= 0 {
		heartbeatInterval = 10 // Default 10 seconds
	}

	// Parse template
	// 解析模板
	tmpl, err := template.New("install_script").Parse(installScriptTemplateContent)
	if err != nil {
		return nil, fmt.Errorf("failed to parse install script template: %w", err)
	}

	return &InstallScriptGenerator{
		controlPlaneAddr:  controlPlaneAddr,
		grpcAddr:          grpcAddr,
		heartbeatInterval: heartbeatInterval,
		tlsEnabled:        cfg.TLSEnabled,
		template:          tmpl,
	}, nil
}

// Generate generates the install script with the configured settings.
// Generate 使用配置的设置生成安装脚本。
// Requirements: 2.1 - Returns shell script with auto-detection logic for OS and architecture.
func (g *InstallScriptGenerator) Generate() (string, error) {
	data := &InstallScriptData{
		ControlPlaneAddr:           g.formatControlPlaneURL(),
		GRPCAddr:                   g.grpcAddr,
		InstallDir:                 DefaultInstallDir,
		ConfigDir:                  DefaultConfigDir,
		AgentBinary:                DefaultAgentBinary,
		ServiceName:                DefaultServiceName,
		SupportDir:                 DefaultSupportDir,
		STXJavaProxyVersion:        seatunnelmeta.DefaultSTXJavaProxyVersion,
		STXJavaProxyJarFileName:    seatunnelmeta.STXJavaProxyJarFileName(seatunnelmeta.DefaultSTXJavaProxyVersion),
		STXJavaProxyScriptFileName: seatunnelmeta.STXJavaProxyScriptFileName,
		HeartbeatInterval:          fmt.Sprintf("%ds", g.heartbeatInterval),
		TLSEnabled:                 g.tlsEnabled,
		AgentCAFile:                DefaultAgentCAFile,
	}

	return g.GenerateWithData(data)
}

// GenerateWithData generates the install script with custom data.
// GenerateWithData 使用自定义数据生成安装脚本。
func (g *InstallScriptGenerator) GenerateWithData(data *InstallScriptData) (string, error) {
	if data == nil {
		return "", fmt.Errorf("install script data cannot be nil")
	}

	// Set defaults if not provided
	// 如果未提供则设置默认值
	if data.ControlPlaneAddr == "" {
		data.ControlPlaneAddr = g.formatControlPlaneURL()
	}
	if data.GRPCAddr == "" {
		data.GRPCAddr = g.grpcAddr
	}
	if data.InstallDir == "" {
		data.InstallDir = DefaultInstallDir
	}
	if data.ConfigDir == "" {
		data.ConfigDir = DefaultConfigDir
	}
	if data.AgentBinary == "" {
		data.AgentBinary = DefaultAgentBinary
	}
	if data.ServiceName == "" {
		data.ServiceName = DefaultServiceName
	}
	if data.SupportDir == "" {
		data.SupportDir = DefaultSupportDir
	}
	if data.STXJavaProxyVersion == "" {
		data.STXJavaProxyVersion = seatunnelmeta.DefaultSTXJavaProxyVersion
	}
	if data.STXJavaProxyJarFileName == "" {
		data.STXJavaProxyJarFileName = seatunnelmeta.STXJavaProxyJarFileName(data.STXJavaProxyVersion)
	}
	if data.STXJavaProxyScriptFileName == "" {
		data.STXJavaProxyScriptFileName = seatunnelmeta.STXJavaProxyScriptFileName
	}
	if data.AgentCAFile == "" {
		data.AgentCAFile = DefaultAgentCAFile
	}

	var buf bytes.Buffer
	if err := g.template.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute install script template: %w", err)
	}

	return buf.String(), nil
}

// formatControlPlaneURL formats the Control Plane address as a full URL.
// formatControlPlaneURL 将 Control Plane 地址格式化为完整 URL。
func (g *InstallScriptGenerator) formatControlPlaneURL() string {
	addr := g.controlPlaneAddr
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

// GetSupportedPlatforms returns all supported platforms.
// GetSupportedPlatforms 返回所有支持的平台。
func GetSupportedPlatforms() []SupportedPlatform {
	return SupportedPlatforms
}

// IsPlatformSupported checks if a platform is supported.
// IsPlatformSupported 检查平台是否受支持。
func IsPlatformSupported(os, arch string) bool {
	os = strings.ToLower(os)
	arch = strings.ToLower(arch)

	for _, p := range SupportedPlatforms {
		if p.OS == os && p.Arch == arch {
			return true
		}
	}
	return false
}

// GetBinaryName returns the binary name for a platform.
// GetBinaryName 返回平台的二进制文件名称。
func GetBinaryName(os, arch string) (string, bool) {
	os = strings.ToLower(os)
	arch = strings.ToLower(arch)

	for _, p := range SupportedPlatforms {
		if p.OS == os && p.Arch == arch {
			return p.BinaryName, true
		}
	}
	return "", false
}

// NormalizeArch normalizes architecture names to standard format.
// NormalizeArch 将架构名称标准化为标准格式。
// Requirements: 2.1 - Supports architecture detection (x86_64 -> amd64, aarch64 -> arm64).
func NormalizeArch(arch string) string {
	arch = strings.ToLower(arch)
	switch arch {
	case "x86_64", "amd64":
		return "amd64"
	case "aarch64", "arm64":
		return "arm64"
	default:
		return arch
	}
}

// NormalizeOS normalizes OS names to standard format.
// NormalizeOS 将操作系统名称标准化为标准格式。
func NormalizeOS(os string) string {
	return strings.ToLower(os)
}

// installScriptTemplateContent is the template for the Agent install script.
// installScriptTemplateContent 是 Agent 安装脚本的模板。
// Requirements: 2.1, 2.2, 2.3, 2.4, 2.5, 2.6 - Implements one-click Agent installation.
const installScriptTemplateContent = `#!/bin/bash
# ============================================================================
# STX Agent Install Script
# STX Agent 安装脚本
# Generated by STX Control Plane
# 由 STX Control Plane 生成
# ============================================================================
# - Auto-detects OS/arch (linux, darwin / amd64, arm64)
# - Downloads Agent binary and support assets from Control Plane
# - Installs under a single home dir (default: $HOME/.stx/agent)
# - Non-root friendly: Linux uses systemd --user; Darwin uses launchd
# - Root on Linux uses system systemd
# ============================================================================

set -e

# ==================== Configuration 配置 ====================
CONTROL_PLANE_ADDR="{{.ControlPlaneAddr}}"
GRPC_ADDR="{{.GRPCAddr}}"
DEFAULT_INSTALL_DIR="{{.InstallDir}}"
AGENT_BINARY="{{.AgentBinary}}"
SERVICE_NAME="{{.ServiceName}}"
CAPABILITY_PROXY_VERSION="{{.STXJavaProxyVersion}}"
GRPC_TLS_ENABLED="{{if .TLSEnabled}}true{{else}}false{{end}}"
LAUNCHD_LABEL="org.apache.stx.${SERVICE_NAME}"

INSTALL_DIR=""
IS_ROOT=0
USE_SYSTEMD_USER=0
OS_TYPE=""

# Derived after resolve_paths / 路径解析后填充
BIN_DIR=""
CONFIG_DIR=""
LOG_DIR=""
SUPPORT_DIR=""
SUPPORT_LIB_DIR=""
SUPPORT_SCRIPT_DIR=""
CAPABILITY_PROXY_JAR=""
CAPABILITY_PROXY_SCRIPT=""
AGENT_CA_FILE=""
AGENT_CA_DIR=""
START_WRAPPER=""
SYSTEMD_UNIT_PATH=""
LAUNCHD_PLIST_PATH=""

# ==================== Colors 颜色 ====================
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

log_info() { echo -e "${GREEN}[INFO]${NC} $1"; }
log_warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
log_error() { echo -e "${RED}[ERROR]${NC} $1"; }
log_step() { echo -e "${BLUE}[STEP]${NC} $1"; }

print_usage() {
    cat << EOF
Usage: bash install.sh [OPTIONS]
用法: bash install.sh [选项]

Options / 选项:
  --install-dir=DIR   Agent home directory (default: ${DEFAULT_INSTALL_DIR})
                      Agent 主目录（默认: ${DEFAULT_INSTALL_DIR}）
                      Layout / 目录结构:
                        DIR/bin/${AGENT_BINARY}
                        DIR/etc/config.yaml
                        DIR/logs/agent.log
                        DIR/lib/  DIR/scripts/
  -h, --help          Show this help / 显示帮助

Examples / 示例:
  curl -sSL <cp>/api/v1/agent/install.sh | bash -s -- --install-dir=~/.stx/agent/
  bash install.sh --install-dir=/data/stx-agent
EOF
}

# Expand ~ / $HOME and normalize install-dir.
# 展开 ~ / $HOME 并规范化安装目录。
# Note: never use ${var#~/} — bash tilde-expands the # pattern into $HOME/,
# producing broken paths like /Users/x/~/.stx/agent.
# 注意：不要用 ${var#~/}——bash 会把 # 模式里的 ~/ 展开成 $HOME/，
# 从而得到 /Users/x/~/.stx/agent 这种错误路径。
expand_path() {
    local raw="$1"
    local tilde_slash='~/'
    local home_var='$HOME'
    local home_brace='${HOME}'
    # Strip wrapping quotes / 去掉包裹引号
    raw="${raw%\"}"
    raw="${raw#\"}"
    raw="${raw%\'}"
    raw="${raw#\'}"

    if [ "${raw}" = "~" ]; then
        raw="${HOME}"
    elif [ "${raw#"${tilde_slash}"}" != "${raw}" ]; then
        raw="${HOME}/${raw#"${tilde_slash}"}"
    elif [ "${raw#"${home_var}"}" != "${raw}" ]; then
        # Expand literal $HOME... from generated defaults / 展开脚本默认值中的字面 $HOME
        raw="${HOME}${raw#"${home_var}"}"
    elif [ "${raw#"${home_brace}"}" != "${raw}" ]; then
        raw="${HOME}${raw#"${home_brace}"}"
    fi

    # Collapse trailing slashes except root / 去掉末尾多余斜杠
    while [ "${#raw}" -gt 1 ] && [ "${raw%/}" != "${raw}" ]; do
        raw="${raw%/}"
    done
    printf '%s' "${raw}"
}

parse_args() {
    while [ $# -gt 0 ]; do
        case "$1" in
            --install-dir=*)
                INSTALL_DIR="${1#*=}"
                ;;
            --install-dir)
                shift
                if [ $# -eq 0 ]; then
                    log_error "--install-dir requires a value"
                    log_error "--install-dir 需要参数值"
                    exit 1
                fi
                INSTALL_DIR="$1"
                ;;
            -h|--help)
                print_usage
                exit 0
                ;;
            *)
                log_error "Unknown option: $1"
                log_error "未知选项: $1"
                print_usage
                exit 1
                ;;
        esac
        shift
    done
}

resolve_paths() {
    if [ -z "${INSTALL_DIR}" ]; then
        INSTALL_DIR="${DEFAULT_INSTALL_DIR}"
    fi
    INSTALL_DIR="$(expand_path "${INSTALL_DIR}")"
    if [ -z "${INSTALL_DIR}" ]; then
        log_error "Install directory is empty"
        log_error "安装目录为空"
        exit 1
    fi

    BIN_DIR="${INSTALL_DIR}/bin"
    CONFIG_DIR="${INSTALL_DIR}/etc"
    LOG_DIR="${INSTALL_DIR}/logs"
    SUPPORT_DIR="${INSTALL_DIR}"
    SUPPORT_LIB_DIR="${SUPPORT_DIR}/lib"
    SUPPORT_SCRIPT_DIR="${SUPPORT_DIR}/scripts"
    CAPABILITY_PROXY_JAR="${SUPPORT_LIB_DIR}/{{.STXJavaProxyJarFileName}}"
    CAPABILITY_PROXY_SCRIPT="${SUPPORT_SCRIPT_DIR}/{{.STXJavaProxyScriptFileName}}"
    AGENT_CA_FILE="${CONFIG_DIR}/certs/ca.crt"
    AGENT_CA_DIR="$(dirname "${AGENT_CA_FILE}")"
    START_WRAPPER="${BIN_DIR}/${AGENT_BINARY}-start.sh"

    if [ "$(id -u)" -eq 0 ]; then
        IS_ROOT=1
        USE_SYSTEMD_USER=0
        SYSTEMD_UNIT_PATH="/etc/systemd/system/${SERVICE_NAME}.service"
    else
        IS_ROOT=0
        USE_SYSTEMD_USER=1
        SYSTEMD_UNIT_PATH="${HOME}/.config/systemd/user/${SERVICE_NAME}.service"
    fi
    LAUNCHD_PLIST_PATH="${HOME}/Library/LaunchAgents/${LAUNCHD_LABEL}.plist"

    log_info "Agent home: ${INSTALL_DIR}"
    log_info "Agent 主目录: ${INSTALL_DIR}"
    if [ "${IS_ROOT}" -eq 1 ]; then
        log_info "Running as root (system service mode)"
        log_info "当前为 root（系统服务模式）"
    else
        log_info "Running as non-root (user service mode)"
        log_info "当前为非 root（用户服务模式）"
    fi
}

systemctl_do() {
    if [ "${USE_SYSTEMD_USER}" -eq 1 ]; then
        systemctl --user "$@"
    else
        systemctl "$@"
    fi
}

stop_existing_service() {
    case "${OS_TYPE}" in
        linux)
            if command -v systemctl >/dev/null 2>&1; then
                systemctl_do stop "${SERVICE_NAME}" 2>/dev/null || true
                systemctl_do disable "${SERVICE_NAME}" 2>/dev/null || true
            fi
            ;;
        darwin)
            if [ -f "${LAUNCHD_PLIST_PATH}" ] && command -v launchctl >/dev/null 2>&1; then
                launchctl bootout "gui/$(id -u)" "${LAUNCHD_PLIST_PATH}" 2>/dev/null || true
                launchctl unload "${LAUNCHD_PLIST_PATH}" 2>/dev/null || true
            fi
            ;;
    esac
}

cleanup() {
    local exit_code=$?
    if [ $exit_code -ne 0 ]; then
        log_error "Installation failed with exit code: ${exit_code}"
        log_error "安装失败，退出码: ${exit_code}"
        log_info "Cleaning up..."
        log_info "正在清理..."

        stop_existing_service

        rm -f "${BIN_DIR}/${AGENT_BINARY}" 2>/dev/null || true
        rm -f "${START_WRAPPER}" 2>/dev/null || true
        rm -rf "${CONFIG_DIR}" 2>/dev/null || true
        rm -rf "${LOG_DIR}" 2>/dev/null || true
        rm -rf "${SUPPORT_LIB_DIR}" 2>/dev/null || true
        rm -rf "${SUPPORT_SCRIPT_DIR}" 2>/dev/null || true
        rm -f "${SYSTEMD_UNIT_PATH}" 2>/dev/null || true
        rm -f "${LAUNCHD_PLIST_PATH}" 2>/dev/null || true
        rm -f "/tmp/${AGENT_BINARY}" 2>/dev/null || true
        rm -f "/tmp/${SERVICE_NAME}-ca.crt" 2>/dev/null || true
        rm -f "/tmp/${SERVICE_NAME}-{{.STXJavaProxyJarFileName}}" 2>/dev/null || true
        rm -f "/tmp/${SERVICE_NAME}-{{.STXJavaProxyScriptFileName}}" 2>/dev/null || true

        if command -v systemctl >/dev/null 2>&1; then
            systemctl_do daemon-reload 2>/dev/null || true
        fi

        log_info "Cleanup completed"
        log_info "清理完成"
    fi
    exit $exit_code
}

trap cleanup EXIT

detect_os() {
    local os_type
    os_type=$(uname -s | tr '[:upper:]' '[:lower:]')
    case "${os_type}" in
        linux) echo "linux" ;;
        darwin) echo "darwin" ;;
        *)
            log_error "Unsupported operating system: ${os_type}"
            log_error "不支持的操作系统: ${os_type}"
            log_info "Supported: linux, darwin / 支持: linux, darwin"
            exit 1
            ;;
    esac
}

detect_arch() {
    local arch
    arch=$(uname -m)
    case "${arch}" in
        x86_64|amd64) echo "amd64" ;;
        aarch64|arm64) echo "arm64" ;;
        *)
            log_error "Unsupported architecture: ${arch}"
            log_error "不支持的架构: ${arch}"
            log_info "Supported: amd64 (x86_64), arm64 (aarch64)"
            log_info "支持: amd64 (x86_64), arm64 (aarch64)"
            exit 1
            ;;
    esac
}

check_dependencies() {
    log_step "Checking dependencies..."
    log_step "正在检查依赖..."

    if ! command -v curl >/dev/null 2>&1 && ! command -v wget >/dev/null 2>&1; then
        log_error "Neither curl nor wget is available"
        log_error "curl 和 wget 都不可用"
        exit 1
    fi

    OS_TYPE="$(detect_os)"
    case "${OS_TYPE}" in
        linux)
            if ! command -v systemctl >/dev/null 2>&1; then
                log_warn "systemctl not found; will install files and print manual start command"
                log_warn "未找到 systemctl；将仅安装文件并打印手动启动命令"
            fi
            ;;
        darwin)
            if ! command -v launchctl >/dev/null 2>&1; then
                log_warn "launchctl not found; will install files and print manual start command"
                log_warn "未找到 launchctl；将仅安装文件并打印手动启动命令"
            fi
            ;;
    esac

    log_info "Dependencies check passed"
    log_info "依赖检查通过"
}

download_agent() {
    local os_type=$1
    local arch=$2
    local download_url="${CONTROL_PLANE_ADDR}/api/v1/agent/download?os=${os_type}&arch=${arch}"
    local temp_file="/tmp/${AGENT_BINARY}"

    log_step "Downloading Agent binary..."
    log_step "正在下载 Agent 二进制文件..."
    log_info "URL: ${download_url}"

    if command -v curl >/dev/null 2>&1; then
        if ! curl -fsSL -o "${temp_file}" "${download_url}"; then
            log_error "Failed to download Agent binary using curl"
            log_error "使用 curl 下载 Agent 二进制文件失败"
            exit 1
        fi
    else
        if ! wget -q -O "${temp_file}" "${download_url}"; then
            log_error "Failed to download Agent binary using wget"
            log_error "使用 wget 下载 Agent 二进制文件失败"
            exit 1
        fi
    fi

    if [ ! -s "${temp_file}" ]; then
        log_error "Downloaded file is missing or empty: ${temp_file}"
        log_error "下载的文件不存在或为空: ${temp_file}"
        exit 1
    fi

    local file_size
    file_size=$(stat -c%s "${temp_file}" 2>/dev/null || stat -f%z "${temp_file}" 2>/dev/null || echo "unknown")
    log_info "Downloaded ${file_size} bytes / 已下载 ${file_size} 字节"
}

download_support_assets() {
    local jar_url="${CONTROL_PLANE_ADDR}/api/v1/agent/assets/stx-java-proxy.jar?version=${CAPABILITY_PROXY_VERSION}"
    local script_url="${CONTROL_PLANE_ADDR}/api/v1/agent/assets/stx-java-proxy.sh"
    local temp_jar="/tmp/${SERVICE_NAME}-{{.STXJavaProxyJarFileName}}"
    local temp_script="/tmp/${SERVICE_NAME}-{{.STXJavaProxyScriptFileName}}"

    log_step "Downloading Agent support assets..."
    log_step "正在下载 Agent 辅助资产..."

    if command -v curl >/dev/null 2>&1; then
        if ! curl -fsSL -o "${temp_jar}" "${jar_url}"; then
            log_error "Failed to download stx-java-proxy jar using curl"
            log_error "使用 curl 下载 stx-java-proxy jar 失败"
            exit 1
        fi
        if ! curl -fsSL -o "${temp_script}" "${script_url}"; then
            log_error "Failed to download stx-java-proxy script using curl"
            log_error "使用 curl 下载 stx-java-proxy 脚本失败"
            exit 1
        fi
    else
        if ! wget -q -O "${temp_jar}" "${jar_url}"; then
            log_error "Failed to download stx-java-proxy jar using wget"
            log_error "使用 wget 下载 stx-java-proxy jar 失败"
            exit 1
        fi
        if ! wget -q -O "${temp_script}" "${script_url}"; then
            log_error "Failed to download stx-java-proxy script using wget"
            log_error "使用 wget 下载 stx-java-proxy 脚本失败"
            exit 1
        fi
    fi

    if [ ! -s "${temp_jar}" ] || [ ! -s "${temp_script}" ]; then
        log_error "Downloaded support assets are missing or empty"
        log_error "下载的辅助资产不存在或为空"
        exit 1
    fi

    log_info "Capability proxy assets downloaded"
    log_info "Capability proxy 资产下载完成"
}

download_ca() {
    if [ "${GRPC_TLS_ENABLED}" != "true" ]; then
        log_info "Control Plane gRPC TLS is disabled; skipping CA download"
        log_info "Control Plane gRPC TLS 未启用，跳过 CA 下载"
        return 0
    fi

    local ca_url="${CONTROL_PLANE_ADDR}/api/v1/agent/ca.crt"
    local temp_ca="/tmp/${SERVICE_NAME}-ca.crt"

    log_step "Downloading Control Plane CA certificate..."
    log_step "正在下载 Control Plane CA 证书..."
    log_info "URL: ${ca_url}"

    if command -v curl >/dev/null 2>&1; then
        if ! curl -fsSL -o "${temp_ca}" "${ca_url}"; then
            log_error "Failed to download CA certificate using curl"
            log_error "使用 curl 下载 CA 证书失败"
            exit 1
        fi
    else
        if ! wget -q -O "${temp_ca}" "${ca_url}"; then
            log_error "Failed to download CA certificate using wget"
            log_error "使用 wget 下载 CA 证书失败"
            exit 1
        fi
    fi

    if [ ! -s "${temp_ca}" ]; then
        log_error "Downloaded CA certificate is missing or empty"
        log_error "下载的 CA 证书不存在或为空"
        exit 1
    fi

    mkdir -p "${AGENT_CA_DIR}"
    mv "${temp_ca}" "${AGENT_CA_FILE}"
    chmod 0644 "${AGENT_CA_FILE}"

    log_info "CA certificate installed to ${AGENT_CA_FILE}"
    log_info "CA 证书已安装到 ${AGENT_CA_FILE}"
}

install_agent() {
    local temp_file="/tmp/${AGENT_BINARY}"

    log_step "Installing Agent binary..."
    log_step "正在安装 Agent 二进制文件..."

    mkdir -p "${BIN_DIR}" "${CONFIG_DIR}" "${LOG_DIR}"
    mv "${temp_file}" "${BIN_DIR}/${AGENT_BINARY}"
    chmod +x "${BIN_DIR}/${AGENT_BINARY}"

    log_info "Agent binary installed to ${BIN_DIR}/${AGENT_BINARY}"
    log_info "Agent 二进制文件已安装到 ${BIN_DIR}/${AGENT_BINARY}"

    log_step "Creating configuration..."
    log_step "正在创建配置..."

    if [ -n "${AGENT_ID:-}" ]; then
        :
    elif command -v sha256sum >/dev/null 2>&1; then
        AGENT_ID="agent-$( (cat /etc/machine-id 2>/dev/null; hostname 2>/dev/null; uname -n 2>/dev/null) | tr -d ' \n\r' | sha256sum | head -c 16)"
    else
        AGENT_ID="agent-$( (cat /etc/machine-id 2>/dev/null; hostname 2>/dev/null; date +%s 2>/dev/null) | tr -d ' \n\r' | md5sum 2>/dev/null | head -c 16 || echo "id$$")"
    fi
    log_info "Generated fixed agent ID: ${AGENT_ID}"
    log_info "已生成固定 Agent ID：${AGENT_ID}"

    local ca_file_value=""
    if [ "${GRPC_TLS_ENABLED}" = "true" ]; then
        ca_file_value="${AGENT_CA_FILE}"
    fi

    cat > "${CONFIG_DIR}/config.yaml" << EOF
# ============================================================================
# STX Agent Configuration
# STX Agent 配置文件
# Generated by install script
# 由安装脚本生成
# ============================================================================

agent:
  id: "${AGENT_ID}"

control_plane:
  addresses:
    - "${GRPC_ADDR}"
  tls:
    enabled: {{if .TLSEnabled}}true{{else}}false{{end}}
    cert_file: ""
    key_file: ""
    ca_file: "${ca_file_value}"
  token: ""

heartbeat:
  interval: {{.HeartbeatInterval}}

log:
  level: info
  file: ${LOG_DIR}/agent.log
  max_size: 100
  max_backups: 5
  max_age: 7

seatunnel:
  install_dir: /opt/seatunnel
EOF

    log_info "Configuration file created at ${CONFIG_DIR}/config.yaml"
    log_info "配置文件已创建于 ${CONFIG_DIR}/config.yaml"
}

install_support_assets() {
    local temp_jar="/tmp/${SERVICE_NAME}-{{.STXJavaProxyJarFileName}}"
    local temp_script="/tmp/${SERVICE_NAME}-{{.STXJavaProxyScriptFileName}}"

    log_step "Installing Agent support assets..."
    log_step "正在安装 Agent 辅助资产..."

    mkdir -p "${SUPPORT_LIB_DIR}" "${SUPPORT_SCRIPT_DIR}"
    mv "${temp_jar}" "${CAPABILITY_PROXY_JAR}"
    mv "${temp_script}" "${CAPABILITY_PROXY_SCRIPT}"
    chmod 0644 "${CAPABILITY_PROXY_JAR}"
    chmod +x "${CAPABILITY_PROXY_SCRIPT}"

    log_info "Capability proxy jar installed to ${CAPABILITY_PROXY_JAR}"
    log_info "Capability proxy jar 已安装到 ${CAPABILITY_PROXY_JAR}"
    log_info "Capability proxy script installed to ${CAPABILITY_PROXY_SCRIPT}"
    log_info "Capability proxy 脚本已安装到 ${CAPABILITY_PROXY_SCRIPT}"
}

create_start_wrapper() {
    cat > "${START_WRAPPER}" << EOF
#!/bin/bash
# STX Agent Startup Wrapper / STX Agent 启动包装脚本
if [ -f /etc/profile ]; then
    # shellcheck disable=SC1091
    source /etc/profile
fi
if [ -f "\${HOME}/.bashrc" ]; then
    # shellcheck disable=SC1091
    source "\${HOME}/.bashrc"
fi
if [ -f "\${HOME}/.bash_profile" ]; then
    # shellcheck disable=SC1091
    source "\${HOME}/.bash_profile"
fi
if [ -z "\$JAVA_HOME" ]; then
    for java_dir in /usr/lib/jvm/java-* /usr/java/* /opt/java/* /usr/local/java* /Library/Java/JavaVirtualMachines/*/Contents/Home; do
        if [ -d "\$java_dir" ] && [ -x "\$java_dir/bin/java" ]; then
            export JAVA_HOME="\$java_dir"
            export PATH="\$JAVA_HOME/bin:\$PATH"
            break
        fi
    done
fi
if [ -n "\$JAVA_HOME" ] && [ -d "\$JAVA_HOME/bin" ]; then
    export PATH="\$JAVA_HOME/bin:\$PATH"
fi
export STX_JAVA_PROXY_HOME="${SUPPORT_DIR}"
export STX_JAVA_PROXY_SCRIPT="${CAPABILITY_PROXY_SCRIPT}"
exec "${BIN_DIR}/${AGENT_BINARY}" "\$@"
EOF
    chmod +x "${START_WRAPPER}"
    log_info "Startup wrapper created at ${START_WRAPPER}"
    log_info "启动包装脚本已创建于 ${START_WRAPPER}"
}

create_systemd_service() {
    log_step "Creating systemd service..."
    log_step "正在创建 systemd 服务..."

    if ! command -v systemctl >/dev/null 2>&1; then
        log_warn "systemctl not available, skipping service creation"
        log_warn "systemctl 不可用，跳过服务创建"
        return 0
    fi

    mkdir -p "$(dirname "${SYSTEMD_UNIT_PATH}")"

    local wanted_by="multi-user.target"
    local user_lines=""
    if [ "${USE_SYSTEMD_USER}" -eq 1 ]; then
        wanted_by="default.target"
    else
        user_lines="User=root
Group=root"
    fi

    cat > "${SYSTEMD_UNIT_PATH}" << EOF
[Unit]
Description=STX Agent Service
Documentation=https://seatunnel.apache.org/
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
${user_lines}
ExecStart=/bin/bash ${START_WRAPPER} --config ${CONFIG_DIR}/config.yaml
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=${SERVICE_NAME}
KillMode=process
NoNewPrivileges=false
ProtectSystem=false
ProtectHome=false
LimitNOFILE=65536
LimitNPROC=65536
LimitCORE=infinity
Environment="PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

[Install]
WantedBy=${wanted_by}
EOF

    systemctl_do daemon-reload
    systemctl_do enable "${SERVICE_NAME}"

    if [ "${USE_SYSTEMD_USER}" -eq 1 ]; then
        log_info "User systemd service created: ${SYSTEMD_UNIT_PATH}"
        log_info "用户级 systemd 服务已创建: ${SYSTEMD_UNIT_PATH}"
        log_info "Tip: enable lingering for background run after logout: loginctl enable-linger \$USER"
        log_info "提示: 若需注销后仍运行，可执行: loginctl enable-linger \$USER"
    else
        log_info "System systemd service created: ${SYSTEMD_UNIT_PATH}"
        log_info "系统级 systemd 服务已创建: ${SYSTEMD_UNIT_PATH}"
    fi
}

create_launchd_service() {
    log_step "Creating launchd service..."
    log_step "正在创建 launchd 服务..."

    if ! command -v launchctl >/dev/null 2>&1; then
        log_warn "launchctl not available, skipping service creation"
        log_warn "launchctl 不可用，跳过服务创建"
        return 0
    fi

    mkdir -p "$(dirname "${LAUNCHD_PLIST_PATH}")"

    cat > "${LAUNCHD_PLIST_PATH}" << EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>${LAUNCHD_LABEL}</string>
    <key>ProgramArguments</key>
    <array>
        <string>/bin/bash</string>
        <string>${START_WRAPPER}</string>
        <string>--config</string>
        <string>${CONFIG_DIR}/config.yaml</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>${LOG_DIR}/launchd.out.log</string>
    <key>StandardErrorPath</key>
    <string>${LOG_DIR}/launchd.err.log</string>
    <key>EnvironmentVariables</key>
    <dict>
        <key>PATH</key>
        <string>/usr/local/bin:/usr/bin:/bin:/opt/homebrew/bin</string>
        <key>STX_JAVA_PROXY_HOME</key>
        <string>${SUPPORT_DIR}</string>
        <key>STX_JAVA_PROXY_SCRIPT</key>
        <string>${CAPABILITY_PROXY_SCRIPT}</string>
    </dict>
</dict>
</plist>
EOF

    # Prefer modern bootstrap; fall back to load -w / 优先 bootstrap，失败则 load -w
    launchctl bootout "gui/$(id -u)" "${LAUNCHD_PLIST_PATH}" 2>/dev/null || true
    if ! launchctl bootstrap "gui/$(id -u)" "${LAUNCHD_PLIST_PATH}" 2>/dev/null; then
        launchctl unload "${LAUNCHD_PLIST_PATH}" 2>/dev/null || true
        launchctl load -w "${LAUNCHD_PLIST_PATH}"
    fi

    log_info "launchd service created: ${LAUNCHD_PLIST_PATH}"
    log_info "launchd 服务已创建: ${LAUNCHD_PLIST_PATH}"
}

create_service() {
    create_start_wrapper
    case "${OS_TYPE}" in
        linux) create_systemd_service ;;
        darwin) create_launchd_service ;;
        *)
            log_warn "No service manager integration for OS: ${OS_TYPE}"
            log_warn "当前操作系统无服务管理集成: ${OS_TYPE}"
            ;;
    esac
}

start_agent() {
    log_step "Starting Agent service..."
    log_step "正在启动 Agent 服务..."

    case "${OS_TYPE}" in
        linux)
            if ! command -v systemctl >/dev/null 2>&1; then
                log_warn "systemctl not available, please start Agent manually:"
                log_warn "systemctl 不可用，请手动启动 Agent:"
                log_info "  /bin/bash ${START_WRAPPER} --config ${CONFIG_DIR}/config.yaml"
                return 0
            fi
            systemctl_do start "${SERVICE_NAME}"
            sleep 3
            if systemctl_do is-active --quiet "${SERVICE_NAME}"; then
                log_info "Agent service started successfully"
                log_info "Agent 服务启动成功"
            else
                log_error "Failed to start Agent service"
                log_error "启动 Agent 服务失败"
                if [ "${USE_SYSTEMD_USER}" -eq 1 ]; then
                    log_info "Check logs: journalctl --user -u ${SERVICE_NAME} -n 50"
                    log_info "查看日志: journalctl --user -u ${SERVICE_NAME} -n 50"
                else
                    log_info "Check logs: journalctl -u ${SERVICE_NAME} -n 50"
                    log_info "查看日志: journalctl -u ${SERVICE_NAME} -n 50"
                fi
                exit 1
            fi
            ;;
        darwin)
            if ! command -v launchctl >/dev/null 2>&1; then
                log_warn "launchctl not available, please start Agent manually:"
                log_warn "launchctl 不可用，请手动启动 Agent:"
                log_info "  /bin/bash ${START_WRAPPER} --config ${CONFIG_DIR}/config.yaml"
                return 0
            fi
            # kickstart if bootstrapped; otherwise load already started it
            # 若已 bootstrap 则 kickstart；否则 load 时通常已拉起
            launchctl kickstart -k "gui/$(id -u)/${LAUNCHD_LABEL}" 2>/dev/null || true
            sleep 2
            if launchctl print "gui/$(id -u)/${LAUNCHD_LABEL}" >/dev/null 2>&1 || \
               launchctl list 2>/dev/null | grep -q "${LAUNCHD_LABEL}"; then
                log_info "Agent service started successfully"
                log_info "Agent 服务启动成功"
            else
                log_warn "Could not confirm launchd job; check logs under ${LOG_DIR}"
                log_warn "未能确认 launchd 任务状态，请查看 ${LOG_DIR} 下日志"
            fi
            ;;
        *)
            log_info "  /bin/bash ${START_WRAPPER} --config ${CONFIG_DIR}/config.yaml"
            ;;
    esac

    log_info "Waiting for Agent to register with Control Plane..."
    log_info "正在等待 Agent 向 Control Plane 注册..."
    sleep 2
}

print_summary() {
    echo ""
    echo -e "${GREEN}============================================${NC}"
    echo -e "${GREEN}  Installation Completed Successfully!${NC}"
    echo -e "${GREEN}  安装成功完成！${NC}"
    echo -e "${GREEN}============================================${NC}"
    echo ""
    echo -e "Agent home / Agent 主目录: ${INSTALL_DIR}"
    echo ""
    echo -e "${BLUE}Installation Details / 安装详情:${NC}"
    echo -e "  Binary:  ${BIN_DIR}/${AGENT_BINARY}"
    echo -e "  Config:  ${CONFIG_DIR}/config.yaml"
    echo -e "  Logs:    ${LOG_DIR}/agent.log"
    echo -e "  Proxy:   ${CAPABILITY_PROXY_JAR}"
    echo -e "  Script:  ${CAPABILITY_PROXY_SCRIPT}"
    if [ "${GRPC_TLS_ENABLED}" = "true" ]; then
        echo -e "  CA:      ${AGENT_CA_FILE}"
        echo -e "  TLS:     enabled (one-way)"
    else
        echo -e "  TLS:     disabled"
    fi
    echo ""
    echo -e "${BLUE}Useful Commands / 常用命令:${NC}"
    case "${OS_TYPE}" in
        linux)
            if [ "${USE_SYSTEMD_USER}" -eq 1 ]; then
                echo -e "  systemctl --user status ${SERVICE_NAME}"
                echo -e "  journalctl --user -u ${SERVICE_NAME} -f"
                echo -e "  systemctl --user restart ${SERVICE_NAME}"
                echo -e "  systemctl --user stop ${SERVICE_NAME}"
            else
                echo -e "  systemctl status ${SERVICE_NAME}"
                echo -e "  journalctl -u ${SERVICE_NAME} -f"
                echo -e "  systemctl restart ${SERVICE_NAME}"
                echo -e "  systemctl stop ${SERVICE_NAME}"
            fi
            ;;
        darwin)
            echo -e "  launchctl print gui/\$(id -u)/${LAUNCHD_LABEL}"
            echo -e "  launchctl kickstart -k gui/\$(id -u)/${LAUNCHD_LABEL}"
            echo -e "  launchctl bootout gui/\$(id -u) ${LAUNCHD_PLIST_PATH}"
            echo -e "  tail -f ${LOG_DIR}/agent.log"
            ;;
    esac
    echo -e "  tail -f ${LOG_DIR}/agent.log"
    echo ""
    echo -e "  Uninstall / 卸载:"
    echo -e "    curl -sSL ${CONTROL_PLANE_ADDR}/api/v1/agent/uninstall.sh | bash -s -- --install-dir=${INSTALL_DIR} --remove-logs"
    echo ""
}

main() {
    echo ""
    echo -e "${BLUE}============================================${NC}"
    echo -e "${BLUE}  STX Agent Installation Script${NC}"
    echo -e "${BLUE}  STX Agent 安装脚本${NC}"
    echo -e "${BLUE}============================================${NC}"
    echo ""

    parse_args "$@"
    resolve_paths
    check_dependencies

    log_step "Detecting platform..."
    log_step "正在检测平台..."
    local arch
    OS_TYPE="$(detect_os)"
    arch="$(detect_arch)"
    log_info "Detected OS: ${OS_TYPE}, Architecture: ${arch}"
    log_info "检测到操作系统: ${OS_TYPE}, 架构: ${arch}"

    download_agent "${OS_TYPE}" "${arch}"
    download_support_assets
    download_ca
    install_agent
    install_support_assets
    create_service
    start_agent
    print_summary

    trap - EXIT
}

main "$@"
`
