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

package grpc

import (
	"context"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/LeonYoah/stx/internal/apps/host"
	pb "github.com/LeonYoah/stx/internal/proto/agent"
)

// ReportHostMetrics 接收 Agent 随心跳周期上报的 stx_host_* 样本并直写内嵌 tsdb（E3）。
// agent_id → 已绑定主机的解析走 agentManager；内嵌可观测未开启时静默丢弃（样本不进 tsdb，
// Agent 不需要感知）。
// ReportHostMetrics accepts the stx_host_* samples an Agent pushes on the heartbeat cadence and
// writes them into the embedded tsdb (E3). agent_id resolves to its bound host through
// agentManager; when embedded observability is off the round is dropped silently (samples
// never reach the tsdb and the agent need not care).
func (s *Server) ReportHostMetrics(ctx context.Context, req *pb.ReportHostMetricsRequest) (*pb.ReportHostMetricsResponse, error) {
	if req.AgentId == "" {
		return nil, status.Error(codes.InvalidArgument, "agent_id is required")
	}
	if len(req.Samples) == 0 {
		return &pb.ReportHostMetricsResponse{Success: true}, nil
	}
	conn, ok := s.agentManager.GetAgent(req.AgentId)
	if !ok || conn.HostID == 0 {
		// 未绑定主机的 Agent 仍然回成功并丢弃样本：未绑定是合法部署形态（装完还没
		// 分配主机），回 NotFound 只会让 Agent 每轮上报刷 warn，且改变不了结果。
		// An unbound agent still gets success with samples dropped: unbound is a
		// legitimate deployment state (installed but not yet assigned), and returning
		// NotFound would only spam the agent's warn log every round without changing
		// the outcome.
		return &pb.ReportHostMetricsResponse{Success: true}, nil
	}
	// 主机名 / IP 用于 tsdb 身份标签；查询失败降级为空串，不阻断写入。
	// Host name / IP feed the tsdb identity labels; a lookup failure degrades to empty
	// strings instead of blocking the write.
	hostName, instance := "", ""
	if s.hostService != nil {
		if h, err := s.hostService.Get(ctx, conn.HostID); err == nil && h != nil {
			hostName, instance = h.Name, h.IPAddress
		} else {
			s.logger.Warn("Failed to resolve host for metrics labels",
				zap.String("agent_id", req.AgentId), zap.Uint("host_id", conn.HostID), zap.Error(err))
		}
	}
	samples := make([]host.HostMetricSample, 0, len(req.Samples))
	for _, sample := range req.Samples {
		samples = append(samples, host.HostMetricSample{
			Name: sample.Name, Labels: sample.Labels, Value: sample.Value,
		})
	}
	written, err := host.WriteHostMetrics(ctx, conn.HostID, hostName, instance, samples)
	if err == nil && written > 0 {
		s.logger.Debug("Host metrics written",
			zap.String("agent_id", req.AgentId), zap.Uint("host_id", conn.HostID), zap.Int("samples", written))
	}
	if err != nil {
		s.logger.Error("Failed to write host metrics",
			zap.String("agent_id", req.AgentId), zap.Uint("host_id", conn.HostID), zap.Error(err))
		return nil, status.Error(codes.Internal, "failed to write host metrics")
	}
	return &pb.ReportHostMetricsResponse{Success: true}, nil
}
