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
	"encoding/json"
	"errors"
	"log"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/LeonYoah/stx/internal/apps/cluster"
	"github.com/LeonYoah/stx/internal/apps/monitor"
	"github.com/LeonYoah/stx/internal/pkg/promquery"
)

// 可用性告警的集群规则键（与 cluster_rule_catalog.go 的目录键一致）。
// Rule keys of the availability alerts (matching the catalog keys in cluster_rule_catalog.go).
const (
	clusterRuleKeyAgentOffline       = "agent_offline"
	clusterRuleKeyMasterUnavailable  = "master_unavailable"
	clusterRuleKeyWorkerInsufficient = "worker_insufficient"
)

// 节点离线的判定原因标识；写入事件 Details.reasons 供通知与排查使用。
// Reason identifiers for node_offline; written to the event Details.reasons for
// notifications and troubleshooting.
const (
	availabilityReasonProcessStopped      = "process_stopped"
	availabilityReasonProcessMissing      = "process_missing"
	availabilityReasonNodeError           = "node_error"
	availabilityReasonHostVisibilityLost  = "host_visibility_lost"
	availabilityReasonMetricsEndpointDown = "metrics_endpoint_down"
	availabilityReasonNodeStateDown       = "node_state_down"
	// availabilityReasonEngineNotReady 引擎半初始化/僵尸 JVM：up==1 且拿到了真实 JVM
	// 指标，但 node_state 样本完全缺失（不是 0 而是缺失）。
	// availabilityReasonEngineNotReady marks a half-initialized / zombie JVM: up==1 with real JVM
	// metrics present while node_state samples are entirely missing (absent, not zero).
	availabilityReasonEngineNotReady = "engine_not_ready"
)

const (
	availabilityReasonHostOffline             = "host_offline"
	availabilityReasonNoHealthyMaster         = "no_healthy_master"
	availabilityReasonWorkerBelowBaseline     = "worker_below_baseline"
	availabilityEvaluationMode                = "availability"
	availabilityDetailKeyFiringEventID        = "firing_event_id"
	availabilityDetailKeyLegacyOfflineEventID = "offline_event_id"
)

// availabilityRuleEventTypes 把每条可用性规则映射到它的 firing / recovered 事件类型；
// 告警实例、投递与生命周期解析都靠这个映射闭环。
// availabilityRuleEventTypes maps every availability rule to its firing / recovered event
// types; alert instances, delivery and lifecycle resolution all close the loop through it.
var availabilityRuleEventTypes = map[string][2]monitor.ProcessEventType{
	AlertRuleKeyNodeOffline:          {monitor.EventTypeNodeOffline, monitor.EventTypeNodeRecovered},
	clusterRuleKeyAgentOffline:       {monitor.EventTypeAgentOffline, monitor.EventTypeAgentRecovered},
	clusterRuleKeyMasterUnavailable:  {monitor.EventTypeMasterUnavailable, monitor.EventTypeMasterRecovered},
	clusterRuleKeyWorkerInsufficient: {monitor.EventTypeWorkerInsufficient, monitor.EventTypeWorkerRecovered},
}

// availabilityRecoveredFiringTypes 把恢复事件类型反查回触发类型，供恢复投递找到源事件。
// availabilityRecoveredFiringTypes maps a recovered event type back to its firing type so
// recovery delivery can locate the source event.
var availabilityRecoveredFiringTypes = map[monitor.ProcessEventType]monitor.ProcessEventType{
	monitor.EventTypeNodeRecovered:   monitor.EventTypeNodeOffline,
	monitor.EventTypeAgentRecovered:  monitor.EventTypeAgentOffline,
	monitor.EventTypeMasterRecovered: monitor.EventTypeMasterUnavailable,
	monitor.EventTypeWorkerRecovered: monitor.EventTypeWorkerInsufficient,
}

// availabilityFiringRecoveredTypes 由触发类型查恢复类型，供告警实例生命周期解析配对。
// availabilityFiringRecoveredTypes maps a firing event type to its recovered type for
// alert-instance lifecycle pairing.
var availabilityFiringRecoveredTypes = map[monitor.ProcessEventType]monitor.ProcessEventType{
	monitor.EventTypeNodeOffline:        monitor.EventTypeNodeRecovered,
	monitor.EventTypeAgentOffline:       monitor.EventTypeAgentRecovered,
	monitor.EventTypeMasterUnavailable:  monitor.EventTypeMasterRecovered,
	monitor.EventTypeWorkerInsufficient: monitor.EventTypeWorkerRecovered,
}

// availabilityPendingKey 标识 pending 计时器里的一个告警主体（规则 + 集群 + 节点 + 主机；
// 主机 / 集群级主体对应字段为 0）。
// availabilityPendingKey identifies one alert subject in the pending timer (rule + cluster +
// node + host; host / cluster scoped subjects leave the respective fields 0).
type availabilityPendingKey struct {
	RuleKey   string
	ClusterID uint
	NodeID    uint
	HostID    uint
}

// availabilityEvaluator 持有 for 语义的内存 pending 起点：主体一次不满足即清零，
// 与 Prometheus 的 pending→firing 语义一致；进程重启后靠事件表里的阶段去重而不是 pending。
// availabilityEvaluator holds the in-memory pending starts implementing the for semantics:
// a single healthy observation clears the start, matching Prometheus pending→firing;
// across restarts dedupe relies on the event-table episode, not on pending.
type availabilityEvaluator struct {
	mu      sync.Mutex
	pending map[availabilityPendingKey]time.Time
}

func newAvailabilityEvaluator() *availabilityEvaluator {
	return &availabilityEvaluator{pending: map[availabilityPendingKey]time.Time{}}
}

// mark 记录主体本轮不健康，返回 pending 起点与是否已持续满 forSeconds。
// mark records one unhealthy observation and returns the pending start plus whether it has
// been continuously unhealthy for forSeconds.
func (e *availabilityEvaluator) mark(key availabilityPendingKey, forSeconds int, now time.Time) (time.Time, bool) {
	if e == nil {
		return now.UTC(), true
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	start, ok := e.pending[key]
	if !ok || start.IsZero() {
		start = now.UTC()
		e.pending[key] = start
	}
	if forSeconds <= 0 {
		return start, true
	}
	return start, !now.UTC().Before(start.Add(time.Duration(forSeconds) * time.Second))
}

func (e *availabilityEvaluator) clear(key availabilityPendingKey) {
	if e == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.pending, key)
}

// prune 清理本轮已评估集群中主体已消失（节点 / 主机被移除）遗留的 pending 起点，
// 未评估到的集群（GetNodes 失败等）保留原状，下轮继续。
// prune drops pending starts whose subjects disappeared (node / host removed) inside
// clusters evaluated this tick; clusters that were not evaluated (e.g. GetNodes failed)
// keep their pending state for the next tick.
func (e *availabilityEvaluator) prune(evaluatedClusters map[uint]struct{}, seen map[availabilityPendingKey]struct{}) {
	if e == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for key := range e.pending {
		if _, ok := evaluatedClusters[key.ClusterID]; !ok {
			continue
		}
		if _, ok := seen[key]; !ok {
			delete(e.pending, key)
		}
	}
}

func (s *Service) ensureAvailabilityEvaluator() *availabilityEvaluator {
	if s.availabilityEvaluator == nil {
		s.availabilityEvaluator = newAvailabilityEvaluator()
	}
	return s.availabilityEvaluator
}

// availabilityMetricSignals 是本轮评估的内嵌 TSDB 即时快照：clusterID|instance → down/存在标记。
// availabilityMetricSignals is the per-tick embedded-TSDB instant snapshot:
// clusterID|instance → down/presence markers.
type availabilityMetricSignals struct {
	endpointDown     map[string]struct{}
	endpointUp       map[string]struct{}
	nodeStateDown    map[string]struct{}
	nodeStatePresent map[string]struct{}
	jvmMetricsSeen   map[string]struct{}
}

func availabilitySignalKey(clusterID uint, instance string) string {
	return strconv.FormatUint(uint64(clusterID), 10) + "|" + strings.TrimSpace(instance)
}

// collectAvailabilityMetricSignals 一次性查询 up / node_state 即时样本；观测关闭或查询失败时
// 返回空快照，进程侧原因照常评估（指标原因缺失不误报）。
// collectAvailabilityMetricQueries queries up / node_state instant samples once per tick; when
// observability is off or a query fails it returns an empty snapshot so process-side reasons
// still evaluate (missing metric evidence never raises a false positive).
func (s *Service) collectAvailabilityMetricSignals(ctx context.Context, now time.Time) *availabilityMetricSignals {
	signals := &availabilityMetricSignals{
		endpointDown:     map[string]struct{}{},
		endpointUp:       map[string]struct{}{},
		nodeStateDown:    map[string]struct{}{},
		nodeStatePresent: map[string]struct{}{},
		jvmMetricsSeen:   map[string]struct{}{},
	}
	if promquery.UnavailableReason() != "" {
		return signals
	}
	signalKey := func(sample *promquery.Sample) (string, bool) {
		clusterID, err := strconv.ParseUint(strings.TrimSpace(sample.Metric["cluster_id"]), 10, 32)
		if err != nil || clusterID == 0 {
			return "", false
		}
		instance := strings.TrimSpace(sample.Metric["instance"])
		if instance == "" {
			return "", false
		}
		return availabilitySignalKey(uint(clusterID), instance), true
	}
	collect := func(expr string, dst map[string]struct{}) {
		samples, err := promquery.Instant(ctx, expr, now)
		if err != nil {
			log.Printf("[Monitoring] availability instant query failed (%s): %v", expr, err)
			return
		}
		for i := range samples {
			sample := samples[i]
			if sample.Value != 0 {
				continue
			}
			if key, ok := signalKey(&sample); ok {
				dst[key] = struct{}{}
			}
		}
	}
	collect(`up{job="seatunnel_engine_http"}`, signals.endpointDown)
	collect(`node_state{job="seatunnel_engine_http"}`, signals.nodeStateDown)
	// 存在性采集：up==1、node_state 任意样本、jvm_threads_current 任意样本分别入位，
	// 供「引擎半初始化（up=1 + JVM 指标在 + node_state 缺失）」判定，缺失指标绝不误报。
	// Presence collection: up==1, any node_state sample and any jvm_threads_current sample are
	// recorded separately for the "half-initialized engine" rule (up=1 + JVM metrics + node_state
	// missing); absent metrics never raise a false positive.
	collectPresence := func(expr string, dst map[string]struct{}) {
		samples, err := promquery.Instant(ctx, expr, now)
		if err != nil {
			log.Printf("[Monitoring] availability instant query failed (%s): %v", expr, err)
			return
		}
		for i := range samples {
			if key, ok := signalKey(&samples[i]); ok {
				dst[key] = struct{}{}
			}
		}
	}
	collectPresence(`up{job="seatunnel_engine_http"} == 1`, signals.endpointUp)
	collectPresence(`node_state{job="seatunnel_engine_http"}`, signals.nodeStatePresent)
	collectPresence(`jvm_threads_current{job="seatunnel_engine_http"}`, signals.jvmMetricsSeen)
	return signals
}

// availabilityNodeOfflineReasons 汇总单个节点当前的全部不健康原因：进程侧原因（与旧
// evaluateNodeOfflineState 一致，去掉 host_offline）加上内嵌 TSDB 即时样本原因；
// 同一节点多个原因合并进同一条 node_offline。
// availabilityNodeOfflineReasons collects every unhealthy reason of one node right now:
// process-side reasons (same as the retired evaluateNodeOfflineState, minus host_offline)
// plus embedded-TSDB instant-sample reasons; multiple reasons merge into one node_offline.
func availabilityNodeOfflineReasons(node *cluster.NodeInfo, signals *availabilityMetricSignals) []string {
	if node == nil {
		return nil
	}
	reasons := make([]string, 0, 4)
	if node.IsOnline {
		// Agent 心跳超时时节点状态是派生值（GetNodes 会把 status 标成 offline），进程侧原因不可信，
		// 由 agent_offline 承担；引擎指标证据仍然有效，Agent 离线 + up==0 会各出一条告警。
		// While the agent heartbeat is dead the node status is a derived value (GetNodes marks
		// it offline), so process-side reasons cannot be trusted and belong to agent_offline;
		// engine-metric evidence stays valid, and agent offline + up==0 each raise an alert.
		switch {
		case node.Status == cluster.NodeStatusOffline:
			reasons = append(reasons, availabilityReasonHostVisibilityLost)
		case node.Status == cluster.NodeStatusError:
			reasons = append(reasons, availabilityReasonNodeError)
		case node.Status == cluster.NodeStatusStopped:
			reasons = append(reasons, availabilityReasonProcessStopped)
		case node.ProcessPID <= 0:
			reasons = append(reasons, availabilityReasonProcessMissing)
		}
	}
	if signals == nil {
		return reasons
	}
	instance := availabilityNodeInstance(node)
	if instance == "" {
		return reasons
	}
	key := availabilitySignalKey(node.ClusterID, instance)
	if _, down := signals.endpointDown[key]; down {
		reasons = append(reasons, availabilityReasonMetricsEndpointDown)
	}
	if _, down := signals.nodeStateDown[key]; down {
		reasons = append(reasons, availabilityReasonNodeStateDown)
	}
	// 引擎半初始化/僵尸 JVM：抓取端点活着且确实拿到了 JVM 指标，但 node_state 样本完全缺失。
	// 只有 up==1 而没有 jvm_threads_current 时不触发，避免普通 up=1 用例误报。
	// Half-initialized / zombie JVM: the scrape endpoint is alive and real JVM metrics arrive, yet
	// node_state samples are entirely missing. Never fires on bare up==1 without jvm_threads_current.
	if _, up := signals.endpointUp[key]; up {
		if _, hasJVM := signals.jvmMetricsSeen[key]; hasJVM {
			if _, hasState := signals.nodeStatePresent[key]; !hasState {
				reasons = append(reasons, availabilityReasonEngineNotReady)
			}
		}
	}
	return reasons
}

// availabilityNodeInstance 计算节点的抓取 instance 标签（HostIP:HazelcastPort，与
// metrics_targets.go 的目标生成一致）。
// availabilityNodeInstance computes the scrape instance label of a node (HostIP:HazelcastPort,
// matching the target generation in metrics_targets.go).
func availabilityNodeInstance(node *cluster.NodeInfo) string {
	if node == nil {
		return ""
	}
	host := strings.TrimSpace(node.HostIP)
	if host == "" || node.HazelcastPort <= 0 {
		return ""
	}
	return net.JoinHostPort(host, strconv.Itoa(node.HazelcastPort))
}

// availabilityNodeHealthy 是集群级规则（master / worker 计数）判定“健康节点”的口径：
// Agent 在线且没有任何节点离线原因。
// availabilityNodeHealthy is the "healthy node" definition used by cluster-level rules
// (master / worker counting): agent online and no node-offline reasons.
func availabilityNodeHealthy(node *cluster.NodeInfo, reasons []string) bool {
	return node != nil && node.IsOnline && len(reasons) == 0
}

func availabilityMasterRole(node *cluster.NodeInfo) bool {
	if node == nil {
		return false
	}
	return node.Role == cluster.NodeRoleMaster || node.Role == cluster.NodeRoleMasterWorker
}

func availabilityWorkerRole(node *cluster.NodeInfo, separated bool) bool {
	if node == nil {
		return false
	}
	if separated {
		return node.Role == cluster.NodeRoleWorker
	}
	return node.Role == cluster.NodeRoleWorker || node.Role == cluster.NodeRoleMasterWorker
}

// EvaluateAvailabilityAlerts 扫描全部受管集群，按集群规则目录评估 node_offline /
// agent_offline / master_unavailable / worker_insufficient 四条可用性规则，
// 触发与恢复都落成 process_events（RecordEvent 钩子负责后续投递）。
// EvaluateAvailabilityAlerts scans every managed cluster and evaluates the four availability
// rules of the cluster rule catalog (node_offline / agent_offline / master_unavailable /
// worker_insufficient); firing and recovery are persisted as process events and the
// RecordEvent hook takes care of delivery.
func (s *Service) EvaluateAvailabilityAlerts(ctx context.Context) error {
	if s == nil || s.clusterService == nil || s.monitorService == nil {
		return nil
	}
	clusters, _, err := s.clusterService.List(ctx, &cluster.ClusterFilter{Page: 1, PageSize: 1000})
	if err != nil {
		return err
	}

	now := timeNowUTC()
	signals := s.collectAvailabilityMetricSignals(ctx, now)
	seen := map[availabilityPendingKey]struct{}{}
	evaluated := map[uint]struct{}{}
	var evalErrs []error
	for _, item := range clusters {
		if item == nil {
			continue
		}
		cfg, err := s.monitorService.GetOrCreateConfig(ctx, item.ID)
		if err != nil {
			evalErrs = append(evalErrs, err)
			continue
		}
		if cfg == nil || !cfg.AutoMonitor {
			continue
		}
		nodes, err := s.clusterService.GetNodes(ctx, item.ID)
		if err != nil {
			evalErrs = append(evalErrs, err)
			continue
		}
		evaluated[item.ID] = struct{}{}
		if err := s.evaluateClusterAvailability(ctx, item, nodes, signals, seen, now); err != nil {
			evalErrs = append(evalErrs, err)
		}
	}
	if evaluator := s.availabilityEvaluator; evaluator != nil {
		evaluator.prune(evaluated, seen)
	}
	return errors.Join(evalErrs...)
}

// evaluateClusterAvailability 评估单个集群的四条可用性规则。
// evaluateClusterAvailability evaluates the four availability rules of one cluster.
func (s *Service) evaluateClusterAvailability(
	ctx context.Context,
	item *cluster.Cluster,
	nodes []*cluster.NodeInfo,
	signals *availabilityMetricSignals,
	seen map[availabilityPendingKey]struct{},
	now time.Time,
) error {
	if item == nil {
		return nil
	}
	evalNodes := make([]*cluster.NodeInfo, 0, len(nodes))
	for _, node := range nodes {
		if node == nil || shouldSkipNodeOfflineEvaluation(node) {
			continue
		}
		evalNodes = append(evalNodes, node)
	}

	reasonsByNode := make(map[uint][]string, len(evalNodes))
	for _, node := range evalNodes {
		reasonsByNode[node.ID] = availabilityNodeOfflineReasons(node, signals)
	}

	var evalErrs []error
	rules := map[string]*resolvedRuleConfig{}
	for _, key := range []string{AlertRuleKeyNodeOffline, clusterRuleKeyAgentOffline, clusterRuleKeyMasterUnavailable, clusterRuleKeyWorkerInsufficient} {
		rule, err := s.ResolveClusterRule(ctx, key, item.ID)
		if err != nil {
			evalErrs = append(evalErrs, err)
			continue
		}
		rules[key] = rule
	}

	for _, node := range evalNodes {
		reasons := reasonsByNode[node.ID]
		subject := availabilityPendingKey{RuleKey: AlertRuleKeyNodeOffline, ClusterID: item.ID, NodeID: node.ID, HostID: node.HostID}
		seen[subject] = struct{}{}
		event := &monitor.ProcessEvent{
			ClusterID:   item.ID,
			NodeID:      node.ID,
			HostID:      node.HostID,
			EventType:   monitor.EventTypeNodeOffline,
			PID:         node.ProcessPID,
			ProcessName: monitoringProcessNameForRole(string(node.Role)),
			InstallDir:  node.InstallDir,
			Role:        string(node.Role),
		}
		details := map[string]string{
			"reason":          strings.Join(reasons, ","),
			"reasons":         strings.Join(reasons, ","),
			"host_name":       strings.TrimSpace(node.HostName),
			"host_ip":         strings.TrimSpace(node.HostIP),
			"node_status":     string(node.Status),
			"is_online":       strconv.FormatBool(node.IsOnline),
			"process_pid":     strconv.Itoa(node.ProcessPID),
			"evaluation_mode": availabilityEvaluationMode,
		}
		if err := s.observeAvailabilitySubject(ctx, subject, len(reasons) > 0, rules[AlertRuleKeyNodeOffline], event, details, now); err != nil {
			evalErrs = append(evalErrs, err)
		}
	}

	// K8s 集群没有 Agent 心跳语义，agent_offline 规则不适用（Pod 健康由集群健康检查与引擎指标承担）。
	// K8s clusters have no agent heartbeat semantics, so agent_offline does not apply
	// (pod health is covered by cluster health checks and engine metrics).
	if !item.IsKubernetes() {
		s.evaluateAgentOfflineSubjects(ctx, item, evalNodes, rules[clusterRuleKeyAgentOffline], seen, now, &evalErrs)
	}
	s.evaluateMasterUnavailableSubject(ctx, item, evalNodes, reasonsByNode, rules[clusterRuleKeyMasterUnavailable], seen, now, &evalErrs)
	s.evaluateWorkerInsufficientSubject(ctx, item, evalNodes, reasonsByNode, rules[clusterRuleKeyWorkerInsufficient], seen, now, &evalErrs)
	return errors.Join(evalErrs...)
}

// evaluateAgentOfflineSubjects 按主机聚合 Agent 心跳：一台主机上任意受管节点离线即算该主机
// Agent 离线，一台主机只产生一条告警。
// evaluateAgentOfflineSubjects aggregates agent heartbeat per host: any managed node with a
// dead agent marks the host offline, and one host produces at most one alert.
func (s *Service) evaluateAgentOfflineSubjects(
	ctx context.Context,
	item *cluster.Cluster,
	evalNodes []*cluster.NodeInfo,
	rule *resolvedRuleConfig,
	seen map[availabilityPendingKey]struct{},
	now time.Time,
	evalErrs *[]error,
) {
	byHost := make(map[uint][]*cluster.NodeInfo)
	for _, node := range evalNodes {
		if node.HostID == 0 {
			continue
		}
		byHost[node.HostID] = append(byHost[node.HostID], node)
	}
	for hostID, hostNodes := range byHost {
		subject := availabilityPendingKey{RuleKey: clusterRuleKeyAgentOffline, ClusterID: item.ID, HostID: hostID}
		seen[subject] = struct{}{}
		rep := hostNodes[0]
		offline := false
		names := make([]string, 0, len(hostNodes))
		for _, node := range hostNodes {
			if !node.IsOnline {
				offline = true
			}
			if name := strings.TrimSpace(node.HostName); name != "" {
				names = append(names, name)
			}
		}
		event := &monitor.ProcessEvent{
			ClusterID: item.ID,
			HostID:    hostID,
			EventType: monitor.EventTypeAgentOffline,
		}
		details := map[string]string{
			"reason":          availabilityReasonHostOffline,
			"host_name":       strings.TrimSpace(rep.HostName),
			"host_ip":         strings.TrimSpace(rep.HostIP),
			"nodes":           strings.Join(names, ","),
			"evaluation_mode": availabilityEvaluationMode,
		}
		if err := s.observeAvailabilitySubject(ctx, subject, offline, rule, event, details, now); err != nil {
			*evalErrs = append(*evalErrs, err)
		}
	}
}

// evaluateMasterUnavailableSubject 评估 master_unavailable：可评估的 master 角色节点存在且
// 健康数为 0 时触发。
// evaluateMasterUnavailableSubject evaluates master_unavailable: fires when evaluable
// master-role nodes exist but none of them is healthy.
func (s *Service) evaluateMasterUnavailableSubject(
	ctx context.Context,
	item *cluster.Cluster,
	evalNodes []*cluster.NodeInfo,
	reasonsByNode map[uint][]string,
	rule *resolvedRuleConfig,
	seen map[availabilityPendingKey]struct{},
	now time.Time,
	evalErrs *[]error,
) {
	deployed, healthy := 0, 0
	unhealthyNames := make([]string, 0, 4)
	for _, node := range evalNodes {
		if !availabilityMasterRole(node) {
			continue
		}
		deployed++
		if availabilityNodeHealthy(node, reasonsByNode[node.ID]) {
			healthy++
			continue
		}
		if name := strings.TrimSpace(node.HostName); name != "" {
			unhealthyNames = append(unhealthyNames, name)
		}
	}
	if deployed == 0 {
		// 集群尚无可评估的 master 节点（安装 / 部署中）时不评估，避免误报。
		// Clusters with no evaluable master node (installing / deploying) are skipped to
		// avoid false positives.
		return
	}
	subject := availabilityPendingKey{RuleKey: clusterRuleKeyMasterUnavailable, ClusterID: item.ID}
	seen[subject] = struct{}{}
	sort.Strings(unhealthyNames)
	event := &monitor.ProcessEvent{ClusterID: item.ID, EventType: monitor.EventTypeMasterUnavailable}
	details := map[string]string{
		"reason":           availabilityReasonNoHealthyMaster,
		"deployed_masters": strconv.Itoa(deployed),
		"healthy_masters":  strconv.Itoa(healthy),
		"unhealthy_nodes":  strings.Join(unhealthyNames, ","),
		"evaluation_mode":  availabilityEvaluationMode,
	}
	if err := s.observeAvailabilitySubject(ctx, subject, healthy == 0, rule, event, details, now); err != nil {
		*evalErrs = append(*evalErrs, err)
	}
}

// evaluateWorkerInsufficientSubject 评估 worker_insufficient：健康 worker 数低于阈值；
// 阈值 nil 时取当前部署的 worker 数，部署数为 0 跳过。
// evaluateWorkerInsufficientSubject evaluates worker_insufficient: fires when the healthy
// worker count stays below the threshold; a nil threshold means the deployed worker count,
// and clusters with zero deployed workers are skipped.
func (s *Service) evaluateWorkerInsufficientSubject(
	ctx context.Context,
	item *cluster.Cluster,
	evalNodes []*cluster.NodeInfo,
	reasonsByNode map[uint][]string,
	rule *resolvedRuleConfig,
	seen map[availabilityPendingKey]struct{},
	now time.Time,
	evalErrs *[]error,
) {
	separated := item.DeploymentMode == cluster.DeploymentModeSeparated
	deployed, healthy := 0, 0
	for _, node := range evalNodes {
		if !availabilityWorkerRole(node, separated) {
			continue
		}
		deployed++
		if availabilityNodeHealthy(node, reasonsByNode[node.ID]) {
			healthy++
		}
	}
	if deployed == 0 {
		return
	}
	threshold := float64(deployed)
	if rule != nil && rule.Threshold != nil {
		threshold = *rule.Threshold
	}
	subject := availabilityPendingKey{RuleKey: clusterRuleKeyWorkerInsufficient, ClusterID: item.ID}
	seen[subject] = struct{}{}
	event := &monitor.ProcessEvent{ClusterID: item.ID, EventType: monitor.EventTypeWorkerInsufficient}
	details := map[string]string{
		"reason":           availabilityReasonWorkerBelowBaseline,
		"deployed_workers": strconv.Itoa(deployed),
		"healthy_workers":  strconv.Itoa(healthy),
		"threshold":        strconv.FormatFloat(threshold, 'f', -1, 64),
		"evaluation_mode":  availabilityEvaluationMode,
	}
	if err := s.observeAvailabilitySubject(ctx, subject, float64(healthy) < threshold, rule, event, details, now); err != nil {
		*evalErrs = append(*evalErrs, err)
	}
}

// observeAvailabilitySubject 是每条可用性规则的核心状态机：健康 → 清 pending，阶段仍开 → 写
// 恢复事件；不健康 → 阶段已开则提醒投递，否则 pending 满 for 后写触发事件。
// observeAvailabilitySubject is the per-rule state machine: healthy clears pending and closes
// an open episode with a recovery event; unhealthy either re-dispatches the stored firing
// event of an open episode or fires once the pending timer covers the rule's for window.
func (s *Service) observeAvailabilitySubject(
	ctx context.Context,
	subject availabilityPendingKey,
	unhealthy bool,
	rule *resolvedRuleConfig,
	firingEvent *monitor.ProcessEvent,
	details map[string]string,
	now time.Time,
) error {
	evaluator := s.ensureAvailabilityEvaluator()
	if rule == nil || !rule.Enabled {
		evaluator.clear(subject)
		return nil
	}
	types, ok := availabilityRuleEventTypes[subject.RuleKey]
	if !ok || firingEvent == nil || s.monitorService == nil {
		return nil
	}

	latest, err := s.monitorService.GetLatestSubjectEventByTypes(ctx, subject.ClusterID, subject.NodeID, subject.HostID,
		[]monitor.ProcessEventType{types[0], types[1]})
	if err != nil {
		return err
	}
	active := latest != nil && latest.EventType == types[0]

	if !unhealthy {
		evaluator.clear(subject)
		if !active {
			return nil
		}
		return s.recordAvailabilityRecovered(ctx, subject, types[1], latest, now)
	}
	if active {
		// 阶段已触发：沿用既有 firing 事件做提醒投递，下游按冷却 / 去重键控制频率。
		// Episode already firing: re-dispatch the stored firing event; cooldown and the
		// dedupe key gate the actual send frequency downstream.
		return s.DispatchAlertPolicyEvent(ctx, latest)
	}

	since, matured := evaluator.mark(subject, rule.ForSeconds, now)
	if !matured {
		return nil
	}
	if subject.RuleKey == clusterRuleKeyAgentOffline && s.shouldSuppressAvailabilityDuringStartup(now) {
		return nil
	}
	if details == nil {
		details = map[string]string{}
	}
	details["observed_since"] = since.UTC().Format(time.RFC3339)
	details["grace_seconds"] = strconv.Itoa(rule.ForSeconds)
	raw, err := json.Marshal(details)
	if err != nil {
		return err
	}
	firingEvent.Details = string(raw)
	return s.monitorService.RecordEvent(ctx, firingEvent)
}

// recordAvailabilityRecovered 写入主体的恢复事件（带触发事件 ID，供恢复投递与生命周期解析回溯）。
// recordAvailabilityRecovered writes the subject's recovery event carrying the firing event id
// so recovery delivery and lifecycle resolution can trace back.
func (s *Service) recordAvailabilityRecovered(
	ctx context.Context,
	subject availabilityPendingKey,
	recoveredType monitor.ProcessEventType,
	firingEvent *monitor.ProcessEvent,
	now time.Time,
) error {
	detailMap := map[string]string{
		"recovered_at":    now.UTC().Format(time.RFC3339),
		"evaluation_mode": availabilityEvaluationMode,
	}
	if firingEvent != nil && firingEvent.ID > 0 {
		detailMap[availabilityDetailKeyFiringEventID] = strconv.FormatUint(uint64(firingEvent.ID), 10)
		if recoveredType == monitor.EventTypeNodeRecovered {
			// node_recovered 的旧读取方认 offline_event_id；保持写入以兼容恢复投递的解析路径。
			// Legacy readers of node_recovered look for offline_event_id; keep writing it for
			// the recovery-dispatch resolution path.
			detailMap[availabilityDetailKeyLegacyOfflineEventID] = strconv.FormatUint(uint64(firingEvent.ID), 10)
		}
	}
	raw, err := json.Marshal(detailMap)
	if err != nil {
		return err
	}
	event := &monitor.ProcessEvent{
		ClusterID: subject.ClusterID,
		NodeID:    subject.NodeID,
		HostID:    subject.HostID,
		EventType: recoveredType,
		Details:   string(raw),
	}
	if firingEvent != nil {
		event.PID = firingEvent.PID
		event.ProcessName = firingEvent.ProcessName
		event.InstallDir = firingEvent.InstallDir
		event.Role = firingEvent.Role
	}
	return s.monitorService.RecordEvent(ctx, event)
}

// shouldSuppressAvailabilityDuringStartup 在 STX 冷启动后的 Agent 重连窗口内抑制告警：
// 这段时间所有主机心跳都还没回来，agent_offline 会全量误报。
// shouldSuppressAvailabilityDuringStartup suppresses alerts inside the agent reconnect window
// after an STX cold start: heartbeats have not come back yet and agent_offline would fire
// everywhere.
func (s *Service) shouldSuppressAvailabilityDuringStartup(now time.Time) bool {
	if s == nil || s.nodeHealthEvaluatorStartedAt.IsZero() || s.nodeHealthStartupSuppression <= 0 {
		return false
	}
	return now.Before(s.nodeHealthEvaluatorStartedAt.UTC().Add(s.nodeHealthStartupSuppression))
}
