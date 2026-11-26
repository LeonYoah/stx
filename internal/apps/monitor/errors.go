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

import "errors"

// Error definitions for monitor package.
// 监控包的错误定义。
var (
	// ErrConfigNotFound indicates the monitor config was not found.
	// ErrConfigNotFound 表示监控配置未找到。
	ErrConfigNotFound = errors.New("monitor config not found")

	// ErrInvalidMonitorInterval indicates the monitor interval is invalid.
	// ErrInvalidMonitorInterval 表示监控间隔无效。
	ErrInvalidMonitorInterval = errors.New("monitor interval must be between 1 and 60 seconds")

	// ErrKubernetesMonitorConfigUnsupported 表示 K8s 集群不支持 Agent 自愈配置。
	// ErrKubernetesMonitorConfigUnsupported indicates Kubernetes clusters do not support agent self-healing config.
	ErrKubernetesMonitorConfigUnsupported = errors.New("kubernetes cluster process self-healing is natively managed by kubelet RestartPolicy and Liveness/Readiness probes; agent self-healing config is not applicable")

	// ErrInvalidRestartDelay indicates the restart delay is invalid.
	// ErrInvalidRestartDelay 表示重启延迟无效。
	ErrInvalidRestartDelay = errors.New("restart delay must be between 1 and 300 seconds")

	// ErrInvalidMaxRestarts indicates the max restarts is invalid.
	// ErrInvalidMaxRestarts 表示最大重启次数无效。
	ErrInvalidMaxRestarts = errors.New("max restarts must be between 1 and 10")

	// ErrInvalidTimeWindow indicates the time window is invalid.
	// ErrInvalidTimeWindow 表示时间窗口无效。
	ErrInvalidTimeWindow = errors.New("time window must be between 60 and 3600 seconds")

	// ErrInvalidCooldownPeriod indicates the cooldown period is invalid.
	// ErrInvalidCooldownPeriod 表示冷却时间无效。
	ErrInvalidCooldownPeriod = errors.New("cooldown period must be between 60 and 86400 seconds")

	// ErrEventNotFound indicates the process event was not found.
	// ErrEventNotFound 表示进程事件未找到。
	ErrEventNotFound = errors.New("process event not found")
)
