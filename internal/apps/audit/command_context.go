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

package audit

import "context"

type commandMetadataContextKey struct{}

// CommandMetadata 保存 Agent 命令与用户请求、公共执行之间的关联。
// CommandMetadata stores links between an Agent command, the user request, and the shared execution.
type CommandMetadata struct {
	RequestID   string
	ExecutionID string
	OwnerUserID uint
}

// WithCommandMetadata 把 Agent 命令关联信息写入上下文。
// WithCommandMetadata attaches Agent command linkage metadata to a context.
func WithCommandMetadata(ctx context.Context, metadata CommandMetadata) context.Context {
	return context.WithValue(ctx, commandMetadataContextKey{}, metadata)
}

// CommandMetadataFromContext 从上下文读取 Agent 命令关联信息。
// CommandMetadataFromContext reads Agent command linkage metadata from a context.
func CommandMetadataFromContext(ctx context.Context) CommandMetadata {
	if ctx == nil {
		return CommandMetadata{}
	}
	metadata, _ := ctx.Value(commandMetadataContextKey{}).(CommandMetadata)
	return metadata
}
