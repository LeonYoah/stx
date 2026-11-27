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

// Package regclient 是极简 Docker Registry HTTP API v2 客户端：
// 只实现 STX 需要的 ping、catalog、tags 三个只读调用。
// Package regclient is a minimal Docker Registry HTTP API v2 client implementing
// only the three read-only calls STX needs: ping, catalog and tags.
package regclient

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// Client 单仓库只读客户端；scheme 在首次请求时探测（HTTPS 优先，失败回落 HTTP
// 并记录，私仓普遍无证书）。探测结果通过 Scheme() 暴露供落库展示。
// Client is a per-registry read-only client; the scheme is probed on first request
// (HTTPS first, HTTP fallback recorded since private registries commonly lack certs).
type Client struct {
	Host     string
	Username string
	Password string

	scheme   string
	insecure bool
	http     *http.Client
}

// New 构造客户端，host 必须已剥离 scheme 与尾斜杠。
// New builds a client; host must already be stripped of scheme and trailing slashes.
func New(host, username, password string) *Client {
	return &Client{
		Host:     strings.TrimSpace(host),
		Username: strings.TrimSpace(username),
		Password: password,
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

// Ping 检测仓库可达性与凭据有效性：200 或匿名 401（challenge 存在即证明在线）算通。
// Ping probes reachability and credential validity: 200 or an anonymous 401 (the
// challenge itself proves the registry is alive) counts as reachable.
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.do(ctx, "/v2/", true)
	return err
}

// Catalog 拉取仓库镜像名列表（/v2/_catalog，分页自动跟随）。
// Catalog fetches the repository list, following pagination.
func (c *Client) Catalog(ctx context.Context) ([]string, error) {
	repos := make([]string, 0)
	last := ""
	for {
		path := "/v2/_catalog?n=1000"
		if last != "" {
			path += "&last=" + last
		}
		body, err := c.do(ctx, path, false)
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
		repos = append(repos, resp.Repositories...)
		if len(resp.Repositories) < 1000 {
			break
		}
		last = resp.Repositories[len(resp.Repositories)-1]
	}
	return repos, nil
}

// Tags 拉取指定镜像的 tag 列表（/v2/<repo>/tags/list）。
// Tags fetches the tag list of one repository.
func (c *Client) Tags(ctx context.Context, repo string) ([]string, error) {
	repo = strings.Trim(strings.TrimSpace(repo), "/")
	if repo == "" {
		return nil, fmt.Errorf("regclient: repo is required")
	}
	body, err := c.do(ctx, "/v2/"+repo+"/tags/list", false)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Tags []string `json:"tags"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("regclient: parse tags: %w", err)
	}
	return resp.Tags, nil
}

// do 执行一次请求，自动完成 scheme 探测与 basic auth 注入。
// do performs one request with scheme probing and basic-auth injection.
func (c *Client) do(ctx context.Context, path string, allowUnauthorized bool) ([]byte, error) {
	if c.Host == "" {
		return nil, fmt.Errorf("regclient: host is required")
	}
	schemes := []string{"https", "http"}
	if c.scheme != "" {
		schemes = []string{c.scheme}
	}
	var lastErr error
	for _, scheme := range schemes {
		body, retryable, err := c.doWithScheme(ctx, scheme, path, allowUnauthorized)
		if err == nil {
			c.scheme = scheme
			return body, nil
		}
		lastErr = err
		// 仅协议层失败（TLS 握手、连接拒绝）才值得换 scheme 重试；
		// 已得到 HTTP 响应（401/404 等）说明 scheme 正确，直接返回错误。
		// Only transport-level failures (TLS handshake, refused connections) justify
		// a scheme switch; any HTTP response means the scheme was right.
		if !retryable {
			return nil, err
		}
	}
	return nil, lastErr
}

func (c *Client) doWithScheme(ctx context.Context, scheme, path string, allowUnauthorized bool) (body []byte, retryable bool, err error) {
	url := scheme + "://" + c.Host + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false, err
	}
	if c.Username != "" {
		req.SetBasicAuth(c.Username, c.Password)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, true, fmt.Errorf("regclient: %s: %w", url, err)
	}
	defer resp.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	switch {
	case resp.StatusCode == http.StatusOK:
		return payload, false, nil
	case resp.StatusCode == http.StatusUnauthorized:
		if c.Username != "" {
			// 配了凭据还 401 就是凭据错误，与调用场景无关。
			// A 401 despite configured credentials means wrong credentials regardless
			// of the calling context.
			return nil, false, fmt.Errorf("regclient: authentication failed for %s@%s", c.Username, c.Host)
		}
		if allowUnauthorized {
			return payload, false, nil
		}
		return nil, false, fmt.Errorf("regclient: %s requires authentication", c.Host)
	case resp.StatusCode == http.StatusNotFound:
		return nil, false, fmt.Errorf("regclient: not found: %s", url)
	default:
		return nil, false, fmt.Errorf("regclient: %s returned %d: %s", url, resp.StatusCode, strings.TrimSpace(string(payload))[:200])
	}
}
