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
	"fmt"
	"strings"
	"time"

	"github.com/LeonYoah/stx/internal/pkg/timex"
	"github.com/robfig/cron/v3"
)

// DefaultTimezone 保留以兼容旧引用；运行时回退请使用 NormalizeTimezone("")（跟随 app.display_timezone）。
// DefaultTimezone is kept for compatibility; runtime fallback uses NormalizeTimezone("") which follows app.display_timezone.
const DefaultTimezone = timex.DefaultDisplayTimezone

var Parser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)

func NormalizeTimezone(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return timex.DisplayTimezone()
	}
	return trimmed
}

func LoadLocation(value string) (*time.Location, error) {
	return time.LoadLocation(NormalizeTimezone(value))
}

func Validate(expr string) error {
	trimmed := strings.TrimSpace(expr)
	if trimmed == "" {
		return fmt.Errorf("cron expression is empty")
	}
	_, err := Parser.Parse(trimmed)
	return err
}

func MatchMinuteWindow(expr string, now time.Time, locationName string) (bool, time.Time, time.Time, error) {
	schedule, err := Parser.Parse(strings.TrimSpace(expr))
	if err != nil {
		return false, time.Time{}, time.Time{}, err
	}
	loc, err := LoadLocation(locationName)
	if err != nil {
		return false, time.Time{}, time.Time{}, err
	}
	localized := now.In(loc).Truncate(time.Minute)
	previousMinute := localized.Add(-time.Minute)
	if !schedule.Next(previousMinute).Equal(localized) {
		return false, time.Time{}, time.Time{}, nil
	}
	return true, localized, localized.Add(time.Minute), nil
}

func NextRun(expr string, from time.Time, locationName string) (*time.Time, error) {
	schedule, err := Parser.Parse(strings.TrimSpace(expr))
	if err != nil {
		return nil, err
	}
	loc, err := LoadLocation(locationName)
	if err != nil {
		return nil, err
	}
	next := schedule.Next(from.In(loc))
	return &next, nil
}
