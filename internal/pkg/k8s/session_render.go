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

import "fmt"

// 本文件集中维护 stx_deployed 一键部署渲染进 ConfigMap 的全部配置文件模板，
// 严格对齐上游生产推荐（rest-api v1 关闭、seatunnel.yaml 负责 HTTP 监听、hazelcast k8s service-dns 发现）。
// This file holds all config file templates rendered into the ConfigMap for stx_deployed
// one-click clusters, strictly aligned with upstream production guidance (REST v1 disabled,
// seatunnel.yaml owns the HTTP listener, hazelcast kubernetes service-dns discovery).

// renderSeatunnelYAML 渲染 seatunnel.yaml：HTTP 监听端口来自 master_http_port，指标开启。
// renderSeatunnelYAML renders seatunnel.yaml: the HTTP listener port comes from
// master_http_port and telemetry metrics stay enabled.
func renderSeatunnelYAML(cfg *SessionConfig) string {
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
      enable-http: true
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
          namespace: /tmp/seatunnel/checkpoint_snapshot/
          storage.type: hdfs
          fs.defaultFS: file:///tmp/
    telemetry:
      metric:
        enabled: true
`, cfg.MasterHTTPPort)
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
%s`, clusterCode, clusterCode, cfg.Namespace, cfg.HazelcastPort, cfg.HazelcastPort, extra)
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
	return `# JVM Dump
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
	return map[string]string{
		"seatunnel.yaml":       renderSeatunnelYAML(cfg),
		"hazelcast-master.yaml": renderHazelcastMemberYAML(cfg, clusterCode, "master"),
		"hazelcast-worker.yaml": renderHazelcastMemberYAML(cfg, clusterCode, "worker"),
		"hazelcast-client.yaml": renderHazelcastClientYAML(cfg, clusterCode),
		"jvm_master_options":   renderJVMOptions(),
		"jvm_worker_options":   renderJVMOptions(),
		"jvm_client_options":   renderJVMOptions(),
		"log4j2.properties":    renderLog4j2(),
	}
}

// sessionConfigMountKeys 是容器内 /opt/seatunnel/config 下按 subPath 挂载的文件清单。
// sessionConfigMountKeys lists files mounted into /opt/seatunnel/config via subPath.
func sessionConfigMountKeys() []string {
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
