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
	"time"

	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
)

// externalRefCancelWait 是同一 ref 处于停止中时等待终态的上限，测试可改短。
// externalRefCancelWait is how long submit waits for a cancelling job on the same ref; tests may shorten it.
var externalRefCancelWait = 2 * time.Minute

const externalRefCancelPoll = 2 * time.Second

// attachExternalRef 在同一外部引用仍有作业时接管或等到停止完成。
// attachExternalRef attaches to a still-running job on the same ref, or waits out cancellation.
func (s *Service) attachExternalRef(ctx context.Context, ref string) (*JobInstance, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, nil
	}
	existing, err := s.repo.GetActiveJobByExternalRef(ctx, ref)
	if err != nil || existing == nil {
		return nil, err
	}
	if isCancellingJobStatus(existing.Status) {
		deadline := time.Now().Add(externalRefCancelWait)
		for time.Now().Before(deadline) {
			refreshed, refreshErr := s.refreshJobInstance(ctx, existing)
			if refreshErr == nil && refreshed != nil {
				existing = refreshed
			}
			if isFinalNormalizedJobStatus(existing.Status) {
				return nil, nil
			}
			if !isCancellingJobStatus(existing.Status) && isActiveJobStatus(existing.Status) {
				break
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(externalRefCancelPoll):
			}
			latest, loadErr := s.repo.GetJobInstanceByID(ctx, existing.ID)
			if loadErr != nil {
				return nil, loadErr
			}
			existing = latest
		}
		if isCancellingJobStatus(existing.Status) {
			return nil, ErrJobCancellingTimeout
		}
		if isFinalNormalizedJobStatus(existing.Status) {
			return nil, nil
		}
	}
	if !isActiveJobStatus(existing.Status) {
		return nil, nil
	}
	existing.AttachGeneration++
	if existing.AttachGeneration < 1 {
		existing.AttachGeneration = 1
	}
	if err := s.repo.UpdateJobInstance(ctx, existing); err != nil {
		return nil, err
	}
	return existing, nil
}

// CancelJobByRef 按外部引用停止作业；generation 为 0 时使用当前代次。
// CancelJobByRef stops the job for an external ref; generation 0 means "use the current generation".
func (s *Service) CancelJobByRef(ctx context.Context, actor executionapp.Actor, ref string, generation int) (*JobInstance, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, nil
	}
	existing, err := s.repo.GetActiveJobByExternalRef(ctx, ref)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, nil
	}
	if generation > 0 && existing.AttachGeneration != generation {
		return nil, ErrAttachGenerationMismatch
	}
	if isFinalNormalizedJobStatus(existing.Status) {
		return existing, nil
	}
	return s.CancelJobForActor(ctx, actor, existing.ID, false)
}

func (s *Service) enforceSchedulerScope(context.Context, uint, uint) error {
	// Worker 使用管理员签发的 stx_ 登录令牌，权限与对应用户一致，不再按任务收窄。
	// Workers use admin-issued stx_ login tokens with the same user permissions; no per-task narrowing.
	return nil
}
