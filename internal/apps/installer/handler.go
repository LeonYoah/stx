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

// Package installer provides SeaTunnel installation management APIs for Control Plane.
// installer 包提供 Control Plane 的 SeaTunnel 安装管理 API。
package installer

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/LeonYoah/stx/internal/config"
	"github.com/LeonYoah/stx/internal/logger"
	"github.com/gin-gonic/gin"
)

// Handler provides HTTP handlers for installation management.
// Handler 提供安装管理的 HTTP 处理器。
type Handler struct {
	service *Service
}

// NewHandler creates a new Handler instance.
// NewHandler 创建一个新的 Handler 实例。
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// ==================== Package Management APIs 安装包管理 API ====================

// ListPackagesResponse represents the response for listing packages.
// ListPackagesResponse 表示获取安装包列表的响应。
type ListPackagesResponse struct {
	ErrorMsg string             `json:"error_msg"`
	Data     *AvailableVersions `json:"data"`
}

// ListPackages handles GET /api/v1/packages - lists available packages.
// ListPackages 处理 GET /api/v1/packages - 获取可用安装包列表。
// @Tags packages
// @Produce json
// @Success 200 {object} ListPackagesResponse
// @Router /api/v1/packages [get]
func (h *Handler) ListPackages(c *gin.Context) {
	versions, err := h.service.ListAvailableVersions(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, ListPackagesResponse{ErrorMsg: err.Error()})
		return
	}

	c.JSON(http.StatusOK, ListPackagesResponse{Data: versions})
}

// GetPackageInfoResponse represents the response for getting package info.
// GetPackageInfoResponse 表示获取安装包信息的响应。
type GetPackageInfoResponse struct {
	ErrorMsg string       `json:"error_msg"`
	Data     *PackageInfo `json:"data"`
}

// RefreshVersionsResponse represents the response for refreshing versions.
// RefreshVersionsResponse 表示刷新版本列表的响应。
type RefreshVersionsResponse struct {
	ErrorMsg string   `json:"error_msg"`
	Data     []string `json:"data"`
}

// RefreshVersions handles POST /api/v1/packages/versions/refresh - refreshes version list from Apache Archive.
// RefreshVersions 处理 POST /api/v1/packages/versions/refresh - 从 Apache Archive 刷新版本列表。
// @Tags packages
// @Produce json
// @Success 200 {object} RefreshVersionsResponse
// @Router /api/v1/packages/versions/refresh [post]
func (h *Handler) RefreshVersions(c *gin.Context) {
	versions, err := h.service.RefreshVersions(c.Request.Context())
	if err != nil {
		// Return fallback versions with warning / 返回备用版本并带警告
		logger.WarnF(c.Request.Context(), "[Installer] 刷新版本列表失败，使用备用列表: %v", err)
		c.JSON(http.StatusOK, RefreshVersionsResponse{
			ErrorMsg: "无法从 Apache Archive 获取版本列表，使用备用列表 / Failed to fetch from Apache Archive, using fallback list",
			Data:     versions,
		})
		return
	}

	logger.InfoF(c.Request.Context(), "[Installer] 刷新版本列表成功，共 %d 个版本", len(versions))
	c.JSON(http.StatusOK, RefreshVersionsResponse{Data: versions})
}

// GetPackageInfo handles GET /api/v1/packages/:version - gets package info.
// GetPackageInfo 处理 GET /api/v1/packages/:version - 获取安装包信息。
// @Tags packages
// @Produce json
// @Param version path string true "版本号"
// @Success 200 {object} GetPackageInfoResponse
// @Router /api/v1/packages/{version} [get]
func (h *Handler) GetPackageInfo(c *gin.Context) {
	version := c.Param("version")
	if version == "" {
		c.JSON(http.StatusBadRequest, GetPackageInfoResponse{ErrorMsg: "版本号不能为空 / Version is required"})
		return
	}

	info, err := h.service.GetPackageInfo(c.Request.Context(), version)
	if err != nil {
		c.JSON(http.StatusNotFound, GetPackageInfoResponse{ErrorMsg: err.Error()})
		return
	}

	c.JSON(http.StatusOK, GetPackageInfoResponse{Data: info})
}

// UploadPackageResponse represents the response for uploading a package.
// UploadPackageResponse 表示上传安装包的响应。
type UploadPackageResponse struct {
	ErrorMsg string       `json:"error_msg"`
	Data     *PackageInfo `json:"data"`
}

// UploadChunkResponse represents the response for uploading one package chunk.
// UploadChunkResponse 表示上传单个安装包分片的响应。
type UploadChunkResponse struct {
	ErrorMsg string                    `json:"error_msg"`
	Data     *PackageChunkUploadResult `json:"data"`
}

// UploadPackage handles POST /api/v1/packages/upload - uploads a package.
// UploadPackage 处理 POST /api/v1/packages/upload - 上传安装包。
// @Tags packages
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "安装包文件"
// @Param version formData string true "版本号"
// @Success 200 {object} UploadPackageResponse
// @Router /api/v1/packages/upload [post]
func (h *Handler) UploadPackage(c *gin.Context) {
	version := c.PostForm("version")
	if version == "" {
		c.JSON(http.StatusBadRequest, UploadPackageResponse{ErrorMsg: "版本号不能为空 / Version is required"})
		return
	}

	file, err := c.FormFile("file")
	if err != nil {
		statusCode := http.StatusBadRequest
		message := "文件上传失败 / File upload failed: " + err.Error()
		// Common when upstream proxy rejects/interrupts large upload body.
		// 常见于上游网关限制或中断大文件上传。
		if errors.Is(err, io.ErrUnexpectedEOF) ||
			strings.Contains(err.Error(), "unexpected EOF") ||
			strings.Contains(strings.ToLower(err.Error()), "request body too large") {
			statusCode = http.StatusRequestEntityTooLarge
			message = "上传内容被中断或超过网关限制（可能是 Cloudflare/Nginx 上传大小限制） / Upload interrupted or blocked by upstream size limit"
		}
		c.JSON(statusCode, UploadPackageResponse{ErrorMsg: message})
		return
	}

	maxPackageSize := config.GetMaxPackageSize()
	if maxPackageSize > 0 && file.Size > maxPackageSize {
		c.JSON(http.StatusRequestEntityTooLarge, UploadPackageResponse{
			ErrorMsg: fmt.Sprintf(
				"安装包超过配置上限（%.2f MB > %.2f MB） / Package exceeds configured max size",
				float64(file.Size)/1024.0/1024.0,
				float64(maxPackageSize)/1024.0/1024.0,
			),
		})
		return
	}

	info, err := h.service.UploadPackage(c.Request.Context(), version, file)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidPackageVersion),
			errors.Is(err, ErrInvalidPackageFile),
			errors.Is(err, ErrInvalidPackagePath):
			c.JSON(http.StatusBadRequest, UploadPackageResponse{ErrorMsg: err.Error()})
		case errors.Is(err, ErrPackageAlreadyExists):
			c.JSON(http.StatusConflict, UploadPackageResponse{ErrorMsg: err.Error()})
		case errors.Is(err, ErrPackageTooLarge):
			c.JSON(http.StatusRequestEntityTooLarge, UploadPackageResponse{ErrorMsg: err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, UploadPackageResponse{ErrorMsg: err.Error()})
		}
		return
	}

	logger.InfoF(c.Request.Context(), "[Installer] 上传安装包成功: %s", version)
	c.JSON(http.StatusOK, UploadPackageResponse{Data: info})
}

// UploadPackageChunk handles POST /api/v1/packages/upload/chunk - uploads one package chunk.
// UploadPackageChunk 处理 POST /api/v1/packages/upload/chunk - 上传安装包分片。
// @Tags packages
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "分片文件"
// @Param version formData string true "版本号"
// @Param upload_id formData string true "上传会话 ID"
// @Param chunk_index formData int true "分片索引（从 0 开始）"
// @Param total_chunks formData int true "总分片数"
// @Param total_size formData int true "原始文件总大小（字节）"
// @Param file_name formData string false "原始文件名"
// @Success 200 {object} UploadChunkResponse
// @Router /api/v1/packages/upload/chunk [post]
func (h *Handler) UploadPackageChunk(c *gin.Context) {
	version := strings.TrimSpace(c.PostForm("version"))
	uploadID := strings.TrimSpace(c.PostForm("upload_id"))
	fileName := strings.TrimSpace(c.PostForm("file_name"))
	chunkIndexStr := strings.TrimSpace(c.PostForm("chunk_index"))
	totalChunksStr := strings.TrimSpace(c.PostForm("total_chunks"))
	totalSizeStr := strings.TrimSpace(c.PostForm("total_size"))

	if version == "" {
		c.JSON(http.StatusBadRequest, UploadChunkResponse{ErrorMsg: "版本号不能为空 / Version is required"})
		return
	}
	if uploadID == "" {
		c.JSON(http.StatusBadRequest, UploadChunkResponse{ErrorMsg: "上传会话 ID 不能为空 / upload_id is required"})
		return
	}
	if chunkIndexStr == "" || totalChunksStr == "" || totalSizeStr == "" {
		c.JSON(http.StatusBadRequest, UploadChunkResponse{ErrorMsg: "缺少分片元数据 / missing chunk metadata"})
		return
	}

	chunkIndex, err := strconv.Atoi(chunkIndexStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, UploadChunkResponse{ErrorMsg: "chunk_index 必须为整数 / chunk_index must be an integer"})
		return
	}
	totalChunks, err := strconv.Atoi(totalChunksStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, UploadChunkResponse{ErrorMsg: "total_chunks 必须为整数 / total_chunks must be an integer"})
		return
	}
	totalSize, err := strconv.ParseInt(totalSizeStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, UploadChunkResponse{ErrorMsg: "total_size 必须为整数 / total_size must be an integer"})
		return
	}

	file, err := c.FormFile("file")
	if err != nil {
		statusCode := http.StatusBadRequest
		message := "分片上传失败 / Chunk upload failed: " + err.Error()
		if errors.Is(err, io.ErrUnexpectedEOF) ||
			strings.Contains(err.Error(), "unexpected EOF") ||
			strings.Contains(strings.ToLower(err.Error()), "request body too large") {
			statusCode = http.StatusRequestEntityTooLarge
			message = "上传内容被中断或超过网关限制（可能是 Cloudflare/Nginx 上传大小限制） / Upload interrupted or blocked by upstream size limit"
		}
		c.JSON(statusCode, UploadChunkResponse{ErrorMsg: message})
		return
	}

	result, err := h.service.UploadPackageChunk(c.Request.Context(), &PackageChunkUploadRequest{
		Version:     version,
		UploadID:    uploadID,
		ChunkIndex:  chunkIndex,
		TotalChunks: totalChunks,
		TotalSize:   totalSize,
		FileName:    fileName,
	}, file)
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidPackageVersion),
			errors.Is(err, ErrInvalidPackageFile),
			errors.Is(err, ErrInvalidPackagePath),
			errors.Is(err, ErrInvalidUploadID),
			errors.Is(err, ErrInvalidChunkIndex):
			c.JSON(http.StatusBadRequest, UploadChunkResponse{ErrorMsg: err.Error()})
		case errors.Is(err, ErrChunkOutOfOrder),
			errors.Is(err, ErrPackageAlreadyExists):
			c.JSON(http.StatusConflict, UploadChunkResponse{ErrorMsg: err.Error()})
		case errors.Is(err, ErrPackageTooLarge):
			c.JSON(http.StatusRequestEntityTooLarge, UploadChunkResponse{ErrorMsg: err.Error()})
		default:
			c.JSON(http.StatusInternalServerError, UploadChunkResponse{ErrorMsg: err.Error()})
		}
		return
	}

	c.JSON(http.StatusOK, UploadChunkResponse{Data: result})
}

// DeletePackageResponse represents the response for deleting a package.
// DeletePackageResponse 表示删除安装包的响应。
type DeletePackageResponse struct {
	ErrorMsg string `json:"error_msg"`
	Data     any    `json:"data"`
}

// DeletePackage handles DELETE /api/v1/packages/:version - deletes a local package.
// DeletePackage 处理 DELETE /api/v1/packages/:version - 删除本地安装包。
// @Tags packages
// @Produce json
// @Param version path string true "版本号"
// @Success 200 {object} DeletePackageResponse
// @Router /api/v1/packages/{version} [delete]
func (h *Handler) DeletePackage(c *gin.Context) {
	version := c.Param("version")
	if version == "" {
		c.JSON(http.StatusBadRequest, DeletePackageResponse{ErrorMsg: "版本号不能为空 / Version is required"})
		return
	}

	if err := h.service.DeletePackage(c.Request.Context(), version); err != nil {
		c.JSON(http.StatusInternalServerError, DeletePackageResponse{ErrorMsg: err.Error()})
		return
	}

	logger.InfoF(c.Request.Context(), "[Installer] 删除安装包成功: %s", version)
	c.JSON(http.StatusOK, DeletePackageResponse{})
}

// ==================== Download APIs 下载 API ====================

// DownloadResponse represents the response for download operations.
// DownloadResponse 表示下载操作的响应。
type DownloadResponse struct {
	ErrorMsg string        `json:"error_msg"`
	Data     *DownloadTask `json:"data"`
}

// DownloadListResponse represents the response for listing downloads.
// DownloadListResponse 表示获取下载列表的响应。
type DownloadListResponse struct {
	ErrorMsg string          `json:"error_msg"`
	Data     []*DownloadTask `json:"data"`
}

// StartDownload handles POST /api/v1/packages/download - starts downloading a package.
// StartDownload 处理 POST /api/v1/packages/download - 开始下载安装包。
// @Tags packages
// @Accept json
// @Produce json
// @Param request body DownloadRequest true "下载请求"
// @Success 200 {object} DownloadResponse
// @Router /api/v1/packages/download [post]
func (h *Handler) StartDownload(c *gin.Context) {
	var req DownloadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, DownloadResponse{ErrorMsg: err.Error()})
		return
	}

	task, err := h.service.StartDownload(c.Request.Context(), &req)
	if err != nil {
		// If download is already in progress, return the existing task / 如果下载已在进行中，返回现有任务
		if err == ErrDownloadInProgress {
			c.JSON(http.StatusOK, DownloadResponse{Data: task, ErrorMsg: "下载已在进行中 / Download already in progress"})
			return
		}
		if errors.Is(err, ErrInvalidPackageVersion) {
			c.JSON(http.StatusBadRequest, DownloadResponse{ErrorMsg: err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, DownloadResponse{ErrorMsg: err.Error()})
		return
	}

	logger.InfoF(c.Request.Context(), "[Installer] 开始下载安装包: %s from %s", req.Version, req.Mirror)
	c.JSON(http.StatusOK, DownloadResponse{Data: task})
}

// GetDownloadStatus handles GET /api/v1/packages/download/:version - gets download status.
// GetDownloadStatus 处理 GET /api/v1/packages/download/:version - 获取下载状态。
// @Tags packages
// @Produce json
// @Param version path string true "版本号"
// @Success 200 {object} DownloadResponse
// @Router /api/v1/packages/download/{version} [get]
func (h *Handler) GetDownloadStatus(c *gin.Context) {
	version := c.Param("version")
	if version == "" {
		c.JSON(http.StatusBadRequest, DownloadResponse{ErrorMsg: "版本号不能为空 / Version is required"})
		return
	}

	task, err := h.service.GetDownloadStatus(c.Request.Context(), version)
	if err != nil {
		c.JSON(http.StatusNotFound, DownloadResponse{ErrorMsg: err.Error()})
		return
	}

	c.JSON(http.StatusOK, DownloadResponse{Data: task})
}

// CancelDownload handles POST /api/v1/packages/download/:version/cancel - cancels a download.
// CancelDownload 处理 POST /api/v1/packages/download/:version/cancel - 取消下载。
// @Tags packages
// @Produce json
// @Param version path string true "版本号"
// @Success 200 {object} DownloadResponse
// @Router /api/v1/packages/download/{version}/cancel [post]
func (h *Handler) CancelDownload(c *gin.Context) {
	version := c.Param("version")
	if version == "" {
		c.JSON(http.StatusBadRequest, DownloadResponse{ErrorMsg: "版本号不能为空 / Version is required"})
		return
	}

	task, err := h.service.CancelDownload(c.Request.Context(), version)
	if err != nil {
		if errors.Is(err, ErrInvalidPackageVersion) {
			c.JSON(http.StatusBadRequest, DownloadResponse{ErrorMsg: err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, DownloadResponse{ErrorMsg: err.Error()})
		return
	}

	logger.InfoF(c.Request.Context(), "[Installer] 取消下载: %s", version)
	c.JSON(http.StatusOK, DownloadResponse{Data: task})
}

// ListDownloads handles GET /api/v1/packages/downloads - lists all download tasks.
// ListDownloads 处理 GET /api/v1/packages/downloads - 获取所有下载任务。
// @Tags packages
// @Produce json
// @Success 200 {object} DownloadListResponse
// @Router /api/v1/packages/downloads [get]
func (h *Handler) ListDownloads(c *gin.Context) {
	tasks := h.service.ListDownloads(c.Request.Context())
	c.JSON(http.StatusOK, DownloadListResponse{Data: tasks})
}

// ==================== Precheck APIs 预检查 API ====================

// PrecheckRequest represents the request for precheck.
// PrecheckRequest 表示预检查请求。
type PrecheckRequest struct {
	MinMemoryMB    int64  `json:"min_memory_mb"`
	MinCPUCores    int    `json:"min_cpu_cores"`
	MinDiskSpaceMB int64  `json:"min_disk_space_mb"`
	InstallDir     string `json:"install_dir"`
	Ports          []int  `json:"ports"`
}

// PrecheckResponse represents the response for precheck.
// PrecheckResponse 表示预检查响应。
type PrecheckResponse struct {
	ErrorMsg string          `json:"error_msg"`
	Data     *PrecheckResult `json:"data"`
}

// RuntimeStorageValidationResponse represents runtime storage validation response.
// RuntimeStorageValidationResponse 表示运行时存储校验响应。
type RuntimeStorageValidationResponse struct {
	ErrorMsg string                          `json:"error_msg"`
	Data     *RuntimeStorageValidationResult `json:"data"`
}

// RunPrecheck handles POST /api/v1/hosts/:id/precheck - runs precheck on a host.
// RunPrecheck 处理 POST /api/v1/hosts/:id/precheck - 在主机上运行预检查。
// @Tags installation
// @Accept json
// @Produce json
// @Param id path int true "主机ID"
// @Param request body PrecheckRequest false "预检查参数"
// @Success 200 {object} PrecheckResponse
// @Router /api/v1/hosts/{id}/precheck [post]
func (h *Handler) RunPrecheck(c *gin.Context) {
	hostID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, PrecheckResponse{ErrorMsg: "无效的主机 ID / Invalid host ID"})
		return
	}

	var req PrecheckRequest
	// Use default values if not provided / 如果未提供则使用默认值
	if err := c.ShouldBindJSON(&req); err != nil {
		// Ignore binding errors, use defaults / 忽略绑定错误，使用默认值
	}

	result, err := h.service.RunPrecheck(c.Request.Context(), uint(hostID), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, PrecheckResponse{ErrorMsg: err.Error()})
		return
	}

	c.JSON(http.StatusOK, PrecheckResponse{Data: result})
}

// ValidateRuntimeStorage handles POST /api/v1/installer/runtime-storage/validate.
// ValidateRuntimeStorage 处理 POST /api/v1/installer/runtime-storage/validate。
func (h *Handler) ValidateRuntimeStorage(c *gin.Context) {
	var req RuntimeStorageValidationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, RuntimeStorageValidationResponse{ErrorMsg: err.Error()})
		return
	}
	result, err := h.service.ValidateRuntimeStorage(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusBadRequest, RuntimeStorageValidationResponse{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, RuntimeStorageValidationResponse{Data: result})
}

// ==================== Installation APIs 安装 API ====================

// InstallResponse represents the response for installation.
// InstallResponse 表示安装响应。
type InstallResponse struct {
	ErrorMsg string              `json:"error_msg"`
	Data     *InstallationStatus `json:"data"`
}

// StartInstallation handles POST /api/v1/hosts/:id/install - starts installation.
// StartInstallation 处理 POST /api/v1/hosts/:id/install - 开始安装。
// @Tags installation
// @Accept json
// @Produce json
// @Param id path int true "主机ID"
// @Param request body InstallationRequest true "安装请求"
// @Success 200 {object} InstallResponse
// @Router /api/v1/hosts/{id}/install [post]
func (h *Handler) StartInstallation(c *gin.Context) {
	hostID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, InstallResponse{ErrorMsg: "无效的主机 ID / Invalid host ID"})
		return
	}

	var req InstallationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, InstallResponse{ErrorMsg: err.Error()})
		return
	}

	// Set host ID from path / 从路径设置主机 ID
	req.HostID = strconv.FormatUint(hostID, 10)

	status, err := h.service.StartInstallation(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, InstallResponse{ErrorMsg: err.Error()})
		return
	}

	logger.InfoF(c.Request.Context(), "[Installer] 开始安装: host=%d, version=%s", hostID, req.Version)
	c.JSON(http.StatusOK, InstallResponse{Data: status})
}

// GetInstallationStatus handles GET /api/v1/hosts/:id/install/status - gets installation status.
// GetInstallationStatus 处理 GET /api/v1/hosts/:id/install/status - 获取安装状态。
// @Tags installation
// @Produce json
// @Param id path int true "主机ID"
// @Success 200 {object} InstallResponse
// @Router /api/v1/hosts/{id}/install/status [get]
func (h *Handler) GetInstallationStatus(c *gin.Context) {
	hostID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, InstallResponse{ErrorMsg: "无效的主机 ID / Invalid host ID"})
		return
	}

	status, err := h.service.GetInstallationStatus(c.Request.Context(), uint(hostID))
	if err != nil {
		c.JSON(http.StatusNotFound, InstallResponse{ErrorMsg: err.Error()})
		return
	}

	c.JSON(http.StatusOK, InstallResponse{Data: status})
}

// RetryStepRequest represents the request for retrying a step.
// RetryStepRequest 表示重试步骤的请求。
type RetryStepRequest struct {
	Step string `json:"step" binding:"required"`
}

// RetryStep handles POST /api/v1/hosts/:id/install/retry - retries a failed step.
// RetryStep 处理 POST /api/v1/hosts/:id/install/retry - 重试失败的步骤。
// @Tags installation
// @Accept json
// @Produce json
// @Param id path int true "主机ID"
// @Param request body RetryStepRequest true "重试请求"
// @Success 200 {object} InstallResponse
// @Router /api/v1/hosts/{id}/install/retry [post]
func (h *Handler) RetryStep(c *gin.Context) {
	hostID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, InstallResponse{ErrorMsg: "无效的主机 ID / Invalid host ID"})
		return
	}

	var req RetryStepRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, InstallResponse{ErrorMsg: err.Error()})
		return
	}

	status, err := h.service.RetryStep(c.Request.Context(), uint(hostID), req.Step)
	if err != nil {
		c.JSON(http.StatusInternalServerError, InstallResponse{ErrorMsg: err.Error()})
		return
	}

	logger.InfoF(c.Request.Context(), "[Installer] 重试步骤: host=%d, step=%s", hostID, req.Step)
	c.JSON(http.StatusOK, InstallResponse{Data: status})
}

// CancelInstallation handles POST /api/v1/hosts/:id/install/cancel - cancels installation.
// CancelInstallation 处理 POST /api/v1/hosts/:id/install/cancel - 取消安装。
// @Tags installation
// @Produce json
// @Param id path int true "主机ID"
// @Success 200 {object} InstallResponse
// @Router /api/v1/hosts/{id}/install/cancel [post]
func (h *Handler) CancelInstallation(c *gin.Context) {
	hostID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, InstallResponse{ErrorMsg: "无效的主机 ID / Invalid host ID"})
		return
	}

	status, err := h.service.CancelInstallation(c.Request.Context(), uint(hostID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, InstallResponse{ErrorMsg: err.Error()})
		return
	}

	logger.InfoF(c.Request.Context(), "[Installer] 取消安装: host=%d", hostID)
	c.JSON(http.StatusOK, InstallResponse{Data: status})
}
