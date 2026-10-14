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

package config

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// 本地时区写入的 updated_at 必须能被 UTC 边界筛到。
// updated_at written in local time must match UTC bounds.
func TestListUpdatedByClusterBetweenAcrossTimezones(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&Config{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := NewRepository(db)
	ctx := context.Background()
	updated := time.Date(2026, 10, 14, 12, 54, 29, 0, time.Local)
	if err := db.Create(&Config{ClusterID: 1, ConfigType: ConfigType("seatunnel.yaml"), CreatedAt: updated, UpdatedAt: updated}).Error; err != nil {
		t.Fatalf("create config: %v", err)
	}
	start := updated.Add(-time.Minute)
	end := updated.Add(time.Minute)
	for _, tc := range []struct {
		name       string
		start, end time.Time
		want       int
	}{
		{"utc bounds", start.UTC(), end.UTC(), 1},
		{"local bounds", start, end, 1},
		{"window after", end.UTC(), end.Add(time.Minute).UTC(), 0},
	} {
		got, err := repo.ListUpdatedByClusterBetween(ctx, 1, tc.start, tc.end)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if len(got) != tc.want {
			t.Errorf("%s: got %d rows want %d", tc.name, len(got), tc.want)
		}
	}
}
