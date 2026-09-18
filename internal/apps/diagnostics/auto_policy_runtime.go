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

package diagnostics

import (
	"context"
	"log"
	"time"

	"github.com/LeonYoah/stx/internal/apps/cluster"
)

const autoPolicyEvaluationInterval = time.Minute

// StartAutoPolicyRuntime starts the background evaluator for scheduled and Prometheus-backed auto policies.
// StartAutoPolicyRuntime 启动定时巡检与 Prometheus 自动策略的后台评估循环。
func (s *Service) StartAutoPolicyRuntime(ctx context.Context) {
	if s == nil || s.policyChecker == nil || s.clusterService == nil {
		return
	}

	s.autoPolicyRuntime.Do(func() {
		go func() {
			ticker := time.NewTicker(autoPolicyEvaluationInterval)
			defer ticker.Stop()

			for {
				s.runAutoPolicyEvaluationRound(ctx, time.Now().UTC())

				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
				}
			}
		}()
	})
}

func (s *Service) runAutoPolicyEvaluationRound(ctx context.Context, now time.Time) {
	if s == nil || s.policyChecker == nil || s.clusterService == nil {
		return
	}

	clusters, _, err := s.clusterService.List(ctx, &cluster.ClusterFilter{Page: 1, PageSize: 1000})
	if err != nil {
		log.Printf("[DiagnosticsAutoPolicy] list clusters failed: %v", err)
		return
	}

	for _, item := range clusters {
		if item == nil || item.ID == 0 {
			continue
		}
		if err := s.policyChecker.CheckScheduledPolicies(ctx, item.ID, now); err != nil {
			log.Printf("[DiagnosticsAutoPolicy] scheduled evaluation failed: cluster_id=%d err=%v", item.ID, err)
		}
		if err := s.policyChecker.CheckPrometheusPolicies(ctx, item.ID, now); err != nil {
			log.Printf("[DiagnosticsAutoPolicy] prometheus evaluation failed: cluster_id=%d err=%v", item.ID, err)
		}
	}
}
