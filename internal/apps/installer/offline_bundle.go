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
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/LeonYoah/stx/internal/config"
	"github.com/LeonYoah/stx/internal/logger"
)

const (
	// OfflineBundleFormatV1 是 SeaTunnel 离线资产包格式标识。
	// OfflineBundleFormatV1 is the SeaTunnel offline asset bundle format id.
	OfflineBundleFormatV1 = "stx-seatunnel-offline-bundle/v1"
)

var (
	// ErrOfflineBundleNotFound 表示离线包不存在。
	// ErrOfflineBundleNotFound means the offline bundle was not found.
	ErrOfflineBundleNotFound = errors.New("offline bundle not found / 离线资产包未找到")
	// ErrOfflineBundleInvalid 表示离线包无效或不完整。
	// ErrOfflineBundleInvalid means the offline bundle is invalid or incomplete.
	ErrOfflineBundleInvalid = errors.New("offline bundle invalid / 离线资产包无效")
	// ErrOfflineBundlePackageMissing 表示本地缺少目标版本安装包。
	// ErrOfflineBundlePackageMissing means the local SeaTunnel package is missing.
	ErrOfflineBundlePackageMissing = errors.New("local seatunnel package missing / 本地安装包缺失，请先下载或上传")
)

// OfflineBundleCreateRequest 创建离线资产包请求。
// OfflineBundleCreateRequest is the create request for an offline asset bundle.
type OfflineBundleCreateRequest struct {
	Version        string `json:"version" binding:"required"`
	IncludePlugins *bool  `json:"include_plugins"`
	IncludeSource  bool   `json:"include_source"`
}

// OfflineBundleManifest 描述离线资产包内容。
// OfflineBundleManifest describes offline asset bundle contents.
type OfflineBundleManifest struct {
	Format            string                     `json:"format"`
	SeatunnelVersion  string                     `json:"seatunnel_version"`
	CreatedAt         time.Time                  `json:"created_at"`
	PackageFileName   string                     `json:"package_file_name"`
	PackageSize       int64                      `json:"package_size"`
	PackageSHA256     string                     `json:"package_sha256,omitempty"`
	SourceFileName    string                     `json:"source_file_name,omitempty"`
	SourceSize        int64                      `json:"source_size,omitempty"`
	IncludePlugins    bool                       `json:"include_plugins"`
	PluginCount       int                        `json:"plugin_count"`
	PluginFiles       []OfflineBundlePluginEntry `json:"plugin_files,omitempty"`
	Notes             string                     `json:"notes,omitempty"`
}

// OfflineBundlePluginEntry 记录打包进离线包的插件文件。
// OfflineBundlePluginEntry records a plugin file packed into the bundle.
type OfflineBundlePluginEntry struct {
	RelativePath string `json:"relative_path"`
	Size         int64  `json:"size"`
}

// OfflineBundleInfo 是已生成离线包的列表/创建响应。
// OfflineBundleInfo is the list/create response for a generated offline bundle.
type OfflineBundleInfo struct {
	Name             string                 `json:"name"`
	FileName         string                 `json:"file_name"`
	SeatunnelVersion string                 `json:"seatunnel_version"`
	FileSize         int64                  `json:"file_size"`
	CreatedAt        time.Time              `json:"created_at"`
	IncludePlugins   bool                   `json:"include_plugins"`
	PluginCount      int                    `json:"plugin_count"`
	DownloadPath     string                 `json:"download_path"`
	Manifest         *OfflineBundleManifest `json:"manifest,omitempty"`
}

func (s *Service) offlineBundlesDir() string {
	return filepath.Join(filepath.Dir(s.packageDir), "offline-bundles")
}

func (s *Service) pluginsDir() string {
	// 优先使用与 packages 同级的 plugins 目录（测试与默认 ./lib/{packages,plugins} 布局一致）。
	// Prefer plugins sibling of packages (matches tests and default ./lib/{packages,plugins} layout).
	sibling := filepath.Join(filepath.Dir(s.packageDir), "plugins")
	if info, err := os.Stat(sibling); err == nil && info.IsDir() {
		return sibling
	}
	return config.GetPluginsDir()
}

func offlineBundleFileName(version string) string {
	return fmt.Sprintf("stx-seatunnel-offline-%s.tar.gz", strings.TrimSpace(version))
}

func offlineBundleRootName(version string) string {
	return fmt.Sprintf("stx-seatunnel-offline-%s", strings.TrimSpace(version))
}

// CreateOfflineBundle 从本地仓组装指定版本的离线资产包（安装包 + 可选插件目录）。
// CreateOfflineBundle assembles an offline asset bundle from local package (+ optional plugins dir).
func (s *Service) CreateOfflineBundle(ctx context.Context, req *OfflineBundleCreateRequest) (*OfflineBundleInfo, error) {
	if req == nil {
		return nil, ErrInvalidPackageVersion
	}
	version := strings.TrimSpace(req.Version)
	if version == "" {
		return nil, ErrInvalidPackageVersion
	}
	includePlugins := true
	if req.IncludePlugins != nil {
		includePlugins = *req.IncludePlugins
	}

	pkgName := packageFileName(version)
	pkgPath := filepath.Join(s.packageDir, pkgName)
	pkgStat, err := os.Stat(pkgPath)
	if err != nil || !pkgStat.Mode().IsRegular() {
		return nil, ErrOfflineBundlePackageMissing
	}

	bundlesDir := s.offlineBundlesDir()
	if err := os.MkdirAll(bundlesDir, 0o755); err != nil {
		return nil, fmt.Errorf("create offline bundles dir: %w", err)
	}

	stagingRoot, err := os.MkdirTemp(config.GetTempDir(), "stx-offline-bundle-*")
	if err != nil {
		return nil, fmt.Errorf("create staging dir: %w", err)
	}
	defer os.RemoveAll(stagingRoot)

	rootName := offlineBundleRootName(version)
	bundleRoot := filepath.Join(stagingRoot, rootName)
	packagesOut := filepath.Join(bundleRoot, "packages")
	if err := os.MkdirAll(packagesOut, 0o755); err != nil {
		return nil, err
	}
	if err := copyFile(pkgPath, filepath.Join(packagesOut, pkgName)); err != nil {
		return nil, fmt.Errorf("copy package: %w", err)
	}

	checksum, _ := calculateChecksum(pkgPath)
	manifest := &OfflineBundleManifest{
		Format:           OfflineBundleFormatV1,
		SeatunnelVersion: version,
		CreatedAt:        time.Now().UTC(),
		PackageFileName:  pkgName,
		PackageSize:      pkgStat.Size(),
		PackageSHA256:    checksum,
		IncludePlugins:   includePlugins,
		Notes:            "Unpack into STX packages_dir and plugins_dir, or use import API. / 解压至 packages_dir 与 plugins_dir，或使用 import 接口。",
	}

	if req.IncludeSource {
		srcName := sourcePackageFileName(version)
		srcPath := filepath.Join(s.packageDir, srcName)
		if srcStat, srcErr := os.Stat(srcPath); srcErr == nil && srcStat.Mode().IsRegular() {
			if err := copyFile(srcPath, filepath.Join(packagesOut, srcName)); err != nil {
				return nil, fmt.Errorf("copy source package: %w", err)
			}
			manifest.SourceFileName = srcName
			manifest.SourceSize = srcStat.Size()
		}
	}

	if includePlugins {
		pluginSrc := filepath.Join(s.pluginsDir(), version)
		pluginDst := filepath.Join(bundleRoot, "plugins", version)
		entries, copyErr := copyDirCollect(pluginSrc, pluginDst)
		if copyErr != nil && !errors.Is(copyErr, os.ErrNotExist) {
			return nil, fmt.Errorf("copy plugins: %w", copyErr)
		}
		manifest.PluginFiles = entries
		manifest.PluginCount = len(entries)
	}

	manifestPath := filepath.Join(bundleRoot, "MANIFEST.json")
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(manifestPath, manifestBytes, 0o644); err != nil {
		return nil, err
	}

	outName := offlineBundleFileName(version)
	outPath := filepath.Join(bundlesDir, outName)
	tmpOut := outPath + ".tmp"
	_ = os.Remove(tmpOut)
	if err := createTarGz(tmpOut, stagingRoot, rootName); err != nil {
		_ = os.Remove(tmpOut)
		return nil, fmt.Errorf("create tar.gz: %w", err)
	}
	if err := os.Rename(tmpOut, outPath); err != nil {
		_ = os.Remove(tmpOut)
		return nil, err
	}

	info, err := s.statOfflineBundle(outPath, manifest)
	if err != nil {
		return nil, err
	}
	logger.InfoF(ctx, "[Installer] 已创建离线资产包 %s (%d bytes)", outName, info.FileSize)
	return info, nil
}

// ListOfflineBundles 列出已生成的离线资产包。
// ListOfflineBundles lists generated offline asset bundles.
func (s *Service) ListOfflineBundles(ctx context.Context) ([]OfflineBundleInfo, error) {
	_ = ctx
	dir := s.offlineBundlesDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []OfflineBundleInfo{}, nil
		}
		return nil, err
	}
	out := make([]OfflineBundleInfo, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".tar.gz") {
			continue
		}
		info, err := s.statOfflineBundle(filepath.Join(dir, entry.Name()), nil)
		if err != nil {
			continue
		}
		out = append(out, *info)
	}
	return out, nil
}

// ResolveOfflineBundlePath 解析可下载的离线包本地路径。
// ResolveOfflineBundlePath resolves a downloadable offline bundle local path.
func (s *Service) ResolveOfflineBundlePath(name string) (string, error) {
	clean := filepath.Base(strings.TrimSpace(name))
	if clean == "" || clean == "." || clean == ".." {
		return "", ErrOfflineBundleNotFound
	}
	if !strings.HasSuffix(clean, ".tar.gz") {
		clean = clean + ".tar.gz"
	}
	path := filepath.Join(s.offlineBundlesDir(), clean)
	if _, err := os.Stat(path); err != nil {
		return "", ErrOfflineBundleNotFound
	}
	return path, nil
}

// DeleteOfflineBundle 删除已生成的离线资产包。
// DeleteOfflineBundle deletes a generated offline asset bundle.
func (s *Service) DeleteOfflineBundle(ctx context.Context, name string) error {
	_ = ctx
	path, err := s.ResolveOfflineBundlePath(name)
	if err != nil {
		return err
	}
	return os.Remove(path)
}

// ImportOfflineBundle 将离线资产包导入本地 packages_dir / plugins_dir。
// ImportOfflineBundle imports an offline asset bundle into local packages_dir / plugins_dir.
func (s *Service) ImportOfflineBundle(ctx context.Context, srcPath string) (*OfflineBundleInfo, error) {
	if strings.TrimSpace(srcPath) == "" {
		return nil, ErrOfflineBundleInvalid
	}
	staging, err := os.MkdirTemp(config.GetTempDir(), "stx-offline-import-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(staging)

	if err := extractTarGz(srcPath, staging); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrOfflineBundleInvalid, err)
	}

	root, manifest, err := findOfflineBundleRoot(staging)
	if err != nil {
		return nil, err
	}

	packagesSrc := filepath.Join(root, "packages")
	if err := os.MkdirAll(s.packageDir, 0o755); err != nil {
		return nil, err
	}
	if err := copyDirOverwrite(packagesSrc, s.packageDir); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("import packages: %w", err)
	}

	pluginsSrc := filepath.Join(root, "plugins")
	pluginsDst := s.pluginsDir()
	if err := os.MkdirAll(pluginsDst, 0o755); err != nil {
		return nil, err
	}
	if err := copyDirOverwrite(pluginsSrc, pluginsDst); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("import plugins: %w", err)
	}

	// 导入后把包归档到 offline-bundles，便于再次下载。
	// Archive imported file into offline-bundles for re-download.
	bundlesDir := s.offlineBundlesDir()
	_ = os.MkdirAll(bundlesDir, 0o755)
	outName := offlineBundleFileName(manifest.SeatunnelVersion)
	outPath := filepath.Join(bundlesDir, outName)
	if err := copyFile(srcPath, outPath); err != nil {
		logger.WarnF(ctx, "[Installer] 导入成功但归档离线包失败: %v", err)
	}

	info, err := s.statOfflineBundle(outPath, manifest)
	if err != nil {
		// 仍返回 manifest 摘要 / Still return manifest summary
		return &OfflineBundleInfo{
			Name:             strings.TrimSuffix(outName, ".tar.gz"),
			FileName:         outName,
			SeatunnelVersion: manifest.SeatunnelVersion,
			CreatedAt:        manifest.CreatedAt,
			IncludePlugins:   manifest.IncludePlugins,
			PluginCount:      manifest.PluginCount,
			DownloadPath:     "/api/v1/packages/offline-bundles/" + outName + "/download",
			Manifest:         manifest,
		}, nil
	}
	logger.InfoF(ctx, "[Installer] 已导入离线资产包 version=%s plugins=%d", manifest.SeatunnelVersion, manifest.PluginCount)
	return info, nil
}

func (s *Service) statOfflineBundle(path string, known *OfflineBundleManifest) (*OfflineBundleInfo, error) {
	stat, err := os.Stat(path)
	if err != nil {
		return nil, ErrOfflineBundleNotFound
	}
	fileName := filepath.Base(path)
	name := strings.TrimSuffix(fileName, ".tar.gz")
	info := &OfflineBundleInfo{
		Name:         name,
		FileName:     fileName,
		FileSize:     stat.Size(),
		CreatedAt:    stat.ModTime().UTC(),
		DownloadPath: "/api/v1/packages/offline-bundles/" + fileName + "/download",
	}
	manifest := known
	if manifest == nil {
		manifest, _ = peekOfflineBundleManifest(path)
	}
	if manifest != nil {
		info.SeatunnelVersion = manifest.SeatunnelVersion
		info.IncludePlugins = manifest.IncludePlugins
		info.PluginCount = manifest.PluginCount
		info.CreatedAt = manifest.CreatedAt
		info.Manifest = manifest
	} else if strings.HasPrefix(name, "stx-seatunnel-offline-") {
		info.SeatunnelVersion = strings.TrimPrefix(name, "stx-seatunnel-offline-")
	}
	return info, nil
}

func peekOfflineBundleManifest(tarGzPath string) (*OfflineBundleManifest, error) {
	f, err := os.Open(tarGzPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		base := filepath.Base(hdr.Name)
		if base != "MANIFEST.json" || hdr.FileInfo().IsDir() {
			continue
		}
		data, err := io.ReadAll(io.LimitReader(tr, 4<<20))
		if err != nil {
			return nil, err
		}
		var manifest OfflineBundleManifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			return nil, err
		}
		return &manifest, nil
	}
	return nil, ErrOfflineBundleInvalid
}

func findOfflineBundleRoot(staging string) (string, *OfflineBundleManifest, error) {
	var foundRoot string
	var foundManifest *OfflineBundleManifest
	_ = filepath.WalkDir(staging, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if filepath.Base(path) != "MANIFEST.json" {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		var manifest OfflineBundleManifest
		if json.Unmarshal(data, &manifest) != nil {
			return nil
		}
		if manifest.Format != OfflineBundleFormatV1 && !strings.HasPrefix(manifest.Format, "stx-seatunnel-offline-bundle/") {
			return nil
		}
		foundRoot = filepath.Dir(path)
		foundManifest = &manifest
		return filepath.SkipAll
	})
	if foundRoot == "" || foundManifest == nil {
		return "", nil, ErrOfflineBundleInvalid
	}
	return foundRoot, foundManifest, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func copyDirCollect(src, dst string) ([]OfflineBundlePluginEntry, error) {
	entries := make([]OfflineBundlePluginEntry, 0)
	info, err := os.Stat(src)
	if err != nil {
		return entries, err
	}
	if !info.IsDir() {
		return entries, fmt.Errorf("not a directory: %s", src)
	}
	err = filepath.WalkDir(src, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, relErr := filepath.Rel(src, path)
		if relErr != nil {
			return relErr
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		fi, statErr := d.Info()
		if statErr != nil {
			return statErr
		}
		if err := copyFile(path, target); err != nil {
			return err
		}
		entries = append(entries, OfflineBundlePluginEntry{
			RelativePath: filepath.ToSlash(filepath.Join(filepath.Base(src), rel)),
			Size:         fi.Size(),
		})
		return nil
	})
	return entries, err
}

func copyDirOverwrite(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("not a directory: %s", src)
	}
	return filepath.WalkDir(src, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, relErr := filepath.Rel(src, path)
		if relErr != nil {
			return relErr
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target)
	})
}

func createTarGz(outPath, parentDir, rootName string) error {
	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer out.Close()
	gz := gzip.NewWriter(out)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()

	rootPath := filepath.Join(parentDir, rootName)
	return filepath.WalkDir(rootPath, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, relErr := filepath.Rel(parentDir, path)
		if relErr != nil {
			return relErr
		}
		info, infoErr := d.Info()
		if infoErr != nil {
			return infoErr
		}
		hdr, hdrErr := tar.FileInfoHeader(info, "")
		if hdrErr != nil {
			return hdrErr
		}
		hdr.Name = filepath.ToSlash(rel)
		if d.IsDir() {
			hdr.Name += "/"
			return tw.WriteHeader(hdr)
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		f, openErr := os.Open(path)
		if openErr != nil {
			return openErr
		}
		_, copyErr := io.Copy(tw, f)
		_ = f.Close()
		return copyErr
	})
}

func extractTarGz(srcPath, destDir string) error {
	f, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		cleanName := filepath.Clean(hdr.Name)
		if strings.HasPrefix(cleanName, "..") {
			return fmt.Errorf("invalid path in archive: %s", hdr.Name)
		}
		target := filepath.Join(destDir, cleanName)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode)&0o777)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				_ = out.Close()
				return err
			}
			if err := out.Close(); err != nil {
				return err
			}
		}
	}
}
