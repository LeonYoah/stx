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

package diagnostics

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/LeonYoah/stx/internal/apps/audit"
	"github.com/LeonYoah/stx/internal/apps/cluster"
	appconfig "github.com/LeonYoah/stx/internal/apps/config"
	executionapp "github.com/LeonYoah/stx/internal/apps/execution"
	"github.com/LeonYoah/stx/internal/apps/monitor"
	monitoringapp "github.com/LeonYoah/stx/internal/apps/monitoring"
	"github.com/LeonYoah/stx/internal/config"
	"github.com/LeonYoah/stx/internal/logger"
	"gopkg.in/yaml.v3"
)

var (
	diagnosticDisplayTimezoneOnce sync.Once
	diagnosticDisplayTimezone     *time.Location
	diagnosticFileNameUnsafeChars = regexp.MustCompile(`[<>:"/\\|?*\s]+`)
	diagnosticFileNameDashRuns    = regexp.MustCompile(`-+`)
)

const diagnosticHTMLLogPreviewLineLimit = 1000

type diagnosticBundleArtifact struct {
	StepCode   DiagnosticStepCode `json:"step_code"`
	Category   string             `json:"category"`
	Format     string             `json:"format"`
	Status     string             `json:"status"`
	Path       string             `json:"path,omitempty"`
	RemotePath string             `json:"remote_path,omitempty"`
	NodeID     uint               `json:"node_id,omitempty"`
	HostID     uint               `json:"host_id,omitempty"`
	HostName   string             `json:"host_name,omitempty"`
	SizeBytes  int64              `json:"size_bytes,omitempty"`
	Message    string             `json:"message,omitempty"`
}

type diagnosticBundleManifest struct {
	Version         string                      `json:"version"`
	TaskID          uint                        `json:"task_id"`
	ClusterID       uint                        `json:"cluster_id"`
	TriggerSource   DiagnosticTaskSourceType    `json:"trigger_source"`
	SourceRef       DiagnosticTaskSourceRef     `json:"source_ref"`
	Options         DiagnosticTaskOptions       `json:"options"`
	Status          DiagnosticTaskStatus        `json:"status"`
	Summary         string                      `json:"summary"`
	LookbackMinutes int                         `json:"lookback_minutes,omitempty"`
	WindowStart     *time.Time                  `json:"window_start,omitempty"`
	WindowEnd       *time.Time                  `json:"window_end,omitempty"`
	GeneratedAt     time.Time                   `json:"generated_at"`
	Artifacts       []*diagnosticBundleArtifact `json:"artifacts"`
}

type diagnosticBundleExecutionState struct {
	WindowStart      *time.Time
	WindowEnd        *time.Time
	LookbackMinutes  int
	ErrorGroup       *SeatunnelErrorGroup
	ErrorEvents      []*SeatunnelErrorEvent
	LogSamples       []diagnosticCollectedLogSample
	InspectionDetail *ClusterInspectionReportDetailData
	ProcessEvents    []*monitor.ProcessEvent
	AlertSnapshot    []*monitoringapp.AlertInstance
	ClusterSnapshot  *cluster.Cluster
	ConfigSnapshot   *diagnosticConfigSnapshotSummary
	MetricsSnapshot  *diagnosticPrometheusSnapshot
	Artifacts        []*diagnosticBundleArtifact
}

type diagnosticCollectionWindow struct {
	Start           time.Time
	End             time.Time
	LookbackMinutes int
}

type diagnosticCollectionWindowPayload struct {
	StartAt         time.Time `json:"start_at"`
	EndAt           time.Time `json:"end_at"`
	LookbackMinutes int       `json:"lookback_minutes"`
}

type diagnosticConfigSnapshotSummary struct {
	ClusterID          uint                           `json:"cluster_id"`
	ClusterName        string                         `json:"cluster_name,omitempty"`
	DeploymentMode     string                         `json:"deployment_mode,omitempty"`
	CollectedAt        time.Time                      `json:"collected_at"`
	Files              []diagnosticConfigSnapshotFile `json:"files"`
	KeyHighlights      []diagnosticConfigKeyHighlight `json:"key_highlights,omitempty"`
	FilePreviews       []diagnosticConfigFilePreview  `json:"file_previews,omitempty"`
	DirectoryManifests []diagnosticDirectoryManifest  `json:"directory_manifests,omitempty"`
	ConfigChanges      []diagnosticConfigChangeRecord `json:"config_changes,omitempty"`
	CollectionNotes    []diagnosticConfigSnapshotNote `json:"collection_notes,omitempty"`
}

type diagnosticConfigSnapshotFile struct {
	HostID      uint   `json:"host_id"`
	HostName    string `json:"host_name,omitempty"`
	HostIP      string `json:"host_ip,omitempty"`
	NodeID      uint   `json:"node_id"`
	Role        string `json:"role,omitempty"`
	InstallDir  string `json:"install_dir,omitempty"`
	ConfigType  string `json:"config_type"`
	RemotePath  string `json:"remote_path,omitempty"`
	LocalPath   string `json:"local_path,omitempty"`
	SizeBytes   int64  `json:"size_bytes,omitempty"`
	ContentHash string `json:"content_hash,omitempty"`
}

type diagnosticConfigSnapshotNote struct {
	HostID     uint   `json:"host_id"`
	Role       string `json:"role,omitempty"`
	ConfigType string `json:"config_type,omitempty"`
	Message    string `json:"message"`
}

type diagnosticConfigKeyHighlight struct {
	HostID     uint                       `json:"host_id"`
	HostName   string                     `json:"host_name,omitempty"`
	HostIP     string                     `json:"host_ip,omitempty"`
	NodeID     uint                       `json:"node_id"`
	Role       string                     `json:"role,omitempty"`
	ConfigType string                     `json:"config_type"`
	RemotePath string                     `json:"remote_path,omitempty"`
	Items      []diagnosticConfigKeyValue `json:"items"`
}

type diagnosticConfigKeyValue struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type diagnosticConfigFilePreview struct {
	HostID     uint   `json:"host_id"`
	HostName   string `json:"host_name,omitempty"`
	HostIP     string `json:"host_ip,omitempty"`
	NodeID     uint   `json:"node_id"`
	Role       string `json:"role,omitempty"`
	ConfigType string `json:"config_type"`
	RemotePath string `json:"remote_path,omitempty"`
	Preview    string `json:"preview"`
}

type diagnosticDirectoryManifest struct {
	HostID     uint                              `json:"host_id"`
	HostName   string                            `json:"host_name,omitempty"`
	HostIP     string                            `json:"host_ip,omitempty"`
	NodeID     uint                              `json:"node_id"`
	Role       string                            `json:"role,omitempty"`
	Directory  string                            `json:"directory"`
	LocalPath  string                            `json:"local_path,omitempty"`
	EntryCount int                               `json:"entry_count"`
	Entries    []diagnosticDirectoryManifestItem `json:"entries"`
}

type diagnosticDirectoryManifestItem struct {
	Name    string    `json:"name"`
	Path    string    `json:"path"`
	Size    int64     `json:"size"`
	Mode    string    `json:"mode,omitempty"`
	ModTime time.Time `json:"mod_time,omitempty"`
	IsDir   bool      `json:"is_dir"`
}

type diagnosticConfigChangeRecord struct {
	ConfigID   uint      `json:"config_id"`
	ClusterID  uint      `json:"cluster_id"`
	HostID     *uint     `json:"host_id,omitempty"`
	HostScope  string    `json:"host_scope,omitempty"`
	ConfigType string    `json:"config_type"`
	FilePath   string    `json:"file_path,omitempty"`
	Version    int       `json:"version"`
	UpdatedAt  time.Time `json:"updated_at"`
	UpdatedBy  uint      `json:"updated_by"`
	IsTemplate bool      `json:"is_template"`
}

type diagnosticCollectedLogSample struct {
	HostID      uint      `json:"host_id"`
	HostName    string    `json:"host_name,omitempty"`
	HostIP      string    `json:"host_ip,omitempty"`
	NodeID      uint      `json:"node_id"`
	Role        string    `json:"role,omitempty"`
	SourceFile  string    `json:"source_file"`
	LocalPath   string    `json:"local_path,omitempty"`
	WindowStart time.Time `json:"window_start"`
	WindowEnd   time.Time `json:"window_end"`
	Content     string    `json:"content"`
}

type agentPullConfigResult struct {
	Success    bool   `json:"success"`
	Message    string `json:"message"`
	ConfigType string `json:"config_type"`
	Content    string `json:"content"`
	FilePath   string `json:"file_path"`
}

type agentDirectoryListingResult struct {
	Path    string                            `json:"path"`
	Entries []diagnosticDirectoryManifestItem `json:"entries"`
}

type diagnosticPrometheusSnapshot struct {
	ClusterID       uint                         `json:"cluster_id"`
	WindowStart     time.Time                    `json:"window_start"`
	WindowEnd       time.Time                    `json:"window_end"`
	StepSeconds     int                          `json:"step_seconds"`
	Signals         []diagnosticPrometheusSignal `json:"signals"`
	CollectionNotes []string                     `json:"collection_notes,omitempty"`
}

type diagnosticPrometheusSignal struct {
	Key           string                              `json:"key"`
	Title         string                              `json:"title"`
	Summary       string                              `json:"summary"`
	PromQL        string                              `json:"promql"`
	Unit          string                              `json:"unit"`
	Threshold     float64                             `json:"threshold"`
	ThresholdText string                              `json:"threshold_text"`
	Status        string                              `json:"status"`
	Comparator    string                              `json:"comparator"`
	Series        []diagnosticPrometheusSeriesSummary `json:"series"`
}

type diagnosticPrometheusPoint struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
}

type diagnosticPrometheusSeriesSummary struct {
	Instance  string                      `json:"instance"`
	MinValue  float64                     `json:"min_value"`
	MinAt     *time.Time                  `json:"min_at,omitempty"`
	MaxValue  float64                     `json:"max_value"`
	MaxAt     *time.Time                  `json:"max_at,omitempty"`
	LastValue float64                     `json:"last_value"`
	LastAt    *time.Time                  `json:"last_at,omitempty"`
	Samples   int                         `json:"samples"`
	Points    []diagnosticPrometheusPoint `json:"points,omitempty"`
}

type diagnosticBundleHTMLPayload struct {
	Language    DiagnosticLanguage `json:"language"`
	GeneratedAt time.Time          `json:"generated_at"`
	// Summary：聚焦一句话结论 + 少量关键指标
	Health diagnosticBundleHTMLHealthSummary `json:"health"`
	// 3.0 信息架构：结论、分类、时间线、关键信号
	Findings   []diagnosticBundleHTMLFindingCard  `json:"findings"`
	Categories []diagnosticBundleHTMLCategoryCard `json:"categories"`
	Timeline   []diagnosticBundleHTMLTimelineItem `json:"timeline"`
	KeySignals []diagnosticBundleHTMLSignalCard   `json:"key_signals"`
	// Critical Findings：按严重级别排序的关键发现（来自巡检发现或错误/告警上下文）
	Inspection *diagnosticBundleHTMLInspectionPanel `json:"inspection,omitempty"`
	// Evidence：证据详情，按需展开
	ErrorContext    *diagnosticBundleHTMLErrorPanel      `json:"error_context,omitempty"`
	AlertSnapshot   *diagnosticBundleHTMLAlertPanel      `json:"alert_snapshot,omitempty"`
	ProcessEvents   *diagnosticBundleHTMLProcessPanel    `json:"process_events,omitempty"`
	ConfigSnapshot  *diagnosticBundleHTMLConfigPanel     `json:"config_snapshot,omitempty"`
	MetricsSnapshot *diagnosticBundleHTMLMetricsPanel    `json:"metrics_snapshot,omitempty"`
	ThreadDumps     []diagnosticBundleHTMLThreadDumpItem `json:"thread_dumps,omitempty"`
	ArtifactGroups  []diagnosticBundleHTMLArtifactGroup  `json:"artifact_groups"`

	// 附录类信息：任务概览、执行过程、溯源与建议，弱化展示
	Cluster            *diagnosticBundleHTMLClusterSummary `json:"cluster,omitempty"`
	Task               diagnosticBundleHTMLTaskSummary     `json:"task"`
	TaskExecution      diagnosticBundleHTMLExecutionPanel  `json:"task_execution"`
	SourceTraceability []diagnosticBundleHTMLTraceItem     `json:"source_traceability"`
	Recommendations    []diagnosticBundleHTMLAdvice        `json:"recommendations"`
	PassedChecks       []diagnosticBundleHTMLAdvice        `json:"passed_checks"`
}

type diagnosticBundleHTMLHealthSummary struct {
	Tone          string                           `json:"tone"`
	Title         string                           `json:"title"`
	Summary       string                           `json:"summary"`
	ClusterLabel  string                           `json:"cluster_label"`
	WindowLabel   string                           `json:"window_label"`
	ImpactSummary string                           `json:"impact_summary"`
	PrimaryFocus  string                           `json:"primary_focus"`
	LastSignalAt  string                           `json:"last_signal_at"`
	Metrics       []diagnosticBundleHTMLMetricCard `json:"metrics"`
}

type diagnosticBundleHTMLFindingCard struct {
	Severity string `json:"severity"`
	Category string `json:"category"`
	Title    string `json:"title"`
	Summary  string `json:"summary"`
	Impact   string `json:"impact"`
	Action   string `json:"action"`
}

type diagnosticBundleHTMLCategoryCard struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Count int    `json:"count"`
	Note  string `json:"note"`
}

type diagnosticBundleHTMLTimelineItem struct {
	OccurredAt time.Time `json:"occurred_at"`
	TimeLabel  string    `json:"time_label"`
	Tone       string    `json:"tone"`
	Title      string    `json:"title"`
	Details    string    `json:"details"`
}

type diagnosticBundleHTMLTaskSummary struct {
	ID            uint                             `json:"id"`
	Status        DiagnosticTaskStatus             `json:"status"`
	TriggerSource DiagnosticTaskSourceType         `json:"trigger_source"`
	Summary       string                           `json:"summary"`
	CreatedBy     string                           `json:"created_by"`
	StartedAt     *time.Time                       `json:"started_at,omitempty"`
	CompletedAt   *time.Time                       `json:"completed_at,omitempty"`
	BundleDir     string                           `json:"bundle_dir"`
	ManifestPath  string                           `json:"manifest_path"`
	IndexPath     string                           `json:"index_path"`
	Options       DiagnosticTaskOptions            `json:"options"`
	SelectedNodes []diagnosticBundleHTMLNodeTarget `json:"selected_nodes"`
}

type diagnosticBundleHTMLClusterSummary struct {
	ID             uint                              `json:"id"`
	Name           string                            `json:"name"`
	Version        string                            `json:"version"`
	Status         string                            `json:"status"`
	DeploymentMode string                            `json:"deployment_mode"`
	InstallDir     string                            `json:"install_dir"`
	NodeCount      int                               `json:"node_count"`
	Nodes          []diagnosticBundleHTMLClusterNode `json:"nodes"`
}

type diagnosticBundleHTMLClusterNode struct {
	ClusterNodeID uint   `json:"cluster_node_id"`
	HostLabel     string `json:"host_label"`
	Role          string `json:"role"`
	HostID        uint   `json:"host_id"`
	InstallDir    string `json:"install_dir"`
	Status        string `json:"status"`
	ProcessPID    int    `json:"process_pid"`
}

type diagnosticBundleHTMLInspectionPanel struct {
	Summary         string                        `json:"summary"`
	Status          InspectionReportStatus        `json:"status"`
	RequestedBy     string                        `json:"requested_by"`
	LookbackMinutes int                           `json:"lookback_minutes"`
	CriticalCount   int                           `json:"critical_count"`
	WarningCount    int                           `json:"warning_count"`
	InfoCount       int                           `json:"info_count"`
	StartedAt       *time.Time                    `json:"started_at,omitempty"`
	FinishedAt      *time.Time                    `json:"finished_at,omitempty"`
	Findings        []diagnosticBundleHTMLFinding `json:"findings"`
}

type diagnosticBundleHTMLFinding struct {
	Severity       string `json:"severity"`
	CheckName      string `json:"check_name"`
	CheckCode      string `json:"check_code"`
	Summary        string `json:"summary"`
	Recommendation string `json:"recommendation"`
	Evidence       string `json:"evidence"`
}

type diagnosticBundleHTMLErrorPanel struct {
	GroupTitle       string                           `json:"group_title"`
	ExceptionClass   string                           `json:"exception_class"`
	OccurrenceCount  int64                            `json:"occurrence_count"`
	FirstSeenAt      *time.Time                       `json:"first_seen_at,omitempty"`
	LastSeenAt       *time.Time                       `json:"last_seen_at,omitempty"`
	SampleMessage    string                           `json:"sample_message"`
	RecentEventCount int                              `json:"recent_event_count"`
	Events           []diagnosticBundleHTMLErrorEvent `json:"events"`
	LogSamples       []diagnosticBundleHTMLLogSample  `json:"log_samples"`
}

type diagnosticBundleHTMLErrorEvent struct {
	OccurredAt string `json:"occurred_at"`
	Role       string `json:"role"`
	HostLabel  string `json:"host_label"`
	SourceFile string `json:"source_file"`
	JobID      string `json:"job_id"`
	Message    string `json:"message"`
	Evidence   string `json:"evidence"`
}

type diagnosticBundleHTMLAlertPanel struct {
	Total       int                             `json:"total"`
	Critical    int                             `json:"critical"`
	Warning     int                             `json:"warning"`
	Firing      int                             `json:"firing"`
	FirstSeenAt string                          `json:"first_seen_at"`
	LastSeenAt  string                          `json:"last_seen_at"`
	Alerts      []diagnosticBundleHTMLAlertItem `json:"alerts"`
}

type diagnosticBundleHTMLAlertItem struct {
	Name        string `json:"name"`
	Severity    string `json:"severity"`
	Status      string `json:"status"`
	CreatedAt   string `json:"created_at"`
	FiringAt    string `json:"firing_at"`
	LastSeenAt  string `json:"last_seen_at"`
	ResolvedAt  string `json:"resolved_at"`
	Summary     string `json:"summary"`
	Description string `json:"description"`
}

type diagnosticBundleHTMLProcessPanel struct {
	Total  int                                `json:"total"`
	ByType []diagnosticBundleHTMLMetricCard   `json:"by_type"`
	Events []diagnosticBundleHTMLProcessEvent `json:"events"`
}

// diagnosticBundleHTMLConfigFileEntry 将单份配置的关键摘要与原始内容预览聚合展示。
// diagnosticBundleHTMLConfigFileEntry aggregates key highlights and raw content preview for a single config file.
type diagnosticBundleHTMLConfigFileEntry struct {
	HostID     uint                       `json:"host_id"`
	HostName   string                     `json:"host_name,omitempty"`
	HostIP     string                     `json:"host_ip,omitempty"`
	NodeID     uint                       `json:"node_id"`
	Role       string                     `json:"role,omitempty"`
	ConfigType string                     `json:"config_type"`
	RemotePath string                     `json:"remote_path,omitempty"`
	Items      []diagnosticConfigKeyValue `json:"items,omitempty"`
	Preview    string                     `json:"preview,omitempty"`
}

type diagnosticBundleHTMLConfigPanel struct {
	FileCount          int                                   `json:"file_count"`
	KeyHighlightCount  int                                   `json:"key_highlight_count"`
	DirectoryCount     int                                   `json:"directory_count"`
	ChangedConfigCount int                                   `json:"changed_config_count"`
	ConfigFileEntries  []diagnosticBundleHTMLConfigFileEntry `json:"config_file_entries"`
	KeyHighlights      []diagnosticConfigKeyHighlight        `json:"key_highlights"`
	FilePreviews       []diagnosticConfigFilePreview         `json:"file_previews"`
	RecentChanges      []diagnosticConfigChangeRecord        `json:"recent_changes"`
	RemainingChanges   []diagnosticConfigChangeRecord        `json:"remaining_changes"`
	Files              []diagnosticConfigSnapshotFile        `json:"files"`
	DirectoryManifests []diagnosticDirectoryManifest         `json:"directory_manifests"`
	ConfigChanges      []diagnosticConfigChangeRecord        `json:"config_changes"`
	CollectionNotes    []diagnosticConfigSnapshotNote        `json:"collection_notes"`
}

// diagnosticBundleHTMLThreadDumpItem 描述节点线程栈快照及其本地预览。
// diagnosticBundleHTMLThreadDumpItem describes a node thread dump snapshot and its local preview.
type diagnosticBundleHTMLThreadDumpItem struct {
	HostID       uint   `json:"host_id"`
	HostName     string `json:"host_name,omitempty"`
	HostIP       string `json:"host_ip,omitempty"`
	HostLabel    string `json:"host_label"`
	NodeID       uint   `json:"node_id"`
	Role         string `json:"role,omitempty"`
	Tool         string `json:"tool,omitempty"`
	RemotePath   string `json:"remote_path,omitempty"`
	LocalPath    string `json:"local_path,omitempty"`
	RelativePath string `json:"relative_path,omitempty"`
	PreviewURL   string `json:"preview_url,omitempty"`
	SizeBytes    int64  `json:"size_bytes"`
	SizeLabel    string `json:"size_label"`
	Preview      string `json:"preview"`
	TotalLines   int    `json:"total_lines"`
}

type diagnosticBundleHTMLMetricsPanel struct {
	SignalCount        int                          `json:"signal_count"`
	AnomalyCount       int                          `json:"anomaly_count"`
	HighlightedSignals []diagnosticPrometheusSignal `json:"highlighted_signals"`
	AdditionalSignals  []diagnosticPrometheusSignal `json:"additional_signals"`
	CollectionNotes    []string                     `json:"collection_notes,omitempty"`
}

type diagnosticBundleHTMLProcessEvent struct {
	CreatedAt   string `json:"created_at"`
	EventType   string `json:"event_type"`
	ProcessName string `json:"process_name"`
	NodeLabel   string `json:"node_label"`
	Details     string `json:"details"`
}

type diagnosticBundleHTMLLogSample struct {
	HostLabel           string `json:"host_label"`
	SourceFile          string `json:"source_file"`
	WindowLabel         string `json:"window_label"`
	PreviewContent      string `json:"preview_content"`
	PreviewLineCount    int    `json:"preview_line_count"`
	PreviewTruncated    bool   `json:"preview_truncated"`
	FullLogRelativePath string `json:"full_log_relative_path,omitempty"`
	FullLogPreviewURL   string `json:"full_log_preview_url,omitempty"`
}

type diagnosticBundleHTMLExecutionPanel struct {
	Steps []diagnosticBundleHTMLExecutionStep `json:"steps"`
	Nodes []diagnosticBundleHTMLExecutionNode `json:"nodes"`
}

type diagnosticBundleHTMLExecutionStep struct {
	Sequence    int    `json:"sequence"`
	Code        string `json:"code"`
	Title       string `json:"title"`
	Status      string `json:"status"`
	Message     string `json:"message"`
	Error       string `json:"error"`
	StartedAt   string `json:"started_at"`
	CompletedAt string `json:"completed_at"`
}

type diagnosticBundleHTMLExecutionNode struct {
	HostLabel   string `json:"host_label"`
	Role        string `json:"role"`
	Status      string `json:"status"`
	CurrentStep string `json:"current_step"`
	Message     string `json:"message"`
	Error       string `json:"error"`
	StartedAt   string `json:"started_at"`
	CompletedAt string `json:"completed_at"`
}

type diagnosticBundleHTMLArtifactGroup struct {
	Key   string                             `json:"key"`
	Label string                             `json:"label"`
	Items []diagnosticBundleHTMLArtifactView `json:"items"`
}

type diagnosticBundleHTMLArtifactView struct {
	Category      string `json:"category"`
	CategoryLabel string `json:"category_label"`
	StepCode      string `json:"step_code"`
	Status        string `json:"status"`
	Format        string `json:"format"`
	HostLabel     string `json:"host_label"`
	LocalPath     string `json:"local_path"`
	RelativePath  string `json:"relative_path"`
	RemotePath    string `json:"remote_path"`
	SizeLabel     string `json:"size_label"`
	Message       string `json:"message"`
	Preview       string `json:"preview"`
	PreviewNote   string `json:"preview_note"`
}

type diagnosticBundleHTMLNodeTarget struct {
	HostLabel   string `json:"host_label"`
	Role        string `json:"role"`
	InstallDir  string `json:"install_dir"`
	ClusterNode string `json:"cluster_node"`
}

type diagnosticBundleHTMLTraceItem struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type diagnosticBundleHTMLMetricCard struct {
	Label string `json:"label"`
	Value string `json:"value"`
	Note  string `json:"note"`
}

type diagnosticBundleHTMLSignalCard struct {
	Key            string                      `json:"key"`
	Title          string                      `json:"title"`
	Status         string                      `json:"status"`
	Summary        string                      `json:"summary"`
	ThresholdText  string                      `json:"threshold_text"`
	Instance       string                      `json:"instance"`
	LastValue      string                      `json:"last_value"`
	PeakValue      string                      `json:"peak_value"`
	PeakAt         string                      `json:"peak_at"`
	Interpretation string                      `json:"interpretation"`
	Threshold      float64                     `json:"threshold"`
	Comparator     string                      `json:"comparator"`
	Unit           string                      `json:"unit"`
	Points         []diagnosticPrometheusPoint `json:"points"`
}

type diagnosticBundleHTMLAdvice struct {
	Title   string `json:"title"`
	Details string `json:"details"`
}

type agentThreadDumpResult struct {
	Status      string    `json:"status"`
	PID         int       `json:"pid"`
	Role        string    `json:"role"`
	InstallDir  string    `json:"install_dir"`
	Tool        string    `json:"tool"`
	OutputPath  string    `json:"output_path"`
	SizeBytes   int64     `json:"size_bytes"`
	Message     string    `json:"message"`
	Content     string    `json:"content"`
	CollectedAt time.Time `json:"collected_at"`
}

type agentJVMDumpResult struct {
	Status         string    `json:"status"`
	PID            int       `json:"pid"`
	Role           string    `json:"role"`
	InstallDir     string    `json:"install_dir"`
	Tool           string    `json:"tool"`
	OutputPath     string    `json:"output_path"`
	SizeBytes      int64     `json:"size_bytes"`
	FreeBytes      int64     `json:"free_bytes"`
	RequiredBytes  int64     `json:"required_bytes"`
	EstimatedBytes int64     `json:"estimated_bytes"`
	Message        string    `json:"message"`
	CollectedAt    time.Time `json:"collected_at"`
}

func (s *Service) StartDiagnosticTask(ctx context.Context, taskID uint) error {
	if s == nil || s.repo == nil {
		return ErrDiagnosticsRepositoryUnavailable
	}
	task, err := s.repo.GetDiagnosticTaskByID(ctx, taskID)
	if err != nil {
		return err
	}
	switch task.Status {
	case DiagnosticTaskStatusRunning:
		return s.syncExecutionFromDiagnosticTask(ctx, task)
	case DiagnosticTaskStatusSucceeded:
		return s.syncExecutionFromDiagnosticTask(ctx, task)
	case DiagnosticTaskStatusCancelRequested, DiagnosticTaskStatusCancelling, DiagnosticTaskStatusCancelled, DiagnosticTaskStatusFailed:
		return ErrDiagnosticTaskAlreadyFinished
	}
	now := time.Now().UTC()
	startedAt := task.StartedAt
	if startedAt == nil {
		startedAt = &now
	}
	if task.CurrentStep == "" {
		task.CurrentStep = resolveInitialDiagnosticTaskStep(DefaultDiagnosticTaskSteps(), task.Options.Normalize())
	}
	updated, err := s.repo.UpdateDiagnosticTaskFields(ctx, task.ID, []DiagnosticTaskStatus{DiagnosticTaskStatusPending, DiagnosticTaskStatusReady}, map[string]any{
		"status":       DiagnosticTaskStatusRunning,
		"current_step": task.CurrentStep,
		"started_at":   startedAt,
		"updated_at":   now,
	})
	if err != nil {
		return err
	}
	if !updated {
		latest, loadErr := s.repo.GetDiagnosticTaskByID(ctx, task.ID)
		if loadErr != nil {
			return loadErr
		}
		if latest.Status == DiagnosticTaskStatusRunning {
			return s.syncExecutionFromDiagnosticTask(ctx, latest)
		}
		return ErrDiagnosticTaskAlreadyFinished
	}
	task.Status = DiagnosticTaskStatusRunning
	task.StartedAt = startedAt
	task.UpdatedAt = now
	s.publishDiagnosticTaskEvent(newDiagnosticTaskUpdatedEvent(task))
	if err := s.syncExecutionFromDiagnosticTask(ctx, task); err != nil {
		return err
	}
	go s.executeDiagnosticTask(context.Background(), task.ID)
	return nil
}

func (s *Service) executeDiagnosticTask(ctx context.Context, taskID uint) {
	task, err := s.repo.GetDiagnosticTaskByID(ctx, taskID)
	if err != nil {
		logger.ErrorF(ctx, "[DiagnosticsTask] load task failed: task_id=%d err=%v", taskID, err)
		return
	}
	metadata := audit.CommandMetadata{
		ExecutionID:   task.ExecutionID,
		OwnerUserID:   task.CreatedBy,
		OwnerUsername: strings.TrimSpace(task.CreatedByName),
	}
	if s.executionService != nil && strings.TrimSpace(task.ExecutionID) != "" {
		item, loadErr := s.executionService.Get(ctx, executionapp.Actor{UserID: uint64(task.CreatedBy), IsAdmin: task.CreatedBy == 0}, task.ExecutionID)
		if loadErr == nil {
			metadata.RequestID = item.RequestID
			metadata.ClientType = item.ClientType
		}
	}
	ctx = audit.WithCommandMetadata(ctx, metadata)
	if err := s.runDiagnosticTask(ctx, task); err != nil {
		logger.ErrorF(ctx, "[DiagnosticsTask] run task failed: task_id=%d err=%v", taskID, err)
	}
}

func (s *Service) runDiagnosticTask(ctx context.Context, task *DiagnosticTask) error {
	if task == nil {
		return ErrDiagnosticTaskNotFound
	}
	if cancelled, err := s.cancelDiagnosticTaskAtBoundary(ctx, task.ID); err != nil || cancelled {
		return err
	}
	stepsByCode := make(map[DiagnosticStepCode]*DiagnosticTaskStep, len(task.Steps))
	for _, step := range task.Steps {
		stepsByCode[step.Code] = &DiagnosticTaskStep{
			ID:          step.ID,
			TaskID:      step.TaskID,
			Code:        step.Code,
			Sequence:    step.Sequence,
			Title:       step.Title,
			Description: step.Description,
			Status:      step.Status,
			Message:     step.Message,
			Error:       step.Error,
			StartedAt:   step.StartedAt,
			CompletedAt: step.CompletedAt,
			CreatedAt:   step.CreatedAt,
			UpdatedAt:   step.UpdatedAt,
		}
	}
	nodesByClusterNodeID := make(map[uint]*DiagnosticNodeExecution, len(task.NodeExecutions))
	for _, node := range task.NodeExecutions {
		copyNode := node
		nodesByClusterNodeID[node.ClusterNodeID] = &copyNode
	}

	state := &diagnosticBundleExecutionState{}
	bundleDir := diagnosticTaskBundleDir(task.ID)
	if err := os.MkdirAll(bundleDir, 0o755); err != nil {
		return s.failDiagnosticTask(ctx, task, DiagnosticStepCodeCollectConfigSnapshot, fmt.Errorf("create bundle dir: %w", err))
	}
	task.BundleDir = bundleDir
	updated, err := s.repo.UpdateDiagnosticTaskFields(ctx, task.ID, []DiagnosticTaskStatus{DiagnosticTaskStatusRunning}, map[string]any{
		"bundle_dir": bundleDir,
		"updated_at": time.Now().UTC(),
	})
	if err != nil {
		return err
	}
	if !updated {
		if cancelled, cancelErr := s.cancelDiagnosticTaskAtBoundary(ctx, task.ID); cancelErr != nil || cancelled {
			return cancelErr
		}
		return ErrDiagnosticTaskAlreadyFinished
	}

	for _, planStep := range DefaultDiagnosticTaskSteps() {
		if cancelled, err := s.cancelDiagnosticTaskAtBoundary(ctx, task.ID); err != nil || cancelled {
			return err
		}
		step := stepsByCode[planStep.Code]
		if step == nil {
			continue
		}
		if step.Status == DiagnosticTaskStatusSkipped {
			continue
		}

		if err := s.beginDiagnosticTaskStep(ctx, task, step); err != nil {
			return err
		}

		stepErr := s.executeDiagnosticPlanStep(ctx, task, step, planStep, nodesByClusterNodeID, state, bundleDir)
		if stepErr != nil {
			if err := s.failDiagnosticTaskStep(ctx, task, step, stepErr); err != nil {
				return err
			}
			if cancelled, err := s.cancelDiagnosticTaskAtBoundary(ctx, task.ID); err != nil || cancelled {
				return err
			}
			if planStep.Required {
				return s.failDiagnosticTask(ctx, task, step.Code, stepErr)
			}
			_ = s.AppendDiagnosticStepLog(ctx, &DiagnosticStepLog{
				TaskID:         task.ID,
				TaskStepID:     uintPtr(step.ID),
				StepCode:       step.Code,
				Level:          DiagnosticLogLevelWarn,
				EventType:      DiagnosticLogEventTypeNote,
				Message:        bilingualText("可选步骤失败，任务继续执行。", "Optional step failed and task will continue."),
				CreatedAt:      time.Now().UTC(),
				CommandSummary: stepErr.Error(),
			})
			continue
		}

		if err := s.finishDiagnosticTaskStep(ctx, task, step, bilingualText("步骤执行完成。", "Step completed.")); err != nil {
			return err
		}
		if cancelled, err := s.cancelDiagnosticTaskAtBoundary(ctx, task.ID); err != nil || cancelled {
			return err
		}
	}

	now := time.Now().UTC()
	task.Summary = strings.TrimSpace(task.Summary)
	if task.Summary == "" {
		task.Summary = bilingualText("诊断任务执行完成。", "Diagnostic bundle task completed.")
	}
	updated, err = s.repo.UpdateDiagnosticTaskFields(ctx, task.ID, []DiagnosticTaskStatus{DiagnosticTaskStatusRunning}, map[string]any{
		"status":       DiagnosticTaskStatusSucceeded,
		"current_step": DiagnosticStepCodeComplete,
		"completed_at": now,
		"updated_at":   now,
		"summary":      task.Summary,
	})
	if err != nil {
		return err
	}
	if !updated {
		if cancelled, cancelErr := s.cancelDiagnosticTaskAtBoundary(ctx, task.ID); cancelErr != nil || cancelled {
			return cancelErr
		}
		return ErrDiagnosticTaskAlreadyFinished
	}
	task.Status = DiagnosticTaskStatusSucceeded
	task.CurrentStep = DiagnosticStepCodeComplete
	task.CompletedAt = &now
	task.UpdatedAt = now
	s.publishDiagnosticTaskEvent(newDiagnosticTaskUpdatedEvent(task))
	if err := s.syncExecutionFromDiagnosticTask(ctx, task); err != nil {
		return err
	}
	if task.ManifestPath != "" {
		if err := writeDiagnosticBundleManifestFile(task.ManifestPath, task, state.Artifacts, state); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) executeDiagnosticPlanStep(ctx context.Context, task *DiagnosticTask, step *DiagnosticTaskStep, planStep DiagnosticPlanStep, nodesByClusterNodeID map[uint]*DiagnosticNodeExecution, state *diagnosticBundleExecutionState, bundleDir string) error {
	switch step.Code {
	case DiagnosticStepCodeCollectErrorContext:
		return s.executeCollectErrorContextStep(ctx, task, step, state, bundleDir)
	case DiagnosticStepCodeCollectProcessEvents:
		return s.executeCollectProcessEventsStep(ctx, task, step, state, bundleDir)
	case DiagnosticStepCodeCollectAlertSnapshot:
		return s.executeCollectAlertSnapshotStep(ctx, task, step, state, bundleDir)
	case DiagnosticStepCodeCollectConfigSnapshot:
		return s.executeCollectConfigSnapshotStep(ctx, task, step, nodesByClusterNodeID, state, bundleDir)
	case DiagnosticStepCodeCollectLogSample:
		return s.executeCollectLogSampleStep(ctx, task, step, nodesByClusterNodeID, state, bundleDir)
	case DiagnosticStepCodeCollectThreadDump:
		return s.executeCollectThreadDumpStep(ctx, task, step, nodesByClusterNodeID, state, bundleDir)
	case DiagnosticStepCodeCollectJVMDump:
		return s.executeCollectJVMDumpStep(ctx, task, step, nodesByClusterNodeID, state, bundleDir)
	case DiagnosticStepCodeAssembleManifest:
		return s.executeAssembleManifestStep(ctx, task, step, state, bundleDir)
	case DiagnosticStepCodeRenderHTMLSummary:
		return s.executeRenderHTMLSummaryStep(ctx, task, step, state, bundleDir)
	case DiagnosticStepCodeComplete:
		return s.AppendDiagnosticStepLog(ctx, &DiagnosticStepLog{
			TaskID:     task.ID,
			TaskStepID: uintPtr(step.ID),
			StepCode:   step.Code,
			Level:      DiagnosticLogLevelInfo,
			EventType:  DiagnosticLogEventTypeSuccess,
			Message:    bilingualText("诊断任务执行完成。", "Diagnostic task completed."),
			CreatedAt:  time.Now().UTC(),
		})
	default:
		return nil
	}
}

func (s *Service) executeCollectErrorContextStep(ctx context.Context, task *DiagnosticTask, step *DiagnosticTaskStep, state *diagnosticBundleExecutionState, bundleDir string) error {
	payload := map[string]interface{}{
		"trigger_source": task.TriggerSource,
		"source_ref":     task.SourceRef,
	}
	if task.SourceRef.InspectionReportID > 0 {
		detail, err := s.GetInspectionReportDetail(ctx, task.SourceRef.InspectionReportID)
		if err != nil {
			return err
		}
		state.InspectionDetail = detail
		payload["inspection_detail"] = detail
	}
	window := resolveDiagnosticCollectionWindow(task, state.InspectionDetail)
	state.WindowStart = timePtr(window.Start)
	state.WindowEnd = timePtr(window.End)
	state.LookbackMinutes = window.LookbackMinutes
	payload["collection_window"] = diagnosticCollectionWindowPayload{
		StartAt:         window.Start,
		EndAt:           window.End,
		LookbackMinutes: window.LookbackMinutes,
	}
	group, events, err := s.resolveDiagnosticErrorContext(ctx, task, window)
	if err != nil {
		return err
	}
	if group != nil {
		state.ErrorGroup = group
		state.ErrorEvents = events
		payload["error_group"] = group
		payload["error_events"] = events
	}
	return s.writeDiagnosticJSONArtifact(ctx, task, step, state, bundleDir, "error-context.json", payload, &diagnosticBundleArtifact{
		StepCode: step.Code,
		Category: "error_context",
		Format:   "json",
		Status:   "created",
	})
}

func (s *Service) resolveDiagnosticErrorContext(ctx context.Context, task *DiagnosticTask, window diagnosticCollectionWindow) (*SeatunnelErrorGroup, []*SeatunnelErrorEvent, error) {
	if s == nil || s.repo == nil || task == nil {
		return nil, nil, nil
	}
	groupID := task.SourceRef.ErrorGroupID
	if groupID == 0 {
		groups, _, err := s.repo.ListErrorGroups(ctx, &SeatunnelErrorGroupFilter{
			ClusterID: task.ClusterID,
			StartTime: timePtr(window.Start),
			EndTime:   timePtr(window.End),
			Page:      1,
			PageSize:  1,
		})
		if err != nil {
			return nil, nil, err
		}
		if len(groups) == 0 || groups[0] == nil {
			return nil, nil, nil
		}
		groupID = groups[0].ID
	}
	group, err := s.repo.GetErrorGroupByID(ctx, groupID)
	if err != nil {
		return nil, nil, err
	}
	events, _, err := s.repo.ListErrorEvents(ctx, &SeatunnelErrorEventFilter{
		ErrorGroupID: group.ID,
		ClusterID:    task.ClusterID,
		StartTime:    timePtr(window.Start),
		EndTime:      timePtr(window.End),
		Page:         1,
		PageSize:     100,
	})
	if err != nil {
		return nil, nil, err
	}
	return group, events, nil
}

func (s *Service) executeCollectProcessEventsStep(ctx context.Context, task *DiagnosticTask, step *DiagnosticTaskStep, state *diagnosticBundleExecutionState, bundleDir string) error {
	window := resolveDiagnosticCollectionWindow(task, state.InspectionDetail)
	state.WindowStart = timePtr(window.Start)
	state.WindowEnd = timePtr(window.End)
	state.LookbackMinutes = window.LookbackMinutes
	if s.monitorService == nil {
		state.ProcessEvents = []*monitor.ProcessEvent{}
	} else {
		loadedFromFilteredQuery := false
		if filteredReader, ok := s.monitorService.(interface {
			ListEvents(ctx context.Context, filter *monitor.ProcessEventFilter) ([]*monitor.ProcessEventWithHost, int64, error)
		}); ok {
			rows, _, err := filteredReader.ListEvents(ctx, &monitor.ProcessEventFilter{
				ClusterID: task.ClusterID,
				StartTime: timePtr(window.Start),
				EndTime:   timePtr(window.End),
				Page:      1,
				PageSize:  100,
			})
			if err != nil {
				return err
			}
			events := make([]*monitor.ProcessEvent, 0, len(rows))
			for _, row := range rows {
				if row == nil {
					continue
				}
				eventCopy := row.ProcessEvent
				events = append(events, &eventCopy)
			}
			state.ProcessEvents = filterDiagnosticProcessEventsByWindow(events, window.Start, window.End)
			loadedFromFilteredQuery = true
		}
		if !loadedFromFilteredQuery || len(state.ProcessEvents) == 0 {
			events, err := s.monitorService.ListClusterEvents(ctx, task.ClusterID, 200)
			if err != nil {
				return err
			}
			state.ProcessEvents = filterDiagnosticProcessEventsByWindow(events, window.Start, window.End)
		}
	}
	return s.writeDiagnosticJSONArtifact(ctx, task, step, state, bundleDir, "process-events.json", state.ProcessEvents, &diagnosticBundleArtifact{
		StepCode: step.Code,
		Category: "process_events",
		Format:   "json",
		Status:   "created",
	})
}

func (s *Service) executeCollectAlertSnapshotStep(ctx context.Context, task *DiagnosticTask, step *DiagnosticTaskStep, state *diagnosticBundleExecutionState, bundleDir string) error {
	window := resolveDiagnosticCollectionWindow(task, state.InspectionDetail)
	state.WindowStart = timePtr(window.Start)
	state.WindowEnd = timePtr(window.End)
	state.LookbackMinutes = window.LookbackMinutes
	alerts := make([]*monitoringapp.AlertInstance, 0)
	if s.monitoringService != nil {
		data, err := s.monitoringService.ListAlertInstances(ctx, &monitoringapp.AlertInstanceFilter{
			ClusterID: fmt.Sprintf("%d", task.ClusterID),
			Page:      1,
			PageSize:  200,
		})
		if err != nil {
			return err
		}
		if data != nil {
			alerts = data.Alerts
		}
	}
	state.AlertSnapshot = filterDiagnosticAlertsByWindow(alerts, window.Start, window.End, task.SourceRef.AlertID)
	if err := s.writeDiagnosticJSONArtifact(ctx, task, step, state, bundleDir, "alert-snapshot.json", state.AlertSnapshot, &diagnosticBundleArtifact{
		StepCode: step.Code,
		Category: "alert_snapshot",
		Format:   "json",
		Status:   "created",
	}); err != nil {
		return err
	}
	metricsSnapshot, err := s.collectDiagnosticPrometheusSnapshot(ctx, task, window)
	if err != nil {
		_ = s.AppendDiagnosticStepLog(ctx, &DiagnosticStepLog{
			TaskID:         task.ID,
			TaskStepID:     uintPtr(step.ID),
			StepCode:       step.Code,
			Level:          DiagnosticLogLevelWarn,
			EventType:      DiagnosticLogEventTypeNote,
			Message:        bilingualText(fmt.Sprintf("指标快照采集失败：%v", err), fmt.Sprintf("Metrics snapshot collection failed: %v", err)),
			CommandSummary: "metrics_snapshot",
			CreatedAt:      time.Now().UTC(),
		})
		return nil
	}
	if metricsSnapshot != nil && len(metricsSnapshot.Signals) > 0 {
		state.MetricsSnapshot = metricsSnapshot
		if err := s.writeDiagnosticJSONArtifact(ctx, task, step, state, bundleDir, "metrics-snapshot.json", metricsSnapshot, &diagnosticBundleArtifact{
			StepCode: step.Code,
			Category: "metrics_snapshot",
			Format:   "json",
			Status:   "created",
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) executeCollectConfigSnapshotStep(ctx context.Context, task *DiagnosticTask, step *DiagnosticTaskStep, nodesByClusterNodeID map[uint]*DiagnosticNodeExecution, state *diagnosticBundleExecutionState, bundleDir string) error {
	if s.clusterService == nil {
		return fmt.Errorf("cluster service is unavailable")
	}
	if s.agentSender == nil {
		return fmt.Errorf("agent sender is unavailable")
	}
	clusterInfo, err := s.clusterService.Get(ctx, task.ClusterID)
	if err != nil {
		return err
	}
	state.ClusterSnapshot = clusterInfo
	window := resolveDiagnosticCollectionWindow(task, state.InspectionDetail)
	state.WindowStart = timePtr(window.Start)
	state.WindowEnd = timePtr(window.End)
	state.LookbackMinutes = window.LookbackMinutes
	configDir := filepath.Join(bundleDir, "configs")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return err
	}
	summary := &diagnosticConfigSnapshotSummary{
		ClusterID:          task.ClusterID,
		ClusterName:        strings.TrimSpace(clusterInfo.Name),
		DeploymentMode:     strings.TrimSpace(string(clusterInfo.DeploymentMode)),
		CollectedAt:        time.Now().UTC(),
		Files:              make([]diagnosticConfigSnapshotFile, 0, len(task.SelectedNodes)*6),
		KeyHighlights:      make([]diagnosticConfigKeyHighlight, 0, len(task.SelectedNodes)*4),
		FilePreviews:       make([]diagnosticConfigFilePreview, 0, len(task.SelectedNodes)*4),
		DirectoryManifests: make([]diagnosticDirectoryManifest, 0, len(task.SelectedNodes)*4),
		ConfigChanges:      make([]diagnosticConfigChangeRecord, 0),
		CollectionNotes:    make([]diagnosticConfigSnapshotNote, 0, len(task.SelectedNodes)),
	}
	var successCount int
	var errs []string
	for _, selected := range sortedDiagnosticTaskTargets(task.SelectedNodes) {
		node := nodesByClusterNodeID[selected.ClusterNodeID]
		if node == nil {
			continue
		}
		if err := s.beginDiagnosticNodeStep(ctx, step, node, bilingualText("正在采集 SeaTunnel 配置文件。", "Collecting SeaTunnel config files.")); err != nil {
			return err
		}
		seenPaths := make(map[string]struct{})
		configTypes := buildDiagnosticConfigTypesForTarget(clusterInfo.DeploymentMode, selected.Role)
		nodeSuccess := false
		for _, configType := range configTypes {
			result, detail, err := s.pullDiagnosticConfigFile(ctx, selected, configType)
			if err != nil || result == nil || !result.Success || strings.TrimSpace(result.Content) == "" {
				summary.CollectionNotes = append(summary.CollectionNotes, diagnosticConfigSnapshotNote{
					HostID:     selected.HostID,
					Role:       selected.Role,
					ConfigType: string(configType),
					Message:    detail,
				})
				_ = s.AppendDiagnosticStepLog(ctx, &DiagnosticStepLog{
					TaskID:          task.ID,
					TaskStepID:      uintPtr(step.ID),
					NodeExecutionID: uintPtr(node.ID),
					StepCode:        step.Code,
					Level:           DiagnosticLogLevelWarn,
					EventType:       DiagnosticLogEventTypeNote,
					Message:         bilingualText(fmt.Sprintf("读取 %s 失败：%s", configType, detail), fmt.Sprintf("Failed to read %s: %s", configType, detail)),
					CommandSummary:  string(configType),
					CreatedAt:       time.Now().UTC(),
				})
				continue
			}
			if err := s.collectDiagnosticConfigArtifact(ctx, task, step, state, summary, configDir, selected, string(configType), firstNonEmptyString(result.FilePath, appconfig.GetConfigFilePath(configType), string(configType)), result.Content, node, seenPaths); err != nil {
				return err
			}
			nodeSuccess = true
		}
		for _, extraFile := range buildDiagnosticExtraConfigFilesForTarget(clusterInfo.DeploymentMode, selected.Role) {
			content, detail, err := s.pullDiagnosticRawFile(ctx, selected, filepath.Join(selected.InstallDir, "config", extraFile))
			if err != nil || strings.TrimSpace(content) == "" {
				summary.CollectionNotes = append(summary.CollectionNotes, diagnosticConfigSnapshotNote{
					HostID:     selected.HostID,
					Role:       selected.Role,
					ConfigType: extraFile,
					Message:    detail,
				})
				continue
			}
			if err := s.collectDiagnosticConfigArtifact(ctx, task, step, state, summary, configDir, selected, extraFile, filepath.Join(selected.InstallDir, "config", extraFile), content, node, seenPaths); err != nil {
				return err
			}
			nodeSuccess = true
		}
		for _, inventoryDir := range []string{"config", "lib", "connectors", "plugins"} {
			manifest, detail, err := s.pullDiagnosticDirectoryManifest(ctx, selected, filepath.Join(selected.InstallDir, inventoryDir))
			if err != nil || manifest == nil {
				summary.CollectionNotes = append(summary.CollectionNotes, diagnosticConfigSnapshotNote{
					HostID:     selected.HostID,
					Role:       selected.Role,
					ConfigType: inventoryDir,
					Message:    detail,
				})
				continue
			}
			if err := s.collectDiagnosticDirectoryManifestArtifact(ctx, task, step, state, summary, configDir, selected, inventoryDir, manifest, node); err != nil {
				return err
			}
			nodeSuccess = true
		}
		if nodeSuccess {
			successCount++
			if err := s.finishDiagnosticNodeStep(ctx, step, node, DiagnosticTaskStatusSucceeded, bilingualText("配置文件采集完成。", "Config files collected.")); err != nil {
				return err
			}
			continue
		}
		errs = append(errs, fmt.Sprintf("host=%d", selected.HostID))
		if err := s.finishDiagnosticNodeStep(ctx, step, node, DiagnosticTaskStatusFailed, bilingualText("未采集到有效配置文件。", "No config files were collected.")); err != nil {
			return err
		}
	}
	if successCount == 0 {
		return formatDiagnosticAllNodesFailed("全部节点配置文件采集失败", "Config file collection failed on all nodes", errs)
	}
	if s.configRepo != nil {
		changes, err := s.configRepo.ListUpdatedByClusterBetween(ctx, task.ClusterID, window.Start, window.End)
		if err != nil {
			summary.CollectionNotes = append(summary.CollectionNotes, diagnosticConfigSnapshotNote{
				Message: fmt.Sprintf("load config change history failed: %v", err),
			})
		} else {
			summary.ConfigChanges = buildDiagnosticConfigChangeRecords(changes)
		}
	}
	state.ConfigSnapshot = summary
	return s.writeDiagnosticJSONArtifact(ctx, task, step, state, bundleDir, "config-snapshot.json", summary, &diagnosticBundleArtifact{
		StepCode: step.Code,
		Category: "config_snapshot",
		Format:   "json",
		Status:   "created",
	})
}

func (s *Service) pullDiagnosticConfigFile(ctx context.Context, target DiagnosticTaskNodeTarget, configType appconfig.ConfigType) (*agentPullConfigResult, string, error) {
	success, output, err := s.sendDiagnosticAgentCommandWithRetry(ctx, target.AgentID, "pull_config", map[string]string{
		"install_dir": target.InstallDir,
		"config_type": string(configType),
	})
	if err != nil || !success {
		detail := resolveDiagnosticCommandFailure(output, err, "配置文件采集失败。", "Config collection failed.")
		return nil, detail, err
	}
	var result agentPullConfigResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		return nil, fmt.Sprintf("parse pull_config response failed: %v", err), err
	}
	if !result.Success {
		return &result, firstNonEmptyString(strings.TrimSpace(result.Message), "pull_config failed"), nil
	}
	return &result, "", nil
}

func (s *Service) pullDiagnosticRawFile(ctx context.Context, target DiagnosticTaskNodeTarget, absolutePath string) (string, string, error) {
	success, output, err := s.sendDiagnosticAgentCommandWithRetry(ctx, target.AgentID, "get_logs", map[string]string{
		"log_file": absolutePath,
		"mode":     "all",
	})
	if err != nil || !success {
		detail := resolveDiagnosticCommandFailure(output, err, "文件采集失败。", "File collection failed.")
		return "", detail, err
	}
	return output, "", nil
}

func (s *Service) pullDiagnosticDirectoryManifest(ctx context.Context, target DiagnosticTaskNodeTarget, absolutePath string) (*agentDirectoryListingResult, string, error) {
	success, output, err := s.sendDiagnosticAgentCommandWithRetry(ctx, target.AgentID, "get_logs", map[string]string{
		"log_file": absolutePath,
		"mode":     "list",
	})
	if err != nil || !success {
		detail := resolveDiagnosticCommandFailure(output, err, "目录清单采集失败。", "Directory manifest collection failed.")
		return nil, detail, err
	}
	var result agentDirectoryListingResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		return nil, fmt.Sprintf("parse directory listing failed: %v", err), err
	}
	return &result, "", nil
}

func (s *Service) collectDiagnosticConfigArtifact(ctx context.Context, task *DiagnosticTask, step *DiagnosticTaskStep, state *diagnosticBundleExecutionState, summary *diagnosticConfigSnapshotSummary, configDir string, target DiagnosticTaskNodeTarget, configType string, remotePath string, content string, node *DiagnosticNodeExecution, seenPaths map[string]struct{}) error {
	normalizedRemotePath := strings.TrimSpace(remotePath)
	if normalizedRemotePath == "" {
		normalizedRemotePath = strings.TrimSpace(configType)
	}
	if _, ok := seenPaths[normalizedRemotePath]; ok {
		return nil
	}
	seenPaths[normalizedRemotePath] = struct{}{}

	hostDir := filepath.Join(configDir, fmt.Sprintf("host-%d-%s", target.HostID, normalizeDiagnosticFileRole(target.Role)))
	if err := os.MkdirAll(hostDir, 0o755); err != nil {
		return err
	}
	fileName := filepath.Base(normalizedRemotePath)
	localPath := filepath.Join(hostDir, fileName)
	safeContent := audit.RedactText(content)
	if err := os.WriteFile(localPath, []byte(safeContent), 0o644); err != nil {
		return err
	}
	hash := buildDiagnosticContentHash(safeContent)
	artifact := &diagnosticBundleArtifact{
		StepCode:   step.Code,
		Category:   "config_snapshot",
		Format:     detectDiagnosticConfigFormat(fileName),
		Status:     "created",
		Path:       localPath,
		RemotePath: normalizedRemotePath,
		NodeID:     target.NodeID,
		HostID:     target.HostID,
		HostName:   target.HostName,
		SizeBytes:  int64(len(safeContent)),
		Message:    configType,
	}
	state.Artifacts = append(state.Artifacts, artifact)
	summary.Files = append(summary.Files, diagnosticConfigSnapshotFile{
		HostID:      target.HostID,
		HostName:    target.HostName,
		HostIP:      target.HostIP,
		NodeID:      target.NodeID,
		Role:        target.Role,
		InstallDir:  target.InstallDir,
		ConfigType:  configType,
		RemotePath:  normalizedRemotePath,
		LocalPath:   localPath,
		SizeBytes:   int64(len(safeContent)),
		ContentHash: hash,
	})
	if preview := buildDiagnosticConfigPreview(safeContent); strings.TrimSpace(preview) != "" {
		summary.FilePreviews = append(summary.FilePreviews, diagnosticConfigFilePreview{
			HostID:     target.HostID,
			HostName:   target.HostName,
			HostIP:     target.HostIP,
			NodeID:     target.NodeID,
			Role:       target.Role,
			ConfigType: configType,
			RemotePath: normalizedRemotePath,
			Preview:    preview,
		})
	}
	if items := extractDiagnosticConfigHighlights(configType, normalizedRemotePath, safeContent); len(items) > 0 {
		summary.KeyHighlights = append(summary.KeyHighlights, diagnosticConfigKeyHighlight{
			HostID:     target.HostID,
			HostName:   target.HostName,
			HostIP:     target.HostIP,
			NodeID:     target.NodeID,
			Role:       target.Role,
			ConfigType: configType,
			RemotePath: normalizedRemotePath,
			Items:      items,
		})
	}
	return s.AppendDiagnosticStepLog(ctx, &DiagnosticStepLog{
		TaskID:          task.ID,
		TaskStepID:      uintPtr(step.ID),
		NodeExecutionID: uintPtr(node.ID),
		StepCode:        step.Code,
		Level:           DiagnosticLogLevelInfo,
		EventType:       DiagnosticLogEventTypeSuccess,
		Message:         bilingualText(fmt.Sprintf("已采集 %s 到 %s", configType, localPath), fmt.Sprintf("Collected %s to %s", configType, localPath)),
		CommandSummary:  configType,
		CreatedAt:       time.Now().UTC(),
		Metadata: DiagnosticLogMetadata{
			"remote_path": normalizedRemotePath,
			"local_path":  localPath,
			"hash":        hash,
		},
	})
}

func (s *Service) collectDiagnosticDirectoryManifestArtifact(ctx context.Context, task *DiagnosticTask, step *DiagnosticTaskStep, state *diagnosticBundleExecutionState, summary *diagnosticConfigSnapshotSummary, configDir string, target DiagnosticTaskNodeTarget, category string, manifest *agentDirectoryListingResult, node *DiagnosticNodeExecution) error {
	if manifest == nil {
		return nil
	}
	hostDir := filepath.Join(configDir, fmt.Sprintf("host-%d-%s", target.HostID, normalizeDiagnosticFileRole(target.Role)))
	if err := os.MkdirAll(hostDir, 0o755); err != nil {
		return err
	}
	localPath := filepath.Join(hostDir, fmt.Sprintf("%s-manifest.json", strings.TrimSpace(category)))
	payload, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(localPath, payload, 0o644); err != nil {
		return err
	}
	state.Artifacts = append(state.Artifacts, &diagnosticBundleArtifact{
		StepCode:   step.Code,
		Category:   "directory_inventory",
		Format:     "json",
		Status:     "created",
		Path:       localPath,
		RemotePath: manifest.Path,
		NodeID:     target.NodeID,
		HostID:     target.HostID,
		HostName:   target.HostName,
		SizeBytes:  int64(len(payload)),
		Message:    category,
	})
	summary.DirectoryManifests = append(summary.DirectoryManifests, diagnosticDirectoryManifest{
		HostID:     target.HostID,
		HostName:   target.HostName,
		HostIP:     target.HostIP,
		NodeID:     target.NodeID,
		Role:       target.Role,
		Directory:  manifest.Path,
		LocalPath:  localPath,
		EntryCount: len(manifest.Entries),
		Entries:    manifest.Entries,
	})
	return s.AppendDiagnosticStepLog(ctx, &DiagnosticStepLog{
		TaskID:          task.ID,
		TaskStepID:      uintPtr(step.ID),
		NodeExecutionID: uintPtr(node.ID),
		StepCode:        step.Code,
		Level:           DiagnosticLogLevelInfo,
		EventType:       DiagnosticLogEventTypeSuccess,
		Message:         bilingualText(fmt.Sprintf("已采集 %s 目录清单到 %s", category, localPath), fmt.Sprintf("Collected %s directory manifest to %s", category, localPath)),
		CommandSummary:  category,
		CreatedAt:       time.Now().UTC(),
	})
}

type diagnosticConfigHighlightSpec struct {
	Label string
	Paths []string
}

func extractDiagnosticConfigHighlights(configType, remotePath, content string) []diagnosticConfigKeyValue {
	switch strings.TrimSpace(strings.ToLower(configType)) {
	case strings.ToLower(string(appconfig.ConfigTypeSeatunnel)),
		strings.ToLower(string(appconfig.ConfigTypeHazelcast)),
		strings.ToLower(string(appconfig.ConfigTypeHazelcastMaster)),
		strings.ToLower(string(appconfig.ConfigTypeHazelcastWorker)),
		strings.ToLower(string(appconfig.ConfigTypeHazelcastClient)):
		return extractDiagnosticYAMLHighlights(configType, content)
	case "jvm_options", "jvm_master_options", "jvm_worker_options", "jvm_client_options":
		return extractDiagnosticJVMOptionHighlights(content)
	case "log4j2.properties", "log4j2_client.properties":
		return extractDiagnosticLog4jHighlights(content)
	case "plugin_config":
		return extractDiagnosticPluginConfigHighlights(remotePath, content)
	default:
		return nil
	}
}

func extractDiagnosticYAMLHighlights(configType, content string) []diagnosticConfigKeyValue {
	var payload interface{}
	if err := yaml.Unmarshal([]byte(content), &payload); err != nil {
		return nil
	}
	flattened := make(map[string]string)
	flattenDiagnosticConfigValue("", payload, flattened)
	specs := buildDiagnosticYAMLHighlightSpecs(configType)
	result := make([]diagnosticConfigKeyValue, 0, len(specs))
	for _, spec := range specs {
		value := lookupDiagnosticConfigValue(flattened, spec.Paths...)
		if value == "" {
			continue
		}
		result = append(result, diagnosticConfigKeyValue{
			Label: spec.Label,
			Value: truncateString(normalizeDiagnosticDisplayText(value), 120),
		})
	}
	return result
}

func flattenDiagnosticConfigValue(prefix string, value interface{}, output map[string]string) {
	switch typed := value.(type) {
	case map[string]interface{}:
		for key, item := range typed {
			flattenDiagnosticConfigValue(joinDiagnosticConfigPath(prefix, key), item, output)
		}
	case map[interface{}]interface{}:
		for key, item := range typed {
			flattenDiagnosticConfigValue(joinDiagnosticConfigPath(prefix, fmt.Sprint(key)), item, output)
		}
	case []interface{}:
		if prefix == "" {
			return
		}
		scalars := make([]string, 0, len(typed))
		allScalars := true
		for _, item := range typed {
			switch item.(type) {
			case map[string]interface{}, map[interface{}]interface{}, []interface{}:
				allScalars = false
			default:
				scalars = append(scalars, strings.TrimSpace(fmt.Sprint(item)))
			}
		}
		if allScalars {
			output[strings.ToLower(prefix)] = summarizeDiagnosticScalarList(scalars)
			return
		}
		for index, item := range typed {
			flattenDiagnosticConfigValue(fmt.Sprintf("%s[%d]", prefix, index), item, output)
		}
	default:
		if prefix == "" {
			return
		}
		output[strings.ToLower(prefix)] = strings.TrimSpace(fmt.Sprint(value))
	}
}

func joinDiagnosticConfigPath(prefix, segment string) string {
	segment = strings.TrimSpace(strings.ToLower(segment))
	if prefix == "" {
		return segment
	}
	return prefix + "." + segment
}

func summarizeDiagnosticScalarList(items []string) string {
	filtered := make([]string, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item) == "" {
			continue
		}
		filtered = append(filtered, strings.TrimSpace(item))
	}
	if len(filtered) == 0 {
		return ""
	}
	if len(filtered) <= 4 {
		return strings.Join(filtered, ", ")
	}
	return fmt.Sprintf("%s, ... (%d items)", strings.Join(filtered[:4], ", "), len(filtered))
}

func lookupDiagnosticConfigValue(flattened map[string]string, paths ...string) string {
	if len(flattened) == 0 {
		return ""
	}
	for _, path := range paths {
		normalizedPath := strings.TrimSpace(strings.ToLower(path))
		if normalizedPath == "" {
			continue
		}
		if value := strings.TrimSpace(flattened[normalizedPath]); value != "" {
			return value
		}
	}
	for _, path := range paths {
		normalizedPath := strings.TrimSpace(strings.ToLower(path))
		if normalizedPath == "" {
			continue
		}
		for key, value := range flattened {
			if strings.HasSuffix(key, normalizedPath) && strings.TrimSpace(value) != "" {
				return value
			}
		}
	}
	return ""
}

func buildDiagnosticYAMLHighlightSpecs(configType string) []diagnosticConfigHighlightSpec {
	switch strings.TrimSpace(strings.ToLower(configType)) {
	case strings.ToLower(string(appconfig.ConfigTypeSeatunnel)):
		return []diagnosticConfigHighlightSpec{
			{Label: bilingualText("Metrics", "Metrics"), Paths: []string{"metrics.enabled"}},
			{Label: bilingualText("Prometheus", "Prometheus"), Paths: []string{"metrics.prometheus.enabled"}},
			{Label: bilingualText("JMX", "JMX"), Paths: []string{"metrics.jmx.enabled"}},
			{Label: bilingualText("备份副本", "Backup Count"), Paths: []string{"seatunnel.engine.backup-count"}},
			{Label: bilingualText("Checkpoint 间隔", "Checkpoint Interval"), Paths: []string{"seatunnel.engine.checkpoint.interval"}},
			{Label: bilingualText("HTTP 端口", "HTTP Port"), Paths: []string{"seatunnel.engine.http.port", "seatunnel.engine.rest.port"}},
		}
	case strings.ToLower(string(appconfig.ConfigTypeHazelcast)),
		strings.ToLower(string(appconfig.ConfigTypeHazelcastMaster)),
		strings.ToLower(string(appconfig.ConfigTypeHazelcastWorker)):
		return []diagnosticConfigHighlightSpec{
			{Label: bilingualText("集群名", "Cluster Name"), Paths: []string{"hazelcast.cluster-name"}},
			{Label: bilingualText("监听端口", "Listen Port"), Paths: []string{"hazelcast.network.port.port"}},
			{Label: bilingualText("TCP/IP Join", "TCP/IP Join"), Paths: []string{"hazelcast.network.join.tcp-ip.enabled"}},
			{Label: bilingualText("Multicast", "Multicast"), Paths: []string{"hazelcast.network.join.multicast.enabled"}},
			{Label: bilingualText("Partition Group", "Partition Group"), Paths: []string{"hazelcast.partition-group.enabled"}},
			{Label: bilingualText("CP Member", "CP Member"), Paths: []string{"hazelcast.cp-subsystem.cp-member-count"}},
		}
	case strings.ToLower(string(appconfig.ConfigTypeHazelcastClient)):
		return []diagnosticConfigHighlightSpec{
			{Label: bilingualText("集群名", "Cluster Name"), Paths: []string{"hazelcast-client.cluster-name"}},
			{Label: bilingualText("客户端地址", "Client Addresses"), Paths: []string{"hazelcast-client.network.cluster-members", "hazelcast-client.network.addresses"}},
			{Label: bilingualText("重连模式", "Reconnect Mode"), Paths: []string{"hazelcast-client.connection-strategy.reconnect-mode"}},
			{Label: bilingualText("异步启动", "Async Start"), Paths: []string{"hazelcast-client.connection-strategy.async-start"}},
		}
	default:
		return nil
	}
}

func extractDiagnosticJVMOptionHighlights(content string) []diagnosticConfigKeyValue {
	if strings.TrimSpace(content) == "" {
		return nil
	}
	lineMap := make(map[string]string)
	for _, rawLine := range strings.Split(content, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		switch {
		case strings.HasPrefix(line, "-Xms"):
			lineMap["xms"] = strings.TrimPrefix(line, "-Xms")
		case strings.HasPrefix(line, "-Xmx"):
			lineMap["xmx"] = strings.TrimPrefix(line, "-Xmx")
		case strings.HasPrefix(line, "-XX:MaxDirectMemorySize="):
			lineMap["direct_memory"] = strings.TrimPrefix(line, "-XX:MaxDirectMemorySize=")
		case strings.HasPrefix(line, "-XX:MaxMetaspaceSize="):
			lineMap["metaspace"] = strings.TrimPrefix(line, "-XX:MaxMetaspaceSize=")
		case strings.HasPrefix(line, "-XX:HeapDumpPath="):
			lineMap["heap_dump_path"] = strings.TrimPrefix(line, "-XX:HeapDumpPath=")
		case line == "-XX:+HeapDumpOnOutOfMemoryError":
			lineMap["heap_dump_on_oom"] = "true"
		case line == "-XX:+ExitOnOutOfMemoryError":
			lineMap["exit_on_oom"] = "true"
		case strings.HasPrefix(line, "-XX:OnOutOfMemoryError="):
			lineMap["on_oom"] = strings.TrimPrefix(line, "-XX:OnOutOfMemoryError=")
		}
	}
	specs := []struct {
		key   string
		label string
	}{
		{key: "xms", label: "Xms"},
		{key: "xmx", label: "Xmx"},
		{key: "direct_memory", label: bilingualText("直接内存", "Direct Memory")},
		{key: "metaspace", label: bilingualText("Metaspace", "Metaspace")},
		{key: "heap_dump_on_oom", label: bilingualText("OOM HeapDump", "OOM HeapDump")},
		{key: "heap_dump_path", label: bilingualText("HeapDump 路径", "HeapDump Path")},
		{key: "exit_on_oom", label: bilingualText("OOM 退出", "Exit On OOM")},
		{key: "on_oom", label: bilingualText("OOM Hook", "OOM Hook")},
	}
	result := make([]diagnosticConfigKeyValue, 0, len(specs))
	for _, spec := range specs {
		if value := strings.TrimSpace(lineMap[spec.key]); value != "" {
			result = append(result, diagnosticConfigKeyValue{
				Label: spec.label,
				Value: truncateString(value, 120),
			})
		}
	}
	return result
}

func extractDiagnosticLog4jHighlights(content string) []diagnosticConfigKeyValue {
	properties := parseDiagnosticProperties(content)
	if len(properties) == 0 {
		return nil
	}
	result := make([]diagnosticConfigKeyValue, 0, 4)
	if level := strings.TrimSpace(properties["rootlogger.level"]); level != "" {
		result = append(result, diagnosticConfigKeyValue{
			Label: bilingualText("Root 日志级别", "Root Log Level"),
			Value: truncateString(level, 120),
		})
	}
	if fileName := firstNonEmptyString(properties["appender.rolling.filename"], properties["appender.file.filename"]); strings.TrimSpace(fileName) != "" {
		result = append(result, diagnosticConfigKeyValue{
			Label: bilingualText("主日志文件", "Primary Log File"),
			Value: truncateString(fileName, 120),
		})
	}
	routingEnabled := false
	routingPattern := ""
	for key, value := range properties {
		if strings.Contains(key, "routing") {
			routingEnabled = true
		}
		if key == "appender.routing.routes.pattern" {
			routingPattern = value
		}
	}
	if routingEnabled {
		value := bilingualText("已启用", "enabled")
		if strings.TrimSpace(routingPattern) != "" {
			value = truncateString(fmt.Sprintf("%s (%s)", value, routingPattern), 120)
		}
		result = append(result, diagnosticConfigKeyValue{
			Label: bilingualText("RoutingAppender", "RoutingAppender"),
			Value: value,
		})
	}
	return result
}

func parseDiagnosticProperties(content string) map[string]string {
	properties := make(map[string]string)
	for _, rawLine := range strings.Split(content, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}
		index := strings.IndexAny(line, "=:")
		if index <= 0 {
			continue
		}
		key := strings.TrimSpace(strings.ToLower(line[:index]))
		value := strings.TrimSpace(line[index+1:])
		if key == "" || value == "" {
			continue
		}
		properties[key] = value
	}
	return properties
}

func extractDiagnosticPluginConfigHighlights(remotePath, content string) []diagnosticConfigKeyValue {
	count := 0
	for _, rawLine := range strings.Split(content, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		count++
	}
	if count == 0 {
		return nil
	}
	return []diagnosticConfigKeyValue{
		{
			Label: bilingualText("插件规则数", "Plugin Rules"),
			Value: fmt.Sprintf("%d", count),
		},
		{
			Label: bilingualText("来源文件", "Source File"),
			Value: truncateString(filepath.Base(strings.TrimSpace(remotePath)), 120),
		},
	}
}

func buildDiagnosticConfigPreview(content string) string {
	if strings.TrimSpace(content) == "" {
		return ""
	}
	return strings.TrimSpace(content)
}

func buildDiagnosticExtraConfigFilesForTarget(mode cluster.DeploymentMode, role string) []string {
	items := []string{
		"seatunnel-env.sh",
		"log4j2.properties",
		"log4j2_client.properties",
		"plugin_config",
		"jvm_client_options",
	}
	switch mode {
	case cluster.DeploymentModeSeparated:
		switch strings.TrimSpace(role) {
		case string(cluster.NodeRoleMaster):
			items = append(items, "jvm_master_options")
		case string(cluster.NodeRoleWorker):
			items = append(items, "jvm_worker_options")
		case string(cluster.NodeRoleMasterWorker):
			items = append(items, "jvm_master_options", "jvm_worker_options")
		default:
			items = append(items, "jvm_master_options", "jvm_worker_options")
		}
	default:
		items = append(items, "jvm_options")
	}
	return deduplicateDiagnosticStrings(items)
}

func buildDiagnosticConfigChangeRecords(configs []*appconfig.Config) []diagnosticConfigChangeRecord {
	if len(configs) == 0 {
		return []diagnosticConfigChangeRecord{}
	}
	result := make([]diagnosticConfigChangeRecord, 0, len(configs))
	for _, item := range configs {
		if item == nil {
			continue
		}
		hostID := item.HostID
		result = append(result, diagnosticConfigChangeRecord{
			ConfigID:   item.ID,
			ClusterID:  item.ClusterID,
			HostID:     hostID,
			HostScope:  buildDiagnosticConfigChangeScope(item),
			ConfigType: string(item.ConfigType),
			FilePath:   item.FilePath,
			Version:    item.Version,
			UpdatedAt:  item.UpdatedAt,
			UpdatedBy:  item.UpdatedBy,
			IsTemplate: item.IsTemplate(),
		})
	}
	return result
}

func buildDiagnosticConfigChangeScope(item *appconfig.Config) string {
	if item == nil {
		return "-"
	}
	if item.IsTemplate() {
		return "template"
	}
	if item.HostID != nil {
		return fmt.Sprintf("host #%d", *item.HostID)
	}
	return "node"
}

func buildDiagnosticConfigTypesForTarget(mode cluster.DeploymentMode, role string) []appconfig.ConfigType {
	result := []appconfig.ConfigType{appconfig.ConfigTypeSeatunnel}
	switch mode {
	case cluster.DeploymentModeSeparated:
		switch strings.TrimSpace(role) {
		case string(cluster.NodeRoleMaster):
			result = append(result, appconfig.ConfigTypeHazelcastMaster)
		case string(cluster.NodeRoleWorker):
			result = append(result, appconfig.ConfigTypeHazelcastWorker)
		case string(cluster.NodeRoleMasterWorker):
			result = append(result, appconfig.ConfigTypeHazelcastMaster, appconfig.ConfigTypeHazelcastWorker)
		default:
			result = append(result, appconfig.ConfigTypeHazelcastMaster, appconfig.ConfigTypeHazelcastWorker)
		}
	default:
		result = append(result, appconfig.ConfigTypeHazelcast)
	}
	result = append(result, appconfig.ConfigTypeHazelcastClient)
	return deduplicateDiagnosticConfigTypes(result)
}

func deduplicateDiagnosticConfigTypes(items []appconfig.ConfigType) []appconfig.ConfigType {
	if len(items) == 0 {
		return []appconfig.ConfigType{}
	}
	result := make([]appconfig.ConfigType, 0, len(items))
	seen := make(map[appconfig.ConfigType]struct{}, len(items))
	for _, item := range items {
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}

func deduplicateDiagnosticStrings(items []string) []string {
	if len(items) == 0 {
		return []string{}
	}
	result := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}

func detectDiagnosticConfigFormat(name string) string {
	trimmed := strings.TrimSpace(strings.ToLower(name))
	switch {
	case strings.HasSuffix(trimmed, ".yaml"), strings.HasSuffix(trimmed, ".yml"):
		return "yaml"
	case strings.HasSuffix(trimmed, ".properties"):
		return "properties"
	default:
		return "text"
	}
}

func buildDiagnosticContentHash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", sum[:8])
}

func (s *Service) executeCollectLogSampleStep(ctx context.Context, task *DiagnosticTask, step *DiagnosticTaskStep, nodesByClusterNodeID map[uint]*DiagnosticNodeExecution, state *diagnosticBundleExecutionState, bundleDir string) error {
	if s.agentSender == nil {
		return fmt.Errorf("agent sender is unavailable")
	}
	window := resolveDiagnosticCollectionWindow(task, state.InspectionDetail)
	state.WindowStart = timePtr(window.Start)
	state.WindowEnd = timePtr(window.End)
	state.LookbackMinutes = window.LookbackMinutes
	logDir := filepath.Join(bundleDir, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return err
	}
	var successCount int
	var errs []string
	for _, selected := range sortedDiagnosticTaskTargets(task.SelectedNodes) {
		node := nodesByClusterNodeID[selected.ClusterNodeID]
		if node == nil {
			continue
		}
		if err := s.beginDiagnosticNodeStep(ctx, step, node, bilingualText("正在采集日志样本。", "Collecting log samples.")); err != nil {
			return err
		}
		candidates := buildDiagnosticLogCandidates(selected, state.ErrorEvents)
		var nodeSuccess bool
		for _, candidate := range candidates {
			snippet, detail, err := s.collectDiagnosticWindowedLogSnippet(ctx, selected, candidate, window)
			if err != nil || strings.TrimSpace(snippet) == "" {
				detail = firstNonEmptyString(detail, bilingualText("日志样本采集失败。", "Failed to collect log sample."))
				_ = s.AppendDiagnosticStepLog(ctx, &DiagnosticStepLog{
					TaskID:          task.ID,
					TaskStepID:      uintPtr(step.ID),
					NodeExecutionID: uintPtr(node.ID),
					StepCode:        step.Code,
					Level:           DiagnosticLogLevelWarn,
					EventType:       DiagnosticLogEventTypeNote,
					Message:         bilingualText(fmt.Sprintf("从 %s 采集日志样本失败：%s", candidate, detail), fmt.Sprintf("Failed to collect log sample from %s: %s", candidate, detail)),
					CommandSummary:  candidate,
					CreatedAt:       time.Now().UTC(),
				})
				continue
			}
			fileName := buildDiagnosticLogSampleFileName(selected.HostID, selected.HostName, candidate)
			localPath := filepath.Join(logDir, fileName)
			safeSnippet := audit.RedactText(snippet)
			if err := os.WriteFile(localPath, []byte(safeSnippet), 0o644); err != nil {
				return err
			}
			state.Artifacts = append(state.Artifacts, &diagnosticBundleArtifact{
				StepCode:  step.Code,
				Category:  "log_sample",
				Format:    "log",
				Status:    "created",
				Path:      localPath,
				NodeID:    selected.NodeID,
				HostID:    selected.HostID,
				HostName:  selected.HostName,
				SizeBytes: int64(len(safeSnippet)),
				Message:   candidate,
			})
			state.LogSamples = append(state.LogSamples, diagnosticCollectedLogSample{
				HostID:      selected.HostID,
				HostName:    selected.HostName,
				HostIP:      selected.HostIP,
				NodeID:      selected.NodeID,
				Role:        selected.Role,
				SourceFile:  candidate,
				LocalPath:   localPath,
				WindowStart: window.Start,
				WindowEnd:   window.End,
				Content:     safeSnippet,
			})
			_ = s.AppendDiagnosticStepLog(ctx, &DiagnosticStepLog{
				TaskID:          task.ID,
				TaskStepID:      uintPtr(step.ID),
				NodeExecutionID: uintPtr(node.ID),
				StepCode:        step.Code,
				Level:           DiagnosticLogLevelInfo,
				EventType:       DiagnosticLogEventTypeSuccess,
				Message:         bilingualText(fmt.Sprintf("已从 %s 采集日志样本", candidate), fmt.Sprintf("Collected log sample from %s", candidate)),
				CommandSummary:  candidate,
				CreatedAt:       time.Now().UTC(),
			})
			nodeSuccess = true
			successCount++
			break
		}
		if nodeSuccess {
			if err := s.finishDiagnosticNodeStep(ctx, step, node, DiagnosticTaskStatusSucceeded, bilingualText("日志样本采集完成。", "Log sample collected.")); err != nil {
				return err
			}
			continue
		}
		errs = append(errs, fmt.Sprintf("host=%d", selected.HostID))
		if err := s.finishDiagnosticNodeStep(ctx, step, node, DiagnosticTaskStatusFailed, bilingualText("未采集到日志样本。", "No log sample collected.")); err != nil {
			return err
		}
	}
	if successCount == 0 {
		return formatDiagnosticAllNodesFailed("全部节点都未采集到日志样本", "No log samples collected on any node", errs)
	}
	sort.SliceStable(state.LogSamples, func(i, j int) bool {
		if state.LogSamples[i].HostID != state.LogSamples[j].HostID {
			return state.LogSamples[i].HostID < state.LogSamples[j].HostID
		}
		return state.LogSamples[i].SourceFile < state.LogSamples[j].SourceFile
	})
	return nil
}

func (s *Service) executeCollectThreadDumpStep(ctx context.Context, task *DiagnosticTask, step *DiagnosticTaskStep, nodesByClusterNodeID map[uint]*DiagnosticNodeExecution, state *diagnosticBundleExecutionState, bundleDir string) error {
	if s.agentSender == nil {
		return fmt.Errorf("agent sender is unavailable")
	}
	outputDir := filepath.Join(bundleDir, "thread-dumps")
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return err
	}
	var successCount int
	var errs []string
	for _, selected := range sortedDiagnosticTaskTargets(task.SelectedNodes) {
		node := nodesByClusterNodeID[selected.ClusterNodeID]
		if node == nil {
			continue
		}
		if err := s.beginDiagnosticNodeStep(ctx, step, node, bilingualText("正在采集线程栈。", "Collecting thread dump.")); err != nil {
			return err
		}
		success, output, err := s.sendDiagnosticAgentCommandWithRetry(ctx, selected.AgentID, "thread_dump", map[string]string{
			"install_dir": selected.InstallDir,
			"role":        selected.Role,
		})
		if err != nil || !success {
			detail := resolveDiagnosticCommandFailure(output, err, "线程栈采集失败。", "Thread dump failed.")
			errs = append(errs, fmt.Sprintf("host=%d: %s", selected.HostID, detail))
			_ = s.AppendDiagnosticStepLog(ctx, &DiagnosticStepLog{
				TaskID:          task.ID,
				TaskStepID:      uintPtr(step.ID),
				NodeExecutionID: uintPtr(node.ID),
				StepCode:        step.Code,
				Level:           DiagnosticLogLevelError,
				EventType:       DiagnosticLogEventTypeFailed,
				Message:         detail,
				CommandSummary:  selected.Role,
				CreatedAt:       time.Now().UTC(),
			})
			if finishErr := s.finishDiagnosticNodeStep(ctx, step, node, DiagnosticTaskStatusFailed, detail); finishErr != nil {
				return finishErr
			}
			continue
		}
		var result agentThreadDumpResult
		if err := json.Unmarshal([]byte(output), &result); err != nil {
			return err
		}
		localPath := filepath.Join(outputDir, fmt.Sprintf("thread-dump-host-%d-%s.txt", selected.HostID, normalizeDiagnosticFileRole(selected.Role)))
		if err := os.WriteFile(localPath, []byte(result.Content), 0o644); err != nil {
			return err
		}
		state.Artifacts = append(state.Artifacts, &diagnosticBundleArtifact{
			StepCode:   step.Code,
			Category:   "thread_dump",
			Format:     "txt",
			Status:     result.Status,
			Path:       localPath,
			RemotePath: result.OutputPath,
			NodeID:     selected.NodeID,
			HostID:     selected.HostID,
			HostName:   selected.HostName,
			SizeBytes:  result.SizeBytes,
			Message:    result.Tool,
		})
		if err := s.AppendDiagnosticStepLog(ctx, &DiagnosticStepLog{
			TaskID:          task.ID,
			TaskStepID:      uintPtr(step.ID),
			NodeExecutionID: uintPtr(node.ID),
			StepCode:        step.Code,
			Level:           DiagnosticLogLevelInfo,
			EventType:       DiagnosticLogEventTypeSuccess,
			Message:         bilingualText(fmt.Sprintf("线程栈已保存到 %s", localPath), fmt.Sprintf("Thread dump collected to %s", localPath)),
			CommandSummary:  result.Tool,
			CreatedAt:       time.Now().UTC(),
			Metadata: DiagnosticLogMetadata{
				"remote_path": result.OutputPath,
			},
		}); err != nil {
			return err
		}
		if err := s.finishDiagnosticNodeStep(ctx, step, node, DiagnosticTaskStatusSucceeded, bilingualText("线程栈采集完成。", "Thread dump collected.")); err != nil {
			return err
		}
		successCount++
	}
	if successCount == 0 {
		return formatDiagnosticAllNodesFailed("全部节点线程栈采集失败", "Thread dump failed on all nodes", errs)
	}
	return nil
}

func (s *Service) executeCollectJVMDumpStep(ctx context.Context, task *DiagnosticTask, step *DiagnosticTaskStep, nodesByClusterNodeID map[uint]*DiagnosticNodeExecution, state *diagnosticBundleExecutionState, bundleDir string) error {
	if s.agentSender == nil {
		return fmt.Errorf("agent sender is unavailable")
	}
	outputDir := filepath.Join(bundleDir, "jvm-dumps")
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return err
	}
	var successCount int
	var skippedCount int
	var errs []string
	for _, selected := range sortedDiagnosticTaskTargets(task.SelectedNodes) {
		node := nodesByClusterNodeID[selected.ClusterNodeID]
		if node == nil {
			continue
		}
		if err := s.beginDiagnosticNodeStep(ctx, step, node, bilingualText("正在采集 JVM Dump。", "Collecting JVM dump.")); err != nil {
			return err
		}
		success, output, err := s.sendDiagnosticAgentCommandWithRetry(ctx, selected.AgentID, "jvm_dump", map[string]string{
			"install_dir": selected.InstallDir,
			"role":        selected.Role,
			"min_free_mb": fmt.Sprintf("%d", task.Options.JVMDumpMinFreeMB),
		})
		if err != nil || !success {
			detail := resolveDiagnosticCommandFailure(output, err, "JVM Dump 采集失败。", "JVM dump failed.")
			errs = append(errs, fmt.Sprintf("host=%d: %s", selected.HostID, detail))
			_ = s.AppendDiagnosticStepLog(ctx, &DiagnosticStepLog{
				TaskID:          task.ID,
				TaskStepID:      uintPtr(step.ID),
				NodeExecutionID: uintPtr(node.ID),
				StepCode:        step.Code,
				Level:           DiagnosticLogLevelError,
				EventType:       DiagnosticLogEventTypeFailed,
				Message:         detail,
				CommandSummary:  selected.Role,
				CreatedAt:       time.Now().UTC(),
			})
			if finishErr := s.finishDiagnosticNodeStep(ctx, step, node, DiagnosticTaskStatusFailed, detail); finishErr != nil {
				return finishErr
			}
			continue
		}
		var result agentJVMDumpResult
		if err := json.Unmarshal([]byte(output), &result); err != nil {
			return err
		}
		localPath := filepath.Join(outputDir, fmt.Sprintf("jvm-dump-host-%d-%s.json", selected.HostID, normalizeDiagnosticFileRole(selected.Role)))
		if err := os.WriteFile(localPath, []byte(output), 0o644); err != nil {
			return err
		}
		status := DiagnosticTaskStatusSucceeded
		if result.Status == "skipped" {
			status = DiagnosticTaskStatusSkipped
			skippedCount++
		} else {
			successCount++
		}
		state.Artifacts = append(state.Artifacts, &diagnosticBundleArtifact{
			StepCode:   step.Code,
			Category:   "jvm_dump",
			Format:     "json",
			Status:     result.Status,
			Path:       localPath,
			RemotePath: result.OutputPath,
			NodeID:     selected.NodeID,
			HostID:     selected.HostID,
			HostName:   selected.HostName,
			SizeBytes:  result.SizeBytes,
			Message:    result.Message,
		})
		if err := s.AppendDiagnosticStepLog(ctx, &DiagnosticStepLog{
			TaskID:          task.ID,
			TaskStepID:      uintPtr(step.ID),
			NodeExecutionID: uintPtr(node.ID),
			StepCode:        step.Code,
			Level:           DiagnosticLogLevelInfo,
			EventType:       DiagnosticLogEventTypeNote,
			Message:         result.Message,
			CommandSummary:  result.Tool,
			CreatedAt:       time.Now().UTC(),
			Metadata: DiagnosticLogMetadata{
				"remote_path":    result.OutputPath,
				"free_bytes":     result.FreeBytes,
				"required_bytes": result.RequiredBytes,
			},
		}); err != nil {
			return err
		}
		if err := s.finishDiagnosticNodeStep(ctx, step, node, status, result.Message); err != nil {
			return err
		}
	}
	if successCount == 0 && skippedCount == 0 {
		return formatDiagnosticAllNodesFailed("全部节点 JVM Dump 采集失败", "JVM dump failed on all nodes", errs)
	}
	return nil
}

func (s *Service) executeAssembleManifestStep(ctx context.Context, task *DiagnosticTask, step *DiagnosticTaskStep, state *diagnosticBundleExecutionState, bundleDir string) error {
	manifestPath := filepath.Join(bundleDir, "manifest.json")
	manifestArtifact := &diagnosticBundleArtifact{
		StepCode: step.Code,
		Category: "manifest",
		Format:   "json",
		Status:   "created",
		Path:     manifestPath,
		Message:  bilingualText("诊断包 Manifest", "Diagnostic bundle manifest"),
	}
	if err := writeDiagnosticBundleManifestFile(manifestPath, task, append(cloneDiagnosticArtifacts(state.Artifacts), manifestArtifact), state); err != nil {
		return err
	}
	fileInfo, err := os.Stat(manifestPath)
	if err != nil {
		return err
	}
	manifestArtifact.SizeBytes = fileInfo.Size()
	task.ManifestPath = manifestPath
	if err := s.UpdateDiagnosticTask(ctx, task); err != nil {
		return err
	}
	state.Artifacts = append(state.Artifacts, manifestArtifact)
	return nil
}

func (s *Service) executeRenderHTMLSummaryStep(ctx context.Context, task *DiagnosticTask, step *DiagnosticTaskStep, state *diagnosticBundleExecutionState, bundleDir string) error {
	indexPath := filepath.Join(bundleDir, "index.html")
	htmlArtifact := &diagnosticBundleArtifact{
		StepCode: step.Code,
		Category: "diagnostic_report",
		Format:   "html",
		Status:   "created",
		Path:     indexPath,
		Message:  bilingualText("离线诊断报告", "Offline diagnostic report"),
	}

	// 重新拉取最新的任务步骤与节点执行状态，确保报告中附录步骤与节点不再显示初始等待状态
	// Re-fetch latest task steps and node executions from repo to ensure appendix steps and nodes do not show initial pending state
	if s.repo != nil {
		if freshSteps, err := s.repo.ListDiagnosticTaskSteps(ctx, task.ID); err == nil && len(freshSteps) > 0 {
			task.Steps = make([]DiagnosticTaskStep, 0, len(freshSteps))
			for _, st := range freshSteps {
				if st != nil {
					task.Steps = append(task.Steps, *st)
				}
			}
		}
		if freshNodes, err := s.repo.ListDiagnosticNodeExecutions(ctx, task.ID); err == nil && len(freshNodes) > 0 {
			task.NodeExecutions = make([]DiagnosticNodeExecution, 0, len(freshNodes))
			for _, nd := range freshNodes {
				if nd != nil {
					task.NodeExecutions = append(task.NodeExecutions, *nd)
				}
			}
		}
	}
	now := time.Now().UTC()
	for i := range task.Steps {
		if task.Steps[i].Code == step.Code {
			task.Steps[i].Status = DiagnosticTaskStatusSucceeded
			if task.Steps[i].StartedAt == nil {
				task.Steps[i].StartedAt = &now
			}
			task.Steps[i].CompletedAt = &now
			task.Steps[i].Message = bilingualText("离线诊断报告生成完成。", "Offline diagnostic report generated.")
		}
	}

	payload := buildDiagnosticBundleHTMLPayload(task, state, bundleDir, append(cloneDiagnosticArtifacts(state.Artifacts), htmlArtifact))
	renderTargets := []struct {
		Path string
		Lang DiagnosticLanguage
	}{
		{Path: indexPath, Lang: DiagnosticLanguageZH},
		{Path: filepath.Join(bundleDir, "index.zh.html"), Lang: DiagnosticLanguageZH},
		{Path: filepath.Join(bundleDir, "index.en.html"), Lang: DiagnosticLanguageEN},
	}
	var primaryContent []byte
	for _, target := range renderTargets {
		content, err := renderDiagnosticBundleHTMLDocument(payload, target.Lang)
		if err != nil {
			return err
		}
		if err := os.WriteFile(target.Path, content, 0o644); err != nil {
			return err
		}
		if target.Path == indexPath {
			primaryContent = content
		}
	}
	task.IndexPath = indexPath
	if err := s.UpdateDiagnosticTask(ctx, task); err != nil {
		return err
	}
	htmlArtifact.SizeBytes = int64(len(primaryContent))
	state.Artifacts = append(state.Artifacts, htmlArtifact)
	if task.ManifestPath != "" {
		if err := writeDiagnosticBundleManifestFile(task.ManifestPath, task, state.Artifacts, state); err != nil {
			return err
		}
	}
	return nil
}

func resolveDiagnosticCollectionWindow(task *DiagnosticTask, inspectionDetail *ClusterInspectionReportDetailData) diagnosticCollectionWindow {
	end := time.Now().UTC()
	lookbackMinutes := 0
	taskOverridesWindow := false
	if inspectionDetail != nil && inspectionDetail.Report != nil {
		report := inspectionDetail.Report
		if report.FinishedAt != nil && !report.FinishedAt.IsZero() {
			end = report.FinishedAt.UTC()
		} else if report.StartedAt != nil && !report.StartedAt.IsZero() {
			end = report.StartedAt.UTC()
		}
		if report.LookbackMinutes >= minInspectionLookbackMinutes && report.LookbackMinutes <= maxInspectionLookbackMinutes {
			lookbackMinutes = report.LookbackMinutes
		}
	}
	if task != nil && task.LookbackMinutes >= minInspectionLookbackMinutes && task.LookbackMinutes <= maxInspectionLookbackMinutes {
		lookbackMinutes = task.LookbackMinutes
		taskOverridesWindow = true
	}
	if taskOverridesWindow {
		end = time.Now().UTC()
	}
	if lookbackMinutes == 0 {
		lookbackMinutes = defaultInspectionLookbackMinutes
	}
	start := end.Add(-time.Duration(lookbackMinutes) * time.Minute)
	return diagnosticCollectionWindow{
		Start:           start,
		End:             end,
		LookbackMinutes: lookbackMinutes,
	}
}

func filterDiagnosticProcessEventsByWindow(events []*monitor.ProcessEvent, start, end time.Time) []*monitor.ProcessEvent {
	if len(events) == 0 {
		return []*monitor.ProcessEvent{}
	}
	filtered := make([]*monitor.ProcessEvent, 0, len(events))
	for _, event := range events {
		if event == nil {
			continue
		}
		createdAt := event.CreatedAt.UTC()
		if createdAt.Before(start) || createdAt.After(end) {
			continue
		}
		filtered = append(filtered, event)
	}
	return filtered
}

func filterDiagnosticAlertsByWindow(alerts []*monitoringapp.AlertInstance, start, end time.Time, sourceAlertID string) []*monitoringapp.AlertInstance {
	if len(alerts) == 0 {
		return []*monitoringapp.AlertInstance{}
	}
	sourceAlertID = strings.TrimSpace(sourceAlertID)
	filtered := make([]*monitoringapp.AlertInstance, 0, len(alerts))
	for _, alert := range alerts {
		if alert == nil {
			continue
		}
		if sourceAlertID != "" && strings.TrimSpace(alert.AlertID) == sourceAlertID {
			filtered = append(filtered, alert)
			continue
		}
		if shouldIncludeDiagnosticAlert(alert, start, end) {
			filtered = append(filtered, alert)
		}
	}
	return filtered
}

func shouldIncludeDiagnosticAlert(alert *monitoringapp.AlertInstance, start, end time.Time) bool {
	if alert == nil {
		return false
	}
	switch alert.Status {
	case monitoringapp.AlertDisplayStatusFiring:
		firingAt := alert.FiringAt.UTC()
		lastSeenAt := alert.LastSeenAt.UTC()
		return !firingAt.After(end) && !lastSeenAt.Before(start)
	case monitoringapp.AlertDisplayStatusResolved:
		if alert.ResolvedAt != nil && !alert.ResolvedAt.IsZero() {
			resolvedAt := alert.ResolvedAt.UTC()
			return !resolvedAt.Before(start) && !resolvedAt.After(end)
		}
		lastSeenAt := alert.LastSeenAt.UTC()
		return !lastSeenAt.Before(start) && !lastSeenAt.After(end)
	default:
		return false
	}
}

func timePtr(value time.Time) *time.Time {
	normalized := value.UTC()
	return &normalized
}

type diagnosticPrometheusSignalSpec struct {
	Key            string
	Title          string
	Unit           string
	Threshold      float64
	ThresholdText  string
	StatusOnBreach string
	Comparator     string
	PromQL         string
}

type diagnosticPrometheusQueryRangeResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string                                 `json:"resultType"`
		Result     []diagnosticPrometheusQueryRangeVector `json:"result"`
	} `json:"data"`
	Error string `json:"error,omitempty"`
}

type diagnosticPrometheusQueryRangeVector struct {
	Metric map[string]string `json:"metric"`
	Values [][]interface{}   `json:"values"`
}

func (s *Service) collectDiagnosticPrometheusSnapshot(ctx context.Context, task *DiagnosticTask, window diagnosticCollectionWindow) (*diagnosticPrometheusSnapshot, error) {
	baseURL := strings.TrimSpace(config.Config.Observability.Prometheus.URL)
	if baseURL == "" || task == nil || task.ClusterID == 0 {
		return nil, nil
	}
	specs := buildDiagnosticPrometheusSignalSpecs(task.ClusterID)
	if len(specs) == 0 {
		return nil, nil
	}
	stepSeconds := computeDiagnosticPrometheusStepSeconds(window)
	snapshot := &diagnosticPrometheusSnapshot{
		ClusterID:   task.ClusterID,
		WindowStart: window.Start,
		WindowEnd:   window.End,
		StepSeconds: stepSeconds,
		Signals:     make([]diagnosticPrometheusSignal, 0, len(specs)),
	}
	for _, spec := range specs {
		signal, err := queryDiagnosticPrometheusSignal(ctx, baseURL, window, stepSeconds, spec)
		if err != nil {
			snapshot.CollectionNotes = append(snapshot.CollectionNotes, fmt.Sprintf("%s: %v", spec.Key, err))
			continue
		}
		snapshot.Signals = append(snapshot.Signals, *signal)
	}
	if len(snapshot.Signals) == 0 && len(snapshot.CollectionNotes) == 0 {
		return nil, nil
	}
	return snapshot, nil
}

func buildDiagnosticPrometheusSignalSpecs(clusterID uint) []diagnosticPrometheusSignalSpec {
	matchers := []string{
		`job="seatunnel_engine_http"`,
		fmt.Sprintf(`cluster_id="%d"`, clusterID),
	}
	oldGenPoolMatcher := `pool=~"G1 Old Gen|PS Old Gen|CMS Old Gen|Tenured Gen"`
	selector := func(metric string, extra ...string) string {
		items := make([]string, 0, len(matchers)+len(extra))
		items = append(items, matchers...)
		items = append(items, extra...)
		return buildDiagnosticPrometheusSelector(metric, items...)
	}
	return []diagnosticPrometheusSignalSpec{
		{
			Key:            "cpu_usage_high",
			Title:          bilingualText("CPU 负载", "CPU Load"),
			Unit:           "cores",
			Threshold:      0.8,
			ThresholdText:  "> 0.8 cores",
			StatusOnBreach: "warning",
			Comparator:     "gt",
			PromQL:         fmt.Sprintf(`sum by (instance) (rate(%s[5m]))`, selector("process_cpu_seconds_total")),
		},
		{
			Key:            "memory_usage_high",
			Title:          bilingualText("JVM Heap 使用率", "JVM Heap Usage"),
			Unit:           "ratio",
			Threshold:      0.8,
			ThresholdText:  "> 80%",
			StatusOnBreach: "warning",
			Comparator:     "gt",
			PromQL: fmt.Sprintf(`max by (instance) ((%s) / clamp_min((%s), 1))`,
				selector("jvm_memory_bytes_used", `area="heap"`),
				selector("jvm_memory_bytes_max", `area="heap"`),
			),
		},
		{
			Key:            "fd_usage_high",
			Title:          bilingualText("FD 使用率", "FD Usage"),
			Unit:           "ratio",
			Threshold:      0.8,
			ThresholdText:  "> 80%",
			StatusOnBreach: "warning",
			Comparator:     "gt",
			PromQL: fmt.Sprintf(`max by (instance) ((%s) / clamp_min((%s), 1))`,
				selector("process_open_fds"),
				selector("process_max_fds"),
			),
		},
		{
			Key:            "old_gen_usage_high",
			Title:          bilingualText("Old Gen 使用率", "Old Gen Usage"),
			Unit:           "ratio",
			Threshold:      0.8,
			ThresholdText:  "> 80%",
			StatusOnBreach: "warning",
			Comparator:     "gt",
			PromQL: fmt.Sprintf(`max by (instance) ((%s) / clamp_min((%s), 1))`,
				selector("jvm_memory_pool_bytes_used", oldGenPoolMatcher),
				selector("jvm_memory_pool_bytes_max", oldGenPoolMatcher),
			),
		},
		{
			Key:            "gc_time_ratio_high",
			Title:          bilingualText("GC 时间占比", "GC Time Ratio"),
			Unit:           "percent",
			Threshold:      10,
			ThresholdText:  "> 10% (5m)",
			StatusOnBreach: "warning",
			Comparator:     "gt",
			PromQL:         fmt.Sprintf(`100 * sum by (instance) (rate(%s[5m]))`, selector("jvm_gc_collection_seconds_sum")),
		},
		{
			Key:            "deadlocked_threads_detected",
			Title:          bilingualText("死锁线程", "Deadlocked Threads"),
			Unit:           "count",
			Threshold:      0,
			ThresholdText:  "> 0",
			StatusOnBreach: "critical",
			Comparator:     "gt",
			PromQL:         fmt.Sprintf(`max by (instance) (%s)`, selector("jvm_threads_deadlocked")),
		},
		{
			Key:            "job_thread_pool_rejection_high",
			Title:          bilingualText("作业线程池拒绝数", "Job Thread Pool Rejections"),
			Unit:           "count",
			Threshold:      0,
			ThresholdText:  "> 0",
			StatusOnBreach: "critical",
			Comparator:     "gt",
			PromQL:         fmt.Sprintf(`sum by (instance) (increase(%s[5m]))`, selector("job_thread_pool_rejection_total")),
		},
		{
			Key:            "split_brain_risk",
			Title:          bilingualText("Hazelcast 分区安全", "Hazelcast Partition Safety"),
			Unit:           "bool",
			Threshold:      1,
			ThresholdText:  "< 1",
			StatusOnBreach: "critical",
			Comparator:     "lt",
			PromQL:         fmt.Sprintf(`min by (instance) (%s)`, selector("hazelcast_partition_isClusterSafe")),
		},
	}
}

func queryDiagnosticPrometheusSignal(ctx context.Context, baseURL string, window diagnosticCollectionWindow, stepSeconds int, spec diagnosticPrometheusSignalSpec) (*diagnosticPrometheusSignal, error) {
	series, err := queryDiagnosticPrometheusRange(ctx, baseURL, spec.PromQL, window.Start, window.End, stepSeconds)
	if err != nil {
		return nil, err
	}
	summaries := make([]diagnosticPrometheusSeriesSummary, 0, len(series))
	breachCount := 0
	for _, item := range series {
		summary, breached := summarizeDiagnosticPrometheusSeries(item, spec.Comparator, spec.Threshold)
		if summary.Samples == 0 {
			continue
		}
		summaries = append(summaries, summary)
		if breached {
			breachCount++
		}
	}
	sort.SliceStable(summaries, func(i, j int) bool {
		return compareDiagnosticPrometheusSeries(summaries[i], summaries[j], spec.Comparator)
	})
	if len(summaries) > 5 {
		summaries = summaries[:5]
	}
	status := "healthy"
	summaryText := bilingualText("诊断窗口内未发现明显异常。", "No significant anomaly was detected in the diagnostics window.")
	if breachCount > 0 {
		status = spec.StatusOnBreach
		top := "-"
		if len(summaries) > 0 {
			top = summaries[0].Instance
		}
		summaryText = bilingualText(
			fmt.Sprintf("诊断窗口内共有 %d 个实例触达阈值，最突出实例：%s。", breachCount, top),
			fmt.Sprintf("%d instance(s) breached the threshold in this window, top instance: %s.", breachCount, top),
		)
	}
	return &diagnosticPrometheusSignal{
		Key:           spec.Key,
		Title:         spec.Title,
		Summary:       summaryText,
		PromQL:        spec.PromQL,
		Unit:          spec.Unit,
		Threshold:     spec.Threshold,
		ThresholdText: spec.ThresholdText,
		Status:        status,
		Comparator:    spec.Comparator,
		Series:        summaries,
	}, nil
}

func queryDiagnosticPrometheusRange(ctx context.Context, baseURL, promQL string, start, end time.Time, stepSeconds int) ([]diagnosticPrometheusQueryRangeVector, error) {
	apiURL := joinDiagnosticPrometheusURL(baseURL, "/api/v1/query_range")
	if apiURL == "" {
		return nil, fmt.Errorf("prometheus url is empty")
	}
	params := url.Values{}
	params.Set("query", promQL)
	params.Set("start", strconv.FormatFloat(float64(start.UTC().Unix()), 'f', -1, 64))
	params.Set("end", strconv.FormatFloat(float64(end.UTC().Unix()), 'f', -1, 64))
	params.Set("step", strconv.Itoa(stepSeconds))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 6 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		payload, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("prometheus query_range status %d: %s", resp.StatusCode, strings.TrimSpace(string(payload)))
	}
	var payload diagnosticPrometheusQueryRangeResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&payload); err != nil {
		return nil, err
	}
	if !strings.EqualFold(strings.TrimSpace(payload.Status), "success") {
		return nil, fmt.Errorf("prometheus query_range returned status %q: %s", payload.Status, strings.TrimSpace(payload.Error))
	}
	return payload.Data.Result, nil
}

func summarizeDiagnosticPrometheusSeries(item diagnosticPrometheusQueryRangeVector, comparator string, threshold float64) (diagnosticPrometheusSeriesSummary, bool) {
	summary := diagnosticPrometheusSeriesSummary{
		Instance: firstNonEmptyString(strings.TrimSpace(item.Metric["instance"]), strings.TrimSpace(item.Metric["cluster"]), "-"),
		MinValue: 0,
		MaxValue: 0,
		Points:   make([]diagnosticPrometheusPoint, 0, len(item.Values)),
	}
	if len(item.Values) == 0 {
		return summary, false
	}
	first := true
	breached := false
	for _, sample := range item.Values {
		if len(sample) < 2 {
			continue
		}
		value, ok := parseDiagnosticPrometheusSampleValue(sample[1])
		if !ok {
			continue
		}
		ts, tsOK := parseDiagnosticPrometheusSampleValue(sample[0])
		if first {
			summary.MinValue = value
			summary.MaxValue = value
			if tsOK {
				sampledAt := time.Unix(int64(ts), 0).UTC()
				summary.MinAt = &sampledAt
				summary.MaxAt = &sampledAt
			}
			first = false
		}
		if value < summary.MinValue {
			summary.MinValue = value
			if tsOK {
				sampledAt := time.Unix(int64(ts), 0).UTC()
				summary.MinAt = &sampledAt
			}
		}
		if value > summary.MaxValue {
			summary.MaxValue = value
			if tsOK {
				sampledAt := time.Unix(int64(ts), 0).UTC()
				summary.MaxAt = &sampledAt
			}
		}
		summary.LastValue = value
		if tsOK {
			sampledAt := time.Unix(int64(ts), 0).UTC()
			summary.LastAt = &sampledAt
			summary.Points = append(summary.Points, diagnosticPrometheusPoint{
				Timestamp: sampledAt,
				Value:     value,
			})
		}
		summary.Samples++
		switch comparator {
		case "lt":
			if value < threshold {
				breached = true
			}
		default:
			if value > threshold {
				breached = true
			}
		}
	}
	return summary, breached
}

func compareDiagnosticPrometheusSeries(left, right diagnosticPrometheusSeriesSummary, comparator string) bool {
	switch comparator {
	case "lt":
		if left.MinValue != right.MinValue {
			return left.MinValue < right.MinValue
		}
	default:
		if left.MaxValue != right.MaxValue {
			return left.MaxValue > right.MaxValue
		}
	}
	return left.Instance < right.Instance
}

func parseDiagnosticPrometheusSampleValue(value interface{}) (float64, bool) {
	switch typed := value.(type) {
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return parsed, err == nil
	case float64:
		return typed, true
	default:
		return 0, false
	}
}

func computeDiagnosticPrometheusStepSeconds(window diagnosticCollectionWindow) int {
	totalSeconds := int(window.End.Sub(window.Start).Seconds())
	if totalSeconds <= 0 {
		return 60
	}
	step := totalSeconds / 24
	if step < 60 {
		step = 60
	}
	if step > 300 {
		step = 300
	}
	return step
}

func buildDiagnosticPrometheusSelector(metric string, matchers ...string) string {
	items := make([]string, 0, len(matchers))
	for _, matcher := range matchers {
		matcher = strings.TrimSpace(matcher)
		if matcher == "" {
			continue
		}
		items = append(items, matcher)
	}
	if len(items) == 0 {
		return strings.TrimSpace(metric)
	}
	return fmt.Sprintf("%s{%s}", strings.TrimSpace(metric), strings.Join(items, ","))
}

func joinDiagnosticPrometheusURL(baseURL, suffix string) string {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		return ""
	}
	if suffix == "" {
		return base
	}
	if !strings.HasPrefix(suffix, "/") {
		suffix = "/" + suffix
	}
	return base + suffix
}

func (s *Service) beginDiagnosticTaskStep(ctx context.Context, task *DiagnosticTask, step *DiagnosticTaskStep) error {
	now := time.Now().UTC()
	updated, err := s.repo.UpdateDiagnosticTaskFields(ctx, task.ID, []DiagnosticTaskStatus{DiagnosticTaskStatusRunning}, map[string]any{
		"current_step": step.Code,
		"updated_at":   now,
	})
	if err != nil {
		return err
	}
	if !updated {
		return ErrDiagnosticTaskAlreadyFinished
	}
	task.Status = DiagnosticTaskStatusRunning
	task.CurrentStep = step.Code
	task.UpdatedAt = now
	s.publishDiagnosticTaskEvent(newDiagnosticTaskUpdatedEvent(task))
	step.Status = DiagnosticTaskStatusRunning
	step.StartedAt = &now
	step.CompletedAt = nil
	step.Message = step.Description
	for i := range task.Steps {
		if task.Steps[i].Code == step.Code {
			task.Steps[i] = *step
			break
		}
	}
	return s.UpdateDiagnosticTaskStep(ctx, step)
}

func (s *Service) finishDiagnosticTaskStep(ctx context.Context, task *DiagnosticTask, step *DiagnosticTaskStep, message string) error {
	now := time.Now().UTC()
	step.Status = DiagnosticTaskStatusSucceeded
	step.Message = message
	step.Error = ""
	step.CompletedAt = &now
	for i := range task.Steps {
		if task.Steps[i].Code == step.Code {
			task.Steps[i] = *step
			break
		}
	}
	return s.UpdateDiagnosticTaskStep(ctx, step)
}

func (s *Service) failDiagnosticTaskStep(ctx context.Context, task *DiagnosticTask, step *DiagnosticTaskStep, stepErr error) error {
	now := time.Now().UTC()
	step.Status = DiagnosticTaskStatusFailed
	step.Error = stepErr.Error()
	step.Message = stepErr.Error()
	step.CompletedAt = &now
	for i := range task.Steps {
		if task.Steps[i].Code == step.Code {
			task.Steps[i] = *step
			break
		}
	}
	return s.UpdateDiagnosticTaskStep(ctx, step)
}

func (s *Service) failDiagnosticTask(ctx context.Context, task *DiagnosticTask, failureStep DiagnosticStepCode, taskErr error) error {
	now := time.Now().UTC()
	updated, err := s.repo.UpdateDiagnosticTaskFields(ctx, task.ID, []DiagnosticTaskStatus{DiagnosticTaskStatusRunning}, map[string]any{
		"status":         DiagnosticTaskStatusFailed,
		"failure_step":   failureStep,
		"failure_reason": taskErr.Error(),
		"completed_at":   now,
		"updated_at":     now,
	})
	if err != nil {
		return err
	}
	if !updated {
		if cancelled, cancelErr := s.cancelDiagnosticTaskAtBoundary(ctx, task.ID); cancelErr != nil || cancelled {
			return cancelErr
		}
		return ErrDiagnosticTaskAlreadyFinished
	}
	task.Status = DiagnosticTaskStatusFailed
	task.FailureStep = failureStep
	task.FailureReason = taskErr.Error()
	task.CompletedAt = &now
	task.UpdatedAt = now
	s.publishDiagnosticTaskEvent(newDiagnosticTaskUpdatedEvent(task))
	return s.syncExecutionFromDiagnosticTask(ctx, task)
}

func (s *Service) beginDiagnosticNodeStep(ctx context.Context, step *DiagnosticTaskStep, node *DiagnosticNodeExecution, message string) error {
	now := time.Now().UTC()
	node.TaskStepID = uintPtr(step.ID)
	node.CurrentStep = step.Code
	node.Status = DiagnosticTaskStatusRunning
	node.Message = message
	node.Error = ""
	node.StartedAt = &now
	return s.UpdateDiagnosticNodeExecution(ctx, node)
}

func (s *Service) finishDiagnosticNodeStep(ctx context.Context, step *DiagnosticTaskStep, node *DiagnosticNodeExecution, status DiagnosticTaskStatus, message string) error {
	now := time.Now().UTC()
	node.TaskStepID = uintPtr(step.ID)
	node.CurrentStep = step.Code
	node.Status = status
	node.Message = message
	if status != DiagnosticTaskStatusFailed {
		node.Error = ""
	}
	node.CompletedAt = &now
	return s.UpdateDiagnosticNodeExecution(ctx, node)
}

func (s *Service) writeDiagnosticJSONArtifact(ctx context.Context, task *DiagnosticTask, step *DiagnosticTaskStep, state *diagnosticBundleExecutionState, bundleDir, fileName string, payload interface{}, artifact *diagnosticBundleArtifact) error {
	bytes, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	bytes = []byte(audit.RedactText(string(bytes)))
	path := filepath.Join(bundleDir, fileName)
	if err := os.WriteFile(path, bytes, 0o644); err != nil {
		return err
	}
	artifact.Path = path
	artifact.SizeBytes = int64(len(bytes))
	state.Artifacts = append(state.Artifacts, artifact)
	return s.AppendDiagnosticStepLog(ctx, &DiagnosticStepLog{
		TaskID:         task.ID,
		TaskStepID:     uintPtr(step.ID),
		StepCode:       step.Code,
		Level:          DiagnosticLogLevelInfo,
		EventType:      DiagnosticLogEventTypeSuccess,
		Message:        bilingualText(fmt.Sprintf("已生成 %s", fileName), fmt.Sprintf("Created %s", fileName)),
		CommandSummary: path,
		CreatedAt:      time.Now().UTC(),
	})
}

func writeDiagnosticBundleManifestFile(path string, task *DiagnosticTask, artifacts []*diagnosticBundleArtifact, state *diagnosticBundleExecutionState) error {
	manifest := buildDiagnosticBundleManifest(task, artifacts, state)
	payload, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, payload, 0o644)
}

func buildDiagnosticBundleManifest(task *DiagnosticTask, artifacts []*diagnosticBundleArtifact, state *diagnosticBundleExecutionState) *diagnosticBundleManifest {
	if task == nil {
		return nil
	}
	window := resolveDiagnosticCollectionWindow(task, nil)
	if state != nil {
		if state.WindowStart != nil && !state.WindowStart.IsZero() {
			window.Start = state.WindowStart.UTC()
		}
		if state.WindowEnd != nil && !state.WindowEnd.IsZero() {
			window.End = state.WindowEnd.UTC()
		}
		if state.LookbackMinutes > 0 {
			window.LookbackMinutes = state.LookbackMinutes
		}
		if state.InspectionDetail != nil {
			window = resolveDiagnosticCollectionWindow(task, state.InspectionDetail)
		}
	}
	return &diagnosticBundleManifest{
		Version:         "v2",
		TaskID:          task.ID,
		ClusterID:       task.ClusterID,
		TriggerSource:   task.TriggerSource,
		SourceRef:       task.SourceRef,
		Options:         task.Options.Normalize(),
		Status:          task.Status,
		Summary:         task.Summary,
		LookbackMinutes: window.LookbackMinutes,
		WindowStart:     timePtr(window.Start),
		WindowEnd:       timePtr(window.End),
		GeneratedAt:     time.Now().UTC(),
		Artifacts:       cloneDiagnosticArtifacts(artifacts),
	}
}

func cloneDiagnosticArtifacts(src []*diagnosticBundleArtifact) []*diagnosticBundleArtifact {
	if len(src) == 0 {
		return []*diagnosticBundleArtifact{}
	}
	result := make([]*diagnosticBundleArtifact, 0, len(src))
	for _, item := range src {
		if item == nil {
			continue
		}
		copyItem := *item
		result = append(result, &copyItem)
	}
	return result
}

func buildDiagnosticBundleHTMLPayload(task *DiagnosticTask, state *diagnosticBundleExecutionState, bundleDir string, artifacts []*diagnosticBundleArtifact) *diagnosticBundleHTMLPayload {
	payload := &diagnosticBundleHTMLPayload{
		GeneratedAt:        time.Now().UTC(),
		Task:               buildDiagnosticBundleHTMLTaskSummary(task),
		SourceTraceability: buildDiagnosticBundleHTMLTraceItems(task),
		TaskExecution:      buildDiagnosticBundleHTMLExecutionPanel(task),
		ArtifactGroups:     buildDiagnosticBundleHTMLArtifactGroups(bundleDir, artifacts),
	}
	payload.Health = buildDiagnosticBundleHTMLHealthSummary(task, state, artifacts)
	payload.Findings = buildDiagnosticBundleHTMLFindingCards(task, state)
	payload.Categories = buildDiagnosticBundleHTMLCategoryCards(task, state)
	payload.Timeline = buildDiagnosticBundleHTMLTimeline(task, state)
	payload.KeySignals = buildDiagnosticBundleHTMLSignalCards(state)
	payload.ThreadDumps = buildDiagnosticBundleHTMLThreadDumps(bundleDir, artifacts, task)
	if state == nil {
		payload.Recommendations = buildDiagnosticBundleHTMLRecommendations(task, state)
		payload.PassedChecks = buildDiagnosticBundleHTMLPassedChecks(task, state, artifacts)
		return payload
	}
	// Evidence & 附录
	payload.Cluster = buildDiagnosticBundleHTMLClusterSummary(state.ClusterSnapshot)
	payload.Inspection = buildDiagnosticBundleHTMLInspectionPanel(state.InspectionDetail)
	payload.ErrorContext = buildDiagnosticBundleHTMLErrorPanel(bundleDir, task.ID, state.ErrorGroup, state.ErrorEvents, state.LogSamples)
	payload.AlertSnapshot = buildDiagnosticBundleHTMLAlertPanel(state.AlertSnapshot)
	payload.ProcessEvents = buildDiagnosticBundleHTMLProcessPanel(state.ProcessEvents)
	payload.ConfigSnapshot = buildDiagnosticBundleHTMLConfigPanel(state.ConfigSnapshot)
	payload.MetricsSnapshot = buildDiagnosticBundleHTMLMetricsPanel(state.MetricsSnapshot)
	payload.Recommendations = buildDiagnosticBundleHTMLRecommendations(task, state)
	payload.PassedChecks = buildDiagnosticBundleHTMLPassedChecks(task, state, artifacts)
	return payload
}

func buildDiagnosticBundleHTMLTaskSummary(task *DiagnosticTask) diagnosticBundleHTMLTaskSummary {
	summary := diagnosticBundleHTMLTaskSummary{}
	if task == nil {
		return summary
	}
	selectedNodes := make([]diagnosticBundleHTMLNodeTarget, 0, len(task.SelectedNodes))
	for _, node := range sortedDiagnosticTaskTargets(task.SelectedNodes) {
		selectedNodes = append(selectedNodes, diagnosticBundleHTMLNodeTarget{
			HostLabel:   resolveDiagnosticHostLabel(node.HostName, node.HostID, node.HostIP),
			Role:        normalizeDiagnosticDisplayText(node.Role),
			InstallDir:  normalizeDiagnosticDisplayText(node.InstallDir),
			ClusterNode: fmt.Sprintf("#%d", node.ClusterNodeID),
		})
	}
	return diagnosticBundleHTMLTaskSummary{
		ID:            task.ID,
		Status:        task.Status,
		TriggerSource: task.TriggerSource,
		Summary:       normalizeDiagnosticDisplayText(task.Summary),
		CreatedBy:     firstNonEmptyString(strings.TrimSpace(task.CreatedByName), fmt.Sprintf("%d", task.CreatedBy)),
		StartedAt:     task.StartedAt,
		CompletedAt:   task.CompletedAt,
		BundleDir:     normalizeDiagnosticDisplayText(task.BundleDir),
		ManifestPath:  normalizeDiagnosticDisplayText(task.ManifestPath),
		IndexPath:     normalizeDiagnosticDisplayText(task.IndexPath),
		Options:       task.Options.Normalize(),
		SelectedNodes: selectedNodes,
	}
}

func buildDiagnosticBundleHTMLTraceItems(task *DiagnosticTask) []diagnosticBundleHTMLTraceItem {
	if task == nil {
		return nil
	}
	items := []diagnosticBundleHTMLTraceItem{
		{Label: bilingualText("触发方式", "How it was triggered"), Value: resolveDiagnosticTriggerSourceLabel(task.TriggerSource)},
	}
	if task.SourceRef.ErrorGroupID > 0 {
		items = append(items, diagnosticBundleHTMLTraceItem{
			Label: bilingualText("错误组", "Error Group"),
			Value: fmt.Sprintf("#%d", task.SourceRef.ErrorGroupID),
		})
	}
	if task.SourceRef.InspectionReportID > 0 {
		items = append(items, diagnosticBundleHTMLTraceItem{
			Label: bilingualText("巡检报告", "Inspection Report"),
			Value: fmt.Sprintf("#%d", task.SourceRef.InspectionReportID),
		})
	}
	if task.SourceRef.InspectionFindingID > 0 {
		items = append(items, diagnosticBundleHTMLTraceItem{
			Label: bilingualText("巡检发现", "Inspection Finding"),
			Value: fmt.Sprintf("#%d", task.SourceRef.InspectionFindingID),
		})
	}
	if alertID := strings.TrimSpace(task.SourceRef.AlertID); alertID != "" {
		items = append(items, diagnosticBundleHTMLTraceItem{
			Label: bilingualText("告警 ID", "Alert ID"),
			Value: alertID,
		})
	}
	return items
}

func buildDiagnosticBundleHTMLClusterSummary(snapshot *cluster.Cluster) *diagnosticBundleHTMLClusterSummary {
	if snapshot == nil {
		return nil
	}
	nodes := make([]diagnosticBundleHTMLClusterNode, 0, len(snapshot.Nodes))
	for _, node := range snapshot.Nodes {
		nodes = append(nodes, diagnosticBundleHTMLClusterNode{
			ClusterNodeID: node.ID,
			HostLabel:     fmt.Sprintf("Host #%d", node.HostID),
			Role:          normalizeDiagnosticDisplayText(string(node.Role)),
			HostID:        node.HostID,
			InstallDir:    normalizeDiagnosticDisplayText(node.InstallDir),
			Status:        normalizeDiagnosticDisplayText(string(node.Status)),
			ProcessPID:    node.ProcessPID,
		})
	}
	sort.SliceStable(nodes, func(i, j int) bool {
		if nodes[i].HostID != nodes[j].HostID {
			return nodes[i].HostID < nodes[j].HostID
		}
		return nodes[i].Role < nodes[j].Role
	})
	return &diagnosticBundleHTMLClusterSummary{
		ID:             snapshot.ID,
		Name:           normalizeDiagnosticDisplayText(snapshot.Name),
		Version:        normalizeDiagnosticDisplayText(snapshot.Version),
		Status:         normalizeDiagnosticDisplayText(string(snapshot.Status)),
		DeploymentMode: normalizeDiagnosticDisplayText(string(snapshot.DeploymentMode)),
		InstallDir:     normalizeDiagnosticDisplayText(snapshot.InstallDir),
		NodeCount:      len(snapshot.Nodes),
		Nodes:          nodes,
	}
}

func buildDiagnosticBundleHTMLInspectionPanel(detail *ClusterInspectionReportDetailData) *diagnosticBundleHTMLInspectionPanel {
	if detail == nil || detail.Report == nil {
		return nil
	}
	findings := make([]diagnosticBundleHTMLFinding, 0, len(detail.Findings))
	for _, finding := range detail.Findings {
		if finding == nil {
			continue
		}
		findings = append(findings, diagnosticBundleHTMLFinding{
			Severity:       normalizeDiagnosticDisplayText(string(finding.Severity)),
			CheckName:      normalizeDiagnosticDisplayText(firstNonEmptyString(finding.CheckName, finding.CheckCode)),
			CheckCode:      normalizeDiagnosticDisplayText(finding.CheckCode),
			Summary:        normalizeDiagnosticDisplayText(finding.Summary),
			Recommendation: normalizeDiagnosticDisplayText(finding.Recommendation),
			Evidence:       normalizeDiagnosticDisplayText(finding.EvidenceSummary),
		})
	}
	return &diagnosticBundleHTMLInspectionPanel{
		Summary:         normalizeDiagnosticDisplayText(detail.Report.Summary),
		Status:          detail.Report.Status,
		RequestedBy:     normalizeDiagnosticDisplayText(detail.Report.RequestedBy),
		LookbackMinutes: firstNonZeroInt(detail.Report.LookbackMinutes, defaultInspectionLookbackMinutes),
		CriticalCount:   detail.Report.CriticalCount,
		WarningCount:    detail.Report.WarningCount,
		InfoCount:       detail.Report.InfoCount,
		StartedAt:       detail.Report.StartedAt,
		FinishedAt:      detail.Report.FinishedAt,
		Findings:        findings,
	}
}

func buildDiagnosticBundleHTMLErrorPanel(bundleDir string, taskID uint, group *SeatunnelErrorGroup, events []*SeatunnelErrorEvent, logSamples []diagnosticCollectedLogSample) *diagnosticBundleHTMLErrorPanel {
	if group == nil && len(events) == 0 && len(logSamples) == 0 {
		return nil
	}
	panel := &diagnosticBundleHTMLErrorPanel{
		Events:     make([]diagnosticBundleHTMLErrorEvent, 0, len(events)),
		LogSamples: make([]diagnosticBundleHTMLLogSample, 0, len(logSamples)),
	}
	if group != nil {
		panel.GroupTitle = normalizeDiagnosticDisplayText(group.Title)
		panel.ExceptionClass = normalizeDiagnosticDisplayText(group.ExceptionClass)
		panel.OccurrenceCount = group.OccurrenceCount
		panel.FirstSeenAt = &group.FirstSeenAt
		panel.LastSeenAt = &group.LastSeenAt
		panel.SampleMessage = normalizeDiagnosticDisplayText(group.SampleMessage)
	}
	sortedEvents := make([]*SeatunnelErrorEvent, 0, len(events))
	sortedEvents = append(sortedEvents, events...)
	sort.SliceStable(sortedEvents, func(i, j int) bool {
		left := sortedEvents[i]
		right := sortedEvents[j]
		if left == nil || right == nil {
			return left != nil
		}
		if !left.OccurredAt.Equal(right.OccurredAt) {
			return left.OccurredAt.Before(right.OccurredAt)
		}
		return left.ID < right.ID
	})
	for _, event := range sortedEvents {
		if event == nil {
			continue
		}
		panel.Events = append(panel.Events, diagnosticBundleHTMLErrorEvent{
			OccurredAt: formatDiagnosticBundleTimeValue(event.OccurredAt),
			Role:       normalizeDiagnosticDisplayText(event.Role),
			HostLabel:  resolveDiagnosticHostLabel("", event.HostID, ""),
			SourceFile: normalizeDiagnosticDisplayText(event.SourceFile),
			JobID:      normalizeDiagnosticDisplayText(event.JobID),
			Message:    normalizeDiagnosticDisplayText(event.Message),
			Evidence:   normalizeDiagnosticDisplayText(event.Evidence),
		})
	}
	panel.RecentEventCount = len(panel.Events)
	for _, item := range logSamples {
		previewContent, previewTruncated, previewLineCount := buildDiagnosticLogPreview(item.Content, diagnosticHTMLLogPreviewLineLimit)
		fullLogRelativePath := resolveDiagnosticBundleRelativePath(bundleDir, item.LocalPath)
		panel.LogSamples = append(panel.LogSamples, diagnosticBundleHTMLLogSample{
			HostLabel:           resolveDiagnosticHostLabel(item.HostName, item.HostID, item.HostIP),
			SourceFile:          normalizeDiagnosticDisplayText(item.SourceFile),
			WindowLabel:         fmt.Sprintf("%s ~ %s", formatDiagnosticBundleTimeValue(item.WindowStart), formatDiagnosticBundleTimeValue(item.WindowEnd)),
			PreviewContent:      previewContent,
			PreviewLineCount:    previewLineCount,
			PreviewTruncated:    previewTruncated,
			FullLogRelativePath: fullLogRelativePath,
			FullLogPreviewURL:   buildDiagnosticTaskPreviewFileURL(taskID, fullLogRelativePath),
		})
	}
	return panel
}

func buildDiagnosticBundleHTMLAlertPanel(alerts []*monitoringapp.AlertInstance) *diagnosticBundleHTMLAlertPanel {
	if len(alerts) == 0 {
		return nil
	}
	panel := &diagnosticBundleHTMLAlertPanel{
		Alerts: make([]diagnosticBundleHTMLAlertItem, 0, len(alerts)),
	}
	for _, alert := range alerts {
		if alert == nil {
			continue
		}
		item := diagnosticBundleHTMLAlertItem{
			Name:        normalizeDiagnosticDisplayText(alert.AlertName),
			Severity:    normalizeDiagnosticDisplayText(string(alert.Severity)),
			Status:      normalizeDiagnosticDisplayText(string(alert.Status)),
			CreatedAt:   formatDiagnosticBundleTimeValue(alert.CreatedAt),
			FiringAt:    formatDiagnosticBundleTimeValue(alert.FiringAt),
			LastSeenAt:  formatDiagnosticBundleTimeValue(alert.LastSeenAt),
			ResolvedAt:  formatDiagnosticBundleTime(alert.ResolvedAt),
			Summary:     normalizeDiagnosticDisplayText(alert.Summary),
			Description: normalizeDiagnosticDisplayText(alert.Description),
		}
		switch alert.Severity {
		case monitoringapp.AlertSeverityCritical:
			panel.Critical++
		default:
			panel.Warning++
		}
		if alert.Status == monitoringapp.AlertDisplayStatusFiring {
			panel.Firing++
		}
		if panel.FirstSeenAt == "" || (!alert.CreatedAt.IsZero() && formatDiagnosticBundleTimeValue(alert.CreatedAt) < panel.FirstSeenAt) {
			panel.FirstSeenAt = formatDiagnosticBundleTimeValue(alert.CreatedAt)
		}
		lastSeen := formatDiagnosticBundleTimeValue(alert.LastSeenAt)
		if panel.LastSeenAt == "" || lastSeen > panel.LastSeenAt {
			panel.LastSeenAt = lastSeen
		}
		panel.Alerts = append(panel.Alerts, item)
	}
	panel.Total = len(panel.Alerts)
	return panel
}

func buildDiagnosticBundleHTMLProcessPanel(events []*monitor.ProcessEvent) *diagnosticBundleHTMLProcessPanel {
	if len(events) == 0 {
		return nil
	}
	panel := &diagnosticBundleHTMLProcessPanel{
		Total:  len(events),
		Events: make([]diagnosticBundleHTMLProcessEvent, 0, len(events)),
	}
	typeCounter := make(map[string]int)
	for _, event := range events {
		if event == nil {
			continue
		}
		eventType := normalizeDiagnosticDisplayText(string(event.EventType))
		typeCounter[eventType]++
		panel.Events = append(panel.Events, diagnosticBundleHTMLProcessEvent{
			CreatedAt:   formatDiagnosticBundleTimeValue(event.CreatedAt),
			EventType:   eventType,
			ProcessName: normalizeDiagnosticDisplayText(event.ProcessName),
			NodeLabel:   resolveDiagnosticHostLabel("", event.HostID, fmt.Sprintf("node-%d", event.NodeID)),
			Details:     normalizeDiagnosticDisplayText(event.Details),
		})
	}
	typeKeys := make([]string, 0, len(typeCounter))
	for key := range typeCounter {
		typeKeys = append(typeKeys, key)
	}
	sort.Strings(typeKeys)
	panel.ByType = make([]diagnosticBundleHTMLMetricCard, 0, len(typeKeys))
	for _, key := range typeKeys {
		panel.ByType = append(panel.ByType, diagnosticBundleHTMLMetricCard{
			Label: key,
			Value: fmt.Sprintf("%d", typeCounter[key]),
		})
	}
	return panel
}

// buildDiagnosticBundleHTMLThreadDumps 收集线程栈快照文件并提取预览文本与元数据。
// buildDiagnosticBundleHTMLThreadDumps collects thread dump snapshot files and extracts preview text and metadata.
func buildDiagnosticBundleHTMLThreadDumps(bundleDir string, artifacts []*diagnosticBundleArtifact, task *DiagnosticTask) []diagnosticBundleHTMLThreadDumpItem {
	var dumpArtifacts []*diagnosticBundleArtifact
	for _, art := range artifacts {
		if art != nil && art.Category == "thread_dump" {
			dumpArtifacts = append(dumpArtifacts, art)
		}
	}

	// 兜底扫描磁盘 thread-dumps 目录（用于离线重算或测试重建）
	// Fallback to scanning disk thread-dumps directory (for offline rebuild or test fixtures)
	dumpDir := filepath.Join(bundleDir, "thread-dumps")
	if len(dumpArtifacts) == 0 && strings.TrimSpace(bundleDir) != "" {
		entries, err := os.ReadDir(dumpDir)
		if err == nil {
			for _, entry := range entries {
				if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".txt") {
					continue
				}
				info, err := entry.Info()
				if err != nil {
					continue
				}
				dumpArtifacts = append(dumpArtifacts, &diagnosticBundleArtifact{
					Category:  "thread_dump",
					Format:    "txt",
					Status:    "succeeded",
					Path:      filepath.Join(dumpDir, entry.Name()),
					SizeBytes: info.Size(),
				})
			}
		}
	}

	if len(dumpArtifacts) == 0 {
		return nil
	}

	// 建立选定节点查找索引
	// Build lookup index for selected task nodes
	targetByHostID := make(map[uint]DiagnosticTaskNodeTarget)
	if task != nil {
		for _, node := range task.SelectedNodes {
			targetByHostID[node.HostID] = node
		}
	}

	items := make([]diagnosticBundleHTMLThreadDumpItem, 0, len(dumpArtifacts))
	for _, art := range dumpArtifacts {
		filePath := art.Path
		if !filepath.IsAbs(filePath) && bundleDir != "" {
			filePath = filepath.Join(bundleDir, filePath)
		}
		contentBytes, err := os.ReadFile(filePath)
		if err != nil {
			relPath := filepath.Join(dumpDir, filepath.Base(art.Path))
			contentBytes, err = os.ReadFile(relPath)
			if err != nil {
				continue
			}
			filePath = relPath
		}

		fullContent := string(contentBytes)
		lines := strings.Split(strings.ReplaceAll(fullContent, "\r\n", "\n"), "\n")
		totalLines := len(lines)
		previewLines := lines
		if len(previewLines) > 300 {
			previewLines = previewLines[:300]
		}
		preview := strings.Join(previewLines, "\n")
		if totalLines > 300 {
			preview += fmt.Sprintf("\n... (%d lines truncated, view full thread dump)", totalLines-300)
		}

		hostID := art.HostID
		hostName := art.HostName
		hostIP := ""
		role := ""
		nodeID := art.NodeID

		// 从文件名推断 host 和 role（例如 thread-dump-host-10-hybrid.txt）
		// Infer host and role from filename (e.g. thread-dump-host-10-hybrid.txt)
		baseName := filepath.Base(filePath)
		if hostID == 0 {
			var parsedHost uint
			var parsedRole string
			if n, _ := fmt.Sscanf(baseName, "thread-dump-host-%d-%s", &parsedHost, &parsedRole); n >= 1 {
				hostID = parsedHost
				role = strings.TrimSuffix(parsedRole, ".txt")
			}
		}

		if tgt, ok := targetByHostID[hostID]; ok {
			if hostName == "" {
				hostName = tgt.HostName
			}
			if hostIP == "" {
				hostIP = tgt.HostIP
			}
			if role == "" {
				role = tgt.Role
			}
			if nodeID == 0 {
				nodeID = tgt.NodeID
			}
		}

		hostLabel := resolveDiagnosticHostLabel(hostName, hostID, hostIP)
		sizeBytes := art.SizeBytes
		if sizeBytes <= 0 {
			sizeBytes = int64(len(contentBytes))
		}

		relPath := filepath.Join("thread-dumps", baseName)
		var previewURL string
		if task != nil && task.ID > 0 {
			previewURL = fmt.Sprintf("/api/v1/diagnostics/tasks/%d/files/%s", task.ID, filepath.ToSlash(relPath))
		}

		toolName := art.Message
		if toolName == "" {
			toolName = "jcmd Thread.print"
		}

		items = append(items, diagnosticBundleHTMLThreadDumpItem{
			HostID:       hostID,
			HostName:     hostName,
			HostIP:       hostIP,
			HostLabel:    hostLabel,
			NodeID:       nodeID,
			Role:         role,
			Tool:         toolName,
			RemotePath:   art.RemotePath,
			LocalPath:    filePath,
			RelativePath: filepath.ToSlash(relPath),
			PreviewURL:   previewURL,
			SizeBytes:    sizeBytes,
			SizeLabel:    formatDiagnosticBytes(sizeBytes),
			Preview:      preview,
			TotalLines:   totalLines,
		})
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].HostID != items[j].HostID {
			return items[i].HostID < items[j].HostID
		}
		return items[i].Role < items[j].Role
	})
	return items
}

func buildDiagnosticBundleHTMLConfigPanel(summary *diagnosticConfigSnapshotSummary) *diagnosticBundleHTMLConfigPanel {
	if summary == nil {
		return nil
	}
	recentChanges := append([]diagnosticConfigChangeRecord(nil), summary.ConfigChanges...)
	remainingChanges := []diagnosticConfigChangeRecord{}
	if len(recentChanges) > 8 {
		remainingChanges = append(remainingChanges, recentChanges[8:]...)
		recentChanges = recentChanges[:8]
	}

	// 聚合每个配置文件对应的关键摘要与文本预览
	// Aggregate key highlights and raw content preview per configuration file
	fileEntries := make([]diagnosticBundleHTMLConfigFileEntry, 0, len(summary.Files)+len(summary.FilePreviews))
	previewMap := make(map[string]string, len(summary.FilePreviews))
	for _, p := range summary.FilePreviews {
		key := fmt.Sprintf("%d:%s:%s", p.HostID, p.ConfigType, p.RemotePath)
		// 历史快照也在渲染前再次遮盖，避免旧数据直达报告。 / Redact legacy previews again before rendering to avoid exposing old snapshots.
		previewMap[key] = audit.RedactText(p.Preview)
	}
	itemsMap := make(map[string][]diagnosticConfigKeyValue, len(summary.KeyHighlights))
	for _, h := range summary.KeyHighlights {
		key := fmt.Sprintf("%d:%s:%s", h.HostID, h.ConfigType, h.RemotePath)
		items := append([]diagnosticConfigKeyValue(nil), h.Items...)
		for i := range items {
			items[i].Value = audit.RedactText(items[i].Value)
		}
		itemsMap[key] = items
	}

	seenKeys := make(map[string]bool)
	// 优先根据 Files 列表按序构建
	// Build by Files list in priority order
	for _, file := range summary.Files {
		key := fmt.Sprintf("%d:%s:%s", file.HostID, file.ConfigType, file.RemotePath)
		seenKeys[key] = true
		fileEntries = append(fileEntries, diagnosticBundleHTMLConfigFileEntry{
			HostID:     file.HostID,
			HostName:   file.HostName,
			HostIP:     file.HostIP,
			NodeID:     file.NodeID,
			Role:       file.Role,
			ConfigType: file.ConfigType,
			RemotePath: file.RemotePath,
			Items:      itemsMap[key],
			Preview:    previewMap[key],
		})
	}

	// 兜底处理仅存在于 KeyHighlights 或 FilePreviews 中的配置（例如单测或历史数据）
	// Fallback for configs only existing in KeyHighlights or FilePreviews (e.g. unit tests or legacy data)
	for _, h := range summary.KeyHighlights {
		key := fmt.Sprintf("%d:%s:%s", h.HostID, h.ConfigType, h.RemotePath)
		if seenKeys[key] {
			continue
		}
		seenKeys[key] = true
		fileEntries = append(fileEntries, diagnosticBundleHTMLConfigFileEntry{
			HostID:     h.HostID,
			HostName:   h.HostName,
			HostIP:     h.HostIP,
			NodeID:     h.NodeID,
			Role:       h.Role,
			ConfigType: h.ConfigType,
			RemotePath: h.RemotePath,
			Items:      itemsMap[key],
			Preview:    previewMap[key],
		})
	}
	for _, p := range summary.FilePreviews {
		key := fmt.Sprintf("%d:%s:%s", p.HostID, p.ConfigType, p.RemotePath)
		if seenKeys[key] {
			continue
		}
		seenKeys[key] = true
		fileEntries = append(fileEntries, diagnosticBundleHTMLConfigFileEntry{
			HostID:     p.HostID,
			HostName:   p.HostName,
			HostIP:     p.HostIP,
			NodeID:     p.NodeID,
			Role:       p.Role,
			ConfigType: p.ConfigType,
			RemotePath: p.RemotePath,
			Items:      itemsMap[key],
			Preview:    previewMap[key],
		})
	}

	// 历史摘要和预览也要走同一套遮盖，避免兼容字段再次带出明文。
	// Apply the same redaction to legacy summary fields to prevent plaintext from resurfacing.
	safeHighlights := append([]diagnosticConfigKeyHighlight(nil), summary.KeyHighlights...)
	for i := range safeHighlights {
		key := fmt.Sprintf("%d:%s:%s", safeHighlights[i].HostID, safeHighlights[i].ConfigType, safeHighlights[i].RemotePath)
		safeHighlights[i].Items = itemsMap[key]
	}
	safePreviews := append([]diagnosticConfigFilePreview(nil), summary.FilePreviews...)
	for i := range safePreviews {
		key := fmt.Sprintf("%d:%s:%s", safePreviews[i].HostID, safePreviews[i].ConfigType, safePreviews[i].RemotePath)
		safePreviews[i].Preview = previewMap[key]
	}

	return &diagnosticBundleHTMLConfigPanel{
		FileCount:          len(summary.Files),
		KeyHighlightCount:  len(summary.KeyHighlights),
		DirectoryCount:     len(summary.DirectoryManifests),
		ChangedConfigCount: len(summary.ConfigChanges),
		ConfigFileEntries:  fileEntries,
		KeyHighlights:      safeHighlights,
		FilePreviews:       safePreviews,
		RecentChanges:      recentChanges,
		RemainingChanges:   remainingChanges,
		Files:              summary.Files,
		DirectoryManifests: summary.DirectoryManifests,
		ConfigChanges:      summary.ConfigChanges,
		CollectionNotes:    summary.CollectionNotes,
	}
}

func buildDiagnosticBundleHTMLMetricsPanel(snapshot *diagnosticPrometheusSnapshot) *diagnosticBundleHTMLMetricsPanel {
	if snapshot == nil || (len(snapshot.Signals) == 0 && len(snapshot.CollectionNotes) == 0) {
		return nil
	}
	highlighted := make([]diagnosticPrometheusSignal, 0, len(snapshot.Signals))
	additional := make([]diagnosticPrometheusSignal, 0, len(snapshot.Signals))
	anomalyCount := 0
	for _, signal := range snapshot.Signals {
		if strings.EqualFold(strings.TrimSpace(signal.Status), "healthy") || strings.TrimSpace(signal.Status) == "" {
			additional = append(additional, signal)
			continue
		}
		anomalyCount++
		highlighted = append(highlighted, signal)
	}
	if len(highlighted) == 0 {
		highlighted = append(highlighted, snapshot.Signals...)
		if len(highlighted) > 3 {
			additional = append([]diagnosticPrometheusSignal(nil), highlighted[3:]...)
			highlighted = append([]diagnosticPrometheusSignal(nil), highlighted[:3]...)
		} else {
			additional = []diagnosticPrometheusSignal{}
		}
	}
	return &diagnosticBundleHTMLMetricsPanel{
		SignalCount:        len(snapshot.Signals),
		AnomalyCount:       anomalyCount,
		HighlightedSignals: highlighted,
		AdditionalSignals:  additional,
		CollectionNotes:    append([]string(nil), snapshot.CollectionNotes...),
	}
}

func buildDiagnosticBundleHTMLExecutionPanel(task *DiagnosticTask) diagnosticBundleHTMLExecutionPanel {
	panel := diagnosticBundleHTMLExecutionPanel{
		Steps: []diagnosticBundleHTMLExecutionStep{},
		Nodes: []diagnosticBundleHTMLExecutionNode{},
	}
	if task == nil {
		return panel
	}
	steps := make([]DiagnosticTaskStep, 0, len(task.Steps))
	steps = append(steps, task.Steps...)
	sort.SliceStable(steps, func(i, j int) bool {
		if steps[i].Sequence != steps[j].Sequence {
			return steps[i].Sequence < steps[j].Sequence
		}
		return steps[i].ID < steps[j].ID
	})
	for _, step := range steps {
		panel.Steps = append(panel.Steps, diagnosticBundleHTMLExecutionStep{
			Sequence:    step.Sequence,
			Code:        string(step.Code),
			Title:       normalizeDiagnosticDisplayText(step.Title),
			Status:      normalizeDiagnosticDisplayText(string(step.Status)),
			Message:     normalizeDiagnosticDisplayText(step.Message),
			Error:       normalizeDiagnosticDisplayText(step.Error),
			StartedAt:   formatDiagnosticBundleTime(step.StartedAt),
			CompletedAt: formatDiagnosticBundleTime(step.CompletedAt),
		})
	}
	nodes := make([]DiagnosticNodeExecution, 0, len(task.NodeExecutions))
	nodes = append(nodes, task.NodeExecutions...)
	sort.SliceStable(nodes, func(i, j int) bool {
		if nodes[i].HostID != nodes[j].HostID {
			return nodes[i].HostID < nodes[j].HostID
		}
		return nodes[i].Role < nodes[j].Role
	})
	for _, node := range nodes {
		panel.Nodes = append(panel.Nodes, diagnosticBundleHTMLExecutionNode{
			HostLabel:   resolveDiagnosticHostLabel(node.HostName, node.HostID, node.HostIP),
			Role:        normalizeDiagnosticDisplayText(node.Role),
			Status:      normalizeDiagnosticDisplayText(string(node.Status)),
			CurrentStep: normalizeDiagnosticDisplayText(string(node.CurrentStep)),
			Message:     normalizeDiagnosticDisplayText(node.Message),
			Error:       normalizeDiagnosticDisplayText(node.Error),
			StartedAt:   formatDiagnosticBundleTime(node.StartedAt),
			CompletedAt: formatDiagnosticBundleTime(node.CompletedAt),
		})
	}
	return panel
}

func buildDiagnosticBundleHTMLHealthSummary(task *DiagnosticTask, state *diagnosticBundleExecutionState, artifacts []*diagnosticBundleArtifact) diagnosticBundleHTMLHealthSummary {
	summary := diagnosticBundleHTMLHealthSummary{
		Tone:          "neutral",
		Title:         "",
		Summary:       "",
		ClusterLabel:  resolveDiagnosticClusterLabel(task, state),
		WindowLabel:   resolveDiagnosticWindowLabel(task, state),
		ImpactSummary: bilingualText("当前影响范围正在根据错误、巡检、告警与进程证据综合判断。", "Impact is summarized from errors, inspections, alerts and process evidence."),
		PrimaryFocus:  bilingualText("继续查看关键发现与时间线。", "Continue with key findings and the timeline."),
		LastSignalAt:  resolveDiagnosticLastSignalAt(state),
		Metrics:       []diagnosticBundleHTMLMetricCard{},
	}
	riskTone := resolveDiagnosticRiskTone(task, state)
	summary.Tone = riskTone

	if state != nil && state.InspectionDetail != nil && state.InspectionDetail.Report != nil {
		report := state.InspectionDetail.Report
		// 按巡检结果给出一句话结论与语气
		switch {
		case report.Status == InspectionReportStatusFailed || report.CriticalCount > 0:
			summary.Title = bilingualText("集群存在严重风险", "Cluster requires immediate attention")
			summary.Summary = normalizeDiagnosticDisplayText(firstNonEmptyString(report.Summary, report.ErrorMessage))
		case report.WarningCount > 0:
			summary.Title = bilingualText("集群存在待排查问题", "Cluster has issues to investigate")
			summary.Summary = normalizeDiagnosticDisplayText(report.Summary)
		default:
			summary.Title = bilingualText("巡检未发现明显异常", "Inspection found no critical issue")
			summary.Summary = normalizeDiagnosticDisplayText(report.Summary)
		}
		summary.ImpactSummary = bilingualText(
			fmt.Sprintf("巡检窗口内共发现 %d 项异常（严重 %d / 告警 %d / 信息 %d）。", report.FindingTotal, report.CriticalCount, report.WarningCount, report.InfoCount),
			fmt.Sprintf("%d findings were generated in the inspection window (%d critical / %d warning / %d info).", report.FindingTotal, report.CriticalCount, report.WarningCount, report.InfoCount),
		)
		summary.PrimaryFocus = buildDiagnosticPrimaryFocus(state)
		// 仅保留与“时间范围 + 发现数”直接相关的少量指标
		windowLabel := fmt.Sprintf("%d min", firstNonZeroInt(report.LookbackMinutes, defaultInspectionLookbackMinutes))
		windowNote := ""
		if state.WindowStart != nil && state.WindowEnd != nil {
			windowNote = fmt.Sprintf("%s ~ %s", formatDiagnosticBundleTime(state.WindowStart), formatDiagnosticBundleTime(state.WindowEnd))
		}
		summary.Metrics = append(summary.Metrics,
			diagnosticBundleHTMLMetricCard{
				Label: bilingualText("时间范围", "Time Window"),
				Value: windowLabel,
				Note:  windowNote,
			},
			diagnosticBundleHTMLMetricCard{
				Label: bilingualText("发现统计", "Findings"),
				Value: fmt.Sprintf("%d", report.FindingTotal),
				Note: bilingualText(
					fmt.Sprintf("严重 %d / 告警 %d / 信息 %d", report.CriticalCount, report.WarningCount, report.InfoCount),
					fmt.Sprintf("critical %d / warning %d / info %d", report.CriticalCount, report.WarningCount, report.InfoCount),
				),
			},
		)
		if state.MetricsSnapshot != nil {
			anomalyCount := 0
			for _, signal := range state.MetricsSnapshot.Signals {
				if strings.TrimSpace(signal.Status) != "" && !strings.EqualFold(signal.Status, "healthy") {
					anomalyCount++
				}
			}
			summary.Metrics = append(summary.Metrics, diagnosticBundleHTMLMetricCard{
				Label: bilingualText("指标信号", "Metric Signals"),
				Value: fmt.Sprintf("%d", len(state.MetricsSnapshot.Signals)),
				Note: bilingualText(
					fmt.Sprintf("异常信号 %d", anomalyCount),
					fmt.Sprintf("%d anomalous signal(s)", anomalyCount),
				),
			})
		}
	} else {
		// 无巡检上下文时，仅给出非常简短的概览
		window := resolveDiagnosticCollectionWindow(task, nil)
		if state != nil {
			if state.WindowStart != nil && state.WindowEnd != nil {
				window.Start = state.WindowStart.UTC()
				window.End = state.WindowEnd.UTC()
			}
			if state.LookbackMinutes > 0 {
				window.LookbackMinutes = state.LookbackMinutes
			}
		}
		summary.Title, summary.Summary = resolveDiagnosticHealthCopy(riskTone, state)
		summary.PrimaryFocus = buildDiagnosticPrimaryFocus(state)
		summary.Metrics = append(summary.Metrics,
			diagnosticBundleHTMLMetricCard{
				Label: bilingualText("集群", "Cluster"),
				Value: summary.ClusterLabel,
				Note:  "",
			},
			diagnosticBundleHTMLMetricCard{
				Label: bilingualText("时间范围", "Time Window"),
				Value: fmt.Sprintf("%d min", window.LookbackMinutes),
				Note:  fmt.Sprintf("%s ~ %s", formatDiagnosticBundleTimeValue(window.Start), formatDiagnosticBundleTimeValue(window.End)),
			},
		)
		if state != nil && state.MetricsSnapshot != nil {
			anomalyCount := 0
			for _, signal := range state.MetricsSnapshot.Signals {
				if strings.TrimSpace(signal.Status) != "" && !strings.EqualFold(signal.Status, "healthy") {
					anomalyCount++
				}
			}
			summary.Metrics = append(summary.Metrics, diagnosticBundleHTMLMetricCard{
				Label: bilingualText("指标信号", "Metric Signals"),
				Value: fmt.Sprintf("%d", len(state.MetricsSnapshot.Signals)),
				Note: bilingualText(
					fmt.Sprintf("异常信号 %d", anomalyCount),
					fmt.Sprintf("%d anomalous signal(s)", anomalyCount),
				),
			})
		}
	}
	return summary
}

func buildDiagnosticBundleHTMLFindingCards(task *DiagnosticTask, state *diagnosticBundleExecutionState) []diagnosticBundleHTMLFindingCard {
	items := make([]diagnosticBundleHTMLFindingCard, 0, 5)
	appendItem := func(item diagnosticBundleHTMLFindingCard) {
		if strings.TrimSpace(item.Title) == "" {
			return
		}
		item.Severity = normalizeDiagnosticDisplayText(item.Severity)
		item.Category = normalizeDiagnosticDisplayText(item.Category)
		item.Title = normalizeDiagnosticDisplayText(item.Title)
		item.Summary = normalizeDiagnosticDisplayText(item.Summary)
		item.Impact = normalizeDiagnosticDisplayText(item.Impact)
		item.Action = normalizeDiagnosticDisplayText(item.Action)
		items = append(items, item)
	}

	if state != nil && state.ErrorGroup != nil {
		appendItem(diagnosticBundleHTMLFindingCard{
			Severity: resolveDiagnosticRiskTone(task, state),
			Category: resolveDiagnosticPrimaryCategory(state),
			Title:    firstNonEmptyString(state.ErrorGroup.Title, state.ErrorGroup.ExceptionClass, bilingualText("错误上下文", "Error Context")),
			Summary: bilingualText(
				fmt.Sprintf("诊断窗口内累计出现 %d 次，最近一次发生在 %s。", state.ErrorGroup.OccurrenceCount, formatDiagnosticBundleTimeValue(state.ErrorGroup.LastSeenAt)),
				fmt.Sprintf("%d occurrences were observed in this window, and the last one happened at %s.", state.ErrorGroup.OccurrenceCount, formatDiagnosticBundleTimeValue(state.ErrorGroup.LastSeenAt)),
			),
			Impact: buildDiagnosticFindingImpact(state),
			Action: buildDiagnosticPrimaryFocus(state),
		})
	}

	if state != nil && state.InspectionDetail != nil {
		for _, finding := range state.InspectionDetail.Findings {
			if finding == nil {
				continue
			}
			appendItem(diagnosticBundleHTMLFindingCard{
				Severity: strings.ToLower(strings.TrimSpace(string(finding.Severity))),
				Category: mapInspectionFindingToDiagnosticCategory(finding),
				Title:    firstNonEmptyString(finding.CheckName, finding.CheckCode),
				Summary:  firstNonEmptyString(finding.Summary, finding.EvidenceSummary),
				Impact: bilingualText(
					fmt.Sprintf("影响范围：%s", resolveDiagnosticInspectionImpact(state.InspectionDetail.Report)),
					fmt.Sprintf("Impact: %s", resolveDiagnosticInspectionImpact(state.InspectionDetail.Report)),
				),
				Action: firstNonEmptyString(finding.Recommendation, buildDiagnosticPrimaryFocus(state)),
			})
			if len(items) >= 3 {
				break
			}
		}
	}

	if len(items) < 3 {
		resourceSummary := buildDiagnosticResourceFinding(state)
		if resourceSummary.Title != "" {
			appendItem(resourceSummary)
		}
	}
	if len(items) > 4 {
		items = items[:4]
	}
	return items
}

func buildDiagnosticBundleHTMLCategoryCards(task *DiagnosticTask, state *diagnosticBundleExecutionState) []diagnosticBundleHTMLCategoryCard {
	counts := map[string]int{
		"dependency":    0,
		"configuration": 0,
		"resource":      0,
		"process":       0,
		"unknown":       0,
	}
	if state != nil {
		primary := resolveDiagnosticPrimaryCategory(state)
		if primary != "" {
			counts[primary]++
		}
		if state.InspectionDetail != nil {
			for _, finding := range state.InspectionDetail.Findings {
				if finding == nil {
					continue
				}
				counts[mapInspectionFindingToDiagnosticCategory(finding)]++
			}
		}
		if state.MetricsSnapshot != nil {
			for _, signal := range state.MetricsSnapshot.Signals {
				if strings.EqualFold(strings.TrimSpace(signal.Status), "healthy") || strings.TrimSpace(signal.Status) == "" {
					continue
				}
				if isDiagnosticResourceSignal(signal.Key) {
					counts["resource"]++
				}
			}
		}
		if len(state.ProcessEvents) > 0 {
			counts["process"]++
		}
	}
	cards := []diagnosticBundleHTMLCategoryCard{
		{Key: "dependency", Label: bilingualText("外部依赖", "Dependency"), Count: counts["dependency"], Note: bilingualText("连接超时、依赖不可达、DNS / 网络链路异常。", "Timeouts, dependency reachability and network resolution issues.")},
		{Key: "configuration", Label: bilingualText("运行配置", "Configuration"), Count: counts["configuration"], Note: bilingualText("catalog / connector / 运行配置偏差。", "Catalog, connector and runtime configuration drift.")},
		{Key: "resource", Label: bilingualText("资源信号", "Resource"), Count: counts["resource"], Note: bilingualText("CPU、Heap、Old Gen、GC、FD 等资源压力。", "CPU, Heap, Old Gen, GC, FD and related pressure.")},
		{Key: "process", Label: bilingualText("进程恢复", "Process"), Count: counts["process"], Note: bilingualText("crashed、restart_failed、节点短暂离线。", "crashed, restart_failed and brief node offline events.")},
		{Key: "unknown", Label: bilingualText("待确认", "Unknown"), Count: counts["unknown"], Note: bilingualText("证据不足或需要进一步补充采集。", "Evidence is still insufficient and needs more collection.")},
	}
	return cards
}

func buildDiagnosticBundleHTMLTimeline(task *DiagnosticTask, state *diagnosticBundleExecutionState) []diagnosticBundleHTMLTimelineItem {
	items := make([]diagnosticBundleHTMLTimelineItem, 0, 16)
	appendItem := func(at time.Time, tone, title, details string) {
		if at.IsZero() || strings.TrimSpace(title) == "" {
			return
		}
		items = append(items, diagnosticBundleHTMLTimelineItem{
			OccurredAt: at.UTC(),
			TimeLabel:  formatDiagnosticBundleTimeValue(at),
			Tone:       normalizeDiagnosticDisplayText(tone),
			Title:      normalizeDiagnosticDisplayText(title),
			Details:    normalizeDiagnosticDisplayText(details),
		})
	}
	if state != nil {
		for _, event := range state.ErrorEvents {
			if event == nil {
				continue
			}
			appendItem(event.OccurredAt, resolveDiagnosticRiskTone(task, state), bilingualText("错误事件", "Error Event"), firstNonEmptyString(event.Message, event.Evidence))
		}
		for _, event := range state.ProcessEvents {
			if event == nil {
				continue
			}
			appendItem(event.CreatedAt, "warning", bilingualText("进程事件", "Process Event"), fmt.Sprintf("%s · %s", normalizeDiagnosticDisplayText(string(event.EventType)), normalizeDiagnosticDisplayText(event.Details)))
		}
		for _, alert := range state.AlertSnapshot {
			if alert == nil {
				continue
			}
			appendItem(alert.FiringAt, strings.ToLower(strings.TrimSpace(string(alert.Severity))), bilingualText("告警触发", "Alert Fired"), firstNonEmptyString(alert.AlertName, alert.Summary))
		}
		if state.MetricsSnapshot != nil {
			for _, signal := range state.MetricsSnapshot.Signals {
				if strings.EqualFold(strings.TrimSpace(signal.Status), "healthy") || len(signal.Series) == 0 {
					continue
				}
				top := signal.Series[0]
				if top.MaxAt == nil {
					continue
				}
				appendItem(*top.MaxAt, signal.Status, bilingualText("指标峰值", "Metric Peak"), fmt.Sprintf("%s · %s", signal.Title, formatDiagnosticMetricValue(signal.Unit, top.MaxValue)))
			}
		}
	}
	if task != nil && task.StartedAt != nil && !task.StartedAt.IsZero() {
		appendItem(*task.StartedAt, "neutral", bilingualText("开始采集", "Task Started"), normalizeDiagnosticDisplayText(task.Summary))
	}
	if task != nil && task.CompletedAt != nil && !task.CompletedAt.IsZero() {
		appendItem(*task.CompletedAt, strings.ToLower(strings.TrimSpace(string(task.Status))), bilingualText("生成诊断报告", "Report Generated"), bilingualText("采集执行完成。", "Task execution completed."))
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].OccurredAt.Before(items[j].OccurredAt)
	})
	if len(items) > 12 {
		items = items[len(items)-12:]
	}
	return items
}

func buildDiagnosticBundleHTMLSignalCards(state *diagnosticBundleExecutionState) []diagnosticBundleHTMLSignalCard {
	if state == nil || state.MetricsSnapshot == nil || len(state.MetricsSnapshot.Signals) == 0 {
		return []diagnosticBundleHTMLSignalCard{}
	}
	preferred := []string{"cpu_usage_high", "memory_usage_high", "old_gen_usage_high", "gc_time_ratio_high"}
	byKey := make(map[string]diagnosticPrometheusSignal, len(state.MetricsSnapshot.Signals))
	for _, signal := range state.MetricsSnapshot.Signals {
		byKey[signal.Key] = signal
	}
	cards := make([]diagnosticBundleHTMLSignalCard, 0, 4)
	for _, key := range preferred {
		signal, ok := byKey[key]
		if !ok {
			continue
		}
		cards = append(cards, buildDiagnosticBundleHTMLSignalCard(signal))
	}
	if len(cards) == 0 {
		for _, signal := range state.MetricsSnapshot.Signals {
			cards = append(cards, buildDiagnosticBundleHTMLSignalCard(signal))
			if len(cards) >= 4 {
				break
			}
		}
	}
	return cards
}

func buildDiagnosticBundleHTMLSignalCard(signal diagnosticPrometheusSignal) diagnosticBundleHTMLSignalCard {
	card := diagnosticBundleHTMLSignalCard{
		Key:            signal.Key,
		Title:          normalizeDiagnosticDisplayText(signal.Title),
		Status:         normalizeDiagnosticDisplayText(signal.Status),
		Summary:        normalizeDiagnosticDisplayText(signal.Summary),
		ThresholdText:  normalizeDiagnosticDisplayText(signal.ThresholdText),
		Interpretation: buildDiagnosticSignalInterpretation(signal),
		Threshold:      signal.Threshold,
		Comparator:     signal.Comparator,
		Unit:           signal.Unit,
		Points:         []diagnosticPrometheusPoint{},
	}
	if len(signal.Series) == 0 {
		return card
	}
	series := signal.Series[0]
	card.Instance = normalizeDiagnosticDisplayText(series.Instance)
	card.LastValue = formatDiagnosticMetricValue(signal.Unit, series.LastValue)
	card.PeakValue = formatDiagnosticMetricValue(signal.Unit, series.MaxValue)
	card.PeakAt = formatDiagnosticBundleTime(series.MaxAt)
	card.Points = append(card.Points, series.Points...)
	return card
}

func buildDiagnosticBundleHTMLArtifactGroups(bundleDir string, artifacts []*diagnosticBundleArtifact) []diagnosticBundleHTMLArtifactGroup {
	if len(artifacts) == 0 {
		return []diagnosticBundleHTMLArtifactGroup{}
	}
	groupMap := make(map[string][]diagnosticBundleHTMLArtifactView)
	for _, artifact := range artifacts {
		if artifact == nil {
			continue
		}
		view := buildDiagnosticBundleHTMLArtifactView(bundleDir, artifact)
		groupMap[artifact.Category] = append(groupMap[artifact.Category], view)
	}
	order := []string{
		"error_context",
		"metrics_snapshot",
		"config_snapshot",
		"directory_inventory",
		"alert_snapshot",
		"process_events",
		"log_sample",
		"thread_dump",
		"jvm_dump",
		"manifest",
		"html_summary",
		"diagnostic_report",
	}
	seen := make(map[string]struct{}, len(order))
	result := make([]diagnosticBundleHTMLArtifactGroup, 0, len(groupMap))
	for _, key := range order {
		items, ok := groupMap[key]
		if !ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, diagnosticBundleHTMLArtifactGroup{
			Key:   key,
			Label: resolveDiagnosticArtifactCategoryLabel(key),
			Items: items,
		})
	}
	remaining := make([]string, 0)
	for key := range groupMap {
		if _, ok := seen[key]; ok {
			continue
		}
		remaining = append(remaining, key)
	}
	sort.Strings(remaining)
	for _, key := range remaining {
		result = append(result, diagnosticBundleHTMLArtifactGroup{
			Key:   key,
			Label: resolveDiagnosticArtifactCategoryLabel(key),
			Items: groupMap[key],
		})
	}
	return result
}

func resolveDiagnosticClusterLabel(task *DiagnosticTask, state *diagnosticBundleExecutionState) string {
	if state != nil && state.ClusterSnapshot != nil {
		name := strings.TrimSpace(state.ClusterSnapshot.Name)
		if name != "" {
			return fmt.Sprintf("%s (#%d)", name, state.ClusterSnapshot.ID)
		}
		if state.ClusterSnapshot.ID > 0 {
			return fmt.Sprintf("#%d", state.ClusterSnapshot.ID)
		}
	}
	if task != nil && task.ClusterID > 0 {
		return fmt.Sprintf("#%d", task.ClusterID)
	}
	return "-"
}

func resolveDiagnosticWindowLabel(task *DiagnosticTask, state *diagnosticBundleExecutionState) string {
	window := resolveDiagnosticCollectionWindow(task, nil)
	if state != nil && state.WindowStart != nil && state.WindowEnd != nil {
		window.Start = state.WindowStart.UTC()
		window.End = state.WindowEnd.UTC()
	}
	return fmt.Sprintf("%s ~ %s", formatDiagnosticBundleTimeValue(window.Start), formatDiagnosticBundleTimeValue(window.End))
}

func resolveDiagnosticRiskTone(task *DiagnosticTask, state *diagnosticBundleExecutionState) string {
	if task != nil {
		switch task.Status {
		case DiagnosticTaskStatusFailed:
			return "critical"
		case DiagnosticTaskStatusRunning:
			return "warning"
		}
	}
	if state != nil {
		if state.InspectionDetail != nil && state.InspectionDetail.Report != nil {
			report := state.InspectionDetail.Report
			if report.Status == InspectionReportStatusFailed || report.CriticalCount > 0 {
				return "critical"
			}
			if report.WarningCount > 0 {
				return "warning"
			}
		}
		for _, event := range state.ProcessEvents {
			if event == nil {
				continue
			}
			switch event.EventType {
			case monitor.EventTypeRestartFailed, monitor.EventTypeCrashed:
				return "critical"
			case monitor.EventTypeNodeOffline:
				return "warning"
			}
		}
		// 检查 Prometheus 指标快照中的异常风险等级（防 nil 保护）
		// Inspect risk tone from Prometheus signals with nil-safety protection.
		if state.MetricsSnapshot != nil {
			for _, signal := range state.MetricsSnapshot.Signals {
				status := strings.ToLower(strings.TrimSpace(signal.Status))
				if status == "critical" {
					return "critical"
				}
				if status == "warning" {
					return "warning"
				}
			}
		}
		if state.ErrorGroup != nil {
			return "warning"
		}
	}
	return "healthy"
}

func resolveDiagnosticHealthCopy(riskTone string, state *diagnosticBundleExecutionState) (string, string) {
	primary := buildDiagnosticPrimaryFocus(state)
	switch riskTone {
	case "critical":
		return bilingualText("当前问题已影响运行稳定性", "Current issue impacts runtime stability"), primary
	case "warning":
		return bilingualText("当前存在待定位异常信号", "There are anomaly signals to investigate"), primary
	default:
		return bilingualText("当前未见明显高风险信号", "No high-risk signal is visible in this window"),
			bilingualText("当前报告主要用于复盘问题窗口内的关键证据。", "This report is mainly for reviewing evidence in the diagnostics window.")
	}
}

func buildDiagnosticPrimaryFocus(state *diagnosticBundleExecutionState) string {
	switch resolveDiagnosticPrimaryCategory(state) {
	case "dependency":
		return bilingualText("优先检查外部依赖连通性、DNS 解析与 connector / catalog 配置。", "Check dependency reachability, DNS resolution and connector/catalog settings first.")
	case "configuration":
		return bilingualText("优先核对最近配置变更与运行时配置是否一致。", "Compare recent config changes with runtime configs first.")
	case "resource":
		return bilingualText("优先检查资源曲线峰值、阈值命中时刻和是否存在持续高位。", "Review resource peaks, threshold breaches and sustained high usage first.")
	case "process":
		return bilingualText("优先检查 crashed / restart_failed 事件与启动脚本、依赖路径。", "Inspect crashed / restart_failed events together with startup scripts and dependency paths first.")
	default:
		return bilingualText("优先从关键发现与时间线中确认最早异常信号。", "Start from key findings and the timeline to identify the earliest anomaly.")
	}
}

func resolveDiagnosticPrimaryCategory(state *diagnosticBundleExecutionState) string {
	if state == nil {
		return "unknown"
	}
	if state.ErrorGroup != nil {
		text := strings.ToLower(strings.Join([]string{state.ErrorGroup.Title, state.ErrorGroup.SampleMessage, state.ErrorGroup.ExceptionClass}, " "))
		if category := classifyDiagnosticTextCategory(text); category != "unknown" {
			return category
		}
	}
	for _, event := range state.ProcessEvents {
		if event == nil {
			continue
		}
		switch event.EventType {
		case monitor.EventTypeRestartFailed, monitor.EventTypeCrashed, monitor.EventTypeNodeOffline:
			return "process"
		}
	}
	if state.MetricsSnapshot != nil {
		for _, signal := range state.MetricsSnapshot.Signals {
			if !strings.EqualFold(strings.TrimSpace(signal.Status), "healthy") && isDiagnosticResourceSignal(signal.Key) {
				return "resource"
			}
		}
	}
	if state.InspectionDetail != nil {
		for _, finding := range state.InspectionDetail.Findings {
			if finding == nil {
				continue
			}
			category := mapInspectionFindingToDiagnosticCategory(finding)
			if category != "unknown" {
				return category
			}
		}
	}
	return "unknown"
}

func mapInspectionFindingToDiagnosticCategory(finding *ClusterInspectionFindingInfo) string {
	if finding == nil {
		return "unknown"
	}
	text := strings.ToLower(strings.Join([]string{finding.CheckCode, finding.CheckName, finding.Summary, finding.Recommendation, finding.EvidenceSummary}, " "))
	return classifyDiagnosticTextCategory(text)
}

func classifyDiagnosticTextCategory(text string) string {
	switch {
	case strings.Contains(text, "deadline_exceeded"),
		strings.Contains(text, "dependency"),
		strings.Contains(text, "timeout"),
		strings.Contains(text, "connection"),
		strings.Contains(text, "connect timed out"),
		strings.Contains(text, "network"):
		return "dependency"
	case strings.Contains(text, "dns"),
		strings.Contains(text, "unknownhost"),
		strings.Contains(text, "no such host"),
		strings.Contains(text, "name or service not known"),
		strings.Contains(text, "connection refused"),
		strings.Contains(text, "refused"),
		strings.Contains(text, "unreachable"),
		strings.Contains(text, "reset by peer"),
		strings.Contains(text, "broken pipe"):
		return "dependency"
	case strings.Contains(text, "config"),
		strings.Contains(text, "catalog"),
		strings.Contains(text, "connector"),
		strings.Contains(text, "plugin"),
		strings.Contains(text, "classnotfound"),
		strings.Contains(text, "no suitable driver"),
		strings.Contains(text, "invalid argument"),
		strings.Contains(text, "parse config"),
		strings.Contains(text, "yaml"):
		return "configuration"
	case strings.Contains(text, "cpu"),
		strings.Contains(text, "heap"),
		strings.Contains(text, "memory"),
		strings.Contains(text, "gc"),
		strings.Contains(text, "fd"):
		return "resource"
	case strings.Contains(text, "restart"),
		strings.Contains(text, "offline"),
		strings.Contains(text, "process"):
		return "process"
	default:
		return "unknown"
	}
}

func isDiagnosticResourceSignal(key string) bool {
	switch strings.TrimSpace(key) {
	case "cpu_usage_high", "memory_usage_high", "fd_usage_high", "old_gen_usage_high", "gc_time_ratio_high":
		return true
	default:
		return false
	}
}

func buildDiagnosticFindingImpact(state *diagnosticBundleExecutionState) string {
	if state == nil {
		return "-"
	}
	hostSet := make(map[string]struct{})
	for _, event := range state.ErrorEvents {
		if event == nil {
			continue
		}
		hostSet[resolveDiagnosticHostLabel("", event.HostID, "")] = struct{}{}
	}
	for _, event := range state.ProcessEvents {
		if event == nil {
			continue
		}
		hostSet[resolveDiagnosticHostLabel("", event.HostID, "")] = struct{}{}
	}
	if len(hostSet) == 0 {
		return bilingualText("影响范围待确认", "Impact scope needs confirmation")
	}
	labels := make([]string, 0, len(hostSet))
	for label := range hostSet {
		labels = append(labels, label)
	}
	sort.Strings(labels)
	return strings.Join(labels, ", ")
}

func resolveDiagnosticInspectionImpact(report *ClusterInspectionReportInfo) string {
	if report == nil {
		return bilingualText("待确认", "to be confirmed")
	}
	return bilingualText(
		fmt.Sprintf("严重 %d / 告警 %d / 信息 %d", report.CriticalCount, report.WarningCount, report.InfoCount),
		fmt.Sprintf("%d critical / %d warning / %d info", report.CriticalCount, report.WarningCount, report.InfoCount),
	)
}

func buildDiagnosticResourceFinding(state *diagnosticBundleExecutionState) diagnosticBundleHTMLFindingCard {
	if state == nil || state.MetricsSnapshot == nil {
		return diagnosticBundleHTMLFindingCard{}
	}
	resourceSignals := make([]diagnosticPrometheusSignal, 0, 4)
	for _, signal := range state.MetricsSnapshot.Signals {
		if isDiagnosticResourceSignal(signal.Key) {
			resourceSignals = append(resourceSignals, signal)
		}
	}
	if len(resourceSignals) == 0 {
		return diagnosticBundleHTMLFindingCard{}
	}
	abnormal := 0
	for _, signal := range resourceSignals {
		if strings.TrimSpace(signal.Status) != "" && !strings.EqualFold(signal.Status, "healthy") {
			abnormal++
		}
	}
	severity := "healthy"
	summary := bilingualText("CPU、Heap、Old Gen 与 GC 未表现出持续高位。", "CPU, Heap, Old Gen and GC do not show a sustained high-usage pattern.")
	if abnormal > 0 {
		severity = "warning"
		summary = bilingualText(
			fmt.Sprintf("共有 %d 个资源类指标触达阈值，需要结合时间线继续判断。", abnormal),
			fmt.Sprintf("%d resource-oriented signals breached the threshold and should be reviewed on the timeline.", abnormal),
		)
	}
	return diagnosticBundleHTMLFindingCard{
		Severity: severity,
		Category: "resource",
		Title:    bilingualText("资源侧信号总结", "Resource Signal Summary"),
		Summary:  summary,
		Impact:   bilingualText("覆盖 CPU / Heap / Old Gen / GC 等核心运行指标。", "Covers CPU, Heap, Old Gen and GC signals."),
		Action:   buildDiagnosticSignalInterpretation(resourceSignals[0]),
	}
}

func buildDiagnosticSignalInterpretation(signal diagnosticPrometheusSignal) string {
	if len(signal.Series) == 0 {
		return signal.Summary
	}
	top := signal.Series[0]
	switch strings.ToLower(strings.TrimSpace(signal.Status)) {
	case "critical", "warning":
		return bilingualText(
			fmt.Sprintf("峰值 %s，实例 %s 于 %s 命中阈值 %s。", formatDiagnosticMetricValue(signal.Unit, top.MaxValue), top.Instance, formatDiagnosticBundleTime(top.MaxAt), signal.ThresholdText),
			fmt.Sprintf("Peak %s on %s at %s, breaching %s.", formatDiagnosticMetricValue(signal.Unit, top.MaxValue), top.Instance, formatDiagnosticBundleTime(top.MaxAt), signal.ThresholdText),
		)
	default:
		return bilingualText(
			fmt.Sprintf("最新值 %s，诊断窗口内未见持续阈值命中。", formatDiagnosticMetricValue(signal.Unit, top.LastValue)),
			fmt.Sprintf("Latest value %s, with no sustained threshold breach in this window.", formatDiagnosticMetricValue(signal.Unit, top.LastValue)),
		)
	}
}

func resolveDiagnosticLastSignalAt(state *diagnosticBundleExecutionState) string {
	var latest *time.Time
	track := func(value *time.Time) {
		if value == nil || value.IsZero() {
			return
		}
		if latest == nil || latest.Before(value.UTC()) {
			normalized := value.UTC()
			latest = &normalized
		}
	}
	if state != nil {
		if state.ErrorGroup != nil {
			track(&state.ErrorGroup.LastSeenAt)
		}
		for _, event := range state.ProcessEvents {
			if event == nil {
				continue
			}
			value := event.CreatedAt
			track(&value)
		}
		for _, alert := range state.AlertSnapshot {
			if alert == nil {
				continue
			}
			value := alert.LastSeenAt
			track(&value)
		}
	}
	return formatDiagnosticBundleTime(latest)
}

func buildDiagnosticBundleHTMLRecommendations(task *DiagnosticTask, state *diagnosticBundleExecutionState) []diagnosticBundleHTMLAdvice {
	items := make([]diagnosticBundleHTMLAdvice, 0, 3)
	appendAdvice := func(title, details string) {
		title = strings.TrimSpace(title)
		details = strings.TrimSpace(details)
		if title == "" && details == "" {
			return
		}
		items = append(items, diagnosticBundleHTMLAdvice{
			Title:   normalizeDiagnosticDisplayText(title),
			Details: normalizeDiagnosticDisplayText(details),
		})
	}

	// 优先从巡检发现中挑选 1~2 条最关键建议
	if state != nil && state.InspectionDetail != nil {
		for _, finding := range state.InspectionDetail.Findings {
			if finding == nil {
				continue
			}
			appendAdvice(
				firstNonEmptyString(finding.CheckName, finding.Summary),
				firstNonEmptyString(finding.Recommendation, finding.EvidenceSummary),
			)
			if len(items) >= 2 {
				break
			}
		}
	}
	// 兜底：针对错误组或告警给出 1 条高层动作建议
	if len(items) == 0 && state != nil && state.ErrorGroup != nil {
		appendAdvice(
			bilingualText("优先排查错误组根因", "Prioritize the root cause of the error group"),
			firstNonEmptyString(state.ErrorGroup.SampleMessage, state.ErrorGroup.Title),
		)
	}
	if len(items) == 0 && state != nil && len(state.AlertSnapshot) > 0 {
		alert := state.AlertSnapshot[0]
		if alert != nil {
			appendAdvice(
				bilingualText("先处理活动告警", "Address active alerts first"),
				firstNonEmptyString(alert.Summary, alert.Description, alert.AlertName),
			)
		}
	}
	// 不再强行追加通用长段落，仅在确有 JVM Dump 时补充一条提示
	if task != nil && task.Options.IncludeJVMDump {
		appendAdvice(
			bilingualText("复查 JVM Dump 元数据", "Review JVM dump metadata"),
			bilingualText("当前 MVP 仅登记 JVM Dump 远端路径与元数据，如需 hprof 二进制回传请走后续增强能力。", "The MVP records JVM dump metadata and remote paths only. Binary HPROF upload is deferred to a later enhancement."),
		)
	}
	return items
}

func buildDiagnosticBundleHTMLPassedChecks(task *DiagnosticTask, state *diagnosticBundleExecutionState, artifacts []*diagnosticBundleArtifact) []diagnosticBundleHTMLAdvice {
	items := make([]diagnosticBundleHTMLAdvice, 0, 4)
	appendAdvice := func(title, details string) {
		title = strings.TrimSpace(title)
		details = strings.TrimSpace(details)
		if title == "" && details == "" {
			return
		}
		items = append(items, diagnosticBundleHTMLAdvice{
			Title:   normalizeDiagnosticDisplayText(title),
			Details: normalizeDiagnosticDisplayText(details),
		})
	}

	artifactCountByCategory := make(map[string]int)
	for _, artifact := range artifacts {
		if artifact == nil {
			continue
		}
		artifactCountByCategory[strings.TrimSpace(artifact.Category)]++
	}
	if artifactCountByCategory["config_snapshot"] > 0 {
		appendAdvice(
			bilingualText("运行配置文件已采集", "Runtime config files collected"),
			bilingualText("报告已附带 seatunnel.yaml、hazelcast 配置等运行配置文件快照。", "The report contains runtime config snapshots such as seatunnel.yaml and Hazelcast configs."),
		)
	}
	if artifactCountByCategory["log_sample"] > 0 {
		appendAdvice(
			bilingualText("日志样本已落盘", "Log sample collected"),
			bilingualText("至少一个节点已采集到用于排查的日志样本。", "At least one node produced a log sample for investigation."),
		)
	}
	if artifactCountByCategory["thread_dump"] > 0 {
		appendAdvice(
			bilingualText("线程栈已采集", "Thread dump collected"),
			bilingualText("可直接打开线程栈产物定位阻塞、卡死或线程热点。", "Thread dump artifacts are available for blocking and hotspot analysis."),
		)
	}
	if task != nil && !task.Options.IncludeJVMDump {
		appendAdvice(
			bilingualText("JVM Dump 按策略跳过", "JVM dump intentionally skipped"),
			bilingualText("当前任务未开启 JVM Dump，不视为任务失败。", "JVM dump was disabled by task option and is not treated as a failure."),
		)
	}
	if state != nil && state.InspectionDetail != nil && state.InspectionDetail.Report != nil && state.InspectionDetail.Report.CriticalCount == 0 {
		appendAdvice(
			bilingualText("巡检未发现严重问题", "No critical inspection findings"),
			bilingualText("本次巡检上下文中没有 critical 级别发现项。", "The inspection context contains no critical findings."),
		)
	}
	if state != nil && len(state.AlertSnapshot) == 0 {
		appendAdvice(
			bilingualText("未检测到活动告警", "No active alerts detected"),
			bilingualText("告警快照为空，说明当前上下文没有关联的 firing 告警。", "The alert snapshot is empty, which means no firing alerts were associated with this context."),
		)
	}
	return items
}

func buildDiagnosticBundleHTMLArtifactView(bundleDir string, artifact *diagnosticBundleArtifact) diagnosticBundleHTMLArtifactView {
	relativePath := resolveDiagnosticBundleRelativePath(bundleDir, artifact.Path)
	preview, previewNote := readDiagnosticArtifactPreview(artifact)
	return diagnosticBundleHTMLArtifactView{
		Category:      artifact.Category,
		CategoryLabel: resolveDiagnosticArtifactCategoryLabel(artifact.Category),
		StepCode:      normalizeDiagnosticDisplayText(string(artifact.StepCode)),
		Status:        normalizeDiagnosticDisplayText(artifact.Status),
		Format:        normalizeDiagnosticDisplayText(artifact.Format),
		HostLabel:     resolveDiagnosticHostLabel(artifact.HostName, artifact.HostID, ""),
		LocalPath:     normalizeDiagnosticDisplayText(artifact.Path),
		RelativePath:  normalizeDiagnosticDisplayText(relativePath),
		RemotePath:    normalizeDiagnosticDisplayText(artifact.RemotePath),
		SizeLabel:     formatDiagnosticBytes(artifact.SizeBytes),
		Message:       normalizeDiagnosticDisplayText(artifact.Message),
		Preview:       preview,
		PreviewNote:   previewNote,
	}
}

func resolveDiagnosticArtifactCategoryLabel(category string) string {
	switch strings.TrimSpace(category) {
	case "error_context":
		return bilingualText("错误上下文", "Error Context")
	case "process_events":
		return bilingualText("进程事件", "Process Events")
	case "alert_snapshot":
		return bilingualText("告警快照", "Alert Snapshot")
	case "config_snapshot":
		return bilingualText("运行配置文件", "Runtime Config Files")
	case "directory_inventory":
		return bilingualText("目录清单", "Directory Inventory")
	case "metrics_snapshot":
		return bilingualText("指标快照", "Metrics Snapshot")
	case "log_sample":
		return bilingualText("日志样本", "Log Sample")
	case "thread_dump":
		return bilingualText("线程栈", "Thread Dump")
	case "jvm_dump":
		return bilingualText("JVM Dump", "JVM Dump")
	case "manifest":
		return bilingualText("Manifest", "Manifest")
	case "html_summary":
		return bilingualText("诊断报告", "Diagnostic Report")
	case "diagnostic_report":
		return bilingualText("诊断报告", "Diagnostic Report")
	default:
		return normalizeDiagnosticDisplayText(category)
	}
}

func resolveDiagnosticTriggerSourceLabel(source DiagnosticTaskSourceType) string {
	switch source {
	case DiagnosticTaskSourceManual:
		return bilingualText("手动创建", "Manual")
	case DiagnosticTaskSourceInspectionFinding:
		return bilingualText("巡检发现触发", "Inspection Finding")
	case DiagnosticTaskSourceErrorGroup:
		return bilingualText("错误组触发", "Error Group")
	case DiagnosticTaskSourceAlert:
		return bilingualText("告警触发", "Alert")
	default:
		return normalizeDiagnosticDisplayText(string(source))
	}
}

func resolveDiagnosticBundleRelativePath(bundleDir, path string) string {
	trimmedBundle := strings.TrimSpace(bundleDir)
	trimmedPath := strings.TrimSpace(path)
	if trimmedBundle == "" || trimmedPath == "" {
		return ""
	}
	relative, err := filepath.Rel(trimmedBundle, trimmedPath)
	if err != nil {
		return trimmedPath
	}
	if strings.HasPrefix(relative, "..") {
		return trimmedPath
	}
	return filepath.ToSlash(relative)
}

func readDiagnosticArtifactPreview(artifact *diagnosticBundleArtifact) (string, string) {
	if artifact == nil {
		return "", ""
	}
	category := strings.TrimSpace(artifact.Category)
	if category == "diagnostic_report" || category == "html_summary" {
		return "", bilingualText("当前文件即离线诊断报告首页。", "This file is the offline diagnostic report itself.")
	}
	path := strings.TrimSpace(artifact.Path)
	if path == "" {
		return "", ""
	}
	bytes, err := os.ReadFile(path)
	if err != nil {
		return "", bilingualText("产物预览读取失败。", "Failed to read artifact preview.")
	}
	content := string(bytes)
	if strings.TrimSpace(content) == "" {
		return "", bilingualText("产物文件为空。", "Artifact file is empty.")
	}
	const maxPreviewRunes = 6000
	preview, truncated := truncateDiagnosticText(content, maxPreviewRunes)
	if truncated {
		return preview, bilingualText("仅展示前 6000 个字符，完整内容请打开对应文件。", "Preview shows the first 6000 characters only. Open the file for full content.")
	}
	return preview, bilingualText("已展示完整产物内容。", "Showing full artifact content.")
}

func truncateDiagnosticText(value string, maxRunes int) (string, bool) {
	if maxRunes <= 0 {
		return "", false
	}
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value, false
	}
	return string(runes[:maxRunes]), true
}

func resolveDiagnosticHostLabel(hostName string, hostID uint, fallback string) string {
	if trimmed := strings.TrimSpace(hostName); trimmed != "" {
		return trimmed
	}
	if trimmed := strings.TrimSpace(fallback); trimmed != "" {
		return trimmed
	}
	if hostID > 0 {
		return fmt.Sprintf("#%d", hostID)
	}
	return "-"
}

func normalizeDiagnosticDisplayText(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "-"
	}
	return trimmed
}

func firstNonZeroInt(values ...int) int {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}

func formatDiagnosticBundleTime(value *time.Time) string {
	if value == nil || value.IsZero() {
		return "-"
	}
	return formatDiagnosticBundleTimeValue(*value)
}

func formatDiagnosticBundleTimeValue(value time.Time) string {
	if value.IsZero() {
		return "-"
	}
	return value.In(diagnosticDisplayLocation()).Format("2006-01-02 15:04:05 MST")
}

func diagnosticDisplayLocation() *time.Location {
	diagnosticDisplayTimezoneOnce.Do(func() {
		name := strings.TrimSpace(os.Getenv("STX_REPORT_TIMEZONE"))
		if name == "" {
			name = "Asia/Shanghai"
		}
		loc, err := time.LoadLocation(name)
		if err != nil {
			diagnosticDisplayTimezone = time.FixedZone("CST", 8*3600)
			return
		}
		diagnosticDisplayTimezone = loc
	})
	if diagnosticDisplayTimezone == nil {
		return time.FixedZone("CST", 8*3600)
	}
	return diagnosticDisplayTimezone
}

func (s *Service) sendDiagnosticAgentCommandWithRetry(ctx context.Context, agentID, command string, params map[string]string) (bool, string, error) {
	if s == nil || s.agentSender == nil {
		return false, "", fmt.Errorf("agent sender is unavailable")
	}
	var (
		success bool
		output  string
		err     error
	)
	for attempt := 0; attempt < 8; attempt++ {
		success, output, err = s.agentSender.SendCommand(ctx, agentID, command, params)
		if !shouldRetryDiagnosticAgentCommand(err, output) {
			return success, output, err
		}
		select {
		case <-ctx.Done():
			return success, output, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return success, output, err
}

func shouldRetryDiagnosticAgentCommand(err error, output string) bool {
	combined := strings.ToLower(strings.TrimSpace(firstNonEmptyString(output, fmt.Sprint(err))))
	if combined == "" {
		return false
	}
	return strings.Contains(combined, "command stream not available") ||
		strings.Contains(combined, "agent not found") ||
		strings.Contains(combined, "agent command stream not found") ||
		strings.Contains(combined, "no active command stream")
}

func formatDiagnosticBytes(size int64) string {
	if size <= 0 {
		return "-"
	}
	units := []string{"B", "KB", "MB", "GB", "TB"}
	value := float64(size)
	unit := 0
	for value >= 1024 && unit < len(units)-1 {
		value = value / 1024
		unit++
	}
	if unit == 0 {
		return fmt.Sprintf("%d %s", size, units[unit])
	}
	return fmt.Sprintf("%.1f %s", value, units[unit])
}

func formatDiagnosticMetricValue(unit string, value float64) string {
	switch strings.TrimSpace(unit) {
	case "ratio":
		return fmt.Sprintf("%.1f%%", value*100)
	case "percent":
		return fmt.Sprintf("%.1f%%", value)
	case "cores":
		return fmt.Sprintf("%.2f", value)
	case "bool":
		if value >= 1 {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprintf("%.2f", value)
	}
}

func newDiagnosticBundleHTMLTemplate(lang DiagnosticLanguage) (*template.Template, error) {
	lang = normalizeDiagnosticLanguage(string(lang))
	return template.New("diagnostic-summary").Funcs(template.FuncMap{
		"pair": func(zh, en string) template.HTML {
			return renderDiagnosticLocalizedPairByLanguage(zh, en, lang)
		},
		"loc": func(value interface{}) template.HTML {
			if value == nil {
				return renderDiagnosticLocalizedTextByLanguage("-", lang)
			}
			return renderDiagnosticLocalizedTextByLanguage(fmt.Sprint(value), lang)
		},
		"formatTime": func(value interface{}) string {
			switch typed := value.(type) {
			case *time.Time:
				return formatDiagnosticBundleTime(typed)
			case time.Time:
				return formatDiagnosticBundleTimeValue(typed)
			default:
				return "-"
			}
		},
		"statusClass": func(status interface{}) string {
			return diagnosticHTMLStatusClass(fmt.Sprint(status))
		},
		"toneClass": func(tone interface{}) string {
			return diagnosticHTMLToneClass(fmt.Sprint(tone))
		},
		"formatBytes": func(size int64) string {
			return formatDiagnosticBytes(size)
		},
		"formatMetricValue": func(unit string, value float64) string {
			return formatDiagnosticMetricValue(unit, value)
		},
		"metricChartSVG": func(points []diagnosticPrometheusPoint, threshold float64, comparator string, unit string) template.HTML {
			return renderDiagnosticMetricChart(points, threshold, comparator, unit, lang)
		},
		"shortHash": func(value string) string {
			value = strings.TrimSpace(value)
			if len(value) <= 12 {
				return value
			}
			return value[:12]
		},
		"stxBrandMark": func() template.URL {
			return template.URL(stxBrandMarkBase64)
		},
	}).Parse(diagnosticBundleHTMLTemplate)
}

func renderDiagnosticBundleHTMLDocument(payload *diagnosticBundleHTMLPayload, lang DiagnosticLanguage) ([]byte, error) {
	tmpl, err := newDiagnosticBundleHTMLTemplate(lang)
	if err != nil {
		return nil, err
	}
	documentLang := normalizeDiagnosticLanguage(string(lang))
	if payload == nil {
		payload = &diagnosticBundleHTMLPayload{}
	}
	payloadCopy := *payload
	payloadCopy.Language = documentLang
	var buffer bytes.Buffer
	if err := tmpl.Execute(&buffer, &payloadCopy); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func renderDiagnosticMetricChart(points []diagnosticPrometheusPoint, threshold float64, comparator string, unit string, lang DiagnosticLanguage) template.HTML {
	if len(points) < 2 {
		return ""
	}
	const width = 360.0
	const height = 156.0
	const leftPadding = 48.0
	const rightPadding = 16.0
	const topPadding = 12.0
	const bottomPadding = 32.0

	minValue := points[0].Value
	maxValue := points[0].Value
	for _, point := range points[1:] {
		if point.Value < minValue {
			minValue = point.Value
		}
		if point.Value > maxValue {
			maxValue = point.Value
		}
	}
	if threshold < minValue {
		minValue = threshold
	}
	if threshold > maxValue {
		maxValue = threshold
	}
	if minValue == maxValue {
		maxValue = minValue + 1
	}

	start := points[0].Timestamp.Unix()
	end := points[len(points)-1].Timestamp.Unix()
	if end <= start {
		end = start + 1
	}
	plotWidth := width - leftPadding - rightPadding
	plotHeight := height - topPadding - bottomPadding
	pathParts := make([]string, 0, len(points))
	for index, point := range points {
		x := leftPadding + (float64(point.Timestamp.Unix()-start)/float64(end-start))*plotWidth
		y := topPadding + (1-((point.Value-minValue)/(maxValue-minValue)))*plotHeight
		if index == 0 {
			pathParts = append(pathParts, fmt.Sprintf("M %.2f %.2f", x, y))
			continue
		}
		pathParts = append(pathParts, fmt.Sprintf("L %.2f %.2f", x, y))
	}
	thresholdY := topPadding + (1-((threshold-minValue)/(maxValue-minValue)))*plotHeight
	if thresholdY < topPadding {
		thresholdY = topPadding
	}
	if thresholdY > height-bottomPadding {
		thresholdY = height - bottomPadding
	}
	strokeColor := "#2563eb"
	if strings.TrimSpace(comparator) == "lt" {
		strokeColor = "#dc2626"
	}
	axisColor := "#94a3b8"
	gridColor := "#e2e8f0"
	lastX := leftPadding + (float64(points[len(points)-1].Timestamp.Unix()-start)/float64(end-start))*plotWidth
	lastY := topPadding + (1-((points[len(points)-1].Value-minValue)/(maxValue-minValue)))*plotHeight
	midTime := points[0].Timestamp.Add(points[len(points)-1].Timestamp.Sub(points[0].Timestamp) / 2)
	formatLabel := func(ts time.Time) string {
		return ts.Local().Format("01-02 15:04")
	}
	yTop := formatDiagnosticMetricValue(unit, maxValue)
	yBottom := formatDiagnosticMetricValue(unit, minValue)
	thresholdLabel := chooseDiagnosticLocalizedText(diagnosticLocalizedText{
		ZH: fmt.Sprintf("阈值 %s", formatDiagnosticMetricValue(unit, threshold)),
		EN: fmt.Sprintf("Threshold %s", formatDiagnosticMetricValue(unit, threshold)),
	}, lang)
	ariaLabel := chooseDiagnosticLocalizedText(diagnosticLocalizedText{
		ZH: "诊断指标图表",
		EN: "diagnostic metric chart",
	}, lang)
	svg := fmt.Sprintf(
		`<svg viewBox="0 0 %.0f %.0f" class="metric-chart" preserveAspectRatio="none" aria-label="%s">
<line x1="%.2f" y1="%.2f" x2="%.2f" y2="%.2f" class="metric-chart-axis" style="stroke:%s"/>
<line x1="%.2f" y1="%.2f" x2="%.2f" y2="%.2f" class="metric-chart-axis" style="stroke:%s"/>
<line x1="%.2f" y1="%.2f" x2="%.2f" y2="%.2f" class="metric-chart-grid" style="stroke:%s"/>
<line x1="%.2f" y1="%.2f" x2="%.2f" y2="%.2f" class="metric-chart-threshold"/>
<path d="%s" class="metric-chart-path" style="stroke:%s"/>
<circle cx="%.2f" cy="%.2f" r="3.2" class="metric-chart-dot"/>
<text x="%.2f" y="%.2f" class="metric-chart-label y-top">%s</text>
<text x="%.2f" y="%.2f" class="metric-chart-label y-bottom">%s</text>
<text x="%.2f" y="%.2f" class="metric-chart-label x-start">%s</text>
<text x="%.2f" y="%.2f" class="metric-chart-label x-mid">%s</text>
<text x="%.2f" y="%.2f" class="metric-chart-label x-end">%s</text>
<text x="%.2f" y="%.2f" class="metric-chart-label threshold-label">%s</text>
</svg>`,
		width, height, template.HTMLEscapeString(ariaLabel),
		leftPadding, topPadding, leftPadding, height-bottomPadding, axisColor,
		leftPadding, height-bottomPadding, width-rightPadding, height-bottomPadding, axisColor,
		leftPadding, topPadding+plotHeight/2, width-rightPadding, topPadding+plotHeight/2, gridColor,
		leftPadding, thresholdY, width-rightPadding, thresholdY,
		strings.Join(pathParts, " "), strokeColor,
		lastX, lastY,
		8.0, topPadding+4.0, template.HTMLEscapeString(yTop),
		8.0, height-bottomPadding, template.HTMLEscapeString(yBottom),
		leftPadding, height-10.0, template.HTMLEscapeString(formatLabel(points[0].Timestamp)),
		leftPadding+plotWidth/2, height-10.0, template.HTMLEscapeString(formatLabel(midTime)),
		width-rightPadding, height-10.0, template.HTMLEscapeString(formatLabel(points[len(points)-1].Timestamp)),
		width-rightPadding, thresholdY-4.0, template.HTMLEscapeString(thresholdLabel),
	)
	return template.HTML(svg)
}

func diagnosticHTMLStatusClass(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "succeeded", "completed", "healthy", "created":
		return "status-ok"
	case "failed", "critical":
		return "status-critical"
	case "running", "warning":
		return "status-warn"
	case "skipped":
		return "status-skip"
	default:
		return "status-neutral"
	}
}

func diagnosticHTMLToneClass(tone string) string {
	switch strings.ToLower(strings.TrimSpace(tone)) {
	case "healthy":
		return "tone-healthy"
	case "warning":
		return "tone-warning"
	case "critical":
		return "tone-critical"
	default:
		return "tone-neutral"
	}
}

func diagnosticTaskBundleDir(taskID uint) string {
	baseDir := strings.TrimSpace(config.GetStorageConfig().BaseDir)
	if baseDir == "" {
		baseDir = "./data/storage"
	}
	return filepath.Join(baseDir, "diagnostics", "tasks", fmt.Sprintf("%d", taskID))
}

func buildDiagnosticLogCandidates(target DiagnosticTaskNodeTarget, errorEvents []*SeatunnelErrorEvent) []string {
	candidates := make([]string, 0, 4)
	seen := make(map[string]struct{})
	for _, event := range errorEvents {
		if event == nil {
			continue
		}
		if target.NodeID > 0 && event.NodeID > 0 {
			if event.NodeID != target.NodeID {
				continue
			}
		} else if event.HostID != target.HostID {
			continue
		}
		if role := strings.TrimSpace(target.Role); role != "" && strings.TrimSpace(event.Role) != "" && !strings.EqualFold(event.Role, role) {
			continue
		}
		if installDir := strings.TrimSpace(target.InstallDir); installDir != "" && strings.TrimSpace(event.InstallDir) != "" && strings.TrimSpace(event.InstallDir) != installDir {
			continue
		}
		if path := strings.TrimSpace(event.SourceFile); path != "" {
			if _, ok := seen[path]; !ok {
				seen[path] = struct{}{}
				candidates = append(candidates, path)
			}
		}
	}
	// target.InstallDir 是远端 Seatunnel 安装路径，通常运行在 Linux 上，这里必须使用 POSIX 路径拼接。
	defaultLog := path.Join(target.InstallDir, "logs", diagnosticDefaultLogFile(target.Role))
	if _, ok := seen[defaultLog]; !ok {
		candidates = append(candidates, defaultLog)
	}
	return candidates
}

var diagnosticLogTimestampPattern = regexp.MustCompile(`(?:\[[^\]]*\]\s+)?(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}(?:,\d{3})?)`)

func (s *Service) collectDiagnosticWindowedLogSnippet(ctx context.Context, target DiagnosticTaskNodeTarget, candidate string, window diagnosticCollectionWindow) (string, string, error) {
	if s.agentSender == nil {
		return "", "agent sender is unavailable", fmt.Errorf("agent sender is unavailable")
	}
	contents := make([]string, 0, 4)
	days := diagnosticWindowDays(window.Start, window.End)
	for _, day := range days {
		params := map[string]string{
			"log_file":   candidate,
			"mode":       "all",
			"start_time": window.Start.Format(time.RFC3339),
			"end_time":   window.End.Format(time.RFC3339),
		}
		if isDiagnosticCurrentLogDay(day, window.End) {
			// current active file
		} else {
			params["date"] = day.Format("2006-01-02")
		}
		success, output, err := s.sendDiagnosticAgentCommandWithRetry(ctx, target.AgentID, "get_logs", params)
		if err != nil || !success {
			continue
		}
		if strings.TrimSpace(output) != "" {
			contents = append(contents, output)
		}
	}
	if len(contents) == 0 {
		return "", bilingualText("未读取到日志文件内容。", "No log file content was read."), fmt.Errorf("no log content")
	}

	snippet, _, sawTimestamp := extractDiagnosticLogWindowContent(contents, window.Start, window.End)
	if strings.TrimSpace(snippet) == "" {
		if sawTimestamp {
			return "", bilingualText("指定时间窗内未命中日志片段。", "No log entries matched the diagnostics window."), nil
		}
		snippet = strings.TrimSpace(strings.Join(contents, "\n"))
	}
	return snippet, "", nil
}

func buildDiagnosticLogPreview(content string, maxLines int) (string, bool, int) {
	normalized := normalizeDiagnosticDisplayText(content)
	if normalized == "" || maxLines <= 0 {
		return normalized, false, 0
	}
	lines := strings.Split(normalized, "\n")
	if len(lines) <= maxLines {
		return normalized, false, len(lines)
	}
	return strings.Join(lines[:maxLines], "\n"), true, maxLines
}

func sanitizeDiagnosticFileNameSegment(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	sanitized := diagnosticFileNameUnsafeChars.ReplaceAllString(trimmed, "-")
	sanitized = diagnosticFileNameDashRuns.ReplaceAllString(sanitized, "-")
	sanitized = strings.Trim(sanitized, ".-_")
	return sanitized
}

func buildDiagnosticLogSampleFileName(hostID uint, hostName, sourcePath string) string {
	baseName := strings.TrimSpace(filepath.Base(sourcePath))
	if baseName == "" || baseName == "." || baseName == string(filepath.Separator) {
		baseName = "log-sample.log"
	}
	fileNamePrefix := sanitizeDiagnosticFileNameSegment(hostName)
	if fileNamePrefix == "" {
		fileNamePrefix = fmt.Sprintf("host-%d", hostID)
	}
	fileName := fmt.Sprintf("%s-%s", fileNamePrefix, baseName)
	if filepath.Ext(baseName) == "" {
		return fileName + ".log"
	}
	return fileName
}

func buildDiagnosticTaskPreviewFileURL(taskID uint, relativePath string) string {
	relativePath = strings.Trim(filepath.ToSlash(strings.TrimSpace(relativePath)), "/")
	if taskID == 0 || relativePath == "" {
		return ""
	}
	parts := strings.Split(relativePath, "/")
	for index, part := range parts {
		parts[index] = url.PathEscape(part)
	}
	return fmt.Sprintf("/api/v1/diagnostics/tasks/%d/files/%s", taskID, strings.Join(parts, "/"))
}

func diagnosticWindowDays(start, end time.Time) []time.Time {
	if end.Before(start) {
		start, end = end, start
	}
	start = start.Local()
	end = end.Local()
	startDay := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location())
	endDay := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, end.Location())
	days := make([]time.Time, 0, int(endDay.Sub(startDay)/(24*time.Hour))+1)
	for day := startDay; !day.After(endDay); day = day.Add(24 * time.Hour) {
		days = append(days, day)
	}
	return days
}

func isDiagnosticCurrentLogDay(day, end time.Time) bool {
	y1, m1, d1 := day.Date()
	y2, m2, d2 := end.Local().Date()
	return y1 == y2 && m1 == m2 && d1 == d2
}

func extractDiagnosticLogWindowContent(contents []string, start, end time.Time) (string, bool, bool) {
	type logEntry struct {
		ts      time.Time
		hasTime bool
		lines   []string
	}
	entries := make([]logEntry, 0, 64)
	appendLine := func(current *logEntry, line string) *logEntry {
		if current == nil {
			entry := logEntry{lines: []string{line}}
			entries = append(entries, entry)
			return &entries[len(entries)-1]
		}
		current.lines = append(current.lines, line)
		return current
	}

	var current *logEntry
	for _, content := range contents {
		for _, line := range strings.Split(content, "\n") {
			if ts, ok := parseDiagnosticLogTimestamp(line); ok {
				entry := logEntry{
					ts:      ts,
					hasTime: true,
					lines:   []string{line},
				}
				entries = append(entries, entry)
				current = &entries[len(entries)-1]
				continue
			}
			current = appendLine(current, line)
		}
	}

	filtered := make([]string, 0, 256)
	sawTimestamp := false
	matchedWindow := false
	for _, entry := range entries {
		if entry.hasTime {
			sawTimestamp = true
		}
		if entry.hasTime && (entry.ts.Before(start) || entry.ts.After(end)) {
			continue
		}
		if entry.hasTime {
			matchedWindow = true
		}
		for _, line := range entry.lines {
			filtered = append(filtered, line)
		}
	}
	return strings.TrimSpace(strings.Join(filtered, "\n")), matchedWindow, sawTimestamp
}

func parseDiagnosticLogTimestamp(line string) (time.Time, bool) {
	matches := diagnosticLogTimestampPattern.FindStringSubmatch(line)
	if len(matches) != 2 {
		return time.Time{}, false
	}
	value := strings.TrimSpace(matches[1])
	layouts := []string{"2006-01-02 15:04:05,000", "2006-01-02 15:04:05"}
	for _, layout := range layouts {
		if parsed, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func diagnosticDefaultLogFile(role string) string {
	switch strings.TrimSpace(role) {
	case "master":
		return "seatunnel-engine-master.log"
	case "worker":
		return "seatunnel-engine-worker.log"
	default:
		return "seatunnel-engine-server.log"
	}
}

func sortedDiagnosticTaskTargets(targets DiagnosticTaskNodeTargets) []DiagnosticTaskNodeTarget {
	items := make([]DiagnosticTaskNodeTarget, 0, len(targets))
	items = append(items, targets...)
	sort.Slice(items, func(i, j int) bool {
		if items[i].HostID != items[j].HostID {
			return items[i].HostID < items[j].HostID
		}
		return items[i].Role < items[j].Role
	})
	return items
}

func normalizeDiagnosticFileRole(role string) string {
	role = strings.TrimSpace(role)
	if role == "" || role == "master/worker" {
		return "hybrid"
	}
	return strings.ReplaceAll(role, "/", "-")
}

const diagnosticBundleHTMLTemplate = `<!DOCTYPE html>
<html lang="{{if eq .Language "en"}}en{{else}}zh-CN{{end}}">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0" />
  <title>{{pair "STX 诊断报告" "STX Diagnostic Report"}}</title>
  <link rel="icon" type="image/png" href="{{stxBrandMark}}" />
  <link rel="apple-touch-icon" href="{{stxBrandMark}}" />
  <script>
    (function() {
      try {
        var t = localStorage.getItem('stx-diagnostic-theme');
        if (t === 'dark' || t === 'light') {
          document.documentElement.setAttribute('data-theme', t);
        } else if (window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches) {
          document.documentElement.setAttribute('data-theme', 'dark');
        }
      } catch (e) {}
    })();
  </script>
  <style>
    /*
     * 诊断报告设计规范与通用美学约定（遵照 .trellis/spec/frontend/ui-conventions.md）
     * Diagnostic report design specifications & UI conventions.
     */
    :root, html[data-theme="light"] {
      color-scheme: light dark;
      --bg: #f8fafc;
      --panel: #ffffff;
      --panel-soft: #f8fbff;
      --panel-hover: #f1f5f9;
      --border: #e2e8f0;
      --border-strong: #cbd5e1;
      --muted: #64748b;
      --text: #0f172a;
      --text-sub: #334155;
      --primary: #2563eb;
      --primary-soft: #eff6ff;
      --primary-border: #bfdbfe;
      --ok: #10b981;
      --ok-soft: #f0fdf4;
      --ok-border: rgba(16, 185, 129, 0.28);
      --warn: #f59e0b;
      --warn-soft: #fffbeb;
      --warn-border: rgba(245, 158, 11, 0.28);
      --critical: #ef4444;
      --critical-soft: #fef2f2;
      --critical-border: rgba(239, 68, 68, 0.28);
      --neutral: #3b82f6;
      --neutral-soft: #eff6ff;
      --neutral-border: rgba(59, 130, 246, 0.24);
      --skip: #94a3b8;
      --skip-soft: #f8fafc;
      --skip-border: rgba(148, 163, 184, 0.36);
      --code-bg: #0f172a;
      --code-text: #e2e8f0;
      --code-inline-bg: #eff6ff;
      --code-inline-text: #1d4ed8;
      --code-inline-border: rgba(37, 99, 235, 0.16);
      /* 统一圆角规范 / Standardized border radius hierarchy */
      --radius-lg: 16px;
      --radius-md: 12px;
      --radius-sm: 8px;
      --radius-full: 999px;
    }

    @media (prefers-color-scheme: dark) {
      :root:not([data-theme="light"]) {
        --bg: #0b0f19;
        --panel: #111827;
        --panel-soft: #172033;
        --panel-hover: #1f293d;
        --border: #1f2937;
        --border-strong: #374151;
        --muted: #9ca3af;
        --text: #f9fafb;
        --text-sub: #cbd5e1;
        --primary: #3b82f6;
        --primary-soft: rgba(59, 130, 246, 0.16);
        --primary-border: rgba(59, 130, 246, 0.32);
        --ok: #10b981;
        --ok-soft: rgba(16, 185, 129, 0.14);
        --ok-border: rgba(16, 185, 129, 0.28);
        --warn: #f59e0b;
        --warn-soft: rgba(245, 158, 11, 0.14);
        --warn-border: rgba(245, 158, 11, 0.28);
        --critical: #ef4444;
        --critical-soft: rgba(239, 68, 68, 0.16);
        --critical-border: rgba(239, 68, 68, 0.32);
        --neutral: #60a5fa;
        --neutral-soft: rgba(59, 130, 246, 0.16);
        --neutral-border: rgba(59, 130, 246, 0.3);
        --skip: #64748b;
        --skip-soft: rgba(100, 116, 139, 0.16);
        --skip-border: rgba(100, 116, 139, 0.3);
        --code-bg: #030712;
        --code-text: #e2e8f0;
        --code-inline-bg: rgba(59, 130, 246, 0.14);
        --code-inline-text: #93c5fd;
        --code-inline-border: rgba(59, 130, 246, 0.28);
      }
    }

    html[data-theme="dark"] {
      --bg: #0b0f19;
      --panel: #111827;
      --panel-soft: #172033;
      --panel-hover: #1f293d;
      --border: #1f2937;
      --border-strong: #374151;
      --muted: #9ca3af;
      --text: #f9fafb;
      --text-sub: #cbd5e1;
      --primary: #3b82f6;
      --primary-soft: rgba(59, 130, 246, 0.16);
      --primary-border: rgba(59, 130, 246, 0.32);
      --ok: #10b981;
      --ok-soft: rgba(16, 185, 129, 0.14);
      --ok-border: rgba(16, 185, 129, 0.28);
      --warn: #f59e0b;
      --warn-soft: rgba(245, 158, 11, 0.14);
      --warn-border: rgba(245, 158, 11, 0.28);
      --critical: #ef4444;
      --critical-soft: rgba(239, 68, 68, 0.16);
      --critical-border: rgba(239, 68, 68, 0.32);
      --neutral: #60a5fa;
      --neutral-soft: rgba(59, 130, 246, 0.16);
      --neutral-border: rgba(59, 130, 246, 0.3);
      --skip: #64748b;
      --skip-soft: rgba(100, 116, 139, 0.16);
      --skip-border: rgba(100, 116, 139, 0.3);
      --code-bg: #030712;
      --code-text: #e2e8f0;
      --code-inline-bg: rgba(59, 130, 246, 0.14);
      --code-inline-text: #93c5fd;
      --code-inline-border: rgba(59, 130, 246, 0.28);
    }

    * { box-sizing: border-box; }
    html { scroll-behavior: smooth; }
    body {
      margin: 0;
      padding: 24px;
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
      color: var(--text);
      background: var(--bg);
      line-height: 1.6;
      -webkit-font-smoothing: antialiased;
      transition: background 0.2s ease, color 0.2s ease;
    }
    h1, h2, h3, h4, p { margin: 0; }
    a { color: var(--primary); text-decoration: none; }
    a:hover { text-decoration: underline; }
    .page {
      max-width: 1520px;
      margin: 0 auto;
      display: flex;
      flex-direction: column;
      gap: 18px;
    }

    /* 报告主容器与侧栏 / Report Shell & Navigation Sidebar */
    .report-shell {
      display: grid;
      grid-template-columns: 260px minmax(0, 1fr);
      gap: 20px;
      align-items: start;
    }
    .report-sidebar {
      position: sticky;
      top: 20px;
      align-self: start;
      max-height: calc(100vh - 40px);
      display: flex;
      flex-direction: column;
      border: 1px solid var(--border);
      border-radius: var(--radius-lg);
      background: var(--panel);
      padding: 16px 14px;
      box-shadow: 0 2px 12px rgba(0, 0, 0, 0.05);
      min-width: 0;
      overflow-y: auto;
    }
    .sidebar-brand {
      padding-bottom: 12px;
      border-bottom: 1px solid var(--border);
      margin-bottom: 12px;
    }
    .stx-brand-lockup {
      display: flex;
      align-items: center;
      gap: 10px;
    }
    .stx-brand-logo {
      width: 32px;
      height: 32px;
      border-radius: 8px;
      object-fit: contain;
      flex-shrink: 0;
    }
    .stx-brand-text {
      min-width: 0;
      display: flex;
      flex-direction: column;
    }
    .stx-brand-title {
      font-size: 16px;
      font-weight: 800;
      letter-spacing: 0.04em;
      color: var(--text);
      line-height: 1.2;
    }
    .stx-brand-subtitle {
      font-size: 11px;
      color: var(--muted);
      font-weight: 500;
      line-height: 1.3;
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
    }
    .sidebar-status-pill {
      display: flex;
      align-items: center;
      gap: 7px;
      padding: 6px 10px;
      border-radius: var(--radius-full);
      font-size: 12px;
      font-weight: 600;
      margin-bottom: 12px;
      border: 1px solid var(--border);
      background: var(--panel-soft);
      min-width: 0;
    }
    .sidebar-status-pill .status-dot {
      width: 7px;
      height: 7px;
      border-radius: 50%;
      flex-shrink: 0;
    }
    .sidebar-status-pill .status-text {
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
      min-width: 0;
    }
    .sidebar-status-pill.tone-healthy {
      background: var(--ok-soft);
      border-color: var(--ok-border);
      color: var(--ok);
    }
    .sidebar-status-pill.tone-healthy .status-dot { background: var(--ok); }
    .sidebar-status-pill.tone-warning {
      background: var(--warn-soft);
      border-color: var(--warn-border);
      color: var(--warn);
    }
    .sidebar-status-pill.tone-warning .status-dot { background: var(--warn); }
    .sidebar-status-pill.tone-critical {
      background: var(--critical-soft);
      border-color: var(--critical-border);
      color: var(--critical);
    }
    .sidebar-status-pill.tone-critical .status-dot { background: var(--critical); }
    .sidebar-status-pill.tone-neutral {
      background: var(--neutral-soft);
      border-color: var(--neutral-border);
      color: var(--neutral);
    }
    .sidebar-status-pill.tone-neutral .status-dot { background: var(--neutral); }

    .sidebar-nav {
      display: flex;
      flex-direction: column;
      gap: 4px;
      flex: 1 1 auto;
      min-width: 0;
    }
    .sidebar-link {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 8px;
      text-decoration: none;
      color: var(--text-sub);
      border: 1px solid transparent;
      border-radius: var(--radius-sm);
      padding: 8px 10px;
      transition: all 0.16s ease;
      background: transparent;
      min-width: 0;
    }
    .sidebar-link:hover {
      background: var(--panel-hover);
      border-color: var(--border);
      color: var(--text);
      text-decoration: none;
    }
    .sidebar-link.active {
      background: var(--primary-soft);
      border-color: var(--primary-border);
      color: var(--primary);
      font-weight: 600;
    }
    .sidebar-link .meta {
      min-width: 0;
      overflow: hidden;
    }
    .sidebar-link .title {
      font-size: 13px;
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
    }
    .sidebar-link .desc {
      margin-top: 1px;
      color: var(--muted);
      font-size: 11px;
      line-height: 1.3;
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
    }
    .sidebar-link .count {
      flex-shrink: 0;
      min-width: 18px;
      height: 18px;
      border-radius: var(--radius-full);
      background: var(--border);
      color: var(--muted);
      display: inline-flex;
      align-items: center;
      justify-content: center;
      font-size: 11px;
      font-weight: 700;
      padding: 0 5px;
    }
    .sidebar-link .count.warn {
      background: var(--warn-soft);
      color: var(--warn);
      border: 1px solid var(--warn-border);
    }
    .sidebar-link.active .count {
      background: var(--primary);
      color: #ffffff;
    }

    .sidebar-footer {
      margin-top: 14px;
      padding-top: 12px;
      border-top: 1px solid var(--border);
      display: flex;
      flex-direction: column;
      gap: 8px;
    }
    .sidebar-action-btn {
      width: 100%;
      display: flex;
      align-items: center;
      justify-content: center;
      gap: 6px;
      padding: 7px 10px;
      border-radius: var(--radius-sm);
      border: 1px solid var(--border);
      background: var(--panel-soft);
      color: var(--muted);
      font-size: 11px;
      font-weight: 600;
      cursor: pointer;
      transition: all 0.16s ease;
    }
    .sidebar-action-btn:hover {
      background: var(--panel-hover);
      color: var(--text);
      border-color: var(--border-strong);
    }
    .report-main {
      min-width: 0;
      min-height: calc(100vh - 40px);
    }
    .tab-page {
      display: none;
      min-width: 0;
      min-height: calc(100vh - 40px);
    }
    .tab-page.active {
      display: flex;
      flex-direction: column;
      gap: 18px;
    }
    .tab-page.active > .section:last-child,
    .tab-page.active > .hero:last-child {
      flex: 1 1 auto;
    }

    /* 头部决策横幅（首屏聚焦、拒绝花哨渐变与堆叠） / Executive Hero Header */
    .hero {
      background: var(--panel);
      border: 1px solid var(--border);
      border-radius: var(--radius-lg);
      padding: 22px 24px;
      box-shadow: 0 1px 3px rgba(15, 23, 42, 0.03);
    }
    .hero-grid {
      display: grid;
      grid-template-columns: minmax(0, 1.35fr) minmax(320px, 0.65fr);
      gap: 20px;
      align-items: start;
    }
    .hero-badges {
      display: flex;
      flex-wrap: wrap;
      gap: 6px;
      margin-bottom: 12px;
      align-items: center;
    }
    .hero-kicker {
      color: var(--primary);
      font-size: 11px;
      font-weight: 700;
      letter-spacing: 0.08em;
      text-transform: uppercase;
      margin-bottom: 6px;
    }
    .hero-title {
      font-size: 26px;
      font-weight: 700;
      line-height: 1.25;
      margin-bottom: 8px;
      color: var(--text);
    }
    .hero-summary {
      color: var(--text-sub);
      max-width: 840px;
      font-size: 14px;
      line-height: 1.6;
    }
    .hero-side {
      border: 1px solid var(--border);
      border-radius: var(--radius-md);
      background: var(--panel-soft);
      padding: 16px;
    }
    .hero-side.tone-healthy {
      background: var(--ok-soft);
      border-color: var(--ok-border);
    }
    .hero-side.tone-warning {
      background: var(--warn-soft);
      border-color: var(--warn-border);
    }
    .hero-side.tone-critical {
      background: var(--critical-soft);
      border-color: var(--critical-border);
    }
    .hero-side.tone-neutral {
      background: var(--neutral-soft);
      border-color: var(--neutral-border);
    }
    .side-label,
    .focus-label,
    .panel-label,
    .subsection-label {
      color: var(--muted);
      font-size: 11px;
      font-weight: 700;
      letter-spacing: 0.05em;
      text-transform: uppercase;
      margin-bottom: 8px;
    }

    /* 状态徽标与原子组件（绝对防破碎） / Badges with atomic protection */
    .badge {
      display: inline-flex;
      align-items: center;
      gap: 5px;
      padding: 3px 9px;
      border-radius: var(--radius-full);
      border: 1px solid var(--border);
      font-size: 12px;
      font-weight: 600;
      background: var(--panel);
      color: var(--text);
      white-space: nowrap;
      flex-shrink: 0;
      line-height: 1.4;
    }
    .status-ok { background: var(--ok-soft); border-color: var(--ok-border); color: var(--ok); }
    .status-warn { background: var(--warn-soft); border-color: var(--warn-border); color: var(--warn); }
    .status-critical { background: var(--critical-soft); border-color: var(--critical-border); color: var(--critical); }
    .status-neutral { background: var(--neutral-soft); border-color: var(--neutral-border); color: var(--neutral); }
    .status-skip { background: var(--skip-soft); border-color: var(--skip-border); color: var(--skip); }

    /* 指标卡片网格与数值单行防护 / Metric & Stat Cards single-line integrity */
    .metric-grid,
    .stat-grid {
      display: grid;
      grid-template-columns: repeat(2, minmax(0, 1fr));
      gap: 10px;
    }
    .metric-card,
    .stat-card {
      border: 1px solid var(--border);
      border-radius: var(--radius-md);
      background: var(--panel);
      padding: 12px 14px;
      min-height: 76px;
      display: flex;
      flex-direction: column;
      justify-content: space-between;
    }
    .metric-card .label,
    .stat-card .label {
      color: var(--muted);
      font-size: 11px;
      font-weight: 600;
      text-transform: uppercase;
      letter-spacing: 0.04em;
      margin-bottom: 4px;
      white-space: nowrap;
    }
    .metric-card .value,
    .stat-card .value {
      font-size: 18px;
      font-weight: 700;
      line-height: 1.2;
      color: var(--text);
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
    }
    .metric-card .note,
    .stat-card .note {
      color: var(--muted);
      font-size: 11px;
      margin-top: 4px;
      line-height: 1.4;
    }

    /* 页面分段卡片 / Sections & Panels */
    .section {
      background: var(--panel);
      border: 1px solid var(--border);
      border-radius: var(--radius-lg);
      padding: 20px 24px;
      box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
    }
    .section-heading {
      display: flex;
      align-items: flex-start;
      justify-content: space-between;
      gap: 16px;
      margin-bottom: 16px;
    }
    .section-lead {
      margin-top: 4px;
      color: var(--muted);
      font-size: 13px;
    }
    .grid-2 {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(360px, 1fr));
      gap: 16px;
    }
    .focus-grid {
      display: grid;
      grid-template-columns: minmax(0, 1.1fr) minmax(0, 0.9fr) minmax(0, 1fr);
      gap: 14px;
    }
    .focus-panel,
    .detail-panel {
      border: 1px solid var(--border);
      border-radius: var(--radius-md);
      background: var(--panel);
      padding: 16px;
    }
    .focus-panel.tone-healthy {
      background: var(--ok-soft);
      border-color: var(--ok-border);
    }
    .focus-panel.tone-warning {
      background: var(--warn-soft);
      border-color: var(--warn-border);
    }
    .focus-panel.tone-critical {
      background: var(--critical-soft);
      border-color: var(--critical-border);
    }
    .focus-panel.tone-neutral {
      background: var(--neutral-soft);
      border-color: var(--neutral-border);
    }
    .focus-panel h3,
    .detail-title {
      font-size: 18px;
      line-height: 1.35;
      margin-bottom: 8px;
      color: var(--text);
    }
    .focus-panel p,
    .detail-panel p {
      color: var(--muted);
    }
    .panel-note {
      margin-top: 10px;
      color: var(--muted);
      font-size: 12px;
    }
    .detail-columns {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(380px, 1fr));
      gap: 16px;
    }

    /* 属性键值列表（原子防破碎、标签对齐） / Description Lists */
    .dl {
      display: flex;
      flex-direction: column;
      gap: 0;
    }
    .dl-row {
      display: grid;
      grid-template-columns: 160px minmax(0, 1fr);
      gap: 14px;
      padding: 8px 0;
      border-bottom: 1px solid var(--border);
      align-items: baseline;
    }
    .dl-row:last-child { border-bottom: none; }
    .dl-term {
      color: var(--muted);
      font-size: 13px;
      font-weight: 500;
      white-space: nowrap;
    }
    .dl-value {
      font-size: 13px;
      color: var(--text);
      word-break: normal;
      overflow-wrap: break-word;
    }
    .subsection + .subsection {
      margin-top: 16px;
      padding-top: 16px;
      border-top: 1px solid var(--border);
    }
    .list {
      display: flex;
      flex-direction: column;
      gap: 10px;
    }
    .entry {
      padding: 12px 0;
      border-bottom: 1px solid var(--border);
    }
    .entry:first-child { padding-top: 0; }
    .entry:last-child {
      border-bottom: none;
      padding-bottom: 0;
    }
    .entry-header {
      display: flex;
      align-items: flex-start;
      justify-content: space-between;
      gap: 12px;
      flex-wrap: wrap;
      margin-bottom: 6px;
    }
    .entry-title {
      font-weight: 600;
      font-size: 14px;
      line-height: 1.4;
      color: var(--text);
    }
    .muted {
      color: var(--muted);
    }
    .muted.wrap-text,
    .wrap-text {
      white-space: normal;
      overflow-wrap: anywhere;
      word-break: break-word;
    }
    .small { font-size: 12px; }
    .callout {
      margin-top: 12px;
      padding: 12px 14px;
      border-radius: var(--radius-sm);
      background: var(--panel-soft);
      border: 1px solid var(--border);
      font-size: 13px;
      color: var(--text);
    }
    .callout.critical {
      background: var(--critical-soft);
      border-color: var(--critical-border);
    }
    .callout.warn {
      background: var(--warn-soft);
      border-color: var(--warn-border);
    }
    .list-clean {
      margin: 0;
      padding-left: 18px;
      display: flex;
      flex-direction: column;
      gap: 8px;
    }
    .list-clean li { color: var(--text); }

    /*
     * 表格排版防破碎与横向溢出保护（遵循 UI 规范 12.4 策略 B）
     * Anti-forced-wrapping table layouts with horizontal scroll protection.
     */
    .table-wrap {
      border: 1px solid var(--border);
      border-radius: var(--radius-md);
      overflow-x: auto;
      overflow-y: hidden;
      margin-top: 12px;
      background: var(--panel);
      -webkit-overflow-scrolling: touch;
    }
    table {
      width: 100%;
      border-collapse: collapse;
      font-size: 13px;
      line-height: 1.5;
    }
    th {
      background: var(--panel-soft);
      color: var(--muted);
      font-weight: 600;
      font-size: 12px;
      letter-spacing: 0.02em;
      padding: 10px 14px;
      text-align: left;
      border-bottom: 1px solid var(--border);
      white-space: nowrap;
      user-select: none;
    }
    td {
      padding: 10px 14px;
      text-align: left;
      border-bottom: 1px solid var(--border);
      vertical-align: top;
      color: var(--text);
    }
    tr:last-child td { border-bottom: none; }
    tbody tr:hover { background: var(--panel-hover); }

    /* 语义化数据列防断裂规则 / Semantic column atomic integrity classes */
    .col-time,
    .col-host,
    .col-role,
    .col-status,
    .col-id,
    .col-size,
    .col-hash,
    .col-seq,
    .col-type,
    .col-scope,
    .col-version,
    .col-node,
    .col-name,
    .col-proc,
    .col-step,
    .cell-nowrap {
      white-space: nowrap;
      word-break: keep-all;
    }
    .col-desc,
    .cell-wrap {
      white-space: normal;
      overflow-wrap: break-word;
      word-break: normal;
      min-width: 240px;
    }
    .col-file {
      white-space: nowrap;
      word-break: keep-all;
    }

    /* 进程事件与指标快照特定尺寸微调 / Process & Metric tables layout tuning */
    .process-events-table,
    .metric-signals-table {
      width: max-content;
      min-width: 100%;
    }
    .process-events-table th:first-child,
    .process-events-table td:first-child {
      width: 188px;
      min-width: 188px;
      white-space: nowrap;
    }
    .process-events-table th:nth-child(2),
    .process-events-table td:nth-child(2) {
      width: 156px;
      min-width: 156px;
      white-space: nowrap;
    }
    .process-events-table th:nth-child(3),
    .process-events-table td:nth-child(3) {
      width: 140px;
      min-width: 140px;
      white-space: nowrap;
    }
    .process-events-table th:nth-child(4),
    .process-events-table td:nth-child(4) {
      width: 180px;
      min-width: 180px;
      white-space: nowrap;
    }
    .process-events-table th:nth-child(5),
    .process-events-table td:nth-child(5) {
      min-width: 420px;
      white-space: normal;
      overflow-wrap: anywhere;
      word-break: break-word;
    }
    .metric-signals-table th:first-child,
    .metric-signals-table td:first-child {
      width: 132px;
      min-width: 132px;
      white-space: nowrap;
    }
    .metric-signals-table th:nth-child(2),
    .metric-signals-table td:nth-child(2) {
      width: 392px;
      min-width: 392px;
    }
    .metric-signals-table th:nth-child(3),
    .metric-signals-table td:nth-child(3) {
      width: 132px;
      min-width: 132px;
      white-space: nowrap;
    }
    .metric-signals-table th:nth-child(4),
    .metric-signals-table td:nth-child(4) {
      width: 84px;
      min-width: 84px;
      white-space: nowrap;
    }
    .metric-signals-table th:nth-child(5),
    .metric-signals-table td:nth-child(5) {
      width: 88px;
      min-width: 88px;
      white-space: nowrap;
    }

    .artifact-group + .artifact-group {
      margin-top: 20px;
      padding-top: 20px;
      border-top: 1px solid var(--border);
    }
    .artifact-grid {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(340px, 1fr));
      gap: 14px;
    }
    .artifact-card {
      border: 1px solid var(--border);
      border-radius: var(--radius-md);
      background: var(--panel);
      padding: 16px;
      display: flex;
      flex-direction: column;
      gap: 12px;
    }
    .artifact-meta {
      display: grid;
      grid-template-columns: repeat(2, minmax(0, 1fr));
      gap: 10px;
    }
    .artifact-meta .meta-item {
      border: 1px solid var(--border);
      border-radius: var(--radius-sm);
      padding: 8px 10px;
      background: var(--panel-soft);
    }
    .meta-item .label {
      color: var(--muted);
      font-size: 11px;
      margin-bottom: 4px;
    }
    .meta-item .value {
      font-size: 13px;
      color: var(--text);
      word-break: normal;
      overflow-wrap: break-word;
    }

    /* 行内代码与路径保护（整行展示、杜绝字中折行） / Inline Code & Path Integrity */
    code.inline {
      font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", monospace;
      background: var(--code-inline-bg);
      color: var(--code-inline-text);
      border: 1px solid var(--code-inline-border);
      padding: 1.5px 6px;
      border-radius: 6px;
      font-size: 12px;
      white-space: nowrap;
      word-break: keep-all;
      display: inline-block;
      vertical-align: baseline;
    }
    pre {
      margin: 0;
      background: var(--code-bg);
      color: var(--code-text);
      padding: 14px 16px;
      border-radius: var(--radius-sm);
      overflow: auto;
      line-height: 1.55;
      font-size: 12px;
      max-height: 420px;
      white-space: pre-wrap;
      word-break: break-word;
      font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
    }
    /* 配置文件页签允许横向滚动，避免较长文件名挤压内容。 / Config tabs scroll horizontally so long file names do not squeeze content. */
    .config-file-tabs {
      overflow-x: auto;
      flex-wrap: nowrap;
      max-width: 100%;
      padding-bottom: 4px;
    }
    .config-file-tabs .inner-tab-btn {
      flex: 0 0 auto;
      max-width: 260px;
      overflow: hidden;
      text-overflow: ellipsis;
    }
    .config-file-tabs .inner-tab-btn:focus-visible {
      outline: 2px solid var(--primary);
      outline-offset: 2px;
    }
    .config-preview-details {
      margin-top: 12px;
      border-top: 1px solid var(--border);
      padding-top: 10px;
    }
    .config-preview-summary {
      cursor: pointer;
      font-size: 13px;
      font-weight: 500;
      color: var(--primary);
      display: inline-flex;
      align-items: center;
      gap: 6px;
      user-select: none;
      outline: none;
    }
    .config-preview-summary:hover {
      text-decoration: underline;
    }
    .config-preview-summary::-webkit-details-marker {
      display: none;
    }
    .config-preview-summary::before {
      content: '▶';
      font-size: 10px;
      transition: transform 0.15s ease;
      display: inline-block;
    }
    .config-preview-details[open] > .config-preview-summary::before {
      transform: rotate(90deg);
    }
    .config-preview-details[open] > .config-preview-summary {
      margin-bottom: 8px;
    }
    .copyable-block {
      margin-top: 10px;
    }
    .copyable-actions {
      display: flex;
      flex-wrap: wrap;
      gap: 8px;
      justify-content: flex-end;
      margin-bottom: 8px;
    }
    .copy-btn,
    .log-action-btn,
    .modal-close-btn {
      border: 1px solid var(--primary-border);
      background: var(--primary-soft);
      color: var(--primary);
      border-radius: var(--radius-sm);
      padding: 6px 12px;
      font-size: 12px;
      font-weight: 600;
      cursor: pointer;
      transition: all 0.16s ease;
      text-decoration: none;
      display: inline-flex;
      align-items: center;
      justify-content: center;
      white-space: nowrap;
    }
    .copy-btn:hover,
    .log-action-btn:hover,
    .modal-close-btn:hover {
      background: var(--panel-hover);
      text-decoration: none;
    }
    .copy-btn.copied {
      background: var(--ok-soft);
      border-color: var(--ok-border);
      color: var(--ok);
    }
    .copy-btn.failed {
      background: var(--critical-soft);
      border-color: var(--critical-border);
      color: var(--critical);
    }
    .copy-btn .label-copied,
    .copy-btn .label-failed {
      display: none;
    }
    .copy-btn.copied .label-copy,
    .copy-btn.failed .label-copy {
      display: none;
    }
    .copy-btn.copied .label-copied {
      display: inline;
    }
    .copy-btn.failed .label-failed {
      display: inline;
    }
    .log-preview-note {
      margin-top: 8px;
    }
    .full-log-modal {
      position: fixed;
      inset: 0;
      background: rgba(15, 23, 42, 0.56);
      backdrop-filter: blur(4px);
      display: none;
      align-items: center;
      justify-content: center;
      padding: 24px;
      z-index: 1000;
    }
    .full-log-modal.active {
      display: flex;
    }
    .full-log-dialog {
      width: min(1120px, 100%);
      height: min(82vh, 920px);
      background: var(--panel);
      border-radius: var(--radius-lg);
      box-shadow: 0 24px 60px rgba(0, 0, 0, 0.35);
      border: 1px solid var(--border);
      display: flex;
      flex-direction: column;
      overflow: hidden;
    }
    .full-log-header {
      padding: 16px 20px;
      border-bottom: 1px solid var(--border);
      background: var(--panel-soft);
      display: flex;
      gap: 12px;
      align-items: center;
      justify-content: space-between;
    }
    .full-log-header-actions {
      display: flex;
      flex-wrap: wrap;
      gap: 8px;
      align-items: center;
      justify-content: flex-end;
    }
    .full-log-body {
      position: relative;
      flex: 1;
      min-height: 0;
      background: var(--code-bg);
    }
    .full-log-loading {
      position: absolute;
      inset: 0;
      display: none;
      align-items: center;
      justify-content: center;
      text-align: center;
      padding: 24px;
      background: rgba(15, 23, 42, 0.82);
      color: #e2e8f0;
      font-size: 14px;
      z-index: 1;
    }
    .full-log-loading.active {
      display: flex;
    }
    .full-log-frame {
      width: 100%;
      height: 100%;
      border: none;
      background: var(--panel);
    }
    details {
      border: 1px dashed var(--border);
      border-radius: var(--radius-sm);
      padding: 12px 14px;
      background: var(--panel-soft);
    }
    details summary {
      cursor: pointer;
      font-weight: 600;
      color: var(--text);
    }
    .metric-chart {
      width: 100%;
      min-width: 320px;
      height: 156px;
      display: block;
      background: var(--panel-soft);
      border: 1px solid var(--border);
      border-radius: var(--radius-sm);
    }
    .metric-chart-axis { stroke: var(--border-strong); stroke-width: 1.2; }
    .metric-chart-grid { stroke: var(--border); stroke-width: 1; stroke-dasharray: 3 4; }
    .metric-chart-path { fill: none; stroke: var(--primary); stroke-width: 2.4; stroke-linecap: round; stroke-linejoin: round; }
    .metric-chart-threshold { stroke: var(--warn); stroke-width: 1.2; stroke-dasharray: 4 3; }
    .metric-chart-dot { fill: var(--text); }
    .metric-chart-label { fill: var(--muted); font-size: 10px; }
    .metric-chart-label.x-mid { text-anchor: middle; }
    .metric-chart-label.x-end,
    .metric-chart-label.threshold-label { text-anchor: end; }

    .summary-grid,
    .finding-grid,
    .category-grid,
    .signal-card-grid {
      display: grid;
      gap: 14px;
    }
    .summary-grid,
    .category-grid {
      grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
    }
    .finding-grid,
    .signal-card-grid {
      grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
    }
    .finding-card,
    .category-card,
    .signal-card {
      border: 1px solid var(--border);
      border-radius: var(--radius-md);
      padding: 16px;
      background: var(--panel);
    }
    .finding-meta,
    .category-meta {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 12px;
      margin-bottom: 10px;
    }
    .category-count {
      font-size: 28px;
      line-height: 1;
      font-weight: 700;
      color: var(--text);
      font-variant-numeric: tabular-nums;
    }
    .timeline-list {
      display: flex;
      flex-direction: column;
      gap: 12px;
    }
    .timeline-item {
      display: grid;
      grid-template-columns: 160px 16px minmax(0, 1fr);
      gap: 12px;
      align-items: start;
    }
    .timeline-time {
      color: var(--muted);
      font-size: 12px;
      font-weight: 600;
      white-space: nowrap;
      padding-top: 2px;
      font-variant-numeric: tabular-nums;
    }
    .timeline-dot-wrap {
      display: flex;
      justify-content: center;
    }
    .timeline-dot {
      margin-top: 6px;
      width: 10px;
      height: 10px;
      border-radius: var(--radius-full);
      background: var(--muted);
    }
    .timeline-dot.tone-critical { background: var(--critical); }
    .timeline-dot.tone-warning { background: var(--warn); }
    .timeline-dot.tone-healthy { background: var(--ok); }
    .timeline-content {
      min-width: 0;
      padding-bottom: 12px;
      border-bottom: 1px dashed var(--border);
      overflow-wrap: break-word;
      word-break: normal;
    }
    .timeline-content:last-child {
      border-bottom: none;
      padding-bottom: 0;
    }
    .timeline-content .entry-title,
    .timeline-content .muted {
      overflow-wrap: break-word;
      word-break: normal;
    }
    .signal-head {
      display: flex;
      justify-content: space-between;
      gap: 12px;
      align-items: flex-start;
      margin-bottom: 12px;
    }
    .signal-stats {
      display: grid;
      grid-template-columns: repeat(3, minmax(0, 1fr));
      gap: 8px;
      margin-bottom: 12px;
    }
    .signal-stat {
      border-radius: var(--radius-sm);
      background: var(--panel-soft);
      border: 1px solid var(--border);
      padding: 8px 10px;
    }
    .signal-stat .label {
      font-size: 11px;
      color: var(--muted);
      margin-bottom: 3px;
      white-space: nowrap;
    }
    .signal-stat .value {
      font-size: 14px;
      font-weight: 600;
      color: var(--text);
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
    }

    /* 下钻穿透卡片（概览快速穿透） / Quick Drill-down Cards */
    .overview-drill-grid {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
      gap: 14px;
    }
    .drill-card {
      border: 1px solid var(--border);
      border-radius: var(--radius-md);
      background: var(--panel);
      padding: 16px;
      text-decoration: none;
      color: inherit;
      display: block;
      transition: transform 0.16s ease, box-shadow 0.16s ease, border-color 0.16s ease;
    }
    .drill-card:hover {
      transform: translateY(-2px);
      border-color: var(--primary);
      box-shadow: 0 10px 24px rgba(0, 0, 0, 0.08);
      text-decoration: none;
    }
    .drill-card .title {
      font-size: 15px;
      font-weight: 700;
      color: var(--text);
      white-space: nowrap;
    }
    .drill-card .count {
      margin-top: 8px;
      font-size: 26px;
      font-weight: 700;
      color: var(--primary);
      line-height: 1;
      font-variant-numeric: tabular-nums;
    }
    .drill-card .desc {
      margin-top: 8px;
      font-size: 12px;
      color: var(--muted);
      line-height: 1.5;
    }

    /* 二级内部 Tab 标签栏 / Inner Tab Toolbars */
    .inner-tab-toolbar {
      display: flex;
      flex-wrap: wrap;
      gap: 8px;
      margin: 14px 0 18px;
    }
    .inner-tab-btn {
      border: 1px solid var(--border);
      background: var(--panel-soft);
      color: var(--text-sub);
      border-radius: var(--radius-full);
      padding: 7px 14px;
      font-size: 12px;
      font-weight: 600;
      cursor: pointer;
      transition: all 0.16s ease;
      white-space: nowrap;
    }
    .inner-tab-btn:hover {
      background: var(--panel-hover);
      color: var(--text);
    }
    .inner-tab-btn.active {
      background: var(--primary);
      border-color: var(--primary);
      color: #ffffff;
      box-shadow: 0 4px 12px rgba(37,99,235,0.25);
    }
    .inner-tab-panel {
      display: none;
      min-width: 0;
    }
    .inner-tab-panel.active {
      display: block;
    }
    .empty {
      border: 1px dashed var(--border);
      border-radius: var(--radius-sm);
      padding: 16px;
      color: var(--muted);
      background: var(--panel-soft);
      font-size: 13px;
    }

    /* 附录已确认正常项检查网格 / Appendix Passed Check Items Grid */
    .passed-checklist-grid {
      display: grid;
      grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
      gap: 10px;
      margin-top: 10px;
    }
    .passed-check-item {
      display: flex;
      align-items: flex-start;
      gap: 8px;
      padding: 10px 12px;
      border-radius: var(--radius-sm);
      border: 1px solid var(--ok-border);
      background: var(--ok-soft);
      min-width: 0;
    }
    .passed-check-item .check-icon {
      color: var(--ok);
      font-weight: 700;
      line-height: 1;
      font-size: 14px;
      flex-shrink: 0;
      margin-top: 2px;
    }
    .passed-check-item .check-text {
      min-width: 0;
    }
    .passed-check-item .check-title {
      font-size: 13px;
      font-weight: 600;
      color: var(--text);
      line-height: 1.3;
    }
    .passed-check-item .check-desc {
      font-size: 11px;
      color: var(--muted);
      line-height: 1.4;
      margin-top: 2px;
    }

    /* 采集备注与诊断警告卡片 / Collection Notes & Warning Callouts */
    .collection-notes-card {
      border: 1px solid var(--warn-border);
      background: var(--warn-soft);
      border-radius: var(--radius-md);
      padding: 14px 16px;
      margin-bottom: 16px;
    }
    .collection-notes-header {
      display: flex;
      align-items: center;
      gap: 8px;
      font-weight: 700;
      font-size: 14px;
      color: var(--warn);
      margin-bottom: 8px;
    }
    .collection-notes-hint {
      font-size: 12px;
      line-height: 1.5;
      color: var(--text-sub);
      margin-bottom: 10px;
    }
    .error-text {
      color: var(--critical);
      font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
      font-size: 12px;
    }

    /* 调试与内部路径折叠区 / Debug & Internal Path Collapsible Section */
    .debug-details {
      margin-top: 14px;
      border: 1px dashed var(--border);
      border-radius: var(--radius-sm);
      background: var(--panel-soft);
      padding: 10px 14px;
    }
    .debug-details summary {
      cursor: pointer;
      font-size: 12px;
      font-weight: 600;
      color: var(--muted);
    }
    .debug-details summary:hover {
      color: var(--text);
    }
    .debug-details .dl {
      margin-top: 10px;
    }

    @media (max-width: 1280px) {
      .report-shell {
        grid-template-columns: 1fr;
      }
      .report-sidebar {
        position: static;
        min-height: auto;
      }
      .report-main,
      .tab-page {
        min-height: auto;
      }
      .sidebar-nav {
        display: grid;
        grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
      }
    }
    @media (max-width: 1120px) {
      .hero-grid,
      .focus-grid {
        grid-template-columns: 1fr;
      }
    }
    @media (max-width: 900px) {
      body { padding: 14px; }
      .section, .hero { padding: 16px; }
      .dl-row {
        grid-template-columns: 1fr;
        gap: 4px;
      }
      .metric-grid,
      .stat-grid,
      .artifact-meta,
      .signal-stats {
        grid-template-columns: 1fr;
      }
      .timeline-item {
        grid-template-columns: 1fr;
      }
    }
</style>
</head>
<body>
  <div class="page report-shell">
    <aside class="report-sidebar">
      <div class="sidebar-brand">
        <div class="stx-brand-lockup">
          <img src="{{stxBrandMark}}" alt="STX" class="stx-brand-logo" />
          <div class="stx-brand-text">
            <span class="stx-brand-title">STX</span>
            <span class="stx-brand-subtitle">{{pair "诊断报告" "Diagnostic Report"}}</span>
          </div>
        </div>
      </div>
      <div class="sidebar-status-pill {{toneClass .Health.Tone}}">
        <span class="status-dot"></span>
        <span class="status-text">{{loc .Health.Tone}} · {{.Health.ClusterLabel}}</span>
      </div>
      <nav class="sidebar-nav">
        <a class="sidebar-link active" data-tab-link="tab-overview" href="#tab-overview">
          <div class="meta">
            <div class="title">{{pair "总览" "Overview"}}</div>
          </div>
        </a>
        <a class="sidebar-link" data-tab-link="tab-findings" href="#tab-findings">
          <div class="meta">
            <div class="title">{{pair "关键发现" "Findings"}}</div>
          </div>
          {{if .Findings}}<span class="count warn">{{len .Findings}}</span>{{end}}
        </a>
        <a class="sidebar-link" data-tab-link="tab-timeline" href="#tab-timeline">
          <div class="meta">
            <div class="title">{{pair "时间线" "Timeline"}}</div>
          </div>
          {{if .Timeline}}<span class="count">{{len .Timeline}}</span>{{end}}
        </a>
        <a class="sidebar-link" data-tab-link="tab-signals" href="#tab-signals">
          <div class="meta">
            <div class="title">{{pair "指标" "Signals"}}</div>
          </div>
          {{if .MetricsSnapshot}}{{if gt .MetricsSnapshot.SignalCount 0}}<span class="count">{{.MetricsSnapshot.SignalCount}}</span>{{end}}{{end}}
        </a>
        <a class="sidebar-link" data-tab-link="tab-evidence" href="#tab-evidence">
          <div class="meta">
            <div class="title">{{pair "相关证据" "Evidence"}}</div>
          </div>
          {{if .ErrorContext}}{{if gt .ErrorContext.RecentEventCount 0}}<span class="count">{{.ErrorContext.RecentEventCount}}</span>{{end}}{{end}}
        </a>
        <a class="sidebar-link" data-tab-link="tab-appendix" href="#tab-appendix">
          <div class="meta">
            <div class="title">{{pair "附录与节点" "Appendix & Nodes"}}</div>
          </div>
        </a>
      </nav>
      <div class="sidebar-footer">
        <button class="sidebar-action-btn" id="theme-toggle-btn" type="button" title="{{pair "切换浅色/暗色模式" "Toggle Light/Dark Theme"}}">
          <span class="theme-icon-light" style="display: none;">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="5"></circle><line x1="12" y1="1" x2="12" y2="3"></line><line x1="12" y1="21" x2="12" y2="23"></line><line x1="4.22" y1="4.22" x2="5.64" y2="5.64"></line><line x1="18.36" y1="18.36" x2="19.78" y2="19.78"></line><line x1="1" y1="12" x2="3" y2="12"></line><line x1="21" y1="12" x2="23" y2="12"></line><line x1="4.22" y1="19.78" x2="5.64" y2="18.36"></line><line x1="18.36" y1="5.64" x2="19.78" y2="4.22"></line></svg>
          </span>
          <span class="theme-icon-dark" style="display: none;">
            <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"></path></svg>
          </span>
          <span id="theme-toggle-label">{{pair "切换主题" "Switch Theme"}}</span>
        </button>
        <button class="sidebar-action-btn" type="button" onclick="window.print()">
          <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M6 9V2h12v7"></path><path d="M6 18H4a2 2 0 0 1-2-2v-5a2 2 0 0 1 2-2h16a2 2 0 0 1 2 2v5a2 2 0 0 1-2 2h-2"></path><rect x="6" y="14" width="12" height="8"></rect></svg>
          <span>{{pair "打印 / 导出 PDF" "Print / Export PDF"}}</span>
        </button>
      </div>
    </aside>

    <main class="report-main">
    <div class="tab-page active" id="tab-overview" data-tab-page="tab-overview">
    <header class="hero">
      <div class="hero-grid">
        <div class="hero-main">
          <div class="hero-kicker">STX · {{pair "诊断结论" "Diagnosis Overview"}}</div>
          <h1 class="hero-title">{{loc .Health.Title}}</h1>
          <div class="hero-badges">
            <span class="badge {{statusClass .Health.Tone}}">{{loc .Health.Tone}}</span>
            <span class="badge">{{.Health.ClusterLabel}}</span>
            <span class="badge">{{.Health.WindowLabel}}</span>
            <span class="badge" style="color: var(--muted); font-weight: 500;">{{pair "生成于" "Generated"}} {{formatTime .GeneratedAt}}</span>
          </div>
          <p class="hero-summary">{{loc .Health.Summary}}</p>
          <div class="summary-grid" style="margin-top: 18px;">
            <div class="stat-card">
              <div class="label">{{pair "影响范围" "Blast Radius / Impact"}}</div>
              <div class="value" style="font-size: 15px; font-weight: 600; line-height: 1.4;">{{loc .Health.ImpactSummary}}</div>
            </div>
            <div class="stat-card">
              <div class="label">{{pair "优先排查" "Priority Focus"}}</div>
              <div class="value" style="font-size: 15px; font-weight: 600; line-height: 1.4;">{{loc .Health.PrimaryFocus}}</div>
            </div>
          </div>
        </div>
        <aside class="hero-side {{toneClass .Health.Tone}}">
          <div class="side-label">{{pair "建议动作" "Suggested Actions"}}</div>
          {{if .Recommendations}}
          <div class="list">
            {{range .Recommendations}}
            <div class="entry" style="background: rgba(255,255,255,0.85); padding: 8px 10px; border-radius: var(--radius-sm); border: 1px solid rgba(0,0,0,0.04);">
              <div class="entry-title" style="font-size: 13px;">{{loc .Title}}</div>
              <div class="muted small" style="margin-top: 4px;">{{loc .Details}}</div>
            </div>
            {{end}}
          </div>
          {{else}}
          <div class="empty">{{pair "当前没有额外建议，可直接查看关键发现与时间线。" "No extra advice is available for now."}}</div>
          {{end}}
          {{if .Health.Metrics}}
          <div class="side-label" style="margin-top: 16px;">{{pair "核心指标" "Key Signals"}}</div>
          <div class="metric-grid">
            {{range .Health.Metrics}}
            <div class="metric-card">
              <div class="label">{{loc .Label}}</div>
              <div class="value">{{.Value}}</div>
              {{if .Note}}<div class="note">{{loc .Note}}</div>{{end}}
            </div>
            {{end}}
          </div>
          {{end}}
        </aside>
      </div>
    </header>

    <section class="section" id="focus">
      <div class="section-heading">
        <div>
          <h2>{{pair "诊断全景与下钻" "Overview & Drill-down"}}</h2>
          <div class="section-lead">{{pair "快速穿透到关键发现、时序时间线、重点指标与深层证据。" "Drill down into critical findings, timeline, prioritized signals, and runtime evidence."}}</div>
        </div>
      </div>
      <div class="inner-tab-toolbar">
        <button type="button" class="inner-tab-btn active" data-inner-tab-group="overview" data-inner-tab-key="next">{{pair "快速下钻" "Quick Drill-down"}}</button>
        <button type="button" class="inner-tab-btn" data-inner-tab-group="overview" data-inner-tab-key="summary">{{pair "核心快照" "Core Highlights"}}</button>
      </div>
      <div class="inner-tab-panel active" data-inner-tab-group="overview" data-inner-tab-key="next">
        <div class="overview-drill-grid">
          <a class="drill-card" href="#tab-findings">
            <div class="title">{{pair "关键发现" "Critical Findings"}}</div>
            <div class="count">{{len .Findings}}</div>
            <div class="desc">{{pair "查看最需要优先处理的问题和建议动作。" "Review the most urgent issues and next actions."}}</div>
          </a>
          <a class="drill-card" href="#tab-timeline">
            <div class="title">{{pair "时间线" "Timeline & Categories"}}</div>
            <div class="count">{{len .Timeline}}</div>
            <div class="desc">{{pair "按时间查看异常、事件和峰值的先后关系。" "See the order of anomalies, events and peaks over time."}}</div>
          </a>
          <a class="drill-card" href="#tab-signals">
            <div class="title">{{pair "重点指标" "Key Signals"}}</div>
            <div class="count">{{if .MetricsSnapshot}}{{.MetricsSnapshot.SignalCount}}{{else}}0{{end}}</div>
            <div class="desc">{{pair "查看关键指标和趋势变化。" "Inspect prioritized signals and their trends."}}</div>
          </a>
          <a class="drill-card" href="#tab-evidence">
            <div class="title">{{pair "相关证据" "Evidence"}}</div>
            <div class="count">{{if .ErrorContext}}{{.ErrorContext.RecentEventCount}}{{else}}0{{end}}</div>
            <div class="desc">{{pair "查看日志、配置和运行时上下文。" "Open logs, config and runtime context."}}</div>
          </a>
        </div>
      </div>
      <div class="inner-tab-panel" data-inner-tab-group="overview" data-inner-tab-key="summary">
        <div class="summary-grid">
          <article class="focus-panel">
            <div class="focus-label">{{pair "核心现象" "Core Signals"}}</div>
            <div class="entry-title" style="margin-bottom: 6px;">{{loc .Health.PrimaryFocus}}</div>
            {{if .ErrorContext}}<div class="panel-note"><strong>{{pair "主要错误组" "Top Error Group"}}</strong>: {{.ErrorContext.GroupTitle}} ({{.ErrorContext.OccurrenceCount}} {{pair "次出现" "events"}})</div>{{end}}
          </article>
          <article class="focus-panel">
            <div class="focus-label">{{pair "波及范围" "Blast Radius"}}</div>
            <div class="entry-title" style="margin-bottom: 6px;">{{loc .Health.ImpactSummary}}</div>
            <div class="panel-note"><strong>{{pair "观测窗口" "Window"}}</strong>: {{.Health.WindowLabel}}</div>
          </article>
          <article class="focus-panel">
            <div class="focus-label">{{pair "根因分类分布" "Root Cause Distribution"}}</div>
            {{if .Categories}}
            <div style="display: flex; gap: 6px; flex-wrap: wrap; margin-top: 6px;">
              {{range .Categories}}
              <span class="badge"><strong>{{loc .Label}}</strong>: {{.Count}}</span>
              {{end}}
            </div>
            {{else}}
            <div class="muted small" style="margin-top: 6px;">{{pair "暂无归类数据" "No categories classified"}}</div>
            {{end}}
          </article>
        </div>
      </div>
    </section>
    </div>

    <div class="tab-page" id="tab-findings" data-tab-page="tab-findings">
    <section class="section" id="findings">
      <div class="section-heading">
        <div>
          <h2>{{pair "关键发现" "Critical Findings"}}</h2>
        </div>
      </div>
      {{if .Findings}}
      <div class="finding-grid">
        {{range .Findings}}
        <article class="finding-card">
          <div class="finding-meta">
            <div>
              <div class="entry-title">{{loc .Title}}</div>
              <div class="muted small">{{loc .Category}}</div>
            </div>
            <span class="badge {{statusClass .Severity}}">{{loc .Severity}}</span>
          </div>
          <div>{{loc .Summary}}</div>
          <div class="muted small" style="margin-top: 10px;">{{loc .Impact}}</div>
          <div class="callout" style="margin-top: 12px;">{{loc .Action}}</div>
        </article>
        {{end}}
      </div>
      {{else}}
      <div class="empty">{{pair "当前诊断窗口内没有生成结构化关键发现，请继续查看相关证据确认是否存在偶发问题。" "No structured findings were generated in this window. Continue with the evidence view if needed."}}</div>
      {{end}}
    </section>
    </div>

    <div class="tab-page" id="tab-timeline" data-tab-page="tab-timeline">
    <section class="section" id="categories">
      <div class="section-heading">
        <div>
          <h2>{{pair "根因分类" "Root Cause Categories"}}</h2>
        </div>
      </div>
      <div class="grid-2">
        <div class="category-grid">
          {{range .Categories}}
          <article class="category-card">
            <div class="category-meta">
              <div class="entry-title">{{loc .Label}}</div>
              <div class="category-count">{{.Count}}</div>
            </div>
            <div class="muted small">{{loc .Note}}</div>
          </article>
          {{end}}
        </div>
        <div class="detail-panel">
          <div class="panel-label">{{pair "时间线" "Timeline"}}</div>
          {{if .Timeline}}
          <div class="timeline-list">
            {{range .Timeline}}
            <div class="timeline-item">
              <div class="timeline-time">{{.TimeLabel}}</div>
              <div class="timeline-dot-wrap"><span class="timeline-dot {{toneClass .Tone}}"></span></div>
              <div class="timeline-content">
                <div class="entry-title">{{loc .Title}}</div>
                <div class="muted small" style="margin-top: 4px;">{{loc .Details}}</div>
              </div>
            </div>
            {{end}}
          </div>
          {{else}}
          <div class="empty">{{pair "当前没有可展示的时间线事件。" "No timeline events are available for this report."}}</div>
          {{end}}
        </div>
      </div>
    </section>
    </div>

    <div class="tab-page" id="tab-signals" data-tab-page="tab-signals">
    <section class="section" id="signals">
      <div class="section-heading">
        <div>
          <h2>{{pair "重点指标" "Key Signals"}}</h2>
        </div>
      </div>
      {{if .KeySignals}}
      <div class="signal-card-grid">
        {{range .KeySignals}}
        <article class="signal-card">
          <div class="signal-head">
            <div>
              <div class="entry-title">{{loc .Title}}</div>
              <div class="muted small">{{loc .ThresholdText}}</div>
            </div>
            <span class="badge {{statusClass .Status}}">{{loc .Status}}</span>
          </div>
          <div class="signal-stats">
            <div class="signal-stat"><div class="label">{{pair "实例" "Instance"}}</div><div class="value">{{.Instance}}</div></div>
            <div class="signal-stat"><div class="label">{{pair "峰值" "Peak"}}</div><div class="value">{{.PeakValue}}</div></div>
            <div class="signal-stat"><div class="label">{{pair "最新值" "Last"}}</div><div class="value">{{.LastValue}}</div></div>
          </div>
          {{metricChartSVG .Points .Threshold .Comparator .Unit}}
          <div class="muted small" style="margin-top: 12px;">{{pair "峰值时间" "Peak At"}} {{.PeakAt}}</div>
          <div style="margin-top: 10px;">{{loc .Interpretation}}</div>
        </article>
        {{end}}
      </div>
      {{else}}
      <div class="empty">{{pair "未采集到关键指标信号。" "No prioritized signals are available for this report."}}</div>
      {{end}}
    </section>
    </div>

    <div class="tab-page" id="tab-evidence" data-tab-page="tab-evidence">
    <section class="section" id="evidence">
      <div class="section-heading">
        <div>
          <h2>{{pair "相关证据" "Evidence"}}</h2>
        </div>
      </div>
      <div class="inner-tab-toolbar">
        <button type="button" class="inner-tab-btn active" data-inner-tab-group="evidence" data-inner-tab-key="error">{{pair "错误" "Errors"}}</button>
        <button type="button" class="inner-tab-btn" data-inner-tab-group="evidence" data-inner-tab-key="inspection">{{pair "巡检" "Inspection"}}</button>
        <button type="button" class="inner-tab-btn" data-inner-tab-group="evidence" data-inner-tab-key="config">{{pair "配置" "Config"}}</button>
        <button type="button" class="inner-tab-btn" data-inner-tab-group="evidence" data-inner-tab-key="metrics">{{pair "指标" "Signals"}}</button>
        <button type="button" class="inner-tab-btn" data-inner-tab-group="evidence" data-inner-tab-key="threaddump">{{pair "线程栈" "Thread Dumps"}}</button>
        <button type="button" class="inner-tab-btn" data-inner-tab-group="evidence" data-inner-tab-key="alerts">{{pair "告警" "Alerts"}}</button>
        <button type="button" class="inner-tab-btn" data-inner-tab-group="evidence" data-inner-tab-key="process">{{pair "进程" "Process"}}</button>
      </div>

      <div class="inner-tab-panel active" data-inner-tab-group="evidence" data-inner-tab-key="error">
        <div class="detail-panel">
          <div class="panel-label">{{pair "错误上下文" "Error Context"}}</div>
          {{if .ErrorContext}}
          <div class="dl">
            <div class="dl-row"><div class="dl-term">{{pair "错误组" "Error Group"}}</div><div class="dl-value">{{.ErrorContext.GroupTitle}}</div></div>
            <div class="dl-row"><div class="dl-term">{{pair "异常类型" "Exception"}}</div><div class="dl-value">{{.ErrorContext.ExceptionClass}}</div></div>
            <div class="dl-row"><div class="dl-term">{{pair "累计次数" "Occurrences"}}</div><div class="dl-value">{{.ErrorContext.OccurrenceCount}}</div></div>
            <div class="dl-row"><div class="dl-term">{{pair "最近事件数" "Recent Events"}}</div><div class="dl-value">{{.ErrorContext.RecentEventCount}}</div></div>
            <div class="dl-row"><div class="dl-term">{{pair "首次出现" "First Seen"}}</div><div class="dl-value">{{formatTime .ErrorContext.FirstSeenAt}}</div></div>
            <div class="dl-row"><div class="dl-term">{{pair "最近出现" "Last Seen"}}</div><div class="dl-value">{{formatTime .ErrorContext.LastSeenAt}}</div></div>
          </div>
          <div class="callout critical">{{.ErrorContext.SampleMessage}}</div>
          {{if .ErrorContext.Events}}
          <div class="table-wrap">
            <table>
              <thead>
                <tr>
                  <th class="col-time">{{pair "发生时间" "Occurred At"}}</th>
                  <th class="col-host">{{pair "主机" "Host"}}</th>
                  <th class="col-role">{{pair "角色" "Role"}}</th>
                  <th class="col-id">{{pair "作业 ID" "Job ID"}}</th>
                  <th class="col-file">{{pair "来源文件" "Source File"}}</th>
                </tr>
              </thead>
              <tbody>
                {{range .ErrorContext.Events}}
                <tr>
                  <td class="col-time">{{.OccurredAt}}</td>
                  <td class="col-host">{{.HostLabel}}</td>
                  <td class="col-role">{{.Role}}</td>
                  <td class="col-id">{{.JobID}}</td>
                  <td class="col-file"><code class="inline">{{.SourceFile}}</code></td>
                </tr>
                <tr>
                  <td colspan="5" class="cell-wrap">
                    <div>{{.Message}}</div>
                    {{if .Evidence}}<div class="muted small wrap-text" style="margin-top: 6px;">{{.Evidence}}</div>{{end}}
                  </td>
                </tr>
                {{end}}
              </tbody>
            </table>
          </div>
          {{end}}
          {{if .ErrorContext.LogSamples}}
          <div class="subsection">
            <div class="subsection-label">{{pair "原始错误日志" "Raw Error Logs"}}</div>
            <div class="list">
              {{range .ErrorContext.LogSamples}}
              <div class="entry">
                <div class="entry-header">
                  <div>
                    <div class="entry-title">{{.HostLabel}}</div>
                    <div class="muted small"><code class="inline">{{.SourceFile}}</code></div>
                  </div>
                  <span class="badge">{{.WindowLabel}}</span>
                </div>
                <div class="copyable-block">
                  <div class="copyable-actions">
                    <button type="button" class="copy-btn" data-copy-button>
                      <span class="label-copy">{{pair "复制预览" "Copy Preview"}}</span>
                      <span class="label-copied">{{pair "已复制" "Copied"}}</span>
                      <span class="label-failed">{{pair "复制失败" "Copy failed"}}</span>
                    </button>
                    {{if .FullLogRelativePath}}
                    <button
                      type="button"
                      class="log-action-btn"
                      data-full-log-button
                      data-log-relative-path="{{.FullLogRelativePath}}"
                      data-log-preview-url="{{.FullLogPreviewURL}}"
                      data-log-title="{{.HostLabel}} · {{.SourceFile}}"
                    >
                      {{pair "查看完整日志" "View Full Log"}}
                    </button>
                    <a
                      class="log-action-btn"
                      data-full-log-link
                      data-log-relative-path="{{.FullLogRelativePath}}"
                      data-log-preview-url="{{.FullLogPreviewURL}}"
                      href="{{.FullLogRelativePath}}"
                      target="_blank"
                      rel="noopener noreferrer"
                    >
                      {{pair "新窗口打开" "Open in New Window"}}
                    </a>
                    {{end}}
                  </div>
                  {{if .PreviewTruncated}}
                  <div class="muted small log-preview-note">
                    {{pair "当前仅展示前" "Showing first"}} {{.PreviewLineCount}} {{pair "行预览，完整日志请使用上方按钮查看。" "lines only. Use the actions above to open the full log."}}
                  </div>
                  {{end}}
                  <pre>{{.PreviewContent}}</pre>
                </div>
              </div>
              {{end}}
            </div>
          </div>
          {{end}}
          {{else}}
          <div class="empty">{{pair "当前诊断报告未附带错误组上下文。" "No error-group context is attached to this report."}}</div>
          {{end}}
        </div>
      </div>

      <div class="inner-tab-panel" data-inner-tab-group="evidence" data-inner-tab-key="inspection">
        <div class="detail-panel">
          <div class="panel-label">{{pair "巡检上下文" "Inspection Context"}}</div>
          {{if .Inspection}}
          <div class="dl">
            <div class="dl-row"><div class="dl-term">{{pair "摘要" "Summary"}}</div><div class="dl-value">{{loc .Inspection.Summary}}</div></div>
            <div class="dl-row"><div class="dl-term">{{pair "状态" "Status"}}</div><div class="dl-value"><span class="badge {{statusClass .Inspection.Status}}">{{loc .Inspection.Status}}</span></div></div>
            <div class="dl-row"><div class="dl-term">{{pair "发起人" "Requested By"}}</div><div class="dl-value">{{.Inspection.RequestedBy}}</div></div>
            <div class="dl-row"><div class="dl-term">{{pair "巡检时间范围" "Lookback Window"}}</div><div class="dl-value">{{.Inspection.LookbackMinutes}} {{pair "分钟" "min"}}</div></div>
            <div class="dl-row"><div class="dl-term">{{pair "开始时间" "Started At"}}</div><div class="dl-value">{{formatTime .Inspection.StartedAt}}</div></div>
            <div class="dl-row"><div class="dl-term">{{pair "完成时间" "Finished At"}}</div><div class="dl-value">{{formatTime .Inspection.FinishedAt}}</div></div>
          </div>
          {{else}}
          <div class="empty">{{pair "当前诊断报告没有巡检详情上下文。" "No inspection context is attached to this report."}}</div>
          {{end}}
        </div>
      </div>

      <div class="inner-tab-panel" data-inner-tab-group="evidence" data-inner-tab-key="config">
        <div class="detail-panel">
          <div class="panel-label">{{pair "运行配置" "Runtime Config"}}</div>
          {{if .ConfigSnapshot}}
          <div class="stat-grid">
            <div class="stat-card"><div class="label">{{pair "文件数" "Files"}}</div><div class="value">{{.ConfigSnapshot.FileCount}}</div></div>
            <div class="stat-card"><div class="label">{{pair "关键项" "Key Settings"}}</div><div class="value">{{.ConfigSnapshot.KeyHighlightCount}}</div></div>
            <div class="stat-card"><div class="label">{{pair "目录清单" "Inventories"}}</div><div class="value">{{.ConfigSnapshot.DirectoryCount}}</div></div>
            <div class="stat-card"><div class="label">{{pair "配置变更" "DB Changes"}}</div><div class="value">{{.ConfigSnapshot.ChangedConfigCount}}</div></div>
          </div>
          {{if .ConfigSnapshot.ConfigFileEntries}}
          <div class="subsection">
            <div class="subsection-label">{{pair "配置详情与预览" "Configurations & Previews"}}</div>
            <div class="inner-tab-toolbar config-file-tabs" role="tablist" aria-label="{{pair "配置文件" "Configuration files"}}">
              {{range $index, $file := .ConfigSnapshot.ConfigFileEntries}}
              <button type="button" role="tab" id="config-file-tab-{{$index}}" aria-controls="config-file-panel-{{$index}}" aria-selected="{{if eq $index 0}}true{{else}}false{{end}}" tabindex="{{if eq $index 0}}0{{else}}-1{{end}}" class="inner-tab-btn{{if eq $index 0}} active{{end}}" data-inner-tab-group="config-files" data-inner-tab-key="{{$index}}" title="{{$file.RemotePath}}">
                {{$file.ConfigType}} · {{if $file.HostName}}{{$file.HostName}}{{else}}{{pair "主机" "Host"}} #{{$file.HostID}}{{end}} / {{$file.Role}}
              </button>
              {{end}}
            </div>
            {{range $index, $file := .ConfigSnapshot.ConfigFileEntries}}
            <div class="inner-tab-panel{{if eq $index 0}} active{{end}}" role="tabpanel" id="config-file-panel-{{$index}}" aria-labelledby="config-file-tab-{{$index}}" data-inner-tab-group="config-files" data-inner-tab-key="{{$index}}">
              <div class="entry">
                <div class="entry-header">
                  <div>
                    <div class="entry-title">{{.ConfigType}}</div>
                    <div class="muted small">{{if .HostName}}{{.HostName}}{{else}}{{pair "主机" "Host"}} #{{.HostID}}{{end}} / {{.Role}}</div>
                  </div>
                </div>
                <div class="muted small"><code class="inline">{{.RemotePath}}</code></div>
                {{if .Items}}
                <div class="dl" style="margin-top: 10px;">
                  {{range .Items}}
                  <div class="dl-row">
                    <div class="dl-term">{{loc .Label}}</div>
                    <div class="dl-value">{{.Value}}</div>
                  </div>
                  {{end}}
                </div>
                {{end}}
                {{if .Preview}}
                <details class="config-preview-details">
                  <summary class="config-preview-summary">
                    <span>{{pair "查看已脱敏配置" "View Redacted Config"}}</span>
                  </summary>
                  <div class="copyable-block">
                    <div class="copyable-actions">
                      <button type="button" class="copy-btn" data-copy-button>
                        <span class="label-copy">{{pair "复制内容" "Copy"}}</span>
                        <span class="label-copied">{{pair "已复制" "Copied"}}</span>
                        <span class="label-failed">{{pair "复制失败" "Copy failed"}}</span>
                      </button>
                    </div>
                    <pre>{{.Preview}}</pre>
                  </div>
                </details>
                {{end}}
              </div>
            </div>
            {{end}}
          </div>
          {{else}}
          {{if .ConfigSnapshot.KeyHighlights}}
          <div class="subsection">
            <div class="subsection-label">{{pair "关键配置摘要" "Key Runtime Settings"}}</div>
            <div class="list">
              {{range .ConfigSnapshot.KeyHighlights}}
              <div class="entry">
                <div class="entry-header">
                  <div>
                    <div class="entry-title">{{.ConfigType}}</div>
                    <div class="muted small">{{if .HostName}}{{.HostName}}{{else}}{{pair "主机" "Host"}} #{{.HostID}}{{end}} / {{.Role}}</div>
                  </div>
                </div>
                <div class="muted small"><code class="inline">{{.RemotePath}}</code></div>
                <div class="dl">
                  {{range .Items}}
                  <div class="dl-row">
                    <div class="dl-term">{{loc .Label}}</div>
                    <div class="dl-value">{{.Value}}</div>
                  </div>
                  {{end}}
                </div>
              </div>
              {{end}}
            </div>
          </div>
          {{end}}
          {{if .ConfigSnapshot.FilePreviews}}
          <div class="subsection">
            <div class="subsection-label">{{pair "配置文件预览" "Config Preview"}}</div>
            <div class="list">
              {{range .ConfigSnapshot.FilePreviews}}
              <div class="entry">
                <div class="entry-header">
                  <div>
                    <div class="entry-title">{{.ConfigType}}</div>
                    <div class="muted small">{{if .HostName}}{{.HostName}}{{else}}{{pair "主机" "Host"}} #{{.HostID}}{{end}} / {{.Role}}</div>
                  </div>
                </div>
                <div class="muted small"><code class="inline">{{.RemotePath}}</code></div>
                <div class="copyable-block">
                  <div class="copyable-actions">
                    <button type="button" class="copy-btn" data-copy-button>
                      <span class="label-copy">{{pair "复制内容" "Copy"}}</span>
                      <span class="label-copied">{{pair "已复制" "Copied"}}</span>
                      <span class="label-failed">{{pair "复制失败" "Copy failed"}}</span>
                    </button>
                  </div>
                  <pre>{{.Preview}}</pre>
                </div>
              </div>
              {{end}}
            </div>
          </div>
          {{end}}
          {{end}}
          {{if .ConfigSnapshot.RecentChanges}}
          <div class="subsection">
            <div class="subsection-label">{{pair "窗口内变化轨迹" "Change Timeline"}}</div>
            <div class="list">
              {{range .ConfigSnapshot.RecentChanges}}
              <div class="entry">
                <div class="entry-header">
                  <div>
                    <div class="entry-title">{{.ConfigType}}</div>
                    <div class="muted small">{{.HostScope}} · {{pair "版本" "Version"}} {{.Version}}</div>
                  </div>
                  <span class="badge">{{formatTime .UpdatedAt}}</span>
                </div>
                <div><code class="inline">{{.FilePath}}</code></div>
              </div>
              {{end}}
            </div>
            {{if .ConfigSnapshot.RemainingChanges}}
            <details style="margin-top: 12px;">
              <summary>{{pair "查看其余配置变更" "View remaining config changes"}} ({{len .ConfigSnapshot.RemainingChanges}})</summary>
              <div class="table-wrap">
                <table>
                  <thead>
                    <tr>
                      <th class="col-time">{{pair "更新时间" "Updated At"}}</th>
                      <th class="col-type">{{pair "配置类型" "Config Type"}}</th>
                      <th class="col-scope">{{pair "范围" "Scope"}}</th>
                      <th class="col-version">{{pair "版本" "Version"}}</th>
                      <th class="col-file">{{pair "路径" "Path"}}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {{range .ConfigSnapshot.RemainingChanges}}
                    <tr>
                      <td class="col-time">{{formatTime .UpdatedAt}}</td>
                      <td class="col-type">{{.ConfigType}}</td>
                      <td class="col-scope">{{.HostScope}}</td>
                      <td class="col-version">{{.Version}}</td>
                      <td class="col-file"><code class="inline">{{.FilePath}}</code></td>
                    </tr>
                    {{end}}
                  </tbody>
                </table>
              </div>
            </details>
            {{end}}
          </div>
          {{end}}
          {{if .ConfigSnapshot.Files}}
          <div class="subsection">
            <details>
              <summary>{{pair "查看原始运行配置文件清单" "View raw runtime config files"}}</summary>
              <div class="table-wrap">
                <table class="metric-signals-table">
                  <thead>
                    <tr>
                      <th class="col-host">{{pair "主机" "Host"}}</th>
                      <th class="col-role">{{pair "角色" "Role"}}</th>
                      <th class="col-type">{{pair "类型" "Type"}}</th>
                      <th class="col-file">{{pair "远程路径" "Remote Path"}}</th>
                      <th class="col-size">{{pair "大小" "Size"}}</th>
                      <th class="col-hash">{{pair "哈希" "Hash"}}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {{range .ConfigSnapshot.Files}}
                    <tr>
                      <td class="col-host">{{if .HostName}}{{.HostName}}{{else}}{{pair "主机" "Host"}} #{{.HostID}}{{end}}</td>
                      <td class="col-role">{{.Role}}</td>
                      <td class="col-type">{{.ConfigType}}</td>
                      <td class="col-file"><code class="inline">{{.RemotePath}}</code></td>
                      <td class="col-size">{{formatBytes .SizeBytes}}</td>
                      <td class="col-hash"><code class="inline">{{shortHash .ContentHash}}</code></td>
                    </tr>
                    {{end}}
                  </tbody>
                </table>
              </div>
            </details>
          </div>
          {{end}}
          {{if .ConfigSnapshot.DirectoryManifests}}
          <div class="subsection">
            <details>
              <summary>{{pair "目录清单（配置 / 共享依赖 / 连接器 / 隔离依赖）" "Directory inventories"}}</summary>
              <div class="list" style="margin-top: 12px;">
                {{range .ConfigSnapshot.DirectoryManifests}}
                <div class="entry">
                  <div class="entry-header">
                    <div class="entry-title">{{.Directory}}</div>
                    <span class="badge">{{.EntryCount}} {{pair "项" "entries"}}</span>
                  </div>
                  <div class="muted small">{{pair "主机" "Host"}} #{{.HostID}} / {{.Role}}</div>
                  <div class="table-wrap" style="margin-top: 10px;">
                    <table>
                      <thead>
                        <tr>
                          <th class="col-name">{{pair "名称" "Name"}}</th>
                          <th class="col-file">{{pair "路径" "Path"}}</th>
                          <th class="col-size">{{pair "大小" "Size"}}</th>
                          <th class="col-time">{{pair "修改时间" "Modified"}}</th>
                        </tr>
                      </thead>
                      <tbody>
                        {{range .Entries}}
                        <tr>
                          <td class="col-name">{{.Name}}</td>
                          <td class="col-file"><code class="inline">{{.Path}}</code></td>
                          <td class="col-size">{{formatBytes .Size}}</td>
                          <td class="col-time">{{formatTime .ModTime}}</td>
                        </tr>
                        {{end}}
                      </tbody>
                    </table>
                  </div>
                </div>
                {{end}}
              </div>
            </details>
          </div>
          {{end}}
          {{if .ConfigSnapshot.CollectionNotes}}
          <div class="subsection">
            <div class="collection-notes-card">
              <div class="collection-notes-header">
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3Z"></path><line x1="12" y1="9" x2="12" y2="13"></line><line x1="12" y1="17" x2="12.01" y2="17"></line></svg>
                <span>{{pair "采集告警与环境提示" "Collection Warnings & Environment Hints"}} ({{len .ConfigSnapshot.CollectionNotes}})</span>
              </div>
              <div class="collection-notes-hint">
                {{pair "提示：若集群部署在系统临时目录（如 /tmp），操作系统定时维护机制（如 macOS tmp_cleaner 或 Linux systemd-tmpfiles）可能会自动清理活跃配置文件，导致采集失败或仅残留 .bak 备份文件。建议将生产或长期运行的集群部署在标准工作路径（如 /opt 或 ~/.seatunnel）。" "Notice: If the cluster is deployed in system temporary directories (e.g. /tmp), OS periodic maintenance daemons (such as macOS tmp_cleaner or Linux systemd-tmpfiles) may purge active config files, causing collection failures or leaving only .bak backup files. Consider deploying long-running clusters to standard paths (e.g. /opt or ~/.seatunnel)."}}
              </div>
              <div class="table-wrap" style="margin-top: 10px;">
                <table>
                  <thead>
                    <tr>
                      <th class="col-type">{{pair "配置类型" "Config Type"}}</th>
                      <th class="col-scope">{{pair "范围 / 主机" "Scope / Host"}}</th>
                      <th class="col-desc">{{pair "采集说明" "Message"}}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {{range .ConfigSnapshot.CollectionNotes}}
                    <tr>
                      <td class="col-type"><strong>{{if .ConfigType}}{{.ConfigType}}{{else}}-{{end}}</strong></td>
                      <td class="col-scope"><span class="badge">{{if .Role}}{{.Role}}{{else}}{{pair "系统" "System"}}{{end}}</span> {{if .HostID}}{{pair "主机" "Host"}} #{{.HostID}}{{else}}{{pair "集群" "Cluster"}}{{end}}</td>
                      <td class="col-desc error-text">{{loc .Message}}</td>
                    </tr>
                    {{end}}
                  </tbody>
                </table>
              </div>
            </div>
          </div>
          {{end}}
          {{else}}
          <div class="empty">{{pair "未采集到 SeaTunnel 运行配置文件。" "No runtime config files were collected."}}</div>
          {{end}}
        </div>
      </div>

      <div class="inner-tab-panel" data-inner-tab-group="evidence" data-inner-tab-key="metrics">
        <div class="detail-panel">
          <div class="panel-label">{{pair "更多指标" "More Signals"}}</div>
          {{if .MetricsSnapshot}}
          <div class="stat-grid">
            <div class="stat-card"><div class="label">{{pair "指标数" "Signals"}}</div><div class="value">{{.MetricsSnapshot.SignalCount}}</div></div>
            <div class="stat-card"><div class="label">{{pair "异常数" "Anomalies"}}</div><div class="value">{{.MetricsSnapshot.AnomalyCount}}</div></div>
          </div>
          {{if .MetricsSnapshot.CollectionNotes}}
          <div class="subsection">
            <div class="subsection-label">{{pair "采集备注" "Collection Notes"}}</div>
            <div class="list">
              {{range .MetricsSnapshot.CollectionNotes}}
              <div class="entry">
                <div>{{loc .}}</div>
              </div>
              {{end}}
            </div>
          </div>
          {{end}}
          {{if .MetricsSnapshot.AdditionalSignals}}
          <div class="subsection">
            <details>
              <summary>{{pair "查看其余指标" "View remaining signals"}} ({{len .MetricsSnapshot.AdditionalSignals}})</summary>
              <div class="list" style="margin-top: 12px;">
                {{range .MetricsSnapshot.AdditionalSignals}}
                <div class="entry">
                  <div class="entry-header">
                    <div>
                      <div class="entry-title">{{loc .Title}}</div>
                      <div class="muted small">{{loc .ThresholdText}}</div>
                    </div>
                    <span class="badge {{statusClass .Status}}">{{loc .Status}}</span>
                  </div>
                  <div>{{loc .Summary}}</div>
                </div>
                {{end}}
              </div>
            </details>
          </div>
          {{end}}
          {{if and (not .MetricsSnapshot.AdditionalSignals) .MetricsSnapshot.HighlightedSignals}}
          <div class="subsection">
            <details>
              <summary>{{pair "查看原始重点指标摘要" "View raw prioritized signal summaries"}} ({{len .MetricsSnapshot.HighlightedSignals}})</summary>
              <div class="list" style="margin-top: 12px;">
                {{range .MetricsSnapshot.HighlightedSignals}}
                <div class="entry">
                  <div class="entry-header">
                    <div>
                      <div class="entry-title">{{loc .Title}}</div>
                      <div class="muted small">{{loc .ThresholdText}}</div>
                    </div>
                    <span class="badge {{statusClass .Status}}">{{loc .Status}}</span>
                  </div>
                  <div>{{loc .Summary}}</div>
                </div>
                {{end}}
              </div>
            </details>
          </div>
          {{end}}
          {{else}}
          <div class="empty">{{pair "未采集到 Prometheus 指标快照。" "No Prometheus metrics snapshot was collected."}}</div>
          {{end}}
        </div>
      </div>

      <div class="inner-tab-panel" data-inner-tab-group="evidence" data-inner-tab-key="threaddump">
        <div class="detail-panel">
          <div class="panel-label">{{pair "线程栈快照" "Thread Dumps"}}</div>
          {{if .ThreadDumps}}
          <div class="stat-grid">
            <div class="stat-card"><div class="label">{{pair "快照总数" "Total Dumps"}}</div><div class="value">{{len .ThreadDumps}}</div></div>
          </div>
          <div class="list" style="margin-top: 14px;">
            {{range .ThreadDumps}}
            <div class="entry">
              <div class="entry-header">
                <div>
                  <div class="entry-title">{{pair "线程栈快照" "Thread Dump"}} · {{.Role}}</div>
                  <div class="muted small">{{.HostLabel}} · {{pair "工具" "Tool"}}: <code class="inline">{{if .Tool}}{{.Tool}}{{else}}jcmd Thread.print{{end}}</code> · {{pair "大小" "Size"}}: {{.SizeLabel}} ({{.TotalLines}} {{pair "行" "lines"}})</div>
                </div>
              </div>
              <div class="copyable-block" style="margin-top: 10px;">
                <div class="copyable-actions">
                  <button type="button" class="copy-btn" data-copy-button>
                    <span class="label-copy">{{pair "复制预览" "Copy Preview"}}</span>
                    <span class="label-copied">{{pair "已复制" "Copied"}}</span>
                    <span class="label-failed">{{pair "复制失败" "Copy failed"}}</span>
                  </button>
                  {{if .RelativePath}}
                  <button
                    type="button"
                    class="log-action-btn"
                    data-full-log-button
                    data-log-relative-path="{{.RelativePath}}"
                    data-log-preview-url="{{.PreviewURL}}"
                    data-log-title="{{.HostLabel}} · {{pair "线程栈" "Thread Dump"}} ({{.Role}})"
                  >
                    {{pair "查看完整堆栈" "View Full Thread Dump"}}
                  </button>
                  <a
                    class="log-action-btn"
                    data-full-log-link
                    data-log-relative-path="{{.RelativePath}}"
                    data-log-preview-url="{{.PreviewURL}}"
                    href="{{.RelativePath}}"
                    target="_blank"
                    rel="noopener noreferrer"
                  >
                    {{pair "新窗口打开" "Open in New Window"}}
                  </a>
                  {{end}}
                </div>
                <pre>{{.Preview}}</pre>
              </div>
            </div>
            {{end}}
          </div>
          {{else}}
          <div class="empty">{{pair "未采集到线程栈快照，或任务未勾选线程栈采集选项。" "No thread dump captured, or thread dump collection was not selected for this task."}}</div>
          {{end}}
        </div>
      </div>

      <div class="inner-tab-panel" data-inner-tab-group="evidence" data-inner-tab-key="alerts">
        <div class="detail-panel">
          <div class="panel-label">{{pair "告警快照" "Alert Snapshot"}}</div>
          {{if .AlertSnapshot}}
          <div class="stat-grid">
            <div class="stat-card"><div class="label">{{pair "告警总数" "Total Alerts"}}</div><div class="value">{{.AlertSnapshot.Total}}</div></div>
            <div class="stat-card"><div class="label">{{pair "严重" "Critical"}}</div><div class="value">{{.AlertSnapshot.Critical}}</div></div>
            <div class="stat-card"><div class="label">{{pair "警告" "Warning"}}</div><div class="value">{{.AlertSnapshot.Warning}}</div></div>
            <div class="stat-card"><div class="label">{{pair "告警中" "Firing"}}</div><div class="value">{{.AlertSnapshot.Firing}}</div></div>
          </div>
          <div class="dl" style="margin-top: 14px;">
            <div class="dl-row"><div class="dl-term">{{pair "首次告警" "First Seen"}}</div><div class="dl-value">{{.AlertSnapshot.FirstSeenAt}}</div></div>
            <div class="dl-row"><div class="dl-term">{{pair "最近告警" "Last Seen"}}</div><div class="dl-value">{{.AlertSnapshot.LastSeenAt}}</div></div>
          </div>
          <div class="subsection">
            <div class="subsection-label">{{pair "告警明细" "Alert Details"}}</div>
            <div class="list">
              {{range .AlertSnapshot.Alerts}}
              <div class="entry">
                <div class="entry-header">
                  <div>
                    <div class="entry-title">{{.Name}}</div>
                    <div class="muted small wrap-text">{{pair "创建于" "Created"}} {{.CreatedAt}} · {{pair "告警于" "Firing"}} {{.FiringAt}} · {{pair "最近出现" "Last Seen"}} {{.LastSeenAt}}</div>
                  </div>
                  <div style="display:flex; gap:8px; flex-wrap:wrap;">
                    <span class="badge {{statusClass .Severity}}">{{loc .Severity}}</span>
                    <span class="badge {{statusClass .Status}}">{{loc .Status}}</span>
                  </div>
                </div>
                {{if ne .ResolvedAt "-"}}<div class="muted small wrap-text">{{pair "恢复于" "Resolved"}} {{.ResolvedAt}}</div>{{end}}
                {{if .Summary}}<div style="margin-top: 8px;">{{loc .Summary}}</div>{{end}}
                {{if .Description}}<div class="muted wrap-text" style="margin-top: 8px;">{{loc .Description}}</div>{{end}}
              </div>
              {{end}}
            </div>
          </div>
          {{else}}
          <div class="empty">{{pair "未采集到活动告警。" "No alert snapshot was collected."}}</div>
          {{end}}
        </div>
      </div>

      <div class="inner-tab-panel" data-inner-tab-group="evidence" data-inner-tab-key="process">
        <div class="detail-panel">
          <div class="panel-label">{{pair "进程信号" "Process Signals"}}</div>
          {{if .ProcessEvents}}
          <div class="stat-grid">
            <div class="stat-card"><div class="label">{{pair "事件总数" "Total Events"}}</div><div class="value">{{.ProcessEvents.Total}}</div></div>
            {{range .ProcessEvents.ByType}}
            <div class="stat-card"><div class="label">{{loc .Label}}</div><div class="value">{{.Value}}</div></div>
            {{end}}
          </div>
          <div class="table-wrap" style="margin-top: 14px;">
            <table class="process-events-table">
              <thead>
                <tr>
                  <th class="col-time">{{pair "发生时间" "Created At"}}</th>
                  <th class="col-type">{{pair "事件类型" "Event Type"}}</th>
                  <th class="col-proc">{{pair "进程" "Process"}}</th>
                  <th class="col-node">{{pair "节点" "Node"}}</th>
                  <th class="col-desc">{{pair "详情" "Details"}}</th>
                </tr>
              </thead>
              <tbody>
                {{range .ProcessEvents.Events}}
                <tr>
                  <td class="col-time">{{.CreatedAt}}</td>
                  <td class="col-type">{{loc .EventType}}</td>
                  <td class="col-proc">{{.ProcessName}}</td>
                  <td class="col-node">{{.NodeLabel}}</td>
                  <td class="col-desc">{{loc .Details}}</td>
                </tr>
                {{end}}
              </tbody>
            </table>
          </div>
          {{else}}
          <div class="empty">{{pair "未采集到近期进程事件。" "No process events were collected."}}</div>
          {{end}}
        </div>
      </div>
    </section>
    </div>

    <div class="tab-page" id="tab-appendix" data-tab-page="tab-appendix">
    <section class="section" id="task-overview">
      <div class="section-heading">
        <div>
          <h2>{{pair "附录与节点信息" "Appendix & Nodes"}}</h2>
        </div>
      </div>
      <div class="inner-tab-toolbar">
        <button type="button" class="inner-tab-btn active" data-inner-tab-group="appendix" data-inner-tab-key="task">{{pair "任务" "Task"}}</button>
        <button type="button" class="inner-tab-btn" data-inner-tab-group="appendix" data-inner-tab-key="execution">{{pair "执行" "Execution"}}</button>
        <button type="button" class="inner-tab-btn" data-inner-tab-group="appendix" data-inner-tab-key="cluster">{{pair "集群" "Cluster"}}</button>
      </div>

      <div class="inner-tab-panel active" data-inner-tab-group="appendix" data-inner-tab-key="task">
        <div class="detail-columns">
          <div class="detail-panel">
            <div class="panel-label">{{pair "任务信息" "Task Metadata"}}</div>
            <div class="dl">
              <div class="dl-row"><div class="dl-term">{{pair "任务摘要" "Summary"}}</div><div class="dl-value">{{loc .Task.Summary}}</div></div>
              <div class="dl-row"><div class="dl-term">{{pair "创建人" "Created By"}}</div><div class="dl-value">{{.Task.CreatedBy}}</div></div>
              <div class="dl-row"><div class="dl-term">{{pair "开始时间" "Started At"}}</div><div class="dl-value">{{formatTime .Task.StartedAt}}</div></div>
              <div class="dl-row"><div class="dl-term">{{pair "完成时间" "Completed At"}}</div><div class="dl-value">{{formatTime .Task.CompletedAt}}</div></div>
            </div>
            <details class="debug-details">
              <summary>{{pair "查看诊断包内部存储路径" "View bundle storage paths"}}</summary>
              <div class="dl">
                <div class="dl-row"><div class="dl-term">{{pair "诊断包目录" "Bundle Dir"}}</div><div class="dl-value"><code class="inline">{{.Task.BundleDir}}</code></div></div>
                <div class="dl-row"><div class="dl-term">{{pair "清单文件" "Manifest"}}</div><div class="dl-value"><code class="inline">{{.Task.ManifestPath}}</code></div></div>
                <div class="dl-row"><div class="dl-term">{{pair "报告入口" "Report Index"}}</div><div class="dl-value"><code class="inline">{{.Task.IndexPath}}</code></div></div>
              </div>
            </details>
          </div>

          <div class="detail-panel">
            <div class="panel-label">{{pair "来源与选项" "Source & Options"}}</div>
            <div class="dl">
              {{range .SourceTraceability}}
              {{if and (ne .Value "-") (ne .Value "")}}
              <div class="dl-row"><div class="dl-term">{{loc .Label}}</div><div class="dl-value">{{loc .Value}}</div></div>
              {{end}}
              {{end}}
              <div class="dl-row"><div class="dl-term">{{pair "线程栈采集" "Thread Dump"}}</div><div class="dl-value">{{if .Task.Options.IncludeThreadDump}}{{pair "已开启" "Enabled"}}{{else}}{{pair "已关闭" "Disabled"}}{{end}}</div></div>
              <div class="dl-row"><div class="dl-term">JVM Dump</div><div class="dl-value">{{if .Task.Options.IncludeJVMDump}}{{pair "已开启" "Enabled"}}{{else}}{{pair "已关闭" "Disabled"}}{{end}}</div></div>
              {{if .Task.Options.IncludeJVMDump}}
              <div class="dl-row"><div class="dl-term">{{pair "JVM Dump 最小空间" "Min Free Space"}}</div><div class="dl-value">{{.Task.Options.JVMDumpMinFreeMB}} MB</div></div>
              {{end}}
            </div>
          </div>
        </div>

        <div class="detail-columns" style="margin-top: 18px;">
          <div class="detail-panel">
            <div class="panel-label">{{pair "目标节点" "Selected Nodes"}}</div>
            {{if .Task.SelectedNodes}}
            <div class="table-wrap" style="margin-top: 0;">
              <table>
                <thead>
                  <tr>
                    <th class="col-host">{{pair "主机" "Host"}}</th>
                    <th class="col-role">{{pair "角色" "Role"}}</th>
                    <th class="col-node">{{pair "集群节点" "Cluster Node"}}</th>
                    <th class="col-file">{{pair "安装目录" "Install Dir"}}</th>
                  </tr>
                </thead>
                <tbody>
                  {{range .Task.SelectedNodes}}
                  <tr>
                    <td class="col-host">{{.HostLabel}}</td>
                    <td class="col-role">{{.Role}}</td>
                    <td class="col-node">{{.ClusterNode}}</td>
                    <td class="col-file"><code class="inline">{{.InstallDir}}</code></td>
                  </tr>
                  {{end}}
                </tbody>
              </table>
            </div>
            {{else}}
            <div class="empty">{{pair "未记录目标节点。" "No selected nodes recorded."}}</div>
            {{end}}
          </div>

          <div class="detail-panel">
            <div class="panel-label">{{pair "已确认正常项" "Confirmed Normal"}} ({{if .PassedChecks}}{{len .PassedChecks}}{{else}}0{{end}})</div>
            {{if .PassedChecks}}
            <div class="passed-checklist-grid">
              {{range .PassedChecks}}
              <div class="passed-check-item">
                <span class="check-icon">✓</span>
                <div class="check-text">
                  <div class="check-title">{{loc .Title}}</div>
                  <div class="check-desc">{{loc .Details}}</div>
                </div>
              </div>
              {{end}}
            </div>
            {{else}}
            <div class="empty">{{pair "当前没有可展示的已通过项。" "No passed checks are available for this report."}}</div>
            {{end}}
          </div>
        </div>
      </div>

      <div class="inner-tab-panel" data-inner-tab-group="appendix" data-inner-tab-key="execution">
        <div class="grid-2">
          <div class="detail-panel">
            <div class="panel-label">{{pair "步骤状态" "Steps"}}</div>
            {{if .TaskExecution.Steps}}
            <div class="table-wrap" style="margin-top: 0;">
              <table>
                <thead>
                  <tr>
                    <th class="col-seq">#</th>
                    <th class="col-step">{{pair "步骤" "Step"}}</th>
                    <th class="col-status">{{pair "状态" "Status"}}</th>
                    <th class="col-desc">{{pair "信息" "Message"}}</th>
                    <th class="col-time">{{pair "时间" "Time"}}</th>
                  </tr>
                </thead>
                <tbody>
                  {{range .TaskExecution.Steps}}
                  <tr>
                    <td class="col-seq">{{.Sequence}}</td>
                    <td class="col-step"><strong>{{loc .Title}}</strong><div class="muted small">{{.Code}}</div></td>
                    <td class="col-status"><span class="badge {{statusClass .Status}}">{{loc .Status}}</span></td>
                    <td class="col-desc">{{if ne .Error "-"}}{{loc .Error}}{{else}}{{loc .Message}}{{end}}</td>
                    <td class="col-time">{{.StartedAt}} → {{.CompletedAt}}</td>
                  </tr>
                  {{end}}
                </tbody>
              </table>
            </div>
            {{else}}
            <div class="empty">{{pair "未记录任务步骤。" "No task steps recorded."}}</div>
            {{end}}
          </div>

          <div class="detail-panel">
            <div class="panel-label">{{pair "节点执行" "Node Execution"}}</div>
            {{if .TaskExecution.Nodes}}
            <div class="table-wrap" style="margin-top: 0;">
              <table>
                <thead>
                  <tr>
                    <th class="col-host">{{pair "主机" "Host"}}</th>
                    <th class="col-role">{{pair "角色" "Role"}}</th>
                    <th class="col-status">{{pair "状态" "Status"}}</th>
                    <th class="col-step">{{pair "当前步骤" "Current Step"}}</th>
                    <th class="col-desc">{{pair "信息" "Message"}}</th>
                  </tr>
                </thead>
                <tbody>
                  {{range .TaskExecution.Nodes}}
                  <tr>
                    <td class="col-host">{{.HostLabel}}</td>
                    <td class="col-role">{{.Role}}</td>
                    <td class="col-status"><span class="badge {{statusClass .Status}}">{{loc .Status}}</span></td>
                    <td class="col-step">{{loc .CurrentStep}}</td>
                    <td class="col-desc">{{if ne .Error "-"}}{{loc .Error}}{{else}}{{loc .Message}}{{end}}</td>
                  </tr>
                  {{end}}
                </tbody>
              </table>
            </div>
            {{else}}
            <div class="empty">{{pair "未记录节点执行信息。" "No node executions recorded."}}</div>
            {{end}}
          </div>
        </div>
      </div>

      <div class="inner-tab-panel" data-inner-tab-group="appendix" data-inner-tab-key="cluster">
        {{if .Cluster}}
        <div class="detail-columns">
          <div class="detail-panel">
            <div class="panel-label">{{pair "集群信息" "Cluster Snapshot"}}</div>
            <div class="dl">
              <div class="dl-row"><div class="dl-term">{{pair "名称" "Name"}}</div><div class="dl-value">{{.Cluster.Name}}</div></div>
              <div class="dl-row"><div class="dl-term">{{pair "版本" "Version"}}</div><div class="dl-value">{{.Cluster.Version}}</div></div>
              <div class="dl-row"><div class="dl-term">{{pair "状态" "Status"}}</div><div class="dl-value"><span class="badge {{statusClass .Cluster.Status}}">{{loc .Cluster.Status}}</span></div></div>
              <div class="dl-row"><div class="dl-term">{{pair "部署方式" "Deployment"}}</div><div class="dl-value">{{.Cluster.DeploymentMode}}</div></div>
              <div class="dl-row"><div class="dl-term">{{pair "安装目录" "Install Dir"}}</div><div class="dl-value"><code class="inline">{{.Cluster.InstallDir}}</code></div></div>
              <div class="dl-row"><div class="dl-term">{{pair "节点数" "Node Count"}}</div><div class="dl-value">{{.Cluster.NodeCount}}</div></div>
            </div>
          </div>
          <div class="detail-panel">
            <div class="panel-label">{{pair "节点信息" "Nodes"}}</div>
            <div class="table-wrap" style="margin-top: 0;">
              <table>
                <thead>
                  <tr>
                    <th class="col-host">{{pair "主机" "Host"}}</th>
                    <th class="col-node">{{pair "集群节点" "Cluster Node"}}</th>
                    <th class="col-role">{{pair "角色" "Role"}}</th>
                    <th class="col-status">{{pair "状态" "Status"}}</th>
                    <th class="col-id">PID</th>
                    <th class="col-file">{{pair "安装目录" "Install Dir"}}</th>
                  </tr>
                </thead>
                <tbody>
                  {{range .Cluster.Nodes}}
                  <tr>
                    <td class="col-host">{{.HostLabel}}</td>
                    <td class="col-node">#{{.ClusterNodeID}}</td>
                    <td class="col-role">{{.Role}}</td>
                    <td class="col-status"><span class="badge {{statusClass .Status}}">{{loc .Status}}</span></td>
                    <td class="col-id">{{.ProcessPID}}</td>
                    <td class="col-file"><code class="inline">{{.InstallDir}}</code></td>
                  </tr>
                  {{end}}
                </tbody>
              </table>
            </div>
          </div>
        </div>
        {{else}}
        <div class="empty">{{pair "未采集到集群快照。" "Cluster snapshot was not collected."}}</div>
        {{end}}
      </div>
    </section>
    </div>
    </main>
  </div>
  <div class="full-log-modal" id="full-log-modal" aria-hidden="true">
    <div class="full-log-dialog" role="dialog" aria-modal="true" aria-labelledby="full-log-modal-title">
      <div class="full-log-header">
        <div>
          <div class="eyebrow">STX</div>
          <div class="entry-title" id="full-log-modal-title">{{pair "完整原始日志" "Full Raw Log"}}</div>
        </div>
        <div class="full-log-header-actions">
          <a
            class="log-action-btn"
            id="full-log-modal-open"
            href="#"
            target="_blank"
            rel="noopener noreferrer"
          >
            {{pair "新窗口打开" "Open in New Window"}}
          </a>
          <button type="button" class="modal-close-btn" id="full-log-modal-close">
            {{pair "关闭" "Close"}}
          </button>
        </div>
      </div>
      <div class="full-log-body">
        <div class="full-log-loading active" id="full-log-modal-loading">
          {{pair "正在加载完整日志..." "Loading full log..."}}
        </div>
        <iframe
          class="full-log-frame"
          id="full-log-modal-frame"
          src="about:blank"
          title="{{pair "完整原始日志" "Full Raw Log"}}"
        ></iframe>
      </div>
    </div>
  </div>
  <script>
    (function() {
      const defaultTab = 'tab-overview';
      const pages = Array.from(document.querySelectorAll('[data-tab-page]'));
      const links = Array.from(document.querySelectorAll('[data-tab-link]'));
      function initInnerTabs() {
        const groups = Array.from(new Set(Array.from(document.querySelectorAll('[data-inner-tab-group]')).map((node) => node.dataset.innerTabGroup).filter(Boolean)));
        groups.forEach((group) => {
          const selector = '[data-inner-tab-group=\"' + group + '\"][data-inner-tab-key]';
          const buttons = Array.from(document.querySelectorAll(selector + '.inner-tab-btn'));
          const panels = Array.from(document.querySelectorAll(selector + '.inner-tab-panel'));
          if (!buttons.length || !panels.length) {
            return;
          }
          const activate = (key) => {
            buttons.forEach((button) => {
              button.classList.toggle('active', button.dataset.innerTabKey === key);
              if (button.getAttribute('role') === 'tab') {
                button.setAttribute('aria-selected', String(button.dataset.innerTabKey === key));
                button.tabIndex = button.dataset.innerTabKey === key ? 0 : -1;
              }
            });
            panels.forEach((panel) => {
              panel.classList.toggle('active', panel.dataset.innerTabKey === key);
            });
          };
          const initial = buttons.find((button) => button.classList.contains('active'))?.dataset.innerTabKey || buttons[0].dataset.innerTabKey;
          buttons.forEach((button) => {
            button.addEventListener('click', () => activate(button.dataset.innerTabKey));
            if (button.getAttribute('role') === 'tab') {
              // 配置页签支持方向键与首尾跳转。 / Configuration tabs support arrow, Home and End keys.
              button.addEventListener('keydown', (event) => {
                const index = buttons.indexOf(button);
                const next = event.key === 'ArrowRight' ? (index + 1) % buttons.length
                  : event.key === 'ArrowLeft' ? (index - 1 + buttons.length) % buttons.length
                  : event.key === 'Home' ? 0 : event.key === 'End' ? buttons.length - 1 : -1;
                if (next < 0) return;
                event.preventDefault();
                activate(buttons[next].dataset.innerTabKey);
                buttons[next].focus();
              });
            }
          });
          activate(initial);
        });
      }
      async function copyText(text) {
        if (navigator.clipboard && navigator.clipboard.writeText) {
          await navigator.clipboard.writeText(text);
          return;
        }
        const textarea = document.createElement('textarea');
        textarea.value = text;
        textarea.setAttribute('readonly', 'readonly');
        textarea.style.position = 'fixed';
        textarea.style.opacity = '0';
        document.body.appendChild(textarea);
        textarea.focus();
        textarea.select();
        try {
          document.execCommand('copy');
        } finally {
          document.body.removeChild(textarea);
        }
      }
      function initCopyButtons() {
        const buttons = Array.from(document.querySelectorAll('[data-copy-button]'));
        buttons.forEach((button) => {
          let resetTimer = null;
          button.addEventListener('click', async () => {
            const pre = button.closest('.copyable-block')?.querySelector('pre');
            const text = pre ? pre.innerText : '';
            if (!text) {
              return;
            }
            try {
              await copyText(text);
              button.classList.remove('failed');
              button.classList.add('copied');
            } catch (error) {
              button.classList.remove('copied');
              button.classList.add('failed');
            }
            if (resetTimer) {
              window.clearTimeout(resetTimer);
            }
            resetTimer = window.setTimeout(() => {
              button.classList.remove('copied', 'failed');
            }, 1800);
          });
        });
      }
      function resolveFullLogSource(node) {
        if (!node) {
          return '';
        }
        const previewURL = (node.dataset.logPreviewUrl || '').trim();
        const relativePath = (node.dataset.logRelativePath || '').trim();
        if (window.location.protocol === 'http:' || window.location.protocol === 'https:') {
          return previewURL || relativePath;
        }
        return relativePath || previewURL;
      }
      function initFullLogActions() {
        const modal = document.getElementById('full-log-modal');
        const modalTitle = document.getElementById('full-log-modal-title');
        const modalOpen = document.getElementById('full-log-modal-open');
        const modalClose = document.getElementById('full-log-modal-close');
        const modalFrame = document.getElementById('full-log-modal-frame');
        const modalLoading = document.getElementById('full-log-modal-loading');
        const links = Array.from(document.querySelectorAll('[data-full-log-link]'));
        links.forEach((link) => {
          const href = resolveFullLogSource(link);
          if (href) {
            link.setAttribute('href', href);
          }
        });
        if (!modal || !modalTitle || !modalOpen || !modalClose || !modalFrame || !modalLoading) {
          return;
        }
        const openModal = (src, title) => {
          if (!src) {
            return;
          }
          modalTitle.textContent = title || modalTitle.textContent;
          modalOpen.setAttribute('href', src);
          modal.classList.add('active');
          modal.setAttribute('aria-hidden', 'false');
          document.body.style.overflow = 'hidden';
          modalLoading.classList.add('active');
          modalFrame.setAttribute('src', src);
        };
        const closeModal = () => {
          modal.classList.remove('active');
          modal.setAttribute('aria-hidden', 'true');
          document.body.style.overflow = '';
          modalFrame.setAttribute('src', 'about:blank');
          modalOpen.setAttribute('href', '#');
          modalLoading.classList.add('active');
        };
        modalFrame.addEventListener('load', () => {
          modalLoading.classList.remove('active');
        });
        modalClose.addEventListener('click', closeModal);
        modal.addEventListener('click', (event) => {
          if (event.target === modal) {
            closeModal();
          }
        });
        document.addEventListener('keydown', (event) => {
          if (event.key === 'Escape' && modal.classList.contains('active')) {
            closeModal();
          }
        });
        const buttons = Array.from(document.querySelectorAll('[data-full-log-button]'));
        buttons.forEach((button) => {
          button.addEventListener('click', () => {
            openModal(
              resolveFullLogSource(button),
              (button.dataset.logTitle || '').trim()
            );
          });
        });
      }
      function applyTab() {
        const hash = (window.location.hash || '#' + defaultTab).replace(/^#/, '');
        const active = pages.some((page) => page.dataset.tabPage === hash) ? hash : defaultTab;
        pages.forEach((page) => {
          page.classList.toggle('active', page.dataset.tabPage === active);
        });
        links.forEach((link) => {
          link.classList.toggle('active', link.dataset.tabLink === active);
        });
        if (window.location.hash.replace(/^#/, '') !== active) {
          history.replaceState(null, '', '#' + active);
        }
        window.scrollTo({top: 0, behavior: 'auto'});
      }
      function initThemeToggle() {
        const STORAGE_KEY = 'stx-diagnostic-theme';
        const toggleBtn = document.getElementById('theme-toggle-btn');
        const iconLight = toggleBtn ? toggleBtn.querySelector('.theme-icon-light') : null;
        const iconDark = toggleBtn ? toggleBtn.querySelector('.theme-icon-dark') : null;
        const label = document.getElementById('theme-toggle-label');
        const isZH = {{if eq .Language "en"}}false{{else}}true{{end}};

        function getStoredTheme() {
          return localStorage.getItem(STORAGE_KEY);
        }

        function getSystemTheme() {
          return window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
        }

        function applyTheme(theme) {
          document.documentElement.setAttribute('data-theme', theme);
          if (iconLight && iconDark) {
            if (theme === 'dark') {
              iconLight.style.display = 'inline-flex';
              iconDark.style.display = 'none';
              if (label) label.textContent = isZH ? '浅色模式' : 'Light Mode';
            } else {
              iconLight.style.display = 'none';
              iconDark.style.display = 'inline-flex';
              if (label) label.textContent = isZH ? '暗色模式' : 'Dark Mode';
            }
          }
        }

        const initialTheme = getStoredTheme() || getSystemTheme();
        applyTheme(initialTheme);

        if (window.matchMedia) {
          window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', (e) => {
            if (!getStoredTheme()) {
              applyTheme(e.matches ? 'dark' : 'light');
            }
          });
        }

        if (toggleBtn) {
          toggleBtn.addEventListener('click', () => {
            const current = document.documentElement.getAttribute('data-theme') || getSystemTheme();
            const next = current === 'dark' ? 'light' : 'dark';
            localStorage.setItem(STORAGE_KEY, next);
            applyTheme(next);
          });
        }
      }
      window.addEventListener('hashchange', applyTab);
      initInnerTabs();
      initCopyButtons();
      initFullLogActions();
      initThemeToggle();
      applyTab();
    })();
  </script>
</body>
</html>`
