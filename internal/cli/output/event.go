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
package output

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
)

// Event 表示写入 stderr 的单行 JSON 事件。
// Event represents one JSON event written to stderr.
type Event struct {
	Event          string   `json:"event"`
	Code           string   `json:"code,omitempty"`
	Message        string   `json:"message,omitempty"`
	Retryable      *bool    `json:"retryable,omitempty"`
	RequestID      string   `json:"request_id,omitempty"`
	OperationID    string   `json:"operation_id,omitempty"`
	ExecutionID    string   `json:"execution_id,omitempty"`
	ConfirmationID string   `json:"confirmation_id,omitempty"`
	RiskLevel      string   `json:"risk_level,omitempty"`
	ExpiresAt      string   `json:"expires_at,omitempty"`
	Level          string   `json:"level,omitempty"`
	Progress       *int     `json:"progress,omitempty"`
	MissingFields  []string `json:"missing_fields,omitempty"`
	// JobID 关联 sync 作业实例，供 --wait 进度/日志事件使用。
	// JobID links a sync job instance for --wait progress/log events.
	JobID uint `json:"job_id,omitempty"`
	// Status 为作业或执行状态快照。
	// Status is a job or execution status snapshot.
	Status string `json:"status,omitempty"`
	// SyncState 反映 STX 与引擎对账观察面（读库字段）。
	// SyncState reflects STX engine reconciler observability (DB read path).
	SyncState string `json:"sync_state,omitempty"`
	// SyncLagMs 为 API 派生的同步滞后毫秒数（可选）。
	// SyncLagMs is optional derived sync lag in milliseconds from the API.
	SyncLagMs *int64 `json:"sync_lag_ms,omitempty"`
	// NextOffset 为日志增量读偏移令牌。
	// NextOffset is the incremental log read offset token.
	NextOffset string `json:"next_offset,omitempty"`
	// Logs 为 log_chunk 事件的增量正文（含真实换行，JSON 中转义）。
	// Logs carries incremental text for log_chunk events (real newlines, JSON-escaped).
	Logs string `json:"logs,omitempty"`
	// Lines 为按行拆分的日志，便于传输后仍可按行阅读。
	// Lines is the line-split form so transferred logs stay readable per line.
	Lines []string `json:"lines,omitempty"`
	// EmptyReason 说明日志暂不可用（如 logs_not_ready）。
	// EmptyReason explains missing logs (e.g. logs_not_ready).
	EmptyReason string `json:"empty_reason,omitempty"`
}

// EventFormat controls how EventWriter serializes events to the sink.
// EventFormat 控制 EventWriter 如何把事件写到输出。
type EventFormat string

const (
	// EventFormatJSON writes one NDJSON object per event (machine-oriented).
	// EventFormatJSON 每个事件写一行 NDJSON（机器可读）。
	EventFormatJSON EventFormat = "json"
	// EventFormatText writes plain human-readable lines (DolphinScheduler / terminal friendly).
	// EventFormatText 写纯文本行（适合 DS / 终端阅读）。
	EventFormatText EventFormat = "text"
)

// EventWriter 以并发安全方式写入事件（NDJSON 或纯文本）。
// EventWriter writes events safely as NDJSON or plain text.
type EventWriter struct {
	writer io.Writer
	format EventFormat
	mu     sync.Mutex
}

// NewEventWriter 创建默认 NDJSON 事件写入器。
// NewEventWriter creates a stderr event writer in NDJSON mode.
func NewEventWriter(writer io.Writer) *EventWriter {
	return &EventWriter{writer: writer, format: EventFormatJSON}
}

// NewTextEventWriter 创建纯文本事件写入器（人类可读，供 --wait 默认使用）。
// NewTextEventWriter creates a plain-text event writer (human-readable; default for --wait).
func NewTextEventWriter(writer io.Writer) *EventWriter {
	return &EventWriter{writer: writer, format: EventFormatText}
}

// SetFormat switches serialization format; empty keeps current.
// SetFormat 切换序列化格式；空值保持不变。
func (w *EventWriter) SetFormat(format EventFormat) {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	switch format {
	case EventFormatJSON, EventFormatText:
		w.format = format
	}
}

// Emit 按当前格式写入一个事件。
// Emit writes one event using the configured format.
func (w *EventWriter) Emit(event Event) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.format == EventFormatText {
		return w.emitTextLocked(event)
	}
	return json.NewEncoder(w.writer).Encode(event)
}

func (w *EventWriter) emitTextLocked(event Event) error {
	name := strings.TrimSpace(event.Event)
	switch name {
	case "log_line":
		return writePlainLine(w.writer, event.Message)
	case "log_chunk":
		if len(event.Lines) > 0 {
			for _, line := range event.Lines {
				if err := writePlainLine(w.writer, line); err != nil {
					return err
				}
			}
			return nil
		}
		return writePlainBlock(w.writer, event.Logs)
	case "job_metrics":
		// Metrics body is already emitted as log_line / plain lines; skip envelope noise.
		// 指标正文已按行写出；跳过事件外壳以免 DS 日志出现 JSON。
		return nil
	case "idempotency_key":
		return nil
	case "job_wait":
		msg := strings.TrimSpace(event.Message)
		if msg == "" {
			msg = fmt.Sprintf("waiting for job %d", event.JobID)
		}
		return writePlainLine(w.writer, "[stx] "+msg)
	case "job_status":
		status := strings.TrimSpace(event.Status)
		if status == "" {
			status = strings.TrimSpace(event.Message)
		}
		if status == "" {
			return nil
		}
		return writePlainLine(w.writer, "[stx] job_status="+status)
	case "log_notice", "warning":
		msg := strings.TrimSpace(event.Message)
		if msg == "" {
			return nil
		}
		return writePlainLine(w.writer, "[stx] warn: "+msg)
	case "error":
		msg := strings.TrimSpace(event.Message)
		if event.Code != "" {
			msg = strings.TrimSpace(event.Code + ": " + msg)
		}
		if msg == "" {
			msg = "error"
		}
		return writePlainLine(w.writer, "[stx] error: "+msg)
	case "job_observation_lost":
		msg := strings.TrimSpace(event.Message)
		if msg == "" {
			msg = "engine observation lost"
		}
		return writePlainLine(w.writer, "[stx] error: "+msg)
	default:
		msg := strings.TrimSpace(event.Message)
		if msg == "" {
			return nil
		}
		if name != "" {
			return writePlainLine(w.writer, "[stx] "+name+": "+msg)
		}
		return writePlainLine(w.writer, msg)
	}
}

func writePlainLine(w io.Writer, line string) error {
	line = strings.TrimRight(line, "\r\n")
	_, err := io.WriteString(w, line+"\n")
	return err
}

func writePlainBlock(w io.Writer, block string) error {
	block = strings.ReplaceAll(block, "\r\n", "\n")
	block = strings.ReplaceAll(block, "\r", "\n")
	if strings.TrimSpace(block) == "" {
		return nil
	}
	for _, line := range strings.Split(block, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if err := writePlainLine(w, line); err != nil {
			return err
		}
	}
	return nil
}

// EmitError 写入最终错误事件。
// EmitError writes the final error event.
func (w *EventWriter) EmitError(err error) error {
	classified := ClassifyError(err)
	retryable := classified.Retryable
	return w.Emit(Event{
		Event:     "error",
		Code:      classified.Code,
		Message:   classified.Message,
		Retryable: &retryable,
		RequestID: classified.RequestID,
	})
}
