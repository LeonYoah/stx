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
	"fmt"
	"strings"
)

// 本文件集中维护 stx_deployed 一键部署渲染进 ConfigMap 的全部配置文件模板，
// 严格对齐上游生产推荐（rest-api v1 关闭、seatunnel.yaml 负责 HTTP 监听、hazelcast k8s service-dns 发现）。
// This file holds all config file templates rendered into the ConfigMap for stx_deployed
// one-click clusters, strictly aligned with upstream production guidance (REST v1 disabled,
// seatunnel.yaml owns the HTTP listener, hazelcast kubernetes service-dns discovery).

// renderSeatunnelYAML 渲染 seatunnel.yaml：HTTP 监听端口来自 master_http_port，指标开启；
// Runtime/Checkpoint 草稿存在时按部署向导输入动态渲染。
// renderSeatunnelYAML renders seatunnel.yaml: the HTTP listener port comes from
// master_http_port and telemetry metrics stay enabled; Runtime/Checkpoint drafts
// render dynamically when the deploy wizard supplied them.
func renderSeatunnelYAML(cfg *SessionConfig) string {
	// enable-http 恒为 true：控制面一切运行时能力（/overview、作业提交、事件、监控）
	// 都依赖引擎 REST，关闭等于创建即残废，因此不暴露关闭路径。
	// enable-http is always true: every control-plane runtime capability (/overview,
	// job submission, events, metrics) depends on the engine REST API, so turning it
	// off yields a dead-on-arrival cluster and the disable path is not exposed.
	enableHTTP := "true"
	checkpointPlugin := `          namespace: /tmp/seatunnel/checkpoint_snapshot/
          storage.type: hdfs
          fs.defaultFS: file:///tmp/`
	if cp := cfg.Checkpoint; cp != nil && cp.Enabled {
		checkpointPlugin = renderStoragePluginConfig(cp, "/tmp/seatunnel/checkpoint_snapshot/")
	}
	return fmt.Sprintf(`seatunnel:
  engine:
    history-job-expire-minutes: 1440
    backup-count: 1
    queue-type: blockingqueue
    print-execution-info-interval: 60
    print-job-metrics-info-interval: 60
    classloader-cache-mode: true
    slot-service:
      dynamic-slot: true
    http:
      enable-http: %s
      port: %d
      enable-dynamic-port: false
      port-range: 100
    checkpoint:
      interval: 300000
      timeout: 10000
      storage:
        type: hdfs
        max-retained: 3
        plugin-config:
%s
    telemetry:
      metric:
        enabled: true
`, enableHTTP, cfg.MasterHTTPPort, checkpointPlugin)
}

// renderStoragePluginConfig 把存储草稿渲染为 plugin-config 行；LOCAL_FILE 走 file:// 本地路径。
// renderStoragePluginConfig renders a storage draft into plugin-config lines; LOCAL_FILE
// maps to file:// local paths.
func renderStoragePluginConfig(spec *SessionStorageSpec, defaultNamespace string) string {
	namespace := strings.TrimSpace(spec.Namespace)
	if namespace == "" {
		namespace = defaultNamespace
	}
	storageType := strings.ToUpper(strings.TrimSpace(spec.StorageType))
	lines := []string{"          namespace: " + namespace}
	switch storageType {
	case "", "LOCAL_FILE":
		lines = append(lines,
			"          storage.type: hdfs",
			"          fs.defaultFS: file:///tmp/")
	case "S3":
		lines = append(lines,
			"          storage.type: s3",
			"          fs.s3a.endpoint: "+strings.TrimSpace(spec.Endpoint))
		if spec.Bucket != "" {
			lines = append(lines, "          s3.bucket: "+strings.TrimSpace(spec.Bucket))
		}
		if spec.AccessKey != "" {
			lines = append(lines, "          fs.s3a.access.key: "+strings.TrimSpace(spec.AccessKey))
		}
		if spec.SecretKey != "" {
			lines = append(lines, "          fs.s3a.secret.key: "+strings.TrimSpace(spec.SecretKey))
		}
	case "OSS":
		lines = append(lines,
			"          storage.type: oss",
			"          fs.oss.endpoint: "+strings.TrimSpace(spec.Endpoint))
		if spec.Bucket != "" {
			lines = append(lines, "          oss.bucket: "+strings.TrimSpace(spec.Bucket))
		}
		if spec.AccessKey != "" {
			lines = append(lines, "          fs.oss.accessKeyId: "+strings.TrimSpace(spec.AccessKey))
		}
		if spec.SecretKey != "" {
			lines = append(lines, "          fs.oss.accessKeySecret: "+strings.TrimSpace(spec.SecretKey))
		}
	case "HDFS":
		lines = append(lines,
			"          storage.type: hdfs",
			"          fs.defaultFS: "+strings.TrimSpace(spec.Endpoint))
	default:
		lines = append(lines,
			"          storage.type: hdfs",
			"          fs.defaultFS: file:///tmp/")
	}
	// 透传向导额外键值（HA/Kerberos 等高级项）。
	// Pass through wizard extras (HA/Kerberos advanced keys).
	for key, value := range spec.Extra {
		lines = append(lines, "          "+key+": "+value)
	}
	return strings.Join(lines, "\n")
}

// renderHazelcastMemberYAML 渲染 master/worker 共用的 Hazelcast 成员配置（worker 追加 member-attributes）。
// renderHazelcastMemberYAML renders the shared hazelcast member config for master/worker
// (worker appends member-attributes).
func renderHazelcastMemberYAML(cfg *SessionConfig, clusterCode, role string) string {
	extra := ""
	if role == "worker" {
		extra = `  member-attributes:
    rule:
      type: string
      value: worker
`
	}
	return fmt.Sprintf(`hazelcast:
  cluster-name: %s
  network:
    rest-api:
      enabled: false
      endpoint-groups:
        CLUSTER_WRITE:
          enabled: true
        DATA:
          enabled: true
    join:
      kubernetes:
        enabled: true
        service-dns: %s.%s.svc.cluster.local
        service-port: %d
    port:
      auto-increment: false
      port: %d
  properties:
    hazelcast.invocation.max.retry.count: 20
    hazelcast.tcp.join.port.try.count: 30
    hazelcast.logging.type: log4j2
    hazelcast.operation.generic.thread.count: 50
    hazelcast.heartbeat.failuredetector.type: phi-accrual
    hazelcast.heartbeat.interval.seconds: 2
    hazelcast.max.no.heartbeat.seconds: 180
    hazelcast.heartbeat.phiaccrual.failuredetector.threshold: 10
    hazelcast.heartbeat.phiaccrual.failuredetector.sample.size: 200
    hazelcast.heartbeat.phiaccrual.failuredetector.min.std.dev.millis: 100
%s`, clusterCode, clusterCode, cfg.Namespace, cfg.HazelcastPort, cfg.HazelcastPort, extra) + renderIMAPMapStoreYAML(cfg)
}

// renderIMAPMapStoreYAML 渲染 IMAP map-store 段（追加到 hazelcast yaml 末尾）；禁用或空草稿返回空。
// renderIMAPMapStoreYAML renders the IMAP map-store section appended to the hazelcast
// yaml; disabled or empty drafts yield nothing.
func renderIMAPMapStoreYAML(cfg *SessionConfig) string {
	imap := cfg.IMAP
	if imap == nil {
		return ""
	}
	if !imap.Enabled || strings.EqualFold(strings.TrimSpace(imap.StorageType), "DISABLED") {
		return `  map:
    engine*:
      map-store:
        enabled: false
`
	}
	pluginLines := renderStoragePluginConfig(imap, "imap-data")
	// 缩进对齐 map-store.properties 层级（8 空格 → 10 空格）。
	// Re-indent plugin lines to the map-store properties level.
	indented := strings.ReplaceAll(pluginLines, "\n          ", "\n            ")
	indented = strings.TrimPrefix(indented, "          ")
	return `  map:
    engine*:
      map-store:
        enabled: true
        initial-mode: EAGER
        factory-class-name: org.apache.seatunnel.engine.server.persistence.FileMapStoreFactory
        properties:
            ` + indented + `
`
}

// renderHazelcastClientYAML 渲染客户端配置，master 的 REST 提交与成员发现都走集群 Service。
// renderHazelcastClientYAML renders the client config; REST submissions and member discovery
// both go through the cluster service.
func renderHazelcastClientYAML(cfg *SessionConfig, clusterCode string) string {
	return fmt.Sprintf(`hazelcast-client:
  cluster-name: %s
  properties:
    hazelcast.logging.type: log4j2
  connection-strategy:
    connection-retry:
      cluster-connect-timeout-millis: 3000
  network:
    cluster-members:
      - %s.%s.svc.cluster.local:%d
`, clusterCode, clusterCode, cfg.Namespace, cfg.HazelcastPort)
}

// renderJVMOptions 渲染 JVM 选项文件（master/worker 同一份保守默认）。
// renderJVMOptions renders the JVM options file (one conservative default shared by
// master and worker).
func renderJVMOptions() string {
	return renderJVMOptionsWithConfig(nil)
}

// renderJVMOptionsWithConfig 按 JVM 草稿渲染堆内存与附加参数；空草稿保持保守默认。
// renderJVMOptionsWithConfig renders heap and extra options from the JVM draft; an
// empty draft keeps the conservative default.
func renderJVMOptionsWithConfig(jvm *SessionJVMConfig) string {
	heap := "-Xms2g -Xmx2g"
	extra := ""
	if jvm != nil {
		xms := strings.TrimSpace(jvm.Xms)
		xmx := strings.TrimSpace(jvm.Xmx)
		if xms != "" || xmx != "" {
			if xms == "" {
				xms = "2g"
			}
			if xmx == "" {
				xmx = "2g"
			}
			heap = "-Xms" + xms + " -Xmx" + xmx
		}
		for _, opt := range jvm.ExtraOptions {
			if trimmed := strings.TrimSpace(opt); trimmed != "" {
				extra += trimmed + "\n"
			}
		}
	}
	return heap + "\n" + extra + `# JVM Dump
-XX:+HeapDumpOnOutOfMemoryError
-XX:HeapDumpPath=/tmp/seatunnel/dump/zeta-server

# Metaspace
-XX:MaxMetaspaceSize=2g

# G1GC
-XX:+UseG1GC
`
}

// renderLog4j2 渲染 log4j2.properties，日志写容器内 /tmp/seatunnel/logs（控制台也可读 stdout）。
// renderLog4j2 renders log4j2.properties writing to the in-container /tmp/seatunnel/logs
// (stdout remains readable as well).
func renderLog4j2() string {
	return `monitorInterval = 60

property.file_path = ${sys:seatunnel.logs.path:-/tmp/seatunnel/logs}
property.file_name = ${sys:seatunnel.logs.file_name:-seatunnel}
property.file_split_size = 100MB
property.file_count = 100
property.file_ttl = 7d

rootLogger.level = INFO

logger.zeta.name=org.apache.seatunnel.engine
logger.zeta.level=INFO

logger.jetty.name=org.eclipse.jetty
logger.jetty.level=WARN

logger.hazelcast.name=com.hazelcast
logger.hazelcast.level=INFO

appender.console.type = Console
appender.console.name = LogToConsole
appender.console.layout.type = PatternLayout
appender.console.layout.pattern = %d{yyyy-MM-dd HH:mm:ss,SSS} %-5p [%-30c{1}] [%t] - %m%n

rootLogger.appenderRef.stdout.ref = LogToConsole
`
}

// sessionConfigFiles 返回 ConfigMap 应包含的文件名到内容的映射。
// sessionConfigFiles returns the file name to content mapping for the ConfigMap.
func sessionConfigFiles(cfg *SessionConfig, clusterCode string) map[string]string {
	// hybrid 模式成员/客户端合一：单一 hazelcast.yaml 与 jvm_options。
	// Hybrid mode unifies member and client config into a single hazelcast.yaml and
	// jvm_options file.
	if cfg.IsHybrid() {
		return map[string]string{
			"seatunnel.yaml":        renderSeatunnelYAML(cfg),
			"hazelcast.yaml":        renderHazelcastMemberYAML(cfg, clusterCode, "hybrid"),
			"hazelcast-client.yaml": renderHazelcastClientYAML(cfg, clusterCode),
			"jvm_options":           renderJVMOptionsWithConfig(cfg.JVM),
			"jvm_client_options":    renderJVMOptions(),
			"log4j2.properties":     renderLog4j2(),
		}
	}
	return map[string]string{
		"seatunnel.yaml":        renderSeatunnelYAML(cfg),
		"hazelcast-master.yaml": renderHazelcastMemberYAML(cfg, clusterCode, "master"),
		"hazelcast-worker.yaml": renderHazelcastMemberYAML(cfg, clusterCode, "worker"),
		"hazelcast-client.yaml": renderHazelcastClientYAML(cfg, clusterCode),
		"jvm_master_options":    renderJVMOptionsWithConfig(cfg.JVM),
		"jvm_worker_options":    renderJVMOptionsWithConfig(cfg.JVM),
		"jvm_client_options":    renderJVMOptions(),
		"log4j2.properties":     renderLog4j2(),
	}
}

// sessionConfigMountKeys 是容器内 /opt/seatunnel/config 下按 subPath 挂载的文件清单。
// sessionConfigMountKeys lists files mounted into /opt/seatunnel/config via subPath.
func sessionConfigMountKeys(cfg ...*SessionConfig) []string {
	if len(cfg) > 0 && cfg[0] != nil && cfg[0].IsHybrid() {
		return []string{
			"hazelcast-client.yaml",
			"hazelcast.yaml",
			"jvm_client_options",
			"jvm_options",
			"log4j2.properties",
			"seatunnel.yaml",
		}
	}
	return []string{
		"hazelcast-client.yaml",
		"hazelcast-master.yaml",
		"hazelcast-worker.yaml",
		"jvm_client_options",
		"jvm_master_options",
		"jvm_worker_options",
		"log4j2.properties",
		"seatunnel.yaml",
	}
}
