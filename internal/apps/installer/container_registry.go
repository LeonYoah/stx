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
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/LeonYoah/stx/internal/pkg/locale"
	"github.com/LeonYoah/stx/internal/pkg/regclient"
	"github.com/LeonYoah/stx/internal/utils/secretbox"
)

// DockerHubRegistryHost 官方公共仓库的登记 host（系统预置行，不可删除或改名）。
// DockerHubRegistryHost is the registered host of the official public registry
// (system-seeded row, immutable host and non-deletable).
const DockerHubRegistryHost = "docker.io"

// DockerHubOfficialRepo docker.io 上 SeaTunnel 官方镜像路径的预填值（上游事实，可编辑）。
// DockerHubOfficialRepo is the prefilled official image path on docker.io (an
// upstream fact, still editable).
const DockerHubOfficialRepo = "apache/seatunnel"

// ContainerRegistry 登记的镜像仓库：凭据每仓库一份，部署/升级按镜像 host 精确匹配；
// 可选配置 DefaultRepo 作为下游镜像选择器的默认镜像路径，探测仅校验连通性与认证，不做全仓 _catalog 扫库。
// ContainerRegistry is a registered image registry: one credential set per host,
// matched exactly by image host at deploy/upgrade time. Optional DefaultRepo prefills
// the downstream image picker; probing verifies reachability and auth without sweeping /v2/_catalog.
type ContainerRegistry struct {
	ID       uint   `json:"id" gorm:"primaryKey;autoIncrement"`
	Host     string `json:"host" gorm:"size:255;uniqueIndex;not null"`
	// DefaultRepo 可选默认镜像路径（如 apache/seatunnel 或 bigdata/seatunnel），供部署/升级自动预填并实时查 tag。
	// DefaultRepo is an optional default repository path (e.g. apache/seatunnel) prefilled in deploy/upgrade pickers.
	DefaultRepo string `json:"default_repo" gorm:"column:default_repo;size:255"`
	Username    string `json:"username" gorm:"size:255"`
	// 密码 secretbox 加密落库且永不随响应返回（json:"-"）。
	// The password is secretbox-encrypted at rest and never serialized.
	Password string `json:"-" gorm:"size:1024"`
	// IsSystem 预置行（docker.io）：host 不可改、不可删除；允许补充 Docker Hub 凭据以防匿名限流。
	// IsSystem marks seeded rows (docker.io): host is immutable and non-deletable,
	// while user credentials remain editable (e.g. to bypass anonymous rate limits).
	IsSystem     bool       `json:"is_system" gorm:"column:is_system"`
	CatalogJSON  string     `json:"-" gorm:"column:catalog_json;type:text"`
	CatalogRepos []string   `json:"catalog_repos" gorm:"-"`
	ProbeScheme  string     `json:"probe_scheme" gorm:"column:probe_scheme;size:8"`
	ProbeStatus  string     `json:"probe_status" gorm:"column:probe_status;size:16"`
	ProbeError   string     `json:"probe_error" gorm:"column:probe_error;size:512"`
	ProbeAt      *time.Time `json:"probe_at" gorm:"column:probe_at"`
	HasAuth      bool       `json:"has_auth" gorm:"-"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// TableName 固定表名 container_registries。
// TableName pins the table name.
func (ContainerRegistry) TableName() string { return "container_registries" }

// ContainerRegistryRequest 创建/更新仓库的请求体。
// ContainerRegistryRequest is the create/update payload.
type ContainerRegistryRequest struct {
	Host        string `json:"host"`
	DefaultRepo string `json:"default_repo"`
	Username    string `json:"username"`
	Password    string `json:"password"`
}

// ContainerRegistryProbeRequest 注册/编辑态不落库探测请求（编辑态传 id 且密码留空时自动复用库内密文）。
// ContainerRegistryProbeRequest is the transient probe payload; when ID is given with an
// empty password, the stored secret is reused so edit-drawer tests do not false-fail 401.
type ContainerRegistryProbeRequest struct {
	ID          uint   `json:"id"`
	Host        string `json:"host"`
	DefaultRepo string `json:"default_repo"`
	Username    string `json:"username"`
	Password    string `json:"password"`
}

// ContainerRegistryProbeResult 注册/编辑态测试连接返回值。
// ContainerRegistryProbeResult is the transient probe response payload.
type ContainerRegistryProbeResult struct {
	Status string `json:"status"`
	Scheme string `json:"scheme"`
	Error  string `json:"error,omitempty"`
}

// NormalizeRegistryHost 清洗仓库 host：剥离误带 scheme 与尾斜杠并统一小写。
// NormalizeRegistryHost strips a mistakenly carried scheme, trailing slashes, and lowercases.
func NormalizeRegistryHost(raw string) string {
	host := strings.TrimSpace(raw)
	for _, scheme := range []string{"https://", "http://"} {
		host = strings.TrimPrefix(host, scheme)
	}
	return strings.ToLower(strings.TrimRight(host, "/"))
}

// NormalizeDefaultRepo 清洗默认镜像路径：去掉首尾空白与斜杠。
// NormalizeDefaultRepo trims whitespace and leading/trailing slashes from a default repository path.
func NormalizeDefaultRepo(raw string) string {
	return strings.Trim(strings.TrimSpace(raw), "/")
}

// DockerConfigAuthKey 返回写入 .dockerconfigjson auths 映射的权威键值：
// kubelet/containerd 对 Docker Hub 识别 https://index.docker.io/v1/，其余仓库用裸 host[:port]。
// DockerConfigAuthKey returns the canonical .dockerconfigjson auths key: kubelet/containerd
// expect https://index.docker.io/v1/ for Docker Hub, and bare host[:port] for private registries.
func DockerConfigAuthKey(host string) string {
	normalized := NormalizeRegistryHost(host)
	if normalized == DockerHubRegistryHost || normalized == "index.docker.io" || normalized == "registry-1.docker.io" {
		return "https://index.docker.io/v1/"
	}
	return normalized
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
	if r.CatalogRepos == nil {
		r.CatalogRepos = []string{}
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
	if strings.ContainsAny(first, ".:") || strings.EqualFold(first, "localhost") {
		return strings.ToLower(first)
	}
	return DockerHubRegistryHost
}

var (
	errRegistryNotFound        = errors.New("installer: container registry not found")
	errRegistryAlreadyExists   = errors.New("installer: container registry already exists")
	errRegistryValidation      = errors.New("installer: invalid container registry request")
	errSystemRegistryProtected = errors.New("installer: system container registry is protected")
)

// ErrRegistryNotFound 暴露仓库不存在哨兵错误（HTTP 404）。
// ErrRegistryNotFound exposes the not-found sentinel error (HTTP 404).
func ErrRegistryNotFound() error { return errRegistryNotFound }

// ErrRegistryAlreadyExists 暴露仓库 host 冲突哨兵错误（HTTP 409）。
// ErrRegistryAlreadyExists exposes the duplicate-host sentinel error (HTTP 409).
func ErrRegistryAlreadyExists() error { return errRegistryAlreadyExists }

// ErrRegistryValidation 暴露参数校验失败哨兵错误（HTTP 400）。
// ErrRegistryValidation exposes the validation sentinel error (HTTP 400).
func ErrRegistryValidation() error { return errRegistryValidation }

// ErrSystemRegistryProtected 暴露系统行保护哨兵错误（HTTP 400）。
// ErrSystemRegistryProtected exposes the system-row protection sentinel error (HTTP 400).
func ErrSystemRegistryProtected() error { return errSystemRegistryProtected }

// validateRegistryRequest 校验仓库请求并清洗 host 与默认镜像路径。
// validateRegistryRequest validates the payload and normalizes host and default_repo.
func validateRegistryRequest(ctx context.Context, req *ContainerRegistryRequest, requirePassword bool) error {
	req.Host = NormalizeRegistryHost(req.Host)
	req.DefaultRepo = NormalizeDefaultRepo(req.DefaultRepo)
	req.Username = strings.TrimSpace(req.Username)
	if req.Host == "" {
		return fmt.Errorf("%w: %s", errRegistryValidation, locale.Pick(ctx, "仓库地址不能为空", "registry host is required"))
	}
	if strings.ContainsAny(req.Host, "/ \t\r\n") {
		return fmt.Errorf("%w: %s", errRegistryValidation, locale.Pick(ctx, "仓库地址不能包含路径或空白字符（仅填写 host[:port]）", "registry host must be a bare host[:port] without paths or whitespace"))
	}
	if req.DefaultRepo != "" {
		if strings.Contains(req.DefaultRepo, "://") || strings.ContainsAny(req.DefaultRepo, ":@ \t\r\n") {
			return fmt.Errorf("%w: %s", errRegistryValidation, locale.Pick(ctx, "默认镜像名仅填写仓库路径（如 apache/seatunnel），不要包含协议、Tag 或空白字符", "default repository must be a repository path only (e.g. apache/seatunnel) without scheme, tag, or whitespace"))
		}
	}
	if req.Username == "" && strings.TrimSpace(req.Password) != "" {
		return fmt.Errorf("%w: %s", errRegistryValidation, locale.Pick(ctx, "填写密码时必须同时填写用户名", "username is required when a password is provided"))
	}
	if requirePassword && req.Username != "" && strings.TrimSpace(req.Password) == "" {
		return fmt.Errorf("%w: %s", errRegistryValidation, locale.Pick(ctx, "填写用户名时必须提供密码", "password is required when a username is provided"))
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
	if err := validateRegistryRequest(ctx, req, true); err != nil {
		return nil, err
	}
	var count int64
	if err := s.db.WithContext(ctx).Model(&ContainerRegistry{}).Where("host = ?", req.Host).Count(&count).Error; err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, fmt.Errorf("%w: %s", errRegistryAlreadyExists, fmt.Sprintf(locale.Pick(ctx, "镜像仓库 %s 已存在", "registry %s already exists"), req.Host))
	}
	row := &ContainerRegistry{
		Host:        req.Host,
		DefaultRepo: req.DefaultRepo,
		Username:    req.Username,
	}
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

// UpdateContainerRegistry 更新仓库：
// 1) 系统行（docker.io）锁定 host 不可改，仅允许补充/清除 Docker Hub 凭据；
// 2) 已有凭据时密码留空保持原值；匿名仓库填写用户名时必须同时提供密码。
// UpdateContainerRegistry updates a registry: system row locks its host while permitting
// credentials; empty password preserves existing ciphertext, but switching from anonymous
// to authenticated requires a non-empty password.
func (s *Service) UpdateContainerRegistry(ctx context.Context, id uint, req *ContainerRegistryRequest) (*ContainerRegistry, error) {
	var row ContainerRegistry
	if err := s.db.WithContext(ctx).First(&row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: %s", errRegistryNotFound, locale.Pick(ctx, "镜像仓库不存在", "container registry not found"))
		}
		return nil, err
	}
	if err := validateRegistryRequest(ctx, req, false); err != nil {
		return nil, err
	}
	if row.IsSystem && req.Host != row.Host {
		return nil, fmt.Errorf("%w: %s", errSystemRegistryProtected, locale.Pick(ctx, "系统预置仓库地址不可修改", "system registry host cannot be modified"))
	}
	if req.Host != row.Host {
		var count int64
		if err := s.db.WithContext(ctx).Model(&ContainerRegistry{}).Where("host = ? AND id != ?", req.Host, row.ID).Count(&count).Error; err != nil {
			return nil, err
		}
		if count > 0 {
			return nil, fmt.Errorf("%w: %s", errRegistryAlreadyExists, fmt.Sprintf(locale.Pick(ctx, "镜像仓库 %s 已存在", "registry %s already exists"), req.Host))
		}
	}
	if req.Username != "" && strings.TrimSpace(req.Password) == "" && strings.TrimSpace(row.Password) == "" {
		return nil, fmt.Errorf("%w: %s", errRegistryValidation, locale.Pick(ctx, "首次配置用户名时必须提供密码", "password is required when enabling authentication"))
	}
	row.Host = req.Host
	row.DefaultRepo = req.DefaultRepo
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
			return fmt.Errorf("%w: %s", errRegistryNotFound, locale.Pick(ctx, "镜像仓库不存在", "container registry not found"))
		}
		return err
	}
	if row.IsSystem {
		return fmt.Errorf("%w: %s", errSystemRegistryProtected, locale.Pick(ctx, "系统预置仓库不可删除", "system registry cannot be deleted"))
	}
	return s.db.WithContext(ctx).Delete(&row).Error
}

// formatRegistryProbeError 将底层 regclient 鉴权错误转为可读文案。
// formatRegistryProbeError formats regclient auth errors into localized user-facing messages.
func formatRegistryProbeError(ctx context.Context, err error) string {
	switch {
	case errors.Is(err, regclient.ErrAuthRequired):
		return locale.Pick(ctx, "仓库要求认证，请填写用户名和密码", "registry requires authentication; please provide username and password")
	case errors.Is(err, regclient.ErrAuthFailed):
		return locale.Pick(ctx, "认证失败，请检查用户名和密码", "authentication failed; please check username and password")
	default:
		return err.Error()
	}
}

// ProbeRegistryTransient 注册/编辑前不落库探测：
// 当传入 ID 且密码留空时自动回填库内已有密码，避免编辑态测试误报 401；
// 仅校验 /v2/ 连通性与授权有效性，不扫全仓 /v2/_catalog。
// ProbeRegistryTransient probes without persisting; when ID is supplied with an empty
// password, stored credentials are decrypted and used so edit-mode tests work seamlessly.
// It verifies /v2/ connectivity and authorization without sweeping /v2/_catalog.
func (s *Service) ProbeRegistryTransient(ctx context.Context, req *ContainerRegistryProbeRequest) (*ContainerRegistryProbeResult, error) {
	if req == nil {
		return nil, fmt.Errorf("%w: %s", errRegistryValidation, locale.Pick(ctx, "请求体不能为空", "request body is required"))
	}
	baseReq := &ContainerRegistryRequest{
		Host:        req.Host,
		DefaultRepo: req.DefaultRepo,
		Username:    req.Username,
		Password:    req.Password,
	}
	if err := validateRegistryRequest(ctx, baseReq, false); err != nil {
		return nil, err
	}
	password := strings.TrimSpace(baseReq.Password)
	if req.ID > 0 && baseReq.Username != "" && password == "" {
		var existing ContainerRegistry
		if err := s.db.WithContext(ctx).First(&existing, req.ID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, fmt.Errorf("%w: %s", errRegistryNotFound, locale.Pick(ctx, "镜像仓库不存在", "container registry not found"))
			}
			return nil, err
		}
		decrypted, decErr := existing.DecryptPassword()
		if decErr != nil {
			return nil, decErr
		}
		if decrypted == "" {
			return nil, fmt.Errorf("%w: %s", errRegistryValidation, locale.Pick(ctx, "请填写仓库密码后再测试连接", "password is required before testing authentication"))
		}
		password = decrypted
	} else if baseReq.Username != "" && password == "" {
		return nil, fmt.Errorf("%w: %s", errRegistryValidation, locale.Pick(ctx, "填写用户名时必须提供密码", "password is required when a username is provided"))
	}

	normalized := baseReq.Host
	if normalized == DockerHubRegistryHost && baseReq.Username == "" {
		return &ContainerRegistryProbeResult{Status: "public", Scheme: "https"}, nil
	}
	client := regclient.New(normalized, baseReq.Username, password)
	if err := client.PingStrict(ctx); err != nil {
		return &ContainerRegistryProbeResult{
			Status: "error",
			Scheme: client.Scheme(),
			Error:  formatRegistryProbeError(ctx, err),
		}, nil
	}
	return &ContainerRegistryProbeResult{Status: "ok", Scheme: client.Scheme()}, nil
}

// ProbeContainerRegistry 探测仓库连通性与授权状态（同步执行，10s 上限），不再全量扫 /v2/_catalog。
// ProbeContainerRegistry probes connectivity and authorization synchronously without sweeping /v2/_catalog.
func (s *Service) ProbeContainerRegistry(ctx context.Context, row *ContainerRegistry) {
	now := time.Now()
	row.ProbeAt = &now
	row.CatalogJSON = ""
	password, err := row.DecryptPassword()
	if err != nil {
		row.ProbeStatus = "error"
		row.ProbeError = err.Error()
		_ = s.db.WithContext(ctx).Model(row).Updates(map[string]interface{}{
			"probe_status": row.ProbeStatus, "probe_error": row.ProbeError, "probe_at": row.ProbeAt,
			"catalog_json": "",
		}).Error
		row.afterLoad()
		return
	}
	if row.Host == DockerHubRegistryHost && strings.TrimSpace(row.Username) == "" {
		// 未配置凭据的 docker.io 保持 public 公开态。
		// Anonymous docker.io stays in public state without probing.
		row.ProbeStatus = "public"
		row.ProbeError = ""
		row.ProbeScheme = "https"
	} else {
		client := regclient.New(row.Host, row.Username, password)
		if err := client.PingStrict(ctx); err != nil {
			row.ProbeStatus = "error"
			row.ProbeError = formatRegistryProbeError(ctx, err)
			row.ProbeScheme = client.Scheme()
		} else {
			row.ProbeStatus = "ok"
			row.ProbeError = ""
			row.ProbeScheme = client.Scheme()
		}
	}
	_ = s.db.WithContext(ctx).Model(row).Updates(map[string]interface{}{
		"probe_status": row.ProbeStatus, "probe_error": row.ProbeError, "probe_at": row.ProbeAt,
		"probe_scheme": row.ProbeScheme, "catalog_json": "",
	}).Error
	row.afterLoad()
}

// ProbeRegistryTags 实时探测指定镜像的 tag 列表（不缓存，按语义版本倒序返回）。
// ProbeRegistryTags fetches the tag list of one repo live and returns tags sorted with
// semantic versions descending first.
func (s *Service) ProbeRegistryTags(ctx context.Context, id uint, repo string) ([]string, error) {
	var row ContainerRegistry
	if err := s.db.WithContext(ctx).First(&row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: %s", errRegistryNotFound, locale.Pick(ctx, "镜像仓库不存在", "container registry not found"))
		}
		return nil, err
	}
	cleanRepo := strings.Trim(strings.TrimSpace(repo), "/")
	if cleanRepo == "" {
		return nil, fmt.Errorf("%w: %s", errRegistryValidation, locale.Pick(ctx, "镜像名不能为空", "repository name is required"))
	}
	password, err := row.DecryptPassword()
	if err != nil {
		return nil, err
	}
	if row.Host == DockerHubRegistryHost {
		if row.HasCredentials() {
			client := regclient.New(row.Host, row.Username, password)
			tags, tagErr := client.Tags(ctx, cleanRepo)
			if tagErr == nil {
				return sortImageTags(tags), nil
			}
		}
		// 匿名或 OCI tags 接口受限时回退 Docker Hub 公开 tags 接口。
		// Fall back to the Docker Hub public tags API when anonymous or OCI tags fail.
		tags, hubErr := fetchDockerHubTags(ctx, cleanRepo)
		if hubErr != nil {
			return nil, hubErr
		}
		return sortImageTags(tags), nil
	}
	client := regclient.New(row.Host, row.Username, password)
	tags, err := client.Tags(ctx, cleanRepo)
	if err != nil {
		return nil, err
	}
	return sortImageTags(tags), nil
}

// GetContainerRegistry 按 ID 读取仓库并回填派生字段。
// GetContainerRegistry loads one registry with derived fields populated.
func (s *Service) GetContainerRegistry(ctx context.Context, id uint) (*ContainerRegistry, error) {
	var row ContainerRegistry
	if err := s.db.WithContext(ctx).First(&row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("%w: %s", errRegistryNotFound, locale.Pick(ctx, "镜像仓库不存在", "container registry not found"))
		}
		return nil, err
	}
	row.afterLoad()
	return &row, nil
}

var semverTagPattern = regexp.MustCompile(`^v?(\d+)\.(\d+)(?:\.(\d+))?(?:([-+].*))?$`)

type parsedSemverTag struct {
	raw     string
	valid   bool
	major   int
	minor   int
	patch   int
	hasPre  bool
}

func parseSemverTag(tag string) parsedSemverTag {
	m := semverTagPattern.FindStringSubmatch(strings.TrimSpace(tag))
	if len(m) == 0 {
		return parsedSemverTag{raw: tag}
	}
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	patch := 0
	if m[3] != "" {
		patch, _ = strconv.Atoi(m[3])
	}
	return parsedSemverTag{
		raw:    tag,
		valid:  true,
		major:  major,
		minor:  minor,
		patch:  patch,
		hasPre: m[4] != "",
	}
}

// sortImageTags 将语义版本 tag 按从新到旧排在最前（同版本纯数字优先于带后缀变体），
// 非语义 tag 排在后面按字典序排列。
// sortImageTags sorts semantic version tags descending first (clean release before
// suffixed variants), with non-semver tags ordered lexicographically afterward.
func sortImageTags(tags []string) []string {
	if len(tags) <= 1 {
		return tags
	}
	out := append([]string(nil), tags...)
	sort.SliceStable(out, func(i, j int) bool {
		a := parseSemverTag(out[i])
		b := parseSemverTag(out[j])
		if a.valid != b.valid {
			return a.valid
		}
		if !a.valid {
			return out[i] < out[j]
		}
		if a.major != b.major {
			return a.major > b.major
		}
		if a.minor != b.minor {
			return a.minor > b.minor
		}
		if a.patch != b.patch {
			return a.patch > b.patch
		}
		if a.hasPre != b.hasPre {
			return !a.hasPre
		}
		return out[i] < out[j]
	})
	return out
}
