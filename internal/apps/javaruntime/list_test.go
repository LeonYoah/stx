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

package javaruntime

import "testing"

func TestParseList(t *testing.T) {
	out := `{"success":true,"message":"found 2","details":{"jdks":"[{\"java_home\":\"/opt/jdk8\",\"version\":8,\"source\":\"scan\"},{\"java_home\":\"/opt/jdk17\",\"version\":17,\"source\":\"env\"}]","count":"2"}}`
	got := ParseList(true, out)
	if len(got) != 2 || got[0].Version != 8 || got[1].JavaHome != "/opt/jdk17" {
		t.Fatalf("got %+v", got)
	}
	if ParseList(false, out) != nil || ParseList(true, "not-json") != nil {
		t.Fatal("failures must yield nil")
	}
}
