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
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/LeonYoah/stx/agent/internal/config"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestIsTransportError(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "unavailable status", err: status.Error(codes.Unavailable, "conn down"), want: true},
		{name: "wrapped connection refused", err: fmt.Errorf("heartbeat failed: %w", errors.New("dial tcp 127.0.0.1:17890: connection refused")), want: true},
		{name: "transport is closing", err: errors.New("rpc error: code = Unavailable desc = transport is closing"), want: true},
		{name: "not found business", err: status.Error(codes.NotFound, "agent not registered"), want: false},
		{name: "invalid argument", err: status.Error(codes.InvalidArgument, "bad req"), want: false},
		{name: "generic message", err: errors.New("something else"), want: false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := IsTransportError(tc.err)
			if got != tc.want {
				t.Fatalf("IsTransportError(%v)=%v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestMarkDisconnectedClearsConnectedFlag(t *testing.T) {
	t.Parallel()

	c := &Client{
		stopCh:    make(chan struct{}),
		backoff:   NewExponentialBackoff(),
		connected: true,
	}
	c.MarkDisconnected()
	if c.IsConnected() {
		t.Fatal("expected IsConnected() false after MarkDisconnected")
	}
}

func TestTryReconnectSingleFlight(t *testing.T) {
	t.Parallel()

	c := &Client{
		stopCh:  make(chan struct{}),
		backoff: NewExponentialBackoff(),
	}
	// 人为占用重连锁，验证第二次调用立即返回 ErrReconnectInProgress。
	// Hold the reconnect lock so the second call returns ErrReconnectInProgress immediately.
	c.reconnectMu.Lock()
	c.reconnecting = true
	c.reconnectMu.Unlock()

	err := c.TryReconnect(context.Background())
	if !errors.Is(err, ErrReconnectInProgress) {
		t.Fatalf("expected ErrReconnectInProgress, got %v", err)
	}
}

func TestTryReconnectReleasesFlagAfterCancel(t *testing.T) {
	t.Parallel()

	c := &Client{
		config: &config.Config{
			ControlPlane: config.ControlPlaneConfig{
				// 不可达地址，确保 Connect 失败从而停在退避循环里。
				// Unreachable address so Connect fails and stays in the backoff loop.
				Addresses: []string{"127.0.0.1:1"},
			},
		},
		stopCh:  make(chan struct{}),
		backoff: NewExponentialBackoff(),
	}
	ctx, cancel := context.WithCancel(context.Background())

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = c.TryReconnect(ctx)
	}()

	// 等单飞标志置位 / wait until single-flight flag is set
	deadline := time.Now().Add(2 * time.Second)
	for {
		c.reconnectMu.Lock()
		busy := c.reconnecting
		c.reconnectMu.Unlock()
		if busy || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	c.reconnectMu.Lock()
	busy := c.reconnecting
	c.reconnectMu.Unlock()
	if !busy {
		cancel()
		wg.Wait()
		t.Fatal("expected reconnecting=true while TryReconnect is running")
	}

	cancel()
	wg.Wait()

	c.reconnectMu.Lock()
	busy = c.reconnecting
	c.reconnectMu.Unlock()
	if busy {
		t.Fatal("expected reconnecting=false after TryReconnect returns")
	}
}
