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

// Package version 保存 STX 服务端和 CLI 共用的版本信息。
// Package version stores version information shared by the STX server and CLI.
package version

// Version 是当前 STX 发布版本，可在发布构建中通过 ldflags 覆盖。
// Version is the current STX release version and can be overridden with ldflags.
var Version = "0.1.0"

// MinCLIVersion 是当前服务端接受的最低 CLI 版本。
// MinCLIVersion is the minimum CLI version accepted by the current server.
var MinCLIVersion = "0.1.0"
