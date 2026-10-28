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
	"os"
	"path/filepath"
	"testing"
)

func TestSanitizeClusterCode(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		fallbackID uint
		expected   string
	}{
		{
			name:       "english and numbers with symbols",
			raw:        "Production-Cluster_01!",
			fallbackID: 1,
			expected:   "productioncluster01",
		},
		{
			name:       "already valid lowercase code",
			raw:        "stxzeta8",
			fallbackID: 2,
			expected:   "stxzeta8",
		},
		{
			name:       "chinese characters only falls back to stxcluster with id",
			raw:        "生产测试集群",
			fallbackID: 5,
			expected:   "stxcluster5",
		},
		{
			name:       "chinese mixed with english and numbers",
			raw:        "实时计算 Zeta-02",
			fallbackID: 3,
			expected:   "zeta02",
		},
		{
			name:       "single char falls back to stxcluster",
			raw:        "A",
			fallbackID: 8,
			expected:   "stxcluster8",
		},
		{
			name:       "over 32 chars truncated",
			raw:        "abcdefghijklmnopqrstuvwxyz0123456789extra",
			fallbackID: 9,
			expected:   "abcdefghijklmnopqrstuvwxyz012345",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actual := SanitizeClusterCode(tc.raw, tc.fallbackID)
			if actual != tc.expected {
				t.Fatalf("expected %q, got %q", tc.expected, actual)
			}
			if !IsValidClusterCode(actual) {
				t.Fatalf("result %q should be valid cluster code", actual)
			}
		})
	}
}

// TestTryExtractClusterNameFromLocalInstallDir 验证从本地安装目录下的 hazelcast.yaml 提取 cluster-name。
// TestTryExtractClusterNameFromLocalInstallDir verifies extracting cluster-name from hazelcast.yaml under local installDir.
func TestTryExtractClusterNameFromLocalInstallDir(t *testing.T) {
	tmpDir := t.TempDir()
	configDir := filepath.Join(tmpDir, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}

	hzYAML := `hazelcast:
  cluster-name: production-sea-01
  network:
    port:
      port: 5801
`
	if err := os.WriteFile(filepath.Join(configDir, "hazelcast.yaml"), []byte(hzYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	extracted := TryExtractClusterNameFromLocalInstallDir(tmpDir)
	if extracted != "production-sea-01" {
		t.Fatalf("expected 'production-sea-01', got %q", extracted)
	}

	sanitized := SanitizeClusterCode(extracted, 0)
	if sanitized != "productionsea01" {
		t.Fatalf("expected 'productionsea01', got %q", sanitized)
	}
}
