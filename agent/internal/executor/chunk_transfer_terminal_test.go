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
	"encoding/base64"
	"testing"

	pb "github.com/LeonYoah/stx/agent"
)

// TestChunkTransferRepliesAreTerminal 验证插件/安装包分块传输的中间块回复为终态，避免 CP 等到超时。
// TestChunkTransferRepliesAreTerminal verifies intermediate chunk replies are terminal so the CP never waits for the timeout.
func TestChunkTransferRepliesAreTerminal(t *testing.T) {
	dir := t.TempDir()
	InitPluginManager(dir)
	chunk := base64.StdEncoding.EncodeToString([]byte("hello"))

	resp, err := HandleTransferPluginCommand(context.Background(), &pb.CommandRequest{
		CommandId: "c1",
		Type:      pb.CommandType_TRANSFER_PLUGIN,
		Parameters: map[string]string{
			"plugin_name": "jdbc", "version": "2.3.13", "file_type": "connector", "file_name": "a.jar",
			"chunk": chunk, "offset": "0", "total_size": "10", "is_last": "false", "install_path": dir,
		},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != pb.CommandStatus_SUCCESS {
		t.Fatalf("plugin intermediate chunk status=%v error=%s, want SUCCESS", resp.Status, resp.Error)
	}

	mgr := GetPackageTransferManager()
	mgr.SetDirectories(t.TempDir(), t.TempDir())
	resp, err = HandleTransferPackageCommand(context.Background(), &pb.CommandRequest{
		CommandId: "c2",
		Type:      pb.CommandType_TRANSFER_PACKAGE,
		Parameters: map[string]string{
			"version": "9.9.9-test", "chunk": chunk, "offset": "0", "total_size": "10", "is_last": "false",
		},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != pb.CommandStatus_SUCCESS {
		t.Fatalf("package intermediate chunk status=%v, want SUCCESS", resp.Status)
	}
	mgr.mu.Lock()
	mgr.cleanupTransfer("9.9.9-test")
	mgr.mu.Unlock()
}
