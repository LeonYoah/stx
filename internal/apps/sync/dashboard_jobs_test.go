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
package sync

import (
	"context"
	"reflect"
	"testing"
	"time"

	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
	"github.com/LeonYoah/stx/internal/pkg/timex"
)

func jobDag(names ...string) JSONMap {
	vertices := make([]interface{}, 0, len(names))
	for _, name := range names {
		vertices = append(vertices, map[string]interface{}{"vertexName": name})
	}
	return JSONMap{"job_dag": map[string]interface{}{"vertexInfoMap": vertices}}
}

func TestParseJobPluginsFromVertexNames(t *testing.T) {
	sources, sinks := parseJobPlugins(&JobInstance{ResultPreview: jobDag(
		"pipeline-1 [Source[0]-MySQL-CDC]",
		"pipeline-1 [Transform[0]-Sql]",
		"pipeline-1 [Sink[0]-Doris-MultiTableSink]",
		"pipeline-2 [Sink[1]-Doris]",
		"pipeline-2 [Sink[2]-Console]",
	)})
	if !reflect.DeepEqual(sources, []string{"MySQL-CDC"}) || !reflect.DeepEqual(sinks, []string{"Doris", "Console"}) {
		t.Fatalf("sources %v sinks %v", sources, sinks)
	}
	if s, k := parseJobPlugins(&JobInstance{}); s != nil || k != nil {
		t.Fatalf("empty dag = %v %v", s, k)
	}
}

func TestJobRunCalendarBucketsByDisplayDay(t *testing.T) {
	service := newTestSyncService(t)
	db := service.repo.db
	// 2026-11-15 是周日；Asia/Shanghai 下 UTC 16:30 已是次日。/ 2026-11-15 is a Sunday; 16:30 UTC is the next day in Shanghai.
	previous := timex.DisplayTimezone()
	if err := timex.SetDisplayTimezone("Asia/Shanghai"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = timex.SetDisplayTimezone(previous) })
	now := time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)
	at := func(v time.Time) *time.Time { return &v }
	jobs := []*JobInstance{
		{TaskID: 1, RunType: RunTypeRun, Status: JobStatusSuccess, StartedAt: at(time.Date(2026, 11, 15, 2, 0, 0, 0, time.UTC)), SubmitSpec: JSONMap{"cluster_id": uint(1)}},
		{TaskID: 1, RunType: RunTypeRun, Status: JobStatusFailed, StartedAt: at(time.Date(2026, 11, 15, 3, 0, 0, 0, time.UTC)), SubmitSpec: JSONMap{"cluster_id": uint(1)}},
		{TaskID: 1, RunType: RunTypeRun, Status: JobStatusSuccess, StartedAt: at(time.Date(2026, 11, 9, 16, 30, 0, 0, time.UTC)), SubmitSpec: JSONMap{"cluster_id": uint(2)}},
		{TaskID: 1, RunType: RunTypePreview, Status: JobStatusSuccess, StartedAt: at(time.Date(2026, 11, 15, 3, 0, 0, 0, time.UTC))},
	}
	for _, job := range jobs {
		if err := db.Create(job).Error; err != nil {
			t.Fatal(err)
		}
	}
	cal, err := service.JobRunCalendar(context.Background(), 0, 2, now)
	if err != nil {
		t.Fatalf("calendar: %v", err)
	}
	if cal.EndDate != "2026-11-15" || len(cal.Runs) != 2 {
		t.Fatalf("calendar = %+v", cal)
	}
	// 两周日历的列是 11-02 周与 11-09 周。最后一列周日（索引 6）：2 次运行、1 次失败；
	// UTC 11-09 16:30 在上海已是 11-10 周二，落在第二列索引 1；预览运行不计。
	// The two columns are the weeks of 11-02 and 11-09. Sunday of the last column (index 6): 2 runs, 1 failure;
	// 16:30 UTC on 11-09 is Tuesday 11-10 in Shanghai, so it lands in column 2 index 1; previews do not count.
	if cal.Runs[1][6] != 2 || cal.Failed[1][6] != 1 || cal.Runs[1][1] != 1 || cal.Runs[1][0] != 0 || cal.Runs[0][1] != 0 {
		t.Fatalf("runs = %v failed = %v", cal.Runs, cal.Failed)
	}
	scoped, err := service.JobRunCalendar(context.Background(), 2, 2, now)
	if err != nil || scoped.Runs[1][6] != 0 || scoped.Runs[1][1] != 1 {
		t.Fatalf("cluster 2 calendar = %+v %v", scoped, err)
	}
}

func TestJobAnnotationsAndDescribe(t *testing.T) {
	service := newTestSyncService(t)
	_, privateTask := seedJobListFixture(t, service)
	db := service.repo.db
	if err := db.AutoMigrate(&EngineEvent{}); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	start := time.Date(2026, 11, 14, 0, 0, 0, 0, time.UTC)
	end := start.Add(24 * time.Hour)
	if err := db.Model(&JobInstance{}).Where("status IN ?", []JobStatus{JobStatusFailed, JobStatusFailing}).
		Update("finished_at", start.Add(2*time.Hour)).Error; err != nil {
		t.Fatal(err)
	}
	recover := &JobInstance{TaskID: privateTask.ID, RunType: RunTypeRecover, Status: JobStatusRunning, StartedAt: ptrTime(start.Add(3 * time.Hour))}
	if err := db.Create(recover).Error; err != nil {
		t.Fatal(err)
	}
	jobID := recover.ID
	if err := db.Create(&EngineEvent{ClusterID: 1, EventType: "SCHEMA_CHANGE_ADD_COLUMN", EventTimeMs: start.Add(4 * time.Hour).UnixMilli(),
		JobInstanceID: &jobID, DedupeKey: "k1", ReceivedAt: start}).Error; err != nil {
		t.Fatal(err)
	}
	other := executionapp.Actor{UserID: 2}
	items, err := service.JobAnnotationsBetween(ctx, other, 0, start, end)
	if err != nil {
		t.Fatalf("annotations: %v", err)
	}
	kinds := map[string]int{}
	for _, item := range items {
		kinds[item.Kind]++
		if item.Kind == "job_recovered" && item.Viewable {
			t.Fatalf("recover of a private task must not be viewable for user 2: %+v", item)
		}
	}
	if kinds["job_failed"] != 2 || kinds["job_recovered"] != 1 || kinds["schema_change"] != 1 {
		t.Fatalf("annotation kinds = %v", kinds)
	}

	briefs, err := service.DescribeJobsForDashboard(ctx, other, []uint{2, jobID, 999})
	if err != nil {
		t.Fatalf("describe: %v", err)
	}
	if len(briefs) != 2 || !briefs[2].Viewable || briefs[jobID].Viewable || briefs[2].Name != "orders-cdc" || briefs[2].ClusterID != 1 {
		t.Fatalf("briefs = %+v", briefs)
	}
}

func ptrTime(v time.Time) *time.Time { return &v }
