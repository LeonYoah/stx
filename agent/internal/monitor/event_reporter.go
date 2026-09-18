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

package monitor

import (
	"context"
	"sync"
	"time"

	"github.com/LeonYoah/stx/agent/internal/logger"
)

// DefaultEventCacheSize is the default size of the event cache
// DefaultEventCacheSize 是事件缓存的默认大小
const DefaultEventCacheSize = 1000

// DefaultBatchSize is the default batch size for event reporting
// DefaultBatchSize 是事件上报的默认批量大小
const DefaultBatchSize = 100

// EventReportFunc is the function type for reporting events
// EventReportFunc 是上报事件的函数类型
type EventReportFunc func(events []*ProcessEvent) error

// EventReporter handles event caching and batch reporting
// EventReporter 处理事件缓存和批量上报
// Requirements 3.3, 3.4, 7.6: Generate events, report to Control Plane, cache during offline
// 需求 3.3, 3.4, 7.6：生成事件、上报到 Control Plane、离线时缓存
type EventReporter struct {
	eventCache    []*ProcessEvent
	cacheSize     int
	batchSize     int
	reportFunc    EventReportFunc
	isConnected   bool
	mu            sync.Mutex
	flushInterval time.Duration
	stopCh        chan struct{}
}

// NewEventReporter creates a new EventReporter instance
// NewEventReporter 创建一个新的 EventReporter 实例
func NewEventReporter(reportFunc EventReportFunc) *EventReporter {
	return &EventReporter{
		eventCache:    make([]*ProcessEvent, 0, DefaultEventCacheSize),
		cacheSize:     DefaultEventCacheSize,
		batchSize:     DefaultBatchSize,
		reportFunc:    reportFunc,
		isConnected:   true,
		flushInterval: 10 * time.Second,
		stopCh:        make(chan struct{}),
	}
}

// SetCacheSize sets the maximum cache size
// SetCacheSize 设置最大缓存大小
func (r *EventReporter) SetCacheSize(size int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cacheSize = size
}

// SetBatchSize sets the batch size for reporting
// SetBatchSize 设置上报的批量大小
func (r *EventReporter) SetBatchSize(size int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.batchSize = size
}

// SetReportFunc sets the report function (e.g. after gRPC connection is ready).
// SetReportFunc 设置上报函数（例如在 gRPC 连接就绪后）。
func (r *EventReporter) SetReportFunc(fn EventReportFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reportFunc = fn
}

// SetConnected sets the connection status
// SetConnected 设置连接状态
func (r *EventReporter) SetConnected(connected bool) {
	r.mu.Lock()
	wasDisconnected := !r.isConnected
	r.isConnected = connected
	r.mu.Unlock()

	// If reconnected, try to flush cached events
	// 如果重新连接，尝试刷新缓存的事件
	if connected && wasDisconnected {
		ctx := context.Background()
		logger.InfoF(ctx, "[EventReporter] Connection restored, flushing cached events / 连接恢复，刷新缓存的事件")
		go r.FlushEvents()
	}
}

// Start starts the periodic flush goroutine
// Start 启动定期刷新 goroutine
func (r *EventReporter) Start() {
	go r.flushLoop()
}

// Stop stops the event reporter
// Stop 停止事件上报器
func (r *EventReporter) Stop() {
	close(r.stopCh)
}

// flushLoop periodically flushes events
// flushLoop 定期刷新事件
func (r *EventReporter) flushLoop() {
	ticker := time.NewTicker(r.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-r.stopCh:
			return
		case <-ticker.C:
			r.FlushEvents()
		}
	}
}

// ReportEvent adds an event to the cache and attempts to report
// ReportEvent 将事件添加到缓存并尝试上报
// Requirements 3.3, 3.4: Generate and report process events
// 需求 3.3, 3.4：生成并上报进程事件
func (r *EventReporter) ReportEvent(event *ProcessEvent) {
	ctx := context.Background()
	r.mu.Lock()
	defer r.mu.Unlock()

	// Add to cache / 添加到缓存
	if len(r.eventCache) >= r.cacheSize {
		// Remove oldest event if cache is full / 如果缓存已满则移除最旧的事件
		r.eventCache = r.eventCache[1:]
	}
	r.eventCache = append(r.eventCache, event)

	logger.InfoF(ctx, "[EventReporter] Event cached: type=%s, name=%s, pid=%d / 事件已缓存：类型=%s，名称=%s，PID=%d",
		event.Type, event.Name, event.PID, event.Type, event.Name, event.PID)

	// Try to report immediately if connected and batch size reached
	// 如果已连接且达到批量大小，尝试立即上报
	if r.isConnected && len(r.eventCache) >= r.batchSize {
		go r.FlushEvents()
	}
}

// FlushEvents attempts to report all cached events
// FlushEvents 尝试上报所有缓存的事件
// Requirements 7.6: Batch report cached events when connection restored
// 需求 7.6：连接恢复后批量上报缓存的事件
func (r *EventReporter) FlushEvents() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.flushEventsLocked()
}

// flushEventsLocked flushes events (must be called with lock held)
// flushEventsLocked 刷新事件（必须在持有锁的情况下调用）
func (r *EventReporter) flushEventsLocked() {
	ctx := context.Background()
	if len(r.eventCache) == 0 {
		return
	}

	if !r.isConnected {
		ctx := context.Background()
		logger.WarnF(ctx, "[EventReporter] Not connected, keeping %d events in cache / 未连接，保留 %d 个事件在缓存中",
			len(r.eventCache), len(r.eventCache))
		return
	}

	if r.reportFunc == nil {
		ctx := context.Background()
		logger.WarnF(ctx, "[EventReporter] No report function set / 未设置上报函数")
		return
	}

	// Report in batches / 批量上报
	for len(r.eventCache) > 0 {
		batchEnd := r.batchSize
		if batchEnd > len(r.eventCache) {
			batchEnd = len(r.eventCache)
		}

		batch := r.eventCache[:batchEnd]
		err := r.reportFunc(batch)
		if err != nil {
			logger.ErrorF(ctx, "[EventReporter] Failed to report events: %v / 上报事件失败：%v", err, err)
			// Keep events in cache for retry / 保留事件在缓存中以便重试
			return
		}

		// Remove reported events from cache / 从缓存中移除已上报的事件
		r.eventCache = r.eventCache[batchEnd:]
		logger.InfoF(ctx, "[EventReporter] Reported %d events, %d remaining / 上报了 %d 个事件，剩余 %d 个",
			batchEnd, len(r.eventCache), batchEnd, len(r.eventCache))
	}
}

// GetCachedEventCount returns the number of cached events
// GetCachedEventCount 返回缓存的事件数量
func (r *EventReporter) GetCachedEventCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.eventCache)
}

// ClearCache clears all cached events
// ClearCache 清除所有缓存的事件
func (r *EventReporter) ClearCache() {
	ctx := context.Background()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.eventCache = make([]*ProcessEvent, 0, r.cacheSize)
	logger.InfoF(ctx, "[EventReporter] Cache cleared / 缓存已清除")
}
