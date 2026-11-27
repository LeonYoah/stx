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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// fetchDockerHubTags 走 Docker Hub 公开 API 拉取 tag（registry v2 对 Hub 需要
// 匿名 token 流程，Hub API 无需认证且语义相同）。
// fetchDockerHubTags uses the public Docker Hub API (the registry v2 flow requires
// an anonymous token dance while the Hub API needs no auth for the same data).
func fetchDockerHubTags(ctx context.Context, repo string) ([]string, error) {
	repo = strings.Trim(strings.TrimSpace(repo), "/")
	if repo == "" {
		return nil, fmt.Errorf("installer: repo is required")
	}
	if !strings.Contains(repo, "/") {
		// Hub 官方镜像的省略写法是 library/<repo>。
		// Docker Hub official images live under the implicit library/ namespace.
		repo = "library/" + repo
	}
	tags := make([]string, 0)
	url := fmt.Sprintf("https://hub.docker.com/v2/repositories/%s/tags?page_size=100", repo)
	client := &http.Client{Timeout: 10 * time.Second}
	for url != "" && len(tags) < 500 {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("installer: docker hub tags: %w", err)
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("installer: docker hub tags for %s returned %d", repo, resp.StatusCode)
		}
		var page struct {
			Results []struct {
				Name string `json:"name"`
			} `json:"results"`
			Next string `json:"next"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, fmt.Errorf("installer: parse docker hub tags: %w", err)
		}
		for _, result := range page.Results {
			tags = append(tags, result.Name)
		}
		url = page.Next
	}
	return tags, nil
}
