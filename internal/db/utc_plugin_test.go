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

package db

import (
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/LeonYoah/stx/internal/pkg/timex"
)

type utcProbe struct {
	ID         uint
	TaskID     uint
	FinishedAt *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func openUTCProbeDB(t *testing.T) *gorm.DB {
	t.Helper()
	database, err := gorm.Open(sqlite.Open("file::memory:"), &gorm.Config{NowFunc: timex.Now})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Use(UTCPlugin{}); err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&utcProbe{}); err != nil {
		t.Fatal(err)
	}
	return database
}

// TestUTCRangeQueries 在 TZ=UTC 与 TZ=Asia/Shanghai 下验证：无论写入/查询使用何种 Location，SQLite 区间查询结果一致。
// TestUTCRangeQueries verifies SQLite range queries are correct under both UTC and Asia/Shanghai process zones
// regardless of the Location of written values and query bounds.
func TestUTCRangeQueries(t *testing.T) {
	shanghai, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Skip("tzdata unavailable")
	}
	for _, processLoc := range []*time.Location{time.UTC, shanghai} {
		t.Run(processLoc.String(), func(t *testing.T) {
			original := time.Local
			time.Local = processLoc
			defer func() { time.Local = original }()

			database := openUTCProbeDB(t)
			now := time.Now()
			finishedLocal := now.In(shanghai)
			rows := []utcProbe{
				{TaskID: 1},                                     // autoCreateTime -> NowFunc (UTC)
				{TaskID: 1, CreatedAt: now.In(shanghai)},        // explicit +08:00 value
				{TaskID: 1, CreatedAt: now.UTC()},               // explicit UTC value
				{TaskID: 1, FinishedAt: &finishedLocal},         // pointer field
				{TaskID: 2, CreatedAt: now.Add(-2 * time.Hour)}, // outside window
			}
			if err := database.Create(&rows).Error; err != nil {
				t.Fatal(err)
			}

			var raws []string
			database.Raw("SELECT created_at FROM utc_probes").Scan(&raws)
			for _, raw := range raws {
				if raw[len(raw)-1] != 'Z' {
					t.Fatalf("expected UTC (Z) storage, got %q", raw)
				}
			}

			for name, bounds := range map[string][2]time.Time{
				"utc":      {now.Add(-time.Minute).UTC(), now.Add(time.Minute).UTC()},
				"shanghai": {now.Add(-time.Minute).In(shanghai), now.Add(time.Minute).In(shanghai)},
				"local":    {now.Add(-time.Minute), now.Add(time.Minute)},
			} {
				var count int64
				if err := database.Model(&utcProbe{}).Where("task_id = ?", 1).
					Where("created_at >= ? AND created_at < ?", bounds[0], bounds[1]).Count(&count).Error; err != nil {
					t.Fatal(err)
				}
				if count != 4 {
					t.Fatalf("%s bounds: expected 4 rows in window, got %d", name, count)
				}
			}

			var finished int64
			database.Model(&utcProbe{}).Where("finished_at < ?", now.Add(time.Minute).In(shanghai)).Count(&finished)
			if finished != 1 {
				t.Fatalf("pointer field compare: expected 1, got %d", finished)
			}

			// Updates(map) 中的时间也被规范化 / map updates are normalized too
			database.Model(&utcProbe{}).Where("task_id = ?", 2).Updates(map[string]interface{}{"finished_at": now.In(shanghai)})
			var raw string
			database.Raw("SELECT finished_at FROM utc_probes WHERE task_id = 2").Scan(&raw)
			if raw[len(raw)-1] != 'Z' {
				t.Fatalf("map update not normalized: %q", raw)
			}

			// 删除条件同样规范化 / delete conditions are normalized
			res := database.Where("created_at < ?", now.Add(-time.Hour).In(shanghai)).Delete(&utcProbe{})
			if res.RowsAffected != 1 {
				t.Fatalf("delete: expected 1, got %d", res.RowsAffected)
			}
		})
	}
}
