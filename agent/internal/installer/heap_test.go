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
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeHeapSizeMB(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   int
		want int
	}{
		{name: "legacy 1GB", in: 1, want: 1024},
		{name: "legacy 2GB", in: 2, want: 2048},
		{name: "legacy 128GB ceiling", in: 128, want: 128 * 1024},
		{name: "new 512MB", in: 512, want: 512},
		{name: "new 2048MB", in: 2048, want: 2048},
		{name: "zero unchanged", in: 0, want: 0},
		{name: "negative unchanged", in: -1, want: -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := NormalizeHeapSizeMB(tc.in); got != tc.want {
				t.Fatalf("NormalizeHeapSizeMB(%d)=%d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

func TestValidateHeapSizeMB(t *testing.T) {
	t.Parallel()
	if err := ValidateHeapSizeMB(512); err != nil {
		t.Fatalf("512MB must pass: %v", err)
	}
	if err := ValidateHeapSizeMB(511); err == nil {
		t.Fatal("511MB must fail (below minimum)")
	}
	if err := ValidateHeapSizeMB(maxHeapMB + 1); err == nil {
		t.Fatal("above max must fail")
	}
}

func TestJVMConfigValidateAcceptsLegacyGBAndNewMB(t *testing.T) {
	t.Parallel()
	legacy := &JVMConfig{HybridHeapSize: 2, MasterHeapSize: 1, WorkerHeapSize: 1}
	if err := legacy.Validate(); err != nil {
		t.Fatalf("legacy GB values must validate: %v", err)
	}
	modern := &JVMConfig{HybridHeapSize: 512, MasterHeapSize: 1024, WorkerHeapSize: 2048}
	if err := modern.Validate(); err != nil {
		t.Fatalf("MB values must validate: %v", err)
	}
	// 反例：200 既不是合法遗留 GB（换算后仍 <512? 200>128 so treated as MB）也不够最小堆。
	// Counterexample: 200 is treated as MB (not legacy GB) and is below the minimum.
	bad := &JVMConfig{HybridHeapSize: 200, MasterHeapSize: 200, WorkerHeapSize: 200}
	if err := bad.Validate(); err == nil {
		t.Fatal("200MB must fail validation")
	}
}

func TestModifyJVMOptionsWritesMBNotGB(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "jvm_options")
	original := "## JVM Heap\n# -Xms2g\n# -Xmx2g\n-XX:+UseG1GC\n"
	if err := os.WriteFile(path, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	mgr := &InstallerManager{}
	if err := mgr.modifyJVMOptions(path, NormalizeHeapSizeMB(512)); err != nil {
		t.Fatalf("modifyJVMOptions: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	if !strings.Contains(text, "-Xms512m") || !strings.Contains(text, "-Xmx512m") {
		t.Fatalf("expected -Xms512m/-Xmx512m, got:\n%s", text)
	}
	if strings.Contains(text, "-Xms512g") || strings.Contains(text, "-Xmx512g") {
		t.Fatalf("must not write g suffix for 512MB input:\n%s", text)
	}
	if strings.Contains(text, "-Xms2g") || strings.Contains(text, "-Xmx2g") {
		t.Fatalf("old g lines must be replaced:\n%s", text)
	}
}

func TestModifyJVMOptionsLegacyTwoGBBecomes2048m(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "jvm_options")
	if err := os.WriteFile(path, []byte("-Xms1g\n-Xmx1g\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mgr := &InstallerManager{}
	if err := mgr.modifyJVMOptions(path, NormalizeHeapSizeMB(2)); err != nil {
		t.Fatalf("modifyJVMOptions: %v", err)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	if !strings.Contains(text, "-Xms2048m") || !strings.Contains(text, "-Xmx2048m") {
		t.Fatalf("legacy 2GB must become 2048m, got:\n%s", text)
	}
}

func TestFormatHeapFlagsNeverUseGSuffix(t *testing.T) {
	t.Parallel()
	if got := FormatHeapXms(512); got != "-Xms512m" {
		t.Fatalf("FormatHeapXms(512)=%q", got)
	}
	if got := FormatHeapXmx(2048); got != "-Xmx2048m" {
		t.Fatalf("FormatHeapXmx(2048)=%q", got)
	}
}
