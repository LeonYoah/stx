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

package plugin

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 兜底 URL 列表应包含全部镜像：主源被限流/宕机时仍有下一个可试。
// buildArtifactURLs should list every mirror so a rate-limited or down primary still has a next source.
func TestBuildArtifactURLs_allMirrorsFallback(t *testing.T) {
	d := &Downloader{}

	urls := d.buildArtifactURLs(MirrorURLs[MirrorSourceApache], "org/apache/seatunnel", "connector-jdbc", "2.3.13", "connector-jdbc-2.3.13.jar", MirrorSourceApache)
	if len(urls) != len(MirrorURLs) {
		t.Fatalf("apache mirror should fall back to all mirrors, got %v", urls)
	}
	if urls[0] != fmt.Sprintf("%s/org/apache/seatunnel/connector-jdbc/2.3.13/connector-jdbc-2.3.13.jar", MirrorURLs[MirrorSourceApache]) {
		t.Fatalf("primary must be the chosen mirror first, got %v", urls)
	}

	urls = d.buildArtifactURLs(MirrorURLs[MirrorSourceAliyun], "org/apache/seatunnel", "connector-jdbc", "2.3.13", "connector-jdbc-2.3.13.jar", MirrorSourceAliyun)
	if len(urls) != len(MirrorURLs) || urls[0] != fmt.Sprintf("%s/org/apache/seatunnel/connector-jdbc/2.3.13/connector-jdbc-2.3.13.jar", MirrorURLs[MirrorSourceAliyun]) {
		t.Fatalf("aliyun mirror should stay primary then fall back, got %v", urls)
	}
	if urls[1] != fmt.Sprintf("%s/org/apache/seatunnel/connector-jdbc/2.3.13/connector-jdbc-2.3.13.jar", MirrorURLs[MirrorSourceApache]) {
		t.Fatalf("apache should be the first fallback, got %v", urls)
	}
}

// 403/429/5xx/404/传输失败换下一个镜像重试，全部可重试错误耗尽后才失败。
// Retry the next mirror on 403/429/5xx/404/transport errors; fail only after retryable sources are exhausted.
func TestDownloadArtifactWithFallback_retriesNextMirrorOnRetryableError(t *testing.T) {
	rateLimited := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer rateLimited.Close()

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("jar-bytes"))
	}))
	defer origin.Close()

	d := &Downloader{httpClient: http.DefaultClient}
	target := filepath.Join(t.TempDir(), "connector-jdbc-2.3.13.jar")
	progress := &DownloadProgress{}

	jarURL, _, err := d.downloadArtifactWithFallback(context.Background(), []string{rateLimited.URL + "/a.jar", origin.URL + "/b.jar"}, target, progress, nil)
	if err != nil {
		t.Fatalf("expected fallback to succeed, got %v", err)
	}
	if jarURL != origin.URL+"/b.jar" {
		t.Fatalf("expected the fallback mirror URL, got %s", jarURL)
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "jar-bytes" {
		t.Fatalf("expected downloaded content, got %q err=%v", data, err)
	}
}

// 镜像侧 403 也可能是限流或区域拦截，应继续尝试下一个镜像。
// A mirror-side 403 can be rate limiting or regional blocking, so try the next mirror.
func TestDownloadArtifactWithFallback_forbiddenFallsBack(t *testing.T) {
	forbidden := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer forbidden.Close()

	hits := 0
	second := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		_, _ = w.Write([]byte("jar-bytes"))
	}))
	defer second.Close()

	d := &Downloader{httpClient: http.DefaultClient}
	target := filepath.Join(t.TempDir(), "x.jar")

	jarURL, _, err := d.downloadArtifactWithFallback(context.Background(), []string{forbidden.URL + "/a.jar", second.URL + "/b.jar"}, target, &DownloadProgress{}, nil)
	if err != nil {
		t.Fatalf("expected fallback to succeed, got %v", err)
	}
	if jarURL != second.URL+"/b.jar" {
		t.Fatalf("expected the fallback mirror URL, got %s", jarURL)
	}
	if hits != 1 {
		t.Fatalf("403 should fall back to the next mirror exactly once, got %d hits", hits)
	}
}

func TestShouldTryNextMirror(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{ErrFileNotFound, true},
		{httpStatusError{code: http.StatusTooManyRequests}, true},
		{httpStatusError{code: http.StatusBadGateway}, true},
		{httpStatusError{code: http.StatusForbidden}, true},
		{transportError{err: errors.New("connection reset")}, true},
		{ErrDownloadCancelled, false},
		{fmt.Errorf("wrap: %w", httpStatusError{code: http.StatusServiceUnavailable}), true},
	}
	for _, c := range cases {
		if got := shouldTryNextMirror(c.err); got != c.want {
			t.Errorf("shouldTryNextMirror(%v) = %v, want %v", c.err, got, c.want)
		}
	}
}

func TestDownloadPlugin_dependencyFailureIsRecordedWithoutBlocking(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "missing-dep") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if strings.HasSuffix(r.URL.Path, ".sha1") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte("jar-bytes"))
	}))
	defer server.Close()

	oldMirrors := make(map[MirrorSource]string, len(MirrorURLs))
	for key, value := range MirrorURLs {
		oldMirrors[key] = value
		MirrorURLs[key] = server.URL
	}
	defer func() {
		for key, value := range oldMirrors {
			MirrorURLs[key] = value
		}
	}()

	pluginsDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(pluginsDir, "9.9.91", "connectors"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginsDir, "9.9.91", "connectors", "connector-jdbc-9.9.91.jar"), []byte("connector"), 0o644); err != nil {
		t.Fatal(err)
	}

	d := NewDownloader(pluginsDir)
	plugin := &Plugin{
		Name:       "jdbc",
		ArtifactID: "connector-jdbc",
		GroupID:    "org.apache.seatunnel",
		Version:    "9.9.91",
		Dependencies: []PluginDependency{
			{GroupID: "example", ArtifactID: "good-dep", Version: "1.0", TargetDir: "lib"},
			{GroupID: "example", ArtifactID: "missing-dep", Version: "1.0", TargetDir: "lib"},
		},
	}

	var finalCallbackProgress *DownloadProgress
	if err := d.DownloadPlugin(context.Background(), plugin, MirrorSourceApache, []string{"mysql"}, true, false, func(p *DownloadProgress) {
		cp := *p
		cp.DependencyFailures = append([]PluginDependencyFailure(nil), p.DependencyFailures...)
		finalCallbackProgress = &cp
	}); err != nil {
		t.Fatalf("expected dependency failure not to block plugin download, got %v", err)
	}
	if finalCallbackProgress == nil || finalCallbackProgress.Status != "completed" || len(finalCallbackProgress.DependencyFailures) != 1 {
		t.Fatalf("expected final callback with completed status and dependency failure, got %+v", finalCallbackProgress)
	}
	if _, err := os.Stat(filepath.Join(pluginsDir, "9.9.91", "lib", "good-dep-1.0.jar")); err != nil {
		t.Fatalf("expected successful dependency to be downloaded: %v", err)
	}

	metadata, err := d.readPluginMetadata("jdbc", "9.9.91")
	if err != nil {
		t.Fatalf("expected plugin metadata: %v", err)
	}
	if len(metadata.DependencyFailures) != 1 || metadata.DependencyFailures[0].ArtifactID != "missing-dep" {
		t.Fatalf("expected missing dependency failure in metadata, got %+v", metadata.DependencyFailures)
	}

	locals, err := d.ListLocalPlugins()
	if err != nil {
		t.Fatalf("list local plugins: %v", err)
	}
	if len(locals) != 1 || len(locals[0].DependencyFailures) != 1 || locals[0].DependencyFailures[0].ArtifactID != "missing-dep" {
		t.Fatalf("expected local plugin to expose dependency failure, got %+v", locals)
	}

	// 手动补齐缺失依赖后，ListLocalPlugins 应自动收敛告警。
	// Once the missing dependency jar is present on disk, ListLocalPlugins should stop reporting it as failed.
	if err := os.WriteFile(filepath.Join(pluginsDir, "9.9.91", "lib", "missing-dep-1.0.jar"), []byte("manual-jar"), 0o644); err != nil {
		t.Fatal(err)
	}
	locals, err = d.ListLocalPlugins()
	if err != nil {
		t.Fatalf("list local plugins after manual fix: %v", err)
	}
	if len(locals) != 1 || len(locals[0].DependencyFailures) != 0 {
		t.Fatalf("expected dependency failures to clear once jar exists, got %+v", locals)
	}
}

func TestDownloadDependenciesUsesDirectSourceURL(t *testing.T) {
	zipBuffer := &bytes.Buffer{}
	zipWriter := zip.NewWriter(zipBuffer)
	zipEntry, err := zipWriter.Create("bundle/lib/bos-hdfs-sdk-1.0.4-community.jar")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := zipEntry.Write([]byte("bos-jar")); err != nil {
		t.Fatal(err)
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatal(err)
	}

	tarBuffer := &bytes.Buffer{}
	gzipWriter := gzip.NewWriter(tarBuffer)
	tarWriter := tar.NewWriter(gzipWriter)
	jindoBytes := []byte("jindo-jar")
	if err := tarWriter.WriteHeader(&tar.Header{Name: "jindosdk-4.6.1/lib/jindo-sdk-4.6.1.jar", Mode: 0o644, Size: int64(len(jindoBytes)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write(jindoBytes); err != nil {
		t.Fatal(err)
	}
	jindoCoreBytes := []byte("jindo-core-jar")
	if err := tarWriter.WriteHeader(&tar.Header{Name: "jindosdk-4.6.1/lib/jindo-core-4.6.1.jar", Mode: 0o644, Size: int64(len(jindoCoreBytes)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write(jindoCoreBytes); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}

	jindoArchiveHits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/esdk-obs-java-3.19.7.3.jar":
			_, _ = w.Write([]byte("obs-jar"))
		case "/bos.zip":
			_, _ = w.Write(zipBuffer.Bytes())
		case "/jindo.tar.gz":
			jindoArchiveHits++
			_, _ = w.Write(tarBuffer.Bytes())
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	pluginsDir := t.TempDir()
	d := NewDownloader(pluginsDir)
	plugin := &Plugin{
		Name:    "file-mixed",
		Version: "3.1.0",
		Dependencies: []PluginDependency{
			{GroupID: "com.huawei.storage", ArtifactID: "esdk-obs-java", Version: "3.19.7.3", TargetDir: "lib", SourceURL: server.URL + "/esdk-obs-java-3.19.7.3.jar"},
			{GroupID: "com.baidubce", ArtifactID: "bos-hdfs-sdk", Version: "1.0.4-community", TargetDir: "lib", SourceURL: server.URL + "/bos.zip"},
			{GroupID: "com.aliyun.jindodata", ArtifactID: "jindo-sdk", Version: "4.6.1", TargetDir: "lib", SourceURL: server.URL + "/jindo.tar.gz"},
			{GroupID: "com.aliyun.jindodata", ArtifactID: "jindo-core", Version: "4.6.1", TargetDir: "lib", SourceURL: server.URL + "/jindo.tar.gz"},
		},
	}

	failures, err := d.DownloadDependencies(context.Background(), plugin, MirrorSourceApache, nil)
	if err != nil {
		t.Fatalf("DownloadDependencies returned error: %v", err)
	}
	if len(failures) != 0 {
		t.Fatalf("expected no dependency failures, got %+v", failures)
	}
	for name, want := range map[string]string{
		"esdk-obs-java-3.19.7.3.jar":       "obs-jar",
		"bos-hdfs-sdk-1.0.4-community.jar": "bos-jar",
		"jindo-sdk-4.6.1.jar":              "jindo-jar",
		"jindo-core-4.6.1.jar":             "jindo-core-jar",
	} {
		data, err := os.ReadFile(filepath.Join(pluginsDir, "3.1.0", "lib", name))
		if err != nil || string(data) != want {
			t.Fatalf("expected %s content %q, got %q err=%v", name, want, data, err)
		}
	}
	if jindoArchiveHits != 1 {
		t.Fatalf("expected shared jindo archive to be downloaded once, got %d", jindoArchiveHits)
	}
}

func TestConnectorEmbeddedVersionMatchesInternalSuffix(t *testing.T) {
	if !connectorEmbeddedVersionMatches("3.1.0", "3.1.0-hotfix1") {
		t.Fatalf("expected base embedded version to match internal suffix version")
	}
	if !connectorEmbeddedVersionMatches("3.1.0-hotfix1", "3.1.0-hotfix1") {
		t.Fatalf("expected exact embedded version to match")
	}
	if connectorEmbeddedVersionMatches("3.1.1", "3.1.0-hotfix1") {
		t.Fatalf("different embedded version must not match")
	}

	pluginsDir := t.TempDir()
	const version = "3.1.0-hotfix1"
	connectorsDir := filepath.Join(pluginsDir, version, "connectors")
	if err := os.MkdirAll(connectorsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(connectorsDir, "connector-fake-3.1.0.jar"), []byte("base-fake-jar"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(connectorsDir, "connector-other-3.1.1.jar"), []byte("wrong-ver"), 0o644); err != nil {
		t.Fatal(err)
	}

	d := NewDownloader(pluginsDir)
	locals, err := d.ListLocalPlugins()
	if err != nil {
		t.Fatalf("ListLocalPlugins failed: %v", err)
	}
	if len(locals) != 1 || locals[0].Name != "fake" || locals[0].Version != version {
		t.Fatalf("expected base-version jar to be listed under %s, got %+v", version, locals)
	}
	if !d.IsConnectorDownloaded("fake", version) || !d.IsConnectorDownloadedByArtifactID("connector-fake", version) {
		t.Fatalf("expected IsConnectorDownloaded* to find base-version jar under %s", version)
	}
	data, err := d.ReadPluginFile("fake", version)
	if err != nil || string(data) != "base-fake-jar" {
		t.Fatalf("ReadPluginFile failed: data=%q err=%v", data, err)
	}
	data, err = d.ReadPluginFileByArtifactID("connector-fake", version)
	if err != nil || string(data) != "base-fake-jar" {
		t.Fatalf("ReadPluginFileByArtifactID failed: data=%q err=%v", data, err)
	}
	if err := d.DeleteLocalPlugin("fake", version); err != nil {
		t.Fatalf("DeleteLocalPlugin failed for base-version jar: %v", err)
	}
	if d.IsConnectorDownloaded("fake", version) {
		t.Fatalf("expected connector to be deleted")
	}
}
