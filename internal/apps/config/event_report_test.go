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

package config

import (
	"context"
	"errors"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const seatunnelYAMLSample = `seatunnel:
  engine:
    # keep this comment
    history-job-expire-minutes: 1440
    http:
      enable-http: true
      port: 8080
`

func decodeEventReport(t *testing.T, content string) map[string]any {
	t.Helper()
	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(content), &parsed); err != nil {
		t.Fatalf("patched yaml is invalid: %v\n%s", err, content)
	}
	seatunnel, _ := parsed["seatunnel"].(map[string]any)
	engine, _ := seatunnel["engine"].(map[string]any)
	block, _ := engine["event-report-http"].(map[string]any)
	return block
}

func TestPatchEventReportHTTP(t *testing.T) {
	settings := &EventReportSettings{
		URL:               "http://stx:17800/api/v1/sync/engine-events/1",
		Headers:           map[string]string{"Authorization": "Bearer abc", "Content-Type": "application/json"},
		ReportNonTerminal: true,
	}

	t.Run("adds block and keeps other keys and comments", func(t *testing.T) {
		patched, err := PatchEventReportHTTP(seatunnelYAMLSample, settings)
		if err != nil {
			t.Fatalf("patch: %v", err)
		}
		block := decodeEventReport(t, patched)
		if block["url"] != settings.URL || block["report-non-terminal-job-state"] != true {
			t.Fatalf("unexpected block: %v", block)
		}
		headers, _ := block["headers"].(map[string]any)
		if headers["Authorization"] != "Bearer abc" {
			t.Fatalf("unexpected headers: %v", headers)
		}
		if !strings.Contains(patched, "# keep this comment") || !strings.Contains(patched, "port: 8080") {
			t.Fatalf("existing content lost:\n%s", patched)
		}
	})

	t.Run("replaces existing block", func(t *testing.T) {
		first, _ := PatchEventReportHTTP(seatunnelYAMLSample, &EventReportSettings{URL: "http://old"})
		second, err := PatchEventReportHTTP(first, settings)
		if err != nil {
			t.Fatalf("patch: %v", err)
		}
		if strings.Count(second, "event-report-http") != 1 || decodeEventReport(t, second)["url"] != settings.URL {
			t.Fatalf("expected a single replaced block:\n%s", second)
		}
	})

	t.Run("terminal only omits report-non-terminal-job-state", func(t *testing.T) {
		patched, _ := PatchEventReportHTTP(seatunnelYAMLSample, &EventReportSettings{URL: "http://x"})
		if strings.Contains(patched, "report-non-terminal-job-state") {
			t.Fatalf("2.3.12 config must not contain the 3.x-only key:\n%s", patched)
		}
	})

	t.Run("removes block", func(t *testing.T) {
		enabled, _ := PatchEventReportHTTP(seatunnelYAMLSample, settings)
		removed, err := PatchEventReportHTTP(enabled, nil)
		if err != nil {
			t.Fatalf("remove: %v", err)
		}
		if strings.Contains(removed, "event-report-http") || !strings.Contains(removed, "history-job-expire-minutes") {
			t.Fatalf("unexpected removal result:\n%s", removed)
		}
	})

	t.Run("removing an absent block returns content unchanged", func(t *testing.T) {
		removed, err := PatchEventReportHTTP(seatunnelYAMLSample, nil)
		if err != nil || removed != seatunnelYAMLSample {
			t.Fatalf("expected untouched content, err=%v\n%s", err, removed)
		}
	})

	t.Run("creates engine section when missing", func(t *testing.T) {
		patched, err := PatchEventReportHTTP("seatunnel:\n  engine:\n", settings)
		if err != nil {
			t.Fatalf("patch: %v", err)
		}
		if decodeEventReport(t, patched)["url"] != settings.URL {
			t.Fatalf("expected block under empty engine:\n%s", patched)
		}
		empty, err := PatchEventReportHTTP("", settings)
		if err != nil || decodeEventReport(t, empty)["url"] != settings.URL {
			t.Fatalf("expected block in empty document, err=%v\n%s", err, empty)
		}
	})

	t.Run("invalid yaml is rejected", func(t *testing.T) {
		if _, err := PatchEventReportHTTP("seatunnel: [unclosed", settings); err == nil {
			t.Fatalf("expected parse error")
		}
		if _, err := PatchEventReportHTTP("seatunnel:\n  engine: 5\n", settings); err == nil {
			t.Fatalf("expected non-mapping engine error")
		}
	})
}

func TestApplyEventReportConfig_patchesTemplateAndNodesIndividually(t *testing.T) {
	svc, db, agent := newNodeSyncTestService(t)
	nodes := seedNodeSyncConfigs(t, db, 21, 22)
	agent.offline[22] = true
	ctx := t.Context()
	settings := &EventReportSettings{URL: "http://stx/api/v1/sync/engine-events/1", Headers: map[string]string{"Authorization": "Bearer t"}, ReportNonTerminal: true}

	result, err := svc.ApplyEventReportConfig(ctx, 1, settings, 7)
	if err != nil {
		t.Fatalf("ApplyEventReportConfig: %v", err)
	}
	if len(result.Configs) != 3 {
		t.Fatalf("expected template + 2 nodes, got %+v", result.Configs)
	}
	for _, item := range result.Configs {
		switch {
		case item.Template:
			if !item.Changed || item.Pushed || item.Error != "" {
				t.Fatalf("unexpected template outcome: %+v", item)
			}
		case *item.HostID == 21:
			if !item.Pushed || item.Pending {
				t.Fatalf("online node should be pushed: %+v", item)
			}
		case *item.HostID == 22:
			if !item.Pending || item.Error == "" {
				t.Fatalf("offline node should be pending: %+v", item)
			}
		}
	}
	// 节点各自的内容（backup-count: 1）被保留，没有被模板（backup-count: 2）覆盖。
	// Node-specific content (backup-count: 1) survives instead of being overwritten by the template (backup-count: 2).
	online := reloadConfig(t, db, nodes[21].ID)
	if !strings.Contains(online.Content.String(), "backup-count: 1") || decodeEventReport(t, online.Content.String())["url"] != settings.URL {
		t.Fatalf("node content not patched in place:\n%s", online.Content)
	}
	if len(agent.pushed[21]) != 1 {
		t.Fatalf("expected one push to host 21, got %v", agent.pushed[21])
	}

	// 关闭时移除配置块；离线节点基于待推送内容继续打补丁。
	// Disabling removes the block; the offline node is patched on top of its pending content.
	if _, err := svc.ApplyEventReportConfig(ctx, 1, nil, 7); err != nil {
		t.Fatalf("disable: %v", err)
	}
	online = reloadConfig(t, db, nodes[21].ID)
	if strings.Contains(online.Content.String(), "event-report-http") {
		t.Fatalf("block should be removed:\n%s", online.Content)
	}
	offline := reloadConfig(t, db, nodes[22].ID)
	if offline.HasPendingPush() && strings.Contains(offline.PendingContent.String(), "event-report-http") {
		t.Fatalf("pending content should no longer enable reporting:\n%s", offline.PendingContent)
	}
}

func TestApplyEventReportConfig_missingConfigs(t *testing.T) {
	svc, _, _ := newNodeSyncTestService(t)
	if _, err := svc.ApplyEventReportConfig(t.Context(), 99, &EventReportSettings{URL: "http://x"}, 1); err != ErrSeatunnelConfigMissing {
		t.Fatalf("expected ErrSeatunnelConfigMissing, got %v", err)
	}
}

// TestApplyEventReportConfig_k8sConfigMapWriteBack 验证 K8s 集群走 ConfigMap 直写：
// 开启写入 event-report-http、关闭移除干净、缺失 seatunnel.yaml 报错、不触碰库表配置。
// TestApplyEventReportConfig_k8sConfigMapWriteBack verifies Kubernetes clusters write
// directly into the ConfigMap: enabling writes event-report-http, disabling removes it
// cleanly, a missing seatunnel.yaml errors out and the database tables stay untouched.
func TestApplyEventReportConfig_k8sConfigMapWriteBack(t *testing.T) {
	svc, _, _ := newNodeSyncTestService(t)
	source := &fakeK8sEventConfigSource{files: map[string]string{
		"seatunnel.yaml": "seatunnel:\n  engine:\n    history-job-expire-minutes: 1440\n",
	}}
	svc.SetK8sConfigMapSource(source)

	settings := &EventReportSettings{
		URL:               "http://10.0.2.2:17800/api/v1/sync/engine-events/5",
		Headers:           map[string]string{"Authorization": "Bearer token"},
		ReportNonTerminal: true,
	}
	result, err := svc.ApplyEventReportConfig(context.Background(), 5, settings, 1)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(result.Configs) != 1 || !result.Configs[0].Changed || !result.Configs[0].Pushed {
		t.Fatalf("unexpected result: %+v", result.Configs)
	}
	written := source.files["seatunnel.yaml"]
	if !strings.Contains(written, "event-report-http") || !strings.Contains(written, settings.URL) {
		t.Fatalf("configmap not patched:\n%s", written)
	}
	if !strings.Contains(written, "history-job-expire-minutes") {
		t.Fatalf("unrelated content must be preserved:\n%s", written)
	}

	// 关闭移除干净 / disabling removes the block cleanly
	result, err = svc.ApplyEventReportConfig(context.Background(), 5, nil, 1)
	if err != nil {
		t.Fatalf("disable: %v", err)
	}
	written = source.files["seatunnel.yaml"]
	if strings.Contains(written, "event-report-http") {
		t.Fatalf("block must be removed:\n%s", written)
	}

	// 无变化时 Changed=false 且不产生写入 / no-op writes report unchanged
	result, err = svc.ApplyEventReportConfig(context.Background(), 5, nil, 1)
	if err != nil {
		t.Fatalf("noop: %v", err)
	}
	if result.Configs[0].Changed {
		t.Fatalf("no-op must report unchanged")
	}

	// 缺 seatunnel.yaml 报错 / missing file errors out
	source.files = map[string]string{"log4j2.properties": "x"}
	if _, err := svc.ApplyEventReportConfig(context.Background(), 5, settings, 1); !errors.Is(err, ErrSeatunnelConfigMissing) {
		t.Fatalf("expected ErrSeatunnelConfigMissing, got %v", err)
	}
}

// fakeK8sEventConfigSource 实现 K8sConfigMapSource 的内存版（写回直接改 map）。
// fakeK8sEventConfigSource is an in-memory K8sConfigMapSource (writes mutate the map).
type fakeK8sEventConfigSource struct {
	files map[string]string
}

func (f *fakeK8sEventConfigSource) IsKubernetesCluster(_ context.Context, _ uint) bool { return true }

func (f *fakeK8sEventConfigSource) ReadK8sConfigMapByID(_ context.Context, _ uint) (map[string]string, error) {
	return f.files, nil
}

func (f *fakeK8sEventConfigSource) WriteK8sConfigMapFileByID(_ context.Context, _ uint, fileName, content string) error {
	f.files[fileName] = content
	return nil
}
