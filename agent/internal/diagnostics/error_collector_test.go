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

package diagnostics

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	pb "github.com/LeonYoah/stx/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeLogSender struct {
	agentID   string
	connected bool
	batches   [][]*pb.LogEntry
}

func (f *fakeLogSender) IsConnected() bool {
	return f.connected
}

func (f *fakeLogSender) GetAgentID() string {
	return f.agentID
}

func (f *fakeLogSender) SendLogEntries(_ context.Context, entries []*pb.LogEntry) error {
	copied := make([]*pb.LogEntry, 0, len(entries))
	for _, entry := range entries {
		if entry == nil {
			continue
		}
		copyEntry := *entry
		copied = append(copied, &copyEntry)
	}
	f.batches = append(f.batches, copied)
	return nil
}

func TestCollectorCollectsEngineAndJobErrorsIncrementally(t *testing.T) {
	root := t.TempDir()
	installDir := filepath.Join(root, "seatunnel")
	logDir := filepath.Join(installDir, "logs")
	require.NoError(t, os.MkdirAll(logDir, 0o755))

	engineLog := filepath.Join(logDir, "seatunnel-engine-worker.log")
	jobLog := filepath.Join(logDir, "job-898380162133917698.log")

	engineContent := "2026-03-10 10:00:00,001 INFO [main] bootstrap\n" +
		"2026-03-10 10:00:01,001 ERROR [main] java.lang.IllegalStateException: worker unhealthy\n" +
		"\tat org.apache.seatunnel.Engine.run(Engine.java:100)\n"
	jobContent := "2026-03-10 10:00:02,001 ERROR [task] org.apache.seatunnel.engine.server.exception.TaskExecuteException: job failed\n" +
		"\tat org.apache.seatunnel.Job.run(Job.java:200)\n"
	require.NoError(t, os.WriteFile(engineLog, []byte(engineContent), 0o644))
	require.NoError(t, os.WriteFile(jobLog, []byte(jobContent), 0o644))

	sender := &fakeLogSender{agentID: "agent-1", connected: true}
	collector := NewCollector(sender)
	collector.ReplaceTargets([]*ScanTarget{{Name: "seatunnel-worker", InstallDir: installDir, Role: "worker"}})

	require.NoError(t, collector.CollectOnce(t.Context()))
	require.Len(t, sender.batches, 1)
	require.Len(t, sender.batches[0], 2)

	assert.Equal(t, "seatunnel_error", sender.batches[0][0].Fields["source"])
	assert.Equal(t, installDir, sender.batches[0][0].Fields["install_dir"])
	assert.Equal(t, "worker", sender.batches[0][0].Fields["role"])

	jobEntryFound := false
	for _, entry := range sender.batches[0] {
		if entry.Fields["source_kind"] == "job" {
			jobEntryFound = true
			assert.Equal(t, "898380162133917698", entry.Fields["job_id"])
		}
	}
	assert.True(t, jobEntryFound)

	require.NoError(t, collector.CollectOnce(t.Context()))
	assert.Len(t, sender.batches, 1)

	appendContent := "2026-03-10 10:00:03,001 ERROR [main] java.lang.IllegalStateException: worker unhealthy\n\tat org.apache.seatunnel.Engine.run(Engine.java:300)\n"
	file, err := os.OpenFile(engineLog, os.O_APPEND|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	_, err = file.WriteString(appendContent)
	require.NoError(t, err)
	require.NoError(t, file.Close())

	require.NoError(t, collector.CollectOnce(t.Context()))
	require.Len(t, sender.batches, 2)
	require.Len(t, sender.batches[1], 1)
	assert.Contains(t, sender.batches[1][0].Message, "IllegalStateException")
}

func TestCollectorParsesSeatunnelPrefixedErrorLogs(t *testing.T) {
	root := t.TempDir()
	installDir := filepath.Join(root, "seatunnel")
	logDir := filepath.Join(installDir, "logs")
	require.NoError(t, os.MkdirAll(logDir, 0o755))

	engineLog := filepath.Join(logDir, "seatunnel-engine-coordinator.log")
	content := "" +
		"[1083779072695205889] 2026-03-10 23:51:07,375 INFO  [.c.c.DefaultClassLoaderService] [seatunnel-coordinator-service-2] - Create classloader for job 1 with jars [file:/opt/seatunnel-2.3.11-new/connectors/connector-fake-2.3.11.jar]\n" +
		"[1083779072695205889] 2026-03-10 23:51:18,677 ERROR [i.m.c.AbstractMilvusGrpcClient] [seatunnel-coordinator-service-2] - DEADLINE_EXCEEDED: deadline exceeded after 9.869890448s. Name resolution delay 0.006655146 seconds. [closed=[], open=[[wait_for_ready, buffered_nanos=9873352186, waiting_for_connection]]]\n" +
		"[1083779072695205889] 2026-03-10 23:51:18,678 ERROR [i.m.c.AbstractMilvusGrpcClient] [seatunnel-coordinator-service-2] - Failed to initialize connection. Error: DEADLINE_EXCEEDED: deadline exceeded after 9.869890448s. Name resolution delay 0.006655146 seconds. [closed=[], open=[[wait_for_ready, buffered_nanos=9873352186, waiting_for_connection]]]\n" +
		"[1083779072695205889] 2026-03-10 23:51:18,683 ERROR [o.a.s.e.s.CoordinatorService  ] [seatunnel-coordinator-service-2] - [38.55.133.202]:5801 [seatunnel] [5.1] submit job 1083779072695205889 error org.apache.seatunnel.common.exception.SeaTunnelRuntimeException: ErrorCode:[API-09], ErrorDescription:[Handle save mode failed]\n" +
		"\tat org.apache.seatunnel.engine.server.master.JobMaster.handleSaveMode(JobMaster.java:573)\n" +
		"Caused by: java.lang.RuntimeException: Failed to initialize connection. Error: DEADLINE_EXCEEDED: deadline exceeded after 9.869890448s.\n" +
		"[] 2026-03-10 23:51:18,710 INFO  [c.h.i.s.t.TcpServerConnection ] [hz.main.IO.thread-in-1] - [38.55.133.202]:5801 [seatunnel] [5.1] Connection[id=2] closed. Reason: Connection closed by the other side\n"
	require.NoError(t, os.WriteFile(engineLog, []byte(content), 0o644))

	sender := &fakeLogSender{agentID: "agent-1", connected: true}
	collector := NewCollector(sender)
	collector.ReplaceTargets([]*ScanTarget{{Name: "seatunnel-coordinator", InstallDir: installDir, Role: "coordinator"}})

	require.NoError(t, collector.CollectOnce(t.Context()))
	require.Len(t, sender.batches, 1)
	require.Len(t, sender.batches[0], 3)

	first := sender.batches[0][0]
	assert.Equal(t, "1083779072695205889", first.Fields["execution_id"])
	assert.Equal(t, "i.m.c.AbstractMilvusGrpcClient", first.Fields["logger_name"])
	assert.Equal(t, "seatunnel-coordinator-service-2", first.Fields["thread_name"])
	assert.Contains(t, first.Message, "DEADLINE_EXCEEDED")
	assert.NotContains(t, first.Message, "AbstractMilvusGrpcClient")

	last := sender.batches[0][2]
	assert.Contains(t, last.Message, "Failed to initialize connection")
	assert.Contains(t, last.Fields["body"], "Caused by: java.lang.RuntimeException")
	assert.Contains(t, last.Fields["body"], "SeaTunnelRuntimeException")
	assert.NotContains(t, last.Fields["body"], "Connection[id=2] closed")
}

func TestCollectorMergesSeatunnelFatalWrapperBlockIntoSingleEntry(t *testing.T) {
	root := t.TempDir()
	installDir := filepath.Join(root, "seatunnel")
	logDir := filepath.Join(installDir, "logs")
	require.NoError(t, os.MkdirAll(logDir, 0o755))

	engineLog := filepath.Join(logDir, "seatunnel-engine-server.log")
	content := "" +
		"[] 2026-03-21 13:21:35,462 ERROR [o.a.s.c.s.SeaTunnel           ] [main] - \n" +
		"===============================================================================\n" +
		"[] 2026-03-21 13:21:35,462 ERROR [o.a.s.c.s.SeaTunnel           ] [main] - Fatal Error,\n" +
		"[] 2026-03-21 13:21:35,462 ERROR [o.a.s.c.s.SeaTunnel           ] [main] - Please submit bug report in https://github.com/apache/seatunnel/issues\n" +
		"[] 2026-03-21 13:21:35,466 ERROR [o.a.s.c.s.SeaTunnel           ] [main] - Reason:Invalid YAML configuration\n" +
		"[] 2026-03-21 13:21:35,471 ERROR [o.a.s.c.s.SeaTunnel           ] [main] - Exception StackTrace:com.hazelcast.config.InvalidConfigurationException: Invalid YAML configuration\n" +
		"\tat com.hazelcast.config.YamlConfigBuilder.parseAndBuildConfig(YamlConfigBuilder.java:151)\n" +
		"Caused by: com.hazelcast.internal.yaml.YamlException: An error occurred while loading and parsing the YAML stream\n" +
		"[] 2026-03-21 13:21:35,471 ERROR [o.a.s.c.s.SeaTunnel           ] [main] - \n" +
		"===============================================================================\n" +
		"[] 2026-03-21 13:21:35,900 INFO [o.a.s.c.s.SeaTunnel           ] [main] - startup aborted\n"
	require.NoError(t, os.WriteFile(engineLog, []byte(content), 0o644))

	sender := &fakeLogSender{agentID: "agent-1", connected: true}
	collector := NewCollector(sender)
	collector.ReplaceTargets([]*ScanTarget{{Name: "seatunnel-server", InstallDir: installDir, Role: "server"}})

	require.NoError(t, collector.CollectOnce(t.Context()))
	require.Len(t, sender.batches, 1)
	require.Len(t, sender.batches[0], 1)

	entry := sender.batches[0][0]
	assert.Contains(t, entry.Message, "YamlException")
	assert.Contains(t, entry.Message, "loading and parsing the YAML stream")
	assert.Contains(t, entry.Fields["body"], "Reason:Invalid YAML configuration")
	assert.Contains(t, entry.Fields["body"], "Exception StackTrace:com.hazelcast.config.InvalidConfigurationException")
	assert.Contains(t, entry.Fields["body"], "Caused by: com.hazelcast.internal.yaml.YamlException")
}

func TestCollectorRecoversErrorsFromRotatedLogGap(t *testing.T) {
	root := t.TempDir()
	installDir := filepath.Join(root, "seatunnel")
	logDir := filepath.Join(installDir, "logs")
	require.NoError(t, os.MkdirAll(logDir, 0o755))

	activeLog := filepath.Join(logDir, "seatunnel-engine-server.log")
	initialContent := "" +
		"2026-03-11 00:30:00,001 INFO [main] bootstrap\n" +
		"2026-03-11 00:30:01,001 ERROR [main] java.lang.IllegalStateException: first error\n" +
		"\tat org.apache.seatunnel.Engine.run(Engine.java:100)\n"
	require.NoError(t, os.WriteFile(activeLog, []byte(initialContent), 0o644))

	sender := &fakeLogSender{agentID: "agent-1", connected: true}
	collector := NewCollector(sender)
	collector.ReplaceTargets([]*ScanTarget{{Name: "seatunnel-server", InstallDir: installDir, Role: "server"}})

	require.NoError(t, collector.CollectOnce(t.Context()))
	require.Len(t, sender.batches, 1)
	require.Len(t, sender.batches[0], 1)

	file, err := os.OpenFile(activeLog, os.O_APPEND|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	_, err = file.WriteString("2026-03-11 00:30:02,001 ERROR [main] java.lang.RuntimeException: rotated gap error\n\tat org.apache.seatunnel.Engine.run(Engine.java:200)\n")
	require.NoError(t, err)
	require.NoError(t, file.Close())

	rotatedLog := activeLog + ".2026-03-11-1"
	require.NoError(t, os.Rename(activeLog, rotatedLog))
	newActiveContent := "" +
		"2026-03-11 00:30:03,001 ERROR [main] java.lang.IllegalArgumentException: new active error\n" +
		"\tat org.apache.seatunnel.Engine.run(Engine.java:300)\n"
	require.NoError(t, os.WriteFile(activeLog, []byte(newActiveContent), 0o644))

	require.NoError(t, collector.CollectOnce(t.Context()))
	require.Len(t, sender.batches, 2)
	require.Len(t, sender.batches[1], 2)
	assert.Equal(t, rotatedLog, sender.batches[1][0].Fields["source_file"])
	assert.Contains(t, sender.batches[1][0].Message, "rotated gap error")
	assert.Equal(t, activeLog, sender.batches[1][1].Fields["source_file"])
	assert.Contains(t, sender.batches[1][1].Message, "new active error")
}
