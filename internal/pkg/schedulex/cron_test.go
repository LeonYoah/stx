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

package schedulex

import (
	"testing"
	"time"

	"github.com/LeonYoah/stx/internal/pkg/timex"
)

func TestMatchMinuteWindowUsesTaskTimezone(t *testing.T) {
	now := time.Date(2026, 10, 14, 7, 30, 20, 0, time.UTC) // 15:30 Asia/Shanghai, 03:30 America/New_York
	cases := []struct {
		expr, tz string
		want     bool
	}{
		{"30 15 * * *", "Asia/Shanghai", true},
		{"30 15 * * *", "UTC", false},
		{"30 3 * * *", "America/New_York", true},
		{"30 7 * * *", "UTC", true},
	}
	for _, c := range cases {
		matched, start, end, err := MatchMinuteWindow(c.expr, now, c.tz)
		if err != nil || matched != c.want {
			t.Fatalf("%s@%s: matched=%v err=%v", c.expr, c.tz, matched, err)
		}
		if matched && (!start.Equal(time.Date(2026, 10, 14, 7, 30, 0, 0, time.UTC)) || end.Sub(start) != time.Minute) {
			t.Fatalf("%s@%s: bad window %v-%v", c.expr, c.tz, start, end)
		}
	}
}

func TestNormalizeTimezoneFallsBackToDisplayTimezone(t *testing.T) {
	if err := timex.SetDisplayTimezone("Europe/Berlin"); err != nil {
		t.Fatal(err)
	}
	defer timex.SetDisplayTimezone(timex.DefaultDisplayTimezone)
	if got := NormalizeTimezone(" "); got != "Europe/Berlin" {
		t.Fatalf("got %s", got)
	}
	if got := NormalizeTimezone("UTC"); got != "UTC" {
		t.Fatalf("got %s", got)
	}
}
