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

package userwrite

import (
	"fmt"
	"net/http"
	"net/mail"
	"strconv"
	"strings"

	"github.com/LeonYoah/stx/internal/apps/auth"
	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
	"github.com/LeonYoah/stx/internal/db"
	"github.com/LeonYoah/stx/internal/logger"
	"github.com/gin-gonic/gin"
)

// UpdateProfile 更新当前登录用户的个人资料，并为 CLI/API 调用记录公共执行。
// UpdateProfile updates the current user profile and records shared execution for CLI/API calls.
func (h *Handler) UpdateProfile(c *gin.Context) {
	userID := auth.GetUserIDFromContext(c)
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, auth.UpdateProfileResponse{ErrorMsg: "未登录"})
		return
	}
	var req auth.UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, auth.UpdateProfileResponse{ErrorMsg: err.Error()})
		return
	}
	email := strings.TrimSpace(req.Email)
	language := auth.NormalizeLanguage(req.Language)
	hasEmail := email != ""
	hasLanguage := strings.TrimSpace(req.Language) != ""
	if !hasEmail && !hasLanguage {
		c.JSON(http.StatusBadRequest, auth.UpdateProfileResponse{ErrorMsg: "至少提供一个可更新字段 / At least one field is required"})
		return
	}
	if hasEmail {
		if _, err := mail.ParseAddress(email); err != nil {
			c.JSON(http.StatusBadRequest, auth.UpdateProfileResponse{ErrorMsg: "邮箱格式不正确 / Invalid email format"})
			return
		}
	}
	if hasLanguage && language == "" {
		c.JSON(http.StatusBadRequest, auth.UpdateProfileResponse{ErrorMsg: "语言不合法 / Invalid language"})
		return
	}
	requestHash, err := executionapp.HashRequest(struct {
		Email    string `json:"email,omitempty"`
		Language string `json:"language,omitempty"`
	}{Email: email, Language: language})
	if err != nil {
		c.JSON(http.StatusInternalServerError, auth.UpdateProfileResponse{ErrorMsg: auth.ErrMsgInternalError})
		return
	}
	item, existing, beginErr := h.begin(c, "auth.profile.update", "auth", strconv.FormatUint(userID, 10), executionapp.RiskLevelR1,
		"修改当前用户的邮箱或语言偏好，可再次修改恢复。", requestHash)
	if beginErr != nil {
		executionapp.WriteError(c, beginErr)
		return
	}
	if existing {
		if item == nil || item.Status != executionapp.StatusSucceeded {
			status := executionapp.Status("")
			if item != nil {
				status = item.Status
			}
			executionapp.WriteError(c, fmt.Errorf("%w: previous status %s", executionapp.ErrConcurrentUpdate, status))
			return
		}
		user, loadErr := auth.FindByID(db.GetDB(c.Request.Context()), userID)
		if loadErr != nil {
			c.JSON(http.StatusNotFound, auth.UpdateProfileResponse{ErrorMsg: "用户不存在"})
			return
		}
		c.JSON(http.StatusOK, auth.UpdateProfileResponse{Data: user.ToUserInfo()})
		return
	}

	user, err := auth.FindByID(db.GetDB(c.Request.Context()), userID)
	if err != nil {
		h.finish(c, item, "", err)
		c.JSON(http.StatusNotFound, auth.UpdateProfileResponse{ErrorMsg: "用户不存在"})
		return
	}
	updates := make(map[string]interface{}, 2)
	if hasEmail {
		updates["email"] = email
	}
	if hasLanguage {
		updates["language"] = language
	}
	if err := db.GetDB(c.Request.Context()).Model(user).Updates(updates).Error; err != nil {
		h.finish(c, item, "", err)
		logger.ErrorF(c.Request.Context(), "[Auth] 更新个人信息失败: user_id=%d err=%v", userID, err)
		c.JSON(http.StatusInternalServerError, auth.UpdateProfileResponse{ErrorMsg: auth.ErrMsgInternalError})
		return
	}
	if hasEmail {
		user.Email = email
	}
	if hasLanguage {
		user.Language = language
	}
	h.finish(c, item, strconv.FormatUint(userID, 10), nil)
	logger.InfoF(c.Request.Context(), "[Auth] 更新个人信息成功: user_id=%d email_updated=%t language_updated=%t", userID, hasEmail, hasLanguage)
	c.JSON(http.StatusOK, auth.UpdateProfileResponse{Data: user.ToUserInfo()})
}
