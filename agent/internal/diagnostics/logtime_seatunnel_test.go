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

package diagnostics

import (
	"testing"
	"time"

	"github.com/LeonYoah/stx/agent/internal/logtime"
)

// TestParseSeatunnelLogHeaderReturnsUTC 使用真实 SeaTunnel 日志行格式验证解析为 UTC 时刻。
// Real SeaTunnel log line format must yield a UTC instant interpreted in the configured log zone.
func TestParseSeatunnelLogHeaderReturnsUTC(t *testing.T) {
	newYork, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("tzdata unavailable")
	}
	logtime.SetLocation(newYork)
	defer logtime.SetLocation(time.Local)
	line := "[] 2026-10-14 03:31:03,960 ERROR [o.a.s.e.s.c.CoordinatorService] [hz.main.generic-operation.thread-1] - Job failed"
	header := parseLogHeader(line)
	if header == nil {
		t.Fatal("header not parsed")
	}
	want := time.Date(2026, 10, 14, 7, 31, 3, 960_000_000, time.UTC)
	if !header.Timestamp.Equal(want) || header.Timestamp.Location() != time.UTC {
		t.Fatalf("got %v want %v", header.Timestamp, want)
	}
}
