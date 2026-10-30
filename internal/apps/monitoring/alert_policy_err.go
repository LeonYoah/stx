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

import "errors"

var (
	// ErrAlertPolicyNotFound indicates the requested alert policy does not exist.
	// ErrAlertPolicyNotFound 表示请求的告警策略不存在。
	ErrAlertPolicyNotFound = errors.New("monitoring: alert policy not found")
	// ErrAlertPolicyInvalidID indicates the alert policy ID is invalid.
	// ErrAlertPolicyInvalidID 表示告警策略 ID 非法。
	ErrAlertPolicyInvalidID = errors.New("monitoring: invalid alert policy id")
	// ErrAlertPolicyInvalidClusterScope indicates the cluster scope is invalid.
	// ErrAlertPolicyInvalidClusterScope 表示策略集群作用域非法。
	ErrAlertPolicyInvalidClusterScope = errors.New("monitoring: invalid alert policy cluster scope")
	// ErrAlertPolicyClusterNotFound indicates the referenced cluster does not exist.
	// ErrAlertPolicyClusterNotFound 表示引用的集群不存在。
	ErrAlertPolicyClusterNotFound = errors.New("monitoring: alert policy cluster not found")
	// ErrAlertPolicyNotificationChannelNotFound indicates a referenced channel does not exist.
	// ErrAlertPolicyNotificationChannelNotFound 表示引用的通知渠道不存在。
	ErrAlertPolicyNotificationChannelNotFound = errors.New("monitoring: alert policy notification channel not found")
	// ErrAlertPolicyReceiverUserNotFound indicates a referenced receiver user does not exist.
	// ErrAlertPolicyReceiverUserNotFound 表示引用的通知接收用户不存在。
	ErrAlertPolicyReceiverUserNotFound = errors.New("monitoring: alert policy receiver user not found")
	// ErrAlertPolicyReceiverUserInactive indicates a referenced receiver user is inactive.
	// ErrAlertPolicyReceiverUserInactive 表示引用的通知接收用户已被禁用。
	ErrAlertPolicyReceiverUserInactive = errors.New("monitoring: alert policy receiver user inactive")
	// ErrAlertPolicyReceiverUserEmailMissing indicates a referenced receiver user has no valid email.
	// ErrAlertPolicyReceiverUserEmailMissing 表示引用的通知接收用户未配置有效邮箱。
	ErrAlertPolicyReceiverUserEmailMissing = errors.New("monitoring: alert policy receiver user email missing")
	// ErrAlertPolicyUnsupportedTemplate indicates the template key is unknown.
	// ErrAlertPolicyUnsupportedTemplate 表示模板键不存在。
	ErrAlertPolicyUnsupportedTemplate = errors.New("monitoring: unsupported alert policy template")
	// ErrAlertPolicyTemplateTypeMismatch indicates the template does not match the policy type.
	// ErrAlertPolicyTemplateTypeMismatch 表示模板与策略类型不匹配。
	ErrAlertPolicyTemplateTypeMismatch = errors.New("monitoring: alert policy template type mismatch")
	// ErrAlertPolicyLegacyBridgeRequiresClusterScope indicates a legacy-backed policy must target one cluster.
	// ErrAlertPolicyLegacyBridgeRequiresClusterScope 表示旧规则桥接策略必须绑定具体集群。
	ErrAlertPolicyLegacyBridgeRequiresClusterScope = errors.New("monitoring: legacy-backed policy requires concrete cluster scope")
	// ErrAlertPolicyLegacyRuleUnsupported indicates the referenced legacy rule bridge target is unsupported.
	// ErrAlertPolicyLegacyRuleUnsupported 表示引用的旧规则桥接目标不受支持。
	ErrAlertPolicyLegacyRuleUnsupported = errors.New("monitoring: unsupported legacy rule bridge")
	// ErrAlertPolicyLegacyBridgeConflict indicates another policy already bridges the same legacy rule.
	// ErrAlertPolicyLegacyBridgeConflict 表示已有其他策略桥接到同一条旧规则。
	ErrAlertPolicyLegacyBridgeConflict = errors.New("monitoring: duplicate legacy rule bridge policy")
)

// 作业告警（sync_job）策略的错误。
// Errors of job alert (sync_job) policies.
var (
	// ErrAlertPolicySyncJobConflict 表示该任务已经有一条作业告警策略。
	// ErrAlertPolicySyncJobConflict means the task already has a job alert policy.
	ErrAlertPolicySyncJobConflict = errors.New("monitoring: the sync task already has a job alert policy")
	// ErrAlertPolicySyncJobTaskRequired 表示作业告警策略缺少 task_id。
	// ErrAlertPolicySyncJobTaskRequired means a job alert policy is missing task_id.
	ErrAlertPolicySyncJobTaskRequired = errors.New("monitoring: task_id is required for sync_job policies")
	// ErrAlertPolicySyncJobInvalidRule 表示规则为空或不是作业级规则。
	// ErrAlertPolicySyncJobInvalidRule means the rules are empty or not job-level rules.
	ErrAlertPolicySyncJobInvalidRule = errors.New("monitoring: sync_job policies need at least one job rule (sync_submit_failed, sync_submit_timeout, sync_job_failed, sync_stop_timeout, sync_cdc_delay_high)")
	// ErrAlertPolicySyncJobInvalidCDCDelay 表示 B4 条件的阈值或持续时间超出范围。
	// ErrAlertPolicySyncJobInvalidCDCDelay means the B4 condition threshold or duration is out of range.
	ErrAlertPolicySyncJobInvalidCDCDelay = errors.New("monitoring: sync_cdc_delay_high needs threshold 10-86400 seconds and window_minutes 1-60 (0 or empty means the default 60 s / 5 min)")
	// ErrAlertPolicySyncJobChannelRequired 表示作业告警策略没有选择通知渠道。
	// ErrAlertPolicySyncJobChannelRequired means a job alert policy selects no notification channel.
	ErrAlertPolicySyncJobChannelRequired = errors.New("monitoring: sync_job policies need at least one notification channel")
	// ErrAlertPolicySyncJobChannelDisabled 表示选择的通知渠道已停用。
	// ErrAlertPolicySyncJobChannelDisabled means a selected notification channel is disabled.
	ErrAlertPolicySyncJobChannelDisabled = errors.New("monitoring: selected notification channel is disabled")
	// ErrAlertPolicySyncJobEmailRecipientMissing 表示选了邮件渠道却没有任何收件人。
	// ErrAlertPolicySyncJobEmailRecipientMissing means an email channel is selected but nobody would receive it.
	ErrAlertPolicySyncJobEmailRecipientMissing = errors.New("monitoring: email channel selected but no recipient: pick receivers, notify the task owner, or configure recipients on the channel")
	// ErrAlertPolicySyncJobTypeChange 表示不能在 sync_job 与其他策略类型之间转换。
	// ErrAlertPolicySyncJobTypeChange means a policy cannot switch between sync_job and other types.
	ErrAlertPolicySyncJobTypeChange = errors.New("monitoring: cannot change a policy to or from sync_job")
)
