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

// Package releasebundle provides temporary download and install endpoints for STX bundles.
// releasebundle 包提供 STX 离线发布包的临时下载与安装端点。
package releasebundle

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/LeonYoah/stx/internal/apps/auth"
	"github.com/LeonYoah/stx/internal/config"
	"github.com/LeonYoah/stx/internal/db"
	"github.com/LeonYoah/stx/internal/logger"
	"github.com/gin-gonic/gin"
)

const (
	// DefaultBundlePattern selects the CentOS 7 compatible STX release bundle.
	// DefaultBundlePattern 选择适用于 CentOS 7 的 STX 发布包。
	DefaultBundlePattern = "stx-*-linux-amd64-node18-glibc217-without-observability.tar.gz"
	// DefaultReleaseDir is the default directory containing built release tarballs.
	// DefaultReleaseDir 是默认的发布包输出目录。
	DefaultReleaseDir = "./dist/releases"
)

// ErrorResponse represents a simple JSON error payload.
// ErrorResponse 表示简单的 JSON 错误响应。
type ErrorResponse struct {
	ErrorMsg string `json:"error_msg"`
}

// HandlerConfig configures the release bundle handler.
// HandlerConfig 配置发布包处理器。
type HandlerConfig struct {
	// ReleaseDir points to the directory containing built release tarballs.
	// ReleaseDir 指向发布包产物所在目录。
	ReleaseDir string
	// BundlePattern narrows the bundle selection to the desired package family.
	// BundlePattern 用于将可选发布包限制为目标包族。
	BundlePattern string
	// ValidateCredentials validates the provided username/password pair.
	// ValidateCredentials 校验提供的用户名和密码。
	ValidateCredentials func(ctx context.Context, username, password string) (bool, error)
}

// Handler exposes temporary download/install endpoints for STX bundles.
// Handler 暴露 STX 离线包的临时下载安装端点。
type Handler struct {
	releaseDir    string
	bundlePattern string
	validateAuth  func(ctx context.Context, username, password string) (bool, error)
}

// NewHandler creates a new release bundle handler.
// NewHandler 创建新的发布包处理器。
func NewHandler(cfg *HandlerConfig) *Handler {
	if cfg == nil {
		cfg = &HandlerConfig{}
	}
	if cfg.ReleaseDir == "" {
		cfg.ReleaseDir = DefaultReleaseDir
	}
	if cfg.BundlePattern == "" {
		cfg.BundlePattern = DefaultBundlePattern
	}
	return &Handler{
		releaseDir:    cfg.ReleaseDir,
		bundlePattern: cfg.BundlePattern,
		validateAuth:  coalesceCredentialValidator(cfg.ValidateCredentials),
	}
}

// GetInstallScript handles GET /api/v1/stx/install.sh and returns a one-click install script.
// GetInstallScript 处理 GET /api/v1/stx/install.sh 并返回一键安装脚本。
func (h *Handler) GetInstallScript(c *gin.Context) {
	_, bundleName, err := h.resolveLatestBundle()
	if err != nil {
		logger.ErrorF(c.Request.Context(), "[ReleaseBundle] Resolve latest bundle failed: %v", err)
		c.JSON(http.StatusNotFound, ErrorResponse{ErrorMsg: "release bundle not found"})
		return
	}

	baseURL := buildRequestBaseURL(c)
	scriptURL := baseURL + "/api/v1/stx/install.sh"
	downloadURL := baseURL + "/api/v1/stx/download"
	script, err := GenerateInstallScript(InstallScriptData{
		DownloadURL:    downloadURL,
		ExampleCommand: fmt.Sprintf("curl -fsSL %s | sudo bash", scriptURL),
	})
	if err != nil {
		logger.ErrorF(c.Request.Context(), "[ReleaseBundle] Generate install script failed: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{ErrorMsg: "failed to generate install script"})
		return
	}

	c.Header("Content-Type", "text/x-shellscript; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename=install.sh")
	c.Header("X-STX-Bundle", bundleName)
	c.String(http.StatusOK, script)
}

// DownloadBundle handles GET /api/v1/stx/download and serves the latest matching bundle.
// DownloadBundle 处理 GET /api/v1/stx/download 并返回最新匹配的发布包。
func (h *Handler) DownloadBundle(c *gin.Context) {
	if !h.requireDownloadAuth(c) {
		return
	}

	bundlePath, bundleName, err := h.resolveLatestBundle()
	if err != nil {
		logger.ErrorF(c.Request.Context(), "[ReleaseBundle] Resolve latest bundle failed: %v", err)
		c.JSON(http.StatusNotFound, ErrorResponse{ErrorMsg: "release bundle not found"})
		return
	}

	c.Header("Content-Type", "application/gzip")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", bundleName))
	c.File(bundlePath)
}

func (h *Handler) requireDownloadAuth(c *gin.Context) bool {
	username, password, ok := c.Request.BasicAuth()
	if !ok {
		h.respondUnauthorized(c)
		return false
	}

	valid, err := h.validateAuth(c.Request.Context(), strings.TrimSpace(username), password)
	if err != nil {
		logger.ErrorF(c.Request.Context(), "[ReleaseBundle] Validate credentials failed: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{ErrorMsg: "failed to validate credentials"})
		return false
	}
	if !valid {
		h.respondUnauthorized(c)
		return false
	}
	return true
}

func (h *Handler) respondUnauthorized(c *gin.Context) {
	c.Header("WWW-Authenticate", `Basic realm="STX Bundle Download"`)
	c.JSON(http.StatusUnauthorized, ErrorResponse{ErrorMsg: "authentication required"})
}

func coalesceCredentialValidator(validator func(ctx context.Context, username, password string) (bool, error)) func(ctx context.Context, username, password string) (bool, error) {
	if validator != nil {
		return validator
	}
	return defaultCredentialValidator
}

func defaultCredentialValidator(ctx context.Context, username, password string) (bool, error) {
	if username == "" || password == "" {
		return false, nil
	}
	user, err := auth.FindByUsername(db.GetDB(ctx), username)
	if err != nil {
		return false, nil
	}
	if !user.IsActive {
		return false, nil
	}
	return user.CheckPassword(password), nil
}

func (h *Handler) resolveLatestBundle() (string, string, error) {
	pattern := filepath.Join(h.releaseDir, h.bundlePattern)
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return "", "", fmt.Errorf("glob release bundles: %w", err)
	}
	if len(matches) == 0 {
		return "", "", fmt.Errorf("no bundle matches pattern %q in %q", h.bundlePattern, h.releaseDir)
	}

	type bundleCandidate struct {
		path    string
		name    string
		modTime int64
	}

	candidates := make([]bundleCandidate, 0, len(matches))
	for _, match := range matches {
		info, err := os.Stat(match)
		if err != nil || info.IsDir() {
			continue
		}
		candidates = append(candidates, bundleCandidate{
			path:    match,
			name:    filepath.Base(match),
			modTime: info.ModTime().UnixNano(),
		})
	}
	if len(candidates) == 0 {
		return "", "", fmt.Errorf("no readable bundle found in %q", h.releaseDir)
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].modTime == candidates[j].modTime {
			return candidates[i].name > candidates[j].name
		}
		return candidates[i].modTime > candidates[j].modTime
	})

	return candidates[0].path, candidates[0].name, nil
}

func buildRequestBaseURL(c *gin.Context) string {
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	if forwardedProto := firstForwardedValue(c.GetHeader("X-Forwarded-Proto")); forwardedProto != "" {
		scheme = forwardedProto
	}

	host := c.Request.Host
	if forwardedHost := firstForwardedValue(c.GetHeader("X-Forwarded-Host")); forwardedHost != "" {
		host = forwardedHost
	}

	if isLoopbackOrPrivateHost(host) {
		if external := config.GetExternalURL(); external != "" {
			if parsed, err := url.Parse(external); err == nil && parsed.Host != "" && !isLoopbackOrPrivateHost(parsed.Host) {
				return fmt.Sprintf("%s://%s", parsed.Scheme, parsed.Host)
			}
		}
	}

	if host == "" {
		host = "127.0.0.1:17800"
	}
	return fmt.Sprintf("%s://%s", scheme, host)
}

func firstForwardedValue(value string) string {
	if value == "" {
		return ""
	}
	return strings.TrimSpace(strings.Split(value, ",")[0])
}

func isLoopbackOrPrivateHost(hostport string) bool {
	if hostport == "" {
		return false
	}

	host := hostport
	if parsedHost, _, err := net.SplitHostPort(hostport); err == nil {
		host = parsedHost
	}
	host = strings.Trim(host, "[]")
	lower := strings.ToLower(host)
	if lower == "localhost" {
		return true
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate()
}
