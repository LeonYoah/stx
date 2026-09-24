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

package sync

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ListCuratedTemplates handles POST /api/v1/sync/curated-templates/list.
// ListCuratedTemplates 处理精选模板列表。
func (h *Handler) ListCuratedTemplates(c *gin.Context) {
	var req ListCuratedTemplatesRequest
	if err := c.ShouldBindJSON(&req); err != nil && err.Error() != "EOF" {
		// Allow empty body.
		if c.Request.ContentLength != 0 {
			c.JSON(http.StatusBadRequest, CuratedTemplateListResponse{ErrorMsg: err.Error()})
			return
		}
	}
	data, err := h.service.ListCuratedTemplates(c.Request.Context(), &req, getCurrentUserID(c))
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), CuratedTemplateListResponse{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, CuratedTemplateListResponse{Data: data})
}

// CreateCuratedTemplate handles POST /api/v1/sync/curated-templates.
// CreateCuratedTemplate 处理创建精选模板。
func (h *Handler) CreateCuratedTemplate(c *gin.Context) {
	var req CreateCuratedTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, CuratedTemplateResponse{ErrorMsg: err.Error()})
		return
	}
	item, err := h.service.CreateCuratedTemplate(c.Request.Context(), &req, getCurrentUserID(c))
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), CuratedTemplateResponse{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, CuratedTemplateResponse{Data: item})
}

// UpdateCuratedTemplate handles PUT /api/v1/sync/curated-templates/:id.
// UpdateCuratedTemplate 处理更新精选模板。
func (h *Handler) UpdateCuratedTemplate(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		c.JSON(http.StatusBadRequest, CuratedTemplateResponse{ErrorMsg: "invalid id"})
		return
	}
	var req UpdateCuratedTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, CuratedTemplateResponse{ErrorMsg: err.Error()})
		return
	}
	item, err := h.service.UpdateCuratedTemplate(c.Request.Context(), id, &req, getCurrentUserID(c))
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), CuratedTemplateResponse{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, CuratedTemplateResponse{Data: item})
}

// DeleteCuratedTemplate handles DELETE /api/v1/sync/curated-templates/:id.
// DeleteCuratedTemplate 处理删除精选模板。
func (h *Handler) DeleteCuratedTemplate(c *gin.Context) {
	id, ok := parseUintParam(c, "id")
	if !ok {
		c.JSON(http.StatusBadRequest, CuratedTemplateResponse{ErrorMsg: "invalid id"})
		return
	}
	if err := h.service.DeleteCuratedTemplate(c.Request.Context(), id, getCurrentUserID(c)); err != nil {
		c.JSON(h.getStatusCodeForError(err), CuratedTemplateResponse{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, CuratedTemplateResponse{})
}

// ForkCuratedTemplate handles POST /api/v1/sync/curated-templates/fork.
// ForkCuratedTemplate 处理 fork 内置精选。
func (h *Handler) ForkCuratedTemplate(c *gin.Context) {
	var req ForkCuratedTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, CuratedTemplateResponse{ErrorMsg: err.Error()})
		return
	}
	item, err := h.service.ForkCuratedTemplate(c.Request.Context(), &req, getCurrentUserID(c))
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), CuratedTemplateResponse{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, CuratedTemplateResponse{Data: item})
}

// ParseCuratedCombo handles POST /api/v1/sync/curated-templates/parse-combo.
// ParseCuratedCombo 处理一键组合四节解析。
func (h *Handler) ParseCuratedCombo(c *gin.Context) {
	var req ParseCuratedComboRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, CuratedComboParseResponse{ErrorMsg: err.Error()})
		return
	}
	data, err := h.service.ParseCuratedCombo(c.Request.Context(), &req)
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), CuratedComboParseResponse{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, CuratedComboParseResponse{Data: data})
}

type curatedInsertRequest struct {
	BuiltinID string `json:"builtin_id"`
	ID        uint   `json:"id"`
	ClusterID uint   `json:"cluster_id"`
}

type curatedInsertResponse struct {
	ErrorMsg string `json:"error_msg"`
	Data     *struct {
		Content string `json:"content"`
	} `json:"data"`
}

// RenderCuratedTemplate handles POST /api/v1/sync/curated-templates/render.
// RenderCuratedTemplate 按集群版本渲染可插入正文（含低版本 plugin IO 改写）。
func (h *Handler) RenderCuratedTemplate(c *gin.Context) {
	var req curatedInsertRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, curatedInsertResponse{ErrorMsg: err.Error()})
		return
	}
	content, err := h.service.GetCuratedTemplateContentForInsert(
		c.Request.Context(),
		req.BuiltinID,
		getCurrentUserID(c),
		req.ID,
		req.ClusterID,
	)
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), curatedInsertResponse{ErrorMsg: err.Error()})
		return
	}
	c.JSON(http.StatusOK, curatedInsertResponse{Data: &struct {
		Content string `json:"content"`
	}{Content: content}})
}
