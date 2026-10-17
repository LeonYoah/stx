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
	"errors"
	"testing"
	"time"

	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
)

func TestAttachExternalRefIncrementsGeneration(t *testing.T) {
	service := newTestSyncService(t)
	ctx := context.Background()
	externalRefCancelWait = 50 * time.Millisecond
	t.Cleanup(func() { externalRefCancelWait = 2 * time.Minute })

	if err := service.repo.CreateJobInstance(ctx, &JobInstance{
		TaskID:           1,
		TaskVersion:      1,
		RunType:          RunTypeRun,
		Status:           JobStatusRunning,
		ExternalRef:      "ds:11:22",
		AttachGeneration: 1,
	}); err != nil {
		t.Fatalf("create job: %v", err)
	}

	attached, err := service.attachExternalRef(ctx, "ds:11:22")
	if err != nil {
		t.Fatalf("attach: %v", err)
	}
	if attached == nil || attached.AttachGeneration != 2 {
		t.Fatalf("expected generation 2, got %+v", attached)
	}

	if _, err := service.CancelJobByRef(ctx, executionapp.Actor{UserID: 1, IsAdmin: true}, "ds:11:22", 1); !errors.Is(err, ErrAttachGenerationMismatch) {
		t.Fatalf("stale generation should return mismatch, got %v", err)
	}
}

func TestAttachExternalRefResubmitsAfterTerminal(t *testing.T) {
	service := newTestSyncService(t)
	ctx := context.Background()
	if err := service.repo.CreateJobInstance(ctx, &JobInstance{
		TaskID:      1,
		TaskVersion: 1,
		RunType:     RunTypeRun,
		Status:      JobStatusFailed,
		ExternalRef: "ds:11:22",
	}); err != nil {
		t.Fatalf("create job: %v", err)
	}
	attached, err := service.attachExternalRef(ctx, "ds:11:22")
	if err != nil {
		t.Fatalf("attach: %v", err)
	}
	if attached != nil {
		t.Fatalf("terminal job should not attach: %+v", attached)
	}
}

func TestCancelJobByRefMissingIsDone(t *testing.T) {
	service := newTestSyncService(t)
	job, err := service.CancelJobByRef(context.Background(), executionapp.Actor{UserID: 1, IsAdmin: true}, "ds:missing:1", 0)
	if err != nil {
		t.Fatalf("cancel missing ref: %v", err)
	}
	if job != nil {
		t.Fatalf("expected nil job for missing ref, got %+v", job)
	}
}
