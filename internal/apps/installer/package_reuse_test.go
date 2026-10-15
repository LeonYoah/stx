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

package installer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// fakePkgAgentManager records TransferPackageFile calls. Embedded nil interface covers other methods.
// fakePkgAgentManager 记录 TransferPackageFile 调用；其余方法由嵌入的 nil 接口占位。
type fakePkgAgentManager struct {
	AgentManager
	hints     []string
	checksums []string
	fail      bool
}

func (f *fakePkgAgentManager) TransferPackageFile(ctx context.Context, agentID, version, localPath, checksum, cachedRemotePath string, onProgress func(sent, total int64)) (string, bool, error) {
	f.hints = append(f.hints, cachedRemotePath)
	f.checksums = append(f.checksums, checksum)
	if f.fail {
		return "", false, errors.New("boom")
	}
	return "/agent/packages/" + filepath.Base(localPath), cachedRemotePath != "", nil
}

// TestPackageReuseIsChecksumBased 验证复用基于校验和：未变化时提供复用提示，CP 包变化或传输失败后不再提示。
// TestPackageReuseIsChecksumBased verifies the reuse hint is only offered for identical CP content.
func TestPackageReuseIsChecksumBased(t *testing.T) {
	dir := t.TempDir()
	fake := &fakePkgAgentManager{}
	s := NewService(dir, fake)
	pkg := filepath.Join(dir, packageFileName("2.3.13"))
	if err := os.WriteFile(pkg, []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}

	// 1) first transfer: no hint / 首次传输：无提示
	p, sum1, err := s.transferPackageFileToAgent(context.Background(), "a1", "2.3.13", pkg, nil)
	if err != nil || p == "" || sum1 == "" {
		t.Fatalf("first: p=%s sum=%s err=%v", p, sum1, err)
	}
	// 2) unchanged: hint offered (Agent still verifies sha) / 未变化：提供提示（Agent 仍校验 sha）
	if _, _, err := s.transferPackageFileToAgent(context.Background(), "a1", "2.3.13", pkg, nil); err != nil {
		t.Fatal(err)
	}
	// 3) CP package replaced: new checksum, no hint / CP 包被替换：新校验和、无提示
	if err := os.WriteFile(pkg, []byte("v2-different"), 0o644); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(time.Minute)
	_ = os.Chtimes(pkg, future, future)
	_, sum3, err := s.transferPackageFileToAgent(context.Background(), "a1", "2.3.13", pkg, nil)
	if err != nil || sum3 == sum1 {
		t.Fatalf("changed: sum3=%s sum1=%s err=%v", sum3, sum1, err)
	}
	// 4) failure drops cache / 失败后清除缓存
	fake.fail = true
	if _, _, err := s.transferPackageFileToAgent(context.Background(), "a1", "2.3.13", pkg, nil); err == nil {
		t.Fatal("expected error")
	}
	fake.fail = false
	if _, _, err := s.transferPackageFileToAgent(context.Background(), "a1", "2.3.13", pkg, nil); err != nil {
		t.Fatal(err)
	}

	want := []string{"", "/agent/packages/" + filepath.Base(pkg), "", "/agent/packages/" + filepath.Base(pkg), ""}
	if len(fake.hints) != len(want) {
		t.Fatalf("hints=%v", fake.hints)
	}
	for i := range want {
		if fake.hints[i] != want[i] {
			t.Fatalf("call %d hint=%q want %q (all=%v)", i, fake.hints[i], want[i], fake.hints)
		}
	}
	if fake.checksums[2] != sum3 || fake.checksums[0] != sum1 {
		t.Fatalf("checksums=%v", fake.checksums)
	}
}

// TestCachedPackageChecksumSkipsRehash 验证大小与修改时间未变时不重新计算校验和。
// TestCachedPackageChecksumSkipsRehash verifies no rehash while size+mtime are unchanged.
func TestCachedPackageChecksumSkipsRehash(t *testing.T) {
	dir := t.TempDir()
	s := NewService(dir, &fakePkgAgentManager{})
	p := filepath.Join(dir, "f")
	_ = os.WriteFile(p, []byte("abc"), 0o644)
	sum, _, err := s.cachedPackageChecksum(p)
	if err != nil {
		t.Fatal(err)
	}
	// Poison the cache to prove it is used / 篡改缓存以证明确实命中缓存
	s.preparedAssetMu.Lock()
	e := s.packageChecksums[p]
	e.Sum = "cached"
	s.packageChecksums[p] = e
	s.preparedAssetMu.Unlock()
	if got, _, _ := s.cachedPackageChecksum(p); got != "cached" {
		t.Fatalf("expected cache hit, got %s", got)
	}
	future := time.Now().Add(time.Minute)
	_ = os.Chtimes(p, future, future)
	if got, _, _ := s.cachedPackageChecksum(p); got != sum {
		t.Fatalf("expected rehash after mtime change, got %s", got)
	}
}
