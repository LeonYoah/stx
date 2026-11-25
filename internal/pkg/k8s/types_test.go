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
)

func TestParseSessionConfig_defaultsApplied(t *testing.T) {
	cfg, err := ParseSessionConfig(map[string]interface{}{
		"host_id":   7,
		"image":     "apache/seatunnel:3.1.0",
		"namespace": "seatunnel",
	})
	if err != nil {
		t.Fatalf("ParseSessionConfig error: %v", err)
	}
	if cfg.MasterHTTPPort != 8080 {
		t.Fatalf("expected default master_http_port 8080, got %d", cfg.MasterHTTPPort)
	}
	if cfg.HazelcastPort != 5801 {
		t.Fatalf("expected default hazelcast_port 5801, got %d", cfg.HazelcastPort)
	}
	if cfg.AccessMode != AccessModeAPIServerProxy {
		t.Fatalf("expected default access_mode apiserver_proxy, got %q", cfg.AccessMode)
	}
	if cfg.ManagementSource != ManagementSourceAdopted {
		t.Fatalf("expected default management_source adopted, got %q", cfg.ManagementSource)
	}
	if cfg.JavaProxy.Mode != JavaProxyModeManaged {
		t.Fatalf("expected default java_proxy mode managed, got %q", cfg.JavaProxy.Mode)
	}
	if cfg.JavaProxy.Port != 18080 {
		t.Fatalf("expected default proxy port 18080, got %d", cfg.JavaProxy.Port)
	}
	if cfg.MasterWorkloadKind != "StatefulSet" || cfg.WorkerWorkloadKind != "StatefulSet" {
		t.Fatalf("expected StatefulSet defaults, got %q/%q", cfg.MasterWorkloadKind, cfg.WorkerWorkloadKind)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate error: %v", err)
	}
}

func TestParseSessionConfig_nilGivesDefaults(t *testing.T) {
	cfg, err := ParseSessionConfig(nil)
	if err != nil {
		t.Fatalf("ParseSessionConfig nil error: %v", err)
	}
	if cfg.Namespace != "default" {
		t.Fatalf("expected default namespace, got %q", cfg.Namespace)
	}
}

func TestSessionConfigValidate_portContract(t *testing.T) {
	base := func() *SessionConfig {
		cfg, _ := ParseSessionConfig(map[string]interface{}{
			"host_id": 1,
			"image":   "apache/seatunnel:3.1.0",
		})
		return cfg
	}
	t.Run("hazelcast_port out of range", func(t *testing.T) {
		cfg := base()
		cfg.HazelcastPort = 0
		if err := cfg.Validate(); err == nil {
			t.Fatal("expected error for port 0")
		}
		cfg.HazelcastPort = 65536
		if err := cfg.Validate(); err == nil {
			t.Fatal("expected error for port 65536")
		}
	})
	t.Run("master_http_port out of range", func(t *testing.T) {
		cfg := base()
		cfg.MasterHTTPPort = -1
		if err := cfg.Validate(); err == nil {
			t.Fatal("expected error for negative port")
		}
	})
	t.Run("ports must differ", func(t *testing.T) {
		cfg := base()
		cfg.HazelcastPort = 8080
		cfg.MasterHTTPPort = 8080
		if err := cfg.Validate(); err == nil {
			t.Fatal("expected error for identical ports")
		}
	})
	t.Run("host_id required", func(t *testing.T) {
		cfg := base()
		cfg.HostID = 0
		if err := cfg.Validate(); err == nil {
			t.Fatal("expected error for missing host_id")
		}
	})
	t.Run("image required", func(t *testing.T) {
		cfg := base()
		cfg.Image = ""
		if err := cfg.Validate(); err == nil {
			t.Fatal("expected error for empty image")
		}
	})
	t.Run("external mode requires base url", func(t *testing.T) {
		cfg := base()
		cfg.AccessMode = AccessModeExternal
		if err := cfg.Validate(); err == nil {
			t.Fatal("expected error for missing external_base_url")
		}
		cfg.ExternalBaseURL = "http://127.0.0.1:8080"
		if err := cfg.Validate(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	t.Run("bad management source", func(t *testing.T) {
		cfg := base()
		cfg.ManagementSource = "bogus"
		if err := cfg.Validate(); err == nil {
			t.Fatal("expected error for bad management_source")
		}
	})
	t.Run("bad proxy mode", func(t *testing.T) {
		cfg := base()
		cfg.JavaProxy.Mode = "bogus"
		if err := cfg.Validate(); err == nil {
			t.Fatal("expected error for bad java_proxy mode")
		}
	})
}
