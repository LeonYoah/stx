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

package stupgrade

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	pathpkg "path"
	"strings"
)

func readTargetConfigContentsFromPackage(packagePath, targetVersion string, targetPaths []string) (map[string]string, []BlockingIssue) {
	normalizedTargets := dedupeSortedStrings(targetPaths)
	result := make(map[string]string, len(normalizedTargets))
	if strings.TrimSpace(packagePath) == "" || len(normalizedTargets) == 0 {
		return result, nil
	}

	file, err := os.Open(packagePath)
	if err != nil {
		return result, []BlockingIssue{blockingIssue(
			CheckCategoryPackage,
			"package_read_failed",
			fmt.Sprintf("cannot read target package %s for config merge", targetVersion),
			map[string]string{
				"package_path": packagePath,
				"version":      targetVersion,
			},
		)}
	}
	defer file.Close()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return result, []BlockingIssue{blockingIssue(
			CheckCategoryPackage,
			"package_invalid_archive",
			fmt.Sprintf("target package %s is not a valid tar.gz archive / 目标版本 %s 的安装包不是有效的 tar.gz 归档", targetVersion, targetVersion),
			map[string]string{
				"package_path": packagePath,
				"version":      targetVersion,
			},
		)}
	}
	defer gzipReader.Close()

	reader := tar.NewReader(gzipReader)
	wanted := make(map[string]struct{}, len(normalizedTargets))
	for _, targetPath := range normalizedTargets {
		wanted[normalizeConfigArchivePath(targetPath)] = struct{}{}
	}

	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return result, []BlockingIssue{blockingIssue(
				CheckCategoryPackage,
				"package_invalid_archive",
				fmt.Sprintf("target package %s cannot be read completely", targetVersion),
				map[string]string{
					"package_path": packagePath,
					"version":      targetVersion,
				},
			)}
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			continue
		}

		matchedPath, ok := matchTargetConfigPath(header.Name, wanted)
		if !ok {
			continue
		}

		content, err := io.ReadAll(reader)
		if err != nil {
			return result, []BlockingIssue{blockingIssue(
				CheckCategoryPackage,
				"package_invalid_archive",
				fmt.Sprintf("target config %s cannot be extracted from package %s", matchedPath, targetVersion),
				map[string]string{
					"package_path": packagePath,
					"target_path":  matchedPath,
					"version":      targetVersion,
				},
			)}
		}
		result[matchedPath] = string(content)
		delete(wanted, matchedPath)
	}

	issues := make([]BlockingIssue, 0, len(wanted))
	for targetPath := range wanted {
		issues = append(issues, blockingIssue(
			CheckCategoryPackage,
			"package_config_missing",
			fmt.Sprintf("target package %s does not contain config file %s", targetVersion, targetPath),
			map[string]string{
				"package_path": packagePath,
				"target_path":  targetPath,
				"version":      targetVersion,
			},
		))
	}
	return result, issues
}

func matchTargetConfigPath(archivePath string, wanted map[string]struct{}) (string, bool) {
	normalizedArchivePath := normalizeConfigArchivePath(archivePath)
	for targetPath := range wanted {
		if normalizedArchivePath == targetPath || strings.HasSuffix(normalizedArchivePath, "/"+targetPath) {
			return targetPath, true
		}
	}
	return "", false
}

func normalizeConfigArchivePath(value string) string {
	trimmed := strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	trimmed = strings.TrimPrefix(trimmed, "./")
	if trimmed == "" {
		return ""
	}
	return pathpkg.Clean(trimmed)
}
