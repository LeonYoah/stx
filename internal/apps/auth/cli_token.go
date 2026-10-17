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
	"unicode/utf8"

	"github.com/LeonYoah/stx/internal/db"
	"github.com/LeonYoah/stx/internal/logger"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	// DefaultCLITokenTTL 是密码登录未指定有效期时使用的期限。
	// DefaultCLITokenTTL is used when password login does not specify an expiry.
	DefaultCLITokenTTL = 7 * 24 * time.Hour

	// MinCLITokenTTLDays 是自定义有效期允许的最少天数。
	// MinCLITokenTTLDays is the minimum custom lifetime in days.
	MinCLITokenTTLDays = 1
	// MaxCLITokenTTLDays 是自定义有效期允许的最多天数。
	// MaxCLITokenTTLDays is the maximum custom lifetime in days.
	MaxCLITokenTTLDays = 3650

	// CLITokenStatusActive 表示令牌可用。
	// CLITokenStatusActive means the token can still be used.
	CLITokenStatusActive = "active"
	// CLITokenStatusExpired 表示令牌已过期。
	// CLITokenStatusExpired means the token has expired.
	CLITokenStatusExpired = "expired"
	// CLITokenStatusRevoked 表示令牌已吊销。
	// CLITokenStatusRevoked means the token has been revoked.
	CLITokenStatusRevoked = "revoked"

	cliTokenPrefix     = "stx_"
	cliTokenBytes      = 32
	cliTokenNameMaxLen = 100
)

var (
	// ErrInvalidCLITokenTTL 表示令牌有效期不是 1d-3650d 或 never。
	// ErrInvalidCLITokenTTL means the lifetime is not 1d-3650d or never.
	ErrInvalidCLITokenTTL = errors.New("auth: CLI 令牌有效期必须是 1d 到 3650d，或 never")
	// ErrInvalidCLIToken 表示令牌不存在、已撤销或已过期。
	// ErrInvalidCLIToken means the token is unknown, revoked, or expired.
	ErrInvalidCLIToken = errors.New("auth: CLI 令牌无效")
	// ErrCLITokenRevoked 表示令牌已经撤销。
	// ErrCLITokenRevoked means the token has been revoked.
	ErrCLITokenRevoked = errors.New("auth: CLI 令牌已撤销")
	// ErrCLITokenExpired 表示令牌已经过期。
	// ErrCLITokenExpired means the token has expired.
	ErrCLITokenExpired = errors.New("auth: CLI 令牌已过期")
	// ErrCLITokenNameRequired 表示管理员签发令牌时未填写名称。
	// ErrCLITokenNameRequired means an admin-issued token is missing a name.
	ErrCLITokenNameRequired = errors.New("auth: CLI 令牌名称不能为空")
	// ErrCLITokenNameTooLong 表示令牌名称超过 100 个字符。
	// ErrCLITokenNameTooLong means the token name exceeds 100 characters.
	ErrCLITokenNameTooLong = errors.New("auth: CLI 令牌名称不能超过 100 个字符")
	// ErrCLITokenNotFound 表示按编号找不到令牌。
	// ErrCLITokenNotFound means the token id does not exist.
	ErrCLITokenNotFound = errors.New("auth: CLI 令牌不存在")
)

// CLIToken 保存 CLI 令牌的不可逆信息，不保存令牌原文。
// CLIToken stores irreversible token data and never stores the plaintext token.
type CLIToken struct {
	ID          uint64     `json:"id" gorm:"primaryKey;autoIncrement"`
	UserID      uint64     `json:"user_id" gorm:"index;not null"`
	Name        string     `json:"name" gorm:"size:100;index"`
	TokenPrefix string     `json:"token_prefix" gorm:"size:16;index;not null"`
	TokenHash   string     `json:"-" gorm:"column:token_hash;size:64;unique;not null"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty" gorm:"index"`
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
	ID        uint64     `json:"id"`
	Token     string     `json:"token"`
	Prefix    string     `json:"prefix"`
	Name      string     `json:"name,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	Permanent bool       `json:"permanent"`
}

// CLITokenLifetime 表示解析后的令牌有效期。
// CLITokenLifetime is a parsed token lifetime.
type CLITokenLifetime struct {
	Permanent bool
	TTL       time.Duration
}

// CLITokenInfo 是管理员列表使用的令牌元数据，不含哈希和原文。
// CLITokenInfo is administrator list metadata without the hash or plaintext.
type CLITokenInfo struct {
	ID          uint64     `json:"id"`
	UserID      uint64     `json:"user_id"`
	Username    string     `json:"username"`
	Name        string     `json:"name"`
	TokenPrefix string     `json:"token_prefix"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	RevokedAt   *time.Time `json:"revoked_at,omitempty"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	Permanent   bool       `json:"permanent"`
	Status      string     `json:"status"`
}

// CLITokenFilter 是管理员令牌列表的筛选条件。
// CLITokenFilter is the administrator token list filter.
type CLITokenFilter struct {
	UserID   uint64
	Username string
	Name     string
	Status   string
	Page     int
	PageSize int
}

// CLITokenCounts 是令牌状态统计。
// CLITokenCounts is the token status summary.
type CLITokenCounts struct {
	Total   int64 `json:"total"`
	Active  int64 `json:"active"`
	Expired int64 `json:"expired"`
	Revoked int64 `json:"revoked"`
}

type cliTokenListRow struct {
	CLIToken
	Username string `gorm:"column:username"`
}

// ParseCLITokenTTL 校验并解析 7d/30d/90d/never 或自定义 Nd。
// ParseCLITokenTTL validates presets 7d/30d/90d/never or a custom Nd lifetime.
func ParseCLITokenTTL(value string) (CLITokenLifetime, error) {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return CLITokenLifetime{TTL: DefaultCLITokenTTL}, nil
	}
	if value == "never" || value == "permanent" {
		return CLITokenLifetime{Permanent: true}, nil
	}
	if !strings.HasSuffix(value, "d") {
		return CLITokenLifetime{}, ErrInvalidCLITokenTTL
	}
	days, err := strconv.Atoi(strings.TrimSuffix(value, "d"))
	if err != nil || days < MinCLITokenTTLDays || days > MaxCLITokenTTLDays {
		return CLITokenLifetime{}, ErrInvalidCLITokenTTL
	}
	return CLITokenLifetime{TTL: time.Duration(days) * 24 * time.Hour}, nil
}

// IssueCLIToken 校验用户并生成随机令牌，只把哈希和元数据写入数据库。
// IssueCLIToken validates the user and stores only a hash and metadata.
func IssueCLIToken(database *gorm.DB, user *User, expiresIn string, now time.Time) (*CLITokenIssue, error) {
	return issueCLIToken(database, user, "", expiresIn, now)
}

// IssueNamedCLIToken 由管理员签发带名称的 CLI 令牌，原文只返回一次。
// IssueNamedCLIToken issues a named CLI token for an administrator; plaintext is returned once.
func IssueNamedCLIToken(database *gorm.DB, user *User, name, expiresIn string, now time.Time) (*CLITokenIssue, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrCLITokenNameRequired
	}
	if utf8.RuneCountInString(name) > cliTokenNameMaxLen {
		return nil, ErrCLITokenNameTooLong
	}
	return issueCLIToken(database, user, name, expiresIn, now)
}

func issueCLIToken(database *gorm.DB, user *User, name, expiresIn string, now time.Time) (*CLITokenIssue, error) {
	if database == nil {
		return nil, errors.New("auth: 数据库连接未初始化")
	}
	if user == nil || user.ID == 0 || !user.IsActive {
		return nil, ErrUserInactive
	}

	lifetime, err := ParseCLITokenTTL(expiresIn)
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
	digest := sha256.Sum256([]byte(plaintext))
	token := &CLIToken{
		UserID:      user.ID,
		Name:        name,
		TokenPrefix: plaintext[:12],
		TokenHash:   fmt.Sprintf("%x", digest[:]),
	}
	var expiresAt *time.Time
	if !lifetime.Permanent {
		value := now.Add(lifetime.TTL)
		expiresAt = &value
		token.ExpiresAt = expiresAt
	}
	if err := database.Create(token).Error; err != nil {
		return nil, fmt.Errorf("auth: 保存 CLI 令牌失败: %w", err)
	}

	return &CLITokenIssue{
		ID:        token.ID,
		Token:     plaintext,
		Prefix:    token.TokenPrefix,
		Name:      name,
		ExpiresAt: expiresAt,
		Permanent: lifetime.Permanent,
	}, nil
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
	if token.ExpiresAt != nil && !token.ExpiresAt.After(now) {
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

// FindCLITokenByID 按编号读取令牌元数据。
// FindCLITokenByID loads token metadata by id.
func FindCLITokenByID(database *gorm.DB, id uint64) (*CLIToken, error) {
	if database == nil {
		return nil, errors.New("auth: 数据库连接未初始化")
	}
	if id == 0 {
		return nil, ErrCLITokenNotFound
	}
	var token CLIToken
	if err := database.First(&token, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrCLITokenNotFound
		}
		return nil, fmt.Errorf("auth: 查询 CLI 令牌失败: %w", err)
	}
	return &token, nil
}

// RevokeCLITokenByID 按编号吊销令牌；重复吊销保持成功。
// RevokeCLITokenByID revokes a token by id; repeated revocation remains successful.
func RevokeCLITokenByID(database *gorm.DB, id uint64, now time.Time) (*CLIToken, error) {
	token, err := FindCLITokenByID(database, id)
	if err != nil {
		return nil, err
	}
	if now.IsZero() {
		now = time.Now()
	}
	if token.RevokedAt != nil {
		return token, nil
	}
	if err := database.Model(&CLIToken{}).Where("id = ? AND revoked_at IS NULL", token.ID).Update("revoked_at", now).Error; err != nil {
		return nil, fmt.Errorf("auth: 吊销 CLI 令牌失败: %w", err)
	}
	token.RevokedAt = &now
	return token, nil
}

// ListCLITokens 返回管理员可见的令牌元数据与状态统计。
// ListCLITokens returns administrator-visible token metadata and status counts.
func ListCLITokens(database *gorm.DB, filter CLITokenFilter, now time.Time) ([]CLITokenInfo, int64, CLITokenCounts, error) {
	if database == nil {
		return nil, 0, CLITokenCounts{}, errors.New("auth: 数据库连接未初始化")
	}
	if now.IsZero() {
		now = time.Now()
	}
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 {
		filter.PageSize = 20
	}
	if filter.PageSize > 100 {
		filter.PageSize = 100
	}

	base := database.Table("auth_cli_tokens AS t").
		Joins("LEFT JOIN auth_users AS u ON u.id = t.user_id")
	base = applyCLITokenMetadataFilter(base, filter)

	counts, err := countCLITokens(base, now)
	if err != nil {
		return nil, 0, CLITokenCounts{}, err
	}

	listQuery := applyCLITokenStatusFilter(base.Session(&gorm.Session{}), filter.Status, now)
	var total int64
	if err := listQuery.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, CLITokenCounts{}, fmt.Errorf("auth: 统计 CLI 令牌失败: %w", err)
	}

	var rows []cliTokenListRow
	offset := (filter.Page - 1) * filter.PageSize
	if err := listQuery.Select("t.id, t.user_id, t.name, t.token_prefix, t.expires_at, t.revoked_at, t.last_used_at, t.created_at, t.updated_at, u.username AS username").
		Order("t.id DESC").
		Offset(offset).
		Limit(filter.PageSize).
		Scan(&rows).Error; err != nil {
		return nil, 0, CLITokenCounts{}, fmt.Errorf("auth: 查询 CLI 令牌失败: %w", err)
	}

	items := make([]CLITokenInfo, 0, len(rows))
	for _, row := range rows {
		items = append(items, row.CLIToken.ToInfo(row.Username, now))
	}
	return items, total, counts, nil
}

func applyCLITokenMetadataFilter(query *gorm.DB, filter CLITokenFilter) *gorm.DB {
	if filter.UserID > 0 {
		query = query.Where("t.user_id = ?", filter.UserID)
	}
	if username := strings.TrimSpace(filter.Username); username != "" {
		query = query.Where("LOWER(u.username) LIKE LOWER(?)", username+"%")
	}
	if name := strings.TrimSpace(filter.Name); name != "" {
		query = query.Where("LOWER(t.name) LIKE LOWER(?)", "%"+name+"%")
	}
	return query
}

func applyCLITokenStatusFilter(query *gorm.DB, status string, now time.Time) *gorm.DB {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case CLITokenStatusRevoked:
		return query.Where("t.revoked_at IS NOT NULL")
	case CLITokenStatusExpired:
		return query.Where("t.revoked_at IS NULL AND t.expires_at IS NOT NULL AND t.expires_at <= ?", now)
	case CLITokenStatusActive:
		return query.Where("t.revoked_at IS NULL AND (t.expires_at IS NULL OR t.expires_at > ?)", now)
	default:
		return query
	}
}

func countCLITokens(base *gorm.DB, now time.Time) (CLITokenCounts, error) {
	var counts CLITokenCounts
	if err := base.Session(&gorm.Session{}).Count(&counts.Total).Error; err != nil {
		return CLITokenCounts{}, fmt.Errorf("auth: 统计 CLI 令牌失败: %w", err)
	}
	if err := applyCLITokenStatusFilter(base.Session(&gorm.Session{}), CLITokenStatusActive, now).Count(&counts.Active).Error; err != nil {
		return CLITokenCounts{}, fmt.Errorf("auth: 统计有效 CLI 令牌失败: %w", err)
	}
	if err := applyCLITokenStatusFilter(base.Session(&gorm.Session{}), CLITokenStatusExpired, now).Count(&counts.Expired).Error; err != nil {
		return CLITokenCounts{}, fmt.Errorf("auth: 统计过期 CLI 令牌失败: %w", err)
	}
	if err := applyCLITokenStatusFilter(base.Session(&gorm.Session{}), CLITokenStatusRevoked, now).Count(&counts.Revoked).Error; err != nil {
		return CLITokenCounts{}, fmt.Errorf("auth: 统计已吊销 CLI 令牌失败: %w", err)
	}
	return counts, nil
}

// CLITokenStatusOf 根据吊销和过期时间计算令牌状态。
// CLITokenStatusOf computes token status from revocation and expiry.
func CLITokenStatusOf(token *CLIToken, now time.Time) string {
	if token == nil {
		return ""
	}
	if token.RevokedAt != nil {
		return CLITokenStatusRevoked
	}
	if token.ExpiresAt != nil && !token.ExpiresAt.After(now) {
		return CLITokenStatusExpired
	}
	return CLITokenStatusActive
}

// ToInfo 把令牌行转成不含哈希的展示结构。
// ToInfo converts a token row into display metadata without the hash.
func (t CLIToken) ToInfo(username string, now time.Time) CLITokenInfo {
	if now.IsZero() {
		now = time.Now()
	}
	return CLITokenInfo{
		ID:          t.ID,
		UserID:      t.UserID,
		Username:    username,
		Name:        t.Name,
		TokenPrefix: t.TokenPrefix,
		ExpiresAt:   t.ExpiresAt,
		RevokedAt:   t.RevokedAt,
		LastUsedAt:  t.LastUsedAt,
		CreatedAt:   t.CreatedAt,
		Permanent:   t.ExpiresAt == nil,
		Status:      CLITokenStatusOf(&t, now),
	}
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
	Token     string     `json:"token"`
	TokenType string     `json:"token_type"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	Permanent bool       `json:"permanent"`
	User      *UserInfo  `json:"user"`
}

// CLILoginResponse 是 CLI 登录响应。
// CLILoginResponse is the CLI login response.
type CLILoginResponse struct {
	ErrorMsg string        `json:"error_msg"`
	Data     *CLILoginData `json:"data"`
}

// CLIWhoAmIData 是 whoami 返回的用户和当前令牌元数据。
// CLIWhoAmIData is the whoami payload with the current token metadata.
type CLIWhoAmIData struct {
	UserInfo
	TokenID        uint64     `json:"token_id,omitempty"`
	TokenPrefix    string     `json:"token_prefix,omitempty"`
	TokenName      string     `json:"token_name,omitempty"`
	TokenExpiresAt *time.Time `json:"token_expires_at,omitempty"`
	Permanent      bool       `json:"permanent"`
}

// CLIWhoAmIResponse 是 CLI whoami 响应。
// CLIWhoAmIResponse is the CLI whoami response.
type CLIWhoAmIResponse struct {
	ErrorMsg string         `json:"error_msg"`
	Data     *CLIWhoAmIData `json:"data"`
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

	logger.InfoF(c.Request.Context(), "[Auth] CLI 登录成功: user_id=%d username=%s token_prefix=%s", user.ID, user.Username, issue.Prefix)
	c.JSON(http.StatusOK, CLILoginResponse{Data: &CLILoginData{
		Token:     issue.Token,
		TokenType: "Bearer",
		ExpiresAt: issue.ExpiresAt,
		Permanent: issue.Permanent,
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

// CLIWhoAmI 返回当前 CLI 令牌对应的用户和令牌元数据。
// CLIWhoAmI returns the user and token metadata associated with the current CLI token.
// @Tags auth
// @Produce json
// @Success 200 {object} CLIWhoAmIResponse
// @Router /api/v1/auth/cli/whoami [get]
func CLIWhoAmI(c *gin.Context) {
	user := GetUserFromContext(c)
	if user == nil {
		c.JSON(http.StatusUnauthorized, CLIWhoAmIResponse{ErrorMsg: ErrMsgCLITokenInvalid})
		return
	}
	data := &CLIWhoAmIData{UserInfo: *user.ToUserInfo()}
	if token := GetCLITokenFromGin(c); token != nil {
		data.TokenID = token.ID
		data.TokenPrefix = token.TokenPrefix
		data.TokenName = token.Name
		data.TokenExpiresAt = token.ExpiresAt
		data.Permanent = token.ExpiresAt == nil
	}
	c.JSON(http.StatusOK, CLIWhoAmIResponse{Data: data})
}
