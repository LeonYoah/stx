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
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	seatunnelmeta "github.com/LeonYoah/stx/internal/seatunnel"
)

// TestEnsureSTXJavaProxySupportAssetsDownloadsFromAssetBase 覆盖 DEF-R2-1：本地缺资产时从控制面资产基址拉取脚本与 jar。
// TestEnsureSTXJavaProxySupportAssetsDownloadsFromAssetBase covers DEF-R2-1: missing local assets are pulled from the Control Plane asset base.
func TestEnsureSTXJavaProxySupportAssetsDownloadsFromAssetBase(t *testing.T) {
	home := t.TempDir()
	t.Setenv(stxJavaProxyHomeEnvVar, home)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/" + seatunnelmeta.STXJavaProxyScriptFileName:
			_, _ = w.Write([]byte("#!/bin/sh\necho proxy\n"))
		case "/stx-java-proxy-v2.jar":
			_, _ = w.Write([]byte("jar-v2-bytes"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	if err := EnsureSTXJavaProxySupportAssets(context.Background(), t.TempDir(), "2.3.13", server.URL); err != nil {
		t.Fatalf("EnsureSTXJavaProxySupportAssets: %v", err)
	}

	script := filepath.Join(home, "scripts", seatunnelmeta.STXJavaProxyScriptFileName)
	jar := filepath.Join(home, "lib", seatunnelmeta.STXJavaProxyJarFileName("2.3.13"))
	if got, err := os.ReadFile(script); err != nil || string(got) == "" {
		t.Fatalf("script not staged at %s: %v %q", script, err, got)
	}
	if got, err := os.ReadFile(jar); err != nil || string(got) != "jar-v2-bytes" {
		t.Fatalf("jar not staged at %s: %v %q", jar, err, got)
	}
}

// TestEnsureSTXJavaProxySupportAssetsNoopWhenPresent 本地已有资产时不再覆盖。
// TestEnsureSTXJavaProxySupportAssetsNoopWhenPresent leaves existing local assets untouched.
func TestEnsureSTXJavaProxySupportAssetsNoopWhenPresent(t *testing.T) {
	home := t.TempDir()
	t.Setenv(stxJavaProxyHomeEnvVar, home)
	script := filepath.Join(home, "scripts", seatunnelmeta.STXJavaProxyScriptFileName)
	jar := filepath.Join(home, "lib", "stx-java-proxy-v2.jar")
	if err := os.MkdirAll(filepath.Dir(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(jar), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(script, []byte("existing-script"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(jar, []byte("existing-jar"), 0o644); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("unexpected download request %s", r.URL.Path)
	}))
	t.Cleanup(server.Close)

	if err := EnsureSTXJavaProxySupportAssets(context.Background(), t.TempDir(), "v2", server.URL); err != nil {
		t.Fatalf("EnsureSTXJavaProxySupportAssets: %v", err)
	}
	if got, _ := os.ReadFile(script); string(got) != "existing-script" {
		t.Fatalf("script overwritten: %q", got)
	}
	if got, _ := os.ReadFile(jar); string(got) != "existing-jar" {
		t.Fatalf("jar overwritten: %q", got)
	}
}
