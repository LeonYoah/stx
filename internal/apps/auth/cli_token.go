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

package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/LeonYoah/stx/internal/db"
	"github.com/LeonYoah/stx/internal/logger"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	// DefaultCLITokenTTL 是未指定有效期时使用的期限。
	// DefaultCLITokenTTL is used when the caller does not specify an expiry.
	DefaultCLITokenTTL = 7 * 24 * time.Hour

	// MaxCLITokenTTL 是 CLI 令牌允许的最长有效期。
	// MaxCLITokenTTL is the maximum lifetime accepted for a CLI token.
	MaxCLITokenTTL = 30 * 24 * time.Hour

	cliTokenPrefix = "stx_"
	cliTokenBytes  = 32
)

var (
	// ErrInvalidCLITokenTTL 表示令牌有效期不是 7 到 30 天内的整数天数。
	// ErrInvalidCLITokenTTL means the lifetime is not an integer number of days from 7 to 30.
	ErrInvalidCLITokenTTL = errors.New("auth: CLI 令牌有效期必须是 7d 到 30d")
	// ErrInvalidCLIToken 表示令牌不存在、已撤销或已过期。
	// ErrInvalidCLIToken means the token is unknown, revoked, or expired.
	ErrInvalidCLIToken = errors.New("auth: CLI 令牌无效")
	// ErrCLITokenRevoked 表示令牌已经撤销。
	// ErrCLITokenRevoked means the token has been revoked.
	ErrCLITokenRevoked = errors.New("auth: CLI 令牌已撤销")
	// ErrCLITokenExpired 表示令牌已经过期。
	// ErrCLITokenExpired means the token has expired.
	ErrCLITokenExpired = errors.New("auth: CLI 令牌已过期")
)

// CLIToken 保存 CLI 令牌的不可逆信息，不保存令牌原文。
// CLIToken stores irreversible token data and never stores the plaintext token.
type CLIToken struct {
	ID          uint64     `json:"id" gorm:"primaryKey;autoIncrement"`
	UserID      uint64     `json:"user_id" gorm:"index;not null"`
	TokenPrefix string     `json:"token_prefix" gorm:"size:16;index;not null"`
	TokenHash   string     `json:"-" gorm:"column:token_hash;size:64;unique;not null"`
	ExpiresAt   time.Time  `json:"expires_at" gorm:"index;not null"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty" gorm:"index"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt   time.Time  `json:"updated_at" gorm:"autoUpdateTime"`
}

// TableName 指定 CLI 令牌表名。
// TableName specifies the CLI token table name.
func (CLIToken) TableName() string {
	return "auth_cli_tokens"
}

// CLITokenIssue 是创建令牌后仅返回一次的结果。
// CLITokenIssue is returned once when a token is created.
type CLITokenIssue struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// ParseCLITokenTTL 校验并解析形如 7d、14d、30d 的有效期。
// ParseCLITokenTTL validates and parses lifetimes such as 7d, 14d, or 30d.
func ParseCLITokenTTL(value string) (time.Duration, error) {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return DefaultCLITokenTTL, nil
	}
	if !strings.HasSuffix(value, "d") {
		return 0, ErrInvalidCLITokenTTL
	}

	days, err := strconv.Atoi(strings.TrimSuffix(value, "d"))
	if err != nil || days < 7 || days > 30 {
		return 0, ErrInvalidCLITokenTTL
	}
	return time.Duration(days) * 24 * time.Hour, nil
}

// IssueCLIToken 校验用户并生成随机令牌，只把哈希和元数据写入数据库。
// IssueCLIToken validates the user and stores only a hash and metadata.
func IssueCLIToken(database *gorm.DB, user *User, expiresIn string, now time.Time) (*CLITokenIssue, error) {
	if database == nil {
		return nil, errors.New("auth: 数据库连接未初始化")
	}
	if user == nil || user.ID == 0 || !user.IsActive {
		return nil, ErrUserInactive
	}

	ttl, err := ParseCLITokenTTL(expiresIn)
	if err != nil {
		return nil, err
	}
	if now.IsZero() {
		now = time.Now()
	}

	secret := make([]byte, cliTokenBytes)
	if _, err := rand.Read(secret); err != nil {
		return nil, fmt.Errorf("auth: 生成 CLI 令牌失败: %w", err)
	}
	plaintext := cliTokenPrefix + base64.RawURLEncoding.EncodeToString(secret)
	expiresAt := now.Add(ttl)
	digest := sha256.Sum256([]byte(plaintext))
	token := &CLIToken{
		UserID:      user.ID,
		TokenPrefix: plaintext[:12],
		TokenHash:   fmt.Sprintf("%x", digest[:]),
		ExpiresAt:   expiresAt,
	}
	if err := database.Create(token).Error; err != nil {
		return nil, fmt.Errorf("auth: 保存 CLI 令牌失败: %w", err)
	}

	return &CLITokenIssue{Token: plaintext, ExpiresAt: expiresAt}, nil
}

// AuthenticateCLIToken 校验令牌、用户状态，并更新最近使用时间。
// AuthenticateCLIToken validates the token and user, then updates last-used time.
func AuthenticateCLIToken(database *gorm.DB, plaintext string, now time.Time) (*CLIToken, *User, error) {
	if database == nil {
		return nil, nil, errors.New("auth: 数据库连接未初始化")
	}
	if strings.TrimSpace(plaintext) == "" {
		return nil, nil, ErrInvalidCLIToken
	}
	if now.IsZero() {
		now = time.Now()
	}

	digest := sha256.Sum256([]byte(plaintext))
	var token CLIToken
	if err := database.Where("token_hash = ?", fmt.Sprintf("%x", digest[:])).First(&token).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrInvalidCLIToken
		}
		return nil, nil, fmt.Errorf("auth: 查询 CLI 令牌失败: %w", err)
	}
	if token.RevokedAt != nil {
		return nil, nil, ErrCLITokenRevoked
	}
	if !token.ExpiresAt.After(now) {
		return nil, nil, ErrCLITokenExpired
	}

	user, err := FindByID(database, token.UserID)
	if err != nil {
		return nil, nil, ErrInvalidCLIToken
	}
	if !user.IsActive {
		return nil, nil, ErrUserInactive
	}
	if err := database.Model(&CLIToken{}).Where("id = ?", token.ID).Update("last_used_at", now).Error; err != nil {
		return nil, nil, fmt.Errorf("auth: 更新 CLI 令牌使用时间失败: %w", err)
	}
	token.LastUsedAt = &now
	return &token, user, nil
}

// RevokeCLIToken 撤销令牌；重复撤销保持成功，便于登出请求幂等。
// RevokeCLIToken revokes a token; repeated revocation remains successful.
func RevokeCLIToken(database *gorm.DB, plaintext string, now time.Time) error {
	if database == nil {
		return errors.New("auth: 数据库连接未初始化")
	}
	if strings.TrimSpace(plaintext) == "" {
		return ErrInvalidCLIToken
	}
	if now.IsZero() {
		now = time.Now()
	}
	digest := sha256.Sum256([]byte(plaintext))
	var token CLIToken
	if err := database.Where("token_hash = ?", fmt.Sprintf("%x", digest[:])).First(&token).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrInvalidCLIToken
		}
		return fmt.Errorf("auth: 查询 CLI 令牌失败: %w", err)
	}
	if token.RevokedAt != nil {
		return nil
	}
	return database.Model(&CLIToken{}).Where("id = ? AND revoked_at IS NULL", token.ID).Update("revoked_at", now).Error
}

// CLILoginRequest 是 CLI 登录请求。
// CLILoginRequest is the CLI login request.
type CLILoginRequest struct {
	Username  string `json:"username" binding:"required"`
	Password  string `json:"password" binding:"required"`
	ExpiresIn string `json:"expires_in"`
}

// CLILoginData 是 CLI 登录响应中的令牌信息。
// CLILoginData contains the token information in a CLI login response.
type CLILoginData struct {
	Token     string    `json:"token"`
	TokenType string    `json:"token_type"`
	ExpiresAt time.Time `json:"expires_at"`
	User      *UserInfo `json:"user"`
}

// CLILoginResponse 是 CLI 登录响应。
// CLILoginResponse is the CLI login response.
type CLILoginResponse struct {
	ErrorMsg string        `json:"error_msg"`
	Data     *CLILoginData `json:"data"`
}

// CLILogin 通过用户名密码签发 CLI Bearer 令牌。
// CLILogin issues a CLI Bearer token after username/password verification.
// @Tags auth
// @Accept json
// @Produce json
// @Param request body CLILoginRequest true "CLI 登录请求"
// @Success 200 {object} CLILoginResponse
// @Router /api/v1/auth/cli/login [post]
func CLILogin(c *gin.Context) {
	if !isCLIRequest(c) {
		c.JSON(http.StatusBadRequest, CLILoginResponse{ErrorMsg: ErrMsgCLIHeaderRequired})
		return
	}

	var req CLILoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, CLILoginResponse{ErrorMsg: ErrMsgEmptyCredentials})
		return
	}
	username := strings.TrimSpace(req.Username)
	if username == "" || req.Password == "" {
		c.JSON(http.StatusBadRequest, CLILoginResponse{ErrorMsg: ErrMsgEmptyCredentials})
		return
	}

	database := db.GetDB(c.Request.Context())
	if database == nil {
		c.JSON(http.StatusInternalServerError, CLILoginResponse{ErrorMsg: ErrMsgInternalError})
		return
	}
	user, err := FindByUsername(database, username)
	if err != nil || !user.CheckPassword(req.Password) {
		logger.InfoF(c.Request.Context(), "[Auth] CLI 登录失败: %s", username)
		c.JSON(http.StatusUnauthorized, CLILoginResponse{ErrorMsg: ErrMsgInvalidCredentials})
		return
	}
	if !user.IsActive {
		c.JSON(http.StatusForbidden, CLILoginResponse{ErrorMsg: ErrMsgUserInactive})
		return
	}

	issue, err := IssueCLIToken(database, user, req.ExpiresIn, time.Now())
	if err != nil {
		if errors.Is(err, ErrInvalidCLITokenTTL) {
			c.JSON(http.StatusBadRequest, CLILoginResponse{ErrorMsg: ErrInvalidCLITokenTTL.Error()})
			return
		}
		logger.ErrorF(c.Request.Context(), "[Auth] 签发 CLI 令牌失败: %v", err)
		c.JSON(http.StatusInternalServerError, CLILoginResponse{ErrorMsg: ErrMsgInternalError})
		return
	}

	logger.InfoF(c.Request.Context(), "[Auth] CLI 登录成功: user_id=%d username=%s token_prefix=%s", user.ID, user.Username, issue.Token[:12])
	c.JSON(http.StatusOK, CLILoginResponse{Data: &CLILoginData{
		Token:     issue.Token,
		TokenType: "Bearer",
		ExpiresAt: issue.ExpiresAt,
		User:      user.ToUserInfo(),
	}})
}

// CLILogout 撤销当前 CLI Bearer 令牌。
// CLILogout revokes the current CLI Bearer token.
// @Tags auth
// @Produce json
// @Success 200 {object} LogoutResponse
// @Router /api/v1/auth/cli/logout [post]
func CLILogout(c *gin.Context) {
	token, ok := bearerToken(c.GetHeader("Authorization"))
	if !ok {
		c.JSON(http.StatusUnauthorized, LogoutResponse{ErrorMsg: ErrMsgCLITokenInvalid})
		return
	}
	if err := RevokeCLIToken(db.GetDB(c.Request.Context()), token, time.Now()); err != nil {
		if errors.Is(err, ErrInvalidCLIToken) || errors.Is(err, ErrCLITokenRevoked) {
			c.JSON(http.StatusUnauthorized, LogoutResponse{ErrorMsg: ErrMsgCLITokenInvalid})
			return
		}
		logger.ErrorF(c.Request.Context(), "[Auth] CLI 登出失败: %v", err)
		c.JSON(http.StatusInternalServerError, LogoutResponse{ErrorMsg: ErrMsgInternalError})
		return
	}
	c.JSON(http.StatusOK, LogoutResponse{})
}

// CLIWhoAmI 返回当前 CLI 令牌对应的用户。
// CLIWhoAmI returns the user associated with the current CLI token.
// @Tags auth
// @Produce json
// @Success 200 {object} UserInfoResponse
// @Router /api/v1/auth/cli/whoami [get]
func CLIWhoAmI(c *gin.Context) {
	user := GetUserFromContext(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, UserInfoResponse{ErrorMsg: ErrMsgCLITokenInvalid})
		return
	}
	c.JSON(http.StatusOK, UserInfoResponse{Data: user.ToUserInfo()})
}
