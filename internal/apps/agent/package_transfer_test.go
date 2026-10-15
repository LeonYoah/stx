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

package agent

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pb "github.com/LeonYoah/stx/internal/proto/agent"
	"google.golang.org/grpc"
)

func writeTempPackage(t *testing.T, size int) (string, []byte) {
	t.Helper()
	data := make([]byte, size)
	if _, err := rand.Read(data); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "apache-seatunnel-9.9.9-bin.tar.gz")
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p, data
}

func collect(t *testing.T, m *Manager, req *pb.DownloadPackageRequest) ([]byte, error) {
	t.Helper()
	var buf bytes.Buffer
	next := req.GetOffset()
	err := m.StreamPackage(context.Background(), req, func(c *pb.PackageChunk) error {
		if c.Offset != next {
			t.Fatalf("chunk offset %d, want %d", c.Offset, next)
		}
		if len(c.Data) > PackageStreamChunkSize {
			t.Fatalf("chunk too large: %d", len(c.Data))
		}
		next += int64(len(c.Data))
		buf.Write(c.Data)
		return nil
	})
	return buf.Bytes(), err
}

// TestStreamPackageFullAndResume 验证完整下载与按偏移续传。
// TestStreamPackageFullAndResume verifies full streaming and resume-from-offset.
func TestStreamPackageFullAndResume(t *testing.T) {
	m := NewManager(nil)
	path, data := writeTempPackage(t, 3*PackageStreamChunkSize+123)
	ps, err := m.RegisterPackageServe("agent-1", path, "sum")
	if err != nil {
		t.Fatal(err)
	}

	got, err := collect(t, m, &pb.DownloadPackageRequest{AgentId: "agent-1", TransferId: ps.ID})
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("full stream mismatch: err=%v len=%d", err, len(got))
	}
	if ps.Served() != int64(len(data)) {
		t.Fatalf("served=%d want %d", ps.Served(), len(data))
	}

	off := int64(PackageStreamChunkSize + 7)
	got, err = collect(t, m, &pb.DownloadPackageRequest{AgentId: "agent-1", TransferId: ps.ID, Offset: off})
	if err != nil || !bytes.Equal(got, data[off:]) {
		t.Fatalf("resume mismatch: err=%v len=%d", err, len(got))
	}
}

// TestStreamPackageRejectsUnknownOrForeign 验证未知传输或其他 Agent 被拒绝。
// TestStreamPackageRejectsUnknownOrForeign verifies unknown ids and foreign agents are rejected.
func TestStreamPackageRejectsUnknownOrForeign(t *testing.T) {
	m := NewManager(nil)
	path, _ := writeTempPackage(t, 10)
	ps, _ := m.RegisterPackageServe("agent-1", path, "")

	if _, err := collect(t, m, &pb.DownloadPackageRequest{AgentId: "agent-1", TransferId: "nope"}); !errors.Is(err, ErrPackageServeNotFound) {
		t.Fatalf("unknown id: got %v", err)
	}
	if _, err := collect(t, m, &pb.DownloadPackageRequest{AgentId: "agent-2", TransferId: ps.ID}); !errors.Is(err, ErrPackageServeNotFound) {
		t.Fatalf("foreign agent: got %v", err)
	}
	if _, err := collect(t, m, &pb.DownloadPackageRequest{AgentId: "agent-1", TransferId: ps.ID, Offset: 11}); err == nil {
		t.Fatal("expected invalid offset error")
	}
	m.ReleasePackageServe(ps.ID)
	if _, err := collect(t, m, &pb.DownloadPackageRequest{AgentId: "agent-1", TransferId: ps.ID}); !errors.Is(err, ErrPackageServeNotFound) {
		t.Fatalf("released id: got %v", err)
	}
}

// fakeAgentStream simulates an Agent: on TRANSFER_PACKAGE it pulls via StreamPackage and replies.
// fakeAgentStream 模拟 Agent：收到 TRANSFER_PACKAGE 后通过 StreamPackage 拉取并回复。
type fakeAgentStream struct {
	grpc.BidiStreamingServer[pb.CommandResponse, pb.CommandRequest]
	m       *Manager
	agentID string
	stall   bool
	got     chan []byte
}

func (f *fakeAgentStream) Send(req *pb.CommandRequest) error {
	if req.Type != pb.CommandType_TRANSFER_PACKAGE || req.Parameters["mode"] != "pull" {
		return errors.New("unexpected command")
	}
	go func() {
		if f.stall {
			return // never pulls / 从不拉取
		}
		var buf bytes.Buffer
		err := f.m.StreamPackage(context.Background(), &pb.DownloadPackageRequest{
			AgentId: f.agentID, TransferId: req.Parameters["transfer_id"],
		}, func(c *pb.PackageChunk) error { buf.Write(c.Data); return nil })
		f.got <- buf.Bytes()
		resp := &pb.CommandResponse{CommandId: req.CommandId, Status: pb.CommandStatus_SUCCESS}
		if err != nil {
			resp.Status, resp.Error = pb.CommandStatus_FAILED, err.Error()
		} else {
			out, _ := json.Marshal(map[string]any{"success": true, "local_path": "/agent/pkgs/" + req.Parameters["file_name"]})
			resp.Output = string(out)
		}
		f.m.HandleCommandResponse(resp)
	}()
	return nil
}

func registerFakeAgent(t *testing.T, m *Manager, id string, s *fakeAgentStream) {
	t.Helper()
	if _, err := m.RegisterAgent(context.Background(), &pb.RegisterRequest{AgentId: id, Hostname: "h", IpAddress: "127.0.0.1"}); err != nil {
		t.Fatal(err)
	}
	if err := m.SetAgentStream(id, s); err != nil {
		t.Fatal(err)
	}
}

// TestTransferPackageEndToEnd 验证 TransferPackage 驱动 Agent 拉取并返回远端路径。
// TestTransferPackageEndToEnd verifies TransferPackage drives the Agent pull and returns the remote path.
func TestTransferPackageEndToEnd(t *testing.T) {
	m := NewManager(nil)
	s := &fakeAgentStream{m: m, agentID: "agent-1", got: make(chan []byte, 1)}
	registerFakeAgent(t, m, "agent-1", s)
	path, data := writeTempPackage(t, 2*PackageStreamChunkSize+5)

	var lastSent int64
	res, err := m.TransferPackage(context.Background(), "agent-1", "9.9.9", path, "sum", PackagePullOptions{
		PollInterval: 10 * time.Millisecond,
		OnProgress:   func(sent, total int64) { lastSent = sent },
	})
	if err != nil {
		t.Fatalf("TransferPackage: %v", err)
	}
	if res.LocalPath != "/agent/pkgs/apache-seatunnel-9.9.9-bin.tar.gz" || res.Reused {
		t.Fatalf("result=%+v", res)
	}
	if got := <-s.got; !bytes.Equal(got, data) {
		t.Fatal("agent received different bytes")
	}
	if lastSent != int64(len(data)) {
		t.Fatalf("progress=%d want %d", lastSent, len(data))
	}
	// Transfer id must be revoked afterwards. / 完成后传输标识必须被撤销。
	n := 0
	m.packageServes.Range(func(_, _ any) bool { n++; return true })
	if n != 0 {
		t.Fatalf("expected serve released, %d left", n)
	}
}

// TestTransferPackageStallDetected 验证 Agent 不拉取数据时快速判定停滞而非等待超长超时。
// TestTransferPackageStallDetected verifies a stalled pull fails fast instead of waiting a long timeout.
func TestTransferPackageStallDetected(t *testing.T) {
	m := NewManager(nil)
	registerFakeAgent(t, m, "agent-1", &fakeAgentStream{m: m, agentID: "agent-1", stall: true, got: make(chan []byte, 1)})
	path, _ := writeTempPackage(t, 100)

	start := time.Now()
	_, err := m.TransferPackage(context.Background(), "agent-1", "9.9.9", path, "", PackagePullOptions{
		PollInterval: 10 * time.Millisecond,
		StallTimeout: 100 * time.Millisecond,
	})
	if !errors.Is(err, ErrPackageTransferStalled) {
		t.Fatalf("expected stall error, got %v", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatal("stall detection too slow")
	}
	if !strings.Contains(err.Error(), "停滞") {
		t.Fatalf("expected bilingual message, got %q", err.Error())
	}
}
