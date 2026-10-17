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
	"io"
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

// EventWriter 以并发安全方式写入 NDJSON 事件。
// EventWriter writes NDJSON events safely across concurrent callers.
type EventWriter struct {
	writer io.Writer
	mu     sync.Mutex
}

// NewEventWriter 创建 stderr 事件写入器。
// NewEventWriter creates a stderr event writer.
func NewEventWriter(writer io.Writer) *EventWriter {
	return &EventWriter{writer: writer}
}

// Emit 写入一个完整 JSON 对象并追加换行。
// Emit writes one complete JSON object followed by a newline.
func (w *EventWriter) Emit(event Event) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return json.NewEncoder(w.writer).Encode(event)
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
