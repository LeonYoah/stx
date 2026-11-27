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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestRegistry(t *testing.T, authUser, authPass string, repos map[string][]string) *httptest.Server {
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
	return strings.TrimPrefix(server.URL, "http://")
}

func TestPingAnonymousRegistry(t *testing.T) {
	server := newTestRegistry(t, "", "", nil)
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
	server := newTestRegistry(t, "u", "p", nil)
	defer server.Close()
	host := hostOf(server)
	// 错误凭据必须报错。/ wrong credentials must fail.
	if err := New(host, "u", "wrong").Ping(context.Background()); err == nil {
		t.Fatal("expected auth failure")
	}
	// 正确凭据通过。/ correct credentials pass.
	if err := New(host, "u", "p").Ping(context.Background()); err != nil {
		t.Fatalf("ping: %v", err)
	}
	// 匿名 ping 命中 401 仍算仓库在线。/ anonymous ping hitting 401 still means alive.
	if err := New(host, "", "").Ping(context.Background()); err != nil {
		t.Fatalf("anonymous ping should prove liveness: %v", err)
	}
}

func TestCatalogAndTags(t *testing.T) {
	repos := map[string][]string{
		"seatunnel":                {"3.1.0", "3.1.1"},
		"data-team/seatunnel-prod": {"3.1.1-custom"},
	}
	server := newTestRegistry(t, "u", "p", repos)
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
	// 匿名 catalog 命中 401 必须报错而非静默。/ anonymous catalog on auth registry must error.
	if _, err := New(hostOf(server), "", "").Catalog(context.Background()); err == nil {
		t.Fatal("expected auth-required error for anonymous catalog")
	}
}
