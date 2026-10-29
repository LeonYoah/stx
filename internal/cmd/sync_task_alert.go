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

package cmd

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	clioutput "github.com/LeonYoah/stx/internal/cli/output"
	"github.com/spf13/cobra"
)

const (
	syncTaskAlertSetOperationID = "sync.task.alert.set"
	syncTaskAlertSetImpact      = "修改该任务的作业告警配置，保存后立即生效（不需要重新发布）。"
)

// syncTaskAlertConfig 是 GET /api/v1/sync/tasks/:id/alert 返回的配置，用于 --disable 时保留其余字段。
// syncTaskAlertConfig is the config returned by GET /api/v1/sync/tasks/:id/alert, used by --disable to keep the
// other fields.
type syncTaskAlertConfig struct {
	Configured      bool     `json:"configured"`
	Rules           []string `json:"rules"`
	ChannelIDs      []uint   `json:"channel_ids"`
	ReceiverUserIDs []uint   `json:"receiver_user_ids"`
	NotifyTaskOwner bool     `json:"notify_task_owner"`
	CooldownMinutes int      `json:"cooldown_minutes"`
	SendRecovery    bool     `json:"send_recovery"`
	// B4 的阈值（秒）与持续时间（分钟）。 / B4 threshold (seconds) and duration (minutes).
	CDCDelayThresholdSeconds int `json:"cdc_delay_threshold_seconds"`
	CDCDelayDurationMinutes  int `json:"cdc_delay_duration_minutes"`
}

const (
	syncTaskAlertRuleCDCDelay = "sync_cdc_delay_high"
	syncTaskAlertRulesHelp    = "sync_submit_failed, sync_submit_timeout, sync_job_failed, sync_stop_timeout, sync_cdc_delay_high"
)

// newSyncTaskAlertSetCommand 新建或修改任务的作业告警配置（R1）。CLI 不提供删除：停用用 --disable，删除只在网页和 API 提供。
// newSyncTaskAlertSetCommand creates or updates a task's job alert config (R1). The CLI offers no delete: disable
// with --disable; deleting is only available in the web UI and API.
func newSyncTaskAlertSetCommand(storeProvider authStoreProvider) *cobra.Command {
	var options secureWriteOptions
	var rules []string
	var channelIDs []uint
	var receiverIDs []uint
	var notifyOwner, noRecovery, disable bool
	var cooldown, cdcThreshold, cdcDuration int

	command := &cobra.Command{
		Use:   "set <task_id>",
		Short: "Create or update the job alert configuration of a sync task",
		Long: "Create or update the job alert configuration of a sync task. It takes effect immediately and is not\n" +
			"versioned with the task. Rules: " + syncTaskAlertRulesHelp + ".\n" +
			"sync_cdc_delay_high (B4) alerts when a CDC streaming job's read delay stays above --cdc-delay-threshold seconds\n" +
			"for --cdc-delay-duration minutes; it needs a SeaTunnel 2.3.12+ cluster with the event channel enabled. Without the two flags the\n" +
			"current values are kept (defaults 60 s / 5 min).\n" +
			"Use --disable to stop alerting while keeping the configuration; deleting is only offered in the web UI and API.",
		Example: "stx sync task alert set 1 --rules sync_job_failed,sync_submit_timeout --channel 2 --confirm\n" +
			"stx sync task alert set 1 --rules sync_job_failed,sync_cdc_delay_high --cdc-delay-threshold 120 --cdc-delay-duration 10 --channel 2 --confirm\n" +
			"stx sync task alert set 1 --disable --confirm",
		Args: usageArgs(cobra.ExactArgs(1)),
		RunE: func(command *cobra.Command, args []string) error {
			taskID, err := strconv.ParseUint(strings.TrimSpace(args[0]), 10, 64)
			if err != nil || taskID == 0 {
				return clioutput.NewError(clioutput.CodeUsage, "task_id must be a positive integer", clioutput.ExitUsage, false)
			}
			ruleList := splitAlertRules(rules)
			keepCurrent := disable && len(ruleList) == 0 && len(channelIDs) == 0
			if !keepCurrent && len(ruleList) == 0 {
				return clioutput.NewError(clioutput.CodeUsage, "--rules is required (or use --disable alone to disable the current config)", clioutput.ExitUsage, false)
			}
			if !keepCurrent && len(channelIDs) == 0 {
				return clioutput.NewError(clioutput.CodeUsage, "--channel is required", clioutput.ExitUsage, false)
			}
			if command.Flags().Changed("cooldown") && (cooldown < 0 || cooldown > 1440) {
				return clioutput.NewError(clioutput.CodeUsage, "--cooldown must be between 0 and 1440 minutes", clioutput.ExitUsage, false)
			}
			thresholdSet := command.Flags().Changed("cdc-delay-threshold")
			durationSet := command.Flags().Changed("cdc-delay-duration")
			// 0 表示默认值，与 API 一致。 / 0 means the default, as in the API.
			if thresholdSet && cdcThreshold != 0 && (cdcThreshold < 10 || cdcThreshold > 86400) {
				return clioutput.NewError(clioutput.CodeUsage, "--cdc-delay-threshold must be between 10 and 86400 seconds (0 = default 60)", clioutput.ExitUsage, false)
			}
			if durationSet && cdcDuration != 0 && (cdcDuration < 1 || cdcDuration > 60) {
				return clioutput.NewError(clioutput.CodeUsage, "--cdc-delay-duration must be between 1 and 60 minutes (0 = default 5)", clioutput.ExitUsage, false)
			}
			client, headers, err := prepareSecureWrite(command, storeProvider, syncTaskAlertSetOperationID, &options, syncTaskAlertSetImpact)
			if err != nil {
				return err
			}
			path := fmt.Sprintf("/api/v1/sync/tasks/%d/alert", taskID)
			body := map[string]any{
				"enabled":           !disable,
				"rules":             ruleList,
				"channel_ids":       channelIDs,
				"receiver_user_ids": receiverIDs,
				"notify_task_owner": notifyOwner,
				"send_recovery":     !noRecovery,
			}
			if command.Flags().Changed("cooldown") {
				body["cooldown_minutes"] = cooldown
			}
			if thresholdSet {
				body["cdc_delay_threshold_seconds"] = cdcThreshold
			}
			if durationSet {
				body["cdc_delay_duration_minutes"] = cdcDuration
			}
			if !keepCurrent && containsString(ruleList, syncTaskAlertRuleCDCDelay) && (!thresholdSet || !durationSet) {
				// 勾选了 B4 但没给参数：读出现有配置保留原值，没有配置时由服务端补默认值。
				// B4 selected without its flags: keep the current values; without a config the server fills defaults.
				var current syncTaskAlertConfig
				if _, err := client.Request(command.Context(), http.MethodGet, path, nil, &current); err != nil {
					return err
				}
				if current.Configured {
					if !thresholdSet {
						body["cdc_delay_threshold_seconds"] = current.CDCDelayThresholdSeconds
					}
					if !durationSet {
						body["cdc_delay_duration_minutes"] = current.CDCDelayDurationMinutes
					}
				}
			}
			if keepCurrent {
				// 只停用：读出现有配置原样写回，只把 enabled 改成 false。
				// Disable only: read the current config and write it back with enabled=false.
				var current syncTaskAlertConfig
				if _, err := client.Request(command.Context(), http.MethodGet, path, nil, &current); err != nil {
					return err
				}
				if !current.Configured {
					return clioutput.NewError(clioutput.CodeUsage, "the task has no alert configuration to disable", clioutput.ExitUsage, false)
				}
				body = map[string]any{
					"enabled":                     false,
					"rules":                       current.Rules,
					"channel_ids":                 current.ChannelIDs,
					"receiver_user_ids":           current.ReceiverUserIDs,
					"notify_task_owner":           current.NotifyTaskOwner,
					"cooldown_minutes":            current.CooldownMinutes,
					"send_recovery":               current.SendRecovery,
					"cdc_delay_threshold_seconds": current.CDCDelayThresholdSeconds,
					"cdc_delay_duration_minutes":  current.CDCDelayDurationMinutes,
				}
			}
			var data any
			requestID, err := client.RequestWithHeaders(command.Context(), http.MethodPut, path, body, headers, &data)
			if err != nil {
				return handleSecureWriteError(command, syncTaskAlertSetOperationID, err)
			}
			return renderCommandResultWithRequestID(command, syncTaskAlertSetOperationID, requestID, data)
		},
	}
	addSecureWriteFlags(command, &options)
	command.Flags().StringSliceVar(&rules, "rules", nil, "Rules to alert on, comma-separated or repeated: "+syncTaskAlertRulesHelp)
	command.Flags().UintSliceVar(&channelIDs, "channel", nil, "Notification channel ID (repeatable)")
	command.Flags().UintSliceVar(&receiverIDs, "receiver", nil, "Receiver user ID for email channels (repeatable)")
	command.Flags().BoolVar(&notifyOwner, "notify-owner", false, "Also email the task creator (email channels only)")
	command.Flags().IntVar(&cooldown, "cooldown", 60, "Minutes between reminders while the problem persists; 0 disables reminders")
	command.Flags().BoolVar(&noRecovery, "no-recovery", false, "Do not send a recovery notification")
	command.Flags().BoolVar(&disable, "disable", false, "Disable alerting; alone it keeps the current rules and channels")
	command.Flags().IntVar(&cdcThreshold, "cdc-delay-threshold", 0, "sync_cdc_delay_high: read delay threshold in seconds, 10-86400 (0 = default 60; omitted keeps the current value)")
	command.Flags().IntVar(&cdcDuration, "cdc-delay-duration", 0, "sync_cdc_delay_high: minutes the delay must stay above the threshold, 1-60 (0 = default 5; omitted keeps the current value)")
	return command
}

// splitAlertRules 拆分逗号分隔或重复传入的规则，去掉空白和重复。
// splitAlertRules splits comma-separated or repeated rules and drops blanks and duplicates.
func splitAlertRules(values []string) []string {
	seen := map[string]bool{}
	var rules []string
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			part = strings.TrimSpace(part)
			if part == "" || seen[part] {
				continue
			}
			seen[part] = true
			rules = append(rules, part)
		}
	}
	return rules
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
