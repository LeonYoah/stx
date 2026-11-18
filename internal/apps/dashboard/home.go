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
package dashboard

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/LeonYoah/stx/internal/apps/auth"
	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
	"github.com/LeonYoah/stx/internal/apps/monitoring"
	syncapp "github.com/LeonYoah/stx/internal/apps/sync"
	"github.com/LeonYoah/stx/internal/pkg/syncalert"
)

// homeAlertLimit 是「需要处理」里最多带入的告警条数。
// homeAlertLimit caps how many alerts enter the "needs attention" strip.
const homeAlertLimit = 5

// HomeJobSummaryProvider 提供首页作业汇总（由 sync 服务实现）。
// HomeJobSummaryProvider supplies the home job summary (implemented by the sync service).
type HomeJobSummaryProvider interface {
	GetHomeJobSummary(ctx context.Context, actor executionapp.Actor, clusterID uint, now time.Time) (*syncapp.HomeJobSummary, error)
}

// HomeMonitoringProvider 提供集群健康与告警（由 monitoring 服务实现）。
// HomeMonitoringProvider supplies cluster health and alerts (implemented by the monitoring service).
type HomeMonitoringProvider interface {
	GetClustersHealthFor(ctx context.Context, clusterID uint) (*monitoring.ClusterHealthData, error)
	ListAlertInstances(ctx context.Context, filter *monitoring.AlertInstanceFilter) (*monitoring.AlertInstanceListData, error)
}

// HomeClusterStats 是首页「集群健康」卡的数据。
// HomeClusterStats feeds the home "cluster health" card.
type HomeClusterStats struct {
	Total       int `json:"total"`
	Healthy     int `json:"healthy"`
	Degraded    int `json:"degraded"`
	Unhealthy   int `json:"unhealthy"`
	NodesOnline int `json:"nodes_online"`
	NodesTotal  int `json:"nodes_total"`
}

// HomeAlertStats 是首页「活跃告警」卡的数据。
// HomeAlertStats feeds the home "active alerts" card.
type HomeAlertStats struct {
	Active   int64 `json:"active"`
	Critical int64 `json:"critical"`
}

// HomeSummary 是 GET /api/v1/dashboard/home/summary 的返回。
// HomeSummary is the payload of GET /api/v1/dashboard/home/summary.
type HomeSummary struct {
	GeneratedAt time.Time                   `json:"generated_at"`
	ClusterID   uint                        `json:"cluster_id,omitempty"`
	Jobs        *syncapp.HomeJobSummary     `json:"jobs"`
	Clusters    HomeClusterStats            `json:"clusters"`
	Alerts      HomeAlertStats              `json:"alerts"`
	Attention   []syncapp.HomeAttentionItem `json:"attention"`
	// PartialErrors 记录取数失败的区块（jobs / clusters / alerts），其余区块照常返回。
	// PartialErrors lists blocks that failed to load (jobs / clusters / alerts); the rest still return.
	PartialErrors []string `json:"partial_errors,omitempty"`
}

// HomeService 组合作业、集群与告警数据生成首页汇总。
// HomeService combines job, cluster and alert data into the home summary.
type HomeService struct {
	jobs       HomeJobSummaryProvider
	monitoring HomeMonitoringProvider
	now        func() time.Time
}

// NewHomeService 创建首页汇总服务。
// NewHomeService creates the home summary service.
func NewHomeService(jobs HomeJobSummaryProvider, monitoringProvider HomeMonitoringProvider) *HomeService {
	return &HomeService{jobs: jobs, monitoring: monitoringProvider, now: time.Now}
}

// GetHomeSummary 生成首页汇总；单个区块失败只记入 PartialErrors，不让整页报错。
// GetHomeSummary builds the home summary; a failed block is only listed in PartialErrors so the page still loads.
func (s *HomeService) GetHomeSummary(ctx context.Context, actor executionapp.Actor, clusterID uint) *HomeSummary {
	now := s.now().UTC()
	out := &HomeSummary{GeneratedAt: now, ClusterID: clusterID, Attention: []syncapp.HomeAttentionItem{}}

	if s.jobs != nil {
		jobs, err := s.jobs.GetHomeJobSummary(ctx, actor, clusterID, now)
		if err != nil {
			out.PartialErrors = append(out.PartialErrors, "jobs")
		} else {
			out.Jobs = jobs
			out.Attention = append(out.Attention, jobs.Attention...)
		}
	}

	if s.monitoring != nil {
		if health, err := s.monitoring.GetClustersHealthFor(ctx, clusterID); err != nil {
			out.PartialErrors = append(out.PartialErrors, "clusters")
		} else {
			for _, item := range health.Clusters {
				if item == nil || (clusterID > 0 && item.ClusterID != clusterID) {
					continue
				}
				out.Clusters.Total++
				out.Clusters.NodesOnline += item.OnlineNodes
				out.Clusters.NodesTotal += item.TotalNodes
				out.Alerts.Active += item.ActiveAlerts
				out.Alerts.Critical += item.CriticalAlerts
				switch item.HealthStatus {
				case "healthy":
					out.Clusters.Healthy++
				case "degraded":
					out.Clusters.Degraded++
				case "unhealthy":
					out.Clusters.Unhealthy++
				}
			}
		}
		filter := &monitoring.AlertInstanceFilter{Status: monitoring.AlertDisplayStatusFiring, Page: 1, PageSize: homeAlertLimit}
		if clusterID > 0 {
			filter.ClusterID = strconv.FormatUint(uint64(clusterID), 10)
		}
		if alerts, err := s.monitoring.ListAlertInstances(ctx, filter); err != nil {
			out.PartialErrors = append(out.PartialErrors, "alerts")
		} else {
			for _, alert := range alerts.Alerts {
				if alert == nil {
					continue
				}
				severity := "warning"
				if alert.Severity == monitoring.AlertSeverityCritical {
					severity = "critical"
				}
				out.Attention = append(out.Attention, syncapp.HomeAttentionItem{
					Kind: "alert", Severity: severity, Title: alert.AlertName, Detail: alert.ClusterName,
					AlertID: alert.AlertID, At: alert.FiringAt, Viewable: true,
				})
			}
		}
	}
	out.Attention = s.suppressHandledJobFailures(ctx, out.Attention, clusterID)
	sortHomeAttention(out.Attention)
	return out
}

// suppressHandledJobFailures 把已被人工关闭的「作业失败」告警覆盖的失败作业条目从「需要处理」里去掉。
// 「已关闭」语义是"该任务此刻之前的失败已被人工处理"：按任务取最近一次关闭时间，条目时间早于它的不再提示；
// 关闭后再失败会开新的告警阶段、新条目照常出现。查询失败时原样返回——宁可多显示，也不能吞掉真实失败。
// suppressHandledJobFailures drops job_failed items already covered by a manually closed sync_job_failed
// alert. "Closed" means "this task's failures up to now are handled", so per task the latest close time wins
// and earlier items leave the strip; a failure after the close opens a new episode and still appears. On lookup
// failure the items pass through — over-showing is safer than swallowing a real failure.
func (s *HomeService) suppressHandledJobFailures(ctx context.Context, items []syncapp.HomeAttentionItem, clusterID uint) []syncapp.HomeAttentionItem {
	if s.monitoring == nil {
		return items
	}
	hasJobFailed := false
	for i := range items {
		if items[i].Kind == "job_failed" {
			hasJobFailed = true
			break
		}
	}
	if !hasJobFailed {
		return items
	}
	filter := &monitoring.AlertInstanceFilter{
		SourceType: monitoring.AlertSourceTypeJob,
		Status:     monitoring.AlertDisplayStatusClosed,
		Unbounded:  true,
	}
	if clusterID > 0 {
		filter.ClusterID = strconv.FormatUint(uint64(clusterID), 10)
	}
	closed, err := s.monitoring.ListAlertInstances(ctx, filter)
	if err != nil || closed == nil {
		return items
	}
	handledUntil := make(map[uint]time.Time, len(closed.Alerts))
	for _, alert := range closed.Alerts {
		if alert == nil || alert.ClosedAt == nil || alert.SourceRef == nil ||
			alert.RuleKey != syncalert.RuleJobFailed || alert.SourceRef.TaskID == 0 {
			continue
		}
		taskID := alert.SourceRef.TaskID
		if until, ok := handledUntil[taskID]; !ok || alert.ClosedAt.After(until) {
			handledUntil[taskID] = *alert.ClosedAt
		}
	}
	if len(handledUntil) == 0 {
		return items
	}
	kept := make([]syncapp.HomeAttentionItem, 0, len(items))
	for _, item := range items {
		if item.Kind == "job_failed" {
			if until, ok := handledUntil[item.TaskID]; ok && !item.At.After(until) {
				continue
			}
		}
		kept = append(kept, item)
	}
	return kept
}

// sortHomeAttention 严重在前，同级按时间倒序。
// sortHomeAttention puts critical first, then newest first.
func sortHomeAttention(items []syncapp.HomeAttentionItem) {
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && homeAttentionLess(items[j], items[j-1]); j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
}

func homeAttentionLess(a, b syncapp.HomeAttentionItem) bool {
	if a.Severity != b.Severity {
		return a.Severity == "critical"
	}
	return a.At.After(b.At)
}

// HomeHandler 处理首页汇总接口。
// HomeHandler serves the home summary endpoint.
type HomeHandler struct {
	service *HomeService
}

// NewHomeHandler 创建首页汇总处理器。
// NewHomeHandler creates the home summary handler.
func NewHomeHandler(service *HomeService) *HomeHandler {
	return &HomeHandler{service: service}
}

// GetHomeSummary 返回首页指标卡与「需要处理」条的数据。
// GetHomeSummary returns data for the home KPI cards and the "needs attention" strip.
// @Summary 首页汇总（作业 / 集群 / 告警 / 需要处理）
// @Tags Dashboard
// @Produce json
// @Param cluster_id query int false "集群 ID，空表示全部集群"
// @Success 200 {object} DashboardDataResponse{data=HomeSummary}
// @Failure 400 {object} DashboardDataResponse
// @Router /api/v1/dashboard/home/summary [get]
func (h *HomeHandler) GetHomeSummary(c *gin.Context) {
	var clusterID uint
	if raw := strings.TrimSpace(c.Query("cluster_id")); raw != "" {
		parsed, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, DashboardDataResponse{ErrorMsg: "invalid cluster_id"})
			return
		}
		clusterID = uint(parsed)
	}
	actor := executionapp.Actor{}
	if user := auth.GetUserFromContext(c); user != nil {
		actor = executionapp.Actor{UserID: uint64(user.ID), IsAdmin: user.IsAdmin}
	}
	c.JSON(http.StatusOK, DashboardDataResponse{Data: h.service.GetHomeSummary(c.Request.Context(), actor, clusterID)})
}
