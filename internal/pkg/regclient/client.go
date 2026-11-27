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

// Package regclient 是极简 OCI Distribution / Docker Registry HTTP API v2 客户端：
// 支持 Basic Auth 与 Bearer Token Auth（Harbor/ACR/GHCR/Quay/Docker Hub），
// 实现 ping、catalog、tags 三个只读调用。
// Package regclient is a minimal OCI Distribution / Docker Registry HTTP API v2
// client supporting Basic Auth and Bearer Token Auth (Harbor/ACR/GHCR/Quay/Docker Hub),
// implementing ping, catalog, and tags read-only calls.
package regclient

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	dockerHubAliasHost    = "docker.io"
	dockerHubRegistryHost = "registry-1.docker.io"
	maxCatalogRepos       = 2000
	maxTags               = 500
)

var (
	// ErrAuthFailed 表示配置了凭据但仓库返回认证失败。
	// ErrAuthFailed indicates credentials were provided but rejected by the registry.
	ErrAuthFailed = errors.New("regclient: authentication failed")
	// ErrAuthRequired 表示匿名访问被拒绝，仓库要求认证。
	// ErrAuthRequired indicates anonymous access was denied because auth is required.
	ErrAuthRequired = errors.New("regclient: authentication required")
)

// Client 单仓库只读客户端；scheme 在首次请求时探测（HTTPS 优先，失败回落 HTTP
// 并记录，私仓普遍无证书；docker.io 仅走 HTTPS 到 registry-1.docker.io）。
// Client is a per-registry read-only client; the scheme is probed on first request
// (HTTPS first, HTTP fallback recorded for private registries; docker.io maps to
// registry-1.docker.io over HTTPS only).
type Client struct {
	Host     string
	Username string
	Password string

	apiHost    string
	scheme     string
	onlyHTTPS  bool
	http       *http.Client
}

// New 构造客户端，host 必须已剥离 scheme 与尾斜杠。
// New builds a client; host must already be stripped of scheme and trailing slashes.
func New(host, username, password string) *Client {
	normalized := strings.TrimSpace(host)
	apiHost := normalized
	onlyHTTPS := false
	if strings.EqualFold(normalized, dockerHubAliasHost) || strings.EqualFold(normalized, "index.docker.io") {
		apiHost = dockerHubRegistryHost
		onlyHTTPS = true
	}
	return &Client{
		Host:      normalized,
		Username:  strings.TrimSpace(username),
		Password:  password,
		apiHost:   apiHost,
		onlyHTTPS: onlyHTTPS,
		http: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				// 私仓自签证书普遍，跳过校验；凭据本来就只发往用户自己登记的仓库。
				// Self-signed certs are the norm for private registries; credentials
				// only ever go to hosts the user explicitly registered.
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
				DialContext: (&net.Dialer{
					Timeout: 5 * time.Second,
				}).DialContext,
			},
		},
	}
}

// Scheme 返回探测到的 scheme（https/http），未探测前为空。
// Scheme returns the probed scheme, empty before the first request.
func (c *Client) Scheme() string { return c.scheme }

// Ping 检测仓库可达性与凭据有效性：200 或匿名 401（challenge 存在即证明在线）算通；
// 若配置了凭据却认证失败则返回 ErrAuthFailed。
// Ping probes reachability and credential validity: 200 or an anonymous 401
// (challenge proves liveness) succeeds; configured credentials failing auth return ErrAuthFailed.
func (c *Client) Ping(ctx context.Context) error {
	_, _, err := c.do(ctx, "/v2/", true)
	return err
}

// PingStrict 检测仓库可达性并要求授权通过：匿名访问收到 401 挑战且无法免密访问时返回 ErrAuthRequired。
// PingStrict probes reachability and requires authorization, returning ErrAuthRequired when
// anonymous access is rejected by the registry.
func (c *Client) PingStrict(ctx context.Context) error {
	_, _, err := c.do(ctx, "/v2/", false)
	return err
}

// Catalog 拉取仓库镜像名列表（/v2/_catalog，支持 Link 头与 last 分页，上限 2000）。
// Catalog fetches the repository list following Link headers or last pagination, capped at 2000.
func (c *Client) Catalog(ctx context.Context) ([]string, error) {
	repos := make([]string, 0)
	nextPath := "/v2/_catalog?n=1000"
	visited := make(map[string]struct{})
	for nextPath != "" && len(repos) < maxCatalogRepos {
		if _, seen := visited[nextPath]; seen {
			break
		}
		visited[nextPath] = struct{}{}

		body, headers, err := c.do(ctx, nextPath, false)
		if err != nil {
			return nil, err
		}
		var resp struct {
			Repositories []string `json:"repositories"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("regclient: parse catalog: %w", err)
		}
		if len(resp.Repositories) == 0 {
			break
		}
		for _, repo := range resp.Repositories {
			repos = append(repos, repo)
			if len(repos) >= maxCatalogRepos {
				return repos, nil
			}
		}

		linkNext := parseNextLink(headers.Get("Link"))
		if linkNext != "" {
			nextPath = linkNext
			continue
		}
		if len(resp.Repositories) < 1000 {
			break
		}
		last := resp.Repositories[len(resp.Repositories)-1]
		nextPath = "/v2/_catalog?n=1000&last=" + url.QueryEscape(last)
	}
	return repos, nil
}

// Tags 拉取指定镜像的 tag 列表（/v2/<repo>/tags/list，支持 Link 分页，上限 500）。
// Tags fetches the tag list of one repository with Link pagination, capped at 500.
func (c *Client) Tags(ctx context.Context, repo string) ([]string, error) {
	repo = strings.Trim(strings.TrimSpace(repo), "/")
	if repo == "" {
		return nil, fmt.Errorf("regclient: repo is required")
	}
	if c.apiHost == dockerHubRegistryHost && !strings.Contains(repo, "/") {
		// Docker Hub 官方镜像的隐式命名空间是 library/。
		// Docker Hub official images live under the implicit library/ namespace.
		repo = "library/" + repo
	}
	tags := make([]string, 0)
	nextPath := "/v2/" + repo + "/tags/list?n=500"
	visited := make(map[string]struct{})
	for nextPath != "" && len(tags) < maxTags {
		if _, seen := visited[nextPath]; seen {
			break
		}
		visited[nextPath] = struct{}{}

		body, headers, err := c.do(ctx, nextPath, false)
		if err != nil {
			return nil, err
		}
		var resp struct {
			Tags []string `json:"tags"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("regclient: parse tags: %w", err)
		}
		for _, tag := range resp.Tags {
			tags = append(tags, tag)
			if len(tags) >= maxTags {
				return tags, nil
			}
		}
		nextPath = parseNextLink(headers.Get("Link"))
	}
	return tags, nil
}

func (c *Client) do(ctx context.Context, path string, allowUnauthorized bool) ([]byte, http.Header, error) {
	if c.apiHost == "" {
		return nil, nil, fmt.Errorf("regclient: host is required")
	}
	schemes := []string{"https", "http"}
	if c.onlyHTTPS {
		schemes = []string{"https"}
	}
	if c.scheme != "" {
		schemes = []string{c.scheme}
	}
	var lastErr error
	for _, scheme := range schemes {
		body, headers, retryable, err := c.doWithScheme(ctx, scheme, path, allowUnauthorized)
		if err == nil {
			c.scheme = scheme
			return body, headers, nil
		}
		lastErr = err
		// 仅协议/连接层失败才换 scheme 重试；收到明确 HTTP 响应（如 401/404）说明协议对。
		// Only transport-level failures justify switching scheme; an HTTP response proves scheme.
		if !retryable {
			return nil, nil, err
		}
	}
	return nil, nil, lastErr
}

func (c *Client) doWithScheme(ctx context.Context, scheme, path string, allowUnauthorized bool) ([]byte, http.Header, bool, error) {
	targetURL := c.resolveURL(scheme, path)
	// 先不带 Authorization 探测，准确拿 WWW-Authenticate（Bearer / Basic）再按挑战回应，
	// 避免对 Bearer 仓库盲发 Basic 导致某些实现直接拒绝或泄露凭据头。
	// Probe without Authorization first to inspect WWW-Authenticate (Bearer/Basic) cleanly.
	resp, payload, err := c.execGET(ctx, targetURL, "")
	if err != nil {
		return nil, nil, true, fmt.Errorf("regclient: %s: %w", targetURL, err)
	}
	if resp.StatusCode == http.StatusOK {
		return payload, resp.Header, false, nil
	}
	if resp.StatusCode == http.StatusUnauthorized {
		authHeader := resp.Header.Get("WWW-Authenticate")
		schemeKind, params := parseWWWAuthenticate(authHeader)
		if schemeKind == "bearer" {
			if c.Username == "" && allowUnauthorized {
				// 匿名 Ping 收到 Bearer challenge，若 realm 可访问或匿名 token 可换则也算通；
				// 只要服务器吐出 challenge 就已证明仓库在线。
				// Anonymous Ping reaching a Bearer challenge already proves liveness.
				return payload, resp.Header, false, nil
			}
			token, tokErr := c.fetchBearerToken(ctx, params, path)
			if tokErr != nil {
				return nil, nil, false, tokErr
			}
			retryResp, retryPayload, retryErr := c.execGET(ctx, targetURL, "Bearer "+token)
			if retryErr != nil {
				return nil, nil, false, fmt.Errorf("regclient: %s: %w", targetURL, retryErr)
			}
			if retryResp.StatusCode == http.StatusOK {
				return retryPayload, retryResp.Header, false, nil
			}
			if retryResp.StatusCode == http.StatusUnauthorized || retryResp.StatusCode == http.StatusForbidden {
				if c.Username != "" {
					return nil, nil, false, fmt.Errorf("%w for %s@%s", ErrAuthFailed, c.Username, c.Host)
				}
				if allowUnauthorized {
					return retryPayload, retryResp.Header, false, nil
				}
				return nil, nil, false, fmt.Errorf("%w: %s", ErrAuthRequired, c.Host)
			}
			if retryResp.StatusCode == http.StatusNotFound {
				return nil, nil, false, fmt.Errorf("regclient: not found: %s", targetURL)
			}
			return nil, nil, false, fmt.Errorf("regclient: %s returned %d: %s", targetURL, retryResp.StatusCode, truncateErrorBody(retryPayload, 200))
		}

		// Basic Auth 或无 Bearer 头：配置了凭据时带 Basic 重试一次。
		// Basic Auth or plain 401: retry once with Basic Auth when credentials are present.
		if c.Username != "" {
			retryResp, retryPayload, retryErr := c.execGETWithBasic(ctx, targetURL)
			if retryErr != nil {
				return nil, nil, false, fmt.Errorf("regclient: %s: %w", targetURL, retryErr)
			}
			if retryResp.StatusCode == http.StatusOK {
				return retryPayload, retryResp.Header, false, nil
			}
			if retryResp.StatusCode == http.StatusUnauthorized || retryResp.StatusCode == http.StatusForbidden {
				return nil, nil, false, fmt.Errorf("%w for %s@%s", ErrAuthFailed, c.Username, c.Host)
			}
			if retryResp.StatusCode == http.StatusNotFound {
				return nil, nil, false, fmt.Errorf("regclient: not found: %s", targetURL)
			}
			return nil, nil, false, fmt.Errorf("regclient: %s returned %d: %s", targetURL, retryResp.StatusCode, truncateErrorBody(retryPayload, 200))
		}
		if allowUnauthorized {
			return payload, resp.Header, false, nil
		}
		return nil, nil, false, fmt.Errorf("%w: %s", ErrAuthRequired, c.Host)
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil, false, fmt.Errorf("regclient: not found: %s", targetURL)
	}
	return nil, nil, false, fmt.Errorf("regclient: %s returned %d: %s", targetURL, resp.StatusCode, truncateErrorBody(payload, 200))
}

func (c *Client) resolveURL(scheme, pathOrURL string) string {
	if strings.HasPrefix(pathOrURL, "https://") || strings.HasPrefix(pathOrURL, "http://") {
		return pathOrURL
	}
	if !strings.HasPrefix(pathOrURL, "/") {
		pathOrURL = "/" + pathOrURL
	}
	return scheme + "://" + c.apiHost + pathOrURL
}

func (c *Client) execGET(ctx context.Context, targetURL, authHeader string) (*http.Response, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, nil, err
	}
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	return resp, payload, nil
}

func (c *Client) execGETWithBasic(ctx context.Context, targetURL string) (*http.Response, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, nil, err
	}
	req.SetBasicAuth(c.Username, c.Password)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	return resp, payload, nil
}

func (c *Client) fetchBearerToken(ctx context.Context, params map[string]string, requestPath string) (string, error) {
	realm := strings.TrimSpace(params["realm"])
	if realm == "" {
		if c.Username != "" {
			return "", fmt.Errorf("%w for %s@%s: missing bearer realm", ErrAuthFailed, c.Username, c.Host)
		}
		return "", fmt.Errorf("%w: %s missing bearer realm", ErrAuthRequired, c.Host)
	}
	parsedRealm, err := url.Parse(realm)
	if err != nil {
		return "", fmt.Errorf("regclient: invalid bearer realm %q: %w", realm, err)
	}
	query := parsedRealm.Query()
	if svc := strings.TrimSpace(params["service"]); svc != "" {
		query.Set("service", svc)
	}
	scope := strings.TrimSpace(params["scope"])
	if scope == "" {
		scope = inferBearerScope(requestPath)
	}
	if scope != "" {
		query.Set("scope", scope)
	}
	parsedRealm.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsedRealm.String(), nil)
	if err != nil {
		return "", err
	}
	if c.Username != "" {
		req.SetBasicAuth(c.Username, c.Password)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("regclient: token exchange failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		if c.Username != "" {
			return "", fmt.Errorf("%w for %s@%s", ErrAuthFailed, c.Username, c.Host)
		}
		return "", fmt.Errorf("%w: %s", ErrAuthRequired, c.Host)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("regclient: token endpoint returned %d: %s", resp.StatusCode, truncateErrorBody(body, 200))
	}
	var tokenResp struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("regclient: parse token response: %w", err)
	}
	tok := strings.TrimSpace(tokenResp.Token)
	if tok == "" {
		tok = strings.TrimSpace(tokenResp.AccessToken)
	}
	if tok == "" {
		return "", fmt.Errorf("regclient: empty token from %s", realm)
	}
	return tok, nil
}

func inferBearerScope(requestPath string) string {
	cleanPath := requestPath
	if idx := strings.Index(cleanPath, "?"); idx >= 0 {
		cleanPath = cleanPath[:idx]
	}
	if strings.HasPrefix(cleanPath, "/v2/_catalog") {
		return "registry:catalog:*"
	}
	if strings.HasPrefix(cleanPath, "/v2/") && strings.HasSuffix(cleanPath, "/tags/list") {
		repo := strings.TrimSuffix(strings.TrimPrefix(cleanPath, "/v2/"), "/tags/list")
		if repo != "" {
			return "repository:" + repo + ":pull"
		}
	}
	return ""
}

var authParamPattern = regexp.MustCompile(`([a-zA-Z0-9_-]+)="([^"]*)"`)

// parseWWWAuthenticate 解析 Bearer / Basic 挑战头与参数映射。
// parseWWWAuthenticate parses Bearer / Basic challenge headers into scheme + key-value params.
func parseWWWAuthenticate(header string) (string, map[string]string) {
	trimmed := strings.TrimSpace(header)
	if trimmed == "" {
		return "", nil
	}
	parts := strings.SplitN(trimmed, " ", 2)
	scheme := strings.ToLower(strings.TrimSpace(parts[0]))
	params := map[string]string{}
	if len(parts) == 2 {
		for _, m := range authParamPattern.FindAllStringSubmatch(parts[1], -1) {
			if len(m) == 3 {
				params[strings.ToLower(m[1])] = m[2]
			}
		}
	}
	return scheme, params
}

// parseNextLink 解析 RFC 5988 Link 头中的 rel="next" 链接。
// parseNextLink extracts the rel="next" target from an RFC 5988 Link header.
func parseNextLink(header string) string {
	if strings.TrimSpace(header) == "" {
		return ""
	}
	for _, entry := range strings.Split(header, ",") {
		segments := strings.Split(entry, ";")
		if len(segments) < 2 {
			continue
		}
		isNext := false
		for _, seg := range segments[1:] {
			s := strings.TrimSpace(seg)
			if strings.EqualFold(s, `rel="next"`) || strings.EqualFold(s, "rel=next") {
				isNext = true
				break
			}
		}
		if !isNext {
			continue
		}
		rawURL := strings.TrimSpace(segments[0])
		rawURL = strings.TrimPrefix(rawURL, "<")
		rawURL = strings.TrimSuffix(rawURL, ">")
		return strings.TrimSpace(rawURL)
	}
	return ""
}

// truncateErrorBody 按 rune 安全截断错误响应体，避免短响应体或截断多字节字符导致越界/乱码。
// truncateErrorBody safely truncates error payloads by rune count, preventing slice-bounds panics.
func truncateErrorBody(payload []byte, maxRunes int) string {
	text := strings.TrimSpace(string(payload))
	if maxRunes <= 0 || text == "" {
		return text
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes]) + "..."
}
