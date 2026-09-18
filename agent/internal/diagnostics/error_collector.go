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

// Package diagnostics provides agent-side diagnostics evidence collection.
// diagnostics 包提供 Agent 侧的诊断证据采集能力。
package diagnostics

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	pb "github.com/LeonYoah/stx/agent"
	"github.com/LeonYoah/stx/agent/internal/logger"
)

const (
	defaultScanInterval   = 10 * time.Second
	defaultInitialTail    = int64(256 * 1024)
	defaultMaxPayloadSize = 16 * 1024
	defaultMaxEntries     = 200
	errorBlockMergeWindow = 2 * time.Second
)

var (
	logHeaderPattern         = regexp.MustCompile(`^(?:\[([^\]]*)\]\s+)?(\d{4}-\d{2}-\d{2}[ T]\d{2}:\d{2}:\d{2}(?:[\.,]\d{3})?)\s+(TRACE|DEBUG|INFO|WARN|ERROR|FATAL)\b(.*)$`)
	jobLogPattern            = regexp.MustCompile(`^job-([^./]+)\.log$`)
	headerOnlyMessagePattern = regexp.MustCompile(`^(?:\[[^\]]*\]\s*){1,4}-?\s*$`)
)

// LogSender describes the gRPC sender needed by the collector.
// LogSender 描述采集器需要的 gRPC 日志发送能力。
type LogSender interface {
	IsConnected() bool
	GetAgentID() string
	SendLogEntries(ctx context.Context, entries []*pb.LogEntry) error
}

// ScanTarget describes one managed Seatunnel runtime target on the host.
// ScanTarget 描述主机上一个受管的 Seatunnel 运行目标。
type ScanTarget struct {
	Name       string
	InstallDir string
	Role       string
}

// Collector incrementally scans Seatunnel logs and reports ERROR evidence.
// Collector 增量扫描 Seatunnel 日志并上报 ERROR 证据。
type Collector struct {
	sender LogSender

	scanInterval   time.Duration
	initialTail    int64
	maxPayloadSize int
	maxEntries     int

	mu      sync.RWMutex
	targets map[string]*ScanTarget
	cursors map[string]*fileCursor
}

type fileCursor struct {
	Offset      int64
	Initialized bool
}

type parsedEntry struct {
	OccurredAt  time.Time
	Summary     string
	Body        string
	CursorStart int64
	CursorEnd   int64
	ExecutionID string
	LoggerName  string
	ThreadName  string
}

// NewCollector creates a Seatunnel ERROR collector.
// NewCollector 创建 Seatunnel ERROR 采集器。
func NewCollector(sender LogSender) *Collector {
	return &Collector{
		sender:         sender,
		scanInterval:   defaultScanInterval,
		initialTail:    defaultInitialTail,
		maxPayloadSize: defaultMaxPayloadSize,
		maxEntries:     defaultMaxEntries,
		targets:        make(map[string]*ScanTarget),
		cursors:        make(map[string]*fileCursor),
	}
}

// SetInitialCursor sets an initial cursor offset for a given file cursor key.
// SetInitialCursor 为指定游标 key 设置初始偏移量（用于 Agent 启动时对齐服务端游标）。
func (c *Collector) SetInitialCursor(key string, offset int64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if offset < 0 {
		offset = 0
	}
	cursor := c.cursors[key]
	if cursor == nil {
		cursor = &fileCursor{}
		c.cursors[key] = cursor
	}
	cursor.Offset = offset
	cursor.Initialized = true
}

// ReplaceTargets replaces the current managed runtime targets.
// ReplaceTargets 替换当前受管运行目标列表。
func (c *Collector) ReplaceTargets(targets []*ScanTarget) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.targets = make(map[string]*ScanTarget, len(targets))
	for _, target := range targets {
		if target == nil {
			continue
		}
		key := buildTargetKey(target.InstallDir, target.Role)
		c.targets[key] = &ScanTarget{
			Name:       strings.TrimSpace(target.Name),
			InstallDir: strings.TrimSpace(target.InstallDir),
			Role:       strings.TrimSpace(target.Role),
		}
	}
}

// Start starts the periodic scan loop.
// Start 启动周期扫描循环。
func (c *Collector) Start(ctx context.Context) {
	if c == nil {
		return
	}

	go func() {
		_ = c.CollectOnce(ctx)

		ticker := time.NewTicker(c.scanInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := c.CollectOnce(ctx); err != nil {
					logger.WarnF(ctx, "[Diagnostics] Seatunnel error collection failed: %v / Seatunnel 错误采集失败：%v", err, err)
				}
			}
		}
	}()
}

// CollectOnce performs one incremental scan cycle.
// CollectOnce 执行一次增量扫描周期。
func (c *Collector) CollectOnce(ctx context.Context) error {
	if c == nil || c.sender == nil || !c.sender.IsConnected() {
		return nil
	}

	targets := c.snapshotTargets()
	if len(targets) == 0 {
		return nil
	}

	pendingCursors := make(map[string]int64)
	entries := make([]*pb.LogEntry, 0)

	for _, target := range targets {
		targetEntries, updates, err := c.collectTargetEntries(ctx, target)
		if err != nil {
			logger.WarnF(ctx, "[Diagnostics] Failed to scan target %s:%s: %v / 扫描目标 %s:%s 失败：%v",
				target.InstallDir, target.Role, err, target.InstallDir, target.Role, err)
			continue
		}
		for key, offset := range updates {
			pendingCursors[key] = offset
		}
		entries = append(entries, targetEntries...)
		if len(entries) >= c.maxEntries {
			entries = entries[:c.maxEntries]
			break
		}
	}

	if len(entries) > 0 {
		if err := c.sender.SendLogEntries(ctx, entries); err != nil {
			return err
		}
		logger.InfoF(ctx, "[Diagnostics] Reported %d Seatunnel error entries from %d targets / 已从 %d 个目标上报 %d 条 Seatunnel 错误日志",
			len(entries), len(targets), len(targets), len(entries))
	}

	c.commitCursors(pendingCursors)
	return nil
}

func (c *Collector) snapshotTargets() []*ScanTarget {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make([]*ScanTarget, 0, len(c.targets))
	for _, target := range c.targets {
		copyTarget := *target
		result = append(result, &copyTarget)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].InstallDir == result[j].InstallDir {
			return result[i].Role < result[j].Role
		}
		return result[i].InstallDir < result[j].InstallDir
	})
	return result
}

func (c *Collector) collectTargetEntries(ctx context.Context, target *ScanTarget) ([]*pb.LogEntry, map[string]int64, error) {
	files, err := resolveLogFiles(target.InstallDir)
	if err != nil {
		return nil, nil, err
	}
	files = filterLogFilesByRole(files, target.Role)

	entries := make([]*pb.LogEntry, 0)
	updates := make(map[string]int64)
	agentID := c.sender.GetAgentID()
	if agentID == "" {
		return nil, nil, nil
	}

	for _, filePath := range files {
		if len(entries) >= c.maxEntries {
			break
		}
		cursorKey := buildFileCursorKey(target.InstallDir, target.Role, filePath)
		cursor := c.getCursor(cursorKey)
		forceFromStart := false

		if info, statErr := os.Stat(filePath); statErr == nil && !info.IsDir() && cursor != nil && cursor.Initialized && info.Size() < cursor.Offset {
			logger.InfoF(ctx, "[Diagnostics] Detected log reset/rotation for %s (old_offset=%d, new_size=%d) / 检测到日志重置或滚动：%s（旧偏移=%d，新大小=%d）",
				filePath, cursor.Offset, info.Size(), filePath, cursor.Offset, info.Size())
			rotatedEntries, rotatedUpdates, rotatedErr := c.collectRotatedGapEntries(ctx, target, filePath, cursor.Offset)
			if rotatedErr != nil {
				logger.WarnF(ctx, "[Diagnostics] Failed to scan rotated gap for %s: %v / 扫描滚动日志缺口失败：%s：%v",
					filePath, rotatedErr, filePath, rotatedErr)
			} else {
				entries = append(entries, rotatedEntries...)
				for key, offset := range rotatedUpdates {
					updates[key] = offset
				}
			}
			forceFromStart = true
			cursor = nil
		}

		parsedEntries, nextOffset, err := c.scanFile(target, filePath, cursor, forceFromStart)
		if err != nil {
			logger.WarnF(ctx, "[Diagnostics] Failed to scan file %s: %v / 扫描文件 %s 失败：%v", filePath, err, filePath, err)
			continue
		}
		updates[cursorKey] = nextOffset

		for _, item := range parsedEntries {
			fields := map[string]string{
				"source":       "seatunnel_error",
				"install_dir":  target.InstallDir,
				"role":         target.Role,
				"source_file":  filePath,
				"source_kind":  detectSourceKind(filePath),
				"cursor_start": strconv.FormatInt(item.CursorStart, 10),
				"cursor_end":   strconv.FormatInt(item.CursorEnd, 10),
				"body":         item.Body,
			}
			if target.Name != "" {
				fields["process_name"] = target.Name
			}
			if item.ExecutionID != "" {
				fields["execution_id"] = item.ExecutionID
			}
			if item.LoggerName != "" {
				fields["logger_name"] = item.LoggerName
			}
			if item.ThreadName != "" {
				fields["thread_name"] = item.ThreadName
			}
			if jobID := detectJobID(filePath); jobID != "" {
				fields["job_id"] = jobID
			}
			entries = append(entries, &pb.LogEntry{
				AgentId:   agentID,
				Timestamp: item.OccurredAt.UnixMilli(),
				Level:     pb.LogLevel_ERROR,
				Message:   item.Summary,
				Fields:    fields,
			})
			if len(entries) >= c.maxEntries {
				break
			}
		}
	}

	return entries, updates, nil
}

func filterLogFilesByRole(files []string, role string) []string {
	role = strings.ToLower(strings.TrimSpace(role))
	if len(files) == 0 || role == "" || role == "hybrid" || role == "master/worker" || role == "master_worker" {
		return files
	}

	// On hybrid deployment, master and worker may be on the same host and share the same installDir/logs.
	// If we scan all `seatunnel-engine-*.log` for each role target, we will duplicate the same evidence.
	// Filter engine logs by role to keep one-to-one mapping between target role and log file(s).
	result := make([]string, 0, len(files))
	for _, filePath := range files {
		base := strings.ToLower(filepath.Base(strings.TrimSpace(filePath)))

		// job-*.log is usually produced by worker execution.
		if strings.HasPrefix(base, "job-") {
			if role == "worker" {
				result = append(result, filePath)
			}
			continue
		}

		if strings.Contains(base, "seatunnel-engine-master") || strings.Contains(base, "engine-master") || strings.Contains(base, "master.log") {
			if role == "master" {
				result = append(result, filePath)
			}
			continue
		}
		if strings.Contains(base, "seatunnel-engine-worker") || strings.Contains(base, "engine-worker") || strings.Contains(base, "worker.log") {
			if role == "worker" {
				result = append(result, filePath)
			}
			continue
		}

		// Unknown engine log name: keep it for safety (better some noise than missing evidence).
		result = append(result, filePath)
	}
	return result
}

func (c *Collector) collectRotatedGapEntries(ctx context.Context, target *ScanTarget, activeFilePath string, previousOffset int64) ([]*pb.LogEntry, map[string]int64, error) {
	rotatedFilePath, err := findLatestRotatedFile(activeFilePath)
	if err != nil || rotatedFilePath == "" {
		return nil, nil, err
	}

	rotatedCursorKey := buildFileCursorKey(target.InstallDir, target.Role, rotatedFilePath)
	startOffset := previousOffset
	if rotatedCursor := c.getCursor(rotatedCursorKey); rotatedCursor != nil && rotatedCursor.Initialized && rotatedCursor.Offset > startOffset {
		startOffset = rotatedCursor.Offset
	}

	parsedEntries, nextOffset, err := c.scanFile(target, rotatedFilePath, &fileCursor{
		Offset:      startOffset,
		Initialized: true,
	}, false)
	if err != nil {
		return nil, nil, err
	}

	agentID := c.sender.GetAgentID()
	if agentID == "" {
		return nil, map[string]int64{rotatedCursorKey: nextOffset}, nil
	}

	entries := make([]*pb.LogEntry, 0, len(parsedEntries))
	for _, item := range parsedEntries {
		fields := map[string]string{
			"source":       "seatunnel_error",
			"install_dir":  target.InstallDir,
			"role":         target.Role,
			"source_file":  rotatedFilePath,
			"source_kind":  detectSourceKind(rotatedFilePath),
			"cursor_start": strconv.FormatInt(item.CursorStart, 10),
			"cursor_end":   strconv.FormatInt(item.CursorEnd, 10),
			"body":         item.Body,
		}
		if target.Name != "" {
			fields["process_name"] = target.Name
		}
		if item.ExecutionID != "" {
			fields["execution_id"] = item.ExecutionID
		}
		if item.LoggerName != "" {
			fields["logger_name"] = item.LoggerName
		}
		if item.ThreadName != "" {
			fields["thread_name"] = item.ThreadName
		}
		if jobID := detectJobID(rotatedFilePath); jobID != "" {
			fields["job_id"] = jobID
		}
		entries = append(entries, &pb.LogEntry{
			AgentId:   agentID,
			Timestamp: item.OccurredAt.UnixMilli(),
			Level:     pb.LogLevel_ERROR,
			Message:   item.Summary,
			Fields:    fields,
		})
		if len(entries) >= c.maxEntries {
			break
		}
	}

	if len(entries) > 0 {
		logger.InfoF(ctx, "[Diagnostics] Recovered %d Seatunnel error entries from rotated file %s / 已从滚动日志 %s 补采 %d 条 Seatunnel 错误",
			len(entries), rotatedFilePath, rotatedFilePath, len(entries))
	}

	return entries, map[string]int64{rotatedCursorKey: nextOffset}, nil
}

func (c *Collector) scanFile(target *ScanTarget, filePath string, cursor *fileCursor, forceFromStart bool) ([]*parsedEntry, int64, error) {
	file, err := os.Open(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, 0, nil
		}
		return nil, 0, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, 0, err
	}
	if info.IsDir() {
		return nil, 0, nil
	}

	startOffset := int64(0)
	if cursor != nil && cursor.Initialized {
		startOffset = cursor.Offset
	}
	if startOffset > info.Size() {
		startOffset = 0
	}
	if !forceFromStart && (cursor == nil || !cursor.Initialized) && info.Size() > c.initialTail {
		startOffset = info.Size() - c.initialTail
	}
	if startOffset < 0 {
		startOffset = 0
	}

	if _, err := file.Seek(startOffset, io.SeekStart); err != nil {
		return nil, 0, err
	}

	reader := bufio.NewReader(file)
	entries := make([]*parsedEntry, 0)
	var current *entryBuilder
	position := startOffset

	for {
		line, readErr := reader.ReadString('\n')
		if readErr != nil && readErr != io.EOF {
			return nil, 0, readErr
		}

		lineStart := position
		position += int64(len(line))
		trimmed := strings.TrimRight(line, "\r\n")

		if header := parseLogHeader(trimmed); header != nil {
			if current != nil && shouldMergeSeatunnelErrorHeader(current, header) {
				current.AppendHeaderLine(trimmed, header)
			} else if current != nil {
				current.EndOffset = lineStart
				if entry := current.Build(c.maxPayloadSize); entry != nil {
					entries = append(entries, entry)
				}
				current = nil
			}

			if current == nil && (header.Level == "ERROR" || header.Level == "FATAL") {
				current = &entryBuilder{
					OccurredAt:  header.Timestamp,
					Summary:     header.Message,
					CursorStart: lineStart,
					Lines:       []string{trimmed},
					ExecutionID: header.ExecutionID,
					LoggerName:  header.LoggerName,
					ThreadName:  header.ThreadName,
				}
			}
		} else if current != nil {
			current.Lines = append(current.Lines, trimmed)
		}

		if readErr == io.EOF {
			break
		}
	}

	if current != nil {
		current.EndOffset = position
		if entry := current.Build(c.maxPayloadSize); entry != nil {
			entries = append(entries, entry)
		}
	}

	return entries, position, nil
}

func (c *Collector) getCursor(key string) *fileCursor {
	c.mu.RLock()
	defer c.mu.RUnlock()

	cursor, ok := c.cursors[key]
	if !ok {
		return nil
	}
	copyCursor := *cursor
	return &copyCursor
}

func (c *Collector) commitCursors(updates map[string]int64) {
	if len(updates) == 0 {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	for key, offset := range updates {
		cursor, ok := c.cursors[key]
		if !ok {
			cursor = &fileCursor{}
			c.cursors[key] = cursor
		}
		cursor.Offset = offset
		cursor.Initialized = true
	}
}

func resolveLogFiles(installDir string) ([]string, error) {
	logDir := filepath.Join(strings.TrimSpace(installDir), "logs")
	if logDir == "" {
		return nil, nil
	}
	if _, err := os.Stat(logDir); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	patterns := []string{
		filepath.Join(logDir, "seatunnel-engine-*.log"),
		filepath.Join(logDir, "job-*.log"),
	}

	unique := make(map[string]struct{})
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, fmt.Errorf("glob %s: %w", pattern, err)
		}
		for _, match := range matches {
			unique[match] = struct{}{}
		}
	}

	files := make([]string, 0, len(unique))
	for filePath := range unique {
		files = append(files, filePath)
	}
	sort.Strings(files)
	return files, nil
}

func buildTargetKey(installDir, role string) string {
	return strings.TrimSpace(installDir) + "::" + strings.TrimSpace(role)
}

func buildFileCursorKey(installDir, role, filePath string) string {
	return buildTargetKey(installDir, role) + "::" + strings.TrimSpace(filePath)
}

func detectSourceKind(filePath string) string {
	base := filepath.Base(filePath)
	if strings.HasPrefix(base, "job-") {
		return "job"
	}
	return "engine"
}

func detectJobID(filePath string) string {
	matches := jobLogPattern.FindStringSubmatch(filepath.Base(filePath))
	if len(matches) != 2 {
		return ""
	}
	return matches[1]
}

func findLatestRotatedFile(activeFilePath string) (string, error) {
	matches, err := filepath.Glob(activeFilePath + ".*")
	if err != nil {
		return "", fmt.Errorf("glob rotated %s: %w", activeFilePath, err)
	}

	type candidate struct {
		path    string
		modTime time.Time
	}
	candidates := make([]candidate, 0, len(matches))
	for _, match := range matches {
		if strings.HasSuffix(match, ".gz") {
			continue
		}
		info, statErr := os.Stat(match)
		if statErr != nil || info.IsDir() {
			continue
		}
		candidates = append(candidates, candidate{path: match, modTime: info.ModTime()})
	}
	if len(candidates) == 0 {
		return "", nil
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].modTime.Equal(candidates[j].modTime) {
			return candidates[i].path > candidates[j].path
		}
		return candidates[i].modTime.After(candidates[j].modTime)
	})
	return candidates[0].path, nil
}

type logHeader struct {
	Timestamp   time.Time
	Level       string
	Message     string
	ExecutionID string
	LoggerName  string
	ThreadName  string
}

func parseLogHeader(line string) *logHeader {
	matches := logHeaderPattern.FindStringSubmatch(strings.TrimSpace(line))
	if len(matches) != 5 {
		return nil
	}

	timestamp := parseLogTime(matches[2])
	message, loggerName, threadName := extractLogMessage(strings.TrimSpace(matches[4]))
	if message == "" {
		message = strings.TrimSpace(line)
	}

	return &logHeader{
		Timestamp:   timestamp,
		Level:       strings.TrimSpace(matches[3]),
		Message:     message,
		ExecutionID: strings.TrimSpace(matches[1]),
		LoggerName:  loggerName,
		ThreadName:  threadName,
	}
}

func extractLogMessage(value string) (message, loggerName, threadName string) {
	rest := strings.TrimSpace(value)
	segments := make([]string, 0, 2)

	for len(segments) < 2 && strings.HasPrefix(rest, "[") {
		end := strings.Index(rest, "]")
		if end <= 0 {
			break
		}
		segments = append(segments, strings.TrimSpace(rest[1:end]))
		rest = strings.TrimSpace(rest[end+1:])
	}

	rest = strings.TrimSpace(strings.TrimPrefix(rest, "-"))
	rest = strings.TrimSpace(rest)
	if rest == "" {
		rest = strings.TrimSpace(value)
	}

	if len(segments) > 0 {
		loggerName = segments[0]
	}
	if len(segments) > 1 {
		threadName = segments[1]
	}

	return rest, loggerName, threadName
}

func parseLogTime(value string) time.Time {
	layouts := []string{
		"2006-01-02 15:04:05,000",
		"2006-01-02 15:04:05.000",
		"2006-01-02T15:04:05,000",
		"2006-01-02T15:04:05.000",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
	}
	trimmed := strings.TrimSpace(value)
	for _, layout := range layouts {
		if ts, err := time.ParseInLocation(layout, trimmed, time.Local); err == nil {
			return ts
		}
	}
	return time.Now()
}

// shouldMergeSeatunnelErrorHeader decides whether a new ERROR/FATAL header should
// be treated as part of the current Seatunnel fatal-wrapper block.
// shouldMergeSeatunnelErrorHeader 判断新 ERROR/FATAL 头是否应并入当前 Seatunnel 致命错误包装块。
func shouldMergeSeatunnelErrorHeader(current *entryBuilder, next *logHeader) bool {
	if current == nil || next == nil {
		return false
	}
	if !isSeatunnelContinuationHeader(next.Message) {
		return false
	}
	if current.ExecutionID != strings.TrimSpace(next.ExecutionID) {
		return false
	}
	if current.LoggerName != strings.TrimSpace(next.LoggerName) {
		return false
	}
	if current.ThreadName != strings.TrimSpace(next.ThreadName) {
		return false
	}
	if current.OccurredAt.IsZero() || next.Timestamp.IsZero() {
		return true
	}
	delta := next.Timestamp.Sub(current.OccurredAt)
	if delta < 0 {
		delta = -delta
	}
	return delta <= errorBlockMergeWindow
}

func isSeatunnelContinuationHeader(message string) bool {
	normalized := strings.ToLower(strings.TrimSpace(normalizeCollectorMessageLine(message)))
	if normalized == "" {
		return true
	}
	return normalized == "fatal error" ||
		normalized == "fatal error," ||
		normalized == "please submit bug report in https://github.com/apache/seatunnel/issues" ||
		strings.HasPrefix(normalized, "reason:") ||
		strings.HasPrefix(normalized, "exception stacktrace:") ||
		strings.HasPrefix(normalized, "caused by:")
}

func normalizeCollectorMessageLine(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if headerOnlyMessagePattern.MatchString(trimmed) {
		return ""
	}
	if header := parseLogHeader(trimmed); header != nil {
		trimmed = strings.TrimSpace(header.Message)
		if headerOnlyMessagePattern.MatchString(trimmed) {
			return ""
		}
	}
	return strings.TrimSpace(trimmed)
}

func summarizeCollectorLine(value string) string {
	line := strings.TrimSpace(normalizeCollectorMessageLine(value))
	if line == "" {
		return ""
	}
	for _, prefix := range []string{"Caused by:", "Exception StackTrace:", "Reason:"} {
		if strings.HasPrefix(line, prefix) {
			line = strings.TrimSpace(strings.TrimPrefix(line, prefix))
			break
		}
	}
	return strings.TrimSpace(line)
}

func collectorSummaryPriority(value string) int {
	normalized := strings.ToLower(strings.TrimSpace(normalizeCollectorMessageLine(value)))
	switch {
	case normalized == "":
		return 0
	case strings.HasPrefix(normalized, "caused by:"):
		return 5
	case strings.HasPrefix(normalized, "exception stacktrace:"):
		return 4
	case strings.HasPrefix(normalized, "reason:"):
		return 3
	case normalized == "fatal error" || normalized == "fatal error,":
		return 1
	case normalized == "please submit bug report in https://github.com/apache/seatunnel/issues":
		return 1
	default:
		return 2
	}
}

type entryBuilder struct {
	OccurredAt  time.Time
	Summary     string
	CursorStart int64
	EndOffset   int64
	Lines       []string
	ExecutionID string
	LoggerName  string
	ThreadName  string
}

// AppendHeaderLine merges one additional ERROR/FATAL header into the current
// builder so multi-line Seatunnel fatal-wrapper blocks stay as one event.
// AppendHeaderLine 将额外的 ERROR/FATAL 头并入当前构建器，使 Seatunnel 致命错误包装块保持为单条事件。
func (b *entryBuilder) AppendHeaderLine(rawLine string, header *logHeader) {
	if b == nil {
		return
	}
	b.Lines = append(b.Lines, rawLine)
	if header == nil {
		return
	}
	if better := chooseCollectorSummary(b.Summary, header.Message); better != "" {
		b.Summary = better
	}
}

func (b *entryBuilder) Build(maxPayloadSize int) *parsedEntry {
	if b == nil || len(b.Lines) == 0 {
		return nil
	}
	body := strings.TrimSpace(strings.Join(b.Lines, "\n"))
	if body == "" {
		return nil
	}
	if len(body) > maxPayloadSize {
		body = body[:maxPayloadSize]
	}
	summary := chooseCollectorSummary(b.Summary, b.Lines...)
	if summary == "" {
		summary = body
	}
	if len(summary) > 512 {
		summary = summary[:512]
	}
	occurredAt := b.OccurredAt
	if occurredAt.IsZero() {
		occurredAt = time.Now()
	}
	return &parsedEntry{
		OccurredAt:  occurredAt,
		Summary:     summary,
		Body:        body,
		CursorStart: b.CursorStart,
		CursorEnd:   b.EndOffset,
		ExecutionID: b.ExecutionID,
		LoggerName:  b.LoggerName,
		ThreadName:  b.ThreadName,
	}
}

func chooseCollectorSummary(current string, candidates ...string) string {
	best := strings.TrimSpace(summarizeCollectorLine(current))
	bestPriority := collectorSummaryPriority(current)
	for _, candidate := range candidates {
		priority := collectorSummaryPriority(candidate)
		if priority < bestPriority {
			continue
		}
		line := strings.TrimSpace(summarizeCollectorLine(candidate))
		if line == "" {
			continue
		}
		if priority > bestPriority || best == "" {
			best = line
			bestPriority = priority
		}
	}
	if best != "" {
		return best
	}
	return firstNonEmptyLine(candidates)
}

func firstNonEmptyLine(lines []string) string {
	for _, line := range lines {
		trimmed := strings.TrimSpace(summarizeCollectorLine(line))
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}
