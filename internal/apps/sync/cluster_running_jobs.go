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

package sync

import (
	"context"
	"errors"
	"sort"
	"strings"
)

// ErrClusterEngineUnavailable 无法解析集群引擎地址或未配置引擎客户端，运行中作业未知。
// ErrClusterEngineUnavailable means the cluster engine endpoint cannot be resolved or no engine client is
// configured, so running jobs are unknown.
var ErrClusterEngineUnavailable = errors.New("sync: cluster engine is unavailable / 集群引擎不可用")

// ListClusterRunningJobs 按集群维度读取 SeaTunnel 引擎当前运行中的作业（/running-jobs），供集群扩缩容护栏使用。
// 结果按作业编号排序且永不为 nil；引擎不可达时返回错误，调用方据此把运行中作业视为未知。
// ListClusterRunningJobs reads the jobs currently running on a cluster's SeaTunnel engine (/running-jobs) for
// cluster scaling guardrails. The result is sorted by job ID and never nil; an unreachable engine returns an
// error so callers can treat running jobs as unknown.
func (s *Service) ListClusterRunningJobs(ctx context.Context, clusterID uint) ([]*EngineJobListItem, error) {
	items := make([]*EngineJobListItem, 0)
	if s == nil || s.engineClient == nil || s.runtimeResolver == nil {
		return items, ErrClusterEngineUnavailable
	}
	endpoint, err := s.runtimeResolver.ResolveEngineEndpoint(ctx, clusterID, nil)
	if err != nil {
		return items, errors.Join(ErrClusterEngineUnavailable, err)
	}
	if endpoint == nil {
		return items, ErrClusterEngineUnavailable
	}
	running, err := s.engineClient.ListRunningJobs(ctx, endpoint)
	if err != nil {
		return items, err
	}
	for _, item := range running {
		if item == nil || strings.TrimSpace(item.JobID) == "" {
			continue
		}
		items = append(items, item)
	}
	sort.SliceStable(items, func(i, j int) bool {
		return strings.TrimSpace(items[i].JobID) < strings.TrimSpace(items[j].JobID)
	})
	return items, nil
}
