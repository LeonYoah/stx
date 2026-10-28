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

package cluster

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

var clusterCodeRegex = regexp.MustCompile(`^[a-z0-9]{2,32}$`)

// IsValidClusterCode 校验集群编码是否符合规范（2-32位纯小写字母与数字）。
// IsValidClusterCode validates if cluster code matches format (2-32 chars of lowercase letters and digits).
func IsValidClusterCode(code string) bool {
	return clusterCodeRegex.MatchString(code)
}

// SanitizeClusterCode 清洗并生成合法的集群编码。
// 规则：保留英文字母与数字，转小写；若清洗后有效字符不足2位，则使用 "stxcluster" 结合 fallbackID 兜底；最大截断至 32 位。
// SanitizeClusterCode sanitizes and generates a valid cluster code.
// Rule: preserves letters & digits in lowercase; falls back to "stxcluster" + fallbackID if < 2 chars; max 32 chars.
func SanitizeClusterCode(raw string, fallbackID uint) string {
	var sb strings.Builder
	for _, r := range strings.ToLower(raw) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			sb.WriteRune(r)
		}
	}
	res := sb.String()
	if len(res) > 32 {
		res = res[:32]
	}
	if len(res) < 2 {
		if fallbackID > 0 {
			res = fmt.Sprintf("stxcluster%d", fallbackID)
		} else {
			res = "stxcluster"
		}
	}
	return res
}

// TryExtractClusterNameFromLocalInstallDir 尝试从本地安装目录下的 Hazelcast 配置文件解析 cluster-name。
// TryExtractClusterNameFromLocalInstallDir tries to extract cluster-name from Hazelcast config under local installDir.
func TryExtractClusterNameFromLocalInstallDir(installDir string) string {
	if strings.TrimSpace(installDir) == "" {
		return ""
	}
	candidates := []string{
		filepath.Join(installDir, "config", "hazelcast.yaml"),
		filepath.Join(installDir, "config", "hazelcast-master.yaml"),
		filepath.Join(installDir, "config", "hazelcast-worker.yaml"),
	}
	for _, p := range candidates {
		if content, err := os.ReadFile(p); err == nil {
			if name := extractClusterNameFromYAMLBytes(content); name != "" {
				return name
			}
		}
	}
	return ""
}

// extractClusterNameFromYAMLBytes 从 YAML 字节中解析 hazelcast.cluster-name。
// extractClusterNameFromYAMLBytes extracts hazelcast.cluster-name from YAML bytes.
func extractClusterNameFromYAMLBytes(content []byte) string {
	var root yaml.Node
	if err := yaml.Unmarshal(content, &root); err != nil || len(root.Content) == 0 {
		return ""
	}
	doc := root.Content[0]
	if doc.Kind != yaml.MappingNode {
		return ""
	}
	for i := 0; i+1 < len(doc.Content); i += 2 {
		if doc.Content[i].Value == "hazelcast" && doc.Content[i+1].Kind == yaml.MappingNode {
			m := doc.Content[i+1]
			for j := 0; j+1 < len(m.Content); j += 2 {
				if m.Content[j].Value == "cluster-name" {
					return strings.TrimSpace(m.Content[j+1].Value)
				}
			}
		}
	}
	return ""
}
