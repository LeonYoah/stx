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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRenderSeatunnelYAMLRuntimeOverrides(t *testing.T) {
	cfg := &SessionConfig{MasterHTTPPort: 8080}
	off := false
	cfg.Runtime = &SessionRuntimeConfig{EnableHTTPServer: &off, EventReportHTTP: "http://stx.example.com/api/v1/engine-events"}
	cfg.Checkpoint = &SessionStorageSpec{
		Enabled: true, StorageType: "S3", Namespace: "ckpt", Endpoint: "https://s3.example.com", Bucket: "bkt", AccessKey: "ak", SecretKey: "sk",
	}
	out := renderSeatunnelYAML(cfg)
	assert.Contains(t, out, "enable-http: false")
	assert.Contains(t, out, "url: http://stx.example.com/api/v1/engine-events")
	assert.Contains(t, out, "storage.type: s3")
	assert.Contains(t, out, "fs.s3a.endpoint: https://s3.example.com")
	assert.Contains(t, out, "s3.bucket: bkt")
	assert.Contains(t, out, "namespace: ckpt")
}

func TestRenderSeatunnelYAMLDefaultsUnchanged(t *testing.T) {
	cfg := &SessionConfig{MasterHTTPPort: 8080}
	out := renderSeatunnelYAML(cfg)
	assert.Contains(t, out, "enable-http: true")
	assert.NotContains(t, out, "event-report-http")
	assert.Contains(t, out, "fs.defaultFS: file:///tmp/")
}

func TestRenderJVMOptionsWithConfigHeap(t *testing.T) {
	out := renderJVMOptionsWithConfig(&SessionJVMConfig{Xms: "1g", Xmx: "2g", ExtraOptions: []string{"-XX:+UseZGC"}})
	assert.True(t, strings.HasPrefix(out, "-Xms1g -Xmx2g\n"))
	assert.Contains(t, out, "-XX:+UseZGC")
	defaultOut := renderJVMOptions()
	assert.True(t, strings.HasPrefix(defaultOut, "-Xms2g -Xmx2g\n"))
}

func TestRenderIMAPMapStoreYAML(t *testing.T) {
	cfg := &SessionConfig{IMAP: &SessionStorageSpec{Enabled: true, StorageType: "LOCAL_FILE"}}
	out := renderIMAPMapStoreYAML(cfg)
	assert.Contains(t, out, "map-store")
	assert.Contains(t, out, "enabled: true")
	cfg2 := &SessionConfig{IMAP: &SessionStorageSpec{Enabled: false, StorageType: "DISABLED"}}
	out2 := renderIMAPMapStoreYAML(cfg2)
	assert.Contains(t, out2, "enabled: false")
	assert.Empty(t, renderIMAPMapStoreYAML(&SessionConfig{}))
}

func TestSessionConfigFilesHybrid(t *testing.T) {
	cfg := &SessionConfig{MasterHTTPPort: 8080, Namespace: "default", HazelcastPort: 5801, DeploymentMode: "hybrid"}
	files := sessionConfigFiles(cfg, "demo")
	assert.Contains(t, files, "hazelcast.yaml")
	assert.Contains(t, files, "jvm_options")
	assert.NotContains(t, files, "hazelcast-master.yaml")
	keys := sessionConfigMountKeys(cfg)
	assert.Contains(t, keys, "hazelcast.yaml")
	assert.NotContains(t, keys, "hazelcast-master.yaml")
	sep := sessionConfigFiles(&SessionConfig{MasterHTTPPort: 8080, Namespace: "default", HazelcastPort: 5801}, "demo")
	assert.Contains(t, sep, "hazelcast-master.yaml")
}
