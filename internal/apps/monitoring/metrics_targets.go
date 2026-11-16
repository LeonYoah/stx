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

package monitoring

import (
	"context"
	"strconv"
	"strings"

	"github.com/LeonYoah/stx/internal/pkg/embedts"
)

// scrapeJobName 是 SeaTunnel 引擎抓取目标的 job 标签值（沿用退役 prometheus.yml 的 job_name）。
// scrapeJobName is the job label value for SeaTunnel engine scrape targets (kept from the
// retired prometheus.yml job_name).
const scrapeJobName = "seatunnel_engine_http"

// ScrapeTargets 实现 embedts.MetricsTargetProvider：把受管集群在线节点翻译成抓取目标集。
// 注入标签与原 HTTP SD 目标组标签保持一致（job/instance/cluster/cluster_id/cluster_name/env），
// 保证既有 PromQL 查询（大盘、告警模板）的标签匹配不漂移。
// ScrapeTargets implements embedts.MetricsTargetProvider: it translates online nodes of managed
// clusters into scrape targets. Injected labels stay identical to the retired HTTP SD target
// groups (job/instance/cluster/cluster_id/cluster_name/env) so existing PromQL queries
// (dashboards, alert templates) keep matching the same labels.
func (s *Service) ScrapeTargets(ctx context.Context) ([]embedts.ScrapeTarget, error) {
	targets, err := s.collectManagedMetricsTargets(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]embedts.ScrapeTarget, 0, len(targets))
	for _, item := range targets {
		if item == nil {
			continue
		}
		clusterName := strings.TrimSpace(item.ClusterName)
		if clusterName == "" {
			clusterName = "unknown"
		}
		env := strings.TrimSpace(item.Env)
		if env == "" {
			env = "unknown"
		}
		clusterID := strconv.FormatUint(uint64(item.ClusterID), 10)
		if item.ClusterID == 0 {
			clusterID = "static"
		}
		out = append(out, embedts.ScrapeTarget{
			URL: item.ProbeURL,
			Labels: map[string]string{
				"job":          scrapeJobName,
				"instance":     item.Target,
				"cluster":      clusterName, // 大盘 PromQL 变量按 cluster 过滤 / dashboard PromQL filters on cluster
				"cluster_name": clusterName,
				"cluster_id":   clusterID,
				"env":          env,
			},
		})
	}
	return out, nil
}
