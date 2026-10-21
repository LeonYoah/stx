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

package stupgrade

import (
	"context"
	"fmt"
	"strings"
	"time"

	clusterapp "github.com/LeonYoah/stx/internal/apps/cluster"
)

const (
	// defaultRuntimeHealthTimeout 是升级/回滚健康检查等待 SeaTunnel 进程与集群就绪的默认上限。
	// defaultRuntimeHealthTimeout is the default upper bound for upgrade/rollback health checks to wait
	// for SeaTunnel processes and cluster membership to become ready.
	defaultRuntimeHealthTimeout = 3 * time.Minute
	// defaultRuntimeHealthInterval 是健康检查的轮询间隔。
	// defaultRuntimeHealthInterval is the polling interval of health checks.
	defaultRuntimeHealthInterval = 5 * time.Second
)

// ClusterHealthReader 是可选能力：读取集群聚合健康状态（含进程与成员一致性）。
// ClusterHealthReader is an optional capability that reads the aggregated cluster health
// (including process liveness and member agreement).
type ClusterHealthReader interface {
	GetStatus(ctx context.Context, clusterID uint) (*clusterapp.ClusterStatusInfo, error)
}

// SetRuntimeHealthWait 设置健康检查的等待上限与轮询间隔（主要用于测试）。
// SetRuntimeHealthWait sets the health-check wait limit and polling interval (mainly for tests).
func (s *Service) SetRuntimeHealthWait(timeout, interval time.Duration) {
	if s == nil {
		return
	}
	s.healthTimeout = timeout
	s.healthInterval = interval
}

func (s *Service) runtimeHealthAttempts() (int, time.Duration) {
	timeout := s.healthTimeout
	if timeout <= 0 {
		timeout = defaultRuntimeHealthTimeout
	}
	interval := s.healthInterval
	if interval <= 0 {
		interval = defaultRuntimeHealthInterval
	}
	attempts := int(timeout / interval)
	if attempts < 1 {
		attempts = 1
	}
	return attempts, interval
}

// processCheckRole 把节点角色映射为 Agent check_process 使用的角色。
// processCheckRole maps a node role to the role understood by the Agent check_process command.
func processCheckRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "master":
		return "master"
	case "worker":
		return "worker"
	default:
		return "hybrid"
	}
}

// waitForNodeProcessRunning 轮询 Agent check_process，直到目标安装目录下的 SeaTunnel 进程真正运行。
// 集群启动是异步的（由 Agent 自动拉起），因此不能只检查一次。
// waitForNodeProcessRunning polls the Agent check_process command until the SeaTunnel process of the
// given install dir is actually running. Cluster start is asynchronous (the Agent auto-starts the process),
// so a single probe is not enough.
func (s *Service) waitForNodeProcessRunning(ctx context.Context, hostID uint, installDir, role string) error {
	attempts, interval := s.runtimeHealthAttempts()
	var lastReason string
	for attempt := 1; attempt <= attempts; attempt++ {
		agentID, connected := s.agentCommandSender.GetAgentByHostID(hostID)
		if !connected || agentID == "" {
			lastReason = fmt.Sprintf("agent of host %d is not connected / 主机 %d 的 Agent 未连接", hostID, hostID)
		} else {
			success, output, err := s.agentCommandSender.SendCommand(ctx, agentID, "check_process", map[string]string{
				"sub_command": "check_process",
				"role":        processCheckRole(role),
				"install_dir": installDir,
			})
			if err == nil && success {
				return nil
			}
			if err != nil {
				lastReason = err.Error()
			} else {
				lastReason = extractAgentResultMessage(output)
			}
		}
		if attempt < attempts {
			if err := s.waitForRetry(ctx, interval); err != nil {
				return err
			}
		}
	}
	return fmt.Errorf("SeaTunnel process in %s is not running: %s / %s 下的 SeaTunnel 进程未运行：%s", installDir, lastReason, installDir, lastReason)
}

// waitForClusterHealthy 在集群操作器支持时，轮询聚合健康状态直到 healthy（进程全部运行且成员一致）。
// waitForClusterHealthy polls the aggregated cluster health until healthy (all processes running and
// members in agreement) when the cluster operator supports it.
func (s *Service) waitForClusterHealthy(ctx context.Context, clusterID uint) error {
	reader, ok := s.clusterOperator.(ClusterHealthReader)
	if !ok || reader == nil {
		return nil
	}
	attempts, interval := s.runtimeHealthAttempts()
	lastReason := ""
	for attempt := 1; attempt <= attempts; attempt++ {
		status, err := reader.GetStatus(ctx, clusterID)
		if err != nil {
			lastReason = err.Error()
		} else if status != nil && status.HealthStatus == clusterapp.HealthStatusHealthy {
			return nil
		} else if status != nil {
			lastReason = strings.Join(status.HealthReasons, "; ")
			if lastReason == "" {
				lastReason = string(status.HealthStatus)
			}
		}
		if attempt < attempts {
			if err := s.waitForRetry(ctx, interval); err != nil {
				return err
			}
		}
	}
	return fmt.Errorf("cluster %d did not become healthy: %s / 集群 %d 未恢复健康：%s", clusterID, lastReason, clusterID, lastReason)
}
