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
	"time"

	"github.com/LeonYoah/stx/internal/pkg/timex"
)

// EngineLocation 返回该集群解析 SeaTunnel 无偏移时间应使用的时区：优先在线 Master（或 master_worker）心跳上报的 IANA 时区，否则任一 Master 已上报的时区。
// EngineLocation is the zone for naive SeaTunnel times in this cluster: the online master (or master_worker) heartbeat
// IANA zone first, otherwise any master's last reported zone.
// engineLocationQueryTimeout 是 GetNodes 查库的上限：调用方 ctx 没有截止时间时才套上，避免 Background 卡住作业对账。
// engineLocationQueryTimeout caps the GetNodes DB lookup; it is applied only when the caller ctx has no deadline so
// a Background ctx cannot stall job reconcile.
const engineLocationQueryTimeout = 2 * time.Second

func (s *Service) EngineLocation(ctx context.Context, clusterID uint) *time.Location {
	if s == nil || s.hostProvider == nil || clusterID == 0 {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, engineLocationQueryTimeout)
		defer cancel()
	}
	if _, hostInfo, err := s.pickSTXJavaProxyNode(ctx, clusterID); err == nil && hostInfo != nil {
		if loc := timex.LoadIANA(hostInfo.Timezone); loc != nil {
			return loc
		}
	}
	nodes, err := s.GetNodes(ctx, clusterID)
	if err != nil {
		return nil
	}
	for _, node := range nodes {
		if node == nil {
			continue
		}
		if node.Role != NodeRoleMaster && node.Role != NodeRoleMasterWorker {
			continue
		}
		hostInfo, err := s.hostProvider.GetHostByID(ctx, node.HostID)
		if err != nil || hostInfo == nil {
			continue
		}
		if loc := timex.LoadIANA(hostInfo.Timezone); loc != nil {
			return loc
		}
	}
	return nil
}
