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
	"github.com/LeonYoah/stx/agent/internal/config"
	"github.com/LeonYoah/stx/agent/internal/installer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type noopProgressReporter struct{}

func (noopProgressReporter) Report(progress int32, output string) error {
	return nil
}

func TestHandleRemoveInstallDirCommandRejectsUnmanagedDir(t *testing.T) {
	configuredBase := filepath.Join(t.TempDir(), "seatunnel")
	require.NoError(t, os.MkdirAll(configuredBase, 0o755))

	unmanagedDir := filepath.Join(t.TempDir(), "outside")
	require.NoError(t, os.MkdirAll(unmanagedDir, 0o755))

	agent := NewAgent(&config.Config{
		SeaTunnel: config.SeaTunnelConfig{InstallDir: configuredBase},
	})

	cmd := &pb.CommandRequest{
		CommandId:  "cmd-1",
		Parameters: map[string]string{"install_dir": unmanagedDir},
	}

	resp, err := agent.handleRemoveInstallDirCommand(context.Background(), cmd, noopProgressReporter{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "please remove it manually")
	assert.Equal(t, pb.CommandStatus_FAILED, resp.Status)

	_, statErr := os.Stat(unmanagedDir)
	assert.NoError(t, statErr)
}

func TestHandleRemoveInstallDirCommandRejectsLegacyLookingCustomDir(t *testing.T) {
	configuredBase := filepath.Join(t.TempDir(), "seatunnel")
	require.NoError(t, os.MkdirAll(configuredBase, 0o755))

	targetDir := filepath.Join(t.TempDir(), "custom", "seatunnel-2.3.13")
	mustWriteInstallFile(t, filepath.Join(targetDir, "bin", "seatunnel-cluster.sh"), "echo start")
	mustWriteInstallFile(t, filepath.Join(targetDir, "bin", "stop-seatunnel-cluster.sh"), "echo stop")

	agent := NewAgent(&config.Config{
		SeaTunnel: config.SeaTunnelConfig{InstallDir: configuredBase},
	})

	cmd := &pb.CommandRequest{
		CommandId:  "cmd-2",
		Parameters: map[string]string{"install_dir": targetDir},
	}

	resp, err := agent.handleRemoveInstallDirCommand(context.Background(), cmd, noopProgressReporter{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "please remove it manually")
	assert.Equal(t, pb.CommandStatus_FAILED, resp.Status)

	_, statErr := os.Stat(targetDir)
	assert.NoError(t, statErr)
}

func TestHandleRemoveInstallDirCommandAllowsMarkerManagedDir(t *testing.T) {
	targetDir := filepath.Join(t.TempDir(), "marker-only")
	require.NoError(t, os.MkdirAll(targetDir, 0o755))
	require.NoError(t, installer.WriteManagedInstallMarker(targetDir))

	agent := NewAgent(&config.Config{})

	cmd := &pb.CommandRequest{
		CommandId:  "cmd-3",
		Parameters: map[string]string{"install_dir": targetDir},
	}

	resp, err := agent.handleRemoveInstallDirCommand(context.Background(), cmd, noopProgressReporter{})
	require.NoError(t, err)
	assert.Equal(t, pb.CommandStatus_SUCCESS, resp.Status)

	_, statErr := os.Stat(targetDir)
	assert.True(t, os.IsNotExist(statErr))
}

func mustWriteInstallFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}
