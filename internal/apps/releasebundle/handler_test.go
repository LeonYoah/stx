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

package releasebundle

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func setupTestRouter(handler *Handler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/v1/stx/install.sh", handler.GetInstallScript)
	r.GET("/api/v1/stx/download", handler.DownloadBundle)
	return r
}

func TestGetInstallScriptUsesRequestHost(t *testing.T) {
	dir := t.TempDir()
	bundleName := "stx-test-linux-amd64-node18-glibc217-without-observability.tar.gz"
	if err := os.WriteFile(filepath.Join(dir, bundleName), []byte("bundle"), 0o644); err != nil {
		t.Fatalf("write bundle: %v", err)
	}

	handler := NewHandler(&HandlerConfig{
		ReleaseDir:    dir,
		BundlePattern: "stx-*.tar.gz",
	})
	router := setupTestRouter(handler)

	req, _ := http.NewRequest("GET", "/api/v1/stx/install.sh", nil)
	req.Host = "cpa.120500.xyz"
	req.Header.Set("X-Forwarded-Proto", "https")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "https://cpa.120500.xyz/api/v1/stx/download") {
		t.Fatalf("expected install script to contain request host based download url, got: %s", body)
	}
	if !strings.Contains(body, "STX_USERNAME") || !strings.Contains(body, "STX username:") {
		t.Fatalf("expected install script to prompt for credentials, got: %s", body)
	}
	if got := w.Header().Get("X-STX-Bundle"); got != bundleName {
		t.Fatalf("expected bundle header %q, got %q", bundleName, got)
	}
}

func TestDownloadBundleRequiresAuth(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "stx-test-linux-amd64-node18-glibc217-without-observability.tar.gz"), []byte("bundle"), 0o644); err != nil {
		t.Fatalf("write bundle: %v", err)
	}

	handler := NewHandler(&HandlerConfig{
		ReleaseDir:    dir,
		BundlePattern: "stx-*.tar.gz",
		ValidateCredentials: func(ctx context.Context, username, password string) (bool, error) {
			return username == "admin" && password == "secret", nil
		},
	})
	router := setupTestRouter(handler)

	req, _ := http.NewRequest("GET", "/api/v1/stx/download", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
	if got := w.Header().Get("WWW-Authenticate"); !strings.Contains(got, "Basic") {
		t.Fatalf("expected basic auth challenge, got %q", got)
	}
}

func TestDownloadBundleReturnsLatestMatchingBundle(t *testing.T) {
	dir := t.TempDir()
	older := filepath.Join(dir, "stx-older-linux-amd64-node18-glibc217-without-observability.tar.gz")
	newer := filepath.Join(dir, "stx-newer-linux-amd64-node18-glibc217-without-observability.tar.gz")
	if err := os.WriteFile(older, []byte("older"), 0o644); err != nil {
		t.Fatalf("write older bundle: %v", err)
	}
	if err := os.WriteFile(newer, []byte("newer"), 0o644); err != nil {
		t.Fatalf("write newer bundle: %v", err)
	}
	now := time.Now()
	if err := os.Chtimes(older, now.Add(-time.Hour), now.Add(-time.Hour)); err != nil {
		t.Fatalf("chtimes older: %v", err)
	}
	if err := os.Chtimes(newer, now, now); err != nil {
		t.Fatalf("chtimes newer: %v", err)
	}

	handler := NewHandler(&HandlerConfig{
		ReleaseDir:    dir,
		BundlePattern: "stx-*.tar.gz",
		ValidateCredentials: func(ctx context.Context, username, password string) (bool, error) {
			return username == "admin" && password == "secret", nil
		},
	})
	router := setupTestRouter(handler)

	req, _ := http.NewRequest("GET", "/api/v1/stx/download", nil)
	req.SetBasicAuth("admin", "secret")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if got := w.Header().Get("Content-Disposition"); !strings.Contains(got, filepath.Base(newer)) {
		t.Fatalf("expected content disposition to contain latest bundle name, got %q", got)
	}
	if w.Body.String() != "newer" {
		t.Fatalf("expected latest bundle content, got %q", w.Body.String())
	}
}

func TestDownloadBundleReturnsNotFoundWhenMissing(t *testing.T) {
	handler := NewHandler(&HandlerConfig{
		ReleaseDir:    t.TempDir(),
		BundlePattern: "stx-*.tar.gz",
		ValidateCredentials: func(ctx context.Context, username, password string) (bool, error) {
			return username == "admin" && password == "secret", nil
		},
	})
	router := setupTestRouter(handler)

	req, _ := http.NewRequest("GET", "/api/v1/stx/download", nil)
	req.SetBasicAuth("admin", "secret")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "release bundle not found") {
		t.Fatalf("expected not found message, got %s", w.Body.String())
	}
}
