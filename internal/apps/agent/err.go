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

// Package agent provides Agent connection management for the STX Control Plane.
// agent 包提供 STX Control Plane 的 Agent 连接管理功能。
package agent

// Error messages for Agent Manager operations (defined in manager.go)
// Agent Manager 操作的错误消息（在 manager.go 中定义）
// - ErrAgentNotFound: Agent not found / 未找到 Agent
// - ErrAgentNotConnected: Agent not connected / Agent 未连接
// - ErrCommandTimeout: Command execution timeout / 命令执行超时
// - ErrStreamNotAvailable: Command stream not available / 命令流不可用
