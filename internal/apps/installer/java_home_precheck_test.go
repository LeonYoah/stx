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
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

type javaPrecheckHostProvider struct{ host *HostInfo }

func (p *javaPrecheckHostProvider) GetHostByID(context.Context, uint) (*HostInfo, error) {
	return p.host, nil
}

// javaPrecheckAgent 模拟 Agent：端口空闲、目录可写，check_java 返回预设结果并记录参数。
// javaPrecheckAgent fakes the Agent: ports free, directory writable, check_java returns a preset result and
// records its parameters.
type javaPrecheckAgent struct {
	AgentManager
	javaSuccess bool
	javaDetails map[string]string
	javaParams  []map[string]string
}

func (a *javaPrecheckAgent) SendCommand(_ context.Context, _ string, commandType string, params map[string]string) (bool, string, error) {
	switch commandType {
	case "check_port":
		return false, "", nil
	case "check_directory":
		return true, "", nil
	case "check_java":
		copied := make(map[string]string, len(params))
		for k, v := range params {
			copied[k] = v
		}
		a.javaParams = append(a.javaParams, copied)
		payload, _ := json.Marshal(map[string]any{"success": a.javaSuccess, "message": "java checked", "details": a.javaDetails})
		return a.javaSuccess, string(payload), nil
	}
	return false, "", nil
}

func newJavaPrecheckService(t *testing.T, agent *javaPrecheckAgent) *Service {
	t.Helper()
	now := time.Now()
	service := NewService(t.TempDir(), agent)
	service.SetHostProvider(&javaPrecheckHostProvider{host: &HostInfo{ID: 10, Name: "node-a", AgentID: "agent-a", AgentStatus: "installed", LastSeen: &now}})
	return service
}

func javaItemOf(t *testing.T, result *PrecheckResult) PrecheckItem {
	t.Helper()
	for _, item := range result.Items {
		if item.Name == "java" {
			return item
		}
	}
	t.Fatalf("java item missing: %+v", result.Items)
	return PrecheckItem{}
}

// TestRunPrecheckCustomJavaHome 覆盖 AC2/AC3/AC1c：指定路径时只检查该 JDK，结果带实际路径与版本，失败阻断并点名节点与路径。
// TestRunPrecheckCustomJavaHome covers AC2/AC3/AC1c: with a path only that JDK is checked, the result carries the
// actual path and version, and failures block naming the node and path.
func TestRunPrecheckCustomJavaHome(t *testing.T) {
	cases := []struct {
		name       string
		success    bool
		details    map[string]string
		wantStatus CheckStatus
		wantText   []string
	}{
		{name: "jdk8 passes", success: true, details: map[string]string{"java_home_source": "custom", "java_home": "/opt/jdk8", "java_bin": "/opt/jdk8/bin/java", "installed_version": "8", "version_string": "1.8.0_392"},
			wantStatus: CheckStatusPassed, wantText: []string{"node-a", "/opt/jdk8/bin/java"}},
		{name: "jdk17 warns", success: true, details: map[string]string{"java_home_source": "custom", "java_home": "/opt/jdk8", "java_bin": "/opt/jdk8/bin/java", "installed_version": "17"},
			wantStatus: CheckStatusWarning, wantText: []string{"8 或 11"}},
		{name: "missing path blocks", success: false, details: map[string]string{"java_home_source": "custom", "java_home": "/opt/jdk8", "java_bin": "/opt/jdk8/bin/java", "error_reason": "not_found"},
			wantStatus: CheckStatusFailed, wantText: []string{"node-a", "/opt/jdk8/bin/java"}},
		{name: "old agent blocks", success: true, details: map[string]string{"installed_version": "8"},
			wantStatus: CheckStatusFailed, wantText: []string{"node-a", "Agent"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			agent := &javaPrecheckAgent{javaSuccess: tc.success, javaDetails: tc.details}
			result, err := newJavaPrecheckService(t, agent).RunPrecheck(context.Background(), 10, &PrecheckRequest{InstallDir: "/opt/seatunnel", JavaHome: "/opt/jdk8/"})
			if err != nil {
				t.Fatalf("RunPrecheck error: %v", err)
			}
			item := javaItemOf(t, result)
			if item.Status != tc.wantStatus {
				t.Fatalf("java status = %s, want %s (%s)", item.Status, tc.wantStatus, item.Message)
			}
			for _, text := range tc.wantText {
				if !strings.Contains(item.Message, text) {
					t.Fatalf("message %q should contain %q", item.Message, text)
				}
			}
			if tc.wantStatus == CheckStatusFailed && result.OverallStatus != CheckStatusFailed {
				t.Fatalf("overall = %s, want failed (blocking)", result.OverallStatus)
			}
			if len(agent.javaParams) != 1 || agent.javaParams[0]["java_home"] != "/opt/jdk8" {
				t.Fatalf("check_java params = %v, want normalized java_home", agent.javaParams)
			}
			if item.Details["java_home_source"] != "custom" {
				t.Fatalf("details source = %v", item.Details["java_home_source"])
			}
		})
	}
}

// TestRunPrecheckRejectsRelativeJavaHome：非法路径不下发到 Agent 并阻断。
// TestRunPrecheckRejectsRelativeJavaHome: an invalid path never reaches the Agent and blocks.
func TestRunPrecheckRejectsRelativeJavaHome(t *testing.T) {
	agent := &javaPrecheckAgent{}
	result, err := newJavaPrecheckService(t, agent).RunPrecheck(context.Background(), 10, &PrecheckRequest{JavaHome: "jdk8"})
	if err != nil {
		t.Fatalf("RunPrecheck error: %v", err)
	}
	if item := javaItemOf(t, result); item.Status != CheckStatusFailed || item.Details["error_reason"] != "invalid_path" {
		t.Fatalf("item = %+v, want invalid_path failure", item)
	}
	if len(agent.javaParams) != 0 {
		t.Fatalf("invalid path must not be sent to the Agent: %v", agent.javaParams)
	}
}

// TestRunPrecheckEmptyJavaHomeKeepsDetection 覆盖 AC6：未指定路径时沿用自动探测（显式空 java_home），结果与原来一致。
// TestRunPrecheckEmptyJavaHomeKeepsDetection covers AC6: without a path auto detection is kept (explicit empty
// java_home) and the result matches the original behavior.
func TestRunPrecheckEmptyJavaHomeKeepsDetection(t *testing.T) {
	agent := &javaPrecheckAgent{javaSuccess: true, javaDetails: map[string]string{"installed_version": "17", "java_home_source": "detected", "java_home": "/usr/lib/jvm/jdk17", "java_bin": "/usr/lib/jvm/jdk17/bin/java"}}
	result, err := newJavaPrecheckService(t, agent).RunPrecheck(context.Background(), 10, &PrecheckRequest{InstallDir: "/opt/seatunnel"})
	if err != nil {
		t.Fatalf("RunPrecheck error: %v", err)
	}
	item := javaItemOf(t, result)
	if item.Status == CheckStatusFailed {
		t.Fatalf("auto detection with Java 17 must not fail: %+v", item)
	}
	// 探测到的路径只用于前端预填（AC1b）。/ The detected path is only used for frontend prefill (AC1b).
	if item.Details["java_home"] != "/usr/lib/jvm/jdk17" || item.Details["java_home_source"] != "detected" {
		t.Fatalf("detected path should be reported for prefill: %+v", item.Details)
	}
	if len(agent.javaParams) != 1 {
		t.Fatalf("check_java calls = %d", len(agent.javaParams))
	}
	if value, ok := agent.javaParams[0]["java_home"]; !ok || value != "" {
		t.Fatalf("auto precheck must send an explicit empty java_home, got %v", agent.javaParams[0])
	}
}

// TestBuildInstallParamsCarriesJavaHome 覆盖 AC4：安装命令始终带 java_home 键（为空即自动探测）。
// TestBuildInstallParamsCarriesJavaHome covers AC4: the install command always carries the java_home key (empty
// means auto detection).
func TestBuildInstallParamsCarriesJavaHome(t *testing.T) {
	for _, tc := range []struct{ in, want string }{{"/opt/jdk8/", "/opt/jdk8"}, {"", ""}} {
		params := buildInstallParams(&InstallationRequest{Version: "2.3.12", InstallDir: "/opt/seatunnel", JavaHome: tc.in})
		value, ok := params["java_home"]
		if !ok || value != tc.want {
			t.Fatalf("java_home = %q (present=%v), want %q", value, ok, tc.want)
		}
	}
}

// TestStartClusterAfterInstallPersistsJavaHome 覆盖 AC4c：安装完成登记节点时把节点 JDK 交给唯一来源 cluster_nodes.java_home。
// TestStartClusterAfterInstallPersistsJavaHome covers AC4c: registering the node after install hands the node JDK
// to the single source cluster_nodes.java_home.
func TestStartClusterAfterInstallPersistsJavaHome(t *testing.T) {
	service := NewService(t.TempDir(), nil)
	starter := &stubInstallationNodeStarter{startOK: true, startMsg: "started"}
	service.SetNodeStarter(starter)
	service.startClusterAfterInstall(context.Background(), "agent-test", &InstallationRequest{
		HostID: "10", ClusterID: "9", NodeRole: NodeRoleMasterWorker, InstallDir: "/opt/seatunnel",
		ClusterPort: 5801, WorkerPort: 5802, HTTPPort: 8080, JavaHome: "/opt/jdk8",
	}, &InstallationStatus{Status: StepStatusRunning})
	if starter.ensureCalls != 1 || starter.ensureJavaHome != "/opt/jdk8" {
		t.Fatalf("ensure calls=%d java_home=%q, want 1 and /opt/jdk8", starter.ensureCalls, starter.ensureJavaHome)
	}
}
