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

package selfupgrade

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/LeonYoah/stx/internal/config"
	stxversion "github.com/LeonYoah/stx/internal/version"
	"github.com/google/uuid"
)

// DefaultAssetCatalog returns the release assets this console upgrade expects.
// DefaultAssetCatalog 返回控制台自升级期望的发版资产清单。
func DefaultAssetCatalog() []AssetSpec {
	return []AssetSpec{
		{ID: "stx", Name: fmt.Sprintf("stx-%s-%s", runtimeGOOS(), runtimeGOARCH()), Description: "控制面二进制 / Control plane binary", Required: true},
		{ID: "agent", Name: fmt.Sprintf("stx-agent-%s-%s", runtimeGOOS(), runtimeGOARCH()), Description: "Agent 二进制 / Agent binary", Required: true},
		{ID: "frontend", Name: "frontend-standalone-*.tar.gz", Description: "前端静态包 / Frontend standalone", Required: false, Glob: true},
		{ID: "proxy", Name: "stx-java-proxy-*.jar", Description: "全部代际 Proxy jar（glob）/ All-epoch Proxy jars", Required: true, Glob: true},
		{ID: "proxy_sh", Name: "stx-java-proxy.sh", Description: "Proxy 封装脚本 / Proxy launcher", Required: true},
		{ID: "sums", Name: "SHA256SUMS", Description: "校验清单 / Checksum manifest", Required: true},
	}
}

func runtimeGOOS() string {
	if v := os.Getenv("STX_SELF_UPGRADE_GOOS"); v != "" {
		return v
	}
	return runtime.GOOS
}

func runtimeGOARCH() string {
	if v := os.Getenv("STX_SELF_UPGRADE_GOARCH"); v != "" {
		return v
	}
	return runtime.GOARCH
}

// defaultGHProxyNodes defines GitHub reverse proxy mirrors for environments with restricted direct GitHub access.
// defaultGHProxyNodes 定义在中国内网等无法直连 GitHub 时使用的 gh-proxy 镜像节点池。
var defaultGHProxyNodes = []string{
	"https://v4.gh-proxy.org",
	"https://gh-proxy.org",
	"https://v6.gh-proxy.org",
	"https://cdn.gh-proxy.org",
	"https://axisnow.gh-proxy.org",
	"https://gh-proxy.com",
}

// IsRunningInDocker detects if the current process is running inside a Docker/container environment.
// IsRunningInDocker 检测当前进程是否运行在 Docker/容器环境中。
func IsRunningInDocker() bool {
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	if os.Getenv("STX_RUNTIME_MODE") == "docker" || os.Getenv("DOCKER_CONTAINER") == "true" {
		return true
	}
	if data, err := os.ReadFile("/proc/1/cgroup"); err == nil {
		content := string(data)
		if strings.Contains(content, "docker") || strings.Contains(content, "containerd") || strings.Contains(content, "kubepods") {
			return true
		}
	}
	return false
}

// Service orchestrates STX console self-upgrade. / Service 编排 STX 控制台自升级。
type Service struct {
	mu sync.Mutex

	stagingDir string
	httpClient *http.Client
	mockMode   bool
	allowSim   bool

	checked         bool
	updateAvailable bool
	latestVersion   string
	releaseURL      string
	lastCheck       *CheckResult

	stage   StageState
	task    *TaskState
	cancelF context.CancelFunc
}

// NewService creates a self-upgrade service rooted at dataDir/self-upgrade.
// NewService 在 dataDir/self-upgrade 下创建自升级服务。
func NewService(dataDir string) *Service {
	if dataDir == "" {
		dataDir = "./data"
	}
	staging := filepath.Join(dataDir, "self-upgrade")
	_ = os.MkdirAll(staging, 0o755)

	env := ""
	if config.Config.App.Env != "" {
		env = config.Config.App.Env
	}
	mock := os.Getenv("STX_SELF_UPGRADE_MOCK") == "1" || env == "development" || env == ""
	allowSim := env != "production"

	s := &Service{
		stagingDir: staging,
		httpClient: &http.Client{Timeout: 20 * time.Second},
		mockMode:   mock,
		allowSim:   allowSim,
		stage: StageState{
			Status:      StatusIdle,
			Assets:      initialAssets(),
			Cancellable: false,
		},
	}
	return s
}

func initialAssets() []AssetState {
	specs := DefaultAssetCatalog()
	out := make([]AssetState, 0, len(specs))
	for _, spec := range specs {
		out = append(out, AssetState{AssetSpec: spec, Status: AssetPending})
	}
	return out
}

// Status returns the aggregate console status. / 返回聚合状态。
func (s *Service) Status() StatusResponse {
	s.mu.Lock()
	defer s.mu.Unlock()
	info := stxversion.Current()
	overall := StatusIdle
	if s.task != nil {
		overall = s.task.Status
	} else if s.stage.Status == StatusStaged {
		overall = StatusStaged
	} else if s.stage.Status == StatusStaging || s.stage.Status == StatusFailed {
		overall = s.stage.Status
	} else if s.checked {
		if s.updateAvailable {
			overall = StatusAvailable
		} else {
			overall = StatusUpToDate
		}
	}
	deploymentType := "host"
	dockerCmd := ""
	if IsRunningInDocker() {
		deploymentType = "docker"
		dockerCmd = "docker compose pull && docker compose up -d"
	}
	arch := fmt.Sprintf("%s/%s", runtimeGOOS(), runtimeGOARCH())
	resp := StatusResponse{
		CurrentVersion:          info.Version,
		GitCommit:               info.GitCommit,
		BuildTime:               info.BuildTime,
		LatestVersion:           s.latestVersion,
		UpdateAvailable:         s.updateAvailable,
		Checked:                 s.checked,
		OverallStatus:           overall,
		Stage:                   cloneStage(s.stage),
		LastCheck:               s.lastCheck,
		MockMode:                s.mockMode,
		DeploymentType:          deploymentType,
		DockerComposeUpgradeCmd: dockerCmd,
		Architecture:            arch,
	}
	if s.task != nil {
		cp := *s.task
		resp.ActiveTask = &cp
	}
	return resp
}

// Check probes for a newer release. / 检查是否有新版本。
func (s *Service) Check(ctx context.Context, req CheckRequest) (*CheckResult, error) {
	if err := s.beginExclusive("check"); err != nil {
		return nil, err
	}
	defer s.endExclusive()

	if s.allowSim {
		switch req.SimulateFailure {
		case CodeCheckFail:
			result := &CheckResult{
				CurrentVersion: stxversion.Current().Version,
				CheckedAt:      time.Now().UTC().Format(time.RFC3339),
				ErrorCode:      CodeCheckFail,
				Message:        "failed to reach release metadata",
			}
			s.mu.Lock()
			s.lastCheck = result
			s.checked = true
			s.updateAvailable = false
			s.mu.Unlock()
			return result, ErrCheckFail
		case CodeAlreadyLatest:
			cur := stxversion.Current().Version
			result := &CheckResult{
				CurrentVersion:  cur,
				LatestVersion:   cur,
				UpdateAvailable: false,
				CheckedAt:       time.Now().UTC().Format(time.RFC3339),
				Message:         "already latest",
				ErrorCode:       CodeAlreadyLatest,
			}
			s.mu.Lock()
			s.checked = true
			s.updateAvailable = false
			s.latestVersion = cur
			s.lastCheck = result
			s.mu.Unlock()
			return result, nil
		}
	}

	cur := stxversion.Current().Version
	latest, url, err := s.resolveLatest(ctx, req.ForceLatest)
	if err != nil {
		result := &CheckResult{
			CurrentVersion: cur,
			CheckedAt:      time.Now().UTC().Format(time.RFC3339),
			ErrorCode:      CodeCheckFail,
			Message:        err.Error(),
		}
		s.mu.Lock()
		s.lastCheck = result
		s.checked = true
		s.updateAvailable = false
		s.mu.Unlock()
		return result, fmt.Errorf("%w: %v", ErrCheckFail, err)
	}

	available := stxversion.Compare(cur, latest) < 0
	result := &CheckResult{
		CurrentVersion:  cur,
		LatestVersion:   latest,
		UpdateAvailable: available,
		ReleaseURL:      url,
		CheckedAt:       time.Now().UTC().Format(time.RFC3339),
	}
	if !available {
		result.Message = "already latest"
		result.ErrorCode = CodeAlreadyLatest
	}

	s.mu.Lock()
	s.checked = true
	s.updateAvailable = available
	s.latestVersion = latest
	s.releaseURL = url
	s.lastCheck = result
	// Reset stage when a new check finds a different target.
	if available {
		s.stage = StageState{Status: StatusIdle, Assets: initialAssets(), TargetVersion: latest}
	}
	s.mu.Unlock()
	return result, nil
}

func (s *Service) resolveLatest(ctx context.Context, force string) (string, string, error) {
	if force != "" {
		v := stxversion.Normalize(force)
		return v, fmt.Sprintf("https://github.com/LeonYoah/stx/releases/tag/v%s", v), nil
	}
	if s.mockMode {
		cur := stxversion.Current().Version
		parts := strings.Split(cur, ".")
		if len(parts) >= 3 {
			// bump patch for demo: 1.0.0 -> 1.0.1
			var patch int
			fmt.Sscanf(parts[2], "%d", &patch)
			latest := fmt.Sprintf("%s.%s.%d", parts[0], parts[1], patch+1)
			return latest, fmt.Sprintf("https://github.com/LeonYoah/stx/releases/tag/v%s", latest), nil
		}
		return "1.0.1", "https://github.com/LeonYoah/stx/releases/tag/v1.0.1", nil
	}

	owner := envOr("STX_GITHUB_OWNER", "LeonYoah")
	repo := envOr("STX_GITHUB_REPO", "stx")
	api := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", owner, repo)

	// 1. 直连 GitHub API（带超时限制）/ 1. Direct GitHub API with timeout
	directCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(directCtx, http.MethodGet, api, nil)
	if err == nil {
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("User-Agent", "stx-self-upgrade")
		resp, doErr := s.httpClient.Do(req)
		if doErr == nil && resp.StatusCode < 300 {
			defer resp.Body.Close()
			var payload struct {
				TagName string `json:"tag_name"`
				HTMLURL string `json:"html_url"`
			}
			if decErr := json.NewDecoder(resp.Body).Decode(&payload); decErr == nil && payload.TagName != "" {
				return stxversion.Normalize(payload.TagName), payload.HTMLURL, nil
			}
		} else if resp != nil {
			resp.Body.Close()
		}
	}

	// 2. 直连超时或失败时，依次探测 gh-proxy 镜像节点 / 2. Fallback to probe gh-proxy mirror nodes
	for _, node := range defaultGHProxyNodes {
		mirrorURL := fmt.Sprintf("%s/https://github.com/%s/%s/releases/latest", strings.TrimRight(node, "/"), owner, repo)
		proxyCtx, pCancel := context.WithTimeout(ctx, 4*time.Second)
		pReq, pErr := http.NewRequestWithContext(proxyCtx, http.MethodHead, mirrorURL, nil)
		if pErr == nil {
			pClient := &http.Client{
				Timeout: 4 * time.Second,
				CheckRedirect: func(req *http.Request, via []*http.Request) error {
					return http.ErrUseLastResponse
				},
			}
			pResp, pDoErr := pClient.Do(pReq)
			if pDoErr == nil {
				loc := pResp.Header.Get("Location")
				pResp.Body.Close()
				pCancel()
				if loc != "" && strings.Contains(loc, "/tag/") {
					parts := strings.Split(loc, "/tag/")
					tag := parts[len(parts)-1]
					return stxversion.Normalize(tag), fmt.Sprintf("https://github.com/%s/%s/releases/tag/%s", owner, repo, tag), nil
				}
			} else {
				pCancel()
			}
		} else {
			pCancel()
		}
	}

	return "", "", fmt.Errorf("%w: direct GitHub and gh-proxy mirrors are all unreachable", ErrNetworkUnreachable)
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// StartDownload stages assets from the network (or mock). / 联网预置资产。
func (s *Service) StartDownload(ctx context.Context, req StageDownloadRequest) (*TaskState, error) {
	s.mu.Lock()
	if !s.checked || !s.updateAvailable {
		s.mu.Unlock()
		if s.checked && !s.updateAvailable {
			return nil, ErrAlreadyLatest
		}
		return nil, fmt.Errorf("%w: check for updates first", ErrInvalid())
	}
	if s.stage.Status == StatusStaging || (s.task != nil && (s.task.Status == StatusStaging || s.task.Status == StatusUpgrading)) {
		s.mu.Unlock()
		return nil, ErrConcurrentUpgrade
	}
	target := req.TargetVersion
	if target == "" {
		target = s.latestVersion
	}
	sim := ""
	if s.allowSim {
		sim = req.SimulateFailure
	}
	if sim == CodeDiskFull {
		s.mu.Unlock()
		return nil, ErrDiskFull
	}
	if sim == CodeConcurrentUpgrade {
		s.mu.Unlock()
		return nil, ErrConcurrentUpgrade
	}

	taskID := uuid.NewString()
	now := time.Now().UTC()
	task := &TaskState{
		ID:            taskID,
		Kind:          "stage",
		Status:        StatusStaging,
		TargetVersion: target,
		Cancellable:   true,
		StartedAt:     &now,
	}
	s.task = task
	s.stage = StageState{
		Mode:          ModeOnline,
		Status:        StatusStaging,
		Progress:      0,
		TargetVersion: target,
		Assets:        initialAssets(),
		Cancellable:   true,
		Message:       "downloading release assets",
	}
	runCtx, cancel := context.WithCancel(context.Background())
	s.cancelF = cancel
	s.mu.Unlock()

	go s.runDownload(runCtx, taskID, target, sim)
	cp := *task
	return &cp, nil
}

func ErrInvalid() error {
	return fmt.Errorf("%s", CodeInvalidRequest)
}

func (s *Service) runDownload(ctx context.Context, taskID, target, sim string) {
	assets := DefaultAssetCatalog()
	for i, spec := range assets {
		select {
		case <-ctx.Done():
			s.failStage(taskID, ErrCancelled, CodeCancelled, "staging cancelled")
			return
		default:
		}

		s.mu.Lock()
		if i < len(s.stage.Assets) {
			s.stage.Assets[i].Status = AssetLoading
			s.stage.Progress = int(float64(i) / float64(len(assets)) * 100)
			s.stage.Message = "downloading " + spec.Name
		}
		s.mu.Unlock()

		time.Sleep(180 * time.Millisecond)

		if sim == CodeDownloadPartial && i == len(assets)/2 {
			s.mu.Lock()
			if i < len(s.stage.Assets) {
				s.stage.Assets[i].Status = AssetFailed
				s.stage.Assets[i].Message = "connection reset while downloading"
			}
			s.mu.Unlock()
			s.failStage(taskID, ErrDownloadPartial, CodeDownloadPartial, "partial download failure on "+spec.Name)
			return
		}

		path, size, err := s.materializeAsset(target, spec)
		if err != nil {
			s.mu.Lock()
			if i < len(s.stage.Assets) {
				s.stage.Assets[i].Status = AssetFailed
				s.stage.Assets[i].Message = err.Error()
			}
			s.mu.Unlock()
			s.failStage(taskID, ErrDownloadPartial, CodeDownloadPartial, err.Error())
			return
		}

		s.mu.Lock()
		if i < len(s.stage.Assets) {
			s.stage.Assets[i].Status = AssetReady
			s.stage.Assets[i].Path = path
			s.stage.Assets[i].Size = size
			s.stage.Progress = int(float64(i+1) / float64(len(assets)) * 100)
		}
		s.mu.Unlock()
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.task == nil || s.task.ID != taskID {
		return
	}
	now := time.Now().UTC()
	s.stage.Status = StatusStaged
	s.stage.Progress = 100
	s.stage.Message = "assets staged in local cache"
	s.stage.Cancellable = false
	s.task.Status = StatusStaged
	s.task.Cancellable = false
	s.task.CompletedAt = &now
	s.cancelF = nil
}

func (s *Service) materializeAsset(target string, spec AssetSpec) (string, int64, error) {
	dir := filepath.Join(s.stagingDir, "v"+stxversion.Normalize(target))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		if isNoSpace(err) {
			return "", 0, ErrDiskFull
		}
		return "", 0, err
	}
	name := spec.Name
	if spec.Glob {
		switch spec.ID {
		case "proxy":
			name = "stx-java-proxy-v3.jar"
		case "frontend":
			name = fmt.Sprintf("frontend-standalone-v%s-%s-%s.tar.gz", stxversion.Normalize(target), runtimeGOOS(), runtimeGOARCH())
		}
	}
	path := filepath.Join(dir, name)
	content := fmt.Sprintf("stx-self-upgrade-stub version=%s asset=%s created=%s\n", target, name, time.Now().UTC().Format(time.RFC3339))
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		if isNoSpace(err) {
			return "", 0, ErrDiskFull
		}
		return "", 0, err
	}
	// Also drop a second proxy jar so glob matching is meaningful in mock mode.
	if spec.ID == "proxy" {
		_ = os.WriteFile(filepath.Join(dir, "stx-java-proxy-v2.jar"), []byte(content), 0o644)
	}
	fi, err := os.Stat(path)
	if err != nil {
		return path, int64(len(content)), nil
	}
	return path, fi.Size(), nil
}

func isNoSpace(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no space") || strings.Contains(msg, "disk quota")
}

// ImportOffline validates and stages an uploaded archive or asset set.
// ImportOffline 校验并预置上传的离线包。
func (s *Service) ImportOffline(filename string, r io.Reader, simulateFailure string) (*StageState, error) {
	s.mu.Lock()
	if s.stage.Status == StatusStaging || (s.task != nil && s.task.Status == StatusUpgrading) {
		s.mu.Unlock()
		return nil, ErrConcurrentUpgrade
	}
	if !s.checked || !s.updateAvailable {
		s.mu.Unlock()
		if s.checked && !s.updateAvailable {
			return nil, ErrAlreadyLatest
		}
		return nil, fmt.Errorf("%w: check for updates first", ErrInvalid())
	}
	target := s.latestVersion
	sim := ""
	if s.allowSim {
		sim = simulateFailure
	}
	s.mu.Unlock()

	if sim == CodeBadOfflineImport || filename == "" {
		return nil, ErrBadOfflineImport
	}
	if sim == CodeDiskFull {
		return nil, ErrDiskFull
	}

	// Read a limited prefix to bind the upload; real unpack can follow later.
	limited := io.LimitReader(r, 8<<20)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadOfflineImport, err)
	}
	if len(data) == 0 {
		return nil, ErrBadOfflineImport
	}
	lower := strings.ToLower(filename)
	if !strings.HasSuffix(lower, ".tar.gz") && !strings.HasSuffix(lower, ".tgz") && !strings.HasSuffix(lower, ".zip") {
		return nil, fmt.Errorf("%w: unsupported archive type", ErrBadOfflineImport)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	assets := initialAssets()
	for i := range assets {
		path, size, err := s.materializeAsset(target, assets[i].AssetSpec)
		if err != nil {
			return nil, err
		}
		assets[i].Status = AssetReady
		assets[i].Path = path
		assets[i].Size = size
	}
	s.stage = StageState{
		Mode:          ModeOffline,
		Status:        StatusStaged,
		Progress:      100,
		TargetVersion: target,
		Assets:        assets,
		Message:       "offline import written to local cache",
		Cancellable:   false,
	}
	now := time.Now().UTC()
	s.task = &TaskState{
		ID:            uuid.NewString(),
		Kind:          "stage",
		Status:        StatusStaged,
		TargetVersion: target,
		Cancellable:   false,
		StartedAt:     &now,
		CompletedAt:   &now,
	}
	cp := cloneStage(s.stage)
	return &cp, nil
}

// CancelStage cancels an in-flight staging operation. / 取消进行中的预置。
func (s *Service) CancelStage() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stage.Status != StatusStaging || s.cancelF == nil {
		return fmt.Errorf("%w: nothing cancellable", ErrInvalid())
	}
	s.cancelF()
	return nil
}

// Execute starts the upgrade using local staged packages only.
// Execute 仅使用本地预置包执行升级。
func (s *Service) Execute(ctx context.Context, req ExecuteRequest) (*TaskState, error) {
	s.mu.Lock()
	if s.stage.Status != StatusStaged {
		s.mu.Unlock()
		return nil, ErrNotStaged
	}
	if s.task != nil && s.task.Status == StatusUpgrading {
		s.mu.Unlock()
		return nil, ErrConcurrentUpgrade
	}
	sim := ""
	if s.allowSim {
		sim = req.SimulateFailure
	}
	if sim == CodeConcurrentUpgrade {
		s.mu.Unlock()
		return nil, ErrConcurrentUpgrade
	}

	target := s.stage.TargetVersion
	now := time.Now().UTC()
	taskID := uuid.NewString()
	steps := []StepState{
		{Code: StepControlPlane, Label: "控制面 / Control plane", Status: StepPending},
		{Code: StepAgent, Label: "Agent", Status: StepPending},
		{Code: StepProxy, Label: "Proxy (stx-java-proxy-*.jar)", Status: StepPending},
		{Code: StepVerify, Label: "版本校验 / Verify", Status: StepPending},
	}
	task := &TaskState{
		ID:            taskID,
		Kind:          "execute",
		Status:        StatusUpgrading,
		TargetVersion: target,
		CurrentStep:   StepControlPlane,
		Steps:         steps,
		Cancellable:   true,
		StartedAt:     &now,
	}
	s.task = task
	runCtx, cancel := context.WithCancel(context.Background())
	s.cancelF = cancel
	s.mu.Unlock()

	go s.runExecute(runCtx, taskID, target, sim, req.Apply)
	cp := *task
	return &cp, nil
}

func (s *Service) runExecute(ctx context.Context, taskID, target, sim string, apply bool) {
	order := []string{StepControlPlane, StepAgent, StepProxy, StepVerify}
	failMap := map[string]error{
		CodeStepFailControlPlane: ErrStepFailControlPlane,
		CodeStepFailAgent:        ErrStepFailAgent,
		CodeStepFailProxy:        ErrStepFailProxy,
		CodeVerifyFail:           ErrVerifyFail,
	}
	failStep := map[string]string{
		CodeStepFailControlPlane: StepControlPlane,
		CodeStepFailAgent:        StepAgent,
		CodeStepFailProxy:        StepProxy,
		CodeVerifyFail:           StepVerify,
	}

	for _, code := range order {
		select {
		case <-ctx.Done():
			s.failExecute(taskID, code, ErrCancelled, CodeCancelled, "upgrade cancelled")
			return
		default:
		}

		s.mu.Lock()
		if s.task == nil || s.task.ID != taskID {
			s.mu.Unlock()
			return
		}
		s.task.CurrentStep = code
		now := time.Now().UTC()
		for i := range s.task.Steps {
			if s.task.Steps[i].Code == code {
				s.task.Steps[i].Status = StepRunning
				s.task.Steps[i].StartedAt = &now
				s.task.Steps[i].Message = "applying " + code
			}
		}
		// After control plane starts, cancel becomes unsafe in MVP.
		if code == StepControlPlane {
			s.task.Cancellable = false
		}
		s.mu.Unlock()

		time.Sleep(220 * time.Millisecond)

		if want, ok := failStep[sim]; ok && want == code {
			s.failExecute(taskID, code, failMap[sim], sim, "simulated failure on "+code)
			return
		}

		if err := s.applyStep(target, code, apply); err != nil {
			s.failExecute(taskID, code, err, ErrorCode(err), err.Error())
			return
		}

		s.mu.Lock()
		end := time.Now().UTC()
		if s.task != nil && s.task.ID == taskID {
			for i := range s.task.Steps {
				if s.task.Steps[i].Code == code {
					s.task.Steps[i].Status = StepSucceeded
					s.task.Steps[i].EndedAt = &end
					s.task.Steps[i].Message = "ok"
				}
			}
		}
		s.mu.Unlock()
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.task == nil || s.task.ID != taskID {
		return
	}
	now := time.Now().UTC()
	s.task.Status = StatusSucceeded
	s.task.Cancellable = false
	s.task.CompletedAt = &now
	s.task.CurrentStep = ""
	s.updateAvailable = false
	s.latestVersion = target
	s.stage.Status = StatusSucceeded
	s.stage.Message = "upgrade completed (MVP dry-apply)"
	s.cancelF = nil
}

func (s *Service) applyStep(target, code string, apply bool) error {
	dir := filepath.Join(s.stagingDir, "v"+stxversion.Normalize(target))
	switch code {
	case StepControlPlane:
		if IsRunningInDocker() {
			// In Docker container, control plane upgrade is handled via container image pull & restart.
			// 在 Docker 容器环境中，控制面升级由拉取新镜像并重启容器负责，跳过容器内写二进制。
			return nil
		}
		matches, _ := filepath.Glob(filepath.Join(dir, "stx-*"))
		if len(matches) == 0 {
			return fmt.Errorf("%w: control plane package missing", ErrStepFailControlPlane)
		}
	case StepAgent:
		matches, _ := filepath.Glob(filepath.Join(dir, "stx-agent-*"))
		if len(matches) == 0 {
			return fmt.Errorf("%w: agent package missing", ErrStepFailAgent)
		}
	case StepProxy:
		matches, _ := filepath.Glob(filepath.Join(dir, "stx-java-proxy-*.jar"))
		if len(matches) == 0 {
			return fmt.Errorf("%w: no stx-java-proxy-*.jar in local cache", ErrStepFailProxy)
		}
	case StepVerify:
		// MVP: verify staged assets still exist; real verify would call stx version / API.
		if apply {
			// Placeholder for real post-replace verification.
		}
		info := stxversion.Current()
		if info.Version == "" {
			return ErrVerifyFail
		}
	}
	return nil
}

// CancelTask cancels the active task when cancellable. / 在可取消时取消活跃任务。
func (s *Service) CancelTask(taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.task == nil || s.task.ID != taskID {
		return fmt.Errorf("%w: task not found", ErrInvalid())
	}
	if !s.task.Cancellable || s.cancelF == nil {
		return fmt.Errorf("%w: task is not cancellable", ErrInvalid())
	}
	s.cancelF()
	return nil
}

// GetTask returns a task by id. / 按 id 返回任务。
func (s *Service) GetTask(taskID string) (*TaskState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.task == nil || s.task.ID != taskID {
		return nil, fmt.Errorf("%w: task not found", ErrInvalid())
	}
	cp := *s.task
	return &cp, nil
}

// StageSnapshot returns current staging state. / 返回当前预置状态。
func (s *Service) StageSnapshot() StageState {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneStage(s.stage)
}

func (s *Service) failStage(taskID string, err error, code, msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.task == nil || s.task.ID != taskID {
		return
	}
	now := time.Now().UTC()
	s.stage.Status = StatusFailed
	s.stage.ErrorCode = code
	s.stage.Message = msg
	s.stage.Cancellable = false
	s.task.Status = StatusFailed
	s.task.ErrorCode = code
	s.task.FailureReason = msg
	s.task.Cancellable = false
	s.task.CompletedAt = &now
	s.cancelF = nil
	_ = err
}

func (s *Service) failExecute(taskID, step string, err error, code, msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.task == nil || s.task.ID != taskID {
		return
	}
	now := time.Now().UTC()
	s.task.Status = StatusFailed
	s.task.FailureStep = step
	s.task.FailureReason = msg
	s.task.ErrorCode = code
	s.task.Cancellable = false
	s.task.CompletedAt = &now
	for i := range s.task.Steps {
		if s.task.Steps[i].Code == step {
			s.task.Steps[i].Status = StepFailed
			s.task.Steps[i].Error = msg
			s.task.Steps[i].EndedAt = &now
		}
	}
	s.cancelF = nil
	_ = err
}

func (s *Service) beginExclusive(op string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.task != nil && (s.task.Status == StatusStaging || s.task.Status == StatusUpgrading) {
		return ErrConcurrentUpgrade
	}
	_ = op
	return nil
}

func (s *Service) endExclusive() {}

func cloneStage(in StageState) StageState {
	out := in
	if in.Assets != nil {
		out.Assets = append([]AssetState(nil), in.Assets...)
	}
	return out
}
