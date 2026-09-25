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

package cmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	stxversion "github.com/LeonYoah/stx/internal/version"
)

func TestVersionCommand(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exitCode := executeCommand(NewRootCommand(), []string{"version"}, stdout, stderr)
	if exitCode != 0 {
		t.Fatalf("stx version exit = %d, stderr=%s", exitCode, stderr.String())
	}
	var payload struct {
		OperationID string `json:"operation_id"`
		Data        struct {
			Version string `json:"version"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatalf("parse version output: %v\nraw=%s", err, stdout.String())
	}
	if payload.OperationID != "version.get" {
		t.Fatalf("operation_id = %q, want version.get", payload.OperationID)
	}
	if payload.Data.Version != stxversion.Normalize(stxversion.Version) {
		t.Fatalf("version = %q, want %q", payload.Data.Version, stxversion.Version)
	}
	if strings.TrimSpace(stderr.String()) != "" {
		t.Fatalf("stderr should be empty, got %q", stderr.String())
	}
}

func TestRootVersionFlag(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	exitCode := executeCommand(NewRootCommand(), []string{"--version"}, stdout, stderr)
	if exitCode != 0 {
		t.Fatalf("stx --version exit = %d, stderr=%s", exitCode, stderr.String())
	}
	out := strings.TrimSpace(stdout.String())
	if !strings.HasPrefix(out, "stx "+stxversion.Normalize(stxversion.Version)) {
		t.Fatalf("--version output = %q, want prefix %q", out, "stx "+stxversion.Version)
	}
}

