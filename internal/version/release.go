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

package version

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// releaseVersionPattern 限定可用于构造本地路径的发布版本格式（可选 v 前缀的 SemVer，可带预发布后缀）。
// releaseVersionPattern restricts release versions that may be used to build local paths
// (SemVer with an optional v prefix and an optional pre-release suffix).
var releaseVersionPattern = regexp.MustCompile(`^[vV]?[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.]+)?$`)

// ValidateReleaseVersion 校验发布版本字符串，拒绝空值、路径分隔符、".."、绝对路径及非版本格式输入。
// ValidateReleaseVersion validates a release version string and rejects empty values, path separators,
// "..", absolute paths, and anything that is not a version such as v1.2.3 or 1.2.3-beta.1.
func ValidateReleaseVersion(v string) error {
	if v == "" {
		return fmt.Errorf("version is empty")
	}
	if strings.Contains(v, "..") || strings.ContainsAny(v, `/\`) || filepath.IsAbs(v) || filepath.VolumeName(v) != "" {
		return fmt.Errorf("version %q must not contain path elements", v)
	}
	if !releaseVersionPattern.MatchString(v) {
		return fmt.Errorf("version %q is not a valid release version (expected v1.2.3 or 1.2.3-beta.1)", v)
	}
	return nil
}
