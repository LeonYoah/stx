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
	"sync"
	"time"
)

// notificationChannelHealth 在进程内记录每个渠道最近一次投递结果，供同步告警重试排序和"渠道恢复后立即补发"使用：
//   - 最近一次失败的渠道视为"可疑"，重试时排在健康渠道之后，可疑渠道之间按最近失败时间从早到晚轮转，卡住的渠道
//     不会一直占着巡检预算；
//   - 渠道失败后又有一次发送成功，或被重新保存 / 启用（UpdateNotificationChannel），就标记为"已恢复"，它在退避中的
//     首次告警 / 恢复投递下一轮巡检立即重试，不用等退避（最长 30 分钟）。
//
// 只保存在内存里：进程重启后没有历史，所有渠道按健康处理、照常按退避重试。
//
// notificationChannelHealth keeps, in process, each channel's latest delivery outcome for ordering sync alert retries
// and "retry right away once the channel recovers":
//   - a channel whose latest send failed is "suspect": retries put it after healthy channels and rotate suspects by
//     least recent failure, so a hung channel cannot keep eating the sweep budget;
//   - once a send to a failed channel succeeds again, or the channel is saved / re-enabled (UpdateNotificationChannel),
//     it is "recovered": its first alert / recovery deliveries still in backoff are retried on the next sweep instead
//     of waiting out the backoff (up to 30 minutes).
//
// In memory only: after a restart there is no history, every channel counts as healthy and the backoff applies as usual.
type notificationChannelHealth struct {
	mu       sync.Mutex
	channels map[uint]*notificationChannelState
}

type notificationChannelState struct {
	// lastFailureAt 是最近一次失败的时间；healthy 为 true 时表示失败之后已经成功或被重新配置过。
	// lastFailureAt is the latest failure; healthy means the channel succeeded or was reconfigured since then.
	lastFailureAt time.Time
	healthy       bool
	// recovered 表示渠道在失败之后又成功或被重新配置过（之前从未失败的渠道不算）。
	// recovered means the channel succeeded or was reconfigured after a failure (a channel that never failed is not).
	recovered bool
}

func (h *notificationChannelHealth) state(channelID uint) *notificationChannelState {
	if h.channels == nil {
		h.channels = map[uint]*notificationChannelState{}
	}
	state := h.channels[channelID]
	if state == nil {
		state = &notificationChannelState{healthy: true}
		h.channels[channelID] = state
	}
	return state
}

func (h *notificationChannelHealth) recordSuccess(channelID uint) {
	h.mu.Lock()
	defer h.mu.Unlock()
	state := h.state(channelID)
	if !state.healthy {
		state.recovered = true
	}
	state.healthy = true
}

func (h *notificationChannelHealth) recordFailure(channelID uint, at time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	state := h.state(channelID)
	state.healthy = false
	state.recovered = false
	state.lastFailureAt = at
}

// recordChanged 在渠道被重新保存（改地址、重新启用等）时调用：之前失败的投递值得立即再试一次。
// recordChanged is called when the channel is saved again (new endpoint, re-enabled, ...): earlier failed deliveries
// deserve an immediate retry.
func (h *notificationChannelHealth) recordChanged(channelID uint) {
	h.mu.Lock()
	defer h.mu.Unlock()
	state := h.state(channelID)
	state.healthy = true
	state.recovered = true
}

// snapshot 返回渠道当前是否可疑、最近失败时间，以及是否在失败后恢复过。
// snapshot returns whether the channel is suspect, its latest failure and whether it recovered after a failure.
func (h *notificationChannelHealth) snapshot(channelID uint) (suspect bool, lastFailureAt time.Time, recovered bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	state := h.channels[channelID]
	if state == nil {
		return false, time.Time{}, false
	}
	return !state.healthy, state.lastFailureAt, state.recovered
}

// consumeRecovered 在渠道的退避记录已被提前重试后清掉"已恢复"标记，避免每轮都绕过退避。
// consumeRecovered clears the recovered mark once the channel's backed-off rows were retried early, so later sweeps
// honour the backoff again.
func (h *notificationChannelHealth) consumeRecovered(channelID uint) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if state := h.channels[channelID]; state != nil {
		state.recovered = false
	}
}
