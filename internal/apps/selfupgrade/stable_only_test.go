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
	"context"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	stxversion "github.com/LeonYoah/stx/internal/version"
)

// releaseAPIStub 只应答 GitHub releases/latest，并记录所有请求路径（用于断言不再请求 releases 列表）。
// releaseAPIStub answers only GitHub releases/latest and records every request path
// (to assert the releases list endpoint is no longer used).
type releaseAPIStub struct {
	mu    sync.Mutex
	tag   string
	paths []string
}

func (r *releaseAPIStub) RoundTrip(req *http.Request) (*http.Response, error) {
	r.mu.Lock()
	r.paths = append(r.paths, req.URL.Host+req.URL.RequestURI())
	r.mu.Unlock()
	status, body := http.StatusNotFound, `{"message":"Not Found"}`
	if req.URL.Host == "api.github.com" && req.URL.Path == "/repos/LeonYoah/stx/releases/latest" {
		status = http.StatusOK
		body = `{"tag_name":"v` + r.tag + `","html_url":"https://github.com/LeonYoah/stx/releases/tag/v` + r.tag + `"}`
	}
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}, nil
}

func newStableOnlyService(t *testing.T, current, remoteLatest string) (*Service, *releaseAPIStub) {
	t.Helper()
	t.Setenv("STX_GITHUB_OWNER", "LeonYoah")
	t.Setenv("STX_GITHUB_REPO", "stx")
	old := stxversion.Version
	stxversion.Version = current
	t.Cleanup(func() { stxversion.Version = old })

	s := NewService(filepath.Join(t.TempDir(), "data"))
	s.mockMode = false
	stub := &releaseAPIStub{tag: remoteLatest}
	s.httpClient = &http.Client{Transport: stub}
	return s, stub
}

func TestCheckFollowsStableReleasesOnly(t *testing.T) {
	cases := []struct {
		name          string
		current       string
		remoteLatest  string
		wantAvailable bool
		wantLatest    string
	}{
		{name: "stable current, same stable latest", current: "1.1.1", remoteLatest: "1.1.1", wantAvailable: false, wantLatest: "1.1.1"},
		{name: "stable current, pre-release mis-marked as latest is ignored", current: "1.1.1", remoteLatest: "1.1.2-beta.1", wantAvailable: false, wantLatest: "1.1.1"},
		{name: "pre-release current, newer stable latest prompts", current: "1.1.2-beta.1", remoteLatest: "1.1.2", wantAvailable: true, wantLatest: "1.1.2"},
		{name: "stable current, newer stable latest prompts", current: "1.1.1", remoteLatest: "1.1.2", wantAvailable: true, wantLatest: "1.1.2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, stub := newStableOnlyService(t, tc.current, tc.remoteLatest)
			result, err := s.Check(context.Background(), CheckRequest{})
			if err != nil {
				t.Fatalf("Check() error = %v (result=%+v)", err, result)
			}
			if result.UpdateAvailable != tc.wantAvailable {
				t.Fatalf("UpdateAvailable = %v, want %v (result=%+v)", result.UpdateAvailable, tc.wantAvailable, result)
			}
			if result.LatestVersion != tc.wantLatest {
				t.Fatalf("LatestVersion = %q, want %q", result.LatestVersion, tc.wantLatest)
			}
			if !tc.wantAvailable && result.ErrorCode != CodeAlreadyLatest {
				t.Fatalf("ErrorCode = %q, want %q", result.ErrorCode, CodeAlreadyLatest)
			}
			if strings.Contains(result.LatestVersion, "-") && result.LatestVersion != tc.current {
				t.Fatalf("pre-release %q must never be offered as latest", result.LatestVersion)
			}
			for _, p := range stub.paths {
				if strings.Contains(p, "/releases?") {
					t.Fatalf("releases list endpoint must not be queried, got %q", p)
				}
			}
			if len(stub.paths) != 1 || stub.paths[0] != "api.github.com/repos/LeonYoah/stx/releases/latest" {
				t.Fatalf("expected a single releases/latest request, got %v", stub.paths)
			}
		})
	}
}

func TestCheckForceLatestPreReleaseStillAllowed(t *testing.T) {
	s, stub := newStableOnlyService(t, "1.1.1", "1.1.1")
	result, err := s.Check(context.Background(), CheckRequest{ForceLatest: "v1.2.0-beta.1"})
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !result.UpdateAvailable || result.LatestVersion != "1.2.0-beta.1" {
		t.Fatalf("explicit ForceLatest pre-release should be offered, got %+v", result)
	}
	if len(stub.paths) != 0 {
		t.Fatalf("ForceLatest must not query GitHub, got %v", stub.paths)
	}
}
