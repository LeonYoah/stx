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
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	pb "github.com/LeonYoah/stx/agent"
)

// TestPullHandshakeReuseAndOverwrite 验证：文件一致则跳过下载；文件被删除或内容不一致则重新拉取并覆盖。
// TestPullHandshakeReuseAndOverwrite: identical file is reused; deleted or mismatching file is re-pulled and overwritten.
func TestPullHandshakeReuseAndOverwrite(t *testing.T) {
	mgr := GetPackageTransferManager()
	pkgDir := t.TempDir()
	mgr.SetDirectories(t.TempDir(), pkgDir)

	data, sum := randData(t, 4096)
	var opens atomic.Int32
	SetPackageStreamOpener(func(ctx context.Context, id string, off int64) (PackageChunkStream, error) {
		opens.Add(1)
		return &fakeChunkStream{ctx: ctx, data: data, offset: off, chunk: 1024, failAfter: -1}, nil
	})
	defer SetPackageStreamOpener(nil)

	const fileName = "apache-seatunnel-9.9.9-bin.tar.gz"
	final := filepath.Join(pkgDir, fileName)
	run := func(cachedPath string) map[string]any {
		t.Helper()
		resp, err := HandleTransferPackageCommand(context.Background(), &pb.CommandRequest{
			CommandId: "c",
			Parameters: map[string]string{
				"mode": "pull", "transfer_id": "t", "version": "9.9.9", "file_name": fileName,
				"total_size": strconv.Itoa(len(data)), "checksum": sum, "cached_path": cachedPath,
			},
		}, nil)
		if err != nil || resp.Status != pb.CommandStatus_SUCCESS {
			t.Fatalf("resp=%+v err=%v", resp, err)
		}
		out := map[string]any{}
		_ = json.Unmarshal([]byte(resp.Output), &out)
		return out
	}

	// Missing → pull / 不存在 → 拉取
	if out := run(""); out["reused"] == true || opens.Load() != 1 {
		t.Fatalf("first: %v opens=%d", out, opens.Load())
	}
	// Identical → reuse, no stream / 一致 → 复用，不开流
	if out := run(final); out["reused"] != true || out["local_path"] != final || opens.Load() != 1 {
		t.Fatalf("reuse: %v opens=%d", out, opens.Load())
	}
	// Mismatching content (same size, new mtime) → re-pull & overwrite / 内容不一致 → 重新拉取并覆盖
	bad := make([]byte, len(data))
	if err := os.WriteFile(final, bad, 0o644); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(time.Minute)
	_ = os.Chtimes(final, future, future)
	if out := run(final); out["reused"] == true || opens.Load() != 2 {
		t.Fatalf("mismatch: %v opens=%d", out, opens.Load())
	}
	if got, _ := os.ReadFile(final); string(got) != string(data) {
		t.Fatal("file not overwritten with correct content")
	}
	// Deleted → re-pull / 被删除 → 重新拉取
	_ = os.Remove(final)
	if out := run(final); out["reused"] == true || opens.Load() != 3 {
		t.Fatalf("deleted: %v opens=%d", out, opens.Load())
	}
	if _, err := os.Stat(final); err != nil {
		t.Fatal("package should exist again")
	}
}
