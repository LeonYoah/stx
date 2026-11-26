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
)

// ContainerImage 注册的 SeaTunnel 容器镜像（K8s 部署/升级引用）。
// ContainerImage is a registered SeaTunnel container image referenced by Kubernetes
// deploys and upgrades.
type ContainerImage struct {
	ID               uint       `json:"id" gorm:"primaryKey;autoIncrement"`
	Name             string     `json:"name" gorm:"size:100;uniqueIndex;not null"`
	Image            string     `json:"image" gorm:"size:512;not null"`
	SeatunnelVersion string     `json:"seatunnel_version" gorm:"column:seatunnel_version;size:32"`
	ImagePullPolicy  string     `json:"image_pull_policy" gorm:"column:image_pull_policy;size:32"`
	ImagePullSecrets string     `json:"image_pull_secrets" gorm:"column:image_pull_secrets;size:512"`
	IsDefault        bool       `json:"is_default" gorm:"column:is_default"`
	Description      string     `json:"description" gorm:"size:512"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	DeletedAt        *time.Time `json:"-" gorm:"index"`
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
	req.Name = strings.TrimSpace(req.Name)
	req.Image = strings.TrimSpace(req.Image)
	req.SeatunnelVersion = strings.TrimSpace(req.SeatunnelVersion)
	req.ImagePullPolicy = strings.TrimSpace(req.ImagePullPolicy)
	if req.Name == "" {
		return errors.New("name is required")
	}
	if req.Image == "" {
		return errors.New("image is required")
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
	image := &ContainerImage{
		Name:             req.Name,
		Image:            req.Image,
		SeatunnelVersion: req.SeatunnelVersion,
		ImagePullPolicy:  req.ImagePullPolicy,
		ImagePullSecrets: strings.TrimSpace(req.ImagePullSecrets),
		IsDefault:        req.IsDefault,
		Description:      strings.TrimSpace(req.Description),
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
		return tx.Model(&ContainerImage{}).Where("id = ?", id).Updates(map[string]interface{}{
			"name":              req.Name,
			"image":             req.Image,
			"seatunnel_version": req.SeatunnelVersion,
			"image_pull_policy": req.ImagePullPolicy,
			"image_pull_secrets": strings.TrimSpace(req.ImagePullSecrets),
			"is_default":        req.IsDefault,
			"description":       strings.TrimSpace(req.Description),
		}).Error
	})
	if err != nil {
		return nil, err
	}
	if err := s.db.WithContext(ctx).First(&existing, id).Error; err != nil {
		return nil, err
	}
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
