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
	agentdiagnostics "github.com/LeonYoah/stx/agent/internal/diagnostics"
	"github.com/LeonYoah/stx/agent/internal/process"
	"github.com/LeonYoah/stx/internal/processidentity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureLogSender 记录已上报日志条目的 LogSender。
// captureLogSender is a LogSender that records reported entries.
type captureLogSender struct {
	entries []*pb.LogEntry
}

func (s *captureLogSender) IsConnected() bool     { return true }
func (s *captureLogSender) GetAgentID() string    { return "agent-test" }
func (s *captureLogSender) SendLogEntries(_ context.Context, entries []*pb.LogEntry) error {
	s.entries = append(s.entries, entries...)
	return nil
}

// writeEngineErrorLog 在安装目录下放一份含 ERROR 的引擎日志。
// writeEngineErrorLog drops an engine log containing an ERROR line under the install dir.
func writeEngineErrorLog(t *testing.T, installDir string) {
	t.Helper()
	logDir := filepath.Join(installDir, "logs")
	require.NoError(t, os.MkdirAll(logDir, 0o755))
	content := "2026-03-10 10:00:01,001 ERROR [main] java.lang.IllegalStateException: engine exploded\n" +
		"\tat org.apache.seatunnel.Engine.run(Engine.java:123)\n"
	require.NoError(t, os.WriteFile(filepath.Join(logDir, "seatunnel-engine-master.log"), []byte(content), 0o644))
}

// TestUpdateDiagnosticsTargetsFollowsTrackedProcesses START/RESTART 在 TrackProcess 后调用
// updateDiagnosticsTargets：目标进入 collector 后其 ERROR 日志才会被扫描上报。
// TestUpdateDiagnosticsTargetsFollowsTrackedProcesses: START/RESTART call updateDiagnosticsTargets
// after TrackProcess; only then does the collector scan and report that target's ERROR logs.
func TestUpdateDiagnosticsTargetsFollowsTrackedProcesses(t *testing.T) {
	ctx := context.Background()
	agent := NewAgent(&config.Config{})
	sender := &captureLogSender{}
	agent.errorCollector = agentdiagnostics.NewCollector(sender)

	installDir := t.TempDir()
	writeEngineErrorLog(t, installDir)

	// 未跟踪任何进程时目标为空，采集不到日志。
	// With nothing tracked, targets are empty and nothing is collected.
	agent.updateDiagnosticsTargets(ctx)
	require.NoError(t, agent.errorCollector.CollectOnce(ctx))
	assert.Empty(t, sender.entries)

	// START/RESTART 路径：TrackProcess + updateDiagnosticsTargets 后 ERROR 日志被上报。
	// START/RESTART path: after TrackProcess + updateDiagnosticsTargets the ERROR log is reported.
	name := processidentity.ManagedName(installDir, "master")
	agent.processMonitor.TrackProcess(name, 4321, installDir, "master", &process.StartParams{})
	agent.updateDiagnosticsTargets(ctx)
	require.NoError(t, agent.errorCollector.CollectOnce(ctx))
	require.NotEmpty(t, sender.entries, "tracked process must become a diagnostics scan target")
	assert.Equal(t, name, sender.entries[0].Fields["process_name"])
	assert.Equal(t, installDir, sender.entries[0].Fields["install_dir"])

	// UPDATE_MONITOR_CONFIG 移除跟踪后同步目标，停止采集。
	// After UPDATE_MONITOR_CONFIG untracks the process, syncing targets stops collection.
	sender.entries = nil
	agent.processMonitor.UntrackProcessSilent(name)
	agent.updateDiagnosticsTargets(ctx)
	require.NoError(t, agent.errorCollector.CollectOnce(ctx))
	assert.Empty(t, sender.entries)
}
