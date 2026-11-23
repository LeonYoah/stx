package plugin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

// 429/5xx/404/传输失败换下一个镜像重试，全部可重试错误耗尽后才失败。
// Retry the next mirror on 429/5xx/404/transport errors; fail only after retryable sources are exhausted.
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

// 永久错误（如 403）不换源，直接失败。
// Permanent errors (e.g. 403) must not trigger a mirror switch.
func TestDownloadArtifactWithFallback_permanentErrorStops(t *testing.T) {
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

	_, _, err := d.downloadArtifactWithFallback(context.Background(), []string{forbidden.URL + "/a.jar", second.URL + "/b.jar"}, target, &DownloadProgress{}, nil)
	if err == nil || !errors.Is(err, ErrDownloadFailed) {
		t.Fatalf("expected download failure, got %v", err)
	}
	if hits != 0 {
		t.Fatalf("403 must not fall back to the next mirror, got %d hits", hits)
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
		{httpStatusError{code: http.StatusForbidden}, false},
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
