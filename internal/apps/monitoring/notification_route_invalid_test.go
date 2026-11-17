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
	"testing"

	"github.com/stretchr/testify/require"
)

// TestNotificationRouteInvalidReason 覆盖失效判定：旧告警类别全部失效、
// 新告警类别与空值（匹配任意 metric 告警）有效。
// TestNotificationRouteInvalidReason covers the invalid judgment: every legacy alert
// category is invalid, while the new categories and empty (match-any metric) stay valid.
func TestNotificationRouteInvalidReason(t *testing.T) {
	for _, legacy := range []string{"local_process_event", "remote_alertmanager", "sync", "custom_promql"} {
		reason := NotificationRouteInvalidReason(legacy)
		require.Equal(t, NotificationRouteInvalidReasonLegacySourceType, reason, "expected %q to be invalid", legacy)
	}
	for _, valid := range []string{"", "platform_health", "metric", "job"} {
		require.Empty(t, NotificationRouteInvalidReason(valid), "expected %q to be valid", valid)
	}
}

// TestToNotificationRouteDTOInvalidReason 验证 DTO 携带失效原因。
// TestToNotificationRouteDTOInvalidReason verifies the DTO carries the invalid reason.
func TestToNotificationRouteDTOInvalidReason(t *testing.T) {
	legacy := &NotificationRoute{ID: 7, Name: "legacy", Enabled: true, SourceType: "remote_alertmanager", ChannelID: 1}
	dto := toNotificationRouteDTO(legacy)
	require.Equal(t, "remote_alertmanager", dto.SourceType)
	require.Equal(t, NotificationRouteInvalidReasonLegacySourceType, dto.InvalidReason)
	require.Equal(t, "remote_alertmanager", dto.InvalidDetail)

	current := &NotificationRoute{ID: 8, Name: "current", Enabled: true, SourceType: "metric", ChannelID: 1}
	dto = toNotificationRouteDTO(current)
	require.Empty(t, dto.InvalidReason)
	require.Empty(t, dto.InvalidDetail)
}
