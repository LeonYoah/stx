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
	"strings"
	"testing"
	"time"

	"github.com/LeonYoah/stx/internal/pkg/timex"
)

func TestFormatAlertNotificationTimeFollowsDisplayTimezone(t *testing.T) {
	defer timex.SetDisplayTimezone(timex.DefaultDisplayTimezone)
	at := time.Date(2026, 10, 14, 7, 30, 0, 0, time.UTC)
	for tz, want := range map[string]string{
		"Asia/Shanghai":    "2026-10-14 15:30:00 +08:00 (Asia/Shanghai)",
		"America/New_York": "2026-10-14 03:30:00 -04:00 (America/New_York)",
		"UTC":              "2026-10-14 07:30:00 +00:00 (UTC)",
	} {
		if err := timex.SetDisplayTimezone(tz); err != nil {
			t.Fatal(err)
		}
		got := formatAlertNotificationTime(at.In(time.Local))
		if !strings.Contains(got, want) || !strings.Contains(got, "2026-10-14T07:30:00Z (UTC)") {
			t.Fatalf("%s: got %q", tz, got)
		}
	}
}
