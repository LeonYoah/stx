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

package regclient

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestBasicRegistry(t *testing.T, authUser, authPass string, repos map[string][]string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if authUser != "" {
			h := r.Header.Get("Authorization")
			want := "Basic " + base64.StdEncoding.EncodeToString([]byte(authUser+":"+authPass))
			if h != want {
				w.Header().Set("WWW-Authenticate", `Basic realm="test"`)
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
		}
		switch {
		case r.URL.Path == "/v2/":
			w.WriteHeader(http.StatusOK)
		case strings.HasPrefix(r.URL.Path, "/v2/_catalog"):
			names := make([]string, 0, len(repos))
			for name := range repos {
				names = append(names, name)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"repositories":["` + strings.Join(names, `","`) + `"]}`))
		case strings.HasSuffix(r.URL.Path, "/tags/list"):
			repo := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v2/"), "/tags/list")
			tags, ok := repos[repo]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"` + repo + `","tags":["` + strings.Join(tags, `","`) + `"]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func hostOf(server *httptest.Server) string {
	return strings.TrimPrefix(strings.TrimPrefix(server.URL, "http://"), "https://")
}

func TestPingAnonymousRegistry(t *testing.T) {
	server := newTestBasicRegistry(t, "", "", nil)
	defer server.Close()
	client := New(hostOf(server), "", "")
	if err := client.Ping(context.Background()); err != nil {
		t.Fatalf("ping: %v", err)
	}
	if client.Scheme() != "http" {
		t.Fatalf("scheme = %q, want http fallback", client.Scheme())
	}
}

func TestPingAuthRegistry(t *testing.T) {
	server := newTestBasicRegistry(t, "u", "p", nil)
	defer server.Close()
	host := hostOf(server)
	if err := New(host, "u", "wrong").Ping(context.Background()); !errors.Is(err, ErrAuthFailed) {
		t.Fatalf("expected ErrAuthFailed, got %v", err)
	}
	if err := New(host, "u", "p").Ping(context.Background()); err != nil {
		t.Fatalf("ping: %v", err)
	}
	if err := New(host, "", "").Ping(context.Background()); err != nil {
		t.Fatalf("anonymous ping should prove liveness: %v", err)
	}
	if err := New(host, "", "").PingStrict(context.Background()); !errors.Is(err, ErrAuthRequired) {
		t.Fatalf("anonymous strict ping expected ErrAuthRequired, got %v", err)
	}
	if err := New(host, "u", "p").PingStrict(context.Background()); err != nil {
		t.Fatalf("authenticated strict ping: %v", err)
	}
}

func TestCatalogAndTags(t *testing.T) {
	repos := map[string][]string{
		"seatunnel":                {"3.1.0", "3.1.1"},
		"data-team/seatunnel-prod": {"3.1.1-custom"},
	}
	server := newTestBasicRegistry(t, "u", "p", repos)
	defer server.Close()
	client := New(hostOf(server), "u", "p")
	got, err := client.Catalog(context.Background())
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("catalog len = %d, want 2: %v", len(got), got)
	}
	tags, err := client.Tags(context.Background(), "seatunnel")
	if err != nil {
		t.Fatalf("tags: %v", err)
	}
	if len(tags) != 2 || tags[0] != "3.1.0" {
		t.Fatalf("tags = %v", tags)
	}
	if _, err := client.Tags(context.Background(), "missing"); err == nil {
		t.Fatal("expected not found")
	}
	if _, err := New(hostOf(server), "", "").Catalog(context.Background()); !errors.Is(err, ErrAuthRequired) {
		t.Fatalf("expected ErrAuthRequired, got %v", err)
	}
}

// TestShortErrorPayloadNoPanic 验证短响应体（<200 字节）与空响应体绝不触发切片越界 panic。
// TestShortErrorPayloadNoPanic verifies short (<200 byte) or empty error responses never panic.
func TestShortErrorPayloadNoPanic(t *testing.T) {
	for _, body := range []string{"", "forbidden", "502 bad gateway"} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(body))
		}))
		err := New(hostOf(server), "", "").Ping(context.Background())
		server.Close()
		if err == nil {
			t.Fatalf("expected error for body %q", body)
		}
	}
}

// TestBearerTokenAuthFlowAndLinkPagination 验证 Harbor/ACR 风格的 Bearer Token 换取与 Link 头分页。
// TestBearerTokenAuthFlowAndLinkPagination verifies Harbor/ACR-style Bearer token exchange and Link pagination.
func TestBearerTokenAuthFlowAndLinkPagination(t *testing.T) {
	var authServer *httptest.Server
	authServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "robot" || pass != "secret-123" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		scope := r.URL.Query().Get("scope")
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"token":"tok:%s"}`, scope)
	}))
	defer authServer.Close()

	regServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		switch {
		case r.URL.Path == "/v2/":
			if auth == "" {
				w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="%s/service/token",service="harbor-registry"`, authServer.URL))
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			if !strings.HasPrefix(auth, "Bearer tok:") {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/v2/_catalog":
			if auth != "Bearer tok:registry:catalog:*" {
				w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="%s/service/token",service="harbor-registry",scope="registry:catalog:*"`, authServer.URL))
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			if r.URL.Query().Get("page") == "2" {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"repositories":["team/c"]}`))
				return
			}
			w.Header().Set("Link", `</v2/_catalog?page=2>; rel="next"`)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"repositories":["team/a","team/b"]}`))
		case r.URL.Path == "/v2/team/a/tags/list":
			if auth != "Bearer tok:repository:team/a:pull" {
				w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="%s/service/token",service="harbor-registry",scope="repository:team/a:pull"`, authServer.URL))
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			if r.URL.Query().Get("page") == "2" {
				_ = json.NewEncoder(w).Encode(map[string]any{"tags": []string{"3.1.1"}})
				return
			}
			w.Header().Set("Link", `</v2/team/a/tags/list?page=2>; rel="next"`)
			_ = json.NewEncoder(w).Encode(map[string]any{"tags": []string{"3.1.0"}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer regServer.Close()

	// 错误密码必须在 token endpoint 被拒并报 ErrAuthFailed。
	// Wrong password must fail at token exchange with ErrAuthFailed.
	if err := New(hostOf(regServer), "robot", "wrong").Ping(context.Background()); !errors.Is(err, ErrAuthFailed) {
		t.Fatalf("expected ErrAuthFailed, got %v", err)
	}

	client := New(hostOf(regServer), "robot", "secret-123")
	if err := client.Ping(context.Background()); err != nil {
		t.Fatalf("bearer ping: %v", err)
	}
	repos, err := client.Catalog(context.Background())
	if err != nil {
		t.Fatalf("bearer catalog: %v", err)
	}
	if len(repos) != 3 || repos[2] != "team/c" {
		t.Fatalf("unexpected paginated repos: %v", repos)
	}
	tags, err := client.Tags(context.Background(), "team/a")
	if err != nil {
		t.Fatalf("bearer tags: %v", err)
	}
	if len(tags) != 2 || tags[0] != "3.1.0" || tags[1] != "3.1.1" {
		t.Fatalf("unexpected paginated tags: %v", tags)
	}
}

func TestDockerHubAliasMapping(t *testing.T) {
	c := New("docker.io", "", "")
	if c.apiHost != dockerHubRegistryHost || !c.onlyHTTPS {
		t.Fatalf("expected docker.io to map to %s HTTPS-only, got host=%s onlyHTTPS=%v", dockerHubRegistryHost, c.apiHost, c.onlyHTTPS)
	}
}
