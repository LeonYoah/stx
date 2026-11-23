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

package process

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestStopProcessNoPIDsIsSuccess(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// SeaTunnel start script path is required only for Start; Stop needs install dir for matching.
	if err := os.MkdirAll(filepath.Join(dir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	mgr := NewProcessManager()
	name := "seatunnel-test@stop"
	mgr.processes.Store(name, &ManagedProcess{
		Name:       name,
		Status:     StatusRunning,
		InstallDir: dir,
		Role:       "master/worker",
		PID:        0,
	})
	if err := mgr.StopProcess(context.Background(), name, &StopParams{
		InstallDir: dir,
		Role:       "master/worker",
	}); err != nil {
		t.Fatalf("already-stopped stop must succeed: %v", err)
	}
	value, ok := mgr.processes.Load(name)
	if !ok {
		t.Fatal("process entry missing")
	}
	proc := value.(*ManagedProcess)
	proc.mu.RLock()
	defer proc.mu.RUnlock()
	if proc.Status != StatusStopped {
		t.Fatalf("status=%s, want stopped", proc.Status)
	}
}

func TestStartProcessAlreadyRunningError(t *testing.T) {
	t.Parallel()
	mgr := NewProcessManager()
	name := "seatunnel-test@running"
	mgr.processes.Store(name, &ManagedProcess{
		Name:   name,
		Status: StatusRunning,
		PID:    1,
	})
	err := mgr.StartProcess(context.Background(), name, &StartParams{InstallDir: t.TempDir()})
	if err != ErrProcessAlreadyRunning {
		t.Fatalf("want ErrProcessAlreadyRunning, got %v", err)
	}
}
