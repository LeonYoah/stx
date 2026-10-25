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

package stupgrade

import (
	"context"
	"testing"
	"time"

	clusterapp "github.com/LeonYoah/stx/internal/apps/cluster"
	installerapp "github.com/LeonYoah/stx/internal/apps/installer"
)

func TestBuildTargetInstallDir_DefaultsToSiblingOfSource(t *testing.T) {
	cases := []struct {
		name, source, from, to, want string
	}{
		{"versioned source dir", "/opt/seatunnel-2.3.11", "2.3.11", "2.3.12", "/opt/seatunnel-2.3.12"},
		{"unversioned source dir", "/opt/stx-node/seatunnel", "2.3.11", "3.0.0", "/opt/stx-node/seatunnel-3.0.0"},
		{"trailing slash", "/data/st/", "2.3.11", "2.3.12", "/data/st-2.3.12"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := buildTargetInstallDir(tc.source, tc.from, tc.to); got != tc.want {
				t.Fatalf("buildTargetInstallDir(%q) = %q, want %q", tc.source, got, tc.want)
			}
		})
	}
}

func newTargetDirPrecheckService(t *testing.T, sender *stubAgentCommandSender) *Service {
	t.Helper()
	service := newPlanningService(t, nil)
	packagePath := createTestPackage(t, map[string]string{"config/seatunnel.yaml": "seatunnel: default"})
	service.SetPackageProvider(&stubPackageProvider{info: &installerapp.PackageInfo{
		Version: "2.3.12", FileName: "apache-seatunnel-2.3.12-bin.tar.gz", IsLocal: true,
		LocalPath: packagePath, FileSize: 1024, Checksum: "abc123",
	}})
	service.SetAgentCommandSender(sender)
	return service
}

func TestService_RunPrecheck_blocksUnwritableTargetInstallDir(t *testing.T) {
	sender := &stubAgentCommandSender{
		agents:         map[uint]string{101: "agent-node-a"},
		unwritableDirs: map[string]bool{"/readonly/seatunnel-2.3.12": true},
	}
	service := newTargetDirPrecheckService(t, sender)

	result, err := service.RunPrecheck(context.Background(), &PrecheckRequest{
		ClusterID: 1, TargetVersion: "2.3.12", TargetInstallDir: "/readonly/seatunnel-2.3.12",
	})
	if err != nil {
		t.Fatalf("RunPrecheck returned error: %v", err)
	}
	if result.Ready {
		t.Fatal("expected precheck to block an unwritable target install dir")
	}
	assertIssueCode(t, result.Issues, "target_dir_not_writable")
}

func TestService_RunPrecheck_writableTargetInstallDirPasses(t *testing.T) {
	sender := &stubAgentCommandSender{agents: map[uint]string{101: "agent-node-a"}}
	service := newTargetDirPrecheckService(t, sender)

	result, err := service.RunPrecheck(context.Background(), &PrecheckRequest{ClusterID: 1, TargetVersion: "2.3.12"})
	if err != nil {
		t.Fatalf("RunPrecheck returned error: %v", err)
	}
	for _, issue := range result.Issues {
		if issue.Code == "target_dir_not_writable" {
			t.Fatalf("unexpected writability issue: %+v", issue)
		}
	}
	if countSubCommandInDir(sender.commands, "check_path_ready", "") != 1 {
		t.Fatalf("expected one check_path_ready probe, got %+v", sender.commands)
	}
	if got := sender.commands[0].params["path"]; got != "/opt/seatunnel-2.3.12" {
		t.Fatalf("expected the default target dir to be probed, got %q", got)
	}
}

type stubHealthReader struct {
	*stubClusterOperator
	statuses []clusterapp.HealthStatus
	calls    int
}

func (s *stubHealthReader) GetStatus(_ context.Context, clusterID uint) (*clusterapp.ClusterStatusInfo, error) {
	status := s.statuses[len(s.statuses)-1]
	if s.calls < len(s.statuses) {
		status = s.statuses[s.calls]
	}
	s.calls++
	return &clusterapp.ClusterStatusInfo{ClusterID: clusterID, HealthStatus: status, HealthReasons: []string{"node-b process not running"}}, nil
}

func TestWaitForClusterHealthy(t *testing.T) {
	service := NewService(nil)
	service.SetRuntimeHealthWait(10*time.Millisecond, time.Millisecond)
	service.waitForRetry = func(context.Context, time.Duration) error { return nil }

	reader := &stubHealthReader{stubClusterOperator: &stubClusterOperator{}, statuses: []clusterapp.HealthStatus{clusterapp.HealthStatusUnhealthy, clusterapp.HealthStatusHealthy}}
	service.SetClusterOperator(reader)
	if err := service.waitForClusterHealthy(context.Background(), 1); err != nil {
		t.Fatalf("expected healthy after retry, got %v", err)
	}

	reader = &stubHealthReader{stubClusterOperator: &stubClusterOperator{}, statuses: []clusterapp.HealthStatus{clusterapp.HealthStatusUnhealthy}}
	service.SetClusterOperator(reader)
	err := service.waitForClusterHealthy(context.Background(), 1)
	if err == nil || reader.calls != 10 {
		t.Fatalf("expected failure after 10 attempts, got err=%v calls=%d", err, reader.calls)
	}

	// 不支持健康读取的操作器直接跳过。Operators without health reading are skipped.
	service.SetClusterOperator(&stubClusterOperator{})
	if err := service.waitForClusterHealthy(context.Background(), 1); err != nil {
		t.Fatalf("expected skip, got %v", err)
	}
}
