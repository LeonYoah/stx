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

package cluster

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/LeonYoah/stx/internal/seatunnel"
	"github.com/gin-gonic/gin"
)

const (
	webUIMasterCacheTTL     = 60 * time.Second
	webUIMasterProbeTimeout = 1500 * time.Millisecond
	webUIMasterProbePath    = "/metrics/realtime/jobs"
)

type webUIMasterSelector struct {
	mu      sync.Mutex
	entries map[uint]webUIMasterEntry
	client  *http.Client
	now     func() time.Time
}

type webUIMasterEntry struct {
	nodeID    uint
	confirmed bool
	expiresAt time.Time
}

func (s *webUIMasterSelector) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

func (s *webUIMasterSelector) httpClient() *http.Client {
	if s.client != nil {
		return s.client
	}
	return &http.Client{Timeout: webUIMasterProbeTimeout}
}

func webUIProxyCandidates(nodes []*NodeInfo) (online []*NodeInfo, fallback *NodeInfo) {
	for _, node := range nodes {
		if node == nil || node.APIPort <= 0 {
			continue
		}
		if node.Role != NodeRoleMaster && node.Role != NodeRoleMasterWorker {
			continue
		}
		if fallback == nil {
			fallback = node
		}
		if node.IsOnline {
			online = append(online, node)
		}
	}
	return online, fallback
}

func webUINodeAddress(node *NodeInfo) string {
	host := strings.TrimSpace(node.HostIP)
	if host == "" {
		host = strings.TrimSpace(node.HostName)
	}
	if host == "" {
		return ""
	}
	return host + ":" + strconv.Itoa(node.APIPort)
}

func (s *webUIMasterSelector) activeMasterNodeID(ctx context.Context, clusterID uint, nodes []*NodeInfo) (uint, bool) {
	online, _ := webUIProxyCandidates(nodes)
	if len(online) == 0 {
		return 0, false
	}
	if len(online) == 1 {
		return online[0].ID, true
	}

	s.mu.Lock()
	if entry, ok := s.entries[clusterID]; ok && s.clock().Before(entry.expiresAt) {
		nodeID, confirmed := entry.nodeID, entry.confirmed
		s.mu.Unlock()
		return nodeID, confirmed
	}
	s.mu.Unlock()

	chosen := online[0]
	confirmed := false
	for _, node := range online {
		if s.probe(ctx, node) {
			chosen = node
			confirmed = true
			break
		}
	}
	s.mu.Lock()
	if s.entries == nil {
		s.entries = map[uint]webUIMasterEntry{}
	}
	s.entries[clusterID] = webUIMasterEntry{nodeID: chosen.ID, confirmed: confirmed, expiresAt: s.clock().Add(webUIMasterCacheTTL)}
	s.mu.Unlock()
	if !confirmed {
		return 0, false
	}
	return chosen.ID, true
}

func (s *webUIMasterSelector) pick(ctx context.Context, clusterID uint, nodes []*NodeInfo) *NodeInfo {
	online, fallback := webUIProxyCandidates(nodes)
	if len(online) == 0 {
		return fallback
	}
	if len(online) == 1 {
		return online[0]
	}
	activeID, _ := s.activeMasterNodeID(ctx, clusterID, nodes)
	for _, node := range online {
		if node.ID == activeID {
			return node
		}
	}
	return online[0]
}

func (s *webUIMasterSelector) probe(ctx context.Context, node *NodeInfo) bool {
	address := webUINodeAddress(node)
	if address == "" {
		return false
	}
	probeCtx, cancel := context.WithTimeout(ctx, webUIMasterProbeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, "http://"+address+webUIMasterProbePath, nil)
	if err != nil {
		return false
	}
	resp, err := s.httpClient().Do(req)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

type WebUIEndpoints struct {
	Available     bool   `json:"available"`
	Reason        string `json:"reason,omitempty"`
	ProxyBaseURL  string `json:"proxy_base_url"`
	NativeBaseURL string `json:"native_base_url"`
	NodeID        uint   `json:"node_id,omitempty"`
	HostName      string `json:"host_name,omitempty"`
	JobPathTemplate string `json:"job_path_template"`
}

type WebUIEndpointsResponse struct {
	ErrorMsg string          `json:"error_msg"`
	Data     *WebUIEndpoints `json:"data"`
}

// @Summary 获取 SeaTunnel 集群观测地址
// @Tags clusters
// @Produce json
// @Param id path int true "集群ID"
// @Success 200 {object} WebUIEndpointsResponse
// @Failure 400 {object} WebUIEndpointsResponse
// @Failure 404 {object} WebUIEndpointsResponse
// @Router /api/v1/clusters/{id}/webui-endpoints [get]
func (h *Handler) GetWebUIEndpoints(c *gin.Context) {
	clusterID, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, WebUIEndpointsResponse{ErrorMsg: "无效的集群 ID / Invalid cluster ID"})
		return
	}
	clusterInfo, err := h.service.Get(c.Request.Context(), uint(clusterID))
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), WebUIEndpointsResponse{ErrorMsg: err.Error()})
		return
	}
	nodes, err := h.service.GetNodes(c.Request.Context(), uint(clusterID))
	if err != nil {
		c.JSON(h.getStatusCodeForError(err), WebUIEndpointsResponse{ErrorMsg: err.Error()})
		return
	}
	result := &WebUIEndpoints{
		ProxyBaseURL:    "/api/v1/clusters/" + strconv.FormatUint(clusterID, 10) + "/webui/",
		JobPathTemplate: "#/jobs/{job_id}",
	}
	if !seatunnel.CapabilitiesForVersion(clusterInfo.Version).SupportsHTTPService {
		result.Reason = "version_unsupported"
		c.JSON(http.StatusOK, WebUIEndpointsResponse{Data: result})
		return
	}
	if runtime, ok := clusterInfo.Config["runtime"].(map[string]interface{}); ok {
		if enabled, isBool := runtime["enable_http"].(bool); isBool && !enabled {
			result.Reason = "http_disabled"
			c.JSON(http.StatusOK, WebUIEndpointsResponse{Data: result})
			return
		}
	}
	target := h.webUIMaster.pick(c.Request.Context(), uint(clusterID), nodes)
	switch {
	case target == nil:
		result.Reason = "no_webui_node"
	case webUINodeAddress(target) == "":
		result.Reason = "node_address_missing"
	default:
		result.Available = true
		result.NativeBaseURL = "http://" + webUINodeAddress(target) + "/"
		result.NodeID = target.ID
		result.HostName = target.HostName
	}
	c.JSON(http.StatusOK, WebUIEndpointsResponse{Data: result})
}
