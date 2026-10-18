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

package selfupgrade

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Handler exposes self-upgrade HTTP APIs. / Handler 暴露自升级 HTTP API。
type Handler struct {
	service *Service
}

// Response is the common envelope. / 统一响应信封。
type Response struct {
	ErrorMsg  string      `json:"error_msg"`
	ErrorCode string      `json:"error_code,omitempty"`
	Data      interface{} `json:"data"`
}

// NewHandler creates a handler. / 创建处理器。
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// GetStatus returns aggregate status. / 返回聚合状态。
func (h *Handler) GetStatus(c *gin.Context) {
	c.JSON(http.StatusOK, Response{Data: h.service.Status()})
}

// Check handles version check. / 处理版本检查。
func (h *Handler) Check(c *gin.Context) {
	var req CheckRequest
	_ = c.ShouldBindJSON(&req)
	result, err := h.service.Check(c.Request.Context(), req)
	if err != nil {
		code := ErrorCode(err)
		status := statusFor(err)
		c.JSON(status, Response{ErrorMsg: err.Error(), ErrorCode: code, Data: result})
		return
	}
	c.JSON(http.StatusOK, Response{Data: result})
}

// StartDownload starts online staging. / 启动联网预置。
func (h *Handler) StartDownload(c *gin.Context) {
	var req StageDownloadRequest
	_ = c.ShouldBindJSON(&req)
	task, err := h.service.StartDownload(c.Request.Context(), req)
	if err != nil {
		c.JSON(statusFor(err), Response{ErrorMsg: err.Error(), ErrorCode: ErrorCode(err)})
		return
	}
	c.JSON(http.StatusAccepted, Response{Data: task})
}

// ImportOffline handles multipart offline import. / 处理离线导入。
func (h *Handler) ImportOffline(c *gin.Context) {
	sim := c.PostForm("simulate_failure")
	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: "file is required", ErrorCode: CodeBadOfflineImport})
		return
	}
	f, err := file.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{ErrorMsg: err.Error(), ErrorCode: CodeBadOfflineImport})
		return
	}
	defer f.Close()

	stage, err := h.service.ImportOffline(file.Filename, f, sim)
	if err != nil {
		c.JSON(statusFor(err), Response{ErrorMsg: err.Error(), ErrorCode: ErrorCode(err)})
		return
	}
	c.JSON(http.StatusOK, Response{Data: stage})
}

// GetStage returns staging snapshot. / 返回预置快照。
func (h *Handler) GetStage(c *gin.Context) {
	c.JSON(http.StatusOK, Response{Data: h.service.StageSnapshot()})
}

// CancelStage cancels staging. / 取消预置。
func (h *Handler) CancelStage(c *gin.Context) {
	if err := h.service.CancelStage(); err != nil {
		c.JSON(statusFor(err), Response{ErrorMsg: err.Error(), ErrorCode: ErrorCode(err)})
		return
	}
	c.JSON(http.StatusOK, Response{Data: gin.H{"ok": true}})
}

// Execute starts upgrade from local packages. / 从本地包开升。
func (h *Handler) Execute(c *gin.Context) {
	var req ExecuteRequest
	_ = c.ShouldBindJSON(&req)
	task, err := h.service.Execute(c.Request.Context(), req)
	if err != nil {
		c.JSON(statusFor(err), Response{ErrorMsg: err.Error(), ErrorCode: ErrorCode(err)})
		return
	}
	c.JSON(http.StatusAccepted, Response{Data: task})
}

// GetTask returns a task. / 返回任务。
func (h *Handler) GetTask(c *gin.Context) {
	task, err := h.service.GetTask(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, Response{ErrorMsg: err.Error(), ErrorCode: ErrorCode(err)})
		return
	}
	c.JSON(http.StatusOK, Response{Data: task})
}

// CancelTask cancels a task. / 取消任务。
func (h *Handler) CancelTask(c *gin.Context) {
	if err := h.service.CancelTask(c.Param("id")); err != nil {
		c.JSON(statusFor(err), Response{ErrorMsg: err.Error(), ErrorCode: ErrorCode(err)})
		return
	}
	c.JSON(http.StatusOK, Response{Data: gin.H{"ok": true}})
}

func statusFor(err error) int {
	switch {
	case errors.Is(err, ErrForbiddenNonAdmin):
		return http.StatusForbidden
	case errors.Is(err, ErrConcurrentUpgrade):
		return http.StatusConflict
	case errors.Is(err, ErrAlreadyLatest):
		return http.StatusOK
	case errors.Is(err, ErrCheckFail), errors.Is(err, ErrNetworkUnreachable):
		return http.StatusBadGateway
	case errors.Is(err, ErrDiskFull):
		return http.StatusInsufficientStorage
	case errors.Is(err, ErrBadOfflineImport), errors.Is(err, ErrNotStaged):
		return http.StatusBadRequest
	case errors.Is(err, ErrCancelled):
		return http.StatusConflict
	default:
		return http.StatusBadRequest
	}
}
