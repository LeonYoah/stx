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

package dockercompose

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const wantTZ = "${TZ:-Asia/Shanghai}"

type composeFile struct {
	Services map[string]composeService `yaml:"services"`
}

type composeService struct {
	Environment any `yaml:"environment"`
}

// TestComposeServicesInjectTimezone 断言 Docker Compose 三套栈合并后每个容器都注入 TZ。
// TestComposeServicesInjectTimezone asserts every merged Compose service injects TZ.
func TestComposeServicesInjectTimezone(t *testing.T) {
	root := repoRoot(t)
	dockerDir := filepath.Join(root, "deploy", "docker")
	base := loadCompose(t, filepath.Join(dockerDir, "compose.base.yml"))
	if len(base.Services) == 0 {
		t.Fatal("compose.base.yml has no services")
	}
	for name := range base.Services {
		env := envOf(t, "compose.base.yml", name, base.Services[name])
		assertServiceTZ(t, "compose.base.yml", name, env)
		if name == "stx-backend" {
			assertBackendLegacyTZ(t, "compose.base.yml", env)
		}
	}

	overlays, err := filepath.Glob(filepath.Join(dockerDir, "docker-compose*.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(overlays) == 0 {
		t.Fatal("no docker-compose*.yml overlays")
	}
	for _, overlayPath := range overlays {
		overlay := loadCompose(t, overlayPath)
		merged := mergeServices(t, filepath.Base(overlayPath), base, overlay)
		if len(merged) == 0 {
			t.Errorf("%s: merged to zero services", filepath.Base(overlayPath))
		}
		for name, svc := range merged {
			env := envOf(t, filepath.Base(overlayPath), name, svc)
			assertServiceTZ(t, filepath.Base(overlayPath), name, env)
			if name == "stx-backend" {
				assertBackendLegacyTZ(t, filepath.Base(overlayPath), env)
			}
		}
	}
}

// TestComposeTimezoneDefaults 断言 .env 与镜像 Dockerfile 的默认时区与 Compose 一致。
// TestComposeTimezoneDefaults asserts .env and image Dockerfiles share the Compose default timezone.
func TestComposeTimezoneDefaults(t *testing.T) {
	root := repoRoot(t)
	for _, rel := range []string{
		"deploy/docker/.env",
		"deploy/docker/.env.cn.example",
	} {
		body := readFile(t, filepath.Join(root, rel))
		if !activeEnvLine(body, "TZ=Asia/Shanghai") {
			t.Errorf("%s: missing active TZ=Asia/Shanghai", rel)
		}
	}
	for _, rel := range []string{
		".github/docker/Dockerfile.backend",
		".github/docker/Dockerfile.frontend",
		".github/docker/Dockerfile.all-in-one",
	} {
		body := readFile(t, filepath.Join(root, rel))
		if !strings.Contains(body, "ENV TZ=Asia/Shanghai") {
			t.Errorf("%s: missing ENV TZ=Asia/Shanghai", rel)
		}
	}
}

const wantLegacyTZ = "UTC"

func assertServiceTZ(t *testing.T, file, service string, env map[string]string) {
	t.Helper()
	got := env["TZ"]
	if got != wantTZ {
		t.Errorf("%s service %s: TZ=%q, want %q", file, service, got, wantTZ)
	}
}

func assertBackendLegacyTZ(t *testing.T, file string, env map[string]string) {
	t.Helper()
	got := env["STX_DATABASE_LEGACY_TIMEZONE"]
	if got != wantLegacyTZ {
		t.Errorf("%s service stx-backend: STX_DATABASE_LEGACY_TIMEZONE=%q, want %q", file, got, wantLegacyTZ)
	}
}

// mergeServices 按 Compose mapping 规则合并 overlay：environment 键级合并，overlay 不得用 list 覆盖掉 base 的 TZ。
// mergeServices merges overlay services like Compose mappings: environment is key-merged so a list overlay cannot drop base TZ.
func mergeServices(t *testing.T, overlayName string, base, overlay composeFile) map[string]composeService {
	t.Helper()
	out := make(map[string]composeService, len(base.Services)+len(overlay.Services))
	for name, svc := range base.Services {
		out[name] = svc
	}
	for name, ov := range overlay.Services {
		baseSvc, ok := out[name]
		if !ok {
			out[name] = ov
			continue
		}
		baseEnv := envOf(t, "compose.base.yml", name, baseSvc)
		ovEnv := envOf(t, overlayName, name, ov)
		merged := make(map[string]string, len(baseEnv)+len(ovEnv))
		for k, v := range baseEnv {
			merged[k] = v
		}
		for k, v := range ovEnv {
			merged[k] = v
		}
		out[name] = composeService{Environment: stringMapToAny(merged)}
	}
	return out
}

func envOf(t *testing.T, file, service string, svc composeService) map[string]string {
	t.Helper()
	env, err := parseEnvironment(svc.Environment)
	if err != nil {
		t.Fatalf("%s service %s: %v", file, service, err)
	}
	return env
}

// parseEnvironment 只接受 mapping 形态的 environment；list 会在 Compose 合并时整段替换，容易丢掉 TZ。
// parseEnvironment accepts mapping environment only; a list replaces the whole block in Compose merge and can drop TZ.
func parseEnvironment(v any) (map[string]string, error) {
	if v == nil {
		return map[string]string{}, nil
	}
	switch typed := v.(type) {
	case map[string]any:
		out := make(map[string]string, len(typed))
		for k, raw := range typed {
			out[k] = fmt.Sprint(raw)
		}
		return out, nil
	case map[string]string:
		return typed, nil
	case []any, []string:
		return nil, fmt.Errorf("environment must be a mapping so overlay merge keeps TZ; got %T", v)
	default:
		return nil, fmt.Errorf("unsupported environment type %T", v)
	}
}

func stringMapToAny(in map[string]string) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func loadCompose(t *testing.T, path string) composeFile {
	t.Helper()
	var cf composeFile
	if err := yaml.Unmarshal([]byte(readFile(t, path)), &cf); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	if cf.Services == nil {
		cf.Services = map[string]composeService{}
	}
	return cf
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func activeEnvLine(body, want string) bool {
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if trimmed == want {
			return true
		}
	}
	return false
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			if _, err := os.Stat(filepath.Join(dir, "deploy", "docker", "compose.base.yml")); err == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("repo root not found")
		}
		dir = parent
	}
}
