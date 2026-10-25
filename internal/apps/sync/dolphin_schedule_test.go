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
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	gosync "sync"
	"testing"

	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
)

// scheduleFake 是有状态的 Dolphin 假服务：下线工作流时连带下线其定时（与 DolphinScheduler 一致），
// 记录工作流上下线与定时上线调用。
// scheduleFake is a stateful Dolphin fake: offlining the workflow also offlines its schedules (as DolphinScheduler
// does); it records workflow release and schedule online calls.
type scheduleFake struct {
	mu              gosync.Mutex
	releaseState    string
	schedules       map[int64]string // id -> releaseState
	releases        []string
	onlined         []int64
	failScheduleGET bool
}

var scheduleOnlinePath = regexp.MustCompile(`/schedules/(\d+)/online$`)

func (f *scheduleFake) handler(t *testing.T) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		path := request.URL.Path
		switch {
		case strings.Contains(path, "workflow-definition"):
			http.NotFound(writer, request)
		case strings.HasSuffix(path, "/process-definition") && request.Method == http.MethodGet:
			writeDolphinOK(writer, map[string]any{"totalList": []any{}})
		case strings.Contains(path, "/process-definition/9/release"):
			_ = request.ParseForm()
			state := request.Form.Get("releaseState")
			f.releases = append(f.releases, state)
			f.releaseState = state
			if state == "OFFLINE" {
				for id := range f.schedules {
					f.schedules[id] = "OFFLINE"
				}
			}
			writeDolphinOK(writer, true)
		case strings.Contains(path, "/process-definition/9") && request.Method == http.MethodGet:
			writeDolphinOK(writer, map[string]any{
				"processDefinition": map[string]any{"code": 9, "name": "wf", "version": 3, "releaseState": f.releaseState},
				"taskDefinitionList": []map[string]any{
					{"code": 88, "name": "node", "taskType": "SHELL", "version": 1, "taskParams": map[string]any{"rawScript": "old"}},
				},
				"processTaskRelationList": []map[string]any{{"preTaskCode": 0, "postTaskCode": 88}},
			})
		case strings.Contains(path, "/process-definition/9") && request.Method == http.MethodPut:
			writeDolphinOK(writer, map[string]any{})
		case scheduleOnlinePath.MatchString(path):
			id, _ := strconv.ParseInt(scheduleOnlinePath.FindStringSubmatch(path)[1], 10, 64)
			f.schedules[id] = "ONLINE"
			f.onlined = append(f.onlined, id)
			writeDolphinOK(writer, true)
		case strings.HasSuffix(path, "/schedules") && request.Method == http.MethodGet:
			if f.failScheduleGET {
				http.Error(writer, "boom", http.StatusInternalServerError)
				return
			}
			list := make([]any, 0, len(f.schedules))
			for _, id := range []int64{11, 12} {
				if state, ok := f.schedules[id]; ok {
					list = append(list, map[string]any{"id": id, "crontab": "0 0 2 * * ? *", "timezoneId": "Asia/Shanghai", "releaseState": state})
				}
			}
			writeDolphinOK(writer, map[string]any{"totalList": list})
		default:
			writeDolphinOK(writer, map[string]any{})
		}
	}
}

func pushBoundUpdate(t *testing.T, fake *scheduleFake) (*DolphinPushResult, error) {
	t.Helper()
	server := httptest.NewServer(fake.handler(t))
	t.Cleanup(server.Close)
	service, task, connID := newBoundPushService(t, server)
	if err := service.repo.SaveSchedulerBinding(context.Background(), &TaskSchedulerBinding{
		TaskID: task.ID, Kind: SchedulerKindDolphin, ConnectionID: connID, ProjectCode: 7, WorkflowCode: 9, TaskCode: 88,
	}); err != nil {
		t.Fatalf("save binding: %v", err)
	}
	return service.PushDolphin(context.Background(), executionapp.Actor{UserID: 1, IsAdmin: true}, task.ID, &DolphinPushRequest{
		Mode: "update", ConnectionID: connID,
	})
}

// TestPushDolphinKeepsPausedSchedulePaused 回归：推送在线工作流会先下线（Dolphin 连带下线定时）再上线；
// 之后只能恢复推送前在线的定时，用户暂停的定时必须保持暂停。
// TestPushDolphinKeepsPausedSchedulePaused is a regression test: pushing into an online workflow offlines it (Dolphin
// offlines its schedules too) and re-onlines it; only schedules online before the push may be restored, and a
// schedule the user paused must stay paused.
func TestPushDolphinKeepsPausedSchedulePaused(t *testing.T) {
	fake := &scheduleFake{releaseState: "ONLINE", schedules: map[int64]string{11: "ONLINE", 12: "OFFLINE"}}
	result, err := pushBoundUpdate(t, fake)
	if err != nil {
		t.Fatalf("update push: %v", err)
	}
	if !reflect.DeepEqual(fake.releases, []string{"OFFLINE", "ONLINE"}) {
		t.Fatalf("an online workflow must be offlined and re-onlined, got %v", fake.releases)
	}
	if !reflect.DeepEqual(fake.onlined, []int64{11}) || fake.schedules[12] != "OFFLINE" || fake.schedules[11] != "ONLINE" {
		t.Fatalf("only the previously online schedule may be restored: onlined=%v states=%v", fake.onlined, fake.schedules)
	}
	if !reflect.DeepEqual(result.SchedulesRestored, []int64{11}) || len(result.Schedules) != 2 {
		t.Fatalf("result must report schedules and the restored ones: %+v", result)
	}
}

// TestPushDolphinOfflineWorkflowStaysOffline 原本下线的工作流：不上线、不碰定时。
// TestPushDolphinOfflineWorkflowStaysOffline: a workflow that was offline is neither onlined nor are its schedules touched.
func TestPushDolphinOfflineWorkflowStaysOffline(t *testing.T) {
	fake := &scheduleFake{releaseState: "OFFLINE", schedules: map[int64]string{11: "OFFLINE"}}
	result, err := pushBoundUpdate(t, fake)
	if err != nil {
		t.Fatalf("update push: %v", err)
	}
	if len(fake.releases) != 0 || len(fake.onlined) != 0 {
		t.Fatalf("an offline workflow must stay offline: releases=%v onlined=%v", fake.releases, fake.onlined)
	}
	if result.WouldOffline || len(result.SchedulesRestored) != 0 || len(result.Schedules) != 1 {
		t.Fatalf("unexpected result for an offline workflow: %+v", result)
	}
}

// TestPushDolphinAbortsWhenSchedulesUnreadable 在线工作流读不到定时：中止推送，不下线、不改图。
// TestPushDolphinAbortsWhenSchedulesUnreadable: when an online workflow's schedules cannot be read, the push aborts
// without offlining or changing the graph.
func TestPushDolphinAbortsWhenSchedulesUnreadable(t *testing.T) {
	fake := &scheduleFake{releaseState: "ONLINE", schedules: map[int64]string{11: "ONLINE"}, failScheduleGET: true}
	if _, err := pushBoundUpdate(t, fake); err == nil || !strings.Contains(err.Error(), "nothing was changed") {
		t.Fatalf("expected an abort before any change, got %v", err)
	}
	if len(fake.releases) != 0 {
		t.Fatalf("the workflow must not be offlined when schedules are unreadable: %v", fake.releases)
	}
}
