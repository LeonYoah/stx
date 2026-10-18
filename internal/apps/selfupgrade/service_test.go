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

package selfupgrade

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	dir := t.TempDir()
	s := NewService(filepath.Join(dir, "data"))
	s.mockMode = true
	s.allowSim = true
	return s
}

func TestCheckAlreadyLatest(t *testing.T) {
	s := newTestService(t)
	result, err := s.Check(context.Background(), CheckRequest{SimulateFailure: CodeAlreadyLatest})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if result.UpdateAvailable {
		t.Fatalf("expected no update")
	}
	if result.ErrorCode != CodeAlreadyLatest {
		t.Fatalf("code=%s", result.ErrorCode)
	}
}

func TestCheckFail(t *testing.T) {
	s := newTestService(t)
	_, err := s.Check(context.Background(), CheckRequest{SimulateFailure: CodeCheckFail})
	if !errors.Is(err, ErrCheckFail) {
		t.Fatalf("want check_fail, got %v", err)
	}
}

func TestDownloadPartial(t *testing.T) {
	s := newTestService(t)
	if _, err := s.Check(context.Background(), CheckRequest{}); err != nil {
		t.Fatal(err)
	}
	task, err := s.StartDownload(context.Background(), StageDownloadRequest{SimulateFailure: CodeDownloadPartial})
	if err != nil {
		t.Fatal(err)
	}
	waitTask(t, s, task.ID, StatusFailed, 5*time.Second)
	got, _ := s.GetTask(task.ID)
	if got.ErrorCode != CodeDownloadPartial {
		t.Fatalf("code=%s", got.ErrorCode)
	}
}

func TestBadOfflineImport(t *testing.T) {
	s := newTestService(t)
	if _, err := s.Check(context.Background(), CheckRequest{}); err != nil {
		t.Fatal(err)
	}
	_, err := s.ImportOffline("bad.bin", bytes.NewReader([]byte("x")), CodeBadOfflineImport)
	if !errors.Is(err, ErrBadOfflineImport) {
		t.Fatalf("got %v", err)
	}
}

func TestExecuteStepFailAndHappy(t *testing.T) {
	s := newTestService(t)
	if _, err := s.Check(context.Background(), CheckRequest{}); err != nil {
		t.Fatal(err)
	}
	task, err := s.StartDownload(context.Background(), StageDownloadRequest{})
	if err != nil {
		t.Fatal(err)
	}
	waitTask(t, s, task.ID, StatusStaged, 5*time.Second)

	failTask, err := s.Execute(context.Background(), ExecuteRequest{SimulateFailure: CodeStepFailAgent})
	if err != nil {
		t.Fatal(err)
	}
	waitTask(t, s, failTask.ID, StatusFailed, 5*time.Second)
	got, _ := s.GetTask(failTask.ID)
	if got.FailureStep != StepAgent {
		t.Fatalf("failure_step=%s", got.FailureStep)
	}

	// Restage and succeed.
	s2 := newTestService(t)
	if _, err := s2.Check(context.Background(), CheckRequest{}); err != nil {
		t.Fatal(err)
	}
	st, err := s2.StartDownload(context.Background(), StageDownloadRequest{})
	if err != nil {
		t.Fatal(err)
	}
	waitTask(t, s2, st.ID, StatusStaged, 5*time.Second)
	ok, err := s2.Execute(context.Background(), ExecuteRequest{})
	if err != nil {
		t.Fatal(err)
	}
	waitTask(t, s2, ok.ID, StatusSucceeded, 5*time.Second)
}

func TestConcurrentAndDiskFull(t *testing.T) {
	s := newTestService(t)
	if _, err := s.Check(context.Background(), CheckRequest{}); err != nil {
		t.Fatal(err)
	}
	_, err := s.StartDownload(context.Background(), StageDownloadRequest{SimulateFailure: CodeDiskFull})
	if !errors.Is(err, ErrDiskFull) {
		t.Fatalf("got %v", err)
	}
	_, err = s.StartDownload(context.Background(), StageDownloadRequest{SimulateFailure: CodeConcurrentUpgrade})
	if !errors.Is(err, ErrConcurrentUpgrade) {
		t.Fatalf("got %v", err)
	}
}

func TestExecuteRequiresStage(t *testing.T) {
	s := newTestService(t)
	_, err := s.Execute(context.Background(), ExecuteRequest{})
	if !errors.Is(err, ErrNotStaged) {
		t.Fatalf("got %v", err)
	}
}

// TestStatusDeploymentType 验证自升级状态包含有效的部署环境类型及系统架构。
// TestStatusDeploymentType verifies self-upgrade status reports valid deployment type and architecture.
func TestStatusDeploymentType(t *testing.T) {
	s := newTestService(t)
	status := s.Status()
	if status.DeploymentType != "host" && status.DeploymentType != "docker" {
		t.Fatalf("expected host or docker, got %s", status.DeploymentType)
	}
	if status.Architecture == "" {
		t.Fatalf("expected non-empty architecture")
	}
}

func waitTask(t *testing.T, s *Service, id, want string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		task, err := s.GetTask(id)
		if err == nil && task.Status == want {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	task, _ := s.GetTask(id)
	status := ""
	if task != nil {
		status = task.Status
	}
	t.Fatalf("timeout waiting for %s, last=%s", want, status)
}
