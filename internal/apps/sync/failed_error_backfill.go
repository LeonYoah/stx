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
	"strings"
	gosync "sync"
	"sync/atomic"
	"time"
)

// 引擎先报 FAILED、约 50 秒后才在 job-info 里填上 errorMsg（2026-10-28 在 SeaTunnel 3.0.0 实测）。作业一进终态，
// 刷新就会被跳过，错误信息因此一直为空。这里在失败后的有限窗口内低频补拉 job-info，直到拿到错误信息。
// The engine reports FAILED first and fills errorMsg in job-info about 50s later (measured on SeaTunnel 3.0.0,
// 2026-10-28). Refreshes are skipped once a job is terminal, so the error stayed empty. Within a bounded window after
// the failure, job-info is re-fetched at a low rate until the error message arrives.
const (
	failedErrorBackfillWindow   = 5 * time.Minute
	failedErrorBackfillInterval = 15 * time.Second
	failedErrorBackfillBatch    = 20
)

// failedErrorBackfill 记录补拉是否在进行以及每个作业上次补拉的时间；零值可用。
// failedErrorBackfill tracks whether a backfill pass is running and when each job was last probed; the zero value works.
type failedErrorBackfill struct {
	running atomic.Bool
	mu      gosync.Mutex
	probed  map[uint]time.Time
}

// due 判断作业是否到了下一次补拉时间，并清掉超出窗口的记录。
// due reports whether the job is due for another probe and prunes entries past the window.
func (b *failedErrorBackfill) due(jobID uint, now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.probed == nil {
		b.probed = map[uint]time.Time{}
	}
	for id, at := range b.probed {
		if now.Sub(at) > failedErrorBackfillWindow {
			delete(b.probed, id)
		}
	}
	if last, ok := b.probed[jobID]; ok && now.Sub(last) < failedErrorBackfillInterval {
		return false
	}
	b.probed[jobID] = now
	return true
}

// awaitingEngineErrorMessage 判断失败作业是否还在等引擎补上错误信息（失败不久、错误为空、有引擎作业）。
// awaitingEngineErrorMessage reports whether a failed job is still waiting for the engine's error message (failed
// recently, empty error, has an engine job).
func awaitingEngineErrorMessage(job *JobInstance, now time.Time) bool {
	return job != nil &&
		job.Status == JobStatusFailed &&
		strings.TrimSpace(job.ErrorMessage) == "" &&
		strings.TrimSpace(job.EngineJobID) != "" &&
		job.FinishedAt != nil &&
		now.Sub(*job.FinishedAt) < failedErrorBackfillWindow
}

// backfillFailedJobErrors 在后台为刚失败、错误为空的作业补拉一次 job-info；同一时刻只跑一轮，不阻塞对账节拍。
// backfillFailedJobErrors re-fetches job-info in the background for recently failed jobs with an empty error; only one
// pass runs at a time and it never blocks the reconcile tick.
func (s *Service) backfillFailedJobErrors(ctx context.Context, now time.Time) {
	if s == nil || s.repo == nil || s.engineClient == nil || !s.errorBackfill.running.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer s.errorBackfill.running.Store(false)
		jobs, err := s.repo.ListRecentFailedJobsWithoutError(ctx, now.Add(-failedErrorBackfillWindow), failedErrorBackfillBatch)
		if err != nil {
			return
		}
		for _, job := range jobs {
			if !awaitingEngineErrorMessage(job, now) || !s.errorBackfill.due(job.ID, now) {
				continue
			}
			refreshCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
			_, _ = s.refreshJobInstance(refreshCtx, job)
			cancel()
		}
	}()
}
