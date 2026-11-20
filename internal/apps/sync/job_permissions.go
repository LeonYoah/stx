package sync

import (
	"context"

	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
)

// JobPermissions 是某个用户对一条作业的权限裁决，读取、列表、首页、大盘和控制入口都以它为准。
// 只回答"能不能"，作业是否处于可停止 / 可恢复的生命周期由调用方再判断。
// JobPermissions is one user's verdict on one job; read, list, home, dashboard and control entries all defer to
// it. It only answers "may the user", while whether the lifecycle allows stopping / recovering is up to callers.
type JobPermissions struct {
	CanView    bool
	CanStop    bool
	CanRecover bool
}

// EvaluateJobPermissions 统一裁决作业权限。外部作业：登录用户只读可见，仅管理员可停止，任何人不可恢复。
// STX 作业：管理员与发起人可见可停；任务仍存在时，可查看任务者可见、任务所有者可停、可运行任务者可恢复。
// task 传入作业所属任务（不存在或已删除传 nil）。
// EvaluateJobPermissions decides job permissions in one place. External jobs: visible read-only to signed-in
// users, stoppable by admins only, never recoverable. STX jobs: admins and the submitter may view and stop; while
// the task still exists, task viewers may view, the task owner may stop and task runners may recover. task is the
// owning task, or nil when missing or deleted.
func EvaluateJobPermissions(job *JobInstance, task *Task, actor executionapp.Actor) JobPermissions {
	if job == nil {
		return JobPermissions{}
	}
	if job.Source == JobSourceExternal {
		return JobPermissions{CanView: true, CanStop: actor.IsAdmin}
	}
	userID := uint(actor.UserID)
	if job.TaskDeletedAt != nil || job.TaskID == 0 {
		task = nil
	}
	isSubmitter := job.CreatedBy != 0 && job.CreatedBy == userID
	perms := JobPermissions{
		CanView: actor.IsAdmin || isSubmitter || (task != nil && task.CanUserView(userID, false)),
		CanStop: actor.IsAdmin || isSubmitter || (task != nil && task.CreatedBy != 0 && task.CreatedBy == userID),
	}
	perms.CanRecover = task != nil && job.RunType != RunTypePreview && job.PlatformJobID != "" &&
		task.CanUserRun(userID, actor.IsAdmin)
	return perms
}

func applyJobPermissions(job *JobInstance, perms JobPermissions) {
	job.CanStop = perms.CanStop
	job.CanRecover = perms.CanRecover
}

// loadJobPermissions 读取作业所属任务后裁决权限；任务读取失败按任务不存在处理。
// loadJobPermissions loads the owning task and evaluates permissions; a failed task read counts as no task.
func (s *Service) loadJobPermissions(ctx context.Context, actor executionapp.Actor, job *JobInstance) JobPermissions {
	var task *Task
	if job != nil && job.Source != JobSourceExternal && job.TaskID > 0 && job.TaskDeletedAt == nil {
		if loaded, err := s.repo.GetTaskByID(ctx, job.TaskID); err == nil {
			task = loaded
		}
	}
	return EvaluateJobPermissions(job, task, actor)
}

// decorateJobPermissions 批量给列表行填权限，同一任务只读一次。
// decorateJobPermissions fills permissions on list rows, loading each task once.
func (s *Service) decorateJobPermissions(ctx context.Context, actor executionapp.Actor, jobs []*JobInstance) {
	tasks := map[uint]*Task{}
	for _, job := range jobs {
		if job == nil {
			continue
		}
		var task *Task
		if job.Source != JobSourceExternal && job.TaskID > 0 && job.TaskDeletedAt == nil {
			cached, ok := tasks[job.TaskID]
			if !ok {
				if loaded, err := s.repo.GetTaskByID(ctx, job.TaskID); err == nil {
					cached = loaded
				}
				tasks[job.TaskID] = cached
			}
			task = cached
		}
		applyJobPermissions(job, EvaluateJobPermissions(job, task, actor))
	}
}
