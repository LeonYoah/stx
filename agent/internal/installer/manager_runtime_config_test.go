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
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestModifySeaTunnelConfigSkipsUnsupportedRuntimeKeys(t *testing.T) {
	configPath := writeRuntimeConfigFixture(t)
	manager := NewInstallerManager()
	dynamicSlot := false
	slotNum := 4
	slotAllocationStrategy := "SYSTEM_LOAD"
	historyJobExpireMinutes := 2880
	scheduledDeletionEnable := false

	err := manager.modifySeaTunnelConfig(configPath, &InstallParams{
		Version:                 "2.3.2",
		HTTPPort:                8080,
		DynamicSlot:             &dynamicSlot,
		SlotNum:                 &slotNum,
		SlotAllocationStrategy:  slotAllocationStrategy,
		JobScheduleStrategy:     "WAIT",
		HistoryJobExpireMinutes: &historyJobExpireMinutes,
		ScheduledDeletionEnable: &scheduledDeletionEnable,
	})
	if err != nil {
		t.Fatalf("modifySeaTunnelConfig returned error: %v", err)
	}

	root := readRuntimeConfigMap(t, configPath)
	engine := mustNestedMap(t, root, "seatunnel", "engine")
	slotService := mustNestedMap(t, engine, "slot-service")
	if got := slotService["dynamic-slot"]; got != false {
		t.Fatalf("expected dynamic-slot=false, got %#v", got)
	}
	if got := slotService["slot-num"]; got != 4 {
		t.Fatalf("expected slot-num=4, got %#v", got)
	}
	if _, ok := slotService["slot-allocation-strategy"]; ok {
		t.Fatalf("expected old version to skip slot-allocation-strategy, got %#v", slotService["slot-allocation-strategy"])
	}
	if _, ok := engine["history-job-expire-minutes"]; ok {
		t.Fatalf("expected old version to skip history-job-expire-minutes, got %#v", engine["history-job-expire-minutes"])
	}
	if _, ok := engine["job-schedule-strategy"]; ok {
		t.Fatalf("expected old version to skip job-schedule-strategy, got %#v", engine["job-schedule-strategy"])
	}

	logs := mustNestedMap(t, engine, "telemetry", "logs")
	if got := logs["scheduled-deletion-enable"]; got != true {
		t.Fatalf("expected unsupported version to keep existing scheduled-deletion-enable=true, got %#v", got)
	}
}

func TestModifySeaTunnelConfigWritesSupportedRuntimeKeys(t *testing.T) {
	configPath := writeRuntimeConfigFixture(t)
	manager := NewInstallerManager()
	enableHTTP := false
	dynamicSlot := false
	slotNum := 8
	slotAllocationStrategy := "SYSTEM_LOAD"
	historyJobExpireMinutes := 720
	scheduledDeletionEnable := false

	err := manager.modifySeaTunnelConfig(configPath, &InstallParams{
		Version:                 "2.3.10",
		HTTPPort:                8080,
		EnableHTTP:              &enableHTTP,
		DynamicSlot:             &dynamicSlot,
		SlotNum:                 &slotNum,
		SlotAllocationStrategy:  slotAllocationStrategy,
		JobScheduleStrategy:     "WAIT",
		HistoryJobExpireMinutes: &historyJobExpireMinutes,
		ScheduledDeletionEnable: &scheduledDeletionEnable,
	})
	if err != nil {
		t.Fatalf("modifySeaTunnelConfig returned error: %v", err)
	}

	root := readRuntimeConfigMap(t, configPath)
	engine := mustNestedMap(t, root, "seatunnel", "engine")
	httpConfig := mustNestedMap(t, engine, "http")
	if got := httpConfig["enable-http"]; got != false {
		t.Fatalf("expected enable-http=false, got %#v", got)
	}
	slotService := mustNestedMap(t, engine, "slot-service")
	if got := slotService["dynamic-slot"]; got != false {
		t.Fatalf("expected dynamic-slot=false, got %#v", got)
	}
	if got := slotService["slot-num"]; got != 8 {
		t.Fatalf("expected slot-num=8, got %#v", got)
	}
	if got := slotService["slot-allocation-strategy"]; got != "SYSTEM_LOAD" {
		t.Fatalf("expected slot-allocation-strategy=SYSTEM_LOAD, got %#v", got)
	}
	if got := engine["job-schedule-strategy"]; got != "WAIT" {
		t.Fatalf("expected job-schedule-strategy=WAIT, got %#v", got)
	}
	if got := engine["history-job-expire-minutes"]; got != 720 {
		t.Fatalf("expected history-job-expire-minutes=720, got %#v", got)
	}

	logs := mustNestedMap(t, engine, "telemetry", "logs")
	if got := logs["scheduled-deletion-enable"]; got != false {
		t.Fatalf("expected scheduled-deletion-enable=false, got %#v", got)
	}
}

func TestModifyLog4j2ConfigSwitchesAppenderReference(t *testing.T) {
	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "log4j2.properties")
	content := "rootLogger.appenderRef.file.ref = fileAppender\n"
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write log4j2 fixture: %v", err)
	}

	manager := NewInstallerManager()
	if err := manager.modifyLog4j2Config(configPath, &InstallParams{JobLogMode: JobLogModePerJob}); err != nil {
		t.Fatalf("modifyLog4j2Config returned error: %v", err)
	}

	updated, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read modified log4j2: %v", err)
	}
	if string(updated) != "rootLogger.appenderRef.file.ref = routingAppender\n" {
		t.Fatalf("expected routingAppender, got %q", string(updated))
	}
}

func writeRuntimeConfigFixture(t *testing.T) string {
	t.Helper()

	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "seatunnel.yaml")
	content := `seatunnel:
  engine:
    slot-service:
      dynamic-slot: true
      slot-num: 2
    telemetry:
      metric:
        enabled: false
      logs:
        scheduled-deletion-enable: true
`
	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write runtime config fixture: %v", err)
	}
	return configPath
}

func readRuntimeConfigMap(t *testing.T, path string) map[string]any {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read runtime config: %v", err)
	}
	var root map[string]any
	if err := yaml.Unmarshal(content, &root); err != nil {
		t.Fatalf("failed to parse runtime config yaml: %v", err)
	}
	return root
}

func mustNestedMap(t *testing.T, value map[string]any, path ...string) map[string]any {
	t.Helper()

	current := value
	for _, key := range path {
		next, ok := current[key]
		if !ok {
			t.Fatalf("expected key %q in map %#v", key, current)
		}
		typed, ok := next.(map[string]any)
		if !ok {
			t.Fatalf("expected map[string]any at %q, got %#v", key, next)
		}
		current = typed
	}
	return current
}
