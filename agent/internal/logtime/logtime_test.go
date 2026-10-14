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

package logtime

import (
	"testing"
	"time"
)

func TestParseReturnsUTC(t *testing.T) {
	shanghai, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Skip("tzdata unavailable")
	}
	once.Do(func() {})
	loc = shanghai
	got, ok := Parse("2026-10-14 13:31:03,960", "2006-01-02 15:04:05,000")
	if !ok || got.Location() != time.UTC || !got.Equal(time.Date(2026, 10, 14, 5, 31, 3, 960_000_000, time.UTC)) {
		t.Fatalf("got %v %v", got, ok)
	}
	if _, ok := Parse("nope", "2006-01-02"); ok {
		t.Fatal("expected failure")
	}
}
