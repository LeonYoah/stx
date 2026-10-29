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
	"errors"
	"fmt"
	"log"
	"net/mail"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/LeonYoah/stx/internal/config"
	"github.com/LeonYoah/stx/internal/pkg/syncalert"
	"github.com/LeonYoah/stx/internal/pkg/timex"
)

const (
	// syncAlertQueueSize 是信号缓冲区容量；满了丢弃新信号，保证 sync 侧永不阻塞。
	// syncAlertQueueSize is the signal buffer capacity; when full new signals are dropped so sync never blocks.
	syncAlertQueueSize = 1024
	// syncAlertSweepInterval 是冷却提醒巡检的间隔。
	// syncAlertSweepInterval is how often the cooldown reminder sweep runs.
	syncAlertSweepInterval = time.Minute
	// syncAlertMessageDisplayLimit 是告警中心和通知里错误摘要的最大字符数，与信号存储上限一致（syncalert.MaxMessageLength）。
	// syncAlertMessageDisplayLimit caps the error summary shown in the alert center and notifications; it equals the
	// stored limit (syncalert.MaxMessageLength) so storage and display agree.
	syncAlertMessageDisplayLimit = syncalert.MaxMessageLength
	// defaultSyncStatusUnavailableWindow 是 D1 规则窗口未配置时的默认值。
	// defaultSyncStatusUnavailableWindow is the D1 rule window when none is configured.
	defaultSyncStatusUnavailableWindow = 300 * time.Second
)

// syncAlertRuleMeta 是规则目录里的名称与默认级别；第一期级别固定，不支持配置。
// syncAlertRuleMeta holds a rule's name and default severity; in phase one the severity is fixed.
type syncAlertRuleMeta struct {
	Name     string
	Severity AlertSeverity
}

var syncAlertRuleCatalog = map[string]syncAlertRuleMeta{
	syncalert.RuleSubmitFailed:         {Name: "作业提交失败", Severity: AlertSeverityCritical},
	syncalert.RuleSubmitTimeout:        {Name: "作业提交超时", Severity: AlertSeverityCritical},
	syncalert.RuleJobFailed:            {Name: "作业运行失败", Severity: AlertSeverityCritical},
	syncalert.RuleStopTimeout:          {Name: "作业停止超时", Severity: AlertSeverityWarning},
	syncalert.RuleCDCDelayHigh:         {Name: "CDC 延迟过高", Severity: AlertSeverityWarning},
	syncalert.RuleStatusUnavailable:    {Name: "作业状态获取失败", Severity: AlertSeverityWarning},
	syncalert.RuleEventChannelDegraded: {Name: "事件通道降级", Severity: AlertSeverityWarning},
}

// syncAlertRuleName 返回规则名称，未知规则原样返回键。
// syncAlertRuleName returns the rule name, or the key itself for unknown rules.
func syncAlertRuleName(ruleKey string) string {
	if meta, ok := syncAlertRuleCatalog[ruleKey]; ok {
		return meta.Name
	}
	return ruleKey
}

// SyncAlertHub 接收 sync 的告警信号，用一个后台 worker 串行处理：建立或累计告警阶段、按策略投递、恢复，
// 每分钟巡检一次冷却提醒。实现 syncalert.Reporter。
// SyncAlertHub receives sync alert signals and processes them serially on one background worker: it opens or
// counts episodes, delivers per policy and resolves them, and sweeps cooldown reminders every minute. It implements
// syncalert.Reporter.
type SyncAlertHub struct {
	service *Service
	queue   chan syncalert.Signal
	dropped atomic.Int64
	// lastDropLogMinute 让缓冲区满的 warning 每分钟最多打一条。
	// lastDropLogMinute keeps the buffer-full warning to at most one per minute.
	lastDropLogMinute atomic.Int64
	now               func() time.Time
}

// NewSyncAlertHub 创建告警 hub；调用 Start 之前收到的信号会留在缓冲区里。
// NewSyncAlertHub creates the alert hub; signals received before Start stay in the buffer.
func (s *Service) NewSyncAlertHub() *SyncAlertHub {
	return &SyncAlertHub{service: s, queue: make(chan syncalert.Signal, syncAlertQueueSize)}
}

func (h *SyncAlertHub) clock() time.Time {
	if h.now != nil {
		return h.now().UTC()
	}
	return timeNowUTC()
}

// ReportSyncAlert 非阻塞地放入信号；缓冲区满时丢弃并计数。
// ReportSyncAlert enqueues a signal without blocking; when the buffer is full the signal is dropped and counted.
func (h *SyncAlertHub) ReportSyncAlert(signal syncalert.Signal) {
	if h == nil {
		return
	}
	if signal.OccurredAt.IsZero() {
		signal.OccurredAt = h.clock()
	}
	select {
	case h.queue <- signal:
	default:
		dropped := h.dropped.Add(1)
		minute := h.clock().Unix() / 60
		if h.lastDropLogMinute.Swap(minute) != minute {
			log.Printf("[Monitoring] sync alert buffer full, signal dropped: rule=%s task=%d cluster=%d dropped_total=%d / 同步告警缓冲区已满，信号被丢弃",
				signal.RuleKey, signal.TaskID, signal.ClusterID, dropped)
		}
	}
}

// Start 启动 worker：处理信号，并每分钟巡检一次冷却提醒；信号与巡检在同一个协程里串行，不会互相竞争。
// Start launches the worker: it handles signals and sweeps reminders every minute; both run serially on the same
// goroutine so they never race.
func (h *SyncAlertHub) Start(ctx context.Context) {
	if h == nil || h.service == nil || h.service.repo == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(syncAlertSweepInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case signal := <-h.queue:
				if err := h.handle(ctx, signal); err != nil {
					log.Printf("[Monitoring] handle sync alert failed: rule=%s task=%d cluster=%d err=%v / 处理同步告警失败",
						signal.RuleKey, signal.TaskID, signal.ClusterID, err)
				}
			case <-ticker.C:
				if err := h.sweep(ctx); err != nil {
					log.Printf("[Monitoring] sweep sync alert reminders failed: %v / 同步告警提醒巡检失败", err)
				}
			}
		}
	}()
}

// handle 处理一条信号。
// handle processes one signal.
func (h *SyncAlertHub) handle(ctx context.Context, signal syncalert.Signal) error {
	if signal.OccurredAt.IsZero() {
		signal.OccurredAt = h.clock()
	}
	signal.OccurredAt = signal.OccurredAt.UTC()
	// sync 不知道集群名，这里补上，供告警中心和通知显示。
	// sync does not know cluster names, so fill it in here for the alert center and notifications.
	if strings.TrimSpace(signal.ClusterName) == "" && signal.ClusterID > 0 && h.service.clusterService != nil {
		if info, err := h.service.clusterService.Get(ctx, signal.ClusterID); err == nil && info != nil {
			signal.ClusterName = strings.TrimSpace(info.Name)
		}
	}
	switch signal.Kind {
	case syncalert.KindFire:
		switch {
		case syncalert.IsTaskRule(signal.RuleKey):
			return h.fireTask(ctx, signal)
		case syncalert.IsClusterRule(signal.RuleKey):
			return h.fireCluster(ctx, signal)
		}
		return nil
	case syncalert.KindObserve:
		return h.observeCluster(ctx, signal)
	case syncalert.KindResolve:
		return h.resolve(ctx, signal)
	case syncalert.KindRefresh:
		return h.refreshTask(ctx, signal)
	default:
		return nil
	}
}

// fireTask 处理作业级 Fire：任务没有配置这条规则时直接丢弃，不建阶段（"不配不发"）。
// fireTask handles a job-level Fire: when the task has not configured the rule the signal is dropped and no episode
// is created ("no config, no alert").
func (h *SyncAlertHub) fireTask(ctx context.Context, signal syncalert.Signal) error {
	if signal.TaskID == 0 {
		return nil
	}
	policy, err := h.service.repo.GetSyncJobAlertPolicyByTaskID(ctx, signal.TaskID)
	if err != nil {
		return err
	}
	if policy == nil || !policy.Enabled || !syncJobPolicyHasRule(policy, signal.RuleKey) {
		return nil
	}
	episode, created, err := h.upsertEpisode(ctx, SyncAlertEpisodeScopeTask, signal, syncAlertRuleCatalog[signal.RuleKey].Severity)
	if err != nil || !created {
		return err
	}
	return h.dispatchEpisode(ctx, episode, []*AlertPolicy{policy}, NotificationDeliveryEventTypeFiring, true, false)
}

// fireCluster 处理集群级 Fire：集群规则停用时丢弃；没有策略时只建阶段，告警中心照样能看到。
// fireCluster handles a cluster-level Fire: a disabled cluster rule drops it; without policies only the episode is
// created and still shows in the alert center.
func (h *SyncAlertHub) fireCluster(ctx context.Context, signal syncalert.Signal) error {
	if signal.ClusterID == 0 {
		return nil
	}
	rule, err := h.clusterRule(ctx, signal.ClusterID, signal.RuleKey)
	if err != nil {
		return err
	}
	if rule == nil || !rule.Enabled {
		return nil
	}
	episode, created, err := h.upsertEpisode(ctx, SyncAlertEpisodeScopeCluster, signal, normalizeAlertPolicySeverity(rule.Severity))
	if err != nil || !created {
		return err
	}
	policies, err := h.clusterPolicies(ctx, signal.ClusterID, signal.RuleKey)
	if err != nil {
		return err
	}
	return h.dispatchEpisode(ctx, episode, policies, NotificationDeliveryEventTypeFiring, true, false)
}

// observeCluster 处理 D1 观测：持续失败超过规则窗口才算成立；已有阶段时只累计次数。
// observeCluster handles a D1 observation: it only fires once failures have lasted the rule window; an existing
// episode just counts the occurrence.
func (h *SyncAlertHub) observeCluster(ctx context.Context, signal syncalert.Signal) error {
	if signal.ClusterID == 0 || !syncalert.IsClusterRule(signal.RuleKey) {
		return nil
	}
	existing, err := h.service.repo.FindFiringSyncAlertEpisode(ctx, SyncAlertEpisodeScopeCluster, signal.ClusterID, 0, signal.RuleKey)
	if err != nil {
		return err
	}
	if existing != nil {
		return h.fireCluster(ctx, signal)
	}
	rule, err := h.clusterRule(ctx, signal.ClusterID, signal.RuleKey)
	if err != nil || rule == nil || !rule.Enabled {
		return err
	}
	window := time.Duration(rule.WindowSeconds) * time.Second
	if window <= 0 {
		window = defaultSyncStatusUnavailableWindow
	}
	since := signal.Since
	if since.IsZero() {
		since = signal.OccurredAt
	}
	if signal.OccurredAt.Sub(since.UTC()) < window {
		return nil
	}
	return h.fireCluster(ctx, signal)
}

// resolve 按 ResolveScope 关闭阶段，并按策略的"发送恢复通知"投递恢复。
// resolve closes episodes by ResolveScope and delivers recovery when the policy sends recovery notifications.
func (h *SyncAlertHub) resolve(ctx context.Context, signal syncalert.Signal) error {
	repo := h.service.repo
	switch signal.ResolveScope {
	case syncalert.ResolveTaskRunHealthy, syncalert.ResolveJobTerminal:
		if signal.TaskID == 0 {
			return nil
		}
		rules := []string{syncalert.RuleSubmitFailed, syncalert.RuleSubmitTimeout, syncalert.RuleJobFailed}
		if signal.ResolveScope == syncalert.ResolveJobTerminal {
			rules = []string{syncalert.RuleStopTimeout}
		}
		episodes, err := repo.ListFiringSyncAlertEpisodesByTask(ctx, signal.TaskID, rules)
		if err != nil || len(episodes) == 0 {
			return err
		}
		policy, err := repo.GetSyncJobAlertPolicyByTaskID(ctx, signal.TaskID)
		if err != nil {
			return err
		}
		var errs []error
		for _, episode := range episodes {
			if signal.ResolveScope == syncalert.ResolveJobTerminal && episode.LastJobID != signal.JobID {
				continue
			}
			errs = append(errs, h.resolveEpisode(ctx, episode, signal.OccurredAt, syncAlertResolveRecovered, singlePolicy(policy)))
		}
		return errors.Join(errs...)
	case syncalert.ResolveClusterRule:
		if signal.ClusterID == 0 {
			return nil
		}
		episode, err := repo.FindFiringSyncAlertEpisode(ctx, SyncAlertEpisodeScopeCluster, signal.ClusterID, 0, signal.RuleKey)
		if err != nil || episode == nil {
			return err
		}
		policies, err := h.clusterPolicies(ctx, signal.ClusterID, signal.RuleKey)
		if err != nil {
			return err
		}
		return h.resolveEpisode(ctx, episode, signal.OccurredAt, syncAlertResolveRecovered, policies)
	case syncalert.ResolveCDCDelay:
		return h.resolveCDCDelay(ctx, signal)
	default:
		return nil
	}
}

// resolveEpisode 结束阶段；reason 为 recovered 时按策略投递恢复通知。
// resolveEpisode ends an episode; for the recovered reason it delivers recovery notifications per policy.
func (h *SyncAlertHub) resolveEpisode(ctx context.Context, episode *SyncAlertEpisode, at time.Time, reason string, policies []*AlertPolicy) error {
	resolvedAt := at.UTC()
	episode.Status = SyncAlertEpisodeResolved
	episode.ResolvedAt = &resolvedAt
	episode.ResolveReason = reason
	if err := h.service.repo.SaveSyncAlertEpisode(ctx, episode); err != nil {
		return err
	}
	if reason != syncAlertResolveRecovered {
		return nil
	}
	recovering := make([]*AlertPolicy, 0, len(policies))
	for _, policy := range policies {
		if policy != nil && policy.Enabled && policy.SendRecovery {
			recovering = append(recovering, policy)
		}
	}
	return h.dispatchEpisode(ctx, episode, recovering, NotificationDeliveryEventTypeResolved, false, false)
}

// upsertEpisode 找到同一个键的 firing 阶段并累计次数；没有（或已被人工关闭）时新建。返回是否新建。
// upsertEpisode finds the firing episode of the key and counts the occurrence; when there is none (or it was
// manually closed) it creates one. It reports whether a new episode was created.
func (h *SyncAlertHub) upsertEpisode(ctx context.Context, scope SyncAlertEpisodeScope, signal syncalert.Signal, severity AlertSeverity) (*SyncAlertEpisode, bool, error) {
	repo := h.service.repo
	taskID := signal.TaskID
	if scope == SyncAlertEpisodeScopeCluster {
		taskID = 0
	}
	existing, err := repo.FindFiringSyncAlertEpisode(ctx, scope, signal.ClusterID, taskID, signal.RuleKey)
	if err != nil {
		return nil, false, err
	}
	if existing != nil {
		closed, err := h.episodeClosed(ctx, existing)
		if err != nil {
			return nil, false, err
		}
		if closed {
			// 人工关闭后再出问题：结束旧阶段，开新阶段并重新通知。
			// A problem after a manual close ends the old episode and opens a new one that notifies again.
			if err := h.resolveEpisode(ctx, existing, signal.OccurredAt, syncAlertResolveClosed, nil); err != nil {
				return nil, false, err
			}
			existing = nil
		}
	}
	if existing != nil {
		existing.OccurrenceCount++
		applySignalToEpisode(existing, signal)
		return existing, false, repo.SaveSyncAlertEpisode(ctx, existing)
	}
	episode := &SyncAlertEpisode{
		Scope:           scope,
		ClusterID:       signal.ClusterID,
		TaskID:          taskID,
		RuleKey:         signal.RuleKey,
		Status:          SyncAlertEpisodeFiring,
		Severity:        severity,
		OccurrenceCount: 1,
		FirstFiredAt:    signal.OccurredAt,
	}
	applySignalToEpisode(episode, signal)
	if err := repo.CreateSyncAlertEpisode(ctx, episode); err != nil {
		return nil, false, err
	}
	return episode, true, nil
}

// applySignalToEpisode 用最近一次信号更新阶段的展示字段；空值不覆盖已有内容。
// applySignalToEpisode refreshes the episode's display fields from the latest signal; empty values keep the old ones.
func applySignalToEpisode(episode *SyncAlertEpisode, signal syncalert.Signal) {
	episode.LastOccurredAt = signal.OccurredAt
	if name := normalizeSyncAlertText(signal.ClusterName, 255); name != "" {
		episode.ClusterName = name
	}
	if name := normalizeSyncAlertText(signal.TaskName, 255); name != "" {
		episode.TaskName = name
	}
	if signal.TaskCreatorID > 0 {
		episode.TaskCreatorID = signal.TaskCreatorID
	}
	if signal.JobID > 0 {
		episode.LastJobID = signal.JobID
		episode.LastEngineJobID = normalizeSyncAlertText(signal.EngineJobID, 64)
		episode.LastTriggerSource = normalizeSyncAlertText(signal.TriggerSource, 40)
	}
	if message := normalizeSyncAlertText(signal.Message, syncalert.MaxMessageLength); message != "" {
		episode.LastMessage = message
	}
	if signal.CDCDelay != nil {
		setEpisodeCDCDelayDetail(episode, signal.CDCDelay)
	}
}

// episodeClosed 判断阶段是否在告警中心被人工关闭。
// episodeClosed reports whether the episode was manually closed in the alert center.
func (h *SyncAlertHub) episodeClosed(ctx context.Context, episode *SyncAlertEpisode) (bool, error) {
	state, err := h.service.repo.GetAlertStateBySourceKey(ctx, buildSyncAlertSourceKey(episode.ID))
	if err != nil {
		return false, err
	}
	return resolveAlertHandlingStatus(state, timex.Now()) == AlertHandlingStatusClosed, nil
}

// sweep 给 firing 阶段发冷却提醒：复用投递去重，只有过了策略冷却时间才会真正重发；静默和关闭照常生效。
// sweep sends cooldown reminders for firing episodes: it reuses delivery dedupe so a notification is only resent
// after the policy cooldown, and silence and close still apply.
func (h *SyncAlertHub) sweep(ctx context.Context) error {
	episodes, err := h.service.repo.ListFiringSyncAlertEpisodes(ctx, 500)
	if err != nil {
		return err
	}
	var errs []error
	for _, episode := range episodes {
		var policies []*AlertPolicy
		if episode.Scope == SyncAlertEpisodeScopeTask {
			policy, err := h.service.repo.GetSyncJobAlertPolicyByTaskID(ctx, episode.TaskID)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			if policy == nil || !policy.Enabled || !syncJobPolicyHasRule(policy, episode.RuleKey) {
				continue
			}
			// B4 "无法确认"超过 24 小时：停止提醒，阶段保留。
			// B4 "unconfirmable" for over 24 hours: reminders stop while the episode stays open.
			if syncAlertRemindersPaused(episode) {
				continue
			}
			policies = []*AlertPolicy{policy}
		} else {
			policies, err = h.clusterPolicies(ctx, episode.ClusterID, episode.RuleKey)
			if err != nil {
				errs = append(errs, err)
				continue
			}
		}
		reminding := make([]*AlertPolicy, 0, len(policies))
		for _, policy := range policies {
			if policy.CooldownMinutes > 0 {
				reminding = append(reminding, policy)
			}
		}
		if len(reminding) == 0 {
			continue
		}
		errs = append(errs, h.dispatchEpisode(ctx, episode, reminding, NotificationDeliveryEventTypeFiring, false, true))
	}
	return errors.Join(errs...)
}

// dispatchEpisode 把阶段投递到每条策略的渠道，并更新策略的执行统计；reminder 表示冷却提醒（标题不同）。
// dispatchEpisode delivers the episode to each policy's channels and updates the policy execution stats; reminder
// marks a cooldown reminder (with a different title).
func (h *SyncAlertHub) dispatchEpisode(ctx context.Context, episode *SyncAlertEpisode, policies []*AlertPolicy, eventType NotificationDeliveryEventType, countMatch, reminder bool) error {
	if len(policies) == 0 {
		return nil
	}
	repo := h.service.repo
	channels, err := repo.ListNotificationChannels(ctx)
	if err != nil {
		return err
	}
	channelMap := make(map[uint]*NotificationChannel, len(channels))
	for _, channel := range channels {
		if channel != nil {
			channelMap[channel.ID] = channel
		}
	}
	sourceKey := buildSyncAlertSourceKey(episode.ID)
	matchedAt := h.clock()
	var errs []error
	for _, policy := range policies {
		if policy == nil {
			continue
		}
		var extra []string
		if episode.Scope == SyncAlertEpisodeScopeTask && policy.NotifyTaskOwner {
			extra = h.service.resolveUserEmails(ctx, episode.TaskCreatorID)
		}
		message := buildSyncAlertMessage(episode, policy, eventType, extra, h.clock(), reminder)
		summary, dispatchErr := h.service.dispatchAlertPolicyChannels(ctx, message, policy, channelMap, eventType, sourceKey, eventType == NotificationDeliveryEventTypeResolved)
		if dispatchErr != nil {
			errs = append(errs, dispatchErr)
		}
		update := &AlertPolicyExecutionStateUpdate{
			DeliveryCountDelta:  summary.DeliveryCountDelta,
			LastDeliveredAt:     summary.LastDeliveredAt,
			LastExecutionStatus: summary.LastExecutionStatus,
			LastExecutionError:  strings.TrimSpace(summary.LastExecutionError),
		}
		if countMatch {
			update.MatchCountDelta = 1
			update.LastMatchedAt = &matchedAt
		}
		if err := repo.ApplyAlertPolicyExecutionState(ctx, policy.ID, update); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// clusterRule 读取集群的默认规则，必要时先补齐默认规则。
// clusterRule loads the cluster's default rule, creating missing defaults first.
func (h *SyncAlertHub) clusterRule(ctx context.Context, clusterID uint, ruleKey string) (*AlertRule, error) {
	rules, err := h.service.getClusterRuleMap(ctx, clusterID, map[uint]map[string]*AlertRule{})
	if err != nil {
		return nil, err
	}
	return rules[ruleKey], nil
}

// clusterPolicies 返回绑定到集群这条规则的启用策略。
// clusterPolicies returns the enabled policies bound to the cluster's rule.
func (h *SyncAlertHub) clusterPolicies(ctx context.Context, clusterID uint, ruleKey string) ([]*AlertPolicy, error) {
	return h.service.repo.ListEnabledAlertPoliciesByClusterAndLegacyRuleKey(ctx, strconv.FormatUint(uint64(clusterID), 10), ruleKey)
}

// syncJobPolicyHasRule 判断任务策略的条件里是否勾选了这条规则。
// syncJobPolicyHasRule reports whether the task policy's conditions select the rule.
func syncJobPolicyHasRule(policy *AlertPolicy, ruleKey string) bool {
	for _, rule := range syncJobPolicyRules(policy) {
		if rule == ruleKey {
			return true
		}
	}
	return false
}

// syncJobPolicyRules 从策略条件里取出勾选的规则键。
// syncJobPolicyRules extracts the selected rule keys from the policy conditions.
func syncJobPolicyRules(policy *AlertPolicy) []string {
	if policy == nil {
		return nil
	}
	var rules []string
	for _, condition := range unmarshalAlertPolicyConditions(policy.ConditionsJSON) {
		if condition != nil && syncalert.IsTaskRule(strings.TrimSpace(condition.MetricKey)) {
			rules = append(rules, strings.TrimSpace(condition.MetricKey))
		}
	}
	return rules
}

func singlePolicy(policy *AlertPolicy) []*AlertPolicy {
	if policy == nil {
		return nil
	}
	return []*AlertPolicy{policy}
}

// resolveUserEmails 查询用户的有效邮箱；用户不存在、停用或没有邮箱时返回空，不让告警失败。
// resolveUserEmails looks up a user's valid email; a missing, inactive or email-less user yields nothing rather than
// failing the alert.
func (s *Service) resolveUserEmails(ctx context.Context, userID uint) []string {
	if s == nil || s.repo == nil || userID == 0 {
		return nil
	}
	users, err := s.repo.ListUsersByIDs(ctx, []uint64{uint64(userID)})
	if err != nil {
		return nil
	}
	var emails []string
	for _, user := range users {
		if user == nil || !user.IsActive {
			continue
		}
		email := strings.TrimSpace(user.Email)
		if _, parseErr := mail.ParseAddress(email); parseErr == nil {
			emails = append(emails, email)
		}
	}
	return emails
}

// syncAlertJobLink 返回作业深链 {console_url}/sync?task=<id>&job=<id>；没有配置控制台地址时返回空。
// 不用 external_url：它是 Agent、Alertmanager 回调的后端地址，默认安装里和前端不是同一个端口。
// syncAlertJobLink returns the job deep link {console_url}/sync?task=<id>&job=<id>; empty without a console URL.
// external_url is not used: it is the backend address agents and Alertmanager call back, and in default installs it
// is not on the same port as the frontend.
func syncAlertJobLink(taskID, jobID uint) string {
	if taskID == 0 {
		return ""
	}
	base := config.GetConsoleURL()
	if base == "" {
		return ""
	}
	query := url.Values{}
	query.Set("task", strconv.FormatUint(uint64(taskID), 10))
	if jobID > 0 {
		query.Set("job", strconv.FormatUint(uint64(jobID), 10))
	}
	return base + "/sync?" + query.Encode()
}

// syncAlertTriggerSourceLabel 把触发来源转成通知里的文字。
// syncAlertTriggerSourceLabel turns the trigger source into the text shown in notifications.
func syncAlertTriggerSourceLabel(source string) string {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "manual":
		return "手动"
	case "schedule":
		return "定时"
	case "dolphin":
		return "Dolphin 调度"
	case "":
		return ""
	default:
		return source
	}
}

// syncAlertSubject 返回阶段的主体文字：作业级为"任务 <名称>"，集群级为"集群 <名称>"。
// syncAlertSubject returns the episode subject: "任务 <name>" for job level, "集群 <name>" for cluster level.
func syncAlertSubject(episode *SyncAlertEpisode) string {
	if episode.Scope == SyncAlertEpisodeScopeTask {
		return "任务 " + firstNonEmpty(episode.TaskName, fmt.Sprintf("#%d", episode.TaskID))
	}
	return "集群 " + firstNonEmpty(episode.ClusterName, fmt.Sprintf("#%d", episode.ClusterID))
}

// buildSyncAlertTitle 返回通知标题，格式与本地进程事件一致：[STX][告警|恢复][级别] 主体 · 规则。
// buildSyncAlertTitle returns the notification title in the local process event format:
// [STX][告警|恢复][severity] subject · rule.
// B4 的标题带当前延迟，提醒再加"[提醒] 已持续 X 分钟"，见 cdcDelayTitleSuffix。
// B4 titles carry the current delay and reminders add "[提醒] 已持续 X 分钟"; see cdcDelayTitleSuffix.
func buildSyncAlertTitle(episode *SyncAlertEpisode, eventType NotificationDeliveryEventType, reminder bool, now time.Time) string {
	severity := strings.ToUpper(string(normalizeAlertPolicySeverity(episode.Severity)))
	kind := "告警"
	if eventType == NotificationDeliveryEventTypeResolved {
		kind = "恢复"
	}
	title := fmt.Sprintf("[STX][%s][%s] %s · %s", kind, resolveAlertSeverityLabelZH(severity), syncAlertSubject(episode), syncAlertRuleName(episode.RuleKey))
	if episode.RuleKey == syncalert.RuleCDCDelayHigh {
		title += cdcDelayTitleSuffix(episode, eventType, reminder, now)
	}
	return title
}

// buildSyncAlertFields 返回通知正文字段。
// buildSyncAlertFields returns the notification body fields.
func buildSyncAlertFields(episode *SyncAlertEpisode, policy *AlertPolicy, eventType NotificationDeliveryEventType, link string, now time.Time) []localAlertMessageField {
	fields := []localAlertMessageField{
		{Label: "状态", Value: resolveLocalAlertDeliveryStateLabel(eventType)},
		{Label: "规则", Value: syncAlertRuleName(episode.RuleKey)},
		{Label: "集群", Value: fmt.Sprintf("%s (%d)", firstNonEmpty(episode.ClusterName, "unknown"), episode.ClusterID)},
	}
	appendField := func(label, value string, multiline bool) {
		if value = strings.TrimSpace(value); value != "" {
			fields = append(fields, localAlertMessageField{Label: label, Value: value, Multiline: multiline})
		}
	}
	if episode.Scope == SyncAlertEpisodeScopeTask {
		appendField("任务", fmt.Sprintf("%s (#%d)", firstNonEmpty(episode.TaskName, "unknown"), episode.TaskID), false)
	}
	if episode.LastJobID > 0 {
		appendField("作业", fmt.Sprintf("#%d", episode.LastJobID), false)
	}
	appendField("引擎作业 ID", episode.LastEngineJobID, false)
	appendField("触发来源", syncAlertTriggerSourceLabel(episode.LastTriggerSource), false)
	appendField("累计次数", strconv.Itoa(episode.OccurrenceCount), false)
	appendField("首次发生", formatAlertNotificationTime(episode.FirstFiredAt), false)
	appendField("最近发生", formatAlertNotificationTime(episode.LastOccurredAt), false)
	if eventType == NotificationDeliveryEventTypeResolved && episode.ResolvedAt != nil {
		appendField("恢复时间", formatAlertNotificationTime(*episode.ResolvedAt), false)
	}
	if episode.RuleKey == syncalert.RuleCDCDelayHigh {
		// B4 不是错误：延迟数值用单独的字段展示，不放进"错误摘要"。
		// B4 is not an error: the delay values get their own fields instead of the error summary.
		for _, field := range cdcDelayFields(episode, eventType, now) {
			appendField(field.Label, field.Value, field.Multiline)
		}
	} else {
		appendField("错误摘要", syncalert.TruncateMessage(episode.LastMessage, syncAlertMessageDisplayLimit), true)
	}
	if policy != nil {
		appendField("策略", fmt.Sprintf("%s (%d)", firstNonEmpty(policy.Name, "unknown"), policy.ID), false)
	}
	appendField("链接", link, false)
	return fields
}

// buildSyncAlertMessage 把阶段组装成与来源无关的 alertMessage，五类渠道都支持。
// buildSyncAlertMessage assembles the episode into a source-agnostic alertMessage for all five channel types.
func buildSyncAlertMessage(episode *SyncAlertEpisode, policy *AlertPolicy, eventType NotificationDeliveryEventType, extraRecipients []string, now time.Time, reminder bool) *alertMessage {
	snapshot := *episode
	link := syncAlertJobLink(snapshot.TaskID, snapshot.LastJobID)
	title := buildSyncAlertTitle(&snapshot, eventType, reminder, now)
	fields := buildSyncAlertFields(&snapshot, policy, eventType, link, now)
	return &alertMessage{
		SourceType:      AlertSourceTypeSync,
		ClusterID:       strconv.FormatUint(uint64(snapshot.ClusterID), 10),
		ClusterName:     snapshot.ClusterName,
		AlertName:       strings.TrimSpace(firstNonEmpty(policy.Name, syncAlertRuleName(snapshot.RuleKey))),
		ExtraRecipients: extraRecipients,
		Build: func(channel *NotificationChannel, recipients []string) (interface{}, error) {
			if channel == nil {
				return nil, fmt.Errorf("notification channel not found")
			}
			text := renderAlertMessageText(title, fields)
			switch channel.Type {
			case NotificationChannelTypeWebhook:
				return map[string]interface{}{
					"title":   title,
					"message": text,
					"alert":   buildSyncAlertWebhookPayload(&snapshot, policy, eventType, link, reminder),
					"sent_at": now.Format(time.RFC3339),
				}, nil
			case NotificationChannelTypeWeCom, NotificationChannelTypeDingTalk:
				return map[string]interface{}{"msgtype": "text", "text": map[string]string{"content": text}}, nil
			case NotificationChannelTypeFeishu:
				return map[string]interface{}{"msg_type": "text", "content": map[string]string{"text": text}}, nil
			case NotificationChannelTypeEmail:
				severity := strings.ToUpper(string(normalizeAlertPolicySeverity(snapshot.Severity)))
				return &emailNotificationPayload{
					Subject: title,
					Text:    text,
					HTML:    renderAlertMessageHTML(title, severity, resolveLocalAlertMessageVisual(eventType), fields),
					To:      recipients,
				}, nil
			default:
				return nil, fmt.Errorf("unsupported channel type")
			}
		},
	}
}

// renderAlertMessageText 把标题和字段拼成纯文本。
// renderAlertMessageText joins the title and fields as plain text.
func renderAlertMessageText(title string, fields []localAlertMessageField) string {
	parts := []string{title}
	for _, field := range fields {
		if strings.TrimSpace(field.Value) == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s：%s", field.Label, field.Value))
	}
	return strings.Join(parts, "\n")
}

// buildSyncAlertWebhookPayload 返回 webhook 里结构化的 alert 对象。
// buildSyncAlertWebhookPayload returns the structured alert object sent to webhooks.
func buildSyncAlertWebhookPayload(episode *SyncAlertEpisode, policy *AlertPolicy, eventType NotificationDeliveryEventType, link string, reminder bool) map[string]interface{} {
	alert := map[string]interface{}{
		"source_type":      AlertSourceTypeSync,
		"source_key":       buildSyncAlertSourceKey(episode.ID),
		"status":           strings.ToLower(string(eventType)),
		"scope":            episode.Scope,
		"rule_key":         episode.RuleKey,
		"rule_name":        syncAlertRuleName(episode.RuleKey),
		"severity":         normalizeAlertPolicySeverity(episode.Severity),
		"cluster_id":       strconv.FormatUint(uint64(episode.ClusterID), 10),
		"cluster_name":     episode.ClusterName,
		"occurrence_count": episode.OccurrenceCount,
		"first_fired_at":   episode.FirstFiredAt.UTC().Format(time.RFC3339),
		"last_occurred_at": episode.LastOccurredAt.UTC().Format(time.RFC3339),
		"message":          syncalert.TruncateMessage(episode.LastMessage, syncAlertMessageDisplayLimit),
	}
	if episode.Scope == SyncAlertEpisodeScopeTask {
		alert["task_id"] = episode.TaskID
		alert["task_name"] = episode.TaskName
	}
	if episode.LastJobID > 0 {
		alert["job_id"] = episode.LastJobID
		alert["engine_job_id"] = episode.LastEngineJobID
		alert["trigger_source"] = episode.LastTriggerSource
	}
	if episode.ResolvedAt != nil {
		alert["resolved_at"] = episode.ResolvedAt.UTC().Format(time.RFC3339)
	}
	if link != "" {
		alert["link"] = link
	}
	if policy != nil {
		alert["policy_id"] = policy.ID
		alert["policy_name"] = policy.Name
		alert["policy_type"] = policy.PolicyType
	}
	if episode.ResolveReason != "" {
		alert["resolve_reason"] = episode.ResolveReason
	}
	if reminder {
		alert["reminder"] = true
	}
	if episode.RuleKey == syncalert.RuleCDCDelayHigh {
		appendCDCDelayWebhookFields(alert, episode)
	}
	return alert
}
