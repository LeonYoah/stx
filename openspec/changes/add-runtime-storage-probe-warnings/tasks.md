<!--
Licensed to the Apache Software Foundation (ASF) under one or more
contributor license agreements.  See the NOTICE file distributed with
this work for additional information regarding copyright ownership.
The ASF licenses this file to You under the Apache License, Version 2.0
(the "License"); you may not use this file except in compliance with
the License.  You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
-->

## 1. 规格与范围

- [x] 1.1 明确第一阶段只做安装链路内真实 runtime probe，不直接替换安装前 validate 页面
- [x] 1.2 明确 probe 失败只 warning、不阻塞安装

## 2. Agent 安装链路

- [x] 2.1 新增 checkpoint / IMAP runtime probe request builder，复用现有存储配置映射
- [x] 2.2 新增 proxy 资产发现与 one-shot CLI 执行逻辑，支持 request/response JSON 文件交换
- [x] 2.3 在 `configure_checkpoint` 步骤中接入远端存储 runtime probe，失败仅 warning
- [x] 2.4 在 `configure_imap` 步骤中接入远端存储 runtime probe，失败仅 warning
- [x] 2.5 为 proxy 缺失、probe 失败、probe 成功场景补充 Agent 单测

## 3. 控制面与前端展示

- [x] 3.1 在安装状态模型中新增 `warnings` 字段
- [x] 3.2 在安装状态轮询中聚合 warning message，并保留去重后的 warnings
- [x] 3.3 在安装向导/进度页展示 warnings

## 4. 验证

- [x] 4.1 验证远端 checkpoint probe 失败后安装仍能继续
- [x] 4.2 验证远端 IMAP probe 失败后安装仍能继续
- [x] 4.3 验证 proxy 资产缺失时用户能看到明确 warning

## 5. 资产分发与打包

- [x] 5.1 调整根目录 `.gitignore`，移除对 `src/main` 的误伤规则
- [x] 5.2 将 stx-java-proxy 启动脚本统一维护到根 `scripts/` 目录，并保留旧工具目录包装入口
- [x] 5.3 在控制面发布包中分发 `lib/stx-java-proxy-{seatunnelVersion}.jar` 与 `scripts/stx-java-proxy.sh`
- [x] 5.4 在控制面 Agent 分发接口中提供 stx-java-proxy jar / script 下载端点
- [x] 5.5 在 Agent 安装脚本中下载并安装 stx-java-proxy 资产，并通过 support home / script 环境变量注入给 Agent 进程
- [x] 5.6 支持按 SeaTunnel 版本选择 stx-java-proxy jar，找不到时回退到 `2.3.13`
