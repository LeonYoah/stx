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

package installer

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/LeonYoah/stx/internal/utils/secretbox"
)

// ContainerImage 注册的 SeaTunnel 容器镜像（K8s 部署/升级引用）。
// ContainerImage is a registered SeaTunnel container image referenced by Kubernetes
// deploys and upgrades.
type ContainerImage struct {
	ID               uint   `json:"id" gorm:"primaryKey;autoIncrement"`
	Name             string `json:"name" gorm:"size:100;uniqueIndex;not null"`
	Image            string `json:"image" gorm:"size:512;not null"`
	SeatunnelVersion string `json:"seatunnel_version" gorm:"column:seatunnel_version;size:32"`
	ImagePullPolicy  string `json:"image_pull_policy" gorm:"column:image_pull_policy;size:32"`
	ImagePullSecrets string `json:"image_pull_secrets" gorm:"column:image_pull_secrets;size:512"`
	// 私有仓库凭据：密码 secretbox 加密落库且永不随响应返回（json:"-"），
	// 前端仅凭 HasRegistryAuth 判断是否已配置。
	// Private registry credentials: the password is secretbox-encrypted at rest and never
	// serialized (json:"-"); the frontend only sees HasRegistryAuth.
	RegistryHost     string     `json:"registry_host" gorm:"column:registry_host;size:255"`
	RegistryUsername string     `json:"registry_username" gorm:"column:registry_username;size:255"`
	RegistryPassword string     `json:"-" gorm:"column:registry_password;size:1024"`
	HasRegistryAuth  bool       `json:"has_registry_auth" gorm:"-"`
	IsDefault        bool       `json:"is_default" gorm:"column:is_default"`
	Description      string     `json:"description" gorm:"size:512"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// TableName 固定表名 container_images。
// TableName pins the table name.
func (ContainerImage) TableName() string { return "container_images" }

// ContainerImageRequest 创建/更新容器镜像的请求体。
// ContainerImageRequest is the create/update payload for a container image.
type ContainerImageRequest struct {
	Name             string `json:"name"`
	Image            string `json:"image"`
	SeatunnelVersion string `json:"seatunnel_version"`
	ImagePullPolicy  string `json:"image_pull_policy"`
	ImagePullSecrets string `json:"image_pull_secrets"`
	IsDefault        bool   `json:"is_default"`
	Description      string `json:"description"`
	// 私有仓库凭据（可选）：password 留空表示保持不变（更新时）或不设置（创建时）。
	// Optional private registry credentials: an empty password keeps the existing one
	// on update or means unset on create.
	RegistryHost     string `json:"registry_host"`
	RegistryUsername string `json:"registry_username"`
	RegistryPassword string `json:"registry_password"`
}

// ContainerImageListResult 列表响应：items + 由版本目录生成的官方镜像预设。
// ContainerImageListResult is the list response: registered items plus official
// presets derived from the version catalog.
type ContainerImageListResult struct {
	Items           []*ContainerImage `json:"items"`
	OfficialPresets []*ContainerImage `json:"official_presets"`
}

// validateContainerImageRequest 校验镜像请求的最小字段与默认值。
// validateContainerImageRequest validates minimal fields and applies defaults.
func validateContainerImageRequest(req *ContainerImageRequest) error {
	// 私有仓库凭据完整性：host 与 username 必须成对出现；password 在创建时必填、
	// 更新时留空表示保持（由调用方区分，校验层只拦半截状态）。
	// Registry credential completeness: host and username must come as a pair; the
	// password is required on create and empty-means-keep on update (the caller knows
	// which), so validation only rejects half-filled states.
	host := strings.TrimSpace(req.RegistryHost)
	username := strings.TrimSpace(req.RegistryUsername)
	password := strings.TrimSpace(req.RegistryPassword)
	filled := 0
	for _, v := range []string{host, username, password} {
		if v != "" {
			filled++
		}
	}
	if filled > 0 && host == "" {
		return errors.New("installer: registry host is required when credentials are provided")
	}
	if filled > 0 && username == "" {
		return errors.New("installer: registry username is required when credentials are provided")
	}

	req.Name = strings.TrimSpace(req.Name)
	req.Image = strings.TrimSpace(req.Image)
	req.SeatunnelVersion = strings.TrimSpace(req.SeatunnelVersion)
	req.ImagePullPolicy = strings.TrimSpace(req.ImagePullPolicy)
	// registry_host 统一清洗（剥离误带 scheme 与尾斜杠），与 Secret 代建处同一口径。
	// Registry host is normalized in one place (scheme and trailing slashes stripped),
	// matching the secret materialization path.
	req.RegistryHost = sanitizeRegistryHostForImage(req.RegistryHost)
	if req.Name == "" {
		return errors.New("name is required")
	}
	if req.Image == "" {
		return errors.New("image is required")
	}
	// 误带 scheme 的 image 引用必然后续拉取失败，明确校验错误而不是放进库等炸。
	// An image reference carrying a scheme always fails to pull later, so reject it
	// with a clear validation error instead of persisting a time bomb.
	if strings.HasPrefix(req.Image, "http://") || strings.HasPrefix(req.Image, "https://") {
		return errors.New("image must not include a scheme (http:// or https://); use the bare registry host")
	}
	if !strings.Contains(req.Image, ":") && !strings.Contains(req.Image, "@") {
		return errors.New("image must include a tag or digest")
	}
	if req.ImagePullPolicy == "" {
		req.ImagePullPolicy = "IfNotPresent"
	}
	switch req.ImagePullPolicy {
	case "Always", "IfNotPresent", "Never":
	default:
		return errors.New("image_pull_policy must be Always, IfNotPresent or Never")
	}
	return nil
}

// ListContainerImages 返回已注册镜像与官方预设。
// ListContainerImages returns registered images plus official presets.
func (s *Service) ListContainerImages(ctx context.Context) (*ContainerImageListResult, error) {
	items := []*ContainerImage{}
	if err := s.db.WithContext(ctx).Order("is_default DESC, id ASC").Find(&items).Error; err != nil {
		return nil, err
	}
	for _, item := range items {
		item.HasRegistryAuth = item.HasCredentials()
	}
	result := &ContainerImageListResult{Items: items, OfficialPresets: []*ContainerImage{}}
	if versions, err := s.ListAvailableVersions(ctx); err == nil && versions != nil {
		for _, v := range versions.Versions {
			version := strings.TrimSpace(v)
			if version == "" {
				continue
			}
			result.OfficialPresets = append(result.OfficialPresets, &ContainerImage{
				Name:             fmt.Sprintf("apache/seatunnel:%s", version),
				Image:            fmt.Sprintf("apache/seatunnel:%s", version),
				SeatunnelVersion: version,
				ImagePullPolicy:  "IfNotPresent",
				Description:      "official preset",
			})
		}
	}
	return result, nil
}

// applyRegistryPassword 加密并设置注册表密码（空密码=不设置）。
// applyRegistryPassword encrypts and sets the registry password (empty = unset).
func applyRegistryPassword(image *ContainerImage, password string) error {
	trimmed := strings.TrimSpace(password)
	if trimmed == "" {
		return nil
	}
	encrypted, err := secretbox.Encrypt(trimmed)
	if err != nil {
		return fmt.Errorf("installer: encrypt registry password: %w", err)
	}
	image.RegistryPassword = encrypted
	return nil
}

// DecryptRegistryPassword 解密注册表密码供部署期代建 imagePullSecret 使用。
// DecryptRegistryPassword decrypts the registry password for deploy-time pull-secret
// materialization.
func (i *ContainerImage) DecryptRegistryPassword() (string, error) {
	if strings.TrimSpace(i.RegistryPassword) == "" {
		return "", nil
	}
	return secretbox.Decrypt(i.RegistryPassword)
}

// HasCredentials 判定是否配置了完整可用的私有仓库凭据。
// HasCredentials reports whether complete private-registry credentials are configured.
func (i *ContainerImage) HasCredentials() bool {
	return strings.TrimSpace(i.RegistryHost) != "" && strings.TrimSpace(i.RegistryUsername) != "" && strings.TrimSpace(i.RegistryPassword) != ""
}

// FindContainerImageByImage 按镜像引用精确匹配注册行（部署期解析凭据用）。
// FindContainerImageByImage finds the registered row by exact image reference for
// deploy-time credential resolution.
func (s *Service) FindContainerImageByImage(ctx context.Context, image string) (*ContainerImage, error) {
	var row ContainerImage
	if err := s.db.WithContext(ctx).Where("image = ?", strings.TrimSpace(image)).First(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

// CreateContainerImage 注册新镜像；默认镜像唯一，置默认时清除其它默认。
// CreateContainerImage registers an image; the default flag is unique, so setting it
// clears other defaults.
func (s *Service) CreateContainerImage(ctx context.Context, req *ContainerImageRequest) (*ContainerImage, error) {
	if req == nil {
		return nil, errors.New("request is required")
	}
	if err := validateContainerImageRequest(req); err != nil {
		return nil, err
	}
	// 创建时凭据必须完整：host/username 已填而密码为空会产生永远无法生效的半截配置。
	// Creation requires complete credentials: host/username without a password would
	// persist a half-configured credential that can never pull.
	if strings.TrimSpace(req.RegistryHost) != "" && strings.TrimSpace(req.RegistryPassword) == "" {
		return nil, errors.New("installer: registry password is required when creating credentials")
	}
	image := &ContainerImage{
		Name:             req.Name,
		Image:            req.Image,
		SeatunnelVersion: req.SeatunnelVersion,
		ImagePullPolicy:  req.ImagePullPolicy,
		ImagePullSecrets: strings.TrimSpace(req.ImagePullSecrets),
		IsDefault:        req.IsDefault,
		Description:      strings.TrimSpace(req.Description),
		RegistryHost:     strings.TrimSpace(req.RegistryHost),
		RegistryUsername: strings.TrimSpace(req.RegistryUsername),
	}
	if err := applyRegistryPassword(image, req.RegistryPassword); err != nil {
		return nil, err
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if image.IsDefault {
			if err := tx.Model(&ContainerImage{}).Where("is_default = ?", true).Update("is_default", false).Error; err != nil {
				return err
			}
		}
		return tx.Create(image).Error
	})
	if err != nil {
		return nil, err
	}
	image.HasRegistryAuth = image.HasCredentials()
	return image, nil
}

// UpdateContainerImage 更新已注册镜像。
// UpdateContainerImage updates a registered image.
func (s *Service) UpdateContainerImage(ctx context.Context, id uint, req *ContainerImageRequest) (*ContainerImage, error) {
	if req == nil {
		return nil, errors.New("request is required")
	}
	if err := validateContainerImageRequest(req); err != nil {
		return nil, err
	}
	var existing ContainerImage
	if err := s.db.WithContext(ctx).First(&existing, id).Error; err != nil {
		return nil, err
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if req.IsDefault {
			if err := tx.Model(&ContainerImage{}).Where("is_default = ? AND id <> ?", true, id).Update("is_default", false).Error; err != nil {
				return err
			}
		}
		updates := map[string]interface{}{
			"name":               req.Name,
			"image":              req.Image,
			"seatunnel_version":  req.SeatunnelVersion,
			"image_pull_policy":  req.ImagePullPolicy,
			"image_pull_secrets": strings.TrimSpace(req.ImagePullSecrets),
			"is_default":         req.IsDefault,
			"description":        strings.TrimSpace(req.Description),
			"registry_host":      strings.TrimSpace(req.RegistryHost),
			"registry_username":  strings.TrimSpace(req.RegistryUsername),
		}
		// 密码留空=保持既有密文；显式传入才重加密覆盖。清空凭据靠清空 host/username。
		// An empty password keeps the existing ciphertext; only an explicit value is
		// re-encrypted. Clearing host/username is how credentials are removed.
		if trimmed := strings.TrimSpace(req.RegistryPassword); trimmed != "" {
			encrypted, err := secretbox.Encrypt(trimmed)
			if err != nil {
				return fmt.Errorf("installer: encrypt registry password: %w", err)
			}
			updates["registry_password"] = encrypted
		}
		return tx.Model(&ContainerImage{}).Where("id = ?", id).Updates(updates).Error
	})
	if err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).First(&existing, id).Error; err != nil {
		return nil, err
	}
	existing.HasRegistryAuth = existing.HasCredentials()
	return &existing, nil
}

// DeleteContainerImage 删除已注册镜像。
// DeleteContainerImage deletes a registered image.
func (s *Service) DeleteContainerImage(ctx context.Context, id uint) error {
	result := s.db.WithContext(ctx).Delete(&ContainerImage{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("container image not found")
	}
	return nil
}

// sanitizeRegistryHostForImage 与 k8s.SanitizeRegistryHost 同口径的本地实现：
// installer 包不反向依赖 k8s 包，保持分层。
// sanitizeRegistryHostForImage mirrors k8s.SanitizeRegistryHost locally: the installer
// package must not depend back on the k8s package per layering.
func sanitizeRegistryHostForImage(host string) string {
	trimmed := strings.TrimSpace(host)
	trimmed = strings.TrimPrefix(trimmed, "http://")
	trimmed = strings.TrimPrefix(trimmed, "https://")
	return strings.TrimRight(trimmed, "/")
}
