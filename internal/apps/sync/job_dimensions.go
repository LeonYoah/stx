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

	"github.com/LeonYoah/stx/internal/logger"
	"gorm.io/gorm"
)

// jobDimensionBackfillBatch 是启动回填每批处理的行数。
// jobDimensionBackfillBatch is the number of rows handled per startup backfill batch.
const jobDimensionBackfillBatch = 500

// normalizeJobMode 把各种来源的模式统一为 streaming / batch，认不出时返回空串。
// normalizeJobMode maps a mode from any source to streaming / batch, or "" when it is unknown.
func normalizeJobMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case string(TaskModeBatch):
		return string(TaskModeBatch)
	case string(TaskModeStreaming):
		return string(TaskModeStreaming)
	default:
		return ""
	}
}

// fillJobDimensions 从提交快照补齐 cluster_id 与 job_mode；已有值不覆盖。
// fillJobDimensions fills cluster_id and job_mode from the submit snapshot; existing values are kept.
func fillJobDimensions(job *JobInstance) {
	if job == nil {
		return
	}
	if job.Source == "" {
		job.Source = JobSourceSTX
	}
	if job.ClusterID == 0 {
		job.ClusterID = uintValue(job.SubmitSpec, "cluster_id")
	}
	if job.JobMode == "" {
		job.JobMode = normalizeJobMode(jobRunMode(job))
	}
}

// BeforeCreate 在写入前派生维度列，覆盖提交、恢复、接管、预览等所有创建路径。
// BeforeCreate derives the dimension columns before insert, covering submit, recover, attach and preview alike.
func (j *JobInstance) BeforeCreate(_ *gorm.DB) error {
	fillJobDimensions(j)
	return nil
}

// effectiveJobMode 返回响应用的模式：优先维度列，其次现算。
// effectiveJobMode returns the mode for responses: the dimension column first, then derived on the fly.
func effectiveJobMode(job *JobInstance) string {
	if job == nil {
		return ""
	}
	if job.JobMode != "" {
		return job.JobMode
	}
	return normalizeJobMode(jobRunMode(job))
}

// BackfillJobDimensions 一次性补齐历史作业的 cluster_id / job_mode：快照里取不到时退回任务当前的集群与模式。
// 幂等，可重复执行；失败只记日志，不影响启动。
// BackfillJobDimensions fills cluster_id / job_mode on old jobs once, falling back to the task's current cluster and
// mode when the snapshot lacks them. It is idempotent; failures are logged and never block startup.
func (s *Service) BackfillJobDimensions(ctx context.Context) {
	if s == nil || s.repo == nil {
		return
	}
	updated, err := s.repo.BackfillJobDimensions(ctx, jobDimensionBackfillBatch)
	if err != nil {
		logger.WarnF(ctx, "[Sync] job dimension backfill failed: updated=%d err=%v", updated, err)
		return
	}
	if updated > 0 {
		logger.InfoF(ctx, "[Sync] job dimension backfill done: updated=%d", updated)
	}
}
