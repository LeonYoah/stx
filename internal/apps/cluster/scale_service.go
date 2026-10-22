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

package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/LeonYoah/stx/internal/apps/audit"
	"github.com/LeonYoah/stx/internal/apps/auth"
	appconfig "github.com/LeonYoah/stx/internal/apps/config"
	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
	installerapp "github.com/LeonYoah/stx/internal/apps/installer"
	pluginapp "github.com/LeonYoah/stx/internal/apps/plugin"
	"github.com/LeonYoah/stx/internal/db"
	"github.com/LeonYoah/stx/internal/logger"
)

// 伸缩操作在公共执行记录中的模块名与操作编号（与 internal/operation 登记一致）。
// Module name and operation IDs of scaling operations in shared execution records (aligned with internal/operation).
const (
	scaleExecutionModule      = "cluster"
	scaleOutOperationID       = "cluster.scale-out.execute"
	scaleInOperationID        = "cluster.scale-in.execute"
	scaleRetryOperationID     = "cluster.scale-operation.retry"
	scaleNotCancellableReason = "scaling operations cannot be cancelled safely; retry or scale in to clean up"
)

// ScaleConfigManager 是伸缩编排使用的配置服务能力（由 config.Service 实现）。
// ScaleConfigManager is the config service capability used by the scaling orchestrator (implemented by config.Service).
type ScaleConfigManager interface {
	GetByCluster(ctx context.Context, clusterID uint) ([]*appconfig.ConfigInfo, error)
	PatchMemberList(ctx context.Context, clusterID uint, deploymentMode string, members []string, opRef string, userID uint) (*appconfig.MemberListPatchResult, error)
	ApplyTemplatesToHost(ctx context.Context, clusterID uint, hostID uint, deploymentMode string, members []string, opRef string, userID uint) (*appconfig.ConfigChangeResult, error)
	RegisterHostConfigsFromNode(ctx context.Context, clusterID uint, hostID uint, installDir string, userID uint) (*appconfig.ConfigChangeResult, error)
	DeleteHostConfigs(ctx context.Context, clusterID uint, hostID uint) (int, error)
}

// ScaleInstaller 是扩容安装模式使用的安装器能力（由 installer.Service 实现）。
// ScaleInstaller is the installer capability used by install-mode scale-out (implemented by installer.Service).
type ScaleInstaller interface {
	StartInstallation(ctx context.Context, req *installerapp.InstallationRequest) (*installerapp.InstallationStatus, error)
	GetInstallationStatusSnapshot(ctx context.Context, hostID uint) (*installerapp.InstallationStatus, error)
}

// ScalePluginReplayer 是新节点补齐集群插件的能力（由 plugin.Service 实现）。
// ScalePluginReplayer replays cluster plugins to a joining node (implemented by plugin.Service).
type ScalePluginReplayer interface {
	ListReplayCandidates(ctx context.Context, clusterID uint) ([]pluginapp.PluginReplayCandidate, error)
	ReplayClusterPluginsToHost(ctx context.Context, clusterID uint, hostID uint, installDir string) (*pluginapp.PluginReplayReport, error)
}

// ScaleRunningJob 是集群上一个运行中的作业。
// ScaleRunningJob is one job running on the cluster.
type ScaleRunningJob struct {
	JobID   string `json:"job_id"`
	JobName string `json:"job_name"`
}

// RunningJobsProvider 按集群维度读取运行中作业；由 router 适配 sync 服务注入，避免 cluster 直接依赖 sync。
// RunningJobsProvider reads running jobs per cluster; router injects a sync adapter so cluster never imports sync.
type RunningJobsProvider interface {
	ListClusterRunningJobs(ctx context.Context, clusterID uint) ([]ScaleRunningJob, error)
}

// ScaleDiscoveredProcess 是主机上发现的一个 SeaTunnel 进程。
// ScaleDiscoveredProcess is one SeaTunnel process discovered on a host.
type ScaleDiscoveredProcess struct {
	PID           int
	Role          string
	InstallDir    string
	Version       string
	HazelcastPort int
	APIPort       int
}

// ScaleProcessDiscoverer 读取主机上运行中的 SeaTunnel 进程（由 router 适配 discovery 服务注入）。
// ScaleProcessDiscoverer lists running SeaTunnel processes on a host (router injects a discovery adapter).
type ScaleProcessDiscoverer interface {
	DiscoverSeaTunnelProcesses(ctx context.Context, hostID uint) ([]ScaleDiscoveredProcess, error)
}

// ClusterMonitorPusher 按集群向主机 Agent 下发监控跟踪列表（由 gRPC Server 实现）。
// ClusterMonitorPusher pushes a cluster-scoped monitor tracking list to a host Agent (implemented by the gRPC server).
type ClusterMonitorPusher interface {
	PushClusterMonitorConfigToHost(ctx context.Context, clusterID uint, hostID uint) error
}

// scaleTiming 汇总编排中的轮询间隔与超时，测试可缩短。
// scaleTiming collects the orchestrator's poll intervals and timeouts so tests can shorten them.
type scaleTiming struct {
	pollInterval        time.Duration
	installTimeout      time.Duration
	loopbackNodeTimeout time.Duration
	membershipTimeout   time.Duration
	javaProxyTimeout    time.Duration
	stopTimeout         time.Duration
}

func defaultScaleTiming() scaleTiming {
	return scaleTiming{
		pollInterval:        3 * time.Second,
		installTimeout:      30 * time.Minute,
		loopbackNodeTimeout: 180 * time.Second,
		membershipTimeout:   180 * time.Second,
		javaProxyTimeout:    90 * time.Second,
		stopTimeout:         60 * time.Second,
	}
}

// ScaleService 编排集群扩缩容：只读计划（Plan）+ 异步分步执行（Execute），步骤状态持久化并可从失败步骤重试。
// ScaleService orchestrates cluster scaling: a read-only Plan plus asynchronous step execution whose state is
// persisted and can be retried from the failed step.
type ScaleService struct {
	svc        *Service
	repo       *ScaleRepository
	config     ScaleConfigManager
	installer  ScaleInstaller
	plugins    ScalePluginReplayer
	jobs       RunningJobsProvider
	discoverer ScaleProcessDiscoverer
	monitor    ClusterMonitorPusher
	execution  *executionapp.Service

	locksMu      sync.Mutex
	clusterLocks map[uint]*sync.Mutex

	// launch 启动后台执行，默认新开协程；测试可替换为同步执行。
	// launch starts background execution in a new goroutine by default; tests may run it synchronously.
	launch func(func())
	timing scaleTiming
	now    func() time.Time
}

// NewScaleService 创建伸缩编排服务；其余依赖通过 Set* 注入。
// NewScaleService creates the scaling orchestrator; other dependencies are injected through Set*.
func NewScaleService(svc *Service, repo *ScaleRepository) *ScaleService {
	return &ScaleService{
		svc:          svc,
		repo:         repo,
		clusterLocks: make(map[uint]*sync.Mutex),
		launch:       func(fn func()) { go fn() },
		timing:       defaultScaleTiming(),
		now:          time.Now,
	}
}

// SetConfigManager 注入配置服务。
// SetConfigManager injects the config service.
func (s *ScaleService) SetConfigManager(manager ScaleConfigManager) { s.config = manager }

// SetInstaller 注入安装器。
// SetInstaller injects the installer.
func (s *ScaleService) SetInstaller(installer ScaleInstaller) { s.installer = installer }

// SetPluginReplayer 注入插件补齐能力。
// SetPluginReplayer injects the plugin replayer.
func (s *ScaleService) SetPluginReplayer(replayer ScalePluginReplayer) { s.plugins = replayer }

// SetRunningJobsProvider 注入运行中作业读取能力。
// SetRunningJobsProvider injects the running jobs provider.
func (s *ScaleService) SetRunningJobsProvider(provider RunningJobsProvider) { s.jobs = provider }

// SetProcessDiscoverer 注入主机进程发现能力。
// SetProcessDiscoverer injects the host process discoverer.
func (s *ScaleService) SetProcessDiscoverer(discoverer ScaleProcessDiscoverer) {
	s.discoverer = discoverer
}

// SetMonitorPusher 注入按集群下发监控配置的能力。
// SetMonitorPusher injects the cluster-scoped monitor config pusher.
func (s *ScaleService) SetMonitorPusher(pusher ClusterMonitorPusher) { s.monitor = pusher }

// SetExecutionService 注入公共执行服务（R2 提交、R1 重试与审计）。
// SetExecutionService injects the shared execution service (R2 submission, R1 retry and auditing).
func (s *ScaleService) SetExecutionService(service *executionapp.Service) { s.execution = service }

// ScaleExecutionRequest 保存伸缩写请求的公共执行元数据（不含请求正文）。
// ScaleExecutionRequest stores shared-execution metadata of a scaling write request (no request body).
type ScaleExecutionRequest struct {
	RequestID      string
	IdempotencyKey string
	RequestHash    string
	Confirmed      bool
	ConfirmationID string
	ClientType     string
	Lang           string
	// Username 是发起人登录名，随操作记录保存，用于并发冲突时告知其他用户是谁在操作。
	// Username is the initiator's login name, stored with the operation so a conflict can tell other users who
	// is running it.
	Username string
}

// ScaleSubmitResult 是提交或重试伸缩操作的响应。
// ScaleSubmitResult is the response of submitting or retrying a scaling operation.
type ScaleSubmitResult struct {
	ExecutionID string              `json:"execution_id"`
	OperationID uint                `json:"operation_id"`
	Operation   *ScaleOperationInfo `json:"operation"`
}

// scaleText 按界面语言返回单语言文案（zh 默认，en 可选），避免中英拼接。
// scaleText returns single-language text for the UI language (zh by default, en optional) instead of mixing both.
func scaleText(lang, zh, en string) string {
	if lang == "en" {
		return en
	}
	return zh
}

// normalizeScaleLang 归一化语言参数。
// normalizeScaleLang normalizes the language parameter.
func normalizeScaleLang(value string) string {
	return auth.NormalizeLanguage(value)
}

// clusterLock 返回集群级提交锁，保证同一集群的“检查进行中操作 + 创建操作”是原子的。
// clusterLock returns the per-cluster submission lock so "check running + create" is atomic per cluster.
func (s *ScaleService) clusterLock(clusterID uint) *sync.Mutex {
	s.locksMu.Lock()
	defer s.locksMu.Unlock()
	lock, ok := s.clusterLocks[clusterID]
	if !ok {
		lock = &sync.Mutex{}
		s.clusterLocks[clusterID] = lock
	}
	return lock
}

// ensureRunnable 检查执行所需的最小依赖。
// ensureRunnable checks the minimum dependencies required to execute.
func (s *ScaleService) ensureRunnable() error {
	if s == nil || s.svc == nil || s.repo == nil {
		return ErrScaleNotConfigured
	}
	if s.svc.hostProvider == nil || s.svc.agentSender == nil {
		return fmt.Errorf("%w: agent command sender or host provider is missing", ErrScaleNotConfigured)
	}
	if s.config == nil {
		return fmt.Errorf("%w: config service is missing", ErrScaleNotConfigured)
	}
	return nil
}

// SubmitScaleOut 校验 R2 确认与幂等后重算计划，计划指纹一致、无阻断项且确认齐全时创建扩容操作并异步执行。
// SubmitScaleOut validates R2 confirmation and idempotency, recomputes the plan and, when the fingerprint
// matches, nothing blocks and acknowledgements are complete, creates the scale-out operation and runs it
// asynchronously.
func (s *ScaleService) SubmitScaleOut(ctx context.Context, actor executionapp.Actor, clusterID uint, req *ScaleOutSubmitRequest, meta ScaleExecutionRequest) (*ScaleSubmitResult, error) {
	if req == nil {
		return nil, ErrScaleInvalidRequest
	}
	if err := s.ensureRunnable(); err != nil {
		return nil, err
	}
	lang := normalizeScaleLang(meta.Lang)
	return s.submit(ctx, actor, clusterID, meta, scaleSubmitSpec{
		operationID: scaleOutOperationID,
		impact:      "扩容会在目标主机安装或纳管 SeaTunnel，修改集群成员配置并推送到所有节点；存在回环成员地址时会逐个重启现有节点。",
		buildPlan: func(ctx context.Context) (*ScalePlan, error) {
			return s.PlanScaleOut(ctx, clusterID, &req.ScaleOutRequest, lang)
		},
		fingerprint:        req.PlanFingerprint,
		loopbackRestartAck: req.LoopbackRestartAck,
	})
}

// SubmitScaleIn 校验 R2 确认与幂等后重算缩容计划，满足护栏时创建缩容操作并异步执行。
// SubmitScaleIn validates R2 confirmation and idempotency, recomputes the scale-in plan and, when the guardrails
// pass, creates the scale-in operation and runs it asynchronously.
func (s *ScaleService) SubmitScaleIn(ctx context.Context, actor executionapp.Actor, clusterID uint, nodeID uint, req *ScaleInSubmitRequest, meta ScaleExecutionRequest) (*ScaleSubmitResult, error) {
	if req == nil {
		return nil, ErrScaleInvalidRequest
	}
	if err := s.ensureRunnable(); err != nil {
		return nil, err
	}
	lang := normalizeScaleLang(meta.Lang)
	return s.submit(ctx, actor, clusterID, meta, scaleSubmitSpec{
		operationID: scaleInOperationID,
		impact:      "缩容会停止目标节点进程、修改剩余节点的成员配置并删除节点记录；运行中的作业可能 failover 到其他节点。",
		buildPlan: func(ctx context.Context) (*ScalePlan, error) {
			return s.PlanScaleIn(ctx, clusterID, nodeID, lang)
		},
		fingerprint: req.PlanFingerprint,
		jobsRiskAck: req.JobsRiskAck,
	})
}

// ScaleInProgressError 描述集群上正在进行的伸缩操作，用于 409 响应告知调用方是谁在操作。
// 它包装 ErrClusterScaleInProgress，errors.Is 判断保持不变。
// ScaleInProgressError describes the scaling operation already running on a cluster, so a 409 response can tell the
// caller who is running it. It wraps ErrClusterScaleInProgress, so errors.Is checks keep working.
type ScaleInProgressError struct {
	OperationID   uint      `json:"operation_id"`
	Kind          ScaleKind `json:"kind"`
	CreatedBy     uint      `json:"created_by"`
	CreatedByName string    `json:"created_by_name"`
}

// newScaleInProgressError 由进行中的操作构造冲突错误；操作为空时退化为哨兵错误。
// newScaleInProgressError builds the conflict error from the running operation; falls back to the sentinel when nil.
func newScaleInProgressError(running *ClusterScaleOperation) error {
	if running == nil {
		return ErrClusterScaleInProgress
	}
	return &ScaleInProgressError{
		OperationID:   running.ID,
		Kind:          running.Kind,
		CreatedBy:     running.CreatedBy,
		CreatedByName: running.CreatedByName,
	}
}

// Error 返回中英双语说明；拿不到用户名时显示用户 ID。
// Error returns a bilingual message; the user ID is shown when the login name is unknown.
func (e *ScaleInProgressError) Error() string {
	who := strings.TrimSpace(e.CreatedByName)
	if who == "" {
		who = fmt.Sprintf("user#%d", e.CreatedBy)
	}
	return fmt.Sprintf("集群已有进行中的伸缩操作 #%d（%s，发起人 %s），请等待其结束 / another scaling operation #%d (%s, started by %s) is running on this cluster; wait for it to finish",
		e.OperationID, e.Kind, who, e.OperationID, e.Kind, who)
}

// Unwrap 让 errors.Is(err, ErrClusterScaleInProgress) 继续成立。
// Unwrap keeps errors.Is(err, ErrClusterScaleInProgress) true.
func (e *ScaleInProgressError) Unwrap() error {
	return ErrClusterScaleInProgress
}

// scaleSubmitSpec 描述一次提交的差异部分。
// scaleSubmitSpec describes the varying parts of one submission.
type scaleSubmitSpec struct {
	operationID        string
	impact             string
	buildPlan          func(context.Context) (*ScalePlan, error)
	fingerprint        string
	loopbackRestartAck bool
	jobsRiskAck        bool
}

func (s *ScaleService) submit(ctx context.Context, actor executionapp.Actor, clusterID uint, meta ScaleExecutionRequest, spec scaleSubmitSpec) (*ScaleSubmitResult, error) {
	if s.execution != nil {
		existing, found, err := s.execution.FindIdempotent(ctx, actor, spec.operationID, meta.IdempotencyKey, meta.RequestHash)
		if err != nil {
			return nil, err
		}
		if found {
			return s.resultForExecution(ctx, existing)
		}
		if err := s.execution.Authorize(ctx, actor, executionapp.AuthorizationInput{
			OperationID:    spec.operationID,
			RiskLevel:      executionapp.RiskLevelR2,
			Impact:         spec.impact,
			IdempotencyKey: strings.TrimSpace(meta.IdempotencyKey),
			RequestHash:    strings.TrimSpace(meta.RequestHash),
			Confirmed:      meta.Confirmed,
			ConfirmationID: strings.TrimSpace(meta.ConfirmationID),
		}); err != nil {
			return nil, err
		}
	}

	lock := s.clusterLock(clusterID)
	lock.Lock()
	defer lock.Unlock()

	if running, err := s.repo.FindRunningByCluster(ctx, clusterID); err != nil {
		return nil, err
	} else if running != nil {
		return nil, newScaleInProgressError(running)
	}

	plan, err := spec.buildPlan(ctx)
	if err != nil {
		return nil, err
	}
	if err := validateSubmittedPlan(plan, spec.fingerprint, spec.loopbackRestartAck, spec.jobsRiskAck); err != nil {
		return nil, err
	}

	var item *executionapp.Execution
	if s.execution != nil {
		created, _, err := s.execution.Create(ctx, executionapp.CreateInput{
			OperationID:       spec.operationID,
			OwnerUserID:       actor.UserID,
			ActorType:         executionapp.ActorTypeUser,
			Module:            scaleExecutionModule,
			RequestID:         strings.TrimSpace(meta.RequestID),
			IdempotencyKey:    strings.TrimSpace(meta.IdempotencyKey),
			RequestHash:       strings.TrimSpace(meta.RequestHash),
			RiskLevel:         executionapp.RiskLevelR2,
			Status:            executionapp.StatusPending,
			Cancellable:       false,
			CancellableReason: scaleNotCancellableReason,
			ClientType:        strings.TrimSpace(meta.ClientType),
		})
		if err != nil {
			return nil, err
		}
		item = created
	}

	op := &ClusterScaleOperation{
		ClusterID:          clusterID,
		HostID:             plan.Target.HostID,
		Kind:               plan.Kind,
		Mode:               plan.Mode,
		Status:             ScaleOperationRunning,
		CurrentStep:        ScaleStepValidatePlan,
		LoopbackRestartAck: spec.loopbackRestartAck,
		JobsRiskAck:        spec.jobsRiskAck,
		Lang:               plan.Lang,
		CreatedBy:          uint(actor.UserID),
		CreatedByName:      strings.TrimSpace(meta.Username),
	}
	if item != nil {
		op.ExecutionID = item.ExecutionID
	}
	if plan.Kind == ScaleKindIn && plan.Target.NodeID > 0 {
		op.setNodeIDs([]uint{plan.Target.NodeID})
	} else {
		op.setNodeIDs(nil)
	}
	planJSON, err := json.Marshal(plan)
	if err != nil {
		return nil, err
	}
	op.PlanJSON = db.ScriptText(planJSON)
	steps := make([]ScaleStepState, 0, len(plan.Steps))
	for _, name := range plan.Steps {
		steps = append(steps, ScaleStepState{Name: name, Status: ScaleStepPending})
	}
	op.setSteps(steps)
	if err := s.repo.Create(ctx, op); err != nil {
		s.failExecution(ctx, item, "scale_operation_create_failed", err)
		return nil, err
	}
	if err := s.activateExecution(ctx, op, item); err != nil {
		return nil, err
	}

	logger.InfoF(ctx, "[ClusterScale] 伸缩操作已提交 / Scaling operation submitted: cluster=%d, op=%d, kind=%s, mode=%s, execution=%s",
		clusterID, op.ID, op.Kind, op.Mode, op.ExecutionID)
	s.start(op, meta, actor)
	return &ScaleSubmitResult{ExecutionID: op.ExecutionID, OperationID: op.ID, Operation: op.toInfo()}, nil
}

// validateSubmittedPlan 对比客户端指纹并复核阻断项与显式确认。
// validateSubmittedPlan compares the client fingerprint and re-checks blocking items and acknowledgements.
func validateSubmittedPlan(plan *ScalePlan, fingerprint string, loopbackRestartAck, jobsRiskAck bool) error {
	if plan == nil {
		return ErrScaleInvalidRequest
	}
	if strings.TrimSpace(fingerprint) != plan.Fingerprint {
		return ErrScalePlanChanged
	}
	if !plan.CanSubmit {
		return fmt.Errorf("%w: %s", ErrScalePlanBlocked, strings.Join(plan.BlockingReasons, "; "))
	}
	if plan.RequiresRestartAck && !loopbackRestartAck {
		return ErrScaleRestartAckRequired
	}
	if plan.RequiresJobsAck && !jobsRiskAck {
		return ErrScaleJobsRiskAckRequired
	}
	return nil
}

// RetryOperation 从失败步骤重试伸缩操作（R1）：创建新的公共执行记录并沿用原计划快照，已完成的步骤不会重跑。
// RetryOperation retries a scaling operation from the failed step (R1): a new shared execution is created and the
// original plan snapshot is reused; completed steps are not re-run.
func (s *ScaleService) RetryOperation(ctx context.Context, actor executionapp.Actor, clusterID uint, operationID uint, meta ScaleExecutionRequest) (*ScaleSubmitResult, error) {
	if err := s.ensureRunnable(); err != nil {
		return nil, err
	}
	if s.execution != nil {
		existing, found, err := s.execution.FindIdempotent(ctx, actor, scaleRetryOperationID, meta.IdempotencyKey, meta.RequestHash)
		if err != nil {
			return nil, err
		}
		if found {
			return s.resultForExecution(ctx, existing)
		}
		if err := s.execution.Authorize(ctx, actor, executionapp.AuthorizationInput{
			OperationID:    scaleRetryOperationID,
			RiskLevel:      executionapp.RiskLevelR1,
			Impact:         "重试会从失败步骤继续执行伸缩操作，可能再次修改节点配置、启动或停止进程。",
			IdempotencyKey: strings.TrimSpace(meta.IdempotencyKey),
			RequestHash:    strings.TrimSpace(meta.RequestHash),
			Confirmed:      meta.Confirmed,
			ConfirmationID: strings.TrimSpace(meta.ConfirmationID),
		}); err != nil {
			return nil, err
		}
	}
	if _, err := s.repo.GetForOwner(ctx, clusterID, operationID, uint(actor.UserID), actor.IsAdmin); err != nil {
		return nil, err
	}

	lock := s.clusterLock(clusterID)
	lock.Lock()
	defer lock.Unlock()

	op, err := s.repo.GetByID(ctx, operationID)
	if err != nil {
		return nil, err
	}
	if op.Status != ScaleOperationFailed {
		return nil, ErrScaleOperationNotRetryable
	}
	if running, err := s.repo.FindRunningByCluster(ctx, clusterID); err != nil {
		return nil, err
	} else if running != nil {
		return nil, newScaleInProgressError(running)
	}

	var item *executionapp.Execution
	if s.execution != nil {
		created, _, err := s.execution.Create(ctx, executionapp.CreateInput{
			OperationID:       scaleRetryOperationID,
			OwnerUserID:       actor.UserID,
			ActorType:         executionapp.ActorTypeUser,
			Module:            scaleExecutionModule,
			RequestID:         strings.TrimSpace(meta.RequestID),
			IdempotencyKey:    strings.TrimSpace(meta.IdempotencyKey),
			RequestHash:       strings.TrimSpace(meta.RequestHash),
			RiskLevel:         executionapp.RiskLevelR1,
			Status:            executionapp.StatusPending,
			Cancellable:       false,
			CancellableReason: scaleNotCancellableReason,
			ClientType:        strings.TrimSpace(meta.ClientType),
		})
		if err != nil {
			return nil, err
		}
		item = created
		op.ExecutionID = item.ExecutionID
	}

	// 失败步骤恢复为待执行，保留尝试次数；已完成步骤保持原状。
	// The failed step goes back to pending while keeping its attempt count; completed steps stay as they are.
	steps := op.steps()
	for i := range steps {
		if !steps[i].Status.isDone() {
			steps[i].Status = ScaleStepPending
			steps[i].Error = ""
		}
	}
	op.setSteps(steps)
	op.Status = ScaleOperationRunning
	op.Error = ""
	op.FinishedAt = nil
	if lang := strings.TrimSpace(meta.Lang); lang != "" {
		op.Lang = normalizeScaleLang(lang)
	}
	if err := s.repo.Save(ctx, op); err != nil {
		s.failExecution(ctx, item, "scale_operation_retry_failed", err)
		return nil, err
	}
	if err := s.activateExecution(ctx, op, item); err != nil {
		return nil, err
	}
	logger.InfoF(ctx, "[ClusterScale] 伸缩操作重试 / Scaling operation retried: cluster=%d, op=%d, execution=%s", clusterID, op.ID, op.ExecutionID)
	s.start(op, meta, actor)
	return &ScaleSubmitResult{ExecutionID: op.ExecutionID, OperationID: op.ID, Operation: op.toInfo()}, nil
}

// activateExecution 绑定执行记录与伸缩操作并迁移为 running；失败时把操作与执行记录都标记失败，避免操作永久停在 running。
// activateExecution binds the execution to the operation and moves it to running; on failure both the operation and
// the execution are marked failed so the operation never stays running forever.
func (s *ScaleService) activateExecution(ctx context.Context, op *ClusterScaleOperation, item *executionapp.Execution) error {
	if item == nil {
		return nil
	}
	err := s.execution.BindModuleRef(ctx, item.ExecutionID, scaleModuleRef(op.ID))
	if err == nil {
		err = s.execution.Transition(ctx, item.ExecutionID, executionapp.StatusPending, executionapp.StatusRunning, nil)
	}
	if err == nil {
		return nil
	}
	now := s.now()
	op.Status = ScaleOperationFailed
	op.Error = truncateScaleText(err.Error(), 2000)
	op.FinishedAt = &now
	if saveErr := s.repo.Save(ctx, op); saveErr != nil {
		logger.WarnF(ctx, "[ClusterScale] 标记伸缩操作失败时出错 / Failed to mark scaling operation failed: op=%d, err=%v", op.ID, saveErr)
	}
	s.failExecution(ctx, item, "scale_operation_start_failed", err)
	return err
}

// start 用脱离请求的上下文启动后台执行，并恢复请求编号、执行编号与客户端来源供 Agent 命令审计关联。
// start launches background execution with a request-independent context and restores the request ID, execution
// ID and client type so Agent command audits stay linked.
func (s *ScaleService) start(op *ClusterScaleOperation, meta ScaleExecutionRequest, actor executionapp.Actor) {
	bgCtx := audit.WithCommandMetadata(context.Background(), audit.CommandMetadata{
		RequestID:   strings.TrimSpace(meta.RequestID),
		ExecutionID: op.ExecutionID,
		OwnerUserID: uint(actor.UserID),
		ClientType:  strings.TrimSpace(meta.ClientType),
	})
	operationID := op.ID
	s.launch(func() {
		s.runOperation(bgCtx, operationID)
	})
}

// GetOperation 在归属条件下读取伸缩操作详情。
// GetOperation loads a scaling operation detail under ownership scoping.
func (s *ScaleService) GetOperation(ctx context.Context, actor executionapp.Actor, clusterID uint, operationID uint) (*ScaleOperationInfo, error) {
	op, err := s.repo.GetForOwner(ctx, clusterID, operationID, uint(actor.UserID), actor.IsAdmin)
	if err != nil {
		return nil, err
	}
	return op.toInfo(), nil
}

// ListOperations 在归属条件下列出集群伸缩操作（最新在前），结果永不为 nil。
// ListOperations lists a cluster's scaling operations under ownership scoping (newest first); never nil.
func (s *ScaleService) ListOperations(ctx context.Context, actor executionapp.Actor, clusterID uint, status ScaleOperationStatus, limit int) ([]*ScaleOperationInfo, error) {
	ops, err := s.repo.List(ctx, ScaleOperationFilter{
		ClusterID:   clusterID,
		Status:      status,
		OwnerUserID: uint(actor.UserID),
		IncludeAll:  actor.IsAdmin,
		Limit:       limit,
	})
	if err != nil {
		return nil, err
	}
	infos := make([]*ScaleOperationInfo, 0, len(ops))
	for _, op := range ops {
		infos = append(infos, op.toInfo())
	}
	return infos, nil
}

// RecoverInterruptedOperations 在控制面启动时把仍为 running 的伸缩操作标记为失败（interrupted_by_restart），
// 运行中的步骤记为失败，公共执行记录同步失败；用户可从失败步骤重试。
// RecoverInterruptedOperations marks scaling operations still running at Control Plane startup as failed
// (interrupted_by_restart); the running step is marked failed and the shared execution fails too, so the user can
// retry from the failed step.
func (s *ScaleService) RecoverInterruptedOperations(ctx context.Context) (int, error) {
	if s == nil || s.repo == nil {
		return 0, nil
	}
	ops, err := s.repo.ListRunning(ctx)
	if err != nil {
		return 0, err
	}
	recovered := 0
	for _, op := range ops {
		now := s.now()
		steps := op.steps()
		for i := range steps {
			if steps[i].Status == ScaleStepRunning {
				steps[i].Status = ScaleStepFailed
				steps[i].Error = scaleErrorInterruptedByRestart
				steps[i].FinishedAt = &now
			}
		}
		op.setSteps(steps)
		op.Status = ScaleOperationFailed
		op.Error = scaleErrorInterruptedByRestart
		op.FinishedAt = &now
		if err := s.repo.Save(ctx, op); err != nil {
			logger.WarnF(ctx, "[ClusterScale] 标记被打断的伸缩操作失败 / Failed to mark interrupted scaling operation: op=%d, err=%v", op.ID, err)
			continue
		}
		s.finishExecution(ctx, op, errors.New(scaleErrorInterruptedByRestart))
		recovered++
	}
	if recovered > 0 {
		logger.WarnF(ctx, "[ClusterScale] 控制面重启打断了 %d 个伸缩操作，已标记失败可重试 / %d scaling operation(s) interrupted by restart were marked failed and can be retried", recovered, recovered)
	}
	return recovered, nil
}

// resultForExecution 用幂等命中的执行记录还原提交结果。
// resultForExecution rebuilds the submission result from an idempotent execution hit.
func (s *ScaleService) resultForExecution(ctx context.Context, item *executionapp.Execution) (*ScaleSubmitResult, error) {
	if item == nil {
		return nil, executionapp.ErrConcurrentUpdate
	}
	var operationID uint
	if _, err := fmt.Sscanf(strings.TrimSpace(item.ModuleRef), "scale:%d", &operationID); err != nil || operationID == 0 {
		return nil, executionapp.ErrConcurrentUpdate
	}
	op, err := s.repo.GetByID(ctx, operationID)
	if err != nil {
		return nil, err
	}
	return &ScaleSubmitResult{ExecutionID: item.ExecutionID, OperationID: op.ID, Operation: op.toInfo()}, nil
}

// finishExecution 把伸缩操作终态同步到公共执行记录（尽力而为，失败只记日志）。
// finishExecution syncs the operation's terminal state to the shared execution (best effort; failures are logged).
func (s *ScaleService) finishExecution(ctx context.Context, op *ClusterScaleOperation, runErr error) {
	if s.execution == nil || op == nil || strings.TrimSpace(op.ExecutionID) == "" {
		return
	}
	target := executionapp.StatusSucceeded
	updates := map[string]any{
		"result_ref":         scaleResultRef(op.ClusterID, op.ID),
		"cancellable":        false,
		"cancellable_reason": "completed",
		"progress":           100,
	}
	if runErr != nil {
		target = executionapp.StatusFailed
		updates["error_code"] = "scale_operation_failed"
		updates["error_message"] = truncateScaleText(runErr.Error(), 1000)
		delete(updates, "progress")
	}
	if err := s.execution.Transition(ctx, op.ExecutionID, executionapp.StatusRunning, target, updates); err != nil {
		logger.WarnF(ctx, "[ClusterScale] 同步执行记录终态失败 / Failed to finish execution: op=%d, execution=%s, err=%v", op.ID, op.ExecutionID, err)
	}
}

// failExecution 在操作尚未创建成功时把待执行的执行记录标记失败。
// failExecution fails a pending execution when the operation could not be created.
func (s *ScaleService) failExecution(ctx context.Context, item *executionapp.Execution, code string, cause error) {
	if s.execution == nil || item == nil {
		return
	}
	_ = s.execution.Transition(ctx, item.ExecutionID, executionapp.StatusPending, executionapp.StatusFailed, map[string]any{
		"cancellable":   false,
		"error_code":    code,
		"error_message": truncateScaleText(errorString(cause), 1000),
	})
}

func scaleModuleRef(operationID uint) string {
	return fmt.Sprintf("scale:%d", operationID)
}

func scaleResultRef(clusterID, operationID uint) string {
	return fmt.Sprintf("clusters/%d/scale-operations/%d", clusterID, operationID)
}

// truncateScaleText 按字符截断文本。
// truncateScaleText truncates text by runes.
func truncateScaleText(text string, maxRunes int) string {
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes]) + "..."
}
