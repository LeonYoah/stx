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
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

var maliciousTargetVersions = []string{
	"../../etc",
	"..",
	"v1.2.3/../../x",
	"/tmp/evil",
	`..\..\evil`,
	`1.2.3\evil`,
	"latest",
	"1.2",
}

func TestStartDownloadRejectsInvalidTargetVersion(t *testing.T) {
	for _, target := range maliciousTargetVersions {
		t.Run(target, func(t *testing.T) {
			s := newTestService(t)
			if _, err := s.Check(context.Background(), CheckRequest{}); err != nil {
				t.Fatal(err)
			}
			task, err := s.StartDownload(context.Background(), StageDownloadRequest{TargetVersion: target})
			if !errors.Is(err, ErrInvalidRequest) {
				t.Fatalf("StartDownload(%q) err = %v, want invalid_request", target, err)
			}
			if task != nil {
				t.Fatalf("StartDownload(%q) returned task %+v", target, task)
			}
			if ErrorCode(err) != CodeInvalidRequest || statusFor(err) != http.StatusBadRequest {
				t.Fatalf("code=%s status=%d", ErrorCode(err), statusFor(err))
			}
			if s.StageSnapshot().Status == StatusStaging {
				t.Fatalf("stage must not start for %q", target)
			}
		})
	}
}

func TestStartDownloadAcceptsValidTargetVersion(t *testing.T) {
	s := newTestService(t)
	if _, err := s.Check(context.Background(), CheckRequest{}); err != nil {
		t.Fatal(err)
	}
	task, err := s.StartDownload(context.Background(), StageDownloadRequest{TargetVersion: "v1.0.1-beta.1"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if task.TargetVersion != "v1.0.1-beta.1" {
		t.Fatalf("target=%s", task.TargetVersion)
	}
	_ = s.CancelStage()
}

func TestCheckRejectsInvalidForceLatest(t *testing.T) {
	for _, target := range maliciousTargetVersions {
		s := newTestService(t)
		_, err := s.Check(context.Background(), CheckRequest{ForceLatest: target})
		if !errors.Is(err, ErrInvalidRequest) {
			t.Fatalf("Check(force_latest=%q) err = %v, want invalid_request", target, err)
		}
		if s.Status().LatestVersion != "" {
			t.Fatalf("latest version must not be stored for %q", target)
		}
	}
}

func TestImportOfflineRejectsInvalidTargetVersion(t *testing.T) {
	s := newTestService(t)
	if _, err := s.Check(context.Background(), CheckRequest{}); err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.latestVersion = "../../escape"
	s.mu.Unlock()
	_, err := s.ImportOffline("stx-upgrade.tar.gz", bytes.NewReader([]byte("x")), "")
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("err = %v, want invalid_request", err)
	}
	if _, statErr := os.Stat(filepath.Join(filepath.Dir(s.stagingDir), "escape")); !os.IsNotExist(statErr) {
		t.Fatalf("escape directory must not be created: %v", statErr)
	}
}

func TestExecuteRejectsInvalidTargetVersion(t *testing.T) {
	s := newTestService(t)
	s.mu.Lock()
	s.stage = StageState{Status: StatusStaged, TargetVersion: "../../escape"}
	s.mu.Unlock()
	task, err := s.Execute(context.Background(), ExecuteRequest{})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("err = %v, want invalid_request", err)
	}
	if task != nil {
		t.Fatalf("unexpected task %+v", task)
	}
}

func TestApplyStepAndMaterializeRejectInvalidTargetVersion(t *testing.T) {
	s := newTestService(t)
	if err := s.applyStep("../../escape", StepVerify, false); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("applyStep err = %v", err)
	}
	if _, _, err := s.materializeAsset("../../escape", DefaultAssetCatalog()[0]); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("materializeAsset err = %v", err)
	}
}

func TestVersionStagingDirStaysUnderStagingDir(t *testing.T) {
	s := newTestService(t)
	dir, err := s.versionStagingDir("1.2.3-rc.1")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(s.stagingDir, "v1.2.3-rc.1"); dir != want {
		t.Fatalf("dir=%s want=%s", dir, want)
	}
	for _, target := range append([]string{""}, maliciousTargetVersions...) {
		if _, err := s.versionStagingDir(target); !errors.Is(err, ErrInvalidRequest) {
			t.Fatalf("versionStagingDir(%q) err = %v", target, err)
		}
	}
}

func TestHandlerStartDownloadInvalidTargetVersionReturnsBadRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := newTestService(t)
	if _, err := s.Check(context.Background(), CheckRequest{}); err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.POST("/stage/download", NewHandler(s).StartDownload)
	req := httptest.NewRequest(http.MethodPost, "/stage/download", strings.NewReader(`{"target_version":"../../etc"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.ErrorCode != CodeInvalidRequest {
		t.Fatalf("error_code=%s", resp.ErrorCode)
	}
}
