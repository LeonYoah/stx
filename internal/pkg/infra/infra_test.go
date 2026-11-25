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

package infra

import "testing"

func TestIsK8s(t *testing.T) {
	if IsK8s(string(TypeBareMetal)) {
		t.Fatal("bare_metal must not be k8s")
	}
	for _, v := range []string{string(TypeK8sSession), string(TypeK8sAppTemplate), string(TypeK8sAppInstance), "kubernetes"} {
		if !IsK8s(v) {
			t.Fatalf("%s must be k8s", v)
		}
	}
	if IsK8s("yarn_session") {
		t.Fatal("yarn_session must not be k8s")
	}
}

func TestIsK8sSession(t *testing.T) {
	if !IsK8sSession(string(TypeK8sSession)) || !IsK8sSession("kubernetes") {
		t.Fatal("session and legacy literal must match")
	}
	if IsK8sSession(string(TypeK8sAppInstance)) {
		t.Fatal("app instance is not a session cluster")
	}
}

func TestEffective(t *testing.T) {
	if Effective("") != string(TypeBareMetal) {
		t.Fatal("empty must fall back to bare_metal")
	}
	if Effective(string(TypeK8sSession)) != string(TypeK8sSession) {
		t.Fatal("explicit values pass through")
	}
}
