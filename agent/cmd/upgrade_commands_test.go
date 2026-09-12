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

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	pb "github.com/LeonYoah/stx/agent"
	"github.com/LeonYoah/stx/agent/internal/installer"
)

func TestAgent_handleManagedUpgradeCommand_requiresSubCommand(t *testing.T) {
	agent := &Agent{}
	_, err := agent.handleManagedUpgradeCommand(context.Background(), &pb.CommandRequest{CommandId: "test"}, nil)
	if err == nil {
		t.Fatal("expected missing sub_command to be rejected")
	}
}

func TestAgent_handleManagedUpgradeCommand_runsSmokeTestTemplate(t *testing.T) {
	installDir := t.TempDir()
	mustWriteExecutableForAgentTest(t, filepath.Join(installDir, "bin", "seatunnel-cluster.sh"), "#!/usr/bin/env bash\nexit 0\n")
	mustWriteExecutableForAgentTest(t, filepath.Join(installDir, "bin", "seatunnel.sh"), "#!/usr/bin/env bash\necho smoke-ok\n")
	mustWriteFileForAgentTest(t, filepath.Join(installDir, "config", "v2.batch.config.template"), "env {}")

	agent := &Agent{installerManager: installer.NewInstallerManager()}
	resp, err := agent.handleManagedUpgradeCommand(context.Background(), &pb.CommandRequest{
		CommandId: "test",
		Parameters: map[string]string{
			"sub_command": "run_smoke_test_template",
			"install_dir": installDir,
		},
	}, nil)
	if err != nil {
		t.Fatalf("expected smoke test command to succeed, got %v", err)
	}
	if resp == nil || resp.Status != pb.CommandStatus_SUCCESS {
		t.Fatalf("expected successful response, got %+v", resp)
	}
}

func mustWriteFileForAgentTest(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("failed to create dir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write file %s: %v", path, err)
	}
}

func mustWriteExecutableForAgentTest(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("failed to create dir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0755); err != nil {
		t.Fatalf("failed to write executable %s: %v", path, err)
	}
}
