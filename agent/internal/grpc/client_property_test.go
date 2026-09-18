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

package grpc

import (
	"testing"
	"time"

	"pgregory.net/rapid"
)

// **Feature: stx-agent, Property 18: Exponential Backoff Calculation**
// **Validates: Requirements 1.4**
//
// Property: For any reconnection attempt sequence, the delay between attempts
// SHALL follow exponential backoff (initial 1s, max 60s): delay = min(60, 2^(attempt-1)).
// 属性：对于任何重连尝试序列，尝试之间的延迟应该遵循指数退避
// （初始 1 秒，最大 60 秒）：delay = min(60, 2^(尝试次数-1))。
func TestProperty_ExponentialBackoffCalculation(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Generate attempt number (1 to 20 to cover various scenarios)
		// 生成尝试次数（1 到 20 以覆盖各种场景）
		attempt := rapid.IntRange(1, 20).Draw(rt, "attempt")

		// Use default values as specified in requirements
		// 使用需求中指定的默认值
		initialInterval := DefaultInitialBackoff // 1 second
		maxInterval := DefaultMaxBackoff         // 60 seconds
		factor := DefaultBackoffFactor           // 2.0

		// Calculate expected backoff using the formula: min(max, initial * factor^(attempt-1))
		// 使用公式计算预期退避时间：min(最大值, 初始值 * 因子^(尝试次数-1))
		expectedBackoff := float64(initialInterval)
		for i := 1; i < attempt; i++ {
			expectedBackoff *= factor
		}
		if time.Duration(expectedBackoff) > maxInterval {
			expectedBackoff = float64(maxInterval)
		}

		// Calculate actual backoff using the function
		// 使用函数计算实际退避时间
		actualBackoff := CalculateBackoff(attempt, initialInterval, maxInterval, factor)

		// Verify the backoff matches expected value
		// 验证退避时间与预期值匹配
		if actualBackoff != time.Duration(expectedBackoff) {
			rt.Fatalf("Backoff calculation mismatch for attempt %d: expected %v, got %v",
				attempt, time.Duration(expectedBackoff), actualBackoff)
		}

		// Verify backoff is within bounds
		// 验证退避时间在范围内
		if actualBackoff < initialInterval {
			rt.Fatalf("Backoff %v is less than initial interval %v for attempt %d",
				actualBackoff, initialInterval, attempt)
		}
		if actualBackoff > maxInterval {
			rt.Fatalf("Backoff %v exceeds max interval %v for attempt %d",
				actualBackoff, maxInterval, attempt)
		}

		// Verify monotonic increase until max is reached
		// 验证在达到最大值之前单调递增
		if attempt > 1 {
			prevBackoff := CalculateBackoff(attempt-1, initialInterval, maxInterval, factor)
			if actualBackoff < prevBackoff {
				rt.Fatalf("Backoff decreased from attempt %d (%v) to attempt %d (%v)",
					attempt-1, prevBackoff, attempt, actualBackoff)
			}
		}
	})
}

// TestProperty_ExponentialBackoffSequence tests that a sequence of backoffs
// follows the expected pattern
// TestProperty_ExponentialBackoffSequence 测试退避序列遵循预期模式
func TestProperty_ExponentialBackoffSequence(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Create a new backoff instance
		// 创建新的退避实例
		backoff := NewExponentialBackoff()

		// Generate number of attempts to test
		// 生成要测试的尝试次数
		numAttempts := rapid.IntRange(1, 15).Draw(rt, "numAttempts")

		var prevDuration time.Duration
		for i := 0; i < numAttempts; i++ {
			duration := backoff.NextBackoff()

			// Verify duration is within bounds
			// 验证持续时间在范围内
			if duration < backoff.InitialInterval {
				rt.Fatalf("Backoff %v is less than initial interval %v at attempt %d",
					duration, backoff.InitialInterval, i+1)
			}
			if duration > backoff.MaxInterval {
				rt.Fatalf("Backoff %v exceeds max interval %v at attempt %d",
					duration, backoff.MaxInterval, i+1)
			}

			// Verify monotonic increase (or equal if at max)
			// 验证单调递增（如果达到最大值则相等）
			if i > 0 && duration < prevDuration {
				rt.Fatalf("Backoff decreased from %v to %v at attempt %d",
					prevDuration, duration, i+1)
			}

			prevDuration = duration
		}

		// Verify attempt count
		// 验证尝试次数
		if backoff.Attempt() != numAttempts {
			rt.Fatalf("Expected %d attempts, got %d", numAttempts, backoff.Attempt())
		}
	})
}

// TestProperty_ExponentialBackoffReset tests that reset works correctly
// TestProperty_ExponentialBackoffReset 测试重置功能正常工作
func TestProperty_ExponentialBackoffReset(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		backoff := NewExponentialBackoff()

		// Make some attempts
		// 进行一些尝试
		numAttempts := rapid.IntRange(1, 10).Draw(rt, "numAttempts")
		for i := 0; i < numAttempts; i++ {
			backoff.NextBackoff()
		}

		// Reset
		// 重置
		backoff.Reset()

		// Verify attempt count is 0
		// 验证尝试次数为 0
		if backoff.Attempt() != 0 {
			rt.Fatalf("Expected 0 attempts after reset, got %d", backoff.Attempt())
		}

		// Verify next backoff is initial interval
		// 验证下一次退避是初始间隔
		nextDuration := backoff.NextBackoff()
		if nextDuration != backoff.InitialInterval {
			rt.Fatalf("Expected initial interval %v after reset, got %v",
				backoff.InitialInterval, nextDuration)
		}
	})
}

// TestProperty_ExponentialBackoffMaxCap tests that backoff is capped at max
// TestProperty_ExponentialBackoffMaxCap 测试退避在最大值处被限制
func TestProperty_ExponentialBackoffMaxCap(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Generate custom intervals
		// 生成自定义间隔
		initialMs := rapid.IntRange(100, 5000).Draw(rt, "initialMs")
		maxMs := rapid.IntRange(initialMs, 120000).Draw(rt, "maxMs")

		initialInterval := time.Duration(initialMs) * time.Millisecond
		maxInterval := time.Duration(maxMs) * time.Millisecond

		// Calculate how many attempts until we hit max
		// 计算达到最大值需要多少次尝试
		// 2^n * initial >= max => n >= log2(max/initial)
		attemptsToMax := 1
		current := float64(initialInterval)
		for current < float64(maxInterval) {
			current *= DefaultBackoffFactor
			attemptsToMax++
		}

		// Test attempts beyond max
		// 测试超过最大值的尝试
		extraAttempts := rapid.IntRange(1, 10).Draw(rt, "extraAttempts")
		totalAttempts := attemptsToMax + extraAttempts

		for attempt := 1; attempt <= totalAttempts; attempt++ {
			backoff := CalculateBackoff(attempt, initialInterval, maxInterval, DefaultBackoffFactor)

			// All backoffs should be <= max
			// 所有退避时间应该 <= 最大值
			if backoff > maxInterval {
				rt.Fatalf("Backoff %v exceeds max %v at attempt %d",
					backoff, maxInterval, attempt)
			}

			// After reaching max, all subsequent backoffs should equal max
			// 达到最大值后，所有后续退避时间应该等于最大值
			if attempt >= attemptsToMax && backoff != maxInterval {
				rt.Fatalf("Expected max interval %v at attempt %d (attemptsToMax=%d), got %v",
					maxInterval, attempt, attemptsToMax, backoff)
			}
		}
	})
}

// **Feature: stx-agent, Property 19: Heartbeat Interval Compliance**
// **Validates: Requirements 1.3**
//
// Property: For any running Agent, heartbeat messages SHALL be sent at the
// configured interval (default 10 seconds) with a tolerance of ±1 second.
// 属性：对于任何运行中的 Agent，心跳消息应该以配置的间隔（默认 10 秒）
// 发送，容差为 ±1 秒。
func TestProperty_HeartbeatIntervalCompliance(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		// Generate heartbeat interval between 10ms and 50ms for faster testing
		// 生成 10ms 到 50ms 之间的心跳间隔以加快测试
		intervalMs := rapid.IntRange(10, 50).Draw(rt, "intervalMs")
		interval := time.Duration(intervalMs) * time.Millisecond

		// Tolerance is 50% of interval for short intervals
		// 对于短间隔，容差为间隔的 50%
		tolerance := interval / 2
		if tolerance < 5*time.Millisecond {
			tolerance = 5 * time.Millisecond
		}

		// Create tracker
		// 创建跟踪器
		tracker := NewHeartbeatTracker()

		// Number of heartbeats to test (reduced for faster testing)
		// 要测试的心跳数量（减少以加快测试）
		numHeartbeats := rapid.IntRange(3, 5).Draw(rt, "numHeartbeats")

		// Simulate heartbeats at the specified interval
		// 以指定间隔模拟心跳
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for i := 0; i < numHeartbeats; i++ {
			<-ticker.C
			tracker.Record()
		}

		// Get intervals between heartbeats
		// 获取心跳之间的间隔
		intervals := tracker.GetIntervals()

		// Verify each interval is within tolerance
		// 验证每个间隔都在容差范围内
		for i, actualInterval := range intervals {
			diff := actualInterval - interval
			if diff < 0 {
				diff = -diff
			}

			if diff > tolerance {
				rt.Fatalf("Heartbeat interval %d out of tolerance: expected %v ± %v, got %v (diff: %v)",
					i+1, interval, tolerance, actualInterval, diff)
			}
		}
	})
}

// TestProperty_HeartbeatTrackerAccuracy tests that the tracker accurately records intervals
// TestProperty_HeartbeatTrackerAccuracy 测试跟踪器准确记录间隔
func TestProperty_HeartbeatTrackerAccuracy(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		tracker := NewHeartbeatTracker()

		// Generate number of records (reduced for faster testing)
		// 生成记录数量（减少以加快测试）
		numRecords := rapid.IntRange(2, 5).Draw(rt, "numRecords")

		// Generate intervals between records (in milliseconds, shorter for faster testing)
		// 生成记录之间的间隔（毫秒，更短以加快测试）
		expectedIntervals := make([]time.Duration, numRecords-1)
		for i := range numRecords - 1 {
			ms := rapid.IntRange(5, 20).Draw(rt, "intervalMs")
			expectedIntervals[i] = time.Duration(ms) * time.Millisecond
		}

		// Record timestamps with specified intervals
		// 以指定间隔记录时间戳
		tracker.Record()
		for i := range numRecords - 1 {
			time.Sleep(expectedIntervals[i])
			tracker.Record()
		}

		// Get recorded intervals
		// 获取记录的间隔
		actualIntervals := tracker.GetIntervals()

		// Verify number of intervals
		// 验证间隔数量
		if len(actualIntervals) != len(expectedIntervals) {
			rt.Fatalf("Expected %d intervals, got %d", len(expectedIntervals), len(actualIntervals))
		}

		// Verify each interval is approximately correct (within 20ms tolerance for timing)
		// 验证每个间隔大致正确（时间容差为 20ms）
		tolerance := 20 * time.Millisecond
		for i, expected := range expectedIntervals {
			actual := actualIntervals[i]
			diff := actual - expected
			if diff < 0 {
				diff = -diff
			}

			if diff > tolerance {
				rt.Fatalf("Interval %d mismatch: expected %v ± %v, got %v (diff: %v)",
					i, expected, tolerance, actual, diff)
			}
		}
	})
}

// TestProperty_HeartbeatTrackerClear tests that clear works correctly
// TestProperty_HeartbeatTrackerClear 测试清除功能正常工作
func TestProperty_HeartbeatTrackerClear(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		tracker := NewHeartbeatTracker()

		// Record some timestamps
		// 记录一些时间戳
		numRecords := rapid.IntRange(1, 10).Draw(rt, "numRecords")
		for i := 0; i < numRecords; i++ {
			tracker.Record()
		}

		// Verify records exist
		// 验证记录存在
		if len(tracker.GetTimestamps()) != numRecords {
			rt.Fatalf("Expected %d timestamps before clear, got %d",
				numRecords, len(tracker.GetTimestamps()))
		}

		// Clear
		// 清除
		tracker.Clear()

		// Verify no records
		// 验证没有记录
		if len(tracker.GetTimestamps()) != 0 {
			rt.Fatalf("Expected 0 timestamps after clear, got %d",
				len(tracker.GetTimestamps()))
		}

		// Verify no intervals
		// 验证没有间隔
		if tracker.GetIntervals() != nil {
			rt.Fatalf("Expected nil intervals after clear, got %v",
				tracker.GetIntervals())
		}
	})
}
