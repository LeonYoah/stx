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

const (
	// jobObsCollectorInterval 是 Checkpoint / busy / backpressure 后台采集间隔。
	// jobObsCollectorInterval is the background interval for Checkpoint / busy / backpressure collection.
	jobObsCollectorInterval = 60 * time.Second
	// jobObsCollectorWindowMs 是采集 busy/bp 时请求引擎的窗口。
	// jobObsCollectorWindowMs is the engine window requested when collecting busy/bp.
	jobObsCollectorWindowMs int64 = 180000
)

// StartJobObservabilityCollector 在 STX observability 开启时，后台采集运行中作业的 Checkpoint；
// busy/backpressure（realtime）仅在作业开启引擎可观测性时采集。
// StartJobObservabilityCollector starts background Checkpoint collection for running jobs when STX observability
// is enabled; busy/backpressure (realtime) still requires the job's engine observability.
func (s *Service) StartJobObservabilityCollector(ctx context.Context) {
	if s == nil || s.repo == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(jobObsCollectorInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.collectJobObservabilityMetrics(ctx)
			}
		}
	}()
}

func (s *Service) collectJobObservabilityMetrics(ctx context.Context) {
	if config.Config == nil || !config.Config.Observability.Enabled {
		return
	}
	if s.repo == nil || s.engineClient == nil {
		return
	}
	jobs, err := s.repo.ListNonTerminalJobInstances(ctx, 500)
	if err != nil || len(jobs) == 0 {
		return
	}
	exporter := getJobPrometheusExporter()
	for _, job := range jobs {
		if job == nil || !isRealtimeObservableStatus(job.Status) {
			continue
		}
		// Checkpoint is core engine REST (/jobs/checkpoints); gate only on STX observability (checked above).
		// Checkpoint 是引擎核心 REST（/jobs/checkpoints），只受上方 STX observability 开关约束。
		s.collectOneJobCheckpoint(ctx, job, exporter)
		// Busy / backpressure still require engine.observability on the job.
		// 忙碌度 / 背压仍要求作业开启 engine.observability。
		if jobHasEngineObservability(job) {
			s.collectOneJobRealtime(ctx, job, exporter)
		}
	}
}

func (s *Service) collectOneJobCheckpoint(ctx context.Context, job *JobInstance, exporter *jobPrometheusExporter) {
	if job == nil || exporter == nil || s.engineClient == nil || s.runtimeResolver == nil {
		return
	}
	if strings.TrimSpace(job.EngineJobID) == "" {
		return
	}
	clusterID := clusterIDForJob(ctx, s, job, map[uint]uint{})
	if clusterID == 0 {
		return
	}
	endpoint, err := s.runtimeResolver.ResolveEngineEndpoint(ctx, clusterID, nil)
	if err != nil || endpoint == nil {
		return
	}
	overview, err := s.engineClient.GetJobCheckpointOverview(ctx, endpoint, job.EngineJobID)
	if err != nil || overview == nil {
		return
	}
	var latestDur float64 = -1
	var failedTotal float64
	var latestTs int64
	for _, pipe := range overview.Pipelines {
		if pipe == nil {
			continue
		}
		if pipe.Counts != nil {
			failedTotal += float64(pipe.Counts["FAILED"] + pipe.Counts["failed"] + pipe.Counts["Failed"])
		}
		if cp := pipe.LatestCompleted; cp != nil {
			if cp.CompletedTimestamp >= latestTs {
				latestTs = cp.CompletedTimestamp
				latestDur = float64(cp.DurationMillis) / 1000.0
			}
		}
		if cp := pipe.LatestFailed; cp != nil {
			// Count presence of a latest failed as at least one failure signal when counts map is empty.
			// counts 为空时，存在 LatestFailed 也至少记一次失败信号。
			if pipe.Counts == nil {
				failedTotal++
			}
		}
	}
	exporter.observeCheckpoint(job, latestDur, failedTotal)
}

func (s *Service) collectOneJobRealtime(ctx context.Context, job *JobInstance, exporter *jobPrometheusExporter) {
	if job == nil || exporter == nil {
		return
	}
	// Reuse the coalesced realtime path so master discovery / caches stay shared with the UI.
	// 复用带合并的 realtime 路径，与 UI 共享 master 发现与缓存。
	result, err := s.getJobRealtime(ctx, job, jobObsCollectorWindowMs)
	if err != nil || result == nil || !result.Available {
		return
	}
	exporter.observeRealtime(job, result.Vertices, result.Edges)
}

