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
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/LeonYoah/stx/internal/pkg/regclient"
	"github.com/LeonYoah/stx/internal/utils/secretbox"
)

// DockerHubRegistryHost 官方公共仓库的登记 host（系统预置行，不可删除）。
// DockerHubRegistryHost is the registered host of the official public registry
// (system-seeded row, not deletable).
const DockerHubRegistryHost = "docker.io"

// DockerHubOfficialRepo docker.io 上 SeaTunnel 官方镜像路径的预填值（上游事实，可编辑）。
// DockerHubOfficialRepo is the prefilled official image path on docker.io (an
// upstream fact, still editable).
const DockerHubOfficialRepo = "apache/seatunnel"

// ContainerRegistry 登记的镜像仓库：凭据每仓库一份，部署/升级按镜像 host 精确匹配。
// Catalog/探测状态为缓存值，注册与手动测试时刷新。
// ContainerRegistry is a registered image registry: one credential set per host,
// matched exactly by image host at deploy/upgrade time. Catalog and probe status
// are cached values refreshed on registration and manual tests.
type ContainerRegistry struct {
	ID       uint   `json:"id" gorm:"primaryKey;autoIncrement"`
	Host     string `json:"host" gorm:"size:255;uniqueIndex;not null"`
	Username string `json:"username" gorm:"size:255"`
	// 密码 secretbox 加密落库且永不随响应返回（json:"-"）。
	// The password is secretbox-encrypted at rest and never serialized.
	Password string `json:"-" gorm:"size:1024"`
	// IsSystem 预置行（docker.io）：不可删除，用户名/密码字段无意义。
	// IsSystem marks seeded rows (docker.io): not deletable.
	IsSystem     bool   `json:"is_system" gorm:"column:is_system"`
	CatalogJSON  string `json:"-" gorm:"column:catalog_json;type:text"`
	CatalogRepos []string `json:"catalog_repos" gorm:"-"`
	ProbeScheme  string `json:"probe_scheme" gorm:"column:probe_scheme;size:8"`
	ProbeStatus  string `json:"probe_status" gorm:"column:probe_status;size:16"`
	ProbeError   string `json:"probe_error" gorm:"column:probe_error;size:512"`
	ProbeAt      *time.Time `json:"probe_at" gorm:"column:probe_at"`
	HasAuth      bool      `json:"has_auth" gorm:"-"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// TableName 固定表名 container_registries。
// TableName pins the table name.
func (ContainerRegistry) TableName() string { return "container_registries" }

// ContainerRegistryRequest 创建/更新仓库的请求体。
// ContainerRegistryRequest is the create/update payload.
type ContainerRegistryRequest struct {
	Host     string `json:"host"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// NormalizeRegistryHost 清洗仓库 host：剥离误带 scheme 与尾斜杠。
// NormalizeRegistryHost strips a mistakenly carried scheme and trailing slashes.
func NormalizeRegistryHost(raw string) string {
	host := strings.TrimSpace(raw)
	for _, scheme := range []string{"https://", "http://"} {
		host = strings.TrimPrefix(host, scheme)
	}
	return strings.TrimRight(host, "/")
}

// DecryptPassword 解密仓库密码；未设置返回空串。
// DecryptPassword decrypts the registry password; empty when unset.
func (r *ContainerRegistry) DecryptPassword() (string, error) {
	if strings.TrimSpace(r.Password) == "" {
		return "", nil
	}
	return secretbox.Decrypt(r.Password)
}

// HasCredentials 判定是否配置了完整凭据。
// HasCredentials reports whether complete credentials are configured.
func (r *ContainerRegistry) HasCredentials() bool {
	return strings.TrimSpace(r.Username) != "" && strings.TrimSpace(r.Password) != ""
}

// afterLoad 回填派生字段（HasAuth / CatalogRepos）。
// afterLoad populates derived fields.
func (r *ContainerRegistry) afterLoad() {
	r.HasAuth = r.HasCredentials()
	if strings.TrimSpace(r.CatalogJSON) != "" {
		_ = json.Unmarshal([]byte(r.CatalogJSON), &r.CatalogRepos)
	}
}

// RegistryImageHost 从镜像引用解析仓库 host：第一段含 "." 或 ":" 或为 localhost
// 才是 host，否则是 docker.io 公共镜像。
// RegistryImageHost extracts the registry host from an image reference: the first
// segment is a host only when it contains a dot or colon or is localhost; otherwise
// the image is a docker.io public one.
func RegistryImageHost(image string) string {
	image = strings.TrimSpace(image)
	if image == "" {
		return ""
	}
	if !strings.Contains(image, "/") {
		// 无斜杠的单名镜像（seatunnel:3.1.0）按 docker 语义就是 docker.io 公共镜像。
		// A slashless single-name image is a docker.io public image by docker semantics.
		return DockerHubRegistryHost
	}
	first := strings.SplitN(image, "/", 2)[0]
	if strings.ContainsAny(first, ".:") || first == "localhost" {
		return first
	}
	return DockerHubRegistryHost
}

var errRegistryNotFound = errors.New("installer: container registry not found")

// ErrRegistryNotFound 暴露哨兵错误供上层 errors.Is 判断。
// ErrRegistryNotFound exposes the sentinel for errors.Is checks.
func ErrRegistryNotFound() error { return errRegistryNotFound }

// validateRegistryRequest 校验仓库请求并清洗 host。
// validateRegistryRequest validates the payload and normalizes the host.
func validateRegistryRequest(req *ContainerRegistryRequest, requirePassword bool) error {
	req.Host = NormalizeRegistryHost(req.Host)
	req.Username = strings.TrimSpace(req.Username)
	if req.Host == "" {
		return errors.New("installer: registry host is required")
	}
	if strings.ContainsAny(req.Host, "/ \t") {
		return errors.New("installer: registry host must be a bare host[:port] without path")
	}
	if req.Username == "" && strings.TrimSpace(req.Password) != "" {
		return errors.New("installer: registry username is required when a password is provided")
	}
	if requirePassword && req.Username != "" && strings.TrimSpace(req.Password) == "" {
		return errors.New("installer: registry password is required when a username is provided")
	}
	return nil
}

// ListContainerRegistries 列出全部仓库并回填派生字段。
// ListContainerRegistries lists all registries with derived fields populated.
func (s *Service) ListContainerRegistries(ctx context.Context) ([]*ContainerRegistry, error) {
	var rows []*ContainerRegistry
	if err := s.db.WithContext(ctx).Order("is_system DESC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		row.afterLoad()
	}
	return rows, nil
}

// FindRegistryByHost 按 host 精确查找仓库。
// FindRegistryByHost finds a registry by exact host.
func (s *Service) FindRegistryByHost(ctx context.Context, host string) (*ContainerRegistry, error) {
	var row ContainerRegistry
	if err := s.db.WithContext(ctx).Where("host = ?", NormalizeRegistryHost(host)).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errRegistryNotFound
		}
		return nil, err
	}
	row.afterLoad()
	return &row, nil
}

// CreateContainerRegistry 创建仓库（host 唯一），成功后立即同步探测。
// CreateContainerRegistry creates a registry (unique host) and probes synchronously.
func (s *Service) CreateContainerRegistry(ctx context.Context, req *ContainerRegistryRequest) (*ContainerRegistry, error) {
	if err := validateRegistryRequest(req, true); err != nil {
		return nil, err
	}
	var count int64
	if err := s.db.WithContext(ctx).Model(&ContainerRegistry{}).Where("host = ?", req.Host).Count(&count).Error; err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, fmt.Errorf("installer: registry %s already exists", req.Host)
	}
	row := &ContainerRegistry{Host: req.Host, Username: req.Username}
	if strings.TrimSpace(req.Password) != "" {
		encrypted, err := secretbox.Encrypt(strings.TrimSpace(req.Password))
		if err != nil {
			return nil, err
		}
		row.Password = encrypted
	}
	if err := s.db.WithContext(ctx).Create(row).Error; err != nil {
		return nil, err
	}
	s.ProbeContainerRegistry(ctx, row)
	row.afterLoad()
	return row, nil
}

// UpdateContainerRegistry 更新仓库；密码留空保持原值，system 行仅允许更新探测缓存。
// UpdateContainerRegistry updates a registry; empty password keeps the stored one.
func (s *Service) UpdateContainerRegistry(ctx context.Context, id uint, req *ContainerRegistryRequest) (*ContainerRegistry, error) {
	var row ContainerRegistry
	if err := s.db.WithContext(ctx).First(&row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errRegistryNotFound
		}
		return nil, err
	}
	if err := validateRegistryRequest(req, false); err != nil {
		return nil, err
	}
	if row.IsSystem {
		return nil, errors.New("installer: system registry cannot be edited")
	}
	if req.Host != row.Host {
		var count int64
		if err := s.db.WithContext(ctx).Model(&ContainerRegistry{}).Where("host = ? AND id != ?", req.Host, row.ID).Count(&count).Error; err != nil {
			return nil, err
		}
		if count > 0 {
			return nil, fmt.Errorf("installer: registry %s already exists", req.Host)
		}
	}
	row.Host = req.Host
	row.Username = req.Username
	if strings.TrimSpace(req.Password) != "" {
		encrypted, err := secretbox.Encrypt(strings.TrimSpace(req.Password))
		if err != nil {
			return nil, err
		}
		row.Password = encrypted
	} else if req.Username == "" {
		// 用户名清空 = 明确改为匿名，密码一并清掉避免半截状态。
		// Clearing the username is an explicit switch to anonymous; the password is
		// dropped too so no half-configured state remains.
		row.Password = ""
	}
	if err := s.db.WithContext(ctx).Save(&row).Error; err != nil {
		return nil, err
	}
	s.ProbeContainerRegistry(ctx, &row)
	row.afterLoad()
	return &row, nil
}

// DeleteContainerRegistry 删除仓库；system 行不可删除。
// DeleteContainerRegistry deletes a registry; system rows are protected.
func (s *Service) DeleteContainerRegistry(ctx context.Context, id uint) error {
	var row ContainerRegistry
	if err := s.db.WithContext(ctx).First(&row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errRegistryNotFound
		}
		return err
	}
	if row.IsSystem {
		return errors.New("installer: system registry cannot be deleted")
	}
	return s.db.WithContext(ctx).Delete(&row).Error
}

// ProbeContainerRegistry 探测连通性并刷新 catalog 缓存（同步执行，10s 上限）。
// ProbeContainerRegistry probes connectivity and refreshes the catalog cache
// synchronously with the client's own timeout.
// ProbeRegistryTransient 注册前不落库探测：用户点「测试连接」时凭据尚未入库，
// 直接用表单值 ping+catalog 验证，成功后注册流程会再正式探测一次。
// ProbeRegistryTransient probes without persisting: the credentials are not yet
// stored when the user clicks "test connection" in the register dialog, so the
// form values probe the registry directly.
func (s *Service) ProbeRegistryTransient(ctx context.Context, host, username, password string) (string, string, []string, error) {
	req := &ContainerRegistryRequest{Host: host, Username: username, Password: password}
	if err := validateRegistryRequest(req, false); err != nil {
		return "", "", nil, err
	}
	normalized := req.Host
	if normalized == DockerHubRegistryHost {
		return "public", "https", nil, nil
	}
	client := regclient.New(normalized, username, password)
	if err := client.Ping(ctx); err != nil {
		return "error", client.Scheme(), nil, err
	}
	repos, err := client.Catalog(ctx)
	if err != nil {
		return "reachable", client.Scheme(), nil, nil
	}
	return "ok", client.Scheme(), repos, nil
}

func (s *Service) ProbeContainerRegistry(ctx context.Context, row *ContainerRegistry) {
	now := time.Now()
	row.ProbeAt = &now
	password, err := row.DecryptPassword()
	if err != nil {
		row.ProbeStatus = "error"
		row.ProbeError = err.Error()
		_ = s.db.WithContext(ctx).Model(row).Updates(map[string]interface{}{
			"probe_status": row.ProbeStatus, "probe_error": row.ProbeError, "probe_at": row.ProbeAt,
		}).Error
		return
	}
	if row.Host == DockerHubRegistryHost {
		// docker.io 无匿名 catalog 可枚举，也无凭据概念：连通性不做远程探测，
		// 状态恒为 public，镜像路径由前端预填 apache/seatunnel。
		// docker.io exposes no anonymous catalog and needs no credentials: skip the
		// remote probe entirely.
		row.ProbeStatus = "public"
		row.ProbeError = ""
		row.ProbeScheme = "https"
	} else {
		client := regclient.New(row.Host, row.Username, password)
		if err := client.Ping(ctx); err != nil {
			row.ProbeStatus = "error"
			row.ProbeError = err.Error()
		} else {
			row.ProbeScheme = client.Scheme()
			repos, err := client.Catalog(ctx)
			if err != nil {
				// ping 通但 catalog 失败（匿名仓库禁用枚举等）：连通但无缓存，
				// 前端退化为可输入 combobox。
				// Ping passed but catalog failed (e.g. anonymous catalog disabled):
				// reachable without a cache; the frontend degrades to a combobox.
				row.ProbeStatus = "reachable"
				row.ProbeError = err.Error()
				row.CatalogJSON = ""
			} else {
				row.ProbeStatus = "ok"
				row.ProbeError = ""
				payload, _ := json.Marshal(repos)
				row.CatalogJSON = string(payload)
			}
		}
	}
	_ = s.db.WithContext(ctx).Model(row).Updates(map[string]interface{}{
		"probe_status": row.ProbeStatus, "probe_error": row.ProbeError, "probe_at": row.ProbeAt,
		"probe_scheme": row.ProbeScheme, "catalog_json": row.CatalogJSON,
	}).Error
	row.afterLoad()
}

// ProbeRegistryTags 实时探测指定镜像的 tag 列表（不缓存，前端选择时调用）。
// ProbeRegistryTags fetches the tag list of one repo live (not cached).
func (s *Service) ProbeRegistryTags(ctx context.Context, id uint, repo string) ([]string, error) {
	var row ContainerRegistry
	if err := s.db.WithContext(ctx).First(&row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errRegistryNotFound
		}
		return nil, err
	}
	if row.Host == DockerHubRegistryHost {
		return fetchDockerHubTags(ctx, repo)
	}
	password, err := row.DecryptPassword()
	if err != nil {
		return nil, err
	}
	client := regclient.New(row.Host, row.Username, password)
	return client.Tags(ctx, strings.TrimSpace(repo))
}

// GetContainerRegistry 按 ID 读取仓库并回填派生字段。
// GetContainerRegistry loads one registry with derived fields populated.
func (s *Service) GetContainerRegistry(ctx context.Context, id uint) (*ContainerRegistry, error) {
	var row ContainerRegistry
	if err := s.db.WithContext(ctx).First(&row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errRegistryNotFound
		}
		return nil, err
	}
	row.afterLoad()
	return &row, nil
}
