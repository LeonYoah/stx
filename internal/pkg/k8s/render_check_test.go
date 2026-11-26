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

package k8s

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

func TestRenderDynamicRuntimeSections(t *testing.T) {
	off := false
	slotNum := 8
	expire := 2880
	cfg := &SessionConfig{
		MasterHTTPPort: 8080,
		Image:          "apache/seatunnel:3.1.0",
		Runtime: &SessionRuntimeConfig{
			DynamicSlot:             &off,
			SlotNum:                 &slotNum,
			SlotAllocationStrategy:  "random",
			JobScheduleStrategy:     "reject",
			HistoryJobExpireMinutes: &expire,
		},
	}
	out := renderSeatunnelYAML(cfg)
	var parsed map[string]interface{}
	if err := yaml.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("rendered yaml invalid: %v\n%s", err, out)
	}
	assert.Contains(t, out, "dynamic-slot: false")
	assert.Contains(t, out, "slot-num: 8")
	assert.Contains(t, out, "slot-allocate-strategy: RANDOM")
	assert.Contains(t, out, "job-schedule-strategy: REJECT")
	assert.Contains(t, out, "history-job-expire-minutes: 2880")
	assert.NotContains(t, out, "slot-allocation-strategy")
}

func TestRenderPerRoleHeap(t *testing.T) {
	cfg := &SessionConfig{
		MasterHTTPPort: 8080,
		DeploymentMode: "separated",
		JVM:            &SessionJVMConfig{MasterHeapMB: 1024, WorkerHeapMB: 3072},
	}
	files := sessionConfigFiles(cfg, "demo")
	assert.Contains(t, files["jvm_master_options"], "-Xmx1024m")
	assert.Contains(t, files["jvm_master_options"], "-Xms512m")
	assert.Contains(t, files["jvm_worker_options"], "-Xmx3072m")
	hybrid := &SessionConfig{MasterHTTPPort: 8080, DeploymentMode: "hybrid", JVM: &SessionJVMConfig{HybridHeapMB: 2048}}
	hf := sessionConfigFiles(hybrid, "demo")
	assert.Contains(t, hf["jvm_options"], "-Xmx2048m")
}
