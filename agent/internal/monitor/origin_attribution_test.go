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

package monitor

import "testing"

func TestManualFollowupAttributionConsumedOnce(t *testing.T) {
	m := NewProcessMonitor()
	m.TrackProcessSilent("st-1", 0, "/opt/st", "master", nil)
	m.SetProcessOriginAttribution("st-1", "cmd-1", "req-1")

	commandID, requestID, ok := m.ConsumeManualFollowupAttribution("st-1")
	if !ok || commandID != "cmd-1" || requestID != "req-1" {
		t.Fatalf("first consume failed: ok=%v cmd=%q req=%q", ok, commandID, requestID)
	}
	_, _, ok = m.ConsumeManualFollowupAttribution("st-1")
	if ok {
		t.Fatal("second consume should be false so crash restarts stay system/auto")
	}
}
