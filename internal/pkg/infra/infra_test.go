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
	if IsK8s(string(TypeBareMetal)) || IsK8s("") {
		t.Fatal("bare_metal and empty must not be k8s")
	}
	for _, v := range []string{string(TypeK8sSession), string(TypeK8sAppTemplate), string(TypeK8sAppInstance), "kubernetes"} {
		if !IsK8s(v) {
			t.Fatalf("%s must be k8s", v)
		}
	}
	if IsK8s(string(TypeYarnSession)) || IsK8s(string(TypeYarnAppInstance)) {
		t.Fatal("yarn shapes must not be k8s")
	}
}

func TestIsK8sSession(t *testing.T) {
	if !IsK8sSession(string(TypeK8sSession)) || !IsK8sSession("kubernetes") {
		t.Fatal("session and legacy literal must match")
	}
	if IsK8sSession(string(TypeK8sAppInstance)) || IsK8sSession(string(TypeYarnSession)) {
		t.Fatal("app instance and yarn session must not match k8s session")
	}
}

func TestPlatformExecutionAndDeployMode(t *testing.T) {
	cases := []struct {
		raw      string
		platform RuntimePlatform
		execMode ExecutionMode
		deploy   DeployMode
	}{
		{"", PlatformStandalone, ExecutionModeSession, DeployModeStandalone},
		{string(TypeBareMetal), PlatformStandalone, ExecutionModeSession, DeployModeStandalone},
		{"kubernetes", PlatformKubernetes, ExecutionModeSession, DeployModeK8sSession},
		{string(TypeK8sSession), PlatformKubernetes, ExecutionModeSession, DeployModeK8sSession},
		{string(TypeK8sAppTemplate), PlatformKubernetes, ExecutionModeApplication, DeployModeK8sApplication},
		{string(TypeK8sAppInstance), PlatformKubernetes, ExecutionModeApplication, DeployModeK8sApplication},
		{string(TypeYarnSession), PlatformYarn, ExecutionModeSession, DeployModeYarnSession},
		{string(TypeYarnAppTemplate), PlatformYarn, ExecutionModeApplication, DeployModeYarnApplication},
		{string(TypeYarnAppInstance), PlatformYarn, ExecutionModeApplication, DeployModeYarnApplication},
	}
	for _, tc := range cases {
		if got := PlatformOf(tc.raw); got != tc.platform {
			t.Fatalf("PlatformOf(%q)=%q want %q", tc.raw, got, tc.platform)
		}
		if got := ExecutionModeOf(tc.raw); got != tc.execMode {
			t.Fatalf("ExecutionModeOf(%q)=%q want %q", tc.raw, got, tc.execMode)
		}
		if got := DeployModeOf(tc.raw); got != tc.deploy {
			t.Fatalf("DeployModeOf(%q)=%q want %q", tc.raw, got, tc.deploy)
		}
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
