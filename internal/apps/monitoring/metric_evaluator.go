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
	_ "embed"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/rulefmt"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/rules"

	clusterapp "github.com/LeonYoah/stx/internal/apps/cluster"
	"github.com/LeonYoah/stx/internal/config"
	"github.com/LeonYoah/stx/internal/logger"
	"github.com/LeonYoah/stx/internal/pkg/embedts"
	"github.com/LeonYoah/stx/internal/version"
)

const (
	// builtinMetricRulesSource / managedMetricRulesSource 是传给 rules.Manager.Update 的「文件」标识：
	// 进程内方案不落规则文件，这两个字符串只充当规则组命名空间（GroupKey = file;name）。
	// builtinMetricRulesSource / managedMetricRulesSource are the "file" identifiers handed to
	// rules.Manager.Update: the in-process design never writes rule files, so the strings only
	// namespace the rule groups (GroupKey = file;name).
	builtinMetricRulesSource = "stx-builtin-rules"
	managedMetricRulesSource = "stx-managed-policies"

	// managedMetricRuleGroupName 保持与原 Prometheus rule 文件中的 group 名一致，保证告警指纹、
	// 告警序列标签与对外展示语义不漂移。
	// managedMetricRuleGroupName keeps the group name used by the old Prometheus rule file so
	// alert fingerprints, alert-sample labels and outward-facing semantics do not drift.
	managedMetricRuleGroupName = "stx-managed-alert-policies"

	metricEvaluatorDefaultInterval = 15 * time.Second
	// metricEvaluatorDefaultResendDelay 沿用原 Alertmanager repeat_interval（10 分钟）：
	// 触发中的告警按同样节奏重推，去重与冷却仍由既有投递缝裁决。
	// metricEvaluatorDefaultResendDelay reuses the old Alertmanager repeat_interval (10 minutes):
	// firing alerts are re-pushed on the same cadence while dedupe and cooldown stay with the
	// existing delivery seam.
	metricEvaluatorDefaultResendDelay = 10 * time.Minute
	// ForGracePeriod / OutageTolerance 取 Prometheus 默认值：恢复重启后对「刚好断点」的 pending
	// 状态容忍与远程宕机语义与外部进程版一致。
	// ForGracePeriod / OutageTolerance take the Prometheus defaults so restored pending state
	// and outage semantics after a restart match the external-process build.
	metricEvaluatorForGracePeriod  = 10 * time.Minute
	metricEvaluatorOutageTolerance = 30 * time.Minute

	// metricEvaluatorDispatchCapacity 是 NotifyFunc → 投递缝的缓冲队列容量；打满时丢弃并限速记 WARN，
	// firing 告警会按 resend 节奏自愈，resolved 事件在 15m resolvedRetention 内也会被重发。
	// metricEvaluatorDispatchCapacity is the buffered NotifyFunc → delivery-seam queue size; on
	// overflow records are dropped with a rate-limited WARN. Firing alerts self-heal via resends
	// and resolved events are re-emitted inside the 15m resolvedRetention window.
	metricEvaluatorDispatchCapacity = 1024
	metricEvaluatorDropWarnInterval = time.Minute
)

// metricRuleExprParser 是规则表达式的共享解析器（线程安全）。
// metricRuleExprParser is the shared rule expression parser (thread-safe).
var metricRuleExprParser = parser.NewParser(parser.Options{})

// builtinMetricRuleGroupsOnce 惰性解析一次内嵌默认规则文件；解析失败属于构建缺陷，直接让评估器
// 启动失败而不是静默跑一个无规则的进程。
// builtinMetricRuleGroupsOnce lazily parses the embedded default rule file once; a parse failure
// is a build defect, so the evaluator fails to start instead of silently running without rules.
var builtinMetricRuleGroupsOnce = sync.OnceValues(func() (*rulefmt.RuleGroups, error) {
	groups, errs := rulefmt.Parse(builtinSeaTunnelRulesYAML, false, model.UTF8Validation, metricRuleExprParser, promslog.NewNopLogger())
	if len(errs) > 0 {
		return nil, fmt.Errorf("builtin seatunnel rules are invalid: %w", errors.Join(errs...))
	}
	return groups, nil
})

//go:embed rules/seatunnel-default-rules.yml
var builtinSeaTunnelRulesYAML []byte

// MetricEvaluatorOptions 内嵌告警评估器可配项；零值字段取默认值。
// MetricEvaluatorOptions holds the embedded alert evaluator tunables; zero fields take defaults.
type MetricEvaluatorOptions struct {
	// Interval 是规则组评估间隔（observability.eval_interval）。
	// Interval is the rule group evaluation interval (observability.eval_interval).
	Interval time.Duration

	// ResendDelay 是 firing 告警重推间隔（沿用原 AM repeat_interval 语义）。
	// ResendDelay is the firing alert re-push interval (mirrors the old AM repeat_interval).
	ResendDelay time.Duration

	// DispatchCapacity 是通知投递缓冲队列长度。
	// DispatchCapacity is the notification dispatch queue capacity.
	DispatchCapacity int
}

func (o MetricEvaluatorOptions) withDefaults() MetricEvaluatorOptions {
	if o.Interval <= 0 {
		o.Interval = metricEvaluatorDefaultInterval
	}
	if o.ResendDelay <= 0 {
		o.ResendDelay = metricEvaluatorDefaultResendDelay
	}
	if o.DispatchCapacity <= 0 {
		o.DispatchCapacity = metricEvaluatorDispatchCapacity
	}
	return o
}

// MetricEvaluator 是内嵌告警评估器：包装 prometheus/rules.Manager，用 embedts 的引擎/存储评估
// 内嵌默认规则 + DB 指标策略规则，把 pending/firing/resolved 状态机交给上游实现，NotifyFunc 只做
// 「rules.Alert → RemoteAlertRecord → 既有投递缝」的转换，不复制去重/冷却/静默/恢复语义。
// MetricEvaluator is the embedded alert evaluator: it wraps prometheus/rules.Manager, evaluates
// builtin default rules plus DB metric policy rules against the embedts engine/storage, leaves
// the pending/firing/resolved state machine to upstream, and lets NotifyFunc only translate
// rules.Alert → RemoteAlertRecord → existing delivery seam, without duplicating
// dedupe/cooldown/silence/recovery semantics.
type MetricEvaluator struct {
	service *Service
	opts    MetricEvaluatorOptions

	// sources 是 Update 传入的规则组命名空间列表；测试可注入自定义 source 以直接喂入
	// rulefmt.RuleGroups（生产路径固定为 builtin + managed）。
	// sources is the rule-source namespace list handed to Update; tests may inject a custom
	// source to feed rulefmt.RuleGroups directly (production is always builtin + managed).
	sources []string

	loader  *metricRuleGroupLoader
	manager *rules.Manager

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	dispatchCh chan *RemoteAlertRecord
	reloadCh   chan struct{}

	droppedDispatches atomic.Int64
	lastDropWarnAt    atomic.Int64 // unixnano，限速记丢弃日志 / unixnano, rate-limits drop WARN logs
}

// NewMetricEvaluator 构造评估器：预检依赖并解析内嵌规则；Start 才真正启动 manager 与循环。
// NewMetricEvaluator builds the evaluator: it validates dependencies and parses the builtin
// rules eagerly; Start is what actually boots the manager and loops.
func NewMetricEvaluator(service *Service, opts MetricEvaluatorOptions) (*MetricEvaluator, error) {
	if service == nil || service.repo == nil {
		return nil, errors.New("monitoring repository is not configured")
	}
	builtinGroups, err := builtinMetricRuleGroupsOnce()
	if err != nil {
		return nil, err
	}
	e := &MetricEvaluator{
		service:    service,
		opts:       opts.withDefaults(),
		sources:    []string{builtinMetricRulesSource, managedMetricRulesSource},
		loader:     newMetricRuleGroupLoader(),
		dispatchCh: nil, // Start 中初始化 / initialised in Start
		reloadCh:   make(chan struct{}, 1),
	}
	e.loader.setGroups(builtinMetricRulesSource, builtinGroups)
	return e, nil
}

// Start 启动评估器：从 embedts 取引擎/查询/追加三元组，装配 rules.Manager 并拉起
// manager.Run、规则组重建循环与投递循环。embedts 未就绪返回错误。
// Start boots the evaluator: it takes the engine/queryable/appendable triple from embedts,
// wires the rules.Manager and spawns manager.Run, the rule-group rebuild loop and the dispatch
// loop. An unready embedts returns an error.
func (e *MetricEvaluator) Start(ctx context.Context) error {
	engine, queryable := embedts.QueryEngine()
	appendable := embedts.AppendableV2()
	if engine == nil || queryable == nil || appendable == nil {
		return errors.New("embedded tsdb is not ready")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	e.ctx, e.cancel = context.WithCancel(ctx)
	e.dispatchCh = make(chan *RemoteAlertRecord, e.opts.DispatchCapacity)

	e.manager = rules.NewManager(&rules.ManagerOptions{
		Context:         e.ctx,
		QueryFunc:       rules.EngineQueryFunc(engine, queryable),
		Queryable:       queryable,
		AppendableV2:    appendable,
		NotifyFunc:      e.onRuleAlerts,
		Logger:          slog.New(&metricEvaluatorSlogHandler{ctx: e.ctx}),
		Registerer:      prometheus.NewRegistry(),
		ExternalURL:     &url.URL{},
		ResendDelay:     e.opts.ResendDelay,
		ForGracePeriod:  metricEvaluatorForGracePeriod,
		OutageTolerance: metricEvaluatorOutageTolerance,
		GroupLoader:     e.loader,
	})

	e.wg.Add(4)
	go func() {
		defer e.wg.Done()
		// manager.Run 阻塞到 Stop；ctx 取消时由监视协程触发 manager.Stop 回收组协程。
		// manager.Run blocks until Stop; the watcher goroutine triggers manager.Stop on ctx
		// cancellation so group goroutines are reaped.
		e.manager.Run()
	}()
	go func() {
		defer e.wg.Done()
		<-e.ctx.Done()
		e.manager.Stop()
	}()
	go func() {
		defer e.wg.Done()
		e.reloadLoop()
	}()
	go func() {
		defer e.wg.Done()
		e.dispatchLoop()
	}()
	return nil
}

// Stop 停止评估器并等待全部协程退出；未启动时为空操作。
// Stop halts the evaluator and waits for all goroutines; a no-op when not started.
func (e *MetricEvaluator) Stop() {
	if e == nil || e.cancel == nil {
		return
	}
	e.cancel()
	e.wg.Wait()
}

// RequestReload 请求立即重建规则组（策略 CRUD/启停后调用）；非阻塞，已在排队时合并。
// RequestReload asks for an immediate rule-group rebuild (called after policy CRUD/enable
// changes); it never blocks and coalesces with an already-queued request.
func (e *MetricEvaluator) RequestReload() {
	if e == nil {
		return
	}
	select {
	case e.reloadCh <- struct{}{}:
	default:
	}
}

// reloadLoop 按评估间隔兜底重建规则组，并消费即时触发：Update 在组内容未变时是 no-op
// （oldg.Equals(newg)），周期重建成本仅一次 DB 列举 + 表达式解析。
// reloadLoop rebuilds the rule groups on the eval interval as a safety net and also serves
// immediate triggers: Update is a no-op for unchanged groups (oldg.Equals(newg)), so a periodic
// rebuild only costs one DB listing plus expression parsing.
func (e *MetricEvaluator) reloadLoop() {
	e.updateRuleGroups(e.ctx)
	ticker := time.NewTicker(e.opts.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-e.ctx.Done():
			return
		case <-ticker.C:
			e.updateRuleGroups(e.ctx)
		case <-e.reloadCh:
			e.updateRuleGroups(e.ctx)
		}
	}
}

// updateRuleGroups 重建 managed 规则组并推给 manager；构建失败时保留上一轮规则集（Update 本身
// 在加载失败时也会回滚）。
// updateRuleGroups rebuilds the managed rule group and pushes it to the manager; when the build
// fails the previous rule set stays in place (Update itself also rolls back on load failure).
func (e *MetricEvaluator) updateRuleGroups(ctx context.Context) {
	if slices.Contains(e.sources, managedMetricRulesSource) {
		groups, err := e.buildManagedRuleGroups(ctx)
		if err != nil {
			logger.WarnF(ctx, "managed metric rule group rebuild failed, keeping previous set: %v", err)
			return
		}
		e.loader.setGroups(managedMetricRulesSource, groups)
	}
	if err := e.manager.Update(e.opts.Interval, e.sources, labels.EmptyLabels(), "", nil); err != nil {
		logger.WarnF(ctx, "rule group update failed: %v", err)
	}
}

// buildManagedRuleGroups 从规则目录生成托管 tsdb 规则组：每条启用规则一条全局 rule
// （`cluster_id!~"<例外集群>"` 排除所有例外集群）+ 每个存在例外且启用规则的集群一条
// rule（`cluster_id="<id>"`）；labels 带 managed_by=stx 与 rule_key。
// buildManagedRuleGroups generates the managed tsdb rule group from the rule catalog: one
// global rule per enabled rule (`cluster_id!~"<exception clusters>"` excludes every overridden
// cluster) plus one rule per cluster that has an override and still enables the rule
// (`cluster_id="<id>"`); labels carry managed_by=stx and rule_key.
func (e *MetricEvaluator) buildManagedRuleGroups(ctx context.Context) (*rulefmt.RuleGroups, error) {
	layers, err := e.service.loadClusterRuleLayers(ctx)
	if err != nil {
		return nil, err
	}
	clusterNames, err := e.service.listManagedAlertPolicyClusterNames(ctx)
	if err != nil {
		return nil, err
	}
	ruleList := make([]rulefmt.Rule, 0, len(clusterRuleCatalog))
	for _, def := range clusterRuleCatalog {
		if def == nil || def.Engine != clusterRuleEngineTSDB || def.Expr == nil {
			continue
		}
		exceptionIDs := layers.overrideClusterIDs(def.Key)
		globalMatcher := ""
		if len(exceptionIDs) > 0 {
			globalMatcher = clusterRuleExceptionMatcher(exceptionIDs)
		}
		global := resolveClusterRuleFromLayers(def, layers, 0)
		if global != nil && global.Enabled {
			ruleList = append(ruleList, *buildManagedClusterRule(global, globalMatcher, clusterNames["all"]))
		}
		for _, clusterID := range exceptionIDs {
			resolved := resolveClusterRuleFromLayers(def, layers, clusterID)
			if resolved == nil || !resolved.Enabled {
				continue
			}
			matcher := fmt.Sprintf(`cluster_id="%d"`, clusterID)
			ruleList = append(ruleList, *buildManagedClusterRule(resolved, matcher, clusterNames[strconv.FormatUint(uint64(clusterID), 10)]))
		}
	}
	return &rulefmt.RuleGroups{
		Groups: []rulefmt.RuleGroup{
			{Name: managedMetricRuleGroupName, Rules: ruleList},
		},
	}, nil
}

// clusterRuleExceptionMatcher 生成「排除全部例外集群」的负向正则匹配器；Prometheus 的
// 正则匹配整体锚定，id 列表用 | 连接不会误匹配更长的 id。
// clusterRuleExceptionMatcher builds the negative-regex matcher excluding every exception
// cluster; Prometheus anchors regex matches, so joining ids with | never mis-matches longer
// ids.
func clusterRuleExceptionMatcher(exceptionIDs []uint) string {
	parts := make([]string, 0, len(exceptionIDs))
	for _, id := range exceptionIDs {
		parts = append(parts, strconv.FormatUint(uint64(id), 10))
	}
	return fmt.Sprintf(`cluster_id!~"%s"`, strings.Join(parts, "|"))
}

// buildManagedClusterRule 把一条解析后的 tsdb 规则适配为 rulefmt.Rule：alert 名取稳定
// rule_key，labels 带 managed_by=stx / rule_key / severity，for 取生效持续时间。
// buildManagedClusterRule adapts one resolved tsdb rule into a rulefmt.Rule: the alert name is
// the stable rule_key, labels carry managed_by=stx / rule_key / severity, and 'for' is the
// resolved duration.
func buildManagedClusterRule(cfg *resolvedRuleConfig, clusterMatcher string, clusterScope string) *rulefmt.Rule {
	def := cfg.Definition
	annotations := map[string]string{
		"rule_key":      def.Key,
		"rule_name":     escapePrometheusTemplateText(def.Name),
		"cluster_scope": escapePrometheusTemplateText(firstNonEmpty(clusterScope, "全部集群")),
		"condition":     buildClusterRuleConditionText(cfg),
		"summary":       fmt.Sprintf(`{{ $labels.cluster_name }} / {{ $labels.instance }}：%s`, escapePrometheusTemplateText(def.Name)),
		"description":   fmt.Sprintf(`%s；当前值：{{ printf "%%.6f" $value }}。`, escapePrometheusTemplateText(def.Description)),
		"current_value": `{{ printf "%.6f" $value }}`,
	}
	return &rulefmt.Rule{
		Alert: def.Key,
		Expr:  def.Expr(cfg, clusterMatcher),
		For:   model.Duration(time.Duration(cfg.ForSeconds) * time.Second),
		Labels: map[string]string{
			"managed_by": "stx",
			"rule_key":   def.Key,
			"severity":   string(resolveRuleSeverity(cfg)),
		},
		Annotations: annotations,
	}
}

// buildClusterRuleConditionText 生成通知里的判定条件摘要（持续时长 + 阈值条件）。
// buildClusterRuleConditionText renders the alert condition summary used in notifications
// (duration + threshold clause).
func buildClusterRuleConditionText(cfg *resolvedRuleConfig) string {
	text := fmt.Sprintf("持续 %s", formatClusterRuleForDuration(cfg.ForSeconds))
	if cfg.Definition != nil && cfg.Definition.Threshold != nil {
		text += fmt.Sprintf("，条件：%s %s", cfg.Definition.Threshold.Operator, formatClusterRuleThreshold(cfg.thresholdValue()))
	}
	return text
}

func formatClusterRuleForDuration(forSeconds int) string {
	if forSeconds <= 0 {
		return "0m"
	}
	if forSeconds%60 == 0 {
		return fmt.Sprintf("%dm", forSeconds/60)
	}
	return fmt.Sprintf("%ds", forSeconds)
}

// onRuleAlerts 是 rules.NotifyFunc：把上游状态机给出的 firing/resolved 告警翻译成
// RemoteAlertRecord 投到缓冲队列；绝不在这里做持久化或网络投递，避免慢投递阻塞规则评估。
// onRuleAlerts is the rules.NotifyFunc: it translates firing/resolved alerts emitted by the
// upstream state machine into RemoteAlertRecords enqueued on the buffered queue; it never does
// persistence or network delivery inline so slow channels cannot block rule evaluation.
func (e *MetricEvaluator) onRuleAlerts(_ context.Context, _ string, alerts ...*rules.Alert) {
	for _, alert := range alerts {
		record := remoteAlertRecordFromRuleAlert(alert, timeNowUTC())
		if record == nil {
			continue
		}
		select {
		case e.dispatchCh <- record:
		default:
			dropped := e.droppedDispatches.Add(1)
			now := time.Now().UnixNano()
			if last := e.lastDropWarnAt.Load(); now-last >= int64(metricEvaluatorDropWarnInterval) && e.lastDropWarnAt.CompareAndSwap(last, now) {
				logger.WarnF(e.ctx, "[Observability] 告警投递队列已满，累计丢弃 %d 条；firing 告警按重推节奏自愈 / alert dispatch queue is full, %d records dropped so far; firing alerts self-heal on resend", dropped, dropped)
			}
		}
	}
}

// dispatchLoop 单协程消费投递队列，保证同一告警的 firing→resolved 顺序不乱；
// 逐条走 ingestRemoteAlertRecord 既有投递缝。
// dispatchLoop drains the dispatch queue on one goroutine so the firing→resolved order of the
// same alert cannot interleave; each record goes through the existing ingestRemoteAlertRecord
// delivery seam.
func (e *MetricEvaluator) dispatchLoop() {
	for {
		select {
		case <-e.ctx.Done():
			return
		case record := <-e.dispatchCh:
			if record == nil {
				continue
			}
			if err := e.service.ingestRemoteAlertRecord(e.ctx, record); err != nil {
				logger.WarnF(e.ctx, "builtin alert delivery failed: %v", err)
			}
		}
	}
}

// remoteAlertRecordFromRuleAlert 把 rules.Alert 映射为 RemoteAlertRecord：fingerprint 用标签集
// hash（与 AM 同源算法），starts_at 用 FiredAt，resolved 事件的 ends_at/resolved_at 取
// ResolvedAt，firing 事件的 ends_at 取 ValidUntil。
// remoteAlertRecordFromRuleAlert maps a rules.Alert onto a RemoteAlertRecord: fingerprint is the
// label-set hash (the algorithm Alertmanager uses), starts_at is FiredAt, ends_at/resolved_at for
// resolved events come from ResolvedAt, and ends_at for firing events is ValidUntil.
func remoteAlertRecordFromRuleAlert(alert *rules.Alert, now time.Time) *RemoteAlertRecord {
	if alert == nil {
		return nil
	}
	resolved := !alert.ResolvedAt.IsZero() || alert.State == rules.StateInactive
	if alert.State != rules.StateFiring && !resolved {
		// pending 等中间态不该抵达投递缝；兜底丢弃。
		// Pending and other intermediate states must never reach the delivery seam; drop defensively.
		return nil
	}

	labelsMap := alert.Labels.Map()
	annotationsMap := alert.Annotations.Map()

	status := string(NotificationDeliveryEventTypeFiring)
	var resolvedAt *time.Time
	endsAt := int64(0)
	if resolved {
		status = string(NotificationDeliveryEventTypeResolved)
		t := alert.ResolvedAt
		if t.IsZero() {
			t = now
		}
		t = t.UTC()
		resolvedAt = &t
		endsAt = t.Unix()
	} else if !alert.ValidUntil.IsZero() {
		endsAt = alert.ValidUntil.Unix()
	}

	startsAt := alert.FiredAt
	if startsAt.IsZero() {
		startsAt = alert.ActiveAt
	}
	if startsAt.IsZero() {
		startsAt = now
	}
	startsAt = startsAt.UTC()

	clusterID := strings.TrimSpace(labelsMap["cluster_id"])
	if clusterID == "" {
		clusterID = "unknown"
	}
	clusterName := strings.TrimSpace(labelsMap["cluster_name"])
	if clusterName == "" {
		clusterName = "unknown"
	}
	env := strings.TrimSpace(labelsMap["env"])
	if env == "" {
		env = "unknown"
	}
	alertName := strings.TrimSpace(firstNonEmpty(labelsMap["policy_name"], annotationsMap["policy_name"], annotationsMap["rule_name"], labelsMap["alertname"]))
	if alertName == "" {
		alertName = "unknown_alert"
	}

	return &RemoteAlertRecord{
		Fingerprint:     fmt.Sprintf("%016x", alert.Labels.Hash()),
		StartsAt:        startsAt.Unix(),
		Status:          status,
		Receiver:        managedAlertmanagerDefaultReceiver,
		AlertName:       alertName,
		Severity:        strings.TrimSpace(labelsMap["severity"]),
		ClusterID:       clusterID,
		ClusterName:     clusterName,
		Env:             env,
		GeneratorURL:    "",
		Summary:         strings.TrimSpace(firstNonEmpty(annotationsMap["summary"], annotationsMap["message"])),
		Description:     strings.TrimSpace(firstNonEmpty(annotationsMap["description"], annotationsMap["details"])),
		LabelsJSON:      mustMarshalJSON(labelsMap),
		AnnotationsJSON: mustMarshalJSON(annotationsMap),
		EndsAt:          endsAt,
		ResolvedAt:      resolvedAt,
		LastReceivedAt:  now,
	}
}

// metricRuleGroupLoader 是进程内 GroupLoader：用标识符查内存中的规则组集，表达式解析委托给
// 共享 parser。Update 传入的「文件名」在此只是命名空间键。
// metricRuleGroupLoader is the in-process GroupLoader: identifiers look up rule group sets held
// in memory and expression parsing defers to the shared parser. The "file names" handed to
// Update are just namespace keys here.
type metricRuleGroupLoader struct {
	mu     sync.RWMutex
	groups map[string]*rulefmt.RuleGroups
}

func newMetricRuleGroupLoader() *metricRuleGroupLoader {
	return &metricRuleGroupLoader{groups: map[string]*rulefmt.RuleGroups{}}
}

func (l *metricRuleGroupLoader) setGroups(identifier string, groups *rulefmt.RuleGroups) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if groups == nil {
		groups = &rulefmt.RuleGroups{}
	}
	l.groups[identifier] = groups
}

// Load 返回标识符对应的规则组集；未知标识符按空组集处理，保证 Update 不因缺「文件」失败。
// Load returns the rule group set for an identifier; unknown identifiers resolve to an empty
// set so Update never fails on a missing "file".
func (l *metricRuleGroupLoader) Load(identifier string, _ bool, _ model.ValidationScheme) (*rulefmt.RuleGroups, []error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if groups := l.groups[identifier]; groups != nil {
		return groups, nil
	}
	return &rulefmt.RuleGroups{}, nil
}

func (l *metricRuleGroupLoader) Parse(query string) (parser.Expr, error) {
	return metricRuleExprParser.ParseExpr(query)
}

// metricEvaluatorSlogHandler 把 rules.Manager 的 slog 输出桥到项目 logger：warn/error 透传，
// info/debug 收敛为 debug。规则评估失败（PromQL 错误、写回失败）必须落到项目日志里可见，
// 用 NopLogger 会让「规则没评估」静默丢失。
// metricEvaluatorSlogHandler bridges rules.Manager's slog output to the project logger:
// warn/error pass through while info/debug collapse to debug. Rule evaluation failures (PromQL
// errors, write-back failures) must stay visible in project logs — a NopLogger would silently
// lose "rules did not evaluate".
type metricEvaluatorSlogHandler struct {
	ctx context.Context
}

func (h *metricEvaluatorSlogHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *metricEvaluatorSlogHandler) Handle(_ context.Context, r slog.Record) error {
	msg := r.Message
	r.Attrs(func(a slog.Attr) bool {
		msg += " " + a.Key + "=" + a.Value.String()
		return true
	})
	switch {
	case r.Level >= slog.LevelError:
		logger.ErrorF(h.ctx, "[Observability] rules manager: %s", msg)
	case r.Level >= slog.LevelWarn:
		logger.WarnF(h.ctx, "[Observability] rules manager: %s", msg)
	default:
		logger.DebugF(h.ctx, "[Observability] rules manager: %s", msg)
	}
	return nil
}

func (h *metricEvaluatorSlogHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *metricEvaluatorSlogHandler) WithGroup(_ string) slog.Handler      { return h }

// ── 以下 helper 自 managed_alert_policy_sync.go 迁入：策略 → PromQL 告警规则的适配逻辑。 ──
// ── The helpers below moved in from managed_alert_policy_sync.go: the policy → PromQL alert
//    rule adaptation logic. ──

// managedAlertmanagerDefaultReceiver 复用原 AM 默认接收器名，作为内置评估器生成记录的
// Receiver 取值，保持告警中心「接收器」列语义不变。
// managedAlertmanagerDefaultReceiver reuses the old AM default receiver name as the Receiver
// value on evaluator-generated records, keeping the alert center's receiver column unchanged.
const managedAlertmanagerDefaultReceiver = "stx-webhook"

// listManagedAlertPolicyClusterNames 列出集群 id→名映射，供规则注解展示集群范围。
// listManagedAlertPolicyClusterNames lists cluster id→name pairs for cluster-scope annotations.
func (s *Service) listManagedAlertPolicyClusterNames(ctx context.Context) (map[string]string, error) {
	result := map[string]string{"all": "全部集群"}
	if s == nil || s.clusterService == nil {
		return result, nil
	}

	clusters, _, err := s.clusterService.List(ctx, &clusterapp.ClusterFilter{Page: 1, PageSize: 1000})
	if err != nil {
		return nil, err
	}
	for _, item := range clusters {
		if item == nil || item.ID == 0 {
			continue
		}
		result[strconv.FormatUint(uint64(item.ID), 10)] = strings.TrimSpace(item.Name)
	}
	return result, nil
}

// buildManagedMetricPolicyRule 把一条启用中的指标模板策略适配为 rulefmt.Rule：
// 标签/注解/表达式与原写 Prometheus 规则文件的字段完全一致，保证指纹与通知文案不漂移；
// for 直接转成 model.Duration（原先经 "Nm" 字符串过一道 yaml）。
// buildManagedMetricPolicyRule adapts one enabled metrics-template policy into a rulefmt.Rule:
// labels/annotations/expression are byte-for-byte what the old Prometheus rule file carried so
// fingerprints and notification text do not drift; 'for' converts straight to model.Duration
// (previously round-tripped through an "Nm" yaml string).
func buildManagedMetricPolicyRule(policy *AlertPolicy, clusterNames map[string]string) (*rulefmt.Rule, bool, error) {
	if policy == nil || !policy.Enabled {
		return nil, false, nil
	}

	clusterScopeName := strings.TrimSpace(firstNonEmpty(clusterNames[strings.TrimSpace(policy.ClusterID)], policy.ClusterID, "全部集群"))
	ruleLabels := map[string]string{
		"managed_by":  "stx",
		"policy_id":   strconv.FormatUint(uint64(policy.ID), 10),
		"policy_name": strings.TrimSpace(firstNonEmpty(policy.Name, "未命名策略")),
		"policy_type": string(policy.PolicyType),
		"severity":    strings.TrimSpace(firstNonEmpty(string(normalizeAlertPolicySeverity(policy.Severity)), string(AlertSeverityWarning))),
	}
	annotations := map[string]string{
		"policy_id":     strconv.FormatUint(uint64(policy.ID), 10),
		"policy_name":   strings.TrimSpace(firstNonEmpty(policy.Name, "未命名策略")),
		"cluster_scope": clusterScopeName,
	}
	if strings.TrimSpace(policy.TemplateKey) != "" {
		ruleLabels["template_key"] = strings.TrimSpace(policy.TemplateKey)
		annotations["template_key"] = strings.TrimSpace(policy.TemplateKey)
		annotations["template_name"] = alertPolicyTemplateDisplayNameZH(strings.TrimSpace(policy.TemplateKey))
	}

	var expr string
	forMinutes := 1
	switch policy.PolicyType {
	case AlertPolicyBuilderKindMetricsTemplate:
		template, ok := findAlertPolicyTemplateSummary(strings.TrimSpace(policy.TemplateKey))
		if !ok {
			return nil, false, nil
		}
		condition := resolveMetricPolicyCondition(policy, template)
		annotations["condition"] = buildMetricConditionDescriptionZH(condition)
		annotations["summary"] = buildManagedMetricAlertSummaryAnnotation(policy, template)
		annotations["description"] = buildManagedMetricAlertDescriptionAnnotation(policy, template, condition)
		annotations["current_value"] = `{{ printf "%.6f" $value }}`

		builtExpr, err := buildMetricTemplatePromQL(policy, template, condition)
		if err != nil {
			return nil, false, err
		}
		expr = builtExpr
		forMinutes = maxInt(condition.WindowMinutes, 1)
	default:
		return nil, false, nil
	}

	rule := &rulefmt.Rule{
		Alert:       sanitizeManagedPrometheusAlertName(policy.ID),
		Expr:        expr,
		For:         model.Duration(time.Duration(forMinutes) * time.Minute),
		Labels:      ruleLabels,
		Annotations: annotations,
	}
	return rule, true, nil
}

func sanitizeManagedPrometheusAlertName(policyID uint) string {
	return fmt.Sprintf("STXManagedPolicy_%d", policyID)
}

func findAlertPolicyTemplateSummary(templateKey string) (*AlertPolicyTemplateSummaryDTO, bool) {
	for _, item := range allAlertPolicyTemplateSummaries() {
		if item == nil {
			continue
		}
		if strings.TrimSpace(item.Key) == strings.TrimSpace(templateKey) {
			return item, true
		}
	}
	return nil, false
}

type resolvedMetricPolicyCondition struct {
	Operator      string
	Threshold     string
	WindowMinutes int
}

// ListEnabledManagedMetricAlertPolicies 返回启用中的 metrics_template 策略（按 id 排序），
// 是 managed 规则组的数据源。
// ListEnabledManagedMetricAlertPolicies returns enabled metrics-template policies ordered by
// id; it is the data source of the managed rule group.
func (r *Repository) ListEnabledManagedMetricAlertPolicies(ctx context.Context) ([]*AlertPolicy, error) {
	var policies []*AlertPolicy
	if err := r.db.WithContext(ctx).
		Where("enabled = ? AND policy_type IN ?", true, []AlertPolicyBuilderKind{AlertPolicyBuilderKindMetricsTemplate}).
		Order("id ASC").
		Find(&policies).Error; err != nil {
		return nil, err
	}
	return policies, nil
}

func resolveMetricPolicyCondition(policy *AlertPolicy, template *AlertPolicyTemplateSummaryDTO) resolvedMetricPolicyCondition {
	condition := resolvedMetricPolicyCondition{
		Operator:      strings.TrimSpace(firstNonEmpty(template.DefaultOperator, ">")),
		Threshold:     strings.TrimSpace(firstNonEmpty(template.DefaultThreshold, "0")),
		WindowMinutes: template.DefaultWindowMins,
	}
	conditions := unmarshalAlertPolicyConditions(policy.ConditionsJSON)
	if len(conditions) > 0 && conditions[0] != nil {
		if value := strings.TrimSpace(conditions[0].Operator); value != "" {
			condition.Operator = value
		}
		if value := strings.TrimSpace(conditions[0].Threshold); value != "" {
			condition.Threshold = value
		}
		if conditions[0].WindowMinutes > 0 {
			condition.WindowMinutes = conditions[0].WindowMinutes
		}
	}
	if condition.WindowMinutes <= 0 {
		condition.WindowMinutes = 1
	}
	if condition.Operator == "" {
		condition.Operator = ">"
	}
	if condition.Threshold == "" {
		condition.Threshold = "0"
	}
	return condition
}

func buildMetricTemplatePromQL(policy *AlertPolicy, template *AlertPolicyTemplateSummaryDTO, condition resolvedMetricPolicyCondition) (string, error) {
	if template == nil {
		return "", fmt.Errorf("template is required")
	}

	matchers := []string{`job="seatunnel_engine_http"`}
	clusterID := strings.TrimSpace(policy.ClusterID)
	if clusterID != "" && !strings.EqualFold(clusterID, "all") {
		matchers = append(matchers, fmt.Sprintf(`cluster_id=%q`, clusterID))
	}
	window := fmt.Sprintf("%dm", maxInt(condition.WindowMinutes, 1))
	op := strings.TrimSpace(condition.Operator)
	threshold := strings.TrimSpace(condition.Threshold)
	if threshold == "" {
		threshold = "0"
	}

	switch strings.TrimSpace(template.Key) {
	case "cpu_usage_high":
		return fmt.Sprintf(`sum by (cluster_id, cluster_name, cluster, instance) (rate(%s[%s])) %s %s`, buildPrometheusSelector("process_cpu_seconds_total", matchers...), window, op, threshold), nil
	case "memory_usage_high":
		used := buildPrometheusSelector("jvm_memory_bytes_used", append(matchers, `area="heap"`)...)
		maxv := buildPrometheusSelector("jvm_memory_bytes_max", append(matchers, `area="heap"`)...)
		return fmt.Sprintf(`max by (cluster_id, cluster_name, cluster, instance) ((%s) / clamp_min((%s), 1)) %s %s`, used, maxv, op, threshold), nil
	case "fd_usage_high":
		openFds := buildPrometheusSelector("process_open_fds", matchers...)
		maxFds := buildPrometheusSelector("process_max_fds", matchers...)
		return fmt.Sprintf(`max by (cluster_id, cluster_name, cluster, instance) ((%s) / clamp_min((%s), 1)) %s %s`, openFds, maxFds, op, threshold), nil
	case "failed_jobs_high":
		return fmt.Sprintf(`sum by (cluster_id, cluster_name, cluster, instance) (%s) %s %s`, buildPrometheusSelector("job_count", append(matchers, `state="FAILED"`)...), op, threshold), nil
	case "job_thread_pool_queue_backlog_high":
		return fmt.Sprintf(`max by (cluster_id, cluster_name, cluster, instance) (%s) %s %s`, buildPrometheusSelector("job_thread_pool_queueTaskCount", matchers...), op, threshold), nil
	case "job_thread_pool_rejection_high":
		return fmt.Sprintf(`sum by (cluster_id, cluster_name, cluster, instance) (increase(%s[%s])) %s %s`, buildPrometheusSelector("job_thread_pool_rejection_total", matchers...), window, op, threshold), nil
	case "deadlocked_threads_detected":
		return fmt.Sprintf(`max by (cluster_id, cluster_name, cluster, instance) (%s) %s %s`, buildPrometheusSelector("jvm_threads_deadlocked", matchers...), op, threshold), nil
	case "split_brain_risk":
		return fmt.Sprintf(`min by (cluster_id, cluster_name, cluster, instance) (%s) %s %s`, buildPrometheusSelector("hazelcast_partition_isClusterSafe", matchers...), op, threshold), nil
	case AlertRuleKeyPlatformServerMemoryHigh,
		AlertRuleKeyPlatformServerHTTP5xxRatioHigh,
		AlertRuleKeyPlatformTSDBCompactionFailed,
		AlertRuleKeyPlatformAgentMemoryHigh,
		AlertRuleKeyPlatformAgentVersionMismatch:
		// 平台自监控模板不走 seatunnel_engine_http 匹配器：信号来自内嵌自抓取
		//（job=stx_embedded）与 Agent 上报通道（stx_agent_* / stx_host_*）。
		// Platform self-monitoring templates skip the seatunnel_engine_http matcher:
		// signals come from the embedded self-scrape (job=stx_embedded) and the
		// agent report channel (stx_agent_* / stx_host_*).
		return buildPlatformMetricPromQL(template.Key, window, op, threshold)
	default:
		return "", fmt.Errorf("unsupported metrics template: %s", template.Key)
	}
}

// buildPlatformMetricPromQL 生成平台自监控模板的 PromQL 表达式。
// window 作用于 range 函数窗口（5xx 比例），阈值/算子来自策略条件；
// 版本不一致模板把当前 server 版本注入 stx_agent_build_info 的版本不等匹配器（%q 转义）。
// buildPlatformMetricPromQL renders the PromQL for platform self-monitoring
// templates. window feeds range functions (5xx ratio), operator/threshold come
// from the policy condition; the version-mismatch template injects the current
// server version into the stx_agent_build_info inequality matcher (%q-escaped).
func buildPlatformMetricPromQL(templateKey, window, op, threshold string) (string, error) {
	switch strings.TrimSpace(templateKey) {
	case AlertRuleKeyPlatformServerMemoryHigh:
		return fmt.Sprintf(`process_resident_memory_bytes{job="stx_embedded"} %s %s`, op, threshold), nil
	case AlertRuleKeyPlatformServerHTTP5xxRatioHigh:
		return fmt.Sprintf(`sum(rate(stx_server_http_requests_total{code="5xx"}[%s])) / sum(rate(stx_server_http_requests_total[%s])) %s %s`, window, window, op, threshold), nil
	case AlertRuleKeyPlatformTSDBCompactionFailed:
		// 固定 1h 窗口累计 compaction 失败次数；触发时机由 for 持续时间控制。
		// The 1h window is pinned: compaction failures accumulate, and the for
		// duration controls when the alert fires.
		return fmt.Sprintf(`increase(prometheus_tsdb_compactions_failed_total{job="stx_embedded"}[1h]) %s %s`, op, threshold), nil
	case AlertRuleKeyPlatformAgentMemoryHigh:
		return fmt.Sprintf(`max by (host_id, host_name) (stx_agent_process_resident_memory_bytes) %s %s`, op, threshold), nil
	case AlertRuleKeyPlatformAgentVersionMismatch:
		return fmt.Sprintf(`max by (host_id, host_name, version) (stx_agent_build_info{version!=%q}) %s %s`, version.Version, op, threshold), nil
	default:
		return "", fmt.Errorf("unsupported platform metrics template: %s", templateKey)
	}
}

func buildPrometheusSelector(metric string, matchers ...string) string {
	trimmedMetric := strings.TrimSpace(metric)
	items := make([]string, 0, len(matchers))
	for _, matcher := range matchers {
		matcher = strings.TrimSpace(matcher)
		if matcher == "" {
			continue
		}
		items = append(items, matcher)
	}
	if len(items) == 0 {
		return trimmedMetric
	}
	return fmt.Sprintf("%s{%s}", trimmedMetric, strings.Join(items, ","))
}

func buildMetricConditionDescriptionZH(condition resolvedMetricPolicyCondition) string {
	return fmt.Sprintf("持续 %d 分钟，条件：%s %s", maxInt(condition.WindowMinutes, 1), strings.TrimSpace(firstNonEmpty(condition.Operator, ">")), strings.TrimSpace(firstNonEmpty(condition.Threshold, "0")))
}

func buildManagedMetricAlertSummaryAnnotation(policy *AlertPolicy, template *AlertPolicyTemplateSummaryDTO) string {
	policyName := escapePrometheusTemplateText(strings.TrimSpace(firstNonEmpty(policy.Name, "未命名策略")))
	templateName := escapePrometheusTemplateText(alertPolicyTemplateDisplayNameZH(strings.TrimSpace(template.Key)))
	return fmt.Sprintf(`{{ $labels.cluster_name }} / {{ $labels.instance }} 触发指标告警：%s（%s）`, policyName, templateName)
}

func buildManagedMetricAlertDescriptionAnnotation(policy *AlertPolicy, template *AlertPolicyTemplateSummaryDTO, condition resolvedMetricPolicyCondition) string {
	description := escapePrometheusTemplateText(strings.TrimSpace(firstNonEmpty(
		policy.Description,
		alertPolicyTemplateDescriptionZH(template.Key),
		alertPolicyTemplateDisplayNameZH(template.Key),
	)))
	conditionText := escapePrometheusTemplateText(buildMetricConditionDescriptionZH(condition))
	return fmt.Sprintf(`%s；当前值：{{ printf "%%.6f" $value }}；%s。`, description, conditionText)
}

func escapePrometheusTemplateText(value string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(strings.TrimSpace(value))
}

func alertPolicyTemplateDisplayNameZH(templateKey string) string {
	switch strings.TrimSpace(templateKey) {
	case "master_unavailable":
		return "Master 不可用"
	case "worker_insufficient":
		return "可用 Worker 不足"
	case AlertRuleKeyNodeOffline:
		return "节点离线"
	case "agent_offline":
		return "Agent 离线"
	case AlertRuleKeyProcessCrashed:
		return "进程崩溃"
	case AlertRuleKeyProcessRestartFailed:
		return "进程重启失败"
	case AlertRuleKeyProcessRestartLimitReached:
		return "达到重启上限"
	case AlertRuleKeyClusterRestartRequested:
		return "重启已触发"
	case AlertRuleKeyNodeStopRequested:
		return "节点停止已触发"
	case "cpu_usage_high":
		return "CPU 异常"
	case "memory_usage_high":
		return "内存异常"
	case "fd_usage_high":
		return "文件句柄异常"
	case "failed_jobs_high":
		return "失败作业异常"
	case "job_thread_pool_queue_backlog_high":
		return "作业线程池积压"
	case "job_thread_pool_rejection_high":
		return "作业线程池拒绝"
	case "deadlocked_threads_detected":
		return "死锁线程异常"
	case "split_brain_risk":
		return "集群安全性异常"
	case AlertRuleKeyPlatformServerMemoryHigh:
		return "STX server 内存过高"
	case AlertRuleKeyPlatformServerHTTP5xxRatioHigh:
		return "STX server 5xx 比例过高"
	case AlertRuleKeyPlatformTSDBCompactionFailed:
		return "内嵌 tsdb compaction 失败"
	case AlertRuleKeyPlatformAgentMemoryHigh:
		return "Agent 内存过高"
	case AlertRuleKeyPlatformAgentVersionMismatch:
		return "Agent 版本不一致"
	default:
		return strings.TrimSpace(firstNonEmpty(templateKey, "指标告警"))
	}
}

func alertPolicyTemplateDescriptionZH(templateKey string) string {
	switch strings.TrimSpace(templateKey) {
	case "master_unavailable":
		return "检测受管 SeaTunnel 集群是否已经没有健康的 Master / Coordinator 节点。"
	case "worker_insufficient":
		return "检测单个集群中健康 Worker 数量是否持续低于预期基线。"
	case AlertRuleKeyNodeOffline:
		return "检测节点心跳或运行态可见性是否持续丢失。"
	case "agent_offline":
		return "检测控制面是否失去了集群运维所需的 Agent 连接。"
	case AlertRuleKeyProcessCrashed:
		return "跟踪受管 SeaTunnel 进程是否发生重复崩溃。"
	case AlertRuleKeyProcessRestartFailed:
		return "检测自动重启已经无法恢复故障进程的情况。"
	case AlertRuleKeyProcessRestartLimitReached:
		return "检测运行时是否进入类似 crash-loop 的重启上限异常。"
	case AlertRuleKeyClusterRestartRequested:
		return "当用户在控制面触发集群或节点重启时发送通知。"
	case AlertRuleKeyNodeStopRequested:
		return "当用户在控制面手动停止节点时立即发送通知。"
	case "cpu_usage_high":
		return "基于 process_cpu_seconds_total 持续检测 SeaTunnel 进程 CPU 负载，表示评估窗口内的平均 CPU 核使用量，而不是宿主机总 CPU 百分比。"
	case "memory_usage_high":
		return "基于 jvm_memory_bytes_used 和 jvm_memory_bytes_max 持续检测 SeaTunnel JVM 堆内存使用率是否超过阈值。"
	case "fd_usage_high":
		return "基于 process_open_fds 和 process_max_fds 提前发现文件句柄耗尽风险。"
	case "failed_jobs_high":
		return "基于带 FAILED 状态标签的 job_count 持续检测失败作业数量是否异常。"
	case "job_thread_pool_queue_backlog_high":
		return "基于 job_thread_pool_queueTaskCount 检测作业线程池队列积压，避免演变为高延迟或饥饿。"
	case "job_thread_pool_rejection_high":
		return "当 job_thread_pool_rejection_total 持续增长时告警，表示任务已被拒绝而不是被排队或执行。"
	case "deadlocked_threads_detected":
		return "基于 jvm_threads_deadlocked 检测 JVM 死锁线程，避免任务执行与恢复流程被卡死。"
	case "split_brain_risk":
		return "基于 hazelcast_partition_isClusterSafe 检测集群分区安全性异常，提前发现脑裂风险。"
	case AlertRuleKeyPlatformServerMemoryHigh:
		return "检测 STX server 进程 RSS 内存是否持续超过配置的内存上限。"
	case AlertRuleKeyPlatformServerHTTP5xxRatioHigh:
		return "检测控制面全部 HTTP 请求中 5xx 响应的占比是否持续超过配置阈值。"
	case AlertRuleKeyPlatformTSDBCompactionFailed:
		return "内嵌 tsdb 报告 compaction 失败时立即告警；持续失败会让可观测历史数据面临丢失风险。"
	case AlertRuleKeyPlatformAgentMemoryHigh:
		return "检测任一受管主机上的 Agent 进程 RSS 是否持续超过配置的内存上限。"
	case AlertRuleKeyPlatformAgentVersionMismatch:
		return "检测受管 Agent 上报的版本号是否持续与 STX server 版本不一致。"
	default:
		return strings.TrimSpace(firstNonEmpty(alertPolicyTemplateDisplayNameZH(templateKey), "指标告警"))
	}
}

func maxInt(left, right int) int {
	if left > right {
		return left
	}
	return right
}

// metricEvalIntervalFromConfig 解析 observability.eval_interval（Prometheus duration 语法）；
// 空值或非法值回落默认评估间隔。
// metricEvalIntervalFromConfig parses observability.eval_interval (Prometheus duration syntax);
// empty or invalid values fall back to the default eval interval.
func metricEvalIntervalFromConfig() time.Duration {
	trimmed := strings.TrimSpace(config.Config.Observability.EvalInterval)
	if trimmed == "" {
		return metricEvaluatorDefaultInterval
	}
	d, err := model.ParseDuration(trimmed)
	if err != nil || d <= 0 {
		return metricEvaluatorDefaultInterval
	}
	return time.Duration(d)
}
