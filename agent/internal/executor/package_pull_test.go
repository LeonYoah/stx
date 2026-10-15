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

package executor

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	pb "github.com/LeonYoah/stx/agent"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// fakeChunkStream serves data from offset, optionally failing/hanging after failAfter chunks.
// fakeChunkStream 从指定偏移量提供数据，可在 failAfter 块后失败或挂起。
type fakeChunkStream struct {
	ctx       context.Context
	data      []byte
	offset    int64
	chunk     int
	sent      int
	failAfter int // <0 never / 小于 0 表示不失败
	hang      bool
}

func (s *fakeChunkStream) Recv() (*pb.PackageChunk, error) {
	if s.failAfter >= 0 && s.sent >= s.failAfter {
		if s.hang {
			<-s.ctx.Done()
			return nil, status.FromContextError(s.ctx.Err()).Err()
		}
		return nil, status.Error(codes.Unavailable, "connection reset")
	}
	if s.offset >= int64(len(s.data)) {
		return nil, io.EOF
	}
	end := s.offset + int64(s.chunk)
	if end > int64(len(s.data)) {
		end = int64(len(s.data))
	}
	c := &pb.PackageChunk{Data: s.data[s.offset:end], Offset: s.offset, TotalSize: int64(len(s.data))}
	s.offset = end
	s.sent++
	return c, nil
}

func newTestPTM(t *testing.T) *PackageTransferManager {
	dir := t.TempDir()
	return &PackageTransferManager{
		tempDir:         filepath.Join(dir, "tmp"),
		packageDir:      filepath.Join(dir, "pkgs"),
		activeTransfers: map[string]*packageTransferState{},
	}
}

func randData(t *testing.T, n int) ([]byte, string) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(b)
	return b, hex.EncodeToString(sum[:])
}

var fastOpts = PullOptions{IdleTimeout: 200 * time.Millisecond, MaxAttempts: 3, Backoff: time.Millisecond}

// TestPullResumesAfterInterruptions 验证流中断/挂起后从已接收偏移量续传并通过校验。
// TestPullResumesAfterInterruptions verifies resume from the received offset after resets and idle hangs.
func TestPullResumesAfterInterruptions(t *testing.T) {
	m := newTestPTM(t)
	data, sum := randData(t, 10*1024+17)
	var opens atomic.Int32
	var offsets []int64
	open := func(ctx context.Context, id string, off int64) (PackageChunkStream, error) {
		n := opens.Add(1)
		offsets = append(offsets, off)
		s := &fakeChunkStream{ctx: ctx, data: data, offset: off, chunk: 1024, failAfter: -1}
		switch n {
		case 1:
			s.failAfter = 3 // reset after 3 chunks / 3 块后连接重置
		case 2:
			s.failAfter, s.hang = 2, true // stall: idle timeout must fire / 停滞：必须触发空闲超时
		}
		return s, nil
	}
	var last int64
	path, err := m.Pull(context.Background(), open, PullRequest{TransferID: "t1", FileName: "pkg.tar.gz", TotalSize: int64(len(data)), Checksum: sum},
		fastOpts, func(r, total int64) { last = r })
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	got, _ := os.ReadFile(path)
	if !bytes.Equal(got, data) {
		t.Fatal("content mismatch")
	}
	if opens.Load() != 3 || offsets[1] != 3*1024 || offsets[2] != 5*1024 {
		t.Fatalf("unexpected resume offsets: %v", offsets)
	}
	if last != int64(len(data)) {
		t.Fatalf("progress=%d", last)
	}
	if filepath.Dir(path) != m.packageDir {
		t.Fatalf("path=%s", path)
	}
	if entries, _ := os.ReadDir(m.tempDir); len(entries) != 0 {
		t.Fatalf("temp not cleaned: %v", entries)
	}
}

// TestPullChecksumMismatch 验证校验和不一致时失败且不留下文件。
// TestPullChecksumMismatch verifies a checksum mismatch fails and leaves nothing behind.
func TestPullChecksumMismatch(t *testing.T) {
	m := newTestPTM(t)
	data, _ := randData(t, 2048)
	open := func(ctx context.Context, id string, off int64) (PackageChunkStream, error) {
		return &fakeChunkStream{ctx: ctx, data: data, offset: off, chunk: 512, failAfter: -1}, nil
	}
	_, err := m.Pull(context.Background(), open, PullRequest{TransferID: "t", FileName: "p.tgz", Checksum: strings.Repeat("0", 64)}, fastOpts, nil)
	if err == nil || !strings.Contains(err.Error(), "校验和不匹配") {
		t.Fatalf("expected checksum error, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(m.packageDir, "p.tgz")); !os.IsNotExist(statErr) {
		t.Fatal("package should not exist")
	}
}

// TestPullNonRetryableAndExhausted 验证 NotFound 立即失败，持续无进展时在重试上限后失败。
// TestPullNonRetryableAndExhausted verifies NotFound fails immediately and no-progress retries are bounded.
func TestPullNonRetryableAndExhausted(t *testing.T) {
	m := newTestPTM(t)
	var opens atomic.Int32
	notFound := func(ctx context.Context, id string, off int64) (PackageChunkStream, error) {
		opens.Add(1)
		return nil, status.Error(codes.NotFound, "gone")
	}
	if _, err := m.Pull(context.Background(), notFound, PullRequest{TransferID: "t", FileName: "p"}, fastOpts, nil); err == nil || opens.Load() != 1 {
		t.Fatalf("expected immediate failure, err=%v opens=%d", err, opens.Load())
	}

	opens.Store(0)
	data, _ := randData(t, 100)
	broken := func(ctx context.Context, id string, off int64) (PackageChunkStream, error) {
		opens.Add(1)
		return &fakeChunkStream{ctx: ctx, data: data, offset: off, chunk: 10, failAfter: 0}, nil
	}
	_, err := m.Pull(context.Background(), broken, PullRequest{TransferID: "t", FileName: "p"}, fastOpts, nil)
	if err == nil || opens.Load() != int32(fastOpts.MaxAttempts) {
		t.Fatalf("expected bounded retries, err=%v opens=%d", err, opens.Load())
	}
}

// TestPullRejectsUnsafeFileName 验证拒绝带路径的文件名。
// TestPullRejectsUnsafeFileName verifies file names with path components are rejected.
func TestPullRejectsUnsafeFileName(t *testing.T) {
	m := newTestPTM(t)
	open := func(ctx context.Context, id string, off int64) (PackageChunkStream, error) {
		return nil, errors.New("must not open")
	}
	if _, err := m.Pull(context.Background(), open, PullRequest{TransferID: "t", FileName: "../evil"}, fastOpts, nil); err == nil {
		t.Fatal("expected error")
	}
}
