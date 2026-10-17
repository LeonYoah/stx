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

package admin

import (
	"net/http"
	"time"

	"github.com/LeonYoah/stx/internal/apps/auth"
	"github.com/LeonYoah/stx/internal/db"
	"github.com/gin-gonic/gin"
)

// ListCLITokensRequest 是管理员令牌列表请求。
// ListCLITokensRequest is the administrator token list request.
type ListCLITokensRequest struct {
	Current  int    `json:"current" form:"current" binding:"min=1"`
	Size     int    `json:"size" form:"size" binding:"min=1,max=100"`
	UserID   uint64 `json:"user_id" form:"user_id"`
	Username string `json:"username" form:"username"`
	Name     string `json:"name" form:"name"`
	Status   string `json:"status" form:"status"`
}

// ListCLITokensData 是令牌列表响应数据。
// ListCLITokensData is the token list response payload.
type ListCLITokensData struct {
	Total   int64               `json:"total"`
	Active  int64               `json:"active"`
	Expired int64               `json:"expired"`
	Revoked int64               `json:"revoked"`
	Tokens  []auth.CLITokenInfo `json:"tokens"`
}

// ListCLITokensResponse 是管理员令牌列表响应。
// ListCLITokensResponse is the administrator token list response.
type ListCLITokensResponse struct {
	ErrorMsg string             `json:"error_msg"`
	Data     *ListCLITokensData `json:"data"`
}

// CreateCLITokenRequest 是管理员签发令牌请求。
// CreateCLITokenRequest is the administrator token issue request.
type CreateCLITokenRequest struct {
	UserID    uint64 `json:"user_id" binding:"required"`
	Name      string `json:"name" binding:"required,max=100"`
	ExpiresIn string `json:"expires_in"`
}

// CreateCLITokenResult 是签发令牌后仅一次包含原文的结果。
// CreateCLITokenResult is the issue result and includes plaintext only once.
type CreateCLITokenResult struct {
	auth.CLITokenInfo
	Token string `json:"token,omitempty"`
}

// CreateCLITokenResponse 是管理员签发令牌响应。
// CreateCLITokenResponse is the administrator token issue response.
type CreateCLITokenResponse struct {
	ErrorMsg string                `json:"error_msg"`
	Data     *CreateCLITokenResult `json:"data"`
}

// RevokeCLITokenResponse 是管理员吊销令牌响应。
// RevokeCLITokenResponse is the administrator token revoke response.
type RevokeCLITokenResponse struct {
	ErrorMsg string             `json:"error_msg"`
	Data     *auth.CLITokenInfo `json:"data"`
}

// ListCLITokensHandler 列出全部 CLI 令牌元数据，不返回原文。
// ListCLITokensHandler lists CLI token metadata and never returns plaintext.
// @Tags admin
// @Param request query ListCLITokensRequest true "request query"
// @Produce json
// @Success 200 {object} ListCLITokensResponse
// @Router /api/v1/admin/cli-tokens [get]
func ListCLITokensHandler(c *gin.Context) {
	req := &ListCLITokensRequest{}
	if err := c.ShouldBindQuery(req); err != nil {
		c.JSON(http.StatusBadRequest, ListCLITokensResponse{ErrorMsg: err.Error()})
		return
	}

	tokens, total, counts, err := auth.ListCLITokens(db.DB(c.Request.Context()), auth.CLITokenFilter{
		UserID:   req.UserID,
		Username: req.Username,
		Name:     req.Name,
		Status:   req.Status,
		Page:     req.Current,
		PageSize: req.Size,
	}, time.Now())
	if err != nil {
		c.JSON(http.StatusInternalServerError, ListCLITokensResponse{ErrorMsg: err.Error()})
		return
	}
	if tokens == nil {
		tokens = []auth.CLITokenInfo{}
	}

	c.JSON(http.StatusOK, ListCLITokensResponse{
		Data: &ListCLITokensData{
			Total:   total,
			Active:  counts.Active,
			Expired: counts.Expired,
			Revoked: counts.Revoked,
			Tokens:  tokens,
		},
	})
}

// NewCreateCLITokenResult 根据签发结果和所属用户组装一次性响应。
// NewCreateCLITokenResult builds the one-time issue response for the owning user.
func NewCreateCLITokenResult(issue *auth.CLITokenIssue, user *auth.User, now time.Time) *CreateCLITokenResult {
	if issue == nil {
		return nil
	}
	username := ""
	userID := uint64(0)
	if user != nil {
		username = user.Username
		userID = user.ID
	}
	info := auth.CLITokenInfo{
		ID:          issue.ID,
		UserID:      userID,
		Username:    username,
		Name:        issue.Name,
		TokenPrefix: issue.Prefix,
		ExpiresAt:   issue.ExpiresAt,
		CreatedAt:   now,
		Permanent:   issue.Permanent,
		Status:      auth.CLITokenStatusActive,
	}
	return &CreateCLITokenResult{CLITokenInfo: info, Token: issue.Token}
}
