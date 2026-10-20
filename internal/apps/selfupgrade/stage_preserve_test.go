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
	"fmt"
	"os"
	"testing"
	"time"

	stxversion "github.com/LeonYoah/stx/internal/version"
)

// bumpPatch 返回比当前版本高 n 个补丁号的版本。
// bumpPatch returns a version n patch levels above the current one.
func bumpPatch(t *testing.T, n int) string {
	t.Helper()
	var major, minor, patch int
	if _, err := fmtSscanf(stxversion.Current().Version, &major, &minor, &patch); err != nil {
		t.Fatalf("parse current version: %v", err)
	}
	return versionString(major, minor, patch+n)
}

func assertStagedUsable(t *testing.T, s *Service, wantVersion string) {
	t.Helper()
	st := s.Status()
	if st.Stage.Status != StatusStaged {
		t.Fatalf("stage status = %s, want staged", st.Stage.Status)
	}
	if st.Stage.TargetVersion != wantVersion {
		t.Fatalf("staged version = %s, want %s", st.Stage.TargetVersion, wantVersion)
	}
	if st.OverallStatus != StatusStaged {
		t.Fatalf("overall status = %s, want staged", st.OverallStatus)
	}
	for _, asset := range st.Stage.Assets {
		if asset.Status != AssetReady {
			t.Fatalf("asset %s status = %s, want ready", asset.ID, asset.Status)
		}
		if _, err := os.Stat(asset.Path); err != nil {
			t.Fatalf("asset %s missing on disk: %v", asset.ID, err)
		}
	}
}

func TestRecheckSameVersionKeepsOnlineStagedAssets(t *testing.T) {
	s := newTestService(t)
	first, err := s.Check(context.Background(), CheckRequest{})
	if err != nil {
		t.Fatal(err)
	}
	task, err := s.StartDownload(context.Background(), StageDownloadRequest{})
	if err != nil {
		t.Fatal(err)
	}
	waitTask(t, s, task.ID, StatusStaged, 5*time.Second)
	assertStagedUsable(t, s, first.LatestVersion)

	result, err := s.Check(context.Background(), CheckRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if !result.StagePreserved || result.StagedVersion != first.LatestVersion {
		t.Fatalf("check result = %+v, want preserved staged %s", result, first.LatestVersion)
	}
	assertStagedUsable(t, s, first.LatestVersion)

	exec, err := s.Execute(context.Background(), ExecuteRequest{})
	if err != nil {
		t.Fatalf("execute after re-check: %v", err)
	}
	if exec.TargetVersion != first.LatestVersion {
		t.Fatalf("execute target = %s", exec.TargetVersion)
	}
	waitTask(t, s, exec.ID, StatusSucceeded, 10*time.Second)
}

func TestOfflineImportSurvivesCheckWithDifferentLatest(t *testing.T) {
	s := newTestService(t)
	if _, err := s.Check(context.Background(), CheckRequest{}); err != nil {
		t.Fatal(err)
	}
	imported, err := s.ImportOffline("stx-upgrade.tar.gz", bytes.NewReader([]byte("offline")), "")
	if err != nil {
		t.Fatal(err)
	}
	stagedVersion := imported.TargetVersion

	newer := bumpPatch(t, 5)
	result, err := s.Check(context.Background(), CheckRequest{ForceLatest: newer})
	if err != nil {
		t.Fatal(err)
	}
	if !result.UpdateAvailable || result.LatestVersion != newer {
		t.Fatalf("check result = %+v, want newer %s available", result, newer)
	}
	if !result.StagePreserved || result.StagedVersion != stagedVersion {
		t.Fatalf("check result = %+v, want preserved staged %s", result, stagedVersion)
	}
	assertStagedUsable(t, s, stagedVersion)
	if st := s.Status(); st.LatestVersion != newer || st.Stage.Mode != ModeOffline {
		t.Fatalf("status latest=%s mode=%s", st.LatestVersion, st.Stage.Mode)
	}
	if _, err := s.Execute(context.Background(), ExecuteRequest{}); err != nil {
		t.Fatalf("execute offline staged package after check: %v", err)
	}
}

func TestNewerVersionAvailableDoesNotWipeStagedPackage(t *testing.T) {
	s := newTestService(t)
	staged := bumpPatch(t, 1)
	if _, err := s.Check(context.Background(), CheckRequest{ForceLatest: staged}); err != nil {
		t.Fatal(err)
	}
	task, err := s.StartDownload(context.Background(), StageDownloadRequest{})
	if err != nil {
		t.Fatal(err)
	}
	waitTask(t, s, task.ID, StatusStaged, 5*time.Second)

	newer := bumpPatch(t, 2)
	if _, err := s.Check(context.Background(), CheckRequest{ForceLatest: newer}); err != nil {
		t.Fatal(err)
	}
	assertStagedUsable(t, s, staged)

	// 显式重新预置才会替换 / only an explicit new stage replaces it
	task, err = s.StartDownload(context.Background(), StageDownloadRequest{})
	if err != nil {
		t.Fatal(err)
	}
	waitTask(t, s, task.ID, StatusStaged, 5*time.Second)
	assertStagedUsable(t, s, newer)
}

func TestCheckDuringStagingDoesNotResetStage(t *testing.T) {
	s := newTestService(t)
	if _, err := s.Check(context.Background(), CheckRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartDownload(context.Background(), StageDownloadRequest{}); err != nil {
		t.Fatal(err)
	}
	// Check 会被并发保护拒绝；即使绕过入口，协调逻辑也不能清理进行中的预置。
	// Check is rejected by the concurrency guard; even past it, reconciliation must keep in-flight staging.
	if _, err := s.Check(context.Background(), CheckRequest{}); err != ErrConcurrentUpgrade {
		t.Fatalf("check during staging err = %v, want concurrent_upgrade", err)
	}
	s.mu.Lock()
	preserved := s.reconcileStageAfterCheckLocked(stxversion.Current().Version, bumpPatch(t, 3), true)
	status := s.stage.Status
	s.mu.Unlock()
	if !preserved || status != StatusStaging {
		t.Fatalf("preserved=%v stage=%s, want staging kept", preserved, status)
	}
	_ = s.CancelStage()
}

func TestCheckResetsStageWhenStagedFilesMissing(t *testing.T) {
	s := newTestService(t)
	if _, err := s.Check(context.Background(), CheckRequest{}); err != nil {
		t.Fatal(err)
	}
	imported, err := s.ImportOffline("stx-upgrade.tar.gz", bytes.NewReader([]byte("offline")), "")
	if err != nil {
		t.Fatal(err)
	}
	for _, asset := range imported.Assets {
		_ = os.Remove(asset.Path)
	}
	result, err := s.Check(context.Background(), CheckRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if result.StagePreserved {
		t.Fatalf("missing staged files must not be preserved: %+v", result)
	}
	st := s.Status()
	if st.Stage.Status != StatusIdle || st.OverallStatus != StatusAvailable {
		t.Fatalf("stage=%s overall=%s, want idle/available", st.Stage.Status, st.OverallStatus)
	}
	if _, err := s.Execute(context.Background(), ExecuteRequest{}); err != ErrNotStaged {
		t.Fatalf("execute err = %v, want not_staged", err)
	}
}

func fmtSscanf(v string, major, minor, patch *int) (int, error) {
	return fmt.Sscanf(stxversion.Normalize(v), "%d.%d.%d", major, minor, patch)
}

func versionString(major, minor, patch int) string {
	return fmt.Sprintf("%d.%d.%d", major, minor, patch)
}
