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
	"strings"
	"time"

	"github.com/LeonYoah/stx/internal/config"
)

// StartJobReconciler 启动按集群对账 SeaTunnel 作业状态的后台任务。
// StartJobReconciler starts the per-cluster SeaTunnel job status reconciler.
func (s *Service) StartJobReconciler(ctx context.Context) {
	if s == nil || s.repo == nil || s.engineClient == nil {
		return
	}
	intervalSeconds := 10
	if config.Config != nil {
		if config.Config.Sync.JobReconcilerIntervalSeconds < 0 {
			return
		}
		if config.Config.Sync.JobReconcilerIntervalSeconds > 0 {
			intervalSeconds = config.Config.Sync.JobReconcilerIntervalSeconds
		}
	}
	interval := time.Duration(intervalSeconds) * time.Second
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = s.reconcileActiveJobs(ctx)
			}
		}
	}()
}

func (s *Service) reconcileActiveJobs(ctx context.Context) error {
	jobs, err := s.repo.ListNonTerminalJobInstances(ctx, 500)
	if err != nil || len(jobs) == 0 {
		return err
	}
	taskCluster := map[uint]uint{}
	grouped := map[uint][]*JobInstance{}
	for _, job := range jobs {
		if job == nil {
			continue
		}
		if submitSpecExecutionMode(job.SubmitSpec) == "local" {
			if _, refreshErr := s.refreshLocalJob(ctx, job); refreshErr != nil {
				s.recordJobSyncFailure(ctx, job)
			}
			continue
		}
		clusterID := clusterIDForJob(ctx, s, job, taskCluster)
		if clusterID == 0 {
			continue
		}
		grouped[clusterID] = append(grouped[clusterID], job)
	}
	for clusterID, items := range grouped {
		s.reconcileClusterJobs(ctx, clusterID, items)
	}
	return nil
}

func clusterIDForJob(ctx context.Context, s *Service, job *JobInstance, cache map[uint]uint) uint {
	if job == nil {
		return 0
	}
	if id := uintValue(job.SubmitSpec, "cluster_id"); id > 0 {
		return id
	}
	if job.TaskID == 0 || s == nil || s.repo == nil {
		return 0
	}
	if cached, ok := cache[job.TaskID]; ok {
		return cached
	}
	task, err := s.repo.GetTaskByID(ctx, job.TaskID)
	if err != nil || task == nil {
		return 0
	}
	cache[job.TaskID] = task.ClusterID
	return task.ClusterID
}

func (s *Service) reconcileClusterJobs(ctx context.Context, clusterID uint, jobs []*JobInstance) {
	if len(jobs) == 0 {
		return
	}
	var endpoint *EngineEndpoint
	if s.runtimeResolver != nil {
		resolved, err := s.runtimeResolver.ResolveEngineEndpoint(ctx, clusterID, nil)
		if err == nil {
			endpoint = resolved
		}
	}
	if endpoint == nil {
		for _, job := range jobs {
			endpoint = endpointFromSubmitSpec(job.SubmitSpec)
			if endpoint != nil {
				break
			}
		}
	}
	if endpoint == nil {
		for _, job := range jobs {
			s.recordJobSyncFailure(ctx, job)
		}
		return
	}

	running, runErr := s.engineClient.ListRunningJobs(ctx, endpoint)
	if runErr != nil {
		for _, job := range jobs {
			s.recordJobSyncFailure(ctx, job)
		}
		return
	}
	runningMap := map[string]*EngineJobListItem{}
	for _, item := range running {
		if item == nil {
			continue
		}
		runningMap[strings.TrimSpace(item.JobID)] = item
	}

	finishedMap := map[string]*EngineJobListItem{}
	for _, state := range []string{"", "FINISHED", "FAILED", "CANCELED", "CANCELLED"} {
		items, err := s.engineClient.ListFinishedJobs(ctx, endpoint, state)
		if err != nil {
			continue
		}
		for _, item := range items {
			if item == nil {
				continue
			}
			finishedMap[strings.TrimSpace(item.JobID)] = item
		}
	}

	for _, job := range jobs {
		if job == nil {
			continue
		}
		engineJobID := strings.TrimSpace(job.EngineJobID)
		if engineJobID == "" {
			continue
		}
		if item, ok := runningMap[engineJobID]; ok {
			if _, err := s.applyEngineJobInfoUpdate(ctx, job, engineListItemToInfo(item)); err != nil {
				s.recordJobSyncFailure(ctx, job)
			}
			continue
		}
		if item, ok := finishedMap[engineJobID]; ok {
			info := engineListItemToInfo(item)
			if isFinalEngineJobStatus(strings.TrimSpace(item.JobStatus)) && strings.TrimSpace(item.FinishedTime) != "" {
				if _, err := s.applyEngineJobInfoUpdate(ctx, job, info); err != nil {
					s.recordJobSyncFailure(ctx, job)
				}
				continue
			}
			detail, detailErr := s.engineClient.GetJobInfo(ctx, endpoint, engineJobID)
			if detailErr != nil {
				if _, err := s.applyEngineJobInfoUpdate(ctx, job, info); err != nil {
					s.recordJobSyncFailure(ctx, job)
				}
				continue
			}
			if _, err := s.applyEngineJobInfoUpdate(ctx, job, detail); err != nil {
				s.recordJobSyncFailure(ctx, job)
			}
			continue
		}
		// 列表里暂时看不到：可能是刚提交，或引擎侧已丢；低频打一次详情。
		// Not visible in list yet: may be freshly submitted or missing on engine; fetch detail occasionally.
		if time.Since(job.UpdatedAt) < 15*time.Second {
			continue
		}
		detail, detailErr := s.engineClient.GetJobInfo(ctx, endpoint, engineJobID)
		if detailErr != nil {
			s.recordJobSyncFailure(ctx, job)
			continue
		}
		if _, err := s.applyEngineJobInfoUpdate(ctx, job, detail); err != nil {
			s.recordJobSyncFailure(ctx, job)
		}
	}
}
