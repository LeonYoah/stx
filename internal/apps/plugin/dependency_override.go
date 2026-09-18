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

package plugin

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/LeonYoah/stx/internal/config"
	"github.com/LeonYoah/stx/internal/seatunnel"
)

var uploadedJarNamePattern = regexp.MustCompile(`^(?P<artifact>.+)-(?P<version>\d[\w.\-+]*)\.jar$`)

// ListDependencies returns all user-added dependencies for a plugin/version.
// ListDependencies 返回插件指定版本下的所有用户新增依赖。
func (s *Service) ListDependencies(ctx context.Context, pluginName, seatunnelVersion string) ([]PluginDependencyConfig, error) {
	if strings.TrimSpace(seatunnelVersion) == "" {
		seatunnelVersion = seatunnel.DefaultVersion()
	}
	return s.repo.ListDependencies(ctx, pluginName, seatunnelVersion)
}

// AddDependency adds one Maven-based user dependency.
// AddDependency 添加一条基于 Maven 坐标的用户依赖。
func (s *Service) AddDependency(ctx context.Context, req *AddDependencyRequest) (*PluginDependencyConfig, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required / 请求不能为空")
	}
	version := strings.TrimSpace(req.SeatunnelVersion)
	if version == "" {
		version = seatunnel.DefaultVersion()
	}

	targetDir, err := s.resolveUserDependencyTargetDir(ctx, req.PluginName, version, strings.TrimSpace(req.TargetDir))
	if err != nil {
		return nil, err
	}

	dep := &PluginDependencyConfig{
		PluginName:       req.PluginName,
		SeatunnelVersion: version,
		GroupID:          strings.TrimSpace(req.GroupID),
		ArtifactID:       strings.TrimSpace(req.ArtifactID),
		Version:          strings.TrimSpace(req.Version),
		TargetDir:        targetDir,
		SourceType:       PluginDependencySourceMaven,
	}
	if err := s.repo.UpsertDependency(ctx, dep); err != nil {
		return nil, err
	}
	return s.repo.FindDependencyByNaturalKey(ctx, dep.PluginName, dep.SeatunnelVersion, dep.GroupID, dep.ArtifactID, dep.Version, dep.TargetDir, dep.SourceType)
}

// UploadDependency uploads one custom jar and registers it as a user-added dependency.
// UploadDependency 上传一个自定义 jar，并登记为用户新增依赖。
func (s *Service) UploadDependency(ctx context.Context, req *UploadDependencyRequest, file *multipart.FileHeader) (*PluginDependencyConfig, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required / 请求不能为空")
	}
	if file == nil {
		return nil, fmt.Errorf("file is required / 必须上传文件")
	}
	if !strings.HasSuffix(strings.ToLower(file.Filename), ".jar") {
		return nil, fmt.Errorf("only .jar files are supported / 仅支持上传 .jar 文件")
	}

	seatunnelVersion := strings.TrimSpace(req.SeatunnelVersion)
	if seatunnelVersion == "" {
		seatunnelVersion = seatunnel.DefaultVersion()
	}

	inferredArtifact, inferredVersion := inferUploadedJarCoordinates(file.Filename)
	artifactID := firstNonEmpty(req.ArtifactID, inferredArtifact)
	depVersion := firstNonEmpty(req.Version, inferredVersion)
	if strings.TrimSpace(artifactID) == "" {
		return nil, fmt.Errorf("artifact_id is required / artifact_id 不能为空")
	}
	if strings.TrimSpace(depVersion) == "" {
		return nil, fmt.Errorf("version is required / version 不能为空")
	}

	targetDir, err := s.resolveUserDependencyTargetDir(ctx, req.PluginName, seatunnelVersion, strings.TrimSpace(req.TargetDir))
	if err != nil {
		return nil, err
	}
	groupID := firstNonEmpty(req.GroupID, "uploaded")

	existing, err := s.repo.FindDependencyByNaturalKey(ctx, req.PluginName, seatunnelVersion, groupID, artifactID, depVersion, targetDir, PluginDependencySourceUpload)
	if err != nil {
		return nil, err
	}

	storedPath, checksum, fileSize, err := s.storeUploadedDependencyFile(req.PluginName, seatunnelVersion, artifactID, depVersion, file)
	if err != nil {
		return nil, err
	}

	dep := &PluginDependencyConfig{
		PluginName:       req.PluginName,
		SeatunnelVersion: seatunnelVersion,
		GroupID:          groupID,
		ArtifactID:       artifactID,
		Version:          depVersion,
		TargetDir:        targetDir,
		SourceType:       PluginDependencySourceUpload,
		OriginalFileName: file.Filename,
		StoredPath:       storedPath,
		FileSize:         fileSize,
		Checksum:         checksum,
	}
	if err := s.repo.UpsertDependency(ctx, dep); err != nil {
		_ = os.Remove(storedPath)
		return nil, err
	}
	if existing != nil && existing.StoredPath != "" && existing.StoredPath != storedPath {
		_ = os.Remove(existing.StoredPath)
	}
	return s.repo.FindDependencyByNaturalKey(ctx, dep.PluginName, dep.SeatunnelVersion, dep.GroupID, dep.ArtifactID, dep.Version, dep.TargetDir, dep.SourceType)
}

// DeleteDependency deletes one user-added dependency and removes uploaded file when needed.
// DeleteDependency 删除一条用户新增依赖，并在需要时清理上传文件。
func (s *Service) DeleteDependency(ctx context.Context, depID uint) error {
	dep, err := s.repo.GetDependencyByID(ctx, depID)
	if err != nil {
		return err
	}
	if err := s.repo.DeleteDependency(ctx, depID); err != nil {
		return err
	}
	if dep.SourceType == PluginDependencySourceUpload && strings.TrimSpace(dep.StoredPath) != "" {
		_ = os.Remove(dep.StoredPath)
	}
	return nil
}

// DisableDependency disables one official dependency item for a plugin/version.
// DisableDependency 禁用插件某个版本下的一条官方依赖。
func (s *Service) DisableDependency(ctx context.Context, req *DisableDependencyRequest) (*PluginDependencyDisable, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required / 请求不能为空")
	}
	seatunnelVersion := strings.TrimSpace(req.SeatunnelVersion)
	if seatunnelVersion == "" {
		seatunnelVersion = seatunnel.DefaultVersion()
	}
	targetDir, err := normalizePluginTargetDir(req.TargetDir)
	if err != nil {
		return nil, err
	}
	item := &PluginDependencyDisable{
		PluginName:       req.PluginName,
		SeatunnelVersion: seatunnelVersion,
		GroupID:          strings.TrimSpace(req.GroupID),
		ArtifactID:       strings.TrimSpace(req.ArtifactID),
		Version:          strings.TrimSpace(req.Version),
		TargetDir:        targetDir,
	}
	if err := s.repo.UpsertDependencyDisable(ctx, item); err != nil {
		return nil, err
	}
	return s.repo.FindDependencyDisableByNaturalKey(ctx, item.PluginName, item.SeatunnelVersion, item.GroupID, item.ArtifactID, item.Version, item.TargetDir)
}

// EnableDependency removes one disabled official dependency record.
// EnableDependency 重新启用一条被禁用的官方依赖。
func (s *Service) EnableDependency(ctx context.Context, disableID uint) error {
	return s.repo.DeleteDependencyDisable(ctx, disableID)
}

func (s *Service) resolveUserDependencyTargetDir(ctx context.Context, pluginName, seatunnelVersion, targetDir string) (string, error) {
	targetDir = strings.TrimSpace(targetDir)
	if targetDir != "" {
		normalized, err := normalizePluginTargetDir(targetDir)
		if err != nil {
			return "", err
		}
		if normalized == "connectors" {
			return "", fmt.Errorf("custom dependencies cannot target connectors / 自定义依赖不能放到 connectors 目录")
		}
		return normalized, nil
	}

	artifactID := getArtifactID(pluginName)
	if info, err := s.GetPluginInfo(ctx, pluginName, seatunnelVersion); err == nil && info != nil && strings.TrimSpace(info.ArtifactID) != "" {
		artifactID = info.ArtifactID
	}
	resolved := defaultPluginDependencyTargetDir(seatunnelVersion, artifactID)
	if resolved == "connectors" {
		return "", fmt.Errorf("custom dependencies cannot target connectors / 自定义依赖不能放到 connectors 目录")
	}
	return resolved, nil
}

func inferUploadedJarCoordinates(fileName string) (string, string) {
	name := strings.TrimSpace(filepath.Base(fileName))
	if strings.HasSuffix(strings.ToLower(name), ".jar") {
		name = name[:len(name)-4]
	}
	if matches := uploadedJarNamePattern.FindStringSubmatch(filepath.Base(fileName)); len(matches) == 3 {
		return strings.TrimSpace(matches[1]), strings.TrimSpace(matches[2])
	}
	return strings.TrimSpace(name), ""
}

func (s *Service) storeUploadedDependencyFile(pluginName, seatunnelVersion, artifactID, depVersion string, file *multipart.FileHeader) (string, string, int64, error) {
	src, err := file.Open()
	if err != nil {
		return "", "", 0, fmt.Errorf("failed to open uploaded jar: %w", err)
	}
	defer src.Close()

	baseDir := filepath.Join(config.GetPluginsDir(), "_uploaded_dependencies", pluginName, seatunnelVersion)
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return "", "", 0, fmt.Errorf("failed to create upload directory: %w", err)
	}

	storedFileName := fmt.Sprintf("%s-%s-%d.jar", sanitizePathSegment(artifactID), sanitizePathSegment(depVersion), time.Now().UnixNano())
	targetPath := filepath.Join(baseDir, storedFileName)
	dst, err := os.Create(targetPath)
	if err != nil {
		return "", "", 0, fmt.Errorf("failed to create uploaded jar: %w", err)
	}
	defer dst.Close()

	hasher := sha1.New()
	written, err := io.Copy(io.MultiWriter(dst, hasher), src)
	if err != nil {
		_ = os.Remove(targetPath)
		return "", "", 0, fmt.Errorf("failed to save uploaded jar: %w", err)
	}

	return targetPath, hex.EncodeToString(hasher.Sum(nil)), written, nil
}

func sanitizePathSegment(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	replacer := strings.NewReplacer("/", "_", "\\", "_", "..", "_", " ", "_", ":", "_")
	return replacer.Replace(value)
}
