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

// Package syncalert 定义数据同步作业告警在 sync（产生信号）与 monitoring（判定、投递）之间的契约，两边都只依赖本包。
// Package syncalert defines the contract for sync job alerts between sync (which produces signals) and monitoring
// (which evaluates and delivers them); both sides depend only on this package.
package syncalert

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// 作业级规则：用户在任务上配置了才生效。
// Job-level rules: they only apply when the user configures alerts on the task.
const (
	// RuleSubmitFailed 是提交到引擎被明确拒绝，或者找不到可用的引擎端点（A1）。
	// RuleSubmitFailed fires when the engine definitely rejects a submit or no engine endpoint is usable (A1).
	RuleSubmitFailed = "sync_submit_failed"
	// RuleSubmitTimeout 是提交结果一直未知，过了截止时间被判失败（A2）。
	// RuleSubmitTimeout fires when an unknown submit outcome passes its deadline and is failed (A2).
	RuleSubmitTimeout = "sync_submit_timeout"
	// RuleJobFailed 是作业转为失败，包括引擎已无此作业（B1）。
	// RuleJobFailed fires when a job turns failed, including when the engine no longer has it (B1).
	RuleJobFailed = "sync_job_failed"
	// RuleStopTimeout 是停止请求过了截止时间作业仍未停下（C1）。
	// RuleStopTimeout fires when a job is still running after its stop deadline (C1).
	RuleStopTimeout = "sync_stop_timeout"
)

// 集群级规则：每个集群默认开启，与"节点离线"一样作为默认规则。
// Cluster-level rules: enabled by default on every cluster, like "node offline".
const (
	// RuleStatusUnavailable 是集群的作业状态持续获取不到（D1）。
	// RuleStatusUnavailable fires when job status of a cluster keeps being unavailable (D1).
	RuleStatusUnavailable = "sync_status_unavailable"
	// RuleEventChannelDegraded 是集群事件通道从健康变为降级（D2）。
	// RuleEventChannelDegraded fires when the cluster event channel turns from healthy to degraded (D2).
	RuleEventChannelDegraded = "sync_event_channel_degraded"
)

// TaskRules 返回任务上可以勾选的规则，顺序固定。
// TaskRules returns the rules a task can select, in a fixed order.
func TaskRules() []string {
	return []string{RuleSubmitFailed, RuleSubmitTimeout, RuleJobFailed, RuleStopTimeout}
}

// ClusterRules 返回集群级默认规则。
// ClusterRules returns the cluster-level default rules.
func ClusterRules() []string {
	return []string{RuleStatusUnavailable, RuleEventChannelDegraded}
}

// IsTaskRule 判断规则键是否属于作业级规则。
// IsTaskRule reports whether the rule key is a job-level rule.
func IsTaskRule(rule string) bool {
	for _, candidate := range TaskRules() {
		if candidate == rule {
			return true
		}
	}
	return false
}

// IsClusterRule 判断规则键是否属于集群级规则。
// IsClusterRule reports whether the rule key is a cluster-level rule.
func IsClusterRule(rule string) bool {
	for _, candidate := range ClusterRules() {
		if candidate == rule {
			return true
		}
	}
	return false
}

// SignalKind 是信号类型。
// SignalKind is the kind of a signal.
type SignalKind string

const (
	// KindFire 表示发生一次（作业级）或状态成立（集群级 D2）。
	// KindFire means one occurrence (job level) or a condition that now holds (cluster level D2).
	KindFire SignalKind = "fire"
	// KindResolve 表示按 ResolveScope 解除。
	// KindResolve clears episodes according to ResolveScope.
	KindResolve SignalKind = "resolve"
	// KindObserve 表示 D1 的持续失败观测，带 Since；由 monitoring 按规则窗口判断是否成熟。
	// KindObserve is a D1 sustained-failure observation carrying Since; monitoring decides maturity by the rule window.
	KindObserve SignalKind = "observe"
)

// ResolveScope 指定 Resolve 关闭哪些告警阶段。
// ResolveScope selects which alert episodes a Resolve closes.
type ResolveScope string

const (
	// ResolveTaskRunHealthy 关闭任务的 A1、A2、B1：批作业运行成功，或实时作业进入运行中。
	// ResolveTaskRunHealthy closes the task's A1, A2 and B1: a batch job succeeded or a streaming job is running.
	ResolveTaskRunHealthy ResolveScope = "task_run_healthy"
	// ResolveJobTerminal 关闭这个作业的 C1：作业最终停下。
	// ResolveJobTerminal closes this job's C1: the job finally stopped.
	ResolveJobTerminal ResolveScope = "job_terminal"
	// ResolveClusterRule 关闭集群这条规则的阶段。
	// ResolveClusterRule closes the cluster's episode for this rule.
	ResolveClusterRule ResolveScope = "cluster_rule"
)

// MaxMessageLength 是信号里错误信息的最大长度（字符）。
// MaxMessageLength is the maximum length (in characters) of a signal's message.
const MaxMessageLength = 2000

// Signal 是 sync 发给告警模块的一条信号；所有字段都是快照，告警模块不回查 sync。
// Signal is one signal sync sends to the alert module; every field is a snapshot and monitoring never calls back.
type Signal struct {
	Kind          SignalKind
	RuleKey       string
	ResolveScope  ResolveScope
	ClusterID     uint
	ClusterName   string
	TaskID        uint
	TaskName      string
	TaskCreatorID uint
	JobID         uint
	EngineJobID   string
	// TriggerSource 是作业的触发来源：manual、schedule、dolphin、cli 等。
	// TriggerSource is how the job was started: manual, schedule, dolphin, cli and so on.
	TriggerSource string
	RunType       string
	// Message 已脱敏，最长 MaxMessageLength 个字符。
	// Message is already redacted and at most MaxMessageLength characters.
	Message string
	// Since 只用于 Observe：这一段连续失败从何时开始。
	// Since is only used by Observe: when the current run of failures started.
	Since      time.Time
	OccurredAt time.Time
}

// Reporter 接收 sync 产生的告警信号；实现必须非阻塞。
// Reporter receives alert signals produced by sync; implementations must not block.
type Reporter interface {
	ReportSyncAlert(signal Signal)
}

// TruncateMessage 把错误信息截到 limit 个字符，超出部分用省略号表示。
// TruncateMessage cuts a message to limit characters and marks the cut with an ellipsis.
func TruncateMessage(message string, limit int) string {
	message = strings.TrimSpace(message)
	runes := []rune(message)
	if limit <= 0 || len(runes) <= limit {
		return message
	}
	return string(runes[:limit]) + "…"
}

// 默认值与范围。
// Defaults and ranges.
const (
	// DefaultCooldownMinutes 是未填写时的提醒间隔。
	// DefaultCooldownMinutes is the reminder interval when none is given.
	DefaultCooldownMinutes = 60
	// MaxCooldownMinutes 是提醒间隔上限（一天）。
	// MaxCooldownMinutes caps the reminder interval (one day).
	MaxCooldownMinutes = 1440
)

// TaskConfig 是任务告警配置，也是 GET/PUT /api/v1/sync/tasks/:id/alert 的数据结构。
// TaskConfig is a task's alert configuration and the payload of GET/PUT /api/v1/sync/tasks/:id/alert.
type TaskConfig struct {
	Configured      bool     `json:"configured"`
	PolicyID        uint     `json:"policy_id,omitempty"`
	Enabled         bool     `json:"enabled"`
	Rules           []string `json:"rules"`
	ChannelIDs      []uint   `json:"channel_ids"`
	ReceiverUserIDs []uint   `json:"receiver_user_ids"`
	NotifyTaskOwner bool     `json:"notify_task_owner"`
	CooldownMinutes int      `json:"cooldown_minutes"`
	SendRecovery    bool     `json:"send_recovery"`
}

// TaskRef 是告警模块保存任务策略时需要的任务快照。
// TaskRef is the task snapshot the alert module needs to store a task policy.
type TaskRef struct {
	ID        uint
	Name      string
	ClusterID uint
	CreatorID uint
}

// TaskPolicyStore 读写以任务为范围的作业告警策略，由告警模块实现。
// TaskPolicyStore reads and writes task-scoped job alert policies; the alert module implements it.
type TaskPolicyStore interface {
	GetTaskAlertConfig(ctx context.Context, taskID uint) (*TaskConfig, error)
	SaveTaskAlertConfig(ctx context.Context, task TaskRef, cfg *TaskConfig) (*TaskConfig, error)
	DeleteTaskAlertConfig(ctx context.Context, taskID uint) error
}

// ErrInvalidConfig 表示任务告警配置不合法（HTTP 400）。
// ErrInvalidConfig means the task alert configuration is invalid (HTTP 400).
var ErrInvalidConfig = errors.New("invalid task alert config")

// ErrConflict 表示同一个任务已经有一条作业告警策略（HTTP 409）。
// ErrConflict means the task already has a job alert policy (HTTP 409).
var ErrConflict = errors.New("task already has a job alert policy")

// InvalidConfig 包装一条具体的校验错误，保留 ErrInvalidConfig 以便映射为 400。
// InvalidConfig wraps one concrete validation error while keeping ErrInvalidConfig for the 400 mapping.
func InvalidConfig(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalidConfig, fmt.Sprintf(format, args...))
}

// Normalize 去重、排序规则与 ID，并补齐默认值；不做校验。
// Normalize de-duplicates and sorts rules and IDs and fills defaults; it does not validate.
func (c *TaskConfig) Normalize() *TaskConfig {
	if c == nil {
		return nil
	}
	normalized := *c
	order := map[string]int{}
	for i, rule := range TaskRules() {
		order[rule] = i
	}
	seen := map[string]bool{}
	rules := make([]string, 0, len(c.Rules))
	for _, rule := range c.Rules {
		rule = strings.TrimSpace(rule)
		if rule == "" || seen[rule] {
			continue
		}
		seen[rule] = true
		rules = append(rules, rule)
	}
	sort.SliceStable(rules, func(i, j int) bool {
		left, leftOK := order[rules[i]]
		right, rightOK := order[rules[j]]
		if leftOK && rightOK {
			return left < right
		}
		return leftOK && !rightOK
	})
	normalized.Rules = rules
	normalized.ChannelIDs = uniqueIDs(c.ChannelIDs)
	normalized.ReceiverUserIDs = uniqueIDs(c.ReceiverUserIDs)
	return &normalized
}

// Validate 校验规则和数值范围；渠道和用户是否存在由告警模块校验。
// Validate checks rules and value ranges; whether channels and users exist is checked by the alert module.
func (c *TaskConfig) Validate() error {
	if c == nil {
		return InvalidConfig("config is required")
	}
	if len(c.Rules) == 0 {
		return InvalidConfig("select at least one rule")
	}
	for _, rule := range c.Rules {
		if !IsTaskRule(rule) {
			return InvalidConfig("unknown rule %q", rule)
		}
	}
	if len(c.ChannelIDs) == 0 {
		return InvalidConfig("select at least one notification channel")
	}
	for _, id := range c.ChannelIDs {
		if id == 0 {
			return InvalidConfig("channel_ids must not contain zero")
		}
	}
	for _, id := range c.ReceiverUserIDs {
		if id == 0 {
			return InvalidConfig("receiver_user_ids must not contain zero")
		}
	}
	if c.CooldownMinutes < 0 || c.CooldownMinutes > MaxCooldownMinutes {
		return InvalidConfig("cooldown_minutes must be between 0 and %d", MaxCooldownMinutes)
	}
	return nil
}

// uniqueIDs 去掉重复 ID，保留顺序；0 原样保留，交给 Validate 报错。
// uniqueIDs drops duplicate IDs in order; zeros are kept so Validate can reject them.
func uniqueIDs(values []uint) []uint {
	result := make([]uint, 0, len(values))
	seen := map[uint]bool{}
	for _, value := range values {
		if value != 0 && seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}
