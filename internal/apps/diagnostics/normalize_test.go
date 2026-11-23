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
	"strings"
	"testing"
)

func TestBuildErrorFingerprintGroupsDeadlineExceededVariants(t *testing.T) {
	message1 := "DEADLINE_EXCEEDED: deadline exceeded after 9.996847312s. Name resolution delay 0.000990884 seconds. [closed=[], open=[[wait_for_ready, buffered_nanos=9999873497, waiting_for_connection]]]"
	message2 := "Failed to initialize connection. Error: DEADLINE_EXCEEDED: deadline exceeded after 9.988364578s. Name resolution delay 0.001774617 seconds. [closed=[], open=[[wait_for_ready, buffered_nanos=9989656015, waiting_for_connection]]]"
	evidence3 := "" +
		"[38.55.133.202]:5801 [seatunnel] [5.1] submit job 1083784734460346369 error org.apache.seatunnel.common.exception.SeaTunnelRuntimeException: ErrorCode:[API-09], ErrorDescription:[Handle save mode failed]\n" +
		"\tat org.apache.seatunnel.engine.server.master.JobMaster.handleSaveMode(JobMaster.java:573)\n" +
		"Caused by: java.lang.RuntimeException: Failed to initialize connection. Error: DEADLINE_EXCEEDED: deadline exceeded after 9.986412457s. Name resolution delay 0.003577568 seconds. [closed=[], open=[[wait_for_ready, buffered_nanos=9997862387, waiting_for_connection]]]"

	fp1, normalized1, _, title1 := BuildErrorFingerprint(message1, message1)
	fp2, normalized2, _, title2 := BuildErrorFingerprint(message2, message2)
	fp3, normalized3, _, title3 := BuildErrorFingerprint(message2, evidence3)

	if fp1 != fp2 || fp2 != fp3 {
		t.Fatalf("expected same fingerprint for deadline exceeded variants, got fp1=%s fp2=%s fp3=%s", fp1, fp2, fp3)
	}
	if title1 == "" || title2 == "" || title3 == "" {
		t.Fatalf("expected non-empty titles, got %q %q %q", title1, title2, title3)
	}
	if normalized1 != normalized2 || normalized2 != normalized3 {
		t.Fatalf("expected same normalized text for deadline exceeded variants")
	}
}

func TestBuildErrorFingerprintIgnoresWrapperNoiseAndUsesRealException(t *testing.T) {
	message := "Fatal Error,"
	evidence := "" +
		"===============================================================================\n" +
		"Fatal Error,\n" +
		"Please submit bug report in https://github.com/apache/seatunnel/issues\n" +
		"Reason:SeaTunnel job executed failed\n" +
		"Exception StackTrace:org.apache.seatunnel.core.starter.exception.CommandExecuteException: SeaTunnel job executed failed\n" +
		"\tat org.apache.seatunnel.core.starter.seatunnel.command.ClientExecuteCommand.execute(ClientExecuteCommand.java:228)\n" +
		"Caused by: java.lang.OutOfMemoryError: Java heap space\n"

	fp, normalized, exceptionClass, title := BuildErrorFingerprint(message, evidence)
	if fp == "" {
		t.Fatal("expected fingerprint for wrapped exception evidence")
	}
	if exceptionClass != "java.lang.OutOfMemoryError" {
		t.Fatalf("expected root exception class, got %q", exceptionClass)
	}
	if title != "java.lang.OutOfMemoryError: Java heap space" {
		t.Fatalf("unexpected title %q", title)
	}
	if normalized == "" {
		t.Fatal("expected normalized text")
	}
}

func TestBuildErrorFingerprintReturnsEmptyForNoiseOnlyEvidence(t *testing.T) {
	message := "Fatal Error,"
	evidence := "" +
		"===============================================================================\n" +
		"Fatal Error,\n" +
		"Please submit bug report in https://github.com/apache/seatunnel/issues\n" +
		"Reason:SeaTunnel job executed failed\n"

	fp, normalized, exceptionClass, title := BuildErrorFingerprint(message, evidence)
	if fp != "" || normalized != "" || exceptionClass != "" || title != "" {
		t.Fatalf("expected noise-only evidence to be ignored, got fp=%q normalized=%q exception=%q title=%q", fp, normalized, exceptionClass, title)
	}
}

func TestBuildErrorFingerprintReturnsEmptyForNoiseOnlyEvidenceWithSeatunnelLogHeader(t *testing.T) {
	message := "Fatal Error,"
	evidence := "" +
		"[] 2026-03-15 00:33:22,900 ERROR [o.a.s.c.s.SeaTunnel           ] [main] - Fatal Error,\n" +
		"===============================================================================\n" +
		"Fatal Error,\n" +
		"Please submit bug report in https://github.com/apache/seatunnel/issues\n" +
		"Reason:SeaTunnel job executed failed\n"

	fp, normalized, exceptionClass, title := BuildErrorFingerprint(message, evidence)
	if fp != "" || normalized != "" || exceptionClass != "" || title != "" {
		t.Fatalf("expected noise-only evidence with log header to be ignored, got fp=%q normalized=%q exception=%q title=%q", fp, normalized, exceptionClass, title)
	}
}

func TestBuildErrorFingerprintUsesRootCauseFromMergedSeatunnelFatalBlock(t *testing.T) {
	message := "Fatal Error,"
	evidence := "" +
		"[] 2026-03-21 13:21:35,462 ERROR [o.a.s.c.s.SeaTunnel           ] [main] - \n" +
		"===============================================================================\n" +
		"[] 2026-03-21 13:21:35,462 ERROR [o.a.s.c.s.SeaTunnel           ] [main] - Fatal Error,\n" +
		"[] 2026-03-21 13:21:35,462 ERROR [o.a.s.c.s.SeaTunnel           ] [main] - Please submit bug report in https://github.com/apache/seatunnel/issues\n" +
		"[] 2026-03-21 13:21:35,466 ERROR [o.a.s.c.s.SeaTunnel           ] [main] - Reason:Invalid YAML configuration\n" +
		"[] 2026-03-21 13:21:35,471 ERROR [o.a.s.c.s.SeaTunnel           ] [main] - Exception StackTrace:com.hazelcast.config.InvalidConfigurationException: Invalid YAML configuration\n" +
		"\tat com.hazelcast.config.YamlConfigBuilder.parseAndBuildConfig(YamlConfigBuilder.java:151)\n" +
		"Caused by: com.hazelcast.internal.yaml.YamlException: An error occurred while loading and parsing the YAML stream\n"

	fp, normalized, exceptionClass, title := BuildErrorFingerprint(message, evidence)
	if fp == "" {
		t.Fatal("expected fingerprint for merged Seatunnel fatal block")
	}
	if exceptionClass != "com.hazelcast.internal.yaml.YamlException" {
		t.Fatalf("expected merged fatal block root cause class, got %q", exceptionClass)
	}
	if title != "com.hazelcast.internal.yaml.YamlException: An error occurred while loading and parsing the YAML stream" {
		t.Fatalf("unexpected merged fatal block title %q", title)
	}
	if normalized == "" {
		t.Fatal("expected normalized text for merged fatal block")
	}
}

// TestBuildErrorFingerprintConvergesGetSubjectUnsupported 现场裂组样本：FileMapStore 报错、
// Hazelcast 成员前缀行与裸异常行必须收敛为同一指纹。
// TestBuildErrorFingerprintConvergesGetSubjectUnsupported: FileMapStore error lines, member-prefixed
// lines and the bare exception line must all converge on one fingerprint.
func TestBuildErrorFingerprintConvergesGetSubjectUnsupported(t *testing.T) {
	exceptionLine := "java.lang.UnsupportedOperationException: getSubject is not supported"
	samples := []struct {
		message  string
		evidence string
	}{
		{
			message: "Failed to initialize IMap storage for map 'engine_checkpoint-id-map'",
			evidence: "2026-04-10 10:00:00,000 ERROR [c.h.m.i.m.MapStoreContext] - Failed to initialize IMap storage for map 'engine_checkpoint-id-map'\n" +
				"java.lang.UnsupportedOperationException: getSubject is not supported\n" +
				"\tat com.hazelcast.map.impl.mapstore.MapStoreContextFactory.createMapStoreContext(MapStoreContextFactory.java:123)",
		},
		{
			message: "Failed to initialize IMap storage for map 'engine_runningJobInfo'",
			evidence: "2026-04-10 10:00:01,000 ERROR [c.h.m.i.m.MapStoreContext] - Failed to initialize IMap storage for map 'engine_runningJobInfo'\n" +
				"java.lang.UnsupportedOperationException: getSubject is not supported\n" +
				"\tat com.hazelcast.map.impl.mapstore.MapStoreContextFactory.createMapStoreContext(MapStoreContextFactory.java:123)",
		},
		{
			message: "[192.168.0.105]:35801 [seatunnel] [5.1.7] getSubject is not supported",
			evidence: "[192.168.0.105]:35801 [seatunnel] [5.1.7] getSubject is not supported\n" +
				"java.lang.UnsupportedOperationException: getSubject is not supported\n" +
				"\tat java.base/java.lang.Thread.run(Thread.java:1583)",
		},
		{
			message:  exceptionLine,
			evidence: exceptionLine + "\n\tat java.base/java.lang.Thread.run(Thread.java:1583)",
		},
	}

	var wantFp, wantNormalized string
	for i, sample := range samples {
		fp, normalized, exceptionClass, title := BuildErrorFingerprint(sample.message, sample.evidence)
		if exceptionClass != "java.lang.UnsupportedOperationException" {
			t.Fatalf("sample %d: expected exception class java.lang.UnsupportedOperationException, got %q", i, exceptionClass)
		}
		if title != exceptionLine {
			t.Fatalf("sample %d: expected title %q, got %q", i, exceptionLine, title)
		}
		if i == 0 {
			wantFp, wantNormalized = fp, normalized
			continue
		}
		if fp != wantFp || normalized != wantNormalized {
			t.Fatalf("sample %d: expected same fingerprint %q, got %q (normalized %q)", i, wantFp, fp, normalized)
		}
	}
}

// TestBuildErrorFingerprintStripsHazelcastMemberPrefix 验证成员前缀剥离：WorkerHeartbeatOperation
// 日志携带 [host]:port [cluster] [version] 前缀时仍提取顶层 NPE，标题/指纹不含 IP 与集群名。
// TestBuildErrorFingerprintStripsHazelcastMemberPrefix: with a "[host]:port [cluster] [version]"
// prefix the top-level NPE is still extracted, and title/fingerprint carry no IP or cluster name.
func TestBuildErrorFingerprintStripsHazelcastMemberPrefix(t *testing.T) {
	message := "[192.168.0.105]:35801 [seatunnel] [5.1.7] Cannot invoke \"com.hazelcast.internal.services.ManagedService.init\" because \"this.coordinatorService\" is null"
	evidence := "" +
		"[192.168.0.105]:35801 [seatunnel] [5.1.7] WorkerHeartbeatOperation failed\n" +
		"java.lang.NullPointerException: Cannot invoke \"com.hazelcast.internal.services.ManagedService.init\" because \"this.coordinatorService\" is null\n" +
		"\tat com.hazelcast.cluster.impl.operations.WorkerHeartbeatOperation.run(WorkerHeartbeatOperation.java:75)\n" +
		"\tat com.hazelcast.spi.impl.operationservice.Operation.call(Operation.java:148)\n"

	fp, normalized, exceptionClass, title := BuildErrorFingerprint(message, evidence)
	if fp == "" {
		t.Fatal("expected fingerprint")
	}
	if exceptionClass != "java.lang.NullPointerException" {
		t.Fatalf("expected java.lang.NullPointerException, got %q", exceptionClass)
	}
	if strings.Contains(title, "192.168.0.105") || strings.Contains(title, "seatunnel]") ||
		strings.Contains(normalized, "192.168") || strings.Contains(normalized, "[seatunnel]") {
		t.Fatalf("expected no IP/cluster prefix in title/normalized, got title=%q normalized=%q", title, normalized)
	}
}
