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

// Package auth 提供用户认证相关功能
package auth

import (
	"errors"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// 错误定义
var (
	ErrUserNotFound      = errors.New("auth: 用户不存在")
	ErrInvalidPassword   = errors.New("auth: 密码错误")
	ErrUserInactive      = errors.New("auth: 用户已禁用")
	ErrEmptyCredentials  = errors.New("auth: 用户名或密码不能为空")
	ErrPasswordTooShort  = errors.New("auth: 密码长度不能少于6位")
	ErrUserAlreadyExists = errors.New("auth: 用户名已存在")
)

// DefaultBcryptCost 默认 bcrypt 加密成本
const DefaultBcryptCost = 10

// DefaultLanguage 默认用户语言偏好
const DefaultLanguage = "zh"

// NormalizeLanguage 归一化用户语言偏好
func NormalizeLanguage(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "en":
		return "en"
	default:
		return DefaultLanguage
	}
}

// User 用户模型
// 用于存储系统用户信息，支持用户名密码认证和 OAuth 认证
type User struct {
	ID           uint64    `json:"id" gorm:"primaryKey;autoIncrement"`
	Username     string    `json:"username" gorm:"size:255;unique;not null"`
	PasswordHash string    `json:"-" gorm:"column:password_hash;size:255"` // 密码哈希，OAuth 用户可为空
	Nickname     string    `json:"nickname" gorm:"size:255"`
	Email        string    `json:"email" gorm:"size:255;index"`
	Language     string    `json:"language" gorm:"size:8;default:zh"`
	AvatarURL    string    `json:"avatar_url" gorm:"column:avatar_url;size:255"` // 头像 URL
	OAuthID      string    `json:"oauth_id" gorm:"column:oauth_id;size:255;index"`               // OAuth 提供商 ID，格式: provider:id
	IsActive     bool      `json:"is_active" gorm:"default:true"`
	IsAdmin      bool      `json:"is_admin" gorm:"default:false"`
	LastLoginAt  time.Time `json:"last_login_at"`
	CreatedAt    time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt    time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

// TableName 指定表名
// 使用 auth_users 避免与 oauth.User 的 users 表冲突
func (User) TableName() string {
	return "auth_users"
}

// SetPassword 设置用户密码
// 使用 bcrypt 算法对密码进行哈希处理
// cost 参数指定 bcrypt 的计算成本，如果为 0 则使用默认值
func (u *User) SetPassword(password string, cost int) error {
	if password == "" {
		return ErrEmptyCredentials
	}
	if len(password) < 6 {
		return ErrPasswordTooShort
	}

	if cost <= 0 {
		cost = DefaultBcryptCost
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	if err != nil {
		return err
	}

	u.PasswordHash = string(hash)
	return nil
}

// CheckPassword 验证密码是否正确
// 使用 bcrypt 算法比较密码哈希
func (u *User) CheckPassword(password string) bool {
	if password == "" || u.PasswordHash == "" {
		return false
	}

	err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password))
	return err == nil
}

// IsValidBcryptHash 检查密码哈希是否为有效的 bcrypt 哈希
func (u *User) IsValidBcryptHash() bool {
	if u.PasswordHash == "" {
		return false
	}
	// bcrypt 哈希以 $2a$, $2b$ 或 $2y$ 开头
	if len(u.PasswordHash) < 4 {
		return false
	}
	prefix := u.PasswordHash[:4]
	return prefix == "$2a$" || prefix == "$2b$" || prefix == "$2y$"
}

// FindByUsername 根据用户名查找用户
func FindByUsername(db *gorm.DB, username string) (*User, error) {
	var user User
	if err := db.Where("username = ?", username).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &user, nil
}

// FindByID 根据 ID 查找用户
func FindByID(db *gorm.DB, id uint64) (*User, error) {
	var user User
	if err := db.First(&user, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &user, nil
}

// Create 创建新用户
func (u *User) Create(db *gorm.DB) error {
	return db.Create(u).Error
}

// UpdateLastLogin 更新最后登录时间
func (u *User) UpdateLastLogin(db *gorm.DB) error {
	u.LastLoginAt = time.Now()
	return db.Model(u).Update("last_login_at", u.LastLoginAt).Error
}

// UserInfo 用户信息（用于 API 响应，不包含敏感信息）
type UserInfo struct {
	ID          uint64    `json:"id"`
	Username    string    `json:"username"`
	Nickname    string    `json:"nickname"`
	Email       string    `json:"email"`
	Language    string    `json:"language"`
	AvatarURL   string    `json:"avatar_url"`
	IsActive    bool      `json:"is_active"`
	IsAdmin     bool      `json:"is_admin"`
	LastLoginAt time.Time `json:"last_login_at"`
	CreatedAt   time.Time `json:"created_at"`
}

// ToUserInfo 将 User 转换为 UserInfo
func (u *User) ToUserInfo() *UserInfo {
	return &UserInfo{
		ID:          u.ID,
		Username:    u.Username,
		Nickname:    u.Nickname,
		Email:       u.Email,
		Language:    NormalizeLanguage(u.Language),
		AvatarURL:   u.AvatarURL,
		IsActive:    u.IsActive,
		IsAdmin:     u.IsAdmin,
		LastLoginAt: u.LastLoginAt,
		CreatedAt:   u.CreatedAt,
	}
}

// FindByOAuthID 根据 OAuth ID 查找用户
func FindByOAuthID(db *gorm.DB, oauthID string) (*User, error) {
	var user User
	err := db.Where("oauth_id = ?", oauthID).Order("id ASC").First(&user).Error
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "oauth_id") {
		err = db.Where("o_auth_id = ?", oauthID).Order("id ASC").First(&user).Error
	}
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &user, nil
}
