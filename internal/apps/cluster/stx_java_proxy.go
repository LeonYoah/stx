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
	"encoding/json"
	"fmt"
	"strings"

	"github.com/LeonYoah/stx/internal/logger"
	"github.com/LeonYoah/stx/internal/seatunnel"
)

// STXJavaProxyStatus represents the managed stx-java-proxy state for a cluster.
type STXJavaProxyStatus struct {
	ClusterID   uint                   `json:"cluster_id"`
	ClusterName string                 `json:"cluster_name,omitempty"`
	NodeID      uint                   `json:"node_id,omitempty"`
	HostID      uint                   `json:"host_id,omitempty"`
	HostName    string                 `json:"host_name,omitempty"`
	HostIP      string                 `json:"host_ip,omitempty"`
	Role        string                 `json:"role,omitempty"`
	InstallDir  string                 `json:"install_dir,omitempty"`
	Version     string                 `json:"version,omitempty"`
	Service     string                 `json:"service,omitempty"`
	Managed     bool                   `json:"managed"`
	Running     bool                   `json:"running"`
	Healthy     bool                   `json:"healthy"`
	Endpoint    string                 `json:"endpoint,omitempty"`
	Port        int                    `json:"port,omitempty"`
	PID         int                    `json:"pid,omitempty"`
	LogPath     string                 `json:"log_path,omitempty"`
	Message     string                 `json:"message,omitempty"`
	JvmOpts     string                 `json:"jvm_opts,omitempty"`
	JvmMemory   map[string]interface{} `json:"jvm_memory,omitempty"`
}

// UpdateSTXJavaProxyConfigRequest represents a request to update proxy parameters.
// UpdateSTXJavaProxyConfigRequest 表示更新 proxy 参数请求。
type UpdateSTXJavaProxyConfigRequest struct {
	JvmOpts *string `json:"jvm_opts"`
	Port    *int    `json:"port,omitempty"`
	Restart *bool   `json:"restart,omitempty"`
}

// STXJavaProxyLogPreviewResult represents a service.log preview result.
// STXJavaProxyLogPreviewResult 表示 service.log 预览结果。
type STXJavaProxyLogPreviewResult struct {
	ClusterID uint   `json:"cluster_id"`
	LogPath   string `json:"log_path,omitempty"`
	Lines     int    `json:"lines,omitempty"`
	Logs      string `json:"logs,omitempty"`
}

func (s *Service) GetSTXJavaProxyStatus(ctx context.Context, clusterID uint) (*STXJavaProxyStatus, error) {
	return s.executeSTXJavaProxyCommand(ctx, clusterID, "status")
}

func (s *Service) StartSTXJavaProxy(ctx context.Context, clusterID uint) (*STXJavaProxyStatus, error) {
	return s.executeSTXJavaProxyCommand(ctx, clusterID, "start")
}

func (s *Service) StopSTXJavaProxy(ctx context.Context, clusterID uint) (*STXJavaProxyStatus, error) {
	return s.executeSTXJavaProxyCommand(ctx, clusterID, "stop")
}

func (s *Service) RestartSTXJavaProxy(ctx context.Context, clusterID uint) (*STXJavaProxyStatus, error) {
	return s.executeSTXJavaProxyCommand(ctx, clusterID, "restart")
}

func (s *Service) GetSTXJavaProxyServiceLog(
	ctx context.Context,
	clusterID uint,
	lines int,
) (*STXJavaProxyLogPreviewResult, error) {
	if lines <= 0 {
		lines = 200
	}
	clusterInfo, err := s.repo.GetByID(ctx, clusterID, false)
	if err != nil {
		return nil, err
	}
	node, hostInfo, err := s.pickSTXJavaProxyNode(ctx, clusterID)
	if err != nil {
		return nil, err
	}
	status, err := s.GetSTXJavaProxyStatus(ctx, clusterID)
	if err != nil && status == nil {
		return nil, err
	}
	logPath := strings.TrimSpace(status.LogPath)
	if logPath == "" {
		// 新 Agent 会回报 Agent home 下的 log_path；此处仅兜底旧路径约定。
		// Newer agents report log_path under Agent home; this is a legacy path fallback only.
		installDir := strings.TrimSpace(node.InstallDir)
		if installDir == "" {
			installDir = strings.TrimSpace(clusterInfo.InstallDir)
		}
		if installDir == "" {
			installDir = seatunnel.DefaultInstallDir("")
		}
		logPath = fmt.Sprintf("%s/.stx/stx-java-proxy/service.log", installDir)
	}
	success, message, sendErr := s.agentSender.SendCommand(ctx, hostInfo.AgentID, "get_logs", map[string]string{
		"log_file": logPath,
		"lines":    fmt.Sprintf("%d", lines),
		"mode":     "tail",
	})
	result := &STXJavaProxyLogPreviewResult{
		ClusterID: clusterID,
		LogPath:   logPath,
		Lines:     lines,
		Logs:      message,
	}
	if sendErr != nil {
		return result, fmt.Errorf("failed to preview stx-java-proxy service log: %w", sendErr)
	}
	if !success {
		return result, fmt.Errorf("%s", firstNonEmpty(message, "failed to preview stx-java-proxy service log"))
	}
	return result, nil
}

func (s *Service) executeSTXJavaProxyCommand(ctx context.Context, clusterID uint, commandType string) (*STXJavaProxyStatus, error) {
	if s.agentSender == nil {
		return nil, fmt.Errorf("agent sender is not configured")
	}
	if s.hostProvider == nil {
		return nil, fmt.Errorf("host provider is not configured")
	}

	clusterInfo, err := s.repo.GetByID(ctx, clusterID, false)
	if err != nil {
		return nil, err
	}
	node, hostInfo, err := s.pickSTXJavaProxyNode(ctx, clusterID)
	if err != nil {
		return nil, err
	}

	params := map[string]string{
		"service":     "stx_java_proxy",
		"cluster_id":  fmt.Sprintf("%d", clusterID),
		"node_id":     fmt.Sprintf("%d", node.ID),
		"install_dir": node.InstallDir,
		"version":     clusterInfo.Version,
	}
	// 启动/重启时带上资产基址，Agent 本地缺脚本/jar 时可从控制面拉取（DEF-R2-1）。
	// On start/restart, pass asset base so Agents can pull missing script/jar from Control Plane (DEF-R2-1).
	if (commandType == "start" || commandType == "restart") && strings.TrimSpace(s.assetBaseURL) != "" {
		params["asset_base_url"] = strings.TrimRight(strings.TrimSpace(s.assetBaseURL), "/")
	}
	// 启动/重启/状态查询时把集群配置的 java-proxy 端口与 JVM 参数下发给 Agent。
	// Pass cluster-configured java-proxy port and JVM options to Agent.
	if proxyCfg := clusterInfo.Config.GetSTXJavaProxyConfig(); proxyCfg != nil {
		if proxyCfg.Port > 0 {
			params["port"] = fmt.Sprintf("%d", proxyCfg.Port)
		}
		if strings.TrimSpace(proxyCfg.JvmOpts) != "" {
			params["jvm_opts"] = strings.TrimSpace(proxyCfg.JvmOpts)
		}
	} else if ports := clusterInfo.Config.GetPortConfig(); ports != nil && ports.JavaProxyPort > 0 {
		params["port"] = fmt.Sprintf("%d", ports.JavaProxyPort)
	}

	success, message, sendErr := s.agentSender.SendCommand(ctx, hostInfo.AgentID, commandType, params)
	status := decodeSTXJavaProxyStatus(clusterInfo, node, hostInfo, firstNonEmpty(message, errorString(sendErr)))
	if sendErr != nil {
		return status, sendErr
	}
	if !success {
		return status, fmt.Errorf("%s", firstNonEmpty(status.Message, parseCommandMessage(message), "stx-java-proxy command failed"))
	}
	return status, nil
}

func (s *Service) UpdateSTXJavaProxyConfig(
	ctx context.Context,
	clusterID uint,
	req *UpdateSTXJavaProxyConfigRequest,
) (*STXJavaProxyStatus, error) {
	if req == nil {
		return nil, fmt.Errorf("request cannot be empty")
	}
	clusterInfo, err := s.repo.GetByID(ctx, clusterID, false)
	if err != nil {
		return nil, err
	}
	if clusterInfo.Config == nil {
		clusterInfo.Config = make(ClusterConfig)
	}

	proxyCfg := clusterInfo.Config.GetSTXJavaProxyConfig()
	if proxyCfg == nil {
		proxyCfg = &STXJavaProxyConfig{}
	}
	if req.JvmOpts != nil {
		proxyCfg.JvmOpts = strings.TrimSpace(*req.JvmOpts)
	}
	if req.Port != nil && *req.Port > 0 {
		proxyCfg.Port = *req.Port
	}
	clusterInfo.Config["stx_java_proxy"] = map[string]interface{}{
		"jvm_opts": proxyCfg.JvmOpts,
		"port":     proxyCfg.Port,
	}
	if proxyCfg.Port > 0 {
		ports := clusterInfo.Config.GetPortConfig()
		if ports == nil {
			ports = &ClusterPortConfig{}
		}
		ports.JavaProxyPort = proxyCfg.Port
		clusterInfo.Config["ports"] = ports
	}

	if err := s.repo.Update(ctx, clusterInfo); err != nil {
		return nil, fmt.Errorf("failed to save cluster proxy configuration: %w", err)
	}

	shouldRestart := true
	if req.Restart != nil {
		shouldRestart = *req.Restart
	}

	if shouldRestart {
		return s.RestartSTXJavaProxy(ctx, clusterID)
	}
	return s.GetSTXJavaProxyStatus(ctx, clusterID)
}

// stxJavaProxyAutoStartEnabled 判断集群是否允许自动拉起 stx-java-proxy；只有显式配置 stx_java_proxy.auto_start=false 才关闭。
// stxJavaProxyAutoStartEnabled reports whether the cluster allows auto-starting stx-java-proxy; only an explicit
// stx_java_proxy.auto_start=false turns it off.
func stxJavaProxyAutoStartEnabled(config ClusterConfig) bool {
	rawProxy, ok := config["stx_java_proxy"].(map[string]interface{})
	if !ok {
		return true
	}
	autoStart, exists := rawProxy["auto_start"].(bool)
	return !exists || autoStart
}

// EnsureSTXJavaProxyOnHost 在 Agent（重新）就绪后，对 stx-java-proxy 选点落在该主机上的集群补一次启动。
// 以前只在注册集群时自动拉起一次，主机重启或进程退出后编辑器校验、DAG、表结构等依赖它的功能会一直不可用。
// Agent 侧启动是幂等的，已在运行时只做健康检查。
// EnsureSTXJavaProxyOnHost starts stx-java-proxy for clusters whose proxy node is on this host once its Agent is
// (re)ready. It used to be auto-started only at cluster registration, so after a host reboot or a process exit the
// features depending on it (editor validation, DAG, schemas) stayed unavailable. Starting is idempotent on the Agent.
func (s *Service) EnsureSTXJavaProxyOnHost(ctx context.Context, hostID uint) {
	if s == nil || s.repo == nil || s.agentSender == nil || s.hostProvider == nil || hostID == 0 {
		return
	}
	clusters, err := s.repo.GetClustersWithHostID(ctx, hostID)
	if err != nil {
		logger.WarnF(ctx, "[Cluster] list clusters of host %d for stx-java-proxy failed: %v / 查询主机 %d 的集群失败: %v", hostID, err, hostID, err)
		return
	}
	for _, clusterInfo := range clusters {
		if clusterInfo == nil || !stxJavaProxyAutoStartEnabled(clusterInfo.Config) {
			continue
		}
		_, hostInfo, err := s.pickSTXJavaProxyNode(ctx, clusterInfo.ID)
		if err != nil || hostInfo == nil || hostInfo.ID != hostID {
			continue
		}
		if _, err := s.StartSTXJavaProxy(ctx, clusterInfo.ID); err != nil {
			logger.WarnF(ctx, "[Cluster] ensure stx-java-proxy for cluster %d on agent ready failed: %v / Agent 就绪后拉起集群 %d 的 stx-java-proxy 失败: %v", clusterInfo.ID, err, clusterInfo.ID, err)
			continue
		}
		logger.InfoF(ctx, "[Cluster] ensured stx-java-proxy for cluster %d after agent ready / Agent 就绪后已确保集群 %d 的 stx-java-proxy 运行", clusterInfo.ID, clusterInfo.ID)
	}
}

func (s *Service) pickSTXJavaProxyNode(ctx context.Context, clusterID uint) (*NodeInfo, *HostInfo, error) {
	nodes, err := s.GetNodes(ctx, clusterID)
	if err != nil {
		return nil, nil, err
	}
	for _, node := range nodes {
		if node == nil || !node.IsOnline {
			continue
		}
		if node.Role != NodeRoleMaster && node.Role != NodeRoleMasterWorker {
			continue
		}
		hostInfo, err := s.hostProvider.GetHostByID(ctx, node.HostID)
		if err != nil {
			continue
		}
		if hostInfo == nil || !hostInfo.IsOnline(s.heartbeatTimeout) || strings.TrimSpace(hostInfo.AgentID) == "" {
			continue
		}
		return node, hostInfo, nil
	}
	return nil, nil, fmt.Errorf("no online master node with agent available for stx-java-proxy management")
}

func decodeSTXJavaProxyStatus(clusterInfo *Cluster, node *NodeInfo, hostInfo *HostInfo, message string) *STXJavaProxyStatus {
	status := &STXJavaProxyStatus{
		ClusterID:   clusterInfo.ID,
		ClusterName: clusterInfo.Name,
		Version:     clusterInfo.Version,
		Service:     "stx_java_proxy",
		Message:     parseCommandMessage(message),
	}
	if node != nil {
		status.NodeID = node.ID
		status.HostID = node.HostID
		status.HostName = node.HostName
		status.HostIP = node.HostIP
		status.Role = string(node.Role)
		status.InstallDir = node.InstallDir
	}
	if hostInfo != nil {
		status.HostName = firstNonEmpty(status.HostName, hostInfo.Name)
		status.HostIP = firstNonEmpty(status.HostIP, hostInfo.IPAddress)
	}

	var payload STXJavaProxyStatus
	if err := json.Unmarshal([]byte(strings.TrimSpace(message)), &payload); err == nil {
		if payload.Service != "" {
			status.Service = payload.Service
		}
		status.Managed = payload.Managed
		status.Running = payload.Running
		status.Healthy = payload.Healthy
		status.Endpoint = payload.Endpoint
		status.Port = payload.Port
		status.PID = payload.PID
		status.LogPath = payload.LogPath
		status.Message = firstNonEmpty(payload.Message, status.Message)
		status.JvmOpts = payload.JvmOpts
		status.JvmMemory = payload.JvmMemory
	}
	if status.JvmOpts == "" && clusterInfo != nil {
		if proxyCfg := clusterInfo.Config.GetSTXJavaProxyConfig(); proxyCfg != nil {
			status.JvmOpts = proxyCfg.JvmOpts
		}
	}
	return status
}
