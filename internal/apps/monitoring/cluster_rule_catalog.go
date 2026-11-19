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
	"fmt"
	"strconv"
	"strings"
	"time"
)

// clusterRuleEngine 标记规则由哪个判定入口消费；availability 引擎的规则本阶段只登记目录，
// 判定实现由后续阶段补充。
// clusterRuleEngine marks which evaluation entry consumes a rule; availability-engine rules are
// catalogued now while their evaluator lands in a later phase.
type clusterRuleEngine string

const (
	clusterRuleEngineEvent        clusterRuleEngine = "event"
	clusterRuleEngineTSDB         clusterRuleEngine = "tsdb"
	clusterRuleEngineSync         clusterRuleEngine = "sync"
	clusterRuleEngineAvailability clusterRuleEngine = "availability"
)

// 集群规则的通知方式取值："" 与 "default" 都表示沿用默认通知；"none" 等价 warning，不外投。
// Cluster-rule notification mode values: "" and "default" both mean the default notification;
// "none" behaves like warning severity and never delivers externally.
const (
	clusterRuleNotifyModeDefault = "default"
	clusterRuleNotifyModeCustom  = "custom"
	clusterRuleNotifyModeNone    = "none"
)

// 阈值单位：percent 面向用户百分比，表达式内部自行换算（如 CPU 需换算成 ratio）。
// Threshold units: percent is the user-facing unit; expressions convert internally (e.g. CPU
// becomes a ratio).
const (
	clusterRuleThresholdUnitPercent = "percent"
	clusterRuleThresholdUnitCount   = "count"
	clusterRuleThresholdUnitWorkers = "workers"
)

// ThresholdSpec 描述一条规则可编辑阈值的算子、单位、默认值与允许范围；算子不开放编辑。
// ThresholdSpec describes one rule's editable threshold: fixed operator (not user-editable),
// unit, default and the allowed range.
type ThresholdSpec struct {
	Operator string
	Unit     string
	Default  float64
	Min      float64
	Max      float64
}

// ClusterRuleDefinition 是集群级规则目录的唯一来源（取代原 YAML alerts 组、指标模板与
// monitoring_alert_rules 种子）。Expr 只给 tsdb 规则使用：按解析后的配置与集群匹配器
// 生成 PromQL。
// ClusterRuleDefinition is the single source of truth for cluster-level rules (replacing the
// YAML alerts group, metric templates and the monitoring_alert_rules seeds). Expr is only used
// by tsdb rules: it builds the PromQL from the resolved config and the cluster matcher.
type ClusterRuleDefinition struct {
	Key            string
	Name           string
	NameEN         string
	Description    string
	Engine         clusterRuleEngine
	Severity       AlertSeverity
	DefaultEnabled bool
	Threshold      *ThresholdSpec
	DefaultFor     time.Duration
	ForEditable    bool
	Expr           func(cfg *resolvedRuleConfig, clusterMatcher string) string
}

// clusterRuleAlertLabelManagedBy / clusterRuleAlertLabelRuleKey 是托管 TSDB 规则必带的标签，
// 投递缝按它们识别并解析规则。
// clusterRuleAlertLabelManagedBy / clusterRuleAlertLabelRuleKey are the labels every managed
// tsdb rule carries; the delivery seam resolves the rule from them.
const (
	clusterRuleAlertLabelManagedBy = "managed_by"
	clusterRuleAlertLabelRuleKey   = "rule_key"
)

const clusterRuleEngineMetricJob = `job="seatunnel_engine_http"`

// 集群级 tsdb 规则聚合输出标签：让告警自带 cluster_id / cluster_name，供投递缝与告警中心定位集群。
// The label set cluster-level tsdb rules aggregate by: alerts carry cluster_id / cluster_name so
// the delivery seam and the alert center can locate the cluster.
const clusterRuleAlertGroupBy = "cluster_id, cluster_name, cluster, instance"

// formatClusterRuleThreshold 把阈值格式化成 PromQL 里的普通十进制字面量。
// formatClusterRuleThreshold renders a threshold as a plain decimal literal for PromQL.
func formatClusterRuleThreshold(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

// thresholdValue 取解析后的生效阈值；没有解析值时回落目录默认。
// thresholdValue returns the resolved effective threshold, falling back to the catalog default.
func (c *resolvedRuleConfig) thresholdValue() float64 {
	if c == nil {
		return 0
	}
	if c.Threshold != nil {
		return *c.Threshold
	}
	if c.Definition != nil && c.Definition.Threshold != nil {
		return c.Definition.Threshold.Default
	}
	return 0
}

// tsdbMetricSelector 生成 job + 集群匹配器的指标选择器；集群匹配器为空时全局生效。
// tsdbMetricSelector builds a selector with the engine job and the cluster matcher; an empty
// matcher means the rule applies globally.
func tsdbMetricSelector(metric string, clusterMatcher string, extra ...string) string {
	matchers := make([]string, 0, len(extra)+2)
	matchers = append(matchers, clusterRuleEngineMetricJob)
	if trimmed := strings.TrimSpace(clusterMatcher); trimmed != "" {
		matchers = append(matchers, trimmed)
	}
	matchers = append(matchers, extra...)
	return buildPrometheusSelector(metric, matchers...)
}

// tsdbThresholdExpr 是最常见的「选择器 > 阈值」形状。
// tsdbThresholdExpr is the most common "selector > threshold" shape.
func tsdbThresholdExpr(selector string, operator string, cfg *resolvedRuleConfig) string {
	op := strings.TrimSpace(operator)
	if op == "" {
		op = ">"
	}
	return fmt.Sprintf("%s %s %s", selector, op, formatClusterRuleThreshold(cfg.thresholdValue()))
}

func tsdbExprDeadlockedThreads(cfg *resolvedRuleConfig, clusterMatcher string) string {
	return tsdbThresholdExpr(tsdbMetricSelector("jvm_threads_deadlocked", clusterMatcher), ">", cfg)
}

func tsdbExprJobThreadPoolRejection(cfg *resolvedRuleConfig, clusterMatcher string) string {
	// 内置录制序列按 (cluster, instance) 聚合，不带 cluster_id；这里内联到原始指标，让集群匹配器生效。
	// The builtin recorded series aggregates by (cluster, instance) and drops cluster_id; inlining
	// the raw metric keeps the cluster matcher effective.
	return tsdbThresholdExpr(
		fmt.Sprintf("sum by (%s) (rate(%s[5m]))", clusterRuleAlertGroupBy, tsdbMetricSelector("job_thread_pool_rejection_total", clusterMatcher)),
		">", cfg,
	)
}

func tsdbExprMemoryUsage(cfg *resolvedRuleConfig, clusterMatcher string) string {
	// 录制规则不裁剪标签，seatunnel:jvm_heap_usage_percent 自带 cluster_id，可直接加集群匹配器。
	// The recording rule preserves labels, so seatunnel:jvm_heap_usage_percent carries cluster_id
	// and accepts the cluster matcher directly.
	return tsdbThresholdExpr(tsdbMetricSelector("seatunnel:jvm_heap_usage_percent", clusterMatcher), ">", cfg)
}

func tsdbExprFDUsage(cfg *resolvedRuleConfig, clusterMatcher string) string {
	return tsdbThresholdExpr(tsdbMetricSelector("seatunnel:fd_usage_percent", clusterMatcher), ">", cfg)
}

func tsdbExprGCTime(cfg *resolvedRuleConfig, clusterMatcher string) string {
	return tsdbThresholdExpr(tsdbMetricSelector("seatunnel:gc_time_ratio_percent5m", clusterMatcher), ">", cfg)
}

func tsdbExprCPUUsage(cfg *resolvedRuleConfig, clusterMatcher string) string {
	// 用户阈值是百分比，内置表达式以 CPU 核数（ratio）比较，这里换算回 ratio。
	// The user threshold is a percent while the builtin expression compares CPU cores (ratio),
	// so it is converted back to a ratio here.
	threshold := cfg.thresholdValue() / 100
	return fmt.Sprintf("rate(%s[5m]) > %s",
		tsdbMetricSelector("process_cpu_seconds_total", clusterMatcher),
		formatClusterRuleThreshold(threshold))
}

func tsdbExprJobThreadPoolQueueBacklog(cfg *resolvedRuleConfig, clusterMatcher string) string {
	return tsdbThresholdExpr(tsdbMetricSelector("job_thread_pool_queueTaskCount", clusterMatcher), ">", cfg)
}

func tsdbExprCoordinatorBacklogGrowing(cfg *resolvedRuleConfig, clusterMatcher string) string {
	depth := fmt.Sprintf("max by (%s) (%s)", clusterRuleAlertGroupBy, tsdbMetricSelector("job_thread_pool_queueTaskCount", clusterMatcher))
	submitRate := fmt.Sprintf("sum by (%s) (rate(%s[5m]))", clusterRuleAlertGroupBy, tsdbMetricSelector("job_thread_pool_task_total", clusterMatcher))
	completeRate := fmt.Sprintf("sum by (%s) (rate(%s[5m]))", clusterRuleAlertGroupBy, tsdbMetricSelector("job_thread_pool_completedTask_total", clusterMatcher))
	return fmt.Sprintf("%s > %s and (%s - %s) > 0.5",
		depth, formatClusterRuleThreshold(cfg.thresholdValue()), submitRate, completeRate)
}

func tsdbExprFailedJobs(cfg *resolvedRuleConfig, clusterMatcher string) string {
	return tsdbThresholdExpr(
		fmt.Sprintf("sum by (%s) (%s)", clusterRuleAlertGroupBy, tsdbMetricSelector("job_count", clusterMatcher, `state="FAILED"`)),
		">", cfg,
	)
}

func tsdbExprSplitBrainRisk(_ *resolvedRuleConfig, clusterMatcher string) string {
	return fmt.Sprintf("%s == 0 or %s == 0",
		tsdbMetricSelector("hazelcast_partition_isClusterSafe", clusterMatcher),
		tsdbMetricSelector("hazelcast_partition_isLocalMemberSafe", clusterMatcher))
}

// clusterRuleCatalog 列出全部 21 条集群级规则（PRD D4 的默认开关、D6 的阈值与持续时间、D9 的默认 warning 级别）。
// clusterRuleCatalog lists all 21 cluster-level rules (PRD D4 default switches, D6 thresholds and
// durations, D9 default warning severity).
var clusterRuleCatalog = []*ClusterRuleDefinition{
	{
		Key:            AlertRuleKeyNodeOffline,
		Name:           "节点离线",
		NameEN:         "Node offline",
		Description:    "节点在宽限窗口后仍不可用，覆盖主机离线和进程持续不可见两类场景",
		Engine:         clusterRuleEngineAvailability,
		Severity:       AlertSeverityWarning,
		DefaultEnabled: true,
		DefaultFor:     2 * time.Minute,
		ForEditable:    true,
	},
	{
		Key:            "master_unavailable",
		Name:           "Master 不可用",
		NameEN:         "Master unavailable",
		Description:    "集群内已没有健康的 Master / Coordinator 节点",
		Engine:         clusterRuleEngineAvailability,
		Severity:       AlertSeverityWarning,
		DefaultEnabled: true,
		DefaultFor:     time.Minute,
		ForEditable:    true,
	},
	{
		Key:            "worker_insufficient",
		Name:           "可用 Worker 不足",
		NameEN:         "Insufficient workers",
		Description:    "集群中健康 Worker 数量持续低于预期基线",
		Engine:         clusterRuleEngineAvailability,
		Severity:       AlertSeverityWarning,
		DefaultEnabled: true,
		// 阈值缺省表示「等于部署的 worker 数」，由可用性评估器解释。
		// A missing threshold means "equal to the deployed worker count", interpreted by the
		// availability evaluator.
		Threshold:   &ThresholdSpec{Operator: "<", Unit: clusterRuleThresholdUnitWorkers, Default: 0, Min: 1, Max: 10000},
		DefaultFor:  2 * time.Minute,
		ForEditable: true,
	},
	{
		Key:            "agent_offline",
		Name:           "Agent 离线",
		NameEN:         "Agent offline",
		Description:    "控制面失去了集群运维所需的 Agent 连接",
		Engine:         clusterRuleEngineAvailability,
		Severity:       AlertSeverityWarning,
		DefaultEnabled: true,
		DefaultFor:     2 * time.Minute,
		ForEditable:    true,
	},
	{
		Key:            AlertRuleKeyProcessCrashed,
		Name:           "进程崩溃",
		NameEN:         "Process crashed",
		Description:    "受管 SeaTunnel 进程发生崩溃",
		Engine:         clusterRuleEngineEvent,
		Severity:       AlertSeverityWarning,
		DefaultEnabled: true,
	},
	{
		Key:            AlertRuleKeyProcessRestartFailed,
		Name:           "进程重启失败",
		NameEN:         "Process restart failed",
		Description:    "自动重启无法恢复故障进程",
		Engine:         clusterRuleEngineEvent,
		Severity:       AlertSeverityWarning,
		DefaultEnabled: true,
	},
	{
		Key:            AlertRuleKeyProcessRestartLimitReached,
		Name:           "达到重启上限",
		NameEN:         "Restart limit reached",
		Description:    "进程进入类似 crash-loop 的重启上限异常",
		Engine:         clusterRuleEngineEvent,
		Severity:       AlertSeverityWarning,
		DefaultEnabled: true,
	},
	{
		Key:            "sync_status_unavailable",
		Name:           "作业状态获取失败",
		NameEN:         "Job status unavailable",
		Description:    "集群的作业状态持续获取不到，期间作业状态可能已经过时",
		Engine:         clusterRuleEngineSync,
		Severity:       AlertSeverityWarning,
		DefaultEnabled: true,
		DefaultFor:     5 * time.Minute,
		ForEditable:    true,
	},
	{
		Key:            "split_brain_risk",
		Name:           "分区不安全",
		NameEN:         "Partition unsafe",
		Description:    "Hazelcast 分区安全检查结果异常，存在脑裂风险",
		Engine:         clusterRuleEngineTSDB,
		Severity:       AlertSeverityWarning,
		DefaultEnabled: true,
		DefaultFor:     2 * time.Minute,
		ForEditable:    true,
		Expr:           tsdbExprSplitBrainRisk,
	},
	{
		Key:            "deadlocked_threads_detected",
		Name:           "线程死锁",
		NameEN:         "Deadlocked threads",
		Description:    "JVM 检测到死锁线程，任务执行与恢复流程可能被卡死",
		Engine:         clusterRuleEngineTSDB,
		Severity:       AlertSeverityWarning,
		DefaultEnabled: true,
		Threshold:      &ThresholdSpec{Operator: ">", Unit: clusterRuleThresholdUnitCount, Default: 0, Min: 0, Max: 100000},
		DefaultFor:     time.Minute,
		ForEditable:    true,
		Expr:           tsdbExprDeadlockedThreads,
	},
	{
		Key:            "job_thread_pool_rejection_high",
		Name:           "任务被拒绝",
		NameEN:         "Job thread pool rejecting tasks",
		Description:    "作业线程池开始拒绝任务而不是排队或执行",
		Engine:         clusterRuleEngineTSDB,
		Severity:       AlertSeverityWarning,
		DefaultEnabled: true,
		Threshold:      &ThresholdSpec{Operator: ">", Unit: clusterRuleThresholdUnitCount, Default: 0, Min: 0, Max: 1000000},
		DefaultFor:     5 * time.Minute,
		ForEditable:    true,
		Expr:           tsdbExprJobThreadPoolRejection,
	},
	{
		Key:            "memory_usage_high",
		Name:           "JVM 堆内存高",
		NameEN:         "JVM heap usage high",
		Description:    "SeaTunnel JVM 堆内存使用率持续超过阈值",
		Engine:         clusterRuleEngineTSDB,
		Severity:       AlertSeverityWarning,
		DefaultEnabled: true,
		Threshold:      &ThresholdSpec{Operator: ">", Unit: clusterRuleThresholdUnitPercent, Default: 85, Min: 1, Max: 100},
		DefaultFor:     10 * time.Minute,
		ForEditable:    true,
		Expr:           tsdbExprMemoryUsage,
	},
	{
		Key:            "fd_usage_high",
		Name:           "FD 使用率高",
		NameEN:         "FD usage high",
		Description:    "进程文件句柄使用率持续超过阈值，存在句柄耗尽风险",
		Engine:         clusterRuleEngineTSDB,
		Severity:       AlertSeverityWarning,
		DefaultEnabled: true,
		Threshold:      &ThresholdSpec{Operator: ">", Unit: clusterRuleThresholdUnitPercent, Default: 80, Min: 1, Max: 100},
		DefaultFor:     10 * time.Minute,
		ForEditable:    true,
		Expr:           tsdbExprFDUsage,
	},
	{
		Key:            "cpu_usage_high",
		Name:           "CPU 高",
		NameEN:         "CPU usage high",
		Description:    "SeaTunnel 进程 CPU 负载持续超过阈值（核数占比，不是宿主机总 CPU 百分比）",
		Engine:         clusterRuleEngineTSDB,
		Severity:       AlertSeverityWarning,
		DefaultEnabled: false,
		Threshold:      &ThresholdSpec{Operator: ">", Unit: clusterRuleThresholdUnitPercent, Default: 80, Min: 1, Max: 100},
		DefaultFor:     10 * time.Minute,
		ForEditable:    true,
		Expr:           tsdbExprCPUUsage,
	},
	{
		Key:            "gc_time_high",
		Name:           "GC 耗时高",
		NameEN:         "GC time high",
		Description:    "JVM GC 时间占比持续超过阈值",
		Engine:         clusterRuleEngineTSDB,
		Severity:       AlertSeverityWarning,
		DefaultEnabled: false,
		Threshold:      &ThresholdSpec{Operator: ">", Unit: clusterRuleThresholdUnitPercent, Default: 20, Min: 1, Max: 100},
		DefaultFor:     10 * time.Minute,
		ForEditable:    true,
		Expr:           tsdbExprGCTime,
	},
	{
		Key:            "job_thread_pool_queue_backlog_high",
		Name:           "线程池队列积压",
		NameEN:         "Job thread pool queue backlog",
		Description:    "作业线程池队列任务数持续超过阈值，可能演变为高延迟或饥饿",
		Engine:         clusterRuleEngineTSDB,
		Severity:       AlertSeverityWarning,
		DefaultEnabled: false,
		Threshold:      &ThresholdSpec{Operator: ">", Unit: clusterRuleThresholdUnitCount, Default: 100, Min: 1, Max: 1000000},
		DefaultFor:     5 * time.Minute,
		ForEditable:    true,
		Expr:           tsdbExprJobThreadPoolQueueBacklog,
	},
	{
		Key:            "coordinator_backlog_growing",
		Name:           "积压持续增长",
		NameEN:         "Coordinator backlog growing",
		Description:    "作业线程池队列深度超过阈值且提交速率持续高于完成速率",
		Engine:         clusterRuleEngineTSDB,
		Severity:       AlertSeverityWarning,
		DefaultEnabled: false,
		// 阈值含义是队列深度；「提交 > 完成」的另一半条件是固定表达式。
		// The threshold is the queue depth; the "submit outruns completion" half is a fixed
		// expression.
		Threshold:   &ThresholdSpec{Operator: ">", Unit: clusterRuleThresholdUnitCount, Default: 50, Min: 1, Max: 1000000},
		DefaultFor:  10 * time.Minute,
		ForEditable: true,
		Expr:        tsdbExprCoordinatorBacklogGrowing,
	},
	{
		Key:            "failed_jobs_high",
		Name:           "失败作业数过高",
		NameEN:         "Failed jobs high",
		Description:    "集群内 FAILED 状态的作业数量持续超过阈值",
		Engine:         clusterRuleEngineTSDB,
		Severity:       AlertSeverityWarning,
		DefaultEnabled: false,
		Threshold:      &ThresholdSpec{Operator: ">", Unit: clusterRuleThresholdUnitCount, Default: 0, Min: 0, Max: 1000000},
		DefaultFor:     time.Minute,
		ForEditable:    true,
		Expr:           tsdbExprFailedJobs,
	},
	{
		Key:            "sync_event_channel_degraded",
		Name:           "作业事件通道降级",
		NameEN:         "Event channel degraded",
		Description:    "集群的 SeaTunnel 事件通道从健康变为降级，时间线与 CDC 延迟数据可能中断",
		Engine:         clusterRuleEngineSync,
		Severity:       AlertSeverityWarning,
		DefaultEnabled: false,
	},
	{
		Key:            AlertRuleKeyClusterRestartRequested,
		Name:           "集群重启请求",
		NameEN:         "Cluster restart requested",
		Description:    "通过控制面发起的集群或节点重启",
		Engine:         clusterRuleEngineEvent,
		Severity:       AlertSeverityWarning,
		DefaultEnabled: false,
	},
	{
		Key:            AlertRuleKeyNodeStopRequested,
		Name:           "节点停止请求",
		NameEN:         "Node stop requested",
		Description:    "通过控制面手动停止的节点",
		Engine:         clusterRuleEngineEvent,
		Severity:       AlertSeverityWarning,
		DefaultEnabled: false,
	},
}

// lookupClusterRuleDefinition 按稳定键查目录条目；未知键返回 nil。
// lookupClusterRuleDefinition finds one catalog entry by its stable key; unknown keys yield nil.
func lookupClusterRuleDefinition(ruleKey string) *ClusterRuleDefinition {
	key := strings.TrimSpace(ruleKey)
	for _, def := range clusterRuleCatalog {
		if def != nil && def.Key == key {
			return def
		}
	}
	return nil
}
