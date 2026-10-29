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

package syncalert

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestTaskConfigNormalizeAndValidate(t *testing.T) {
	cfg := (&TaskConfig{
		Rules:           []string{RuleStopTimeout, " " + RuleSubmitFailed, RuleStopTimeout},
		ChannelIDs:      []uint{3, 1, 3},
		ReceiverUserIDs: []uint{7, 7},
		CooldownMinutes: 60,
	}).Normalize()
	if want := []string{RuleSubmitFailed, RuleStopTimeout}; !reflect.DeepEqual(cfg.Rules, want) {
		t.Fatalf("rules not normalized: %v", cfg.Rules)
	}
	if !reflect.DeepEqual(cfg.ChannelIDs, []uint{3, 1}) || !reflect.DeepEqual(cfg.ReceiverUserIDs, []uint{7}) {
		t.Fatalf("ids not de-duplicated: %v %v", cfg.ChannelIDs, cfg.ReceiverUserIDs)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}

	cases := map[string]*TaskConfig{
		"no rules":        {ChannelIDs: []uint{1}},
		"unknown rule":    {Rules: []string{RuleStatusUnavailable}, ChannelIDs: []uint{1}},
		"no channel":      {Rules: []string{RuleJobFailed}},
		"zero channel":    {Rules: []string{RuleJobFailed}, ChannelIDs: []uint{0}},
		"cooldown range":  {Rules: []string{RuleJobFailed}, ChannelIDs: []uint{1}, CooldownMinutes: 1441},
		"zero receiver":   {Rules: []string{RuleJobFailed}, ChannelIDs: []uint{1}, ReceiverUserIDs: []uint{0}},
		"negative period": {Rules: []string{RuleJobFailed}, ChannelIDs: []uint{1}, CooldownMinutes: -1},
	}
	for name, bad := range cases {
		if err := bad.Normalize().Validate(); !errors.Is(err, ErrInvalidConfig) {
			t.Fatalf("%s: expected ErrInvalidConfig, got %v", name, err)
		}
	}
}

func TestTruncateMessage(t *testing.T) {
	if got := TruncateMessage("  short  ", 10); got != "short" {
		t.Fatalf("short message changed: %q", got)
	}
	long := strings.Repeat("错", 12)
	if got := TruncateMessage(long, 10); got != strings.Repeat("错", 10)+"…" {
		t.Fatalf("truncate by characters: %q", got)
	}
}

func TestTaskConfigCDCDelayDefaultsAndRange(t *testing.T) {
	// 旧客户端不带两个新字段（为 0）：补默认值后通过。
	// Old clients omit both new fields (zero): defaults are applied and the config passes.
	cfg := (&TaskConfig{Rules: []string{RuleCDCDelayHigh}, ChannelIDs: []uint{1}}).Normalize()
	if cfg.CDCDelayThresholdSeconds != DefaultCDCDelayThresholdSeconds || cfg.CDCDelayDurationMinutes != DefaultCDCDelayDurationMinutes {
		t.Fatalf("defaults not applied: %+v", cfg)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("defaults must validate: %v", err)
	}
	if !cfg.HasRule(RuleCDCDelayHigh) || cfg.HasRule(RuleJobFailed) {
		t.Fatalf("HasRule wrong: %+v", cfg.Rules)
	}
	// 上下界都可以。 / Both bounds are accepted.
	for _, ok := range [][2]int{{10, 1}, {86400, 60}} {
		c := (&TaskConfig{Rules: []string{RuleJobFailed}, ChannelIDs: []uint{1}, CDCDelayThresholdSeconds: ok[0], CDCDelayDurationMinutes: ok[1]}).Normalize()
		if err := c.Validate(); err != nil {
			t.Fatalf("bounds %v rejected: %v", ok, err)
		}
	}
	// 超出范围：不论是否勾选 B4 都是 400。 / Out of range is a 400 whether or not B4 is selected.
	for _, bad := range [][2]int{{9, 5}, {86401, 5}, {60, 61}, {-1, 5}, {60, -1}} {
		c := (&TaskConfig{Rules: []string{RuleJobFailed}, ChannelIDs: []uint{1}, CDCDelayThresholdSeconds: bad[0], CDCDelayDurationMinutes: bad[1]}).Normalize()
		if err := c.Validate(); !errors.Is(err, ErrInvalidConfig) {
			t.Fatalf("%v: expected ErrInvalidConfig, got %v", bad, err)
		}
	}
	if !IsTaskRule(RuleCDCDelayHigh) || TaskRules()[len(TaskRules())-1] != RuleCDCDelayHigh {
		t.Fatalf("B4 must be the last task rule: %v", TaskRules())
	}
}

func TestFormatDelayZH(t *testing.T) {
	cases := map[int64]string{0: "0秒", 45_000: "45秒", 192_000: "3分12秒", 180_000: "3分", 300_963: "5分1秒", 3_720_000: "1小时2分", 7_200_000: "2小时"}
	for ms, want := range cases {
		if got := FormatDelayZH(ms); got != want {
			t.Fatalf("FormatDelayZH(%d)=%q want %q", ms, got, want)
		}
	}
}
