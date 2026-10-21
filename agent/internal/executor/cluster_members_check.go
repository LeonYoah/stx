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

package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// PrecheckSubCommandCheckClusterMembers 读取本机 Hazelcast 成员列表，用于判断集群是否分裂。
// PrecheckSubCommandCheckClusterMembers reads the local Hazelcast member list to detect split clusters.
const PrecheckSubCommandCheckClusterMembers PrecheckSubCommand = "check_cluster_members"

// clusterMembersHTTPClient 用于访问本机 Hazelcast REST；测试可替换。
// clusterMembersHTTPClient is used to reach the local Hazelcast REST API; tests may replace it.
var clusterMembersHTTPClient = &http.Client{Timeout: 5 * time.Second}

type hazelcastClusterView struct {
	Members []struct {
		Address     string `json:"address"`
		LiteMember  bool   `json:"liteMember"`
		LocalMember bool   `json:"localMember"`
	} `json:"members"`
}

// handleCheckClusterMembers 访问 http://<host>:<port>/hazelcast/rest/cluster 并返回成员数量与地址。
// host 依次尝试 127.0.0.1 与参数 host（Hazelcast 可能只绑定在业务网卡上）。
// handleCheckClusterMembers queries http://<host>:<port>/hazelcast/rest/cluster and returns the member
// count and addresses. It tries 127.0.0.1 first and then the "host" parameter (Hazelcast may bind only
// to the business interface).
func handleCheckClusterMembers(ctx context.Context, params map[string]string) (*PrecheckResult, error) {
	port, err := strconv.Atoi(strings.TrimSpace(params["port"]))
	if err != nil || port <= 0 || port > 65535 {
		return &PrecheckResult{Success: false, Message: fmt.Sprintf("invalid port: %q", params["port"])}, nil
	}
	hosts := []string{"127.0.0.1"}
	if host := strings.TrimSpace(params["host"]); host != "" && host != "127.0.0.1" {
		hosts = append(hosts, host)
	}

	var lastErr error
	for _, host := range hosts {
		view, err := fetchHazelcastClusterView(ctx, net.JoinHostPort(host, strconv.Itoa(port)))
		if err != nil {
			lastErr = err
			continue
		}
		addresses := make([]string, 0, len(view.Members))
		dataMembers := 0
		for _, member := range view.Members {
			addresses = append(addresses, strings.NewReplacer("[", "", "]", "").Replace(member.Address))
			if !member.LiteMember {
				dataMembers++
			}
		}
		return &PrecheckResult{
			Success: true,
			Message: fmt.Sprintf("Hazelcast cluster has %d members / Hazelcast 集群有 %d 个成员", len(view.Members), len(view.Members)),
			Details: map[string]string{
				"member_count":      strconv.Itoa(len(view.Members)),
				"data_member_count": strconv.Itoa(dataMembers),
				"members":           strings.Join(addresses, ","),
			},
		}, nil
	}
	return &PrecheckResult{
		Success: false,
		Message: fmt.Sprintf("failed to read Hazelcast members / 读取 Hazelcast 成员失败: %v", lastErr),
	}, nil
}

func fetchHazelcastClusterView(ctx context.Context, hostPort string) (*hazelcastClusterView, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+hostPort+"/hazelcast/rest/cluster", nil)
	if err != nil {
		return nil, err
	}
	resp, err := clusterMembersHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	var view hazelcastClusterView
	if err := json.Unmarshal(body, &view); err != nil {
		return nil, fmt.Errorf("invalid cluster response: %w", err)
	}
	if len(view.Members) == 0 {
		return nil, fmt.Errorf("cluster response has no members")
	}
	return &view, nil
}
