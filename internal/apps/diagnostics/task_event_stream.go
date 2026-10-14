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
	"sync"
	"time"

	"github.com/LeonYoah/stx/internal/pkg/timex"
)

// DiagnosticTaskEventType represents the event type in diagnostics task streaming.
// DiagnosticTaskEventType 表示诊断任务流中的事件类型。
type DiagnosticTaskEventType string

const (
	DiagnosticTaskEventTypeSnapshot    DiagnosticTaskEventType = "snapshot"
	DiagnosticTaskEventTypeTaskUpdated DiagnosticTaskEventType = "task_updated"
	DiagnosticTaskEventTypeStepUpdated DiagnosticTaskEventType = "step_updated"
	DiagnosticTaskEventTypeNodeUpdated DiagnosticTaskEventType = "node_updated"
	DiagnosticTaskEventTypeLogAppended DiagnosticTaskEventType = "log_appended"
)

// DiagnosticTaskEvent describes an incremental diagnostics task event for the frontend.
// DiagnosticTaskEvent 描述流向前端的诊断任务增量事件。
type DiagnosticTaskEvent struct {
	TaskID          uint                    `json:"task_id"`
	EventType       DiagnosticTaskEventType `json:"event_type"`
	Timestamp       time.Time               `json:"timestamp"`
	TaskStatus      DiagnosticTaskStatus    `json:"task_status,omitempty"`
	StepStatus      DiagnosticTaskStatus    `json:"step_status,omitempty"`
	NodeStatus      DiagnosticTaskStatus    `json:"node_status,omitempty"`
	CurrentStep     DiagnosticStepCode      `json:"current_step,omitempty"`
	StepID          *uint                   `json:"step_id,omitempty"`
	StepCode        DiagnosticStepCode      `json:"step_code,omitempty"`
	NodeExecutionID *uint                   `json:"node_execution_id,omitempty"`
	HostID          uint                    `json:"host_id,omitempty"`
	HostName        string                  `json:"host_name,omitempty"`
	Level           DiagnosticLogLevel      `json:"level,omitempty"`
	LogEventType    DiagnosticLogEventType  `json:"log_event_type,omitempty"`
	Message         string                  `json:"message,omitempty"`
	Error           string                  `json:"error,omitempty"`
	CommandSummary  string                  `json:"command_summary,omitempty"`
	FailureReason   string                  `json:"failure_reason,omitempty"`
}

type diagnosticTaskEventHub struct {
	mu          sync.RWMutex
	subscribers map[uint]map[chan DiagnosticTaskEvent]struct{}
}

func newDiagnosticTaskEventHub() *diagnosticTaskEventHub {
	return &diagnosticTaskEventHub{
		subscribers: make(map[uint]map[chan DiagnosticTaskEvent]struct{}),
	}
}

func (h *diagnosticTaskEventHub) Subscribe(taskID uint) (<-chan DiagnosticTaskEvent, func()) {
	ch := make(chan DiagnosticTaskEvent, 128)

	h.mu.Lock()
	if _, ok := h.subscribers[taskID]; !ok {
		h.subscribers[taskID] = make(map[chan DiagnosticTaskEvent]struct{})
	}
	h.subscribers[taskID][ch] = struct{}{}
	h.mu.Unlock()

	return ch, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if taskSubscribers, ok := h.subscribers[taskID]; ok {
			if _, exists := taskSubscribers[ch]; exists {
				delete(taskSubscribers, ch)
				close(ch)
			}
			if len(taskSubscribers) == 0 {
				delete(h.subscribers, taskID)
			}
		}
	}
}

func (h *diagnosticTaskEventHub) Publish(event DiagnosticTaskEvent) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	taskSubscribers := h.subscribers[event.TaskID]
	for ch := range taskSubscribers {
		select {
		case ch <- event:
		default:
		}
	}
}

func (s *Service) ensureDiagnosticTaskEventHub() *diagnosticTaskEventHub {
	if s == nil {
		return nil
	}
	s.taskEventsOnce.Do(func() {
		if s.taskEvents == nil {
			s.taskEvents = newDiagnosticTaskEventHub()
		}
	})
	return s.taskEvents
}

// SubscribeDiagnosticTaskEvents subscribes to one diagnostics task event stream.
// SubscribeDiagnosticTaskEvents 订阅单个诊断任务的事件流。
func (s *Service) SubscribeDiagnosticTaskEvents(taskID uint) (<-chan DiagnosticTaskEvent, func()) {
	if s == nil {
		ch := make(chan DiagnosticTaskEvent)
		close(ch)
		return ch, func() {}
	}
	hub := s.ensureDiagnosticTaskEventHub()
	if hub == nil {
		ch := make(chan DiagnosticTaskEvent)
		close(ch)
		return ch, func() {}
	}
	return hub.Subscribe(taskID)
}

func (s *Service) publishDiagnosticTaskEvent(event DiagnosticTaskEvent) {
	hub := s.ensureDiagnosticTaskEventHub()
	if hub == nil {
		return
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = timex.Now()
	}
	hub.Publish(event)
}

func newDiagnosticTaskSnapshotEvent(task *DiagnosticTask) DiagnosticTaskEvent {
	return DiagnosticTaskEvent{
		TaskID:        task.ID,
		EventType:     DiagnosticTaskEventTypeSnapshot,
		TaskStatus:    task.Status,
		CurrentStep:   task.CurrentStep,
		Message:       task.Summary,
		FailureReason: task.FailureReason,
	}
}

func newDiagnosticTaskUpdatedEvent(task *DiagnosticTask) DiagnosticTaskEvent {
	return DiagnosticTaskEvent{
		TaskID:        task.ID,
		EventType:     DiagnosticTaskEventTypeTaskUpdated,
		TaskStatus:    task.Status,
		CurrentStep:   task.CurrentStep,
		Message:       task.Summary,
		FailureReason: task.FailureReason,
	}
}

func newDiagnosticStepUpdatedEvent(step *DiagnosticTaskStep) DiagnosticTaskEvent {
	return DiagnosticTaskEvent{
		TaskID:     step.TaskID,
		EventType:  DiagnosticTaskEventTypeStepUpdated,
		StepID:     uintPtr(step.ID),
		StepCode:   step.Code,
		StepStatus: step.Status,
		Message:    step.Message,
		Error:      step.Error,
	}
}

func newDiagnosticNodeUpdatedEvent(node *DiagnosticNodeExecution) DiagnosticTaskEvent {
	return DiagnosticTaskEvent{
		TaskID:          node.TaskID,
		EventType:       DiagnosticTaskEventTypeNodeUpdated,
		NodeExecutionID: uintPtr(node.ID),
		NodeStatus:      node.Status,
		CurrentStep:     node.CurrentStep,
		StepID:          node.TaskStepID,
		HostID:          node.HostID,
		HostName:        node.HostName,
		Message:         node.Message,
		Error:           node.Error,
	}
}

func newDiagnosticLogAppendedEvent(log *DiagnosticStepLog) DiagnosticTaskEvent {
	return DiagnosticTaskEvent{
		TaskID:          log.TaskID,
		EventType:       DiagnosticTaskEventTypeLogAppended,
		StepID:          log.TaskStepID,
		StepCode:        log.StepCode,
		NodeExecutionID: log.NodeExecutionID,
		Level:           log.Level,
		LogEventType:    log.EventType,
		Message:         log.Message,
		CommandSummary:  log.CommandSummary,
	}
}

func uintPtr(value uint) *uint {
	v := value
	return &v
}
